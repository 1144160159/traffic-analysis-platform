package dataquality

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

const (
	testDQTenant = "tenant-dq-transaction"
	testDQUser   = "user-dq-requester"
	testDQTrace  = "0123456789abcdef0123456789abcdef"
)

func newBaselineMonitorMocks(t *testing.T) (*Monitor, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	clickhouseDB, clickhouseMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create ClickHouse mock: %v", err)
	}
	t.Cleanup(func() { _ = clickhouseDB.Close() })
	postgresDB, postgresMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create PostgreSQL mock: %v", err)
	}
	t.Cleanup(func() { _ = postgresDB.Close() })

	monitor := NewMonitor(clickhouseDB, MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(postgresDB)
	return monitor, clickhouseMock, postgresMock
}

func expectBaselineSourceReads(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT avg\(pps\), avg\(bps\), avg\(pktlen_mean\), count\(\)`).
		WithArgs(testDQTenant).
		WillReturnRows(sqlmock.NewRows([]string{"avg_pps", "avg_bps", "avg_pktlen", "sample_count"}).
			AddRow(101.5, 202.5, 303.5, int64(404)))
	mock.ExpectQuery(`SELECT name,type,default_kind FROM system\.columns`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "type", "default_kind"}).
			AddRow("tenant_id", "String", "").
			AddRow("event_id", "String", "").
			AddRow("event_ts", "Int64", ""))
	mock.ExpectQuery(`SELECT if\(count\(\) = 0, '', toString\(max\(ts\)\)\)`).
		WithArgs(testDQTenant).
		WillReturnRows(sqlmock.NewRows([]string{"feature_watermark"}).AddRow("2026-08-04 08:00:00"))
}

func expectBaselineTransaction(mock sqlmock.Sqlmock, failureStep string) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(testDQTenant + ":flows_raw").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO data_quality_datasets`).
		WithArgs(testDQTenant, testDQUser, testDQTrace, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(baseline_version\),0\)\+1`).
		WithArgs(testDQTenant).
		WillReturnRows(sqlmock.NewRows([]string{"baseline_version"}).AddRow(int64(1)))
	mock.ExpectExec(`UPDATE data_quality_baselines SET status='superseded'`).
		WithArgs(testDQTenant, "flows_raw", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO data_quality_baselines`).
		WithArgs(
			sqlmock.AnyArg(), testDQTenant, "flows_raw", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg(),
			uint64(404), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), testDQUser, testDQTrace,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	outbox := mock.ExpectExec(`INSERT INTO data_quality_outbox`).
		WithArgs(sqlmock.AnyArg(), testDQTenant, sqlmock.AnyArg(), int64(1), sqlmock.AnyArg(), testDQTrace, sqlmock.AnyArg())
	if failureStep == "outbox" {
		outbox.WillReturnError(errors.New("outbox unavailable"))
		mock.ExpectRollback()
		return
	}
	outbox.WillReturnResult(sqlmock.NewResult(0, 1))

	audit := mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), testDQTenant, testDQUser, sqlmock.AnyArg(), sqlmock.AnyArg(), testDQTrace, sqlmock.AnyArg())
	if failureStep == "audit" {
		audit.WillReturnError(errors.New("audit unavailable"))
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestUpdateBaselineCommitsDatasetBaselineOutboxAndAuditAtomically(t *testing.T) {
	monitor, clickhouseMock, postgresMock := newBaselineMonitorMocks(t)
	expectBaselineSourceReads(clickhouseMock)
	expectBaselineTransaction(postgresMock, "")

	baseline, err := monitor.UpdateBaseline(context.Background(), testDQTenant, testDQUser, testDQTrace)
	if err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}
	if baseline.BaselineVersion != 1 || baseline.SampleCount != 404 || len(baseline.SchemaColumns) != 3 {
		t.Fatalf("unexpected persistent baseline: %#v", baseline)
	}
	if len(baseline.SchemaSHA256) != 64 {
		t.Fatalf("schema hash length=%d want=64", len(baseline.SchemaSHA256))
	}
	if baseline.SourceWatermarks["clickhouse.feature_stat.max_ts"] != "2026-08-04 08:00:00" {
		t.Fatalf("unexpected source watermarks: %#v", baseline.SourceWatermarks)
	}
	if err := clickhouseMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ClickHouse expectations: %v", err)
	}
	if err := postgresMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("PostgreSQL expectations: %v", err)
	}
}

func TestUpdateBaselineRollsBackWhenOutboxOrAuditFails(t *testing.T) {
	for _, failureStep := range []string{"outbox", "audit"} {
		t.Run(failureStep, func(t *testing.T) {
			monitor, clickhouseMock, postgresMock := newBaselineMonitorMocks(t)
			expectBaselineSourceReads(clickhouseMock)
			expectBaselineTransaction(postgresMock, failureStep)

			baseline, err := monitor.UpdateBaseline(context.Background(), testDQTenant, testDQUser, testDQTrace)
			if err == nil || !strings.Contains(err.Error(), "insert baseline "+failureStep) {
				t.Fatalf("error=%v want insert baseline %s failure", err, failureStep)
			}
			if baseline != nil {
				t.Fatalf("baseline must not be returned after %s failure: %#v", failureStep, baseline)
			}
			if err := clickhouseMock.ExpectationsWereMet(); err != nil {
				t.Fatalf("ClickHouse expectations: %v", err)
			}
			if err := postgresMock.ExpectationsWereMet(); err != nil {
				t.Fatalf("PostgreSQL expectations: %v", err)
			}
		})
	}
}
