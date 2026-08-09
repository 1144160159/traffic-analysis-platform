package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	campaignProjectionClickHouse = "clickhouse"
	campaignProjectionOpenSearch = "opensearch"
	campaignProjectionNebula     = "nebulagraph"
)

var campaignProjectionTargetOrder = []string{
	campaignProjectionClickHouse,
	campaignProjectionOpenSearch,
	campaignProjectionNebula,
}

// CampaignProjectionEvent is the immutable inbox event presented to each
// external target. ProjectionVersion is aggregate_revision for aggregate
// events and relation_revision for membership events; ProjectionKey keeps the
// two independent ordering domains disjoint.
type CampaignProjectionEvent struct {
	Stream            string          `json:"stream"`
	EventID           string          `json:"event_id"`
	TenantID          string          `json:"tenant_id"`
	AggregateID       string          `json:"aggregate_id"`
	CampaignID        string          `json:"campaign_id"`
	RelationID        string          `json:"relation_id,omitempty"`
	AlertID           string          `json:"alert_id,omitempty"`
	EventType         string          `json:"event_type"`
	SchemaVersion     int             `json:"schema_version"`
	AggregateRevision int64           `json:"aggregate_revision"`
	RelationRevision  int64           `json:"relation_revision"`
	PartitionKey      string          `json:"partition_key"`
	TraceID           string          `json:"trace_id"`
	Payload           json.RawMessage `json:"payload"`
	ReceivedAt        time.Time       `json:"received_at"`
}

func (event CampaignProjectionEvent) ProjectionKey() string {
	if event.Stream == campaignMembershipStream {
		return "relation:" + event.RelationID
	}
	return "campaign:" + event.CampaignID
}

func (event CampaignProjectionEvent) ProjectionVersion() int64 {
	if event.Stream == campaignMembershipStream {
		return event.RelationRevision
	}
	return event.AggregateRevision
}

// CampaignProjectionTarget renders deterministic bytes and applies them
// idempotently. Apply can be repeated if the external effect succeeded but the
// PostgreSQL watermark transaction was interrupted.
type CampaignProjectionTarget interface {
	Name() string
	Projection(CampaignProjectionEvent) ([]byte, error)
	Apply(context.Context, CampaignProjectionEvent, []byte) error
}

type CampaignTargetProjectionWorkerConfig struct {
	WorkerID    string
	Lease       time.Duration
	Interval    time.Duration
	MaxAttempts int
	Logger      *zap.Logger
}

type CampaignTargetProjectionWorker struct {
	db      *sql.DB
	targets map[string]CampaignProjectionTarget
	cfg     CampaignTargetProjectionWorkerConfig
}

type leasedCampaignProjection struct {
	Event         CampaignProjectionEvent
	TargetStatus  map[string]string
	AttemptCount  int
	ValidationErr error
}

