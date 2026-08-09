package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	assetProjectionOpenSearch = "opensearch"
	assetProjectionNebula     = "nebulagraph"
)

// AssetProjectionTarget renders a deterministic target document and applies it
// idempotently. Apply may be called again when the external write succeeded but
// the PostgreSQL watermark transaction was interrupted.
type AssetProjectionTarget interface {
	Name() string
	Projection(AssetUpsertedV2) ([]byte, error)
	Apply(context.Context, AssetUpsertedV2, []byte) error
}

type AssetProjectionWorkerConfig struct {
	WorkerID    string
	Lease       time.Duration
	Interval    time.Duration
	MaxAttempts int
	Logger      *zap.Logger
}

type AssetProjectionWorker struct {
	db      *sql.DB
	targets map[string]AssetProjectionTarget
	cfg     AssetProjectionWorkerConfig
}

type leasedAssetProjection struct {
	EventID          string
	TenantID         string
	AssetID          string
	AggregateVersion int64
	Payload          []byte
	PayloadSHA256    string
	OSStatus         string
	NebulaStatus     string
	AttemptCount     int
}

func NewAssetProjectionWorker(
	db *sql.DB,
	targets []AssetProjectionTarget,
	cfg AssetProjectionWorkerConfig,
) (*AssetProjectionWorker, error) {
	if db == nil {
		return nil, fmt.Errorf("asset projection database is required")
	}
	if cfg.WorkerID == "" {
		return nil, fmt.Errorf("asset projection worker_id is required")
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 45 * time.Second
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	targetMap := make(map[string]AssetProjectionTarget, len(targets))
	for _, target := range targets {
		if target == nil {
			return nil, fmt.Errorf("asset projection target is nil")
		}
		name := target.Name()
		if name != assetProjectionOpenSearch && name != assetProjectionNebula {
			return nil, fmt.Errorf("unsupported asset projection target %q", name)
		}
		if _, exists := targetMap[name]; exists {
			return nil, fmt.Errorf("duplicate asset projection target %q", name)
		}
		targetMap[name] = target
	}
	if len(targetMap) != 2 {
		return nil, fmt.Errorf("asset projection requires opensearch and nebulagraph targets")
	}
	return &AssetProjectionWorker{db: db, targets: targetMap, cfg: cfg}, nil
}

func (w *AssetProjectionWorker) VerifySchema(ctx context.Context) error {
	var inboxColumns, watermarkColumns int
	if err := w.db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE table_name='asset_projection_inbox'),
		  count(*) FILTER (WHERE table_name='asset_projection_watermarks')
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND (
		    (table_name='asset_projection_inbox' AND column_name IN (
		      'event_id','tenant_id','asset_id','aggregate_version','schema_version',
		      'partition_key','trace_id','payload','payload_sha256','kafka_partition',
		      'kafka_offset','os_status','nebula_status','status','attempt_count',
		      'available_at','locked_by','locked_until','last_error','created_at',
		      'updated_at','applied_at'
		    ))
		    OR
		    (table_name='asset_projection_watermarks' AND column_name IN (
		      'tenant_id','asset_id','target','aggregate_version','event_id',
		      'projection_sha256','applied_at'
		    ))
		  )`,
	).Scan(&inboxColumns, &watermarkColumns); err != nil {
		return fmt.Errorf("verify asset projection schema: %w", err)
	}
	if inboxColumns != 22 || watermarkColumns != 7 {
		return fmt.Errorf(
			"asset projection schema incomplete: inbox_columns=%d/22 watermark_columns=%d/7",
			inboxColumns,
			watermarkColumns,
		)
	}
	return nil
}

func (w *AssetProjectionWorker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			found, err := w.ProjectNext(ctx)
			if err != nil {
				w.cfg.Logger.Warn("Asset projection attempt failed", zap.Error(err))
			}
			if found {
				timer.Reset(time.Millisecond)
			} else {
				timer.Reset(w.cfg.Interval)
			}
		}
	}
}

// ProjectNext leases the oldest runnable revision for one asset. A later
// revision cannot be leased while an earlier revision is pending or processing,
// preventing the Nebula target from being overwritten out of order.
func (w *AssetProjectionWorker) ProjectNext(ctx context.Context) (bool, error) {
	row, found, err := w.leaseNext(ctx)
	if err != nil || !found {
		return found, err
	}

	var attemptErrors []error
	if row.OSStatus == "pending" {
		if err := w.applyTarget(ctx, row, w.targets[assetProjectionOpenSearch]); err != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("opensearch: %w", err))
			if recordErr := w.recordTargetFailure(ctx, row, assetProjectionOpenSearch, err); recordErr != nil {
				attemptErrors = append(attemptErrors, recordErr)
			}
		}
	}
	if row.NebulaStatus == "pending" {
		if err := w.applyTarget(ctx, row, w.targets[assetProjectionNebula]); err != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("nebulagraph: %w", err))
			if recordErr := w.recordTargetFailure(ctx, row, assetProjectionNebula, err); recordErr != nil {
				attemptErrors = append(attemptErrors, recordErr)
			}
		}
	}

	if err := w.finishAttempt(ctx, row, attemptErrors); err != nil {
		attemptErrors = append(attemptErrors, err)
	}
	return true, errors.Join(attemptErrors...)
}

func (w *AssetProjectionWorker) leaseNext(ctx context.Context) (leasedAssetProjection, bool, error) {
	var row leasedAssetProjection
	leaseSeconds := int64(w.cfg.Lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	err := w.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT current.event_id
		  FROM asset_projection_inbox current
		  WHERE (
		      (current.status='pending' AND current.available_at<=now())
		      OR (current.status='processing' AND current.locked_until<now())
		    )
		    AND (current.os_status='pending' OR current.nebula_status='pending')
		    AND NOT EXISTS (
		      SELECT 1
		      FROM asset_projection_inbox prior
		      WHERE prior.tenant_id=current.tenant_id
		        AND prior.asset_id=current.asset_id
		        AND prior.aggregate_version<current.aggregate_version
		        AND prior.status IN ('pending','processing')
		    )
		  ORDER BY current.available_at,current.created_at,current.event_id
		  FOR UPDATE SKIP LOCKED
		  LIMIT 1
		)
		UPDATE asset_projection_inbox inbox
		SET status='processing',
		    attempt_count=inbox.attempt_count+1,
		    locked_by=$1,
		    locked_until=now()+($2*interval '1 second'),
		    updated_at=now()
		FROM candidate
		WHERE inbox.event_id=candidate.event_id
		RETURNING inbox.event_id::text,inbox.tenant_id,inbox.asset_id::text,
		          inbox.aggregate_version,inbox.payload::text,inbox.payload_sha256,inbox.os_status,
		          inbox.nebula_status,inbox.attempt_count`,
		w.cfg.WorkerID, leaseSeconds,
	).Scan(
		&row.EventID, &row.TenantID, &row.AssetID, &row.AggregateVersion,
		&row.Payload, &row.PayloadSHA256, &row.OSStatus, &row.NebulaStatus, &row.AttemptCount,
	)
	if err == sql.ErrNoRows {
		return leasedAssetProjection{}, false, nil
	}
	if err != nil {
		return leasedAssetProjection{}, false, fmt.Errorf("lease asset projection: %w", err)
	}
	return row, true, nil
}

