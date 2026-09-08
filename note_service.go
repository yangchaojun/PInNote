package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TrashRetentionDays is how long a note stays recoverable in the trash
// before it is purged for good.
const TrashRetentionDays = 60

// ErrNoteNotFound is returned when a note with the given id does not exist.
var ErrNoteNotFound = errors.New("note not found")

// Note is a single markdown note. DeletedAt is a unix timestamp (seconds)
// while the note sits in the trash, and null while the note is live.
type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Pinned    bool   `json:"pinned"`
	DeletedAt *int64 `json:"deletedAt"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// NoteService exposes note persistence to the frontend via Wails bindings.
type NoteService struct {
	app *appHandle
	db  *sql.DB
}

func NewNoteService(db *sql.DB) *NoteService {
	return &NoteService{db: db}
}

// SetApp wires the application handle after application.New so that the
// service can broadcast change events to every window.
func (s *NoteService) SetApp(a *appHandle) { s.app = a }

// deriveTitle returns the first non-empty line of the content, stripped of
// markdown decoration, truncated to a reasonable length, so the pin window's
// header tooltip always has something readable to show.
func deriveTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// Paired marks must go before the block-prefix trim: TrimLeft would
		// otherwise eat one side of a pair ("*斜体*" → "斜体*") and strand it.
		line = stripPairedInlineMarks(line)
		line = stripCheckboxPrefix(line)
		line = strings.TrimLeft(line, "#>-*+[0-9] ")
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 60 {
			line = string(runes[:60]) + "…"
		}
		return line
	}
	return "无标题笔记"
}

// stripCheckboxPrefix removes a GFM task marker ("`- [ ] `", "`- [x] `") from
// the start of a line.
func stripCheckboxPrefix(line string) string {
	for _, prefix := range []string{"- [ ] ", "- [x] ", "- [X] "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return line
}

// stripPairedInlineMarks removes the first pair of each inline markdown
// delimiter (`~~`, `**`, “ ` “, `*`, `_`) from a line, so e.g. a first line
// of "**加粗**首行" titles as "加粗首行". Delimiters are tried longest-first
// so `**` is not consumed as two `*` pairs.
func stripPairedInlineMarks(line string) string {
	for changed := true; changed; {
		changed = false
		for _, mark := range []string{"~~", "**", "`", "*", "_"} {
			i := strings.Index(line, mark)
			if i < 0 {
				continue
			}
			j := strings.Index(line[i+len(mark):], mark)
			if j < 0 {
				continue
			}
			line = line[:i] + line[i+len(mark):i+len(mark)+j] + line[i+2*len(mark)+j:]
			changed = true
		}
	}
	return line
}

func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func scanNote(row interface{ Scan(...any) error }) (Note, error) {
	var n Note
	var pinned int
	err := row.Scan(&n.ID, &n.Title, &n.Content, &pinned, &n.DeletedAt, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return Note{}, err
	}
	n.Pinned = pinned == 1
	return n, nil
}

const noteColumns = "id, title, content, pinned, deleted_at, created_at, updated_at"

// CreateNote inserts a new note with the given markdown content and returns
// it.
func (s *NoteService) CreateNote(content string) (Note, error) {
	now := time.Now().Unix()
	n := Note{
		ID:        newID(),
		Title:     deriveTitle(content),
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.Exec(
		`INSERT INTO notes (id, title, content, pinned, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, 0, NULL, ?, ?)`,
		n.ID, n.Title, n.Content, n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return Note{}, fmt.Errorf("create note: %w", err)
	}
	s.notifyChanged()
	return n, nil
}

// GetNote returns a single note by id, including notes in the trash.
func (s *NoteService) GetNote(id string) (Note, error) {
	n, err := scanNote(s.db.QueryRow(`SELECT `+noteColumns+` FROM notes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNoteNotFound
	}
	return n, err
}

// ListNotes returns all live (non-deleted) notes, pinned notes first, most
// recently updated within each group.
func (s *NoteService) ListNotes() ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT ` + noteColumns + ` FROM notes
		 WHERE deleted_at IS NULL
		 ORDER BY pinned DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

// latestLiveNote returns the most recently updated live note, or nil when
// there are none — the menu bar icon's "get back to my notes" lookup. Kept
// unexported so the bound service API stays the frontend's only read path.
func (s *NoteService) latestLiveNote() (*Note, error) {
	n, err := scanNote(s.db.QueryRow(
		`SELECT ` + noteColumns + ` FROM notes
		 WHERE deleted_at IS NULL
		 ORDER BY updated_at DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest note: %w", err)
	}
	return &n, nil
}

// ListTrash returns all notes currently in the trash, most recently deleted
// first.
func (s *NoteService) ListTrash() ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT ` + noteColumns + ` FROM notes
		 WHERE deleted_at IS NOT NULL
		 ORDER BY deleted_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}
	defer rows.Close()
	return collectNotes(rows)
}

