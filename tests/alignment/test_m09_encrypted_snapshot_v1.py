from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts/alignment"
if str(SCRIPTS) not in sys.path:
    sys.path.insert(0, str(SCRIPTS))

from build_feature_contract_registry import build_registry  # noqa: E402


class M09EncryptedSnapshotV1Tests(unittest.TestCase):
    def test_feature_contract_is_exact_and_preserves_legacy_reads(self) -> None:
        registry = build_registry()
        feature = next(
            item for item in registry["features"]
            if item["feature_id"] == "F-ENCRYPTED-001"
        )
        formal = feature["formal_contract"]
        self.assertEqual("EXACT", formal["openapi_binding_status"])
        self.assertEqual([], formal["validation_errors"])
        self.assertEqual([], feature["blocking_gaps"])
        self.assertEqual(
            {
                "GET /v1/encrypted-traffic/stats",
                "GET /v1/encrypted-traffic/sessions",
                "GET /v1/encrypted-traffic/ja3",
                "GET /v1/encrypted-traffic/tunnels",
                "GET /v1/encrypted-traffic/exfiltration",
                "GET /v1/encrypted-traffic/evidence",
            },
            set(formal["preserved_api_operations"]),
        )

    def test_openapi_exposes_bounded_partial_aware_snapshot(self) -> None:
        openapi = json.loads(
            (ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text()
        )
        operation = openapi["paths"]["/v1/encrypted-traffic/snapshot"]["get"]
        self.assertEqual("getEncryptedTrafficSnapshot", operation["operationId"])
        self.assertEqual("F-ENCRYPTED-001", operation["x-feature-id"])
        self.assertEqual("alert:read", operation["x-required-scope"])
        parameters = {item["name"]: item for item in operation["parameters"]}
        self.assertTrue(parameters["start_time"]["required"])
        self.assertTrue(parameters["end_time"]["required"])
        self.assertEqual(8192, parameters["continuation"]["schema"]["maxLength"])
        availability = openapi["components"]["schemas"]["EncryptedTrafficSnapshotSection"]["properties"]["availability"]["enum"]
        self.assertEqual(
            ["available", "zero", "no_sample", "not_computable", "unavailable", "forbidden"],
            availability,
        )

    def test_go_handler_keeps_security_and_resource_gates_explicit(self) -> None:
        source = (
            ROOT / "go/control-plane/internal/alert/api/encrypted_traffic_snapshot_v1.go"
        ).read_text()
        for marker in (
            "TENANT_SOURCE_FORBIDDEN",
            "ScopeAlertRead",
            "ScopePcapRead",
            "ScopePcapDownload",
            "encryptedTrafficSnapshotMaxWindow",
            "encryptedTrafficSnapshotMaxEvidenceRefs",
            "encryptedTrafficSnapshotMaxResponseBytes",
            "encryptedTrafficSnapshotTimeout",
            "indicator_only",
            "sample_bytes_not_persisted",
            "readEncryptedSnapshotTableColumns",
        ):
            self.assertIn(marker, source)


if __name__ == "__main__":
    unittest.main()
