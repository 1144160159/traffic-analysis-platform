package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
)

const AckPipelineID = "baseline-activation-ack-v1"

var ErrAckConsumerNotReady = errors.New("behavior baseline activation ACK consumer is not ready")

type AckReadinessStore struct {
	db  *sql.DB
	now func() time.Time
}

func NewAckReadinessStore(db *sql.DB) (*AckReadinessStore, error) {
	if db == nil {
		return nil, fmt.Errorf("behavior baseline ACK readiness database is required")
	}
	return &AckReadinessStore{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (store *AckReadinessStore) RecordLifecycle(
	ctx context.Context,
	receipt commonkafka.GroupLifecycleReceipt,
	candidateSHA256 string,
	lease time.Duration,
) error {
	if store == nil || store.db == nil || !sha256Pattern.MatchString(candidateSHA256) ||
		receipt.Topic != ActivationAckTopic || receipt.GroupID != ActivationAckGroup ||
		strings.TrimSpace(receipt.OwnerID) == "" || strings.TrimSpace(receipt.MemberID) == "" ||
		receipt.OwnerEpoch <= 0 || receipt.GenerationID < 0 || receipt.ObservedAt.IsZero() {
		return fmt.Errorf("invalid behavior baseline ACK readiness receipt")
	}
	if receipt.State == commonkafka.GroupLifecycleReady {
		if lease < 10*time.Second || lease > 5*time.Minute {
			return fmt.Errorf("behavior baseline ACK readiness lease must be between 10 seconds and 5 minutes")
		}
	} else {
		lease = 0
	}
	assignmentsJSON, err := json.Marshal(receipt.Assignments)
	if err != nil {
		return fmt.Errorf("marshal behavior baseline ACK assignments: %w", err)
	}
	ownerID := receipt.OwnerID + ":" + receipt.MemberID
	observedAt := receipt.ObservedAt.UTC()
	var expires interface{}
	if lease > 0 {
		expires = observedAt.Add(lease)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin behavior baseline ACK readiness: %w", err)
	}
	defer tx.Rollback()
	var currentOwner string
	var currentEpoch int64
	var currentObserved time.Time
	err = tx.QueryRowContext(ctx, `SELECT owner_id,owner_epoch,observed_at
		FROM behavior_baseline_ack_readiness_current_v1 WHERE pipeline_id=$1 AND consumer_group=$2 FOR UPDATE`,
		AckPipelineID, ActivationAckGroup).Scan(&currentOwner, &currentEpoch, &currentObserved)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lock behavior baseline ACK readiness: %w", err)
	}
	if err == nil {
		if receipt.OwnerEpoch < currentEpoch || (receipt.OwnerEpoch == currentEpoch && observedAt.Before(currentObserved)) {
			return nil
		}
		if receipt.OwnerEpoch == currentEpoch && currentOwner != ownerID {
			return fmt.Errorf("behavior baseline ACK readiness owner collision")
		}
	}
	receiptID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_ack_readiness_history_v1 (
		receipt_id,pipeline_id,consumer_group,observed_topic,candidate_sha256,owner_id,owner_epoch,generation_id,
		state,assignments,observed_at,lease_expires_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)`, receiptID, AckPipelineID, ActivationAckGroup,
		ActivationAckTopic, candidateSHA256, ownerID, receipt.OwnerEpoch, receipt.GenerationID,
		string(receipt.State), string(assignmentsJSON), observedAt, expires)
	if err != nil {
		return fmt.Errorf("append behavior baseline ACK readiness: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO behavior_baseline_ack_readiness_current_v1 (
		pipeline_id,consumer_group,observed_topic,candidate_sha256,receipt_id,owner_id,owner_epoch,generation_id,
		state,assignments,observed_at,lease_expires_at,updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,now())
	ON CONFLICT (pipeline_id,consumer_group) DO UPDATE SET observed_topic=EXCLUDED.observed_topic,
		candidate_sha256=EXCLUDED.candidate_sha256,receipt_id=EXCLUDED.receipt_id,owner_id=EXCLUDED.owner_id,
		owner_epoch=EXCLUDED.owner_epoch,generation_id=EXCLUDED.generation_id,state=EXCLUDED.state,
		assignments=EXCLUDED.assignments,observed_at=EXCLUDED.observed_at,lease_expires_at=EXCLUDED.lease_expires_at,updated_at=now()`,
		AckPipelineID, ActivationAckGroup, ActivationAckTopic, candidateSHA256, receiptID, ownerID,
		receipt.OwnerEpoch, receipt.GenerationID, string(receipt.State), string(assignmentsJSON), observedAt, expires)
	if err != nil {
		return fmt.Errorf("update behavior baseline ACK readiness: %w", err)
	}
	return tx.Commit()
}

func (store *AckReadinessStore) AssertReadyTx(ctx context.Context, tx *sql.Tx, candidateSHA256 string) error {
	if store == nil || tx == nil || !sha256Pattern.MatchString(candidateSHA256) {
		return fmt.Errorf("%w: invalid gate inputs", ErrAckConsumerNotReady)
	}
	var topic, candidate, state string
	var expires sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT observed_topic,candidate_sha256,state,lease_expires_at
		FROM behavior_baseline_ack_readiness_current_v1 WHERE pipeline_id=$1 AND consumer_group=$2 FOR SHARE`,
		AckPipelineID, ActivationAckGroup).Scan(&topic, &candidate, &state, &expires)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: no assigned ACK generation", ErrAckConsumerNotReady)
	}
	if err != nil {
		return fmt.Errorf("read behavior baseline ACK readiness: %w", err)
	}
	if topic != ActivationAckTopic || candidate != candidateSHA256 || state != string(commonkafka.GroupLifecycleReady) ||
		!expires.Valid || !expires.Time.After(store.now()) {
		return fmt.Errorf("%w: ACK generation is stale, revoked or belongs to another candidate", ErrAckConsumerNotReady)
	}
	return nil
}
