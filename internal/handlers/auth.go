package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/auth"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

const (
	// TwoFactorCookieName is the cookie name for pending 2FA verification.
	TwoFactorCookieName = "caddyshack_2fa_pending"

	// TwoFactorTokenExpiry is how long a pending 2FA token is valid.
	TwoFactorTokenExpiry = 5 * time.Minute
)

// pendingAuth holds information about a pending 2FA authentication.
type pendingAuth struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

// pendingAuthStore stores pending 2FA authentications.
type pendingAuthStore struct {
	mu      sync.RWMutex
	pending map[string]*pendingAuth
}

// newPendingAuthStore creates a new pending auth store.
func newPendingAuthStore() *pendingAuthStore {
	return &pendingAuthStore{
		pending: make(map[string]*pendingAuth),
	}
}

// Create creates a new pending auth token.
func (s *pendingAuthStore) Create(userID int64, username string) (string, error) {
	token, err := generatePendingToken()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending[token] = &pendingAuth{
		UserID:    userID,
		Username:  username,
		ExpiresAt: time.Now().Add(TwoFactorTokenExpiry),
	}

	return token, nil
}

// Get retrieves and removes a pending auth by token.
func (s *pendingAuthStore) Get(token string) (*pendingAuth, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.pending[token]
	if !ok {
		return nil, false
	}

	// Remove the token (one-time use)
	delete(s.pending, token)

	// Check expiry
	if time.Now().After(pending.ExpiresAt) {
		return nil, false
	}

	return pending, true
}

// CleanExpired removes expired pending auths.
func (s *pendingAuthStore) CleanExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, pending := range s.pending {
		if now.After(pending.ExpiresAt) {
			delete(s.pending, token)
		}
	}
}

func generatePendingToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// AuthHandler handles login and logout requests.
type AuthHandler struct {
	tmpl         *templates.Templates
	auth         *middleware.Auth
	totpStore    *auth.TOTPStore
	pendingStore *pendingAuthStore
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(tmpl *templates.Templates, authMW *middleware.Auth) *AuthHandler {
	return &AuthHandler{
		tmpl:         tmpl,
		auth:         authMW,
		pendingStore: newPendingAuthStore(),
	}
}

// SetTOTPStore sets the TOTP store for 2FA support.
func (h *AuthHandler) SetTOTPStore(store *auth.TOTPStore) {
	h.totpStore = store
}

// LoginData holds data for the login page.
type LoginData struct {
	Error          string
	Show2FA        bool
	PendingToken   string
	ShowBackupCode bool
}

// LoginPage renders the login form.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard
	if h.auth.ValidSession(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := templates.PageData{
		Title: "Login",
		Data:  LoginData{},
	}
	if err := h.tmpl.Render(w, "login.html", data); err != nil {
		http.Error(w, "Failed to render login page", http.StatusInternalServerError)
	}
}

// Login handles the login form submission.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginError(w, "Invalid form data")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	// Authenticate user
	user, err := h.auth.AuthenticateUser(username, password)
	if err != nil {
		h.renderLoginError(w, "Invalid username or password")
		return
	}

	// Check if 2FA is enabled for this user
	if h.totpStore != nil && h.auth.MultiUserMode {
		totpEnabled, _, _, _ := h.totpStore.GetTOTPStatus(user.ID)
		if totpEnabled {
			// Create pending auth token
			pendingToken, err := h.pendingStore.Create(user.ID, user.Username)
			if err != nil {
				h.renderLoginError(w, "Failed to initiate 2FA verification")
				return
			}

			// Set pending auth cookie
			http.SetCookie(w, &http.Cookie{
				Name:     TwoFactorCookieName,
				Value:    pendingToken,
				Path:     "/",
				MaxAge:   int(TwoFactorTokenExpiry.Seconds()),
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteStrictMode,
			})

			// Render 2FA verification page
			h.render2FAPage(w, pendingToken, "", false)
			return
		}
	}

	// No 2FA required, complete login
	h.completeLogin(w, r, user)
}

