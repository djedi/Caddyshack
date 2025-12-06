package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
	"github.com/djedi/caddyshack/internal/middleware"
	"github.com/djedi/caddyshack/internal/templates"
)

// ContainersData holds data displayed on the containers page.
type ContainersData struct {
	Containers      []ContainerView
	Stats           docker.ContainerStats
	Error           string
	HasError        bool
	DockerAvailable bool
	DockerEnabled   bool
}

// ContainerView is a view model for container information.
type ContainerView struct {
	ID          string
	Name        string
	Image       string
	State       string
	Status      string
	Ports       []string
	HealthState string
	StateColor  string // Tailwind color class
}

// ContainersHandler handles requests for the containers pages.
type ContainersHandler struct {
	templates     *templates.Templates
	dockerClient  *docker.Client
	errorHandler  *ErrorHandler
	dockerEnabled bool
}

// NewContainersHandler creates a new ContainersHandler.
func NewContainersHandler(tmpl *templates.Templates, cfg *config.Config) *ContainersHandler {
	var client *docker.Client
	if cfg.DockerEnabled {
		client = docker.NewClient(cfg.DockerSocket)
	}

	return &ContainersHandler{
		templates:     tmpl,
		dockerClient:  client,
		errorHandler:  NewErrorHandler(tmpl),
		dockerEnabled: cfg.DockerEnabled,
	}
}

