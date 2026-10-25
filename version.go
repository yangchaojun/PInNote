package main

import "github.com/egoist/mygo"

// appVersion is the version of this build. `mygo build` links the config's
// version into the app; anything else (go build, go run, mygo dev) means
// "not a release", which the self-update guard treats as dev.
func appVersion() string {
	switch v := mygo.App.Version(); v {
	case "", "dev", "(devel)":
		return "dev"
	default:
		return v
	}
}
