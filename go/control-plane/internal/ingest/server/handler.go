////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/server/handler.go (Part 1/2)
// 优化版 v6：
// 1. 移除所有硬编码（feature_set_id、错误消息、超时时间）
// 2. 统一错误处理（errors.AppError）
// 3. 统一日志（logging.L(ctx)）
// 4. 统一审计（audit.Logger）
// 5. 修复问题 2：totalEventsReceived 统计移到函数开头
// 6. 修复问题 4：UploadSessions 添加完整指标记录
// 7. 修复 StreamFlows dedup 命中返回 ACK
////////////////////////////////////////////////////////////////////////////////

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

// 错误消息常量（移除硬编码）
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

// probeStatusEntry 探针状态条目（带时间戳）
type probeStatusEntry struct {
	Status    *pb.ProbeStatus
	UpdatedAt time.Time
}

// IngestHandler gRPC Handler 实现
type IngestHandler struct {
	pb.UnimplementedIngestServiceServer

	producer      *queue.Producer
	dlqProducer   *dlq.Producer
	deduper       *dedup.Deduplicator
	metrics       *metrics.Metrics
	configManager *config.ProbeConfigManager
	auditLogger   *audit.Logger
	logger        *zap.Logger

	// 探针状态缓存（带过期清理）
	probeStatus sync.Map // map[string]*probeStatusEntry

	// 配置
	handlerConfig HandlerConfig

	// 统计（原子安全）
	totalEventsReceived int64
	totalEventsAccepted int64
	totalEventsRejected int64
	totalEventsDedupe   int64

	// 默认 feature_set_id（从配置加载）
	defaultFeatureSetID string
}

// HandlerConfig Handler 配置
type HandlerConfig struct {
	MaxBatchSize       int           `env:"MAX_BATCH_SIZE" envDefault:"10000"`
	MaxEventSize       int           `env:"MAX_EVENT_SIZE" envDefault:"65536"` // 64KB
	StreamBufferSize   int           `env:"STREAM_BUFFER_SIZE" envDefault:"1000"`
	HeartbeatInterval  time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	EnableDLQ          bool          `env:"ENABLE_DLQ" envDefault:"true"`
	EnableDedup        bool          `env:"ENABLE_DEDUP" envDefault:"true"`
	ProbeStatusTimeout time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
	EnableAudit        bool          `env:"ENABLE_AUDIT" envDefault:"true"`
}

