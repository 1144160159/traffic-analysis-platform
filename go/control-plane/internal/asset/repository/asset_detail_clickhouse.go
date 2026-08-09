package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

const (
	assetObservationSource = "clickhouse.sessions"
	assetAlertSource       = "clickhouse.alerts.argmax_state_v1"
)

// AssetDetailClickHouseReader resolves only the current PostgreSQL-owned asset
// identity supplied by the service. Every query is tenant, identity, lower
// watermark and PG snapshot upper-bound constrained.
type AssetDetailClickHouseReader struct {
	db         *sql.DB
	queryLimit time.Duration
	lookback   time.Duration
	alertLimit int
}

func NewAssetDetailClickHouseReader(db *sql.DB, cfg config.AssetDetailConfig) (*AssetDetailClickHouseReader, error) {
	if db == nil {
		return nil, fmt.Errorf("asset detail ClickHouse database is required")
	}
	if cfg.ClickHouseQuery <= 0 || cfg.ClickHouseLookback <= 0 {
		return nil, fmt.Errorf("asset detail ClickHouse query timeout and lookback must be positive")
	}
	if cfg.ClickHouseAlertLimit <= 0 || cfg.ClickHouseAlertLimit > 500 {
		return nil, fmt.Errorf("asset detail ClickHouse alert limit must be within 1..500")
	}
	return &AssetDetailClickHouseReader{
		db:         db,
		queryLimit: cfg.ClickHouseQuery,
		lookback:   cfg.ClickHouseLookback,
		alertLimit: cfg.ClickHouseAlertLimit,
	}, nil
}

func (r *AssetDetailClickHouseReader) ReadAssetObservations(
	ctx context.Context,
	tenantID string,
	asset *config.AssetRecord,
	asOf time.Time,
) (*config.AssetObservationSummary, map[string]string, error) {
	if tenantID == "" || asset == nil || asset.TenantID != tenantID {
		return nil, nil, fmt.Errorf("tenant-scoped asset is required")
	}
	ip := strings.TrimSpace(asset.IPAddress)
	if ip == "" {
		return nil, nil, fmt.Errorf("asset has no current authoritative IP identity")
	}
	if asOf.IsZero() {
		return nil, nil, fmt.Errorf("PostgreSQL snapshot as_of is required")
	}
	windowEnd := asOf.UTC()
	windowStart := windowEnd.Add(-r.lookback)
	queryCtx, cancel := context.WithTimeout(ctx, r.queryLimit)
	defer cancel()

	var (
		sessionCount, bytesTotal, packetsTotal, distinctPeers uint64
		protocolsJSON                                         string
		firstObservedMillis, lastObservedMillis               int64
	)
	err := r.db.QueryRowContext(queryCtx, `
		SELECT
			count() AS session_count,
			coalesce(sum(bytes_total), 0) AS bytes_total,
			coalesce(sum(num_pkts), 0) AS packets_total,
			uniqExact(if(src_ip = ?, dst_ip, src_ip)) AS distinct_peers,
			toJSONString(arraySort(groupUniqArray(32)(protocol))) AS protocols_json,
			coalesce(min(ts_start), 0) AS first_observed,
			coalesce(max(ts_end), 0) AS last_observed
		FROM traffic.sessions
		WHERE tenant_id = ?
		  AND ts_start >= ? AND ts_start <= ?
		  AND (src_ip = ? OR dst_ip = ?)`,
		ip, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli(), ip, ip,
	).Scan(
		&sessionCount, &bytesTotal, &packetsTotal, &distinctPeers,
		&protocolsJSON, &firstObservedMillis, &lastObservedMillis,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query bounded asset observations: %w", err)
	}
	protocols := make([]uint32, 0)
	if protocolsJSON != "" {
		if err := json.Unmarshal([]byte(protocolsJSON), &protocols); err != nil {
			return nil, nil, fmt.Errorf("decode asset observation protocols: %w", err)
		}
	}
	summary := &config.AssetObservationSummary{
		AssetID: asset.AssetID,
		ResolvedIdentity: config.AssetResolvedIdentity{
			Kind: "ip", Value: ip, AssetRevision: asset.Revision,
		},
		Source: assetObservationSource, WindowStart: windowStart, WindowEnd: windowEnd,
		SessionCount: sessionCount, BytesTotal: bytesTotal, PacketsTotal: packetsTotal,
		DistinctPeers: distinctPeers, Protocols: protocols,
	}
	watermarks := map[string]string{
		"clickhouse.sessions.query_as_of": windowEnd.Format(time.RFC3339Nano),
	}
	if firstObservedMillis > 0 {
		value := time.UnixMilli(firstObservedMillis).UTC()
		summary.FirstObservedAt = &value
	}
	if lastObservedMillis > 0 {
		value := time.UnixMilli(lastObservedMillis).UTC()
		summary.LastObservedAt = &value
		watermarks["clickhouse.sessions.max_ts_end"] = value.Format(time.RFC3339Nano)
	}
	return summary, watermarks, nil
}

