package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// HistoryData holds data for the history page.
type HistoryData struct {
	History        []store.ConfigHistory
	SuccessMessage string
	ErrorMessage   string
}

// HistoryHandler handles requests for configuration history.
type HistoryHandler struct {
	templates    *templates.Templates
	store        *store.Store
	cfg          *config.Config
	errorHandler *ErrorHandler
}

// NewHistoryHandler creates a new HistoryHandler.
func NewHistoryHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *HistoryHandler {
	return &HistoryHandler{
		templates:    tmpl,
		store:        s,
		cfg:          cfg,
		errorHandler: NewErrorHandler(tmpl),
	}
}

// List handles GET /history requests.
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	history, err := h.store.ListConfigs(h.cfg.HistoryLimit)
	if err != nil {
		h.errorHandler.InternalServerError(w, r, err)
		return
	}

	// Check for success or error messages from query params
	historyData := HistoryData{
		History: history,
	}
	if successMsg := r.URL.Query().Get("success"); successMsg != "" {
		historyData.SuccessMessage = successMsg
	}
	if errorMsg := r.URL.Query().Get("error"); errorMsg != "" {
		historyData.ErrorMessage = errorMsg
	}

	data := WithPermissions(r, "History", "history", historyData)

	if err := h.templates.Render(w, "history.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// View handles GET /history/{id}/view requests - shows raw content.
func (h *HistoryHandler) View(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseIDFromPath(r.URL.Path)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid history ID")
		return
	}

	config, err := h.store.GetConfig(id)
	if err != nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Return the content wrapped in a pre tag for proper formatting
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<pre class="whitespace-pre-wrap text-gray-800">`))
	// HTML escape the content
	escaped := strings.ReplaceAll(config.Content, "&", "&amp;")
	escaped = strings.ReplaceAll(escaped, "<", "&lt;")
	escaped = strings.ReplaceAll(escaped, ">", "&gt;")
	w.Write([]byte(escaped))
	w.Write([]byte(`</pre>`))
}

// Diff handles GET /history/{id}/diff requests - shows diff against current.
func (h *HistoryHandler) Diff(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseIDFromPath(r.URL.Path)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid history ID")
		return
	}

	// Get the selected version
	selected, err := h.store.GetConfig(id)
	if err != nil {
		h.errorHandler.NotFound(w, r)
		return
	}

	// Get the current (latest) version
	latest, err := h.store.LatestConfig()
	if err != nil || latest == nil {
		h.errorHandler.InternalServerError(w, r, fmt.Errorf("could not find current configuration"))
		return
	}

	// Generate diff
	diff := generateDiff(selected.Content, latest.Content)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div class="diff-container">`))
	w.Write([]byte(`<div class="mb-2 text-sm text-gray-600">Comparing version #` + strconv.FormatInt(id, 10) + ` to current (#` + strconv.FormatInt(latest.ID, 10) + `)</div>`))
	w.Write([]byte(`<pre class="whitespace-pre-wrap">`))
	w.Write([]byte(diff))
	w.Write([]byte(`</pre></div>`))
}

