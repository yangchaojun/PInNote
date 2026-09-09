//go:build !production

package main

// requireSignature is false in non-production builds so the update flow can
// be exercised against a locally published, unsigned release. Release builds
// go through `-tags production` (build/darwin/Taskfile.yml) and get the
// fail-closed constant in update_verify_prod.go.
const requireSignature = false
