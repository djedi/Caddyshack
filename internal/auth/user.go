package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Role represents a user's role in the system.
type Role string

const (
	// RoleAdmin has full access to all features.
	RoleAdmin Role = "admin"

	// RoleEditor can manage sites and snippets but not users or global settings.
	RoleEditor Role = "editor"

	// RoleViewer has read-only access.
	RoleViewer Role = "viewer"
)

// ValidRoles is a list of all valid roles.
var ValidRoles = []Role{RoleAdmin, RoleEditor, RoleViewer}

// IsValid checks if the role is a valid role.
func (r Role) IsValid() bool {
	for _, valid := range ValidRoles {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the role.
func (r Role) String() string {
	return string(r)
}

// User represents a user in the system.
type User struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	LastLogin    *time.Time
}

// Session represents an authenticated user session.
type Session struct {
	ID        int64
	UserID    int64
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionDuration is how long a session is valid.
const SessionDuration = 24 * time.Hour

// bcrypt cost for password hashing
const bcryptCost = 12

var (
	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidCredentials is returned when credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrUsernameExists is returned when a username already exists.
	ErrUsernameExists = errors.New("username already exists")

	// ErrInvalidRole is returned when an invalid role is specified.
	ErrInvalidRole = errors.New("invalid role")

	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session expired")
)

// UserStore provides database operations for users and sessions.
type UserStore struct {
	db *sql.DB
}

// NewUserStore creates a new UserStore.
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a password with its hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Create creates a new user.
func (s *UserStore) Create(username, email, password string, role Role) (*User, error) {
	if !role.IsValid() {
		return nil, ErrInvalidRole
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	result, err := s.db.Exec(
		`INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, ?)`,
		username, email, hash, string(role),
	)
	if err != nil {
		// Check for unique constraint violation
		if isUniqueConstraintError(err) {
			return nil, ErrUsernameExists
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting user ID: %w", err)
	}

	return &User{
		ID:           id,
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    time.Now(),
	}, nil
}

// GetByID retrieves a user by ID.
func (s *UserStore) GetByID(id int64) (*User, error) {
	user := &User{}
	var lastLogin sql.NullTime
	var role string

	err := s.db.QueryRow(
		`SELECT id, username, email, password_hash, role, created_at, last_login
		 FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &role, &user.CreatedAt, &lastLogin)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	user.Role = Role(role)
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return user, nil
}

// GetByUsername retrieves a user by username.
func (s *UserStore) GetByUsername(username string) (*User, error) {
	user := &User{}
	var lastLogin sql.NullTime
	var role string

	err := s.db.QueryRow(
		`SELECT id, username, email, password_hash, role, created_at, last_login
		 FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &role, &user.CreatedAt, &lastLogin)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}

	user.Role = Role(role)
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return user, nil
}

// List retrieves all users.
func (s *UserStore) List() ([]*User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, email, password_hash, role, created_at, last_login
		 FROM users ORDER BY username`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var lastLogin sql.NullTime
		var role string

		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &role, &user.CreatedAt, &lastLogin); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}

		user.Role = Role(role)
		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating users: %w", err)
	}

	return users, nil
}

// Update updates a user's information (excluding password).
func (s *UserStore) Update(id int64, username, email string, role Role) error {
	if !role.IsValid() {
		return ErrInvalidRole
	}

	result, err := s.db.Exec(
		`UPDATE users SET username = ?, email = ?, role = ? WHERE id = ?`,
		username, email, string(role), id,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrUsernameExists
		}
		return fmt.Errorf("updating user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking update: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdatePassword updates a user's password.
func (s *UserStore) UpdatePassword(id int64, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	result, err := s.db.Exec(
		`UPDATE users SET password_hash = ? WHERE id = ?`,
		hash, id,
	)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking update: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// Delete deletes a user.
func (s *UserStore) Delete(id int64) error {
	result, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking delete: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp for a user.
func (s *UserStore) UpdateLastLogin(id int64) error {
	_, err := s.db.Exec(
		`UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating last login: %w", err)
	}
	return nil
}

// Count returns the number of users in the system.
func (s *UserStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// Authenticate validates credentials and returns the user if valid.
func (s *UserStore) Authenticate(username, password string) (*User, error) {
	user, err := s.GetByUsername(username)
	if err != nil {
		if err == ErrUserNotFound {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Update last login timestamp
	_ = s.UpdateLastLogin(user.ID)

	return user, nil
}

// CreateSession creates a new session for a user.
func (s *UserStore) CreateSession(userID int64) (*Session, error) {
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	expiresAt := time.Now().Add(SessionDuration)

	result, err := s.db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)`,
		userID, token, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting session ID: %w", err)
	}

	return &Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

// GetSessionByToken retrieves a session by its token.
func (s *UserStore) GetSessionByToken(token string) (*Session, error) {
	session := &Session{}

	err := s.db.QueryRow(
		`SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = ?`,
		token,
	).Scan(&session.ID, &session.UserID, &session.Token, &session.CreatedAt, &session.ExpiresAt)

	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = s.DeleteSession(token)
		return nil, ErrSessionExpired
	}

	return session, nil
}

// ValidateSession checks if a session token is valid and returns the user.
func (s *UserStore) ValidateSession(token string) (*User, error) {
	session, err := s.GetSessionByToken(token)
	if err != nil {
		return nil, err
	}

	return s.GetByID(session.UserID)
}

// DeleteSession removes a session by token.
func (s *UserStore) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteUserSessions removes all sessions for a user.
func (s *UserStore) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions: %w", err)
	}
	return nil
}

// CleanExpiredSessions removes all expired sessions.
func (s *UserStore) CleanExpiredSessions() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, fmt.Errorf("cleaning expired sessions: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting deleted count: %w", err)
	}

	return count, nil
}

// ListUserSessions lists all active sessions for a user.
func (s *UserStore) ListUserSessions(userID int64) ([]*Session, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, token, created_at, expires_at
		 FROM sessions WHERE user_id = ? AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		if err := rows.Scan(&session.ID, &session.UserID, &session.Token, &session.CreatedAt, &session.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}

	return sessions, nil
}

// generateToken generates a secure random token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// isUniqueConstraintError checks if the error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	// SQLite returns "UNIQUE constraint failed" in the error message
	return err != nil && (
		contains(err.Error(), "UNIQUE constraint failed") ||
		contains(err.Error(), "unique constraint"))
}

// contains is a helper for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Permission represents an action that can be performed.
type Permission string

const (
	// PermViewDashboard allows viewing the dashboard.
	PermViewDashboard Permission = "view:dashboard"

	// PermViewSites allows viewing sites.
	PermViewSites Permission = "view:sites"

	// PermEditSites allows creating, editing, and deleting sites.
	PermEditSites Permission = "edit:sites"

	// PermViewSnippets allows viewing snippets.
	PermViewSnippets Permission = "view:snippets"

	// PermEditSnippets allows creating, editing, and deleting snippets.
	PermEditSnippets Permission = "edit:snippets"

	// PermViewGlobal allows viewing global options.
	PermViewGlobal Permission = "view:global"

	// PermEditGlobal allows editing global options.
	PermEditGlobal Permission = "edit:global"

	// PermViewHistory allows viewing config history.
	PermViewHistory Permission = "view:history"

	// PermRestoreHistory allows restoring config from history.
	PermRestoreHistory Permission = "restore:history"

	// PermViewLogs allows viewing logs.
	PermViewLogs Permission = "view:logs"

	// PermViewCerts allows viewing certificates.
	PermViewCerts Permission = "view:certs"

	// PermViewContainers allows viewing containers.
	PermViewContainers Permission = "view:containers"

	// PermManageContainers allows managing containers (start/stop/restart).
	PermManageContainers Permission = "manage:containers"

	// PermViewDomains allows viewing domains.
	PermViewDomains Permission = "view:domains"

	// PermEditDomains allows editing domains.
	PermEditDomains Permission = "edit:domains"

	// PermImportExport allows importing and exporting configuration.
	PermImportExport Permission = "import:export"

	// PermViewNotifications allows viewing notifications.
	PermViewNotifications Permission = "view:notifications"

	// PermManageNotifications allows acknowledging notifications.
	PermManageNotifications Permission = "manage:notifications"

	// PermViewUsers allows viewing users.
	PermViewUsers Permission = "view:users"

	// PermManageUsers allows creating, editing, and deleting users.
	PermManageUsers Permission = "manage:users"

	// PermViewAuditLog allows viewing the audit log.
	PermViewAuditLog Permission = "view:audit"
)

// rolePermissions defines what permissions each role has.
var rolePermissions = map[Role][]Permission{
	RoleViewer: {
		PermViewDashboard,
		PermViewSites,
		PermViewSnippets,
		PermViewGlobal,
		PermViewHistory,
		PermViewLogs,
		PermViewCerts,
		PermViewContainers,
		PermViewDomains,
		PermViewNotifications,
	},
	RoleEditor: {
		PermViewDashboard,
		PermViewSites,
		PermEditSites,
		PermViewSnippets,
		PermEditSnippets,
		PermViewGlobal,
		PermViewHistory,
		PermRestoreHistory,
		PermViewLogs,
		PermViewCerts,
		PermViewContainers,
		PermViewDomains,
		PermEditDomains,
		PermImportExport,
		PermViewNotifications,
		PermManageNotifications,
	},
	RoleAdmin: {
		PermViewDashboard,
		PermViewSites,
		PermEditSites,
		PermViewSnippets,
		PermEditSnippets,
		PermViewGlobal,
		PermEditGlobal,
		PermViewHistory,
		PermRestoreHistory,
		PermViewLogs,
		PermViewCerts,
		PermViewContainers,
		PermManageContainers,
		PermViewDomains,
		PermEditDomains,
		PermImportExport,
		PermViewNotifications,
		PermManageNotifications,
		PermViewUsers,
		PermManageUsers,
		PermViewAuditLog,
	},
}

// HasPermission checks if a role has a specific permission.
func (r Role) HasPermission(perm Permission) bool {
	perms, ok := rolePermissions[r]
	if !ok {
		return false
	}

	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// GetPermissions returns all permissions for a role.
func (r Role) GetPermissions() []Permission {
	perms, ok := rolePermissions[r]
	if !ok {
		return nil
	}
	return perms
}

// CanEdit returns true if the role can edit content (sites, snippets, etc.)
func (r Role) CanEdit() bool {
	return r == RoleAdmin || r == RoleEditor
}

// CanManageUsers returns true if the role can manage users.
func (r Role) CanManageUsers() bool {
	return r == RoleAdmin
}

// CanEditGlobal returns true if the role can edit global settings.
func (r Role) CanEditGlobal() bool {
	return r == RoleAdmin
}

// NotificationPreferences represents a user's notification preferences.
type NotificationPreferences struct {
	UserID             int64
	NotifyCertExpiry   bool
	NotifyDomainExpiry bool
	NotifyConfigChange bool
	NotifyCaddyReload  bool
	NotifyContainerDown bool
	NotifySystem       bool
}

// DefaultNotificationPreferences returns the default notification preferences.
func DefaultNotificationPreferences(userID int64) *NotificationPreferences {
	return &NotificationPreferences{
		UserID:             userID,
		NotifyCertExpiry:   true,
		NotifyDomainExpiry: true,
		NotifyConfigChange: true,
		NotifyCaddyReload:  true,
		NotifyContainerDown: true,
		NotifySystem:       true,
	}
}

// GetNotificationPreferences retrieves notification preferences for a user.
// If no preferences exist, returns defaults with all notifications enabled.
func (s *UserStore) GetNotificationPreferences(userID int64) (*NotificationPreferences, error) {
	prefs := &NotificationPreferences{UserID: userID}

	err := s.db.QueryRow(`
		SELECT notify_cert_expiry, notify_domain_expiry, notify_config_change,
		       notify_caddy_reload, notify_container_down, notify_system
		FROM user_notification_preferences WHERE user_id = ?
	`, userID).Scan(
		&prefs.NotifyCertExpiry, &prefs.NotifyDomainExpiry, &prefs.NotifyConfigChange,
		&prefs.NotifyCaddyReload, &prefs.NotifyContainerDown, &prefs.NotifySystem,
	)

	if err == sql.ErrNoRows {
		// Return defaults if no preferences exist
		return DefaultNotificationPreferences(userID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting notification preferences: %w", err)
	}

	return prefs, nil
}

// SaveNotificationPreferences saves or updates notification preferences for a user.
func (s *UserStore) SaveNotificationPreferences(prefs *NotificationPreferences) error {
	_, err := s.db.Exec(`
		INSERT INTO user_notification_preferences
			(user_id, notify_cert_expiry, notify_domain_expiry, notify_config_change,
			 notify_caddy_reload, notify_container_down, notify_system, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			notify_cert_expiry = excluded.notify_cert_expiry,
			notify_domain_expiry = excluded.notify_domain_expiry,
			notify_config_change = excluded.notify_config_change,
			notify_caddy_reload = excluded.notify_caddy_reload,
			notify_container_down = excluded.notify_container_down,
			notify_system = excluded.notify_system,
			updated_at = CURRENT_TIMESTAMP
	`, prefs.UserID, prefs.NotifyCertExpiry, prefs.NotifyDomainExpiry, prefs.NotifyConfigChange,
		prefs.NotifyCaddyReload, prefs.NotifyContainerDown, prefs.NotifySystem)

	if err != nil {
		return fmt.Errorf("saving notification preferences: %w", err)
	}

	return nil
}

// DashboardPreferences represents a user's dashboard layout preferences.
type DashboardPreferences struct {
	UserID           int64
	WidgetOrder      []string
	HiddenWidgets    []string
	CollapsedWidgets []string
}

// DefaultWidgetOrder is the default order of dashboard widgets.
var DefaultWidgetOrder = []string{"sites", "snippets", "containers", "certificates", "status"}

// DefaultDashboardPreferences returns the default dashboard preferences.
func DefaultDashboardPreferences(userID int64) *DashboardPreferences {
	return &DashboardPreferences{
		UserID:           userID,
		WidgetOrder:      DefaultWidgetOrder,
		HiddenWidgets:    []string{},
		CollapsedWidgets: []string{},
	}
}

// GetDashboardPreferences retrieves dashboard preferences for a user.
// If no preferences exist, returns defaults.
func (s *UserStore) GetDashboardPreferences(userID int64) (*DashboardPreferences, error) {
	var widgetOrderJSON, hiddenWidgetsJSON, collapsedWidgetsJSON string

	err := s.db.QueryRow(`
		SELECT widget_order, hidden_widgets, collapsed_widgets
		FROM user_dashboard_preferences WHERE user_id = ?
	`, userID).Scan(&widgetOrderJSON, &hiddenWidgetsJSON, &collapsedWidgetsJSON)

	if err == sql.ErrNoRows {
		return DefaultDashboardPreferences(userID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting dashboard preferences: %w", err)
	}

	prefs := &DashboardPreferences{UserID: userID}

	if err := json.Unmarshal([]byte(widgetOrderJSON), &prefs.WidgetOrder); err != nil {
		prefs.WidgetOrder = DefaultWidgetOrder
	}
	if err := json.Unmarshal([]byte(hiddenWidgetsJSON), &prefs.HiddenWidgets); err != nil {
		prefs.HiddenWidgets = []string{}
	}
	if err := json.Unmarshal([]byte(collapsedWidgetsJSON), &prefs.CollapsedWidgets); err != nil {
		prefs.CollapsedWidgets = []string{}
	}

	return prefs, nil
}

// SaveDashboardPreferences saves or updates dashboard preferences for a user.
func (s *UserStore) SaveDashboardPreferences(prefs *DashboardPreferences) error {
	widgetOrderJSON, err := json.Marshal(prefs.WidgetOrder)
	if err != nil {
		return fmt.Errorf("marshaling widget order: %w", err)
	}

	hiddenWidgetsJSON, err := json.Marshal(prefs.HiddenWidgets)
	if err != nil {
		return fmt.Errorf("marshaling hidden widgets: %w", err)
	}

	collapsedWidgetsJSON, err := json.Marshal(prefs.CollapsedWidgets)
	if err != nil {
		return fmt.Errorf("marshaling collapsed widgets: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO user_dashboard_preferences
			(user_id, widget_order, hidden_widgets, collapsed_widgets, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			widget_order = excluded.widget_order,
			hidden_widgets = excluded.hidden_widgets,
			collapsed_widgets = excluded.collapsed_widgets,
			updated_at = CURRENT_TIMESTAMP
	`, prefs.UserID, widgetOrderJSON, hiddenWidgetsJSON, collapsedWidgetsJSON)

	if err != nil {
		return fmt.Errorf("saving dashboard preferences: %w", err)
	}

	return nil
}

// IsWidgetHidden checks if a widget is hidden.
func (p *DashboardPreferences) IsWidgetHidden(widgetID string) bool {
	for _, w := range p.HiddenWidgets {
		if w == widgetID {
			return true
		}
	}
	return false
}

// IsWidgetCollapsed checks if a widget is collapsed.
func (p *DashboardPreferences) IsWidgetCollapsed(widgetID string) bool {
	for _, w := range p.CollapsedWidgets {
		if w == widgetID {
			return true
		}
	}
	return false
}
