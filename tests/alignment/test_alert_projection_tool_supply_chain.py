#!/usr/bin/env python3

from __future__ import annotations

import sys
import subprocess
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from capture_alert_projection_tool_supply_chain import (  # noqa: E402
    assess_vulnerability_scan,
    build_cyclonedx,
    parse_go_version_modules,
    scanner_metadata,
    run_govulncheck,
    validate_image_binding,
)


GO_VERSION_OUTPUT = """/tmp/tool: go1.25.12
\tpath\texample.test/project/cmd/tool
\tmod\texample.test/project\t(devel)\t
\tdep\texample.test/dependency-b\tv2.0.0\th1:bbbb=
\tdep\texample.test/dependency-a\tv1.0.0\th1:aaaa=
\tbuild\tCGO_ENABLED=0
"""


def sarif(*levels: str) -> str:
    results = [{"level": level, "ruleId": f"GO-{index}"} for index, level in enumerate(levels)]
    return __import__("json").dumps({"version": "2.1.0", "runs": [{"results": results}]})


class AlertProjectionToolSupplyChainTests(unittest.TestCase):
    def test_cyclonedx_is_sorted_and_deterministic(self) -> None:
        metadata = parse_go_version_modules(GO_VERSION_OUTPUT)
        self.assertEqual([item["path"] for item in metadata["modules"]], [
            "example.test/dependency-a", "example.test/dependency-b",
        ])
        binaries = {"tool-b": metadata, "tool-a": metadata}
        hashes = {"tool-a": "a" * 64, "tool-b": "b" * 64}
        first = build_cyclonedx(binaries, hashes)
        second = build_cyclonedx(dict(reversed(list(binaries.items()))), hashes)
        self.assertEqual(first, second)
        self.assertEqual(first["bomFormat"], "CycloneDX")
        self.assertEqual(first["specVersion"], "1.5")
        self.assertEqual([item["ref"] for item in first["dependencies"]], sorted(
            item["ref"] for item in first["dependencies"]
        ))

    def test_sarif_error_blocks_even_when_sarif_exit_code_is_zero(self) -> None:
        result = assess_vulnerability_scan(
            text_returncode=3, text_output="Your code is affected by 1 vulnerability.",
            sarif_returncode=0, sarif_output=sarif("error"),
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["reachable_known_vulnerabilities"], 1)

    def test_text_exit_three_blocks_when_sarif_omits_error(self) -> None:
        result = assess_vulnerability_scan(
            text_returncode=3, text_output="Your code is affected by 1 vulnerability.",
            sarif_returncode=0, sarif_output=sarif("note"),
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertIn("text scan reports reachable vulnerabilities", " ".join(result["errors"]))

    def test_transport_failure_is_retried_but_vulnerability_result_is_terminal(self) -> None:
        transient = subprocess.CompletedProcess(["govulncheck"], 1, "unexpected EOF")
        clean = subprocess.CompletedProcess(["govulncheck"], 0, "No vulnerabilities found.")
        with patch("capture_alert_projection_tool_supply_chain.run", side_effect=[transient, clean]) as runner:
            completed, attempts = run_govulncheck(["govulncheck"], cwd=ROOT)
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(attempts, 2)
        self.assertEqual(runner.call_count, 2)

        vulnerable = subprocess.CompletedProcess(["govulncheck"], 3, "affected")
        with patch("capture_alert_projection_tool_supply_chain.run", return_value=vulnerable) as runner:
            completed, attempts = run_govulncheck(["govulncheck"], cwd=ROOT)
        self.assertEqual(completed.returncode, 3)
        self.assertEqual(attempts, 1)
        self.assertEqual(runner.call_count, 1)

    def test_clean_scan_records_nonreachable_module_finding(self) -> None:
        result = assess_vulnerability_scan(
            text_returncode=0,
            text_output=(
                "Your code is affected by 0 vulnerabilities.\n"
                "This scan also found 0 vulnerabilities in packages you import and 1 "
                "vulnerability in modules you require, but your code doesn't appear to call these vulnerabilities."
            ),
            sarif_returncode=0, sarif_output=sarif("note"),
        )
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["nonreachable_module_vulnerabilities"], 1)

    def test_stale_database_fails_closed(self) -> None:
        result = scanner_metadata(
            "Scanner: govulncheck@v1.1.4\nDB updated: 2026-08-01 00:00:00 +0000 UTC\n",
            now=datetime(2026, 8, 8, tzinfo=timezone.utc), maximum_db_age_seconds=86_400,
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertIn("stale", " ".join(result["errors"]))

    def test_missing_scanner_metadata_fails_closed(self) -> None:
        result = scanner_metadata(
            "No version metadata\n", now=datetime(2026, 8, 8, tzinfo=timezone.utc),
            maximum_db_age_seconds=86_400,
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertGreaterEqual(len(result["errors"]), 2)

    def test_local_image_and_source_label_drift_remain_on_hold(self) -> None:
        inspection = {
            "RepoDigests": [],
            "Config": {"Labels": {
                "org.opencontainers.image.revision": "wrong-head",
                "org.opencontainers.image.source-content-sha256": "0" * 64,
            }},
        }
        errors = validate_image_binding(
            inspection=inspection, image="traffic/tools:local", mode="local",
            g0_content_sha256="a" * 64, g0_head="b" * 40,
        )
        self.assertIn("image source-content label does not match G0", errors)
        self.assertIn("image revision label does not match G0", errors)
        self.assertIn("local image has no approval-eligible repository manifest digest", errors)
        self.assertIn("registry signature verification is absent", errors)
        self.assertIn("SLSA provenance verification is absent", errors)


if __name__ == "__main__":
    unittest.main()
