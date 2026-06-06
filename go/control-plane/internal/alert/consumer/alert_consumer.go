package consumer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/persistence"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// EvidenceGenerator 自动生成证据（由 AlertConsumer 调用）
type EvidenceGenerator interface {
	GenerateForAlert(ctx context.Context, alert *persistence.Alert) ([]string, error)
}

// AlertConsumer 从 Kafka 消费告警，去重后批量写入 ClickHouse + 自动生成证据
type AlertConsumer struct {
	consumer      *kafka.Consumer
	repo          *repository.AlertRepository
	dedup         *dedup.RedisDedup
	evidenceGen   EvidenceGenerator // 自动证据生成器
	logger        *zap.Logger
	topic         string
	groupID       string
	batchSize     int
	flushInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAlertConsumer 创建告警消费者
func NewAlertConsumer(
	consumer *kafka.Consumer,
	repo *repository.AlertRepository,
	dedupSvc *dedup.RedisDedup,
	logger *zap.Logger,
	topic, groupID string,
) *AlertConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &AlertConsumer{
		consumer: consumer, repo: repo, dedup: dedupSvc,
		logger: logger, topic: topic, groupID: groupID,
		batchSize: 100, flushInterval: 2 * time.Second,
		ctx: ctx, cancel: cancel,
	}
}

// SetEvidenceGenerator 设置证据生成器（由 main.go 在初始化后注入）
func (c *AlertConsumer) SetEvidenceGenerator(gen EvidenceGenerator) {
	c.evidenceGen = gen
}

// Start 启动消费循环
func (c *AlertConsumer) Start(ctx context.Context) error {
	c.logger.Info("Alert consumer starting",
		zap.String("topic", c.topic),
		zap.String("group_id", c.groupID))

	return c.consumer.BatchConsume(ctx, c.batchSize, c.flushInterval, c.handleBatch)
}

// StartAsync 异步启动
func (c *AlertConsumer) StartAsync(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.Start(ctx); err != nil && err != context.Canceled {
			c.logger.Error("Alert consumer stopped with error", zap.Error(err))
		}
	}()
}

// Stop 优雅停止
func (c *AlertConsumer) Stop() {
	c.logger.Info("Stopping alert consumer")
	c.cancel()
	c.wg.Wait()
	c.logger.Info("Alert consumer stopped")
}

// handleBatch 批量处理告警消息
func (c *AlertConsumer) handleBatch(ctx context.Context, messages []*kafka.ReceivedMessage) error {
	if len(messages) == 0 {
		return nil
	}

	alerts := make([]*persistence.Alert, 0, len(messages))
	now := time.Now()

	for _, msg := range messages {
		alert, err := c.processMessage(ctx, msg, now)
		if err != nil {
			c.logger.Warn("Failed to process alert message, skipping",
				zap.Int64("offset", msg.Offset),
				zap.Int("partition", msg.Partition),
				zap.Error(err))
			continue
		}
		if alert != nil {
			alerts = append(alerts, alert)
		}
	}

	if len(alerts) == 0 {
		return nil
	}

	// 自动生成证据（业务闭环：Alert → Evidence 自动关联）
	if c.evidenceGen != nil {
		for _, alert := range alerts {
			evidenceIDs, err := c.evidenceGen.GenerateForAlert(ctx, alert)
			if err != nil {
				c.logger.Warn("Failed to generate evidence for alert",
					zap.String("alert_id", alert.AlertID), zap.Error(err))
			} else if len(evidenceIDs) > 0 {
				alert.EvidenceIDs = append(alert.EvidenceIDs, evidenceIDs...)
			}
		}
	}

	if err := c.repo.BatchUpsertAlerts(ctx, alerts); err != nil {
		return fmt.Errorf("batch upsert alerts: %w", err)
	}

	c.logger.Debug("Alert batch processed",
		zap.Int("messages", len(messages)),
		zap.Int("alerts", len(alerts)))
	return nil
}

