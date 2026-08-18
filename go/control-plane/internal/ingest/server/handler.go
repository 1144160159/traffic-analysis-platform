package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dlq"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/metrics"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/queue"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	errMsgEmptyRequest        = "empty request"
	errMsgBatchTooLarge       = "batch size exceeds maximum"
	errMsgTenantIDRequired    = "tenant_id not found in context"
	errMsgProbeIDRequired     = "probe_id not found in context"
	errMsgFileKeyRequired     = "file_key is required"
	errMsgEventNil            = "event is nil"
	errMsgHeaderNil           = "event header is nil"
	errMsgTupleNil            = "tuple is nil"
	errMsgIPRequired          = "src_ip and dst_ip are required"
	errMsgSessionIDRequired   = "session_id is required"
	errMsgCommunityIDRequired = "community_id is required"
	errMsgEventTooLarge       = "event size exceeds maximum"

	msgSuccess              = "success"
	msgPcapKafkaAccepted    = "pcap metadata durably accepted by Kafka; downstream index pending"
	msgPartialFailure       = "partial failure: %s"
	msgRejectedDeduplicated = "%d rejected, %d deduplicated"
)

type probeStatusEntry struct {
	Status    *pb.ProbeStatus
	UpdatedAt time.Time
}

type IngestHandler struct {
	pb.UnimplementedIngestServiceServer

	producer           *queue.Producer
	writeFlowEvents    func(context.Context, []*pb.FlowEvent) (queue.BatchWriteResult, error)
	writePcapIndex     func(context.Context, *pb.PcapIndexMeta) error
	writeAssetBindings func(context.Context, []*pb.MacIpBinding) (queue.AssetBindingWriteResult, error)
	dlqProducer        *dlq.Producer
	deduper            *dedup.Deduplicator
	metrics            *metrics.Metrics
	configManager      *config.ProbeConfigManager
	controlBridge      ProbeControlBridge
	controlBridgeMu    sync.RWMutex
	probeRegistry      ProbeRegistry
	auditLogger        *audit.Logger
	logger             *zap.Logger

	probeStatus sync.Map

	handlerConfig HandlerConfig

	totalEventsReceived int64
	totalEventsAccepted int64
	totalEventsRejected int64
	totalEventsDedupe   int64

	defaultFeatureSetID string
}

// ProbeControlBridge durably accepts Agent ACKs and returns only commands
// already routed to the authenticated tenant/probe identity. Implementations
// must not return an accepted ACK id before it is durable.
type ProbeControlBridge interface {
	Exchange(
		ctx context.Context,
		tenantID string,
		probeID string,
		acks []*pb.ProbeOperationAck,
	) (commands []*pb.ProbeOperationCommand, acceptedAckOperationIDs []string, err error)
}

type HandlerConfig struct {
	MaxBatchSize         int           `env:"MAX_BATCH_SIZE" envDefault:"10000"`
	MaxEventSize         int           `env:"MAX_EVENT_SIZE" envDefault:"65536"`
	StreamBufferSize     int           `env:"STREAM_BUFFER_SIZE" envDefault:"1000"`
	HeartbeatInterval    time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	EnableDLQ            bool          `env:"ENABLE_DLQ" envDefault:"true"`
	EnableDedup          bool          `env:"ENABLE_DEDUP" envDefault:"true"`
	ProbeStatusTimeout   time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
	EnableAudit          bool          `env:"ENABLE_AUDIT" envDefault:"true"`
	FlowWriterEnabled    bool
	PcapWriterEnabled    bool
	BindingWriterEnabled bool
	CanaryTenantID       string
	CanaryProbeIDs       []string
}

func NewIngestHandlerWithConfig(
	logger *zap.Logger,
	producer *queue.Producer,
	dlqProducer *dlq.Producer,
	m *metrics.Metrics,
	cfg HandlerConfig,
) *IngestHandler {

	if cfg.ProbeStatusTimeout <= 0 {
		cfg.ProbeStatusTimeout = config.DefaultProbeStatusTimeout
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = config.DefaultMaxBatchSize
	}
	if cfg.MaxEventSize <= 0 {
		cfg.MaxEventSize = config.DefaultMaxEventSize
	}
	if cfg.StreamBufferSize <= 0 {
		cfg.StreamBufferSize = config.DefaultStreamBufferSize
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = config.DefaultHeartbeatInterval
	}

	h := &IngestHandler{
		producer:            producer,
		dlqProducer:         dlqProducer,
		metrics:             m,
		logger:              logger,
		handlerConfig:       cfg,
		defaultFeatureSetID: config.DefaultFeatureSetID,
	}
	if producer != nil && cfg.FlowWriterEnabled {
		h.writeFlowEvents = producer.WriteFlowEvents
	}
	if producer != nil && cfg.PcapWriterEnabled {
		h.writePcapIndex = producer.WritePcapIndex
	}
	if producer != nil && cfg.BindingWriterEnabled {
		h.writeAssetBindings = producer.WriteAssetBindings
	}

	logger.Info("Handler initialized",
		zap.Bool("enable_dedup", cfg.EnableDedup),
		zap.Bool("enable_dlq", cfg.EnableDLQ),
		zap.Bool("enable_audit", cfg.EnableAudit),
		zap.Duration("probe_status_timeout", cfg.ProbeStatusTimeout),
		zap.String("default_feature_set_id", h.defaultFeatureSetID))

	return h
}

// writerScopeAllows is the final identity barrier before an M02 canary writer.
// An empty scope is accepted only for package-local tests that inject a writer
// directly; production configuration rejects enabled writers without an exact
// tenant and explicit probe set.
func (h *IngestHandler) writerScopeAllows(tenantID, probeID string) bool {
	if h.handlerConfig.CanaryTenantID == "" && len(h.handlerConfig.CanaryProbeIDs) == 0 {
		return true
	}
	if tenantID != h.handlerConfig.CanaryTenantID {
		return false
	}
	for _, allowedProbeID := range h.handlerConfig.CanaryProbeIDs {
		if probeID == allowedProbeID {
			return true
		}
	}
	return false
}

func (h *IngestHandler) SetConfigManager(cm *config.ProbeConfigManager) {
	h.configManager = cm
	h.logger.Info("Config manager set")
}

func (h *IngestHandler) SetProbeControlBridge(bridge ProbeControlBridge) {
	h.controlBridgeMu.Lock()
	h.controlBridge = bridge
	h.controlBridgeMu.Unlock()
	h.logger.Info("Probe control bridge set", zap.Bool("enabled", bridge != nil))
}

func (h *IngestHandler) SetProbeRegistry(registry ProbeRegistry) {
	h.probeRegistry = registry
	h.logger.Info("Probe registry set", zap.Bool("enabled", registry != nil))
}

func (h *IngestHandler) SetDeduplicator(d *dedup.Deduplicator) {
	h.deduper = d
	h.logger.Info("Deduplicator set", zap.Bool("enabled", d != nil))
}

func (h *IngestHandler) SetDLQProducer(dlq *dlq.Producer) {
	h.dlqProducer = dlq
	h.logger.Info("DLQ producer set", zap.Bool("enabled", dlq != nil))
}

func (h *IngestHandler) SetAuditLogger(auditLogger *audit.Logger) {
	h.auditLogger = auditLogger
	h.logger.Info("Audit logger set", zap.Bool("enabled", auditLogger != nil))
}

