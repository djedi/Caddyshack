package store

import (
	"os"
	"testing"
	"time"
)

func TestAuditLog(t *testing.T) {
	// Create a temp database
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Create store
	store, err := New(tmpPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("CreateAuditEntry", func(t *testing.T) {
		// Note: UserID is nil because we don't have a real user in the test DB
		entry := &AuditEntry{
			Username:     "testuser",
			Action:       ActionSiteCreate,
			ResourceType: ResourceSite,
			ResourceID:   "example.com",
			Details:      "Created site with type: reverse_proxy",
			IPAddress:    "192.168.1.1",
		}

		err := store.CreateAuditEntry(entry)
		if err != nil {
			t.Fatalf("Failed to create audit entry: %v", err)
		}

		if entry.ID == 0 {
			t.Error("Expected entry ID to be set")
		}
	})

	t.Run("ListAuditEntries", func(t *testing.T) {
		// Create a few more entries
		for i := 0; i < 3; i++ {
			entry := &AuditEntry{
				Username:     "admin",
				Action:       ActionSiteUpdate,
				ResourceType: ResourceSite,
				ResourceID:   "test.com",
				Details:      "Updated site",
				IPAddress:    "127.0.0.1",
			}
			if err := store.CreateAuditEntry(entry); err != nil {
				t.Fatalf("Failed to create audit entry: %v", err)
			}
		}

		// List all entries
		entries, err := store.ListAuditEntries(AuditListOptions{})
		if err != nil {
			t.Fatalf("Failed to list audit entries: %v", err)
		}

		if len(entries) < 4 {
			t.Errorf("Expected at least 4 entries, got %d", len(entries))
		}
	})

	t.Run("ListAuditEntriesWithActionFilter", func(t *testing.T) {
		entries, err := store.ListAuditEntries(AuditListOptions{
			Action: string(ActionSiteCreate),
		})
		if err != nil {
			t.Fatalf("Failed to list audit entries: %v", err)
		}

		for _, e := range entries {
			if e.Action != ActionSiteCreate {
				t.Errorf("Expected action %s, got %s", ActionSiteCreate, e.Action)
			}
		}
	})

	t.Run("ListAuditEntriesWithResourceTypeFilter", func(t *testing.T) {
		entries, err := store.ListAuditEntries(AuditListOptions{
			ResourceType: string(ResourceSite),
		})
		if err != nil {
			t.Fatalf("Failed to list audit entries: %v", err)
		}

		for _, e := range entries {
			if e.ResourceType != ResourceSite {
				t.Errorf("Expected resource type %s, got %s", ResourceSite, e.ResourceType)
			}
		}
	})

	t.Run("ListAuditEntriesWithLimit", func(t *testing.T) {
		entries, err := store.ListAuditEntries(AuditListOptions{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("Failed to list audit entries: %v", err)
		}

		if len(entries) > 2 {
			t.Errorf("Expected at most 2 entries, got %d", len(entries))
		}
	})

	t.Run("ListAuditEntriesWithOffset", func(t *testing.T) {
		// Get first 2 entries
		firstPage, err := store.ListAuditEntries(AuditListOptions{Limit: 2})
		if err != nil {
			t.Fatalf("Failed to list first page: %v", err)
		}

		// Get next 2 entries
		secondPage, err := store.ListAuditEntries(AuditListOptions{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("Failed to list second page: %v", err)
		}

		// Verify they're different
		if len(firstPage) > 0 && len(secondPage) > 0 {
			if firstPage[0].ID == secondPage[0].ID {
				t.Error("Expected different entries on different pages")
			}
		}
	})

	t.Run("CountAuditEntries", func(t *testing.T) {
		count, err := store.CountAuditEntries(AuditListOptions{})
		if err != nil {
			t.Fatalf("Failed to count audit entries: %v", err)
		}

		if count < 4 {
			t.Errorf("Expected at least 4 entries, got %d", count)
		}
	})

	t.Run("CountAuditEntriesWithFilter", func(t *testing.T) {
		count, err := store.CountAuditEntries(AuditListOptions{
			Action: string(ActionSiteCreate),
		})
		if err != nil {
			t.Fatalf("Failed to count audit entries: %v", err)
		}

		if count != 1 {
			t.Errorf("Expected 1 entry with action %s, got %d", ActionSiteCreate, count)
		}
	})

	t.Run("GetAuditEntry", func(t *testing.T) {
		// Create an entry
		entry := &AuditEntry{
			Username:     "gettest",
			Action:       ActionUserCreate,
			ResourceType: ResourceUser,
			ResourceID:   "newuser",
			Details:      "Created user",
			IPAddress:    "10.0.0.1",
		}
		if err := store.CreateAuditEntry(entry); err != nil {
			t.Fatalf("Failed to create audit entry: %v", err)
		}

		// Retrieve it
		retrieved, err := store.GetAuditEntry(entry.ID)
		if err != nil {
			t.Fatalf("Failed to get audit entry: %v", err)
		}

		if retrieved.Username != "gettest" {
			t.Errorf("Expected username 'gettest', got '%s'", retrieved.Username)
		}
		if retrieved.Action != ActionUserCreate {
			t.Errorf("Expected action %s, got %s", ActionUserCreate, retrieved.Action)
		}
	})

	t.Run("GetDistinctActions", func(t *testing.T) {
		actions, err := store.GetDistinctActions()
		if err != nil {
			t.Fatalf("Failed to get distinct actions: %v", err)
		}

		if len(actions) < 2 {
			t.Errorf("Expected at least 2 distinct actions, got %d", len(actions))
		}
	})

	t.Run("GetDistinctUsers", func(t *testing.T) {
		users, err := store.GetDistinctUsers()
		if err != nil {
			t.Fatalf("Failed to get distinct users: %v", err)
		}

		if len(users) < 2 {
			t.Errorf("Expected at least 2 distinct users, got %d", len(users))
		}
	})

	t.Run("PruneAuditLog", func(t *testing.T) {
		// Count before pruning
		countBefore, err := store.CountAuditEntries(AuditListOptions{})
		if err != nil {
			t.Fatalf("Failed to count before prune: %v", err)
		}

		// Prune entries older than 1 year (should not delete anything)
		deleted, err := store.PruneAuditLog(365 * 24 * time.Hour)
		if err != nil {
			t.Fatalf("Failed to prune audit log: %v", err)
		}

		if deleted != 0 {
			t.Errorf("Expected 0 entries deleted, got %d", deleted)
		}

		// Count after pruning
		countAfter, err := store.CountAuditEntries(AuditListOptions{})
		if err != nil {
			t.Fatalf("Failed to count after prune: %v", err)
		}

		if countBefore != countAfter {
			t.Errorf("Expected count to remain %d, got %d", countBefore, countAfter)
		}
	})

	t.Run("DateRangeFilter", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)
		tomorrow := now.Add(24 * time.Hour)

		// Should include today's entries
		entries, err := store.ListAuditEntries(AuditListOptions{
			StartDate: &yesterday,
			EndDate:   &tomorrow,
		})
		if err != nil {
			t.Fatalf("Failed to list entries with date filter: %v", err)
		}

		if len(entries) == 0 {
			t.Error("Expected entries within date range")
		}

		// Should not include entries with future start date
		futureDate := now.Add(7 * 24 * time.Hour)
		entries, err = store.ListAuditEntries(AuditListOptions{
			StartDate: &futureDate,
		})
		if err != nil {
			t.Fatalf("Failed to list entries with future date filter: %v", err)
		}

		if len(entries) != 0 {
			t.Errorf("Expected 0 entries with future date filter, got %d", len(entries))
		}
	})
}

func TestAuditActions(t *testing.T) {
	// Test that all action constants are defined
	actions := []AuditAction{
		ActionSiteCreate,
		ActionSiteUpdate,
		ActionSiteDelete,
		ActionSnippetCreate,
		ActionSnippetUpdate,
		ActionSnippetDelete,
		ActionUserCreate,
		ActionUserUpdate,
		ActionUserDelete,
		ActionUserLogin,
		ActionUserLogout,
		ActionDomainCreate,
		ActionDomainUpdate,
		ActionDomainDelete,
		ActionConfigImport,
		ActionConfigExport,
		ActionConfigRestore,
		ActionConfigReload,
		ActionGlobalUpdate,
	}

	for _, action := range actions {
		if action == "" {
			t.Error("Action should not be empty string")
		}
	}
}

func TestAuditResourceTypes(t *testing.T) {
	// Test that all resource type constants are defined
	types := []AuditResourceType{
		ResourceSite,
		ResourceSnippet,
		ResourceUser,
		ResourceDomain,
		ResourceConfig,
		ResourceGlobal,
	}

	for _, rt := range types {
		if rt == "" {
			t.Error("Resource type should not be empty string")
		}
	}
}