func NewCampaignTargetProjectionWorker(
	db *sql.DB,
	targets []CampaignProjectionTarget,
	cfg CampaignTargetProjectionWorkerConfig,
) (*CampaignTargetProjectionWorker, error) {
	if db == nil {
		return nil, fmt.Errorf("campaign target projection database is required")
	}
	if strings.TrimSpace(cfg.WorkerID) == "" {
		return nil, fmt.Errorf("campaign target projection worker_id is required")
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
	targetMap := make(map[string]CampaignProjectionTarget, len(targets))
	for _, target := range targets {
		if target == nil {
			return nil, fmt.Errorf("campaign projection target is nil")
		}
		name := target.Name()
		if name != campaignProjectionClickHouse && name != campaignProjectionOpenSearch && name != campaignProjectionNebula {
			return nil, fmt.Errorf("unsupported campaign projection target %q", name)
		}
		if _, exists := targetMap[name]; exists {
			return nil, fmt.Errorf("duplicate campaign projection target %q", name)
		}
		targetMap[name] = target
	}
	if len(targetMap) != len(campaignProjectionTargetOrder) {
		return nil, fmt.Errorf("campaign projection requires clickhouse, opensearch and nebulagraph targets")
	}
	return &CampaignTargetProjectionWorker{db: db, targets: targetMap, cfg: cfg}, nil
}

func (worker *CampaignTargetProjectionWorker) VerifySchema(ctx context.Context) error {
	var inboxColumns, watermarkColumns int
	err := worker.db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE table_name='campaign_event_projection_inbox'),
		  count(*) FILTER (WHERE table_name='campaign_target_projection_watermarks')
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
		  (table_name='campaign_event_projection_inbox' AND column_name IN (
		    'stream','event_id','tenant_id','aggregate_id','campaign_id','relation_id','alert_id',
		    'event_type','schema_version','aggregate_revision','relation_revision','partition_key',
		    'trace_id','payload','projection_status','target_status','attempt_count','available_at',
		    'locked_by','locked_until','last_error','applied_at','received_at','updated_at'
		  )) OR
		  (table_name='campaign_target_projection_watermarks' AND column_name IN (
		    'tenant_id','projection_key','target','stream','projection_version','event_id',
		    'projection_sha256','applied_at'
		  ))
		)`).Scan(&inboxColumns, &watermarkColumns)
	if err != nil {
		return fmt.Errorf("verify campaign target projection schema: %w", err)
	}
	if inboxColumns != 24 || watermarkColumns != 8 {
		return fmt.Errorf("campaign target projection schema incomplete: inbox_columns=%d/24 watermark_columns=%d/8", inboxColumns, watermarkColumns)
	}
	return nil
}

func (worker *CampaignTargetProjectionWorker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			found, err := worker.ProjectNext(ctx)
			if err != nil {
				worker.cfg.Logger.Warn("Campaign target projection attempt failed", zap.Error(err))
			}
			if found {
				timer.Reset(time.Millisecond)
			} else {
				timer.Reset(worker.cfg.Interval)
			}
		}
	}
}

func (worker *CampaignTargetProjectionWorker) ProjectNext(ctx context.Context) (bool, error) {
	row, found, err := worker.leaseNext(ctx)
	if err != nil || !found {
		return found, err
	}
	var attemptErrors []error
	if row.ValidationErr != nil {
		attemptErrors = append(attemptErrors, row.ValidationErr)
		for _, targetName := range campaignProjectionTargetOrder {
			if status := row.TargetStatus[targetName]; status == "applied" || status == "dead" {
				continue
			}
			if recordErr := worker.recordTargetFailure(ctx, row, targetName, row.ValidationErr); recordErr != nil {
				attemptErrors = append(attemptErrors, recordErr)
			}
		}
		if finishErr := worker.finishAttempt(ctx, row, attemptErrors); finishErr != nil {
			attemptErrors = append(attemptErrors, finishErr)
		}
		return true, errors.Join(attemptErrors...)
	}
	for _, targetName := range campaignProjectionTargetOrder {
		if row.TargetStatus[targetName] != "pending" {
			continue
		}
		if err := worker.applyTarget(ctx, row, worker.targets[targetName]); err != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("%s: %w", targetName, err))
			if recordErr := worker.recordTargetFailure(ctx, row, targetName, err); recordErr != nil {
				attemptErrors = append(attemptErrors, recordErr)
			}
		}
	}
	if err := worker.finishAttempt(ctx, row, attemptErrors); err != nil {
		attemptErrors = append(attemptErrors, err)
	}
	return true, errors.Join(attemptErrors...)
}

func (worker *CampaignTargetProjectionWorker) leaseNext(ctx context.Context) (leasedCampaignProjection, bool, error) {
	var row leasedCampaignProjection
	var payload, relationID, targetStatus string
	leaseSeconds := int64(worker.cfg.Lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	err := worker.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT current.stream,current.event_id
		  FROM campaign_event_projection_inbox current
		  WHERE (
		      (current.projection_status IN ('pending','partial') AND current.available_at<=now())
		      OR (current.projection_status='processing' AND current.locked_until<now())
		    )
		    AND EXISTS (
		      SELECT 1 FROM jsonb_each_text(current.target_status) target
		      WHERE target.key IN ('clickhouse','opensearch','nebulagraph') AND target.value='pending'
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM campaign_event_projection_inbox prior
		      WHERE prior.tenant_id=current.tenant_id
		        AND prior.stream=current.stream
		        AND prior.aggregate_id=current.aggregate_id
		        AND (CASE WHEN prior.stream='aggregate' THEN prior.aggregate_revision ELSE prior.relation_revision END)
		            < (CASE WHEN current.stream='aggregate' THEN current.aggregate_revision ELSE current.relation_revision END)
		        AND prior.projection_status IN ('pending','processing','partial')
		    )
		  ORDER BY current.available_at,current.received_at,current.stream,current.event_id
		  FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE campaign_event_projection_inbox inbox
		SET projection_status='processing',attempt_count=inbox.attempt_count+1,
		    locked_by=$1,locked_until=now()+($2*interval '1 second'),updated_at=now()
		FROM candidate
		WHERE inbox.stream=candidate.stream AND inbox.event_id=candidate.event_id
		RETURNING inbox.stream,inbox.event_id::text,inbox.tenant_id,inbox.aggregate_id,
		          inbox.campaign_id,coalesce(inbox.relation_id::text,''),inbox.alert_id,
		          inbox.event_type,inbox.schema_version,inbox.aggregate_revision,
		          inbox.relation_revision,inbox.partition_key,inbox.trace_id,inbox.payload::text,
		          inbox.target_status::text,inbox.attempt_count,inbox.received_at`, worker.cfg.WorkerID, leaseSeconds).Scan(
		&row.Event.Stream, &row.Event.EventID, &row.Event.TenantID, &row.Event.AggregateID,
		&row.Event.CampaignID, &relationID, &row.Event.AlertID, &row.Event.EventType,
		&row.Event.SchemaVersion, &row.Event.AggregateRevision, &row.Event.RelationRevision,
		&row.Event.PartitionKey, &row.Event.TraceID, &payload, &targetStatus, &row.AttemptCount,
		&row.Event.ReceivedAt,
	)
	if err == sql.ErrNoRows {
		return leasedCampaignProjection{}, false, nil
	}
	if err != nil {
		return leasedCampaignProjection{}, false, fmt.Errorf("lease campaign projection: %w", err)
	}
	row.Event.RelationID = relationID
	row.Event.Payload = json.RawMessage(payload)
	if err := json.Unmarshal([]byte(targetStatus), &row.TargetStatus); err != nil {
		row.TargetStatus = make(map[string]string, len(campaignProjectionTargetOrder))
		row.ValidationErr = fmt.Errorf("decode campaign target status: %w", err)
	}
	for _, target := range campaignProjectionTargetOrder {
		if status := row.TargetStatus[target]; status != "pending" && status != "applied" && status != "dead" {
			row.ValidationErr = errors.Join(
				row.ValidationErr,
				fmt.Errorf("invalid %s campaign target status %q", target, status),
			)
		}
	}
	if err := validateDurableCampaignProjection(row.Event); err != nil {
		row.ValidationErr = errors.Join(row.ValidationErr, err)
	}
	return row, true, nil
}

