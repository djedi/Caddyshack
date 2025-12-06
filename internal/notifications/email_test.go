package notifications

import (
	"strings"
	"testing"
	"time"
)

func TestEmailSender_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config EmailConfig
		want   bool
	}{
		{
			name: "fully configured",
			config: EmailConfig{
				Enabled:     true,
				SMTPHost:    "smtp.example.com",
				FromAddress: "from@example.com",
				ToAddresses: []string{"to@example.com"},
			},
			want: true,
		},
		{
			name: "disabled",
			config: EmailConfig{
				Enabled:     false,
				SMTPHost:    "smtp.example.com",
				FromAddress: "from@example.com",
				ToAddresses: []string{"to@example.com"},
			},
			want: false,
		},
		{
			name: "missing SMTP host",
			config: EmailConfig{
				Enabled:     true,
				SMTPHost:    "",
				FromAddress: "from@example.com",
				ToAddresses: []string{"to@example.com"},
			},
			want: false,
		},
		{
			name: "missing from address",
			config: EmailConfig{
				Enabled:     true,
				SMTPHost:    "smtp.example.com",
				FromAddress: "",
				ToAddresses: []string{"to@example.com"},
			},
			want: false,
		},
		{
			name: "missing to addresses",
			config: EmailConfig{
				Enabled:     true,
				SMTPHost:    "smtp.example.com",
				FromAddress: "from@example.com",
				ToAddresses: []string{},
			},
			want: false,
		},
		{
			name: "nil to addresses",
			config: EmailConfig{
				Enabled:     true,
				SMTPHost:    "smtp.example.com",
				FromAddress: "from@example.com",
				ToAddresses: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := NewEmailSender(tt.config)
			if got := sender.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailSender_buildSubject(t *testing.T) {
	sender := NewEmailSender(EmailConfig{})

	tests := []struct {
		name     string
		notif    *Notification
		expected string
	}{
		{
			name: "error severity",
			notif: &Notification{
				Severity: SeverityError,
				Title:    "Certificate Expired",
			},
			expected: "[ERROR] Caddyshack: Certificate Expired",
		},
		{
			name: "critical severity",
			notif: &Notification{
				Severity: SeverityCritical,
				Title:    "Certificate Expiring Soon",
			},
			expected: "[CRITICAL] Caddyshack: Certificate Expiring Soon",
		},
		{
			name: "warning severity",
			notif: &Notification{
				Severity: SeverityWarning,
				Title:    "Certificate Expiring",
			},
			expected: "[WARNING] Caddyshack: Certificate Expiring",
		},
		{
			name: "info severity",
			notif: &Notification{
				Severity: SeverityInfo,
				Title:    "System Update",
			},
			expected: "[INFO] Caddyshack: System Update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sender.buildSubject(tt.notif)
			if got != tt.expected {
				t.Errorf("buildSubject() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEmailSender_buildTextBody(t *testing.T) {
	sender := NewEmailSender(EmailConfig{})

	notif := &Notification{
		Type:      TypeCertExpiry,
		Severity:  SeverityCritical,
		Title:     "Certificate Expiring",
		Message:   "The certificate for example.com expires in 7 days.",
		CreatedAt: time.Date(2025, 12, 5, 10, 30, 0, 0, time.UTC),
		Data:      `{"domain":"example.com"}`,
	}

	body := sender.buildTextBody(notif)

	// Check that key parts are present
	if !strings.Contains(body, "Type: cert_expiry") {
		t.Error("Text body should contain the notification type")
	}
	if !strings.Contains(body, "Severity: critical") {
		t.Error("Text body should contain the severity")
	}
	if !strings.Contains(body, "Title: Certificate Expiring") {
		t.Error("Text body should contain the title")
	}
	if !strings.Contains(body, "The certificate for example.com expires in 7 days.") {
		t.Error("Text body should contain the message")
	}
}

func TestEmailSender_buildHTMLBody(t *testing.T) {
	sender := NewEmailSender(EmailConfig{})

	notif := &Notification{
		Type:      TypeCertExpiry,
		Severity:  SeverityError,
		Title:     "Certificate Expired",
		Message:   "The certificate for example.com has expired.",
		CreatedAt: time.Date(2025, 12, 5, 10, 30, 0, 0, time.UTC),
	}

	body, err := sender.buildHTMLBody(notif)
	if err != nil {
		t.Fatalf("buildHTMLBody() error = %v", err)
	}

	// Check that key parts are present
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("HTML body should contain DOCTYPE")
	}
	if !strings.Contains(body, "Certificate Expired") {
		t.Error("HTML body should contain the title")
	}
	if !strings.Contains(body, "Error") {
		t.Error("HTML body should contain severity label")
	}
	if !strings.Contains(body, "#dc2626") {
		t.Error("HTML body should contain error severity color")
	}
	if !strings.Contains(body, "Certificate Expiry") {
		t.Error("HTML body should contain formatted type label")
	}
}

func TestShouldSendEmail(t *testing.T) {
	tests := []struct {
		name          string
		severity      Severity
		sendOnWarning bool
		want          bool
	}{
		{"error always sends", SeverityError, false, true},
		{"critical always sends", SeverityCritical, false, true},
		{"warning not sent by default", SeverityWarning, false, false},
		{"warning sent when enabled", SeverityWarning, true, true},
		{"info never sent", SeverityInfo, false, false},
		{"info never sent even with warning enabled", SeverityInfo, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notif := &Notification{Severity: tt.severity}
			if got := ShouldSendEmail(notif, tt.sendOnWarning); got != tt.want {
				t.Errorf("ShouldSendEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailConfig_TypeLabels(t *testing.T) {
	sender := NewEmailSender(EmailConfig{})

	tests := []struct {
		notifType     Type
		expectedLabel string
	}{
		{TypeCertExpiry, "Certificate Expiry"},
		{TypeDomainExpiry, "Domain Expiry"},
		{TypeConfigChange, "Configuration Change"},
		{TypeCaddyReload, "Caddy Reload"},
		{TypeContainerDown, "Container Down"},
		{TypeSystem, "System"},
	}

	for _, tt := range tests {
		t.Run(string(tt.notifType), func(t *testing.T) {
			notif := &Notification{
				Type:      tt.notifType,
				Severity:  SeverityInfo,
				Title:     "Test",
				Message:   "Test message",
				CreatedAt: time.Now(),
			}
			body, err := sender.buildHTMLBody(notif)
			if err != nil {
				t.Fatalf("buildHTMLBody() error = %v", err)
			}
			if !strings.Contains(body, tt.expectedLabel) {
				t.Errorf("HTML body should contain type label %q", tt.expectedLabel)
			}
		})
	}
}

func TestEmailSenderDisabled_SendNotification(t *testing.T) {
	// When disabled, SendNotification should return nil without error
	sender := NewEmailSender(EmailConfig{Enabled: false})

	notif := &Notification{
		Type:      TypeCertExpiry,
		Severity:  SeverityCritical,
		Title:     "Test",
		Message:   "Test message",
		CreatedAt: time.Now(),
	}

	err := sender.SendNotification(notif)
	if err != nil {
		t.Errorf("SendNotification() on disabled sender should not return error, got: %v", err)
	}
}
