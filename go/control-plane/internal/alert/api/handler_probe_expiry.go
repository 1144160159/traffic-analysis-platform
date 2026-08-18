package api

import (
	"context"
	"fmt"
)

const expireProbeOperationsSQL = `WITH candidates AS (
	SELECT p.operation_id,p.status AS from_status
	FROM probe_operations p
	WHERE p.status IN ('accepted','delivered') AND p.expires_at <= now()
	ORDER BY p.expires_at,p.operation_id
	LIMIT $1
	FOR UPDATE SKIP LOCKED
), expired AS (
	UPDATE probe_operations p
	SET status='expired',state_revision=p.state_revision+1,
	    ack_error='operation expired before ACK',updated_at=now()
	FROM candidates c
	WHERE p.operation_id=c.operation_id
	RETURNING p.operation_id,p.tenant_id,p.probe_id,p.command_revision,
	          p.state_revision,COALESCE(NULLIF(p.trace_id,''),p.operation_id::text) AS trace_id,
	          c.from_status,p.expires_at,uuid_generate_v4() AS event_id
), history AS (
	INSERT INTO probe_operation_history
		(operation_id,tenant_id,state_revision,from_status,to_status,detail)
	SELECT operation_id,tenant_id,state_revision,from_status,'expired',
	       jsonb_build_object(
	         'reason','operation expired before ACK',
	         'event_id',event_id,
	         'event_type','traffic.probe.v2.OperationExpired'
	       )
	FROM expired
	RETURNING operation_id,state_revision
)
INSERT INTO probe_operation_outbox
	(event_id,operation_id,tenant_id,event_type,partition_key,
	 aggregate_version,schema_version,payload)
SELECT e.event_id,e.operation_id,e.tenant_id,
	   'traffic.probe.v2.OperationExpired',e.tenant_id||':'||e.probe_id,
	   e.state_revision,2,
	   jsonb_build_object(
	     'event_id',e.event_id,
	     'event_type','traffic.probe.v2.OperationExpired',
	     'schema_version',2,
	     'tenant_id',e.tenant_id,
	     'probe_id',e.probe_id,
	     'operation_id',e.operation_id,
	     'command_revision',e.command_revision,
	     'state_revision',e.state_revision,
	     'revision',e.state_revision,
	     'status','expired',
	     'trace_id',e.trace_id,
	     'reason','operation expired before ACK',
	     'expired_at',e.expires_at
	   )
FROM expired e
JOIN history h USING (operation_id,state_revision)`

// expireProbeOperations is the sole authority for the accepted/delivered to
// expired transition. State, history and the distinct lifecycle outbox event
// either commit together or remain entirely absent.
func (h *SystemHandler) expireProbeOperations(ctx context.Context, limit int) (int, error) {
	if h.pgDB == nil {
		return 0, fmt.Errorf("probe operation expiry database is unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx, err := h.pgDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin probe operation expiry transaction: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, expireProbeOperationsSQL, limit)
	if err != nil {
		return 0, fmt.Errorf("expire probe operations atomically: %w", err)
	}
	expired, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect expired probe operation count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit probe operation expiry transaction: %w", err)
	}
	return int(expired), nil
}
