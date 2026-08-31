package main

import (
	"fmt"
	"log"
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

// WindowService creates and manages the note windows: the main list window
// and one frameless always-on-top window per pinned note.
type WindowService struct {
	handle *appHandle

	mu    sync.Mutex
	theme string // "light" or "dark"; drives native window background colours.
}

func NewWindowService(h *appHandle) *WindowService {
	return &WindowService{handle: h, theme: "dark"}
}

func (s *WindowService) Theme() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.theme
}

// mainBackground / pinBackground return the native window backdrop colours for
// the current theme. They mirror the frontend's --bg-top and --pin-bg values.
func (s *WindowService) mainBackground() application.RGBA {
	if s.Theme() == "light" {
		return application.NewRGBA(0xf7, 0xf8, 0xfa, 0xff)
	}
	return application.NewRGB(0x16, 0x17, 0x1a)
}

func (s *WindowService) pinBackground() application.RGBA {
	if s.Theme() == "light" {
		return application.NewRGBA(0xfa, 0xfa, 0xfb, 0xf2)
	}
	return application.NewRGBA(0x1e, 0x1f, 0x22, 0xf2)
}

// SetTheme records the frontend theme and repaints the native backdrop of
// every open window so the uncovered/under-construction window surface matches.
// Called by the frontend at startup and whenever the theme toggles.
func (s *WindowService) SetTheme(mode string) error {
	if s.handle.app == nil {
		return fmt.Errorf("application not ready")
	}
	if mode != "light" && mode != "dark" {
		return fmt.Errorf("unknown theme %q", mode)
	}
	s.mu.Lock()
	s.theme = mode
	s.mu.Unlock()
	for _, win := range s.handle.app.Window.GetAll() {
		if win.Name() == "main" {
			win.SetBackgroundColour(s.mainBackground())
		} else {
			win.SetBackgroundColour(s.pinBackground())
		}
	}
	return nil
}

// PinnedWindowName returns the internal window name for a pinned note.
func PinnedWindowName(noteID string) string { return "pin-" + noteID }

// OpenPinnedWindow opens a frameless, always-on-top desktop window showing
// the given note. If the window already exists it is focused instead. Returns
// true when a new window was created.
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
	// desktop widget; the frontend fine-tunes the size to fit the content.
	win := s.handle.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            name,
		Title:           "PinNote",
		Width:           380,
		Height:          300,
		Frameless:       true,
		AlwaysOnTop:     true,
		URL:             "/#/pin/" + noteID,
		BackgroundColour: s.pinBackground(),
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
		Mac: application.MacWindow{
			Backdrop:            application.MacBackdropTranslucent,
			WindowLevel:         application.MacWindowLevelFloating,
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

// FocusMainWindow brings the main note list window to the front.
func (s *WindowService) FocusMainWindow() error {
	if s.handle.app == nil {
		return fmt.Errorf("application not ready")
	}
	if win, ok := s.handle.app.Window.GetByName("main"); ok {
		win.Show()
		win.Focus()
	}
	return nil
}

// restorePinnedWindows reopens a window for every pinned note that is not in
// the trash. Called once at startup so pins survive an app restart.
func restorePinnedWindows(h *appHandle, notes *NoteService, windows *WindowService) {
	list, err := notes.ListNotes()
	if err != nil {
		log.Printf("list notes at startup: %v", err)
		return
	}
	for _, n := range list {
		if n.Pinned && n.DeletedAt == nil {
			if _, err := windows.OpenPinnedWindow(n.ID); err != nil {
				log.Printf("restore pinned window for %s: %v", n.ID, err)
			}
		}
	}
}
