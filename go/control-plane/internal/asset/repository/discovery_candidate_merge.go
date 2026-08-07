package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func (r *AssetRepository) MergeDiscoveryCandidateAtomic(
	ctx context.Context,
	tenantID, runID, candidateID string,
	command config.DiscoveryCandidateMergeCommand,
) (*config.DiscoveryCandidateMergeResult, error) {
	requestJSON, _ := json.Marshal(map[string]any{
		"operation":                   "merge_candidate",
		"run_id":                      runID,
		"candidate_id":                candidateID,
		"expected_candidate_revision": command.ExpectedCandidateRevision,
		"expected_asset_revision":     command.ExpectedAssetRevision,
		"merge_mode":                  command.MergeMode,
		"reason":                      command.Reason,
		"actor":                       command.Actor,
	})
	requestHash := discoveryObservationFingerprint(requestJSON)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Serialize the read-before-write idempotency decision so simultaneous
	// retries return the stored result instead of observing a transient state
	// conflict after the first transaction commits.
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:discovery-control:%s", len(tenantID), tenantID, command.IdempotencyKey),
	); err != nil {
		return nil, err
	}
	var priorOperation, priorCandidateID, priorHash string
	var priorPayload []byte
	err = tx.QueryRowContext(ctx, `
		SELECT operation,COALESCE(candidate_id::text,''),request_hash,result_payload
		  FROM asset_discovery_control_requests
		 WHERE tenant_id=$1 AND idempotency_key=$2
		 FOR UPDATE`,
		tenantID, command.IdempotencyKey,
	).Scan(&priorOperation, &priorCandidateID, &priorHash, &priorPayload)
	if err == nil {
		if priorOperation != "merge_candidate" || priorCandidateID != candidateID || priorHash != requestHash {
			return nil, ErrDiscoveryIdempotencyConflict
		}
		var result config.DiscoveryCandidateMergeResult
		if err := json.Unmarshal(priorPayload, &result); err != nil {
			return nil, fmt.Errorf("decode candidate merge replay: %w", err)
		}
		candidate, err := getDiscoveryCandidateTx(ctx, tx, tenantID, runID, candidateID, false)
		if err != nil {
			return nil, err
		}
		result.Candidate = candidate
		result.IdempotentReplay = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var runStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT status
		  FROM asset_discovery_runs
		 WHERE tenant_id=$1 AND run_id=$2
		 FOR UPDATE`,
		tenantID, runID,
	).Scan(&runStatus); err != nil {
		return nil, err
	}
	if runStatus != config.DiscoveryStatusSucceeded &&
		runStatus != config.DiscoveryStatusPartial {
		return nil, ErrDiscoveryStateConflict
	}
	candidate, err := getDiscoveryCandidateTx(ctx, tx, tenantID, runID, candidateID, true)
	if err != nil {
		return nil, err
	}
	if candidate.Revision != command.ExpectedCandidateRevision {
		return nil, ErrDiscoveryRevisionConflict
	}
	if candidate.Status != "pending" && candidate.Status != "approved" {
		return nil, ErrDiscoveryStateConflict
	}
	canonicalMAC, err := canonicalCandidateMAC(candidate.Observation.MACAddress)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		fmt.Sprintf("%d:%s:%s", len(tenantID), tenantID, canonicalMAC),
	); err != nil {
		return nil, err
	}
	existing, err := findAssetByMACTx(ctx, tx, tenantID, canonicalMAC)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	now := time.Now().UTC()
	rec := &config.AssetRecord{
		TenantID:   tenantID,
		AssetType:  "network-device",
		Status:     "active",
		IPAddress:  strings.TrimSpace(candidate.Observation.IPAddress),
		MACAddress: canonicalMAC,
		Hostname:   strings.TrimSpace(candidate.Observation.Hostname),
		Vendor:     strings.TrimSpace(candidate.Observation.Vendor),
		OSType:     strings.TrimSpace(candidate.Observation.OSType),
		Source:     "active-confirmed:" + command.MergeMode,
		VlanID:     strings.TrimSpace(candidate.Observation.VlanID),
		SwitchPort: strings.TrimSpace(candidate.Observation.SwitchPort),
	}
	result := &config.DiscoveryCandidateMergeResult{
		AssetCreated: existing == nil,
		EventID:      uuid.NewString(),
		TraceID:      command.TraceID,
	}
	var oldJSON []byte
	if existing == nil {
		if command.ExpectedAssetRevision != 0 {
			return nil, ErrAssetRevisionConflict
		}
		rec.AssetID = uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("traffic.asset:"+tenantID+":"+canonicalMAC),
		).String()
		rec.Revision = 1
		rec.FirstSeen = now
		rec.LastSeen = now
		ensureAssetDefaults(rec)
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO assets(
				asset_id,revision,display_code,tenant_id,asset_type,status,
				ip_address,mac_address,hostname,vendor,os_type,source,vlan_id,
				switch_port,department,campus,owner,criticality,tags,metadata,
				first_seen,last_seen
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,
				$15,$16,$17,$18,$19,$20,$21,$22
			)
			RETURNING asset_id::text,revision`,
			rec.AssetID, rec.Revision, rec.DisplayCode, rec.TenantID,
			rec.AssetType, rec.Status, rec.IPAddress, rec.MACAddress,
			rec.Hostname, rec.Vendor, rec.OSType, rec.Source, rec.VlanID,
			rec.SwitchPort, rec.Department, rec.Campus, rec.Owner,
			rec.Criticality, jsonObject(rec.Tags), jsonObject(rec.Metadata),
			rec.FirstSeen, rec.LastSeen,
		).Scan(&result.AssetID, &result.AssetRevision); err != nil {
			return nil, fmt.Errorf("insert merged discovery asset: %w", err)
		}
	} else {
		if command.ExpectedAssetRevision != existing.Revision {
			return nil, ErrAssetRevisionConflict
		}
		rec.AssetID = existing.AssetID
		rec.FirstSeen = existing.FirstSeen
		mergeAssetGovernance(rec, existing)
		rec.LastSeen = now
		oldJSON, _ = json.Marshal(existing)
		if err := tx.QueryRowContext(ctx, `
			UPDATE assets
			   SET display_code=$1,asset_type=$2,status=$3,ip_address=$4,
			       hostname=$5,vendor=$6,os_type=$7,source=$8,vlan_id=$9,
			       switch_port=$10,department=$11,campus=$12,owner=$13,
			       criticality=$14,tags=$15,metadata=$16,last_seen=$17,
			       revision=revision+1,updated_at=now()
			 WHERE tenant_id=$18 AND asset_id=$19 AND revision=$20
			RETURNING asset_id::text,revision`,
			rec.DisplayCode, rec.AssetType, rec.Status, rec.IPAddress,
			rec.Hostname, rec.Vendor, rec.OSType, rec.Source, rec.VlanID,
			rec.SwitchPort, rec.Department, rec.Campus, rec.Owner,
			rec.Criticality, jsonObject(rec.Tags), jsonObject(rec.Metadata),
			rec.LastSeen, tenantID, existing.AssetID, command.ExpectedAssetRevision,
		).Scan(&result.AssetID, &result.AssetRevision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrAssetRevisionConflict
			}
			return nil, fmt.Errorf("update merged discovery asset: %w", err)
		}
	}
	rec.AssetID = result.AssetID
	rec.Revision = result.AssetRevision
	newJSON, _ := json.Marshal(rec)
	eventType := "updated"
	if result.AssetCreated {
		eventType = "first_seen"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_events(
			event_uuid,asset_id,tenant_id,event_type,revision,trace_id,old_value,new_value
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		result.EventID, result.AssetID, tenantID, eventType,
		result.AssetRevision, command.TraceID, jsonBytesOrNil(oldJSON), newJSON,
	); err != nil {
		return nil, fmt.Errorf("insert merged discovery history: %w", err)
	}
	auditDetail, _ := json.Marshal(map[string]any{
		"run_id":             runID,
		"candidate_id":       candidateID,
		"candidate_revision": command.ExpectedCandidateRevision + 1,
		"asset_revision":     result.AssetRevision,
		"asset_created":      result.AssetCreated,
		"merge_mode":         command.MergeMode,
		"reason":             command.Reason,
		"idempotency_key":    command.IdempotencyKey,
		"trace_id":           command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent,request_id,trace_id,success,risk_level,result
		) VALUES (
			$1,$2,$3,'ASSET_DISCOVERY_CANDIDATE_MERGED',
			'asset_discovery_candidate',$4,$5,$6,$7,$8,$9,true,'medium','accepted'
		)`,
		"asset-discovery-merge-"+result.EventID, tenantID, command.Actor,
		candidateID, auditDetail, command.ClientIP, command.UserAgent,
		command.RequestID, command.TraceID,
	); err != nil {
		return nil, fmt.Errorf("insert candidate merge audit: %w", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"event_id":          result.EventID,
		"event_type":        "traffic.asset.v2.AssetUpserted",
		"schema_version":    2,
		"aggregate_version": result.AssetRevision,
		"partition_key":     tenantID + ":" + result.AssetID,
		"tenant_id":         tenantID,
		"asset_id":          result.AssetID,
		"revision":          result.AssetRevision,
		"trace_id":          command.TraceID,
		"asset":             rec,
		"source": map[string]any{
			"run_id": runID, "candidate_id": candidateID,
		},
	})
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO asset_event_outbox(
			event_id,tenant_id,asset_id,aggregate_version,schema_version,
			partition_key,event_type,payload,status,available_at
		) VALUES (
			$1,$2,$3,$4,2,$5,'traffic.asset.v2.AssetUpserted',$6,'pending',now()
		)
		RETURNING outbox_id`,
		result.EventID, tenantID, result.AssetID, result.AssetRevision,
		tenantID+":"+result.AssetID, payload,
	).Scan(&result.OutboxID); err != nil {
		return nil, fmt.Errorf("insert candidate merge outbox: %w", err)
	}
	candidate.Status = "merged"
	candidate.Revision++
	candidate.SourceAssetID = result.AssetID
	candidate.DecisionReason = command.Reason
	candidate.DecidedBy = command.Actor
	candidate.DecidedAt = now
	update, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_candidates
		   SET status='merged',revision=$5,source_asset_id=$6,
		       decision_reason=$7,decided_by=$8,decided_at=$9
		 WHERE tenant_id=$1 AND run_id=$2 AND candidate_id=$3
		   AND revision=$4 AND status IN ('pending','approved')`,
		tenantID, runID, candidateID, command.ExpectedCandidateRevision,
		candidate.Revision, result.AssetID, command.Reason, command.Actor, now,
	)
	if err != nil {
		return nil, err
	}
	if affected, _ := update.RowsAffected(); affected != 1 {
		return nil, ErrDiscoveryRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE asset_discovery_runs
		   SET discovered_assets=(
		         SELECT count(*) FROM asset_discovery_candidates
		          WHERE tenant_id=$1 AND run_id=$2 AND status='merged'
		       ),
		       updated_at=now()
		 WHERE tenant_id=$1 AND run_id=$2`,
		tenantID, runID,
	); err != nil {
		return nil, err
	}
	result.Candidate = candidate
	storedResult := *result
	storedResult.Candidate = nil
	resultJSON, _ := json.Marshal(storedResult)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_discovery_control_requests(
			request_id,tenant_id,run_id,operation,candidate_id,idempotency_key,
			request_hash,expected_revision,resulting_revision,result_payload,
			actor,reason,trace_id
		) VALUES (
			$1,$2,$3,'merge_candidate',$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12
		)`,
		uuid.New(), tenantID, runID, candidateID, command.IdempotencyKey,
		requestHash, command.ExpectedCandidateRevision, candidate.Revision,
		string(resultJSON), command.Actor, command.Reason, command.TraceID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func getDiscoveryCandidateTx(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, runID, candidateID string,
	forUpdate bool,
) (*config.DiscoveryCandidate, error) {
	query := `
		SELECT candidate_id::text,run_id,tenant_id,fingerprint,observation,
		       status,revision,COALESCE(source_asset_id::text,''),
		       decision_reason,decided_by,discovered_at,decided_at
		  FROM asset_discovery_candidates
		 WHERE tenant_id=$1 AND run_id=$2 AND candidate_id=$3`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var candidate config.DiscoveryCandidate
	var observation []byte
	var decidedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, query, tenantID, runID, candidateID).Scan(
		&candidate.CandidateID, &candidate.RunID, &candidate.TenantID,
		&candidate.Fingerprint, &observation, &candidate.Status,
		&candidate.Revision, &candidate.SourceAssetID,
		&candidate.DecisionReason, &candidate.DecidedBy,
		&candidate.DiscoveredAt, &decidedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(observation, &candidate.Observation); err != nil {
		return nil, err
	}
	if decidedAt.Valid {
		candidate.DecidedAt = decidedAt.Time
	}
	return &candidate, nil
}

func canonicalCandidateMAC(value string) (string, error) {
	hardware, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("candidate mac_address is invalid: %w", err)
	}
	return strings.ToLower(hardware.String()), nil
}
