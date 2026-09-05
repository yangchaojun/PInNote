import * as bindings from "../../bindings/pinnote";

export interface Note {
  id: string;
  title: string;
  content: string;
  pinned: boolean;
  deletedAt: number | null;
  createdAt: number;
  updatedAt: number;
}

const notes = bindings.NoteService;
const windows = bindings.WindowService;

export function getNote(id: string): Promise<Note> {
  return notes.GetNote(id) as Promise<Note>;
}

// A Go nil slice marshals to JSON null; coalesce to [] so callers can rely on
// array invariants.
export function listNotes(): Promise<Note[]> {
  return (notes.ListNotes() as Promise<Note[] | null>).then((r) => r ?? []);
}

export function listTrash(): Promise<Note[]> {
  return (notes.ListTrash() as Promise<Note[] | null>).then((r) => r ?? []);
}

export function updateNote(id: string, content: string): Promise<Note> {
  return notes.UpdateNote(id, content) as Promise<Note>;
}

// Hard-delete when the note's content is blank (pin-only form factor's
// empty-note rule); returns true when the note was deleted.
export function discardIfEmpty(id: string): Promise<boolean> {
  return notes.DiscardIfEmpty(id) as Promise<boolean>;
}

export function trashNote(id: string): Promise<Note> {
  return notes.TrashNote(id) as Promise<Note>;
}

export function openPinnedWindow(id: string): Promise<boolean> {
  return windows.OpenPinnedWindow(id) as Promise<boolean>;
}

export function closePinnedWindow(id: string): Promise<void> {
  return windows.ClosePinnedWindow(id) as Promise<void>;
}

export function getTheme(): Promise<"light" | "dark"> {
  return windows.GetTheme() as Promise<"light" | "dark">;
}

export function setTheme(mode: "light" | "dark"): Promise<void> {
  return windows.SetTheme(mode) as Promise<void>;
}
