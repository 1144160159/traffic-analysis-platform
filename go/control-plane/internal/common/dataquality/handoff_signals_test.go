package dataquality

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

type fakeKafkaOffsetReader struct {
	snapshot KafkaLagSnapshot
	err      error
}

func (f fakeKafkaOffsetReader) ReadLag(context.Context, string, string) (KafkaLagSnapshot, error) {
	return f.snapshot, f.err
}

type fakeFlinkWatermarkReader struct {
	snapshot FlinkWatermarkSnapshot
	err      error
}

func (f fakeFlinkWatermarkReader) ReadWatermark(context.Context, string, string, string) (FlinkWatermarkSnapshot, error) {
	return f.snapshot, f.err
}

type fakeSinkCommitReader struct {
	value    string
	observed time.Time
	err      error
}

func (f fakeSinkCommitReader) ReadSinkCommit(context.Context, string, string) (string, time.Time, error) {
	return f.value, f.observed, f.err
}

func testFlowSignalCollector(t *testing.T, kafkaReader KafkaOffsetReader, flinkReader FlinkWatermarkReader, sinkReader SinkCommitReader) *CompositeSignalCollector {
	t.Helper()
	contract := DefaultFlowDatasetContract("flow.events.v1", "flink-session-job", "Session Aggregation Job V2", "Assign FlowEvent Watermarks")
	kafkaDefinition, _ := SignalDefinitionFor(contract, SignalKindKafkaOffset)
	flinkDefinition, _ := SignalDefinitionFor(contract, SignalKindFlinkWatermark)
	sinkDefinition, _ := SignalDefinitionFor(contract, SignalKindSinkCommit)
	businessDefinition, _ := SignalDefinitionFor(contract, SignalKindBusinessVersion)
	objectDefinition, _ := SignalDefinitionFor(contract, SignalKindObjectManifest)
	collector, err := NewCompositeSignalCollector(contract,
		NewKafkaOffsetCollector(kafkaDefinition, "flow.events.v1", "flink-session-job", kafkaReader),
		NewFlinkWatermarkCollector(flinkDefinition, "Session Aggregation Job V2", "Assign FlowEvent Watermarks", "currentOutputWatermark", flinkReader),
		NewSinkCommitCollector(sinkDefinition, sinkReader),
		NewNotApplicableSignalCollector(businessDefinition),
		NewNotApplicableSignalCollector(objectDefinition),
	)
	if err != nil {
		t.Fatalf("NewCompositeSignalCollector: %v", err)
	}
	return collector
}

func TestCompositeSignalCollectorKeepsMeasuredUnknownAndNotApplicableDistinct(t *testing.T) {
	collected := time.Date(2026, 8, 4, 4, 30, 0, 0, time.UTC)
	watermark := collected.Add(-2 * time.Second).UnixMilli()
	sinkObserved := collected.Add(-time.Second)
	collector := testFlowSignalCollector(t,
		fakeKafkaOffsetReader{snapshot: KafkaLagSnapshot{TotalLag: 7, TotalEndOffset: 107, TotalCommittedOffset: 100, Partitions: []KafkaPartitionLag{{Partition: 0, FirstOffset: 0, EndOffset: 107, CommittedOffset: 100, Lag: 7}}}},
		fakeFlinkWatermarkReader{snapshot: FlinkWatermarkSnapshot{JobID: "job-1", VertexID: "vertex-1", Watermark: watermark, SubtaskMetrics: []FlinkSubtaskWatermark{{MetricID: "0.currentOutputWatermark", Value: watermark}}}},
		fakeSinkCommitReader{value: fmt.Sprint(sinkObserved.UnixMilli()), observed: sinkObserved},
	)
	signals := collector.Collect(context.Background(), SignalCollectionRequest{TenantID: "tenant-a", TraceID: "trace-a", CollectedAt: collected})
	if len(signals) != 5 {
		t.Fatalf("signals=%d want=5", len(signals))
	}
	byKind := make(map[string]HandoffSignal, len(signals))
	for _, signal := range signals {
		byKind[signal.SourceKind] = signal
		if err := validateHandoffSignal(signal); err != nil {
			t.Fatalf("validate %s: %v", signal.SourceKind, err)
		}
	}
	if got := *byKind[SignalKindKafkaOffset].WatermarkValue; got != "7" {
		t.Fatalf("Kafka lag=%s want=7", got)
	}
	if got := *byKind[SignalKindFlinkWatermark].WatermarkValue; got != fmt.Sprint(watermark) {
		t.Fatalf("Flink watermark=%s want=%d", got, watermark)
	}
	if byKind[SignalKindBusinessVersion].MeasurementState != SignalStatusNotApplicable || byKind[SignalKindObjectManifest].MeasurementState != SignalStatusNotApplicable {
		t.Fatalf("non-applicable states were not preserved: %#v", byKind)
	}
}

