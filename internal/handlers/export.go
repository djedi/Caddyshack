package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/templates"
)

// ExportHandler handles requests for exporting Caddyfile configurations.
type ExportHandler struct {
	templates    *templates.Templates
	config       *config.Config
	adminClient  *caddy.AdminClient
	errorHandler *ErrorHandler
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(tmpl *templates.Templates, cfg *config.Config) *ExportHandler {
	return &ExportHandler{
		templates:    tmpl,
		config:       cfg,
		adminClient:  caddy.NewAdminClient(cfg.CaddyAdminAPI),
		errorHandler: NewErrorHandler(tmpl),
	}
}

// ExportCaddyfile handles GET /export and returns the current Caddyfile as a downloadable file.
func (h *ExportHandler) ExportCaddyfile(w http.ResponseWriter, r *http.Request) {
	// Read the current Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	content, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("reading Caddyfile: %w", err))
		return
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("Caddyfile-%s.txt", timestamp)

	// Set headers for file download
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
}

// ExportJSON handles GET /export/json and returns the current running config as JSON.
func (h *ExportHandler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get the current running config from Caddy Admin API
	configJSON, err := h.adminClient.GetConfig(ctx)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("getting config from Caddy: %w", err))
		return
	}

	// Pretty-print the JSON
	var prettyJSON []byte
	var rawData interface{}
	if err := json.Unmarshal(configJSON, &rawData); err == nil {
		prettyJSON, _ = json.MarshalIndent(rawData, "", "  ")
	} else {
		prettyJSON = configJSON
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("caddy-config-%s.json", timestamp)

	// Set headers for file download
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(prettyJSON)))

	w.WriteHeader(http.StatusOK)
	w.Write(prettyJSON)
}
