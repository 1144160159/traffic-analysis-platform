package service

import (
	"strings"
	"testing"
)

func validModelShadowAck() ModelAppliedAck {
	sha := strings.Repeat("a", 64)
	return ModelAppliedAck{
		SchemaVersion: 1, AckType: "shadow_load", Status: "shadow_ready",
		EventID: "22222222-2222-4222-8222-222222222222", TenantID: "tenant-a",
		ModelID: "model-a", Version: "v2", ArtifactURI: "s3://models/p/manifest.json",
		ArtifactSHA256: sha, PackageID: "33333333-3333-5333-8333-333333333333",
		PackageSHA256: sha, AggregateRevision: 7, SubtaskIndex: 0, Parallelism: 4,
	}
}

func validModelShadowEvent() ModelUpdateEvent {
	sha := strings.Repeat("a", 64)
	return ModelUpdateEvent{
		SchemaVersion: 2, Action: "shadow-load", TenantID: "tenant-a", ModelID: "model-a",
		Version: "v2", ArtifactManifestURI: "s3://models/p/manifest.json",
		ArtifactManifestSHA256: sha, PackageID: "33333333-3333-5333-8333-333333333333",
		PackageSHA256: sha, EvaluationSHA256: sha, ExplanationSHA256: sha,
		GraphSnapshotID: "graph-7", GraphSnapshotSHA256: sha, AggregateRevision: 7,
	}
}

func TestValidateModelShadowAck(t *testing.T) {
	valid := validModelShadowAck()
	if err := validateModelShadowAck(valid, 4); err != nil {
		t.Fatalf("valid shadow acknowledgement rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ModelAppliedAck)
	}{
		{"partial-parallelism", func(value *ModelAppliedAck) { value.Parallelism = 3 }},
		{"old-revision", func(value *ModelAppliedAck) { value.AggregateRevision = 0 }},
		{"digest-mismatch", func(value *ModelAppliedAck) { value.ArtifactSHA256 = strings.Repeat("b", 64) }},
		{"unknown-status", func(value *ModelAppliedAck) { value.Status = "applied" }},
		{"failed-without-error", func(value *ModelAppliedAck) { value.Status = "failed" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := validateModelShadowAck(candidate, 4); err == nil {
				t.Fatal("invalid shadow acknowledgement was accepted")
			}
		})
	}
}

func TestValidateModelShadowEventContract(t *testing.T) {
	ack := validModelShadowAck()
	event := validModelShadowEvent()
	if err := validateModelShadowEventContract(ack, event); err != nil {
		t.Fatalf("valid shadow event contract rejected: %v", err)
	}

	wrongGraph := event
	wrongGraph.GraphSnapshotSHA256 = "short"
	if err := validateModelShadowEventContract(ack, wrongGraph); err == nil {
		t.Fatal("invalid graph identity was accepted")
	}
	wrongRevision := ack
	wrongRevision.AggregateRevision++
	if err := validateModelShadowEventContract(wrongRevision, event); err == nil {
		t.Fatal("mismatched aggregate revision was accepted")
	}
}
