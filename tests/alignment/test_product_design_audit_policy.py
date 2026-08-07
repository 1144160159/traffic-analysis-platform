import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

import sys
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_product_design_audit_policy import AGENT, POLICY, PROCESS, verify  # noqa: E402


def copy_candidate(parent: Path) -> Path:
    candidate = parent / "repo"
    for relative in (POLICY, PROCESS, AGENT):
        target = candidate / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)
    return candidate


class ProductDesignAuditPolicyTest(unittest.TestCase):
    def test_repository_policy_passes(self) -> None:
        result = verify(ROOT)
        self.assertEqual("PASS", result["status"], result)
        self.assertFalse(result["figma_enabled"])
        self.assertTrue(result["closed_findings_excluded"])

    def test_figma_audit_enable_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / POLICY
            payload = json.loads(path.read_text())
            payload["figma_enabled"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("Figma disabled" in error for error in result["errors"]))

    def test_closed_findings_returning_to_open_reports_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / POLICY
            payload = json.loads(path.read_text())
            payload["finding_lifecycle"]["default_progress_report_statuses"].append("CLOSED")
            payload["audit_output"]["show_closed_findings"] = True
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("exclude CLOSED" in error or "repeat CLOSED" in error for error in result["errors"]))

    def test_current_run_screenshot_guard_removal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            candidate = copy_candidate(Path(directory))
            path = candidate / POLICY
            payload = json.loads(path.read_text())
            payload["evidence"]["current_run_screenshots_only"] = False
            path.write_text(json.dumps(payload))
            result = verify(candidate)
            self.assertEqual("FAIL", result["status"])
            self.assertTrue(any("current_run_screenshots_only" in error for error in result["errors"]))


if __name__ == "__main__":
    unittest.main()
