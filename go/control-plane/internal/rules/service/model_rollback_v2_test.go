package service

import (
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestModelRollbackRequestHashBindsServingAndTargetIdentity(t *testing.T) {
	base := modelRollbackRequestHashInput{
		GovernanceVersion: modelRollbackGovernanceVersion,
		TenantID:          "tenant-a", ModelID: "11111111-1111-4111-8111-111111111111",
		FromModelVersion: "v2", FromRevision: 2, FromPackageSHA256: strings.Repeat("a", 64),
		TargetModelVersion: "v1", TargetRevision: 1, TargetPackageSHA256: strings.Repeat("b", 64),
		ConsumerDeploymentID: "behavior-job-r1", ConsumerProfileSHA256: strings.Repeat("c", 64),
		ExpectedParallelism: 4, Reason: "candidate error rate exceeded the stop threshold",
		RequestedBy: "22222222-2222-4222-8222-222222222222",
		ActionID:    "33333333-3333-4333-8333-333333333333",
	}
	first, err := hashModelRollbackRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashModelRollbackRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("rollback request hash is not deterministic: %q %q", first, second)
	}
	changed := base
	changed.FromRevision++
	third, err := hashModelRollbackRequest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("active revision drift was not bound into the rollback request hash")
	}
}

func TestGovernedModelArtifactSHA256FailsClosed(t *testing.T) {
	mv := &model.ModelVersion{Metrics: map[string]interface{}{
		"artifact_sha256": strings.Repeat("a", 64),
	}}
	if _, err := governedModelArtifactSHA256(mv); err != nil {
		t.Fatalf("valid artifact identity rejected: %v", err)
	}
	mv.Metrics["artifact_sha256"] = strings.Repeat("A", 64)
	if _, err := governedModelArtifactSHA256(mv); err == nil {
		t.Fatal("uppercase artifact identity must fail closed")
	}
}

func TestBuildModelRollbackEventSeparatesAttemptAndCompensation(t *testing.T) {
	config := DefaultModelServiceConfig()
	config.AppliedAckExpectedParallelism = 4
	config.ModelConsumerDeploymentID = "behavior-job-r1"
	config.ModelConsumerProfileSHA256 = strings.Repeat("c", 64)
	service := &ModelService{config: config}
	mv := &model.ModelVersion{
		ModelVersion: "v1", ModelID: "11111111-1111-4111-8111-111111111111",
		TenantID: "tenant-a", ArtifactURI: "s3://models/v1/model.onnx",
		Metrics: map[string]interface{}{"artifact_sha256": strings.Repeat("a", 64)},
	}
	attempt, err := service.buildModelRollbackEvent(
		mv, "22222222-2222-4222-8222-222222222222", modelRollbackPhaseAttempt,
		"v2", 2, "33333333-3333-4333-8333-333333333333",
	)
	if err != nil {
		t.Fatal(err)
	}
	compensation, err := service.buildModelRollbackEvent(
		mv, attempt.RollbackID, modelRollbackPhaseCompensation,
		"v2", 2, "44444444-4444-4444-8444-444444444444",
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Action != "rollback-activated" || compensation.Action != "rollback-compensate" {
		t.Fatalf("rollback phases share an ambiguous action: %q %q", attempt.Action, compensation.Action)
	}
	if attempt.SchemaVersion != 2 || compensation.SchemaVersion != 2 ||
		attempt.ConsumerProfileSHA256 != config.ModelConsumerProfileSHA256 {
		t.Fatal("rollback event omitted schema or frozen consumer identity")
	}
}
