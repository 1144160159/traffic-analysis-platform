package index

import (
	"strings"
	"testing"
	"time"
)

func validRestorationSource() RestorationSource {
	start := uint64(0)
	end := uint64(80)
	return RestorationSource{
		TenantID: "tenant-a", ProbeID: "probe-a", ProjectionIdentity: strings.Repeat("a", 64),
		FileKey: "pcap/tenant-a/probe-a/capture.pcap", Bucket: "pcap-archive",
		ObjectVersion: "version-1", ETag: "etag-1", SHA256: strings.Repeat("b", 64),
		OriginalSize: 100, StoredSize: 80, Compression: "zstd", ManifestVersion: 2,
		CommunityID: "1:test", FlowID: "flow-a", OffsetStart: &start, OffsetEnd: &end,
		TsStart: time.Unix(100, 0), TsEnd: time.Unix(101, 0),
	}
}

func TestRestorationSourceRequiresExactTenantAndManifestV2Receipt(t *testing.T) {
	source := validRestorationSource()
	if err := source.Validate("tenant-a", "probe-a"); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*RestorationSource)
	}{
		{"tenant", func(value *RestorationSource) { value.TenantID = "tenant-b" }},
		{"community", func(value *RestorationSource) { value.CommunityID = "" }},
		{"flow", func(value *RestorationSource) { value.FlowID = "" }},
		{"version", func(value *RestorationSource) { value.ObjectVersion = "" }},
		{"sha", func(value *RestorationSource) { value.SHA256 = "unknown" }},
		{"manifest", func(value *RestorationSource) { value.ManifestVersion = 1 }},
		{"compression", func(value *RestorationSource) { value.Compression = "gzip" }},
		{"range", func(value *RestorationSource) { *value.OffsetEnd = 1 }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := validRestorationSource()
			mutation.edit(&candidate)
			if err := candidate.Validate("tenant-a", "probe-a"); err == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}

func TestRestorationQueryIsParameterizedAndRequiresManifestAuthority(t *testing.T) {
	sql := restorationSourceSQL(17)
	for _, fragment := range []string{
		"tenant_id = ?", "probe_id = ?", "community_id = ?", "flow_id = ?",
		"ts_start <= fromUnixTimestamp64Milli(?)",
		"ts_end >= fromUnixTimestamp64Milli(?)",
		"manifest_version >= 2", "object_version != ''", "projection_identity",
		"LIMIT 17",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL lacks %q: %s", fragment, sql)
		}
	}
	if strings.Contains(sql, "tenant-a") {
		t.Fatal("query interpolates tenant data")
	}
}

func TestRestorationSourceQueryRejectsBroadOrUnboundedSelection(t *testing.T) {
	valid := RestorationSourceQuery{
		TenantID: "tenant-a", ProbeID: "probe-a", CommunityID: "1:test", FlowID: "flow-a",
		StartTime: time.Unix(100, 0), EndTime: time.Unix(101, 0), Limit: 10,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for _, candidate := range []RestorationSourceQuery{
		{},
		{TenantID: "tenant-a", StartTime: valid.StartTime, EndTime: valid.EndTime, Limit: 10},
		{TenantID: "tenant-a", ProbeID: "p", CommunityID: "c", FlowID: "f", StartTime: valid.EndTime, EndTime: valid.StartTime, Limit: 10},
		{TenantID: "tenant-a", ProbeID: "p", CommunityID: "c", FlowID: "f", StartTime: valid.StartTime, EndTime: valid.EndTime, Limit: 1001},
	} {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("unsafe broad query accepted: %#v", candidate)
		}
	}
}

func validRestorationSession() (RestorationSessionQuery, RestorationSessionAuthority) {
	query := RestorationSessionQuery{
		TenantID: "tenant-a", SessionID: "session-a", CommunityID: "1:test", PrimaryFlowID: "flow-a",
		StartTime: time.Unix(101, 0), EndTime: time.Unix(109, 0),
	}
	authority := RestorationSessionAuthority{
		TenantID: query.TenantID, SessionID: query.SessionID, CommunityID: query.CommunityID,
		EventID: "session-event-a", ProbeID: "probe-a", FlowIDs: []string{"flow-a", "flow-b"},
		TsStart: time.Unix(100, 0), TsEnd: time.Unix(110, 0),
		EventSchemaVersion: "v1", AggregateVersion: 1, IdentityVersion: "session-id-sha256-v1",
		SessionVersion: 1, Completeness: "SESSION_COMPLETENESS_COMPLETE",
	}
	return query, authority
}

func TestRestorationSessionAuthorityBindsIdentityFlowTimeAndVersion(t *testing.T) {
	query, authority := validRestorationSession()
	if err := authority.Validate(query); err != nil {
		t.Fatalf("valid session authority rejected: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*RestorationSessionAuthority)
	}{
		{"tenant", func(value *RestorationSessionAuthority) { value.TenantID = "tenant-b" }},
		{"event", func(value *RestorationSessionAuthority) { value.EventID = "" }},
		{"probe", func(value *RestorationSessionAuthority) { value.ProbeID = "" }},
		{"legacy-schema", func(value *RestorationSessionAuthority) { value.EventSchemaVersion = "" }},
		{"legacy-identity", func(value *RestorationSessionAuthority) { value.IdentityVersion = "" }},
		{"missing-flow", func(value *RestorationSessionAuthority) { value.FlowIDs = []string{"flow-b"} }},
		{"narrow-window", func(value *RestorationSessionAuthority) { value.TsEnd = query.EndTime.Add(-time.Second) }},
		{"inconsistent-complete", func(value *RestorationSessionAuthority) { value.IsPartial = true }},
		{"invalid-completeness", func(value *RestorationSessionAuthority) { value.Completeness = "SESSION_COMPLETENESS_INVALID" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := authority
			candidate.FlowIDs = append([]string(nil), authority.FlowIDs...)
			mutation.edit(&candidate)
			if err := candidate.Validate(query); err == nil {
				t.Fatal("invalid session authority accepted")
			}
		})
	}
}

func TestRestorationSessionSQLSelectsLatestVersionWithoutInterpolatedIdentity(t *testing.T) {
	sql := restorationSessionSQL()
	for _, fragment := range []string{
		"FROM traffic.sessions", "tenant_id = ?", "session_id = ?", "community_id = ?",
		"argMax(flow_ids", "tuple(aggregate_version, session_version, ingest_ts, event_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("session SQL lacks %q: %s", fragment, sql)
		}
	}
	if strings.Contains(sql, "tenant-a") || strings.Contains(sql, "session-a") {
		t.Fatal("session query interpolates authority identity")
	}
}

func TestRestorationSchemaCheckRequiresVersionedSessionAndManifestV2IndexColumns(t *testing.T) {
	sql := restorationSchemaSQL()
	for _, fragment := range []string{
		"FROM system.columns", "database = currentDatabase()",
		"table IN ('sessions','pcap_index_v2','pcap_index_v2_local')",
		"identity_version", "session_version", "completeness", "is_partial",
		"projection_identity", "object_version", "manifest_version", "offset_start", "offset_end",
		"DateTime64(3, UTC)", "ReplicatedReplacingMergeTree", "cityHash64(tenant_id,file_key)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("restoration schema SQL lacks %q: %s", fragment, sql)
		}
	}
}
