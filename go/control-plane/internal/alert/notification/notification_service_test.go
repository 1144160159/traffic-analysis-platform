package notification

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRateLimiter(t *testing.T) {
	limiter := newRateLimiter(3)

	for i := 0; i < 3; i++ {
		if !limiter.Allow() {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if limiter.Allow() {
		t.Error("4th request should be rate-limited")
	}
}

func TestSeverityCheck(t *testing.T) {
	s := NewNotificationService(NotifyConfig{}, zap.NewNop())

	tests := []struct{ severity, min string; expected bool }{
		{"critical", "high", true},
		{"high", "high", true},
		{"high", "critical", false},
		{"medium", "high", false},
		{"low", "high", false},
	}
	for _, tc := range tests {
		result := s.isSeverityAtLeast(tc.severity, tc.min)
		if result != tc.expected {
			t.Errorf("isSeverityAtLeast(%s, %s) = %v, want %v",
				tc.severity, tc.min, result, tc.expected)
		}
	}
}

func TestShouldNotify(t *testing.T) {
	s := NewNotificationService(NotifyConfig{MinSeverity: "high"}, zap.NewNop())

	if !s.shouldNotify(&AlertInfo{Severity: "critical"}) {
		t.Error("critical should always notify")
	}
	if !s.shouldNotify(&AlertInfo{Severity: "high"}) {
		t.Error("high should notify when min=high")
	}
	if s.shouldNotify(&AlertInfo{Severity: "medium"}) {
		t.Error("medium should not notify when min=high")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := NotifyConfig{}
	if cfg.MinSeverity != "" {
		t.Log("default MinSeverity is empty (all levels)")
	}
	if cfg.RateLimitPerMin != 0 {
		t.Log("default RateLimitPerMin is 0 (no limit)")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	limiter := newRateLimiter(1)
	if !limiter.Allow() {
		t.Fatal("first request should be allowed")
	}
	if limiter.Allow() {
		t.Fatal("second request should be blocked")
	}
	// Simulate window reset by clearing
	limiter.mu.Lock()
	limiter.window = []time.Time{}
	limiter.mu.Unlock()
	if !limiter.Allow() {
		t.Fatal("after reset, request should be allowed")
	}
}
