package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/djedi/caddyshack/internal/config"
	"github.com/djedi/caddyshack/internal/docker"
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

	pageData := templates.PageData{
		Title:     "Containers",
		ActiveNav: "containers",
		Data:      data,
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
