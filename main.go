package main

// PinNote — 打开即写、自动保存、启动恢复的桌面便签。每条笔记就是一个
// 常驻桌面的 pin 窗口：无边框、置顶、跨 Space，没有主面板（ADR-0001）。
// Native UI 形态：mygo 绘制的纯 Go 界面，无 webview、无前端构建。

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
	updaterui "github.com/egoist/mygo/plugins/updater"
	nativeui "github.com/egoist/mygo/plugins/updater/native"
)

func main() {
	db, err := openDB()
	if err != nil {
		log.Fatalf("PinNote: %v", err)
	}
	defer db.Close()

	app := &pinApp{db: db}
	app.notes = NewNoteService(db)
	app.windows = NewWindowService(db)
	app.updates = NewUpdateService(db, app.windows)
	app.windows.SetApp(app)
	app.wire()

	// The framework update card, in native UI so no webview rides along.
	// Automatic checks stay off: our own loop drives the tray badge and
	// never opens anything on its own (ADR-0003 D2).
	mygo.Use(nativeui.New(updaterui.Options{DisableAutomaticChecks: true}))

	// Keep running when all pin windows are closed so notes stay reachable
	// via the global shortcut; the menu bar allows quitting.
	mygo.App.OnWindowAllClosed(func() {})

	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { app.quitting = true })
	// 退出收尾 (§5.2): flush every editor, then apply the empty-note rule —
	// a never-typed note is deleted rather than left as clutter. The editors
	// live in this process, so this synchronous pass is the whole barrier.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		app.windows.FlushAll()
		for _, id := range app.windows.livePinNoteIDs() {
			if _, err := app.notes.DiscardIfEmpty(id); err != nil {
				log.Printf("discard empty note on quit: %v", err)
			}
		}
	})

	mygo.App.WhenReady(func() {
		app.windows.ApplyTheme()
		app.refreshNotes()

		// Menu-bar fallbacks for the keyboard-first flows (spec §4.4). On
		// macOS the AppMenu role must come first; EditMenu carries the
		// clipboard shortcuts into the editor.
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Role: mygo.RoleEditMenu},
			{Label: "PinNote", Submenu: []*mygo.MenuItem{
				{Label: "新建笔记", Accelerator: "CmdOrCtrl+Option+N", Click: func(*mygo.MenuItem, *mygo.Window) { app.newNote() }},
				{Label: "删除当前笔记", Accelerator: "CmdOrCtrl+Backspace", Click: func(*mygo.MenuItem, *mygo.Window) {
					app.windows.RequestFrontmostDelete()
				}},
				{Label: "切换主题", Accelerator: "CmdOrCtrl+Shift+D", Click: func(*mygo.MenuItem, *mygo.Window) {
					app.windows.ToggleTheme()
				}},
			}},
		})
		if runtime.GOOS != "darwin" {
			menu.Append(mygo.Separator())
			menu.Append(&mygo.MenuItem{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }})
		}
		mygo.App.SetMenu(menu)

		// Global shortcut: summon a fresh pin window from anywhere.
		if err := mygo.GlobalShortcut.Register("CmdOrCtrl+Option+N", app.newNote); err != nil {
			log.Printf("register global shortcut: %v", err)
		}

		setupTray(app)

		// Permanently remove notes whose 60-day trash window has expired.
		if purged, err := app.notes.PurgeExpiredTrash(); err != nil {
			log.Printf("purge expired trash: %v", err)
		} else if purged > 0 {
			log.Printf("purged %d expired note(s) from trash", purged)
		}

		// Restore a window for every live note from the previous session.
		RestoreAllNoteWindows(app.notes, app.windows)

		app.updates.Start()
		defer app.updates.Stop()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
