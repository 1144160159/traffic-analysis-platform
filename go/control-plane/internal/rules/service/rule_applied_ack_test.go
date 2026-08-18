package service

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/converter"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestHandleRuleUpdateAppliedAckKeepsPartialStateUntilExactSubtaskSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	command := model.RuleCommand{
		EventID: "11111111-1111-4111-8111-111111111111", Action: "update",
		Timestamp: time.UnixMilli(1720000000000), OperatorID: "operator-1", Version: 7,
		Rule: &model.Rule{RuleID: "rule-1", TenantID: "tenant-1", Version: 7, Enabled: true},
	}
	commandPayload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	ack := RuleUpdateAppliedAck{
		SchemaVersion: 1, EventID: command.EventID, TenantID: "tenant-1", RuleID: "rule-1",
		Version: 7, CurrentVersion: 7, Action: "update",
		Checksum:     converter.CommandToProto(&command).Checksum,
		SubtaskIndex: 0, Parallelism: 4, Status: "applied",
		Timestamp: "2026-08-14T10:00:00Z",
	}
	ackPayload, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule_id, payload, published")).
		WithArgs(command.EventID).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "payload", "published"}).
			AddRow("rule-1", commandPayload, true))
	mock.ExpectExec("INSERT INTO rule_update_applied_acks").
		WithArgs(command.EventID, "tenant-1", "rule-1", int64(7), "update", ack.Checksum,
			0, 4, "applied", int64(7), "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT COUNT\\(DISTINCT subtask_index\\)").
		WithArgs(command.EventID).
		WillReturnRows(sqlmock.NewRows([]string{
			"success_count", "has_failure", "failure_reason", "min_subtask", "max_subtask",
		}).AddRow(1, false, "", 0, 0))
	mock.ExpectExec("UPDATE rule_outbox SET").
		WithArgs(command.EventID, "partial", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	service := &RuleService{
		db:     db,
		config: RuleServiceConfig{AppliedAckExpectedParallelism: 4},
	}
	if err := service.HandleRuleUpdateAppliedAck(context.Background(), ackPayload); err != nil {
		t.Fatalf("handle acknowledgement: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRuleAckRuntimeStatusRequiresExactFullSetAndFailsOnNegativeAck(t *testing.T) {
	tests := []struct {
		name                        string
		success, min, max, expected int
		failure                     bool
		want                        string
	}{
		{name: "partial", success: 3, min: 0, max: 2, expected: 4, want: "partial"},
		{name: "gap", success: 4, min: 0, max: 4, expected: 4, want: "partial"},
		{name: "complete", success: 4, min: 0, max: 3, expected: 4, want: "applied"},
		{name: "conflict", success: 4, min: 0, max: 3, expected: 4, failure: true, want: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ruleAckRuntimeStatus(
				test.success, test.min, test.max, test.failure, test.expected)
			if got != test.want {
				t.Fatalf("status=%q want=%q", got, test.want)
			}
		})
	}
}

func TestValidateRuleUpdateAppliedAckRejectsConsumerControlledParallelism(t *testing.T) {
	ack := RuleUpdateAppliedAck{
		SchemaVersion: 1, EventID: "event-1", TenantID: "tenant-1", RuleID: "rule-1",
		Version: 1, CurrentVersion: 1, Action: "update",
		Checksum:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SubtaskIndex: 0, Parallelism: 2, Status: "applied",
		Timestamp: "2026-08-14T10:00:00Z",
	}
	if err := validateRuleUpdateAppliedAck(ack, 4); err == nil {
		t.Fatal("expected server-owned parallelism mismatch to fail")
	}
}

func TestExpireTimedOutRuleApplicationsMarksOnlyOverduePendingEventsFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE rule_outbox").
		WithArgs(now.Add(-2*time.Minute), "ACK_TIMEOUT: exact Flink subtask set not received within 2m0s").
		WillReturnResult(sqlmock.NewResult(0, 3))

	service := &RuleService{db: db, config: RuleServiceConfig{AppliedAckTimeout: 2 * time.Minute}}
	count, err := service.expireTimedOutRuleApplications(context.Background(), now)
	if err != nil {
		t.Fatalf("expireTimedOutRuleApplications() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("expired count = %d, want 3", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
