package attackchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	graphprojection "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
)

var ErrInvalidSnapshot = errors.New("invalid attack-chain snapshot")

type Identity struct {
	TenantID    string `json:"tenant_id"`
	EntityType  string `json:"entity_type"`
	CanonicalID string `json:"canonical_id"`
	VertexID    string `json:"vertex_id"`
}

type EvidenceAnchor struct {
	TenantID      string `json:"tenant_id"`
	EvidenceID    string `json:"evidence_id"`
	Kind          string `json:"kind"`
	ImmutableURI  string `json:"immutable_uri"`
	SHA256        string `json:"sha256"`
	SourceEventID string `json:"source_event_id"`
	OccurredAt    int64  `json:"occurred_at"`
	Available     bool   `json:"available"`
}

type Edge struct {
	EdgeID       string           `json:"edge_id"`
	RelationType string           `json:"relation_type"`
	Stage        string           `json:"stage"`
	Source       Identity         `json:"source"`
	Target       Identity         `json:"target"`
	EventTime    int64            `json:"event_time"`
	Provenance   string           `json:"provenance"`
	Confidence   float64          `json:"confidence"`
	Uncertainty  string           `json:"uncertainty"`
	Evidence     []EvidenceAnchor `json:"evidence"`
}

type Path struct {
	PathID             string   `json:"path_id"`
	Kind               string   `json:"kind"`
	Edges              []Edge   `json:"edges"`
	Confidence         float64  `json:"confidence"`
	Uncertainty        string   `json:"uncertainty"`
	ContradictsPathIDs []string `json:"contradicts_path_ids"`
	Partial            bool     `json:"partial"`
	PartialReasons     []string `json:"partial_reasons"`
	PathSHA256         string   `json:"path_sha256"`
}

type GraphSnapshot struct {
	SnapshotID       string            `json:"snapshot_id"`
	SchemaVersion    string            `json:"schema_version"`
	Nodes            []Identity        `json:"nodes"`
	EdgeIDs          []string          `json:"edge_ids"`
	LabelRefs        map[string]string `json:"label_refs"`
	EvidenceRefs     []string          `json:"evidence_refs"`
	SourceWatermarks map[string]string `json:"source_watermarks"`
	NodeCount        int               `json:"node_count"`
	EdgeCount        int               `json:"edge_count"`
	NodeSHA256       string            `json:"node_sha256"`
	EdgeSHA256       string            `json:"edge_sha256"`
	SnapshotSHA256   string            `json:"snapshot_sha256"`
}

type Snapshot struct {
	SnapshotID           string        `json:"snapshot_id"`
	TenantID             string        `json:"tenant_id"`
	ChainID              string        `json:"chain_id"`
	Version              uint64        `json:"version"`
	AsOf                 time.Time     `json:"as_of"`
	Source               Identity      `json:"source"`
	Target               Identity      `json:"target"`
	Stages               []string      `json:"stages"`
	CandidatePath        Path          `json:"candidate_path"`
	AlternativePaths     []Path        `json:"alternative_paths"`
	GraphSnapshot        GraphSnapshot `json:"graph_snapshot"`
	Partial              bool          `json:"partial"`
	PartialReasons       []string      `json:"partial_reasons"`
	Truncated            bool          `json:"truncated"`
	TruncationReason     string        `json:"truncation_reason,omitempty"`
	ContinuationBoundary string        `json:"continuation_boundary,omitempty"`
	SnapshotSHA256       string        `json:"snapshot_sha256"`
}

type AssembleInput struct {
	SnapshotID       string
	TenantID         string
	ChainID          string
	Version          uint64
	AsOf             time.Time
	Source           Identity
	Target           Identity
	Stages           []string
	CandidatePath    Path
	AlternativePaths []Path
	GraphSnapshot    GraphSnapshot
	MaxDepth         int
	MaxAlternatives  int
	HasMorePaths     bool
	Continuation     string
	// PartialReasons records fail-visible source limitations which cannot be
	// inferred from an individual edge (for example, a closed ClickHouse
	// window that reached its fact budget). The assembler canonicalizes these
	// together with unavailable evidence and partial path reasons.
	PartialReasons []string
}

