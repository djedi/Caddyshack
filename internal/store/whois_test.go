package store

import (
	"os"
	"testing"
	"time"
)

func TestWHOISCache(t *testing.T) {
	// Create a temporary database
	tmpFile, err := os.CreateTemp("", "caddyshack-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create store
	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// First, create a domain to associate WHOIS cache with
	domain := &Domain{
		Name:      "example.com",
		Registrar: "",
		AutoAdded: false,
	}
	if err := store.CreateDomain(domain); err != nil {
		t.Fatalf("Failed to create domain: %v", err)
	}

	t.Run("GetWHOISCache returns nil for non-existent cache", func(t *testing.T) {
		cache, err := store.GetWHOISCache(domain.ID)
		if err != nil {
			t.Fatalf("GetWHOISCache returned error: %v", err)
		}
		if cache != nil {
			t.Error("Expected nil cache for non-existent entry")
		}
	})

	t.Run("SaveWHOISCache creates new entry", func(t *testing.T) {
		expiry := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		created := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		lookupTime := time.Now()

		cache := &WHOISCache{
			DomainID:    domain.ID,
			Registrar:   "Test Registrar Inc.",
			ExpiryDate:  &expiry,
			CreatedDate: &created,
			NameServers: []string{"ns1.example.com", "ns2.example.com"},
			Status:      []string{"clientTransferProhibited", "serverDeleteProhibited"},
			RawData:     "Domain Name: EXAMPLE.COM\nRegistrar: Test Registrar Inc.",
			LookupTime:  lookupTime,
		}

		if err := store.SaveWHOISCache(cache); err != nil {
			t.Fatalf("SaveWHOISCache returned error: %v", err)
		}

		// Verify it was saved
		retrieved, err := store.GetWHOISCache(domain.ID)
		if err != nil {
			t.Fatalf("GetWHOISCache returned error: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected to retrieve cache entry")
		}

		if retrieved.Registrar != "Test Registrar Inc." {
			t.Errorf("Registrar = %q, want %q", retrieved.Registrar, "Test Registrar Inc.")
		}
		if retrieved.ExpiryDate == nil || !retrieved.ExpiryDate.Equal(expiry) {
			t.Errorf("ExpiryDate = %v, want %v", retrieved.ExpiryDate, expiry)
		}
		if len(retrieved.NameServers) != 2 {
			t.Errorf("NameServers length = %d, want 2", len(retrieved.NameServers))
		}
		if len(retrieved.Status) != 2 {
			t.Errorf("Status length = %d, want 2", len(retrieved.Status))
		}
	})

	t.Run("SaveWHOISCache updates existing entry", func(t *testing.T) {
		newExpiry := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

		cache := &WHOISCache{
			DomainID:    domain.ID,
			Registrar:   "Updated Registrar",
			ExpiryDate:  &newExpiry,
			NameServers: []string{"ns1.new.com"},
			Status:      []string{"active"},
			LookupTime:  time.Now(),
		}

		if err := store.SaveWHOISCache(cache); err != nil {
			t.Fatalf("SaveWHOISCache returned error: %v", err)
		}

		retrieved, err := store.GetWHOISCache(domain.ID)
		if err != nil {
			t.Fatalf("GetWHOISCache returned error: %v", err)
		}

		if retrieved.Registrar != "Updated Registrar" {
			t.Errorf("Registrar = %q, want %q", retrieved.Registrar, "Updated Registrar")
		}
		if !retrieved.ExpiryDate.Equal(newExpiry) {
			t.Errorf("ExpiryDate = %v, want %v", retrieved.ExpiryDate, newExpiry)
		}
		if len(retrieved.NameServers) != 1 {
			t.Errorf("NameServers length = %d, want 1", len(retrieved.NameServers))
		}
	})

	t.Run("IsWHOISCacheStale returns true for old cache", func(t *testing.T) {
		// First update cache with old lookup time
		oldTime := time.Now().Add(-48 * time.Hour)
		cache := &WHOISCache{
			DomainID:   domain.ID,
			Registrar:  "Test",
			LookupTime: oldTime,
		}
		if err := store.SaveWHOISCache(cache); err != nil {
			t.Fatalf("SaveWHOISCache returned error: %v", err)
		}

		stale, err := store.IsWHOISCacheStale(domain.ID, 24*time.Hour)
		if err != nil {
			t.Fatalf("IsWHOISCacheStale returned error: %v", err)
		}
		if !stale {
			t.Error("Expected cache to be stale")
		}
	})

	t.Run("IsWHOISCacheStale returns false for fresh cache", func(t *testing.T) {
		// Update cache with recent lookup time
		cache := &WHOISCache{
			DomainID:   domain.ID,
			Registrar:  "Test",
			LookupTime: time.Now(),
		}
		if err := store.SaveWHOISCache(cache); err != nil {
			t.Fatalf("SaveWHOISCache returned error: %v", err)
		}

		stale, err := store.IsWHOISCacheStale(domain.ID, 24*time.Hour)
		if err != nil {
			t.Fatalf("IsWHOISCacheStale returned error: %v", err)
		}
		if stale {
			t.Error("Expected cache to not be stale")
		}
	})

	t.Run("IsWHOISCacheStale returns true for non-existent cache", func(t *testing.T) {
		stale, err := store.IsWHOISCacheStale(9999, 24*time.Hour)
		if err != nil {
			t.Fatalf("IsWHOISCacheStale returned error: %v", err)
		}
		if !stale {
			t.Error("Expected non-existent cache to be considered stale")
		}
	})

	t.Run("DeleteWHOISCache removes entry", func(t *testing.T) {
		if err := store.DeleteWHOISCache(domain.ID); err != nil {
			t.Fatalf("DeleteWHOISCache returned error: %v", err)
		}

		cache, err := store.GetWHOISCache(domain.ID)
		if err != nil {
			t.Fatalf("GetWHOISCache returned error: %v", err)
		}
		if cache != nil {
			t.Error("Expected nil cache after deletion")
		}
	})

	t.Run("WHOIS cache is deleted when domain is deleted", func(t *testing.T) {
		// Create a new domain
		newDomain := &Domain{
			Name:      "test-delete.com",
			AutoAdded: false,
		}
		if err := store.CreateDomain(newDomain); err != nil {
			t.Fatalf("Failed to create domain: %v", err)
		}

		// Add WHOIS cache
		cache := &WHOISCache{
			DomainID:   newDomain.ID,
			Registrar:  "Test",
			LookupTime: time.Now(),
		}
		if err := store.SaveWHOISCache(cache); err != nil {
			t.Fatalf("SaveWHOISCache returned error: %v", err)
		}

		// Delete domain
		if err := store.DeleteDomain(newDomain.ID); err != nil {
			t.Fatalf("DeleteDomain returned error: %v", err)
		}

		// Verify WHOIS cache is also deleted (due to foreign key cascade)
		retrieved, err := store.GetWHOISCache(newDomain.ID)
		if err != nil {
			t.Fatalf("GetWHOISCache returned error: %v", err)
		}
		if retrieved != nil {
			t.Error("Expected WHOIS cache to be deleted when domain is deleted")
		}
	})
}

func TestWHOISCacheWithNilDates(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "caddyshack-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	domain := &Domain{
		Name:      "nodates.com",
		AutoAdded: false,
	}
	if err := store.CreateDomain(domain); err != nil {
		t.Fatalf("Failed to create domain: %v", err)
	}

	// Save cache with nil dates
	cache := &WHOISCache{
		DomainID:    domain.ID,
		Registrar:   "Test Registrar",
		ExpiryDate:  nil,
		CreatedDate: nil,
		UpdatedDate: nil,
		NameServers: []string{},
		Status:      []string{},
		LookupTime:  time.Now(),
	}

	if err := store.SaveWHOISCache(cache); err != nil {
		t.Fatalf("SaveWHOISCache returned error: %v", err)
	}

	retrieved, err := store.GetWHOISCache(domain.ID)
	if err != nil {
		t.Fatalf("GetWHOISCache returned error: %v", err)
	}

	if retrieved.ExpiryDate != nil {
		t.Error("Expected nil ExpiryDate")
	}
	if retrieved.CreatedDate != nil {
		t.Error("Expected nil CreatedDate")
	}
	if retrieved.UpdatedDate != nil {
		t.Error("Expected nil UpdatedDate")
	}
}
