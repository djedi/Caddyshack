package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// ImportData holds data displayed on the import page.
type ImportData struct {
	SuccessMessage string
	ErrorMessage   string
}

// ImportPreviewData holds data for the import preview partial.
type ImportPreviewData struct {
	Content       string
	Sites         []caddy.Site
	Snippets      []caddy.Snippet
	GlobalOptions *caddy.GlobalOptions
	IsValid       bool
	ValidationErr string
	SiteCount     int
	SnippetCount  int
}

// ImportHandler handles requests for importing Caddyfile configurations.
type ImportHandler struct {
	templates    *templates.Templates
	config       *config.Config
	adminClient  *caddy.AdminClient
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewImportHandler creates a new ImportHandler.
func NewImportHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *ImportHandler {
	return &ImportHandler{
		templates:    tmpl,
		config:       cfg,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// ImportPage handles GET /import and displays the import form.
func (h *ImportHandler) ImportPage(w http.ResponseWriter, r *http.Request) {
	importData := ImportData{}

	// Check for success or error messages from query params
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		importData.SuccessMessage = successMsg
	}
	if errorMsg := r.URL.Query().Get("error"); errorMsg != "" {
		importData.ErrorMessage = errorMsg
	}
	if reloadErr := r.URL.Query().Get("reload_error"); reloadErr != "" {
		if importData.SuccessMessage != "" {
			importData.SuccessMessage += " (Warning: Caddy reload failed: " + reloadErr + ")"
		} else {
			importData.ErrorMessage = "Caddy reload failed: " + reloadErr
		}
	}

	data := templates.PageData{
		ActiveNav: "import",
		Data:      importData,
	}

	if err := h.templates.Render(w, "import.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Preview handles POST /import/preview and returns a preview of the import.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var content string

	// Check if this is a file upload or pasted content
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle file upload
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
			h.renderPreviewError(w, "Failed to parse upload: "+err.Error())
			return
		}

		file, _, err := r.FormFile("caddyfile")
		if err != nil {
			h.renderPreviewError(w, "No file uploaded")
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			h.renderPreviewError(w, "Failed to read file: "+err.Error())
			return
		}
		content = string(data)
	} else {
		// Handle pasted content
		if err := r.ParseForm(); err != nil {
			h.renderPreviewError(w, "Failed to parse form: "+err.Error())
			return
		}
		content = r.FormValue("content")
	}

	if strings.TrimSpace(content) == "" {
		h.renderPreviewError(w, "No content provided")
		return
	}

	// Parse the Caddyfile content
	parser := caddy.NewParser(content)

	sites, err := parser.ParseSites()
	if err != nil {
		h.renderPreviewError(w, "Failed to parse sites: "+err.Error())
		return
	}

	snippets, err := parser.ParseSnippets()
	if err != nil {
		h.renderPreviewError(w, "Failed to parse snippets: "+err.Error())
		return
	}

	globalOptions, err := parser.ParseGlobalOptions()
	if err != nil {
		h.renderPreviewError(w, "Failed to parse global options: "+err.Error())
		return
	}

	// Validate using Caddy Admin API if available
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	isValid := true
	validationErr := ""
	if err := h.adminClient.ValidateConfig(ctx, content); err != nil {
		isValid = false
		validationErr = err.Error()
	}

	previewData := ImportPreviewData{
		Content:       content,
		Sites:         sites,
		Snippets:      snippets,
		GlobalOptions: globalOptions,
		IsValid:       isValid,
		ValidationErr: validationErr,
		SiteCount:     len(sites),
		SnippetCount:  len(snippets),
	}

	h.renderPreview(w, previewData)
}

// Apply handles POST /import/apply and applies the imported configuration.
func (h *ImportHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderImportError(w, r, "Failed to parse form: "+err.Error())
		return
	}

	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" {
		h.renderImportError(w, r, "No content provided")
		return
	}

	// Validate using Caddy Admin API before applying
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.adminClient.ValidateConfig(ctx, content); err != nil {
		h.renderImportError(w, r, "Validation failed: "+err.Error())
		return
	}

	// Save current configuration to history before overwriting
	reader := caddy.NewReader(h.config.CaddyfilePath)
	existingContent, err := reader.Read()
	if err != nil && !errors.Is(err, caddy.ErrCaddyfileNotFound) {
		h.renderImportError(w, r, "Failed to read current Caddyfile: "+err.Error())
		return
	}

	// Only save history if there's existing content and it's different
	if existingContent != "" && existingContent != content {
		if err := h.store.SaveConfigHistory(existingContent, "Before import"); err != nil {
			log.Printf("Warning: failed to save config history: %v", err)
		}
		// Prune old history entries
		if err := h.store.PruneConfigHistory(h.config.HistoryLimit); err != nil {
			log.Printf("Warning: failed to prune config history: %v", err)
		}
	}

	// Write the new Caddyfile
	if err := os.WriteFile(h.config.CaddyfilePath, []byte(content), 0644); err != nil {
		h.renderImportError(w, r, "Failed to write Caddyfile: "+err.Error())
		return
	}

	// Reload Caddy
	reloadCtx, reloadCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer reloadCancel()

	reloadErr := ""
	if err := h.adminClient.Reload(reloadCtx, content); err != nil {
		reloadErr = err.Error()
	}

	// Redirect to import page with success message
	if reloadErr != "" {
		http.Redirect(w, r, "/import?success="+url.QueryEscape("Import applied successfully")+"&reload_error="+url.QueryEscape(reloadErr), http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/import?success="+url.QueryEscape("Import applied and Caddy reloaded successfully"), http.StatusSeeOther)
	}
}

