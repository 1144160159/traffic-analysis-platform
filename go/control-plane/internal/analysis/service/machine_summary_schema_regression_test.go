package service

import (
	"context"
	"encoding/json"
	"testing"
)

// 回归锚点(2026-08-18 根因修复):machine_summary_schema_ref 是执行哈希冻结字段。
// live 事故:装配层/解析器丢失该字段 → 同一内容两种冻结哈希(3937277f 带字段 / 2777f3ab 丢字段)
// → 按需 AUTO_DEFAULT 内联提交 409 plan binding mismatch。
// 此测试把"带字段内容必得 3937277f"钉死,防止字段再次从冻结输入中丢失。
func TestMachineSummarySchemaRefIsPartOfExecutionHash(t *testing.T) {
	spec := json.RawMessage(`{"probe_id":"probe-8-2tb-veth","interface":"ta-veth-in","byte_limit":0,"pcap_object":"s3://analysis-bench/datasets/ustc-tfc2016/malware/Zeus.pcap","pcap_sha256":"d38e6cd7e9e063afe5696fd6a605350b57cc430ac21b32acbec7af70ec45d414","packet_limit":100000}`)
	base := NormalizedAnalysisIntent{
		TenantID: "default", TaskDefinitionID: "4104a7bf-133b-4800-a27b-00dffd6f6134",
		PlanSource: "AUTO_DEFAULT", SourceKind: "PCAP_REPLAY", SourceSpec: spec,
		SelectedFeatureIDs: []string{"f1"}, FeatureSetID: "fs-v1",
		EncryptedRecognitionModelRef: "enc@v1",
		ThreatDetectorRefs:           []string{"det@v1"}, RuleRefs: []string{"rule@v1"},
		StageDAG:         json.RawMessage(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy: json.RawMessage(`{"allow_partial":false}`),
		ResourceBudget:   json.RawMessage(`{"cpu":2}`),
		CatalogRevision:  1,
	}
	c := NewPlanCompiler()
	withField := base
	withField.MachineSummarySchemaRef = "summary-v1"
	got1, err := c.Compile(context.Background(), withField)
	if err != nil {
		t.Fatal(err)
	}
	if got1.ExecutionSpecSHA256 != "3937277f4026d71ace95ea51b7a970f919064a10f56da4712b3ade55783299aa" {
		t.Fatalf("with machine_summary_schema_ref=summary-v1: %s (want live rev1 frozen 3937277f…)", got1.ExecutionSpecSHA256)
	}
	withoutField := base
	got2, err := c.Compile(context.Background(), withoutField)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ExecutionSpecSHA256 == got1.ExecutionSpecSHA256 {
		t.Fatalf("machine_summary_schema_ref must change execution hash; both=%s", got1.ExecutionSpecSHA256)
	}
	if got2.ExecutionSpecSHA256 != "2777f3ab94e32c3be0d65f8db5fbe4c02a287dba77a2af0a20c542f53de28790" {
		t.Fatalf("without field: %s (want the drift-era 2777f3ab… as pinned reference)", got2.ExecutionSpecSHA256)
	}
}

// 解析器与模板必须把 machine_summary_schema_ref 带入 intent(修复断言)。
func TestResolversCarryMachineSummarySchemaRef(t *testing.T) {
	c := NewPlanCompiler()
	tpl := &DefaultTemplate{
		TaskDefinitionID: "d1", PlanSource: "AUTO_DEFAULT", PlanRevision: 1,
		SourceKind: "PCAP_REPLAY", SourceSpec: json.RawMessage(`{"pcap_object":"s3://x/p.pcap","packet_limit":10}`),
		FeatureSetID: "fs-1", RecognitionModel: "enc@v1", DetectorRefs: []string{"det@v1"},
		RuleRefs: []string{"rule@v1"}, MachineSummarySchemaRef: "summary-v1",
		StageDAG:         json.RawMessage(`{"stages":["S1","S2","S3","S4","S5"]}`),
		CompletionPolicy: json.RawMessage(`{"allow_partial":false}`),
		ResourceBudget:   json.RawMessage(`{"cpu":2}`),
	}
	cat := CatalogSnapshot{
		Revision:          1,
		FeatureSets:       map[string][]string{"fs-1": {"f1"}},
		RecognitionModels: []string{"enc@v1"},
		ThreatDetectors:   []string{"det@v1"},
		Rules:             []string{"rule@v1"},
	}
	ctx := context.Background()

	defIntent, err := NewDefaultPlanResolver(c).Resolve(ctx, ResolveRequest{
		TenantID: "default", TaskDefinitionID: "d1", PlanSource: "AUTO_DEFAULT",
		Catalog: cat, Template: tpl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if defIntent.MachineSummarySchemaRef != "summary-v1" {
		t.Fatalf("default resolver dropped machine_summary_schema_ref: %q", defIntent.MachineSummarySchemaRef)
	}

	custIntent, err := NewCustomPlanResolver(c).Resolve(ctx, ResolveRequest{
		TenantID: "default", TaskDefinitionID: "d1", PlanSource: "MANUAL_CUSTOM",
		Catalog: cat, Template: tpl, CustomOverrides: json.RawMessage(`{"rule_refs":["rule@v1"]}`),
		CustomReleased: true, Approved: true, Actor: "op",
	})
	if err != nil {
		t.Fatal(err)
	}
	if custIntent.MachineSummarySchemaRef != "summary-v1" {
		t.Fatalf("custom resolver dropped machine_summary_schema_ref: %q", custIntent.MachineSummarySchemaRef)
	}
}
