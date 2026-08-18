package attackchain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type temporalAuthorityFunc func(context.Context, SourceScope) (TemporalFactSnapshot, error)

func (function temporalAuthorityFunc) LoadTemporalFacts(ctx context.Context, scope SourceScope) (TemporalFactSnapshot, error) {
	return function(ctx, scope)
}

type graphAuthorityFunc func(context.Context, SourceScope) (GraphPathSnapshot, error)

func (function graphAuthorityFunc) LoadGraphPaths(ctx context.Context, scope SourceScope) (GraphPathSnapshot, error) {
	return function(ctx, scope)
}

type analystAuthorityFunc func(context.Context, SourceScope) (AnalystFactSnapshot, error)

func (function analystAuthorityFunc) LoadAnalystFacts(ctx context.Context, scope SourceScope) (AnalystFactSnapshot, error) {
	return function(ctx, scope)
}

type evidenceAuthorityFunc func(context.Context, string, time.Time, []EvidenceAnchor) (EvidenceVerificationBatch, error)

func (function evidenceAuthorityFunc) VerifyEvidence(
	ctx context.Context,
	tenantID string,
	asOf time.Time,
	anchors []EvidenceAnchor,
) (EvidenceVerificationBatch, error) {
	return function(ctx, tenantID, asOf, anchors)
}

type recordingSnapshotWriter struct{ snapshots []Snapshot }

func (writer *recordingSnapshotWriter) Save(_ context.Context, snapshot Snapshot) error {
	writer.snapshots = append(writer.snapshots, snapshot)
	return nil
}

func TestAssemblerServiceJoinsFourAuthoritiesAtOneSnapshot(t *testing.T) {
	request, temporal, graph, analyst := validSourceAssembly()
	writer := &recordingSnapshotWriter{}
	service := mustSourceAssemblerService(t, temporal, graph, analyst, func(_ context.Context, tenantID string, asOf time.Time, anchors []EvidenceAnchor) (EvidenceVerificationBatch, error) {
		items := make([]EvidenceVerification, 0, len(anchors))
		for _, anchor := range anchors {
			available := anchor.EvidenceID != "analyst-1"
			items = append(items, EvidenceVerification{EvidenceID: anchor.EvidenceID, SHA256: anchor.SHA256, Available: available})
		}
		return EvidenceVerificationBatch{TenantID: tenantID, AsOf: asOf, Watermark: "minio-version-set:7", Items: items}, nil
	}, writer)

	snapshot, err := service.AssembleAndSave(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(writer.snapshots) != 1 || writer.snapshots[0].SnapshotSHA256 != snapshot.SnapshotSHA256 {
		t.Fatalf("canonical snapshot was not persisted exactly once: %+v", writer.snapshots)
	}
	if !snapshot.Partial || !containsString(snapshot.PartialReasons, "path:alternative-1") {
		t.Fatalf("unavailable MinIO evidence was not exposed as partial: %+v", snapshot.PartialReasons)
	}
	if snapshot.GraphSnapshot.SourceWatermarks["clickhouse"] != "clickhouse:42" ||
		snapshot.GraphSnapshot.SourceWatermarks["nebulagraph"] != "nebula:43" ||
		snapshot.GraphSnapshot.SourceWatermarks["postgresql"] != "postgres:44" ||
		snapshot.GraphSnapshot.SourceWatermarks["minio"] != "minio-version-set:7" {
		t.Fatalf("four-store watermarks were not frozen: %+v", snapshot.GraphSnapshot.SourceWatermarks)
	}
	if snapshot.CandidatePath.Edges[0].Provenance != "observed" || snapshot.AlternativePaths[0].Edges[0].Provenance != "analyst" {
		t.Fatalf("source ownership was lost: %+v", snapshot)
	}
}

func TestAssemblerServiceRejectsMixedAsOfBeforeWriting(t *testing.T) {
	request, temporal, graph, analyst := validSourceAssembly()
	graph.AsOf = graph.AsOf.Add(time.Second)
	writer := &recordingSnapshotWriter{}
	service := mustSourceAssemblerService(t, temporal, graph, analyst, exactEvidenceVerifier, writer)
	if _, err := service.AssembleAndSave(context.Background(), request); !errors.Is(err, ErrInconsistentAssemblySource) {
		t.Fatalf("mixed source snapshot was not rejected: %v", err)
	}
	if len(writer.snapshots) != 0 {
		t.Fatal("mixed source snapshot reached the writer")
	}
}

func TestAssemblerServiceRejectsMissingAndExtraFactAuthorities(t *testing.T) {
	request, temporal, graph, analyst := validSourceAssembly()
	tests := map[string]func(*TemporalFactSnapshot){
		"missing": func(value *TemporalFactSnapshot) { value.Facts = value.Facts[:1] },
		"extra": func(value *TemporalFactSnapshot) {
			value.Facts = append(value.Facts, EdgeFact{EdgeID: strings.Repeat("f", 64), Provenance: "observed"})
		},
		"wrong_authority": func(value *TemporalFactSnapshot) { value.Facts[0].Provenance = "analyst" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := temporal
			candidate.Facts = append([]EdgeFact(nil), temporal.Facts...)
			mutate(&candidate)
			writer := &recordingSnapshotWriter{}
			service := mustSourceAssemblerService(t, candidate, graph, analyst, exactEvidenceVerifier, writer)
			if _, err := service.AssembleAndSave(context.Background(), request); !errors.Is(err, ErrInconsistentAssemblySource) {
				t.Fatalf("fact authority drift was not rejected: %v", err)
			}
			if len(writer.snapshots) != 0 {
				t.Fatal("invalid fact set reached the writer")
			}
		})
	}
}

