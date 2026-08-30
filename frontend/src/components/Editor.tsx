import { Suspense, lazy, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Button, Empty, Input, Tooltip } from "antd";
import {
  BoldOutlined,
  CheckSquareOutlined,
  CodeOutlined,
  ItalicOutlined,
  LinkOutlined,
  OrderedListOutlined,
  StrikethroughOutlined,
  UnorderedListOutlined,
  EyeOutlined,
  EditOutlined,
} from "@ant-design/icons";
// The markdown renderer (~150 KB) is only needed once the preview is opened.
const MarkdownView = lazy(() => import("./MarkdownView").then((m) => ({ default: m.MarkdownView })));
import * as api from "../lib/api";
import * as md from "../lib/markdown";
import { useUIStore } from "../store";

interface EditorProps {
  note: api.Note;
  /** Applies a content change (autosave mutations are handled here). */
  onContentChange: (id: string, content: string) => void;
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>;
}

/**
 * Markdown source editor with formatting shortcuts and an optional live
 * preview pane. Saves are debounced by the caller via onContentChange.
 */
export function Editor({ note, onContentChange, textareaRef }: EditorProps) {
  const [draft, setDraft] = useState(note.content);
  const localRef = useRef<HTMLTextAreaElement | null>(null);
  const pendingSelection = useRef<[number, number] | null>(null);
  const showPreview = useUIStore((s) => s.showPreview);

  // Sync draft when switching notes (or when another window changed it while
  // this note is not being edited).
  useEffect(() => {
    setDraft(note.content);
  }, [note.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Accept external content updates (e.g. checkbox toggled in preview).
  useEffect(() => {
    if (document.activeElement !== localRef.current && note.content !== draft) {
      setDraft(note.content);
    }
  }, [note.content]); // eslint-disable-line react-hooks/exhaustive-deps

  const applyFormat = useCallback(
    (fn: md.FormatFn) => {
      const el = localRef.current;
      if (!el) return;
      const result = fn(draft, el.selectionStart, el.selectionEnd);
      // Selection is restored after the controlled re-render commits (a plain
      // RAF would race with React's value assignment resetting the caret).
      pendingSelection.current = [result.selectionStart, result.selectionEnd];
      setDraft(result.content);
      onContentChange(note.id, result.content);
    },
    [draft, note.id, onContentChange],
  );

  useLayoutEffect(() => {
    if (pendingSelection.current && localRef.current) {
      const [s, e] = pendingSelection.current;
      localRef.current.focus();
      localRef.current.setSelectionRange(s, e);
      pendingSelection.current = null;
    }
  }, [draft]);

  // Expose formatting shortcuts to the app-level key handler.
  useEffect(() => {
    const host = localRef.current as unknown as { __applyFormat?: (fn: md.FormatFn) => void } | null;
    if (host) host.__applyFormat = applyFormat;
    return () => {
      const h = localRef.current as unknown as { __applyFormat?: unknown } | null;
      if (h) delete h.__applyFormat;
    };
  }, [applyFormat]);

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setDraft(e.target.value);
    onContentChange(note.id, e.target.value);
  };

  const toggleCheckbox = (line: number) => {
    const next = md.toggleCheckboxAtLine(note.content, line);
    setDraft(next);
    onContentChange(note.id, next);
  };

  const formatButtons: Array<[React.ReactNode, string, md.FormatFn]> = [
    [<BoldOutlined key="b" />, "粗体 ⌘B", md.formatBold],
    [<ItalicOutlined key="i" />, "斜体 ⌘I", md.formatItalic],
    [<StrikethroughOutlined key="s" />, "删除线", md.formatStrikethrough],
    [<CodeOutlined key="c" />, "行内代码", md.formatCode],
    [<LinkOutlined key="l" />, "链接 ⌘K", md.formatLink],
    [<UnorderedListOutlined key="ul" />, "无序列表 ⇧⌘8", md.formatBulletList],
    [<OrderedListOutlined key="ol" />, "有序列表 ⇧⌘7", md.formatOrderedList],
    [<CheckSquareOutlined key="cb" />, "待办事项 ⇧⌘X", md.formatCheckbox],
  ];

  return (
    <div className="editor">
      <div className="editor-toolbar">
        {formatButtons.map(([icon, tip, fn], i) => (
          <Tooltip key={i} title={tip}>
            <Button type="text" size="small" icon={icon} onClick={() => applyFormat(fn)} tabIndex={-1} />
          </Tooltip>
        ))}
        <span className="editor-toolbar-spacer" />
        <Tooltip title={showPreview ? "关闭预览 ⌘E" : "预览 ⌘E"}>
          <Button
            type="text"
            size="small"
            icon={showPreview ? <EditOutlined /> : <EyeOutlined />}
            onClick={() => useUIStore.getState().togglePreview()}
            tabIndex={-1}
          />
        </Tooltip>
      </div>
      <div className={`editor-body${showPreview ? " split" : ""}`}>
        <Input.TextArea
          ref={(el) => {
            localRef.current = el?.resizableTextArea?.textArea ?? null;
            if (textareaRef) textareaRef.current = el?.resizableTextArea?.textArea ?? null;
          }}
          value={draft}
          onChange={handleChange}
          placeholder="用 Markdown 记录：# 标题、- 列表、- [ ] 待办…"
          variant="borderless"
          className="editor-textarea"
          spellCheck={false}
          onKeyDown={(e) => {
            // Enter inside a checkbox/list line continues the list.
            if (e.key === "Enter" && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
              const el = localRef.current;
              if (!el) return;
              const before = draft.slice(0, el.selectionStart);
              const currentLine = before.slice(before.lastIndexOf("\n") + 1);
              const cont = currentLine.match(/^(\s*)([-*+]\s\[[ xX]\]\s|[-*+]\s|\d+\.\s)/);
              if (cont) {
                e.preventDefault();
                const item = cont[2];
                // Empty item: pressing Enter ends the list.
                if (currentLine.trim() === item.trim()) {
                  const next = draft.slice(0, before.length - currentLine.length) + draft.slice(before.length);
                  setDraft(next);
                  onContentChange(note.id, next);
                  const pos = el.selectionStart - currentLine.length;
                  requestAnimationFrame(() => el.setSelectionRange(pos, pos));
                } else {
                  const insert = "\n" + cont[1] + (item.match(/\d+/) ? incrementOrdered(item) : item.replace(/\[[xX]\]/, "[ ]"));
                  const pos = el.selectionStart;
                  const next = draft.slice(0, pos) + insert + draft.slice(el.selectionEnd);
                  setDraft(next);
                  onContentChange(note.id, next);
                  const target = pos + insert.length;
                  requestAnimationFrame(() => el.setSelectionRange(target, target));
                }
              }
            }
          }}
        />
        {showPreview && (
          <div className="editor-preview">
            {draft.trim() ? (
              <Suspense fallback={<span className="preview-loading">加载预览…</span>}>
                <MarkdownView content={draft} onToggleCheckbox={toggleCheckbox} />
              </Suspense>
            ) : (
              <Empty description="暂无内容" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function incrementOrdered(item: string): string {
  return item.replace(/\d+/, (n) => String(Number(n) + 1));
}
