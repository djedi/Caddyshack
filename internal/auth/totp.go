package auth

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	// TOTPIssuer is the issuer name shown in authenticator apps.
	TOTPIssuer = "Caddyshack"

	// BackupCodeCount is the number of backup codes to generate.
	BackupCodeCount = 10

	// BackupCodeLength is the length of each backup code (in characters).
	BackupCodeLength = 8
)

var (
	// ErrTOTPAlreadyEnabled is returned when trying to enable 2FA when it's already enabled.
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")

	// ErrTOTPNotEnabled is returned when trying to use 2FA when it's not enabled.
	ErrTOTPNotEnabled = errors.New("two-factor authentication is not enabled")

	// ErrInvalidTOTPCode is returned when a TOTP code is invalid.
	ErrInvalidTOTPCode = errors.New("invalid verification code")

	// ErrInvalidBackupCode is returned when a backup code is invalid.
	ErrInvalidBackupCode = errors.New("invalid backup code")

	// ErrNoBackupCodes is returned when there are no unused backup codes.
	ErrNoBackupCodes = errors.New("no backup codes available")
)

// TOTPSetup holds the information needed to set up 2FA.
type TOTPSetup struct {
	Secret     string
	QRCodeData string // Base64 encoded PNG
	URL        string
}

// BackupCode represents a backup code for account recovery.
type BackupCode struct {
	ID        int64
	UserID    int64
	CodeHash  string
	UsedAt    *time.Time
	CreatedAt time.Time
}

// GenerateTOTPSecret generates a new TOTP secret for a user.
func GenerateTOTPSecret(username string) (*TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
		AccountName: username,
		SecretSize:  32,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("generating TOTP key: %w", err)
	}

	// Generate QR code image
	img, err := key.Image(200, 200)
	if err != nil {
		return nil, fmt.Errorf("generating QR code: %w", err)
	}

	// Encode image to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding QR code PNG: %w", err)
	}

	return &TOTPSetup{
		Secret:     key.Secret(),
		QRCodeData: base64.StdEncoding.EncodeToString(buf.Bytes()),
		URL:        key.URL(),
	}, nil
}

// ValidateTOTPCode validates a TOTP code against a secret.
func ValidateTOTPCode(code, secret string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes generates a set of backup codes.
func GenerateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code, err := generateBackupCode()
		if err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		codes[i] = code
	}
	return codes, nil
}

// generateBackupCode generates a single backup code.
func generateBackupCode() (string, error) {
	// Generate random bytes
	b := make([]byte, BackupCodeLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Encode as base32 (uppercase letters and digits, easy to read)
	encoded := base32.StdEncoding.EncodeToString(b)

	// Take only the first BackupCodeLength characters and format with dashes
	code := encoded[:BackupCodeLength]

	// Format as XXXX-XXXX for readability
	return code[:4] + "-" + code[4:], nil
}

// HashBackupCode hashes a backup code for storage.
func HashBackupCode(code string) (string, error) {
	// Remove dashes for consistent hashing
	normalized := normalizeBackupCode(code)

	hash, err := bcrypt.GenerateFromPassword([]byte(normalized), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing backup code: %w", err)
	}
	return string(hash), nil
}

// CheckBackupCode compares a backup code with its hash.
func CheckBackupCode(code, hash string) bool {
	normalized := normalizeBackupCode(code)
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(normalized))
	return err == nil
}

// normalizeBackupCode removes dashes and converts to uppercase.
func normalizeBackupCode(code string) string {
	result := ""
	for _, c := range code {
		if c != '-' {
			// Convert to uppercase for case-insensitive matching
			if c >= 'a' && c <= 'z' {
				c = c - 'a' + 'A'
			}
			result += string(c)
		}
	}
	return result
}

// TOTPStore provides database operations for TOTP and backup codes.
type TOTPStore struct {
	db *sql.DB
}

// NewTOTPStore creates a new TOTPStore.
func NewTOTPStore(db *sql.DB) *TOTPStore {
	return &TOTPStore{db: db}
}

