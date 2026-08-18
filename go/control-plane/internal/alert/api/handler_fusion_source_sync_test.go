package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

type fusionReadinessGateStub struct{ err error }

func (stub fusionReadinessGateStub) AssertReadyTx(context.Context, *sql.Tx, string) error {
	return stub.err
}

type fusionCommandPublisherStub struct {
	receipt commonkafka.BrokerReceipt
	err     error
}

func (stub fusionCommandPublisherStub) Send(context.Context, string, []byte, ...commonkafka.MessageHeader) (commonkafka.BrokerReceipt, error) {
	return stub.receipt, stub.err
}

func TestSyncFusionSourceV1CommitsJobOutboxAndAuditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	handler.SetFusionV1FeatureFlag(true)
	handler.SetFusionV1Runtime(strings.Repeat("a", 64), fusionReadinessGateStub{}, fusionCommandPublisherStub{})
	start := time.Unix(1000, 0).UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"window_start": start.Format(time.RFC3339), "window_end": start.Add(time.Hour).Format(time.RFC3339),
		"expected_source_version": 0, "reason": "manual four-source refresh",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fusion/sources/traffic/sync", strings.NewReader(string(body)))
	req.Header.Set("Idempotency-Key", "fusion-sync-a")
	req = mux.SetURLVars(requestWithClaims(req, testClaims{
		userID: "00000000-0000-0000-0000-000000000301", tenantID: "tenant-a", username: "fusion-writer",
		roles: []string{"admin"}, permissions: []string{"rule:write"},
	}), map[string]string{"id": "traffic"})
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM fusion_source_sync_jobs j JOIN fusion_projection_outbox o`).
		WithArgs(fusion.SourceSyncEventType, "tenant-a", "fusion-sync-a").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO fusion_source_sync_jobs`).
		WithArgs(sqlmock.AnyArg(), "tenant-a", "traffic", "flow", "fusion-sync-a", sqlmock.AnyArg(), sqlmock.AnyArg(),
			start, start.Add(time.Hour), int64(0), "manual four-source refresh", "00000000-0000-0000-0000-000000000301", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO fusion_projection_outbox`).
		WithArgs(sqlmock.AnyArg(), "tenant-a", sqlmock.AnyArg(), fusion.SourceSyncEventType, sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "tenant-a", "00000000-0000-0000-0000-000000000301",
			"FUSION_SOURCE_SYNC_REQUESTED", "fusion_source_sync_job", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	handler.SyncFusionSource(recorder, req)
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"status":"queued"`) ||
		!strings.Contains(recorder.Body.String(), `"outbox_status":"pending"`) {
		t.Fatalf("expected durable accepted receipt, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFusionSourceV1FailsBeforeWritesWhenConsumerLeaseIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	handler.SetFusionV1FeatureFlag(true)
	handler.SetFusionV1Runtime(strings.Repeat("a", 64), fusionReadinessGateStub{err: fusion.ErrProjectionNotReady}, fusionCommandPublisherStub{})
	start := time.Unix(1000, 0).UTC()
	body := `{"window_start":"` + start.Format(time.RFC3339) + `","window_end":"` + start.Add(time.Hour).Format(time.RFC3339) + `","reason":"refresh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fusion/sources/traffic/sync", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", "fusion-sync-a")
	req = mux.SetURLVars(requestWithClaims(req, testClaims{
		userID: "00000000-0000-0000-0000-000000000301", tenantID: "tenant-a", roles: []string{"admin"}, permissions: []string{"rule:write"},
	}), map[string]string{"id": "traffic"})
	mock.ExpectBegin()
	mock.ExpectRollback()
	recorder := httptest.NewRecorder()
	handler.SyncFusionSource(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "FUSION_CONSUMER_NOT_READY") {
		t.Fatalf("expected fail-closed readiness response, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
