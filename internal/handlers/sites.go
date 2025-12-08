package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// SiteCardData holds a site and its associated container status for display on site cards.
type SiteCardData struct {
	Site            caddy.Site
	Container       *ContainerStatus
	DockerEnabled   bool
	DockerAvailable bool
}

// SitesData holds data displayed on the sites list page.
type SitesData struct {
	Sites          []SiteCardData
	Error          string
	HasError       bool
	SuccessMessage string
	ReloadError    string
}

// ContainerStatus holds container information for display in site views.
type ContainerStatus struct {
	Name        string
	State       string
	StateColor  string
	HealthState string
	Available   bool
}

// SiteDetailData holds data displayed on the site detail page.
type SiteDetailData struct {
	Site            SiteView
	Error           string
	HasError        bool
	Container       *ContainerStatus
	ProxyTarget     string
	DockerEnabled   bool
	DockerAvailable bool
}

// SiteFormData holds data for the site add/edit form.
type SiteFormData struct {
	Site              *SiteFormValues // nil for new site, populated for edit
	Error             string
	HasError          bool
	AvailableSnippets []SnippetOption // Available snippets for selection
}

// SnippetOption represents a snippet available for import.
type SnippetOption struct {
	Name        string
	Description string // First directive or preview of snippet content
	Selected    bool   // Whether this snippet is currently imported
}

// SiteFormValues represents the form field values for creating/editing a site.
type SiteFormValues struct {
	Domain           string
	OriginalDomain   string   // The original domain (for editing)
	Type             string   // "reverse_proxy", "static", "redirect"
	Target           string   // for reverse_proxy
	RootPath         string   // for static
	RedirectUrl      string   // for redirect
	RedirectCode     string   // for redirect (301, 302, etc.)
	EnableTls        bool
	Imports          []string // Imported snippet names
	CustomDirectives string   // Raw custom directives (advanced mode)
}

// SiteView is a view model for a single site with helper fields.
type SiteView struct {
	caddy.Site
	PrimaryAddress string // First address for display/linking
	FormattedBlock string // Formatted raw block for display
}

// SitesHandler handles requests for the sites pages.
type SitesHandler struct {
	templates     *templates.Templates
	config        *config.Config
	adminClient   *caddy.AdminClient
	store         *store.Store
	errorHandler  *ErrorHandler
	dockerClient  *docker.Client
	dockerEnabled bool
	auditLogger   *AuditLogger
}

