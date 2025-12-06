package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookSender_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		configs  []WebhookConfig
		expected bool
	}{
		{
			name:     "no configs",
			configs:  nil,
			expected: false,
		},
		{
			name:     "empty configs",
			configs:  []WebhookConfig{},
			expected: false,
		},
		{
			name: "disabled config",
			configs: []WebhookConfig{
				{URL: "http://example.com/webhook", Enabled: false},
			},
			expected: false,
		},
		{
			name: "enabled but empty URL",
			configs: []WebhookConfig{
				{URL: "", Enabled: true},
			},
			expected: false,
		},
		{
			name: "single enabled config",
			configs: []WebhookConfig{
				{URL: "http://example.com/webhook", Enabled: true},
			},
			expected: true,
		},
		{
			name: "mixed configs",
			configs: []WebhookConfig{
				{URL: "http://example.com/webhook1", Enabled: false},
				{URL: "http://example.com/webhook2", Enabled: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := NewWebhookSender(tt.configs)
			if got := sender.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWebhookSender_EnabledCount(t *testing.T) {
	configs := []WebhookConfig{
		{URL: "http://example.com/webhook1", Enabled: true},
		{URL: "http://example.com/webhook2", Enabled: false},
		{URL: "http://example.com/webhook3", Enabled: true},
		{URL: "", Enabled: true}, // Empty URL shouldn't count
	}

	sender := NewWebhookSender(configs)
	if got := sender.EnabledCount(); got != 2 {
		t.Errorf("EnabledCount() = %v, want 2", got)
	}
}

func TestWebhookSender_SendNotification(t *testing.T) {
	receivedPayload := make(chan *WebhookPayload, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("User-Agent") != "Caddyshack-Webhook/1.0" {
			t.Errorf("Expected User-Agent Caddyshack-Webhook/1.0, got %s", r.Header.Get("User-Agent"))
		}

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode payload: %v", err)
		}
		receivedPayload <- &payload

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configs := []WebhookConfig{
		{URL: server.URL, Enabled: true},
	}

	sender := NewWebhookSender(configs)

	notification := &Notification{
		ID:        123,
		Type:      TypeCertExpiry,
		Severity:  SeverityWarning,
		Title:     "Test Title",
		Message:   "Test Message",
		Data:      `{"domain":"example.com"}`,
		CreatedAt: time.Now(),
	}

	results := sender.SendNotification(notification)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Error != nil {
		t.Errorf("Unexpected error: %v", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}

	select {
	case payload := <-receivedPayload:
		if payload.ID != 123 {
			t.Errorf("Expected ID 123, got %d", payload.ID)
		}
		if payload.Type != string(TypeCertExpiry) {
			t.Errorf("Expected type cert_expiry, got %s", payload.Type)
		}
		if payload.Severity != string(SeverityWarning) {
			t.Errorf("Expected severity warning, got %s", payload.Severity)
		}
		if payload.Title != "Test Title" {
			t.Errorf("Expected title 'Test Title', got %s", payload.Title)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for payload")
	}
}

func TestWebhookSender_CustomHeaders(t *testing.T) {
	receivedHeaders := make(chan http.Header, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders <- r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configs := []WebhookConfig{
		{
			URL:     server.URL,
			Enabled: true,
			Headers: map[string]string{
				"Authorization": "Bearer secret-token",
				"X-Custom":      "custom-value",
			},
		},
	}

	sender := NewWebhookSender(configs)

	notification := &Notification{
		ID:        1,
		Type:      TypeSystem,
		Severity:  SeverityInfo,
		Title:     "Test",
		Message:   "Test",
		CreatedAt: time.Now(),
	}

	sender.SendNotification(notification)

	select {
	case headers := <-receivedHeaders:
		if headers.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("Expected Authorization header, got %s", headers.Get("Authorization"))
		}
		if headers.Get("X-Custom") != "custom-value" {
			t.Errorf("Expected X-Custom header, got %s", headers.Get("X-Custom"))
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for headers")
	}
}

func TestWebhookSender_RetryOnFailure(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configs := []WebhookConfig{
		{URL: server.URL, Enabled: true},
	}

	sender := NewWebhookSender(configs,
		WithMaxRetries(3),
		WithBaseDelay(10*time.Millisecond),
		WithMaxDelay(50*time.Millisecond),
	)

	notification := &Notification{
		ID:        1,
		Type:      TypeSystem,
		Severity:  SeverityInfo,
		Title:     "Test",
		Message:   "Test",
		CreatedAt: time.Now(),
	}

	results := sender.SendNotification(notification)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Error != nil {
		t.Errorf("Unexpected error: %v", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}
	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected server to receive 3 requests, got %d", attempts)
	}
}

func TestWebhookSender_NoRetryOn4xx(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	configs := []WebhookConfig{
		{URL: server.URL, Enabled: true},
	}

	sender := NewWebhookSender(configs,
		WithMaxRetries(3),
		WithBaseDelay(10*time.Millisecond),
	)

	notification := &Notification{
		ID:        1,
		Type:      TypeSystem,
		Severity:  SeverityInfo,
		Title:     "Test",
		Message:   "Test",
		CreatedAt: time.Now(),
	}

	results := sender.SendNotification(notification)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", result.StatusCode)
	}
	// Should not retry on 4xx errors (except 429)
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry on 4xx), got %d", result.Attempts)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected server to receive 1 request, got %d", attempts)
	}
}

func TestWebhookSender_RetryOn429(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configs := []WebhookConfig{
		{URL: server.URL, Enabled: true},
	}

	sender := NewWebhookSender(configs,
		WithMaxRetries(3),
		WithBaseDelay(10*time.Millisecond),
	)

	notification := &Notification{
		ID:        1,
		Type:      TypeSystem,
		Severity:  SeverityInfo,
		Title:     "Test",
		Message:   "Test",
		CreatedAt: time.Now(),
	}

	results := sender.SendNotification(notification)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}
	// Should retry on 429
	if result.Attempts != 2 {
		t.Errorf("Expected 2 attempts (retry on 429), got %d", result.Attempts)
	}
}

func TestWebhookSender_MultipleEndpoints(t *testing.T) {
	var endpoint1Calls, endpoint2Calls int32

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&endpoint1Calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&endpoint2Calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	configs := []WebhookConfig{
		{URL: server1.URL, Enabled: true},
		{URL: server2.URL, Enabled: true},
	}

	sender := NewWebhookSender(configs)

	notification := &Notification{
		ID:        1,
		Type:      TypeSystem,
		Severity:  SeverityInfo,
		Title:     "Test",
		Message:   "Test",
		CreatedAt: time.Now(),
	}

	results := sender.SendNotification(notification)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Error != nil {
			t.Errorf("Unexpected error for %s: %v", result.URL, result.Error)
		}
		if result.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 for %s, got %d", result.URL, result.StatusCode)
		}
	}

	if atomic.LoadInt32(&endpoint1Calls) != 1 {
		t.Errorf("Expected endpoint1 to receive 1 call, got %d", endpoint1Calls)
	}
	if atomic.LoadInt32(&endpoint2Calls) != 1 {
		t.Errorf("Expected endpoint2 to receive 1 call, got %d", endpoint2Calls)
	}
}

