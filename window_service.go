package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// appHandle is the seam between services and the running Wails application.
// It lets services emit events without importing a circular dependency on the
// concrete app instance created in main.
type appHandle struct {
	app *application.App
	mu  sync.Mutex
}

func (h *appHandle) emit(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.app != nil {
		h.app.Event.Emit(name, data)
	}
}

// on subscribes to an application event once the app exists. The returned
// function removes the listener; it is a no-op when there is no app yet.
func (h *appHandle) on(name string, callback func(*application.CustomEvent)) func() {
	h.mu.Lock()
	app := h.app
	h.mu.Unlock()
	if app == nil {
		return func() {}
	}
	return app.Event.On(name, callback)
}

// pinnedWindowPrefix is the window-name prefix for pin windows; the note id is
// the remainder.
const pinnedWindowPrefix = "pin-"

const (
	// flushTopic asks every pin window to persist its editor buffer; it
	// carries the token of the round being answered.
	flushTopic = "pin:flush-requested"

	// flushAckTopic is the answer to flushTopic.
	flushAckTopic = "pin:flushed"
)

// flushRequest is the barrier payload sent to every renderer.
type flushRequest struct {
	Token string `json:"token"`
}

// flushRound is one in-flight barrier: the token identifying it plus the
// per-window channels its answers land in. An answer must carry that token, so
// a window that was slow on a previous round cannot be mistaken for consent on
// this one.
type flushRound struct {
	token   string
	waiters map[string]chan bool
	acked   int
}

// flushAnswer is a decoded pin:flushed.
type flushAnswer struct {
	token  string
	noteID string
	dirty  bool
}

// WindowService manages the pin windows: one frameless always-on-top window
// per live note. It is also the single source of truth for the theme, which
// is persisted in the settings table and broadcast to every window.
type WindowService struct {
	handle *appHandle
	db     *sql.DB

	mu    sync.Mutex
	theme string // "light" or "dark"; drives native window background colours.

	flushMu  sync.Mutex
	flushing *flushRound

	// Barrier seams: enumerating and addressing windows requires a running
	// application, so both are injectable and the protocol stays testable
	// (same style as noteWindowOpener).
	listPins  func() []string
	broadcast func(name string, data any)
}

func NewWindowService(h *appHandle, db *sql.DB) *WindowService {
	theme, _, err := getSetting(db, "theme")
	if err != nil {
		log.Printf("read theme setting: %v", err)
	}
	if theme != "light" {
		theme = "dark"
	}
	s := &WindowService{handle: h, db: db, theme: theme}
	s.listPins = s.livePinNoteIDs
	s.broadcast = h.emit
	return s
}

func (s *WindowService) Theme() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.theme
}

// pinBackground returns the native window backdrop colour for the current
// theme. It mirrors the frontend's --pin-bg value.
func (s *WindowService) pinBackground() application.RGBA {
	if s.Theme() == "light" {
		return application.NewRGBA(0xfa, 0xfa, 0xfb, 0xf2)
	}
	return application.NewRGBA(0x1e, 0x1f, 0x22, 0xf2)
}

// GetTheme exposes the persisted theme so each window can apply it at
// startup (the Go side is the single source of truth).
func (s *WindowService) GetTheme() string { return s.Theme() }

// SetTheme persists the theme, repaints the native backdrop of every open
// window, and broadcasts theme:changed so all windows (and the antd
// ConfigProvider) switch in the same frame.
func (s *WindowService) SetTheme(mode string) error {
	if mode != "light" && mode != "dark" {
		return fmt.Errorf("unknown theme %q", mode)
	}
	if err := setSetting(s.db, "theme", mode); err != nil {
		return fmt.Errorf("persist theme: %w", err)
	}
	s.mu.Lock()
	s.theme = mode
	s.mu.Unlock()
	if s.handle.app != nil {
		for _, win := range s.handle.app.Window.GetAll() {
			win.SetBackgroundColour(s.pinBackground())
		}
		s.handle.emit("theme:changed", mode)
	}
	return nil
}

// PinnedWindowName returns the internal window name for a pinned note.
func PinnedWindowName(noteID string) string { return pinnedWindowPrefix + noteID }

// OpenPinnedWindow opens a frameless, always-on-top desktop window showing
// the given note. If the window already exists it is focused instead. Returns
// true when a new window was created. Height is user-draggable (no content
// auto-fit), minimum 150px.
func (s *WindowService) OpenPinnedWindow(noteID string) (bool, error) {
	if s.handle.app == nil {
		return false, fmt.Errorf("application not ready")
	}
	name := PinnedWindowName(noteID)
	if win, ok := s.handle.app.Window.GetByName(name); ok {
		win.Show()
		win.Focus()
		return false, nil
	}

	// Anchor the window near the top-right of the screen so it feels like a
	// desktop widget; the window size only changes by user dragging.
	win := s.handle.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            "PinNote",
		Width:            380,
		Height:           300,
		MinHeight:        150,
		Frameless:        true,
		AlwaysOnTop:      true,
		URL:              "/#/pin/" + noteID,
		BackgroundColour: s.pinBackground(),
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
		Mac: application.MacWindow{
			Backdrop:    application.MacBackdropTranslucent,
			WindowLevel: application.MacWindowLevelFloating,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
	})
	if win == nil {
		return false, fmt.Errorf("create pinned window for note %s", noteID)
	}
	win.Show()
	return true, nil
}

// ClosePinnedWindow closes the pinned window for a note if it is open.
func (s *WindowService) ClosePinnedWindow(noteID string) error {
	if s.handle.app == nil {
		return fmt.Errorf("application not ready")
	}
	if win, ok := s.handle.app.Window.GetByName(PinnedWindowName(noteID)); ok {
		win.Close()
	}
	return nil
}

