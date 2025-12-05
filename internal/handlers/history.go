package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

// HistoryData holds data for the history page.
type HistoryData struct {
	History []store.ConfigHistory
}

// HistoryHandler handles requests for configuration history.
type HistoryHandler struct {
	templates *templates.Templates
	store     *store.Store
	cfg       *config.Config
}

// NewHistoryHandler creates a new HistoryHandler.
func NewHistoryHandler(tmpl *templates.Templates, cfg *config.Config, s *store.Store) *HistoryHandler {
	return &HistoryHandler{
		templates: tmpl,
		store:     s,
		cfg:       cfg,
	}
}

// List handles GET /history requests.
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	history, err := h.store.ListConfigs(h.cfg.HistoryLimit)
	if err != nil {
		http.Error(w, "Failed to load history", http.StatusInternalServerError)
		return
	}

	data := templates.PageData{
		Title:     "History",
		ActiveNav: "history",
		Data: HistoryData{
			History: history,
		},
	}

	if err := h.templates.Render(w, "history.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// View handles GET /history/{id}/view requests - shows raw content.
func (h *HistoryHandler) View(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid history ID", http.StatusBadRequest)
		return
	}

	config, err := h.store.GetConfig(id)
	if err != nil {
		http.Error(w, "History entry not found", http.StatusNotFound)
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
		http.Error(w, "Invalid history ID", http.StatusBadRequest)
		return
	}

	// Get the selected version
	selected, err := h.store.GetConfig(id)
	if err != nil {
		http.Error(w, "History entry not found", http.StatusNotFound)
		return
	}

	// Get the current (latest) version
	latest, err := h.store.LatestConfig()
	if err != nil || latest == nil {
		http.Error(w, "Could not find current configuration", http.StatusInternalServerError)
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
