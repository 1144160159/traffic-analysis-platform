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
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var (
	ErrAssetRevisionConflict    = errors.New("asset revision conflict")
	ErrAssetIdempotencyConflict = errors.New("asset idempotency conflict")
)

type assetUpsertIdentity struct {
	TenantID               string              `json:"tenant_id"`
	ActionID               string              `json:"action_id"`
	ExpectedRevision       int64               `json:"expected_revision"`
	ResolveCurrentRevision bool                `json:"resolve_current_revision"`
	Actor                  string              `json:"actor"`
	Reason                 string              `json:"reason"`
	HistoryEventType       string              `json:"history_event_type"`
	ObservedAt             time.Time           `json:"observed_at,omitempty"`
	Asset                  *config.AssetRecord `json:"asset"`
}

// UpsertAtomic commits the authoritative asset, immutable history, minimum
// audit, projection outbox and idempotency result in one PostgreSQL transaction.
func (r *AssetRepository) UpsertAtomic(
	ctx context.Context,
	rec *config.AssetRecord,
	command config.AssetUpsertCommand,
) (*config.AssetUpsertResult, error) {
	actionID := command.ActionID
	if actionID == "" {
		actionID = config.AssetUpsertAction
	}
	reason := command.Reason
	if reason == "" {
		reason = "asset upsert"
	}
	requestAsset := *rec
	requestAsset.Revision = 0
	requestAsset.FirstSeen = time.Time{}
	requestAsset.LastSeen = time.Time{}
	requestJSON, err := json.Marshal(assetUpsertIdentity{
		TenantID:               rec.TenantID,
		ActionID:               actionID,
		ExpectedRevision:       command.ExpectedRevision,
		ResolveCurrentRevision: command.ResolveCurrentRevision,
		Actor:                  command.Actor,
		Reason:                 reason,
		HistoryEventType:       command.HistoryEventType,
		ObservedAt:             command.ObservedAt.UTC(),
		Asset:                  &requestAsset,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal asset upsert identity: %w", err)
	}
	digest := sha256.Sum256(requestJSON)
	requestHash := hex.EncodeToString(digest[:])
	working := *rec
	rec = &working

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin asset upsert: %w", err)
	}
	defer tx.Rollback()
	// Serialize the request identity before checking its ledger row. Without
	// this lock two concurrent deliveries can both miss the row; the loser then
	// observes only a unique-key error instead of the committed replay result.
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"asset-upsert-idem:"+rec.TenantID+":"+command.IdempotencyKey,
	); err != nil {
		return nil, fmt.Errorf("lock asset upsert identity: %w", err)
	}

	var prior config.AssetUpsertResult
	var priorHash, priorActor string
	err = tx.QueryRowContext(ctx, `
		SELECT request_hash,actor,asset_id::text,created,resulting_revision,
		       event_id::text,outbox_id,trace_id
		FROM asset_upsert_requests
		WHERE tenant_id=$1 AND idempotency_key=$2
		FOR UPDATE`,
		rec.TenantID, command.IdempotencyKey,
	).Scan(
		&priorHash, &priorActor, &prior.AssetID, &prior.Created, &prior.Revision,
		&prior.EventID, &prior.OutboxID, &prior.TraceID,
	)
	if err == nil {
		if priorHash != requestHash || priorActor != command.Actor {
			return nil, ErrAssetIdempotencyConflict
		}
		prior.IdempotentReplay = true
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit asset idempotency replay: %w", err)
		}
		return &prior, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("read asset idempotency result: %w", err)
	}

	// Serialize create/update decisions for one tenant-scoped canonical MAC.
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:%s", len(rec.TenantID), rec.TenantID, rec.MACAddress),
	); err != nil {
		return nil, fmt.Errorf("lock asset identity: %w", err)
	}

	existing, err := findAssetByMACTx(ctx, tx, rec.TenantID, rec.MACAddress)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("read authoritative asset: %w", err)
	}

	now := time.Now().UTC()
	observedAt := command.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = now
	}
	result := &config.AssetUpsertResult{
		AssetID: rec.AssetID,
		Created: existing == nil,
		EventID: uuid.NewString(),
		TraceID: command.TraceID,
	}
	effectiveExpectedRevision := command.ExpectedRevision
	var oldJSON []byte
	if existing == nil {
		if !command.ResolveCurrentRevision && command.ExpectedRevision != 0 {
			return nil, ErrAssetRevisionConflict
		}
		effectiveExpectedRevision = 0
		if rec.AssetID == "" {
			rec.AssetID = uuid.NewSHA1(
				uuid.NameSpaceURL,
				[]byte("traffic.asset:"+rec.TenantID+":"+rec.MACAddress),
			).String()
			result.AssetID = rec.AssetID
		}
		ensureAssetDefaults(rec)
		rec.FirstSeen = observedAt
		rec.LastSeen = observedAt
		rec.Revision = 1
		err = tx.QueryRowContext(ctx, `
			INSERT INTO assets (
			  asset_id,revision,display_code,tenant_id,asset_type,status,ip_address,
			  mac_address,hostname,vendor,os_type,source,vlan_id,switch_port,department,
			  campus,owner,criticality,tags,metadata,first_seen,last_seen
			) VALUES (
			  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
			)
			RETURNING asset_id::text,revision`,
			rec.AssetID, rec.Revision, rec.DisplayCode, rec.TenantID, rec.AssetType, rec.Status,
			rec.IPAddress, rec.MACAddress, rec.Hostname, rec.Vendor, rec.OSType, rec.Source,
			rec.VlanID, rec.SwitchPort, rec.Department, rec.Campus, rec.Owner, rec.Criticality,
			jsonObject(rec.Tags), jsonObject(rec.Metadata), rec.FirstSeen, rec.LastSeen,
		).Scan(&result.AssetID, &result.Revision)
	} else {
		if command.ResolveCurrentRevision {
			effectiveExpectedRevision = existing.Revision
		} else if command.ExpectedRevision != existing.Revision {
			return nil, ErrAssetRevisionConflict
		}
		if rec.AssetID != "" && rec.AssetID != existing.AssetID {
			return nil, ErrAssetIdempotencyConflict
		}
		rec.AssetID = existing.AssetID
		rec.FirstSeen = existing.FirstSeen
		mergeAssetGovernance(rec, existing)
		rec.LastSeen = observedAt
		if rec.LastSeen.Before(existing.LastSeen) {
			rec.LastSeen = existing.LastSeen
		}
		oldJSON, _ = json.Marshal(existing)
		err = tx.QueryRowContext(ctx, `
			UPDATE assets
			SET display_code=$1,asset_type=$2,status=$3,ip_address=$4,hostname=$5,
			    vendor=$6,os_type=$7,source=$8,vlan_id=$9,switch_port=$10,
			    department=$11,campus=$12,owner=$13,criticality=$14,tags=$15,
			    metadata=$16,last_seen=$17,revision=revision+1,updated_at=now()
			WHERE tenant_id=$18 AND asset_id=$19 AND revision=$20
			RETURNING asset_id::text,revision`,
			rec.DisplayCode, rec.AssetType, rec.Status, rec.IPAddress, rec.Hostname,
			rec.Vendor, rec.OSType, rec.Source, rec.VlanID, rec.SwitchPort,
			rec.Department, rec.Campus, rec.Owner, rec.Criticality,
			jsonObject(rec.Tags), jsonObject(rec.Metadata), rec.LastSeen,
			rec.TenantID, existing.AssetID, effectiveExpectedRevision,
		).Scan(&result.AssetID, &result.Revision)
	}
	if err == sql.ErrNoRows {
		return nil, ErrAssetRevisionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("write authoritative asset: %w", err)
	}
	rec.AssetID = result.AssetID
	rec.Revision = result.Revision
	newJSON, err := json.Marshal(rec)
	if err != nil {
		return nil, fmt.Errorf("marshal asset history: %w", err)
	}

	eventType := "updated"
	if result.Created {
		eventType = "first_seen"
	}
	if command.HistoryEventType != "" {
		eventType = command.HistoryEventType
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_events (
		  event_uuid,asset_id,tenant_id,event_type,revision,trace_id,old_value,new_value
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		result.EventID, result.AssetID, rec.TenantID, eventType, result.Revision,
		command.TraceID, jsonBytesOrNil(oldJSON), newJSON,
	); err != nil {
		return nil, fmt.Errorf("insert asset history: %w", err)
	}

	auditDetail, _ := json.Marshal(map[string]any{
		"action_id":       actionID,
		"created":         result.Created,
		"revision":        result.Revision,
		"reason":          reason,
		"observed_at":     observedAt,
		"idempotency_key": command.IdempotencyKey,
		"event_id":        result.EventID,
		"trace_id":        command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
		  event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,
		  user_agent,request_id,trace_id,success,risk_level,result
		) VALUES ($1,$2,$3,$4,'asset',$5,$6,$7,$8,$9,$10,true,'medium','accepted')`,
		"asset-audit-"+result.EventID, rec.TenantID, command.Actor,
		strings.ToUpper(strings.ReplaceAll(actionID, "-", "_")), result.AssetID, auditDetail,
		command.ClientIP, command.UserAgent, command.RequestID, command.TraceID,
	); err != nil {
		return nil, fmt.Errorf("insert asset audit: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"action_id":         actionID,
		"event_id":          result.EventID,
		"event_type":        "traffic.asset.v2.AssetUpserted",
		"schema_version":    2,
		"aggregate_version": result.Revision,
		"partition_key":     rec.TenantID + ":" + result.AssetID,
		"tenant_id":         rec.TenantID,
		"asset_id":          result.AssetID,
		"revision":          result.Revision,
		"trace_id":          command.TraceID,
		"reason":            reason,
		"observed_at":       observedAt,
		"asset":             rec,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal asset outbox: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO asset_event_outbox (
		  event_id,tenant_id,asset_id,aggregate_version,schema_version,
		  partition_key,event_type,payload,status,available_at
		) VALUES ($1,$2,$3,$4,2,$5,'traffic.asset.v2.AssetUpserted',$6,'pending',now())
		RETURNING outbox_id`,
		result.EventID, rec.TenantID, result.AssetID, result.Revision,
		rec.TenantID+":"+result.AssetID, payload,
	).Scan(&result.OutboxID); err != nil {
		return nil, fmt.Errorf("insert asset outbox: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_upsert_requests (
		  request_id,tenant_id,idempotency_key,request_hash,actor,expected_revision,
		  asset_id,created,resulting_revision,event_id,outbox_id,trace_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		uuid.NewString(), rec.TenantID, command.IdempotencyKey, requestHash, command.Actor,
		effectiveExpectedRevision, result.AssetID, result.Created, result.Revision,
		result.EventID, result.OutboxID, command.TraceID,
	); err != nil {
		return nil, fmt.Errorf("insert asset idempotency result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit asset upsert: %w", err)
	}
	return result, nil
}

func findAssetByMACTx(ctx context.Context, tx *sql.Tx, tenantID, mac string) (*config.AssetRecord, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT asset_id,revision,display_code,tenant_id,asset_type,status,ip_address,mac_address,
		       hostname,vendor,os_type,source,vlan_id,switch_port,department,campus,owner,
		       criticality,tags,metadata,first_seen,last_seen
		FROM assets
		WHERE tenant_id=$1 AND mac_address=$2
		FOR UPDATE`,
		tenantID, mac,
	)
	return scanAsset(row)
}

func jsonBytesOrNil(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
