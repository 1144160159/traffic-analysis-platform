import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_data_quality_control_plane import (  # noqa: E402
    COMMON_SCHEMA,
    CAPTURE,
    CONTRACT,
    DATASET_SIGNAL_CONTRACT,
    DOCKER_SCHEMA,
    FEATURE_CONTRACT,
    GOVERNANCE_MIGRATION,
    GOVERNANCE,
    GOVERNANCE_HANDLER,
    GOVERNANCE_HTTP_TEST,
    GOVERNANCE_TEST,
    GOVERNANCE_CAPTURE,
    EVALUATION_MIGRATION,
    EVALUATION,
    EVALUATION_TEST,
    EXPAND_RENDERER,
    REPAIR_MIGRATION,
    REPLAY_PROJECTION_MIGRATION,
    REPAIR,
    REPAIR_TEST,
    REPAIR_EVIDENCE,
    REPAIR_EVIDENCE_TEST,
    REPAIR_EXECUTOR,
    REPAIR_REPLAY_DRIVER,
    REPAIR_REPLAY_DRIVER_TEST,
    REPAIR_PROJECTION_CONSUMER,
    REPAIR_PROJECTION_CONSUMER_TEST,
    HANDLER,
    HANDOFF_REPOSITORY,
    HANDOFF_SIGNALS,
    HANDOFF_TEST,
    K8S_SCHEMA,
    MAIN,
    MIGRATION,
    MONITOR,
    MONITOR_PERSISTENCE_TEST,
    OPENAPI,
    RUNBOOK,
    SCHEMA_CAPTURE,
    ALERT_DEPLOYMENT,
    verify,
)
from capture_data_quality_control_plane import (  # noqa: E402
    EXPECTED_MIGRATIONS,
    EXPECTED_TABLES,
    evidence_path,
    finite_flink_watermarks,
    postgres_schema_query,
    scan_evidence_secrets,
    select_flink_watermark_metric_ids,
)
from sync_data_quality_postgres_entrypoints import (  # noqa: E402
    BEGIN_MARKER,
    END_MARKER,
    check as check_legacy_entrypoint_mirrors,
)


