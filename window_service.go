package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

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

// WindowService manages the pin windows: one frameless always-on-top window
// per live note. It is also the single source of truth for the theme, which
// is persisted in the settings table and broadcast to every window.
type WindowService struct {
	handle *appHandle
	db     *sql.DB

	mu    sync.Mutex
	theme string // "light" or "dark"; drives native window background colours.
}

func NewWindowService(h *appHandle, db *sql.DB) *WindowService {
	theme, _, err := getSetting(db, "theme")
	if err != nil {
		log.Printf("read theme setting: %v", err)
	}
	if theme != "light" {
		theme = "dark"
	}
	return &WindowService{handle: h, db: db, theme: theme}
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
func PinnedWindowName(noteID string) string { return "pin-" + noteID }

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
		if !strings.HasPrefix(win.Name(), "pin-") || !win.IsFocused() {
			continue
		}
		win.EmitEvent("pin:delete-requested", strings.TrimPrefix(win.Name(), "pin-"))
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
