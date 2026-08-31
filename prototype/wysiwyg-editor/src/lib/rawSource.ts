/**
 * rawSource: an atom block node holding markdown the editor cannot model
 * verbatim (HTML blocks, comments, link definitions, anything the audit
 * rejects). `renderMarkdown` returns `source` unchanged, so uneditable
 * content is never lost or rewritten on save.
 *
 * Mechanism follows research §2 Q2: atom block + verbatim serializer. We do
 * NOT register a markdownTokenizer or parseMarkdown — the manager's block
 * parse never produces this node type; blocks are demoted to it by the
 * app-level audit loader in audit.ts, which is where "what is safe to parse"
 * is decided.
 */
import { Node } from "@tiptap/core";

export interface RawSourceOptions {
  /** Offer re-parsing the source as markdown into editable content, at the node's position. */
  onConvert?: (source: string, pos: number) => void;
}

export const RawSource = Node.create<RawSourceOptions>({
  name: "rawSource",
  group: "block",
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      source: { default: "" },
    };
  },

  parseHTML() {
    return [{ tag: "div[data-raw-source]" }];
  },

  renderHTML({ node }) {
    return [
      "div",
      { "data-raw-source": "" },
      ["pre", String(node.attrs?.source ?? "")],
    ];
  },

  renderMarkdown(node) {
    return String(node.attrs?.source ?? "");
  },

  addNodeView() {
    const { onConvert } = this.options;
    return ({ node, getPos }) => {
      const dom = document.createElement("div");
      dom.className = "raw-source-block";
      dom.setAttribute("contenteditable", "false");
      const source = String(node.attrs?.source ?? "");
      const pre = document.createElement("pre");
      pre.textContent = source;
      const convert = document.createElement("button");
      convert.type = "button";
      convert.textContent = "转为可编辑";
      convert.addEventListener("click", () => {
        const pos = getPos();
        if (typeof pos === "number") onConvert?.(source, pos);
      });
      dom.append(pre, convert);
      return { dom };
    };
  },
});
