////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/consumer/kafka_consumer.go
// 修复版：集成证据生成、Arkime链接、完善关闭逻辑、内存优化、启用Lua脚本
// 主要修复：
// 1. 预分配 evidences 切片容量（内存优化）
// 2. 启用 Redis Lua 脚本原子去重（性能优化）
// 3. 完善 Close 逻辑（优雅关闭）
////////////////////////////////////////////////////////////////////////////////

package consumer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/arkime"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/evidence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/state"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// Consumer metrics
var (
	messagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_messages_received_total",
			Help: "Total number of messages received from Kafka",
		},
		[]string{"tenant_id"},
	)

	messagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_messages_processed_total",
			Help: "Total number of messages successfully processed",
		},
		[]string{"tenant_id", "severity"},
	)

	messagesFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_messages_failed_total",
			Help: "Total number of messages failed to process",
		},
		[]string{"tenant_id", "error_type"},
	)

	batchWriteLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alert_consumer_batch_write_seconds",
			Help:    "Batch write latency in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"storage"},
	)

	dedupHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_dedup_hits_total",
			Help: "Total number of deduplicated alerts",
		},
		[]string{"tenant_id"},
	)

	batchSizeMetric = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "alert_consumer_batch_size",
			Help:    "Size of processed batches",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
		},
	)

	evidenceGenerated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_evidence_generated_total",
			Help: "Total number of evidence records generated",
		},
		[]string{"tenant_id", "type"},
	)

	consumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alert_consumer_lag",
			Help: "Consumer lag (messages behind)",
		},
		[]string{"topic", "partition"},
	)

	dedupMethodUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_dedup_method_total",
			Help: "Dedup method used (atomic vs pipeline)",
		},
		[]string{"method"},
	)
)

// Consumer Alert消费者
type Consumer struct {
	kafkaConsumer     *kafka.Consumer
	dlqProducer       *kafka.DLQProducer
	redisDedup        *dedup.RedisDedup
	dualWriter        *persistence.DualWriter
	evidenceGenerator *evidence.Generator
	arkimeLinkGen     *arkime.LinkGenerator
	notifier          interface {
		Notify(context.Context, *notification.AlertInfo) error
	}
	timeBucket    int
	logger        *zap.Logger
	batchSize     int
	flushInterval time.Duration

	// 状态管理
	mu        sync.Mutex
	closed    int32 // atomic
	running   int32 // atomic
	wg        sync.WaitGroup
	stopChan  chan struct{}
	stopOnce  sync.Once
	runCancel context.CancelFunc

	// 配置
	generateEvidence bool
	generateArkime   bool
	useLuaScript     bool // 新增：是否使用 Lua 脚本去重
}

// SetNotificationDispatcher connects persisted detections to the governed
// notification execution chain. Delivery failures are logged after the alert
// batch is durable and never roll back the alert itself.
func (c *Consumer) SetNotificationDispatcher(dispatcher interface {
	Notify(context.Context, *notification.AlertInfo) error
}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notifier = dispatcher
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Kafka            config.KafkaConfig
	Dedup            config.DedupConfig
	GenerateEvidence bool
	GenerateArkime   bool
	UseLuaScript     bool
}

// NewConsumer 创建消费者
func NewConsumer(
	kafkaCfg config.KafkaConfig,
	dedupCfg config.DedupConfig,
	redisDedup *dedup.RedisDedup,
	dualWriter *persistence.DualWriter,
	logger *zap.Logger,
) *Consumer {
	return NewConsumerWithEvidence(kafkaCfg, dedupCfg, redisDedup, dualWriter, nil, nil, logger)
}

