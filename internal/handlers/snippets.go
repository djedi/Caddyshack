package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// SnippetsData holds data displayed on the snippets list page.
type SnippetsData struct {
	Snippets       []SnippetView
	Error          string
	HasError       bool
	SuccessMessage string
	ReloadError    string
}

// SnippetView is a view model for a single snippet with helper fields.
type SnippetView struct {
	caddy.Snippet
	Preview     string   // First few lines of content for display
	UsageCount  int      // Number of sites using this snippet
	UsedBySites []string // Names of sites using this snippet
}

// SnippetFormData holds data for the snippet add/edit form.
type SnippetFormData struct {
	Snippet  *SnippetFormValues // nil for new snippet, populated for edit
	Error    string
	HasError bool
}

// SnippetFormValues represents the form field values for creating/editing a snippet.
type SnippetFormValues struct {
	Name         string
	OriginalName string // The original name (for editing)
	Content      string // Raw content of the snippet
}

// SnippetsHandler handles requests for the snippets pages.
type SnippetsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	adminClient  *caddy.AdminClient
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewSnippetsHandler creates a new SnippetsHandler.
func NewSnippetsHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *SnippetsHandler {
	return &SnippetsHandler{
		templates:    tmpl,
		config:       cfg,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the snippets list page.
func (h *SnippetsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := SnippetsData{}

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
		// Parse snippets and sites from the Caddyfile
		parser := caddy.NewParser(content)
		snippets, err := parser.ParseSnippets()
		if err != nil {
			data.Error = "Failed to parse Caddyfile: " + err.Error()
			data.HasError = true
		} else {
			// Get sites to determine snippet usage
			sites, _ := parser.ParseSites()

			// Build snippet views with usage info
			for _, snippet := range snippets {
				view := SnippetView{
					Snippet: snippet,
					Preview: getSnippetPreview(snippet),
				}

				// Count usage across sites
				for _, site := range sites {
					for _, imp := range site.Imports {
						if imp == snippet.Name {
							view.UsageCount++
							if len(site.Addresses) > 0 {
								view.UsedBySites = append(view.UsedBySites, site.Addresses[0])
							}
							break
						}
					}
				}

				data.Snippets = append(data.Snippets, view)
			}
		}
	}

	pageData := templates.PageData{
		Title:     "Snippets",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippets.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// getSnippetPreview returns the first few lines of a snippet's content for display.
func getSnippetPreview(snippet caddy.Snippet) string {
	if len(snippet.Directives) == 0 {
		return "(empty)"
	}

	var lines []string
	maxLines := 3

	for i, directive := range snippet.Directives {
		if i >= maxLines {
			break
		}

		line := directive.Name
		if len(directive.Args) > 0 {
			line += " " + strings.Join(directive.Args, " ")
		}

		// Truncate long lines
		if len(line) > 50 {
			line = line[:47] + "..."
		}

		lines = append(lines, line)
	}

	preview := strings.Join(lines, "\n")
	if len(snippet.Directives) > maxLines {
		preview += "\n..."
	}

	return preview
}

// Detail handles GET requests for the snippet detail page.
func (h *SnippetsHandler) Detail(w http.ResponseWriter, r *http.Request) {
	// Extract snippet name from URL path (e.g., /snippets/site_log)
	path := r.URL.Path
	name := strings.TrimPrefix(path, "/snippets/")
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		http.Redirect(w, r, "/snippets", http.StatusFound)
		return
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Parse snippets and sites
	parser := caddy.NewParser(content)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	sites, _ := parser.ParseSites()

	// Find the snippet matching the name
	var found *caddy.Snippet
	for i := range snippets {
		if snippets[i].Name == name {
			found = &snippets[i]
			break
		}
	}

	if found == nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Build view with usage info
	view := SnippetView{
		Snippet: *found,
		Preview: getSnippetPreview(*found),
	}

	// Find sites using this snippet
	for _, site := range sites {
		for _, imp := range site.Imports {
			if imp == found.Name {
				view.UsageCount++
				if len(site.Addresses) > 0 {
					view.UsedBySites = append(view.UsedBySites, site.Addresses[0])
				}
				break
			}
		}
	}

	// Format the raw block for display
	formattedContent := formatSnippetContent(found)

	type SnippetDetailData struct {
		Snippet          SnippetView
		FormattedContent string
		Error            string
		HasError         bool
	}

	data := SnippetDetailData{
		Snippet:          view,
		FormattedContent: formattedContent,
	}

	pageData := templates.PageData{
		Title:     name + " - Snippet Details",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-detail.html", pageData); err != nil {
		log.Printf("Error rendering snippet detail template: %v", err)
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// formatSnippetContent formats a snippet's content for display.
func formatSnippetContent(snippet *caddy.Snippet) string {
	if snippet == nil {
		return ""
	}

	writer := caddy.NewWriter()
	return writer.WriteSnippet(snippet)
}

// New handles GET requests for the new snippet form page.
func (h *SnippetsHandler) New(w http.ResponseWriter, r *http.Request) {
	data := SnippetFormData{
		Snippet: nil, // nil indicates new snippet
	}

	pageData := templates.PageData{
		Title:     "Add Snippet",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Create handles POST requests to create a new snippet.
func (h *SnippetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil)
		return
	}

	// Extract form values
	name := strings.TrimSpace(r.FormValue("name"))
	content := r.FormValue("content")

	// Store form values for re-rendering on error
	formValues := &SnippetFormValues{
		Name:    name,
		Content: content,
	}

	// Validate name
	if name == "" {
		h.renderFormError(w, r, "Snippet name is required", formValues)
		return
	}

	if !isValidSnippetName(name) {
		h.renderFormError(w, r, "Invalid snippet name. Must start with a letter or underscore, followed by letters, numbers, or underscores.", formValues)
		return
	}

	// Validate content
	if strings.TrimSpace(content) == "" {
		h.renderFormError(w, r, "Snippet content is required", formValues)
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	fileContent, err := reader.Read()
	if err != nil && !errors.Is(err, caddy.ErrCaddyfileNotFound) {
		h.renderFormError(w, r, "Failed to read Caddyfile: "+err.Error(), formValues)
		return
	}

	// Parse the existing config
	var caddyfile *caddy.Caddyfile
	if fileContent != "" {
		parser := caddy.NewParser(fileContent)
		caddyfile, err = parser.ParseAll()
		if err != nil {
			h.renderFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), formValues)
			return
		}
	} else {
		caddyfile = &caddy.Caddyfile{}
	}

	// Check if snippet already exists
	for _, snippet := range caddyfile.Snippets {
		if snippet.Name == name {
			h.renderFormError(w, r, "A snippet with this name already exists", formValues)
			return
		}
	}

	// Create the new snippet by parsing the content
	newSnippet, err := parseSnippetContent(name, content)
	if err != nil {
		h.renderFormError(w, r, "Invalid snippet content: "+err.Error(), formValues)
		return
	}

	// Add the new snippet to the config
	caddyfile.Snippets = append(caddyfile.Snippets, *newSnippet)

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
	if err := h.saveAndWriteCaddyfile(fileContent, newContent, "Before adding snippet: "+name); err != nil {
		h.renderFormError(w, r, "Failed to save Caddyfile: "+err.Error(), formValues)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Redirect to snippets list with appropriate message
	if reloadErr != nil {
		w.Header().Set("HX-Redirect", "/snippets?reload_error="+url.QueryEscape(reloadErr.Error()))
	} else {
		w.Header().Set("HX-Redirect", "/snippets?success="+url.QueryEscape("Snippet created and Caddy reloaded"))
	}
	w.WriteHeader(http.StatusOK)
}

// Edit handles GET requests for the snippet edit form page.
func (h *SnippetsHandler) Edit(w http.ResponseWriter, r *http.Request) {
	// Extract snippet name from URL path (e.g., /snippets/site_log/edit)
	path := r.URL.Path
	name := strings.TrimPrefix(path, "/snippets/")
	name = strings.TrimSuffix(name, "/edit")

	if name == "" {
		http.Redirect(w, r, "/snippets", http.StatusFound)
		return
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to read Caddyfile: "+err.Error(), nil, name)
		return
	}

	// Parse snippets from the Caddyfile
	parser := caddy.NewParser(content)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), nil, name)
		return
	}

	// Find the snippet matching the name
	var found *caddy.Snippet
	for i := range snippets {
		if snippets[i].Name == name {
			found = &snippets[i]
			break
		}
	}

	if found == nil {
		h.renderEditFormError(w, r, "Snippet not found: "+name, nil, name)
		return
	}

	// Convert Snippet to SnippetFormValues
	formValues := snippetToFormValues(found)

	data := SnippetFormData{
		Snippet: formValues,
	}

	pageData := templates.PageData{
		Title:     "Edit Snippet - " + name,
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Update handles PUT requests to update an existing snippet.
func (h *SnippetsHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Extract snippet name from URL path (e.g., /snippets/site_log)
	path := r.URL.Path
	originalName := strings.TrimPrefix(path, "/snippets/")
	originalName = strings.TrimSuffix(originalName, "/")

	if originalName == "" {
		h.renderEditFormError(w, r, "Invalid snippet path", nil, "")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderEditFormError(w, r, "Failed to parse form data", nil, originalName)
		return
	}

	// Extract form values
	name := strings.TrimSpace(r.FormValue("name"))
	content := r.FormValue("content")

	// Store form values for re-rendering on error
	formValues := &SnippetFormValues{
		Name:         name,
		OriginalName: originalName,
		Content:      content,
	}

	// Validate name
	if name == "" {
		h.renderEditFormError(w, r, "Snippet name is required", formValues, originalName)
		return
	}

	if !isValidSnippetName(name) {
		h.renderEditFormError(w, r, "Invalid snippet name. Must start with a letter or underscore, followed by letters, numbers, or underscores.", formValues, originalName)
		return
	}

	// Validate content
	if strings.TrimSpace(content) == "" {
		h.renderEditFormError(w, r, "Snippet content is required", formValues, originalName)
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	fileContent, err := reader.Read()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to read Caddyfile: "+err.Error(), formValues, originalName)
		return
	}

	// Parse the existing config
	parser := caddy.NewParser(fileContent)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		h.renderEditFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), formValues, originalName)
		return
	}

	// Find and update the snippet
	snippetIndex := -1
	for i := range caddyfile.Snippets {
		if caddyfile.Snippets[i].Name == originalName {
			snippetIndex = i
			break
		}
	}

	if snippetIndex == -1 {
		h.renderEditFormError(w, r, "Snippet not found: "+originalName, formValues, originalName)
		return
	}

	// Check if new name conflicts with another snippet (if name changed)
	if name != originalName {
		for i, snippet := range caddyfile.Snippets {
			if i == snippetIndex {
				continue
			}
			if snippet.Name == name {
				h.renderEditFormError(w, r, "A snippet with this name already exists", formValues, originalName)
				return
			}
		}
	}

	// Create the updated snippet by parsing the content
	updatedSnippet, err := parseSnippetContent(name, content)
	if err != nil {
		h.renderEditFormError(w, r, "Invalid snippet content: "+err.Error(), formValues, originalName)
		return
	}

	// Replace the snippet in the config
	caddyfile.Snippets[snippetIndex] = *updatedSnippet

	// Generate the new Caddyfile content
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Validate the new Caddyfile via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.adminClient.ValidateConfig(ctx, newContent); err != nil {
		h.renderEditFormError(w, r, "Invalid configuration: "+err.Error(), formValues, originalName)
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(fileContent, newContent, "Before updating snippet: "+originalName); err != nil {
		h.renderEditFormError(w, r, "Failed to save Caddyfile: "+err.Error(), formValues, originalName)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Redirect to snippets list with appropriate message
	if reloadErr != nil {
		w.Header().Set("HX-Redirect", "/snippets?reload_error="+url.QueryEscape(reloadErr.Error()))
	} else {
		w.Header().Set("HX-Redirect", "/snippets?success="+url.QueryEscape("Snippet updated and Caddy reloaded"))
	}
	w.WriteHeader(http.StatusOK)
}

// Delete handles DELETE requests to remove a snippet.
func (h *SnippetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Extract snippet name from URL path (e.g., /snippets/site_log)
	path := r.URL.Path
	name := strings.TrimPrefix(path, "/snippets/")
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		h.errorHandler.BadRequest(w, r, "Invalid snippet path")
		return
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	fileContent, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Parse the existing config
	parser := caddy.NewParser(fileContent)
	caddyfile, err := parser.ParseAll()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Find and remove the snippet
	snippetIndex := -1
	for i := range caddyfile.Snippets {
		if caddyfile.Snippets[i].Name == name {
			snippetIndex = i
			break
		}
	}

	if snippetIndex == -1 {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Remove the snippet from the slice
	caddyfile.Snippets = append(caddyfile.Snippets[:snippetIndex], caddyfile.Snippets[snippetIndex+1:]...)

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
	if err := h.saveAndWriteCaddyfile(fileContent, newContent, "Before deleting snippet: "+name); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// For HTMX requests, redirect to refresh the snippet list
	if isHTMXRequest(r) {
		if reloadErr != nil {
			w.Header().Set("HX-Redirect", "/snippets?reload_error="+url.QueryEscape(reloadErr.Error()))
		} else {
			w.Header().Set("HX-Redirect", "/snippets?success="+url.QueryEscape("Snippet deleted and Caddy reloaded"))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect to snippets list
	if reloadErr != nil {
		http.Redirect(w, r, "/snippets?reload_error="+url.QueryEscape(reloadErr.Error()), http.StatusFound)
	} else {
		http.Redirect(w, r, "/snippets?success="+url.QueryEscape("Snippet deleted and Caddy reloaded"), http.StatusFound)
	}
}

// Helper functions

// isValidSnippetName checks if a snippet name is valid.
// Must start with a letter or underscore, followed by letters, numbers, or underscores.
func isValidSnippetName(name string) bool {
	if name == "" {
		return false
	}
	// Match: ^[a-zA-Z_][a-zA-Z0-9_]*$
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, name)
	return matched
}

// parseSnippetContent parses snippet content into a Snippet struct.
func parseSnippetContent(name, content string) (*caddy.Snippet, error) {
	// Create a temporary snippet block to parse
	snippetBlock := "(" + name + ") {\n" + content + "\n}\n"

	parser := caddy.NewParser(snippetBlock)
	snippets, err := parser.ParseSnippets()
	if err != nil {
		return nil, err
	}

	if len(snippets) == 0 {
		return nil, errors.New("failed to parse snippet content")
	}

	return &snippets[0], nil
}

// snippetToFormValues converts a Snippet struct to SnippetFormValues for form pre-population.
func snippetToFormValues(snippet *caddy.Snippet) *SnippetFormValues {
	// Generate the content from directives
	var content strings.Builder

	for _, directive := range snippet.Directives {
		writeDirectiveForForm(&content, directive, 0)
	}

	return &SnippetFormValues{
		Name:         snippet.Name,
		OriginalName: snippet.Name,
		Content:      strings.TrimSpace(content.String()),
	}
}

// writeDirectiveForForm writes a directive to a string builder for form display.
func writeDirectiveForForm(sb *strings.Builder, directive caddy.Directive, depth int) {
	indent := strings.Repeat("\t", depth)
	sb.WriteString(indent)
	sb.WriteString(directive.Name)

	for _, arg := range directive.Args {
		sb.WriteString(" ")
		sb.WriteString(arg)
	}

	if len(directive.Block) > 0 {
		sb.WriteString(" {\n")
		for _, nested := range directive.Block {
			writeDirectiveForForm(sb, nested, depth+1)
		}
		sb.WriteString(indent)
		sb.WriteString("}")
	}

	sb.WriteString("\n")
}

// renderFormError renders the form with an error message.
func (h *SnippetsHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SnippetFormValues) {
	log.Printf("Snippet form error: %s", errMsg)

	data := SnippetFormData{
		Snippet:  formValues,
		Error:    errMsg,
		HasError: true,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "snippet-form", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// For regular requests, render the full page
	pageData := templates.PageData{
		Title:     "Add Snippet",
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-new.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderEditFormError renders the edit form with an error message.
func (h *SnippetsHandler) renderEditFormError(w http.ResponseWriter, r *http.Request, errMsg string, formValues *SnippetFormValues, originalName string) {
	log.Printf("Snippet edit form error: %s [name: %s]", errMsg, originalName)

	if formValues == nil {
		formValues = &SnippetFormValues{
			OriginalName: originalName,
			Name:         originalName,
		}
	}
	if formValues.OriginalName == "" {
		formValues.OriginalName = originalName
	}

	data := SnippetFormData{
		Snippet:  formValues,
		Error:    errMsg,
		HasError: true,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "snippet-form", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// For regular requests, render the full page
	pageData := templates.PageData{
		Title:     "Edit Snippet - " + originalName,
		ActiveNav: "snippets",
		Data:      data,
	}

	if err := h.templates.Render(w, "snippet-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// saveAndWriteCaddyfile saves the current Caddyfile to history and writes the new content.
func (h *SnippetsHandler) saveAndWriteCaddyfile(currentContent, newContent, comment string) error {
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
	return os.WriteFile(h.config.CaddyfilePath, []byte(newContent), 0644)
}

// reloadCaddy reloads the Caddy configuration with the given content.
func (h *SnippetsHandler) reloadCaddy(content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.adminClient.Reload(ctx, content)
}
