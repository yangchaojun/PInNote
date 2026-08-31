/**
 * The regression net research §1 pitfall 8 demands: every construct PinNote
 * renders today must round-trip as the same document, and every construct it
 * cannot render must survive a save verbatim.
 */
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import type { Node as PMNode } from "@tiptap/pm/model";
import { EditorState } from "@tiptap/pm/state";
import { createAuditContext } from "./context";
import { guardSaveRoundTrip, loadMarkdownLossless } from "./audit";
import type { AuditContext } from "./audit";
import { LOSSY_FIXTURES, NORMALIZED_FIXTURES, SAFE_FIXTURES } from "./fixtures";

let ctx: AuditContext;
let destroy: () => void;

beforeAll(() => {
  ({ ctx, destroy } = createAuditContext());
});
afterAll(() => destroy());

/** md → lossless-loaded doc (what the pin window does on open). */
function openNote(md: string): { doc: PMNode; demoted: string[] } {
  return loadMarkdownLossless(md, ctx);
}

/** One simulated save: serialize the live doc, then reopen it the same way the pin window loads (audit included). */
function saveAndReopen(doc: PMNode): { markdown: string; reopened: PMNode } {
  const guard = guardSaveRoundTrip(doc, ctx);
  expect(guard.ok, `save guard refused: ${guard.error}`).toBe(true);
  const reopened = loadMarkdownLossless(guard.markdown, ctx).doc;
  return { markdown: guard.markdown, reopened };
}

describe("safe subset: AST-level lossless round-trip", () => {
  for (const f of SAFE_FIXTURES) {
    it(`${f.name} — loads with no demotion and survives a save unchanged`, () => {
      const { doc, demoted } = openNote(f.md);
      expect(demoted, "safe syntax must not be demoted to rawSource").toEqual([]);
      const { reopened } = saveAndReopen(doc);
      expect(doc.eq(reopened), "reopen drifted the document").toBe(true);
    });
  }
});

describe("safe subset: serializer reaches a fixed point", () => {
  for (const f of SAFE_FIXTURES) {
    it(`${f.name} — a second save rewrites nothing`, () => {
      const { doc } = openNote(f.md);
      const { markdown: md1, reopened } = saveAndReopen(doc);
      const { markdown: md2 } = saveAndReopen(reopened);
      expect(md2).toBe(md1);
    });
  }
});

describe("lossy syntax: zero silent data loss", () => {
  for (const f of LOSSY_FIXTURES) {
    it(`${f.name} — demoted verbatim and byte-stable through a save`, () => {
      const { doc, demoted } = openNote(f.md);
      expect(demoted.length, "unsafe syntax must be demoted, not parsed").toBeGreaterThan(0);
      const { markdown } = saveAndReopen(doc);
      expect(markdown).toContain(f.md.trim());
      // Reopening the saved file must demote identically (stability).
      const again = openNote(markdown);
      expect(again.demoted.length).toBeGreaterThan(0);
    });
  }
});

describe("normalization is documented, not silent", () => {
  for (const f of NORMALIZED_FIXTURES) {
    it(`${f.name} — doc identical, source canonicalized`, () => {
      const { doc, demoted } = openNote(f.md);
      expect(demoted).toEqual([]);
      const { markdown } = saveAndReopen(doc);
      expect(markdown.trim()).not.toBe(f.md.trim()); // normalization happened
      expect(doc.eq(loadMarkdownLossless(markdown, ctx).doc)).toBe(true);
    });
  }
});

describe("checkbox write-back (replaces toggleCheckboxAtLine)", () => {
  it("toggling the taskItem attr serializes back as - [x]", () => {
    const { doc } = openNote("- [ ] buy milk\n- [x] already done");
    // Exactly what TaskItem's NodeView does on change: setNodeMarkup with the
    // flipped attr, then the normal serialize-on-update path (research §4).
    const state = EditorState.create({ schema: ctx.schema, doc });
    let target = -1;
    let targetNode: PMNode | undefined;
    doc.descendants((node, pos) => {
      if (target === -1 && node.type.name === "taskItem" && node.attrs.checked !== true) {
        target = pos;
        targetNode = node;
      }
      return true;
    });
    expect(target).toBeGreaterThanOrEqual(0);
    const tr = state.tr.setNodeMarkup(target, undefined, { ...targetNode!.attrs, checked: true });
    const markdown = ctx.manager.serialize(tr.doc.toJSON());
    expect(markdown).toContain("- [x] buy milk");
    expect(markdown).toContain("- [x] already done");
    // And the toggled doc still passes the save guard.
    const guard = guardSaveRoundTrip(tr.doc, ctx);
    expect(guard.ok, guard.error).toBe(true);
  });
});

