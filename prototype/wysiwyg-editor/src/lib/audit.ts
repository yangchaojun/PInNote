/**
 * Lossless loader + save guard for the "Markdown fact-source, WYSIWYG view"
 * model (research §2, revised by the prototype).
 *
 * A block is safe to parse iff it passes three gates:
 *  1. every marked token type inside it is in the schema's covered set
 *     (catches silent stripping the other gates can't see — measured:
 *     `<details>` parses to its inner text and the tags vanish);
 *  2. the parse result only contains known top-level node types
 *     (catches silent drops, e.g. link definitions);
 *  3. behavioral fixed point serialize∘parse∘serialize + doc equality —
 *     auto-tracks TipTap extension upgrades (early-release risk, §1.8).
 * Ticket 01 recommended the static whitelist alone (gate 1); the prototype
 * adds 2–3 so the audit can't go stale silently.
 *
 * Unsafe blocks are demoted to `rawSource` atom nodes holding the block's
 * verbatim source, which renderMarkdown writes back byte-for-byte.
 */
import { Fragment } from "@tiptap/pm/model";
import type { Node, Schema } from "@tiptap/pm/model";
import type { JSONContent } from "@tiptap/core";
import type { MarkdownManager } from "@tiptap/markdown";

export interface AuditContext {
  manager: MarkdownManager;
  schema: Schema;
}

/**
 * Top-level block types the editor understands. A parse result containing
 * anything else means data left the model (the silent-drop failure mode).
 */
const KNOWN_TOP_LEVEL_BLOCKS = new Set([
  "paragraph",
  "heading",
  "bulletList",
  "orderedList",
  "taskList",
  "blockquote",
  "codeBlock",
  "table",
  "horizontalRule",
  "rawSource",
]);

export interface BlockAudit {
  raw: string;
  ok: boolean;
  /** marked token types seen in the block (for diagnostics + the type gate). */
  tokenTypes?: string[];
  /** Reason for rejection, for diagnostics. */
  reason?: string;
}

/**
 * Gate 1 (static): every marked token type (block or inline, at any nesting
 * level) the schema + serializer cover — research §1's per-syntax table.
 * Types outside it (`html`, `def`, …) are unsafe even if the fixed point
 * seems stable, because the manager strips them silently:
 * `<details>…</details>` parses to its text and the tags vanish.
 */
const SAFE_TOKEN_TYPES = new Set([
  // blocks
  "space",
  "code",
  "fences",
  "heading",
  "paragraph",
  "blockquote",
  "list",
  "list_item",
  "taskList", // custom token types registered by @tiptap's own tokenizers
  "taskItem",
  "table",
  "hr",
  // inlines
  "text",
  "escape",
  "strong",
  "em",
  "codespan",
  "br",
  "del",
  "link",
  "image",
  "checkbox", // GFM task markers inside mixed lists (@tiptap tokenizer → taskItem.checked)
]);

/** Recursively collect every marked token type appearing in a block's raw. */
function collectTokenTypes(raw: string, ctx: AuditContext): string[] {
  const found = new Set<string>();
  const isToken = (v: unknown): v is { type: string } =>
    typeof v === "object" && v !== null && typeof (v as { type?: unknown }).type === "string";
  const walk = (token: { type: string }) => {
    found.add(token.type);
    for (const value of Object.values(token)) {
      if (!Array.isArray(value)) continue;
      for (const child of value) {
        if (Array.isArray(child)) child.filter(isToken).forEach(walk);
        else if (isToken(child)) walk(child);
      }
    }
  };
  for (const top of ctx.manager.instance.lexer(raw)) walk(top);
  return [...found];
}

/**
 * Audit one markdown block. Three gates:
 *  1. token-type whitelist (catches silent stripping the fixed point can't see);
 *  2. parse-result node types must be fully known (catches silent drops);
 *  3. behavioral fixed point serialize∘parse∘serialize + doc equality (catches
 *     churn and drift, auto-tracking extension upgrades per research §1.8).
 */
export function auditBlock(raw: string, ctx: AuditContext): BlockAudit {
  const trimmed = raw.trim();
  if (trimmed.length === 0) return { raw, ok: false, reason: "blank block" };
  let types: string[];
  try {
    types = collectTokenTypes(trimmed, ctx);
  } catch (err) {
    return { raw, ok: false, reason: `lexer threw: ${String(err)}` };
  }
  for (const type of types) {
    if (!SAFE_TOKEN_TYPES.has(type)) {
      return { raw, tokenTypes: types, ok: false, reason: `unsafe token type "${type}"` };
    }
  }
  try {
    const first = ctx.manager.parse(trimmed);
    const children = first.content ?? [];
    if (children.length === 0) {
      return { raw, ok: false, reason: "parsed to empty document (content would be dropped)" };
    }
    for (const child of children) {
      if (!child.type || !KNOWN_TOP_LEVEL_BLOCKS.has(child.type)) {
        return { raw, ok: false, reason: `unknown top-level node type "${child.type}"` };
      }
    }
    const md1 = ctx.manager.serialize(first);
    const second = ctx.manager.parse(md1);
    const md2 = ctx.manager.serialize(second);
    if (md1 !== md2) {
      return { raw, ok: false, reason: "serializer is not a fixed point (saves would churn)" };
    }
    if (!ctx.schema.nodeFromJSON(first).eq(ctx.schema.nodeFromJSON(second))) {
      return { raw, ok: false, reason: "serialize→parse drifts the document" };
    }
    return { raw, tokenTypes: types, ok: true };
  } catch (err) {
    return { raw, ok: false, reason: `pipeline threw: ${String(err)}` };
  }
}

