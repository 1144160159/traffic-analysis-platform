package index

import (
	"strings"
	"testing"
	"time"
)

func TestVerifiedCutSourceQueryIsProbeScopedAndParameterized(t *testing.T) {
	query := VerifiedCutSourceQuery{
		TenantID: "tenant-a", ProbeID: "probe-a", CommunityID: "1:test",
		StartTime: time.Unix(100, 0), EndTime: time.Unix(101, 0), Limit: 17,
	}
	if err := query.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	sql := verifiedCutSourceSQL(query.Limit)
	for _, fragment := range []string{
		"tenant_id = ?", "probe_id = ?", "(? = '' OR community_id = ?)",
		"manifest_version >= 2", "object_version != ''", "LIMIT 17",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("verified cut SQL lacks %q: %s", fragment, sql)
		}
	}
}

func TestVerifiedCutSourceQueryRejectsBroadOrUnboundedSelection(t *testing.T) {
	base := VerifiedCutSourceQuery{
		TenantID: "tenant-a", ProbeID: "probe-a",
		StartTime: time.Unix(100, 0), EndTime: time.Unix(101, 0), Limit: 10,
	}
	for _, candidate := range []VerifiedCutSourceQuery{
		{},
		{TenantID: base.TenantID, StartTime: base.StartTime, EndTime: base.EndTime, Limit: 10},
		{TenantID: base.TenantID, ProbeID: base.ProbeID, StartTime: base.EndTime, EndTime: base.StartTime, Limit: 10},
		{TenantID: base.TenantID, ProbeID: base.ProbeID, StartTime: base.StartTime, EndTime: base.EndTime, Limit: 1001},
	} {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("unsafe query accepted: %#v", candidate)
		}
	}
}
