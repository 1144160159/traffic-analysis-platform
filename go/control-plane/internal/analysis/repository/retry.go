// Package repository Stage retry 合同(§76.47.3):校验输入可重放性后创建同 run 新 attempt。
// SHARED_STREAM 无可重放输入返回 STAGE_RETRY_UNSUPPORTED(只能整 Run retry);
// 不得从"当前最新 Topic/表"重新执行旧 stage(跨窗口/版本漂移)。
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RetryStageResult 重试结果。
type RetryStageResult struct {
	NewAttemptID string
	Attempt      int32
}

// RetryStageAtomic run 非终态 + 最新 attempt FAILED + attempt+1 ≤ maxAttempts →
// 插入同节点新 attempt(PENDING,新 fencing token 由领取时分配)+ 审计。
func (r *Repo) RetryStageAtomic(ctx context.Context, tenantID, runID, executionNodeID string, maxAttempts int32, actor string) (*RetryStageResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. run 非终态
	var runState string
	if err := tx.QueryRowContext(ctx, `
		SELECT state FROM analysis_runs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, runID).Scan(&runState); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("run not found")
		}
		return nil, err
	}
	switch runState {
	case "SUCCEEDED", "PARTIALLY_SUCCEEDED", "FAILED", "CANCELLED":
		return nil, fmt.Errorf("STAGE_RETRY_UNSUPPORTED: run %s is terminal", runState)
	}

	// 2. 最新 attempt:必须 FAILED;activation_mode 必须可重放
	var attempt int32
	var state, activationMode string
	if err := tx.QueryRowContext(ctx, `
		SELECT attempt, state, activation_mode FROM analysis_stage_attempts
		WHERE tenant_id=$1 AND run_id=$2 AND execution_node_id=$3
		ORDER BY attempt DESC LIMIT 1 FOR UPDATE`, tenantID, runID, executionNodeID).Scan(&attempt, &state, &activationMode); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("STAGE_RETRY_UNSUPPORTED: attempt not found")
		}
		return nil, err
	}
	if state != "FAILED" {
		return nil, fmt.Errorf("STAGE_RETRY_UNSUPPORTED: latest attempt state is %s (must be FAILED)", state)
	}
	if activationMode != "DEDICATED_OPERATION" {
		// SHARED_STREAM/AUTHORITY_LOCAL 无冻结可重放输入;只能整 Run retry。
		return nil, fmt.Errorf("STAGE_RETRY_UNSUPPORTED: activation_mode=%s has no replayable input; retry the whole run", activationMode)
	}
	if attempt+1 > maxAttempts {
		return nil, fmt.Errorf("STAGE_RETRY_UNSUPPORTED: attempt budget exhausted (attempt=%d, max=%d)", attempt, maxAttempts)
	}

	// 3. 新 attempt(新 fencing token 由领取时分配)
	newAttemptID := uuid.NewString()
	newAttempt := attempt + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_stage_attempts(id, tenant_id, run_id, business_phase_id, execution_node_id, attempt, state, provider_mode, activation_mode, created_at)
		SELECT $1, tenant_id, run_id, business_phase_id, execution_node_id, $2, 'PENDING', provider_mode, activation_mode, now()
		FROM analysis_stage_attempts WHERE id = (
			SELECT id FROM analysis_stage_attempts
			WHERE tenant_id=$3 AND run_id=$4 AND execution_node_id=$5
			ORDER BY attempt DESC LIMIT 1)`,
		newAttemptID, newAttempt, tenantID, runID, executionNodeID); err != nil {
		return nil, fmt.Errorf("insert retry attempt: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'stage_attempt',$2::uuid,'RETRIED',$3,jsonb_build_object('execution_node_id',$4::text,'attempt',$5::int))`,
		tenantID, newAttemptID, actor, executionNodeID, newAttempt); err != nil {
		return nil, fmt.Errorf("retry history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RetryStageResult{NewAttemptID: newAttemptID, Attempt: newAttempt}, nil
}

var _ = time.Now