func (w *AssetProjectionWorker) applyTarget(
	ctx context.Context,
	row leasedAssetProjection,
	target AssetProjectionTarget,
) error {
	var event AssetUpsertedV2
	canonicalPayload, err := canonicalJSON(row.Payload)
	if err != nil {
		return fmt.Errorf("canonicalize durable payload: %w", err)
	}
	payloadHash := sha256.Sum256(canonicalPayload)
	if hex.EncodeToString(payloadHash[:]) != row.PayloadSHA256 {
		return fmt.Errorf("durable payload checksum mismatch")
	}
	if err := json.Unmarshal(row.Payload, &event); err != nil {
		return fmt.Errorf("decode durable payload: %w", err)
	}
	if err := validateAssetProjectionEvent(event); err != nil {
		return err
	}
	if event.EventID != row.EventID ||
		event.TenantID != row.TenantID ||
		event.AssetID != row.AssetID ||
		event.AggregateVersion != row.AggregateVersion {
		return fmt.Errorf("durable payload does not match inbox identity")
	}
	projection, err := target.Projection(event)
	if err != nil {
		return fmt.Errorf("render projection: %w", err)
	}
	hash := sha256.Sum256(projection)
	projectionSHA := hex.EncodeToString(hash[:])

	state, err := w.readWatermark(ctx, row, target.Name())
	if err != nil {
		return err
	}
	if state.exists {
		switch {
		case state.version > row.AggregateVersion:
			return w.markTargetApplied(ctx, row, target.Name(), projectionSHA, true)
		case state.version == row.AggregateVersion:
			if state.eventID != row.EventID || state.projectionSHA != projectionSHA {
				return fmt.Errorf("projection watermark identity collision")
			}
			return w.markTargetApplied(ctx, row, target.Name(), projectionSHA, false)
		}
	}

	if err := target.Apply(ctx, event, projection); err != nil {
		return err
	}
	return w.markTargetApplied(ctx, row, target.Name(), projectionSHA, false)
}

type projectionWatermark struct {
	exists        bool
	version       int64
	eventID       string
	projectionSHA string
}

func (w *AssetProjectionWorker) readWatermark(
	ctx context.Context,
	row leasedAssetProjection,
	target string,
) (projectionWatermark, error) {
	var state projectionWatermark
	err := w.db.QueryRowContext(ctx, `
		SELECT aggregate_version,event_id::text,projection_sha256
		FROM asset_projection_watermarks
		WHERE tenant_id=$1 AND asset_id=$2 AND target=$3`,
		row.TenantID, row.AssetID, target,
	).Scan(&state.version, &state.eventID, &state.projectionSHA)
	if err == sql.ErrNoRows {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read %s projection watermark: %w", target, err)
	}
	state.exists = true
	return state, nil
}

