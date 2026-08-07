package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestInsertDiscoveryResourceAuditWritesTraceableActorColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(
			sqlmock.AnyArg(), "tenant-a", "analyst-a",
			"ASSET_DISCOVERY_CREDENTIAL_UPSERT", "credential-a", sqlmock.AnyArg(),
			"192.0.2.10", "remediation-test", "request-a", "trace-a",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = insertDiscoveryResourceAudit(
		context.Background(), tx, "tenant-a", "credential-a",
		"ASSET_DISCOVERY_CREDENTIAL_UPSERT",
		config.DiscoveryResourceCommand{
			ActionID: "asset-discovery-credential-upsert",
			Actor:    "analyst-a", Reason: "test traceable audit",
			TraceID: "trace-a", RequestID: "request-a",
			ClientIP: "192.0.2.10", UserAgent: "remediation-test",
		},
		map[string]any{"revision": 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
