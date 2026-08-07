package nebula

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	nebula_go "github.com/vesoft-inc/nebula-go/v3"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

const workbenchNodeQuery = `LOOKUP ON entity
WHERE entity.tenant_id == $tenant_id
YIELD entity.entity_id AS entity_id,
      entity.entity_type AS entity_type,
      entity.label AS label,
      entity.detail AS detail,
      entity.risk_score AS risk_score,
      entity.risk_level AS risk_level,
      entity.x AS x,
      entity.y AS y,
      entity.icon AS icon,
      entity.metadata_json AS metadata_json,
      entity.updated_at AS updated_at;`

const workbenchEdgeQuery = `LOOKUP ON relation
WHERE relation.tenant_id == $tenant_id
YIELD relation.relation_id AS relation_id,
      relation.source_id AS source_id,
      relation.target_id AS target_id,
      relation.relation_type AS relation_type,
      relation.risk_level AS risk_level,
      relation.evidence_id AS evidence_id,
      relation.attributes_json AS attributes_json,
      relation.weight AS weight,
      relation.observed_at AS observed_at;`

const assetProjectionNodeQuery = `FETCH PROP ON entity %s
YIELD entity.entity_id AS entity_id,
      entity.entity_type AS entity_type,
      entity.label AS label,
      entity.detail AS detail,
      entity.risk_score AS risk_score,
      entity.risk_level AS risk_level,
      entity.x AS x,
      entity.y AS y,
      entity.icon AS icon,
      entity.metadata_json AS metadata_json,
      entity.updated_at AS updated_at`

const assetProjectionEdgeQuery = `GO FROM %s OVER relation BIDIRECT
WHERE relation.tenant_id == $tenant_id
YIELD relation.relation_id AS relation_id,
      relation.source_id AS source_id,
      relation.target_id AS target_id,
      relation.relation_type AS relation_type,
      relation.risk_level AS risk_level,
      relation.evidence_id AS evidence_id,
      relation.attributes_json AS attributes_json,
      relation.weight AS weight,
      relation.observed_at AS observed_at`

// WorkbenchStore serves the entity workbench directly from NebulaGraph using
// the official Go SDK. The session pool is bound to one graph space and user,
// which keeps credentials and space selection out of individual queries.
type WorkbenchStore struct {
	pool   *nebula_go.SessionPool
	logger *zap.Logger
}

// AssetEntityProjection is the deterministic graph representation of an
// authoritative asset revision. Revision is also embedded in Metadata so that
// reconciliation can compare NebulaGraph state with PostgreSQL watermarks.
type AssetEntityProjection struct {
	TenantID  string                 `json:"tenant_id"`
	AssetID   string                 `json:"asset_id"`
	Label     string                 `json:"label"`
	Detail    string                 `json:"detail"`
	RiskScore int64                  `json:"risk_score"`
	RiskLevel string                 `json:"risk_level"`
	Icon      string                 `json:"icon"`
	Metadata  map[string]interface{} `json:"metadata"`
	Revision  int64                  `json:"revision"`
	UpdatedAt int64                  `json:"updated_at"`
}