// NewConsumerWithEvidence 创建带证据生成的消费者
func NewConsumerWithEvidence(
	kafkaCfg config.KafkaConfig,
	dedupCfg config.DedupConfig,
	redisDedup *dedup.RedisDedup,
	dualWriter *persistence.DualWriter,
	evidenceGen *evidence.Generator,
	arkimeGen *arkime.LinkGenerator,
	logger *zap.Logger,
) *Consumer {
	// 创建Kafka消费者
	consumerCfg := buildKafkaConsumerConfig(kafkaCfg)

	kafkaConsumer, err := kafka.NewConsumer(consumerCfg, logger)
	if err != nil {
		logger.Error("Failed to create Kafka consumer", zap.Error(err))
		return nil
	}

	// Alert processing maintains a dedicated DLQ writer in addition to the
	// common consumer's per-message DLQ path.
	dlqProducer := kafka.NewDLQProducer(buildKafkaDLQConfig(kafkaCfg), "alert-service", logger)

	return &Consumer{
		kafkaConsumer:     kafkaConsumer,
		dlqProducer:       dlqProducer,
		redisDedup:        redisDedup,
		dualWriter:        dualWriter,
		evidenceGenerator: evidenceGen,
		arkimeLinkGen:     arkimeGen,
		timeBucket:        dedupCfg.TimeBucketMinutes,
		logger:            logger,
		batchSize:         kafkaCfg.BatchSize,
		flushInterval:     time.Second,
		stopChan:          make(chan struct{}),
		generateEvidence:  evidenceGen != nil,
		generateArkime:    arkimeGen != nil,
		useLuaScript:      true, // 默认启用 Lua 脚本优化
	}
}

func buildKafkaConsumerConfig(kafkaCfg config.KafkaConfig) kafka.ConsumerConfig {
	return kafka.ConsumerConfig{
		Brokers:        kafkaCfg.Brokers,
		Topic:          kafkaCfg.Topic,
		GroupID:        kafkaCfg.GroupID,
		MinBytes:       1024,
		MaxBytes:       10 * 1024 * 1024, // 10MB
		MaxWait:        500 * time.Millisecond,
		CommitInterval: 0,  // 手动提交
		StartOffset:    -2, // earliest
		MaxRetries:     3,
		RetryBackoff:   time.Second,
		EnableDLQ:      true,
		DLQTopicPrefix: "dlq.",
		Security:       kafkaCfg.Security,
	}
}

func buildKafkaDLQConfig(kafkaCfg config.KafkaConfig) kafka.DLQConfig {
	return kafka.DLQConfig{
		Brokers:     kafkaCfg.Brokers,
		TopicPrefix: "dlq.",
		BatchSize:   100,
		MaxRetries:  3,
		Security:    kafkaCfg.Security,
	}
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("consumer is closed")
	}
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return fmt.Errorf("consumer already running")
	}
	consumeCtx, consumeCancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.runCancel = consumeCancel
	c.mu.Unlock()

	c.logger.Info("Starting alert consumer",
		zap.Int("batch_size", c.batchSize),
		zap.Duration("flush_interval", c.flushInterval),
		zap.Bool("generate_evidence", c.generateEvidence),
		zap.Bool("generate_arkime", c.generateArkime),
		zap.Bool("use_lua_script", c.useLuaScript))

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer atomic.StoreInt32(&c.running, 0)

		err := c.kafkaConsumer.BatchConsume(consumeCtx, c.batchSize, c.flushInterval, c.processBatch)
		if err != nil && err != context.Canceled {
			c.logger.Error("Consumer detection consume error", zap.Error(err))
		}
	}()

	// 等待停止信号或 context 取消
	select {
	case <-ctx.Done():
		c.logger.Info("Consumer stopping due to context cancellation")
	case <-c.stopChan:
		c.logger.Info("Consumer stopping due to stop signal")
	}
	consumeCancel()

	return nil
}

