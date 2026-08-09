from __future__ import annotations

import importlib.util
import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


VERIFY = load_module("verify_opensearch_ha_security_restore", ROOT / "scripts/alignment/verify_opensearch_ha_security_restore.py")
TOOL = load_module("opensearch_snapshot_restore", ROOT / "scripts/alignment/opensearch_snapshot_restore.py")


class OpenSearchHASecurityRestoreVerifierTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        for relative in VERIFY.REQUIRED:
            source = ROOT / relative
            target = self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def assert_fails(self, fragment: str) -> None:
        result = VERIFY.verify(self.root)
        self.assertEqual("FAIL", result["status"])
        self.assertTrue(any(fragment in error for error in result["errors"]), result["errors"])

    def mutate_text(self, relative: Path, old: str, new: str) -> None:
        path = self.root / relative
        text = path.read_text(encoding="utf-8")
        self.assertIn(old, text)
        path.write_text(text.replace(old, new, 1), encoding="utf-8")

    def test_baseline_candidate_passes_static_verification(self) -> None:
        self.assertEqual("PASS", VERIFY.verify(self.root)["status"])

    def test_two_zone_contract_is_rejected(self) -> None:
        path = self.root / VERIFY.CONTRACT
        payload = json.loads(path.read_text())
        payload["failure_domains"]["required_distinct_zones"] = 2
        path.write_text(json.dumps(payload), encoding="utf-8")
        self.assert_fails("three-zone")

    def test_plaintext_http_overlay_is_rejected(self) -> None:
        self.mutate_text(VERIFY.OVERLAY, "plugins.security.ssl.http.enabled\n          value: \"true\"",
                         "plugins.security.ssl.http.enabled\n          value: \"false\"")
        self.assert_fails("rendered target did not override plaintext HTTP")

    def test_admin_health_probe_is_rejected(self) -> None:
        self.mutate_text(VERIFY.OVERLAY, "--cert /var/run/opensearch-monitor/tls.crt", "-u admin:${OPENSEARCH_INITIAL_ADMIN_PASSWORD}")
        self.assert_fails("health probes must not use the admin identity")

    def test_default_deploy_reference_is_rejected(self) -> None:
        path = self.root / VERIFY.DEFAULT_DEPLOY
        path.write_text(path.read_text() + "\n# opensearch-ha-v1\n", encoding="utf-8")
        self.assert_fails("default deploy path")

    def test_wildcard_service_role_is_rejected(self) -> None:
        path = self.root / VERIFY.ROLES
        payload = json.loads(path.read_text())
        payload["roles"][0]["index_permissions"][0]["index_patterns"] = ["*"]
        path.write_text(json.dumps(payload), encoding="utf-8")
        self.assert_fails("wildcard-index")

    def test_missing_snapshot_failure_alert_is_rejected(self) -> None:
        self.mutate_text(VERIFY.ALERTS, "alert: OpenSearchSnapshotFailure", "record: OpenSearchSnapshotFailure")
        self.assert_fails("alert rule missing")

    def test_same_cluster_restore_guard_removal_is_rejected(self) -> None:
        self.mutate_text(VERIFY.SNAPSHOT_TOOL, "same-cluster restore is forbidden", "source and target unexpectedly match")
        self.assert_fails("snapshot/restore safety guard missing")

    def test_unapproved_image_becoming_runnable_is_rejected(self) -> None:
        approved = "registry.example/traffic/opensearch@sha256:" + "1" * 64
        self.mutate_text(VERIFY.OVERLAY, VERIFY.PLACEHOLDER, approved)
        self.assert_fails("fail-safe image guard")

    def test_renderer_without_scoped_load_override_is_rejected(self) -> None:
        self.mutate_text(VERIFY.RENDERER, "LoadRestrictionsNone", "LoadRestrictionsRootOnly")
        self.assert_fails("guarded renderer")

    def test_empty_outage_semantics_are_rejected(self) -> None:
        path = self.root / VERIFY.CONTRACT
        payload = json.loads(path.read_text())
        payload["client_degradation"]["unavailable_must_not_render_as_empty"] = False
        path.write_text(json.dumps(payload), encoding="utf-8")
        self.assert_fails("browser degradation")


class SnapshotRestoreGuardTest(unittest.TestCase):
    def manifest(self, directory: Path) -> Path:
        required = sorted(TOOL.REQUIRED_VERIFICATION)
        payload = {
            "approval_status": "APPROVED",
            "operation": "opensearch_isolated_restore",
            "approval_id": "CHG-1",
            "approved_by": "reviewer",
            "expires_at": "2099-01-01T00:00:00Z",
            "target_isolated": True,
            "source_endpoint_sha256": "a" * 64,
            "target_endpoint_sha256": "b" * 64,
            "source_cluster_uuid": "source-uuid",
            "target_cluster_uuid": "target-uuid",
            "repository": "traffic-s3",
            "snapshot": "alerts-20260804",
            "indices": ["alerts-v2"],
            "rename_pattern": "^(.+)$",
            "rename_replacement": "restore-drill1-$1",
            "verification": {
                "required": required,
                "indices": [{"source_index": "alerts-v2", "restored_index": "restore-drill1-alerts-v2",
                             "expected": {}, "samples": [], "queries": []}],
            },
        }
        path = directory / "approval.json"
        path.write_text(json.dumps(payload), encoding="utf-8")
        return path

    def test_execute_requires_hash_pinned_approval(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = self.manifest(Path(directory))
            with self.assertRaisesRegex(TOOL.GuardError, "requires --approved-manifest-sha256"):
                TOOL.load_approval(path, None, True)

    def test_unsafe_rename_rule_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = self.manifest(Path(directory))
            payload = json.loads(path.read_text())
            payload["rename_pattern"] = ".*"
            path.write_text(json.dumps(payload), encoding="utf-8")
            with self.assertRaisesRegex(TOOL.GuardError, "rename_pattern"):
                TOOL.load_approval(path, None, False)

    def test_plaintext_endpoint_is_rejected(self) -> None:
        with self.assertRaisesRegex(TOOL.GuardError, "must use https"):
            TOOL.OpenSearchClient("http://opensearch.example:9200")


if __name__ == "__main__":
    unittest.main()
