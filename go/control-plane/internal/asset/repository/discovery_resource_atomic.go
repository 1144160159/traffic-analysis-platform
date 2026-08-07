package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

var (
	ErrDiscoveryResourceIdempotencyConflict = errors.New("discovery resource idempotency key conflict")
	ErrDiscoveryResourceRevisionConflict    = errors.New("discovery resource revision conflict")
)

const (
	discoveryCredentialResource = "credential"
	discoveryTopologyResource   = "topology_link"
)

func (r *AssetRepository) UpsertDiscoveryCredentialAtomic(
	ctx context.Context,
	credential *config.DiscoveryCredential,
	command config.DiscoveryResourceCommand,
	requestHash string,
) (*config.DiscoveryCredential, error) {
	if credential == nil {
		return nil, fmt.Errorf("discovery credential required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if replay, err := loadDiscoveryCredentialReplay(ctx, tx, credential.TenantID, command.IdempotencyKey, requestHash); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	now := time.Now().UTC()
	var existing config.DiscoveryCredential
	var endpoint, createdBy sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT credential_id,tenant_id,name,protocol,endpoint,secret_ref,created_by,
		       revision,created_at,updated_at
		FROM asset_discovery_credentials
		WHERE tenant_id=$1 AND credential_id=$2
		FOR UPDATE`, credential.TenantID, credential.CredentialID).Scan(
		&existing.CredentialID, &existing.TenantID, &existing.Name, &existing.Protocol,
		&endpoint, &existing.SecretRef, &createdBy, &existing.Revision,
		&existing.CreatedAt, &existing.UpdatedAt,
	)
	created := errors.Is(err, sql.ErrNoRows)
	if err != nil && !created {
		return nil, err
	}
	if !created {
		existing.Endpoint = endpoint.String
		existing.CreatedBy = createdBy.String
	}
	expected := command.ExpectedRevision
	if created {
		if expected != 0 {
			return nil, ErrDiscoveryResourceRevisionConflict
		}
		credential.Revision = 1
		credential.CreatedAt = now
		credential.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO asset_discovery_credentials(
				credential_id,tenant_id,name,protocol,endpoint,secret_ref,created_by,
				revision,created_at,updated_at
			) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,1,$8,$8)`,
			credential.CredentialID, credential.TenantID, credential.Name, credential.Protocol,
			credential.Endpoint, credential.SecretRef, credential.CreatedBy, now); err != nil {
			return nil, err
		}
	} else {
		if command.ResolveCurrentRevision {
			expected = existing.Revision
		}
		if expected != existing.Revision {
			return nil, ErrDiscoveryResourceRevisionConflict
		}
		credential.Revision = existing.Revision + 1
		credential.CreatedAt = existing.CreatedAt
		credential.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `
			UPDATE asset_discovery_credentials
			SET name=$3,protocol=$4,endpoint=NULLIF($5,''),secret_ref=$6,created_by=$7,
			    revision=$8,updated_at=$9
			WHERE tenant_id=$1 AND credential_id=$2 AND revision=$10`,
			credential.TenantID, credential.CredentialID, credential.Name, credential.Protocol,
			credential.Endpoint, credential.SecretRef, credential.CreatedBy,
			credential.Revision, now, expected); err != nil {
			return nil, err
		}
	}
	credential.ActionID = command.ActionID
	credential.Reason = command.Reason
	oldValue := credentialAuditValue(&existing)
	if created {
		oldValue = map[string]any{}
	}
	newValue := credentialAuditValue(credential)
	if err := insertDiscoveryResourceHistory(ctx, tx, credential.TenantID, discoveryCredentialResource,
		credential.CredentialID, credential.Revision, command, oldValue, newValue); err != nil {
		return nil, err
	}
	eventType := "traffic.asset.discovery.v1.CredentialUpserted"
	eventID, outboxID, err := insertDiscoveryResourceOutbox(ctx, tx, credential.TenantID,
		discoveryCredentialResource, credential.CredentialID, credential.Revision,
		eventType, command, newValue)
	if err != nil {
		return nil, err
	}
	if err := insertDiscoveryResourceAudit(ctx, tx, credential.TenantID, credential.CredentialID,
		"ASSET_DISCOVERY_CREDENTIAL_UPSERT", command, newValue); err != nil {
		return nil, err
	}
	resultJSON, _ := json.Marshal(credential)
	if err := insertDiscoveryResourceRequest(ctx, tx, credential.TenantID, discoveryCredentialResource,
		credential.CredentialID, requestHash, expected, credential.Revision, eventID, outboxID,
		command, resultJSON); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return credential, nil
}

