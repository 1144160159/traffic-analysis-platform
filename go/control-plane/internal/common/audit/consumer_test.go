package audit

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	auditkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func makeMsg(data []byte) *auditkafka.ReceivedMessage {
	return &auditkafka.ReceivedMessage{
		Message: kafkago.Message{Value: data, Offset: 0, Partition: 0},
	}
}

func TestConsumerIsNotReadyBeforeSchemaVerification(t *testing.T) {
	consumer := &Consumer{}
	if consumer.Ready() {
		t.Fatal("consumer must fail closed before schema verification")
	}
}

func TestConsumerPermanentPayloadRemainsReadyForDurableDLQ(t *testing.T) {
	consumer := &Consumer{}
	consumer.ready.Store(true)
	err := consumer.handleMessageWithReadiness(context.Background(), makeMsg([]byte("not-an-audit-event")))
	if err == nil || !auditkafka.IsPermanent(err) {
		t.Fatal("expected invalid audit payload to fail")
	}
	if !consumer.Ready() {
		t.Fatal("permanent payload is handled by the DLQ barrier and must not mimic a PG outage")
	}
}

func TestConsumerTransientPersistenceFailureWithdrawsReadiness(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin().WillReturnError(context.DeadlineExceeded)

	event := &pb.AuditLog{EventId: "audit-pg-failure", TenantId: "tenant-a", Action: "EXPORT"}
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	consumer := &Consumer{db: db, logger: zap.NewNop()}
	consumer.ready.Store(true)
	err = consumer.handleMessageWithReadiness(context.Background(), makeMsg(payload))
	if err == nil || auditkafka.IsPermanent(err) {
		t.Fatalf("PG failure must remain transient, got %v", err)
	}
	if consumer.Ready() {
		t.Fatal("transient PG failure must withdraw readiness")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerPersistsMessageTransactionallyBeforeSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectPrepare(regexp.QuoteMeta("INSERT INTO audit_logs"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_logs")).
		WithArgs(
			"audit-transaction", "tenant-a", "", "EXPORT", "unknown",
			"", "{}", "", "", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	event := &pb.AuditLog{EventId: "audit-transaction", TenantId: "tenant-a", Action: "EXPORT"}
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	consumer := &Consumer{db: db, logger: zap.NewNop()}
	if err := consumer.handleMessageWithReadiness(context.Background(), makeMsg(payload)); err != nil {
		t.Fatalf("handleMessageWithReadiness: %v", err)
	}
	if !consumer.Ready() {
		t.Fatal("successful transaction must restore readiness")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func completeAuditSchemaRows() *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"})
	for name, column := range requiredAuditSchema {
		nullable := "NO"
		if column.nullable {
			nullable = "YES"
		}
		rows.AddRow(name, column.dataType, nullable)
	}
	return rows
}

func TestVerifySchemaAcceptsMigratedAuditTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name, data_type, is_nullable")).
		WillReturnRows(completeAuditSchemaRows())
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	consumer := &Consumer{db: db}
	if err := consumer.verifySchema(context.Background()); err != nil {
		t.Fatalf("verifySchema() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySchemaFailsClosedWithoutRequiredColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := completeAuditSchemaRows()
	// Use a fresh incomplete result because sqlmock rows cannot remove a row.
	rows = sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable"})
	for name, column := range requiredAuditSchema {
		if name == "event_id" {
			continue
		}
		nullable := "NO"
		if column.nullable {
			nullable = "YES"
		}
		rows.AddRow(name, column.dataType, nullable)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name, data_type, is_nullable")).
		WillReturnRows(rows)

	consumer := &Consumer{db: db}
	err = consumer.verifySchema(context.Background())
	if err == nil || !regexp.MustCompile(`missing column event_id`).MatchString(err.Error()) {
		t.Fatalf("verifySchema() error = %v, want missing event_id", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySchemaFailsClosedWithoutEventIDUniqueIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT column_name, data_type, is_nullable")).
		WillReturnRows(completeAuditSchemaRows())
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	consumer := &Consumer{db: db}
	err = consumer.verifySchema(context.Background())
	if err == nil || !regexp.MustCompile(`unique index missing`).MatchString(err.Error()) {
		t.Fatalf("verifySchema() error = %v, want unique index missing", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestParseAuditLogMessage(t *testing.T) {
	c := &Consumer{}

	batch := &pb.AuditLogBatch{
		Events: []*pb.AuditLog{{
			EventId: "audit-001", TenantId: "tenant1", UserId: "user1",
			Action: "ALERT_TRIAGE", ObjectType: "alert", ObjectId: "alert-001",
			Detail: `{"note":"confirmed"}`, IpAddr: "192.168.1.1",
			UserAgent: "curl/7.68", CreatedAt: 1717670400000,
		}},
	}
	data, _ := proto.Marshal(batch)

	result, err := c.parseMessages(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected one result, got %d", len(result))
	}
	if result[0].eventID != "audit-001" {
		t.Errorf("expected audit-001, got %s", result[0].eventID)
	}
}

func TestParseAuditLogBatchPreservesEveryEvent(t *testing.T) {
	c := &Consumer{}
	batch := &pb.AuditLogBatch{Events: []*pb.AuditLog{
		{EventId: "audit-001", TenantId: "tenant1", Action: "FIRST"},
		{EventId: "audit-002", TenantId: "tenant1", Action: "SECOND"},
		{EventId: "audit-003", TenantId: "tenant1", Action: "THIRD"},
	}}
	data, _ := proto.Marshal(batch)

	result, err := c.parseMessages(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != len(batch.Events) {
		t.Fatalf("parsed %d events, want %d", len(result), len(batch.Events))
	}
	for index, event := range batch.Events {
		if result[index].eventID != event.EventId {
			t.Fatalf("event %d id=%s want=%s", index, result[index].eventID, event.EventId)
		}
	}
}

func TestParseAuditLogSingle(t *testing.T) {
	c := &Consumer{}
	single := &pb.AuditLog{
		EventId: "audit-single", TenantId: "t2", UserId: "u2",
		Action: "LOGIN", CreatedAt: 1717670400000,
	}
	data, _ := proto.Marshal(single)
	result, err := c.parseMessages(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].eventID != "audit-single" {
		t.Error("failed to parse single AuditLog")
	}
}

func TestParseAuditLogJSON(t *testing.T) {
	c := &Consumer{}
	jsonData := []byte(`{"event_id":"json-audit","tenant_id":"t3","action":"EXPORT","detail":{"format":"pdf"}}`)
	result, err := c.parseMessages(makeMsg(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].eventID != "json-audit" {
		t.Error("failed to parse JSON audit")
	}
	if result[0].objectType != "unknown" || result[0].detail != `{"format":"pdf"}` {
		t.Fatalf("unexpected normalized JSON audit: %+v", result[0])
	}
}

func TestParseAuditLogDefaultsRequiredDatabaseValues(t *testing.T) {
	c := &Consumer{}
	single := &pb.AuditLog{
		EventId: "audit-defaults", TenantId: "tenant1", Action: "EXPORT",
		UserId: "service:report-worker",
	}
	data, _ := proto.Marshal(single)

	result, err := c.parseMessages(makeMsg(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected one result, got %d", len(result))
	}
	if result[0].userID != "service:report-worker" {
		t.Fatalf("user_id=%q", result[0].userID)
	}
	if result[0].objectType != "unknown" || result[0].detail != "{}" {
		t.Fatalf("unexpected defaults: %+v", result[0])
	}
}

func TestParseAuditLogRejectsInvalidDetailJSON(t *testing.T) {
	c := &Consumer{}
	single := &pb.AuditLog{
		EventId: "audit-invalid-detail", TenantId: "tenant1", Action: "EXPORT",
		Detail: "{",
	}
	data, _ := proto.Marshal(single)

	if _, err := c.parseMessages(makeMsg(data)); err == nil || !regexp.MustCompile(`detail must be valid JSON`).MatchString(err.Error()) {
		t.Fatalf("error=%v, want invalid detail rejection", err)
	}
}

func TestParseAuditLogUnknown(t *testing.T) {
	c := &Consumer{}
	_, err := c.parseMessages(makeMsg([]byte("garbage")))
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestDefaultConsumerConfig(t *testing.T) {
	cfg := DefaultConsumerConfig()
	if cfg.Topic != "audit.logs" || cfg.GroupID != "audit-consumer" || cfg.BatchSize != 200 {
		t.Error("default config mismatch")
	}
}
