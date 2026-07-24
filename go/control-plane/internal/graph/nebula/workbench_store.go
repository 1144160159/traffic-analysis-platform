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

// WorkbenchStore serves the entity workbench directly from NebulaGraph using
// the official Go SDK. The session pool is bound to one graph space and user,
// which keeps credentials and space selection out of individual queries.
type WorkbenchStore struct {
	pool   *nebula_go.SessionPool
	logger *zap.Logger
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
