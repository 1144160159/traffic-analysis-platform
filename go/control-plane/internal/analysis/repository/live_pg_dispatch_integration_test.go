//go:build integration

// live PG 集成测试(G04):SOURCE_ACTIVATE 派发循环——领取 PENDING 尝试 →
// CAS RUNNING → 执行器回执 → attempt SUCCEEDED + 回执行落地。
package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
)

type stubExecutor struct{}

func (s stubExecutor) Dispatch(_ context.Context, cmd contract.SourceStageCommand) (*contract.ProviderOperationReceipt, error) {
	return &contract.ProviderOperationReceipt{
		OperationID: cmd.RunID + ":SOURCE_ACTIVATE", State: "COMPLETED",
		InputCount: 42, OutputCount: 7, WatermarkMs: cmd.WindowEndMs,
		Fence: json.RawMessage(`{"kind":"source_fence","packets":42}`),
	}, nil
}
func zapLoggerForTest() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	cfg.OutputPaths = []string{"stderr"}
	l, _ := cfg.Build()
	return l
}

func TestLivePGStageDispatchReceiptCycle(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()

	tenant := "integration-dispatch-" + uuid.NewString()[:8]
	defID := uuid.NewString()
	planID := uuid.NewString()
	spec := "dispatch-spec-" + uuid.NewString()[:8]

	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
		VALUES($1,$2,'def-dispatch','ACTIVE',$2,1,$2)`, defID, tenant); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_revisions(id, tenant_id, task_definition_id, plan_revision, plan_source,
			source_kind, source_spec, selected_feature_ids, feature_set_id, encrypted_recognition_model_ref,
			threat_detector_refs, rule_refs, machine_summary_schema_ref, stage_dag, completion_policy,
			resource_budget, catalog_revision, execution_spec_sha256, plan_revision_sha256, created_by)
		VALUES($1,$2,$3,1,'AUTO_DEFAULT','PCAP_REPLAY',
			'{"pcap_object":"s3://analysis-bench/pcap/dispatch.pcap","pcap_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","packet_limit":1000,"byte_limit":0,"probe_id":"probe-agent"}'::jsonb,
			'["f1"]'::jsonb,'fs-v1','enc@v1','["det@v1"]'::jsonb,'["rule@v1"]'::jsonb,'summary-v1',
			'{"stages":["S1","S2","S3","S4","S5"]}'::jsonb,'{"allow_partial":false}'::jsonb,
			'{"cpu":2}'::jsonb,1,$4,$4,$2)`, planID, tenant, defID, spec); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_plan_governance_heads(tenant_id, plan_id, state, authority_revision)
		VALUES($1,$2,'ACTIVE',1)`, tenant, planID); err != nil {
		t.Fatalf("seed head: %v", err)
	}

	// 物化(直接走仓储事务,产生 PENDING SOURCE_ACTIVATE 尝试)
	triggerID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, task_definition_id, plan_revision, actor)
		VALUES($1,$2,'actor',$3,$4,'PENDING_MATERIALIZATION','ON_DEMAND',$5,1,$2)`,
		triggerID, tenant, "id-"+uuid.NewString()[:8], "req-"+uuid.NewString()[:8], defID); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	receipt, replayed, err := repo.MaterializeAnalysisTaskAtomic(ctx, repository.MaterializeCommand{
		TenantID: tenant, IdentityKind: "actor",
		CanonicalIdentityHash: "id-" + uuid.NewString()[:8], RequestSHA256: "req-" + uuid.NewString()[:8],
		TriggerInstanceID: triggerID, TriggerKind: "ON_DEMAND",
		WindowStartMs: 1700000000000, WindowEndMs: 1700000600000,
		TaskDefinitionID: defID, PlanRevision: 1, ExecutionSpecSHA256: spec,
		EffectiveClass: "INTERACTIVE", EffectivePolicySHA256: "policy-1",
		ResourcePool: "analysis-cpu", ResourceVectorJSON: []byte(`{"cpu":2}`),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		NodesJSON:    []byte(`[{"business_phase_id":"S1","execution_node_id":"SOURCE_ACTIVATE","provider_mode":"DEDICATED_OPERATION","activation_mode":"PIPELINED_STREAM"},{"business_phase_id":"S5","execution_node_id":"MACHINE_FINALIZATION","provider_mode":"AUTHORITY_LOCAL","activation_mode":"AUTHORITY_LOCAL"}]`),
		PlanSpecJSON: []byte(`{"execution_spec_sha256":"` + spec + `"}`),
	})
	if err != nil || replayed || receipt.RunID == "" {
		t.Fatalf("materialize: %v replayed=%v", err, replayed)
	}
	t.Logf("materialized run=%s", receipt.RunID)

	// 派发循环(桩执行器)
	dispatcher := service.NewStageDispatcher(repo, stubExecutor{}, zapLoggerForTest())
	dispatcher.TenantScope = tenant
	n, err := dispatcher.DispatchOnce(ctx, 1)
	if err != nil {
		t.Fatalf("dispatch once: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 dispatched attempt, got %d", n)
	}
	var state, fence, provider string
	var in, out int64
	err = db.QueryRowContext(ctx, `
		SELECT sa.state, COALESCE(sr.provider,''), COALESCE(sr.input_count,0), COALESCE(sr.output_count,0),
			COALESCE(sr.fence::text,'{}')
		FROM analysis_stage_attempts sa
		LEFT JOIN analysis_stage_receipts sr ON sr.run_id=sa.run_id AND sr.execution_node_id=sa.execution_node_id AND sr.attempt=sa.attempt
		WHERE sa.run_id=$1 AND sa.execution_node_id='SOURCE_ACTIVATE'`, receipt.RunID).Scan(&state, &provider, &in, &out, &fence)
	if err != nil {
		t.Fatalf("read attempt state: %v", err)
	}
	if state != "SUCCEEDED" || provider != "analysis-replay" || in != 42 || out != 7 {
		t.Fatalf("attempt/receipt mismatch: state=%s provider=%s in=%d out=%d", state, provider, in, out)
	}
	if !json.Valid([]byte(fence)) {
		t.Fatalf("fence not valid json: %s", fence)
	}
	// 幂等:再跑一轮不得重复派发(无 PENDING 候选)
	n2, err := dispatcher.DispatchOnce(ctx, 1)
	if err != nil || n2 != 0 {
		t.Fatalf("second dispatch must be idle: n=%d err=%v", n2, err)
	}
	t.Logf("dispatch cycle PASS: attempt SUCCEEDED, receipt applied, no re-dispatch")

	// 清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_queue WHERE tenant_id=$1`,
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_business_phase_projections WHERE run_id IN (SELECT id FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_admission_reservations WHERE tenant_id=$1`,
		`DELETE FROM analysis_receipts WHERE tenant_id=$1`,
		`DELETE FROM analysis_outbox WHERE key IN (SELECT id::text FROM analysis_runs WHERE tenant_id=$1)`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_plan_governance_heads WHERE plan_id IN (SELECT id FROM analysis_plan_revisions WHERE tenant_id=$1)`,
		`DELETE FROM analysis_plan_revisions WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM analysis_inbox WHERE event_id LIKE 'dispatch-%' AND tuple_hash LIKE '`+receipt.RunID+`%'`); err != nil {
		t.Logf("cleanup inbox: %v", err)
	}
}

