package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestModelShadowAckPostgresExactQuorum(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MODEL_SHADOW_ACK_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("MODEL_SHADOW_ACK_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "model_shadow_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	defer admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`)

	scopedDSN, err := postgresDSNWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", scopedDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := createModelShadowFixture(ctx, db); err != nil {
		t.Fatal(err)
	}
	config := DefaultModelServiceConfig()
	config.AppliedAckExpectedParallelism = 4
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), config)

	event := validModelShadowEvent()
	event.EventID = "22222222-2222-4222-8222-222222222222"
	insertModelShadowEvent(t, db, event)
	for subtask := 0; subtask < 3; subtask++ {
		ack := validModelShadowAck()
		ack.SubtaskIndex = subtask
		consumeModelShadowAck(t, service, ack)
	}
	assertTableCount(t, db, "model_update_shadow_acks", 3)
	assertTableCount(t, db, "model_update_shadow_ready_receipts", 0)

	last := validModelShadowAck()
	last.SubtaskIndex = 3
	consumeModelShadowAck(t, service, last)
	assertTableCount(t, db, "model_update_shadow_ready_receipts", 1)
	var ready, expected int
	if err := db.QueryRow(`SELECT ready_subtasks,expected_parallelism FROM model_update_shadow_ready_receipts WHERE event_id=$1`, last.EventID).Scan(&ready, &expected); err != nil {
		t.Fatal(err)
	}
	if ready != 4 || expected != 4 {
		t.Fatalf("shadow quorum=%d/%d, want 4/4", ready, expected)
	}

	duplicate := validModelShadowAck()
	duplicate.Status = "duplicate"
	duplicate.SubtaskIndex = 0
	consumeModelShadowAck(t, service, duplicate)
	assertTableCount(t, db, "model_update_shadow_ready_receipts", 1)

	failedEvent := event
	failedEvent.EventID = "44444444-4444-4444-8444-444444444444"
	failedEvent.AggregateRevision = 8
	failedEvent.PackageID = "55555555-5555-5555-8555-555555555555"
	insertModelShadowEvent(t, db, failedEvent)
	for subtask := 0; subtask < 4; subtask++ {
		ack := validModelShadowAck()
		ack.EventID = failedEvent.EventID
		ack.PackageID = failedEvent.PackageID
		ack.AggregateRevision = failedEvent.AggregateRevision
		ack.SubtaskIndex = subtask
		if subtask == 2 {
			ack.Status = "failed"
			ack.Error = "graph schema incompatible"
		}
		consumeModelShadowAck(t, service, ack)
	}
	var failedReceiptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_update_shadow_ready_receipts WHERE event_id=$1`, failedEvent.EventID).Scan(&failedReceiptCount); err != nil {
		t.Fatal(err)
	}
	if failedReceiptCount != 0 {
		t.Fatal("partial/failed shadow acknowledgement created a ready receipt")
	}
}

func createModelShadowFixture(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE model_update_outbox (event_id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,model_id TEXT NOT NULL,model_version TEXT NOT NULL,payload JSONB NOT NULL)`,
		`CREATE TABLE model_update_shadow_acks (event_id TEXT NOT NULL,tenant_id TEXT NOT NULL,model_id TEXT NOT NULL,model_version TEXT NOT NULL,package_id TEXT NOT NULL,package_sha256 TEXT NOT NULL,aggregate_revision BIGINT NOT NULL,subtask_index INT NOT NULL,parallelism INT NOT NULL,status TEXT NOT NULL,error TEXT NOT NULL,payload JSONB NOT NULL,received_at TIMESTAMPTZ NOT NULL,last_seen_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(event_id,subtask_index))`,
		`CREATE TABLE model_update_shadow_ready_receipts (event_id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,model_id TEXT NOT NULL,model_version TEXT NOT NULL,package_id TEXT NOT NULL,package_sha256 TEXT NOT NULL,aggregate_revision BIGINT NOT NULL,expected_parallelism INT NOT NULL,ready_subtasks INT NOT NULL,status TEXT NOT NULL,ready_at TIMESTAMPTZ NOT NULL,last_seen_at TIMESTAMPTZ NOT NULL,expires_at TIMESTAMPTZ NOT NULL,UNIQUE(tenant_id,model_id,aggregate_revision))`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("model shadow fixture: %w", err)
		}
	}
	return nil
}

func insertModelShadowEvent(t *testing.T, db *sql.DB, event ModelUpdateEvent) {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO model_update_outbox(event_id,tenant_id,model_id,model_version,payload) VALUES($1,$2,$3,$4,$5::jsonb)`,
		event.EventID, event.TenantID, event.ModelID, event.Version, string(payload)); err != nil {
		t.Fatal(err)
	}
}

func consumeModelShadowAck(t *testing.T, service *ModelService, ack ModelAppliedAck) {
	t.Helper()
	payload, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleModelAppliedAck(context.Background(), payload); err != nil {
		t.Fatalf("consume shadow acknowledgement subtask %d: %v", ack.SubtaskIndex, err)
	}
}
