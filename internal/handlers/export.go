package handlers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// ExportHandler handles requests for exporting Caddyfile configurations.
type ExportHandler struct {
	templates    *templates.Templates
	config       *config.Config
	store        *store.Store
	adminClient  *caddy.AdminClient
	errorHandler *ErrorHandler
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *ExportHandler {
	return &ExportHandler{
		templates:    tmpl,
		config:       cfg,
		store:        s,
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

// BackupData represents the structure of the backup JSON.
type BackupData struct {
	ExportedAt string                `json:"exported_at"`
	Caddyfile  string                `json:"caddyfile"`
	History    []BackupHistoryEntry  `json:"history"`
}

// BackupHistoryEntry represents a single history entry in the backup.
type BackupHistoryEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
	Comment   string `json:"comment"`
}

// ExportBackup handles GET /export/backup and returns a ZIP file containing
// the current Caddyfile and all configuration history.
func (h *ExportHandler) ExportBackup(w http.ResponseWriter, r *http.Request) {
	// Read the current Caddyfile
	reader := caddy.NewReader(h.config.CaddyfilePath)
	caddyfileContent, err := reader.Read()
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("reading Caddyfile: %w", err))
		return
	}

	// Get all history entries (pass 0 to get all)
	historyEntries, err := h.store.ListConfigs(0)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("reading config history: %w", err))
		return
	}

	// Build backup data structure
	backupHistory := make([]BackupHistoryEntry, len(historyEntries))
	for i, entry := range historyEntries {
		backupHistory[i] = BackupHistoryEntry{
			ID:        entry.ID,
			Timestamp: entry.Timestamp.Format(time.RFC3339),
			Content:   entry.Content,
			Comment:   entry.Comment,
		}
	}

	backupData := BackupData{
		ExportedAt: time.Now().Format(time.RFC3339),
		Caddyfile:  caddyfileContent,
		History:    backupHistory,
	}

	// Convert to JSON
	backupJSON, err := json.MarshalIndent(backupData, "", "  ")
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("marshaling backup data: %w", err))
		return
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	zipFilename := fmt.Sprintf("caddyshack-backup-%s.zip", timestamp)

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipFilename))

	// Create ZIP file directly to response writer
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Add Caddyfile to ZIP
	caddyfileWriter, err := zipWriter.Create("Caddyfile")
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("creating Caddyfile in zip: %w", err))
		return
	}
	if _, err := caddyfileWriter.Write([]byte(caddyfileContent)); err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("writing Caddyfile to zip: %w", err))
		return
	}

	// Add backup.json to ZIP
	backupWriter, err := zipWriter.Create("backup.json")
	if err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("creating backup.json in zip: %w", err))
		return
	}
	if _, err := backupWriter.Write(backupJSON); err != nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("writing backup.json to zip: %w", err))
		return
	}

	// Add individual history files for convenience
	for _, entry := range historyEntries {
		historyFilename := fmt.Sprintf("history/Caddyfile-%d-%s.txt", entry.ID, entry.Timestamp.Format("2006-01-02-150405"))
		historyWriter, err := zipWriter.Create(historyFilename)
		if err != nil {
			continue // Skip this entry if we can't create it
		}
		historyWriter.Write([]byte(entry.Content))
	}
}
