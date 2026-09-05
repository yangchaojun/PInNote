import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import type { Editor } from "@tiptap/core";
import { Placeholder } from "@tiptap/extensions";
import { createExtensions } from "./lib/editorConfig";
import { guardSaveRoundTrip, loadMarkdownLossless } from "./lib/audit";
import type { AuditContext } from "./lib/audit";
import { updateNote } from "./lib/api";

export const AUTOSAVE_DELAY_MS = 400;

export interface PinEditorHandle {
  /**
   * Saves the current doc immediately (bypassing the debounce), through the
   * same round-trip guard as the debounced path. The close/delete flows call
   * this before touching the note; window blur and beforeunload use it as
   * their flush.
   */
  flush: () => Promise<void>;
  /** True while there is content that a flush would still persist. */
  isDirty: () => boolean;
  /** The mounted TipTap editor, for shortcut handlers (⌘K link toggling). */
  getEditor: () => Editor | null;
}

interface PinEditorProps {
  noteId: string;
  /** Markdown fact source at window-open time; loaded once through the audit. */
  initialContent: string;
  /** Called after a successful save so the window can refresh its queries. */
  onSaved: () => void;
}

function auditOf(editor: Editor): AuditContext {
  if (!editor.markdown) throw new Error("no markdown manager on editor");
  return { manager: editor.markdown, schema: editor.schema };
}

/**
 * The whole-window WYSIWYG editor of a pin note. Markdown is the fact source:
 * opening runs note.content through the lossless audit loader, and every save
 * must pass guardSaveRoundTrip (parse(serialize(doc)) ≡ doc) before it is
 * written — a guard failure refuses the write instead of risking content.
 */
export const PinEditor = forwardRef<PinEditorHandle, PinEditorProps>(function PinEditor(
  { noteId, initialContent, onSaved },
  ref,
) {
  const saveTimer = useRef<number | undefined>(undefined);
  const dirty = useRef(false);
  const onSavedRef = useRef(onSaved);
  onSavedRef.current = onSaved;

  // RawSource "转为可编辑": re-run the SAME audit on the stored source and
  // splice the audited nodes back in. Inserting the raw markdown directly
  // would bypass the audit — unsafe sources simply re-demote; data survives.
  const convertRawSource = useCallback((text: string, pos: number) => {
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

  const editor = useEditor({
    extensions: [
      ...createExtensions({ onConvert: convertRawSource }),
      Placeholder.configure({ placeholder: "记点什么…" }),
    ],
    content: "",
    // Copies of editor content carry the markdown fact source, not the
    // plain-text join.
    editorProps: {
      clipboardTextSerializer: (slice) => {
        const ed = editorRef.current;
        if (!ed?.markdown) return slice.content.textBetween(0, slice.content.size, "\n");
        return ed.markdown.serialize({ type: "doc", content: slice.content.toJSON() } as never);
      },
    },
    onUpdate: () => {
      dirty.current = true;
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => {
        void flushRef.current();
      }, AUTOSAVE_DELAY_MS);
    },
  });
  const editorRef = useRef<Editor | null>(null);
  editorRef.current = editor;

  /** The single save path: serialize → invariant guard → UpdateNote. */
  const flush = useCallback(async () => {
    const ed = editorRef.current;
    if (!ed) return;
    window.clearTimeout(saveTimer.current);
    saveTimer.current = undefined;
    if (!dirty.current) return;
    let guard;
    try {
      guard = guardSaveRoundTrip(ed.state.doc, auditOf(ed));
    } catch (err) {
      console.error("pin editor save guard threw; write refused", err);
      return;
    }
    if (!guard.ok) {
      // 拒写并保留降级态：the doc (with its rawSource blocks) stays in the
      // editor untouched; never persist a serialization we cannot reopen.
      console.error("pin editor save refused:", guard.error);
      return;
    }
    try {
      await updateNote(noteId, guard.markdown);
      dirty.current = false;
      onSavedRef.current();
    } catch (err) {
      console.error("pin editor save failed", err);
    }
  }, [noteId]);
  const flushRef = useRef(flush);
  flushRef.current = flush;

  useImperativeHandle(ref, () => ({
    flush,
    isDirty: () => dirty.current,
    getEditor: () => editorRef.current,
  }), [flush]);

  // 三层 flush: window blur persists immediately; beforeunload is the ⌘Q
  // net (best effort — the runtime may be gone before the write lands).
  useEffect(() => {
    const flushNow = () => {
      void flushRef.current().catch(() => undefined);
    };
    window.addEventListener("blur", flushNow);
    window.addEventListener("beforeunload", flushNow);
    return () => {
      window.removeEventListener("blur", flushNow);
      window.removeEventListener("beforeunload", flushNow);
    };
  }, []);

  // Initial open: load the fact source through the audit loader, then put the
  // caret where writing starts. initialContent is read once (via ref): later
  // note refetches are usually echoes of our own saves and must not reload
  // the doc under the user's cursor.
  const initialContentRef = useRef(initialContent);
  initialContentRef.current = initialContent;
  useEffect(() => {
    const ed = editorRef.current;
    if (!ed) return;
    const { doc } = loadMarkdownLossless(initialContentRef.current, auditOf(ed));
    ed.commands.command(({ tr }) => {
      tr.replaceWith(0, ed.state.doc.content.size, doc.content);
      return true;
    });
    ed.commands.focus("end");
    // Loading and focusing dispatch transactions, so onUpdate fires and marks
    // the editor dirty — but opening a note is not an edit and must never
    // produce a write.
    window.clearTimeout(saveTimer.current);
    dirty.current = false;
    return () => {
      window.clearTimeout(saveTimer.current);
    };
  }, [editor]);

  if (!editor) return null;
  return <EditorContent editor={editor} className="pin-editor" />;
});
