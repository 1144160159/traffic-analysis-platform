package server

import (
	"context"
	"fmt"
	"io"
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
	errMsgFileKeyRequired     = "file_key is required"
	errMsgEventNil            = "event is nil"
	errMsgHeaderNil           = "event header is nil"
	errMsgTupleNil            = "tuple is nil"
	errMsgIPRequired          = "src_ip and dst_ip are required"
	errMsgSessionIDRequired   = "session_id is required"
	errMsgCommunityIDRequired = "community_id is required"
	errMsgEventTooLarge       = "event size exceeds maximum"

	msgSuccess              = "success"
	msgPartialFailure       = "partial failure: %s"
	msgRejectedDeduplicated = "%d rejected, %d deduplicated"
)

type probeStatusEntry struct {
	Status    *pb.ProbeStatus
	UpdatedAt time.Time
}

type IngestHandler struct {
	pb.UnimplementedIngestServiceServer

	producer      *queue.Producer
	dlqProducer   *dlq.Producer
	deduper       *dedup.Deduplicator
	metrics       *metrics.Metrics
	configManager *config.ProbeConfigManager
	auditLogger   *audit.Logger
	logger        *zap.Logger

	probeStatus sync.Map

	handlerConfig HandlerConfig

	totalEventsReceived int64
	totalEventsAccepted int64
	totalEventsRejected int64
	totalEventsDedupe   int64

	defaultFeatureSetID string
}

type HandlerConfig struct {
	MaxBatchSize       int           `env:"MAX_BATCH_SIZE" envDefault:"10000"`
	MaxEventSize       int           `env:"MAX_EVENT_SIZE" envDefault:"65536"`
	StreamBufferSize   int           `env:"STREAM_BUFFER_SIZE" envDefault:"1000"`
	HeartbeatInterval  time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	EnableDLQ          bool          `env:"ENABLE_DLQ" envDefault:"true"`
	EnableDedup        bool          `env:"ENABLE_DEDUP" envDefault:"true"`
	ProbeStatusTimeout time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
	EnableAudit        bool          `env:"ENABLE_AUDIT" envDefault:"true"`
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

	logger.Info("Handler initialized",
		zap.Bool("enable_dedup", cfg.EnableDedup),
		zap.Bool("enable_dlq", cfg.EnableDLQ),
		zap.Bool("enable_audit", cfg.EnableAudit),
		zap.Duration("probe_status_timeout", cfg.ProbeStatusTimeout),
		zap.String("default_feature_set_id", h.defaultFeatureSetID))

	return h
}

