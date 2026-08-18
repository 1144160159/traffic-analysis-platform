package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

func rowForTest() *repository.ActivePlanRow {
	return &repository.ActivePlanRow{
		PlanID:              "plan-1",
		PlanRevision:        3,
		PlanSource:          "AUTO_DEFAULT",
		SourceKind:          "PCAP_REPLAY",
		SourceSpec:          json.RawMessage(`{"pcap_object":"s3://bench/pcap/demo.pcap"}`),
		SelectedFeatureIDs:  json.RawMessage(`["pktlen_mean","pktlen_std","fwd_count"]`),
		FeatureSetID:        "fs-baseline-v1",
		RecognitionModel:    "enc@v1",
		DetectorRefs:        json.RawMessage(`["det@v1","det@v2"]`),
		RuleRefs:            json.RawMessage(`["rule@v1"]`),
		StageDAG:            json.RawMessage(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy:    json.RawMessage(`{"allow_partial":false}`),
		ResourceBudget:      json.RawMessage(`{"cpu":2}`),
		CatalogRevision:     7,
		ExecutionSpecSHA256: "spec-sha",
	}
}

func TestBuildTemplateFromPlanRowHappyPath(t *testing.T) {
	tpl, catalog, err := BuildTemplateFromPlanRow("def-1", rowForTest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.TaskDefinitionID != "def-1" || tpl.PlanSource != "AUTO_DEFAULT" || tpl.SourceKind != "PCAP_REPLAY" {
		t.Fatalf("template identity mismatch: %+v", tpl)
	}
	if tpl.FeatureSetID != "fs-baseline-v1" || tpl.RecognitionModel != "enc@v1" {
		t.Fatalf("template model fields mismatch: %+v", tpl)
	}
	if len(tpl.DetectorRefs) != 2 || tpl.DetectorRefs[1] != "det@v2" || len(tpl.RuleRefs) != 1 {
		t.Fatalf("template refs mismatch: %+v", tpl)
	}
	if catalog.Revision != 7 {
		t.Fatalf("catalog revision mismatch: %d", catalog.Revision)
	}
	fs := catalog.FeatureSets["fs-baseline-v1"]
	if len(fs) != 3 || fs[0] != "pktlen_mean" {
		t.Fatalf("catalog feature set mismatch: %+v", fs)
	}
	if len(catalog.RecognitionModels) != 1 || catalog.RecognitionModels[0] != "enc@v1" {
		t.Fatalf("catalog recognition models mismatch: %+v", catalog.RecognitionModels)
	}
	if len(catalog.ThreatDetectors) != 2 || len(catalog.Rules) != 1 {
		t.Fatalf("catalog refs mismatch: %+v", catalog)
	}
}

func TestBuildTemplateFromPlanRowCorruptFeaturesFailsClosed(t *testing.T) {
	row := rowForTest()
	row.SelectedFeatureIDs = json.RawMessage(`{"not":"a-list"}`)
	_, _, err := BuildTemplateFromPlanRow("def-1", row)
	if err == nil || !strings.Contains(err.Error(), string(contract.ErrCodePlanNotApproved)) {
		t.Fatalf("expected plan-not-approved semantic error, got %v", err)
	}
}

func TestBuildTemplateFromPlanRowCorruptDetectorsFailsClosed(t *testing.T) {
	row := rowForTest()
	row.DetectorRefs = json.RawMessage(`{"bad":true}`)
	_, _, err := BuildTemplateFromPlanRow("def-1", row)
	if err == nil || !strings.Contains(err.Error(), string(contract.ErrCodePlanNotApproved)) {
		t.Fatalf("expected plan-not-approved semantic error, got %v", err)
	}
}

func TestBuildTemplateFromPlanRowOptionalRefsAbsent(t *testing.T) {
	row := rowForTest()
	row.RecognitionModel = ""
	row.DetectorRefs = nil
	row.RuleRefs = json.RawMessage("null")
	tpl, catalog, err := BuildTemplateFromPlanRow("def-1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.RecognitionModel != "" || len(tpl.DetectorRefs) != 0 || len(tpl.RuleRefs) != 0 {
		t.Fatalf("optional refs should be empty: %+v", tpl)
	}
	if len(catalog.RecognitionModels) != 0 {
		t.Fatalf("catalog recognition models should be empty: %+v", catalog.RecognitionModels)
	}
}
