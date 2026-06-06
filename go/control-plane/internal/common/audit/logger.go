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

type Logger struct {
	kafkaWriter *kafka.Writer
	logger      *zap.Logger
	serviceName string
	buffer      chan *AuditEvent
	wg          sync.WaitGroup
	closed      int32
	mu          sync.RWMutex

	config Config

	backupDir     string
	backupEnabled bool
	backupFile    *os.File
	backupWriter  *bufio.Writer
	backupMu      sync.Mutex

	sentCount       int64
	droppedCount    int64
	errorCount      int64
	backupCount     int64
	batchSentCount  int64
	batchEventCount int64
}

type Config struct {
	KafkaBrokers    []string      `env:"KAFKA_BROKERS" envSeparator:","`
	Topic           string        `env:"AUDIT_TOPIC" envDefault:"audit.logs"`
	ServiceName     string        `env:"SERVICE_NAME"`
	BufferSize      int           `env:"AUDIT_BUFFER_SIZE" envDefault:"1000"`
	BatchSize       int           `env:"AUDIT_BATCH_SIZE" envDefault:"100"`
	FlushInterval   time.Duration `env:"AUDIT_FLUSH_INTERVAL" envDefault:"1s"`
	ShutdownTimeout time.Duration `env:"AUDIT_SHUTDOWN_TIMEOUT" envDefault:"10s"`

	BackupEnabled bool   `env:"AUDIT_BACKUP_ENABLED" envDefault:"true"`
	BackupDir     string `env:"AUDIT_BACKUP_DIR" envDefault:"/var/log/audit"`

	MaxRetries   int           `env:"AUDIT_MAX_RETRIES" envDefault:"3"`
	RetryBackoff time.Duration `env:"AUDIT_RETRY_BACKOFF" envDefault:"100ms"`

	EnableBatchOptimization bool          `env:"AUDIT_ENABLE_BATCH_OPTIMIZATION" envDefault:"true"`
	MaxBatchWaitTime        time.Duration `env:"AUDIT_MAX_BATCH_WAIT_TIME" envDefault:"500ms"`
}

func NewLogger(cfg Config, logger *zap.Logger) (*Logger, error) {
	if len(cfg.KafkaBrokers) == 0 {
		return nil, nil
	}

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
		Async:        false,
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

	if cfg.BackupEnabled {
		if err := l.initBackup(); err != nil {
			logger.Warn("Failed to initialize audit backup, backup disabled", zap.Error(err))
		} else {
			l.backupEnabled = true
		}
	}

	l.wg.Add(1)
	go l.processBuffer()

	return l, nil
}

