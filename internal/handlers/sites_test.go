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

func setupTestHandler(t *testing.T) (*SitesHandler, string) {
	t.Helper()

	// Create a temporary directory for the Caddyfile and database
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")
	dbPath := filepath.Join(tempDir, "test.db")

	// Find the templates directory relative to the test file
	// We need to go up from internal/handlers to the project root
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

	handler := NewSitesHandler(tmpl, cfg, s)
	return handler, caddyfilePath
}

// caddyAvailable checks if the caddy binary is available in PATH
func caddyAvailable() bool {
	_, err := exec.LookPath("caddy")
	return err == nil
}

func TestCreate_ValidReverseProxy(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Test creating a reverse proxy site
	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:8080")
	form.Set("enable_tls", "true")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	// Check for HX-Redirect header (indicates success)
	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was created
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "example.com") {
		t.Error("Caddyfile should contain 'example.com'")
	}
	if !strings.Contains(string(content), "reverse_proxy") {
		t.Error("Caddyfile should contain 'reverse_proxy'")
	}
	if !strings.Contains(string(content), "localhost:8080") {
		t.Error("Caddyfile should contain 'localhost:8080'")
	}
}

func TestCreate_ValidStaticSite(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "static.example.com")
	form.Set("type", "static")
	form.Set("root_path", "/var/www/html")
	form.Set("enable_tls", "true")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "static.example.com") {
		t.Error("Caddyfile should contain 'static.example.com'")
	}
	if !strings.Contains(string(content), "file_server") {
		t.Error("Caddyfile should contain 'file_server'")
	}
	if !strings.Contains(string(content), "/var/www/html") {
		t.Error("Caddyfile should contain '/var/www/html'")
	}
}

func TestCreate_ValidRedirect(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "old.example.com")
	form.Set("type", "redirect")
	form.Set("redirect_url", "https://new.example.com{uri}")
	form.Set("redirect_code", "301")
	form.Set("enable_tls", "true")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "old.example.com") {
		t.Error("Caddyfile should contain 'old.example.com'")
	}
	if !strings.Contains(string(content), "redir") {
		t.Error("Caddyfile should contain 'redir'")
	}
	if !strings.Contains(string(content), "301") {
		t.Error("Caddyfile should contain '301'")
	}
}

func TestCreate_MissingDomain(t *testing.T) {
	handler, _ := setupTestHandler(t)

	form := url.Values{}
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
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
	if !strings.Contains(body, "Domain is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestCreate_MissingTarget(t *testing.T) {
	handler, _ := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "reverse_proxy")
	// No target

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Backend target is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestCreate_MissingRootPath(t *testing.T) {
	handler, _ := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "static")
	// No root_path

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Root directory is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestCreate_MissingRedirectUrl(t *testing.T) {
	handler, _ := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "redirect")
	// No redirect_url

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Redirect URL is required") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestCreate_InvalidSiteType(t *testing.T) {
	handler, _ := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "invalid_type")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect on validation error")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Invalid site type") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestCreate_DuplicateSite(t *testing.T) {
	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with a site
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to create a site with the same domain
	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:9090")

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when site already exists")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "already exists") {
		t.Errorf("Response should contain error message about duplicate, got: %s", body)
	}
}

func TestCreate_DisableTLS(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:8080")
	// enable_tls not set or "off" - TLS should be disabled

	req := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	// When TLS is disabled, the domain should have http:// prefix
	if !strings.Contains(string(content), "http://example.com") {
		t.Errorf("Caddyfile should contain 'http://example.com' when TLS is disabled, got: %s", string(content))
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"localhost", true},
		{"localhost:8080", true},
		{":8080", true},
		{"http://example.com", true},
		{"https://example.com", true},
		{"example", true},
		{"", false},
		{"domain with spaces", false},
		{"domain\twith\ttabs", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := isValidDomain(tt.domain)
			if result != tt.valid {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, result, tt.valid)
			}
		})
	}
}

