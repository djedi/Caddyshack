package caddy

import (
	"strings"
	"unicode"
)

// GlobalOptions represents the global options block at the start of a Caddyfile.
// Global options affect all sites and are defined in a block at the very beginning.
type GlobalOptions struct {
	Email       string      // ACME email for certificate registration
	ACMECa      string      // Custom ACME CA endpoint
	Admin       string      // Admin API endpoint (e.g., "off" or "localhost:2019")
	Debug       bool        // Enable debug mode
	LogConfig   *LogConfig  // Global logging configuration
	OrderBefore []string    // Directives to order before others
	OrderAfter  []string    // Directives to order after others
	Servers     []Directive // Server options (nested directives)
	RawBlock    string      // Original raw block content for reference
}

// LogConfig represents logging configuration in global options.
type LogConfig struct {
	Output    string // Output destination (e.g., "file /path/to/log")
	Format    string // Log format (e.g., "json", "console")
	Level     string // Log level (e.g., "info", "debug")
	RollSize  string // Roll size for file output
	RollKeep  string // Number of rolled files to keep
	RawBlock  string // Original raw block content
}

// Directive represents a directive within a site block.
type Directive struct {
	Name    string      // e.g., "reverse_proxy", "handle", "import"
	Args    []string    // Arguments following the directive name
	Block   []Directive // Nested directives (for handle, route, etc.)
	RawLine string      // Original line for preservation
}

// Snippet represents a reusable configuration block in a Caddyfile.
// Snippets are defined with (name) { ... } and used with import name.
type Snippet struct {
	Name       string      // Name of the snippet (without parentheses)
	Directives []Directive // Directives within the snippet
	RawBlock   string      // Original raw block content for reference
}

// Site represents a site block in a Caddyfile.
type Site struct {
	Addresses  []string    // Domain(s) for this site (e.g., ["example.com", "www.example.com"])
	Directives []Directive // Directives within the site block
	Imports    []string    // Imported snippet names
	RawBlock   string      // Original raw block content for reference
}

// Parser handles parsing Caddyfile content into structured data.
type Parser struct {
	content string
	pos     int
	lines   []string
}

// NewParser creates a new Parser for the given Caddyfile content.
func NewParser(content string) *Parser {
	return &Parser{
		content: content,
		lines:   strings.Split(content, "\n"),
	}
}

// ParseSites extracts all site blocks from the Caddyfile.
// It skips global options blocks and snippet definitions.
func (p *Parser) ParseSites() ([]Site, error) {
	var sites []Site
	tokens := p.tokenize()

	i := 0
	for i < len(tokens) {
		token := tokens[i]

		// Skip comments
		if strings.HasPrefix(token, "#") {
			i++
			continue
		}

		// Skip global options block (starts with lone '{')
		if token == "{" && (i == 0 || isGlobalOptionsStart(tokens, i)) {
			depth := 1
			i++
			for i < len(tokens) && depth > 0 {
				if tokens[i] == "{" {
					depth++
				} else if tokens[i] == "}" {
					depth--
				}
				i++
			}
			continue
		}

		// Skip snippet definitions (name starts with '(')
		if strings.HasPrefix(token, "(") && strings.HasSuffix(token, ")") {
			// Skip to the opening brace and then the closing brace
			i++
			for i < len(tokens) && tokens[i] != "{" {
				i++
			}
			if i < len(tokens) {
				depth := 1
				i++
				for i < len(tokens) && depth > 0 {
					if tokens[i] == "{" {
						depth++
					} else if tokens[i] == "}" {
						depth--
					}
					i++
				}
			}
			continue
		}

		// Check if this looks like a site address
		if isSiteAddress(token) {
			site, newIdx := p.parseSiteBlock(tokens, i)
			if site != nil {
				sites = append(sites, *site)
			}
			i = newIdx
			continue
		}

		i++
	}

	return sites, nil
}

