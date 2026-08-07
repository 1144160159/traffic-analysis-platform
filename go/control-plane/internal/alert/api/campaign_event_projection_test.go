package api

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyCampaignEventProjectionCommitsInboxDeliveryAndWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_event_projection_inbox")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_event_projection_deliveries")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_event_projection_watermarks")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = handler.ApplyCampaignEventProjection(context.Background(), CampaignEventProjectionInput{
		Stream: "aggregate", EventID: "11111111-1111-4111-8111-111111111111", TenantID: "tenant-a",
		AggregateID: "campaign-a", CampaignID: "campaign-a", EventType: "traffic.campaign.v2.StatusChanged",
		SchemaVersion: 2, AggregateRevision: 3, PartitionKey: "tenant-a:campaign-a", TraceID: "trace-1",
		Payload:    map[string]interface{}{"event_id": "11111111-1111-4111-8111-111111111111"},
		KafkaTopic: CampaignAggregateEventTopic, KafkaPartition: 1, KafkaOffset: 9, ReceivedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("ApplyCampaignEventProjection() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyCampaignEventProjectionRejectsIdentityCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, nil)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO campaign_event_projection_inbox")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (")).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()
	err = handler.ApplyCampaignEventProjection(context.Background(), CampaignEventProjectionInput{
		Stream: "aggregate", EventID: "22222222-2222-4222-8222-222222222222", TenantID: "tenant-a",
		AggregateID: "campaign-a", CampaignID: "campaign-a", EventType: "traffic.campaign.v2.StatusChanged",
		SchemaVersion: 2, AggregateRevision: 3, PartitionKey: "tenant-a:campaign-a", TraceID: "trace-2",
		Payload: map[string]interface{}{"event_id": "changed"}, KafkaTopic: CampaignAggregateEventTopic,
		KafkaPartition: 0, KafkaOffset: 1,
	})
	if err == nil {
		t.Fatal("identity collision must fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
