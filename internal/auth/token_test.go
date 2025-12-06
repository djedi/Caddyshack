package auth

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTokenTestDB(t *testing.T) (*sql.DB, *TokenStore, func()) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("failed to create users table: %v", err)
	}

	// Create api_tokens table
	_, err = db.Exec(`
		CREATE TABLE api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			scopes TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			last_used_at DATETIME,
			revoked_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		t.Fatalf("failed to create api_tokens table: %v", err)
	}

	// Create a test user
	hash, _ := HashPassword("testpass")
	_, err = db.Exec(
		"INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, ?)",
		"testuser", "test@example.com", hash, "admin",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	tokenStore := NewTokenStore(db)

	return db, tokenStore, func() {
		db.Close()
	}
}

func TestTokenCreate(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Create a token
	rawToken, token, err := store.Create(1, "test-token", []TokenScope{ScopeRead}, nil)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Check the raw token has the correct prefix
	if len(rawToken) < len(TokenPrefix) {
		t.Errorf("raw token is too short: %s", rawToken)
	}
	if rawToken[:len(TokenPrefix)] != TokenPrefix {
		t.Errorf("raw token doesn't have correct prefix: %s", rawToken)
	}

	// Check the token record
	if token.ID == 0 {
		t.Error("token ID should not be 0")
	}
	if token.UserID != 1 {
		t.Errorf("expected user ID 1, got %d", token.UserID)
	}
	if token.Name != "test-token" {
		t.Errorf("expected name 'test-token', got %s", token.Name)
	}
	if len(token.Scopes) != 1 || token.Scopes[0] != ScopeRead {
		t.Errorf("expected scopes [read], got %v", token.Scopes)
	}
	if token.ExpiresAt != nil {
		t.Error("expected no expiration")
	}
	if token.IsRevoked() {
		t.Error("token should not be revoked")
	}
	if token.IsExpired() {
		t.Error("token should not be expired")
	}
	if !token.IsValid() {
		t.Error("token should be valid")
	}
}

func TestTokenCreateWithExpiration(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	expiresAt := time.Now().Add(time.Hour)
	_, token, err := store.Create(1, "expiring-token", []TokenScope{ScopeWrite}, &expiresAt)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	if token.ExpiresAt == nil {
		t.Error("expected expiration time to be set")
	}
	if !token.IsValid() {
		t.Error("token should be valid")
	}
}

func TestTokenCreateDuplicateName(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	_, _, err := store.Create(1, "duplicate-token", []TokenScope{ScopeRead}, nil)
	if err != nil {
		t.Fatalf("failed to create first token: %v", err)
	}

	// Try to create another token with the same name
	_, _, err = store.Create(1, "duplicate-token", []TokenScope{ScopeRead}, nil)
	if err != ErrTokenNameExists {
		t.Errorf("expected ErrTokenNameExists, got %v", err)
	}
}

func TestTokenValidate(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	rawToken, _, err := store.Create(1, "validate-token", []TokenScope{ScopeAdmin}, nil)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Validate the token
	token, user, err := store.ValidateToken(rawToken)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if token.Name != "validate-token" {
		t.Errorf("expected name 'validate-token', got %s", token.Name)
	}
	if user.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %s", user.Username)
	}
}

func TestTokenValidateInvalid(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Try to validate an invalid token
	_, _, err := store.ValidateToken("csk_invalid_token_here")
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}

	// Try to validate a token without the prefix
	_, _, err = store.ValidateToken("no_prefix_token")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestTokenValidateExpired(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Create an already-expired token
	expiresAt := time.Now().Add(-time.Hour)
	rawToken, _, err := store.Create(1, "expired-token", []TokenScope{ScopeRead}, &expiresAt)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Try to validate the expired token
	_, _, err = store.ValidateToken(rawToken)
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenRevoke(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	rawToken, token, err := store.Create(1, "revoke-token", []TokenScope{ScopeRead}, nil)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Revoke the token
	err = store.Revoke(token.ID)
	if err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	// Try to validate the revoked token
	_, _, err = store.ValidateToken(rawToken)
	if err != ErrTokenRevoked {
		t.Errorf("expected ErrTokenRevoked, got %v", err)
	}
}

func TestTokenList(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Create multiple tokens
	_, _, _ = store.Create(1, "token-1", []TokenScope{ScopeRead}, nil)
	_, _, _ = store.Create(1, "token-2", []TokenScope{ScopeWrite}, nil)
	_, _, _ = store.Create(1, "token-3", []TokenScope{ScopeAdmin}, nil)

	tokens, err := store.ListByUser(1)
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestTokenListActive(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Create active and revoked tokens
	_, _, _ = store.Create(1, "active-token", []TokenScope{ScopeRead}, nil)
	_, token2, _ := store.Create(1, "revoked-token", []TokenScope{ScopeRead}, nil)
	_ = store.Revoke(token2.ID)

	// Create an expired token
	expiresAt := time.Now().Add(-time.Hour)
	_, _, _ = store.Create(1, "expired-token", []TokenScope{ScopeRead}, &expiresAt)

	tokens, err := store.ListActiveByUser(1)
	if err != nil {
		t.Fatalf("failed to list active tokens: %v", err)
	}

	if len(tokens) != 1 {
		t.Errorf("expected 1 active token, got %d", len(tokens))
	}
	if tokens[0].Name != "active-token" {
		t.Errorf("expected 'active-token', got %s", tokens[0].Name)
	}
}

func TestTokenDelete(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	_, token, err := store.Create(1, "delete-token", []TokenScope{ScopeRead}, nil)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	err = store.Delete(token.ID)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	// Verify it's gone
	_, err = store.GetByID(token.ID)
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestTokenScopes(t *testing.T) {
	tests := []struct {
		scopes   []TokenScope
		hasRead  bool
		hasWrite bool
		hasAdmin bool
	}{
		{[]TokenScope{ScopeRead}, true, false, false},
		{[]TokenScope{ScopeWrite}, false, true, false},
		{[]TokenScope{ScopeAdmin}, true, true, true}, // Admin includes all
		{[]TokenScope{ScopeRead, ScopeWrite}, true, true, false},
	}

	for _, tt := range tests {
		token := &APIToken{Scopes: tt.scopes}

		if got := token.HasScope(ScopeRead); got != tt.hasRead {
			t.Errorf("HasScope(read) for %v: got %v, want %v", tt.scopes, got, tt.hasRead)
		}
		if got := token.HasScope(ScopeWrite); got != tt.hasWrite {
			t.Errorf("HasScope(write) for %v: got %v, want %v", tt.scopes, got, tt.hasWrite)
		}
		if got := token.HasScope(ScopeAdmin); got != tt.hasAdmin {
			t.Errorf("HasScope(admin) for %v: got %v, want %v", tt.scopes, got, tt.hasAdmin)
		}
	}
}

func TestTokenScopeValidity(t *testing.T) {
	if !ScopeRead.IsValid() {
		t.Error("ScopeRead should be valid")
	}
	if !ScopeWrite.IsValid() {
		t.Error("ScopeWrite should be valid")
	}
	if !ScopeAdmin.IsValid() {
		t.Error("ScopeAdmin should be valid")
	}
	if TokenScope("invalid").IsValid() {
		t.Error("'invalid' scope should not be valid")
	}
}

func TestTokenHasWriteAccess(t *testing.T) {
	tests := []struct {
		scopes   []TokenScope
		hasWrite bool
	}{
		{[]TokenScope{ScopeRead}, false},
		{[]TokenScope{ScopeWrite}, true},
		{[]TokenScope{ScopeAdmin}, true},
		{[]TokenScope{ScopeRead, ScopeWrite}, true},
	}

	for _, tt := range tests {
		token := &APIToken{Scopes: tt.scopes}
		if got := token.HasWriteAccess(); got != tt.hasWrite {
			t.Errorf("HasWriteAccess for %v: got %v, want %v", tt.scopes, got, tt.hasWrite)
		}
	}
}

func TestTokenHasAdminAccess(t *testing.T) {
	tests := []struct {
		scopes   []TokenScope
		hasAdmin bool
	}{
		{[]TokenScope{ScopeRead}, false},
		{[]TokenScope{ScopeWrite}, false},
		{[]TokenScope{ScopeAdmin}, true},
		{[]TokenScope{ScopeRead, ScopeWrite}, false},
	}

	for _, tt := range tests {
		token := &APIToken{Scopes: tt.scopes}
		if got := token.HasAdminAccess(); got != tt.hasAdmin {
			t.Errorf("HasAdminAccess for %v: got %v, want %v", tt.scopes, got, tt.hasAdmin)
		}
	}
}

func TestScopeToPermissions(t *testing.T) {
	readPerms := ScopeToPermissions(ScopeRead)
	if len(readPerms) == 0 {
		t.Error("ScopeRead should have permissions")
	}

	writePerms := ScopeToPermissions(ScopeWrite)
	if len(writePerms) == 0 {
		t.Error("ScopeWrite should have permissions")
	}

	adminPerms := ScopeToPermissions(ScopeAdmin)
	if len(adminPerms) == 0 {
		t.Error("ScopeAdmin should have permissions")
	}

	// Admin should have more permissions than write
	if len(adminPerms) <= len(writePerms) {
		t.Error("ScopeAdmin should have more permissions than ScopeWrite")
	}

	// Invalid scope should return nil
	invalidPerms := ScopeToPermissions(TokenScope("invalid"))
	if invalidPerms != nil {
		t.Error("invalid scope should return nil permissions")
	}
}

func TestTokenHasPermission(t *testing.T) {
	token := &APIToken{Scopes: []TokenScope{ScopeRead}}

	if !TokenHasPermission(token, PermViewSites) {
		t.Error("token with read scope should have PermViewSites")
	}

	if TokenHasPermission(token, PermEditSites) {
		t.Error("token with read scope should not have PermEditSites")
	}

	adminToken := &APIToken{Scopes: []TokenScope{ScopeAdmin}}
	if !TokenHasPermission(adminToken, PermManageUsers) {
		t.Error("token with admin scope should have PermManageUsers")
	}
}

func TestCountByUser(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	count, err := store.CountByUser(1)
	if err != nil {
		t.Fatalf("failed to count tokens: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens, got %d", count)
	}

	// Create some tokens
	_, _, _ = store.Create(1, "token-1", []TokenScope{ScopeRead}, nil)
	_, _, _ = store.Create(1, "token-2", []TokenScope{ScopeWrite}, nil)

	count, err = store.CountByUser(1)
	if err != nil {
		t.Fatalf("failed to count tokens: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 tokens, got %d", count)
	}
}

func TestRevokeAllForUser(t *testing.T) {
	_, store, cleanup := setupTokenTestDB(t)
	defer cleanup()

	// Create multiple tokens
	_, _, _ = store.Create(1, "token-1", []TokenScope{ScopeRead}, nil)
	_, _, _ = store.Create(1, "token-2", []TokenScope{ScopeWrite}, nil)
	_, _, _ = store.Create(1, "token-3", []TokenScope{ScopeAdmin}, nil)

	count, err := store.RevokeAllForUser(1)
	if err != nil {
		t.Fatalf("failed to revoke all tokens: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 tokens revoked, got %d", count)
	}

	// Verify all tokens are revoked
	activeTokens, err := store.ListActiveByUser(1)
	if err != nil {
		t.Fatalf("failed to list active tokens: %v", err)
	}
	if len(activeTokens) != 0 {
		t.Errorf("expected 0 active tokens, got %d", len(activeTokens))
	}
}
