package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestModelConsumerReadinessPostgresExactProfileQuorum(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MODEL_CONSUMER_READY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("MODEL_CONSUMER_READY_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "model_consumer_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := createModelConsumerReadinessFixture(ctx, db); err != nil {
		t.Fatal(err)
	}

	base := validModelConsumerReadyAck()
	config := DefaultModelServiceConfig()
	config.AppliedAckExpectedParallelism = 4
	config.ModelConsumerDeploymentID = base.ConsumerDeploymentID
	config.ModelConsumerProfileSHA256 = base.ConsumerProfileSHA256
	config.ModelConsumerReadyTTL = 2 * time.Minute
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), config)

	for subtask := 0; subtask < 3; subtask++ {
		consumeModelConsumerReadyAck(t, service, readinessAckForSubtask(base, subtask))
	}
	assertTableCount(t, db, "model_update_consumer_readiness", 3)
	assertTableCount(t, db, "model_update_consumer_ready_receipts", 0)

	consumeModelConsumerReadyAck(t, service, readinessAckForSubtask(base, 3))
	assertTableCount(t, db, "model_update_consumer_ready_receipts", 1)
	var ready, expected int
	var profile, status string
	var expiresAt time.Time
	if err := db.QueryRow(`SELECT ready_subtasks,expected_parallelism,consumer_profile_sha256,status,expires_at FROM model_update_consumer_ready_receipts WHERE consumer_deployment_id=$1`, base.ConsumerDeploymentID).
		Scan(&ready, &expected, &profile, &status, &expiresAt); err != nil {
		t.Fatal(err)
	}
	if ready != 4 || expected != 4 || profile != base.ConsumerProfileSHA256 || status != "ready" {
		t.Fatalf("consumer quorum=%d/%d profile=%s status=%s", ready, expected, profile, status)
	}
	if remaining := time.Until(expiresAt); remaining <= time.Minute || remaining > 3*time.Minute {
		t.Fatalf("consumer ready receipt TTL is outside the configured window: %v", remaining)
	}

	consumeModelConsumerReadyAck(t, service, readinessAckForSubtask(base, 0))
	assertTableCount(t, db, "model_update_consumer_readiness", 4)
	assertTableCount(t, db, "model_update_consumer_ready_receipts", 1)

	drift := readinessAckForSubtask(base, 0)
	drift.ConsumerProfileSHA256 = strings.Repeat("b", 64)
	drift.ArtifactSHA256 = drift.ConsumerProfileSHA256
	payload, err := json.Marshal(drift)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleModelAppliedAck(ctx, payload); err == nil ||
		!strings.Contains(err.Error(), "does not match server contract") {
		t.Fatalf("profile drift did not fail closed: %v", err)
	}
	assertTableCount(t, db, "model_update_consumer_readiness", 4)
}

func createModelConsumerReadinessFixture(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE model_update_consumer_readiness (consumer_deployment_id TEXT NOT NULL,subtask_index INT NOT NULL,event_id TEXT NOT NULL UNIQUE,consumer_profile_sha256 TEXT NOT NULL,runtime_contract TEXT NOT NULL,runtime_version TEXT NOT NULL,feature_schema_version INT NOT NULL,graph_schema_version INT NOT NULL,supported_model_formats TEXT NOT NULL,parallelism INT NOT NULL,status TEXT NOT NULL,payload JSONB NOT NULL,received_at TIMESTAMPTZ NOT NULL,last_seen_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(consumer_deployment_id,subtask_index))`,
		`CREATE TABLE model_update_consumer_ready_receipts (consumer_deployment_id TEXT PRIMARY KEY,consumer_profile_sha256 TEXT NOT NULL,runtime_contract TEXT NOT NULL,runtime_version TEXT NOT NULL,feature_schema_version INT NOT NULL,graph_schema_version INT NOT NULL,supported_model_formats TEXT NOT NULL,expected_parallelism INT NOT NULL,ready_subtasks INT NOT NULL,status TEXT NOT NULL,ready_at TIMESTAMPTZ NOT NULL,last_seen_at TIMESTAMPTZ NOT NULL,expires_at TIMESTAMPTZ NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("model consumer readiness fixture: %w", err)
		}
	}
	return nil
}

func readinessAckForSubtask(base ModelAppliedAck, subtask int) ModelAppliedAck {
	ack := base
	ack.SubtaskIndex = subtask
	ack.EventID = fmt.Sprintf("11111111-1111-8111-8111-%012d", subtask)
	return ack
}

func consumeModelConsumerReadyAck(t *testing.T, service *ModelService, ack ModelAppliedAck) {
	t.Helper()
	payload, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleModelAppliedAck(context.Background(), payload); err != nil {
		t.Fatalf("consume model readiness subtask %d: %v", ack.SubtaskIndex, err)
	}
}
