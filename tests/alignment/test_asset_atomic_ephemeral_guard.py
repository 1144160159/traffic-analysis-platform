from __future__ import annotations

import hashlib
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "verify_asset_atomic_ephemeral",
    ROOT / "scripts/alignment/verify_asset_atomic_ephemeral.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AssetAtomicEphemeralGuardTest(unittest.TestCase):
    def test_oracle_markers_require_exact_per_test_set(self) -> None:
        events = [
            {"Test": test, "Output": f"TOPIC1_ORACLE PASS {marker}\n"}
            for test, markers in MODULE.ASSET_ORACLE_MARKERS.items()
            for marker in markers
        ]
        actual = MODULE.collect_asset_oracle_markers(events)
        self.assertEqual(set(actual), set(MODULE.ASSET_ORACLE_MARKERS))

    def test_missing_or_duplicate_oracle_marker_is_rejected(self) -> None:
        events = [
            {"Test": test, "Output": f"TOPIC1_ORACLE PASS {marker}\n"}
            for test, markers in MODULE.ASSET_ORACLE_MARKERS.items()
            for marker in markers
        ]
        events.pop()
        with self.assertRaisesRegex(ValueError, "oracle exact-set mismatch"):
            MODULE.collect_asset_oracle_markers(events)

    def test_candidate_manifest_must_bind_current_sources(self) -> None:
        source = ROOT / "go/control-plane/internal/asset/repository/atomic_upsert.go"
        relative = source.relative_to(ROOT).as_posix()
        correct = hashlib.sha256(source.read_bytes()).hexdigest()
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "candidate.json"
            manifest.write_text(
                json.dumps({"source_blob_sha256": {relative: correct}}),
                encoding="utf-8",
            )
            self.assertEqual(
                MODULE.require_candidate_sources(manifest, [source]),
                {relative: correct},
            )
            manifest.write_text(
                json.dumps({"source_blob_sha256": {relative: "0" * 64}}),
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "does not bind current source"):
                MODULE.require_candidate_sources(manifest, [source])


if __name__ == "__main__":
    unittest.main()