// NewSitesHandler creates a new SitesHandler.
func NewSitesHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *SitesHandler {
	var dockerClient *docker.Client
	if cfg.DockerEnabled {
		dockerClient = docker.NewClient(cfg.DockerSocket)
	}

	return &SitesHandler{
		templates:     tmpl,
		config:        cfg,
		adminClient:   caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:         s,
		errorHandler:  NewErrorHandler(tmpl),
		dockerClient:  dockerClient,
		dockerEnabled: cfg.DockerEnabled,
		auditLogger:   NewAuditLogger(s),
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
			// Build SiteCardData with container status for each site
			data.Sites = h.buildSiteCardData(r.Context(), sites)
		}
	}

	pageData := WithPermissions(r, "Sites", "sites", data)

	if err := h.templates.Render(w, "sites.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// buildSiteCardData builds site card data with container status for each site.
func (h *SitesHandler) buildSiteCardData(ctx context.Context, sites []caddy.Site) []SiteCardData {
	result := make([]SiteCardData, len(sites))

	// Check if Docker is available (do this once for all sites)
	dockerAvailable := false
	if h.dockerEnabled && h.dockerClient != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		dockerAvailable = h.dockerClient.IsAvailable(checkCtx)
		cancel()
	}

	for i, site := range sites {
		result[i] = SiteCardData{
			Site:            site,
			DockerEnabled:   h.dockerEnabled,
			DockerAvailable: dockerAvailable,
		}

		// Only try to find container status if Docker is enabled and available
		if dockerAvailable {
			proxyTarget := extractProxyTarget(site.Directives)
			if proxyTarget != "" {
				target := docker.ParseProxyTarget(proxyTarget)
				if target != nil {
					findCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					container, err := h.dockerClient.FindContainerForTarget(findCtx, target)
					cancel()
					if err == nil && container != nil {
						result[i].Container = &ContainerStatus{
							Name:        container.Name,
							State:       container.State,
							StateColor:  getContainerStateColor(container.State, container.HealthState),
							HealthState: container.HealthState,
							Available:   true,
						}
					}
				}
			}
		}
	}

	return result
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

				// Try to find container status for reverse proxy targets
				data.DockerEnabled = h.dockerEnabled
				if h.dockerEnabled && h.dockerClient != nil {
					ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
					defer cancel()

					data.DockerAvailable = h.dockerClient.IsAvailable(ctx)

					if data.DockerAvailable {
						// Extract proxy target from directives
						proxyTarget := extractProxyTarget(found.Directives)
						if proxyTarget != "" {
							data.ProxyTarget = proxyTarget
							target := docker.ParseProxyTarget(proxyTarget)
							if target != nil {
								container, err := h.dockerClient.FindContainerForTarget(ctx, target)
								if err == nil && container != nil {
									data.Container = &ContainerStatus{
										Name:        container.Name,
										State:       container.State,
										StateColor:  getContainerStateColor(container.State, container.HealthState),
										HealthState: container.HealthState,
										Available:   true,
									}
								}
							}
						}
					}
				}
			}
		}
	}

	pageData := WithPermissions(r, domain+" - Site Details", "sites", data)

	if err := h.templates.Render(w, "site-detail.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// extractProxyTarget extracts the first reverse_proxy target from directives.
func extractProxyTarget(directives []caddy.Directive) string {
	for _, d := range directives {
		if d.Name == "reverse_proxy" && len(d.Args) > 0 {
			return d.Args[0]
		}
		// Check nested directives (e.g., in handle blocks)
		if len(d.Block) > 0 {
			if target := extractProxyTarget(d.Block); target != "" {
				return target
			}
		}
	}
	return ""
}

// getContainerStateColor returns a Tailwind color class for the container state.
func getContainerStateColor(state, healthState string) string {
	switch state {
	case "running":
		if healthState == "unhealthy" {
			return "yellow"
		}
		return "green"
	case "paused", "restarting":
		return "yellow"
	default:
		return "red"
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
	// Load available snippets
	availableSnippets := h.loadAvailableSnippets(nil)

	data := SiteFormData{
		Site:              nil, // nil indicates new site
		AvailableSnippets: availableSnippets,
	}

	pageData := WithPermissions(r, "Add Site", "sites", data)

	if err := h.templates.Render(w, "site-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// loadAvailableSnippets reads the Caddyfile and returns snippet options.
// If selectedImports is provided, those snippets will be marked as selected.
func (h *SitesHandler) loadAvailableSnippets(selectedImports []string) []SnippetOption {
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		return nil
	}

	parser := caddy.NewParser(content)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		return nil
	}

	// Create a set of selected imports for quick lookup
	selected := make(map[string]bool)
	for _, imp := range selectedImports {
		selected[imp] = true
	}

	var options []SnippetOption
	for _, s := range snippets {
		// Create a preview/description from the first directive
		description := ""
		if len(s.Directives) > 0 {
			d := s.Directives[0]
			description = d.Name
			if len(d.Args) > 0 {
				description += " " + strings.Join(d.Args, " ")
			}
			// Truncate if too long
			if len(description) > 50 {
				description = description[:47] + "..."
			}
		}

		options = append(options, SnippetOption{
			Name:        s.Name,
			Description: description,
			Selected:    selected[s.Name],
		})
	}

	return options
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
	customDirectives := r.FormValue("custom_directives")

	// Extract selected imports (multiple values with same key)
	imports := r.Form["imports"]

	// Store form values for re-rendering on error
	formValues := &SiteFormValues{
		Domain:           domain,
		Type:             siteType,
		Target:           target,
		RootPath:         rootPath,
		RedirectUrl:      redirectUrl,
		RedirectCode:     redirectCode,
		EnableTls:        enableTls,
		Imports:          imports,
		CustomDirectives: customDirectives,
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
	newSite := createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode, enableTls, imports, customDirectives)

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

	// Log audit event
	h.auditLogger.Log(r, store.ActionSiteCreate, store.ResourceSite, domain, "Created site with type: "+siteType)

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

	// Load available snippets (with current imports marked as selected)
	availableSnippets := h.loadAvailableSnippets(formValues.Imports)

	data := SiteFormData{
		Site:              formValues,
		AvailableSnippets: availableSnippets,
	}

	pageData := WithPermissions(r, "Edit Site - "+domain, "sites", data)

	if err := h.templates.Render(w, "site-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
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
	customDirectives := r.FormValue("custom_directives")

	// Extract selected imports (multiple values with same key)
	imports := r.Form["imports"]

	// Store form values for re-rendering on error
	formValues := &SiteFormValues{
		Domain:           domain,
		OriginalDomain:   originalDomain,
		Type:             siteType,
		Target:           target,
		RootPath:         rootPath,
		RedirectUrl:      redirectUrl,
		RedirectCode:     redirectCode,
		EnableTls:        enableTls,
		Imports:          imports,
		CustomDirectives: customDirectives,
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
	updatedSite := createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode, enableTls, imports, customDirectives)

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

	// Log audit event
	details := "Updated site"
	if domain != originalDomain {
		details = "Renamed site from " + originalDomain + " to " + domain
	}
	h.auditLogger.Log(r, store.ActionSiteUpdate, store.ResourceSite, domain, details)

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
		Imports:        site.Imports,
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

	// Track which directives are "standard" (handled by the form)
	var customDirectives []caddy.Directive

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
		case "import":
			// Already handled via site.Imports, skip
		default:
			// This is a custom directive not handled by the form
			customDirectives = append(customDirectives, directive)
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

	// Format custom directives for the textarea
	if len(customDirectives) > 0 {
		formValues.CustomDirectives = formatDirectivesForTextarea(customDirectives)
	}

	return formValues
}

// formatDirectivesForTextarea formats directives as human-readable text for editing.
func formatDirectivesForTextarea(directives []caddy.Directive) string {
	var sb strings.Builder
	for i, d := range directives {
		formatDirectiveForTextarea(&sb, d, 0)
		if i < len(directives)-1 {
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// formatDirectiveForTextarea formats a single directive with proper indentation.
func formatDirectiveForTextarea(sb *strings.Builder, d caddy.Directive, depth int) {
	indent := strings.Repeat("    ", depth)
	sb.WriteString(indent)
	sb.WriteString(d.Name)
	for _, arg := range d.Args {
		sb.WriteString(" ")
		// Quote args with spaces
		if strings.Contains(arg, " ") && !strings.HasPrefix(arg, "\"") {
			sb.WriteString("\"")
			sb.WriteString(arg)
			sb.WriteString("\"")
		} else {
			sb.WriteString(arg)
		}
	}
	if len(d.Block) > 0 {
		sb.WriteString(" {\n")
		for _, nested := range d.Block {
			formatDirectiveForTextarea(sb, nested, depth+1)
			sb.WriteString("\n")
		}
		sb.WriteString(indent)
		sb.WriteString("}")
	}
}

// renderEditFormError renders the edit form with an error message.
func (h *SitesHandler) renderEditFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SiteFormValues, originalDomain string) {
	log.Printf("Site edit form error: %s [domain: %s]", errMsg, originalDomain)

	if formValues == nil {
		formValues = &SiteFormValues{
			OriginalDomain: originalDomain,
			Domain:         originalDomain,
		}
	}
	if formValues.OriginalDomain == "" {
		formValues.OriginalDomain = originalDomain
	}

	// Load available snippets (with current imports marked as selected)
	availableSnippets := h.loadAvailableSnippets(formValues.Imports)

	data := SiteFormData{
		Site:              formValues,
		Error:             errMsg,
		HasError:          true,
		AvailableSnippets: availableSnippets,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "site-form", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
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
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderFormError renders the form with an error message.
func (h *SitesHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SiteFormValues) {
	log.Printf("Site form error: %s", errMsg)

	// Load available snippets (with current imports marked as selected)
	var selectedImports []string
	if formValues != nil {
		selectedImports = formValues.Imports
	}
	availableSnippets := h.loadAvailableSnippets(selectedImports)

	data := SiteFormData{
		Site:              formValues,
		Error:             errMsg,
		HasError:          true,
		AvailableSnippets: availableSnippets,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "site-form", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
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
		h.errorHandler.InternalServerError(w, r, err)
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
func createSiteFromForm(domain, siteType, target, rootPath, redirectUrl, redirectCode string, enableTls bool, imports []string, customDirectives string) caddy.Site {
	site := caddy.Site{
		Addresses: []string{domain},
		Imports:   imports,
	}

	// Add import directives first
	for _, imp := range imports {
		site.Directives = append(site.Directives, caddy.Directive{
			Name: "import",
			Args: []string{imp},
		})
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

	// Parse and add custom directives
	if customDirectives != "" {
		customDirs := parseCustomDirectives(customDirectives)
		site.Directives = append(site.Directives, customDirs...)
	}

	// Handle TLS - if disabled, add explicit tls internal or http:// prefix
	if !enableTls {
		// For non-TLS sites, we could either use http:// prefix on the domain
		// or add a tls directive. Using http:// prefix is cleaner.
		site.Addresses[0] = "http://" + strings.TrimPrefix(strings.TrimPrefix(domain, "http://"), "https://")
	}

	return site
}

// parseCustomDirectives parses raw directive text into Directive structs.
func parseCustomDirectives(raw string) []caddy.Directive {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	// Create a minimal site block to leverage the existing parser
	siteBlock := "temp.local {\n" + raw + "\n}"
	parser := caddy.NewParser(siteBlock)
	sites, err := parser.ParseSites()
	if err != nil || len(sites) == 0 {
		return nil
	}

	// Filter out import directives (those should be handled separately)
	var directives []caddy.Directive
	for _, d := range sites[0].Directives {
		if d.Name != "import" {
			directives = append(directives, d)
		}
	}

	return directives
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
		h.errorHandler.BadRequest(w, r, "Invalid site path")
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Parse the existing config
	parser := caddy.NewParser(content)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
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
		h.errorHandler.NotFound(w, r)
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
		h.errorHandler.BadRequest(w, r, "Invalid configuration: "+err.Error())
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(newContent, "Before deleting site: "+domain); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Log audit event
	h.auditLogger.Log(r, store.ActionSiteDelete, store.ResourceSite, domain, "Deleted site")

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

// ValidateDirectivesResponse is the JSON response for directive validation.
type ValidateDirectivesResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// ValidateDirectives handles POST requests to validate custom directives.
// It creates a temporary Caddyfile with the directives and validates via Caddy Admin API.
func (h *SitesHandler) ValidateDirectives(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, ValidateDirectivesResponse{
			Valid: false,
			Error: "Failed to parse form data",
		})
		return
	}

	domain := r.FormValue("domain")
	if domain == "" {
		domain = "example.com"
	}
	directives := r.FormValue("directives")

	// If empty directives, it's valid
	if strings.TrimSpace(directives) == "" {
		writeJSONResponse(w, http.StatusOK, ValidateDirectivesResponse{Valid: true})
		return
	}

	// Read the existing Caddyfile to get global options and snippets
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, _ := reader.Read() // Ignore error - we'll create minimal config if needed

	var caddyfile *caddy.Caddyfile
	if content != "" {
		parser := caddy.NewParser(content)
		caddyfile, _ = parser.ParseAll()
	}
	if caddyfile == nil {
		caddyfile = &caddy.Caddyfile{}
	}

	// Create a test site with the custom directives
	testSite := caddy.Site{
		Addresses: []string{domain},
	}

	// Parse the custom directives
	customDirs := parseCustomDirectives(directives)
	testSite.Directives = customDirs

	// Add test site to a copy of the caddyfile
	testCaddyfile := &caddy.Caddyfile{
		GlobalOptions: caddyfile.GlobalOptions,
		Snippets:      caddyfile.Snippets,
		Sites:         []caddy.Site{testSite},
	}

	// Generate the test Caddyfile content
	writer := caddy.NewWriter()
	testContent := writer.WriteCaddyfile(testCaddyfile)

	// Validate via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.adminClient.ValidateConfig(ctx, testContent); err != nil {
		writeJSONResponse(w, http.StatusOK, ValidateDirectivesResponse{
			Valid: false,
			Error: err.Error(),
		})
		return
	}

	writeJSONResponse(w, http.StatusOK, ValidateDirectivesResponse{Valid: true})
}

// writeJSONResponse writes a JSON response with the given status code.
func writeJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}
