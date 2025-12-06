package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TokenScope represents an API token scope/permission.
type TokenScope string

const (
	// ScopeRead allows read access to sites, snippets, and configuration.
	ScopeRead TokenScope = "read"

	// ScopeWrite allows create, update, and delete of sites and snippets.
	ScopeWrite TokenScope = "write"

	// ScopeAdmin allows full administrative access including user management.
	ScopeAdmin TokenScope = "admin"
)

// ValidScopes is a list of all valid token scopes.
var ValidScopes = []TokenScope{ScopeRead, ScopeWrite, ScopeAdmin}

// IsValid checks if the scope is valid.
func (s TokenScope) IsValid() bool {
	for _, valid := range ValidScopes {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the scope.
func (s TokenScope) String() string {
	return string(s)
}

// APIToken represents an API token in the database.
type APIToken struct {
	ID         int64
	UserID     int64
	TokenHash  string
	Name       string
	Scopes     []TokenScope
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// IsExpired returns true if the token has expired.
func (t *APIToken) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// IsRevoked returns true if the token has been revoked.
func (t *APIToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsValid returns true if the token is not expired and not revoked.
func (t *APIToken) IsValid() bool {
	return !t.IsExpired() && !t.IsRevoked()
}

// HasScope checks if the token has the specified scope.
func (t *APIToken) HasScope(scope TokenScope) bool {
	// Admin scope includes all other scopes
	for _, s := range t.Scopes {
		if s == ScopeAdmin || s == scope {
			return true
		}
	}
	return false
}

// HasWriteAccess returns true if the token has write or admin scope.
func (t *APIToken) HasWriteAccess() bool {
	return t.HasScope(ScopeWrite) || t.HasScope(ScopeAdmin)
}

// HasAdminAccess returns true if the token has admin scope.
func (t *APIToken) HasAdminAccess() bool {
	return t.HasScope(ScopeAdmin)
}

// TokenPrefix is the prefix for API tokens for easy identification.
const TokenPrefix = "csk_"

// TokenLength is the number of random bytes in a token (before encoding).
const TokenLength = 32

var (
	// ErrTokenNotFound is returned when a token is not found.
	ErrTokenNotFound = errors.New("token not found")

	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token expired")

	// ErrTokenRevoked is returned when a token has been revoked.
	ErrTokenRevoked = errors.New("token revoked")

	// ErrInvalidToken is returned when a token is invalid.
	ErrInvalidToken = errors.New("invalid token")

	// ErrTokenNameExists is returned when a token name already exists for a user.
	ErrTokenNameExists = errors.New("token name already exists")
)

// TokenStore provides database operations for API tokens.
type TokenStore struct {
	db *sql.DB
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

// generateRawToken generates a secure random token string.
func generateRawToken() (string, error) {
	b := make([]byte, TokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	// Use URL-safe base64 encoding and add prefix
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken creates a SHA-256 hash of the token for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Create creates a new API token for a user.
// Returns the raw token (which should be shown to the user once) and the token record.
func (s *TokenStore) Create(userID int64, name string, scopes []TokenScope, expiresAt *time.Time) (string, *APIToken, error) {
	// Generate raw token
	rawToken, err := generateRawToken()
	if err != nil {
		return "", nil, err
	}

	// Hash token for storage
	tokenHash := hashToken(rawToken)

	// Validate scopes
	for _, scope := range scopes {
		if !scope.IsValid() {
			return "", nil, fmt.Errorf("invalid scope: %s", scope)
		}
	}

	// Serialize scopes to JSON
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling scopes: %w", err)
	}

	// Check if name already exists for this user
	var count int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM api_tokens WHERE user_id = ? AND name = ? AND revoked_at IS NULL`,
		userID, name,
	).Scan(&count)
	if err != nil {
		return "", nil, fmt.Errorf("checking token name: %w", err)
	}
	if count > 0 {
		return "", nil, ErrTokenNameExists
	}

	// Insert token
	result, err := s.db.Exec(
		`INSERT INTO api_tokens (user_id, token_hash, name, scopes, expires_at) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, name, string(scopesJSON), expiresAt,
	)
	if err != nil {
		return "", nil, fmt.Errorf("creating token: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return "", nil, fmt.Errorf("getting token ID: %w", err)
	}

	token := &APIToken{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	return rawToken, token, nil
}

// ValidateToken validates a raw token and returns the token record and associated user.
// It also updates the last_used_at timestamp.
func (s *TokenStore) ValidateToken(rawToken string) (*APIToken, *User, error) {
	// Check prefix
	if !strings.HasPrefix(rawToken, TokenPrefix) {
		return nil, nil, ErrInvalidToken
	}

	// Hash the token for lookup
	tokenHash := hashToken(rawToken)

	// Look up token
	var token APIToken
	var scopesJSON string
	var expiresAt, lastUsedAt, revokedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, user_id, token_hash, name, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens WHERE token_hash = ?
	`, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.Name, &scopesJSON,
		&token.CreatedAt, &expiresAt, &lastUsedAt, &revokedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("getting token: %w", err)
	}

	// Parse nullable times
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}

	// Parse scopes
	if err := json.Unmarshal([]byte(scopesJSON), &token.Scopes); err != nil {
		return nil, nil, fmt.Errorf("parsing scopes: %w", err)
	}

	// Check if revoked
	if token.IsRevoked() {
		return nil, nil, ErrTokenRevoked
	}

	// Check if expired
	if token.IsExpired() {
		return nil, nil, ErrTokenExpired
	}

	// Update last_used_at
	_, err = s.db.Exec(
		`UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`,
		token.ID,
	)
	if err != nil {
		// Log but don't fail - this is not critical
		fmt.Printf("failed to update token last_used_at: %v\n", err)
	}

	// Get the user
	var user User
	var userLastLogin sql.NullTime
	var role string

	err = s.db.QueryRow(`
		SELECT id, username, email, password_hash, role, created_at, last_login
		FROM users WHERE id = ?
	`, token.UserID).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &role, &user.CreatedAt, &userLastLogin,
	)

	if err == sql.ErrNoRows {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("getting user: %w", err)
	}

	user.Role = Role(role)
	if userLastLogin.Valid {
		user.LastLogin = &userLastLogin.Time
	}

	return &token, &user, nil
}

// GetByID retrieves a token by ID.
func (s *TokenStore) GetByID(id int64) (*APIToken, error) {
	var token APIToken
	var scopesJSON string
	var expiresAt, lastUsedAt, revokedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, user_id, token_hash, name, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens WHERE id = ?
	`, id).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.Name, &scopesJSON,
		&token.CreatedAt, &expiresAt, &lastUsedAt, &revokedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting token: %w", err)
	}

	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}

	if err := json.Unmarshal([]byte(scopesJSON), &token.Scopes); err != nil {
		return nil, fmt.Errorf("parsing scopes: %w", err)
	}

	return &token, nil
}

// ListByUser lists all tokens for a user.
func (s *TokenStore) ListByUser(userID int64) ([]*APIToken, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, token_hash, name, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var token APIToken
		var scopesJSON string
		var expiresAt, lastUsedAt, revokedAt sql.NullTime

		if err := rows.Scan(
			&token.ID, &token.UserID, &token.TokenHash, &token.Name, &scopesJSON,
			&token.CreatedAt, &expiresAt, &lastUsedAt, &revokedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}

		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			token.LastUsedAt = &lastUsedAt.Time
		}
		if revokedAt.Valid {
			token.RevokedAt = &revokedAt.Time
		}

		if err := json.Unmarshal([]byte(scopesJSON), &token.Scopes); err != nil {
			return nil, fmt.Errorf("parsing scopes: %w", err)
		}

		tokens = append(tokens, &token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tokens: %w", err)
	}

	return tokens, nil
}

// ListActiveByUser lists all active (non-revoked, non-expired) tokens for a user.
func (s *TokenStore) ListActiveByUser(userID int64) ([]*APIToken, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, token_hash, name, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens
		WHERE user_id = ?
		AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing active tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var token APIToken
		var scopesJSON string
		var expiresAt, lastUsedAt, revokedAt sql.NullTime

		if err := rows.Scan(
			&token.ID, &token.UserID, &token.TokenHash, &token.Name, &scopesJSON,
			&token.CreatedAt, &expiresAt, &lastUsedAt, &revokedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}

		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			token.LastUsedAt = &lastUsedAt.Time
		}
		if revokedAt.Valid {
			token.RevokedAt = &revokedAt.Time
		}

		if err := json.Unmarshal([]byte(scopesJSON), &token.Scopes); err != nil {
			return nil, fmt.Errorf("parsing scopes: %w", err)
		}

		tokens = append(tokens, &token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tokens: %w", err)
	}

	return tokens, nil
}

// Revoke revokes a token by ID.
func (s *TokenStore) Revoke(id int64) error {
	result, err := s.db.Exec(
		`UPDATE api_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND revoked_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking revoke: %w", err)
	}
	if rows == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// RevokeAllForUser revokes all tokens for a user.
func (s *TokenStore) RevokeAllForUser(userID int64) (int64, error) {
	result, err := s.db.Exec(
		`UPDATE api_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("revoking user tokens: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting revoked count: %w", err)
	}

	return rows, nil
}

// Delete permanently deletes a token by ID.
func (s *TokenStore) Delete(id int64) error {
	result, err := s.db.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking delete: %w", err)
	}
	if rows == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// CleanExpiredTokens removes all expired tokens.
func (s *TokenStore) CleanExpiredTokens() (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return 0, fmt.Errorf("cleaning expired tokens: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting deleted count: %w", err)
	}

	return count, nil
}

// CountByUser returns the count of active tokens for a user.
func (s *TokenStore) CountByUser(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM api_tokens
		WHERE user_id = ?
		AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting tokens: %w", err)
	}
	return count, nil
}

// ScopeToPermissions maps a token scope to the permissions it grants.
func ScopeToPermissions(scope TokenScope) []Permission {
	switch scope {
	case ScopeRead:
		return []Permission{
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
		}
	case ScopeWrite:
		return []Permission{
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
		}
	case ScopeAdmin:
		return []Permission{
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
		}
	default:
		return nil
	}
}

// TokenHasPermission checks if a token has the specified permission.
func TokenHasPermission(token *APIToken, perm Permission) bool {
	for _, scope := range token.Scopes {
		perms := ScopeToPermissions(scope)
		for _, p := range perms {
			if p == perm {
				return true
			}
		}
	}
	return false
}
