package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var (
	ErrDiscoveryIdempotencyConflict = errors.New("discovery idempotency key conflict")
	ErrDiscoveryOverlapConflict     = errors.New("overlapping discovery job is already active")
	ErrDiscoveryRevisionConflict    = errors.New("discovery job revision conflict")
	ErrDiscoveryStateConflict       = errors.New("discovery job state conflict")
)

const discoveryRunColumns = `
	run_id,tenant_id,mode,target_cidr,credential_id,action_id,status,revision,
	requested_by,reason,rate_limit_per_second,security_window_start,
	security_window_end,approved_by,trace_id,discovered_assets,
	discovered_links,discovered_candidates,rejected_records,result_watermark,
	error_message,queued_at,started_at,updated_at,completed_at`

func (r *AssetRepository) CreateDiscoveryJobAtomic(
	ctx context.Context,
	run *config.DiscoveryRun,
	command config.DiscoveryJobCommand,
	requestHash string,
) (*config.DiscoveryRun, error) {
	if run == nil {
		return nil, fmt.Errorf("discovery run required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Serialize active-network checks per tenant without locking unrelated
	// tenants. This makes the overlap check race-safe without a destructive
	// exclusion constraint rollout.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('asset-discovery:' || $1))`, run.TenantID); err != nil {
		return nil, err
	}

	existing, err := getDiscoveryRunByIdempotencyTx(ctx, tx, run.TenantID, command.IdempotencyKey)
	if err == nil {
		var existingHash string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_hash FROM asset_discovery_runs
			WHERE tenant_id=$1 AND idempotency_key=$2`,
			run.TenantID, command.IdempotencyKey,
		).Scan(&existingHash); err != nil {
			return nil, err
		}
		if existingHash != requestHash {
			return nil, ErrDiscoveryIdempotencyConflict
		}
		existing.IdempotentReplay = true
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if run.TargetCIDR != "" {
		var overlapping string
		err := tx.QueryRowContext(ctx, `
			SELECT run_id
			  FROM asset_discovery_runs
			 WHERE tenant_id=$1
			   AND target_network && $2::cidr
			   AND status IN ('queued','running','cancel_requested')
			 ORDER BY queued_at
			 LIMIT 1`,
			run.TenantID, run.TargetCIDR,
		).Scan(&overlapping)
		if err == nil {
			return nil, fmt.Errorf("%w: %s", ErrDiscoveryOverlapConflict, overlapping)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_runs (
			run_id,tenant_id,mode,target_cidr,target_network,credential_id,
			action_id,status,revision,requested_by,reason,rate_limit_per_second,
			security_window_start,security_window_end,approved_by,
			idempotency_key,request_hash,trace_id,queued_at,started_at,updated_at
		) VALUES (
			$1,$2,$3,NULLIF($4,''),NULLIF($4,'')::cidr,NULLIF($5,''),
			$6,'queued',1,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16,$16
		)`,
		run.RunID, run.TenantID, run.Mode, run.TargetCIDR, run.CredentialID,
		run.ActionID, run.RequestedBy, run.Reason, run.RateLimit,
		nullableTime(run.SecurityFrom), nullableTime(run.SecurityTo), run.ApprovedBy,
		command.IdempotencyKey, requestHash, run.TraceID, run.QueuedAt,
	)
	if err != nil {
		return nil, err
	}

	detail := map[string]any{
		"action_id":             run.ActionID,
		"mode":                  run.Mode,
		"target_cidr":           run.TargetCIDR,
		"credential_id":         run.CredentialID,
		"rate_limit_per_second": run.RateLimit,
		"request_id":            command.RequestID,
		"trace_id":              command.TraceID,
		"actor":                 command.Actor,
	}
	detailJSON, _ := json.Marshal(detail)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,'','queued',1,$3,$4,$5,$6::jsonb)`,
		run.RunID, run.TenantID, command.Actor, run.Reason, command.TraceID, string(detailJSON),
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent
		) VALUES ($1,$2,NULL,'ASSET_DISCOVERY_JOB_ACCEPTED',
			'asset_discovery_run',$3,$4::jsonb,NULLIF($5,''),NULLIF($6,''))`,
		uuid.NewString(), run.TenantID, run.RunID, string(detailJSON), command.ClientIP, command.UserAgent,
	); err != nil {
		return nil, err
	}
	eventPayload := map[string]any{
		"event_id":          uuid.NewString(),
		"event_type":        "traffic.asset.discovery.v1.JobAccepted",
		"schema_version":    1,
		"aggregate_version": 1,
		"tenant_id":         run.TenantID,
		"resource_type":     "run",
		"resource_id":       run.RunID,
		"action_id":         run.ActionID,
		"revision":          1,
		"run_id":            run.RunID,
		"partition_key":     run.TenantID + ":" + run.RunID,
		"trace_id":          run.TraceID,
		"status":            config.DiscoveryStatusQueued,
	}
	eventID, _ := uuid.Parse(eventPayload["event_id"].(string))
	eventJSON, _ := json.Marshal(eventPayload)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_outbox(
			event_id,run_id,resource_type,resource_id,action_id,tenant_id,
			aggregate_version,schema_version,partition_key,event_type,payload
		) VALUES ($1,$2,'run',$2,$3,$4,1,1,$5,$6,$7::jsonb)`,
		eventID, run.RunID, run.ActionID, run.TenantID, eventPayload["partition_key"],
		eventPayload["event_type"], string(eventJSON),
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *AssetRepository) GetDiscoveryRun(ctx context.Context, tenantID, runID string) (*config.DiscoveryRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1 AND run_id=$2`, tenantID, runID)
	return scanDiscoveryRun(row)
}