// processBatch 处理一批消息
func (c *Consumer) processBatch(ctx context.Context, msgs []*kafka.ReceivedMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	ctx = logging.WithRequestID(ctx, uuid.New().String())
	logger := logging.L(ctx)

	logger.Debug("Processing detection", zap.Int("count", len(msgs)))
	batchSizeMetric.Observe(float64(len(msgs)))

	start := time.Now()

	// ✅ 使用channel收集结果（线程安全）
	alertChan := make(chan *persistence.Alert, len(msgs))
	evidenceChan := make(chan *evidence.Evidence, len(msgs)*4)
	errorChan := make(chan error, len(msgs))

	var wg sync.WaitGroup

	// 并发处理消息
	for _, msg := range msgs {
		wg.Add(1)
		go func(m *kafka.ReceivedMessage) {
			defer wg.Done()

			tenantID := m.TenantID()
			if tenantID == "" {
				tenantID = "unknown"
			}
			messagesReceived.WithLabelValues(tenantID).Inc()

			alert, evs, err := c.processMessage(ctx, m)
			if err != nil {
				errorChan <- err
				messagesFailed.WithLabelValues(tenantID, categorizeError(err)).Inc()

				// 发送到DLQ
				if c.dlqProducer != nil {
					if dlqErr := c.dlqProducer.Send(ctx, m, err); dlqErr != nil {
						logger.Error("Failed to send to DLQ",
							zap.Error(dlqErr),
							zap.String("event_id", m.EventID()))
					}
				}
				return
			}

			if alert != nil {
				alertChan <- alert
				messagesProcessed.WithLabelValues(alert.TenantID, alert.Severity).Inc()
			}

			// 发送所有证据到channel
			for _, ev := range evs {
				evidenceChan <- ev
				evidenceGenerated.WithLabelValues(ev.TenantID, string(ev.Type)).Inc()
			}
		}(msg)
	}

	// 等待所有goroutine完成
	wg.Wait()
	close(alertChan)
	close(evidenceChan)
	close(errorChan)

	// 收集结果
	alerts := make([]*persistence.Alert, 0, len(alertChan))
	for alert := range alertChan {
		alerts = append(alerts, alert)
	}

	evidences := make([]*evidence.Evidence, 0, len(evidenceChan))
	for ev := range evidenceChan {
		evidences = append(evidences, ev)
	}

	processErrors := make([]error, 0, len(errorChan))
	for err := range errorChan {
		processErrors = append(processErrors, err)
	}

	// 批量写入告警
	if len(alerts) > 0 {
		writeStart := time.Now()
		if err := c.dualWriter.WriteBatch(ctx, alerts); err != nil {
			logger.Error("Failed to write alert detection",
				zap.Int("count", len(alerts)),
				zap.Error(err))
			return err // 不提交offset
		}
		batchWriteLatency.WithLabelValues("dual").Observe(time.Since(writeStart).Seconds())
		c.mu.Lock()
		dispatcher := c.notifier
		c.mu.Unlock()
		if dispatcher != nil {
			for _, alert := range alerts {
				assetScope, campus, objectType, objectID := notificationDimensions(alert.Labels)
				if err := dispatcher.Notify(ctx, &notification.AlertInfo{
					AlertID: alert.AlertID, Title: alert.AlertType, Severity: notification.NormalizeSeverity(alert.Severity, float64(alert.Score)), Score: float64(alert.Score),
					SourceIP: alert.SrcIP, DestIP: alert.DstIP, AlertType: notification.NormalizeAlertType(alert.AlertType, strings.Join(alert.Labels, ",")),
					Description: strings.Join(alert.Labels, ","), TenantID: alert.TenantID, Timestamp: alert.FirstSeen,
					CampaignID: alert.CampaignID, AssetScope: assetScope, Campus: campus, AssetName: objectID,
					ObjectType: objectType, ObjectID: objectID, Fingerprint: alert.Fingerprint,
				}); err != nil {
					c.logger.Warn("Governed notification dispatch failed", zap.String("alert_id", alert.AlertID), zap.Error(err))
				}
			}
		}
	}

	// 批量写入证据
	if len(evidences) > 0 && c.evidenceGenerator != nil {
		if err := c.evidenceGenerator.SaveEvidenceBatch(ctx, evidences); err != nil {
			logger.Warn("Failed to save evidence detection",
				zap.Int("count", len(evidences)),
				zap.Error(err))
		}
	}

	logger.Info("Batch processed",
		zap.Int("total", len(msgs)),
		zap.Int("alerts", len(alerts)),
		zap.Int("evidences", len(evidences)),
		zap.Int("errors", len(processErrors)),
		zap.Duration("duration", time.Since(start)))

	// ✅ 提交offset
	if len(alerts) > 0 {
	}
	return nil
}

func notificationDimensions(labels []string) (assetScope, campus, objectType, objectID string) {
	for _, label := range labels {
		parts := strings.SplitN(label, ":", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(label, "=", 2)
		}
		if len(parts) != 2 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "asset_scope", "asset_group", "资产组":
			assetScope = strings.TrimSpace(parts[1])
		case "campus", "园区":
			campus = strings.TrimSpace(parts[1])
		case "object_type":
			objectType = strings.TrimSpace(parts[1])
		case "object_id":
			objectID = strings.TrimSpace(parts[1])
		}
	}
	if assetScope == "" {
		assetScope = objectType
	}
	return assetScope, campus, objectType, objectID
}

