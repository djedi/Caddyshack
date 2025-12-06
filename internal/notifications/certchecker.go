package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/caddy"
)

// CertificateChecker checks certificate expiry and creates notifications.
type CertificateChecker struct {
	notificationService *Service
	adminClient         *caddy.AdminClient
	checkInterval       time.Duration
	warningThreshold    int // days before expiry to trigger warning
	criticalThreshold   int // days before expiry to trigger critical
	stopCh              chan struct{}
	wg                  sync.WaitGroup
	running             bool
	mu                  sync.Mutex
}

// CertExpiryData is stored in the notification data field to identify unique cert/threshold combinations.
type CertExpiryData struct {
	Domain    string `json:"domain"`
	Threshold string `json:"threshold"` // "30", "7", "expired"
	ExpiresAt string `json:"expires_at,omitempty"`
}

// NewCertificateChecker creates a new certificate checker.
func NewCertificateChecker(notificationService *Service, caddyAdminAPI string) *CertificateChecker {
	return &CertificateChecker{
		notificationService: notificationService,
		adminClient:         caddy.NewAdminClient(caddyAdminAPI),
		checkInterval:       24 * time.Hour, // Check once per day
		warningThreshold:    30,             // 30 days
		criticalThreshold:   7,              // 7 days
		stopCh:              make(chan struct{}),
	}
}

// WithCheckInterval sets a custom check interval (useful for testing).
func (c *CertificateChecker) WithCheckInterval(interval time.Duration) *CertificateChecker {
	c.checkInterval = interval
	return c
}

// WithThresholds sets custom warning and critical thresholds in days.
func (c *CertificateChecker) WithThresholds(warning, critical int) *CertificateChecker {
	c.warningThreshold = warning
	c.criticalThreshold = critical
	return c
}

// Start begins the background certificate checking job.
func (c *CertificateChecker) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	c.wg.Add(1)
	go c.run()
}

// Stop stops the background certificate checking job.
func (c *CertificateChecker) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.mu.Unlock()

	close(c.stopCh)
	c.wg.Wait()
}

// run is the main loop for the certificate checker.
func (c *CertificateChecker) run() {
	defer c.wg.Done()

	// Run an initial check on startup (with a small delay to let things initialize)
	timer := time.NewTimer(10 * time.Second)
	select {
	case <-timer.C:
		c.CheckAll()
	case <-c.stopCh:
		timer.Stop()
		return
	}

	// Then run periodically
	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.CheckAll()
		case <-c.stopCh:
			return
		}
	}
}

// CheckAll checks all certificates and creates notifications as needed.
func (c *CertificateChecker) CheckAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if Caddy is reachable
	status, err := c.adminClient.GetStatus(ctx)
	if err != nil || status == nil || !status.Running {
		log.Println("Certificate checker: Caddy not reachable, skipping check")
		return
	}

	// Get all certificates
	certs, err := c.adminClient.GetCertificates(ctx)
	if err != nil {
		log.Printf("Certificate checker: failed to get certificates: %v", err)
		return
	}

	for _, cert := range certs {
		if err := c.checkCertificate(cert); err != nil {
			log.Printf("Certificate checker: error checking %s: %v", cert.Domain, err)
		}
	}
}

// checkCertificate checks a single certificate and creates notifications if needed.
func (c *CertificateChecker) checkCertificate(cert caddy.CertificateInfo) error {
	// Skip if we don't have expiry info
	if cert.NotAfter.IsZero() {
		return nil
	}

	daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)

	// Determine which threshold (if any) this certificate triggers
	var threshold string
	var severity Severity
	var title, message string

	switch {
	case daysRemaining < 0:
		// Certificate has expired
		threshold = "expired"
		severity = SeverityError
		title = fmt.Sprintf("Certificate Expired: %s", cert.Domain)
		message = fmt.Sprintf("The certificate for %s expired on %s.",
			cert.Domain, cert.NotAfter.Format("Jan 02, 2006"))
	case daysRemaining <= c.criticalThreshold:
		// Critical: expires within 7 days
		threshold = "7"
		severity = SeverityCritical
		title = fmt.Sprintf("Certificate Expiring Soon: %s", cert.Domain)
		message = fmt.Sprintf("The certificate for %s expires in %d days (on %s). Immediate action required.",
			cert.Domain, daysRemaining, cert.NotAfter.Format("Jan 02, 2006"))
	case daysRemaining <= c.warningThreshold:
		// Warning: expires within 30 days
		threshold = "30"
		severity = SeverityWarning
		title = fmt.Sprintf("Certificate Expiring: %s", cert.Domain)
		message = fmt.Sprintf("The certificate for %s expires in %d days (on %s).",
			cert.Domain, daysRemaining, cert.NotAfter.Format("Jan 02, 2006"))
	default:
		// Certificate is fine, no notification needed
		return nil
	}

	// Create data payload for deduplication
	data := CertExpiryData{
		Domain:    cert.Domain,
		Threshold: threshold,
		ExpiresAt: cert.NotAfter.Format(time.RFC3339),
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling data: %w", err)
	}

	// Check if we already have an unacknowledged notification for this cert/threshold
	exists, err := c.notificationService.ExistsUnacknowledged(TypeCertExpiry, string(dataJSON))
	if err != nil {
		return fmt.Errorf("checking existing notification: %w", err)
	}

	if exists {
		// Already have a notification for this, skip
		return nil
	}

	// Create the notification
	_, err = c.notificationService.Create(TypeCertExpiry, severity, title, message, string(dataJSON))
	if err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}

	log.Printf("Certificate checker: created %s notification for %s (expires in %d days)",
		severity, cert.Domain, daysRemaining)

	return nil
}

// CheckNow runs an immediate certificate check (useful for testing or manual triggers).
func (c *CertificateChecker) CheckNow() {
	c.CheckAll()
}
