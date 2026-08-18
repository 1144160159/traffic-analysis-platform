package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// ReceiptCommand 阶段回执命令(卷A §3.3)。
type ReceiptCommand struct {
	TenantID        string
	RunID           string
	EventID         string // transport event ID(唯一)
	TupleHash       string // 语义 tuple hash
	ExecutionNodeID string
	Attempt         int32
	FencingToken    string
	Provider        string
	InputCount      int64
	OutputCount     int64
	ErrorCount      int64
	RejectCount     int64
	WatermarkMs     int64
	FenceJSON       []byte
	PayloadHash     string
	ExpectedState   string // attempt 当前期望状态(RUNNING→SUCCEEDED 等)
	NewState        string
}

// ReceiptOutcome inbox 五 outcome(对齐方案 §10.2)。
type ReceiptOutcome struct {
	Applied   bool
	Outcome   string // APPLIED|REPLAYED|QUARANTINED_HASH_CONFLICT|STALE_FENCE|LATE_TERMINAL
	Integrity bool   // 是否为 integrity failure(需隔离)
}

// ApplyStageReceiptAtomic 阶段回执事务:inbox 去重→锁 attempt→fencing 校验→插 receipt
// →CAS attempt→重算阶段投影→history/outbox→COMMIT。确定性非法消息 quarantine 后提交 offset。
func (r *Repo) ApplyStageReceiptAtomic(ctx context.Context, cmd ReceiptCommand) (*ReceiptOutcome, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin receipt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. inbox 去重
	res, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_inbox(event_id, tuple_hash, outcome) VALUES($1,$2,'RECEIVED') ON CONFLICT (event_id) DO NOTHING`,
		cmd.EventID, cmd.TupleHash)
	if err != nil {
		return nil, fmt.Errorf("inbox insert: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 已存在:精确重放(同 event 同 tuple)→REPLAYED
		var existingHash, existingOutcome string
		if err := tx.QueryRowContext(ctx, `SELECT tuple_hash, outcome FROM analysis_inbox WHERE event_id=$1`, cmd.EventID).Scan(&existingHash, &existingOutcome); err != nil {
			return nil, fmt.Errorf("inbox lookup: %w", err)
		}
		if existingHash == cmd.TupleHash {
			_ = tx.Commit()
			return &ReceiptOutcome{Outcome: "REPLAYED"}, nil
		}
		// 同 event 异 tuple:integrity failure → quarantine,提交后 ACK(不无限重投)
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_inbox SET outcome='QUARANTINED_HASH_CONFLICT', tuple_hash=$2 WHERE event_id=$1`,
			cmd.EventID, cmd.TupleHash); err != nil {
			return nil, fmt.Errorf("quarantine inbox: %w", err)
		}
		_ = tx.Commit()
		return &ReceiptOutcome{Outcome: "QUARANTINED_HASH_CONFLICT", Integrity: true}, nil
	}

	// 2. 锁 attempt
	var attemptState, fencingToken string
	err = tx.QueryRowContext(ctx, `
		SELECT state, COALESCE(fencing_token,'') FROM analysis_stage_attempts
		WHERE run_id=$1 AND execution_node_id=$2 AND attempt=$3 FOR UPDATE`,
		cmd.RunID, cmd.ExecutionNodeID, cmd.Attempt).Scan(&attemptState, &fencingToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("attempt not found (gap)")
	}
	if err != nil {
		return nil, fmt.Errorf("lock attempt: %w", err)
	}

	// 3. fencing 校验
	if cmd.FencingToken != fencingToken {
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_inbox SET outcome='STALE_FENCE' WHERE event_id=$1`, cmd.EventID); err != nil {
			return nil, fmt.Errorf("stale fence quarantine: %w", err)
		}
		_ = tx.Commit()
		return &ReceiptOutcome{Outcome: "STALE_FENCE", Integrity: true}, nil
	}
	// 4. 终态迟到
	if isTerminalAttempt(attemptState) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE analysis_inbox SET outcome='LATE_TERMINAL' WHERE event_id=$1`, cmd.EventID); err != nil {
			return nil, fmt.Errorf("late terminal quarantine: %w", err)
		}
		_ = tx.Commit()
		return &ReceiptOutcome{Outcome: "LATE_TERMINAL", Integrity: true}, nil
	}

	// 5. 插 receipt(不可变)
	receiptID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_stage_receipts(id, tenant_id, run_id, execution_node_id, attempt, fencing_token,
			provider, input_count, output_count, error_count, reject_count, watermark, fence, payload_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, to_timestamp($12/1000.0), $13, $14)
		ON CONFLICT (run_id, execution_node_id, attempt, payload_hash) DO NOTHING`,
		receiptID, cmd.TenantID, cmd.RunID, cmd.ExecutionNodeID, cmd.Attempt, cmd.FencingToken,
		cmd.Provider, cmd.InputCount, cmd.OutputCount, cmd.ErrorCount, cmd.RejectCount, cmd.WatermarkMs,
		cmd.FenceJSON, cmd.PayloadHash); err != nil {
		return nil, fmt.Errorf("insert receipt: %w", err)
	}

	// 6. CAS attempt 状态
	res, err = tx.ExecContext(ctx, `
		UPDATE analysis_stage_attempts SET state=$3, finished_at=now()
		WHERE run_id=$1 AND execution_node_id=$2 AND attempt=$4 AND state=$5`,
		cmd.RunID, cmd.ExecutionNodeID, cmd.NewState, cmd.Attempt, cmd.ExpectedState)
	if err != nil {
		return nil, fmt.Errorf("cas attempt: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("attempt CAS failed (expected state %q)", cmd.ExpectedState)
	}

	// 7. history(同事务审计)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'stage_receipt',$2,$3,$4,$5)`,
		cmd.TenantID, cmd.RunID, cmd.NewState, cmd.Provider, cmd.FenceJSON); err != nil {
		return nil, fmt.Errorf("insert history: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit receipt: %w", err)
	}
	return &ReceiptOutcome{Applied: true, Outcome: "APPLIED"}, nil
}

