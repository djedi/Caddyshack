package handlers

import (
	"net/http"

	"github.com/dustinredseam/caddyshack/internal/templates"
)

// DashboardData holds data displayed on the dashboard page.
type DashboardData struct {
	SiteCount    int
	SnippetCount int
	CaddyRunning bool
}

// DashboardHandler handles requests for the dashboard page.
type DashboardHandler struct {
	templates *templates.Templates
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(tmpl *templates.Templates) *DashboardHandler {
	return &DashboardHandler{
		templates: tmpl,
	}
}

// ServeHTTP handles GET requests for the dashboard.
func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Placeholder data - will be populated from actual Caddyfile parsing later
	data := templates.PageData{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
		Data: DashboardData{
			SiteCount:    0,
			SnippetCount: 0,
			CaddyRunning: false,
		},
	}

	if err := h.templates.Render(w, "dashboard.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
