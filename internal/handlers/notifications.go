package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/notifications"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// NotificationsData holds data displayed on the notifications page.
type NotificationsData struct {
	Notifications      []notifications.Notification
	UnreadCount        int
	FilterSeverity     string
	FilterType         string
	ShowAcknowledged   bool
	AvailableSeverities []string
	AvailableTypes     []string
}

// NotificationsHandler handles requests for the notifications pages.
type NotificationsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	notifService *notifications.Service
	errorHandler *ErrorHandler
}

// NewNotificationsHandler creates a new NotificationsHandler.
func NewNotificationsHandler(tmpl *templates.Templates, cfg *config.Config, db *store.Store) *NotificationsHandler {
	return &NotificationsHandler{
		templates:    tmpl,
		config:       cfg,
		notifService: notifications.NewService(db.DB()),
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the notifications page (full notification center).
func (h *NotificationsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := NotificationsData{
		AvailableSeverities: []string{"info", "warning", "critical", "error"},
		AvailableTypes: []string{
			string(notifications.TypeCertExpiry),
			string(notifications.TypeDomainExpiry),
			string(notifications.TypeConfigChange),
			string(notifications.TypeCaddyReload),
			string(notifications.TypeContainerDown),
			string(notifications.TypeSystem),
		},
	}

	// Get filter parameters from query string
	data.FilterSeverity = r.URL.Query().Get("severity")
	data.FilterType = r.URL.Query().Get("type")
	data.ShowAcknowledged = r.URL.Query().Get("show_acknowledged") == "true"

	// Get unread count
	unreadCount, err := h.notifService.UnreadCount()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}
	data.UnreadCount = unreadCount

	// Get notifications based on filters
	var notifs []notifications.Notification
	if data.FilterSeverity != "" {
		notifs, err = h.notifService.ListBySeverity(notifications.Severity(data.FilterSeverity), 100, data.ShowAcknowledged)
	} else if data.FilterType != "" {
		notifs, err = h.notifService.ListByType(notifications.Type(data.FilterType), 100, data.ShowAcknowledged)
	} else {
		notifs, err = h.notifService.List(100, data.ShowAcknowledged)
	}

	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}
	data.Notifications = notifs

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "notifications-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "Notifications",
		ActiveNav: "notifications",
		Data:      data,
	}

	if err := h.templates.Render(w, "notifications.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// BadgeData holds data for the notification badge.
type BadgeData struct {
	UnreadCount         int
	CriticalCount       int
	WarningCount        int
	HasCritical         bool
	HasWarning          bool
}

// Badge handles GET requests for the notification badge count (for HTMX polling).
func (h *NotificationsHandler) Badge(w http.ResponseWriter, r *http.Request) {
	data := BadgeData{}

	// Get total unread count
	unreadCount, err := h.notifService.UnreadCount()
	if err != nil {
		// Return empty badge on error
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(""))
		return
	}
	data.UnreadCount = unreadCount

	// Get critical count
	criticalCount, err := h.notifService.UnreadCountBySeverity(notifications.SeverityCritical)
	if err == nil {
		data.CriticalCount = criticalCount
		data.HasCritical = criticalCount > 0
	}

	// Get warning count
	warningCount, err := h.notifService.UnreadCountBySeverity(notifications.SeverityWarning)
	if err == nil {
		data.WarningCount = warningCount
		data.HasWarning = warningCount > 0
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "notification-badge.html", data); err != nil {
		w.Write([]byte(""))
	}
}

// PanelData holds data for the notification panel dropdown.
type PanelData struct {
	Notifications []notifications.Notification
	UnreadCount   int
	HasMore       bool
}

// Panel handles GET requests for the notification panel dropdown (for HTMX).
func (h *NotificationsHandler) Panel(w http.ResponseWriter, r *http.Request) {
	data := PanelData{}

	// Get recent unread notifications (limit to 5 for the panel)
	notifs, err := h.notifService.List(5, false)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}
	data.Notifications = notifs

	// Get total unread count to show "more" indicator
	unreadCount, err := h.notifService.UnreadCount()
	if err == nil {
		data.UnreadCount = unreadCount
		data.HasMore = unreadCount > 5
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "notification-panel.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Acknowledge handles PUT requests to acknowledge a single notification.
func (h *NotificationsHandler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	// Extract notification ID from path: /notifications/{id}/acknowledge
	path := strings.TrimPrefix(r.URL.Path, "/notifications/")
	path = strings.TrimSuffix(path, "/acknowledge")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	if err := h.notifService.Acknowledge(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Return empty response to remove the notification from the list
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// AcknowledgeAll handles POST requests to acknowledge all notifications.
func (h *NotificationsHandler) AcknowledgeAll(w http.ResponseWriter, r *http.Request) {
	_, err := h.notifService.AcknowledgeAll()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Check if this is an HTMX request
	if r.Header.Get("HX-Request") == "true" {
		// Return empty list
		data := NotificationsData{
			Notifications: []notifications.Notification{},
			UnreadCount:   0,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "notifications-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// Redirect to notifications page
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// Delete handles DELETE requests to delete a notification.
func (h *NotificationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Extract notification ID from path: /notifications/{id}
	path := strings.TrimPrefix(r.URL.Path, "/notifications/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	if err := h.notifService.Delete(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Return empty response to remove the notification from the list
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}
