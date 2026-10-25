package main

// The regression net ADR-0002 demands, ported from the TipTap-era corpus
// (frontend/src/lib/fixtures.ts): every construct the editor renders today
// must round-trip as the same document, and every construct it cannot render
// must survive a save verbatim. Expectations are calibrated to the mygo
// richtext contract: tables and footnotes demote (the editor cannot represent
// them), inline images are now native (they used to demote under TipTap).

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/richtext"
)

type fixture struct {
	name string
	md   string
}

var safeFixtures = []fixture{
	{name: "heading h1", md: "# Title"},
	{name: "heading h6", md: "###### Small"},
	{name: "paragraph", md: "Just a paragraph."},
	{name: "bold", md: "Some **bold** text"},
	{name: "italic", md: "Some *italic* text"},
	{name: "strikethrough", md: "Some ~~gone~~ text"},
	{name: "inline code", md: "Call `run()` now"},
	{name: "link", md: "See [docs](https://example.com) here"},
	{name: "link with title", md: `See [docs](https://example.com "The Docs") here`},
	{name: "autolink", md: "Visit <https://example.com> now"},
	{name: "nested emphasis", md: "**bold with *italic* inside**"},
	{name: "combined marks", md: "***both*** and ~~**bold strike**~~"},
	{name: "word underscore", md: "snake_case_names a_b_c stay literal"},
	{name: "ampersand angle", md: "Tom & Jerry <3 angle > chars"},
	{name: "bullet list", md: "- item a\n- item b"},
	{name: "nested bullets", md: "- top\n  - inner\n    - deep"},
	{name: "ordered list", md: "1. first\n2. second\n3. third"},
	{name: "ordered with bold items", md: "1. **one**\n2. *two*"},
	{name: "task list", md: "- [ ] todo\n- [x] done"},
	{name: "nested tasks", md: "- [ ] parent\n  - [x] child"},
	{name: "mixed task/bullet list", md: "- plain item\n- [x] task item\n- another plain"},
	{name: "task with marks", md: "- [ ] **bold** task `code`"},
	{name: "fenced code", md: "```js\nconst a = 1;\n```"},
	{name: "fenced no lang", md: "```\nplain\n```"},
	{name: "fenced with bold inside", md: "```md\n**not bold**\n```"},
	{name: "blockquote", md: "> quoted line\n> second line"},
	{name: "blockquote with list", md: "> - a\n> - b"},
	{name: "nested blockquote", md: "> outer\n>\n> > inner"},
	{name: "inline image (native attachment)", md: "see ![alt text](https://example.com/a.png) here"},
	{name: "backtick-adjacent codespan", md: "`` special `code` ``"},
	{name: "multiblock doc", md: "# T\n\nintro\n\n- [ ] a\n- [x] b\n\n```\ncode\n```\n\n> quote"},
	{name: "hard break (two trailing spaces)", md: "line1  \nline2"},
	{name: "cjk content", md: "# 中文标题\n\n**加粗** 与 *斜体* 混排\n\n- [ ] 任务项\n- [x] 已完成"},
	{name: "empty", md: ""},
}

var lossyFixtures = []fixture{
	{name: "html comment", md: "<!-- todo: keep me -->"},
	{name: "html block", md: "<details>\n<summary>more</summary>\nhidden body\n</details>"},
	{name: "link definition", md: `[r]: https://example.com "ref"`},
	{name: "html entity raw", md: "&#9829; <span>sp</span>"},
	{name: "table", md: "| a | b |\n|---|---|\n| 1 | 2 |"},
	{name: "table alignment", md: "| l | c | r |\n|:--|:-:|--:|\n| 1 | 2 | 3 |"},
	{name: "table with marks", md: "| a | b |\n|---|---|\n| **1** | `2` |"},
	{name: "footnote definition", md: "[^1]: the note"},
	// A thematic break parses to a blank paragraph that the serializer
	// drops: measured, it must demote so the "---" survives verbatim.
	{name: "hr", md: "text\n\n---\n\nmore"},
}

