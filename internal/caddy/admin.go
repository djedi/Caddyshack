package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AdminClient provides methods to interact with the Caddy Admin API.
type AdminClient struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// CaddyStatus represents the status information from Caddy.
type CaddyStatus struct {
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
}

// AdminError represents an error response from the Caddy Admin API.
type AdminError struct {
	StatusCode int
	Message    string
}

func (e *AdminError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("caddy admin api error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("caddy admin api error (status %d)", e.StatusCode)
}

// NewAdminClient creates a new AdminClient with the given base URL.
// The baseURL should be something like "http://localhost:2019".
func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		timeout: 30 * time.Second,
	}
}

// WithTimeout sets a custom timeout for API requests.
func (c *AdminClient) WithTimeout(timeout time.Duration) *AdminClient {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
	return c
}

// WithHTTPClient sets a custom HTTP client.
func (c *AdminClient) WithHTTPClient(client *http.Client) *AdminClient {
	c.httpClient = client
	return c
}

// Reload loads a new configuration into Caddy from a Caddyfile.
// It POSTs to the /load endpoint with the Caddyfile content.
func (c *AdminClient) Reload(ctx context.Context, caddyfileContent string) error {
	url := c.baseURL + "/load"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(caddyfileContent))
	if err != nil {
		return fmt.Errorf("creating reload request: %w", err)
	}

	// Tell Caddy we're sending a Caddyfile, not JSON
	req.Header.Set("Content-Type", "text/caddyfile")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// ReloadFromFile reloads Caddy configuration from a file path.
// This uses the adapt endpoint to convert Caddyfile to JSON first.
func (c *AdminClient) ReloadFromFile(ctx context.Context, caddyfilePath string) error {
	// Read the Caddyfile content
	reader := NewReader(caddyfilePath)
	content, err := reader.Read()
	if err != nil {
		return fmt.Errorf("reading caddyfile: %w", err)
	}

	return c.Reload(ctx, content)
}

// GetConfig retrieves the current running configuration from Caddy.
// It returns the raw JSON configuration.
func (c *AdminClient) GetConfig(ctx context.Context) (json.RawMessage, error) {
	url := c.baseURL + "/config/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating config request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading config response: %w", err)
	}

	return json.RawMessage(body), nil
}

// GetStatus checks the status of the Caddy server.
// It returns status information including whether Caddy is running.
func (c *AdminClient) GetStatus(ctx context.Context) (*CaddyStatus, error) {
	// Try to hit the config endpoint to check if Caddy is running
	url := c.baseURL + "/config/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &CaddyStatus{Running: false}, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Connection refused or timeout means Caddy is not running/reachable
		return &CaddyStatus{Running: false}, nil
	}
	defer resp.Body.Close()

	// If we get a response, Caddy is running
	status := &CaddyStatus{
		Running: true,
	}

	// Try to get version from the Server header
	serverHeader := resp.Header.Get("Server")
	if serverHeader != "" {
		status.Version = serverHeader
	}

	return status, nil
}

// Ping checks if the Caddy Admin API is reachable.
// It returns nil if Caddy is reachable, or an error if not.
func (c *AdminClient) Ping(ctx context.Context) error {
	url := c.baseURL + "/config/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin api not reachable: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// ValidateConfig validates a Caddyfile configuration via the Admin API.
// It uses the /adapt endpoint to convert the Caddyfile to JSON, which validates it.
// Returns nil if valid, or an error describing the validation failure.
func (c *AdminClient) ValidateConfig(ctx context.Context, caddyfileContent string) error {
	url := c.baseURL + "/adapt"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(caddyfileContent))
	if err != nil {
		return fmt.Errorf("creating validate request: %w", err)
	}

	// Tell Caddy we're sending a Caddyfile
	req.Header.Set("Content-Type", "text/caddyfile")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// Stop gracefully stops the Caddy server.
func (c *AdminClient) Stop(ctx context.Context) error {
	url := c.baseURL + "/stop"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("creating stop request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// parseError extracts error information from an HTTP response.
func (c *AdminClient) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as JSON error
	var jsonErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &jsonErr); err == nil && jsonErr.Error != "" {
		return &AdminError{
			StatusCode: resp.StatusCode,
			Message:    jsonErr.Error,
		}
	}

	// Use body as message if it's not empty
	msg := strings.TrimSpace(string(body))
	return &AdminError{
		StatusCode: resp.StatusCode,
		Message:    msg,
	}
}