func (h *IngestHandler) StartProbeStatusCleaner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		h.logger.Info("Probe status cleaner started",
			zap.Duration("timeout", h.handlerConfig.ProbeStatusTimeout))

		for {
			select {
			case <-ctx.Done():
				h.logger.Info("Probe status cleaner stopped")
				return
			case <-ticker.C:
				h.cleanExpiredProbeStatus()
			}
		}
	}()
}

func (h *IngestHandler) cleanExpiredProbeStatus() {
	threshold := time.Now().Add(-h.handlerConfig.ProbeStatusTimeout)
	expiredCount := 0

	h.probeStatus.Range(func(key, value interface{}) bool {
		entry := value.(*probeStatusEntry)
		if entry.UpdatedAt.Before(threshold) {
			h.probeStatus.Delete(key)
			expiredCount++
		}
		return true
	})

	if expiredCount > 0 {
		h.logger.Debug("Cleaned expired probe status entries",
			zap.Int("count", expiredCount))
	}
}

func (h *IngestHandler) isDedupEnabled() bool {
	return h.handlerConfig.EnableDedup && h.deduper != nil
}

func (h *IngestHandler) recordAudit(ctx context.Context, eventType audit.EventType, tenantID, probeID, action string, detail map[string]interface{}) {
	if !h.handlerConfig.EnableAudit || h.auditLogger == nil {
		return
	}

	h.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       probeID,
		Action:       action,
		ResourceType: "ingest",
		Result:       audit.ResultSuccess,
		Detail:       detail,
	})
}

func (h *IngestHandler) getFeatureSetID(ctx context.Context, tenantID, probeID string) string {

	if h.configManager != nil {
		cfg, err := h.configManager.GetConfig(ctx, tenantID, probeID)
		if err == nil && cfg != nil && cfg.FeatureSetVersion != "" {
			return cfg.FeatureSetVersion
		}
	}

	if h.defaultFeatureSetID != "" {
		return h.defaultFeatureSetID
	}

	return config.DefaultFeatureSetID
}

func (h *IngestHandler) UploadFlows(ctx context.Context, req *pb.UploadFlowsRequest) (*pb.UploadFlowsResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_flows")
	defer span.End()
	start := time.Now()

	if req != nil && len(req.Events) > 0 {
		atomic.AddInt64(&h.totalEventsReceived, int64(len(req.Events)))
	}

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	if tenantID == "" {
		h.metrics.RecordReject401()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, "", probeID, "upload_flows", map[string]interface{}{
			"reason": "missing_tenant_id",
		})
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}
	if probeID == "" {
		h.metrics.RecordReject401()
		return nil, status.Error(codes.Unauthenticated, errMsgProbeIDRequired)
	}

	if req == nil || len(req.Events) == 0 {
		return &pb.UploadFlowsResponse{
			Accepted: 0,
			Rejected: 0,
			Message:  errMsgEmptyRequest,
		}, nil
	}

	if len(req.Events) > h.handlerConfig.MaxBatchSize {
		h.metrics.RecordError("batch_too_large")
		h.metrics.RecordReject400()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, tenantID, probeID, "upload_flows", map[string]interface{}{
			"reason":     "batch_too_large",
			"batch_size": len(req.Events),
			"max_size":   h.handlerConfig.MaxBatchSize,
		})
		return nil, status.Errorf(codes.InvalidArgument,
			"%s: %d > %d", errMsgBatchTooLarge, len(req.Events), h.handlerConfig.MaxBatchSize)
	}

	h.metrics.RecordBatchSize(tenantID, len(req.Events))

	logger.Debug("Received flow events",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int("count", len(req.Events)),
		zap.String("compression", req.Compression))

	itemResults := make([]*pb.FlowItemResult, len(req.Events))
	for inputIndex, event := range req.Events {
		if event == nil {
			itemResults[inputIndex] = rejectedFlowItem(inputIndex, "", "EVENT_NIL")
			continue
		}
		if err := bindFlowEventIdentity(event, tenantID, probeID); err != nil {
			h.metrics.RecordReject403()
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	validEvents := make([]*pb.FlowEvent, 0, len(req.Events))
	validInputIndexes := make([]int, 0, len(req.Events))
	validEventIDs := make([]string, 0, len(req.Events))
	nowMs := time.Now().UnixMilli()
	for inputIndex, event := range req.Events {
		if event == nil {
			continue
		}
		if event.Header.IngestTs == 0 {
			event.Header.IngestTs = nowMs
		}
		if event.Header.FeatureSetId == "" {
			event.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
		}
		if err := h.validateFlowEvent(event); err != nil {
			logger.Debug("Event validation failed",
				zap.String("event_id", event.Header.EventId),
				zap.Error(err))
			itemResults[inputIndex] = rejectedFlowItem(inputIndex, event.Header.EventId, "FLOW_VALIDATION_FAILED")
			continue
		}
		validEvents = append(validEvents, event)
		validInputIndexes = append(validInputIndexes, inputIndex)
		validEventIDs = append(validEventIDs, event.Header.EventId)
	}

	claims := make([]dedup.Claim, len(validEvents))
	if h.isDedupEnabled() && len(validEvents) > 0 {
		var claimErr error
		claims, claimErr = h.deduper.ClaimBatch(ctx, tenantID, probeID, validEventIDs)
		if claimErr != nil {
			h.metrics.RecordReject503()
			return nil, status.Errorf(codes.Unavailable, "dedup claim failed: %v", claimErr)
		}
	}

	sendEvents := make([]*pb.FlowEvent, 0, len(validEvents))
	sendInputIndexes := make([]int, 0, len(validEvents))
	sendClaims := make([]dedup.Claim, 0, len(validEvents))
	for validIndex, event := range validEvents {
		claim := claims[validIndex]
		if h.isDedupEnabled() {
			switch claim.Status {
			case dedup.ClaimDuplicateCommitted:
				itemResults[validInputIndexes[validIndex]] = &pb.FlowItemResult{
					InputIndex:  uint32(validInputIndexes[validIndex]),
					EventId:     event.Header.EventId,
					Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED,
					ReasonCode:  "DEDUP_COMMITTED",
					AckScope:    "TENANT_PROBE_EVENT",
				}
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			case dedup.ClaimInFlight:
				itemResults[validInputIndexes[validIndex]] = &pb.FlowItemResult{
					InputIndex:  uint32(validInputIndexes[validIndex]),
					EventId:     event.Header.EventId,
					Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE,
					ReasonCode:  "DEDUP_CLAIM_IN_FLIGHT",
					AckScope:    "TENANT_PROBE_EVENT",
				}
				continue
			default:
				h.metrics.RecordDedupMiss()
			}
		}
		sendEvents = append(sendEvents, event)
		sendInputIndexes = append(sendInputIndexes, validInputIndexes[validIndex])
		sendClaims = append(sendClaims, claim)
	}

	var writeErr error
	if len(sendEvents) > 0 {
		var writeResult queue.BatchWriteResult
		kafkaStart := time.Now()
		if !h.writerScopeAllows(tenantID, probeID) {
			writeErr = fmt.Errorf("flow writer is not enabled for authenticated canary scope")
			writeResult.Items = make([]queue.FlowWriteItemResult, len(sendEvents))
			for i, event := range sendEvents {
				writeResult.Items[i] = queue.FlowWriteItemResult{InputIndex: i, EventID: event.Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, ReasonCode: "CANARY_SCOPE_NOT_ACTIVE", AckScope: "TENANT_PROBE_EVENT"}
			}
		} else if h.writeFlowEvents == nil {
			writeErr = fmt.Errorf("flow producer is not configured")
			writeResult.Items = make([]queue.FlowWriteItemResult, len(sendEvents))
			for i, event := range sendEvents {
				writeResult.Items[i] = queue.FlowWriteItemResult{InputIndex: i, EventID: event.Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, ReasonCode: "KAFKA_NOT_CONFIGURED", AckScope: "KAFKA_RECORD"}
			}
		} else {
			writeResult, writeErr = h.writeFlowEvents(ctx, sendEvents)
		}
		h.metrics.RecordKafkaLatency(config.TopicFlowEvents, time.Since(kafkaStart))
		if exactErr := writeResult.ValidateExactSet(len(sendEvents)); exactErr != nil {
			if h.isDedupEnabled() {
				h.deduper.ReleaseBatch(ctx, sendClaims)
			}
			return nil, status.Errorf(codes.Internal, "producer result contract violated: %v", exactErr)
		}

		committedClaims := make([]dedup.Claim, 0, len(sendClaims))
		releasedClaims := make([]dedup.Claim, 0, len(sendClaims))
		for _, writeItem := range writeResult.Items {
			requestIndex := sendInputIndexes[writeItem.InputIndex]
			itemResults[requestIndex] = &pb.FlowItemResult{
				InputIndex:  uint32(requestIndex),
				EventId:     writeItem.EventID,
				Disposition: writeItem.Disposition,
				ReasonCode:  writeItem.ReasonCode,
				AckScope:    writeItem.AckScope,
			}
			if h.isDedupEnabled() {
				if writeItem.Disposition == pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED {
					committedClaims = append(committedClaims, sendClaims[writeItem.InputIndex])
				} else {
					releasedClaims = append(releasedClaims, sendClaims[writeItem.InputIndex])
				}
			}
		}
		if h.isDedupEnabled() {
			if commitErr := h.deduper.CommitBatch(ctx, committedClaims); commitErr != nil {
				logger.Error("Kafka ACK committed but dedup projection failed", zap.Error(commitErr))
			}
			h.deduper.ReleaseBatch(ctx, releasedClaims)
		}
	}

	var accepted, rejected, deduped, retryable int32
	rejectedIDs := make([]string, 0)
	for inputIndex, item := range itemResults {
		if item == nil {
			item = &pb.FlowItemResult{InputIndex: uint32(inputIndex), Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN, ReasonCode: "INTERNAL_RESULT_MISSING", AckScope: "INPUT_ITEM"}
			itemResults[inputIndex] = item
		}
		switch item.Disposition {
		case pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED:
			accepted++
		case pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED:
			accepted++
			deduped++
		case pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_REJECTED_INVALID:
			rejected++
			rejectedIDs = append(rejectedIDs, item.EventId)
		default:
			retryable++
		}
	}
	atomic.AddInt64(&h.totalEventsAccepted, int64(accepted))
	atomic.AddInt64(&h.totalEventsRejected, int64(rejected))

	h.metrics.RecordFlowEvents(tenantID, int64(accepted))
	if rejected > 0 {
		h.metrics.RecordFlowEventsRejected(tenantID, int64(rejected))
	}

	h.metrics.RecordLatency("upload_flows", time.Since(start))

	response := &pb.UploadFlowsResponse{
		Accepted:         accepted,
		Rejected:         rejected,
		RejectedIds:      rejectedIDs,
		ItemResults:      itemResults,
		ResponseRevision: 1,
	}

	if writeErr != nil {
		response.Message = fmt.Sprintf(msgPartialFailure, writeErr.Error())
	} else if retryable > 0 {
		response.Message = fmt.Sprintf(msgPartialFailure, "one or more events are not terminal")
	} else if rejected > 0 || deduped > 0 {
		response.Message = fmt.Sprintf(msgRejectedDeduplicated, rejected, deduped)
	} else {
		response.Message = msgSuccess
	}

	logger.Info("Flow events processed",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int32("accepted", accepted),
		zap.Int32("rejected", rejected),
		zap.Int32("deduped", deduped),
		zap.Int32("retryable", retryable),
		zap.Duration("duration", time.Since(start)))

	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_flows", map[string]interface{}{
		"accepted":  accepted,
		"rejected":  rejected,
		"deduped":   deduped,
		"retryable": retryable,
	})

	// gRPC cannot deliver a response body together with a non-OK status. Until
	// every deployed Agent advertises exact-set ACK support, any nonterminal
	// outcome must fail the RPC so a legacy client cannot delete a whole batch.
	if retryable > 0 && req.AcceptedResponseRevision < 1 {
		h.metrics.RecordKafkaError()
		h.metrics.RecordReject503()
		return nil, status.Error(codes.Unavailable, response.Message)
	}
	return response, nil
}

