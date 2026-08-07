package repository

import (
	"reflect"
	"strings"
	"testing"
)

func TestAppendTaskFiltersIncludesSourceContext(t *testing.T) {
	query, args, next := appendTaskFilters("SELECT 1 WHERE true", nil, 1, TaskListFilter{
		AlertID: "AL-1", CampaignID: "CP-1", BaselineID: "BL-1", EvidenceID: "EV-1", EvidenceType: "PCAP",
	})
	for _, fragment := range []string{"params->>'alert_id'", "params->>'campaign_id'", "params->>'baseline_id'", "params->>'evidence_id'", "lower(params->>'evidence_type')"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query %q missing %q", query, fragment)
		}
	}
	wantArgs := []interface{}{"AL-1", "CP-1", "BL-1", "EV-1", "PCAP"}
	if !reflect.DeepEqual(args, wantArgs) || next != 6 {
		t.Fatalf("args=%v next=%d, want args=%v next=6", args, next, wantArgs)
	}
}