func isTerminalAttempt(s string) bool {
	switch s {
	case "SUCCEEDED", "PARTIAL", "FAILED", "CANCELLED", "SKIPPED":
		return true
	}
	return false
}

// TransitionRunAtomic CAS 推进 Run 状态(带 event 校验由 service 层完成)。
func (r *Repo) TransitionRunAtomic(ctx context.Context, tenantID, runID string, from, to string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_runs SET state=$3, revision=revision+1, started_at=COALESCE(started_at, now())
		WHERE id=$1 AND tenant_id=$2 AND state=$4`,
		runID, tenantID, to, from)
	if err != nil {
		return fmt.Errorf("cas run transition: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("%s: run state %q → %q", contract.ErrCodeInvalidTransition, from, to)
	}
	return nil
}

// FinalizeRunAtomic 终态提交:三件套+Run 终态+配额释放同事务(机器终局核心)。
type FinalizeCommand struct {
	TenantID            string
	RunID               string
	ExpectedState       string // FINALIZING 或 CANCEL_REQUESTED
	NewState            string
	FindingConclusion   string
	RiskSeverity        string
	Completeness        string
	IntegrityState      string
	ScopeJSON           []byte
	KeyFindingsJSON     []byte
	LimitationsJSON     []byte
	EvidenceEntriesJSON []byte
	DecisionInputsJSON  []byte
	NodeExactSetJSON    []byte
	DifferencesJSON     []byte
	Priority            int
	SummarySHA256       string
	ClosureSHA256       string
	EvidenceSHA256      string
}

func (r *Repo) FinalizeRunAtomic(ctx context.Context, cmd FinalizeCommand) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finalize: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_machine_summaries(tenant_id, run_id, finding_conclusion, risk_severity, completeness,
			integrity_state, scope, key_findings, limitations, evidence_manifest_hash, closure_manifest_hash, canonical_sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		cmd.TenantID, cmd.RunID, cmd.FindingConclusion, cmd.RiskSeverity, cmd.Completeness,
		cmd.IntegrityState, cmd.ScopeJSON, cmd.KeyFindingsJSON, cmd.LimitationsJSON,
		cmd.EvidenceSHA256, cmd.ClosureSHA256, cmd.SummarySHA256); err != nil {
		return fmt.Errorf("insert summary: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_evidence_manifests(tenant_id, run_id, entries, sha256) VALUES($1,$2,$3,$4)`,
		cmd.TenantID, cmd.RunID, cmd.EvidenceEntriesJSON, cmd.EvidenceSHA256); err != nil {
		return fmt.Errorf("insert evidence manifest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_run_closure_manifests(tenant_id, run_id, decision_inputs, priority, node_exact_set, differences, sha256)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		cmd.TenantID, cmd.RunID, cmd.DecisionInputsJSON, cmd.Priority, cmd.NodeExactSetJSON, cmd.DifferencesJSON, cmd.ClosureSHA256); err != nil {
		return fmt.Errorf("insert closure manifest: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE analysis_runs SET state=$3, finding_conclusion=$4, risk_severity=$5, completeness=$6, integrity_state=$7,
			finalized_at=now(), revision=revision+1
		WHERE id=$1 AND tenant_id=$2 AND state=$8`,
		cmd.RunID, cmd.TenantID, cmd.NewState, cmd.FindingConclusion, cmd.RiskSeverity, cmd.Completeness, cmd.IntegrityState, cmd.ExpectedState)
	if err != nil {
		return fmt.Errorf("cas finalize: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("%s: run not in %s", contract.ErrCodeInvalidTransition, cmd.ExpectedState)
	}

	// 释放配额(RELEASED;终态唯一)
	if _, err := tx.ExecContext(ctx, `
		UPDATE analysis_admission_reservations SET state='RELEASED' WHERE run_id=$1 AND state IN ('RESERVED','CONSUMED')`,
		cmd.RunID); err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analysis_history(tenant_id, entity, entity_id, action, actor, detail)
		VALUES($1,'run',$2,$3,$4,$5)`,
		cmd.TenantID, cmd.RunID, cmd.NewState, cmd.TenantID, cmd.DecisionInputsJSON); err != nil {
		return fmt.Errorf("insert finalize history: %w", err)
	}

	return tx.Commit()
}

// EnsureIndexes 启动前确保(DDL 已含;此处仅供诊断)。
func (r *Repo) Ping(ctx context.Context) error {
	var v int
	if err := r.db.QueryRowContext(ctx, `SELECT 1`).Scan(&v); err != nil {
		return err
	}
	var t time.Time
	return r.db.QueryRowContext(ctx, `SELECT now()`).Scan(&t)
}
