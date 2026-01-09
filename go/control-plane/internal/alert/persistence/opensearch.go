////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/persistence/opensearch.go
// 修复版：确保 count 字段在索引映射中
////////////////////////////////////////////////////////////////////////////////

package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// OpenSearchWriter OpenSearch写入器
type OpenSearchWriter struct {
	client    *opensearch.Client
	indexName string
	logger    *zap.Logger
	mu        sync.RWMutex
	closed    bool
}

// NewOpenSearchWriter 创建OpenSearch写入器
func NewOpenSearchWriter(addrs []string, username, password, indexName string, logger *zap.Logger) (*OpenSearchWriter, error) {
	cfg := opensearch.Config{
		Addresses: addrs,
		Username:  username,
		Password:  password,
		Transport: &retryTransport{
			base:       http.DefaultTransport,
			maxRetries: 3,
			retryDelay: 100 * time.Millisecond,
		},
	}

	client, err := opensearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create opensearch client: %w", err)
	}

	// 测试连接
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to opensearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("opensearch info error: %s", res.Status())
	}

	logger.Info("Connected to OpenSearch",
		zap.Strings("addresses", addrs),
		zap.String("index", indexName))

	w := &OpenSearchWriter{
		client:    client,
		indexName: indexName,
		logger:    logger,
	}

	// 确保索引模板存在
	if err := w.EnsureIndex(context.Background()); err != nil {
		logger.Warn("Failed to ensure index template", zap.Error(err))
	}

	return w, nil
}

// retryTransport 带重试的传输层
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	retryDelay time.Duration
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(t.retryDelay * time.Duration(attempt))
		}

		// Clone request for retry
		reqCopy := req.Clone(req.Context())
		if req.Body != nil {
			// For retries, we need to reset the body
			if req.GetBody != nil {
				reqCopy.Body, _ = req.GetBody()
			}
		}

		resp, err = t.base.RoundTrip(reqCopy)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}
	}

	return resp, err
}