// GetTOTPStatus returns whether 2FA is enabled for a user and when it was enabled.
func (s *TOTPStore) GetTOTPStatus(userID int64) (enabled bool, secret string, verifiedAt *time.Time, err error) {
	var totpSecret string
	var totpEnabled bool
	var verifiedAtNullable sql.NullTime

	err = s.db.QueryRow(`
		SELECT totp_enabled, totp_secret, totp_verified_at
		FROM users WHERE id = ?
	`, userID).Scan(&totpEnabled, &totpSecret, &verifiedAtNullable)

	if err == sql.ErrNoRows {
		return false, "", nil, ErrUserNotFound
	}
	if err != nil {
		return false, "", nil, fmt.Errorf("getting TOTP status: %w", err)
	}

	if verifiedAtNullable.Valid {
		verifiedAt = &verifiedAtNullable.Time
	}

	return totpEnabled, totpSecret, verifiedAt, nil
}

// SetTOTPSecret sets the TOTP secret for a user (before verification).
func (s *TOTPStore) SetTOTPSecret(userID int64, secret string) error {
	result, err := s.db.Exec(`
		UPDATE users SET totp_secret = ? WHERE id = ?
	`, secret, userID)
	if err != nil {
		return fmt.Errorf("setting TOTP secret: %w", err)
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

// EnableTOTP enables 2FA for a user after they've verified the code.
func (s *TOTPStore) EnableTOTP(userID int64) error {
	result, err := s.db.Exec(`
		UPDATE users SET totp_enabled = 1, totp_verified_at = CURRENT_TIMESTAMP
		WHERE id = ? AND totp_secret != ''
	`, userID)
	if err != nil {
		return fmt.Errorf("enabling TOTP: %w", err)
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

// DisableTOTP disables 2FA for a user and clears their backup codes.
func (s *TOTPStore) DisableTOTP(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear TOTP fields
	_, err = tx.Exec(`
		UPDATE users SET totp_secret = '', totp_enabled = 0, totp_verified_at = NULL
		WHERE id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("disabling TOTP: %w", err)
	}

	// Delete all backup codes
	_, err = tx.Exec(`DELETE FROM user_backup_codes WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting backup codes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// SaveBackupCodes saves a set of backup codes for a user.
// It first deletes any existing backup codes.
func (s *TOTPStore) SaveBackupCodes(userID int64, codes []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing codes
	_, err = tx.Exec(`DELETE FROM user_backup_codes WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting existing backup codes: %w", err)
	}

	// Insert new codes
	for _, code := range codes {
		hash, err := HashBackupCode(code)
		if err != nil {
			return fmt.Errorf("hashing backup code: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO user_backup_codes (user_id, code_hash) VALUES (?, ?)
		`, userID, hash)
		if err != nil {
			return fmt.Errorf("inserting backup code: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ValidateBackupCode validates and marks a backup code as used.
func (s *TOTPStore) ValidateBackupCode(userID int64, code string) error {
	// Get unused backup codes
	rows, err := s.db.Query(`
		SELECT id, code_hash FROM user_backup_codes
		WHERE user_id = ? AND used_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("getting backup codes: %w", err)
	}

	// Collect all backup codes first
	type backupCode struct {
		id   int64
		hash string
	}
	var codes []backupCode
	for rows.Next() {
		var bc backupCode
		if err := rows.Scan(&bc.id, &bc.hash); err != nil {
			rows.Close()
			return fmt.Errorf("scanning backup code: %w", err)
		}
		codes = append(codes, bc)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating backup codes: %w", err)
	}

	// Check each code
	for _, bc := range codes {
		if CheckBackupCode(code, bc.hash) {
			// Mark as used
			_, err := s.db.Exec(`
				UPDATE user_backup_codes SET used_at = CURRENT_TIMESTAMP WHERE id = ?
			`, bc.id)
			if err != nil {
				return fmt.Errorf("marking backup code as used: %w", err)
			}
			return nil
		}
	}

	return ErrInvalidBackupCode
}

// GetBackupCodeCount returns the number of unused backup codes for a user.
func (s *TOTPStore) GetBackupCodeCount(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM user_backup_codes
		WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting backup codes: %w", err)
	}
	return count, nil
}

// HasBackupCodes returns true if the user has any unused backup codes.
func (s *TOTPStore) HasBackupCodes(userID int64) (bool, error) {
	count, err := s.GetBackupCodeCount(userID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
