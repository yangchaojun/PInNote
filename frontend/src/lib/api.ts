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

export const TRASH_RETENTION_DAYS = 60;

const notes = bindings.NoteService;
const windows = bindings.WindowService;

export function createNote(content: string): Promise<Note> {
  return notes.CreateNote(content) as Promise<Note>;
}

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

export function trashNote(id: string): Promise<Note> {
  return notes.TrashNote(id) as Promise<Note>;
}

export function restoreNote(id: string): Promise<Note> {
  return notes.RestoreNote(id) as Promise<Note>;
}

export function deleteNoteForever(id: string): Promise<void> {
  return notes.DeleteNoteForever(id) as Promise<void>;
}

export function emptyTrash(): Promise<void> {
  return notes.EmptyTrash() as Promise<void>;
}

export function setPinned(id: string, pinned: boolean): Promise<Note> {
  return notes.SetPinned(id, pinned) as Promise<Note>;
}

export function purgeExpiredTrash(): Promise<number> {
  return notes.PurgeExpiredTrash() as Promise<number>;
}

export function openPinnedWindow(id: string): Promise<boolean> {
  return windows.OpenPinnedWindow(id) as Promise<boolean>;
}

export function closePinnedWindow(id: string): Promise<void> {
  return windows.ClosePinnedWindow(id) as Promise<void>;
}

export function focusMainWindow(): Promise<void> {
  return windows.FocusMainWindow() as Promise<void>;
}

/** Days left before a trashed note is permanently purged. */
export function daysUntilPurge(deletedAt: number, now = Date.now()): number {
  const elapsedDays = Math.floor((now - deletedAt * 1000) / 86_400_000);
  return Math.max(0, TRASH_RETENTION_DAYS - elapsedDays);
}
