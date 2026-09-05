/**
 * Smoke test: verify the whole headless pipeline works before betting the
 * round-trip suite on it (vitest environment: jsdom — see vite.config.ts).
 */
import { describe, expect, it } from "vitest";
import { createAuditContext } from "./context";
import { auditBlock, guardSaveRoundTrip, loadMarkdownLossless, splitBlocks } from "./audit";

describe("headless pipeline", () => {
  it("builds schema + manager without mounting a view", () => {
    const { ctx, destroy } = createAuditContext();
    expect(ctx.schema).toBeTruthy();
    expect(typeof ctx.manager.parse).toBe("function");
    destroy();
  });

  it("parses and serializes basic markdown", () => {
    const { ctx, destroy } = createAuditContext();
    const md = "# Title\n\nSome **bold** text.\n";
    const json = ctx.manager.parse(md);
    expect(json.content?.map((c) => c.type)).toEqual(["heading", "paragraph"]);
    const out = ctx.manager.serialize(json);
    expect(out).toContain("**bold**");
    expect(out).toContain("# Title");
    destroy();
  });

  it("serializes rawSource verbatim", () => {
    const { ctx, destroy } = createAuditContext();
    const json = {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "before" }] },
        { type: "rawSource", attrs: { source: "<!-- a comment -->\n" } },
        { type: "paragraph", content: [{ type: "text", text: "after" }] },
      ],
    };
    const out = ctx.manager.serialize(json);
    expect(out).toContain("<!-- a comment -->");
    expect(out).toContain("before");
    expect(out).toContain("after");
    destroy();
  });

  it("splits blocks and audits a simple doc", () => {
    const { ctx, destroy } = createAuditContext();
    const md = "# Title\n\npara one\n\n- item a\n- item b\n";
    const blocks = splitBlocks(md, ctx);
    expect(blocks.length).toBe(3);
    for (const b of blocks) {
      const a = auditBlock(b, ctx);
      if (!a.ok) console.log("AUDIT-REJECT", JSON.stringify(b), a.reason);
    }
    const { doc, demoted } = loadMarkdownLossless(md, ctx);
    console.log("DEMOTE:", demoted);
    const guard = guardSaveRoundTrip(doc, ctx);
    console.log("GUARD:", guard.ok, guard.error, "\n---\n" + guard.markdown);
    expect(guard.ok).toBe(true);
    destroy();
  });
});
