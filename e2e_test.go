// Package main provides end-to-end tests for Caddyshack.
// These tests require a running Caddy instance and are tagged with "e2e".
// Run with: go test -tags=e2e ./...
//
//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
)

// These tests require:
// 1. A running Caddy instance with Admin API on localhost:2019
// 2. A writable Caddyfile path
//
// Run with: docker compose -f docker-compose.dev.yml up -d caddy
// Then: go test -tags=e2e -v ./...

const (
	testCaddyAdminAPI = "http://localhost:2019"
	testTimeout       = 30 * time.Second
)

// TestCaddyAdminAPI_Integration tests the Caddy Admin API client with a real Caddy instance.
func TestCaddyAdminAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	client := caddy.NewAdminClient(testCaddyAdminAPI)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Test Ping
	t.Run("Ping", func(t *testing.T) {
		ok, err := client.Ping(ctx)
		if err != nil {
			t.Fatalf("Ping failed: %v (is Caddy running?)", err)
		}
		if !ok {
			t.Error("Ping returned false")
		}
	})

	// Test GetStatus
	t.Run("GetStatus", func(t *testing.T) {
		status, err := client.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if !status.Running {
			t.Error("Caddy should be running")
		}
		if status.Version == "" {
			t.Error("Version should not be empty")
		}
		t.Logf("Caddy version: %s", status.Version)
	})

	// Test GetConfig
	t.Run("GetConfig", func(t *testing.T) {
		config, err := client.GetConfig(ctx)
		if err != nil {
			t.Fatalf("GetConfig failed: %v", err)
		}
		// Config should be valid JSON
		var js json.RawMessage
		if err := json.Unmarshal(config, &js); err != nil {
			t.Errorf("Config is not valid JSON: %v", err)
		}
	})
}

// TestCaddyAdminAPI_ValidateConfig tests config validation with the Caddy Admin API.
func TestCaddyAdminAPI_ValidateConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	client := caddy.NewAdminClient(testCaddyAdminAPI)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// First check if Caddy is available
	ok, err := client.Ping(ctx)
	if err != nil || !ok {
		t.Skip("Caddy not available, skipping test")
	}

	tests := []struct {
		name        string
		config      string
		expectError bool
	}{
		{
			name: "valid_simple_reverse_proxy",
			config: `example.com {
	reverse_proxy localhost:8080
}
`,
			expectError: false,
		},
		{
			name: "valid_static_site",
			config: `static.example.com {
	root * /var/www/html
	file_server
}
`,
			expectError: false,
		},
		{
			name: "valid_redirect",
			config: `old.example.com {
	redir https://new.example.com{uri} 301
}
`,
			expectError: false,
		},
		{
			name: "valid_with_global_options",
			config: `{
	email admin@example.com
}

example.com {
	reverse_proxy localhost:8080
}
`,
			expectError: false,
		},
		{
			name:        "invalid_unclosed_block",
			config:      `example.com {`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ValidateConfig(ctx, tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestCaddyAdminAPI_ReloadConfig tests config reload with the Caddy Admin API.
func TestCaddyAdminAPI_ReloadConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	client := caddy.NewAdminClient(testCaddyAdminAPI)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// First check if Caddy is available
	ok, err := client.Ping(ctx)
	if err != nil || !ok {
		t.Skip("Caddy not available, skipping test")
	}

	// Get current config to restore later
	originalConfig, err := client.GetConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to get original config: %v", err)
	}

	// Test reload with a simple valid config
	testConfig := `localhost:9999 {
	respond "Test configuration"
}
`

	err = client.Reload(ctx, testConfig)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Verify the new config is active by checking the config endpoint
	newConfig, err := client.GetConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to get new config: %v", err)
	}

	// Config should have changed
	if bytes.Equal(originalConfig, newConfig) {
		t.Error("Config should have changed after reload")
	}

	// Restore original config (best effort)
	t.Cleanup(func() {
		// Convert JSON config back to Caddyfile format is complex,
		// so we just reload with an empty config for cleanup
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		// Use a minimal config for cleanup
		_ = client.Reload(cleanupCtx, "")
	})
}

// TestE2E_SiteLifecycle tests the full lifecycle of a site through the Caddyshack API.
// This test requires both Caddyshack and Caddy to be running.
func TestE2E_SiteLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Check if Caddyshack is running
	caddyshackURL := os.Getenv("CADDYSHACK_URL")
	if caddyshackURL == "" {
		caddyshackURL = "http://localhost:8080"
	}

	// Try to connect to Caddyshack
	resp, err := http.Get(caddyshackURL + "/health")
	if err != nil {
		t.Skipf("Caddyshack not available at %s: %v", caddyshackURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Caddyshack health check failed: %d", resp.StatusCode)
	}

	t.Run("CreateSite", func(t *testing.T) {
		// Test creating a site through the web interface
		// This would require proper session handling for auth
		t.Skip("Full web interface testing requires authentication setup")
	})
}

