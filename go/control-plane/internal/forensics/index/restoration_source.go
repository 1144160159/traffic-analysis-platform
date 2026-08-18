package index

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var lowercaseSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

const (
	restorationSessionSchemaVersion   = "v1"
	restorationSessionIdentityVersion = "session-id-sha256-v1"
)

// RestorationSessionQuery binds a restoration request to the versioned
// traffic.sessions authority before any PCAP object is read. The primary flow
// is the application/control flow; an FTP passive data flow is authorized
// separately by its immutable PCAP row and the advertised endpoint proof.
type RestorationSessionQuery struct {
	TenantID      string
	SessionID     string
	CommunityID   string
	PrimaryFlowID string
	StartTime     time.Time
	EndTime       time.Time
}

// RestorationSessionAuthority is persisted into the restoration manifest so
// an audit can identify the exact versioned session row used for admission.
type RestorationSessionAuthority struct {
	TenantID           string    `json:"tenant_id"`
	SessionID          string    `json:"session_id"`
	CommunityID        string    `json:"community_id"`
	EventID            string    `json:"event_id"`
	ProbeID            string    `json:"probe_id"`
	FlowIDs            []string  `json:"flow_ids"`
	TsStart            time.Time `json:"ts_start"`
	TsEnd              time.Time `json:"ts_end"`
	EventSchemaVersion string    `json:"event_schema_version"`
	AggregateVersion   uint64    `json:"aggregate_version"`
	IdentityVersion    string    `json:"identity_version"`
	SessionVersion     uint64    `json:"session_version"`
	Completeness       string    `json:"completeness"`
	IsPartial          bool      `json:"is_partial"`
}

func (query RestorationSessionQuery) Validate() error {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.SessionID) == "" ||
		strings.TrimSpace(query.CommunityID) == "" || strings.TrimSpace(query.PrimaryFlowID) == "" {
		return errors.New("restoration session identities are required")
	}
	if query.StartTime.IsZero() || query.EndTime.IsZero() || !query.EndTime.After(query.StartTime) {
		return errors.New("restoration session time window is invalid")
	}
	return nil
}

func (authority RestorationSessionAuthority) Validate(query RestorationSessionQuery) error {
	if err := query.Validate(); err != nil {
		return err
	}
	if authority.TenantID != query.TenantID || authority.SessionID != query.SessionID ||
		authority.CommunityID != query.CommunityID {
		return errors.New("restoration session authority identity mismatch")
	}
	if strings.TrimSpace(authority.EventID) == "" || strings.TrimSpace(authority.ProbeID) == "" {
		return errors.New("restoration session authority lacks event or probe identity")
	}
	if authority.EventSchemaVersion != restorationSessionSchemaVersion ||
		authority.IdentityVersion != restorationSessionIdentityVersion ||
		authority.AggregateVersion == 0 || authority.SessionVersion == 0 {
		return errors.New("restoration session authority uses an unsupported version")
	}
	if authority.TsStart.IsZero() || authority.TsEnd.Before(authority.TsStart) ||
		query.StartTime.Before(authority.TsStart) || query.EndTime.After(authority.TsEnd) {
		return errors.New("restoration capture window escapes session authority")
	}
	primaryFound := false
	for _, flowID := range authority.FlowIDs {
		if strings.TrimSpace(flowID) == "" {
			return errors.New("restoration session authority contains an empty flow identity")
		}
		if flowID == query.PrimaryFlowID {
			primaryFound = true
		}
	}
	if !primaryFound {
		return errors.New("restoration primary flow is absent from session authority")
	}
	switch authority.Completeness {
	case "SESSION_COMPLETENESS_COMPLETE":
		if authority.IsPartial {
			return errors.New("complete restoration session is marked partial")
		}
	case "SESSION_COMPLETENESS_PARTIAL", "SESSION_COMPLETENESS_TRUNCATED":
		if !authority.IsPartial {
			return errors.New("incomplete restoration session is not marked partial")
		}
	default:
		return errors.New("restoration session completeness is not admissible")
	}
	return nil
}

func restorationSessionSQL() string {
	return `
		SELECT DISTINCT
			count(),
			argMax(tenant_id, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(session_id, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(community_id, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(event_id, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(probe_id, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(flow_ids, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(ts_start, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(ts_end, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(event_schema_version, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(aggregate_version, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(identity_version, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(session_version, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(completeness, tuple(aggregate_version, session_version, ingest_ts, event_id)),
			argMax(is_partial, tuple(aggregate_version, session_version, ingest_ts, event_id))
		FROM traffic.sessions
		WHERE tenant_id = ? AND session_id = ? AND community_id = ?`
}

