package handlers

import (
	"errors"
	"net/http"

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
