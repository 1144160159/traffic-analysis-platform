#!/usr/bin/env python3
"""Evaluate the T1-M10-N005 apply boundary before any mutating client starts."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


ROOT_HINT = Path(__file__).resolve().parents[2]
if str(ROOT_HINT) not in sys.path:
    sys.path.insert(0, str(ROOT_HINT))

from scripts.alignment import build_m10_approved_additive_plan as builder
from scripts.alignment import verify_m10_approved_additive_plan as verifier


def evaluate_apply_authorization(root: Path, plan: dict[str, Any]) -> dict[str, Any]:
    expected = builder.build(root)
    errors = verifier.validate(expected, plan)
    blockers = list(plan.get("blocking_codes", [])) if isinstance(plan.get("blocking_codes"), list) else []
    if plan.get("status") != "AUTHORIZED":
        errors.append("plan status is not AUTHORIZED")
    if plan.get("apply_allowed") is not True:
        errors.append("apply_allowed is not true")
    if blockers:
        errors.append("plan retains blocking codes")
    artifacts = plan.get("artifacts", [])
    if not isinstance(artifacts, list) or len(artifacts) != len(builder.PROPOSED_ARTIFACTS):
        errors.append("complete proposed artifact set is required")
    elif any(item.get("safety_class") != "ADDITIVE_STATIC" for item in artifacts if isinstance(item, dict)):
        errors.append("every artifact must be classified ADDITIVE_STATIC")
    reasons = sorted(set(errors))
    return {
        "artifact_kind": "M10_ADDITIVE_APPLY_AUTHORIZATION_DECISION",
        "task_id": "T1-M10-N005",
        "decision": "AUTHORIZED" if not reasons else "BLOCKED",
        "candidate_id": plan.get("candidate_id"),
        "plan_basis_sha256": plan.get("plan_basis_sha256"),
        "reason_count": len(reasons),
        "reasons": reasons,
        "mutating_client_started": False,
        "shared_infrastructure_touched": False,
        "production_applied": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=builder.ROOT)
    parser.add_argument("--plan", type=Path)
    parser.add_argument("--expect-blocked", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    root = args.root.resolve(strict=True)
    plan_path = args.plan or root / builder.OUTPUT_RELATIVE
    plan = builder.load_json(plan_path)
    decision = evaluate_apply_authorization(root, plan)
    print(json.dumps(decision, sort_keys=True) if args.json else json.dumps(decision, indent=2, sort_keys=True))
    if args.expect_blocked:
        return 0 if decision["decision"] == "BLOCKED" else 1
    return 0 if decision["decision"] == "AUTHORIZED" else 3


if __name__ == "__main__":
    raise SystemExit(main())
