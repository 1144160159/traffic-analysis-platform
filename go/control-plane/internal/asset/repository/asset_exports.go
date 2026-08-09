package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var (
	ErrAssetExportIdempotencyConflict        = errors.New("asset export idempotency conflict")
	ErrAssetExportRowLimit                   = errors.New("asset export row limit exceeded")
	ErrAssetExportLeaseConflict              = errors.New("asset export worker lease conflict")
	ErrAssetColumnPreferenceRevisionConflict = errors.New("asset column preference revision conflict")
)

const assetExportJobColumns = `
	job_id::text,tenant_id,action_id,format,status,revision,columns,query,
	query_sha256,reason,snapshot_id,as_of,source_watermarks,row_count,object_bucket,
	object_key,mime_type,artifact_sha256,size_bytes,retention_until,
	error_message,attempts,created_by,trace_id,created_at,updated_at,completed_at,
	locked_by`

func (r *AssetRepository) CreateAssetExportJobAtomic(
	ctx context.Context,
	job *config.AssetExportJob,
	command config.AssetExportCommand,
) (*config.AssetExportJob, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:asset-export:%s", len(job.TenantID), job.TenantID, command.IdempotencyKey),
	); err != nil {
		return nil, err
	}
	existing, err := getAssetExportJobByIdempotencyTx(
		ctx, tx, job.TenantID, command.IdempotencyKey,
	)
	if err == nil {
		if existing.QuerySHA256 != job.QuerySHA256 {
			return nil, ErrAssetExportIdempotencyConflict
		}
		existing.IdempotentReplay = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	columnsJSON, _ := json.Marshal(job.Columns)
	filterJSON, _ := json.Marshal(job.Filter)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_export_jobs(
			job_id,tenant_id,action_id,format,status,revision,columns,query,
			query_sha256,idempotency_key,reason,created_by,trace_id
		) VALUES (
			$1,$2,$3,$4,'accepted',1,$5::jsonb,$6::jsonb,$7,$8,$9,$10,$11
		)`,
		job.JobID, job.TenantID, job.ActionID, job.Format,
		string(columnsJSON), string(filterJSON), job.QuerySHA256,
		command.IdempotencyKey, job.Reason, command.Actor, command.TraceID,
	); err != nil {
		return nil, err
	}
	eventID := uuid.NewString()
	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": "traffic.asset.export.v1.Requested",
		"schema_version": 1, "aggregate_version": 1,
		"tenant_id": job.TenantID, "job_id": job.JobID,
		"partition_key": job.TenantID + ":" + job.JobID,
		"query_sha256":  job.QuerySHA256, "format": job.Format,
		"trace_id": command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_export_outbox(
			event_id,job_id,tenant_id,event_type,aggregate_version,
			partition_key,payload
		) VALUES ($1,$2,$3,'traffic.asset.export.v1.Requested',1,$4,$5::jsonb)`,
		eventID, job.JobID, job.TenantID, job.TenantID+":"+job.JobID, string(payload),
	); err != nil {
		return nil, err
	}
	detail, _ := json.Marshal(map[string]any{
		"action_id": job.ActionID, "format": job.Format, "columns": job.Columns,
		"filter": job.Filter, "query_sha256": job.QuerySHA256,
		"idempotency_key_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(command.IdempotencyKey))),
		"request_id":             command.RequestID, "trace_id": command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent,request_id,trace_id,success,risk_level,result
		) VALUES (
			$1,$2,$3,'ASSET_EXPORT_REQUESTED','asset_export',$4,$5::jsonb,
			NULLIF($6,''),NULLIF($7,''),$8,$9,true,'medium','accepted'
		)`,
		"asset-export-requested-"+eventID, job.TenantID, command.Actor,
		job.JobID, string(detail), command.ClientIP, command.UserAgent,
		command.RequestID, command.TraceID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAssetExportJob(ctx, job.TenantID, job.JobID)
}

func (r *AssetRepository) GetAssetExportJob(
	ctx context.Context,
	tenantID, jobID string,
) (*config.AssetExportJob, error) {
	return scanAssetExportJob(r.db.QueryRowContext(ctx, `
		SELECT `+assetExportJobColumns+`
		  FROM asset_export_jobs
		 WHERE tenant_id=$1 AND job_id=$2`,
		tenantID, jobID,
	))
}

func getAssetExportJobByIdempotencyTx(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, key string,
) (*config.AssetExportJob, error) {
	return scanAssetExportJob(tx.QueryRowContext(ctx, `
		SELECT `+assetExportJobColumns+`
		  FROM asset_export_jobs
		 WHERE tenant_id=$1 AND idempotency_key=$2
		 FOR UPDATE`,
		tenantID, key,
	))
}

func (r *AssetRepository) ClaimAssetExportJob(
	ctx context.Context,
	workerID string,
	lease time.Duration,
) (*config.AssetExportJob, error) {
	leaseSeconds := int(lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 300
	}
	return scanAssetExportJob(r.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT job_id AS candidate_job_id
			  FROM asset_export_jobs
			 WHERE (status='accepted' AND next_attempt_at<=now())
			    OR (status='running' AND locked_until<now())
			 ORDER BY created_at,job_id
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE asset_export_jobs j
		   SET status='running',revision=j.revision+1,attempts=j.attempts+1,
		       locked_by=$1,locked_until=now()+($2::text || ' seconds')::interval,
		       updated_at=now()
		  FROM candidate c
		 WHERE j.job_id=c.candidate_job_id
		RETURNING `+assetExportJobColumns,
		workerID, leaseSeconds,
	))
}