func (r *AssetDetailClickHouseReader) ReadAssetAlertContext(
	ctx context.Context,
	tenantID string,
	asset *config.AssetRecord,
	asOf time.Time,
) (*config.AssetAlertContext, map[string]string, error) {
	if tenantID == "" || asset == nil || asset.TenantID != tenantID {
		return nil, nil, fmt.Errorf("tenant-scoped asset is required")
	}
	ip := strings.TrimSpace(asset.IPAddress)
	if ip == "" {
		return nil, nil, fmt.Errorf("asset has no current authoritative IP identity")
	}
	if asOf.IsZero() {
		return nil, nil, fmt.Errorf("PostgreSQL snapshot as_of is required")
	}
	windowEnd := asOf.UTC()
	windowStart := windowEnd.Add(-r.lookback)
	queryCtx, cancel := context.WithTimeout(ctx, r.queryLimit)
	defer cancel()

	rows, err := r.db.QueryContext(queryCtx, `
		SELECT
			alert_id,
			argMax(severity, tuple(state_version, updated_at, event_id)) AS latest_severity,
			argMax(status, tuple(state_version, updated_at, event_id)) AS latest_status,
			argMax(alert_type, tuple(state_version, updated_at, event_id)) AS latest_alert_type,
			argMax(src_ip, tuple(state_version, updated_at, event_id)) AS latest_src_ip,
			argMax(dst_ip, tuple(state_version, updated_at, event_id)) AS latest_dst_ip,
			argMax(src_port, tuple(state_version, updated_at, event_id)) AS latest_src_port,
			argMax(dst_port, tuple(state_version, updated_at, event_id)) AS latest_dst_port,
			argMax(protocol, tuple(state_version, updated_at, event_id)) AS latest_protocol,
			argMax(score, tuple(state_version, updated_at, event_id)) AS latest_score,
			toJSONString(argMax(evidence_ids, tuple(state_version, updated_at, event_id))) AS evidence_ids_json,
			argMax(first_seen, tuple(state_version, updated_at, event_id)) AS latest_first_seen,
			argMax(last_seen, tuple(state_version, updated_at, event_id)) AS latest_last_seen,
			argMax(state_version, tuple(state_version, updated_at, event_id)) AS latest_state_version,
			argMax(event_id, tuple(state_version, updated_at, event_id)) AS latest_event_id
		FROM traffic.alerts
		WHERE tenant_id = ?
		  AND last_seen >= ? AND last_seen <= ?
		  AND (src_ip = ? OR dst_ip = ?)
		GROUP BY alert_id
		ORDER BY latest_last_seen DESC, alert_id ASC
		LIMIT ?`,
		tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli(), ip, ip, r.alertLimit+1,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query bounded asset alert context: %w", err)
	}
	defer rows.Close()

	result := &config.AssetAlertContext{
		AssetID: asset.AssetID,
		ResolvedIdentity: config.AssetResolvedIdentity{
			Kind: "ip", Value: ip, AssetRevision: asset.Revision,
		},
		Source: assetAlertSource, WindowStart: windowStart, WindowEnd: windowEnd,
		Alerts: make([]config.AssetAlertSummary, 0, r.alertLimit),
	}
	var maxLastSeen time.Time
	for rows.Next() {
		var item config.AssetAlertSummary
		var evidenceJSON string
		var firstSeenMillis, lastSeenMillis int64
		if err := rows.Scan(
			&item.AlertID, &item.Severity, &item.Status, &item.AlertType,
			&item.SourceIP, &item.DestinationIP, &item.SourcePort, &item.DestinationPort,
			&item.Protocol, &item.Score, &evidenceJSON, &firstSeenMillis, &lastSeenMillis,
			&item.StateVersion, &item.EventID,
		); err != nil {
			return nil, nil, fmt.Errorf("scan bounded asset alert context: %w", err)
		}
		if evidenceJSON != "" {
			if err := json.Unmarshal([]byte(evidenceJSON), &item.EvidenceIDs); err != nil {
				return nil, nil, fmt.Errorf("decode alert %s evidence IDs: %w", item.AlertID, err)
			}
		}
		if item.EvidenceIDs == nil {
			item.EvidenceIDs = []string{}
		}
		item.FirstSeen = time.UnixMilli(firstSeenMillis).UTC()
		item.LastSeen = time.UnixMilli(lastSeenMillis).UTC()
		if len(result.Alerts) == r.alertLimit {
			result.Truncated = true
			continue
		}
		result.Alerts = append(result.Alerts, item)
		if item.LastSeen.After(maxLastSeen) {
			maxLastSeen = item.LastSeen
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate bounded asset alert context: %w", err)
	}
	watermarks := map[string]string{
		"clickhouse.alerts.query_as_of": windowEnd.Format(time.RFC3339Nano),
	}
	if !maxLastSeen.IsZero() {
		watermarks["clickhouse.alerts.max_last_seen"] = maxLastSeen.Format(time.RFC3339Nano)
	}
	return result, watermarks, nil
}
