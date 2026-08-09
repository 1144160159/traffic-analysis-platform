package server

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/DATA-DOG/go-sqlmock"
)

func expectNewProbeRegistration(
	mock sqlmock.Sqlmock,
	tenantID string,
	probeID string,
	softwareVersion string,
) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("probe-registry:" + probeID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT request_sha256").
		WithArgs(tenantID, sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT tenant_id,revision").
		WithArgs(probeID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO probes").
		WithArgs(probeID, tenantID, probeID, sqlmock.AnyArg(), softwareVersion, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO probe_registry_history").
		WithArgs(sqlmock.AnyArg(), tenantID, probeID, int64(1), probeRegisteredEventType, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(sqlmock.AnyArg(), tenantID, probeID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestPostgresProbeRegistryRegisterAndHeartbeat(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	registry := NewPostgresProbeRegistry(db)
	expectNewProbeRegistration(mock, "default", "probe-agent", "0.1.0")
	mock.ExpectExec("INSERT INTO probe_registry_outbox").
		WithArgs(sqlmock.AnyArg(), "default", "probe-agent", probeRegisteredEventType, int64(1), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO probe_registry_requests").
		WithArgs("default", sqlmock.AnyArg(), sqlmock.AnyArg(), "probe-agent", sqlmock.AnyArg(), int64(1), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := registry.Register(
		context.Background(),
		"default",
		"probe-agent",
		"0.1.0",
		"candidate",
		&pb.HardwareInfo{CpuCores: 8},
	); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE probes").
		WithArgs("default", "probe-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := registry.Heartbeat(
		context.Background(),
		"default",
		"probe-agent",
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresProbeRegistryRejectsCrossTenantRebind(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	registry := NewPostgresProbeRegistry(db)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("probe-registry:probe-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT request_sha256").
		WithArgs("other", sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT tenant_id,revision").
		WithArgs("probe-agent").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "revision"}).AddRow("default", int64(4)))
	mock.ExpectRollback()
	if err := registry.Register(
		context.Background(),
		"other",
		"probe-agent",
		"",
		"",
		nil,
	); err == nil {
		t.Fatal("expected cross-tenant binding rejection")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresProbeRegistryRollsBackWhenOutboxInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	registry := NewPostgresProbeRegistry(db)
	expectNewProbeRegistration(mock, "default", "probe-agent", "0.1.0")
	mock.ExpectExec("INSERT INTO probe_registry_outbox").
		WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()
	if err := registry.Register(
		context.Background(), "default", "probe-agent", "0.1.0", "candidate", nil,
	); err == nil {
		t.Fatal("expected registration transaction failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