// WriteAlert 写入单个告警
func (w *OpenSearchWriter) WriteAlert(ctx context.Context, alert *Alert) error {
	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		return fmt.Errorf("writer is closed")
	}
	w.mu.RUnlock()

	ctx, span := otel.StartSpan(ctx, "opensearch_writer.write_alert")
	defer span.End()

	indexName := fmt.Sprintf("%s-%s", w.indexName, alert.FirstSeen.Format("2006-01-02"))

	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	start := time.Now()
	req := opensearchapi.IndexRequest{
		Index:      indexName,
		DocumentID: alert.AlertID,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, w.client)
	if err != nil {
		w.logger.Error("Failed to write alert to OpenSearch",
			zap.String("alert_id", alert.AlertID),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return fmt.Errorf("index request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		w.logger.Error("OpenSearch index error",
			zap.String("status", res.Status()),
			zap.ByteString("body", bodyBytes))
		return fmt.Errorf("opensearch error: %s", res.Status())
	}

	w.logger.Debug("Alert written to OpenSearch",
		zap.String("alert_id", alert.AlertID),
		zap.String("index", indexName),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// WriteBatch 批量写入告警
func (w *OpenSearchWriter) WriteBatch(ctx context.Context, alerts []*Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		return fmt.Errorf("writer is closed")
	}
	w.mu.RUnlock()

	ctx, span := otel.StartSpan(ctx, "opensearch_writer.write_batch")
	defer span.End()

	start := time.Now()

	var buf bytes.Buffer
	for _, alert := range alerts {
		indexName := fmt.Sprintf("%s-%s", w.indexName, alert.FirstSeen.Format("2006-01-02"))

		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    alert.AlertID,
			},
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			w.logger.Error("Failed to marshal meta", zap.Error(err))
			continue
		}
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		docBytes, err := json.Marshal(alert)
		if err != nil {
			w.logger.Error("Failed to marshal alert",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
			continue
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body:    bytes.NewReader(buf.Bytes()),
		Refresh: "false",
	}

	res, err := req.Do(ctx, w.client)
	if err != nil {
		w.logger.Error("Bulk request failed",
			zap.Int("count", len(alerts)),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err))
		otel.RecordError(ctx, err)
		return fmt.Errorf("bulk request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		w.logger.Error("Bulk response error",
			zap.String("status", res.Status()),
			zap.ByteString("body", bodyBytes))
		return fmt.Errorf("bulk response error: %s", res.Status())
	}

	// 检查是否有部分失败
	var bulkResp struct {
		Errors bool `json:"errors"`
		Items  []struct {
			Index struct {
				ID     string      `json:"_id"`
				Status int         `json:"status"`
				Error  interface{} `json:"error,omitempty"`
			} `json:"index"`
		} `json:"items"`
	}

	if err := json.NewDecoder(res.Body).Decode(&bulkResp); err == nil && bulkResp.Errors {
		errorCount := 0
		for _, item := range bulkResp.Items {
			if item.Index.Error != nil {
				errorCount++
				w.logger.Warn("Bulk item error",
					zap.String("id", item.Index.ID),
					zap.Int("status", item.Index.Status))
			}
		}
		w.logger.Warn("Bulk write had partial failures",
			zap.Int("total", len(alerts)),
			zap.Int("errors", errorCount))
	}

	w.logger.Info("Batch write completed",
		zap.Int("count", len(alerts)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// Ping 健康检查
func (w *OpenSearchWriter) Ping(ctx context.Context) error {
	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		return fmt.Errorf("writer is closed")
	}
	w.mu.RUnlock()

	res, err := w.client.Ping(w.client.Ping.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ping error: %s", res.Status())
	}

	return nil
}

// Close 关闭连接
func (w *OpenSearchWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	w.logger.Info("OpenSearch writer closed")
	// OpenSearch client不需要显式关闭
	return nil
}

// EnsureIndex 确保索引存在（创建索引模板）
func (w *OpenSearchWriter) EnsureIndex(ctx context.Context) error {
	// 创建索引模板，包含 count 字段
	template := map[string]interface{}{
		"index_patterns": []string{w.indexName + "-*"},
		"template": map[string]interface{}{
			"settings": map[string]interface{}{
				"number_of_shards":   3,
				"number_of_replicas": 1,
				"refresh_interval":   "5s",
			},
			"mappings": map[string]interface{}{
				"properties": map[string]interface{}{
					"tenant_id":      map[string]string{"type": "keyword"},
					"alert_id":       map[string]string{"type": "keyword"},
					"fingerprint":    map[string]string{"type": "keyword"},
					"community_id":   map[string]string{"type": "keyword"},
					"session_id":     map[string]string{"type": "keyword"},
					"campaign_id":    map[string]string{"type": "keyword"},
					"src_ip":         map[string]string{"type": "ip"},
					"dst_ip":         map[string]string{"type": "ip"},
					"src_port":       map[string]string{"type": "integer"},
					"dst_port":       map[string]string{"type": "integer"},
					"protocol":       map[string]string{"type": "short"},
					"alert_type":     map[string]string{"type": "keyword"},
					"labels":         map[string]string{"type": "keyword"},
					"score":          map[string]string{"type": "float"},
					"severity":       map[string]string{"type": "keyword"},
					"first_seen":     map[string]string{"type": "date"},
					"last_seen":      map[string]string{"type": "date"},
					"count":          map[string]string{"type": "integer"}, // 添加 count 字段
					"status":         map[string]string{"type": "keyword"},
					"assignee":       map[string]string{"type": "keyword"},
					"updated_ts":     map[string]string{"type": "date"},
					"model_version":  map[string]string{"type": "keyword"},
					"rule_version":   map[string]string{"type": "keyword"},
					"feature_set_id": map[string]string{"type": "keyword"},
					"evidence_ids":   map[string]string{"type": "keyword"},
					"event_id":       map[string]string{"type": "keyword"},
				},
			},
		},
	}

	body, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	req := opensearchapi.IndicesPutIndexTemplateRequest{
		Name: w.indexName + "-template",
		Body: bytes.NewReader(body),
	}

	res, err := req.Do(ctx, w.client)
	if err != nil {
		return fmt.Errorf("failed to create index template: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return fmt.Errorf("index template error: %s - %s", res.Status(), string(bodyBytes))
	}

	w.logger.Info("Index template created/updated",
		zap.String("template", w.indexName+"-template"))

	return nil
}