func TestIsHTMXRequest(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"with HX-Request header", "true", true},
		{"without HX-Request header", "", false},
		{"with wrong HX-Request value", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("HX-Request", tt.header)
			}

			result := isHTMXRequest(req)
			if result != tt.expected {
				t.Errorf("isHTMXRequest() = %v, want %v", result, tt.expected)
			}
		})
	}

	// Test with nil request
	if isHTMXRequest(nil) {
		t.Error("isHTMXRequest(nil) should return false")
	}
}

func TestSiteToFormValues_ReverseProxy(t *testing.T) {
	site := &caddy.Site{
		Addresses: []string{"example.com"},
		Directives: []caddy.Directive{
			{Name: "reverse_proxy", Args: []string{"localhost:8080"}},
		},
	}

	formValues := siteToFormValues(site, "example.com")

	if formValues.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got %q", formValues.Domain)
	}
	if formValues.OriginalDomain != "example.com" {
		t.Errorf("Expected original domain 'example.com', got %q", formValues.OriginalDomain)
	}
	if formValues.Type != "reverse_proxy" {
		t.Errorf("Expected type 'reverse_proxy', got %q", formValues.Type)
	}
	if formValues.Target != "localhost:8080" {
		t.Errorf("Expected target 'localhost:8080', got %q", formValues.Target)
	}
	if !formValues.EnableTls {
		t.Error("Expected EnableTls to be true")
	}
}

func TestSiteToFormValues_StaticSite(t *testing.T) {
	site := &caddy.Site{
		Addresses: []string{"static.example.com"},
		Directives: []caddy.Directive{
			{Name: "root", Args: []string{"*", "/var/www/html"}},
			{Name: "file_server", Args: []string{}},
		},
	}

	formValues := siteToFormValues(site, "static.example.com")

	if formValues.Type != "static" {
		t.Errorf("Expected type 'static', got %q", formValues.Type)
	}
	if formValues.RootPath != "/var/www/html" {
		t.Errorf("Expected root path '/var/www/html', got %q", formValues.RootPath)
	}
}

func TestSiteToFormValues_Redirect(t *testing.T) {
	site := &caddy.Site{
		Addresses: []string{"old.example.com"},
		Directives: []caddy.Directive{
			{Name: "redir", Args: []string{"https://new.example.com{uri}", "301"}},
		},
	}

	formValues := siteToFormValues(site, "old.example.com")

	if formValues.Type != "redirect" {
		t.Errorf("Expected type 'redirect', got %q", formValues.Type)
	}
	if formValues.RedirectUrl != "https://new.example.com{uri}" {
		t.Errorf("Expected redirect URL 'https://new.example.com{uri}', got %q", formValues.RedirectUrl)
	}
	if formValues.RedirectCode != "301" {
		t.Errorf("Expected redirect code '301', got %q", formValues.RedirectCode)
	}
}

func TestSiteToFormValues_HttpPrefix(t *testing.T) {
	site := &caddy.Site{
		Addresses: []string{"http://example.com"},
		Directives: []caddy.Directive{
			{Name: "reverse_proxy", Args: []string{"localhost:8080"}},
		},
	}

	formValues := siteToFormValues(site, "http://example.com")

	if formValues.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com' (without http://), got %q", formValues.Domain)
	}
	if formValues.EnableTls {
		t.Error("Expected EnableTls to be false for http:// domain")
	}
}

func TestUpdate_ValidUpdate(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with a site
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Update the site to use a different target
	form := url.Values{}
	form.Set("domain", "example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:9090")
	form.Set("enable_tls", "true")

	req := httptest.NewRequest(http.MethodPut, "/sites/example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was updated
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "localhost:9090") {
		t.Error("Caddyfile should contain 'localhost:9090' after update")
	}
	if strings.Contains(string(content), "localhost:8080") {
		t.Error("Caddyfile should NOT contain old 'localhost:8080' after update")
	}
}