func rejectedFlowItem(inputIndex int, eventID, reasonCode string) *pb.FlowItemResult {
	return &pb.FlowItemResult{
		InputIndex:  uint32(inputIndex),
		EventId:     eventID,
		Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_REJECTED_INVALID,
		ReasonCode:  reasonCode,
		AckScope:    "INPUT_ITEM",
	}
}

func bindFlowEventIdentity(event *pb.FlowEvent, tenantID, probeID string) error {
	if event == nil {
		return fmt.Errorf("%s", errMsgEventNil)
	}
	if event.Header == nil {
		event.Header = &pb.EventHeader{}
	}
	if event.Header.TenantId != "" && event.Header.TenantId != tenantID {
		return fmt.Errorf("flow event tenant_id does not match authenticated tenant")
	}
	if event.Header.ProbeId != "" && event.Header.ProbeId != probeID {
		return fmt.Errorf("flow event probe_id does not match authenticated probe")
	}
	event.Header.TenantId = tenantID
	event.Header.ProbeId = probeID
	return nil
}

func (h *IngestHandler) validateFlowEvent(event *pb.FlowEvent) *errors.AppError {
	if event == nil {
		return errors.New(errors.ErrCodeInvalidRequest, errMsgEventNil)
	}
	if event.Header == nil {
		return errors.New(errors.ErrCodeInvalidRequest, errMsgHeaderNil)
	}
	if event.Header.TenantId == "" {
		return errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if event.Header.ProbeId == "" {
		return errors.New(errors.ErrCodeMissingParameter, "probe_id is required")
	}
	if event.Header.EventId == "" {
		return errors.New(errors.ErrCodeMissingParameter, "event_id is required")
	}
	if event.Header.EventTs <= 0 {
		return errors.New(errors.ErrCodeMissingParameter, "event_ts is required")
	}
	if event.Tuple == nil {
		return errors.New(errors.ErrCodeInvalidRequest, errMsgTupleNil)
	}
	if event.Tuple.SrcIp == "" || event.Tuple.DstIp == "" {
		return errors.New(errors.ErrCodeMissingParameter, errMsgIPRequired)
	}

	actualSize := proto.Size(event)
	if actualSize > h.handlerConfig.MaxEventSize {
		return errors.Newf(errors.ErrCodeOutOfRange,
			"%s: %d > %d", errMsgEventTooLarge, actualSize, h.handlerConfig.MaxEventSize)
	}

	return nil
}

func (h *IngestHandler) validateSessionEvent(session *pb.SessionEvent) *errors.AppError {
	if session == nil {
		return errors.New(errors.ErrCodeInvalidRequest, errMsgEventNil)
	}
	if session.Header == nil {
		return errors.New(errors.ErrCodeInvalidRequest, errMsgHeaderNil)
	}
	if session.Header.TenantId == "" {
		return errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if session.SessionId == "" {
		return errors.New(errors.ErrCodeMissingParameter, errMsgSessionIDRequired)
	}
	if session.CommunityId == "" {
		return errors.New(errors.ErrCodeMissingParameter, errMsgCommunityIDRequired)
	}

	actualSize := proto.Size(session)
	if actualSize > h.handlerConfig.MaxEventSize {
		return errors.Newf(errors.ErrCodeOutOfRange,
			"%s: %d > %d", errMsgEventTooLarge, actualSize, h.handlerConfig.MaxEventSize)
	}

	return nil
}

func (h *IngestHandler) GetStats() HandlerStats {
	return HandlerStats{
		TotalEventsReceived: atomic.LoadInt64(&h.totalEventsReceived),
		TotalEventsAccepted: atomic.LoadInt64(&h.totalEventsAccepted),
		TotalEventsRejected: atomic.LoadInt64(&h.totalEventsRejected),
		TotalEventsDedupe:   atomic.LoadInt64(&h.totalEventsDedupe),
		ActiveProbes:        h.countActiveProbes(),
		DedupEnabled:        h.isDedupEnabled(),
	}
}

type HandlerStats struct {
	TotalEventsReceived int64
	TotalEventsAccepted int64
	TotalEventsRejected int64
	TotalEventsDedupe   int64
	ActiveProbes        int
	DedupEnabled        bool
}

func (h *IngestHandler) countActiveProbes() int {
	count := 0
	threshold := time.Now().Add(-h.handlerConfig.ProbeStatusTimeout)

	h.probeStatus.Range(func(key, value interface{}) bool {
		entry := value.(*probeStatusEntry)
		if entry.UpdatedAt.After(threshold) {
			count++
		}
		return true
	})

	return count
}

func (h *IngestHandler) UploadSessions(ctx context.Context, req *pb.UploadSessionsRequest) (*pb.UploadSessionsResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_sessions")
	defer span.End()
	start := time.Now()

	if req != nil && len(req.Sessions) > 0 {
		atomic.AddInt64(&h.totalEventsReceived, int64(len(req.Sessions)))
	}

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	if tenantID == "" {
		h.metrics.RecordReject401()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, "", probeID, "upload_sessions", map[string]interface{}{
			"reason": "missing_tenant_id",
		})
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}

	if req == nil || len(req.Sessions) == 0 {
		return &pb.UploadSessionsResponse{
			Accepted: 0,
			Rejected: 0,
			Message:  errMsgEmptyRequest,
		}, nil
	}

	if len(req.Sessions) > h.handlerConfig.MaxBatchSize {
		h.metrics.RecordError("batch_too_large")
		h.metrics.RecordReject400()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, tenantID, probeID, "upload_sessions", map[string]interface{}{
			"reason":     "batch_too_large",
			"batch_size": len(req.Sessions),
			"max_size":   h.handlerConfig.MaxBatchSize,
		})
		return nil, status.Errorf(codes.InvalidArgument,
			"%s: %d > %d", errMsgBatchTooLarge, len(req.Sessions), h.handlerConfig.MaxBatchSize)
	}

	h.metrics.RecordBatchSize(tenantID, len(req.Sessions))

	logger.Debug("Received session events",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int("count", len(req.Sessions)))

	validSessions := make([]*pb.SessionEvent, 0, len(req.Sessions))
	rejectedIDs := make([]string, 0)
	dedupedIDs := make([]string, 0)
	retryableIDs := make([]string, 0)

	now := time.Now()
	nowMs := now.UnixMilli()

	for _, session := range req.Sessions {
		if session == nil {
			continue
		}

		if session.Header == nil {
			session.Header = &pb.EventHeader{}
		}

		if session.Header.EventId == "" {
			session.Header.EventId = uuid.New().String()
		}
		if session.Header.TenantId == "" {
			session.Header.TenantId = tenantID
		}
		if session.Header.ProbeId == "" {
			session.Header.ProbeId = probeID
		}
		if session.Header.EventTs == 0 {
			session.Header.EventTs = nowMs
		}
		if session.Header.IngestTs == 0 {
			session.Header.IngestTs = nowMs
		}

		if session.Header.FeatureSetId == "" {
			session.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
		}

		if err := h.validateSessionEvent(session); err != nil {
			logger.Debug("Session validation failed",
				zap.String("session_id", session.SessionId),
				zap.Error(err))
			rejectedIDs = append(rejectedIDs, session.Header.EventId)
			continue
		}

		validSessions = append(validSessions, session)
	}

	// 原子去重:与 flows 路径一致,使用 ClaimBatch 以 tenant:probe:event 复合键
	// 在 Redis SetNX 上原子领取,消除 check-then-set 竞态导致的并发重复入 Kafka,
	// 并在 Kafka acks=all 屏障后 CommitBatch / ReleaseBatch。
	claims := make([]dedup.Claim, len(validSessions))
	if h.isDedupEnabled() && len(validSessions) > 0 {
		eventIDs := make([]string, len(validSessions))
		for i, s := range validSessions {
			eventIDs[i] = s.Header.EventId
		}
		var claimErr error
		claims, claimErr = h.deduper.ClaimBatch(ctx, tenantID, probeID, eventIDs)
		if claimErr != nil {
			h.metrics.RecordReject503()
			return nil, status.Errorf(codes.Unavailable, "session dedup claim failed: %v", claimErr)
		}
	}

	sendSessions := make([]*pb.SessionEvent, 0, len(validSessions))
	sendClaims := make([]dedup.Claim, 0, len(validSessions))
	for validIndex, session := range validSessions {
		claim := claims[validIndex]
		if h.isDedupEnabled() {
			switch claim.Status {
			case dedup.ClaimDuplicateCommitted:
				dedupedIDs = append(dedupedIDs, session.Header.EventId)
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			case dedup.ClaimInFlight:
				retryableIDs = append(retryableIDs, session.Header.EventId)
				continue
			default:
				h.metrics.RecordDedupMiss()
			}
		}
		sendSessions = append(sendSessions, session)
		sendClaims = append(sendClaims, claim)
	}

	var writeErr error
	if len(sendSessions) > 0 {
		kafkaStart := time.Now()
		writeErr = h.producer.WriteSessionEvents(ctx, sendSessions)
		h.metrics.RecordKafkaLatency(config.TopicSessionEvents, time.Since(kafkaStart))

		if writeErr != nil {
			logger.Error("Failed to write session events to Kafka",
				zap.Int("count", len(sendSessions)),
				zap.Error(writeErr))

			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				if dlqErr := h.dlqProducer.SendSessionEvents(ctx, sendSessions, writeErr); dlqErr != nil {
					logger.Error("Failed to persist rejected session events to DLQ",
						zap.Int("count", len(sendSessions)),
						zap.Error(dlqErr))
					h.metrics.RecordError("session_dlq_write_failed")
				}
			}

			if h.isDedupEnabled() {
				h.deduper.ReleaseBatch(ctx, sendClaims)
			}

			h.metrics.RecordKafkaError()
			h.metrics.RecordReject503()
			atomic.AddInt64(&h.totalEventsRejected, int64(len(sendSessions)))
		} else {
			if h.isDedupEnabled() {
				if commitErr := h.deduper.CommitBatch(ctx, sendClaims); commitErr != nil {
					logger.Error("Kafka ACK committed but session dedup projection failed", zap.Error(commitErr))
				}
			}
			atomic.AddInt64(&h.totalEventsAccepted, int64(len(sendSessions)))
		}
	}

	accepted := int32(0)
	if writeErr == nil {
		accepted = int32(len(sendSessions))
	}
	rejected := int32(len(rejectedIDs) + len(retryableIDs))
	deduped := int32(len(dedupedIDs))

	if accepted > 0 {
		h.metrics.RecordSessionEvents(tenantID, int64(accepted))

		var totalBytes int64
		for _, s := range validSessions {
			totalBytes += int64(proto.Size(s))
		}
		h.metrics.RecordSessionBytes(tenantID, totalBytes)
	}

	if rejected > 0 {
		h.metrics.RecordSessionEventsRejected(tenantID, int64(rejected))
	}

	h.metrics.RecordLatency("upload_sessions", time.Since(start))

	response := &pb.UploadSessionsResponse{
		Accepted:    accepted,
		Rejected:    rejected + deduped,
		RejectedIds: append(rejectedIDs, dedupedIDs...),
	}

	if writeErr != nil {
		response.Message = fmt.Sprintf(msgPartialFailure, writeErr.Error())
	} else if rejected > 0 || deduped > 0 {
		response.Message = fmt.Sprintf(msgRejectedDeduplicated, rejected, deduped)
	} else {
		response.Message = msgSuccess
	}

	logger.Info("Session events processed",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int32("accepted", accepted),
		zap.Int32("rejected", rejected),
		zap.Int32("deduped", deduped),
		zap.Duration("duration", time.Since(start)))

	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_sessions", map[string]interface{}{
		"accepted": accepted,
		"rejected": rejected,
		"deduped":  deduped,
	})

	return response, nil
}

