package attackchain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrInconsistentAssemblySource = errors.New("inconsistent attack-chain assembly source")

// AssemblyRequest fixes one tenant, one chain and one closed point-in-time
// snapshot across ClickHouse, NebulaGraph, PostgreSQL and the evidence store.
// Every source receives the same value; source adapters must not replace AsOf
// with their local wall clock.
type AssemblyRequest struct {
	SnapshotID      string
	GraphSnapshotID string
	TenantID        string
	ChainID         string
	Version         uint64
	AsOf            time.Time
	WindowFrom      time.Time
	Source          Identity
	Target          Identity
	Stages          []string
	MaxDepth        int
	MaxAlternatives int
	MaxFacts        int
}

type SourceScope struct {
	TenantID        string
	ChainID         string
	AsOf            time.Time
	WindowFrom      time.Time
	Source          Identity
	Target          Identity
	MaxDepth        int
	MaxAlternatives int
	MaxFacts        int
}

// EdgeFact is the time/provenance/evidence authority joined onto a graph path
// edge. ClickHouse owns observed/derived facts; PostgreSQL owns analyst facts.
type EdgeFact struct {
	EdgeID      string
	EventTime   int64
	Provenance  string
	Confidence  float64
	Uncertainty string
	Evidence    []EvidenceAnchor
}

type TemporalFactSnapshot struct {
	TenantID       string
	ChainID        string
	AsOf           time.Time
	Watermark      string
	Facts          []EdgeFact
	Truncated      bool
	PartialReasons []string
}

type AnalystFactSnapshot struct {
	TenantID       string
	ChainID        string
	AsOf           time.Time
	Watermark      string
	Facts          []EdgeFact
	Truncated      bool
	PartialReasons []string
}

type PathEdge struct {
	EdgeID       string
	RelationType string
	Stage        string
	Source       Identity
	Target       Identity
}

type PathSkeleton struct {
	PathID             string
	Kind               string
	Edges              []PathEdge
	Confidence         float64
	Uncertainty        string
	ContradictsPathIDs []string
}

type GraphPathSnapshot struct {
	TenantID             string
	ChainID              string
	AsOf                 time.Time
	Watermark            string
	SchemaVersion        string
	Nodes                []Identity
	EdgeIDs              []string
	LabelRefs            map[string]string
	CandidatePath        PathSkeleton
	AlternativePaths     []PathSkeleton
	HasMorePaths         bool
	ContinuationBoundary string
	PartialReasons       []string
}

type EvidenceVerification struct {
	EvidenceID string
	SHA256     string
	Available  bool
}

type EvidenceVerificationBatch struct {
	TenantID  string
	AsOf      time.Time
	Watermark string
	Items     []EvidenceVerification
}

type TemporalFactAuthority interface {
	LoadTemporalFacts(context.Context, SourceScope) (TemporalFactSnapshot, error)
}

type GraphPathAuthority interface {
	LoadGraphPaths(context.Context, SourceScope) (GraphPathSnapshot, error)
}

type AnalystConclusionAuthority interface {
	LoadAnalystFacts(context.Context, SourceScope) (AnalystFactSnapshot, error)
}

type EvidenceAuthority interface {
	VerifyEvidence(context.Context, string, time.Time, []EvidenceAnchor) (EvidenceVerificationBatch, error)
}

type SnapshotWriter interface {
	Save(context.Context, Snapshot) error
}

type AssemblerService struct {
	temporal TemporalFactAuthority
	graph    GraphPathAuthority
	analyst  AnalystConclusionAuthority
	evidence EvidenceAuthority
	writer   SnapshotWriter
}

func NewAssemblerService(
	temporal TemporalFactAuthority,
	graph GraphPathAuthority,
	analyst AnalystConclusionAuthority,
	evidence EvidenceAuthority,
	writer SnapshotWriter,
) (*AssemblerService, error) {
	if temporal == nil || graph == nil || analyst == nil || evidence == nil || writer == nil {
		return nil, fmt.Errorf("all attack-chain assembly authorities and the snapshot writer are required")
	}
	return &AssemblerService{temporal: temporal, graph: graph, analyst: analyst, evidence: evidence, writer: writer}, nil
}

