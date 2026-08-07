package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrRepairReconcileUnavailable = errors.New("authoritative repair reconciliation is unavailable")

// ClickHouseRepairEvidenceProvider derives repair evidence from persisted
// PostgreSQL scope and bounded ClickHouse facts. It never accepts tenant,
// window, budget or SQL expressions from the HTTP summary payload.
type ClickHouseRepairEvidenceProvider struct {
	controlDB *sql.DB
	factsDB   *sql.DB
	timeout   time.Duration
}

func NewClickHouseRepairEvidenceProvider(controlDB, factsDB *sql.DB, timeout time.Duration) *ClickHouseRepairEvidenceProvider {
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 15 * time.Second
	}
	return &ClickHouseRepairEvidenceProvider{controlDB: controlDB, factsDB: factsDB, timeout: timeout}
}

func (p *ClickHouseRepairEvidenceProvider) DryRun(ctx context.Context, tenantID, repairID string) (map[string]interface{}, error) {
	if p == nil || p.controlDB == nil || p.factsDB == nil {
		return nil, fmt.Errorf("repair evidence databases are unavailable")
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(repairID) == "" {
		return nil, fmt.Errorf("repair evidence tenant and repair identity are required")
	}
	var operationID, status string
	var scopeJSON, budgetJSON []byte
	err := p.controlDB.QueryRowContext(ctx, `
		SELECT operation_id,status,input_scope,resource_budget
		FROM data_quality_repairs
		WHERE tenant_id=$1 AND repair_id=$2
	`, tenantID, repairID).Scan(&operationID, &status, &scopeJSON, &budgetJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRepairNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load repair scope for dry-run: %w", err)
	}
	if operationID != "flow_replay_window_v1" || status != "planned" {
		return nil, ErrRepairConflict
	}
	var scope, budget map[string]interface{}
	if err := json.Unmarshal(scopeJSON, &scope); err != nil {
		return nil, fmt.Errorf("decode persisted repair scope: %w", err)
	}
	if err := json.Unmarshal(budgetJSON, &budget); err != nil {
		return nil, fmt.Errorf("decode persisted repair budget: %w", err)
	}
	if err := validateRepairScope(tenantID, scope, budget); err != nil {
		return nil, fmt.Errorf("persisted repair scope failed validation: %w", err)
	}
	windowStart, _ := time.Parse(time.RFC3339, stringValue(scope["window_start"]))
	windowEnd, _ := time.Parse(time.RFC3339, stringValue(scope["window_end"]))
	effectiveTimeout := p.timeout
	if budgetTimeout := time.Duration(int64Value(budget["max_duration_seconds"])) * time.Second; budgetTimeout > 0 && budgetTimeout < effectiveTimeout {
		effectiveTimeout = budgetTimeout
	}
	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()
	const query = `SELECT count(), uniqExact(event_id), groupBitXor(cityHash64(event_id)),
		if(count()=0, 0, min(ingest_ts)), if(count()=0, 0, max(ingest_ts))
		FROM traffic.flows_raw
		WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?`
	var totalRows, distinctEvents, eventIDHash uint64
	var minIngest, maxIngest int64
	if err := p.factsDB.QueryRowContext(queryCtx, query, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli()).Scan(
		&totalRows, &distinctEvents, &eventIDHash, &minIngest, &maxIngest,
	); err != nil {
		return nil, fmt.Errorf("collect bounded ClickHouse repair dry-run: %w", err)
	}
	duplicateRows := uint64(0)
	if totalRows > distinctEvents {
		duplicateRows = totalRows - distinctEvents
	}
	maxRows := uint64(int64Value(budget["max_rows"]))
	return map[string]interface{}{
		"within_budget":         totalRows > 0 && totalRows <= maxRows,
		"destructive":           false,
		"estimated_rows":        totalRows,
		"distinct_event_ids":    distinctEvents,
		"duplicate_rows":        duplicateRows,
		"event_id_xor_hash":     fmt.Sprintf("%016x", eventIDHash),
		"evidence_source":       "clickhouse.traffic.flows_raw",
		"evidence_collected_at": time.Now().UTC().Format(time.RFC3339Nano),
		"source_watermarks": map[string]interface{}{
			"window_start":         windowStart.UTC().Format(time.RFC3339Nano),
			"window_end":           windowEnd.UTC().Format(time.RFC3339Nano),
			"min_ingest_ts_millis": minIngest,
			"max_ingest_ts_millis": maxIngest,
		},
	}, nil
}

func (p *ClickHouseRepairEvidenceProvider) Reconcile(ctx context.Context, tenantID, repairID string) (map[string]interface{}, error) {
	if p == nil || p.controlDB == nil || p.factsDB == nil {
		return nil, ErrRepairReconcileUnavailable
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(repairID) == "" {
		return nil, fmt.Errorf("repair evidence tenant and repair identity are required")
	}
	var operationID, status string
	var scopeJSON, budgetJSON []byte
	err := p.controlDB.QueryRowContext(ctx, `
		SELECT operation_id,status,input_scope,resource_budget
		FROM data_quality_repairs
		WHERE tenant_id=$1 AND repair_id=$2
	`, tenantID, repairID).Scan(&operationID, &status, &scopeJSON, &budgetJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRepairNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load repair scope for reconcile: %w", err)
	}
	if operationID != "flow_replay_window_v1" || status != "executed" {
		return nil, ErrRepairConflict
	}
	var scope, budget map[string]interface{}
	if err := json.Unmarshal(scopeJSON, &scope); err != nil {
		return nil, fmt.Errorf("decode persisted reconcile scope: %w", err)
	}
	if err := json.Unmarshal(budgetJSON, &budget); err != nil {
		return nil, fmt.Errorf("decode persisted reconcile budget: %w", err)
	}
	if err := validateRepairScope(tenantID, scope, budget); err != nil {
		return nil, fmt.Errorf("persisted reconcile scope failed validation: %w", err)
	}
	windowStart, _ := time.Parse(time.RFC3339, stringValue(scope["window_start"]))
	windowEnd, _ := time.Parse(time.RFC3339, stringValue(scope["window_end"]))
	maxRows := int64Value(budget["max_rows"])
	effectiveTimeout := p.timeout
	if budgetTimeout := time.Duration(int64Value(budget["max_duration_seconds"])) * time.Second; budgetTimeout > 0 && budgetTimeout < effectiveTimeout {
		effectiveTimeout = budgetTimeout
	}
	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()

	sourceRows, err := p.factsDB.QueryContext(queryCtx, `
		SELECT DISTINCT event_id
		FROM traffic.flows_raw
		WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?
		ORDER BY event_id
		LIMIT ?
	`, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli(), maxRows+1)
	if err != nil {
		return nil, fmt.Errorf("read bounded ClickHouse reconcile source: %w", err)
	}
	sourceIDs, err := scanRepairEventIDs(sourceRows, maxRows)
	if err != nil {
		return nil, fmt.Errorf("read bounded ClickHouse reconcile identities: %w", err)
	}

	targetRows, err := p.controlDB.QueryContext(queryCtx, `
		SELECT event_id FROM data_quality_flow_replay_projection
		WHERE tenant_id=$1 AND repair_id=$2
		ORDER BY event_id LIMIT $3
	`, tenantID, repairID, maxRows+1)
	if err != nil {
		return nil, fmt.Errorf("read PostgreSQL replay projection target: %w", err)
	}
	targetIDs, err := scanRepairEventIDs(targetRows, maxRows)
	if err != nil {
		return nil, fmt.Errorf("read bounded PostgreSQL replay projection identities: %w", err)
	}

	receiptRows, err := p.controlDB.QueryContext(queryCtx, `
		SELECT event_id FROM data_quality_replay_projection_receipts
		WHERE tenant_id=$1 AND repair_id=$2 AND projection_id=$3
		ORDER BY event_id LIMIT $4
	`, tenantID, repairID, FlowReplayProjectionVersion, maxRows+1)
	if err != nil {
		return nil, fmt.Errorf("read PostgreSQL replay projection receipts: %w", err)
	}
	receiptIDs, err := scanRepairEventIDs(receiptRows, maxRows)
	if err != nil {
		return nil, fmt.Errorf("read bounded PostgreSQL replay receipt identities: %w", err)
	}

	missingTarget, extraTarget := differenceRepairEventIDs(sourceIDs, targetIDs), differenceRepairEventIDs(targetIDs, sourceIDs)
	missingReceipt, extraReceipt := differenceRepairEventIDs(targetIDs, receiptIDs), differenceRepairEventIDs(receiptIDs, targetIDs)
	var hashMismatchCount int64
	if err := p.controlDB.QueryRowContext(queryCtx, `
		SELECT count(*)
		FROM data_quality_flow_replay_projection p
		JOIN data_quality_replay_projection_receipts r
		  ON r.tenant_id=p.tenant_id AND r.repair_id=p.repair_id AND r.event_id=p.event_id
		 AND r.projection_id=$3
		WHERE p.tenant_id=$1 AND p.repair_id=$2
		  AND (p.source_event_sha256<>r.source_event_sha256 OR p.source_event_sha256<>r.target_payload_sha256)
	`, tenantID, repairID, FlowReplayProjectionVersion).Scan(&hashMismatchCount); err != nil {
		return nil, fmt.Errorf("verify replay projection receipt hashes: %w", err)
	}
	allMatch := len(missingTarget) == 0 && len(extraTarget) == 0 && len(missingReceipt) == 0 &&
		len(extraReceipt) == 0 && hashMismatchCount == 0
	return map[string]interface{}{
		"all_match": allMatch, "server_derived": true,
		"source_count": len(sourceIDs), "target_count": len(targetIDs), "receipt_count": len(receiptIDs),
		"missing_count": len(missingTarget), "extra_count": len(extraTarget),
		"missing_receipt_count": len(missingReceipt), "extra_receipt_count": len(extraReceipt),
		"hash_mismatch_count": hashMismatchCount,
		"missing_event_ids":   missingTarget, "extra_event_ids": extraTarget,
		"missing_receipt_event_ids": missingReceipt, "extra_receipt_event_ids": extraReceipt,
		"source_authority":      "clickhouse.traffic.flows_raw",
		"target_authority":      "postgresql.data_quality_flow_replay_projection",
		"receipt_authority":     "postgresql.data_quality_replay_projection_receipts",
		"projection_version":    FlowReplayProjectionVersion,
		"evidence_collected_at": time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func scanRepairEventIDs(rows *sql.Rows, maxRows int64) (map[string]struct{}, error) {
	defer rows.Close()
	ids := make(map[string]struct{})
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(eventID) == "" {
			return nil, fmt.Errorf("empty event identity")
		}
		ids[eventID] = struct{}{}
		if int64(len(ids)) > maxRows {
			return nil, fmt.Errorf("reconcile identity set exceeds approved max_rows")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func differenceRepairEventIDs(left, right map[string]struct{}) []string {
	difference := make([]string, 0)
	for eventID := range left {
		if _, exists := right[eventID]; !exists {
			difference = append(difference, eventID)
		}
	}
	sort.Strings(difference)
	return difference
}
