package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// ProfileData holds data for the profile page.
type ProfileData struct {
	User                    *ProfileUserView
	Sessions                []SessionView
	NotificationPreferences *NotificationPreferencesView
	Error                   string
	HasError                bool
	SuccessMessage          string
	PasswordError           string
	PasswordSuccess         string
	SessionsMessage         string
	NotificationsMessage    string
	NotificationsError      string
	TOTPEnabled             bool
	BackupCodeCount         int
}

// NotificationPreferencesView represents notification preferences for display.
type NotificationPreferencesView struct {
	NotifyCertExpiry    bool
	NotifyDomainExpiry  bool
	NotifyConfigChange  bool
	NotifyCaddyReload   bool
	NotifyContainerDown bool
	NotifySystem        bool
}

// ProfileUserView represents the current user for display.
type ProfileUserView struct {
	ID            int64
	Username      string
	Email         string
	Role          auth.Role
	RoleDisplay   string
	CreatedAt     string
	LastLoginText string
}

// SessionView represents a session for display.
type SessionView struct {
	ID        int64
	CreatedAt string
	ExpiresAt string
	IsCurrent bool
}

// ProfileHandler handles requests for the user profile page.
type ProfileHandler struct {
	templates    *templates.Templates
	config       *config.Config
	userStore    *auth.UserStore
	totpStore    *auth.TOTPStore
	authMW       *middleware.Auth
	errorHandler *ErrorHandler
}

