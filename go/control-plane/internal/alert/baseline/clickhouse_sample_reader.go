package baseline

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ClickHouseSampleReader struct {
	db *sql.DB
}

func NewClickHouseSampleReader(db *sql.DB) (*ClickHouseSampleReader, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: ClickHouse database is required", ErrInvalidRequest)
	}
	return &ClickHouseSampleReader{db: db}, nil
}

func (reader *ClickHouseSampleReader) ReadDynamicSample(ctx context.Context, job BuildJob) (DynamicSampleResult, error) {
	if reader == nil || reader.db == nil || job.BaselineKind != "dynamic" || job.WindowStart == nil || job.WindowEnd == nil ||
		!job.WindowStart.Before(*job.WindowEnd) || strings.TrimSpace(job.TenantID) == "" {
		return DynamicSampleResult{}, fmt.Errorf("%w: invalid ClickHouse baseline sample request", ErrInvalidRequest)
	}
	if job.EntityType == "account" {
		return reader.readAccountSample(ctx, job)
	}
	filter, value, err := sessionEntityFilter(job.EntityType, job.EntityID)
	if err != nil {
		return DynamicSampleResult{}, err
	}
	query := fmt.Sprintf(`SELECT count(),countIf(is_partial=0),
		ifNull(maxIf(event_time_end_ms,is_partial=0),0),ifNull(maxIf(source_watermark_ms,is_partial=0),0),
		avgIf(toFloat64(bytes_total),is_partial=0),stddevPopIf(toFloat64(bytes_total),is_partial=0),
		avgIf(toFloat64(num_pkts),is_partial=0),stddevPopIf(toFloat64(num_pkts),is_partial=0),
		avgIf(toFloat64(duration_ms),is_partial=0),stddevPopIf(toFloat64(duration_ms),is_partial=0)
		FROM traffic.sessions WHERE tenant_id=? AND ts_start>=? AND ts_start<? AND %s`, filter)
	var total, eligible, maxEventMS, watermarkMS int64
	var bytesMean, bytesStd, packetsMean, packetsStd, durationMean, durationStd float64
	err = reader.db.QueryRowContext(ctx, query, job.TenantID, job.WindowStart.UnixMilli(), job.WindowEnd.UnixMilli(), value).Scan(
		&total, &eligible, &maxEventMS, &watermarkMS, &bytesMean, &bytesStd, &packetsMean, &packetsStd, &durationMean, &durationStd)
	if err != nil {
		return DynamicSampleResult{}, fmt.Errorf("read ClickHouse session baseline sample: %w", err)
	}
	quality, reasons := sessionSampleQuality(total, eligible)
	var maxEventTime *time.Time
	if maxEventMS > 0 {
		value := time.UnixMilli(maxEventMS).UTC()
		maxEventTime = &value
	}
	querySHA, _ := canonicalSHA256(map[string]interface{}{"query_version": "sessions-dynamic-baseline-v1", "entity_type": job.EntityType})
	return DynamicSampleResult{
		MaxEventTime: maxEventTime, RowCount: total, EligibleRowCount: eligible,
		QualityStatus: quality, PartialReasons: reasons,
		SourceWatermark: map[string]interface{}{"table": "traffic.sessions", "source_watermark_ms": watermarkMS,
			"window_start_ms": job.WindowStart.UnixMilli(), "window_end_ms": job.WindowEnd.UnixMilli()},
		SourceQuerySHA256: querySHA,
		Statistics: map[string]interface{}{
			"bytes_per_session":   map[string]float64{"mean": bytesMean, "stddev": bytesStd},
			"packets_per_session": map[string]float64{"mean": packetsMean, "stddev": packetsStd},
			"duration_ms":         map[string]float64{"mean": durationMean, "stddev": durationStd},
		},
		Provenance: map[string]interface{}{"query_version": "sessions-dynamic-baseline-v1", "entity_filter": filter,
			"quality_semantics": "only is_partial=0 rows are eligible"},
	}, nil
}

