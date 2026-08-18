package baseline

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEvaluateTxBindsActiveVersionAndEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	windowStart := time.Unix(100, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	request := EvaluationRequest{TenantID: "tenant-a", BaselineID: "asset:asset-a",
		MetricName: "bytes_per_session", ObservedValue: 14, ObservedAt: windowEnd.Add(time.Hour),
		EvidenceRefs: []string{"event-a"}, TraceID: "trace-a"}
	snapshotSHA := repeatHex("d")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT lifecycle_state,baseline_kind,active_version,sample_policy::text`).
		WithArgs(request.TenantID, request.BaselineID).WillReturnRows(sqlmock.NewRows([]string{
		"state", "kind", "active_version", "policy",
	}).AddRow("active", "dynamic", int64(3), `{"max_active_age_seconds":86400}`))
	mock.ExpectQuery(`SELECT lifecycle_state,quality_status,snapshot_sha256,statistics::text`).
		WithArgs(request.TenantID, request.BaselineID, int64(3)).WillReturnRows(sqlmock.NewRows([]string{
		"state", "quality", "snapshot", "statistics", "thresholds", "window_start", "window_end",
	}).AddRow("active", "complete", snapshotSHA, `{"bytes_per_session":{"mean":10,"stddev":2}}`,
		`{"warning_multiplier":2,"alert_multiplier":3}`, windowStart, windowEnd))
	mock.ExpectExec(`INSERT INTO behavior_baseline_drift_evaluations_v1`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().EvaluateTx(context.Background(), tx, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.BaselineVersion != 3 || receipt.SnapshotSHA256 != snapshotSHA || receipt.Disposition != "warning" ||
		receipt.DeviationScore == nil || *receipt.DeviationScore != 2 || len(receipt.EvidenceRefs) != 1 {
		t.Fatalf("evaluation lost version, threshold or evidence identity: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateTxPersistsMissingActiveBaselineInsteadOfReturningLowRisk(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := EvaluationRequest{TenantID: "tenant-a", BaselineID: "asset:asset-a", MetricName: "bytes_per_session",
		ObservedValue: 1, ObservedAt: time.Unix(200, 0).UTC(), EvidenceRefs: []string{"event-a"}, TraceID: "trace-a"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT lifecycle_state,baseline_kind,active_version,sample_policy::text`).
		WillReturnRows(sqlmock.NewRows([]string{"state", "kind", "active", "policy"}).
			AddRow("learning", "dynamic", nil, `{"max_active_age_seconds":86400}`))
	mock.ExpectExec(`INSERT INTO behavior_baseline_drift_evaluations_v1`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().EvaluateTx(context.Background(), tx, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != "missing" || receipt.QualityStatus != "unavailable" || receipt.FailureCode != "BASELINE_NOT_ACTIVE" {
		t.Fatalf("missing baseline was not fail-visible: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
