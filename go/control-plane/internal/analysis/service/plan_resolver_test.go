package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func testTemplate() *DefaultTemplate {
	return &DefaultTemplate{
		TaskDefinitionID: "def-1",
		PlanSource:       "AUTO_DEFAULT",
		SourceKind:       "LIVE_STREAM_WINDOW",
		SourceSpec:       json.RawMessage(`{"window_ms":60000}`),
		FeatureSetID:     "fs-v1",
		RecognitionModel: "enc-recog@v2",
		DetectorRefs:     []string{"rule-scan-detect@v3", "behavior-known@v7"},
		RuleRefs:         []string{"rule-scan-detect@v3"},
		StageDAG:         json.RawMessage(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy: json.RawMessage(`{"allow_partial":false,"zero_input_policy":"ALLOW_NO_DATA"}`),
		ResourceBudget:   json.RawMessage(`{"cpu":2,"mem_mb":4096}`),
	}
}

func testCatalog() CatalogSnapshot {
	return CatalogSnapshot{
		Revision:          12,
		FeatureSets:       map[string][]string{"fs-v1": {"pktlen_mean", "iat_mean_ms", "tcp_flag_syn_cnt"}},
		RecognitionModels: []string{"enc-recog@v2", "enc-recog@v3"},
		ThreatDetectors:   []string{"rule-scan-detect@v3", "behavior-known@v7", "behavior-extra@v1"},
		Rules:             []string{"rule-scan-detect@v3"},
		Probes:            []string{"probe-a", "probe-b"},
	}
}

func testResolveRequest() ResolveRequest {
	return ResolveRequest{
		TenantID: "tenant-a", TaskDefinitionID: "def-1",
		Catalog: testCatalog(), Template: testTemplate(),
		Actor: "analyst-1", ActorScopes: []string{"analysis:run"},
	}
}

func TestDefaultPlanResolverUsesTemplateExactSet(t *testing.T) {
	r := NewDefaultPlanResolver(NewPlanCompiler())
	intent, err := r.Resolve(context.Background(), testResolveRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if intent.PlanSource != "AUTO_DEFAULT" || len(intent.SelectedFeatureIDs) != 3 {
		t.Fatalf("unexpected intent: %+v", intent)
	}
	if intent.SelectionOrigins[0] != "default-template:fs-v1" {
		t.Fatalf("unexpected origins: %v", intent.SelectionOrigins)
	}
}

func TestCustomPlanResolverRejectsWhenNotReleased(t *testing.T) {
	r := NewCustomPlanResolver(NewPlanCompiler())
	req := testResolveRequest()
	req.PlanSource = "MANUAL_CUSTOM"
	req.CustomOverrides = json.RawMessage(`{"selected_feature_ids":["pktlen_mean"]}`)
	req.CustomReleased = false
	_, err := r.Resolve(context.Background(), req)
	if err == nil {
		t.Fatalf("expected FEATURE_NOT_RELEASED")
	}
	if !isCode(err, contract.ErrCodeFeatureNotReleased) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCustomPlanResolverFieldOverridesWithOrigins(t *testing.T) {
	r := NewCustomPlanResolver(NewPlanCompiler())
	req := testResolveRequest()
	req.CustomOverrides = json.RawMessage(`{
		"source_kind":"PCAP_REPLAY",
		"source_spec":{"object_ref":"s3://flink-checkpoints/eval/frozen-v1","start_ms":0,"end_ms":60000},
		"probe_id":"probe-a",
		"selected_feature_ids":["pktlen_mean","iat_mean_ms"],
		"encrypted_recognition_model_ref":"enc-recog@v3",
		"threat_detector_refs":["rule-scan-detect@v3"],
		"thresholds":{"severity_low":0.4}
	}`)
	req.CustomReleased = true
	req.Approved = true
	intent, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if intent.PlanSource != "MANUAL_CUSTOM" || intent.SourceKind != "PCAP_REPLAY" {
		t.Fatalf("unexpected intent: %+v", intent)
	}
	if intent.EncryptedRecognitionModelRef != "enc-recog@v3" {
		t.Fatalf("recognition override not applied")
	}
	if len(intent.ThreatDetectorRefs) != 1 || intent.ThreatDetectorRefs[0] != "rule-scan-detect@v3" {
		t.Fatalf("detector override not applied: %v", intent.ThreatDetectorRefs)
	}
	hasOrigin := false
	for _, o := range intent.SelectionOrigins {
		if strings.Contains(o, "field:encrypted_recognition_model_ref") && strings.Contains(o, "analyst-1") {
			hasOrigin = true
		}
	}
	if !hasOrigin {
		t.Fatalf("selection_origins missing actor/field records: %v", intent.SelectionOrigins)
	}
}

func TestCustomPlanResolverRejectsUnknownCatalogRefs(t *testing.T) {
	r := NewCustomPlanResolver(NewPlanCompiler())
	req := testResolveRequest()
	req.CustomOverrides = json.RawMessage(`{"threat_detector_refs":["detector-not-in-catalog@v9"]}`)
	req.CustomReleased = true
	req.Approved = true
	_, err := r.Resolve(context.Background(), req)
	if err == nil {
		t.Fatalf("expected catalog rejection")
	}
}

func TestCustomPlanResolverRequiresApproval(t *testing.T) {
	r := NewCustomPlanResolver(NewPlanCompiler())
	req := testResolveRequest()
	req.CustomOverrides = json.RawMessage(`{"selected_feature_ids":["pktlen_mean"]}`)
	req.CustomReleased = true
	req.Approved = false
	_, err := r.Resolve(context.Background(), req)
	if !isCode(err, contract.ErrCodePlanNotApproved) {
		t.Fatalf("expected approval required, got %v", err)
	}
}

func TestCustomResolverSameCompilerParity(t *testing.T) {
	// AUTO 与 MANUAL 同基线、无有效覆盖差异时,执行哈希必须一致(plan_source 不进执行哈希)
	c := NewPlanCompiler()
	def := NewDefaultPlanResolver(c)
	cus := NewCustomPlanResolver(c)
	auto, err := def.Resolve(context.Background(), testResolveRequest())
	if err != nil {
		t.Fatalf("auto: %v", err)
	}
	req := testResolveRequest()
	// custom 显式选择与模板相同的 exact-set(等价覆盖)
	req.CustomOverrides = json.RawMessage(`{"selected_feature_ids":["pktlen_mean","iat_mean_ms","tcp_flag_syn_cnt"]}`)
	req.CustomReleased = true
	req.Approved = true
	man, err := cus.Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("custom: %v", err)
	}
	a, _ := c.Compile(context.Background(), *auto)
	b, _ := c.Compile(context.Background(), *man)
	if a.ExecutionSpecSHA256 != b.ExecutionSpecSHA256 {
		t.Fatalf("AUTO/MANUAL equivalent overrides must share execution hash: %s vs %s",
			a.ExecutionSpecSHA256, b.ExecutionSpecSHA256)
	}
	if a.PlanRevisionSHA256 == b.PlanRevisionSHA256 {
		t.Fatalf("plan_revision_sha256 must differ by plan_source/origins")
	}
}

func TestMergeJSONThresholdsIntoPolicy(t *testing.T) {
	base := json.RawMessage(`{"allow_partial":false}`)
	out := mergeJSON(base, map[string]interface{}{"thresholds": json.RawMessage(`{"severity_low":0.4}`)})
	m := map[string]interface{}{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("merged json invalid: %v", err)
	}
	if m["allow_partial"] != false {
		t.Fatalf("base field lost: %+v", m)
	}
	if _, ok := m["thresholds"]; !ok {
		t.Fatalf("thresholds missing: %+v", m)
	}
}

func TestTriggerIdentityHash(t *testing.T) {
	h1 := identityHash("tenant-a", "actor", "actor-1:key-1")
	h2 := identityHash("tenant-a", "actor", "actor-1:key-1")
	if h1 != h2 {
		t.Fatalf("identity hash not deterministic")
	}
	h3 := identityHash("tenant-a", "actor", "actor-1:key-2")
	if h1 == h3 {
		t.Fatalf("different payloads must differ")
	}
}

// isCode 统一错误框架断言(errors.As AppError + 码比对)。
func isCode(err error, code commonerrors.ErrorCode) bool {
	var ae *commonerrors.AppError
	return errors.As(err, &ae) && ae.Code == code
}
