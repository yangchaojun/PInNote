package main

// The trash panel — the 60-day recycle bin's index, opened from the tray
// menu's 回收站 item (issue #8). Rows restore a note (its pin window comes
// back with it); 清空回收站 permanently drops everything after a
// confirmation. Like the notes panel it is an index, not a resident UI: it
// hides as soon as it loses focus.

import (
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TrashPanel is the trash index window.
type TrashPanel struct {
	app *pinApp
	win *mygo.Window

	confirmEmpty bool // the 清空回收站 button's confirmation dialog
}

// trashDaysLeft reports how many whole days remain before the startup purge
// drops the note, 0 on its last day.
func trashDaysLeft(deletedAt int64) int64 {
	left := TrashRetentionDays - int64(time.Since(time.Unix(deletedAt, 0)).Hours()/24)
	if left < 0 {
		return 0
	}
	return left
}

func (p *TrashPanel) view(c *ui.Context) {
	t := c.Theme()
	trash := p.app.trashCache

	ui.Column(c).Fill().Background(t.Background).Padding(8).Gap(6).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "回收站").FontSize(13).Bold().Grow(1)
			if len(trash) > 0 {
				if ui.Button(c, "清空回收站").FontSize(11).Height(22).TextColor(t.Danger).Clicked() {
					p.confirmEmpty = true
				}
			}
		})
		if len(trash) == 0 {
			p.emptyState(c, t)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, n := range trash {
					p.row(c, t, n)
				}
			})
		})
		p.emptyDialog(c, t)
	})
}

func (p *TrashPanel) emptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
		ui.Icon(c, trashIcon).FontSize(26).TextColor(t.TextMuted).Label("回收站")
		ui.Text(c, "回收站是空的").TextColor(t.TextMuted)
		ui.Text(c, "删除的笔记在这里保留 60 天，到期自动清除。").FontSize(11).TextColor(t.TextMuted)
	})
}

func (p *TrashPanel) row(c *ui.Context, t *ui.Theme, n Note) {
	title := n.Title
	if title == "" {
		title = "无标题笔记"
	}
	var deleted string
	if n.DeletedAt != nil {
		deleted = time.Unix(*n.DeletedAt, 0).Format("1月2日 15:04")
	}
	row := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 8).
		Background(t.Surface).Radius(6).Label("已删除笔记 " + title).Tooltip(title)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, clampRunes(title, 24)).FontSize(13)
			if excerpt := noteExcerpt(n.Content, 64); excerpt != "" {
				ui.Text(c, clampRunes(excerpt, 30)).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Text(c, deleted+" 删除 · "+formatDaysLeft(trashDaysLeft(*n.DeletedAt))).
				FontSize(11).TextColor(t.TextMuted)
		})
		if ui.Button(c, "恢复").FontSize(11).Height(22).Clicked() {
			p.restore(n.ID)
		}
	})
}

// formatDaysLeft spells the purge countdown; the last day reads as 今天,
// which a bare 0 would not.
func formatDaysLeft(days int64) string {
	if days <= 0 {
		return "今天清除"
	}
	return fmt.Sprintf("%d 天后清除", days)
}

// restore moves a note back to the live list and brings its pin window
// along — restoring is asking for the note back, so show it.
func (p *TrashPanel) restore(id string) {
	if _, err := p.app.notes.RestoreNote(id); err != nil {
		log.Printf("restore note: %v", err)
		return
	}
	if _, err := p.app.windows.OpenPinnedWindow(id); err != nil {
		log.Printf("open restored note window: %v", err)
	}
}

// emptyDialog is 清空回收站's second confirmation: emptying is a bulk
// permanent delete with no undo.
func (p *TrashPanel) emptyDialog(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &p.confirmEmpty, func() {
		count := len(p.app.trashCache)
		ui.Column(c).Width(300).Gap(10).Children(func() {
			ui.Text(c, "清空回收站？").FontSize(13).Bold()
			ui.Text(c, "将永久删除全部 "+fmt.Sprint(count)+" 条笔记，此操作不可恢复。").FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					p.confirmEmpty = false
				}
				if ui.Button(c, "清空").TextColor(t.Danger).Clicked() {
					p.confirmEmpty = false
					if err := p.app.notes.EmptyTrash(); err != nil {
						log.Printf("empty trash: %v", err)
					}
				}
			})
		})
	})
}

func (p *TrashPanel) Hide() { p.win.Hide() }
