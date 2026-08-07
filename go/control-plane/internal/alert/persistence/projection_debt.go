package persistence

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
)

const defaultAlertProjectionTargetVersion = "legacy-date-index-v1"

// ProjectionDebtRecorder is the durable commit barrier between the ClickHouse
// source of truth and the rebuildable OpenSearch alert projection.
type ProjectionDebtRecorder interface {
	RecordProjectionDebt(context.Context, []*Alert, string, error) error
}

type ProjectionDebtStore struct {
	db *sql.DB
}

type ProjectionDebt struct {
	TenantID, AlertID, SourceEventID, SourceSHA256, TargetIndexVersion string
	SourceVersion                                                      int64
	AttemptCount                                                       int
}

type ProjectionScope struct {
	TenantID           string
	StartTime          time.Time
	EndTime            time.Time
	BusinessIDs        []string
	TargetIndexVersion string
	MaxDocuments       int
}

type ProjectionReconcileRun struct {
	RunID, TenantID, RequestedBy, TraceID, Mode string
	Scope                                       ProjectionScope
}

type ProjectionReconcileResult struct {
	Status                                             string
	SourceCount, TargetCount, MissingCount, ExtraCount int
	StaleCount, RepairedCount, ErrorCount              int
	MissingIDs, ExtraIDs, StaleIDs                     []string
	VerificationTargetCount                            int
	RemainingMissingCount, RemainingExtraCount         int
	RemainingStaleCount                                int
	RemainingMissingIDs, RemainingExtraIDs             []string
	RemainingStaleIDs                                  []string
	WatermarkMismatchCount                             int
	WatermarkMismatchIDs                               []string
	StopReason                                         string
	Partial                                            bool
	VerificationPerformed, WatermarksConverged         bool
	RepairConverged                                    bool
}

func NewProjectionDebtStore(db *sql.DB) *ProjectionDebtStore {
	return &ProjectionDebtStore{db: db}
}

func (s *ProjectionDebtStore) CheckSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("alert projection debt PostgreSQL connection is required")
	}
	var version string
	if err := s.db.QueryRowContext(ctx, `
		SELECT version FROM alignment_schema_migrations WHERE version='202608041100'
	`).Scan(&version); err != nil {
		return fmt.Errorf("alert projection debt schema 202608041100 is not ready: %w", err)
	}
	return nil
}