/** Top-level block raws for a document, via the manager's own marked lexer. */
export function splitBlocks(md: string, ctx: AuditContext): string[] {
  return ctx.manager.instance
    .lexer(md)
    .filter((t) => t.type !== "space")
    .map((t) => t.raw);
}

/**
 * Parse markdown to a schema-valid doc. An empty / whitespace-only source
 * parses to a childless doc, which the content-bearing `doc: block+` spec
 * rejects — that blank case becomes a single empty paragraph (the pin
 * window's normal empty-editing state, decision Q11).
 */
export function parseToDoc(md: string, ctx: AuditContext): Node {
  const children = ctx.manager.parse(md).content ?? [];
  return ctx.schema.nodeFromJSON({
    type: "doc",
    content: children.length > 0 ? children : [{ type: "paragraph" }],
  });
}

/**
 * Lossless document load: every top-level block either becomes real editable
 * content (audit passes) or a verbatim rawSource node.
 */
export function loadMarkdownLossless(md: string, ctx: AuditContext): { doc: Node; demoted: string[] } {
  const content: JSONContent[] = [];
  const demoted: string[] = [];
  for (const block of splitBlocks(md, ctx)) {
    const trimmed = block.trim();
    if (trimmed.length === 0) continue;
    // Symmetric counterpart of the serializer's EMPTY_PARAGRAPH_MARKDOWN
    // convention (research §1 pitfall 4): an `&nbsp;` block is an empty
    // paragraph, not unsafe html.
    if (trimmed === "&nbsp;") {
      content.push({ type: "paragraph" });
      continue;
    }
    const audit = auditBlock(trimmed, ctx);
    if (audit.ok) {
      content.push(...(ctx.manager.parse(trimmed).content ?? []));
    } else {
      demoted.push(trimmed);
      content.push({ type: "rawSource", attrs: { source: trimmed } });
    }
  }
  const doc = ctx.schema.nodeFromJSON({
    type: "doc",
    content: content.length > 0 ? content : [{ type: "paragraph" }],
  });
  doc.check();
  return { doc, demoted };
}

export interface SaveGuardResult {
  ok: boolean;
  /** The markdown to persist — only trustworthy when ok is true. */
  markdown: string;
  error?: string;
}

/**
 * Top-level blank paragraphs serialize/reopen inconsistently: TrailingNode
 * keeps a scaffolding paragraph at the doc end; a lone empty paragraph in
 * the middle collapses into block spacing; `&nbsp;` noise marks the 2nd+
 * consecutive one (research §1 pitfall 4). None of them carries content, so
 * the save invariant compares docs AFTER dropping top-level blank
 * paragraphs — guarding DATA, not blank-line formatting (measured: without
 * this, pressing Enter twice mid-note permanently wedged saving).
 */
function isBlankishParagraph(n: Node): boolean {
  if (n.type.name !== "paragraph") return false;
  if (n.childCount === 0) return true;
  if (n.childCount !== 1) return false;
  const child = n.firstChild!;
  return child.isText && /^[ \u00a0]*$/.test(child.text ?? "");
}

export function stripBlankParagraphs(doc: Node): Node {
  const children = doc.children.filter((c) => !isBlankishParagraph(c));
  if (children.length === 0) {
    return doc.copy(Fragment.from([doc.type.schema.nodes.paragraph.create()]));
  }
  return doc.copy(Fragment.from(children));
}

/**
 * Pre-write invariant (research §2 "防丢保险"): the live doc must survive
 * serialize → reopen (through the same audit loader) unchanged; otherwise
 * the write is refused rather than allowed to corrupt the fact source.
 */
export function guardSaveRoundTrip(doc: Node, ctx: AuditContext): SaveGuardResult {
  try {
    const markdown = ctx.manager.serialize(doc.toJSON());
    // Reopen exactly like the pin window opening the note again (audit
    // included): demoted rawSource blocks must re-demote identically; a bare
    // manager.parse would "lose" the very content rawSource preserves.
    const back = loadMarkdownLossless(markdown, ctx).doc;
    if (!stripBlankParagraphs(doc).eq(stripBlankParagraphs(back))) {
      return { ok: false, markdown, error: "round-trip changed the document; refusing write" };
    }
    return { ok: true, markdown };
  } catch (err) {
    return { ok: false, markdown: "", error: `round-trip threw: ${String(err)}` };
  }
}
