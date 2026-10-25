package main

// confirmAsk is the panels' second confirmation, asked in its own flyout
// window beside the asking panel (issue #10): a ui.Modal inside a tray
// popover is clipped by the popover's small bounds, while a flyout may
// extend past its parent and stays in the display's work area.

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type confirmAsk struct {
	parent *mygo.Window // the panel asking; the flyout hands the keyboard back
	win    *mygo.Window // the open flyout, nil while closed
	active bool         // from open to OnClosed, before win is even assigned

	title     string
	message   string
	action    string
	onConfirm func()
}

// open asks title/message beside anchor, action naming the confirming
// button. Asking again while open replaces the question.
func (a *confirmAsk) open(anchor ui.Rect, title, message, action string, onConfirm func()) {
	a.title, a.message, a.action, a.onConfirm = title, message, action, onConfirm
	if a.win != nil {
		a.win.SetAnchor(flyoutAnchor(anchor))
		a.win.Update(func() {})
		return
	}
	a.active = true // set first: the flyout's keyboard can blur the panel before win is assigned
	a.win = mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:    a.parent,
		Anchor:    flyoutAnchor(anchor),
		Gap:       4,
		Width:     300,
		Height:    130,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(a.view),
	})
	a.win.OnClosed(func() {
		a.win = nil
		a.active = false
		a.onConfirm = nil
		a.parent.Focus()
	})
}

func (a *confirmAsk) view(c *ui.Context) {
	t := c.Theme()
	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().Background(t.Surface).Padding(12).Gap(10)
	switch runtime.GOOS {
	case "darwin":
		// The shadow outlines what the content draws; Windows outlines and
		// rounds the window itself; only Linux needs a border.
		box.Radius(10)
	case "windows":
	default:
		box.Radius(10).Border(1, t.Border)
	}
	box.Children(func() {
		ui.Text(c, a.title).FontSize(13).Bold()
		ui.Text(c, a.message).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.win.Close()
			}
			if ui.Button(c, a.action).TextColor(t.Danger).Clicked() {
				onConfirm := a.onConfirm
				a.win.Close()
				onConfirm()
			}
		})
	})
}

// flyoutAnchor adapts an element's bounds to a flyout anchor.
func flyoutAnchor(r ui.Rect) mygo.Rectangle {
	return mygo.Rectangle{X: int(r.X), Y: int(r.Y), Width: int(r.W), Height: int(r.H)}
}