func (r *AssetRepository) LoadAssetExportSnapshot(
	ctx context.Context,
	tenantID string,
	filter config.AssetListFilter,
	maxRows int,
) (*config.AssetExportSnapshot, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	conditions, args := assetListWhere(tenantID, filter)
	where := strings.Join(conditions, " AND ")
	var asOf, maxUpdated time.Time
	var snapshotXIDs string
	var maxRevision int64
	var total int
	if err := tx.QueryRowContext(ctx, `
		SELECT clock_timestamp(),pg_current_snapshot()::text,
		       COALESCE(max(updated_at),to_timestamp(0)),
		       COALESCE(max(revision),0),count(*)
		  FROM assets WHERE `+where,
		args...,
	).Scan(&asOf, &snapshotXIDs, &maxUpdated, &maxRevision, &total); err != nil {
		return nil, err
	}
	if total > maxRows {
		return nil, fmt.Errorf("%w: total=%d max=%d", ErrAssetExportRowLimit, total, maxRows)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT asset_id,revision,display_code,tenant_id,asset_type,status,
		       ip_address,mac_address,hostname,vendor,os_type,source,vlan_id,
		       switch_port,department,campus,owner,criticality,tags,metadata,
		       first_seen,last_seen
		  FROM assets WHERE `+where+` ORDER BY asset_id`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := make([]*config.AssetRecord, 0, total)
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%d\x00%d",
		tenantID, snapshotXIDs, asOf.UTC().Format(time.RFC3339Nano), maxRevision, total,
	)))
	return &config.AssetExportSnapshot{
		Assets:     assets,
		SnapshotID: "asset-export-" + fmt.Sprintf("%x", digest[:16]),
		AsOf:       asOf.UTC(),
		SourceWatermarks: map[string]string{
			"postgresql.assets.snapshot":   snapshotXIDs,
			"postgresql.assets.updated_at": maxUpdated.UTC().Format(time.RFC3339Nano),
			"postgresql.assets.revision":   fmt.Sprintf("%d", maxRevision),
			"postgresql.assets.count":      fmt.Sprintf("%d", total),
		},
	}, nil
}

func (r *AssetRepository) CompleteAssetExportJob(
	ctx context.Context,
	job *config.AssetExportJob,
	workerID string,
) (*config.AssetExportJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	watermarksJSON, _ := json.Marshal(job.SourceWatermarks)
	result, err := tx.ExecContext(ctx, `
		UPDATE asset_export_jobs
		   SET status='completed',revision=revision+1,snapshot_id=$3,as_of=$4,
		       source_watermarks=$5::jsonb,row_count=$6,object_bucket=$7,
		       object_key=$8,mime_type=$9,artifact_sha256=$10,size_bytes=$11,
		       retention_until=$12,error_message='',locked_by='',locked_until=NULL,
		       updated_at=now(),completed_at=now()
		 WHERE tenant_id=$1 AND job_id=$2 AND status='running' AND locked_by=$13`,
		job.TenantID, job.JobID, job.SnapshotID, job.AsOf,
		string(watermarksJSON), job.RowCount, job.ObjectBucket, job.ObjectKey,
		job.MIMEType, job.ArtifactSHA256, job.SizeBytes, job.RetentionUntil, workerID,
	)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, ErrAssetExportLeaseConflict
	}
	var revision int64
	if err := tx.QueryRowContext(ctx,
		`SELECT revision FROM asset_export_jobs WHERE tenant_id=$1 AND job_id=$2`,
		job.TenantID, job.JobID,
	).Scan(&revision); err != nil {
		return nil, err
	}
	eventID := uuid.NewString()
	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": "traffic.asset.export.v1.Completed",
		"schema_version": 1, "aggregate_version": revision,
		"tenant_id": job.TenantID, "job_id": job.JobID,
		"partition_key": job.TenantID + ":" + job.JobID,
		"snapshot_id":   job.SnapshotID, "artifact_sha256": job.ArtifactSHA256,
		"size_bytes": job.SizeBytes, "row_count": job.RowCount,
		"object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
		"trace_id": job.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_export_outbox(
			event_id,job_id,tenant_id,event_type,aggregate_version,
			partition_key,payload
		) VALUES ($1,$2,$3,'traffic.asset.export.v1.Completed',$4,$5,$6::jsonb)`,
		eventID, job.JobID, job.TenantID, revision,
		job.TenantID+":"+job.JobID, string(payload),
	); err != nil {
		return nil, err
	}
	detail, _ := json.Marshal(map[string]any{
		"snapshot_id": job.SnapshotID, "source_watermarks": job.SourceWatermarks,
		"row_count": job.RowCount, "object_bucket": job.ObjectBucket,
		"object_key": job.ObjectKey, "artifact_sha256": job.ArtifactSHA256,
		"size_bytes": job.SizeBytes, "retention_until": job.RetentionUntil,
		"trace_id": job.TraceID, "event_id": eventID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			trace_id,success,risk_level,result
		) VALUES (
			$1,$2,$3,'ASSET_EXPORT_COMPLETED','asset_export',$4,$5::jsonb,
			$6,true,'medium','completed'
		)`,
		"asset-export-completed-"+eventID, job.TenantID, job.CreatedBy,
		job.JobID, string(detail), job.TraceID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAssetExportJob(ctx, job.TenantID, job.JobID)
}

func (r *AssetRepository) FailAssetExportJob(
	ctx context.Context,
	job *config.AssetExportJob,
	cause error,
) error {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE asset_export_jobs
		   SET status=CASE WHEN attempts<5 THEN 'accepted' ELSE 'failed' END,
		       revision=revision+1,error_message=$3,
		       next_attempt_at=CASE WHEN attempts<5
		         THEN now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval
		         ELSE next_attempt_at END,
		       locked_by='',locked_until=NULL,updated_at=now(),
		       completed_at=CASE WHEN attempts<5 THEN NULL ELSE now() END
		 WHERE tenant_id=$1 AND job_id=$2 AND status='running'`,
		job.TenantID, job.JobID, message,
	)
	return err
}

