#!/usr/bin/env python3
"""Verify the M09 ASSEMBLED BOM and fail-closed release pointer."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m09_integrated_bom as builder
from scripts.alignment import build_m09_release_artifacts as release_builder


ROOT = builder.ROOT
BOM_PATH = ROOT / builder.OUTPUT_RELATIVE
INDEX_PATH = ROOT / "doc/02_acceptance/topic1/t1-m09-p064-idx-n024-s1/evidence-index.json"
POINTER_PATH = ROOT / "contracts/releases/topic1/t1-m09-release-pointer.json"
GLOBAL_INDEX_PATH = ROOT / "contracts/alignment/evidence-index.json"


def load(path: Path) -> dict[str, Any]:
    return builder.load_json(path)


def validate(
    expected: dict[str, Any],
    bom: dict[str, Any],
    index: dict[str, Any],
    pointer: dict[str, Any],
    global_index: dict[str, Any] | None = None,
) -> list[str]:
    errors: list[str] = []
    expected_index = release_builder.build_index(
        expected, builder.sha256_file(BOM_PATH)
    )
    expected_pointer = release_builder.build_pointer(
        expected,
        builder.sha256_file(BOM_PATH),
        expected_index,
        release_builder.rendered_sha256(expected_index),
    )
    if bom != expected:
        errors.append("integrated BOM does not equal deterministic builder output")
    if index != expected_index:
        errors.append("M09 evidence index does not equal deterministic builder output")
    if pointer != expected_pointer:
        errors.append("M09 release pointer does not equal deterministic builder output")
    if bom.get("bom_state") != "ASSEMBLED" or bom.get("engineering_status") != "PASS":
        errors.append("integrated BOM is not structurally ASSEMBLED")
    if len(bom.get("components", [])) != 23:
        errors.append("integrated BOM must contain exactly 23 task components")
    task_ids = [item.get("task_id") for item in bom.get("components", [])]
    if task_ids != [item[0] for item in builder.COMPONENTS]:
        errors.append("integrated BOM component order/identity drifted")
    if any(item.get("evidence_status") != "PASS" for item in bom.get("components", [])):
        errors.append("integrated BOM contains non-PASS component evidence")
    closure = bom.get("closure", {})
    if closure.get("same_candidate_manifest") is not False or bom.get("candidate_id") is not None:
        errors.append("integrated BOM falsely claims one same candidate")
    if closure.get("verified_windows_journeys") != 0:
        errors.append("integrated BOM falsely claims Windows journey verification")
    if closure.get("complete_cross_storage_traces") != 0:
        errors.append("integrated BOM falsely claims complete cross-storage traces")
    expected_blockers = {
        "SAME_CANDIDATE_MANIFEST_REQUIRED",
        "WINDOWS_CHROME_SEVEN_JOURNEYS_REQUIRED",
        "CROSS_STORAGE_TRACE_FINAL_EFFECT_REQUIRED",
        "PRODUCTION_APPLIED_REQUIRED",
    }
    if set(bom.get("blocking_codes", [])) != expected_blockers:
        errors.append("integrated BOM blocking code set drifted")
    if bom.get("promotion_allowed") is not False or bom.get("promotion_status") != "BLOCKED":
        errors.append("integrated BOM must remain promotion-blocked")
    bom_sha = builder.sha256_file(BOM_PATH) if BOM_PATH.is_file() else None
    if index.get("artifact_kind") != "M09_EVIDENCE_INDEX" or index.get("status") != "ASSEMBLED":
        errors.append("M09 evidence index identity/status mismatch")
    if index.get("bom") != {"path": str(builder.OUTPUT_RELATIVE), "sha256": bom_sha}:
        errors.append("M09 evidence index BOM binding mismatch")
    if index.get("assembly_id") != bom.get("assembly_id"):
        errors.append("M09 evidence index assembly identity mismatch")
    if index.get("promotion_allowed") is not False or index.get("production_applied") is not False:
        errors.append("M09 evidence index overclaims promotion or production")
    index_sha = builder.sha256_file(INDEX_PATH) if INDEX_PATH.is_file() else None
    if pointer.get("artifact_kind") != "M09_RELEASE_POINTER_DECISION":
        errors.append("M09 release pointer identity mismatch")
    if pointer.get("status") != "NO_GO" or pointer.get("promotion_allowed") is not False:
        errors.append("M09 release pointer must remain NO_GO")
    if pointer.get("assembly_id") != bom.get("assembly_id") or pointer.get("candidate_id") is not None:
        errors.append("M09 release pointer candidate/assembly identity mismatch")
    if pointer.get("integrated_bom") != {"path": str(builder.OUTPUT_RELATIVE), "sha256": bom_sha}:
        errors.append("M09 release pointer BOM binding mismatch")
    if pointer.get("evidence_index") != {
        "path": str(INDEX_PATH.relative_to(ROOT)),
        "sha256": index_sha,
    }:
        errors.append("M09 release pointer evidence-index binding mismatch")
    if set(pointer.get("blocking_codes", [])) != expected_blockers:
        errors.append("M09 release pointer blocking code set drifted")
    if pointer.get("production_applied") is not False:
        errors.append("M09 release pointer falsely claims production application")
    if global_index is not None:
        entry = global_index.get("latest_m09_integrated_bom")
        expected_entry = {
            "milestone_id": "T1-M09",
            "accountable_task": "T1-M09-N024",
            "assembly_id": bom.get("assembly_id"),
            "integrated_bom": str(builder.OUTPUT_RELATIVE),
            "integrated_bom_sha256": bom_sha,
            "evidence_index": str(INDEX_PATH.relative_to(ROOT)),
            "evidence_index_sha256": index_sha,
            "release_pointer": str(POINTER_PATH.relative_to(ROOT)),
            "release_pointer_sha256": builder.sha256_file(POINTER_PATH),
            "status": "ASSEMBLED_NO_GO",
            "promotion_allowed": False,
            "production_applied": False,
            "note": "The exact-hash N001-N023 Kubernetes component index is assembled, but no single candidate manifest, seven verified Windows Chrome journeys, complete cross-storage traces, or production application exists.",
        }
        if entry != expected_entry:
            errors.append("global evidence index M09 pointer drifted")
    return errors


def main() -> int:
    for path in (BOM_PATH, INDEX_PATH, POINTER_PATH, GLOBAL_INDEX_PATH):
        if not path.is_file():
            print(f"FAIL: required N024 artifact is absent: {path.relative_to(ROOT)}")
            return 1
    errors = validate(
        builder.build(ROOT),
        load(BOM_PATH),
        load(INDEX_PATH),
        load(POINTER_PATH),
        load(GLOBAL_INDEX_PATH),
    )
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N024 ASSEMBLED BOM is current and promotion remains NO_GO")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
