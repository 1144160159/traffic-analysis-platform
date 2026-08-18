package api

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

const probeReadinessPostgresSentinel = "codex_ephemeral_m02_probe_pipeline_sentinel"

// TestProbePipelineReadinessRealPostgresFence exercises the production SQL
// ownership CAS and the transaction-bound dispatcher claim against a fresh,
// explicitly sentinel-protected PostgreSQL database.
func TestProbePipelineReadinessRealPostgresFence(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROBE_PIPELINE_READINESS_EPHEMERAL_PG_DSN"))
	if dsn == "" {
		t.Skip("PROBE_PIPELINE_READINESS_EPHEMERAL_PG_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var marker string
	if err := db.QueryRowContext(ctx, `SELECT marker FROM `+probeReadinessPostgresSentinel+` LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel PostgreSQL: marker=%q err=%v", marker, err)
	}
	ensureProbeReadinessIntegrationOutboxSchema(t, ctx, db)
	if _, err := db.ExecContext(ctx, `DELETE FROM probe_pipeline_readiness_epochs WHERE pipeline_id=$1`, alertconfig.ProbeOperationPipelineID); err != nil {
		t.Fatal(err)
	}
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	oldOwner := probeReadinessIntegrationReceipt(
		alertconfig.ProbeCommandDeliveryConsumer, "ingest-command-group", "owner-old", 100, now,
	)
	if err := store.IssueRenewRevoke(ctx, oldOwner); err != nil {
		t.Fatal(err)
	}
	newOwner := probeReadinessIntegrationReceipt(
		alertconfig.ProbeCommandDeliveryConsumer, "ingest-command-group", "owner-new", 101, now.Add(time.Millisecond),
	)
	if err := store.IssueRenewRevoke(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	staleRenew := oldOwner
	staleRenew.ObservedAt = now.Add(2 * time.Millisecond)
	staleRenew.LeaseExpiresAt = staleRenew.ObservedAt.Add(20 * time.Second)
	if err := store.IssueRenewRevoke(ctx, staleRenew); !errors.Is(err, ErrProbeReadinessStaleOwner) {
		t.Fatalf("stale renewal err=%v want ErrProbeReadinessStaleOwner", err)
	}
	staleRevoke := staleRenew
	staleRevoke.State = alertconfig.ProbePipelineRevoked
	staleRevoke.LeaseExpiresAt = time.Time{}
	if err := store.IssueRenewRevoke(ctx, staleRevoke); !errors.Is(err, ErrProbeReadinessStaleOwner) {
		t.Fatalf("stale revoke err=%v want ErrProbeReadinessStaleOwner", err)
	}
	var currentOwner string
	var currentEpoch int64
	var currentReady bool
	if err := db.QueryRowContext(ctx, `SELECT owner_id,owner_epoch,ready
		FROM probe_pipeline_readiness_epochs
		WHERE pipeline_id=$1 AND consumer_role=$2`,
		alertconfig.ProbeOperationPipelineID, string(alertconfig.ProbeCommandDeliveryConsumer),
	).Scan(&currentOwner, &currentEpoch, &currentReady); err != nil {
		t.Fatal(err)
	}
	if currentOwner != newOwner.OwnerID || currentEpoch != newOwner.OwnerEpoch || !currentReady {
		t.Fatalf("successor ownership changed by stale owner: owner=%s epoch=%d ready=%v", currentOwner, currentEpoch, currentReady)
	}

	for index, role := range []alertconfig.ProbePipelineConsumerRole{
		alertconfig.ProbeAckAuthorityConsumer,
		alertconfig.ProbeLifecycleProjectionConsumer,
	} {
		receipt := probeReadinessIntegrationReceipt(
			role, "alert-"+strings.ToLower(string(role)), "alert-owner", int64(200+index), now.Add(time.Duration(index+2)*time.Millisecond),
		)
		if err := store.IssueRenewRevoke(ctx, receipt); err != nil {
			t.Fatal(err)
		}
	}

	operationID := uuid.New()
	eventID := uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO probe_operations
		(operation_id,tenant_id,probe_id,status,expires_at)
		VALUES ($1,'tenant-integration','probe-integration','accepted',now()+interval '1 minute')`, operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO probe_operation_outbox
		(event_id,operation_id,tenant_id,event_type,aggregate_version,schema_version,partition_key,payload)
		VALUES ($1,$2,'tenant-integration','traffic.probe.v2.OperationRequested',1,2,'tenant-integration:probe-integration','{}'::jsonb)`,
		eventID, operationID); err != nil {
		t.Fatal(err)
	}
	gate, err := NewProbeDispatcherGate(store)
	if err != nil {
		t.Fatal(err)
	}
	items, err := gate.AllowClaim(ctx, "readiness-integration-worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EventID != eventID.String() || items[0].OperationID != operationID.String() {
		t.Fatalf("claimed items=%#v", items)
	}
	var publishState, lockedBy string
	if err := db.QueryRowContext(ctx, `SELECT publish_state,locked_by FROM probe_operation_outbox WHERE event_id=$1`, eventID).
		Scan(&publishState, &lockedBy); err != nil {
		t.Fatal(err)
	}
	if publishState != "OUTCOME_UNKNOWN" || lockedBy != "readiness-integration-worker" {
		t.Fatalf("claim durability state=%s locked_by=%s", publishState, lockedBy)
	}

	if _, err := db.ExecContext(ctx, `UPDATE probe_pipeline_readiness_epochs
		SET lease_expires_at=now()-interval '1 second'
		WHERE pipeline_id=$1 AND consumer_role=$2`,
		alertconfig.ProbeOperationPipelineID, string(alertconfig.ProbeLifecycleProjectionConsumer)); err != nil {
		t.Fatal(err)
	}
	if _, err := gate.AllowClaim(ctx, "worker-after-expiry", 10); !errors.Is(err, ErrProbePipelineNotReady) {
		t.Fatalf("expired readiness gate err=%v want ErrProbePipelineNotReady", err)
	}
}

func probeReadinessIntegrationReceipt(
	role alertconfig.ProbePipelineConsumerRole,
	group, owner string,
	epoch int64,
	observedAt time.Time,
) alertconfig.ProbePipelineReadinessReceipt {
	return alertconfig.ProbePipelineReadinessReceipt{
		PipelineID: alertconfig.ProbeOperationPipelineID, ConsumerRole: role,
		ConsumerGroup: group, OwnerID: owner, OwnerEpoch: epoch,
		State: alertconfig.ProbePipelineReady, ObservedAt: observedAt,
		LeaseExpiresAt: observedAt.Add(20 * time.Second),
	}
}

func ensureProbeReadinessIntegrationOutboxSchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE TABLE IF NOT EXISTS probe_operations (
			operation_id UUID PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			probe_id TEXT NOT NULL,
			status TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE IF NOT EXISTS probe_operation_outbox (
			event_id UUID PRIMARY KEY,
			operation_id UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
			tenant_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
			schema_version INTEGER NOT NULL CHECK (schema_version > 0),
			partition_key TEXT NOT NULL,
			payload JSONB NOT NULL,
			published BOOLEAN NOT NULL DEFAULT false,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			locked_until TIMESTAMPTZ,
			locked_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			published_at TIMESTAMPTZ,
			publish_state TEXT NOT NULL DEFAULT 'PENDING',
			broker_topic TEXT NOT NULL DEFAULT '',
			broker_partition INTEGER,
			broker_offset BIGINT,
			publish_attempt UUID,
			acked_at TIMESTAMPTZ
		)`); err != nil {
		t.Fatal(err)
	}
}
