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

// The menu bar icon's left click summons whatever this returns.
func TestLatestLiveNote(t *testing.T) {
	s := newTestService(t)

	if n, err := s.latestLiveNote(); err != nil || n != nil {
		t.Fatalf("latestLiveNote on empty db = %v, %v; want nil, nil", n, err)
	}

	older, _ := s.CreateNote("较早的笔记")
	newest, _ := s.CreateNote("最新的笔记")
	// updated_at has second resolution; backdate to make the order explicit.
	if _, err := s.db.Exec(
		`UPDATE notes SET updated_at = updated_at - 60 WHERE id = ?`, older.ID,
	); err != nil {
		t.Fatal(err)
	}

	latest, err := s.latestLiveNote()
	if err != nil {
		t.Fatalf("latestLiveNote: %v", err)
	}
	if latest == nil || latest.ID != newest.ID {
		t.Fatalf("latestLiveNote = %v, want note %s", latest, newest.ID)
	}

	// Trashing the newest note falls back to the next most recent live one.
	if _, err := s.TrashNote(newest.ID); err != nil {
		t.Fatal(err)
	}
	latest, err = s.latestLiveNote()
	if err != nil {
		t.Fatalf("latestLiveNote after trash: %v", err)
	}
	if latest == nil || latest.ID != older.ID {
		t.Fatalf("latestLiveNote = %v, want note %s", latest, older.ID)
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

func TestRestoreLatestNoteWindow(t *testing.T) {
	s := newTestService(t)
	s.CreateNote("旧笔记") // stays closed — startup opens only the newest note
	latest, _ := s.CreateNote("最新笔记")
	trashed, _ := s.CreateNote("被删")
	if _, err := s.TrashNote(trashed.ID); err != nil {
		t.Fatal(err)
	}

	opener := &fakeOpener{}
	RestoreLatestNoteWindow(s, opener)

	// Only the most recently updated live note gets its window back; the
	// older live note and the trashed one stay closed.
	if len(opener.opened) != 1 || opener.opened[0] != latest.ID {
		t.Fatalf("opened = %v, want only the latest live note %s", opener.opened, latest.ID)
	}
}

func TestRestoreLatestNoteWindowNoNotes(t *testing.T) {
	s := newTestService(t)
	opener := &fakeOpener{}
	RestoreLatestNoteWindow(s, opener)
	if len(opener.opened) != 0 {
		t.Fatalf("opened = %v, want none when there are no live notes", opener.opened)
	}
}

func TestEmptyTrash(t *testing.T) {
	s := newTestService(t)
	trashed1, _ := s.CreateNote("甲")
	trashed2, _ := s.CreateNote("乙")
	live, _ := s.CreateNote("丙")
	for _, id := range []string{trashed1.ID, trashed2.ID} {
		if _, err := s.TrashNote(id); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.EmptyTrash(); err != nil {
		t.Fatalf("empty trash: %v", err)
	}
	if got, _ := s.ListTrash(); len(got) != 0 {
		t.Errorf("trash = %v, want empty", got)
	}
	// Only the trash is emptied; live notes stay.
	if list, _ := s.ListNotes(); len(list) != 1 || list[0].ID != live.ID {
		t.Errorf("live list = %v, want only %s", list, live.ID)
	}
}

func TestTrashDaysLeft(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		deletedAt  int64
		daysBehind int64
	}{
		{now, 60},
		{now - 30*86400, 30},
		{now - 59*86400, 1},
		{now - 61*86400, 0},
	}
	for _, c := range cases {
		if got := trashDaysLeft(c.deletedAt); got != c.daysBehind {
			t.Errorf("trashDaysLeft(%d) = %d, want %d", c.deletedAt, got, c.daysBehind)
		}
	}
}

func TestThemePersistence(t *testing.T) {
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Default is dark when nothing has been persisted.
	w := NewWindowService(db)
	w.applyTheme = func(string) {} // no running app in tests
	if w.Theme() != "dark" {
		t.Errorf("default theme = %q, want dark", w.Theme())
	}

	// SetTheme persists and is read back by a fresh service — the Go side
	// is the single source of truth.
	if err := w.SetTheme("light"); err != nil {
		t.Fatalf("set theme: %v", err)
	}
	if w.Theme() != "light" {
		t.Errorf("in-memory theme = %q, want light", w.Theme())
	}
	reopened := NewWindowService(db)
	reopened.applyTheme = func(string) {}
	if reopened.Theme() != "light" {
		t.Errorf("persisted theme = %q, want light", reopened.Theme())
	}

	// Invalid values are rejected and change nothing.
	if err := w.SetTheme("sepia"); err == nil {
		t.Errorf("invalid theme should be rejected")
	}
	if reopened.Theme() != "light" {
		t.Errorf("theme should be unchanged after invalid set")
	}
}
