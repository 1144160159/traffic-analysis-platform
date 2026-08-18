package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func validateModelShadowAck(ack ModelAppliedAck, expectedParallelism int) error {
	if ack.AckType != "shadow_load" {
		return fmt.Errorf("model shadow acknowledgement requires ack_type shadow_load")
	}
	switch ack.Status {
	case "shadow_ready", "stale", "duplicate", "failed":
	default:
		return fmt.Errorf("invalid model shadow acknowledgement status %q", ack.Status)
	}
	if strings.TrimSpace(ack.EventID) == "" || strings.TrimSpace(ack.TenantID) == "" ||
		strings.TrimSpace(ack.ModelID) == "" || strings.TrimSpace(ack.Version) == "" ||
		strings.TrimSpace(ack.PackageID) == "" {
		return fmt.Errorf("model shadow acknowledgement is missing event, scope, version or package identity")
	}
	if !consumerSHA256Pattern.MatchString(ack.PackageSHA256) || ack.ArtifactSHA256 != ack.PackageSHA256 {
		return fmt.Errorf("model shadow acknowledgement package SHA-256 is invalid or inconsistent")
	}
	if ack.AggregateRevision <= 0 {
		return fmt.Errorf("model shadow acknowledgement aggregate revision must be positive")
	}
	if expectedParallelism <= 0 || ack.Parallelism != expectedParallelism ||
		ack.SubtaskIndex < 0 || ack.SubtaskIndex >= expectedParallelism {
		return fmt.Errorf("model shadow acknowledgement subtask %d/%d does not match server contract %d",
			ack.SubtaskIndex, ack.Parallelism, expectedParallelism)
	}
	if ack.Status == "failed" && strings.TrimSpace(ack.Error) == "" {
		return fmt.Errorf("failed model shadow acknowledgement requires an error")
	}
	return nil
}

func validateModelShadowEventContract(ack ModelAppliedAck, event ModelUpdateEvent) error {
	if event.SchemaVersion != 2 || event.Action != "shadow-load" {
		return fmt.Errorf("model update outbox event is not a schema v2 shadow-load contract")
	}
	if ack.TenantID != event.TenantID || ack.ModelID != event.ModelID || ack.Version != event.Version {
		return fmt.Errorf("model shadow acknowledgement scope or version does not match event contract")
	}
	if ack.PackageID != event.PackageID || ack.PackageSHA256 != event.PackageSHA256 ||
		ack.AggregateRevision != event.AggregateRevision {
		return fmt.Errorf("model shadow acknowledgement package identity or revision does not match event contract")
	}
	if strings.TrimSpace(event.ArtifactManifestURI) == "" || ack.ArtifactURI != event.ArtifactManifestURI {
		return fmt.Errorf("model shadow acknowledgement artifact manifest URI does not match event contract")
	}
	for name, value := range map[string]string{
		"artifact_manifest_sha256": event.ArtifactManifestSHA256,
		"package_sha256":           event.PackageSHA256,
		"evaluation_sha256":        event.EvaluationSHA256,
		"explanation_sha256":       event.ExplanationSHA256,
		"graph_snapshot_sha256":    event.GraphSnapshotSHA256,
	} {
		if !consumerSHA256Pattern.MatchString(value) {
			return fmt.Errorf("model shadow event %s is not a lowercase SHA-256", name)
		}
	}
	if strings.TrimSpace(event.GraphSnapshotID) == "" || event.AggregateRevision <= 0 {
		return fmt.Errorf("model shadow event graph snapshot or aggregate revision is missing")
	}
	return nil
}

