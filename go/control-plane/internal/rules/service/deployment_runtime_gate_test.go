package service

import (
	"context"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/DATA-DOG/go-sqlmock"
)

func runtimeGateTestService(dbConfig DeploymentServiceConfig) *DeploymentService {
	return &DeploymentService{config: dbConfig}
}

func runtimeGateConfig() DeploymentServiceConfig {
	config := DefaultDeploymentServiceConfig()
	config.EnableRuntimeAckGate = true
	config.RuleAppliedExpectedParallelism = 2
	config.ModelAppliedExpectedParallelism = 2
	return config
}

func runtimeGateDeployment() *model.Deployment {
	return &model.Deployment{
		DeploymentID: "deployment-1",
		TenantID:     "tenant-a",
		RuleVersion:  "rule-version-7",
		ModelVersion: "model-version-3",
		Status:       string(model.DeploymentStatusGray),
	}
}

func expectRuleRuntimeReceipt(mock sqlmock.Sqlmock, eventID, status string, success, failed int) {
	mock.ExpectQuery("SELECT COALESCE\\(o.payload->>'event_id'").
		WithArgs("rule-version-7", "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "published", "runtime_status", "runtime_applied_at", "last_error",
			"received", "success", "failed", "parallelism", "min_subtask", "max_subtask",
		}).AddRow(eventID, true, status, time.Now(), "", success+failed, success, failed, 2, 0, success-1))
}

func expectModelRuntimeReceipt(mock sqlmock.Sqlmock, eventID string, success, failed int) {
	mock.ExpectQuery("SELECT o.event_id").
		WithArgs("model-version-3", "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "published", "published_at", "last_error", "received",
			"success", "failed", "parallelism", "min_subtask", "max_subtask",
		}).AddRow(eventID, true, time.Now(), "", success+failed, success, failed, 2, 0, success-1))
}

func TestDeploymentRuntimeGatePartialModelAckStopsExpansion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectRuleRuntimeReceipt(mock, "11111111-1111-4111-8111-111111111111", "applied", 2, 0)
	expectModelRuntimeReceipt(mock, "22222222-2222-4222-8222-222222222222", 1, 0)

	gate, err := runtimeGateTestService(runtimeGateConfig()).discoverDeploymentRuntimeGate(
		context.Background(), db, runtimeGateDeployment(), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Ready || gate.ExpansionAllowed || gate.Status != "blocked" {
		t.Fatalf("partial ACK gate unexpectedly allows expansion: %+v", gate)
	}
	if gate.Rule == nil || gate.Rule.Status != "applied" || gate.Model == nil || gate.Model.Status != "partial" {
		t.Fatalf("unexpected component receipts: %+v", gate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRuntimeGateBindsApprovalToExactEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectRuleRuntimeReceipt(mock, "33333333-3333-4333-8333-333333333333", "applied", 2, 0)
	expectModelRuntimeReceipt(mock, "44444444-4444-4444-8444-444444444444", 2, 0)
	approved := &model.DeploymentRuntimeGate{
		Enabled: true, Status: "ready", Ready: true, ExpansionAllowed: true,
		Rule:  &model.DeploymentRuntimeReceipt{Component: "rule", ComponentID: "rule-version-7", EventID: "old-rule-event", ExpectedAcks: 2, Status: "applied"},
		Model: &model.DeploymentRuntimeReceipt{Component: "model", ComponentID: "model-version-3", EventID: "44444444-4444-4444-8444-444444444444", ExpectedAcks: 2, Status: "applied"},
	}
	_, err = runtimeGateTestService(runtimeGateConfig()).verifyApprovedDeploymentRuntimeGate(
		context.Background(), db, runtimeGateDeployment(),
		map[string]interface{}{deploymentRuntimeGateConfigurationKey: approved}, false,
	)
	if err == nil {
		t.Fatal("event drift after approval must fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRuntimeGateRequiresGrayProjectionBeforeFullActivation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ruleEvent := "55555555-5555-4555-8555-555555555555"
	modelEvent := "66666666-6666-4666-8666-666666666666"
	expectRuleRuntimeReceipt(mock, ruleEvent, "applied", 2, 0)
	expectModelRuntimeReceipt(mock, modelEvent, 2, 0)
	mock.ExpectQuery("SELECT o.event_id").
		WithArgs("tenant-a", "deployment-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "published", "projected_at", "kafka_partition", "kafka_offset", "last_error",
		}).AddRow("77777777-7777-4777-8777-777777777777", true, nil, nil, nil, ""))
	approved := &model.DeploymentRuntimeGate{
		Enabled: true, Status: "ready", Ready: true, ExpansionAllowed: true,
		Rule:  &model.DeploymentRuntimeReceipt{Component: "rule", ComponentID: "rule-version-7", EventID: ruleEvent, ExpectedAcks: 2, Status: "applied"},
		Model: &model.DeploymentRuntimeReceipt{Component: "model", ComponentID: "model-version-3", EventID: modelEvent, ExpectedAcks: 2, Status: "applied"},
	}
	_, err = runtimeGateTestService(runtimeGateConfig()).verifyApprovedDeploymentRuntimeGate(
		context.Background(), db, runtimeGateDeployment(),
		map[string]interface{}{deploymentRuntimeGateConfigurationKey: approved}, true,
	)
	if err == nil {
		t.Fatal("missing gray projection ACK must stop full activation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRuntimeGateDisabledPreservesCompatibility(t *testing.T) {
	config := DefaultDeploymentServiceConfig()
	gate, err := runtimeGateTestService(config).discoverDeploymentRuntimeGate(
		context.Background(), nil, runtimeGateDeployment(), true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Enabled || !gate.Ready || !gate.ExpansionAllowed || gate.Status != "disabled" {
		t.Fatalf("default-off compatibility gate is invalid: %+v", gate)
	}
}
