package handlers

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// SitesData holds data displayed on the sites list page.
type SitesData struct {
	Sites    []caddy.Site
	Error    string
	HasError bool
}

// SiteDetailData holds data displayed on the site detail page.
type SiteDetailData struct {
	Site     SiteView
	Error    string
	HasError bool
}

// SiteFormData holds data for the site add/edit form.
type SiteFormData struct {
	Site     *SiteFormValues // nil for new site, populated for edit
	Error    string
	HasError bool
}

// SiteFormValues represents the form field values for creating/editing a site.
type SiteFormValues struct {
	Domain       string
	Type         string // "reverse_proxy", "static", "redirect"
	Target       string // for reverse_proxy
	RootPath     string // for static
	RedirectUrl  string // for redirect
	RedirectCode string // for redirect (301, 302, etc.)
	EnableTls    bool
}

// SiteView is a view model for a single site with helper fields.
type SiteView struct {
	caddy.Site
	PrimaryAddress string // First address for display/linking
	FormattedBlock string // Formatted raw block for display
}

// SitesHandler handles requests for the sites pages.
type SitesHandler struct {
	templates *templates.Templates
	config    *config.Config
}

// NewSitesHandler creates a new SitesHandler.
func NewSitesHandler(tmpl *templates.Templates, cfg *config.Config) *SitesHandler {
	return &SitesHandler{
		templates: tmpl,
		config:    cfg,
	}
}

// List handles GET requests for the sites list page.
func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	data := SitesData{}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if errors.Is(err, caddy.ErrCaddyfileNotFound) {
			data.Error = "Caddyfile not found at " + h.config.CaddyfilePath
		} else {
			data.Error = "Failed to read Caddyfile: " + err.Error()
		}
		data.HasError = true
	} else {
		// Parse sites from the Caddyfile
		parser := caddy.NewParser(content)
		sites, err := parser.ParseSites()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			data.Sites = sites
		}
	}

	pageData := templates.PageData{
		Title:     "Sites",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "sites.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Detail handles GET requests for the site detail page.
func (h *SitesHandler) Detail(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path (e.g., /sites/example.com)
	path := r.URL.Path
	domain := strings.TrimPrefix(path, "/sites/")
	domain = strings.TrimSuffix(domain, "/")

	if domain == "" {
		http.Redirect(w, r, "/sites", http.StatusFound)
		return
	}

	data := SiteDetailData{}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if errors.Is(err, caddy.ErrCaddyfileNotFound) {
			data.Error = "Caddyfile not found at " + h.config.CaddyfilePath
		} else {
			data.Error = "Failed to read Caddyfile: " + err.Error()
		}
		data.HasError = true
	} else {
		// Parse sites from the Caddyfile
		parser := caddy.NewParser(content)
		sites, err := parser.ParseSites()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			// Find the site matching the domain
			var found *caddy.Site
			for i := range sites {
				for _, addr := range sites[i].Addresses {
					if addr == domain {
						found = &sites[i]
						break
					}
				}
				if found != nil {
					break
				}
			}

			if found == nil {
				data.Error = "Site not found: " + domain
				data.HasError = true
			} else {
				data.Site = SiteView{
					Site:           *found,
					PrimaryAddress: found.Addresses[0],
					FormattedBlock: formatRawBlock(found.RawBlock),
				}
			}
		}
	}

	pageData := templates.PageData{
		Title:     domain + " - Site Details",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-detail.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// formatRawBlock formats a raw block string for display.
// It adds proper indentation for readability.
func formatRawBlock(raw string) string {
	if raw == "" {
		return ""
	}

	// The raw block is stored as space-separated tokens.
	// Convert it into a more readable multi-line format.
	var result strings.Builder
	depth := 0
	tokens := strings.Fields(raw)

	for i, token := range tokens {
		if token == "}" {
			depth--
			if depth < 0 {
				depth = 0
			}
			result.WriteString(strings.Repeat("    ", depth))
			result.WriteString("}\n")
		} else if token == "{" {
			result.WriteString("{\n")
			depth++
		} else {
			// Check if next token is "{" for inline brace
			if i+1 < len(tokens) && tokens[i+1] == "{" {
				result.WriteString(strings.Repeat("    ", depth))
				result.WriteString(token + " ")
			} else {
				result.WriteString(strings.Repeat("    ", depth))
				result.WriteString(token + "\n")
			}
		}
	}

	return strings.TrimSpace(result.String())
}

// New handles GET requests for the new site form page.
func (h *SitesHandler) New(w http.ResponseWriter, r *http.Request) {
	data := SiteFormData{
		Site: nil, // nil indicates new site
	}

	pageData := templates.PageData{
		Title:     "Add Site",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-new.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create handles POST requests to create a new site.
func (h *SitesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil)
		return
	}

	// Extract form values
	domain := strings.TrimSpace(r.FormValue("domain"))
	siteType := r.FormValue("type")
	target := strings.TrimSpace(r.FormValue("target"))
	rootPath := strings.TrimSpace(r.FormValue("root_path"))
	redirectUrl := strings.TrimSpace(r.FormValue("redirect_url"))
	redirectCode := r.FormValue("redirect_code")
	enableTls := r.FormValue("enable_tls") == "on" || r.FormValue("enable_tls") == "true"

	// Store form values for re-rendering on error
	formValues := &SiteFormValues{
		Domain:       domain,
		Type:         siteType,
		Target:       target,
		RootPath:     rootPath,
		RedirectUrl:  redirectUrl,
		RedirectCode: redirectCode,
		EnableTls:    enableTls,
	}

	// Validate required fields
	if domain == "" {
		h.renderFormError(w, r, "Domain is required", formValues)
		return
	}

	// Validate domain format (basic check)
	if !isValidDomain(domain) {
		h.renderFormError(w, r, "Invalid domain format", formValues)
		return
	}

	// Validate type-specific required fields
	switch siteType {
	case "reverse_proxy":
		if target == "" {
			h.renderFormError(w, r, "Backend target is required for reverse proxy", formValues)
			return
		}
	case "static":
		if rootPath == "" {
			h.renderFormError(w, r, "Root directory is required for static file server", formValues)
			return
		}
	case "redirect":
		if redirectUrl == "" {
			h.renderFormError(w, r, "Redirect URL is required", formValues)
			return
		}
	default:
		h.renderFormError(w, r, "Invalid site type", formValues)
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil && !errors.Is(err, caddy.ErrCaddyfileNotFound) {
		h.renderFormError(w, r, "Failed to read Caddyfile: "+err.Error(), formValues)
		return
	}

	// Parse the existing config
	var caddyfile *caddy.Caddyfile
	if content != "" {
		parser := caddy.NewParser(content)
		caddyfile, err = parser.ParseAll()
		if err != nil {
			h.renderFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), formValues)
			return
		}
	} else {
		caddyfile = &caddy.Caddyfile{}
	}

	// Check if site already exists
	for _, site := range caddyfile.Sites {
		for _, addr := range site.Addresses {
			if addr == domain {
				h.renderFormError(w, r, "A site with this domain already exists", formValues)
				return
			}
		}
	}

	// Create the new site
	newSite := createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode, enableTls)

	// Add the new site to the config
	caddyfile.Sites = append(caddyfile.Sites, newSite)

	// Generate the new Caddyfile content
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Validate the new Caddyfile
	validator := caddy.NewValidator()
	result, err := validator.ValidateContent(newContent)
	if err != nil {
		h.renderFormError(w, r, "Failed to validate configuration: "+err.Error(), formValues)
		return
	}
	if !result.Valid {
		h.renderFormError(w, r, "Invalid configuration: "+result.Error(), formValues)
		return
	}

	// Write the new Caddyfile
	if err := writeCaddyfile(h.config.CaddyfilePath, newContent); err != nil {
		h.renderFormError(w, r, "Failed to save Caddyfile: "+err.Error(), formValues)
		return
	}

	// Redirect to sites list on success (using HX-Redirect for HTMX)
	w.Header().Set("HX-Redirect", "/sites")
	w.WriteHeader(http.StatusOK)
}