// processMessage 处理单条消息
func (c *AlertConsumer) processMessage(ctx context.Context, msg *kafka.ReceivedMessage, now time.Time) (*persistence.Alert, error) {
	tenantID := msg.TenantID()
	if tenantID == "" {
		tenantID = "default"
	}

	// 反序列化
	pbAlert, err := c.unmarshalAlert(msg.Value)
	if err != nil {
		return nil, err
	}
	if pbAlert == nil {
		return nil, nil
	}

	// 计算去重指纹
	fingerprint := dedup.CalculateAlertFingerprint(
		tenantID,
		pbAlert.AlertType,
		pbAlert.SrcIp,
		pbAlert.DstIp,
		pbAlert.DstPort,
		pbAlert.Severity.String(),
		pbAlert.FirstSeen,
		5, // 5 分钟时间桶
	)

	// 去重检查（如果 dedup 可用）
	eventTs := pbAlert.FirstSeen
	if eventTs == 0 {
		eventTs = now.UnixMilli()
	}

	isNew := true
	var count int64

	if c.dedup != nil {
		result, err := c.dedup.CheckAndIncrementWithTenant(ctx, fingerprint, eventTs, tenantID)
		if err != nil {
			c.logger.Warn("Dedup check failed, treating as new",
				zap.String("fingerprint", fingerprint),
				zap.Error(err))
		} else {
			isNew = result.IsNew
			count = result.Count
		}
	}

	if !isNew {
		return nil, nil // 已去重
	}

	// 构建持久化 Alert
	alertID := pbAlert.AlertId
	if alertID == "" {
		alertID = uuid.New().String()
	}

	firstSeen := time.UnixMilli(pbAlert.FirstSeen)
	if firstSeen.IsZero() {
		firstSeen = now
	}
	lastSeen := time.UnixMilli(pbAlert.LastSeen)
	if lastSeen.IsZero() {
		lastSeen = now
	}

	severity := pbAlert.Severity.String()
	if severity == "" || severity == "SEVERITY_UNSPECIFIED" {
		severity = "medium"
	}

	status := pbAlert.Status.String()
	if status == "" || status == "ALERT_STATUS_UNSPECIFIED" {
		status = "new"
	}

	labels := pbAlert.Labels
	if labels == nil {
		labels = []string{}
	}
	evidenceIDs := pbAlert.EvidenceIds
	if evidenceIDs == nil {
		evidenceIDs = []string{}
	}

	if count == 0 {
		count = 1
	}

	return &persistence.Alert{
		TenantID:     tenantID,
		AlertID:      alertID,
		Fingerprint:  fingerprint,
		CommunityID:  pbAlert.CommunityId,
		SessionID:    pbAlert.SessionId,
		CampaignID:   pbAlert.CampaignId,
		SrcIP:        pbAlert.SrcIp,
		DstIP:        pbAlert.DstIp,
		SrcPort:      uint16(pbAlert.SrcPort),
		DstPort:      uint16(pbAlert.DstPort),
		Protocol:     uint8(pbAlert.Protocol),
		AlertType:    pbAlert.AlertType,
		Labels:       labels,
		Score:        pbAlert.Score,
		Severity:     severity,
		FirstSeen:    firstSeen,
		LastSeen:     lastSeen,
		Count:        int32(count),
		Status:       status,
		Assignee:     pbAlert.Assignee,
		UpdatedTs:    now,
		ModelVersion: pbAlert.ModelVersion,
		RuleVersion:  pbAlert.RuleVersion,
		FeatureSetID: pbAlert.FeatureSetId,
		EvidenceIDs:  evidenceIDs,
		EventID:      alertID,
	}, nil
}

// unmarshalAlert 反序列化告警消息（支持 Alert 和 DetectionBatch 格式）
func (c *AlertConsumer) unmarshalAlert(data []byte) (*pb.Alert, error) {
	// 尝试 Alert
	var alert pb.Alert
	if err := proto.Unmarshal(data, &alert); err == nil && alert.AlertId != "" {
		return &alert, nil
	}

	// 尝试 DetectionBatch
	var batch pb.DetectionBatch
	if err := proto.Unmarshal(data, &batch); err == nil {
		// 从 DetectionBatch 提取告警信息
		if len(batch.Businesses) > 0 {
			biz := batch.Businesses[0]
			return &pb.Alert{
				TenantId:    batch.TenantId,
				AlertId:     uuid.New().String(),
				CommunityId: biz.CommunityId,
				SessionId:   biz.SessionId,
				CampaignId:  biz.CampaignId,
				AlertType:   biz.DetectionType,
				Labels:      []string{biz.Label},
				Score:       biz.Score,
				Severity:    pb.Severity_SEVERITY_MEDIUM,
				FirstSeen:   biz.Ts,
				LastSeen:    biz.Ts,
				RuleVersion: biz.RuleVersion,
				ModelVersion: biz.ModelVersion,
			}, nil
		}
	}

	return nil, fmt.Errorf("unmarshal alert: unknown format")
}