// List handles GET requests for the containers list page.
func (h *ContainersHandler) List(w http.ResponseWriter, r *http.Request) {
	data := ContainersData{
		DockerEnabled: h.dockerEnabled,
	}

	if !h.dockerEnabled {
		data.DockerAvailable = false
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Check if Docker is available
		data.DockerAvailable = h.dockerClient.IsAvailable(ctx)

		if !data.DockerAvailable {
			data.Error = "Unable to connect to Docker. Please ensure Docker is running and the socket is accessible."
			data.HasError = true
		} else {
			// Get containers
			containers, err := h.dockerClient.ListContainers(ctx)
			if err != nil {
				data.Error = "Failed to retrieve container information: " + err.Error()
				data.HasError = true
			} else {
				// Get stats
				stats, _ := h.dockerClient.GetContainerStats(ctx)
				if stats != nil {
					data.Stats = *stats
				}

				// Convert to view models
				data.Containers = make([]ContainerView, 0, len(containers))
				for _, c := range containers {
					data.Containers = append(data.Containers, containerToView(c))
				}
			}
		}
	}

	// Get permissions for the template
	perms := middleware.GetUserPermissions(r)

	pageData := templates.PageData{
		Title:       "Containers",
		ActiveNav:   "containers",
		Data:        data,
		Permissions: perms,
	}

	if err := h.templates.Render(w, "containers.html", pageData); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Widget handles GET requests for the container status widget (for HTMX polling).
func (h *ContainersHandler) Widget(w http.ResponseWriter, r *http.Request) {
	data := struct {
		DockerAvailable bool
		DockerEnabled   bool
		Stats           docker.ContainerStats
	}{
		DockerEnabled: h.dockerEnabled,
	}

	if h.dockerEnabled && h.dockerClient != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		data.DockerAvailable = h.dockerClient.IsAvailable(ctx)
		if data.DockerAvailable {
			stats, _ := h.dockerClient.GetContainerStats(ctx)
			if stats != nil {
				data.Stats = *stats
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "container-widget.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// Start handles POST requests to start a container.
func (h *ContainersHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containerID := extractContainerID(r.URL.Path)
	if containerID == "" {
		h.renderActionError(w, "Container ID is required")
		return
	}

	if !h.dockerEnabled || h.dockerClient == nil {
		h.renderActionError(w, "Docker integration is not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.dockerClient.StartContainer(ctx, containerID); err != nil {
		h.renderActionError(w, "Failed to start container: "+err.Error())
		return
	}

	// Fetch the updated container info
	h.renderContainerRow(w, r, containerID)
}

// Stop handles POST requests to stop a container.
func (h *ContainersHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containerID := extractContainerID(r.URL.Path)
	if containerID == "" {
		h.renderActionError(w, "Container ID is required")
		return
	}

	if !h.dockerEnabled || h.dockerClient == nil {
		h.renderActionError(w, "Docker integration is not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Use a 10 second timeout for graceful shutdown
	if err := h.dockerClient.StopContainer(ctx, containerID, 10); err != nil {
		h.renderActionError(w, "Failed to stop container: "+err.Error())
		return
	}

	// Fetch the updated container info
	h.renderContainerRow(w, r, containerID)
}

// Restart handles POST requests to restart a container.
func (h *ContainersHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containerID := extractContainerID(r.URL.Path)
	if containerID == "" {
		h.renderActionError(w, "Container ID is required")
		return
	}

	if !h.dockerEnabled || h.dockerClient == nil {
		h.renderActionError(w, "Docker integration is not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Use a 10 second timeout for graceful shutdown before restart
	if err := h.dockerClient.RestartContainer(ctx, containerID, 10); err != nil {
		h.renderActionError(w, "Failed to restart container: "+err.Error())
		return
	}

	// Fetch the updated container info
	h.renderContainerRow(w, r, containerID)
}

// Logs handles GET requests to view container logs.
func (h *ContainersHandler) Logs(w http.ResponseWriter, r *http.Request) {
	containerID := extractContainerID(r.URL.Path)
	if containerID == "" {
		h.renderActionError(w, "Container ID is required")
		return
	}

	if !h.dockerEnabled || h.dockerClient == nil {
		h.renderActionError(w, "Docker integration is not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get container info for the name
	container, err := h.dockerClient.GetContainer(ctx, containerID)
	if err != nil || container == nil {
		h.renderActionError(w, "Container not found")
		return
	}

	// Parse query params
	tail := 100 // Default to last 100 lines
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 {
			tail = parsed
		}
	}

	logs, err := h.dockerClient.ContainerLogs(ctx, containerID, tail, 0)
	if err != nil {
		h.renderActionError(w, "Failed to fetch logs: "+err.Error())
		return
	}

	// Get permissions for the template
	perms := middleware.GetUserPermissions(r)

	data := struct {
		Container   *docker.ContainerInfo
		Logs        string
		Tail        int
		Permissions *middleware.UserPermissions
	}{
		Container:   container,
		Logs:        logs,
		Tail:        tail,
		Permissions: perms,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "container-logs.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderContainerRow renders a single container row after an action.
func (h *ContainersHandler) renderContainerRow(w http.ResponseWriter, r *http.Request, containerID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	container, err := h.dockerClient.GetContainer(ctx, containerID)
	if err != nil || container == nil {
		h.renderActionError(w, "Failed to fetch container info")
		return
	}

	view := containerToView(*container)

	// Get permissions for the template
	perms := middleware.GetUserPermissions(r)

	data := struct {
		Container   ContainerView
		Permissions *middleware.UserPermissions
	}{
		Container:   view,
		Permissions: perms,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.RenderPartial(w, "container-row.html", data); err != nil {
		h.errorHandler.InternalServerError(w, r, err)
	}
}

// renderActionError renders an error message for HTMX swaps.
func (h *ContainersHandler) renderActionError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	h.templates.RenderPartial(w, "error-message.html", map[string]string{"Message": message})
}

// extractContainerID extracts the container ID from a URL path.
// Expected paths: /containers/{id}/start, /containers/{id}/stop, /containers/{id}/restart, /containers/{id}/logs
func extractContainerID(path string) string {
	path = strings.TrimPrefix(path, "/containers/")
	parts := strings.Split(path, "/")
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// containerToView converts a ContainerInfo to a ContainerView.
func containerToView(c docker.ContainerInfo) ContainerView {
	view := ContainerView{
		ID:          c.ID,
		Name:        c.Name,
		Image:       c.Image,
		State:       c.State,
		Status:      c.Status,
		Ports:       c.Ports,
		HealthState: c.HealthState,
	}

	// Set state color based on container state
	switch c.State {
	case "running":
		if c.HealthState == "unhealthy" {
			view.StateColor = "yellow"
		} else {
			view.StateColor = "green"
		}
	case "paused":
		view.StateColor = "yellow"
	case "restarting":
		view.StateColor = "yellow"
	default: // exited, dead, created
		view.StateColor = "red"
	}

	return view
}