func (reader *ClickHouseSampleReader) readAccountSample(ctx context.Context, job BuildJob) (DynamicSampleResult, error) {
	const query = `SELECT count(),ifNull(toInt64(toUnixTimestamp(max(timestamp)))*1000,0),
		uniqExact(source_ip),uniqExact(resource),uniqExact(event_type)
		FROM traffic.user_events WHERE tenant_id=? AND timestamp>=? AND timestamp<? AND username=?`
	var total, maxEventMS, sourceIPs, resources, eventTypes int64
	err := reader.db.QueryRowContext(ctx, query, job.TenantID, *job.WindowStart, *job.WindowEnd, job.EntityID).Scan(
		&total, &maxEventMS, &sourceIPs, &resources, &eventTypes)
	if err != nil {
		return DynamicSampleResult{}, fmt.Errorf("read ClickHouse account baseline sample: %w", err)
	}
	reasons := []string{"source_completeness_unavailable"}
	if total == 0 {
		reasons = append(reasons, "not_arrived")
	}
	var maxEventTime *time.Time
	if maxEventMS > 0 {
		value := time.UnixMilli(maxEventMS).UTC()
		maxEventTime = &value
	}
	querySHA, _ := canonicalSHA256(map[string]interface{}{"query_version": "user-events-dynamic-baseline-v1"})
	return DynamicSampleResult{
		MaxEventTime: maxEventTime, RowCount: total, EligibleRowCount: 0, QualityStatus: "partial",
		PartialReasons: uniqueSorted(reasons),
		SourceWatermark: map[string]interface{}{"table": "traffic.user_events", "max_event_time_ms": maxEventMS,
			"window_start": job.WindowStart.UTC(), "window_end": job.WindowEnd.UTC()},
		SourceQuerySHA256: querySHA,
		Statistics: map[string]interface{}{"events_per_window": total, "source_ip_count": sourceIPs,
			"resource_count": resources, "event_type_count": eventTypes},
		Provenance: map[string]interface{}{"query_version": "user-events-dynamic-baseline-v1",
			"quality_semantics": "legacy user_events has no row-level completeness marker; fail-visible partial"},
	}, nil
}

func sessionEntityFilter(entityType, entityID string) (string, interface{}, error) {
	switch entityType {
	case "asset":
		if strings.TrimSpace(entityID) == "" {
			return "", nil, fmt.Errorf("%w: asset identity is empty", ErrInvalidRequest)
		}
		return "src_ip=?", entityID, nil
	case "port":
		value, err := strconv.ParseUint(entityID, 10, 16)
		if err != nil {
			return "", nil, fmt.Errorf("%w: port identity is invalid", ErrInvalidRequest)
		}
		return "dst_port=?", uint32(value), nil
	case "protocol":
		value, err := strconv.ParseUint(entityID, 10, 8)
		if err != nil {
			return "", nil, fmt.Errorf("%w: protocol identity is invalid", ErrInvalidRequest)
		}
		return "protocol=?", uint8(value), nil
	case "time":
		value, err := strconv.ParseUint(entityID, 10, 5)
		if err != nil || value > 23 {
			return "", nil, fmt.Errorf("%w: hour identity is invalid", ErrInvalidRequest)
		}
		return "toHour(toDateTime(intDiv(ts_start,1000)))=?", uint8(value), nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported dynamic baseline entity type %q", ErrInvalidRequest, entityType)
	}
}

func sessionSampleQuality(total, eligible int64) (string, []string) {
	reasons := make([]string, 0, 2)
	if total == 0 {
		reasons = append(reasons, "not_arrived")
	}
	if eligible < total {
		reasons = append(reasons, "partial_source_rows_excluded")
	}
	if len(reasons) > 0 {
		return "partial", reasons
	}
	return "complete", reasons
}
