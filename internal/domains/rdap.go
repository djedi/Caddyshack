package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RDAPClient performs RDAP lookups for domains.
// RDAP (Registration Data Access Protocol) is the modern replacement for WHOIS.
type RDAPClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

// RDAPResponse represents the JSON response from an RDAP server.
type RDAPResponse struct {
	Handle     string       `json:"handle"`
	LDHName    string       `json:"ldhName"`
	Status     []string     `json:"status"`
	Events     []RDAPEvent  `json:"events"`
	Entities   []RDAPEntity `json:"entities"`
	Nameservers []RDAPNameserver `json:"nameservers"`
}

// RDAPEvent represents an event in the RDAP response (registration, expiration, etc.)
type RDAPEvent struct {
	EventAction string `json:"eventAction"`
	EventDate   string `json:"eventDate"`
}

// RDAPEntity represents an entity in the RDAP response (registrar, registrant, etc.)
type RDAPEntity struct {
	Handle   string        `json:"handle"`
	Roles    []string      `json:"roles"`
	VCardArray interface{} `json:"vcardArray"`
	PublicIDs []struct {
		Type       string `json:"type"`
		Identifier string `json:"identifier"`
	} `json:"publicIds"`
}

// RDAPNameserver represents a nameserver in the RDAP response.
type RDAPNameserver struct {
	LDHName string `json:"ldhName"`
}

// NewRDAPClient creates a new RDAP client with default settings.
func NewRDAPClient() *RDAPClient {
	return &RDAPClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		timeout: 15 * time.Second,
	}
}

// Lookup performs an RDAP lookup for the given domain and returns a WHOISResult.
// This allows the RDAP client to be used interchangeably with the WHOIS client.
func (c *RDAPClient) Lookup(domain string) (*WHOISResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.LookupWithContext(ctx, domain)
}

// LookupWithContext performs an RDAP lookup with a context.
func (c *RDAPClient) LookupWithContext(ctx context.Context, domain string) (*WHOISResult, error) {
	// Normalize domain
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, fmt.Errorf("empty domain")
	}

	// Get the appropriate RDAP server for the TLD
	rdapServer := c.getRDAPServer(domain)
	if rdapServer == "" {
		return nil, fmt.Errorf("no RDAP server found for domain: %s", domain)
	}

	// Perform the lookup
	url := rdapServer + "/domain/" + domain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/rdap+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RDAP query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("domain not found: %s", domain)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RDAP server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rdapResp RDAPResponse
	if err := json.Unmarshal(body, &rdapResp); err != nil {
		return nil, fmt.Errorf("parsing RDAP response: %w", err)
	}

	// Convert to WHOISResult
	result := c.toWHOISResult(domain, &rdapResp, string(body))
	result.LookupTime = time.Now()

	return result, nil
}

