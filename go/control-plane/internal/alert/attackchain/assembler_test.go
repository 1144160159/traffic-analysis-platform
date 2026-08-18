package attackchain

import (
	"errors"
	"strings"
	"testing"
	"time"

	graphprojection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
)

func TestAssembleVersionedAttackChainRetainsContradictoryAlternatives(t *testing.T) {
	input := validAssembleInput()
	snapshot, err := Assemble(input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Partial || snapshot.Truncated || len(snapshot.AlternativePaths) != 1 ||
		len(snapshot.SnapshotSHA256) != 64 || snapshot.GraphSnapshot.NodeCount != 3 || snapshot.GraphSnapshot.EdgeCount != 3 {
		t.Fatalf("unexpected assembled snapshot: %+v", snapshot)
	}
	if snapshot.Stages[0] != "initial_access" || snapshot.CandidatePath.ContradictsPathIDs[0] != "alternative-1" {
		t.Fatalf("ordered stages or contradiction was lost: %+v", snapshot)
	}
	again, err := Assemble(input)
	if err != nil || again.SnapshotSHA256 != snapshot.SnapshotSHA256 {
		t.Fatalf("snapshot assembly is not deterministic: %v %s %s", err, snapshot.SnapshotSHA256, again.SnapshotSHA256)
	}
}

func TestAssembleAttackChainMakesUnavailableEvidencePartial(t *testing.T) {
	input := validAssembleInput()
	input.CandidatePath.Edges[0].Evidence[0].Available = false
	snapshot, err := Assemble(input)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Partial || !snapshot.CandidatePath.Partial || len(snapshot.PartialReasons) == 0 {
		t.Fatalf("unavailable evidence was hidden: %+v", snapshot)
	}
}

func TestAssembleAttackChainRejectsOverDepthCycleReverseTimeAndEvidenceFree(t *testing.T) {
	tests := map[string]func(*AssembleInput){
		"over_depth": func(input *AssembleInput) { input.MaxDepth = 1 },
		"cycle":      func(input *AssembleInput) { input.CandidatePath.Edges[1].Target = input.Source },
		"reverse_time": func(input *AssembleInput) {
			input.CandidatePath.Edges[1].EventTime = input.CandidatePath.Edges[0].EventTime - 1
		},
		"evidence_free": func(input *AssembleInput) { input.CandidatePath.Edges[0].Evidence = nil },
		"derived_as_observed": func(input *AssembleInput) {
			input.CandidatePath.Edges[0].Provenance = "observed"
			input.CandidatePath.Edges[0].Evidence[0].Kind = "rule"
		},
		"absent_contradiction": func(input *AssembleInput) { input.AlternativePaths[0].ContradictsPathIDs = []string{"omitted-path"} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validAssembleInput()
			mutate(&input)
			if _, err := Assemble(input); !errors.Is(err, ErrInvalidSnapshot) {
				t.Fatalf("invalid attack chain was not rejected: %v", err)
			}
		})
	}
}

func TestAssembleAttackChainExposesAlternativeTruncation(t *testing.T) {
	input := validAssembleInput()
	second := input.AlternativePaths[0]
	second.PathID = "alternative-2"
	second.ContradictsPathIDs = nil
	input.CandidatePath.ContradictsPathIDs = nil
	input.AlternativePaths[0].ContradictsPathIDs = nil
	input.AlternativePaths = append(input.AlternativePaths, second)
	input.MaxAlternatives = 1
	input.HasMorePaths = true
	input.Continuation = "cursor-after-alternative-1"
	snapshot, err := Assemble(input)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Truncated || snapshot.TruncationReason != "path_budget" ||
		snapshot.ContinuationBoundary != input.Continuation || len(snapshot.AlternativePaths) != 1 {
		t.Fatalf("path truncation was hidden: %+v", snapshot)
	}
}

func validAssembleInput() AssembleInput {
	identity := func(entityType, canonicalID string) Identity {
		vertexID, _ := graphprojection.VertexID("tenant-a", entityType, canonicalID)
		return Identity{TenantID: "tenant-a", EntityType: entityType, CanonicalID: canonicalID, VertexID: vertexID}
	}
	source := identity("asset", "source")
	middle := identity("ip", "middle")
	target := identity("asset", "target")
	edgeID := func(source, target Identity) string {
		value, _ := graphprojection.EdgeID("tenant-a", "attack_transition", source.VertexID, target.VertexID)
		return value
	}
	evidence := func(id, kind string, occurred int64) EvidenceAnchor {
		return EvidenceAnchor{
			TenantID: "tenant-a", EvidenceID: id, Kind: kind,
			ImmutableURI: "minio://evidence/tenant-a/" + id + ".json", SHA256: strings.Repeat("a", 64),
			SourceEventID: "event-" + id, OccurredAt: occurred, Available: true,
		}
	}
	candidate := Path{
		PathID: "candidate-1", Kind: "candidate", Confidence: 0.92,
		ContradictsPathIDs: []string{"alternative-1"},
		Edges: []Edge{
			{EdgeID: edgeID(source, middle), RelationType: "attack_transition", Stage: "initial_access", Source: source, Target: middle, EventTime: 1700000001000, Provenance: "observed", Confidence: 0.99, Evidence: []EvidenceAnchor{evidence("observed-1", "event", 1700000000000)}},
			{EdgeID: edgeID(middle, target), RelationType: "attack_transition", Stage: "execution", Source: middle, Target: target, EventTime: 1700000002000, Provenance: "derived", Confidence: 0.85, Uncertainty: "rule correlation may have false positives", Evidence: []EvidenceAnchor{evidence("rule-1", "rule", 1700000001000)}},
		},
	}
	alternative := Path{
		PathID: "alternative-1", Kind: "alternative", Confidence: 0.55,
		Uncertainty:        "direct route is also consistent with the observed window",
		ContradictsPathIDs: []string{"candidate-1"},
		Edges: []Edge{{
			EdgeID: edgeID(source, target), RelationType: "attack_transition", Stage: "initial_access", Source: source, Target: target,
			EventTime: 1700000001500, Provenance: "analyst", Confidence: 0.55,
			Uncertainty: "analyst conclusion is provisional", Evidence: []EvidenceAnchor{evidence("analyst-1", "analyst_conclusion", 1700000001400)},
		}},
	}
	return AssembleInput{
		SnapshotID: "snapshot-1", TenantID: "tenant-a", ChainID: "chain-1", Version: 1,
		AsOf: time.Date(2023, 11, 14, 22, 14, 0, 0, time.UTC), Source: source, Target: target,
		Stages: []string{"initial_access", "execution"}, CandidatePath: candidate,
		AlternativePaths: []Path{alternative}, MaxDepth: 5, MaxAlternatives: 5,
		GraphSnapshot: GraphSnapshot{
			SnapshotID: "graph-snapshot-1", SchemaVersion: "gnn-graph/v1", Nodes: []Identity{target, source, middle},
			EdgeIDs: []string{edgeID(source, target), edgeID(source, middle), edgeID(middle, target)},
			LabelRefs: map[string]string{
				source.VertexID: "label:source:v1", middle.VertexID: "label:transit:v1", target.VertexID: "label:target:v1",
			},
			EvidenceRefs:     []string{"observed-1", "rule-1", "analyst-1"},
			SourceWatermarks: map[string]string{"clickhouse": "1700000002000:2", "nebulagraph": "partition-0:42", "postgresql": "analyst-revision:7"},
		},
	}
}
