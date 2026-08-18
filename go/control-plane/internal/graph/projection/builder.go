package projection

import (
	"fmt"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

type SourceInput struct {
	EventID          string
	TenantID         string
	TraceID          string
	Producer         string
	SourceSystem     string
	SourceEventID    string
	AggregateType    string
	AggregateID      string
	AggregateVersion uint64
	SourceSHA256     string
	OccurredAt       int64
	ProducedAt       int64
}

type EntityInput struct {
	Source      SourceInput
	EntityType  string
	CanonicalID string
	Attributes  map[string]string
	ValidFrom   int64
	ValidTo     int64
}

type RelationInput struct {
	Source            SourceInput
	RelationType      string
	SourceEntityType  string
	SourceCanonicalID string
	TargetEntityType  string
	TargetCanonicalID string
	ProvenanceKind    trafficv1.GraphProvenanceKind
	Confidence        float64
	Uncertainty       string
	Evidence          []*trafficv1.GraphEvidenceAnchor
	ValidFrom         int64
	ValidTo           int64
	Revoked           bool
}

func BuildEntityEvent(input EntityInput) (*trafficv1.GraphProjectionEvent, error) {
	vertexID, err := VertexID(input.Source.TenantID, input.EntityType, input.CanonicalID)
	if err != nil {
		return nil, err
	}
	entity := &trafficv1.GraphProjectedEntity{
		Identity: &trafficv1.GraphEntityIdentity{
			TenantId: input.Source.TenantID, EntityType: input.EntityType,
			CanonicalId: input.CanonicalID, VertexId: vertexID,
		},
		Attributes: cloneAttributes(input.Attributes), ValidFrom: input.ValidFrom, ValidTo: input.ValidTo,
		Source: sourceMessage(input.Source),
	}
	hash, err := EntityProjectionSHA256(entity)
	if err != nil {
		return nil, err
	}
	entity.ProjectionSha256 = hash
	event := &trafficv1.GraphProjectionEvent{
		Header:       headerMessage(input.Source, EntityUpsertedEventType),
		PartitionKey: input.Source.TenantID + ":" + vertexID,
		Projection:   &trafficv1.GraphProjectionEvent_Entity{Entity: entity},
	}
	if err := ValidateEvent(event); err != nil {
		return nil, fmt.Errorf("build graph entity projection: %w", err)
	}
	return event, nil
}

func BuildRelationEvent(input RelationInput) (*trafficv1.GraphProjectionEvent, error) {
	sourceVID, err := VertexID(input.Source.TenantID, input.SourceEntityType, input.SourceCanonicalID)
	if err != nil {
		return nil, err
	}
	targetVID, err := VertexID(input.Source.TenantID, input.TargetEntityType, input.TargetCanonicalID)
	if err != nil {
		return nil, err
	}
	edgeID, err := EdgeID(input.Source.TenantID, input.RelationType, sourceVID, targetVID)
	if err != nil {
		return nil, err
	}
	evidence := make([]*trafficv1.GraphEvidenceAnchor, 0, len(input.Evidence))
	for _, item := range input.Evidence {
		if item == nil {
			evidence = append(evidence, nil)
			continue
		}
		evidence = append(evidence, proto.Clone(item).(*trafficv1.GraphEvidenceAnchor))
	}
	relation := &trafficv1.GraphProjectedRelation{
		TenantId: input.Source.TenantID, EdgeId: edgeID, RelationType: input.RelationType,
		SourceIdentity: &trafficv1.GraphEntityIdentity{
			TenantId: input.Source.TenantID, EntityType: input.SourceEntityType,
			CanonicalId: input.SourceCanonicalID, VertexId: sourceVID,
		},
		TargetIdentity: &trafficv1.GraphEntityIdentity{
			TenantId: input.Source.TenantID, EntityType: input.TargetEntityType,
			CanonicalId: input.TargetCanonicalID, VertexId: targetVID,
		},
		ProvenanceKind: input.ProvenanceKind, Confidence: input.Confidence,
		Uncertainty: input.Uncertainty, Evidence: evidence,
		ValidFrom: input.ValidFrom, ValidTo: input.ValidTo,
		Source: sourceMessage(input.Source), Revoked: input.Revoked,
	}
	hash, err := RelationProjectionSHA256(relation)
	if err != nil {
		return nil, err
	}
	relation.ProjectionSha256 = hash
	eventType := RelationUpsertedEventType
	if input.Revoked {
		eventType = RelationRevokedEventType
	}
	event := &trafficv1.GraphProjectionEvent{
		Header:       headerMessage(input.Source, eventType),
		PartitionKey: input.Source.TenantID + ":" + edgeID,
		Projection:   &trafficv1.GraphProjectionEvent_Relation{Relation: relation},
	}
	if err := ValidateEvent(event); err != nil {
		return nil, fmt.Errorf("build graph relation projection: %w", err)
	}
	return event, nil
}

func headerMessage(source SourceInput, eventType string) *trafficv1.EventHeader {
	return &trafficv1.EventHeader{
		EventId: source.EventID, TenantId: source.TenantID, EventType: eventType,
		SchemaVersion: SchemaVersion, AggregateType: source.AggregateType,
		AggregateId: source.AggregateID, AggregateVersion: source.AggregateVersion,
		OccurredAt: source.OccurredAt, ProducedAt: source.ProducedAt,
		TraceId: source.TraceID, CausationId: source.SourceEventID, Producer: source.Producer,
	}
}

func sourceMessage(source SourceInput) *trafficv1.GraphProjectionSource {
	return &trafficv1.GraphProjectionSource{
		SourceSystem: source.SourceSystem, SourceEventId: source.SourceEventID,
		AggregateType: source.AggregateType, AggregateId: source.AggregateID,
		AggregateVersion: source.AggregateVersion, SourceSha256: source.SourceSHA256,
		OccurredAt: source.OccurredAt,
	}
}

func cloneAttributes(attributes map[string]string) map[string]string {
	result := make(map[string]string, len(attributes))
	for key, value := range attributes {
		result[key] = value
	}
	return result
}
