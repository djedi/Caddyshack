package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupGlobalOptionsTestHandler(t *testing.T) (*GlobalOptionsHandler, string) {
	t.Helper()

	// Create a temporary directory for the Caddyfile
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	// Find the templates directory relative to the test file
	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		HistoryLimit:  50,
	}

	handler := NewGlobalOptionsHandler(tmpl, cfg, db)
	return handler, caddyfilePath
}

func TestGlobalOptionsList_NoGlobalOptions(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with no global options
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Global Options") {
		t.Errorf("Response should contain 'Global Options' page title")
	}
	if !strings.Contains(body, "No Global Options Configured") {
		t.Error("Response should indicate no global options are configured")
	}
}

func TestGlobalOptionsList_WithGlobalOptions(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with global options
	existingContent := `{
	email admin@example.com
	admin localhost:2019
	debug
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Global Options") {
		t.Error("Response should contain 'Global Options' page title")
	}
	if !strings.Contains(body, "admin@example.com") {
		t.Error("Response should contain the email")
	}
	if !strings.Contains(body, "localhost:2019") {
		t.Error("Response should contain the admin API address")
	}
	if !strings.Contains(body, "Enabled") {
		t.Error("Response should indicate debug mode is enabled")
	}
}

func TestGlobalOptionsList_WithLogging(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with logging configuration
	existingContent := `{
	email admin@example.com
	log {
		output file /var/log/caddy/access.log
		format json
		level info
	}
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Logging Configuration") {
		t.Error("Response should contain 'Logging Configuration' section")
	}
	if !strings.Contains(body, "json") {
		t.Error("Response should contain the log format")
	}
	if !strings.Contains(body, "info") {
		t.Error("Response should contain the log level")
	}
}

func TestGlobalOptionsList_WithACMECa(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with custom ACME CA
	existingContent := `{
	email admin@example.com
	acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "acme-staging") {
		t.Error("Response should contain the ACME CA URL")
	}
}

func TestGlobalOptionsList_AdminOff(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with admin off
	existingContent := `{
	admin off
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Disabled") {
		t.Error("Response should indicate admin API is disabled")
	}
}

func TestGlobalOptionsList_CaddyfileNotFound(t *testing.T) {
	handler, _ := setupGlobalOptionsTestHandler(t)

	// Don't create the Caddyfile - it should not exist

	req := httptest.NewRequest(http.MethodGet, "/global-options", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Caddyfile not found") {
		t.Error("Response should contain error about Caddyfile not found")
	}
}

func TestGlobalOptionsList_WithSuccessMessage(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a valid Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte("example.com {\n\treverse_proxy localhost:8080\n}"), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options?success=Settings+saved", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Settings saved") {
		t.Error("Response should contain success message")
	}
}

func TestGlobalOptionsList_WithReloadError(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a valid Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte("example.com {\n\treverse_proxy localhost:8080\n}"), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options?reload_error=Reload+failed", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Reload failed") {
		t.Error("Response should contain reload error message")
	}
}

func TestGlobalOptionsEdit_NoExistingConfig(t *testing.T) {
	handler, _ := setupGlobalOptionsTestHandler(t)

	// Don't create the Caddyfile - it should not exist

	req := httptest.NewRequest(http.MethodGet, "/global-options/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Edit Global Options") {
		t.Error("Response should contain 'Edit Global Options' page title")
	}
	// Form should be rendered with empty values
	if !strings.Contains(body, "ACME Email") {
		t.Error("Response should contain form fields")
	}
}

func TestGlobalOptionsEdit_WithExistingConfig(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with global options
	existingContent := `{
	email admin@example.com
	admin localhost:2019
	debug
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "admin@example.com") {
		t.Error("Response should contain pre-filled email value")
	}
	if !strings.Contains(body, "localhost:2019") {
		t.Error("Response should contain pre-filled admin value")
	}
}

func TestGlobalOptionsEdit_WithLogging(t *testing.T) {
	handler, caddyfilePath := setupGlobalOptionsTestHandler(t)

	// Create a Caddyfile with logging configuration
	existingContent := `{
	email admin@example.com
	log {
		output file /var/log/caddy/access.log
		format json
	}
}

example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/global-options/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "file /var/log/caddy/access.log") {
		t.Error("Response should contain pre-filled log output value")
	}
}
