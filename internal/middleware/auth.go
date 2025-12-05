package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "caddyshack_session"

	// SessionDuration is how long a session is valid.
	SessionDuration = 24 * time.Hour
)

// Session represents an authenticated user session.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// SessionStore manages authenticated sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a new session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// Create creates a new session and returns the token.
func (s *SessionStore) Create() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[token] = &Session{
		Token:     token,
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	return token, nil
}

// Valid checks if a session token is valid.
func (s *SessionStore) Valid(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[token]
	if !ok {
		return false
	}

	if time.Now().After(session.ExpiresAt) {
		// Session expired, clean it up
		go s.Delete(token)
		return false
	}

	return true
}

// Delete removes a session.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// CleanExpired removes all expired sessions.
func (s *SessionStore) CleanExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Auth holds authentication configuration.
type Auth struct {
	Username string
	Password string
	Sessions *SessionStore
}

// NewAuth creates a new Auth with the given credentials.
func NewAuth(username, password string) *Auth {
	return &Auth{
		Username: username,
		Password: password,
		Sessions: NewSessionStore(),
	}
}

// ValidateCredentials checks if the username and password are correct.
func (a *Auth) ValidateCredentials(username, password string) bool {
	// Use constant-time comparison to prevent timing attacks
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(a.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(a.Password)) == 1
	return userMatch && passMatch
}

// CreateSession creates a new authenticated session.
func (a *Auth) CreateSession() (string, error) {
	return a.Sessions.Create()
}

// ValidSession checks if the request has a valid session.
func (a *Auth) ValidSession(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return false
	}
	return a.Sessions.Valid(cookie.Value)
}

// DeleteSession removes the session from the request.
func (a *Auth) DeleteSession(r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return
	}
	a.Sessions.Delete(cookie.Value)
}

// Middleware returns an HTTP middleware that requires authentication.
// If credentials are not configured, it allows all requests through.
// It supports both session-based auth (cookie) and HTTP Basic Auth.
func (a *Auth) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no credentials configured, skip authentication
			if a.Username == "" || a.Password == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check for valid session cookie first
			if a.ValidSession(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Fall back to HTTP Basic Auth
			user, pass, ok := r.BasicAuth()
			if ok && a.ValidateCredentials(user, pass) {
				next.ServeHTTP(w, r)
				return
			}

			// Not authenticated - redirect to login page
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

// BasicAuth returns a middleware that requires HTTP Basic Authentication.
// If username or password are empty, the middleware allows all requests through.
// Deprecated: Use Auth.Middleware() instead for session support.
func BasicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no credentials configured, skip authentication
			if username == "" || password == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, pass, ok := r.BasicAuth()
			if !ok {
				unauthorized(w)
				return
			}

			// Use constant-time comparison to prevent timing attacks
			userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
			passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1

			if !userMatch || !passMatch {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Caddyshack"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