var normalizedFixtures = []fixture{
	{name: "setext to atx", md: "Title\n====="},
	{name: "upper x checkbox", md: "- [X] done"},
	{name: "asterisk bullets to dash", md: "* a\n* b"},
	{name: "ordered start renumbered", md: "5. fifth\n6. sixth"},
	{name: "indented code to fenced", md: "    indented code\n    line two"},
	{name: "unclosed fence gains closer", md: "```js\nconst a = 1;\nno closing fence\n"},
	{name: "footnote ref escapes to literal", md: "text[^1]"},
}

// openNote is what a pin window does on open.
func openNote(t *testing.T, md string) (richtext.Text, []string) {
	t.Helper()
	doc, demoted := LoadMarkdownLossless(md)
	return doc, demoted
}

// saveAndReopen is one simulated save through the guard.
func saveAndReopen(t *testing.T, doc richtext.Text) (string, richtext.Text) {
	t.Helper()
	md, ok := GuardSaveRoundTrip(doc)
	if !ok {
		t.Fatalf("save guard refused the write\nserialized:\n%s", md)
	}
	reopened, _ := LoadMarkdownLossless(md)
	return md, reopened
}

func TestSafeFixturesRoundTrip(t *testing.T) {
	for _, f := range safeFixtures {
		t.Run(f.name, func(t *testing.T) {
			doc, demoted := openNote(t, f.md)
			if len(demoted) != 0 {
				t.Fatalf("safe syntax demoted to raw source: %v", demoted)
			}
			md, reopened := saveAndReopen(t, doc)
			if !textEquivalent(normalizeForGuard(doc), normalizeForGuard(reopened)) {
				t.Fatalf("reopen drifted the document\nfirst save:\n%s", md)
			}
			md2, reopened2 := saveAndReopen(t, reopened)
			if md2 != md {
				t.Fatalf("second save rewrote the source\nfirst:\n%q\nsecond:\n%q", md, md2)
			}
			_ = reopened2
		})
	}
}

func TestLossyFixturesSurviveVerbatim(t *testing.T) {
	for _, f := range lossyFixtures {
		t.Run(f.name, func(t *testing.T) {
			doc, demoted := openNote(t, f.md)
			if len(demoted) == 0 {
				t.Fatal("unsafe syntax must be demoted, not parsed")
			}
			md, _ := saveAndReopen(t, doc)
			if !strings.Contains(md, strings.TrimSpace(f.md)) {
				t.Fatalf("raw block lost verbatim bytes\nsaved:\n%s", md)
			}
			again, demotedAgain := openNote(t, md)
			if len(demotedAgain) == 0 {
				t.Fatal("reopening the saved file must demote identically")
			}
			if _, ok := GuardSaveRoundTrip(again); !ok {
				t.Fatal("the reopened raw block must still pass the save guard")
			}
		})
	}
}

func TestNormalizedFixturesCanonicalize(t *testing.T) {
	for _, f := range normalizedFixtures {
		t.Run(f.name, func(t *testing.T) {
			doc, demoted := openNote(t, f.md)
			if len(demoted) != 0 {
				t.Fatalf("demoted: %v", demoted)
			}
			md, reopened := saveAndReopen(t, doc)
			if strings.TrimSpace(md) == strings.TrimSpace(f.md) {
				t.Fatal("expected normalization, source unchanged")
			}
			if !textEquivalent(normalizeForGuard(doc), normalizeForGuard(reopened)) {
				t.Fatalf("normalization drifted the document\nsaved:\n%s", md)
			}
		})
	}
}

func TestBlankParagraphsNeverWedgeSaving(t *testing.T) {
	// Pressing Enter twice mid-note: markdown cannot represent the blank
	// line as content, so the guard must tolerate its collapse.
	doc := richtext.TextOf(
		richtext.Paragraph{Runs: []richtext.Run{{Text: "a"}}},
		richtext.Paragraph{},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "b"}}},
	)
	if _, ok := GuardSaveRoundTrip(doc); !ok {
		t.Fatal("a lone blank paragraph between blocks must not block a save")
	}

	// The trailing scaffolding paragraph the editor keeps while typing.
	trailing := richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "T"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "body"}}},
		richtext.Paragraph{},
	)
	if _, ok := GuardSaveRoundTrip(trailing); !ok {
		t.Fatal("a trailing empty paragraph must not block a save")
	}
}

