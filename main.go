package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	db, err := openDB()
	if err != nil {
		log.Fatalf("PinNote: %v", err)
	}
	defer db.Close()

	handle := &appHandle{}
	noteService := NewNoteService(db)
	windowService := NewWindowService(handle, db)

	app := application.New(application.Options{
		Name:        "PinNote",
		Description: "打开即写、自动保存、启动恢复的桌面便签",
		Services: []application.Service{
			application.NewService(noteService),
			application.NewService(windowService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Keep running when all pin windows are closed so notes stay
			// reachable via the global shortcut; the menu bar allows quitting.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	handle.app = app
	noteService.SetApp(handle)

	// Every live note gets a desktop pin window on startup; the main panel is
	// gone (ADR-0001) and ⌘⌥N is the only way to create a note.
	newNote := func() {
		note, err := noteService.CreateNote("")
		if err != nil {
			log.Printf("create note: %v", err)
			return
		}
		if _, err := windowService.OpenPinnedWindow(note.ID); err != nil {
			log.Printf("open pin window: %v", err)
		}
	}

	// Menu-bar fallbacks for the keyboard-first flows (spec §4.4). On macOS
	// the AppMenu role must come first (it provides the standard application
	// menu with Quit), and the fallback actions live in a submenu — menu bar
	// entries are submenus; AppKit drops leaf items. EditMenu is required:
	// ⌘V/⌘C/⌘X in the webviews are dispatched through the menu's paste:/copy:
	// selectors, and without it every clipboard shortcut silently dies.
	menu := application.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
		menu.AddRole(application.EditMenu)
	}
	actions := menu.AddSubmenu("PinNote")
	actions.Add("新建笔记").SetAccelerator("CmdOrCtrl+Option+N").OnClick(func(*application.Context) { newNote() })
	actions.Add("删除当前笔记").SetAccelerator("CmdOrCtrl+Backspace").OnClick(func(*application.Context) {
		if err := windowService.RequestFrontmostDelete(); err != nil {
			log.Printf("request frontmost delete: %v", err)
		}
	})
	actions.Add("切换主题").SetAccelerator("CmdOrCtrl+Shift+D").OnClick(func(*application.Context) {
		next := "light"
		if windowService.Theme() == "light" {
			next = "dark"
		}
		if err := windowService.SetTheme(next); err != nil {
			log.Printf("switch theme from menu: %v", err)
		}
	})
	if runtime.GOOS != "darwin" {
		menu.AddSeparator()
		menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	}
	app.Menu.SetApplicationMenu(menu)

	// Global shortcut: summon a fresh pin window from anywhere.
	if err := app.GlobalShortcut.Register("CmdOrCtrl+Option+N", newNote); err != nil {
		log.Printf("register global shortcut: %v", err)
	}

	// Left click on the menu bar icon is the mouse-side way back into a
	// resident app: focus the most recently updated note, creating the first
	// note when the desktop has none. The context menu's 显示笔记列表 covers the
	// "show me the whole index" case instead.
	summonLatest := func() {
		latest, err := noteService.latestLiveNote()
		if err != nil {
			log.Printf("summon latest note: %v", err)
			return
		}
		if latest == nil {
			newNote()
			return
		}
		if _, err := windowService.OpenPinnedWindow(latest.ID); err != nil {
			log.Printf("summon latest note: %v", err)
		}
	}
	setupTray(app, windowService.pinBackground(), newNote, summonLatest)

	// Permanently remove notes whose 60-day trash window has expired.
	if purged, err := noteService.PurgeExpiredTrash(); err != nil {
		log.Printf("purge expired trash: %v", err)
	} else if purged > 0 {
		log.Printf("purged %d expired note(s) from trash", purged)
	}

	// Restore a window for every live note from the previous session.
	RestoreAllNoteWindows(noteService, windowService)

	err = app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
