package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	// Verify migrations ran
	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() error = %v", err)
	}
	if version != 5 {
		t.Errorf("SchemaVersion() = %d, want 5", version)
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Try to create database in non-existent directory
	_, err := New("/nonexistent/path/test.db")
	if err == nil {
		t.Error("New() expected error for invalid path")
	}
}

func TestStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStore_DB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	db := s.DB()
	if db == nil {
		t.Error("DB() returned nil")
	}

	// Verify we can execute queries
	if err := db.Ping(); err != nil {
		t.Errorf("DB().Ping() error = %v", err)
	}
}

func TestStore_MigrationsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store first time
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() first call error = %v", err)
	}
	s1.Close()

	// Create store second time - migrations should be idempotent
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() second call error = %v", err)
	}
	defer s2.Close()

	version, err := s2.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() error = %v", err)
	}
	if version != 5 {
		t.Errorf("SchemaVersion() = %d, want 5", version)
	}
}

// newTestStore creates a new store for testing with a temporary database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return s
}
