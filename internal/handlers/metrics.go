package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
)

// MetricsHandler handles requests for the Prometheus metrics endpoint.
type MetricsHandler struct {
	cfg          *config.Config
	adminClient  *caddy.AdminClient
	dockerClient *docker.Client

	// Track config reloads (needs to be incremented externally)
	configReloads int64
	reloadMutex   sync.RWMutex

	// Track application start time for uptime calculation
	startTime time.Time
}

// NewMetricsHandler creates a new MetricsHandler.
func NewMetricsHandler(cfg *config.Config) *MetricsHandler {
	h := &MetricsHandler{
		cfg:         cfg,
		adminClient: caddy.NewAdminClient(cfg.CaddyAdminAPI),
		startTime:   time.Now(),
	}

	if cfg.DockerEnabled {
		h.dockerClient = docker.NewClient(cfg.DockerSocket)
	}

	return h
}

// IncrementConfigReloads increments the config reload counter.
// This should be called by other handlers when a config reload occurs.
func (h *MetricsHandler) IncrementConfigReloads() {
	h.reloadMutex.Lock()
	defer h.reloadMutex.Unlock()
	h.configReloads++
}

// GetConfigReloads returns the current count of config reloads.
func (h *MetricsHandler) GetConfigReloads() int64 {
	h.reloadMutex.RLock()
	defer h.reloadMutex.RUnlock()
	return h.configReloads
}

// Metrics handles GET requests for the Prometheus metrics endpoint.
func (h *MetricsHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// Collect and write all metrics
	h.writeCaddyMetrics(ctx, w)
	h.writeCertificateMetrics(ctx, w)
	h.writeContainerMetrics(ctx, w)
	h.writeApplicationMetrics(w)
}

// writeCaddyMetrics writes Caddy server status metrics.
func (h *MetricsHandler) writeCaddyMetrics(ctx context.Context, w http.ResponseWriter) {
	// Caddy status metric
	status, err := h.adminClient.GetStatus(ctx)

	// caddyshack_caddy_up: 1 if Caddy is running, 0 otherwise
	caddyUp := 0
	if err == nil && status != nil && status.Running {
		caddyUp = 1
	}
	fmt.Fprintf(w, "# HELP caddyshack_caddy_up Whether Caddy server is running (1 = up, 0 = down)\n")
	fmt.Fprintf(w, "# TYPE caddyshack_caddy_up gauge\n")
	fmt.Fprintf(w, "caddyshack_caddy_up %d\n", caddyUp)

	// Caddy version info (as a label on an info metric)
	if status != nil && status.Version != "" {
		fmt.Fprintf(w, "# HELP caddyshack_caddy_info Caddy server information\n")
		fmt.Fprintf(w, "# TYPE caddyshack_caddy_info gauge\n")
		fmt.Fprintf(w, "caddyshack_caddy_info{version=%q} 1\n", status.Version)
	}

	// Config reload counter
	fmt.Fprintf(w, "# HELP caddyshack_config_reloads_total Total number of configuration reloads\n")
	fmt.Fprintf(w, "# TYPE caddyshack_config_reloads_total counter\n")
	fmt.Fprintf(w, "caddyshack_config_reloads_total %d\n", h.GetConfigReloads())

	fmt.Fprintln(w)
}

// writeCertificateMetrics writes TLS certificate status metrics.
func (h *MetricsHandler) writeCertificateMetrics(ctx context.Context, w http.ResponseWriter) {
	certs, err := h.adminClient.GetCertificates(ctx)
	if err != nil {
		// Write zero values if we can't fetch certificates
		fmt.Fprintf(w, "# HELP caddyshack_certificates_total Total number of managed certificates\n")
		fmt.Fprintf(w, "# TYPE caddyshack_certificates_total gauge\n")
		fmt.Fprintf(w, "caddyshack_certificates_total 0\n")
		fmt.Fprintln(w)
		return
	}

	// Count certificates by status
	var total, valid, expiring, expired, unknown int
	for _, cert := range certs {
		total++
		switch cert.Status {
		case "valid":
			valid++
		case "expiring":
			expiring++
		case "expired":
			expired++
		default:
			unknown++
		}
	}

	// Total certificates
	fmt.Fprintf(w, "# HELP caddyshack_certificates_total Total number of managed certificates\n")
	fmt.Fprintf(w, "# TYPE caddyshack_certificates_total gauge\n")
	fmt.Fprintf(w, "caddyshack_certificates_total %d\n", total)

	// Certificates by status
	fmt.Fprintf(w, "# HELP caddyshack_certificates_status Number of certificates by status\n")
	fmt.Fprintf(w, "# TYPE caddyshack_certificates_status gauge\n")
	fmt.Fprintf(w, "caddyshack_certificates_status{status=\"valid\"} %d\n", valid)
	fmt.Fprintf(w, "caddyshack_certificates_status{status=\"expiring\"} %d\n", expiring)
	fmt.Fprintf(w, "caddyshack_certificates_status{status=\"expired\"} %d\n", expired)
	fmt.Fprintf(w, "caddyshack_certificates_status{status=\"unknown\"} %d\n", unknown)

	// Per-domain certificate days remaining (useful for alerting)
	fmt.Fprintf(w, "# HELP caddyshack_certificate_expiry_days Days until certificate expires\n")
	fmt.Fprintf(w, "# TYPE caddyshack_certificate_expiry_days gauge\n")
	for _, cert := range certs {
		if cert.DaysRemaining >= 0 {
			fmt.Fprintf(w, "caddyshack_certificate_expiry_days{domain=%q} %d\n", cert.Domain, cert.DaysRemaining)
		}
	}

	fmt.Fprintln(w)
}

