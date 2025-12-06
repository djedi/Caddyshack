package handlers

import (
	"log"
	"net/http"

	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/store"
)

// AuditLogger provides methods for logging audit events.
type AuditLogger struct {
	store *store.Store
}

// NewAuditLogger creates a new AuditLogger.
func NewAuditLogger(s *store.Store) *AuditLogger {
	return &AuditLogger{store: s}
}

// Log logs an audit event.
func (a *AuditLogger) Log(r *http.Request, action store.AuditAction, resourceType store.AuditResourceType, resourceID, details string) {
	if a == nil || a.store == nil {
		return
	}

	entry := &store.AuditEntry{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    getClientIP(r),
		Username:     "system",
	}

	// Get user from context if available
	user := middleware.GetUserFromContext(r.Context())
	if user != nil {
		entry.UserID = &user.ID
		entry.Username = user.Username
	}

	if err := a.store.CreateAuditEntry(entry); err != nil {
		log.Printf("Failed to create audit entry: %v", err)
	}
}

// LogWithUser logs an audit event with a specific username (for login events before user is in context).
func (a *AuditLogger) LogWithUser(r *http.Request, action store.AuditAction, resourceType store.AuditResourceType, resourceID, details, username string, userID *int64) {
	if a == nil || a.store == nil {
		return
	}

	entry := &store.AuditEntry{
		UserID:       userID,
		Username:     username,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    getClientIP(r),
	}

	if err := a.store.CreateAuditEntry(entry); err != nil {
		log.Printf("Failed to create audit entry: %v", err)
	}
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header (when behind proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check for X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
