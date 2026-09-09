package main

import (
	"crypto/ed25519"
	"encoding/hex"
)

// updatePublicKeyHex is the trust root for release signatures: the
// hex-encoded ed25519 public key printed by `go run ./tools/sign-release
// keygen`. The private half never enters this repository; it lives in the
// PINNOTE_UPDATE_KEY repository secret.
//
// Deliberately empty until a key is generated. Every production build then
// refuses every release instead of installing bytes it cannot authenticate,
// so shipping auto-update without a key fails loudly rather than silently.
const updatePublicKeyHex = "243aedecf8a27c3577c9ae7b1f269e84bfd7b4625c01ee233d7772ab5e5e3e45"

// updatePublicKey decodes the pinned key, returning nil when it is absent or
// malformed (which updater.Config treats as "no trust root configured").
func updatePublicKey() []byte {
	raw, err := hex.DecodeString(updatePublicKeyHex)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil
	}
	return raw
}