func (r *AssetRepository) UpsertTopologyLinkAtomic(
	ctx context.Context,
	link *config.TopologyLink,
	command config.DiscoveryResourceCommand,
	requestHash string,
) (*config.TopologyLink, error) {
	if link == nil {
		return nil, fmt.Errorf("topology link required")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if replay, err := loadTopologyLinkReplay(ctx, tx, link.TenantID, command.IdempotencyKey, requestHash); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	var existingID string
	var existingRevision int64
	err = tx.QueryRowContext(ctx, `
		SELECT link_id,revision
		FROM asset_topology_links
		WHERE tenant_id=$1 AND source_mac=$2 AND neighbor_mac=$3 AND protocol=$4
		  AND source_interface=$5 AND neighbor_interface=$6
		FOR UPDATE`, link.TenantID, link.SourceMAC, link.NeighborMAC, link.Protocol,
		link.SourceInterface, link.NeighborInterface).Scan(&existingID, &existingRevision)
	created := errors.Is(err, sql.ErrNoRows)
	if err != nil && !created {
		return nil, err
	}
	expected := command.ExpectedRevision
	if created {
		if expected != 0 {
			return nil, ErrDiscoveryResourceRevisionConflict
		}
		link.Revision = 1
	} else {
		if command.ResolveCurrentRevision {
			expected = existingRevision
		}
		if expected != existingRevision {
			return nil, ErrDiscoveryResourceRevisionConflict
		}
		link.LinkID = existingID
		link.Revision = existingRevision + 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO asset_topology_links(
			link_id,tenant_id,run_id,source_asset_id,source_mac,source_ip,source_interface,
			neighbor_asset_id,neighbor_mac,neighbor_ip,neighbor_interface,protocol,confidence,
			revision,observed_at
		) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7,
		          NULLIF($8,''),$9,NULLIF($10,''),$11,$12,$13,$14,$15)
		ON CONFLICT (tenant_id,source_mac,neighbor_mac,protocol,source_interface,neighbor_interface)
		DO UPDATE SET run_id=EXCLUDED.run_id,source_asset_id=EXCLUDED.source_asset_id,
			source_ip=EXCLUDED.source_ip,neighbor_asset_id=EXCLUDED.neighbor_asset_id,
			neighbor_ip=EXCLUDED.neighbor_ip,confidence=EXCLUDED.confidence,
			revision=EXCLUDED.revision,observed_at=EXCLUDED.observed_at`,
		link.LinkID, link.TenantID, link.RunID, link.SourceAssetID, link.SourceMAC,
		link.SourceIP, link.SourceInterface, link.NeighborAssetID, link.NeighborMAC,
		link.NeighborIP, link.NeighborInterface, link.Protocol, link.Confidence,
		link.Revision, link.ObservedAt); err != nil {
		return nil, err
	}
	oldValue := map[string]any{"revision": existingRevision}
	if created {
		oldValue = map[string]any{}
	}
	newValue := topologyAuditValue(link)
	if err := insertDiscoveryResourceHistory(ctx, tx, link.TenantID, discoveryTopologyResource,
		link.LinkID, link.Revision, command, oldValue, newValue); err != nil {
		return nil, err
	}
	eventType := "traffic.asset.discovery.v1.TopologyLinkUpserted"
	eventID, outboxID, err := insertDiscoveryResourceOutbox(ctx, tx, link.TenantID,
		discoveryTopologyResource, link.LinkID, link.Revision, eventType, command, newValue)
	if err != nil {
		return nil, err
	}
	if err := insertDiscoveryResourceAudit(ctx, tx, link.TenantID, link.LinkID,
		"ASSET_DISCOVERY_TOPOLOGY_LINK_UPSERT", command, newValue); err != nil {
		return nil, err
	}
	resultJSON, _ := json.Marshal(link)
	if err := insertDiscoveryResourceRequest(ctx, tx, link.TenantID, discoveryTopologyResource,
		link.LinkID, requestHash, expected, link.Revision, eventID, outboxID, command, resultJSON); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return link, nil
}

func loadDiscoveryCredentialReplay(ctx context.Context, tx *sql.Tx, tenantID, key, requestHash string) (*config.DiscoveryCredential, error) {
	var storedHash string
	var payload []byte
	err := tx.QueryRowContext(ctx, `SELECT request_hash,result_payload FROM asset_discovery_resource_requests WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key).Scan(&storedHash, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if storedHash != requestHash {
		return nil, ErrDiscoveryResourceIdempotencyConflict
	}
	var result config.DiscoveryCredential
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	result.IdempotentReplay = true
	return &result, nil
}

func loadTopologyLinkReplay(ctx context.Context, tx *sql.Tx, tenantID, key, requestHash string) (*config.TopologyLink, error) {
	var storedHash string
	var payload []byte
	err := tx.QueryRowContext(ctx, `SELECT request_hash,result_payload FROM asset_discovery_resource_requests WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key).Scan(&storedHash, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if storedHash != requestHash {
		return nil, ErrDiscoveryResourceIdempotencyConflict
	}
	var result config.TopologyLink
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	result.IdempotentReplay = true
	return &result, nil
}

func insertDiscoveryResourceHistory(ctx context.Context, tx *sql.Tx, tenantID, resourceType, resourceID string, revision int64, command config.DiscoveryResourceCommand, oldValue, newValue map[string]any) error {
	oldJSON, _ := json.Marshal(oldValue)
	newJSON, _ := json.Marshal(newValue)
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_discovery_resource_history(tenant_id,resource_type,resource_id,revision,action_id,actor,reason,trace_id,old_value,new_value) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)`, tenantID, resourceType, resourceID, revision, command.ActionID, command.Actor, command.Reason, command.TraceID, string(oldJSON), string(newJSON))
	return err
}

func insertDiscoveryResourceOutbox(ctx context.Context, tx *sql.Tx, tenantID, resourceType, resourceID string, revision int64, eventType string, command config.DiscoveryResourceCommand, value map[string]any) (uuid.UUID, int64, error) {
	eventID := uuid.New()
	partitionKey := tenantID + ":" + resourceID
	payload := map[string]any{
		"event_id": eventID.String(), "event_type": eventType, "schema_version": 1,
		"aggregate_version": revision, "tenant_id": tenantID, "resource_type": resourceType,
		"resource_id": resourceID, "action_id": command.ActionID, "partition_key": partitionKey,
		"trace_id": command.TraceID, "revision": revision, "resource": value,
	}
	payloadJSON, _ := json.Marshal(payload)
	var outboxID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO asset_discovery_outbox(event_id,run_id,resource_type,resource_id,action_id,tenant_id,aggregate_version,schema_version,partition_key,event_type,payload) VALUES($1,NULL,$2,$3,$4,$5,$6,1,$7,$8,$9::jsonb) RETURNING outbox_id`, eventID, resourceType, resourceID, command.ActionID, tenantID, revision, partitionKey, eventType, string(payloadJSON)).Scan(&outboxID)
	return eventID, outboxID, err
}

func insertDiscoveryResourceAudit(ctx context.Context, tx *sql.Tx, tenantID, resourceID, action string, command config.DiscoveryResourceCommand, value map[string]any) error {
	detail, _ := json.Marshal(map[string]any{"action_id": command.ActionID, "reason": command.Reason, "trace_id": command.TraceID, "request_id": command.RequestID, "resource": value})
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(
			event_id,tenant_id,user_id,action,object_type,object_id,detail,
			ip_addr,user_agent,request_id,trace_id,success,risk_level,result
		) VALUES(
			$1,$2,NULLIF($3,''),$4,'asset_discovery_resource',$5,$6::jsonb,
			NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),true,'medium','accepted'
		)`,
		uuid.NewString(), tenantID, command.Actor, action, resourceID, string(detail),
		command.ClientIP, command.UserAgent, command.RequestID, command.TraceID,
	)
	return err
}

