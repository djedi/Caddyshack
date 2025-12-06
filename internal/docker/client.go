package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client provides methods to interact with the Docker API via Unix socket.
type Client struct {
	socketPath string
	httpClient *http.Client
	timeout    time.Duration
}

// Container represents a Docker container with relevant information.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Ports   []ContainerPort   `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

// ContainerPort represents a port mapping for a container.
type ContainerPort struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// ContainerInfo provides a simplified view of container information.
type ContainerInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Image       string   `json:"image"`
	State       string   `json:"state"`
	Status      string   `json:"status"`
	Created     int64    `json:"created"`
	Ports       []string `json:"ports"`
	HealthState string   `json:"health_state"`
}

// ContainerStats contains container statistics.
type ContainerStats struct {
	Running   int `json:"running"`
	Stopped   int `json:"stopped"`
	Unhealthy int `json:"unhealthy"`
	Total     int `json:"total"`
}

// DockerError represents an error from the Docker API.
type DockerError struct {
	StatusCode int
	Message    string
}

func (e *DockerError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("docker api error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("docker api error (status %d)", e.StatusCode)
}

// NewClient creates a new Docker client that connects via Unix socket.
// The socketPath should typically be "/var/run/docker.sock".
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
					return net.DialTimeout("unix", socketPath, 5*time.Second)
				},
			},
			Timeout: 30 * time.Second,
		},
		timeout: 30 * time.Second,
	}
}

// WithTimeout sets a custom timeout for API requests.
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
	return c
}

// Ping checks if Docker is available and reachable.
// Returns nil if Docker is available, or an error if not.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/_ping", nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("docker not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// IsAvailable checks if Docker is available without returning an error.
func (c *Client) IsAvailable(ctx context.Context) bool {
	return c.Ping(ctx) == nil
}

// ListContainers returns all containers (both running and stopped).
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=true", nil)
	if err != nil {
		return nil, fmt.Errorf("creating list request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var containers []Container
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("parsing containers: %w", err)
	}

	// Convert to ContainerInfo
	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		result = append(result, containerToInfo(c))
	}

	// Sort by name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// GetContainer returns information about a specific container by name or ID.
func (c *Client) GetContainer(ctx context.Context, nameOrID string) (*ContainerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://docker/containers/%s/json", nameOrID), nil)
	if err != nil {
		return nil, fmt.Errorf("creating inspect request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var inspectData struct {
		ID      string `json:"Id"`
		Name    string `json:"Name"`
		Created string `json:"Created"`
		State   struct {
			Status  string `json:"Status"`
			Running bool   `json:"Running"`
			Health  *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}

	if err := json.Unmarshal(body, &inspectData); err != nil {
		return nil, fmt.Errorf("parsing container: %w", err)
	}

	// Extract ports
	var ports []string
	for containerPort, bindings := range inspectData.NetworkSettings.Ports {
		for _, binding := range bindings {
			if binding.HostPort != "" {
				ports = append(ports, fmt.Sprintf("%s:%s->%s", binding.HostIP, binding.HostPort, containerPort))
			}
		}
	}

	healthState := ""
	if inspectData.State.Health != nil {
		healthState = inspectData.State.Health.Status
	}

	info := &ContainerInfo{
		ID:          inspectData.ID[:12],
		Name:        strings.TrimPrefix(inspectData.Name, "/"),
		Image:       inspectData.Config.Image,
		State:       inspectData.State.Status,
		Status:      inspectData.State.Status,
		Ports:       ports,
		HealthState: healthState,
	}

	return info, nil
}

// GetContainerStats returns aggregate stats about all containers.
func (c *Client) GetContainerStats(ctx context.Context) (*ContainerStats, error) {
	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	stats := &ContainerStats{
		Total: len(containers),
	}

	for _, container := range containers {
		switch container.State {
		case "running":
			if container.HealthState == "unhealthy" {
				stats.Unhealthy++
			} else {
				stats.Running++
			}
		default:
			stats.Stopped++
		}
	}

	return stats, nil
}

// FindContainerByPort finds containers that expose a specific port.
// Useful for matching reverse proxy targets to containers.
func (c *Client) FindContainerByPort(ctx context.Context, port int) ([]ContainerInfo, error) {
	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	var matches []ContainerInfo
	portStr := fmt.Sprintf(":%d", port)

	for _, container := range containers {
		for _, p := range container.Ports {
			// Check if port matches in any port mapping
			if strings.Contains(p, portStr) {
				matches = append(matches, container)
				break
			}
		}
	}

	return matches, nil
}

// parseError extracts error information from an HTTP response.
func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as JSON error
	var jsonErr struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &jsonErr); err == nil && jsonErr.Message != "" {
		return &DockerError{
			StatusCode: resp.StatusCode,
			Message:    jsonErr.Message,
		}
	}

	// Use body as message if it's not empty
	msg := strings.TrimSpace(string(body))
	return &DockerError{
		StatusCode: resp.StatusCode,
		Message:    msg,
	}
}

// ProxyTarget represents a parsed reverse proxy target with host and port.
type ProxyTarget struct {
	Host    string
	Port    int
	RawAddr string
}

// ParseProxyTarget parses a reverse proxy target address into host and port.
// Supports formats like "http://hostname:port", "hostname:port", ":port", etc.
func ParseProxyTarget(target string) *ProxyTarget {
	if target == "" {
		return nil
	}

	pt := &ProxyTarget{RawAddr: target}

	// Remove protocol prefix
	addr := target
	if strings.HasPrefix(addr, "http://") {
		addr = strings.TrimPrefix(addr, "http://")
	} else if strings.HasPrefix(addr, "https://") {
		addr = strings.TrimPrefix(addr, "https://")
	}

	// Remove any path
	if idx := strings.Index(addr, "/"); idx > 0 {
		addr = addr[:idx]
	}

	// Check for port
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		pt.Host = addr[:idx]
		if port, err := strconv.Atoi(addr[idx+1:]); err == nil {
			pt.Port = port
		}
	} else {
		pt.Host = addr
		// Default ports
		if strings.HasPrefix(target, "https://") {
			pt.Port = 443
		} else {
			pt.Port = 80
		}
	}

	return pt
}

// FindContainerForTarget attempts to find a container that matches a proxy target.
// It checks both by port and by container name matching the host.
func (c *Client) FindContainerForTarget(ctx context.Context, target *ProxyTarget) (*ContainerInfo, error) {
	if target == nil {
		return nil, nil
	}

	containers, err := c.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	// Try to match by container name first (exact match)
	for _, container := range containers {
		if container.Name == target.Host {
			return &container, nil
		}
	}

	// Try to match by port if target has a port specified
	if target.Port > 0 {
		portStr := fmt.Sprintf(":%d", target.Port)
		for _, container := range containers {
			for _, p := range container.Ports {
				if strings.Contains(p, portStr) {
					return &container, nil
				}
			}
		}
	}

	return nil, nil
}

// containerToInfo converts a Container to ContainerInfo.
func containerToInfo(c Container) ContainerInfo {
	name := ""
	if len(c.Names) > 0 {
		// Remove leading slash from container name
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Format ports
	var ports []string
	for _, p := range c.Ports {
		if p.PublicPort > 0 {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
		} else {
			ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}

	return ContainerInfo{
		ID:      c.ID[:12],
		Name:    name,
		Image:   c.Image,
		State:   c.State,
		Status:  c.Status,
		Created: c.Created,
		Ports:   ports,
	}
}
