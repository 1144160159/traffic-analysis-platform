package fusion

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ClickHouseSourceFactReader struct {
	db *sql.DB
}

func NewClickHouseSourceFactReader(db *sql.DB) (*ClickHouseSourceFactReader, error) {
	if db == nil {
		return nil, fmt.Errorf("ClickHouse source-fact database is required")
	}
	return &ClickHouseSourceFactReader{db: db}, nil
}

func (reader *ClickHouseSourceFactReader) ReadSourceFacts(
	ctx context.Context,
	tenantID string,
	sourceID string,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) (SourceFactBatch, error) {
	if reader == nil || reader.db == nil || strings.TrimSpace(tenantID) == "" ||
		!windowStart.Before(windowEnd) || limit < 1 || limit > MaxSourceFacts {
		return SourceFactBatch{}, fmt.Errorf("invalid source-fact read request")
	}
	table, ok := sourceFactTable(sourceID)
	if !ok {
		return SourceFactBatch{}, fmt.Errorf("unsupported fusion source %q", sourceID)
	}
	var total int64
	countQuery := fmt.Sprintf(`SELECT count() FROM %s WHERE tenant_id=? AND event_time_ms>=? AND event_time_ms<?`, table)
	if err := reader.db.QueryRowContext(ctx, countQuery, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli()).Scan(&total); err != nil {
		return SourceFactBatch{}, fmt.Errorf("count %s source facts: %w", sourceID, err)
	}
	query := fmt.Sprintf(`SELECT aggregate_id,event_id,event_time_ms,source_topic,source_partition,source_offset,
		source_payload_sha256,source_version,projection_identity,payload_base64,projection_hash
		FROM %s WHERE tenant_id=? AND event_time_ms>=? AND event_time_ms<?
		ORDER BY event_time_ms,source_topic,source_partition,source_offset,projection_identity LIMIT ?`, table)
	rows, err := reader.db.QueryContext(ctx, query, tenantID, windowStart.UnixMilli(), windowEnd.UnixMilli(), limit)
	if err != nil {
		return SourceFactBatch{}, fmt.Errorf("read %s source facts: %w", sourceID, err)
	}
	defer rows.Close()
	facts := make([]SourceFact, 0, minInt64(total, int64(limit)))
	for rows.Next() {
		var fact SourceFact
		var eventTimeMS int64
		if err := rows.Scan(
			&fact.AggregateID, &fact.EventID, &eventTimeMS, &fact.SourceTopic,
			&fact.SourcePartition, &fact.SourceOffset, &fact.SourcePayloadSHA256,
			&fact.SourceVersion, &fact.ProjectionIdentity, &fact.PayloadBase64,
			&fact.ProjectionHash,
		); err != nil {
			return SourceFactBatch{}, fmt.Errorf("scan %s source fact: %w", sourceID, err)
		}
		fact.EventTime = time.UnixMilli(eventTimeMS).UTC()
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return SourceFactBatch{}, fmt.Errorf("iterate %s source facts: %w", sourceID, err)
	}
	return SourceFactBatch{Facts: facts, Truncated: total > int64(limit), Total: total}, nil
}

func sourceFactTable(sourceID string) (string, bool) {
	switch strings.TrimSpace(sourceID) {
	case "traffic":
		return "traffic.source_flow_facts_v1", true
	case "asset":
		return "traffic.source_asset_facts_v1", true
	case "log":
		return "traffic.source_device_log_facts_v1", true
	case "behavior":
		return "traffic.source_user_behavior_facts_v1", true
	default:
		return "", false
	}
}

func minInt64(left, right int64) int {
	if left < right {
		return int(left)
	}
	return int(right)
}
