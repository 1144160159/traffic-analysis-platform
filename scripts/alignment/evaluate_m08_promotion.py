#!/usr/bin/env python3
"""Evaluate the M08 exact evidence index and fail-closed promotion decision."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/alignment/m08-promotion-gate.v1.json"
SHA_RE = re.compile(r"^[0-9a-f]{64}$")
MISSING = object()


def add_issue(
    issues: list[dict[str, Any]], code: str, identity: str, detail: Any = None
) -> None:
    issues.append({"code": code, "identity": identity, "detail": detail})


def load_binding(
    binding: Any,
    *,
    issues: list[dict[str, Any]],
    identity: str,
) -> tuple[dict[str, Any], dict[str, str] | None]:
    if not isinstance(binding, dict) or set(binding) != {"path", "sha256"}:
        add_issue(issues, "BINDING_SHAPE", identity, binding)
        return {}, None
    relative = binding.get("path")
    expected_sha = binding.get("sha256")
    if not isinstance(relative, str) or not SHA_RE.fullmatch(str(expected_sha or "")):
        add_issue(issues, "BINDING_IDENTITY", identity, binding)
        return {}, None
    path = (ROOT / relative).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()) or not path.is_file():
        add_issue(issues, "BINDING_PATH", identity, relative)
        return {}, None
    observed_sha = hashlib.sha256(path.read_bytes()).hexdigest()
    if observed_sha != expected_sha:
        add_issue(
            issues,
            "BINDING_HASH",
            identity,
            {"expected": expected_sha, "observed": observed_sha},
        )
        return {}, None
    try:
        body = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        add_issue(issues, "BINDING_JSON", identity, str(error))
        return {}, None
    if not isinstance(body, dict):
        add_issue(issues, "BINDING_JSON", identity, type(body).__name__)
        return {}, None
    return body, {"path": relative, "sha256": expected_sha}


def pointer_value(body: dict[str, Any], pointer: str) -> Any:
    if not pointer.startswith("/"):
        raise ValueError(f"invalid JSON pointer: {pointer}")
    value: Any = body
    for raw_token in pointer[1:].split("/"):
        token = raw_token.replace("~1", "/").replace("~0", "~")
        if not isinstance(value, dict) or token not in value:
            return MISSING
        value = value[token]
    return value


def assertion_passes(observed: Any, operator: str, expected: Any) -> bool:
    if observed is MISSING:
        return False
    if operator == "equals":
        return observed == expected and type(observed) is type(expected)
    if operator == "greater_than":
        return (
            isinstance(observed, (int, float))
            and not isinstance(observed, bool)
            and observed > expected
        )
    raise ValueError(f"unsupported assertion operator: {operator}")


def evaluate_assertions(
    body: dict[str, Any],
    assertions: list[dict[str, Any]],
    *,
    issues: list[dict[str, Any]],
    evidence_id: str,
    code: str,
) -> None:
    for assertion in assertions:
        pointer = assertion["pointer"]
        operator = assertion["operator"]
        expected = assertion["expected"]
        observed = pointer_value(body, pointer)
        if not assertion_passes(observed, operator, expected):
            add_issue(
                issues,
                code,
                f"{evidence_id}{pointer}",
                {
                    "operator": operator,
                    "expected": expected,
                    "observed": "<MISSING>" if observed is MISSING else observed,
                },
            )


def evaluate(manifest: dict[str, Any], contract: dict[str, Any]) -> dict[str, Any]:
    engineering_errors: list[dict[str, Any]] = []
    promotion_blockers: list[dict[str, Any]] = []
    if set(manifest) != set(contract["manifest_fields"]):
        add_issue(
            engineering_errors,
            "MANIFEST_EXACT_FIELDS",
            "manifest",
            sorted(manifest),
        )
    if (
        manifest.get("schema_version") != 1
        or manifest.get("artifact_kind") != "M08_PROMOTION_MANIFEST"
    ):
        add_issue(engineering_errors, "MANIFEST_IDENTITY", "manifest")

    profile_id = manifest.get("profile_id")
    environment_id = manifest.get("environment_id")
    candidate_id = manifest.get("candidate_id")
    if not isinstance(profile_id, str) or not profile_id:
        add_issue(engineering_errors, "PROFILE_ID", "manifest", profile_id)
    if not isinstance(environment_id, str) or not environment_id:
        add_issue(engineering_errors, "ENVIRONMENT_ID", "manifest", environment_id)
    if candidate_id is not None and not SHA_RE.fullmatch(str(candidate_id)):
        add_issue(engineering_errors, "CANDIDATE_ID", "manifest", candidate_id)

    if manifest.get("allowed_claims") != contract["allowed_claims"]:
        add_issue(engineering_errors, "ALLOWED_CLAIMS", "claims")
    if manifest.get("forbidden_claims") != contract["forbidden_claims"]:
        add_issue(engineering_errors, "FORBIDDEN_CLAIMS", "claims")

    required_evidence = contract["required_evidence"]
    evidence = manifest.get("evidence", {})
    if not isinstance(evidence, dict):
        add_issue(
            engineering_errors,
            "EVIDENCE_SHAPE",
            "evidence",
            type(evidence).__name__,
        )
        evidence = {}
    if set(evidence) != set(required_evidence):
        add_issue(
            engineering_errors,
            "EVIDENCE_EXACT_SET",
            "evidence",
            sorted(evidence),
        )

    current_evidence: dict[str, dict[str, str]] = {}
    evidence_bodies: dict[str, dict[str, Any]] = {}
    for evidence_id, rule in required_evidence.items():
        body, binding = load_binding(
            evidence.get(evidence_id),
            issues=engineering_errors,
            identity=evidence_id,
        )
        if binding is not None:
            current_evidence[evidence_id] = binding
        if not body:
            continue
        evidence_bodies[evidence_id] = body
        if body.get("artifact_kind") != rule["artifact_kind"]:
            add_issue(
                engineering_errors,
                "EVIDENCE_ARTIFACT_KIND",
                evidence_id,
                {
                    "expected": rule["artifact_kind"],
                    "observed": body.get("artifact_kind"),
                },
            )
        evaluate_assertions(
            body,
            rule["engineering_assertions"],
            issues=engineering_errors,
            evidence_id=evidence_id,
            code="ENGINEERING_ASSERTION",
        )

    candidate_binding = manifest.get("candidate_manifest")
    current_candidate: dict[str, str] | None = None
    candidate_body: dict[str, Any] = {}
    if candidate_binding is None:
        add_issue(
            promotion_blockers,
            "CANDIDATE_MANIFEST_REQUIRED",
            "candidate-manifest",
        )
    else:
        candidate_body, current_candidate = load_binding(
            candidate_binding,
            issues=promotion_blockers,
            identity="candidate-manifest",
        )
        if current_candidate is not None and candidate_id != current_candidate["sha256"]:
            add_issue(
                promotion_blockers,
                "CANDIDATE_MANIFEST_HASH",
                "candidate-manifest",
                {"candidate_id": candidate_id, "sha256": current_candidate["sha256"]},
            )
        if candidate_body and (
            candidate_body.get("artifact_kind")
            != "M08_IMPLEMENTATION_CANDIDATE_MANIFEST"
            or candidate_body.get("status") != "FROZEN"
            or candidate_body.get("profile_id") != profile_id
            or candidate_body.get("environment_id") != environment_id
        ):
            add_issue(
                promotion_blockers,
                "CANDIDATE_MANIFEST_RESULT",
                "candidate-manifest",
            )

    if manifest.get("production_applied") is not True:
        add_issue(
            promotion_blockers,
            "PRODUCTION_APPLIED_REQUIRED",
            "manifest",
            manifest.get("production_applied"),
        )

    for evidence_id, body in evidence_bodies.items():
        rule = required_evidence[evidence_id]
        evaluate_assertions(
            body,
            rule["promotion_assertions"],
            issues=promotion_blockers,
            evidence_id=evidence_id,
            code="PROMOTION_ASSERTION",
        )
        if candidate_id is not None and (
            body.get("candidate_manifest_sha256") != candidate_id
            or body.get("profile_id") != profile_id
            or body.get("environment_id") != environment_id
        ):
            add_issue(
                promotion_blockers,
                "EVIDENCE_SCOPE",
                evidence_id,
                {
                    "candidate_manifest_sha256": body.get(
                        "candidate_manifest_sha256"
                    ),
                    "profile_id": body.get("profile_id"),
                    "environment_id": body.get("environment_id"),
                },
            )

    engineering_status = "PASS" if not engineering_errors else "BLOCKED"
    promotion_allowed = engineering_status == "PASS" and not promotion_blockers
    return {
        "schema_version": 1,
        "artifact_kind": "M08_PROMOTION_EVALUATION",
        "accountable_task": contract["accountable_task"],
        "candidate_id": candidate_id,
        "profile_id": profile_id,
        "environment_id": environment_id,
        "engineering_status": engineering_status,
        "promotion_status": "PASS" if promotion_allowed else "BLOCKED",
        "promotion_allowed": promotion_allowed,
        "current_candidate_manifest": current_candidate,
        "current_evidence": current_evidence,
        "engineering_errors": engineering_errors,
        "promotion_blockers": promotion_blockers,
        "allowed_claims": contract["allowed_claims"] if engineering_status == "PASS" else [],
        "forbidden_claims": contract["forbidden_claims"],
        "production_applied": False,
        "automatic_repair": False,
    }


def write_immutable(path: Path, payload: str) -> None:
    output = path.resolve()
    if not output.is_relative_to(ROOT.resolve()):
        raise ValueError("output must be inside repository")
    encoded = payload.encode("utf-8")
    if output.exists():
        if output.read_bytes() != encoded:
            raise ValueError(f"refusing to overwrite immutable evaluation: {output}")
        return
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(f".{output.name}.tmp")
    temporary.write_bytes(encoded)
    temporary.replace(output)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    manifest_path = args.manifest.resolve(strict=True)
    if not manifest_path.is_relative_to(ROOT.resolve()):
        raise SystemExit("manifest must be inside repository")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    result = evaluate(manifest, contract)
    payload = json.dumps(result, sort_keys=True, indent=2) + "\n"
    if args.output is not None:
        try:
            write_immutable(args.output, payload)
        except ValueError as error:
            raise SystemExit(str(error)) from error
    print(payload, end="")
    return 0 if result["promotion_allowed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
