package caddy

import (
	"strings"
	"testing"
)

func TestWriteSiteSimple(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com"},
		Directives: []Directive{
			{Name: "reverse_proxy", Args: []string{"localhost:8080"}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `example.com {
	reverse_proxy localhost:8080
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSiteMultipleAddresses(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com", "www.example.com"},
		Directives: []Directive{
			{Name: "reverse_proxy", Args: []string{"localhost:8080"}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `example.com www.example.com {
	reverse_proxy localhost:8080
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSiteWithImports(t *testing.T) {
	site := &Site{
		Addresses: []string{"portainer.redseam.com"},
		Directives: []Directive{
			{Name: "import", Args: []string{"proxy_headers"}},
			{Name: "import", Args: []string{"site_log"}},
			{Name: "reverse_proxy", Args: []string{"http://108.181.221.120:9000"}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `portainer.redseam.com {
	import proxy_headers
	import site_log
	reverse_proxy http://108.181.221.120:9000
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSiteWithNestedBlock(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com"},
		Directives: []Directive{
			{
				Name: "handle",
				Args: []string{"/api/*"},
				Block: []Directive{
					{Name: "reverse_proxy", Args: []string{"localhost:3000"}},
				},
			},
			{
				Name: "handle",
				Block: []Directive{
					{Name: "file_server"},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `example.com {
	handle /api/* {
		reverse_proxy localhost:3000
	}
	handle {
		file_server
	}
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSiteWithDeeplyNestedBlocks(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com"},
		Directives: []Directive{
			{
				Name: "handle",
				Args: []string{"/api"},
				Block: []Directive{
					{
						Name: "reverse_proxy",
						Args: []string{"localhost:3000"},
						Block: []Directive{
							{Name: "header_up", Args: []string{"Host", "example.com"}},
						},
					},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `example.com {
	handle /api {
		reverse_proxy localhost:3000 {
			header_up Host example.com
		}
	}
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSiteWithQuotedArgs(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com"},
		Directives: []Directive{
			{Name: "respond", Args: []string{"Hello World"}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	// Should quote the argument since it contains a space
	if !strings.Contains(result, "\"Hello World\"") {
		t.Errorf("Expected argument to be quoted, got:\n%s", result)
	}
}

func TestWriteSiteWithMatcher(t *testing.T) {
	site := &Site{
		Addresses: []string{"example.com"},
		Directives: []Directive{
			{Name: "@api", Args: []string{"path", "/api/*"}},
			{Name: "handle", Args: []string{"@api"}, Block: []Directive{
				{Name: "reverse_proxy", Args: []string{"localhost:3000"}},
			}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	expected := `example.com {
	@api path /api/*
	handle @api {
		reverse_proxy localhost:3000
	}
}
`

	if result != expected {
		t.Errorf("WriteSite output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSites(t *testing.T) {
	sites := []Site{
		{
			Addresses:  []string{"site1.com"},
			Directives: []Directive{{Name: "respond", Args: []string{"Site 1"}}},
		},
		{
			Addresses:  []string{"site2.com"},
			Directives: []Directive{{Name: "respond", Args: []string{"Site 2"}}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSites(sites)

	// Should contain both sites
	if !strings.Contains(result, "site1.com") || !strings.Contains(result, "site2.com") {
		t.Errorf("WriteSites should contain both sites, got:\n%s", result)
	}

	// Should have a blank line between sites
	if !strings.Contains(result, "}\n\nsite2.com") {
		t.Errorf("Expected blank line between sites, got:\n%s", result)
	}
}

func TestWriteSnippet(t *testing.T) {
	snippet := &Snippet{
		Name: "proxy_headers",
		Directives: []Directive{
			{
				Name: "header",
				Block: []Directive{
					{Name: "X-Proxied-By", Args: []string{"\"Caddy\""}},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteSnippet(snippet)

	expected := `(proxy_headers) {
	header {
		X-Proxied-By "Caddy"
	}
}
`

	if result != expected {
		t.Errorf("WriteSnippet output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteSnippets(t *testing.T) {
	snippets := []Snippet{
		{
			Name:       "snippet1",
			Directives: []Directive{{Name: "header", Args: []string{"X-One", "1"}}},
		},
		{
			Name:       "snippet2",
			Directives: []Directive{{Name: "header", Args: []string{"X-Two", "2"}}},
		},
	}

	writer := NewWriter()
	result := writer.WriteSnippets(snippets)

	// Should contain both snippets
	if !strings.Contains(result, "(snippet1)") || !strings.Contains(result, "(snippet2)") {
		t.Errorf("WriteSnippets should contain both snippets, got:\n%s", result)
	}
}

func TestWriteGlobalOptionsSimple(t *testing.T) {
	opts := &GlobalOptions{
		Email: "admin@example.com",
	}

	writer := NewWriter()
	result := writer.WriteGlobalOptions(opts)

	expected := `{
	email admin@example.com
}
`

	if result != expected {
		t.Errorf("WriteGlobalOptions output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteGlobalOptionsWithLog(t *testing.T) {
	opts := &GlobalOptions{
		Email: "admin@example.com",
		LogConfig: &LogConfig{
			Output:   "file /var/log/caddy/access.log",
			Format:   "json",
			RollSize: "10mb",
			RollKeep: "5",
		},
	}

	writer := NewWriter()
	result := writer.WriteGlobalOptions(opts)

	// Should contain email
	if !strings.Contains(result, "email admin@example.com") {
		t.Errorf("Expected email in output, got:\n%s", result)
	}

	// Should contain log block
	if !strings.Contains(result, "log {") {
		t.Errorf("Expected log block in output, got:\n%s", result)
	}

	// Should contain roll_size and roll_keep
	if !strings.Contains(result, "roll_size 10mb") {
		t.Errorf("Expected roll_size in output, got:\n%s", result)
	}

	if !strings.Contains(result, "roll_keep 5") {
		t.Errorf("Expected roll_keep in output, got:\n%s", result)
	}
}

func TestWriteGlobalOptionsWithDebug(t *testing.T) {
	opts := &GlobalOptions{
		Email: "admin@example.com",
		Debug: true,
	}

	writer := NewWriter()
	result := writer.WriteGlobalOptions(opts)

	if !strings.Contains(result, "debug") {
		t.Errorf("Expected debug in output, got:\n%s", result)
	}
}

func TestWriteGlobalOptionsWithAdmin(t *testing.T) {
	opts := &GlobalOptions{
		Admin: "off",
	}

	writer := NewWriter()
	result := writer.WriteGlobalOptions(opts)

	if !strings.Contains(result, "admin off") {
		t.Errorf("Expected 'admin off' in output, got:\n%s", result)
	}
}

func TestWriteGlobalOptionsNil(t *testing.T) {
	writer := NewWriter()
	result := writer.WriteGlobalOptions(nil)

	if result != "" {
		t.Errorf("Expected empty string for nil options, got:\n%s", result)
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	writer := NewWriter()

	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "\"with space\""},
		{"\"already quoted\"", "\"already quoted\""},   // Already quoted (starts and ends with quotes)
		{"'single quoted'", "'single quoted'"},         // Already single quoted
		{"has{brace", "\"has{brace\""},
		{"has}brace", "\"has}brace\""},
		{"has\ttab", "\"has\ttab\""},
	}

	for _, tc := range tests {
		result := writer.quoteIfNeeded(tc.input)
		if result != tc.expected {
			t.Errorf("quoteIfNeeded(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

// Test round-trip: parse -> write -> parse should produce equivalent structures
func TestParseWriteRoundTrip(t *testing.T) {
	caddyfile := `example.com www.example.com {
	import common_headers
	reverse_proxy localhost:8080
	handle /api/* {
		reverse_proxy localhost:3000
	}
}
`

	// Parse original
	parser := NewParser(caddyfile)
	sites, err := parser.ParseSites()
	if err != nil {
		t.Fatalf("Failed to parse original: %v", err)
	}

	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}

	// Write back
	writer := NewWriter()
	written := writer.WriteSite(&sites[0])

	// Parse the written output
	parser2 := NewParser(written)
	sites2, err := parser2.ParseSites()
	if err != nil {
		t.Fatalf("Failed to parse written output: %v", err)
	}

	if len(sites2) != 1 {
		t.Fatalf("Expected 1 site after round-trip, got %d", len(sites2))
	}

	// Compare addresses
	if len(sites[0].Addresses) != len(sites2[0].Addresses) {
		t.Errorf("Address count mismatch: %d vs %d", len(sites[0].Addresses), len(sites2[0].Addresses))
	}

	for i, addr := range sites[0].Addresses {
		if addr != sites2[0].Addresses[i] {
			t.Errorf("Address mismatch at %d: %s vs %s", i, addr, sites2[0].Addresses[i])
		}
	}

	// Compare imports
	if len(sites[0].Imports) != len(sites2[0].Imports) {
		t.Errorf("Import count mismatch: %d vs %d", len(sites[0].Imports), len(sites2[0].Imports))
	}
}

func TestParseWriteSnippetRoundTrip(t *testing.T) {
	caddyfile := `(common_headers) {
	header X-Content-Type-Options nosniff
	header X-Frame-Options DENY
}
`

	// Parse original
	parser := NewParser(caddyfile)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		t.Fatalf("Failed to parse original: %v", err)
	}

	if len(snippets) != 1 {
		t.Fatalf("Expected 1 snippet, got %d", len(snippets))
	}

	// Write back
	writer := NewWriter()
	written := writer.WriteSnippet(&snippets[0])

	// Parse the written output
	parser2 := NewParser(written)
	snippets2, err := parser2.ParseSnippets()
	if err != nil {
		t.Fatalf("Failed to parse written output: %v", err)
	}

	if len(snippets2) != 1 {
		t.Fatalf("Expected 1 snippet after round-trip, got %d", len(snippets2))
	}

	// Compare names
	if snippets[0].Name != snippets2[0].Name {
		t.Errorf("Snippet name mismatch: %s vs %s", snippets[0].Name, snippets2[0].Name)
	}
}

func TestParseWriteGlobalOptionsRoundTrip(t *testing.T) {
	caddyfile := `{
	email admin@example.com
	debug
}
`

	// Parse original
	parser := NewParser(caddyfile)
	opts, err := parser.ParseGlobalOptions()
	if err != nil {
		t.Fatalf("Failed to parse original: %v", err)
	}

	if opts == nil {
		t.Fatalf("Expected global options, got nil")
	}

	// Write back
	writer := NewWriter()
	written := writer.WriteGlobalOptions(opts)

	// Parse the written output
	parser2 := NewParser(written)
	opts2, err := parser2.ParseGlobalOptions()
	if err != nil {
		t.Fatalf("Failed to parse written output: %v", err)
	}

	if opts2 == nil {
		t.Fatalf("Expected global options after round-trip, got nil")
	}

	// Compare email
	if opts.Email != opts2.Email {
		t.Errorf("Email mismatch: %s vs %s", opts.Email, opts2.Email)
	}

	// Compare debug
	if opts.Debug != opts2.Debug {
		t.Errorf("Debug mismatch: %v vs %v", opts.Debug, opts2.Debug)
	}
}

func TestWriteComplexSiteFromPromptMd(t *testing.T) {
	// Recreate a site similar to dustytune.com from prompt.md
	site := &Site{
		Addresses: []string{"dustytune.com"},
		Directives: []Directive{
			{Name: "import", Args: []string{"proxy_headers"}},
			{Name: "import", Args: []string{"site_log"}},
			{Name: "import", Args: []string{"php_bot_trap"}},
			{Name: "reverse_proxy", Args: []string{"108.181.221.120:3080"}},
			{
				Name: "handle",
				Args: []string{"/9OoybqZpHP.js"},
				Block: []Directive{
					{Name: "rewrite", Args: []string{"*", "/sa.js"}},
					{
						Name: "reverse_proxy",
						Args: []string{"https://slimlytics.redseam.com"},
						Block: []Directive{
							{Name: "header_up", Args: []string{"Host", "slimlytics.redseam.com"}},
						},
					},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteSite(site)

	// Verify key elements are present
	if !strings.Contains(result, "dustytune.com {") {
		t.Errorf("Missing site address in output:\n%s", result)
	}

	if !strings.Contains(result, "import proxy_headers") {
		t.Errorf("Missing import proxy_headers in output:\n%s", result)
	}

	if !strings.Contains(result, "handle /9OoybqZpHP.js {") {
		t.Errorf("Missing handle block in output:\n%s", result)
	}

	if !strings.Contains(result, "header_up Host slimlytics.redseam.com") {
		t.Errorf("Missing nested header_up in output:\n%s", result)
	}
}

func TestWriteCaddyfileNil(t *testing.T) {
	writer := NewWriter()
	result := writer.WriteCaddyfile(nil)

	if result != "" {
		t.Errorf("Expected empty string for nil Caddyfile, got:\n%s", result)
	}
}

func TestWriteCaddyfileEmpty(t *testing.T) {
	writer := NewWriter()
	result := writer.WriteCaddyfile(&Caddyfile{})

	if result != "" {
		t.Errorf("Expected empty string for empty Caddyfile, got:\n%s", result)
	}
}

func TestWriteCaddyfileGlobalOptionsOnly(t *testing.T) {
	cf := &Caddyfile{
		GlobalOptions: &GlobalOptions{
			Email: "admin@example.com",
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	// When only global options are present, there's a trailing newline for separation
	expected := "{" + "\n" +
		"\temail admin@example.com" + "\n" +
		"}" + "\n" +
		"\n"

	if result != expected {
		t.Errorf("WriteCaddyfile output mismatch.\nExpected:\n%q\nGot:\n%q", expected, result)
	}
}

func TestWriteCaddyfileSitesOnly(t *testing.T) {
	cf := &Caddyfile{
		Sites: []Site{
			{
				Addresses:  []string{"example.com"},
				Directives: []Directive{{Name: "respond", Args: []string{"Hello"}}},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	expected := `example.com {
	respond Hello
}
`

	if result != expected {
		t.Errorf("WriteCaddyfile output mismatch.\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestWriteCaddyfileSnippetsOnly(t *testing.T) {
	cf := &Caddyfile{
		Snippets: []Snippet{
			{
				Name:       "common",
				Directives: []Directive{{Name: "header", Args: []string{"X-Test", "1"}}},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	// When only snippets are present, there's a trailing newline for separation
	expected := "(common) {" + "\n" +
		"\theader X-Test 1" + "\n" +
		"}" + "\n" +
		"\n"

	if result != expected {
		t.Errorf("WriteCaddyfile output mismatch.\nExpected:\n%q\nGot:\n%q", expected, result)
	}
}

func TestWriteCaddyfileFullExample(t *testing.T) {
	cf := &Caddyfile{
		GlobalOptions: &GlobalOptions{
			Email: "admin@example.com",
		},
		Snippets: []Snippet{
			{
				Name: "proxy_headers",
				Directives: []Directive{
					{
						Name: "header",
						Block: []Directive{
							{Name: "X-Proxied-By", Args: []string{"\"Caddy\""}},
						},
					},
				},
			},
		},
		Sites: []Site{
			{
				Addresses: []string{"example.com"},
				Directives: []Directive{
					{Name: "import", Args: []string{"proxy_headers"}},
					{Name: "reverse_proxy", Args: []string{"localhost:8080"}},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	// Check order: global options, then snippets, then sites
	globalIdx := strings.Index(result, "email admin@example.com")
	snippetIdx := strings.Index(result, "(proxy_headers)")
	siteIdx := strings.Index(result, "example.com {")

	if globalIdx == -1 {
		t.Error("Missing global options in output")
	}
	if snippetIdx == -1 {
		t.Error("Missing snippet in output")
	}
	if siteIdx == -1 {
		t.Error("Missing site in output")
	}

	if globalIdx > snippetIdx {
		t.Error("Global options should appear before snippets")
	}
	if snippetIdx > siteIdx {
		t.Error("Snippets should appear before sites")
	}
}

func TestWriteCaddyfileMultipleSites(t *testing.T) {
	cf := &Caddyfile{
		Sites: []Site{
			{
				Addresses:  []string{"site1.com"},
				Directives: []Directive{{Name: "respond", Args: []string{"Site 1"}}},
			},
			{
				Addresses:  []string{"site2.com"},
				Directives: []Directive{{Name: "respond", Args: []string{"Site 2"}}},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	// Should have both sites with proper separation
	if !strings.Contains(result, "site1.com {") {
		t.Error("Missing site1.com in output")
	}
	if !strings.Contains(result, "site2.com {") {
		t.Error("Missing site2.com in output")
	}

	// Sites should be separated by blank line
	if !strings.Contains(result, "}\n\nsite2.com") {
		t.Errorf("Expected blank line between sites, got:\n%s", result)
	}
}

func TestWriteCaddyfileRoundTrip(t *testing.T) {
	// Test the full round trip: parse -> write -> parse
	input := `{
	email admin@example.com
}

(proxy_headers) {
	header X-Proxied-By Caddy
}

example.com {
	import proxy_headers
	reverse_proxy localhost:8080
}
`

	// Parse original
	parser := NewParser(input)
	cf, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse original: %v", err)
	}

	// Write back
	writer := NewWriter()
	output := writer.WriteCaddyfile(cf)

	// Parse the written output
	parser2 := NewParser(output)
	cf2, err := parser2.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse written output: %v", err)
	}

	// Compare components
	if cf.GlobalOptions == nil && cf2.GlobalOptions != nil {
		t.Error("Global options mismatch after round-trip")
	}
	if cf.GlobalOptions != nil && cf2.GlobalOptions == nil {
		t.Error("Global options mismatch after round-trip")
	}
	if cf.GlobalOptions != nil && cf2.GlobalOptions != nil {
		if cf.GlobalOptions.Email != cf2.GlobalOptions.Email {
			t.Errorf("Email mismatch: %s vs %s", cf.GlobalOptions.Email, cf2.GlobalOptions.Email)
		}
	}

	if len(cf.Snippets) != len(cf2.Snippets) {
		t.Errorf("Snippet count mismatch: %d vs %d", len(cf.Snippets), len(cf2.Snippets))
	}

	if len(cf.Sites) != len(cf2.Sites) {
		t.Errorf("Site count mismatch: %d vs %d", len(cf.Sites), len(cf2.Sites))
	}
}

func TestWriteCaddyfileFromPromptMd(t *testing.T) {
	// Test with a Caddyfile similar to the example in prompt.md
	cf := &Caddyfile{
		GlobalOptions: &GlobalOptions{
			Email: "dustin@redseam.com",
			LogConfig: &LogConfig{
				Output:   "file /var/log/caddy/access.log",
				Format:   "json",
				RollSize: "10mb",
				RollKeep: "5",
			},
		},
		Snippets: []Snippet{
			{
				Name: "site_log",
				Directives: []Directive{
					{
						Name: "log",
						Block: []Directive{
							{Name: "output", Args: []string{"file", "/var/log/caddy/access.log"}},
							{Name: "format", Args: []string{"json"}},
						},
					},
				},
			},
			{
				Name: "proxy_headers",
				Directives: []Directive{
					{
						Name: "header",
						Block: []Directive{
							{Name: "X-Proxied-By", Args: []string{"\"Caddy\""}},
						},
					},
				},
			},
		},
		Sites: []Site{
			{
				Addresses: []string{"portainer.redseam.com"},
				Directives: []Directive{
					{Name: "import", Args: []string{"proxy_headers"}},
					{Name: "import", Args: []string{"site_log"}},
					{Name: "reverse_proxy", Args: []string{"http://108.181.221.120:9000"}},
				},
			},
			{
				Addresses: []string{"dustytune.com"},
				Directives: []Directive{
					{Name: "import", Args: []string{"proxy_headers"}},
					{Name: "import", Args: []string{"site_log"}},
					{Name: "reverse_proxy", Args: []string{"108.181.221.120:3080"}},
				},
			},
		},
	}

	writer := NewWriter()
	result := writer.WriteCaddyfile(cf)

	// Verify structure
	if !strings.Contains(result, "email dustin@redseam.com") {
		t.Error("Missing email in output")
	}

	if !strings.Contains(result, "(site_log)") {
		t.Error("Missing site_log snippet in output")
	}

	if !strings.Contains(result, "(proxy_headers)") {
		t.Error("Missing proxy_headers snippet in output")
	}

	if !strings.Contains(result, "portainer.redseam.com {") {
		t.Error("Missing portainer.redseam.com site in output")
	}

	if !strings.Contains(result, "dustytune.com {") {
		t.Error("Missing dustytune.com site in output")
	}

	// Verify order
	emailIdx := strings.Index(result, "email dustin@redseam.com")
	snippetIdx := strings.Index(result, "(site_log)")
	siteIdx := strings.Index(result, "portainer.redseam.com {")

	if emailIdx > snippetIdx {
		t.Error("Global options should appear before snippets")
	}
	if snippetIdx > siteIdx {
		t.Error("Snippets should appear before sites")
	}
}