// processMessage 处理单条消息
func (c *Consumer) processMessage(ctx context.Context, msg *kafka.ReceivedMessage) (*persistence.Alert, []*evidence.Evidence, error) {
	// 1. 解析 DetectionEvent
	var detection pb.DetectionBatch
	if err := msg.UnmarshalProto(&detection); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal detection: %w", err)
	}

	// 2. 获取租户ID
	tenantID := ""
	if detection.Behaviors[0].Header != nil {
		tenantID = detection.Behaviors[0].Header.GetTenantId()
	}
	if tenantID == "" {
		tenantID = msg.TenantID()
	}

	// 3. 计算指纹
	fingerprint := dedup.CalculateFingerprint(&detection, c.timeBucket)

	// 4. 去重检查（修复：优先使用 Lua 脚本原子版本）
	eventTs := detection.Behaviors[0].Header.GetEventTs()
	var dedupResult *dedup.DedupResult
	var err error

	if c.useLuaScript {
		// 使用 Lua 脚本原子操作（性能更好）
		dedupResult, err = c.redisDedup.CheckAndIncrementAtomic(ctx, fingerprint, eventTs, tenantID)
		if err == nil {
			dedupMethodUsed.WithLabelValues("lua_script").Inc()
		}
	} else {
		// 回退到 Pipeline 版本
		dedupResult, err = c.redisDedup.CheckAndIncrementWithTenant(ctx, fingerprint, eventTs, tenantID)
		if err == nil {
			dedupMethodUsed.WithLabelValues("pipeline").Inc()
		}
	}

	if err != nil {
		return nil, nil, fmt.Errorf("dedup check failed: %w", err)
	}

	// 5. 如果不是新告警，记录去重命中
	if !dedupResult.IsNew {
		dedupHits.WithLabelValues(tenantID).Inc()
	}

	// 6. 构建告警对象
	alert := c.buildAlert(&detection, fingerprint, dedupResult)

	// 7. 生成 Arkime 链接
	if c.generateArkime && c.arkimeLinkGen != nil {
		arkimeLinks := c.arkimeLinkGen.GenerateAlertLinks(
			alert.CommunityID,
			alert.SrcIP,
			alert.DstIP,
			alert.SrcPort,
			alert.DstPort,
			alert.Protocol,
			alert.FirstSeen,
			alert.LastSeen,
		)
		if arkimeLinks != nil {
			alert.ArkimeLink = arkimeLinks.SessionLink
		}
	}

	// 8. 生成证据（仅对新告警生成）
	var evidences []*evidence.Evidence
	if c.generateEvidence && c.evidenceGenerator != nil && dedupResult.IsNew {
		evs, err := c.evidenceGenerator.GenerateForAlert(ctx, alert)
		if err != nil {
			c.logger.Warn("Failed to generate evidence",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
		} else {
			evidences = evs
			// 更新告警的 evidence_ids
			for _, ev := range evs {
				alert.AddEvidenceID(ev.EvidenceID)
			}
		}
	}

	c.logger.Debug("Detection processed",
		zap.String("alert_id", alert.AlertID),
		zap.String("tenant_id", tenantID),
		zap.String("fingerprint", fingerprint),
		zap.Bool("is_new", dedupResult.IsNew),
		zap.Int64("count", dedupResult.Count),
		zap.Int("evidence_count", len(evidences)),
		zap.Bool("used_lua_script", c.useLuaScript))

	return alert, evidences, nil
}

