package index

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VerifiedCutSourceQuery is the M09 worker-facing view of the M02 manifest-v2
// PCAP projection. A probe identity is mandatory so a broad tenant time-range
// query can never become an accidental full-bucket scan.
type VerifiedCutSourceQuery struct {
	TenantID    string
	ProbeID     string
	CommunityID string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
}

func (query VerifiedCutSourceQuery) Validate() error {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.ProbeID) == "" {
		return errors.New("verified PCAP cut requires tenant and probe authority")
	}
	if query.StartTime.IsZero() || query.EndTime.IsZero() || !query.EndTime.After(query.StartTime) {
		return errors.New("verified PCAP cut time window is invalid")
	}
	if query.Limit <= 0 || query.Limit > 1000 {
		return errors.New("verified PCAP cut source limit must be between 1 and 1000")
	}
	return nil
}

func verifiedCutSourceSQL(limit int) string {
	return fmt.Sprintf(`
		SELECT
			tenant_id, probe_id, projection_identity, file_key, bucket,
			object_version, etag, sha256, original_size, stored_size,
			compression, manifest_version, community_id, flow_id,
			offset_start, offset_end, ts_start, ts_end
		FROM traffic.pcap_index_v2
		WHERE tenant_id = ?
		  AND probe_id = ?
		  AND (? = '' OR community_id = ?)
		  AND ts_start <= fromUnixTimestamp64Milli(?)
		  AND ts_end >= fromUnixTimestamp64Milli(?)
		  AND manifest_version >= 2
		  AND bucket != '' AND object_version != '' AND etag != ''
		  AND match(sha256, '^[0-9a-f]{64}$')
		  AND match(projection_identity, '^[0-9a-f]{64}$')
		ORDER BY ts_start ASC, projection_identity ASC
		LIMIT %d`, limit)
}

// LookupVerifiedCutSources returns only immutable versioned object authority.
// The caller must still HEAD and hash the exact object version before parsing.
func (c *IndexClient) LookupVerifiedCutSources(ctx context.Context, query VerifiedCutSourceQuery) ([]RestorationSource, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows, err := c.client.Query(ctx, verifiedCutSourceSQL(query.Limit),
		query.TenantID, query.ProbeID, query.CommunityID, query.CommunityID,
		query.EndTime.UnixMilli(), query.StartTime.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("query verified PCAP cut sources: %w", err)
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
			return nil, fmt.Errorf("scan verified PCAP cut source: %w", err)
		}
		source.TsStart = source.TsStart.UTC()
		source.TsEnd = source.TsEnd.UTC()
		if err := source.Validate(query.TenantID, query.ProbeID); err != nil {
			return nil, fmt.Errorf("invalid verified PCAP cut source %s: %w", source.ProjectionIdentity, err)
		}
		if query.CommunityID != "" && source.CommunityID != query.CommunityID {
			return nil, errors.New("verified PCAP source community authority mismatch")
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate verified PCAP cut sources: %w", err)
	}
	if len(sources) == 0 {
		return nil, errors.New("no immutable manifest-v2 PCAP cut sources found")
	}
	return sources, nil
}
