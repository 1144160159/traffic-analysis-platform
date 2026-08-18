package sourcequality

import (
	"context"
	"database/sql"
	"fmt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Record persists the receipt in the existing append-only audit authority.
// It deliberately does not create or mutate M07 repair/baseline governance rows.
func (r *Repository) Record(ctx context.Context, receipt Receipt) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("source quality receipt repository is unavailable")
	}
	detail, err := receipt.CanonicalDetail()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source quality receipt transaction: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent
		) VALUES ($1,$2,$3,$4,'source_quality_receipt',$5,$6::jsonb,'','')
		ON CONFLICT (event_id) DO UPDATE SET event_id=EXCLUDED.event_id
		WHERE audit_logs.tenant_id=EXCLUDED.tenant_id
		  AND audit_logs.user_id IS NOT DISTINCT FROM EXCLUDED.user_id
		  AND audit_logs.action=EXCLUDED.action
		  AND audit_logs.object_type=EXCLUDED.object_type
		  AND audit_logs.object_id IS NOT DISTINCT FROM EXCLUDED.object_id
		  AND audit_logs.detail=EXCLUDED.detail
		  AND audit_logs.ip_addr IS NOT DISTINCT FROM EXCLUDED.ip_addr
		  AND audit_logs.user_agent IS NOT DISTINCT FROM EXCLUDED.user_agent`,
		receipt.ReceiptID,
		receipt.TenantID,
		"system:"+receipt.ConsumerGroup,
		"SOURCE_QUALITY_"+string(receipt.Category),
		receipt.ReceiptID,
		string(detail),
	)
	if err != nil {
		return fmt.Errorf("persist source quality receipt: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect source quality receipt write: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("%w: receipt_id=%s", ErrReceiptConflict, receipt.ReceiptID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source quality receipt: %w", err)
	}
	return nil
}

// RecordBeforeOffsetCommit is the consumer barrier: a failed durable receipt
// prevents the caller from invoking its source-offset commit callback.
func (r *Repository) RecordBeforeOffsetCommit(
	ctx context.Context,
	receipt Receipt,
	commitOffset func(context.Context, SourceTuple) error,
) error {
	if commitOffset == nil {
		return fmt.Errorf("source offset commit callback is required")
	}
	if err := r.Record(ctx, receipt); err != nil {
		return err
	}
	if err := commitOffset(ctx, receipt.Source); err != nil {
		return fmt.Errorf("source quality receipt is durable but offset commit failed: %w", err)
	}
	return nil
}
