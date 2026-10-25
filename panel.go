package main

// The tray panel — a small index of live notes opened from the tray menu's
// 显示笔记列表 item (the port of TrayPanel.tsx). It is not a main panel
// (ADR-0001 retired that): rows focus the note's pin window or delete it
// (with a second confirmation, issue #6), the empty state offers a single
// create button, and the window hides as soon as a row is chosen or focus
// is lost. Sorted most-recently-updated first.

import (
	"log"
	"regexp"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	excerptImageRe = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	excerptLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	excerptMarkRe  = regexp.MustCompile(`[#>*` + "`" + `~_]+`)
	excerptSpaceRe = regexp.MustCompile(`\s+`)
)

// noteExcerpt is the body after the title line: markdown markers stripped
// (images dropped, links reduced to their text), whitespace collapsed,
// truncated. Ported from TrayPanel.tsx.
func noteExcerpt(content string, max int) string {
	lines := strings.Split(content, "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first == -1 {
		return ""
	}
	plain := excerptImageRe.ReplaceAllString(strings.Join(lines[first+1:], " "), "")
	plain = excerptLinkRe.ReplaceAllString(plain, "$1")
	plain = excerptMarkRe.ReplaceAllString(plain, "")
	plain = excerptSpaceRe.ReplaceAllString(plain, " ")
	plain = strings.TrimSpace(plain)
	runes := []rune(plain)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return plain
}

// Panel is the tray panel window.
type Panel struct {
	app *pinApp
	win *mygo.Window

	// confirmDeleteID is the note a row's delete button asked about; the
	// dialog stays open until it is answered or dismissed.
	confirmDeleteID string
	confirmDelete   bool
}

func (p *Panel) view(c *ui.Context) {
	t := c.Theme()
	notes := p.app.notesCache

	ui.Column(c).Fill().Background(t.Background).Padding(8).Children(func() {
		if len(notes) == 0 {
			p.emptyState(c, t)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, n := range notes {
					p.row(c, t, n)
				}
			})
		})
		p.deleteDialog(c, t)
	})
}

func (p *Panel) emptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
		ui.Icon(c, pushpinIcon).FontSize(26).TextColor(t.TextMuted)
		ui.Text(c, "还没有笔记").TextColor(t.TextMuted)
		if ui.PrimaryButton(c, "新建笔记").Clicked() {
			p.focusOrCreate("")
		}
	})
}

func (p *Panel) row(c *ui.Context, t *ui.Theme, n Note) {
	title := n.Title
	if title == "" {
		title = "无标题笔记"
	}
	excerpt := noteExcerpt(n.Content, 64)
	row := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 8).
		Background(t.Surface).Radius(6).Label("打开笔记 " + title).
		Tooltip(title).OnClick(func() { p.focusOrCreate(n.ID) })
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, clampRunes(title, 24)).FontSize(13)
			if excerpt != "" {
				ui.Text(c, clampRunes(excerpt, 30)).FontSize(11).TextColor(t.TextMuted)
			}
		})
		// The engine hands a click to the innermost interactive element, so
		// this button never fires the row's open action (issue #6).
		if ui.Button(c, "").Label("删除笔记 " + title).MinWidth(24).Height(22).Children(func() {
			ui.Icon(c, trashIcon).FontSize(13)
		}).Clicked() {
			p.confirmDeleteID = n.ID
			p.confirmDelete = true
		}
	})
}

// deleteDialog is a row delete button's second confirmation (issue #6). The
// panel stays open afterwards: deleting is not a jump into a note.
func (p *Panel) deleteDialog(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &p.confirmDelete, func() {
		title := "无标题笔记"
		if n, ok := p.app.liveNote(p.confirmDeleteID); ok && n.Title != "" {
			title = n.Title
		}
		ui.Column(c).Width(300).Gap(10).Children(func() {
			ui.Text(c, "删除笔记「"+clampRunes(title, 20)+"」？").FontSize(13).Bold()
			ui.Text(c, "删除后进入回收站保留 60 天，到期自动清除；回收站没有恢复入口。").FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					p.confirmDelete = false
				}
				if ui.Button(c, "删除").TextColor(t.Danger).Clicked() {
					p.confirmDelete = false
					if _, err := p.app.notes.TrashNote(p.confirmDeleteID); err != nil {
						log.Printf("trash note from panel: %v", err)
					}
				}
			})
		})
	})
}

// focusOrCreate focuses a note's pin window (creating the note when id is
// empty) and collapses the panel like a popover.
func (p *Panel) focusOrCreate(id string) {
	if id == "" {
		p.app.newNote()
	} else if _, err := p.app.windows.OpenPinnedWindow(id); err != nil {
		log.Printf("focus note: %v", err)
	}
	p.Hide()
}

func (p *Panel) Hide() { p.win.Hide() }

func clampRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
