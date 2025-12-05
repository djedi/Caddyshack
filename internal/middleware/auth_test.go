package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBasicAuth(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	tests := []struct {
		name           string
		username       string
		password       string
		reqUser        string
		reqPass        string
		hasAuth        bool
		expectedStatus int
	}{
		{
			name:           "no auth configured - allows request",
			username:       "",
			password:       "",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "auth configured - correct credentials",
			username:       "admin",
			password:       "secret",
			reqUser:        "admin",
			reqPass:        "secret",
			hasAuth:        true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "auth configured - wrong username",
			username:       "admin",
			password:       "secret",
			reqUser:        "wrong",
			reqPass:        "secret",
			hasAuth:        true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "auth configured - wrong password",
			username:       "admin",
			password:       "secret",
			reqUser:        "admin",
			reqPass:        "wrong",
			hasAuth:        true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "auth configured - no credentials provided",
			username:       "admin",
			password:       "secret",
			hasAuth:        false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "only username configured - allows request",
			username:       "admin",
			password:       "",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "only password configured - allows request",
			username:       "",
			password:       "secret",
			hasAuth:        false,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := BasicAuth(tt.username, tt.password)
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.hasAuth {
				req.SetBasicAuth(tt.reqUser, tt.reqPass)
			}

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				wwwAuth := rec.Header().Get("WWW-Authenticate")
				if wwwAuth == "" {
					t.Error("expected WWW-Authenticate header on 401 response")
				}
			}
		})
	}
}

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	// Test session creation
	token, err := store.Create()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Test session validation
	if !store.Valid(token) {
		t.Error("expected session to be valid")
	}

	// Test invalid token
	if store.Valid("invalid-token") {
		t.Error("expected invalid token to be invalid")
	}

	// Test session deletion
	store.Delete(token)
	if store.Valid(token) {
		t.Error("expected deleted session to be invalid")
	}
}

func TestAuth(t *testing.T) {
	auth := NewAuth("admin", "secret")

	// Test credential validation
	if !auth.ValidateCredentials("admin", "secret") {
		t.Error("expected valid credentials to be accepted")
	}
	if auth.ValidateCredentials("admin", "wrong") {
		t.Error("expected wrong password to be rejected")
	}
	if auth.ValidateCredentials("wrong", "secret") {
		t.Error("expected wrong username to be rejected")
	}

	// Test session creation and validation
	token, err := auth.CreateSession()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	})

	if !auth.ValidSession(req) {
		t.Error("expected session to be valid")
	}

	// Test session deletion
	auth.DeleteSession(req)
	if auth.ValidSession(req) {
		t.Error("expected session to be invalid after deletion")
	}
}

func TestAuthMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	t.Run("no auth configured - allows request", func(t *testing.T) {
		auth := NewAuth("", "")
		middleware := auth.Middleware()
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("valid session cookie - allows request", func(t *testing.T) {
		auth := NewAuth("admin", "secret")
		token, _ := auth.CreateSession()

		middleware := auth.Middleware()
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  SessionCookieName,
			Value: token,
		})
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("valid basic auth - allows request", func(t *testing.T) {
		auth := NewAuth("admin", "secret")

		middleware := auth.Middleware()
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("no credentials - redirects to login", func(t *testing.T) {
		auth := NewAuth("admin", "secret")

		middleware := auth.Middleware()
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected redirect (302), got %d", rec.Code)
		}
		location := rec.Header().Get("Location")
		if location != "/login" {
			t.Errorf("expected redirect to /login, got %s", location)
		}
	})

	t.Run("invalid credentials - redirects to login", func(t *testing.T) {
		auth := NewAuth("admin", "secret")

		middleware := auth.Middleware()
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("wrong", "wrong")
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("expected redirect (302), got %d", rec.Code)
		}
	})
}

func TestCleanExpiredSessions(t *testing.T) {
	store := NewSessionStore()

	// Create a session with expired time by directly manipulating the store
	store.mu.Lock()
	store.sessions["expired-token"] = &Session{
		Token:     "expired-token",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.sessions["valid-token"] = &Session{
		Token:     "valid-token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	store.mu.Unlock()

	// Clean expired sessions
	store.CleanExpired()

	// Verify expired session is removed
	if store.Valid("expired-token") {
		t.Error("expected expired token to be removed")
	}

	// Verify valid session is still present
	if !store.Valid("valid-token") {
		t.Error("expected valid token to still be valid")
	}
}
