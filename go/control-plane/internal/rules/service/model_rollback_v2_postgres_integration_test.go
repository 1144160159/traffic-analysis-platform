package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func TestModelRollbackV2PostgresExactQuorumAndCompensation(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MODEL_ROLLBACK_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("MODEL_ROLLBACK_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "model_rollback_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := createModelRollbackFixture(ctx, db); err != nil {
		t.Fatal(err)
	}
	migrationPath := filepath.Join("..", "..", "..", "..", "..", "deployments", "postgres",
		"migrations", "202608151900_m08_model_rollback_v2.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read rollback migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatalf("apply rollback migration: %v", err)
	}

	config := DefaultModelServiceConfig()
	config.EnableModelRollbackV2 = true
	config.AppliedAckExpectedParallelism = 2
	config.ModelConsumerDeploymentID = "behavior-job-r1"
	config.ModelConsumerProfileSHA256 = strings.Repeat("c", 64)
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), config)

	rollbackID := "22222222-2222-4222-8222-222222222222"
	event := rollbackIntegrationEvent(
		"rollback-event-success", rollbackID, modelRollbackPhaseAttempt, "model-v1", "model-v2", 2,
	)
	seedModelRollbackRequest(t, db, event, rollbackID, "job-success", "model-v2", 2, "model-v1", 1)
	consumeModelRollbackAck(t, service, rollbackIntegrationAck(event, 0, "applied", ""))
	assertRollbackState(t, db, "job-success", "PARTIAL", 1, false)
	assertActiveModelVersion(t, db, "model-v2")

	consumeModelRollbackAck(t, service, rollbackIntegrationAck(event, 1, "applied", ""))
	assertRollbackState(t, db, "job-success", "RECOVERED", 2, true)
	assertActiveModelVersion(t, db, "model-v1")
	var jobStatus string
	if err := db.QueryRow(`SELECT status FROM model_action_jobs WHERE job_id='job-success'`).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "completed" {
		t.Fatalf("successful rollback job status=%s, want completed", jobStatus)
	}

	// Start another attempt and fail one target subtask.  The Go state machine
	// must enqueue the exact source package as compensation and retain model-v3
	// as the database active pointer.
	if _, err := db.ExecContext(ctx, `
		UPDATE model_versions SET status='deprecated' WHERE model_version='model-v1';
		INSERT INTO model_versions (
			model_version,model_id,tenant_id,feature_set_id,artifact_uri,artifact_manifest_uri,
			package_id,package_sha256,artifact_manifest_sha256,evaluation_sha256,
			explanation_sha256,graph_snapshot_id,graph_snapshot_sha256,signing_key_id,
			compatibility,revision,registration_idempotency_key,registration_request_sha256,
			metrics,status,created_at,updated_at
		) VALUES (
			'model-v3','11111111-1111-4111-8111-111111111111','tenant-a','feature-v1',
			's3://models/v3/model.onnx','s3://models/v3/manifest.json',
			'33333333-3333-4333-8333-333333333333',repeat('f',64),repeat('f',64),
			repeat('f',64),repeat('f',64),'graph-v3',repeat('f',64),'signing-key-v1',
			'{"runtime_contract":"traffic.behavior.inference.v1","runtime_version":"1.0.0","feature_schema_version":1,"graph_schema_version":1,"feature_set_id":"feature-v1","baseline":{"format":"onnx"},"gnn":{"format":"numpy_npz_v1"}}',
			3,'registration-v3',repeat('f',64),jsonb_build_object('artifact_sha256',repeat('f',64)),
			'active',now(),now()
		);
		UPDATE model_versions SET status='deprecated' WHERE model_version='model-v2';
	`); err != nil {
		t.Fatal(err)
	}
	failureRollbackID := "44444444-4444-4444-8444-444444444444"
	failureEvent := rollbackIntegrationEvent(
		"rollback-event-failure", failureRollbackID, modelRollbackPhaseAttempt, "model-v2", "model-v3", 3,
	)
	seedModelRollbackRequest(t, db, failureEvent, failureRollbackID, "job-failure", "model-v3", 3, "model-v2", 2)
	consumeModelRollbackAck(t, service, rollbackIntegrationAck(failureEvent, 0, "applied", ""))
	consumeModelRollbackAck(t, service, rollbackIntegrationAck(failureEvent, 1, "failed", "warmup failed"))
	assertRollbackState(t, db, "job-failure", "COMPENSATING", 1, false)
	assertActiveModelVersion(t, db, "model-v3")

	var compensationPayload []byte
	if err := db.QueryRow(`
		UPDATE model_update_outbox SET status='published',published_at=now()
		WHERE event_id=(SELECT compensation_event_id FROM model_rollback_requests WHERE action_job_id='job-failure')
		RETURNING payload
	`).Scan(&compensationPayload); err != nil {
		t.Fatal(err)
	}
	var compensation ModelUpdateEvent
	if err := json.Unmarshal(compensationPayload, &compensation); err != nil {
		t.Fatal(err)
	}
	if compensation.Action != "rollback-compensate" || compensation.Version != "model-v3" ||
		compensation.RollbackPhase != modelRollbackPhaseCompensation {
		t.Fatalf("invalid compensation event: %+v", compensation)
	}
	consumeModelRollbackAck(t, service, rollbackIntegrationAck(compensation, 0, "applied", ""))
	consumeModelRollbackAck(t, service, rollbackIntegrationAck(compensation, 1, "applied", ""))
	assertRollbackState(t, db, "job-failure", "FAILED_RESTORED", 1, false)
	assertActiveModelVersion(t, db, "model-v3")
	if err := db.QueryRow(`SELECT status FROM model_action_jobs WHERE job_id='job-failure'`).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "failed" {
		t.Fatalf("compensated failed rollback job status=%s, want failed", jobStatus)
	}
}