func (r *AssetRepository) ListDiscoveryJobs(
	ctx context.Context,
	tenantID string,
	limit int,
) ([]*config.DiscoveryRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1
		 ORDER BY queued_at DESC
		 LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*config.DiscoveryRun
	for rows.Next() {
		run, err := scanDiscoveryRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (r *AssetRepository) CancelDiscoveryJob(
	ctx context.Context,
	tenantID, runID, reason string,
	command config.DiscoveryJobCommand,
	expectedRevision int64,
) (*config.DiscoveryRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	requestPayload, _ := json.Marshal(map[string]any{
		"operation":         "cancel",
		"run_id":            runID,
		"reason":            reason,
		"expected_revision": expectedRevision,
		"actor":             command.Actor,
	})
	requestHash := discoveryObservationFingerprint(requestPayload)
	var replayRunID, replayHash string
	err = tx.QueryRowContext(ctx, `
		SELECT run_id,request_hash
		  FROM asset_discovery_control_requests
		 WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, command.IdempotencyKey,
	).Scan(&replayRunID, &replayHash)
	if err == nil {
		if replayRunID != runID || replayHash != requestHash {
			return nil, ErrDiscoveryIdempotencyConflict
		}
		replayed, err := scanDiscoveryRun(tx.QueryRowContext(ctx, `
			SELECT `+discoveryRunColumns+`
			  FROM asset_discovery_runs
			 WHERE tenant_id=$1 AND run_id=$2`, tenantID, runID))
		if err != nil {
			return nil, err
		}
		replayed.IdempotentReplay = true
		return replayed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1 AND run_id=$2
		 FOR UPDATE`, tenantID, runID)
	current, err := scanDiscoveryRun(row)
	if err != nil {
		return nil, err
	}
	if current.Revision != expectedRevision {
		return nil, ErrDiscoveryRevisionConflict
	}
	var next string
	switch current.Status {
	case config.DiscoveryStatusQueued:
		next = config.DiscoveryStatusCancelled
	case config.DiscoveryStatusRunning:
		next = config.DiscoveryStatusCancelRequested
	default:
		return nil, ErrDiscoveryStateConflict
	}
	nextRevision := current.Revision + 1
	completedAt := any(nil)
	if next == config.DiscoveryStatusCancelled {
		completedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		   SET status=$3,revision=$4,cancel_requested=true,updated_at=now(),
		       completed_at=COALESCE($5,completed_at)
		 WHERE tenant_id=$1 AND run_id=$2`,
		tenantID, runID, next, nextRevision, completedAt,
	); err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(map[string]any{"expected_revision": expectedRevision})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`,
		runID, tenantID, current.Status, next, nextRevision, command.Actor, reason, command.TraceID, string(detailJSON),
	); err != nil {
		return nil, err
	}
	if err := insertDiscoveryStateOutbox(ctx, tx, current, next, nextRevision, command.TraceID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_control_requests(
			request_id,tenant_id,run_id,operation,idempotency_key,request_hash,
			expected_revision,resulting_revision,actor,reason,trace_id
		) VALUES ($1,$2,$3,'cancel',$4,$5,$6,$7,$8,$9,$10)`,
		uuid.New(), tenantID, runID, command.IdempotencyKey, requestHash,
		expectedRevision, nextRevision, command.Actor, reason, command.TraceID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetDiscoveryRun(ctx, tenantID, runID)
}

func (r *AssetRepository) ClaimDiscoveryJob(
	ctx context.Context,
	workerID string,
	lease time.Duration,
) (*config.DiscoveryRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	expired, expiredErr := scanDiscoveryRun(tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE status='queued'
		   AND security_window_end IS NOT NULL
		   AND security_window_end <= now()
		 ORDER BY queued_at,run_id
		 FOR UPDATE SKIP LOCKED
		 LIMIT 1`))
	if expiredErr == nil {
		expired.Revision++
		expired.Status = config.DiscoveryStatusBlocked
		expired.ErrorMessage = "approved security window expired before worker lease"
		now := time.Now().UTC()
		expired.UpdatedAt = now
		expired.CompletedAt = now
		if _, err := tx.ExecContext(ctx, `
			UPDATE asset_discovery_runs
			   SET status='blocked',revision=$2,error_message=$3,
			       updated_at=$4,completed_at=$4
			 WHERE run_id=$1`,
			expired.RunID, expired.Revision, expired.ErrorMessage, now,
		); err != nil {
			return nil, err
		}
		detailJSON, _ := json.Marshal(map[string]any{"worker_id": workerID})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO asset_discovery_run_history(
				run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
			) VALUES ($1,$2,'queued','blocked',$3,$4,$5,$6,$7::jsonb)`,
			expired.RunID, expired.TenantID, expired.Revision, workerID,
			expired.ErrorMessage, expired.TraceID, string(detailJSON),
		); err != nil {
			return nil, err
		}
		if err := insertDiscoveryStateOutbox(
			ctx, tx, expired, config.DiscoveryStatusBlocked, expired.Revision, expired.TraceID,
		); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return expired, nil
	}
	if !errors.Is(expiredErr, sql.ErrNoRows) {
		return nil, expiredErr
	}
	row := tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE (
		       status='queued'
		       OR (status='running' AND locked_until < now())
		     )
		   AND cancel_requested=false
		   AND (security_window_start IS NULL OR security_window_start <= now())
		   AND (security_window_end IS NULL OR security_window_end > now())
		 ORDER BY queued_at,run_id
		 FOR UPDATE SKIP LOCKED
		 LIMIT 1`)
	run, err := scanDiscoveryRun(row)
	if err != nil {
		return nil, err
	}
	fromStatus := run.Status
	run.Status = config.DiscoveryStatusRunning
	run.Revision++
	now := time.Now().UTC()
	run.StartedAt = now
	run.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		   SET status='running',revision=$2,started_at=$3,updated_at=$3,
		       locked_by=$4,locked_until=$5
		 WHERE run_id=$1`,
		run.RunID, run.Revision, now, workerID, now.Add(lease),
	); err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(map[string]any{"worker_id": workerID, "lease_reclaimed": fromStatus == config.DiscoveryStatusRunning})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,$3,'running',$4,$5,'worker lease acquired',$6,$7::jsonb)`,
		run.RunID, run.TenantID, fromStatus, run.Revision, workerID, run.TraceID, string(detailJSON),
	); err != nil {
		return nil, err
	}
	if err := insertDiscoveryStateOutbox(ctx, tx, run, config.DiscoveryStatusRunning, run.Revision, run.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *AssetRepository) CompleteDiscoveryJob(
	ctx context.Context,
	run *config.DiscoveryRun,
	observations []config.DiscoveryObservation,
	rejected int,
	executionErr error,
	actor string,
) (*config.DiscoveryRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := scanDiscoveryRun(tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1 AND run_id=$2
		 FOR UPDATE`, run.TenantID, run.RunID))
	if err != nil {
		return nil, err
	}
	if current.Status != config.DiscoveryStatusRunning &&
		current.Status != config.DiscoveryStatusCancelRequested {
		return nil, ErrDiscoveryStateConflict
	}
	if current.Status == config.DiscoveryStatusCancelRequested {
		observations = nil
	}

	inserted := 0
	for _, observation := range observations {
		observationJSON, err := json.Marshal(observation)
		if err != nil {
			rejected++
			continue
		}
		fingerprint := discoveryObservationFingerprint(observationJSON)
		candidateID := uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("traffic.asset.discovery:"+run.TenantID+":"+run.RunID+":"+fingerprint),
		)
		result, err := tx.ExecContext(ctx, `
			INSERT INTO asset_discovery_candidates(
				candidate_id,run_id,tenant_id,fingerprint,observation
			) VALUES ($1,$2,$3,$4,$5::jsonb)
			ON CONFLICT (tenant_id,run_id,fingerprint) DO NOTHING`,
			candidateID, run.RunID, run.TenantID, fingerprint, string(observationJSON),
		)
		if err != nil {
			return nil, err
		}
		if count, _ := result.RowsAffected(); count > 0 {
			inserted++
		}
	}
	next := config.DiscoveryStatusSucceeded
	errorMessage := ""
	if current.Status == config.DiscoveryStatusCancelRequested {
		next = config.DiscoveryStatusCancelled
	} else if executionErr != nil {
		errorMessage = executionErr.Error()
		if inserted > 0 {
			next = config.DiscoveryStatusPartial
		} else {
			next = config.DiscoveryStatusFailed
		}
	} else if rejected > 0 {
		next = config.DiscoveryStatusPartial
	}
	nextRevision := current.Revision + 1
	completedAt := time.Now().UTC()
	watermark := fmt.Sprintf("%s:%d", run.RunID, inserted)
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		   SET status=$3,revision=$4,discovered_candidates=$5,
		       rejected_records=$6,result_watermark=$7,error_message=NULLIF($8,''),
		       locked_by='',locked_until=NULL,updated_at=$9,completed_at=$9
		 WHERE tenant_id=$1 AND run_id=$2`,
		run.TenantID, run.RunID, next, nextRevision, inserted, rejected,
		watermark, errorMessage, completedAt,
	); err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(map[string]any{
		"candidate_count":  inserted,
		"rejected_records": rejected,
		"error":            errorMessage,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_run_history(
			run_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
		) VALUES ($1,$2,$3,$4,$5,$6,'worker terminal transition',$7,$8::jsonb)`,
		run.RunID, run.TenantID, current.Status, next, nextRevision, actor,
		run.TraceID, string(detailJSON),
	); err != nil {
		return nil, err
	}
	if err := insertDiscoveryStateOutbox(ctx, tx, current, next, nextRevision, run.TraceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetDiscoveryRun(ctx, run.TenantID, run.RunID)
}

func (r *AssetRepository) ListDiscoveryCandidates(
	ctx context.Context,
	tenantID, runID string,
	limit int,
) ([]*config.DiscoveryCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT candidate_id::text,run_id,tenant_id,fingerprint,observation,
		       status,revision,source_asset_id::text,decision_reason,decided_by,
		       discovered_at,decided_at
		  FROM asset_discovery_candidates
		 WHERE tenant_id=$1 AND run_id=$2
		 ORDER BY discovered_at,candidate_id
		 LIMIT $3`, tenantID, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*config.DiscoveryCandidate
	for rows.Next() {
		var candidate config.DiscoveryCandidate
		var observation []byte
		var sourceAsset sql.NullString
		var decidedAt sql.NullTime
		if err := rows.Scan(
			&candidate.CandidateID, &candidate.RunID, &candidate.TenantID,
			&candidate.Fingerprint, &observation, &candidate.Status,
			&candidate.Revision, &sourceAsset, &candidate.DecisionReason,
			&candidate.DecidedBy, &candidate.DiscoveredAt, &decidedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(observation, &candidate.Observation); err != nil {
			return nil, err
		}
		candidate.SourceAssetID = sourceAsset.String
		if decidedAt.Valid {
			candidate.DecidedAt = decidedAt.Time
		}
		result = append(result, &candidate)
	}
	return result, rows.Err()
}

func (r *AssetRepository) ListDiscoveryRunHistory(
	ctx context.Context,
	tenantID, runID string,
) ([]*config.DiscoveryRunHistory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT transition_id,run_id,tenant_id,from_status,to_status,revision,
		       actor,reason,trace_id,detail,created_at
		  FROM asset_discovery_run_history
		 WHERE tenant_id=$1 AND run_id=$2
		 ORDER BY revision`, tenantID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*config.DiscoveryRunHistory
	for rows.Next() {
		var item config.DiscoveryRunHistory
		var detail []byte
		if err := rows.Scan(
			&item.TransitionID, &item.RunID, &item.TenantID, &item.FromStatus,
			&item.ToStatus, &item.Revision, &item.Actor, &item.Reason,
			&item.TraceID, &detail, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(detail, &item.Detail); err != nil {
			return nil, err
		}
		result = append(result, &item)
	}
	return result, rows.Err()
}

func getDiscoveryRunByIdempotencyTx(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, idempotencyKey string,
) (*config.DiscoveryRun, error) {
	return scanDiscoveryRun(tx.QueryRowContext(ctx, `
		SELECT `+discoveryRunColumns+`
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, idempotencyKey,
	))
}

type discoveryRunScanner interface {
	Scan(dest ...any) error
}

func scanDiscoveryRun(row discoveryRunScanner) (*config.DiscoveryRun, error) {
	var run config.DiscoveryRun
	var targetCIDR, credentialID, approvedBy, resultWatermark, errorMessage sql.NullString
	var securityFrom, securityTo, completedAt sql.NullTime
	if err := row.Scan(
		&run.RunID, &run.TenantID, &run.Mode, &targetCIDR, &credentialID,
		&run.ActionID, &run.Status, &run.Revision, &run.RequestedBy,
		&run.Reason, &run.RateLimit, &securityFrom, &securityTo, &approvedBy,
		&run.TraceID, &run.DiscoveredAssets, &run.DiscoveredLinks,
		&run.CandidateCount, &run.RejectedRecords, &resultWatermark,
		&errorMessage, &run.QueuedAt, &run.StartedAt, &run.UpdatedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}
	run.TargetCIDR = targetCIDR.String
	run.CredentialID = credentialID.String
	run.ApprovedBy = approvedBy.String
	run.ResultWatermark = resultWatermark.String
	run.ErrorMessage = errorMessage.String
	if securityFrom.Valid {
		run.SecurityFrom = securityFrom.Time
	}
	if securityTo.Valid {
		run.SecurityTo = securityTo.Time
	}
	if completedAt.Valid {
		run.CompletedAt = completedAt.Time
	}
	return &run, nil
}

func insertDiscoveryStateOutbox(
	ctx context.Context,
	tx *sql.Tx,
	run *config.DiscoveryRun,
	status string,
	revision int64,
	traceID string,
) error {
	eventID := uuid.New()
	eventType := "traffic.asset.discovery.v1.JobStateChanged"
	partitionKey := run.TenantID + ":" + run.RunID
	payload, _ := json.Marshal(map[string]any{
		"event_id": eventID.String(), "event_type": eventType,
		"schema_version": 1, "aggregate_version": revision,
		"tenant_id": run.TenantID, "run_id": run.RunID,
		"resource_type": "run", "resource_id": run.RunID,
		"action_id": run.ActionID, "revision": revision,
		"partition_key": partitionKey, "trace_id": traceID, "status": status,
	})
	_, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_outbox(
			event_id,run_id,resource_type,resource_id,action_id,tenant_id,
			aggregate_version,schema_version,partition_key,event_type,payload
		) VALUES ($1,$2,'run',$2,$3,$4,$5,1,$6,$7,$8::jsonb)`,
		eventID, run.RunID, run.ActionID, run.TenantID, revision, partitionKey, eventType, string(payload),
	)
	return err
}

func discoveryObservationFingerprint(payload []byte) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum[:])
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
