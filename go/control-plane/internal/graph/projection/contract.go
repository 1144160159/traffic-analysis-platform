package projection

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	SchemaVersion             = "1"
	EntityUpsertedEventType   = "graph.entity.upserted.v1"
	RelationUpsertedEventType = "graph.relation.upserted.v1"
	RelationRevokedEventType  = "graph.relation.revoked.v1"
)

var (
	ErrInvalidProjection  = errors.New("invalid graph projection")
	ErrProjectionIdentity = errors.New("graph projection identity mismatch")
	ErrProjectionHash     = errors.New("graph projection hash mismatch")
)

// VertexID returns the 32-character tenant-aware VID used by the traffic_graph
// FIXED_STRING(32) space. Length-prefixing prevents tuple ambiguity.
func VertexID(tenantID, entityType, canonicalID string) (string, error) {
	parts := []string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(entityType),
		strings.TrimSpace(canonicalID),
	}
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("%w: vertex identity fields are required", ErrInvalidProjection)
		}
	}
	sum := hashTuple("graph-vertex/v1", parts...)
	return hex.EncodeToString(sum[:16]), nil
}

// EdgeID is the full deterministic relation identity. NebulaEdgeRank derives
// the stable signed-positive rank used together with source and target VIDs.
func EdgeID(tenantID, relationType, sourceVID, targetVID string) (string, error) {
	parts := []string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(relationType),
		strings.TrimSpace(sourceVID),
		strings.TrimSpace(targetVID),
	}
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("%w: relation identity fields are required", ErrInvalidProjection)
		}
	}
	sum := hashTuple("graph-edge/v1", parts...)
	return hex.EncodeToString(sum[:]), nil
}

func NebulaEdgeRank(edgeID string) (int64, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(edgeID))
	if err != nil || len(raw) != sha256.Size {
		return 0, fmt.Errorf("%w: edge ID must be 64 lowercase hexadecimal characters", ErrProjectionIdentity)
	}
	rank := int64(binary.BigEndian.Uint64(raw[:8]) & math.MaxInt64)
	if rank == 0 {
		rank = 1
	}
	return rank, nil
}

