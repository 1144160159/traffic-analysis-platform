package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

const campaignProjectionContractVersion = "traffic.campaign.projection.v2"

type campaignProjectionDocument struct {
	ContractVersion   string          `json:"contract_version"`
	ProjectionID      string          `json:"projection_id"`
	ProjectionKey     string          `json:"projection_key"`
	ProjectionVersion int64           `json:"projection_version"`
	Stream            string          `json:"stream"`
	EventID           string          `json:"event_id"`
	TenantID          string          `json:"tenant_id"`
	CampaignID        string          `json:"campaign_id"`
	RelationID        string          `json:"relation_id,omitempty"`
	AlertID           string          `json:"alert_id,omitempty"`
	EventType         string          `json:"event_type"`
	SchemaVersion     int             `json:"schema_version"`
	AggregateRevision int64           `json:"aggregate_revision"`
	RelationRevision  int64           `json:"relation_revision"`
	PartitionKey      string          `json:"partition_key"`
	TraceID           string          `json:"trace_id"`
	ReceivedAt        string          `json:"received_at"`
	Payload           json.RawMessage `json:"payload"`
}

func renderCampaignProjectionDocument(event CampaignProjectionEvent) ([]byte, error) {
	if err := validateDurableCampaignProjection(event); err != nil {
		return nil, err
	}
	payload, err := canonicalCampaignProjectionJSON(event.Payload)
	if err != nil {
		return nil, fmt.Errorf("canonicalize campaign projection payload: %w", err)
	}
	document := campaignProjectionDocument{
		ContractVersion:   campaignProjectionContractVersion,
		ProjectionID:      campaignProjectionEventID(event),
		ProjectionKey:     event.ProjectionKey(),
		ProjectionVersion: event.ProjectionVersion(),
		Stream:            event.Stream,
		EventID:           event.EventID,
		TenantID:          event.TenantID,
		CampaignID:        event.CampaignID,
		RelationID:        event.RelationID,
		AlertID:           event.AlertID,
		EventType:         event.EventType,
		SchemaVersion:     event.SchemaVersion,
		AggregateRevision: event.AggregateRevision,
		RelationRevision:  event.RelationRevision,
		PartitionKey:      event.PartitionKey,
		TraceID:           event.TraceID,
		ReceivedAt:        event.ReceivedAt.UTC().Format(time.RFC3339Nano),
		Payload:           payload,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal campaign projection document: %w", err)
	}
	return encoded, nil
}

func canonicalCampaignProjectionJSON(payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func campaignProjectionEventID(event CampaignProjectionEvent) string {
	return campaignProjectionHash(event.TenantID, event.Stream, event.EventID)
}

func campaignProjectionStateID(event CampaignProjectionEvent) string {
	return campaignProjectionHash(event.TenantID, event.ProjectionKey())
}

func campaignProjectionHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(fmt.Sprintf("%d:", len(part))))
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type campaignClickHouseClient interface {
	Exec(context.Context, string, ...interface{}) error
	Ping(context.Context) error
	TableExists(context.Context, string, string) (bool, error)
}

type storageCampaignClickHouseClient struct {
	client *storage.ClickHouseClient
}

func (client storageCampaignClickHouseClient) Exec(ctx context.Context, query string, args ...interface{}) error {
	return client.client.Exec(ctx, query, args...)
}

func (client storageCampaignClickHouseClient) Ping(ctx context.Context) error {
	return client.client.Ping(ctx)
}

