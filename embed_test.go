package caddyshack

import (
	"io/fs"
	"testing"
)

func TestStaticFS(t *testing.T) {
	staticFS := StaticFS()

	// Check that we can read the css directory
	entries, err := fs.ReadDir(staticFS, "css")
	if err != nil {
		t.Fatalf("failed to read css directory: %v", err)
	}

	// Should contain at least input.css and output.css
	found := make(map[string]bool)
	for _, entry := range entries {
		found[entry.Name()] = true
	}

	if !found["input.css"] {
		t.Error("expected to find input.css in css directory")
	}

	if !found["output.css"] {
		t.Error("expected to find output.css in css directory")
	}
}

func TestStaticFS_ReadFile(t *testing.T) {
	staticFS := StaticFS()

	// Read input.css
	content, err := fs.ReadFile(staticFS, "css/input.css")
	if err != nil {
		t.Fatalf("failed to read css/input.css: %v", err)
	}

	if len(content) == 0 {
		t.Error("css/input.css should not be empty")
	}

	// Check it contains Tailwind directives
	if !contains(string(content), "@tailwind") {
		t.Error("css/input.css should contain @tailwind directive")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
