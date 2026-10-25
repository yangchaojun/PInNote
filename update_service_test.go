package main

import (
	"strings"
	"testing"
	"time"
)

func newUpdateServiceForTest(t *testing.T) *UpdateService {
	t.Helper()
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewUpdateService(db, nil)
}

// A test binary is a dev build, so the guard must say no and say why — a
// wrong answer here is a broken release path that only shows up on someone
// else's machine.
func TestUpdateGuardDevBuild(t *testing.T) {
	s := newUpdateServiceForTest(t)
	st := s.Status()
	if st.CanSelfUpdate {
		t.Error("dev build must not self-update")
	}
	if st.GuardReason == "" {
		t.Error("a rejected guard must carry its reason")
	}
	if st.State != stateIdle {
		t.Errorf("state = %q, want idle before Start", st.State)
	}

	// The background loop never opens anything and never reaches the
	// network when the guard said no.
	s.checkInBackground()
	if st = s.Status(); st.State != stateIdle {
		t.Errorf("state after guarded check = %q, want idle", st.State)
	}
}

func TestStatusCarriesLastCheckedAt(t *testing.T) {
	s := newUpdateServiceForTest(t)
	stamp := "2026-01-02T15:04:05Z"
	if err := setSetting(s.db, settingLastCheckedAt, stamp); err != nil {
		t.Fatalf("write setting: %v", err)
	}
	if st := s.Status(); st.LastCheckedAt != stamp {
		t.Errorf("LastCheckedAt = %q, want %q", st.LastCheckedAt, stamp)
	}
}

func TestRecordCheckedAtStamps(t *testing.T) {
	s := newUpdateServiceForTest(t)
	s.recordCheckedAt()
	st := s.Status()
	if st.LastCheckedAt == "" {
		t.Fatal("LastCheckedAt empty after recordCheckedAt")
	}
	at, err := time.Parse(time.RFC3339, st.LastCheckedAt)
	if err != nil {
		t.Fatalf("LastCheckedAt not RFC3339: %v", err)
	}
	if time.Since(at) > time.Minute {
		t.Errorf("LastCheckedAt %v is stale", at)
	}
}

func TestGuardReasonMentionsDev(t *testing.T) {
	s := newUpdateServiceForTest(t)
	if !strings.Contains(s.Status().GuardReason, "开发构建") {
		t.Errorf("dev guard reason = %q, want it to name the dev build", s.Status().GuardReason)
	}
}
