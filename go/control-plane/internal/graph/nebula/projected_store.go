package nebula

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	nebula_go "github.com/vesoft-inc/nebula-go/v3"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/projection"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func recordBool(record *nebula_go.Record, column string) (bool, error) {
	value, err := recordValue(record, column)
	if err != nil || value == nil {
		return false, err
	}
	result, err := value.AsBool()
	if err != nil {
		return false, fmt.Errorf("column %s: %w", column, err)
	}
	return result, nil
}

const projectionReadyQuery = `YIELD 1 AS ready;`

// Apply implements projection.Target. Each mutation is deterministic and safe
// to replay after a Nebula acknowledgement / PostgreSQL commit ambiguity.
func (s *WorkbenchStore) Apply(ctx context.Context, event *trafficv1.GraphProjectionEvent) error {
	if err := projection.ValidateEvent(event); err != nil {
		return err
	}
	if entity := event.GetEntity(); entity != nil {
		return s.applyProjectedEntity(ctx, event.GetHeader(), entity)
	}
	return s.applyProjectedRelation(ctx, event.GetHeader(), event.GetRelation())
}

func (s *WorkbenchStore) applyProjectedEntity(
	ctx context.Context,
	header *trafficv1.EventHeader,
	entity *trafficv1.GraphProjectedEntity,
) error {
	attributes, err := json.Marshal(entity.GetAttributes())
	if err != nil {
		return fmt.Errorf("marshal graph entity attributes: %w", err)
	}
	validFrom, err := nebulaParameterInt(entity.GetValidFrom(), "graph entity valid_from")
	if err != nil {
		return err
	}
	validTo, err := nebulaParameterInt(entity.GetValidTo(), "graph entity valid_to")
	if err != nil {
		return err
	}
	version, err := nebulaParameterUint(entity.GetSource().GetAggregateVersion(), "graph entity aggregate_version")
	if err != nil {
		return err
	}
	updatedAt, err := nebulaParameterInt(header.GetProducedAt(), "graph entity updated_at")
	if err != nil {
		return err
	}
	statement := fmt.Sprintf(`UPSERT VERTEX ON projected_entity_v1 %s
SET tenant_id=$tenant_id,entity_type=$entity_type,canonical_id=$canonical_id,
    attributes_json=$attributes_json,placeholder=false,valid_from=$valid_from,valid_to=$valid_to,
    source_system=$source_system,source_event_id=$source_event_id,
    aggregate_type=$aggregate_type,aggregate_id=$aggregate_id,
    aggregate_version=$aggregate_version,source_sha256=$source_sha256,
    projection_sha256=$projection_sha256,revoked=false,updated_at=$updated_at;`,
		projectionVIDLiteral(entity.GetIdentity().GetVertexId()))
	parameters := map[string]interface{}{
		"tenant_id": entity.GetIdentity().GetTenantId(), "entity_type": entity.GetIdentity().GetEntityType(),
		"canonical_id": entity.GetIdentity().GetCanonicalId(), "attributes_json": string(attributes),
		"valid_from": validFrom, "valid_to": validTo,
		"source_system": entity.GetSource().GetSourceSystem(), "source_event_id": entity.GetSource().GetSourceEventId(),
		"aggregate_type": entity.GetSource().GetAggregateType(), "aggregate_id": entity.GetSource().GetAggregateId(),
		"aggregate_version": version, "source_sha256": entity.GetSource().GetSourceSha256(),
		"projection_sha256": entity.GetProjectionSha256(), "updated_at": updatedAt,
	}
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("upsert projected graph entity: %w", err)
	}
	return nil
}

