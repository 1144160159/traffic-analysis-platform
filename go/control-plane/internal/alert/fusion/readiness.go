package fusion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
)

const ProjectionPipelineID = "fusion-projection-v1"

var (
	ErrProjectionNotReady = errors.New("fusion projection consumer is not ready")
	sha256Pattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type ReadinessStore struct {
	db    *sql.DB
	now   func() time.Time
	group string
}

func NewReadinessStore(db *sql.DB, consumerGroup string) (*ReadinessStore, error) {
	if db == nil || strings.TrimSpace(consumerGroup) == "" {
		return nil, fmt.Errorf("fusion readiness database and consumer group are required")
	}
	return &ReadinessStore{db: db, group: strings.TrimSpace(consumerGroup), now: func() time.Time { return time.Now().UTC() }}, nil
}

func (store *ReadinessStore) RecordLifecycle(
	ctx context.Context,
	receipt commonkafka.GroupLifecycleReceipt,
	candidateSHA256 string,
	lease time.Duration,
) error {
	if store == nil || store.db == nil || !sha256Pattern.MatchString(candidateSHA256) ||
		receipt.Topic != SourceSyncTopic || receipt.GroupID != store.group ||
		strings.TrimSpace(receipt.OwnerID) == "" || strings.TrimSpace(receipt.MemberID) == "" ||
		receipt.OwnerEpoch <= 0 || receipt.GenerationID < 0 || receipt.ObservedAt.IsZero() {
		return fmt.Errorf("invalid fusion projection readiness receipt")
	}
	if receipt.State == commonkafka.GroupLifecycleReady {
		if lease < 10*time.Second || lease > 5*time.Minute {
			return fmt.Errorf("fusion readiness lease must be between 10 seconds and 5 minutes")
		}
	} else {
		lease = 0
	}
	assignmentsJSON, err := json.Marshal(receipt.Assignments)
	if err != nil {
		return fmt.Errorf("marshal fusion projection assignments: %w", err)
	}
	ownerID := receipt.OwnerID + ":" + receipt.MemberID
	observedAt := receipt.ObservedAt.UTC()
	var leaseExpires interface{}
	if lease > 0 {
		leaseExpires = observedAt.Add(lease)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fusion readiness receipt: %w", err)
	}
	defer tx.Rollback()
	var currentOwner string
	var currentEpoch int64
	var currentObservedAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT owner_id,owner_epoch,observed_at
		FROM fusion_projection_readiness_current WHERE pipeline_id=$1 AND consumer_group=$2 FOR UPDATE`,
		ProjectionPipelineID, store.group).Scan(&currentOwner, &currentEpoch, &currentObservedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lock fusion readiness current row: %w", err)
	}
	if err == nil {
		if receipt.OwnerEpoch < currentEpoch || (receipt.OwnerEpoch == currentEpoch && observedAt.Before(currentObservedAt)) {
			return nil
		}
		if receipt.OwnerEpoch == currentEpoch && currentOwner != ownerID {
			return fmt.Errorf("fusion readiness owner collision at epoch %d", receipt.OwnerEpoch)
		}
	}
	receiptID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_projection_readiness_history (
		receipt_id,pipeline_id,consumer_group,observed_topic,candidate_sha256,owner_id,owner_epoch,
		generation_id,state,assignments,observed_at,lease_expires_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)`, receiptID, ProjectionPipelineID,
		store.group, SourceSyncTopic, candidateSHA256, ownerID, receipt.OwnerEpoch, receipt.GenerationID,
		string(receipt.State), string(assignmentsJSON), observedAt, leaseExpires)
	if err != nil {
		return fmt.Errorf("append fusion readiness history: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO fusion_projection_readiness_current (
		pipeline_id,consumer_group,observed_topic,candidate_sha256,receipt_id,owner_id,owner_epoch,
		generation_id,state,assignments,observed_at,lease_expires_at,updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,now())
	ON CONFLICT (pipeline_id,consumer_group) DO UPDATE SET
		observed_topic=EXCLUDED.observed_topic,candidate_sha256=EXCLUDED.candidate_sha256,
		receipt_id=EXCLUDED.receipt_id,owner_id=EXCLUDED.owner_id,owner_epoch=EXCLUDED.owner_epoch,
		generation_id=EXCLUDED.generation_id,state=EXCLUDED.state,assignments=EXCLUDED.assignments,
		observed_at=EXCLUDED.observed_at,lease_expires_at=EXCLUDED.lease_expires_at,updated_at=now()`,
		ProjectionPipelineID, store.group, SourceSyncTopic, candidateSHA256, receiptID, ownerID,
		receipt.OwnerEpoch, receipt.GenerationID, string(receipt.State), string(assignmentsJSON), observedAt, leaseExpires)
	if err != nil {
		return fmt.Errorf("update fusion readiness current row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fusion readiness receipt: %w", err)
	}
	return nil
}

func (store *ReadinessStore) AssertReadyTx(ctx context.Context, tx *sql.Tx, candidateSHA256 string) error {
	if store == nil || tx == nil || !sha256Pattern.MatchString(candidateSHA256) {
		return fmt.Errorf("%w: readiness gate inputs are invalid", ErrProjectionNotReady)
	}
	var observedTopic, currentCandidate, state string
	var leaseExpires sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT observed_topic,candidate_sha256,state,lease_expires_at
		FROM fusion_projection_readiness_current WHERE pipeline_id=$1 AND consumer_group=$2 FOR SHARE`,
		ProjectionPipelineID, store.group).Scan(&observedTopic, &currentCandidate, &state, &leaseExpires)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: no assigned consumer generation", ErrProjectionNotReady)
	}
	if err != nil {
		return fmt.Errorf("read fusion projection readiness: %w", err)
	}
	if observedTopic != SourceSyncTopic || currentCandidate != candidateSHA256 || state != string(commonkafka.GroupLifecycleReady) ||
		!leaseExpires.Valid || !leaseExpires.Time.After(store.now()) {
		return fmt.Errorf("%w: current generation is absent, stale, revoked or belongs to another candidate", ErrProjectionNotReady)
	}
	return nil
}
