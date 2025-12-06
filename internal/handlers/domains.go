package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// DomainView represents a domain for display in templates.
type DomainView struct {
	store.Domain
	ExpiryStatus     string // "valid", "expiring", "expired", or "unknown"
	ExpiryStatusText string // Human-readable status
	DaysUntilExpiry  int    // Days until expiry (negative if expired)
	FormattedExpiry  string // Formatted expiry date
}

// DomainsData holds data displayed on the domains list page.
type DomainsData struct {
	Domains          []DomainView
	Error            string
	HasError         bool
	SuccessMessage   string
	ExpiringCount    int
	ExpiredCount     int
	TotalCount       int
}

// DomainFormData holds data for the domain add/edit form.
type DomainFormData struct {
	Domain   *DomainFormValues
	Error    string
	HasError bool
	IsEdit   bool
}

// DomainFormValues represents the form field values for creating/editing a domain.
type DomainFormValues struct {
	ID         int64
	Name       string
	Registrar  string
	ExpiryDate string // YYYY-MM-DD format for input
	Notes      string
}

// DomainsHandler handles requests for the domains pages.
type DomainsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewDomainsHandler creates a new DomainsHandler.
func NewDomainsHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *DomainsHandler {
	return &DomainsHandler{
		templates:    tmpl,
		config:       cfg,
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the domains list page.
func (h *DomainsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := DomainsData{}

	// Check for success message from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		data.SuccessMessage = successMsg
	}

	// Sync auto-detected domains from Caddyfile
	if err := h.syncAutoDetectedDomains(); err != nil {
		log.Printf("Warning: failed to sync auto-detected domains: %v", err)
	}

	// Get all domains
	domains, err := h.store.ListDomains()
	if err != nil {
		data.Error = "Failed to list domains: " + err.Error()
		data.HasError = true
	} else {
		data.Domains = make([]DomainView, len(domains))
		for i, d := range domains {
			data.Domains[i] = toDomainView(d)
			if data.Domains[i].ExpiryStatus == "expiring" {
				data.ExpiringCount++
			} else if data.Domains[i].ExpiryStatus == "expired" {
				data.ExpiredCount++
			}
		}
		data.TotalCount = len(domains)
	}

	// Check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "domains-list.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	pageData := templates.PageData{
		Title:     "Domains",
		ActiveNav: "domains",
		Data:      data,
	}

	if err := h.templates.Render(w, "domains.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// New handles GET requests for the new domain form page.
func (h *DomainsHandler) New(w http.ResponseWriter, r *http.Request) {
	data := DomainFormData{
		Domain: &DomainFormValues{},
		IsEdit: false,
	}

	pageData := templates.PageData{
		Title:     "Add Domain",
		ActiveNav: "domains",
		Data:      data,
	}

	if err := h.templates.Render(w, "domain-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Create handles POST requests to create a new domain.
func (h *DomainsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil, false)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	registrar := strings.TrimSpace(r.FormValue("registrar"))
	expiryDateStr := strings.TrimSpace(r.FormValue("expiry_date"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	formValues := &DomainFormValues{
		Name:       name,
		Registrar:  registrar,
		ExpiryDate: expiryDateStr,
		Notes:      notes,
	}

	// Validate required fields
	if name == "" {
		h.renderFormError(w, r, "Domain name is required", formValues, false)
		return
	}

	// Check if domain already exists
	existing, err := h.store.GetDomainByName(name)
	if err != nil {
		h.renderFormError(w, r, "Failed to check existing domain: "+err.Error(), formValues, false)
		return
	}
	if existing != nil {
		h.renderFormError(w, r, "A domain with this name already exists", formValues, false)
		return
	}

	// Parse expiry date if provided
	var expiryDate *time.Time
	if expiryDateStr != "" {
		parsed, err := time.Parse("2006-01-02", expiryDateStr)
		if err != nil {
			h.renderFormError(w, r, "Invalid expiry date format", formValues, false)
			return
		}
		expiryDate = &parsed
	}

	// Create the domain
	domain := &store.Domain{
		Name:       name,
		Registrar:  registrar,
		ExpiryDate: expiryDate,
		Notes:      notes,
		AutoAdded:  false,
	}

	if err := h.store.CreateDomain(domain); err != nil {
		h.renderFormError(w, r, "Failed to create domain: "+err.Error(), formValues, false)
		return
	}

	// Redirect to domains list with success message
	w.Header().Set("HX-Redirect", "/domains?success="+url.QueryEscape("Domain added successfully"))
	w.WriteHeader(http.StatusOK)
}

// Edit handles GET requests for the domain edit form page.
func (h *DomainsHandler) Edit(w http.ResponseWriter, r *http.Request) {
	// Extract domain ID from URL path (e.g., /domains/123/edit)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/domains/")
	path = strings.TrimSuffix(path, "/edit")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid domain ID")
		return
	}

	domain, err := h.store.GetDomain(id)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}
	if domain == nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	formValues := &DomainFormValues{
		ID:        domain.ID,
		Name:      domain.Name,
		Registrar: domain.Registrar,
		Notes:     domain.Notes,
	}
	if domain.ExpiryDate != nil {
		formValues.ExpiryDate = domain.ExpiryDate.Format("2006-01-02")
	}

	data := DomainFormData{
		Domain: formValues,
		IsEdit: true,
	}

	pageData := templates.PageData{
		Title:     "Edit Domain - " + domain.Name,
		ActiveNav: "domains",
		Data:      data,
	}

	if err := h.templates.Render(w, "domain-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Update handles PUT requests to update an existing domain.
func (h *DomainsHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Extract domain ID from URL path (e.g., /domains/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/domains/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid domain ID")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil, true)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	registrar := strings.TrimSpace(r.FormValue("registrar"))
	expiryDateStr := strings.TrimSpace(r.FormValue("expiry_date"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	formValues := &DomainFormValues{
		ID:         id,
		Name:       name,
		Registrar:  registrar,
		ExpiryDate: expiryDateStr,
		Notes:      notes,
	}

	// Validate required fields
	if name == "" {
		h.renderFormError(w, r, "Domain name is required", formValues, true)
		return
	}

	// Get existing domain
	domain, err := h.store.GetDomain(id)
	if err != nil {
		h.renderFormError(w, r, "Failed to get domain: "+err.Error(), formValues, true)
		return
	}
	if domain == nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Check if new name conflicts with another domain
	if domain.Name != name {
		existing, err := h.store.GetDomainByName(name)
		if err != nil {
			h.renderFormError(w, r, "Failed to check existing domain: "+err.Error(), formValues, true)
			return
		}
		if existing != nil {
			h.renderFormError(w, r, "A domain with this name already exists", formValues, true)
			return
		}
	}

	// Parse expiry date if provided
	var expiryDate *time.Time
	if expiryDateStr != "" {
		parsed, err := time.Parse("2006-01-02", expiryDateStr)
		if err != nil {
			h.renderFormError(w, r, "Invalid expiry date format", formValues, true)
			return
		}
		expiryDate = &parsed
	}

	// Update the domain
	domain.Name = name
	domain.Registrar = registrar
	domain.ExpiryDate = expiryDate
	domain.Notes = notes
	// Once manually edited, mark as not auto-added
	domain.AutoAdded = false

	if err := h.store.UpdateDomain(domain); err != nil {
		h.renderFormError(w, r, "Failed to update domain: "+err.Error(), formValues, true)
		return
	}

	// Redirect to domains list with success message
	w.Header().Set("HX-Redirect", "/domains?success="+url.QueryEscape("Domain updated successfully"))
	w.WriteHeader(http.StatusOK)
}

// Delete handles DELETE requests to remove a domain.
func (h *DomainsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Extract domain ID from URL path (e.g., /domains/123)
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/domains/")
	path = strings.TrimSuffix(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid domain ID")
		return
	}

	if err := h.store.DeleteDomain(id); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// For HTMX requests, redirect to refresh the list
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/domains?success="+url.QueryEscape("Domain deleted successfully"))
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect to domains list
	http.Redirect(w, r, "/domains?success="+url.QueryEscape("Domain deleted successfully"), http.StatusFound)
}

// Widget handles GET requests for the domains dashboard widget.
func (h *DomainsHandler) Widget(w http.ResponseWriter, r *http.Request) {
	type WidgetData struct {
		TotalCount    int
		ExpiringCount int
		ExpiredCount  int
		HasExpiring   bool
		HasExpired    bool
	}

	data := WidgetData{}

	// Get counts
	domains, err := h.store.ListDomains()
	if err == nil {
		data.TotalCount = len(domains)
		for _, d := range domains {
			view := toDomainView(d)
			if view.ExpiryStatus == "expiring" {
				data.ExpiringCount++
			} else if view.ExpiryStatus == "expired" {
				data.ExpiredCount++
			}
		}
		data.HasExpiring = data.ExpiringCount > 0
		data.HasExpired = data.ExpiredCount > 0
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "domains-widget.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// syncAutoDetectedDomains extracts domains from the Caddyfile and syncs them to the database.
func (h *DomainsHandler) syncAutoDetectedDomains() error {
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if errors.Is(err, caddy.ErrCaddyfileNotFound) {
			return nil // No Caddyfile, nothing to sync
		}
		return err
	}

	parser := caddy.NewParser(content)
	sites, err := parser.ParseSites()
	if err != nil {
		return err
	}

	// Extract unique domain names from sites
	domainMap := make(map[string]bool)
	for _, site := range sites {
		for _, addr := range site.Addresses {
			domain := extractDomainFromAddress(addr)
			if domain != "" && isValidDomainName(domain) {
				domainMap[domain] = true
			}
		}
	}

	var domainNames []string
	for name := range domainMap {
		domainNames = append(domainNames, name)
	}

	return h.store.SyncAutoAddedDomains(domainNames)
}

// extractDomainFromAddress extracts the domain name from a Caddy address.
// It handles formats like: example.com, http://example.com, example.com:443, etc.
func extractDomainFromAddress(addr string) string {
	// Remove protocol prefix
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	// Remove port suffix
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		// Check if it's a port (all digits after colon)
		portPart := addr[idx+1:]
		if _, err := strconv.Atoi(portPart); err == nil {
			addr = addr[:idx]
		}
	}

	// Remove path
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	return strings.TrimSpace(addr)
}

// isValidDomainName checks if a string is a valid domain name for tracking.
// It excludes localhost, IP addresses, wildcards, and port-only addresses.
func isValidDomainName(domain string) bool {
	// Exclude empty strings
	if domain == "" {
		return false
	}

	// Exclude localhost
	if domain == "localhost" {
		return false
	}

	// Exclude wildcards
	if strings.HasPrefix(domain, "*") {
		return false
	}

	// Exclude port-only addresses
	if strings.HasPrefix(domain, ":") {
		return false
	}

	// Exclude IP addresses (basic check)
	parts := strings.Split(domain, ".")
	if len(parts) == 4 {
		isIP := true
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				isIP = false
				break
			}
		}
		if isIP {
			return false
		}
	}

	// Must contain at least one dot (e.g., example.com)
	if !strings.Contains(domain, ".") {
		return false
	}

	return true
}

// toDomainView converts a Domain to a DomainView with status information.
func toDomainView(d store.Domain) DomainView {
	view := DomainView{
		Domain: d,
	}

	if d.ExpiryDate == nil {
		view.ExpiryStatus = "unknown"
		view.ExpiryStatusText = "No expiry date set"
		return view
	}

	now := time.Now()
	days := int(d.ExpiryDate.Sub(now).Hours() / 24)
	view.DaysUntilExpiry = days
	view.FormattedExpiry = d.ExpiryDate.Format("January 2, 2006")

	if days < 0 {
		view.ExpiryStatus = "expired"
		view.ExpiryStatusText = "Expired"
	} else if days <= 60 {
		view.ExpiryStatus = "expiring"
		if days == 0 {
			view.ExpiryStatusText = "Expires today"
		} else if days == 1 {
			view.ExpiryStatusText = "Expires tomorrow"
		} else {
			view.ExpiryStatusText = "Expires in " + strconv.Itoa(days) + " days"
		}
	} else {
		view.ExpiryStatus = "valid"
		view.ExpiryStatusText = "Valid"
	}

	return view
}

// renderFormError renders the form with an error message.
func (h *DomainsHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *DomainFormValues, isEdit bool) {
	log.Printf("Domain form error: %s", errMsg)

	if formValues == nil {
		formValues = &DomainFormValues{}
	}

	data := DomainFormData{
		Domain:   formValues,
		Error:    errMsg,
		HasError: true,
		IsEdit:   isEdit,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "domain-form.html", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// For regular requests, render the full page
	templateName := "domain-new.html"
	title := "Add Domain"
	if isEdit {
		templateName = "domain-edit.html"
		title = "Edit Domain"
	}

	pageData := templates.PageData{
		Title:     title,
		ActiveNav: "domains",
		Data:      data,
	}

	if err := h.templates.Render(w, templateName, pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}
