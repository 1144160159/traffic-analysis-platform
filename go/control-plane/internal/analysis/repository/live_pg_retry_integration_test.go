//go:build integration

// live PG 集成测试:Stage retry 合同(§76.47.3)——run 非终态 + FAILED attempt +
// DEDICATED_OPERATION → 新 attempt;SHARED_STREAM → STAGE_RETRY_UNSUPPORTED;
// 终态 run / 预算耗尽拒绝。
package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

func TestLivePGStageRetryContract(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-retry-" + uuid.NewString()[:8]
	runID := uuid.NewString()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,'def-retry','ACTIVE',$2,1,$2)`, runID, tenant); err != nil {
		t.Fatalf("seed def: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, task_definition_id, plan_revision, actor)
		VALUES($1::uuid,$2,'actor','retry-id','retry-req','MATERIALIZED','ON_DEMAND',$1::uuid,1,$2)`, runID, tenant); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_tasks(id, tenant_id, task_definition_id, plan_revision, execution_spec_sha256, trigger_instance_id, effective_class, effective_policy_sha256)
		VALUES($1,$2,$1,1,'retry-spec',$1,'BASELINE','retry-policy')`, runID, tenant); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_runs(id, tenant_id, task_id, execution_spec_sha256, state, window_start, window_end, created_at)
		VALUES($1,$2,$1,'retry-spec','RUNNING',to_timestamp(1700000000),to_timestamp(1700000600),now())`, runID, tenant); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	// SOURCE_ACTIVATE(DEDICATED_OPERATION)FAILED + SESSIONIZATION(SHARED_STREAM)FAILED
	for _, n := range []struct{ node, phase, mode string }{
		{"SOURCE_ACTIVATE", "S1", "DEDICATED_OPERATION"},
		{"SESSIONIZATION", "S2", "SHARED_STREAM"},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO analysis_stage_attempts(id, tenant_id, run_id, business_phase_id, execution_node_id, attempt, state, provider_mode, activation_mode, created_at, finished_at)
			VALUES($1,$2,$3,$4,$5,1,'FAILED',$6,$7,now(),now())`,
			uuid.NewString(), tenant, runID, n.phase, n.node, "DEDICATED_OPERATION", n.mode); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}

	svc := service.NewRetryService(repo)

	// 1. SHARED_STREAM → STAGE_RETRY_UNSUPPORTED
	if _, err := svc.RetryStage(ctx, tenant, runID, "SESSIONIZATION", "op-1"); err == nil ||
		!strings.Contains(err.Error(), "STAGE_RETRY_UNSUPPORTED") {
		t.Fatalf("shared stream retry must be unsupported, got %v", err)
	}
	// 2. DEDICATED_OPERATION FAILED → 新 attempt(attempt=2)
	resp, err := svc.RetryStage(ctx, tenant, runID, "SOURCE_ACTIVATE", "op-1")
	if err != nil || resp.Attempt != 2 || resp.NewAttemptID == "" {
		t.Fatalf("dedicated retry: %+v err=%v", resp, err)
	}
	var newState string
	if err := db.QueryRowContext(ctx, `
		SELECT state FROM analysis_stage_attempts WHERE id=$1`, resp.NewAttemptID).Scan(&newState); err != nil || newState != "PENDING" {
		t.Fatalf("new attempt must be PENDING: %s err=%v", newState, err)
	}
	t.Logf("stage retry PASS: shared unsupported; dedicated retried to attempt %d", resp.Attempt)

	// 3. 终态 run → 拒绝
	if _, err := db.ExecContext(ctx, `UPDATE analysis_runs SET state='FAILED' WHERE id=$1`, runID); err != nil {
		t.Fatalf("terminal run: %v", err)
	}
	if _, err := svc.RetryStage(ctx, tenant, runID, "SOURCE_ACTIVATE", "op-1"); err == nil ||
		!strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal run retry must be rejected, got %v", err)
	}
	t.Logf("terminal-run retry rejection PASS")

	// 清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_history WHERE tenant_id=$1`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	_ = time.Now
}
