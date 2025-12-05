package caddy

import (
	"testing"
)

// Example Caddyfile from prompt.md
const exampleCaddyfile = `{
  email dustin@redseam.com
  log {
    output file /var/log/caddy/access.log {
      roll_size 10mb
      roll_keep 5
    }
    format json
  }
}

# Reusable logging snippet
(site_log) {
  log {
    output file /var/log/caddy/access.log {
      roll_size 10mb
      roll_keep 5
    }
    format json
  }
}

# Reusable header snippet
(proxy_headers) {
  header {
    X-Proxied-By "Caddy"
  }
}

(php_bot_trap) {
    @php_trap path *.php
    handle @php_trap {
        redir https://speed.cloudflare.com/__down?bytes=1500000000 307
    }
}

portainer.redseam.com {
  import proxy_headers
  import site_log
  reverse_proxy http://108.181.221.120:9000
}

dustytune.com {
  import proxy_headers
  import site_log
  import php_bot_trap
  reverse_proxy 108.181.221.120:3080

  # Analytics proxy
  handle /9OoybqZpHP.js {
      rewrite * /sa.js
      reverse_proxy https://slimlytics.redseam.com {
          header_up Host slimlytics.redseam.com
      }
  }
}`

func TestParseSites(t *testing.T) {
	parser := NewParser(exampleCaddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 2 {
		t.Fatalf("Expected 2 sites, got %d", len(sites))
	}

	// Check first site (portainer.redseam.com)
	site1 := sites[0]
	if len(site1.Addresses) != 1 || site1.Addresses[0] != "portainer.redseam.com" {
		t.Errorf("Expected site 1 address 'portainer.redseam.com', got %v", site1.Addresses)
	}

	// Check imports for first site
	if len(site1.Imports) != 2 {
		t.Errorf("Expected 2 imports for site 1, got %d", len(site1.Imports))
	} else {
		if site1.Imports[0] != "proxy_headers" {
			t.Errorf("Expected first import 'proxy_headers', got '%s'", site1.Imports[0])
		}
		if site1.Imports[1] != "site_log" {
			t.Errorf("Expected second import 'site_log', got '%s'", site1.Imports[1])
		}
	}

	// Check second site (dustytune.com)
	site2 := sites[1]
	if len(site2.Addresses) != 1 || site2.Addresses[0] != "dustytune.com" {
		t.Errorf("Expected site 2 address 'dustytune.com', got %v", site2.Addresses)
	}

	// Check imports for second site
	if len(site2.Imports) != 3 {
		t.Errorf("Expected 3 imports for site 2, got %d", len(site2.Imports))
	}
}

func TestParseSitesWithMultipleAddresses(t *testing.T) {
	caddyfile := `example.com www.example.com {
  reverse_proxy localhost:8080
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	site := sites[0]
	if len(site.Addresses) != 2 {
		t.Errorf("Expected 2 addresses, got %d: %v", len(site.Addresses), site.Addresses)
	}
	if site.Addresses[0] != "example.com" {
		t.Errorf("Expected first address 'example.com', got '%s'", site.Addresses[0])
	}
	if site.Addresses[1] != "www.example.com" {
		t.Errorf("Expected second address 'www.example.com', got '%s'", site.Addresses[1])
	}
}

func TestParseSitesSkipsGlobalOptions(t *testing.T) {
	caddyfile := `{
  email admin@example.com
}

example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site (should skip global options), got %d", len(sites))
	}

	if sites[0].Addresses[0] != "example.com" {
		t.Errorf("Expected address 'example.com', got '%s'", sites[0].Addresses[0])
	}
}

