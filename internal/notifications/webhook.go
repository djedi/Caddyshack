package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"
)

// WebhookConfig holds configuration for a webhook endpoint.
type WebhookConfig struct {
	// URL is the webhook endpoint URL.
	URL string

	// Headers are optional headers to include with each request.
	Headers map[string]string

	// Enabled determines if this webhook is active.
	Enabled bool
}

// WebhookPayload is the JSON payload sent to webhook endpoints.
type WebhookPayload struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Data      string    `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Timestamp int64     `json:"timestamp"`
}

// WebhookResult contains the result of a webhook delivery attempt.
type WebhookResult struct {
	URL        string
	StatusCode int
	Error      error
	Attempts   int
}

// WebhookSender handles sending notifications to webhook endpoints.
type WebhookSender struct {
	configs    []WebhookConfig
	httpClient *http.Client

	// Retry configuration
	maxRetries     int
	baseDelay      time.Duration
	maxDelay       time.Duration

	// For background retries
	mu             sync.Mutex
	pendingRetries []pendingWebhook
}

// pendingWebhook represents a webhook delivery that needs to be retried.
type pendingWebhook struct {
	payload     *WebhookPayload
	config      WebhookConfig
	attempt     int
	retryAfter  time.Time
}

// WebhookSenderOption is a functional option for configuring WebhookSender.
type WebhookSenderOption func(*WebhookSender)

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) WebhookSenderOption {
	return func(s *WebhookSender) {
		s.maxRetries = n
	}
}

// WithBaseDelay sets the base delay for exponential backoff.
func WithBaseDelay(d time.Duration) WebhookSenderOption {
	return func(s *WebhookSender) {
		s.baseDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(d time.Duration) WebhookSenderOption {
	return func(s *WebhookSender) {
		s.maxDelay = d
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) WebhookSenderOption {
	return func(s *WebhookSender) {
		s.httpClient = c
	}
}

// NewWebhookSender creates a new WebhookSender with the given configurations.
func NewWebhookSender(configs []WebhookConfig, opts ...WebhookSenderOption) *WebhookSender {
	s := &WebhookSender{
		configs:    configs,
		maxRetries: 3,
		baseDelay:  1 * time.Second,
		maxDelay:   30 * time.Second,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		pendingRetries: make([]pendingWebhook, 0),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// IsEnabled returns true if at least one webhook is configured and enabled.
func (w *WebhookSender) IsEnabled() bool {
	for _, c := range w.configs {
		if c.Enabled && c.URL != "" {
			return true
		}
	}
	return false
}

// EnabledCount returns the number of enabled webhook endpoints.
func (w *WebhookSender) EnabledCount() int {
	count := 0
	for _, c := range w.configs {
		if c.Enabled && c.URL != "" {
			count++
		}
	}
	return count
}

// notificationToPayload converts a Notification to a WebhookPayload.
func notificationToPayload(n *Notification) *WebhookPayload {
	return &WebhookPayload{
		ID:        n.ID,
		Type:      string(n.Type),
		Severity:  string(n.Severity),
		Title:     n.Title,
		Message:   n.Message,
		Data:      n.Data,
		CreatedAt: n.CreatedAt,
		Timestamp: n.CreatedAt.Unix(),
	}
}

// SendNotification sends a notification to all enabled webhook endpoints.
// Returns a slice of results for each webhook endpoint.
func (w *WebhookSender) SendNotification(n *Notification) []WebhookResult {
	if !w.IsEnabled() {
		return nil
	}

	payload := notificationToPayload(n)
	return w.sendToAll(payload)
}

// sendToAll sends the payload to all enabled webhook endpoints.
func (w *WebhookSender) sendToAll(payload *WebhookPayload) []WebhookResult {
	var results []WebhookResult

	for _, config := range w.configs {
		if !config.Enabled || config.URL == "" {
			continue
		}

		result := w.sendWithRetry(payload, config)
		results = append(results, result)
	}

	return results
}

// sendWithRetry sends the payload to a single webhook with retry logic.
func (w *WebhookSender) sendWithRetry(payload *WebhookPayload, config WebhookConfig) WebhookResult {
	result := WebhookResult{
		URL: config.URL,
	}

	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		result.Attempts = attempt + 1

		statusCode, err := w.send(payload, config)
		result.StatusCode = statusCode
		result.Error = err

		if err == nil && statusCode >= 200 && statusCode < 300 {
			// Success
			return result
		}

		// Don't retry on client errors (4xx) except rate limiting (429)
		if statusCode >= 400 && statusCode < 500 && statusCode != 429 {
			return result
		}

		// Calculate backoff delay
		if attempt < w.maxRetries {
			delay := w.calculateDelay(attempt)
			time.Sleep(delay)
		}
	}

	return result
}

// calculateDelay calculates the delay for exponential backoff.
func (w *WebhookSender) calculateDelay(attempt int) time.Duration {
	delay := time.Duration(float64(w.baseDelay) * math.Pow(2, float64(attempt)))
	if delay > w.maxDelay {
		delay = w.maxDelay
	}
	return delay
}

// send sends the payload to a single webhook endpoint.
func (w *WebhookSender) send(payload *WebhookPayload, config WebhookConfig) (int, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshaling payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Caddyshack-Webhook/1.0")

	// Add custom headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// SendNotificationAsync sends a notification asynchronously and logs any errors.
func (w *WebhookSender) SendNotificationAsync(n *Notification) {
	go func() {
		results := w.SendNotification(n)
		for _, r := range results {
			if r.Error != nil || r.StatusCode < 200 || r.StatusCode >= 300 {
				log.Printf("Webhook delivery failed to %s: status=%d, error=%v, attempts=%d",
					r.URL, r.StatusCode, r.Error, r.Attempts)
			}
		}
	}()
}

// ShouldSendWebhook determines if a webhook should be sent for a notification based on severity.
// By default, webhooks are sent for all notifications.
func ShouldSendWebhook(n *Notification, minSeverity Severity) bool {
	severityOrder := map[Severity]int{
		SeverityInfo:     0,
		SeverityWarning:  1,
		SeverityCritical: 2,
		SeverityError:    3,
	}

	notifLevel, ok1 := severityOrder[n.Severity]
	minLevel, ok2 := severityOrder[minSeverity]

	if !ok1 || !ok2 {
		return true // Default to sending if severity is unknown
	}

	return notifLevel >= minLevel
}

// WebhookNotifier wraps the notification service to send webhooks when notifications are created.
type WebhookNotifier struct {
	*Service
	webhookSender *WebhookSender
	minSeverity   Severity
}

// NewWebhookNotifier creates a notifier that sends webhooks for notifications.
func NewWebhookNotifier(service *Service, webhookSender *WebhookSender, minSeverity Severity) *WebhookNotifier {
	return &WebhookNotifier{
		Service:       service,
		webhookSender: webhookSender,
		minSeverity:   minSeverity,
	}
}

// Create creates a notification and optionally sends webhooks.
func (n *WebhookNotifier) Create(notificationType Type, severity Severity, title, message, data string) (*Notification, error) {
	notif, err := n.Service.Create(notificationType, severity, title, message, data)
	if err != nil {
		return nil, err
	}

	// Send webhook if enabled and severity warrants it
	if n.webhookSender != nil && n.webhookSender.IsEnabled() && ShouldSendWebhook(notif, n.minSeverity) {
		n.webhookSender.SendNotificationAsync(notif)
	}

	return notif, nil
}

// CombinedNotifier wraps both email and webhook notification capabilities.
type CombinedNotifier struct {
	*Service
	emailSender    *EmailSender
	webhookSender  *WebhookSender
	sendOnWarning  bool
	webhookMinSev  Severity
}

// NewCombinedNotifier creates a notifier that can send both email and webhook notifications.
func NewCombinedNotifier(service *Service, emailSender *EmailSender, webhookSender *WebhookSender, sendOnWarning bool, webhookMinSeverity Severity) *CombinedNotifier {
	return &CombinedNotifier{
		Service:       service,
		emailSender:   emailSender,
		webhookSender: webhookSender,
		sendOnWarning: sendOnWarning,
		webhookMinSev: webhookMinSeverity,
	}
}

// Create creates a notification and sends email/webhook notifications as configured.
func (n *CombinedNotifier) Create(notificationType Type, severity Severity, title, message, data string) (*Notification, error) {
	notif, err := n.Service.Create(notificationType, severity, title, message, data)
	if err != nil {
		return nil, err
	}

	// Send email if enabled and severity warrants it
	if n.emailSender != nil && n.emailSender.IsEnabled() && ShouldSendEmail(notif, n.sendOnWarning) {
		if err := n.emailSender.SendNotification(notif); err != nil {
			log.Printf("Failed to send email notification: %v", err)
		}
	}

	// Send webhook if enabled and severity warrants it
	if n.webhookSender != nil && n.webhookSender.IsEnabled() && ShouldSendWebhook(notif, n.webhookMinSev) {
		n.webhookSender.SendNotificationAsync(notif)
	}

	return notif, nil
}
