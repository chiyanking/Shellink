//go:build embedui

package webui

import (
	"embed"
	"io/fs"
)

// Include Vite's underscore-prefixed shared chunks as well as other assets.
//
//go:embed all:dist
var embeddedUI embed.FS

// Assets returns the built Web UI without requiring files on disk at runtime.
func Assets() (fs.FS, error) {
	return fs.Sub(embeddedUI, "dist")
}
