package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

const (
	SignalStatusMeasured      = "measured"
	SignalStatusUnknown       = "unknown"
	SignalStatusNotApplicable = "not_applicable"
	SignalStatusError         = "error"

	SignalKindKafkaOffset     = "kafka_offset"
	SignalKindFlinkWatermark  = "flink_watermark"
	SignalKindSinkCommit      = "sink_commit"
	SignalKindBusinessVersion = "business_version"
	SignalKindObjectManifest  = "object_manifest"
)

// DatasetSignalContract is the executable subset of the versioned dataset
// hand-off contract. It deliberately states applicability per dataset so an
// absent aggregate revision or object manifest can never be reported as zero.
type DatasetSignalContract struct {
	ContractVersion       string
	DatasetID             string
	DisplayName           string
	Owner                 string
	SchemaVersion         int64
	BusinessKeys          []string
	AllowedLatenessSecond int64
	RetentionSeconds      int64
	Upstreams             []string
	Downstreams           []string
	SLOTarget             float64
	Signals               []SignalDefinition
}

type SignalDefinition struct {
	Kind          string
	SourceID      string
	Required      bool
	Unit          string
	Applicability string
	Description   string
	Metadata      map[string]interface{}
}

type HandoffSignal struct {
	TenantID         string                 `json:"tenant_id"`
	DatasetID        string                 `json:"dataset_id"`
	SourceKind       string                 `json:"source_kind"`
	SourceID         string                 `json:"source_id"`
	PartitionID      string                 `json:"partition_id"`
	MeasurementState string                 `json:"measurement_status"`
	WatermarkValue   *string                `json:"watermark_value,omitempty"`
	ObservedAt       *time.Time             `json:"observed_at,omitempty"`
	CollectedAt      time.Time              `json:"collected_at"`
	TraceID          string                 `json:"trace_id"`
	MeasurementError string                 `json:"measurement_error,omitempty"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type SignalCollectionRequest struct {
	TenantID    string
	DatasetID   string
	TraceID     string
	CollectedAt time.Time
}

type SignalCollector interface {
	Definition() SignalDefinition
	Collect(context.Context, SignalCollectionRequest) (*HandoffSignal, error)
}

type CompositeSignalCollector struct {
	contract   DatasetSignalContract
	collectors []SignalCollector
}

func NewCompositeSignalCollector(contract DatasetSignalContract, collectors ...SignalCollector) (*CompositeSignalCollector, error) {
	if err := validateDatasetSignalContract(contract); err != nil {
		return nil, err
	}
	byKind := make(map[string]SignalCollector, len(collectors))
	for _, collector := range collectors {
		if collector == nil {
			return nil, fmt.Errorf("nil signal collector")
		}
		kind := collector.Definition().Kind
		if _, exists := byKind[kind]; exists {
			return nil, fmt.Errorf("duplicate collector for signal kind %s", kind)
		}
		byKind[kind] = collector
	}
	ordered := make([]SignalCollector, 0, len(contract.Signals))
	for _, definition := range contract.Signals {
		collector, exists := byKind[definition.Kind]
		if !exists {
			return nil, fmt.Errorf("missing collector for signal kind %s", definition.Kind)
		}
		if collector.Definition().SourceID != definition.SourceID {
			return nil, fmt.Errorf("collector source_id %s does not match contract %s", collector.Definition().SourceID, definition.SourceID)
		}
		ordered = append(ordered, collector)
	}
	return &CompositeSignalCollector{contract: contract, collectors: ordered}, nil
}

func (c *CompositeSignalCollector) Contract() DatasetSignalContract { return c.contract }

func (c *CompositeSignalCollector) Collect(ctx context.Context, request SignalCollectionRequest) []HandoffSignal {
	if request.CollectedAt.IsZero() {
		request.CollectedAt = time.Now().UTC()
	}
	request.DatasetID = c.contract.DatasetID
	result := make([]HandoffSignal, 0, len(c.collectors))
	for _, collector := range c.collectors {
		definition := collector.Definition()
		signal, err := collector.Collect(ctx, request)
		if err != nil {
			result = append(result, HandoffSignal{
				TenantID: request.TenantID, DatasetID: request.DatasetID,
				SourceKind: definition.Kind, SourceID: definition.SourceID,
				MeasurementState: SignalStatusError, CollectedAt: request.CollectedAt,
				TraceID: request.TraceID, MeasurementError: truncateSignalError(err.Error()),
				Metadata: mergeSignalMetadata(definition.Metadata, map[string]interface{}{
					"required": definition.Required, "unit": definition.Unit,
				}),
			})
			continue
		}
		if signal == nil {
			result = append(result, HandoffSignal{
				TenantID: request.TenantID, DatasetID: request.DatasetID,
				SourceKind: definition.Kind, SourceID: definition.SourceID,
				MeasurementState: SignalStatusUnknown, CollectedAt: request.CollectedAt,
				TraceID: request.TraceID, Metadata: mergeSignalMetadata(definition.Metadata, map[string]interface{}{
					"required": definition.Required, "unit": definition.Unit, "reason": "collector returned no measurement",
				}),
			})
			continue
		}
		signal.TenantID = request.TenantID
		signal.DatasetID = request.DatasetID
		signal.SourceKind = definition.Kind
		signal.SourceID = definition.SourceID
		signal.TraceID = request.TraceID
		signal.CollectedAt = request.CollectedAt
		signal.Metadata = mergeSignalMetadata(definition.Metadata, signal.Metadata)
		signal.Metadata["required"] = definition.Required
		signal.Metadata["unit"] = definition.Unit
		result = append(result, *signal)
	}
	return result
}

func DefaultFlowDatasetContract(kafkaTopic, kafkaGroup, flinkJobName, flinkVertex string) DatasetSignalContract {
	kafkaSourceID := kafkaTopic + "/" + kafkaGroup
	return DatasetSignalContract{
		ContractVersion: "data-quality-dataset-signals-v1", DatasetID: "flows_raw",
		DisplayName: "Raw flow facts", Owner: "data-reliability", SchemaVersion: 1,
		BusinessKeys: []string{"event_id"}, AllowedLatenessSecond: 60, RetentionSeconds: 2592000,
		Upstreams: []string{kafkaTopic}, Downstreams: []string{flinkJobName, "traffic.sessions"}, SLOTarget: 0.999,
		Signals: []SignalDefinition{
			{Kind: SignalKindKafkaOffset, SourceID: kafkaSourceID, Required: true, Unit: "records", Applicability: "required", Description: "broker end offset minus committed consumer-group offset", Metadata: map[string]interface{}{"topic": kafkaTopic, "consumer_group": kafkaGroup, "scope": "shared_pipeline"}},
			{Kind: SignalKindFlinkWatermark, SourceID: flinkJobName + "/" + flinkVertex, Required: true, Unit: "unix_ms", Applicability: "required", Description: "minimum finite currentOutputWatermark across active source subtasks", Metadata: map[string]interface{}{"job_name": flinkJobName, "vertex_contains": flinkVertex, "scope": "shared_pipeline"}},
			{Kind: SignalKindSinkCommit, SourceID: "clickhouse.traffic.flows_raw.max_ingest_ts", Required: true, Unit: "unix_ms", Applicability: "required", Description: "latest tenant-scoped ClickHouse ingest commit"},
			{Kind: SignalKindBusinessVersion, SourceID: "flows_raw.immutable_event", Required: false, Unit: "none", Applicability: SignalStatusNotApplicable, Description: "immutable flow facts do not have an aggregate revision"},
			{Kind: SignalKindObjectManifest, SourceID: "flows_raw.no_object_payload", Required: false, Unit: "none", Applicability: SignalStatusNotApplicable, Description: "flow facts are not object-backed artifacts"},
		},
	}
}

func validateDatasetSignalContract(contract DatasetSignalContract) error {
	if contract.ContractVersion == "" || contract.DatasetID == "" || contract.DisplayName == "" || contract.Owner == "" || contract.SchemaVersion <= 0 {
		return fmt.Errorf("dataset signal contract identity fields are required")
	}
	if len(contract.Signals) != 5 {
		return fmt.Errorf("dataset %s must declare all five hand-off signal kinds", contract.DatasetID)
	}
	allowed := map[string]bool{SignalKindKafkaOffset: true, SignalKindFlinkWatermark: true, SignalKindSinkCommit: true, SignalKindBusinessVersion: true, SignalKindObjectManifest: true}
	seen := make(map[string]bool, len(contract.Signals))
	for _, signal := range contract.Signals {
		if !allowed[signal.Kind] || seen[signal.Kind] || signal.SourceID == "" || signal.Unit == "" {
			return fmt.Errorf("invalid or duplicate signal definition %q", signal.Kind)
		}
		if signal.Applicability != "required" && signal.Applicability != SignalStatusNotApplicable {
			return fmt.Errorf("signal %s has invalid applicability %q", signal.Kind, signal.Applicability)
		}
		if signal.Required != (signal.Applicability == "required") {
			return fmt.Errorf("signal %s required/applicability mismatch", signal.Kind)
		}
		seen[signal.Kind] = true
	}
	return nil
}

type KafkaPartitionLag struct {
	Partition       int   `json:"partition"`
	FirstOffset     int64 `json:"first_offset"`
	EndOffset       int64 `json:"end_offset"`
	CommittedOffset int64 `json:"committed_offset"`
	Lag             int64 `json:"lag"`
}

type KafkaLagSnapshot struct {
	TotalLag             int64
	TotalEndOffset       int64
	TotalCommittedOffset int64
	Partitions           []KafkaPartitionLag
}

type KafkaOffsetReader interface {
	ReadLag(context.Context, string, string) (KafkaLagSnapshot, error)
}

type KafkaBrokerOffsetReader struct {
	client    *kafkago.Client
	transport *kafkago.Transport
}

func NewKafkaBrokerOffsetReader(brokers []string, security commonkafka.SecurityConfig, timeout time.Duration) (*KafkaBrokerOffsetReader, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers are required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	transport, err := security.Transport("data-quality-offset-reader")
	if err != nil {
		return nil, err
	}
	return &KafkaBrokerOffsetReader{
		client:    &kafkago.Client{Addr: kafkago.TCP(brokers...), Timeout: timeout, Transport: transport},
		transport: transport,
	}, nil
}

func (r *KafkaBrokerOffsetReader) Close() {
	if r != nil && r.transport != nil {
		r.transport.CloseIdleConnections()
	}
}

func (r *KafkaBrokerOffsetReader) ReadLag(ctx context.Context, topic, group string) (KafkaLagSnapshot, error) {
	if r == nil || r.client == nil || topic == "" || group == "" {
		return KafkaLagSnapshot{}, fmt.Errorf("Kafka offset reader, topic and consumer group are required")
	}
	metadata, err := r.client.Metadata(ctx, &kafkago.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return KafkaLagSnapshot{}, fmt.Errorf("read Kafka topic metadata: %w", err)
	}
	if len(metadata.Topics) != 1 || metadata.Topics[0].Name != topic {
		return KafkaLagSnapshot{}, fmt.Errorf("Kafka topic %s metadata is missing", topic)
	}
	if metadata.Topics[0].Error != nil {
		return KafkaLagSnapshot{}, fmt.Errorf("Kafka topic %s metadata: %w", topic, metadata.Topics[0].Error)
	}
	partitionIDs := make([]int, 0, len(metadata.Topics[0].Partitions))
	requests := make([]kafkago.OffsetRequest, 0, len(metadata.Topics[0].Partitions)*2)
	for _, partition := range metadata.Topics[0].Partitions {
		if partition.Error != nil {
			return KafkaLagSnapshot{}, fmt.Errorf("Kafka topic %s partition %d metadata: %w", topic, partition.ID, partition.Error)
		}
		partitionIDs = append(partitionIDs, partition.ID)
		requests = append(requests, kafkago.FirstOffsetOf(partition.ID), kafkago.LastOffsetOf(partition.ID))
	}
	if len(partitionIDs) == 0 {
		return KafkaLagSnapshot{}, fmt.Errorf("Kafka topic %s has no partitions", topic)
	}
	committedResponse, err := r.client.OffsetFetch(ctx, &kafkago.OffsetFetchRequest{GroupID: group, Topics: map[string][]int{topic: partitionIDs}})
	if err != nil {
		return KafkaLagSnapshot{}, fmt.Errorf("read Kafka committed offsets: %w", err)
	}
	if committedResponse.Error != nil {
		return KafkaLagSnapshot{}, fmt.Errorf("read Kafka consumer group %s: %w", group, committedResponse.Error)
	}
	listResponse, err := r.client.ListOffsets(ctx, &kafkago.ListOffsetsRequest{Topics: map[string][]kafkago.OffsetRequest{topic: requests}})
	if err != nil {
		return KafkaLagSnapshot{}, fmt.Errorf("read Kafka end offsets: %w", err)
	}
	committedByPartition := make(map[int]int64, len(partitionIDs))
	for _, partition := range committedResponse.Topics[topic] {
		if partition.Error != nil {
			return KafkaLagSnapshot{}, fmt.Errorf("read Kafka committed offset partition %d: %w", partition.Partition, partition.Error)
		}
		committedByPartition[partition.Partition] = partition.CommittedOffset
	}
	offsetsByPartition := make(map[int]kafkago.PartitionOffsets, len(partitionIDs))
	for _, partition := range listResponse.Topics[topic] {
		if partition.Error != nil {
			return KafkaLagSnapshot{}, fmt.Errorf("read Kafka end offset partition %d: %w", partition.Partition, partition.Error)
		}
		offsetsByPartition[partition.Partition] = partition
	}
	snapshot := KafkaLagSnapshot{Partitions: make([]KafkaPartitionLag, 0, len(partitionIDs))}
	sort.Ints(partitionIDs)
	for _, partitionID := range partitionIDs {
		offsets, exists := offsetsByPartition[partitionID]
		if !exists || offsets.FirstOffset < 0 || offsets.LastOffset < 0 {
			return KafkaLagSnapshot{}, fmt.Errorf("Kafka offsets incomplete for partition %d", partitionID)
		}
		committed, exists := committedByPartition[partitionID]
		if !exists {
			return KafkaLagSnapshot{}, fmt.Errorf("Kafka committed offset missing for partition %d", partitionID)
		}
		lagBase := committed
		if lagBase < offsets.FirstOffset {
			lagBase = offsets.FirstOffset
		}
		lag := offsets.LastOffset - lagBase
		if lag < 0 {
			lag = 0
		}
		if snapshot.TotalLag > math.MaxInt64-lag || snapshot.TotalEndOffset > math.MaxInt64-offsets.LastOffset || snapshot.TotalCommittedOffset > math.MaxInt64-lagBase {
			return KafkaLagSnapshot{}, fmt.Errorf("Kafka offset totals overflow int64")
		}
		snapshot.TotalLag += lag
		snapshot.TotalEndOffset += offsets.LastOffset
		snapshot.TotalCommittedOffset += lagBase
		snapshot.Partitions = append(snapshot.Partitions, KafkaPartitionLag{Partition: partitionID, FirstOffset: offsets.FirstOffset, EndOffset: offsets.LastOffset, CommittedOffset: committed, Lag: lag})
	}
	return snapshot, nil
}

type KafkaOffsetCollector struct {
	definition SignalDefinition
	topic      string
	group      string
	reader     KafkaOffsetReader
}

func NewKafkaOffsetCollector(definition SignalDefinition, topic, group string, reader KafkaOffsetReader) *KafkaOffsetCollector {
	return &KafkaOffsetCollector{definition: definition, topic: topic, group: group, reader: reader}
}

func (c *KafkaOffsetCollector) Definition() SignalDefinition { return c.definition }

func (c *KafkaOffsetCollector) Collect(ctx context.Context, request SignalCollectionRequest) (*HandoffSignal, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("Kafka offset reader is not configured")
	}
	snapshot, err := c.reader.ReadLag(ctx, c.topic, c.group)
	if err != nil {
		return nil, err
	}
	value := strconv.FormatInt(snapshot.TotalLag, 10)
	observed := request.CollectedAt
	return &HandoffSignal{MeasurementState: SignalStatusMeasured, WatermarkValue: &value, ObservedAt: &observed, Metadata: map[string]interface{}{
		"end_offset_total": snapshot.TotalEndOffset, "committed_offset_total": snapshot.TotalCommittedOffset,
		"partition_count": len(snapshot.Partitions), "partitions": snapshot.Partitions,
	}}, nil
}

type FlinkSubtaskWatermark struct {
	MetricID string `json:"metric_id"`
	Value    int64  `json:"value"`
}

type FlinkWatermarkSnapshot struct {
	JobID          string
	VertexID       string
	Watermark      int64
	SubtaskMetrics []FlinkSubtaskWatermark
}

type FlinkWatermarkReader interface {
	ReadWatermark(context.Context, string, string, string) (FlinkWatermarkSnapshot, error)
}

type FlinkRESTWatermarkReader struct {
	baseURL string
	client  *http.Client
}

func NewFlinkRESTWatermarkReader(baseURL string, timeout time.Duration) (*FlinkRESTWatermarkReader, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid Flink REST URL")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &FlinkRESTWatermarkReader{baseURL: parsed.String(), client: &http.Client{Timeout: timeout, Transport: transport}}, nil
}

func (r *FlinkRESTWatermarkReader) ReadWatermark(ctx context.Context, jobName, vertexContains, metricContains string) (FlinkWatermarkSnapshot, error) {
	if r == nil || r.client == nil || jobName == "" || vertexContains == "" || metricContains == "" {
		return FlinkWatermarkSnapshot{}, fmt.Errorf("Flink reader and stable job/vertex/metric names are required")
	}
	var jobs struct {
		Jobs []struct {
			ID    string `json:"jid"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"jobs"`
	}
	if err := r.getJSON(ctx, "/jobs/overview", &jobs); err != nil {
		return FlinkWatermarkSnapshot{}, err
	}
	jobID := ""
	for _, job := range jobs.Jobs {
		if job.Name == jobName && job.State == "RUNNING" {
			if jobID != "" {
				return FlinkWatermarkSnapshot{}, fmt.Errorf("multiple RUNNING Flink jobs named %s", jobName)
			}
			jobID = job.ID
		}
	}
	if jobID == "" {
		return FlinkWatermarkSnapshot{}, fmt.Errorf("RUNNING Flink job %s not found", jobName)
	}
	var detail struct {
		Vertices []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"vertices"`
	}
	if err := r.getJSON(ctx, "/jobs/"+url.PathEscape(jobID), &detail); err != nil {
		return FlinkWatermarkSnapshot{}, err
	}
	vertexID := ""
	for _, vertex := range detail.Vertices {
		if strings.Contains(vertex.Name, vertexContains) && vertex.Status == "RUNNING" {
			if vertexID != "" {
				return FlinkWatermarkSnapshot{}, fmt.Errorf("multiple RUNNING vertices contain %s", vertexContains)
			}
			vertexID = vertex.ID
		}
	}
	if vertexID == "" {
		return FlinkWatermarkSnapshot{}, fmt.Errorf("RUNNING Flink vertex containing %s not found", vertexContains)
	}
	metricsPath := "/jobs/" + url.PathEscape(jobID) + "/vertices/" + url.PathEscape(vertexID) + "/metrics"
	var catalog []struct {
		ID string `json:"id"`
	}
	if err := r.getJSON(ctx, metricsPath, &catalog); err != nil {
		return FlinkWatermarkSnapshot{}, err
	}
	metricIDs := make([]string, 0)
	for _, metric := range catalog {
		if strings.Contains(metric.ID, metricContains) {
			metricIDs = append(metricIDs, metric.ID)
		}
	}
	if len(metricIDs) == 0 {
		return FlinkWatermarkSnapshot{}, fmt.Errorf("Flink metric containing %s not found", metricContains)
	}
	sort.Strings(metricIDs)
	query := url.Values{"get": []string{strings.Join(metricIDs, ",")}}
	var values []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	}
	if err := r.getJSON(ctx, metricsPath+"?"+query.Encode(), &values); err != nil {
		return FlinkWatermarkSnapshot{}, err
	}
	snapshot := FlinkWatermarkSnapshot{JobID: jobID, VertexID: vertexID, Watermark: math.MaxInt64}
	for _, metric := range values {
		value, err := strconv.ParseInt(metric.Value, 10, 64)
		if err != nil || value == math.MinInt64 {
			continue
		}
		snapshot.SubtaskMetrics = append(snapshot.SubtaskMetrics, FlinkSubtaskWatermark{MetricID: metric.ID, Value: value})
		if value < snapshot.Watermark {
			snapshot.Watermark = value
		}
	}
	if len(snapshot.SubtaskMetrics) == 0 {
		return FlinkWatermarkSnapshot{}, fmt.Errorf("Flink watermark has no finite subtask values")
	}
	return snapshot, nil
}

