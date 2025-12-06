package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// SearchResult represents a single search result item.
type SearchResult struct {
	Type        string `json:"type"`        // "site", "snippet", "log", "page"
	Title       string `json:"title"`       // Display title
	Description string `json:"description"` // Brief description or preview
	URL         string `json:"url"`         // Link to the item
	Icon        string `json:"icon"`        // Icon type for display
	Match       string `json:"match"`       // What part matched the search
}

// SearchData holds data for the search results.
type SearchData struct {
	Query        string         `json:"query"`
	Results      []SearchResult `json:"results"`
	TotalResults int            `json:"totalResults"`
	Error        string         `json:"error,omitempty"`
	HasError     bool           `json:"hasError"`
}

// navigationPages defines the quick navigation pages.
var navigationPages = []SearchResult{
	{Type: "page", Title: "Dashboard", Description: "View server status and overview", URL: "/", Icon: "home"},
	{Type: "page", Title: "Sites", Description: "Manage reverse proxy sites", URL: "/sites", Icon: "globe"},
	{Type: "page", Title: "New Site", Description: "Add a new site configuration", URL: "/sites/new", Icon: "plus"},
	{Type: "page", Title: "Snippets", Description: "Manage reusable configuration snippets", URL: "/snippets", Icon: "code"},
	{Type: "page", Title: "New Snippet", Description: "Create a new snippet", URL: "/snippets/new", Icon: "plus"},
	{Type: "page", Title: "Certificates", Description: "View SSL certificate status", URL: "/certificates", Icon: "shield"},
	{Type: "page", Title: "Global Options", Description: "Configure global Caddy settings", URL: "/global-options", Icon: "settings"},
	{Type: "page", Title: "Logs", Description: "View Caddy access logs", URL: "/logs", Icon: "file-text"},
	{Type: "page", Title: "Containers", Description: "View Docker container status", URL: "/containers", Icon: "box"},
	{Type: "page", Title: "Domains", Description: "Manage domain registrations", URL: "/domains", Icon: "link"},
	{Type: "page", Title: "Notifications", Description: "View system notifications", URL: "/notifications", Icon: "bell"},
	{Type: "page", Title: "History", Description: "View configuration history", URL: "/history", Icon: "clock"},
	{Type: "page", Title: "Import", Description: "Import Caddyfile configuration", URL: "/import", Icon: "upload"},
	{Type: "page", Title: "Users", Description: "Manage user accounts", URL: "/users", Icon: "users"},
	{Type: "page", Title: "Audit Log", Description: "View audit trail", URL: "/audit", Icon: "list"},
	{Type: "page", Title: "Profile", Description: "Manage your profile settings", URL: "/profile", Icon: "user"},
}

