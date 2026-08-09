package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const alertBatchTestSigningSecret = "alert-batch-test-selection-signing-secret-0001"

func TestNormalizeAlertBatchSelectionRejectsMissingRevisionAndDuplicate(t *testing.T) {
	if _, err := normalizeAlertBatchSelection("snapshot-valid", []AlertBatchSelectionItem{{AlertID: "alert-1"}}); err == nil {
		t.Fatal("selection without a positive state revision must fail")
	}
	if _, err := normalizeAlertBatchSelection("snapshot-valid", []AlertBatchSelectionItem{{AlertID: "alert-1", StateVersion: 1}, {AlertID: " alert-1 ", StateVersion: 2}}); err == nil {
		t.Fatal("duplicate alert identity must fail")
	}
}

func TestNormalizeAlertBatchSelectionPreservesFrozenOrder(t *testing.T) {
	items, err := normalizeAlertBatchSelection("snapshot-valid", []AlertBatchSelectionItem{{AlertID: " alert-b ", StateVersion: 2}, {AlertID: "alert-a", StateVersion: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].AlertID != "alert-b" || items[1].AlertID != "alert-a" {
		t.Fatalf("frozen order changed: %#v", items)
	}
	stable := stableAlertBatchItems(items)
	if stable[0].AlertID != "alert-a" || stable[1].AlertID != "alert-b" {
		t.Fatalf("stable evidence order unexpected: %#v", stable)
	}
}

func TestAlertBatchSelectionTokenIsDeterministicAndTenantBound(t *testing.T) {
	handler := NewAlertBatchAssignmentHandler(nil, nil, true, alertBatchTestSigningSecret)
	token, err := handler.deriveSelectionToken("tenant-a", "be8e925d-3caa-4a96-ac23-c81618ef9bc4")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := handler.deriveSelectionToken("tenant-a", "be8e925d-3caa-4a96-ac23-c81618ef9bc4")
	if err != nil {
		t.Fatal(err)
	}
	otherTenant, err := handler.deriveSelectionToken("tenant-b", "be8e925d-3caa-4a96-ac23-c81618ef9bc4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(token); err != nil || replayed != token || otherTenant == token {
		t.Fatalf("selection token is not deterministic and tenant-bound: token=%q replayed=%q other=%q", token, replayed, otherTenant)
	}
	digest := alertBatchTokenSHA(token)
	if len(digest) != 64 || strings.Contains(digest, token) {
		t.Fatalf("invalid stable token digest: %q", digest)
	}
}

func TestAlertBatchAssignmentDisabledFailsClosed(t *testing.T) {
	handler := NewAlertBatchAssignmentHandler(nil, nil, false, "")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/batches/selections", strings.NewReader(`{"snapshot_id":"snapshot-valid","items":[{"alert_id":"a","state_version":1}]}`))
	handler.CreateSelection(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "FEATURE_DISABLED") {
		t.Fatalf("disabled feature must fail closed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertBatchAssignmentRequiresIdentityAndWriteScope(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAlertBatchAssignmentHandler(db, nil, true, alertBatchTestSigningSecret)
	body := `{"snapshot_id":"snapshot-valid","items":[{"alert_id":"a","state_version":1}]}`

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/batches/selections", strings.NewReader(body))
	unauthenticated.Header.Set("Idempotency-Key", "alert-batch-selection-0001")
	recorder := httptest.NewRecorder()
	handler.CreateSelection(recorder, unauthenticated)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing identity status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	denied := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/batches/selections", strings.NewReader(body))
	denied.Header.Set("Idempotency-Key", "alert-batch-selection-0002")
	ctx := context.WithValue(denied.Context(), httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "actor-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertRead})
	denied = denied.WithContext(ctx)
	recorder = httptest.NewRecorder()
	handler.CreateSelection(recorder, denied)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "alert:write") {
		t.Fatalf("read-only identity status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertBatchAssignmentQueryDoesNotRequireIdempotencyKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAlertBatchAssignmentHandler(db, nil, true, alertBatchTestSigningSecret)
	mock.ExpectQuery("SELECT batch_id::text").WithArgs("tenant-a", "be8e925d-3caa-4a96-ac23-c81618ef9bc4").WillReturnError(sql.ErrNoRows)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/batches/assign/be8e925d-3caa-4a96-ac23-c81618ef9bc4", nil)
	request = mux.SetURLVars(request, map[string]string{"batch_id": "be8e925d-3caa-4a96-ac23-c81618ef9bc4"})
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "actor-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, []string{authmodel.ScopeAlertWrite})
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	handler.GetAssignment(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("query without idempotency key status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
