package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/store"
	"github.com/djedi/caddyshack/internal/templates"
)

func setupHistoryHandler(t *testing.T) (*HistoryHandler, *store.Store, string) {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	caddyfilePath := filepath.Join(tempDir, "Caddyfile")

	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
	})

	cfg := &config.Config{
		CaddyfilePath: caddyfilePath,
		HistoryLimit:  50,
		CaddyAdminAPI: "http://localhost:2019",
	}

	handler := NewHistoryHandler(tmpl, cfg, s)
	return handler, s, caddyfilePath
}

func TestHistoryHandler_List_Empty(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "History") {
		t.Errorf("Response should contain 'History', got: %s", body)
	}
}

func TestHistoryHandler_List_WithHistory(t *testing.T) {
	handler, s, _ := setupHistoryHandler(t)

	// Add some history entries
	if _, err := s.SaveConfig("test config 1", "Initial config"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}
	if _, err := s.SaveConfig("test config 2", "Updated config"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "History") {
		t.Errorf("Response should contain 'History', got: %s", body)
	}
}

func TestHistoryHandler_List_WithSuccessMessage(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history?success=Configuration+restored", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Configuration restored") {
		t.Errorf("Response should contain success message, got: %s", body)
	}
}

func TestHistoryHandler_List_WithErrorMessage(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history?error=Validation+failed", nil)
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Validation failed") {
		t.Errorf("Response should contain error message, got: %s", body)
	}
}

func TestHistoryHandler_View_Success(t *testing.T) {
	handler, s, _ := setupHistoryHandler(t)

	// Add a history entry
	if _, err := s.SaveConfig("example.com {\n    reverse_proxy localhost:8080\n}", "Test config"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/history/1/view", nil)
	rec := httptest.NewRecorder()

	handler.View(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "example.com") {
		t.Errorf("Response should contain config content, got: %s", body)
	}
	if !strings.Contains(body, "reverse_proxy") {
		t.Errorf("Response should contain 'reverse_proxy', got: %s", body)
	}
}

func TestHistoryHandler_View_NotFound(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history/999/view", nil)
	rec := httptest.NewRecorder()

	handler.View(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestHistoryHandler_View_InvalidID(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history/invalid/view", nil)
	rec := httptest.NewRecorder()

	handler.View(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestHistoryHandler_View_HTMLEscaping(t *testing.T) {
	handler, s, _ := setupHistoryHandler(t)

	// Add a history entry with HTML content that should be escaped
	if _, err := s.SaveConfig("<script>alert('xss')</script>", "Test XSS"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/history/1/view", nil)
	rec := httptest.NewRecorder()

	handler.View(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("HTML should be escaped in response")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("Response should contain escaped HTML, got: %s", body)
	}
}

func TestHistoryHandler_Diff_Success(t *testing.T) {
	handler, s, _ := setupHistoryHandler(t)

	// Add two history entries
	if _, err := s.SaveConfig("old config", "Old version"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}
	if _, err := s.SaveConfig("new config", "New version"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/history/1/diff", nil)
	rec := httptest.NewRecorder()

	handler.Diff(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "diff-container") {
		t.Errorf("Response should contain diff container, got: %s", body)
	}
}

func TestHistoryHandler_Diff_NotFound(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history/999/diff", nil)
	rec := httptest.NewRecorder()

	handler.Diff(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestHistoryHandler_Diff_InvalidID(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/history/abc/diff", nil)
	rec := httptest.NewRecorder()

	handler.Diff(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestHistoryHandler_Diff_NoCurrentConfig(t *testing.T) {
	handler, s, _ := setupHistoryHandler(t)

	// Add only one history entry (no "current" to compare to)
	if _, err := s.SaveConfig("only config", "Only version"); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/history/1/diff", nil)
	rec := httptest.NewRecorder()

	handler.Diff(rec, req)

	// Should return OK with diff against itself
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHistoryHandler_ParseIDFromPath(t *testing.T) {
	handler, _, _ := setupHistoryHandler(t)

	tests := []struct {
		path      string
		expected  int64
		expectErr bool
	}{
		{"/history/1/view", 1, false},
		{"/history/42/diff", 42, false},
		{"/history/123/restore", 123, false},
		{"/history/abc/view", 0, true},
		{"/history", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			id, err := handler.parseIDFromPath(tt.path)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for path %q, got nil", tt.path)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error for path %q: %v", tt.path, err)
			}
			if !tt.expectErr && id != tt.expected {
				t.Errorf("Expected ID %d for path %q, got %d", tt.expected, tt.path, id)
			}
		})
	}
}

func TestComputeDiff_NoChanges(t *testing.T) {
	old := []string{"line1", "line2", "line3"}
	new := []string{"line1", "line2", "line3"}

	diff := computeDiff(old, new)

	if len(diff) != 3 {
		t.Errorf("expected 3 diff lines, got %d", len(diff))
	}

	for _, d := range diff {
		if d.Type != diffUnchanged {
			t.Errorf("expected all unchanged, got type %v", d.Type)
		}
	}
}

func TestComputeDiff_Addition(t *testing.T) {
	old := []string{"line1", "line3"}
	new := []string{"line1", "line2", "line3"}

	diff := computeDiff(old, new)

	addedCount := 0
	for _, d := range diff {
		if d.Type == diffAdded && d.Text == "line2" {
			addedCount++
		}
	}

	if addedCount != 1 {
		t.Errorf("expected 1 added line 'line2', got %d", addedCount)
	}
}

func TestComputeDiff_Removal(t *testing.T) {
	old := []string{"line1", "line2", "line3"}
	new := []string{"line1", "line3"}

	diff := computeDiff(old, new)

	removedCount := 0
	for _, d := range diff {
		if d.Type == diffRemoved && d.Text == "line2" {
			removedCount++
		}
	}

	if removedCount != 1 {
		t.Errorf("expected 1 removed line 'line2', got %d", removedCount)
	}
}

func TestComputeDiff_Replacement(t *testing.T) {
	old := []string{"line1", "old_content", "line3"}
	new := []string{"line1", "new_content", "line3"}

	diff := computeDiff(old, new)

	hasRemoved := false
	hasAdded := false
	for _, d := range diff {
		if d.Type == diffRemoved && d.Text == "old_content" {
			hasRemoved = true
		}
		if d.Type == diffAdded && d.Text == "new_content" {
			hasAdded = true
		}
	}

	if !hasRemoved {
		t.Error("expected 'old_content' to be removed")
	}
	if !hasAdded {
		t.Error("expected 'new_content' to be added")
	}
}

func TestComputeDiff_Empty(t *testing.T) {
	old := []string{}
	new := []string{"line1"}

	diff := computeDiff(old, new)

	if len(diff) != 1 {
		t.Errorf("expected 1 diff line, got %d", len(diff))
	}
	if diff[0].Type != diffAdded {
		t.Errorf("expected added line, got type %v", diff[0].Type)
	}
}

func TestGenerateDiff_HTMLEscaping(t *testing.T) {
	old := "test <script>alert('xss')</script>"
	new := "safe content"

	diff := generateDiff(old, new)

	if strings.Contains(diff, "<script>") {
		t.Error("HTML should be escaped in diff output")
	}
	if !strings.Contains(diff, "&lt;script&gt;") {
		t.Error("expected escaped HTML in diff output")
	}
}