func (h *IngestHandler) UploadAssetBindings(
	ctx context.Context,
	req *pb.UploadAssetBindingsRequest,
) (*pb.UploadAssetBindingsResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_asset_bindings")
	defer span.End()
	start := time.Now()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)
	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	if tenantID == "" {
		h.metrics.RecordReject401()
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}
	if probeID == "" {
		h.metrics.RecordReject401()
		return nil, status.Error(codes.Unauthenticated, errMsgProbeIDRequired)
	}
	if req == nil || len(req.Bindings) == 0 {
		h.metrics.RecordReject400()
		return nil, status.Error(codes.InvalidArgument, "at least one asset binding is required")
	}
	if len(req.Bindings) > h.handlerConfig.MaxBatchSize {
		h.metrics.RecordReject400()
		return nil, status.Error(codes.InvalidArgument, errMsgBatchTooLarge)
	}
	if (req.TenantId != "" && req.TenantId != tenantID) || (req.ProbeId != "" && req.ProbeId != probeID) {
		h.metrics.RecordReject403()
		return nil, status.Error(codes.PermissionDenied, "asset binding request identity does not match authenticated probe")
	}

	itemResults := make([]*pb.AssetBindingItemResult, len(req.Bindings))
	validBindings := make([]*pb.MacIpBinding, 0, len(req.Bindings))
	validInputIndexes := make([]int, 0, len(req.Bindings))
	for inputIndex, binding := range req.Bindings {
		if binding != nil && ((binding.TenantId != "" && binding.TenantId != tenantID) ||
			(binding.ProbeId != "" && binding.ProbeId != probeID)) {
			h.metrics.RecordReject403()
			return nil, status.Error(codes.PermissionDenied, "asset binding item identity does not match authenticated probe")
		}
		if binding != nil {
			binding.TenantId = tenantID
			binding.ProbeId = probeID
		}
		if reasonCode := validateAssetBinding(binding, h.handlerConfig.MaxEventSize, time.Now().UTC()); reasonCode != "" {
			observationID := ""
			if binding != nil {
				observationID = binding.ObservationId
			}
			itemResults[inputIndex] = &pb.AssetBindingItemResult{
				InputIndex: uint32(inputIndex), ObservationId: observationID,
				Disposition: pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID,
				ReasonCode:  reasonCode, AckScope: "INPUT_ITEM",
			}
			continue
		}
		validBindings = append(validBindings, binding)
		validInputIndexes = append(validInputIndexes, inputIndex)
	}

	var writeErr error
	if len(validBindings) > 0 {
		var writeResult queue.AssetBindingWriteResult
		kafkaStart := time.Now()
		switch {
		case !h.writerScopeAllows(tenantID, probeID):
			writeErr = fmt.Errorf("asset binding writer is not enabled for authenticated canary scope")
			writeResult.Items = retryableAssetBindingWriteItems(validBindings, "CANARY_SCOPE_NOT_ACTIVE")
		case h.writeAssetBindings == nil:
			writeErr = fmt.Errorf("asset binding Kafka producer is not configured")
			writeResult.Items = retryableAssetBindingWriteItems(validBindings, "KAFKA_NOT_CONFIGURED")
		default:
			writeResult, writeErr = h.writeAssetBindings(ctx, validBindings)
		}
		h.metrics.RecordKafkaLatency(config.TopicAssetBindings, time.Since(kafkaStart))
		if err := writeResult.ValidateExactSet(len(validBindings)); err != nil {
			return nil, status.Errorf(codes.Internal, "asset binding producer result contract violated: %v", err)
		}
		for _, writeItem := range writeResult.Items {
			requestIndex := validInputIndexes[writeItem.InputIndex]
			itemResults[requestIndex] = &pb.AssetBindingItemResult{
				InputIndex: uint32(requestIndex), ObservationId: writeItem.ObservationID,
				Disposition: writeItem.Disposition, ReasonCode: writeItem.ReasonCode, AckScope: writeItem.AckScope,
			}
		}
	}

	var accepted, rejected, retryable int32
	for inputIndex, item := range itemResults {
		if item == nil {
			item = &pb.AssetBindingItemResult{
				InputIndex:  uint32(inputIndex),
				Disposition: pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_OUTCOME_UNKNOWN,
				ReasonCode:  "INTERNAL_RESULT_MISSING", AckScope: "INPUT_ITEM",
			}
			itemResults[inputIndex] = item
		}
		switch item.Disposition {
		case pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED,
			pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_DUPLICATE_COMMITTED:
			accepted++
		case pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID:
			rejected++
		default:
			retryable++
		}
	}
	response := &pb.UploadAssetBindingsResponse{
		Accepted: accepted, Rejected: rejected, ItemResults: itemResults, ResponseRevision: 1,
	}
	if writeErr != nil {
		response.Message = fmt.Sprintf(msgPartialFailure, writeErr.Error())
	} else if retryable > 0 {
		response.Message = fmt.Sprintf(msgPartialFailure, "one or more asset bindings are not terminal")
	} else if rejected > 0 {
		response.Message = fmt.Sprintf("%d asset bindings rejected", rejected)
	} else {
		response.Message = msgSuccess
	}
	h.metrics.RecordLatency("upload_asset_bindings", time.Since(start))
	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_asset_bindings", map[string]interface{}{
		"accepted": accepted, "rejected": rejected, "retryable": retryable,
	})
	if retryable > 0 && req.AcceptedResponseRevision < 1 {
		h.metrics.RecordKafkaError()
		h.metrics.RecordReject503()
		return nil, status.Error(codes.Unavailable, response.Message)
	}
	return response, nil
}

