package config

import "runtime/debug"

// Version is the build's release tag, set once at startup from main.Version (which the
// shared release script stamps with -ldflags). It lives here so any package can read it
// without importing main, and defaults to "dev" for a plain `go build`.
var Version = "dev"

// BuildString describes the running build for a HUD corner, a bug report or -version.
// The tag alone is not enough to identify a build made outside the release script, so a
// dev build is followed by the commit it came from — and a "+" when the worktree was
// dirty. That commit costs nothing to obtain: the Go toolchain records it in the build
// info, and it survives -trimpath.
func BuildString() string {
	if Version != "dev" {
		return Version
	}
	rev, dirty, ok := vcsInfo()
	if !ok {
		return Version
	}
	s := Version + " " + rev
	if dirty {
		s += "+"
	}
	return s
}

// vcsInfo reports the short commit hash the binary was built from and whether the
// worktree was dirty. Not ok when the binary was built outside a repository.
func vcsInfo() (rev string, dirty, ok bool) {
	info, read := debug.ReadBuildInfo()
	if !read {
		return "", false, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return rev, dirty, rev != ""
}
