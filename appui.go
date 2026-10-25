package main

import (
	"database/sql"
	"log"

	"github.com/egoist/mygo"
)

// pinApp is the shared state of the running app. Every mutation happens on
// the main thread (view callbacks, tray and menu callbacks) or hops to it
// (RunOnMain), so the caches below need no locks of their own.
type pinApp struct {
	db      *sql.DB
	notes   *NoteService
	windows *WindowService
	updates *UpdateService
	panel   *Panel
	trash   *TrashPanel
	search  *SearchPanel
	tray    *mygo.Tray // anchors every index panel under the menu-bar icon

	// notesCache mirrors the live notes (updated_at desc) for the panel and
	// the pin windows; trashCache mirrors the trashed notes (deleted_at
	// desc) for the trash panel, and trashSet holds their ids so a window
	// can show its deleted-note placeholder instead of vanishing.
	notesCache []Note
	trashCache []Note
	trashSet   map[string]bool

	// quitting is set by OnBeforeQuit so window close handlers let the
	// app-quit close pass through without their collapse flows.
	quitting bool
}

// wire connects the services to the shared state. Call once before the app
// is ready; onChange may fire from any goroutine, so it hops to the main
// thread.
func (a *pinApp) wire() {
	a.notes.SetOnChange(func() {
		mygo.RunOnMain(a.refreshNotes)
	})
}

// refreshNotes reloads the caches and syncs every window with them. Main
// thread only.
func (a *pinApp) refreshNotes() {
	list, err := a.notes.ListNotes()
	if err != nil {
		log.Printf("list notes: %v", err)
		return
	}
	a.notesCache = list

	trash, err := a.notes.ListTrash()
	if err != nil {
		log.Printf("list trash: %v", err)
		return
	}
	a.trashCache = trash
	a.trashSet = map[string]bool{}
	for _, n := range trash {
		a.trashSet[n.ID] = true
	}

	for _, p := range a.windows.pins {
		p.syncWithCaches()
	}
	if a.panel != nil {
		a.panel.win.Update(func() {})
	}
	if a.trash != nil {
		a.trash.win.Update(func() {})
	}
	if a.search != nil {
		// An open search panel keeps matching the same query as the notes
		// change under it (a note saved in a pin window, one trashed).
		a.search.win.Update(a.search.reload)
	}
}

// hideIndexPanels hides every index panel but keep, so the tray never stacks
// two popovers under the icon and ⌘⇧F never opens over the list it was
// called from.
func (a *pinApp) hideIndexPanels(keep *mygo.Window) {
	if a.panel != nil && a.panel.win != keep {
		a.panel.Hide()
	}
	if a.trash != nil && a.trash.win != keep {
		a.trash.Hide()
	}
	if a.search != nil && a.search.win != keep {
		a.search.Hide()
	}
}

// ShowNotesPanel opens 显示笔记列表 under the tray icon.
func (a *pinApp) ShowNotesPanel() {
	if a.panel == nil {
		return
	}
	a.hideIndexPanels(a.panel.win)
	showWindowAtTray(a.panel.win, a.tray)
}

// ShowTrashPanel opens 回收站 under the tray icon.
func (a *pinApp) ShowTrashPanel() {
	if a.trash == nil {
		return
	}
	a.hideIndexPanels(a.trash.win)
	showWindowAtTray(a.trash.win, a.tray)
}

// ShowSearch opens the ⌘⇧F search panel, empty and ready to type.
func (a *pinApp) ShowSearch() {
	if a.search == nil {
		return
	}
	a.hideIndexPanels(a.search.win)
	a.search.present()
	showWindowAtTray(a.search.win, a.tray)
}

// liveNote returns the cached live note with the given id.
func (a *pinApp) liveNote(id string) (Note, bool) {
	for _, n := range a.notesCache {
		if n.ID == id {
			return n, true
		}
	}
	return Note{}, false
}

// newNote creates a note and opens its pin window — the ⌘⌥N flow.
func (a *pinApp) newNote() {
	note, err := a.notes.CreateNote("")
	if err != nil {
		log.Printf("create note: %v", err)
		return
	}
	if _, err := a.windows.OpenPinnedWindow(note.ID); err != nil {
		log.Printf("open pin window: %v", err)
	}
}

// summonLatest focuses the most recently updated note, creating the first
// note when the desktop has none — the tray icon's left click.
func (a *pinApp) summonLatest() {
	latest, err := a.notes.latestLiveNote()
	if err != nil {
		log.Printf("summon latest note: %v", err)
		return
	}
	if latest == nil {
		a.newNote()
		return
	}
	if _, err := a.windows.OpenPinnedWindow(latest.ID); err != nil {
		log.Printf("summon latest note: %v", err)
	}
}
