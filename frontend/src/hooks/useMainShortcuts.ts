import { useEffect } from "react";
import * as md from "../lib/markdown";
import { useUIStore } from "../store";
import { useThemeStore } from "../theme";

/**
 * Registers the main window's keyboard shortcuts. Returns nothing; mount once
 * at the app root. Formatting shortcuts are forwarded into the focused
 * editor textarea.
 */
export function useMainShortcuts(handlers: {
  newNote: () => void;
  trashActive: (id?: string) => void;
  pinActive: (id?: string) => void;
  moveSelection: (delta: number) => void;
  editorRef: React.RefObject<HTMLTextAreaElement | null>;
  searchRef: React.RefObject<HTMLInputElement | null>;
}) {
  const { newNote, trashActive, pinActive, moveSelection, editorRef, searchRef } = handlers;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      const ui = useUIStore.getState();
      const target = e.target as HTMLElement;
      const inInput = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA";

      // --- Formatting shortcuts: only while typing in the editor ---
      if (mod && inInput && target.tagName === "TEXTAREA") {
        const fmt = FORMAT_KEYS.get(e.key.toLowerCase() + (e.shiftKey ? "+shift" : ""));
        if (fmt) {
          e.preventDefault();
          (editorRef.current as unknown as { __applyFormat?: (fn: unknown) => void })?.__applyFormat?.(fmt);
          return;
        }
      }

      // --- Global (window-level) shortcuts ---
      if (mod && !e.shiftKey && e.key.toLowerCase() === "n") {
        e.preventDefault();
        newNote();
      } else if (mod && !e.shiftKey && e.key.toLowerCase() === "f") {
        e.preventDefault();
        ui.setView("notes");
        searchRef.current?.focus();
        searchRef.current?.select();
      } else if (mod && !e.shiftKey && e.key.toLowerCase() === "e") {
        e.preventDefault();
        ui.togglePreview();
      } else if (mod && e.shiftKey && e.key.toLowerCase() === "t") {
        e.preventDefault();
        ui.setView(ui.view === "trash" ? "notes" : "trash");
      } else if (mod && e.shiftKey && e.key.toLowerCase() === "d") {
        e.preventDefault();
        useThemeStore.getState().toggleTheme();
      } else if (mod && e.shiftKey && e.key === "/") {
        e.preventDefault();
        ui.setShortcutsOpen(!ui.shortcutsOpen);
      } else if (mod && e.key === "Backspace") {
        if (ui.view === "notes" && ui.activeNoteId) {
          e.preventDefault();
          trashActive();
        }
      } else if (mod && !e.shiftKey && e.key.toLowerCase() === "p") {
        if (ui.view === "notes" && ui.activeNoteId) {
          e.preventDefault();
          pinActive();
        }
      } else if (e.altKey && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
        if (ui.view === "notes") {
          e.preventDefault();
          moveSelection(e.key === "ArrowDown" ? 1 : -1);
        }
      } else if (e.key === "Escape") {
        if (ui.shortcutsOpen) ui.setShortcutsOpen(false);
        else if (ui.searchQuery) {
          ui.setSearchQuery("");
          searchRef.current?.blur();
        } else if (document.activeElement === searchRef.current) searchRef.current?.blur();
      }
    };

    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [newNote, trashActive, pinActive, moveSelection, editorRef, searchRef]);
}

const FORMAT_KEYS = new Map<string, md.FormatFn>([
  ["b", md.formatBold],
  ["i", md.formatItalic],
  ["k", md.formatLink],
  ["c+shift", md.formatCheckbox],
  ["7+shift", md.formatOrderedList],
  ["8+shift", md.formatBulletList],
  ["9+shift", md.formatBlockquote],
]);
