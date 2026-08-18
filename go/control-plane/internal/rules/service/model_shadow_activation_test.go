package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	rulespublisher "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/publisher"
)

const (
	shadowTestModelID     = "11111111-1111-4111-8111-111111111111"
	shadowTestRequesterID = "22222222-2222-4222-8222-222222222222"
	shadowTestApproverID  = "33333333-3333-4333-8333-333333333333"
	shadowTestPackageID   = "44444444-4444-4444-8444-444444444444"
)

func governedShadowCompatibility() map[string]interface{} {
	return map[string]interface{}{
		"runtime_contract":       "traffic.behavior.inference.v1",
		"runtime_version":        "1.0.0",
		"feature_set_id":         "feature-v1",
		"feature_schema_version": float64(1),
		"graph_schema_version":   float64(1),
		"baseline":               map[string]interface{}{"format": "onnx"},
		"gnn":                    map[string]interface{}{"format": "numpy_npz_v1"},
	}
}

func TestPrepareModelShadowActivationFailClosedValidation(t *testing.T) {
	revision := int64(0)
	req := &PrepareModelShadowActivationRequest{
		ModelID: shadowTestModelID, ModelVersion: "v1", IdempotencyKey: "shadow-activation-0001",
		ExpectedRevision: &revision, RequestedBy: shadowTestRequesterID,
		ApprovalReason: "approved for isolated shadow loading",
	}
	opCtx := &OperationContext{TenantID: "tenant-a", UserID: shadowTestApproverID, Authenticated: true}
	service := &ModelService{config: DefaultModelServiceConfig()}
	if _, err := service.PrepareModelShadowActivation(context.Background(), req, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeServiceUnavailable) {
		t.Fatalf("default-off writer returned %v", err)
	}

	req.RequestedBy = shadowTestApproverID
	if err := req.validate(opCtx.TenantID, opCtx.UserID); !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidRequest) {
		t.Fatalf("self-approval returned %v", err)
	}
	req.RequestedBy = shadowTestRequesterID
	req.ExpectedRevision = nil
	if err := req.validate(opCtx.TenantID, opCtx.UserID); !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidRequest) {
		t.Fatalf("missing expected revision returned %v", err)
	}
}

func TestValidateGovernedShadowMetadataRequiresExactConsumerContract(t *testing.T) {
	config := DefaultModelServiceConfig()
	service := &ModelService{config: config}
	digest := strings.Repeat("a", 64)
	mv := &model.ModelVersion{
		FeatureSetID: "feature-v1", ArtifactURI: "s3://models/baseline.onnx",
		ArtifactManifestURI: "s3://models/model-artifact-manifest.json",
		PackageID:           shadowTestPackageID, PackageSHA256: digest, ArtifactManifestSHA256: digest,
		EvaluationSHA256: digest, ExplanationSHA256: digest, GraphSnapshotID: "graph-1",
		GraphSnapshotSHA256: digest, SigningKeyID: "ed25519-key-1",
		Compatibility: governedShadowCompatibility(),
	}
	if err := service.validateGovernedShadowMetadata(mv); err != nil {
		t.Fatalf("exact governed metadata rejected: %v", err)
	}
	mv.Compatibility["graph_schema_version"] = float64(2)
	if err := service.validateGovernedShadowMetadata(mv); !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidStateTransition) {
		t.Fatalf("graph schema drift returned %v", err)
	}
}

