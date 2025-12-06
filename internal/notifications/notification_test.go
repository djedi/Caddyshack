package notifications

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/djedi/caddyshack/internal/store"
)

// newTestService creates a new notification service for testing with a temporary database.
func newTestService(t *testing.T) *Service {
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

func TestService_Create(t *testing.T) {
	svc := newTestService(t)

	n, err := svc.Create(TypeCertExpiry, SeverityWarning, "Test Title", "Test Message", `{"domain":"example.com"}`)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if n.ID == 0 {
		t.Error("Create() notification ID should not be 0")
	}
	if n.Type != TypeCertExpiry {
		t.Errorf("Create() type = %v, want %v", n.Type, TypeCertExpiry)
	}
	if n.Severity != SeverityWarning {
		t.Errorf("Create() severity = %v, want %v", n.Severity, SeverityWarning)
	}
	if n.Title != "Test Title" {
		t.Errorf("Create() title = %v, want %v", n.Title, "Test Title")
	}
	if n.Message != "Test Message" {
		t.Errorf("Create() message = %v, want %v", n.Message, "Test Message")
	}
	if n.Data != `{"domain":"example.com"}` {
		t.Errorf("Create() data = %v, want %v", n.Data, `{"domain":"example.com"}`)
	}
	if n.CreatedAt.IsZero() {
		t.Error("Create() created_at should not be zero")
	}
	if n.AcknowledgedAt != nil {
		t.Error("Create() acknowledged_at should be nil")
	}
}

func TestService_GetByID(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("GetByID() ID = %v, want %v", got.ID, created.ID)
	}
	if got.Title != created.Title {
		t.Errorf("GetByID() Title = %v, want %v", got.Title, created.Title)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.GetByID(99999)
	if err == nil {
		t.Error("GetByID() expected error for non-existent ID")
	}
}

func TestService_List(t *testing.T) {
	svc := newTestService(t)

	// Create a few notifications
	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message 1", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeSystem, SeverityInfo, "System 1", "Message 2", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	n3, err := svc.Create(TypeCaddyReload, SeverityError, "Reload 1", "Message 3", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Acknowledge one
	if err := svc.Acknowledge(n3.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// List all (excluding acknowledged)
	list, err := svc.List(0, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d notifications, want 2", len(list))
	}

	// List all (including acknowledged)
	listAll, err := svc.List(0, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listAll) != 3 {
		t.Errorf("List() returned %d notifications, want 3", len(listAll))
	}
}

func TestService_List_Limit(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := svc.List(3, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List() returned %d notifications, want 3", len(list))
	}
}

func TestService_ListByType(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeCertExpiry, SeverityCritical, "Cert 2", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeSystem, SeverityInfo, "System", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.ListByType(TypeCertExpiry, 0, false)
	if err != nil {
		t.Fatalf("ListByType() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListByType() returned %d notifications, want 2", len(list))
	}
}

func TestService_ListBySeverity(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeSystem, SeverityWarning, "System", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeCaddyReload, SeverityCritical, "Reload", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.ListBySeverity(SeverityWarning, 0, false)
	if err != nil {
		t.Fatalf("ListBySeverity() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListBySeverity() returned %d notifications, want 2", len(list))
	}
}

func TestService_Acknowledge(t *testing.T) {
	svc := newTestService(t)

	n, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Acknowledge(n.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// Verify acknowledged
	got, err := svc.GetByID(n.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.AcknowledgedAt == nil {
		t.Error("Acknowledge() should set acknowledged_at")
	}
	if !got.IsAcknowledged() {
		t.Error("IsAcknowledged() should return true")
	}
}

func TestService_Acknowledge_AlreadyAcknowledged(t *testing.T) {
	svc := newTestService(t)

	n, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Acknowledge once
	if err := svc.Acknowledge(n.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// Acknowledge again should fail
	err = svc.Acknowledge(n.ID)
	if err == nil {
		t.Error("Acknowledge() should return error for already acknowledged notification")
	}
}

func TestService_AcknowledgeAll(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < 3; i++ {
		_, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err := svc.AcknowledgeAll()
	if err != nil {
		t.Fatalf("AcknowledgeAll() error = %v", err)
	}
	if count != 3 {
		t.Errorf("AcknowledgeAll() = %d, want 3", count)
	}

	// Verify all acknowledged
	unread, err := svc.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount() error = %v", err)
	}
	if unread != 0 {
		t.Errorf("UnreadCount() = %d, want 0", unread)
	}
}

func TestService_Delete(t *testing.T) {
	svc := newTestService(t)

	n, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Delete(n.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = svc.GetByID(n.ID)
	if err == nil {
		t.Error("Delete() should remove the notification")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := newTestService(t)

	err := svc.Delete(99999)
	if err == nil {
		t.Error("Delete() should return error for non-existent ID")
	}
}

func TestService_DeleteOlderThan(t *testing.T) {
	svc := newTestService(t)

	// Create notifications
	n1, err := svc.Create(TypeSystem, SeverityInfo, "Old", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	n2, err := svc.Create(TypeSystem, SeverityInfo, "New", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Acknowledge both
	if err := svc.Acknowledge(n1.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}
	if err := svc.Acknowledge(n2.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// Wait a bit then delete notifications older than a short time
	// SQLite's CURRENT_TIMESTAMP has second precision, so we need to wait long enough
	time.Sleep(1500 * time.Millisecond)

	// DeleteOlderThan(500ms) means: delete if created_at < now - 500ms
	// Since notifications were created > 1.5 seconds ago, they should be deleted
	count, err := svc.DeleteOlderThan(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("DeleteOlderThan() error = %v", err)
	}
	if count != 2 {
		t.Errorf("DeleteOlderThan() = %d, want 2", count)
	}
}

func TestService_DeleteOlderThan_KeepsUnacknowledged(t *testing.T) {
	svc := newTestService(t)

	// Create an unacknowledged notification
	_, err := svc.Create(TypeSystem, SeverityInfo, "Unack", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// DeleteOlderThan should not delete unacknowledged
	count, err := svc.DeleteOlderThan(0)
	if err != nil {
		t.Fatalf("DeleteOlderThan() error = %v", err)
	}
	if count != 0 {
		t.Errorf("DeleteOlderThan() = %d, want 0 (should keep unacknowledged)", count)
	}
}

func TestService_UnreadCount(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(TypeSystem, SeverityInfo, "Test", "Message", "")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	count, err := svc.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount() error = %v", err)
	}
	if count != 5 {
		t.Errorf("UnreadCount() = %d, want 5", count)
	}
}

func TestService_UnreadCountBySeverity(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeCertExpiry, SeverityCritical, "Cert 2", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Create(TypeSystem, SeverityCritical, "System", "Message", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	count, err := svc.UnreadCountBySeverity(SeverityCritical)
	if err != nil {
		t.Fatalf("UnreadCountBySeverity() error = %v", err)
	}
	if count != 2 {
		t.Errorf("UnreadCountBySeverity() = %d, want 2", count)
	}
}

func TestService_ExistsUnacknowledged(t *testing.T) {
	svc := newTestService(t)

	// Create a notification
	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message", `{"domain":"example.com","threshold":"30d"}`)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Check if exists
	exists, err := svc.ExistsUnacknowledged(TypeCertExpiry, `{"domain":"example.com","threshold":"30d"}`)
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if !exists {
		t.Error("ExistsUnacknowledged() should return true for existing notification")
	}

	// Check for different data
	exists, err = svc.ExistsUnacknowledged(TypeCertExpiry, `{"domain":"other.com"}`)
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if exists {
		t.Error("ExistsUnacknowledged() should return false for different data")
	}
}

func TestService_ExistsUnacknowledged_AcknowledgedDoesNotMatch(t *testing.T) {
	svc := newTestService(t)

	// Create and acknowledge a notification
	n, err := svc.Create(TypeCertExpiry, SeverityWarning, "Cert 1", "Message", `{"domain":"example.com"}`)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Acknowledge(n.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// Check if exists (should be false since it's acknowledged)
	exists, err := svc.ExistsUnacknowledged(TypeCertExpiry, `{"domain":"example.com"}`)
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if exists {
		t.Error("ExistsUnacknowledged() should return false for acknowledged notification")
	}
}

func TestNotification_IsAcknowledged(t *testing.T) {
	// Test with nil AcknowledgedAt
	n := &Notification{AcknowledgedAt: nil}
	if n.IsAcknowledged() {
		t.Error("IsAcknowledged() should return false when AcknowledgedAt is nil")
	}

	// Test with non-nil AcknowledgedAt
	now := time.Now()
	n.AcknowledgedAt = &now
	if !n.IsAcknowledged() {
		t.Error("IsAcknowledged() should return true when AcknowledgedAt is set")
	}
}

func TestSeverityAndTypeConstants(t *testing.T) {
	// Test that constants have expected values
	if SeverityInfo != "info" {
		t.Errorf("SeverityInfo = %v, want info", SeverityInfo)
	}
	if SeverityWarning != "warning" {
		t.Errorf("SeverityWarning = %v, want warning", SeverityWarning)
	}
	if SeverityCritical != "critical" {
		t.Errorf("SeverityCritical = %v, want critical", SeverityCritical)
	}
	if SeverityError != "error" {
		t.Errorf("SeverityError = %v, want error", SeverityError)
	}

	if TypeCertExpiry != "cert_expiry" {
		t.Errorf("TypeCertExpiry = %v, want cert_expiry", TypeCertExpiry)
	}
	if TypeDomainExpiry != "domain_expiry" {
		t.Errorf("TypeDomainExpiry = %v, want domain_expiry", TypeDomainExpiry)
	}
	if TypeConfigChange != "config_change" {
		t.Errorf("TypeConfigChange = %v, want config_change", TypeConfigChange)
	}
	if TypeCaddyReload != "caddy_reload" {
		t.Errorf("TypeCaddyReload = %v, want caddy_reload", TypeCaddyReload)
	}
	if TypeContainerDown != "container_down" {
		t.Errorf("TypeContainerDown = %v, want container_down", TypeContainerDown)
	}
	if TypeSystem != "system" {
		t.Errorf("TypeSystem = %v, want system", TypeSystem)
	}
}
