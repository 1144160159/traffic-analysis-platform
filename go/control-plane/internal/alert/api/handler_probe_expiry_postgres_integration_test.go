package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestExpireProbeOperationsPostgresAtomicity(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROBE_OPERATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROBE_OPERATION_TEST_POSTGRES_DSN is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`); err != nil {
		t.Fatal(err)
	}
	schema := "probe_expiry_" + strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	db, err := sql.Open("postgres", postgresDSNWithSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE probe_operations (
			operation_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, probe_id TEXT NOT NULL,
			status TEXT NOT NULL, command_revision BIGINT NOT NULL, state_revision BIGINT NOT NULL,
			trace_id TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, ack_error TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE probe_operation_history (
			history_id BIGSERIAL PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id),
			tenant_id TEXT NOT NULL, state_revision BIGINT NOT NULL, from_status TEXT NOT NULL,
			to_status TEXT NOT NULL, detail JSONB NOT NULL, UNIQUE(operation_id,state_revision)
		);
		CREATE TABLE probe_operation_outbox (
			event_id UUID PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id),
			tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, partition_key TEXT NOT NULL,
			aggregate_version BIGINT NOT NULL, schema_version INTEGER NOT NULL, payload JSONB NOT NULL,
			published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			published_at TIMESTAMPTZ, UNIQUE(operation_id,event_type)
		)`); err != nil {
		t.Fatal(err)
	}
	operationID := "11111111-1111-4111-8111-111111111111"
	if _, err := db.ExecContext(ctx, `INSERT INTO probe_operations
		(operation_id,tenant_id,probe_id,status,command_revision,state_revision,trace_id,expires_at)
		VALUES ($1::uuid,'tenant-a','probe-a','accepted',7,1,'trace-a',now()-interval '1 minute')`, operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE FUNCTION reject_expiry_event() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected expiry outbox failure'; END $$;
		CREATE TRIGGER reject_expiry_event BEFORE INSERT ON probe_operation_outbox
		FOR EACH ROW EXECUTE FUNCTION reject_expiry_event()`); err != nil {
		t.Fatal(err)
	}
	handler := NewSystemHandler(nil, db, zap.NewNop())
	if count, err := handler.expireProbeOperations(ctx, 10); err == nil || count != 0 {
		t.Fatalf("injected failure count=%d err=%v, want zero/error", count, err)
	}
	assertProbeExpiryFacts(t, ctx, db, operationID, "accepted", 0, 0)
	if _, err := db.ExecContext(ctx, `DROP TRIGGER reject_expiry_event ON probe_operation_outbox`); err != nil {
		t.Fatal(err)
	}
	count, err := handler.expireProbeOperations(ctx, 10)
	if err != nil || count != 1 {
		t.Fatalf("expiry count=%d err=%v", count, err)
	}
	assertProbeExpiryFacts(t, ctx, db, operationID, "expired", 1, 1)
	count, err = handler.expireProbeOperations(ctx, 10)
	if err != nil || count != 0 {
		t.Fatalf("replay count=%d err=%v, want zero/nil", count, err)
	}
	assertProbeExpiryFacts(t, ctx, db, operationID, "expired", 1, 1)
}

func TestProbeOperationExpiredProjectionMigration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROBE_OPERATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROBE_OPERATION_TEST_POSTGRES_DSN is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`); err != nil {
		t.Fatal(err)
	}
	schema := "probe_projection_" + strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	db, err := sql.Open("postgres", postgresDSNWithSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE probe_operation_event_projection (
			event_id UUID PRIMARY KEY,
			event_type TEXT NOT NULL CONSTRAINT probe_operation_event_projection_event_type_check
			CHECK (event_type='traffic.probe.v2.OperationAcknowledged')
		)`); err != nil {
		t.Fatal(err)
	}
	_, currentFile, _, _ := runtime.Caller(0)
	migrationPath := filepath.Join(filepath.Dir(currentFile), "../../../../../deployments/postgres/migrations/202608131110_probe_operation_expired_projection_v1.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	for index, eventType := range []string{
		probeOperationAcknowledgedEvent,
		probeOperationExpiredEvent,
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO probe_operation_event_projection(event_id,event_type)
			VALUES (uuid_generate_v4(),$1)`, eventType); err != nil {
			t.Fatalf("event type %d %s rejected: %v", index, eventType, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO probe_operation_event_projection(event_id,event_type)
		VALUES (uuid_generate_v4(),'traffic.probe.v2.OperationDeleted')`); err == nil {
		t.Fatal("unknown lifecycle type passed projection constraint")
	}
}

func TestProbeOperationOutboxBrokerReceiptMigration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROBE_OPERATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROBE_OPERATION_TEST_POSTGRES_DSN is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`); err != nil {
		t.Fatal(err)
	}
	schema := "probe_receipt_" + strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	db, err := sql.Open("postgres", postgresDSNWithSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE probe_operation_outbox (
		event_id UUID PRIMARY KEY,
		published BOOLEAN NOT NULL DEFAULT false,
		next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	CREATE INDEX idx_probe_operation_outbox_pending
	ON probe_operation_outbox(next_attempt_at,created_at) WHERE published=false;
	INSERT INTO probe_operation_outbox(event_id,published)
	VALUES ('11111111-1111-4111-8111-111111111111',true),
	       ('22222222-2222-4222-8222-222222222222',false)`); err != nil {
		t.Fatal(err)
	}
	_, currentFile, _, _ := runtime.Caller(0)
	migrationPath := filepath.Join(filepath.Dir(currentFile), "../../../../../deployments/postgres/migrations/202608131100_probe_operation_outbox_broker_receipt_v1.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ {
		if _, err := db.ExecContext(ctx, string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", run, err)
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT published,publish_state
		FROM probe_operation_outbox ORDER BY event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		published bool
		state     string
	}{{true, "KAFKA_ACKED"}, {false, "PENDING"}}
	index := 0
	for rows.Next() {
		var published bool
		var state string
		if err := rows.Scan(&published, &state); err != nil {
			t.Fatal(err)
		}
		if index >= len(want) || published != want[index].published || state != want[index].state {
			t.Fatalf("row %d published/state=%t/%s", index, published, state)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(want) {
		t.Fatalf("rows=%d want=%d", index, len(want))
	}
	if _, err := db.ExecContext(ctx, `UPDATE probe_operation_outbox
		SET published=true,publish_state='OUTCOME_UNKNOWN'
		WHERE event_id='22222222-2222-4222-8222-222222222222'`); err == nil {
		t.Fatal("published/outcome-unknown divergence passed compatibility constraint")
	}
	if _, err := db.ExecContext(ctx, `UPDATE probe_operation_outbox
		SET publish_state='INVALID'
		WHERE event_id='22222222-2222-4222-8222-222222222222'`); err == nil {
		t.Fatal("invalid publish_state passed constraint")
	}
}

func TestReadinessFenceClaimConcurrentRevokePostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROBE_OPERATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROBE_OPERATION_TEST_POSTGRES_DSN is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`); err != nil {
		t.Fatal(err)
	}
	schema := "probe_fence_" + strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	db, err := sql.Open("postgres", postgresDSNWithSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE probe_operations (
			operation_id UUID PRIMARY KEY, status TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE probe_operation_outbox (
			event_id UUID PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id),
			tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, partition_key TEXT NOT NULL,
			aggregate_version BIGINT NOT NULL, schema_version INTEGER NOT NULL, payload JSONB NOT NULL,
			published BOOLEAN NOT NULL DEFAULT false, publish_state TEXT NOT NULL DEFAULT 'PENDING',
			attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
			locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			publish_attempt UUID, broker_topic TEXT NOT NULL DEFAULT '', broker_partition INTEGER,
			broker_offset BIGINT, acked_at TIMESTAMPTZ
		);
		CREATE TABLE probe_pipeline_readiness_epochs (
			pipeline_id TEXT NOT NULL, consumer_role TEXT NOT NULL, consumer_group TEXT NOT NULL,
			owner_id TEXT NOT NULL, owner_epoch BIGINT NOT NULL, ready BOOLEAN NOT NULL,
			observed_at TIMESTAMPTZ NOT NULL, lease_expires_at TIMESTAMPTZ, revoked_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(pipeline_id,consumer_role)
		);
		INSERT INTO probe_operations(operation_id,status,expires_at)
		VALUES ('11111111-1111-4111-8111-111111111111','accepted',now()+interval '1 hour');
		INSERT INTO probe_operation_outbox
			(event_id,operation_id,tenant_id,event_type,partition_key,aggregate_version,schema_version,payload)
		VALUES ('22222222-2222-4222-8222-222222222222',
		        '11111111-1111-4111-8111-111111111111','tenant-a',
		        'traffic.probe.v2.OperationRequested','tenant-a:probe-a',1,2,
		        '{"event_id":"22222222-2222-4222-8222-222222222222"}'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index, role := range []string{"COMMAND_DELIVERY", "ACK_AUTHORITY", "LIFECYCLE_PROJECTION"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO probe_pipeline_readiness_epochs
			(pipeline_id,consumer_role,consumer_group,owner_id,owner_epoch,ready,observed_at,lease_expires_at)
			VALUES ('probe-operation-v2',$1,$2,$3,$4,true,$5,$6)`,
			role, "group-"+role, "owner-"+role, int64(index+1), now, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	const advisoryLock int64 = 781312003
	lockConnection, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lockConnection.Close()
	if _, err := lockConnection.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLock); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION block_probe_claim() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  PERFORM pg_advisory_lock(%d);
		  PERFORM pg_advisory_unlock(%d);
		  RETURN NEW;
		END $$;
		CREATE TRIGGER block_probe_claim BEFORE UPDATE ON probe_operation_outbox
		FOR EACH ROW EXECUTE FUNCTION block_probe_claim()`, advisoryLock, advisoryLock)); err != nil {
		t.Fatal(err)
	}
	store, err := NewProbePipelineReadinessStore(db)
	if err != nil {
		t.Fatal(err)
	}
	type claimResult struct {
		items []probeOperationOutboxItem
		err   error
	}
	claimDone := make(chan claimResult, 1)
	go func() {
		items, claimErr := store.FenceClaim(ctx, "worker-race", 10)
		claimDone <- claimResult{items: items, err: claimErr}
	}()
	waitForPostgresAdvisoryLock(t, ctx, db)

	revoked := alertconfig.ProbePipelineReadinessReceipt{
		PipelineID:    alertconfig.ProbeOperationPipelineID,
		ConsumerRole:  alertconfig.ProbeAckAuthorityConsumer,
		ConsumerGroup: "group-ACK_AUTHORITY", OwnerID: "owner-ACK_AUTHORITY", OwnerEpoch: 2,
		State: alertconfig.ProbePipelineRevoked, ObservedAt: now.Add(time.Second),
	}
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- store.IssueRenewRevoke(ctx, revoked) }()
	select {
	case err := <-revokeDone:
		t.Fatalf("revoke completed before the fenced claim released its readiness locks: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := lockConnection.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLock); err != nil {
		t.Fatal(err)
	}
	claim := <-claimDone
	if claim.err != nil || len(claim.items) != 1 {
		t.Fatalf("claim items=%d err=%v", len(claim.items), claim.err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FenceClaim(ctx, "worker-after-revoke", 10); !errors.Is(err, ErrProbePipelineNotReady) {
		t.Fatalf("claim after revoke err=%v, want closed fence", err)
	}
}

func waitForPostgresAdvisoryLock(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var found bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype='advisory' AND granted=false
		)`).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fenced claim did not reach the injected advisory lock")
}

func postgresDSNWithSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func assertProbeExpiryFacts(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	operationID string,
	wantStatus string,
	wantHistory int,
	wantOutbox int,
) {
	t.Helper()
	var status string
	var history int
	var outbox int
	if err := db.QueryRowContext(ctx, `SELECT status FROM probe_operations WHERE operation_id=$1::uuid`, operationID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM probe_operation_history WHERE operation_id=$1::uuid`, operationID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM probe_operation_outbox
		WHERE operation_id=$1::uuid AND event_type='traffic.probe.v2.OperationExpired'`, operationID).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || history != wantHistory || outbox != wantOutbox {
		t.Fatalf("facts status/history/outbox=%s/%d/%d want %s/%d/%d", status, history, outbox, wantStatus, wantHistory, wantOutbox)
	}
}