func hashTuple(domain string, values ...string) [sha256.Size]byte {
	h := sha256.New()
	writeLengthPrefixed(h, domain)
	for _, value := range values {
		writeLengthPrefixed(h, value)
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeLengthPrefixed(w byteWriter, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = w.Write(length[:])
	_, _ = w.Write([]byte(value))
}

// ValidateEvent enforces tenant, identity, ordering, provenance and integrity
// invariants before an event may enter the durable projector inbox.
func ValidateEvent(event *trafficv1.GraphProjectionEvent) error {
	if event == nil || event.GetHeader() == nil {
		return fmt.Errorf("%w: event header is required", ErrInvalidProjection)
	}
	header := event.GetHeader()
	if strings.TrimSpace(header.GetEventId()) == "" ||
		strings.TrimSpace(header.GetTenantId()) == "" ||
		strings.TrimSpace(header.GetAggregateType()) == "" ||
		strings.TrimSpace(header.GetAggregateId()) == "" ||
		!isTraceID(header.GetTraceId()) ||
		header.GetSchemaVersion() != SchemaVersion ||
		header.GetAggregateVersion() == 0 ||
		header.GetOccurredAt() <= 0 ||
		header.GetProducedAt() <= 0 ||
		header.GetProducedAt() < header.GetOccurredAt() {
		return fmt.Errorf("%w: incomplete event envelope", ErrInvalidProjection)
	}
	if event.GetEntity() != nil {
		return validateEntityEvent(event)
	}
	if event.GetRelation() != nil {
		return validateRelationEvent(event)
	}
	return fmt.Errorf("%w: exactly one projection payload is required", ErrInvalidProjection)
}

func validateEntityEvent(event *trafficv1.GraphProjectionEvent) error {
	header, entity := event.GetHeader(), event.GetEntity()
	if header.GetEventType() != EntityUpsertedEventType || entity.GetRevoked() {
		return fmt.Errorf("%w: unsupported entity event type or revocation", ErrInvalidProjection)
	}
	identity := entity.GetIdentity()
	if err := validateIdentity(header.GetTenantId(), identity); err != nil {
		return err
	}
	if err := validateSource(header, entity.GetSource()); err != nil {
		return err
	}
	if err := validateValidity(entity.GetValidFrom(), entity.GetValidTo()); err != nil {
		return err
	}
	if event.GetPartitionKey() != header.GetTenantId()+":"+identity.GetVertexId() {
		return fmt.Errorf("%w: entity partition key is not canonical", ErrProjectionIdentity)
	}
	expected, err := EntityProjectionSHA256(entity)
	if err != nil {
		return err
	}
	if entity.GetProjectionSha256() != expected {
		return fmt.Errorf("%w: entity projection hash differs", ErrProjectionHash)
	}
	return nil
}

func validateRelationEvent(event *trafficv1.GraphProjectionEvent) error {
	header, relation := event.GetHeader(), event.GetRelation()
	expectedType := RelationUpsertedEventType
	if relation.GetRevoked() {
		expectedType = RelationRevokedEventType
	}
	if header.GetEventType() != expectedType || relation.GetTenantId() != header.GetTenantId() {
		return fmt.Errorf("%w: relation event type or tenant differs", ErrInvalidProjection)
	}
	if err := validateIdentity(header.GetTenantId(), relation.GetSourceIdentity()); err != nil {
		return err
	}
	if err := validateIdentity(header.GetTenantId(), relation.GetTargetIdentity()); err != nil {
		return err
	}
	expectedEdgeID, err := EdgeID(
		header.GetTenantId(), relation.GetRelationType(),
		relation.GetSourceIdentity().GetVertexId(), relation.GetTargetIdentity().GetVertexId())
	if err != nil {
		return err
	}
	if relation.GetEdgeId() != expectedEdgeID {
		return fmt.Errorf("%w: relation edge ID is not canonical", ErrProjectionIdentity)
	}
	if event.GetPartitionKey() != header.GetTenantId()+":"+expectedEdgeID {
		return fmt.Errorf("%w: relation partition key is not canonical", ErrProjectionIdentity)
	}
	if strings.TrimSpace(relation.GetRelationType()) == "" ||
		relation.GetProvenanceKind() == trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_UNSPECIFIED ||
		math.IsNaN(relation.GetConfidence()) || math.IsInf(relation.GetConfidence(), 0) ||
		relation.GetConfidence() < 0 || relation.GetConfidence() > 1 ||
		len(relation.GetEvidence()) == 0 {
		return fmt.Errorf("%w: relation provenance, confidence and evidence are required", ErrInvalidProjection)
	}
	evidenceKinds := make(map[string]struct{}, len(relation.GetEvidence()))
	evidenceIDs := make(map[string]struct{}, len(relation.GetEvidence()))
	for _, evidence := range relation.GetEvidence() {
		if err := validateEvidence(header.GetTenantId(), evidence); err != nil {
			return err
		}
		if _, exists := evidenceIDs[evidence.GetEvidenceId()]; exists {
			return fmt.Errorf("%w: duplicate relation evidence identity", ErrInvalidProjection)
		}
		evidenceIDs[evidence.GetEvidenceId()] = struct{}{}
		evidenceKinds[evidence.GetEvidenceKind()] = struct{}{}
		if evidence.GetOccurredAt() < relation.GetValidFrom() ||
			(relation.GetValidTo() != 0 && evidence.GetOccurredAt() > relation.GetValidTo()) {
			return fmt.Errorf("%w: evidence falls outside relation validity", ErrInvalidProjection)
		}
	}
	if err := validateProvenanceEvidence(relation.GetProvenanceKind(), evidenceKinds); err != nil {
		return err
	}
	if relation.GetProvenanceKind() != trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED &&
		strings.TrimSpace(relation.GetUncertainty()) == "" {
		return fmt.Errorf("%w: derived and analyst relations require explicit uncertainty", ErrInvalidProjection)
	}
	if err := validateSource(header, relation.GetSource()); err != nil {
		return err
	}
	if err := validateValidity(relation.GetValidFrom(), relation.GetValidTo()); err != nil {
		return err
	}
	expected, err := RelationProjectionSHA256(relation)
	if err != nil {
		return err
	}
	if relation.GetProjectionSha256() != expected {
		return fmt.Errorf("%w: relation projection hash differs", ErrProjectionHash)
	}
	return nil
}

func validateProvenanceEvidence(kind trafficv1.GraphProvenanceKind, evidenceKinds map[string]struct{}) error {
	switch kind {
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED:
		if len(evidenceKinds) != 1 {
			return fmt.Errorf("%w: observed relation may only contain event evidence", ErrInvalidProjection)
		}
		if _, ok := evidenceKinds["event"]; !ok {
			return fmt.Errorf("%w: observed relation requires event evidence", ErrInvalidProjection)
		}
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_DERIVED:
		if _, rule := evidenceKinds["rule"]; !rule {
			if _, model := evidenceKinds["model"]; !model {
				return fmt.Errorf("%w: derived relation requires rule or model evidence", ErrInvalidProjection)
			}
		}
		if _, analyst := evidenceKinds["analyst_conclusion"]; analyst {
			return fmt.Errorf("%w: derived relation cannot carry analyst provenance", ErrInvalidProjection)
		}
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_ANALYST:
		if _, ok := evidenceKinds["analyst_conclusion"]; !ok {
			return fmt.Errorf("%w: analyst relation requires analyst conclusion evidence", ErrInvalidProjection)
		}
	default:
		return fmt.Errorf("%w: unsupported relation provenance", ErrInvalidProjection)
	}
	return nil
}

func validateIdentity(tenantID string, identity *trafficv1.GraphEntityIdentity) error {
	if identity == nil || identity.GetTenantId() != tenantID {
		return fmt.Errorf("%w: entity tenant differs from envelope", ErrProjectionIdentity)
	}
	expected, err := VertexID(tenantID, identity.GetEntityType(), identity.GetCanonicalId())
	if err != nil {
		return err
	}
	if identity.GetVertexId() != expected {
		return fmt.Errorf("%w: vertex ID is not canonical", ErrProjectionIdentity)
	}
	return nil
}

func validateSource(header *trafficv1.EventHeader, source *trafficv1.GraphProjectionSource) error {
	if source == nil || strings.TrimSpace(source.GetSourceSystem()) == "" ||
		strings.TrimSpace(source.GetSourceEventId()) == "" ||
		strings.TrimSpace(source.GetAggregateType()) == "" ||
		strings.TrimSpace(source.GetAggregateId()) == "" ||
		source.GetAggregateType() != header.GetAggregateType() ||
		source.GetAggregateId() != header.GetAggregateId() ||
		source.GetAggregateVersion() != header.GetAggregateVersion() ||
		source.GetOccurredAt() != header.GetOccurredAt() ||
		!isSHA256(source.GetSourceSha256()) {
		return fmt.Errorf("%w: source identity or ordering differs from envelope", ErrInvalidProjection)
	}
	if header.GetCausationId() != "" && header.GetCausationId() != source.GetSourceEventId() {
		return fmt.Errorf("%w: source event differs from causation ID", ErrInvalidProjection)
	}
	return nil
}

func validateEvidence(tenantID string, evidence *trafficv1.GraphEvidenceAnchor) error {
	if evidence == nil || strings.TrimSpace(evidence.GetEvidenceId()) == "" ||
		strings.TrimSpace(evidence.GetEvidenceKind()) == "" ||
		strings.TrimSpace(evidence.GetImmutableUri()) == "" ||
		strings.TrimSpace(evidence.GetSourceEventId()) == "" ||
		evidence.GetOccurredAt() <= 0 || !isSHA256(evidence.GetSha256()) {
		return fmt.Errorf("%w: incomplete relation evidence anchor", ErrInvalidProjection)
	}
	switch evidence.GetEvidenceKind() {
	case "event", "rule", "model", "analyst_conclusion":
	default:
		return fmt.Errorf("%w: unsupported relation evidence kind", ErrInvalidProjection)
	}
	if strings.Contains(evidence.GetEvidenceId(), "\x00") || strings.Contains(tenantID, "\x00") {
		return fmt.Errorf("%w: evidence identity contains a forbidden separator", ErrInvalidProjection)
	}
	return nil
}

func validateValidity(validFrom, validTo int64) error {
	if validFrom <= 0 || (validTo != 0 && validTo < validFrom) {
		return fmt.Errorf("%w: invalid projection validity interval", ErrInvalidProjection)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isTraceID(value string) bool {
	if len(value) != 32 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value != strings.Repeat("0", 32)
}

type canonicalIdentity struct {
	TenantID    string `json:"tenant_id"`
	EntityType  string `json:"entity_type"`
	CanonicalID string `json:"canonical_id"`
	VertexID    string `json:"vertex_id"`
}

type canonicalSource struct {
	SourceSystem     string `json:"source_system"`
	SourceEventID    string `json:"source_event_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion uint64 `json:"aggregate_version"`
	SourceSHA256     string `json:"source_sha256"`
	OccurredAt       int64  `json:"occurred_at"`
}

type canonicalEvidence struct {
	EvidenceID    string `json:"evidence_id"`
	EvidenceKind  string `json:"evidence_kind"`
	ImmutableURI  string `json:"immutable_uri"`
	SHA256        string `json:"sha256"`
	SourceEventID string `json:"source_event_id"`
	OccurredAt    int64  `json:"occurred_at"`
}

func EntityProjectionSHA256(entity *trafficv1.GraphProjectedEntity) (string, error) {
	if entity == nil || entity.GetIdentity() == nil || entity.GetSource() == nil {
		return "", fmt.Errorf("%w: incomplete entity projection", ErrInvalidProjection)
	}
	keys := make([]string, 0, len(entity.GetAttributes()))
	for key := range entity.GetAttributes() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	attributes := make([][2]string, 0, len(keys))
	for _, key := range keys {
		attributes = append(attributes, [2]string{key, entity.GetAttributes()[key]})
	}
	payload := struct {
		Identity   canonicalIdentity `json:"identity"`
		Attributes [][2]string       `json:"attributes"`
		ValidFrom  int64             `json:"valid_from"`
		ValidTo    int64             `json:"valid_to"`
		Source     canonicalSource   `json:"source"`
		Revoked    bool              `json:"revoked"`
	}{canonicalIdentityOf(entity.GetIdentity()), attributes, entity.GetValidFrom(), entity.GetValidTo(), canonicalSourceOf(entity.GetSource()), entity.GetRevoked()}
	return canonicalSHA256(payload)
}

func RelationProjectionSHA256(relation *trafficv1.GraphProjectedRelation) (string, error) {
	if relation == nil || relation.GetSourceIdentity() == nil || relation.GetTargetIdentity() == nil || relation.GetSource() == nil {
		return "", fmt.Errorf("%w: incomplete relation projection", ErrInvalidProjection)
	}
	evidence := make([]canonicalEvidence, 0, len(relation.GetEvidence()))
	for _, item := range relation.GetEvidence() {
		evidence = append(evidence, canonicalEvidence{
			EvidenceID: item.GetEvidenceId(), EvidenceKind: item.GetEvidenceKind(),
			ImmutableURI: item.GetImmutableUri(), SHA256: item.GetSha256(),
			SourceEventID: item.GetSourceEventId(), OccurredAt: item.GetOccurredAt(),
		})
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].EvidenceID == evidence[j].EvidenceID {
			return evidence[i].SHA256 < evidence[j].SHA256
		}
		return evidence[i].EvidenceID < evidence[j].EvidenceID
	})
	payload := struct {
		TenantID     string                        `json:"tenant_id"`
		EdgeID       string                        `json:"edge_id"`
		RelationType string                        `json:"relation_type"`
		SourceEntity canonicalIdentity             `json:"source_identity"`
		TargetEntity canonicalIdentity             `json:"target_identity"`
		Provenance   trafficv1.GraphProvenanceKind `json:"provenance_kind"`
		Confidence   float64                       `json:"confidence"`
		Uncertainty  string                        `json:"uncertainty"`
		Evidence     []canonicalEvidence           `json:"evidence"`
		ValidFrom    int64                         `json:"valid_from"`
		ValidTo      int64                         `json:"valid_to"`
		Source       canonicalSource               `json:"source"`
		Revoked      bool                          `json:"revoked"`
	}{
		relation.GetTenantId(), relation.GetEdgeId(), relation.GetRelationType(),
		canonicalIdentityOf(relation.GetSourceIdentity()), canonicalIdentityOf(relation.GetTargetIdentity()),
		relation.GetProvenanceKind(), relation.GetConfidence(), relation.GetUncertainty(), evidence,
		relation.GetValidFrom(), relation.GetValidTo(), canonicalSourceOf(relation.GetSource()), relation.GetRevoked(),
	}
	return canonicalSHA256(payload)
}

func canonicalIdentityOf(identity *trafficv1.GraphEntityIdentity) canonicalIdentity {
	return canonicalIdentity{identity.GetTenantId(), identity.GetEntityType(), identity.GetCanonicalId(), identity.GetVertexId()}
}

func canonicalSourceOf(source *trafficv1.GraphProjectionSource) canonicalSource {
	return canonicalSource{
		source.GetSourceSystem(), source.GetSourceEventId(), source.GetAggregateType(),
		source.GetAggregateId(), source.GetAggregateVersion(), source.GetSourceSha256(), source.GetOccurredAt(),
	}
}

func canonicalSHA256(value interface{}) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize projection: %v", ErrInvalidProjection, err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