func (w *AssetProjectionWorker) markTargetApplied(
	ctx context.Context,
	row leasedAssetProjection,
	target string,
	projectionSHA string,
	superseded bool,
) error {
	column, err := targetStatusColumn(target)
	if err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin %s watermark: %w", target, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:%s", len(row.TenantID), row.TenantID, row.AssetID),
	); err != nil {
		return fmt.Errorf("lock %s watermark: %w", target, err)
	}

	var existingVersion int64
	var existingEvent, existingSHA string
	err = tx.QueryRowContext(ctx, `
		SELECT aggregate_version,event_id::text,projection_sha256
		FROM asset_projection_watermarks
		WHERE tenant_id=$1 AND asset_id=$2 AND target=$3
		FOR UPDATE`,
		row.TenantID, row.AssetID, target,
	).Scan(&existingVersion, &existingEvent, &existingSHA)
	switch {
	case err == sql.ErrNoRows:
		if superseded {
			return fmt.Errorf("%s watermark disappeared while marking superseded event", target)
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO asset_projection_watermarks (
			  tenant_id,asset_id,target,aggregate_version,event_id,projection_sha256
			) VALUES ($1,$2,$3,$4,$5,$6)`,
			row.TenantID, row.AssetID, target, row.AggregateVersion, row.EventID, projectionSHA,
		); err != nil {
			return fmt.Errorf("insert %s watermark: %w", target, err)
		}
	case err != nil:
		return fmt.Errorf("lock %s watermark row: %w", target, err)
	case existingVersion > row.AggregateVersion:
		// A newer authoritative revision has already won. The older inbox row
		// is complete for this target without regressing the watermark.
	case existingVersion == row.AggregateVersion:
		if existingEvent != row.EventID || existingSHA != projectionSHA {
			return fmt.Errorf("%s watermark identity collision", target)
		}
	default:
		if _, err = tx.ExecContext(ctx, `
			UPDATE asset_projection_watermarks
			SET aggregate_version=$4,event_id=$5,projection_sha256=$6,applied_at=now()
			WHERE tenant_id=$1 AND asset_id=$2 AND target=$3`,
			row.TenantID, row.AssetID, target, row.AggregateVersion, row.EventID, projectionSHA,
		); err != nil {
			return fmt.Errorf("advance %s watermark: %w", target, err)
		}
	}

	result, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE asset_projection_inbox
		SET %s='applied',updated_at=now()
		WHERE event_id=$1 AND status='processing' AND locked_by=$2`, column),
		row.EventID, w.cfg.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("mark %s projection applied: %w", target, err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("%s projection lease collision", target)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s projection watermark: %w", target, err)
	}
	return nil
}

func (w *AssetProjectionWorker) recordTargetFailure(
	ctx context.Context,
	row leasedAssetProjection,
	target string,
	targetErr error,
) error {
	column, err := targetStatusColumn(target)
	if err != nil {
		return err
	}
	status := "pending"
	if row.AttemptCount >= w.cfg.MaxAttempts {
		status = "dead"
	}
	result, err := w.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE asset_projection_inbox
		SET %s=$3,last_error=$4,updated_at=now()
		WHERE event_id=$1 AND status='processing' AND locked_by=$2`, column),
		row.EventID, w.cfg.WorkerID, status, targetErr.Error(),
	)
	if err != nil {
		return fmt.Errorf("record %s projection failure: %w", target, err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("%s projection failure lease collision", target)
	}
	return nil
}

func (w *AssetProjectionWorker) finishAttempt(
	ctx context.Context,
	row leasedAssetProjection,
	attemptErrors []error,
) error {
	retrySeconds := 1 << min(row.AttemptCount, 8)
	lastError := ""
	if joined := errors.Join(attemptErrors...); joined != nil {
		lastError = joined.Error()
	}
	result, err := w.db.ExecContext(ctx, `
		UPDATE asset_projection_inbox
		SET status=CASE
		      WHEN os_status='applied' AND nebula_status='applied' THEN 'applied'
		      WHEN os_status='dead' OR nebula_status='dead' THEN 'dead'
		      ELSE 'pending'
		    END,
		    applied_at=CASE
		      WHEN os_status='applied' AND nebula_status='applied' THEN now()
		      ELSE applied_at
		    END,
		    available_at=CASE
		      WHEN os_status='pending' OR nebula_status='pending'
		        THEN now()+($3*interval '1 second')
		      ELSE available_at
		    END,
		    locked_by='',
		    locked_until=NULL,
		    last_error=$4,
		    updated_at=now()
		WHERE event_id=$1 AND status='processing' AND locked_by=$2`,
		row.EventID, w.cfg.WorkerID, retrySeconds, lastError,
	)
	if err != nil {
		return fmt.Errorf("finalize asset projection attempt: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("asset projection finalization lease collision")
	}
	return nil
}

func targetStatusColumn(target string) (string, error) {
	switch target {
	case assetProjectionOpenSearch:
		return "os_status", nil
	case assetProjectionNebula:
		return "nebula_status", nil
	default:
		return "", fmt.Errorf("unsupported projection target %q", target)
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
