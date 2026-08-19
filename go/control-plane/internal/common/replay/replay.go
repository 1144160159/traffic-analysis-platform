// Package replay 定义跨服务统一的 DLQ/回放门面契约(GoF Facade/Strategy 落点)。
//
// 现状:
//   - ingest/dlq.ReplayManager 是首个实现者(通过类型别名复用本包契约,
//     见 internal/ingest/dlq/replay_manager.go);
//   - audit-materializer 当前使用 consumer 级 DLQ(EnableDLQ/DLQPermanentOnly)
//     保证审计事件持久性,接入应用级回放时复用本包契约;
//   - 其他服务的 DLQ 重放接入点应统一实现 Manager,避免各域自建不一致语义。
package replay

import (
	"context"
	"sync"
	"time"
)

const (
	ReplayStatusDryRun    = "dry_run"
	ReplayStatusCompleted = "completed"
	ReplayStatusPartial   = "partial"
)

// ReplayRequest 一次回放的冻结请求(与具体存储/传输实现无关)。
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

// ReplayResult 一次回放的不可变结果。
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

// ReplayAuditEntry 回放过程中的单条审计动作。
type ReplayAuditEntry struct {
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	TenantID  string                 `json:"tenant_id"`
	Result    string                 `json:"result"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ReplayIdempotencyStore 幂等键 → 结果的持久化契约。
type ReplayIdempotencyStore interface {
	Get(ctx context.Context, key string) (ReplayResult, bool, error)
	Put(ctx context.Context, key string, result ReplayResult) error
}

// MemoryReplayIdempotencyStore 内存实现(测试/单实例场景)。
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

// Manager 回放门面:幂等、审批 fail-closed、带审计的重放入口。
// 各服务实现本接口即可接入统一回放语义。
type Manager interface {
	Replay(ctx context.Context, req ReplayRequest) (*ReplayResult, error)
}