func (r *AssetRepository) GetAssetColumnPreference(
	ctx context.Context,
	tenantID, userID, viewID string,
) (*config.AssetColumnPreference, error) {
	return scanAssetColumnPreference(r.db.QueryRowContext(ctx, `
		SELECT tenant_id,user_id,view_id,columns,revision,updated_by,created_at,updated_at
		  FROM asset_column_preferences
		 WHERE tenant_id=$1 AND user_id=$2 AND view_id=$3`,
		tenantID, userID, viewID,
	))
}

func (r *AssetRepository) UpsertAssetColumnPreference(
	ctx context.Context,
	tenantID, userID string,
	command config.AssetColumnPreferenceCommand,
) (*config.AssetColumnPreference, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var current int64
	err = tx.QueryRowContext(ctx, `
		SELECT revision FROM asset_column_preferences
		 WHERE tenant_id=$1 AND user_id=$2 AND view_id=$3
		 FOR UPDATE`,
		tenantID, userID, command.ViewID,
	).Scan(&current)
	columnsJSON, _ := json.Marshal(command.Columns)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if command.ExpectedRevision != 0 {
			return nil, ErrAssetColumnPreferenceRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO asset_column_preferences(
				tenant_id,user_id,view_id,columns,revision,updated_by
			) VALUES ($1,$2,$3,$4::jsonb,1,$5)`,
			tenantID, userID, command.ViewID, string(columnsJSON), command.Actor,
		); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if current != command.ExpectedRevision {
			return nil, ErrAssetColumnPreferenceRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE asset_column_preferences
			   SET columns=$4::jsonb,revision=revision+1,updated_by=$5,updated_at=now()
			 WHERE tenant_id=$1 AND user_id=$2 AND view_id=$3 AND revision=$6`,
			tenantID, userID, command.ViewID, string(columnsJSON), command.Actor, current,
		); err != nil {
			return nil, err
		}
	}
	detail, _ := json.Marshal(map[string]any{
		"view_id": command.ViewID, "columns": command.Columns,
		"previous_revision": command.ExpectedRevision,
		"reason":            command.Reason, "request_id": command.RequestID,
		"trace_id": command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent,request_id,trace_id,success,risk_level,result
		) VALUES (
			$1,$2,$3,'ASSET_COLUMN_PREFERENCE_UPDATED',
			'asset_column_preference',$4,$5::jsonb,NULLIF($6,''),
			NULLIF($7,''),$8,$9,true,'low','success'
		)`,
		"asset-column-preference-"+uuid.NewString(), tenantID, userID,
		command.ViewID, string(detail), command.ClientIP, command.UserAgent,
		command.RequestID, command.TraceID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAssetColumnPreference(ctx, tenantID, userID, command.ViewID)
}

func (r *AssetRepository) RecordAssetExportDownload(
	ctx context.Context,
	job *config.AssetExportJob,
	actor, traceID, requestID, clientIP, userAgent string,
) error {
	detail, _ := json.Marshal(map[string]any{
		"artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes,
		"object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
		"retention_until": job.RetentionUntil, "trace_id": traceID,
	})
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent,request_id,trace_id,success,risk_level,result
		) VALUES (
			$1,$2,$3,'ASSET_EXPORT_DOWNLOADED','asset_export',$4,$5::jsonb,
			NULLIF($6,''),NULLIF($7,''),$8,$9,true,'medium','success'
		)`,
		"asset-export-download-"+uuid.NewString(), job.TenantID, actor,
		job.JobID, string(detail), clientIP, userAgent, requestID, traceID,
	)
	return err
}

