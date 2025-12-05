package caddy

import (
	"strings"
)

// Writer handles generating Caddyfile content from structured data.
type Writer struct {
	indent string
}

// NewWriter creates a new Writer with default settings.
func NewWriter() *Writer {
	return &Writer{
		indent: "\t",
	}
}

// WriteSite generates a Caddyfile site block from a Site struct.
func (w *Writer) WriteSite(site *Site) string {
	var sb strings.Builder

	// Write addresses
	sb.WriteString(strings.Join(site.Addresses, " "))
	sb.WriteString(" {\n")

	// Write directives
	for _, directive := range site.Directives {
		w.writeDirective(&sb, directive, 1)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// writeDirective writes a single directive with proper indentation.
func (w *Writer) writeDirective(sb *strings.Builder, directive Directive, depth int) {
	indent := strings.Repeat(w.indent, depth)

	// Write the directive name
	sb.WriteString(indent)
	sb.WriteString(directive.Name)

	// Write arguments
	for _, arg := range directive.Args {
		sb.WriteString(" ")
		sb.WriteString(w.quoteIfNeeded(arg))
	}

	// If there's a nested block, write it
	if len(directive.Block) > 0 {
		sb.WriteString(" {\n")
		for _, nested := range directive.Block {
			w.writeDirective(sb, nested, depth+1)
		}
		sb.WriteString(indent)
		sb.WriteString("}")
	}

	sb.WriteString("\n")
}

// quoteIfNeeded adds quotes around a string if it contains spaces or special characters.
func (w *Writer) quoteIfNeeded(s string) string {
	// If already quoted, return as-is
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		return s
	}

	// Check if quoting is needed
	needsQuotes := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '{' || r == '}' || r == '"' {
			needsQuotes = true
			break
		}
	}

	if needsQuotes {
		// Escape any existing quotes and wrap in quotes
		escaped := strings.ReplaceAll(s, "\"", "\\\"")
		return "\"" + escaped + "\""
	}

	return s
}

