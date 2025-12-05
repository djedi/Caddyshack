package templates

import (
	"html/template"
	"io"
	"path/filepath"
)

// Templates holds the parsed templates for rendering pages.
type Templates struct {
	templates *template.Template
}

// PageData holds common data passed to all templates.
type PageData struct {
	Title     string
	ActiveNav string
	Data      any
}

// New parses all templates from the given directory and returns a Templates instance.
func New(templatesDir string) (*Templates, error) {
	// Parse all templates together so they can reference each other
	pattern := filepath.Join(templatesDir, "**", "*.html")
	tmpl, err := template.ParseGlob(pattern)
	if err != nil {
		// Try parsing individual directories if glob doesn't match
		tmpl = template.New("")

		// Parse layouts first
		layoutsPattern := filepath.Join(templatesDir, "layouts", "*.html")
		tmpl, err = tmpl.ParseGlob(layoutsPattern)
		if err != nil {
			return nil, err
		}

		// Parse pages
		pagesPattern := filepath.Join(templatesDir, "pages", "*.html")
		tmpl, err = tmpl.ParseGlob(pagesPattern)
		if err != nil && err.Error() != "template: pattern matches no files: "+pagesPattern {
			return nil, err
		}

		// Parse partials
		partialsPattern := filepath.Join(templatesDir, "partials", "*.html")
		tmpl, err = tmpl.ParseGlob(partialsPattern)
		if err != nil && err.Error() != "template: pattern matches no files: "+partialsPattern {
			return nil, err
		}
	}

	return &Templates{templates: tmpl}, nil
}

// Render renders the named template to the writer.
func (t *Templates) Render(w io.Writer, name string, data PageData) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
