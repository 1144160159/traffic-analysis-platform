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
	"strings"
	"sync"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/opensearchbulk"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// OpenSearchWriter OpenSearch写入器
type OpenSearchWriter struct {
	client      *opensearch.Client
	readTarget  string
	writeTarget string
	exactTarget bool
	// legacyReadKeywordFields is only used while reading a frozen legacy
	// dynamic-mapping index. V2 aliases expose first-class keyword fields and
	// must keep this disabled.
	legacyReadKeywordFields bool
	logger                  *zap.Logger
	mu                      sync.RWMutex
	closed                  bool
}

// NewOpenSearchWriter 创建OpenSearch写入器
func NewOpenSearchWriter(addrs []string, username, password, writeTarget string, exactTarget bool, logger *zap.Logger) (*OpenSearchWriter, error) {
	readTarget := writeTarget
	if !exactTarget {
		readTarget += "-*"
	}
	return newOpenSearchWriter(addrs, username, password, readTarget, writeTarget, exactTarget, !exactTarget, logger)
}

// NewOpenSearchReconcileTarget decouples the projection read target from the
// write target. This is required during migration: the frozen legacy read
// index can be an exact name such as "alerts", while approved repair writes
// must still target the versioned V2 write alias. The constructor performs no
// schema or alias mutations.
func NewOpenSearchReconcileTarget(addrs []string, username, password, readTarget, writeTarget string, exactWriteTarget, legacyReadKeywordFields bool, logger *zap.Logger) (*OpenSearchWriter, error) {
	if strings.TrimSpace(readTarget) == "" {
		return nil, fmt.Errorf("opensearch read target is required")
	}
	return newOpenSearchWriter(addrs, username, password, readTarget, writeTarget, exactWriteTarget, legacyReadKeywordFields, logger)
}

func newOpenSearchWriter(addrs []string, username, password, readTarget, writeTarget string, exactTarget, legacyReadKeywordFields bool, logger *zap.Logger) (*OpenSearchWriter, error) {
	if writeTarget == "" {
		return nil, fmt.Errorf("opensearch write target is required")
	}
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
		zap.String("read_target", readTarget),
		zap.String("write_target", writeTarget),
		zap.Bool("exact_target", exactTarget),
		zap.Bool("legacy_read_keyword_fields", legacyReadKeywordFields))

	w := &OpenSearchWriter{
		client:                  client,
		readTarget:              readTarget,
		writeTarget:             writeTarget,
		exactTarget:             exactTarget,
		legacyReadKeywordFields: legacyReadKeywordFields,
		logger:                  logger,
	}

	return w, nil
}

func (w *OpenSearchWriter) targetFor(firstSeen time.Time) string {
	if w.exactTarget {
		return w.writeTarget
	}
	return fmt.Sprintf("%s-%s", w.writeTarget, firstSeen.Format("2006-01-02"))
}