describe("mounted-editor reality (found in the browser sandbox)", () => {
  it("a TrailingNode scaffolding paragraph never blocks a save", () => {
    const { doc } = openNote("# T\n\nbody");
    // StarterKit's TrailingNode keeps the live doc ending with an empty
    // paragraph; it must not make the guard refuse every real save.
    const trailing = ctx.schema.nodeFromJSON({
      type: "doc",
      content: [...doc.toJSON().content!, { type: "paragraph" }],
    });
    const guard = guardSaveRoundTrip(trailing, ctx);
    expect(guard.ok, guard.error).toBe(true);
  });
});

describe("literal markdown-looking text (SafeParagraph escaping)", () => {
  it("a paragraph starting with '## ' survives the save intact", () => {
    // The stock paragraph renderer does not escape block intros; without our
    // override this exact case made the guard refuse every real edit (found
    // in the browser sandbox by typing "## typing test").
    const doc = ctx.schema.nodeFromJSON({
      type: "doc",
      content: [
        { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "T" }] },
        { type: "paragraph", content: [{ type: "text", text: "## typing test" }] },
        { type: "paragraph", content: [{ type: "text", text: "- not a list" }] },
        { type: "paragraph", content: [{ type: "text", text: "1. not a list either" }] },
      ],
    });
    const guard = guardSaveRoundTrip(doc, ctx);
    expect(guard.ok, guard.error).toBe(true);
    const reopened = loadMarkdownLossless(guard.markdown, ctx).doc;
    expect(doc.eq(reopened)).toBe(true);
    expect(guard.markdown).toContain("\\##");
  });
});

  it("a lone blank paragraph between blocks does not wedge saving", () => {
    // Pressing Enter twice mid-note is the most common way to create this
    // shape; markdown cannot represent the blank line as content, so the
    // guard must tolerate its collapse instead of refusing forever.
    const doc = ctx.schema.nodeFromJSON({
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "a" }] },
        { type: "paragraph" },
        { type: "paragraph", content: [{ type: "text", text: "b" }] },
      ],
    });
    const guard = guardSaveRoundTrip(doc, ctx);
    expect(guard.ok, guard.error).toBe(true);
    expect(guard.markdown).toContain("a");
    expect(guard.markdown).toContain("b");
  });

  it("consecutive empty paragraphs (the &nbsp; noise) never block a save", () => {
    // 2nd+ consecutive empty paragraph serializes to `&nbsp;` (research §1.4)
    // and reopens as a U+00A0 text node — must not fail the guard.
    const doc = ctx.schema.nodeFromJSON({
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "body" }] },
        { type: "paragraph" },
        { type: "paragraph" },
      ],
    });
    const guard = guardSaveRoundTrip(doc, ctx);
    expect(guard.ok, guard.error).toBe(true);
  });

describe("mixed task/bullet list split (research §1 pitfall 3)", () => {
  it("splits into adjacent lists yet stays a fixed point", () => {
    const { doc, demoted } = openNote("- plain\n- [x] task\n- plain again");
    expect(demoted).toEqual([]);
    const types = doc.children.map((c) => c.type.name);
    expect(types).toContain("taskList");
    expect(types.filter((t) => t === "bulletList").length + types.filter((t) => t === "taskList").length)
      .toBeGreaterThanOrEqual(2);
    const { markdown } = saveAndReopen(doc);
    const { markdown: md2 } = saveAndReopen(loadMarkdownLossless(markdown, ctx).doc);
    expect(md2).toBe(markdown);
  });
});
