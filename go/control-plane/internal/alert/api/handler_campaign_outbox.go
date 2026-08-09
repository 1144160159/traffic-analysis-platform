package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	CampaignAggregateEventTopic  = "campaign.events.v2"
	CampaignMembershipEventTopic = "campaign.membership.events.v2"
	campaignAggregateStream      = "aggregate"
	campaignMembershipStream     = "membership"
	campaignOutboxMaxAttempts    = 8
)

type campaignOutboxItem struct {
	Stream            string
	EventID           string
	TenantID          string
	AggregateID       string
	CampaignID        string
	RelationID        string
	AlertID           string
	EventType         string
	PartitionKey      string
	AggregateRevision int64
	RelationRevision  int64
	SchemaVersion     int
	Attempts          int
	Payload           []byte
	TraceID           string
}

type campaignEventEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	TenantID         string `json:"tenant_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	SchemaVersion    int    `json:"schema_version"`
	CampaignID       string `json:"campaign_id"`
	AlertID          string `json:"alert_id"`
	RelationID       string `json:"relation_id"`
	RelationRevision int64  `json:"relation_revision"`
	CampaignRevision int64  `json:"campaign_revision"`
	TraceID          string `json:"trace_id"`
}

// StartCampaignEventOutboxWorker starts one bounded worker for the aggregate
// and membership streams. It refuses partial configuration so a deployment
// cannot silently publish only one side of a membership transaction.
func (h *SystemHandler) StartCampaignEventOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.pgDB == nil {
		return fmt.Errorf("campaign event outbox database is unavailable")
	}
	if h.campaignEventPublish == nil || h.campaignMemberPublish == nil {
		return fmt.Errorf("both campaign event Kafka publishers are required")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := hostnameOrDefault() + ":campaign-outbox:" + uuid.NewString()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainCampaignEventOutboxes(ctx, workerID, 50); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain campaign event outboxes", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *SystemHandler) drainCampaignEventOutboxes(ctx context.Context, workerID string, limit int) (int, error) {
	if h.pgDB == nil || h.campaignEventPublish == nil || h.campaignMemberPublish == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	aggregates, err := h.claimCampaignAggregateOutbox(ctx, workerID, limit)
	if err != nil {
		return 0, err
	}
	processed := h.publishCampaignItems(ctx, workerID, aggregates)
	remaining := limit - len(aggregates)
	if remaining <= 0 {
		return processed, nil
	}
	memberships, err := h.claimCampaignMembershipOutbox(ctx, workerID, remaining)
	if err != nil {
		return processed, err
	}
	return processed + h.publishCampaignItems(ctx, workerID, memberships), nil
}

func (h *SystemHandler) claimCampaignAggregateOutbox(ctx context.Context, workerID string, limit int) ([]campaignOutboxItem, error) {
	rows, err := h.pgDB.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id FROM campaign_aggregate_outbox
			WHERE status IN ('pending','processing') AND published=false
			  AND next_attempt_at<=now()
			  AND (status='pending' OR locked_until IS NULL OR locked_until<now())
			ORDER BY next_attempt_at,created_at,event_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE campaign_aggregate_outbox o
			SET status='processing',attempts=attempts+1,last_attempt_at=now(),
			    locked_until=now()+interval '60 seconds',locked_by=$2
			FROM candidates c WHERE o.event_id=c.event_id
			RETURNING o.event_id::text,o.tenant_id,o.aggregate_id,o.event_type,
			          o.partition_key,o.aggregate_revision,o.schema_version,o.attempts,o.payload::text
		)
		SELECT event_id,tenant_id,aggregate_id,event_type,partition_key,
		       aggregate_revision,schema_version,attempts,payload FROM claimed ORDER BY event_id`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]campaignOutboxItem, 0, limit)
	for rows.Next() {
		var item campaignOutboxItem
		var payload string
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.AggregateID, &item.EventType,
			&item.PartitionKey, &item.AggregateRevision, &item.SchemaVersion, &item.Attempts, &payload); err != nil {
			return items, err
		}
		item.Stream = campaignAggregateStream
		item.CampaignID = item.AggregateID
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) claimCampaignMembershipOutbox(ctx context.Context, workerID string, limit int) ([]campaignOutboxItem, error) {
	rows, err := h.pgDB.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id FROM campaign_alert_link_outbox
			WHERE status IN ('pending','processing') AND published=false
			  AND next_attempt_at<=now()
			  AND (status='pending' OR locked_until IS NULL OR locked_until<now())
			ORDER BY next_attempt_at,created_at,event_id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE campaign_alert_link_outbox o
			SET status='processing',attempts=attempts+1,last_attempt_at=now(),
			    locked_until=now()+interval '60 seconds',locked_by=$2
			FROM candidates c WHERE o.event_id=c.event_id
			RETURNING o.event_id,o.tenant_id,o.aggregate_id,o.event_type,
			          o.partition_key,o.aggregate_version,o.schema_version,o.attempts,o.payload
		)
		SELECT c.event_id::text,c.tenant_id,c.aggregate_id::text,h.campaign_id,h.alert_id,
		       c.event_type,c.partition_key,c.aggregate_version,h.campaign_revision,
		       c.schema_version,c.attempts,c.payload::text
		FROM claimed c
		JOIN campaign_alert_link_history h ON h.event_id=c.event_id
		ORDER BY c.event_id`, limit, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]campaignOutboxItem, 0, limit)
	for rows.Next() {
		var item campaignOutboxItem
		var payload string
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.AggregateID, &item.CampaignID,
			&item.AlertID, &item.EventType, &item.PartitionKey, &item.RelationRevision,
			&item.AggregateRevision, &item.SchemaVersion, &item.Attempts, &payload); err != nil {
			return items, err
		}
		item.Stream = campaignMembershipStream
		item.RelationID = item.AggregateID
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SystemHandler) publishCampaignItems(ctx context.Context, workerID string, items []campaignOutboxItem) int {
	processed := 0
	for _, item := range items {
		if err := h.publishCampaignOutboxItem(ctx, workerID, &item); err != nil {
			if h.logger != nil {
				h.logger.Warn("Campaign event delivery failed", zap.String("stream", item.Stream), zap.String("event_id", item.EventID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed
}

func (h *SystemHandler) publishCampaignOutboxItem(ctx context.Context, workerID string, item *campaignOutboxItem) error {
	if err := validateCampaignOutboxItem(item); err != nil {
		h.failCampaignOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	publish := h.campaignEventPublish
	topic := CampaignAggregateEventTopic
	if item.Stream == campaignMembershipStream {
		publish = h.campaignMemberPublish
		topic = CampaignMembershipEventTopic
	}
	headers := []commonkafka.MessageHeader{
		{Key: "event_id", Value: item.EventID}, {Key: "event_type", Value: item.EventType},
		{Key: "tenant_id", Value: item.TenantID}, {Key: "stream", Value: item.Stream},
		{Key: "aggregate_id", Value: item.AggregateID}, {Key: "campaign_id", Value: item.CampaignID},
		{Key: "aggregate_version", Value: fmt.Sprint(item.AggregateRevision)},
		{Key: "relation_revision", Value: fmt.Sprint(item.RelationRevision)},
		{Key: "schema_version", Value: fmt.Sprint(item.SchemaVersion)},
		{Key: "trace_id", Value: item.TraceID}, {Key: "target_topic", Value: topic},
	}
	if err := publish(ctx, item.PartitionKey, item.Payload, headers...); err != nil {
		h.failCampaignOutboxItem(ctx, workerID, *item, err.Error())
		return err
	}
	table := campaignOutboxTable(item.Stream)
	result, err := h.pgDB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s
		SET status='published',published=true,last_error='',published_at=now(),
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND status='processing' AND published=false AND locked_by=$2`, table), item.EventID, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("campaign %s outbox lease lost after Kafka acknowledgement", item.Stream)
	}
	return nil
}

