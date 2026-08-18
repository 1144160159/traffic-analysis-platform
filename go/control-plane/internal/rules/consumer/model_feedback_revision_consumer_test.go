package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeModelFeedbackPredictionAuthority struct {
	prediction ModelFeedbackPrediction
	err        error
}

func (authority fakeModelFeedbackPredictionAuthority) LookupModelFeedbackPrediction(
	context.Context, string,
) (ModelFeedbackPrediction, error) {
	return authority.prediction, authority.err
}

type fakeModelFeedbackRevisionApplier struct {
	inputs []ModelFeedbackRevisionProjectionInput
	err    error
}

func (applier *fakeModelFeedbackRevisionApplier) ApplyModelFeedbackRevision(
	_ context.Context, input ModelFeedbackRevisionProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func modelFeedbackMessage(t *testing.T) (*commonkafka.ReceivedMessage, modelFeedbackEventV1) {
	t.Helper()
	event := modelFeedbackEventV1{
		EventID:   "11111111-1111-4111-8111-111111111111",
		EventType: modelFeedbackEventType, SchemaVersion: 1, AggregateVersion: 1,
		FeedbackID: "22222222-2222-4222-8222-222222222222",
		TenantID:   "tenant-a", PredictionID: "prediction-1", AlertID: "alert-1",
		Label: "FP", LabelRevision: 1, AdjudicationState: "ADJUDICATED",
		ReasonCode: "KNOWN_SCANNER", ModelVersion: "model-v2", RuleVersion: "rule-v7",
		AdjudicatedBy: "reviewer-a", OccurredAtMS: 1786773600000,
		TraceID: "0123456789abcdef0123456789abcdef",
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]segmentkafka.Header, 0, 8)
	for name, value := range map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": event.TenantID, "feedback_id": event.FeedbackID,
		"prediction_id": event.PredictionID, "label_revision": "1",
	} {
		headers = append(headers, segmentkafka.Header{Key: name, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: modelFeedbackEventType, Key: []byte(event.TenantID + ":" + event.FeedbackID),
		Partition: 2, Offset: 41, Value: payload, Headers: headers,
	}}, event
}

func modelFeedbackAuthority(event modelFeedbackEventV1) ModelFeedbackPrediction {
	return ModelFeedbackPrediction{
		TenantID: event.TenantID, PredictionID: event.PredictionID, AlertID: event.AlertID,
		ModelVersion: event.ModelVersion, RuleVersion: event.RuleVersion,
	}
}

func TestModelFeedbackRevisionConsumerAppliesAuthorityBoundEvent(t *testing.T) {
	message, event := modelFeedbackMessage(t)
	applier := &fakeModelFeedbackRevisionApplier{}
	consumer := &ModelFeedbackRevisionConsumer{
		authority: fakeModelFeedbackPredictionAuthority{prediction: modelFeedbackAuthority(event)},
		applier:   applier,
	}
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want=1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.PayloadSHA256 == "" || input.KafkaTopic != modelFeedbackEventType ||
		input.LabelRevision != 1 || input.AdjudicationState != "ADJUDICATED" {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestModelFeedbackRevisionConsumerRejectsCrossTenantAuthority(t *testing.T) {
	message, event := modelFeedbackMessage(t)
	authority := modelFeedbackAuthority(event)
	authority.TenantID = "tenant-b"
	applier := &fakeModelFeedbackRevisionApplier{}
	consumer := &ModelFeedbackRevisionConsumer{
		authority: fakeModelFeedbackPredictionAuthority{prediction: authority}, applier: applier,
	}
	err := consumer.handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("cross-tenant authority error=%v want permanent", err)
	}
	if len(applier.inputs) != 0 {
		t.Fatal("cross-tenant feedback reached the projection")
	}
}

func TestModelFeedbackRevisionConsumerRejectsUnknownFieldAndDuplicateHeader(t *testing.T) {
	message, event := modelFeedbackMessage(t)
	var payload map[string]any
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = true
	message.Value, _ = json.Marshal(payload)
	consumer := &ModelFeedbackRevisionConsumer{
		authority: fakeModelFeedbackPredictionAuthority{prediction: modelFeedbackAuthority(event)},
		applier:   &fakeModelFeedbackRevisionApplier{},
	}
	if err := consumer.handle(context.Background(), message); err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("unknown field error=%v want permanent", err)
	}

	message, _ = modelFeedbackMessage(t)
	message.Headers = append(message.Headers, segmentkafka.Header{Key: "event_id", Value: []byte(event.EventID)})
	if err := consumer.handle(context.Background(), message); err == nil || !commonkafka.IsPermanent(err) {
		t.Fatalf("duplicate header error=%v want permanent", err)
	}
}

