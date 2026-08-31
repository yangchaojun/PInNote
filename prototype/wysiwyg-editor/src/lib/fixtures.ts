/**
 * Corpus for the round-trip suite. The "safe" list mirrors the exact subset
 * PinNote renders today (frontend/src/components/MarkdownView.tsx via
 * react-markdown+remark-gfm) plus every construct the old toolbar
 * (frontend/src/lib/markdown.ts FormatFns) could produce. The "lossy" list
 * is the silent-drop / cross-renderer-divergence set research §1 flagged.
 */

export interface Fixture {
  name: string;
  md: string;
}

/** Must round-trip through parse→serialize→parse as the same doc. */
export const SAFE_FIXTURES: Fixture[] = [
  { name: "heading h1", md: "# Title" },
  { name: "heading h6", md: "###### Small" },
  { name: "setext h1", md: "Title\n=====" },
  { name: "setext h2", md: "Sub\n---" },
  { name: "paragraph", md: "Just a paragraph." },
  { name: "bold", md: "Some **bold** text" },
  { name: "italic", md: "Some *italic* text" },
  { name: "strikethrough", md: "Some ~~gone~~ text" },
  { name: "inline code", md: "Call `run()` now" },
  { name: "link", md: "See [docs](https://example.com) here" },
  { name: "link with title", md: 'See [docs](https://example.com "The Docs") here' },
  { name: "autolink", md: "Visit <https://example.com> now" },
  { name: "nested emphasis", md: "**bold with *italic* inside**" },
  { name: "combined marks", md: "***both*** and ~~**bold strike**~~" },
  { name: "word underscore (escape churn)", md: "snake_case_names a_b_c stay literal" },
  { name: "ampersand angle", md: "Tom & Jerry <3 angle > chars" },
  { name: "bullet list", md: "- item a\n- item b" },
  { name: "nested bullets", md: "- top\n  - inner\n    - deep" },
  { name: "ordered list", md: "1. first\n2. second\n3. third" },
  { name: "ordered list start", md: "5. fifth\n6. sixth" },
  { name: "ordered with bold items", md: "1. **one**\n2. *two*" },
  { name: "task list", md: "- [ ] todo\n- [x] done" },
  { name: "task list upper X", md: "- [X] done upper" },
  { name: "nested tasks", md: "- [ ] parent\n  - [x] child" },
  { name: "mixed task/bullet list", md: "- plain item\n- [x] task item\n- another plain" },
  { name: "task with marks", md: "- [ ] **bold** task `code`" },
  { name: "fenced code", md: "```js\nconst a = 1;\n```" },
  { name: "fenced no lang", md: "```\nplain\n```" },
  { name: "fenced with bold inside", md: "```md\n**not bold**\n```" },
  { name: "blockquote", md: "> quoted line\n> second line" },
  { name: "blockquote with list", md: "> - a\n> - b" },
  { name: "nested blockquote", md: "> outer\n>\n> > inner" },
  { name: "hr", md: "text\n\n---\n\nmore" },
  { name: "table", md: "| a | b |\n|---|---|\n| 1 | 2 |" },
  { name: "table alignment", md: "| l | c | r |\n|:--|:-:|--:|\n| 1 | 2 | 3 |" },
  { name: "table with marks", md: "| a | b |\n|---|---|\n| **1** | `2` |" },
  { name: "table with pipe in code", md: "| a | b |\n|---|---|\n| `x \\| y` | 2 |" },
  { name: "table task cell", md: "| done | item |\n|---|---|\n| - [x] | task |" },
  { name: "multiblock doc", md: "# T\n\nintro\n\n- [ ] a\n- [x] b\n\n```\ncode\n```\n\n> quote\n\n| h |\n|---|\n| v |" },
  { name: "cjk content", md: "# 中文标题\n\n**加粗** 与 *斜体* 混排\n\n- [ ] 任务项\n- [x] 已完成" },
  { name: "empty", md: "" },
];

/**
 * Must NOT be silently dropped: the loader demotes them to rawSource nodes
 * whose verbatim source survives a save (byte-for-byte for the block).
 */
export const LOSSY_FIXTURES: Fixture[] = [
  { name: "html comment", md: "<!-- todo: keep me -->" },
  { name: "html block", md: "<details>\n<summary>more</summary>\nhidden body\n</details>" },
  { name: "link definition", md: "[r]: https://example.com \"ref\"" },
  { name: "html entity raw", md: "&#9829; <span>sp</span>" },
];

/** Constructs where a verbatim write-back is impossible — normalization is the contract. */
export const NORMALIZED_FIXTURES: Fixture[] = [
  { name: "asterisk bullets to dash", md: "* a\n* b" },
  { name: "plus bullets to dash", md: "+ a\n+ b" },
  { name: "setext to atx", md: "Title\n===" },
  { name: "upper x checkbox", md: "- [X] done" },
  { name: "tight ordered relabel", md: "1. a\n9. b\n7. c" },
];

// Discoveries that moved between categories during the test run (kept here so
// the corpus reflects measured behavior, not guesses):
// - "`` special `code` ``" (backtick-adjacent codespan): serializer is NOT a
//   fixed point -> behaviorally demoted to rawSource. Data-safe, not editable.
// - unclosed fence: parses to a codeBlock and gains its closing fence on
//   save -> normalization, not loss.
// - "text[^1]" footnotes: marked has no footnote token; survives as literal
//   text with added backslash escapes -> normalization (remark would render a
//   real footnote: cross-renderer semantic drift, zero data loss).
LOSSY_FIXTURES.push(
  { name: "backtick-adjacent codespan", md: "`` special `code` ``" },
);
NORMALIZED_FIXTURES.push(
  { name: "unclosed fence gains closer", md: "```js\nconst a = 1;\nno closing fence\n" },
  { name: "footnote-ish escapes", md: "text[^1]" },
);
