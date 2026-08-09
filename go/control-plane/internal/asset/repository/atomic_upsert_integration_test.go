package repository_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	assetConsumer "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/consumer"
	assetRepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

type recordingAssetPublisher struct{}

func (p *recordingAssetPublisher) Send(
	_ context.Context,
	_ string,
	_ []byte,
	_ ...kafkaCommon.MessageHeader,
) error {
	return nil
}

type integrationProjectionTarget struct {
	name      string
	calls     int
	failCalls int
}

func (t *integrationProjectionTarget) Name() string {
	return t.name
}

func (t *integrationProjectionTarget) Projection(event assetConsumer.AssetUpsertedV2) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event_id": event.EventID,
		"asset_id": event.AssetID,
		"revision": event.AggregateVersion,
		"target":   t.name,
	})
}

func (t *integrationProjectionTarget) Apply(
	_ context.Context,
	_ assetConsumer.AssetUpsertedV2,
	_ []byte,
) error {
	t.calls++
	if t.calls <= t.failCalls {
		return errors.New("injected target failure")
	}
	return nil
}

// This test is deliberately guarded by both an explicit DSN and a sentinel
// table. It must never run against a shared or production PostgreSQL instance.
func TestAssetAtomicUpsertPostgresIntegration(t *testing.T) {
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
	if err := cleanupAssetAtomicIntegrationTenant(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupAssetAtomicIntegrationTenant(db); err != nil {
			t.Errorf("cleanup integration tenant: %v", err)
		}
	}()
	if _, err := db.Exec(`
		INSERT INTO tenants(tenant_id,name)
		VALUES ('asset-atomic-integration','Asset Atomic Integration')
		ON CONFLICT (tenant_id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	repo, err := assetRepository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	rec := &config.AssetRecord{
		TenantID:   "asset-atomic-integration",
		MACAddress: "02:11:22:33:44:55",
		IPAddress:  "10.22.33.44",
		AssetType:  "server",
		Status:     "active",
		Source:     "manual",
	}
	create := config.AssetUpsertCommand{
		ExpectedRevision: 0,
		IdempotencyKey:   "asset-atomic-integration-create",
		Actor:            "integration-creator",
		TraceID:          "trace-asset-create",
		RequestID:        "request-asset-create",
	}
	first, err := repo.UpsertAtomic(context.Background(), rec, create)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	expectedAssetID := uuid.NewSHA1(
		uuid.NameSpaceURL,
		[]byte("traffic.asset:"+rec.TenantID+":"+rec.MACAddress),
	).String()
	if first.AssetID != expectedAssetID {
		t.Fatalf("asset_id=%q want deterministic %q", first.AssetID, expectedAssetID)
	}
	replay, err := repo.UpsertAtomic(context.Background(), rec, create)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replay.IdempotentReplay || replay.AssetID != first.AssetID || replay.EventID != first.EventID || replay.OutboxID != first.OutboxID {
		t.Fatalf("replay diverged: first=%+v replay=%+v", first, replay)
	}
	conflicting := *rec
	conflicting.Hostname = "different-request"
	if _, err := repo.UpsertAtomic(context.Background(), &conflicting, create); !errors.Is(err, assetRepository.ErrAssetIdempotencyConflict) {
		t.Fatalf("idempotency conflict err=%v", err)
	}

	update := *rec
	update.Hostname = "asset-atomic-updated"
	updateCommand := config.AssetUpsertCommand{
		ExpectedRevision: 1,
		IdempotencyKey:   "asset-atomic-integration-update",
		Actor:            "integration-updater",
		TraceID:          "trace-asset-update",
		RequestID:        "request-asset-update",
	}
	updated, err := repo.UpsertAtomic(context.Background(), &update, updateCommand)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Revision != 2 || updated.Created {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	staleCommand := updateCommand
	staleCommand.IdempotencyKey = "asset-atomic-integration-stale"
	if _, err := repo.UpsertAtomic(context.Background(), &update, staleCommand); !errors.Is(err, assetRepository.ErrAssetRevisionConflict) {
		t.Fatalf("stale revision err=%v", err)
	}
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	if _, err := db.Exec(`UPDATE assets SET last_seen=$2 WHERE tenant_id=$1`, rec.TenantID, cutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	inactiveCommand := config.AssetInactiveCommand{
		ActionID:       config.AssetInactiveSweepAction,
		IdempotencyKey: "asset-atomic-integration-inactive",
		Actor:          "asset-lifecycle-scheduler",
		Reason:         "integration inactive sweep",
		TraceID:        "trace-asset-inactive",
		RequestID:      "request-asset-inactive",
		Cutoff:         cutoff,
	}
	inactive, err := repo.MarkInactiveSinceAtomic(context.Background(), rec.TenantID, inactiveCommand)
	if err != nil {
		t.Fatalf("inactive sweep: %v", err)
	}
	if inactive.Count != 1 || inactive.IdempotentReplay {
		t.Fatalf("unexpected inactive result: %+v", inactive)
	}
	inactiveReplay, err := repo.MarkInactiveSinceAtomic(context.Background(), rec.TenantID, inactiveCommand)
	if err != nil || !inactiveReplay.IdempotentReplay || inactiveReplay.Count != 1 || inactiveReplay.EventIDs[0] != inactive.EventIDs[0] {
		t.Fatalf("inactive replay=%+v err=%v", inactiveReplay, err)
	}

	var assets, history, upsertAudits, inactiveAudits, outbox, requests, inactiveRequests int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM assets WHERE tenant_id='asset-atomic-integration'),
		  (SELECT count(*) FROM asset_events WHERE tenant_id='asset-atomic-integration'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id='asset-atomic-integration' AND action='ASSET_UPSERT'),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id='asset-atomic-integration' AND action='ASSET_INACTIVE_SWEEP'),
		  (SELECT count(*) FROM asset_event_outbox WHERE tenant_id='asset-atomic-integration'),
		  (SELECT count(*) FROM asset_upsert_requests WHERE tenant_id='asset-atomic-integration'),
		  (SELECT count(*) FROM asset_inactive_requests WHERE tenant_id='asset-atomic-integration')`,
	).Scan(&assets, &history, &upsertAudits, &inactiveAudits, &outbox, &requests, &inactiveRequests); err != nil {
		t.Fatal(err)
	}
	if assets != 1 || history != 3 || upsertAudits != 2 || inactiveAudits != 1 || outbox != 3 || requests != 2 || inactiveRequests != 1 {
		t.Fatalf("transaction counts assets=%d history=%d upsert_audits=%d inactive_audits=%d outbox=%d requests=%d inactive_requests=%d", assets, history, upsertAudits, inactiveAudits, outbox, requests, inactiveRequests)
	}

	publisher := &recordingAssetPublisher{}
	dispatcher, err := assetRepository.NewAssetOutboxDispatcher(db, publisher, assetRepository.OutboxDispatcherConfig{
		WorkerID: "asset-integration-dispatcher",
		Lease:    30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	for expected := 0; expected < 3; expected++ {
		found, err := dispatcher.DispatchNext(context.Background())
		if err != nil || !found {
			t.Fatalf("dispatch %d found=%v err=%v", expected, found, err)
		}
	}
	if found, err := dispatcher.DispatchNext(context.Background()); err != nil || found {
		t.Fatalf("empty dispatch found=%v err=%v", found, err)
	}
	var published int
	if err := db.QueryRow(`
		SELECT count(*) FROM asset_event_outbox
		WHERE tenant_id='asset-atomic-integration' AND status='published'`,
	).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != 3 {
		t.Fatalf("published outbox=%d want=3", published)
	}

	eventConsumer, err := assetConsumer.NewAssetProjectionEventConsumer(db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`
		SELECT payload::text
		FROM asset_event_outbox
		WHERE tenant_id='asset-atomic-integration'
		ORDER BY aggregate_version`)
	if err != nil {
		t.Fatal(err)
	}
	var projectionPayloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		projectionPayloads = append(projectionPayloads, payload)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for offset, payload := range projectionPayloads {
		var event assetConsumer.AssetUpsertedV2
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatal(err)
		}
		if err := eventConsumer.Accept(
			context.Background(),
			event,
			0,
			int64(offset),
			payload,
		); err != nil {
			t.Fatalf("accept projection event %d: %v", offset, err)
		}
	}

	osTarget := &integrationProjectionTarget{name: "opensearch", failCalls: 1}
	nebulaTarget := &integrationProjectionTarget{name: "nebulagraph"}
	worker, err := assetConsumer.NewAssetProjectionWorker(
		db,
		[]assetConsumer.AssetProjectionTarget{osTarget, nebulaTarget},
		assetConsumer.AssetProjectionWorkerConfig{
			WorkerID:    "asset-integration-projector",
			Lease:       30 * time.Second,
			MaxAttempts: 3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.VerifySchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if found, err := worker.ProjectNext(context.Background()); !found || err == nil {
		t.Fatalf("expected injected partial failure found=%v err=%v", found, err)
	}
	var osStatus, nebulaStatus, projectionStatus string
	if err := db.QueryRow(`
		SELECT os_status,nebula_status,status
		FROM asset_projection_inbox
		WHERE aggregate_version=1 AND tenant_id='asset-atomic-integration'`,
	).Scan(&osStatus, &nebulaStatus, &projectionStatus); err != nil {
		t.Fatal(err)
	}
	if osStatus != "pending" || nebulaStatus != "applied" || projectionStatus != "pending" {
		t.Fatalf("partial projection state os=%s nebula=%s status=%s", osStatus, nebulaStatus, projectionStatus)
	}
	if _, err := db.Exec(`
		UPDATE asset_projection_inbox
		SET available_at=now()
		WHERE tenant_id='asset-atomic-integration' AND status='pending'`); err != nil {
		t.Fatal(err)
	}
	for expected := 0; expected < 3; expected++ {
		found, err := worker.ProjectNext(context.Background())
		if err != nil || !found {
			t.Fatalf("projection retry %d found=%v err=%v", expected, found, err)
		}
	}
	if found, err := worker.ProjectNext(context.Background()); err != nil || found {
		t.Fatalf("empty projection found=%v err=%v", found, err)
	}
	if osTarget.calls != 4 {
		t.Fatalf("OpenSearch calls=%d want=4 (one retry plus revisions 2 and 3)", osTarget.calls)
	}
	if nebulaTarget.calls != 3 {
		t.Fatalf("NebulaGraph calls=%d want=3 (successful target not repeated for revision 1)", nebulaTarget.calls)
	}
	var applied, watermarks int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM asset_projection_inbox
		   WHERE tenant_id='asset-atomic-integration' AND status='applied'),
		  (SELECT count(*) FROM asset_projection_watermarks
		   WHERE tenant_id='asset-atomic-integration' AND aggregate_version=3)`,
	).Scan(&applied, &watermarks); err != nil {
		t.Fatal(err)
	}
	if applied != 3 || watermarks != 2 {
		t.Fatalf("projection reconciliation applied=%d watermarks_at_v3=%d", applied, watermarks)
	}
}

func cleanupAssetAtomicIntegrationTenant(db *sql.DB) error {
	const tenantID = "asset-atomic-integration"
	for _, statement := range []string{
		`DELETE FROM asset_projection_watermarks WHERE tenant_id=$1`,
		`DELETE FROM asset_projection_inbox WHERE tenant_id=$1`,
		`DELETE FROM asset_inactive_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_upsert_requests WHERE tenant_id=$1`,
		`DELETE FROM asset_event_outbox WHERE tenant_id=$1`,
		`DELETE FROM asset_events WHERE tenant_id=$1`,
		`DELETE FROM audit_logs WHERE tenant_id=$1 AND action IN ('ASSET_UPSERT','ASSET_INACTIVE_SWEEP')`,
		`DELETE FROM assets WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE tenant_id=$1`,
	} {
		if _, err := db.Exec(statement, tenantID); err != nil {
			return err
		}
	}
	return nil
}
