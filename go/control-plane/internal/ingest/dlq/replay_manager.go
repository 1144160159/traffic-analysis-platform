package dlq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/replay"
	"go.uber.org/zap"
)

// 统一回放门面契约(common/replay):本包通过类型别名复用共享契约,
// 既有调用方(如 cmd/ingest-gateway)零改动。ReplayManager 实现
// replay.Manager,供其他服务按统一门面接入。
const (
	ReplayStatusDryRun    = replay.ReplayStatusDryRun
	ReplayStatusCompleted = replay.ReplayStatusCompleted
	ReplayStatusPartial   = replay.ReplayStatusPartial
)

type FallbackReplayer interface {
	GetFallbackStats() (fileCount int, totalSize int64, err error)
	ReplayFallbackFiles(ctx context.Context) FallbackReplayReport
	ReplayFallbackFilesForTenant(ctx context.Context, tenantID string) FallbackReplayReport
}

type ReplayRequest = replay.ReplayRequest
type ReplayResult = replay.ReplayResult
type ReplayAuditEntry = replay.ReplayAuditEntry
type ReplayIdempotencyStore = replay.ReplayIdempotencyStore
type MemoryReplayIdempotencyStore = replay.MemoryReplayIdempotencyStore

var NewMemoryReplayIdempotencyStore = replay.NewMemoryReplayIdempotencyStore

type ReplayManager struct {
	replayer  FallbackReplayer
	store     ReplayIdempotencyStore
	approvals ReplayApprovalStore
	logger    *zap.Logger
	now       func() time.Time
	mu        sync.Mutex
}

func NewReplayManager(replayer FallbackReplayer, store ReplayIdempotencyStore, logger *zap.Logger) *ReplayManager {
	if store == nil {
		store = NewMemoryReplayIdempotencyStore()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReplayManager{
		replayer: replayer,
		store:    store,
		logger:   logger,
		now:      time.Now,
	}
}

// SetApprovalStore 注入审批台账。未注入时回放一律拒绝(fail-closed)。
func (m *ReplayManager) SetApprovalStore(s ReplayApprovalStore) {
	m.approvals = s
}

// ApprovalStore 返回当前审批台账(可能为 nil)。
func (m *ReplayManager) ApprovalStore() ReplayApprovalStore {
	return m.approvals
}

// Replay 实现统一回放门面(replay.Manager),语义与 ReplayFallback 完全一致,
// 供其他服务按共同契约接入 DLQ 回放。
func (m *ReplayManager) Replay(ctx context.Context, req ReplayRequest) (*ReplayResult, error) {
	return m.ReplayFallback(ctx, req)
}

// 编译期断言:ReplayManager 实现统一回放门面。
var _ replay.Manager = (*ReplayManager)(nil)

func (m *ReplayManager) ReplayFallback(ctx context.Context, req ReplayRequest) (*ReplayResult, error) {
	if err := validateReplayRequest(req); err != nil {
		return nil, err
	}
	if err := m.verifyApproval(ctx, req); err != nil {
		return nil, err
	}
	if m.replayer == nil {
		return nil, fmt.Errorf("dlq replay executor is not configured")
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)

	// 修复:幂等判定只在锁内做快照读取,长 I/O(ReplayFallbackFilesForTenant)
	// 移到锁外执行,避免全局串行化所有租户的回放。
	m.mu.Lock()
	existing, ok, err := m.store.Get(ctx, idempotencyKey)
	m.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("get replay idempotency record: %w", err)
	}
	if ok {
		existing.Duplicate = true
		existing.AuditTrail = append(existing.AuditTrail, ReplayAuditEntry{
			Action:    "dlq_replay_duplicate",
			Actor:     req.RequestedBy,
			TenantID:  req.TenantID,
			Result:    "deduplicated",
			CreatedAt: m.now(),
			Detail: map[string]interface{}{
				"idempotency_key": idempotencyKey,
				"replay_id":       existing.ReplayID,
			},
		})
		return &existing, nil
	}

	startedAt := m.now()
	fileCount, totalSize, err := m.replayer.GetFallbackStats()
	if err != nil {
		return nil, fmt.Errorf("get dlq fallback stats: %w", err)
	}

	result := ReplayResult{
		ReplayID:               replayID(req),
		Status:                 ReplayStatusDryRun,
		TenantID:               strings.TrimSpace(req.TenantID),
		RequestedBy:            strings.TrimSpace(req.RequestedBy),
		ApprovedBy:             strings.TrimSpace(req.ApprovedBy),
		ApprovalID:             strings.TrimSpace(req.ApprovalID),
		IdempotencyKey:         idempotencyKey,
		Reason:                 strings.TrimSpace(req.Reason),
		RepairSummary:          strings.TrimSpace(req.RepairSummary),
		StartedAt:              startedAt,
		PreFallbackFiles:       fileCount,
		PreFallbackBytes:       totalSize,
		RemainingFallbackFiles: fileCount,
		RemainingFallbackBytes: totalSize,
		AuditTrail: []ReplayAuditEntry{
			{
				Action:    "dlq_replay_approved",
				Actor:     strings.TrimSpace(req.ApprovedBy),
				TenantID:  strings.TrimSpace(req.TenantID),
				Result:    "approved",
				CreatedAt: startedAt,
				Detail: map[string]interface{}{
					"approval_id":     strings.TrimSpace(req.ApprovalID),
					"requested_by":    strings.TrimSpace(req.RequestedBy),
					"repair_summary":  strings.TrimSpace(req.RepairSummary),
					"idempotency_key": idempotencyKey,
					"dry_run":         req.DryRun,
				},
			},
		},
	}

	if !req.DryRun {
		// 租户隔离:按请求租户做过滤回放,只重放该租户的消息并保留其他租户行。
		report := m.replayer.ReplayFallbackFilesForTenant(ctx, strings.TrimSpace(req.TenantID))
		result.ReplayedFiles = report.ReplayedFiles
		result.FailedFiles = report.FailedFiles
		result.RemainingFallbackFiles = report.RemainingFallbackFiles
		result.RemainingFallbackBytes = report.RemainingFallbackBytes
		result.Errors = report.Errors
		result.Status = ReplayStatusCompleted
		if report.FailedFiles > 0 {
			result.Status = ReplayStatusPartial
		}
		result.AuditTrail = append(result.AuditTrail, ReplayAuditEntry{
			Action:    "dlq_replay_executed",
			Actor:     strings.TrimSpace(req.RequestedBy),
			TenantID:  strings.TrimSpace(req.TenantID),
			Result:    result.Status,
			CreatedAt: m.now(),
			Detail: map[string]interface{}{
				"replayed_files":  report.ReplayedFiles,
				"failed_files":    report.FailedFiles,
				"remaining_files": report.RemainingFallbackFiles,
			},
		})
	}

	result.FinishedAt = m.now()
	// 最终幂等写入在锁内执行;若并发/重试已写入,返回既有记录(duplicate)。
	m.mu.Lock()
	if prev, prevOK, getErr := m.store.Get(ctx, idempotencyKey); getErr == nil && prevOK {
		m.mu.Unlock()
		prev.Duplicate = true
		prev.AuditTrail = append(prev.AuditTrail, ReplayAuditEntry{
			Action:    "dlq_replay_duplicate",
			Actor:     req.RequestedBy,
			TenantID:  req.TenantID,
			Result:    "deduplicated",
			CreatedAt: m.now(),
			Detail:    map[string]interface{}{"idempotency_key": idempotencyKey, "replay_id": prev.ReplayID},
		})
		return &prev, nil
	} else if getErr != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("recheck replay idempotency record: %w", getErr)
	}
	putErr := m.store.Put(ctx, idempotencyKey, result)
	m.mu.Unlock()
	if putErr != nil {
		return nil, fmt.Errorf("store replay idempotency record: %w", putErr)
	}
	m.logger.Info("DLQ fallback replay request recorded",
		zap.String("replay_id", result.ReplayID),
		zap.String("status", result.Status),
		zap.String("tenant_id", result.TenantID),
		zap.String("approved_by", result.ApprovedBy),
		zap.Bool("dry_run", req.DryRun))
	return &result, nil
}

