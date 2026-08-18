#!/usr/bin/env python3
"""Quality gate for the governed model pipeline.

Reads an immutable evaluation manifest (governed_evaluation.py output) and
enforces the task-book gate bounds on the 95% confidence intervals:

  * known_attack_recall          (预警准确率 / detection rate, signed method):
                                 lower 95% CI >= min_known_attack_recall (default 0.95)
  * normal_false_positive_rate   (误报率 / false-positive rate):
                                 upper 95% CI <= max_false_positive_rate (default 0.05)
  * unknown_recall               lower 95% CI >= min_unknown_recall (default 0.80)

The assessment numbers themselves are defined by the frozen signed method
(mlops/eval_packages/topic1_blind/metric-definition.md) and are never rewritten
here.  This gate only *blocks* export/registration when a bound is not met, so
"PASS" can no longer be produced unconditionally by the evaluation step.

The manifest identity is verified first; a stale or tampered manifest cannot
pass the gate.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any, Mapping


def load_manifest(path: str) -> Mapping[str, Any]:
    """Load and identity-verify an immutable evaluation manifest."""
    target = Path(path).resolve()
    if not target.is_file():
        raise FileNotFoundError(f"evaluation manifest not found: {target}")
    with target.open("r", encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError("evaluation manifest must be a JSON object")
    # Identity bind: recompute the canonical meaning hash and deterministic ID.
    from governed_evaluation import validate_evaluation_manifest_identity

    validate_evaluation_manifest_identity(value)
    return value


def evaluate_manifest_gate(
    manifest: Mapping[str, Any],
    *,
    min_known_attack_recall: float = 0.95,
    max_false_positive_rate: float = 0.05,
    min_unknown_recall: float = 0.80,
) -> tuple[bool, list[dict[str, Any]]]:
    """Enforce gate bounds on the manifest's 95% confidence intervals.

    Fails closed: a missing confidence interval is a failure, never a pass.
    Point estimates are reported for diagnostics only and never substitute for
    the interval bounds.
    """
    intervals = manifest.get("confidence_intervals") or {}
    metrics = manifest.get("metrics") or {}
    checks: list[dict[str, Any]] = []

    def interval(name: str) -> dict[str, Any]:
        value = intervals.get(name)
        if not isinstance(value, dict):
            raise ValueError(f"evaluation manifest is missing confidence interval: {name}")
        return value

    known_attack_ci = interval("known_attack_recall")
    fpr_ci = interval("normal_false_positive_rate")
    unknown_ci = interval("unknown_recall")

    def add_check(name: str, passed: bool, detail: str) -> None:
        checks.append({"name": name, "passed": bool(passed), "detail": detail})

    lower = known_attack_ci.get("lower")
    add_check(
        "known_attack_recall",
        lower is not None and float(lower) >= min_known_attack_recall,
        f"known_attack_recall lower 95% CI {lower} >= {min_known_attack_recall}"
        f" (point {metrics.get('known_attack_recall')})",
    )
    upper = fpr_ci.get("upper")
    add_check(
        "normal_false_positive_rate",
        upper is not None and float(upper) <= max_false_positive_rate,
        f"normal_false_positive_rate upper 95% CI {upper} <= {max_false_positive_rate}"
        f" (point {metrics.get('normal_false_positive_rate')})",
    )
    unknown_lower = unknown_ci.get("lower")
    add_check(
        "unknown_recall",
        unknown_lower is not None and float(unknown_lower) >= min_unknown_recall,
        f"unknown_recall lower 95% CI {unknown_lower} >= {min_unknown_recall}"
        f" (point {metrics.get('unknown_recall')})",
    )
    return all(check["passed"] for check in checks), checks


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Enforce task-book quality bounds on an evaluation manifest"
    )
    parser.add_argument("--manifest", required=True, help="evaluation-manifest.json path")
    parser.add_argument("--min-known-attack-recall", type=float, default=0.95,
                        help="lower 95% CI bound for known_attack_recall (预警准确率/检测率)")
    parser.add_argument("--max-false-positive-rate", type=float, default=0.05,
                        help="upper 95% CI bound for normal_false_positive_rate (误报率)")
    parser.add_argument("--min-unknown-recall", type=float, default=0.80,
                        help="lower 95% CI bound for unknown_recall")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    manifest = load_manifest(args.manifest)
    passed, checks = evaluate_manifest_gate(
        manifest,
        min_known_attack_recall=args.min_known_attack_recall,
        max_false_positive_rate=args.max_false_positive_rate,
        min_unknown_recall=args.min_unknown_recall,
    )
    print(json.dumps({
        "evaluation_id": manifest.get("evaluation_id"),
        "evaluation_sha256": manifest.get("evaluation_sha256"),
        "result": "PASS" if passed else "FAIL",
        "gate": checks,
    }, sort_keys=True, indent=2))
    # Persist the report for Argo artifact collection when requested.
    report_path = os.environ.get("GATE_REPORT_PATH", "").strip()
    if report_path:
        target = Path(report_path).resolve()
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open("w", encoding="utf-8") as handle:
            json.dump({
                "evaluation_id": manifest.get("evaluation_id"),
                "evaluation_sha256": manifest.get("evaluation_sha256"),
                "result": "PASS" if passed else "FAIL",
                "gate": checks,
            }, handle, sort_keys=True, indent=2)
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