// ParseSnippets extracts all snippet definitions from the Caddyfile.
// Snippets are defined as (name) { ... } blocks.
func (p *Parser) ParseSnippets() ([]Snippet, error) {
	var snippets []Snippet
	tokens := p.tokenize()

	i := 0
	for i < len(tokens) {
		token := tokens[i]

		// Skip comments
		if strings.HasPrefix(token, "#") {
			i++
			continue
		}

		// Check for snippet definition (name starts and ends with parentheses)
		if strings.HasPrefix(token, "(") && strings.HasSuffix(token, ")") {
			snippet, newIdx := p.parseSnippetBlock(tokens, i)
			if snippet != nil {
				snippets = append(snippets, *snippet)
			}
			i = newIdx
			continue
		}

		// Skip global options block (starts with lone '{')
		if token == "{" && isGlobalOptionsStart(tokens, i) {
			depth := 1
			i++
			for i < len(tokens) && depth > 0 {
				if tokens[i] == "{" {
					depth++
				} else if tokens[i] == "}" {
					depth--
				}
				i++
			}
			continue
		}

		// Skip site blocks
		if isSiteAddress(token) {
			// Skip to opening brace
			for i < len(tokens) && tokens[i] != "{" {
				i++
			}
			if i < len(tokens) {
				depth := 1
				i++
				for i < len(tokens) && depth > 0 {
					if tokens[i] == "{" {
						depth++
					} else if tokens[i] == "}" {
						depth--
					}
					i++
				}
			}
			continue
		}

		i++
	}

	return snippets, nil
}

// parseSnippetBlock parses a single snippet definition starting at index i.
// Returns the parsed Snippet and the new index after the block.
func (p *Parser) parseSnippetBlock(tokens []string, i int) (*Snippet, int) {
	token := tokens[i]

	// Extract snippet name (remove parentheses)
	name := strings.TrimPrefix(token, "(")
	name = strings.TrimSuffix(name, ")")

	if name == "" {
		return nil, i + 1
	}

	snippet := &Snippet{Name: name}
	i++ // move past snippet name

	// Skip to opening brace
	for i < len(tokens) && tokens[i] != "{" {
		i++
	}

	if i >= len(tokens) {
		return nil, i
	}

	i++ // skip '{'

	// Parse until closing brace
	depth := 1
	startIdx := i

	for i < len(tokens) && depth > 0 {
		if tokens[i] == "{" {
			depth++
		} else if tokens[i] == "}" {
			depth--
			if depth == 0 {
				break
			}
		}
		i++
	}

	// Extract raw block content
	blockTokens := tokens[startIdx:i]
	snippet.RawBlock = strings.Join(blockTokens, " ")

	// Parse directives from block tokens (ignore imports for snippets)
	snippet.Directives, _ = parseDirectives(blockTokens)

	if i < len(tokens) && tokens[i] == "}" {
		i++ // skip closing '}'
	}

	return snippet, i
}

// parseSiteBlock parses a single site block starting at index i.
// Returns the parsed Site and the new index after the block.
func (p *Parser) parseSiteBlock(tokens []string, i int) (*Site, int) {
	site := &Site{}

	// Collect all addresses (could be multiple on same line or consecutive)
	for i < len(tokens) && tokens[i] != "{" {
		addr := tokens[i]
		if strings.HasPrefix(addr, "#") {
			i++
			continue
		}
		if !isSiteAddress(addr) {
			break
		}
		site.Addresses = append(site.Addresses, addr)
		i++
	}

	if len(site.Addresses) == 0 {
		return nil, i
	}

	// Expect opening brace
	if i >= len(tokens) || tokens[i] != "{" {
		return nil, i
	}
	i++ // skip '{'

	// Parse directives until closing brace
	depth := 1
	var rawLines []string
	startIdx := i

	for i < len(tokens) && depth > 0 {
		if tokens[i] == "{" {
			depth++
		} else if tokens[i] == "}" {
			depth--
			if depth == 0 {
				break
			}
		}
		i++
	}

	// Extract raw block content
	blockTokens := tokens[startIdx:i]
	site.RawBlock = strings.Join(blockTokens, " ")
	rawLines = append(rawLines, site.RawBlock)

	// Parse directives from block tokens
	site.Directives, site.Imports = parseDirectives(blockTokens)

	if i < len(tokens) && tokens[i] == "}" {
		i++ // skip closing '}'
	}

	return site, i
}

