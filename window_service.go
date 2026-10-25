package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/egoist/mygo"
)

// WindowService manages the pin windows — one frameless always-on-top window
// per live note — and is the single source of truth for the theme, persisted
// in the settings table and applied app-wide through mygo.Theme.
type WindowService struct {
	db *sql.DB

	theme string // "light" or "dark"; read on the main thread
	pins  map[string]*PinWindow
	app   *pinApp

	// applyTheme pushes a theme into the app-wide look; a seam so tests can
	// run without a UI backend.
	applyTheme func(mode string)
}

func NewWindowService(db *sql.DB) *WindowService {
	theme, _, err := getSetting(db, "theme")
	if err != nil {
		log.Printf("read theme setting: %v", err)
	}
	if theme != "light" {
		theme = "dark"
	}
	s := &WindowService{db: db, theme: theme, pins: map[string]*PinWindow{}}
	s.applyTheme = func(mode string) { mygo.Theme.SetSource(themeSource(mode)) }
	return s
}

// SetApp wires the shared app state once it exists in main.
func (s *WindowService) SetApp(a *pinApp) { s.app = a }

func (s *WindowService) Theme() string { return s.theme }

// ApplyTheme pushes the persisted theme into the app's look. Called once at
// startup; later switches go through SetTheme.
func (s *WindowService) ApplyTheme() {
	s.applyTheme(s.theme)
}

func themeSource(mode string) mygo.ThemeSource {
	if mode == "light" {
		return mygo.ThemeLight
	}
	return mygo.ThemeDark
}

// SetTheme persists the theme and switches every window in the same repaint:
// mygo.Theme drives all views, which rebuild their backgrounds from it.
func (s *WindowService) SetTheme(mode string) error {
	if mode != "light" && mode != "dark" {
		return fmt.Errorf("unknown theme %q", mode)
	}
	if err := setSetting(s.db, "theme", mode); err != nil {
		return fmt.Errorf("persist theme: %w", err)
	}
	s.theme = mode
	s.applyTheme(mode)
	return nil
}

// ToggleTheme flips between light and dark (⌘⇧D).
func (s *WindowService) ToggleTheme() {
	next := "light"
	if s.theme == "light" {
		next = "dark"
	}
	if err := s.SetTheme(next); err != nil {
		log.Printf("switch theme: %v", err)
	}
}

// OpenPinnedWindow opens the pin window of the given note, focusing it when
// it already exists. Returns true when a new window was created.
func (s *WindowService) OpenPinnedWindow(noteID string) (bool, error) {
	if p, ok := s.pins[noteID]; ok {
		p.win.Show()
		p.win.Focus()
		return false, nil
	}
	p, err := newPinWindow(s.app, noteID)
	if err != nil {
		return false, err
	}
	s.pins[noteID] = p
	return true, nil
}

// ClosePinnedWindow collapses the pin window of a note if it is open.
func (s *WindowService) ClosePinnedWindow(noteID string) {
	if p, ok := s.pins[noteID]; ok {
		p.collapse()
	}
}

// RequestFrontmostDelete asks the focused pin window to delete its note —
// the menu-bar fallback for ⌫⌘.
func (s *WindowService) RequestFrontmostDelete() {
	for _, p := range s.pins {
		if p.win.IsFocused() {
			p.deleteNote()
			return
		}
	}
}

// FlushAll persists every open editor buffer synchronously. All documents
// live in this process, so the update barrier of the webview era collapses
// into one direct call (ADR-0003 D7's intent, stronger than its protocol).
func (s *WindowService) FlushAll() {
	for _, p := range s.pins {
		p.flush()
	}
}

// livePinNoteIDs lists the notes that currently have a pin window.
func (s *WindowService) livePinNoteIDs() []string {
	ids := make([]string, 0, len(s.pins))
	for id := range s.pins {
		ids = append(ids, id)
	}
	return ids
}

// forget drops a closed window from the registry (called by the window).
func (s *WindowService) forget(noteID string) {
	delete(s.pins, noteID)
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