type campaignProjectionPayloadIdentity struct {
	EventID          string `json:"event_id"`
	TenantID         string `json:"tenant_id"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	CampaignID       string `json:"campaign_id"`
	RelationID       string `json:"relation_id"`
	AlertID          string `json:"alert_id"`
	RelationRevision int64  `json:"relation_revision"`
	CampaignRevision int64  `json:"campaign_revision"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	PartitionKey     string `json:"partition_key"`
	TraceID          string `json:"trace_id"`
}

func validateDurableCampaignProjection(event CampaignProjectionEvent) error {
	if event.Stream != campaignAggregateStream && event.Stream != campaignMembershipStream {
		return fmt.Errorf("invalid campaign projection stream %q", event.Stream)
	}
	if event.EventID == "" || event.TenantID == "" || event.CampaignID == "" || event.EventType == "" ||
		event.PartitionKey != event.TenantID+":"+event.CampaignID || event.SchemaVersion != 2 || event.TraceID == "" ||
		event.ReceivedAt.IsZero() {
		return fmt.Errorf("invalid durable campaign projection identity")
	}
	var identity campaignProjectionPayloadIdentity
	if err := json.Unmarshal(event.Payload, &identity); err != nil {
		return fmt.Errorf("decode durable campaign projection payload: %w", err)
	}
	if identity.EventID != event.EventID || identity.TenantID != event.TenantID ||
		identity.AggregateID != event.CampaignID || identity.AggregateVersion != event.AggregateRevision ||
		identity.CampaignID != event.CampaignID || identity.EventType != event.EventType ||
		identity.SchemaVersion != event.SchemaVersion || identity.PartitionKey != event.PartitionKey ||
		identity.TraceID != event.TraceID {
		return fmt.Errorf("durable campaign projection payload identity collision")
	}
	if event.Stream == campaignAggregateStream {
		if event.AggregateID != event.CampaignID || event.AggregateRevision <= 0 ||
			event.RelationID != "" || event.RelationRevision != 0 {
			return fmt.Errorf("invalid aggregate campaign projection identity")
		}
		return nil
	}
	if event.AggregateID != event.RelationID || event.RelationID == "" || event.AlertID == "" ||
		event.RelationRevision <= 0 || identity.RelationID != event.RelationID ||
		identity.AlertID != event.AlertID || identity.RelationRevision != event.RelationRevision ||
		identity.CampaignRevision != event.AggregateRevision {
		return fmt.Errorf("invalid membership campaign projection identity")
	}
	return nil
}

