package main

import (
	_ "embed"
	"log"
	"runtime"

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
// / 退出, where 显示笔记列表 opens the note index at /#/panel. The panel is an
// index, not a resident UI, so it hides as soon as it loses focus.
func setupTray(app *application.App, pinBackground application.RGBA, newNote, summonLatest func()) {
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
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	tray.AttachWindow(panel)

	panel.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
		panel.Hide()
	})

	tray.OnClick(summonLatest)
}
