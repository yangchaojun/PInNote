package main

// Lossless loader + save guard for the "Markdown fact-source, WYSIWYG view"
// model, ported from the TipTap-era frontend audit (frontend/src/lib/audit.ts,
// ADR-0002): every top-level block either parses into the rich-text document
// (and survives parse→serialize→parse unchanged) or becomes verbatim
// raw-source paragraphs that the save path writes back byte-for-byte.
// Nothing is allowed to vanish silently.
//
// Measured richtext behaviors this design works around (see the corpus test):
//   - thematic breaks parse to nothing editable (a blank paragraph) → raw;
//   - fenced code keeps its lines as CodeBlock paragraphs but drops the
//     language, so the loader stashes it in a Tag and the save path re-emits
//     the fence with it;
//   - a link definition is consumed by the parser without leaving a block,
//     so the splitter walks source gaps too;
//   - Text.Equal is pointer-unequal on attachments, so the guard compares
//     documents with its own attachment-aware equivalence.

import (
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/egoist/mygo/ui/richtext"
)

const (
	// rawSourceTag marks the runs of a demoted raw-source line: the editor
	// shows it as Markdown source instead of a rendering, and the save path
	// writes the block back verbatim. A raw block is consecutive raw-tagged
	// paragraphs, one per line, joined with "\n" on save.
	rawSourceTag = "pinnote-raw-source"

	// codeLangTagPrefix marks the runs of a fenced code block with its
	// language, which ParagraphStyle cannot hold.
	codeLangTagPrefix = "pinnote-code-lang:"
)

// safeBlockKinds are the block kinds the rich-text model represents
// faithfully. HTML blocks, tables (flattened to tab-separated lines),
// footnote definitions, display math and thematic breaks do not survive a
// round trip: they demote to raw source instead of silently degrading.
var safeBlockKinds = map[parser.Kind]bool{
	parser.Paragraph: true,
	parser.Heading:   true,
	parser.CodeBlock: true,
	parser.Quote:     true,
	parser.List:      true,
	parser.Item:      true,
}

// safeInlineKinds are the inline kinds the rich-text model represents.
// Images are native attachments; inline math stays as written. Raw inline
// HTML and footnote references do not survive, so they demote.
var safeInlineKinds = map[parser.InlineKind]bool{
	parser.Text:          true,
	parser.SoftBreak:     true,
	parser.LineBreak:     true,
	parser.Code:          true,
	parser.Emphasis:      true,
	parser.Strong:        true,
	parser.Strikethrough: true,
	parser.Link:          true,
	parser.Image:         true,
	parser.Math:          true,
}

// sourceBlock is one top-level slice of the source: a parsed block, or a gap
// the parser consumed without leaving a block (link definitions and the
// like), which is raw by definition.
type sourceBlock struct {
	raw      string
	covered  bool // the parser produced a block for these bytes
	language string
}

