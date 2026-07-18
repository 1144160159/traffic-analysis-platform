package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelAppliedAckContractRejectsConsumerControlledParallelism(t *testing.T) {
	payload, err := json.Marshal(ModelUpdateEvent{
		ArtifactURI:                "s3://traffic-models/acceptance/model.json",
		ExpectedAppliedParallelism: 4,
		Metrics: map[string]interface{}{
			"artifact_sha256": strings.Repeat("a", 64),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := parseModelAppliedContract(payload, 4)
	if err != nil {
		t.Fatal(err)
	}
	ack := ModelAppliedAck{
		Status:         "applied",
		ArtifactURI:    contract.ArtifactURI,
		ArtifactSHA256: contract.ArtifactSHA256,
		SubtaskIndex:   0,
		Parallelism:    1,
	}
	if err := validateModelAppliedAckContract(ack, contract, true); err == nil || !strings.Contains(err.Error(), "server contract 4") {
		t.Fatalf("expected server-controlled parallelism rejection, got %v", err)
	}
}

func TestModelAppliedAckContractRejectsArtifactMismatch(t *testing.T) {
	contract := modelAppliedContract{
		ArtifactURI:                "s3://traffic-models/acceptance/model.json",
		ArtifactSHA256:             strings.Repeat("a", 64),
		ExpectedAppliedParallelism: 4,
	}
	valid := ModelAppliedAck{
		Status:         "applied",
		ArtifactURI:    contract.ArtifactURI,
		ArtifactSHA256: contract.ArtifactSHA256,
		SubtaskIndex:   0,
		Parallelism:    4,
	}
	if err := validateModelAppliedAckContract(valid, contract, true); err != nil {
		t.Fatalf("valid acknowledgement rejected: %v", err)
	}

	wrongURI := valid
	wrongURI.ArtifactURI = "s3://traffic-models/acceptance/other.json"
	if err := validateModelAppliedAckContract(wrongURI, contract, true); err == nil || !strings.Contains(err.Error(), "artifact_uri") {
		t.Fatalf("expected artifact URI rejection, got %v", err)
	}

	wrongSHA := valid
	wrongSHA.ArtifactSHA256 = strings.Repeat("b", 64)
	if err := validateModelAppliedAckContract(wrongSHA, contract, true); err == nil || !strings.Contains(err.Error(), "artifact_sha256") {
		t.Fatalf("expected artifact SHA rejection, got %v", err)
	}
}

func TestModelAppliedAckContractFailsClosedWithoutActionFingerprint(t *testing.T) {
	contract := modelAppliedContract{
		ArtifactURI:                "s3://traffic-models/acceptance/model.json",
		ExpectedAppliedParallelism: 4,
	}
	ack := ModelAppliedAck{
		Status:         "applied",
		ArtifactURI:    contract.ArtifactURI,
		ArtifactSHA256: strings.Repeat("a", 64),
		Parallelism:    4,
	}
	if err := validateModelAppliedAckContract(ack, contract, true); err == nil || !strings.Contains(err.Error(), "missing artifact_sha256") {
		t.Fatalf("expected missing fingerprint rejection, got %v", err)
	}
}
