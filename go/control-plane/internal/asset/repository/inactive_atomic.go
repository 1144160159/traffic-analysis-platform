package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var ErrAssetInactiveIdempotencyConflict = errors.New("asset inactive idempotency conflict")

type assetInactiveIdentity struct {
	TenantID string `json:"tenant_id"`
	ActionID string `json:"action_id"`
	Actor    string `json:"actor"`
	Reason   string `json:"reason"`
	Cutoff   string `json:"cutoff"`
}

// MarkInactiveSinceAtomic applies one lifecycle sweep as one PostgreSQL
// transaction. Every changed asset receives the same-transaction revision,
// history, audit and projection outbox record before the batch result is made
// replayable through asset_inactive_requests.
func (r *AssetRepository) MarkInactiveSinceAtomic(
	ctx context.Context,
	tenantID string,
	command config.AssetInactiveCommand,
) (*config.AssetInactiveResult, error) {
	actionID := command.ActionID
	if actionID == "" {
		actionID = config.AssetInactiveSweepAction
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "asset inactivity lifecycle sweep"
	}
	cutoff := command.Cutoff.UTC()
	requestJSON, err := json.Marshal(assetInactiveIdentity{
		TenantID: tenantID,
		ActionID: actionID,
		Actor:    command.Actor,
		Reason:   reason,
		Cutoff:   cutoff.Format(timeFormatRFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal inactive sweep identity: %w", err)
	}
	digest := sha256.Sum256(requestJSON)
	requestHash := hex.EncodeToString(digest[:])

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin inactive sweep: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"asset-inactive-idem:"+tenantID+":"+command.IdempotencyKey,
	); err != nil {
		return nil, fmt.Errorf("lock inactive sweep identity: %w", err)
	}

	var priorHash, priorActor, priorTrace string
	var priorPayload []byte
	err = tx.QueryRowContext(ctx, `
		SELECT request_hash,actor,result_payload::text,trace_id
		FROM asset_inactive_requests
		WHERE tenant_id=$1 AND idempotency_key=$2
		FOR UPDATE`, tenantID, command.IdempotencyKey,
	).Scan(&priorHash, &priorActor, &priorPayload, &priorTrace)
	if err == nil {
		if priorHash != requestHash || priorActor != command.Actor {
			return nil, ErrAssetInactiveIdempotencyConflict
		}
		var prior config.AssetInactiveResult
		if err := json.Unmarshal(priorPayload, &prior); err != nil {
			return nil, fmt.Errorf("decode inactive sweep replay: %w", err)
		}
		prior.TraceID = priorTrace
		prior.IdempotentReplay = true
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit inactive sweep replay: %w", err)
		}
		return &prior, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("read inactive sweep replay: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"asset-inactive-tenant:"+tenantID,
	); err != nil {
		return nil, fmt.Errorf("lock inactive sweep tenant: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT asset_id,revision,display_code,tenant_id,asset_type,status,ip_address,mac_address,
		       hostname,vendor,os_type,source,vlan_id,switch_port,department,campus,owner,
		       criticality,tags,metadata,first_seen,last_seen
		FROM assets
		WHERE tenant_id=$1 AND last_seen<$2 AND status IS DISTINCT FROM 'inactive'
		ORDER BY asset_id
		FOR UPDATE`, tenantID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("select inactive sweep candidates: %w", err)
	}
	candidates := make([]*config.AssetRecord, 0)
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("scan inactive sweep candidate: %w", scanErr)
		}
		candidates = append(candidates, asset)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate inactive sweep candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close inactive sweep candidates: %w", err)
	}

	result := &config.AssetInactiveResult{TraceID: command.TraceID, EventIDs: make([]string, 0, len(candidates))}
	for _, asset := range candidates {
		oldJSON, marshalErr := json.Marshal(asset)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal inactive old asset: %w", marshalErr)
		}
		var revision int64
		if err := tx.QueryRowContext(ctx, `
			UPDATE assets
			SET status='inactive',revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND asset_id=$2 AND revision=$3
			RETURNING revision`, tenantID, asset.AssetID, asset.Revision,
		).Scan(&revision); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrAssetRevisionConflict
			}
			return nil, fmt.Errorf("mark asset inactive: %w", err)
		}
		asset.Status = "inactive"
		asset.Revision = revision
		newJSON, marshalErr := json.Marshal(asset)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal inactive new asset: %w", marshalErr)
		}
		eventID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO asset_events (
			  event_uuid,asset_id,tenant_id,event_type,revision,trace_id,old_value,new_value
			) VALUES ($1,$2,$3,'inactive',$4,$5,$6,$7)`,
			eventID, asset.AssetID, tenantID, revision, command.TraceID, oldJSON, newJSON,
		); err != nil {
			return nil, fmt.Errorf("insert inactive history: %w", err)
		}
		auditDetail, _ := json.Marshal(map[string]any{
			"action_id": actionID, "reason": reason, "cutoff": cutoff,
			"revision": revision, "event_id": eventID,
			"idempotency_key": command.IdempotencyKey, "trace_id": command.TraceID,
		})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_logs (
			  event_id,tenant_id,user_id,action,object_type,object_id,detail,
			  request_id,trace_id,success,risk_level,result
			) VALUES ($1,$2,$3,'ASSET_INACTIVE_SWEEP','asset',$4,$5,$6,$7,true,'medium','accepted')`,
			"asset-audit-"+eventID, tenantID, command.Actor, asset.AssetID,
			auditDetail, command.RequestID, command.TraceID,
		); err != nil {
			return nil, fmt.Errorf("insert inactive audit: %w", err)
		}
		payload, marshalErr := json.Marshal(map[string]any{
			"action_id": actionID, "event_id": eventID,
			"event_type": "traffic.asset.v2.AssetUpserted", "schema_version": 2,
			"aggregate_version": revision, "partition_key": tenantID + ":" + asset.AssetID,
			"tenant_id": tenantID, "asset_id": asset.AssetID, "revision": revision,
			"trace_id": command.TraceID, "reason": reason, "asset": asset,
		})
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal inactive outbox: %w", marshalErr)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO asset_event_outbox (
			  event_id,tenant_id,asset_id,aggregate_version,schema_version,
			  partition_key,event_type,payload,status,available_at
			) VALUES ($1,$2,$3,$4,2,$5,'traffic.asset.v2.AssetUpserted',$6,'pending',now())`,
			eventID, tenantID, asset.AssetID, revision, tenantID+":"+asset.AssetID, payload,
		); err != nil {
			return nil, fmt.Errorf("insert inactive outbox: %w", err)
		}
		result.EventIDs = append(result.EventIDs, eventID)
	}
	result.Count = len(result.EventIDs)
	resultPayload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal inactive sweep result: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_inactive_requests (
		  request_id,tenant_id,idempotency_key,request_hash,actor,action_id,reason,
		  cutoff,affected_count,result_payload,trace_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		uuid.NewString(), tenantID, command.IdempotencyKey, requestHash, command.Actor,
		actionID, reason, cutoff, result.Count, resultPayload, command.TraceID,
	); err != nil {
		return nil, fmt.Errorf("insert inactive sweep request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit inactive sweep: %w", err)
	}
	return result, nil
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
