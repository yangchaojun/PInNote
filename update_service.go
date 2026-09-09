package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	// updateRepository is the GitHub Releases source, spelled exactly like the
	// remote so the API path is never a guess.
	updateRepository = "yangchaojun/PInNote"

	// releasesPage is the manual fallback for builds that cannot replace their
	// own bundle (ADR-0003 D8).
	releasesPage = "https://github.com/" + updateRepository + "/releases/latest"

	// checksumAssetName names the sibling asset holding sha256sum lines.
	checksumAssetName = "SHA256SUMS.txt"

	// Background rhythm (ADR-0003 D2): the first check waits long enough to
	// land after window restore, and every interval carries jitter so a user
	// with several machines does not hit the API at the same minute.
	firstCheckDelay  = 45 * time.Second
	checkInterval    = 12 * time.Hour
	checkJitter      = 15 * time.Minute
	backgroundBudget = 60 * time.Second

	// flushWait is how long the barrier waits for pin windows to confirm their
	// buffers reached the database (ADR-0003 D7).
	flushWait = 3 * time.Second

	settingSkippedVersion = "update.skippedVersion"
	settingLastCheckedAt  = "update.lastCheckedAt"
	settingChannel        = "update.channel"

	// channelBeta opts into prereleases. Read once in Configure: changing
	// tracks mid-flight would finish a download started against another feed.
	channelBeta = "beta"
)

// Tray-visible states, mirroring updater.State. The card owns the progress
// detail; the menu bar only ever needs to answer "is there something for me".
const (
	stateIdle        = "idle"
	stateChecking    = "checking"
	stateUpToDate    = "up-to-date"
	stateAvailable   = "available"
	stateDownloading = "downloading"
	stateVerifying   = "verifying"
	stateInstalling  = "installing"
	stateReady       = "ready"
	stateError       = "error"
)

