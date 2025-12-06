package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
	"github.com/djedi/caddyshack/internal/templates"
)

func TestContainersHandlerList_Disabled(t *testing.T) {
	cfg := &config.Config{
		DockerEnabled: false,
	}

	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	handler := NewContainersHandler(tmpl, cfg)

	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Check that the response mentions Docker being disabled
	body := rr.Body.String()
	if !containsString(body, "Docker Integration Disabled") && !containsString(body, "Docker integration disabled") {
		t.Error("expected response to indicate Docker is disabled")
	}
}

func TestContainersHandlerWidget_Disabled(t *testing.T) {
	cfg := &config.Config{
		DockerEnabled: false,
	}

	tmpl, err := templates.New("../../templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	handler := NewContainersHandler(tmpl, cfg)

	req := httptest.NewRequest(http.MethodGet, "/containers/widget", nil)
	rr := httptest.NewRecorder()

	handler.Widget(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !containsString(body, "disabled") {
		t.Error("expected response to indicate Docker is disabled")
	}
}

func TestContainerToView(t *testing.T) {
	tests := []struct {
		name          string
		container     docker.ContainerInfo
		expectedColor string
	}{
		{
			name: "running container",
			container: docker.ContainerInfo{
				ID:    "abc123",
				Name:  "test",
				State: "running",
			},
			expectedColor: "green",
		},
		{
			name: "running but unhealthy",
			container: docker.ContainerInfo{
				ID:          "abc123",
				Name:        "test",
				State:       "running",
				HealthState: "unhealthy",
			},
			expectedColor: "yellow",
		},
		{
			name: "paused container",
			container: docker.ContainerInfo{
				ID:    "abc123",
				Name:  "test",
				State: "paused",
			},
			expectedColor: "yellow",
		},
		{
			name: "exited container",
			container: docker.ContainerInfo{
				ID:    "abc123",
				Name:  "test",
				State: "exited",
			},
			expectedColor: "red",
		},
		{
			name: "dead container",
			container: docker.ContainerInfo{
				ID:    "abc123",
				Name:  "test",
				State: "dead",
			},
			expectedColor: "red",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := containerToView(tt.container)
			if view.StateColor != tt.expectedColor {
				t.Errorf("expected StateColor %s, got %s", tt.expectedColor, view.StateColor)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