func TestValidateModelFeedbackRevisionStateMachine(t *testing.T) {
	input := modelFeedbackProjectionFixture(t)
	if err := validateModelFeedbackRevision(nil, input); err != nil {
		t.Fatalf("first revision rejected: %v", err)
	}
	input.LabelRevision = 2
	input.PreviousEventID = input.EventID
	if err := validateModelFeedbackRevision(nil, input); !errors.Is(err, errModelFeedbackOutOfOrder) {
		t.Fatalf("first revision gap error=%v", err)
	}

	head := &modelFeedbackRevisionHead{
		TenantID: input.TenantID, PredictionID: input.PredictionID, AlertID: input.AlertID,
		ModelVersion: input.ModelVersion, RuleVersion: input.RuleVersion, Label: input.Label,
		LabelRevision: 1, AdjudicationState: "ADJUDICATED", LastEventID: input.EventID,
	}
	input.EventID = "33333333-3333-4333-8333-333333333333"
	input.LabelRevision = 2
	input.PreviousEventID = head.LastEventID
	input.AdjudicationState = "RETRACTED"
	if err := validateModelFeedbackRevision(head, input); err != nil {
		t.Fatalf("ordered retraction rejected: %v", err)
	}
	input.Label = "TP"
	if err := validateModelFeedbackRevision(head, input); !errors.Is(err, errModelFeedbackConflict) {
		t.Fatalf("changed-label retraction error=%v", err)
	}
	head.AdjudicationState = "RETRACTED"
	input.Label = head.Label
	if err := validateModelFeedbackRevision(head, input); !errors.Is(err, errModelFeedbackRetracted) {
		t.Fatalf("post-retraction revision error=%v", err)
	}
}

func modelFeedbackProjectionFixture(t *testing.T) ModelFeedbackRevisionProjectionInput {
	t.Helper()
	message, event := modelFeedbackMessage(t)
	return ModelFeedbackRevisionProjectionInput{
		EventID: event.EventID, FeedbackID: event.FeedbackID, TenantID: event.TenantID,
		PredictionID: event.PredictionID, AlertID: event.AlertID, Label: event.Label,
		LabelRevision: event.LabelRevision, AdjudicationState: event.AdjudicationState,
		ReasonCode: event.ReasonCode, ModelVersion: event.ModelVersion,
		RuleVersion: event.RuleVersion, AdjudicatedBy: event.AdjudicatedBy,
		OccurredAtMS: event.OccurredAtMS, TraceID: event.TraceID,
		Payload: message.Value, PayloadSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		KafkaTopic: message.Topic, KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
}

func TestPostgresModelFeedbackRevisionProjectionCommitsFirstRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresModelFeedbackRevisionProjectionWithReadiness(
		db,
		ModelFeedbackConsumerReadinessOptions{
			ConsumerGroup:   "rule-manager-model-feedback-revision-v1",
			CandidateSHA256: strings.Repeat("c", 64),
			ContractSHA256:  strings.Repeat("d", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := modelFeedbackProjectionFixture(t)
	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE event_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("WHERE feedback_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE tenant_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM model_feedback_revision_head").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO model_feedback_revision_inbox").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_feedback_revision_head").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_feedback_revision_receipt").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_feedback_consumer_readiness_receipt").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := projection.ApplyModelFeedbackRevision(context.Background(), input); err != nil {
		t.Fatalf("first revision rejected: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresModelFeedbackRevisionProjectionRejectsUnapprovedReadinessIdentity(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = NewPostgresModelFeedbackRevisionProjectionWithReadiness(
		db,
		ModelFeedbackConsumerReadinessOptions{
			ConsumerGroup:   "rule-manager-model-feedback-revision-v1",
			CandidateSHA256: strings.Repeat("0", 64),
			ContractSHA256:  strings.Repeat("d", 64),
		},
	)
	if err == nil {
		t.Fatal("zero candidate digest was accepted for readiness")
	}
}

func TestPostgresModelFeedbackRevisionProjectionAcceptsExactReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresModelFeedbackRevisionProjection(db)
	input := modelFeedbackProjectionFixture(t)
	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE event_id").WillReturnRows(sqlmock.NewRows(
		[]string{"feedback_id", "label_revision", "payload_sha256"},
	).AddRow(input.FeedbackID, input.LabelRevision, input.PayloadSHA256))
	mock.ExpectCommit()
	if err := projection.ApplyModelFeedbackRevision(context.Background(), input); err != nil {
		t.Fatalf("exact replay rejected: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresModelFeedbackRevisionProjectionRejectsSameEventDifferentHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresModelFeedbackRevisionProjection(db)
	input := modelFeedbackProjectionFixture(t)
	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE event_id").WillReturnRows(sqlmock.NewRows(
		[]string{"feedback_id", "label_revision", "payload_sha256"},
	).AddRow(input.FeedbackID, input.LabelRevision, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	mock.ExpectRollback()
	err = projection.ApplyModelFeedbackRevision(context.Background(), input)
	if !errors.Is(err, errModelFeedbackConflict) {
		t.Fatalf("same event different hash error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresModelFeedbackRevisionProjectionRejectsSecondAggregateForPrediction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresModelFeedbackRevisionProjection(db)
	input := modelFeedbackProjectionFixture(t)
	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE event_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("WHERE feedback_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WHERE tenant_id").WillReturnRows(
		sqlmock.NewRows([]string{"feedback_id"}).AddRow(
			"99999999-9999-4999-8999-999999999999",
		),
	)
	mock.ExpectRollback()
	err = projection.ApplyModelFeedbackRevision(context.Background(), input)
	if !errors.Is(err, errModelFeedbackConflict) {
		t.Fatalf("second prediction aggregate error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
