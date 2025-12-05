package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// SnippetsData holds data displayed on the snippets list page.
type SnippetsData struct {
	Snippets       []SnippetView
	Error          string
	HasError       bool
	SuccessMessage string
	ReloadError    string
}

// SnippetView is a view model for a single snippet with helper fields.
type SnippetView struct {
	caddy.Snippet
	Preview    string   // First few lines of content for display
	UsageCount int      // Number of sites using this snippet
	UsedBySites []string // Names of sites using this snippet
}

// SnippetsHandler handles requests for the snippets pages.
type SnippetsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	adminClient  *caddy.AdminClient
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewSnippetsHandler creates a new SnippetsHandler.
func NewSnippetsHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *SnippetsHandler {
	return &SnippetsHandler{
		templates:    tmpl,
		config:       cfg,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the snippets list page.
func (h *SnippetsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := SnippetsData{}

	// Check for success or reload error messages from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}
	if reloadErr := r.URL.Query().Get("reload_error"); reloadErr != "" {
		data.ReloadError = reloadErr
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if errors.Is(err, caddy.ErrCaddyfileNotFound) {
			data.Error = "Caddyfile not found at " + h.config.CaddyfilePath
		} else {
			data.Error = "Failed to read Caddyfile: " + err.Error()
		}
		data.HasError = true
	} else {
		// Parse snippets and sites from the Caddyfile
		parser := caddy.NewParser(content)
		snippets, err := parser.ParseSnippets()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			// Get sites to determine snippet usage
			sites, _ := parser.ParseSites()

			// Build snippet views with usage info
			for _, snippet := range snippets {
				view := SnippetView{
					Snippet: snippet,
					Preview: getSnippetPreview(snippet),
				}

				// Count usage across sites
				for _, site := range sites {
					for _, imp := range site.Imports {
						if imp == snippet.Name {
							view.UsageCount++
							if len(site.Addresses) > 0 {
								view.UsedBySites = append(view.UsedBySites, site.Addresses[0])
							}
							break
						}
					}
				}

				data.Snippets = append(data.Snippets, view)
			}
		}
	}

	pageData := templates.PageData{
		Title:     "Snippets",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippets.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// getSnippetPreview returns the first few lines of a snippet's content for display.
func getSnippetPreview(snippet caddy.Snippet) string {
	if len(snippet.Directives) == 0 {
		return "(empty)"
	}

	var lines []string
	maxLines := 3

	for i, directive := range snippet.Directives {
		if i >= maxLines {
			break
		}

		line := directive.Name
		if len(directive.Args) > 0 {
			line += " " + strings.Join(directive.Args, " ")
		}

		// Truncate long lines
		if len(line) > 50 {
			line = line[:47] + "..."
		}

		lines = append(lines, line)
	}

	preview := strings.Join(lines, "\n")
	if len(snippet.Directives) > maxLines {
		preview += "\n..."
	}

	return preview
}

// Detail handles GET requests for the snippet detail page.
func (h *SnippetsHandler) Detail(w http.ResponseWriter, r *http.Request) {
	// Extract snippet name from URL path (e.g., /snippets/site_log)
	path := r.URL.Path
	name := strings.TrimPrefix(path, "/snippets/")
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		http.Redirect(w, r, "/snippets", http.StatusFound)
		return
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Parse snippets and sites
	parser := caddy.NewParser(content)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	sites, _ := parser.ParseSites()

	// Find the snippet matching the name
	var found *caddy.Snippet
	for i := range snippets {
		if snippets[i].Name == name {
			found = &snippets[i]
			break
		}
	}

	if found == nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Build view with usage info
	view := SnippetView{
		Snippet: *found,
		Preview: getSnippetPreview(*found),
	}

	// Find sites using this snippet
	for _, site := range sites {
		for _, imp := range site.Imports {
			if imp == found.Name {
				view.UsageCount++
				if len(site.Addresses) > 0 {
					view.UsedBySites = append(view.UsedBySites, site.Addresses[0])
				}
				break
			}
		}
	}

	// Format the raw block for display
	formattedContent := formatSnippetContent(found)

	type SnippetDetailData struct {
		Snippet          SnippetView
		FormattedContent string
		Error            string
		HasError         bool
	}

	data := SnippetDetailData{
		Snippet:          view,
		FormattedContent: formattedContent,
	}

	pageData := templates.PageData{
		Title:     name + " - Snippet Details",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-detail.html", pageData); err != nil {
		log.Printf("Error rendering snippet detail template: %v", err)
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// formatSnippetContent formats a snippet's content for display.
func formatSnippetContent(snippet *caddy.Snippet) string {
	if snippet == nil {
		return ""
	}

	writer := caddy.NewWriter()
	return writer.WriteSnippet(snippet)
}