func TestUpdate_ChangeDomain(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with a site
	existingContent := `old.example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Update the site with a new domain
	form := url.Values{}
	form.Set("domain", "new.example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:8080")
	form.Set("enable_tls", "true")

	req := httptest.NewRequest(http.MethodPut, "/sites/old.example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was updated
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if !strings.Contains(string(content), "new.example.com") {
		t.Error("Caddyfile should contain 'new.example.com' after domain change")
	}
	if strings.Contains(string(content), "old.example.com") {
		t.Error("Caddyfile should NOT contain old 'old.example.com' after domain change")
	}
}

func TestUpdate_SiteNotFound(t *testing.T) {
	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with a different site
	existingContent := `other.example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to update a non-existent site
	form := url.Values{}
	form.Set("domain", "nonexistent.example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:9090")

	req := httptest.NewRequest(http.MethodPut, "/sites/nonexistent.example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when site not found")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Site not found") {
		t.Errorf("Response should contain 'Site not found', got: %s", body)
	}
}

func TestUpdate_DuplicateDomain(t *testing.T) {
	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with two sites
	existingContent := `site1.example.com {
	reverse_proxy localhost:8080
}

site2.example.com {
	reverse_proxy localhost:9090
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to update site1 with the same domain as site2
	form := url.Values{}
	form.Set("domain", "site2.example.com")
	form.Set("type", "reverse_proxy")
	form.Set("target", "localhost:8080")

	req := httptest.NewRequest(http.MethodPut, "/sites/site1.example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("Should not redirect when domain conflicts with another site")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "already exists") {
		t.Errorf("Response should contain 'already exists', got: %s", body)
	}
}

func TestDelete_ValidDelete(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with multiple sites
	existingContent := `site1.example.com {
	reverse_proxy localhost:8080
}

site2.example.com {
	reverse_proxy localhost:9090
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Delete site1
	req := httptest.NewRequest(http.MethodDelete, "/sites/site1.example.com", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile was updated
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if strings.Contains(string(content), "site1.example.com") {
		t.Error("Caddyfile should NOT contain 'site1.example.com' after delete")
	}
	if !strings.Contains(string(content), "site2.example.com") {
		t.Error("Caddyfile should still contain 'site2.example.com' after delete")
	}
}

func TestDelete_SiteNotFound(t *testing.T) {
	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with a different site
	existingContent := `other.example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Try to delete a non-existent site
	req := httptest.NewRequest(http.MethodDelete, "/sites/nonexistent.example.com", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Not Found") {
		t.Errorf("Response should contain 'Not Found', got: %s", body)
	}
}

func TestDelete_EmptyDomain(t *testing.T) {
	handler, _ := setupTestHandler(t)

	// Try to delete with empty domain
	req := httptest.NewRequest(http.MethodDelete, "/sites/", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestDelete_LastSite(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile with just one site
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Delete the only site
	req := httptest.NewRequest(http.MethodDelete, "/sites/example.com", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Header().Get("HX-Redirect") != "/sites" {
		t.Errorf("Expected HX-Redirect to /sites, got %q", rec.Header().Get("HX-Redirect"))
		t.Logf("Response body: %s", rec.Body.String())
	}

	// Verify the Caddyfile is now empty (or contains just whitespace)
	content, err := os.ReadFile(caddyfilePath)
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	if strings.Contains(string(content), "example.com") {
		t.Error("Caddyfile should NOT contain 'example.com' after delete")
	}
}

func TestDelete_NonHTMXRequest(t *testing.T) {
	if !caddyAvailable() {
		t.Skip("Skipping test: caddy binary not available")
	}

	handler, caddyfilePath := setupTestHandler(t)

	// Create an existing Caddyfile
	existingContent := `example.com {
	reverse_proxy localhost:8080
}
`
	if err := os.WriteFile(caddyfilePath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write existing Caddyfile: %v", err)
	}

	// Delete without HTMX header
	req := httptest.NewRequest(http.MethodDelete, "/sites/example.com", nil)
	// No HX-Request header

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	// Should redirect (302 Found) for non-HTMX requests
	if rec.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", rec.Code)
	}

	if rec.Header().Get("Location") != "/sites" {
		t.Errorf("Expected Location header to /sites, got %q", rec.Header().Get("Location"))
	}
}