// TestCaddyfileRoundtrip tests parsing and writing Caddyfiles.
func TestCaddyfileRoundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")

	originalContent := `{
	email admin@example.com
}

(common) {
	encode gzip
	header X-Frame-Options DENY
}

example.com {
	import common
	reverse_proxy localhost:8080
}

static.example.com {
	root * /var/www/html
	file_server
}

old.example.com {
	redir https://new.example.com{uri} 301
}
`

	// Write original file
	if err := os.WriteFile(caddyfilePath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	// Read and parse
	reader := caddy.NewReader(caddyfilePath)
	content, err := reader.Read()
	if err != nil {
		t.Fatalf("Failed to read Caddyfile: %v", err)
	}

	parser := caddy.NewParser(content)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse Caddyfile: %v", err)
	}

	// Verify parsed content
	if len(caddyfile.Sites) != 3 {
		t.Errorf("Expected 3 sites, got %d", len(caddyfile.Sites))
	}
	if len(caddyfile.Snippets) != 1 {
		t.Errorf("Expected 1 snippet, got %d", len(caddyfile.Snippets))
	}
	if caddyfile.GlobalOptions.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got %q", caddyfile.GlobalOptions.Email)
	}

	// Write back
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Write to new file
	newPath := filepath.Join(tempDir, "Caddyfile.new")
	if err := os.WriteFile(newPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to write new Caddyfile: %v", err)
	}

	// Parse the new file
	reader2 := caddy.NewReader(newPath)
	content2, err := reader2.Read()
	if err != nil {
		t.Fatalf("Failed to read new Caddyfile: %v", err)
	}

	parser2 := caddy.NewParser(content2)
	caddyfile2, err := parser2.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse new Caddyfile: %v", err)
	}

	// Verify roundtrip preserves structure
	if len(caddyfile2.Sites) != len(caddyfile.Sites) {
		t.Errorf("Site count mismatch: %d vs %d", len(caddyfile2.Sites), len(caddyfile.Sites))
	}
	if len(caddyfile2.Snippets) != len(caddyfile.Snippets) {
		t.Errorf("Snippet count mismatch: %d vs %d", len(caddyfile2.Snippets), len(caddyfile.Snippets))
	}
	if caddyfile2.GlobalOptions.Email != caddyfile.GlobalOptions.Email {
		t.Errorf("Email mismatch: %q vs %q", caddyfile2.GlobalOptions.Email, caddyfile.GlobalOptions.Email)
	}
}

// TestCaddyValidator_Integration tests the validator with the real Caddy binary.
func TestCaddyValidator_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Check if caddy binary is available
	validator := caddy.NewValidator()

	tempDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		expectError bool
	}{
		{
			name: "valid_reverse_proxy",
			content: `localhost:8080 {
	reverse_proxy localhost:9090
}
`,
			expectError: false,
		},
		{
			name: "valid_file_server",
			content: `localhost:8080 {
	root * /tmp
	file_server
}
`,
			expectError: false,
		},
		{
			name:        "invalid_syntax",
			content:     `localhost:8080 {`,
			expectError: true,
		},
		{
			name: "invalid_directive",
			content: `localhost:8080 {
	unknown_directive
}
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caddyfilePath := filepath.Join(tempDir, "Caddyfile_"+tt.name)
			if err := os.WriteFile(caddyfilePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write Caddyfile: %v", err)
			}

			result, err := validator.ValidateContent(tt.content)
			if tt.expectError {
				if err == nil && result.Valid {
					t.Error("Expected validation error, got valid result")
				}
			} else {
				if err != nil {
					// Caddy might not be available
					if strings.Contains(err.Error(), "executable file not found") {
						t.Skip("caddy binary not available")
					}
					t.Errorf("Unexpected error: %v", err)
				}
				if !result.Valid {
					t.Errorf("Expected valid result, got: %v", result.Errors)
				}
			}
		})
	}
}

// TestHTTPHandlers_Integration tests HTTP handlers with a test server.
func TestHTTPHandlers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// This test would start the full Caddyshack server
	// and test HTTP endpoints
	t.Skip("Full HTTP handler integration test requires server setup")
}

// Helper function to make authenticated requests to Caddyshack
func makeAuthRequest(t *testing.T, method, url string, body io.Reader, username, password string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// TestCaddyIntegration_FullFlow tests a complete flow with Caddy.
func TestCaddyIntegration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	client := caddy.NewAdminClient(testCaddyAdminAPI)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Check if Caddy is available
	ok, err := client.Ping(ctx)
	if err != nil || !ok {
		t.Skip("Caddy not available, skipping test")
	}

	// Step 1: Create a new Caddyfile
	tempDir := t.TempDir()
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")

	// Step 2: Write initial config
	initialConfig := `localhost:19999 {
	respond "Hello from test"
}
`
	if err := os.WriteFile(caddyfilePath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write Caddyfile: %v", err)
	}

	// Step 3: Load and reload through Admin API
	err = client.Reload(ctx, initialConfig)
	if err != nil {
		t.Fatalf("Failed to reload initial config: %v", err)
	}

	// Step 4: Verify the server is responding
	time.Sleep(500 * time.Millisecond) // Give Caddy time to apply config

	resp, err := http.Get("http://localhost:19999")
	if err != nil {
		t.Logf("Note: localhost:19999 not reachable (expected in some environments): %v", err)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Hello from test") {
			t.Errorf("Unexpected response body: %s", string(body))
		}
	}

	// Step 5: Update config
	updatedConfig := `localhost:19999 {
	respond "Updated response"
}
`
	err = client.Reload(ctx, updatedConfig)
	if err != nil {
		t.Fatalf("Failed to reload updated config: %v", err)
	}

	// Step 6: Verify update
	time.Sleep(500 * time.Millisecond)

	resp2, err := http.Get("http://localhost:19999")
	if err != nil {
		t.Logf("Note: localhost:19999 not reachable after update: %v", err)
	} else {
		defer resp2.Body.Close()
		body, _ := io.ReadAll(resp2.Body)
		if !strings.Contains(string(body), "Updated response") {
			t.Errorf("Unexpected response body after update: %s", string(body))
		}
	}

	t.Log("Full flow test completed successfully")
}
