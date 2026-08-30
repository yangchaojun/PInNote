import { create } from "zustand";

export type View = "notes" | "trash";

interface UIState {
  /** Currently selected note id in the main window. */
  activeNoteId: string | null;
  searchQuery: string;
  view: View;
  /** Markdown source editor vs rendered preview in the main editor. */
  showPreview: boolean;
  shortcutsOpen: boolean;
  setActiveNote: (id: string | null) => void;
  setSearchQuery: (q: string) => void;
  setView: (v: View) => void;
  togglePreview: () => void;
  setShowPreview: (v: boolean) => void;
  setShortcutsOpen: (v: boolean) => void;
}

export const useUIStore = create<UIState>((set) => ({
  activeNoteId: null,
  searchQuery: "",
  view: "notes",
  showPreview: false,
  shortcutsOpen: false,
  setActiveNote: (id) => set({ activeNoteId: id }),
  setSearchQuery: (q) => set({ searchQuery: q }),
  setView: (v) => set({ view: v, activeNoteId: null }),
  togglePreview: () => set((s) => ({ showPreview: !s.showPreview })),
  setShowPreview: (v) => set({ showPreview: v }),
  setShortcutsOpen: (v) => set({ shortcutsOpen: v }),
}));