func validateCampaignOutboxItem(item *campaignOutboxItem) error {
	var envelope campaignEventEnvelope
	if err := json.Unmarshal(item.Payload, &envelope); err != nil {
		return fmt.Errorf("decode campaign outbox envelope: %w", err)
	}
	if _, err := uuid.Parse(item.EventID); err != nil || envelope.EventID != item.EventID {
		return fmt.Errorf("campaign event_id is invalid or mismatched")
	}
	if item.SchemaVersion != 2 || envelope.SchemaVersion != item.SchemaVersion ||
		envelope.EventType != item.EventType || envelope.TenantID != item.TenantID ||
		envelope.PartitionKey != item.PartitionKey || strings.TrimSpace(envelope.TraceID) == "" {
		return fmt.Errorf("campaign event envelope identity mismatch")
	}
	item.TraceID = envelope.TraceID
	if !validCampaignLifecycleEvent(item.EventType) {
		return fmt.Errorf("unsupported campaign lifecycle event_type")
	}
	if item.Stream == campaignAggregateStream {
		if envelope.AggregateType != "campaign" || envelope.AggregateID != item.CampaignID ||
			envelope.AggregateVersion != item.AggregateRevision || item.AggregateRevision <= 0 {
			return fmt.Errorf("campaign aggregate identity mismatch")
		}
		return nil
	}
	if item.Stream != campaignMembershipStream ||
		(item.EventType != "traffic.campaign.v2.AlertLinked" && item.EventType != "traffic.campaign.v2.AlertUnlinked") {
		return fmt.Errorf("unsupported campaign membership stream event")
	}
	if envelope.RelationID != item.RelationID || envelope.CampaignID != item.CampaignID ||
		envelope.AlertID != item.AlertID || envelope.RelationRevision != item.RelationRevision ||
		envelope.CampaignRevision != item.AggregateRevision || item.RelationRevision <= 0 || item.AggregateRevision <= 0 {
		return fmt.Errorf("campaign membership identity mismatch")
	}
	return nil
}