func insertDiscoveryResourceRequest(ctx context.Context, tx *sql.Tx, tenantID, resourceType, resourceID, requestHash string, expected, resulting int64, eventID uuid.UUID, outboxID int64, command config.DiscoveryResourceCommand, resultJSON []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_discovery_resource_requests(request_id,tenant_id,resource_type,resource_id,action_id,idempotency_key,request_hash,expected_revision,resulting_revision,result_payload,event_id,outbox_id,actor,reason,trace_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,$14,$15)`, uuid.New(), tenantID, resourceType, resourceID, command.ActionID, command.IdempotencyKey, requestHash, expected, resulting, string(resultJSON), eventID, outboxID, command.Actor, command.Reason, command.TraceID)
	return err
}

func credentialAuditValue(value *config.DiscoveryCredential) map[string]any {
	return map[string]any{"credential_id": value.CredentialID, "name": value.Name, "protocol": value.Protocol, "endpoint": value.Endpoint, "secret_ref_present": value.SecretRef != "", "revision": value.Revision}
}

func topologyAuditValue(value *config.TopologyLink) map[string]any {
	return map[string]any{"link_id": value.LinkID, "run_id": value.RunID, "source_asset_id": value.SourceAssetID, "source_mac": value.SourceMAC, "source_ip": value.SourceIP, "source_interface": value.SourceInterface, "neighbor_asset_id": value.NeighborAssetID, "neighbor_mac": value.NeighborMAC, "neighbor_ip": value.NeighborIP, "neighbor_interface": value.NeighborInterface, "protocol": value.Protocol, "confidence": value.Confidence, "observed_at": value.ObservedAt.UTC(), "revision": value.Revision}
}
