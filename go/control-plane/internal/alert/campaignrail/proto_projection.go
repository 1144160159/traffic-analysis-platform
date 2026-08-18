package campaignrail

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	ProtoTopic           = "campaigns.v1"
	ProtoRailID          = "cep_protobuf"
	AggregateJSONTopic   = "campaign.events.v2"
	AggregateJSONRailID  = "aggregate_json_v2"
	MembershipJSONTopic  = "campaign.membership.events.v2"
	MembershipJSONRailID = "membership_json_v2"
	ProtoMessageType     = "traffic.v1.Campaign"
	ProtoSchema          = "1"
	ProtoSourceService   = "flink-cep-job"
)

var ErrProtoIdentityCollision = errors.New("campaign protobuf identity collision")
var ErrConsumerNotReady = errors.New("campaign consumer is not ready")

type ProtoProjectionInput struct {
	Campaign       *trafficv1.Campaign
	Payload        []byte
	PayloadSHA256  string
	KafkaTopic     string
	KafkaPartition int
	KafkaOffset    int64
	ReceivedAt     time.Time
}

type ConsumerReceipt struct {
	RailID          string
	CandidateSHA256 string
	SourceTopic     string
	ConsumerGroup   string
	State           string
	EventID         string
	SourcePartition int
	SourceOffset    int64
	ObservedAt      time.Time
}

type ProtoProjectionStore struct {
	db  *sql.DB
	now func() time.Time
}