func validCampaignLifecycleEvent(value string) bool {
	switch value {
	case "traffic.campaign.v2.OwnerAssigned", "traffic.campaign.v2.StatusChanged",
		"traffic.campaign.v2.ReportRequested", "traffic.campaign.v2.ReportCompleted", "traffic.campaign.v2.ReportFailed",
		"traffic.campaign.v2.SoarRequested", "traffic.campaign.v2.SoarCompleted",
		"traffic.campaign.v2.SoarPartial", "traffic.campaign.v2.SoarFailed",
		"traffic.campaign.v2.SoarCompensated", "traffic.campaign.v2.SoarCompensationFailed",
		"traffic.campaign.v2.AlertLinked", "traffic.campaign.v2.AlertUnlinked",
		"traffic.campaign.v2.Merged", "traffic.campaign.v2.MergeReceived",
		"traffic.campaign.v2.MembershipBackfilled":
		return true
	default:
		return false
	}
}

func (h *SystemHandler) failCampaignOutboxItem(ctx context.Context, workerID string, item campaignOutboxItem, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	status := "pending"
	if item.Attempts >= campaignOutboxMaxAttempts {
		status = "dead"
	}
	table := campaignOutboxTable(item.Stream)
	_, _ = h.pgDB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s
		SET status=$2,last_error=$3,
		    next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval,
		    dead_at=CASE WHEN $2='dead' THEN now() ELSE NULL END,
		    locked_until=NULL,locked_by=''
		WHERE event_id=$1::uuid AND status='processing' AND published=false AND locked_by=$4`, table),
		item.EventID, status, message, workerID)
}

func campaignOutboxTable(stream string) string {
	if stream == campaignMembershipStream {
		return "campaign_alert_link_outbox"
	}
	return "campaign_aggregate_outbox"
}