// handleModelShadowAck stores an inbox row for every Flink subtask.  It emits
// no activation event and creates a time-bounded ready receipt only for the
// exact server-controlled subtask quorum.
func (s *ModelService) handleModelShadowAck(ctx context.Context, ack ModelAppliedAck, payload []byte) error {
	expected := s.config.AppliedAckExpectedParallelism
	if err := validateModelShadowAck(ack, expected); err != nil {
		return err
	}
	if s.config.ModelShadowReadyTTL <= 0 {
		return fmt.Errorf("model shadow readiness TTL must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model shadow acknowledgement: %w", err)
	}
	defer tx.Rollback()

	var outboxTenant, outboxModel, outboxVersion string
	var eventPayload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id, model_id, model_version, payload
		FROM model_update_outbox WHERE event_id=$1 FOR UPDATE
	`, ack.EventID).Scan(&outboxTenant, &outboxModel, &outboxVersion, &eventPayload); err != nil {
		return fmt.Errorf("resolve model shadow outbox event %s: %w", ack.EventID, err)
	}
	if ack.TenantID != outboxTenant || ack.ModelID != outboxModel || ack.Version != outboxVersion {
		return fmt.Errorf("model shadow acknowledgement outbox scope mismatch for event %s", ack.EventID)
	}
	var event ModelUpdateEvent
	if err := json.Unmarshal(eventPayload, &event); err != nil {
		return fmt.Errorf("decode model shadow event contract: %w", err)
	}
	if err := validateModelShadowEventContract(ack, event); err != nil {
		return err
	}

	ackResult, err := tx.ExecContext(ctx, `
		INSERT INTO model_update_shadow_acks (
			event_id,tenant_id,model_id,model_version,package_id,package_sha256,
			aggregate_revision,subtask_index,parallelism,status,error,payload,received_at,last_seen_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,now(),now())
		ON CONFLICT (event_id,subtask_index) DO UPDATE SET
			status=CASE
				WHEN model_update_shadow_acks.status IN ('failed','stale') THEN model_update_shadow_acks.status
				WHEN model_update_shadow_acks.status='shadow_ready' THEN model_update_shadow_acks.status
				ELSE EXCLUDED.status END,
			error=CASE
				WHEN model_update_shadow_acks.status IN ('failed','stale') THEN model_update_shadow_acks.error
				WHEN model_update_shadow_acks.status='shadow_ready' THEN model_update_shadow_acks.error
				ELSE EXCLUDED.error END,
			payload=EXCLUDED.payload,last_seen_at=now()
		WHERE model_update_shadow_acks.tenant_id=EXCLUDED.tenant_id
		  AND model_update_shadow_acks.model_id=EXCLUDED.model_id
		  AND model_update_shadow_acks.model_version=EXCLUDED.model_version
		  AND model_update_shadow_acks.package_id=EXCLUDED.package_id
		  AND model_update_shadow_acks.package_sha256=EXCLUDED.package_sha256
		  AND model_update_shadow_acks.aggregate_revision=EXCLUDED.aggregate_revision
		  AND model_update_shadow_acks.parallelism=EXCLUDED.parallelism
	`, ack.EventID, ack.TenantID, ack.ModelID, ack.Version, ack.PackageID,
		ack.PackageSHA256, ack.AggregateRevision, ack.SubtaskIndex, ack.Parallelism,
		ack.Status, ack.Error, string(payload))
	if err != nil {
		return fmt.Errorf("persist model shadow acknowledgement: %w", err)
	}
	if rows, err := ackResult.RowsAffected(); err != nil || rows != 1 {
		if err == nil {
			err = sql.ErrNoRows
		}
		return fmt.Errorf("model shadow acknowledgement identity conflict: %w", err)
	}

	var readyCount, minSubtask, maxSubtask int
	var hasFailure bool
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='shadow_ready'),
		       COALESCE(MIN(subtask_index) FILTER (WHERE status='shadow_ready'),-1),
		       COALESCE(MAX(subtask_index) FILTER (WHERE status='shadow_ready'),-1),
		       COALESCE(BOOL_OR(status IN ('failed','stale')),false)
		FROM model_update_shadow_acks WHERE event_id=$1
	`, ack.EventID).Scan(&readyCount, &minSubtask, &maxSubtask, &hasFailure); err != nil {
		return fmt.Errorf("aggregate model shadow acknowledgements: %w", err)
	}
	if hasFailure {
		if _, err := tx.ExecContext(ctx, `DELETE FROM model_update_shadow_ready_receipts WHERE event_id=$1`, ack.EventID); err != nil {
			return fmt.Errorf("revoke failed model shadow ready receipt: %w", err)
		}
	} else if readyCount == expected && minSubtask == 0 && maxSubtask == expected-1 {
		expiresAt := time.Now().UTC().Add(s.config.ModelShadowReadyTTL)
		result, err := tx.ExecContext(ctx, `
			INSERT INTO model_update_shadow_ready_receipts (
				event_id,tenant_id,model_id,model_version,package_id,package_sha256,
				aggregate_revision,expected_parallelism,ready_subtasks,status,ready_at,last_seen_at,expires_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,'shadow_ready',now(),now(),$9)
			ON CONFLICT (event_id) DO UPDATE SET last_seen_at=now(),expires_at=EXCLUDED.expires_at
			WHERE model_update_shadow_ready_receipts.tenant_id=EXCLUDED.tenant_id
			  AND model_update_shadow_ready_receipts.model_id=EXCLUDED.model_id
			  AND model_update_shadow_ready_receipts.model_version=EXCLUDED.model_version
			  AND model_update_shadow_ready_receipts.package_id=EXCLUDED.package_id
			  AND model_update_shadow_ready_receipts.package_sha256=EXCLUDED.package_sha256
			  AND model_update_shadow_ready_receipts.aggregate_revision=EXCLUDED.aggregate_revision
			  AND model_update_shadow_ready_receipts.expected_parallelism=EXCLUDED.expected_parallelism
		`, ack.EventID, ack.TenantID, ack.ModelID, ack.Version, ack.PackageID,
			ack.PackageSHA256, ack.AggregateRevision, expected, expiresAt)
		if err != nil {
			return fmt.Errorf("persist model shadow ready receipt: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			if err == nil {
				err = sql.ErrNoRows
			}
			return fmt.Errorf("model shadow ready receipt identity conflict: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model shadow acknowledgement: %w", err)
	}
	return nil
}
