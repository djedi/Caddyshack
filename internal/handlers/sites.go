package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// SitesData holds data displayed on the sites list page.
type SitesData struct {
	Sites          []caddy.Site
	Error          string
	HasError       bool
	SuccessMessage string
	ReloadError    string
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
	Domain         string
	OriginalDomain string // The original domain (for editing)
	Type           string // "reverse_proxy", "static", "redirect"
	Target         string // for reverse_proxy
	RootPath       string // for static
	RedirectUrl    string // for redirect
	RedirectCode   string // for redirect (301, 302, etc.)
	EnableTls      bool
}

// SiteView is a view model for a single site with helper fields.
type SiteView struct {
	caddy.Site
	PrimaryAddress string // First address for display/linking
	FormattedBlock string // Formatted raw block for display
}

// SitesHandler handles requests for the sites pages.
type SitesHandler struct {
	templates   *templates.Templates
	config      *config.Config
	adminClient *caddy.AdminClient
	store       *store.Store
}

// NewSitesHandler creates a new SitesHandler.
func NewSitesHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *SitesHandler {
	return &SitesHandler{
		templates:   tmpl,
		config:      cfg,
		adminClient: caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:       s,
	}
}

// List handles GET requests for the sites list page.
func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	data := SitesData{}

	// Check for success or reload error messages from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}
	if reloadErr := r.URL.Query().Get("reload_error"); reloadErr != "" {
		data.ReloadError = reloadErr
	}

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
					if addressMatches(addr, domain) {
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
			if addressMatches(addr, domain) {
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

	// Validate the new Caddyfile via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.adminClient.ValidateConfig(ctx, newContent); err != nil {
		h.renderFormError(w, r, "Invalid configuration: "+err.Error(), formValues)
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(newContent, "Before adding site: "+domain); err != nil {
		h.renderFormError(w, r, "Failed to save Caddyfile: "+err.Error(), formValues)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Redirect to sites list with appropriate message
	if reloadErr != nil {
		w.Header().Set("HX-Redirect", "/sites?reload_error="+url.QueryEscape(reloadErr.Error()))
	} else {
		w.Header().Set("HX-Redirect", "/sites?success="+url.QueryEscape("Site created and Caddy reloaded"))
	}
	w.WriteHeader(http.StatusOK)
}

// Edit handles GET requests for the site edit form page.
func (h *SitesHandler) Edit(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path (e.g., /sites/example.com/edit)
	path := r.URL.Path
	domain := strings.TrimPrefix(path, "/sites/")
	domain = strings.TrimSuffix(domain, "/edit")

	if domain == "" {
		http.Redirect(w, r, "/sites", http.StatusFound)
		return
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to read Caddyfile: "+err.Error(), nil, domain)
		return
	}

	// Parse sites from the Caddyfile
	parser := caddy.NewParser(content)
	sites, err := parser.ParseSites()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), nil, domain)
		return
	}

	// Find the site matching the domain
	var found *caddy.Site
	for i := range sites {
		for _, addr := range sites[i].Addresses {
			if addressMatches(addr, domain) {
				found = &sites[i]
				break
			}
		}
		if found != nil {
			break
		}
	}

	if found == nil {
		h.renderEditFormError(w, r, "Site not found: "+domain, nil, domain)
		return
	}

	// Convert Site to SiteFormValues
	formValues := siteToFormValues(found, domain)

	data := SiteFormData{
		Site: formValues,
	}

	pageData := templates.PageData{
		Title:     "Edit Site - " + domain,
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-edit.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Update handles PUT requests to update an existing site.
func (h *SitesHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path (e.g., /sites/example.com)
	path := r.URL.Path
	originalDomain := strings.TrimPrefix(path, "/sites/")
	originalDomain = strings.TrimSuffix(originalDomain, "/")

	if originalDomain == "" {
		h.renderEditFormError(w, r, "Invalid site path", nil, "")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderEditFormError(w, r, "Failed to parse form data", nil, originalDomain)
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
		Domain:         domain,
		OriginalDomain: originalDomain,
		Type:           siteType,
		Target:         target,
		RootPath:       rootPath,
		RedirectUrl:    redirectUrl,
		RedirectCode:   redirectCode,
		EnableTls:      enableTls,
	}

	// Validate required fields
	if domain == "" {
		h.renderEditFormError(w, r, "Domain is required", formValues, originalDomain)
		return
	}

	// Validate domain format (basic check)
	if !isValidDomain(domain) {
		h.renderEditFormError(w, r, "Invalid domain format", formValues, originalDomain)
		return
	}

	// Validate type-specific required fields
	switch siteType {
	case "reverse_proxy":
		if target == "" {
			h.renderEditFormError(w, r, "Backend target is required for reverse proxy", formValues, originalDomain)
			return
		}
	case "static":
		if rootPath == "" {
			h.renderEditFormError(w, r, "Root directory is required for static file server", formValues, originalDomain)
			return
		}
	case "redirect":
		if redirectUrl == "" {
			h.renderEditFormError(w, r, "Redirect URL is required", formValues, originalDomain)
			return
		}
	default:
		h.renderEditFormError(w, r, "Invalid site type", formValues, originalDomain)
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to read Caddyfile: "+err.Error(), formValues, originalDomain)
		return
	}

	// Parse the existing config
	parser := caddy.NewParser(content)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), formValues, originalDomain)
		return
	}

	// Find and update the site
	siteIndex := -1
	for i := range caddyfile.Sites {
		for _, addr := range caddyfile.Sites[i].Addresses {
			if addressMatches(addr, originalDomain) {
				siteIndex = i
				break
			}
		}
		if siteIndex != -1 {
			break
		}
	}

	if siteIndex == -1 {
		h.renderEditFormError(w, r, "Site not found: "+originalDomain, formValues, originalDomain)
		return
	}

	// Check if new domain conflicts with another site (if domain changed)
	if normalizeAddress(domain) != normalizeAddress(originalDomain) {
		for i, site := range caddyfile.Sites {
			if i == siteIndex {
				continue
			}
			for _, addr := range site.Addresses {
				if addressMatches(addr, domain) {
					h.renderEditFormError(w, r, "A site with this domain already exists", formValues, originalDomain)
					return
				}
			}
		}
	}

	// Create the updated site
	updatedSite := createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode, enableTls)

	// Replace the site in the config
	caddyfile.Sites[siteIndex] = updatedSite

	// Generate the new Caddyfile content
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Validate the new Caddyfile via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.adminClient.ValidateConfig(ctx, newContent); err != nil {
		h.renderEditFormError(w, r, "Invalid configuration: "+err.Error(), formValues, originalDomain)
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(newContent, "Before updating site: "+originalDomain); err != nil {
		h.renderEditFormError(w, r, "Failed to save Caddyfile: "+err.Error(), formValues, originalDomain)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Redirect to sites list with appropriate message
	if reloadErr != nil {
		w.Header().Set("HX-Redirect", "/sites?reload_error="+url.QueryEscape(reloadErr.Error()))
	} else {
		w.Header().Set("HX-Redirect", "/sites?success="+url.QueryEscape("Site updated and Caddy reloaded"))
	}
	w.WriteHeader(http.StatusOK)
}

// siteToFormValues converts a Site struct to SiteFormValues for form pre-population.
func siteToFormValues(site *caddy.Site, originalDomain string) *SiteFormValues {
	formValues := &SiteFormValues{
		OriginalDomain: originalDomain,
		EnableTls:      true,
	}

	// Get the domain (strip http:// prefix if present)
	if len(site.Addresses) > 0 {
		domain := site.Addresses[0]
		if strings.HasPrefix(domain, "http://") {
			formValues.Domain = strings.TrimPrefix(domain, "http://")
			formValues.EnableTls = false
		} else if strings.HasPrefix(domain, "https://") {
			formValues.Domain = strings.TrimPrefix(domain, "https://")
			formValues.EnableTls = true
		} else {
			formValues.Domain = domain
		}
	}

	// Determine site type and extract values from directives
	for _, directive := range site.Directives {
		switch directive.Name {
		case "reverse_proxy":
			formValues.Type = "reverse_proxy"
			if len(directive.Args) > 0 {
				formValues.Target = directive.Args[0]
			}
		case "root":
			// Root is typically paired with file_server
			if len(directive.Args) > 1 {
				formValues.RootPath = directive.Args[1]
			} else if len(directive.Args) > 0 {
				formValues.RootPath = directive.Args[0]
			}
		case "file_server":
			formValues.Type = "static"
		case "redir":
			formValues.Type = "redirect"
			if len(directive.Args) > 0 {
				formValues.RedirectUrl = directive.Args[0]
			}
			if len(directive.Args) > 1 {
				formValues.RedirectCode = directive.Args[1]
			} else {
				formValues.RedirectCode = "301"
			}
		}
	}

	// Default type if not determined
	if formValues.Type == "" {
		formValues.Type = "reverse_proxy"
	}

	// Default root path for static sites
	if formValues.Type == "static" && formValues.RootPath == "" {
		formValues.RootPath = "/var/www/html"
	}

	return formValues
}

// renderEditFormError renders the edit form with an error message.
func (h *SitesHandler) renderEditFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SiteFormValues, originalDomain string) {
	if formValues == nil {
		formValues = &SiteFormValues{
			OriginalDomain: originalDomain,
			Domain:         originalDomain,
		}
	}
	if formValues.OriginalDomain == "" {
		formValues.OriginalDomain = originalDomain
	}

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
		Title:     "Edit Site - " + originalDomain,
		ActiveNav: "sites",
		Data:      data,
	}

	if err := h.templates.Render(w, "site-edit.html", pageData); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
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

