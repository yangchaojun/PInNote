//go:build production

package main

// requireSignature makes an authenticated release mandatory before the
// updater writes any bytes to disk.
//
// PinNote ships ad-hoc signed and unsigned by Apple (ADR-0003 Q1), and the
// self-update download does not set the quarantine xattr, so Gatekeeper never
// looks at the replaced bundle. The pinned ed25519 key is therefore the only
// thing standing between a compromised release feed and code execution on
// every users machine - an update path that tolerated unsigned releases
// would be indistinguishable from no verification at all.
const requireSignature = true
