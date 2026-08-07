import copy
import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from cross_store_reconcile import ReconcileInputError, reconcile  # noqa: E402


TRACE_A = "0123456789abcdef0123456789abcdef"
TRACE_B = "abcdef0123456789abcdef0123456789"
HASH_A = "a" * 64
HASH_B = "b" * 64


def source(name: str, records: list[dict]) -> dict:
    return {
        "source": name,
        "watermark": {
            "position_kind": "aggregate_version",
            "position": "7",
            "observed_at": "2026-08-04T13:00:00Z",
            "trace_id": TRACE_A,
            "state": "complete",
        },
        "records": records,
    }


def record(record_id: str, version: int = 7, sha256: str = HASH_A, trace_id: str = TRACE_A) -> dict:
    return {"record_id": record_id, "version": version, "sha256": sha256, "trace_id": trace_id}


def base_payload() -> dict:
    return {
        "schema_version": 1,
        "tenant_id": "tenant-a",
        "data_domain": "alerts",
        "authoritative_source": "postgresql",
        "sources": [
            source("postgresql", [record("alert-a"), record("alert-b")]),
            source("kafka", [record("alert-a"), record("alert-b")]),
            source("clickhouse", [record("alert-a"), record("alert-b")]),
            source("opensearch", [record("alert-a"), record("alert-b")]),
            source("nebulagraph", [record("alert-a"), record("alert-b")]),
            source("minio", [record("alert-a"), record("alert-b")]),
            source("audit", [record("alert-a"), record("alert-b")]),
        ],
    }


class CrossStoreReconcileTest(unittest.TestCase):
    def test_equal_seven_source_manifest_including_audit_passes_with_stable_hash(self) -> None:
        payload = base_payload()
        first = reconcile(payload)
        second = reconcile(copy.deepcopy(payload))
        self.assertEqual("PASS", first["status"])
        self.assertFalse(first["partial"])
        self.assertEqual(first["report_sha256"], second["report_sha256"])
        self.assertEqual(0, sum(first["counts"].values()))
        self.assertEqual(sorted(first["source_watermarks"]), ["audit", "clickhouse", "kafka", "minio", "nebulagraph", "opensearch", "postgresql"])

    def test_all_required_difference_classes_are_explicit_and_plan_only(self) -> None:
        payload = base_payload()
        payload["sources"][1]["records"] = [
            record("alert-a", version=6),
            record("alert-b", sha256=HASH_B),
            record("alert-extra"),
        ]
        payload["sources"][2]["records"] = [record("alert-a"), record("alert-b", trace_id=TRACE_B)]
        payload["sources"][3]["records"] = [record("alert-a"), {"record_id": "broken", "version": -1}]
        result = reconcile(payload)
        self.assertEqual("PARTIAL", result["status"])
        self.assertEqual(1, result["counts"]["missing"])
        self.assertEqual(1, result["counts"]["extra"])
        self.assertEqual(1, result["counts"]["stale_version"])
        self.assertEqual(1, result["counts"]["hash_mismatch"])
        self.assertEqual(1, result["counts"]["trace_mismatch"])
        self.assertEqual(1, result["counts"]["unparseable"])
        self.assertEqual("plan_only", result["repair_plan"]["mode"])
        self.assertFalse(result["repair_plan"]["automatic_execution"])
        self.assertTrue(all(item["action"] == "quarantine_review_no_delete" for item in result["repair_plan"]["quarantine_review"]))

    def test_wildcard_tenant_and_domain_are_rejected(self) -> None:
        for key in ("tenant_id", "data_domain"):
            payload = base_payload()
            payload[key] = "*"
            with self.assertRaises(ReconcileInputError):
                reconcile(payload)

    def test_input_bound_is_fail_closed(self) -> None:
        with self.assertRaisesRegex(ReconcileInputError, "exceeding max_records"):
            reconcile(base_payload(), max_records=5)

    def test_malformed_watermark_is_not_silently_treated_as_complete(self) -> None:
        payload = base_payload()
        del payload["sources"][1]["watermark"]["position"]
        result = reconcile(payload)
        self.assertEqual("PARTIAL", result["status"])
        self.assertEqual(1, result["counts"]["unparseable"])
        self.assertEqual(["unparseable_input"], result["repair_plan"]["stop_reasons"])

    def test_output_is_json_serializable(self) -> None:
        json.dumps(reconcile(base_payload()), sort_keys=True)


if __name__ == "__main__":
    unittest.main()