// writeContainerMetrics writes Docker container status metrics.
func (h *MetricsHandler) writeContainerMetrics(ctx context.Context, w http.ResponseWriter) {
	// Docker enabled metric
	fmt.Fprintf(w, "# HELP caddyshack_docker_enabled Whether Docker integration is enabled\n")
	fmt.Fprintf(w, "# TYPE caddyshack_docker_enabled gauge\n")
	if h.cfg.DockerEnabled {
		fmt.Fprintf(w, "caddyshack_docker_enabled 1\n")
	} else {
		fmt.Fprintf(w, "caddyshack_docker_enabled 0\n")
		fmt.Fprintln(w)
		return
	}

	// Docker available metric
	dockerAvailable := 0
	if h.dockerClient != nil && h.dockerClient.IsAvailable(ctx) {
		dockerAvailable = 1
	}
	fmt.Fprintf(w, "# HELP caddyshack_docker_available Whether Docker daemon is reachable\n")
	fmt.Fprintf(w, "# TYPE caddyshack_docker_available gauge\n")
	fmt.Fprintf(w, "caddyshack_docker_available %d\n", dockerAvailable)

	if dockerAvailable == 0 || h.dockerClient == nil {
		fmt.Fprintln(w)
		return
	}

	// Get container stats
	stats, err := h.dockerClient.GetContainerStats(ctx)
	if err != nil {
		fmt.Fprintln(w)
		return
	}

	// Total containers
	fmt.Fprintf(w, "# HELP caddyshack_containers_total Total number of Docker containers\n")
	fmt.Fprintf(w, "# TYPE caddyshack_containers_total gauge\n")
	fmt.Fprintf(w, "caddyshack_containers_total %d\n", stats.Total)

	// Containers by status
	fmt.Fprintf(w, "# HELP caddyshack_containers_status Number of containers by status\n")
	fmt.Fprintf(w, "# TYPE caddyshack_containers_status gauge\n")
	fmt.Fprintf(w, "caddyshack_containers_status{status=\"running\"} %d\n", stats.Running)
	fmt.Fprintf(w, "caddyshack_containers_status{status=\"stopped\"} %d\n", stats.Stopped)
	fmt.Fprintf(w, "caddyshack_containers_status{status=\"unhealthy\"} %d\n", stats.Unhealthy)

	fmt.Fprintln(w)
}

// writeApplicationMetrics writes Caddyshack application metrics.
func (h *MetricsHandler) writeApplicationMetrics(w http.ResponseWriter) {
	// Application uptime in seconds
	uptime := time.Since(h.startTime).Seconds()
	fmt.Fprintf(w, "# HELP caddyshack_uptime_seconds Time since Caddyshack started in seconds\n")
	fmt.Fprintf(w, "# TYPE caddyshack_uptime_seconds gauge\n")
	fmt.Fprintf(w, "caddyshack_uptime_seconds %.2f\n", uptime)

	// Application info
	fmt.Fprintf(w, "# HELP caddyshack_info Caddyshack application information\n")
	fmt.Fprintf(w, "# TYPE caddyshack_info gauge\n")
	fmt.Fprintf(w, "caddyshack_info{docker_enabled=%q,multi_user=%q} 1\n",
		boolToString(h.cfg.DockerEnabled),
		boolToString(h.cfg.MultiUserMode))

	fmt.Fprintln(w)
}

// boolToString converts a bool to "true" or "false" string.
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
