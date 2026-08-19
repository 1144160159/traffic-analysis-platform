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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	segmentKafka "github.com/segmentio/kafka-go"
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

	consumerLastCommittedOffset = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alert_consumer_last_committed_offset",
			Help: "Last Kafka offset acknowledged as committed by topic and partition",
		},
		[]string{"topic", "partition"},
	)

	consumerLastCommittedEvent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alert_consumer_last_committed_event_info",
			Help: "Last event ID whose Kafka offset was acknowledged as committed",
		},
		[]string{"topic", "partition", "event_id"},
	)

	dedupMethodUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_dedup_method_total",
			Help: "Dedup method used (atomic vs pipeline)",
		},
		[]string{"method"},
	)

	whitelistSuppressed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_whitelist_suppressed_total",
			Help: "Total number of detections suppressed by an applied whitelist rule projection",
		},
		[]string{"tenant_id"},
	)

	whitelistLookupFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_consumer_whitelist_lookup_failures_total",
			Help: "Total number of whitelist projection lookup failures that failed open",
		},
		[]string{"tenant_id"},
	)
)

// Consumer Alert消费者
type Consumer struct {
	kafkaConsumer     *kafka.Consumer
	dlqProducer       alertDLQProducer
	redisDedup        *dedup.RedisDedup
	dualWriter        *persistence.DualWriter
	evidenceGenerator *evidence.Generator
	arkimeLinkGen     *arkime.LinkGenerator
	notifier          interface {
		Notify(context.Context, *notification.AlertInfo) error
	}
	whitelistMatcher interface {
		MatchDetection(context.Context, string, string, string, string) (bool, error)
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
	commitMetricMu   sync.Mutex
	lastCommitEvent  map[string]string
	topic            string

	// pendingEvidence 证据落库失败后的进程内补偿队列：下一批次循环重试，
	// 避免"offset 照常提交 + 证据永久丢失"。进程崩溃时该队列会丢失
	// （降级语义，见 processBatch 注释）。
	pendingEvidence []*evidence.Evidence
}

// maxEvidencePerAlert 单个告警可能产出的证据上限（stat/sequence/fingerprint/arkime）。
const maxEvidencePerAlert = 16

// maxPendingEvidence 进程内证据补偿队列上限。
const maxPendingEvidence = 10000

type alertDLQProducer interface {
	Send(context.Context, *kafka.ReceivedMessage, error) error
	Close() error
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

// SetWhitelistMatcher connects alert ingestion to the durable rule-manager
// projection. A lookup failure is fail-open: detections remain visible while
// the broken control-plane dependency is surfaced through logs and metrics.
func (c *Consumer) SetWhitelistMatcher(matcher interface {
	MatchDetection(context.Context, string, string, string, string) (bool, error)
}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.whitelistMatcher = matcher
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

	consumer := &Consumer{
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
		lastCommitEvent:   make(map[string]string),
		topic:             kafkaCfg.Topic,
	}
	kafkaConsumer.SetCommitObserver(consumer.observeCommittedMessages)
	return consumer
}

func (c *Consumer) observeCommittedMessages(messages []segmentKafka.Message) {
	latest := make(map[int]segmentKafka.Message)
	for _, message := range messages {
		current, exists := latest[message.Partition]
		if !exists || message.Offset > current.Offset {
			latest[message.Partition] = message
		}
	}
	c.commitMetricMu.Lock()
	defer c.commitMetricMu.Unlock()
	for partition, message := range latest {
		partitionLabel := strconv.Itoa(partition)
		consumerLastCommittedOffset.WithLabelValues(c.kafkaConsumerTopic(), partitionLabel).Set(float64(message.Offset))
		eventID := kafkaMessageHeader(message, "event_id")
		if eventID == "" {
			eventID = kafkaMessageHeader(message, "x-event-id")
		}
		key := c.kafkaConsumerTopic() + ":" + partitionLabel
		if previous := c.lastCommitEvent[key]; previous != "" {
			consumerLastCommittedEvent.DeleteLabelValues(c.kafkaConsumerTopic(), partitionLabel, previous)
		}
		if eventID != "" {
			consumerLastCommittedEvent.WithLabelValues(c.kafkaConsumerTopic(), partitionLabel, eventID).Set(1)
			c.lastCommitEvent[key] = eventID
		}
	}
	if lag, err := c.kafkaConsumer.Lag(context.Background()); err == nil {
		consumerLag.WithLabelValues(c.kafkaConsumerTopic(), "all").Set(float64(lag))
	}
}

func (c *Consumer) kafkaConsumerTopic() string { return c.topic }

func kafkaMessageHeader(message segmentKafka.Message, key string) string {
	for _, header := range message.Headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
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
		// DLQ 收敛为单一专用 producer（c.dlqProducer）：关闭 common consumer
		// 内建 DLQ 写路径，避免同一批失败消息被写两份 dlq.<topic> 记录。
		EnableDLQ:      false,
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

		// 批次失败（offset 未提交）时重启消费循环并退避，避免单一批次
		// 失败永久停摆消费者；消息在 offset 提交前会被重新投递，配合
		// 事件级去重与专用 DLQ 保证至少一次语义。
		for {
			err := c.kafkaConsumer.BatchConsume(consumeCtx, c.batchSize, c.flushInterval, c.processBatch)
			if err == nil || err == context.Canceled || consumeCtx.Err() != nil {
				if err != nil && err != context.Canceled {
					c.logger.Info("Alert consumer stopping", zap.Error(err))
				}
				return
			}
			c.logger.Error("Alert consumer batch consume error, restarting after backoff",
				zap.Error(err))
			select {
			case <-consumeCtx.Done():
				return
			case <-time.After(2 * time.Second):
			}
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

	// 补偿：先重试上一轮未落库的证据（幂等插入，重复重试不会产生新行）
	if c.evidenceGenerator != nil {
		c.mu.Lock()
		pending := c.pendingEvidence
		c.pendingEvidence = nil
		c.mu.Unlock()
		if len(pending) > 0 {
			if err := c.evidenceGenerator.SaveEvidenceBatch(ctx, pending); err != nil {
				c.mu.Lock()
				c.pendingEvidence = append(c.pendingEvidence, pending...)
				if len(c.pendingEvidence) > maxPendingEvidence {
					c.pendingEvidence = c.pendingEvidence[len(c.pendingEvidence)-maxPendingEvidence:]
				}
				pendingAfter := len(c.pendingEvidence)
				c.mu.Unlock()
				logger.Error("Compensation retry for pending evidence failed",
					zap.Int("pending", pendingAfter),
					zap.Error(err))
			} else {
				logger.Info("Compensation retry for pending evidence succeeded",
					zap.Int("count", len(pending)))
			}
		}
	}

	// ✅ 使用channel收集结果（线程安全）
	alertChan := make(chan *persistence.Alert, len(msgs))
	// 容量 = 消息数 × 单告警证据上限常量，避免单告警证据数超 4 时 send 阻塞
	// 导致 wg.Wait() 永久等待（死锁）；另对 Wait 本身加超时兜底。
	evidenceChan := make(chan *evidence.Evidence, len(msgs)*maxEvidencePerAlert)
	errorChan := make(chan error, len(msgs))
	unsafeFailureChan := make(chan error, len(msgs))

	var wg sync.WaitGroup

	// 并发处理消息
	for _, msg := range msgs {
		wg.Add(1)
		go func(m *kafka.ReceivedMessage) {
			defer wg.Done()
			// 单条消息故障隔离：panic 不得击穿 BatchConsume 循环
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered while processing message",
						zap.Any("panic", r),
						zap.String("event_id", m.EventID()))
					unsafeFailureChan <- fmt.Errorf("event %s panicked during processing: %v", m.EventID(), r)
				}
			}()

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
						unsafeFailureChan <- fmt.Errorf("event %s failed processing and DLQ persistence: %w", m.EventID(), dlqErr)
					}
				} else {
					unsafeFailureChan <- fmt.Errorf("event %s failed processing while DLQ is unavailable: %w", m.EventID(), err)
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

	// 等待所有goroutine完成（加超时兜底，防止极端情况下通道阻塞死锁）
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(60 * time.Second):
		logger.Error("Alert consumer batch processing timed out waiting for message goroutines",
			zap.Int("count", len(msgs)))
		return errors.New("alert consumer batch processing timed out")
	}
	close(alertChan)
	close(evidenceChan)
	close(errorChan)
	close(unsafeFailureChan)

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
	unsafeFailures := make([]error, 0, len(unsafeFailureChan))
	for err := range unsafeFailureChan {
		unsafeFailures = append(unsafeFailures, err)
	}
	if len(unsafeFailures) > 0 {
		// Returning an error is the commit barrier used by BatchConsume. The
		// common consumer will not advance the batch offset unless its own DLQ
		// handoff is durably acknowledged.
		return errors.Join(unsafeFailures...)
	}

	// 批量写入告警
	if len(alerts) > 0 {
		writeStart := time.Now()
		outcome, err := c.dualWriter.WriteBatchWithOutcome(ctx, alerts)
		var projectionPending *persistence.ProjectionPendingError
		if err != nil && !(errors.As(err, &projectionPending) && outcome.ClickHouseCommitted && outcome.DebtRecorded) {
			logger.Error("Failed to write alert detection",
				zap.Int("count", len(alerts)),
				zap.Error(err))
			return err // 不提交offset
		}
		if projectionPending != nil {
			logger.Warn("Alert source committed with durable OpenSearch projection debt",
				zap.Int("alert_count", len(alerts)),
				zap.Int("projection_debt_count", outcome.DebtCount),
				zap.Error(projectionPending))
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

	// 批量写入证据（失败进进程内补偿队列，下一批次循环重试，不静默丢弃）
	if len(evidences) > 0 && c.evidenceGenerator != nil {
		if err := c.evidenceGenerator.SaveEvidenceBatch(ctx, evidences); err != nil {
			c.mu.Lock()
			c.pendingEvidence = append(c.pendingEvidence, evidences...)
			// 有界补偿：防止上游持续失败时内存无界增长
			if len(c.pendingEvidence) > maxPendingEvidence {
				c.pendingEvidence = c.pendingEvidence[len(c.pendingEvidence)-maxPendingEvidence:]
			}
			pending := len(c.pendingEvidence)
			c.mu.Unlock()
			logger.Error("Failed to save evidence, queued for compensation retry",
				zap.Int("count", len(evidences)),
				zap.Int("pending", pending),
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
	if len(detection.Behaviors) == 0 || detection.Behaviors[0] == nil {
		return nil, nil, fmt.Errorf("invalid detection batch: behaviors must contain at least one item")
	}
	if err := validateDetectionBehavior(detection.Behaviors[0]); err != nil {
		return nil, nil, err
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
	tuple := detection.Behaviors[0].GetTuple()
	c.mu.Lock()
	whitelistMatcher := c.whitelistMatcher
	c.mu.Unlock()
	if whitelistMatcher != nil {
		matched, matchErr := whitelistMatcher.MatchDetection(ctx, tenantID, tuple.GetSrcIp(), tuple.GetDstIp(), fingerprint)
		if matchErr != nil {
			whitelistLookupFailures.WithLabelValues(tenantID).Inc()
			c.logger.Warn("Whitelist projection lookup failed open",
				zap.String("tenant_id", tenantID), zap.String("event_id", msg.EventID()), zap.Error(matchErr))
		} else if matched {
			whitelistSuppressed.WithLabelValues(tenantID).Inc()
			c.logger.Info("Detection suppressed by applied whitelist projection",
				zap.String("tenant_id", tenantID), zap.String("event_id", msg.EventID()),
				zap.String("fingerprint", fingerprint))
			return nil, nil, nil
		}
	}

	// 4. 去重检查（修复：优先使用 Lua 脚本原子版本）
	eventTs := canonicalDetectionEventMillis(&detection)
	if eventTs <= 0 {
		return nil, nil, fmt.Errorf("invalid detection behavior: source event timestamp is required")
	}
	eventID := detection.Behaviors[0].Header.GetEventId()
	dedupResult, err := c.redisDedup.CheckAndIncrementEventAtomic(ctx, fingerprint, eventID, eventTs, tenantID)
	if err == nil {
		dedupMethodUsed.WithLabelValues("event_identity_lua_script").Inc()
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
		zap.Bool("exact_replay", dedupResult.ExactReplay),
		zap.Int64("count", dedupResult.Count),
		zap.Int("evidence_count", len(evidences)),
		zap.Bool("used_lua_script", c.useLuaScript))

	return alert, evidences, nil
}

func validateDetectionBehavior(behavior *pb.DetectionBehavior) error {
	if behavior == nil {
		return fmt.Errorf("invalid detection behavior: item is nil")
	}
	header := behavior.GetHeader()
	if header == nil {
		return fmt.Errorf("invalid detection behavior: event header is required")
	}
	required := map[string]string{
		"event_id": header.GetEventId(), "tenant_id": header.GetTenantId(),
		"event_type": header.GetEventType(), "schema_version": header.GetSchemaVersion(),
		"aggregate_type": header.GetAggregateType(), "aggregate_id": header.GetAggregateId(),
		"trace_id": header.GetTraceId(), "causation_id": header.GetCausationId(),
		"correlation_id": header.GetCorrelationId(), "idempotency_key": header.GetIdempotencyKey(),
		"producer": header.GetProducer(),
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("invalid detection behavior: envelope %s is required", field)
		}
	}
	if header.GetEventType() != "traffic.detection.behavior.v1" || header.GetSchemaVersion() != "1" {
		return fmt.Errorf("invalid detection behavior: unsupported event contract %s/%s", header.GetEventType(), header.GetSchemaVersion())
	}
	if header.GetAggregateVersion() == 0 || header.GetOccurredAt() <= 0 || header.GetProducedAt() <= 0 {
		return fmt.Errorf("invalid detection behavior: aggregate version and event timestamps must be positive")
	}
	if header.GetIdempotencyKey() != header.GetEventId() {
		return fmt.Errorf("invalid detection behavior: idempotency_key must equal event_id")
	}
	if behavior.GetTuple() == nil {
		return fmt.Errorf("invalid detection behavior: source tuple is required")
	}
	tuple := behavior.GetTuple()
	if strings.TrimSpace(tuple.GetSrcIp()) == "" || strings.TrimSpace(tuple.GetDstIp()) == "" || tuple.GetProtocol() == 0 {
		return fmt.Errorf("invalid detection behavior: source tuple addresses and protocol are required")
	}
	return nil
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
	traceID := ""
	featureSetID := ""
	probeID := ""
	runID := ""
	var eventTs int64

	if header != nil {
		tenantID = header.GetTenantId()
		eventID = header.GetEventId()
		traceID = header.GetTraceId()
		featureSetID = header.GetFeatureSetId()
		probeID = header.GetProbeId()
		runID = header.GetRunId()
		eventTs = header.GetEventTs()
	}
	if eventTs <= 0 {
		eventTs = detection.Behaviors[0].GetTs()
	}
	if eventTs <= 0 {
		eventTs = detection.GetCreatedAt()
	}

	// Preserve the real source tuple carried through Session -> Feature ->
	// Detection. Empty hard-coded network identity is not a valid alert.
	tuple := detection.Behaviors[0].GetTuple()
	srcIP := tuple.GetSrcIp()
	dstIP := tuple.GetDstIp()
	srcPort := uint16(tuple.GetSrcPort())
	dstPort := uint16(tuple.GetDstPort())
	protocol := uint8(tuple.GetProtocol())

	// Preserve detector labels: notification routing, asset/campus scoping and
	// downstream investigation all depend on this business context.
	labels := append([]string(nil), detection.Behaviors[0].GetLabels()...)
	labels = append(labels,
		"object_type:"+detection.Behaviors[0].GetObjectType(),
		"object_id:"+detection.Behaviors[0].GetObjectId(),
	)

	// Evidence references are source facts. Do not synthesize references from
	// empty placeholders; retain the producer-provided flow/evidence IDs.
	evidenceIDs := append([]string(nil), detection.Behaviors[0].GetEvidenceIds()...)

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

	// Stable identity makes replay effectively-once at the projection ID
	// boundary even when the detector does not provide alert_id.
	alertID := stableAlertID(tenantID, eventID, fingerprint)

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
		SessionID:   detection.Behaviors[0].GetObjectId(),
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

		Status:   state.StatusNew.String(),
		Assignee: "",
		// Bind the projection version to source event time. Reprocessing the same
		// event after a downstream failure must produce the same version and hash,
		// while OpenSearch external_gte rejects an older event for the same alert.
		UpdatedTs: time.UnixMilli(eventTs).UTC(),

		ModelVersion: detection.Behaviors[0].GetModelVersion(),
		RuleVersion:  "",
		FeatureSetID: featureSetID,

		EvidenceIDs: evidenceIDs,
		EventID:     eventID,
		TraceID:     traceID,
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

func canonicalDetectionEventMillis(detection *pb.DetectionBatch) int64 {
	if detection == nil || len(detection.Behaviors) == 0 || detection.Behaviors[0] == nil {
		return 0
	}
	behavior := detection.Behaviors[0]
	if eventTs := behavior.GetHeader().GetEventTs(); eventTs > 0 {
		return eventTs
	}
	if eventTs := behavior.GetTs(); eventTs > 0 {
		return eventTs
	}
	return detection.GetCreatedAt()
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
