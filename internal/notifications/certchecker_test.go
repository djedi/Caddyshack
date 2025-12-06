package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/djedi/caddyshack/internal/store"
)

// newTestServiceAndStore creates a new notification service and store for testing.
func newTestServiceAndStore(t *testing.T) (*Service, *store.Store) {
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

	return NewService(s.DB()), s
}

// mockCaddyServer creates a mock Caddy Admin API server.
func mockCaddyServer(t *testing.T, configResponse interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/config/":
			w.Header().Set("Content-Type", "application/json")
			if configResponse == nil {
				w.Write([]byte("{}"))
			} else {
				json.NewEncoder(w).Encode(configResponse)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCertificateChecker_NewCertificateChecker(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)
	checker := NewCertificateChecker(svc, "http://localhost:2019")

	if checker == nil {
		t.Fatal("NewCertificateChecker() returned nil")
	}
	if checker.checkInterval != 24*time.Hour {
		t.Errorf("checkInterval = %v, want %v", checker.checkInterval, 24*time.Hour)
	}
	if checker.warningThreshold != 30 {
		t.Errorf("warningThreshold = %d, want 30", checker.warningThreshold)
	}
	if checker.criticalThreshold != 7 {
		t.Errorf("criticalThreshold = %d, want 7", checker.criticalThreshold)
	}
}

func TestCertificateChecker_WithCheckInterval(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)
	checker := NewCertificateChecker(svc, "http://localhost:2019").
		WithCheckInterval(1 * time.Hour)

	if checker.checkInterval != 1*time.Hour {
		t.Errorf("checkInterval = %v, want %v", checker.checkInterval, 1*time.Hour)
	}
}

func TestCertificateChecker_WithThresholds(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)
	checker := NewCertificateChecker(svc, "http://localhost:2019").
		WithThresholds(60, 14)

	if checker.warningThreshold != 60 {
		t.Errorf("warningThreshold = %d, want 60", checker.warningThreshold)
	}
	if checker.criticalThreshold != 14 {
		t.Errorf("criticalThreshold = %d, want 14", checker.criticalThreshold)
	}
}

func TestCertificateChecker_StartStop(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)
	checker := NewCertificateChecker(svc, "http://localhost:2019")

	// Start should not block
	checker.Start()

	// Should be able to call Start again (idempotent)
	checker.Start()

	// Stop should not block
	checker.Stop()

	// Should be able to call Stop again (idempotent)
	checker.Stop()
}

func TestCertificateChecker_CheckAll_CaddyNotReachable(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)
	// Use an invalid URL so Caddy is not reachable
	checker := NewCertificateChecker(svc, "http://localhost:1")

	// CheckAll should not panic when Caddy is not reachable
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

func TestCertificateChecker_CheckAll_NoCertificates(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)

	// Mock Caddy server with no certificates
	server := mockCaddyServer(t, map[string]interface{}{
		"apps": map[string]interface{}{},
	})
	defer server.Close()

	checker := NewCertificateChecker(svc, server.URL)
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

func TestCertExpiryData_JSON(t *testing.T) {
	data := CertExpiryData{
		Domain:    "example.com",
		Threshold: "30",
		ExpiresAt: "2025-01-15T00:00:00Z",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded CertExpiryData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Domain != data.Domain {
		t.Errorf("Domain = %v, want %v", decoded.Domain, data.Domain)
	}
	if decoded.Threshold != data.Threshold {
		t.Errorf("Threshold = %v, want %v", decoded.Threshold, data.Threshold)
	}
	if decoded.ExpiresAt != data.ExpiresAt {
		t.Errorf("ExpiresAt = %v, want %v", decoded.ExpiresAt, data.ExpiresAt)
	}
}

func TestCertificateChecker_DuplicatePrevention(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)

	// Create a mock notification with the same data format as the checker would create
	data := CertExpiryData{
		Domain:    "example.com",
		Threshold: "30",
		ExpiresAt: "2025-02-15T00:00:00Z",
	}
	dataJSON, _ := json.Marshal(data)

	// Create an existing notification
	_, err := svc.Create(TypeCertExpiry, SeverityWarning, "Test", "Test", string(dataJSON))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify the notification exists
	exists, err := svc.ExistsUnacknowledged(TypeCertExpiry, string(dataJSON))
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if !exists {
		t.Error("ExistsUnacknowledged() should return true")
	}

	// A different threshold should not match
	data2 := CertExpiryData{
		Domain:    "example.com",
		Threshold: "7",
		ExpiresAt: "2025-02-15T00:00:00Z",
	}
	dataJSON2, _ := json.Marshal(data2)

	exists, err = svc.ExistsUnacknowledged(TypeCertExpiry, string(dataJSON2))
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if exists {
		t.Error("ExistsUnacknowledged() should return false for different threshold")
	}
}

func TestCertificateChecker_AcknowledgedNotificationAllowsNew(t *testing.T) {
	svc, _ := newTestServiceAndStore(t)

	data := CertExpiryData{
		Domain:    "example.com",
		Threshold: "30",
		ExpiresAt: "2025-02-15T00:00:00Z",
	}
	dataJSON, _ := json.Marshal(data)

	// Create and acknowledge a notification
	n, err := svc.Create(TypeCertExpiry, SeverityWarning, "Test", "Test", string(dataJSON))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Acknowledge(n.ID); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}

	// ExistsUnacknowledged should return false after acknowledging
	exists, err := svc.ExistsUnacknowledged(TypeCertExpiry, string(dataJSON))
	if err != nil {
		t.Fatalf("ExistsUnacknowledged() error = %v", err)
	}
	if exists {
		t.Error("ExistsUnacknowledged() should return false for acknowledged notification")
	}
}
