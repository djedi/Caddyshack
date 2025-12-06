package handlers

import (
	"log"
	"net/http"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// TOTPSetupData holds data for the 2FA setup page.
type TOTPSetupData struct {
	QRCodeData      string
	Secret          string
	BackupCodes     []string
	Error           string
	Success         string
	TOTPEnabled     bool
	BackupCodeCount int
}

// TOTPHandler handles two-factor authentication requests.
type TOTPHandler struct {
	templates    *templates.Templates
	config       *config.Config
	userStore    *auth.UserStore
	totpStore    *auth.TOTPStore
	errorHandler *ErrorHandler
}

// NewTOTPHandler creates a new TOTPHandler.
func NewTOTPHandler(tmpl *templates.Templates, cfg *config.Config, userStore *auth.UserStore, totpStore *auth.TOTPStore) *TOTPHandler {
	return &TOTPHandler{
		templates:    tmpl,
		config:       cfg,
		userStore:    userStore,
		totpStore:    totpStore,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// Setup shows the 2FA setup page.
func (h *TOTPHandler) Setup(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Check if 2FA is already enabled
	enabled, _, _, err := h.totpStore.GetTOTPStatus(user.ID)
	if err != nil && err != auth.ErrUserNotFound {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	if enabled {
		// Show the already enabled page with option to disable
		backupCount, _ := h.totpStore.GetBackupCodeCount(user.ID)
		data := TOTPSetupData{
			TOTPEnabled:     true,
			BackupCodeCount: backupCount,
		}
		pageData := WithPermissionsAndConfig(r, h.config, "Two-Factor Authentication", "profile", data)
		if err := h.templates.Render(w, "totp-setup.html", pageData); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// Generate a new TOTP secret
	setup, err := auth.GenerateTOTPSecret(user.Username)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Save the secret (not yet verified)
	if err := h.totpStore.SetTOTPSecret(user.ID, setup.Secret); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	data := TOTPSetupData{
		QRCodeData:  setup.QRCodeData,
		Secret:      setup.Secret,
		TOTPEnabled: false,
	}
	pageData := WithPermissionsAndConfig(r, h.config, "Set Up Two-Factor Authentication", "profile", data)
	if err := h.templates.Render(w, "totp-setup.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Verify verifies the TOTP code and enables 2FA.
func (h *TOTPHandler) Verify(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderSetupError(w, r, user, "Failed to parse form data")
		return
	}

	code := r.FormValue("code")
	if code == "" {
		h.renderSetupError(w, r, user, "Verification code is required")
		return
	}

	// Get the pending secret
	enabled, secret, _, err := h.totpStore.GetTOTPStatus(user.ID)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	if enabled {
		h.renderSetupError(w, r, user, "Two-factor authentication is already enabled")
		return
	}

	if secret == "" {
		h.renderSetupError(w, r, user, "Please generate a QR code first")
		return
	}

	// Validate the code
	if !auth.ValidateTOTPCode(code, secret) {
		h.renderSetupError(w, r, user, "Invalid verification code. Please try again.")
		return
	}

	// Generate backup codes
	backupCodes, err := auth.GenerateBackupCodes(auth.BackupCodeCount)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Save backup codes
	if err := h.totpStore.SaveBackupCodes(user.ID, backupCodes); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Enable TOTP
	if err := h.totpStore.EnableTOTP(user.ID); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Show backup codes to user (they need to save them)
	data := TOTPSetupData{
		TOTPEnabled:     true,
		BackupCodes:     backupCodes,
		BackupCodeCount: len(backupCodes),
		Success:         "Two-factor authentication has been enabled. Save your backup codes below.",
	}
	pageData := WithPermissionsAndConfig(r, h.config, "Two-Factor Authentication Enabled", "profile", data)
	if err := h.templates.Render(w, "totp-setup.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Disable disables 2FA for the current user.
func (h *TOTPHandler) Disable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderSetupError(w, r, user, "Failed to parse form data")
		return
	}

	password := r.FormValue("password")
	if password == "" {
		h.renderSetupError(w, r, user, "Password is required to disable 2FA")
		return
	}

	// Verify password
	_, err := h.userStore.Authenticate(user.Username, password)
	if err != nil {
		h.renderSetupError(w, r, user, "Incorrect password")
		return
	}

	// Disable TOTP
	if err := h.totpStore.DisableTOTP(user.ID); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Redirect to setup page with success message
	http.Redirect(w, r, "/profile/2fa?disabled=1", http.StatusFound)
}

// RegenerateBackupCodes regenerates backup codes for the user.
func (h *TOTPHandler) RegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderSetupError(w, r, user, "Failed to parse form data")
		return
	}

	password := r.FormValue("password")
	if password == "" {
		h.renderSetupError(w, r, user, "Password is required to regenerate backup codes")
		return
	}

	// Verify password
	_, err := h.userStore.Authenticate(user.Username, password)
	if err != nil {
		h.renderSetupError(w, r, user, "Incorrect password")
		return
	}

	// Check if 2FA is enabled
	enabled, _, _, err := h.totpStore.GetTOTPStatus(user.ID)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	if !enabled {
		h.renderSetupError(w, r, user, "Two-factor authentication is not enabled")
		return
	}

	// Generate new backup codes
	backupCodes, err := auth.GenerateBackupCodes(auth.BackupCodeCount)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Save backup codes
	if err := h.totpStore.SaveBackupCodes(user.ID, backupCodes); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Show new backup codes
	data := TOTPSetupData{
		TOTPEnabled:     true,
		BackupCodes:     backupCodes,
		BackupCodeCount: len(backupCodes),
		Success:         "Your backup codes have been regenerated. Save your new codes below.",
	}
	pageData := WithPermissionsAndConfig(r, h.config, "New Backup Codes", "profile", data)
	if err := h.templates.Render(w, "totp-setup.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// AdminDisable allows an admin to disable 2FA for another user.
func (h *TOTPHandler) AdminDisable(w http.ResponseWriter, r *http.Request, targetUserID int64) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		h.errorHandler.Unauthorized(w, r)
		return
	}

	// Check admin permission
	if !user.Role.HasPermission(auth.PermManageUsers) {
		h.errorHandler.Forbidden(w, r)
		return
	}

	// Disable TOTP for target user
	if err := h.totpStore.DisableTOTP(targetUserID); err != nil {
		if err == auth.ErrUserNotFound {
			h.errorHandler.NotFound(w, r)
			return
		}
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Return success for HTMX
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="text-green-600 dark:text-green-400">2FA disabled</span>`))
}

// renderSetupError re-renders the setup page with an error.
func (h *TOTPHandler) renderSetupError(w http.ResponseWriter, r *http.Request, user *auth.User, errMsg string) {
	// Check current status
	enabled, secret, _, _ := h.totpStore.GetTOTPStatus(user.ID)

	data := TOTPSetupData{
		TOTPEnabled: enabled,
		Error:       errMsg,
	}

	if !enabled && secret != "" {
		// Regenerate QR code for the pending secret
		setup, err := auth.GenerateTOTPSecret(user.Username)
		if err != nil {
			log.Printf("Error generating TOTP QR: %v", err)
		} else {
			// Use the existing secret, just regenerate QR
			data.QRCodeData = setup.QRCodeData
			data.Secret = secret
		}
	} else if enabled {
		backupCount, _ := h.totpStore.GetBackupCodeCount(user.ID)
		data.BackupCodeCount = backupCount
	}

	pageData := WithPermissionsAndConfig(r, h.config, "Two-Factor Authentication", "profile", data)
	w.WriteHeader(http.StatusBadRequest)
	if err := h.templates.Render(w, "totp-setup.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}