func retryableAssetBindingWriteItems(bindings []*pb.MacIpBinding, reasonCode string) []queue.AssetBindingWriteItemResult {
	items := make([]queue.AssetBindingWriteItemResult, len(bindings))
	for inputIndex, binding := range bindings {
		items[inputIndex] = queue.AssetBindingWriteItemResult{
			InputIndex: inputIndex, ObservationID: binding.ObservationId,
			Disposition: pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE,
			ReasonCode:  reasonCode, AckScope: "KAFKA_RECORD",
		}
	}
	return items
}

func validateAssetBinding(binding *pb.MacIpBinding, maxEventSize int, now time.Time) string {
	if binding == nil {
		return "BINDING_REQUIRED"
	}
	if proto.Size(binding) > maxEventSize {
		return "BINDING_TOO_LARGE"
	}
	if strings.TrimSpace(binding.ObservationId) == "" || binding.ObservationId != strings.TrimSpace(binding.ObservationId) {
		return "OBSERVATION_ID_REQUIRED"
	}
	parsedMAC, err := net.ParseMAC(binding.MacAddress)
	if err != nil || len(parsedMAC) != 6 || strings.ToLower(parsedMAC.String()) != binding.MacAddress {
		return "MAC_NOT_CANONICAL"
	}
	parsedIP := net.ParseIP(binding.IpAddress)
	if parsedIP == nil || parsedIP.String() != binding.IpAddress {
		return "IP_NOT_CANONICAL"
	}
	if binding.Source != "arp" && binding.Source != "dhcp" {
		return "SOURCE_INVALID"
	}
	if binding.ObservedAt <= 0 || binding.ObservedAt > now.Add(5*time.Minute).UnixMilli() {
		return "OBSERVED_AT_INVALID"
	}
	if binding.SchemaVersion != 1 {
		return "SCHEMA_VERSION_UNSUPPORTED"
	}
	return ""
}