func TestShouldSendWebhook(t *testing.T) {
	tests := []struct {
		name        string
		severity    Severity
		minSeverity Severity
		expected    bool
	}{
		{"info >= info", SeverityInfo, SeverityInfo, true},
		{"warning >= info", SeverityWarning, SeverityInfo, true},
		{"critical >= info", SeverityCritical, SeverityInfo, true},
		{"error >= info", SeverityError, SeverityInfo, true},
		{"info >= warning", SeverityInfo, SeverityWarning, false},
		{"warning >= warning", SeverityWarning, SeverityWarning, true},
		{"critical >= warning", SeverityCritical, SeverityWarning, true},
		{"error >= warning", SeverityError, SeverityWarning, true},
		{"info >= critical", SeverityInfo, SeverityCritical, false},
		{"warning >= critical", SeverityWarning, SeverityCritical, false},
		{"critical >= critical", SeverityCritical, SeverityCritical, true},
		{"error >= critical", SeverityError, SeverityCritical, true},
		{"info >= error", SeverityInfo, SeverityError, false},
		{"warning >= error", SeverityWarning, SeverityError, false},
		{"critical >= error", SeverityCritical, SeverityError, false},
		{"error >= error", SeverityError, SeverityError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Notification{Severity: tt.severity}
			if got := ShouldSendWebhook(n, tt.minSeverity); got != tt.expected {
				t.Errorf("ShouldSendWebhook() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNotificationToPayload(t *testing.T) {
	now := time.Now()
	notification := &Notification{
		ID:        42,
		Type:      TypeCertExpiry,
		Severity:  SeverityCritical,
		Title:     "Certificate Expiring",
		Message:   "Certificate for example.com expires in 7 days",
		Data:      `{"domain":"example.com","days_remaining":7}`,
		CreatedAt: now,
	}

	payload := notificationToPayload(notification)

	if payload.ID != 42 {
		t.Errorf("Expected ID 42, got %d", payload.ID)
	}
	if payload.Type != string(TypeCertExpiry) {
		t.Errorf("Expected type %s, got %s", TypeCertExpiry, payload.Type)
	}
	if payload.Severity != string(SeverityCritical) {
		t.Errorf("Expected severity %s, got %s", SeverityCritical, payload.Severity)
	}
	if payload.Title != "Certificate Expiring" {
		t.Errorf("Expected title 'Certificate Expiring', got %s", payload.Title)
	}
	if payload.Message != "Certificate for example.com expires in 7 days" {
		t.Errorf("Unexpected message: %s", payload.Message)
	}
	if payload.Data != `{"domain":"example.com","days_remaining":7}` {
		t.Errorf("Unexpected data: %s", payload.Data)
	}
	if payload.Timestamp != now.Unix() {
		t.Errorf("Expected timestamp %d, got %d", now.Unix(), payload.Timestamp)
	}
}