func (s *WorkbenchStore) applyProjectedRelation(
	ctx context.Context,
	header *trafficv1.EventHeader,
	relation *trafficv1.GraphProjectedRelation,
) error {
	if relation == nil {
		return fmt.Errorf("graph relation projection is required")
	}
	if err := s.ensureProjectedEndpoint(ctx, header, relation.GetSourceIdentity(), relation.GetSource(), relation.GetProjectionSha256()); err != nil {
		return fmt.Errorf("ensure graph relation source: %w", err)
	}
	if err := s.ensureProjectedEndpoint(ctx, header, relation.GetTargetIdentity(), relation.GetSource(), relation.GetProjectionSha256()); err != nil {
		return fmt.Errorf("ensure graph relation target: %w", err)
	}
	rank, err := projection.NebulaEdgeRank(relation.GetEdgeId())
	if err != nil {
		return err
	}
	evidenceJSON, err := marshalEvidence(relation.GetEvidence())
	if err != nil {
		return err
	}
	validFrom, err := nebulaParameterInt(relation.GetValidFrom(), "graph relation valid_from")
	if err != nil {
		return err
	}
	validTo, err := nebulaParameterInt(relation.GetValidTo(), "graph relation valid_to")
	if err != nil {
		return err
	}
	version, err := nebulaParameterUint(relation.GetSource().GetAggregateVersion(), "graph relation aggregate_version")
	if err != nil {
		return err
	}
	updatedAt, err := nebulaParameterInt(header.GetProducedAt(), "graph relation updated_at")
	if err != nil {
		return err
	}
	statement := fmt.Sprintf(`UPSERT EDGE ON projected_relation_v1 %s->%s@%d
SET tenant_id=$tenant_id,edge_id=$edge_id,relation_type=$relation_type,
    source_id=$source_id,target_id=$target_id,provenance_kind=$provenance_kind,
    confidence=$confidence,uncertainty=$uncertainty,evidence_json=$evidence_json,
    valid_from=$valid_from,valid_to=$valid_to,source_system=$source_system,
    source_event_id=$source_event_id,aggregate_type=$aggregate_type,
    aggregate_id=$aggregate_id,aggregate_version=$aggregate_version,
    source_sha256=$source_sha256,projection_sha256=$projection_sha256,
    revoked=$revoked,updated_at=$updated_at;`,
		projectionVIDLiteral(relation.GetSourceIdentity().GetVertexId()),
		projectionVIDLiteral(relation.GetTargetIdentity().GetVertexId()), rank)
	parameters := map[string]interface{}{
		"tenant_id": relation.GetTenantId(), "edge_id": relation.GetEdgeId(),
		"relation_type": relation.GetRelationType(), "source_id": relation.GetSourceIdentity().GetCanonicalId(),
		"target_id":       relation.GetTargetIdentity().GetCanonicalId(),
		"provenance_kind": provenanceName(relation.GetProvenanceKind()),
		"confidence":      relation.GetConfidence(), "uncertainty": relation.GetUncertainty(),
		"evidence_json": evidenceJSON, "valid_from": validFrom, "valid_to": validTo,
		"source_system": relation.GetSource().GetSourceSystem(), "source_event_id": relation.GetSource().GetSourceEventId(),
		"aggregate_type": relation.GetSource().GetAggregateType(), "aggregate_id": relation.GetSource().GetAggregateId(),
		"aggregate_version": version, "source_sha256": relation.GetSource().GetSourceSha256(),
		"projection_sha256": relation.GetProjectionSha256(), "revoked": relation.GetRevoked(), "updated_at": updatedAt,
	}
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("upsert projected graph relation: %w", err)
	}
	return nil
}

func (s *WorkbenchStore) ensureProjectedEndpoint(
	ctx context.Context,
	header *trafficv1.EventHeader,
	identity *trafficv1.GraphEntityIdentity,
	source *trafficv1.GraphProjectionSource,
	relationProjectionSHA256 string,
) error {
	version, err := nebulaParameterUint(source.GetAggregateVersion(), "graph endpoint aggregate_version")
	if err != nil {
		return err
	}
	updatedAt, err := nebulaParameterInt(header.GetProducedAt(), "graph endpoint updated_at")
	if err != nil {
		return err
	}
	statement := fmt.Sprintf(`INSERT VERTEX IF NOT EXISTS projected_entity_v1(
  tenant_id,entity_type,canonical_id,attributes_json,placeholder,valid_from,valid_to,
  source_system,source_event_id,aggregate_type,aggregate_id,aggregate_version,
  source_sha256,projection_sha256,revoked,updated_at
) VALUES %s:($tenant_id,$entity_type,$canonical_id,"{\"placeholder\":\"relation_endpoint\"}",
  true,$valid_from,0,$source_system,$source_event_id,$aggregate_type,$aggregate_id,
  $aggregate_version,$source_sha256,$projection_sha256,false,$updated_at);`,
		projectionVIDLiteral(identity.GetVertexId()))
	parameters := map[string]interface{}{
		"tenant_id": identity.GetTenantId(), "entity_type": identity.GetEntityType(),
		"canonical_id": identity.GetCanonicalId(), "valid_from": updatedAt,
		"source_system": source.GetSourceSystem(), "source_event_id": source.GetSourceEventId(),
		"aggregate_type": source.GetAggregateType(), "aggregate_id": source.GetAggregateId(),
		"aggregate_version": version, "source_sha256": source.GetSourceSha256(),
		"projection_sha256": relationProjectionSHA256, "updated_at": updatedAt,
	}
	_, err = s.execute(ctx, statement, parameters)
	return err
}

