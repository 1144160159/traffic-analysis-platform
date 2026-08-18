package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
)

// OutboxRow 待发布事件行(权威 topic 出站)。
type OutboxRow struct {
	ID      int64
	EventID string
	Topic   string
	Key     string
	Payload json.RawMessage
}

// NextPendingOutboxBatch 领取 topic 的 PENDING 事件(FOR UPDATE SKIP LOCKED,多副本安全)。
func (r *Repo) NextPendingOutboxBatch(ctx context.Context, topic string, limit int) ([]OutboxRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_id, topic, key, payload FROM analysis_outbox
		WHERE topic=$1 AND state='PENDING' AND next_attempt_at <= now()
		ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED`, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxRow
	for rows.Next() {
		var o OutboxRow
		if err := rows.Scan(&o.ID, &o.EventID, &o.Topic, &o.Key, &o.Payload); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// MarkOutboxPublished 发布成功:state=PUBLISHED + broker ACK 台账
// (RequiredAcks=all 的 nil 返回 = 全 ISR broker ACK 事实;published_at 记录)。
func (r *Repo) MarkOutboxPublished(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE analysis_outbox SET state='PUBLISHED', broker_ack=true, published_at=now()
		WHERE id=$1 AND state='PENDING'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("outbox CAS failed for id %d", id)
	}
	return nil
}

// OutboxLedgerRow outbox ACK 台账视图(对账/告警用)。
type OutboxLedgerRow struct {
	Topic                   string  `json:"topic"`
	State                   string  `json:"state"`
	Count                   int64   `json:"count"`
	OldestPendingAgeSeconds float64 `json:"oldest_pending_age_seconds"`
}

// OutboxLedger 按 topic/state 聚合 outbox 台账(旧 PENDING 即投递停滞告警输入)。
func (r *Repo) OutboxLedger(ctx context.Context) ([]OutboxLedgerRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT topic, state, count(*),
			COALESCE((SELECT EXTRACT(EPOCH FROM now() - MIN(next_attempt_at))
				FROM analysis_outbox o2 WHERE o2.topic=o1.topic AND o2.state='PENDING'), 0)
		FROM analysis_outbox o1 GROUP BY topic, state ORDER BY topic, state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxLedgerRow
	for rows.Next() {
		var o OutboxLedgerRow
		if err := rows.Scan(&o.Topic, &o.State, &o.Count, &o.OldestPendingAgeSeconds); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// MarkOutboxFailed 发布失败:attempts+1 + 指数退避(next_attempt_at 上限 5 分钟)。
func (r *Repo) MarkOutboxFailed(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE analysis_outbox
		SET attempts=attempts+1, next_attempt_at=now() + (LEAST(POWER(2, attempts)::int, 300) * interval '1 second')
		WHERE id=$1 AND state='PENDING'`, id)
	return err
}

// EnqueueRunSubscriptionUpdate 把更新后的 RunSubscription(带 fencing token)重新入 outbox,
// 由中继发布到 analysis.run.events.v1(路由器按 revision 单调更新订阅状态)。
func (r *Repo) EnqueueRunSubscriptionUpdate(ctx context.Context, sub contract.RunSubscription) error {
	payload, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO analysis_outbox(event_id, topic, key, payload)
		VALUES(gen_random_uuid()::text, $1, $2, $3::jsonb)`,
		commoncontracts.TopicAnalysisRunEvents, sub.RunID, string(payload))
	return err
}
