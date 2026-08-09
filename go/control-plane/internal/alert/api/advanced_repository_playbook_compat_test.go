package api

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"go.uber.org/zap"
)

func TestQueryPlaybookExecutionsFallsBackToLegacySchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	queryPrefix := regexp.QuoteMeta("SELECT execution_id, tenant_id, playbook_name, alert_id,")
	mock.ExpectQuery(queryPrefix).
		WithArgs("tenant-a", "block-scanner", 100).
		WillReturnError(&pq.Error{
			Code:    "42703",
			Column:  "playbook_version",
			Message: `column "playbook_version" does not exist`,
		})

	createdAt := time.Date(2026, time.August, 5, 4, 30, 0, 0, time.UTC)
	mock.ExpectQuery(queryPrefix).
		WithArgs("tenant-a", "block-scanner", 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"execution_id", "tenant_id", "playbook_name", "alert_id",
			"success_actions", "failed_actions", "duration_ms",
			"request_payload", "result_payload", "mode", "status",
			"rollback_of", "effect_payload", "requested_by", "rolled_back_at", "created_at",
		}).AddRow(
			"execution-1", "tenant-a", "block-scanner", "alert-1",
			2, 0, int64(75),
			`{"reason":"legacy drill"}`, `{"status":"succeeded"}`, "drill", "succeeded",
			"", `{"external_effect_applied":false}`, "operator-a", nil, createdAt,
		))

	records, err := NewAdvancedRepository(db, zap.NewNop()).ListPlaybookExecutionsByName(
		context.Background(), "tenant-a", "block-scanner", 100,
	)
	if err != nil {
		t.Fatalf("list legacy executions: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one execution, got %d", len(records))
	}
	record := records[0]
	if record.ExecutionID != "execution-1" || record.PlaybookName != "block-scanner" || record.LegacyPlaybook != "block-scanner" {
		t.Fatalf("unexpected identity fields: %#v", record)
	}
	if record.ApprovalStatus != "not_required" || record.ExecutorStatus != "simulated" {
		t.Fatalf("legacy safety defaults drifted: %#v", record)
	}
	if record.UpdatedAt == nil || !record.UpdatedAt.Equal(createdAt) {
		t.Fatalf("legacy updated_at should use created_at, got %#v", record.UpdatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestQueryPlaybookExecutionsDoesNotHideUnknownColumnDrift(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT execution_id, tenant_id, playbook_name, alert_id,")).
		WithArgs("tenant-a", "block-scanner", 100).
		WillReturnError(&pq.Error{
			Code:    "42703",
			Column:  "unexpected_column",
			Message: `column "unexpected_column" does not exist`,
		})

	_, err = NewAdvancedRepository(db, zap.NewNop()).ListPlaybookExecutionsByName(
		context.Background(), "tenant-a", "block-scanner", 100,
	)
	if err == nil {
		t.Fatal("unknown schema drift must remain fail-closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