func NewProtoProjectionStore(db *sql.DB) (*ProtoProjectionStore, error) {
	if db == nil {
		return nil, fmt.Errorf("campaign protobuf projection database is required")
	}
	return &ProtoProjectionStore{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (store *ProtoProjectionStore) VerifySchema(ctx context.Context) error {
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name IN (
		  'campaign_proto_projection_inbox_v1','campaign_proto_projection_current_v1',
		  'campaign_consumer_readiness_v1','campaign_rail_correlation_v1',
		  'campaign_rail_reconcile_runs_v1')`).Scan(&count); err != nil {
		return err
	}
	if count != 5 {
		return fmt.Errorf("campaign dual-rail schema is incomplete: got %d of 5 tables", count)
	}
	return nil
}

func (store *ProtoProjectionStore) ApplyProtoProjection(ctx context.Context, input ProtoProjectionInput) error {
	if err := ValidateProtoProjectionInput(input); err != nil {
		return err
	}
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = store.now()
	}
	campaign := input.Campaign
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO campaign_proto_projection_inbox_v1 (
		event_id,tenant_id,campaign_id,campaign_type,event_time_start_ms,event_time_end_ms,
		payload_sha256,payload_protobuf,source_topic,source_partition,source_offset,received_at,state
	) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'received') ON CONFLICT DO NOTHING`,
		campaign.GetEventId(), campaign.GetTenantId(), campaign.GetCampaignId(), campaign.GetCampaignType(),
		campaign.GetTsStart(), campaign.GetTsEnd(), input.PayloadSHA256, input.Payload,
		input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.ReceivedAt)
	if err != nil {
		return fmt.Errorf("insert campaign protobuf inbox: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		var exact bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM campaign_proto_projection_inbox_v1
			WHERE event_id=$1::uuid AND tenant_id=$2 AND campaign_id=$3 AND campaign_type=$4
			  AND event_time_start_ms=$5 AND event_time_end_ms=$6 AND payload_sha256=$7
			  AND payload_protobuf=$8 AND source_topic=$9 AND source_partition=$10 AND source_offset=$11)`,
			campaign.GetEventId(), campaign.GetTenantId(), campaign.GetCampaignId(), campaign.GetCampaignType(),
			campaign.GetTsStart(), campaign.GetTsEnd(), input.PayloadSHA256, input.Payload,
			input.KafkaTopic, input.KafkaPartition, input.KafkaOffset).Scan(&exact); err != nil {
			return err
		}
		if !exact {
			return fmt.Errorf("%w: event or Kafka position changed meaning", ErrProtoIdentityCollision)
		}
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO campaign_proto_projection_current_v1 (
		tenant_id,campaign_id,event_id,payload_sha256,event_time_start_ms,event_time_end_ms,
		campaign_type,score,source_topic,source_partition,source_offset
	) VALUES ($1,$2,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
		campaign.GetTenantId(), campaign.GetCampaignId(), campaign.GetEventId(), input.PayloadSHA256,
		campaign.GetTsStart(), campaign.GetTsEnd(), campaign.GetCampaignType(), campaign.GetScore(),
		input.KafkaTopic, input.KafkaPartition, input.KafkaOffset)
	if err != nil {
		return fmt.Errorf("insert campaign protobuf current projection: %w", err)
	}
	currentInserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if currentInserted == 0 {
		var exact bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM campaign_proto_projection_current_v1
			WHERE tenant_id=$1 AND campaign_id=$2 AND event_id=$3::uuid AND payload_sha256=$4
			  AND source_topic=$5 AND source_partition=$6 AND source_offset=$7)`,
			campaign.GetTenantId(), campaign.GetCampaignId(), campaign.GetEventId(), input.PayloadSHA256,
			input.KafkaTopic, input.KafkaPartition, input.KafkaOffset).Scan(&exact); err != nil {
			return err
		}
		if !exact {
			return fmt.Errorf("%w: campaign ID already has different bytes", ErrProtoIdentityCollision)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_proto_projection_inbox_v1
		SET state='applied',applied_at=COALESCE(applied_at,now())
		WHERE event_id=$1::uuid AND state IN ('received','applied')`, campaign.GetEventId()); err != nil {
		return err
	}
	return tx.Commit()
}

func ValidateProtoProjectionInput(input ProtoProjectionInput) error {
	if input.Campaign == nil || input.KafkaTopic != ProtoTopic || input.KafkaPartition < 0 || input.KafkaOffset < 0 ||
		len(input.Payload) == 0 || !validSHA256(input.PayloadSHA256) {
		return fmt.Errorf("invalid campaign protobuf projection envelope")
	}
	digest := sha256.Sum256(input.Payload)
	if input.PayloadSHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("campaign protobuf payload SHA mismatch")
	}
	campaign := input.Campaign
	tenantID := strings.TrimSpace(campaign.GetTenantId())
	if tenantID == "" || strings.EqualFold(tenantID, "unknown") || strings.TrimSpace(campaign.GetCampaignId()) == "" ||
		strings.TrimSpace(campaign.GetEventId()) == "" || strings.TrimSpace(campaign.GetCampaignType()) == "" ||
		campaign.GetTsStart() <= 0 || campaign.GetTsEnd() < campaign.GetTsStart() || campaign.GetScore() < 0 || campaign.GetScore() > 1 ||
		campaign.GetHeader() == nil || campaign.GetHeader().GetTenantId() != tenantID ||
		campaign.GetHeader().GetEventId() != campaign.GetEventId() || len(campaign.GetAlerts()) == 0 || len(campaign.GetEntities()) == 0 {
		return fmt.Errorf("incomplete or inconsistent campaign protobuf payload")
	}
	return nil
}

func (store *ProtoProjectionStore) RecordConsumerReceipt(ctx context.Context, receipt ConsumerReceipt) error {
	if !validConsumerReceipt(receipt) {
		return fmt.Errorf("invalid campaign consumer readiness receipt")
	}
	var eventID interface{}
	var partition, offset interface{}
	if receipt.State == "ready" {
		eventID, partition, offset = receipt.EventID, receipt.SourcePartition, receipt.SourceOffset
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO campaign_consumer_readiness_v1 (
		rail_id,candidate_sha256,source_topic,consumer_group,state,last_event_id,
		last_source_partition,last_source_offset,observed_at
	) VALUES ($1,$2,$3,$4,$5,$6::uuid,$7,$8,$9)
	ON CONFLICT (rail_id,candidate_sha256,source_topic,consumer_group) DO UPDATE SET
	  state=EXCLUDED.state,last_event_id=EXCLUDED.last_event_id,
	  last_source_partition=EXCLUDED.last_source_partition,last_source_offset=EXCLUDED.last_source_offset,
	  observed_at=EXCLUDED.observed_at,updated_at=now()
	WHERE campaign_consumer_readiness_v1.observed_at<=EXCLUDED.observed_at`,
		receipt.RailID, receipt.CandidateSHA256, receipt.SourceTopic, receipt.ConsumerGroup,
		receipt.State, eventID, partition, offset, receipt.ObservedAt)
	return err
}

func (store *ProtoProjectionStore) AssertConsumerReady(
	ctx context.Context,
	railID, candidateSHA256, topic, group string,
) error {
	var state string
	var eventID sql.NullString
	var partition sql.NullInt64
	var offset sql.NullInt64
	if err := store.db.QueryRowContext(ctx, `SELECT state,last_event_id::text,last_source_partition,last_source_offset
		FROM campaign_consumer_readiness_v1
		WHERE rail_id=$1 AND candidate_sha256=$2 AND source_topic=$3 AND consumer_group=$4`,
		railID, candidateSHA256, topic, group).Scan(&state, &eventID, &partition, &offset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConsumerNotReady
		}
		return err
	}
	if state != "ready" || !eventID.Valid || !partition.Valid || !offset.Valid {
		return ErrConsumerNotReady
	}
	return nil
}

func validConsumerReceipt(receipt ConsumerReceipt) bool {
	expectedTopic := map[string]string{
		ProtoRailID: ProtoTopic, AggregateJSONRailID: AggregateJSONTopic,
		MembershipJSONRailID: MembershipJSONTopic,
	}[receipt.RailID]
	if expectedTopic == "" || !validSHA256(receipt.CandidateSHA256) ||
		receipt.SourceTopic != expectedTopic || strings.TrimSpace(receipt.ConsumerGroup) == "" || receipt.ObservedAt.IsZero() {
		return false
	}
	if receipt.State == "stopped" {
		return true
	}
	return receipt.State == "ready" && strings.TrimSpace(receipt.EventID) != "" &&
		receipt.SourcePartition >= 0 && receipt.SourceOffset >= 0
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