// NewProfileHandler creates a new ProfileHandler.
func NewProfileHandler(tmpl *templates.Templates, cfg *config.Config, userStore *auth.UserStore, authMW *middleware.Auth) *ProfileHandler {
	return &ProfileHandler{
		templates:    tmpl,
		config:       cfg,
		userStore:    userStore,
		totpStore:    auth.NewTOTPStore(userStore.DB()),
		authMW:       authMW,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// Show handles GET requests for the profile page.
func (h *ProfileHandler) Show(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Get fresh user data from database
	dbUser, err := h.userStore.GetByID(user.ID)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Get user sessions
	sessions, err := h.userStore.ListUserSessions(user.ID)
	if err != nil {
		log.Printf("Error listing user sessions: %v", err)
		sessions = nil
	}

	// Get current session token from cookie
	currentToken := ""
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		currentToken = cookie.Value
	}

	// Get notification preferences
	prefs, err := h.userStore.GetNotificationPreferences(user.ID)
	if err != nil {
		log.Printf("Error getting notification preferences: %v", err)
		prefs = auth.DefaultNotificationPreferences(user.ID)
	}

	// Get TOTP status
	totpEnabled := false
	backupCodeCount := 0
	if h.totpStore != nil {
		totpEnabled, _, _, _ = h.totpStore.GetTOTPStatus(user.ID)
		if totpEnabled {
			backupCodeCount, _ = h.totpStore.GetBackupCodeCount(user.ID)
		}
	}

	data := h.buildProfileData(dbUser, sessions, currentToken, prefs)
	data.TOTPEnabled = totpEnabled
	data.BackupCodeCount = backupCodeCount

	// Check for success message from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}

	pageData := WithPermissionsAndConfig(r, h.config, "My Profile", "profile", data)

	if err := h.templates.Render(w, "profile.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// UpdatePassword handles PUT requests to change the user's password.
func (h *ProfileHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderPasswordError(w, r, user, "Failed to parse form data")
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_new_password")

	// Validate required fields
	if currentPassword == "" {
		h.renderPasswordError(w, r, user, "Current password is required")
		return
	}

	if newPassword == "" {
		h.renderPasswordError(w, r, user, "New password is required")
		return
	}

	if newPassword != confirmPassword {
		h.renderPasswordError(w, r, user, "New passwords do not match")
		return
	}

	if len(newPassword) < 8 {
		h.renderPasswordError(w, r, user, "New password must be at least 8 characters")
		return
	}

	// Verify current password
	_, err := h.userStore.Authenticate(user.Username, currentPassword)
	if err != nil {
		h.renderPasswordError(w, r, user, "Current password is incorrect")
		return
	}

	// Update password
	if err := h.userStore.UpdatePassword(user.ID, newPassword); err != nil {
		h.renderPasswordError(w, r, user, "Failed to update password: "+err.Error())
		return
	}

	// Return success message
	h.renderPasswordSuccess(w, r, user, "Password changed successfully")
}

// LogoutSession handles DELETE requests to log out a specific session.
func (h *ProfileHandler) LogoutSession(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Extract session ID from URL path (e.g., /profile/sessions/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/profile/sessions/")
	path = strings.TrimSuffix(path, "/")

	sessionID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid session ID")
		return
	}

	// Get the session to find its token
	sessions, err := h.userStore.ListUserSessions(user.ID)
	if err != nil {
		h.renderSessionsError(w, r, user, "Failed to list sessions")
		return
	}

	// Find and delete the session
	var tokenToDelete string
	for _, s := range sessions {
		if s.ID == sessionID {
			tokenToDelete = s.Token
			break
		}
	}

	if tokenToDelete == "" {
		h.renderSessionsError(w, r, user, "Session not found")
		return
	}

	// Get current session token to prevent deleting own session
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		if cookie.Value == tokenToDelete {
			h.renderSessionsError(w, r, user, "Cannot log out current session")
			return
		}
	}

	if err := h.userStore.DeleteSession(tokenToDelete); err != nil {
		h.renderSessionsError(w, r, user, "Failed to log out session")
		return
	}

	h.renderSessionsList(w, r, user, "Session logged out successfully")
}

// LogoutOtherSessions handles POST requests to log out all other sessions.
func (h *ProfileHandler) LogoutOtherSessions(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Get current session token
	currentToken := ""
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		currentToken = cookie.Value
	}

	if currentToken == "" {
		h.renderSessionsError(w, r, user, "Could not determine current session")
		return
	}

	// Get all sessions
	sessions, err := h.userStore.ListUserSessions(user.ID)
	if err != nil {
		h.renderSessionsError(w, r, user, "Failed to list sessions")
		return
	}

	// Delete all sessions except the current one
	deletedCount := 0
	for _, s := range sessions {
		if s.Token != currentToken {
			if err := h.userStore.DeleteSession(s.Token); err != nil {
				log.Printf("Failed to delete session %d: %v", s.ID, err)
			} else {
				deletedCount++
			}
		}
	}

	message := "Logged out of all other sessions"
	if deletedCount == 1 {
		message = "Logged out of 1 other session"
	} else if deletedCount > 1 {
		message = "Logged out of " + strconv.Itoa(deletedCount) + " other sessions"
	}

	h.renderSessionsList(w, r, user, message)
}

// buildProfileData constructs ProfileData from user, sessions, and notification preferences.
func (h *ProfileHandler) buildProfileData(user *auth.User, sessions []*auth.Session, currentToken string, prefs *auth.NotificationPreferences) ProfileData {
	userView := &ProfileUserView{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	}

	// Role display name
	switch user.Role {
	case auth.RoleAdmin:
		userView.RoleDisplay = "Administrator"
	case auth.RoleEditor:
		userView.RoleDisplay = "Editor"
	case auth.RoleViewer:
		userView.RoleDisplay = "Viewer"
	default:
		userView.RoleDisplay = string(user.Role)
	}

	// Format dates
	userView.CreatedAt = user.CreatedAt.Format("January 2, 2006")
	if user.LastLogin != nil {
		userView.LastLoginText = user.LastLogin.Format("January 2, 2006 at 3:04 PM")
	} else {
		userView.LastLoginText = "Never"
	}

	// Build session views
	sessionViews := make([]SessionView, len(sessions))
	for i, s := range sessions {
		sessionViews[i] = SessionView{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
			ExpiresAt: s.ExpiresAt.Format("Jan 2, 2006 3:04 PM"),
			IsCurrent: s.Token == currentToken,
		}
	}

	// Build notification preferences view
	prefsView := &NotificationPreferencesView{
		NotifyCertExpiry:    prefs.NotifyCertExpiry,
		NotifyDomainExpiry:  prefs.NotifyDomainExpiry,
		NotifyConfigChange:  prefs.NotifyConfigChange,
		NotifyCaddyReload:   prefs.NotifyCaddyReload,
		NotifyContainerDown: prefs.NotifyContainerDown,
		NotifySystem:        prefs.NotifySystem,
	}

	return ProfileData{
		User:                    userView,
		Sessions:                sessionViews,
		NotificationPreferences: prefsView,
	}
}

// UpdateNotificationPreferences handles PUT requests to update notification preferences.
func (h *ProfileHandler) UpdateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderNotificationsError(w, r, user, "Failed to parse form data")
		return
	}

	// Parse checkbox values (checkboxes only send value when checked)
	prefs := &auth.NotificationPreferences{
		UserID:             user.ID,
		NotifyCertExpiry:   r.FormValue("notify_cert_expiry") == "on",
		NotifyDomainExpiry: r.FormValue("notify_domain_expiry") == "on",
		NotifyConfigChange: r.FormValue("notify_config_change") == "on",
		NotifyCaddyReload:  r.FormValue("notify_caddy_reload") == "on",
		NotifyContainerDown: r.FormValue("notify_container_down") == "on",
		NotifySystem:       r.FormValue("notify_system") == "on",
	}

	if err := h.userStore.SaveNotificationPreferences(prefs); err != nil {
		h.renderNotificationsError(w, r, user, "Failed to save preferences: "+err.Error())
		return
	}

	h.renderNotificationsSuccess(w, r, user, prefs, "Preferences saved successfully")
}

