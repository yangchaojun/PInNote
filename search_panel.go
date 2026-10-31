package main

// The search panel — ⌘⇧F's index of every live note (issue #12). It takes
// the tray panel's form: a lightweight index, not a resident UI (ADR-0001).
// A query field sits over the note list; Up and Down choose, Enter focuses
// the note's pin window, Escape dismisses. Matching is one LIKE pass in
// SQLite (NoteService.SearchNotes), so neither the schema nor the markdown
// fact source changes.

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/search.svg
var searchSVG []byte

// searchIcon is the query row's glyph.
var searchIcon = func() *ui.SVG {
	icon, err := ui.ParseSVG(searchSVG)
	if err != nil {
		panic("parse search.svg: " + err.Error())
	}
	return icon
}()

// SearchPanel is the ⌘⇧F window and its keyboard-navigable result list. All
// fields are main-thread.
type SearchPanel struct {
	app     *pinApp
	win     *mygo.Window
	query   string
	results []Note
	chosen  int // the highlighted row, -1 when there are none
	list    ui.ListState
	input   ui.Handle // the query field, focused whenever the panel opens
}

func (p *SearchPanel) view(c *ui.Context) {
	t := c.Theme()

	// The keys are claimed here rather than by the list, because the query
	// field holds the keyboard: without this, Up and Down would move the
	// caret instead of the choice.
	if c.Shortcut(0, ui.KeyDown) {
		p.choose(p.chosen + 1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		p.choose(p.chosen - 1)
	}
	if c.Shortcut(0, ui.KeyEnter) {
		p.openAt(p.chosen)
	}
	if c.Shortcut(0, ui.KeyEscape) {
		p.Hide()
	}

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(10, 10, 6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(14).TextColor(t.TextMuted)
			if ui.TextInputBase(c, &p.query).Bind(&p.input).Grow(1).FontSize(14).
				Placeholder("搜索笔记…").Label("搜索笔记").Changed() {
				// A new query starts at the top of its results.
				p.chosen = 0
				p.reload()
			}
		})
		ui.Divider(c)

		p.list.Selected = &p.chosen
		p.list.Key = func(i int) any { return p.results[i].ID }
		p.list.Label = func(i int) string { return p.results[i].Title }
		list := ui.List(c, &p.list, len(p.results), func(i int) { p.row(c, i) }).Grow(1).Padding(6).Children(func() {
			if len(p.results) > 0 {
				return
			}
			msg := "没有匹配的笔记"
			if p.query == "" {
				msg = "还没有笔记"
			}
			ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).Padding(16)
		})
		if list.Submitted() {
			p.openAt(p.chosen)
		}
	})
}

// row shows one result: its derived title and, when there is one, the body's
// excerpt — the same pair the tray panel lists.
func (p *SearchPanel) row(c *ui.Context, i int) {
	n := p.results[i]
	t := c.Theme()
	title := n.Title
	if title == "" {
		title = "无标题笔记"
	}
	bg, text, muted := ui.Transparent, t.Text, t.TextMuted
	if i == p.chosen {
		bg, text, muted = t.Accent.Alpha(0.85), t.AccentText, t.AccentText.Alpha(0.75)
	}
	excerpt := noteExcerpt(n.Content, 60)
	ui.Column(c).Gap(2).Padding(8, 8).Radius(6).Background(bg).
		Label("打开笔记 " + title).Tooltip(title).
		OnClick(func() { p.openAt(i) }).
		Children(func() {
			ui.Text(c, clampRunes(title, 24)).FontSize(13).TextColor(text).SingleLine()
			if excerpt != "" {
				ui.Text(c, clampRunes(excerpt, 30)).FontSize(11).TextColor(muted).SingleLine()
			}
		})
}

// choose moves the highlight into the results and keeps it in view.
func (p *SearchPanel) choose(i int) {
	if len(p.results) == 0 {
		p.chosen = -1
		return
	}
	p.chosen = max(0, min(i, len(p.results)-1))
	p.list.ScrollIntoView(p.chosen)
}

// reload re-runs the query, keeping the highlight on the same row where the
// new results still reach it.
func (p *SearchPanel) reload() {
	notes, err := p.app.notes.SearchNotes(p.query)
	if err != nil {
		log.Printf("search notes: %v", err)
		return
	}
	p.results = notes
	p.choose(p.chosen)
}

// present resets the panel to an empty query with the first result chosen —
// the state ⌘⇧F opens it in.
func (p *SearchPanel) present() {
	p.query = ""
	p.chosen = 0
	p.reload()
	p.input.Focus()
}

// openAt focuses a result's pin window and collapses the panel, as a row
// click in the tray panel does.
func (p *SearchPanel) openAt(i int) {
	if i < 0 || i >= len(p.results) {
		return
	}
	id := p.results[i].ID
	p.Hide()
	if _, err := p.app.windows.OpenPinnedWindow(id); err != nil {
		log.Printf("focus note from search: %v", err)
	}
}

func (p *SearchPanel) Hide() { p.win.Hide() }
