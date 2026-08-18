package baseline

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordActivationAckDoesNotActivateUntilRequiredExactSetIsAcked(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ack := testActivationAck()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status,candidate_sha256`).WithArgs(ack.TenantID, ack.BaselineID,
		ack.BaselineVersion, ack.ConsumerID).WillReturnRows(sqlmock.NewRows([]string{
		"status", "candidate", "event", "ack",
	}).AddRow("pending", ack.CandidateSHA256, "", ""))
	mock.ExpectQuery(`SELECT version_id::text,lifecycle_state,quality_status`).WithArgs(
		ack.TenantID, ack.BaselineID, ack.BaselineVersion).WillReturnRows(sqlmock.NewRows([]string{
		"version_id", "state", "quality", "snapshot", "candidate", "revision",
	}).AddRow("00000000-0000-0000-0000-000000000501", "frozen", "complete", ack.SnapshotSHA256,
		ack.CandidateSHA256, int64(1)))
	mock.ExpectExec(`UPDATE behavior_baseline_activation_targets_v1 SET status='acked'`).
		WithArgs(ack.EventID, ack.AckSHA256, ack.AppliedAt, ack.TenantID, ack.BaselineID,
			ack.BaselineVersion, ack.ConsumerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT consumer_id,status FROM behavior_baseline_activation_targets_v1`).
		WithArgs(ack.TenantID, ack.BaselineID, ack.BaselineVersion).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_id", "status"}).
			AddRow(ack.ConsumerID, "acked").AddRow("model-service-v1", "pending"))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().RecordActivationAckTx(context.Background(), tx, ack)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.LifecycleState != "frozen" || len(receipt.PendingConsumers) != 1 || receipt.PendingConsumers[0] != "model-service-v1" {
		t.Fatalf("partial ACK falsely activated baseline: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordActivationAckActivatesAfterLastRequiredAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ack := testActivationAck()
	versionID := "00000000-0000-0000-0000-000000000502"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status,candidate_sha256`).WillReturnRows(sqlmock.NewRows([]string{
		"status", "candidate", "event", "ack",
	}).AddRow("pending", ack.CandidateSHA256, "", ""))
	mock.ExpectQuery(`SELECT version_id::text,lifecycle_state,quality_status`).WillReturnRows(sqlmock.NewRows([]string{
		"version_id", "state", "quality", "snapshot", "candidate", "revision",
	}).AddRow(versionID, "frozen", "complete", ack.SnapshotSHA256, ack.CandidateSHA256, int64(1)))
	mock.ExpectExec(`UPDATE behavior_baseline_activation_targets_v1 SET status='acked'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT consumer_id,status FROM behavior_baseline_activation_targets_v1`).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_id", "status"}).
			AddRow(ack.ConsumerID, "acked").AddRow("model-service-v1", "acked"))
	mock.ExpectQuery(`SELECT lifecycle_state,revision,active_version FROM behavior_baseline_definitions_v1`).
		WillReturnRows(sqlmock.NewRows([]string{"state", "revision", "active"}).AddRow("frozen", int64(1), nil))
	mock.ExpectExec(`UPDATE behavior_baseline_versions_v1 SET lifecycle_state='active'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_definitions_v1 SET lifecycle_state='active'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_approval_requests_v1 SET status='consumed'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_outbox_v1`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_history_v1`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().RecordActivationAckTx(context.Background(), tx, ack)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.LifecycleState != "active" || receipt.ActivationEventID == "" || len(receipt.AckedConsumers) != 2 {
		t.Fatalf("last ACK did not activate exact target set: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordActivationAckReplayReportsPersistedActiveState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ack := testActivationAck()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status,candidate_sha256`).
		WithArgs(ack.TenantID, ack.BaselineID, ack.BaselineVersion, ack.ConsumerID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "candidate", "event", "ack"}).
			AddRow("acked", ack.CandidateSHA256, ack.EventID, ack.AckSHA256))
	mock.ExpectQuery(`SELECT consumer_id,status FROM behavior_baseline_activation_targets_v1`).
		WithArgs(ack.TenantID, ack.BaselineID, ack.BaselineVersion).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_id", "status"}).AddRow(ack.ConsumerID, "acked"))
	mock.ExpectQuery(`SELECT lifecycle_state FROM behavior_baseline_versions_v1`).
		WithArgs(ack.TenantID, ack.BaselineID, ack.BaselineVersion).
		WillReturnRows(sqlmock.NewRows([]string{"lifecycle_state"}).AddRow("active"))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().RecordActivationAckTx(context.Background(), tx, ack)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.LifecycleState != "active" {
		t.Fatalf("replayed ACK lost persisted active state: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordActivationAckRetiresPreviousVersionBeforePublishingActivation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ack := testActivationAck()
	ack.BaselineVersion = 2
	newVersionID := "00000000-0000-0000-0000-000000000520"
	oldVersionID := "00000000-0000-0000-0000-000000000519"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status,candidate_sha256`).WillReturnRows(sqlmock.NewRows([]string{
		"status", "candidate", "event", "ack",
	}).AddRow("pending", ack.CandidateSHA256, "", ""))
	mock.ExpectQuery(`SELECT version_id::text,lifecycle_state,quality_status`).WillReturnRows(sqlmock.NewRows([]string{
		"version_id", "state", "quality", "snapshot", "candidate", "revision",
	}).AddRow(newVersionID, "frozen", "complete", ack.SnapshotSHA256, ack.CandidateSHA256, int64(7)))
	mock.ExpectExec(`UPDATE behavior_baseline_activation_targets_v1 SET status='acked'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT consumer_id,status FROM behavior_baseline_activation_targets_v1`).
		WillReturnRows(sqlmock.NewRows([]string{"consumer_id", "status"}).AddRow(ack.ConsumerID, "acked"))
	mock.ExpectQuery(`SELECT lifecycle_state,revision,active_version FROM behavior_baseline_definitions_v1`).
		WillReturnRows(sqlmock.NewRows([]string{"state", "revision", "active"}).AddRow("active", int64(7), int64(1)))
	mock.ExpectQuery(`SELECT version_id::text,snapshot_sha256`).
		WithArgs(ack.TenantID, ack.BaselineID, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"version_id", "snapshot_sha256"}).AddRow(oldVersionID, repeatHex("d")))
	mock.ExpectExec(`UPDATE behavior_baseline_versions_v1 SET lifecycle_state='retired'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// SQL expectations are ordered: the retirement event must be inserted before
	// the new active-version event for this Kafka partition.
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_outbox_v1`).
		WithArgs(sqlmock.AnyArg(), ack.TenantID, ack.BaselineID, "baseline_version", oldVersionID,
			int64(1), "baseline.version.retired.v1", ack.TenantID+":"+ack.BaselineID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), ack.TraceID).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_versions_v1 SET lifecycle_state='active'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_definitions_v1 SET lifecycle_state='active'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_approval_requests_v1 SET status='consumed'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_outbox_v1`).
		WithArgs(sqlmock.AnyArg(), ack.TenantID, ack.BaselineID, "baseline_version", newVersionID,
			int64(2), "baseline.version.activated.v1", ack.TenantID+":"+ack.BaselineID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), ack.TraceID).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_history_v1`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().RecordActivationAckTx(context.Background(), tx, ack)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.LifecycleState != "active" || receipt.ActivationEventID == "" {
		t.Fatalf("replacement activation failed: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRollbackCreatesNewFrozenVersionWithoutSwitchingOnlineState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := RollbackRequest{
		TenantID: "tenant-a", BaselineID: "asset:asset-a", TargetStableVersion: 1, ExpectedRevision: 9,
		CandidateSHA256: repeatHex("a"), IdempotencyKey: "rollback-request-a", RequestedBy: "user-a",
		Reason: "restore last acknowledged stable version", TraceID: "trace-rollback-a",
	}
	windowStart := time.Unix(100, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	sampleID := "00000000-0000-0000-0000-000000000530"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT command_type,request_sha256,response_body::text`).
		WithArgs(request.TenantID, request.IdempotencyKey).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT lifecycle_state,revision,next_version,active_version,previous_stable_version`).
		WithArgs(request.TenantID, request.BaselineID).
		WillReturnRows(sqlmock.NewRows([]string{"state", "revision", "next", "active", "previous"}).
			AddRow("active", int64(9), int64(3), int64(2), int64(1)))
	mock.ExpectQuery(`SELECT baseline_kind,COALESCE\(sample_snapshot_id::text,''\),window_start,window_end`).
		WithArgs(request.TenantID, request.BaselineID, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "sample", "start", "end", "algorithm", "thresholds",
			"statistics", "quality", "snapshot"}).AddRow("dynamic", sampleID, windowStart, windowEnd,
			"zscore-v1", `{"warning":2,"alert":3}`, `{"bytes":{"mean":10,"stddev":2}}`,
			"complete", repeatHex("b")))
	mock.ExpectExec(`INSERT INTO behavior_baseline_versions_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, int64(3), "dynamic", int64(9),
			sampleID, windowStart, windowEnd, "zscore-v1", `{"warning":2,"alert":3}`,
			`{"bytes":{"mean":10,"stddev":2}}`, sqlmock.AnyArg(), request.CandidateSHA256,
			int64(1), request.RequestedBy).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE behavior_baseline_definitions_v1 SET next_version=next_version\+1`).
		WithArgs(request.RequestedBy, request.TenantID, request.BaselineID, int64(9), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_outbox_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, "baseline_version", sqlmock.AnyArg(),
			int64(3), "baseline.version.frozen.v1", request.TenantID+":"+request.BaselineID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), request.TraceID).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_lifecycle_history_v1`).
		WithArgs(sqlmock.AnyArg(), request.TenantID, request.BaselineID, int64(9), sqlmock.AnyArg(),
			"active", "active", "baseline.rollback-version.created.v1", request.Reason,
			request.RequestedBy, request.TraceID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO behavior_baseline_command_receipts_v1`).
		WithArgs(request.TenantID, request.IdempotencyKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	tx, _ := db.BeginTx(context.Background(), nil)
	receipt, err := NewRepository().RequestRollbackTx(context.Background(), tx, request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RollbackVersion != 3 || receipt.TargetStableVersion != 1 || receipt.LifecycleState != "frozen" ||
		receipt.SnapshotSHA256 == "" || receipt.EventID == "" {
		t.Fatalf("rollback did not create an immutable frozen candidate: %#v", receipt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func testActivationAck() ActivationAck {
	return ActivationAck{
		EventID: "00000000-0000-0000-0000-000000000510", TenantID: "tenant-a",
		BaselineID: "asset:asset-a", BaselineVersion: 1, ConsumerID: "flink-behavior-v1",
		CandidateSHA256: repeatHex("a"), SnapshotSHA256: repeatHex("b"), AckSHA256: repeatHex("c"),
		AppliedAt: time.Unix(200, 0).UTC(), TraceID: "trace-a",
	}
}
