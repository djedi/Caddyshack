package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitStore_RecordLoginAttempt(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 3,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	store := NewRateLimitStore(config)

	// First attempt should be allowed
	allowed, remaining, _ := store.RecordLoginAttempt("192.168.1.1")
	if !allowed {
		t.Error("First attempt should be allowed")
	}
	if remaining != 2 {
		t.Errorf("Expected 2 remaining attempts, got %d", remaining)
	}

	// Second attempt should be allowed
	allowed, remaining, _ = store.RecordLoginAttempt("192.168.1.1")
	if !allowed {
		t.Error("Second attempt should be allowed")
	}
	if remaining != 1 {
		t.Errorf("Expected 1 remaining attempt, got %d", remaining)
	}

	// Third attempt should trigger lockout
	allowed, remaining, _ = store.RecordLoginAttempt("192.168.1.1")
	if allowed {
		t.Error("Third attempt should trigger lockout")
	}
	if remaining != 0 {
		t.Errorf("Expected 0 remaining attempts, got %d", remaining)
	}

	// Fourth attempt should be blocked (locked out)
	allowed, _, _ = store.RecordLoginAttempt("192.168.1.1")
	if allowed {
		t.Error("Fourth attempt should be blocked due to lockout")
	}
}

func TestRateLimitStore_DifferentIPs(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	store := NewRateLimitStore(config)

	// IP1 first attempt
	allowed, _, _ := store.RecordLoginAttempt("192.168.1.1")
	if !allowed {
		t.Error("IP1 first attempt should be allowed")
	}

	// IP2 first attempt (should have its own counter)
	allowed, remaining, _ := store.RecordLoginAttempt("192.168.1.2")
	if !allowed {
		t.Error("IP2 first attempt should be allowed")
	}
	if remaining != 1 {
		t.Errorf("IP2 should have 1 remaining attempt, got %d", remaining)
	}

	// IP1 second attempt (triggers lockout)
	allowed, _, _ = store.RecordLoginAttempt("192.168.1.1")
	if allowed {
		t.Error("IP1 second attempt should trigger lockout")
	}

	// IP2 second attempt should still be allowed (independent)
	allowed, _, _ = store.RecordLoginAttempt("192.168.1.2")
	if allowed {
		t.Error("IP2 second attempt should trigger lockout")
	}
}

func TestRateLimitStore_IsLockedOut(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	store := NewRateLimitStore(config)

	// Initially not locked out
	locked, _ := store.IsLockedOut("192.168.1.1")
	if locked {
		t.Error("Should not be locked out initially")
	}

	// First attempt - allowed
	allowed, _, _ := store.RecordLoginAttempt("192.168.1.1")
	if !allowed {
		t.Error("First attempt should be allowed")
	}

	// Not locked out yet
	locked, _ = store.IsLockedOut("192.168.1.1")
	if locked {
		t.Error("Should not be locked out after first attempt")
	}

	// Second attempt - triggers lockout
	allowed, _, _ = store.RecordLoginAttempt("192.168.1.1")
	if allowed {
		t.Error("Second attempt should trigger lockout")
	}

	// Now should be locked out
	locked, remaining := store.IsLockedOut("192.168.1.1")
	if !locked {
		t.Error("Should be locked out after max attempts")
	}
	if remaining <= 0 {
		t.Error("Remaining time should be positive")
	}
}

func TestRateLimitStore_ClearLockout(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	store := NewRateLimitStore(config)

	// Trigger lockout (need 2 attempts to lock out)
	store.RecordLoginAttempt("192.168.1.1")
	store.RecordLoginAttempt("192.168.1.1")

	// Should be locked out
	locked, _ := store.IsLockedOut("192.168.1.1")
	if !locked {
		t.Error("Should be locked out")
	}

	// Clear lockout
	store.ClearLockout("192.168.1.1")

	// Should no longer be locked out
	locked, _ = store.IsLockedOut("192.168.1.1")
	if locked {
		t.Error("Should not be locked out after clearing")
	}
}

func TestAPIRateLimitStore_RecordAPIRequest(t *testing.T) {
	config := &RateLimitConfig{
		APIMaxRequests: 3,
		APIWindow:      time.Minute,
		Enabled:        true,
	}
	store := NewAPIRateLimitStore(config)

	// First request should be allowed
	allowed, remaining, _ := store.RecordAPIRequest("user:1")
	if !allowed {
		t.Error("First request should be allowed")
	}
	if remaining != 2 {
		t.Errorf("Expected 2 remaining requests, got %d", remaining)
	}

	// Second request should be allowed
	allowed, remaining, _ = store.RecordAPIRequest("user:1")
	if !allowed {
		t.Error("Second request should be allowed")
	}
	if remaining != 1 {
		t.Errorf("Expected 1 remaining request, got %d", remaining)
	}

	// Third request should be allowed
	allowed, remaining, _ = store.RecordAPIRequest("user:1")
	if !allowed {
		t.Error("Third request should be allowed")
	}
	if remaining != 0 {
		t.Errorf("Expected 0 remaining requests, got %d", remaining)
	}

	// Fourth request should be blocked
	allowed, _, _ = store.RecordAPIRequest("user:1")
	if allowed {
		t.Error("Fourth request should be blocked")
	}
}

