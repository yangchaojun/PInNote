package main

import (
	"testing"
	"time"
)

func newTestService(t *testing.T) *NoteService {
	t.Helper()
	// Reuse openDB's schema against an in-memory database.
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewNoteService(db)
}

func TestCreateAndListNotes(t *testing.T) {
	s := newTestService(t)
	n, err := s.CreateNote("# 会议记录\n\n讨论了排期")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if n.Title != "会议记录" {
		t.Errorf("derived title = %q, want 会议记录", n.Title)
	}
	list, err := s.ListNotes()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
}

func TestEmptyContentTitle(t *testing.T) {
	s := newTestService(t)
	n, _ := s.CreateNote("")
	if n.Title != "无标题笔记" {
		t.Errorf("empty title = %q", n.Title)
	}
}

func TestTrashRestoreFlow(t *testing.T) {
	s := newTestService(t)
	n, _ := s.CreateNote("待办：买牛奶")

	if _, err := s.TrashNote(n.ID); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if list, _ := s.ListNotes(); len(list) != 0 {
		t.Errorf("live list should be empty after trashing")
	}
	trash, _ := s.ListTrash()
	if len(trash) != 1 || trash[0].ID != n.ID {
		t.Fatalf("trash = %v", trash)
	}
	if _, err := s.RestoreNote(n.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if list, _ := s.ListNotes(); len(list) != 1 {
		t.Errorf("note should be restored")
	}
	if got, _ := s.ListTrash(); len(got) != 0 {
		t.Errorf("trash should be empty after restore")
	}
}

func TestPurgeExpiredTrash(t *testing.T) {
	s := newTestService(t)
	fresh, _ := s.CreateNote("新的")
	old, _ := s.CreateNote("旧的")

	now := time.Now().Unix()
	if _, err := s.TrashNote(fresh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TrashNote(old.ID); err != nil {
		t.Fatal(err)
	}
	// Backdate one note beyond the 60-day retention window.
	if _, err := s.db.Exec(
		`UPDATE notes SET deleted_at = ? WHERE id = ?`,
		now-(TrashRetentionDays+5)*86400, old.ID,
	); err != nil {
		t.Fatal(err)
	}

	purged, err := s.PurgeExpiredTrash()
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}
	if _, err := s.GetNote(old.ID); err == nil {
		t.Errorf("expired note should be gone")
	}
	if _, err := s.GetNote(fresh.ID); err != nil {
		t.Errorf("recent note should survive: %v", err)
	}
}

func TestPinnedUpdateAndTrashInteraction(t *testing.T) {
	s := newTestService(t)
	n, _ := s.CreateNote("内容")
	if _, err := s.SetPinned(n.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetNote(n.ID)
	if !got.Pinned {
		t.Errorf("note should be pinned")
	}
	// Trashing must unpin.
	if _, err := s.TrashNote(n.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetNote(n.ID)
	if got.Pinned {
		t.Errorf("trashed note should be unpinned")
	}
	// Trash note content cannot be edited.
	if _, err := s.UpdateNote(n.ID, "x"); err == nil {
		t.Errorf("editing a trashed note should fail")
	}
}
