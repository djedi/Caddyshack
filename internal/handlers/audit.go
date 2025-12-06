package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// AuditData holds data displayed on the audit log page.
type AuditData struct {
	Entries        []AuditEntryView
	Error          string
	HasError       bool
	DistinctUsers  []string
	DistinctActions []string
	ResourceTypes  []string
	Filters        AuditFilters
	TotalCount     int
	CurrentPage    int
	TotalPages     int
	PageSize       int
	HasNextPage    bool
	HasPrevPage    bool
}

// AuditFilters represents the current filter state.
type AuditFilters struct {
	User         string
	Action       string
	ResourceType string
	StartDate    string
	EndDate      string
}

// AuditEntryView represents an audit entry for display in templates.
type AuditEntryView struct {
	ID              int64
	Username        string
	UserID          *int64
	Action          string
	ActionDisplay   string
	ResourceType    string
	ResourceTypeDisplay string
	ResourceID      string
	ResourceLink    string
	Details         string
	IPAddress       string
	CreatedAt       string
	CreatedAtRelative string
}

// AuditHandler handles requests for the audit log page.
type AuditHandler struct {
	templates    *templates.Templates
	config       *config.Config
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *AuditHandler {
	return &AuditHandler{
		templates:    tmpl,
		config:       cfg,
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the audit log page.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	data := AuditData{
		PageSize:    50,
		CurrentPage: 1,
	}

	// Parse query parameters for filtering
	q := r.URL.Query()

	// Parse page number
	if page := q.Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			data.CurrentPage = p
		}
	}

	// Parse filters
	data.Filters = AuditFilters{
		User:         q.Get("user"),
		Action:       q.Get("action"),
		ResourceType: q.Get("resource_type"),
		StartDate:    q.Get("start_date"),
		EndDate:      q.Get("end_date"),
	}

	// Build query options
	opts := store.AuditListOptions{
		Limit:  data.PageSize,
		Offset: (data.CurrentPage - 1) * data.PageSize,
	}

	if data.Filters.Action != "" {
		opts.Action = data.Filters.Action
	}

	if data.Filters.ResourceType != "" {
		opts.ResourceType = data.Filters.ResourceType
	}

	// Parse date filters
	if data.Filters.StartDate != "" {
		if t, err := time.Parse("2006-01-02", data.Filters.StartDate); err == nil {
			opts.StartDate = &t
		}
	}

	if data.Filters.EndDate != "" {
		if t, err := time.Parse("2006-01-02", data.Filters.EndDate); err == nil {
			// Set to end of day
			endOfDay := t.Add(24*time.Hour - time.Second)
			opts.EndDate = &endOfDay
		}
	}

	// Get total count for pagination
	count, err := h.store.CountAuditEntries(opts)
	if err != nil {
		data.Error = "Failed to count audit entries: " + err.Error()
		data.HasError = true
	} else {
		data.TotalCount = count
		data.TotalPages = (count + data.PageSize - 1) / data.PageSize
		if data.TotalPages == 0 {
			data.TotalPages = 1
		}
		data.HasNextPage = data.CurrentPage < data.TotalPages
		data.HasPrevPage = data.CurrentPage > 1
	}

	// Get audit entries
	if !data.HasError {
		entries, err := h.store.ListAuditEntries(opts)
		if err != nil {
			data.Error = "Failed to list audit entries: " + err.Error()
			data.HasError = true
		} else {
			data.Entries = make([]AuditEntryView, len(entries))
			for i, e := range entries {
				data.Entries[i] = toAuditEntryView(e)
			}
		}
	}

	// Get filter options (distinct values)
	if users, err := h.store.GetDistinctUsers(); err == nil {
		data.DistinctUsers = users
	}
	if actions, err := h.store.GetDistinctActions(); err == nil {
		data.DistinctActions = actions
	}

	// Define resource types for filter dropdown
	data.ResourceTypes = []string{
		string(store.ResourceSite),
		string(store.ResourceSnippet),
		string(store.ResourceUser),
		string(store.ResourceDomain),
		string(store.ResourceConfig),
		string(store.ResourceGlobal),
	}

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "audit-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := WithPermissions(r, "Audit Log", "audit", data)

	if err := h.templates.Render(w, "audit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// toAuditEntryView converts an AuditEntry to an AuditEntryView.
func toAuditEntryView(e *store.AuditEntry) AuditEntryView {
	view := AuditEntryView{
		ID:           e.ID,
		Username:     e.Username,
		UserID:       e.UserID,
		Action:       string(e.Action),
		ResourceType: string(e.ResourceType),
		ResourceID:   e.ResourceID,
		Details:      e.Details,
		IPAddress:    e.IPAddress,
		CreatedAt:    e.CreatedAt.Format("Jan 2, 2006 3:04:05 PM"),
	}

	// Generate relative time
	view.CreatedAtRelative = relativeTime(e.CreatedAt)

	// Generate display names for action
	view.ActionDisplay = formatAction(e.Action)

	// Generate display names for resource type
	view.ResourceTypeDisplay = formatResourceType(e.ResourceType)

	// Generate link to resource if possible
	view.ResourceLink = generateResourceLink(e.ResourceType, e.ResourceID)

	return view
}

// relativeTime returns a human-readable relative time string.
func relativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return strconv.Itoa(mins) + " minutes ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(hours) + " hours ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return strconv.Itoa(days) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

// formatAction returns a human-readable action name.
func formatAction(action store.AuditAction) string {
	actionNames := map[store.AuditAction]string{
		store.ActionSiteCreate:    "Created Site",
		store.ActionSiteUpdate:    "Updated Site",
		store.ActionSiteDelete:    "Deleted Site",
		store.ActionSnippetCreate: "Created Snippet",
		store.ActionSnippetUpdate: "Updated Snippet",
		store.ActionSnippetDelete: "Deleted Snippet",
		store.ActionUserCreate:    "Created User",
		store.ActionUserUpdate:    "Updated User",
		store.ActionUserDelete:    "Deleted User",
		store.ActionUserLogin:     "Logged In",
		store.ActionUserLogout:    "Logged Out",
		store.ActionDomainCreate:  "Created Domain",
		store.ActionDomainUpdate:  "Updated Domain",
		store.ActionDomainDelete:  "Deleted Domain",
		store.ActionConfigImport:  "Imported Config",
		store.ActionConfigExport:  "Exported Config",
		store.ActionConfigRestore: "Restored Config",
		store.ActionConfigReload:  "Reloaded Caddy",
		store.ActionGlobalUpdate:  "Updated Global Options",
	}

	if name, ok := actionNames[action]; ok {
		return name
	}
	return string(action)
}

// formatResourceType returns a human-readable resource type name.
func formatResourceType(rt store.AuditResourceType) string {
	typeNames := map[store.AuditResourceType]string{
		store.ResourceSite:    "Site",
		store.ResourceSnippet: "Snippet",
		store.ResourceUser:    "User",
		store.ResourceDomain:  "Domain",
		store.ResourceConfig:  "Configuration",
		store.ResourceGlobal:  "Global Options",
	}

	if name, ok := typeNames[rt]; ok {
		return name
	}
	return string(rt)
}

// generateResourceLink generates a link to the resource if applicable.
func generateResourceLink(rt store.AuditResourceType, resourceID string) string {
	if resourceID == "" {
		return ""
	}

	switch rt {
	case store.ResourceSite:
		return "/sites/" + resourceID
	case store.ResourceSnippet:
		return "/snippets/" + resourceID
	case store.ResourceUser:
		return "/users/" + resourceID + "/edit"
	case store.ResourceDomain:
		return "/domains/" + resourceID + "/edit"
	case store.ResourceConfig:
		if resourceID != "" {
			return "/history/" + resourceID + "/view"
		}
		return "/history"
	case store.ResourceGlobal:
		return "/global-options"
	default:
		return ""
	}
}
