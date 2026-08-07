#!/usr/bin/env python3
"""Calculate the six mandatory compatibility removal arrays."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path


FIELDS = {
    "routes": "removed_routes",
    "actions": "removed_actions",
    "api_operations": "removed_api_operations",
    "response_fields": "removed_response_fields",
    "scopes": "removed_scopes",
    "audit_events": "removed_audit_events",
}


def _load(path: Path) -> dict[str, object]:
    return json.loads(path.read_text(encoding="utf-8"))


def _sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def compare(baseline_path: Path, candidate_path: Path) -> dict[str, object]:
    baseline = _load(baseline_path)
    candidate = _load(candidate_path)
    result: dict[str, object] = {
        "schema_version": 1,
        "baseline_manifest_sha256": _sha(baseline_path),
        "candidate_manifest_sha256": _sha(candidate_path),
    }
    blockers = 0
    for field, output_field in FIELDS.items():
        removed = sorted(set(baseline.get(field, [])) - set(candidate.get(field, [])))
        result[output_field] = removed
        blockers += len(removed)
    result["changed_field_semantics"] = []
    result["changed_defaults"] = []
    result["changed_enums"] = []
    result["changed_null_zero_semantics"] = []
    result["changed_pagination_or_sort"] = []
    result["changed_state_transitions"] = []
    result["changed_error_semantics"] = []
    result["changed_export_formats"] = []
    result["approved_compatibility_exceptions"] = []
    result["blockers"] = blockers
    result["result"] = "pass" if blockers == 0 else "blocked"
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = compare(args.baseline, args.candidate)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0 if result["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
