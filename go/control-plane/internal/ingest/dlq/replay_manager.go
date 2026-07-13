package dlq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	ReplayStatusDryRun    = "dry_run"
	ReplayStatusCompleted = "completed"
	ReplayStatusPartial   = "partial"
)

type FallbackReplayer interface {
	GetFallbackStats() (fileCount int, totalSize int64, err error)
	ReplayFallbackFiles(ctx context.Context) FallbackReplayReport
}

type ReplayRequest struct {
	TenantID        string `json:"tenant_id"`
	RequestedBy     string `json:"requested_by"`
	ApprovedBy      string `json:"approved_by"`
	ApprovalID      string `json:"approval_id"`
	Reason          string `json:"reason"`
	RepairSummary   string `json:"repair_summary"`
	IdempotencyKey  string `json:"idempotency_key"`
	DryRun          bool   `json:"dry_run"`
	RequestedAtUnix int64  `json:"requested_at_unix,omitempty"`
}

type ReplayResult struct {
	ReplayID               string             `json:"replay_id"`
	Status                 string             `json:"status"`
	Duplicate              bool               `json:"duplicate"`
	TenantID               string             `json:"tenant_id"`
	RequestedBy            string             `json:"requested_by"`
	ApprovedBy             string             `json:"approved_by"`
	ApprovalID             string             `json:"approval_id"`
	IdempotencyKey         string             `json:"idempotency_key"`
	Reason                 string             `json:"reason"`
	RepairSummary          string             `json:"repair_summary"`
	StartedAt              time.Time          `json:"started_at"`
	FinishedAt             time.Time          `json:"finished_at"`
	PreFallbackFiles       int                `json:"pre_fallback_files"`
	PreFallbackBytes       int64              `json:"pre_fallback_bytes"`
	ReplayedFiles          int                `json:"replayed_files"`
	FailedFiles            int                `json:"failed_files"`
	RemainingFallbackFiles int                `json:"remaining_fallback_files"`
	RemainingFallbackBytes int64              `json:"remaining_fallback_bytes"`
	AuditTrail             []ReplayAuditEntry `json:"audit_trail"`
	Errors                 []string           `json:"errors,omitempty"`
}

type ReplayAuditEntry struct {
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	TenantID  string                 `json:"tenant_id"`
	Result    string                 `json:"result"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type ReplayIdempotencyStore interface {
	Get(ctx context.Context, key string) (ReplayResult, bool, error)
	Put(ctx context.Context, key string, result ReplayResult) error
}

type MemoryReplayIdempotencyStore struct {
	mu      sync.Mutex
	results map[string]ReplayResult
}

func NewMemoryReplayIdempotencyStore() *MemoryReplayIdempotencyStore {
	return &MemoryReplayIdempotencyStore{results: make(map[string]ReplayResult)}
}

func (s *MemoryReplayIdempotencyStore) Get(_ context.Context, key string) (ReplayResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.results[key]
	return result, ok, nil
}

func (s *MemoryReplayIdempotencyStore) Put(_ context.Context, key string, result ReplayResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[key] = result
	return nil
}

type ReplayManager struct {
	replayer FallbackReplayer
	store    ReplayIdempotencyStore
	logger   *zap.Logger
	now      func() time.Time
	mu       sync.Mutex
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

func (m *ReplayManager) ReplayFallback(ctx context.Context, req ReplayRequest) (*ReplayResult, error) {
	if err := validateReplayRequest(req); err != nil {
		return nil, err
	}
	if m.replayer == nil {
		return nil, fmt.Errorf("dlq replay executor is not configured")
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok, err := m.store.Get(ctx, idempotencyKey)
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
		report := m.replayer.ReplayFallbackFiles(ctx)
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
	if err := m.store.Put(ctx, idempotencyKey, result); err != nil {
		return nil, fmt.Errorf("store replay idempotency record: %w", err)
	}
	m.logger.Info("DLQ fallback replay request recorded",
		zap.String("replay_id", result.ReplayID),
		zap.String("status", result.Status),
		zap.String("tenant_id", result.TenantID),
		zap.String("approved_by", result.ApprovedBy),
		zap.Bool("dry_run", req.DryRun))
	return &result, nil
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
