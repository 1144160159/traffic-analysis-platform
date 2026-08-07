import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from build_adapter_risk_registry import build_registry, production_sources, scan_source  # noqa: E402
from verify_common_response_adapter import validate  # noqa: E402


class AdapterRiskScannerTest(unittest.TestCase):
    def finding_rules(self, source: str) -> set[str]:
        return {item["rule_id"] for item in scan_source("web/ui/src/example.ts", source)}

    def test_detects_hardcoded_private_business_address(self) -> None:
        self.assertIn("hardcoded_private_ip_literal", self.finding_rules("const source = '10.20.4.18';"))

    def test_detects_fabricated_comparison_value(self) -> None:
        self.assertIn("fabricated_comparison_literal", self.finding_rules("const delta = '较昨日 +12.6%';"))

    def test_detects_truthy_numeric_fallback(self) -> None:
        self.assertIn("truthy_numeric_fallback", self.finding_rules("const total = numberAt(summary, ['total']) || rows.length;"))

    def test_detects_runtime_mock_but_allows_erased_type_import(self) -> None:
        runtime_rules = self.finding_rules("import { mockSnapshot } from '@/services/mockData';")
        type_rules = self.finding_rules("import type { PageSnapshot } from '@/services/mockData';")
        self.assertIn("runtime_mock_dependency", runtime_rules)
        self.assertNotIn("runtime_mock_dependency", type_rules)

    def test_detects_visual_fixture_query_bypass(self) -> None:
        source = "return buildVisualBreakdownSnapshot(route.page);"
        self.assertIn("runtime_visual_fixture_bypass", self.finding_rules(source))

    def test_detects_fabricated_generic_snapshot_values_and_ids(self) -> None:
        value_rules = self.finding_rules("const value = rows.length + index * 3;")
        id_rules = self.finding_rules("const id = `${pageId.toUpperCase()}-${String(index).padStart(4, '0')}`;")
        self.assertIn("fabricated_generic_snapshot", value_rules)
        self.assertIn("fabricated_generic_snapshot", id_rules)

    def test_detects_summary_count_derived_from_display_state(self) -> None:
        source = "const count = summary.reportable_count ?? (completeness > 0 ? 1 : 0);"
        self.assertIn("derived_summary_fallback", self.finding_rules(source))
        length_source = "const count = summary.pending_evidence_count ?? evidenceRows.filter(Boolean).length;"
        self.assertIn("derived_summary_fallback", self.finding_rules(length_source))

    def test_detects_business_row_inference(self) -> None:
        source = "const sourceRows = events.length ? events : users.length ? users : protocols;"
        self.assertIn("derived_business_collection", self.finding_rules(source))

    def test_detects_default_demo_resource_id(self) -> None:
        source = "const normalizedId = alertId || 'AL-20260620-000123';"
        self.assertIn("default_demo_resource_id", self.finding_rules(source))

    def test_detects_fixed_detail_business_fallbacks(self) -> None:
        time_rules = self.finding_rules("const firstSeen = value || '2026-06-20 03:42:11';")
        identity_rules = self.finding_rules("const assignee = value || 'sec_analyst';")
        self.assertIn("fixed_detail_business_fallback", time_rules)
        self.assertIn("fixed_detail_business_fallback", identity_rules)

    def test_detects_fixed_detail_metrics_and_timeline(self) -> None:
        metric_rules = self.finding_rules("metric('处置进度', status === '已结束' ? '100%' : '68%', status, 'warn')")
        timeline_rules = self.finding_rules("timelineItem('03:43:47', 'C2 连接', '高危通信确认', 'risk')")
        self.assertIn("fixed_detail_metric_literal", metric_rules)
        self.assertIn("fixed_detail_timeline_literal", timeline_rules)

    def test_detects_fixed_detail_collection_fallback(self) -> None:
        source = "const tags = values.length ? values : ['C2通信', '横向移动', '可疑外联'];"
        self.assertIn("fixed_detail_collection_fallback", self.finding_rules(source))

    def test_detects_fixed_numeric_business_fallback_but_allows_layout_defaults(self) -> None:
        business = "const rate = optionalRatioAt(stats, ['success_rate']) ?? 99.2;"
        count = "const total = rows.length || 82;"
        layout = "width: optionalNumberFrom(item, ['width']) ?? 104,"
        self.assertIn("fixed_numeric_business_fallback", self.finding_rules(business))
        self.assertIn("fixed_numeric_business_fallback", self.finding_rules(count))
        self.assertNotIn("fixed_numeric_business_fallback", self.finding_rules(layout))

    def test_detects_runtime_business_fixture_objects(self) -> None:
        self.assertIn(
            "runtime_business_fixture_object",
            self.finding_rules("const screenVisuals = data?.visuals?.screen ?? fallbackScreenVisuals;"),
        )
        self.assertIn(
            "runtime_business_fixture_object",
            self.finding_rules("const visualTargetEgressRows = [{ name: '北美洲', value: 42.7 }];"),
        )

    def test_detects_generated_business_trends(self) -> None:
        self.assertIn("generated_business_trend", self.finding_rules("const seed = rowSeed(selected);"))
        self.assertIn(
            "generated_business_trend",
            self.finding_rules("const values = buildModelMetricTrend(selected, index);"),
        )

    def test_detects_fixed_page_metric_fallbacks(self) -> None:
        self.assertIn(
            "fixed_page_metric_fallback",
            self.finding_rules("const f1 = numericRowValue(selected, '__f1_score', 0.948);"),
        )
        self.assertIn(
            "fixed_page_metric_fallback",
            self.finding_rules("const fallbackMetric = (label: string) => ({ label, value: '0' });"),
        )

    def test_detects_fixed_screen_visual_fixtures(self) -> None:
        self.assertIn(
            "fixed_screen_visual_fixture",
            self.finding_rules("probeMapNodes: live.nodes.length ? live.nodes : ["),
        )
        self.assertIn(
            "fixed_screen_visual_fixture",
            self.finding_rules("topologyNodes: ["),
        )
        self.assertIn(
            "fixed_screen_visual_fixture",
            self.finding_rules("{ name: '初始访问簇', value: 26 },"),
        )

    def test_finding_identity_is_deterministic(self) -> None:
        source = "const total = numberAt(summary, ['total']) || rows.length;"
        first = scan_source("web/ui/src/example.ts", source)
        second = scan_source("web/ui/src/example.ts", source)
        self.assertEqual(first, second)
        self.assertEqual(len({item["finding_id"] for item in first}), len(first))


class CommonResponseAdapterRepositoryTest(unittest.TestCase):
    def test_registry_covers_all_production_pages_and_services(self) -> None:
        sources = production_sources(ROOT)
        self.assertIn("web/ui/src/pages/AlertDetailPage.tsx", sources)
        self.assertIn("web/ui/src/services/campaignDetailApi.ts", sources)
        self.assertNotIn("web/ui/src/services/mockData.ts", sources)
        self.assertFalse(any(".test." in source or ".spec." in source for source in sources))

    def test_registered_production_adapter_risks_are_zero(self) -> None:
        registry = build_registry(ROOT)
        self.assertEqual(registry["status"], "verifying")
        self.assertEqual(registry["coverage"]["open_findings"], 0)
        self.assertTrue(all(count == 0 for count in registry["coverage"]["counts_by_rule"].values()))

    def test_repository_protocol_and_ratchet_pass(self) -> None:
        result = validate(ROOT)
        self.assertEqual(result["status"], "PASS", result["errors"])
        self.assertEqual(result["repository_integrity"], "PASS")
        self.assertEqual(result["remediation_coverage"], "PARTIAL")
        self.assertEqual(result["formal_contracts"], 3)


if __name__ == "__main__":
    unittest.main()
