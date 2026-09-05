import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "antd";
import { CloseOutlined, PushpinOutlined } from "@ant-design/icons";
import { Events, Window as WailsWindow } from "@wailsio/runtime";
import * as api from "./lib/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { PinEditor, type PinEditorHandle } from "./PinEditor";
import { usePinShortcuts } from "./hooks/usePinShortcuts";
import { ShortcutsModal } from "./components/ShortcutsModal";

/**
 * A pinned desktop note — the app's entire UI (pin-only form factor).
 * Frameless, always-on-top, opens ready to type, autosaves. Closing the
 * window is only "collapse": the note stays and its window returns on the
 * next launch. A blank note is hard-deleted on close instead.
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

  const [helpOpen, setHelpOpen] = useState(false);
  const editorRef = useRef<PinEditorHandle | null>(null);

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["notes"] });
    queryClient.invalidateQueries({ queryKey: ["trash"] });
  }, [queryClient]);

  // Keep queries in sync with edits made in other windows.
  useEffect(() => Events.On("notes:changed", refresh), [refresh]);

  // A live note is one that close/quit may apply the empty-note rule to.
  const liveRef = useRef(false);
  liveRef.current = !!note;

  // Close: flush first, then apply the empty-note rule, then collapse. A note
  // that never got content is hard-deleted (never enters the trash). When the
  // flush failed the note is kept — the editor may hold content that never
  // reached the DB, and DiscardIfEmpty would judge on the stale blank record.
  const close = useCallback(async () => {
    await editorRef.current?.flush().catch((err) => console.error("flush on close", err));
    if (editorRef.current?.isDirty()) {
      console.error("pin window close: unsaved edits remain; keeping the note");
    } else {
      await api.discardIfEmpty(noteId).catch((err) => console.error("discard empty", err));
    }
    await api.closePinnedWindow(noteId).catch((err) => console.error("close window", err));
  }, [noteId]);

  // Delete (⌘⌫ / menu bar): flush first so in-flight edits are not lost,
  // then trash (60-day recycle bin), then close.
  const deleteNote = useCallback(async () => {
    await editorRef.current?.flush().catch((err) => console.error("flush on delete", err));
    await api.trashNote(noteId).catch((err) => console.error("trash note", err));
    await api.closePinnedWindow(noteId).catch((err) => console.error("close window", err));
  }, [noteId]);

  // Menu-bar fallback for ⌘⌫: the backend emits an app-wide event carrying
  // the target note id (window events broadcast); only the addressed window acts.
  useEffect(
    () =>
      Events.On("pin:delete-requested", (ev) => {
        const d = (ev as { data?: unknown }).data;
        const target = Array.isArray(d) ? d[0] : d;
        if (target !== noteId) return;
        void deleteNote();
      }),
    [deleteNote, noteId],
  );

  // 退出收尾 (§5.2): on app quit, apply the empty-note rule after the
  // editor's flush has run (child effects registered their beforeunload
  // first). Only fires for live notes — a just-trashed note must keep its
  // 60-day window, and DiscardIfEmpty itself only deletes blank content.
  useEffect(() => {
    const onUnload = () => {
      if (liveRef.current) {
        api.discardIfEmpty(noteId).catch(() => undefined);
      }
    };
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, [noteId]);

  // Close automatically when the note is deleted or purged elsewhere.
  useEffect(() => {
    if (notes && trashNotes && !note && !trashedNote) {
      WailsWindow.Close().catch(() => undefined);
    }
  }, [notes, trashNotes, note, trashedNote]);

  usePinShortcuts({
    getEditor: () => editorRef.current?.getEditor() ?? null,
    onDelete: () => void deleteNote(),
    onToggleHelp: () => setHelpOpen((v) => !v),
    helpOpen,
    onCloseHelp: () => setHelpOpen(false),
  });

  if (!note && !trashedNote) {
    return <div className="pin-window">加载中…</div>;
  }

  if (trashedNote && !note) {
    return (
      <div className="pin-window">
        <div className="pin-header">
          <PushpinOutlined className="pin-header-icon" />
          <div className="pin-header-drag" title="已移入回收站" />
          <Button
            size="small"
            type="text"
            icon={<CloseOutlined />}
            onClick={() => void api.closePinnedWindow(noteId)}
            tabIndex={-1}
          />
        </div>
        <div className="pin-body">
          <p className="pin-hint">该笔记已被删除。</p>
        </div>
      </div>
    );
  }

  // The header shows no title while in use; hovering the drag area reveals
  // the derived first-line title as a native tooltip.
  return (
    <div className="pin-window">
      <div className="pin-header">
        <PushpinOutlined className="pin-header-icon" />
        <div className="pin-header-drag" title={note!.title || "无标题笔记"} />
        <Button size="small" type="text" icon={<CloseOutlined />} onClick={() => void close()} tabIndex={-1} />
      </div>
      <div className="pin-body">
        <PinEditor ref={editorRef} noteId={noteId} initialContent={note!.content} onSaved={refresh} />
      </div>
      <ShortcutsModal open={helpOpen} onClose={() => setHelpOpen(false)} />
    </div>
  );
}
