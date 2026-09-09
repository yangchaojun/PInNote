package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// devVersion is Version when nothing was injected at build time, and the
// guard answer for "this binary is not a release".
const devVersion = "dev"

// repositoryWalkLimit bounds the ancestor walk; a bundle is never deeper
// than a handful of directories below a checkout.
const repositoryWalkLimit = 12

// bundlePath returns the .app bundle enclosing exePath, or "" when the binary
// does not live in one. It mirrors updater.bundleTarget so the guard and the
// swap always agree on what would be replaced.
func bundlePath(exePath string) string {
	parts := strings.Split(filepath.Clean(exePath), string(os.PathSeparator))
	for i, part := range parts {
		if strings.HasSuffix(part, ".app") {
			return string(os.PathSeparator) + filepath.Join(parts[1:i+1]...)
		}
	}
	return ""
}

// selfUpdateDecision is the guard of ADR-0003 D8: replacing our own bundle is
// allowed only for an installed app whose bundle directory we can actually
// write. When it says no, the menu item degrades to opening the download
// page, and reason is the sentence shown for that.
func selfUpdateDecision(exePath, version string) (bool, string) {
	if version == "" || version == devVersion {
		return false, "开发构建不支持自动更新"
	}
	bundle := bundlePath(exePath)
	if bundle == "" {
		return false, "当前不是从 .app 运行，无法替换自身"
	}
	if strings.HasSuffix(bundle, ".dev.app") {
		return false, "开发 bundle 不支持自动更新"
	}
	if insideRepository(bundle) {
		return false, "仓库工作区内的构建不支持自动更新"
	}
	parent := filepath.Dir(bundle)
	if !dirWritable(parent) {
		return false, fmt.Sprintf("%s 不可写，无法完成换包", parent)
	}
	return true, ""
}

// insideRepository reports whether any ancestor of path is a checkout. A
// `wails3 build` product sitting in bin/ must never overwrite repository
// state, and PinNote.dev.app is exactly that case.
func insideRepository(path string) bool {
	dir := filepath.Dir(path)
	for i := 0; i < repositoryWalkLimit; i++ {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
	return false
}

// dirWritable probes with a real file rather than access(2): as root, or on a
// read-only mounted DMG, the permission bits alone lie about whether a rename
// can succeed next to the bundle.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".pinnote-write-test")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
