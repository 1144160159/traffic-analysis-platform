#!/usr/bin/env python3
"""Build the N024 evidence index and fail-closed release pointer."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m09_integrated_bom as bom_builder


ROOT = bom_builder.ROOT
INDEX_RELATIVE = Path(
    "doc/02_acceptance/topic1/t1-m09-p064-idx-n024-s1/evidence-index.json"
)
POINTER_RELATIVE = Path("contracts/releases/topic1/t1-m09-release-pointer.json")


def build_index(bom: dict[str, Any], bom_sha256: str) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "artifact_kind": "M09_EVIDENCE_INDEX",
        "milestone_id": "T1-M09",
        "accountable_task": "T1-M09-N024",
        "atomic_pr_id": "T1-M09-P064-IDX-n024-s1",
        "status": "ASSEMBLED",
        "assembly_id": bom["assembly_id"],
        "candidate_id": bom["candidate_id"],
        "profile_id": bom["profile_id"],
        "environment_id": bom["environment_id"],
        "bom": {
            "path": str(bom_builder.OUTPUT_RELATIVE),
            "sha256": bom_sha256,
        },
        "component_evidence": [
            {"task_id": item["task_id"], **item["evidence"]}
            for item in bom["components"]
        ],
        "journey_input": bom["journey_input"],
        "closure": bom["closure"],
        "engineering_status": bom["engineering_status"],
        "promotion_status": bom["promotion_status"],
        "promotion_allowed": bom["promotion_allowed"],
        "blocking_codes": bom["blocking_codes"],
        "production_applied": False,
        "does_not_prove": [
            "new runtime evidence",
            "one same-candidate M09 end-to-end journey",
            "designated Windows Chrome acceptance",
            "authorization to promote M09",
            "M09 or project completion",
        ],
    }


def build_pointer(
    bom: dict[str, Any],
    bom_sha256: str,
    evidence_index: dict[str, Any],
    evidence_index_sha256: str,
) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "artifact_kind": "M09_RELEASE_POINTER_DECISION",
        "milestone_id": "T1-M09",
        "accountable_task": "T1-M09-N024",
        "atomic_pr_id": "T1-M09-P065-PROM-n024-s2",
        "status": "NO_GO",
        "milestone_state": "IMPLEMENTING",
        "assembly_id": bom["assembly_id"],
        "candidate_id": bom["candidate_id"],
        "profile_id": bom["profile_id"],
        "environment_id": bom["environment_id"],
        "integrated_bom": {
            "path": str(bom_builder.OUTPUT_RELATIVE),
            "sha256": bom_sha256,
        },
        "evidence_index": {
            "path": str(INDEX_RELATIVE),
            "sha256": evidence_index_sha256,
        },
        "engineering_status": evidence_index["engineering_status"],
        "promotion_status": "BLOCKED",
        "promotion_allowed": False,
        "blocking_codes": evidence_index["blocking_codes"],
        "allowed_claims": bom["allowed_claims"],
        "forbidden_claims": bom["forbidden_claims"],
        "production_applied": False,
        "automatic_repair": False,
        "supersedes_sha256": None,
    }


def expected_artifacts(root: Path = ROOT) -> tuple[dict[str, Any], dict[str, Any]]:
    bom_path = root / bom_builder.OUTPUT_RELATIVE
    if not bom_path.is_file():
        raise ValueError("M09 integrated BOM is absent")
    expected_bom = bom_builder.build(root)
    actual_bom = bom_builder.load_json(bom_path)
    if actual_bom != expected_bom:
        raise ValueError("M09 integrated BOM is stale")
    bom_sha = bom_builder.sha256_file(bom_path)
    index = build_index(actual_bom, bom_sha)
    index_sha = rendered_sha256(index)
    pointer = build_pointer(actual_bom, bom_sha, index, index_sha)
    return index, pointer


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def rendered_sha256(value: dict[str, Any]) -> str:
    return hashlib.sha256(render(value).encode()).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    index, pointer = expected_artifacts(ROOT)
    expected = ((INDEX_RELATIVE, index), (POINTER_RELATIVE, pointer))
    if args.check:
        stale = [
            str(relative)
            for relative, body in expected
            if not (ROOT / relative).is_file()
            or (ROOT / relative).read_text(encoding="utf-8") != render(body)
        ]
        if stale:
            print(f"FAIL: stale M09 release artifacts: {','.join(stale)}")
            return 1
        print("PASS: M09 evidence index and NO_GO pointer are deterministic")
        return 0
    for relative, body in expected:
        path = ROOT / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(render(body), encoding="utf-8")
    print(json.dumps({"status": "NO_GO", "assembly_id": pointer["assembly_id"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
