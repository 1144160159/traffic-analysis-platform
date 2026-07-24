package notification

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSendChannelFailsClosedAndUsesRequestedProvider(t *testing.T) {
	empty := NewNotificationService(NotifyConfig{RateLimitPerMin: 10}, zap.NewNop())
	if err := empty.SendChannel(context.Background(), "webhook", &AlertInfo{AlertID: "a-1", Severity: "high"}); !errors.Is(err, ErrChannelNotConfigured) {
		t.Fatalf("missing provider error=%v want ErrChannelNotConfigured", err)
	}
	if err := empty.SendChannel(context.Background(), "sms", &AlertInfo{AlertID: "a-2", Severity: "high"}); !errors.Is(err, ErrChannelUnsupported) {
		t.Fatalf("unsupported provider error=%v want ErrChannelUnsupported", err)
	}

	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s want POST", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	service := NewNotificationService(NotifyConfig{WebhookURL: server.URL, RateLimitPerMin: 10}, zap.NewNop())
	if err := service.SendChannel(context.Background(), "webhook", &AlertInfo{AlertID: "a-3", Severity: "high"}); err != nil {
		t.Fatalf("send webhook: %v", err)
	}
	if called != 1 {
		t.Fatalf("called=%d want=1", called)
	}
}

func TestSeverityCheck(t *testing.T) {
	s := NewNotificationService(NotifyConfig{}, zap.NewNop())

	tests := []struct {
		severity, min string
		expected      bool
	}{
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

func TestNormalizeDetectorContracts(t *testing.T) {
	if got := NormalizeSeverity("SEVERITY_CRITICAL", 0); got != "critical" {
		t.Fatalf("protobuf severity=%q want critical", got)
	}
	if got := NormalizeSeverity("port_scan", 0.82); got != "high" {
		t.Fatalf("score fallback severity=%q want high", got)
	}
	if got := NormalizeSeverity("", 82); got != "high" {
		t.Fatalf("percentage score severity=%q want high", got)
	}
	if got := NormalizeAlertType("flow", "port_scan,asset_scope:核心资产"); got != "攻击告警" {
		t.Fatalf("normalized alert type=%q want 攻击告警", got)
	}
}

func TestGovernedNotifyRecordsEveryDeliveryOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	service := NewNotificationService(NotifyConfig{WebhookURL: server.URL, MinSeverity: "low", RateLimitPerMin: 10}, zap.NewNop())
	service.SetChannelResolver(func(context.Context, *AlertInfo) ([]ChannelRoute, error) {
		return []ChannelRoute{{Channel: "webhook", RuleID: "rule-1", TargetName: "安全值班组"}, {Channel: "feishu", RuleID: "rule-2", TargetName: "安全管理组"}}, nil
	})
	var results []DeliveryResult
	service.SetDeliveryRecorder(func(_ context.Context, result DeliveryResult) error {
		results = append(results, result)
		return nil
	})
	err := service.Notify(context.Background(), &AlertInfo{AlertID: "a-governed", TenantID: "default", AlertType: "port_scan", Severity: "SEVERITY_HIGH"})
	if err == nil || !strings.Contains(err.Error(), "feishu") {
		t.Fatalf("Notify error=%v want configured webhook plus failed feishu", err)
	}
	if len(results) != 2 || results[0].Status != "sent" || results[1].Status != "failed" {
		t.Fatalf("delivery results=%+v", results)
	}
}

func TestWebhookProvidersRejectHTTP200BusinessErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":40013,"errmsg":"invalid appid"}`))
	}))
	defer server.Close()
	service := NewNotificationService(NotifyConfig{WechatWebhook: server.URL, RateLimitPerMin: 10}, zap.NewNop())
	err := service.SendChannel(context.Background(), "wechat", &AlertInfo{AlertID: "a-provider", Severity: "high"})
	if err == nil || !strings.Contains(err.Error(), "code=40013") {
		t.Fatalf("business error=%v want provider code", err)
	}
}

func TestWebhookProvidersRequireExplicitBusinessSuccess(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "non-json", body: "accepted"},
		{name: "missing-code", body: `{"message":"accepted"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			service := NewNotificationService(NotifyConfig{WechatWebhook: server.URL, RateLimitPerMin: 10}, zap.NewNop())
			if err := service.SendChannel(context.Background(), "wechat", &AlertInfo{AlertID: "a-explicit", Severity: "high"}); err == nil {
				t.Fatal("ambiguous HTTP 200 provider response must fail closed")
			}
		})
	}
}

func TestRoleDestinationOverridesGlobalWebhook(t *testing.T) {
	globalCalls, roleCalls := 0, 0
	global := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		globalCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer global.Close()
	role := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		roleCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer role.Close()
	service := NewNotificationService(NotifyConfig{WebhookURL: global.URL, RateLimitPerMin: 10}, zap.NewNop())
	alert := &AlertInfo{AlertID: "a-role", Severity: "high", TargetName: "安全运营主管", Destinations: []string{role.URL}}
	if err := service.SendChannel(context.Background(), "webhook", alert); err != nil {
		t.Fatalf("send role destination: %v", err)
	}
	if globalCalls != 0 || roleCalls != 1 {
		t.Fatalf("global_calls=%d role_calls=%d want 0/1", globalCalls, roleCalls)
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
