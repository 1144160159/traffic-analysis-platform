package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newFeedbackWorkerTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func feedbackInboxFixture(t *testing.T) (modelFeedbackInboxItem, modelFeedbackPayload) {
	t.Helper()
	eventID := "f4b8ce9e-f91f-5d66-af6f-a24b559dce8b"
	payload := modelFeedbackPayload{
		EventID: eventID, EventType: "alert.feedback.v1",
		SchemaVersion: 1, AggregateVersion: 1, FeedbackID: eventID,
		AlertID: "alert-001", TenantID: "tenant-a", UserID: "",
		Label: "FP", ReasonCode: "benign_scanner", Comment: "known scanner",
		AddToWhitelist: true, AlertType: "scan", Severity: "medium",
		Labels: []string{"network"}, ModelVersion: "model-v1",
		RuleVersion: "rule-v2", Timestamp: 1760000000123,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return modelFeedbackInboxItem{
		FeedbackID: eventID, EventID: eventID,
		TenantID: "tenant-a", AlertID: "alert-001", UserID: "",
		Label: "FP", ReasonCode: "benign_scanner",
		ModelVersion: "model-v1", RuleVersion: "rule-v2",
		EventTimestamp: 1760000000123, Payload: raw,
	}, payload
}

func feedbackClickHouseRow(item modelFeedbackInboxItem, payload modelFeedbackPayload) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"feedback_id", "alert_id", "tenant_id", "user_id", "label",
		"reason_code", "comment", "add_to_whitelist", "alert_type", "severity",
		"model_version", "rule_version", "created_at",
	}).AddRow(
		item.FeedbackID, item.AlertID, item.TenantID, item.UserID, item.Label,
		item.ReasonCode, payload.Comment, payload.AddToWhitelist,
		payload.AlertType, payload.Severity, item.ModelVersion, item.RuleVersion,
		time.UnixMilli(item.EventTimestamp).UTC(),
	)
}

