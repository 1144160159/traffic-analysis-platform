package service

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestDeploymentRuntimeGateEphemeralKubernetes(t *testing.T) {
	if os.Getenv("DEPLOYMENT_RUNTIME_GATE_K8S_INTEGRATION") != "run-scoped-only" {
		t.Skip("run-scoped Kubernetes sentinel is required")
	}
	dsn := strings.TrimSpace(os.Getenv("DEPLOYMENT_RUNTIME_GATE_K8S_PG_DSN"))
	runID := strings.TrimSpace(os.Getenv("DEPLOYMENT_RUNTIME_GATE_K8S_RUN_ID"))
	if dsn == "" || runID == "" {
		t.Fatal("Kubernetes PostgreSQL and run identity are required")
	}
	parsedRunID, err := uuid.Parse(runID)
	if err != nil || parsedRunID.String() != runID {
		t.Fatalf("canonical Kubernetes run UUID is required: %q", runID)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var marker string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_rule_model_review_sentinel LIMIT 1`).Scan(&marker); err != nil || marker != "ephemeral-only" {
		t.Fatalf("refusing non-ephemeral PostgreSQL: marker=%q err=%v", marker, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	tenantID := "m09-n019-" + strings.ReplaceAll(runID, "-", "")[:12]
	ruleID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":rule")).String()
	modelID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":model")).String()
	ruleVersion := "rule-version-7"
	modelVersion := "model-version-3"
	deploymentID := "deployment-" + strings.ReplaceAll(runID, "-", "")[:12]
	oldDeploymentID := "rollback-" + strings.ReplaceAll(runID, "-", "")[:12]
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenants(tenant_id,name) VALUES($1,'M09 N019 rule model review')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_versions(rule_version,rule_id,tenant_id,version) VALUES($1,$2,$3,7)`, ruleVersion, ruleID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO model_versions(model_version,model_id,tenant_id) VALUES($1,$2,$3)`, modelVersion, modelID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO deployments(deployment_id,tenant_id,rule_version,model_version,status) VALUES($1,$2,$3,$4,'gray'),($5,$2,'rule-version-6','model-version-2','superseded')`, deploymentID, tenantID, ruleVersion, modelVersion, oldDeploymentID); err != nil {
		t.Fatal(err)
	}

	ruleEventOne := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":rule-event-1")).String()
	modelEvent := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":model-event")).String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_outbox(rule_id,payload,published,runtime_status) VALUES($1,jsonb_build_object('event_id',$2::text,'version',7,'rule',jsonb_build_object('tenant_id',$3::text)),true,'partial')`, ruleID, ruleEventOne, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_update_applied_acks(event_id,subtask_index,parallelism,status) VALUES($1,0,2,'applied')`, ruleEventOne); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO model_update_outbox(event_id,tenant_id,model_id,model_version,action,status,published_at) VALUES($1,$2,$3,$4,'activate','published',now())`, modelEvent, tenantID, modelID, modelVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO model_update_applied_acks(event_id,subtask_index,parallelism,status) VALUES($1,0,2,'applied'),($1,1,2,'applied')`, modelEvent); err != nil {
		t.Fatal(err)
	}

	deployment := &model.Deployment{
		DeploymentID: deploymentID, TenantID: tenantID,
		RuleVersion: ruleVersion, ModelVersion: modelVersion,
		Status: string(model.DeploymentStatusGray),
	}
	config := DefaultDeploymentServiceConfig()
	config.EnableRuntimeAckGate = true
	config.RuleAppliedExpectedParallelism = 2
	config.ModelAppliedExpectedParallelism = 2
	service := &DeploymentService{config: config}

	partial, err := service.discoverDeploymentRuntimeGate(ctx, tx, deployment, false)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Ready || partial.ExpansionAllowed || partial.Rule == nil || partial.Rule.Status != "partial" || partial.Model == nil || partial.Model.Status != "applied" {
		t.Fatalf("partial runtime ACK did not stop expansion: %+v", partial)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_update_applied_acks(event_id,subtask_index,parallelism,status) VALUES($1,1,2,'applied')`, ruleEventOne); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE rule_outbox SET runtime_status='applied',runtime_applied_at=now() WHERE payload->>'event_id'=$1`, ruleEventOne); err != nil {
		t.Fatal(err)
	}
	approved, err := service.discoverDeploymentRuntimeGate(ctx, tx, deployment, false)
	if err != nil {
		t.Fatal(err)
	}
	if !approved.Ready || approved.Rule.ComponentID != ruleVersion || approved.Model.ComponentID != modelVersion {
		t.Fatalf("exact rule/model ACK set did not become version-bound ready state: %+v", approved)
	}
	configuration := map[string]interface{}{deploymentRuntimeGateConfigurationKey: approved}
	if _, err := service.verifyApprovedDeploymentRuntimeGate(ctx, tx, deployment, configuration, false); err != nil {
		t.Fatalf("unchanged exact approval receipt was rejected: %v", err)
	}

	ruleEventTwo := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":rule-event-2")).String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_outbox(rule_id,payload,published,runtime_status) VALUES($1,jsonb_build_object('event_id',$2::text,'version',7,'rule',jsonb_build_object('tenant_id',$3::text)),true,'partial')`, ruleID, ruleEventTwo, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_update_applied_acks(event_id,subtask_index,parallelism,status) VALUES($1,0,2,'applied')`, ruleEventTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := service.verifyApprovedDeploymentRuntimeGate(ctx, tx, deployment, configuration, false); err == nil || !strings.Contains(err.Error(), "blocked expansion") {
		t.Fatalf("new partial event did not fail closed: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_update_applied_acks(event_id,subtask_index,parallelism,status) VALUES($1,1,2,'applied')`, ruleEventTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE rule_outbox SET runtime_status='applied',runtime_applied_at=now() WHERE payload->>'event_id'=$1`, ruleEventTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := service.verifyApprovedDeploymentRuntimeGate(ctx, tx, deployment, configuration, false); err == nil || !strings.Contains(err.Error(), "event changed after approval") {
		t.Fatalf("event drift did not require a new independent approval: %v", err)
	}
	approved, err = service.discoverDeploymentRuntimeGate(ctx, tx, deployment, false)
	if err != nil {
		t.Fatal(err)
	}
	configuration[deploymentRuntimeGateConfigurationKey] = approved

	grayEvent := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":gray-event")).String()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deployment_outbox(event_id,deployment_id,tenant_id,event_type,status)
		VALUES($1,$2,$3,'gray_started','published')
	`, grayEvent, deploymentID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.verifyApprovedDeploymentRuntimeGate(ctx, tx, deployment, configuration, true); err == nil || !strings.Contains(err.Error(), "gray deployment projection") {
		t.Fatalf("missing gray projection ACK did not stop activation: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deployment_event_projection(event_id,deployment_id,tenant_id,action,status,kafka_partition,kafka_offset)
		VALUES($1::uuid,$2,$3,'gray_started','gray',1,42)
	`, grayEvent, deploymentID, tenantID); err != nil {
		t.Fatal(err)
	}
	ready, err := service.verifyApprovedDeploymentRuntimeGate(ctx, tx, deployment, configuration, true)
	if err != nil {
		t.Fatalf("complete gray projection ACK was rejected: %v", err)
	}
	if !ready.ExpansionAllowed || ready.DeploymentProjection == nil || ready.DeploymentProjection.KafkaPartition == nil || *ready.DeploymentProjection.KafkaPartition != 1 || ready.DeploymentProjection.KafkaOffset == nil || *ready.DeploymentProjection.KafkaOffset != 42 {
		t.Fatalf("gray projection receipt is incomplete: %+v", ready)
	}

	var recoverable int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM deployments WHERE deployment_id=$1 AND tenant_id=$2 AND rule_version='rule-version-6' AND model_version='model-version-2' AND status='superseded'`, oldDeploymentID, tenantID).Scan(&recoverable); err != nil || recoverable != 1 {
		t.Fatalf("old rollback target was not preserved: count=%d err=%v", recoverable, err)
	}
	t.Log("PASS partial_ack_blocked=true exact_ack_ready=true event_drift_reapproval=true gray_projection_required=true old_version_recoverable=true")
}
