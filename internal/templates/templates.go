package templates

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Templates holds the parsed templates for rendering pages.
type Templates struct {
	baseTemplates *template.Template             // layouts and partials
	pageTemplates map[string]*template.Template // page-specific templates
}

// PageData holds common data passed to all templates.
type PageData struct {
	Title       string
	ActiveNav   string
	Data        any
	Permissions any // User permissions for UI rendering (middleware.UserPermissions)
}

// New parses all templates from the given directory and returns a Templates instance.
// This loads templates from the filesystem.
func New(templatesDir string) (*Templates, error) {
	return newFromDirFS(os.DirFS(templatesDir))
}

// NewFromFS parses all templates from an embedded filesystem and returns a Templates instance.
func NewFromFS(fsys fs.FS) (*Templates, error) {
	return newFromDirFS(fsys)
}

// templateFuncs provides custom functions for templates.
var templateFuncs = template.FuncMap{
	// dict creates a map from key-value pairs for passing to templates
	"dict": func(values ...any) map[string]any {
		if len(values)%2 != 0 {
			return nil
		}
		dict := make(map[string]any, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil
			}
			dict[key] = values[i+1]
		}
		return dict
	},
	// hasPrefix checks if a string has the given prefix
	"hasPrefix": func(s, prefix string) bool {
		return strings.HasPrefix(s, prefix)
	},
	// sub subtracts b from a
	"sub": func(a, b int) int {
		return a - b
	},
	// add adds a and b
	"add": func(a, b int) int {
		return a + b
	},
	// upper converts the first character of a string to uppercase
	"upper": func(s string) string {
		return strings.ToUpper(s)
	},
	// slice extracts a substring from a string
	"slice": func(s string, start, end int) string {
		if start < 0 {
			start = 0
		}
		if end > len(s) {
			end = len(s)
		}
		if start >= end || start >= len(s) {
			return ""
		}
		return s[start:end]
	},
}

// newFromDirFS parses all templates from a filesystem (either os.DirFS or embed.FS).
func newFromDirFS(fsys fs.FS) (*Templates, error) {
	t := &Templates{
		pageTemplates: make(map[string]*template.Template),
	}

	// Parse layouts and partials as the base templates with custom functions
	t.baseTemplates = template.New("").Funcs(templateFuncs)

	// Parse layouts
	layoutFiles, err := fs.Glob(fsys, "layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob layouts: %w", err)
	}
	if len(layoutFiles) == 0 {
		return nil, fmt.Errorf("no layout templates found")
	}
	t.baseTemplates, err = t.baseTemplates.ParseFS(fsys, layoutFiles...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layouts: %w", err)
	}

	// Parse partials
	partialFiles, err := fs.Glob(fsys, "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob partials: %w", err)
	}
	if len(partialFiles) > 0 {
		t.baseTemplates, err = t.baseTemplates.ParseFS(fsys, partialFiles...)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partials: %w", err)
		}
	}

	// Parse each page template separately with a clone of the base
	pageFiles, err := fs.Glob(fsys, "pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob pages: %w", err)
	}

	for _, pagePath := range pageFiles {
		pageName := filepath.Base(pagePath)
		if !strings.HasSuffix(pageName, ".html") {
			continue
		}

		// Clone the base templates for this page
		pageTemplate, err := t.baseTemplates.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone base templates for %s: %w", pageName, err)
		}

		// Parse this page template into the clone
		pageTemplate, err = pageTemplate.ParseFS(fsys, pagePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page template %s: %w", pageName, err)
		}

		t.pageTemplates[pageName] = pageTemplate
	}

	return t, nil
}

// Render renders the named template to the writer.
func (t *Templates) Render(w io.Writer, name string, data PageData) error {
	pageTemplate, ok := t.pageTemplates[name]
	if !ok {
		return fmt.Errorf("template not found: %s", name)
	}
	return pageTemplate.ExecuteTemplate(w, name, data)
}

// RenderPartial renders a partial template (from partials directory) to the writer.
// This is useful for HTMX responses that only need to return a fragment.
func (t *Templates) RenderPartial(w io.Writer, name string, data any) error {
	return t.baseTemplates.ExecuteTemplate(w, name, data)
}
