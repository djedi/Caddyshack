package caddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewAdminClient(t *testing.T) {
	client := NewAdminClient("http://localhost:2019")
	if client.baseURL != "http://localhost:2019" {
		t.Errorf("expected baseURL to be http://localhost:2019, got %s", client.baseURL)
	}

	// Test trailing slash is trimmed
	client = NewAdminClient("http://localhost:2019/")
	if client.baseURL != "http://localhost:2019" {
		t.Errorf("expected trailing slash to be trimmed, got %s", client.baseURL)
	}
}

func TestAdminClient_WithTimeout(t *testing.T) {
	client := NewAdminClient("http://localhost:2019")
	client.WithTimeout(60 * time.Second)

	if client.timeout != 60*time.Second {
		t.Errorf("expected timeout to be 60s, got %v", client.timeout)
	}
}

func TestAdminClient_Reload(t *testing.T) {
	tests := []struct {
		name           string
		caddyfile      string
		serverStatus   int
		serverResponse string
		wantErr        bool
		errContains    string
	}{
		{
			name:         "successful reload",
			caddyfile:    "localhost:8080 {\n\trespond \"Hello\"\n}",
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:           "invalid caddyfile",
			caddyfile:      "invalid { config",
			serverStatus:   http.StatusBadRequest,
			serverResponse: `{"error": "parsing caddyfile: unexpected token"}`,
			wantErr:        true,
			errContains:    "parsing caddyfile",
		},
		{
			name:         "server error",
			caddyfile:    "localhost:8080 {}",
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
			errContains:  "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/load" {
					t.Errorf("expected /load path, got %s", r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "text/caddyfile" {
					t.Errorf("expected Content-Type text/caddyfile, got %s", r.Header.Get("Content-Type"))
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != "" {
					w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			client := NewAdminClient(server.URL)
			err := client.Reload(context.Background(), tt.caddyfile)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAdminClient_GetConfig(t *testing.T) {
	expectedConfig := map[string]interface{}{
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{},
			},
		},
	}
	configJSON, _ := json.Marshal(expectedConfig)

	tests := []struct {
		name           string
		serverStatus   int
		serverResponse string
		wantErr        bool
	}{
		{
			name:           "successful get config",
			serverStatus:   http.StatusOK,
			serverResponse: string(configJSON),
			wantErr:        false,
		},
		{
			name:         "server error",
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/config/" {
					t.Errorf("expected /config/ path, got %s", r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != "" {
					w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			client := NewAdminClient(server.URL)
			config, err := client.GetConfig(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if config == nil {
					t.Error("expected config, got nil")
				}
			}
		})
	}
}

func TestAdminClient_GetStatus(t *testing.T) {
	t.Run("caddy running", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", "Caddy/2.7.6")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		status, err := client.GetStatus(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !status.Running {
			t.Error("expected Running to be true")
		}
		if status.Version != "Caddy/2.7.6" {
			t.Errorf("expected version Caddy/2.7.6, got %s", status.Version)
		}
	})

	t.Run("caddy not running", func(t *testing.T) {
		// Use a closed server to simulate unreachable Caddy
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close()

		client := NewAdminClient(server.URL)
		status, err := client.GetStatus(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if status.Running {
			t.Error("expected Running to be false for unreachable server")
		}
	})
}

func TestAdminClient_Ping(t *testing.T) {
	t.Run("successful ping", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		err := client.Ping(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("failed ping", func(t *testing.T) {
		// Use a closed server to simulate unreachable Caddy
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close()

		client := NewAdminClient(server.URL)
		err := client.Ping(context.Background())

		if err == nil {
			t.Error("expected error for unreachable server")
		}
	})
}

func TestAdminClient_Stop(t *testing.T) {
	t.Run("successful stop", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/stop" {
				t.Errorf("expected /stop path, got %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		err := client.Stop(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestAdminError(t *testing.T) {
	err := &AdminError{
		StatusCode: 400,
		Message:    "invalid config",
	}

	expected := "caddy admin api error (status 400): invalid config"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	// Test without message
	err = &AdminError{StatusCode: 500}
	expected = "caddy admin api error (status 500)"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestAdminClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewAdminClient(server.URL)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.Reload(ctx, "test")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAdminClient_GetPKICAInfo(t *testing.T) {
	t.Run("successful get CA info", func(t *testing.T) {
		caResponse := `{
			"id": "local",
			"name": "Caddy Local Authority",
			"root_cn": "Caddy Local Authority - 2024 ECC Root",
			"intermediate_cn": "Caddy Local Authority - ECC Intermediate"
		}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.URL.Path != "/pki/ca/local" {
				t.Errorf("expected /pki/ca/local path, got %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(caResponse))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		caInfo, err := client.GetPKICAInfo(context.Background(), "local")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if caInfo == nil {
			t.Fatal("expected caInfo, got nil")
		}
		if caInfo.ID != "local" {
			t.Errorf("expected ID 'local', got %s", caInfo.ID)
		}
		if caInfo.Name != "Caddy Local Authority" {
			t.Errorf("expected Name 'Caddy Local Authority', got %s", caInfo.Name)
		}
		if !caInfo.Provisioned {
			t.Error("expected Provisioned to be true")
		}
	})

	t.Run("PKI not configured (404)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		caInfo, err := client.GetPKICAInfo(context.Background(), "local")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if caInfo != nil {
			t.Errorf("expected nil for unconfigured PKI, got %v", caInfo)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		_, err := client.GetPKICAInfo(context.Background(), "local")

		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestAdminClient_GetCertificates(t *testing.T) {
	t.Run("extract domains from TLS automation", func(t *testing.T) {
		configResponse := `{
			"apps": {
				"tls": {
					"automation": {
						"policies": [
							{
								"subjects": ["example.com", "www.example.com"]
							}
						]
					}
				},
				"http": {
					"servers": {}
				}
			}
		}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(configResponse))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		certs, err := client.GetCertificates(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(certs) != 2 {
			t.Errorf("expected 2 certificates, got %d", len(certs))
		}

		// Check domains are present
		domains := make(map[string]bool)
		for _, cert := range certs {
			domains[cert.Domain] = true
		}
		if !domains["example.com"] {
			t.Error("expected example.com in certificates")
		}
		if !domains["www.example.com"] {
			t.Error("expected www.example.com in certificates")
		}
	})

	t.Run("filter localhost domains", func(t *testing.T) {
		configResponse := `{
			"apps": {
				"tls": {
					"automation": {
						"policies": [
							{
								"subjects": ["example.com", "localhost", "127.0.0.1"]
							}
						]
					}
				},
				"http": {
					"servers": {}
				}
			}
		}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(configResponse))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		certs, err := client.GetCertificates(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(certs) != 1 {
			t.Errorf("expected 1 certificate (localhost filtered), got %d", len(certs))
		}
		if certs[0].Domain != "example.com" {
			t.Errorf("expected example.com, got %s", certs[0].Domain)
		}
	})

	t.Run("extract from HTTP server routes", func(t *testing.T) {
		configResponse := `{
			"apps": {
				"tls": {},
				"http": {
					"servers": {
						"srv0": {
							"listen": [":443"],
							"routes": [
								{
									"match": [{"host": ["api.example.com"]}]
								}
							]
						}
					}
				}
			}
		}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(configResponse))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		certs, err := client.GetCertificates(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(certs) != 1 {
			t.Errorf("expected 1 certificate, got %d", len(certs))
		}
		if certs[0].Domain != "api.example.com" {
			t.Errorf("expected api.example.com, got %s", certs[0].Domain)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		configResponse := `{"apps": {}}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(configResponse))
		}))
		defer server.Close()

		client := NewAdminClient(server.URL)
		certs, err := client.GetCertificates(context.Background())

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(certs) != 0 {
			t.Errorf("expected 0 certificates for empty config, got %d", len(certs))
		}
	})
}

func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		domain   string
		expected bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"::1", true},
		{"myapp.local", true},
		{"app.localhost", true},
		{"example.com", false},
		{"www.example.com", false},
		{"api.mycompany.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := isLocalhost(tt.domain)
			if result != tt.expected {
				t.Errorf("isLocalhost(%q) = %v, expected %v", tt.domain, result, tt.expected)
			}
		})
	}
}
