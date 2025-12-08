package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupAuthHandler(t *testing.T) (*AuthHandler, *middleware.Auth) {
	t.Helper()
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	auth := middleware.NewAuth("admin", "password123")
	return NewAuthHandler(tmpl, auth), auth
}

func TestAuthHandler_LoginPage(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	handler.LoginPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Sign In") {
		t.Errorf("Response should contain 'Sign In', got: %s", body)
	}
}

func TestAuthHandler_LoginPage_AlreadyAuthenticated(t *testing.T) {
	handler, auth := setupAuthHandler(t)

	// Create a valid session
	token, err := auth.CreateSession()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{
		Name:  middleware.SessionCookieName,
		Value: token,
	})
	rec := httptest.NewRecorder()

	handler.LoginPage(rec, req)

	// Should redirect to dashboard
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/" {
		t.Errorf("Expected redirect to '/', got: %s", rec.Header().Get("Location"))
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	// Should redirect to dashboard on success
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/" {
		t.Errorf("Expected redirect to '/', got: %s", rec.Header().Get("Location"))
	}

	// Should set session cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == middleware.SessionCookieName {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	} else {
		if sessionCookie.Value == "" {
			t.Error("Session cookie should have a value")
		}
		if !sessionCookie.HttpOnly {
			t.Error("Session cookie should be HttpOnly")
		}
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "wrongpassword")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Invalid username or password") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestAuthHandler_Login_MissingCredentials(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	form := url.Values{}
	// Missing username and password

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	handler, auth := setupAuthHandler(t)

	// Create a valid session first
	token, err := auth.CreateSession()
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  middleware.SessionCookieName,
		Value: token,
	})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	// Should redirect to login page
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/login" {
		t.Errorf("Expected redirect to '/login', got: %s", rec.Header().Get("Location"))
	}

	// Should clear session cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == middleware.SessionCookieName {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set (for deletion)")
	} else {
		if sessionCookie.MaxAge != -1 {
			t.Errorf("Session cookie should be expired (MaxAge=-1), got: %d", sessionCookie.MaxAge)
		}
	}

	// Session should no longer be valid
	if auth.Sessions.Valid(token) {
		t.Error("Session should no longer be valid after logout")
	}
}

func TestAuthHandler_Logout_NoSession(t *testing.T) {
	handler, _ := setupAuthHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	// No session cookie
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	// Should still redirect to login page
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/login" {
		t.Errorf("Expected redirect to '/login', got: %s", rec.Header().Get("Location"))
	}
}