func restorationSchemaSQL() string {
	return `
		SELECT
			uniqExactIf(name, table='sessions' AND name IN (
				'tenant_id','session_id','community_id','event_id','probe_id','flow_ids','ts_start','ts_end',
				'event_schema_version','aggregate_version','identity_version','session_version','completeness','is_partial'
			)) = 14
			AND countIf(table IN ('pcap_index_v2','pcap_index_v2_local')) = 58
			AND countIf(table IN ('pcap_index_v2','pcap_index_v2_local') AND (name,replaceAll(type,char(39),'')) IN (
				('tenant_id','String'),('probe_id','String'),('file_key','String'),('bucket','String'),
				('object_version','String'),('etag','String'),('original_size','UInt64'),('stored_size','UInt64'),
				('compression','LowCardinality(String)'),('manifest_version','UInt16'),('kafka_topic','String'),
				('kafka_partition','Int32'),('kafka_offset','Int64'),('kafka_key_sha256','FixedString(64)'),
				('kafka_headers_sha256','FixedString(64)'),('raw_sha256','FixedString(64)'),
				('projection_identity','FixedString(64)'),('ts_start','DateTime64(3, UTC)'),
				('ts_end','DateTime64(3, UTC)'),('byte_size','UInt64'),('zstd_level','UInt8'),
				('sha256','String'),('community_id','String'),('flow_id','String'),
				('offset_start','Nullable(UInt64)'),('offset_end','Nullable(UInt64)'),
				('bloom_filter_b64','String'),('community_ids','Array(String)'),
				('created_ts','DateTime64(3, UTC)')
			)) = 58
			AND (SELECT engine FROM system.tables WHERE database=currentDatabase() AND name='pcap_index_v2_local')
				= 'ReplicatedReplacingMergeTree'
			AND position(replaceAll(replaceAll(
				(SELECT engine_full FROM system.tables WHERE database=currentDatabase() AND name='pcap_index_v2'),
				' ', ''), char(96), ''), 'cityHash64(tenant_id,file_key)') > 0
		FROM system.columns
		WHERE database = currentDatabase() AND table IN ('sessions','pcap_index_v2','pcap_index_v2_local')`
}

// VerifyRestorationSchema prevents an explicitly enabled writer from becoming
// ready against a legacy ClickHouse schema. Runtime services never run DDL.
func (c *IndexClient) VerifyRestorationSchema(ctx context.Context) error {
	row, err := c.client.QueryRow(ctx, restorationSchemaSQL())
	if err != nil {
		return fmt.Errorf("query restoration ClickHouse schema: %w", err)
	}
	var ready bool
	if err := row.Scan(&ready); err != nil {
		return fmt.Errorf("scan restoration ClickHouse schema: %w", err)
	}
	if !ready {
		return errors.New("restoration ClickHouse sessions/pcap_index_v2 schema is not exact")
	}
	return nil
}

// VerifyRestorationSession selects the latest version deterministically. It
// fails closed for legacy/unversioned rows and for a request that widens the
// authoritative session time range.
func (c *IndexClient) VerifyRestorationSession(
	ctx context.Context, query RestorationSessionQuery,
) (RestorationSessionAuthority, error) {
	if err := query.Validate(); err != nil {
		return RestorationSessionAuthority{}, err
	}
	row, err := c.client.QueryRow(ctx, restorationSessionSQL(), query.TenantID, query.SessionID, query.CommunityID)
	if err != nil {
		return RestorationSessionAuthority{}, fmt.Errorf("query restoration session authority: %w", err)
	}
	var (
		count      uint64
		authority  RestorationSessionAuthority
		start, end int64
		isPartial  uint8
	)
	if err := row.Scan(
		&count, &authority.TenantID, &authority.SessionID, &authority.CommunityID,
		&authority.EventID, &authority.ProbeID, &authority.FlowIDs, &start, &end,
		&authority.EventSchemaVersion, &authority.AggregateVersion,
		&authority.IdentityVersion, &authority.SessionVersion,
		&authority.Completeness, &isPartial,
	); err != nil {
		return RestorationSessionAuthority{}, fmt.Errorf("scan restoration session authority: %w", err)
	}
	if count == 0 {
		return RestorationSessionAuthority{}, errors.New("restoration session authority was not found")
	}
	authority.TsStart = timeFromIndexTimestamp(start)
	authority.TsEnd = timeFromIndexTimestamp(end)
	authority.IsPartial = isPartial != 0
	if err := authority.Validate(query); err != nil {
		return RestorationSessionAuthority{}, err
	}
	return authority, nil
}

type RestorationSourceQuery struct {
	TenantID    string
	ProbeID     string
	CommunityID string
	FlowID      string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
}

type RestorationSource struct {
	TenantID           string
	ProbeID            string
	ProjectionIdentity string
	FileKey            string
	Bucket             string
	ObjectVersion      string
	ETag               string
	SHA256             string
	OriginalSize       uint64
	StoredSize         uint64
	Compression        string
	ManifestVersion    uint16
	CommunityID        string
	FlowID             string
	OffsetStart        *uint64
	OffsetEnd          *uint64
	TsStart            time.Time
	TsEnd              time.Time
}

