package auth

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// Create a temporary database file
	f, err := os.CreateTemp("", "caddyshack-test-*.db")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	dbPath := f.Name()
	f.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("opening database: %v", err)
	}

	// Run migrations
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			db.Close()
			os.Remove(dbPath)
			t.Fatalf("running migration: %v", err)
		}
	}

	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
	}

	return db, cleanup
}

func TestHashPassword(t *testing.T) {
	password := "testpassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("HashPassword returned empty hash")
	}

	if hash == password {
		t.Error("HashPassword returned plaintext password")
	}
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"
	wrongPassword := "wrongpassword"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !CheckPassword(password, hash) {
		t.Error("CheckPassword failed for correct password")
	}

	if CheckPassword(wrongPassword, hash) {
		t.Error("CheckPassword succeeded for wrong password")
	}
}

func TestRoleIsValid(t *testing.T) {
	tests := []struct {
		role  Role
		valid bool
	}{
		{RoleAdmin, true},
		{RoleEditor, true},
		{RoleViewer, true},
		{Role("invalid"), false},
		{Role(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.valid {
				t.Errorf("Role(%q).IsValid() = %v, want %v", tt.role, got, tt.valid)
			}
		})
	}
}

func TestUserStore_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "password123", RoleEditor)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("Create returned user with ID 0")
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "test@example.com")
	}
	if user.Role != RoleEditor {
		t.Errorf("Role = %q, want %q", user.Role, RoleEditor)
	}
}

func TestUserStore_CreateDuplicateUsername(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.Create("testuser", "test@example.com", "password123", RoleEditor)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	_, err = store.Create("testuser", "other@example.com", "password456", RoleViewer)
	if err != ErrUsernameExists {
		t.Errorf("Expected ErrUsernameExists, got %v", err)
	}
}

func TestUserStore_CreateInvalidRole(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.Create("testuser", "test@example.com", "password123", Role("invalid"))
	if err != ErrInvalidRole {
		t.Errorf("Expected ErrInvalidRole, got %v", err)
	}
}

func TestUserStore_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	created, err := store.Create("testuser", "test@example.com", "password123", RoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	user, err := store.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if user.ID != created.ID {
		t.Errorf("ID = %d, want %d", user.ID, created.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
}

func TestUserStore_GetByIDNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.GetByID(9999)
	if err != ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserStore_GetByUsername(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.Create("testuser", "test@example.com", "password123", RoleViewer)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	user, err := store.GetByUsername("testuser")
	if err != nil {
		t.Fatalf("GetByUsername failed: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
}

func TestUserStore_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.Create("alice", "alice@example.com", "password123", RoleAdmin)
	if err != nil {
		t.Fatalf("Create alice failed: %v", err)
	}
	_, err = store.Create("bob", "bob@example.com", "password123", RoleEditor)
	if err != nil {
		t.Fatalf("Create bob failed: %v", err)
	}

	users, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("List returned %d users, want 2", len(users))
	}

	// Should be sorted by username
	if users[0].Username != "alice" {
		t.Errorf("First user should be alice, got %q", users[0].Username)
	}
	if users[1].Username != "bob" {
		t.Errorf("Second user should be bob, got %q", users[1].Username)
	}
}

func TestUserStore_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "password123", RoleViewer)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.Update(user.ID, "updateduser", "updated@example.com", RoleEditor)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := store.GetByID(user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if updated.Username != "updateduser" {
		t.Errorf("Username = %q, want %q", updated.Username, "updateduser")
	}
	if updated.Email != "updated@example.com" {
		t.Errorf("Email = %q, want %q", updated.Email, "updated@example.com")
	}
	if updated.Role != RoleEditor {
		t.Errorf("Role = %q, want %q", updated.Role, RoleEditor)
	}
}

func TestUserStore_UpdatePassword(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "oldpassword", RoleViewer)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.UpdatePassword(user.ID, "newpassword")
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	// Old password should no longer work
	_, err = store.Authenticate("testuser", "oldpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials for old password, got %v", err)
	}

	// New password should work
	_, err = store.Authenticate("testuser", "newpassword")
	if err != nil {
		t.Errorf("Authenticate with new password failed: %v", err)
	}
}