func (r *FlinkRESTWatermarkReader) getJSON(ctx context.Context, path string, destination interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build Flink REST request: %w", err)
	}
	response, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("call Flink REST: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Flink REST returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return fmt.Errorf("decode Flink REST response: %w", err)
	}
	return nil
}

type FlinkWatermarkCollector struct {
	definition     SignalDefinition
	jobName        string
	vertexContains string
	metricContains string
	reader         FlinkWatermarkReader
}

func NewFlinkWatermarkCollector(definition SignalDefinition, jobName, vertexContains, metricContains string, reader FlinkWatermarkReader) *FlinkWatermarkCollector {
	return &FlinkWatermarkCollector{definition: definition, jobName: jobName, vertexContains: vertexContains, metricContains: metricContains, reader: reader}
}

func (c *FlinkWatermarkCollector) Definition() SignalDefinition { return c.definition }

func (c *FlinkWatermarkCollector) Collect(ctx context.Context, _ SignalCollectionRequest) (*HandoffSignal, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("Flink watermark reader is not configured")
	}
	snapshot, err := c.reader.ReadWatermark(ctx, c.jobName, c.vertexContains, c.metricContains)
	if err != nil {
		return nil, err
	}
	value := strconv.FormatInt(snapshot.Watermark, 10)
	observed := time.UnixMilli(snapshot.Watermark).UTC()
	return &HandoffSignal{MeasurementState: SignalStatusMeasured, WatermarkValue: &value, ObservedAt: &observed, Metadata: map[string]interface{}{
		"job_id": snapshot.JobID, "vertex_id": snapshot.VertexID, "subtasks": snapshot.SubtaskMetrics,
	}}, nil
}

