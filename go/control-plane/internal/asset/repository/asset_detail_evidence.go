package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

type AssetEvidenceObjectInfo struct {
	Size           int64
	ContentType    string
	ETag           string
	VersionID      string
	LastModified   time.Time
	ChecksumSHA256 string
	UserMetadata   map[string]string
}

type AssetEvidenceObjectStore interface {
	StatObject(context.Context, string, string) (AssetEvidenceObjectInfo, error)
}

type MinIOAssetEvidenceObjectStore struct{ client *minio.Client }

func NewMinIOAssetEvidenceObjectStore(client *minio.Client) (*MinIOAssetEvidenceObjectStore, error) {
	if client == nil {
		return nil, fmt.Errorf("MinIO client is required")
	}
	return &MinIOAssetEvidenceObjectStore{client: client}, nil
}

func (s *MinIOAssetEvidenceObjectStore) StatObject(ctx context.Context, bucket, key string) (AssetEvidenceObjectInfo, error) {
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return AssetEvidenceObjectInfo{}, err
	}
	metadata := make(map[string]string, len(info.UserMetadata))
	for key, value := range info.UserMetadata {
		metadata[strings.ToLower(key)] = value
	}
	return AssetEvidenceObjectInfo{
		Size: info.Size, ContentType: info.ContentType, ETag: info.ETag,
		VersionID: info.VersionID, LastModified: info.LastModified,
		ChecksumSHA256: info.ChecksumSHA256, UserMetadata: metadata,
	}, nil
}

type AssetDetailEvidenceReader struct {
	db         *sql.DB
	objects    AssetEvidenceObjectStore
	queryLimit time.Duration
	limit      int
}

func NewAssetDetailEvidenceReader(db *sql.DB, objects AssetEvidenceObjectStore, queryLimit time.Duration, limit int) (*AssetDetailEvidenceReader, error) {
	if db == nil || objects == nil {
		return nil, fmt.Errorf("ClickHouse and object metadata stores are required")
	}
	if queryLimit <= 0 {
		return nil, fmt.Errorf("asset evidence query timeout must be positive")
	}
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("asset evidence limit must be within 1..500")
	}
	return &AssetDetailEvidenceReader{db: db, objects: objects, queryLimit: queryLimit, limit: limit}, nil
}

type assetEvidenceRow struct {
	EvidenceID string
	AlertID    string
	Timestamp  int64
	Type       string
	Summary    string
	Metrics    string
	Snippet    string
}