// verifyApproval 校验回放请求的审批台账记录。
// fail-closed:台账未配置、记录缺失、状态非 approved、或 tenant/approver/requester
// 任一与请求不符,一律拒绝,不允许把 approval_id 仅作记录字段。
func (m *ReplayManager) verifyApproval(ctx context.Context, req ReplayRequest) error {
	if m.approvals == nil {
		return fmt.Errorf("dlq replay approval store is not configured")
	}
	approval, err := m.approvals.GetApproval(ctx, strings.TrimSpace(req.TenantID), strings.TrimSpace(req.ApprovalID))
	if err != nil {
		return fmt.Errorf("get dlq replay approval: %w", err)
	}
	if approval == nil {
		return fmt.Errorf("dlq replay approval not found")
	}
	if approval.Status != ApprovalStatusApproved {
		return fmt.Errorf("dlq replay approval status is %q, expected %q", approval.Status, ApprovalStatusApproved)
	}
	if strings.TrimSpace(approval.ApprovedBy) != strings.TrimSpace(req.ApprovedBy) {
		return fmt.Errorf("approved_by does not match approval record")
	}
	if strings.TrimSpace(approval.RequestedBy) != strings.TrimSpace(req.RequestedBy) {
		return fmt.Errorf("requested_by does not match approval record")
	}
	if !strings.EqualFold(strings.TrimSpace(approval.TenantID), strings.TrimSpace(req.TenantID)) {
		return fmt.Errorf("approval tenant does not match request tenant")
	}
	return nil
}

func validateReplayRequest(req ReplayRequest) error {
	required := map[string]string{
		"tenant_id":       req.TenantID,
		"requested_by":    req.RequestedBy,
		"approved_by":     req.ApprovedBy,
		"approval_id":     req.ApprovalID,
		"reason":          req.Reason,
		"repair_summary":  req.RepairSummary,
		"idempotency_key": req.IdempotencyKey,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if strings.EqualFold(strings.TrimSpace(req.RequestedBy), strings.TrimSpace(req.ApprovedBy)) {
		return fmt.Errorf("approved_by must be different from requested_by")
	}
	if len(strings.TrimSpace(req.Reason)) < 8 {
		return fmt.Errorf("reason must be at least 8 characters")
	}
	if len(strings.TrimSpace(req.RepairSummary)) < 8 {
		return fmt.Errorf("repair_summary must be at least 8 characters")
	}
	return nil
}

func replayID(req ReplayRequest) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(req.TenantID),
		strings.TrimSpace(req.ApprovalID),
		strings.TrimSpace(req.IdempotencyKey),
	}, "|")))
	return "dlq-replay-" + hex.EncodeToString(sum[:])[:20]
}
