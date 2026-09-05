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

func TestDiscardIfEmpty(t *testing.T) {
	s := newTestService(t)

	// An empty note is hard-deleted: it never reaches the trash.
	empty, _ := s.CreateNote("")
	deleted, err := s.DiscardIfEmpty(empty.ID)
	if err != nil || !deleted {
		t.Fatalf("discard empty note: deleted=%v err=%v", deleted, err)
	}
	if _, err := s.GetNote(empty.ID); err == nil {
		t.Errorf("note should be gone entirely")
	}
	if trash, _ := s.ListTrash(); len(trash) != 0 {
		t.Errorf("empty note must not enter the trash")
	}

	// A whitespace-only note counts as empty too.
	ws, _ := s.CreateNote("   \n\t\n")
	deleted, err = s.DiscardIfEmpty(ws.ID)
	if err != nil || !deleted {
		t.Fatalf("discard whitespace note: deleted=%v err=%v", deleted, err)
	}

	// A note with content is kept, whatever its content is.
	kept, _ := s.CreateNote("有内容")
	deleted, err = s.DiscardIfEmpty(kept.ID)
	if err != nil || deleted {
		t.Fatalf("non-empty note must survive: deleted=%v err=%v", deleted, err)
	}
	if _, err := s.GetNote(kept.ID); err != nil {
		t.Errorf("content note should survive: %v", err)
	}

	// Unknown ids are a no-op, not an error.
	deleted, err = s.DiscardIfEmpty("nonexistent")
	if err != nil || deleted {
		t.Fatalf("discard nonexistent: deleted=%v err=%v", deleted, err)
	}
}

func TestDeriveTitleEnhanced(t *testing.T) {
	cases := []struct{ content, want string }{
		{"# 标题\n\n正文", "标题"},
		{"- [ ] 买牛奶\n- [x] 已做", "买牛奶"},
		{"- [x] 已完成任务", "已完成任务"},
		{"- [X] 大写 X", "大写 X"},
		{"**加粗**首行", "加粗首行"},
		{"`code` 行", "code 行"},
		{"~~删除线~~ 与 **粗体**", "删除线 与 粗体"},
		{"*斜体* 开头", "斜体 开头"},
		{"> 引用行", "引用行"},
		{"普通首行", "普通首行"},
		{"", "无标题笔记"},
		{"\n \n- [ ] 全空", "全空"},
	}
	for _, c := range cases {
		if got := deriveTitle(c.content); got != c.want {
			t.Errorf("deriveTitle(%q) = %q, want %q", c.content, got, c.want)
		}
	}
}

// fakeOpener records which notes got a restored window.
type fakeOpener struct{ opened []string }

func (f *fakeOpener) OpenPinnedWindow(id string) (bool, error) {
	f.opened = append(f.opened, id)
	return true, nil
}

func TestRestoreAllNoteWindows(t *testing.T) {
	s := newTestService(t)
	live1, _ := s.CreateNote("第一")
	live2, _ := s.CreateNote("第二")
	trashed, _ := s.CreateNote("被删")
	if _, err := s.TrashNote(trashed.ID); err != nil {
		t.Fatal(err)
	}

	opener := &fakeOpener{}
	RestoreAllNoteWindows(s, opener)

	// Every live note gets its window back; trashed notes stay closed.
	if len(opener.opened) != 2 {
		t.Fatalf("opened = %v, want exactly the 2 live notes", opener.opened)
	}
	got := map[string]bool{}
	for _, id := range opener.opened {
		got[id] = true
	}
	if !got[live1.ID] || !got[live2.ID] {
		t.Errorf("opened = %v, want %s and %s", opener.opened, live1.ID, live2.ID)
	}
	if got[trashed.ID] {
		t.Errorf("trashed note must not be restored")
	}
}

func TestThemePersistence(t *testing.T) {
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Default is dark when nothing has been persisted.
	w := NewWindowService(&appHandle{}, db)
	if w.GetTheme() != "dark" {
		t.Errorf("default theme = %q, want dark", w.GetTheme())
	}

	// SetTheme persists (works without a running app) and is read back by a
	// fresh service — the Go side is the single source of truth.
	if err := w.SetTheme("light"); err != nil {
		t.Fatalf("set theme: %v", err)
	}
	if w.Theme() != "light" {
		t.Errorf("in-memory theme = %q, want light", w.Theme())
	}
	reopened := NewWindowService(&appHandle{}, db)
	if reopened.GetTheme() != "light" {
		t.Errorf("persisted theme = %q, want light", reopened.GetTheme())
	}

	// Invalid values are rejected and change nothing.
	if err := w.SetTheme("sepia"); err == nil {
		t.Errorf("invalid theme should be rejected")
	}
	if reopened.Theme() != "light" {
		t.Errorf("theme should be unchanged after invalid set")
	}
}