func Assemble(input AssembleInput) (Snapshot, error) {
	if strings.TrimSpace(input.SnapshotID) == "" || strings.TrimSpace(input.TenantID) == "" ||
		strings.TrimSpace(input.ChainID) == "" || input.Version == 0 || input.AsOf.IsZero() ||
		input.MaxDepth <= 0 || input.MaxDepth > 32 || input.MaxAlternatives < 0 || input.MaxAlternatives > 100 {
		return Snapshot{}, fmt.Errorf("%w: incomplete snapshot envelope or budget", ErrInvalidSnapshot)
	}
	if err := validateIdentity(input.TenantID, input.Source); err != nil {
		return Snapshot{}, err
	}
	if err := validateIdentity(input.TenantID, input.Target); err != nil {
		return Snapshot{}, err
	}
	stages, err := orderedUniqueStrings(input.Stages)
	if err != nil || len(stages) == 0 {
		return Snapshot{}, fmt.Errorf("%w: stages must be non-empty and unique", ErrInvalidSnapshot)
	}
	graph, err := canonicalGraphSnapshot(input.TenantID, input.AsOf, input.GraphSnapshot)
	if err != nil {
		return Snapshot{}, err
	}
	candidate, err := canonicalPath(input.TenantID, "candidate", input.Source, input.Target, input.CandidatePath, stages, input.MaxDepth)
	if err != nil {
		return Snapshot{}, fmt.Errorf("candidate path: %w", err)
	}
	if len(input.AlternativePaths) > input.MaxAlternatives+1 {
		return Snapshot{}, fmt.Errorf("%w: alternative query exceeded max+1 budget", ErrInvalidSnapshot)
	}
	paths := append([]Path(nil), input.AlternativePaths...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].PathID < paths[j].PathID })
	truncated := input.HasMorePaths || len(paths) > input.MaxAlternatives
	if len(paths) > input.MaxAlternatives {
		paths = paths[:input.MaxAlternatives]
	}
	alternatives := make([]Path, 0, len(paths))
	pathIDs := map[string]struct{}{candidate.PathID: {}}
	for _, path := range paths {
		value, pathErr := canonicalPath(input.TenantID, "alternative", input.Source, input.Target, path, stages, input.MaxDepth)
		if pathErr != nil {
			return Snapshot{}, fmt.Errorf("alternative path: %w", pathErr)
		}
		if _, exists := pathIDs[value.PathID]; exists {
			return Snapshot{}, fmt.Errorf("%w: duplicate path identity", ErrInvalidSnapshot)
		}
		pathIDs[value.PathID] = struct{}{}
		alternatives = append(alternatives, value)
	}
	allPaths := append([]Path{candidate}, alternatives...)
	for _, path := range allPaths {
		for _, contradiction := range path.ContradictsPathIDs {
			if _, exists := pathIDs[contradiction]; !exists || contradiction == path.PathID {
				return Snapshot{}, fmt.Errorf("%w: contradiction references an absent or identical path", ErrInvalidSnapshot)
			}
		}
	}
	if err := validatePathsBoundToGraph(allPaths, graph); err != nil {
		return Snapshot{}, err
	}
	partialReasons := append([]string(nil), input.PartialReasons...)
	for _, path := range allPaths {
		if path.Partial {
			partialReasons = append(partialReasons, "path:"+path.PathID)
		}
	}
	if graph.NodeCount == 0 || graph.EdgeCount == 0 {
		partialReasons = append(partialReasons, "graph_snapshot_empty")
	}
	partialReasons, err = canonicalSetStrings(partialReasons)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: invalid partial reason", ErrInvalidSnapshot)
	}
	snapshot := Snapshot{
		SnapshotID: input.SnapshotID, TenantID: input.TenantID, ChainID: input.ChainID,
		Version: input.Version, AsOf: input.AsOf.UTC(), Source: input.Source, Target: input.Target,
		Stages: stages, CandidatePath: candidate, AlternativePaths: alternatives, GraphSnapshot: graph,
		Partial: len(partialReasons) > 0, PartialReasons: partialReasons, Truncated: truncated,
	}
	if truncated {
		snapshot.TruncationReason = "path_budget"
		snapshot.ContinuationBoundary = strings.TrimSpace(input.Continuation)
		if snapshot.ContinuationBoundary == "" && len(alternatives) > 0 {
			snapshot.ContinuationBoundary = alternatives[len(alternatives)-1].PathID
		}
		if snapshot.ContinuationBoundary == "" {
			return Snapshot{}, fmt.Errorf("%w: truncated snapshot lacks continuation boundary", ErrInvalidSnapshot)
		}
	}
	snapshot.SnapshotSHA256, err = canonicalSHA(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// ValidateSnapshot reassembles a stored snapshot and rejects any payload or
// hash drift. Repository writes call this even when the caller used Assemble.
func ValidateSnapshot(snapshot Snapshot) error {
	rebuilt, err := Assemble(AssembleInput{
		SnapshotID: snapshot.SnapshotID, TenantID: snapshot.TenantID, ChainID: snapshot.ChainID,
		Version: snapshot.Version, AsOf: snapshot.AsOf, Source: snapshot.Source, Target: snapshot.Target,
		Stages: snapshot.Stages, CandidatePath: snapshot.CandidatePath,
		AlternativePaths: snapshot.AlternativePaths, GraphSnapshot: snapshot.GraphSnapshot,
		MaxDepth: 32, MaxAlternatives: len(snapshot.AlternativePaths),
		HasMorePaths: snapshot.Truncated, Continuation: snapshot.ContinuationBoundary,
		PartialReasons: snapshot.PartialReasons,
	})
	if err != nil {
		return err
	}
	if rebuilt.SnapshotSHA256 != snapshot.SnapshotSHA256 {
		return fmt.Errorf("%w: snapshot hash differs from canonical assembly", ErrInvalidSnapshot)
	}
	return nil
}

func canonicalPath(tenantID, expectedKind string, source, target Identity, path Path, stages []string, maxDepth int) (Path, error) {
	if strings.TrimSpace(path.PathID) == "" || path.Kind != expectedKind || len(path.Edges) == 0 || len(path.Edges) > maxDepth ||
		math.IsNaN(path.Confidence) || math.IsInf(path.Confidence, 0) || path.Confidence < 0 || path.Confidence > 1 {
		return Path{}, fmt.Errorf("%w: path envelope or depth", ErrInvalidSnapshot)
	}
	seenEdges, seenNodes := make(map[string]struct{}), map[string]struct{}{source.VertexID: {}}
	previous := source
	partialReasons := append([]string(nil), path.PartialReasons...)
	stageSet := make(map[string]struct{}, len(stages))
	for _, stage := range stages {
		stageSet[stage] = struct{}{}
	}
	for index := range path.Edges {
		edge := &path.Edges[index]
		if err := validateEdge(tenantID, *edge, stageSet); err != nil {
			return Path{}, err
		}
		if edge.Source.VertexID != previous.VertexID {
			return Path{}, fmt.Errorf("%w: path edge endpoints are not contiguous", ErrInvalidSnapshot)
		}
		if index > 0 && edge.EventTime < path.Edges[index-1].EventTime {
			return Path{}, fmt.Errorf("%w: path event time reversed", ErrInvalidSnapshot)
		}
		if _, exists := seenEdges[edge.EdgeID]; exists {
			return Path{}, fmt.Errorf("%w: path repeats an edge", ErrInvalidSnapshot)
		}
		if _, exists := seenNodes[edge.Target.VertexID]; exists {
			return Path{}, fmt.Errorf("%w: path contains a cycle", ErrInvalidSnapshot)
		}
		seenEdges[edge.EdgeID], seenNodes[edge.Target.VertexID] = struct{}{}, struct{}{}
		previous = edge.Target
		for _, evidence := range edge.Evidence {
			if !evidence.Available {
				partialReasons = append(partialReasons, "evidence_unavailable:"+evidence.EvidenceID)
			}
		}
	}
	if path.Edges[0].Source.VertexID != source.VertexID || previous.VertexID != target.VertexID {
		return Path{}, fmt.Errorf("%w: path source or target differs from chain", ErrInvalidSnapshot)
	}
	if expectedKind == "alternative" && strings.TrimSpace(path.Uncertainty) == "" {
		return Path{}, fmt.Errorf("%w: alternative path requires uncertainty", ErrInvalidSnapshot)
	}
	path.ContradictsPathIDs, _ = canonicalStrings(path.ContradictsPathIDs)
	path.PartialReasons, _ = canonicalStrings(partialReasons)
	path.Partial = len(path.PartialReasons) > 0
	path.PathSHA256 = ""
	var err error
	path.PathSHA256, err = canonicalSHA(path)
	return path, err
}

func validateEdge(tenantID string, edge Edge, stages map[string]struct{}) error {
	if !isHex(edge.EdgeID, 64) || strings.TrimSpace(edge.RelationType) == "" || edge.EventTime <= 0 || math.IsNaN(edge.Confidence) || math.IsInf(edge.Confidence, 0) || edge.Confidence < 0 || edge.Confidence > 1 || len(edge.Evidence) == 0 {
		return fmt.Errorf("%w: incomplete attack-chain edge", ErrInvalidSnapshot)
	}
	if _, exists := stages[edge.Stage]; !exists {
		return fmt.Errorf("%w: edge stage is undeclared", ErrInvalidSnapshot)
	}
	if err := validateIdentity(tenantID, edge.Source); err != nil {
		return err
	}
	if err := validateIdentity(tenantID, edge.Target); err != nil {
		return err
	}
	expectedEdgeID, err := graphprojection.EdgeID(tenantID, edge.RelationType, edge.Source.VertexID, edge.Target.VertexID)
	if err != nil || edge.EdgeID != expectedEdgeID {
		return fmt.Errorf("%w: edge identity is not deterministic", ErrInvalidSnapshot)
	}
	kinds := make(map[string]struct{})
	seen := make(map[string]struct{})
	for _, evidence := range edge.Evidence {
		if evidence.TenantID != tenantID || strings.TrimSpace(evidence.EvidenceID) == "" ||
			strings.TrimSpace(evidence.ImmutableURI) == "" || !isHex(evidence.SHA256, 64) ||
			strings.TrimSpace(evidence.SourceEventID) == "" || evidence.OccurredAt <= 0 {
			return fmt.Errorf("%w: incomplete or cross-tenant edge evidence", ErrInvalidSnapshot)
		}
		if evidence.OccurredAt > edge.EventTime {
			return fmt.Errorf("%w: evidence occurs after edge", ErrInvalidSnapshot)
		}
		switch evidence.Kind {
		case "event", "rule", "model", "analyst_conclusion":
		default:
			return fmt.Errorf("%w: unsupported edge evidence kind", ErrInvalidSnapshot)
		}
		if _, exists := seen[evidence.EvidenceID]; exists {
			return fmt.Errorf("%w: duplicate edge evidence", ErrInvalidSnapshot)
		}
		seen[evidence.EvidenceID], kinds[evidence.Kind] = struct{}{}, struct{}{}
	}
	switch edge.Provenance {
	case "observed":
		if len(kinds) != 1 {
			return fmt.Errorf("%w: observed edge carries non-event evidence", ErrInvalidSnapshot)
		}
		if _, ok := kinds["event"]; !ok {
			return fmt.Errorf("%w: observed edge lacks event evidence", ErrInvalidSnapshot)
		}
	case "derived":
		_, rule := kinds["rule"]
		_, model := kinds["model"]
		if (!rule && !model) || strings.TrimSpace(edge.Uncertainty) == "" {
			return fmt.Errorf("%w: derived edge lacks rule/model evidence or uncertainty", ErrInvalidSnapshot)
		}
		if _, analyst := kinds["analyst_conclusion"]; analyst {
			return fmt.Errorf("%w: derived edge carries analyst provenance", ErrInvalidSnapshot)
		}
	case "analyst":
		if _, ok := kinds["analyst_conclusion"]; !ok || strings.TrimSpace(edge.Uncertainty) == "" {
			return fmt.Errorf("%w: analyst edge lacks conclusion or uncertainty", ErrInvalidSnapshot)
		}
	default:
		return fmt.Errorf("%w: unsupported edge provenance", ErrInvalidSnapshot)
	}
	return nil
}

func canonicalGraphSnapshot(tenantID string, asOf time.Time, graph GraphSnapshot) (GraphSnapshot, error) {
	if strings.TrimSpace(graph.SnapshotID) == "" || graph.SchemaVersion != "gnn-graph/v1" || len(graph.SourceWatermarks) == 0 {
		return GraphSnapshot{}, fmt.Errorf("%w: incomplete graph snapshot", ErrInvalidSnapshot)
	}
	graph.Nodes = append([]Identity(nil), graph.Nodes...)
	seenNodes := make(map[string]struct{})
	for _, node := range graph.Nodes {
		if err := validateIdentity(tenantID, node); err != nil {
			return GraphSnapshot{}, err
		}
		if _, exists := seenNodes[node.VertexID]; exists {
			return GraphSnapshot{}, fmt.Errorf("%w: duplicate graph node", ErrInvalidSnapshot)
		}
		seenNodes[node.VertexID] = struct{}{}
	}
	if len(graph.LabelRefs) != len(seenNodes) {
		return GraphSnapshot{}, fmt.Errorf("%w: graph label references do not cover every node", ErrInvalidSnapshot)
	}
	for vertexID, labelRef := range graph.LabelRefs {
		if _, exists := seenNodes[vertexID]; !exists || strings.TrimSpace(labelRef) == "" {
			return GraphSnapshot{}, fmt.Errorf("%w: graph label reference has no node or value", ErrInvalidSnapshot)
		}
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].VertexID < graph.Nodes[j].VertexID })
	var err error
	graph.EdgeIDs, err = canonicalStrings(graph.EdgeIDs)
	if err != nil {
		return GraphSnapshot{}, fmt.Errorf("%w: duplicate graph edge", ErrInvalidSnapshot)
	}
	for _, edgeID := range graph.EdgeIDs {
		if !isHex(edgeID, 64) {
			return GraphSnapshot{}, fmt.Errorf("%w: invalid graph edge ID", ErrInvalidSnapshot)
		}
	}
	graph.EvidenceRefs, err = canonicalStrings(graph.EvidenceRefs)
	if err != nil || (len(graph.EdgeIDs) > 0 && len(graph.EvidenceRefs) == 0) {
		return GraphSnapshot{}, fmt.Errorf("%w: graph evidence references are empty or duplicated", ErrInvalidSnapshot)
	}
	graph.NodeCount, graph.EdgeCount = len(graph.Nodes), len(graph.EdgeIDs)
	graph.NodeSHA256, err = canonicalSHA(graph.Nodes)
	if err != nil {
		return GraphSnapshot{}, err
	}
	graph.EdgeSHA256, err = canonicalSHA(graph.EdgeIDs)
	if err != nil {
		return GraphSnapshot{}, err
	}
	graph.SnapshotSHA256, err = canonicalSHA(struct {
		TenantID string        `json:"tenant_id"`
		AsOf     time.Time     `json:"as_of"`
		Graph    GraphSnapshot `json:"graph"`
	}{tenantID, asOf.UTC(), GraphSnapshot{
		SnapshotID: graph.SnapshotID, SchemaVersion: graph.SchemaVersion, Nodes: graph.Nodes, EdgeIDs: graph.EdgeIDs,
		LabelRefs: graph.LabelRefs, EvidenceRefs: graph.EvidenceRefs,
		SourceWatermarks: graph.SourceWatermarks, NodeCount: graph.NodeCount, EdgeCount: graph.EdgeCount,
		NodeSHA256: graph.NodeSHA256, EdgeSHA256: graph.EdgeSHA256,
	}})
	return graph, err
}