func (h *IngestHandler) UploadPcapIndex(ctx context.Context, req *pb.UploadPcapIndexRequest) (*pb.UploadPcapIndexResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_pcap_index")
	defer span.End()
	start := time.Now()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	if tenantID == "" {
		h.metrics.RecordReject401()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, "", probeID, "upload_pcap_index", map[string]interface{}{
			"reason": "missing_tenant_id",
		})
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}
	if probeID == "" {
		h.metrics.RecordReject401()
		return nil, status.Error(codes.Unauthenticated, "probe_id not found in context")
	}

	if req == nil || req.Index == nil {
		h.metrics.RecordReject400()
		return &pb.UploadPcapIndexResponse{
			Success: false,
			Message: errMsgEmptyRequest,
		}, nil
	}

	meta := req.Index
	if err := bindPcapIndexIdentity(meta, tenantID, probeID); err != nil {
		h.metrics.RecordReject403()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
			"reason": "authenticated_identity_mismatch",
		})
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	if meta.FileKey == "" {
		h.metrics.RecordReject400()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
			"reason": "missing_file_key",
		})
		return &pb.UploadPcapIndexResponse{
			Success: false,
			Message: errMsgFileKeyRequired,
		}, nil
	}

	logger.Debug("Received PCAP index",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("file_key", meta.FileKey),
		zap.Uint64("byte_size", meta.ByteSize))

	kafkaStart := time.Now()
	if !h.writerScopeAllows(tenantID, probeID) {
		h.metrics.RecordReject503()
		return nil, status.Error(codes.Unavailable, "pcap metadata writer is not enabled for authenticated canary scope")
	}
	if h.writePcapIndex == nil {
		h.metrics.RecordKafkaError()
		h.metrics.RecordReject503()
		return nil, status.Error(codes.Unavailable, "pcap metadata Kafka producer is not configured")
	}
	err := h.writePcapIndex(ctx, meta)
	h.metrics.RecordKafkaLatency(config.TopicPcapIndex, time.Since(kafkaStart))

	if err != nil {
		logger.Error("Failed to write PCAP index",
			zap.String("file_key", meta.FileKey),
			zap.Error(err))

		h.metrics.RecordError("pcap_index_write_failed")
		h.metrics.RecordKafkaError()
		h.metrics.RecordReject503()

		if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
			if dlqErr := h.dlqProducer.SendPcapIndex(ctx, meta, err); dlqErr != nil {
				logger.Error("Failed to persist rejected PCAP index to DLQ",
					zap.String("file_key", meta.FileKey),
					zap.Error(dlqErr))
			}
		}

		h.recordAudit(ctx, audit.EventTypeSystemError, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
			"file_key": meta.FileKey,
			"error":    err.Error(),
		})

		return nil, status.Error(codes.Unavailable, "pcap metadata Kafka durability barrier failed")
	}

	h.metrics.RecordPcapIndex(tenantID)
	h.metrics.RecordLatency("upload_pcap_index", time.Since(start))

	logger.Info("PCAP metadata durably accepted by Kafka",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("file_key", meta.FileKey),
		zap.Duration("duration", time.Since(start)))

	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
		"file_key":      meta.FileKey,
		"size":          meta.ByteSize,
		"ack_scope":     "kafka_durable",
		"final_indexed": false,
	})

	return &pb.UploadPcapIndexResponse{
		Success: true,
		Message: msgPcapKafkaAccepted,
	}, nil
}

func bindPcapIndexIdentity(meta *pb.PcapIndexMeta, tenantID, probeID string) error {
	if meta == nil {
		return fmt.Errorf("pcap index metadata is required")
	}
	if meta.TenantId != "" && meta.TenantId != tenantID {
		return fmt.Errorf("pcap index tenant_id does not match authenticated tenant")
	}
	if meta.ProbeId != "" && meta.ProbeId != probeID {
		return fmt.Errorf("pcap index probe_id does not match authenticated probe")
	}
	meta.TenantId = tenantID
	meta.ProbeId = probeID
	return nil
}

