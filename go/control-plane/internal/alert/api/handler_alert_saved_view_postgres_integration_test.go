package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// TestAlertSavedViewPostgresIntegration requires an explicitly guarded,
// disposable PostgreSQL instance. A generic DSN can never make this test write
// to a developer, shared or production database.
func TestAlertSavedViewPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ALERT_SAVED_VIEW_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ALERT_SAVED_VIEW_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_alert_saved_view_sentinel WHERE marker='ephemeral-only'`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing to run without saved-view ephemeral sentinel: marker=%q err=%v", marker, err)
	}

	suffix := uuid.NewString()
	tenantID := "saved-view-" + suffix
	otherTenantID := "saved-view-other-" + suffix
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	defer cleanupAlertSavedViewIntegration(t, db, tenantID, otherTenantID)

	createBody := alertSavedViewIntegrationBody("critical-alerts", 0, "critical")
	created := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-create-"+suffix, "alert-saved-view-create-0001-"+suffix, createBody)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	createdView := decodeAlertSavedViewIntegrationResponse(t, created)
	if createdView.ViewID == "" || createdView.Revision != 1 || createdView.IdempotentReuse {
		t.Fatalf("unexpected create response: %+v", createdView)
	}

	replayed := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-replay-"+suffix, "alert-saved-view-create-0001-"+suffix, createBody)
	replayedView := decodeAlertSavedViewIntegrationResponse(t, replayed)
	if replayed.Code != http.StatusCreated || replayedView.ViewID != createdView.ViewID || replayedView.Revision != 1 || !replayedView.IdempotentReuse {
		t.Fatalf("unexpected exact replay status=%d response=%+v", replayed.Code, replayedView)
	}

	collision := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-collision-"+suffix, "alert-saved-view-create-0001-"+suffix, alertSavedViewIntegrationBody("critical-alerts", 0, "high"))
	if collision.Code != http.StatusConflict {
		t.Fatalf("changed-payload replay status=%d body=%s", collision.Code, collision.Body.String())
	}

	updated := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-update-"+suffix, "alert-saved-view-update-0002-"+suffix, alertSavedViewIntegrationBody("critical-alerts", 1, "high"))
	updatedView := decodeAlertSavedViewIntegrationResponse(t, updated)
	if updated.Code != http.StatusCreated || updatedView.ViewID != createdView.ViewID || updatedView.Revision != 2 || updatedView.IdempotentReuse {
		t.Fatalf("unexpected update status=%d response=%+v", updated.Code, updatedView)
	}

	stale := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-stale-"+suffix, "alert-saved-view-stale-0003-"+suffix, alertSavedViewIntegrationBody("critical-alerts", 1, "medium"))
	if stale.Code != http.StatusConflict || !bytes.Contains(stale.Body.Bytes(), []byte(`"REVISION_CONFLICT"`)) {
		t.Fatalf("stale update status=%d body=%s", stale.Code, stale.Body.String())
	}

	otherList := performAlertSavedViewListIntegrationRequest(handler.ListAlertViews, otherTenantID, "trace-saved-view-other-"+suffix)
	if otherList.Code != http.StatusOK || !bytes.Contains(otherList.Body.Bytes(), []byte(`"total":0`)) {
		t.Fatalf("cross-tenant list status=%d body=%s", otherList.Code, otherList.Body.String())
	}
	otherCreated := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, otherTenantID, "operator-b", "trace-saved-view-other-create-"+suffix, "alert-saved-view-other-0001-"+suffix, createBody)
	otherView := decodeAlertSavedViewIntegrationResponse(t, otherCreated)
	if otherCreated.Code != http.StatusCreated || otherView.ViewID == createdView.ViewID || otherView.Revision != 1 {
		t.Fatalf("cross-tenant same-name create status=%d response=%+v", otherCreated.Code, otherView)
	}
	var originalRevision int64
	if err := db.QueryRow(`SELECT revision FROM alert_saved_views WHERE tenant_id=$1 AND view_id=$2::uuid`, tenantID, createdView.ViewID).Scan(&originalRevision); err != nil || originalRevision != 2 {
		t.Fatalf("cross-tenant write changed original revision=%d err=%v", originalRevision, err)
	}
	assertAlertSavedViewIntegrationFacts(t, db, tenantID, 1, 2)
	assertAlertSavedViewIntegrationFacts(t, db, otherTenantID, 1, 1)

	functionName := "codex_fail_saved_view_audit"
	if _, err := db.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.tenant_id = %s AND NEW.object_type = 'alert_saved_view' THEN
				RAISE EXCEPTION 'forced saved-view audit failure';
			END IF;
			RETURN NEW;
		END; $$ LANGUAGE plpgsql;
		CREATE TRIGGER codex_fail_saved_view_audit_once BEFORE INSERT ON audit_logs
		FOR EACH ROW EXECUTE FUNCTION %s()`, functionName, quotePostgresLiteral(tenantID), functionName)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(`DROP TRIGGER IF EXISTS codex_fail_saved_view_audit_once ON audit_logs; DROP FUNCTION IF EXISTS codex_fail_saved_view_audit()`)
	}()
	failed := performAlertSavedViewIntegrationRequest(handler.SaveAlertView, tenantID, "operator-a", "trace-saved-view-failed-"+suffix, "alert-saved-view-failed-0004-"+suffix, alertSavedViewIntegrationBody("rollback-view", 0, "critical"))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("forced audit failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	if _, err := db.Exec(`DROP TRIGGER codex_fail_saved_view_audit_once ON audit_logs; DROP FUNCTION codex_fail_saved_view_audit()`); err != nil {
		t.Fatal(err)
	}
	assertAlertSavedViewIntegrationFacts(t, db, tenantID, 1, 2)
	assertAlertSavedViewIntegrationFacts(t, db, otherTenantID, 1, 1)

	var historyTrace, outboxTrace, auditTrace, reconciledViewID string
	if err := db.QueryRow(`SELECT h.trace_id,o.trace_id,h.view_id::text
		FROM alert_saved_view_history h
		JOIN alert_saved_view_outbox o ON o.event_id=h.event_id
		WHERE h.tenant_id=$1 AND h.revision=2`, tenantID).Scan(&historyTrace, &outboxTrace, &reconciledViewID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COALESCE(NULLIF(trace_id,''),detail->>'trace_id')
		FROM audit_logs WHERE tenant_id=$1 AND object_id=$2 AND action='ALERT_VIEW_SAVED'
		ORDER BY created_at DESC LIMIT 1`, tenantID, reconciledViewID).Scan(&auditTrace); err != nil {
		t.Fatal(err)
	}
	if historyTrace == "" || historyTrace != outboxTrace || historyTrace != auditTrace {
		t.Fatalf("trace reconciliation failed history=%q outbox=%q audit=%q", historyTrace, outboxTrace, auditTrace)
	}
	t.Log("alert_saved_view_postgres_transaction=pass")
}

func alertSavedViewIntegrationBody(name string, expectedRevision int64, severity string) string {
	return fmt.Sprintf(`{"action_id":"alert-view-save","action":"save_view","target":%q,"reason":"persist operator alert workspace","expected_revision":%d,"detail":{"filters":{"severity":%q}}}`, name, expectedRevision, severity)
}

func performAlertSavedViewIntegrationRequest(handler func(http.ResponseWriter, *http.Request), tenantID, actor, traceID, idempotencyKey, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/views", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, tenantID)
	request = withUser(request, actor)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertWrite})
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, traceID)
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func performAlertSavedViewListIntegrationRequest(handler func(http.ResponseWriter, *http.Request), tenantID, traceID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/views", nil)
	request = withTenant(request, tenantID)
	request = withUser(request, "viewer-a")
	ctx := context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertRead})
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, traceID)
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func decodeAlertSavedViewIntegrationResponse(t *testing.T, recorder *httptest.ResponseRecorder) alertSavedViewDTO {
	t.Helper()
	var envelope struct {
		Data alertSavedViewDTO `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode saved-view response: %v body=%s", err, recorder.Body.String())
	}
	return envelope.Data
}

