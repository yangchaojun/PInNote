package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundlePath(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"applications", "/Applications/PinNote.app/Contents/MacOS/PinNote", "/Applications/PinNote.app"},
		{"user folder", "/Users/me/Library/Application Support/PinNote.app/Contents/MacOS/PinNote", "/Users/me/Library/Application Support/PinNote.app"},
		{"trailing slash", "/Applications/PinNote.app/Contents/MacOS/PinNote/", "/Applications/PinNote.app"},
		{"bare binary", "/Users/me/projects/pin-note/bin/PinNote", ""},
	}
	for _, tc := range cases {
		if got := bundlePath(tc.in); got != tc.want {
			t.Errorf("%s: bundlePath(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// The guard decides between a real self-update and 打开下载页, so every
// rejection needs to be attributable: a wrong answer here is a broken
// release path that only shows up on someone else's machine.
func TestSelfUpdateDecision(t *testing.T) {
	root := t.TempDir()

	installed := filepath.Join(root, "Applications", "PinNote.app", "Contents", "MacOS", "PinNote")
	mkdirTree(t, installed)

	inRepo := filepath.Join(root, "repo", "bin", "PinNote.app", "Contents", "MacOS", "PinNote")
	mkdirTree(t, inRepo)
	if err := os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755); err != nil {
		t.Fatalf("create .git: %v", err)
	}

	devBundle := filepath.Join(root, "bin", "PinNote.dev.app", "Contents", "MacOS", "PinNote")
	mkdirTree(t, devBundle)

	readonly := filepath.Join(root, "Volumes", "PinNote", "PinNote.app", "Contents", "MacOS", "PinNote")
	mkdirTree(t, readonly)
	// The bundle's parent is where the helper renames through, so that is the
	// directory that has to accept a write.
	parent := filepath.Dir(bundlePath(readonly))
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	cases := []struct {
		name, exe, version string
		want               bool
		reasonContains     string
	}{
		{"installed", installed, "0.2.0", true, ""},
		{"dev version", installed, "dev", false, "开发构建"},
		{"empty version", installed, "", false, "开发构建"},
		{"bare binary", filepath.Join(root, "bin", "PinNote"), "0.2.0", false, ".app"},
		{"dev bundle", devBundle, "0.2.0", false, "开发 bundle"},
		{"inside repository", inRepo, "0.2.0", false, "仓库"},
		{"read-only parent", readonly, "0.2.0", false, "不可写"},
	}
	for _, tc := range cases {
		ok, reason := selfUpdateDecision(tc.exe, tc.version)
		if ok != tc.want {
			t.Errorf("%s: selfUpdateDecision = %v (%q), want %v", tc.name, ok, reason, tc.want)
		}
		if tc.reasonContains != "" && !strings.Contains(reason, tc.reasonContains) {
			t.Errorf("%s: reason %q does not mention %q", tc.name, reason, tc.reasonContains)
		}
		if ok && reason != "" {
			t.Errorf("%s: accepted but reason set: %q", tc.name, reason)
		}
	}
}

func mkdirTree(t *testing.T, file string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(file), err)
	}
}
