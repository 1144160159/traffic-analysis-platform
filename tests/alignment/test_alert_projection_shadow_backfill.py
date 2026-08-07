#!/usr/bin/env python3

from __future__ import annotations

import copy
import hashlib
import json
import sys
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from render_alert_projection_shadow_approval import (  # noqa: E402
    build_review_package,
    shadow_binding_sha256,
)


HEAD = "a" * 40
CONTENT_SHA = "b" * 64
FILE_SHA = "c" * 64


def contract() -> dict:
    return json.loads((ROOT / "contracts/opensearch/projection-shadow-backfill.v1.json").read_text(encoding="utf-8"))


def shadow(now: datetime) -> dict:
    binding = {
        "environment_id": "candidate-a",
        "tenant_id": "tenant-a",
        "start_time": "2026-08-07T10:00:00Z",
        "end_time": "2026-08-07T11:00:00Z",
        "max_documents": 100,
        "target": {
            "cluster_uuid": "cluster-a",
            "read_target": "alerts",
            "write_alias": "alerts-v2-write",
            "write_indices": [{"index": "alerts-v2-000001", "is_write_index": True}],
        },
        "source_count": 2,
        "target_count": 2,
        "differences": [
            {"alert_id": "alert-a", "classification": "missing", "source_sha256": "d" * 64},
            {"alert_id": "alert-b", "classification": "stale", "source_sha256": "e" * 64, "target_sha256": "f" * 64},
            {"alert_id": "alert-c", "classification": "extra", "target_sha256": "1" * 64},
        ],
    }
    return {
        "schema_version": 1,
        "remediation_id": "T-OS-004",
        "mode": "READ_ONLY_SHADOW",
        "status": "DIFF",
        "approval_readiness": "READY_FOR_BOUNDED_REPAIR_REVIEW",
        "captured_at": (now - timedelta(minutes=2)).isoformat(),
        "requested_by": "operator-a",
        "trace_id": "trace-a",
        "binding_sha256": shadow_binding_sha256(binding),
        "binding": binding,
        "missing_count": 1,
        "stale_count": 1,
        "extra_count": 1,
        "source_truncated": False,
        "target_truncated": False,
        "blockers": [],
        "warnings": ["extra OpenSearch documents require manual adjudication and must never be auto-deleted"],
        "read_only_operations": ["clickhouse_projection_select", "opensearch_projection_search"],
        "production_applied": False,
        "production_mutations": [],
    }


def g0() -> dict:
    candidate = {"head": HEAD, "status": []}
    return {
        "schema_version": 1,
        "run_id": "g0-test",
        "gate": "G0",
        "status": "PASS",
        "candidate_before": copy.deepcopy(candidate),
        "candidate_after": copy.deepcopy(candidate),
        "candidate_source": {"content_sha256": CONTENT_SHA},
    }


