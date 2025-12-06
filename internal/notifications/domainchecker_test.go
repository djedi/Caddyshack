package notifications

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/djedi/caddyshack/internal/store"
)

// mockDomainStore is a mock implementation of DomainStore for testing.
type mockDomainStore struct {
	domains []store.Domain
	err     error
}

func (m *mockDomainStore) ListDomains() ([]store.Domain, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.domains, nil
}

// newDomainTestService creates a new notification service for testing.
func newDomainTestService(t *testing.T) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return NewService(s.DB())
}

func TestDomainChecker_NewDomainChecker(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{}
	checker := NewDomainChecker(svc, mockStore)

	if checker == nil {
		t.Fatal("NewDomainChecker() returned nil")
	}
	if checker.checkInterval != 24*time.Hour {
		t.Errorf("checkInterval = %v, want %v", checker.checkInterval, 24*time.Hour)
	}
	if checker.warningThreshold != 60 {
		t.Errorf("warningThreshold = %d, want 60", checker.warningThreshold)
	}
	if checker.criticalThreshold != 14 {
		t.Errorf("criticalThreshold = %d, want 14", checker.criticalThreshold)
	}
}

func TestDomainChecker_WithCheckInterval(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{}
	checker := NewDomainChecker(svc, mockStore).
		WithCheckInterval(1 * time.Hour)

	if checker.checkInterval != 1*time.Hour {
		t.Errorf("checkInterval = %v, want %v", checker.checkInterval, 1*time.Hour)
	}
}

func TestDomainChecker_WithThresholds(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{}
	checker := NewDomainChecker(svc, mockStore).
		WithThresholds(90, 30)

	if checker.warningThreshold != 90 {
		t.Errorf("warningThreshold = %d, want 90", checker.warningThreshold)
	}
	if checker.criticalThreshold != 30 {
		t.Errorf("criticalThreshold = %d, want 30", checker.criticalThreshold)
	}
}

func TestDomainChecker_StartStop(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{}
	checker := NewDomainChecker(svc, mockStore)

	// Start should not block
	checker.Start()

	// Should be able to call Start again (idempotent)
	checker.Start()

	// Stop should not block
	checker.Stop()

	// Should be able to call Stop again (idempotent)
	checker.Stop()
}

func TestDomainChecker_CheckAll_NoDomains(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{domains: []store.Domain{}}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// No notifications should be created
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d notifications, want 0", len(list))
	}
}

func TestDomainChecker_CheckAll_NoExpiryDate(t *testing.T) {
	svc := newDomainTestService(t)
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: nil},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// No notifications should be created for domains without expiry
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d notifications, want 0", len(list))
	}
}

func TestDomainChecker_CheckAll_ValidDomain(t *testing.T) {
	svc := newDomainTestService(t)
	futureDate := time.Now().AddDate(1, 0, 0) // 1 year from now
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &futureDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// No notifications should be created for domains far from expiry
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d notifications, want 0", len(list))
	}
}

func TestDomainChecker_CheckAll_WarningThreshold(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, 45) // 45 days from now (within 60 day warning)
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// Should create a warning notification
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d notifications, want 1", len(list))
	}
	if list[0].Severity != SeverityWarning {
		t.Errorf("Severity = %s, want %s", list[0].Severity, SeverityWarning)
	}
	if list[0].Type != TypeDomainExpiry {
		t.Errorf("Type = %s, want %s", list[0].Type, TypeDomainExpiry)
	}
}

func TestDomainChecker_CheckAll_CriticalThreshold(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, 7) // 7 days from now (within 14 day critical)
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// Should create a critical notification
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d notifications, want 1", len(list))
	}
	if list[0].Severity != SeverityCritical {
		t.Errorf("Severity = %s, want %s", list[0].Severity, SeverityCritical)
	}
}