func TestPrepareModelShadowActivationPostgresAtomicContract(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MODEL_SHADOW_ACTIVATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("MODEL_SHADOW_ACTIVATION_TEST_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "model_shadow_activation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := createModelShadowActivationFixture(ctx, db); err != nil {
		t.Fatal(err)
	}

	config := DefaultModelServiceConfig()
	config.EnableModelShadowActivation = true
	config.EnableModelShadowPublisher = false
	config.ModelConsumerDeploymentID = "behavior-shadow-deployment-v1"
	config.ModelConsumerProfileSHA256 = strings.Repeat("b", 64)
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), config)
	revision := int64(0)
	req := &PrepareModelShadowActivationRequest{
		ModelID: shadowTestModelID, ModelVersion: "v1", IdempotencyKey: "shadow-activation-0001",
		ExpectedRevision: &revision, RequestedBy: shadowTestRequesterID,
		ApprovalReason: "independent approval for isolated shadow loading",
	}
	opCtx := &OperationContext{
		TenantID: "tenant-a", UserID: shadowTestApproverID, Username: "model-approver", Authenticated: true,
	}

	first, err := service.PrepareModelShadowActivation(ctx, req, opCtx)
	if err != nil {
		t.Fatalf("prepare first shadow activation: %v", err)
	}
	if first.State != "outbox_pending" || first.AggregateRevision != 1 || first.ServingActivated {
		t.Fatalf("unexpected first receipt: %+v", first)
	}
	assertModelVersionState(t, db, "v0", "active", 1)
	assertModelVersionState(t, db, "v1", "registered", 1)
	assertTableCount(t, db, "model_shadow_activation_requests", 1)
	assertTableCount(t, db, "model_update_outbox", 1)
	assertTableCount(t, db, "audit_logs", 1)
	assertShadowEvent(t, db, first.EventID)
	readReceipt, err := service.GetModelShadowActivationReceipt(ctx, shadowTestModelID, "v1", first.RequestID, opCtx)
	if err != nil || readReceipt.EventID != first.EventID || readReceipt.State != "outbox_pending" || readReceipt.ServingActivated {
		t.Fatalf("read shadow activation receipt: receipt=%+v err=%v", readReceipt, err)
	}
	if err := service.ActivateModelVersion(ctx, shadowTestModelID, "v1", 100, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidStateTransition) {
		t.Fatalf("legacy direct activation of governed package returned %v", err)
	}
	assertModelVersionState(t, db, "v0", "active", 1)
	assertModelVersionState(t, db, "v1", "registered", 1)
	assertTableCount(t, db, "model_update_outbox", 1)

	// The publisher flag is independently default-off: the writer may commit a
	// pending fact, but the dispatcher must not even claim it.
	claimed, err := service.claimModelUpdateOutbox(ctx, 20)
	if err != nil {
		t.Fatalf("claim with shadow publisher disabled: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("disabled shadow publisher claimed %+v", claimed)
	}
	if broker := strings.TrimSpace(os.Getenv("MODEL_SHADOW_ACTIVATION_TEST_KAFKA_BROKER")); broker != "" {
		topic := strings.TrimSpace(os.Getenv("MODEL_SHADOW_ACTIVATION_TEST_KAFKA_TOPIC"))
		if topic == "" {
			t.Fatal("MODEL_SHADOW_ACTIVATION_TEST_KAFKA_TOPIC is required with Kafka broker")
		}
		createShadowKafkaTopic(t, broker, topic)
		publishConfig := rulespublisher.DefaultPublisherConfig()
		publishConfig.Brokers = []string{broker}
		publishConfig.RuleTopic = topic + "-unused-rule"
		publishConfig.ModelTopic = topic
		publishConfig.ModelActionTopic = ""
		publishConfig.DeploymentTopic = ""
		publishConfig.AuditTopic = ""
		publishConfig.Compression = "none"
		modelPublisher, err := rulespublisher.NewKafkaPublisherWithConfig(publishConfig, zap.NewNop())
		if err != nil {
			t.Fatalf("create real Kafka model publisher: %v", err)
		}
		service.publisher = modelPublisher
		service.config.EnableModelShadowPublisher = true
		if err := service.processModelUpdateOutbox(ctx); err != nil {
			_ = modelPublisher.Close()
			t.Fatalf("publish shadow outbox: %v", err)
		}
		if err := modelPublisher.Close(); err != nil {
			t.Fatalf("close real Kafka model publisher: %v", err)
		}
		var outboxStatus string
		if err := db.QueryRowContext(ctx, `SELECT status FROM model_update_outbox WHERE event_id=$1`, first.EventID).Scan(&outboxStatus); err != nil {
			t.Fatal(err)
		}
		if outboxStatus != "published" {
			diagnosticClaim, diagnosticErr := service.claimModelUpdateOutbox(ctx, 20)
			var action string
			var available, blockedByPrior bool
			var availableAt, databaseNow time.Time
			var lastError string
			var attempts int
			_ = db.QueryRowContext(ctx, `SELECT current.action,current.available_at<=now(),current.available_at,clock_timestamp(),current.last_error,current.attempt_count,EXISTS(
				SELECT 1 FROM model_update_outbox prior WHERE prior.model_id=current.model_id
				AND prior.id<current.id AND prior.status NOT IN ('published','dead'))
				FROM model_update_outbox current WHERE current.event_id=$1`, first.EventID).
				Scan(&action, &available, &availableAt, &databaseNow, &lastError, &attempts, &blockedByPrior)
			t.Fatalf("broker-acknowledged shadow outbox status=%s action=%s available=%t available_at=%s db_now=%s attempts=%d last_error=%q prior=%t notification=%t shadow_publisher=%t publisher_nil=%t diagnostic_claim=%d diagnostic_err=%v",
				outboxStatus, action, available, availableAt.Format(time.RFC3339Nano), databaseNow.Format(time.RFC3339Nano), attempts, lastError, blockedByPrior, service.config.EnableKafkaNotification, service.config.EnableModelShadowPublisher,
				service.publisher == nil, len(diagnosticClaim), diagnosticErr)
		}
		assertPublishedShadowKafkaRecord(t, broker, topic, first.EventID, shadowTestModelID)
	}

	second, err := service.PrepareModelShadowActivation(ctx, req, opCtx)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if second.RequestID != first.RequestID || second.EventID != first.EventID || second.RequestSHA256 != first.RequestSHA256 {
		t.Fatal("idempotent replay returned a different durable identity")
	}
	assertTableCount(t, db, "model_shadow_activation_requests", 1)
	assertTableCount(t, db, "model_update_outbox", 1)
	if _, err := db.ExecContext(ctx, `INSERT INTO model_update_shadow_acks(event_id,status) VALUES ($1,'failed')`, first.EventID); err != nil {
		t.Fatal(err)
	}
	failedReceipt, err := service.GetModelShadowActivationReceipt(ctx, shadowTestModelID, "v1", first.RequestID, opCtx)
	if err != nil || failedReceipt.State != "failed" || failedReceipt.ServingActivated {
		t.Fatalf("failed shadow acknowledgement was not surfaced: receipt=%+v err=%v", failedReceipt, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM model_update_shadow_acks WHERE event_id=$1`, first.EventID); err != nil {
		t.Fatal(err)
	}

	conflict := *req
	conflict.ApprovalReason = "different command content must be rejected"
	if _, err := service.PrepareModelShadowActivation(ctx, &conflict, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("idempotency conflict returned %v", err)
	}

	stale := *req
	stale.IdempotencyKey = "shadow-activation-0002"
	if _, err := service.PrepareModelShadowActivation(ctx, &stale, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("stale aggregate revision returned %v", err)
	}
	assertTableCount(t, db, "model_shadow_activation_requests", 1)

	if _, err := db.ExecContext(ctx, `UPDATE model_update_consumer_ready_receipts SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	nextRevision := int64(1)
	expired := *req
	expired.IdempotencyKey = "shadow-activation-0003"
	expired.ExpectedRevision = &nextRevision
	if _, err := service.PrepareModelShadowActivation(ctx, &expired, opCtx); !commonerrors.IsCode(err, commonerrors.ErrCodeInvalidStateTransition) {
		t.Fatalf("expired consumer receipt returned %v", err)
	}
	assertTableCount(t, db, "model_shadow_activation_requests", 1)

	// Audit persistence is part of the same serializable transaction. Removing
	// it forces every newly written command/outbox/revision fact to roll back.
	if _, err := db.ExecContext(ctx, `UPDATE model_update_consumer_ready_receipts SET expires_at=now()+interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE audit_logs`); err != nil {
		t.Fatal(err)
	}
	rollback := *req
	rollback.IdempotencyKey = "shadow-activation-0004"
	rollback.ExpectedRevision = &nextRevision
	if _, err := service.PrepareModelShadowActivation(ctx, &rollback, opCtx); err == nil {
		t.Fatal("shadow activation succeeded without transactional audit storage")
	}
	assertTableCount(t, db, "model_shadow_activation_requests", 1)
	assertTableCount(t, db, "model_update_outbox", 1)
	var aggregate int64
	if err := db.QueryRowContext(ctx, `SELECT aggregate_revision FROM model_shadow_activation_aggregates`).Scan(&aggregate); err != nil {
		t.Fatal(err)
	}
	if aggregate != 1 {
		t.Fatalf("rolled-back command advanced aggregate to %d", aggregate)
	}
}

func createModelShadowActivationFixture(ctx context.Context, db *sql.DB) error {
	compatibility, _ := json.Marshal(governedShadowCompatibility())
	digest := strings.Repeat("a", 64)
	statements := []string{
		`CREATE TABLE tenants (tenant_id TEXT PRIMARY KEY)`,
		`CREATE TABLE models (model_id UUID PRIMARY KEY,tenant_id TEXT NOT NULL,name TEXT NOT NULL,model_type TEXT NOT NULL,description TEXT,metadata JSONB NOT NULL DEFAULT '{}'::jsonb,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE model_versions (model_version TEXT PRIMARY KEY,model_id UUID NOT NULL,tenant_id TEXT NOT NULL,feature_set_id TEXT NOT NULL,artifact_uri TEXT NOT NULL,artifact_manifest_uri TEXT NOT NULL,package_id TEXT NOT NULL,package_sha256 TEXT NOT NULL,artifact_manifest_sha256 TEXT NOT NULL,evaluation_sha256 TEXT NOT NULL,explanation_sha256 TEXT NOT NULL,graph_snapshot_id TEXT NOT NULL,graph_snapshot_sha256 TEXT NOT NULL,signing_key_id TEXT NOT NULL,compatibility JSONB NOT NULL,revision BIGINT NOT NULL,registration_idempotency_key TEXT NOT NULL DEFAULT '',registration_request_sha256 TEXT NOT NULL DEFAULT '',metrics JSONB NOT NULL,status TEXT NOT NULL,created_by TEXT,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE model_workbench_items (item_id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,model_id UUID NOT NULL,category TEXT NOT NULL,ordinal INT NOT NULL,payload JSONB NOT NULL,occurred_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE model_update_outbox (id BIGSERIAL PRIMARY KEY,event_id TEXT NOT NULL UNIQUE,tenant_id TEXT NOT NULL,model_id TEXT NOT NULL,model_version TEXT NOT NULL,action TEXT NOT NULL,partition_key TEXT NOT NULL,payload JSONB NOT NULL,action_job_id TEXT NOT NULL DEFAULT '',status TEXT NOT NULL DEFAULT 'pending',attempt_count INT NOT NULL DEFAULT 0,available_at TIMESTAMPTZ NOT NULL DEFAULT now(),locked_at TIMESTAMPTZ,locked_by TEXT,published_at TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE model_update_consumer_ready_receipts (consumer_deployment_id TEXT PRIMARY KEY,consumer_profile_sha256 TEXT NOT NULL,runtime_contract TEXT NOT NULL,runtime_version TEXT NOT NULL,feature_schema_version INT NOT NULL,graph_schema_version INT NOT NULL,supported_model_formats TEXT NOT NULL,expected_parallelism INT NOT NULL,ready_subtasks INT NOT NULL,status TEXT NOT NULL,ready_at TIMESTAMPTZ NOT NULL,last_seen_at TIMESTAMPTZ NOT NULL,expires_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE model_shadow_activation_aggregates (tenant_id TEXT NOT NULL,model_id UUID NOT NULL,aggregate_revision BIGINT NOT NULL,updated_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(tenant_id,model_id))`,
		`CREATE TABLE model_shadow_activation_requests (request_id TEXT PRIMARY KEY,event_id TEXT NOT NULL UNIQUE,tenant_id TEXT NOT NULL,model_id UUID NOT NULL,model_version TEXT NOT NULL,package_id TEXT NOT NULL,package_sha256 TEXT NOT NULL,idempotency_key TEXT NOT NULL,request_sha256 TEXT NOT NULL,expected_revision BIGINT NOT NULL,aggregate_revision BIGINT NOT NULL,requested_by TEXT NOT NULL,approved_by TEXT NOT NULL,approval_reason TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,UNIQUE(tenant_id,idempotency_key),UNIQUE(tenant_id,model_id,aggregate_revision))`,
		`CREATE TABLE model_update_shadow_ready_receipts (event_id TEXT PRIMARY KEY,expires_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE model_update_shadow_acks (event_id TEXT NOT NULL,status TEXT NOT NULL)`,
		`CREATE TABLE audit_logs (id BIGSERIAL PRIMARY KEY,tenant_id TEXT NOT NULL,user_id TEXT,action TEXT NOT NULL,object_type TEXT NOT NULL,object_id TEXT,detail JSONB NOT NULL,ip_addr TEXT,user_agent TEXT,created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`INSERT INTO tenants VALUES ('tenant-a')`,
		`INSERT INTO models (model_id,tenant_id,name,model_type) VALUES ('` + shadowTestModelID + `','tenant-a','behavior-classifier','onnx')`,
		`INSERT INTO model_versions (model_version,model_id,tenant_id,feature_set_id,artifact_uri,artifact_manifest_uri,package_id,package_sha256,artifact_manifest_sha256,evaluation_sha256,explanation_sha256,graph_snapshot_id,graph_snapshot_sha256,signing_key_id,compatibility,revision,metrics,status,created_by) VALUES ('v0','` + shadowTestModelID + `','tenant-a','feature-v1','s3://models/v0.onnx','s3://models/v0/manifest.json','package-0','` + digest + `','` + digest + `','` + digest + `','` + digest + `','graph-0','` + digest + `','key-1','` + string(compatibility) + `'::jsonb,1,'{}','active','` + shadowTestRequesterID + `')`,
		`INSERT INTO model_versions (model_version,model_id,tenant_id,feature_set_id,artifact_uri,artifact_manifest_uri,package_id,package_sha256,artifact_manifest_sha256,evaluation_sha256,explanation_sha256,graph_snapshot_id,graph_snapshot_sha256,signing_key_id,compatibility,revision,metrics,status,created_by) VALUES ('v1','` + shadowTestModelID + `','tenant-a','feature-v1','s3://models/v1.onnx','s3://models/v1/manifest.json','` + shadowTestPackageID + `','` + digest + `','` + digest + `','` + digest + `','` + digest + `','graph-1','` + digest + `','key-1','` + string(compatibility) + `'::jsonb,1,'{"f1_score":0.9}','registered','` + shadowTestRequesterID + `')`,
		`INSERT INTO model_workbench_items (item_id,tenant_id,model_id,category,ordinal,payload) VALUES ('gate-1','tenant-a','` + shadowTestModelID + `','review_gates',1,'{"name":"security-review","status":"approved","requested_by":"` + shadowTestRequesterID + `","approved_by":"` + shadowTestApproverID + `"}')`,
		`INSERT INTO model_update_consumer_ready_receipts VALUES ('behavior-shadow-deployment-v1','` + strings.Repeat("b", 64) + `','traffic.behavior.inference.v1','1.0.0',1,1,'onnx,numpy_npz_v1',4,4,'ready',now(),now(),now()+interval '5 minutes')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func assertModelVersionState(t *testing.T, db *sql.DB, version, status string, revision int64) {
	t.Helper()
	var actualStatus string
	var actualRevision int64
	if err := db.QueryRow(`SELECT status,revision FROM model_versions WHERE model_version=$1`, version).Scan(&actualStatus, &actualRevision); err != nil {
		t.Fatal(err)
	}
	if actualStatus != status || actualRevision != revision {
		t.Fatalf("model %s state=%s/%d, want %s/%d", version, actualStatus, actualRevision, status, revision)
	}
}

func assertShadowEvent(t *testing.T, db *sql.DB, eventID string) {
	t.Helper()
	var action, status string
	var raw []byte
	if err := db.QueryRow(`SELECT action,status,payload FROM model_update_outbox WHERE event_id=$1`, eventID).Scan(&action, &status, &raw); err != nil {
		t.Fatal(err)
	}
	var event ModelUpdateEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if action != "shadow-load" || status != "pending" || event.SchemaVersion != 2 ||
		event.Action != "shadow-load" || event.AggregateRevision != 1 || event.PackageID != shadowTestPackageID ||
		event.ArtifactManifestURI != "s3://models/v1/manifest.json" {
		t.Fatalf("invalid schema-v2 shadow event: action=%s status=%s event=%+v", action, status, event)
	}
}

func assertPublishedShadowKafkaRecord(t *testing.T, broker, topic, eventID, modelID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	connection, err := segmentkafka.DialLeader(ctx, "tcp", broker, topic, 0)
	if err != nil {
		t.Fatalf("dial published shadow topic: %v", err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	message, err := connection.ReadMessage(1 << 20)
	if err != nil {
		t.Fatalf("read published shadow record: %v", err)
	}
	var event ModelUpdateEvent
	if err := json.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	headerEventID := ""
	for _, header := range message.Headers {
		if header.Key == "event_id" {
			headerEventID = string(header.Value)
		}
	}
	if string(message.Key) != modelID || headerEventID != eventID || event.EventID != eventID ||
		event.SchemaVersion != 2 || event.Action != "shadow-load" || event.AggregateRevision != 1 {
		t.Fatalf("Kafka shadow record does not match outbox: key=%s header=%s event=%+v", message.Key, headerEventID, event)
	}
}

func createShadowKafkaTopic(t *testing.T, broker, topic string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	connection, err := segmentkafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		t.Fatalf("dial Kafka controller for shadow topic: %v", err)
	}
	defer connection.Close()
	if err := connection.CreateTopics(segmentkafka.TopicConfig{
		Topic: topic, NumPartitions: 1, ReplicationFactor: 1,
	}); err != nil {
		t.Fatalf("create isolated shadow topic: %v", err)
	}
}
