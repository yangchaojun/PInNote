/**
 * Extension set for the prototype editor (wayfinder ticket 02), per
 * research/tiptap-markdown-roundtrip §5: StarterKit WITHOUT Underline
 * (serializes to non-GFM `++text++`), TaskList/TaskItem, the Table set,
 * the official Markdown extension, and our rawSource passthrough block.
 *
 * A factory, not a shared array: TipTap extension configs should not be
 * reused across editors (each consumer builds its own), and the mounted
 * editor configures RawSource with the "转为可编辑" callback while the
 * headless audit context does not.
 */
import StarterKit from "@tiptap/starter-kit";
import { TaskList, TaskItem } from "@tiptap/extension-list";
import { Table, TableRow, TableHeader, TableCell } from "@tiptap/extension-table";
import { Markdown } from "@tiptap/markdown";
import { Paragraph } from "@tiptap/extension-paragraph";
import { RawSource, type RawSourceOptions } from "./rawSource";

/**
 * Paragraph text that happens to start with a markdown block intro (`#`,
 * `>`, `- `, `1. `, …) must be backslash-escaped, or the serializer's output
 * re-parses as that block type instead of a paragraph (measured in the
 * browser sandbox: a literal "## typing test" paragraph made the next save
 * change the document). Otherwise replicate the stock renderer, including
 * the `&nbsp;` rule for consecutive empty paragraphs (research §1 pitfall 4).
 */
const EMPTY_PARAGRAPH_MARKDOWN = "&nbsp;";

const SafeParagraph = Paragraph.extend({
  renderMarkdown(node, h, ctx) {
    if (!node) return "";
    const content = Array.isArray(node.content) ? node.content : [];
    if (content.length === 0) {
      const prev = ctx?.previousNode as { type?: string; content?: unknown[] } | undefined;
      return prev?.type === "paragraph" && (prev.content?.length ?? 0) === 0
        ? EMPTY_PARAGRAPH_MARKDOWN
        : "";
    }
    const md = h.renderChildren(content);
    if (/^#{1,6}\s/.test(md) || /^>/.test(md) || /^[-*+]\s/.test(md)) {
      return `\\${md}`;
    }
    const ordered = md.match(/^(\d{1,9})([.)])(\s)/);
    if (ordered) return `${ordered[1]}\\${ordered[2]}${md.slice(ordered[0].length - 1)}`;
    return md;
  },
});

export function createExtensions(rawSourceOptions: RawSourceOptions = {}) {
  return [
    StarterKit.configure({
      underline: false,
      heading: { levels: [1, 2, 3, 4, 5, 6] },
      paragraph: false,
      link: { openOnClick: false, defaultProtocol: "https" },
    }),
    SafeParagraph,
    TaskList,
    TaskItem.configure({ nested: true }),
    Table.configure({ resizable: false }),
    TableRow,
    TableHeader,
    TableCell,
    RawSource.configure(rawSourceOptions),
    Markdown,
  ];
}