// renderPreviewError renders an error in the preview section.
func (h *ImportHandler) renderPreviewError(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
<div class="bg-white rounded-lg shadow-md p-6">
    <div class="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative" role="alert">
        <strong class="font-bold">Error: </strong>
        <span class="block sm:inline">%s</span>
    </div>
</div>
`, errMsg)
}

// renderPreview renders the preview partial.
func (h *ImportHandler) renderPreview(w http.ResponseWriter, data ImportPreviewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Validation status indicator
	validationHTML := ""
	if data.IsValid {
		validationHTML = `
<div class="flex items-center text-green-600 mb-4">
    <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
    </svg>
    <span class="font-medium">Caddyfile syntax is valid</span>
</div>`
	} else {
		validationHTML = fmt.Sprintf(`
<div class="bg-yellow-100 border border-yellow-400 text-yellow-700 px-4 py-3 rounded mb-4" role="alert">
    <div class="flex items-center">
        <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
        </svg>
        <span class="font-medium">Validation Warning</span>
    </div>
    <p class="mt-2 text-sm">%s</p>
    <p class="mt-1 text-sm">Note: You can still apply this configuration, but it may cause issues.</p>
</div>`, data.ValidationErr)
	}

	// Sites list
	sitesHTML := ""
	if len(data.Sites) > 0 {
		sitesHTML = `<div class="mb-4">
    <h4 class="text-sm font-semibold text-gray-700 mb-2">Sites to import:</h4>
    <ul class="list-disc list-inside text-sm text-gray-600">`
		for _, site := range data.Sites {
			addresses := strings.Join(site.Addresses, ", ")
			sitesHTML += fmt.Sprintf(`<li>%s</li>`, addresses)
		}
		sitesHTML += `</ul></div>`
	}

	// Snippets list
	snippetsHTML := ""
	if len(data.Snippets) > 0 {
		snippetsHTML = `<div class="mb-4">
    <h4 class="text-sm font-semibold text-gray-700 mb-2">Snippets to import:</h4>
    <ul class="list-disc list-inside text-sm text-gray-600">`
		for _, snippet := range data.Snippets {
			snippetsHTML += fmt.Sprintf(`<li>(%s)</li>`, snippet.Name)
		}
		snippetsHTML += `</ul></div>`
	}

	// Global options indicator
	globalHTML := ""
	if data.GlobalOptions != nil && data.GlobalOptions.RawBlock != "" {
		globalHTML = `<div class="mb-4">
    <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
        Includes Global Options
    </span>
</div>`
	}

	// Content preview
	contentPreview := data.Content
	if len(contentPreview) > 2000 {
		contentPreview = contentPreview[:2000] + "\n... (truncated)"
	}

	fmt.Fprintf(w, `
<div class="bg-white rounded-lg shadow-md p-6">
    <h3 class="text-lg font-semibold text-gray-800 mb-4">Import Preview</h3>

    %s

    <div class="grid grid-cols-2 gap-4 mb-4">
        <div class="bg-gray-50 p-3 rounded">
            <span class="text-2xl font-bold text-gray-900">%d</span>
            <span class="text-sm text-gray-500 ml-2">Sites</span>
        </div>
        <div class="bg-gray-50 p-3 rounded">
            <span class="text-2xl font-bold text-gray-900">%d</span>
            <span class="text-sm text-gray-500 ml-2">Snippets</span>
        </div>
    </div>

    %s
    %s
    %s

    <div class="mb-4">
        <h4 class="text-sm font-semibold text-gray-700 mb-2">Content Preview:</h4>
        <pre class="bg-gray-50 p-4 rounded text-sm font-mono overflow-x-auto max-h-64 overflow-y-auto">%s</pre>
    </div>

    <div class="bg-yellow-50 border border-yellow-200 p-4 rounded mb-4">
        <div class="flex items-start">
            <svg class="w-5 h-5 text-yellow-600 mr-2 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
            </svg>
            <div>
                <p class="text-sm text-yellow-800 font-medium">Warning</p>
                <p class="text-sm text-yellow-700">Applying this import will <strong>replace</strong> your current Caddyfile. The current configuration will be saved to history.</p>
            </div>
        </div>
    </div>

    <form method="POST" action="/import/apply" x-data="{ applying: false }" @submit="applying = true">
        <input type="hidden" name="content" value="%s">
        <div class="flex justify-end space-x-3">
            <a href="/import" class="px-4 py-2 text-gray-700 bg-gray-200 rounded-md hover:bg-gray-300 transition-colors">
                Cancel
            </a>
            <button type="submit"
                    class="inline-flex items-center px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    :disabled="applying">
                <svg x-show="applying" class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                </svg>
                <span x-text="applying ? 'Applying...' : 'Apply Import'"></span>
            </button>
        </div>
    </form>
</div>
`, validationHTML, data.SiteCount, data.SnippetCount, globalHTML, sitesHTML, snippetsHTML, escapeHTML(contentPreview), escapeHTML(data.Content))
}

// renderImportError redirects to import page with error message.
func (h *ImportHandler) renderImportError(w http.ResponseWriter, r *http.Request, errMsg string) {
	http.Redirect(w, r, "/import?error="+errMsg, http.StatusSeeOther)
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