func (s *WorkbenchStore) ReadyProjection(ctx context.Context) error {
	if _, err := s.execute(ctx, projectionReadyQuery, nil); err != nil {
		return fmt.Errorf("verify graph projection session: %w", err)
	}
	checks := []string{
		"DESCRIBE TAG projected_entity_v1;",
		"DESCRIBE EDGE projected_relation_v1;",
	}
	for _, statement := range checks {
		if _, err := s.execute(ctx, statement, nil); err != nil {
			return fmt.Errorf("verify graph projection schema: %w", err)
		}
	}
	return nil
}

// LoadReconcileSnapshot returns only authority-shaped projected facts. The
// structural placeholder vertices required by NebulaGraph edge insertion are
// intentionally excluded; they are neither source facts nor delete targets.
func (s *WorkbenchStore) LoadReconcileSnapshot(
	ctx context.Context,
	scope projection.ReconcileScope,
) ([]projection.ProjectionFact, string, error) {
	through, err := nebulaParameterInt(scope.WindowThrough.UnixMilli(), "graph reconcile window_through")
	if err != nil {
		return nil, "", err
	}
	from, err := nebulaParameterInt(scope.WindowFrom.UnixMilli(), "graph reconcile window_from")
	if err != nil {
		return nil, "", err
	}
	parameters := map[string]interface{}{"tenant_id": scope.TenantID, "window_from": from, "window_through": through}
	entityStatement := fmt.Sprintf(`PROFILE LOOKUP ON projected_entity_v1
WHERE projected_entity_v1.tenant_id == $tenant_id
  AND projected_entity_v1.placeholder == false
  AND projected_entity_v1.valid_from <= $window_through
  AND (projected_entity_v1.valid_to == 0 OR projected_entity_v1.valid_to >= $window_from)
YIELD id(vertex) AS projection_id,
      projected_entity_v1.aggregate_version AS aggregate_version,
      projected_entity_v1.projection_sha256 AS projection_sha256,
      projected_entity_v1.revoked AS revoked
| ORDER BY $-.projection_id | LIMIT %d;`, scope.MaxFacts+1)
	relationStatement := fmt.Sprintf(`PROFILE LOOKUP ON projected_relation_v1
WHERE projected_relation_v1.tenant_id == $tenant_id
  AND projected_relation_v1.valid_from <= $window_through
  AND (projected_relation_v1.valid_to == 0 OR projected_relation_v1.valid_to >= $window_from)
YIELD projected_relation_v1.edge_id AS projection_id,
      projected_relation_v1.aggregate_version AS aggregate_version,
      projected_relation_v1.projection_sha256 AS projection_sha256,
      projected_relation_v1.revoked AS revoked
| ORDER BY $-.projection_id | LIMIT %d;`, scope.MaxFacts+1)
	entityResult, err := s.execute(ctx, entityStatement, parameters)
	if err != nil {
		return nil, "", fmt.Errorf("profile reconciled graph entities: %w", err)
	}
	relationResult, err := s.execute(ctx, relationStatement, parameters)
	if err != nil {
		return nil, "", fmt.Errorf("profile reconciled graph relations: %w", err)
	}
	facts, err := decodeProjectionFacts(entityResult, "entity", nil)
	if err != nil {
		return nil, "", err
	}
	facts, err = decodeProjectionFacts(relationResult, "relation", facts)
	if err != nil {
		return nil, "", err
	}
	profilePayload, err := json.Marshal(map[string]interface{}{
		"engine":   "nebula-profile-v1",
		"entity":   entityResult.MakePlanByRow(),
		"relation": relationResult.MakePlanByRow(),
		"rows":     len(facts),
	})
	if err != nil {
		return nil, "", fmt.Errorf("marshal NebulaGraph reconcile PROFILE: %w", err)
	}
	return facts, string(profilePayload), nil
}

