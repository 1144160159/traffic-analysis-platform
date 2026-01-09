////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/audit/logger.go
// 修复版本 v3：
// 1. 修复 #4：增加批量优化，减少高频场景下的性能瓶颈
// 2. 新增 LogBatch 方法用于批量记录
// 3. 优化后台处理器的批处理逻辑
// 4. 增加批量写入指标
////////////////////////////////////////////////////////////////////////////////

package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Logger 审计日志记录器（修复：增加批量优化）
type Logger struct {
	kafkaWriter *kafka.Writer
	logger      *zap.Logger
	serviceName string
	buffer      chan *AuditEvent
	wg          sync.WaitGroup
	closed      int32 // 使用 atomic
	mu          sync.RWMutex

	// 配置
	config Config

	// 本地备份
	backupDir     string
	backupEnabled bool
	backupFile    *os.File
	backupWriter  *bufio.Writer
	backupMu      sync.Mutex

	// 统计（新增批量统计）
	sentCount       int64
	droppedCount    int64
	errorCount      int64
	backupCount     int64
	batchSentCount  int64 // 新增：批量发送次数
	batchEventCount int64 // 新增：批量事件总数
}

// Config 审计日志配置（修复：增加批量配置）
type Config struct {
	KafkaBrokers    []string      `env:"KAFKA_BROKERS" envSeparator:","`
	Topic           string        `env:"AUDIT_TOPIC" envDefault:"audit.logs"`
	ServiceName     string        `env:"SERVICE_NAME"`
	BufferSize      int           `env:"AUDIT_BUFFER_SIZE" envDefault:"1000"`
	BatchSize       int           `env:"AUDIT_BATCH_SIZE" envDefault:"100"`
	FlushInterval   time.Duration `env:"AUDIT_FLUSH_INTERVAL" envDefault:"1s"`
	ShutdownTimeout time.Duration `env:"AUDIT_SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// 本地备份配置
	BackupEnabled bool   `env:"AUDIT_BACKUP_ENABLED" envDefault:"true"`
	BackupDir     string `env:"AUDIT_BACKUP_DIR" envDefault:"/var/log/audit"`

	// 重试配置
	MaxRetries   int           `env:"AUDIT_MAX_RETRIES" envDefault:"3"`
	RetryBackoff time.Duration `env:"AUDIT_RETRY_BACKOFF" envDefault:"100ms"`

	// 修复 #4：新增批量优化配置
	EnableBatchOptimization bool          `env:"AUDIT_ENABLE_BATCH_OPTIMIZATION" envDefault:"true"`
	MaxBatchWaitTime        time.Duration `env:"AUDIT_MAX_BATCH_WAIT_TIME" envDefault:"500ms"`
}

// NewLogger 创建审计日志记录器
func NewLogger(cfg Config, logger *zap.Logger) (*Logger, error) {
	if len(cfg.KafkaBrokers) == 0 {
		return nil, nil // 不配置 Kafka 时返回 nil
	}

	// 设置默认值
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBatchWaitTime <= 0 {
		cfg.MaxBatchWaitTime = 500 * time.Millisecond
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.FlushInterval,
		RequiredAcks: kafka.RequireOne,
		Async:        false, // 使用同步写入确保消息不丢失
		MaxAttempts:  cfg.MaxRetries,
	}

	l := &Logger{
		kafkaWriter: writer,
		logger:      logger,
		serviceName: cfg.ServiceName,
		buffer:      make(chan *AuditEvent, cfg.BufferSize),
		config:      cfg,
		backupDir:   cfg.BackupDir,
	}

	// 初始化本地备份
	if cfg.BackupEnabled {
		if err := l.initBackup(); err != nil {
			logger.Warn("Failed to initialize audit backup, backup disabled", zap.Error(err))
		} else {
			l.backupEnabled = true
		}
	}

	// 启动后台写入goroutine
	l.wg.Add(1)
	go l.processBuffer()

	return l, nil
}

// initBackup 初始化本地备份
func (l *Logger) initBackup() error {
	// 确保目录存在
	if err := os.MkdirAll(l.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	// 创建备份文件（按服务名和日期命名）
	filename := fmt.Sprintf("%s_%s.jsonl", l.serviceName, time.Now().Format("2006-01-02"))
	filepath := filepath.Join(l.backupDir, filename)

	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}

	l.backupFile = file
	l.backupWriter = bufio.NewWriter(file)

	l.logger.Info("Audit backup initialized",
		zap.String("path", filepath))

	return nil
}

// rotateBackupFile 轮转备份文件（每天新建一个文件）
func (l *Logger) rotateBackupFile() error {
	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	if l.backupFile == nil {
		return l.initBackup()
	}

	// 检查是否需要轮转（跨天）
	currentDate := time.Now().Format("2006-01-02")
	expectedFilename := fmt.Sprintf("%s_%s.jsonl", l.serviceName, currentDate)
	expectedPath := filepath.Join(l.backupDir, expectedFilename)

	if l.backupFile.Name() != expectedPath {
		// 关闭旧文件
		l.backupWriter.Flush()
		l.backupFile.Close()

		// 打开新文件
		file, err := os.OpenFile(expectedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to rotate backup file: %w", err)
		}

		l.backupFile = file
		l.backupWriter = bufio.NewWriter(file)

		l.logger.Info("Audit backup file rotated",
			zap.String("path", expectedPath))
	}

	return nil
}

// writeToBackup 写入本地备份
func (l *Logger) writeToBackup(event *AuditEvent) error {
	if !l.backupEnabled || l.backupWriter == nil {
		return nil
	}

	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	// 检查是否需要轮转
	if err := l.rotateBackupFile(); err != nil {
		l.logger.Warn("Failed to rotate backup file", zap.Error(err))
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event for backup: %w", err)
	}

	if _, err := l.backupWriter.Write(data); err != nil {
		return fmt.Errorf("failed to write to backup: %w", err)
	}
	if _, err := l.backupWriter.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline to backup: %w", err)
	}

	atomic.AddInt64(&l.backupCount, 1)
	return nil
}

// writeToBackupBatch 批量写入本地备份（修复 #4：新增）
func (l *Logger) writeToBackupBatch(events []*AuditEvent) error {
	if !l.backupEnabled || l.backupWriter == nil {
		return nil
	}

	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	// 检查是否需要轮转
	if err := l.rotateBackupFile(); err != nil {
		l.logger.Warn("Failed to rotate backup file", zap.Error(err))
	}

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			l.logger.Warn("Failed to marshal event for backup", zap.Error(err))
			continue
		}

		if _, err := l.backupWriter.Write(data); err != nil {
			return fmt.Errorf("failed to write to backup: %w", err)
		}
		if _, err := l.backupWriter.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write newline to backup: %w", err)
		}

		atomic.AddInt64(&l.backupCount, 1)
	}

	return nil
}

// flushBackup 刷新备份缓冲区
func (l *Logger) flushBackup() error {
	if !l.backupEnabled || l.backupWriter == nil {
		return nil
	}

	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	if err := l.backupWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush backup: %w", err)
	}
	if err := l.backupFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync backup file: %w", err)
	}

	return nil
}

// Log 记录审计事件
func (l *Logger) Log(ctx context.Context, event *AuditEvent) {
	if atomic.LoadInt32(&l.closed) == 1 {
		atomic.AddInt64(&l.droppedCount, 1)
		return
	}

	// 填充默认值
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.ServiceName == "" {
		event.ServiceName = l.serviceName
	}

	// 从context中提取信息
	lc := logging.LogContextFromContext(ctx)
	if event.TraceID == "" && lc.TraceID != "" {
		event.TraceID = lc.TraceID
	}
	if event.RequestID == "" && lc.RequestID != "" {
		event.RequestID = lc.RequestID
	}
	if event.TenantID == "" && lc.TenantID != "" {
		event.TenantID = lc.TenantID
	}
	if event.UserID == "" && lc.UserID != "" {
		event.UserID = lc.UserID
	}

	// 设置敏感级别
	if event.Sensitivity == "" {
		info := GetEventTypeInfo(event.EventType)
		event.Sensitivity = info.Sensitivity
	}

	// 非阻塞写入buffer
	select {
	case l.buffer <- event:
		// 成功加入队列
	default:
		// 缓冲区满，写入本地备份
		atomic.AddInt64(&l.droppedCount, 1)
		l.logger.Warn("Audit buffer full, writing to backup",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)))

		// 尝试写入备份
		if err := l.writeToBackup(event); err != nil {
			l.logger.Error("Failed to write to backup", zap.Error(err))
		}
	}
}

// LogBatch 批量记录审计事件（修复 #4：新增）
func (l *Logger) LogBatch(ctx context.Context, events []*AuditEvent) {
	if atomic.LoadInt32(&l.closed) == 1 {
		atomic.AddInt64(&l.droppedCount, int64(len(events)))
		return
	}

	// 填充默认值
	lc := logging.LogContextFromContext(ctx)
	for _, event := range events {
		if event.EventID == "" {
			event.EventID = uuid.New().String()
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}
		if event.ServiceName == "" {
			event.ServiceName = l.serviceName
		}

		// 从context中提取信息
		if event.TraceID == "" && lc.TraceID != "" {
			event.TraceID = lc.TraceID
		}
		if event.RequestID == "" && lc.RequestID != "" {
			event.RequestID = lc.RequestID
		}
		if event.TenantID == "" && lc.TenantID != "" {
			event.TenantID = lc.TenantID
		}
		if event.UserID == "" && lc.UserID != "" {
			event.UserID = lc.UserID
		}

		// 设置敏感级别
		if event.Sensitivity == "" {
			info := GetEventTypeInfo(event.EventType)
			event.Sensitivity = info.Sensitivity
		}
	}

	// 批量写入buffer
	droppedCount := 0
	for _, event := range events {
		select {
		case l.buffer <- event:
			// 成功加入队列
		default:
			droppedCount++
		}
	}

	// 如果有丢弃的事件，尝试写入备份
	if droppedCount > 0 {
		atomic.AddInt64(&l.droppedCount, int64(droppedCount))
		l.logger.Warn("Audit buffer full, some events dropped",
			zap.Int("dropped", droppedCount),
			zap.Int("total", len(events)))

		// 将未能加入队列的事件写入备份
		droppedEvents := events[len(events)-droppedCount:]
		if err := l.writeToBackupBatch(droppedEvents); err != nil {
			l.logger.Error("Failed to write dropped events to backup", zap.Error(err))
		}
	}
}

// processBuffer 处理缓冲区中的事件（修复 #4：增加批量处理逻辑）
func (l *Logger) processBuffer() {
	defer l.wg.Done()

	// 定期刷新备份文件
	flushTicker := time.NewTicker(5 * time.Second)
	defer flushTicker.Stop()

	// 修复 #4：批量处理优化
	if l.config.EnableBatchOptimization {
		l.processBufferBatched(flushTicker)
	} else {
		l.processBufferSingle(flushTicker)
	}
}

// processBufferSingle 单条处理模式（原有逻辑）
func (l *Logger) processBufferSingle(flushTicker *time.Ticker) {
	for {
		select {
		case event, ok := <-l.buffer:
			if !ok {
				// channel 已关闭，处理完毕
				return
			}
			if err := l.writeToKafka(event); err != nil {
				atomic.AddInt64(&l.errorCount, 1)
				l.logger.Error("Failed to write audit event to Kafka",
					zap.String("event_id", event.EventID),
					zap.Error(err))

				// Kafka 写入失败，写入本地备份
				if backupErr := l.writeToBackup(event); backupErr != nil {
					l.logger.Error("Failed to write to backup after Kafka failure",
						zap.String("event_id", event.EventID),
						zap.Error(backupErr))
				}
			} else {
				atomic.AddInt64(&l.sentCount, 1)
			}

		case <-flushTicker.C:
			// 定期刷新备份
			if err := l.flushBackup(); err != nil {
				l.logger.Warn("Failed to flush backup", zap.Error(err))
			}
		}
	}
}

// processBufferBatched 批量处理模式（修复 #4：新增）
func (l *Logger) processBufferBatched(flushTicker *time.Ticker) {
	batch := make([]*AuditEvent, 0, l.config.BatchSize)
	batchTimer := time.NewTimer(l.config.MaxBatchWaitTime)
	defer batchTimer.Stop()

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}

		if err := l.writeToKafkaBatch(batch); err != nil {
			atomic.AddInt64(&l.errorCount, int64(len(batch)))
			l.logger.Error("Failed to write audit batch to Kafka",
				zap.Int("batch_size", len(batch)),
				zap.Error(err))

			// Kafka 写入失败，写入本地备份
			if backupErr := l.writeToBackupBatch(batch); backupErr != nil {
				l.logger.Error("Failed to write batch to backup after Kafka failure",
					zap.Error(backupErr))
			}
		} else {
			atomic.AddInt64(&l.sentCount, int64(len(batch)))
			atomic.AddInt64(&l.batchSentCount, 1)
			atomic.AddInt64(&l.batchEventCount, int64(len(batch)))
		}

		// 清空批次
		batch = batch[:0]

		// 重置定时器
		batchTimer.Reset(l.config.MaxBatchWaitTime)
	}

	for {
		select {
		case event, ok := <-l.buffer:
			if !ok {
				// channel 已关闭，刷新剩余批次
				flushBatch()
				return
			}

			batch = append(batch, event)

			// 达到批次大小，立即刷新
			if len(batch) >= l.config.BatchSize {
				flushBatch()
			}

		case <-batchTimer.C:
			// 批次等待超时，刷新
			flushBatch()

		case <-flushTicker.C:
			// 定期刷新备份
			if err := l.flushBackup(); err != nil {
				l.logger.Warn("Failed to flush backup", zap.Error(err))
			}
		}
	}
}

// writeToKafka 写入Kafka（带重试）
func (l *Logger) writeToKafka(event *AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(event.TenantID),
		Value: data,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "sensitivity", Value: []byte(event.Sensitivity)},
			{Key: "event_id", Value: []byte(event.EventID)},
		},
	}

	var lastErr error
	for attempt := 0; attempt < l.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(l.config.RetryBackoff * time.Duration(attempt))
		}

		// 使用带超时的 context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := l.kafkaWriter.WriteMessages(ctx, msg)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		l.logger.Warn("Kafka write attempt failed",
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	return fmt.Errorf("all Kafka write attempts failed: %w", lastErr)
}

// writeToKafkaBatch 批量写入Kafka（修复 #4：新增）
func (l *Logger) writeToKafkaBatch(events []*AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	messages := make([]kafka.Message, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			l.logger.Warn("Failed to marshal event in batch",
				zap.String("event_id", event.EventID),
				zap.Error(err))
			continue
		}

		msg := kafka.Message{
			Key:   []byte(event.TenantID),
			Value: data,
			Headers: []kafka.Header{
				{Key: "event_type", Value: []byte(event.EventType)},
				{Key: "sensitivity", Value: []byte(event.Sensitivity)},
				{Key: "event_id", Value: []byte(event.EventID)},
			},
		}
		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		return nil
	}

	var lastErr error
	for attempt := 0; attempt < l.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(l.config.RetryBackoff * time.Duration(attempt))
		}

		// 使用带超时的 context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := l.kafkaWriter.WriteMessages(ctx, messages...)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		l.logger.Warn("Kafka batch write attempt failed",
			zap.Int("attempt", attempt+1),
			zap.Int("batch_size", len(messages)),
			zap.Error(err))
	}

	return fmt.Errorf("all Kafka batch write attempts failed: %w", lastErr)
}

// Close 关闭审计日志记录器
func (l *Logger) Close() error {
	// 标记为已关闭
	if !atomic.CompareAndSwapInt32(&l.closed, 0, 1) {
		return nil // 已经关闭
	}

	l.logger.Info("Closing audit logger, waiting for buffer to drain...",
		zap.Int("buffer_size", len(l.buffer)))

	// 处理剩余的缓冲区消息
	remainingMessages := l.drainBuffer()

	// 关闭 buffer channel
	close(l.buffer)

	// 等待所有消息处理完成
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		l.logger.Info("Audit logger buffer drained successfully",
			zap.Int64("sent", atomic.LoadInt64(&l.sentCount)),
			zap.Int64("dropped", atomic.LoadInt64(&l.droppedCount)),
			zap.Int64("errors", atomic.LoadInt64(&l.errorCount)),
			zap.Int64("backup", atomic.LoadInt64(&l.backupCount)),
			zap.Int64("batch_sent", atomic.LoadInt64(&l.batchSentCount)),
			zap.Int64("batch_events", atomic.LoadInt64(&l.batchEventCount)))
	case <-time.After(l.config.ShutdownTimeout):
		l.logger.Warn("Audit logger shutdown timed out, saving remaining to backup",
			zap.Int("remaining", len(l.buffer)))

		// 超时后将剩余消息写入备份
		l.saveRemainingToBackup(remainingMessages)
	}

	// 刷新并关闭备份文件
	if l.backupEnabled {
		if err := l.flushBackup(); err != nil {
			l.logger.Error("Failed to flush backup on close", zap.Error(err))
		}
		l.closeBackup()
	}

	// 关闭 Kafka writer
	return l.kafkaWriter.Close()
}

// drainBuffer 排空缓冲区，返回剩余消息
func (l *Logger) drainBuffer() []*AuditEvent {
	remaining := make([]*AuditEvent, 0)

	for {
		select {
		case event := <-l.buffer:
			remaining = append(remaining, event)
		default:
			return remaining
		}
	}
}

// saveRemainingToBackup 将剩余消息保存到备份
func (l *Logger) saveRemainingToBackup(events []*AuditEvent) {
	if !l.backupEnabled {
		l.logger.Warn("Backup not enabled, messages will be lost",
			zap.Int("count", len(events)))
		return
	}

	savedCount := 0
	for _, event := range events {
		if err := l.writeToBackup(event); err != nil {
			l.logger.Error("Failed to save event to backup",
				zap.String("event_id", event.EventID),
				zap.Error(err))
		} else {
			savedCount++
		}
	}

	l.logger.Info("Saved remaining events to backup",
		zap.Int("total", len(events)),
		zap.Int("saved", savedCount))
}

// closeBackup 关闭备份文件
func (l *Logger) closeBackup() {
	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	if l.backupWriter != nil {
		l.backupWriter.Flush()
	}
	if l.backupFile != nil {
		l.backupFile.Close()
		l.backupFile = nil
	}
}

// GetStats 获取统计信息（修复 #4：增加批量统计）
func (l *Logger) GetStats() LoggerStats {
	return LoggerStats{
		SentCount:       atomic.LoadInt64(&l.sentCount),
		DroppedCount:    atomic.LoadInt64(&l.droppedCount),
		ErrorCount:      atomic.LoadInt64(&l.errorCount),
		BackupCount:     atomic.LoadInt64(&l.backupCount),
		BatchSentCount:  atomic.LoadInt64(&l.batchSentCount),
		BatchEventCount: atomic.LoadInt64(&l.batchEventCount),
		BufferSize:      len(l.buffer),
		BufferCap:       cap(l.buffer),
		IsClosed:        atomic.LoadInt32(&l.closed) == 1,
	}
}

// LoggerStats 日志记录器统计（修复 #4：增加批量字段）
type LoggerStats struct {
	SentCount       int64 `json:"sent_count"`
	DroppedCount    int64 `json:"dropped_count"`
	ErrorCount      int64 `json:"error_count"`
	BackupCount     int64 `json:"backup_count"`
	BatchSentCount  int64 `json:"batch_sent_count"`  // 新增
	BatchEventCount int64 `json:"batch_event_count"` // 新增
	BufferSize      int   `json:"buffer_size"`
	BufferCap       int   `json:"buffer_cap"`
	IsClosed        bool  `json:"is_closed"`
}

// Flush 强制刷新缓冲区
func (l *Logger) Flush(ctx context.Context) error {
	// 等待缓冲区清空
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if len(l.buffer) == 0 {
				// 刷新备份
				if err := l.flushBackup(); err != nil {
					return err
				}
				return nil
			}
		}
	}
}

// 便捷方法（保持向后兼容）

// LogLogin 记录登录事件
func (l *Logger) LogLogin(ctx context.Context, tenantID, userID, username, ipAddr, userAgent string, success bool) {
	result := ResultSuccess
	eventType := EventTypeLogin
	if !success {
		result = ResultFailure
		eventType = EventTypeLoginFailed
	}

	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Username:     username,
		Action:       "login",
		ResourceType: "session",
		Result:       result,
		IPAddr:       ipAddr,
		UserAgent:    userAgent,
	})
}

// LogLogout 记录登出事件
func (l *Logger) LogLogout(ctx context.Context, tenantID, userID, username string) {
	l.Log(ctx, &AuditEvent{
		EventType:    EventTypeLogout,
		TenantID:     tenantID,
		UserID:       userID,
		Username:     username,
		Action:       "logout",
		ResourceType: "session",
		Result:       ResultSuccess,
	})
}

// LogRuleChange 记录规则变更
func (l *Logger) LogRuleChange(ctx context.Context, eventType EventType, tenantID, userID, ruleID string, oldValue, newValue interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "rule",
		ResourceID:   ruleID,
		OldValue:     oldValue,
		NewValue:     newValue,
		Result:       ResultSuccess,
	})
}

// LogDeployment 记录部署事件
func (l *Logger) LogDeployment(ctx context.Context, eventType EventType, tenantID, userID, deploymentID string, detail map[string]interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "deployment",
		ResourceID:   deploymentID,
		Detail:       detail,
		Result:       ResultSuccess,
	})
}

// LogPcapAccess 记录PCAP访问
func (l *Logger) LogPcapAccess(ctx context.Context, eventType EventType, tenantID, userID, fileKey string, detail map[string]interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "pcap",
		ResourceID:   fileKey,
		Detail:       detail,
		Result:       ResultSuccess,
	})
}

// LogExport 记录数据导出
func (l *Logger) LogExport(ctx context.Context, eventType EventType, tenantID, userID string, detail map[string]interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "export",
		Detail:       detail,
		Result:       ResultSuccess,
	})
}

// LogAlertAction 记录告警操作
func (l *Logger) LogAlertAction(ctx context.Context, eventType EventType, tenantID, userID, alertID string, oldStatus, newStatus string) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "alert",
		ResourceID:   alertID,
		OldValue:     map[string]string{"status": oldStatus},
		NewValue:     map[string]string{"status": newStatus},
		Result:       ResultSuccess,
	})
}

// LogUserAction 记录用户操作
func (l *Logger) LogUserAction(ctx context.Context, eventType EventType, tenantID, userID, targetUserID string, detail map[string]interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "user",
		ResourceID:   targetUserID,
		Detail:       detail,
		Result:       ResultSuccess,
	})
}

// LogConfigChange 记录配置变更
func (l *Logger) LogConfigChange(ctx context.Context, tenantID, userID, configKey string, oldValue, newValue interface{}) {
	l.Log(ctx, &AuditEvent{
		EventType:    EventTypeConfigUpdate,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       "config_update",
		ResourceType: "config",
		ResourceID:   configKey,
		OldValue:     oldValue,
		NewValue:     newValue,
		Result:       ResultSuccess,
	})
}

// LogError 记录错误事件
func (l *Logger) LogError(ctx context.Context, eventType EventType, tenantID, userID string, err error, detail map[string]interface{}) {
	if detail == nil {
		detail = make(map[string]interface{})
	}
	detail["error"] = err.Error()

	l.Log(ctx, &AuditEvent{
		EventType:    eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       string(eventType),
		ResourceType: "error",
		Detail:       detail,
		Result:       ResultFailure,
		ErrorMsg:     err.Error(),
	})
}

// BackupReader 备份文件读取器（用于重放）
type BackupReader struct {
	file    *os.File
	scanner *bufio.Scanner
}

// NewBackupReader 创建备份读取器
func NewBackupReader(filepath string) (*BackupReader, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open backup file: %w", err)
	}

	return &BackupReader{
		file:    file,
		scanner: bufio.NewScanner(file),
	}, nil
}

// Read 读取下一个事件
func (r *BackupReader) Read() (*AuditEvent, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, nil // EOF
	}

	var event AuditEvent
	if err := json.Unmarshal(r.scanner.Bytes(), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// Close 关闭读取器
func (r *BackupReader) Close() error {
	return r.file.Close()
}

// ReplayBackup 重放备份文件到 Kafka
func ReplayBackup(filepath string, logger *Logger) (int, int, error) {
	reader, err := NewBackupReader(filepath)
	if err != nil {
		return 0, 0, err
	}
	defer reader.Close()

	successCount := 0
	errorCount := 0

	for {
		event, err := reader.Read()
		if err != nil {
			errorCount++
			continue
		}
		if event == nil {
			break // EOF
		}

		// 重新发送到 Kafka
		if err := logger.writeToKafka(event); err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	return successCount, errorCount, nil
}