// renderFormError renders the form with an error message.
func (h *SitesHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SiteFormValues) {
	data := SiteFormData{
		Site:     formValues,
		Error:    errMsg,
		HasError: true,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "site-form", data); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// For regular requests, render the full page
	pageData := templates.PageData{
		Title:     "Add Site",
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-new.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// isHTMXRequest checks if the request is an HTMX request.
func isHTMXRequest(r *http.Request) bool {
	return r != nil && r.Header.Get("HX-Request") == "true"
}

// isValidDomain performs basic validation on a domain name.
func isValidDomain(domain string) bool {
	// Allow localhost
	if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
		return true
	}

	// Allow port-only addresses like :8080
	if strings.HasPrefix(domain, ":") {
		return true
	}

	// Allow URLs
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return true
	}

	// Basic domain validation - must contain at least one dot or be a single word
	// and not contain spaces or other invalid characters
	if strings.Contains(domain, " ") || strings.Contains(domain, "\t") {
		return false
	}

	return len(domain) > 0
}

// createSiteFromForm creates a Site struct from form values.
func createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode string, enableTls bool) caddy.Site {
	site := caddy.Site{
		Addresses: []string{domain},
	}

	switch siteType {
	case "reverse_proxy":
		site.Directives = append(site.Directives, caddy.Directive{
			Name: "reverse_proxy",
			Args: []string{target},
		})
	case "static":
		site.Directives = append(site.Directives, caddy.Directive{
			Name: "root",
			Args: []string{"*", rootPath},
		})
		site.Directives = append(site.Directives, caddy.Directive{
			Name: "file_server",
		})
	case "redirect":
		code := redirectCode
		if code == "" {
			code = "301"
		}
		site.Directives = append(site.Directives, caddy.Directive{
			Name: "redir",
			Args: []string{redirectUrl, code},
		})
	}

	// Handle TLS - if disabled, add explicit tls internal or http:// prefix
	if !enableTls {
		// For non-TLS sites, we could either use http:// prefix on the domain
		// or add a tls directive. Using http:// prefix is cleaner.
		site.Addresses[0] = "http://" + strings.TrimPrefix(strings.TrimPrefix(domain, "http://"), "https://")
	}

	return site
}

// writeCaddyfile writes content to the Caddyfile path.
func writeCaddyfile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
