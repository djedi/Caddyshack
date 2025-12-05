package caddy

import (
	"strings"
	"unicode"
)

// Directive represents a directive within a site block.
type Directive struct {
	Name    string      // e.g., "reverse_proxy", "handle", "import"
	Args    []string    // Arguments following the directive name
	Block   []Directive // Nested directives (for handle, route, etc.)
	RawLine string      // Original line for preservation
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
