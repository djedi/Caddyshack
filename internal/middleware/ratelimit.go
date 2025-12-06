package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimitConfig holds configuration for rate limiting.
type RateLimitConfig struct {
	// LoginMaxAttempts is the maximum number of login attempts per window.
	LoginMaxAttempts int

	// LoginWindow is the time window for login attempts.
	LoginWindow time.Duration

	// APIMaxRequests is the maximum number of API requests per window.
	APIMaxRequests int

	// APIWindow is the time window for API requests.
	APIWindow time.Duration

	// Enabled controls whether rate limiting is active.
	Enabled bool
}

// DefaultRateLimitConfig returns default rate limiting configuration.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		LoginMaxAttempts: 5,
		LoginWindow:      15 * time.Minute,
		APIMaxRequests:   100,
		APIWindow:        time.Minute,
		Enabled:          true,
	}
}

// RateLimitEntry tracks rate limit state for a single key.
type RateLimitEntry struct {
	Attempts  int
	FirstSeen time.Time
	LockedOut bool
	LockoutUntil time.Time
}

// RateLimitStore provides thread-safe storage for rate limit data.
type RateLimitStore struct {
	mu       sync.RWMutex
	entries  map[string]*RateLimitEntry
	config   *RateLimitConfig
}

// NewRateLimitStore creates a new rate limit store.
func NewRateLimitStore(config *RateLimitConfig) *RateLimitStore {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	store := &RateLimitStore{
		entries: make(map[string]*RateLimitEntry),
		config:  config,
	}
	// Start background cleanup goroutine
	go store.cleanupLoop()
	return store
}

// cleanupLoop periodically removes expired entries.
func (s *RateLimitStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes expired entries.
func (s *RateLimitStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.entries {
		// Remove entries that are past their window and not locked out
		if now.Sub(entry.FirstSeen) > s.config.LoginWindow && !entry.LockedOut {
			delete(s.entries, key)
		}
		// Remove entries whose lockout has expired
		if entry.LockedOut && now.After(entry.LockoutUntil) {
			delete(s.entries, key)
		}
	}
}

// RecordLoginAttempt records a login attempt and returns whether it should be allowed.
// Returns: allowed, remainingAttempts, timeUntilReset
func (s *RateLimitStore) RecordLoginAttempt(key string) (bool, int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	entry, exists := s.entries[key]

	if !exists {
		// First attempt
		s.entries[key] = &RateLimitEntry{
			Attempts:  1,
			FirstSeen: now,
		}
		return true, s.config.LoginMaxAttempts - 1, s.config.LoginWindow
	}

	// Check if locked out
	if entry.LockedOut {
		if now.After(entry.LockoutUntil) {
			// Lockout expired, reset entry
			s.entries[key] = &RateLimitEntry{
				Attempts:  1,
				FirstSeen: now,
			}
			return true, s.config.LoginMaxAttempts - 1, s.config.LoginWindow
		}
		return false, 0, entry.LockoutUntil.Sub(now)
	}

	// Check if window has expired
	if now.Sub(entry.FirstSeen) > s.config.LoginWindow {
		// Window expired, reset entry
		s.entries[key] = &RateLimitEntry{
			Attempts:  1,
			FirstSeen: now,
		}
		return true, s.config.LoginMaxAttempts - 1, s.config.LoginWindow
	}

	// Increment attempts
	entry.Attempts++

	if entry.Attempts >= s.config.LoginMaxAttempts {
		// Lock out the key
		entry.LockedOut = true
		entry.LockoutUntil = now.Add(s.config.LoginWindow)
		return false, 0, s.config.LoginWindow
	}

	remaining := s.config.LoginMaxAttempts - entry.Attempts
	timeLeft := s.config.LoginWindow - now.Sub(entry.FirstSeen)
	return true, remaining, timeLeft
}

// IsLockedOut checks if a key is currently locked out.
func (s *RateLimitStore) IsLockedOut(key string) (bool, time.Duration) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[key]
	if !exists {
		return false, 0
	}

	if !entry.LockedOut {
		return false, 0
	}

	now := time.Now()
	if now.After(entry.LockoutUntil) {
		return false, 0
	}

	return true, entry.LockoutUntil.Sub(now)
}

