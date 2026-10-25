package main

import (
	_ "embed"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/trayicon.png
var trayIcon []byte

// trayIconColor is the accent-tinted icon for platforms without template
// images, and doubles as the "update available" badge on macOS.
//
//go:embed assets/trayicon-color.png
var trayIconColor []byte

// setupTray creates the menu-bar status item — the resident app's visible
// anchor and its mouse-side entry point. Left click summons the most
// recently updated note (creating the first one when no note exists), the
// context menu holds 新建笔记 / 显示笔记列表 / 回收站 / 检查更新 / 退出, and the
// two index items open their panel under the icon. The panels are indexes,
// not a resident UI, so each hides as soon as it loses focus. Updates are
// announced here rather than in a window: with no resident UI, the menu bar
// is the only surface that can say "there is a new version" (ADR-0003 D4).
func setupTray(app *pinApp) {
	panel := &Panel{app: app}
	panel.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "PinNote",
		Width:             280,
		Height:            360,
		Hidden:            true,
		Frameless:         true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Content:           ui.View(panel.view),
	})
	panel.win.SetVisibleOnAllWorkspaces(true)
	panel.win.OnBlur(panel.Hide)
	panel.win.OnClose(func(e *mygo.CloseEvent) {
		// Esc / ⌘W dismiss the panel; a real quit passes through.
		if !app.quitting {
			e.PreventDefault()
			panel.Hide()
		}
	})
	app.panel = panel

	// The trash panel (issue #8) is the same kind of popover: an index of
	// the 60-day recycle bin, not a resident UI.
	trashPanel := &TrashPanel{app: app}
	trashPanel.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "PinNote",
		Width:             300,
		Height:            380,
		Hidden:            true,
		Frameless:         true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Content:           ui.View(trashPanel.view),
	})
	trashPanel.win.SetVisibleOnAllWorkspaces(true)
	trashPanel.win.OnBlur(trashPanel.Hide)
	trashPanel.win.OnClose(func(e *mygo.CloseEvent) {
		if !app.quitting {
			e.PreventDefault()
			trashPanel.Hide()
		}
	})
	app.trash = trashPanel

	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon,
		IconIsTemplate: runtime.GOOS == "darwin",
		ToolTip:        "PinNote",
	})
	if err != nil {
		log.Printf("create tray: %v (tray entry points unavailable)", err)
		return
	}

	// One popover at a time: opening either index hides the other, so the
	// tray never stacks two panels under the icon.
	showPanel := func() {
		trashPanel.Hide()
		showWindowAtTray(panel.win, tray)
	}
	showTrash := func() {
		panel.Hide()
		showWindowAtTray(trashPanel.win, tray)
	}

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "新建笔记", Click: func(*mygo.MenuItem, *mygo.Window) { app.newNote() }},
		{Label: "显示笔记列表", Click: func(*mygo.MenuItem, *mygo.Window) { showPanel() }},
		{Label: "回收站", Click: func(*mygo.MenuItem, *mygo.Window) { showTrash() }},
		mygo.Separator(),
		// Update entries sit between the note actions and 退出 because the
		// tray is the only place a pin-only app can announce a release
		// (ADR-0003 D4).
		{ID: "check", Label: "检查更新…", Click: func(*mygo.MenuItem, *mygo.Window) { app.updates.CheckNow() }},
		{ID: "apply", Label: "更新到新版本…", Hidden: true, Click: func(*mygo.MenuItem, *mygo.Window) { app.updates.CheckNow() }},
		mygo.Separator(),
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	applyItem := menu.ItemByID("apply")
	checkItem := menu.ItemByID("check")

	if runtime.GOOS == "darwin" {
		// No TrayOptions.Menu: it would consume the primary click. Install
		// the menu for the popup only, so left clicks keep summoning.
		tray.OnClick(app.summonLatest)
		tray.OnRightClick(func() {
			panel.Hide()
			trashPanel.Hide()
			tray.SetMenu(menu)
			defer tray.SetMenu(nil)
			tray.PopUpMenu()
		})
	} else {
		tray.SetIcon(trayIconColor, false)
		tray.SetMenu(menu)
	}

	app.updates.OnStatusChanged(func(st UpdateStatus) {
		refreshTray(tray, checkItem, applyItem, st)
	})
	refreshTray(tray, checkItem, applyItem, app.updates.Status())
}

// showWindowAtTray places a panel window under the tray icon on its display
// and shows it focused.
func showWindowAtTray(win *mygo.Window, tray *mygo.Tray) {
	anchor := tray.Bounds()
	if anchor.Width == 0 || anchor.Height == 0 {
		win.Center()
	} else {
		area := mygo.Screen.DisplayNearestPoint(mygo.Point{
			X: anchor.X + anchor.Width/2,
			Y: anchor.Y + anchor.Height/2,
		}).WorkArea
		b := win.Bounds()
		x := anchor.X + (anchor.Width-b.Width)/2
		y := anchor.Y + anchor.Height + 6
		if y+b.Height > area.Y+area.Height {
			y = anchor.Y - b.Height - 6
		}
		x = max(area.X, min(x, area.X+max(0, area.Width-b.Width)))
		y = max(area.Y, min(y, area.Y+max(0, area.Height-b.Height)))
		win.SetPosition(x, y)
	}
	win.Show()
	win.Focus()
}

// refreshTray mirrors the updater onto the two surfaces a pin-only app has:
// the menu bar icon and the context menu. The card owns the rich UI, so this
// stays a badge plus two labels (ADR-0003 D4).
func refreshTray(tray *mygo.Tray, checkItem, applyItem *mygo.MenuItem, st UpdateStatus) {
	pending := st.State == stateAvailable

	if runtime.GOOS == "darwin" && !pending {
		// A template image follows the menu bar appearance; the
		// accent-tinted one doubles as the badge.
		tray.SetIcon(trayIcon, true)
	} else {
		tray.SetIcon(trayIconColor, false)
	}

	switch {
	case pending && st.Version != "":
		tray.SetToolTip("PinNote · 发现新版本 v" + st.Version)
	case st.State == stateChecking:
		tray.SetToolTip("PinNote · 正在检查更新…")
	case st.State == stateError && st.LastError != "":
		tray.SetToolTip("PinNote · " + st.LastError)
	default:
		tray.SetToolTip("PinNote")
	}

	if pending && st.Version != "" {
		applyItem.SetVisible(true)
		applyItem.SetLabel("更新到 v" + st.Version + "…")
	} else {
		applyItem.SetVisible(false)
	}

	if st.CanSelfUpdate {
		checkItem.SetLabel("检查更新…")
	} else {
		// The guard said no, so the only honest action left is handing the
		// user the release page (ADR-0003 D8).
		checkItem.SetLabel("打开下载页")
	}
	// ToolTip on menu items is not supported; the label carries the state.
}