FILES = [
    CONTRACT,
    DATASET_SIGNAL_CONTRACT,
    FEATURE_CONTRACT,
    OPENAPI,
    MIGRATION,
    GOVERNANCE_MIGRATION,
    EVALUATION_MIGRATION,
    REPAIR_MIGRATION,
    REPLAY_PROJECTION_MIGRATION,
    GOVERNANCE,
    EVALUATION,
    EVALUATION_TEST,
    REPAIR,
    REPAIR_TEST,
    REPAIR_EVIDENCE,
    REPAIR_EVIDENCE_TEST,
    REPAIR_EXECUTOR,
    REPAIR_REPLAY_DRIVER,
    REPAIR_REPLAY_DRIVER_TEST,
    REPAIR_PROJECTION_CONSUMER,
    REPAIR_PROJECTION_CONSUMER_TEST,
    GOVERNANCE_HANDLER,
    GOVERNANCE_HTTP_TEST,
    GOVERNANCE_TEST,
    GOVERNANCE_CAPTURE,
    MONITOR,
    MONITOR_PERSISTENCE_TEST,
    HANDOFF_SIGNALS,
    HANDOFF_REPOSITORY,
    HANDOFF_TEST,
    HANDLER,
    MAIN,
    K8S_SCHEMA,
    ALERT_DEPLOYMENT,
    COMMON_SCHEMA,
    DOCKER_SCHEMA,
    RUNBOOK,
    CAPTURE,
    SCHEMA_CAPTURE,
    EXPAND_RENDERER,
]


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in FILES:
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class DataQualityControlPlaneTest(unittest.TestCase):
    def test_live_capture_requires_all_migrations_and_governance_tables(self) -> None:
        query = postgres_schema_query()
        self.assertEqual({"202608041400", "202608041500", "202608041600", "202608041700", "202608041800"}, set(EXPECTED_MIGRATIONS))
        self.assertEqual(15, len(EXPECTED_TABLES))
        for version in EXPECTED_MIGRATIONS:
            self.assertIn(version, query)
        for table in {
            "data_quality_dataset_history",
            "data_quality_rule_history",
            "data_quality_command_requests",
            "data_quality_rule_evaluations",
            "data_quality_repair_history",
            "data_quality_repair_requests",
            "data_quality_flow_replay_projection",
            "data_quality_replay_projection_receipts",
        }:
            self.assertIn(table, EXPECTED_TABLES)

    def test_evidence_secret_scan_detects_assignments_without_reporting_values(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "safe.log").write_text("CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}\n", encoding="utf-8")
            (root / "unsafe.log").write_text("KAFKA_CLIENT_PASSWORD=do-not-report-this\n", encoding="utf-8")
            self.assertEqual(["unsafe.log"], scan_evidence_secrets(root))

    def test_flink_watermark_capture_uses_minimum_finite_subtask_value(self) -> None:
        values, watermark = finite_flink_watermarks(
            [
                {"id": "0.currentOutputWatermark", "value": "1785810005000"},
                {"id": "1.currentOutputWatermark", "value": "-9223372036854775808"},
                {"id": "2.currentOutputWatermark", "value": "1785810004000"},
                {"id": "3.currentOutputWatermark", "value": "not-a-number"},
            ]
        )
        self.assertEqual(1785810004000, watermark)
        self.assertEqual([0, 2], [int(item["metric_id"].split(".")[0]) for item in values])

    def test_flink_watermark_capture_keeps_all_idle_or_invalid_values_unknown(self) -> None:
        values, watermark = finite_flink_watermarks(
            [{"id": "0.currentOutputWatermark", "value": "-9223372036854775808"}]
        )
        self.assertEqual([], values)
        self.assertIsNone(watermark)

    def test_flink_watermark_metric_selection_excludes_other_operators(self) -> None:
        self.assertEqual(
            [
                "0.Assign_FlowEvent_Watermarks.currentOutputWatermark",
                "11.Assign_FlowEvent_Watermarks.currentOutputWatermark",
            ],
            select_flink_watermark_metric_ids(
                [
                    {"id": "11.Assign_FlowEvent_Watermarks.currentOutputWatermark"},
                    {"id": "0.Sink__ClickHouse_Sink_(flows_raw).currentOutputWatermark"},
                    {"id": "0.Assign_FlowEvent_Watermarks.currentOutputWatermark"},
                    {"id": "not-a-subtask.Assign_FlowEvent_Watermarks.currentOutputWatermark"},
                ]
            ),
        )

    def test_external_evidence_path_remains_absolute(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            self.assertEqual(path.resolve().as_posix(), evidence_path(path))

    def test_repository_slice_passes_without_live_or_closure_claim(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertFalse(result["production_applied"])
        self.assertEqual("F-DATAQUALITY-001", result["feature_id"])
        self.assertEqual(15, len(result["persistent_objects"]))
        self.assertIn("kafka_offset", result["real_handoff_signals"])
        self.assertTrue(all(result["legacy_entrypoints_with_control_plane"].values()))
        self.assertEqual([], check_legacy_entrypoint_mirrors(ROOT))

    def test_missing_legacy_entrypoint_mirror_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / COMMON_SCHEMA
            source = path.read_text(encoding="utf-8")
            start = source.index(BEGIN_MARKER)
            end = source.index(END_MARKER, start) + len(END_MARKER)
            path.write_text(source[:start] + source[end:].lstrip("\r\n"), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertFalse(result["legacy_entrypoints_with_control_plane"][COMMON_SCHEMA.as_posix()])
            self.assertTrue(any("missing or stale" in item for item in result["errors"]))

    def test_drifted_legacy_entrypoint_mirror_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / DOCKER_SCHEMA
            source = path.read_text(encoding="utf-8")
            path.write_text(
                source.replace("CREATE TABLE IF NOT EXISTS data_quality_rules", "CREATE TABLE data_quality_rules", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertFalse(result["legacy_entrypoints_with_control_plane"][DOCKER_SCHEMA.as_posix()])

    def test_contract_cannot_claim_closed_or_production_applied(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CONTRACT
            contract = json.loads(path.read_text(encoding="utf-8"))
            contract["status"] = "closed"
            contract["production_applied"] = True
            path.write_text(json.dumps(contract), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim closure" in item for item in result["errors"]))
            self.assertTrue(any("must not claim production apply" in item for item in result["errors"]))

    def test_missing_measurement_cannot_become_pass(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / HANDOFF_REPOSITORY
            source = path.read_text(encoding="utf-8")
            path.write_text(source.replace('Status: "unknown"', 'Status: "pass"'), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("missing token" in item and "unknown" in item for item in result["errors"]))

    def test_clickhouse_insert_rate_proxy_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / MONITOR
            path.write_text(path.read_text(encoding="utf-8") + "\n// kafka_lag_proxy\n", encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("insert-rate proxy" in item for item in result["errors"]))

    def test_missing_persistent_object_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / MIGRATION
            source = path.read_text(encoding="utf-8")
            path.write_text(
                source.replace("CREATE TABLE IF NOT EXISTS data_quality_rules", "CREATE TABLE data_quality_rules", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("missing table: data_quality_rules" in item for item in result["errors"]))

    def test_kubernetes_runner_must_include_versioned_migration(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / K8S_SCHEMA
            source = path.read_text(encoding="utf-8")
            path.write_text(
                source.replace("22-data-quality-control-plane-v1.sql: |", "22-data-quality-control-plane-v1.sql.disabled: |", 1),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])

    def test_kubernetes_runner_must_include_governance_migration(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / K8S_SCHEMA
            source = path.read_text(encoding="utf-8")
            path.write_text(
                source.replace(
                    "22-data-quality-control-plane-v1.sql 23-data-quality-governance-v1.sql 24-data-quality-rule-evaluation-v1.sql 25-data-quality-repair-lifecycle-v1.sql 26-data-quality-replay-projection-v1.sql 27-dashboard-task-execution-pipeline-v1.sql 28-dashboard-task-compensation-v1.sql 29-dashboard-task-dlq-receipt-v1.sql 30-alert-response-external-executor-v1.sql 31-alert-response-dlq-receipt-v1.sql; do",
                    "22-data-quality-control-plane-v1.sql; do",
                ),
                encoding="utf-8",
            )
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("migration runner missing token" in item for item in result["errors"]))

    def test_openapi_must_bind_data_quality_operations_to_feature_contract(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / OPENAPI
            document = json.loads(path.read_text(encoding="utf-8"))
            document["paths"]["/v1/data-quality/baseline"]["post"]["x-feature-id"] = "F-ALERT-001"
            path.write_text(json.dumps(document), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must bind F-DATAQUALITY-001" in item for item in result["errors"]))

    def test_release_blocking_feature_flag_must_remain_default_off(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / FEATURE_CONTRACT
            feature = json.loads(path.read_text(encoding="utf-8"))
            feature["rollout"]["default"] = True
            path.write_text(json.dumps(feature), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("release blocking must remain default-off" in item for item in result["errors"]))

    def test_pre_rollout_capture_cannot_claim_candidate_applied(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / CAPTURE
            source = path.read_text(encoding="utf-8")
            path.write_text(source.replace('"candidate_applied": False', '"candidate_applied": True', 1), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("must not claim candidate or production apply" in item for item in result["errors"]))

    def test_expand_renderer_cannot_drop_suspension_guard(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / EXPAND_RENDERER
            source = path.read_text(encoding="utf-8")
            path.write_text(source.replace('"suspend": True', '"suspend": False', 1), encoding="utf-8")
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("expand renderer missing token" in item for item in result["errors"]))


if __name__ == "__main__":
    unittest.main()