// splitBlocks walks the source's top-level blocks and the gaps between them.
// A gap the parser consumed without leaving a block is raw by definition —
// except when it continues the previous block's construct with no blank line
// in between (a setext heading's underline, a table row's trailing pipe),
// where the bytes belong to the previous block and are merged into it so
// nothing is dropped.
func splitBlocks(md string) []sourceBlock {
	d := parser.Parse(md)
	type entry struct {
		start, end int
		covered    bool
		language   string
	}
	var entries []entry
	pos := 0
	addEntry := func(start, end int, covered bool, language string) {
		if strings.TrimSpace(md[start:end]) == "" {
			return
		}
		entries = append(entries, entry{start: start, end: end, covered: covered, language: language})
	}
	for i := range d.Children(0) {
		b := d.Block(i)
		if b.Start > pos {
			addEntry(pos, b.Start, false, "")
		}
		addEntry(b.Start, b.End, true, b.Language())
		pos = b.End
	}
	if pos < len(md) {
		addEntry(pos, len(md), false, "")
	}

	var out []sourceBlock
	for _, e := range entries {
		raw := md[e.start:e.end]
		if e.covered {
			raw = strings.TrimSuffix(raw, "\n")
		} else if len(out) > 0 && !strings.HasPrefix(raw, "\n\n") {
			// Continuation of the previous construct, not its own block.
			prev := &out[len(out)-1]
			merged := prev.raw + raw
			prev.raw = merged
			continue
		} else {
			raw = strings.TrimSpace(raw)
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		out = append(out, sourceBlock{raw: raw, covered: e.covered, language: e.language})
	}
	return out
}

// coveredKinds reports whether every block and inline kind inside block i is
// one the rich-text model represents (gate 1: the whitelist catches silent
// stripping the fixed point cannot see, as in raw HTML losing its tags).
func coveredKinds(d *parser.Document, i int) bool {
	b := d.Block(i)
	if !safeBlockKinds[b.Kind] {
		return false
	}
	for k := range d.Children(i) {
		if !coveredKinds(d, k) {
			return false
		}
	}
	if b.Kind == parser.Paragraph || b.Kind == parser.Heading {
		for _, in := range d.Inlines(i) {
			if !safeInlineKinds[in.Kind] {
				return false
			}
		}
	}
	return true
}

// auditBlock reports whether a covered source block is safe to parse: it
// parses to something (gate 2), and serialize∘parse∘serialize is a fixed
// point that does not drift the document (gate 3, auto-tracking serializer
// upgrades). Gaps are unsafe by construction.
func auditBlock(b sourceBlock) bool {
	if !b.covered {
		return false
	}
	d := parser.Parse(b.raw)
	children := rootChildren(d)
	if len(children) == 0 {
		return false
	}
	for _, i := range children {
		if !coveredKinds(d, i) {
			return false
		}
	}
	t1 := richtext.ParseMarkdown(b.raw)
	if t1.ParagraphCount() == 0 {
		return false
	}
	s1 := t1.Markdown()
	t2 := richtext.ParseMarkdown(s1)
	if s1 != t2.Markdown() {
		return false
	}
	return textEquivalent(t1, t2)
}

func rootChildren(d *parser.Document) []int {
	var out []int
	for i := range d.Children(0) {
		out = append(out, i)
	}
	return out
}

// rawParagraphs makes the demoted form of a block: one tagged code-block
// paragraph per source line (blank lines included, as tagged runs), so the
// text model never has to embed a newline in a run.
func rawParagraphs(source string) []richtext.Paragraph {
	lines := strings.Split(source, "\n")
	paras := make([]richtext.Paragraph, len(lines))
	for i, line := range lines {
		paras[i] = richtext.Paragraph{
			Style: richtext.ParagraphStyle{CodeBlock: true},
			Runs:  []richtext.Run{{Text: line, Attributes: richtext.Attributes{Tag: rawSourceTag}}},
		}
	}
	return paras
}

// paragraphTag returns the tag of a paragraph's first tagged run, "" when
// none of its runs carry one.
func paragraphTag(p richtext.Paragraph) string {
	for _, r := range p.Runs {
		if r.Attributes.Tag != "" {
			return r.Attributes.Tag
		}
	}
	return ""
}

// paragraphIsRaw reports whether a paragraph is a demoted raw-source line.
func paragraphIsRaw(p richtext.Paragraph) bool {
	return paragraphTag(p) == rawSourceTag
}

// paragraphSource returns the (possibly edited) text of a paragraph.
func paragraphSource(p richtext.Paragraph) string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// tagParagraphs sets tag on every run of the paragraphs.
func tagParagraphs(paras []richtext.Paragraph, tag string) {
	for i := range paras {
		runs := make([]richtext.Run, len(paras[i].Runs))
		for j, r := range paras[i].Runs {
			runs[j] = richtext.Run{Text: r.Text, Attributes: r.Attributes}
			runs[j].Attributes.Tag = tag
		}
		paras[i].Runs = runs
	}
}

// LoadMarkdownLossless parses note content for the editor: every safe block
// becomes real editable rich text (code fences keep their language through a
// tag), every unsafe block becomes verbatim raw-source paragraphs. An empty
// source gives one empty paragraph — the window's normal empty-editing state.
func LoadMarkdownLossless(md string) (richtext.Text, []string) {
	paras := []richtext.Paragraph{}
	demoted := []string{}
	for _, block := range splitBlocks(md) {
		if !auditBlock(block) {
			demoted = append(demoted, block.raw)
			paras = append(paras, rawParagraphs(block.raw)...)
			continue
		}
		parsed := []richtext.Paragraph{}
		for _, p := range richtext.ParseMarkdown(block.raw).Paragraphs() {
			parsed = append(parsed, p)
		}
		if block.language != "" {
			tagParagraphs(parsed, codeLangTagPrefix+block.language)
		}
		paras = append(paras, parsed...)
	}
	if len(paras) == 0 {
		paras = []richtext.Paragraph{{}}
	}
	return richtext.TextOf(paras...), demoted
}

// serializeCodeGroup writes consecutive CodeBlock paragraphs as one fence,
// restoring the language stashed in a tag by the loader.
func serializeCodeGroup(group []richtext.Paragraph) string {
	lang := ""
	if tag := paragraphTag(group[0]); strings.HasPrefix(tag, codeLangTagPrefix) {
		lang = strings.TrimPrefix(tag, codeLangTagPrefix)
	}
	lines := make([]string, len(group))
	for i, p := range group {
		lines[i] = paragraphSource(p)
	}
	var b strings.Builder
	b.WriteString("```")
	b.WriteString(lang)
	b.WriteString("\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n```")
	return b.String()
}

// SerializeNoteText is the save path of the fact source. Raw-source blocks
// are written back verbatim (their lines joined with "\n"); fenced code
// groups keep their language; everything else goes through the rich-text
// Markdown serializer as a group, so lists and quotes keep their structure.
func SerializeNoteText(t richtext.Text) string {
	var b strings.Builder
	writeBlock := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}

	paras := paragraphsOf(t)
	for i := 0; i < len(paras); {
		p := paras[i]
		switch {
		case paragraphIsRaw(p):
			j := i
			var lines []string
			for j < len(paras) && paragraphIsRaw(paras[j]) {
				lines = append(lines, paragraphSource(paras[j]))
				j++
			}
			writeBlock(strings.Join(lines, "\n"))
			i = j
		case p.Style.CodeBlock:
			j := i
			for j < len(paras) && paras[j].Style.CodeBlock && !paragraphIsRaw(paras[j]) {
				j++
			}
			writeBlock(serializeCodeGroup(paras[i:j]))
			i = j
		default:
			j := i
			for j < len(paras) && !paras[j].Style.CodeBlock && !paragraphIsRaw(paras[j]) {
				j++
			}
			writeBlock(richtext.TextOf(paras[i:j]...).Markdown())
			i = j
		}
	}
	return b.String()
}

