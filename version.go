package main

// Version is the version of this build. Release packaging injects it with
// `-ldflags -X main.Version=<number>` (build/darwin/Taskfile.yml), fed from
// the same value CI writes into Info.plist, so the binary and the bundle can
// never disagree about which release they are (ADR-0003 B3).
//
// The default is also a sentinel: "dev" means not a release, which disables
// self-update (ADR-0003 D8).
var Version = "dev"
