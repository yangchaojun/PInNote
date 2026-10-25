package main

// The pin window: a frameless, always-on-top desktop window showing one note
// as a whole-window WYSIWYG rich-text editor (the port of PinWindow.tsx +
// PinEditor.tsx). Markdown stays the fact source: opening runs note.content
// through the lossless loader, every save passes the round-trip guard, and
// closing is only "collapse" — a blank note is hard-deleted instead.

import (
	_ "embed"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

//go:embed assets/pushpin.svg
var pushpinSVG []byte

// pushpinIcon is the header's pin glyph.
var pushpinIcon = func() *ui.SVG {
	icon, err := ui.ParseSVG(pushpinSVG)
	if err != nil {
		panic("parse pushpin.svg: " + err.Error())
	}
	return icon
}()

// autosaveDelay is the debounce between the last edit and the write.
const autosaveDelay = 400 * time.Millisecond

// noteState is what a pin window shows for its note.
type noteState int

const (
	stateLive    noteState = iota // editing
	stateTrashed                  // the note is in the trash (deleted elsewhere)
)

// PinWindow is one note's window and editor. All fields are main-thread.
type PinWindow struct {
	app    *pinApp
	noteID string
	win    *mygo.Window

	doc    *richtext.Document
	dirty  bool
	editor ui.Handle // the editor element, focused when the window opens

	saveTimer *time.Timer

	state         noteState
	title         string
	closing       bool // a close/delete flow is already running
	helpOpen      bool
	linkOpen      bool
	linkURL       string
	confirmDelete bool // the header delete button's confirmation dialog
}

// newPinWindow creates the window and shows it ready to type.
func newPinWindow(app *pinApp, noteID string) (*PinWindow, error) {
	note, err := app.notes.GetNote(noteID)
	if err != nil {
		return nil, err
	}
	text, _ := LoadMarkdownLossless(note.Content)
	p := &PinWindow{
		app:    app,
		noteID: noteID,
		doc:    richtext.NewDocument(text),
		title:  note.Title,
	}
	if note.DeletedAt != nil {
		p.state = stateTrashed
	}

	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:       "PinNote",
		Width:       380,
		Height:      300,
		MinHeight:   150,
		Frameless:   true,
		AlwaysOnTop: true,
		Hidden:      true,
		Content:     ui.View(p.view),
	})
	// 打开即写: show once ready, with the caret already in the editor.
	p.win.OnReadyToShow(func() {
		p.win.Show()
		p.editor.Focus()
	})
	p.win.SetVisibleOnAllWorkspaces(true)
	p.win.OnBlur(func() { p.win.Update(p.flush) })
	p.win.OnClose(func(e *mygo.CloseEvent) {
		// Every close path funnels through collapse(): flush, apply the
		// empty-note rule, then really close. PreventDefault keeps the
		// system close from skipping the flush; an app-quit close sets
		// quitting first and passes through.
		if p.closing || p.app.quitting {
			return
		}
		e.PreventDefault()
		p.collapse()
	})
	return p, nil
}

// markDirty is the editor's Changed callback: arm the debounce timer.
func (p *PinWindow) markDirty() {
	p.dirty = true
	if p.saveTimer != nil {
		p.saveTimer.Stop()
	}
	p.saveTimer = time.AfterFunc(autosaveDelay, func() {
		p.win.Update(p.flush)
	})
}

// flush is the single save path: serialize → invariant guard → UpdateNote.
// A refused guard keeps the document (with its raw blocks) in the editor
// untouched; never persist a serialization we cannot reopen.
func (p *PinWindow) flush() {
	if p.saveTimer != nil {
		p.saveTimer.Stop()
		p.saveTimer = nil
	}
	if !p.dirty || p.state != stateLive {
		return
	}
	md, ok := GuardSaveRoundTrip(p.doc.Text())
	if !ok {
		log.Printf("pin window %s: save refused by round-trip guard", p.noteID)
		return
	}
	if _, err := p.app.notes.UpdateNote(p.noteID, md); err != nil {
		log.Printf("pin window %s: save failed: %v", p.noteID, err)
		return
	}
	p.dirty = false
}

// collapse is the close button's flow: flush first, then apply the
// empty-note rule, then close. When the flush was refused the note is kept —
// the editor may hold content that never reached the database, and
// DiscardIfEmpty would judge on the stale blank record.
func (p *PinWindow) collapse() {
	if p.closing {
		return
	}
	p.closing = true
	p.flush()
	if !p.dirty {
		if _, err := p.app.notes.DiscardIfEmpty(p.noteID); err != nil {
			log.Printf("discard empty note: %v", err)
		}
	}
	p.close()
}

