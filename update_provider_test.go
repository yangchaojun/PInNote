package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	testZipName      = "PinNote-0.2.0-macOS-universal.zip"
	testChecksumName = "SHA256SUMS.txt"
)

func TestSignatureURL(t *testing.T) {
	full := func(filename, url string) *updater.Release {
		return &updater.Release{
			Artifact: updater.Artifact{Filename: filename},
			Metadata: map[string]any{"github.asset.url": url},
		}
	}
	cases := []struct {
		name string
		rel  *updater.Release
		want string
	}{
		{"sibling", full(testZipName, "https://host/o/r/releases/download/v0.2.0/"+testZipName),
			"https://host/o/r/releases/download/v0.2.0/" + testZipName + ".sig"},
		{"no metadata", nil, ""},
		{"url not the asset", full(testZipName, "https://host/o/r/releases/download/v0.2.0/other.zip"),
			""},
	}
	for _, tc := range cases {
		if got := signatureURL(tc.rel); got != tc.want {
			t.Errorf("%s: signatureURL = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMacOSAssetMatcher(t *testing.T) {
	assets := func(names ...string) []ghprovider.ReleaseAsset {
		out := make([]ghprovider.ReleaseAsset, 0, len(names))
		for _, n := range names {
			out = append(out, ghprovider.ReleaseAsset{Name: n})
		}
		return out
	}
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64"}

	cases := []struct {
		name    string
		assets  []ghprovider.ReleaseAsset
		wantIdx int
	}{
		{"universal wins", assets("PinNote-0.2.0-macOS-universal.zip", "PinNote-0.2.0-macOS-arm64.zip"), 0},
		{"dmg is never the artifact", assets("PinNote-0.2.0-macOS-universal.dmg"), -1},
		{"sidecars are skipped", assets("PinNote-0.2.0-macOS-universal.zip.sig", testChecksumName), -1},
		{"arch fallback", assets("PinNote-0.2.0-macOS-arm64.zip"), 0},
		{"other arch is not used", assets("PinNote-0.2.0-macOS-amd64.zip"), -1},
		{"nothing for darwin", assets("PinNote-0.2.0-windows-x64.zip"), -1},
	}
	for _, tc := range cases {
		if got := macOSAssetMatcher(req, tc.assets); got != tc.wantIdx {
			t.Errorf("%s: macOSAssetMatcher = %d, want %d", tc.name, got, tc.wantIdx)
		}
	}
}

// releaseServer stands in for the GitHub Releases API with exactly the assets
// a PinNote release carries.
func releaseServer(t *testing.T, opts map[string]bool) *httptest.Server {
	t.Helper()
	signature := make([]byte, ed25519.SignatureSize)
	for i := range signature {
		signature[i] = byte(i)
	}
	digest := sha256.Sum256([]byte("not really a zip"))
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host + "/download/"
		assets := []map[string]any{
			{"id": 1, "name": testZipName, "size": 13 << 20, "browser_download_url": base + testZipName},
			{"id": 2, "name": "PinNote-0.2.0-macOS-universal.dmg", "size": 14 << 20, "browser_download_url": base + "PinNote-0.2.0-macOS-universal.dmg"},
		}
		if !opts["noChecksums"] {
			assets = append(assets, map[string]any{"id": 3, "name": testChecksumName, "size": 128, "browser_download_url": base + testChecksumName})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.2.0",
			"name":     "PinNote v0.2.0",
			"body":     "notes",
			"assets":   assets,
		})
	})
	mux.HandleFunc("/download/"+testChecksumName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", digest, testZipName)
	})
	mux.HandleFunc("/download/"+testZipName+".sig", func(w http.ResponseWriter, r *http.Request) {
		if opts["noSignature"] {
			http.NotFound(w, r)
			return
		}
		w.Write(signature)
	})
	mux.HandleFunc("/", http.NotFound)
	return httptest.NewServer(mux)
}

func newTestSignedReleases(t *testing.T, serverURL string, requireSignature bool) *signedReleases {
	t.Helper()
	inner, err := ghprovider.New(ghprovider.Config{
		Repository:    "o/r",
		BaseURL:       serverURL,
		ChecksumAsset: testChecksumName,
		AssetMatcher:  macOSAssetMatcher,
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	return &signedReleases{inner: inner, client: http.DefaultClient, requireSignature: requireSignature}
}

func TestSignedReleasesAttachesSignature(t *testing.T) {
	srv := releaseServer(t, nil)
	defer srv.Close()
	p := newTestSignedReleases(t, srv.URL, false)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0", Platform: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if rel == nil {
		t.Fatal("check: no release")
	}
	if rel.Artifact.Filename != testZipName {
		t.Errorf("artifact = %q, want %q", rel.Artifact.Filename, testZipName)
	}
	if rel.Verification == nil {
		t.Fatal("verification missing")
	}
	if len(rel.Verification.Signature) != ed25519.SignatureSize {
		t.Errorf("signature length = %d, want %d", len(rel.Verification.Signature), ed25519.SignatureSize)
	}
	if rel.Verification.SignatureAlgo != "ed25519" {
		t.Errorf("signature algo = %q, want ed25519", rel.Verification.SignatureAlgo)
	}
	wantDigest := sha256.Sum256([]byte("not really a zip"))
	if hex.EncodeToString(rel.Verification.Digest) != hex.EncodeToString(wantDigest[:]) {
		t.Errorf("digest = %x, want the SHA256SUMS entry", rel.Verification.Digest)
	}
}

// The github provider fills a digest and nothing else, so a release without
// SHA256SUMS.txt would reach verification with no digest at all. The wrapper
// has to seed one: pkg/updater only hashes the stream when a Verification
// block exists, and a signature checked against no digest is no check.
func TestSignedReleasesSeedsDigestWhenChecksumsMissing(t *testing.T) {
	srv := releaseServer(t, map[string]bool{"noChecksums": true})
	defer srv.Close()
	p := newTestSignedReleases(t, srv.URL, false)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0", Platform: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if rel.Verification == nil || rel.Verification.DigestAlgo != "sha256" {
		t.Fatalf("verification = %+v, want a seeded sha256 block", rel.Verification)
	}
	if len(rel.Verification.Digest) != 0 {
		t.Errorf("digest = %x, want none", rel.Verification.Digest)
	}
	if len(rel.Verification.Signature) != ed25519.SignatureSize {
		t.Errorf("signature missing")
	}
}

func TestSignedReleasesRefusesUnsignedReleaseInProduction(t *testing.T) {
	srv := releaseServer(t, map[string]bool{"noSignature": true})
	defer srv.Close()
	req := updater.CheckRequest{CurrentVersion: "0.1.0", Platform: "darwin", Arch: "arm64"}

	strict := newTestSignedReleases(t, srv.URL, true)
	_, err := strict.Check(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "refusing an unverifiable update") {
		t.Fatalf("check with requireSignature = %v, want a refusal", err)
	}

	lax := newTestSignedReleases(t, srv.URL, false)
	rel, err := lax.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("check without requireSignature: %v", err)
	}
	if rel.Verification != nil && len(rel.Verification.Signature) != 0 {
		t.Errorf("unexpected signature: %x", rel.Verification.Signature)
	}
}

func TestSignedReleasesUpToDateIsNotAnError(t *testing.T) {
	srv := releaseServer(t, nil)
	defer srv.Close()
	p := newTestSignedReleases(t, srv.URL, true)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.2.0", Platform: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if rel != nil {
		t.Errorf("release = %+v, want nil for an equal version", rel)
	}
}
