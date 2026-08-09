package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type modelActionOutboxRecord struct {
	OutboxID         int64
	EventID          string
	JobID            string
	TenantID         string
	ModelID          string
	PartitionKey     string
	AggregateVersion int64
	Payload          []byte
	AttemptCount     int
	ActionID         string
	RequestedBy      string
	TraceID          string
}

func (s *ModelService) processModelActionOutbox(ctx context.Context) error {
	if s.publisher == nil {
		return nil
	}
	for range 20 {
		record, err := s.claimModelActionOutbox(ctx)
		if err != nil || record == nil {
			return err
		}
		err = s.publisher.PublishModelAction(
			ctx, record.PartitionKey, record.EventID, record.TenantID,
			record.JobID, record.ActionID, record.TraceID,
			record.AggregateVersion, record.Payload,
		)
		if err != nil {
			if failureErr := s.failModelActionOutbox(ctx, record, err); failureErr != nil {
				return failureErr
			}
			continue
		}
		if err := s.completeModelActionDispatch(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *ModelService) claimModelActionOutbox(
	ctx context.Context,
) (*modelActionOutboxRecord, error) {
	record := &modelActionOutboxRecord{}
	err := s.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT outbox_id
			FROM model_action_outbox
			WHERE (
			    status='pending' AND available_at<=now()
			  ) OR (
			    status='processing' AND locked_until<now()
			  )
			ORDER BY outbox_id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE model_action_outbox AS outbox
		SET status='processing',attempt_count=attempt_count+1,
		    locked_until=now()+interval '30 seconds',locked_by=$1
		FROM candidate
		WHERE outbox.outbox_id=candidate.outbox_id
		RETURNING outbox.outbox_id,outbox.event_id::text,outbox.job_id,
		          outbox.tenant_id,outbox.model_id::text,outbox.partition_key,
		          outbox.aggregate_version,outbox.payload,outbox.attempt_count,
		          (SELECT action_id FROM model_action_jobs WHERE job_id=outbox.job_id),
		          (SELECT requested_by FROM model_action_jobs WHERE job_id=outbox.job_id),
		          (SELECT trace_id FROM model_action_jobs WHERE job_id=outbox.job_id)
	`, s.outboxWorkerID).Scan(
		&record.OutboxID, &record.EventID, &record.JobID, &record.TenantID,
		&record.ModelID, &record.PartitionKey, &record.AggregateVersion,
		&record.Payload, &record.AttemptCount, &record.ActionID,
		&record.RequestedBy, &record.TraceID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim model action outbox: %w", err)
	}
	return record, nil
}

func (s *ModelService) completeModelActionDispatch(
	ctx context.Context,
	record *modelActionOutboxRecord,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model action dispatch completion: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE model_action_outbox
		SET status='published',published_at=now(),locked_until=NULL,locked_by='',
		    last_error=''
		WHERE outbox_id=$1 AND event_id=$2::uuid
		  AND status='processing' AND locked_by=$3
	`, record.OutboxID, record.EventID, s.outboxWorkerID)
	if err != nil {
		return fmt.Errorf("complete model action outbox: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("model action outbox lease was lost")
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE model_action_jobs
		SET status=CASE
		      WHEN status IN ('queued','running','dispatched') THEN 'dispatched'
		      ELSE status
		    END,
		    result=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN result || '{"delivery":"kafka_acknowledged","business_completed":false}'::jsonb
		      ELSE result
		    END,
		    error=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN ''
		      ELSE error
		    END,
		    updated_at=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN now()
		      ELSE updated_at
		    END
		WHERE job_id=$1 AND event_id=$2::uuid AND tenant_id=$3
		  AND model_id=$4::uuid AND action_id=$5
	`, record.JobID, record.EventID, record.TenantID, record.ModelID, record.ActionID)
	if err != nil {
		return fmt.Errorf("mark model action dispatched: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("model action authoritative job is missing or mismatched")
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"event_id": record.EventID, "job_id": record.JobID,
		"action_id": record.ActionID, "status": "dispatched",
		"kafka_acknowledged": true, "business_completed": false,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			tenant_id,user_id,action,object_type,object_id,detail
		) VALUES ($1,$2,'MODEL_ACTION_DISPATCHED','model',$3,$4::jsonb)
	`, record.TenantID, record.RequestedBy, record.ModelID, string(detail)); err != nil {
		return fmt.Errorf("audit model action dispatch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model action dispatch completion: %w", err)
	}
	return nil
}

func (s *ModelService) failModelActionOutbox(
	ctx context.Context,
	record *modelActionOutboxRecord,
	publishErr error,
) error {
	status := "pending"
	delay := 5 * time.Second * time.Duration(1<<uint(min(record.AttemptCount-1, 6)))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	if record.AttemptCount >= 8 {
		status = "dead"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model action outbox failure: %w", err)
	}
	defer tx.Rollback()
	var receiptExists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM model_action_execution_inbox
		  WHERE event_id=$1::uuid AND job_id=$2
		)
	`, record.EventID, record.JobID).Scan(&receiptExists); err != nil {
		return fmt.Errorf("reconcile model action outbox failure: %w", err)
	}
	if receiptExists {
		result, err := tx.ExecContext(ctx, `
			UPDATE model_action_outbox
			SET status='published',published_at=COALESCE(published_at,now()),
			    locked_until=NULL,locked_by='',
			    last_error='reconciled after publish error: '||$1
			WHERE outbox_id=$2 AND event_id=$3::uuid
			  AND status='processing' AND locked_by=$4
		`, publishErr.Error(), record.OutboxID, record.EventID, s.outboxWorkerID)
		if err != nil {
			return fmt.Errorf("complete reconciled model action outbox: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return fmt.Errorf("model action outbox reconciliation lease was lost")
		}
		detail, _ := json.Marshal(map[string]interface{}{
			"event_id": record.EventID, "job_id": record.JobID,
			"action_id": record.ActionID, "status": "awaiting_executor",
			"receipt_reconciled": true, "business_completed": false,
			"publish_error": publishErr.Error(),
		})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_logs (
				tenant_id,user_id,action,object_type,object_id,detail
			) VALUES ($1,$2,'MODEL_ACTION_DISPATCH_RECONCILED','model',$3,$4::jsonb)
		`, record.TenantID, record.RequestedBy, record.ModelID, string(detail)); err != nil {
			return fmt.Errorf("audit reconciled model action dispatch: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit reconciled model action dispatch: %w", err)
		}
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE model_action_outbox
		SET status=$1,available_at=$2,locked_until=NULL,locked_by='',
		    last_error=$3
		WHERE outbox_id=$4 AND event_id=$5::uuid
		  AND status='processing' AND locked_by=$6
	`, status, time.Now().UTC().Add(delay), publishErr.Error(),
		record.OutboxID, record.EventID, s.outboxWorkerID)
	if err != nil {
		return fmt.Errorf("record model action outbox failure: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("model action outbox failure lease was lost")
	}
	if status == "dead" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_action_jobs
			SET status='failed',error=$1,
			    result=result || '{"business_completed":false}'::jsonb,
			    updated_at=now()
			WHERE job_id=$2 AND event_id=$3::uuid
			  AND status IN ('queued','running','dispatched')
		`, publishErr.Error(), record.JobID, record.EventID); err != nil {
			return fmt.Errorf("fail dead model action: %w", err)
		}
		detail, _ := json.Marshal(map[string]interface{}{
			"event_id": record.EventID, "job_id": record.JobID,
			"action_id": record.ActionID, "status": "failed",
			"stage": "outbox", "error": publishErr.Error(),
		})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_logs (
				tenant_id,user_id,action,object_type,object_id,detail
			) VALUES ($1,$2,'MODEL_ACTION_DISPATCH_FAILED','model',$3,$4::jsonb)
		`, record.TenantID, record.RequestedBy, record.ModelID, string(detail)); err != nil {
			return fmt.Errorf("audit dead model action outbox: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model action outbox failure: %w", err)
	}
	return nil
}
