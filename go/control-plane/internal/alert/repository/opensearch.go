package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// OpenSearchRepository OpenSearch数据访问层
type OpenSearchRepository struct {
	client    *opensearch.Client
	indexName string
	logger    *zap.Logger
}

// OpenSearchConfig OpenSearch配置
type OpenSearchConfig struct {
	Addresses []string
	Username  string
	Password  string
	IndexName string
}

// NewOpenSearchRepository 创建OpenSearch Repository
func NewOpenSearchRepository(cfg OpenSearchConfig, logger *zap.Logger) (*OpenSearchRepository, error) {
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create opensearch client: %w", err)
	}

	// This repository is used by request handlers; startup should not hang forever
	// if OpenSearch is slow after the primary persistence client has connected.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := opensearchapi.InfoRequest{}.Do(ctx, client)
	if err != nil {
		logger.Warn("OpenSearch repository ping failed, continuing with lazy checks", zap.Error(err))
	} else {
		res.Body.Close()
	}

	return &OpenSearchRepository{
		client:    client,
		indexName: cfg.IndexName,
		logger:    logger,
	}, nil
}

// SearchQuery 搜索查询参数
type SearchQuery struct {
	TenantID   string
	Query      string   // 全文搜索词
	Severity   []string // 严重程度过滤
	Status     []string // 状态过滤
	AlertTypes []string // 告警类型过滤
	Labels     []string // 标签过滤
	SrcIP      string
	DstIP      string
	StartTime  time.Time
	EndTime    time.Time
	From       int
	Size       int
	SortField  string
	SortOrder  string
}

// SearchResult 搜索结果
type SearchResult struct {
	Alerts       []*persistence.Alert   `json:"alerts"`
	Total        int64                  `json:"total"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
	Took         int                    `json:"took"` // 耗时(ms)
}

// Search 全文搜索告警
func (r *OpenSearchRepository) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.search")
	defer span.End()

	// 构建查询
	must := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"tenant_id": query.TenantID,
			},
		},
	}

	// 全文搜索
	if query.Query != "" {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     query.Query,
				"fields":    []string{"alert_type^3", "labels^2", "src_ip", "dst_ip", "community_id"},
				"type":      "best_fields",
				"fuzziness": "AUTO",
			},
		})
	}

	// 时间范围
	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			rangeQuery["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			rangeQuery["lte"] = query.EndTime.Format(time.RFC3339)
		}
		must = append(must, map[string]interface{}{
			"range": map[string]interface{}{
				"last_seen": rangeQuery,
			},
		})
	}

	// 过滤条件
	filter := []map[string]interface{}{}

	if len(query.Severity) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"severity": query.Severity,
			},
		})
	}

	if len(query.Status) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"status": query.Status,
			},
		})
	}

	if len(query.AlertTypes) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"alert_type": query.AlertTypes,
			},
		})
	}

	if len(query.Labels) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"labels": query.Labels,
			},
		})
	}

	if query.SrcIP != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"src_ip": query.SrcIP,
			},
		})
	}

	if query.DstIP != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"dst_ip": query.DstIP,
			},
		})
	}

	// 构建完整查询
	boolQuery := map[string]interface{}{
		"must": must,
	}
	if len(filter) > 0 {
		boolQuery["filter"] = filter
	}

	// 排序
	sortField := "last_seen"
	sortOrder := "desc"
	if query.SortField != "" {
		sortField = query.SortField
	}
	if query.SortOrder != "" {
		sortOrder = query.SortOrder
	}

	// 分页
	from := query.From
	size := query.Size
	if size <= 0 || size > 1000 {
		size = 50
	}

	// 完整请求体
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": boolQuery,
		},
		"sort": []map[string]interface{}{
			{
				sortField: map[string]interface{}{
					"order": sortOrder,
				},
			},
		},
		"from": from,
		"size": size,
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"alert_type": map[string]interface{}{},
				"labels":     map[string]interface{}{},
			},
		},
		"aggs": map[string]interface{}{
			"severity_count": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "severity",
				},
			},
			"status_count": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "status",
				},
			},
			"alert_type_count": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "alert_type",
					"size":  10,
				},
			},
		},
	}

	// 序列化请求
	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal search query")
	}

	// 执行搜索
	indexPattern := fmt.Sprintf("%s-*", r.indexName)
	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(indexPattern),
		r.client.Search.WithBody(bytes.NewReader(body)),
		r.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		r.logger.Error("OpenSearch search failed", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "search failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		r.logger.Error("OpenSearch search error",
			zap.String("status", res.Status()),
			zap.ByteString("body", bodyBytes))
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "search error: %s", res.Status())
	}

	// 解析响应
	var response struct {
		Took int `json:"took"`
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source    persistence.Alert   `json:"_source"`
				Highlight map[string][]string `json:"highlight,omitempty"`
			} `json:"hits"`
		} `json:"hits"`
		Aggregations map[string]interface{} `json:"aggregations,omitempty"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode response")
	}

	// 构建结果
	alerts := make([]*persistence.Alert, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		alert := hit.Source
		alerts = append(alerts, &alert)
	}

	return &SearchResult{
		Alerts:       alerts,
		Total:        response.Hits.Total.Value,
		Aggregations: response.Aggregations,
		Took:         response.Took,
	}, nil
}

