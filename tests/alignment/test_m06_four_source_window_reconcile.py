from __future__ import annotations

import copy
import importlib.util
import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/reconcile_m06_four_source_window.py"
SPEC = importlib.util.spec_from_file_location("reconcile_m06_four_source_window", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)
CONTRACT = json.loads(MODULE.CONTRACT_PATH.read_text(encoding="utf-8"))


def valid_manifest() -> dict:
    candidate = "a" * 64
    profile = "m06-real-four-source"
    environment = "k8s-staging"
    tenant = "tenant-real"
    start, end = 1_700_000_000_000, 1_700_000_060_000
    manifest = {
        "schema_version": 1,
        "artifact_kind": "M06_FOUR_SOURCE_WINDOW_MANIFEST",
        "candidate_id": candidate,
        "profile_id": profile,
        "environment_id": environment,
        "tenant_id": tenant,
        "window": {"start_ms": start, "end_ms": end},
        "producer_rail_acceptance": {},
        "rails": {},
        "production_applied": True,
    }
    for rail_id in CONTRACT["required_producer_rail_acceptance"]:
        manifest["producer_rail_acceptance"][rail_id] = {
            "candidate_id": candidate,
            "profile_id": profile,
            "environment_id": environment,
            "status": "PASS",
            "receipt_sha256": "b" * 64,
        }
    for index, rail in enumerate(CONTRACT["required_rails"]):
        topic = CONTRACT["canonical_topics"][rail]
        trace = f"{index + 1:032x}"
        payload_hash = f"{index + 10:064x}"
        key_hash = f"{index + 20:064x}"
        offset = 40 + index
        identity = {
            "topic": topic,
            "partition": index,
            "offset": offset,
            "key_sha256": key_hash,
            "payload_sha256": payload_hash,
            "trace_id": trace,
        }
        targets = []
        for target_name in CONTRACT["required_targets"][rail]:
            targets.append({
                **identity,
                "target": target_name,
                "status": "applied",
                "source_version": offset + 1,
                "projection_hash": f"{index + 30:064x}",
            })
        manifest["rails"][rail] = {
            "scope": {
                "candidate_id": candidate,
                "profile_id": profile,
                "environment_id": environment,
                "tenant_id": tenant,
                "window_start_ms": start,
                "window_end_ms": end,
            },
            "source_authority": {
                "kind": CONTRACT["required_real_source_kinds"][rail],
                "authority_id": f"real-{rail}-source",
                "real_source": True,
                "fixture": False,
                "postgres_seed": False,
            },
            "raw_input": {
                "receipt_id": f"raw-{rail}",
                "payload_sha256": payload_hash,
                "trace_id": trace,
                "event_time_ms": start + 1_000 + index,
            },
            "producer_receipt": {**identity, "state": "broker_acked", "broker_ack": True},
            "consumer_receipt": {**identity, "status": "accepted", "source_version": offset + 1, "watermark_ms": start + 5_000},
            "max_accepted_event_time_ms": start + 1_000 + index,
            "quality_counts": {
                "accepted": 1,
                "rejected": 0,
                "invalid": 0,
                "late": 0,
                "duplicate": 0,
                "conflict": 0,
                "missing": 0,
            },
            "source_record_count": 1,
            "target_receipts": targets,
        }
    return manifest


class M06FourSourceWindowReconcileTest(unittest.TestCase):
    def assertFailsWith(self, manifest: dict, code: str, rail: str | None = None) -> None:
        result = MODULE.reconcile(manifest, CONTRACT)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any(
            item["code"] == code and (rail is None or item["rail"] == rail)
            for item in result["differences"]
        ), result["differences"])

    def test_valid_real_four_source_window_reconciles(self) -> None:
        result = MODULE.reconcile(valid_manifest(), CONTRACT)
        self.assertEqual("PASS", result["status"])
        self.assertEqual(set(CONTRACT["required_rails"]), set(result["rail_results"]))
        self.assertTrue(all(item["status"] == "PASS" for item in result["rail_results"].values()))
        self.assertTrue(result["production_applied"])
        self.assertFalse(result["automatic_repair"])

    def test_fixture_and_database_seed_never_count_as_real_source(self) -> None:
        manifest = valid_manifest()
        manifest["rails"]["device_log"]["source_authority"]["fixture"] = True
        manifest["rails"]["device_log"]["source_authority"]["postgres_seed"] = True
        self.assertFailsWith(manifest, "SYNTHETIC_SOURCE_FORBIDDEN", "device_log")

    def test_consumer_only_source_is_visible_failure(self) -> None:
        manifest = valid_manifest()
        manifest["rails"]["device_log"]["producer_receipt"]["state"] = "consumer_only"
        manifest["rails"]["device_log"]["producer_receipt"]["broker_ack"] = False
        self.assertFailsWith(manifest, "PRODUCER_NOT_ACKED", "device_log")

    def test_payload_offset_and_source_version_must_close(self) -> None:
        manifest = valid_manifest()
        manifest["rails"]["asset"]["consumer_receipt"]["payload_sha256"] = "f" * 64
        manifest["rails"]["asset"]["consumer_receipt"]["source_version"] = 999
        self.assertFailsWith(manifest, "CONSUMER_RECEIPT_MISMATCH", "asset")
        self.assertFailsWith(manifest, "SOURCE_VERSION", "asset")

    def test_watermark_and_target_exact_set_fail_closed(self) -> None:
        manifest = valid_manifest()
        manifest["rails"]["user_behavior"]["consumer_receipt"]["watermark_ms"] = 1
        manifest["rails"]["asset"]["target_receipts"] = manifest["rails"]["asset"]["target_receipts"][:1]
        self.assertFailsWith(manifest, "WATERMARK", "user_behavior")
        self.assertFailsWith(manifest, "TARGET_EXACT_SET", "asset")

    def test_all_three_new_producer_rail_receipts_are_mandatory(self) -> None:
        manifest = valid_manifest()
        del manifest["producer_rail_acceptance"]["device-logs"]
        self.assertFailsWith(manifest, "PRODUCER_RAIL_EXACT_SET", "run")
        self.assertFailsWith(manifest, "PRODUCER_RAIL_RECEIPT", "run")

    def test_cross_candidate_scope_never_aggregates(self) -> None:
        manifest = valid_manifest()
        manifest["rails"]["flow"]["scope"]["candidate_id"] = "f" * 64
        self.assertFailsWith(manifest, "SCOPE_MISMATCH", "flow")

    def test_dry_run_never_becomes_real_window_acceptance(self) -> None:
        manifest = valid_manifest()
        manifest["production_applied"] = False
        self.assertFailsWith(manifest, "PRODUCTION_APPLIED_REQUIRED", "run")


if __name__ == "__main__":
    unittest.main()