func TestCompositeSignalCollectorIsolatesSourceFailureWithoutFabricatingValue(t *testing.T) {
	collected := time.Now().UTC()
	collector := testFlowSignalCollector(t,
		fakeKafkaOffsetReader{err: errors.New("broker unavailable")},
		fakeFlinkWatermarkReader{snapshot: FlinkWatermarkSnapshot{JobID: "job-1", VertexID: "vertex-1", Watermark: collected.UnixMilli()}},
		fakeSinkCommitReader{value: fmt.Sprint(collected.UnixMilli()), observed: collected},
	)
	signals := collector.Collect(context.Background(), SignalCollectionRequest{TenantID: "tenant-a", TraceID: "trace-a", CollectedAt: collected})
	if signals[0].MeasurementState != SignalStatusError || signals[0].WatermarkValue != nil || !strings.Contains(signals[0].MeasurementError, "broker unavailable") {
		t.Fatalf("Kafka failure was not persisted as value-less error: %#v", signals[0])
	}
	if signals[0].AvailabilityState != SignalAvailabilityUnavailable || signals[0].ValueState != SignalValueNone {
		t.Fatalf("Kafka source failure must be typed unavailable without a value: %#v", signals[0])
	}
	if signals[1].MeasurementState != SignalStatusMeasured || signals[2].MeasurementState != SignalStatusMeasured {
		t.Fatalf("independent collectors should remain measured: %#v", signals)
	}
}

func TestKafkaOffsetSemanticsSeparateZeroNotArrivedAndPartial(t *testing.T) {
	collected := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		snapshot     KafkaLagSnapshot
		measurement  string
		availability string
		completeness string
		valueState   string
	}{
		{
			name: "real zero",
			snapshot: KafkaLagSnapshot{TotalLag: 0, Partitions: []KafkaPartitionLag{
				{Partition: 0, FirstOffset: 0, EndOffset: 12, CommittedOffset: 12, Lag: 0},
			}},
			measurement: SignalStatusMeasured, availability: SignalAvailabilityArrived,
			completeness: SignalCompletenessComplete, valueState: SignalValueZero,
		},
		{
			name: "never arrived",
			snapshot: KafkaLagSnapshot{Partitions: []KafkaPartitionLag{
				{Partition: 0, FirstOffset: 0, EndOffset: 12, CommittedOffset: -1, Lag: 12},
			}},
			measurement: SignalStatusUnknown, availability: SignalAvailabilityNotArrived,
			completeness: SignalCompletenessUnknown, valueState: SignalValueNone,
		},
		{
			name: "partial",
			snapshot: KafkaLagSnapshot{TotalLag: 4, Partitions: []KafkaPartitionLag{
				{Partition: 0, FirstOffset: 0, EndOffset: 12, CommittedOffset: 12, Lag: 0},
				{Partition: 1, FirstOffset: 0, EndOffset: 4, CommittedOffset: -1, Lag: 4},
			}},
			measurement: SignalStatusMeasured, availability: SignalAvailabilityArrived,
			completeness: SignalCompletenessPartial, valueState: SignalValueNonzero,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			collector := testFlowSignalCollector(t,
				fakeKafkaOffsetReader{snapshot: tc.snapshot},
				fakeFlinkWatermarkReader{snapshot: FlinkWatermarkSnapshot{JobID: "job", VertexID: "vertex", Watermark: collected.UnixMilli()}},
				fakeSinkCommitReader{value: fmt.Sprint(collected.UnixMilli()), observed: collected},
			)
			signal := collector.Collect(context.Background(), SignalCollectionRequest{TenantID: "tenant-a", TraceID: "trace-a", CollectedAt: collected})[0]
			if signal.MeasurementState != tc.measurement || signal.AvailabilityState != tc.availability || signal.CompletenessState != tc.completeness || signal.ValueState != tc.valueState {
				t.Fatalf("unexpected typed Kafka signal: %#v", signal)
			}
			if err := validateHandoffSignal(signal); err != nil {
				t.Fatalf("validate typed Kafka signal: %v", err)
			}
		})
	}
}