func (worker *CampaignTargetProjectionWorker) applyTarget(
	ctx context.Context,
	row leasedCampaignProjection,
	target CampaignProjectionTarget,
) error {
	projection, err := target.Projection(row.Event)
	if err != nil {
		return fmt.Errorf("render projection: %w", err)
	}
	hash := sha256.Sum256(projection)
	projectionSHA := hex.EncodeToString(hash[:])
	watermark, err := worker.readTargetWatermark(ctx, row.Event, target.Name())
	if err != nil {
		return err
	}
	version := row.Event.ProjectionVersion()
	if watermark.exists {
		switch {
		case watermark.version > version:
			return worker.markTargetApplied(ctx, row, target.Name(), projectionSHA, true)
		case watermark.version == version:
			if watermark.eventID != row.Event.EventID || watermark.projectionSHA != projectionSHA {
				return fmt.Errorf("campaign projection watermark identity collision")
			}
			return worker.markTargetApplied(ctx, row, target.Name(), projectionSHA, false)
		}
	}
	if err := target.Apply(ctx, row.Event, projection); err != nil {
		return err
	}
	return worker.markTargetApplied(ctx, row, target.Name(), projectionSHA, false)
}

type campaignTargetWatermark struct {
	exists        bool
	version       int64
	eventID       string
	projectionSHA string
}

func (worker *CampaignTargetProjectionWorker) readTargetWatermark(
	ctx context.Context,
	event CampaignProjectionEvent,
	target string,
) (campaignTargetWatermark, error) {
	var watermark campaignTargetWatermark
	err := worker.db.QueryRowContext(ctx, `
		SELECT projection_version,event_id::text,projection_sha256
		FROM campaign_target_projection_watermarks
		WHERE tenant_id=$1 AND projection_key=$2 AND target=$3`,
		event.TenantID, event.ProjectionKey(), target).Scan(
		&watermark.version, &watermark.eventID, &watermark.projectionSHA)
	if err == sql.ErrNoRows {
		return watermark, nil
	}
	if err != nil {
		return watermark, fmt.Errorf("read %s campaign projection watermark: %w", target, err)
	}
	watermark.exists = true
	return watermark, nil
}