// parseDirectives parses directives from a slice of tokens within a block.
func parseDirectives(tokens []string) ([]Directive, []string) {
	var directives []Directive
	var imports []string

	i := 0
	for i < len(tokens) {
		token := tokens[i]

		// Skip empty or brace tokens at this level
		if token == "" || token == "{" || token == "}" {
			i++
			continue
		}

		// Skip comments
		if strings.HasPrefix(token, "#") {
			i++
			continue
		}

		directive := Directive{Name: token, RawLine: token}
		i++

		// Collect arguments until we hit a brace, newline-equivalent, or another directive
		for i < len(tokens) {
			t := tokens[i]
			if t == "{" || t == "}" || strings.HasPrefix(t, "#") {
				break
			}
			// Check if this is the start of a new directive (known directive names)
			if isDirectiveName(t) && len(directive.Args) > 0 {
				break
			}
			directive.Args = append(directive.Args, t)
			directive.RawLine += " " + t
			i++
		}

		// Handle nested block
		if i < len(tokens) && tokens[i] == "{" {
			i++ // skip '{'
			depth := 1
			blockStart := i
			for i < len(tokens) && depth > 0 {
				if tokens[i] == "{" {
					depth++
				} else if tokens[i] == "}" {
					depth--
				}
				if depth > 0 {
					i++
				}
			}
			nestedTokens := tokens[blockStart:i]
			directive.Block, _ = parseDirectives(nestedTokens)
			if i < len(tokens) && tokens[i] == "}" {
				i++ // skip '}'
			}
		}

		// Track imports separately
		if directive.Name == "import" && len(directive.Args) > 0 {
			imports = append(imports, directive.Args[0])
		}

		directives = append(directives, directive)
	}

	return directives, imports
}

