package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	alertconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
)

var (
	ErrProbePipelineNotReady    = errors.New("probe pipeline consumer readiness fence is closed")
	ErrProbeReadinessStaleOwner = errors.New("probe pipeline readiness owner or epoch is stale")
)

type ProbePipelineReadinessStore struct {
	db  *sql.DB
	now func() time.Time
}

func NewProbePipelineReadinessStore(db *sql.DB) (*ProbePipelineReadinessStore, error) {
	if db == nil {
		return nil, fmt.Errorf("probe pipeline readiness database is unavailable")
	}
	return &ProbePipelineReadinessStore{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

// IssueRenewRevoke applies one ownership epoch transition. Same-owner renewal
// is monotonic; a stale owner cannot renew or revoke a successor.
func (store *ProbePipelineReadinessStore) IssueRenewRevoke(
	ctx context.Context,
	receipt alertconfig.ProbePipelineReadinessReceipt,
) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("probe pipeline readiness store is unavailable")
	}
	if err := receipt.Validate(store.now()); err != nil {
		return err
	}
	if receipt.State == alertconfig.ProbePipelineRevoked {
		result, err := store.db.ExecContext(ctx, `
			UPDATE probe_pipeline_readiness_epochs
			SET ready=false,lease_expires_at=NULL,revoked_at=$6,observed_at=$6,updated_at=now()
			WHERE pipeline_id=$1 AND consumer_role=$2 AND consumer_group=$3
			  AND owner_id=$4 AND owner_epoch=$5 AND ready=true`,
			receipt.PipelineID, string(receipt.ConsumerRole), receipt.ConsumerGroup,
			receipt.OwnerID, receipt.OwnerEpoch, receipt.ObservedAt)
		if err != nil {
			return fmt.Errorf("revoke probe pipeline readiness epoch: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			var currentGroup, currentOwner string
			var currentEpoch int64
			var currentReady bool
			queryErr := store.db.QueryRowContext(ctx, `
				SELECT consumer_group,owner_id,owner_epoch,ready
				FROM probe_pipeline_readiness_epochs
				WHERE pipeline_id=$1 AND consumer_role=$2`,
				receipt.PipelineID, string(receipt.ConsumerRole),
			).Scan(&currentGroup, &currentOwner, &currentEpoch, &currentReady)
			if queryErr == nil && currentGroup == receipt.ConsumerGroup &&
				currentOwner == receipt.OwnerID && currentEpoch == receipt.OwnerEpoch && !currentReady {
				return nil
			}
			if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
				return fmt.Errorf("load revoked probe pipeline readiness epoch: %w", queryErr)
			}
			return ErrProbeReadinessStaleOwner
		}
		return nil
	}

	var ownerEpoch int64
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO probe_pipeline_readiness_epochs
			(pipeline_id,consumer_role,consumer_group,owner_id,owner_epoch,ready,
			 observed_at,lease_expires_at,revoked_at)
		VALUES ($1,$2,$3,$4,$5,true,$6,$7,NULL)
		ON CONFLICT (pipeline_id,consumer_role) DO UPDATE SET
			consumer_group=EXCLUDED.consumer_group,
			owner_id=EXCLUDED.owner_id,
			owner_epoch=EXCLUDED.owner_epoch,
			ready=true,
			observed_at=EXCLUDED.observed_at,
			lease_expires_at=EXCLUDED.lease_expires_at,
			revoked_at=NULL,
			updated_at=now()
		WHERE EXCLUDED.owner_epoch > probe_pipeline_readiness_epochs.owner_epoch
		   OR (
			 EXCLUDED.owner_epoch=probe_pipeline_readiness_epochs.owner_epoch
			 AND EXCLUDED.owner_id=probe_pipeline_readiness_epochs.owner_id
			 AND EXCLUDED.consumer_group=probe_pipeline_readiness_epochs.consumer_group
			 AND probe_pipeline_readiness_epochs.ready=true
			 AND probe_pipeline_readiness_epochs.revoked_at IS NULL
			 AND EXCLUDED.observed_at >= probe_pipeline_readiness_epochs.observed_at
		   )
		RETURNING owner_epoch`,
		receipt.PipelineID, string(receipt.ConsumerRole), receipt.ConsumerGroup,
		receipt.OwnerID, receipt.OwnerEpoch, receipt.ObservedAt, receipt.LeaseExpiresAt,
	).Scan(&ownerEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProbeReadinessStaleOwner
	}
	if err != nil {
		return fmt.Errorf("issue or renew probe pipeline readiness epoch: %w", err)
	}
	return nil
}

// FenceClaim locks the exact three readiness rows and claims outbox work in
// the same transaction. A concurrent revoke either commits first and blocks
// the claim, or waits until an already-authorized claim commits.
func (store *ProbePipelineReadinessStore) FenceClaim(
	ctx context.Context,
	workerID string,
	limit int,
) ([]probeOperationOutboxItem, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("probe pipeline readiness store is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT consumer_role
		FROM probe_pipeline_readiness_epochs
		WHERE pipeline_id=$1
		  AND consumer_role IN ('COMMAND_DELIVERY','ACK_AUTHORITY','LIFECYCLE_PROJECTION')
		  AND ready=true AND revoked_at IS NULL AND lease_expires_at > now()
		ORDER BY consumer_role
		FOR UPDATE`, alertconfig.ProbeOperationPipelineID)
	if err != nil {
		return nil, err
	}
	readyRoles := make(map[string]struct{}, 3)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return nil, err
		}
		readyRoles[role] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(readyRoles) != 3 {
		return nil, ErrProbePipelineNotReady
	}
	claimedRows, err := queryProbeOperationOutboxClaims(ctx, tx, workerID, limit)
	if err != nil {
		return nil, err
	}
	items, invalidEventIDs, err := scanProbeOperationOutboxClaims(claimedRows, limit)
	if err != nil {
		return items, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, eventID := range invalidEventIDs {
		releaseInvalidProbeOutboxClaim(ctx, store.db, workerID, eventID)
	}
	return items, nil
}

type ProbeDispatcherGate struct {
	store *ProbePipelineReadinessStore
}

func NewProbeDispatcherGate(store *ProbePipelineReadinessStore) (*ProbeDispatcherGate, error) {
	if store == nil {
		return nil, fmt.Errorf("probe pipeline readiness store is required")
	}
	return &ProbeDispatcherGate{store: store}, nil
}

func (gate *ProbeDispatcherGate) AllowClaim(
	ctx context.Context,
	workerID string,
	limit int,
) ([]probeOperationOutboxItem, error) {
	if gate == nil || gate.store == nil {
		return nil, ErrProbePipelineNotReady
	}
	return gate.store.FenceClaim(ctx, workerID, limit)
}