func (h *IngestHandler) SetConfigManager(cm *config.ProbeConfigManager) {
	h.configManager = cm
	h.logger.Info("Config manager set")
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

	validEvents := make([]*pb.FlowEvent, 0, len(req.Events))
	rejectedIDs := make([]string, 0)
	dedupedIDs := make([]string, 0)

	now := time.Now()
	nowMs := now.UnixMilli()

	for _, event := range req.Events {
		if event == nil {
			continue
		}

		if event.Header == nil {
			event.Header = &pb.EventHeader{}
		}

		if event.Header.EventId == "" {
			event.Header.EventId = uuid.New().String()
		}
		if event.Header.TenantId == "" {
			event.Header.TenantId = tenantID
		}
		if event.Header.ProbeId == "" {
			event.Header.ProbeId = probeID
		}
		if event.Header.EventTs == 0 {
			event.Header.EventTs = nowMs
		}
		if event.Header.IngestTs == 0 {
			event.Header.IngestTs = nowMs
		}

		if event.Header.FeatureSetId == "" {
			event.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
		}

		if h.isDedupEnabled() {
			if h.deduper.IsDuplicate(ctx, event.Header.EventId) {
				dedupedIDs = append(dedupedIDs, event.Header.EventId)
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			}
			h.metrics.RecordDedupMiss()
		}

		if err := h.validateFlowEvent(event); err != nil {
			logger.Debug("Event validation failed",
				zap.String("event_id", event.Header.EventId),
				zap.Error(err))
			rejectedIDs = append(rejectedIDs, event.Header.EventId)
			continue
		}

		validEvents = append(validEvents, event)
	}

	var writeErr error
	if len(validEvents) > 0 {
		kafkaStart := time.Now()
		writeErr = h.producer.WriteFlowEvents(ctx, validEvents)
		h.metrics.RecordKafkaLatency(config.TopicFlowEvents, time.Since(kafkaStart))

		if writeErr != nil {
			logger.Error("Failed to write events to Kafka",
				zap.Int("count", len(validEvents)),
				zap.Error(writeErr))

			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendFlowEvents(ctx, validEvents, writeErr)
			}

			h.metrics.RecordKafkaError()
			h.metrics.RecordReject503()
			atomic.AddInt64(&h.totalEventsRejected, int64(len(validEvents)))

			h.recordAudit(ctx, audit.EventTypeSystemError, tenantID, probeID, "upload_flows", map[string]interface{}{
				"error": writeErr.Error(),
				"count": len(validEvents),
			})
		} else {

			if h.isDedupEnabled() {
				eventIDs := make([]string, len(validEvents))
				for i, e := range validEvents {
					eventIDs[i] = e.Header.EventId
				}
				h.deduper.MarkSeenBatch(ctx, eventIDs)
			}
			atomic.AddInt64(&h.totalEventsAccepted, int64(len(validEvents)))
		}
	}

	accepted := int32(len(validEvents))
	rejected := int32(len(rejectedIDs))
	deduped := int32(len(dedupedIDs))

	if writeErr != nil {
		rejected += accepted
		accepted = 0
	}

	h.metrics.RecordFlowEvents(tenantID, int64(accepted))
	if rejected > 0 {
		h.metrics.RecordFlowEventsRejected(tenantID, int64(rejected))
	}

	h.metrics.RecordLatency("upload_flows", time.Since(start))

	response := &pb.UploadFlowsResponse{
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

	logger.Info("Flow events processed",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int32("accepted", accepted),
		zap.Int32("rejected", rejected),
		zap.Int32("deduped", deduped),
		zap.Duration("duration", time.Since(start)))

	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_flows", map[string]interface{}{
		"accepted": accepted,
		"rejected": rejected,
		"deduped":  deduped,
	})

	return response, nil
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

		if h.isDedupEnabled() {
			if h.deduper.IsDuplicate(ctx, session.Header.EventId) {
				dedupedIDs = append(dedupedIDs, session.Header.EventId)
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			}
			h.metrics.RecordDedupMiss()
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

	var writeErr error
	if len(validSessions) > 0 {
		kafkaStart := time.Now()
		writeErr = h.producer.WriteSessionEvents(ctx, validSessions)
		h.metrics.RecordKafkaLatency(config.TopicSessionEvents, time.Since(kafkaStart))

		if writeErr != nil {
			logger.Error("Failed to write session events to Kafka",
				zap.Int("count", len(validSessions)),
				zap.Error(writeErr))

			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendSessionEvents(ctx, validSessions, writeErr)
			}

			h.metrics.RecordKafkaError()
			h.metrics.RecordReject503()
			atomic.AddInt64(&h.totalEventsRejected, int64(len(validSessions)))
		} else {

			if h.isDedupEnabled() {
				eventIDs := make([]string, len(validSessions))
				for i, s := range validSessions {
					eventIDs[i] = s.Header.EventId
				}
				h.deduper.MarkSeenBatch(ctx, eventIDs)
			}
			atomic.AddInt64(&h.totalEventsAccepted, int64(len(validSessions)))
		}
	}

	accepted := int32(len(validSessions))
	rejected := int32(len(rejectedIDs))
	deduped := int32(len(dedupedIDs))

	if writeErr != nil {
		rejected += accepted
		accepted = 0
	}

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

	if req == nil || req.Index == nil {
		h.metrics.RecordReject400()
		return &pb.UploadPcapIndexResponse{
			Success: false,
			Message: errMsgEmptyRequest,
		}, nil
	}

	meta := req.Index

	if meta.TenantId == "" {
		meta.TenantId = tenantID
	}
	if meta.ProbeId == "" {
		meta.ProbeId = probeID
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
	err := h.producer.WritePcapIndex(ctx, meta)
	h.metrics.RecordKafkaLatency(config.TopicPcapIndex, time.Since(kafkaStart))

	if err != nil {
		logger.Error("Failed to write PCAP index",
			zap.String("file_key", meta.FileKey),
			zap.Error(err))

		h.metrics.RecordError("pcap_index_write_failed")
		h.metrics.RecordKafkaError()
		h.metrics.RecordReject503()

		if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
			h.dlqProducer.SendPcapIndex(ctx, meta, err)
		}

		h.recordAudit(ctx, audit.EventTypeSystemError, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
			"file_key": meta.FileKey,
			"error":    err.Error(),
		})

		return &pb.UploadPcapIndexResponse{
			Success: false,
			Message: "failed to write pcap index: " + err.Error(),
		}, nil
	}

	h.metrics.RecordPcapIndex(tenantID)
	h.metrics.RecordLatency("upload_pcap_index", time.Since(start))

	logger.Info("PCAP index uploaded",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("file_key", meta.FileKey),
		zap.Duration("duration", time.Since(start)))

	h.recordAudit(ctx, audit.EventTypeDataIngested, tenantID, probeID, "upload_pcap_index", map[string]interface{}{
		"file_key": meta.FileKey,
		"size":     meta.ByteSize,
	})

	return &pb.UploadPcapIndexResponse{
		Success: true,
		Message: msgSuccess,
	}, nil
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

		kafkaStart := time.Now()
		err := h.producer.WriteFlowEvents(ctx, buffer)
		h.metrics.RecordKafkaLatency(config.TopicFlowEvents, time.Since(kafkaStart))

		if err != nil {
			logger.Error("Failed to flush stream buffer",
				zap.Int("count", len(buffer)),
				zap.Error(err))
			h.metrics.RecordKafkaError()

			for _, event := range buffer {
				if sendErr := stream.Send(&pb.StreamFlowsResponse{
					EventId:  event.Header.EventId,
					Accepted: false,
					Error:    err.Error(),
				}); sendErr != nil {
					logger.Error("Failed to send NACK", zap.Error(sendErr))
					return sendErr
				}
			}

			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendFlowEvents(ctx, buffer, err)
			}

			buffer = buffer[:0]
			return nil
		}

		if h.isDedupEnabled() {
			eventIDs := make([]string, len(buffer))
			for i, e := range buffer {
				eventIDs[i] = e.Header.EventId
			}
			h.deduper.MarkSeenBatch(ctx, eventIDs)
		}

		for _, event := range buffer {
			if sendErr := stream.Send(&pb.StreamFlowsResponse{
				EventId:  event.Header.EventId,
				Accepted: true,
			}); sendErr != nil {
				return sendErr
			}
			totalAccepted++
		}

		h.metrics.RecordFlowEvents(tenantID, int64(len(buffer)))
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

			now := time.Now()
			nowMs := now.UnixMilli()

			if event.Header == nil {
				event.Header = &pb.EventHeader{}
			}
			if event.Header.EventId == "" {
				event.Header.EventId = uuid.New().String()
			}
			if event.Header.TenantId == "" {
				event.Header.TenantId = tenantID
			}
			if event.Header.ProbeId == "" {
				event.Header.ProbeId = probeID
			}
			if event.Header.EventTs == 0 {
				event.Header.EventTs = nowMs
			}
			if event.Header.IngestTs == 0 {
				event.Header.IngestTs = nowMs
			}
			if event.Header.FeatureSetId == "" {
				event.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
			}

			if h.isDedupEnabled() {
				if h.deduper.IsDuplicate(ctx, event.Header.EventId) {
					totalDeduped++
					h.metrics.RecordDedupHit()

					if sendErr := stream.Send(&pb.StreamFlowsResponse{
						EventId:  event.Header.EventId,
						Accepted: true,
					}); sendErr != nil {
						return sendErr
					}
					continue
				}
				h.metrics.RecordDedupMiss()
			}

			if err := h.validateFlowEvent(event); err != nil {
				if sendErr := stream.Send(&pb.StreamFlowsResponse{
					EventId:  event.Header.EventId,
					Accepted: false,
					Error:    err.Error(),
				}); sendErr != nil {
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

func (h *IngestHandler) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.heartbeat")
	defer span.End()

	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

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

	if tenantID == "" && req != nil {
		tenantID = req.TenantId
	}
	if probeID == "" && req != nil {
		probeID = req.ProbeId
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

	if h.handlerConfig.EnableAudit && h.auditLogger != nil {
		h.recordAudit(ctx, audit.EventTypeProbeRegister, req.TenantId, req.ProbeId, "register_probe", map[string]interface{}{
			"software_version": req.SoftwareVersion,
			"build_commit":     req.BuildCommit,
			"cpu_cores":        req.Hardware.GetCpuCores(),
			"memory_mb":        req.Hardware.GetMemoryMb(),
		})
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