// RecordProjectionDebt is idempotent per tenant, alert, and target version.
// A replay may advance the source version but can never replace newer debt with
// an older projection image.
func (s *ProjectionDebtStore) RecordProjectionDebt(ctx context.Context, alerts []*Alert, targetVersion string, cause error) error {
	if s == nil || s.db == nil {
		return errors.New("alert projection debt store is unavailable")
	}
	targetVersion = normalizeProjectionTargetVersion(targetVersion)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert projection debt transaction: %w", err)
	}
	defer tx.Rollback()

	message := projectionErrorMessage(cause)
	for _, alert := range alerts {
		if alert == nil || strings.TrimSpace(alert.TenantID) == "" || strings.TrimSpace(alert.AlertID) == "" {
			return errors.New("projection debt requires tenant_id and alert_id")
		}
		version := AlertSourceVersion(alert)
		hash, hashErr := AlertProjectionSHA256(alert)
		if hashErr != nil {
			return fmt.Errorf("hash alert %s projection: %w", alert.AlertID, hashErr)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO alert_opensearch_projection_debts(
			  tenant_id,alert_id,source_event_id,source_version,source_sha256,target_index_version,
			  status,attempt_count,available_at,locked_until,locked_by,last_error,
			  first_failed_at,last_failed_at,resolved_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,'pending',0,now(),NULL,'',$7,now(),now(),NULL,now())
			ON CONFLICT (tenant_id,alert_id,target_index_version) DO UPDATE SET
			  source_event_id=EXCLUDED.source_event_id,
			  source_version=EXCLUDED.source_version,
			  source_sha256=EXCLUDED.source_sha256,
			  status='pending', attempt_count=0, available_at=now(), locked_until=NULL, locked_by='',
			  last_error=EXCLUDED.last_error, last_failed_at=now(), resolved_at=NULL, updated_at=now()
			WHERE EXCLUDED.source_version >= alert_opensearch_projection_debts.source_version
		`, alert.TenantID, alert.AlertID, alert.EventID, version, hash, targetVersion, message); err != nil {
			return fmt.Errorf("record alert %s projection debt: %w", alert.AlertID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert projection debt: %w", err)
	}
	return nil
}

func (s *ProjectionDebtStore) ClaimProjectionDebts(ctx context.Context, workerID string, limit int, lease time.Duration) ([]ProjectionDebt, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("alert projection debt store is unavailable")
	}
	if strings.TrimSpace(workerID) == "" || limit < 1 || limit > 1000 || lease <= 0 {
		return nil, errors.New("invalid alert projection debt claim budget")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin projection debt claim: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE alert_opensearch_projection_debts
		SET status='pending', available_at=now(), locked_until=NULL, locked_by='', updated_at=now()
		WHERE status='processing' AND locked_until < now()
	`); err != nil {
		return nil, fmt.Errorf("reclaim expired projection debt leases: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT tenant_id,alert_id,source_event_id,source_version,source_sha256,target_index_version,attempt_count
		FROM alert_opensearch_projection_debts
		WHERE status='pending' AND available_at <= now()
		ORDER BY available_at,first_failed_at,tenant_id,alert_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("select projection debts: %w", err)
	}
	debts := make([]ProjectionDebt, 0, limit)
	for rows.Next() {
		var debt ProjectionDebt
		if err := rows.Scan(&debt.TenantID, &debt.AlertID, &debt.SourceEventID, &debt.SourceVersion,
			&debt.SourceSHA256, &debt.TargetIndexVersion, &debt.AttemptCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan projection debt: %w", err)
		}
		debt.AttemptCount++
		debts = append(debts, debt)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close projection debt rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projection debts: %w", err)
	}
	for _, debt := range debts {
		if _, err := tx.ExecContext(ctx, `
			UPDATE alert_opensearch_projection_debts
			SET status='processing',attempt_count=attempt_count+1,locked_by=$4,
			    locked_until=now()+($5 * interval '1 millisecond'),updated_at=now()
			WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version=$3 AND status='pending'
		`, debt.TenantID, debt.AlertID, debt.TargetIndexVersion, workerID, lease.Milliseconds()); err != nil {
			return nil, fmt.Errorf("lease projection debt %s: %w", debt.AlertID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit projection debt claim: %w", err)
	}
	return debts, nil
}

func (s *ProjectionDebtStore) ResolveProjectionDebt(ctx context.Context, workerID string, debt ProjectionDebt, alert *Alert) error {
	if alert == nil {
		return errors.New("resolved projection alert is nil")
	}
	version := AlertSourceVersion(alert)
	hash, err := AlertProjectionSHA256(alert)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin projection resolution: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO alert_opensearch_projection_watermarks(
		  tenant_id,alert_id,source_event_id,source_version,source_sha256,target_index_version,applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (tenant_id,alert_id,target_index_version) DO UPDATE SET
		  source_event_id=EXCLUDED.source_event_id,source_version=EXCLUDED.source_version,
		  source_sha256=EXCLUDED.source_sha256,applied_at=now()
		WHERE EXCLUDED.source_version >= alert_opensearch_projection_watermarks.source_version
	`, alert.TenantID, alert.AlertID, alert.EventID, version, hash, debt.TargetIndexVersion); err != nil {
		return fmt.Errorf("advance projection watermark: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE alert_opensearch_projection_debts
		SET status='resolved',locked_until=NULL,locked_by='',last_error='',resolved_at=now(),updated_at=now()
		WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version=$3
		  AND status='processing' AND locked_by=$4 AND source_version <= $5
	`, debt.TenantID, debt.AlertID, debt.TargetIndexVersion, workerID, version)
	if err != nil {
		return fmt.Errorf("resolve projection debt: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("projection debt lease was lost before resolve")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit projection debt resolution: %w", err)
	}
	return nil
}

func (s *ProjectionDebtStore) RetryProjectionDebt(ctx context.Context, workerID string, debt ProjectionDebt, cause error, maxAttempts int) error {
	status := "pending"
	if debt.AttemptCount >= maxAttempts {
		status = "dead"
	}
	backoffSeconds := 1 << min(debt.AttemptCount, 8)
	result, err := s.db.ExecContext(ctx, `
		UPDATE alert_opensearch_projection_debts
		SET status=$5,available_at=now()+($6 * interval '1 second'),locked_until=NULL,locked_by='',
		    last_error=$7,last_failed_at=now(),updated_at=now()
		WHERE tenant_id=$1 AND alert_id=$2 AND target_index_version=$3
		  AND status='processing' AND locked_by=$4
	`, debt.TenantID, debt.AlertID, debt.TargetIndexVersion, workerID, status, backoffSeconds, projectionErrorMessage(cause))
	if err != nil {
		return fmt.Errorf("retry projection debt: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("projection debt lease was lost before retry")
	}
	return nil
}

func (s *ProjectionDebtStore) StartProjectionReconcileRun(ctx context.Context, run ProjectionReconcileRun, stopErrorCount int) error {
	ids, err := json.Marshal(run.Scope.BusinessIDs)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO alert_opensearch_reconcile_runs(
		  run_id,tenant_id,requested_by,trace_id,mode,target_index_version,start_time,end_time,
		  business_ids,max_documents,stop_error_count,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'running')
	`, run.RunID, run.TenantID, run.RequestedBy, run.TraceID, run.Mode, run.Scope.TargetIndexVersion,
		nullTime(run.Scope.StartTime), nullTime(run.Scope.EndTime), ids, run.Scope.MaxDocuments, stopErrorCount)
	if err != nil {
		return fmt.Errorf("start alert projection reconcile run: %w", err)
	}
	return nil
}

func (s *ProjectionDebtStore) CompleteProjectionReconcileRun(ctx context.Context, runID string, result ProjectionReconcileResult) error {
	manifest, err := projectionReconcileManifest(result)
	if err != nil {
		return err
	}
	updated, err := s.db.ExecContext(ctx, `
		UPDATE alert_opensearch_reconcile_runs SET
		  status=$2,source_count=$3,target_count=$4,missing_count=$5,extra_count=$6,stale_count=$7,
		  repaired_count=$8,error_count=$9,result_manifest=$10,stop_reason=$11,completed_at=now()
		WHERE run_id=$1 AND status='running'
	`, runID, result.Status, result.SourceCount, result.TargetCount, result.MissingCount, result.ExtraCount,
		result.StaleCount, result.RepairedCount, result.ErrorCount, manifest, result.StopReason)
	if err != nil {
		return fmt.Errorf("complete alert projection reconcile run: %w", err)
	}
	if affected, err := updated.RowsAffected(); err != nil || affected != 1 {
		return errors.New("alert projection reconcile run was not running")
	}
	return nil
}

func projectionReconcileManifest(result ProjectionReconcileResult) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"missing_ids": result.MissingIDs, "extra_ids": result.ExtraIDs, "stale_ids": result.StaleIDs,
		"partial": result.Partial,
		"post_repair_verification": map[string]interface{}{
			"performed": result.VerificationPerformed, "target_count": result.VerificationTargetCount,
			"missing_count": result.RemainingMissingCount, "extra_count": result.RemainingExtraCount,
			"stale_count": result.RemainingStaleCount, "missing_ids": result.RemainingMissingIDs,
			"extra_ids": result.RemainingExtraIDs, "stale_ids": result.RemainingStaleIDs,
			"watermark_mismatch_count": result.WatermarkMismatchCount,
			"watermark_mismatch_ids":   result.WatermarkMismatchIDs,
			"watermarks_converged":     result.WatermarksConverged, "repair_converged": result.RepairConverged,
		},
	})
}

// ListProjectionWatermarkMismatches compares a bounded authoritative image
// with PostgreSQL receipts in one query. A later repair run can therefore
// recover a watermark whose first write failed after OpenSearch acknowledged
// and exposed the document.
func (s *ProjectionDebtStore) ListProjectionWatermarkMismatches(ctx context.Context, alerts []*Alert, targetVersion string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("alert projection debt store is unavailable")
	}
	if len(alerts) > 100000 {
		return nil, errors.New("projection watermark comparison exceeds bounded scope")
	}
	type expectedWatermark struct {
		TenantID, AlertID string
		SourceVersion     int64
		SourceSHA256      string
	}
	expected := make([]expectedWatermark, 0, len(alerts))
	for _, alert := range alerts {
		if alert == nil || strings.TrimSpace(alert.TenantID) == "" || strings.TrimSpace(alert.AlertID) == "" {
			return nil, errors.New("projection watermark comparison requires tenant_id and alert_id")
		}
		hash, err := AlertProjectionSHA256(alert)
		if err != nil {
			return nil, err
		}
		expected = append(expected, expectedWatermark{alert.TenantID, alert.AlertID, AlertSourceVersion(alert), hash})
	}
	payload, err := json.Marshal(expected)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH expected AS (
		  SELECT * FROM jsonb_to_recordset($1::jsonb) AS item(
		    "TenantID" text,"AlertID" text,"SourceVersion" bigint,"SourceSHA256" text
		  )
		)
		SELECT expected."AlertID"
		FROM expected
		LEFT JOIN alert_opensearch_projection_watermarks watermark
		  ON watermark.tenant_id=expected."TenantID"
		 AND watermark.alert_id=expected."AlertID"
		 AND watermark.target_index_version=$2
		WHERE watermark.alert_id IS NULL
		   OR watermark.source_version<>expected."SourceVersion"
		   OR watermark.source_sha256<>expected."SourceSHA256"
		ORDER BY expected."AlertID"
	`, payload, normalizeProjectionTargetVersion(targetVersion))
	if err != nil {
		return nil, fmt.Errorf("compare alert projection watermarks: %w", err)
	}
	defer rows.Close()
	mismatches := make([]string, 0)
	for rows.Next() {
		var alertID string
		if err := rows.Scan(&alertID); err != nil {
			return nil, fmt.Errorf("scan alert projection watermark mismatch: %w", err)
		}
		mismatches = append(mismatches, alertID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert projection watermark mismatches: %w", err)
	}
	return mismatches, nil
}

func (s *ProjectionDebtStore) RecordProjectionApplied(ctx context.Context, alert *Alert, targetVersion string) error {
	if alert == nil {
		return errors.New("applied projection alert is nil")
	}
	hash, err := AlertProjectionSHA256(alert)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO alert_opensearch_projection_watermarks(
		  tenant_id,alert_id,source_event_id,source_version,source_sha256,target_index_version,applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (tenant_id,alert_id,target_index_version) DO UPDATE SET
		  source_event_id=EXCLUDED.source_event_id,source_version=EXCLUDED.source_version,
		  source_sha256=EXCLUDED.source_sha256,applied_at=now()
		WHERE EXCLUDED.source_version >= alert_opensearch_projection_watermarks.source_version
	`, alert.TenantID, alert.AlertID, alert.EventID, AlertSourceVersion(alert), hash, normalizeProjectionTargetVersion(targetVersion))
	if err != nil {
		return fmt.Errorf("record applied alert projection: %w", err)
	}
	return nil
}

func nullTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func normalizeProjectionTargetVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultAlertProjectionTargetVersion
	}
	return value
}

func projectionErrorMessage(err error) string {
	if err == nil {
		return "OpenSearch projection was not acknowledged"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}

// AlertSourceVersion is compatible with OpenSearch external_gte versioning.
// Millisecond source timestamps are preferred; old records get a stable floor.
func AlertSourceVersion(alert *Alert) int64 {
	if alert == nil {
		return 1
	}
	for _, candidate := range []time.Time{alert.UpdatedTs, alert.LastSeen, alert.FirstSeen} {
		if version := candidate.UTC().UnixMilli(); version > 0 {
			return version
		}
	}
	return 1
}

// AlertProjectionSHA256 hashes only fields whose authority is ClickHouse.
// Runtime-only enrichments (attack_phase, arkime_link, evidence_count) are
// intentionally excluded so rebuild and reconciliation remain deterministic.
func AlertProjectionSHA256(alert *Alert) (string, error) {
	if alert == nil {
		return "", errors.New("alert is nil")
	}
	canonical := struct {
		TenantID, AlertID, Fingerprint, CommunityID, SessionID, CampaignID string
		SrcIP, DstIP                                                       string
		SrcPort, DstPort                                                   uint16
		Protocol                                                           uint8
		AlertType                                                          string
		Labels                                                             []string
		Score                                                              float32
		Severity                                                           string
		FirstSeen, LastSeen, UpdatedTs                                     time.Time
		Count                                                              int32
		Status, Assignee, ModelVersion, RuleVersion, FeatureSetID          string
		EvidenceIDs                                                        []string
		EventID                                                            string
	}{
		alert.TenantID, alert.AlertID, alert.Fingerprint, alert.CommunityID, alert.SessionID, alert.CampaignID,
		alert.SrcIP, alert.DstIP, alert.SrcPort, alert.DstPort, alert.Protocol, alert.AlertType,
		alert.Labels, alert.Score, alert.Severity, alert.FirstSeen.UTC(), alert.LastSeen.UTC(), alert.UpdatedTs.UTC(),
		alert.Count, alert.Status, alert.Assignee, alert.ModelVersion, alert.RuleVersion, alert.FeatureSetID,
		alert.EvidenceIDs, alert.EventID,
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
