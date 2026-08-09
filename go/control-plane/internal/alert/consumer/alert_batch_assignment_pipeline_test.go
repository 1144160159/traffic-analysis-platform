package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type alertBatchAuthoritySpy struct {
	projectionCalls int
}

func (spy *alertBatchAuthoritySpy) GetAlert(context.Context, string, string) (*service.AlertDetailDTO, error) {
	panic("unexpected alert lookup")
}

func (spy *alertBatchAuthoritySpy) ProjectAlertAssignment(context.Context, string, string, string, string, uint64, uint64) (*service.AlertAssignmentProjectionResult, error) {
	spy.projectionCalls++
	return &service.AlertAssignmentProjectionResult{}, nil
}

func (spy *alertBatchAuthoritySpy) ProjectAlertAssignmentCompensation(context.Context, string, string, string, string, string, uint64, uint64) (*service.AlertAssignmentProjectionResult, error) {
	spy.projectionCalls++
	return &service.AlertAssignmentProjectionResult{}, nil
}

func alertBatchRequestedFixture() alertBatchAssignmentLifecycleEvent {
	return alertBatchAssignmentLifecycleEvent{
		EventID:             "11111111-1111-4111-8111-111111111111",
		EventType:           alertBatchRequestedEvent,
		SchemaVersion:       1,
		AggregateType:       "alert_assignment_batch",
		AggregateID:         "22222222-2222-4222-8222-222222222222",
		AggregateVersion:    1,
		PartitionKey:        "tenant-a:22222222-2222-4222-8222-222222222222",
		TenantID:            "tenant-a",
		BatchID:             "22222222-2222-4222-8222-222222222222",
		SelectionID:         "33333333-3333-4333-8333-333333333333",
		SelectionSnapshotID: "snapshot-alert-batch-0001",
		SelectionSHA256:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Assignee:            "analyst-a",
		RequestedBy:         "operator-a",
		Reason:              "investigation handoff",
		Status:              "accepted",
		TotalCount:          2,
		TraceID:             "trace-alert-batch-1",
		OccurredAt:          time.Date(2026, 8, 9, 21, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
}

func alertBatchChangedFixture() alertBatchAssignmentLifecycleEvent {
	event := alertBatchRequestedFixture()
	event.EventID = "44444444-4444-4444-8444-444444444444"
	event.EventType = alertAssignmentChangedEvent
	event.AggregateVersion = 2
	event.SelectionID = ""
	event.SelectionSnapshotID = ""
	event.SelectionSHA256 = ""
	event.Status = "running"
	event.Items = []alertAssignmentEventItem{
		{AlertID: "alert-a", Position: 0, ExpectedStateVersion: 1000, ResultingStateVersion: 1001, PreviousAssignee: "", ResultingAssignee: "analyst-a", PreviousStatus: "new", ResultingStatus: "assigned"},
		{AlertID: "alert-b", Position: 1, ExpectedStateVersion: 2000, ResultingStateVersion: 2001, PreviousAssignee: "analyst-b", ResultingAssignee: "analyst-a", PreviousStatus: "triage", ResultingStatus: "assigned"},
	}
	return event
}

func alertBatchMessage(t *testing.T, event alertBatchAssignmentLifecycleEvent, partition int, offset int64) *commonkafka.ReceivedMessage {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType, "schema_version": "1",
		"aggregate_type": event.AggregateType, "aggregate_id": event.BatchID,
		"aggregate_version": "1", "tenant_id": event.TenantID, "batch_id": event.BatchID,
		"trace_id": event.TraceID, "content_type": "application/json", "target_topic": AlertAssignmentEventTopic,
	}
	if event.AggregateVersion == 2 {
		headers["aggregate_version"] = "2"
	}
	kafkaHeaders := make([]segmentkafka.Header, 0, len(headers))
	for key, value := range headers {
		kafkaHeaders = append(kafkaHeaders, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: AlertAssignmentEventTopic, Partition: partition, Offset: offset,
		Key: []byte(event.PartitionKey), Value: payload, Headers: kafkaHeaders,
	}}
}

func TestAlertBatchAssignmentDecoderRejectsUnknownFieldsAndDuplicatePositions(t *testing.T) {
	pipeline := &AlertBatchAssignmentPipeline{topic: AlertAssignmentEventTopic}
	requested := alertBatchRequestedFixture()
	message := alertBatchMessage(t, requested, 0, 10)
	if _, err := pipeline.decodeEvent(message); err != nil {
		t.Fatalf("valid requested event rejected: %v", err)
	}
	message.Headers = append(message.Headers, segmentkafka.Header{Key: "event_id", Value: []byte(requested.EventID)})
	if _, err := pipeline.decodeEvent(message); err == nil {
		t.Fatal("duplicate identity header must be rejected")
	}
	message = alertBatchMessage(t, requested, 0, 10)
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = true
	message.Value, _ = json.Marshal(payload)
	if _, err := pipeline.decodeEvent(message); err == nil {
		t.Fatal("unknown event field must be rejected")
	}

	changed := alertBatchChangedFixture()
	changed.Items[1].Position = changed.Items[0].Position
	if _, err := pipeline.decodeEvent(alertBatchMessage(t, changed, 0, 11)); err == nil {
		t.Fatal("duplicate item positions must be rejected")
	}
}

func TestAlertBatchPermissionDecoderSupportsLegacyAndScopedShapes(t *testing.T) {
	for name, raw := range map[string]string{
		"global wildcard": `{"*":"*"}`,
		"domain wildcard": `{"alert":"*"}`,
		"boolean scope":   `{"alerts:write":true}`,
		"scope list":      `["alert:write"]`,
	} {
		t.Run(name, func(t *testing.T) {
			if !alertBatchPermissionsAllowWrite([]byte(raw)) {
				t.Fatalf("permission shape rejected: %s", raw)
			}
		})
	}
	for _, raw := range []string{`{"alerts:read":true}`, `[]`, `{`, `null`} {
		if alertBatchPermissionsAllowWrite([]byte(raw)) {
			t.Fatalf("read-only or invalid permission shape accepted: %s", raw)
		}
	}
}

func TestAlertBatchAssignmentExactInboxReplaySkipsAuthority(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	event := alertBatchChangedFixture()
	message := alertBatchMessage(t, event, 2, 41)
	payloadSHA, headersSHA := alertBatchMessageDigests(message)
	mock.ExpectQuery("SELECT event_type,tenant_id").WithArgs(event.EventID).WillReturnRows(
		sqlmock.NewRows([]string{"event_type", "tenant_id", "batch_id", "aggregate_type", "aggregate_id", "aggregate_version", "source_topic", "source_partition", "source_offset", "payload_sha256", "headers_sha256", "trace_id"}).
			AddRow(event.EventType, event.TenantID, event.BatchID, event.AggregateType, event.AggregateID, event.AggregateVersion, message.Topic, message.Partition, message.Offset, payloadSHA, headersSHA, event.TraceID),
	)
	spy := &alertBatchAuthoritySpy{}
	pipeline := &AlertBatchAssignmentPipeline{db: db, authority: spy, topic: AlertAssignmentEventTopic}
	if err := pipeline.HandleKafkaMessage(context.Background(), message); err != nil {
		t.Fatalf("exact inbox replay rejected: %v", err)
	}
	if spy.projectionCalls != 0 {
		t.Fatalf("exact inbox replay projected %d external effects", spy.projectionCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertBatchAssignmentIncompleteChangedEventFailsBeforeProjection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	event := alertBatchChangedFixture()
	message := alertBatchMessage(t, event, 3, 52)
	mock.ExpectQuery("SELECT event_type,tenant_id").WithArgs(event.EventID).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT b.status,b.revision").WithArgs(event.TenantID, event.BatchID, alertAssignmentChangedEvent, sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"status", "revision", "total_count", "assignee", "requested_by", "reason", "trace_id", "event_id", "payload_matches"}).
			AddRow("running", 2, event.TotalCount, event.Assignee, event.RequestedBy, event.Reason, event.TraceID, event.EventID, true),
	)
	mock.ExpectQuery("SELECT alert_id,position").WithArgs(event.TenantID, event.BatchID).WillReturnRows(
		sqlmock.NewRows([]string{"alert_id", "position", "expected_state_version", "resulting_state_version", "previous_assignee", "resulting_assignee", "previous_status", "resulting_status"}).
			AddRow(event.Items[0].AlertID, event.Items[0].Position, event.Items[0].ExpectedStateVersion, event.Items[0].ResultingStateVersion, event.Items[0].PreviousAssignee, event.Items[0].ResultingAssignee, event.Items[0].PreviousStatus, event.Items[0].ResultingStatus),
	)
	spy := &alertBatchAuthoritySpy{}
	pipeline := &AlertBatchAssignmentPipeline{db: db, authority: spy, topic: AlertAssignmentEventTopic}
	err = pipeline.HandleKafkaMessage(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("incomplete changed event error=%v, want permanent rejection", err)
	}
	if spy.projectionCalls != 0 {
		t.Fatalf("incomplete event projected %d external effects before authority validation", spy.projectionCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