// deleteNote is ⌘⌫: flush first so in-flight edits are not lost, then trash
// (60-day recycle bin), then close.
func (p *PinWindow) deleteNote() {
	if p.closing {
		return
	}
	p.closing = true
	p.flush()
	if _, err := p.app.notes.TrashNote(p.noteID); err != nil {
		log.Printf("trash note: %v", err)
	}
	p.close()
}

// close destroys the window and drops it from the registry.
func (p *PinWindow) close() {
	if p.saveTimer != nil {
		p.saveTimer.Stop()
		p.saveTimer = nil
	}
	p.app.windows.forget(p.noteID)
	p.win.Destroy()
}

// syncWithCaches reacts to changes made elsewhere: a note trashed in another
// window shows the placeholder; a note purged closes this window.
func (p *PinWindow) syncWithCaches() {
	if p.closing {
		return
	}
	if note, ok := p.app.liveNote(p.noteID); ok {
		p.state = stateLive
		if note.Title != "" {
			p.title = note.Title
		}
		return
	}
	if p.app.trashSet[p.noteID] {
		p.state = stateTrashed
		return
	}
	// Neither live nor trashed: gone for good.
	p.closing = true
	p.close()
}

// pinBackground mirrors the old window backdrop per theme.
func (p *PinWindow) pinBackground(t *ui.Theme) ui.Color {
	if p.app.windows.Theme() == "light" {
		return ui.Hex("#fafafb")
	}
	return ui.Hex("#1e1f22")
}

// caretParagraphIndex returns the sequential index of the paragraph holding
// the caret. Paragraphs yields each paragraph's start offset.
func caretParagraphIndex(t richtext.Text, caret int) int {
	idx, n := 0, 0
	for start := range t.Paragraphs() {
		if caret >= start {
			idx = n
		}
		n++
	}
	return idx
}

func (p *PinWindow) view(c *ui.Context) {
	t := c.Theme()

	// Window-level shortcuts (spec §4.2), registered with OnShortcut so they
	// claim the keys ahead of the focused editor (⌘⌫ deletes the note, not
	// the line). ⌘B / ⌘I / ⌘K are the editor's own; the theme toggle lives
	// on the app menu so it fires once; Esc closes a modal on its own.
	c.OnShortcut(ui.Cmd, ui.KeyBackspace, func() {
		// Window shortcuts fire over open modals; while the delete dialog
		// asks for confirmation, ⌘⌫ must not answer it with an instant
		// delete (issue #6).
		if p.confirmDelete {
			return
		}
		p.deleteNote()
	})
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeySlash, func() { p.helpOpen = !p.helpOpen })

	ui.Column(c).Fill().Background(p.pinBackground(t)).Children(func() {
		p.header(c, t)
		switch p.state {
		case stateTrashed:
			p.trashedBody(c, t)
		default:
			p.editorBody(c, t)
		}
		p.linkDialog(c, t)
		p.helpDialog(c, t)
		p.confirmDeleteDialog(c, t)
	})
}

// header is the drag handle strip: pin glyph, title tooltip, delete and
// collapse buttons.
func (p *PinWindow) header(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(28).Padding(0, 10, 0, 6).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, pushpinIcon).FontSize(13).TextColor(t.TextMuted).Label("PinNote")
		ui.Box(c).Grow(1).Height(20).DragWindow().Tooltip(p.titleTooltip())
		if p.state == stateLive {
			if ui.Button(c, "删除").Label("删除笔记").FontSize(11).Height(22).TextColor(t.Danger).Clicked() {
				p.confirmDelete = true
			}
		}
		if ui.Button(c, "×").Label("收起笔记").FontSize(13).MinWidth(24).Height(22).Clicked() {
			p.collapse()
		}
	})
}

func (p *PinWindow) titleTooltip() string {
	if p.title != "" {
		return p.title
	}
	return "无标题笔记"
}

