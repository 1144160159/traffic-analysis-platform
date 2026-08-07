package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
)

const discoveryAtomicTenant = "asset-discovery-atomic-integration"

// Guarded by the same explicit DSN and sentinel as the asset atomic test. The
// helper creates an owned PostgreSQL container without a persistent volume.
func TestDiscoveryResourceAtomicPostgresIntegration(t *testing.T) {
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
	if err := cleanupDiscoveryAtomicTenant(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupDiscoveryAtomicTenant(db); err != nil {
			t.Errorf("cleanup discovery atomic tenant: %v", err)
		}
	}()
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Discovery Atomic Integration')`, discoveryAtomicTenant); err != nil {
		t.Fatal(err)
	}
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	credentialID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("traffic.asset.discovery.credential:"+discoveryAtomicTenant+":integration-snmp")).String()
	credential := &config.DiscoveryCredential{
		CredentialID: credentialID, TenantID: discoveryAtomicTenant,
		Name: "integration-snmp", Protocol: config.DiscoveryModeSNMP,
		Endpoint: "udp://10.0.0.10:161", SecretRef: "secret/ref/must-not-leak",
		CreatedBy: "integration-actor",
	}
	credentialCreate := config.DiscoveryResourceCommand{
		ActionID: "asset-discovery-credential-upsert", ExpectedRevision: 0,
		IdempotencyKey: "discovery-credential-create-integration", Actor: "integration-actor",
		Reason: "create integration discovery credential", TraceID: "trace-credential-create",
		RequestID: "request-credential-create",
	}
	createdCredential, err := repo.UpsertDiscoveryCredentialAtomic(context.Background(), credential, credentialCreate, strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if createdCredential.Revision != 1 {
		t.Fatalf("credential revision=%d want=1", createdCredential.Revision)
	}
	replayedCredential, err := repo.UpsertDiscoveryCredentialAtomic(context.Background(), credential, credentialCreate, strings.Repeat("a", 64))
	if err != nil || !replayedCredential.IdempotentReplay || replayedCredential.Revision != 1 {
		t.Fatalf("credential replay=%+v err=%v", replayedCredential, err)
	}
	if _, err := repo.UpsertDiscoveryCredentialAtomic(context.Background(), credential, credentialCreate, strings.Repeat("b", 64)); !errors.Is(err, assetRepository.ErrDiscoveryResourceIdempotencyConflict) {
		t.Fatalf("credential idempotency conflict err=%v", err)
	}

	updatedCredential := *credential
	updatedCredential.Endpoint = "udp://10.0.0.11:161"
	credentialUpdate := credentialCreate
	credentialUpdate.ExpectedRevision = 1
	credentialUpdate.IdempotencyKey = "discovery-credential-update-integration"
	credentialUpdate.Reason = "rotate integration discovery endpoint"
	credentialUpdate.TraceID = "trace-credential-update"
	updated, err := repo.UpsertDiscoveryCredentialAtomic(context.Background(), &updatedCredential, credentialUpdate, strings.Repeat("c", 64))
	if err != nil || updated.Revision != 2 {
		t.Fatalf("credential update=%+v err=%v", updated, err)
	}
	stale := credentialUpdate
	stale.IdempotencyKey = "discovery-credential-stale-integration"
	if _, err := repo.UpsertDiscoveryCredentialAtomic(context.Background(), &updatedCredential, stale, strings.Repeat("d", 64)); !errors.Is(err, assetRepository.ErrDiscoveryResourceRevisionConflict) {
		t.Fatalf("credential stale revision err=%v", err)
	}

	now := time.Now().UTC()
	run := &config.DiscoveryRun{
		RunID: uuid.NewString(), TenantID: discoveryAtomicTenant,
		Mode: config.DiscoveryModeSNMP, ActionID: "asset-active-discovery-run",
		Status: config.DiscoveryStatusQueued, Revision: 1,
		RequestedBy: "integration-actor", Reason: "integration synchronous discovery",
		RateLimit: 10, TraceID: "trace-discovery-run", QueuedAt: now, StartedAt: now, UpdatedAt: now,
	}
	runCommand := config.DiscoveryJobCommand{
		IdempotencyKey: "discovery-run-create-integration", Actor: "integration-actor",
		TraceID: run.TraceID, RequestID: "request-discovery-run",
	}
	createdRun, err := repo.CreateDiscoveryJobAtomic(context.Background(), run, runCommand, strings.Repeat("e", 64))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	runningRun, err := repo.StartLegacyDiscoveryRunAtomic(context.Background(), createdRun, runCommand)
	if err != nil || runningRun.Revision != 2 || runningRun.Status != config.DiscoveryStatusRunning {
		t.Fatalf("start run=%+v err=%v", runningRun, err)
	}
	completedRun, err := repo.CompleteLegacyDiscoveryRunAtomic(
		context.Background(), runningRun, config.DiscoveryStatusSucceeded, "", 1, 1, 0, runCommand,
	)
	if err != nil || completedRun.Revision != 3 || completedRun.Status != config.DiscoveryStatusSucceeded {
		t.Fatalf("complete run=%+v err=%v", completedRun, err)
	}

	link := &config.TopologyLink{
		LinkID:   uuid.NewSHA1(uuid.NameSpaceURL, []byte("discovery-topology-integration")).String(),
		TenantID: discoveryAtomicTenant, RunID: run.RunID,
		SourceMAC: "02:00:00:00:00:01", SourceIP: "10.0.0.1", SourceInterface: "eth0",
		NeighborMAC: "02:00:00:00:00:02", NeighborIP: "10.0.0.2", NeighborInterface: "eth1",
		Protocol: config.DiscoveryModeLLDP, Confidence: 90, ObservedAt: now,
	}
	linkCommand := config.DiscoveryResourceCommand{
		ActionID: "asset-discovery-topology-link-upsert", ExpectedRevision: 0,
		IdempotencyKey: "discovery-topology-create-integration", Actor: "integration-actor",
		Reason: "record integration topology edge", TraceID: run.TraceID, RequestID: run.RunID,
	}
	createdLink, err := repo.UpsertTopologyLinkAtomic(context.Background(), link, linkCommand, strings.Repeat("f", 64))
	if err != nil || createdLink.Revision != 1 {
		t.Fatalf("topology create=%+v err=%v", createdLink, err)
	}
	replayedLink, err := repo.UpsertTopologyLinkAtomic(context.Background(), link, linkCommand, strings.Repeat("f", 64))
	if err != nil || !replayedLink.IdempotentReplay || replayedLink.LinkID != createdLink.LinkID {
		t.Fatalf("topology replay=%+v err=%v", replayedLink, err)
	}

	var resourceRequests, resourceHistory, runHistory, audits, outbox, leakedSecrets int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM asset_discovery_resource_requests WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_discovery_resource_history WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_discovery_run_history WHERE tenant_id=$1),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action IN ('ASSET_DISCOVERY_JOB_ACCEPTED','ASSET_ACTIVE_DISCOVERY_COMPLETED','ASSET_DISCOVERY_CREDENTIAL_UPSERT','ASSET_DISCOVERY_TOPOLOGY_LINK_UPSERT')),
		  (SELECT count(*) FROM asset_discovery_outbox WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_discovery_outbox WHERE tenant_id=$1 AND payload::text LIKE '%secret/ref/%')`,
		discoveryAtomicTenant,
	).Scan(&resourceRequests, &resourceHistory, &runHistory, &audits, &outbox, &leakedSecrets); err != nil {
		t.Fatal(err)
	}
	if resourceRequests != 3 || resourceHistory != 3 || runHistory != 3 || audits != 5 || outbox != 6 || leakedSecrets != 0 {
		t.Fatalf("counts requests=%d resource_history=%d run_history=%d audits=%d outbox=%d leaked_secrets=%d", resourceRequests, resourceHistory, runHistory, audits, outbox, leakedSecrets)
	}

	dispatcher, err := assetRepository.NewDiscoveryOutboxDispatcher(db, &recordingAssetPublisher{}, assetRepository.OutboxDispatcherConfig{
		WorkerID: "discovery-integration-dispatcher", Lease: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	for expected := 0; expected < outbox; expected++ {
		found, err := dispatcher.DispatchNext(context.Background())
		if err != nil || !found {
			t.Fatalf("dispatch %d found=%v err=%v", expected, found, err)
		}
	}
	var published int
	if err := db.QueryRow(`SELECT count(*) FROM asset_discovery_outbox WHERE tenant_id=$1 AND status='published'`, discoveryAtomicTenant).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != outbox {
		t.Fatalf("published=%d want=%d", published, outbox)
	}
}

func cleanupDiscoveryAtomicTenant(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM asset_discovery_resource_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_resource_history WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_outbox WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_candidates WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_control_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_run_history WHERE tenant_id=$1`,
		`DELETE FROM asset_topology_links WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_runs WHERE tenant_id=$1`,
		`DELETE FROM asset_discovery_credentials WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := tx.Exec(query, discoveryAtomicTenant); err != nil {
			return err
		}
	}
	return tx.Commit()
}
