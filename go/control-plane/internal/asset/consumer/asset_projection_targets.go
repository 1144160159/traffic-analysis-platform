package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/sourcequality"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

type OpenSearchAssetProjection struct {
	client *opensearch.Client
	index  string
}

type ClickHouseAssetProjection struct {
	db            *sql.DB
	table         string
	consumerGroup string
}

type assetSourceFact struct {
	Rail                   string `json:"rail"`
	TenantID               string `json:"tenant_id"`
	AggregateID            string `json:"aggregate_id"`
	EventID                string `json:"event_id"`
	EventTimeMS            int64  `json:"event_time_ms"`
	IngestTimeMS           int64  `json:"ingest_time_ms"`
	SchemaVersion          string `json:"schema_version"`
	SourceTopic            string `json:"source_topic"`
	SourcePartition        int    `json:"source_partition"`
	SourceOffset           int64  `json:"source_offset"`
	SourceTimestampMS      int64  `json:"source_timestamp_ms"`
	SourcePayloadSHA256    string `json:"source_payload_sha256"`
	SourceVersion          int64  `json:"source_version"`
	ProjectionIdentity     string `json:"projection_identity"`
	SourceQualityReceiptID string `json:"source_quality_receipt_id"`
	PayloadBase64          string `json:"payload_base64"`
	ProjectionHash         string `json:"projection_hash"`
}

func NewClickHouseAssetProjection(
	db *sql.DB,
	table string,
	consumerGroup string,
) (*ClickHouseAssetProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("asset projection ClickHouse database is required")
	}
	if strings.TrimSpace(table) != "traffic.source_asset_facts_v1" {
		return nil, fmt.Errorf("asset source facts are pinned to traffic.source_asset_facts_v1")
	}
	if strings.TrimSpace(consumerGroup) == "" {
		return nil, fmt.Errorf("asset projection consumer group is required")
	}
	return &ClickHouseAssetProjection{
		db: db, table: strings.TrimSpace(table), consumerGroup: strings.TrimSpace(consumerGroup),
	}, nil
}

func (p *ClickHouseAssetProjection) Name() string { return assetProjectionClickHouse }

func (p *ClickHouseAssetProjection) Projection(event AssetUpsertedV2) ([]byte, error) {
	if event.SourceTopic != "asset.events.v2" || event.SourcePartition < 0 ||
		event.SourceOffset < 0 || event.SourceTimestamp <= 0 || len(event.RawPayload) == 0 {
		return nil, fmt.Errorf("asset ClickHouse projection requires durable source coordinates")
	}
	if sourcequality.HashSource(event.RawPayload) != event.SourceSHA256 {
		return nil, fmt.Errorf("asset ClickHouse source payload checksum mismatch")
	}
	eventTimeMS := event.Asset.LastSeen.UnixMilli()
	if eventTimeMS <= 0 || event.AggregateVersion <= 0 {
		return nil, fmt.Errorf("asset ClickHouse event time and version must be positive")
	}
	receipt, err := sourcequality.Build(sourcequality.Input{
		TenantID: event.TenantID, Rail: sourcequality.RailAsset,
		ConsumerGroup: p.consumerGroup,
		Source: sourcequality.SourceTuple{
			Topic: event.SourceTopic, Partition: event.SourcePartition, Offset: event.SourceOffset,
		},
		Category: sourcequality.Accepted, EventID: event.EventID,
		SourceSHA256: event.SourceSHA256, WatermarkMS: -1,
		ObservedAtMS: event.SourceTimestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("build asset source-quality receipt identity: %w", err)
	}
	projectionIdentity := sha256HexString(
		"source-fact/v1\x00asset\x00" + event.TenantID + "\x00" + event.EventID)
	fact := assetSourceFact{
		Rail: "asset", TenantID: event.TenantID, AggregateID: event.AssetID,
		EventID: event.EventID, EventTimeMS: eventTimeMS,
		IngestTimeMS: event.SourceTimestamp, SchemaVersion: "v2",
		SourceTopic: event.SourceTopic, SourcePartition: event.SourcePartition,
		SourceOffset: event.SourceOffset, SourceTimestampMS: event.SourceTimestamp,
		SourcePayloadSHA256: event.SourceSHA256, SourceVersion: event.AggregateVersion,
		ProjectionIdentity: projectionIdentity, SourceQualityReceiptID: receipt.ReceiptID,
		PayloadBase64: base64.StdEncoding.EncodeToString(event.RawPayload),
	}
	unsigned, err := json.Marshal(fact)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned asset source fact: %w", err)
	}
	fact.ProjectionHash = sha256HexBytes(unsigned)
	projection, err := json.Marshal(fact)
	if err != nil {
		return nil, fmt.Errorf("marshal asset source fact: %w", err)
	}
	return projection, nil
}