// Verify2FA handles the 2FA code verification.
func (h *AuthHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginError(w, "Invalid form data")
		return
	}

	// Get pending token from form or cookie
	pendingToken := r.FormValue("pending_token")
	if pendingToken == "" {
		if cookie, err := r.Cookie(TwoFactorCookieName); err == nil {
			pendingToken = cookie.Value
		}
	}

	if pendingToken == "" {
		h.renderLoginError(w, "Session expired. Please login again.")
		return
	}

	// Get pending auth
	pending, ok := h.pendingStore.Get(pendingToken)
	if !ok {
		// Clear cookie
		http.SetCookie(w, &http.Cookie{
			Name:     TwoFactorCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
		h.renderLoginError(w, "Session expired. Please login again.")
		return
	}

	code := r.FormValue("code")
	useBackupCode := r.FormValue("use_backup_code") == "1"

	if code == "" {
		// Put the pending auth back (we consumed it)
		newToken, _ := h.pendingStore.Create(pending.UserID, pending.Username)
		h.render2FAPage(w, newToken, "Verification code is required", useBackupCode)
		return
	}

	// Check if using backup code
	var valid bool
	if useBackupCode {
		err := h.totpStore.ValidateBackupCode(pending.UserID, code)
		valid = err == nil
	} else {
		// Get TOTP secret
		_, secret, _, err := h.totpStore.GetTOTPStatus(pending.UserID)
		if err != nil {
			h.renderLoginError(w, "Failed to verify code")
			return
		}
		valid = auth.ValidateTOTPCode(code, secret)
	}

	if !valid {
		// Put the pending auth back (allow retry)
		newToken, _ := h.pendingStore.Create(pending.UserID, pending.Username)
		if useBackupCode {
			h.render2FAPage(w, newToken, "Invalid backup code", true)
		} else {
			h.render2FAPage(w, newToken, "Invalid verification code", false)
		}
		return
	}

	// Clear 2FA cookie
	http.SetCookie(w, &http.Cookie{
		Name:     TwoFactorCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Create a mock user object for completing login
	user := &auth.User{
		ID:       pending.UserID,
		Username: pending.Username,
		Role:     auth.RoleViewer, // Will be updated when session is validated
	}

	// Complete login
	h.completeLogin(w, r, user)
}

// completeLogin finishes the login process by creating a session and setting the cookie.
func (h *AuthHandler) completeLogin(w http.ResponseWriter, r *http.Request, user *auth.User) {
	var token string
	var err error

	if h.auth.MultiUserMode {
		// In multi-user mode, create a database-backed session
		token, err = h.auth.CreateUserSession(user.ID)
	} else {
		// In legacy mode, create an in-memory session
		token, err = h.auth.CreateSession()
	}
	if err != nil {
		h.renderLoginError(w, "Failed to create session")
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(middleware.SessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to dashboard
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout logs out the user and redirects to login page.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Delete session from store
	h.auth.DeleteSession(r)

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Clear any pending 2FA cookie
	http.SetCookie(w, &http.Cookie{
		Name:     TwoFactorCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (h *AuthHandler) renderLoginError(w http.ResponseWriter, errMsg string) {
	data := templates.PageData{
		Title: "Login",
		Data:  LoginData{Error: errMsg},
	}
	w.WriteHeader(http.StatusUnauthorized)
	if err := h.tmpl.Render(w, "login.html", data); err != nil {
		http.Error(w, "Failed to render login page", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) render2FAPage(w http.ResponseWriter, pendingToken, errMsg string, showBackupCode bool) {
	data := templates.PageData{
		Title: "Two-Factor Authentication",
		Data: LoginData{
			Show2FA:        true,
			PendingToken:   pendingToken,
			Error:          errMsg,
			ShowBackupCode: showBackupCode,
		},
	}
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	if err := h.tmpl.Render(w, "login.html", data); err != nil {
		http.Error(w, "Failed to render login page", http.StatusInternalServerError)
	}
}