// tokenize splits the Caddyfile content into tokens.
func (p *Parser) tokenize() []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range p.content {
		switch {
		case inQuote:
			current.WriteRune(r)
			if r == quoteChar {
				inQuote = false
				tokens = append(tokens, current.String())
				current.Reset()
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
			current.WriteRune(r)
		case r == '{' || r == '}':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		case unicode.IsSpace(r):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		case r == '#':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Consume rest of line as comment
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	// Filter out empty tokens
	var filtered []string
	for _, t := range tokens {
		if t != "" {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

// isSiteAddress checks if a token looks like a site address (domain, IP, or :port).
func isSiteAddress(token string) bool {
	if token == "" || token == "{" || token == "}" {
		return false
	}
	if strings.HasPrefix(token, "#") {
		return false
	}
	if strings.HasPrefix(token, "(") {
		return false
	}

	// Common directive names that aren't site addresses
	directives := []string{
		"import", "reverse_proxy", "file_server", "redir", "rewrite",
		"handle", "handle_path", "route", "header", "encode", "log",
		"tls", "root", "php_fastcgi", "respond", "try_files", "basicauth",
		"@", "matcher", "vars", "templates", "push", "request_body",
	}
	for _, d := range directives {
		if token == d || strings.HasPrefix(token, "@") {
			return false
		}
	}

	// Site addresses typically contain dots (domains), colons (ports), or are localhost
	if strings.Contains(token, ".") ||
		strings.HasPrefix(token, ":") ||
		strings.HasPrefix(token, "http://") ||
		strings.HasPrefix(token, "https://") ||
		token == "localhost" ||
		strings.HasPrefix(token, "localhost:") {
		return true
	}

	return false
}

// isGlobalOptionsStart checks if the brace at position i is the start of a global options block.
func isGlobalOptionsStart(tokens []string, i int) bool {
	if i == 0 {
		return true
	}
	// Look back for any site address - if none found, it's global options
	for j := i - 1; j >= 0; j-- {
		t := tokens[j]
		if strings.HasPrefix(t, "#") {
			continue
		}
		if t == "}" {
			// Previous block ended
			return true
		}
		if isSiteAddress(t) || strings.HasPrefix(t, "(") {
			return false
		}
	}
	return true
}

// isDirectiveName checks if a token is a known Caddy directive name.
func isDirectiveName(token string) bool {
	directives := map[string]bool{
		"import": true, "reverse_proxy": true, "file_server": true,
		"redir": true, "rewrite": true, "handle": true, "handle_path": true,
		"route": true, "header": true, "encode": true, "log": true,
		"tls": true, "root": true, "php_fastcgi": true, "respond": true,
		"try_files": true, "basicauth": true, "vars": true, "templates": true,
		"push": true, "request_body": true, "request_header": true,
		"uri": true, "method": true, "copy_response": true,
		"copy_response_headers": true, "abort": true, "error": true,
		"invoke": true, "map": true, "skip_log": true,
	}
	return directives[token] || strings.HasPrefix(token, "@")
}

// ParseGlobalOptions extracts the global options block from the Caddyfile.
// The global options block appears at the start of the file as { ... } without a site address.
// Returns nil if no global options block exists.
func (p *Parser) ParseGlobalOptions() (*GlobalOptions, error) {
	tokens := p.tokenize()

	// Find the global options block (starts with lone '{' at the beginning)
	i := 0

	// Skip leading comments
	for i < len(tokens) && strings.HasPrefix(tokens[i], "#") {
		i++
	}

	// Check if we have a global options block
	if i >= len(tokens) || tokens[i] != "{" {
		return nil, nil
	}

	// Verify this is a global options block (no site address before it)
	if !isGlobalOptionsStart(tokens, i) {
		return nil, nil
	}

	i++ // skip opening '{'

	// Find the closing brace and extract block content
	depth := 1
	startIdx := i
	for i < len(tokens) && depth > 0 {
		if tokens[i] == "{" {
			depth++
		} else if tokens[i] == "}" {
			depth--
			if depth == 0 {
				break
			}
		}
		i++
	}

	blockTokens := tokens[startIdx:i]
	opts := &GlobalOptions{
		RawBlock: strings.Join(blockTokens, " "),
	}

	// Parse the global options from block tokens
	parseGlobalOptionsBlock(blockTokens, opts)

	return opts, nil
}

// parseGlobalOptionsBlock parses the content of a global options block into a GlobalOptions struct.
func parseGlobalOptionsBlock(tokens []string, opts *GlobalOptions) {
	i := 0
	for i < len(tokens) {
		token := tokens[i]

		// Skip empty tokens and braces
		if token == "" || token == "{" || token == "}" {
			i++
			continue
		}

		// Skip comments
		if strings.HasPrefix(token, "#") {
			i++
			continue
		}

		switch token {
		case "email":
			if i+1 < len(tokens) && !isGlobalOptionKeyword(tokens[i+1]) {
				opts.Email = tokens[i+1]
				i += 2
			} else {
				i++
			}

		case "acme_ca":
			if i+1 < len(tokens) && !isGlobalOptionKeyword(tokens[i+1]) {
				opts.ACMECa = tokens[i+1]
				i += 2
			} else {
				i++
			}

		case "admin":
			if i+1 < len(tokens) && !isGlobalOptionKeyword(tokens[i+1]) {
				opts.Admin = tokens[i+1]
				i += 2
			} else {
				i++
			}

		case "debug":
			opts.Debug = true
			i++

		case "order":
			// order directive_name before/after other_directive
			if i+3 < len(tokens) {
				directive := tokens[i+1]
				position := tokens[i+2]
				if position == "before" {
					opts.OrderBefore = append(opts.OrderBefore, directive)
				} else if position == "after" {
					opts.OrderAfter = append(opts.OrderAfter, directive)
				}
				i += 4
			} else {
				i++
			}

		case "log":
			// Parse log block
			logConfig := &LogConfig{}
			i++
			if i < len(tokens) && tokens[i] == "{" {
				i++ // skip '{'
				depth := 1
				logStart := i
				for i < len(tokens) && depth > 0 {
					if tokens[i] == "{" {
						depth++
					} else if tokens[i] == "}" {
						depth--
						if depth == 0 {
							break
						}
					}
					i++
				}
				logTokens := tokens[logStart:i]
				logConfig.RawBlock = strings.Join(logTokens, " ")
				parseLogConfigBlock(logTokens, logConfig)
				if i < len(tokens) && tokens[i] == "}" {
					i++ // skip '}'
				}
			}
			opts.LogConfig = logConfig

		case "servers":
			// Parse servers block
			i++
			if i < len(tokens) && tokens[i] == "{" {
				i++ // skip '{'
				depth := 1
				serverStart := i
				for i < len(tokens) && depth > 0 {
					if tokens[i] == "{" {
						depth++
					} else if tokens[i] == "}" {
						depth--
						if depth == 0 {
							break
						}
					}
					i++
				}
				serverTokens := tokens[serverStart:i]
				opts.Servers, _ = parseDirectives(serverTokens)
				if i < len(tokens) && tokens[i] == "}" {
					i++ // skip '}'
				}
			}

		default:
			i++
		}
	}
}

// parseLogConfigBlock parses the content of a log block within global options.
func parseLogConfigBlock(tokens []string, logConfig *LogConfig) {
	i := 0
	for i < len(tokens) {
		token := tokens[i]

		if token == "" || token == "{" || token == "}" {
			i++
			continue
		}

		switch token {
		case "output":
			// Collect all output args until next keyword or block
			i++
			var outputParts []string
			for i < len(tokens) && tokens[i] != "{" && !isLogKeyword(tokens[i]) {
				outputParts = append(outputParts, tokens[i])
				i++
			}
			logConfig.Output = strings.Join(outputParts, " ")

			// Handle nested output block (e.g., file with roll_size, roll_keep)
			if i < len(tokens) && tokens[i] == "{" {
				i++ // skip '{'
				depth := 1
				for i < len(tokens) && depth > 0 {
					if tokens[i] == "{" {
						depth++
					} else if tokens[i] == "}" {
						depth--
						if depth == 0 {
							break
						}
					}
					// Parse roll_size and roll_keep within output block
					if tokens[i] == "roll_size" && i+1 < len(tokens) {
						logConfig.RollSize = tokens[i+1]
						i += 2
						continue
					}
					if tokens[i] == "roll_keep" && i+1 < len(tokens) {
						logConfig.RollKeep = tokens[i+1]
						i += 2
						continue
					}
					i++
				}
				if i < len(tokens) && tokens[i] == "}" {
					i++ // skip '}'
				}
			}

		case "format":
			if i+1 < len(tokens) && !isLogKeyword(tokens[i+1]) {
				logConfig.Format = tokens[i+1]
				i += 2
			} else {
				i++
			}

		case "level":
			if i+1 < len(tokens) && !isLogKeyword(tokens[i+1]) {
				logConfig.Level = tokens[i+1]
				i += 2
			} else {
				i++
			}

		default:
			i++
		}
	}
}

// isGlobalOptionKeyword checks if a token is a known global option keyword.
func isGlobalOptionKeyword(token string) bool {
	keywords := map[string]bool{
		"email": true, "acme_ca": true, "admin": true, "debug": true,
		"log": true, "order": true, "servers": true, "storage": true,
		"grace_period": true, "shutdown_delay": true, "auto_https": true,
		"http_port": true, "https_port": true, "default_sni": true,
		"local_certs": true, "skip_install_trust": true, "acme_dns": true,
		"acme_eab": true, "ocsp_stapling": true, "cert_issuer": true,
		"key_type": true, "default_bind": true, "persist_config": true,
		"{": true, "}": true,
	}
	return keywords[token]
}

// isLogKeyword checks if a token is a known log configuration keyword.
func isLogKeyword(token string) bool {
	keywords := map[string]bool{
		"output": true, "format": true, "level": true, "include": true,
		"exclude": true, "sampling": true, "{": true, "}": true,
	}
	return keywords[token]
}

// ParseAll parses the entire Caddyfile and returns all components.
// This is a convenience method that calls ParseGlobalOptions, ParseSnippets, and ParseSites.
func (p *Parser) ParseAll() (*Caddyfile, error) {
	globalOpts, err := p.ParseGlobalOptions()
	if err != nil {
		return nil, err
	}

	snippets, err := p.ParseSnippets()
	if err != nil {
		return nil, err
	}

	sites, err := p.ParseSites()
	if err != nil {
		return nil, err
	}

	return &Caddyfile{
		GlobalOptions: globalOpts,
		Snippets:      snippets,
		Sites:         sites,
	}, nil
}
