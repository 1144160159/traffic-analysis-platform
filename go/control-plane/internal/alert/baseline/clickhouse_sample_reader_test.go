package baseline

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestClickHouseSampleReaderExcludesPartialSessionRowsAndReportsPartial(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	job := BuildJob{TenantID: "tenant-a", BaselineID: "asset:10.0.0.1", BaselineKind: "dynamic",
		EntityType: "asset", EntityID: "10.0.0.1", WindowStart: &start, WindowEnd: &end}
	mock.ExpectQuery(`SELECT count\(\),countIf\(is_partial=0\)`).
		WithArgs(job.TenantID, start.UnixMilli(), end.UnixMilli(), job.EntityID).
		WillReturnRows(sqlmock.NewRows([]string{
			"total", "eligible", "max_event", "watermark", "bytes_mean", "bytes_std",
			"packets_mean", "packets_std", "duration_mean", "duration_std",
		}).AddRow(int64(120), int64(110), end.Add(-time.Second).UnixMilli(), int64(900), 10.0, 2.0, 3.0, 1.0, 40.0, 5.0))
	reader, err := NewClickHouseSampleReader(db)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.ReadDynamicSample(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if result.QualityStatus != "partial" || result.EligibleRowCount != 110 || len(result.PartialReasons) != 1 ||
		result.PartialReasons[0] != "partial_source_rows_excluded" {
		t.Fatalf("partial source rows were not fail-visible: %#v", result)
	}
	if result.MaxEventTime == nil || !result.MaxEventTime.Before(end) || len(result.SourceQuerySHA256) != 64 {
		t.Fatalf("sample window/watermark identity is incomplete: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClickHouseAccountSampleCannotClaimCompleteWithoutCompletenessColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	job := BuildJob{TenantID: "tenant-a", BaselineID: "account:alice", BaselineKind: "dynamic",
		EntityType: "account", EntityID: "alice", WindowStart: &start, WindowEnd: &end}
	mock.ExpectQuery(`SELECT count\(\),ifNull\(toInt64\(toUnixTimestamp\(max\(timestamp\)\)\)\*1000,0\)`).
		WithArgs(job.TenantID, start, end, job.EntityID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_event", "sources", "resources", "types"}).
			AddRow(int64(40), end.Add(-time.Second).UnixMilli(), int64(3), int64(5), int64(2)))
	reader, _ := NewClickHouseSampleReader(db)
	result, err := reader.ReadDynamicSample(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if result.QualityStatus != "partial" || result.EligibleRowCount != 0 ||
		len(result.PartialReasons) == 0 || result.PartialReasons[0] != "source_completeness_unavailable" {
		t.Fatalf("legacy account facts falsely claimed an eligible complete sample: %#v", result)
	}
}

func TestSessionEntityFilterDoesNotAcceptArbitrarySQL(t *testing.T) {
	if _, _, err := sessionEntityFilter("asset; DROP TABLE traffic.sessions", "x"); err == nil {
		t.Fatal("unregistered entity type reached the ClickHouse query")
	}
	if _, _, err := sessionEntityFilter("port", "443 OR 1=1"); err == nil {
		t.Fatal("unparsed port identity reached the ClickHouse query")
	}
}
