package handlers

import (
	"errors"
	"net/http"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
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

// GlobalOptionsHandler handles requests for the global options page.
type GlobalOptionsHandler struct {
	templates    *templates.Templates
	config       *config.Config
	errorHandler *ErrorHandler
}

// NewGlobalOptionsHandler creates a new GlobalOptionsHandler.
func NewGlobalOptionsHandler(tmpl *templates.Templates, cfg *config.Config) *GlobalOptionsHandler {
	return &GlobalOptionsHandler{
		templates:    tmpl,
		config:       cfg,
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