// editorBody is the whole-window editor plus the raw-source action row.
func (p *PinWindow) editorBody(c *ui.Context, t *ui.Theme) {
	rawIdx := -1
	text := p.doc.Text()
	if idx := caretParagraphIndex(text, func() int { _, caret := p.doc.Selection(); return caret }()); idx >= 0 {
		if paras := paragraphsOf(text); idx < len(paras) && paragraphIsRaw(paras[idx]) {
			rawIdx = idx
		}
	}

	ui.Column(c).Grow(1).Padding(2, 8, 4, 8).Children(func() {
		editor := richtext.EditorBase(c, p.doc, richtext.EditorOptions{
			Label:         "笔记",
			Placeholder:   "记点什么…",
			OnLinkCommand: func() { p.linkURL = p.doc.Formatting().Link; p.linkOpen = true },
		}).Grow(1).Bind(&p.editor)
		if editor.Changed() {
			p.markDirty()
		}

		if rawIdx >= 0 {
			ui.Row(c).Padding(6, 6, 2, 6).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "原始 Markdown 块（未渲染，保存时原样保留）").FontSize(11).TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "转为可编辑").KeepFocus().FontSize(11).Clicked() {
					// A refused conversion keeps the raw block: data survives.
					if next, ok := ConvertRawSource(p.doc.Text(), rawIdx); ok {
						p.doc.SetText(next)
					}
				}
			})
		}
	})
}

// trashedBody is the placeholder after the note was deleted elsewhere.
func (p *PinWindow) trashedBody(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
		ui.Icon(c, pushpinIcon).FontSize(22).TextColor(t.TextMuted)
		ui.Text(c, "该笔记已被删除。").TextColor(t.TextMuted)
		if ui.Button(c, "关闭").Clicked() {
			p.collapse()
		}
	})
}

// linkDialog is ⌘K's URL prompt (the old window.prompt).
func (p *PinWindow) linkDialog(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &p.linkOpen, func() {
		ui.Column(c).Width(300).Gap(10).Children(func() {
			ui.Text(c, "链接地址").FontSize(13).Bold()
			input := ui.TextInput(c, &p.linkURL).Placeholder("https://").AutoFocus().Label("URL")
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if p.doc.Formatting().Link != "" {
					if ui.Button(c, "移除链接").Clicked() {
						p.doc.SetLink("")
						p.linkOpen = false
					}
				}
				if ui.PrimaryButton(c, "确定").Clicked() || input.Submitted() {
					p.doc.SetLink(normalizeLinkURL(p.linkURL))
					p.linkOpen = false
				}
			})
		})
	})
}

// confirmDeleteDialog is the header delete button's second confirmation
// (issue #6): the trash keeps the note 60 days, but it has no UI to dig it
// back out, so one click must not be enough. Esc or clicking outside
// cancels.
func (p *PinWindow) confirmDeleteDialog(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &p.confirmDelete, func() {
		ui.Column(c).Width(300).Gap(10).Children(func() {
			ui.Text(c, "删除这条笔记？").FontSize(13).Bold()
			ui.Text(c, "删除后进入回收站保留 60 天，到期自动清除；回收站没有恢复入口。").FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					p.confirmDelete = false
				}
				if ui.Button(c, "删除").TextColor(t.Danger).Clicked() {
					p.confirmDelete = false
					p.deleteNote()
				}
			})
		})
	})
}

func normalizeLinkURL(url string) string {
	if url == "" {
		return ""
	}
	// The old prompt defaulted to https://; keep typed bare domains working.
	if !strings.Contains(url, "://") && !strings.HasPrefix(url, "mailto:") {
		return "https://" + url
	}
	return url
}

// helpDialog is the ⌘⇧/ shortcuts help.
func (p *PinWindow) helpDialog(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &p.helpOpen, func() {
		ui.Column(c).Width(420).Gap(4).Children(func() {
			ui.Text(c, "键盘快捷键").FontSize(15).Bold()
			ui.Divider(c)
			for _, row := range [][2]string{
				{"⌘⌥N", "新建笔记并打开 pin 窗口（全局，任何应用下可用）"},
				{"⌘B / ⌘I / ⌘K", "粗体 / 斜体 / 链接"},
				{"⌘⇧D", "切换亮/暗主题（所有窗口同时生效）"},
				{"⌘⌫", "删除当前笔记（进回收站）并关窗"},
				{"⌘⇧/", "显示本帮助"},
				{"Esc", "关闭本帮助"},
			} {
				ui.Row(c).Padding(6, 0).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, row[0]).FontSize(12).Background(t.Surface).Radius(4).Padding(4, 8)
					ui.Text(c, row[1]).FontSize(13).Grow(1)
				})
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "关闭").Clicked() {
					p.helpOpen = false
				}
			})
		})
	})
}
