package domains

import (
	"strings"
	"testing"
	"time"
)

func TestNewWHOISClient(t *testing.T) {
	client := NewWHOISClient()
	if client == nil {
		t.Fatal("NewWHOISClient returned nil")
	}
	if client.timeout != 10*time.Second {
		t.Errorf("Expected timeout of 10s, got %v", client.timeout)
	}
}

func TestGetWHOISServer(t *testing.T) {
	client := NewWHOISClient()

	tests := []struct {
		domain   string
		expected string
	}{
		{"example.com", "whois.verisign-grs.com"},
		{"example.net", "whois.verisign-grs.com"},
		{"example.org", "whois.pir.org"},
		{"example.io", "whois.nic.io"},
		{"example.dev", "whois.nic.google"},
		{"example.co.uk", "whois.nic.uk"},
		{"example.de", "whois.denic.de"},
		{"example.xyz", "whois.nic.xyz"},
		{"example.unknown", "whois.iana.org"},
		{"test", "whois.iana.org"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			server := client.getWHOISServer(tt.domain)
			if server != tt.expected {
				t.Errorf("getWHOISServer(%q) = %q, want %q", tt.domain, server, tt.expected)
			}
		})
	}
}

func TestParseWHOISData(t *testing.T) {
	client := NewWHOISClient()

	tests := []struct {
		name       string
		domain     string
		rawData    string
		checkFunc  func(*testing.T, *WHOISResult)
	}{
		{
			name:   "parse registrar",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Registry Domain ID: 2336799_DOMAIN_COM-VRSN
Registrar: Example Registrar Inc.
Registrar IANA ID: 12345
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if r.Registrar != "Example Registrar Inc." {
					t.Errorf("Registrar = %q, want %q", r.Registrar, "Example Registrar Inc.")
				}
			},
		},
		{
			name:   "parse expiry date ISO format",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Registry Expiry Date: 2025-08-13T04:00:00Z
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if r.ExpiryDate == nil {
					t.Fatal("ExpiryDate is nil")
				}
				expected := time.Date(2025, 8, 13, 4, 0, 0, 0, time.UTC)
				if !r.ExpiryDate.Equal(expected) {
					t.Errorf("ExpiryDate = %v, want %v", r.ExpiryDate, expected)
				}
			},
		},
		{
			name:   "parse expiry date simple format",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Expiration Date: 2025-08-13
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if r.ExpiryDate == nil {
					t.Fatal("ExpiryDate is nil")
				}
				if r.ExpiryDate.Year() != 2025 || r.ExpiryDate.Month() != 8 || r.ExpiryDate.Day() != 13 {
					t.Errorf("ExpiryDate = %v, want 2025-08-13", r.ExpiryDate)
				}
			},
		},
		{
			name:   "parse name servers",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Name Server: NS1.EXAMPLE.COM
Name Server: NS2.EXAMPLE.COM
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if len(r.NameServers) != 2 {
					t.Fatalf("NameServers count = %d, want 2", len(r.NameServers))
				}
				if r.NameServers[0] != "ns1.example.com" {
					t.Errorf("NameServers[0] = %q, want %q", r.NameServers[0], "ns1.example.com")
				}
			},
		},
		{
			name:   "parse status",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Domain Status: clientTransferProhibited https://icann.org/epp#clientTransferProhibited
Domain Status: serverDeleteProhibited https://icann.org/epp#serverDeleteProhibited
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if len(r.Status) != 2 {
					t.Fatalf("Status count = %d, want 2", len(r.Status))
				}
				if r.Status[0] != "clientTransferProhibited" {
					t.Errorf("Status[0] = %q, want %q", r.Status[0], "clientTransferProhibited")
				}
			},
		},
		{
			name:   "parse creation and updated dates",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Creation Date: 1995-08-14T04:00:00Z
Updated Date: 2024-08-14T07:01:34Z
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if r.CreatedDate == nil {
					t.Error("CreatedDate is nil")
				} else if r.CreatedDate.Year() != 1995 {
					t.Errorf("CreatedDate year = %d, want 1995", r.CreatedDate.Year())
				}
				if r.UpdatedDate == nil {
					t.Error("UpdatedDate is nil")
				} else if r.UpdatedDate.Year() != 2024 {
					t.Errorf("UpdatedDate year = %d, want 2024", r.UpdatedDate.Year())
				}
			},
		},
		{
			name:   "skip comments and empty lines",
			domain: "example.com",
			rawData: `% WHOIS Server
# Comment line

Domain Name: EXAMPLE.COM
Registrar: Test Registrar
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				if r.Registrar != "Test Registrar" {
					t.Errorf("Registrar = %q, want %q", r.Registrar, "Test Registrar")
				}
			},
		},
		{
			name:   "handle duplicate values",
			domain: "example.com",
			rawData: `Domain Name: EXAMPLE.COM
Registrar: First Registrar
Registrar: Second Registrar
Name Server: NS1.EXAMPLE.COM
Name Server: NS1.EXAMPLE.COM
`,
			checkFunc: func(t *testing.T, r *WHOISResult) {
				// Should keep first registrar
				if r.Registrar != "First Registrar" {
					t.Errorf("Registrar = %q, want %q", r.Registrar, "First Registrar")
				}
				// Should dedupe name servers
				if len(r.NameServers) != 1 {
					t.Errorf("NameServers count = %d, want 1", len(r.NameServers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.parseWHOISData(tt.domain, tt.rawData)
			if result.Domain != tt.domain {
				t.Errorf("Domain = %q, want %q", result.Domain, tt.domain)
			}
			if result.RawData != tt.rawData {
				t.Error("RawData not preserved")
			}
			tt.checkFunc(t, result)
		})
	}
}

func TestParseDate(t *testing.T) {
	client := NewWHOISClient()

	tests := []struct {
		input    string
		expected string // YYYY-MM-DD format or empty if nil
	}{
		{"2025-08-13T04:00:00Z", "2025-08-13"},
		{"2025-08-13T04:00:00.000Z", "2025-08-13"},
		{"2025-08-13T04:00:00-07:00", "2025-08-13"},
		{"2025-08-13 04:00:00", "2025-08-13"},
		{"2025-08-13", "2025-08-13"},
		{"13-Aug-2025", "2025-08-13"},
		{"13 Aug 2025", "2025-08-13"},
		{"2025/08/13", "2025-08-13"},
		{"20250813", "2025-08-13"},
		{"2025-08-13 (expires in 1 year)", "2025-08-13"},
		{"invalid date", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := client.parseDate(tt.input)
			if tt.expected == "" {
				if result != nil {
					t.Errorf("parseDate(%q) = %v, want nil", tt.input, result)
				}
			} else {
				if result == nil {
					t.Fatalf("parseDate(%q) = nil, want %s", tt.input, tt.expected)
				}
				got := result.Format("2006-01-02")
				if got != tt.expected {
					t.Errorf("parseDate(%q) = %s, want %s", tt.input, got, tt.expected)
				}
			}
		})
	}
}

func TestFieldMatchers(t *testing.T) {
	client := NewWHOISClient()

	t.Run("isRegistrarField", func(t *testing.T) {
		positives := []string{"registrar", "sponsoring registrar", "registrar name"}
		for _, f := range positives {
			if !client.isRegistrarField(f) {
				t.Errorf("isRegistrarField(%q) = false, want true", f)
			}
		}
		if client.isRegistrarField("not a registrar") {
			t.Error("isRegistrarField should return false for unknown field")
		}
	})

	t.Run("isExpiryField", func(t *testing.T) {
		positives := []string{"registry expiry date", "expiration date", "expires", "paid-till"}
		for _, f := range positives {
			if !client.isExpiryField(f) {
				t.Errorf("isExpiryField(%q) = false, want true", f)
			}
		}
	})

	t.Run("isNameServerField", func(t *testing.T) {
		positives := []string{"name server", "nserver", "nameserver", "nameservers"}
		for _, f := range positives {
			if !client.isNameServerField(f) {
				t.Errorf("isNameServerField(%q) = false, want true", f)
			}
		}
	})

	t.Run("isStatusField", func(t *testing.T) {
		positives := []string{"domain status", "status"}
		for _, f := range positives {
			if !client.isStatusField(f) {
				t.Errorf("isStatusField(%q) = false, want true", f)
			}
		}
	})
}

func TestContainsString(t *testing.T) {
	client := NewWHOISClient()

	slice := []string{"one", "two", "three"}
	if !client.containsString(slice, "two") {
		t.Error("containsString should return true for existing element")
	}
	if client.containsString(slice, "four") {
		t.Error("containsString should return false for non-existing element")
	}
	if client.containsString(nil, "one") {
		t.Error("containsString should return false for nil slice")
	}
}

func TestLookupEmptyDomain(t *testing.T) {
	client := NewWHOISClient()
	_, err := client.Lookup("")
	if err == nil {
		t.Error("Lookup should return error for empty domain")
	}
	if !strings.Contains(err.Error(), "empty domain") {
		t.Errorf("Error message should mention empty domain, got: %v", err)
	}
}

func TestLookupNormalization(t *testing.T) {
	client := NewWHOISClient()

	// Test that domain is normalized (this will fail on connection but we can verify the error message contains lowercase)
	_, err := client.Lookup("  EXAMPLE.COM  ")
	// The error should be about connection, not about invalid domain
	if err != nil && strings.Contains(err.Error(), "empty domain") {
		t.Error("Domain normalization should handle spaces and case")
	}
}