// SearchHandler handles search requests.
type SearchHandler struct {
	templates    *templates.Templates
	config       *config.Config
	errorHandler *ErrorHandler
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(tmpl *templates.Templates, cfg *config.Config) *SearchHandler {
	return &SearchHandler{
		templates:    tmpl,
		config:       cfg,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// Search handles GET requests for search.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := SearchData{Query: query}

	if query == "" {
		// Return quick navigation pages
		data.Results = navigationPages
		data.TotalResults = len(data.Results)
	} else {
		// Search across sites, snippets, and pages
		results := h.performSearch(query)
		data.Results = results
		data.TotalResults = len(results)
	}

	// Check if requesting JSON response
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// Return HTML partial for HTMX
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "search-results.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// performSearch searches across all content types.
func (h *SearchHandler) performSearch(query string) []SearchResult {
	var results []SearchResult
	query = strings.ToLower(query)

	// Search navigation pages first
	for _, page := range navigationPages {
		if matchesQuery(page.Title, query) || matchesQuery(page.Description, query) {
			result := page
			result.Match = findMatchContext(page.Title+" "+page.Description, query)
			results = append(results, result)
		}
	}

	// Search sites
	siteResults := h.searchSites(query)
	results = append(results, siteResults...)

	// Search snippets
	snippetResults := h.searchSnippets(query)
	results = append(results, snippetResults...)

	// Sort results by relevance (pages first, then sites, then snippets)
	sort.SliceStable(results, func(i, j int) bool {
		typeOrder := map[string]int{"page": 0, "site": 1, "snippet": 2, "log": 3}
		return typeOrder[results[i].Type] < typeOrder[results[j].Type]
	})

	// Limit results
	if len(results) > 20 {
		results = results[:20]
	}

	return results
}

// searchSites searches for sites matching the query.
func (h *SearchHandler) searchSites(query string) []SearchResult {
	var results []SearchResult

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if !errors.Is(err, caddy.ErrCaddyfileNotFound) {
			return results
		}
		return results
	}

	parser := caddy.NewParser(content)
	sites, err := parser.ParseSites()
	if err != nil {
		return results
	}

	for _, site := range sites {
		// Check if any address matches
		for _, addr := range site.Addresses {
			if matchesQuery(addr, query) {
				results = append(results, SearchResult{
					Type:        "site",
					Title:       addr,
					Description: getSiteDescription(site),
					URL:         "/sites/" + normalizeAddress(addr),
					Icon:        "globe",
					Match:       findMatchContext(addr, query),
				})
				break // Only add once per site
			}
		}

		// Check directives for matches
		if !siteInResults(results, site.Addresses) {
			for _, directive := range site.Directives {
				directiveStr := directive.Name + " " + strings.Join(directive.Args, " ")
				if matchesQuery(directiveStr, query) {
					addr := site.Addresses[0]
					results = append(results, SearchResult{
						Type:        "site",
						Title:       addr,
						Description: getSiteDescription(site),
						URL:         "/sites/" + normalizeAddress(addr),
						Icon:        "globe",
						Match:       findMatchContext(directiveStr, query),
					})
					break
				}
			}
		}
	}

	return results
}

// searchSnippets searches for snippets matching the query.
func (h *SearchHandler) searchSnippets(query string) []SearchResult {
	var results []SearchResult

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		return results
	}

	parser := caddy.NewParser(content)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		return results
	}

	for _, snippet := range snippets {
		snippetContent := snippetToSearchableContent(snippet)

		// Check if name matches
		if matchesQuery(snippet.Name, query) {
			results = append(results, SearchResult{
				Type:        "snippet",
				Title:       snippet.Name,
				Description: snippetSearchPreview(snippet),
				URL:         "/snippets/" + snippet.Name,
				Icon:        "code",
				Match:       findMatchContext(snippet.Name, query),
			})
			continue
		}

		// Check if content matches
		if matchesQuery(snippetContent, query) {
			results = append(results, SearchResult{
				Type:        "snippet",
				Title:       snippet.Name,
				Description: snippetSearchPreview(snippet),
				URL:         "/snippets/" + snippet.Name,
				Icon:        "code",
				Match:       findMatchContext(snippetContent, query),
			})
		}
	}

	return results
}

// snippetToSearchableContent converts a snippet's directives to a searchable string.
func snippetToSearchableContent(snippet caddy.Snippet) string {
	var parts []string
	for _, directive := range snippet.Directives {
		parts = append(parts, directive.Name)
		parts = append(parts, directive.Args...)
	}
	return strings.Join(parts, " ")
}

// snippetSearchPreview generates a preview of snippet content for search results.
func snippetSearchPreview(snippet caddy.Snippet) string {
	if len(snippet.Directives) == 0 {
		return "(empty)"
	}

	var lines []string
	for i, directive := range snippet.Directives {
		if i >= 2 {
			lines = append(lines, "...")
			break
		}
		line := directive.Name
		if len(directive.Args) > 0 {
			line += " " + strings.Join(directive.Args, " ")
		}
		lines = append(lines, line)
	}

	preview := strings.Join(lines, ", ")
	if len(preview) > 60 {
		preview = preview[:60] + "..."
	}
	return preview
}

// matchesQuery checks if text contains the query (case-insensitive).
func matchesQuery(text, query string) bool {
	return strings.Contains(strings.ToLower(text), query)
}

// findMatchContext finds the context around a match for display.
func findMatchContext(text, query string) string {
	lowerText := strings.ToLower(text)
	idx := strings.Index(lowerText, query)
	if idx == -1 {
		// Return first 50 chars as fallback
		if len(text) > 50 {
			return text[:50] + "..."
		}
		return text
	}

	// Extract context around match
	start := idx - 20
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 30
	if end > len(text) {
		end = len(text)
	}

	result := text[start:end]
	if start > 0 {
		result = "..." + result
	}
	if end < len(text) {
		result = result + "..."
	}

	return result
}

// getSiteDescription generates a description for a site.
func getSiteDescription(site caddy.Site) string {
	for _, directive := range site.Directives {
		switch directive.Name {
		case "reverse_proxy":
			if len(directive.Args) > 0 {
				return "Reverse proxy to " + directive.Args[0]
			}
		case "file_server":
			return "Static file server"
		case "redir":
			if len(directive.Args) > 0 {
				return "Redirect to " + directive.Args[0]
			}
		}
	}
	if len(site.Imports) > 0 {
		return "Imports: " + strings.Join(site.Imports, ", ")
	}
	return "Site configuration"
}

// siteInResults checks if a site is already in the results.
func siteInResults(results []SearchResult, addresses []string) bool {
	for _, result := range results {
		for _, addr := range addresses {
			if result.URL == "/sites/"+normalizeAddress(addr) {
				return true
			}
		}
	}
	return false
}