// WriteSites generates Caddyfile content for multiple sites.
func (w *Writer) WriteSites(sites []Site) string {
	var sb strings.Builder

	for i, site := range sites {
		sb.WriteString(w.WriteSite(&site))
		if i < len(sites)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// WriteSnippet generates a Caddyfile snippet definition from a Snippet struct.
func (w *Writer) WriteSnippet(snippet *Snippet) string {
	var sb strings.Builder

	// Write snippet name in parentheses
	sb.WriteString("(")
	sb.WriteString(snippet.Name)
	sb.WriteString(") {\n")

	// Write directives
	for _, directive := range snippet.Directives {
		w.writeDirective(&sb, directive, 1)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// WriteSnippets generates Caddyfile content for multiple snippets.
func (w *Writer) WriteSnippets(snippets []Snippet) string {
	var sb strings.Builder

	for i, snippet := range snippets {
		sb.WriteString(w.WriteSnippet(&snippet))
		if i < len(snippets)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// WriteGlobalOptions generates a Caddyfile global options block from a GlobalOptions struct.
func (w *Writer) WriteGlobalOptions(opts *GlobalOptions) string {
	if opts == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("{\n")

	// Write email
	if opts.Email != "" {
		sb.WriteString(w.indent)
		sb.WriteString("email ")
		sb.WriteString(opts.Email)
		sb.WriteString("\n")
	}

	// Write ACME CA
	if opts.ACMECa != "" {
		sb.WriteString(w.indent)
		sb.WriteString("acme_ca ")
		sb.WriteString(opts.ACMECa)
		sb.WriteString("\n")
	}

	// Write admin
	if opts.Admin != "" {
		sb.WriteString(w.indent)
		sb.WriteString("admin ")
		sb.WriteString(opts.Admin)
		sb.WriteString("\n")
	}

	// Write debug
	if opts.Debug {
		sb.WriteString(w.indent)
		sb.WriteString("debug\n")
	}

	// Write order directives (before)
	for _, directive := range opts.OrderBefore {
		sb.WriteString(w.indent)
		sb.WriteString("order ")
		sb.WriteString(directive)
		sb.WriteString(" before\n")
	}

	// Write order directives (after)
	for _, directive := range opts.OrderAfter {
		sb.WriteString(w.indent)
		sb.WriteString("order ")
		sb.WriteString(directive)
		sb.WriteString(" after\n")
	}

	// Write log config
	if opts.LogConfig != nil {
		w.writeLogConfig(&sb, opts.LogConfig)
	}

	// Write servers config
	if len(opts.Servers) > 0 {
		sb.WriteString(w.indent)
		sb.WriteString("servers {\n")
		for _, directive := range opts.Servers {
			w.writeDirective(&sb, directive, 2)
		}
		sb.WriteString(w.indent)
		sb.WriteString("}\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

// Caddyfile represents a complete parsed Caddyfile with all its components.
type Caddyfile struct {
	GlobalOptions *GlobalOptions
	Snippets      []Snippet
	Sites         []Site
	Comments      []Comment // Top-level comments to preserve
}

// Comment represents a comment line in a Caddyfile.
type Comment struct {
	Text     string // The comment text (including the #)
	Position string // "global", "snippet", "site", or "top" for top-level comments
	After    string // Name of snippet/site this comment appears after, empty for top
}

// WriteCaddyfile generates a complete Caddyfile from all components.
// Order: global options, snippets, sites.
func (w *Writer) WriteCaddyfile(cf *Caddyfile) string {
	if cf == nil {
		return ""
	}

	var sb strings.Builder

	// Write global options first (if present)
	if cf.GlobalOptions != nil {
		globalOpts := w.WriteGlobalOptions(cf.GlobalOptions)
		if globalOpts != "" {
			sb.WriteString(globalOpts)
			sb.WriteString("\n")
		}
	}

	// Write snippets
	if len(cf.Snippets) > 0 {
		sb.WriteString(w.WriteSnippets(cf.Snippets))
		sb.WriteString("\n")
	}

	// Write sites
	if len(cf.Sites) > 0 {
		sb.WriteString(w.WriteSites(cf.Sites))
	}

	return sb.String()
}

// writeLogConfig writes the log configuration block.
func (w *Writer) writeLogConfig(sb *strings.Builder, logConfig *LogConfig) {
	sb.WriteString(w.indent)
	sb.WriteString("log {\n")

	indent2 := strings.Repeat(w.indent, 2)
	indent3 := strings.Repeat(w.indent, 3)

	// Write output
	if logConfig.Output != "" {
		sb.WriteString(indent2)
		sb.WriteString("output ")
		sb.WriteString(logConfig.Output)

		// Check if we need a nested block for roll_size/roll_keep
		if logConfig.RollSize != "" || logConfig.RollKeep != "" {
			sb.WriteString(" {\n")
			if logConfig.RollSize != "" {
				sb.WriteString(indent3)
				sb.WriteString("roll_size ")
				sb.WriteString(logConfig.RollSize)
				sb.WriteString("\n")
			}
			if logConfig.RollKeep != "" {
				sb.WriteString(indent3)
				sb.WriteString("roll_keep ")
				sb.WriteString(logConfig.RollKeep)
				sb.WriteString("\n")
			}
			sb.WriteString(indent2)
			sb.WriteString("}\n")
		} else {
			sb.WriteString("\n")
		}
	}

	// Write format
	if logConfig.Format != "" {
		sb.WriteString(indent2)
		sb.WriteString("format ")
		sb.WriteString(logConfig.Format)
		sb.WriteString("\n")
	}

	// Write level
	if logConfig.Level != "" {
		sb.WriteString(indent2)
		sb.WriteString("level ")
		sb.WriteString(logConfig.Level)
		sb.WriteString("\n")
	}

	sb.WriteString(w.indent)
	sb.WriteString("}\n")
}
