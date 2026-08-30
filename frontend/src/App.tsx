import { useCallback, useEffect, useMemo, useRef } from "react";
import { Button, Input, Segmented, Tooltip } from "antd";
import {
  DeleteOutlined,
  PushpinOutlined,
  QuestionCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  PlusOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { Editor } from "./components/Editor";
import { NoteList } from "./components/NoteList";
import { TrashView } from "./components/TrashView";
import { ShortcutsModal } from "./components/ShortcutsModal";
import { useMainShortcuts } from "./hooks/useMainShortcuts";
import * as api from "./lib/api";
import { useUIStore } from "./store";

const AUTOSAVE_DELAY_MS = 400;

export function App() {
  const queryClient = useQueryClient();
  const {
    activeNoteId,
    searchQuery,
    view,
    setActiveNote,
    setSearchQuery,
    setView,
    setShortcutsOpen,
  } = useUIStore();

  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);
  const saveTimers = useRef<Map<string, number>>(new Map());

  const { data: notes = [] } = useQuery({ queryKey: ["notes"], queryFn: api.listNotes });
  const { data: trash = [] } = useQuery({ queryKey: ["trash"], queryFn: api.listTrash });

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["notes"] });
    queryClient.invalidateQueries({ queryKey: ["trash"] });
  }, [queryClient]);

  const createMutation = useMutation({
    mutationFn: () => api.createNote(""),
    onSuccess: (note) => {
      invalidate();
      setView("notes");
      setActiveNote(note.id);
      requestAnimationFrame(() => editorRef.current?.focus());
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, content }: { id: string; content: string }) => api.updateNote(id, content),
    onSuccess: invalidate,
  });

  const scheduleSave = useCallback(
    (id: string, content: string) => {
      const timers = saveTimers.current;
      window.clearTimeout(timers.get(id));
      timers.set(
        id,
        window.setTimeout(() => {
          timers.delete(id);
          updateMutation.mutate({ id, content });
        }, AUTOSAVE_DELAY_MS),
      );
    },
    // updateMutation is stable enough for this usage; keep the callback identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [invalidate],
  );

  const trashMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.closePinnedWindow(id).catch(() => undefined);
      return api.trashNote(id);
    },
    onSuccess: () => {
      invalidate();
      setActiveNote(null);
    },
  });

  const pinMutation = useMutation({
    mutationFn: async ({ id, pinned }: { id: string; pinned: boolean }) => {
      await api.setPinned(id, pinned);
      if (pinned) await api.openPinnedWindow(id);
      else await api.closePinnedWindow(id);
      return id;
    },
    onSuccess: invalidate,
  });

  const activeNote = useMemo(
    () => notes.find((n) => n.id === activeNoteId) ?? null,
    [notes, activeNoteId],
  );

  const filtered = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return notes;
    return notes.filter(
      (n) => n.title.toLowerCase().includes(q) || n.content.toLowerCase().includes(q),
    );
  }, [notes, searchQuery]);

  const newNote = useCallback(() => createMutation.mutate(), [createMutation]);

  const moveSelection = useCallback(
    (delta: number) => {
      if (filtered.length === 0) return;
      const idx = filtered.findIndex((n) => n.id === activeNoteId);
      const next = filtered[(idx + delta + filtered.length) % filtered.length] ?? filtered[0];
      setActiveNote(next.id);
      requestAnimationFrame(() => editorRef.current?.focus());
    },
    [filtered, activeNoteId, setActiveNote],
  );

  // Backend broadcasts after every mutation so all windows stay in sync.
  useEffect(() => {
    const off = Events.On("notes:changed", invalidate);
    const offNew = Events.On("app:new-note", () => {
      setView("notes");
      newNote();
    });
    return () => {
      off();
      offNew();
    };
  }, [invalidate, newNote, setView]);

  useMainShortcuts({
    newNote,
    trashActive: (id?: string) => {
      const noteId = id ?? useUIStore.getState().activeNoteId;
      if (noteId) trashMutation.mutate(noteId);
    },
    pinActive: (id?: string) => {
      const noteId = id ?? useUIStore.getState().activeNoteId;
      if (!noteId) return;
      const note = queryClient.getQueryData<api.Note[]>(["notes"])?.find((n) => n.id === noteId);
      pinMutation.mutate({ id: noteId, pinned: note ? !note.pinned : true });
    },
    moveSelection,
    editorRef,
    searchRef,
  });

  // Clean up pending autosave timers on unmount.
  useEffect(() => {
    const timers = saveTimers.current;
    return () => timers.forEach((t) => window.clearTimeout(t));
  }, []);

  return (
    <div className="app">
      <div className="titlebar-drag" />
      <div className="toolbar">
        <Segmented
          value={view}
          onChange={(v) => setView(v as "notes" | "trash")}
          options={[
            { label: `笔记 (${notes.length})`, value: "notes" },
            { label: `回收站 (${trash.length})`, value: "trash" },
          ]}
        />
        <Input
          ref={searchRef as never}
          className="search-input"
          placeholder="搜索笔记 (⌘F)"
          prefix={<SearchOutlined />}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          allowClear
          variant="filled"
        />
        <Tooltip title="固定到桌面 (⌘P)">
          <Button
            type="text"
            icon={<PushpinOutlined />}
            disabled={!activeNote}
            className={activeNote?.pinned ? "pin-active" : ""}
            onClick={() => pinMutation.mutate({ id: activeNote!.id, pinned: !activeNote!.pinned })}
            tabIndex={-1}
          />
        </Tooltip>
        <Tooltip title="移入回收站 (⌘⌫)">
          <Button
            type="text"
            icon={<DeleteOutlined />}
            disabled={!activeNote}
            danger
            onClick={() => trashMutation.mutate(activeNote!.id)}
            tabIndex={-1}
          />
        </Tooltip>
        <Tooltip title="快捷键帮助 (⌘⇧/)">
          <Button type="text" icon={<QuestionCircleOutlined />} onClick={() => setShortcutsOpen(true)} tabIndex={-1} />
        </Tooltip>
        <Button type="primary" icon={<PlusOutlined />} onClick={newNote} tabIndex={-1}>
          新建笔记
        </Button>
      </div>

      <div className="content">
        {view === "notes" ? (
          <>
            <aside className="sidebar">
              <NoteList notes={filtered} activeId={activeNoteId} onSelect={setActiveNote} />
            </aside>
            <main className="main">
              {activeNote ? (
                <Editor key={activeNote.id} note={activeNote} onContentChange={scheduleSave} textareaRef={editorRef} />
              ) : (
                <div className="editor-empty">
                  <p className="editor-empty-title">PinNote</p>
                  <p className="editor-empty-hint">
                    选择左侧笔记，或按 <kbd>⌘N</kbd> 新建一条。
                    <br />
                    全局快捷键 <kbd>⌘⌥N</kbd> 可在任何应用中呼出并新建笔记。
                  </p>
                  <p className="editor-empty-hint">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => invalidate()} tabIndex={-1}>
                      刷新
                    </Button>
                  </p>
                </div>
              )}
            </main>
          </>
        ) : (
          <main className="main">
            <TrashView />
          </main>
        )}
      </div>
      <ShortcutsModal />
    </div>
  );
}