func TestAssemblerServiceRejectsEvidenceSHAOrSetDrift(t *testing.T) {
	request, temporal, graph, analyst := validSourceAssembly()
	tests := map[string]evidenceAuthorityFunc{
		"sha": func(_ context.Context, tenantID string, asOf time.Time, anchors []EvidenceAnchor) (EvidenceVerificationBatch, error) {
			batch, _ := exactEvidenceVerifier(context.Background(), tenantID, asOf, anchors)
			batch.Items[0].SHA256 = strings.Repeat("b", 64)
			return batch, nil
		},
		"missing": func(_ context.Context, tenantID string, asOf time.Time, anchors []EvidenceAnchor) (EvidenceVerificationBatch, error) {
			batch, _ := exactEvidenceVerifier(context.Background(), tenantID, asOf, anchors)
			batch.Items = batch.Items[:len(batch.Items)-1]
			return batch, nil
		},
	}
	for name, verifier := range tests {
		t.Run(name, func(t *testing.T) {
			writer := &recordingSnapshotWriter{}
			service := mustSourceAssemblerService(t, temporal, graph, analyst, verifier, writer)
			if _, err := service.AssembleAndSave(context.Background(), request); !errors.Is(err, ErrInconsistentAssemblySource) {
				t.Fatalf("evidence drift was not rejected: %v", err)
			}
			if len(writer.snapshots) != 0 {
				t.Fatal("invalid evidence set reached the writer")
			}
		})
	}
}

func TestAssemblerServiceMakesSourceBudgetVisible(t *testing.T) {
	request, temporal, graph, analyst := validSourceAssembly()
	temporal.Truncated = true
	temporal.PartialReasons = []string{"late_partition_not_closed"}
	writer := &recordingSnapshotWriter{}
	service := mustSourceAssemblerService(t, temporal, graph, analyst, exactEvidenceVerifier, writer)
	snapshot, err := service.AssembleAndSave(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"clickhouse_fact_budget", "late_partition_not_closed"} {
		if !containsString(snapshot.PartialReasons, reason) {
			t.Fatalf("source limitation %q was hidden: %+v", reason, snapshot.PartialReasons)
		}
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("source partial reasons are not reproducible: %v", err)
	}
}

