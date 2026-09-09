package main

import (
	_ "embed"
	"log"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets/trayicon.png
var trayIcon []byte

// trayIconColor is the accent-tinted icon for platforms without template
// images (SetTemplateIcon is a no-op outside macOS): it stays visible on both
// light and dark taskbars.
//
//go:embed assets/trayicon-color.png
var trayIconColor []byte

// panelWindowName is the tray panel's internal window name (route /#/panel).
const panelWindowName = "panel"

// setupTray creates the menu-bar status item — the resident app's visible
// anchor and its mouse-side entry point. Left click summons the most recently
// updated note (creating the first one when no note exists), because reaching
// your work is the common case; the context menu holds 新建笔记 / 显示笔记列表
// / 检查更新 / 退出, where 显示笔记列表 opens the note index at /#/panel. The
// panel is an index, not a resident UI, so it hides as soon as it loses focus.
// Updates are announced here rather than in a window: with no resident UI, the
// menu bar is the only surface that can say "there is a new version" (D4).
func setupTray(app *application.App, pinBackground application.RGBA, newNote, summonLatest func(), updates *UpdateService) {
	panel := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             panelWindowName,
		Title:            "PinNote",
		Width:            280,
		Height:           360,
		Frameless:        true,
		Hidden:           true,
		DisableResize:    true,
		URL:              "/#/panel",
		BackgroundColour: pinBackground,
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
	if panel == nil {
		log.Printf("create tray panel window: tray entry points unavailable")
		return
	}

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(trayIconColor)
	}
	tray.SetTooltip("PinNote")

	showPanel := func() {
		if err := tray.PositionWindow(panel, 0); err != nil {
			log.Printf("position tray panel: %v", err)
		}
		panel.Show()
		panel.Focus()
	}

	menu := application.NewMenu()
	menu.Add("新建笔记").OnClick(func(*application.Context) { newNote() })
	menu.Add("显示笔记列表").OnClick(func(*application.Context) { showPanel() })
	menu.AddSeparator()
	// Update entries sit between the note actions and 退出 because the tray is
	// the only place a pin-only app can announce a release (ADR-0003 D4).
	checkItem := menu.Add("检查更新…")
	applyItem := menu.Add("更新到新版本…")
	applyItem.SetHidden(true)
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	checkItem.OnClick(func(*application.Context) { updates.CheckNow() })
	applyItem.OnClick(func(*application.Context) { updates.CheckNow() })
	updates.OnStatusChanged(func(st UpdateStatus) {
		refreshTray(tray, checkItem, applyItem, st)
	})
	refreshTray(tray, checkItem, applyItem, updates.Status())

	tray.AttachWindow(panel)

	panel.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
		panel.Hide()
	})

	tray.OnClick(summonLatest)
}

// refreshTray mirrors the updater onto the two surfaces a pin-only app has: the
// menu bar icon and the context menu. The card owns the rich UI, so this stays
// a badge plus two labels (ADR-0003 D4).
func refreshTray(tray *application.SystemTray, checkItem, applyItem *application.MenuItem, st UpdateStatus) {
	pending := st.State == stateAvailable || st.State == stateDownloading ||
		st.State == stateVerifying || st.State == stateInstalling || st.State == stateReady

	if runtime.GOOS == "darwin" && !pending {
		// A template image follows the menu bar appearance; the accent-tinted
		// one doubles as the badge.
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(trayIconColor)
	}

	switch {
	case pending && st.Version != "":
		tray.SetTooltip("PinNote · 发现新版本 v" + st.Version)
	case st.State == stateChecking:
		tray.SetTooltip("PinNote · 正在检查更新…")
	case st.State == stateError && st.LastError != "":
		tray.SetTooltip("PinNote · " + st.LastError)
	default:
		tray.SetTooltip("PinNote")
	}

	if pending && st.Version != "" {
		applyItem.SetHidden(false)
		applyItem.SetLabel("更新到 v" + st.Version + "…")
	} else {
		applyItem.SetHidden(true)
	}

	if st.CanSelfUpdate {
		checkItem.SetLabel("检查更新…")
	} else {
		// The guard said no, so the only honest action left is handing the user
		// the .dmg (ADR-0003 D8).
		checkItem.SetLabel("打开下载页（.dmg）")
	}
	checkItem.SetTooltip(trayCheckTooltip(st))
}

// trayCheckTooltip explains the 检查更新 item: either why it is really a
// download-page shortcut, or when the last automatic look happened.
func trayCheckTooltip(st UpdateStatus) string {
	if !st.CanSelfUpdate {
		return st.GuardReason
	}
	if st.LastCheckedAt == "" {
		return ""
	}
	at, err := time.Parse(time.RFC3339, st.LastCheckedAt)
	if err != nil {
		return ""
	}
	return "上次检查：" + at.Local().Format("01-02 15:04")
}
