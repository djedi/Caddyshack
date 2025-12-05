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

// GlobalOptionsData holds data displayed on the global options page.
type GlobalOptionsData struct {
	GlobalOptions    *caddy.GlobalOptions
	HasGlobalOptions bool
	Error            string
	HasError         bool
	SuccessMessage   string
	ReloadError      string
}

// GlobalOptionsFormData holds data for the global options edit form.
type GlobalOptionsFormData struct {
	GlobalOptions *caddy.GlobalOptions
	Error         string
	HasError      bool
}

// GlobalOptionsHandler handles requests for the global options page.
type GlobalOptionsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	adminClient  *caddy.AdminClient
	store        *store.Store
	errorHandler *ErrorHandler
}

// NewGlobalOptionsHandler creates a new GlobalOptionsHandler.
func NewGlobalOptionsHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *GlobalOptionsHandler {
	return &GlobalOptionsHandler{
		templates:    tmpl,
		config:       cfg,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		store:        s,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET requests for the global options page.
func (h *GlobalOptionsHandler) List(w http.ResponseWriter, r *http.Request) {
	data := GlobalOptionsData{
		GlobalOptions: &caddy.GlobalOptions{}, // Always initialize to avoid nil pointer in template
	}

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
		// Parse global options from the Caddyfile
		parser := caddy.NewParser(content)
		globalOpts, err := parser.ParseGlobalOptions()
		if err != nil {
			data.Error = "Failed to parse global options: " + err.Error()
			data.HasError = true
		} else {
			if globalOpts != nil {
				data.GlobalOptions = globalOpts
				data.HasGlobalOptions = true
			} else {
				// No global options block found, use empty defaults
				data.HasGlobalOptions = false
			}
		}
	}

	pageData := templates.PageData{
		Title:     "Global Options",
		ActiveNav: "global",
		Data:      data,
	}

	if err := h.templates.Render(w, "global-options.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Edit handles GET requests for the global options edit page.
func (h *GlobalOptionsHandler) Edit(w http.ResponseWriter, r *http.Request) {
	data := GlobalOptionsFormData{
		GlobalOptions: &caddy.GlobalOptions{}, // Initialize to avoid nil pointer
	}

	// Read and parse the Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		if !errors.Is(err, caddy.ErrCaddyfileNotFound) {
			data.Error = "Failed to read Caddyfile: " + err.Error()
			data.HasError = true
		}
		// If file not found, continue with empty GlobalOptions
	} else {
		// Parse global options from the Caddyfile
		parser := caddy.NewParser(content)
		globalOpts, err := parser.ParseGlobalOptions()
		if err != nil {
			data.Error = "Failed to parse global options: " + err.Error()
			data.HasError = true
		} else if globalOpts != nil {
			data.GlobalOptions = globalOpts
		}
	}

	// Ensure LogConfig is initialized for the template
	if data.GlobalOptions.LogConfig == nil {
		data.GlobalOptions.LogConfig = &caddy.LogConfig{}
	}

	pageData := templates.PageData{
		Title:     "Edit Global Options",
		ActiveNav: "global",
		Data:      data,
	}

	if err := h.templates.Render(w, "global-options-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Update handles PUT requests to update global options.
func (h *GlobalOptionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFormError(w, r, "Failed to parse form data", nil)
		return
	}

	// Extract form values
	email := strings.TrimSpace(r.FormValue("email"))
	admin := strings.TrimSpace(r.FormValue("admin"))
	acmeCa := strings.TrimSpace(r.FormValue("acme_ca"))
	debug := r.FormValue("debug") == "on" || r.FormValue("debug") == "true"
	logOutput := strings.TrimSpace(r.FormValue("log_output"))
	logFormat := strings.TrimSpace(r.FormValue("log_format"))
	logLevel := strings.TrimSpace(r.FormValue("log_level"))
	logRollSize := strings.TrimSpace(r.FormValue("log_roll_size"))
	logRollKeep := strings.TrimSpace(r.FormValue("log_roll_keep"))
	rawBlock := strings.TrimSpace(r.FormValue("raw_block"))

	// Build the GlobalOptions struct
	globalOpts := &caddy.GlobalOptions{
		Email:  email,
		Admin:  admin,
		ACMECa: acmeCa,
		Debug:  debug,
	}

	// Add log config if any log fields are set
	if logOutput != "" || logFormat != "" || logLevel != "" || logRollSize != "" || logRollKeep != "" {
		globalOpts.LogConfig = &caddy.LogConfig{
			Output:   logOutput,
			Format:   logFormat,
			Level:    logLevel,
			RollSize: logRollSize,
			RollKeep: logRollKeep,
		}
	}

	// If raw block is provided, use it instead of form fields
	if rawBlock != "" {
		globalOpts = &caddy.GlobalOptions{
			RawBlock: rawBlock,
		}
		// Parse the raw block to extract structured fields for display
		// We wrap it in braces to parse it properly
		tempContent := "{\n" + rawBlock + "\n}"
		tempParser := caddy.NewParser(tempContent)
		parsedOpts, err := tempParser.ParseGlobalOptions()
		if err == nil && parsedOpts != nil {
			// Use parsed values but keep the raw block
			globalOpts = parsedOpts
			globalOpts.RawBlock = rawBlock
		}
	}

	// Read and parse the existing Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil && !errors.Is(err, caddy.ErrCaddyfileNotFound) {
		h.renderFormError(w, r, "Failed to read Caddyfile: "+err.Error(), globalOpts)
		return
	}

	// Parse the existing config
	var caddyfile *caddy.Caddyfile
	if content != "" {
		parser := caddy.NewParser(content)
		caddyfile, err = parser.ParseAll()
		if err != nil {
			h.renderFormError(w, r, "Failed to parse Caddyfile: "+err.Error(), globalOpts)
			return
		}
	} else {
		caddyfile = &caddy.Caddyfile{}
	}

	// Update global options
	caddyfile.GlobalOptions = globalOpts

	// Generate the new Caddyfile content
	writer := caddy.NewWriter()
	newContent := writer.WriteCaddyfile(caddyfile)

	// Validate the new Caddyfile via Caddy Admin API
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.adminClient.ValidateConfig(ctx, newContent); err != nil {
		h.renderFormError(w, r, "Invalid configuration: "+err.Error(), globalOpts)
		return
	}

	// Save history and write the new Caddyfile
	if err := h.saveAndWriteCaddyfile(content, newContent, "Before updating global options"); err != nil {
		h.renderFormError(w, r, "Failed to save Caddyfile: "+err.Error(), globalOpts)
		return
	}

	// Reload Caddy configuration
	reloadErr := h.reloadCaddy(newContent)

	// Redirect to global options page with appropriate message
	if reloadErr != nil {
		w.Header().Set("HX-Redirect", "/global-options?reload_error="+url.QueryEscape(reloadErr.Error()))
	} else {
		w.Header().Set("HX-Redirect", "/global-options?success="+url.QueryEscape("Global options updated and Caddy reloaded"))
	}
	w.WriteHeader(http.StatusOK)
}

// renderFormError renders the edit form with an error message.
func (h *GlobalOptionsHandler) renderFormError(w http.ResponseWriter, r *http.Request, errMsg string, globalOpts *caddy.GlobalOptions) {
	log.Printf("Global options form error: %s", errMsg)

	if globalOpts == nil {
		globalOpts = &caddy.GlobalOptions{}
	}
	if globalOpts.LogConfig == nil {
		globalOpts.LogConfig = &caddy.LogConfig{}
	}

	data := GlobalOptionsFormData{
		GlobalOptions: globalOpts,
		Error:         errMsg,
		HasError:      true,
	}

	// For HTMX requests, return just the form partial
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.RenderPartial(w, "global-options-form", data); err != nil {
			h.errorHandler.InternalServerError(w, r, err)
		}
		return
	}

	// For regular requests, render the full page
	pageData := templates.PageData{
		Title:     "Edit Global Options",
		ActiveNav: "global",
		Data:      data,
	}

	if err := h.templates.Render(w, "global-options-edit.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// saveAndWriteCaddyfile saves the current Caddyfile to history and writes the new content.
func (h *GlobalOptionsHandler) saveAndWriteCaddyfile(currentContent, newContent, comment string) error {
	// Only save history if there's existing content and it's different
	if currentContent != "" && currentContent != newContent {
		if err := h.store.SaveConfigHistory(currentContent, comment); err != nil {
			log.Printf("Warning: failed to save config history: %v", err)
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
func (h *GlobalOptionsHandler) reloadCaddy(content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.adminClient.Reload(ctx, content)
}