func TestModelFeedbackInboxProjectExactReplaySkipsInsert(t *testing.T) {
	pg, _ := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorker(pg, ch, nil)
	require.NoError(t, err)
	item, payload := feedbackInboxFixture(t)

	chMock.ExpectQuery("SELECT feedback_id").WithArgs(item.FeedbackID).
		WillReturnRows(feedbackClickHouseRow(item, payload))

	require.NoError(t, worker.project(context.Background(), item))
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxProjectInsertAndReadBack(t *testing.T) {
	pg, _ := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorker(pg, ch, nil)
	require.NoError(t, err)
	item, payload := feedbackInboxFixture(t)

	chMock.ExpectQuery("SELECT feedback_id").WithArgs(item.FeedbackID).
		WillReturnRows(sqlmock.NewRows([]string{
			"feedback_id", "alert_id", "tenant_id", "user_id", "label",
			"reason_code", "comment", "add_to_whitelist", "alert_type", "severity",
			"model_version", "rule_version", "created_at",
		}))
	chMock.ExpectExec("INSERT INTO traffic.alert_feedback").
		WithArgs(
			item.FeedbackID, item.AlertID, item.TenantID, item.UserID,
			item.Label, item.ReasonCode, payload.Comment, payload.AddToWhitelist,
			payload.AlertType, payload.Severity, item.ModelVersion, item.RuleVersion,
			time.UnixMilli(item.EventTimestamp).UTC(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	chMock.ExpectQuery("SELECT feedback_id").WithArgs(item.FeedbackID).
		WillReturnRows(feedbackClickHouseRow(item, payload))

	require.NoError(t, worker.project(context.Background(), item))
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxProjectRejectsCollision(t *testing.T) {
	pg, _ := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorker(pg, ch, nil)
	require.NoError(t, err)
	item, payload := feedbackInboxFixture(t)
	payload.Comment = "different"

	chMock.ExpectQuery("SELECT feedback_id").WithArgs(item.FeedbackID).
		WillReturnRows(feedbackClickHouseRow(item, payload))

	err = worker.project(context.Background(), item)
	require.ErrorContains(t, err, "collision")
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxDrainRetriesWithoutAcknowledgingProjection(t *testing.T) {
	pg, pgMock := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorker(pg, ch, nil)
	require.NoError(t, err)
	worker.workerID = "worker-test"
	item, _ := feedbackInboxFixture(t)

	pgMock.ExpectQuery("WITH candidates AS").WithArgs(worker.batchSize, worker.workerID).
		WillReturnRows(sqlmock.NewRows([]string{
			"feedback_id", "event_id", "tenant_id", "alert_id", "user_id",
			"label", "reason_code", "model_version", "rule_version",
			"event_timestamp_ms", "payload",
		}).AddRow(
			item.FeedbackID, item.EventID, item.TenantID, item.AlertID, item.UserID,
			item.Label, item.ReasonCode, item.ModelVersion, item.RuleVersion,
			item.EventTimestamp, string(item.Payload),
		))
	chMock.ExpectQuery("SELECT feedback_id").WithArgs(item.FeedbackID).
		WillReturnError(errors.New("ClickHouse unavailable"))
	pgMock.ExpectExec("UPDATE model_feedback_inbox").
		WithArgs(item.FeedbackID, worker.workerID, worker.maxAttempts, "reconcile ClickHouse model feedback in traffic.alert_feedback: ClickHouse unavailable").
		WillReturnResult(sqlmock.NewResult(0, 1))

	applied, err := worker.Drain(context.Background())
	require.NoError(t, err)
	require.Zero(t, applied)
	require.NoError(t, pgMock.ExpectationsWereMet())
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxProjectV2FailureIsFailClosed(t *testing.T) {
	pg, _ := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorkerWithOptions(
		pg, ch, nil, ModelFeedbackProjectionOptions{V2Enabled: true},
	)
	require.NoError(t, err)
	item, payload := feedbackInboxFixture(t)

	chMock.ExpectQuery("FROM traffic.alert_feedback").WithArgs(item.FeedbackID).
		WillReturnRows(feedbackClickHouseRow(item, payload))
	chMock.ExpectQuery("FROM traffic.alert_feedback_v2").WithArgs(item.FeedbackID).
		WillReturnRows(sqlmock.NewRows([]string{
			"feedback_id", "alert_id", "tenant_id", "user_id", "label",
			"reason_code", "comment", "add_to_whitelist", "alert_type", "severity",
			"model_version", "rule_version", "created_at",
		}))
	chMock.ExpectExec("INSERT INTO traffic.alert_feedback_v2").
		WithArgs(
			item.FeedbackID, item.AlertID, item.TenantID, item.UserID,
			item.Label, item.ReasonCode, payload.Comment, payload.AddToWhitelist,
			payload.AlertType, payload.Severity, item.ModelVersion, item.RuleVersion,
			time.UnixMilli(item.EventTimestamp).UTC(),
		).
		WillReturnError(errors.New("V2 unavailable"))

	err = worker.project(context.Background(), item)
	require.ErrorContains(t, err, "traffic.alert_feedback_v2")
	require.ErrorContains(t, err, "V2 unavailable")
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxDrainV2FailureDoesNotAcknowledgeInbox(t *testing.T) {
	pg, pgMock := newFeedbackWorkerTestDB(t)
	ch, chMock := newFeedbackWorkerTestDB(t)
	worker, err := NewModelFeedbackInboxWorkerWithOptions(
		pg, ch, nil, ModelFeedbackProjectionOptions{V2Enabled: true},
	)
	require.NoError(t, err)
	worker.workerID = "worker-v2-test"
	item, payload := feedbackInboxFixture(t)

	pgMock.ExpectQuery("WITH candidates AS").WithArgs(worker.batchSize, worker.workerID).
		WillReturnRows(sqlmock.NewRows([]string{
			"feedback_id", "event_id", "tenant_id", "alert_id", "user_id",
			"label", "reason_code", "model_version", "rule_version",
			"event_timestamp_ms", "payload",
		}).AddRow(
			item.FeedbackID, item.EventID, item.TenantID, item.AlertID, item.UserID,
			item.Label, item.ReasonCode, item.ModelVersion, item.RuleVersion,
			item.EventTimestamp, string(item.Payload),
		))
	chMock.ExpectQuery("FROM traffic.alert_feedback").WithArgs(item.FeedbackID).
		WillReturnRows(feedbackClickHouseRow(item, payload))
	chMock.ExpectQuery("FROM traffic.alert_feedback_v2").WithArgs(item.FeedbackID).
		WillReturnError(errors.New("V2 unavailable"))
	pgMock.ExpectExec("UPDATE model_feedback_inbox").WithArgs(
		item.FeedbackID, worker.workerID, worker.maxAttempts,
		"reconcile ClickHouse model feedback in traffic.alert_feedback_v2: V2 unavailable",
	).WillReturnResult(sqlmock.NewResult(0, 1))

	applied, err := worker.Drain(context.Background())
	require.NoError(t, err)
	require.Zero(t, applied)
	require.NoError(t, pgMock.ExpectationsWereMet())
	require.NoError(t, chMock.ExpectationsWereMet())
}

func TestModelFeedbackInboxRejectsUnapprovedV2Table(t *testing.T) {
	pg, _ := newFeedbackWorkerTestDB(t)
	ch, _ := newFeedbackWorkerTestDB(t)
	_, err := NewModelFeedbackInboxWorkerWithOptions(
		pg, ch, nil,
		ModelFeedbackProjectionOptions{V2Enabled: true, V2Table: "traffic.alert_feedback_local"},
	)
	require.ErrorContains(t, err, "unsupported model feedback V2 table")
}

func TestValidateModelFeedbackInboxRejectsPayloadColumnDrift(t *testing.T) {
	item, _ := feedbackInboxFixture(t)
	item.TenantID = "tenant-b"
	_, err := validateModelFeedbackInboxItem(item)
	require.ErrorContains(t, err, "payload/column mismatch")
}