// AssembleAndSave executes the only supported multi-store join. It never
// silently substitutes data from a different tenant, chain or point in time,
// and it persists only after the canonical snapshot and every evidence result
// have passed validation.
func (service *AssemblerService) AssembleAndSave(ctx context.Context, request AssemblyRequest) (Snapshot, error) {
	if err := validateAssemblyRequest(request); err != nil {
		return Snapshot{}, err
	}
	scope := SourceScope{
		TenantID: request.TenantID, ChainID: request.ChainID, AsOf: request.AsOf.UTC(),
		WindowFrom: request.WindowFrom.UTC(), Source: request.Source, Target: request.Target,
		MaxDepth: request.MaxDepth, MaxAlternatives: request.MaxAlternatives, MaxFacts: request.MaxFacts,
	}
	temporal, err := service.temporal.LoadTemporalFacts(ctx, scope)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load ClickHouse attack-chain facts: %w", err)
	}
	graph, err := service.graph.LoadGraphPaths(ctx, scope)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load NebulaGraph attack-chain paths: %w", err)
	}
	analyst, err := service.analyst.LoadAnalystFacts(ctx, scope)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load PostgreSQL analyst conclusions: %w", err)
	}
	if err := validateSourceEnvelopes(request, temporal, graph, analyst); err != nil {
		return Snapshot{}, err
	}

	facts, err := mergeAssemblyFacts(temporal.Facts, analyst.Facts)
	if err != nil {
		return Snapshot{}, err
	}
	candidate, usedFacts, err := materializePath(graph.CandidatePath, facts)
	if err != nil {
		return Snapshot{}, fmt.Errorf("candidate path source join: %w", err)
	}
	alternatives := make([]Path, 0, len(graph.AlternativePaths))
	for _, skeleton := range graph.AlternativePaths {
		path, used, pathErr := materializePath(skeleton, facts)
		if pathErr != nil {
			return Snapshot{}, fmt.Errorf("alternative path source join: %w", pathErr)
		}
		for edgeID := range used {
			usedFacts[edgeID] = struct{}{}
		}
		alternatives = append(alternatives, path)
	}
	if len(usedFacts) != len(facts) {
		return Snapshot{}, fmt.Errorf("%w: fact authority returned %d facts but graph paths used %d", ErrInconsistentAssemblySource, len(facts), len(usedFacts))
	}

	anchors, err := uniqueAssemblyEvidence(candidate, alternatives)
	if err != nil {
		return Snapshot{}, err
	}
	verification, err := service.evidence.VerifyEvidence(ctx, request.TenantID, request.AsOf.UTC(), anchors)
	if err != nil {
		return Snapshot{}, fmt.Errorf("verify immutable attack-chain evidence: %w", err)
	}
	if err := validateEvidenceVerification(request, anchors, verification); err != nil {
		return Snapshot{}, err
	}
	availability := make(map[string]bool, len(verification.Items))
	for _, item := range verification.Items {
		availability[item.EvidenceID] = item.Available
	}
	applyEvidenceAvailability(&candidate, availability)
	for index := range alternatives {
		applyEvidenceAvailability(&alternatives[index], availability)
	}

	edgeIDs := append([]string(nil), graph.EdgeIDs...)
	if len(edgeIDs) == 0 {
		edgeIDs = make([]string, 0, len(usedFacts))
		for edgeID := range usedFacts {
			edgeIDs = append(edgeIDs, edgeID)
		}
	}
	evidenceRefs := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		evidenceRefs = append(evidenceRefs, anchor.EvidenceID)
	}
	partialReasons := append([]string(nil), temporal.PartialReasons...)
	partialReasons = append(partialReasons, graph.PartialReasons...)
	partialReasons = append(partialReasons, analyst.PartialReasons...)
	if temporal.Truncated {
		partialReasons = append(partialReasons, "clickhouse_fact_budget")
	}
	if analyst.Truncated {
		partialReasons = append(partialReasons, "postgresql_analyst_budget")
	}
	snapshot, err := Assemble(AssembleInput{
		SnapshotID: request.SnapshotID, TenantID: request.TenantID, ChainID: request.ChainID,
		Version: request.Version, AsOf: request.AsOf.UTC(), Source: request.Source, Target: request.Target,
		Stages: request.Stages, CandidatePath: candidate, AlternativePaths: alternatives,
		GraphSnapshot: GraphSnapshot{
			SnapshotID: request.GraphSnapshotID, SchemaVersion: graph.SchemaVersion,
			Nodes: graph.Nodes, EdgeIDs: edgeIDs, LabelRefs: graph.LabelRefs, EvidenceRefs: evidenceRefs,
			SourceWatermarks: map[string]string{
				"clickhouse": temporal.Watermark, "nebulagraph": graph.Watermark,
				"postgresql": analyst.Watermark, "minio": verification.Watermark,
			},
		},
		MaxDepth: request.MaxDepth, MaxAlternatives: request.MaxAlternatives,
		HasMorePaths: graph.HasMorePaths, Continuation: graph.ContinuationBoundary,
		PartialReasons: partialReasons,
	})
	if err != nil {
		return Snapshot{}, err
	}
	if err := service.writer.Save(ctx, snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("persist immutable attack-chain snapshot: %w", err)
	}
	return snapshot, nil
}

