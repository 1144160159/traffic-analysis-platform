package projection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

var ErrNoProjectionWork = errors.New("no graph projection work is ready")

type Target interface {
	Ready(context.Context) error
	Apply(context.Context, *trafficv1.GraphProjectionEvent) error
}

type WorkerConfig struct {
	WorkerID    string
	Lease       time.Duration
	Interval    time.Duration
	MaxAttempts int
	Logger      *zap.Logger
}

type Worker struct {
	db     *sql.DB
	target Target
	config WorkerConfig
}

type claimedProjection struct {
	eventID         string
	claimToken      string
	payload         []byte
	payloadSHA256   string
	topic           string
	partition       int
	offset          int64
	sourceTimestamp int64
	attempts        int
	metadata        projectionMetadata
	event           *trafficv1.GraphProjectionEvent
}

func NewWorker(db *sql.DB, target Target, config WorkerConfig) (*Worker, error) {
	if db == nil || target == nil {
		return nil, fmt.Errorf("graph projection database and target are required")
	}
	config.WorkerID = strings.TrimSpace(config.WorkerID)
	if config.WorkerID == "" {
		return nil, fmt.Errorf("graph projection worker ID is required")
	}
	if config.Lease <= 0 {
		config.Lease = 30 * time.Second
	}
	if config.Interval <= 0 {
		config.Interval = 500 * time.Millisecond
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 8
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
	return &Worker{db: db, target: target, config: config}, nil
}

func (worker *Worker) VerifySchema(ctx context.Context) error {
	var tableCount int
	if err := worker.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name IN (
		  'graph_projection_inbox_v1','graph_projection_current_v1',
		  'graph_projection_watermarks_v1','graph_projection_dead_letters_v1'
		)`).Scan(&tableCount); err != nil {
		return fmt.Errorf("verify graph projection PostgreSQL schema: %w", err)
	}
	if tableCount != 4 {
		return fmt.Errorf("graph projection PostgreSQL schema is incomplete: got %d of 4 tables", tableCount)
	}
	if err := worker.target.Ready(ctx); err != nil {
		return fmt.Errorf("verify graph projection target: %w", err)
	}
	return nil
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.config.Interval)
	defer ticker.Stop()
	for {
		if err := worker.ProcessOne(ctx); err != nil && !errors.Is(err, ErrNoProjectionWork) && ctx.Err() == nil {
			worker.config.Logger.Error("Graph projection attempt failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOne applies one partition-head event. PostgreSQL is advanced only
// after the Nebula target acknowledges the deterministic mutation.
func (worker *Worker) ProcessOne(ctx context.Context) error {
	claim, err := worker.claimNext(ctx)
	if err != nil {
		return err
	}
	if err := worker.target.Apply(ctx, claim.event); err != nil {
		if failErr := worker.failAttempt(ctx, claim, "NEBULA_APPLY_FAILED", err); failErr != nil {
			return fmt.Errorf("apply graph projection: %v; record failure: %w", err, failErr)
		}
		return fmt.Errorf("apply graph projection %s: %w", claim.eventID, err)
	}
	if err := worker.complete(ctx, claim); err != nil {
		if failErr := worker.failAttempt(ctx, claim, "POST_ACK_COMMIT_FAILED", err); failErr != nil {
			return fmt.Errorf("commit graph projection: %v; record failure: %w", err, failErr)
		}
		return fmt.Errorf("commit graph projection %s after Nebula acknowledgement: %w", claim.eventID, err)
	}
	worker.config.Logger.Info("Graph projection committed",
		zap.String("event_id", claim.eventID), zap.String("tenant_id", claim.metadata.tenantID),
		zap.String("projection_kind", claim.metadata.kind), zap.String("projection_id", claim.metadata.projectionID),
		zap.Uint64("aggregate_version", claim.metadata.aggregateVersion),
		zap.Int("source_partition", claim.partition), zap.Int64("source_offset", claim.offset))
	return nil
}

func (worker *Worker) claimNext(ctx context.Context) (*claimedProjection, error) {
	claimToken := uuid.NewString()
	row := worker.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT candidate.event_id
		  FROM graph_projection_inbox_v1 candidate
		  WHERE candidate.projection_state='PENDING'
		    AND candidate.next_attempt_at<=now()
		    AND (candidate.claim_token IS NULL OR candidate.claimed_at < now()-($1*interval '1 millisecond'))
		    AND NOT EXISTS (
		      SELECT 1 FROM graph_projection_inbox_v1 prior
		      WHERE prior.source_topic=candidate.source_topic
		        AND prior.source_partition=candidate.source_partition
		        AND prior.source_offset<candidate.source_offset
		        AND prior.projection_state<>'APPLIED'
		    )
		  ORDER BY candidate.inbox_sequence
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE graph_projection_inbox_v1 target
		SET claim_token=$2::uuid,claimed_at=now(),claimed_by=$3,attempts=target.attempts+1
		FROM candidate
		WHERE target.event_id=candidate.event_id
		RETURNING target.event_id,target.raw_payload,target.payload_sha256,
		          target.source_topic,target.source_partition,target.source_offset,
		          target.source_timestamp_ms,target.attempts,target.claim_token::text,target.tenant_id,
		          target.projection_kind,target.projection_id,target.projection_sha256,
		          target.source_event_id,target.source_system,target.source_sha256,
		          target.aggregate_version,target.revoked`,
		worker.config.Lease.Milliseconds(), claimToken, worker.config.WorkerID)
	claim := &claimedProjection{claimToken: claimToken}
	if err := row.Scan(
		&claim.eventID, &claim.payload, &claim.payloadSHA256, &claim.topic,
		&claim.partition, &claim.offset, &claim.sourceTimestamp, &claim.attempts,
		&claim.claimToken,
		&claim.metadata.tenantID, &claim.metadata.kind, &claim.metadata.projectionID,
		&claim.metadata.projectionSHA256, &claim.metadata.sourceEventID,
		&claim.metadata.sourceSystem, &claim.metadata.sourceSHA256,
		&claim.metadata.aggregateVersion, &claim.metadata.revoked,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoProjectionWork
		}
		return nil, fmt.Errorf("claim graph projection partition head: %w", err)
	}
	payloadSum := sha256.Sum256(claim.payload)
	if hex.EncodeToString(payloadSum[:]) != claim.payloadSHA256 {
		_ = worker.failAttempt(ctx, claim, "PAYLOAD_HASH_MISMATCH", ErrProjectionHash)
		return nil, fmt.Errorf("%w: durable inbox payload differs", ErrProjectionHash)
	}
	claim.event = &trafficv1.GraphProjectionEvent{}
	if err := proto.Unmarshal(claim.payload, claim.event); err != nil {
		_ = worker.failAttempt(ctx, claim, "PAYLOAD_DECODE_FAILED", err)
		return nil, fmt.Errorf("decode claimed graph projection: %w", err)
	}
	if err := ValidateEvent(claim.event); err != nil {
		_ = worker.failAttempt(ctx, claim, "PAYLOAD_CONTRACT_DRIFT", err)
		return nil, err
	}
	decodedMetadata, err := metadataOf(claim.event)
	if err != nil {
		_ = worker.failAttempt(ctx, claim, "PAYLOAD_CONTRACT_DRIFT", err)
		return nil, err
	}
	if claim.metadata.tenantID != decodedMetadata.tenantID ||
		claim.metadata.kind != decodedMetadata.kind ||
		claim.metadata.projectionID != decodedMetadata.projectionID ||
		claim.metadata.projectionSHA256 != decodedMetadata.projectionSHA256 ||
		claim.metadata.sourceEventID != decodedMetadata.sourceEventID ||
		claim.metadata.sourceSystem != decodedMetadata.sourceSystem ||
		claim.metadata.sourceSHA256 != decodedMetadata.sourceSHA256 ||
		claim.metadata.aggregateVersion != decodedMetadata.aggregateVersion ||
		claim.metadata.revoked != decodedMetadata.revoked {
		err := fmt.Errorf("durable graph projection metadata differs from payload")
		_ = worker.failAttempt(ctx, claim, "PAYLOAD_METADATA_MISMATCH", err)
		return nil, err
	}
	claim.metadata.validFrom = decodedMetadata.validFrom
	claim.metadata.validTo = decodedMetadata.validTo
	return claim, nil
}

