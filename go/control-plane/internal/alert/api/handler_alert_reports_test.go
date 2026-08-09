package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type stubAlertReportBuilder struct {
	model AlertReportModel
	err   error
}

func (s stubAlertReportBuilder) Build(context.Context, string, string, string) (AlertReportModel, error) {
	return s.model, s.err
}

type memoryAlertReportObjectStore struct {
	bucket      string
	key         string
	contentType string
	content     []byte
}

func (s *memoryAlertReportObjectStore) Put(_ context.Context, bucket, key string, reader io.Reader, _ int64, contentType string) error {
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.bucket, s.key, s.contentType, s.content = bucket, key, contentType, content
	return nil
}

func (s *memoryAlertReportObjectStore) Open(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func (s *memoryAlertReportObjectStore) Remove(_ context.Context, bucket, key string) error {
	if s.bucket == bucket && s.key == key {
		s.content = nil
	}
	return nil
}

func reportModel() AlertReportModel {
	return AlertReportModel{
		SchemaVersion: 2, ContractVersion: 1, SnapshotID: "alert:AL-1:revision:7",
		TenantID: "tenant-a", AlertID: "AL-1",
		Alert:    &service.AlertDetailDTO{AlertDTO: service.AlertDTO{AlertID: "AL-1", TenantID: "tenant-a", StateVersion: 7}},
		Evidence: []*service.EvidenceDTO{}, Assets: []map[string]interface{}{},
		ResponseActions: []map[string]interface{}{}, AuditTrail: []map[string]interface{}{},
		MissingSections:  []string{"asset_context"},
		SourceWatermarks: map[string]string{"clickhouse.alerts.state_version": "7"},
	}
}

func emptyAlertReportJobRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"job_id", "tenant_id", "alert_id", "format", "status", "revision", "requested_snapshot_id", "snapshot_sha256",
		"missing_sections", "source_watermarks", "object_bucket", "object_key", "mime_type", "artifact_sha256",
		"size_bytes", "error_message", "created_by", "created_at", "updated_at", "completed_at", "cancel_requested_at", "cancelled_at",
	})
}

func alertReportJobRows(status string, revision int64) *sqlmock.Rows {
	now := time.Now().UTC()
	return emptyAlertReportJobRows().AddRow(
		"alert-report-job-1", "tenant-a", "AL-1", "pdf", status, revision,
		"alert:AL-1:revision:7", "sha256:snapshot", "[]", `{}`,
		"", "", "", "", int64(0), "", "operator-a", now, now, nil, nil, nil,
	)
}

func alertReportJobRowsWithObject(status string, revision int64) *sqlmock.Rows {
	now := time.Now().UTC()
	return emptyAlertReportJobRows().AddRow(
		"alert-report-job-1", "tenant-a", "AL-1", "pdf", status, revision,
		"alert:AL-1:revision:7", "sha256:snapshot", "[]", `{}`,
		"report-artifacts", "tenant-a/alerts/AL-1/alert-report-job-1.pdf", "application/pdf", "sha256:artifact", int64(128),
		"temporary object cleanup failed", "operator-a", now, now, now, now, nil,
	)
}

func TestAlertReportRequiresExportPermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/export", strings.NewReader(`{}`))
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertRead}))
	recorder := httptest.NewRecorder()

	handler.CreateAlertReport(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCancelAlertReportCommitsRevisionHistoryOutboxAndAudit(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     string
		nextStatus string
	}{
		{name: "accepted is terminally cancelled", status: "accepted", nextStatus: "cancelled"},
		{name: "running enters cooperative cancellation", status: "running", nextStatus: "cancel_requested"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT job_id,request_hash FROM alert_report_control_requests").
				WillReturnRows(sqlmock.NewRows([]string{"job_id", "request_hash"}))
			mock.ExpectQuery("FROM alert_report_jobs WHERE tenant_id=\\$1 AND job_id=\\$2 FOR UPDATE").
				WithArgs("tenant-a", "alert-report-job-1").WillReturnRows(alertReportJobRows(test.status, 2))
			mock.ExpectExec("UPDATE alert_report_jobs SET status=\\$3,revision=\\$4").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("INSERT INTO alert_report_control_requests").WillReturnResult(sqlmock.NewResult(1, 1))
			expectAuditInsert(mock)
			mock.ExpectCommit()

			handler := NewHandler(nil, nil, zap.NewNop())
			handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/alert-report-job-1/cancel", strings.NewReader(
				`{"action_id":"alert-report-cancel","expected_revision":2,"reason":"confirmed operator cancellation"}`,
			))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Tenant-ID", "tenant-a")
			request.Header.Set("X-User-ID", "operator-a")
			request.Header.Set("Idempotency-Key", "alert-report-cancel-key-000001")
			request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertExport}))
			request = mux.SetURLVars(request, map[string]string{"id": "AL-1", "job_id": "alert-report-job-1"})
			recorder := httptest.NewRecorder()

			handler.CancelAlertReport(recorder, request)

			if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"status":"`+test.nextStatus+`"`) ||
				!strings.Contains(recorder.Body.String(), `"revision":3`) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompensateAlertReportRequiresResidualManifestAndCommitsControlState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT job_id,request_hash FROM alert_report_control_requests").
		WillReturnRows(sqlmock.NewRows([]string{"job_id", "request_hash"}))
	mock.ExpectQuery("FROM alert_report_jobs WHERE tenant_id=\\$1 AND job_id=\\$2 FOR UPDATE").
		WithArgs("tenant-a", "alert-report-job-1").WillReturnRows(alertReportJobRowsWithObject("partial", 5))
	mock.ExpectExec("UPDATE alert_report_jobs SET status=\\$3,revision=\\$4").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_control_requests").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/alert-report-job-1/compensations", strings.NewReader(
		`{"action_id":"alert-report-compensate","expected_revision":5,"reason":"confirmed residual object cleanup"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("X-User-ID", "operator-b")
	request.Header.Set("Idempotency-Key", "alert-report-compensate-key-0001")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertExport}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1", "job_id": "alert-report-job-1"})
	recorder := httptest.NewRecorder()

	handler.CompensateAlertReport(recorder, request)

	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"status":"compensating"`) ||
		!strings.Contains(recorder.Body.String(), `"revision":6`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertReportCancellationCleanupRemovesExactObjectBeforeTerminalState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT revision,cancellation_reason FROM alert_report_jobs").
		WithArgs("tenant-a", "alert-report-job-1", "cancel_requested").
		WillReturnRows(sqlmock.NewRows([]string{"revision", "cancellation_reason"}).AddRow(3, "confirmed cancellation"))
	mock.ExpectExec("UPDATE alert_report_jobs SET status=\\$3").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	store := &memoryAlertReportObjectStore{
		bucket: "report-artifacts", key: "tenant-a/alerts/AL-1/alert-report-job-1.pdf", content: []byte("temporary"),
	}
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	err = handler.cleanupAlertReportObject(context.Background(), alertReportJob{
		JobID: "alert-report-job-1", TenantID: "tenant-a", AlertID: "AL-1", Format: "pdf",
		Status: "cancel_requested", ObjectBucket: store.bucket, ObjectKey: store.key, CreatedBy: "operator-a",
	}, store, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if store.content != nil {
		t.Fatal("temporary object was not removed before cancelled terminal state")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAlertReportFreezesSnapshotAndCommitsJobOutboxAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM alert_report_jobs WHERE tenant_id=\\$1 AND idempotency_key=\\$2").
		WithArgs("tenant-a", "alert-report-key-000001").
		WillReturnRows(emptyAlertReportJobRows())
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_report_jobs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetAlertReportBuilder(stubAlertReportBuilder{model: reportModel()})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/export", strings.NewReader(
		`{"action_id":"alert-report-export","format":"pdf","snapshot_id":"alert:AL-1:revision:7","reason":"confirmed report export"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("Idempotency-Key", "alert-report-key-000001")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertExport}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertReport(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202 body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data  map[string]interface{} `json:"data"`
		Meta  httpx.ContractMeta     `json:"meta"`
		Error interface{}            `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["status"] != "accepted" || !strings.HasPrefix(envelope.Data["snapshot_sha256"].(string), "sha256:") {
		t.Fatalf("unexpected report response: %+v", envelope.Data)
	}
	if !envelope.Meta.Partial || envelope.Meta.SnapshotID != "alert:AL-1:revision:7" || envelope.Error != nil {
		t.Fatalf("unexpected report metadata: %+v error=%v", envelope.Meta, envelope.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAlertReportRejectsStaleSnapshotBeforeTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM alert_report_jobs WHERE tenant_id=\\$1 AND idempotency_key=\\$2").
		WithArgs("tenant-a", "alert-report-key-000002").
		WillReturnRows(emptyAlertReportJobRows())

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetAlertReportBuilder(stubAlertReportBuilder{model: reportModel()})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/export", strings.NewReader(
		`{"action_id":"alert-report-export","format":"pdf","snapshot_id":"alert:AL-1:revision:6","reason":"confirmed report export"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("Idempotency-Key", "alert-report-key-000002")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertExport}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertReport(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "SNAPSHOT_CONFLICT") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertReportFeatureFlagProvidesRollbackCutover(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetAlignmentFeatureFlags(false, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/reports/export", nil)
	recorder := httptest.NewRecorder()

	handler.CreateAlertReport(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", recorder.Code)
	}
}

func TestAlertReportArtifactsAreDeterministicAndObjectLevelValid(t *testing.T) {
	model := reportModel()
	snapshot, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "pdf", "docx"} {
		t.Run(format, func(t *testing.T) {
			first, mimeType, extension, err := buildAlertReportArtifact(format, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			second, _, _, err := buildAlertReportArtifact(format, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) || mimeType == "" || extension != format {
				t.Fatalf("artifact is not deterministic: format=%s mime=%s extension=%s", format, mimeType, extension)
			}
			switch format {
			case "json":
				if !json.Valid(first) {
					t.Fatal("JSON artifact is invalid")
				}
			case "pdf":
				if !bytes.HasPrefix(first, []byte("%PDF-1.4")) {
					t.Fatal("PDF artifact is invalid")
				}
			case "docx":
				reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
				if err != nil {
					t.Fatal(err)
				}
				names := map[string]bool{}
				for _, file := range reader.File {
					names[file.Name] = true
				}
				for _, required := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
					if !names[required] {
						t.Fatalf("DOCX missing %s", required)
					}
				}
			}
		})
	}
}

func TestAlertReportWorkerUploadsArtifactThenCommitsManifestAndCompletionEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	model := reportModel()
	snapshot, _ := json.Marshal(model)
	digest := snapshotSHA(snapshot)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE alert_report_jobs j").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "tenant_id", "alert_id", "format", "status", "revision", "snapshot", "snapshot_sha256",
			"object_bucket", "object_key", "mime_type", "artifact_sha256", "size_bytes", "created_by", "previous_status",
		}).AddRow("alert-report-job-1", "tenant-a", "AL-1", "json", "running", 2, string(snapshot), digest,
			"", "", "", "", 0, "analyst-a", "accepted"))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE alert_report_jobs SET status='completed'").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(3))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	store := &memoryAlertReportObjectStore{}
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetAlertReportObjectStore(store)

	if err := handler.processNextAlertReport(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.bucket != "report-artifacts" || store.key != "tenant-a/alerts/AL-1/alert-report-job-1.json" || !json.Valid(store.content) {
		t.Fatalf("unexpected object upload: bucket=%s key=%s content=%s", store.bucket, store.key, store.content)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertReportFailureReleasesLeaseForBoundedRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT tenant_id,alert_id,status,revision,attempts,created_by").
		WithArgs("alert-report-job-1").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "alert_id", "status", "revision", "attempts", "created_by"}).
			AddRow("tenant-a", "AL-1", "running", 2, 1, "analyst-a"))
	mock.ExpectExec("UPDATE alert_report_jobs").
		WithArgs("alert-report-job-1", "object upload failed", "accepted", int64(3), "running").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_report_job_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_report_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	err = handler.failAlertReportJob(context.Background(), "alert-report-job-1", errors.New("object upload failed"))
	if err == nil || err.Error() != "object upload failed" {
		t.Fatalf("unexpected failure result: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func snapshotSHA(content []byte) string {
	model := AlertReportModel{}
	_ = json.Unmarshal(content, &model)
	canonical, _ := json.Marshal(model)
	return opaqueSHA(canonical)
}

func opaqueSHA(content []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(content))
}
