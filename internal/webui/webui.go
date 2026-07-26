// Package webui embeds the frontend's static production build into the
// standalone binary (BACK-09), so `STANDALONE=true financial-tracker`
// serves the whole app — API and UI — from one process on one port with
// no separate frontend server or install step.
//
// dist/ ships with only a placeholder (.gitkeep) in version control: the
// real static build is produced by `make web-build-standalone` (see the
// root README's "Running locally (standalone)" section) and copied in
// immediately before `go build` — go:embed needs the files to exist at
// compile time, so there is no way to embed them without that build
// step happening first. FS() reports which case is present so the
// server can tell a user "no frontend was baked into this binary"
// instead of silently 404ing every page.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded build's root and true, or (nil, false) if only
// the version-controlled placeholder is present — i.e. nobody ran the
// real frontend build before compiling this binary.
func FS() (fs.FS, bool) {
	root, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, false
	}
	return root, true
}