func paragraphsOf(t richtext.Text) []richtext.Paragraph {
	var out []richtext.Paragraph
	for _, p := range t.Paragraphs() {
		out = append(out, p)
	}
	return out
}

// isBlankParagraph reports a paragraph that carries no content: markdown
// cannot represent blank lines as content, so the save invariant compares
// documents after dropping them (guarding data, not blank-line formatting).
// Code-block lines and raw-source lines are content even when empty.
func isBlankParagraph(p richtext.Paragraph) bool {
	if p.Style.CodeBlock || paragraphIsRaw(p) {
		return false
	}
	return strings.TrimSpace(paragraphSource(p)) == ""
}

// normalizeForGuard strips blank paragraphs, keeping at least one paragraph
// so the empty document compares equal to itself.
func normalizeForGuard(t richtext.Text) richtext.Text {
	var kept []richtext.Paragraph
	for _, p := range paragraphsOf(t) {
		if !isBlankParagraph(p) {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return richtext.TextOf(richtext.Paragraph{})
	}
	return richtext.TextOf(kept...)
}

// attachmentEquivalent compares the parts of an attachment that round-trip
// through Markdown (its URL and description); the bitmap and drawing are
// presentation, recreated on load.
func attachmentEquivalent(a, b *richtext.Attachment) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	return a.Name == b.Name && a.URL == b.URL && a.Width == b.Width && a.Height == b.Height
}