type SinkCommitReader interface {
	ReadSinkCommit(context.Context, string, string) (string, time.Time, error)
}

type ClickHouseSinkCommitReader struct{ db *sql.DB }

func NewClickHouseSinkCommitReader(db *sql.DB) *ClickHouseSinkCommitReader {
	return &ClickHouseSinkCommitReader{db: db}
}

func (r *ClickHouseSinkCommitReader) ReadSinkCommit(ctx context.Context, tenantID, datasetID string) (string, time.Time, error) {
	if r == nil || r.db == nil {
		return "", time.Time{}, fmt.Errorf("ClickHouse connection is not configured")
	}
	if datasetID != "flows_raw" {
		return "", time.Time{}, fmt.Errorf("unsupported sink commit dataset %s", datasetID)
	}
	var value string
	if err := r.db.QueryRowContext(ctx, `
		SELECT if(count()=0, '', toString(max(ingest_ts)))
		FROM traffic.flows_raw WHERE tenant_id = ?
	`, tenantID).Scan(&value); err != nil {
		return "", time.Time{}, fmt.Errorf("read ClickHouse sink commit: %w", err)
	}
	if value == "" {
		return "", time.Time{}, nil
	}
	milliseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || milliseconds <= 0 {
		return "", time.Time{}, fmt.Errorf("invalid ClickHouse sink commit %q", value)
	}
	return value, time.UnixMilli(milliseconds).UTC(), nil
}

