package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func testMaterializeCommand() MaterializeCommand {
	return MaterializeCommand{
		TenantID:              "tenant-a",
		IdentityKind:          "actor",
		CanonicalIdentityHash: "id-hash-1",
		RequestSHA256:         "req-hash-1",
		TriggerInstanceID:     "trigger-1",
		TriggerKind:           "ON_DEMAND",
		WindowStartMs:         1000,
		WindowEndMs:           2000,
		TaskDefinitionID:      "def-1",
		PlanRevision:          3,
		ExecutionSpecSHA256:   "spec-1",
		EffectiveClass:        "BASELINE",
		EffectivePolicySHA256: "policy-1",
		ResourcePool:          "analysis-cpu",
		ResourceVectorJSON:    []byte(`{"cpu":2}`),
		ExpiresAt:             time.Now().Add(time.Hour),
		NodesJSON:             []byte(`[{"business_phase_id":"S1","execution_node_id":"SOURCE_ACTIVATE","provider_mode":"DEDICATED_OPERATION","activation_mode":"PIPELINED_STREAM"}]`),
		PlanSpecJSON:          []byte(`{"execution_spec_sha256":"spec-1"}`),
	}
}

func TestMaterializeAnalysisTaskAtomicExactReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_sha256 FROM analysis_materialization_ledger").
		WithArgs("id-hash-1").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256"}).AddRow("req-hash-1"))
	mock.ExpectRollback()

	r := NewRepo(db)
	_, replayed, err := r.MaterializeAnalysisTaskAtomic(context.Background(), testMaterializeCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !replayed {
		t.Fatalf("expected exact replay")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestMaterializeAnalysisTaskAtomicPayloadMismatch(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_sha256 FROM analysis_materialization_ledger").
		WithArgs("id-hash-1").
		WillReturnRows(sqlmock.NewRows([]string{"request_sha256"}).AddRow("other-hash"))
	mock.ExpectRollback()

	r := NewRepo(db)
	_, _, err := r.MaterializeAnalysisTaskAtomic(context.Background(), testMaterializeCommand())
	if err != ErrPayloadMismatch {
		t.Fatalf("expected ErrPayloadMismatch, got %v", err)
	}
}

func TestMaterializeAnalysisTaskAtomicFullPath(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cmd := testMaterializeCommand()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("id-hash-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_sha256 FROM analysis_materialization_ledger").
		WithArgs("id-hash-1").WillReturnRows(sqlmock.NewRows([]string{"request_sha256"}))
	mock.ExpectExec("INSERT INTO analysis_materialization_ledger").
		WithArgs("id-hash-1", "req-hash-1").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT state FROM analysis_trigger_instances").
		WithArgs("trigger-1").WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("PENDING_MATERIALIZATION"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM analysis_plan_revisions").
		WithArgs("def-1", "tenant-a", int64(3), "spec-1").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO analysis_tasks").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_runs").WillReturnResult(sqlmock.NewResult(1, 1))
	// 测试命令 1 个 required 节点:1 attempt + 1 queue 行(§76.45.3 候选队列)
	mock.ExpectExec("INSERT INTO analysis_stage_attempts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_stage_queue").WillReturnResult(sqlmock.NewResult(1, 1))
	// 五段投影(5 次)
	for i := 0; i < 5; i++ {
		mock.ExpectExec("INSERT INTO analysis_business_phase_projections").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("INSERT INTO analysis_admission_reservations").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE analysis_trigger_instances SET materialized_task_id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO analysis_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO analysis_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	r := NewRepo(db)
	receipt, replayed, err := r.MaterializeAnalysisTaskAtomic(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replayed || receipt.TaskID == "" || receipt.RunID == "" {
		t.Fatalf("unexpected receipt: %+v replayed=%v", receipt, replayed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestApplyStageReceiptAtomicAppliedAndReplayed(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cmd := ReceiptCommand{
		TenantID: "tenant-a", RunID: "run-1", EventID: "ev-1", TupleHash: "tuple-1",
		ExecutionNodeID: "RULE_DETECTION", Attempt: 1, FencingToken: "fence-1",
		Provider: "flink-rule-job", InputCount: 10, OutputCount: 10,
		ExpectedState: "RUNNING", NewState: "SUCCEEDED",
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO analysis_inbox").
		WithArgs("ev-1", "tuple-1").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT state, COALESCE\\(fencing_token").
		WithArgs("run-1", "RULE_DETECTION", int32(1)).
		WillReturnRows(sqlmock.NewRows([]string{"state", "fencing_token"}).AddRow("RUNNING", "fence-1"))
	mock.ExpectExec("INSERT INTO analysis_stage_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE analysis_stage_attempts SET state").
		WithArgs("run-1", "RULE_DETECTION", "SUCCEEDED", int32(1), "RUNNING").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO analysis_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	r := NewRepo(db)
	out, err := r.ApplyStageReceiptAtomic(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Applied || out.Outcome != "APPLIED" {
		t.Fatalf("unexpected outcome: %+v", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestApplyStageReceiptAtomicStaleFenceQuarantines(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cmd := ReceiptCommand{TenantID: "tenant-a", RunID: "run-1", EventID: "ev-2", TupleHash: "tuple-2",
		ExecutionNodeID: "RULE_DETECTION", Attempt: 1, FencingToken: "old-fence",
		ExpectedState: "RUNNING", NewState: "SUCCEEDED"}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO analysis_inbox").
		WithArgs("ev-2", "tuple-2").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT state, COALESCE\\(fencing_token").
		WithArgs("run-1", "RULE_DETECTION", int32(1)).
		WillReturnRows(sqlmock.NewRows([]string{"state", "fencing_token"}).AddRow("RUNNING", "new-fence"))
	mock.ExpectExec("UPDATE analysis_inbox SET outcome='STALE_FENCE'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := NewRepo(db)
	out, err := r.ApplyStageReceiptAtomic(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Outcome != "STALE_FENCE" || !out.Integrity {
		t.Fatalf("expected STALE_FENCE quarantine, got %+v", out)
	}
}

func TestApplyStageReceiptAtomicLateTerminal(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cmd := ReceiptCommand{TenantID: "tenant-a", RunID: "run-1", EventID: "ev-3", TupleHash: "tuple-3",
		ExecutionNodeID: "RULE_DETECTION", Attempt: 1, FencingToken: "fence-1",
		ExpectedState: "RUNNING", NewState: "SUCCEEDED"}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO analysis_inbox").
		WithArgs("ev-3", "tuple-3").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT state, COALESCE\\(fencing_token").
		WithArgs("run-1", "RULE_DETECTION", int32(1)).
		WillReturnRows(sqlmock.NewRows([]string{"state", "fencing_token"}).AddRow("SUCCEEDED", "fence-1"))
	mock.ExpectExec("UPDATE analysis_inbox SET outcome='LATE_TERMINAL'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	r := NewRepo(db)
	out, err := r.ApplyStageReceiptAtomic(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Outcome != "LATE_TERMINAL" {
		t.Fatalf("expected LATE_TERMINAL, got %+v", out)
	}
}