func assertAlertSavedViewIntegrationFacts(t *testing.T, db *sql.DB, tenantID string, expectedViews, expectedRevisions int) {
	t.Helper()
	var views, history, outbox, requests, audits int
	if err := db.QueryRow(`SELECT
		(SELECT count(*) FROM alert_saved_views WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_saved_view_history WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_saved_view_outbox WHERE tenant_id=$1),
		(SELECT count(*) FROM alert_saved_view_requests WHERE tenant_id=$1),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND object_type='alert_saved_view')`, tenantID).
		Scan(&views, &history, &outbox, &requests, &audits); err != nil {
		t.Fatal(err)
	}
	if views != expectedViews || history != expectedRevisions || outbox != expectedRevisions || requests != expectedRevisions || audits != expectedRevisions {
		t.Fatalf("atomic facts views=%d history=%d outbox=%d requests=%d audits=%d", views, history, outbox, requests, audits)
	}
}

func cleanupAlertSavedViewIntegration(t *testing.T, db *sql.DB, tenantIDs ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, tenantID := range tenantIDs {
		for _, query := range []string{
			`DELETE FROM audit_logs WHERE tenant_id=$1`,
			`DELETE FROM alert_saved_view_requests WHERE tenant_id=$1`,
			`DELETE FROM alert_saved_view_history WHERE tenant_id=$1`,
			`DELETE FROM alert_saved_view_outbox WHERE tenant_id=$1`,
			`DELETE FROM alert_saved_views WHERE tenant_id=$1`,
		} {
			if _, err := db.ExecContext(ctx, query, tenantID); err != nil {
				t.Errorf("cleanup tenant %s: %v", tenantID, err)
			}
		}
	}
}

func quotePostgresLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
