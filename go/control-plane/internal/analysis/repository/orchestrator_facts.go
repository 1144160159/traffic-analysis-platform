// Package repository Orchestrator 接线事实快照(§5.3 Orchestrator 接线):
// 从权威表一次装载编排输入(RunState/PlanReady/准入/租约/订阅/节点事实)。
// 供影子循环输出确定性决策,与现双 loop 行为对照收集等价性证据;切换前不替代任何写路径。
package repository

import (
	"context"
	"fmt"
)

// OrchestratorFacts 编排输入事实快照(与 service.OrchestratorInputs 一一映射)。
type OrchestratorFacts struct {
	RunID               string
	State               string
	PlanReady           bool // 订阅 PREPARE 已发布(rev1 outbox PUBLISHED)
	SubscriptionActive  bool // 订阅 ACTIVE 已发布(S1 领取后)
	AdmissionValid      bool // 准入预留存在
	ReservationConsumed bool
	HasNodeLease        bool
	ExecutionSpecSHA256 string
	WindowStartMs       int64
	WindowEndMs         int64
	AllBusinessTerminal bool
	ClosureCommitted    bool
	Stages              []RunStageView
}

// LoadOrchestratorFacts 装载 run 编排事实(只读;不存在返回错误)。
func (r *Repo) LoadOrchestratorFacts(ctx context.Context, runID string) (*OrchestratorFacts, error) {
	f := &OrchestratorFacts{RunID: runID}
	var tenantID string
	if err := r.db.QueryRowContext(ctx, `
		SELECT ru.state, ru.execution_spec_sha256,
			(EXTRACT(EPOCH FROM ru.window_start)*1000)::bigint, (EXTRACT(EPOCH FROM ru.window_end)*1000)::bigint,
			ru.tenant_id
		FROM analysis_runs ru WHERE ru.id=$1`, runID).Scan(&f.State, &f.ExecutionSpecSHA256, &f.WindowStartMs, &f.WindowEndMs, &tenantID); err != nil {
		return nil, fmt.Errorf("load run facts: %w", err)
	}
	// 订阅发布事实:PREPARE/ACTIVE 载荷 state 字段。
	states, err := r.publishedSubscriptionStates(ctx, runID)
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		if s == "PREPARE" {
			f.PlanReady = true
		}
		if s == "ACTIVE" {
			f.SubscriptionActive = true
		}
	}
	// 准入预留事实
	var resState string
	err = r.db.QueryRowContext(ctx, `
		SELECT state FROM analysis_admission_reservations WHERE run_id=$1 LIMIT 1`, runID).Scan(&resState)
	switch {
	case err == nil:
		f.AdmissionValid = true
		f.ReservationConsumed = resState == "CONSUMED"
	case isNoRowsErr(err):
		// 无预留(遗留 run):AdmissionValid=false
	default:
		return nil, fmt.Errorf("load reservation facts: %w", err)
	}
	// 租约事实:任一 attempt lease_expires_at 非空
	var leased int
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*) FROM analysis_stage_attempts WHERE run_id=$1 AND lease_expires_at IS NOT NULL`, runID).Scan(&leased); err != nil {
		return nil, err
	}
	f.HasNodeLease = leased > 0
	// 业务终态事实
	var terminal bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT NOT EXISTS(
			SELECT 1 FROM analysis_stage_attempts sa
			JOIN analysis_runs rn ON rn.id = sa.run_id
			WHERE sa.run_id=$1 AND sa.business_phase_id <> 'S5'
			  AND sa.state NOT IN ('SUCCEEDED','FAILED','SKIPPED','CANCELLED'))`, runID).Scan(&terminal); err != nil {
		return nil, err
	}
	f.AllBusinessTerminal = terminal
	f.ClosureCommitted = IsTerminalRunState(f.State)
	stages, err := r.ListRunStages(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	f.Stages = stages
	return f, nil
}

// publishedSubscriptionStates 读取已发布订阅载荷的 state 字段(PREPARE/ACTIVE)。
func (r *Repo) publishedSubscriptionStates(ctx context.Context, runID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(payload->>'state','') FROM analysis_outbox
		WHERE key=$1 AND topic='analysis.run.events.v1' AND state='PUBLISHED'`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListNonTerminalRunIDs 非终态 run id(影子循环候选;LIMIT 封顶)。
// 排除测试/演示租户:影子决策只对照生产面,避免残留数据噪声。
func (r *Repo) ListNonTerminalRunIDs(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM analysis_runs
		WHERE state NOT IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED','CANCELLED')
		  AND tenant_id NOT LIKE 'integration-%' AND tenant_id <> 'live-gw-demo'
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func isNoRowsErr(err error) bool {
	return err != nil && err.Error() == "sql: no rows in result set"
}
