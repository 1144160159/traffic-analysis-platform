package baseline

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRequestBuildTxWritesDefinitionJobOutboxHistoryAndReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	request := validBuildRequest()
	request.WindowStart, request.WindowEnd = &start, &end
	request.MinimumEligibleRows = 100
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1,0\)\)`).
		WithArgs(request.TenantID + "::" + request.IdempotencyKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT command_type,request_sha256,response_body::text`).
		WithArgs(request.TenantID, request.IdempotencyKey).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO behavior_baseline_definitions_v1`).
		WithArgs(request.TenantID, request.BaselineID, request.BaselineKind, request.EntityType, request.EntityID,
			request.AlgorithmVersion, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), request.RequestedBy).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT baseline_kind,entity_type,entity_id,lifecycle_state,revision,next_version`).
		WithArgs(request.TenantID, request.BaselineID).
		WillReturnRows(sqlmock.NewRows([]string{
			"baseline_kind", "entity_type", "entity_id", "lifecycle_state", "revision", "next_version",
			"algorithm_version", "sample_policy", "threshold_spec", "expected_consumers",
		}).AddRow("dynamic", "asset", "asset-a", "learning", int64(1), int64(1), "behavior-zscore-v1",
			`{"max_active_age_seconds":86400,"minimum_eligible_rows":100}`, `{"alert":3,"warning":2}`, `{flink-behavior-v1}`))
	mock.ExpectExec(`UPDATE behavior_baseline_definitions_v1 SET next_version=next_version\+1`).
		WithArgs(request.RequestedBy, request.TenantID, request.BaselineID, int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_build_jobs_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, request.BaselineKind, int64(1), int64(1),
			request.IdempotencyKey, sqlmock.AnyArg(), request.CandidateSHA256, request.WindowStart, request.WindowEnd,
			request.RequestedBy, request.Reason, request.TraceID).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_outbox_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, "baseline_build_job", sqlmock.AnyArg(),
			int64(1), "baseline.build.requested.v1", request.TenantID+":"+request.BaselineID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), request.TraceID).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_history_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, int64(1), sqlmock.AnyArg(), "learning", "learning",
			"baseline.build.requested.v1", request.Reason, request.RequestedBy, request.TraceID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_command_receipts_v1`).
		WithArgs(request.TenantID, request.IdempotencyKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := NewRepository().RequestBuildTx(context.Background(), tx, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "queued" || receipt.TargetVersion != 1 || receipt.JobID == "" || receipt.EventID == "" {
		t.Fatalf("unexpected build receipt: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteDynamicBuildRejectsFutureSampleBeforeWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	maxEventTime := end.Add(time.Second)
	result := DynamicSampleResult{
		TenantID: "tenant-a", JobID: "00000000-0000-0000-0000-000000000301", CandidateSHA256: repeatHex("a"),
		MaxEventTime: &maxEventTime, RowCount: 100, EligibleRowCount: 100, QualityStatus: "complete",
		SourceWatermark: map[string]interface{}{"offset": 1}, SourceQuerySHA256: repeatHex("b"),
		Statistics: map[string]interface{}{}, Provenance: map[string]interface{}{}, CompletedBy: "worker-a",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT baseline_id,baseline_kind,definition_revision,target_version,candidate_sha256`).
		WithArgs(result.TenantID, result.JobID).WillReturnRows(sqlmock.NewRows([]string{
		"baseline_id", "baseline_kind", "definition_revision", "target_version", "candidate_sha256",
		"requested_window_start", "requested_window_end", "status", "requested_by", "reason", "trace_id",
	}).AddRow("asset:asset-a", "dynamic", int64(1), int64(1), result.CandidateSHA256, start, end,
		"queued", "user-a", "rebuild", "trace-a"))
	mock.ExpectRollback()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewRepository().CompleteDynamicBuildTx(context.Background(), tx, result)
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("future sample was not rejected: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