func collectNotes(rows *sql.Rows) ([]Note, error) {
	// Non-nil slice so it marshals to [] instead of null on the frontend.
	notes := make([]Note, 0, 16)
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// UpdateNote replaces the content (and derived title) of a note. Notes in the
// trash cannot be edited.
func (s *NoteService) UpdateNote(id string, content string) (Note, error) {
	res, err := s.db.Exec(
		`UPDATE notes SET content = ?, title = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		content, deriveTitle(content), time.Now().Unix(), id,
	)
	if err != nil {
		return Note{}, fmt.Errorf("update note: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Note{}, ErrNoteNotFound
	}
	s.notifyChanged()
	return s.GetNote(id)
}

// DiscardIfEmpty hard-deletes a note whose current content is blank. This is
// the pin-only form factor's empty-note rule: closing a window (or the exit
// path) flushes the editor first, then calls this — a never-typed note is
// gone immediately instead of living on in the trash. Returns true when the
// note was deleted.
func (s *NoteService) DiscardIfEmpty(id string) (bool, error) {
	n, err := s.GetNote(id)
	if errors.Is(err, ErrNoteNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load note for discard: %w", err)
	}
	if strings.TrimSpace(n.Content) != "" {
		return false, nil
	}
	res, err := s.db.Exec(`DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("discard empty note: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return false, nil
	}
	s.notifyChanged()
	return true, nil
}

// TrashNote moves a note to the trash (recoverable for 60 days) and un-pins
// it.
func (s *NoteService) TrashNote(id string) (Note, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`UPDATE notes SET deleted_at = ?, pinned = 0, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		now, now, id,
	)
	if err != nil {
		return Note{}, fmt.Errorf("trash note: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Note{}, ErrNoteNotFound
	}
	s.notifyChanged()
	return s.GetNote(id)
}

// RestoreNote moves a note out of the trash back into the live list.
//
// FROZEN (pin-only form factor): the trash has no UI in this form factor;
// kept for data compatibility. New code must not call it.
func (s *NoteService) RestoreNote(id string) (Note, error) {
	res, err := s.db.Exec(
		`UPDATE notes SET deleted_at = NULL, updated_at = ?
		 WHERE id = ? AND deleted_at IS NOT NULL`,
		time.Now().Unix(), id,
	)
	if err != nil {
		return Note{}, fmt.Errorf("restore note: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Note{}, ErrNoteNotFound
	}
	s.notifyChanged()
	return s.GetNote(id)
}

// DeleteNoteForever removes a note from the database permanently. Only notes
// already in the trash can be deleted forever.
//
// FROZEN (pin-only form factor): same as RestoreNote — no trash UI exists.
func (s *NoteService) DeleteNoteForever(id string) error {
	res, err := s.db.Exec(`DELETE FROM notes WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("delete note forever: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ErrNoteNotFound
	}
	s.notifyChanged()
	return nil
}

// EmptyTrash permanently deletes every note in the trash.
//
// FROZEN (pin-only form factor): same as RestoreNote — no trash UI exists.
func (s *NoteService) EmptyTrash() error {
	if _, err := s.db.Exec(`DELETE FROM notes WHERE deleted_at IS NOT NULL`); err != nil {
		return fmt.Errorf("empty trash: %w", err)
	}
	s.notifyChanged()
	return nil
}

// SetPinned pins or unpins a note.
//
// FROZEN (pin-only form factor, ADR-0001): the pinned/unpinned distinction is
// retired — every live note always has a window. Kept only so old clients and
// existing data keep working; new code must not call it.
func (s *NoteService) SetPinned(id string, pinned bool) (Note, error) {
	p := 0
	if pinned {
		p = 1
	}
	res, err := s.db.Exec(
		`UPDATE notes SET pinned = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		p, time.Now().Unix(), id,
	)
	if err != nil {
		return Note{}, fmt.Errorf("set pinned: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Note{}, ErrNoteNotFound
	}
	s.notifyChanged()
	return s.GetNote(id)
}

// PurgeExpiredTrash permanently deletes notes that have been in the trash for
// longer than TrashRetentionDays. It runs automatically at startup and
// returns the number of purged notes.
func (s *NoteService) PurgeExpiredTrash() (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -TrashRetentionDays).Unix()
	res, err := s.db.Exec(`DELETE FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge expired trash: %w", err)
	}
	count, _ := res.RowsAffected()
	if count > 0 {
		s.notifyChanged()
	}
	return count, nil
}

// TrashRetentionDays exposes the retention window to the frontend.
//
// FROZEN (pin-only form factor): the purge countdown UI is gone; the constant
// itself still drives the startup purge.
func (s *NoteService) GetTrashRetentionDays() int64 { return TrashRetentionDays }

// notifyChanged broadcasts a lightweight event so every window (main + pinned)
// refreshes its queries.
func (s *NoteService) notifyChanged() {
	if s.app != nil {
		s.app.emit("notes:changed", nil)
	}
}