func validateAssemblyRequest(request AssemblyRequest) error {
	if strings.TrimSpace(request.SnapshotID) == "" || strings.TrimSpace(request.GraphSnapshotID) == "" ||
		strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.ChainID) == "" ||
		request.Version == 0 || request.AsOf.IsZero() || request.WindowFrom.IsZero() ||
		!request.WindowFrom.Before(request.AsOf) || request.MaxDepth < 1 || request.MaxDepth > 32 ||
		request.MaxAlternatives < 0 || request.MaxAlternatives > 100 || request.MaxFacts < 1 || request.MaxFacts > 100000 {
		return fmt.Errorf("%w: invalid closed-window request or budget", ErrInconsistentAssemblySource)
	}
	if err := validateIdentity(request.TenantID, request.Source); err != nil {
		return err
	}
	if err := validateIdentity(request.TenantID, request.Target); err != nil {
		return err
	}
	return nil
}

func validateSourceEnvelopes(
	request AssemblyRequest,
	temporal TemporalFactSnapshot,
	graph GraphPathSnapshot,
	analyst AnalystFactSnapshot,
) error {
	values := []struct {
		name, tenantID, chainID, watermark string
		asOf                               time.Time
	}{
		{"clickhouse", temporal.TenantID, temporal.ChainID, temporal.Watermark, temporal.AsOf},
		{"nebulagraph", graph.TenantID, graph.ChainID, graph.Watermark, graph.AsOf},
		{"postgresql", analyst.TenantID, analyst.ChainID, analyst.Watermark, analyst.AsOf},
	}
	for _, value := range values {
		if value.tenantID != request.TenantID || value.chainID != request.ChainID ||
			!value.asOf.Equal(request.AsOf) || strings.TrimSpace(value.watermark) == "" {
			return fmt.Errorf("%w: %s returned a different tenant, chain, as_of or empty watermark", ErrInconsistentAssemblySource, value.name)
		}
	}
	if graph.SchemaVersion != "gnn-graph/v1" {
		return fmt.Errorf("%w: unsupported graph snapshot schema", ErrInconsistentAssemblySource)
	}
	if graph.CandidatePath.Kind != "candidate" {
		return fmt.Errorf("%w: graph source omitted the candidate path", ErrInconsistentAssemblySource)
	}
	return nil
}

func mergeAssemblyFacts(temporal, analyst []EdgeFact) (map[string]EdgeFact, error) {
	result := make(map[string]EdgeFact, len(temporal)+len(analyst))
	for _, group := range []struct {
		name       string
		facts      []EdgeFact
		provenance map[string]struct{}
	}{
		{"clickhouse", temporal, map[string]struct{}{"observed": {}, "derived": {}}},
		{"postgresql", analyst, map[string]struct{}{"analyst": {}}},
	} {
		for _, fact := range group.facts {
			if strings.TrimSpace(fact.EdgeID) == "" {
				return nil, fmt.Errorf("%w: %s returned an empty edge fact identity", ErrInconsistentAssemblySource, group.name)
			}
			if _, ok := group.provenance[fact.Provenance]; !ok {
				return nil, fmt.Errorf("%w: %s returned %s provenance", ErrInconsistentAssemblySource, group.name, fact.Provenance)
			}
			if _, exists := result[fact.EdgeID]; exists {
				return nil, fmt.Errorf("%w: edge fact %s has multiple authorities", ErrInconsistentAssemblySource, fact.EdgeID)
			}
			result[fact.EdgeID] = fact
		}
	}
	return result, nil
}

