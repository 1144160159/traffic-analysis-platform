package dataquality

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

type replayPublisherStub struct {
	events []*pb.FlowEvent
}

func (*replayPublisherStub) Ready(context.Context) error { return nil }
func (*replayPublisherStub) Target() string              { return "flow.projection-replay.v1" }
func (p *replayPublisherStub) Publish(_ context.Context, _ string, events []*pb.FlowEvent) error {
	p.events = append(p.events, events...)
	return nil
}

func TestClickHouseFlowReplayDriverRechecksBudgetDeduplicatesAndPreservesStableIdentity(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectPing()
	mock.ExpectQuery(`SELECT count\(\)(?s).*WHERE tenant_id = \? AND ingest_ts >= \? AND ingest_ts < \?`).
		WithArgs("tenant-a", int64(1785844800000), int64(1785845100000)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(uint64(2)))
	columns := []string{
		"event_id", "tenant_id", "probe_id", "community_id", "src_ip", "dst_ip", "src_port", "dst_port", "protocol", "direction",
		"ts_start", "ts_end", "duration_ms", "packets_fwd", "packets_bwd", "bytes_fwd", "bytes_bwd", "pps", "bps",
		"tcp_flags_fwd", "tcp_flags_bwd", "tos", "run_id", "feature_set_id", "event_ts", "ingest_ts",
		"pktlen_min", "pktlen_max", "pktlen_mean", "pktlen_std", "iat_min_ms", "iat_max_ms", "iat_mean_ms", "iat_std_ms",
		"active_min_ms", "active_max_ms", "active_mean_ms", "active_std_ms", "idle_min_ms", "idle_max_ms", "idle_mean_ms", "idle_std_ms", "subflow_count",
	}
	values := []driver.Value{
		"event-1", "tenant-a", "probe-a", "community-a", "10.0.0.1", "10.0.0.2", uint32(1234), uint32(443), uint32(6), "bidirectional",
		int64(1785844801000), int64(1785844802000), uint32(1000), uint32(2), uint32(3), uint64(100), uint64(200), float32(5), float32(300),
		uint32(2), uint32(16), uint32(0), "run-a", "features-v1", int64(1785844802000), int64(1785844803000),
		uint32(40), uint32(1500), float32(500), float32(20), float32(1), float32(4), float32(2), float32(1),
		float32(3), float32(9), float32(5), float32(2), float32(10), float32(30), float32(20), float32(4), uint32(1),
	}
	source := sqlmock.NewRows(columns).AddRow(values...).AddRow(values...)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")+`(?s).*FROM traffic\.flows_raw(?s).*ORDER BY ingest_ts,event_id(?s).*LIMIT \?`).
		WithArgs("tenant-a", int64(1785844800000), int64(1785845100000), int64(10)).
		WillReturnRows(source)
	publisher := &replayPublisherStub{}
	driver := NewClickHouseFlowReplayDriver(db, publisher, 1)
	driver.clock = func() time.Time { return time.Date(2026, 8, 4, 12, 20, 0, 0, time.UTC) }
	summary, err := driver.Replay(context.Background(), RepairReplayRequest{
		TenantID: "tenant-a", RepairID: "repair-a", OperationID: "flow_replay_window_v1", Revision: 5, TraceID: "trace-a",
		InputScope:     map[string]interface{}{"dataset_id": "flows_raw", "tenant_id": "tenant-a", "window_start": "2026-08-04T12:00:00Z", "window_end": "2026-08-04T12:05:00Z"},
		ResourceBudget: map[string]interface{}{"max_rows": float64(10), "max_duration_seconds": float64(5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary["published"] != true || summary["published_rows"] != int64(1) || summary["duplicate_rows_skipped"] != int64(1) {
		t.Fatalf("unexpected replay summary: %+v", summary)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one deduplicated replay event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Header.EventId != "event-1" || event.Header.TenantId != "tenant-a" || event.Header.IdempotencyKey != "repair-a:event-1" || event.Header.Producer != "data-quality-repair-executor" || event.Header.CausationId != "repair-a" || event.Header.TraceId != "trace-a" {
		t.Fatalf("stable replay envelope was not preserved: %+v", event.Header)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClickHouseFlowReplayDriverFailsBeforePublishingWhenBudgetDrifts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectPing()
	mock.ExpectQuery(`SELECT count\(\)(?s).*WHERE tenant_id = \?`).
		WithArgs("tenant-a", int64(1785844800000), int64(1785845100000)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(uint64(11)))
	publisher := &replayPublisherStub{}
	driver := NewClickHouseFlowReplayDriver(db, publisher, 500)
	summary, err := driver.Replay(context.Background(), RepairReplayRequest{
		TenantID: "tenant-a", RepairID: "repair-a", OperationID: "flow_replay_window_v1",
		InputScope:     map[string]interface{}{"dataset_id": "flows_raw", "tenant_id": "tenant-a", "window_start": "2026-08-04T12:00:00Z", "window_end": "2026-08-04T12:05:00Z"},
		ResourceBudget: map[string]interface{}{"max_rows": float64(10), "max_duration_seconds": float64(5)},
	})
	if err == nil || summary["published_rows"] != int64(0) || len(publisher.events) != 0 {
		t.Fatalf("budget drift must fail before publish: summary=%+v err=%v events=%d", summary, err, len(publisher.events))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaFlowReplayPublisherRefusesRawIngestAndTopicMismatch(t *testing.T) {
	producer, err := commonkafka.NewProducer(commonkafka.ProducerConfig{
		Brokers: []string{"127.0.0.1:1"}, Topic: "flow.events.v1", RequiredAcks: "all",
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	raw := NewKafkaFlowReplayPublisher(producer, "flow.events.v1", func(context.Context, string) error { return nil })
	if err := raw.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "refuses the raw ingest topic") {
		t.Fatalf("raw ingest replay target must fail closed, got %v", err)
	}
	mismatch := NewKafkaFlowReplayPublisher(producer, "flow.projection-replay.v1", func(context.Context, string) error { return nil })
	if err := mismatch.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match producer topic") {
		t.Fatalf("publisher and producer topic mismatch must fail closed, got %v", err)
	}
}
