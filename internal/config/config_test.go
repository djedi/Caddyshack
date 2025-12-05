package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any existing environment variables
	envVars := []string{
		"CADDYSHACK_PORT",
		"CADDYSHACK_DEV",
		"CADDYSHACK_TEMPLATES_DIR",
		"CADDYSHACK_STATIC_DIR",
		"CADDYSHACK_CADDYFILE",
		"CADDYSHACK_CADDY_API",
		"CADDYSHACK_DB",
		"CADDYSHACK_AUTH_USER",
		"CADDYSHACK_AUTH_PASS",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected Port to be '8080', got %q", cfg.Port)
	}
	if cfg.DevMode != false {
		t.Errorf("expected DevMode to be false, got %v", cfg.DevMode)
	}
	if cfg.TemplatesDir != "templates" {
		t.Errorf("expected TemplatesDir to be 'templates', got %q", cfg.TemplatesDir)
	}
	if cfg.StaticDir != "static" {
		t.Errorf("expected StaticDir to be 'static', got %q", cfg.StaticDir)
	}
	if cfg.CaddyfilePath != "/etc/caddy/Caddyfile" {
		t.Errorf("expected CaddyfilePath to be '/etc/caddy/Caddyfile', got %q", cfg.CaddyfilePath)
	}
	if cfg.CaddyAdminAPI != "http://localhost:2019" {
		t.Errorf("expected CaddyAdminAPI to be 'http://localhost:2019', got %q", cfg.CaddyAdminAPI)
	}
	if cfg.DBPath != "caddyshack.db" {
		t.Errorf("expected DBPath to be 'caddyshack.db', got %q", cfg.DBPath)
	}
	if cfg.AuthUser != "" {
		t.Errorf("expected AuthUser to be empty, got %q", cfg.AuthUser)
	}
	if cfg.AuthPass != "" {
		t.Errorf("expected AuthPass to be empty, got %q", cfg.AuthPass)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("CADDYSHACK_PORT", "9090")
	os.Setenv("CADDYSHACK_DEV", "true")
	os.Setenv("CADDYSHACK_TEMPLATES_DIR", "/custom/templates")
	os.Setenv("CADDYSHACK_STATIC_DIR", "/custom/static")
	os.Setenv("CADDYSHACK_CADDYFILE", "/path/to/Caddyfile")
	os.Setenv("CADDYSHACK_CADDY_API", "http://caddy:2019")
	os.Setenv("CADDYSHACK_DB", "/data/app.db")
	os.Setenv("CADDYSHACK_AUTH_USER", "admin")
	os.Setenv("CADDYSHACK_AUTH_PASS", "secret123")

	defer func() {
		os.Unsetenv("CADDYSHACK_PORT")
		os.Unsetenv("CADDYSHACK_DEV")
		os.Unsetenv("CADDYSHACK_TEMPLATES_DIR")
		os.Unsetenv("CADDYSHACK_STATIC_DIR")
		os.Unsetenv("CADDYSHACK_CADDYFILE")
		os.Unsetenv("CADDYSHACK_CADDY_API")
		os.Unsetenv("CADDYSHACK_DB")
		os.Unsetenv("CADDYSHACK_AUTH_USER")
		os.Unsetenv("CADDYSHACK_AUTH_PASS")
	}()

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected Port to be '9090', got %q", cfg.Port)
	}
	if cfg.DevMode != true {
		t.Errorf("expected DevMode to be true, got %v", cfg.DevMode)
	}
	if cfg.TemplatesDir != "/custom/templates" {
		t.Errorf("expected TemplatesDir to be '/custom/templates', got %q", cfg.TemplatesDir)
	}
	if cfg.StaticDir != "/custom/static" {
		t.Errorf("expected StaticDir to be '/custom/static', got %q", cfg.StaticDir)
	}
	if cfg.CaddyfilePath != "/path/to/Caddyfile" {
		t.Errorf("expected CaddyfilePath to be '/path/to/Caddyfile', got %q", cfg.CaddyfilePath)
	}
	if cfg.CaddyAdminAPI != "http://caddy:2019" {
		t.Errorf("expected CaddyAdminAPI to be 'http://caddy:2019', got %q", cfg.CaddyAdminAPI)
	}
	if cfg.DBPath != "/data/app.db" {
		t.Errorf("expected DBPath to be '/data/app.db', got %q", cfg.DBPath)
	}
	if cfg.AuthUser != "admin" {
		t.Errorf("expected AuthUser to be 'admin', got %q", cfg.AuthUser)
	}
	if cfg.AuthPass != "secret123" {
		t.Errorf("expected AuthPass to be 'secret123', got %q", cfg.AuthPass)
	}
}

func TestDevModeBooleanParsing(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"invalid", false}, // falls back to default
		{"", false},        // falls back to default
	}

	for _, tc := range tests {
		os.Setenv("CADDYSHACK_DEV", tc.value)
		cfg := Load()
		if cfg.DevMode != tc.expected {
			t.Errorf("CADDYSHACK_DEV=%q: expected DevMode=%v, got %v", tc.value, tc.expected, cfg.DevMode)
		}
	}
	os.Unsetenv("CADDYSHACK_DEV")
}

func TestAuthEnabled(t *testing.T) {
	tests := []struct {
		user     string
		pass     string
		expected bool
	}{
		{"admin", "secret", true},
		{"", "secret", false},
		{"admin", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		os.Setenv("CADDYSHACK_AUTH_USER", tc.user)
		os.Setenv("CADDYSHACK_AUTH_PASS", tc.pass)
		cfg := Load()
		if cfg.AuthEnabled() != tc.expected {
			t.Errorf("user=%q, pass=%q: expected AuthEnabled()=%v, got %v",
				tc.user, tc.pass, tc.expected, cfg.AuthEnabled())
		}
	}
	os.Unsetenv("CADDYSHACK_AUTH_USER")
	os.Unsetenv("CADDYSHACK_AUTH_PASS")
}
