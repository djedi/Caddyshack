package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupDashboardHandler(t *testing.T) *DashboardHandler {
	t.Helper()
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:2019", // Default Caddy admin API
	}

	// Pass nil for userStore since we're testing without database
	return NewDashboardHandler(tmpl, cfg, nil)
}

func TestDashboardHandler_ServeHTTP(t *testing.T) {
	handler := setupDashboardHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Errorf("Response should contain 'Dashboard', got: %s", body)
	}
}

func TestDashboardHandler_ServeHTTP_NonRootPath(t *testing.T) {
	handler := setupDashboardHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/some/other/path", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return 404 for non-root paths
	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-root path, got %d", rec.Code)
	}
}

func TestDashboardHandler_Status(t *testing.T) {
	handler := setupDashboardHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	handler.Status(rec, req)

	// Should return 200 OK even if Caddy is not running (status shows as unavailable)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Should render some status output (either running or unavailable)
	if body == "" {
		t.Error("Response should not be empty")
	}
}

func TestDashboardHandler_StatusHTMX(t *testing.T) {
	handler := setupDashboardHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Should NOT contain full HTML page structure for HTMX request
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("HTMX response should not contain full HTML document")
	}
}
