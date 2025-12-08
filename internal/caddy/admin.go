package caddy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
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

// CertificateInfo represents information about a TLS certificate.
type CertificateInfo struct {
	Domain        string    `json:"domain"`
	Issuer        string    `json:"issuer"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	Status        string    `json:"status"` // "valid", "expiring", "expired"
	DaysRemaining int       `json:"days_remaining"`
}

// CAInfo represents information about a Certificate Authority.
type CAInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RootCN      string `json:"root_cn"`
	IntCN       string `json:"intermediate_cn"`
	RootCert    string `json:"root_cert,omitempty"`
	IntCert     string `json:"intermediate_cert,omitempty"`
	Provisioned bool   `json:"provisioned"`
}

// GetPKICAInfo retrieves information about a Certificate Authority from Caddy's PKI app.
// The caID is typically "local" for the default internal CA.
func (c *AdminClient) GetPKICAInfo(ctx context.Context, caID string) (*CAInfo, error) {
	url := c.baseURL + "/pki/ca/" + caID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating pki request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	// 404 means PKI is not configured
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading pki response: %w", err)
	}

	var caInfo CAInfo
	if err := json.Unmarshal(body, &caInfo); err != nil {
		return nil, fmt.Errorf("parsing pki response: %w", err)
	}

	caInfo.Provisioned = true
	return &caInfo, nil
}

// GetCertificates extracts certificate information from Caddy's configuration.
// This parses the TLS automation policies to find managed domains and their certificate status.
// Returns a slice of CertificateInfo for all managed domains.
func (c *AdminClient) GetCertificates(ctx context.Context) ([]CertificateInfo, error) {
	configJSON, err := c.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting caddy config: %w", err)
	}

	// Parse the config to extract TLS automation info
	var config struct {
		Apps struct {
			TLS struct {
				Automation struct {
					Policies []struct {
						Subjects []string `json:"subjects"`
						Issuers  []struct {
							Module string `json:"module"`
							CA     string `json:"ca,omitempty"`
						} `json:"issuers"`
					} `json:"policies"`
				} `json:"automation"`
				Certificates struct {
					Automate []string `json:"automate"`
				} `json:"certificates"`
			} `json:"tls"`
			HTTP struct {
				Servers map[string]struct {
					Listen []string `json:"listen"`
					Routes []struct {
						Match []struct {
							Host []string `json:"host"`
						} `json:"match"`
						Terminal bool `json:"terminal"`
					} `json:"routes"`
					AutoHTTPS struct {
						Skip []string `json:"skip"`
					} `json:"automatic_https"`
					TLSConnectionPolicies []struct {
						Match struct {
							SNI []string `json:"sni"`
						} `json:"match"`
					} `json:"tls_connection_policies"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}

	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("parsing caddy config: %w", err)
	}

	// Collect all domains that might have certificates
	domainSet := make(map[string]bool)

	// From TLS automation policies
	for _, policy := range config.Apps.TLS.Automation.Policies {
		for _, subject := range policy.Subjects {
			if subject != "" && !isLocalhost(subject) {
				domainSet[subject] = true
			}
		}
	}

	// From TLS certificates.automate
	for _, domain := range config.Apps.TLS.Certificates.Automate {
		if domain != "" && !isLocalhost(domain) {
			domainSet[domain] = true
		}
	}

	// From HTTP server routes (hosts with TLS)
	for _, server := range config.Apps.HTTP.Servers {
		// Check if server listens on HTTPS ports
		hasHTTPS := false
		for _, listen := range server.Listen {
			if strings.Contains(listen, ":443") || strings.Contains(listen, "https") {
				hasHTTPS = true
				break
			}
		}
		if !hasHTTPS && len(server.TLSConnectionPolicies) == 0 {
			continue
		}

		for _, route := range server.Routes {
			for _, match := range route.Match {
				for _, host := range match.Host {
					if host != "" && !isLocalhost(host) {
						domainSet[host] = true
					}
				}
			}
		}
	}

	// Build certificate info list by probing each domain for TLS certificate details
	certificates := make([]CertificateInfo, 0, len(domainSet))
	for domain := range domainSet {
		cert := c.probeCertificate(ctx, domain)
		certificates = append(certificates, cert)
	}

	return certificates, nil
}

// GetCertificateDetails attempts to get detailed certificate info.
// This is a placeholder that returns basic info - actual implementation
// would need access to Caddy's certificate storage or use TLS connection probing.
func (c *AdminClient) GetCertificateDetails(ctx context.Context, domain string) (*CertificateInfo, error) {
	// The Caddy Admin API doesn't directly expose certificate details
	// In a production implementation, you would either:
	// 1. Read from Caddy's data directory (default: ~/.local/share/caddy/certificates)
	// 2. Make a TLS connection to the domain and inspect the certificate
	// 3. Use an external certificate monitoring service

	return &CertificateInfo{
		Domain: domain,
		Issuer: "Unknown",
		Status: "unknown",
	}, nil
}

// isLocalhost checks if a domain is localhost or a local IP.
func isLocalhost(domain string) bool {
	domain = strings.ToLower(domain)
	return domain == "localhost" ||
		strings.HasPrefix(domain, "127.") ||
		strings.HasPrefix(domain, "192.168.") ||
		strings.HasPrefix(domain, "10.") ||
		domain == "::1" ||
		strings.HasSuffix(domain, ".local") ||
		strings.HasSuffix(domain, ".localhost")
}

// probeCertificate makes a TLS connection to the domain to get certificate details.
func (c *AdminClient) probeCertificate(ctx context.Context, domain string) CertificateInfo {
	cert := CertificateInfo{
		Domain: domain,
		Issuer: "Unknown",
		Status: "unknown",
	}

	// Create a TLS connection with a short timeout
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", domain+":443", &tls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true, // We want to inspect the cert even if it's invalid
	})
	if err != nil {
		// Connection failed - cert might not exist yet or domain unreachable
		return cert
	}
	defer conn.Close()

	// Get the certificate chain
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return cert
	}

	// Use the leaf certificate (first in chain)
	leafCert := certs[0]

	cert.Issuer = leafCert.Issuer.CommonName
	if cert.Issuer == "" && len(leafCert.Issuer.Organization) > 0 {
		cert.Issuer = leafCert.Issuer.Organization[0]
	}
	cert.NotBefore = leafCert.NotBefore
	cert.NotAfter = leafCert.NotAfter

	// Calculate days remaining and status
	now := time.Now()
	cert.DaysRemaining = int(leafCert.NotAfter.Sub(now).Hours() / 24)

	if now.Before(leafCert.NotBefore) {
		cert.Status = "unknown" // Not yet valid
	} else if now.After(leafCert.NotAfter) {
		cert.Status = "expired"
		cert.DaysRemaining = 0
	} else if cert.DaysRemaining <= 30 {
		cert.Status = "expiring"
	} else {
		cert.Status = "valid"
	}

	return cert
}
