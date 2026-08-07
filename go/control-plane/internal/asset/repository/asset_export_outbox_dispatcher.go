package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"go.uber.org/zap"
)

type AssetExportOutboxDispatcher struct {
	db        *sql.DB
	publisher AssetEventPublisher
	cfg       OutboxDispatcherConfig
}

type assetExportOutboxEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	TenantID         string `json:"tenant_id"`
	JobID            string `json:"job_id"`
	TraceID          string `json:"trace_id"`
}

func NewAssetExportOutboxDispatcher(
	db *sql.DB,
	publisher AssetEventPublisher,
	cfg OutboxDispatcherConfig,
) (*AssetExportOutboxDispatcher, error) {
	if db == nil || publisher == nil {
		return nil, fmt.Errorf("asset export outbox database and publisher are required")
	}
	if cfg.WorkerID == "" {
		return nil, fmt.Errorf("asset export outbox worker_id is required")
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return &AssetExportOutboxDispatcher{db: db, publisher: publisher, cfg: cfg}, nil
}

func (d *AssetExportOutboxDispatcher) VerifySchema(ctx context.Context) error {
	var columns int
	if err := d.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='asset_export_outbox'
		  AND column_name IN (
		    'outbox_id','event_id','job_id','tenant_id','event_type',
		    'aggregate_version','schema_version','partition_key','payload','status',
		    'attempts','next_attempt_at','locked_by','locked_until','last_error',
		    'created_at','published_at'
		  )`).Scan(&columns); err != nil {
		return fmt.Errorf("verify asset export outbox schema: %w", err)
	}
	if columns != 17 {
		return fmt.Errorf("asset export outbox schema incomplete: columns=%d want=17", columns)
	}
	return nil
}

func (d *AssetExportOutboxDispatcher) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			for processed := 0; processed < d.cfg.BatchSize; processed++ {
				found, err := d.DispatchNext(ctx)
				if err != nil {
					d.cfg.Logger.Warn("Asset export outbox dispatch failed", zap.Error(err))
					break
				}
				if !found {
					break
				}
			}
			timer.Reset(d.cfg.Interval)
		}
	}
}

func (d *AssetExportOutboxDispatcher) DispatchNext(ctx context.Context) (bool, error) {
	var (
		outboxID, aggregateVersion int64
		eventID, jobID, tenantID   string
		eventType, partitionKey    string
		payload                    []byte
		schemaVersion, attempts    int
	)
	leaseSeconds := int64(d.cfg.Lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	err := d.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT outbox_id AS candidate_outbox_id
		  FROM asset_export_outbox
		  WHERE (status='pending' AND next_attempt_at<=now())
		     OR (status='processing' AND locked_until<now())
		  ORDER BY next_attempt_at,outbox_id
		  FOR UPDATE SKIP LOCKED
		  LIMIT 1
		)
		UPDATE asset_export_outbox outbox
		SET status='processing',attempts=outbox.attempts+1,locked_by=$1,
		    locked_until=now()+($2*interval '1 second')
		FROM candidate
		WHERE outbox.outbox_id=candidate.candidate_outbox_id
		RETURNING outbox.outbox_id,outbox.event_id::text,outbox.job_id::text,
		          outbox.tenant_id,outbox.event_type,outbox.aggregate_version,
		          outbox.schema_version,outbox.partition_key,outbox.payload::text,
		          outbox.attempts`,
		d.cfg.WorkerID, leaseSeconds,
	).Scan(
		&outboxID, &eventID, &jobID, &tenantID, &eventType,
		&aggregateVersion, &schemaVersion, &partitionKey, &payload, &attempts,
	)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lease asset export outbox: %w", err)
	}

	var envelope assetExportOutboxEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return true, d.recordFailure(ctx, outboxID, attempts, fmt.Errorf("decode asset export event: %w", err))
	}
	allowedEvent := eventType == "traffic.asset.export.v1.Requested" || eventType == "traffic.asset.export.v1.Completed"
	if !allowedEvent || envelope.EventID != eventID || envelope.EventType != eventType ||
		envelope.JobID != jobID || envelope.TenantID != tenantID ||
		envelope.AggregateVersion != aggregateVersion || envelope.SchemaVersion != schemaVersion ||
		envelope.PartitionKey != partitionKey {
		return true, d.recordFailure(ctx, outboxID, attempts, fmt.Errorf("asset export outbox envelope does not match authoritative columns"))
	}
	if err := d.publisher.Send(ctx, partitionKey, payload,
		kafkaCommon.MessageHeader{Key: "event_id", Value: eventID},
		kafkaCommon.MessageHeader{Key: "event_type", Value: eventType},
		kafkaCommon.MessageHeader{Key: "schema_version", Value: strconv.Itoa(schemaVersion)},
		kafkaCommon.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(aggregateVersion, 10)},
		kafkaCommon.MessageHeader{Key: "tenant_id", Value: tenantID},
		kafkaCommon.MessageHeader{Key: "job_id", Value: jobID},
		kafkaCommon.MessageHeader{Key: "trace_id", Value: envelope.TraceID},
	); err != nil {
		return true, d.recordFailure(ctx, outboxID, attempts, err)
	}
	result, err := d.db.ExecContext(ctx, `
		UPDATE asset_export_outbox
		SET status='published',published_at=now(),locked_by='',locked_until=NULL,last_error=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`,
		outboxID, d.cfg.WorkerID,
	)
	if err != nil {
		return true, fmt.Errorf("mark asset export outbox published: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return true, fmt.Errorf("asset export outbox publish state collision")
	}
	return true, nil
}

func (d *AssetExportOutboxDispatcher) recordFailure(ctx context.Context, outboxID int64, attempts int, dispatchErr error) error {
	status := "pending"
	if attempts >= d.cfg.MaxAttempts {
		status = "dead"
	}
	exponent := attempts
	if exponent > 8 {
		exponent = 8
	}
	message := dispatchErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	result, err := d.db.ExecContext(ctx, `
		UPDATE asset_export_outbox
		SET status=$3,next_attempt_at=now()+($4*interval '1 second'),
		    locked_by='',locked_until=NULL,last_error=$5
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`,
		outboxID, d.cfg.WorkerID, status, 1<<exponent, message,
	)
	if err != nil {
		return fmt.Errorf("record asset export outbox failure: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("asset export outbox failure state collision after: %w", dispatchErr)
	}
	return dispatchErr
}
