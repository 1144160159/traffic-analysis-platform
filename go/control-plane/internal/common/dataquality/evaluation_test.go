package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fixedRuleReader struct {
	measurement RuleMeasurement
	err         error
	calls       int
}

func (r *fixedRuleReader) MeasureRule(context.Context, string, string, string, json.RawMessage, time.Time, time.Time) (RuleMeasurement, error) {
	r.calls++
	return r.measurement, r.err
}

func TestSafeFlowPredicateRejectsUnknownInput(t *testing.T) {
	if _, err := safeFlowPredicate("event_id) OR 1=1", json.RawMessage(`{"op":"not_empty"}`)); err == nil {
		t.Fatal("field injection must be rejected")
	}
	if _, err := safeFlowPredicate("event_id", json.RawMessage(`{"op":"raw_sql","sql":"1=1"}`)); err == nil {
		t.Fatal("unknown operators and fields must be rejected")
	}
	expression, err := safeFlowPredicate("event_id", json.RawMessage(`{"op":"not_empty"}`))
	if err != nil || expression != "lengthUTF8(trim(event_id)) > 0" {
		t.Fatalf("valid allowlisted predicate rejected: expression=%q err=%v", expression, err)
	}
}

func TestClickHouseRuleReaderUsesBoundedTenantWindow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(), countIf(lengthUTF8(trim(event_id)) > 0), if(count()=0, 0, max(ingest_ts))")+`(?s).*WHERE tenant_id = \? AND ingest_ts >= \? AND ingest_ts < \?`).
		WithArgs("tenant-a", start.UnixMilli(), end.UnixMilli()).
		WillReturnRows(sqlmock.NewRows([]string{"total", "passed", "max_ingest"}).AddRow(10, 9, end.Add(-time.Second).UnixMilli()))
	measurement, err := NewClickHouseRuleMeasurementReader(db).MeasureRule(context.Background(), "tenant-a", "flows_raw", "event_id", json.RawMessage(`{"op":"not_empty"}`), start, end)
	if err != nil || measurement.TotalCount != 10 || measurement.PassedCount != 9 {
		t.Fatalf("bounded measurement failed: result=%+v err=%v", measurement, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateActiveRuleEmptyWindowIsUnknown(t *testing.T) {
	db, mock, monitor := newRuleEvaluationMocks(t)
	defer db.Close()
	at := time.Date(2026, 8, 4, 1, 7, 0, 0, time.UTC)
	ruleID := uuid.NewString()
	expectActiveRule(mock, ruleID)
	expectEvaluationTransaction(mock, ruleID, "unknown", 0, 0, false, "")
	reader := &fixedRuleReader{measurement: RuleMeasurement{SourceWatermarks: map[string]interface{}{"max_ingest_ts": int64(0)}}}
	results, err := monitor.EvaluateActiveRules(context.Background(), "tenant-a", at, "trace-empty", reader)
	if err != nil || len(results) != 1 || results[0].Status != "unknown" || results[0].QualityEventID != "" {
		t.Fatalf("empty window must be unknown: results=%+v err=%v", results, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateActiveRuleFailurePersistsEventAndTwoOutboxes(t *testing.T) {
	db, mock, monitor := newRuleEvaluationMocks(t)
	defer db.Close()
	ruleID := uuid.NewString()
	expectActiveRule(mock, ruleID)
	expectEvaluationTransaction(mock, ruleID, "fail", 10, 8, true, "")
	reader := &fixedRuleReader{measurement: RuleMeasurement{TotalCount: 10, PassedCount: 8, SourceWatermarks: map[string]interface{}{"max_ingest_ts": int64(1)}}}
	results, err := monitor.EvaluateActiveRules(context.Background(), "tenant-a", time.Date(2026, 8, 4, 1, 7, 0, 0, time.UTC), "trace-fail", reader)
	if err != nil || len(results) != 1 || results[0].Status != "fail" || results[0].QualityEventID == "" || results[0].AffectedCount != 2 {
		t.Fatalf("failed rule persistence mismatch: results=%+v err=%v", results, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateActiveRuleRollsBackWhenOutboxFails(t *testing.T) {
	db, mock, monitor := newRuleEvaluationMocks(t)
	defer db.Close()
	ruleID := uuid.NewString()
	expectActiveRule(mock, ruleID)
	expectEvaluationTransaction(mock, ruleID, "pass", 10, 10, false, "outbox")
	reader := &fixedRuleReader{measurement: RuleMeasurement{TotalCount: 10, PassedCount: 10}}
	_, err := monitor.EvaluateActiveRules(context.Background(), "tenant-a", time.Date(2026, 8, 4, 1, 7, 0, 0, time.UTC), "trace-rollback", reader)
	if err == nil || !regexp.MustCompile(`outbox`).MatchString(err.Error()) {
		t.Fatalf("expected outbox failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateActiveRulesQueriesOnlyActiveDefinitions(t *testing.T) {
	db, mock, monitor := newRuleEvaluationMocks(t)
	defer db.Close()
	mock.ExpectQuery(`FROM data_quality_rules\s+WHERE tenant_id=\$1 AND status='active'`).WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "rule_version", "dataset_id", "field_path", "predicate", "threshold", "window_seconds", "severity", "owner"}))
	reader := &fixedRuleReader{}
	results, err := monitor.EvaluateActiveRules(context.Background(), "tenant-a", time.Now(), "trace-active-only", reader)
	if err != nil || len(results) != 0 || reader.calls != 0 {
		t.Fatalf("non-active rules must not be measured: results=%+v calls=%d err=%v", results, reader.calls, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newRuleEvaluationMocks(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Monitor) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(db)
	return db, mock, monitor
}

func expectActiveRule(mock sqlmock.Sqlmock, ruleID string) {
	mock.ExpectQuery(`FROM data_quality_rules\s+WHERE tenant_id=\$1 AND status='active'`).WithArgs("tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "rule_version", "dataset_id", "field_path", "predicate", "threshold", "window_seconds", "severity", "owner"}).
			AddRow(ruleID, int64(1), "flows_raw", "event_id", []byte(`{"op":"not_empty"}`), []byte(`{"minimum":0.9}`), int64(300), "high", "data-platform"))
}

func expectEvaluationTransaction(mock sqlmock.Sqlmock, ruleID, status string, total, passed int64, qualityEvent bool, failAt string) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM data_quality_rule_evaluations WHERE tenant_id=\$1 AND evaluation_id=\$2`).
		WithArgs("tenant-a", sqlmock.AnyArg()).WillReturnError(sql.ErrNoRows)
	if qualityEvent {
		mock.ExpectExec(`INSERT INTO data_quality_events`).WithArgs(
			sqlmock.AnyArg(), "tenant-a", "flows_raw", ruleID, int64(1), "high", sqlmock.AnyArg(), sqlmock.AnyArg(), total-passed,
			sqlmock.AnyArg(), sqlmock.AnyArg(), "data-platform", sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	qualityID := interface{}(nil)
	qualityIDText := interface{}(nil)
	if qualityEvent {
		qualityID = sqlmock.AnyArg()
		qualityIDText = uuid.NewString()
	}
	mock.ExpectQuery(`INSERT INTO data_quality_rule_evaluations`).WithArgs(
		sqlmock.AnyArg(), "tenant-a", ruleID, int64(1), "flows_raw", sqlmock.AnyArg(), sqlmock.AnyArg(), status,
		total, passed, total-passed, sqlmock.AnyArg(), sqlmock.AnyArg(), qualityID, sqlmock.AnyArg(),
	).WillReturnRows(sqlmock.NewRows([]string{
		"evaluation_id", "tenant_id", "rule_id", "rule_version", "dataset_id", "window_start", "window_end", "status",
		"total_count", "passed_count", "affected_count", "measured_value", "source_watermarks", "quality_event_id", "trace_id", "evaluated_at",
	}).AddRow(uuid.NewString(), "tenant-a", ruleID, int64(1), "flows_raw", time.Now().Add(-5*time.Minute), time.Now(), status,
		total, passed, total-passed, []byte(`{"ratio":1,"minimum":0.9}`), []byte(`{}`), qualityIDText, "trace", time.Now()))
	firstOutbox := mock.ExpectExec(`INSERT INTO data_quality_outbox`).WithArgs(
		sqlmock.AnyArg(), "tenant-a", "rule", ruleID, int64(1), "DATA_QUALITY_RULE_EVALUATED", "tenant-a:"+ruleID, sqlmock.AnyArg(), sqlmock.AnyArg(),
	)
	if failAt == "outbox" {
		firstOutbox.WillReturnError(errors.New("outbox unavailable"))
		mock.ExpectRollback()
		return
	}
	firstOutbox.WillReturnResult(sqlmock.NewResult(0, 1))
	if qualityEvent {
		mock.ExpectExec(`INSERT INTO data_quality_outbox`).WithArgs(
			sqlmock.AnyArg(), "tenant-a", "quality_event", sqlmock.AnyArg(), int64(1), "DATA_QUALITY_EVENT_DETECTED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(`INSERT INTO audit_logs`).WithArgs(
		sqlmock.AnyArg(), "tenant-a", evaluatorActor, "data_quality.rule.evaluated", "data_quality_rule_evaluation", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