func (h *IngestHandler) StreamFlows(stream pb.IngestService_StreamFlowsServer) error {
	ctx := stream.Context()
	ctx, span := otel.StartSpan(ctx, "ingest.stream_flows")
	defer span.End()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	if tenantID == "" {
		h.metrics.RecordReject401()
		return status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}
	if probeID == "" {
		h.metrics.RecordReject401()
		return status.Error(codes.Unauthenticated, errMsgProbeIDRequired)
	}

	h.metrics.IncrActiveConnections()
	defer h.metrics.DecrActiveConnections()

	logger.Info("Stream started",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID))

	eventChan := make(chan *pb.FlowEvent, h.handlerConfig.StreamBufferSize)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		for {
			req, err := stream.Recv()
			if err != nil {
				errChan <- err
				return
			}
			if req.Event == nil {
				continue
			}
			select {
			case eventChan <- req.Event:
			case <-ctx.Done():
				return
			}
		}
	}()

	buffer := make([]*pb.FlowEvent, 0, h.handlerConfig.StreamBufferSize)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var totalReceived, totalAccepted, totalDeduped int64

	flushBuffer := func() error {
		if len(buffer) == 0 {
			return nil
		}

		claims := make([]dedup.Claim, len(buffer))
		if h.isDedupEnabled() {
			eventIDs := make([]string, len(buffer))
			for i, event := range buffer {
				eventIDs[i] = event.Header.EventId
			}
			var claimErr error
			claims, claimErr = h.deduper.ClaimBatch(ctx, tenantID, probeID, eventIDs)
			if claimErr != nil {
				return status.Errorf(codes.Unavailable, "dedup claim failed: %v", claimErr)
			}
		}

		sendEvents := make([]*pb.FlowEvent, 0, len(buffer))
		sendBufferIndexes := make([]int, 0, len(buffer))
		sendClaims := make([]dedup.Claim, 0, len(buffer))
		for bufferIndex, event := range buffer {
			if h.isDedupEnabled() {
				switch claims[bufferIndex].Status {
				case dedup.ClaimDuplicateCommitted:
					if err := stream.Send(streamFlowResponse(event.Header.EventId, pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED, "DEDUP_COMMITTED", "TENANT_PROBE_EVENT")); err != nil {
						return err
					}
					totalAccepted++
					totalDeduped++
					continue
				case dedup.ClaimInFlight:
					if err := stream.Send(streamFlowResponse(event.Header.EventId, pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, "DEDUP_CLAIM_IN_FLIGHT", "TENANT_PROBE_EVENT")); err != nil {
						return err
					}
					continue
				}
			}
			sendEvents = append(sendEvents, event)
			sendBufferIndexes = append(sendBufferIndexes, bufferIndex)
			sendClaims = append(sendClaims, claims[bufferIndex])
		}

		if len(sendEvents) == 0 {
			buffer = buffer[:0]
			return nil
		}

		kafkaStart := time.Now()
		var writeResult queue.BatchWriteResult
		var err error
		if !h.writerScopeAllows(tenantID, probeID) {
			err = fmt.Errorf("flow writer is not enabled for authenticated canary scope")
			writeResult.Items = make([]queue.FlowWriteItemResult, len(sendEvents))
			for i, event := range sendEvents {
				writeResult.Items[i] = queue.FlowWriteItemResult{InputIndex: i, EventID: event.Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, ReasonCode: "CANARY_SCOPE_NOT_ACTIVE", AckScope: "TENANT_PROBE_EVENT"}
			}
		} else if h.writeFlowEvents == nil {
			err = fmt.Errorf("flow producer is not configured")
			writeResult.Items = make([]queue.FlowWriteItemResult, len(sendEvents))
			for i, event := range sendEvents {
				writeResult.Items[i] = queue.FlowWriteItemResult{InputIndex: i, EventID: event.Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, ReasonCode: "KAFKA_NOT_CONFIGURED", AckScope: "KAFKA_RECORD"}
			}
		} else {
			writeResult, err = h.writeFlowEvents(ctx, sendEvents)
		}
		h.metrics.RecordKafkaLatency(config.TopicFlowEvents, time.Since(kafkaStart))
		if exactErr := writeResult.ValidateExactSet(len(sendEvents)); exactErr != nil {
			if h.isDedupEnabled() {
				h.deduper.ReleaseBatch(ctx, sendClaims)
			}
			return status.Errorf(codes.Internal, "producer result contract violated: %v", exactErr)
		}

		if err != nil {
			logger.Error("Failed to flush stream buffer",
				zap.Int("count", len(sendEvents)),
				zap.Error(err))
			h.metrics.RecordKafkaError()

			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendFlowEvents(ctx, sendEvents, err)
			}
		}

		committedClaims := make([]dedup.Claim, 0, len(sendClaims))
		releasedClaims := make([]dedup.Claim, 0, len(sendClaims))
		for _, item := range writeResult.Items {
			event := buffer[sendBufferIndexes[item.InputIndex]]
			if sendErr := stream.Send(streamFlowResponse(event.Header.EventId, item.Disposition, item.ReasonCode, item.AckScope)); sendErr != nil {
				if h.isDedupEnabled() {
					h.deduper.ReleaseBatch(ctx, sendClaims)
				}
				return sendErr
			}
			if item.Disposition == pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED {
				totalAccepted++
				committedClaims = append(committedClaims, sendClaims[item.InputIndex])
			} else {
				releasedClaims = append(releasedClaims, sendClaims[item.InputIndex])
			}
		}
		if h.isDedupEnabled() {
			if commitErr := h.deduper.CommitBatch(ctx, committedClaims); commitErr != nil {
				logger.Error("Kafka ACK committed but stream dedup projection failed", zap.Error(commitErr))
			}
			h.deduper.ReleaseBatch(ctx, releasedClaims)
		}

		h.metrics.RecordFlowEvents(tenantID, int64(len(committedClaims)))
		buffer = buffer[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := flushBuffer(); err != nil {
				logger.Error("Failed to flush on context done", zap.Error(err))
			}
			logger.Info("Stream ended (context done)",
				zap.Int64("total_received", totalReceived),
				zap.Int64("total_accepted", totalAccepted),
				zap.Int64("total_deduped", totalDeduped))
			return ctx.Err()

		case <-ticker.C:
			if err := flushBuffer(); err != nil {
				return err
			}

		case err := <-errChan:
			if err == io.EOF {
				if flushErr := flushBuffer(); flushErr != nil {
					logger.Error("Failed to flush on EOF", zap.Error(flushErr))
				}
				logger.Info("Stream ended (EOF)",
					zap.Int64("total_received", totalReceived),
					zap.Int64("total_accepted", totalAccepted),
					zap.Int64("total_deduped", totalDeduped))
				return nil
			}
			logger.Error("Stream receive error", zap.Error(err))
			return err

		case event, ok := <-eventChan:
			if !ok {
				if err := flushBuffer(); err != nil {
					logger.Error("Failed to flush on channel close", zap.Error(err))
				}
				return nil
			}

			totalReceived++
			atomic.AddInt64(&h.totalEventsReceived, 1)

			if err := bindFlowEventIdentity(event, tenantID, probeID); err != nil {
				return status.Error(codes.PermissionDenied, err.Error())
			}
			if event.Header.IngestTs == 0 {
				event.Header.IngestTs = time.Now().UnixMilli()
			}
			if event.Header.FeatureSetId == "" {
				event.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
			}

			if err := h.validateFlowEvent(event); err != nil {
				if sendErr := stream.Send(streamFlowResponse(event.Header.EventId, pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_REJECTED_INVALID, "FLOW_VALIDATION_FAILED", "INPUT_ITEM")); sendErr != nil {
					return sendErr
				}
				continue
			}

			buffer = append(buffer, event)

			if len(buffer) >= h.handlerConfig.StreamBufferSize {
				if err := flushBuffer(); err != nil {
					return err
				}
			}
		}
	}
}

func streamFlowResponse(eventID string, disposition pb.FlowItemDisposition, reasonCode, ackScope string) *pb.StreamFlowsResponse {
	accepted := disposition == pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED ||
		disposition == pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED
	response := &pb.StreamFlowsResponse{
		EventId:          eventID,
		Accepted:         accepted,
		Disposition:      disposition,
		ReasonCode:       reasonCode,
		AckScope:         ackScope,
		ResponseRevision: 1,
	}
	if !accepted {
		response.Error = reasonCode
	}
	return response
}

