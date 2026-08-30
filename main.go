package main

import (
	"embed"
	"log"

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
	windowService := NewWindowService(handle)

	app := application.New(application.Options{
		Name:        "PinNote",
		Description: "键盘优先的桌面便签笔记",
		Services: []application.Service{
			application.NewService(noteService),
			application.NewService(windowService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Keep running when the main window closes so pinned desktop
			// notes stay alive; the menu-bar app still allows quitting.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	handle.app = app
	noteService.SetApp(handle)

	// Main window: note list + markdown editor.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "PinNote",
		Width:     980,
		Height:    640,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(0x16, 0x17, 0x1a),
	})

	// Global shortcut: summon PinNote and start a new note from anywhere.
	if err := app.GlobalShortcut.Register("CmdOrCtrl+Option+N", func() {
		if err := windowService.FocusMainWindow(); err != nil {
			log.Printf("focus main window: %v", err)
		}
		handle.emit("app:new-note", nil)
	}); err != nil {
		log.Printf("register global shortcut: %v", err)
	}

	// Permanently remove notes whose 60-day trash window has expired.
	if purged, err := noteService.PurgeExpiredTrash(); err != nil {
		log.Printf("purge expired trash: %v", err)
	} else if purged > 0 {
		log.Printf("purged %d expired note(s) from trash", purged)
	}

	// Reopen pinned windows from the previous session.
	restorePinnedWindows(handle, noteService, windowService)

	err = app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
