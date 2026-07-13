package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCampaignQueryFiltersBuildBoundedParameterizedWhere(t *testing.T) {
	request := httptest.NewRequest("GET", "/campaigns?campaign_type=apt&risk=medium&status=investigating&phase=lateral_movement&keyword=RedLync", nil)
	filters, err := campaignQueryFiltersFromRequest(request)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	where, args := buildCampaignWhere("tenant-a", filters, 10, 20)

	for _, fragment := range []string{
		"tenant_id=?", "campaign_type=?", "score>=0.5 AND score<0.8",
		"INTERVAL 24 HOUR", "INTERVAL 7 DAY", "has(attack_phases, ?)",
		"positionCaseInsensitiveUTF8(campaign_id, ?)", "ts_start>=?", "ts_end<=?",
	} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where clause missing %q: %s", fragment, where)
		}
	}
	wantArgs := []interface{}{"tenant-a", "apt", "lateral_movement", "RedLync", "RedLync", int64(10), int64(20)}
	if len(args) != len(wantArgs) {
		t.Fatalf("args=%v want=%v", args, wantArgs)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("args[%d]=%v want=%v", index, args[index], wantArgs[index])
		}
	}
}

func TestCampaignQueryFiltersRejectInvalidValues(t *testing.T) {
	for _, rawQuery := range []string{
		"risk=critical", "status=paused", "phase=drop%20table", "keyword=" + strings.Repeat("x", 129),
	} {
		request := httptest.NewRequest("GET", "/campaigns?"+rawQuery, nil)
		if _, err := campaignQueryFiltersFromRequest(request); err == nil {
			t.Fatalf("query %q should be rejected", rawQuery)
		}
	}
}
