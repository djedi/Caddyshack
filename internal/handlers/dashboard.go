package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// DashboardData holds data displayed on the dashboard page.
type DashboardData struct {
	SiteCount    int
	SnippetCount int
	CaddyStatus  *caddy.CaddyStatus
}

// DashboardHandler handles requests for the dashboard page.
type DashboardHandler struct {
	templates   *templates.Templates
	adminClient *caddy.AdminClient
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(tmpl *templates.Templates, cfg *config.Config) *DashboardHandler {
	return &DashboardHandler{
		templates:   tmpl,
		adminClient: caddy.NewAdminClient(cfg.CaddyAdminAPI),
	}
}

// ServeHTTP handles GET requests for the dashboard.
func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Get Caddy status
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, _ := h.adminClient.GetStatus(ctx)

	data := templates.PageData{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
		Data: DashboardData{
			SiteCount:    0,
			SnippetCount: 0,
			CaddyStatus:  status,
		},
	}

	if err := h.templates.Render(w, "dashboard.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Status handles GET requests for just the status widget (for HTMX polling).
func (h *DashboardHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, _ := h.adminClient.GetStatus(ctx)

	// Wrap status in a struct with Data field to match template expectations
	data := struct {
		Data *caddy.CaddyStatus
	}{
		Data: status,
	}

	if err := h.templates.RenderPartial(w, "status-widget.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
