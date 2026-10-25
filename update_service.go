package main

// Self-update on mygo.Updater (the port of the Wails-era stack, ADR-0003):
// a background loop only ever Checks — a release found there surfaces as a
// tray badge and nothing more (D2/D3). The manual 检查更新 entry flushes every
// editor buffer first, then opens the framework's update card (the native
// updater plugin), which owns download, verification and restart. Editors
// live in this process, so OnWillQuit's synchronous FlushAll is the barrier
// that no restart can jump over (D7's intent, stronger than its protocol).

import (
	"context"
	"database/sql"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	updaterui "github.com/egoist/mygo/plugins/updater"
)

const (
	// updateRepository is the GitHub Releases source, spelled exactly like
	// the remote so the API path is never a guess (mygo.json carries the
	// same value for the feed).
	updateRepository = "yangchaojun/PInNote"

	// releasesPage is the manual fallback for builds that cannot replace
	// their own bundle (ADR-0003 D8).
	releasesPage = "https://github.com/" + updateRepository + "/releases/latest"

	// Background rhythm (ADR-0003 D2): the first check waits long enough to
	// land after window restore, and every interval carries jitter so a user
	// with several machines does not hit the API at the same minute.
	firstCheckDelay  = 45 * time.Second
	checkInterval    = 12 * time.Hour
	checkJitter      = 15 * time.Minute
	backgroundBudget = 60 * time.Second

	settingLastCheckedAt = "update.lastCheckedAt"
)

// Tray-visible states. The card owns the progress detail (downloading,
// verifying, installing), which its own window shows; the menu bar only ever
// needs to answer "is there something for me".
const (
	stateIdle      = "idle"
	stateChecking  = "checking"
	stateUpToDate  = "up-to-date"
	stateAvailable = "available"
	stateError     = "error"
)

// UpdateStatus is the read-only view the tray renders.
type UpdateStatus struct {
	State          string
	CurrentVersion string
	Version        string
	LastError      string
	CanSelfUpdate  bool
	GuardReason    string
	LastCheckedAt  string
}

// UpdateService decides when to look for a release, keeps the menu bar
// label honest, and flushes every buffer before the card can restart the
// app.
type UpdateService struct {
	db      *sql.DB
	windows *WindowService

	ctx    context.Context
	cancel context.CancelFunc
	stop   chan struct{}

	mu            sync.Mutex
	state         string
	version       string
	lastErr       string
	canSelfUpdate bool
	guardReason   string
	inFlight      bool
	onStatus      func(UpdateStatus)
}

// NewUpdateService evaluates the self-update guard once at startup: whether
// this process may replace its own bundle does not change while it runs.
func NewUpdateService(db *sql.DB, windows *WindowService) *UpdateService {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("resolve own executable: %v", err)
		exe = ""
	}
	ok, reason := selfUpdateDecision(exe, appVersion())
	if ok && !mygo.Updater.Enabled() {
		// The framework's own verdict (dev build, package-manager install,
		// no update feed in the build) wins over ours.
		ok = false
		reason = "此构建未携带更新通道（mygo build 未配置或为开发构建）"
	}
	return &UpdateService{
		db:            db,
		windows:       windows,
		state:         stateIdle,
		canSelfUpdate: ok,
		guardReason:   reason,
	}
}

// Start begins the background poll loop.
func (s *UpdateService) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.stop = make(chan struct{})
	go s.loop()
}

// Stop ends the poll loop and cancels in-flight network work.
func (s *UpdateService) Stop() {
	if s.stop != nil {
		close(s.stop)
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *UpdateService) loop() {
	timer := time.NewTimer(firstCheckDelay)
	defer timer.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-timer.C:
			s.checkInBackground()
			timer.Reset(checkInterval + jitterDelay())
		}
	}
}

func jitterDelay() time.Duration {
	return time.Duration(rand.Int63n(int64(checkJitter) + 1))
}

// checkInBackground looks for a release without opening anything: a hit
// surfaces as a menu bar badge (ADR-0003 D2/D3). Silent on failure — a
// laptop waking on a train must not turn into a dialogue (D11).
func (s *UpdateService) checkInBackground() {
	if !s.CanSelfUpdate() {
		return
	}
	s.mu.Lock()
	if s.inFlight || s.state == stateChecking {
		s.mu.Unlock()
		return
	}
	s.inFlight = true
	s.state = stateChecking
	s.mu.Unlock()
	s.publish()

	ctx, cancel := context.WithTimeout(s.ctx, backgroundBudget)
	up, err := mygo.Updater.Check(ctx)
	cancel()

	s.mu.Lock()
	s.inFlight = false
	switch {
	case err != nil:
		log.Printf("background update check: %v", err)
		s.state = stateError
		s.lastErr = "检查更新失败"
	case up == nil:
		s.state = stateUpToDate
		s.version = ""
	default:
		s.state = stateAvailable
		s.version = up.Version
	}
	s.mu.Unlock()
	s.recordCheckedAt()
}

// CheckNow is the manual 检查更新 entry point: flush every buffer (the
// barrier the card's restart cannot be denied, applied before anything can
// download), then open the framework card. Builds that cannot self-update
// get the honest fallback: the release page (ADR-0003 D8).
func (s *UpdateService) CheckNow() {
	if !s.CanSelfUpdate() {
		if err := mygo.Shell.OpenExternal(releasesPage); err != nil {
			log.Printf("open releases page: %v", err)
		}
		return
	}
	s.windows.FlushAll()
	updaterui.CheckForUpdates()
}

// recordCheckedAt stamps the last round trip that reached the network — the
// one piece of updater history worth keeping.
func (s *UpdateService) recordCheckedAt() {
	if err := setSetting(s.db, settingLastCheckedAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Printf("persist last checked at: %v", err)
	}
	s.publish()
}

// OnStatusChanged registers the tray refresh callback. Set once at startup.
func (s *UpdateService) OnStatusChanged(callback func(UpdateStatus)) {
	s.mu.Lock()
	s.onStatus = callback
	s.mu.Unlock()
}

func (s *UpdateService) publish() {
	status := s.Status()
	s.mu.Lock()
	callback := s.onStatus
	s.mu.Unlock()
	if callback != nil {
		mygo.RunOnMain(func() { callback(status) })
	}
}

// CanSelfUpdate reports the startup guard verdict.
func (s *UpdateService) CanSelfUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canSelfUpdate
}

// Status reports the current state plus the guard verdict.
func (s *UpdateService) Status() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := UpdateStatus{
		State:          s.state,
		CurrentVersion: appVersion(),
		CanSelfUpdate:  s.canSelfUpdate,
		GuardReason:    s.guardReason,
	}
	if s.state != stateIdle {
		status.Version = s.version
	}
	status.LastError = s.lastErr
	if checked, ok, err := getSetting(s.db, settingLastCheckedAt); err == nil && ok {
		status.LastCheckedAt = checked
	}
	return status
}
