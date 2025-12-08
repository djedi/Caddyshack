package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// DashboardData holds data displayed on the dashboard page.
type DashboardData struct {
	SiteCount            int
	SnippetCount         int
	CaddyStatus          *caddy.CaddyStatus
	DashboardPreferences *auth.DashboardPreferences
}

// DashboardHandler handles requests for the dashboard page.
type DashboardHandler struct {
	templates     *templates.Templates
	adminClient   *caddy.AdminClient
	userStore     *auth.UserStore
	errorHandler  *ErrorHandler
	multiUser     bool
	caddyfilePath string
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(tmpl *templates.Templates, cfg *config.Config, userStore *auth.UserStore) *DashboardHandler {
	return &DashboardHandler{
		templates:     tmpl,
		adminClient:   caddy.NewAdminClient(cfg.CaddyAdminAPI),
		userStore:     userStore,
		errorHandler:  NewErrorHandler(tmpl),
		multiUser:     cfg.MultiUserMode,
		caddyfilePath: cfg.CaddyfilePath,
	}
}

// ServeHTTP handles GET requests for the dashboard.
func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path
	if r.URL.Path != "/" {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Get Caddy status
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, _ := h.adminClient.GetStatus(ctx)

	// Get site and snippet counts from Caddyfile
	siteCount := 0
	snippetCount := 0
	if content, err := os.ReadFile(h.caddyfilePath); err == nil {
		parser := caddy.NewParser(string(content))
		if sites, err := parser.ParseSites(); err == nil {
			siteCount = len(sites)
		}
		if snippets, err := parser.ParseSnippets(); err == nil {
			snippetCount = len(snippets)
		}
	}

	// Get user dashboard preferences
	var prefs *auth.DashboardPreferences
	user := middleware.GetUserFromContext(r.Context())
	if user != nil && h.userStore != nil {
		var err error
		prefs, err = h.userStore.GetDashboardPreferences(user.ID)
		if err != nil {
			prefs = auth.DefaultDashboardPreferences(user.ID)
		}
	} else {
		prefs = auth.DefaultDashboardPreferences(0)
	}

	data := templates.PageData{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
		Data: DashboardData{
			SiteCount:            siteCount,
			SnippetCount:         snippetCount,
			CaddyStatus:          status,
			DashboardPreferences: prefs,
		},
	}

	if err := h.templates.Render(w, "dashboard.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
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
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// SavePreferencesRequest is the request body for saving dashboard preferences.
type SavePreferencesRequest struct {
	WidgetOrder      []string `json:"widgetOrder"`
	HiddenWidgets    []string `json:"hiddenWidgets"`
	CollapsedWidgets []string `json:"collapsedWidgets"`
}

// SavePreferences handles PUT requests to save dashboard preferences.
func (h *DashboardHandler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if h.userStore == nil {
		http.Error(w, "User preferences not available", http.StatusServiceUnavailable)
		return
	}

	var req SavePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate widget order contains only valid widget IDs
	validWidgets := map[string]bool{
		"sites":        true,
		"snippets":     true,
		"containers":   true,
		"certificates": true,
		"status":       true,
		"performance":  true,
	}

	for _, widgetID := range req.WidgetOrder {
		if !validWidgets[widgetID] {
			http.Error(w, "Invalid widget ID: "+widgetID, http.StatusBadRequest)
			return
		}
	}

	for _, widgetID := range req.HiddenWidgets {
		if !validWidgets[widgetID] {
			http.Error(w, "Invalid widget ID: "+widgetID, http.StatusBadRequest)
			return
		}
	}

	for _, widgetID := range req.CollapsedWidgets {
		if !validWidgets[widgetID] {
			http.Error(w, "Invalid widget ID: "+widgetID, http.StatusBadRequest)
			return
		}
	}

	prefs := &auth.DashboardPreferences{
		UserID:           user.ID,
		WidgetOrder:      req.WidgetOrder,
		HiddenWidgets:    req.HiddenWidgets,
		CollapsedWidgets: req.CollapsedWidgets,
	}

	if err := h.userStore.SaveDashboardPreferences(prefs); err != nil {
		http.Error(w, "Failed to save preferences", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