// Suggest 自动补全建议
func (r *OpenSearchRepository) Suggest(ctx context.Context, tenantID, prefix string, field string, size int) ([]string, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.suggest")
	defer span.End()

	if size <= 0 || size > 20 {
		size = 10
	}

	// 使用prefix查询实现简单补全
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"tenant_id": tenantID,
						},
					},
					{
						"prefix": map[string]interface{}{
							field: map[string]interface{}{
								"value": prefix,
							},
						},
					},
				},
			},
		},
		"aggs": map[string]interface{}{
			"suggestions": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": field,
					"size":  size,
				},
			},
		},
		"size": 0,
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal suggest query")
	}

	indexPattern := fmt.Sprintf("%s-*", r.indexName)
	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(indexPattern),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "suggest failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "suggest error: %s", res.Status())
	}

	var response struct {
		Aggregations struct {
			Suggestions struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int    `json:"doc_count"`
				} `json:"buckets"`
			} `json:"suggestions"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode response")
	}

	suggestions := make([]string, 0, len(response.Aggregations.Suggestions.Buckets))
	for _, bucket := range response.Aggregations.Suggestions.Buckets {
		suggestions = append(suggestions, bucket.Key)
	}

	return suggestions, nil
}

// AggregateQuery 聚合查询参数
type AggregateQuery struct {
	TenantID  string
	Field     string // 聚合字段
	StartTime time.Time
	EndTime   time.Time
	Size      int
}

// AggregateResult 聚合结果
type AggregateResult struct {
	Buckets []AggBucket `json:"buckets"`
}

// AggBucket 聚合桶
type AggBucket struct {
	Key      string `json:"key"`
	DocCount int64  `json:"doc_count"`
}

// Aggregate 聚合统计
func (r *OpenSearchRepository) Aggregate(ctx context.Context, query *AggregateQuery) (*AggregateResult, error) {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.aggregate")
	defer span.End()

	if query.Size <= 0 || query.Size > 100 {
		query.Size = 20
	}

	// 构建查询
	must := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"tenant_id": query.TenantID,
			},
		},
	}

	if !query.StartTime.IsZero() || !query.EndTime.IsZero() {
		rangeQuery := map[string]interface{}{}
		if !query.StartTime.IsZero() {
			rangeQuery["gte"] = query.StartTime.Format(time.RFC3339)
		}
		if !query.EndTime.IsZero() {
			rangeQuery["lte"] = query.EndTime.Format(time.RFC3339)
		}
		must = append(must, map[string]interface{}{
			"range": map[string]interface{}{
				"last_seen": rangeQuery,
			},
		})
	}

	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": must,
			},
		},
		"aggs": map[string]interface{}{
			"field_agg": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": query.Field,
					"size":  query.Size,
				},
			},
		},
		"size": 0,
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal aggregate query")
	}

	indexPattern := fmt.Sprintf("%s-*", r.indexName)
	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(indexPattern),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOpenSearchError, "aggregate failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, errors.Newf(errors.ErrCodeOpenSearchError, "aggregate error: %s", res.Status())
	}

	var response struct {
		Aggregations struct {
			FieldAgg struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"field_agg"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode response")
	}

	buckets := make([]AggBucket, 0, len(response.Aggregations.FieldAgg.Buckets))
	for _, b := range response.Aggregations.FieldAgg.Buckets {
		buckets = append(buckets, AggBucket{
			Key:      b.Key,
			DocCount: b.DocCount,
		})
	}

	return &AggregateResult{Buckets: buckets}, nil
}

// Index 索引单个告警
func (r *OpenSearchRepository) Index(ctx context.Context, alert *persistence.Alert) error {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.index")
	defer span.End()

	indexName := fmt.Sprintf("%s-%s", r.indexName, alert.FirstSeen.Format("2006-01-02"))

	body, err := json.Marshal(alert)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal alert")
	}

	req := opensearchapi.IndexRequest{
		Index:      indexName,
		DocumentID: alert.AlertID,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		r.logger.Error("Failed to index alert", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "index failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Newf(errors.ErrCodeOpenSearchError, "index error: %s", res.Status())
	}

	return nil
}

// BulkIndex 批量索引告警
func (r *OpenSearchRepository) BulkIndex(ctx context.Context, alerts []*persistence.Alert) error {
	ctx, span := otel.StartSpan(ctx, "opensearch_repository.bulk_index")
	defer span.End()

	if len(alerts) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, alert := range alerts {
		indexName := fmt.Sprintf("%s-%s", r.indexName, alert.FirstSeen.Format("2006-01-02"))

		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    alert.AlertID,
			},
		}

		metaBytes, _ := json.Marshal(meta)
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		docBytes, _ := json.Marshal(alert)
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body:    bytes.NewReader(buf.Bytes()),
		Refresh: "false",
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		r.logger.Error("Bulk index failed", zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeOpenSearchError, "bulk index failed")
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		r.logger.Error("Bulk index error",
			zap.String("status", res.Status()),
			zap.ByteString("body", bodyBytes))
		return errors.Newf(errors.ErrCodeOpenSearchError, "bulk index error: %s", res.Status())
	}

	// 检查是否有部分失败
	var bulkResp struct {
		Errors bool `json:"errors"`
		Items  []struct {
			Index struct {
				Error interface{} `json:"error,omitempty"`
			} `json:"index"`
		} `json:"items"`
	}

	if err := json.NewDecoder(res.Body).Decode(&bulkResp); err == nil && bulkResp.Errors {
		errorCount := 0
		for _, item := range bulkResp.Items {
			if item.Index.Error != nil {
				errorCount++
			}
		}
		r.logger.Warn("Bulk index had errors",
			zap.Int("total", len(alerts)),
			zap.Int("errors", errorCount))
	}

	return nil
}

// Ping 健康检查
func (r *OpenSearchRepository) Ping(ctx context.Context) error {
	res, err := r.client.Ping(r.client.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ping failed: %s", res.Status())
	}
	return nil
}

// Close 关闭连接（OpenSearch client不需要显式关闭）
func (r *OpenSearchRepository) Close() error {
	return nil
}