func TestFlinkRESTWatermarkReaderUsesMinimumFiniteSubtaskValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/jobs/overview":
			_, _ = w.Write([]byte(`{"jobs":[{"jid":"job-1","name":"Session Aggregation Job V2","state":"RUNNING"}]}`))
		case r.URL.Path == "/jobs/job-1":
			_, _ = w.Write([]byte(`{"vertices":[{"id":"vertex-1","name":"Assign FlowEvent Watermarks -> Sink","status":"RUNNING"}]}`))
		case r.URL.Path == "/jobs/job-1/vertices/vertex-1/metrics" && r.URL.Query().Get("get") == "":
			_, _ = w.Write([]byte(`[{"id":"0.Assign.currentOutputWatermark"},{"id":"1.Assign.currentOutputWatermark"},{"id":"0.other"}]`))
		case r.URL.Path == "/jobs/job-1/vertices/vertex-1/metrics":
			_, _ = w.Write([]byte(`[{"id":"0.Assign.currentOutputWatermark","value":"1785810005000"},{"id":"1.Assign.currentOutputWatermark","value":"1785810004000"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	reader, err := NewFlinkRESTWatermarkReader(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reader.ReadWatermark(context.Background(), "Session Aggregation Job V2", "Assign FlowEvent Watermarks", "currentOutputWatermark")
	if err != nil {
		t.Fatalf("ReadWatermark: %v", err)
	}
	if snapshot.Watermark != 1785810004000 || len(snapshot.SubtaskMetrics) != 2 {
		t.Fatalf("unexpected Flink snapshot: %#v", snapshot)
	}
}

func TestClickHouseSinkCommitReaderDoesNotTreatEmptyDatasetAsMeasured(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader := NewClickHouseSinkCommitReader(db)
	mock.ExpectQuery(`SELECT if\(count\(\)=0, '', toString\(max\(ingest_ts\)\)\)`).WithArgs("tenant-a").WillReturnRows(sqlmock.NewRows([]string{"commit"}).AddRow(""))
	value, observed, err := reader.ReadSinkCommit(context.Background(), "tenant-a", "flows_raw")
	if err != nil || value != "" || !observed.IsZero() {
		t.Fatalf("empty dataset result value=%q observed=%s err=%v", value, observed, err)
	}
}

func TestPersistHandoffSignalsCommitsAllFiveStatesAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(db)
	now := time.Now().UTC()
	collector := testFlowSignalCollector(t,
		fakeKafkaOffsetReader{snapshot: KafkaLagSnapshot{TotalLag: 0, Partitions: []KafkaPartitionLag{{Partition: 0}}}},
		fakeFlinkWatermarkReader{snapshot: FlinkWatermarkSnapshot{JobID: "job", VertexID: "vertex", Watermark: now.UnixMilli()}},
		fakeSinkCommitReader{value: fmt.Sprint(now.UnixMilli()), observed: now},
	)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO data_quality_datasets`).WithArgs(
		"tenant-a", "flows_raw", "Raw flow facts", "data-reliability", int64(1), "data-quality-dataset-signals-v1",
		sqlmock.AnyArg(), int64(60), int64(2592000), sqlmock.AnyArg(), sqlmock.AnyArg(), 0.999, "trace-a",
	).WillReturnResult(sqlmock.NewResult(0, 1))
	for range 5 {
		mock.ExpectExec(`INSERT INTO data_quality_watermarks`).WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	if _, err := monitor.CollectAndPersistHandoffSignals(context.Background(), "tenant-a", "trace-a", collector); err != nil {
		t.Fatalf("CollectAndPersistHandoffSignals: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistHandoffSignalsRollsBackOnAnyWatermarkFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	monitor := NewMonitor(nil, MonitorConfig{}, zap.NewNop())
	monitor.SetControlDB(db)
	now := time.Now().UTC()
	collector := testFlowSignalCollector(t,
		fakeKafkaOffsetReader{snapshot: KafkaLagSnapshot{}},
		fakeFlinkWatermarkReader{snapshot: FlinkWatermarkSnapshot{JobID: "job", VertexID: "vertex", Watermark: now.UnixMilli()}},
		fakeSinkCommitReader{value: fmt.Sprint(now.UnixMilli()), observed: now},
	)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO data_quality_datasets`).WithArgs(
		"tenant-a", "flows_raw", "Raw flow facts", "data-reliability", int64(1), "data-quality-dataset-signals-v1",
		sqlmock.AnyArg(), int64(60), int64(2592000), sqlmock.AnyArg(), sqlmock.AnyArg(), 0.999, "trace-a",
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO data_quality_watermarks`).WillReturnError(errors.New("watermark storage unavailable"))
	mock.ExpectRollback()
	if _, err := monitor.CollectAndPersistHandoffSignals(context.Background(), "tenant-a", "trace-a", collector); err == nil || !strings.Contains(err.Error(), "upsert kafka_offset") {
		t.Fatalf("expected atomic rollback error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyHandoffSignalsReadsFiveStatesAndEvaluatesOnlyRequiredSignals(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	monitor := NewMonitor(nil, MonitorConfig{MaxKafkaLag: 100, MaxSignalAge: 5 * time.Minute}, zap.NewNop())
	monitor.SetControlDB(db)
	rows := sqlmock.NewRows([]string{
		"tenant_id", "dataset_id", "source_kind", "source_id", "partition_id", "measurement_status",
		"watermark_value", "observed_at", "collected_at", "trace_id", "measurement_error", "metadata",
	}).
		AddRow("tenant-a", "flows_raw", SignalKindBusinessVersion, "flows_raw.immutable_event", "", SignalStatusNotApplicable, nil, nil, now, "trace-a", "", []byte(`{"required":false}`)).
		AddRow("tenant-a", "flows_raw", SignalKindFlinkWatermark, "job/vertex", "", SignalStatusMeasured, fmt.Sprint(now.Add(-time.Second).UnixMilli()), now.Add(-time.Second), now, "trace-a", "", []byte(`{"required":true}`)).
		AddRow("tenant-a", "flows_raw", SignalKindKafkaOffset, "topic/group", "", SignalStatusMeasured, "7", now, now, "trace-a", "", []byte(`{"required":true}`)).
		AddRow("tenant-a", "flows_raw", SignalKindObjectManifest, "flows_raw.no_object_payload", "", SignalStatusNotApplicable, nil, nil, now, "trace-a", "", []byte(`{"required":false}`)).
		AddRow("tenant-a", "flows_raw", SignalKindSinkCommit, "clickhouse.max_ingest_ts", "", SignalStatusMeasured, fmt.Sprint(now.Add(-500*time.Millisecond).UnixMilli()), now.Add(-500*time.Millisecond), now, "trace-a", "", []byte(`{"required":true}`))
	mock.ExpectQuery(`FROM data_quality_watermarks`).WithArgs("tenant-a", "flows_raw").WillReturnRows(rows)
	report := &DataQualityReport{Timestamp: now, Metrics: map[string]float64{}, SourceWatermarks: map[string]interface{}{}}
	monitor.applyHandoffSignals(context.Background(), "tenant-a", report)
	if len(report.Checks) != 3 {
		t.Fatalf("required hand-off checks=%d want=3: %#v", len(report.Checks), report.Checks)
	}
	for _, check := range report.Checks {
		if !check.Measured || check.Status != "pass" {
			t.Fatalf("expected fresh required signal to pass: %#v", check)
		}
	}
	if len(report.SourceWatermarks) != 5 {
		t.Fatalf("source watermarks=%d want=5", len(report.SourceWatermarks))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyHandoffSignalsNeverPassesPartialOrStaleAndNeverDowngradesFailure(t *testing.T) {
	now := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		lag        string
		collected  time.Time
		metadata   string
		wantStatus string
		wantFresh  string
		wantComp   string
		wantValue  string
	}{
		{name: "real zero passes", lag: "0", collected: now, metadata: `{}`, wantStatus: "pass", wantFresh: SignalFreshnessFresh, wantComp: SignalCompletenessComplete, wantValue: SignalValueZero},
		{name: "partial warns", lag: "7", collected: now, metadata: `{"completeness_status":"partial"}`, wantStatus: "warn", wantFresh: SignalFreshnessFresh, wantComp: SignalCompletenessPartial, wantValue: SignalValueNonzero},
		{name: "stale warns", lag: "7", collected: now.Add(-10 * time.Minute), metadata: `{}`, wantStatus: "warn", wantFresh: SignalFreshnessStale, wantComp: SignalCompletenessComplete, wantValue: SignalValueNonzero},
		{name: "stale partial high lag remains fail", lag: "101", collected: now.Add(-10 * time.Minute), metadata: `{"completeness_status":"partial"}`, wantStatus: "fail", wantFresh: SignalFreshnessStale, wantComp: SignalCompletenessPartial, wantValue: SignalValueNonzero},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			monitor := NewMonitor(nil, MonitorConfig{MaxKafkaLag: 100, MaxSignalAge: 5 * time.Minute}, zap.NewNop())
			monitor.SetControlDB(db)
			rows := sqlmock.NewRows([]string{
				"tenant_id", "dataset_id", "source_kind", "source_id", "partition_id", "measurement_status",
				"watermark_value", "observed_at", "collected_at", "trace_id", "measurement_error", "metadata",
			}).AddRow("tenant-a", "flows_raw", SignalKindKafkaOffset, "topic/group", "", SignalStatusMeasured, tc.lag, tc.collected, tc.collected, "trace-a", "", []byte(tc.metadata))
			mock.ExpectQuery(`FROM data_quality_watermarks`).WithArgs("tenant-a", "flows_raw").WillReturnRows(rows)
			report := &DataQualityReport{Timestamp: now, Metrics: map[string]float64{}, SourceWatermarks: map[string]interface{}{}}
			monitor.applyHandoffSignals(context.Background(), "tenant-a", report)
			var check *QualityCheck
			for index := range report.Checks {
				if report.Checks[index].Name == "kafka_consumer_lag" {
					check = &report.Checks[index]
					break
				}
			}
			if check == nil || check.Status != tc.wantStatus || check.Freshness != tc.wantFresh || check.Completeness != tc.wantComp || check.ValueState != tc.wantValue || check.Availability != SignalAvailabilityArrived || !check.Measured {
				t.Fatalf("unexpected Kafka quality check: %#v", check)
			}
			watermark, ok := report.SourceWatermarks[SignalKindKafkaOffset].(HandoffSignal)
			if !ok || watermark.FreshnessState != tc.wantFresh {
				t.Fatalf("source watermark did not retain derived freshness: %#v", report.SourceWatermarks)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