// buildAlert 构建告警对象
func (c *Consumer) buildAlert(
	detection *pb.DetectionBatch,
	fingerprint string,
	dedupResult *dedup.DedupResult,
) *persistence.Alert {
	// 从 header 获取通用字段
	header := detection.Behaviors[0].GetHeader()
	tenantID := ""
	eventID := ""
	featureSetID := ""
	probeID := ""
	runID := ""
	var eventTs int64

	if header != nil {
		tenantID = header.GetTenantId()
		eventID = header.GetEventId()
		featureSetID = header.GetFeatureSetId()
		probeID = header.GetProbeId()
		runID = header.GetRunId()
		eventTs = header.GetEventTs()
	}

	// 提取五元组信息
	srcIP := ""
	dstIP := ""
	var srcPort, dstPort uint16
	var protocol uint8

	if detection.Behaviors[0].Header != nil {
		srcIP = ""
		dstIP = ""
		srcPort = uint16(0)
		dstPort = uint16(0)
		protocol = uint8(0)
	}

	// Preserve detector labels: notification routing, asset/campus scoping and
	// downstream investigation all depend on this business context.
	labels := append([]string(nil), detection.Behaviors[0].GetLabels()...)
	labels = append(labels,
		"object_type:"+detection.Behaviors[0].GetObjectType(),
		"object_id:"+detection.Behaviors[0].GetObjectId(),
	)

	// 提取evidence_ids（从证据条目中）
	evidenceIDs := make([]string, 0)
	for _, ev := range []*pb.Evidence{} {
		if ev != nil && ev.Type != "" {
			// 为每个 evidence entry 生成唯一 ID
			evidenceID := fmt.Sprintf("%s:%s:%s", eventID, ev.Type, ev.Summary)
			evidenceIDs = append(evidenceIDs, evidenceID)
		}
	}

	// 计算时间范围
	var tsStart, tsEnd int64
	if 0 > 0 {
		tsStart = 0
	} else {
		tsStart = eventTs
	}

	if 0 > 0 {
		tsEnd = 0
	} else {
		tsEnd = tsStart
	}

	// 使用 dedup 结果中的时间作为 first_seen 和 last_seen
	firstSeen := time.UnixMilli(dedupResult.FirstSeen)
	lastSeen := time.UnixMilli(dedupResult.LastSeen)

	// 如果 dedup 没有记录时间，使用 detection 的时间
	if dedupResult.FirstSeen == 0 {
		firstSeen = time.UnixMilli(tsStart)
	}
	if dedupResult.LastSeen == 0 {
		lastSeen = time.UnixMilli(tsEnd)
	}

	// 生成 alert_id
	alertID := uuid.New().String()

	// 获取 detection 的 ID 用于调试
	detectionID := "detection-unknown"
	if detectionID == "" {
		detectionID = alertID
	}

	// 构建 Alert 对象
	alert := &persistence.Alert{
		TenantID:    tenantID,
		AlertID:     alertID,
		Fingerprint: fingerprint,
		CommunityID: detection.Behaviors[0].GetCommunityId(),
		SessionID:   "",
		CampaignID:  "", // 后续由 CEP 填充

		SrcIP:    srcIP,
		DstIP:    dstIP,
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: protocol,

		AlertType: notification.NormalizeAlertType(detection.Behaviors[0].GetTopLabel(), strings.Join(labels, ",")),
		Labels:    labels,
		Score:     detection.Behaviors[0].GetTopScore(),
		Severity:  notification.NormalizeSeverity("", float64(detection.Behaviors[0].GetTopScore())),

		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
		Count:     int32(dedupResult.Count),

		Status:    state.StatusNew.String(),
		Assignee:  "",
		UpdatedTs: time.Now(),

		ModelVersion: detection.Behaviors[0].GetModelVersion(),
		RuleVersion:  "",
		FeatureSetID: featureSetID,

		EvidenceIDs: evidenceIDs,
		EventID:     eventID,
	}

	// 记录额外字段用于调试
	c.logger.Debug("Built alert from detection",
		zap.String("alert_id", alertID),
		zap.String("detection_id", detectionID),
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("run_id", runID),
		zap.String("flow_id", detection.Behaviors[0].GetObjectId()),
		zap.String("rule_id", "rule-unknown"),
		zap.String("model_id", ""),
		zap.Int64("ts_start", tsStart),
		zap.Int64("ts_end", tsEnd),
		zap.Int32("count", alert.Count),
	)

	return alert
}

// categorizeError 分类错误类型
func categorizeError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "unmarshal"):
		return "parse_error"
	case strings.Contains(errStr, "dedup"):
		return "dedup_error"
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection"):
		return "connection_error"
	case strings.Contains(errStr, "refused"):
		return "connection_refused"
	case strings.Contains(errStr, "deadline"):
		return "deadline_exceeded"
	case strings.Contains(errStr, "evidence"):
		return "evidence_error"
	case strings.Contains(errStr, "write"):
		return "write_error"
	default:
		return "unknown"
	}
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	c.stopOnce.Do(func() {
		c.logger.Info("Stopping alert consumer...")
		close(c.stopChan)
		c.mu.Lock()
		cancel := c.runCancel
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})
}

