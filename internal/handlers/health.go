package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
)

// ComponentStatus represents the health status of a single component.
type ComponentStatus struct {
	Status  string `json:"status"` // "healthy", "unhealthy", "degraded"
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"` // Response time for the check
}

// HealthResponse represents the comprehensive health check response.
type HealthResponse struct {
	Status     string                     `json:"status"` // "healthy", "unhealthy", "degraded"
	Timestamp  string                     `json:"timestamp"`
	Components map[string]ComponentStatus `json:"components"`
}

// HealthHandler handles health check requests.
type HealthHandler struct {
	cfg          *config.Config
	db           *sql.DB
	adminClient  *caddy.AdminClient
	dockerClient *docker.Client
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(cfg *config.Config, db *sql.DB) *HealthHandler {
	h := &HealthHandler{
		cfg:         cfg,
		db:          db,
		adminClient: caddy.NewAdminClient(cfg.CaddyAdminAPI),
	}

	if cfg.DockerEnabled {
		h.dockerClient = docker.NewClient(cfg.DockerSocket)
	}

	return h
}

// Health handles GET /health requests and returns comprehensive health status.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	response := HealthResponse{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: make(map[string]ComponentStatus),
	}

	// Check all components
	dbStatus := h.checkDatabase(ctx)
	caddyStatus := h.checkCaddy(ctx)

	response.Components["database"] = dbStatus
	response.Components["caddy"] = caddyStatus

	// Check Docker only if enabled
	if h.cfg.DockerEnabled {
		dockerStatus := h.checkDocker(ctx)
		response.Components["docker"] = dockerStatus
	}

	// Determine overall status
	response.Status = h.determineOverallStatus(response.Components)

	// Set appropriate HTTP status code
	httpStatus := http.StatusOK
	if response.Status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// checkDatabase checks database connectivity.
func (h *HealthHandler) checkDatabase(ctx context.Context) ComponentStatus {
	start := time.Now()

	if h.db == nil {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: "database connection not initialized",
		}
	}

	err := h.db.PingContext(ctx)
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	return ComponentStatus{
		Status:  "healthy",
		Message: "connected",
		Latency: latency.String(),
	}
}

// checkCaddy checks Caddy Admin API connectivity.
func (h *HealthHandler) checkCaddy(ctx context.Context) ComponentStatus {
	start := time.Now()

	err := h.adminClient.Ping(ctx)
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	// Get status for additional info
	status, err := h.adminClient.GetStatus(ctx)
	if err != nil || status == nil || !status.Running {
		return ComponentStatus{
			Status:  "degraded",
			Message: "API reachable but status unknown",
			Latency: latency.String(),
		}
	}

	message := "running"
	if status.Version != "" {
		message = "running (" + status.Version + ")"
	}

	return ComponentStatus{
		Status:  "healthy",
		Message: message,
		Latency: latency.String(),
	}
}

// checkDocker checks Docker daemon connectivity.
func (h *HealthHandler) checkDocker(ctx context.Context) ComponentStatus {
	start := time.Now()

	if h.dockerClient == nil {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: "docker client not initialized",
		}
	}

	err := h.dockerClient.Ping(ctx)
	latency := time.Since(start)

	if err != nil {
		return ComponentStatus{
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	return ComponentStatus{
		Status:  "healthy",
		Message: "connected",
		Latency: latency.String(),
	}
}

// determineOverallStatus determines the overall health status based on component statuses.
// - "healthy" if all components are healthy
// - "degraded" if some non-critical components are unhealthy (e.g., Docker)
// - "unhealthy" if critical components (database, caddy) are unhealthy
func (h *HealthHandler) determineOverallStatus(components map[string]ComponentStatus) string {
	criticalComponents := []string{"database", "caddy"}
	hasDegraded := false

	for name, status := range components {
		if status.Status == "unhealthy" {
			// Check if this is a critical component
			for _, critical := range criticalComponents {
				if name == critical {
					return "unhealthy"
				}
			}
			// Non-critical component is unhealthy
			hasDegraded = true
		} else if status.Status == "degraded" {
			hasDegraded = true
		}
	}

	if hasDegraded {
		return "degraded"
	}

	return "healthy"
}

// SimpleHealth handles GET /health requests with a simple OK response.
// This maintains backwards compatibility with the existing health endpoint.
func (h *HealthHandler) SimpleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n"))
}