// TestLivePGGatingBatchOpensOnSourceDispatch 验证流水线闸门以"源阶段已派发"
// (离开 PENDING)为开闸条件:长回放/延迟派发下 S1 可晚于 window_end 才
// SUCCEEDED,若等待 S1 SUCCEEDED 则 S2-S4 回执先到而被确定性丢弃(run 卡死)。
func TestLivePGGatingBatchOpensOnSourceDispatch(t *testing.T) {
	db := liveDBP2(t)
	defer db.Close()
	repo := repository.NewRepo(db)
	ctx := context.Background()
	tenant := "integration-gate-" + uuid.NewString()[:8]

	seedRun := func(runID, sourceState string) {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO analysis_task_definitions(id, tenant_id, name, state, owner, revision, created_by)
			VALUES($1::uuid,$2,'gate-def-'||left($1::text,8),'ACTIVE',$2,1,$2)`, runID, tenant); err != nil {
			t.Fatalf("seed definition: %v", err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO analysis_trigger_instances(id, tenant_id, identity_kind, canonical_identity_hash, request_sha256, state, trigger_kind, task_definition_id, plan_revision, actor)
			VALUES($1::uuid,$2,'actor','gate-id-'||$1::text,'gate-req-'||$1::text,'MATERIALIZED','ON_DEMAND',$1::uuid,1,$2)`, runID, tenant); err != nil {
			t.Fatalf("seed trigger: %v", err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO analysis_tasks(id, tenant_id, task_definition_id, plan_revision, execution_spec_sha256, trigger_instance_id, effective_class, effective_policy_sha256)
			VALUES($1,$2,$1,1,'gate-spec',$1,'INTERACTIVE','gate-policy')`, runID, tenant); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO analysis_runs(id, tenant_id, task_id, execution_spec_sha256, state, window_start, window_end, created_at)
			VALUES($1,$2,$1,'gate-spec','ACCEPTED',to_timestamp(1700000000),to_timestamp(1700000600),now())`,
			runID, tenant); err != nil {
			t.Fatalf("seed run: %v", err)
		}
		nodes := []struct{ node, phase, mode string }{
			{"SOURCE_ACTIVATE", "S1", "PIPELINED_STREAM"},
			{"SESSIONIZATION", "S2", "PIPELINED_STREAM"},
			{"FEATURE_EXTRACTION", "S2", "PIPELINED_STREAM"},
			{"ENCRYPTED_RECOGNIZER", "S3", "PIPELINED_STREAM"},
			{"RULE_DETECTION", "S4", "PIPELINED_STREAM"},
		}
		for _, n := range nodes {
			state := "PENDING"
			if n.node == "SOURCE_ACTIVATE" {
				state = sourceState
			}
			if _, err := db.ExecContext(ctx, `
				INSERT INTO analysis_stage_attempts(id, tenant_id, run_id, execution_node_id, business_phase_id, attempt, state, activation_mode, provider_mode, fencing_token, created_at)
				VALUES($1,$2,$3,$4,$5,1,$6,$7,'DEDICATED_OPERATION','gate-fence',now())`,
				uuid.NewString(), tenant, runID, n.node, n.phase, state, n.mode); err != nil {
				t.Fatalf("seed attempt: %v", err)
			}
		}
	}

	// 场景 1:SOURCE_ACTIVATE 仍 PENDING → 不开闸。
	// 全局领取可能返回其他测试遗留的可开闸 run(新语义下同样合法),逐批消费
	// 这些遗留候选;只断言 pendingRun 自身不被开闸。
	pendingRun := uuid.NewString()
	seedRun(pendingRun, "PENDING")
	drainUntil := func(target string, rounds int) (*repository.GatingBatch, error) {
		for i := 0; i < rounds; i++ {
			batch, err := repo.NextGatingBatch(ctx)
			if err != nil {
				return nil, err
			}
			if batch == nil {
				return nil, nil
			}
			if batch.RunID == target {
				return batch, nil
			}
			// 消费遗留候选:标记 RUNNING 后不再成为候选(等价于闸门应用)。
			for _, a := range batch.Attempts {
				if _, err := db.ExecContext(ctx, `
					UPDATE analysis_stage_attempts SET state='RUNNING', started_at=now() WHERE id=$1`, a.AttemptID); err != nil {
					return nil, err
				}
			}
		}
		return nil, fmt.Errorf("candidate drain exceeded %d rounds", rounds)
	}
	if batch, err := drainUntil(pendingRun, 120); err != nil {
		t.Fatalf("drain: %v", err)
	} else if batch != nil {
		t.Fatalf("source PENDING must not gate, got %+v", batch)
	}

	// 场景 2:SOURCE_ACTIVATE RUNNING(已派发)→ 一次性批量开闸 S2-S4
	runningRun := uuid.NewString()
	seedRun(runningRun, "RUNNING")
	batch, err := drainUntil(runningRun, 120)
	if err != nil {
		t.Fatalf("drain for running run: %v", err)
	}
	if batch == nil {
		t.Fatalf("expected S2-S4 batch for %s, got nil", runningRun)
	}
	if len(batch.Attempts) != 4 {
		t.Fatalf("expected 4 S2-S4 attempts in batch, got %+v", batch.Attempts)
	}
	leads := 0
	for _, a := range batch.Attempts {
		if a.ExecutionNodeID == "SOURCE_ACTIVATE" {
			t.Fatalf("source attempt must not be in the pipeline batch")
		}
		// 批次领队携带预分配 fencing token(闸门应用时共享给全批)
		if a.FencingToken == "gate-fence" {
			leads++
		}
	}
	if leads != 1 {
		t.Fatalf("batch must carry exactly one pre-allocated fence token lead, got %+v", batch.Attempts)
	}
	t.Logf("gate batch PASS: opens on source dispatch, shared fence token lead, S2-S4 only")

	// 清理
	for _, q := range []string{
		`DELETE FROM analysis_stage_attempts WHERE tenant_id=$1`,
		`DELETE FROM analysis_runs WHERE tenant_id=$1`,
		`DELETE FROM analysis_tasks WHERE tenant_id=$1`,
		`DELETE FROM analysis_trigger_instances WHERE tenant_id=$1`,
		`DELETE FROM analysis_task_definitions WHERE tenant_id=$1`,
	} {
		if _, err := db.ExecContext(ctx, q, tenant); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
}
