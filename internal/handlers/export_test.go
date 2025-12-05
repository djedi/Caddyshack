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

func setupExportTestHandler(t *testing.T) (*ExportHandler, string) {
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

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: "http://localhost:2019",
	}

	handler := NewExportHandler(tmpl, cfg, s)
	return handler, caddyfilePath
}

func TestExportCaddyfile_Success(t *testing.T) {
	handler, caddyfilePath := setupExportTestHandler(t)

	// Create a Caddyfile with some content
	caddyfileContent := `example.com {
	reverse_proxy localhost:8080
}

test.example.com {
	reverse_proxy localhost:9090
}
`
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()

	handler.ExportCaddyfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check Content-Type header
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Expected Content-Type 'text/plain', got %q", contentType)
	}

	// Check Content-Disposition header for attachment
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") {
		t.Errorf("Expected Content-Disposition to contain 'attachment', got %q", contentDisposition)
	}
	if !strings.Contains(contentDisposition, "Caddyfile") {
		t.Errorf("Expected filename to contain 'Caddyfile', got %q", contentDisposition)
	}

	// Check that the body contains the Caddyfile content
	body := rec.Body.String()
	if !strings.Contains(body, "example.com") {
		t.Error("Response body should contain 'example.com'")
	}
	if !strings.Contains(body, "reverse_proxy") {
		t.Error("Response body should contain 'reverse_proxy'")
	}
}

func TestExportCaddyfile_FileNotFound(t *testing.T) {
	handler, _ := setupExportTestHandler(t)
	// Don't create the Caddyfile - it should not exist

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()

	handler.ExportCaddyfile(rec, req)

	// Should return an error status
	if rec.Code == http.StatusOK {
		t.Error("Expected non-200 status when Caddyfile doesn't exist")
	}
}

func TestExportCaddyfile_EmptyFile(t *testing.T) {
	handler, caddyfilePath := setupExportTestHandler(t)

	// Create an empty Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()

	handler.ExportCaddyfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Content-Length should be 0 for empty file
	contentLength := rec.Header().Get("Content-Length")
	if contentLength != "0" {
		t.Errorf("Expected Content-Length '0', got %q", contentLength)
	}
}

func TestExportCaddyfile_FilenameContainsDate(t *testing.T) {
	handler, caddyfilePath := setupExportTestHandler(t)

	// Create a Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte("# empty"), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()

	handler.ExportCaddyfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check that filename contains a date-like pattern
	contentDisposition := rec.Header().Get("Content-Disposition")
	// Date format is YYYY-MM-DD
	if !strings.Contains(contentDisposition, "-") {
		t.Error("Expected filename to contain a date with dashes")
	}
}

func TestExportJSON_CaddyNotRunning(t *testing.T) {
	// Create a temporary directory for the Caddyfile
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Use a port that's definitely not in use
	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: "http://127.0.0.1:59999", // Use an unlikely port
	}

	handler := NewExportHandler(tmpl, cfg, s)

	req := httptest.NewRequest(http.MethodGet, "/export/json", nil)
	rec := httptest.NewRecorder()

	handler.ExportJSON(rec, req)

	// Should return an error status since Caddy is not running
	if rec.Code == http.StatusOK {
		t.Error("Expected non-200 status when Caddy is not running")
	}
}

func TestExportJSON_Success(t *testing.T) {
	// Create a mock Caddy server
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"apps":{"http":{"servers":{"srv0":{"listen":[":443"]}}}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCaddy.Close()

	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
	}

	handler := NewExportHandler(tmpl, cfg, s)

	req := httptest.NewRequest(http.MethodGet, "/export/json", nil)
	rec := httptest.NewRecorder()

	handler.ExportJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check Content-Type header
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type 'application/json', got %q", contentType)
	}

	// Check Content-Disposition header for attachment
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") {
		t.Errorf("Expected Content-Disposition to contain 'attachment', got %q", contentDisposition)
	}
	if !strings.Contains(contentDisposition, "caddy-config") {
		t.Errorf("Expected filename to contain 'caddy-config', got %q", contentDisposition)
	}
	if !strings.Contains(contentDisposition, ".json") {
		t.Errorf("Expected filename to end with '.json', got %q", contentDisposition)
	}

	// Check that the body contains valid JSON
	body := rec.Body.String()
	if !strings.Contains(body, "apps") {
		t.Error("Response body should contain 'apps'")
	}
}

func TestExportBackup_Success(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Add some history entries
	if _, err := s.SaveConfig("# Version 1\nexample.com {\n\treverse_proxy localhost:8080\n}\n", "First version"); err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}
	if _, err := s.SaveConfig("# Version 2\nexample.com {\n\treverse_proxy localhost:9090\n}\n", "Second version"); err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Create a Caddyfile
	caddyfileContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: "http://localhost:2019",
	}

	handler := NewExportHandler(tmpl, cfg, s)

	req := httptest.NewRequest(http.MethodGet, "/export/backup", nil)
	rec := httptest.NewRecorder()

	handler.ExportBackup(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check Content-Type header
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/zip" {
		t.Errorf("Expected Content-Type 'application/zip', got %q", contentType)
	}

	// Check Content-Disposition header for attachment
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") {
		t.Errorf("Expected Content-Disposition to contain 'attachment', got %q", contentDisposition)
	}
	if !strings.Contains(contentDisposition, "caddyshack-backup") {
		t.Errorf("Expected filename to contain 'caddyshack-backup', got %q", contentDisposition)
	}
	if !strings.Contains(contentDisposition, ".zip") {
		t.Errorf("Expected filename to end with '.zip', got %q", contentDisposition)
	}

	// Verify it's a valid ZIP file
	body := rec.Body.Bytes()
	if len(body) < 4 {
		t.Error("Response body too short to be a valid ZIP file")
	}
	// ZIP files start with "PK" (0x50 0x4B)
	if body[0] != 0x50 || body[1] != 0x4B {
		t.Error("Response body is not a valid ZIP file (missing magic bytes)")
	}
}

func TestExportBackup_CaddyfileNotFound(t *testing.T) {
	handler, _ := setupExportTestHandler(t)
	// Don't create the Caddyfile - it should not exist

	req := httptest.NewRequest(http.MethodGet, "/export/backup", nil)
	rec := httptest.NewRecorder()

	handler.ExportBackup(rec, req)

	// Should return an error status
	if rec.Code == http.StatusOK {
		t.Error("Expected non-200 status when Caddyfile doesn't exist")
	}
}

func TestExportBackup_EmptyHistory(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Don't add any history entries - leave it empty

	// Create a Caddyfile
	caddyfileContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: "http://localhost:2019",
	}

	handler := NewExportHandler(tmpl, cfg, s)

	req := httptest.NewRequest(http.MethodGet, "/export/backup", nil)
	rec := httptest.NewRecorder()

	handler.ExportBackup(rec, req)

	// Should still succeed with empty history
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check Content-Type header
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/zip" {
		t.Errorf("Expected Content-Type 'application/zip', got %q", contentType)
	}
}