func (p *ClickHouseAssetProjection) Apply(
	ctx context.Context,
	_ AssetUpsertedV2,
	projection []byte,
) error {
	var fact assetSourceFact
	if err := json.Unmarshal(projection, &fact); err != nil {
		return fmt.Errorf("decode asset source fact: %w", err)
	}
	if fact.Rail != "asset" || fact.TenantID == "" || fact.ProjectionIdentity == "" ||
		fact.SourceVersion <= 0 || fact.ProjectionHash == "" {
		return fmt.Errorf("invalid asset source fact projection")
	}
	var count, currentVersion int64
	var currentHash string
	if err := p.db.QueryRowContext(ctx,
		`SELECT count(),if(count()>0,max(source_version),0),`+
			`if(count()>0,argMax(projection_hash,source_version),'') `+
			`FROM traffic.source_asset_facts_v1 WHERE projection_identity=?`,
		fact.ProjectionIdentity,
	).Scan(&count, &currentVersion, &currentHash); err != nil {
		return fmt.Errorf("read asset ClickHouse source-fact version: %w", err)
	}
	if count > 0 {
		switch {
		case currentVersion > fact.SourceVersion:
			return fmt.Errorf("stale asset source-fact version")
		case currentVersion == fact.SourceVersion && currentHash == fact.ProjectionHash:
			return nil
		case currentVersion == fact.SourceVersion:
			return fmt.Errorf("asset source-fact version hash conflict")
		}
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO traffic.source_asset_facts_v1 (
		  rail,tenant_id,aggregate_id,event_id,event_time_ms,ingest_time_ms,
		  schema_version,source_topic,source_partition,source_offset,
		  source_timestamp_ms,source_payload_sha256,source_version,
		  projection_identity,source_quality_receipt_id,payload_base64,projection_hash
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.Rail, fact.TenantID, fact.AggregateID, fact.EventID,
		fact.EventTimeMS, fact.IngestTimeMS, fact.SchemaVersion,
		fact.SourceTopic, fact.SourcePartition, fact.SourceOffset,
		fact.SourceTimestampMS, fact.SourcePayloadSHA256, fact.SourceVersion,
		fact.ProjectionIdentity, fact.SourceQualityReceiptID,
		fact.PayloadBase64, fact.ProjectionHash,
	)
	if err != nil {
		return fmt.Errorf("insert asset ClickHouse source fact: %w", err)
	}
	return nil
}

func (p *ClickHouseAssetProjection) Ready(ctx context.Context) error {
	var exists uint8
	if err := p.db.QueryRowContext(ctx,
		`SELECT count()>0 FROM system.tables WHERE database='traffic' AND name='source_asset_facts_v1'`,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check asset ClickHouse source-fact table: %w", err)
	}
	if exists != 1 {
		return fmt.Errorf("asset ClickHouse source-fact table is absent")
	}
	return nil
}

func sha256HexString(value string) string { return sha256HexBytes([]byte(value)) }

func sha256HexBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

type assetSearchDocument struct {
	ContractVersion string             `json:"contract_version"`
	EventID         string             `json:"event_id"`
	TraceID         string             `json:"trace_id"`
	TenantID        string             `json:"tenant_id"`
	AssetID         string             `json:"asset_id"`
	Revision        int64              `json:"revision"`
	Asset           config.AssetRecord `json:"asset"`
}

func NewOpenSearchAssetProjection(
	addresses []string,
	username string,
	password string,
	index string,
) (*OpenSearchAssetProjection, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("asset projection OpenSearch addresses are required")
	}
	if strings.TrimSpace(index) == "" {
		return nil, fmt.Errorf("asset projection OpenSearch index is required")
	}
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: addresses,
		Username:  username,
		Password:  password,
	})
	if err != nil {
		return nil, fmt.Errorf("create asset projection OpenSearch client: %w", err)
	}
	return &OpenSearchAssetProjection{client: client, index: index}, nil
}

func (p *OpenSearchAssetProjection) Name() string {
	return assetProjectionOpenSearch
}