func TestParseSitesSkipsSnippets(t *testing.T) {
	caddyfile := `(common_headers) {
  header X-Content-Type-Options "nosniff"
}

example.com {
  import common_headers
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site (should skip snippet), got %d", len(sites))
	}

	// Check that the import is tracked
	if len(sites[0].Imports) != 1 || sites[0].Imports[0] != "common_headers" {
		t.Errorf("Expected import 'common_headers', got %v", sites[0].Imports)
	}
}

func TestParseSitesWithPortAddress(t *testing.T) {
	caddyfile := `:8080 {
  respond "Hello from port 8080"
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	if sites[0].Addresses[0] != ":8080" {
		t.Errorf("Expected address ':8080', got '%s'", sites[0].Addresses[0])
	}
}

func TestParseSitesWithHTTPSAddress(t *testing.T) {
	caddyfile := `https://example.com {
  respond "Secure"
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	if sites[0].Addresses[0] != "https://example.com" {
		t.Errorf("Expected address 'https://example.com', got '%s'", sites[0].Addresses[0])
	}
}

func TestParseDirectivesWithReverseProxy(t *testing.T) {
	caddyfile := `example.com {
  reverse_proxy http://localhost:8080
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	// Find reverse_proxy directive
	var found bool
	for _, d := range sites[0].Directives {
		if d.Name == "reverse_proxy" {
			found = true
			if len(d.Args) < 1 || d.Args[0] != "http://localhost:8080" {
				t.Errorf("Expected reverse_proxy args ['http://localhost:8080'], got %v", d.Args)
			}
		}
	}

	if !found {
		t.Errorf("Expected to find reverse_proxy directive")
	}
}

func TestParseDirectivesWithNestedHandle(t *testing.T) {
	caddyfile := `example.com {
  handle /api/* {
    reverse_proxy localhost:3000
  }
  handle {
    file_server
  }
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	// Count handle directives
	handleCount := 0
	for _, d := range sites[0].Directives {
		if d.Name == "handle" {
			handleCount++
		}
	}

	if handleCount != 2 {
		t.Errorf("Expected 2 handle directives, got %d", handleCount)
	}
}

func TestParseSitesEmptyCaddyfile(t *testing.T) {
	parser := NewParser("")
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 0 {
		t.Errorf("Expected 0 sites for empty Caddyfile, got %d", len(sites))
	}
}

func TestParseSitesOnlyGlobalOptions(t *testing.T) {
	caddyfile := `{
  email admin@example.com
  acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}`

	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()

	if err != nil {
		t.Fatalf("ParseSites returned error: %v", err)
	}

	if len(sites) != 0 {
		t.Errorf("Expected 0 sites (only global options), got %d", len(sites))
	}
}

func TestIsSiteAddress(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		{"example.com", true},
		{"www.example.com", true},
		{":8080", true},
		{"localhost", true},
		{"localhost:8080", true},
		{"https://example.com", true},
		{"http://example.com", true},
		{"192.168.1.1", true},
		{"reverse_proxy", false},
		{"import", false},
		{"handle", false},
		{"{", false},
		{"}", false},
		{"", false},
		{"(snippet)", false},
		{"@matcher", false},
	}

	for _, tc := range tests {
		result := isSiteAddress(tc.token)
		if result != tc.expected {
			t.Errorf("isSiteAddress(%q) = %v, expected %v", tc.token, result, tc.expected)
		}
	}
}

func TestParseSnippets(t *testing.T) {
	parser := NewParser(exampleCaddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 3 {
		t.Fatalf("Expected 3 snippets, got %d", len(snippets))
	}

	// Check snippet names
	expectedNames := []string{"site_log", "proxy_headers", "php_bot_trap"}
	for i, expected := range expectedNames {
		if snippets[i].Name != expected {
			t.Errorf("Expected snippet %d name '%s', got '%s'", i, expected, snippets[i].Name)
		}
	}
}

func TestParseSnippetsSimple(t *testing.T) {
	caddyfile := `(common_headers) {
  header X-Content-Type-Options "nosniff"
  header X-Frame-Options "DENY"
}

example.com {
  import common_headers
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 1 {
		t.Fatalf("Expected 1 snippet, got %d", len(snippets))
	}

	snippet := snippets[0]
	if snippet.Name != "common_headers" {
		t.Errorf("Expected snippet name 'common_headers', got '%s'", snippet.Name)
	}

	// Check that directives were parsed
	if len(snippet.Directives) < 1 {
		t.Errorf("Expected at least 1 directive in snippet, got %d", len(snippet.Directives))
	}

	// First directive should be 'header'
	if len(snippet.Directives) > 0 && snippet.Directives[0].Name != "header" {
		t.Errorf("Expected first directive to be 'header', got '%s'", snippet.Directives[0].Name)
	}
}

func TestParseSnippetsWithNestedBlocks(t *testing.T) {
	caddyfile := `(site_log) {
  log {
    output file /var/log/caddy/access.log {
      roll_size 10mb
      roll_keep 5
    }
    format json
  }
}`

	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 1 {
		t.Fatalf("Expected 1 snippet, got %d", len(snippets))
	}

	snippet := snippets[0]
	if snippet.Name != "site_log" {
		t.Errorf("Expected snippet name 'site_log', got '%s'", snippet.Name)
	}

	// Check that the log directive was parsed
	if len(snippet.Directives) < 1 {
		t.Fatalf("Expected at least 1 directive in snippet, got %d", len(snippet.Directives))
	}

	if snippet.Directives[0].Name != "log" {
		t.Errorf("Expected first directive to be 'log', got '%s'", snippet.Directives[0].Name)
	}

	// Check that nested block was captured
	if len(snippet.Directives[0].Block) < 1 {
		t.Errorf("Expected nested directives in log block, got %d", len(snippet.Directives[0].Block))
	}
}

func TestParseSnippetsEmpty(t *testing.T) {
	parser := NewParser("")
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 0 {
		t.Errorf("Expected 0 snippets for empty Caddyfile, got %d", len(snippets))
	}
}

func TestParseSnippetsNoSnippets(t *testing.T) {
	caddyfile := `{
  email admin@example.com
}

example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 0 {
		t.Errorf("Expected 0 snippets (no snippet definitions), got %d", len(snippets))
	}
}

func TestParseSnippetsWithMatcher(t *testing.T) {
	caddyfile := `(php_bot_trap) {
    @php_trap path *.php
    handle @php_trap {
        redir https://example.com 307
    }
}`

	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 1 {
		t.Fatalf("Expected 1 snippet, got %d", len(snippets))
	}

	snippet := snippets[0]
	if snippet.Name != "php_bot_trap" {
		t.Errorf("Expected snippet name 'php_bot_trap', got '%s'", snippet.Name)
	}

	// Check for @php_trap matcher directive
	foundMatcher := false
	for _, d := range snippet.Directives {
		if d.Name == "@php_trap" {
			foundMatcher = true
			break
		}
	}

	if !foundMatcher {
		t.Errorf("Expected to find @php_trap matcher directive in snippet")
	}
}

func TestParseSnippetsMultiple(t *testing.T) {
	caddyfile := `(snippet1) {
  header X-One "1"
}

(snippet2) {
  header X-Two "2"
}

(snippet3) {
  header X-Three "3"
}

example.com {
  import snippet1
  import snippet2
  import snippet3
}`

	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()

	if err != nil {
		t.Fatalf("ParseSnippets returned error: %v", err)
	}

	if len(snippets) != 3 {
		t.Fatalf("Expected 3 snippets, got %d", len(snippets))
	}

	expectedNames := []string{"snippet1", "snippet2", "snippet3"}
	for i, expected := range expectedNames {
		if snippets[i].Name != expected {
			t.Errorf("Expected snippet %d name '%s', got '%s'", i, expected, snippets[i].Name)
		}
	}
}

// Tests for ParseGlobalOptions

func TestParseGlobalOptionsFromExampleCaddyfile(t *testing.T) {
	parser := NewParser(exampleCaddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.Email != "dustin@redseam.com" {
		t.Errorf("Expected email 'dustin@redseam.com', got '%s'", opts.Email)
	}

	if opts.LogConfig == nil {
		t.Fatalf("Expected log config, got nil")
	}

	if opts.LogConfig.Format != "json" {
		t.Errorf("Expected log format 'json', got '%s'", opts.LogConfig.Format)
	}

	if opts.LogConfig.Output != "file /var/log/caddy/access.log" {
		t.Errorf("Expected log output 'file /var/log/caddy/access.log', got '%s'", opts.LogConfig.Output)
	}

	if opts.LogConfig.RollSize != "10mb" {
		t.Errorf("Expected roll_size '10mb', got '%s'", opts.LogConfig.RollSize)
	}

	if opts.LogConfig.RollKeep != "5" {
		t.Errorf("Expected roll_keep '5', got '%s'", opts.LogConfig.RollKeep)
	}
}

func TestParseGlobalOptionsSimple(t *testing.T) {
	caddyfile := `{
  email admin@example.com
}

example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got '%s'", opts.Email)
	}
}

func TestParseGlobalOptionsWithACMECA(t *testing.T) {
	caddyfile := `{
  email admin@example.com
  acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}

example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got '%s'", opts.Email)
	}

	if opts.ACMECa != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Errorf("Expected acme_ca 'https://acme-staging-v02.api.letsencrypt.org/directory', got '%s'", opts.ACMECa)
	}
}

func TestParseGlobalOptionsWithAdmin(t *testing.T) {
	caddyfile := `{
  admin off
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.Admin != "off" {
		t.Errorf("Expected admin 'off', got '%s'", opts.Admin)
	}
}

func TestParseGlobalOptionsWithDebug(t *testing.T) {
	caddyfile := `{
  debug
  email admin@example.com
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if !opts.Debug {
		t.Errorf("Expected debug to be true, got false")
	}

	if opts.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got '%s'", opts.Email)
	}
}

func TestParseGlobalOptionsWithLogLevel(t *testing.T) {
	caddyfile := `{
  log {
    level debug
    format console
  }
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.LogConfig == nil {
		t.Fatalf("Expected log config, got nil")
	}

	if opts.LogConfig.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", opts.LogConfig.Level)
	}

	if opts.LogConfig.Format != "console" {
		t.Errorf("Expected log format 'console', got '%s'", opts.LogConfig.Format)
	}
}

func TestParseGlobalOptionsNoGlobalBlock(t *testing.T) {
	caddyfile := `example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts != nil {
		t.Errorf("Expected nil global options when no global block exists, got %+v", opts)
	}
}

func TestParseGlobalOptionsEmpty(t *testing.T) {
	parser := NewParser("")
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts != nil {
		t.Errorf("Expected nil global options for empty Caddyfile, got %+v", opts)
	}
}

func TestParseGlobalOptionsRawBlock(t *testing.T) {
	caddyfile := `{
  email admin@example.com
  debug
}

example.com {
  respond "Hello"
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if opts.RawBlock == "" {
		t.Errorf("Expected RawBlock to be populated, got empty string")
	}

	// RawBlock should contain the content inside the braces
	if !contains(opts.RawBlock, "email") || !contains(opts.RawBlock, "debug") {
		t.Errorf("Expected RawBlock to contain 'email' and 'debug', got '%s'", opts.RawBlock)
	}
}

func TestParseGlobalOptionsWithOrderDirective(t *testing.T) {
	caddyfile := `{
  order rate_limit before basicauth
  order gzip after encode
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	if len(opts.OrderBefore) != 1 || opts.OrderBefore[0] != "rate_limit" {
		t.Errorf("Expected OrderBefore to contain 'rate_limit', got %v", opts.OrderBefore)
	}

	if len(opts.OrderAfter) != 1 || opts.OrderAfter[0] != "gzip" {
		t.Errorf("Expected OrderAfter to contain 'gzip', got %v", opts.OrderAfter)
	}
}

func TestParseGlobalOptionsOnlySnippets(t *testing.T) {
	caddyfile := `(common_headers) {
  header X-Content-Type-Options "nosniff"
}

example.com {
  import common_headers
}`

	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()

	if err != nil {
		t.Fatalf("ParseGlobalOptions returned error: %v", err)
	}

	if opts != nil {
		t.Errorf("Expected nil global options when only snippets exist, got %+v", opts)
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
