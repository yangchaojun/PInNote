package main

import (
	_ "embed"
	"log"
	"runtime"
	"sync"
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
// anchor and its mouse-side entry point. Left click toggles the tray panel
// (an index of live notes at /#/panel); right click opens the context menu
// (新建笔记 / 显示全部笔记 / 退出). The panel hides when it loses focus; the
// custom click handler refuses to re-show it within a grace window because
// on macOS the blur caused by clicking the status item fires before the
// click itself, and the hide-then-show race would leave the panel open.
func setupTray(app *application.App, pinBackground application.RGBA, newNote, showAllNotes func()) {
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

	menu := application.NewMenu()
	menu.Add("新建笔记").OnClick(func(*application.Context) { newNote() })
	menu.Add("显示全部笔记").OnClick(func(*application.Context) { showAllNotes() })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	tray.AttachWindow(panel)

	var blurMu sync.Mutex
	var blurredAt time.Time
	panel.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) {
		blurMu.Lock()
		blurredAt = time.Now()
		blurMu.Unlock()
		panel.Hide()
	})

	tray.OnClick(func() {
		if panel.IsVisible() {
			panel.Hide()
			return
		}
		blurMu.Lock()
		lastBlur := blurredAt
		blurMu.Unlock()
		if time.Since(lastBlur) < 300*time.Millisecond {
			return
		}
		if err := tray.PositionWindow(panel, 0); err != nil {
			log.Printf("position tray panel: %v", err)
		}
		panel.Show()
		panel.Focus()
	})
}