func (worker *CampaignTargetProjectionWorker) markTargetApplied(
	ctx context.Context,
	row leasedCampaignProjection,
	target, projectionSHA string,
	superseded bool,
) error {
	tx, err := worker.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin %s campaign watermark: %w", target, err)
	}
	defer tx.Rollback()
	lockKey := fmt.Sprintf("%d:%s:%s", len(row.Event.TenantID), row.Event.TenantID, row.Event.ProjectionKey())
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return fmt.Errorf("lock %s campaign watermark: %w", target, err)
	}
	var existingVersion int64
	var existingEvent, existingSHA string
	err = tx.QueryRowContext(ctx, `
		SELECT projection_version,event_id::text,projection_sha256
		FROM campaign_target_projection_watermarks
		WHERE tenant_id=$1 AND projection_key=$2 AND target=$3 FOR UPDATE`,
		row.Event.TenantID, row.Event.ProjectionKey(), target).Scan(
		&existingVersion, &existingEvent, &existingSHA)
	switch {
	case err == sql.ErrNoRows:
		if superseded {
			return fmt.Errorf("%s campaign watermark disappeared while marking superseded event", target)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO campaign_target_projection_watermarks
			(tenant_id,projection_key,target,stream,projection_version,event_id,projection_sha256)
			VALUES ($1,$2,$3,$4,$5,$6::uuid,$7)`, row.Event.TenantID, row.Event.ProjectionKey(),
			target, row.Event.Stream, row.Event.ProjectionVersion(), row.Event.EventID, projectionSHA)
	case err != nil:
		return fmt.Errorf("lock %s campaign watermark row: %w", target, err)
	case existingVersion > row.Event.ProjectionVersion():
		// A newer projection already won; mark this inbox target superseded.
	case existingVersion == row.Event.ProjectionVersion():
		if existingEvent != row.Event.EventID || existingSHA != projectionSHA {
			return fmt.Errorf("%s campaign watermark identity collision", target)
		}
	default:
		_, err = tx.ExecContext(ctx, `
			UPDATE campaign_target_projection_watermarks
			SET stream=$4,projection_version=$5,event_id=$6::uuid,projection_sha256=$7,applied_at=now()
			WHERE tenant_id=$1 AND projection_key=$2 AND target=$3`, row.Event.TenantID,
			row.Event.ProjectionKey(), target, row.Event.Stream, row.Event.ProjectionVersion(),
			row.Event.EventID, projectionSHA)
	}
	if err != nil {
		return fmt.Errorf("advance %s campaign watermark: %w", target, err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE campaign_event_projection_inbox
		SET target_status=jsonb_set(target_status,ARRAY[$4]::text[],to_jsonb('applied'::text),false),updated_at=now()
		WHERE stream=$1 AND event_id=$2::uuid AND projection_status='processing' AND locked_by=$3`,
		row.Event.Stream, row.Event.EventID, worker.cfg.WorkerID, target)
	if err != nil {
		return fmt.Errorf("mark %s campaign projection applied: %w", target, err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("%s campaign projection lease collision", target)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s campaign watermark: %w", target, err)
	}
	return nil
}

func (worker *CampaignTargetProjectionWorker) recordTargetFailure(
	ctx context.Context,
	row leasedCampaignProjection,
	target string,
	targetErr error,
) error {
	status := "pending"
	if row.AttemptCount >= worker.cfg.MaxAttempts {
		status = "dead"
	}
	result, err := worker.db.ExecContext(ctx, `
		UPDATE campaign_event_projection_inbox
		SET target_status=jsonb_set(target_status,ARRAY[$4]::text[],to_jsonb($5::text),true),
		    last_error=$6,updated_at=now()
		WHERE stream=$1 AND event_id=$2::uuid AND projection_status='processing' AND locked_by=$3`,
		row.Event.Stream, row.Event.EventID, worker.cfg.WorkerID, target, status, targetErr.Error())
	if err != nil {
		return fmt.Errorf("record %s campaign projection failure: %w", target, err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("%s campaign projection failure lease collision", target)
	}
	return nil
}

func (worker *CampaignTargetProjectionWorker) finishAttempt(
	ctx context.Context,
	row leasedCampaignProjection,
	attemptErrors []error,
) error {
	retrySeconds := 1 << min(row.AttemptCount, 8)
	lastError := ""
	if joined := errors.Join(attemptErrors...); joined != nil {
		lastError = joined.Error()
	}
	result, err := worker.db.ExecContext(ctx, `
		UPDATE campaign_event_projection_inbox
		SET projection_status=CASE
		      WHEN target_status->>'clickhouse'='applied'
		       AND target_status->>'opensearch'='applied'
		       AND target_status->>'nebulagraph'='applied' THEN 'applied'
		      WHEN target_status->>'clickhouse'='dead'
		        OR target_status->>'opensearch'='dead'
		        OR target_status->>'nebulagraph'='dead' THEN 'dead'
		      WHEN target_status->>'clickhouse'='applied'
		        OR target_status->>'opensearch'='applied'
		        OR target_status->>'nebulagraph'='applied' THEN 'partial'
		      ELSE 'pending'
		    END,
		    applied_at=CASE
		      WHEN target_status->>'clickhouse'='applied'
		       AND target_status->>'opensearch'='applied'
		       AND target_status->>'nebulagraph'='applied' THEN now()
		      ELSE applied_at END,
		    available_at=CASE
		      WHEN target_status->>'clickhouse'='pending'
		        OR target_status->>'opensearch'='pending'
		        OR target_status->>'nebulagraph'='pending'
		      THEN now()+($4*interval '1 second') ELSE available_at END,
		    locked_by='',locked_until=NULL,last_error=$5,updated_at=now()
		WHERE stream=$1 AND event_id=$2::uuid AND projection_status='processing' AND locked_by=$3`,
		row.Event.Stream, row.Event.EventID, worker.cfg.WorkerID, retrySeconds, lastError)
	if err != nil {
		return fmt.Errorf("finalize campaign projection attempt: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("campaign projection finalization lease collision")
	}
	return nil
}
