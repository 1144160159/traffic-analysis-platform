package dlq

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ApprovalStatusApproved 审批台账唯一有效状态;其余状态(如 pending/rejected)
// 在权威迁移中按 fail-closed 处理。
const ApprovalStatusApproved = "approved"

// ErrApprovalNotFound 审批记录不存在(或状态非 approved)。
var ErrApprovalNotFound = fmt.Errorf("dlq replay approval not found")

// ReplayApproval DLQ 回放审批台账记录。权威约束(ENG-ARCH-002):
// 审批命令由 PostgreSQL 受理,同事务写入 state + history + receipt;
// 校验必须逐字段比对 tenant/approval_id/approved_by/requested_by。
type ReplayApproval struct {
	TenantID    string    `json:"tenant_id"`
	ApprovalID  string    `json:"approval_id"`
	RequestedBy string    `json:"requested_by"`
	ApprovedBy  string    `json:"approved_by"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	RequestHash string    `json:"request_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

// ReplayApprovalStore 审批权威端口。实现必须是权威存储(PG);Redis 只允许
// 幂等辅助,不得作为审批命令受理边界(ENG-DB-005/ENG-CMD-001)。
type ReplayApprovalStore interface {
	CreateApproval(ctx context.Context, approval ReplayApproval) error
	GetApproval(ctx context.Context, tenantID, approvalID string) (*ReplayApproval, error)
}

// MemoryReplayApprovalStore 进程内实现,仅供单元测试;生产禁止使用
// (不满足持久权威要求,服务重启即丢失)。
type MemoryReplayApprovalStore struct {
	mu        sync.Mutex
	approvals map[string]ReplayApproval
}

func NewMemoryReplayApprovalStore() *MemoryReplayApprovalStore {
	return &MemoryReplayApprovalStore{approvals: make(map[string]ReplayApproval)}
}

func (s *MemoryReplayApprovalStore) CreateApproval(_ context.Context, approval ReplayApproval) error {
	if s == nil {
		return fmt.Errorf("memory replay approval store is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.TrimSpace(approval.TenantID) + ":" + strings.TrimSpace(approval.ApprovalID)
	if _, ok := s.approvals[key]; ok {
		return fmt.Errorf("approval_id already exists")
	}
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = time.Now()
	}
	s.approvals[key] = approval
	return nil
}

func (s *MemoryReplayApprovalStore) GetApproval(_ context.Context, tenantID, approvalID string) (*ReplayApproval, error) {
	if s == nil {
		return nil, fmt.Errorf("memory replay approval store is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	approval, ok := s.approvals[strings.TrimSpace(tenantID)+":"+strings.TrimSpace(approvalID)]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	return &approval, nil
}
