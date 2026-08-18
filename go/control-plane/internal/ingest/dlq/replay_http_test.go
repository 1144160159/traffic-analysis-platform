package dlq

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	manager := NewReplayManager(replayer, nil, zap.NewNop())
	approvals := NewMemoryReplayApprovalStore()
	if err := approvals.CreateApproval(context.Background(), ReplayApproval{
		TenantID:    "tenant-a",
		ApprovalID:  "APPROVAL-20260628-002",
		RequestedBy: "operator-1",
		ApprovedBy:  "operator-2",
		Status:      ApprovalStatusApproved,
		Reason:      "test approval",
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	manager.SetApprovalStore(approvals)
	handler := NewReplayHTTPHandler(manager, fakeReplayValidator{
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

func TestReplayHTTPHandlerCreateApprovalRequiresAdminScope(t *testing.T) {
	manager := NewReplayManager(&fakeFallbackReplayer{}, nil, zap.NewNop())
	approvals := NewMemoryReplayApprovalStore()
	manager.SetApprovalStore(approvals)
	handler := NewReplayHTTPHandler(manager, fakeReplayValidator{
		info: &auth.TokenInfo{TenantID: "tenant-a", ProbeID: "operator-1", Scopes: []string{config.ScopeDLQReplay}},
	}, zap.NewNop())
	handler.SetApprovalStore(approvals)

	body, _ := json.Marshal(map[string]interface{}{
		"tenant_id":    "tenant-a",
		"approval_id":  "APPROVAL-20260628-003",
		"requested_by": "analyst-1",
		"approved_by":  "operator-2",
		"reason":       "approve replay",
	})
	req := httptest.NewRequest(http.MethodPost, defaultReplayApprovalPath, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()
	handler.HandleCreateApproval(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}
}

func TestReplayHTTPHandlerCreateApprovalAndReplayRoundTrip(t *testing.T) {
	replayer := &fakeFallbackReplayer{fileCount: 1, totalSize: 512}
	manager := NewReplayManager(replayer, nil, zap.NewNop())
	approvals := NewMemoryReplayApprovalStore()
	manager.SetApprovalStore(approvals)
	handler := NewReplayHTTPHandler(manager, fakeReplayValidator{
		info: &auth.TokenInfo{TenantID: "tenant-a", ProbeID: "operator-1", Scopes: []string{config.ScopeAdminAll}},
	}, zap.NewNop())
	handler.SetApprovalStore(approvals)

	createBody, _ := json.Marshal(map[string]interface{}{
		"approval_id":  "APPROVAL-20260628-004",
		"requested_by": "analyst-1",
		"approved_by":  "operator-1",
		"reason":       "approve replay after repair",
	})
	req := httptest.NewRequest(http.MethodPost, defaultReplayApprovalPath, bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()
	handler.HandleCreateApproval(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d want 201 body=%s", rr.Code, rr.Body.String())
	}

	replayBody, _ := json.Marshal(map[string]interface{}{
		"requested_by":    "analyst-1",
		"approved_by":     "operator-1",
		"approval_id":     "APPROVAL-20260628-004",
		"reason":          "recover after schema repair",
		"repair_summary":  "fixed malformed event payloads",
		"idempotency_key": "tenant-a:APPROVAL-20260628-004:dry-run",
		"dry_run":         true,
	})
	req2 := httptest.NewRequest(http.MethodPost, defaultReplayPath, bytes.NewReader(replayBody))
	req2.Header.Set("Authorization", "Bearer token-1")
	rr2 := httptest.NewRecorder()
	handler.HandleReplayFallback(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("replay status=%d want 200 body=%s", rr2.Code, rr2.Body.String())
	}
}