func (h *IngestHandler) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.heartbeat")
	defer span.End()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	if req != nil {
		if tenantID != "" && req.TenantId != "" && tenantID != req.TenantId {
			return nil, status.Error(codes.PermissionDenied, "heartbeat tenant identity mismatch")
		}
		if probeID != "" && req.ProbeId != "" && probeID != req.ProbeId {
			return nil, status.Error(codes.PermissionDenied, "heartbeat probe identity mismatch")
		}
	}
	if probeID == "" && req != nil {
		probeID = req.ProbeId
	}
	if tenantID == "" && req != nil {
		tenantID = req.TenantId
	}

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)

	logger := logging.L(ctx)

	logger.Debug("Heartbeat received",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID))

	if req != nil && h.probeRegistry != nil && tenantID != "" && probeID != "" {
		if err := h.probeRegistry.Heartbeat(ctx, tenantID, probeID); err != nil {
			logger.Warn("Failed to persist probe heartbeat", zap.Error(err))
			return nil, status.Error(codes.Unavailable, "probe heartbeat persistence unavailable")
		}
	}

	if req != nil && req.Status != nil {
		h.probeStatus.Store(probeID, &probeStatusEntry{
			Status:    req.Status,
			UpdatedAt: time.Now(),
		})
		h.metrics.RecordProbeStatus(probeID, req.Status)
	}

	response := &pb.HeartbeatResponse{
		Ok: true,
	}

	h.controlBridgeMu.RLock()
	controlBridge := h.controlBridge
	h.controlBridgeMu.RUnlock()
	if controlBridge != nil && tenantID != "" && probeID != "" {
		var acks []*pb.ProbeOperationAck
		if req != nil {
			acks = req.OperationAcks
		}
		commands, acceptedAckIDs, err := controlBridge.Exchange(
			ctx, tenantID, probeID, acks,
		)
		if err != nil {
			logger.Warn("Probe control exchange failed", zap.Error(err))
			return nil, status.Error(codes.Unavailable, "probe control exchange unavailable")
		}
		response.OperationCommands = commands
		response.AcceptedAckOperationIds = acceptedAckIDs
	}

	if h.configManager != nil && tenantID != "" && probeID != "" {
		probeCfg, err := h.configManager.GetConfig(ctx, tenantID, probeID)
		if err != nil {
			logger.Warn("Failed to get probe config, using default",
				zap.String("probe_id", probeID),
				zap.Error(err))
			probeCfg = h.configManager.GetDefaultConfig()
		}

		if probeCfg.FeatureSetVersion == "" {
			probeCfg.FeatureSetVersion = h.defaultFeatureSetID
		}

		response.Config = probeCfg
	} else {

		response.Config = &pb.ProbeConfig{
			ConfigVersion:     "default",
			SampleRate:        1.0,
			IdleTimeoutSec:    60,
			ActiveTimeoutSec:  300,
			BatchSize:         1000,
			FeatureSetVersion: h.defaultFeatureSetID,
		}
	}

	return response, nil
}

func (h *IngestHandler) GetProbeStatus(probeID string) *pb.ProbeStatus {
	if v, ok := h.probeStatus.Load(probeID); ok {
		entry := v.(*probeStatusEntry)
		if time.Since(entry.UpdatedAt) < h.handlerConfig.ProbeStatusTimeout {
			return entry.Status
		}
		h.probeStatus.Delete(probeID)
	}
	return nil
}

func (h *IngestHandler) GetAllProbeStatus() map[string]*pb.ProbeStatus {
	result := make(map[string]*pb.ProbeStatus)
	threshold := time.Now().Add(-h.handlerConfig.ProbeStatusTimeout)

	h.probeStatus.Range(func(key, value interface{}) bool {
		entry := value.(*probeStatusEntry)
		if entry.UpdatedAt.After(threshold) {
			result[key.(string)] = entry.Status
		}
		return true
	})

	return result
}

func (h *IngestHandler) RegisterProbe(ctx context.Context, req *pb.RegisterProbeRequest) (*pb.RegisterProbeResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.register_probe")
	defer span.End()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	// 身份只来自认证上下文(interceptor 从 mTLS 证书 CN / token claims 注入)。
	// 禁止回退到请求体中的 tenant/probe:无认证上下文时直接拒绝注册,
	// 防止 ALLOW_NO_TOKEN 等部署下任意客户端注册任意租户探针。
	if tenantID == "" || probeID == "" {
		h.metrics.RecordReject401()
		return nil, status.Error(codes.Unauthenticated, "registration requires an authenticated probe identity")
	}

	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	if req == nil {
		h.metrics.RecordReject400()
		return &pb.RegisterProbeResponse{
			Success: false,
			Message: "empty request",
		}, nil
	}

	if req.ProbeId == "" {
		h.metrics.RecordReject400()
		return &pb.RegisterProbeResponse{
			Success: false,
			Message: "probe_id is required",
		}, nil
	}

	if req.TenantId == "" {
		h.metrics.RecordReject400()
		return &pb.RegisterProbeResponse{
			Success: false,
			Message: "tenant_id is required",
		}, nil
	}
	if tenantID != "" && tenantID != req.TenantId {
		return nil, status.Error(codes.PermissionDenied, "registration tenant identity mismatch")
	}
	if probeID != "" && probeID != req.ProbeId {
		return nil, status.Error(codes.PermissionDenied, "registration probe identity mismatch")
	}

	logger.Info("Probe registration request received",
		zap.String("tenant_id", req.TenantId),
		zap.String("probe_id", req.ProbeId),
		zap.String("software_version", req.SoftwareVersion),
		zap.String("build_commit", req.BuildCommit))

	if req.Hardware != nil {
		logger.Info("Probe hardware info",
			zap.String("probe_id", req.ProbeId),
			zap.String("cpu_model", req.Hardware.CpuModel),
			zap.Uint32("cpu_cores", req.Hardware.CpuCores),
			zap.Uint64("memory_mb", req.Hardware.MemoryMb),
			zap.String("os_version", req.Hardware.OsVersion),
			zap.Int("nic_count", len(req.Hardware.Nics)))
	}
	if h.probeRegistry != nil {
		if err := h.probeRegistry.Register(
			ctx,
			req.TenantId,
			req.ProbeId,
			req.SoftwareVersion,
			req.BuildCommit,
			req.Hardware,
		); err != nil {
			logger.Warn("Failed to persist probe registration", zap.Error(err))
			return nil, status.Error(codes.Unavailable, "probe registration persistence unavailable")
		}
	}

	h.probeStatus.Store(req.ProbeId, &probeStatusEntry{
		Status: &pb.ProbeStatus{
			CpuUsage:    0,
			MemoryUsage: 0,
			CapturePps:  0,
			UploadBps:   0,
		},
		UpdatedAt: time.Now(),
	})

	var initialConfig *pb.ProbeConfig
	if h.configManager != nil {
		cfg, err := h.configManager.GetConfig(ctx, req.TenantId, req.ProbeId)
		if err != nil {
			logger.Warn("Failed to get probe config, using default",
				zap.String("probe_id", req.ProbeId),
				zap.Error(err))
			initialConfig = h.configManager.GetDefaultConfig()
		} else {
			initialConfig = cfg
		}
	} else {

		initialConfig = &pb.ProbeConfig{
			ConfigVersion:     "default",
			SampleRate:        1.0,
			IdleTimeoutSec:    60,
			ActiveTimeoutSec:  300,
			BatchSize:         1000,
			FeatureSetVersion: h.defaultFeatureSetID,
		}
	}

	if initialConfig.FeatureSetVersion == "" {
		initialConfig.FeatureSetVersion = h.defaultFeatureSetID
	}

	logger.Info("Probe registered successfully",
		zap.String("tenant_id", req.TenantId),
		zap.String("probe_id", req.ProbeId),
		zap.String("config_version", initialConfig.ConfigVersion))

	return &pb.RegisterProbeResponse{
		Success:       true,
		Message:       "probe registered successfully",
		InitialConfig: initialConfig,
	}, nil
}
