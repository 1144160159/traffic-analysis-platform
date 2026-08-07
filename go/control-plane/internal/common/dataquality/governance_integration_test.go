package dataquality

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestDataQualityGovernanceEphemeralPostgres(t *testing.T) {
	dsn := os.Getenv("DATA_QUALITY_GOVERNANCE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("DATA_QUALITY_GOVERNANCE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_data_quality_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", marker, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tenantID := "dq-governance-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,'DQ Governance Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	defer cleanupDataQualityGovernanceIntegration(t, db, tenantID)
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(db)

	datasetCommand := DatasetCommand{
		TenantID: tenantID, DatasetID: "flows_raw", DisplayName: "Raw flows", Owner: "data-platform",
		SchemaVersion: 1, SignalContractVersion: "data-quality-dataset-signals-v1",
		BusinessKeys: []string{"event_id"}, AllowedLateness: 60, RetentionSeconds: 86400,
		Upstreams: []string{"flow.events.v1"}, Downstreams: []string{"traffic.flows_raw"},
		SLOTarget: .999, Status: "active", ExpectedRevision: 0, ActionID: "dataset-create",
		IdempotencyKey: "dq-dataset-integration-0001", Reason: "register governed flow dataset",
		Actor: "requester-a", TraceID: "trace-dq-dataset-create",
	}
	dataset, err := monitor.UpsertDataset(ctx, datasetCommand)
	if err != nil || dataset.Revision != 1 || dataset.Replayed {
		t.Fatalf("create dataset: record=%+v err=%v", dataset, err)
	}
	replayed, err := monitor.UpsertDataset(ctx, datasetCommand)
	if err != nil || !replayed.Replayed || replayed.Revision != dataset.Revision {
		t.Fatalf("replay dataset: record=%+v err=%v", replayed, err)
	}
	collision := datasetCommand
	collision.DisplayName = "Collision"
	if _, err := monitor.UpsertDataset(ctx, collision); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency collision, got %v", err)
	}
	update := datasetCommand
	update.DisplayName = "Raw network flows"
	update.ExpectedRevision = 1
	update.IdempotencyKey = "dq-dataset-integration-0002"
	update.ActionID = "dataset-update"
	update.Reason = "update governed dataset display name"
	updated, err := monitor.UpsertDataset(ctx, update)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update dataset: record=%+v err=%v", updated, err)
	}

	rule, err := monitor.CreateRule(ctx, RuleCreateCommand{
		TenantID: tenantID, DatasetID: "flows_raw", RuleKey: "flow-event-id-present",
		Dimension: "completeness", FieldPath: "event_id", Predicate: map[string]interface{}{"op": "not_empty"},
		Threshold: map[string]interface{}{"minimum": 1.0}, WindowSeconds: 300,
		Sampling: map[string]interface{}{"rate": 1.0}, Severity: "high", Owner: "data-platform",
		ExemptionPolicy: map[string]interface{}{}, GatePolicy: "observe", ExpectedRevision: 0,
		ActionID: "rule-create", IdempotencyKey: "dq-rule-integration-000001",
		Reason: "create event identity completeness rule", Actor: "requester-a", TraceID: "trace-rule-create",
	})
	if err != nil || rule.Status != "draft" || rule.Revision != 1 {
		t.Fatalf("create rule: record=%+v err=%v", rule, err)
	}
	rule = transitionIntegrationRule(t, monitor, ctx, rule, "start_shadow", "dq-rule-shadow-000000001", "start rule in shadow evaluation", "requester-a")
	rule = transitionIntegrationRule(t, monitor, ctx, rule, "submit_approval", "dq-rule-submit-000000001", "submit observed shadow rule for approval", "requester-a")
	self := RuleTransitionCommand{TenantID: tenantID, RuleID: rule.RuleID, Action: "approve", ExpectedRevision: rule.Revision,
		ActionID: "rule-approve", IdempotencyKey: "dq-rule-self-approve-0001", Reason: "attempt independent approval check",
		Actor: "requester-a", TraceID: "trace-rule-self-approve"}
	if _, err := monitor.TransitionRule(ctx, self); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("expected self approval rejection, got %v", err)
	}
	rule = transitionIntegrationRule(t, monitor, ctx, rule, "approve", "dq-rule-approve-00000001", "independent reviewer approved observed rule", "reviewer-b")
	if rule.Status != "active" || rule.ApprovedBy != "reviewer-b" || rule.Revision != 4 {
		t.Fatalf("unexpected approved rule: %+v", rule)
	}
	evaluationAt := time.Date(2026, 8, 4, 12, 7, 0, 0, time.UTC)
	passReader := &fixedRuleReader{measurement: RuleMeasurement{TotalCount: 10, PassedCount: 10, SourceWatermarks: map[string]interface{}{"clickhouse_max_ingest_ts": evaluationAt.UnixMilli()}}}
	passEvaluations, err := monitor.EvaluateActiveRules(ctx, tenantID, evaluationAt, "trace-evaluation-pass", passReader)
	if err != nil || len(passEvaluations) != 1 || passEvaluations[0].Status != "pass" || passEvaluations[0].QualityEventID != "" {
		t.Fatalf("persist passing active rule evaluation: evaluations=%+v err=%v", passEvaluations, err)
	}
	replayedEvaluations, err := monitor.EvaluateActiveRules(ctx, tenantID, evaluationAt, "trace-evaluation-replay", passReader)
	if err != nil || len(replayedEvaluations) != 1 || !replayedEvaluations[0].Replayed || replayedEvaluations[0].EvaluationID != passEvaluations[0].EvaluationID {
		t.Fatalf("replay stable evaluation: evaluations=%+v err=%v", replayedEvaluations, err)
	}
	failReader := &fixedRuleReader{measurement: RuleMeasurement{TotalCount: 10, PassedCount: 8, SourceWatermarks: map[string]interface{}{"clickhouse_max_ingest_ts": evaluationAt.Add(5 * time.Minute).UnixMilli()}}}
	failEvaluations, err := monitor.EvaluateActiveRules(ctx, tenantID, evaluationAt.Add(5*time.Minute), "trace-evaluation-fail", failReader)
	if err != nil || len(failEvaluations) != 1 || failEvaluations[0].Status != "fail" || failEvaluations[0].QualityEventID == "" || failEvaluations[0].AffectedCount != 2 {
		t.Fatalf("persist failing active rule evaluation: evaluations=%+v err=%v", failEvaluations, err)
	}
	repair, err := monitor.CreateRepair(ctx, RepairCreateCommand{
		TenantID: tenantID, QualityEventID: failEvaluations[0].QualityEventID, OperationID: "flow_replay_window_v1",
		InputScope: map[string]interface{}{
			"dataset_id": "flows_raw", "tenant_id": tenantID,
			"window_start": "2026-08-04T12:05:00Z", "window_end": "2026-08-04T12:10:00Z",
		},
		ResourceBudget: map[string]interface{}{"max_rows": float64(1000), "max_duration_seconds": float64(60)},
		ActionID:       "repair-create", IdempotencyKey: "dq-repair-create-0000001",
		Reason: "plan bounded flow replay for failed quality event", Actor: "requester-a", TraceID: "trace-repair-create",
	})
	if err != nil || repair.Status != "planned" || repair.Revision != 1 {
		t.Fatalf("create bounded repair: repair=%+v err=%v", repair, err)
	}
	repair = transitionIntegrationRepair(t, monitor, ctx, repair, "complete_dry_run", "dq-repair-dry-run-00001", "complete bounded non-destructive repair dry-run", "requester-a", map[string]interface{}{"within_budget": true, "destructive": false, "estimated_rows": float64(2)}, false)
	repair = transitionIntegrationRepair(t, monitor, ctx, repair, "submit_approval", "dq-repair-submit-0000001", "submit successful repair dry-run for approval", "requester-a", map[string]interface{}{}, false)
	selfRepairApproval := RepairTransitionCommand{TenantID: tenantID, RepairID: repair.RepairID, Action: "approve", ExpectedRevision: repair.Revision,
		Summary: map[string]interface{}{}, ActionID: "repair-approve", IdempotencyKey: "dq-repair-self-approve-01",
		Reason: "attempt repair approval separation check", Actor: "requester-a", TraceID: "trace-repair-self-approve"}
	if _, err := monitor.TransitionRepair(ctx, selfRepairApproval, false); !errors.Is(err, ErrRepairApprovalSeparation) {
		t.Fatalf("expected repair self approval rejection, got %v", err)
	}
	repair = transitionIntegrationRepair(t, monitor, ctx, repair, "approve", "dq-repair-approve-000001", "independent reviewer approved bounded replay", "reviewer-b", map[string]interface{}{}, false)
	disabledExecution := RepairTransitionCommand{TenantID: tenantID, RepairID: repair.RepairID, Action: "start_execution", ExpectedRevision: repair.Revision,
		Summary: map[string]interface{}{}, ActionID: "repair-execute", IdempotencyKey: "dq-repair-execute-disabled",
		Reason: "verify repair execution remains default off", Actor: "reviewer-b", TraceID: "trace-repair-disabled"}
	if _, err := monitor.TransitionRepair(ctx, disabledExecution, false); !errors.Is(err, ErrRepairExecutionDisabled) {
		t.Fatalf("expected disabled repair execution, got %v", err)
	}
	repair = transitionIntegrationRepair(t, monitor, ctx, repair, "start_execution", "dq-repair-execute-000001", "start approved bounded replay execution", "executor-a", map[string]interface{}{}, true)
	replayDriver := &integrationRepairReplayDriver{tenantID: tenantID, repairID: repair.RepairID}
	worker := NewRepairExecutionWorker(db, monitor, replayDriver, time.Second, zap.NewNop())
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("execute durable bounded replay: %v", err)
	}
	repair = loadIntegrationRepair(t, db, ctx, tenantID, repair.RepairID)
	if repair.Status != "executed" || repair.Revision != 6 || repair.RepairSummary["server_derived"] != true || repair.RepairSummary["published_rows"] != float64(2) {
		t.Fatalf("unexpected executed repair: %+v", repair)
	}
	repair = transitionIntegrationRepair(t, monitor, ctx, repair, "reconcile", "dq-repair-reconcile-0001", "reconcile replay against authoritative flow facts", "reconciler-a", map[string]interface{}{"all_match": true, "missing_count": float64(0), "extra_count": float64(0)}, true)
	if repair.Status != "reconciled" || repair.CompletedAt == nil {
		t.Fatalf("unexpected reconciled repair: %+v", repair)
	}

	otherDatasets, err := monitor.ListDatasets(ctx, "another-tenant")
	if err != nil || len(otherDatasets) != 0 {
		t.Fatalf("cross-tenant dataset leak: records=%+v err=%v", otherDatasets, err)
	}
	var datasetHistory, ruleHistory, evaluations, qualityEvents, repairs, repairHistory, repairReceipts, outbox, receipts, audits int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM data_quality_dataset_history WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_rule_history WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_rule_evaluations WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_events WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_repairs WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_repair_history WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_repair_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM data_quality_command_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type IN ('data_quality_dataset','data_quality_rule','data_quality_rule_evaluation','data_quality_repair'))`, tenantID).
		Scan(&datasetHistory, &ruleHistory, &evaluations, &qualityEvents, &repairs, &repairHistory, &repairReceipts, &outbox, &receipts, &audits); err != nil {
		t.Fatal(err)
	}
	if datasetHistory != 2 || ruleHistory != 4 || evaluations != 2 || qualityEvents != 1 || repairs != 1 || repairHistory != 7 || repairReceipts != 7 || outbox != 16 || receipts != 6 || audits != 15 {
		t.Fatalf("atomic facts dataset_history=%d rule_history=%d evaluations=%d quality_events=%d repairs=%d repair_history=%d repair_receipts=%d outbox=%d receipts=%d audits=%d", datasetHistory, ruleHistory, evaluations, qualityEvents, repairs, repairHistory, repairReceipts, outbox, receipts, audits)
	}
}

type integrationRepairReplayDriver struct {
	tenantID string
	repairID string
}

func (*integrationRepairReplayDriver) Ready(context.Context) error { return nil }

func (d *integrationRepairReplayDriver) Replay(_ context.Context, request RepairReplayRequest) (map[string]interface{}, error) {
	if request.TenantID != d.tenantID || request.RepairID != d.repairID || request.OperationID != "flow_replay_window_v1" || request.Revision != 5 {
		return nil, fmt.Errorf("unexpected server-loaded replay request: %+v", request)
	}
	return map[string]interface{}{
		"published": true, "published_rows": int64(2), "source_rows": int64(2),
		"replay_target": "integration.fake",
	}, nil
}

func loadIntegrationRepair(t *testing.T, db *sql.DB, ctx context.Context, tenantID, repairID string) *RepairRecord {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	record, err := loadRepairForUpdate(ctx, tx, tenantID, repairID)
	if err != nil {
		t.Fatal(err)
	}
	return &record
}

func transitionIntegrationRule(t *testing.T, monitor *Monitor, ctx context.Context, current *RuleRecord, action, key, reason, actor string) *RuleRecord {
	t.Helper()
	record, err := monitor.TransitionRule(ctx, RuleTransitionCommand{
		TenantID: current.TenantID, RuleID: current.RuleID, Action: action, ExpectedRevision: current.Revision,
		ActionID: "rule-" + action, IdempotencyKey: key, Reason: reason, Actor: actor, TraceID: "trace-" + action,
	})
	if err != nil {
		t.Fatalf("transition %s: %v", action, err)
	}
	return record
}

func transitionIntegrationRepair(t *testing.T, monitor *Monitor, ctx context.Context, current *RepairRecord, action, key, reason, actor string, summary map[string]interface{}, executionEnabled bool) *RepairRecord {
	t.Helper()
	record, err := monitor.TransitionRepair(ctx, RepairTransitionCommand{
		TenantID: current.TenantID, RepairID: current.RepairID, Action: action, ExpectedRevision: current.Revision,
		Summary: summary, ActionID: "repair-" + action, IdempotencyKey: key, Reason: reason, Actor: actor, TraceID: "trace-repair-" + action,
	}, executionEnabled)
	if err != nil {
		t.Fatalf("repair transition %s: %v", action, err)
	}
	return record
}

func cleanupDataQualityGovernanceIntegration(t *testing.T, db *sql.DB, tenantID string) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Errorf("begin governance cleanup: %v", err)
		return
	}
	defer tx.Rollback()
	statements := []string{
		`DELETE FROM data_quality_repair_requests WHERE tenant_id=$1`,
		`DELETE FROM data_quality_repair_history WHERE tenant_id=$1`,
		`DELETE FROM data_quality_command_requests WHERE tenant_id=$1`,
		`DELETE FROM data_quality_rule_history WHERE tenant_id=$1`,
		`DELETE FROM data_quality_dataset_history WHERE tenant_id=$1`,
		`DELETE FROM data_quality_repairs WHERE tenant_id=$1`,
		`DELETE FROM data_quality_rule_evaluations WHERE tenant_id=$1`,
		`DELETE FROM data_quality_events WHERE tenant_id=$1`,
		`DELETE FROM data_quality_outbox WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1 AND object_type IN ('data_quality_dataset','data_quality_rule','data_quality_rule_evaluation','data_quality_repair')`,
		`DELETE FROM data_quality_rules WHERE tenant_id=$1`,
		`DELETE FROM data_quality_datasets WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement, tenantID); err != nil {
			t.Errorf("cleanup governance integration: %v", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("commit governance cleanup: %v", err)
	}
}
