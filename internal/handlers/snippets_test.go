package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupSnippetsTestHandler(t *testing.T) (*SnippetsHandler, string) {
	t.Helper()

	// Create a temporary directory for the Caddyfile and database
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	// Find the templates directory relative to the test file
	templatesDir := "../../templates"

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		HistoryLimit:  50,
	}

	// Initialize the store for testing
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
	})

	handler := NewSnippetsHandler(tmpl, cfg, s)
	return handler, caddyfilePath
}

// snippetCaddyAvailable checks if the caddy binary is available in PATH
func snippetCaddyAvailable() bool {
	_, err := exec.LookPath("caddy")
	return err == nil
}

func TestSnippetCreate_Valid(t *testing.T) {
	if !snippetCaddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Test creating a snippet
	form := url.Values{}
	form.Set("name", "site_log")
	form.Set("content", `log {
	output file /var/log/caddy/access.log
	format json
}`)

	req := httptest.NewRequest(http.MethodPost, "/snippets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	// Check for HX-Redirect header (indicates success)
	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/snippets") {
		t.Errorf("Expected HX-Redirect to /snippets, got %q", redirect)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was created with the snippet
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "(site_log)") {
		t.Error("Caddyfile should contain '(site_log)'")
	}
	if !strings.Contains(string(content), "log") {
		t.Error("Caddyfile should contain 'log'")
	}
}

