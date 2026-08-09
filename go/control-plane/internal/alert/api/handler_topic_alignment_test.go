package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

func topicAlignmentRequest(method, path, body, topic string, permissions ...string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-topic-test")
	request = request.WithContext(ctx)
	return mux.SetURLVars(request, map[string]string{"topic": topic})
}

func emptyTopicActionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"action_id", "action", "tenant_id", "topic", "target", "snapshot_id",
		"expected_revision", "revision", "status", "executor", "reason", "trace_id",
		"receipt", "error", "attempts", "requested_by", "created_at", "updated_at",
	})
}

func TestTopicSnapshotIsImmutablePartialAndNeverReadsSimulation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("FROM topic_scope_overrides")).
		WithArgs("tenant-a", "tunnel").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO topic_snapshot_manifests").
		WillReturnResult(sqlmock.NewResult(1, 1))

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(http.MethodGet, "/api/v1/topics/tunnel/snapshot", "", "tunnel", "topic:read")
	recorder := httptest.NewRecorder()
	handler.GetTopicSnapshot(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data map[string]interface{} `json:"data"`
		Meta httpx.ContractMeta     `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Meta.SnapshotID == "" || response.Data["snapshot_id"] != response.Meta.SnapshotID {
		t.Fatalf("snapshot identity mismatch: data=%v meta=%+v", response.Data["snapshot_id"], response.Meta)
	}
	if !response.Meta.Partial || len(response.Meta.MissingSections) == 0 {
		t.Fatalf("unavailable projections must be explicit partial data: %+v", response.Meta)
	}
	if response.Data["data_mode"] != "partial" {
		t.Fatalf("expected partial data mode, got %v", response.Data["data_mode"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTopicActionRejectsUnavailableHighRiskExecutor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(
		http.MethodPost,
		"/api/v1/topics/tunnel/actions",
		`{"action_id":"contain","target":"10.0.0.1","snapshot_id":"11111111-1111-1111-1111-111111111111","expected_revision":1,"reason":"confirmed containment"}`,
		"tunnel",
		"topic:write",
	)
	request.Header.Set("Idempotency-Key", "topic-action-key-000001")
	recorder := httptest.NewRecorder()
	handler.SubmitTopicAction(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "EXECUTOR_UNAVAILABLE") {
		t.Fatalf("expected fail-closed executor rejection, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTopicActionCommitsJobHistoryOutboxAndAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshotID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery("SELECT resource_revision FROM topic_snapshot_manifests").
		WithArgs(snapshotID, "tenant-a", "apt").
		WillReturnRows(sqlmock.NewRows([]string{"resource_revision"}).AddRow(int64(7)))
	mock.ExpectQuery("FROM topic_actions").
		WithArgs("tenant-a", "topic-action-key-000002").
		WillReturnRows(emptyTopicActionRows())
	mock.ExpectBegin()
	now := time.Now().UTC()
	mock.ExpectQuery("INSERT INTO topic_actions").
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))
	mock.ExpectExec("INSERT INTO topic_action_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO topic_action_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(
		http.MethodPost,
		"/api/v1/topics/apt/actions",
		`{"action_id":"trace","target":"APT-1","snapshot_id":"`+snapshotID+`","expected_revision":7,"reason":"confirmed investigation"}`,
		"apt",
		"topic:write",
	)
	request.Header.Set("Idempotency-Key", "topic-action-key-000002")
	recorder := httptest.NewRecorder()
	handler.SubmitTopicAction(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"accepted"`) ||
		!strings.Contains(recorder.Body.String(), `"action_id":"trace"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTopicActionRejectsIdempotencyKeyPayloadConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshotID := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT resource_revision FROM topic_snapshot_manifests").
		WithArgs(snapshotID, "tenant-a", "apt").
		WillReturnRows(sqlmock.NewRows([]string{"resource_revision"}).AddRow(int64(7)))
	mock.ExpectQuery("FROM topic_actions").
		WithArgs("tenant-a", "topic-action-key-conflict").
		WillReturnRows(emptyTopicActionRows().AddRow(
			"33333333-3333-3333-3333-333333333333", "trace", "tenant-a", "apt", "APT-1", snapshotID,
			int64(7), int64(1), "accepted", "internal_receipt", "confirmed investigation",
			"trace-topic-test", []byte(`{}`), []byte(`{}`), 0,
			"00000000-0000-0000-0000-000000000001", now, now,
		))

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(
		http.MethodPost,
		"/api/v1/topics/apt/actions",
		`{"action_id":"trace","target":"APT-2","snapshot_id":"`+snapshotID+`","expected_revision":7,"reason":"confirmed investigation"}`,
		"apt",
		"topic:write",
	)
	request.Header.Set("Idempotency-Key", "topic-action-key-conflict")
	recorder := httptest.NewRecorder()
	handler.SubmitTopicAction(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "IDEMPOTENCY_KEY_CONFLICT") {
		t.Fatalf("expected idempotency conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTopicActionRejectsConcurrentIdempotencyPayloadConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshotID := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT resource_revision FROM topic_snapshot_manifests").
		WithArgs(snapshotID, "tenant-a", "apt").
		WillReturnRows(sqlmock.NewRows([]string{"resource_revision"}).AddRow(int64(7)))
	mock.ExpectQuery("FROM topic_actions").
		WithArgs("tenant-a", "topic-action-key-race").
		WillReturnRows(emptyTopicActionRows())
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO topic_actions").
		WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()
	mock.ExpectQuery("FROM topic_actions").
		WithArgs("tenant-a", "topic-action-key-race").
		WillReturnRows(emptyTopicActionRows().AddRow(
			"33333333-3333-3333-3333-333333333333", "trace", "tenant-a", "apt", "APT-1", snapshotID,
			int64(7), int64(1), "accepted", "internal_receipt", "confirmed investigation",
			"trace-topic-test", []byte(`{}`), []byte(`{}`), 0,
			"00000000-0000-0000-0000-000000000001", now, now,
		))

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(
		http.MethodPost,
		"/api/v1/topics/apt/actions",
		`{"action_id":"trace","target":"APT-2","snapshot_id":"`+snapshotID+`","expected_revision":7,"reason":"confirmed investigation"}`,
		"apt",
		"topic:write",
	)
	request.Header.Set("Idempotency-Key", "topic-action-key-race")
	recorder := httptest.NewRecorder()
	handler.SubmitTopicAction(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "IDEMPOTENCY_KEY_CONFLICT") {
		t.Fatalf("expected concurrent idempotency conflict, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTopicActionWorkerPersistsReceiptHistoryOutboxAndAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	jobID := "33333333-3333-3333-3333-333333333333"
	snapshotID := "44444444-4444-4444-4444-444444444444"
	mock.ExpectBegin()
	mock.ExpectQuery("WITH candidate AS").
		WillReturnRows(emptyTopicActionRows().AddRow(
			jobID, "trace", "tenant-a", "apt", "APT-1", snapshotID,
			int64(7), int64(2), "running", "internal_receipt", "confirmed investigation",
			"trace-topic-test", []byte(`{}`), []byte(`{}`), 1,
			"00000000-0000-0000-0000-000000000001", now, now,
		))
	mock.ExpectExec("INSERT INTO topic_action_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE topic_actions SET status='completed'").
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectExec("INSERT INTO topic_action_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO topic_action_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO topic_action_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewSystemHandler(nil, db, zap.NewNop())
	if err := handler.processOneTopicAction(context.Background()); err != nil {
		t.Fatalf("process topic action: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetTopicActionReportsKafkaProducerAcknowledgement(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	jobID := "77777777-7777-4777-8777-777777777777"
	snapshotID := "88888888-8888-4888-8888-888888888888"
	mock.ExpectQuery("FROM topic_actions").
		WithArgs("tenant-a", "apt", jobID).
		WillReturnRows(emptyTopicActionRows().AddRow(
			jobID, "trace", "tenant-a", "apt", "APT-1", snapshotID,
			int64(7), int64(3), "completed", "internal_receipt", "confirmed investigation",
			"trace-topic-test", []byte(`{"receipt_id":"receipt-1"}`), []byte(`{}`), 1,
			"00000000-0000-0000-0000-000000000001", now, now,
		))
	mock.ExpectQuery("FROM topic_action_outbox WHERE job_id").
		WithArgs(jobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"count", "published", "attempts", "last_error", "published_at",
		}).AddRow(int64(2), int64(2), int64(2), "", now))

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := topicAlignmentRequest(
		http.MethodGet,
		"/api/v1/topics/apt/actions/"+jobID,
		"",
		"apt",
		"topic:read",
	)
	request = mux.SetURLVars(request, map[string]string{"topic": "apt", "job_id": jobID})
	recorder := httptest.NewRecorder()
	handler.GetTopicActionJob(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"published":2`) ||
		!strings.Contains(recorder.Body.String(), `"pending":0`) ||
		!strings.Contains(recorder.Body.String(), "producer_acked_without_observed_offset:2/2") {
		t.Fatalf("unexpected delivery response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
