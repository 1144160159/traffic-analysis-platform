#!/usr/bin/env python3
"""Fail-closed negative tests for leaf result -> evidence manifest identity."""

from __future__ import annotations

import copy
import importlib.util
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = ROOT / "scripts/alignment/write_evidence_run_manifest.py"
sys.path.insert(0, str(MODULE_PATH.parent))
SPEC = importlib.util.spec_from_file_location("write_evidence_run_manifest", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main() -> int:
    design_sha = "a" * 64
    source_sha = "b" * 64
    design = {"source_blob_sha256": {"go/example.go": source_sha}}
    good = {
        "artifact_kind": "EXACT_GO_TEST_RESULT",
        "subject_pr_id": "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification",
        "candidate_manifest": {"path": "design.json", "sha256": design_sha},
        "profile_id": "PROFILE-A",
        "environment_id": "ENV-A",
        "run_id": "RUN-A",
        "source_blob_sha256": {"go/example.go": source_sha},
        "event_counts": {"TestExact": {"run": 1, "pass": 1}},
    }
    subject = "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification"
    MODULE.validate_result_identity(good, design, design_sha, "PROFILE-A", "ENV-A", "RUN-A", subject)
    mutations = {
        "fake-pass": {"status": "PASS"},
        "cross-candidate": {**good, "candidate_manifest": {"path": "design.json", "sha256": "c" * 64}},
        "cross-profile": {**good, "profile_id": "PROFILE-B"},
        "cross-environment": {**good, "environment_id": "ENV-B"},
        "cross-run": {**good, "run_id": "RUN-B"},
        "cross-leaf": {**good, "subject_pr_id": "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification"},
        "cross-source": {**good, "source_blob_sha256": {"go/example.go": "d" * 64}},
        "empty-source": {**good, "source_blob_sha256": {}},
        "no-exact-events": {**good, "event_counts": {}},
    }
    for case_id, payload in mutations.items():
        try:
            MODULE.validate_result_identity(
                copy.deepcopy(payload), design, design_sha, "PROFILE-A", "ENV-A", "RUN-A", subject
            )
        except ValueError:
            continue
        raise AssertionError(f"identity adapter accepted {case_id}")
    print("PASS evidence manifest identity negatives: fake/candidate/profile/environment/run/source/oracle")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
