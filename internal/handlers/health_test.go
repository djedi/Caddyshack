package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	_ "modernc.org/sqlite"
)

func TestHealthHandler_Health(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	// Create a mock Caddy server
	caddyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Caddy/2.7.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer caddyServer.Close()

	cfg := &config.Config{
		CaddyAdminAPI: caddyServer.URL,
		DockerEnabled: false,
	}

	handler := NewHealthHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check overall status
	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", response.Status)
	}

	// Check timestamp is present
	if response.Timestamp == "" {
		t.Error("expected timestamp to be present")
	}

	// Check database component
	dbStatus, ok := response.Components["database"]
	if !ok {
		t.Error("expected database component in response")
	}
	if dbStatus.Status != "healthy" {
		t.Errorf("expected database status 'healthy', got %q: %s", dbStatus.Status, dbStatus.Message)
	}

	// Check caddy component
	caddyStatus, ok := response.Components["caddy"]
	if !ok {
		t.Error("expected caddy component in response")
	}
	if caddyStatus.Status != "healthy" {
		t.Errorf("expected caddy status 'healthy', got %q: %s", caddyStatus.Status, caddyStatus.Message)
	}

	// Docker should not be present when disabled
	if _, ok := response.Components["docker"]; ok {
		t.Error("docker component should not be present when DockerEnabled is false")
	}
}

func TestHealthHandler_Health_UnhealthyDatabase(t *testing.T) {
	// Create a database and close it to simulate unhealthy state
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	db.Close() // Close immediately to make it unhealthy

	// Create a mock Caddy server
	caddyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Caddy/2.7.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer caddyServer.Close()

	cfg := &config.Config{
		CaddyAdminAPI: caddyServer.URL,
		DockerEnabled: false,
	}

	handler := NewHealthHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	// Should return 503 Service Unavailable when database is unhealthy
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check overall status is unhealthy
	if response.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got %q", response.Status)
	}

	// Check database component is unhealthy
	dbStatus := response.Components["database"]
	if dbStatus.Status != "unhealthy" {
		t.Errorf("expected database status 'unhealthy', got %q", dbStatus.Status)
	}
}

func TestHealthHandler_Health_UnhealthyCaddy(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	// Use an invalid URL for Caddy to simulate it being down
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:1", // Invalid port
		DockerEnabled: false,
	}

	handler := NewHealthHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	// Should return 503 Service Unavailable when Caddy is unhealthy
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check overall status is unhealthy
	if response.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got %q", response.Status)
	}

	// Check caddy component is unhealthy
	caddyStatus := response.Components["caddy"]
	if caddyStatus.Status != "unhealthy" {
		t.Errorf("expected caddy status 'unhealthy', got %q: %s", caddyStatus.Status, caddyStatus.Message)
	}
}

func TestHealthHandler_Health_WithDocker(t *testing.T) {
	// Create a test database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	// Create a mock Caddy server
	caddyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Caddy/2.7.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer caddyServer.Close()

	cfg := &config.Config{
		CaddyAdminAPI: caddyServer.URL,
		DockerEnabled: true,
		DockerSocket:  "/nonexistent/docker.sock", // Will fail but should be included
	}

	handler := NewHealthHandler(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	var response HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Docker should be present when enabled
	dockerStatus, ok := response.Components["docker"]
	if !ok {
		t.Error("docker component should be present when DockerEnabled is true")
	}

	// Docker should be unhealthy since socket doesn't exist
	if dockerStatus.Status != "unhealthy" {
		t.Errorf("expected docker status 'unhealthy', got %q", dockerStatus.Status)
	}

	// Overall status should be degraded (non-critical component down)
	if response.Status != "degraded" {
		t.Errorf("expected overall status 'degraded', got %q", response.Status)
	}
}

func TestHealthHandler_SimpleHealth(t *testing.T) {
	cfg := &config.Config{}
	handler := NewHealthHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.SimpleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if body := rr.Body.String(); body != "ok\n" {
		t.Errorf("expected body 'ok\\n', got %q", body)
	}
}

func TestHealthHandler_DetermineOverallStatus(t *testing.T) {
	cfg := &config.Config{}
	handler := NewHealthHandler(cfg, nil)

	tests := []struct {
		name       string
		components map[string]ComponentStatus
		expected   string
	}{
		{
			name: "all healthy",
			components: map[string]ComponentStatus{
				"database": {Status: "healthy"},
				"caddy":    {Status: "healthy"},
			},
			expected: "healthy",
		},
		{
			name: "critical database unhealthy",
			components: map[string]ComponentStatus{
				"database": {Status: "unhealthy"},
				"caddy":    {Status: "healthy"},
			},
			expected: "unhealthy",
		},
		{
			name: "critical caddy unhealthy",
			components: map[string]ComponentStatus{
				"database": {Status: "healthy"},
				"caddy":    {Status: "unhealthy"},
			},
			expected: "unhealthy",
		},
		{
			name: "non-critical docker unhealthy",
			components: map[string]ComponentStatus{
				"database": {Status: "healthy"},
				"caddy":    {Status: "healthy"},
				"docker":   {Status: "unhealthy"},
			},
			expected: "degraded",
		},
		{
			name: "caddy degraded",
			components: map[string]ComponentStatus{
				"database": {Status: "healthy"},
				"caddy":    {Status: "degraded"},
			},
			expected: "degraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.determineOverallStatus(tt.components)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestHealthHandler_CheckDatabase(t *testing.T) {
	cfg := &config.Config{}

	t.Run("nil database", func(t *testing.T) {
		handler := NewHealthHandler(cfg, nil)
		status := handler.checkDatabase(context.Background())
		if status.Status != "unhealthy" {
			t.Errorf("expected unhealthy status for nil db, got %q", status.Status)
		}
	})

	t.Run("healthy database", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("failed to create test database: %v", err)
		}
		defer db.Close()

		handler := NewHealthHandler(cfg, db)
		status := handler.checkDatabase(context.Background())
		if status.Status != "healthy" {
			t.Errorf("expected healthy status, got %q: %s", status.Status, status.Message)
		}
		if status.Latency == "" {
			t.Error("expected latency to be set")
		}
	})
}
