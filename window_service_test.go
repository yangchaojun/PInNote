package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func newBarrierService(t *testing.T, pins ...string) *WindowService {
	t.Helper()
	db, err := openDBAt(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewWindowService(&appHandle{}, db)
	s.listPins = func() []string { return pins }
	s.broadcast = func(string, any) {}
	return s
}

// ack delivers a window answer the way the event bus would: JSON in, decoded
// map out, sender taken from the window that emitted it. The token is echoed
// from the request, exactly as PinWindow.tsx does.
func ack(token, noteID string, dirty bool) *application.CustomEvent {
	payload, _ := json.Marshal(map[string]any{"token": token, "noteID": noteID, "dirty": dirty})
	var data map[string]any
	_ = json.Unmarshal(payload, &data)
	return &application.CustomEvent{Sender: pinnedWindowPrefix + noteID, Data: data}
}

// answering replays the request token back for the given notes, in broadcast
// order, so a test can describe which windows answered how.
func answering(s *WindowService, dirtyFor string) func(string, any) {
	return func(name string, data any) {
		if name != flushTopic {
			return
		}
		token := data.(flushRequest).Token
		for _, id := range s.listPins() {
			s.receiveFlushAck(ack(token, id, id == dirtyFor))
		}
	}
}

func TestRequestFlushAllSucceedsWhenEveryWindowIsClean(t *testing.T) {
	s := newBarrierService(t, "n1", "n2")
	var mu sync.Mutex
	tokens := 0
	s.broadcast = func(name string, data any) {
		mu.Lock()
		tokens++
		token := data.(flushRequest).Token
		mu.Unlock()
		if name != flushTopic {
			t.Errorf("broadcast %q, want %q", name, flushTopic)
		}
		if token == "" {
			t.Error("broadcast an empty round token")
		}
		for _, id := range []string{"n1", "n2"} {
			s.receiveFlushAck(ack(token, id, false))
		}
	}
	if err := s.requestFlushAll(time.Second); err != nil {
		t.Fatalf("flush all = %v, want nil", err)
	}
	if tokens != 1 {
		t.Errorf("broadcast %d times, want 1", tokens)
	}
}

func TestRequestFlushAllReportsDirtyWindow(t *testing.T) {
	s := newBarrierService(t, "n1", "n2")
	s.broadcast = answering(s, "n2")
	err := s.requestFlushAll(time.Second)
	if err == nil || !strings.Contains(err.Error(), "n2") {
		t.Fatalf("flush all = %v, want an error naming n2", err)
	}
}

// A window that never answers must not be read as consent: the ack is what the
// barrier waits for, and its absence is the case where an editor still holds
// text the database has not seen.
func TestRequestFlushAllTimesOutOnSilence(t *testing.T) {
	s := newBarrierService(t, "n1")
	err := s.requestFlushAll(30 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "1 pin window") {
		t.Fatalf("flush all = %v, want a timeout naming one window", err)
	}
}

func TestRequestFlushAllWithoutWindowsIsClean(t *testing.T) {
	s := newBarrierService(t)
	called := false
	s.broadcast = func(string, any) { called = true }
	if err := s.requestFlushAll(time.Second); err != nil {
		t.Fatalf("flush all = %v, want nil", err)
	}
	if called {
		t.Error("broadcast with no windows to answer")
	}
}

func TestRequestFlushAllRejectsConcurrentRounds(t *testing.T) {
	s := newBarrierService(t, "n1")
	s.flushing = &flushRound{token: "other", waiters: map[string]chan bool{}}
	err := s.requestFlushAll(30 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("flush all = %v, want a busy error", err)
	}
}

// An ack from a superseded round, or for a note that was never asked, must not
// be mistaken for consent.
func TestFlushAcksIgnoreForeignAndStaleNotes(t *testing.T) {
	s := newBarrierService(t, "n1")
	s.broadcast = func(name string, data any) {
		token := data.(flushRequest).Token
		s.receiveFlushAck(ack(token, "other", true))
		s.receiveFlushAck(ack(token, "n1", false))
	}
	if err := s.requestFlushAll(time.Second); err != nil {
		t.Fatalf("flush all = %v, want nil", err)
	}
}

func TestDecodeFlushAckTrustsTheSender(t *testing.T) {
	spoofed := &application.CustomEvent{
		Sender: pinnedWindowPrefix + "real",
		Data:   map[string]any{"token": "t", "noteID": "someone-elses-note", "dirty": true},
	}
	if got := decodeFlushAck(spoofed); got.noteID != "real" || !got.dirty || got.token != "t" {
		t.Errorf("decodeFlushAck = %+v, want (t, real, true)", got)
	}
	payloadOnly := &application.CustomEvent{Data: map[string]any{"token": "t", "noteID": "n9"}}
	if got := decodeFlushAck(payloadOnly); got.noteID != "n9" || got.dirty {
		t.Errorf("decodeFlushAck = %+v, want (t, n9, false)", got)
	}
	if got := decodeFlushAck(&application.CustomEvent{Data: "not an object"}); got.noteID != "" || got.token != "" {
		t.Errorf("decodeFlushAck on junk = %+v, want empties", got)
	}
}

// An answer from a round that already gave up must not be spent on the round
// that replaced it: the note may have been edited again in between.
func TestFlushAckFromStaleRoundIsIgnored(t *testing.T) {
	s := newBarrierService(t, "n1")
	s.broadcast = func(name string, data any) {
		s.receiveFlushAck(ack("token-from-an-earlier-round", "n1", false))
	}
	if err := s.requestFlushAll(40 * time.Millisecond); err == nil {
		t.Fatal("flush all succeeded on a stale ack")
	}
	if got := decodeFlushAck(&application.CustomEvent{Data: map[string]any{"noteID": "n1"}}); got.token != "" {
		t.Errorf("token = %q, want empty for an ack without one", got.token)
	}
}