func mustSourceAssemblerService(
	t *testing.T,
	temporal TemporalFactSnapshot,
	graph GraphPathSnapshot,
	analyst AnalystFactSnapshot,
	verifier evidenceAuthorityFunc,
	writer SnapshotWriter,
) *AssemblerService {
	t.Helper()
	service, err := NewAssemblerService(
		temporalAuthorityFunc(func(context.Context, SourceScope) (TemporalFactSnapshot, error) { return temporal, nil }),
		graphAuthorityFunc(func(context.Context, SourceScope) (GraphPathSnapshot, error) { return graph, nil }),
		analystAuthorityFunc(func(context.Context, SourceScope) (AnalystFactSnapshot, error) { return analyst, nil }),
		verifier,
		writer,
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func exactEvidenceVerifier(
	_ context.Context,
	tenantID string,
	asOf time.Time,
	anchors []EvidenceAnchor,
) (EvidenceVerificationBatch, error) {
	items := make([]EvidenceVerification, 0, len(anchors))
	for _, anchor := range anchors {
		items = append(items, EvidenceVerification{EvidenceID: anchor.EvidenceID, SHA256: anchor.SHA256, Available: true})
	}
	return EvidenceVerificationBatch{TenantID: tenantID, AsOf: asOf, Watermark: "minio:45", Items: items}, nil
}

func validSourceAssembly() (AssemblyRequest, TemporalFactSnapshot, GraphPathSnapshot, AnalystFactSnapshot) {
	input := validAssembleInput()
	request := AssemblyRequest{
		SnapshotID: input.SnapshotID, GraphSnapshotID: input.GraphSnapshot.SnapshotID,
		TenantID: input.TenantID, ChainID: input.ChainID, Version: input.Version, AsOf: input.AsOf,
		WindowFrom: input.AsOf.Add(-time.Hour), Source: input.Source, Target: input.Target,
		Stages: input.Stages, MaxDepth: input.MaxDepth, MaxAlternatives: input.MaxAlternatives, MaxFacts: 100,
	}
	toSkeleton := func(path Path) PathSkeleton {
		edges := make([]PathEdge, 0, len(path.Edges))
		for _, edge := range path.Edges {
			edges = append(edges, PathEdge{EdgeID: edge.EdgeID, RelationType: edge.RelationType, Stage: edge.Stage, Source: edge.Source, Target: edge.Target})
		}
		return PathSkeleton{PathID: path.PathID, Kind: path.Kind, Edges: edges, Confidence: path.Confidence, Uncertainty: path.Uncertainty, ContradictsPathIDs: path.ContradictsPathIDs}
	}
	toFact := func(edge Edge) EdgeFact {
		return EdgeFact{EdgeID: edge.EdgeID, EventTime: edge.EventTime, Provenance: edge.Provenance, Confidence: edge.Confidence, Uncertainty: edge.Uncertainty, Evidence: edge.Evidence}
	}
	temporal := TemporalFactSnapshot{
		TenantID: input.TenantID, ChainID: input.ChainID, AsOf: input.AsOf, Watermark: "clickhouse:42",
		Facts: []EdgeFact{toFact(input.CandidatePath.Edges[0]), toFact(input.CandidatePath.Edges[1])},
	}
	graph := GraphPathSnapshot{
		TenantID: input.TenantID, ChainID: input.ChainID, AsOf: input.AsOf, Watermark: "nebula:43",
		SchemaVersion: "gnn-graph/v1", Nodes: input.GraphSnapshot.Nodes, EdgeIDs: input.GraphSnapshot.EdgeIDs,
		LabelRefs: input.GraphSnapshot.LabelRefs, CandidatePath: toSkeleton(input.CandidatePath),
		AlternativePaths: []PathSkeleton{toSkeleton(input.AlternativePaths[0])},
	}
	analyst := AnalystFactSnapshot{
		TenantID: input.TenantID, ChainID: input.ChainID, AsOf: input.AsOf, Watermark: "postgres:44",
		Facts: []EdgeFact{toFact(input.AlternativePaths[0].Edges[0])},
	}
	return request, temporal, graph, analyst
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