func TestCheckboxWriteBack(t *testing.T) {
	// Clicking a task checkbox in the editor flips ParagraphStyle.Checked;
	// the serializer must write it back as "- [x]".
	doc, demoted := openNote(t, "- [ ] buy milk\n- [x] already done")
	if len(demoted) != 0 {
		t.Fatalf("task list demoted: %v", demoted)
	}
	var paras []richtext.Paragraph
	for _, p := range doc.Paragraphs() {
		paras = append(paras, p)
	}
	toggled := paras[0]
	toggled.Style.Checked = true
	paras[0] = toggled
	md := SerializeNoteText(richtext.TextOf(paras...))
	if !strings.Contains(md, "- [x] buy milk") || !strings.Contains(md, "- [x] already done") {
		t.Fatalf("checkbox did not serialize back as - [x]:\n%s", md)
	}
	if _, ok := GuardSaveRoundTrip(richtext.TextOf(paras...)); !ok {
		t.Fatal("the toggled document must still pass the save guard")
	}
}

func TestLiteralMarkdownLookingText(t *testing.T) {
	// Typing "## typing test" into a body paragraph: the serializer must
	// escape the block intro so it reopens as the same text (the exact case
	// that once wedged every save in the TipTap editor).
	doc := richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "T"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "## typing test"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "- not a list"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "1. not a list either"}}},
	)
	md, ok := GuardSaveRoundTrip(doc)
	if !ok {
		t.Fatalf("literal markdown-looking text refused the save\nserialized:\n%s", md)
	}
	reopened, _ := LoadMarkdownLossless(md)
	if !textEquivalent(normalizeForGuard(doc), normalizeForGuard(reopened)) {
		t.Fatalf("escaped text drifted the document\nsaved:\n%s", md)
	}
}

func TestEmptyDocumentSerializesToEmpty(t *testing.T) {
	doc, demoted := openNote(t, "")
	if len(demoted) != 0 {
		t.Fatalf("empty source demoted: %v", demoted)
	}
	if md := SerializeNoteText(doc); strings.TrimSpace(md) != "" {
		t.Fatalf("empty document serialized to %q, want empty (the empty-note rule depends on it)", md)
	}
}

func TestConvertRawSource(t *testing.T) {
	// An HTML comment demotes; once the user deletes the comment markers the
	// block converts back to editable rich text. Still-unsafe sources stay.
	doc, demoted := openNote(t, "# head\n\n<!-- todo: keep me -->\n\ntail")
	if len(demoted) != 1 {
		t.Fatalf("want the comment demoted, got %v", demoted)
	}
	var idx = -1
	var paras []richtext.Paragraph
	for _, p := range doc.Paragraphs() {
		paras = append(paras, p)
		if paragraphIsRaw(p) && idx < 0 {
			idx = len(paras) - 1
		}
	}
	if idx < 0 {
		t.Fatal("no raw paragraph found")
	}
	// Simulate the user editing the source into a safe paragraph.
	paras[idx].Runs = []richtext.Run{{Text: "just text now", Attributes: richtext.Attributes{Tag: rawSourceTag}}}
	converted, ok := ConvertRawSource(richtext.TextOf(paras...), idx)
	if !ok {
		t.Fatal("safe source must convert")
	}
	for _, p := range converted.Paragraphs() {
		if paragraphIsRaw(p) {
			t.Fatal("safe source must convert to editable rich text")
		}
	}
	// An unsafe source stays raw.
	stillRaw, convertedUnsafe := ConvertRawSource(doc, idx)
	if convertedUnsafe {
		t.Fatal("unsafe source must not convert")
	}
	rawSeen := false
	for _, p := range stillRaw.Paragraphs() {
		if paragraphIsRaw(p) {
			rawSeen = true
		}
	}
	if !rawSeen {
		t.Fatal("unsafe source must stay demoted")
	}
}