func createModelRollbackFixture(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE tenants (tenant_id TEXT PRIMARY KEY);
		CREATE TABLE feature_sets (feature_set_id TEXT PRIMARY KEY);
		CREATE TABLE models (
			model_id UUID PRIMARY KEY,tenant_id TEXT NOT NULL,name TEXT NOT NULL,
			model_type TEXT NOT NULL,description TEXT NOT NULL
		);
		CREATE TABLE model_versions (
			model_version TEXT PRIMARY KEY,model_id UUID NOT NULL,tenant_id TEXT NOT NULL,
			feature_set_id TEXT NOT NULL,artifact_uri TEXT NOT NULL,artifact_manifest_uri TEXT NOT NULL,
			package_id TEXT NOT NULL,package_sha256 TEXT NOT NULL,artifact_manifest_sha256 TEXT NOT NULL,
			evaluation_sha256 TEXT NOT NULL,explanation_sha256 TEXT NOT NULL,graph_snapshot_id TEXT NOT NULL,
			graph_snapshot_sha256 TEXT NOT NULL,signing_key_id TEXT NOT NULL,compatibility JSONB NOT NULL,
			revision BIGINT NOT NULL,registration_idempotency_key TEXT NOT NULL,
			registration_request_sha256 TEXT NOT NULL,metrics JSONB NOT NULL,status TEXT NOT NULL,
			created_by UUID,created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE model_action_jobs (
			job_id TEXT PRIMARY KEY,status TEXT NOT NULL,result JSONB NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE model_update_outbox (
			id BIGSERIAL PRIMARY KEY,event_id TEXT NOT NULL UNIQUE,tenant_id TEXT NOT NULL,
			model_id TEXT NOT NULL,model_version TEXT NOT NULL,action TEXT NOT NULL,
			partition_key TEXT NOT NULL,payload JSONB NOT NULL,action_job_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			published_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE model_update_applied_acks (
			event_id TEXT NOT NULL,tenant_id TEXT NOT NULL,model_id TEXT NOT NULL,
			model_version TEXT NOT NULL,subtask_index INTEGER NOT NULL,parallelism INTEGER NOT NULL,
			status TEXT NOT NULL,artifact_uri TEXT NOT NULL,artifact_sha256 TEXT NOT NULL,
			warmup_score DOUBLE PRECISION,error TEXT NOT NULL,payload JSONB NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(event_id,subtask_index)
		);
		CREATE TABLE audit_logs (
			event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id TEXT NOT NULL,
			user_id UUID,action TEXT NOT NULL,object_type TEXT NOT NULL,object_id TEXT NOT NULL,
			detail JSONB NOT NULL
		);
		INSERT INTO tenants VALUES ('tenant-a');
		INSERT INTO feature_sets VALUES ('feature-v1');
		INSERT INTO models VALUES (
			'11111111-1111-4111-8111-111111111111','tenant-a','behavior','xgboost','fixture'
		);
		INSERT INTO model_versions (
			model_version,model_id,tenant_id,feature_set_id,artifact_uri,artifact_manifest_uri,
			package_id,package_sha256,artifact_manifest_sha256,evaluation_sha256,
			explanation_sha256,graph_snapshot_id,graph_snapshot_sha256,signing_key_id,
			compatibility,revision,registration_idempotency_key,registration_request_sha256,
			metrics,status,created_at,updated_at
		) VALUES
		('model-v1','11111111-1111-4111-8111-111111111111','tenant-a','feature-v1',
		 's3://models/v1/model.onnx','s3://models/v1/manifest.json',
		 '11111111-1111-4111-8111-111111111111',repeat('a',64),repeat('a',64),repeat('a',64),
		 repeat('a',64),'graph-v1',repeat('a',64),'signing-key-v1',
		 '{"runtime_contract":"traffic.behavior.inference.v1","runtime_version":"1.0.0","feature_schema_version":1,"graph_schema_version":1,"feature_set_id":"feature-v1","baseline":{"format":"onnx"},"gnn":{"format":"numpy_npz_v1"}}',
		 1,'registration-v1',repeat('a',64),jsonb_build_object('artifact_sha256',repeat('a',64)),
		 'deprecated',now()-interval '2 hours',now()-interval '2 hours'),
		('model-v2','11111111-1111-4111-8111-111111111111','tenant-a','feature-v1',
		 's3://models/v2/model.onnx','s3://models/v2/manifest.json',
		 '22222222-2222-4222-8222-222222222222',repeat('b',64),repeat('b',64),repeat('b',64),
		 repeat('b',64),'graph-v2',repeat('b',64),'signing-key-v1',
		 '{"runtime_contract":"traffic.behavior.inference.v1","runtime_version":"1.0.0","feature_schema_version":1,"graph_schema_version":1,"feature_set_id":"feature-v1","baseline":{"format":"onnx"},"gnn":{"format":"numpy_npz_v1"}}',
		 2,'registration-v2',repeat('b',64),jsonb_build_object('artifact_sha256',repeat('b',64)),
		 'active',now()-interval '1 hour',now()-interval '1 hour');
	`)
	return err
}

func rollbackIntegrationEvent(eventID, rollbackID, phase, version, fromVersion string, activeRevision int64) ModelUpdateEvent {
	artifactSHA := strings.Repeat("a", 64)
	artifactURI := "s3://models/v1/model.onnx"
	if version == "model-v2" {
		artifactSHA = strings.Repeat("b", 64)
		artifactURI = "s3://models/v2/model.onnx"
	}
	if version == "model-v3" {
		artifactSHA = strings.Repeat("f", 64)
		artifactURI = "s3://models/v3/model.onnx"
	}
	return ModelUpdateEvent{
		EventID: eventID, SchemaVersion: 2, TenantID: "tenant-a",
		ModelID: "11111111-1111-4111-8111-111111111111", Version: version,
		ArtifactURI: artifactURI, Action: "rollback-activated",
		Metrics:                    map[string]interface{}{"artifact_sha256": artifactSHA},
		ExpectedAppliedParallelism: 2, RollbackID: rollbackID, RollbackPhase: phase,
		RollbackFromVersion: fromVersion, ExpectedActiveRevision: activeRevision,
		ConsumerDeploymentID: "behavior-job-r1", ConsumerProfileSHA256: strings.Repeat("c", 64),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func seedModelRollbackRequest(
	t *testing.T, db *sql.DB, event ModelUpdateEvent, rollbackID, jobID,
	fromVersion string, fromRevision int64, targetVersion string, targetRevision int64,
) {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	fromSHA := strings.Repeat("b", 64)
	if fromVersion == "model-v3" {
		fromSHA = strings.Repeat("f", 64)
	}
	targetSHA := strings.Repeat("a", 64)
	if targetVersion == "model-v2" {
		targetSHA = strings.Repeat("b", 64)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO model_action_jobs(job_id,status) VALUES ($1,'running')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
		INSERT INTO model_update_outbox (
			event_id,tenant_id,model_id,model_version,action,partition_key,payload,
			action_job_id,status,published_at
		) VALUES ($1,'tenant-a','11111111-1111-4111-8111-111111111111',$2,
			'rollback-activated','11111111-1111-4111-8111-111111111111',$3::jsonb,$4,'published',now())
	`, event.EventID, targetVersion, string(payload), jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
		INSERT INTO model_rollback_requests (
			rollback_id,action_job_id,rollback_event_id,tenant_id,model_id,
			from_model_version,from_revision,from_package_sha256,
			target_model_version,target_revision,target_package_sha256,
			consumer_deployment_id,consumer_profile_sha256,expected_parallelism,
			request_sha256,reason,requested_by
		) VALUES ($1::uuid,$2,$3,'tenant-a','11111111-1111-4111-8111-111111111111',
			$4,$5,$6,$7,$8,$9,'behavior-job-r1',$10,2,$11,
			'rollback after the exact canary stop threshold fired',
			'99999999-9999-4999-8999-999999999999')
	`, rollbackID, jobID, event.EventID, fromVersion, fromRevision, fromSHA,
		targetVersion, targetRevision, targetSHA, strings.Repeat("c", 64),
		strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func rollbackIntegrationAck(event ModelUpdateEvent, subtask int, status, failure string) ModelAppliedAck {
	artifactSHA, _ := event.Metrics["artifact_sha256"].(string)
	return ModelAppliedAck{
		SchemaVersion: event.SchemaVersion, EventID: event.EventID, TenantID: event.TenantID,
		ModelID: event.ModelID, Version: event.Version, ArtifactURI: event.ArtifactURI,
		ArtifactSHA256: artifactSHA, WarmupScore: 1, SubtaskIndex: subtask, Parallelism: 2,
		Status: status, Error: failure, AckType: "model_update",
		ConsumerDeploymentID:  event.ConsumerDeploymentID,
		ConsumerProfileSHA256: event.ConsumerProfileSHA256,
		RollbackID:            event.RollbackID, RollbackPhase: event.RollbackPhase,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func consumeModelRollbackAck(t *testing.T, service *ModelService, ack ModelAppliedAck) {
	t.Helper()
	payload, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleModelAppliedAck(context.Background(), payload); err != nil {
		t.Fatalf("consume rollback acknowledgement: %v", err)
	}
}

func assertRollbackState(t *testing.T, db *sql.DB, jobID, wantState string, wantApplied int, wantSwitched bool) {
	t.Helper()
	var state string
	var applied int
	var switched bool
	if err := db.QueryRow(`
		SELECT state,applied_subtasks,active_switched
		FROM model_rollback_requests WHERE action_job_id=$1
	`, jobID).Scan(&state, &applied, &switched); err != nil {
		t.Fatal(err)
	}
	if state != wantState || applied != wantApplied || switched != wantSwitched {
		t.Fatalf("rollback state=(%s,%d,%t), want (%s,%d,%t)",
			state, applied, switched, wantState, wantApplied, wantSwitched)
	}
}

func assertActiveModelVersion(t *testing.T, db *sql.DB, want string) {
	t.Helper()
	var active string
	if err := db.QueryRow(`SELECT model_version FROM model_versions WHERE status='active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != want {
		t.Fatalf("active model=%s, want %s", active, want)
	}
}