// attributesEquivalent compares run attributes, attachments by value.
func attributesEquivalent(a, b richtext.Attributes) bool {
	if a.Font != b.Font || a.Size != b.Size || a.Weight != b.Weight ||
		a.Italic != b.Italic || a.Underline != b.Underline ||
		a.Strikethrough != b.Strikethrough || a.Color != b.Color ||
		a.Background != b.Background || a.Code != b.Code ||
		a.Link != b.Link || a.Tag != b.Tag {
		return false
	}
	return attachmentEquivalent(a.Attachment, b.Attachment)
}

// textEquivalent is attachment-aware document equality: Text.Equal compares
// attachment pointers, so a parsed-twice document never equals itself.
func textEquivalent(a, b richtext.Text) bool {
	pa, pb := paragraphsOf(a), paragraphsOf(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i].Style != pb[i].Style || len(pa[i].Runs) != len(pb[i].Runs) {
			return false
		}
		for j := range pa[i].Runs {
			if pa[i].Runs[j].Text != pb[i].Runs[j].Text ||
				!attributesEquivalent(pa[i].Runs[j].Attributes, pb[i].Runs[j].Attributes) {
				return false
			}
		}
	}
	return true
}

// GuardSaveRoundTrip is the pre-write invariant (ADR-0002 "防丢保险"): the
// live document must survive serialize → reopen (through the same lossless
// loader, so raw blocks must re-demote identically) unchanged; otherwise the
// write is refused rather than allowed to corrupt the fact source.
func GuardSaveRoundTrip(t richtext.Text) (string, bool) {
	md := SerializeNoteText(t)
	back, _ := LoadMarkdownLossless(md)
	if !textEquivalent(normalizeForGuard(t), normalizeForGuard(back)) {
		return md, false
	}
	return md, true
}

// ConvertRawSource re-audits the source of a raw block and, when it has
// become safe (e.g. the user deleted the offending syntax), splices the
// parsed paragraphs back in place of the raw lines. Unsafe sources stay
// demoted: data survives either way. Returns the new text and whether the
// conversion happened.
func ConvertRawSource(t richtext.Text, paragraphIndex int) (richtext.Text, bool) {
	paras := paragraphsOf(t)
	if paragraphIndex < 0 || paragraphIndex >= len(paras) || !paragraphIsRaw(paras[paragraphIndex]) {
		return t, false
	}
	start := paragraphIndex
	for start > 0 && paragraphIsRaw(paras[start-1]) {
		start--
	}
	end := paragraphIndex
	for end+1 < len(paras) && paragraphIsRaw(paras[end+1]) {
		end++
	}
	var lines []string
	for _, p := range paras[start : end+1] {
		lines = append(lines, paragraphSource(p))
	}
	src := strings.Join(lines, "\n")

	var language string
	d := parser.Parse(src)
	for _, i := range rootChildren(d) {
		language = d.Block(i).Language()
	}
	blk := sourceBlock{raw: src, covered: true, language: language}
	if !auditBlock(blk) {
		return t, false
	}
	replacement := []richtext.Paragraph{}
	for _, p := range richtext.ParseMarkdown(src).Paragraphs() {
		replacement = append(replacement, p)
	}
	if language != "" {
		tagParagraphs(replacement, codeLangTagPrefix+language)
	}
	next := make([]richtext.Paragraph, 0, len(paras)-(end-start+1)+len(replacement))
	next = append(next, paras[:start]...)
	next = append(next, replacement...)
	next = append(next, paras[end+1:]...)
	return richtext.TextOf(next...), true
}
