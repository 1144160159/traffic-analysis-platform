package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/miniohttp"
)

var assetExportColumns = map[string]func(*config.AssetRecord) any{
	"asset_id":     func(asset *config.AssetRecord) any { return asset.AssetID },
	"revision":     func(asset *config.AssetRecord) any { return asset.Revision },
	"display_code": func(asset *config.AssetRecord) any { return asset.DisplayCode },
	"asset_type":   func(asset *config.AssetRecord) any { return asset.AssetType },
	"status":       func(asset *config.AssetRecord) any { return asset.Status },
	"ip_address":   func(asset *config.AssetRecord) any { return asset.IPAddress },
	"mac_address":  func(asset *config.AssetRecord) any { return asset.MACAddress },
	"hostname":     func(asset *config.AssetRecord) any { return asset.Hostname },
	"vendor":       func(asset *config.AssetRecord) any { return asset.Vendor },
	"os_type":      func(asset *config.AssetRecord) any { return asset.OSType },
	"source":       func(asset *config.AssetRecord) any { return asset.Source },
	"vlan_id":      func(asset *config.AssetRecord) any { return asset.VlanID },
	"switch_port":  func(asset *config.AssetRecord) any { return asset.SwitchPort },
	"department":   func(asset *config.AssetRecord) any { return asset.Department },
	"campus":       func(asset *config.AssetRecord) any { return asset.Campus },
	"owner":        func(asset *config.AssetRecord) any { return asset.Owner },
	"criticality":  func(asset *config.AssetRecord) any { return asset.Criticality },
	"first_seen":   func(asset *config.AssetRecord) any { return asset.FirstSeen.UTC().Format(time.RFC3339Nano) },
	"last_seen":    func(asset *config.AssetRecord) any { return asset.LastSeen.UTC().Format(time.RFC3339Nano) },
}

var defaultAssetExportColumns = []string{
	"display_code", "ip_address", "mac_address", "hostname", "asset_type",
	"status", "department", "campus", "owner", "criticality", "last_seen",
}

type AssetExportObjectStore interface {
	Put(context.Context, string, string, io.Reader, int64, string) error
	Open(context.Context, string, string) (io.ReadCloser, error)
}

type minioAssetExportObjectStore struct {
	client *minio.Client
}

func (s *AssetService) AssetExportJobsEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Export.Enabled
}

func (s *AssetService) CreateAssetExportJob(
	ctx context.Context,
	tenantID string,
	request config.AssetExportRequest,
	command config.AssetExportCommand,
) (*config.AssetExportJob, error) {
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	request.Reason = strings.TrimSpace(request.Reason)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Actor = strings.TrimSpace(command.Actor)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if tenantID == "" || command.Actor == "" || command.TraceID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant, authenticated actor and trace_id are required")
	}
	if request.ActionID != config.AssetExportActionID {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported asset export action_id")
	}
	if request.Format != "csv" && request.Format != "jsonl" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "format must be csv or jsonl")
	}
	if len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "Idempotency-Key must be 16-200 characters")
	}
	if len(request.Reason) < 4 || len(request.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}
	columns, err := normalizeAssetExportColumns(request.Columns)
	if err != nil {
		return nil, err
	}
	request.Columns = columns
	if err := validateAssetExportFilter(request.Filter); err != nil {
		return nil, err
	}
	normalized, _ := json.Marshal(map[string]any{
		"action_id": request.ActionID, "format": request.Format,
		"columns": request.Columns, "filter": request.Filter,
		"reason": request.Reason, "actor": command.Actor,
	})
	digest := sha256.Sum256(normalized)
	now := time.Now().UTC()
	job := &config.AssetExportJob{
		JobID: uuid.NewString(), TenantID: tenantID,
		ActionID: request.ActionID, Format: request.Format,
		Status: config.AssetExportStatusAccepted, Revision: 1,
		Columns: request.Columns, Filter: request.Filter,
		QuerySHA256: fmt.Sprintf("%x", digest[:]), Reason: request.Reason,
		SourceWatermarks: map[string]string{}, CreatedBy: command.Actor,
		TraceID: command.TraceID, CreatedAt: now, UpdatedAt: now,
	}
	return s.repo.CreateAssetExportJobAtomic(ctx, job, command)
}

