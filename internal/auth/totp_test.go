package auth

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	_ "modernc.org/sqlite"
)

func createTestTOTPStore(t *testing.T) (*TOTPStore, *sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Set busy timeout to avoid locking issues in tests
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		t.Fatalf("Failed to set busy timeout: %v", err)
	}

	// Create required tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			totp_secret TEXT NOT NULL DEFAULT '',
			totp_enabled BOOLEAN NOT NULL DEFAULT 0,
			totp_verified_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		);
		CREATE TABLE IF NOT EXISTS user_backup_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			code_hash TEXT NOT NULL,
			used_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	store := NewTOTPStore(db)
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return store, db, cleanup
}

func TestGenerateTOTPSecret(t *testing.T) {
	setup, err := GenerateTOTPSecret("testuser")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error = %v", err)
	}

	if setup.Secret == "" {
		t.Error("GenerateTOTPSecret() returned empty secret")
	}

	if setup.QRCodeData == "" {
		t.Error("GenerateTOTPSecret() returned empty QR code data")
	}

	if setup.URL == "" {
		t.Error("GenerateTOTPSecret() returned empty URL")
	}
}

func TestValidateTOTPCode(t *testing.T) {
	setup, err := GenerateTOTPSecret("testuser")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error = %v", err)
	}

	// Generate a valid code
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("Failed to generate TOTP code: %v", err)
	}

	if !ValidateTOTPCode(code, setup.Secret) {
		t.Error("ValidateTOTPCode() returned false for valid code")
	}

	if ValidateTOTPCode("000000", setup.Secret) {
		t.Error("ValidateTOTPCode() returned true for invalid code")
	}
}

func TestGenerateBackupCodes(t *testing.T) {
	codes, err := GenerateBackupCodes(10)
	if err != nil {
		t.Fatalf("GenerateBackupCodes() error = %v", err)
	}

	if len(codes) != 10 {
		t.Errorf("GenerateBackupCodes() returned %d codes, want 10", len(codes))
	}

	// Check format (XXXX-XXXX)
	for i, code := range codes {
		if len(code) != 9 {
			t.Errorf("Code %d has length %d, want 9", i, len(code))
		}
		if code[4] != '-' {
			t.Errorf("Code %d missing dash at position 4", i)
		}
	}
}

func TestHashAndCheckBackupCode(t *testing.T) {
	code := "ABCD-EFGH"

	hash, err := HashBackupCode(code)
	if err != nil {
		t.Fatalf("HashBackupCode() error = %v", err)
	}

	if hash == "" {
		t.Error("HashBackupCode() returned empty hash")
	}

	if !CheckBackupCode(code, hash) {
		t.Error("CheckBackupCode() returned false for valid code")
	}

	if !CheckBackupCode("abcd-efgh", hash) {
		t.Error("CheckBackupCode() returned false for lowercase valid code")
	}

	if CheckBackupCode("XXXX-YYYY", hash) {
		t.Error("CheckBackupCode() returned true for invalid code")
	}
}

func TestTOTPStore_SetAndGetStatus(t *testing.T) {
	store, db, cleanup := createTestTOTPStore(t)
	defer cleanup()

	// Create a test user
	result, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		"testuser", "hash", "viewer")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID, _ := result.LastInsertId()

	// Initially, TOTP should not be enabled
	enabled, secret, verifiedAt, err := store.GetTOTPStatus(userID)
	if err != nil {
		t.Fatalf("GetTOTPStatus() error = %v", err)
	}
	if enabled {
		t.Error("GetTOTPStatus() returned enabled=true for new user")
	}
	if secret != "" {
		t.Error("GetTOTPStatus() returned non-empty secret for new user")
	}
	if verifiedAt != nil {
		t.Error("GetTOTPStatus() returned non-nil verifiedAt for new user")
	}

	// Set a secret
	testSecret := "ABCDEFGHIJKLMNOP"
	if err := store.SetTOTPSecret(userID, testSecret); err != nil {
		t.Fatalf("SetTOTPSecret() error = %v", err)
	}

	// Check that secret is set but not enabled
	enabled, secret, _, err = store.GetTOTPStatus(userID)
	if err != nil {
		t.Fatalf("GetTOTPStatus() error = %v", err)
	}
	if enabled {
		t.Error("GetTOTPStatus() returned enabled=true after SetTOTPSecret")
	}
	if secret != testSecret {
		t.Errorf("GetTOTPStatus() secret = %s, want %s", secret, testSecret)
	}

	// Enable TOTP
	if err := store.EnableTOTP(userID); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}

	// Check that it's now enabled
	enabled, _, verifiedAt, err = store.GetTOTPStatus(userID)
	if err != nil {
		t.Fatalf("GetTOTPStatus() error = %v", err)
	}
	if !enabled {
		t.Error("GetTOTPStatus() returned enabled=false after EnableTOTP")
	}
	if verifiedAt == nil {
		t.Error("GetTOTPStatus() returned nil verifiedAt after EnableTOTP")
	}

	// Disable TOTP
	if err := store.DisableTOTP(userID); err != nil {
		t.Fatalf("DisableTOTP() error = %v", err)
	}

	// Check that it's disabled and secret is cleared
	enabled, secret, verifiedAt, err = store.GetTOTPStatus(userID)
	if err != nil {
		t.Fatalf("GetTOTPStatus() error = %v", err)
	}
	if enabled {
		t.Error("GetTOTPStatus() returned enabled=true after DisableTOTP")
	}
	if secret != "" {
		t.Error("GetTOTPStatus() returned non-empty secret after DisableTOTP")
	}
	if verifiedAt != nil {
		t.Error("GetTOTPStatus() returned non-nil verifiedAt after DisableTOTP")
	}
}

