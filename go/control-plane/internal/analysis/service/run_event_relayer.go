package service

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// EventPublisher Kafka 权威事件发布端口(测试可桩)。
type EventPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

// RunEventRelayer outbox→Kafka 中继:analysis.run.events.v1 发布 RunSubscription 信封
// (物化即 ACTIVE 订阅,核心卷无 PREPARE/lease 阶段,如实简化),
// 其余权威 topic(plan/report)按原 payload 直发。
type RunEventRelayer struct {
	repo      *repository.Repo
	publisher EventPublisher
	logger    *zap.Logger
	walker    *RunStateWalker
}

// NewRunEventRelayer 构造中继器。
func NewRunEventRelayer(repo *repository.Repo, publisher EventPublisher, logger *zap.Logger) *RunEventRelayer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RunEventRelayer{repo: repo, publisher: publisher, logger: logger}
}

// SetRunStateWalker 注入状态机行走器(订阅发布=PlanReady 事实;nil 时跳过)。
func (r *RunEventRelayer) SetRunStateWalker(w *RunStateWalker) { r.walker = w }

// RelayOnce 处理一批权威事件(plan/run/report 三个 topic,每批上限 limit)。
func (r *RunEventRelayer) RelayOnce(ctx context.Context, limit int) (published int, failed int, err error) {
	if r.publisher == nil {
		return 0, 0, fmt.Errorf("event publisher is not configured")
	}
	for _, topic := range []string{
		commoncontracts.TopicAnalysisPlanEvents,
		commoncontracts.TopicAnalysisRunEvents,
		commoncontracts.TopicAnalysisReportRequests,
	} {
		pub, fail, err := r.relayTopic(ctx, topic, limit)
		published += pub
		failed += fail
		if err != nil {
			return published, failed, err
		}
	}
	return published, failed, nil
}

func (r *RunEventRelayer) relayTopic(ctx context.Context, topic string, limit int) (published int, failed int, err error) {
	rows, err := r.repo.NextPendingOutboxBatch(ctx, topic, limit)
	if err != nil {
		return 0, 0, fmt.Errorf("claim outbox batch: %w", err)
	}
	for _, row := range rows {
		payload, err := r.buildRunSubscriptionPayload(ctx, row)
		if err != nil {
			r.logger.Warn("event payload build failed; marked failed",
				zap.String("event_id", row.EventID), zap.Error(err))
			_ = r.repo.MarkOutboxFailed(ctx, row.ID)
			failed++
			continue
		}
		if err := r.publisher.Publish(ctx, row.Topic, row.Key, payload); err != nil {
			r.logger.Warn("event publish failed",
				zap.String("event_id", row.EventID), zap.Error(err))
			_ = r.repo.MarkOutboxFailed(ctx, row.ID)
			failed++
			continue
		}
		if err := r.repo.MarkOutboxPublished(ctx, row.ID); err != nil {
			r.logger.Warn("mark outbox published failed", zap.Error(err))
			continue
		}
		// ACTIVE 订阅已广播 = PlanReady 事实(PREPARE 只是物化通知,
		// 消费者 exact-set 就绪以 ACTIVE 为准):run 状态行走。
		if r.walker != nil && subscriptionActive(payload) {
			r.walker.Sync(ctx, "", row.Key)
		}
		published++
	}
	return published, failed, nil
}

// buildRunSubscriptionPayload run 事件富化为 RunSubscription 信封;
// plan/report 事件为自包含信封,原样直发(不按 run 富化)。
func (r *RunEventRelayer) buildRunSubscriptionPayload(ctx context.Context, row repository.OutboxRow) ([]byte, error) {
	if row.Topic != commoncontracts.TopicAnalysisRunEvents {
		return row.Payload, nil
	}
	var spec struct {
		TenantID string `json:"tenant_id"`
		RunID    string `json:"run_id"`
		State    string `json:"state"`
	}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &spec); err != nil {
			return nil, fmt.Errorf("decode materialize payload: %w", err)
		}
	}
	// 订阅更新(带 fencing token/revision)已由闸门循环装配:原样直发
	if spec.RunID != "" && spec.State != "" && spec.RunID == row.Key {
		return row.Payload, nil
	}
	var run *repository.RunView
	if spec.TenantID != "" {
		if v, err := r.repo.GetRun(ctx, spec.TenantID, row.Key); err == nil {
			run = v
		}
	}
	if run == nil {
		// 遗留事件回退:payload 无 tenant(早期测试物化行)按 run_id 全租户回源
		legacy, err := r.repo.GetRunByID(ctx, row.Key)
		if err != nil {
			return nil, fmt.Errorf("load run %s: %w", row.Key, err)
		}
		run = legacy
		spec.TenantID = legacy.TenantID
	}
	sub := contract.RunSubscription{
		SchemaVersion:       "1",
		TenantID:            spec.TenantID,
		RunID:               row.Key,
		Revision:            run.Revision,
		State:               "ACTIVE",
		ExecutionSpecSHA256: run.ExecutionSpecSHA256,
		WindowStartMs:       run.WindowStartMs,
		WindowEndMs:         run.WindowEndMs,
	}
	// 物化时已预分配流水线 fence:rev-1 订阅即携带,消除"流先于订阅广播"
	// 竞态(否则回执回显空 fence 被 stale-fence 隔离)。
	if fence, err := r.repo.PipelinedFenceToken(ctx, row.Key); err == nil && fence != "" {
		sub.Fence = json.RawMessage(fmt.Sprintf("%q", fence))
	}
	payload, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("marshal run subscription: %w", err)
	}
	return payload, nil
}


// subscriptionActive 判断已发布订阅信封是否为 ACTIVE(两段订阅:物化发 PREPARE,
// S1 领取发 ACTIVE;PlanReady 只认 ACTIVE)。
func subscriptionActive(payload []byte) bool {
	var sub struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(payload, &sub); err != nil {
		return false
	}
	return sub.State == "ACTIVE"
}
