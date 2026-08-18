package index

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

func TestRestorationSourceClickHouseRoundTrip(t *testing.T) {
	if os.Getenv("M03_RESTORATION_CLICKHOUSE_INTEGRATION_ENABLED") != "true" {
		t.Skip("owned ephemeral ClickHouse is not enabled")
	}
	if os.Getenv("M03_RESTORATION_CLICKHOUSE_SENTINEL") != "codex_ephemeral_m03_restoration_clickhouse" {
		t.Fatal("refusing a ClickHouse instance that is not explicitly owned by this test")
	}
	address := os.Getenv("M03_RESTORATION_CLICKHOUSE_NATIVE_ADDR")
	if address == "" {
		t.Fatal("owned ephemeral ClickHouse native address is required")
	}
	connection, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{address},
		Auth:        clickhouse.Auth{Database: "traffic", Username: "default"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ClickHouse: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := connection.Ping(ctx); err != nil {
		t.Fatalf("ping ClickHouse: %v", err)
	}

	tenantID := "tenant-go-restoration"
	probeID := "probe-go-restoration"
	projectionID := strings.Repeat("a", 64)
	objectSHA := strings.Repeat("b", 64)
	start := time.Date(2026, 8, 14, 0, 0, 0, 123_000_000, time.UTC)
	end := start.Add(1500 * time.Millisecond)
	if err := connection.Exec(ctx, "TRUNCATE TABLE traffic.pcap_index_v2"); err != nil {
		t.Fatalf("reset owned manifest-v2 source table: %v", err)
	}
	if err := connection.Exec(ctx, `INSERT INTO traffic.pcap_index_v2 (
		tenant_id,probe_id,file_key,bucket,object_version,etag,original_size,stored_size,
		compression,manifest_version,kafka_topic,kafka_partition,kafka_offset,kafka_key_sha256,
		kafka_headers_sha256,raw_sha256,projection_identity,ts_start,ts_end,byte_size,zstd_level,
		sha256,community_id,flow_id,offset_start,offset_end,bloom_filter_b64,community_ids,created_ts
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,fromUnixTimestamp64Milli(?),
		fromUnixTimestamp64Milli(?),?,?,?,?,?,?,?,?,?,fromUnixTimestamp64Milli(?))`,
		tenantID, probeID, "pcap/tenant-go-restoration/source.pcap.zst", "pcap-archive",
		"version-1", "etag-1", uint64(100), uint64(80), "zstd", uint16(2),
		"pcap.index.v1", int32(0), int64(7), strings.Repeat("c", 64),
		strings.Repeat("d", 64), strings.Repeat("e", 64), projectionID,
		start.UnixMilli(), end.UnixMilli(), uint64(80), uint8(3), objectSHA, "1:context", "flow-context", uint64(0), uint64(80),
		"", []string{}, end.UnixMilli(),
	); err != nil {
		t.Fatalf("insert manifest-v2 source: %v", err)
	}

	client := NewIndexClient(storage.NewClickHouseClientFromConn(connection, zap.NewNop()), zap.NewNop())
	sources, err := client.LookupRestorationSources(ctx, RestorationSourceQuery{
		TenantID: tenantID, ProbeID: probeID, CommunityID: "1:context", FlowID: "flow-context",
		StartTime: start.Add(250 * time.Millisecond), EndTime: end.Add(-250 * time.Millisecond), Limit: 10,
	})
	if err != nil {
		t.Fatalf("lookup manifest-v2 source: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(sources))
	}
	source := sources[0]
	if source.ProjectionIdentity != projectionID || source.SHA256 != objectSHA ||
		!source.TsStart.Equal(start) || !source.TsEnd.Equal(end) ||
		source.OffsetStart == nil || *source.OffsetStart != 0 ||
		source.OffsetEnd == nil || *source.OffsetEnd != 80 {
		t.Fatalf("manifest-v2 source drifted after ClickHouse scan: %#v", source)
	}

	_, err = client.LookupRestorationSources(ctx, RestorationSourceQuery{
		TenantID: tenantID, ProbeID: probeID, CommunityID: "1:context", FlowID: "flow-context",
		StartTime: end.Add(time.Second), EndTime: end.Add(2 * time.Second), Limit: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "no immutable manifest-v2") {
		t.Fatalf("non-overlapping source query error = %v", err)
	}
	if err := client.VerifyRestorationSchema(ctx); err == nil {
		t.Fatal("single-node non-replicated test table passed production restoration schema attestation")
	} else if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("schema attestation timed out instead of rejecting topology: %v", err)
	}
}
