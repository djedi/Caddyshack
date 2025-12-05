package templates

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Templates holds the parsed templates for rendering pages.
type Templates struct {
	templatesDir string
	baseTemplates *template.Template // layouts and partials
	pageTemplates map[string]*template.Template // page-specific templates
}

// PageData holds common data passed to all templates.
type PageData struct {
	Title     string
	ActiveNav string
	Data      any
}

// New parses all templates from the given directory and returns a Templates instance.
func New(templatesDir string) (*Templates, error) {
	t := &Templates{
		templatesDir: templatesDir,
		pageTemplates: make(map[string]*template.Template),
	}

	// Parse layouts and partials as the base templates
	t.baseTemplates = template.New("")

	// Parse layouts
	layoutsPattern := filepath.Join(templatesDir, "layouts", "*.html")
	var err error
	t.baseTemplates, err = t.baseTemplates.ParseGlob(layoutsPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layouts: %w", err)
	}

	// Parse partials
	partialsPattern := filepath.Join(templatesDir, "partials", "*.html")
	if files, _ := filepath.Glob(partialsPattern); len(files) > 0 {
		t.baseTemplates, err = t.baseTemplates.ParseGlob(partialsPattern)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partials: %w", err)
		}
	}

	// Parse each page template separately with a clone of the base
	pagesDir := filepath.Join(templatesDir, "pages")
	entries, err := os.ReadDir(pagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		pagePath := filepath.Join(pagesDir, entry.Name())

		// Clone the base templates for this page
		pageTemplate, err := t.baseTemplates.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone base templates for %s: %w", entry.Name(), err)
		}

		// Parse this page template into the clone
		pageTemplate, err = pageTemplate.ParseFiles(pagePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page template %s: %w", entry.Name(), err)
		}

		t.pageTemplates[entry.Name()] = pageTemplate
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
