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
}

func NewWindowService(h *appHandle) *WindowService {
	return &WindowService{handle: h}
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
		BackgroundColour: application.NewRGBA(0x1e, 0x1f, 0x22, 0xf2),
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
