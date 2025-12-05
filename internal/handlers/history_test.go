package handlers

import (
	"strings"
	"testing"
)

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
