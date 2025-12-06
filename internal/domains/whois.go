package domains

import (
	"bufio"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// WHOISResult holds the parsed WHOIS data for a domain.
type WHOISResult struct {
	Domain      string
	Registrar   string
	ExpiryDate  *time.Time
	CreatedDate *time.Time
	UpdatedDate *time.Time
	NameServers []string
	Status      []string
	RawData     string
	LookupTime  time.Time
}

// WHOISClient performs WHOIS lookups for domains.
type WHOISClient struct {
	timeout time.Duration
}

// NewWHOISClient creates a new WHOIS client with default settings.
func NewWHOISClient() *WHOISClient {
	return &WHOISClient{
		timeout: 10 * time.Second,
	}
}

// Lookup performs a WHOIS lookup for the given domain.
func (c *WHOISClient) Lookup(domain string) (*WHOISResult, error) {
	// Normalize domain
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, fmt.Errorf("empty domain")
	}

	// Get the appropriate WHOIS server for the TLD
	whoisServer := c.getWHOISServer(domain)

	// Perform the lookup
	rawData, err := c.queryWHOIS(whoisServer, domain)
	if err != nil {
		return nil, fmt.Errorf("WHOIS query failed: %w", err)
	}

	// Parse the result
	result := c.parseWHOISData(domain, rawData)
	result.LookupTime = time.Now()

	return result, nil
}

// getWHOISServer returns the appropriate WHOIS server for the domain's TLD.
func (c *WHOISClient) getWHOISServer(domain string) string {
	// Map of common TLDs to their WHOIS servers
	tldServers := map[string]string{
		"com":    "whois.verisign-grs.com",
		"net":    "whois.verisign-grs.com",
		"org":    "whois.pir.org",
		"info":   "whois.afilias.net",
		"io":     "whois.nic.io",
		"co":     "whois.nic.co",
		"ai":     "whois.nic.ai",
		"dev":    "whois.nic.google",
		"app":    "whois.nic.google",
		"me":     "whois.nic.me",
		"tv":     "whois.nic.tv",
		"cc":     "ccwhois.verisign-grs.com",
		"us":     "whois.nic.us",
		"uk":     "whois.nic.uk",
		"co.uk":  "whois.nic.uk",
		"de":     "whois.denic.de",
		"eu":     "whois.eu",
		"fr":     "whois.nic.fr",
		"nl":     "whois.domain-registry.nl",
		"ca":     "whois.cira.ca",
		"au":     "whois.auda.org.au",
		"nz":     "whois.srs.net.nz",
		"xyz":    "whois.nic.xyz",
		"site":   "whois.nic.site",
		"online": "whois.nic.online",
		"cloud":  "whois.nic.cloud",
		"tech":   "whois.nic.tech",
	}

	// Extract TLD(s) from domain
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "whois.iana.org"
	}

	// Try compound TLD first (e.g., co.uk)
	if len(parts) >= 3 {
		compoundTLD := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if server, ok := tldServers[compoundTLD]; ok {
			return server
		}
	}

	// Try simple TLD
	tld := parts[len(parts)-1]
	if server, ok := tldServers[tld]; ok {
		return server
	}

	// Default to IANA for unknown TLDs
	return "whois.iana.org"
}

// queryWHOIS performs a raw WHOIS query against the specified server.
func (c *WHOISClient) queryWHOIS(server, domain string) (string, error) {
	addr := server + ":43"

	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return "", fmt.Errorf("failed to connect to %s: %w", server, err)
	}
	defer conn.Close()

	// Set read deadline
	conn.SetDeadline(time.Now().Add(c.timeout))

	// Send query
	query := domain + "\r\n"
	if server == "whois.verisign-grs.com" {
		// Verisign requires a special format
		query = "domain " + domain + "\r\n"
	}
	if _, err := conn.Write([]byte(query)); err != nil {
		return "", fmt.Errorf("failed to send query: %w", err)
	}

	// Read response
	var result strings.Builder
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		result.WriteString(scanner.Text())
		result.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return result.String(), nil
}

// parseWHOISData extracts structured data from raw WHOIS output.
func (c *WHOISClient) parseWHOISData(domain, rawData string) *WHOISResult {
	result := &WHOISResult{
		Domain:  domain,
		RawData: rawData,
	}

	lines := strings.Split(rawData, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into key and value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		if value == "" {
			continue
		}

		switch {
		case c.isRegistrarField(key):
			if result.Registrar == "" {
				result.Registrar = value
			}
		case c.isExpiryField(key):
			if result.ExpiryDate == nil {
				if t := c.parseDate(value); t != nil {
					result.ExpiryDate = t
				}
			}
		case c.isCreatedField(key):
			if result.CreatedDate == nil {
				if t := c.parseDate(value); t != nil {
					result.CreatedDate = t
				}
			}
		case c.isUpdatedField(key):
			if result.UpdatedDate == nil {
				if t := c.parseDate(value); t != nil {
					result.UpdatedDate = t
				}
			}
		case c.isNameServerField(key):
			ns := strings.ToLower(value)
			// Remove any trailing info (like IP addresses)
			if idx := strings.Index(ns, " "); idx > 0 {
				ns = ns[:idx]
			}
			if !c.containsString(result.NameServers, ns) {
				result.NameServers = append(result.NameServers, ns)
			}
		case c.isStatusField(key):
			// Extract status without the URL
			status := value
			if idx := strings.Index(status, " "); idx > 0 {
				status = status[:idx]
			}
			if !c.containsString(result.Status, status) {
				result.Status = append(result.Status, status)
			}
		}
	}

	return result
}

func (c *WHOISClient) isRegistrarField(key string) bool {
	registrarFields := []string{
		"registrar",
		"sponsoring registrar",
		"registrar name",
	}
	for _, field := range registrarFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) isExpiryField(key string) bool {
	expiryFields := []string{
		"registry expiry date",
		"registrar registration expiration date",
		"expiration date",
		"expiry date",
		"expires",
		"paid-till",
		"renewal date",
	}
	for _, field := range expiryFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) isCreatedField(key string) bool {
	createdFields := []string{
		"creation date",
		"created",
		"created date",
		"registration date",
		"domain registered",
	}
	for _, field := range createdFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) isUpdatedField(key string) bool {
	updatedFields := []string{
		"updated date",
		"last updated",
		"last modified",
		"modified",
	}
	for _, field := range updatedFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) isNameServerField(key string) bool {
	nsFields := []string{
		"name server",
		"nserver",
		"nameserver",
		"nameservers",
	}
	for _, field := range nsFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) isStatusField(key string) bool {
	statusFields := []string{
		"domain status",
		"status",
	}
	for _, field := range statusFields {
		if key == field {
			return true
		}
	}
	return false
}

func (c *WHOISClient) containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// parseDate attempts to parse a date string in various formats.
func (c *WHOISClient) parseDate(value string) *time.Time {
	// Common date formats used in WHOIS data
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"02 Jan 2006",
		"January 02 2006",
		"2006/01/02",
		"02/01/2006",
		"20060102",
	}

	// Clean up the value
	value = strings.TrimSpace(value)

	// Try to extract date from common suffixes
	if idx := strings.Index(value, " ("); idx > 0 {
		value = value[:idx]
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return &t
		}
	}

	// Try regex to extract ISO-like dates
	re := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	if matches := re.FindStringSubmatch(value); len(matches) > 1 {
		if t, err := time.Parse("2006-01-02", matches[1]); err == nil {
			return &t
		}
	}

	return nil
}