func TestAPIRateLimitStore_GetRemainingRequests(t *testing.T) {
	config := &RateLimitConfig{
		APIMaxRequests: 5,
		APIWindow:      time.Minute,
		Enabled:        true,
	}
	store := NewAPIRateLimitStore(config)

	// Initially should have max requests
	remaining := store.GetRemainingRequests("user:1")
	if remaining != 5 {
		t.Errorf("Expected 5 remaining, got %d", remaining)
	}

	// After one request
	store.RecordAPIRequest("user:1")
	remaining = store.GetRemainingRequests("user:1")
	if remaining != 4 {
		t.Errorf("Expected 4 remaining, got %d", remaining)
	}
}

func TestLoginRateLimitMiddleware(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	limiter := NewRateLimiter(config)

	handler := limiter.LoginRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// First POST request should be allowed
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("First request expected status 200, got %d", rr.Code)
	}

	// Second POST request should trigger lockout
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Second request expected status 429, got %d", rr.Code)
	}

	// Once locked out, all requests from that IP are blocked (including GET)
	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("GET request after lockout expected status 429, got %d", rr.Code)
	}

	// But requests from a different IP should still be allowed
	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET request from different IP expected status 200, got %d", rr.Code)
	}
}

func TestLoginRateLimitMiddleware_GETNotCounted(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	limiter := NewRateLimiter(config)

	handler := limiter.LoginRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET requests should not count towards the limit
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("GET request %d expected status 200, got %d", i+1, rr.Code)
		}
	}

	// Now POST should still have full attempts available
	req = httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("First POST after GETs expected status 200, got %d", rr.Code)
	}
}

func TestLoginRateLimitMiddleware_Disabled(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 1,
		LoginWindow:      time.Minute,
		Enabled:          false, // Disabled
	}
	limiter := NewRateLimiter(config)

	handler := limiter.LoginRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Multiple requests should all be allowed when disabled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestAPIRateLimitMiddleware(t *testing.T) {
	config := &RateLimitConfig{
		APIMaxRequests: 2,
		APIWindow:      time.Minute,
		Enabled:        true,
	}
	limiter := NewRateLimiter(config)

	handler := limiter.APIRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// First request should be allowed
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("First request expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Errorf("Expected X-RateLimit-Remaining=1, got %s", rr.Header().Get("X-RateLimit-Remaining"))
	}

	// Second request should be allowed
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Second request expected status 200, got %d", rr.Code)
	}

	// Third request should be blocked
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("Third request expected status 429, got %d", rr.Code)
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xffHeader  string
		xriHeader  string
		expected   string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For single",
			remoteAddr: "10.0.0.1:12345",
			xffHeader:  "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For multiple",
			remoteAddr: "10.0.0.1:12345",
			xffHeader:  "192.168.1.1, 10.0.0.2, 10.0.0.3",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			xriHeader:  "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			xffHeader:  "192.168.1.1",
			xriHeader:  "192.168.1.2",
			expected:   "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xffHeader != "" {
				req.Header.Set("X-Forwarded-For", tt.xffHeader)
			}
			if tt.xriHeader != "" {
				req.Header.Set("X-Real-IP", tt.xriHeader)
			}

			ip := getClientIP(req)
			if ip != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestLockoutCallback(t *testing.T) {
	config := &RateLimitConfig{
		LoginMaxAttempts: 2,
		LoginWindow:      time.Minute,
		Enabled:          true,
	}
	limiter := NewRateLimiter(config)

	callbackCalled := false
	var callbackIP string
	var callbackDuration time.Duration

	limiter.SetLockoutCallback(func(ip string, duration time.Duration) {
		callbackCalled = true
		callbackIP = ip
		callbackDuration = duration
	})

	handler := limiter.LoginRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request - allowed, no callback
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Give the goroutine time to execute
	time.Sleep(10 * time.Millisecond)
	if callbackCalled {
		t.Error("Lockout callback should not be called after first attempt")
	}

	// Second request - triggers lockout and callback
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Give the goroutine time to execute
	time.Sleep(10 * time.Millisecond)

	if !callbackCalled {
		t.Error("Lockout callback should have been called")
	}
	if callbackIP != "192.168.1.1" {
		t.Errorf("Expected callback IP 192.168.1.1, got %s", callbackIP)
	}
	if callbackDuration <= 0 {
		t.Error("Callback duration should be positive")
	}
}

func TestFormatInt64(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{123, "123"},
		{-123, "-123"},
		{1000000, "1000000"},
	}

	for _, tt := range tests {
		result := formatInt64(tt.input)
		if result != tt.expected {
			t.Errorf("formatInt64(%d) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}
