package dataquality

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

const defaultRepairReplayBatchSize = 500

type FlowReplayPublisher interface {
	Ready(context.Context) error
	Target() string
	Publish(context.Context, string, []*pb.FlowEvent) error
}

// KafkaFlowReplayPublisher intentionally rejects the ordinary source topic.
// A replay must use a dedicated, operator-validated projection replay topic so
// it cannot duplicate traffic.flows_raw by feeding the raw ingest path.
type KafkaFlowReplayPublisher struct {
	producer    *commonkafka.Producer
	topic       string
	targetReady func(context.Context, string) error
}

func NewKafkaFlowReplayPublisher(producer *commonkafka.Producer, topic string, targetReady func(context.Context, string) error) *KafkaFlowReplayPublisher {
	return &KafkaFlowReplayPublisher{producer: producer, topic: strings.TrimSpace(topic), targetReady: targetReady}
}

func (p *KafkaFlowReplayPublisher) Ready(ctx context.Context) error {
	if p == nil || p.producer == nil || p.topic == "" {
		return fmt.Errorf("flow replay publisher is unavailable")
	}
	if p.topic == "flow.events.v1" {
		return fmt.Errorf("flow replay publisher refuses the raw ingest topic")
	}
	if configured := p.producer.Topic(); configured != p.topic {
		return fmt.Errorf("flow replay publisher topic %q does not match producer topic %q", p.topic, configured)
	}
	if p.targetReady == nil {
		return fmt.Errorf("flow replay target consumer readiness is not configured")
	}
	return p.targetReady(ctx, p.topic)
}

func (p *KafkaFlowReplayPublisher) Target() string {
	if p == nil {
		return ""
	}
	return p.topic
}

func (p *KafkaFlowReplayPublisher) Publish(ctx context.Context, repairID string, events []*pb.FlowEvent) error {
	if err := p.Ready(ctx); err != nil {
		return err
	}
	messages := make([]commonkafka.Message, 0, len(events))
	for _, event := range events {
		if event == nil || event.Header == nil || event.Header.EventId == "" || event.Header.TenantId == "" {
			return fmt.Errorf("flow replay event identity is incomplete")
		}
		value, err := proto.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal flow replay event %s: %w", event.Header.EventId, err)
		}
		messages = append(messages, commonkafka.Message{
			Key:   event.Header.TenantId + ":" + event.CommunityId,
			Value: value,
			Headers: []commonkafka.MessageHeader{
				{Key: "tenant_id", Value: event.Header.TenantId},
				{Key: "event_id", Value: event.Header.EventId},
				{Key: "repair_id", Value: repairID},
				{Key: "idempotency_key", Value: event.Header.IdempotencyKey},
				{Key: "content_type", Value: "application/x-protobuf"},
				{Key: "proto_message_type", Value: "traffic.v1.FlowEvent"},
				{Key: "proto_schema_version", Value: "v1"},
				{Key: "replay", Value: "true"},
			},
			Time: time.UnixMilli(event.Header.EventTs),
		})
	}
	return p.producer.SendBatch(ctx, messages)
}

type ClickHouseFlowReplayDriver struct {
	factsDB   *sql.DB
	publisher FlowReplayPublisher
	batchSize int
	clock     func() time.Time
}

func NewClickHouseFlowReplayDriver(factsDB *sql.DB, publisher FlowReplayPublisher, batchSize int) *ClickHouseFlowReplayDriver {
	if batchSize <= 0 || batchSize > 5000 {
		batchSize = defaultRepairReplayBatchSize
	}
	return &ClickHouseFlowReplayDriver{factsDB: factsDB, publisher: publisher, batchSize: batchSize, clock: time.Now}
}

func (d *ClickHouseFlowReplayDriver) Ready(ctx context.Context) error {
	if d == nil || d.factsDB == nil || d.publisher == nil {
		return fmt.Errorf("ClickHouse flow replay driver dependencies are unavailable")
	}
	if err := d.factsDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ClickHouse flow replay source is unavailable: %w", err)
	}
	return d.publisher.Ready(ctx)
}