func TestUserStore_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "password123", RoleViewer)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.Delete(user.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID(user.ID)
	if err != ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound after delete, got %v", err)
	}
}

func TestUserStore_Authenticate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	_, err := store.Create("testuser", "test@example.com", "correctpassword", RoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Correct credentials
	user, err := store.Authenticate("testuser", "correctpassword")
	if err != nil {
		t.Errorf("Authenticate with correct credentials failed: %v", err)
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}

	// Wrong password
	_, err = store.Authenticate("testuser", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials for wrong password, got %v", err)
	}

	// Wrong username
	_, err = store.Authenticate("wronguser", "correctpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials for wrong username, got %v", err)
	}
}

func TestUserStore_Session(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "password123", RoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create session
	session, err := store.CreateSession(user.ID)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.Token == "" {
		t.Error("CreateSession returned empty token")
	}
	if session.UserID != user.ID {
		t.Errorf("UserID = %d, want %d", session.UserID, user.ID)
	}

	// Validate session
	validatedUser, err := store.ValidateSession(session.Token)
	if err != nil {
		t.Fatalf("ValidateSession failed: %v", err)
	}
	if validatedUser.ID != user.ID {
		t.Errorf("ValidateSession returned wrong user: %d, want %d", validatedUser.ID, user.ID)
	}

	// Delete session
	err = store.DeleteSession(session.Token)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Session should no longer be valid
	_, err = store.ValidateSession(session.Token)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound after delete, got %v", err)
	}
}

func TestUserStore_CleanExpiredSessions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	user, err := store.Create("testuser", "test@example.com", "password123", RoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create a session that's already expired
	_, err = db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)`,
		user.ID, "expired_token", time.Now().Add(-1*time.Hour),
	)
	if err != nil {
		t.Fatalf("Creating expired session failed: %v", err)
	}

	// Create a valid session
	validSession, err := store.CreateSession(user.ID)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Clean expired sessions
	count, err := store.CleanExpiredSessions()
	if err != nil {
		t.Fatalf("CleanExpiredSessions failed: %v", err)
	}
	if count != 1 {
		t.Errorf("CleanExpiredSessions deleted %d sessions, want 1", count)
	}

	// Valid session should still work
	_, err = store.ValidateSession(validSession.Token)
	if err != nil {
		t.Errorf("ValidateSession for valid session failed: %v", err)
	}

	// Expired session should not be found
	_, err = store.GetSessionByToken("expired_token")
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for expired session, got %v", err)
	}
}

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role Role
		perm Permission
		want bool
	}{
		// Viewer permissions
		{RoleViewer, PermViewDashboard, true},
		{RoleViewer, PermViewSites, true},
		{RoleViewer, PermEditSites, false},
		{RoleViewer, PermManageUsers, false},

		// Editor permissions
		{RoleEditor, PermViewDashboard, true},
		{RoleEditor, PermViewSites, true},
		{RoleEditor, PermEditSites, true},
		{RoleEditor, PermEditGlobal, false},
		{RoleEditor, PermManageUsers, false},

		// Admin permissions
		{RoleAdmin, PermViewDashboard, true},
		{RoleAdmin, PermEditSites, true},
		{RoleAdmin, PermEditGlobal, true},
		{RoleAdmin, PermManageUsers, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"/"+string(tt.perm), func(t *testing.T) {
			if got := tt.role.HasPermission(tt.perm); got != tt.want {
				t.Errorf("Role(%q).HasPermission(%q) = %v, want %v", tt.role, tt.perm, got, tt.want)
			}
		})
	}
}

func TestRoleCanEdit(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{RoleAdmin, true},
		{RoleEditor, true},
		{RoleViewer, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanEdit(); got != tt.want {
				t.Errorf("Role(%q).CanEdit() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestUserStore_Count(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewUserStore(db)

	// Initially 0 users
	count, err := store.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Count = %d, want 0", count)
	}

	// Add a user
	_, err = store.Create("testuser", "test@example.com", "password123", RoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	count, err = store.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}
}