// renderNotificationsError renders the notifications form with an error.
func (h *ProfileHandler) renderNotificationsError(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	// Get current preferences to preserve form state
	prefs, err := h.userStore.GetNotificationPreferences(user.ID)
	if err != nil {
		prefs = auth.DefaultNotificationPreferences(user.ID)
	}

	prefsView := &NotificationPreferencesView{
		NotifyCertExpiry:    prefs.NotifyCertExpiry,
		NotifyDomainExpiry:  prefs.NotifyDomainExpiry,
		NotifyConfigChange:  prefs.NotifyConfigChange,
		NotifyCaddyReload:   prefs.NotifyCaddyReload,
		NotifyContainerDown: prefs.NotifyContainerDown,
		NotifySystem:        prefs.NotifySystem,
	}

	data := ProfileData{
		NotificationPreferences: prefsView,
		NotificationsError:      errMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-notifications-form.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderNotificationsSuccess renders the notifications form with a success message.
func (h *ProfileHandler) renderNotificationsSuccess(w http.ResponseWriter, r *http.Request, user *auth.User, prefs *auth.NotificationPreferences, msg string) {
	prefsView := &NotificationPreferencesView{
		NotifyCertExpiry:    prefs.NotifyCertExpiry,
		NotifyDomainExpiry:  prefs.NotifyDomainExpiry,
		NotifyConfigChange:  prefs.NotifyConfigChange,
		NotifyCaddyReload:   prefs.NotifyCaddyReload,
		NotifyContainerDown: prefs.NotifyContainerDown,
		NotifySystem:        prefs.NotifySystem,
	}

	data := ProfileData{
		NotificationPreferences: prefsView,
		NotificationsMessage:    msg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-notifications-form.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderPasswordError renders the password form with an error.
func (h *ProfileHandler) renderPasswordError(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	data := ProfileData{
		PasswordError: errMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-password-form.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderPasswordSuccess renders the password form with a success message.
func (h *ProfileHandler) renderPasswordSuccess(w http.ResponseWriter, r *http.Request, user *auth.User, msg string) {
	data := ProfileData{
		PasswordSuccess: msg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-password-form.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderSessionsList renders the sessions list with an optional message.
func (h *ProfileHandler) renderSessionsList(w http.ResponseWriter, r *http.Request, user *auth.User, msg string) {
	// Get updated sessions
	sessions, err := h.userStore.ListUserSessions(user.ID)
	if err != nil {
		log.Printf("Error listing user sessions: %v", err)
		sessions = nil
	}

	// Get current session token
	currentToken := ""
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		currentToken = cookie.Value
	}

	// Build session views for partial render (no prefs needed)
	sessionViews := make([]SessionView, len(sessions))
	for i, s := range sessions {
		sessionViews[i] = SessionView{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
			ExpiresAt: s.ExpiresAt.Format("Jan 2, 2006 3:04 PM"),
			IsCurrent: s.Token == currentToken,
		}
	}

	data := ProfileData{
		Sessions:        sessionViews,
		SessionsMessage: msg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-sessions-list.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderSessionsError renders the sessions list with an error.
func (h *ProfileHandler) renderSessionsError(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	// Get sessions
	sessions, err := h.userStore.ListUserSessions(user.ID)
	if err != nil {
		log.Printf("Error listing user sessions: %v", err)
		sessions = nil
	}

	// Get current session token
	currentToken := ""
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		currentToken = cookie.Value
	}

	// Build session views for partial render (no prefs needed)
	sessionViews := make([]SessionView, len(sessions))
	for i, s := range sessions {
		sessionViews[i] = SessionView{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format("Jan 2, 2006 3:04 PM"),
			ExpiresAt: s.ExpiresAt.Format("Jan 2, 2006 3:04 PM"),
			IsCurrent: s.Token == currentToken,
		}
	}

	data := ProfileData{
		Sessions: sessionViews,
		Error:    errMsg,
		HasError: true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "profile-sessions-list.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}
