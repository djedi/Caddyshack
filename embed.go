package caddyshack

import (
	"embed"
	"io/fs"
)

//go:embed static
var embeddedStatic embed.FS

//go:embed templates
var embeddedTemplates embed.FS

// StaticFS returns the embedded static filesystem.
// The returned fs.FS is rooted at the "static" directory.
func StaticFS() fs.FS {
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic("failed to create sub filesystem for static files: " + err.Error())
	}
	return sub
}

// TemplatesFS returns the embedded templates filesystem.
// The returned fs.FS is rooted at the "templates" directory.
func TemplatesFS() fs.FS {
	sub, err := fs.Sub(embeddedTemplates, "templates")
	if err != nil {
		panic("failed to create sub filesystem for templates: " + err.Error())
	}
	return sub
}
