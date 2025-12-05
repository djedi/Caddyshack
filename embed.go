package caddyshack

import (
	"embed"
	"io/fs"
)

//go:embed static
var embeddedStatic embed.FS

// StaticFS returns the embedded static filesystem.
// The returned fs.FS is rooted at the "static" directory.
func StaticFS() fs.FS {
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic("failed to create sub filesystem for static files: " + err.Error())
	}
	return sub
}
