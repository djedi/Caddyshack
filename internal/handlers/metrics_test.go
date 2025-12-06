package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
)

func TestMetricsHandler_Metrics(t *testing.T) {
	// Create config with Docker disabled for basic test
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:2019",
		DockerEnabled: false,
		MultiUserMode: false,
	}

	handler := NewMetricsHandler(cfg)

	// Make a request to the metrics endpoint
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	resp := w.Result()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected content type text/plain, got %s", contentType)
	}

	body := w.Body.String()

	// Verify Caddy metrics are present
	expectedMetrics := []string{
		"# HELP caddyshack_caddy_up",
		"# TYPE caddyshack_caddy_up gauge",
		"caddyshack_caddy_up",
		"# HELP caddyshack_config_reloads_total",
		"# TYPE caddyshack_config_reloads_total counter",
		"caddyshack_config_reloads_total",
		"# HELP caddyshack_certificates_total",
		"# TYPE caddyshack_certificates_total gauge",
		"# HELP caddyshack_docker_enabled",
		"# TYPE caddyshack_docker_enabled gauge",
		"caddyshack_docker_enabled 0", // Docker is disabled
		"# HELP caddyshack_uptime_seconds",
		"# TYPE caddyshack_uptime_seconds gauge",
		"caddyshack_uptime_seconds",
		"# HELP caddyshack_info",
		"# TYPE caddyshack_info gauge",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected body to contain %q, body:\n%s", metric, body)
		}
	}
}

func TestMetricsHandler_MetricsWithDockerEnabled(t *testing.T) {
	// Create config with Docker enabled
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:2019",
		DockerEnabled: true,
		DockerSocket:  "/var/run/docker.sock",
		MultiUserMode: false,
	}

	handler := NewMetricsHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	// When Docker is enabled but not reachable, we should still see the enabled metric
	if !strings.Contains(body, "caddyshack_docker_enabled 1") {
		t.Errorf("expected body to contain docker enabled metric, body:\n%s", body)
	}

	// Should also have docker_available metric
	if !strings.Contains(body, "caddyshack_docker_available") {
		t.Errorf("expected body to contain docker_available metric, body:\n%s", body)
	}
}

func TestMetricsHandler_ConfigReloads(t *testing.T) {
	cfg := &config.Config{
		CaddyAdminAPI: "http://localhost:2019",
		DockerEnabled: false,
	}

	handler := NewMetricsHandler(cfg)

	// Initially should be 0
	if count := handler.GetConfigReloads(); count != 0 {
		t.Errorf("expected initial reload count 0, got %d", count)
	}

	// Increment and check
	handler.IncrementConfigReloads()
	if count := handler.GetConfigReloads(); count != 1 {
		t.Errorf("expected reload count 1, got %d", count)
	}

	handler.IncrementConfigReloads()
	handler.IncrementConfigReloads()
	if count := handler.GetConfigReloads(); count != 3 {
		t.Errorf("expected reload count 3, got %d", count)
	}

	// Verify it shows up in metrics output
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "caddyshack_config_reloads_total 3") {
		t.Errorf("expected body to contain config_reloads_total 3, body:\n%s", body)
	}
}

func TestMetricsHandler_ApplicationInfo(t *testing.T) {
	tests := []struct {
		name          string
		dockerEnabled bool
		multiUser     bool
		expectedInfo  string
	}{
		{
			name:          "docker disabled, single user",
			dockerEnabled: false,
			multiUser:     false,
			expectedInfo:  `caddyshack_info{docker_enabled="false",multi_user="false"} 1`,
		},
		{
			name:          "docker enabled, single user",
			dockerEnabled: true,
			multiUser:     false,
			expectedInfo:  `caddyshack_info{docker_enabled="true",multi_user="false"} 1`,
		},
		{
			name:          "docker disabled, multi user",
			dockerEnabled: false,
			multiUser:     true,
			expectedInfo:  `caddyshack_info{docker_enabled="false",multi_user="true"} 1`,
		},
		{
			name:          "docker enabled, multi user",
			dockerEnabled: true,
			multiUser:     true,
			expectedInfo:  `caddyshack_info{docker_enabled="true",multi_user="true"} 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				CaddyAdminAPI: "http://localhost:2019",
				DockerEnabled: tt.dockerEnabled,
				DockerSocket:  "/var/run/docker.sock",
				MultiUserMode: tt.multiUser,
			}

			handler := NewMetricsHandler(cfg)

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			w := httptest.NewRecorder()

			handler.Metrics(w, req)

			body := w.Body.String()
			if !strings.Contains(body, tt.expectedInfo) {
				t.Errorf("expected body to contain %q, body:\n%s", tt.expectedInfo, body)
			}
		})
	}
}

func TestBoolToString(t *testing.T) {
	if boolToString(true) != "true" {
		t.Error("expected boolToString(true) to return 'true'")
	}
	if boolToString(false) != "false" {
		t.Error("expected boolToString(false) to return 'false'")
	}
}
