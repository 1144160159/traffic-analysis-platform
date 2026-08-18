#!/usr/bin/env python3
"""Validate the exact-five M02 delivery package without granting release authority."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/alignment/m02-delivery-package.schema.json"
REQUIRED = {
    "install-manifest": "deployments/releases/topic1/m02-install-manifest.v1.json",
    "preflight-plan": "deployments/releases/topic1/m02-preflight-plan.v1.json",
    "upgrade-plan": "deployments/releases/topic1/m02-upgrade-plan.v1.json",
    "rollback-plan": "deployments/releases/topic1/m02-rollback-plan.v1.json",
    "restore-plan": "deployments/releases/topic1/m02-restore-plan.v1.json",
}
INSTALL_ORDER = list(REQUIRED)
ROLLBACK_ACTIONS = [
    "disable Probe production before any downstream change",
    "disable Gateway flow and PCAP writers",
    "restore previous immutable image config and route",
    "retain consumers and reconcile offsets spool objects and indexes",
]
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")


class DeliveryPackageError(ValueError):
    pass


def load_json(path: Path) -> dict[str, Any]:
    def unique(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                raise DeliveryPackageError(f"duplicate JSON property in {path}: {key}")
            result[key] = value
        return result

    value = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique)
    if not isinstance(value, dict):
        raise DeliveryPackageError(f"delivery artifact root must be an object: {path}")
    return value


def validate_definition(instance: Any, definition: str) -> None:
    schema = load_json(SCHEMA)
    schema["$ref"] = f"#/$defs/{definition}"
    # The repository's fail-closed validator takes a path. Keep the selected
    # in-memory root in a bounded OS temporary file, never in the source tree.
    temporary_name: str | None = None
    try:
        with tempfile.NamedTemporaryFile("w", encoding="utf-8", suffix=".json", delete=False) as stream:
            json.dump(schema, stream, sort_keys=True)
            stream.flush()
            temporary_name = stream.name
        validate_against_schema(instance, Path(temporary_name))
    finally:
        if temporary_name is not None:
            Path(temporary_name).unlink(missing_ok=True)


def resolve_artifact(relative: str) -> Path:
    path = ROOT / relative
    resolved = path.resolve(strict=True)
    if resolved != path or path.is_symlink() or not path.is_file():
        raise DeliveryPackageError(f"delivery artifact must be a canonical regular file: {relative}")
    return path


def run_git(*args: str, input_bytes: bytes | None = None) -> bytes:
    completed = subprocess.run(
        ["git", *args],
        cwd=ROOT,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stderr.decode(errors="replace").strip()
        raise DeliveryPackageError(f"git {' '.join(args)} failed: {detail}")
    return completed.stdout


def validate_steps(artifact: dict[str, Any], role: str) -> None:
    steps = artifact["body"]["steps"]
    if [item["order"] for item in steps] != list(range(1, len(steps) + 1)):
        raise DeliveryPackageError(f"{role} steps are not contiguous and ordered")
    if artifact["body"]["rollback_plan_path"] != REQUIRED["rollback-plan"]:
        raise DeliveryPackageError(f"{role} does not bind the canonical rollback plan")
    if artifact["body"]["preserve_durable_data"] is not True:
        raise DeliveryPackageError(f"{role} may not delete durable data")


def validate_identity(artifacts: dict[str, dict[str, Any]]) -> None:
    observed_roles = [artifacts[role]["artifact_role"] for role in REQUIRED]
    if observed_roles != list(REQUIRED) or len(set(observed_roles)) != 5:
        raise DeliveryPackageError("delivery package role set is not the ordered exact-five set")
    identity_fields = ("package_id",)
    expected = {field: artifacts[INSTALL_ORDER[0]][field] for field in identity_fields}
    for role, artifact in artifacts.items():
        conflicts = {field: (expected[field], artifact[field]) for field in identity_fields if artifact[field] != expected[field]}
        if conflicts:
            raise DeliveryPackageError(f"{role} delivery identity conflicts: {conflicts}")


def git_blob_ref(commit: str, relative: str, path: Path) -> dict[str, str]:
    status = run_git("status", "--porcelain=v1", "--", relative).decode().strip()
    if status:
        raise DeliveryPackageError(f"candidate delivery artifact is dirty or untracked: {relative}")
    record = run_git("ls-tree", commit, "--", relative).decode().strip()
    match = re.fullmatch(r"(100644|100755) blob ([0-9a-f]{40})\t(.+)", record)
    if match is None or match.group(3) != relative:
        raise DeliveryPackageError(f"delivery closure does not contain one regular Git blob: {relative}")
    candidate_bytes = run_git("show", f"{commit}:{relative}")
    current_bytes = path.read_bytes()
    if candidate_bytes != current_bytes:
        raise DeliveryPackageError(f"working-tree artifact differs from delivery-closure Git blob: {relative}")
    observed_blob = run_git("hash-object", "--stdin", input_bytes=current_bytes).decode().strip()
    if observed_blob != match.group(2):
        raise DeliveryPackageError(f"Git blob identity mismatch: {relative}")
    return {
        "git_blob_sha1": match.group(2),
        "content_sha256": hashlib.sha256(current_bytes).hexdigest(),
    }


def build_package(
    artifacts: dict[str, dict[str, Any]],
    refs: dict[str, dict[str, str]],
    *,
    candidate_commit: str | None,
    candidate_tree_sha256: str | None,
    candidate_manifest_sha256: str | None,
    profile_id: str | None,
    environment_id: str | None,
) -> dict[str, Any]:
    first = artifacts[INSTALL_ORDER[0]]
    package = {
        "schema_version": 1,
        "artifact_kind": "M02_EXACT_FIVE_DELIVERY_PACKAGE",
        "package_id": first["package_id"],
        "status": "CANDIDATE_BOUND" if candidate_commit is not None else "PENDING_CANDIDATE_BINDING",
        "candidate_commit": candidate_commit,
        "candidate_tree_sha256": candidate_tree_sha256,
        "candidate_manifest_sha256": candidate_manifest_sha256,
        "profile_id": profile_id or "T1_M02_MILESTONE_PROFILE",
        "environment_id": environment_id or "PENDING_ENVIRONMENT_BINDING",
        "artifacts": [
            {"role": role, "path": path, **refs[role]}
            for role, path in REQUIRED.items()
        ],
        "install_order": INSTALL_ORDER,
        "preflight_checks": [item["action"] for item in artifacts["preflight-plan"]["body"]["steps"]],
        "upgrade_steps": artifacts["upgrade-plan"]["body"]["steps"],
        "rollback_steps": artifacts["rollback-plan"]["body"]["steps"],
        "restore_steps": artifacts["restore-plan"]["body"]["steps"],
    }
    validate_against_schema(package, SCHEMA)
    if [item["action"] for item in package["rollback_steps"]] != ROLLBACK_ACTIONS:
        raise DeliveryPackageError("rollback action order differs from the fail-closed contract")
    return package


def validate_delivery_package(
    *,
    candidate_commit: str | None = None,
    candidate_manifest_path: Path | None = None,
    candidate_manifest_sha256: str | None = None,
    profile_id: str | None = None,
    environment_id: str | None = None,
) -> dict[str, Any]:
    candidate_bound = candidate_commit is not None
    artifacts: dict[str, dict[str, Any]] = {}
    refs: dict[str, dict[str, str]] = {}
    for role, relative in REQUIRED.items():
        path = resolve_artifact(relative)
        artifact = load_json(path)
        validate_definition(artifact, "delivery_artifact")
        validate_steps(artifact, role)
        artifacts[role] = artifact
        refs[role] = (
            git_blob_ref(candidate_commit, relative, path)
            if candidate_bound
            else {
                "git_blob_sha1": "0" * 40,
                "content_sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            }
        )
    validate_identity(artifacts)
    manifest_body: dict[str, Any] | None = None
    if candidate_bound:
        if not all((candidate_manifest_path, candidate_manifest_sha256, profile_id, environment_id)):
            raise DeliveryPackageError("candidate validation requires manifest path hash profile and environment")
        if not COMMIT_RE.fullmatch(candidate_commit) or not SHA256_RE.fullmatch(candidate_manifest_sha256 or ""):
            raise DeliveryPackageError("candidate commit or manifest digest is invalid")
        run_git("cat-file", "-e", f"{candidate_commit}^{{commit}}")
        manifest = candidate_manifest_path.resolve(strict=True)
        if not manifest.is_relative_to(ROOT.resolve()) or manifest.is_symlink() or not manifest.is_file():
            raise DeliveryPackageError("candidate manifest must be a regular repository file")
        if hashlib.sha256(manifest.read_bytes()).hexdigest() != candidate_manifest_sha256:
            raise DeliveryPackageError("candidate manifest SHA-256 mismatch")
        try:
            manifest_body = load_json(manifest)
            validate_against_schema(manifest_body, ROOT / "contracts/alignment/implementation-candidate.schema.json")
        except ValueError as error:
            raise DeliveryPackageError(f"candidate manifest schema invalid: {error}") from error
        expected_delivery = [
            {
                "artifact_id": role,
                "path": relative,
                "sha256": refs[role]["content_sha256"],
            }
            for role, relative in REQUIRED.items()
        ]
        if manifest_body["delivery_artifacts"] != expected_delivery:
            raise DeliveryPackageError("candidate manifest delivery artifact exact-set mismatch")
        if manifest_body["environment_id"] != environment_id:
            raise DeliveryPackageError("candidate manifest environment mismatch")
        if manifest_body["implementation_candidate_commit"] != candidate_commit:
            raise DeliveryPackageError("candidate manifest implementation commit mismatch")
    package = build_package(
        artifacts,
        refs,
        candidate_commit=candidate_commit,
        candidate_tree_sha256=(manifest_body["production_tree_content_sha256"] if manifest_body else None),
        candidate_manifest_sha256=candidate_manifest_sha256,
        profile_id=profile_id,
        environment_id=environment_id,
    )
    return {
        "status": "PASS" if candidate_bound else "VALID_TEMPLATE_NOT_CANDIDATE_BOUND",
        "production_applied": False,
        "package": package,
        "package_sha256": hashlib.sha256(
            json.dumps(package, sort_keys=True, separators=(",", ":")).encode()
        ).hexdigest(),
    }


def expect_failure(label: str, action: Any, message: str) -> None:
    try:
        action()
    except DeliveryPackageError as error:
        if message not in str(error):
            raise AssertionError(f"{label} hit the wrong guard: {error}") from error
        return
    raise AssertionError(f"{label} did not fail")


def self_test() -> dict[str, Any]:
    baseline = validate_delivery_package()
    artifacts = {role: load_json(resolve_artifact(path)) for role, path in REQUIRED.items()}
    cross_candidate = copy.deepcopy(artifacts)
    cross_candidate["restore-plan"]["package_id"] = "m02-delivery-other-candidate"
    expect_failure("cross-package", lambda: validate_identity(cross_candidate), "identity conflicts")
    rollback_drift = copy.deepcopy(artifacts)
    rollback_drift["restore-plan"]["body"]["rollback_plan_path"] = REQUIRED["install-manifest"]
    expect_failure("rollback-reference", lambda: validate_steps(rollback_drift["restore-plan"], "restore-plan"), "canonical rollback")
    unordered = copy.deepcopy(artifacts["upgrade-plan"])
    unordered["body"]["steps"][0]["order"] = 2
    expect_failure("step-order", lambda: validate_steps(unordered, "upgrade-plan"), "not contiguous")
    return {
        "status": "PASS",
        "production_applied": False,
        "template_status": baseline["status"],
        "mutation_guards": ["cross-package", "rollback-reference", "step-order"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check-template", action="store_true")
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--candidate-commit")
    parser.add_argument("--candidate-manifest", type=Path)
    parser.add_argument("--candidate-manifest-sha256")
    parser.add_argument("--profile-id")
    parser.add_argument("--environment-id")
    args = parser.parse_args()
    if sum((args.check_template, args.self_test, args.candidate_commit is not None)) != 1:
        parser.error("select exactly one of --check-template, --self-test, or --candidate-commit")
    try:
        if args.self_test:
            result = self_test()
        elif args.check_template:
            result = validate_delivery_package()
        else:
            result = validate_delivery_package(
                candidate_commit=args.candidate_commit,
                candidate_manifest_path=args.candidate_manifest,
                candidate_manifest_sha256=args.candidate_manifest_sha256,
                profile_id=args.profile_id,
                environment_id=args.environment_id,
            )
        print(json.dumps(result, sort_keys=True, indent=2))
        return 0
    except (DeliveryPackageError, ValueError, OSError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "BLOCKED", "production_applied": False, "failure": str(error)}, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
