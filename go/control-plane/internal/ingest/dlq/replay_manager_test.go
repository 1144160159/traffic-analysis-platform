package dlq

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeFallbackReplayer struct {
	fileCount   int
	totalSize   int64
	replayCalls int
	report      FallbackReplayReport
}

func (f *fakeFallbackReplayer) GetFallbackStats() (int, int64, error) {
	return f.fileCount, f.totalSize, nil
}

func (f *fakeFallbackReplayer) ReplayFallbackFiles(context.Context) FallbackReplayReport {
	f.replayCalls++
	if f.report.StartedAt.IsZero() {
		f.report.StartedAt = time.Unix(1, 0)
	}
	if f.report.FinishedAt.IsZero() {
		f.report.FinishedAt = time.Unix(2, 0)
	}
	return f.report
}

func validReplayRequest() ReplayRequest {
	return ReplayRequest{
		TenantID:       "tenant-a",
		RequestedBy:    "analyst-1",
		ApprovedBy:     "operator-2",
		ApprovalID:     "APPROVAL-20260628-001",
		Reason:         "recover parsed bad messages",
		RepairSummary:  "fixed malformed tenant headers",
		IdempotencyKey: "tenant-a:APPROVAL-20260628-001:1",
	}
}

func TestReplayManagerRequiresManualRepairAndApproval(t *testing.T) {
	manager := NewReplayManager(&fakeFallbackReplayer{}, nil, zap.NewNop())
	req := validReplayRequest()
	req.RepairSummary = ""

	_, err := manager.ReplayFallback(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "repair_summary is required") {
		t.Fatalf("expected repair summary validation error, got %v", err)
	}

	req = validReplayRequest()
	req.ApprovedBy = req.RequestedBy
	_, err = manager.ReplayFallback(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "approved_by must be different") {
		t.Fatalf("expected self-approval validation error, got %v", err)
	}
}

func TestReplayManagerDryRunDoesNotReplay(t *testing.T) {
	replayer := &fakeFallbackReplayer{fileCount: 2, totalSize: 2048}
	manager := NewReplayManager(replayer, nil, zap.NewNop())
	req := validReplayRequest()
	req.DryRun = true

	result, err := manager.ReplayFallback(context.Background(), req)
	if err != nil {
		t.Fatalf("ReplayFallback returned error: %v", err)
	}
	if result.Status != ReplayStatusDryRun {
		t.Fatalf("status=%s want %s", result.Status, ReplayStatusDryRun)
	}
	if result.PreFallbackFiles != 2 || result.PreFallbackBytes != 2048 {
		t.Fatalf("unexpected pre stats: files=%d bytes=%d", result.PreFallbackFiles, result.PreFallbackBytes)
	}
	if replayer.replayCalls != 0 {
		t.Fatalf("dry run should not execute replay, calls=%d", replayer.replayCalls)
	}
	if len(result.AuditTrail) == 0 || result.AuditTrail[0].Action != "dlq_replay_approved" {
		t.Fatalf("dry run should still record approval audit trail: %+v", result.AuditTrail)
	}
}

func TestReplayManagerIdempotencyPreventsDuplicateReplay(t *testing.T) {
	replayer := &fakeFallbackReplayer{
		fileCount: 3,
		totalSize: 4096,
		report: FallbackReplayReport{
			ReplayedFiles:          2,
			FailedFiles:            0,
			RemainingFallbackFiles: 1,
		},
	}
	manager := NewReplayManager(replayer, nil, zap.NewNop())
	req := validReplayRequest()

	first, err := manager.ReplayFallback(context.Background(), req)
	if err != nil {
		t.Fatalf("first replay returned error: %v", err)
	}
	if first.Status != ReplayStatusCompleted {
		t.Fatalf("status=%s want %s", first.Status, ReplayStatusCompleted)
	}
	if replayer.replayCalls != 1 {
		t.Fatalf("first replay calls=%d want 1", replayer.replayCalls)
	}

	second, err := manager.ReplayFallback(context.Background(), req)
	if err != nil {
		t.Fatalf("duplicate replay returned error: %v", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate request should be marked duplicate")
	}
	if second.ReplayID != first.ReplayID {
		t.Fatalf("duplicate replay_id=%s want %s", second.ReplayID, first.ReplayID)
	}
	if replayer.replayCalls != 1 {
		t.Fatalf("duplicate replay should not execute again, calls=%d", replayer.replayCalls)
	}
}

func TestReplayManagerReportsPartialReplay(t *testing.T) {
	replayer := &fakeFallbackReplayer{
		fileCount: 2,
		totalSize: 1024,
		report: FallbackReplayReport{
			ReplayedFiles:          1,
			FailedFiles:            1,
			RemainingFallbackFiles: 1,
			Errors:                 []string{"replay success rate too low"},
		},
	}
	manager := NewReplayManager(replayer, nil, zap.NewNop())

	result, err := manager.ReplayFallback(context.Background(), validReplayRequest())
	if err != nil {
		t.Fatalf("ReplayFallback returned error: %v", err)
	}
	if result.Status != ReplayStatusPartial {
		t.Fatalf("status=%s want %s", result.Status, ReplayStatusPartial)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("partial replay should keep executor errors")
	}
}

type failingReplayStore struct {
	getErr error
	putErr error
}

func (s failingReplayStore) Get(context.Context, string) (ReplayResult, bool, error) {
	return ReplayResult{}, false, s.getErr
}

func (s failingReplayStore) Put(context.Context, string, ReplayResult) error {
	return s.putErr
}

func TestReplayManagerFailsClosedWhenIdempotencyStoreGetFails(t *testing.T) {
	replayer := &fakeFallbackReplayer{}
	manager := NewReplayManager(replayer, failingReplayStore{getErr: errors.New("redis unavailable")}, zap.NewNop())

	_, err := manager.ReplayFallback(context.Background(), validReplayRequest())
	if err == nil || !strings.Contains(err.Error(), "get replay idempotency record") {
		t.Fatalf("expected get idempotency error, got %v", err)
	}
	if replayer.replayCalls != 0 {
		t.Fatalf("replay must not execute when idempotency get fails, calls=%d", replayer.replayCalls)
	}
}

func TestReplayManagerFailsClosedWhenIdempotencyStorePutFails(t *testing.T) {
	replayer := &fakeFallbackReplayer{}
	manager := NewReplayManager(replayer, failingReplayStore{putErr: errors.New("redis unavailable")}, zap.NewNop())

	_, err := manager.ReplayFallback(context.Background(), validReplayRequest())
	if err == nil || !strings.Contains(err.Error(), "store replay idempotency record") {
		t.Fatalf("expected put idempotency error, got %v", err)
	}
}