// getRDAPServer returns the RDAP server URL for the given domain's TLD.
func (c *RDAPClient) getRDAPServer(domain string) string {
	// Map of TLDs to their RDAP servers
	// Source: https://data.iana.org/rdap/dns.json
	rdapServers := map[string]string{
		// Generic TLDs
		"com":     "https://rdap.verisign.com/com/v1",
		"net":     "https://rdap.verisign.com/net/v1",
		"org":     "https://rdap.publicinterestregistry.org/rdap",
		"info":    "https://rdap.afilias.net/rdap/info",
		"biz":     "https://rdap.identitydigital.services/rdap",
		"mobi":    "https://rdap.identitydigital.services/rdap",
		"name":    "https://rdap.verisign.com/name/v1",
		"pro":     "https://rdap.identitydigital.services/rdap",

		// New gTLDs (Google)
		"dev":     "https://rdap.nic.google",
		"app":     "https://rdap.nic.google",
		"page":    "https://rdap.nic.google",
		"how":     "https://rdap.nic.google",
		"soy":     "https://rdap.nic.google",
		"new":     "https://rdap.nic.google",

		// New gTLDs (Donuts/Identity Digital)
		"io":      "https://rdap.identitydigital.services/rdap",
		"co":      "https://rdap.identitydigital.services/rdap",
		"me":      "https://rdap.identitydigital.services/rdap",
		"xyz":     "https://rdap.identitydigital.services/rdap",
		"site":    "https://rdap.identitydigital.services/rdap",
		"online":  "https://rdap.identitydigital.services/rdap",
		"tech":    "https://rdap.identitydigital.services/rdap",
		"store":   "https://rdap.identitydigital.services/rdap",
		"cloud":   "https://rdap.identitydigital.services/rdap",
		"live":    "https://rdap.identitydigital.services/rdap",
		"agency":  "https://rdap.identitydigital.services/rdap",
		"digital": "https://rdap.identitydigital.services/rdap",
		"email":   "https://rdap.identitydigital.services/rdap",
		"network": "https://rdap.identitydigital.services/rdap",
		"systems": "https://rdap.identitydigital.services/rdap",
		"services":"https://rdap.identitydigital.services/rdap",
		"software":"https://rdap.identitydigital.services/rdap",
		"solutions":"https://rdap.identitydigital.services/rdap",

		// ccTLDs
		"uk":      "https://rdap.nominet.uk/uk",
		"de":      "https://rdap.denic.de",
		"eu":      "https://rdap.eurid.eu",
		"fr":      "https://rdap.nic.fr",
		"nl":      "https://rdap.sidn.nl",
		"ca":      "https://rdap.ca.fury.ca/rdap",
		"au":      "https://rdap.auda.org.au",
		"nz":      "https://rdap.nzrs.net.nz",
		"jp":      "https://rdap.jprs.jp/rdap",
		"br":      "https://rdap.registro.br",
		"ch":      "https://rdap.nic.ch",
		"se":      "https://rdap.iis.se",
		"no":      "https://rdap.norid.no",
		"fi":      "https://rdap.fi",
		"dk":      "https://rdap.dk-hostmaster.dk",
		"cz":      "https://rdap.nic.cz",
		"pl":      "https://rdap.dns.pl",
		"be":      "https://rdap.dns.be",
		"at":      "https://rdap.nic.at",
		"it":      "https://rdap.nic.it",
		"es":      "https://rdap.nic.es",
		"ru":      "https://rdap.tcinet.ru",
		"cc":      "https://rdap.verisign.com/cc/v1",
		"tv":      "https://rdap.verisign.com/tv/v1",
		"us":      "https://rdap.nic.us",
		"ai":      "https://rdap.identitydigital.services/rdap",
	}

	// Extract TLD from domain
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return ""
	}

	// Try compound TLD first (e.g., co.uk)
	if len(parts) >= 3 {
		compoundTLD := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if server, ok := rdapServers[compoundTLD]; ok {
			return server
		}
	}

	// Try simple TLD
	tld := parts[len(parts)-1]
	if server, ok := rdapServers[tld]; ok {
		return server
	}

	return ""
}

// toWHOISResult converts an RDAP response to a WHOISResult.
func (c *RDAPClient) toWHOISResult(domain string, rdap *RDAPResponse, rawData string) *WHOISResult {
	result := &WHOISResult{
		Domain:  domain,
		RawData: rawData,
		Status:  rdap.Status,
	}

	// Parse events
	for _, event := range rdap.Events {
		eventDate := c.parseRDAPDate(event.EventDate)
		if eventDate == nil {
			continue
		}

		switch event.EventAction {
		case "registration":
			result.CreatedDate = eventDate
		case "expiration":
			result.ExpiryDate = eventDate
		case "last changed", "last update of RDAP database":
			if result.UpdatedDate == nil {
				result.UpdatedDate = eventDate
			}
		}
	}

	// Parse entities to find registrar
	for _, entity := range rdap.Entities {
		for _, role := range entity.Roles {
			if role == "registrar" {
				// Try to get registrar name from vcard or handle
				if entity.Handle != "" {
					result.Registrar = entity.Handle
				}
				// Try to get from publicIds (IANA ID)
				for _, pid := range entity.PublicIDs {
					if pid.Type == "IANA Registrar ID" && pid.Identifier != "" {
						if result.Registrar != "" {
							result.Registrar += " (IANA: " + pid.Identifier + ")"
						}
					}
				}
				// Try to extract name from vcardArray
				if name := c.extractVCardName(entity.VCardArray); name != "" {
					result.Registrar = name
				}
				break
			}
		}
	}

	// Parse nameservers
	for _, ns := range rdap.Nameservers {
		if ns.LDHName != "" {
			result.NameServers = append(result.NameServers, strings.ToLower(ns.LDHName))
		}
	}

	return result
}

// parseRDAPDate parses an RFC3339 date string from RDAP.
func (c *RDAPClient) parseRDAPDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}

	// RDAP uses RFC3339 format
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return &t
		}
	}

	return nil
}

// extractVCardName extracts the organization name from a vCard array.
func (c *RDAPClient) extractVCardName(vcardArray interface{}) string {
	// vcardArray is typically: ["vcard", [[...], ["fn", {}, "text", "Name"], ...]]
	arr, ok := vcardArray.([]interface{})
	if !ok || len(arr) < 2 {
		return ""
	}

	properties, ok := arr[1].([]interface{})
	if !ok {
		return ""
	}

	for _, prop := range properties {
		propArr, ok := prop.([]interface{})
		if !ok || len(propArr) < 4 {
			continue
		}

		propName, ok := propArr[0].(string)
		if !ok {
			continue
		}

		if propName == "fn" || propName == "org" {
			if name, ok := propArr[3].(string); ok && name != "" {
				return name
			}
		}
	}

	return ""
}