func (d *ClickHouseFlowReplayDriver) Replay(ctx context.Context, request RepairReplayRequest) (map[string]interface{}, error) {
	if err := d.Ready(ctx); err != nil {
		return nil, err
	}
	if request.OperationID != "flow_replay_window_v1" {
		return nil, fmt.Errorf("unsupported replay operation %q", request.OperationID)
	}
	if err := validateRepairScope(request.TenantID, request.InputScope, request.ResourceBudget); err != nil {
		return nil, err
	}
	start, _ := time.Parse(time.RFC3339, stringValue(request.InputScope["window_start"]))
	end, _ := time.Parse(time.RFC3339, stringValue(request.InputScope["window_end"]))
	maxRows := int64Value(request.ResourceBudget["max_rows"])
	timeout := time.Duration(int64Value(request.ResourceBudget["max_duration_seconds"])) * time.Second
	executionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var sourceRows uint64
	if err := d.factsDB.QueryRowContext(executionCtx, `
		SELECT count()
		FROM traffic.flows_raw
		WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?
	`, request.TenantID, start.UnixMilli(), end.UnixMilli()).Scan(&sourceRows); err != nil {
		return nil, fmt.Errorf("count bounded flow replay source: %w", err)
	}
	if sourceRows == 0 || sourceRows > uint64(maxRows) {
		return map[string]interface{}{"source_rows": sourceRows, "published_rows": int64(0), "replay_target": d.publisher.Target()}, fmt.Errorf("flow replay source row count is outside approved budget")
	}

	rows, err := d.factsDB.QueryContext(executionCtx, flowReplaySelectSQL, request.TenantID, start.UnixMilli(), end.UnixMilli(), maxRows)
	if err != nil {
		return nil, fmt.Errorf("query bounded flow replay source: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{}, sourceRows)
	batch := make([]*pb.FlowEvent, 0, d.batchSize)
	var publishedRows, duplicateRows int64
	for rows.Next() {
		row := flowReplayRow{}
		if err := row.scan(rows); err != nil {
			return replayProgressSummary(sourceRows, publishedRows, duplicateRows, d.publisher.Target()), fmt.Errorf("scan bounded flow replay source: %w", err)
		}
		if row.eventID == "" {
			return replayProgressSummary(sourceRows, publishedRows, duplicateRows, d.publisher.Target()), fmt.Errorf("flow replay source has an empty stable event identity")
		}
		if _, exists := seen[row.eventID]; exists {
			duplicateRows++
			continue
		}
		seen[row.eventID] = struct{}{}
		batch = append(batch, row.toProto(request, d.clock().UTC()))
		if len(batch) == d.batchSize {
			if err := d.publisher.Publish(executionCtx, request.RepairID, batch); err != nil {
				return replayProgressSummary(sourceRows, publishedRows, duplicateRows, d.publisher.Target()), fmt.Errorf("publish bounded flow replay batch: %w", err)
			}
			publishedRows += int64(len(batch))
			batch = batch[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return replayProgressSummary(sourceRows, publishedRows, duplicateRows, d.publisher.Target()), fmt.Errorf("iterate bounded flow replay source: %w", err)
	}
	if len(batch) > 0 {
		if err := d.publisher.Publish(executionCtx, request.RepairID, batch); err != nil {
			return replayProgressSummary(sourceRows, publishedRows, duplicateRows, d.publisher.Target()), fmt.Errorf("publish final bounded flow replay batch: %w", err)
		}
		publishedRows += int64(len(batch))
	}
	return map[string]interface{}{
		"published": true, "source_rows": sourceRows, "published_rows": publishedRows,
		"duplicate_rows_skipped": duplicateRows, "replay_target": d.publisher.Target(),
	}, nil
}

func replayProgressSummary(sourceRows uint64, publishedRows, duplicateRows int64, target string) map[string]interface{} {
	return map[string]interface{}{
		"published": false, "source_rows": sourceRows, "published_rows": publishedRows,
		"duplicate_rows_skipped": duplicateRows, "replay_target": target,
	}
}

const flowReplaySelectSQL = `SELECT
	event_id,tenant_id,probe_id,community_id,src_ip,dst_ip,src_port,dst_port,protocol,direction,
	ts_start,ts_end,duration_ms,packets_fwd,packets_bwd,bytes_fwd,bytes_bwd,pps,bps,
	tcp_flags_fwd,tcp_flags_bwd,tos,run_id,feature_set_id,event_ts,ingest_ts,
	pktlen_min,pktlen_max,pktlen_mean,pktlen_std,iat_min_ms,iat_max_ms,iat_mean_ms,iat_std_ms,
	active_min_ms,active_max_ms,active_mean_ms,active_std_ms,idle_min_ms,idle_max_ms,idle_mean_ms,idle_std_ms,
	subflow_count
	FROM traffic.flows_raw
	WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?
	ORDER BY ingest_ts,event_id
	LIMIT ?`

type flowReplayRow struct {
	eventID, tenantID, probeID, communityID, srcIP, dstIP, direction, runID, featureSetID string
	srcPort, dstPort, protocol, durationMS, packetsFwd, packetsBwd                        uint32
	bytesFwd, bytesBwd                                                                    uint64
	pps, bps                                                                              float32
	tcpFlagsFwd, tcpFlagsBwd, tos                                                         uint32
	tsStart, tsEnd, eventTS, ingestTS                                                     int64
	pktlenMin, pktlenMax                                                                  uint32
	pktlenMean, pktlenStd, iatMin, iatMax, iatMean, iatStd                                float32
	activeMin, activeMax, activeMean, activeStd                                           float32
	idleMin, idleMax, idleMean, idleStd                                                   float32
	subflowCount                                                                          uint32
}

type rowScanner interface{ Scan(...interface{}) error }

func (r *flowReplayRow) scan(row rowScanner) error {
	return row.Scan(
		&r.eventID, &r.tenantID, &r.probeID, &r.communityID, &r.srcIP, &r.dstIP, &r.srcPort, &r.dstPort, &r.protocol, &r.direction,
		&r.tsStart, &r.tsEnd, &r.durationMS, &r.packetsFwd, &r.packetsBwd, &r.bytesFwd, &r.bytesBwd, &r.pps, &r.bps,
		&r.tcpFlagsFwd, &r.tcpFlagsBwd, &r.tos, &r.runID, &r.featureSetID, &r.eventTS, &r.ingestTS,
		&r.pktlenMin, &r.pktlenMax, &r.pktlenMean, &r.pktlenStd, &r.iatMin, &r.iatMax, &r.iatMean, &r.iatStd,
		&r.activeMin, &r.activeMax, &r.activeMean, &r.activeStd, &r.idleMin, &r.idleMax, &r.idleMean, &r.idleStd,
		&r.subflowCount,
	)
}

func (r flowReplayRow) toProto(request RepairReplayRequest, producedAt time.Time) *pb.FlowEvent {
	return &pb.FlowEvent{
		Header: &pb.EventHeader{
			EventId: r.eventID, TenantId: r.tenantID, RunId: r.runID, EventTs: r.eventTS,
			IngestTs: r.ingestTS, ProbeId: r.probeID, FeatureSetId: r.featureSetID,
			EventType: "flow.replay.v1", SchemaVersion: "v1", AggregateType: "flow", AggregateId: r.eventID,
			AggregateVersion: 1, OccurredAt: r.eventTS, ProducedAt: producedAt.UnixMilli(), TraceId: request.TraceID,
			CausationId: request.RepairID, CorrelationId: request.RepairID,
			IdempotencyKey: request.RepairID + ":" + r.eventID, Producer: "data-quality-repair-executor",
		},
		FlowId: r.eventID, CommunityId: r.communityID,
		Tuple:     &pb.FiveTuple{SrcIp: r.srcIP, DstIp: r.dstIP, SrcPort: r.srcPort, DstPort: r.dstPort, Protocol: r.protocol},
		Direction: r.direction, TsStart: r.tsStart, TsEnd: r.tsEnd, DurationMs: r.durationMS,
		PacketsFwd: r.packetsFwd, PacketsBwd: r.packetsBwd, BytesFwd: r.bytesFwd, BytesBwd: r.bytesBwd,
		Pps: r.pps, Bps: r.bps, TcpFlagsFwd: r.tcpFlagsFwd, TcpFlagsBwd: r.tcpFlagsBwd, Tos: r.tos,
		PktlenStats:  &pb.PacketLengthStats{Min: r.pktlenMin, Max: r.pktlenMax, Mean: r.pktlenMean, Std: r.pktlenStd},
		IatStats:     &pb.InterArrivalStats{MinMs: r.iatMin, MaxMs: r.iatMax, MeanMs: r.iatMean, StdMs: r.iatStd},
		ActiveStats:  &pb.ActiveIdleStats{MinMs: r.activeMin, MaxMs: r.activeMax, MeanMs: r.activeMean, StdMs: r.activeStd},
		IdleStats:    &pb.ActiveIdleStats{MinMs: r.idleMin, MaxMs: r.idleMax, MeanMs: r.idleMean, StdMs: r.idleStd},
		SubflowCount: r.subflowCount,
	}
}
