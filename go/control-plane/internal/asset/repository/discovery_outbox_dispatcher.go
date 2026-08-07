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

type DiscoveryEventPublisher interface {
	Send(context.Context, string, []byte, ...kafkaCommon.MessageHeader) error
}

type DiscoveryOutboxDispatcher struct {
	db        *sql.DB
	publisher DiscoveryEventPublisher
	cfg       OutboxDispatcherConfig
}

type discoveryOutboxEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	TenantID         string `json:"tenant_id"`
	ResourceType     string `json:"resource_type"`
	ResourceID       string `json:"resource_id"`
	ActionID         string `json:"action_id"`
	Revision         int64  `json:"revision"`
	RunID            string `json:"run_id"`
	TraceID          string `json:"trace_id"`
	Status           string `json:"status"`
}

func NewDiscoveryOutboxDispatcher(
	db *sql.DB,
	publisher DiscoveryEventPublisher,
	cfg OutboxDispatcherConfig,
) (*DiscoveryOutboxDispatcher, error) {
	if db == nil || publisher == nil {
		return nil, fmt.Errorf("discovery outbox database and publisher are required")
	}
	if cfg.WorkerID == "" {
		return nil, fmt.Errorf("discovery outbox worker_id is required")
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
	return &DiscoveryOutboxDispatcher{db: db, publisher: publisher, cfg: cfg}, nil
}

func (d *DiscoveryOutboxDispatcher) VerifySchema(ctx context.Context) error {
	var columns int
	if err := d.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name='asset_discovery_outbox'
		  AND column_name IN (
		    'outbox_id','event_id','run_id','resource_type','resource_id','action_id',
		    'tenant_id','aggregate_version',
		    'schema_version','partition_key','event_type','payload','status',
		    'attempt_count','available_at','locked_by','locked_until','last_error',
		    'created_at','published_at'
		  )`).Scan(&columns); err != nil {
		return fmt.Errorf("verify discovery outbox schema: %w", err)
	}
	if columns != 20 {
		return fmt.Errorf("discovery outbox schema incomplete: columns=%d want=20", columns)
	}
	return nil
}

func (d *DiscoveryOutboxDispatcher) Run(ctx context.Context) {
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
					d.cfg.Logger.Warn("Discovery outbox dispatch failed", zap.Error(err))
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

func (d *DiscoveryOutboxDispatcher) DispatchNext(ctx context.Context) (bool, error) {
	var (
		outboxID, aggregateVersion                         int64
		eventID, runID, resourceType, resourceID, actionID string
		tenantID, partitionKey, eventType                  string
		schemaVersion, attemptCount                        int
		payload                                            []byte
	)
	leaseSeconds := int64(d.cfg.Lease.Seconds())
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	err := d.db.QueryRowContext(ctx, `
		WITH candidate AS (
		  SELECT outbox_id
		  FROM asset_discovery_outbox
		  WHERE (status='pending' AND available_at<=now())
		     OR (status='processing' AND locked_until<now())
		  ORDER BY available_at,outbox_id
		  FOR UPDATE SKIP LOCKED
		  LIMIT 1
		)
		UPDATE asset_discovery_outbox outbox
		SET status='processing',
		    attempt_count=outbox.attempt_count+1,
		    locked_by=$1,
		    locked_until=now()+($2*interval '1 second')
		FROM candidate
		WHERE outbox.outbox_id=candidate.outbox_id
		RETURNING outbox.outbox_id,outbox.event_id::text,COALESCE(outbox.run_id,''),
		          outbox.resource_type,outbox.resource_id,outbox.action_id,
		          outbox.tenant_id,outbox.aggregate_version,outbox.schema_version,
		          outbox.partition_key,outbox.event_type,outbox.payload::text,
		          outbox.attempt_count`,
		d.cfg.WorkerID, leaseSeconds,
	).Scan(&outboxID, &eventID, &runID, &resourceType, &resourceID, &actionID, &tenantID, &aggregateVersion,
		&schemaVersion, &partitionKey, &eventType, &payload, &attemptCount)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lease discovery outbox: %w", err)
	}

	var envelope discoveryOutboxEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return true, d.recordFailure(ctx, outboxID, attemptCount, fmt.Errorf("decode discovery event: %w", err))
	}
	if envelope.EventID != eventID || envelope.ResourceType != resourceType ||
		envelope.ResourceID != resourceID || envelope.ActionID != actionID ||
		envelope.TenantID != tenantID || envelope.AggregateVersion != aggregateVersion ||
		envelope.Revision != aggregateVersion ||
		envelope.SchemaVersion != schemaVersion || envelope.PartitionKey != partitionKey ||
		envelope.EventType != eventType || envelope.TraceID == "" ||
		!validDiscoveryResourceEnvelope(resourceType, resourceID, actionID, runID, eventType, envelope) {
		return true, d.recordFailure(ctx, outboxID, attemptCount,
			fmt.Errorf("discovery outbox envelope does not match authoritative columns"))
	}

	if err := d.publisher.Send(ctx, partitionKey, payload,
		kafkaCommon.MessageHeader{Key: "event_id", Value: eventID},
		kafkaCommon.MessageHeader{Key: "event_type", Value: eventType},
		kafkaCommon.MessageHeader{Key: "schema_version", Value: strconv.Itoa(schemaVersion)},
		kafkaCommon.MessageHeader{Key: "aggregate_version", Value: strconv.FormatInt(aggregateVersion, 10)},
		kafkaCommon.MessageHeader{Key: "tenant_id", Value: tenantID},
		kafkaCommon.MessageHeader{Key: "resource_type", Value: resourceType},
		kafkaCommon.MessageHeader{Key: "resource_id", Value: resourceID},
		kafkaCommon.MessageHeader{Key: "action_id", Value: actionID},
		kafkaCommon.MessageHeader{Key: "run_id", Value: runID},
		kafkaCommon.MessageHeader{Key: "trace_id", Value: envelope.TraceID},
	); err != nil {
		return true, d.recordFailure(ctx, outboxID, attemptCount, err)
	}

	result, err := d.db.ExecContext(ctx, `
		UPDATE asset_discovery_outbox
		SET status='published',published_at=now(),locked_by='',locked_until=NULL,last_error=''
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`,
		outboxID, d.cfg.WorkerID)
	if err != nil {
		return true, fmt.Errorf("mark discovery outbox published: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return true, fmt.Errorf("discovery outbox publish state collision")
	}
	return true, nil
}

func validDiscoveryResourceEnvelope(resourceType, resourceID, actionID, runID, eventType string, envelope discoveryOutboxEnvelope) bool {
	switch resourceType {
	case "run":
		return resourceID == runID && envelope.RunID == runID && envelope.Status != "" &&
			actionID == "asset-active-discovery-run" &&
			(eventType == "traffic.asset.discovery.v1.JobAccepted" ||
				eventType == "traffic.asset.discovery.v1.JobStateChanged")
	case "credential":
		return runID == "" && envelope.RunID == "" && actionID == "asset-discovery-credential-upsert" &&
			eventType == "traffic.asset.discovery.v1.CredentialUpserted"
	case "topology_link":
		return runID == "" && envelope.RunID == "" && actionID == "asset-discovery-topology-link-upsert" &&
			eventType == "traffic.asset.discovery.v1.TopologyLinkUpserted"
	default:
		return false
	}
}

func (d *DiscoveryOutboxDispatcher) recordFailure(
	ctx context.Context,
	outboxID int64,
	attemptCount int,
	dispatchErr error,
) error {
	status := "pending"
	if attemptCount >= d.cfg.MaxAttempts {
		status = "dead"
	}
	exponent := attemptCount
	if exponent > 8 {
		exponent = 8
	}
	retrySeconds := 1 << exponent
	result, err := d.db.ExecContext(ctx, `
		UPDATE asset_discovery_outbox
		SET status=$3,
		    available_at=now()+($4*interval '1 second'),
		    locked_by='',locked_until=NULL,last_error=$5
		WHERE outbox_id=$1 AND status='processing' AND locked_by=$2`,
		outboxID, d.cfg.WorkerID, status, retrySeconds, dispatchErr.Error())
	if err != nil {
		return fmt.Errorf("record discovery outbox failure: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return fmt.Errorf("discovery outbox failure state collision after: %w", dispatchErr)
	}
	return dispatchErr
}
