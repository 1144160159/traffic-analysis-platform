package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// FinalizeCandidate 终局候选:S1-S4 全部终态、S5 尚未终态的 run。
type FinalizeCandidate struct {
	TenantID            string
	RunID               string
	TaskID              string
	RunState            string
	ExecutionSpecSHA256 string
	WindowStartMs       int64
	WindowEndMs         int64
	Revision            int64
}

// NextFinalizeCandidate 领取一条 S1-S4 全部终态(SUCCEEDED/FAILED)且 S5 未终态
// 的 run(FOR UPDATE SKIP LOCKED)。S5(RECONCILE/MACHINE_FINALIZATION)是调度
// 权威自身的终局阶段,由终局循环自己完成回执,不依赖外部执行器。
func (r *Repo) NextFinalizeCandidate(ctx context.Context) (*FinalizeCandidate, error) {
	var c FinalizeCandidate
	var windowStart, windowEnd sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT rn.tenant_id, rn.id, rn.task_id, rn.state, rn.execution_spec_sha256,
			rn.window_start, rn.window_end, rn.revision
		FROM analysis_runs rn
		WHERE rn.state IN ('ACCEPTED','PREPARING','QUEUED','RUNNING','FINALIZING')
			AND NOT EXISTS (
				SELECT 1 FROM analysis_stage_attempts sa
				WHERE sa.run_id = rn.id
					AND sa.business_phase_id IN ('S1','S2','S3','S4')
					AND sa.state NOT IN ('SUCCEEDED','FAILED'))
			AND NOT EXISTS (
				SELECT 1 FROM analysis_stage_attempts sa2
				WHERE sa2.run_id = rn.id
					AND sa2.business_phase_id = 'S5'
					AND sa2.state NOT IN ('PENDING','RUNNING'))
			-- 终局需要权威自身两个 S5 节点齐备(旧版物化的残缺 run 不入候选,
			-- 避免 RECONCILE attempt missing 卡死循环)
			AND EXISTS (
				SELECT 1 FROM analysis_stage_attempts sa3
				WHERE sa3.run_id = rn.id AND sa3.execution_node_id = 'RECONCILE')
			AND EXISTS (
				SELECT 1 FROM analysis_stage_attempts sa4
				WHERE sa4.run_id = rn.id AND sa4.execution_node_id = 'MACHINE_FINALIZATION')
		ORDER BY rn.created_at LIMIT 1
		FOR UPDATE OF rn SKIP LOCKED`, ).Scan(
		&c.TenantID, &c.RunID, &c.TaskID, &c.RunState, &c.ExecutionSpecSHA256,
		&windowStart, &windowEnd, &c.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if windowStart.Valid {
		c.WindowStartMs = windowStart.Time.UnixMilli()
	}
	if windowEnd.Valid {
		c.WindowEndMs = windowEnd.Time.UnixMilli()
	}
	return &c, nil
}

// ClosureAttemptFact 单节点终局事实。
type ClosureAttemptFact struct {
	ID              string
	ExecutionNodeID string
	BusinessPhaseID string
	Attempt         int32
	State           string
	FencingToken    string
	FinishedAt      *time.Time
}

// ClosureReceiptFact 单节点回执事实。
type ClosureReceiptFact struct {
	ExecutionNodeID string
	Provider        string
	FencingToken    string
	InputCount      int64
	OutputCount     int64
	ErrorCount      int64
	RejectCount     int64
	PayloadHash     string
	Fence           json.RawMessage
}

// RunClosureFactsRow 终局事实快照(读侧权威;与终局写事务隔离,写事务内再复核)。
type RunClosureFactsRow struct {
	TenantID            string
	RunID               string
	RunState            string
	ExecutionSpecSHA256 string
	WindowStartMs       int64
	WindowEndMs         int64
	Revision            int64
	SourceObjectRef     string
	Attempts            []ClosureAttemptFact
	Receipts            []ClosureReceiptFact
	// S5 尝试(权威自身阶段;终局循环自产回执)
	S5Attempts []ClosureAttemptFact
}

// LoadRunClosureFacts 加载终局事实:run + 全部 attempt + 全部回执。
func (r *Repo) LoadRunClosureFacts(ctx context.Context, runID string) (*RunClosureFactsRow, error) {
	f := &RunClosureFactsRow{RunID: runID}
	var windowStart, windowEnd sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT rn.tenant_id, rn.state, rn.execution_spec_sha256, rn.window_start, rn.window_end, rn.revision
		FROM analysis_runs rn WHERE rn.id=$1`, runID).Scan(
		&f.TenantID, &f.RunState, &f.ExecutionSpecSHA256, &windowStart, &windowEnd, &f.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if windowStart.Valid {
		f.WindowStartMs = windowStart.Time.UnixMilli()
	}
	if windowEnd.Valid {
		f.WindowEndMs = windowEnd.Time.UnixMilli()
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, execution_node_id, business_phase_id, attempt, state, COALESCE(fencing_token,''), finished_at
		FROM analysis_stage_attempts WHERE run_id=$1 ORDER BY business_phase_id, execution_node_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a ClosureAttemptFact
		if err := rows.Scan(&a.ID, &a.ExecutionNodeID, &a.BusinessPhaseID, &a.Attempt, &a.State, &a.FencingToken, &a.FinishedAt); err != nil {
			return nil, err
		}
		if a.BusinessPhaseID == "S5" {
			f.S5Attempts = append(f.S5Attempts, a)
		} else {
			f.Attempts = append(f.Attempts, a)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	recs, err := r.db.QueryContext(ctx, `
		SELECT execution_node_id, provider, fencing_token, input_count, output_count,
			error_count, reject_count, payload_hash, fence
		FROM analysis_stage_receipts WHERE run_id=$1 ORDER BY received_at`, runID)
	if err != nil {
		return nil, err
	}
	defer recs.Close()
	for recs.Next() {
		var p ClosureReceiptFact
		if err := recs.Scan(&p.ExecutionNodeID, &p.Provider, &p.FencingToken, &p.InputCount,
			&p.OutputCount, &p.ErrorCount, &p.RejectCount, &p.PayloadHash, &p.Fence); err != nil {
			return nil, err
		}
		f.Receipts = append(f.Receipts, p)
	}
	return f, recs.Err()
}

// PipelinedFenceToken 取该 run 的流水线阶段(S2/S3/S4)共享预分配 fencing token。
func (r *Repo) PipelinedFenceToken(ctx context.Context, runID string) (string, error) {
	var token string
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(fencing_token,'') FROM analysis_stage_attempts
		WHERE run_id=$1 AND activation_mode='PIPELINED_STREAM'
		ORDER BY created_at LIMIT 1`, runID).Scan(&token)
	if err != nil {
		return "", err
	}
	return token, nil
}