func materializePath(skeleton PathSkeleton, facts map[string]EdgeFact) (Path, map[string]struct{}, error) {
	path := Path{
		PathID: skeleton.PathID, Kind: skeleton.Kind, Confidence: skeleton.Confidence,
		Uncertainty: skeleton.Uncertainty, ContradictsPathIDs: append([]string(nil), skeleton.ContradictsPathIDs...),
		Edges: make([]Edge, 0, len(skeleton.Edges)),
	}
	used := make(map[string]struct{}, len(skeleton.Edges))
	for _, edge := range skeleton.Edges {
		fact, ok := facts[edge.EdgeID]
		if !ok {
			return Path{}, nil, fmt.Errorf("%w: graph edge %s has no time/provenance authority", ErrInconsistentAssemblySource, edge.EdgeID)
		}
		if _, duplicate := used[edge.EdgeID]; duplicate {
			return Path{}, nil, fmt.Errorf("%w: graph path repeats edge %s", ErrInconsistentAssemblySource, edge.EdgeID)
		}
		used[edge.EdgeID] = struct{}{}
		path.Edges = append(path.Edges, Edge{
			EdgeID: edge.EdgeID, RelationType: edge.RelationType, Stage: edge.Stage,
			Source: edge.Source, Target: edge.Target, EventTime: fact.EventTime,
			Provenance: fact.Provenance, Confidence: fact.Confidence, Uncertainty: fact.Uncertainty,
			Evidence: append([]EvidenceAnchor(nil), fact.Evidence...),
		})
	}
	return path, used, nil
}

func uniqueAssemblyEvidence(candidate Path, alternatives []Path) ([]EvidenceAnchor, error) {
	byID := make(map[string]EvidenceAnchor)
	paths := append([]Path{candidate}, alternatives...)
	for _, path := range paths {
		for _, edge := range path.Edges {
			for _, anchor := range edge.Evidence {
				if previous, exists := byID[anchor.EvidenceID]; exists && previous != anchor {
					return nil, fmt.Errorf("%w: evidence %s has conflicting source anchors", ErrInconsistentAssemblySource, anchor.EvidenceID)
				}
				byID[anchor.EvidenceID] = anchor
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]EvidenceAnchor, 0, len(ids))
	for _, id := range ids {
		result = append(result, byID[id])
	}
	return result, nil
}

func validateEvidenceVerification(
	request AssemblyRequest,
	anchors []EvidenceAnchor,
	batch EvidenceVerificationBatch,
) error {
	if batch.TenantID != request.TenantID || !batch.AsOf.Equal(request.AsOf) || strings.TrimSpace(batch.Watermark) == "" {
		return fmt.Errorf("%w: evidence authority returned a different tenant, as_of or empty watermark", ErrInconsistentAssemblySource)
	}
	expected := make(map[string]string, len(anchors))
	for _, anchor := range anchors {
		expected[anchor.EvidenceID] = anchor.SHA256
	}
	seen := make(map[string]struct{}, len(batch.Items))
	for _, item := range batch.Items {
		sha, exists := expected[item.EvidenceID]
		if !exists || item.SHA256 != sha {
			return fmt.Errorf("%w: evidence verification identity or SHA differs", ErrInconsistentAssemblySource)
		}
		if _, duplicate := seen[item.EvidenceID]; duplicate {
			return fmt.Errorf("%w: evidence verification is duplicated", ErrInconsistentAssemblySource)
		}
		seen[item.EvidenceID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: evidence verification is not an exact set", ErrInconsistentAssemblySource)
	}
	return nil
}

func applyEvidenceAvailability(path *Path, availability map[string]bool) {
	for edgeIndex := range path.Edges {
		for evidenceIndex := range path.Edges[edgeIndex].Evidence {
			anchor := &path.Edges[edgeIndex].Evidence[evidenceIndex]
			anchor.Available = availability[anchor.EvidenceID]
		}
	}
}