// parseIDFromPath extracts the ID from paths like /history/{id}/view or /history/{id}/diff
func (h *HistoryHandler) parseIDFromPath(path string) (int64, error) {
	// Path format: /history/{id}/view or /history/{id}/diff
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

// generateDiff creates a simple line-by-line diff between old and new content.
func generateDiff(old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	var result strings.Builder

	// Use a simple longest common subsequence approach for diffing
	diff := computeDiff(oldLines, newLines)

	for _, d := range diff {
		line := strings.ReplaceAll(d.Text, "&", "&amp;")
		line = strings.ReplaceAll(line, "<", "&lt;")
		line = strings.ReplaceAll(line, ">", "&gt;")

		switch d.Type {
		case diffRemoved:
			result.WriteString(`<span class="text-red-600 bg-red-50">- `)
			result.WriteString(line)
			result.WriteString("</span>\n")
		case diffAdded:
			result.WriteString(`<span class="text-green-600 bg-green-50">+ `)
			result.WriteString(line)
			result.WriteString("</span>\n")
		case diffUnchanged:
			result.WriteString(`<span class="text-gray-600">  `)
			result.WriteString(line)
			result.WriteString("</span>\n")
		}
	}

	return result.String()
}

type diffType int

const (
	diffUnchanged diffType = iota
	diffAdded
	diffRemoved
)

type diffLine struct {
	Type diffType
	Text string
}

// computeDiff computes a simple diff between old and new lines using Myers' algorithm variant.
func computeDiff(old, new []string) []diffLine {
	// Build a map of new lines for quick lookup
	oldSet := make(map[string]bool)
	for _, line := range old {
		oldSet[line] = true
	}

	newSet := make(map[string]bool)
	for _, line := range new {
		newSet[line] = true
	}

	// Simple two-pointer approach with line matching
	var result []diffLine
	i, j := 0, 0

	for i < len(old) || j < len(new) {
		if i >= len(old) {
			// Remaining lines in new are additions
			result = append(result, diffLine{Type: diffAdded, Text: new[j]})
			j++
		} else if j >= len(new) {
			// Remaining lines in old are removals
			result = append(result, diffLine{Type: diffRemoved, Text: old[i]})
			i++
		} else if old[i] == new[j] {
			// Lines match
			result = append(result, diffLine{Type: diffUnchanged, Text: old[i]})
			i++
			j++
		} else {
			// Look ahead to find if old[i] exists later in new
			foundOldInNew := false
			for k := j + 1; k < len(new) && k < j+5; k++ {
				if old[i] == new[k] {
					foundOldInNew = true
					break
				}
			}

			// Look ahead to find if new[j] exists later in old
			foundNewInOld := false
			for k := i + 1; k < len(old) && k < i+5; k++ {
				if new[j] == old[k] {
					foundNewInOld = true
					break
				}
			}

			if foundOldInNew && !foundNewInOld {
				// The new line was added
				result = append(result, diffLine{Type: diffAdded, Text: new[j]})
				j++
			} else if foundNewInOld && !foundOldInNew {
				// The old line was removed
				result = append(result, diffLine{Type: diffRemoved, Text: old[i]})
				i++
			} else {
				// Default: show as removed then added (replacement)
				result = append(result, diffLine{Type: diffRemoved, Text: old[i]})
				result = append(result, diffLine{Type: diffAdded, Text: new[j]})
				i++
				j++
			}
		}
	}

	return result
}

// Restore handles POST /history/{id}/restore requests - restores a config version.
func (h *HistoryHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseIDFromPath(r.URL.Path)
	if err != nil {
		h.errorHandler.BadRequest(w, r, "Invalid history ID")
		return
	}

	// Get the config to restore
	configToRestore, err := h.store.GetConfig(id)
	if err != nil {
		redirectWithError(w, r, "History entry not found")
		return
	}

	// Validate the config before applying via Caddy Admin API
	adminClient := caddy.NewAdminClient(h.cfg.CaddyAdminAPI)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := adminClient.ValidateConfig(ctx, configToRestore.Content); err != nil {
		redirectWithError(w, r, fmt.Sprintf("Invalid configuration: %s", err.Error()))
		return
	}

	// Read current Caddyfile content to save to history before restoring
	reader := caddy.NewReader(h.cfg.CaddyfilePath)
	currentContent, err := reader.Read()
	if err == nil && currentContent != "" && currentContent != configToRestore.Content {
		// Save current config to history before overwriting
		if err := h.store.SaveConfigHistory(currentContent, fmt.Sprintf("Before restoring version #%d", id)); err != nil {
			log.Printf("Warning: failed to save config history before restore: %v", err)
		}

		// Prune old history entries
		if err := h.store.PruneConfigHistory(h.cfg.HistoryLimit); err != nil {
			log.Printf("Warning: failed to prune config history: %v", err)
		}
	}

	// Write the restored config to the Caddyfile
	if err := os.WriteFile(h.cfg.CaddyfilePath, []byte(configToRestore.Content), 0644); err != nil {
		redirectWithError(w, r, fmt.Sprintf("Failed to write Caddyfile: %s", err.Error()))
		return
	}

	// Reload Caddy with the restored config
	ctx2, cancel2 := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel2()

	if err := adminClient.Reload(ctx2, configToRestore.Content); err != nil {
		// Config is saved but reload failed
		redirectWithError(w, r, fmt.Sprintf("Configuration restored but Caddy reload failed: %s", err.Error()))
		return
	}

	// Success - redirect back to history page with success message
	redirectURL := "/history?success=" + url.QueryEscape(fmt.Sprintf("Configuration version #%d restored and Caddy reloaded", id))
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// redirectWithError redirects to the history page with an error message.
func redirectWithError(w http.ResponseWriter, r *http.Request, errMsg string) {
	redirectURL := "/history?error=" + url.QueryEscape(errMsg)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}