type SinkCommitCollector struct {
	definition SignalDefinition
	reader     SinkCommitReader
}

func NewSinkCommitCollector(definition SignalDefinition, reader SinkCommitReader) *SinkCommitCollector {
	return &SinkCommitCollector{definition: definition, reader: reader}
}

func (c *SinkCommitCollector) Definition() SignalDefinition { return c.definition }

func (c *SinkCommitCollector) Collect(ctx context.Context, request SignalCollectionRequest) (*HandoffSignal, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("sink commit reader is not configured")
	}
	value, observed, err := c.reader.ReadSinkCommit(ctx, request.TenantID, request.DatasetID)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return &HandoffSignal{MeasurementState: SignalStatusUnknown, Metadata: map[string]interface{}{"reason": "tenant dataset has no committed rows"}}, nil
	}
	return &HandoffSignal{MeasurementState: SignalStatusMeasured, WatermarkValue: &value, ObservedAt: &observed}, nil
}

type NotApplicableSignalCollector struct{ definition SignalDefinition }

func NewNotApplicableSignalCollector(definition SignalDefinition) *NotApplicableSignalCollector {
	return &NotApplicableSignalCollector{definition: definition}
}

func (c *NotApplicableSignalCollector) Definition() SignalDefinition { return c.definition }

func (c *NotApplicableSignalCollector) Collect(_ context.Context, _ SignalCollectionRequest) (*HandoffSignal, error) {
	if c.definition.Applicability != SignalStatusNotApplicable {
		return nil, errors.New("not-applicable collector used for a required signal")
	}
	return &HandoffSignal{MeasurementState: SignalStatusNotApplicable, Metadata: map[string]interface{}{"reason": c.definition.Description}}, nil
}

func SignalDefinitionFor(contract DatasetSignalContract, kind string) (SignalDefinition, error) {
	for _, definition := range contract.Signals {
		if definition.Kind == kind {
			return definition, nil
		}
	}
	return SignalDefinition{}, fmt.Errorf("signal definition %s not found", kind)
}

func mergeSignalMetadata(base, extra map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base)+len(extra))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func truncateSignalError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 2000 {
		return message
	}
	return message[:2000]
}
