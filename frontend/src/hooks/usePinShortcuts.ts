import { useEffect } from "react";
import type { Editor } from "@tiptap/core";
import { toggleTheme } from "../theme";

/**
 * Pin-window keyboard shortcuts (spec §4.2). ⌘B / ⌘I are TipTap's own Bold /
 * Italic keymaps and are deliberately not repeated here (a window-level
 * handler would double-toggle them). Everything else lives here:
 *
 *   ⌘K      toggle link on the selection
 *   ⌘⇧D     switch light/dark theme (Go SetTheme broadcasts to every window)
 *   ⌘⌫      delete the note (to trash) and close the window
 *   ⌘⇧/     toggle the shortcuts help modal
 *   Esc     close the help modal (and nothing else)
 */
export function usePinShortcuts(handlers: {
  getEditor: () => Editor | null;
  onDelete: () => void;
  onToggleHelp: () => void;
  helpOpen: boolean;
  onCloseHelp: () => void;
}) {
  const { getEditor, onDelete, onToggleHelp, helpOpen, onCloseHelp } = handlers;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;

      if (e.key === "Escape") {
        if (helpOpen) {
          e.preventDefault();
          onCloseHelp();
        }
        return;
      }

      if (!mod) return;

      if (e.shiftKey && (e.key === "/" || e.key === "?")) {
        e.preventDefault();
        onToggleHelp();
        return;
      }
      if (e.shiftKey && e.key.toLowerCase() === "d") {
        e.preventDefault();
        void toggleTheme();
        return;
      }
      if (e.key === "Backspace" && !e.shiftKey) {
        e.preventDefault();
        onDelete();
        return;
      }
      if (!e.shiftKey && e.key.toLowerCase() === "k") {
        const ed = getEditor();
        if (!ed) return;
        e.preventDefault();
        if (ed.isActive("link")) {
          ed.chain().focus().unsetLink().run();
          return;
        }
        // WKWebView implements the JS prompt via Wails' UI delegate.
        const url = window.prompt("链接地址", "https://");
        if (url) {
          ed.chain().focus().extendMarkRange("link").setLink({ href: url }).run();
        }
      }
    };

    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [getEditor, onDelete, onToggleHelp, helpOpen, onCloseHelp]);
}