func (s *AssetService) GetAssetExportJob(
	ctx context.Context,
	tenantID, jobID string,
) (*config.AssetExportJob, error) {
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if _, err := uuid.Parse(jobID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "job_id must be a UUID")
	}
	return s.repo.GetAssetExportJob(ctx, tenantID, jobID)
}

func (s *AssetService) StartAssetExportWorker(ctx context.Context) {
	if s == nil || s.cfg == nil || !s.cfg.Export.Enabled || !s.cfg.Export.WorkerEnabled {
		return
	}
	interval := s.cfg.Export.WorkerInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := "asset-export-" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			found, err := s.ProcessNextAssetExport(ctx, workerID)
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("asset export worker iteration failed", zap.Error(err))
			}
			if found {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *AssetService) ProcessNextAssetExport(
	ctx context.Context,
	workerID string,
) (bool, error) {
	lease := 5 * time.Minute
	if s.cfg != nil && s.cfg.Export.WorkerLease > 0 {
		lease = s.cfg.Export.WorkerLease
	}
	job, err := s.repo.ClaimAssetExportJob(ctx, workerID, lease)
	if stderrors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	maxRows := 100000
	maxBytes := int64(100 << 20)
	retention := 7 * 24 * time.Hour
	bucket := "report-artifacts"
	if s.cfg != nil {
		if s.cfg.Export.MaxRows > 0 {
			maxRows = s.cfg.Export.MaxRows
		}
		if s.cfg.Export.MaxBytes > 0 {
			maxBytes = s.cfg.Export.MaxBytes
		}
		if s.cfg.Export.Retention > 0 {
			retention = s.cfg.Export.Retention
		}
		if strings.TrimSpace(s.cfg.Export.Bucket) != "" {
			bucket = strings.TrimSpace(s.cfg.Export.Bucket)
		}
	}
	snapshot, err := s.repo.LoadAssetExportSnapshot(ctx, job.TenantID, job.Filter, maxRows)
	if err != nil {
		_ = s.repo.FailAssetExportJob(ctx, job, err)
		return true, err
	}
	content, mimeType, extension, err := buildAssetExportArtifact(job.Format, job.Columns, snapshot.Assets)
	if err != nil {
		_ = s.repo.FailAssetExportJob(ctx, job, err)
		return true, err
	}
	if int64(len(content)) > maxBytes {
		err = fmt.Errorf("asset export size limit exceeded: bytes=%d max=%d", len(content), maxBytes)
		_ = s.repo.FailAssetExportJob(ctx, job, err)
		return true, err
	}
	store, err := s.assetExportObjectStore()
	if err != nil {
		_ = s.repo.FailAssetExportJob(ctx, job, err)
		return true, err
	}
	job.SnapshotID = snapshot.SnapshotID
	job.AsOf = snapshot.AsOf
	job.SourceWatermarks = snapshot.SourceWatermarks
	job.RowCount = len(snapshot.Assets)
	job.ObjectBucket = bucket
	job.ObjectKey = pathpkg.Join(
		safeAssetObjectSegment(job.TenantID), "assets", "exports",
		job.JobID+"."+extension,
	)
	job.MIMEType = mimeType
	job.SizeBytes = int64(len(content))
	job.ArtifactSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	job.RetentionUntil = time.Now().UTC().Add(retention)
	if err := store.Put(
		ctx, job.ObjectBucket, job.ObjectKey, bytes.NewReader(content),
		job.SizeBytes, job.MIMEType,
	); err != nil {
		_ = s.repo.FailAssetExportJob(ctx, job, err)
		return true, err
	}
	if _, err := s.repo.CompleteAssetExportJob(ctx, job, workerID); err != nil {
		return true, err
	}
	return true, nil
}