func decodeProjectionFacts(
	result interface {
		GetRowSize() int
		GetRowValuesByIndex(int) (*nebula_go.Record, error)
	},
	kind string,
	facts []projection.ProjectionFact,
) ([]projection.ProjectionFact, error) {
	for index := 0; index < result.GetRowSize(); index++ {
		record, err := result.GetRowValuesByIndex(index)
		if err != nil {
			return nil, fmt.Errorf("decode projected %s row %d: %w", kind, index, err)
		}
		projectionID, err := recordString(record, "projection_id")
		if err != nil {
			return nil, err
		}
		version, err := recordInt(record, "aggregate_version")
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("decode projected %s version row %d: %w", kind, index, err)
		}
		projectionSHA, err := recordString(record, "projection_sha256")
		if err != nil {
			return nil, err
		}
		revoked, err := recordBool(record, "revoked")
		if err != nil {
			return nil, err
		}
		facts = append(facts, projection.ProjectionFact{
			Kind: kind, ProjectionID: projectionID, AggregateVersion: uint64(version),
			ProjectionSHA256: projectionSHA, Revoked: revoked,
		})
	}
	return facts, nil
}

// projectionTarget exposes the stricter projection readiness check without
// changing the workbench query store's legacy Ready behavior.
type projectionTarget struct{ store *WorkbenchStore }

func NewProjectionTarget(store *WorkbenchStore) (projection.Target, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("NebulaGraph projection store is required")
	}
	return projectionTarget{store: store}, nil
}

func (target projectionTarget) Ready(ctx context.Context) error {
	return target.store.ReadyProjection(ctx)
}

func (target projectionTarget) Apply(ctx context.Context, event *trafficv1.GraphProjectionEvent) error {
	return target.store.Apply(ctx, event)
}

func projectionVIDLiteral(vid string) string {
	if len(vid) != 32 || vid != strings.ToLower(vid) {
		panic("projection VID reached Nebula adapter without contract validation")
	}
	if _, err := hex.DecodeString(vid); err != nil {
		panic("projection VID reached Nebula adapter without hexadecimal identity")
	}
	return `"` + vid + `"`
}

func nebulaParameterUint(value uint64, field string) (int, error) {
	if value > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("%s exceeds NebulaGraph integer parameter range", field)
	}
	return int(value), nil
}

func provenanceName(kind trafficv1.GraphProvenanceKind) string {
	switch kind {
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_OBSERVED:
		return "observed"
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_DERIVED:
		return "derived"
	case trafficv1.GraphProvenanceKind_GRAPH_PROVENANCE_KIND_ANALYST:
		return "analyst"
	default:
		return "unspecified"
	}
}

func marshalEvidence(items []*trafficv1.GraphEvidenceAnchor) (string, error) {
	type evidence struct {
		EvidenceID    string `json:"evidence_id"`
		EvidenceKind  string `json:"evidence_kind"`
		ImmutableURI  string `json:"immutable_uri"`
		SHA256        string `json:"sha256"`
		SourceEventID string `json:"source_event_id"`
		OccurredAt    int64  `json:"occurred_at"`
	}
	values := make([]evidence, 0, len(items))
	for _, item := range items {
		values = append(values, evidence{
			item.GetEvidenceId(), item.GetEvidenceKind(), item.GetImmutableUri(),
			item.GetSha256(), item.GetSourceEventId(), item.GetOccurredAt(),
		})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].EvidenceID == values[j].EvidenceID {
			return values[i].SHA256 < values[j].SHA256
		}
		return values[i].EvidenceID < values[j].EvidenceID
	})
	payload, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("marshal graph relation evidence: %w", err)
	}
	return string(payload), nil
}
