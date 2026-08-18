package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type stubAlertCampaignLookup struct {
	exists bool
	err    error
}

func (s stubAlertCampaignLookup) Exists(context.Context, string, string) (bool, error) {
	return s.exists, s.err
}

func campaignLinkRequest(body string, permissions ...string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/campaign-links", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, "tenant-a")
	request.Header.Set("Idempotency-Key", "campaign-link-key-000001")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, permissions))
	return mux.SetURLVars(request, map[string]string{"id": "AL-1"})
}

func expectAuditInsert(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WithArgs("audit_logs", "user_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("audit_logs", "event_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestAlertCampaignLinkRequiresBothScopes(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	request := campaignLinkRequest(`{"campaign_id":"CAM-1","expected_revision":0,"reason":"confirmed link"}`, authmodel.ScopeAlertWrite)
	recorder := httptest.NewRecorder()

	handler.LinkAlertToCampaign(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertCampaignLinkFeatureFlagProvidesRollbackCutover(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetAlignmentFeatureFlags(true, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/campaign-links", nil)
	recorder := httptest.NewRecorder()

	handler.LinkAlertToCampaign(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", recorder.Code)
	}
}

func TestAlertCampaignLinkDoesNotRevealCrossTenantCampaign(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetCampaignLookup(stubAlertCampaignLookup{exists: false})
	request := campaignLinkRequest(
		`{"campaign_id":"CAM-OTHER-TENANT","expected_revision":0,"reason":"confirmed link"}`,
		authmodel.ScopeAlertWrite, authmodel.ScopeCampaignWrite,
	)
	recorder := httptest.NewRecorder()

	handler.LinkAlertToCampaign(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertCampaignLinkPersistsRelationHistoryOutboxAndAuditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,state_version FROM campaign_workbench_state").
		WithArgs("tenant-a", "CAM-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "state_version"}))
	mock.ExpectQuery("FROM campaign_alert_links WHERE tenant_id=\\$1 AND idempotency_key=\\$2").
		WithArgs("tenant-a", "campaign-link-key-000001").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "created_at", "updated_at"}))
	mock.ExpectQuery("FROM campaign_alert_links WHERE tenant_id=\\$1 AND campaign_id=\\$2 AND alert_id=\\$3").
		WithArgs("tenant-a", "CAM-1", "AL-1").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "created_at", "updated_at"}))
	mock.ExpectExec("INSERT INTO campaign_alert_links").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "CAM-1", "AL-1", int64(1), "confirmed link", "campaign-link-key-000001", "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_history").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "tenant-a", "CAM-1", "AL-1", int64(1), sqlmock.AnyArg(), "").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO campaign_alert_link_outbox").
		WithArgs(sqlmock.AnyArg(), "tenant-a", sqlmock.AnyArg(), int64(1), "tenant-a:CAM-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetCampaignLookup(stubAlertCampaignLookup{exists: true})
	request := campaignLinkRequest(
		`{"campaign_id":"CAM-1","expected_revision":0,"reason":"confirmed link"}`,
		authmodel.ScopeAlertWrite, authmodel.ScopeCampaignWrite,
	)
	recorder := httptest.NewRecorder()

	handler.LinkAlertToCampaign(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data  alertCampaignLinkDTO `json:"data"`
		Meta  httpx.ContractMeta   `json:"meta"`
		Error interface{}          `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.CampaignID != "CAM-1" || envelope.Data.AlertID != "AL-1" || envelope.Data.Revision != 1 {
		t.Fatalf("unexpected relation: %+v", envelope.Data)
	}
	if envelope.Meta.ContractVersion != 1 || envelope.Meta.SnapshotID == "" || envelope.Error != nil {
		t.Fatalf("invalid contract envelope: %+v error=%v", envelope.Meta, envelope.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertCampaignLinkIdempotentReplayReturnsSameRelationWithoutNewOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,state_version FROM campaign_workbench_state").
		WithArgs("tenant-a", "CAM-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "state_version"}).AddRow("active", 7))
	mock.ExpectQuery("FROM campaign_alert_links WHERE tenant_id=\\$1 AND idempotency_key=\\$2").
		WithArgs("tenant-a", "campaign-link-key-000001").
		WillReturnRows(sqlmock.NewRows([]string{"relation_id", "tenant_id", "alert_id", "campaign_id", "status", "revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-4000-8000-000000000001", "tenant-a", "AL-1", "CAM-1", "linked", 1, now, now))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	handler.SetCampaignLookup(stubAlertCampaignLookup{exists: true})
	request := campaignLinkRequest(
		`{"campaign_id":"CAM-1","expected_revision":0,"reason":"repeat confirmed link"}`,
		authmodel.ScopeAlertWrite, authmodel.ScopeCampaignWrite,
	)
	recorder := httptest.NewRecorder()

	handler.LinkAlertToCampaign(recorder, request)

	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"idempotent_reuse":true`)) {
		t.Fatalf("expected idempotent 200 response, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
