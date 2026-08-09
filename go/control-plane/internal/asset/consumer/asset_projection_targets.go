package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

type OpenSearchAssetProjection struct {
	client *opensearch.Client
	index  string
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