func TestTOTPStore_BackupCodes(t *testing.T) {
	store, db, cleanup := createTestTOTPStore(t)
	defer cleanup()

	// Create a test user
	result, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		"testuser", "hash", "viewer")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID, _ := result.LastInsertId()

	// Generate and save backup codes
	codes, err := GenerateBackupCodes(5)
	if err != nil {
		t.Fatalf("GenerateBackupCodes() error = %v", err)
	}

	if err := store.SaveBackupCodes(userID, codes); err != nil {
		t.Fatalf("SaveBackupCodes() error = %v", err)
	}

	// Check count
	count, err := store.GetBackupCodeCount(userID)
	if err != nil {
		t.Fatalf("GetBackupCodeCount() error = %v", err)
	}
	if count != 5 {
		t.Errorf("GetBackupCodeCount() = %d, want 5", count)
	}

	// Validate a code
	if err := store.ValidateBackupCode(userID, codes[0]); err != nil {
		t.Fatalf("ValidateBackupCode() error = %v", err)
	}

	// Count should be reduced
	count, err = store.GetBackupCodeCount(userID)
	if err != nil {
		t.Fatalf("GetBackupCodeCount() error = %v", err)
	}
	if count != 4 {
		t.Errorf("GetBackupCodeCount() = %d after use, want 4", count)
	}

	// Same code should not work again
	err = store.ValidateBackupCode(userID, codes[0])
	if err == nil {
		t.Error("ValidateBackupCode() should fail for already used code")
	}

	// Invalid code should not work
	err = store.ValidateBackupCode(userID, "XXXX-YYYY")
	if err == nil {
		t.Error("ValidateBackupCode() should fail for invalid code")
	}
}

func TestTOTPStore_DisableClearsBackupCodes(t *testing.T) {
	store, db, cleanup := createTestTOTPStore(t)
	defer cleanup()

	// Create a test user
	result, err := db.Exec(`INSERT INTO users (username, password_hash, role, totp_secret) VALUES (?, ?, ?, ?)`,
		"testuser", "hash", "viewer", "secret")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID, _ := result.LastInsertId()

	// Enable TOTP
	if err := store.EnableTOTP(userID); err != nil {
		t.Fatalf("EnableTOTP() error = %v", err)
	}

	// Save some backup codes
	codes, _ := GenerateBackupCodes(5)
	if err := store.SaveBackupCodes(userID, codes); err != nil {
		t.Fatalf("SaveBackupCodes() error = %v", err)
	}

	// Verify codes exist
	count, _ := store.GetBackupCodeCount(userID)
	if count != 5 {
		t.Errorf("Expected 5 backup codes, got %d", count)
	}

	// Disable TOTP
	if err := store.DisableTOTP(userID); err != nil {
		t.Fatalf("DisableTOTP() error = %v", err)
	}

	// Verify codes are deleted
	count, _ = store.GetBackupCodeCount(userID)
	if count != 0 {
		t.Errorf("Expected 0 backup codes after disable, got %d", count)
	}
}