func (p *OpenSearchAssetProjection) Projection(event AssetUpsertedV2) ([]byte, error) {
	document := assetSearchDocument{
		ContractVersion: "traffic.asset.v2.AssetUpserted",
		EventID:         event.EventID,
		TraceID:         event.TraceID,
		TenantID:        event.TenantID,
		AssetID:         event.AssetID,
		Revision:        event.AggregateVersion,
		Asset:           event.Asset,
	}
	payload, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal asset search document: %w", err)
	}
	return payload, nil
}

func (p *OpenSearchAssetProjection) Apply(
	ctx context.Context,
	event AssetUpsertedV2,
	projection []byte,
) error {
	version := int(event.AggregateVersion)
	request := opensearchapi.IndexRequest{
		Index:       p.index,
		DocumentID:  event.AssetID,
		Body:        bytes.NewReader(projection),
		Refresh:     "false",
		Version:     &version,
		VersionType: "external_gte",
	}
	response, err := request.Do(ctx, p.client)
	if err != nil {
		return fmt.Errorf("index asset search projection: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("index asset search projection: status=%s body=%s", response.Status(), body)
	}
	return nil
}

func (p *OpenSearchAssetProjection) Ready(ctx context.Context) error {
	response, err := p.client.Info(p.client.Info.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("check asset projection OpenSearch readiness: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		return fmt.Errorf("asset projection OpenSearch is not ready: %s", response.Status())
	}
	aliasResponse, err := (opensearchapi.IndicesExistsAliasRequest{
		Name: []string{p.index},
	}).Do(ctx, p.client)
	if err != nil {
		return fmt.Errorf("check asset projection OpenSearch write alias: %w", err)
	}
	defer aliasResponse.Body.Close()
	if aliasResponse.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"asset projection OpenSearch write alias %q is absent: %s",
			p.index,
			aliasResponse.Status(),
		)
	}
	return nil
}

type NebulaAssetProjectionWriter interface {
	UpsertAssetEntity(context.Context, graphNebula.AssetEntityProjection) error
}

type NebulaAssetProjection struct {
	writer NebulaAssetProjectionWriter
}

func NewNebulaAssetProjection(writer NebulaAssetProjectionWriter) (*NebulaAssetProjection, error) {
	if writer == nil {
		return nil, fmt.Errorf("asset projection NebulaGraph writer is required")
	}
	return &NebulaAssetProjection{writer: writer}, nil
}

func (p *NebulaAssetProjection) Name() string {
	return assetProjectionNebula
}

func (p *NebulaAssetProjection) Projection(event AssetUpsertedV2) ([]byte, error) {
	detail := event.Asset.IPAddress
	if event.Asset.MACAddress != "" {
		if detail != "" {
			detail += " / "
		}
		detail += event.Asset.MACAddress
	}
	label := event.Asset.Hostname
	if label == "" {
		label = event.Asset.DisplayCode
	}
	if label == "" {
		label = event.Asset.AssetID
	}
	metadata := make(map[string]any, len(event.Asset.Metadata)+8)
	for key, value := range event.Asset.Metadata {
		metadata[key] = value
	}
	metadata["asset_id"] = event.AssetID
	metadata["revision"] = event.AggregateVersion
	metadata["event_id"] = event.EventID
	metadata["trace_id"] = event.TraceID
	metadata["status"] = event.Asset.Status
	metadata["source"] = event.Asset.Source
	metadata["department"] = event.Asset.Department
	metadata["campus"] = event.Asset.Campus
	metadata["owner"] = event.Asset.Owner
	projection := graphNebula.AssetEntityProjection{
		TenantID:  event.TenantID,
		AssetID:   event.AssetID,
		Label:     label,
		Detail:    detail,
		RiskScore: int64(event.Asset.Criticality * 20),
		RiskLevel: assetRiskLevel(event.Asset.Criticality),
		Icon:      event.Asset.AssetType,
		Metadata:  metadata,
		Revision:  event.AggregateVersion,
		UpdatedAt: event.Asset.LastSeen.UnixMilli(),
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return nil, fmt.Errorf("marshal asset graph projection: %w", err)
	}
	return payload, nil
}

func (p *NebulaAssetProjection) Apply(
	ctx context.Context,
	_ AssetUpsertedV2,
	projection []byte,
) error {
	var entity graphNebula.AssetEntityProjection
	if err := json.Unmarshal(projection, &entity); err != nil {
		return fmt.Errorf("decode asset graph projection: %w", err)
	}
	return p.writer.UpsertAssetEntity(ctx, entity)
}

func assetRiskLevel(criticality int) string {
	switch {
	case criticality >= 4:
		return "high"
	case criticality >= 3:
		return "medium"
	default:
		return "low"
	}
}