class AlertProjectionShadowBackfillTests(unittest.TestCase):
    def setUp(self) -> None:
        self.now = datetime(2026, 8, 8, 0, 0, tzinfo=timezone.utc)

    def render(self, shadow_value: dict | None = None, g0_value: dict | None = None, **kwargs) -> dict:
        return build_review_package(
            shadow=shadow_value or shadow(self.now), shadow_file_sha256=FILE_SHA,
            g0=g0_value or g0(), g0_manifest_sha256="2" * 64,
            contract=contract(), repository_head=HEAD, now=self.now, **kwargs,
        )

    def test_valid_shadow_renders_review_only_package(self) -> None:
        package = self.render()
        self.assertFalse(package["execution_authorized"])
        self.assertFalse(package["production_applied"])
        self.assertEqual(package["production_mutations"], [])
        self.assertEqual(package["bindings"]["repair_ids"], ["alert-a", "alert-b"])
        self.assertEqual(package["bindings"]["write_index"], "alerts-v2-000001")
        self.assertEqual(package["bindings"]["g0_candidate_content_sha256"], CONTENT_SHA)
        self.assertIsInstance(package["proposed_execution"]["argv"], list)
        self.assertIsNone(package["proposed_execution"]["shell"])
        argv = package["proposed_execution"]["argv"]
        self.assertEqual(argv[argv.index("--alert-ids") + 1], "alert-a,alert-b")
        self.assertEqual(argv[argv.index("--expected-cluster-uuid") + 1], "cluster-a")
        self.assertEqual(argv[argv.index("--expected-read-target") + 1], "alerts")
        self.assertEqual(argv[argv.index("--expected-write-index") + 1], "alerts-v2-000001")
        self.assertTrue(all(item["status"] == "PENDING" for item in package["approvals"].values()))
        self.assertIn("immutable alert-projection-tools image digest is not bound", package["blockers"])

    def test_digest_bound_image_still_does_not_authorize_execution(self) -> None:
        package = self.render(immutable_tool_image_digest="sha256:" + "3" * 64)
        self.assertFalse(package["execution_authorized"])
        self.assertEqual(len(package["blockers"]), 1)
        self.assertEqual(package["bindings"]["immutable_tool_image_digest"], "sha256:" + "3" * 64)

    def test_rejects_expired_tampered_or_mutating_shadow(self) -> None:
        cases: dict[str, callable] = {
            "expired": lambda value: value.update(captured_at=(self.now - timedelta(minutes=16)).isoformat()),
            "tampered": lambda value: value["binding"].update(tenant_id="tenant-b"),
            "mutating": lambda value: value.update(production_mutations=["opensearch_bulk"]),
            "truncated": lambda value: value.update(source_truncated=True),
            "not ready": lambda value: value.update(approval_readiness="BLOCKED"),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                value = shadow(self.now)
                mutate(value)
                with self.assertRaises(ValueError):
                    self.render(shadow_value=value)

    def test_rejects_candidate_drift_or_dirty_g0(self) -> None:
        drifted = g0()
        drifted["candidate_after"]["head"] = "d" * 40
        with self.assertRaises(ValueError):
            self.render(g0_value=drifted)
        dirty = g0()
        dirty["candidate_after"]["status"] = [" M unsafe"]
        with self.assertRaises(ValueError):
            self.render(g0_value=dirty)

    def test_rejects_alias_ambiguity_and_count_identity_mismatch(self) -> None:
        ambiguous = shadow(self.now)
        ambiguous["binding"]["target"]["write_indices"].append({"index": "alerts-v2-000002", "is_write_index": True})
        ambiguous["binding_sha256"] = shadow_binding_sha256(ambiguous["binding"])
        with self.assertRaises(ValueError):
            self.render(shadow_value=ambiguous)
        mismatch = shadow(self.now)
        mismatch["missing_count"] = 2
        with self.assertRaises(ValueError):
            self.render(shadow_value=mismatch)

    def test_shadow_cli_has_no_reconcile_store_or_mutating_calls(self) -> None:
        cli = (ROOT / "go/control-plane/cmd/alert-projection-shadow/main.go").read_text(encoding="utf-8")
        for forbidden in (
            '"database/sql"', "NewProjectionDebtStore", "StartProjectionReconcileRun",
            "WriteAlert(", "RefreshProjectionTarget(", "_reindex", "_bulk", "UpdateAliases",
        ):
            self.assertNotIn(forbidden, cli)
        for required in (
            '"start"', '"end"', '"tenant"', '"environment-id"', '"target-write-alias"',
            "BuildShadowManifest", "ProjectionMetadata", "production_mutations=0",
        ):
            self.assertIn(required, cli)

    def test_historical_gap_is_explicitly_non_executable(self) -> None:
        value = contract()["historical_gap"]
        self.assertEqual(value["delta"], 883172)
        self.assertFalse(value["reference_manifest_present_in_candidate"])
        self.assertIn("not a current executable scope", value["interpretation"])


if __name__ == "__main__":
    unittest.main()
