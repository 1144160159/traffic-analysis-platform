package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
)

func TestAssetGovernancePostgresIntegration(t *testing.T) {
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
	const tenant = "asset-governance-integration"
	assetID := uuid.NewString()
	cleanup := func() {
		for _, query := range []string{
			`DELETE FROM asset_governance_control_requests WHERE tenant_id=$1`,
			`DELETE FROM asset_governance_work_order_history WHERE tenant_id=$1`,
			`DELETE FROM asset_governance_outbox WHERE tenant_id=$1`,
			`DELETE FROM asset_governance_work_orders WHERE tenant_id=$1`,
			`DELETE FROM asset_events WHERE tenant_id=$1`,
			`DELETE FROM audit_logs WHERE tenant_id=$1`,
			`DELETE FROM assets WHERE tenant_id=$1`,
			`DELETE FROM tenants WHERE tenant_id=$1`,
		} {
			if _, err := db.Exec(query, tenant); err != nil {
				t.Errorf("cleanup %q: %v", query, err)
			}
		}
	}
	cleanup()
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Asset Governance Integration')`, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets(asset_id,tenant_id,mac_address,asset_type,status,source,revision,lifecycle_state)
		VALUES($1,$2,'02:99:88:77:66:55','server','active','manual',1,'managed')`, assetID, tenant); err != nil {
		t.Fatal(err)
	}
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	create := config.AssetGovernanceCreateCommand{ActionID: config.AssetGovernanceCreateAction, TargetLifecycleState: "isolated",
		Owner: "integration-owner", DueAt: time.Now().UTC().Add(24 * time.Hour), EvidenceRequired: true,
		Reason: "isolate asset after verified compromise", ExpectedAssetRevision: 1,
		IdempotencyKey: "asset-governance-integration-create", TenantID: tenant, Actor: "integration-requester", TraceID: "trace-governance-create"}
	order, err := repo.CreateAssetGovernanceWorkOrder(context.Background(), assetID, create)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if order.Status != "pending_approval" || order.SourceLifecycleState != "managed" || order.Revision != 1 {
		t.Fatalf("unexpected create: %+v", order)
	}
	replay, err := repo.CreateAssetGovernanceWorkOrder(context.Background(), assetID, create)
	if err != nil || !replay.IdempotentReplay || replay.WorkOrderID != order.WorkOrderID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	conflict := create
	conflict.Reason = "different immutable governance request"
	if _, err := repo.CreateAssetGovernanceWorkOrder(context.Background(), assetID, conflict); !errors.Is(err, assetRepository.ErrAssetGovernanceIdempotencyConflict) {
		t.Fatalf("create conflict err=%v", err)
	}
	activeConflict := create
	activeConflict.TargetLifecycleState = "retired"
	activeConflict.IdempotencyKey = "asset-governance-active-conflict"
	if _, err := repo.CreateAssetGovernanceWorkOrder(context.Background(), assetID, activeConflict); !errors.Is(err, assetRepository.ErrAssetGovernanceStateConflict) {
		t.Fatalf("active work-order conflict err=%v", err)
	}
	action := func(actionID, actor, key string, revision int64, evidence []string) (*config.AssetGovernanceWorkOrder, error) {
		return repo.ApplyAssetGovernanceAction(context.Background(), order.WorkOrderID, config.AssetGovernanceActionCommand{
			ActionID: actionID, ExpectedRevision: revision, Reason: "integration governance state transition", EvidenceRefs: evidence,
			IdempotencyKey: key, TenantID: tenant, Actor: actor, TraceID: "trace-" + actionID})
	}
	if _, err := action("asset-governance-approve", "integration-requester", "asset-governance-self-approve", 1, nil); !errors.Is(err, assetRepository.ErrAssetGovernanceSelfApproval) {
		t.Fatalf("self approval err=%v", err)
	}
	order, err = action("asset-governance-approve", "integration-approver", "asset-governance-approve-key", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	order, err = action("asset-governance-start", "integration-owner", "asset-governance-start-key", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := action("asset-governance-complete", "integration-owner", "asset-governance-no-evidence", 3, nil); !errors.Is(err, assetRepository.ErrAssetGovernanceEvidenceRequired) {
		t.Fatalf("missing evidence err=%v", err)
	}
	order, err = action("asset-governance-complete", "integration-owner", "asset-governance-complete-key", 3, []string{"minio://evidence/asset-isolation.json#sha256=012345"})
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != "completed" || order.CurrentLifecycleState != "isolated" || order.ResultingAssetRevision != 2 {
		t.Fatalf("unexpected completion: %+v", order)
	}
	order, err = action("asset-governance-compensate", "integration-approver", "asset-governance-compensate-key", 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != "compensated" || order.CurrentLifecycleState != "managed" || order.ResultingAssetRevision != 3 {
		t.Fatalf("unexpected compensation: %+v", order)
	}
	if _, err := repo.GetAssetGovernanceWorkOrder(context.Background(), "another-tenant", order.WorkOrderID); !errors.Is(err, assetRepository.ErrAssetGovernanceNotFound) {
		t.Fatalf("cross tenant read err=%v", err)
	}
	var lifecycle string
	var revision int64
	var histories, outbox, audits, assetEvents, requests int
	if err := db.QueryRow(`SELECT lifecycle_state,revision,
		(SELECT count(*) FROM asset_governance_work_order_history WHERE tenant_id=$2),
		(SELECT count(*) FROM asset_governance_outbox WHERE tenant_id=$2),
		(SELECT count(*) FROM audit_logs WHERE tenant_id=$2 AND object_type='asset_governance_work_order'),
		(SELECT count(*) FROM asset_events WHERE tenant_id=$2 AND event_type LIKE 'governance_lifecycle_%'),
		(SELECT count(*) FROM asset_governance_control_requests WHERE tenant_id=$2)
		FROM assets WHERE asset_id=$1 AND tenant_id=$2`, assetID, tenant).Scan(&lifecycle, &revision, &histories, &outbox, &audits, &assetEvents, &requests); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "managed" || revision != 3 || histories != 5 || outbox != 5 || audits != 5 || assetEvents != 2 || requests != 4 {
		t.Fatalf("facts lifecycle=%s revision=%d history=%d outbox=%d audit=%d events=%d requests=%d", lifecycle, revision, histories, outbox, audits, assetEvents, requests)
	}
}
