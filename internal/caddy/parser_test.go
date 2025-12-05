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
