// Package web embeds the built frontend so the server ships as a single
// binary. Run `npm run build` in apps/web before building the server.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend, or nil when only the placeholder is
// present (frontend not built yet).
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := sub.Open("index.html"); err != nil {
		return nil
	}
	return sub
}