func validatePathsBoundToGraph(paths []Path, graph GraphSnapshot) error {
	nodes := make(map[string]struct{}, len(graph.Nodes))
	edges := make(map[string]struct{}, len(graph.EdgeIDs))
	evidence := make(map[string]struct{}, len(graph.EvidenceRefs))
	evidenceAnchors := make(map[string]EvidenceAnchor, len(graph.EvidenceRefs))
	for _, node := range graph.Nodes {
		nodes[node.VertexID] = struct{}{}
	}
	for _, edgeID := range graph.EdgeIDs {
		edges[edgeID] = struct{}{}
	}
	for _, evidenceID := range graph.EvidenceRefs {
		evidence[evidenceID] = struct{}{}
	}
	for _, path := range paths {
		for _, edge := range path.Edges {
			if _, exists := edges[edge.EdgeID]; !exists {
				return fmt.Errorf("%w: path edge is absent from graph snapshot", ErrInvalidSnapshot)
			}
			if _, exists := nodes[edge.Source.VertexID]; !exists {
				return fmt.Errorf("%w: path source is absent from graph snapshot", ErrInvalidSnapshot)
			}
			if _, exists := nodes[edge.Target.VertexID]; !exists {
				return fmt.Errorf("%w: path target is absent from graph snapshot", ErrInvalidSnapshot)
			}
			for _, anchor := range edge.Evidence {
				if _, exists := evidence[anchor.EvidenceID]; !exists {
					return fmt.Errorf("%w: path evidence is absent from graph snapshot", ErrInvalidSnapshot)
				}
				if existing, exists := evidenceAnchors[anchor.EvidenceID]; exists && existing != anchor {
					return fmt.Errorf("%w: evidence identity has conflicting anchors", ErrInvalidSnapshot)
				}
				evidenceAnchors[anchor.EvidenceID] = anchor
			}
		}
	}
	return nil
}

func validateIdentity(tenantID string, identity Identity) error {
	if identity.TenantID != tenantID || strings.TrimSpace(identity.EntityType) == "" || strings.TrimSpace(identity.CanonicalID) == "" || !isHex(identity.VertexID, 32) {
		return fmt.Errorf("%w: incomplete or cross-tenant identity", ErrInvalidSnapshot)
	}
	expected, err := graphprojection.VertexID(tenantID, identity.EntityType, identity.CanonicalID)
	if err != nil || identity.VertexID != expected {
		return fmt.Errorf("%w: vertex identity is not deterministic", ErrInvalidSnapshot)
	}
	return nil
}

func canonicalStrings(values []string) ([]string, error) {
	result := make([]string, len(values))
	copy(result, values)
	for index := range result {
		result[index] = strings.TrimSpace(result[index])
		if result[index] == "" {
			return nil, fmt.Errorf("empty value")
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, fmt.Errorf("duplicate value")
		}
	}
	return result, nil
}

func orderedUniqueStrings(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("empty value")
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("duplicate value")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func canonicalSetStrings(values []string) ([]string, error) {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("empty value")
		}
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func canonicalSHA(value interface{}) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func isHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
