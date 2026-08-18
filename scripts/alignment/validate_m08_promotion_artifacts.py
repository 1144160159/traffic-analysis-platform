#!/usr/bin/env python3
"""Validate M08 promotion schemas, exact evidence and the NO-GO pointer."""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
from typing import Any

import jsonschema


ROOT = Path(__file__).resolve().parents[2]
EVALUATOR_PATH = ROOT / "scripts/alignment/evaluate_m08_promotion.py"
INPUT_PATH = (
    ROOT
    / "doc/02_acceptance/topic1/work-orders/t1-m08-p038-idx-n018-s1/promotion-input.json"
)
INDEX_PATH = INPUT_PATH.with_name("current-index.json")
POINTER_PATH = ROOT / "contracts/releases/topic1/t1-m08-release-pointer.json"
MANIFEST_SCHEMA_PATH = ROOT / "contracts/alignment/m08-promotion-manifest.schema.json"
POINTER_SCHEMA_PATH = ROOT / "contracts/alignment/m08-release-pointer.schema.json"


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_evaluator():
    spec = importlib.util.spec_from_file_location("evaluate_m08_promotion", EVALUATOR_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def validate() -> dict[str, Any]:
    promotion_input = load_json(INPUT_PATH)
    persisted_index = load_json(INDEX_PATH)
    pointer = load_json(POINTER_PATH)
    jsonschema.Draft202012Validator(
        load_json(MANIFEST_SCHEMA_PATH)
    ).validate(promotion_input)
    jsonschema.Draft202012Validator(
        load_json(POINTER_SCHEMA_PATH),
        format_checker=jsonschema.FormatChecker(),
    ).validate(pointer)

    evaluator = load_evaluator()
    contract = load_json(evaluator.CONTRACT_PATH)
    evaluated = evaluator.evaluate(promotion_input, contract)
    if evaluated != persisted_index:
        raise ValueError("persisted current index differs from a fresh evaluation")
    if evaluated["engineering_status"] != "PASS":
        raise ValueError("current engineering evidence index is not PASS")
    if evaluated["promotion_status"] != "BLOCKED" or evaluated["promotion_allowed"]:
        raise ValueError("current evidence unexpectedly authorizes M08 promotion")
    if pointer["status"] != "NO_GO" or pointer["promotion_allowed"]:
        raise ValueError("release pointer is not fail-closed")
    if pointer["candidate_id"] is not None:
        raise ValueError("NO-GO pointer must not name a promoted candidate")
    for identity, expected_path in (
        ("promotion_input", INPUT_PATH),
        ("current_index", INDEX_PATH),
    ):
        binding = pointer[identity]
        observed_path = (ROOT / binding["path"]).resolve(strict=True)
        if observed_path != expected_path.resolve() or binding["sha256"] != sha256(
            expected_path
        ):
            raise ValueError(f"release pointer {identity} binding drifted")
    if pointer["allowed_claims"] != contract["allowed_claims"]:
        raise ValueError("allowed claims drifted")
    if pointer["forbidden_claims"] != contract["forbidden_claims"]:
        raise ValueError("forbidden claims drifted")
    if len(evaluated["current_evidence"]) != len(contract["required_evidence"]):
        raise ValueError("current evidence set is incomplete")
    return {
        "schema_version": 1,
        "artifact_kind": "M08_PROMOTION_K8S_INDEX_VALIDATION",
        "accountable_task": "T1-M08-N018",
        "engineering_status": evaluated["engineering_status"],
        "promotion_status": evaluated["promotion_status"],
        "promotion_allowed": evaluated["promotion_allowed"],
        "evidence_count": len(evaluated["current_evidence"]),
        "engineering_error_count": len(evaluated["engineering_errors"]),
        "promotion_blocker_count": len(evaluated["promotion_blockers"]),
        "promotion_input_sha256": sha256(INPUT_PATH),
        "current_index_sha256": sha256(INDEX_PATH),
        "release_pointer_sha256": sha256(POINTER_PATH),
        "production_applied": False,
        "status": "PASS",
    }


def main() -> None:
    print(json.dumps(validate(), sort_keys=True))


if __name__ == "__main__":
    main()
