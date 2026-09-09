package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	// signatureSuffix names the sidecar holding the detached ed25519
	// signature of an artifact sha256 digest.
	signatureSuffix = ".sig"

	// maxSignatureBytes bounds the sidecar read. An ed25519 signature is
	// exactly 64 bytes, so anything bigger is a wrong URL or a hostile answer.
	maxSignatureBytes = 512
)

// errNoSignature reports that a release simply has no sidecar, which is a
// different thing from the sidecar being unreachable or malformed: the first
// is a publishing gap, the second must never be downgraded to an install.
var errNoSignature = errors.New("no signature sidecar")

// signedReleases wraps the GitHub provider and fills the one verification
// field it cannot reach: a signature. providers/github reads SHA256SUMS.txt
// and populates a digest, which proves the bytes arrived intact but not that
// they came from us - the checksum file sits in the same release as the
// artifact and is exactly as editable as the artifact is. Fetching
// <artifact>.sig and checking it against the compile-time key turns an
// attacker who can rewrite the release into an attacker who must also steal
// the signing key (ADR-0003 D5).
type signedReleases struct {
	inner  *ghprovider.Provider
	client *http.Client

	// requireSignature mirrors the build-tag constant rather than reading it
	// inline, so both branches are reachable from a non-production test binary.
	requireSignature bool
}

func newSignedReleases(inner *ghprovider.Provider) *signedReleases {
	return &signedReleases{
		inner:            inner,
		client:           &http.Client{Timeout: 30 * time.Second},
		requireSignature: requireSignature,
	}
}

func (p *signedReleases) Name() string { return p.inner.Name() }

// Check decorates the release the GitHub provider resolved. Refusing a
// release that cannot be authenticated happens here rather than in the
// service because CheckAndInstall walks straight from Check into
// DownloadAndInstall: this is the last point where nothing has been
// downloaded yet.
func (p *signedReleases) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.inner.Check(ctx, req)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, nil
	}
	sig, err := p.fetchSignature(ctx, rel)
	if errors.Is(err, errNoSignature) {
		if p.requireSignature {
			return nil, fmt.Errorf("release %s ships no %s%s: refusing an unverifiable update",
				rel.Version, rel.Artifact.Filename, signatureSuffix)
		}
		return rel, nil
	}
	if err != nil {
		return nil, err
	}
	if rel.Verification == nil {
		// The Updater only hashes the stream while downloading when a
		// Verification block exists (pkg/updater/download.go), and signature
		// verification is performed over that streaming digest. Seed one so
		// a release without SHA256SUMS.txt is still hashed and authenticated
		// rather than checked against a stale digest.
		rel.Verification = &updater.Verification{}
	}
	if rel.Verification.DigestAlgo == "" {
		rel.Verification.DigestAlgo = "sha256"
	}
	rel.Verification.Signature = sig
	rel.Verification.SignatureAlgo = "ed25519"
	return rel, nil
}

func (p *signedReleases) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return p.inner.Download(ctx, rel, dst, onProgress)
}

// fetchSignature reads the sidecar that sits next to the artifact in the same
// release. The asset URL is a browser download URL whose last path segment is
// the filename, so the sibling is that URL with the suffix appended.
func (p *signedReleases) fetchSignature(ctx context.Context, rel *updater.Release) ([]byte, error) {
	url := signatureURL(rel)
	if url == "" {
		return nil, errNoSignature
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("updater: signature request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: signature request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNoSignature
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("updater: signature sidecar: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureBytes))
	if err != nil {
		return nil, fmt.Errorf("updater: read signature sidecar: %w", err)
	}
	if len(body) != ed25519.SignatureSize {
		return nil, fmt.Errorf("updater: signature sidecar is %d bytes, want %d", len(body), ed25519.SignatureSize)
	}
	return body, nil
}

// signatureURL returns the sibling .sig URL for a release produced by the
// GitHub provider, or "" when the release cannot be located by filename.
func signatureURL(rel *updater.Release) string {
	if rel == nil || rel.Metadata == nil || rel.Artifact.Filename == "" {
		return ""
	}
	downloadURL, _ := rel.Metadata["github.asset.url"].(string)
	if downloadURL == "" || !strings.HasSuffix(downloadURL, "/"+rel.Artifact.Filename) {
		return ""
	}
	return downloadURL + signatureSuffix
}

// macOSAssetMatcher chooses the artifact for this machine. The framework
// DefaultAssetMatcher matches platform plus arch substrings, which fails on
// every Intel Mac because we publish one universal artifact (ADR-0003 Q2), and
// it would happily pick the .dmg - a disk image the updater cannot unpack into
// a bundle, so the swap would rename a .dmg over an .app. Zips only,
// universal first, then an arch-specific zip.
func macOSAssetMatcher(req updater.CheckRequest, assets []ghprovider.ReleaseAsset) int {
	for i, a := range assets {
		if isZip(a.Name) && strings.Contains(strings.ToLower(a.Name), "universal") {
			return i
		}
	}
	if req.Arch != "" {
		for i, a := range assets {
			if isZip(a.Name) && strings.Contains(strings.ToLower(a.Name), strings.ToLower(req.Arch)) {
				return i
			}
		}
	}
	return -1
}

func isZip(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".zip")
}
