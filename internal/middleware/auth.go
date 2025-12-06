package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/auth"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "caddyshack_session"

	// SessionDuration is how long a session is valid.
	SessionDuration = 24 * time.Hour
)

// Context key type for user context
type contextKey string

const (
	// UserContextKey is the context key for the authenticated user
	UserContextKey contextKey = "user"
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

// Context key for API token
const (
	// APITokenContextKey is the context key for the authenticated API token
	APITokenContextKey contextKey = "api_token"
)

// Auth holds authentication configuration.
// It supports both legacy single-user basic auth and multi-user database auth.
type Auth struct {
	// Legacy single-user auth
	Username string
	Password string
	Sessions *SessionStore

	// Multi-user database auth
	UserStore     *auth.UserStore
	TokenStore    *auth.TokenStore
	MultiUserMode bool
}

// NewAuth creates a new Auth with the given credentials (legacy mode).
func NewAuth(username, password string) *Auth {
	return &Auth{
		Username:      username,
		Password:      password,
		Sessions:      NewSessionStore(),
		MultiUserMode: false,
	}
}

// NewMultiUserAuth creates a new Auth with database-backed user management.
func NewMultiUserAuth(userStore *auth.UserStore) *Auth {
	return &Auth{
		UserStore:     userStore,
		Sessions:      NewSessionStore(), // Keep for legacy compatibility
		MultiUserMode: true,
	}
}

// SetTokenStore sets the token store for Bearer token authentication.
func (a *Auth) SetTokenStore(tokenStore *auth.TokenStore) {
	a.TokenStore = tokenStore
}

// ValidateCredentials checks if the username and password are correct.
// In multi-user mode, it validates against the database.
// In legacy mode, it validates against the configured credentials.
func (a *Auth) ValidateCredentials(username, password string) bool {
	if a.MultiUserMode && a.UserStore != nil {
		_, err := a.UserStore.Authenticate(username, password)
		return err == nil
	}

	// Legacy mode: use constant-time comparison to prevent timing attacks
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(a.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(a.Password)) == 1
	return userMatch && passMatch
}

// AuthenticateUser validates credentials and returns the user if valid.
// This is used in multi-user mode to get the user object for session creation.
func (a *Auth) AuthenticateUser(username, password string) (*auth.User, error) {
	if a.MultiUserMode && a.UserStore != nil {
		return a.UserStore.Authenticate(username, password)
	}

	// Legacy mode: create a fake admin user for compatibility
	if a.ValidateCredentials(username, password) {
		return &auth.User{
			ID:       0,
			Username: username,
			Role:     auth.RoleAdmin,
		}, nil
	}
	return nil, auth.ErrInvalidCredentials
}

// CreateSession creates a new authenticated session.
// In multi-user mode, it creates a database-backed session.
// In legacy mode, it creates an in-memory session.
func (a *Auth) CreateSession() (string, error) {
	return a.Sessions.Create()
}

// CreateUserSession creates a session for a specific user (multi-user mode).
func (a *Auth) CreateUserSession(userID int64) (string, error) {
	if a.MultiUserMode && a.UserStore != nil {
		session, err := a.UserStore.CreateSession(userID)
		if err != nil {
			return "", err
		}
		return session.Token, nil
	}
	// Fall back to legacy session
	return a.Sessions.Create()
}

// ValidSession checks if the request has a valid session.
func (a *Auth) ValidSession(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return false
	}

	if a.MultiUserMode && a.UserStore != nil {
		_, err := a.UserStore.ValidateSession(cookie.Value)
		return err == nil
	}

	return a.Sessions.Valid(cookie.Value)
}

// GetSessionUser returns the user for the current session.
// Returns nil if no valid session or in legacy mode.
func (a *Auth) GetSessionUser(r *http.Request) *auth.User {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil
	}

	if a.MultiUserMode && a.UserStore != nil {
		user, err := a.UserStore.ValidateSession(cookie.Value)
		if err == nil {
			return user
		}
	}

	// In legacy mode, check if session is valid and return a fake admin user
	if a.Sessions.Valid(cookie.Value) {
		return &auth.User{
			ID:       0,
			Username: a.Username,
			Role:     auth.RoleAdmin,
		}
	}

	return nil
}

// DeleteSession removes the session from the request.
func (a *Auth) DeleteSession(r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return
	}

	if a.MultiUserMode && a.UserStore != nil {
		_ = a.UserStore.DeleteSession(cookie.Value)
	}

	a.Sessions.Delete(cookie.Value)
}

// IsEnabled returns true if authentication is enabled.
func (a *Auth) IsEnabled() bool {
	if a.MultiUserMode {
		return a.UserStore != nil
	}
	return a.Username != "" && a.Password != ""
}

// Middleware returns an HTTP middleware that requires authentication.
// If credentials are not configured, it allows all requests through.
// It supports session-based auth (cookie), HTTP Basic Auth, and Bearer token auth.
func (a *Auth) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no credentials configured, skip authentication
			if !a.IsEnabled() {
				next.ServeHTTP(w, r)
				return
			}

			// Check for valid session cookie first
			if user := a.GetSessionUser(r); user != nil {
				// Add user to context
				ctx := context.WithValue(r.Context(), UserContextKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Check for Bearer token authentication
			if a.TokenStore != nil {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					apiToken, user, err := a.TokenStore.ValidateToken(token)
					if err == nil {
						// Add user and token to context
						ctx := context.WithValue(r.Context(), UserContextKey, user)
						ctx = context.WithValue(ctx, APITokenContextKey, apiToken)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
					// Invalid token - return 401 for API requests
					if isAPIRequest(r) {
						http.Error(w, "Invalid or expired API token", http.StatusUnauthorized)
						return
					}
				}
			}

			// Fall back to HTTP Basic Auth
			user, pass, ok := r.BasicAuth()
			if ok {
				authUser, err := a.AuthenticateUser(user, pass)
				if err == nil {
					// Add user to context
					ctx := context.WithValue(r.Context(), UserContextKey, authUser)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Not authenticated - redirect to login page for web, 401 for API
			if isAPIRequest(r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

// GetUserFromContext retrieves the authenticated user from the request context.
func GetUserFromContext(ctx context.Context) *auth.User {
	user, ok := ctx.Value(UserContextKey).(*auth.User)
	if !ok {
		return nil
	}
	return user
}

// RequireRole returns a middleware that requires a specific role.
func RequireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			for _, role := range roles {
				if user.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	}
}

// RequirePermission returns a middleware that requires a specific permission.
func RequirePermission(perm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.Role.HasPermission(perm) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
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

// isAPIRequest checks if the request is an API request based on headers or path.
func isAPIRequest(r *http.Request) bool {
	// Check for Accept: application/json header
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		return true
	}
	// Check for Authorization header with Bearer token
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}
	// Check for /api/ path prefix
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	return false
}

// GetAPITokenFromContext retrieves the API token from the request context.
func GetAPITokenFromContext(ctx context.Context) *auth.APIToken {
	token, ok := ctx.Value(APITokenContextKey).(*auth.APIToken)
	if !ok {
		return nil
	}
	return token
}

// RequireAPIScope returns a middleware that requires a specific API token scope.
func RequireAPIScope(scope auth.TokenScope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if authenticated via API token
			token := GetAPITokenFromContext(r.Context())
			if token != nil {
				// Using API token - check scope
				if !token.HasScope(scope) {
					http.Error(w, "Insufficient token permissions", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Not using API token - check user role permission
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Map scope to role permissions
			perms := auth.ScopeToPermissions(scope)
			hasPermission := false
			for _, perm := range perms {
				if user.Role.HasPermission(perm) {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
