package dlq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/lib/pq"
)

// PostgresReplayApprovalStore 审批权威实现(ENG-CMD-001/ENG-ARCH-002):
// CreateApproval 在同一 PostgreSQL 事务中提交 state(dlq_replay_approvals)+
// history(dlq_replay_approval_history)+ receipt(dlq_replay_approval_receipts),
// 任何一步失败整体回滚;唯一约束冲突返回稳定 conflict 错误。
// GetApproval 只返回 status=approved 的记录,fail-closed。
type PostgresReplayApprovalStore struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewPostgresReplayApprovalStore(db *sql.DB, logger *zap.Logger) *PostgresReplayApprovalStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PostgresReplayApprovalStore{db: db, logger: logger}
}

func (s *PostgresReplayApprovalStore) CreateApproval(ctx context.Context, approval ReplayApproval) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres replay approval store is not configured")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin approval transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			s.logger.Warn("approval transaction rollback failed", zap.Error(rollbackErr))
		}
	}()

	// 1) state:唯一约束 (tenant_id, approval_id) 保证旧 ID 不复用;
	//    冲突映射为稳定 conflict 错误(ENG-CMD-002)。
	res, err := tx.ExecContext(ctx, `
		INSERT INTO dlq_replay_approvals
			(tenant_id, approval_id, requested_by, approved_by, status, reason, request_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, approval_id) DO NOTHING`,
		strings.TrimSpace(approval.TenantID),
		strings.TrimSpace(approval.ApprovalID),
		strings.TrimSpace(approval.RequestedBy),
		strings.TrimSpace(approval.ApprovedBy),
		approval.Status,
		strings.TrimSpace(approval.Reason),
		strings.TrimSpace(approval.RequestHash),
	)
	if err != nil {
		return fmt.Errorf("insert approval state: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read approval insert rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("approval_id already exists")
	}

	// 2) history:不可变审计轨迹,与 state 同事务。
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dlq_replay_approval_history (tenant_id, approval_id, action, actor)
		VALUES ($1, $2, 'created', $3)`,
		strings.TrimSpace(approval.TenantID),
		strings.TrimSpace(approval.ApprovalID),
		strings.TrimSpace(approval.ApprovedBy),
	); err != nil {
		return fmt.Errorf("insert approval history: %w", err)
	}

	// 3) receipt:accepted 仅在本事务 COMMIT 后成立。
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dlq_replay_approval_receipts (tenant_id, approval_id, status)
		VALUES ($1, $2, 'accepted')`,
		strings.TrimSpace(approval.TenantID),
		strings.TrimSpace(approval.ApprovalID),
	); err != nil {
		return fmt.Errorf("insert approval receipt: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit approval transaction: %w", err)
	}
	s.logger.Info("DLQ replay approval accepted (postgres authority)",
		zap.String("tenant_id", approval.TenantID),
		zap.String("approval_id", approval.ApprovalID),
		zap.String("approved_by", approval.ApprovedBy))
	return nil
}

func (s *PostgresReplayApprovalStore) GetApproval(ctx context.Context, tenantID, approvalID string) (*ReplayApproval, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("postgres replay approval store is not configured")
	}

	var approval ReplayApproval
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id, approval_id, requested_by, approved_by, status, reason, request_hash, created_at
		FROM dlq_replay_approvals
		WHERE tenant_id = $1 AND approval_id = $2 AND status = $3`,
		strings.TrimSpace(tenantID),
		strings.TrimSpace(approvalID),
		ApprovalStatusApproved,
	).Scan(
		&approval.TenantID,
		&approval.ApprovalID,
		&approval.RequestedBy,
		&approval.ApprovedBy,
		&approval.Status,
		&approval.Reason,
		&approval.RequestHash,
		&approval.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query approval: %w", err)
	}
	return &approval, nil
}

// isApprovalUniqueViolation 区分唯一约束冲突与其他错误(ENG-CMD-002:稳定 409)。
func isApprovalUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pq.ErrorCode("23505")
}
