package service

import (
	"strings"
	"testing"
)

func validModelConsumerReadyAck() ModelAppliedAck {
	profile := strings.Repeat("a", 64)
	return ModelAppliedAck{
		SchemaVersion: 1, EventID: "11111111-1111-4111-8111-111111111111",
		TenantID: "*", ModelID: "*", Version: "1.0.0",
		ArtifactURI: "consumer-profile://behavior-job-r1", ArtifactSHA256: profile,
		SubtaskIndex: 0, Parallelism: 4, Status: "consumer_ready", AckType: "consumer_ready",
		ConsumerDeploymentID: "behavior-job-r1", ConsumerProfileSHA256: profile,
		RuntimeContract: "traffic.behavior.inference.v1", RuntimeVersion: "1.0.0",
		FeatureSchemaVersion: 1, GraphSchemaVersion: 1,
		SupportedModelFormats: "onnx,numpy_npz_v1",
	}
}

func TestValidateModelConsumerReadyAck(t *testing.T) {
	valid := validModelConsumerReadyAck()
	config := DefaultModelServiceConfig()
	config.ModelConsumerDeploymentID = valid.ConsumerDeploymentID
	config.ModelConsumerProfileSHA256 = valid.ConsumerProfileSHA256
	if err := validateModelConsumerReadyAck(valid, config); err != nil {
		t.Fatalf("valid readiness rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*ModelAppliedAck)
	}{
		{"partial-parallelism", func(ack *ModelAppliedAck) { ack.Parallelism = 3 }},
		{"profile-mismatch", func(ack *ModelAppliedAck) { ack.ArtifactSHA256 = strings.Repeat("b", 64) }},
		{"graph-schema-zero", func(ack *ModelAppliedAck) { ack.GraphSchemaVersion = 0 }},
		{"gnn-format-missing", func(ack *ModelAppliedAck) { ack.SupportedModelFormats = "onnx" }},
		{"runtime-contract-drift", func(ack *ModelAppliedAck) { ack.RuntimeContract = "traffic.behavior.inference.v2" }},
		{"deployment-drift", func(ack *ModelAppliedAck) { ack.ConsumerDeploymentID = "behavior-job-r2" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := validateModelConsumerReadyAck(candidate, config); err == nil {
				t.Fatal("invalid readiness was accepted")
			}
		})
	}

	unconfigured := DefaultModelServiceConfig()
	if err := validateModelConsumerReadyAck(valid, unconfigured); err == nil {
		t.Fatal("readiness was accepted without a server-controlled profile")
	}
}