func (s *AssetService) ReadAssetExportArtifact(
	ctx context.Context,
	job *config.AssetExportJob,
) ([]byte, error) {
	if job == nil || job.Status != config.AssetExportStatusCompleted ||
		job.ObjectBucket == "" || job.ObjectKey == "" {
		return nil, fmt.Errorf("asset export is not completed")
	}
	if !job.RetentionUntil.IsZero() && time.Now().UTC().After(job.RetentionUntil) {
		return nil, fmt.Errorf("asset export retention has expired")
	}
	maxBytes := int64(100 << 20)
	if s.cfg != nil && s.cfg.Export.MaxBytes > 0 {
		maxBytes = s.cfg.Export.MaxBytes
	}
	if job.SizeBytes < 0 || job.SizeBytes > maxBytes {
		return nil, fmt.Errorf("asset export manifest size is outside the download budget")
	}
	store, err := s.assetExportObjectStore()
	if err != nil {
		return nil, err
	}
	reader, err := store.Open(ctx, job.ObjectBucket, job.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) != job.SizeBytes || int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("asset export object size does not match its manifest")
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(content)) != job.ArtifactSHA256 {
		return nil, fmt.Errorf("asset export object checksum does not match its manifest")
	}
	return content, nil
}

func (s *AssetService) RecordAssetExportDownload(
	ctx context.Context,
	job *config.AssetExportJob,
	actor, traceID, requestID, clientIP, userAgent string,
) error {
	return s.repo.RecordAssetExportDownload(
		ctx, job, actor, traceID, requestID, clientIP, userAgent,
	)
}

func (s *AssetService) GetAssetColumnPreference(
	ctx context.Context,
	tenantID, userID, viewID string,
) (*config.AssetColumnPreference, error) {
	viewID = strings.TrimSpace(viewID)
	if viewID == "" {
		viewID = "asset-inventory"
	}
	if viewID != "asset-inventory" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported asset column preference view_id")
	}
	preference, err := s.repo.GetAssetColumnPreference(ctx, tenantID, userID, viewID)
	if stderrors.Is(err, sql.ErrNoRows) {
		return &config.AssetColumnPreference{
			TenantID: tenantID, UserID: userID, ViewID: viewID,
			Columns:  append([]string(nil), defaultAssetExportColumns...),
			Revision: 0,
		}, nil
	}
	return preference, err
}

func (s *AssetService) UpsertAssetColumnPreference(
	ctx context.Context,
	tenantID, userID string,
	command config.AssetColumnPreferenceCommand,
) (*config.AssetColumnPreference, error) {
	command.ViewID = strings.TrimSpace(command.ViewID)
	if command.ViewID == "" {
		command.ViewID = "asset-inventory"
	}
	if command.ViewID != "asset-inventory" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported asset column preference view_id")
	}
	columns, err := normalizeAssetExportColumns(command.Columns)
	if err != nil {
		return nil, err
	}
	command.Columns = columns
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedRevision < 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "expected_revision cannot be negative")
	}
	if len(command.Reason) < 4 || len(command.Reason) > 1000 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "reason must be 4-1000 characters")
	}
	if tenantID == "" || userID == "" || strings.TrimSpace(command.Actor) == "" ||
		strings.TrimSpace(command.TraceID) == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "tenant, user, actor and trace_id are required")
	}
	return s.repo.UpsertAssetColumnPreference(ctx, tenantID, userID, command)
}

func normalizeAssetExportColumns(columns []string) ([]string, error) {
	if len(columns) == 0 {
		columns = defaultAssetExportColumns
	}
	if len(columns) > len(assetExportColumns) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "too many asset export columns")
	}
	result := make([]string, 0, len(columns))
	seen := map[string]bool{}
	for _, raw := range columns {
		column := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := assetExportColumns[column]; !ok {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "unsupported asset export column: "+column)
		}
		if !seen[column] {
			seen[column] = true
			result = append(result, column)
		}
	}
	if len(result) == 0 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "at least one asset export column is required")
	}
	return result, nil
}

