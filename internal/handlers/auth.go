package handlers

import (
	"net/http"

	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// AuthHandler handles login and logout requests.
type AuthHandler struct {
	tmpl *templates.Templates
	auth *middleware.Auth
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(tmpl *templates.Templates, auth *middleware.Auth) *AuthHandler {
	return &AuthHandler{
		tmpl: tmpl,
		auth: auth,
	}
}

// LoginData holds data for the login page.
type LoginData struct {
	Error string
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

	if !h.auth.ValidateCredentials(username, password) {
		h.renderLoginError(w, "Invalid username or password")
		return
	}

	// Create session
	token, err := h.auth.CreateSession()
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
		Secure:   r.TLS != nil, // Set Secure flag if using HTTPS
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
