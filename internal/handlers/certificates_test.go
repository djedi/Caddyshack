package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

func TestNewCertificatesHandler(t *testing.T) {
	// Create a mock templates instance
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Skipf("Templates not available for testing: %v", err)
	}

	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:2019",
	}

	handler := NewCertificatesHandler(tmpl, cfg)

	if handler == nil {
		t.Error("Expected non-nil handler")
	}
	if handler.templates == nil {
		t.Error("Expected templates to be set")
	}
	if handler.adminClient == nil {
		t.Error("Expected adminClient to be set")
	}
}

func TestCertificatesHandler_List_CaddyNotReachable(t *testing.T) {
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Skipf("Templates not available for testing: %v", err)
	}

	// Use an unreachable URL
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:9999",
	}

	handler := NewCertificatesHandler(tmpl, cfg)

	req := httptest.NewRequest(http.MethodGet, "/certificates", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Caddy Not Reachable") {
		t.Error("Expected 'Caddy Not Reachable' message in response")
	}
}

func TestCertificatesHandler_Widget_CaddyNotReachable(t *testing.T) {
	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Skipf("Templates not available for testing: %v", err)
	}

	// Use an unreachable URL
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:9999",
	}

	handler := NewCertificatesHandler(tmpl, cfg)

	req := httptest.NewRequest(http.MethodGet, "/certificates/widget", nil)
	w := httptest.NewRecorder()

	handler.Widget(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html', got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Caddy not reachable") {
		t.Error("Expected 'Caddy not reachable' message in response")
	}
}

func TestCertificateToView(t *testing.T) {
	tests := []struct {
		name          string
		status        string
		expectedColor string
	}{
		{"valid", "valid", "green"},
		{"expiring", "expiring", "yellow"},
		{"expired", "expired", "red"},
		{"unknown", "unknown", "gray"},
		{"empty", "", "gray"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := caddy.CertificateInfo{
				Domain: "example.com",
				Issuer: "Test CA",
				Status: tt.status,
			}

			view := certificateToView(cert)

			if view.Domain != "example.com" {
				t.Errorf("Expected domain 'example.com', got %s", view.Domain)
			}
			if view.StatusColor != tt.expectedColor {
				t.Errorf("Expected color '%s', got '%s'", tt.expectedColor, view.StatusColor)
			}
		})
	}
}