// ClearLockout removes the lockout for a key (admin use).
func (s *RateLimitStore) ClearLockout(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

// APIRateLimitEntry tracks API rate limit state.
type APIRateLimitEntry struct {
	Requests  int
	WindowStart time.Time
}

// APIRateLimitStore provides thread-safe storage for API rate limits.
type APIRateLimitStore struct {
	mu      sync.RWMutex
	entries map[string]*APIRateLimitEntry
	config  *RateLimitConfig
}

// NewAPIRateLimitStore creates a new API rate limit store.
func NewAPIRateLimitStore(config *RateLimitConfig) *APIRateLimitStore {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	store := &APIRateLimitStore{
		entries: make(map[string]*APIRateLimitEntry),
		config:  config,
	}
	go store.cleanupLoop()
	return store
}

// cleanupLoop periodically removes expired entries.
func (s *APIRateLimitStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes expired entries.
func (s *APIRateLimitStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.entries {
		if now.Sub(entry.WindowStart) > s.config.APIWindow {
			delete(s.entries, key)
		}
	}
}

// RecordAPIRequest records an API request and returns whether it should be allowed.
// Returns: allowed, remainingRequests, timeUntilReset
func (s *APIRateLimitStore) RecordAPIRequest(key string) (bool, int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	entry, exists := s.entries[key]

	if !exists {
		// First request
		s.entries[key] = &APIRateLimitEntry{
			Requests:    1,
			WindowStart: now,
		}
		return true, s.config.APIMaxRequests - 1, s.config.APIWindow
	}

	// Check if window has expired
	if now.Sub(entry.WindowStart) > s.config.APIWindow {
		// Window expired, reset entry
		s.entries[key] = &APIRateLimitEntry{
			Requests:    1,
			WindowStart: now,
		}
		return true, s.config.APIMaxRequests - 1, s.config.APIWindow
	}

	// Increment requests
	entry.Requests++

	if entry.Requests > s.config.APIMaxRequests {
		remaining := s.config.APIWindow - now.Sub(entry.WindowStart)
		return false, 0, remaining
	}

	remaining := s.config.APIMaxRequests - entry.Requests
	timeLeft := s.config.APIWindow - now.Sub(entry.WindowStart)
	return true, remaining, timeLeft
}

// GetRemainingRequests returns the number of remaining API requests for a key.
func (s *APIRateLimitStore) GetRemainingRequests(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[key]
	if !exists {
		return s.config.APIMaxRequests
	}

	now := time.Now()
	if now.Sub(entry.WindowStart) > s.config.APIWindow {
		return s.config.APIMaxRequests
	}

	remaining := s.config.APIMaxRequests - entry.Requests
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RateLimiter provides rate limiting middleware.
type RateLimiter struct {
	loginStore *RateLimitStore
	apiStore   *APIRateLimitStore
	config     *RateLimitConfig

	// OnLockout is called when a lockout occurs.
	// The function receives the IP address and lockout duration.
	OnLockout func(ip string, duration time.Duration)
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	return &RateLimiter{
		loginStore: NewRateLimitStore(config),
		apiStore:   NewAPIRateLimitStore(config),
		config:     config,
	}
}

// SetLockoutCallback sets the callback function for lockout events.
func (r *RateLimiter) SetLockoutCallback(callback func(ip string, duration time.Duration)) {
	r.OnLockout = callback
}

// LoginRateLimit returns middleware that rate limits login attempts.
func (r *RateLimiter) LoginRateLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !r.config.Enabled {
				next.ServeHTTP(w, req)
				return
			}

			ip := getClientIP(req)

			// Check if already locked out
			if locked, remaining := r.loginStore.IsLockedOut(ip); locked {
				w.Header().Set("Retry-After", formatDuration(remaining))
				http.Error(w, "Too many login attempts. Please try again later.", http.StatusTooManyRequests)
				return
			}

			// Only rate limit POST requests (actual login attempts)
			if req.Method != http.MethodPost {
				next.ServeHTTP(w, req)
				return
			}

			allowed, remainingAttempts, resetTime := r.loginStore.RecordLoginAttempt(ip)

			if !allowed {
				// Trigger lockout callback if set
				if r.OnLockout != nil {
					go r.OnLockout(ip, resetTime)
				}

				w.Header().Set("Retry-After", formatDuration(resetTime))
				w.Header().Set("X-RateLimit-Remaining", "0")
				http.Error(w, "Too many login attempts. Please try again later.", http.StatusTooManyRequests)
				return
			}

			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", formatInt(r.config.LoginMaxAttempts))
			w.Header().Set("X-RateLimit-Remaining", formatInt(remainingAttempts))
			w.Header().Set("X-RateLimit-Reset", formatDuration(resetTime))

			next.ServeHTTP(w, req)
		})
	}
}

// APIRateLimit returns middleware that rate limits API requests.
func (r *RateLimiter) APIRateLimit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !r.config.Enabled {
				next.ServeHTTP(w, req)
				return
			}

			// Get rate limit key (prefer user ID/token, fall back to IP)
			key := r.getAPIRateLimitKey(req)

			allowed, remaining, resetTime := r.apiStore.RecordAPIRequest(key)

			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", formatInt(r.config.APIMaxRequests))
			w.Header().Set("X-RateLimit-Remaining", formatInt(remaining))
			w.Header().Set("X-RateLimit-Reset", formatDuration(resetTime))

			if !allowed {
				w.Header().Set("Retry-After", formatDuration(resetTime))
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// getAPIRateLimitKey returns the key to use for API rate limiting.
// It prefers user ID or API token over IP address.
func (r *RateLimiter) getAPIRateLimitKey(req *http.Request) string {
	// Check for user in context (from session auth)
	if user := GetUserFromContext(req.Context()); user != nil {
		return "user:" + formatInt64(user.ID)
	}

	// Check for API token in context
	if token := GetAPITokenFromContext(req.Context()); token != nil {
		return "token:" + formatInt64(token.ID)
	}

	// Fall back to IP address
	return "ip:" + getClientIP(req)
}

// IsLoginLockedOut checks if an IP is locked out from login attempts.
func (r *RateLimiter) IsLoginLockedOut(ip string) (bool, time.Duration) {
	return r.loginStore.IsLockedOut(ip)
}

// ClearLoginLockout clears the login lockout for an IP.
func (r *RateLimiter) ClearLoginLockout(ip string) {
	r.loginStore.ClearLockout(ip)
}

// GetAPIRemainingRequests returns remaining API requests for a key.
func (r *RateLimiter) GetAPIRemainingRequests(key string) int {
	return r.apiStore.GetRemainingRequests(key)
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxy setups)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list (original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = xff[:idx]
		}
		xff = strings.TrimSpace(xff)
		if xff != "" {
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// formatDuration formats a duration in seconds.
func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return formatInt(seconds)
}

// formatInt formats an int as a string.
func formatInt(n int) string {
	return formatInt64(int64(n))
}

// formatInt64 formats an int64 as a string.
func formatInt64(n int64) string {
	// Simple implementation without importing strconv
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	buf := make([]byte, 20)
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}

	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