func (r *AssetDetailEvidenceReader) ReadAssetEvidenceObjects(
	ctx context.Context,
	tenantID string,
	asset *config.AssetRecord,
	asOf time.Time,
	alerts *config.AssetAlertContext,
) (*config.AssetEvidenceObjectSet, map[string]string, bool, error) {
	if tenantID == "" || asset == nil || asset.TenantID != tenantID || alerts == nil || alerts.AssetID != asset.AssetID {
		return nil, nil, false, fmt.Errorf("tenant-scoped asset alert context is required")
	}
	if asOf.IsZero() {
		return nil, nil, false, fmt.Errorf("PostgreSQL snapshot as_of is required")
	}
	requestedSet := make(map[string]struct{})
	for _, alert := range alerts.Alerts {
		for _, evidenceID := range alert.EvidenceIDs {
			if value := strings.TrimSpace(evidenceID); value != "" {
				requestedSet[value] = struct{}{}
			}
		}
	}
	requested := make([]string, 0, len(requestedSet))
	for evidenceID := range requestedSet {
		requested = append(requested, evidenceID)
	}
	sort.Strings(requested)
	truncated := len(requested) > r.limit
	if truncated {
		requested = requested[:r.limit]
	}
	result := &config.AssetEvidenceObjectSet{
		AssetID: asset.AssetID, Source: "clickhouse.evidence+minio.stat.v1",
		Objects: []config.AssetEvidenceObjectManifest{}, MissingEvidenceIDs: []string{}, Truncated: truncated,
	}
	watermarks := map[string]string{"clickhouse.evidence.query_as_of": asOf.UTC().Format(time.RFC3339Nano)}
	if len(requested) == 0 {
		result.Partial = truncated
		return result, watermarks, !truncated, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.queryLimit)
	defer cancel()
	query := fmt.Sprintf(`
		WITH %s AS requested_evidence_ids
		SELECT
			evidence_id,
			argMax(alert_id, tuple(ts, ingest_ts, event_id)) AS latest_alert_id,
			argMax(ts, tuple(ts, ingest_ts, event_id)) AS latest_evidence_ts,
			argMax(type, tuple(ts, ingest_ts, event_id)) AS latest_evidence_type,
			argMax(summary, tuple(ts, ingest_ts, event_id)) AS latest_summary,
			argMax(metrics_json, tuple(ts, ingest_ts, event_id)) AS latest_metrics_json,
			argMax(snippet_ref_json, tuple(ts, ingest_ts, event_id)) AS latest_snippet_ref_json
		FROM traffic.evidence
		WHERE tenant_id = ? AND ts <= ? AND has(requested_evidence_ids, evidence_id)
		GROUP BY evidence_id
		ORDER BY evidence_id ASC`, clickHouseBoundArray(len(requested)))
	args := make([]interface{}, 0, len(requested)+2)
	for _, evidenceID := range requested {
		args = append(args, evidenceID)
	}
	args = append(args, tenantID, asOf.UTC().UnixMilli())
	rows, err := r.db.QueryContext(queryCtx, query, args...)
	if err != nil {
		return nil, nil, false, fmt.Errorf("query bounded asset evidence records: %w", err)
	}
	defer rows.Close()
	records := make(map[string]assetEvidenceRow, len(requested))
	var maxEvidenceAt time.Time
	for rows.Next() {
		var row assetEvidenceRow
		if err := rows.Scan(&row.EvidenceID, &row.AlertID, &row.Timestamp, &row.Type, &row.Summary, &row.Metrics, &row.Snippet); err != nil {
			return nil, nil, false, fmt.Errorf("scan bounded asset evidence record: %w", err)
		}
		records[row.EvidenceID] = row
		if timestamp := time.UnixMilli(row.Timestamp).UTC(); timestamp.After(maxEvidenceAt) {
			maxEvidenceAt = timestamp
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, fmt.Errorf("iterate bounded asset evidence records: %w", err)
	}
	if !maxEvidenceAt.IsZero() {
		watermarks["clickhouse.evidence.max_ts"] = maxEvidenceAt.Format(time.RFC3339Nano)
	}
	var maxObjectModified time.Time
	for _, evidenceID := range requested {
		row, ok := records[evidenceID]
		if !ok {
			result.MissingEvidenceIDs = append(result.MissingEvidenceIDs, evidenceID)
			continue
		}
		ref, sha256Value, ok := evidenceObjectReference(row.Snippet, row.Metrics)
		if !ok {
			result.MissingEvidenceIDs = append(result.MissingEvidenceIDs, evidenceID)
			continue
		}
		info, err := r.objects.StatObject(queryCtx, ref.bucket, ref.key)
		if err != nil {
			result.MissingEvidenceIDs = append(result.MissingEvidenceIDs, evidenceID)
			continue
		}
		sha256Value = firstValidSHA256(sha256Value, info.ChecksumSHA256, info.UserMetadata["sha256"], info.UserMetadata["x-amz-meta-sha256"])
		integrityStatus := "verified_metadata"
		if sha256Value == "" {
			integrityStatus = "unverified_missing_sha256"
			result.MissingEvidenceIDs = append(result.MissingEvidenceIDs, evidenceID)
		}
		evidenceAt := time.UnixMilli(row.Timestamp).UTC()
		manifest := config.AssetEvidenceObjectManifest{
			EvidenceID: evidenceID, AlertID: row.AlertID, EvidenceType: row.Type, Summary: row.Summary,
			Bucket: ref.bucket, ObjectKey: ref.key, ObjectVersion: info.VersionID,
			ContentType: info.ContentType, SizeBytes: info.Size, ETag: info.ETag,
			SHA256: sha256Value, IntegrityStatus: integrityStatus,
			EvidenceAt: evidenceAt, LastModified: info.LastModified.UTC(),
		}
		result.Objects = append(result.Objects, manifest)
		if info.LastModified.After(maxObjectModified) {
			maxObjectModified = info.LastModified.UTC()
		}
	}
	if !maxObjectModified.IsZero() {
		watermarks["minio.evidence.max_last_modified"] = maxObjectModified.Format(time.RFC3339Nano)
	}
	result.Partial = result.Truncated || len(result.MissingEvidenceIDs) > 0
	return result, watermarks, !result.Partial, nil
}

func clickHouseBoundArray(size int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("?,", size), ",") + "]"
}

type evidenceObjectRef struct{ bucket, key string }

func evidenceObjectReference(snippetJSON, metricsJSON string) (evidenceObjectRef, string, bool) {
	snippet := map[string]string{}
	metrics := map[string]interface{}{}
	_ = json.Unmarshal([]byte(snippetJSON), &snippet)
	_ = json.Unmarshal([]byte(metricsJSON), &metrics)
	bucket, key := strings.TrimSpace(snippet["bucket"]), strings.TrimSpace(snippet["object"])
	sha256Value := firstValidSHA256(snippet["sha256"], fmt.Sprint(metrics["sha256"]), fmt.Sprint(metrics["artifact_sha256"]))
	for _, candidate := range []interface{}{metrics["object_path"], metrics["objectPath"], metrics["minio_path"], metrics["minioPath"]} {
		value := strings.TrimSpace(fmt.Sprint(candidate))
		if !strings.HasPrefix(value, "minio://") {
			continue
		}
		parsed, err := url.Parse(value)
		if err == nil {
			if bucket == "" {
				bucket = parsed.Host
			}
			if key == "" {
				key = strings.TrimPrefix(parsed.Path, "/")
			}
		}
	}
	if bucket == "" || key == "" || strings.Contains(key, "..") {
		return evidenceObjectRef{}, sha256Value, false
	}
	return evidenceObjectRef{bucket: bucket, key: key}, sha256Value, true
}

func firstValidSHA256(values ...string) string {
	for _, value := range values {
		value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
		decoded, err := hex.DecodeString(value)
		if err == nil && len(decoded) == 32 {
			return value
		}
	}
	return ""
}
