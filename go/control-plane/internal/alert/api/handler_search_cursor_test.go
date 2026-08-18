package api

import (
	"context"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
)

func TestAlertSearchContractMetaPreservesSnapshotDegradationEvidence(t *testing.T) {
	result := &service.SearchResult{
		SnapshotID:      "alert-snapshot-1",
		AsOf:            "2026-08-15T09:00:00Z",
		Partial:         true,
		MissingSections: []string{"postgresql.alerts.projection_receipts"},
		SourceWatermarks: map[string]string{
			"clickhouse.alerts.source_version": "1786784400000",
			"opensearch.alerts.search":         "2026-08-15T09:00:00Z",
		},
	}
	meta := alertSearchContractMeta(context.Background(), result)
	if !meta.Partial || meta.SnapshotID != result.SnapshotID || meta.AsOf != result.AsOf {
		t.Fatalf("meta = %#v", meta)
	}
	if len(meta.MissingSections) != 1 || meta.MissingSections[0] != result.MissingSections[0] {
		t.Fatalf("missing sections = %#v", meta.MissingSections)
	}
	if meta.SourceWatermarks["clickhouse.alerts.source_version"] != "1786784400000" {
		t.Fatalf("source watermarks = %#v", meta.SourceWatermarks)
	}
}

func TestValidateSearchAlertsRequestAcceptsLegacyAndCursorContracts(t *testing.T) {
	for _, request := range []*SearchAlertsRequest{
		{From: 999, Size: 1, SortField: "last_seen", SortOrder: "desc"},
		{Size: 200, CursorMode: "live", Query: "malware"},
		{Size: 0, CursorMode: "pit", Severity: []string{"high", "critical"}},
		{Size: 0, Cursor: "signed-token", SortField: "alert_id", SortOrder: "asc"},
	} {
		if err := validateSearchAlertsRequest(request); err != nil {
			t.Fatalf("valid request %+v rejected: %v", request, err)
		}
	}
}

func TestValidateSearchAlertsRequestRejectsUnboundedOrAmbiguousInput(t *testing.T) {
	tooMany := make([]string, 21)
	for index := range tooMany {
		tooMany[index] = "value"
	}
	for _, testCase := range []struct {
		name    string
		request *SearchAlertsRequest
	}{
		{"long query", &SearchAlertsRequest{Query: strings.Repeat("界", 257)}},
		{"negative from", &SearchAlertsRequest{From: -1}},
		{"legacy oversized page", &SearchAlertsRequest{Size: 1001}},
		{"cursor oversized page", &SearchAlertsRequest{Size: 201, CursorMode: "live"}},
		{"cursor with offset", &SearchAlertsRequest{From: 1, Size: 20, CursorMode: "pit"}},
		{"unknown cursor mode", &SearchAlertsRequest{CursorMode: "snapshot"}},
		{"unknown sort", &SearchAlertsRequest{SortField: "script_field"}},
		{"unknown order", &SearchAlertsRequest{SortOrder: "random"}},
		{"reverse window", &SearchAlertsRequest{StartTime: 2, EndTime: 1}},
		{"too many terms", &SearchAlertsRequest{Severity: tooMany}},
		{"long filter value", &SearchAlertsRequest{Status: []string{strings.Repeat("s", 129)}}},
		{"oversized cursor", &SearchAlertsRequest{Cursor: strings.Repeat("x", 8193)}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateSearchAlertsRequest(testCase.request); err == nil {
				t.Fatalf("request unexpectedly accepted: %+v", testCase.request)
			}
		})
	}
}
