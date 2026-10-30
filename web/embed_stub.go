//go:build !embedui

package webui

import "io/fs"

// Assets allows Go development and tests before the Web UI has been built.
func Assets() (fs.FS, error) {
	return nil, fs.ErrNotExist
}
