package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type campaignEventKafkaIntegrationResult struct {
	db         *sql.DB
	tenantID   string
	campaignID string
	alertID    string
	eventID    string
	relationID string
	traceID    string
}

// runCampaignEventRealKafkaProjectionBoundary proves the committed business
// transaction is published to both canonical streams, acknowledged only after
// a real broker accepts it, and durably represented by a composite
// (stream,event_id) inbox plus per-topic source watermarks. The returned state
// lets the four-store integration continue from these exact Kafka deliveries
// instead of manufacturing a second event at the target boundary.
func runCampaignEventRealKafkaProjectionBoundary(t *testing.T) campaignEventKafkaIntegrationResult {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("CAMPAIGN_EVENT_EPHEMERAL_PG_DSN"))
	broker := strings.TrimSpace(os.Getenv("CAMPAIGN_EVENT_EPHEMERAL_KAFKA_BROKER"))
	if dsn == "" || broker == "" {
		t.Skip("explicit ephemeral PostgreSQL and Kafka settings are required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_campaign_event_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	tenantID := "campaign-event-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	campaignID := "campaign-" + uuid.NewString()
	alertID := "alert-" + uuid.NewString()
	eventID := uuid.NewString()
	relationID := uuid.NewString()
	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM campaign_event_projection_deliveries WHERE kafka_topic IN ($1,$2)`, CampaignAggregateEventTopic, CampaignMembershipEventTopic)
		_, _ = db.Exec(`DELETE FROM campaign_event_projection_watermarks WHERE kafka_topic IN ($1,$2)`, CampaignAggregateEventTopic, CampaignMembershipEventTopic)
		_, _ = db.Exec(`DELETE FROM campaign_event_projection_inbox WHERE tenant_id=$1`, tenantID)
		for _, table := range []string{"campaign_alert_link_outbox", "campaign_aggregate_outbox", "campaign_alert_link_history", "campaign_aggregate_history", "campaign_alert_links", "campaign_workbench_state", "tenants"} {
			_, _ = db.Exec(fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table), tenantID)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	if _, err := db.Exec(`INSERT INTO tenants(tenant_id,name) VALUES($1,'Campaign Event Integration')`, tenantID); err != nil {
		t.Fatal(err)
	}
	const traceID = "trace-real-campaign"
	payload := map[string]interface{}{"event_id": eventID, "event_type": "traffic.campaign.v2.AlertLinked",
		"tenant_id": tenantID, "schema_version": 2, "aggregate_type": "campaign", "aggregate_id": campaignID,
		"aggregate_version": 2, "partition_key": tenantID + ":" + campaignID, "campaign_id": campaignID,
		"alert_id": alertID, "relation_id": relationID, "relation_revision": 1, "campaign_revision": 2,
		"status": "active", "assignee": "", "member_count": 1, "reason": "real Kafka integration", "trace_id": traceID}
	payloadJSON, _ := json.Marshal(payload)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	statements := []struct {
		query string
		args  []interface{}
	}{
		{`INSERT INTO campaign_workbench_state(tenant_id,campaign_id,state_version,member_count,last_event_id) VALUES($1,$2,2,1,$3::uuid)`, []interface{}{tenantID, campaignID, eventID}},
		{`INSERT INTO campaign_alert_links(relation_id,tenant_id,campaign_id,alert_id,status,revision,campaign_revision,reason,idempotency_key) VALUES($1::uuid,$2,$3,$4,'linked',1,2,'real Kafka integration',$5)`, []interface{}{relationID, tenantID, campaignID, alertID, "campaign-event-integration-" + eventID}},
		{`INSERT INTO campaign_alert_link_history(event_id,relation_id,tenant_id,campaign_id,alert_id,event_type,revision,campaign_revision,payload) VALUES($1::uuid,$2::uuid,$3,$4,$5,'linked',1,2,$6::jsonb)`, []interface{}{eventID, relationID, tenantID, campaignID, alertID, string(payloadJSON)}},
		{`INSERT INTO campaign_aggregate_history(event_id,tenant_id,campaign_id,aggregate_revision,event_type,status,member_count,payload,reason) VALUES($1::uuid,$2,$3,2,'traffic.campaign.v2.AlertLinked','active',1,$4::jsonb,'real Kafka integration')`, []interface{}{eventID, tenantID, campaignID, string(payloadJSON)}},
		{`INSERT INTO campaign_alert_link_outbox(event_id,tenant_id,aggregate_id,aggregate_version,event_type,partition_key,payload) VALUES($1::uuid,$2,$3::uuid,1,'traffic.campaign.v2.AlertLinked',$4,$5::jsonb)`, []interface{}{eventID, tenantID, relationID, tenantID + ":" + campaignID, string(payloadJSON)}},
		{`INSERT INTO campaign_aggregate_outbox(event_id,tenant_id,aggregate_id,aggregate_revision,event_type,partition_key,payload) VALUES($1::uuid,$2,$3,2,'traffic.campaign.v2.AlertLinked',$4,$5::jsonb)`, []interface{}{eventID, tenantID, campaignID, tenantID + ":" + campaignID, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(context.Background(), statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rollback = false
	// Simulate a later committed membership mutation before the older outbox
	// event is delivered. The publisher must obtain campaign identity and
	// revision from the immutable history row keyed by event_id, not from the
	// mutable current relation.
	if _, err := db.Exec(`UPDATE campaign_alert_links SET revision=2,campaign_revision=3,updated_at=now() WHERE relation_id=$1::uuid`, relationID); err != nil {
		t.Fatal(err)
	}
	aggregateProducer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{Brokers: []string{broker}, Topic: CampaignAggregateEventTopic, BatchSize: 1, RequiredAcks: "all", Compression: "none", Async: false, IdempotentKey: "tenant+campaign"}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer aggregateProducer.Close()
	membershipProducer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{Brokers: []string{broker}, Topic: CampaignMembershipEventTopic, BatchSize: 1, RequiredAcks: "all", Compression: "none", Async: false, IdempotentKey: "tenant+campaign"}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer membershipProducer.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.SetCampaignEventProducers(aggregateProducer, membershipProducer)
	processed, err := handler.drainCampaignEventOutboxes(context.Background(), "campaign-real-worker", 10)
	if err != nil || processed != 2 {
		t.Fatalf("drain processed=%d err=%v", processed, err)
	}
	readAndProject := func(topic, stream string) {
		reader := segmentkafka.NewReader(segmentkafka.ReaderConfig{Brokers: []string{broker}, Topic: topic, Partition: 0, MinBytes: 1, MaxBytes: 1 << 20, MaxWait: time.Second})
		defer reader.Close()
		if err := reader.SetOffset(segmentkafka.FirstOffset); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var message segmentkafka.Message
		for {
			message, err = reader.ReadMessage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var candidate map[string]interface{}
			if json.Unmarshal(message.Value, &candidate) == nil && candidate["event_id"] == eventID {
				payload = candidate
				break
			}
		}
		input := CampaignEventProjectionInput{Stream: stream, EventID: eventID, TenantID: tenantID,
			CampaignID: campaignID, EventType: "traffic.campaign.v2.AlertLinked", SchemaVersion: 2,
			AggregateRevision: 2, PartitionKey: tenantID + ":" + campaignID, TraceID: traceID,
			Payload: payload, KafkaTopic: topic, KafkaPartition: message.Partition, KafkaOffset: message.Offset, ReceivedAt: message.Time}
		if stream == campaignAggregateStream {
			input.AggregateID = campaignID
		} else {
			input.AggregateID, input.RelationID, input.AlertID, input.RelationRevision = relationID, relationID, alertID, 1
		}
		if err := handler.ApplyCampaignEventProjection(context.Background(), input); err != nil {
			t.Fatalf("apply %s projection: %v", stream, err)
		}
	}
	readAndProject(CampaignAggregateEventTopic, campaignAggregateStream)
	readAndProject(CampaignMembershipEventTopic, campaignMembershipStream)
	var published, inbox, deliveries, watermarks int
	if err := db.QueryRow(`SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id=$1 AND status='published' AND published=true`, tenantID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	var membershipPublished int
	_ = db.QueryRow(`SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id=$1 AND status='published' AND published=true`, tenantID).Scan(&membershipPublished)
	_ = db.QueryRow(`SELECT count(*) FROM campaign_event_projection_inbox WHERE tenant_id=$1`, tenantID).Scan(&inbox)
	_ = db.QueryRow(`SELECT count(*) FROM campaign_event_projection_deliveries WHERE event_id=$1::uuid`, eventID).Scan(&deliveries)
	_ = db.QueryRow(`SELECT count(*) FROM campaign_event_projection_watermarks WHERE kafka_topic IN ($1,$2)`, CampaignAggregateEventTopic, CampaignMembershipEventTopic).Scan(&watermarks)
	if published != 1 || membershipPublished != 1 || inbox != 2 || deliveries != 2 || watermarks != 2 {
		t.Fatalf("aggregate_published=%d membership_published=%d inbox=%d deliveries=%d watermarks=%d", published, membershipPublished, inbox, deliveries, watermarks)
	}
	return campaignEventKafkaIntegrationResult{
		db: db, tenantID: tenantID, campaignID: campaignID, alertID: alertID,
		eventID: eventID, relationID: relationID, traceID: traceID,
	}
}

// TestCampaignEventRealKafkaProjectionBoundary retains the focused Kafka/PG
// proof while TestCampaignEventRealKafkaFourStoreTrace extends the same helper
// through the real ClickHouse, OpenSearch and NebulaGraph adapters.
func TestCampaignEventRealKafkaProjectionBoundary(t *testing.T) {
	_ = runCampaignEventRealKafkaProjectionBoundary(t)
}
