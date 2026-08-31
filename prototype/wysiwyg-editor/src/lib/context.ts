/**
 * Builds an AuditContext from the exact editor extension set, without
 * mounting a view: the headless Editor only supplies the real schema, and
 * `editor.markdown` (registered by the Markdown extension) is the
 * parse/serialize manager.
 * (Core Editor requires a `window` — tests run under jsdom, the app runs
 * in the browser.)
 */
import { Editor } from "@tiptap/core";
import type { AuditContext } from "./audit";
import { createExtensions } from "./editorConfig";

export function createAuditContext(): { ctx: AuditContext; destroy: () => void } {
  const editor = new Editor({ extensions: createExtensions() });
  const manager = editor.markdown;
  if (!manager) {
    throw new Error("Markdown extension did not register editor.markdown");
  }
  return {
    ctx: { manager, schema: editor.schema },
    destroy: () => editor.destroy(),
  };
}