func scanAssetExportJob(scanner rowScanner) (*config.AssetExportJob, error) {
	var job config.AssetExportJob
	var columnsJSON, filterJSON, watermarksJSON []byte
	var asOf, retention, completed sql.NullTime
	if err := scanner.Scan(
		&job.JobID, &job.TenantID, &job.ActionID, &job.Format, &job.Status,
		&job.Revision, &columnsJSON, &filterJSON, &job.QuerySHA256,
		&job.Reason, &job.SnapshotID, &asOf, &watermarksJSON, &job.RowCount,
		&job.ObjectBucket, &job.ObjectKey, &job.MIMEType,
		&job.ArtifactSHA256, &job.SizeBytes, &retention,
		&job.ErrorMessage, &job.Attempts, &job.CreatedBy, &job.TraceID,
		&job.CreatedAt, &job.UpdatedAt, &completed, &job.LockedBy,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(columnsJSON, &job.Columns); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(filterJSON, &job.Filter); err != nil {
		return nil, err
	}
	if len(watermarksJSON) > 0 {
		if err := json.Unmarshal(watermarksJSON, &job.SourceWatermarks); err != nil {
			return nil, err
		}
	}
	if job.SourceWatermarks == nil {
		job.SourceWatermarks = map[string]string{}
	}
	if asOf.Valid {
		job.AsOf = asOf.Time
	}
	if retention.Valid {
		job.RetentionUntil = retention.Time
	}
	if completed.Valid {
		job.CompletedAt = completed.Time
	}
	return &job, nil
}

func scanAssetColumnPreference(scanner rowScanner) (*config.AssetColumnPreference, error) {
	var preference config.AssetColumnPreference
	var columnsJSON []byte
	if err := scanner.Scan(
		&preference.TenantID, &preference.UserID, &preference.ViewID,
		&columnsJSON, &preference.Revision, &preference.UpdatedBy,
		&preference.CreatedAt, &preference.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(columnsJSON, &preference.Columns); err != nil {
		return nil, err
	}
	return &preference, nil
}
