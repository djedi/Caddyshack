package notifications

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/djedi/caddyshack/internal/store"
)

// DomainStore is an interface for accessing domain data.
type DomainStore interface {
	ListDomains() ([]store.Domain, error)
}

// DomainChecker checks domain expiry and creates notifications.
type DomainChecker struct {
	notificationCreator NotificationCreator
	store               DomainStore
	checkInterval       time.Duration
	warningThreshold    int // days before expiry to trigger warning (60)
	criticalThreshold   int // days before expiry to trigger critical (14)
	stopCh              chan struct{}
	wg                  sync.WaitGroup
	running             bool
	mu                  sync.Mutex
}

// DomainExpiryData is stored in the notification data field to identify unique domain/threshold combinations.
type DomainExpiryData struct {
	DomainID   int64  `json:"domain_id"`
	DomainName string `json:"domain_name"`
	Threshold  string `json:"threshold"` // "60", "14", "expired"
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// NewDomainChecker creates a new domain checker.
func NewDomainChecker(notificationCreator NotificationCreator, domainStore DomainStore) *DomainChecker {
	return &DomainChecker{
		notificationCreator: notificationCreator,
		store:               domainStore,
		checkInterval:       24 * time.Hour, // Check once per day
		warningThreshold:    60,             // 60 days
		criticalThreshold:   14,             // 14 days
		stopCh:              make(chan struct{}),
	}
}

// WithCheckInterval sets a custom check interval (useful for testing).
func (c *DomainChecker) WithCheckInterval(interval time.Duration) *DomainChecker {
	c.checkInterval = interval
	return c
}

// WithThresholds sets custom warning and critical thresholds in days.
func (c *DomainChecker) WithThresholds(warning, critical int) *DomainChecker {
	c.warningThreshold = warning
	c.criticalThreshold = critical
	return c
}

// Start begins the background domain checking job.
func (c *DomainChecker) Start() {
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

// Stop stops the background domain checking job.
func (c *DomainChecker) Stop() {
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

// run is the main loop for the domain checker.
func (c *DomainChecker) run() {
	defer c.wg.Done()

	// Run an initial check on startup (with a small delay to let things initialize)
	timer := time.NewTimer(15 * time.Second)
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

// CheckAll checks all domains and creates notifications as needed.
func (c *DomainChecker) CheckAll() {
	domains, err := c.store.ListDomains()
	if err != nil {
		log.Printf("Domain checker: failed to list domains: %v", err)
		return
	}

	for _, domain := range domains {
		if err := c.checkDomain(domain); err != nil {
			log.Printf("Domain checker: error checking %s: %v", domain.Name, err)
		}
	}
}

// checkDomain checks a single domain and creates notifications if needed.
func (c *DomainChecker) checkDomain(domain store.Domain) error {
	// Skip if no expiry date is set
	if domain.ExpiryDate == nil {
		return nil
	}

	daysRemaining := int(time.Until(*domain.ExpiryDate).Hours() / 24)

	// Determine which threshold (if any) this domain triggers
	var threshold string
	var severity Severity
	var title, message string

	switch {
	case daysRemaining < 0:
		// Domain has expired
		threshold = "expired"
		severity = SeverityError
		title = fmt.Sprintf("Domain Expired: %s", domain.Name)
		message = fmt.Sprintf("The domain %s expired on %s.",
			domain.Name, domain.ExpiryDate.Format("Jan 02, 2006"))
	case daysRemaining <= c.criticalThreshold:
		// Critical: expires within 14 days
		threshold = "14"
		severity = SeverityCritical
		title = fmt.Sprintf("Domain Expiring Soon: %s", domain.Name)
		message = fmt.Sprintf("The domain %s expires in %d days (on %s). Immediate action required.",
			domain.Name, daysRemaining, domain.ExpiryDate.Format("Jan 02, 2006"))
	case daysRemaining <= c.warningThreshold:
		// Warning: expires within 60 days
		threshold = "60"
		severity = SeverityWarning
		title = fmt.Sprintf("Domain Expiring: %s", domain.Name)
		message = fmt.Sprintf("The domain %s expires in %d days (on %s).",
			domain.Name, daysRemaining, domain.ExpiryDate.Format("Jan 02, 2006"))
	default:
		// Domain is fine, no notification needed
		return nil
	}

	// Create data payload for deduplication
	data := DomainExpiryData{
		DomainID:   domain.ID,
		DomainName: domain.Name,
		Threshold:  threshold,
		ExpiresAt:  domain.ExpiryDate.Format(time.RFC3339),
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling data: %w", err)
	}

	// Check if we already have an unacknowledged notification for this domain/threshold
	exists, err := c.notificationCreator.ExistsUnacknowledged(TypeDomainExpiry, string(dataJSON))
	if err != nil {
		return fmt.Errorf("checking existing notification: %w", err)
	}

	if exists {
		// Already have a notification for this, skip
		return nil
	}

	// Create the notification (this may also send an email if EmailNotifier is used)
	_, err = c.notificationCreator.Create(TypeDomainExpiry, severity, title, message, string(dataJSON))
	if err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}

	log.Printf("Domain checker: created %s notification for %s (expires in %d days)",
		severity, domain.Name, daysRemaining)

	return nil
}

// CheckNow runs an immediate domain check (useful for testing or manual triggers).
func (c *DomainChecker) CheckNow() {
	c.CheckAll()
}