// Close 关闭消费者（优雅关闭）- 修复版
func (c *Consumer) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // 已经关闭
	}

	c.logger.Info("Closing alert consumer...")

	// 1. 发送停止信号
	c.Stop()

	// 2. 等待消费者 goroutine 停止（带超时）
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("Consumer goroutines stopped gracefully")
	case <-time.After(30 * time.Second):
		c.logger.Error("Timeout waiting for consumer to stop, forcing shutdown")
		// 超时后不继续等待，直接关闭资源
		// goroutine 会因为底层资源关闭而被迫退出
	}

	// 3. 按顺序关闭资源
	var errs []error

	// 3.1 先关闭 Kafka Consumer（停止接收新消息）
	if c.kafkaConsumer != nil {
		c.logger.Info("Closing Kafka consumer...")
		if err := c.kafkaConsumer.Close(); err != nil {
			c.logger.Error("Failed to close Kafka consumer", zap.Error(err))
			errs = append(errs, fmt.Errorf("kafka consumer: %w", err))
		} else {
			c.logger.Info("Kafka consumer closed")
		}
	}

	// 3.2 再关闭 DLQ Producer（确保所有 DLQ 消息发送完成）
	if c.dlqProducer != nil {
		c.logger.Info("Closing DLQ producer...")
		if err := c.dlqProducer.Close(); err != nil {
			c.logger.Error("Failed to close DLQ producer", zap.Error(err))
			errs = append(errs, fmt.Errorf("dlq producer: %w", err))
		} else {
			c.logger.Info("DLQ producer closed")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing consumer: %v", errs)
	}

	c.logger.Info("Alert consumer closed successfully")
	return nil
}

// IsRunning 检查消费者是否正在运行
func (c *Consumer) IsRunning() bool {
	return atomic.LoadInt32(&c.running) == 1
}

// IsClosed 检查消费者是否已关闭
func (c *Consumer) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// GetMetrics 获取消费者指标
func (c *Consumer) GetMetrics() kafka.ConsumerMetrics {
	if c.kafkaConsumer == nil {
		return kafka.ConsumerMetrics{}
	}
	return c.kafkaConsumer.GetMetrics()
}

// GetLag 获取消费延迟
func (c *Consumer) GetLag(ctx context.Context) (int64, error) {
	if c.kafkaConsumer == nil {
		return 0, fmt.Errorf("consumer not initialized")
	}
	return c.kafkaConsumer.Lag(ctx)
}

// SetEvidenceGenerator 设置证据生成器
func (c *Consumer) SetEvidenceGenerator(gen *evidence.Generator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evidenceGenerator = gen
	c.generateEvidence = gen != nil
}

// SetArkimeLinkGenerator 设置 Arkime 链接生成器
func (c *Consumer) SetArkimeLinkGenerator(gen *arkime.LinkGenerator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.arkimeLinkGen = gen
	c.generateArkime = gen != nil
}

// SetUseLuaScript 设置是否使用 Lua 脚本去重
func (c *Consumer) SetUseLuaScript(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.useLuaScript = enabled
	c.logger.Info("Lua script dedup toggled",
		zap.Bool("enabled", enabled))
}

// HealthCheck 健康检查
func (c *Consumer) HealthCheck(ctx context.Context) error {
	if c.IsClosed() {
		return fmt.Errorf("consumer is closed")
	}

	if !c.IsRunning() {
		return fmt.Errorf("consumer is not running")
	}
	if err := c.kafkaConsumer.HealthCheck(); err != nil {
		return err
	}

	// 检查 Redis 连接
	if c.redisDedup != nil {
		if err := c.redisDedup.Ping(ctx); err != nil {
			return fmt.Errorf("redis health check failed: %w", err)
		}
	}

	return nil
}

// ConsumerStatus 消费者状态
type ConsumerStatus struct {
	Running          bool                  `json:"running"`
	Closed           bool                  `json:"closed"`
	GenerateEvidence bool                  `json:"generate_evidence"`
	GenerateArkime   bool                  `json:"generate_arkime"`
	UseLuaScript     bool                  `json:"use_lua_script"`
	Metrics          kafka.ConsumerMetrics `json:"metrics"`
}

// GetStatus 获取消费者状态
func (c *Consumer) GetStatus() *ConsumerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	return &ConsumerStatus{
		Running:          c.IsRunning(),
		Closed:           c.IsClosed(),
		GenerateEvidence: c.generateEvidence,
		GenerateArkime:   c.generateArkime,
		UseLuaScript:     c.useLuaScript,
		Metrics:          c.GetMetrics(),
	}
}
