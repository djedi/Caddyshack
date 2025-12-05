// Package static provides utilities for serving static files.
package static

import (
	"io/fs"
	"net/http"
	"os"
)

// Handler returns an http.Handler that serves static files from /static/.
// If embeddedFS is not nil, it serves from the embedded filesystem.
// Otherwise, it serves from the provided directory path.
func Handler(embeddedFS fs.FS, dir string) http.Handler {
	var filesystem fs.FS
	if embeddedFS != nil {
		filesystem = embeddedFS
	} else {
		filesystem = os.DirFS(dir)
	}

	return http.StripPrefix("/static/", http.FileServer(http.FS(filesystem)))
}
