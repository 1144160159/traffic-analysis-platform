package baseline

import (
	"context"
	"database/sql"
	"fmt"
)

type AckProjector struct {
	pg         *sql.DB
	repository *Repository
}

func NewAckProjector(pg *sql.DB) (*AckProjector, error) {
	if pg == nil {
		return nil, fmt.Errorf("%w: PostgreSQL is required for activation ACK projection", ErrInvalidRequest)
	}
	return &AckProjector{pg: pg, repository: NewRepository()}, nil
}

func (projector *AckProjector) ApplyActivationAck(ctx context.Context, ack ActivationAck) (ActivationReceipt, error) {
	if projector == nil || projector.pg == nil || projector.repository == nil {
		return ActivationReceipt{}, fmt.Errorf("%w: activation ACK projector is not initialized", ErrInvalidRequest)
	}
	tx, err := projector.pg.BeginTx(ctx, nil)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("begin behavior baseline activation ACK: %w", err)
	}
	defer tx.Rollback()
	receipt, err := projector.repository.RecordActivationAckTx(ctx, tx, ack)
	if err != nil {
		return ActivationReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ActivationReceipt{}, fmt.Errorf("commit behavior baseline activation ACK: %w", err)
	}
	return receipt, nil
}