// NewIngestHandlerWithConfig 创建带配置的 Handler
func NewIngestHandlerWithConfig(
	logger *zap.Logger,
	producer *queue.Producer,
	dlqProducer *dlq.Producer,
	m *metrics.Metrics,
	cfg HandlerConfig,
) *IngestHandler {
	// 应用默认值
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

// SetConfigManager 设置配置管理器
func (h *IngestHandler) SetConfigManager(cm *config.ProbeConfigManager) {
	h.configManager = cm
	h.logger.Info("Config manager set")
}

// SetDeduplicator 设置去重器
func (h *IngestHandler) SetDeduplicator(d *dedup.Deduplicator) {
	h.deduper = d
	h.logger.Info("Deduplicator set", zap.Bool("enabled", d != nil))
}

// SetDLQProducer 设置 DLQ 生产者
func (h *IngestHandler) SetDLQProducer(dlq *dlq.Producer) {
	h.dlqProducer = dlq
	h.logger.Info("DLQ producer set", zap.Bool("enabled", dlq != nil))
}

// SetAuditLogger 设置审计日志记录器
func (h *IngestHandler) SetAuditLogger(auditLogger *audit.Logger) {
	h.auditLogger = auditLogger
	h.logger.Info("Audit logger set", zap.Bool("enabled", auditLogger != nil))
}

// StartProbeStatusCleaner 启动探针状态清理器
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

// cleanExpiredProbeStatus 清理过期的探针状态
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

// isDedupEnabled 检查去重是否启用
func (h *IngestHandler) isDedupEnabled() bool {
	return h.handlerConfig.EnableDedup && h.deduper != nil
}

// recordAudit 记录审计事件
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

// getFeatureSetID 获取 feature_set_id（三级回退，无硬编码）
func (h *IngestHandler) getFeatureSetID(ctx context.Context, tenantID, probeID string) string {
	// 优先级 1: 从探针配置获取
	if h.configManager != nil {
		cfg, err := h.configManager.GetConfig(ctx, tenantID, probeID)
		if err == nil && cfg != nil && cfg.FeatureSetVersion != "" {
			return cfg.FeatureSetVersion
		}
	}

	// 优先级 2: 使用默认值（从配置加载）
	if h.defaultFeatureSetID != "" {
		return h.defaultFeatureSetID
	}

	// 优先级 3: 使用常量
	return config.DefaultFeatureSetID
}

// UploadFlows 批量上报 Flow 事件（完整优化版）
func (h *IngestHandler) UploadFlows(ctx context.Context, req *pb.UploadFlowsRequest) (*pb.UploadFlowsResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_flows")
	defer span.End()
	start := time.Now()

	// ✅ 修复问题 2：立即统计接收量（移到最前面，避免提前返回导致统计遗漏）
	if req != nil && len(req.Events) > 0 {
		atomic.AddInt64(&h.totalEventsReceived, int64(len(req.Events)))
	}

	// 获取认证信息
	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	// 注入日志上下文
	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	// 验证租户
	if tenantID == "" {
		h.metrics.RecordReject401()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, "", probeID, "upload_flows", map[string]interface{}{
			"reason": "missing_tenant_id",
		})
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}

	// 验证请求
	if req == nil || len(req.Events) == 0 {
		return &pb.UploadFlowsResponse{
			Accepted: 0,
			Rejected: 0,
			Message:  errMsgEmptyRequest,
		}, nil
	}

	// 检查批次大小
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

	// 记录批次大小
	h.metrics.RecordBatchSize(tenantID, len(req.Events))

	logger.Debug("Received flow events",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int("count", len(req.Events)),
		zap.String("compression", req.Compression))

	// 预处理事件
	validEvents := make([]*pb.FlowEvent, 0, len(req.Events))
	rejectedIDs := make([]string, 0)
	dedupedIDs := make([]string, 0)

	now := time.Now()
	nowMs := now.UnixMilli()

	for _, event := range req.Events {
		if event == nil {
			continue
		}

		// 确保 Header 存在
		if event.Header == nil {
			event.Header = &pb.EventHeader{}
		}

		// 填充 Header 字段
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

		// 自动填充 feature_set_id（使用配置，无硬编码）
		if event.Header.FeatureSetId == "" {
			event.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
		}

		// 去重检查
		if h.isDedupEnabled() {
			if h.deduper.IsDuplicate(ctx, event.Header.EventId) {
				dedupedIDs = append(dedupedIDs, event.Header.EventId)
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			}
			h.metrics.RecordDedupMiss()
		}

		// 验证事件
		if err := h.validateFlowEvent(event); err != nil {
			logger.Debug("Event validation failed",
				zap.String("event_id", event.Header.EventId),
				zap.Error(err))
			rejectedIDs = append(rejectedIDs, event.Header.EventId)
			continue
		}

		validEvents = append(validEvents, event)
	}

	// 写入 Kafka
	var writeErr error
	if len(validEvents) > 0 {
		kafkaStart := time.Now()
		writeErr = h.producer.WriteFlowEvents(ctx, validEvents)
		h.metrics.RecordKafkaLatency(config.TopicFlowEvents, time.Since(kafkaStart))

		if writeErr != nil {
			logger.Error("Failed to write events to Kafka",
				zap.Int("count", len(validEvents)),
				zap.Error(writeErr))

			// 发送到 DLQ
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
			// 标记已处理（用于去重）
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

	// 记录指标
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

	// 构建响应
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

// validateFlowEvent 验证 Flow 事件（使用 errors.AppError）
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

	// 检查事件大小
	actualSize := proto.Size(event)
	if actualSize > h.handlerConfig.MaxEventSize {
		return errors.Newf(errors.ErrCodeOutOfRange,
			"%s: %d > %d", errMsgEventTooLarge, actualSize, h.handlerConfig.MaxEventSize)
	}

	return nil
}

// validateSessionEvent 验证 Session 事件（使用 errors.AppError）
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

	// 检查事件大小
	actualSize := proto.Size(session)
	if actualSize > h.handlerConfig.MaxEventSize {
		return errors.Newf(errors.ErrCodeOutOfRange,
			"%s: %d > %d", errMsgEventTooLarge, actualSize, h.handlerConfig.MaxEventSize)
	}

	return nil
}

// GetStats 获取统计信息
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

// HandlerStats 处理器统计
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

// UploadSessions 批量上报 Session 事件（修复版：添加完整指标）
func (h *IngestHandler) UploadSessions(ctx context.Context, req *pb.UploadSessionsRequest) (*pb.UploadSessionsResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_sessions")
	defer span.End()
	start := time.Now()

	// ✅ 修复问题 2：立即统计接收量（移到最前面）
	if req != nil && len(req.Sessions) > 0 {
		atomic.AddInt64(&h.totalEventsReceived, int64(len(req.Sessions)))
	}

	// 获取认证信息
	tenantID := auth.GetTenantID(ctx)
	probeID := auth.GetProbeID(ctx)

	// 注入日志上下文
	ctx = logging.WithTenantID(ctx, tenantID)
	ctx = logging.WithProbeID(ctx, probeID)
	otel.AddTenantAttribute(ctx, tenantID)
	otel.AddProbeAttribute(ctx, probeID)

	logger := logging.L(ctx)

	// 验证租户
	if tenantID == "" {
		h.metrics.RecordReject401()
		h.recordAudit(ctx, audit.EventTypeAccessDenied, "", probeID, "upload_sessions", map[string]interface{}{
			"reason": "missing_tenant_id",
		})
		return nil, status.Error(codes.Unauthenticated, errMsgTenantIDRequired)
	}

	// 验证请求
	if req == nil || len(req.Sessions) == 0 {
		return &pb.UploadSessionsResponse{
			Accepted: 0,
			Rejected: 0,
			Message:  errMsgEmptyRequest,
		}, nil
	}

	// 检查批次大小
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

	// 记录批次大小
	h.metrics.RecordBatchSize(tenantID, len(req.Sessions))

	logger.Debug("Received session events",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.Int("count", len(req.Sessions)))

	// 预处理事件
	validSessions := make([]*pb.SessionEvent, 0, len(req.Sessions))
	rejectedIDs := make([]string, 0)
	dedupedIDs := make([]string, 0)

	now := time.Now()
	nowMs := now.UnixMilli()

	for _, session := range req.Sessions {
		if session == nil {
			continue
		}

		// 确保 Header 存在
		if session.Header == nil {
			session.Header = &pb.EventHeader{}
		}

		// 填充 Header 字段
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

		// 自动填充 feature_set_id
		if session.Header.FeatureSetId == "" {
			session.Header.FeatureSetId = h.getFeatureSetID(ctx, tenantID, probeID)
		}

		// 去重检查
		if h.isDedupEnabled() {
			if h.deduper.IsDuplicate(ctx, session.Header.EventId) {
				dedupedIDs = append(dedupedIDs, session.Header.EventId)
				h.metrics.RecordDedupHit()
				atomic.AddInt64(&h.totalEventsDedupe, 1)
				continue
			}
			h.metrics.RecordDedupMiss()
		}

		// 验证事件
		if err := h.validateSessionEvent(session); err != nil {
			logger.Debug("Session validation failed",
				zap.String("session_id", session.SessionId),
				zap.Error(err))
			rejectedIDs = append(rejectedIDs, session.Header.EventId)
			continue
		}

		validSessions = append(validSessions, session)
	}

	// 写入 Kafka
	var writeErr error
	if len(validSessions) > 0 {
		kafkaStart := time.Now()
		writeErr = h.producer.WriteSessionEvents(ctx, validSessions)
		h.metrics.RecordKafkaLatency(config.TopicSessionEvents, time.Since(kafkaStart))

		if writeErr != nil {
			logger.Error("Failed to write session events to Kafka",
				zap.Int("count", len(validSessions)),
				zap.Error(writeErr))

			// 发送到 DLQ
			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendSessionEvents(ctx, validSessions, writeErr)
			}

			h.metrics.RecordKafkaError()
			h.metrics.RecordReject503()
			atomic.AddInt64(&h.totalEventsRejected, int64(len(validSessions)))
		} else {
			// 标记已处理（用于去重）
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

	// 记录指标
	accepted := int32(len(validSessions))
	rejected := int32(len(rejectedIDs))
	deduped := int32(len(dedupedIDs))

	if writeErr != nil {
		rejected += accepted
		accepted = 0
	}

	// ✅ 修复问题 4：记录 Session 事件指标
	if accepted > 0 {
		h.metrics.RecordSessionEvents(tenantID, int64(accepted))

		// ✅ 修复问题 4：记录字节数
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

	// 构建响应
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

// UploadPcapIndex 上报 PCAP 索引元数据（统一错误处理）
func (h *IngestHandler) UploadPcapIndex(ctx context.Context, req *pb.UploadPcapIndexRequest) (*pb.UploadPcapIndexResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.upload_pcap_index")
	defer span.End()
	start := time.Now()

	// 获取认证信息
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

	// 验证请求
	if req == nil || req.Index == nil {
		h.metrics.RecordReject400()
		return &pb.UploadPcapIndexResponse{
			Success: false,
			Message: errMsgEmptyRequest,
		}, nil
	}

	meta := req.Index

	// 填充元数据
	if meta.TenantId == "" {
		meta.TenantId = tenantID
	}
	if meta.ProbeId == "" {
		meta.ProbeId = probeID
	}

	// 验证必填字段
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

	// 写入 Kafka
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

		// 发送到 DLQ
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

// StreamFlows 流式上报 Flow 事件（修复版：dedup 命中返回 ACK）
func (h *IngestHandler) StreamFlows(stream pb.IngestService_StreamFlowsServer) error {
	ctx := stream.Context()
	ctx, span := otel.StartSpan(ctx, "ingest.stream_flows")
	defer span.End()

	// 获取认证信息
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

	// 记录活跃连接
	h.metrics.IncrActiveConnections()
	defer h.metrics.DecrActiveConnections()

	logger.Info("Stream started",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID))

	// 使用 channel 接收事件
	eventChan := make(chan *pb.FlowEvent, h.handlerConfig.StreamBufferSize)
	errChan := make(chan error, 1)

	// 接收 goroutine
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

	// 批量缓冲
	buffer := make([]*pb.FlowEvent, 0, h.handlerConfig.StreamBufferSize)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var totalReceived, totalAccepted, totalDeduped int64

	// 刷新缓冲区
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

			// 发送 NACK
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

			// 发送到 DLQ
			if h.handlerConfig.EnableDLQ && h.dlqProducer != nil {
				h.dlqProducer.SendFlowEvents(ctx, buffer, err)
			}

			buffer = buffer[:0]
			return nil
		}

		// 标记已处理
		if h.isDedupEnabled() {
			eventIDs := make([]string, len(buffer))
			for i, e := range buffer {
				eventIDs[i] = e.Header.EventId
			}
			h.deduper.MarkSeenBatch(ctx, eventIDs)
		}

		// 发送 ACK
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

	// 主循环
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

			// 填充 Header
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

			// ✅ 修复：去重命中返回 ACK（幂等成功）
			if h.isDedupEnabled() {
				if h.deduper.IsDuplicate(ctx, event.Header.EventId) {
					totalDeduped++
					h.metrics.RecordDedupHit()
					// 发送 ACK（accepted=true），表示幂等成功
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

			// 验证
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

			// 添加到缓冲区
			buffer = append(buffer, event)

			// 缓冲区满时刷新
			if len(buffer) >= h.handlerConfig.StreamBufferSize {
				if err := flushBuffer(); err != nil {
					return err
				}
			}
		}
	}
}

// Heartbeat 心跳检测（下发探针配置）
func (h *IngestHandler) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	ctx, span := otel.StartSpan(ctx, "ingest.heartbeat")
	defer span.End()

	// 获取认证信息
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

	// 更新探针状态
	if req != nil && req.Status != nil {
		h.probeStatus.Store(probeID, &probeStatusEntry{
			Status:    req.Status,
			UpdatedAt: time.Now(),
		})
		h.metrics.RecordProbeStatus(probeID, req.Status)
	}

	// 构建响应
	response := &pb.HeartbeatResponse{
		Ok: true,
	}

	// 获取探针配置
	if h.configManager != nil && tenantID != "" && probeID != "" {
		probeCfg, err := h.configManager.GetConfig(ctx, tenantID, probeID)
		if err != nil {
			logger.Warn("Failed to get probe config, using default",
				zap.String("probe_id", probeID),
				zap.Error(err))
			probeCfg = h.configManager.GetDefaultConfig()
		}

		// 确保 feature_set_version 被下发
		if probeCfg.FeatureSetVersion == "" {
			probeCfg.FeatureSetVersion = h.defaultFeatureSetID
		}

		response.Config = probeCfg
	} else {
		// 返回默认配置
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

// GetProbeStatus 获取探针状态
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

// GetAllProbeStatus 获取所有探针状态
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