func TestDomainChecker_CheckAll_Expired(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, -7) // 7 days ago (expired)
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// Should create an error notification
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d notifications, want 1", len(list))
	}
	if list[0].Severity != SeverityError {
		t.Errorf("Severity = %s, want %s", list[0].Severity, SeverityError)
	}
}

func TestDomainChecker_DuplicatePrevention(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, 45) // 45 days from now
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	// Run check twice
	checker.CheckAll()
	checker.CheckAll()

	// Should only have 1 notification (not duplicated)
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d notifications, want 1 (no duplicates)", len(list))
	}
}

func TestDomainChecker_AcknowledgedNotificationAllowsNew(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, 45) // 45 days from now
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	// First check creates notification
	checker.CheckAll()

	list, _ := svc.List(0, true)
	if len(list) != 1 {
		t.Fatalf("Expected 1 notification after first check, got %d", len(list))
	}

	// Acknowledge the notification
	if err := svc.Acknowledge(list[0].ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// Second check should create new notification since previous was acknowledged
	checker.CheckAll()

	// Should have 2 notifications now (1 acknowledged, 1 new)
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d notifications, want 2", len(list))
	}
}

func TestDomainChecker_MultipleDomains(t *testing.T) {
	svc := newDomainTestService(t)
	warningDate := time.Now().AddDate(0, 0, 45)   // Warning
	criticalDate := time.Now().AddDate(0, 0, 7)  // Critical
	expiredDate := time.Now().AddDate(0, 0, -1)  // Expired
	validDate := time.Now().AddDate(1, 0, 0)     // Valid

	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "warning.com", ExpiryDate: &warningDate},
			{ID: 2, Name: "critical.com", ExpiryDate: &criticalDate},
			{ID: 3, Name: "expired.com", ExpiryDate: &expiredDate},
			{ID: 4, Name: "valid.com", ExpiryDate: &validDate},
			{ID: 5, Name: "noexpiry.com", ExpiryDate: nil},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	checker.CheckAll()

	// Should have 3 notifications (warning, critical, expired)
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List() returned %d notifications, want 3", len(list))
	}

	// Verify severities
	severities := make(map[Severity]int)
	for _, n := range list {
		severities[n.Severity]++
	}
	if severities[SeverityWarning] != 1 {
		t.Errorf("Warning notifications = %d, want 1", severities[SeverityWarning])
	}
	if severities[SeverityCritical] != 1 {
		t.Errorf("Critical notifications = %d, want 1", severities[SeverityCritical])
	}
	if severities[SeverityError] != 1 {
		t.Errorf("Error notifications = %d, want 1", severities[SeverityError])
	}
}

func TestDomainExpiryData_JSON(t *testing.T) {
	data := DomainExpiryData{
		DomainID:   123,
		DomainName: "example.com",
		Threshold:  "60",
		ExpiresAt:  "2025-06-15T00:00:00Z",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded DomainExpiryData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.DomainID != data.DomainID {
		t.Errorf("DomainID = %v, want %v", decoded.DomainID, data.DomainID)
	}
	if decoded.DomainName != data.DomainName {
		t.Errorf("DomainName = %v, want %v", decoded.DomainName, data.DomainName)
	}
	if decoded.Threshold != data.Threshold {
		t.Errorf("Threshold = %v, want %v", decoded.Threshold, data.Threshold)
	}
	if decoded.ExpiresAt != data.ExpiresAt {
		t.Errorf("ExpiresAt = %v, want %v", decoded.ExpiresAt, data.ExpiresAt)
	}
}

func TestDomainChecker_CheckNow(t *testing.T) {
	svc := newDomainTestService(t)
	expiryDate := time.Now().AddDate(0, 0, 10) // 10 days from now
	mockStore := &mockDomainStore{
		domains: []store.Domain{
			{ID: 1, Name: "example.com", ExpiryDate: &expiryDate},
		},
	}
	checker := NewDomainChecker(svc, mockStore)

	// CheckNow should work without starting the checker
	checker.CheckNow()

	// Should have created a notification
	list, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d notifications, want 1", len(list))
	}
}