func TestSnippetCreate_MissingName(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	form := url.Values{}
	form.Set("content", "log { format json }")

	req := httptest.NewRequest(http.MethodPost, "/snippets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	// Should NOT redirect on error
	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	// Should return HTML with error message
	body := rec.Body.String()
	if !strings.Contains(body, "name is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestSnippetCreate_MissingContent(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	form := url.Values{}
	form.Set("name", "my_snippet")
	form.Set("content", "")

	req := httptest.NewRequest(http.MethodPost, "/snippets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "content is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestSnippetCreate_InvalidName(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	form := url.Values{}
	form.Set("name", "invalid-name-with-dashes")
	form.Set("content", "log { format json }")

	req := httptest.NewRequest(http.MethodPost, "/snippets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Invalid snippet name") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestSnippetCreate_DuplicateSnippet(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create an existing Caddyfile with a snippet
	existingContent := `(site_log) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to create a snippet with the same name
	form := url.Values{}
	form.Set("name", "site_log")
	form.Set("content", "log { format console }")

	req := httptest.NewRequest(http.MethodPost, "/snippets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when snippet already exists")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "already exists") {
		t.Errorf("Response should contain error message about duplicate, got: %s", body)
	}
}

func TestSnippetUpdate_Valid(t *testing.T) {
	if !snippetCaddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create an existing Caddyfile with a snippet
	existingContent := `(site_log) {
	log {
		format json
	}
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Update the snippet
	form := url.Values{}
	form.Set("name", "site_log")
	form.Set("content", `log {
	format console
}`)

	req := httptest.NewRequest(http.MethodPut, "/snippets/site_log", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/snippets") {
		t.Errorf("Expected HX-Redirect to /snippets, got %q", redirect)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was updated
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "console") {
		t.Error("Caddyfile should contain 'console' after update")
	}
}

func TestSnippetUpdate_NotFound(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create an existing Caddyfile with a different snippet
	existingContent := `(other_snippet) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to update a non-existent snippet
	form := url.Values{}
	form.Set("name", "nonexistent")
	form.Set("content", "log { format console }")

	req := httptest.NewRequest(http.MethodPut, "/snippets/nonexistent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when snippet not found")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "not found") {
		t.Errorf("Response should contain 'not found', got: %s", body)
	}
}

func TestSnippetDelete_Valid(t *testing.T) {
	if !snippetCaddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create an existing Caddyfile with multiple snippets
	existingContent := `(site_log) {
	log { format json }
}

(proxy_headers) {
	header X-Proxied-By "Caddy"
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Delete site_log snippet
	req := httptest.NewRequest(http.MethodDelete, "/snippets/site_log", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	redirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(redirect, "/snippets") {
		t.Errorf("Expected HX-Redirect to /snippets, got %q", redirect)
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was updated
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if strings.Contains(string(content), "(site_log)") {
		t.Error("Caddyfile should NOT contain '(site_log)' after delete")
	}
	if !strings.Contains(string(content), "(proxy_headers)") {
		t.Error("Caddyfile should still contain '(proxy_headers)' after delete")
	}
}

func TestSnippetDelete_NotFound(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create an existing Caddyfile with a different snippet
	existingContent := `(other_snippet) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to delete a non-existent snippet
	req := httptest.NewRequest(http.MethodDelete, "/snippets/nonexistent", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestSnippetDelete_EmptyName(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	// Try to delete with empty name
	req := httptest.NewRequest(http.MethodDelete, "/snippets/", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestSnippetList_NoSnippets(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with no snippets
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Snippets") {
		t.Errorf("Response should contain 'Snippets' page title")
	}
}

func TestSnippetList_WithSnippets(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with snippets
	existingContent := `(site_log) {
	log { format json }
}

(proxy_headers) {
	header X-Proxied-By "Caddy"
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "site_log") {
		t.Error("Response should contain 'site_log'")
	}
	if !strings.Contains(body, "proxy_headers") {
		t.Error("Response should contain 'proxy_headers'")
	}
}

func TestSnippetList_WithSuccessMessage(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a valid Caddyfile
	if err := os.WriteFile(caddyfilePath, []byte("(test) {\n\tlog\n}"), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets?success=Snippet+created", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Snippet created") {
		t.Error("Response should contain success message")
	}
}

func TestSnippetDetail_Success(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with a snippet
	existingContent := `(site_log) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets/site_log", nil)
	rec := httptest.NewRecorder()

	handler.Detail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "site_log") {
		t.Error("Response should contain 'site_log'")
	}
}

func TestSnippetDetail_NotFound(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with a different snippet
	existingContent := `(other_snippet) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.Detail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestSnippetDetail_EmptyName(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/snippets/", nil)
	rec := httptest.NewRecorder()

	handler.Detail(rec, req)

	// Should redirect to snippets list
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/snippets" {
		t.Errorf("Expected redirect to /snippets, got %q", rec.Header().Get("Location"))
	}
}

func TestSnippetNew_Success(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/snippets/new", nil)
	rec := httptest.NewRecorder()

	handler.New(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Add") {
		t.Error("Response should contain 'Add'")
	}
}

func TestSnippetEdit_Success(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with a snippet
	existingContent := `(site_log) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets/site_log/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "site_log") {
		t.Error("Response should contain 'site_log'")
	}
}

func TestSnippetEdit_NotFound(t *testing.T) {
	handler, caddyfilePath := setupSnippetsTestHandler(t)

	// Create a Caddyfile with a different snippet
	existingContent := `(other_snippet) {
	log { format json }
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/snippets/nonexistent/edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "not found") {
		t.Errorf("Response should contain 'not found', got: %s", body)
	}
}

func TestSnippetEdit_EmptyName(t *testing.T) {
	handler, _ := setupSnippetsTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/snippets//edit", nil)
	rec := httptest.NewRecorder()

	handler.Edit(rec, req)

	// Should redirect to snippets list
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}
}

func TestIsValidSnippetName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"site_log", true},
		{"my_snippet", true},
		{"_private", true},
		{"MySnippet", true},
		{"snippet123", true},
		{"_123", true},
		{"123snippet", false}, // Can't start with number
		{"my-snippet", false}, // Dashes not allowed
		{"my.snippet", false}, // Dots not allowed
		{"my snippet", false}, // Spaces not allowed
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSnippetName(tt.name)
			if result != tt.valid {
				t.Errorf("isValidSnippetName(%q) = %v, want %v", tt.name, result, tt.valid)
			}
		})
	}
}

func TestGetSnippetPreview(t *testing.T) {
	tests := []struct {
		name     string
		snippet  caddy.Snippet
		expected string
	}{
		{
			name: "empty snippet",
			snippet: caddy.Snippet{
				Name:       "empty",
				Directives: nil,
			},
			expected: "(empty)",
		},
		{
			name: "single directive",
			snippet: caddy.Snippet{
				Name: "test",
				Directives: []caddy.Directive{
					{Name: "log", Args: []string{}},
				},
			},
			expected: "log",
		},
		{
			name: "directive with args",
			snippet: caddy.Snippet{
				Name: "test",
				Directives: []caddy.Directive{
					{Name: "header", Args: []string{"X-Custom", "value"}},
				},
			},
			expected: "header X-Custom value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSnippetPreview(tt.snippet)
			if result != tt.expected {
				t.Errorf("getSnippetPreview() = %q, want %q", result, tt.expected)
			}
		})
	}
}
