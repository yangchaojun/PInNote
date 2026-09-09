package main

import (
	"context"
	"io"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// stubProvider answers with a fixed release, fresh per call because the
// Updater decorates the one it is given.
type stubProvider struct{ release *updater.Release }

func (p *stubProvider) Name() string { return "stub" }

func (p *stubProvider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	if p.release == nil {
		return nil, nil
	}
	copied := *p.release
	return &copied, nil
}

func (p *stubProvider) Download(context.Context, *updater.Release, io.Writer, func(written, total int64)) error {
	return nil
}

type silentHost struct{ quit int }

func (h *silentHost) Emit(string, ...any) bool         { return false }
func (h *silentHost) OnEvent(string, func(any)) func() { return func() {} }
func (h *silentHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	return nil
}
func (h *silentHost) Quit() { h.quit++ }

func newTestUpdateService(t *testing.T) *UpdateService {
	t.Helper()
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewUpdateService(&appHandle{}, db, nil)
}

// The persisted Skip choice only protects a relaunch if feeding it back into
// Updater.SkipVersion actually suppresses the release. That half belongs to
// the framework, so pin the behaviour rather than assume it.
func TestSkippedVersionMakesCheckReportUpToDate(t *testing.T) {
	u := updater.New(&silentHost{})
	if err := u.Init(updater.Config{
		CurrentVersion: "0.1.0",
		Providers:      []updater.Provider{&stubProvider{release: &updater.Release{Version: "0.2.0"}}},
		Window:         updater.WindowNone,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	u.SkipVersion("0.2.0")
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if rel != nil {
		t.Errorf("check found %+v, want the skipped version suppressed", rel)
	}
	if got := u.State(); got != updater.StateUpToDate {
		t.Errorf("state = %q, want %q", got, updater.StateUpToDate)
	}
}

func TestSkipCurrentReleasePersistsVersion(t *testing.T) {
	s := newTestUpdateService(t)
	s.setState(stateAvailable, &updater.Release{Version: "0.2.0"}, "")
	s.skipCurrentRelease()

	stored, ok, err := getSetting(s.db, settingSkippedVersion)
	if err != nil || !ok {
		t.Fatalf("read skipped version: %v, %v", err, ok)
	}
	if stored != "0.2.0" {
		t.Errorf("skipped version = %q, want 0.2.0", stored)
	}
}

// Without a tracked release there is nothing to skip, and writing an empty
// value would silently clear the user's earlier choice.
func TestSkipCurrentReleaseWithoutReleaseKeepsStoredValue(t *testing.T) {
	s := newTestUpdateService(t)
	if err := setSetting(s.db, settingSkippedVersion, "0.1.5"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.skipCurrentRelease()
	stored, _, err := getSetting(s.db, settingSkippedVersion)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored != "0.1.5" {
		t.Errorf("skipped version = %q, want the previous 0.1.5", stored)
	}
}

func TestStatusTracksPendingRelease(t *testing.T) {
	s := newTestUpdateService(t)
	if got := s.Status().Version; got != "" {
		t.Errorf("initial version = %q, want empty", got)
	}
	s.setState(stateAvailable, &updater.Release{Version: "0.2.0"}, "")
	// The tray still names the version while the download is in flight.
	s.setState(stateDownloading, nil, "")
	st := s.Status()
	if st.Version != "0.2.0" || st.State != stateDownloading {
		t.Errorf("status = %+v, want 0.2.0 downloading", st)
	}
}

func TestRecordCheckedAtPublishesStatus(t *testing.T) {
	s := newTestUpdateService(t)
	var last UpdateStatus
	seen := make(chan UpdateStatus, 1)
	s.OnStatusChanged(func(st UpdateStatus) { seen <- st })
	s.recordCheckedAt()
	select {
	case last = <-seen:
	default:
		t.Fatal("status callback not fired after recording the check")
	}
	if last.LastCheckedAt == "" {
		t.Errorf("last checked at empty")
	}
}
