package dlq

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

type fakeReplayValidator struct {
	info *auth.TokenInfo
	err  error
}

func (v fakeReplayValidator) ValidateWithScopes(context.Context, string, string) (*auth.TokenInfo, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.info, nil
}

func TestReplayHTTPHandlerRequiresBearerToken(t *testing.T) {
	handler := NewReplayHTTPHandler(NewReplayManager(&fakeFallbackReplayer{}, nil, zap.NewNop()), fakeReplayValidator{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, defaultReplayPath, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	handler.HandleReplayFallback(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rr.Code, rr.Body.String())
	}
}

func TestReplayHTTPHandlerRejectsMissingReplayScope(t *testing.T) {
	handler := NewReplayHTTPHandler(NewReplayManager(&fakeFallbackReplayer{}, nil, zap.NewNop()), fakeReplayValidator{
		info: &auth.TokenInfo{TenantID: "tenant-a", ProbeID: "operator-1", Scopes: []string{config.ScopeIngestWrite}},
	}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, defaultReplayPath, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()

	handler.HandleReplayFallback(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "dlq:replay scope required") {
		t.Fatalf("response should explain replay scope requirement: %s", rr.Body.String())
	}
}

func TestReplayHTTPHandlerDryRunUsesTokenTenantAndActorFallback(t *testing.T) {
	replayer := &fakeFallbackReplayer{fileCount: 2, totalSize: 2048}
	handler := NewReplayHTTPHandler(NewReplayManager(replayer, nil, zap.NewNop()), fakeReplayValidator{
		info: &auth.TokenInfo{TenantID: "tenant-a", ProbeID: "operator-1", Scopes: []string{config.ScopeDLQReplay}},
	}, zap.NewNop())
	body := map[string]interface{}{
		"approved_by":     "operator-2",
		"approval_id":     "APPROVAL-20260628-002",
		"reason":          "recover after schema repair",
		"repair_summary":  "fixed malformed event payloads",
		"idempotency_key": "tenant-a:APPROVAL-20260628-002:dry-run",
		"dry_run":         true,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, defaultReplayPath, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()

	handler.HandleReplayFallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	if replayer.replayCalls != 0 {
		t.Fatalf("dry run should not execute replay, calls=%d", replayer.replayCalls)
	}
	var result ReplayResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.TenantID != "tenant-a" || result.RequestedBy != "operator-1" {
		t.Fatalf("tenant/requested_by fallback mismatch: %+v", result)
	}
	if result.Status != ReplayStatusDryRun {
		t.Fatalf("status=%s want %s", result.Status, ReplayStatusDryRun)
	}
}

func TestReplayHTTPHandlerAcceptsAdminWildcardScope(t *testing.T) {
	if !hasReplayScope([]string{config.ScopeAdminAll}) {
		t.Fatalf("admin wildcard should allow dlq replay")
	}
	if !hasReplayScope([]string{"dlq:*"}) {
		t.Fatalf("dlq wildcard should allow dlq replay")
	}
	if hasReplayScope([]string{"ingest:write"}) {
		t.Fatalf("ingest write must not allow dlq replay")
	}
}