func (client storageCampaignClickHouseClient) TableExists(
	ctx context.Context,
	database string,
	table string,
) (bool, error) {
	row, err := client.client.QueryRow(ctx, `
		SELECT count()=1
		FROM system.tables
		WHERE database=? AND name=?`, database, table)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

type ClickHouseCampaignProjection struct {
	client         campaignClickHouseClient
	database       string
	table          string
	qualifiedTable string
}

func NewClickHouseCampaignProjection(
	client *storage.ClickHouseClient,
	qualifiedTable string,
) (*ClickHouseCampaignProjection, error) {
	if client == nil {
		return nil, fmt.Errorf("campaign projection ClickHouse client is required")
	}
	return newClickHouseCampaignProjection(storageCampaignClickHouseClient{client: client}, qualifiedTable)
}

func newClickHouseCampaignProjection(
	client campaignClickHouseClient,
	qualifiedTable string,
) (*ClickHouseCampaignProjection, error) {
	if client == nil {
		return nil, fmt.Errorf("campaign projection ClickHouse client is required")
	}
	database, table, err := parseCampaignClickHouseTable(qualifiedTable)
	if err != nil {
		return nil, err
	}
	return &ClickHouseCampaignProjection{
		client: client, database: database, table: table, qualifiedTable: database + "." + table,
	}, nil
}

func parseCampaignClickHouseTable(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 2 || !campaignProjectionIdentifier(parts[0]) || !campaignProjectionIdentifier(parts[1]) {
		return "", "", fmt.Errorf("campaign projection ClickHouse table must be a qualified identifier")
	}
	return parts[0], parts[1], nil
}

func campaignProjectionIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func (target *ClickHouseCampaignProjection) Name() string { return campaignProjectionClickHouse }

func (target *ClickHouseCampaignProjection) Projection(event CampaignProjectionEvent) ([]byte, error) {
	return renderCampaignProjectionDocument(event)
}

func (target *ClickHouseCampaignProjection) Apply(
	ctx context.Context,
	event CampaignProjectionEvent,
	projection []byte,
) error {
	var document campaignProjectionDocument
	if err := json.Unmarshal(projection, &document); err != nil {
		return fmt.Errorf("decode ClickHouse campaign projection: %w", err)
	}
	projectionHash := sha256.Sum256(projection)
	query := fmt.Sprintf(`INSERT INTO %s
		(projection_id,event_id,tenant_id,stream,projection_key,projection_version,
		 campaign_id,relation_id,alert_id,event_type,schema_version,trace_id,received_at,
		 payload,projection_sha256)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, target.qualifiedTable)
	if err := target.client.Exec(ctx, query,
		document.ProjectionID,
		event.EventID,
		event.TenantID,
		event.Stream,
		event.ProjectionKey(),
		event.ProjectionVersion(),
		event.CampaignID,
		event.RelationID,
		event.AlertID,
		event.EventType,
		event.SchemaVersion,
		event.TraceID,
		event.ReceivedAt.UTC(),
		string(document.Payload),
		hex.EncodeToString(projectionHash[:]),
	); err != nil {
		return fmt.Errorf("insert ClickHouse campaign projection: %w", err)
	}
	return nil
}

func (target *ClickHouseCampaignProjection) Ready(ctx context.Context) error {
	if err := target.client.Ping(ctx); err != nil {
		return fmt.Errorf("campaign projection ClickHouse is not ready: %w", err)
	}
	exists, err := target.client.TableExists(ctx, target.database, target.table)
	if err != nil {
		return fmt.Errorf("check campaign projection ClickHouse table: %w", err)
	}
	if !exists {
		return fmt.Errorf("campaign projection ClickHouse table %q is absent", target.qualifiedTable)
	}
	return nil
}

type OpenSearchCampaignProjection struct {
	client *opensearch.Client
	alias  string
}

func NewOpenSearchCampaignProjection(
	addresses []string,
	username string,
	password string,
	writeAlias string,
) (*OpenSearchCampaignProjection, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("campaign projection OpenSearch addresses are required")
	}
	if strings.TrimSpace(writeAlias) == "" {
		return nil, fmt.Errorf("campaign projection OpenSearch write alias is required")
	}
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: addresses,
		Username:  username,
		Password:  password,
	})
	if err != nil {
		return nil, fmt.Errorf("create campaign projection OpenSearch client: %w", err)
	}
	return &OpenSearchCampaignProjection{client: client, alias: writeAlias}, nil
}

func (target *OpenSearchCampaignProjection) Name() string { return campaignProjectionOpenSearch }

func (target *OpenSearchCampaignProjection) Projection(event CampaignProjectionEvent) ([]byte, error) {
	return renderCampaignProjectionDocument(event)
}

func (target *OpenSearchCampaignProjection) Apply(
	ctx context.Context,
	event CampaignProjectionEvent,
	projection []byte,
) error {
	version := int(event.ProjectionVersion())
	if version <= 0 || int64(version) != event.ProjectionVersion() {
		return fmt.Errorf("campaign projection version exceeds OpenSearch external version range")
	}
	request := opensearchapi.IndexRequest{
		Index:       target.alias,
		DocumentID:  campaignProjectionStateID(event),
		Body:        bytes.NewReader(projection),
		Refresh:     "false",
		Version:     &version,
		VersionType: "external_gte",
	}
	response, err := request.Do(ctx, target.client)
	if err != nil {
		return fmt.Errorf("index OpenSearch campaign projection: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("index OpenSearch campaign projection: status=%s body=%s", response.Status(), body)
	}
	return nil
}

func (target *OpenSearchCampaignProjection) Ready(ctx context.Context) error {
	response, err := target.client.Info(target.client.Info.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("check campaign projection OpenSearch readiness: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		return fmt.Errorf("campaign projection OpenSearch is not ready: %s", response.Status())
	}
	aliasResponse, err := (opensearchapi.IndicesExistsAliasRequest{Name: []string{target.alias}}).Do(ctx, target.client)
	if err != nil {
		return fmt.Errorf("check campaign projection OpenSearch alias: %w", err)
	}
	defer aliasResponse.Body.Close()
	if aliasResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("campaign projection OpenSearch write alias %q is absent: %s", target.alias, aliasResponse.Status())
	}
	return nil
}

type NebulaCampaignProjectionWriter interface {
	Ready(context.Context) error
	UpsertCampaignEntity(context.Context, graphNebula.CampaignEntityProjection) error
	ApplyCampaignMembership(context.Context, graphNebula.CampaignMembershipProjection) error
}

type NebulaCampaignProjection struct {
	writer NebulaCampaignProjectionWriter
}

func NewNebulaCampaignProjection(writer NebulaCampaignProjectionWriter) (*NebulaCampaignProjection, error) {
	if writer == nil {
		return nil, fmt.Errorf("campaign projection NebulaGraph writer is required")
	}
	return &NebulaCampaignProjection{writer: writer}, nil
}

func (target *NebulaCampaignProjection) Name() string { return campaignProjectionNebula }

func (target *NebulaCampaignProjection) Projection(event CampaignProjectionEvent) ([]byte, error) {
	return renderCampaignProjectionDocument(event)
}

func (target *NebulaCampaignProjection) Apply(
	ctx context.Context,
	event CampaignProjectionEvent,
	projection []byte,
) error {
	var document campaignProjectionDocument
	if err := json.Unmarshal(projection, &document); err != nil {
		return fmt.Errorf("decode NebulaGraph campaign projection: %w", err)
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(document.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode NebulaGraph campaign payload: %w", err)
	}
	projectionHash := sha256.Sum256(projection)
	metadata := map[string]interface{}{
		"contract_version":   document.ContractVersion,
		"projection_id":      document.ProjectionID,
		"projection_key":     document.ProjectionKey,
		"projection_version": document.ProjectionVersion,
		"projection_sha256":  hex.EncodeToString(projectionHash[:]),
		"event_id":           document.EventID,
		"event_type":         document.EventType,
		"trace_id":           document.TraceID,
		"received_at":        document.ReceivedAt,
		"payload":            payload,
	}
	if event.Stream == campaignAggregateStream {
		return target.writer.UpsertCampaignEntity(ctx, graphNebula.CampaignEntityProjection{
			TenantID:    event.TenantID,
			CampaignID:  event.CampaignID,
			Label:       event.CampaignID,
			Status:      campaignProjectionString(payload, "status"),
			Assignee:    campaignProjectionString(payload, "assignee"),
			MemberCount: campaignProjectionInt64(payload, "member_count"),
			Metadata:    metadata,
			Revision:    event.AggregateRevision,
			UpdatedAt:   event.ReceivedAt.UnixMilli(),
		})
	}
	return target.writer.ApplyCampaignMembership(ctx, graphNebula.CampaignMembershipProjection{
		TenantID:         event.TenantID,
		RelationID:       event.RelationID,
		CampaignID:       event.CampaignID,
		AlertID:          event.AlertID,
		Linked:           event.EventType == "traffic.campaign.v2.AlertLinked",
		Metadata:         metadata,
		Revision:         event.RelationRevision,
		CampaignRevision: event.AggregateRevision,
		ObservedAt:       event.ReceivedAt.UnixMilli(),
	})
}

func (target *NebulaCampaignProjection) Ready(ctx context.Context) error {
	if err := target.writer.Ready(ctx); err != nil {
		return fmt.Errorf("campaign projection NebulaGraph is not ready: %w", err)
	}
	return nil
}

func campaignProjectionString(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return value
}

func campaignProjectionInt64(payload map[string]interface{}, key string) int64 {
	switch value := payload[key].(type) {
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	case float64:
		return int64(value)
	case int64:
		return value
	default:
		return 0
	}
}
