import copy
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from evaluate_m03_covered_profile import (  # noqa: E402
    ProfileBlocked,
    evaluate,
    load_json,
    sha256_path,
    validate_profile,
)
from capture_m03_covered_profile import RUNTIME_QUERIES, capture  # noqa: E402
from reconcile_m03_clickhouse_events import TABLES  # noqa: E402
from build_topic1_task_registry import validate_against_schema  # noqa: E402


class FakeMetrics:
    def __init__(self, values=None):
        self.values = {query: 0.0 for query in RUNTIME_QUERIES.values()}
        self.values.update(values or {})
        self.calls = []

    def scalar(self, query, at):
        self.calls.append((query, at))
        return self.values[query]


class FakeClickHouse:
    def __init__(self, unique_events):
        self.unique_events = unique_events

    def query_json(self, sql, parameters):
        table = next(name for name in TABLES if f"traffic.{name}" in sql)
        return [{
            "physical_rows": self.unique_events[table],
            "unique_events": self.unique_events[table],
            "duplicate_rows": 0,
            "conflicting_events": 0,
            "blank_event_id_rows": 0,
        }]


class M03CoveredProfileTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m03-profile-", dir=ROOT)
        self.addCleanup(self.temp.cleanup)
        self.temp_path = Path(self.temp.name)
        corpus_source = ROOT / "tests/fixtures/pcap/m03/manifest.v1.json"
        self.corpus_path = self.temp_path / "corpus.json"
        self.corpus_path.write_bytes(corpus_source.read_bytes())
        self.base_path = self.temp_path / "base.json"
        self.base = {
            "schema_version": "1.0.0",
            "artifact_kind": "M02_CAPTURE_PROFILE",
            "profile_id": "M02-CAPTURE-TEN-GIGABIT-V1",
            "profile_status": "APPROVED",
            "candidate_commit": "1" * 40,
            "line_rate_gbps": 10,
            "capture": {"interface_selector": "0000:01:00.0", "mode": "XDP", "queue_count": 4, "frame_size": 4096, "ring_size": 4096},
            "traffic": {"generator": "trex@sha256:" + "2" * 64, "packet_sizes_bytes": [64, 128, 512, 1518], "duration_seconds": 300, "offered_rate_source": "GENERATOR_TX_COUNTER"},
            "measurement": {"points": ["GENERATOR_TX", "NIC_RX", "PROBE_CAPTURE", "KAFKA_OFFSET", "MINIO_OBJECT", "CLICKHOUSE_INDEX"], "clock_source": "ptp0", "max_counter_error_packets": 0},
            "environment": {"environment_id": "lab-10g-a", "nic_model": "Intel-E810", "driver": "ice", "cpu_model": "Xeon", "kernel": "6.8.0", "generator_identity": "trex-a"},
            "stop_thresholds": {"system_attributable_drop_packets": 0, "unexplained_difference_packets": 0, "capture_error_count": 0},
            "counter_attribution_ref": "contracts/quality/m02-capture-counter-attribution.v1.json",
            "approval": {"activity_id": "EXT-T1-M02-N015-PROFILE-APPROVAL", "required_authorities": ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"], "receipts": ["project.sig", "test.sig", "acceptance.sig"]},
        }
        self.base_path.write_text(json.dumps(self.base), encoding="utf-8")
        self.profile_path = self.temp_path / "profile.json"
        self.profile = {
            "schema_version": "1.0.0",
            "profile_id": "M03-COVERED-TEN-GIGABIT-V1",
            "profile_status": "APPROVED",
            "candidate_sha256": "3" * 64,
            "base_m02_profile": {"path": str(self.base_path.relative_to(ROOT)), "sha256": sha256_path(self.base_path)},
            "golden_corpus": {"path": str(self.corpus_path.relative_to(ROOT)), "sha256": sha256_path(self.corpus_path)},
            "scope": {"environment_id": "lab-10g-a", "tenant_id": "tenant-profile", "run_id": "m03-profile-run", "duration_seconds": 300},
            "expected_unique_events": {"flows_raw": 10, "sessions": 4, "feature_stat": 4, "feature_seq": 4, "feature_fp": 2},
            "required_protocol_categories": ["normal", "attack_structure", "ipv6", "tls", "quic", "truncated", "large_flow", "empty"],
            "thresholds": {"system_attributable_drop_packets": 0, "unexplained_difference_packets": 0, "capture_error_count": 0, "failed_checkpoints": 0, "max_final_consumer_lag": 0, "max_consumer_lag_growth": 0, "max_checkpoint_duration_ms": 30000, "max_sink_latency_p95_ms": 1000, "conflicting_event_ids": 0, "unexplained_field_differences": 0},
            "approval": {"required_authorities": ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"], "receipts": {"PROJECT_OWNER": "p.sig", "TEST_OWNER": "t.sig", "ACCEPTANCE_AUTHORITY": "a.sig"}},
        }
        self.profile_path.write_text(json.dumps(self.profile), encoding="utf-8")
        self.observation = {
            "schema_version": 1,
            "profile_sha256": sha256_path(self.profile_path),
            "candidate_sha256": "3" * 64,
            "environment_id": "lab-10g-a",
            "tenant_id": "tenant-profile",
            "run_id": "m03-profile-run",
            "started_at": "2026-08-14T01:00:00Z",
            "ended_at": "2026-08-14T01:05:00Z",
            "duration_seconds": 300,
            "capture": {"system_attributable_drop_packets": 0, "unexplained_difference_packets": 0, "capture_error_count": 0},
            "protocol_category_counts": {"normal": 1, "attack_structure": 1, "ipv6": 1, "tls": 1, "quic": 1, "truncated": 1, "large_flow": 1, "empty": 0},
            "projections": {name: {"unique_events": count, "duplicate_rows": 0, "conflicting_events": 0, "blank_event_id_rows": 0} for name, count in self.profile["expected_unique_events"].items()},
            "flink_jobs": {
                "flink-session-job": {"running_tasks": 24, "expected_tasks": 24, "completed_checkpoints": 5, "failed_checkpoints": 0, "max_checkpoint_duration_ms": 10000},
                "flink-feature-job": {"running_tasks": 18, "expected_tasks": 18, "completed_checkpoints": 5, "failed_checkpoints": 0, "max_checkpoint_duration_ms": 12000},
            },
            "consumer_lag": {"flink-session-job": {"start": 0, "end": 0}, "flink-feature-job": {"start": 0, "end": 0}},
            "sink_latency_p95_ms": {"session-clickhouse": 200.0, "feature-clickhouse": 300.0},
            "parity": {"status": "PASS", "unexplained_field_differences": 0},
        }

    def test_approved_bound_profile_passes_only_for_covered_scope(self) -> None:
        result = evaluate(self.profile, self.observation, sha256_path(self.profile_path))
        self.assertEqual("PASS_FOR_COVERED_PROFILE", result["status"], result)
        self.assertIn("not all-protocol", result["claim_boundary"])

    def test_repository_pending_base_profile_is_blocked(self) -> None:
        pending = ROOT / "contracts/quality/m02-approved-ten-gigabit-or-higher-profile.v1.json"
        profile = copy.deepcopy(self.profile)
        profile["base_m02_profile"] = {"path": str(pending.relative_to(ROOT)), "sha256": sha256_path(pending)}
        with self.assertRaisesRegex(ProfileBlocked, "BLOCK_BASE_PROFILE_NOT_APPROVED"):
            validate_profile(profile)

    def test_repository_m03_candidate_template_is_valid_but_not_executable(self) -> None:
        path = ROOT / "contracts/quality/m03-covered-ten-gigabit-profile.v1.json"
        template = load_json(path)
        validate_against_schema(
            template, ROOT / "contracts/quality/m03-covered-profile.schema.json"
        )
        self.assertEqual("PENDING_SIGNATURE", template["profile_status"])
        self.assertIsNone(template["candidate_sha256"])
        self.assertTrue(all(value is None for value in template["expected_unique_events"].values()))
        with self.assertRaisesRegex(ProfileBlocked, "BLOCK_M03_PROFILE_NOT_APPROVED"):
            validate_profile(template)

    def test_projection_shortfall_duplicate_or_conflict_fails(self) -> None:
        observation = copy.deepcopy(self.observation)
        observation["projections"]["sessions"]["unique_events"] -= 1
        observation["projections"]["feature_stat"]["conflicting_events"] = 1
        result = evaluate(self.profile, observation, sha256_path(self.profile_path))
        self.assertEqual("FAIL", result["status"])
        metrics = {item["metric"] for item in result["failures"]}
        self.assertIn("sessions.unique_events", metrics)
        self.assertIn("feature_stat.conflicting_events", metrics)

    def test_checkpoint_lag_sink_and_parity_thresholds_fail_closed(self) -> None:
        observation = copy.deepcopy(self.observation)
        observation["flink_jobs"]["flink-session-job"]["failed_checkpoints"] = 1
        observation["consumer_lag"]["flink-feature-job"]["end"] = 2
        observation["sink_latency_p95_ms"]["feature-clickhouse"] = 1001
        observation["parity"] = {"status": "FAIL", "unexplained_field_differences": 1}
        result = evaluate(self.profile, observation, sha256_path(self.profile_path))
        metrics = {item["metric"] for item in result["failures"]}
        self.assertIn("flink-session-job.failed_checkpoints", metrics)
        self.assertIn("flink-feature-job.final_lag", metrics)
        self.assertIn("feature-clickhouse.p95_ms", metrics)
        self.assertIn("online_offline_parity", metrics)

    def test_protocol_denominator_and_observation_binding_cannot_drift(self) -> None:
        profile = copy.deepcopy(self.profile)
        profile["required_protocol_categories"][-1] = "normal"
        with self.assertRaisesRegex(ProfileBlocked, "BLOCK_M03_PROFILE_SCHEMA"):
            validate_profile(profile)
        observation = copy.deepcopy(self.observation)
        observation["candidate_sha256"] = "4" * 64
        with self.assertRaisesRegex(ProfileBlocked, "BLOCK_OBSERVATION_BINDING"):
            evaluate(self.profile, observation, sha256_path(self.profile_path))

    def test_read_only_capture_composes_metrics_reconciliation_and_bound_receipts(self) -> None:
        metric_values = {
            RUNTIME_QUERIES["session_running_tasks"]: 24,
            RUNTIME_QUERIES["feature_running_tasks"]: 18,
            RUNTIME_QUERIES["session_completed_checkpoints"]: 5,
            RUNTIME_QUERIES["feature_completed_checkpoints"]: 5,
            RUNTIME_QUERIES["session_failed_checkpoints"]: 0,
            RUNTIME_QUERIES["feature_failed_checkpoints"]: 0,
            RUNTIME_QUERIES["session_checkpoint_duration_ms"]: 10000,
            RUNTIME_QUERIES["feature_checkpoint_duration_ms"]: 12000,
            RUNTIME_QUERIES["session_lag"]: 0,
            RUNTIME_QUERIES["feature_lag"]: 0,
            RUNTIME_QUERIES["session_sink_latency_p95_ms"]: 200,
            RUNTIME_QUERIES["feature_sink_latency_p95_ms"]: 300,
        }
        common = {"schema_version": 1, "candidate_sha256": "3" * 64, "tenant_id": "tenant-profile", "run_id": "m03-profile-run"}
        coverage = {**common, "protocol_category_counts": self.observation["protocol_category_counts"]}
        parity = {**common, "status": "PASS", "unexplained_field_differences": 0}
        capture_receipt = {
            **common,
            "started_at": "2026-08-14T01:00:00Z",
            "ended_at": "2026-08-14T01:05:00Z",
            "duration_seconds": 300,
            "system_attributable_drop_packets": 0,
            "unexplained_difference_packets": 0,
            "capture_error_count": 0,
        }
        observed = capture(
            self.profile,
            sha256_path(self.profile_path),
            coverage,
            parity,
            capture_receipt,
            FakeMetrics(metric_values),
            FakeClickHouse(self.profile["expected_unique_events"]),
            database="traffic",
        )
        self.assertEqual(self.observation, observed)
        self.assertEqual(
            "PASS_FOR_COVERED_PROFILE",
            evaluate(self.profile, observed, sha256_path(self.profile_path))["status"],
        )

    def test_capture_receipts_are_candidate_tenant_and_run_bound(self) -> None:
        common = {"schema_version": 1, "candidate_sha256": "3" * 64, "tenant_id": "tenant-profile", "run_id": "m03-profile-run"}
        coverage = {**common, "protocol_category_counts": self.observation["protocol_category_counts"]}
        parity = {**common, "status": "PASS", "unexplained_field_differences": 0}
        capture_receipt = {**common, "started_at": "2026-08-14T01:00:00Z", "ended_at": "2026-08-14T01:05:00Z", "duration_seconds": 300, "system_attributable_drop_packets": 0, "unexplained_difference_packets": 0, "capture_error_count": 0}
        coverage["run_id"] = "another-run"
        with self.assertRaisesRegex(ProfileBlocked, "BLOCK_COVERAGE_RECEIPT_BINDING"):
            capture(
                self.profile, sha256_path(self.profile_path), coverage, parity,
                capture_receipt, FakeMetrics(),
                FakeClickHouse(self.profile["expected_unique_events"]), database="traffic",
            )


if __name__ == "__main__":
    unittest.main()