// normalizeAddress extracts the domain from an address for comparison.
// It handles both plain domains (example.com) and URL-style addresses (http://example.com).
func normalizeAddress(addr string) string {
	// Strip http:// or https:// prefix
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	// Also handle mangled URLs where // becomes / (from URL path parsing)
	addr = strings.TrimPrefix(addr, "http:/")
	addr = strings.TrimPrefix(addr, "https:/")
	return addr
}

// addressMatches checks if a lookup domain matches a site address.
// It normalizes both values to handle http:// prefixes.
func addressMatches(siteAddr, lookupDomain string) bool {
	return normalizeAddress(siteAddr) == normalizeAddress(lookupDomain)
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

// saveAndWriteCaddyfile saves the current Caddyfile to history and writes the new content.
// The comment describes what change is being made.
func (h *SitesHandler) saveAndWriteCaddyfile(newContent, comment string) error {
	// Read current content to save to history
	reader := caddy.NewReader(h.config.CaddyfilePath)
	currentContent, err := reader.Read()
	if err != nil && !errors.Is(err, caddy.ErrCaddyfileNotFound) {
		return err
	}

	// Only save history if there's existing content and it's different
	if currentContent != "" && currentContent != newContent {
		if err := h.store.SaveConfigHistory(currentContent, comment); err != nil {
			log.Printf("Warning: failed to save config history: %v", err)
			// Continue anyway - we don't want to fail the save just because history failed
		}

		// Prune old history entries
		if err := h.store.PruneConfigHistory(h.config.HistoryLimit); err != nil {
			log.Printf("Warning: failed to prune config history: %v", err)
		}
	}

	// Write the new content
	return writeCaddyfile(h.config.CaddyfilePath, newContent)
}

// Delete handles DELETE requests to remove a site.
func (h *SitesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Extract domain from URL path (e.g., /sites/example.com)
	path := r.URL.Path
	domain := strings.TrimPrefix(path, "/sites/")
	domain = strings.TrimSuffix(domain, "/")

	if domain == "" {
		http.Error(w, "Invalid site path", http.StatusBadRequest)
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		http.Error(w, "Failed to read Caddyfile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the existing config
	parser := caddy.NewParser(content)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		http.Error(w, "Failed to parse Caddyfile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Find and remove the site
	siteIndex := -1
	for i := range caddyfile.Sites {
		for _, addr := range caddyfile.Sites[i].Addresses {
			if addressMatches(addr, domain) {
				siteIndex = i
				break
			}
		}
		if siteIndex != -1 {
			break
		}
	}

	if siteIndex == -1 {
		http.Error(w, "Site not found: "+domain, http.StatusNotFound)
		return
	}

	// Remove the site from the slice
	caddyfile.Sites = append(caddyfile.Sites[:siteIndex], caddyfile.Sites[siteIndex+1:]...)

	// Generate the new Caddyfile content
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Validate the new Caddyfile via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.adminClient.ValidateConfig(ctx, newContent); err != nil {
		http.Error(w, "Invalid configuration: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(newContent, "Before deleting site: "+domain); err != nil {
		http.Error(w, "Failed to save Caddyfile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// For HTMX requests, redirect to refresh the site list
	if isHTMXRequest(r) {
		if reloadErr != nil {
			w.Header().Set("HX-Redirect", "/sites?reload_error="+url.QueryEscape(reloadErr.Error()))
		} else {
			w.Header().Set("HX-Redirect", "/sites?success="+url.QueryEscape("Site deleted and Caddy reloaded"))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect to sites list
	if reloadErr != nil {
		http.Redirect(w, r, "/sites?reload_error="+url.QueryEscape(reloadErr.Error()), http.StatusFound)
	} else {
		http.Redirect(w, r, "/sites?success="+url.QueryEscape("Site deleted and Caddy reloaded"), http.StatusFound)
	}
}

// reloadCaddy reloads the Caddy configuration with the given content.
// Returns an error if the reload fails, but the Caddyfile is already saved.
func (h *SitesHandler) reloadCaddy(content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.adminClient.Reload(ctx, content)
}