// CampaignEntityProjection is the current campaign aggregate materialized in
// the shared entity graph. Metadata carries the immutable source event and the
// PostgreSQL projection watermark used by reconciliation.
type CampaignEntityProjection struct {
	TenantID    string                 `json:"tenant_id"`
	CampaignID  string                 `json:"campaign_id"`
	Label       string                 `json:"label"`
	Status      string                 `json:"status"`
	Assignee    string                 `json:"assignee"`
	MemberCount int64                  `json:"member_count"`
	Metadata    map[string]interface{} `json:"metadata"`
	Revision    int64                  `json:"revision"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// CampaignMembershipProjection represents the current campaign-to-alert edge.
// Linked=false removes the deterministic edge; the immutable event remains in
// PostgreSQL and ClickHouse for replay and audit.
type CampaignMembershipProjection struct {
	TenantID         string                 `json:"tenant_id"`
	RelationID       string                 `json:"relation_id"`
	CampaignID       string                 `json:"campaign_id"`
	AlertID          string                 `json:"alert_id"`
	Linked           bool                   `json:"linked"`
	Metadata         map[string]interface{} `json:"metadata"`
	Revision         int64                  `json:"revision"`
	CampaignRevision int64                  `json:"campaign_revision"`
	ObservedAt       int64                  `json:"observed_at"`
}

type zapNebulaLogger struct{ logger *zap.Logger }

func (l zapNebulaLogger) Info(message string) {
	l.logger.Debug("NebulaGraph client", zap.String("message", message))
}
func (l zapNebulaLogger) Warn(message string) {
	l.logger.Warn("NebulaGraph client", zap.String("message", message))
}
func (l zapNebulaLogger) Error(message string) {
	l.logger.Error("NebulaGraph client", zap.String("message", message))
}
func (l zapNebulaLogger) Fatal(message string) {
	l.logger.Error("NebulaGraph client fatal", zap.String("message", message))
}

func NewWorkbenchStore(cfg config.NebulaConfig, logger *zap.Logger) (*WorkbenchStore, error) {
	addresses := make([]nebula_go.HostAddress, 0, len(cfg.Addresses))
	for _, rawAddress := range cfg.Addresses {
		host, portText, err := net.SplitHostPort(strings.TrimSpace(rawAddress))
		if err != nil {
			return nil, fmt.Errorf("invalid NebulaGraph address %q: %w", rawAddress, err)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("invalid NebulaGraph port in %q: %w", rawAddress, err)
		}
		addresses = append(addresses, nebula_go.HostAddress{Host: host, Port: port})
	}

	poolConfig, err := nebula_go.NewSessionPoolConf(
		cfg.Username,
		cfg.Password,
		addresses,
		cfg.Space,
		nebula_go.WithTimeOut(cfg.Timeout),
		nebula_go.WithIdleTime(cfg.IdleTime),
		nebula_go.WithMaxSize(cfg.MaxPoolSize),
		nebula_go.WithMinSize(cfg.MinPoolSize),
	)
	if err != nil {
		return nil, fmt.Errorf("create NebulaGraph session pool config: %w", err)
	}

	pool, err := nebula_go.NewSessionPool(*poolConfig, zapNebulaLogger{logger: logger})
	if err != nil {
		return nil, fmt.Errorf("connect to NebulaGraph: %w", err)
	}
	return &WorkbenchStore{pool: pool, logger: logger}, nil
}

func (s *WorkbenchStore) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

// Ready verifies the configured graph space through the same authenticated
// session pool used for projections.
func (s *WorkbenchStore) Ready(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("NebulaGraph workbench store is unavailable")
	}
	if _, err := s.execute(ctx, `YIELD 1 AS ready;`, nil); err != nil {
		return fmt.Errorf("verify NebulaGraph workbench store: %w", err)
	}
	return nil
}

func (s *WorkbenchStore) LoadWorkbenchGraph(ctx context.Context, tenantID string) ([]*query.WorkbenchNode, []*query.WorkbenchEdge, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, nil, fmt.Errorf("tenant ID is required")
	}
	parameters := map[string]interface{}{"tenant_id": tenantID}

	nodeResult, err := s.execute(ctx, workbenchNodeQuery, parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("query workbench nodes: %w", err)
	}
	nodes, err := decodeWorkbenchNodes(nodeResult)
	if err != nil {
		return nil, nil, err
	}

	edgeResult, err := s.execute(ctx, workbenchEdgeQuery, parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("query workbench edges: %w", err)
	}
	edges, err := decodeWorkbenchEdges(edgeResult)
	if err != nil {
		return nil, nil, err
	}

	s.logger.Debug("Loaded workbench graph from NebulaGraph",
		zap.String("tenant_id", tenantID),
		zap.Int("nodes", len(nodes)),
		zap.Int("edges", len(edges)))
	return nodes, edges, nil
}

// LoadAssetProjection is intentionally separate from LoadWorkbenchGraph: the
// asset detail endpoint must never load an entire tenant graph. The vertex is
// fetched by a tenant-namespaced deterministic VID and relations are a
// tenant-filtered adjacency traversal; one extra edge makes truncation explicit.
func (s *WorkbenchStore) LoadAssetProjection(
	ctx context.Context,
	tenantID, assetID string,
	relationLimit int,
) (*query.WorkbenchNode, []*query.WorkbenchEdge, bool, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(assetID) == "" {
		return nil, nil, false, fmt.Errorf("tenant ID and asset ID are required")
	}
	if relationLimit <= 0 || relationLimit > 500 {
		return nil, nil, false, fmt.Errorf("relation limit must be within 1..500")
	}
	parameters := map[string]interface{}{"tenant_id": tenantID}
	vidLiteral := tenantVIDLiteral(tenantID, assetID)
	nodeResult, err := s.execute(ctx, fmt.Sprintf(assetProjectionNodeQuery, vidLiteral)+";", parameters)
	if err != nil {
		return nil, nil, false, fmt.Errorf("query bounded asset projection node: %w", err)
	}
	nodes, err := decodeWorkbenchNodes(nodeResult)
	if err != nil {
		return nil, nil, false, err
	}
	if len(nodes) == 0 {
		return nil, nil, false, fmt.Errorf("asset projection not found")
	}
	if len(nodes) > 1 {
		return nil, nil, false, fmt.Errorf("asset projection identity is not unique")
	}
	edgeQuery := fmt.Sprintf(assetProjectionEdgeQuery, vidLiteral)
	edgeStatement := fmt.Sprintf("%s | LIMIT %d;", edgeQuery, relationLimit+1)
	edgeResult, err := s.execute(ctx, edgeStatement, parameters)
	if err != nil {
		return nil, nil, false, fmt.Errorf("query bounded asset projection relations: %w", err)
	}
	edges, err := decodeWorkbenchEdges(edgeResult)
	if err != nil {
		return nil, nil, false, err
	}
	truncated := len(edges) > relationLimit
	if truncated {
		edges = edges[:relationLimit]
	}
	return nodes[0], edges, truncated, nil
}

// UpsertAssetEntity uses a tenant-namespaced deterministic VID and parameterized
// property values. NebulaGraph 3.6 does not accept parameters in VID grammar
// positions, so only the fixed 32-hex hash is rendered as a quoted literal.
// Ordering is enforced by the asset projection inbox/watermark worker; replaying
// the same projection is therefore safe and produces the same vertex.
func (s *WorkbenchStore) UpsertAssetEntity(ctx context.Context, asset AssetEntityProjection) error {
	if strings.TrimSpace(asset.TenantID) == "" ||
		strings.TrimSpace(asset.AssetID) == "" ||
		asset.Revision <= 0 {
		return fmt.Errorf("invalid asset entity projection")
	}
	if asset.RiskScore < 0 {
		asset.RiskScore = 0
	} else if asset.RiskScore > 100 {
		asset.RiskScore = 100
	}
	updatedAt, err := nebulaParameterInt(asset.UpdatedAt, "asset updated_at")
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(asset.Metadata)
	if err != nil {
		return fmt.Errorf("marshal asset entity metadata: %w", err)
	}
	statement := fmt.Sprintf(`UPSERT VERTEX ON entity %s
SET tenant_id=$tenant_id,
    entity_id=$entity_id,
    entity_type="asset",
    label=$label,
    detail=$detail,
    risk_score=$risk_score,
    risk_level=$risk_level,
    x=0.0,
    y=0.0,
    icon=$icon,
    metadata_json=$metadata_json,
    updated_at=$updated_at;`, tenantVIDLiteral(asset.TenantID, asset.AssetID))
	parameters := map[string]interface{}{
		"tenant_id":     asset.TenantID,
		"entity_id":     asset.AssetID,
		"label":         asset.Label,
		"detail":        asset.Detail,
		"risk_score":    int(asset.RiskScore),
		"risk_level":    asset.RiskLevel,
		"icon":          asset.Icon,
		"metadata_json": string(metadataJSON),
		"updated_at":    updatedAt,
	}
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("upsert asset entity: %w", err)
	}
	return nil
}

// UpsertCampaignEntity writes the latest aggregate state to a tenant-scoped
// deterministic vertex. PostgreSQL target watermarks serialize revisions, and
// replaying the same state produces the same vertex and metadata.
func (s *WorkbenchStore) UpsertCampaignEntity(
	ctx context.Context,
	campaign CampaignEntityProjection,
) error {
	if strings.TrimSpace(campaign.TenantID) == "" ||
		strings.TrimSpace(campaign.CampaignID) == "" ||
		campaign.Revision <= 0 ||
		campaign.UpdatedAt <= 0 {
		return fmt.Errorf("invalid campaign entity projection")
	}
	updatedAt, err := nebulaParameterInt(campaign.UpdatedAt, "campaign updated_at")
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(campaign.Metadata)
	if err != nil {
		return fmt.Errorf("marshal campaign entity metadata: %w", err)
	}
	label := strings.TrimSpace(campaign.Label)
	if label == "" {
		label = campaign.CampaignID
	}
	detail := fmt.Sprintf(
		"status=%s assignee=%s members=%d",
		strings.TrimSpace(campaign.Status),
		strings.TrimSpace(campaign.Assignee),
		campaign.MemberCount,
	)
	statement := fmt.Sprintf(`UPSERT VERTEX ON entity %s
SET tenant_id=$tenant_id,
    entity_id=$entity_id,
    entity_type="campaign",
    label=$label,
    detail=$detail,
    risk_score=0,
    risk_level="",
    x=0.0,
    y=0.0,
    icon="campaign",
    metadata_json=$metadata_json,
    updated_at=$updated_at;`, tenantVIDLiteral(campaign.TenantID, campaign.CampaignID))
	parameters := map[string]interface{}{
		"tenant_id":     campaign.TenantID,
		"entity_id":     campaign.CampaignID,
		"label":         label,
		"detail":        detail,
		"metadata_json": string(metadataJSON),
		"updated_at":    updatedAt,
	}
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("upsert campaign entity: %w", err)
	}
	return nil
}

// ApplyCampaignMembership creates or removes one deterministic relation edge.
// Endpoint placeholders use INSERT IF NOT EXISTS so richer alert/campaign
// vertices owned by their authoritative projectors are never overwritten.
func (s *WorkbenchStore) ApplyCampaignMembership(
	ctx context.Context,
	membership CampaignMembershipProjection,
) error {
	if strings.TrimSpace(membership.TenantID) == "" ||
		strings.TrimSpace(membership.RelationID) == "" ||
		strings.TrimSpace(membership.CampaignID) == "" ||
		strings.TrimSpace(membership.AlertID) == "" ||
		membership.Revision <= 0 ||
		membership.CampaignRevision <= 0 ||
		membership.ObservedAt <= 0 {
		return fmt.Errorf("invalid campaign membership projection")
	}
	observedAt, err := nebulaParameterInt(membership.ObservedAt, "campaign membership observed_at")
	if err != nil {
		return err
	}
	if err := s.ensureProjectionEndpoint(
		ctx,
		membership.TenantID,
		membership.CampaignID,
		"campaign",
		membership.CampaignID,
		"campaign",
		membership.ObservedAt,
	); err != nil {
		return err
	}
	if err := s.ensureProjectionEndpoint(
		ctx,
		membership.TenantID,
		membership.AlertID,
		"alert",
		membership.AlertID,
		"alert",
		membership.ObservedAt,
	); err != nil {
		return err
	}
	sourceVID := tenantVIDLiteral(membership.TenantID, membership.CampaignID)
	targetVID := tenantVIDLiteral(membership.TenantID, membership.AlertID)
	parameters := make(map[string]interface{})
	if !membership.Linked {
		statement := fmt.Sprintf(`DELETE EDGE relation %s->%s@0;`, sourceVID, targetVID)
		if _, err := s.execute(ctx, statement, parameters); err != nil {
			return fmt.Errorf("delete campaign membership edge: %w", err)
		}
		return nil
	}
	metadata := make(map[string]interface{}, len(membership.Metadata)+3)
	for key, value := range membership.Metadata {
		metadata[key] = value
	}
	metadata["relation_revision"] = membership.Revision
	metadata["campaign_revision"] = membership.CampaignRevision
	metadata["linked"] = true
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal campaign membership metadata: %w", err)
	}
	parameters["tenant_id"] = membership.TenantID
	parameters["relation_id"] = membership.RelationID
	parameters["source_id"] = membership.CampaignID
	parameters["target_id"] = membership.AlertID
	parameters["attributes_json"] = string(metadataJSON)
	parameters["observed_at"] = observedAt
	statement := fmt.Sprintf(`UPSERT EDGE ON relation %s->%s@0
SET tenant_id=$tenant_id,
    relation_id=$relation_id,
    source_id=$source_id,
    target_id=$target_id,
    relation_type="campaign_alert",
    risk_level="",
    evidence_id="",
    attributes_json=$attributes_json,
    weight=1.0,
    observed_at=$observed_at;`, sourceVID, targetVID)
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("upsert campaign membership edge: %w", err)
	}
	return nil
}

func (s *WorkbenchStore) ensureProjectionEndpoint(
	ctx context.Context,
	tenantID string,
	entityID string,
	entityType string,
	label string,
	icon string,
	updatedAt int64,
) error {
	updatedAtParameter, err := nebulaParameterInt(updatedAt, entityType+" endpoint updated_at")
	if err != nil {
		return err
	}
	statement := fmt.Sprintf(`INSERT VERTEX IF NOT EXISTS entity(
  tenant_id,entity_id,entity_type,label,detail,risk_score,risk_level,x,y,icon,metadata_json,updated_at
) VALUES %s:($tenant_id,$entity_id,$entity_type,$label,"",0,"",0.0,0.0,$icon,"{}",$updated_at);`, tenantVIDLiteral(tenantID, entityID))
	parameters := map[string]interface{}{
		"tenant_id":   tenantID,
		"entity_id":   entityID,
		"entity_type": entityType,
		"label":       label,
		"icon":        icon,
		"updated_at":  updatedAtParameter,
	}
	if _, err := s.execute(ctx, statement, parameters); err != nil {
		return fmt.Errorf("ensure %s projection endpoint: %w", entityType, err)
	}
	return nil
}

func nebulaParameterInt(value int64, field string) (int, error) {
	converted := int(value)
	if int64(converted) != value {
		return 0, fmt.Errorf("%s exceeds NebulaGraph integer parameter range", field)
	}
	return converted, nil
}

func tenantVIDLiteral(tenantID, entityID string) string {
	// hashTenantVID is always exactly 32 lowercase hexadecimal characters, so
	// this is a grammar-safe literal even when the source IDs contain quotes.
	return `"` + hashTenantVID(tenantID, entityID) + `"`
}

func (s *WorkbenchStore) execute(ctx context.Context, statement string, parameters map[string]interface{}) (*nebula_go.ResultSet, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	result, err := s.pool.ExecuteWithParameter(statement, parameters)
	if err != nil {
		return nil, err
	}
	if !result.IsSucceed() {
		return nil, fmt.Errorf("nGQL error %d: %s", result.GetErrorCode(), result.GetErrorMsg())
	}
	return result, nil
}

func decodeWorkbenchNodes(result *nebula_go.ResultSet) ([]*query.WorkbenchNode, error) {
	nodes := make([]*query.WorkbenchNode, 0, result.GetRowSize())
	for index := 0; index < result.GetRowSize(); index++ {
		record, err := result.GetRowValuesByIndex(index)
		if err != nil {
			return nil, fmt.Errorf("decode NebulaGraph node row %d: %w", index, err)
		}
		metadataJSON, err := recordString(record, "metadata_json")
		if err != nil {
			return nil, fmt.Errorf("decode NebulaGraph node metadata row %d: %w", index, err)
		}
		metadata := make(map[string]interface{})
		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				return nil, fmt.Errorf("decode NebulaGraph node metadata JSON row %d: %w", index, err)
			}
		}
		riskScore, err := recordInt(record, "risk_score")
		if err != nil {
			return nil, fmt.Errorf("decode NebulaGraph node risk row %d: %w", index, err)
		}
		if riskScore < 0 {
			riskScore = 0
		} else if riskScore > 100 {
			riskScore = 100
		}
		node := &query.WorkbenchNode{Metadata: metadata, RiskScore: uint8(riskScore)}
		if node.EntityID, err = recordString(record, "entity_id"); err != nil {
			return nil, err
		}
		if node.EntityType, err = recordString(record, "entity_type"); err != nil {
			return nil, err
		}
		if node.Label, err = recordString(record, "label"); err != nil {
			return nil, err
		}
		if node.Detail, err = recordString(record, "detail"); err != nil {
			return nil, err
		}
		if node.RiskLevel, err = recordString(record, "risk_level"); err != nil {
			return nil, err
		}
		if value, valueErr := recordFloat(record, "x"); valueErr != nil {
			return nil, valueErr
		} else {
			node.X = float32(value)
		}
		if value, valueErr := recordFloat(record, "y"); valueErr != nil {
			return nil, valueErr
		} else {
			node.Y = float32(value)
		}
		if node.Icon, err = recordString(record, "icon"); err != nil {
			return nil, err
		}
		if node.UpdatedAt, err = recordInt(record, "updated_at"); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func decodeWorkbenchEdges(result *nebula_go.ResultSet) ([]*query.WorkbenchEdge, error) {
	edges := make([]*query.WorkbenchEdge, 0, result.GetRowSize())
	for index := 0; index < result.GetRowSize(); index++ {
		record, err := result.GetRowValuesByIndex(index)
		if err != nil {
			return nil, fmt.Errorf("decode NebulaGraph edge row %d: %w", index, err)
		}
		attributesJSON, err := recordString(record, "attributes_json")
		if err != nil {
			return nil, fmt.Errorf("decode NebulaGraph edge attributes row %d: %w", index, err)
		}
		attributes := make(map[string]interface{})
		if attributesJSON != "" {
			if err := json.Unmarshal([]byte(attributesJSON), &attributes); err != nil {
				return nil, fmt.Errorf("decode NebulaGraph edge attributes JSON row %d: %w", index, err)
			}
		}
		edge := &query.WorkbenchEdge{Attributes: attributes}
		if edge.RelationID, err = recordString(record, "relation_id"); err != nil {
			return nil, err
		}
		if edge.SourceID, err = recordString(record, "source_id"); err != nil {
			return nil, err
		}
		if edge.TargetID, err = recordString(record, "target_id"); err != nil {
			return nil, err
		}
		if edge.RelationType, err = recordString(record, "relation_type"); err != nil {
			return nil, err
		}
		if edge.RiskLevel, err = recordString(record, "risk_level"); err != nil {
			return nil, err
		}
		if edge.EvidenceID, err = recordString(record, "evidence_id"); err != nil {
			return nil, err
		}
		if value, valueErr := recordFloat(record, "weight"); valueErr != nil {
			return nil, valueErr
		} else {
			edge.Weight = float32(value)
		}
		if edge.ObservedAt, err = recordInt(record, "observed_at"); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func recordValue(record *nebula_go.Record, column string) (*nebula_go.ValueWrapper, error) {
	value, err := record.GetValueByColName(column)
	if err != nil {
		return nil, fmt.Errorf("column %s: %w", column, err)
	}
	if value.IsNull() || value.IsEmpty() {
		return nil, nil
	}
	return value, nil
}

func recordString(record *nebula_go.Record, column string) (string, error) {
	value, err := recordValue(record, column)
	if err != nil || value == nil {
		return "", err
	}
	result, err := value.AsString()
	if err != nil {
		return "", fmt.Errorf("column %s: %w", column, err)
	}
	return result, nil
}

func recordInt(record *nebula_go.Record, column string) (int64, error) {
	value, err := recordValue(record, column)
	if err != nil || value == nil {
		return 0, err
	}
	result, err := value.AsInt()
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", column, err)
	}
	return result, nil
}

func recordFloat(record *nebula_go.Record, column string) (float64, error) {
	value, err := recordValue(record, column)
	if err != nil || value == nil {
		return 0, err
	}
	if value.IsInt() {
		integer, intErr := value.AsInt()
		return float64(integer), intErr
	}
	result, err := value.AsFloat()
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", column, err)
	}
	return result, nil
}
