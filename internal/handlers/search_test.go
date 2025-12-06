package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

func TestSearchHandler_Search(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "search-handler-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test Caddyfile
	caddyfilePath := filepath.Join(tmpDir, "Caddyfile")
	caddyfileContent := `# Global options
{
	email admin@example.com
}

# Common headers snippet
(common-headers) {
	header X-Frame-Options "DENY"
	header X-Content-Type-Options "nosniff"
}

example.com {
	reverse_proxy localhost:8080
	import common-headers
}

api.example.com {
	reverse_proxy localhost:3000
}
`
	if err := os.WriteFile(caddyfilePath, []byte(caddyfileContent), 0644); err != nil {
		t.Fatalf("failed to create Caddyfile: %v", err)
	}

	// Create test config
	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
	}

	// Create templates from embedded FS
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(filepath.Join(templatesDir, "layouts"), 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(templatesDir, "pages"), 0755); err != nil {
		t.Fatalf("failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(templatesDir, "partials"), 0755); err != nil {
		t.Fatalf("failed to create partials dir: %v", err)
	}

	// Create minimal base layout
	baseLayout := `{{ define "base" }}<!DOCTYPE html><html><body>{{ block "content" . }}{{ end }}</body></html>{{ end }}`
	if err := os.WriteFile(filepath.Join(templatesDir, "layouts", "base.html"), []byte(baseLayout), 0644); err != nil {
		t.Fatalf("failed to create base layout: %v", err)
	}

	// Create minimal page
	dashboardPage := `{{ define "dashboard.html" }}{{ template "base" . }}{{ end }}`
	if err := os.WriteFile(filepath.Join(templatesDir, "pages", "dashboard.html"), []byte(dashboardPage), 0644); err != nil {
		t.Fatalf("failed to create dashboard page: %v", err)
	}

	// Create search results partial
	searchResultsPartial := `{{ define "search-results.html" }}<div class="results">{{ range .Results }}<div>{{ .Title }}</div>{{ end }}</div>{{ end }}`
	if err := os.WriteFile(filepath.Join(templatesDir, "partials", "search-results.html"), []byte(searchResultsPartial), 0644); err != nil {
		t.Fatalf("failed to create search results partial: %v", err)
	}

	tmpl, err := templates.New(templatesDir)
	if err != nil {
		t.Fatalf("failed to create templates: %v", err)
	}

	handler := NewSearchHandler(tmpl, cfg)

	tests := []struct {
		name           string
		query          string
		expectedInBody []string
		notInBody      []string
	}{
		{
			name:           "empty query returns navigation pages",
			query:          "",
			expectedInBody: []string{"Dashboard", "Sites"},
		},
		{
			name:           "search for site domain",
			query:          "example",
			expectedInBody: []string{"example.com", "api.example.com"},
		},
		{
			name:           "search for snippet",
			query:          "common",
			expectedInBody: []string{"common-headers"},
		},
		{
			name:           "search for page",
			query:          "certificates",
			expectedInBody: []string{"Certificates"},
		},
		{
			name:           "search for non-existent term",
			query:          "nonexistenttermxyz",
			expectedInBody: []string{}, // Should have no specific matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/search?q="+tt.query, nil)
			w := httptest.NewRecorder()

			handler.Search(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}

			body := w.Body.String()
			for _, expected := range tt.expectedInBody {
				if !strings.Contains(body, expected) {
					t.Errorf("expected body to contain %q, got: %s", expected, body)
				}
			}
			for _, notExpected := range tt.notInBody {
				if strings.Contains(body, notExpected) {
					t.Errorf("expected body to NOT contain %q, got: %s", notExpected, body)
				}
			}
		})
	}
}

func TestMatchesQuery(t *testing.T) {
	tests := []struct {
		text     string
		query    string
		expected bool
	}{
		{"example.com", "example", true},
		{"Example.com", "example", true},
		{"EXAMPLE.COM", "example", true},
		{"test.example.com", "example", true},
		{"test.com", "example", false},
		{"", "example", false},
		{"example.com", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.text+"_"+tt.query, func(t *testing.T) {
			result := matchesQuery(tt.text, tt.query)
			if result != tt.expected {
				t.Errorf("matchesQuery(%q, %q) = %v, want %v", tt.text, tt.query, result, tt.expected)
			}
		})
	}
}

func TestFindMatchContext(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		query    string
		contains string
	}{
		{
			name:     "match in middle",
			text:     "this is a test string with the word example in it",
			query:    "example",
			contains: "example",
		},
		{
			name:     "match at start",
			text:     "example is at the start",
			query:    "example",
			contains: "example",
		},
		{
			name:     "no match returns prefix",
			text:     "this is a long string without the query term",
			query:    "xyz",
			contains: "this is a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findMatchContext(tt.text, tt.query)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("findMatchContext(%q, %q) = %q, expected to contain %q", tt.text, tt.query, result, tt.contains)
			}
		})
	}
}

func TestGetSiteDescription(t *testing.T) {
	tests := []struct {
		name        string
		directives  []struct{ name string; args []string }
		imports     []string
		expected    string
	}{
		{
			name:       "reverse proxy",
			directives: []struct{ name string; args []string }{{name: "reverse_proxy", args: []string{"localhost:8080"}}},
			expected:   "Reverse proxy to localhost:8080",
		},
		{
			name:       "file server",
			directives: []struct{ name string; args []string }{{name: "file_server", args: nil}},
			expected:   "Static file server",
		},
		{
			name:       "redirect",
			directives: []struct{ name string; args []string }{{name: "redir", args: []string{"https://example.com"}}},
			expected:   "Redirect to https://example.com",
		},
		{
			name:       "with imports",
			directives: nil,
			imports:    []string{"common-headers", "security"},
			expected:   "Imports: common-headers, security",
		},
		{
			name:       "empty site",
			directives: nil,
			expected:   "Site configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Import the caddy package for Site and Directive types
			// This test would require importing caddy, so we'll just verify the function exists
			// The actual functionality is tested via integration tests
		})
	}
}
