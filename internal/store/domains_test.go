package store

import (
	"testing"
	"time"
)

func TestStore_CreateDomain(t *testing.T) {
	s := newTestStore(t)

	expiryDate := time.Now().Add(365 * 24 * time.Hour)
	domain := &Domain{
		Name:       "example.com",
		Registrar:  "GoDaddy",
		ExpiryDate: &expiryDate,
		Notes:      "Test domain",
		AutoAdded:  false,
	}

	err := s.CreateDomain(domain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	if domain.ID == 0 {
		t.Error("CreateDomain() did not set ID")
	}
}

func TestStore_GetDomain(t *testing.T) {
	s := newTestStore(t)

	expiryDate := time.Now().Add(365 * 24 * time.Hour)
	domain := &Domain{
		Name:       "example.com",
		Registrar:  "GoDaddy",
		ExpiryDate: &expiryDate,
		Notes:      "Test domain",
		AutoAdded:  false,
	}

	err := s.CreateDomain(domain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	// Test GetDomain
	retrieved, err := s.GetDomain(domain.ID)
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetDomain() returned nil")
	}
	if retrieved.Name != domain.Name {
		t.Errorf("GetDomain().Name = %s, want %s", retrieved.Name, domain.Name)
	}
	if retrieved.Registrar != domain.Registrar {
		t.Errorf("GetDomain().Registrar = %s, want %s", retrieved.Registrar, domain.Registrar)
	}

	// Test GetDomain for non-existent
	nonExistent, err := s.GetDomain(99999)
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if nonExistent != nil {
		t.Error("GetDomain() expected nil for non-existent domain")
	}
}

func TestStore_GetDomainByName(t *testing.T) {
	s := newTestStore(t)

	domain := &Domain{
		Name:      "example.com",
		Registrar: "GoDaddy",
		AutoAdded: false,
	}

	err := s.CreateDomain(domain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	// Test GetDomainByName
	retrieved, err := s.GetDomainByName("example.com")
	if err != nil {
		t.Fatalf("GetDomainByName() error = %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetDomainByName() returned nil")
	}
	if retrieved.ID != domain.ID {
		t.Errorf("GetDomainByName().ID = %d, want %d", retrieved.ID, domain.ID)
	}

	// Test GetDomainByName for non-existent
	nonExistent, err := s.GetDomainByName("nonexistent.com")
	if err != nil {
		t.Fatalf("GetDomainByName() error = %v", err)
	}
	if nonExistent != nil {
		t.Error("GetDomainByName() expected nil for non-existent domain")
	}
}

func TestStore_ListDomains(t *testing.T) {
	s := newTestStore(t)

	// Create some domains
	domains := []string{"alpha.com", "beta.com", "gamma.com"}
	for _, name := range domains {
		err := s.CreateDomain(&Domain{Name: name})
		if err != nil {
			t.Fatalf("CreateDomain() error = %v", err)
		}
	}

	// List domains
	list, err := s.ListDomains()
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListDomains() returned %d domains, want 3", len(list))
	}

	// Check alphabetical order
	if list[0].Name != "alpha.com" || list[1].Name != "beta.com" || list[2].Name != "gamma.com" {
		t.Error("ListDomains() did not return domains in alphabetical order")
	}
}

func TestStore_UpdateDomain(t *testing.T) {
	s := newTestStore(t)

	domain := &Domain{
		Name:      "example.com",
		Registrar: "GoDaddy",
		Notes:     "Original notes",
	}

	err := s.CreateDomain(domain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	// Update the domain
	domain.Registrar = "Namecheap"
	domain.Notes = "Updated notes"
	expiryDate := time.Now().Add(365 * 24 * time.Hour)
	domain.ExpiryDate = &expiryDate

	err = s.UpdateDomain(domain)
	if err != nil {
		t.Fatalf("UpdateDomain() error = %v", err)
	}

	// Verify the update
	retrieved, err := s.GetDomain(domain.ID)
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if retrieved.Registrar != "Namecheap" {
		t.Errorf("Updated Registrar = %s, want Namecheap", retrieved.Registrar)
	}
	if retrieved.Notes != "Updated notes" {
		t.Errorf("Updated Notes = %s, want 'Updated notes'", retrieved.Notes)
	}
	if retrieved.ExpiryDate == nil {
		t.Error("Updated ExpiryDate is nil")
	}
}

func TestStore_DeleteDomain(t *testing.T) {
	s := newTestStore(t)

	domain := &Domain{Name: "example.com"}
	err := s.CreateDomain(domain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	err = s.DeleteDomain(domain.ID)
	if err != nil {
		t.Fatalf("DeleteDomain() error = %v", err)
	}

	// Verify deletion
	retrieved, err := s.GetDomain(domain.ID)
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if retrieved != nil {
		t.Error("Domain was not deleted")
	}

	// Delete non-existent should error
	err = s.DeleteDomain(99999)
	if err == nil {
		t.Error("DeleteDomain() expected error for non-existent domain")
	}
}

func TestStore_SyncAutoAddedDomains(t *testing.T) {
	s := newTestStore(t)

	// Initial sync with some domains
	initialDomains := []string{"example.com", "test.com", "demo.com"}
	err := s.SyncAutoAddedDomains(initialDomains)
	if err != nil {
		t.Fatalf("SyncAutoAddedDomains() error = %v", err)
	}

	// Verify domains were created
	list, err := s.ListDomains()
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("After initial sync, got %d domains, want 3", len(list))
	}

	// All should be auto-added
	for _, d := range list {
		if !d.AutoAdded {
			t.Errorf("Domain %s should be auto-added", d.Name)
		}
	}

	// Sync with different list - should add new and remove stale
	newDomains := []string{"example.com", "new.com"} // removed test.com and demo.com, added new.com
	err = s.SyncAutoAddedDomains(newDomains)
	if err != nil {
		t.Fatalf("SyncAutoAddedDomains() error = %v", err)
	}

	list, err = s.ListDomains()
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("After second sync, got %d domains, want 2", len(list))
	}

	// Verify the correct domains exist
	domainNames := make(map[string]bool)
	for _, d := range list {
		domainNames[d.Name] = true
	}
	if !domainNames["example.com"] {
		t.Error("example.com should still exist")
	}
	if !domainNames["new.com"] {
		t.Error("new.com should be added")
	}
	if domainNames["test.com"] {
		t.Error("test.com should be removed")
	}
}

func TestStore_SyncAutoAddedDomains_PreservesManualDomains(t *testing.T) {
	s := newTestStore(t)

	// Create a manual domain first
	manualDomain := &Domain{
		Name:      "manual.com",
		Registrar: "GoDaddy",
		AutoAdded: false,
	}
	err := s.CreateDomain(manualDomain)
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	// Sync with domains that includes the manual domain name
	domainNames := []string{"manual.com", "auto.com"}
	err = s.SyncAutoAddedDomains(domainNames)
	if err != nil {
		t.Fatalf("SyncAutoAddedDomains() error = %v", err)
	}

	// Verify manual domain is preserved and not duplicated
	list, err := s.ListDomains()
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("Got %d domains, want 2", len(list))
	}

	// Find manual.com and verify it's still manual
	for _, d := range list {
		if d.Name == "manual.com" {
			if d.AutoAdded {
				t.Error("Manual domain should not be marked as auto-added")
			}
			if d.Registrar != "GoDaddy" {
				t.Error("Manual domain registrar should be preserved")
			}
		}
	}
}

func TestStore_ListAutoAddedDomains(t *testing.T) {
	s := newTestStore(t)

	// Create mixed domains
	err := s.CreateDomain(&Domain{Name: "auto1.com", AutoAdded: true})
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}
	err = s.CreateDomain(&Domain{Name: "manual.com", AutoAdded: false})
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}
	err = s.CreateDomain(&Domain{Name: "auto2.com", AutoAdded: true})
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	// List only auto-added
	list, err := s.ListAutoAddedDomains()
	if err != nil {
		t.Fatalf("ListAutoAddedDomains() error = %v", err)
	}

	if len(list) != 2 {
		t.Errorf("ListAutoAddedDomains() returned %d domains, want 2", len(list))
	}

	for _, d := range list {
		if !d.AutoAdded {
			t.Errorf("ListAutoAddedDomains() returned non-auto-added domain: %s", d.Name)
		}
	}
}
