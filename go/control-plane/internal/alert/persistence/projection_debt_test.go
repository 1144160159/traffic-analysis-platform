package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProjectionDebtBatchCommitsAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").
		WithArgs("tenant-a", "alert-a", "event-a", sqlmock.AnyArg(), sqlmock.AnyArg(), "alerts-v2-write", "OpenSearch failed").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").
		WithArgs("tenant-a", "alert-b", "event-b", sqlmock.AnyArg(), sqlmock.AnyArg(), "alerts-v2-write", "OpenSearch failed").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	now := time.Unix(1_800_000_000, 0).UTC()
	alerts := []*Alert{
		{TenantID: "tenant-a", AlertID: "alert-a", EventID: "event-a", UpdatedTs: now},
		{TenantID: "tenant-a", AlertID: "alert-b", EventID: "event-b", UpdatedTs: now},
	}
	if err := NewProjectionDebtStore(db).RecordProjectionDebt(context.Background(), alerts, "alerts-v2-write", errors.New("OpenSearch failed")); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionDebtBatchRollsBackIfAnyRowFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_opensearch_projection_debts").WillReturnError(errors.New("PostgreSQL unavailable"))
	mock.ExpectRollback()
	now := time.Unix(1_800_000_000, 0).UTC()
	alerts := []*Alert{{TenantID: "tenant-a", AlertID: "alert-a", UpdatedTs: now}, {TenantID: "tenant-a", AlertID: "alert-b", UpdatedTs: now}}
	if err := NewProjectionDebtStore(db).RecordProjectionDebt(context.Background(), alerts, "alerts-v2-write", errors.New("OpenSearch failed")); err == nil {
		t.Fatal("partial debt persistence must fail and roll back")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionDebtSchemaReadinessIsMigrationBound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT version FROM alignment_schema_migrations").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("202608041100"))
	if err := NewProjectionDebtStore(db).CheckSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
