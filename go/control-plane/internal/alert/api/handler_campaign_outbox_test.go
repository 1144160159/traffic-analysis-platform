package api

import (
	"context"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestPublishCampaignAggregateMarksPublishedOnlyAfterAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "11111111-1111-4111-8111-111111111111"
	handler.campaignEventPublish = func(_ context.Context, key string, payload []byte, headers ...commonkafka.MessageHeader) error {
		if key != "tenant-a:campaign-a" {
			t.Fatalf("key=%q", key)
		}
		assertProbeHeader(t, headers, "event_id", eventID)
		assertProbeHeader(t, headers, "stream", "aggregate")
		assertProbeHeader(t, headers, "aggregate_version", "3")
		assertProbeHeader(t, headers, "relation_revision", "0")
		assertProbeHeader(t, headers, "target_topic", CampaignAggregateEventTopic)
		assertProbeHeader(t, headers, "trace_id", "trace-1")
		return nil
	}
	item := campaignOutboxItem{Stream: campaignAggregateStream, EventID: eventID, TenantID: "tenant-a",
		AggregateID: "campaign-a", CampaignID: "campaign-a", EventType: "traffic.campaign.v2.StatusChanged",
		PartitionKey: "tenant-a:campaign-a", AggregateRevision: 3, SchemaVersion: 2, Attempts: 1,
		Payload: []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.StatusChanged","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":3,"partition_key":"tenant-a:campaign-a","schema_version":2,"trace_id":"trace-1"}`)}
	mock.ExpectExec("UPDATE campaign_aggregate_outbox").WithArgs(eventID, "worker-a").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := handler.publishCampaignOutboxItem(context.Background(), "worker-a", &item); err != nil {
		t.Fatalf("publishCampaignOutboxItem() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishCampaignMembershipFailureTransitionsToDead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	handler.campaignMemberPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		return errors.New("broker unavailable")
	}
	eventID := "22222222-2222-4222-8222-222222222222"
	relationID := "33333333-3333-4333-8333-333333333333"
	item := campaignOutboxItem{Stream: campaignMembershipStream, EventID: eventID, TenantID: "tenant-a",
		AggregateID: relationID, RelationID: relationID, CampaignID: "campaign-a", AlertID: "alert-a",
		EventType: "traffic.campaign.v2.AlertLinked", PartitionKey: "tenant-a:campaign-a",
		AggregateRevision: 7, RelationRevision: 2, SchemaVersion: 2, Attempts: campaignOutboxMaxAttempts,
		Payload: []byte(`{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.AlertLinked","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":7,"partition_key":"tenant-a:campaign-a","schema_version":2,"campaign_id":"campaign-a","alert_id":"alert-a","relation_id":"` + relationID + `","relation_revision":2,"campaign_revision":7,"trace_id":"trace-2"}`)}
	mock.ExpectExec("UPDATE campaign_alert_link_outbox").
		WithArgs(eventID, "dead", "broker unavailable", "worker-b").WillReturnResult(sqlmock.NewResult(0, 1))
	err = handler.publishCampaignOutboxItem(context.Background(), "worker-b", &item)
	if err == nil || err.Error() != "broker unavailable" {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimCampaignMembershipUsesImmutableHistoryIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	eventID := "77777777-7777-4777-8777-777777777777"
	relationID := "88888888-8888-4888-8888-888888888888"
	payload := `{"event_id":"` + eventID + `","event_type":"traffic.campaign.v2.AlertLinked","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":7,"partition_key":"tenant-a:campaign-a","schema_version":2,"campaign_id":"campaign-a","alert_id":"alert-a","relation_id":"` + relationID + `","relation_revision":2,"campaign_revision":7,"trace_id":"trace-history"}`
	rows := sqlmock.NewRows([]string{
		"event_id", "tenant_id", "aggregate_id", "campaign_id", "alert_id", "event_type",
		"partition_key", "aggregate_version", "campaign_revision", "schema_version", "attempts", "payload",
	}).AddRow(eventID, "tenant-a", relationID, "campaign-a", "alert-a",
		"traffic.campaign.v2.AlertLinked", "tenant-a:campaign-a", int64(2), int64(7), 2, 1, payload)
	mock.ExpectQuery(`(?s)FROM claimed c\s+JOIN campaign_alert_link_history h ON h.event_id=c.event_id`).
		WithArgs(1, "worker-history").WillReturnRows(rows)

	items, err := handler.claimCampaignMembershipOutbox(context.Background(), "worker-history", 1)
	if err != nil {
		t.Fatalf("claimCampaignMembershipOutbox() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	if items[0].CampaignID != "campaign-a" || items[0].AlertID != "alert-a" ||
		items[0].RelationRevision != 2 || items[0].AggregateRevision != 7 {
		t.Fatalf("historical identity mismatch: %+v", items[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCampaignMembershipRejectsRelationCollision(t *testing.T) {
	item := campaignOutboxItem{Stream: campaignMembershipStream,
		EventID: "44444444-4444-4444-8444-444444444444", TenantID: "tenant-a",
		AggregateID: "55555555-5555-4555-8555-555555555555", RelationID: "55555555-5555-4555-8555-555555555555",
		CampaignID: "campaign-a", AlertID: "alert-a", EventType: "traffic.campaign.v2.AlertUnlinked",
		PartitionKey: "tenant-a:campaign-a", AggregateRevision: 8, RelationRevision: 3, SchemaVersion: 2,
		Payload: []byte(`{"event_id":"44444444-4444-4444-8444-444444444444","event_type":"traffic.campaign.v2.AlertUnlinked","tenant_id":"tenant-a","aggregate_type":"campaign","aggregate_id":"campaign-a","aggregate_version":8,"partition_key":"tenant-a:campaign-a","schema_version":2,"campaign_id":"campaign-a","alert_id":"alert-a","relation_id":"66666666-6666-4666-8666-666666666666","relation_revision":3,"campaign_revision":8,"trace_id":"trace-3"}`)}
	if err := validateCampaignOutboxItem(&item); err == nil {
		t.Fatal("relation collision must fail closed")
	}
}
