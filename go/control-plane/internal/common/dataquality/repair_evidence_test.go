package dataquality

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestRepairDryRunUsesPersistedTenantScopeAndBoundedClickHouseQuery(t *testing.T) {
	controlDB, controlMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer controlDB.Close()
	factsDB, factsMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer factsDB.Close()
	repairID := uuid.NewString()
	controlMock.ExpectQuery(`FROM data_quality_repairs\s+WHERE tenant_id=\$1 AND repair_id=\$2`).
		WithArgs("tenant-a", repairID).
		WillReturnRows(sqlmock.NewRows([]string{"operation_id", "status", "input_scope", "resource_budget"}).AddRow(
			"flow_replay_window_v1", "planned",
			[]byte(`{"dataset_id":"flows_raw","tenant_id":"tenant-a","window_start":"2026-08-04T12:00:00Z","window_end":"2026-08-04T12:05:00Z"}`),
			[]byte(`{"max_rows":1000,"max_duration_seconds":5}`),
		))
	factsMock.ExpectQuery(regexp.QuoteMeta("SELECT count(), uniqExact(event_id), groupBitXor(cityHash64(event_id)),")+`(?s).*WHERE tenant_id = \? AND ingest_ts >= \? AND ingest_ts < \?`).
		WithArgs("tenant-a", int64(1785844800000), int64(1785845100000)).
		WillReturnRows(sqlmock.NewRows([]string{"rows", "distinct", "hash", "min_ingest", "max_ingest"}).AddRow(uint64(3), uint64(2), uint64(42), int64(1785844801000), int64(1785845099000)))

	result, err := NewClickHouseRepairEvidenceProvider(controlDB, factsDB, 15*time.Second).DryRun(context.Background(), "tenant-a", repairID)
	if err != nil {
		t.Fatal(err)
	}
	if result["within_budget"] != true || result["destructive"] != false || result["estimated_rows"] != uint64(3) || result["duplicate_rows"] != uint64(1) || result["event_id_xor_hash"] != "000000000000002a" {
		t.Fatalf("unexpected dry-run evidence: %+v", result)
	}
	if err := controlMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := factsMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepairReconcileFailsClosedUntilProjectionOracleExists(t *testing.T) {
	provider := NewClickHouseRepairEvidenceProvider(nil, nil, time.Second)
	if _, err := provider.Reconcile(context.Background(), "tenant-a", uuid.NewString()); !errors.Is(err, ErrRepairReconcileUnavailable) {
		t.Fatalf("reconcile without an authoritative projection oracle must fail closed, got %v", err)
	}
}

func TestRepairReconcileComparesClickHouseTargetAndReceipts(t *testing.T) {
	controlDB, controlMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer controlDB.Close()
	factsDB, factsMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer factsDB.Close()
	repairID := uuid.NewString()
	controlMock.ExpectQuery(`FROM data_quality_repairs\s+WHERE tenant_id=\$1 AND repair_id=\$2`).
		WithArgs("tenant-a", repairID).
		WillReturnRows(sqlmock.NewRows([]string{"operation_id", "status", "input_scope", "resource_budget"}).AddRow(
			"flow_replay_window_v1", "executed",
			[]byte(`{"dataset_id":"flows_raw","tenant_id":"tenant-a","window_start":"2026-08-04T12:00:00Z","window_end":"2026-08-04T12:05:00Z"}`),
			[]byte(`{"max_rows":1000,"max_duration_seconds":5}`),
		))
	factsMock.ExpectQuery(`SELECT DISTINCT event_id(?s).*FROM traffic\.flows_raw`).
		WithArgs("tenant-a", int64(1785844800000), int64(1785845100000), int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"event_id"}).AddRow("event-1").AddRow("event-2"))
	controlMock.ExpectQuery(`SELECT event_id FROM data_quality_flow_replay_projection`).
		WithArgs("tenant-a", repairID, int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"event_id"}).AddRow("event-1").AddRow("event-2"))
	controlMock.ExpectQuery(`SELECT event_id FROM data_quality_replay_projection_receipts`).
		WithArgs("tenant-a", repairID, FlowReplayProjectionVersion, int64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"event_id"}).AddRow("event-1").AddRow("event-2"))
	controlMock.ExpectQuery(`SELECT count\(\*\)(?s).*source_event_sha256`).
		WithArgs("tenant-a", repairID, FlowReplayProjectionVersion).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	result, err := NewClickHouseRepairEvidenceProvider(controlDB, factsDB, 15*time.Second).Reconcile(context.Background(), "tenant-a", repairID)
	if err != nil {
		t.Fatal(err)
	}
	if result["all_match"] != true || result["source_count"] != 2 || result["target_count"] != 2 || result["receipt_count"] != 2 {
		t.Fatalf("unexpected reconcile evidence: %+v", result)
	}
	if err := controlMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := factsMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