// HidePanel hides the tray panel. The panel frontend calls it after focusing
// a note (or creating one) so the panel collapses like a popover.
func (s *WindowService) HidePanel() error {
	if s.handle.app == nil {
		return fmt.Errorf("application not ready")
	}
	if win, ok := s.handle.app.Window.GetByName(panelWindowName); ok {
		win.Hide()
	}
	return nil
}

// RequestFrontmostDelete asks the frontmost pin window to delete its note.
// The frontend owns the flush-then-trash sequence (an in-flight edit must be
// persisted before trashing), so this forwards the request with the note id —
// window EmitEvent is an app-wide broadcast, so every window filters on the
// id. It is the menu-bar fallback for ⌘⌫.
func (s *WindowService) RequestFrontmostDelete() error {
	if s.handle.app == nil {
		return fmt.Errorf("application not ready")
	}
	for _, win := range s.handle.app.Window.GetAll() {
		if !strings.HasPrefix(win.Name(), pinnedWindowPrefix) || !win.IsFocused() {
			continue
		}
		win.EmitEvent("pin:delete-requested", strings.TrimPrefix(win.Name(), pinnedWindowPrefix))
		return nil
	}
	return nil
}

// noteWindowOpener abstracts window creation so the restore logic can be
// tested without a running application.
type noteWindowOpener interface {
	OpenPinnedWindow(noteID string) (bool, error)
}

// RestoreAllNoteWindows reopens a window for every live note. Called once at
// startup so the whole desktop comes back after an app restart — closing a
// window is only "collapse"; the note (and its window) returns on relaunch.
func RestoreAllNoteWindows(notes *NoteService, windows noteWindowOpener) {
	list, err := notes.ListNotes()
	if err != nil {
		log.Printf("list notes at startup: %v", err)
		return
	}
	for _, n := range list {
		if _, err := windows.OpenPinnedWindow(n.ID); err != nil {
			log.Printf("restore note window for %s: %v", n.ID, err)
		}
	}
}

// livePinNoteIDs lists the notes that currently have a pin window, which is
// exactly the set that must be persisted before anything is allowed to quit.
func (s *WindowService) livePinNoteIDs() []string {
	if s.handle.app == nil {
		return nil
	}
	var ids []string
	for _, win := range s.handle.app.Window.GetAll() {
		if name := win.Name(); strings.HasPrefix(name, pinnedWindowPrefix) {
			ids = append(ids, strings.TrimPrefix(name, pinnedWindowPrefix))
		}
	}
	return ids
}

// startFlushListener wires the ack channel. Called once from main, after the
// application handle has been set.
func (s *WindowService) startFlushListener() {
	s.handle.on(flushAckTopic, s.receiveFlushAck)
}

// requestFlushAll asks every open pin window to persist its buffer and waits
// for all of them to answer. ADR-0003 D7: the built-in card's Restart button
// quits through the updater itself with no veto available on the Go side, so
// the swap is only safe once no renderer holds text that has not reached the
// database. Anything short of a full set of clean answers is an error,
// including silence.
func (s *WindowService) requestFlushAll(timeout time.Duration) error {
	targets := s.listPins()
	if len(targets) == 0 {
		return nil
	}
	token := newID()
	round := &flushRound{token: token, waiters: make(map[string]chan bool, len(targets))}
	for _, id := range targets {
		round.waiters[id] = make(chan bool, 1)
	}
	s.flushMu.Lock()
	if s.flushing != nil {
		s.flushMu.Unlock()
		return fmt.Errorf("another flush is already running")
	}
	s.flushing = round
	s.flushMu.Unlock()
	defer func() {
		s.flushMu.Lock()
		s.flushing = nil
		s.flushMu.Unlock()
	}()

	s.broadcast(flushTopic, flushRequest{Token: token})

	deadline := time.After(timeout)
	for _, id := range targets {
		select {
		case dirty := <-round.waiters[id]:
			if dirty {
				return fmt.Errorf("note %s still has unsaved edits", id)
			}
		case <-deadline:
			s.flushMu.Lock()
			missing := len(targets) - round.acked
			s.flushMu.Unlock()
			return fmt.Errorf("%d pin window(s) did not confirm saving", missing)
		}
	}
	return nil
}

// receiveFlushAck records one window answer.
func (s *WindowService) receiveFlushAck(e *application.CustomEvent) {
	s.flushMu.Lock()
	round := s.flushing
	s.flushMu.Unlock()
	if round == nil || e == nil {
		return
	}
	answer := decodeFlushAck(e)
	if answer.token != round.token {
		return
	}
	waiter, ok := round.waiters[answer.noteID]
	if !ok {
		return
	}
	select {
	case waiter <- answer.dirty:
		s.flushMu.Lock()
		round.acked++
		s.flushMu.Unlock()
	default:
		// Already answered for this round.
	}
}

// decodeFlushAck pulls the round token, note id and dirty flag out of an ack.
// Unregistered Wails events arrive as decoded JSON, so the payload is read
// defensively instead of being unmarshalled into a struct; a missing or
// mistyped token therefore answers as "not this round", which is the safe
// direction. The sending window name wins over the payload: a renderer must not
// be able to ack for a note it does not own.
func decodeFlushAck(e *application.CustomEvent) flushAnswer {
	data, _ := e.Data.(map[string]any)
	answer := flushAnswer{
		token:  stringField(data, "token"),
		noteID: stringField(data, "noteID"),
	}
	if sender := strings.TrimPrefix(e.Sender, pinnedWindowPrefix); sender != e.Sender && sender != "" {
		answer.noteID = sender
	}
	answer.dirty, _ = data["dirty"].(bool)
	return answer
}

func stringField(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}
