package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// SitesData holds data displayed on the sites list page.
type SitesData struct {
	Sites    []caddy.Site
	Error    string
	HasError bool
}

// SiteDetailData holds data displayed on the site detail page.
type SiteDetailData struct {
	Site     SiteView
	Error    string
	HasError bool
}

// SiteFormData holds data for the site add/edit form.
type SiteFormData struct {
	Site     *SiteFormValues // nil for new site, populated for edit
	Error    string
	HasError bool
}

// SiteFormValues represents the form field values for creating/editing a site.
type SiteFormValues struct {
	Domain       string
	Type         string // "reverse_proxy", "static", "redirect"
	Target       string // for reverse_proxy
	RootPath     string // for static
	RedirectUrl  string // for redirect
	RedirectCode string // for redirect (301, 302, etc.)
	EnableTls    bool
}

// SiteView is a view model for a single site with helper fields.
type SiteView struct {
	caddy.Site
	PrimaryAddress string // First address for display/linking
	FormattedBlock string // Formatted raw block for display
}

// SitesHandler handles requests for the sites pages.
type SitesHandler struct {
	templates *templates.Templates
	config    *config.Config
}

// NewSitesHandler creates a new SitesHandler.
func NewSitesHandler(tmpl *templates.Templates, cfg *config.Config) *SitesHandler {
	return &SitesHandler{
		templates: tmpl,
		config:    cfg,
	}
}

// List handles GET requests for the sites list page.
func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	data := SitesData{}

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
		// Parse sites from the Caddyfile
		parser := caddy.NewParser(content)
		sites, err := parser.ParseSites()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			data.Sites = sites
		}
	}

	pageData := templates.PageData{
		Title:     "Sites",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "sites.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Detail handles GET requests for the site detail page.
func (h *SitesHandler) Detail(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path (e.g., /sites/example.com)
	path := r.URL.Path
	domain := strings.TrimPrefix(path, "/sites/")
	domain = strings.TrimSuffix(domain, "/")

	if domain == "" {
		http.Redirect(w, r, "/sites", http.StatusFound)
		return
	}

	data := SiteDetailData{}

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
		// Parse sites from the Caddyfile
		parser := caddy.NewParser(content)
		sites, err := parser.ParseSites()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			// Find the site matching the domain
			var found *caddy.Site
			for i := range sites {
				for _, addr := range sites[i].Addresses {
					if addr == domain {
						found = &sites[i]
						break
					}
				}
				if found != nil {
					break
				}
			}

			if found == nil {
				data.Error = "Site not found: " + domain
				data.HasError = true
			} else {
				data.Site = SiteView{
					Site:           *found,
					PrimaryAddress: found.Addresses[0],
					FormattedBlock: formatRawBlock(found.RawBlock),
				}
			}
		}
	}

	pageData := templates.PageData{
		Title:     domain + " - Site Details",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-detail.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// formatRawBlock formats a raw block string for display.
// It adds proper indentation for readability.
func formatRawBlock(raw string) string {
	if raw == "" {
		return ""
	}

	// The raw block is stored as space-separated tokens.
	// Convert it into a more readable multi-line format.
	var result strings.Builder
	depth := 0
	tokens := strings.Fields(raw)

	for i, token := range tokens {
		if token == "}" {
			depth--
			if depth < 0 {
				depth = 0
			}
			result.WriteString(strings.Repeat("    ", depth))
			result.WriteString("}\n")
		} else if token == "{" {
			result.WriteString("{\n")
			depth++
		} else {
			// Check if next token is "{" for inline brace
			if i+1 < len(tokens) && tokens[i+1] == "{" {
				result.WriteString(strings.Repeat("    ", depth))
				result.WriteString(token + " ")
			} else {
				result.WriteString(strings.Repeat("    ", depth))
				result.WriteString(token + "\n")
			}
		}
	}

	return strings.TrimSpace(result.String())
}

// New handles GET requests for the new site form page.
func (h *SitesHandler) New(w http.ResponseWriter, r *http.Request) {
	data := SiteFormData{
		Site: nil, // nil indicates new site
	}

	pageData := templates.PageData{
		Title:     "Add Site",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-new.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
