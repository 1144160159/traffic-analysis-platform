package fusion

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestClickHouseSourceFactReaderReturnsExplicitTruncation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Unix(100, 0).UTC()
	end := start.Add(time.Hour)
	mock.ExpectQuery(`SELECT count\(\) FROM traffic\.source_flow_facts_v1`).
		WithArgs("tenant-a", start.UnixMilli(), end.UnixMilli()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT aggregate_id,event_id,event_time_ms,source_topic,source_partition,source_offset,`).
		WithArgs("tenant-a", start.UnixMilli(), end.UnixMilli(), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"aggregate_id", "event_id", "event_time_ms", "source_topic", "source_partition", "source_offset",
			"source_payload_sha256", "source_version", "projection_identity", "payload_base64", "projection_hash",
		}).AddRow("flow-a", "event-a", start.UnixMilli(), "flow.events.v1", 1, int64(9),
			"payload-sha", uint64(3), "projection-id", "cGF5bG9hZA==", "projection-sha"))
	reader, err := NewClickHouseSourceFactReader(db)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := reader.ReadSourceFacts(context.Background(), "tenant-a", "traffic", start, end, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Truncated || batch.Total != 2 || len(batch.Facts) != 1 || batch.Facts[0].SourceOffset != 9 {
		t.Fatalf("unexpected source fact batch: %#v", batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