func (l *Logger) initBackup() error {

	if err := os.MkdirAll(l.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

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

func (l *Logger) rotateBackupFile() error {
	l.backupMu.Lock()
	defer l.backupMu.Unlock()

	if l.backupFile == nil {
		return l.initBackup()
	}

	currentDate := time.Now().Format("2006-01-02")
	expectedFilename := fmt.Sprintf("%s_%s.jsonl", l.serviceName, currentDate)
	expectedPath := filepath.Join(l.backupDir, expectedFilename)

	if l.backupFile.Name() != expectedPath {

		l.backupWriter.Flush()
		l.backupFile.Close()

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

func (l *Logger) writeToBackup(event *AuditEvent) error {
	if !l.backupEnabled || l.backupWriter == nil {
		return nil
	}

	l.backupMu.Lock()
	defer l.backupMu.Unlock()

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

func (l *Logger) writeToBackupBatch(events []*AuditEvent) error {
	if !l.backupEnabled || l.backupWriter == nil {
		return nil
	}

	l.backupMu.Lock()
	defer l.backupMu.Unlock()

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

func (l *Logger) Log(ctx context.Context, event *AuditEvent) {
	if atomic.LoadInt32(&l.closed) == 1 {
		atomic.AddInt64(&l.droppedCount, 1)
		return
	}

	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.ServiceName == "" {
		event.ServiceName = l.serviceName
	}

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

	if event.Sensitivity == "" {
		info := GetEventTypeInfo(event.EventType)
		event.Sensitivity = info.Sensitivity
	}

	select {
	case l.buffer <- event:

	default:

		atomic.AddInt64(&l.droppedCount, 1)
		l.logger.Warn("Audit buffer full, writing to backup",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)))

		if err := l.writeToBackup(event); err != nil {
			l.logger.Error("Failed to write to backup", zap.Error(err))
		}
	}
}

func (l *Logger) LogBatch(ctx context.Context, events []*AuditEvent) {
	if atomic.LoadInt32(&l.closed) == 1 {
		atomic.AddInt64(&l.droppedCount, int64(len(events)))
		return
	}

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

		if event.Sensitivity == "" {
			info := GetEventTypeInfo(event.EventType)
			event.Sensitivity = info.Sensitivity
		}
	}

	droppedCount := 0
	for _, event := range events {
		select {
		case l.buffer <- event:

		default:
			droppedCount++
		}
	}

	if droppedCount > 0 {
		atomic.AddInt64(&l.droppedCount, int64(droppedCount))
		l.logger.Warn("Audit buffer full, some events dropped",
			zap.Int("dropped", droppedCount),
			zap.Int("total", len(events)))

		droppedEvents := events[len(events)-droppedCount:]
		if err := l.writeToBackupBatch(droppedEvents); err != nil {
			l.logger.Error("Failed to write dropped events to backup", zap.Error(err))
		}
	}
}

func (l *Logger) processBuffer() {
	defer l.wg.Done()

	flushTicker := time.NewTicker(5 * time.Second)
	defer flushTicker.Stop()

	if l.config.EnableBatchOptimization {
		l.processBufferBatched(flushTicker)
	} else {
		l.processBufferSingle(flushTicker)
	}
}

func (l *Logger) processBufferSingle(flushTicker *time.Ticker) {
	for {
		select {
		case event, ok := <-l.buffer:
			if !ok {

				return
			}
			if err := l.writeToKafka(event); err != nil {
				atomic.AddInt64(&l.errorCount, 1)
				l.logger.Error("Failed to write audit event to Kafka",
					zap.String("event_id", event.EventID),
					zap.Error(err))

				if backupErr := l.writeToBackup(event); backupErr != nil {
					l.logger.Error("Failed to write to backup after Kafka failure",
						zap.String("event_id", event.EventID),
						zap.Error(backupErr))
				}
			} else {
				atomic.AddInt64(&l.sentCount, 1)
			}

		case <-flushTicker.C:

			if err := l.flushBackup(); err != nil {
				l.logger.Warn("Failed to flush backup", zap.Error(err))
			}
		}
	}
}

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

			if backupErr := l.writeToBackupBatch(batch); backupErr != nil {
				l.logger.Error("Failed to write batch to backup after Kafka failure",
					zap.Error(backupErr))
			}
		} else {
			atomic.AddInt64(&l.sentCount, int64(len(batch)))
			atomic.AddInt64(&l.batchSentCount, 1)
			atomic.AddInt64(&l.batchEventCount, int64(len(batch)))
		}

		batch = batch[:0]

		batchTimer.Reset(l.config.MaxBatchWaitTime)
	}

	for {
		select {
		case event, ok := <-l.buffer:
			if !ok {

				flushBatch()
				return
			}

			batch = append(batch, event)

			if len(batch) >= l.config.BatchSize {
				flushBatch()
			}

		case <-batchTimer.C:

			flushBatch()

		case <-flushTicker.C:

			if err := l.flushBackup(); err != nil {
				l.logger.Warn("Failed to flush backup", zap.Error(err))
			}
		}
	}
}

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

func (l *Logger) Close() error {

	if !atomic.CompareAndSwapInt32(&l.closed, 0, 1) {
		return nil
	}

	l.logger.Info("Closing audit logger, waiting for buffer to drain...",
		zap.Int("buffer_size", len(l.buffer)))

	remainingMessages := l.drainBuffer()

	close(l.buffer)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

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

		l.saveRemainingToBackup(remainingMessages)
	}

	if l.backupEnabled {
		if err := l.flushBackup(); err != nil {
			l.logger.Error("Failed to flush backup on close", zap.Error(err))
		}
		l.closeBackup()
	}

	return l.kafkaWriter.Close()
}

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

type LoggerStats struct {
	SentCount       int64 `json:"sent_count"`
	DroppedCount    int64 `json:"dropped_count"`
	ErrorCount      int64 `json:"error_count"`
	BackupCount     int64 `json:"backup_count"`
	BatchSentCount  int64 `json:"batch_sent_count"`
	BatchEventCount int64 `json:"batch_event_count"`
	BufferSize      int   `json:"buffer_size"`
	BufferCap       int   `json:"buffer_cap"`
	IsClosed        bool  `json:"is_closed"`
}

func (l *Logger) Flush(ctx context.Context) error {

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if len(l.buffer) == 0 {

				if err := l.flushBackup(); err != nil {
					return err
				}
				return nil
			}
		}
	}
}

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

type BackupReader struct {
	file    *os.File
	scanner *bufio.Scanner
}

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

func (r *BackupReader) Read() (*AuditEvent, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	var event AuditEvent
	if err := json.Unmarshal(r.scanner.Bytes(), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

func (r *BackupReader) Close() error {
	return r.file.Close()
}

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
			break
		}

		if err := logger.writeToKafka(event); err != nil {
			errorCount++
		} else {
			successCount++
		}
	}

	return successCount, errorCount, nil
}
