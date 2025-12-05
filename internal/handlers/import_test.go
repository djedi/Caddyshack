package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupImportTestHandler(t *testing.T) (*ImportHandler, string, *store.Store) {
	t.Helper()

	// Create a temporary directory
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
		CaddyAdminAPI: "http://localhost:2019",
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)
	return handler, caddyfilePath, db
}

func TestImportPage_Display(t *testing.T) {
	handler, _, _ := setupImportTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/import", nil)
	rec := httptest.NewRecorder()

	handler.ImportPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Import Caddyfile") {
		t.Error("Response should contain 'Import Caddyfile'")
	}
	if !strings.Contains(body, "Upload File") {
		t.Error("Response should contain 'Upload File' tab")
	}
	if !strings.Contains(body, "Paste Content") {
		t.Error("Response should contain 'Paste Content' tab")
	}
}

func TestImportPage_WithSuccessMessage(t *testing.T) {
	handler, _, _ := setupImportTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/import?success=Import+applied+successfully", nil)
	rec := httptest.NewRecorder()

	handler.ImportPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Import applied successfully") {
		t.Error("Response should contain success message")
	}
}

func TestImportPage_WithErrorMessage(t *testing.T) {
	handler, _, _ := setupImportTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/import?error=Something+went+wrong", nil)
	rec := httptest.NewRecorder()

	handler.ImportPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Something went wrong") {
		t.Error("Response should contain error message")
	}
}

func TestPreview_PastedContent(t *testing.T) {
	// Create a mock Caddy server that validates successfully
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/adapt" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	// Create form data
	form := url.Values{}
	form.Add("content", `example.com {
	reverse_proxy localhost:8080
}`)

	req := httptest.NewRequest(http.MethodPost, "/import/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Import Preview") {
		t.Error("Response should contain 'Import Preview'")
	}
	if !strings.Contains(body, "example.com") {
		t.Error("Response should contain the site domain")
	}
	if !strings.Contains(body, "1") && !strings.Contains(body, "Sites") {
		t.Error("Response should show site count")
	}
}

func TestPreview_FileUpload(t *testing.T) {
	// Create a mock Caddy server
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/adapt" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	// Create multipart form with file upload
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	fileWriter, err := writer.CreateFormFile("caddyfile", "Caddyfile")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	caddyfileContent := `test.example.com {
	reverse_proxy localhost:9000
}
`
	fileWriter.Write([]byte(caddyfileContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/import/preview", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Import Preview") {
		t.Error("Response should contain 'Import Preview'")
	}
	if !strings.Contains(body, "test.example.com") {
		t.Error("Response should contain the site domain from uploaded file")
	}
}

func TestPreview_EmptyContent(t *testing.T) {
	handler, _, _ := setupImportTestHandler(t)

	form := url.Values{}
	form.Add("content", "")

	req := httptest.NewRequest(http.MethodPost, "/import/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Preview(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "No content provided") {
		t.Error("Response should contain error about empty content")
	}
}

func TestPreview_InvalidSyntax(t *testing.T) {
	// Create a mock Caddy server that rejects invalid config
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/adapt" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "invalid syntax"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	form := url.Values{}
	form.Add("content", "invalid { caddyfile content")

	req := httptest.NewRequest(http.MethodPost, "/import/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Preview(rec, req)

	// Preview should still render, just with a validation warning
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Validation Warning") && !strings.Contains(body, "Import Preview") {
		t.Error("Response should contain either validation warning or import preview")
	}
}

func TestApply_Success(t *testing.T) {
	// Create a mock Caddy server
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adapt":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		case "/load":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	// Apply import
	newContent := `new.example.com {
	reverse_proxy localhost:8080
}
`
	form := url.Values{}
	form.Add("content", newContent)

	req := httptest.NewRequest(http.MethodPost, "/import/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Apply(rec, req)

	// Should redirect
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 (redirect), got %d", rec.Code)
	}

	// Check redirect location contains success
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "success") {
		t.Errorf("Expected redirect to contain 'success', got %q", location)
	}

	// Verify the file was written
	writtenContent, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if string(writtenContent) != newContent {
		t.Errorf("Expected Caddyfile content %q, got %q", newContent, string(writtenContent))
	}
}

func TestApply_WithExistingConfig(t *testing.T) {
	// Create a mock Caddy server
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adapt":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		case "/load":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	// Create existing Caddyfile
	existingContent := `old.example.com {
	reverse_proxy localhost:3000
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	// Apply import
	newContent := `new.example.com {
	reverse_proxy localhost:8080
}
`
	form := url.Values{}
	form.Add("content", newContent)

	req := httptest.NewRequest(http.MethodPost, "/import/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Apply(rec, req)

	// Should redirect
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 (redirect), got %d", rec.Code)
	}

	// Verify old config was saved to history
	history, err := db.ListConfigs(10)
	if err != nil {
		t.Fatalf("Failed to get config history: %v", err)
	}

	if len(history) == 0 {
		t.Error("Expected config history to contain old config")
	} else {
		found := false
		for _, entry := range history {
			if entry.Content == existingContent {
				found = true
				break
			}
		}
		if !found {
			t.Error("Old config should have been saved to history")
		}
	}

	// Verify new content was written
	writtenContent, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if string(writtenContent) != newContent {
		t.Errorf("Expected Caddyfile content %q, got %q", newContent, string(writtenContent))
	}
}

func TestApply_EmptyContent(t *testing.T) {
	handler, _, _ := setupImportTestHandler(t)

	form := url.Values{}
	form.Add("content", "")

	req := httptest.NewRequest(http.MethodPost, "/import/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Apply(rec, req)

	// Should redirect with error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 (redirect), got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "error") {
		t.Errorf("Expected redirect to contain 'error', got %q", location)
	}
}

func TestApply_ValidationFails(t *testing.T) {
	// Create a mock Caddy server that rejects validation
	mockCaddy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/adapt" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "invalid config"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCaddy.Close()

	// Setup handler with mock Caddy
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	templatesDir := "../../templates"
	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		CaddyAdminAPI: mockCaddy.URL,
		HistoryLimit:  50,
	}

	handler := NewImportHandler(tmpl, cfg, db)

	form := url.Values{}
	form.Add("content", "invalid content")

	req := httptest.NewRequest(http.MethodPost, "/import/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Apply(rec, req)

	// Should redirect with error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status 303 (redirect), got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "error") {
		t.Errorf("Expected redirect to contain 'error', got %q", location)
	}

	// Verify file was NOT written
	if _, err := os.Stat(caddyfilePath); err == nil {
		t.Error("Caddyfile should not have been created when validation fails")
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"it's", "it&#39;s"},
		{"<a href=\"test\">", "&lt;a href=&quot;test&quot;&gt;"},
	}

	for _, tc := range tests {
		result := escapeHTML(tc.input)
		if result != tc.expected {
			t.Errorf("escapeHTML(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}