func (worker *Worker) complete(ctx context.Context, claim *claimedProjection) error {
	tx, err := worker.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, storedClaim, storedOwner string
	if err := tx.QueryRowContext(ctx, `
		SELECT projection_state,claim_token::text,claimed_by
		FROM graph_projection_inbox_v1 WHERE event_id=$1 FOR UPDATE`, claim.eventID,
	).Scan(&state, &storedClaim, &storedOwner); err != nil {
		return err
	}
	if state != "PENDING" || storedClaim != claim.claimToken || storedOwner != worker.config.WorkerID {
		return fmt.Errorf("graph projection claim ownership changed")
	}
	var currentVersion sql.NullInt64
	var currentHash sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT aggregate_version,projection_sha256
		FROM graph_projection_current_v1
		WHERE tenant_id=$1 AND projection_kind=$2 AND projection_id=$3
		FOR UPDATE`, claim.metadata.tenantID, claim.metadata.kind, claim.metadata.projectionID,
	).Scan(&currentVersion, &currentHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if currentVersion.Valid {
		if currentVersion.Int64 > int64(claim.metadata.aggregateVersion) {
			return fmt.Errorf("current graph projection version %d exceeds claimed %d",
				currentVersion.Int64, claim.metadata.aggregateVersion)
		}
		if currentVersion.Int64 == int64(claim.metadata.aggregateVersion) &&
			currentHash.String != claim.metadata.projectionSHA256 {
			return fmt.Errorf("current graph projection version has another hash")
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO graph_projection_current_v1 (
		  tenant_id,projection_kind,projection_id,aggregate_type,aggregate_id,
		  aggregate_version,event_id,source_event_id,source_sha256,projection_sha256,
		  revoked,valid_from_ms,valid_to_ms,nebula_acknowledged,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,true,now())
		ON CONFLICT (tenant_id,projection_kind,projection_id) DO UPDATE SET
		  aggregate_type=EXCLUDED.aggregate_type,aggregate_id=EXCLUDED.aggregate_id,
		  aggregate_version=EXCLUDED.aggregate_version,event_id=EXCLUDED.event_id,
		  source_event_id=EXCLUDED.source_event_id,source_sha256=EXCLUDED.source_sha256,
		  projection_sha256=EXCLUDED.projection_sha256,revoked=EXCLUDED.revoked,
		  valid_from_ms=EXCLUDED.valid_from_ms,valid_to_ms=EXCLUDED.valid_to_ms,
		  nebula_acknowledged=true,updated_at=now()
		WHERE graph_projection_current_v1.aggregate_version<EXCLUDED.aggregate_version
		   OR (graph_projection_current_v1.aggregate_version=EXCLUDED.aggregate_version
		       AND graph_projection_current_v1.projection_sha256=EXCLUDED.projection_sha256)`,
		claim.metadata.tenantID, claim.metadata.kind, claim.metadata.projectionID,
		claim.event.GetHeader().GetAggregateType(), claim.event.GetHeader().GetAggregateId(),
		claim.metadata.aggregateVersion, claim.eventID, claim.metadata.sourceEventID,
		claim.metadata.sourceSHA256, claim.metadata.projectionSHA256, claim.metadata.revoked,
		claim.metadata.validFrom, claim.metadata.validTo,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("advance current graph projection affected %d rows: %w", affected, err)
	}
	applyResult, err := tx.ExecContext(ctx, `
		UPDATE graph_projection_inbox_v1
		SET projection_state='APPLIED',claim_token=NULL,claimed_at=NULL,claimed_by='',
		    last_error_code='',last_error_detail='',applied_at=now()
		WHERE event_id=$1 AND claim_token=$2::uuid`, claim.eventID, claim.claimToken)
	if err != nil {
		return err
	}
	if affected, err := applyResult.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("mark graph projection applied affected %d rows: %w", affected, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO graph_projection_watermarks_v1 (
		  source_topic,source_partition,source_offset,event_id,tenant_id,
		  projection_sha256,source_timestamp_ms,projected_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,now(),now())
		ON CONFLICT (tenant_id,source_topic,source_partition) DO UPDATE SET
		  source_offset=EXCLUDED.source_offset,event_id=EXCLUDED.event_id,
		  tenant_id=EXCLUDED.tenant_id,projection_sha256=EXCLUDED.projection_sha256,
		  source_timestamp_ms=EXCLUDED.source_timestamp_ms,
		  projected_at=EXCLUDED.projected_at,updated_at=now()
		WHERE graph_projection_watermarks_v1.source_offset<EXCLUDED.source_offset`,
		claim.topic, claim.partition, claim.offset, claim.eventID,
		claim.metadata.tenantID, claim.metadata.projectionSHA256, claim.sourceTimestamp,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (worker *Worker) failAttempt(
	ctx context.Context,
	claim *claimedProjection,
	code string,
	cause error,
) error {
	if claim == nil || claim.eventID == "" || claim.claimToken == "" {
		return fmt.Errorf("cannot record an incomplete graph projection claim failure")
	}
	code = normalizeCode(code)
	detail := truncateError(cause, 2048)
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if claim.attempts >= worker.config.MaxAttempts {
		result, err := tx.ExecContext(ctx, `
			UPDATE graph_projection_inbox_v1
			SET projection_state='DEAD',claim_token=NULL,claimed_at=NULL,claimed_by='',
			    last_error_code=$3,last_error_detail=$4
			WHERE event_id=$1 AND claim_token=$2::uuid`,
			claim.eventID, claim.claimToken, code, detail)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("mark graph projection dead affected %d rows: %w", affected, err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO graph_projection_dead_letters_v1 (
			  dead_letter_id,event_id,tenant_id,source_topic,source_partition,
			  source_offset,payload_sha256,error_code,error_detail
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (event_id) DO NOTHING`,
			uuid.NewString(), claim.eventID, claim.metadata.tenantID, claim.topic,
			claim.partition, claim.offset, claim.payloadSHA256, code, detail)
		if err != nil {
			return err
		}
	} else {
		backoff := retryBackoff(claim.attempts)
		result, err := tx.ExecContext(ctx, `
			UPDATE graph_projection_inbox_v1
			SET claim_token=NULL,claimed_at=NULL,claimed_by='',last_error_code=$3,last_error_detail=$4,
			    next_attempt_at=now()+($5*interval '1 millisecond')
			WHERE event_id=$1 AND claim_token=$2::uuid AND projection_state='PENDING'`,
			claim.eventID, claim.claimToken, code, detail, backoff.Milliseconds())
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("release graph projection claim affected %d rows: %w", affected, err)
		}
	}
	return tx.Commit()
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	backoff := time.Second * time.Duration(1<<uint(attempt-1))
	if backoff > 5*time.Minute {
		return 5 * time.Minute
	}
	return backoff
}

func truncateError(err error, limit int) string {
	if err == nil {
		return "unspecified graph projection failure"
	}
	value := strings.TrimSpace(err.Error())
	if value == "" {
		value = "unspecified graph projection failure"
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
