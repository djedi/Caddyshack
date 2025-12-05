package store

import (
	"testing"
)

func TestStore_SaveConfig(t *testing.T) {
	s := newTestStore(t)

	id, err := s.SaveConfig("test content", "test comment")
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if id <= 0 {
		t.Errorf("SaveConfig() returned id = %d, want > 0", id)
	}
}

func TestStore_GetConfig(t *testing.T) {
	s := newTestStore(t)

	content := "example.com {\n  reverse_proxy localhost:8080\n}"
	comment := "Added example.com site"

	id, err := s.SaveConfig(content, comment)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	ch, err := s.GetConfig(id)
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if ch.ID != id {
		t.Errorf("GetConfig().ID = %d, want %d", ch.ID, id)
	}
	if ch.Content != content {
		t.Errorf("GetConfig().Content = %q, want %q", ch.Content, content)
	}
	if ch.Comment != comment {
		t.Errorf("GetConfig().Comment = %q, want %q", ch.Comment, comment)
	}
	if ch.Timestamp.IsZero() {
		t.Error("GetConfig().Timestamp is zero")
	}
}

func TestStore_GetConfig_NotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetConfig(999)
	if err == nil {
		t.Error("GetConfig() expected error for non-existent id")
	}
}

func TestStore_ListConfigs(t *testing.T) {
	s := newTestStore(t)

	// Insert multiple configs
	for i := 0; i < 5; i++ {
		_, err := s.SaveConfig("content", "comment")
		if err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
	}

	configs, err := s.ListConfigs(0)
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 5 {
		t.Errorf("ListConfigs() returned %d configs, want 5", len(configs))
	}
}

func TestStore_ListConfigs_WithLimit(t *testing.T) {
	s := newTestStore(t)

	// Insert multiple configs
	for i := 0; i < 5; i++ {
		_, err := s.SaveConfig("content", "comment")
		if err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
	}

	configs, err := s.ListConfigs(3)
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 3 {
		t.Errorf("ListConfigs(3) returned %d configs, want 3", len(configs))
	}
}

func TestStore_ListConfigs_Empty(t *testing.T) {
	s := newTestStore(t)

	configs, err := s.ListConfigs(0)
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("ListConfigs() returned %d configs, want 0", len(configs))
	}
}

func TestStore_LatestConfig(t *testing.T) {
	s := newTestStore(t)

	// Insert configs
	s.SaveConfig("first", "first comment")
	s.SaveConfig("second", "second comment")
	lastID, _ := s.SaveConfig("third", "third comment")

	ch, err := s.LatestConfig()
	if err != nil {
		t.Fatalf("LatestConfig() error = %v", err)
	}
	if ch == nil {
		t.Fatal("LatestConfig() returned nil")
	}
	if ch.ID != lastID {
		t.Errorf("LatestConfig().ID = %d, want %d", ch.ID, lastID)
	}
	if ch.Content != "third" {
		t.Errorf("LatestConfig().Content = %q, want %q", ch.Content, "third")
	}
}

func TestStore_LatestConfig_Empty(t *testing.T) {
	s := newTestStore(t)

	ch, err := s.LatestConfig()
	if err != nil {
		t.Fatalf("LatestConfig() error = %v", err)
	}
	if ch != nil {
		t.Error("LatestConfig() expected nil for empty table")
	}
}

func TestStore_PruneHistory(t *testing.T) {
	s := newTestStore(t)

	// Insert 10 configs
	for i := 0; i < 10; i++ {
		_, err := s.SaveConfig("content", "comment")
		if err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
	}

	// Prune to keep only 3
	deleted, err := s.PruneHistory(3)
	if err != nil {
		t.Fatalf("PruneHistory() error = %v", err)
	}
	if deleted != 7 {
		t.Errorf("PruneHistory() deleted = %d, want 7", deleted)
	}

	// Verify only 3 remain
	count, err := s.ConfigCount()
	if err != nil {
		t.Fatalf("ConfigCount() error = %v", err)
	}
	if count != 3 {
		t.Errorf("ConfigCount() = %d, want 3", count)
	}
}

func TestStore_ConfigCount(t *testing.T) {
	s := newTestStore(t)

	count, err := s.ConfigCount()
	if err != nil {
		t.Fatalf("ConfigCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("ConfigCount() = %d, want 0", count)
	}

	s.SaveConfig("content", "comment")
	s.SaveConfig("content", "comment")

	count, err = s.ConfigCount()
	if err != nil {
		t.Fatalf("ConfigCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("ConfigCount() = %d, want 2", count)
	}
}
