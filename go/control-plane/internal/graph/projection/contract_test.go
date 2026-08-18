package projection

import (
	"errors"
	"strings"
	"testing"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestTenantAwareDeterministicIdentities(t *testing.T) {
	a, err := VertexID("tenant-a", "asset", "asset-1")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := VertexID("tenant-a", "asset", "asset-1")
	b, _ := VertexID("tenant-b", "asset", "asset-1")
	if a != again || a == b || len(a) != 32 {
		t.Fatalf("unexpected tenant-aware VID values: %q %q %q", a, again, b)
	}
	edge, err := EdgeID("tenant-a", "communicates", a, b)
	if err != nil || len(edge) != 64 {
		t.Fatalf("unexpected EID %q: %v", edge, err)
	}
	rank, err := NebulaEdgeRank(edge)
	if err != nil || rank <= 0 {
		t.Fatalf("unexpected edge rank %d: %v", rank, err)
	}
}

func TestValidateObservedRelationProjection(t *testing.T) {
	event := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "event", "")
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("valid observed projection rejected: %v", err)
	}
}

func TestValidateRelationRejectsTenantIdentityAndHashDrift(t *testing.T) {
	event := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "event", "")
	event.GetRelation().SourceIdentity.TenantId = "tenant-b"
	if err := ValidateEvent(event); !errors.Is(err, ErrProjectionIdentity) {
		t.Fatalf("expected identity mismatch, got %v", err)
	}

	event = validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "event", "")
	event.GetRelation().ProjectionSha256 = strings.Repeat("f", 64)
	if err := ValidateEvent(event); !errors.Is(err, ErrProjectionHash) {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestValidateRelationKeepsProvenanceExplicit(t *testing.T) {
	derived := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_DERIVED, "rule", "rule confidence is below one")
	if err := ValidateEvent(derived); err != nil {
		t.Fatalf("valid derived relation rejected: %v", err)
	}

	spoofedObserved := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "rule", "")
	if err := ValidateEvent(spoofedObserved); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("derived evidence serialized as observed was not rejected: %v", err)
	}

	missingUncertainty := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_DERIVED, "model", "model has uncertainty")
	missingUncertainty.GetRelation().Uncertainty = ""
	hash, err := RelationProjectionSHA256(missingUncertainty.GetRelation())
	if err != nil {
		t.Fatal(err)
	}
	missingUncertainty.GetRelation().ProjectionSha256 = hash
	if err := ValidateEvent(missingUncertainty); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("derived relation without uncertainty was not rejected: %v", err)
	}
}

func TestValidateRelationRejectsEvidenceOutsideValidity(t *testing.T) {
	event := validRelationEvent(t, trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED, "event", "")
	event.GetRelation().Evidence[0].OccurredAt = 999
	hash, err := RelationProjectionSHA256(event.GetRelation())
	if err != nil {
		t.Fatal(err)
	}
	event.GetRelation().ProjectionSha256 = hash
	if err := ValidateEvent(event); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("out-of-window evidence was not rejected: %v", err)
	}
}

func validRelationEvent(
	t *testing.T,
	provenance trafficv1.GraphProvenanceKind,
	evidenceKind string,
	uncertainty string,
) *trafficv1.GraphProjectionEvent {
	t.Helper()
	sourceVID, err := VertexID("tenant-a", "asset", "asset-1")
	if err != nil {
		t.Fatal(err)
	}
	targetVID, err := VertexID("tenant-a", "ip", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	edgeID, err := EdgeID("tenant-a", "communicates", sourceVID, targetVID)
	if err != nil {
		t.Fatal(err)
	}
	relation := &trafficv1.GraphProjectedRelation{
		TenantId:     "tenant-a",
		EdgeId:       edgeID,
		RelationType: "communicates",
		SourceIdentity: &trafficv1.GraphEntityIdentity{
			TenantId: "tenant-a", EntityType: "asset", CanonicalId: "asset-1", VertexId: sourceVID,
		},
		TargetIdentity: &trafficv1.GraphEntityIdentity{
			TenantId: "tenant-a", EntityType: "ip", CanonicalId: "192.0.2.10", VertexId: targetVID,
		},
		ProvenanceKind: provenance,
		Confidence:     0.95,
		Uncertainty:    uncertainty,
		Evidence: []*trafficv1.GraphEvidenceAnchor{{
			EvidenceId: "evidence-1", EvidenceKind: evidenceKind,
			ImmutableUri: "minio://evidence/tenant-a/evidence-1.json", Sha256: strings.Repeat("a", 64),
			SourceEventId: "source-event-1", OccurredAt: 1500,
		}},
		ValidFrom: 1000,
		ValidTo:   2000,
		Source: &trafficv1.GraphProjectionSource{
			SourceSystem: "alert-service", SourceEventId: "source-event-1",
			AggregateType: "alert", AggregateId: "alert-1", AggregateVersion: 7,
			SourceSha256: strings.Repeat("b", 64), OccurredAt: 1500,
		},
	}
	hash, err := RelationProjectionSHA256(relation)
	if err != nil {
		t.Fatal(err)
	}
	relation.ProjectionSha256 = hash
	return &trafficv1.GraphProjectionEvent{
		Header: &trafficv1.EventHeader{
			EventId: "projection-event-1", TenantId: "tenant-a", EventType: RelationUpsertedEventType,
			SchemaVersion: SchemaVersion, AggregateType: "alert", AggregateId: "alert-1",
			AggregateVersion: 7, OccurredAt: 1500, ProducedAt: 1600, CausationId: "source-event-1",
			TraceId: "0123456789abcdef0123456789abcdef",
		},
		PartitionKey: "tenant-a:" + edgeID,
		Projection:   &trafficv1.GraphProjectionEvent_Relation{Relation: relation},
	}
}
