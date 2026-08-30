import { Suspense, lazy, useCallback, useEffect, useRef, useState } from "react";
import { Button, Tooltip } from "antd";
import { CloseOutlined, EditOutlined, EyeOutlined, PushpinOutlined } from "@ant-design/icons";
import { Events, Window as WailsWindow } from "@wailsio/runtime";
import * as api from "./lib/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toggleCheckboxAtLine } from "./lib/markdown";

const MarkdownView = lazy(() => import("./components/MarkdownView").then((m) => ({ default: m.MarkdownView })));

const PIN_WIDTH = 380;
const MIN_HEIGHT = 120;
const MAX_HEIGHT_FRACTION = 0.85;

/**
 * A pinned desktop note: frameless, always-on-top window rendered from the
 * same frontend. The window auto-resizes to fit its content, and can be
 * dragged with the header bar / double-clicked to toggle edit mode.
 */
export function PinWindow({ noteId }: { noteId: string }) {
  const queryClient = useQueryClient();
  const { data: notes } = useQuery({ queryKey: ["notes"], queryFn: api.listNotes });
  const note = notes?.find((n) => n.id === noteId);

  const { data: trashNotes } = useQuery({
    queryKey: ["trash"],
    queryFn: api.listTrash,
    enabled: !note,
  });
  const trashedNote = !note ? trashNotes?.find((n) => n.id === noteId) : undefined;

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string | null>(null);
  const contentRef = useRef<HTMLDivElement | null>(null);
  const saveTimer = useRef<number | undefined>(undefined);

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["notes"] });
    queryClient.invalidateQueries({ queryKey: ["trash"] });
  }, [queryClient]);

  // Keep in sync with edits made elsewhere.
  useEffect(() => Events.On("notes:changed", refresh), [refresh]);

  // Close automatically when the note is trashed or purged elsewhere.
  useEffect(() => {
    if (!notes || !trashNotes) return;
    if (!note && !trashedNote) {
      WailsWindow.Close().catch(() => undefined);
    }
  }, [notes, trashNotes, note, trashedNote]);

  // --- Auto-resize: fit the window height to the rendered content ---
  const resize = useCallback(() => {
    const el = contentRef.current;
    if (!el) return;
    const target = Math.min(
      Math.max(el.scrollHeight + 42, MIN_HEIGHT),
      Math.round(window.screen.height * MAX_HEIGHT_FRACTION),
    );
    WailsWindow.SetSize(PIN_WIDTH, target).catch(() => undefined);
  }, []);

  useEffect(() => {
    const observer = new ResizeObserver(() => resize());
    if (contentRef.current) observer.observe(contentRef.current);
    resize();
    return () => observer.disconnect();
  }, [resize, editing, draft, note?.content]);

  const save = useCallback(
    (id: string, content: string) => {
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => {
        api.updateNote(id, content).then(refresh).catch(console.error);
      }, 400);
    },
    [refresh],
  );

  const unpin = async () => {
    await api.setPinned(noteId, false).catch(console.error);
    await api.closePinnedWindow(noteId).catch(console.error);
  };

  const toggleCheckbox = (line: number) => {
    if (!note) return;
    const content = draft ?? note.content;
    const next = toggleCheckboxAtLine(content, line);
    setDraft(next);
    save(noteId, next);
  };

  if (!note && !trashedNote) {
    return <div className="pin-window">加载中…</div>;
  }

  if (trashedNote && !note) {
    return (
      <div className="pin-window">
        <div className="pin-header">
          <span className="pin-header-title">已移入回收站</span>
          <Button size="small" type="text" icon={<CloseOutlined />} onClick={() => api.closePinnedWindow(noteId)} tabIndex={-1} />
        </div>
        <div className="pin-body">
          <p className="pin-note-title">{trashedNote.title || "无标题笔记"}</p>
          <p className="pin-hint">该笔记已被删除。</p>
        </div>
      </div>
    );
  }

  const content = draft ?? note!.content;

  return (
    <div className="pin-window">
      <div
        className="pin-header"
        onDoubleClick={() => setEditing((v) => !v)}
      >
        <PushpinOutlined className="pin-header-icon" />
        <span className="pin-header-title">{note!.title || "无标题笔记"}</span>
        <span className="pin-header-actions">
          <Tooltip title={editing ? "预览" : "编辑"}>
            <Button
              size="small"
              type="text"
              icon={editing ? <EyeOutlined /> : <EditOutlined />}
              onClick={() => setEditing((v) => !v)}
              tabIndex={-1}
            />
          </Tooltip>
          <Tooltip title="取消固定">
            <Button size="small" type="text" icon={<PushpinOutlined />} onClick={unpin} tabIndex={-1} />
          </Tooltip>
          <Button size="small" type="text" icon={<CloseOutlined />} onClick={() => api.closePinnedWindow(noteId)} tabIndex={-1} />
        </span>
      </div>
      <div className="pin-body" ref={contentRef}>
        {editing ? (
          <textarea
            className="pin-edit"
            value={content}
            autoFocus
            onChange={(e) => {
              setDraft(e.target.value);
              save(noteId, e.target.value);
            }}
            placeholder="用 Markdown 记录…"
          />
        ) : (
          <Suspense fallback={<span className="preview-loading">…</span>}>
            <MarkdownView content={content} onToggleCheckbox={toggleCheckbox} />
          </Suspense>
        )}
      </div>
    </div>
  );
}