func validateAssetExportFilter(filter config.AssetListFilter) error {
	if filter.AssetType != "" {
		allowed := map[string]bool{
			"endpoint": true, "server": true, "network-device": true,
			"business-system": true, "unknown": true,
		}
		if !allowed[filter.AssetType] {
			return errors.New(errors.ErrCodeInvalidParameter, "unsupported asset_type filter")
		}
	}
	for name, value := range map[string]string{
		"status": filter.Status, "search": filter.Search,
		"department": filter.Department, "campus": filter.Campus,
		"ip_prefix": filter.IPPrefix, "vendor": filter.Vendor,
	} {
		if len(value) > 200 {
			return errors.New(errors.ErrCodeInvalidParameter, name+" filter is too long")
		}
	}
	return nil
}

func buildAssetExportArtifact(
	format string,
	columns []string,
	assets []*config.AssetRecord,
) ([]byte, string, string, error) {
	var output bytes.Buffer
	switch format {
	case "csv":
		output.WriteString("\uFEFF")
		writer := csv.NewWriter(&output)
		if err := writer.Write(columns); err != nil {
			return nil, "", "", err
		}
		for _, asset := range assets {
			record := make([]string, 0, len(columns))
			for _, column := range columns {
				record = append(record, fmt.Sprint(assetExportColumns[column](asset)))
			}
			if err := writer.Write(record); err != nil {
				return nil, "", "", err
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, "", "", err
		}
		return output.Bytes(), "text/csv; charset=utf-8", "csv", nil
	case "jsonl":
		encoder := json.NewEncoder(&output)
		encoder.SetEscapeHTML(false)
		for _, asset := range assets {
			row := make(map[string]any, len(columns))
			for _, column := range columns {
				row[column] = assetExportColumns[column](asset)
			}
			if err := encoder.Encode(row); err != nil {
				return nil, "", "", err
			}
		}
		return output.Bytes(), "application/x-ndjson", "jsonl", nil
	default:
		return nil, "", "", fmt.Errorf("unsupported asset export format: %s", format)
	}
}

func (s *AssetService) assetExportObjectStore() (AssetExportObjectStore, error) {
	if s.exportObjects != nil {
		return s.exportObjects, nil
	}
	if s.cfg == nil || s.cfg.Export.S3AccessKey == "" || s.cfg.Export.S3SecretKey == "" {
		return nil, fmt.Errorf("asset export object storage credentials are not configured")
	}
	endpoint := strings.TrimPrefix(
		strings.TrimPrefix(strings.TrimSpace(s.cfg.Export.S3Endpoint), "http://"),
		"https://",
	)
	transport, err := miniohttp.NewTransport(s.cfg.Export.S3UseSSL, s.cfg.Export.S3CAFile)
	if err != nil {
		return nil, fmt.Errorf("configure asset export MinIO TLS: %w", err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(
			s.cfg.Export.S3AccessKey, s.cfg.Export.S3SecretKey, "",
		),
		Secure:    s.cfg.Export.S3UseSSL,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}
	s.exportObjects = &minioAssetExportObjectStore{client: client}
	return s.exportObjects, nil
}

func (s *minioAssetExportObjectStore) Put(
	ctx context.Context,
	bucket, key string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	_, err := s.client.PutObject(
		ctx, bucket, key, reader, size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	return err
}

func (s *minioAssetExportObjectStore) Open(
	ctx context.Context,
	bucket, key string,
) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

func safeAssetObjectSegment(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func AssetExportAllowedColumns() []string {
	result := make([]string, 0, len(assetExportColumns))
	for column := range assetExportColumns {
		result = append(result, column)
	}
	sort.Strings(result)
	return result
}