func (query RestorationSourceQuery) Validate() error {
	if strings.TrimSpace(query.TenantID) == "" {
		return errors.New("restoration source tenant_id is required")
	}
	if strings.TrimSpace(query.ProbeID) == "" || strings.TrimSpace(query.CommunityID) == "" || strings.TrimSpace(query.FlowID) == "" {
		return errors.New("restoration source probe, community and flow identities are required")
	}
	if query.StartTime.IsZero() || query.EndTime.IsZero() || !query.EndTime.After(query.StartTime) {
		return errors.New("restoration source time window is invalid")
	}
	if query.Limit <= 0 || query.Limit > 1000 {
		return errors.New("restoration source limit must be between 1 and 1000")
	}
	return nil
}

func (source RestorationSource) Validate(expectedTenant, expectedProbe string) error {
	if source.TenantID != expectedTenant || strings.TrimSpace(source.TenantID) == "" {
		return errors.New("restoration source tenant authority mismatch")
	}
	if source.ProbeID != expectedProbe || strings.TrimSpace(source.ProbeID) == "" {
		return errors.New("restoration source probe authority mismatch")
	}
	for label, value := range map[string]string{
		"projection_identity": source.ProjectionIdentity,
		"file_key":            source.FileKey,
		"bucket":              source.Bucket,
		"object_version":      source.ObjectVersion,
		"etag":                source.ETag,
		"community_id":        source.CommunityID,
		"flow_id":             source.FlowID,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("restoration source %s is missing or unsafe", label)
		}
	}
	if !lowercaseSHA256.MatchString(source.ProjectionIdentity) {
		return errors.New("restoration source projection identity is not lowercase SHA-256")
	}
	if !lowercaseSHA256.MatchString(source.SHA256) {
		return errors.New("restoration source object SHA-256 is invalid")
	}
	if source.StoredSize == 0 || source.OriginalSize == 0 {
		return errors.New("restoration source sizes must be positive")
	}
	if source.ManifestVersion < 2 {
		return errors.New("restoration source manifest version is not v2")
	}
	if source.Compression != "none" && source.Compression != "zstd" {
		return errors.New("restoration source compression is unsupported")
	}
	if source.TsStart.IsZero() || source.TsEnd.IsZero() || source.TsEnd.Before(source.TsStart) {
		return errors.New("restoration source capture window is invalid")
	}
	if (source.OffsetStart == nil) != (source.OffsetEnd == nil) {
		return errors.New("restoration source byte range is incomplete")
	}
	if source.OffsetStart != nil && (*source.OffsetStart != 0 || *source.OffsetEnd != source.StoredSize) {
		return errors.New("restoration source byte range must authorize the exact stored object")
	}
	return nil
}

func restorationSourceSQL(limit int) string {
	return fmt.Sprintf(`
		SELECT
			tenant_id, probe_id, projection_identity, file_key, bucket,
			object_version, etag, sha256, original_size, stored_size,
			compression, manifest_version, community_id, flow_id,
			offset_start, offset_end, ts_start, ts_end
		FROM traffic.pcap_index_v2
		WHERE tenant_id = ?
		  AND probe_id = ?
		  AND community_id = ?
		  AND flow_id = ?
		  AND ts_start <= fromUnixTimestamp64Milli(?)
		  AND ts_end >= fromUnixTimestamp64Milli(?)
		  AND manifest_version >= 2
		  AND bucket != '' AND object_version != '' AND etag != ''
		  AND match(sha256, '^[0-9a-f]{64}$')
		  AND match(projection_identity, '^[0-9a-f]{64}$')
		ORDER BY ts_start ASC, projection_identity ASC
		LIMIT %d`, limit)
}

// LookupRestorationSources is intentionally separate from legacy PCAP cutting.
// It accepts only immutable manifest-v2 authority rows and never silently
// falls back to a payload-only index row.
func (c *IndexClient) LookupRestorationSources(
	ctx context.Context, query RestorationSourceQuery,
) ([]RestorationSource, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows, err := c.client.Query(ctx, restorationSourceSQL(query.Limit),
		query.TenantID, query.ProbeID, query.CommunityID, query.FlowID,
		query.EndTime.UnixMilli(), query.StartTime.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("query restoration source authority: %w", err)
	}
	defer rows.Close()
	sources := make([]RestorationSource, 0)
	for rows.Next() {
		var source RestorationSource
		if err := rows.Scan(
			&source.TenantID, &source.ProbeID, &source.ProjectionIdentity,
			&source.FileKey, &source.Bucket, &source.ObjectVersion, &source.ETag,
			&source.SHA256, &source.OriginalSize, &source.StoredSize,
			&source.Compression, &source.ManifestVersion, &source.CommunityID, &source.FlowID,
			&source.OffsetStart, &source.OffsetEnd, &source.TsStart, &source.TsEnd,
		); err != nil {
			return nil, fmt.Errorf("scan restoration source authority: %w", err)
		}
		source.TsStart = source.TsStart.UTC()
		source.TsEnd = source.TsEnd.UTC()
		if err := source.Validate(query.TenantID, query.ProbeID); err != nil {
			return nil, fmt.Errorf("invalid restoration source %s: %w", source.ProjectionIdentity, err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restoration source authority: %w", err)
	}
	if len(sources) == 0 {
		return nil, errors.New("no immutable manifest-v2 restoration sources found")
	}
	return sources, nil
}
