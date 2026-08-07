package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func TestDiscoveryJobHTTPAcceptedReadAndTenantIsolation(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ASSET_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	const tenantID = "asset-discovery-http-integration"
	if err := cleanupDiscoveryHTTPIntegration(db, tenantID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupDiscoveryHTTPIntegration(db, tenantID); err != nil {
			t.Errorf("cleanup discovery HTTP integration: %v", err)
		}
	}()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES ($1,'Discovery HTTP Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	const signingKey = "discovery-http-integration-signing-key"
	svc := service.New(&config.Config{
		Auth: config.AuthConfig{JWTSigningKey: signingKey},
		Discovery: config.DiscoveryConfig{
			JobsV2Enabled: true,
		},
	}, repo, zap.NewNop())
	handler := NewHTTPHandler(svc, zap.NewNop())
	payload := []byte(`{
		"action_id":"asset-active-discovery-run",
		"mode":"snmp",
		"target_cidr":"192.0.2.0/30",
		"reason":"approved HTTP integration",
		"rate_limit_per_second":10,
		"security_window_start":"2099-01-01T00:00:00Z",
		"security_window_end":"2099-01-01T01:00:00Z",
		"approved_by":"security-approver"
	}`)
	submit := httptest.NewRequest(http.MethodPost, "/api/v1/assets/discovery/runs", bytes.NewReader(payload))
	submit.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenantID, []string{authmodel.ScopeAssetDiscover}))
	submit.Header.Set("Idempotency-Key", "discovery-http-integration-create")
	submit.Header.Set("X-Trace-ID", "trace-discovery-http-integration")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, submit)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var accepted struct {
		Data config.DiscoveryRun `json:"data"`
		Meta struct {
			ContractVersion string `json:"contract_version"`
			TraceID         string `json:"trace_id"`
			State           string `json:"state"`
		} `json:"meta"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Data.RunID == "" || accepted.Data.Status != "queued" ||
		accepted.Meta.ContractVersion != "1" ||
		accepted.Meta.TraceID != "trace-discovery-http-integration" ||
		accepted.Meta.State != "queued" || accepted.Error != nil {
		t.Fatalf("accepted response=%+v", accepted)
	}

	read := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/assets/discovery/runs/"+accepted.Data.RunID,
		nil,
	)
	read.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenantID, []string{authmodel.ScopeAssetRead}))
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, read)
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}
	crossTenant := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/assets/discovery/runs/"+accepted.Data.RunID,
		nil,
	)
	crossTenant.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "another-tenant", []string{authmodel.ScopeAssetRead}))
	crossRecorder := httptest.NewRecorder()
	handler.ServeHTTP(crossRecorder, crossTenant)
	if crossRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status=%d body=%s", crossRecorder.Code, crossRecorder.Body.String())
	}

	cancel := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assets/discovery/runs/"+accepted.Data.RunID+"/cancel",
		bytes.NewBufferString(`{"expected_revision":1,"reason":"security window revoked"}`),
	)
	cancel.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenantID, []string{authmodel.ScopeAssetDiscover}))
	cancel.Header.Set("Idempotency-Key", "discovery-http-integration-cancel")
	cancelRecorder := httptest.NewRecorder()
	handler.ServeHTTP(cancelRecorder, cancel)
	if cancelRecorder.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	const candidateID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if _, err := db.Exec(`
		UPDATE asset_discovery_runs
		   SET status='succeeded', revision=3, completed_at=now()
		 WHERE tenant_id=$1 AND run_id=$2`,
		tenantID, accepted.Data.RunID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO asset_discovery_candidates(
			candidate_id,run_id,tenant_id,fingerprint,observation,status,revision
		) VALUES (
			$3,$2,$1,repeat('a',64),
			'{"ip_address":"192.0.2.1","mac_address":"02:aa:bb:cc:dd:ee","hostname":"http-candidate"}'::jsonb,
			'pending',1
		)`,
		tenantID, accepted.Data.RunID, candidateID,
	); err != nil {
		t.Fatal(err)
	}
	merge := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assets/discovery/runs/"+accepted.Data.RunID+"/candidates/"+candidateID+"/merge",
		bytes.NewBufferString(`{
			"expected_candidate_revision":1,
			"expected_asset_revision":0,
			"merge_mode":"manual",
			"reason":"reviewed HTTP candidate"
		}`),
	)
	merge.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tenantID, []string{authmodel.ScopeAssetDiscover}))
	merge.Header.Set("Idempotency-Key", "discovery-http-candidate-merge")
	merge.Header.Set("X-Trace-ID", "trace-discovery-http-candidate-merge")
	mergeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(mergeRecorder, merge)
	if mergeRecorder.Code != http.StatusOK {
		t.Fatalf("merge status=%d body=%s", mergeRecorder.Code, mergeRecorder.Body.String())
	}
	var merged struct {
		Data config.DiscoveryCandidateMergeResult `json:"data"`
		Meta struct {
			State string `json:"state"`
		} `json:"meta"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(mergeRecorder.Body.Bytes(), &merged); err != nil {
		t.Fatal(err)
	}
	if !merged.Data.AssetCreated || merged.Data.AssetRevision != 1 ||
		merged.Data.Candidate == nil || merged.Data.Candidate.Status != "merged" ||
		merged.Meta.State != "merged" || merged.Error != nil {
		t.Fatalf("merge response=%+v", merged)
	}
	crossMerge := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assets/discovery/runs/"+accepted.Data.RunID+"/candidates/"+candidateID+"/merge",
		bytes.NewBufferString(`{
			"expected_candidate_revision":1,
			"expected_asset_revision":0,
			"merge_mode":"manual",
			"reason":"reviewed HTTP candidate"
		}`),
	)
	crossMerge.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "another-tenant", []string{authmodel.ScopeAssetDiscover}))
	crossMerge.Header.Set("Idempotency-Key", "discovery-http-cross-tenant-merge")
	crossMergeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(crossMergeRecorder, crossMerge)
	if crossMergeRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant merge status=%d body=%s", crossMergeRecorder.Code, crossMergeRecorder.Body.String())
	}
}

func cleanupDiscoveryHTTPIntegration(db *sql.DB, tenantID string) error {
	for _, table := range []string{
		"asset_discovery_candidates",
		"asset_discovery_run_history",
		"asset_discovery_control_requests",
		"asset_discovery_outbox",
		"asset_discovery_runs",
		"asset_discovery_credentials",
		"asset_event_outbox",
		"asset_events",
		"audit_logs",
		"assets",
		"tenants",
	} {
		if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table), tenantID); err != nil {
			return err
		}
	}
	return nil
}
