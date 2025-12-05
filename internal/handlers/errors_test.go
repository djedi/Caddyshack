package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/templates"
)

func setupErrorHandler(t *testing.T) *ErrorHandler {
	t.Helper()
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}
	return NewErrorHandler(tmpl)
}

func TestNotFound(t *testing.T) {
	handler := setupErrorHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.NotFound(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Page Not Found") {
		t.Errorf("Response should contain 'Page Not Found', got: %s", body)
	}
}

func TestNotFound_HTMX(t *testing.T) {
	handler := setupErrorHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.NotFound(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	// HTMX requests should return an error message partial, not a full page
	if !strings.Contains(body, "Page Not Found") {
		t.Errorf("Response should contain 'Page Not Found', got: %s", body)
	}
	// Should not include the full page layout
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("HTMX response should not contain full HTML page")
	}
}

func TestInternalServerError(t *testing.T) {
	handler := setupErrorHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()

	testErr := errors.New("test database error")
	handler.InternalServerError(rec, req, testErr)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Internal Server Error") {
		t.Errorf("Response should contain 'Internal Server Error', got: %s", body)
	}
}

func TestBadRequest(t *testing.T) {
	handler := setupErrorHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/data", nil)
	rec := httptest.NewRecorder()

	handler.BadRequest(rec, req, "Invalid JSON format")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Bad Request") {
		t.Errorf("Response should contain 'Bad Request', got: %s", body)
	}
	if !strings.Contains(body, "Invalid JSON format") {
		t.Errorf("Response should contain custom message, got: %s", body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	handler := setupErrorHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/readonly-endpoint", nil)
	rec := httptest.NewRecorder()

	handler.MethodNotAllowed(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Method Not Allowed") {
		t.Errorf("Response should contain 'Method Not Allowed', got: %s", body)
	}
}

func TestHTTPError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	HTTPError(rec, req, http.StatusForbidden, "Access denied")

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rec.Code)
	}
}

func TestHTTPError_HTMX(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	HTTPError(rec, req, http.StatusForbidden, "Access denied")

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Access denied") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
	if !strings.Contains(body, "bg-red-50") {
		t.Errorf("HTMX response should contain styled error div, got: %s", body)
	}
}