// TargetVersion identifies the exact logical projection generation recorded in
// durable debt and reconcile evidence. It is never inferred from "latest".
func (w *OpenSearchWriter) TargetVersion() string {
	if w.exactTarget {
		return w.writeTarget
	}
	return defaultAlertProjectionTargetVersion
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

	indexName := w.targetFor(alert.FirstSeen)

	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	start := time.Now()
	version := int(AlertSourceVersion(alert))
	req := opensearchapi.IndexRequest{
		Index:       indexName,
		DocumentID:  alert.AlertID,
		Body:        bytes.NewReader(body),
		Refresh:     "false",
		Version:     &version,
		VersionType: "external_gte",
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
		indexName := w.targetFor(alert.FirstSeen)

		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index":       indexName,
				"_id":          alert.AlertID,
				"version":      AlertSourceVersion(alert),
				"version_type": "external_gte",
			},
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("marshal bulk metadata for alert %s: %w", alert.AlertID, err)
		}
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		docBytes, err := json.Marshal(alert)
		if err != nil {
			return fmt.Errorf("marshal alert %s for bulk write: %w", alert.AlertID, err)
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

	if err := opensearchbulk.DecodeSuccess(res.Body, len(alerts)); err != nil {
		w.logger.Error("Bulk write was not fully acknowledged", zap.Error(err))
		otel.RecordError(ctx, err)
		return err
	}

	w.logger.Info("Batch write completed",
		zap.Int("count", len(alerts)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

type openSearchProjectionSource struct {
	Alert
	DedupFingerprint string `json:"dedup_fingerprint"`
}

func (s openSearchProjectionSource) canonicalAlert(legacy bool) Alert {
	alert := s.Alert
	if legacy && strings.TrimSpace(alert.Fingerprint) == "" {
		alert.Fingerprint = s.DedupFingerprint
	}
	return alert
}

// ListProjectionAlerts reads a bounded, stable alert-id ordered projection
// image for T-OS-004 reconciliation. It never uses from+size or an unversioned
// target and fails closed on timeout or shard failure.
func (w *OpenSearchWriter) ListProjectionAlerts(ctx context.Context, scope ProjectionScope) ([]*Alert, bool, error) {
	if strings.TrimSpace(scope.TenantID) == "" || scope.MaxDocuments < 1 || scope.MaxDocuments > 100000 {
		return nil, false, fmt.Errorf("invalid projection reconciliation scope")
	}
	if scope.TargetIndexVersion != w.TargetVersion() {
		return nil, false, fmt.Errorf("projection target mismatch: scope=%s writer=%s", scope.TargetIndexVersion, w.TargetVersion())
	}
	index := w.readTarget
	tenantField := "tenant_id"
	alertIDField := "alert_id"
	if w.legacyReadKeywordFields {
		tenantField += ".keyword"
		alertIDField += ".keyword"
	}
	alerts := make([]*Alert, 0, min(scope.MaxDocuments, 1000))
	var searchAfter []interface{}
	for len(alerts) <= scope.MaxDocuments {
		remaining := scope.MaxDocuments + 1 - len(alerts)
		pageSize := min(remaining, 1000)
		filters := []interface{}{map[string]interface{}{"term": map[string]interface{}{tenantField: scope.TenantID}}}
		if !scope.StartTime.IsZero() || !scope.EndTime.IsZero() {
			rangeBounds := map[string]interface{}{}
			if !scope.StartTime.IsZero() {
				rangeBounds["gte"] = scope.StartTime.UTC().Format(time.RFC3339Nano)
			}
			if !scope.EndTime.IsZero() {
				rangeBounds["lte"] = scope.EndTime.UTC().Format(time.RFC3339Nano)
			}
			filters = append(filters, map[string]interface{}{"range": map[string]interface{}{"last_seen": rangeBounds}})
		}
		if len(scope.BusinessIDs) > 0 {
			filters = append(filters, map[string]interface{}{"terms": map[string]interface{}{alertIDField: scope.BusinessIDs}})
		}
		body := map[string]interface{}{
			"size":             pageSize,
			"query":            map[string]interface{}{"bool": map[string]interface{}{"filter": filters}},
			"sort":             []interface{}{map[string]interface{}{alertIDField: map[string]interface{}{"order": "asc"}}},
			"track_total_hits": false,
		}
		if len(searchAfter) > 0 {
			body["search_after"] = searchAfter
		}
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, false, err
		}
		allowPartial := false
		ignoreUnavailable := false
		request := opensearchapi.SearchRequest{
			Index: []string{index}, Body: bytes.NewReader(payload),
			AllowPartialSearchResults: &allowPartial, IgnoreUnavailable: &ignoreUnavailable,
		}
		response, err := request.Do(ctx, w.client)
		if err != nil {
			return nil, false, fmt.Errorf("read OpenSearch alert projection: %w", err)
		}
		if response.IsError() {
			responseBody, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return nil, false, fmt.Errorf("OpenSearch projection search failed: %s %s", response.Status(), strings.TrimSpace(string(responseBody)))
		}
		var result struct {
			TimedOut bool `json:"timed_out"`
			Shards   struct {
				Failed int `json:"failed"`
			} `json:"_shards"`
			Hits struct {
				Hits []struct {
					Source openSearchProjectionSource `json:"_source"`
					Sort   []interface{}              `json:"sort"`
				} `json:"hits"`
			} `json:"hits"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if decodeErr != nil {
			return nil, false, fmt.Errorf("decode OpenSearch projection search: %w", decodeErr)
		}
		if result.TimedOut || result.Shards.Failed > 0 {
			return nil, false, fmt.Errorf("OpenSearch projection search incomplete: timed_out=%t failed_shards=%d", result.TimedOut, result.Shards.Failed)
		}
		if len(result.Hits.Hits) == 0 {
			break
		}
		for index := range result.Hits.Hits {
			hit := &result.Hits.Hits[index]
			alert := hit.Source.canonicalAlert(w.legacyReadKeywordFields)
			if alert.TenantID != scope.TenantID || strings.TrimSpace(alert.AlertID) == "" {
				return nil, false, fmt.Errorf("OpenSearch projection returned invalid tenant or alert identity")
			}
			alerts = append(alerts, &alert)
			searchAfter = hit.Sort
		}
		if len(result.Hits.Hits) < pageSize {
			break
		}
	}
	truncated := len(alerts) > scope.MaxDocuments
	if truncated {
		alerts = alerts[:scope.MaxDocuments]
	}
	return alerts, truncated, nil
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
