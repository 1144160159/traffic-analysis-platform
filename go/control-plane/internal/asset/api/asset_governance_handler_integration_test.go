package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func TestAssetGovernanceHTTPPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ASSET_GOVERNANCE_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ASSET_GOVERNANCE_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_governance_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	const tenant = "asset-governance-http-integration"
	assetID := uuid.NewString()
	cleanup := func() {
		for _, query := range []string{`DELETE FROM asset_governance_control_requests WHERE tenant_id=$1`, `DELETE FROM asset_governance_work_order_history WHERE tenant_id=$1`, `DELETE FROM asset_governance_outbox WHERE tenant_id=$1`, `DELETE FROM asset_governance_work_orders WHERE tenant_id=$1`, `DELETE FROM audit_logs WHERE tenant_id=$1`, `DELETE FROM assets WHERE tenant_id=$1`, `DELETE FROM tenants WHERE tenant_id=$1`} {
			if _, err := db.Exec(query, tenant); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	}
	cleanup()
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Governance HTTP')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets(asset_id,tenant_id,mac_address,asset_type,status,source,revision,lifecycle_state) VALUES($1,$2,'02:10:20:30:40:50','server','active','manual',1,'managed')`, assetID, tenant); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	const signingKey = "asset-governance-http-integration-signing-key"
	handler := NewHTTPHandler(service.New(&config.Config{Auth: config.AuthConfig{JWTSigningKey: signingKey}, Governance: config.AssetGovernanceConfig{Enabled: true}}, repo, zap.NewNop()), zap.NewNop())
	payload, _ := json.Marshal(map[string]any{"action_id": "asset-governance-work-order-create", "target_lifecycle_state": "isolated", "owner": "asset-test-user", "due_at": time.Now().UTC().Add(24 * time.Hour), "evidence_required": true, "reason": "http integration verified compromise", "expected_asset_revision": 1})
	create := httptest.NewRequest(http.MethodPost, "/api/v1/assets/"+assetID+"/governance/work-orders", bytes.NewReader(payload))
	create.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenant, []string{authmodel.ScopeAssetGovern}))
	create.Header.Set("Idempotency-Key", "asset-governance-http-create-key")
	create.Header.Set("X-Trace-ID", "trace-governance-http-create")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, create)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data config.AssetGovernanceWorkOrder `json:"data"`
		Meta struct {
			TraceID string `json:"trace_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Status != "pending_approval" || envelope.Meta.TraceID != "trace-governance-http-create" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	list := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+assetID+"/governance/work-orders", nil)
	list.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenant, []string{authmodel.ScopeAssetRead}))
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, list)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	denied := httptest.NewRequest(http.MethodPost, "/api/v1/assets/"+assetID+"/governance/work-orders", bytes.NewReader(payload))
	denied.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenant, []string{authmodel.ScopeAssetRead}))
	denied.Header.Set("Idempotency-Key", "asset-governance-http-denied-key")
	deniedRR := httptest.NewRecorder()
	handler.ServeHTTP(deniedRR, denied)
	if deniedRR.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", deniedRR.Code, deniedRR.Body.String())
	}
}