// UpdateStatus is the read-only view the tray renders.
type UpdateStatus struct {
	State          string `json:"state"`
	CurrentVersion string `json:"currentVersion"`
	Version        string `json:"version,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	CanSelfUpdate  bool   `json:"canSelfUpdate"`
	GuardReason    string `json:"guardReason,omitempty"`
	LastCheckedAt  string `json:"lastCheckedAt,omitempty"`
}

// UpdateService drives self-update: it configures app.Updater, decides when to
// look for a release, keeps the menu bar label honest, and puts the brakes on
// the restart path until every note has reached the database. It is not a Wails
// service on purpose - no renderer calls into it, and the update UI is the
// framework card (ADR-0003 Q3).
type UpdateService struct {
	handle  *appHandle
	db      *sql.DB
	windows *WindowService

	app    *application.App
	ctx    context.Context
	cancel context.CancelFunc
	stop   chan struct{}

	mu            sync.Mutex
	configured    bool
	state         string
	release       *updater.Release
	lastErr       string
	canSelfUpdate bool
	guardReason   string
	inFlight      bool
	onStatus      func(UpdateStatus)
}

// NewUpdateService evaluates the self-update guard once at startup: whether this
// process may replace its own bundle does not change while it runs.
func NewUpdateService(handle *appHandle, db *sql.DB, windows *WindowService) *UpdateService {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("resolve own executable: %v", err)
		exe = ""
	}
	ok, reason := selfUpdateDecision(exe, Version)
	return &UpdateService{
		handle:        handle,
		db:            db,
		windows:       windows,
		state:         stateIdle,
		canSelfUpdate: ok,
		guardReason:   reason,
	}
}

// Configure fills updater.Config. Must run after application.New, whose app
// instance is what app.Updater is attached to, and before anything can check.
func (s *UpdateService) Configure(app *application.App) error {
	s.app = app
	provider, err := ghprovider.New(ghprovider.Config{
		Repository:    updateRepository,
		ChecksumAsset: checksumAssetName,
		AssetMatcher:  macOSAssetMatcher,
		Prerelease:    s.channel() == channelBeta,
	})
	if err != nil {
		return fmt.Errorf("update provider: %w", err)
	}
	if requireSignature && len(updatePublicKey()) == 0 {
		// A release build that can authenticate nothing. Say it once at startup
		// instead of letting every check fail with the same surprise.
		log.Printf("update: no pinned public key, so no release can be verified; see tools/sign-release")
	}
	err = app.Updater.Init(updater.Config{
		CurrentVersion: Version,
		Providers:      []updater.Provider{newSignedReleases(provider)},
		PublicKey:      updatePublicKey(),
		Window: &updater.BuiltinWindow{
			CSS: updateCardCSS,
			Options: updater.WindowOptions{
				Title:       "PinNote 更新",
				AlwaysOnTop: true,
			},
		},
		// CheckInterval stays zero deliberately: the built-in poll runs
		// CheckAndInstall, which pops a window and restarts the app without
		// asking - it would quit on top of unsaved notes (ADR-0003 D2).
	})
	if err != nil {
		return fmt.Errorf("init updater: %w", err)
	}
	s.mu.Lock()
	s.configured = true
	s.mu.Unlock()
	// Skipped versions live in the database, so replay the stored choice after
	// Init reset the Updater in-memory one (ADR-0003 D9).
	skipped, ok, err := getSetting(s.db, settingSkippedVersion)
	if err != nil {
		log.Printf("read skipped version: %v", err)
	} else if ok && skipped != "" {
		app.Updater.SkipVersion(skipped)
	}
	return nil
}

// configured reports whether app.Updater finished initialising, which is what
// separates "no updates" from "no updater".
func (s *UpdateService) updaterReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configured
}

// channel returns the update track, defaulting to stable.
func (s *UpdateService) channel() string {
	value, _, err := getSetting(s.db, settingChannel)
	if err != nil {
		log.Printf("read update channel: %v", err)
	}
	if value == channelBeta {
		return channelBeta
	}
	return "stable"
}

// Start subscribes to the updater event stream and begins background polling.
func (s *UpdateService) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.stop = make(chan struct{})
	s.subscribe()
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

// subscribe mirrors the updater events onto tray-visible state and triggers the
// flush barrier. The Updater dispatches every emit on its own goroutine, so no
// handler here may block.
func (s *UpdateService) subscribe() {
	on := s.handle.on
	on(updater.EventCheckStarted, func(*application.CustomEvent) { s.setState(stateChecking, nil, "") })
	on(updater.EventNoUpdate, func(*application.CustomEvent) {
		s.setState(stateUpToDate, nil, "")
		s.recordCheckedAt()
	})
	on(updater.EventUpdateAvailable, func(e *application.CustomEvent) {
		rel, _ := e.Data.(*updater.Release)
		if rel == nil {
			return
		}
		s.setState(stateAvailable, rel, "")
		s.recordCheckedAt()
	})
	on(updater.EventDownloadStarted, func(*application.CustomEvent) { s.setState(stateDownloading, nil, "") })
	on(updater.EventVerifying, func(*application.CustomEvent) { s.setState(stateVerifying, nil, "") })
	on(updater.EventInstalling, func(*application.CustomEvent) {
		s.setState(stateInstalling, nil, "")
		s.barrier("installing")
	})
	on(updater.EventUpdateReady, func(*application.CustomEvent) {
		s.setState(stateReady, nil, "")
		s.barrier("restart")
	})
	on(updater.EventError, func(e *application.CustomEvent) {
		info, _ := e.Data.(updater.ErrorInfo)
		message := info.Message
		if message == "" {
			message = "更新失败"
		}
		log.Printf("updater error at stage %s: %s", info.Stage, message)
		s.setState(stateError, nil, message)
	})
	// Skip reaches the Updater as an in-memory choice only; persisting it is what
	// makes it survive a relaunch (ADR-0003 D9). Remind later needs no state:
	// the badge is the whole nudge and it never nags on its own.
	on(updater.EventUserSkip, func(*application.CustomEvent) { s.skipCurrentRelease() })
}

// setState records the phase and notifies the tray. A nil release keeps the
// version already tracked, so the menu can still name what is being downloaded.
func (s *UpdateService) setState(state string, rel *updater.Release, message string) {
	s.mu.Lock()
	if rel != nil {
		s.release = rel
	}
	s.state = state
	s.lastErr = message
	status := s.statusLocked()
	callback := s.onStatus
	s.mu.Unlock()
	if callback != nil {
		callback(status)
	}
}

// OnStatusChanged registers the tray refresh callback. Set once at startup.
func (s *UpdateService) OnStatusChanged(callback func(UpdateStatus)) {
	s.mu.Lock()
	s.onStatus = callback
	s.mu.Unlock()
}

// Status reports the current state plus the guard verdict.
func (s *UpdateService) Status() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *UpdateService) statusLocked() UpdateStatus {
	status := UpdateStatus{
		State:          s.state,
		CurrentVersion: Version,
		LastError:      s.lastErr,
		CanSelfUpdate:  s.canSelfUpdate,
		GuardReason:    s.guardReason,
	}
	if s.release != nil && s.state != stateIdle {
		status.Version = s.release.Version
	}
	if checked, ok, err := getSetting(s.db, settingLastCheckedAt); err == nil && ok {
		status.LastCheckedAt = checked
	}
	return status
}

// CheckNow is the manual 检查更新 entry point and the only path allowed to open
// the built-in card. CheckAndInstall goes straight from Check into
// DownloadAndInstall - which is why the signature gate lives in the provider
// and why the flush barrier is driven by the install event instead of the
// restart button (ADR-0003 D6/D7). Staging itself is non-destructive: nothing
// is swapped until the user presses Restart and Apply.
func (s *UpdateService) CheckNow() {
	if !s.updaterReady() {
		log.Printf("check for updates: updater not configured")
		return
	}
	if !s.Status().CanSelfUpdate {
		if err := s.OpenDownloadPage(); err != nil {
			log.Printf("open releases page: %v", err)
		}
		return
	}
	s.mu.Lock()
	if s.inFlight {
		s.mu.Unlock()
		return
	}
	s.inFlight = true
	s.mu.Unlock()

	// The menu callback runs on the UI thread, and a 13 MB download is far too
	// long to hold it.
	go func() {
		defer func() {
			s.mu.Lock()
			s.inFlight = false
			s.mu.Unlock()
		}()
		if err := s.app.Updater.CheckAndInstall(s.ctx); err != nil {
			// The card stays open showing the error and the user dismisses it,
			// so this is log-only (ADR-0003 D11).
			log.Printf("check for updates: %v", err)
		}
	}()
}

// OpenDownloadPage is the fallback when self-update is impossible: the release
// page still carries the .dmg, and GuardReason says why.
func (s *UpdateService) OpenDownloadPage() error {
	if s.app == nil {
		return fmt.Errorf("application not ready")
	}
	if err := s.app.Browser.OpenURL(releasesPage); err != nil {
		return fmt.Errorf("open releases page: %w", err)
	}
	return nil
}

// loop runs the background polls. Only Check is ever called there: it opens no
// window, so a release found in the background surfaces as a menu bar badge and
// nothing more (ADR-0003 D2/D3).
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

func (s *UpdateService) checkInBackground() {
	if !s.updaterReady() {
		return
	}
	// The Updater supports one flow at a time; a manual card that is
	// downloading owns the state machine, so skip this tick.
	switch s.app.Updater.State() {
	case updater.StateIdle, updater.StateUpToDate, updater.StateAvailable, updater.StateError:
	default:
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, backgroundBudget)
	defer cancel()
	if _, err := s.app.Updater.Check(ctx); err != nil {
		// Silent by design: a laptop waking on a train must not turn into a
		// dialogue, and the previous state stays on screen (ADR-0003 D11).
		log.Printf("background update check: %v", err)
	}
}

// barrier lets every pin window reach the database before the updater gets
// anywhere near quitting the app. Because the card restart button is not
// vetoable from Go, this has to run ahead of it: not-clean here means abort the
// whole round (ADR-0003 D7).
func (s *UpdateService) barrier(cause string) {
	if s.windows == nil {
		return
	}
	if err := s.windows.requestFlushAll(flushWait); err != nil {
		log.Printf("flush barrier before %s: %v", cause, err)
		s.abortRound(err)
	}
}

// abortRound closes the card and records why, so a note that could not be saved
// is never sitting in an editor buffer when the swap quits the app. The user
// has to run 检查更新 again once the note is saved.
func (s *UpdateService) abortRound(cause error) {
	s.setState(stateError, nil, "有笔记未能保存，本轮更新已取消："+cause.Error())
	if s.app != nil {
		s.app.Event.Emit(updater.EventUserCancel)
	}
}

// skipCurrentRelease persists the version the card just skipped. The tracked
// version is used rather than Updater.SkippedVersion because our listener runs
// before the Updater's own handler for the same event, so the latter has not
// recorded it yet.
func (s *UpdateService) skipCurrentRelease() {
	s.mu.Lock()
	version := ""
	if s.release != nil {
		version = s.release.Version
	}
	s.mu.Unlock()
	if version == "" {
		return
	}
	if s.app != nil {
		s.app.Updater.SkipVersion(version)
	}
	s.applySkip(version)
}

func (s *UpdateService) applySkip(version string) {
	if err := setSetting(s.db, settingSkippedVersion, version); err != nil {
		log.Printf("persist skipped version: %v", err)
	}
}

// recordCheckedAt stamps the last round trip that reached the network. It is
// the one piece of updater history worth keeping: it answers whether this
// install ever gets an answer from GitHub, without any server of our own.
func (s *UpdateService) recordCheckedAt() {
	stamp := time.Now().UTC().Format(time.RFC3339)
	if err := setSetting(s.db, settingLastCheckedAt, stamp); err != nil {
		log.Printf("persist last checked at: %v", err)
		return
	}
	s.mu.Lock()
	status := s.statusLocked()
	callback := s.onStatus
	s.mu.Unlock()
	if callback != nil {
		callback(status)
	}
}
