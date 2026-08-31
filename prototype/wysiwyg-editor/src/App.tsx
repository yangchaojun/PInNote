/**
 * Runnable sandbox for ticket 02's acceptance list.
 *
 * Left: the pin-window editor (full-window WYSIWYG). Every edit goes through
 * the real save path: onUpdate → guardSaveRoundTrip → (simulated) PUT, and
 * the resulting markdown fact-source is shown live on the right. A "reopen"
 * button replays the saved source through the audit loader — exactly what
 * the pin window would do on next launch.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import type { Editor } from "@tiptap/core";
import { createExtensions } from "./lib/editorConfig";
import { guardSaveRoundTrip, loadMarkdownLossless } from "./lib/audit";
import type { AuditContext } from "./lib/audit";

const DEMO_NOTE = `# 购物清单

买 **牛奶**、*鸡蛋*、~~橘子~~

- [ ] 周末采购
- [x] 列清单
  - [x] 检查冰箱

\`\`\`sh
echo "code stays code"
\`\`\`

> 引用一行

| 品项 | 数量 |
|------|:---:|
| 牛奶 | 2 |

<!-- 这行注释不在编辑器的模型里，会以 rawSource 块原样保留 -->

[ref]: https://example.com "链接定义同理"
`;

const PASTE_HTML_SAMPLE = `<h2>来自网页的片段</h2><p>带 <strong>粗体</strong>、<a href="https://example.com">链接</a> 和一个列表：</p><ul><li>第一项</li><li>第二项</li></ul><table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`;

interface SaveState {
  status: "idle" | "ok" | "refused";
  markdown: string;
  error?: string;
  savedAt?: string;
}

function auditOf(editor: Editor): AuditContext {
  if (!editor.markdown) throw new Error("no markdown manager on editor");
  return { manager: editor.markdown, schema: editor.schema };
}

/** The live save path: serialize → invariant guard → "PUT". */
function saveDoc(editor: Editor): SaveState {
  const guard = guardSaveRoundTrip(editor.state.doc, auditOf(editor));
  if (!guard.ok) {
    return { status: "refused", markdown: guard.markdown, error: guard.error };
  }
  return { status: "ok", markdown: guard.markdown, savedAt: new Date().toLocaleTimeString() };
}

export function App() {
  const [source, setSource] = useState(DEMO_NOTE);
  const [save, setSave] = useState<SaveState>({ status: "idle", markdown: "" });
  const [reopenNote, setReopenNote] = useState("");
  const saveTimer = useRef<number | undefined>(undefined);
  const sourceRef = useRef(source);
  sourceRef.current = source;

  const scheduleSave = useCallback((editor: Editor) => {
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(() => setSave(saveDoc(editor)), 400);
  }, []);

  // RawSource "转为可编辑": re-run the SAME audit on the stored source and
  // splice the audited nodes back in. (Inserting the raw markdown directly
  // would bypass the audit — measured: a demoted `<!-- comment -->` silently
  // vanished on conversion. Unsafe sources simply re-demote; data survives.)
  const convert = useCallback((text: string, pos: number) => {
    const ed = editorRef.current;
    if (!ed) return;
    const { doc } = loadMarkdownLossless(text, auditOf(ed));
    const size = ed.state.doc.nodeAt(pos)?.nodeSize ?? 1;
    ed
      .chain()
      .focus()
      .deleteRange({ from: pos, to: pos + size })
      .insertContentAt(pos, doc.toJSON().content ?? [])
      .run();
  }, []);

  const editorRef = useRef<Editor | null>(null);
  const editor = useEditor(
    {
      extensions: createExtensions({
        onConvert: convert,
      }),
      content: "",
      onUpdate: ({ editor: ed }) => scheduleSave(ed),
    },
    [scheduleSave, convert],
  );
  editorRef.current = editor;

  // Initial open: load the demo note through the audit loader.
  useEffect(() => {
    if (!editor) return;
    const { doc, demoted } = loadMarkdownLossless(sourceRef.current, auditOf(editor));
    editor.commands.command(({ tr }) => {
      tr.replaceWith(0, editor.state.doc.content.size, doc.content);
      return true;
    });
    setReopenNote(`${demoted.length} 个块被降级为 rawSource（原样保留）`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor]);

  const reopen = useCallback(() => {
    if (!editor) return;
    const { doc, demoted } = loadMarkdownLossless(save.markdown, auditOf(editor));
    editor.commands.command(({ tr }) => {
      tr.replaceWith(0, editor.state.doc.content.size, doc.content);
      return true;
    });
    setReopenNote(
      demoted.length === 0
        ? "重开完成：全部块可编辑"
        : `重开完成：${demoted.length} 个块仍是 rawSource（与保存前一致）`,
    );
  }, [editor, save.markdown]);

  const pasteRich = useCallback(() => {
    // Same pipeline a real rich-text paste takes: HTML → parseDOM → doc.
    editorRef.current?.chain().focus().insertContent(PASTE_HTML_SAMPLE, { contentType: "html" }).run();
  }, []);

  const insertMarkdownText = useCallback(() => {
    // Plain-text markdown paste (needs the explicit contentType; research §3).
    editorRef.current?.chain().focus().insertContent("1. 有序\n2. **带粗体**\n\n> 引用", { contentType: "markdown" }).run();
  }, []);

  const stats = useMemo(() => {
    const rawBlocks = editor ? editor.state.doc.children.filter((c) => c.type.name === "rawSource").length : 0;
    return { rawBlocks };
  }, [editor, save]);

  return (
    <div className="sandbox">
      <header>
        <h1>PinNote WYSIWYG 原型（ticket 02）</h1>
        <div className="toolbar">
          <button onClick={pasteRich}>模拟富文本粘贴（HTML）</button>
          <button onClick={insertMarkdownText}>插入 Markdown 文本</button>
          <button onClick={reopen} disabled={save.status !== "ok"}>
            用保存结果重开笔记
          </button>
          <span className="status">
            {save.status === "idle" && "编辑后 400ms 自动保存（400ms debounce 与线上一致）"}
            {save.status === "ok" && `✓ 已保存 ${save.savedAt} · doc↔markdown 不变式通过`}
            {save.status === "refused" && `✗ 守卫拒写：${save.error}`}
          </span>
          <span className="status">未解析块：{stats.rawBlocks}（rawSource 原样保留）</span>
        </div>
      </header>
      <main>
        <section className="pane">
          <h2>整窗编辑（WYSIWYG）</h2>
          <p className="hint">
            输入 `**粗体**`、`# `、`- [ ]` 原地即时渲染；点击复选框、⌘B/⌘I 快捷键、直接粘贴富文本均回写事实源。
          </p>
          <div className="editor-scroll">{editor ? <EditorContent editor={editor} /> : null}</div>
          <p className="hint">{reopenNote}</p>
        </section>
        <section className="pane">
          <h2>事实源 Markdown（note.content）</h2>
          <textarea
            className="source"
            value={save.status !== "idle" ? save.markdown : "(尚未保存)"}
            readOnly
          />
        </section>
      </main>
    </div>
  );
}
