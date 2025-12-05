package caddy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReader_Read(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	t.Run("reads existing file", func(t *testing.T) {
		// Create a test Caddyfile
		testContent := `{
	email test@example.com
}

example.com {
	reverse_proxy localhost:8080
}
`
		testFile := filepath.Join(tmpDir, "Caddyfile")
		if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		reader := NewReader(testFile)
		content, err := reader.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if content != testContent {
			t.Errorf("content mismatch:\ngot:  %q\nwant: %q", content, testContent)
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		reader := NewReader(filepath.Join(tmpDir, "nonexistent"))
		_, err := reader.Read()

		if !errors.Is(err, ErrCaddyfileNotFound) {
			t.Errorf("expected ErrCaddyfileNotFound, got: %v", err)
		}
	})

	t.Run("reads empty file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "empty")
		if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		reader := NewReader(testFile)
		content, err := reader.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if content != "" {
			t.Errorf("expected empty content, got: %q", content)
		}
	})
}

func TestReader_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("returns true for existing file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "exists")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		reader := NewReader(testFile)
		if !reader.Exists() {
			t.Error("expected Exists() to return true")
		}
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		reader := NewReader(filepath.Join(tmpDir, "nonexistent"))
		if reader.Exists() {
			t.Error("expected Exists() to return false")
		}
	})
}

func TestReader_Path(t *testing.T) {
	path := "/etc/caddy/Caddyfile"
	reader := NewReader(path)

	if reader.Path() != path {
		t.Errorf("expected path %q, got %q", path, reader.Path())
	}
}
