#!/usr/bin/env python3
"""Fail closed on M02 premerge and protected postmerge equivalence.

The verifier emits a typed result to stdout. It never writes evidence, moves a
release pointer, signs a receipt, or performs a merge.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

from build_topic1_task_registry import candidate_tree_fingerprint, read_candidate_blob, validate_against_schema
from trusted_signature_service import SignatureVerificationRequest
from validate_m02_delivery_package import (
    REQUIRED as DELIVERY_REQUIRED,
    validate_definition as validate_delivery_definition,
    validate_identity as validate_delivery_identity,
    validate_steps as validate_delivery_steps,
)
from validate_m02_external_activity_receipt_contract import validate_semantics as validate_external_receipt
from verify_trusted_signature import TrustedSignatureClientError, verify_exact_payload


ROOT = Path(__file__).resolve().parents[2]
CANDIDATE_SCHEMA = ROOT / "contracts/alignment/implementation-candidate.schema.json"
PROVENANCE_SCHEMA = ROOT / "contracts/alignment/candidate-artifact-provenance-receipt.schema.json"
EVIDENCE_SCHEMA = ROOT / "contracts/alignment/m02-promotion-evidence-index.schema.json"
TASK_INDEX_SCHEMA = ROOT / "contracts/alignment/task-current-evidence-index.schema.json"
INTENT_SCHEMA = ROOT / "contracts/alignment/promotion-intent.schema.json"
EXTERNAL_RECEIPT_SCHEMA = ROOT / "contracts/alignment/external-activity-receipt.schema.json"
RESULT_SCHEMA = ROOT / "contracts/alignment/m02-promotion-equivalence-result.schema.json"
EXPECTED_TASK_IDS = [f"T1-M02-N{number:03d}" for number in range(1, 16)]
PROM_ALLOWED_PATHS = ["contracts/releases/topic1/t1-m02-release-pointer.json"]
MERGE_AUTHORITY_ROLES = ("ACCEPTANCE_AUTHORITY",)
MERGE_AUTHORITY_SCOPES = ("CANDIDATE", "PROFILE", "ENVIRONMENT", "PURPOSE", "ACCEPTANCE")
PROVENANCE_AUTHORITY_ROLES = ("SUPPLY_CHAIN_OWNER",)
PROVENANCE_AUTHORITY_SCOPES = ("CANDIDATE", "PROFILE", "ENVIRONMENT", "PURPOSE", "SUPPLY_CHAIN")


class PromotionBlocked(ValueError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def parse_unique_json_bytes(payload: bytes, context: str) -> dict[str, Any]:
    def unique(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                raise PromotionBlocked("BLOCK_DUPLICATE_JSON_PROPERTY", f"{context}:{key}")
            result[key] = value
        return result

    try:
        body = json.loads(payload, object_pairs_hook=unique)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise PromotionBlocked("BLOCK_JSON_INVALID", context) from error
    if not isinstance(body, dict):
        raise PromotionBlocked("BLOCK_JSON_ROOT", context)
    return body


def load_unique_json(path: Path, schema: Path | None = None) -> tuple[dict[str, Any], bytes]:
    candidate = path if path.is_absolute() else ROOT / path
    try:
        resolved = candidate.resolve(strict=True)
    except OSError as error:
        raise PromotionBlocked("BLOCK_ARTIFACT_MISSING", str(path)) from error
    if (
        not resolved.is_relative_to(ROOT.resolve())
        or resolved != candidate
        or candidate.is_symlink()
        or not candidate.is_file()
    ):
        raise PromotionBlocked("BLOCK_ARTIFACT_PATH", str(path))

    payload = resolved.read_bytes()
    body = parse_unique_json_bytes(payload, str(path))
    if schema is not None:
        try:
            validate_against_schema(body, schema)
        except ValueError as error:
            raise PromotionBlocked("BLOCK_SCHEMA", f"{path}:{error}") from error
    return body, payload


def read_repo_artifact(path: str, *, code: str) -> bytes:
    relative = Path(path)
    if (
        relative.is_absolute()
        or not relative.parts
        or ".." in relative.parts
        or "\x00" in path
        or "\\" in path
        or relative.as_posix() != path
    ):
        raise PromotionBlocked(code, path)
    candidate = ROOT / relative
    try:
        resolved = candidate.resolve(strict=True)
    except OSError as error:
        raise PromotionBlocked(code, path) from error
    if (
        resolved != candidate
        or not resolved.is_relative_to(ROOT.resolve())
        or candidate.is_symlink()
        or not candidate.is_file()
    ):
        raise PromotionBlocked(code, path)
    return candidate.read_bytes()


def run_git(*arguments: str) -> bytes:
    completed = subprocess.run(
        ["git", *arguments], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if completed.returncode != 0:
        raise PromotionBlocked(
            "BLOCK_GIT_COMMAND", f"git {' '.join(arguments)}:{completed.stderr.decode(errors='replace')[:1000]}",
        )
    return completed.stdout


def validate_commit(commit: str) -> None:
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise PromotionBlocked("BLOCK_COMMIT_IDENTITY", commit)
    run_git("cat-file", "-e", f"{commit}^{{commit}}")


def validate_allowed_paths(paths: list[str]) -> None:
    if paths != PROM_ALLOWED_PATHS:
        raise PromotionBlocked("BLOCK_PROM_ALLOWED_PATH_EXACT_SET", repr(paths))
    for path in paths:
        relative = Path(path)
        if relative.is_absolute() or ".." in relative.parts or any(char in path for char in "*?[]\\"):
            raise PromotionBlocked("BLOCK_PROM_ALLOWED_PATH_CANONICAL", path)


def validate_candidate_provenance(
    refs: list[dict[str, Any]],
    request_paths: list[Path],
    *,
    candidate_commit: str,
    profile_id: str,
    environment_id: str,
    verifier_endpoint: str | None,
    policy_fingerprint: str | None,
) -> list[dict[str, str]]:
    if not refs:
        if request_paths:
            raise PromotionBlocked("BLOCK_PROVENANCE_REQUEST_EXTRA", repr([str(path) for path in request_paths]))
        return []
    if not verifier_endpoint or not policy_fingerprint or not re.fullmatch(r"[0-9a-f]{64}", policy_fingerprint):
        raise PromotionBlocked("BLOCK_PROVENANCE_VERIFIER_BINDING", "endpoint and policy fingerprint required")
    requests: dict[str, tuple[SignatureVerificationRequest, Path]] = {}
    for path in request_paths:
        body, _ = load_unique_json(path)
        try:
            request = SignatureVerificationRequest.from_dict(body)
        except ValueError as error:
            raise PromotionBlocked("BLOCK_PROVENANCE_REQUEST_SCHEMA", str(path)) from error
        subject_id = request.signed_payload.subject_id
        if subject_id in requests:
            raise PromotionBlocked("BLOCK_PROVENANCE_REQUEST_DUPLICATE", subject_id)
        requests[subject_id] = (request, path)
    receipts: list[tuple[dict[str, Any], bytes, dict[str, Any]]] = []
    for ref in refs:
        receipt, receipt_bytes = load_unique_json(Path(ref["provenance_receipt_path"]), PROVENANCE_SCHEMA)
        signed_core = {
            key: value
            for key, value in receipt.items()
            if key not in {
                "signed_payload_artifact", "signed_payload_sha256", "signature_artifacts", "verification",
            }
        }
        signed_bytes = read_repo_artifact(
            receipt["signed_payload_artifact"], code="BLOCK_PROVENANCE_SIGNED_PAYLOAD",
        )
        if (
            receipt["artifact_id"] != ref["artifact_id"]
            or receipt["artifact_role"] != ref["artifact_role"]
            or receipt["artifact_path"] != ref["path"]
            or receipt["artifact_sha256"] != ref["sha256"]
            or receipt["candidate_commit"] != candidate_commit
            or receipt["signed_payload_sha256"] != sha256_bytes(canonical_bytes(signed_core))
            or sha256_bytes(signed_bytes) != receipt["signed_payload_sha256"]
        ):
            raise PromotionBlocked("BLOCK_PROVENANCE_RECEIPT_IDENTITY", receipt["receipt_id"])
        for signature in receipt["signature_artifacts"]:
            if sha256_bytes(read_repo_artifact(
                signature["path"], code="BLOCK_PROVENANCE_SIGNATURE_ARTIFACT",
            )) != signature["sha256"]:
                raise PromotionBlocked("BLOCK_PROVENANCE_SIGNATURE_ARTIFACT", receipt["receipt_id"])
        receipts.append((receipt, signed_bytes, ref))
    receipt_ids = [item[0]["receipt_id"] for item in receipts]
    if len(receipt_ids) != len(set(receipt_ids)) or set(requests) != set(receipt_ids):
        raise PromotionBlocked(
            "BLOCK_PROVENANCE_REQUEST_EXACT_SET",
            f"receipts={sorted(receipt_ids)} requests={sorted(requests)}",
        )
    attestations: list[dict[str, str]] = []
    for receipt, signed_bytes, _ref in receipts:
        request, request_path = requests[receipt["receipt_id"]]
        signed = request.signed_payload
        if (
            signed.subject_type != "CANDIDATE_ARTIFACT"
            or signed.subject_id != receipt["receipt_id"]
            or signed.subject_payload.artifact_id != receipt["artifact_id"]
            or signed.subject_payload.content != signed_bytes
            or signed.subject_payload.content_sha256 != receipt["signed_payload_sha256"]
            or signed.subject_payload.size_bytes != len(signed_bytes)
            or signed.candidate_commit != candidate_commit
            or signed.profile_id != profile_id
            or signed.environment_id != environment_id
            or signed.purpose != "CANDIDATE_ARTIFACT_PROVENANCE"
            or tuple(signed.required_authority_roles) != PROVENANCE_AUTHORITY_ROLES
            or tuple(signed.required_scopes) != PROVENANCE_AUTHORITY_SCOPES
            or signed.policy_fingerprint_sha256 != policy_fingerprint
        ):
            raise PromotionBlocked("BLOCK_PROVENANCE_REQUEST_BINDING", receipt["receipt_id"])
        try:
            attestation = verify_exact_payload(
                request,
                endpoint=verifier_endpoint,
                policy_fingerprint=policy_fingerprint,
            )
        except TrustedSignatureClientError as error:
            raise PromotionBlocked("BLOCK_PROVENANCE_VERIFIER", receipt["receipt_id"]) from error
        if attestation.signer is None or "SUPPLY_CHAIN_OWNER" not in attestation.signer.verified_roles:
            raise PromotionBlocked("BLOCK_PROVENANCE_AUTHORITY_ROLE", receipt["receipt_id"])
        attestations.append({
            "receipt_id": receipt["receipt_id"],
            "request_path": str(request_path),
            "attestation_id": attestation.attestation_id,
            "signer_id": attestation.signer.signer_id,
        })
    return attestations


def validate_artifact_ref_shapes(
    refs: list[dict[str, Any]], expected_hashes: list[str], context: str,
) -> list[dict[str, Any]]:
    ids = [item["artifact_id"] for item in refs]
    paths = [item["path"] for item in refs]
    hashes = [item["sha256"] for item in refs]
    if (
        len(ids) != len(set(ids))
        or len(paths) != len(set(paths))
        or len(hashes) != len(set(hashes))
        or len(refs) != len(expected_hashes)
        or set(hashes) != set(expected_hashes)
    ):
        raise PromotionBlocked("BLOCK_CANDIDATE_ARTIFACT_EXACT_SET", context)
    trusted: list[dict[str, Any]] = []
    for item in refs:
        if item["source_kind"] == "CANDIDATE_GIT_BLOB":
            if item["provenance_receipt_path"] is not None or item["provenance_receipt_sha256"] is not None:
                raise PromotionBlocked("BLOCK_CANDIDATE_GIT_PROVENANCE", item["path"])
        elif item["source_kind"] == "TRUSTED_EXTERNAL_ARTIFACT":
            if not item["provenance_receipt_path"] or not item["provenance_receipt_sha256"]:
                raise PromotionBlocked("BLOCK_CANDIDATE_EXTERNAL_PROVENANCE", item["path"])
            trusted.append(item)
        else:
            raise PromotionBlocked("BLOCK_CANDIDATE_SOURCE_KIND", item["path"])
    return trusted


def validate_delivery_artifact_set(
    refs: list[dict[str, Any]], blobs: dict[str, bytes],
) -> None:
    expected_ids = list(DELIVERY_REQUIRED)
    expected_paths = list(DELIVERY_REQUIRED.values())
    if (
        [item["artifact_id"] for item in refs] != expected_ids
        or [item["path"] for item in refs] != expected_paths
        or set(blobs) != set(expected_ids)
    ):
        raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_EXACT_SET", "role/path/order")
    artifacts: dict[str, dict[str, Any]] = {}
    for item in refs:
        role = item["artifact_id"]
        body = blobs[role]
        if sha256_bytes(body) != item["sha256"]:
            raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_BLOB", item["path"])
        artifact = parse_unique_json_bytes(body, item["path"])
        try:
            validate_delivery_definition(artifact, "delivery_artifact")
            validate_delivery_steps(artifact, role)
        except ValueError as error:
            raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_SCHEMA", f"{role}:{error}") from error
        if artifact["artifact_role"] != role:
            raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_ROLE", role)
        artifacts[role] = artifact
    try:
        validate_delivery_identity(artifacts)
    except ValueError as error:
        raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_IDENTITY", str(error)) from error


def validate_candidate(
    path: Path,
    *,
    profile_id: str,
    provenance_request_paths: list[Path],
    verifier_endpoint: str | None,
    policy_fingerprint: str | None,
) -> tuple[dict[str, Any], str, list[dict[str, str]]]:
    candidate, payload = load_unique_json(path, CANDIDATE_SCHEMA)
    candidate_sha = sha256_bytes(payload)
    commit = candidate["implementation_candidate_commit"]
    validate_commit(commit)
    try:
        observed_tree = candidate_tree_fingerprint(
            ROOT, commit, candidate["source_roots"], candidate["excluded_paths"],
        )
    except ValueError as error:
        raise PromotionBlocked("BLOCK_CANDIDATE_TREE", str(error)) from error
    if observed_tree != candidate["production_tree_content_sha256"]:
        raise PromotionBlocked(
            "BLOCK_CANDIDATE_TREE_IDENTITY",
            f"declared={candidate['production_tree_content_sha256']} observed={observed_tree}",
        )
    trusted_external_refs: list[dict[str, Any]] = []
    artifact_groups = (
        ("config_schema_migration_artifacts", "config_schema_migration_hashes"),
        ("model_threshold_dataset_artifacts", "model_threshold_dataset_hashes"),
        ("supply_chain_artifacts", "supply_chain_artifact_hashes"),
        ("runtime_artifacts", "runtime_artifact_hashes"),
    )
    for ref_field, hash_field in artifact_groups:
        refs = candidate[ref_field]
        hashes = candidate[hash_field]
        trusted_external_refs.extend(validate_artifact_ref_shapes(refs, hashes, ref_field))
        for item in refs:
            if item["source_kind"] == "CANDIDATE_GIT_BLOB":
                blob = read_candidate_blob(ROOT, commit, item["path"])
                if blob is None or sha256_bytes(blob) != item["sha256"]:
                    raise PromotionBlocked("BLOCK_CANDIDATE_ARTIFACT_BLOB", item["path"])
            else:
                if sha256_bytes(read_repo_artifact(
                    item["path"], code="BLOCK_CANDIDATE_EXTERNAL_ARTIFACT",
                )) != item["sha256"]:
                    raise PromotionBlocked("BLOCK_CANDIDATE_EXTERNAL_ARTIFACT", item["path"])
                if sha256_bytes(read_repo_artifact(
                    item["provenance_receipt_path"], code="BLOCK_CANDIDATE_EXTERNAL_PROVENANCE",
                )) != item["provenance_receipt_sha256"]:
                    raise PromotionBlocked("BLOCK_CANDIDATE_EXTERNAL_PROVENANCE", item["path"])
    delivery_blobs: dict[str, bytes] = {}
    for item in candidate["delivery_artifacts"]:
        blob = read_candidate_blob(ROOT, commit, item["path"])
        if blob is None:
            raise PromotionBlocked("BLOCK_CANDIDATE_DELIVERY_BLOB", item["path"])
        delivery_blobs[item["artifact_id"]] = blob
    validate_delivery_artifact_set(candidate["delivery_artifacts"], delivery_blobs)
    image_digests = [item["image_digest"] for item in candidate["image_attestations"]]
    if image_digests != candidate["image_digests"] or len(image_digests) != len(set(image_digests)):
        raise PromotionBlocked("BLOCK_CANDIDATE_IMAGE_EXACT_SET", repr(image_digests))
    config_refs = {(item["path"], item["sha256"]): item for item in candidate["config_schema_migration_artifacts"]}
    supply_refs = {(item["path"], item["sha256"]): item for item in candidate["supply_chain_artifacts"]}
    for image in candidate["image_attestations"]:
        if image["deployed_image_digest"] != image["image_digest"]:
            raise PromotionBlocked("BLOCK_CANDIDATE_IMAGE_DEPLOYED_DIGEST", image["image_digest"])
        manifest_ref = config_refs.get((image["manifest_path"], image["manifest_sha256"]))
        manifest_blob = read_candidate_blob(ROOT, commit, image["manifest_path"])
        if (
            manifest_ref is None
            or manifest_ref["source_kind"] != "CANDIDATE_GIT_BLOB"
            or manifest_blob is None
            or sha256_bytes(manifest_blob) != image["manifest_sha256"]
        ):
            raise PromotionBlocked("BLOCK_CANDIDATE_IMAGE_MANIFEST", image["image_digest"])
        attestation_ref = supply_refs.get((image["attestation_path"], image["attestation_sha256"]))
        if (
            attestation_ref is None
            or attestation_ref["source_kind"] != "TRUSTED_EXTERNAL_ARTIFACT"
            or attestation_ref["artifact_role"]
            != f"image-provenance-attestation:{image['image_digest']}"
        ):
            raise PromotionBlocked("BLOCK_CANDIDATE_IMAGE_ATTESTATION", image["image_digest"])
    excluded_paths = {
        item["path"] for item in candidate["excluded_paths"] if item["referenced_by_active_build"]
    }
    prebuilt_paths = [item["path"] for item in candidate["external_or_prebuilt_artifacts"]]
    if len(prebuilt_paths) != len(set(prebuilt_paths)) or excluded_paths != set(prebuilt_paths):
        raise PromotionBlocked("BLOCK_CANDIDATE_PREBUILT_EXACT_SET", repr(prebuilt_paths))
    image_by_digest = {item["image_digest"]: item for item in candidate["image_attestations"]}
    for prebuilt in candidate["external_or_prebuilt_artifacts"]:
        if sha256_bytes(read_repo_artifact(
            prebuilt["path"], code="BLOCK_CANDIDATE_PREBUILT_BINARY",
        )) != prebuilt["binary_sha256"]:
            raise PromotionBlocked("BLOCK_CANDIDATE_PREBUILT_BINARY", prebuilt["path"])
        recipe = read_candidate_blob(ROOT, commit, prebuilt["build_recipe_path"])
        if recipe is None or sha256_bytes(recipe) != prebuilt["build_recipe_sha256"]:
            raise PromotionBlocked("BLOCK_CANDIDATE_PREBUILT_RECIPE", prebuilt["path"])
        if sha256_bytes(read_repo_artifact(
            prebuilt["sbom_or_attestation_path"], code="BLOCK_CANDIDATE_PREBUILT_SBOM",
        )) != prebuilt["sbom_or_attestation_sha256"]:
            raise PromotionBlocked("BLOCK_CANDIDATE_PREBUILT_SBOM", prebuilt["path"])
        image = image_by_digest.get(prebuilt["image_digest"])
        provenance = supply_refs.get((
            prebuilt["sbom_or_attestation_path"], prebuilt["sbom_or_attestation_sha256"],
        ))
        if (
            image is None
            or provenance is None
            or provenance["source_kind"] != "TRUSTED_EXTERNAL_ARTIFACT"
            or provenance["artifact_role"]
            != f"prebuilt-binary-provenance:{prebuilt['image_digest']}:{prebuilt['binary_sha256']}"
            or prebuilt["deployed_image_digest"] != image["deployed_image_digest"]
            or prebuilt["image_internal_binary_sha256"] != prebuilt["binary_sha256"]
        ):
            raise PromotionBlocked("BLOCK_CANDIDATE_PREBUILT_PROVENANCE", prebuilt["path"])
    attestations = validate_candidate_provenance(
        trusted_external_refs,
        provenance_request_paths,
        candidate_commit=commit,
        profile_id=profile_id,
        environment_id=candidate["environment_id"],
        verifier_endpoint=verifier_endpoint,
        policy_fingerprint=policy_fingerprint,
    )
    return candidate, candidate_sha, attestations


def validate_evidence_index(
    path: Path,
    *,
    candidate_sha: str,
    profile_id: str,
    environment_id: str,
    load_task_files: bool = True,
) -> tuple[dict[str, Any], str]:
    index, payload = load_unique_json(path, EVIDENCE_SCHEMA)
    validate_evidence_index_body(
        index,
        candidate_sha=candidate_sha,
        profile_id=profile_id,
        environment_id=environment_id,
    )
    refs = index["task_indexes"]
    if load_task_files:
        for item in refs:
            task_index, task_payload = load_unique_json(Path(item["path"]), TASK_INDEX_SCHEMA)
            if sha256_bytes(task_payload) != item["sha256"]:
                raise PromotionBlocked("BLOCK_TASK_INDEX_HASH", item["task_id"])
            if (
                task_index["task_id"] != item["task_id"]
                or task_index["milestone_id"] != "T1-M02"
                or task_index["candidate_manifest_sha256"] != candidate_sha
                or task_index["profile_id"] != profile_id
                or task_index["environment_id"] != environment_id
                or task_index["status"] != "PASS"
            ):
                raise PromotionBlocked("BLOCK_TASK_INDEX_IDENTITY", item["task_id"])
    return index, sha256_bytes(payload)


def validate_evidence_index_body(
    index: dict[str, Any],
    *,
    candidate_sha: str,
    profile_id: str,
    environment_id: str,
) -> None:
    expected_paths = [
        f"doc/02_acceptance/topic1/tasks/t1-m02-n{number:03d}/current-evidence-index.json"
        for number in range(1, 16)
    ]
    if (
        index["candidate_manifest_sha256"] != candidate_sha
        or index["profile_id"] != profile_id
        or index["environment_id"] != environment_id
    ):
        raise PromotionBlocked("BLOCK_EVIDENCE_IDENTITY", index["index_id"])
    refs = index["task_indexes"]
    if [item["task_id"] for item in refs] != EXPECTED_TASK_IDS or [item["path"] for item in refs] != expected_paths:
        raise PromotionBlocked("BLOCK_EVIDENCE_TASK_EXACT_SET", repr([item["task_id"] for item in refs]))
    active_hashes = {item["sha256"] for item in refs}
    stale = set(index["stale_index_sha256s"])
    superseded = set(index["superseded_index_sha256s"])
    if active_hashes & (stale | superseded) or stale & superseded:
        raise PromotionBlocked("BLOCK_EVIDENCE_STATE_OVERLAP", "active stale superseded overlap")
    exclusion_hashes = {item["sha256"] for item in index["exclusions"]}
    if exclusion_hashes != stale | superseded:
        raise PromotionBlocked("BLOCK_EVIDENCE_EXCLUSION_EXACT_SET", repr(sorted(exclusion_hashes)))


def tree_map(commit: str, roots: list[str] | None = None, excluded: set[str] | None = None) -> dict[str, dict[str, str]]:
    arguments = ["ls-tree", "-r", "-z", "--full-tree", commit]
    if roots:
        arguments.extend(["--", *roots])
    records = run_git(*arguments)
    result: dict[str, dict[str, str]] = {}
    for record in records.split(b"\0"):
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        mode, object_type, _ = metadata.decode("ascii").split(" ")
        path = raw_path.decode("utf-8")
        if object_type != "blob" or any(path == item or path.startswith(item + "/") for item in excluded or set()):
            continue
        blob = read_candidate_blob(ROOT, commit, path)
        if blob is None:
            raise PromotionBlocked("BLOCK_TREE_ENTRY", path)
        result[path] = {"mode": mode, "blob_sha256": sha256_bytes(blob)}
    return result


def tree_fingerprint(entries: dict[str, dict[str, str]], omitted: set[str] | None = None) -> str:
    values = [
        {"path": path, **identity}
        for path, identity in entries.items()
        if path not in (omitted or set())
    ]
    if not values:
        raise PromotionBlocked("BLOCK_TREE_EMPTY", "no production entries")
    return sha256_bytes(canonical_bytes(sorted(values, key=lambda item: item["path"])))


def tree_diff(before: dict[str, dict[str, str]], after: dict[str, dict[str, str]]) -> tuple[list[dict[str, Any]], str]:
    changes = [
        {"path": path, "before": before.get(path), "after": after.get(path)}
        for path in sorted(set(before) | set(after))
        if before.get(path) != after.get(path)
    ]
    return changes, sha256_bytes(canonical_bytes(changes))


def validate_merge_authorization(
    receipt_bytes: bytes,
    receipt: dict[str, Any],
    request_path: Path,
    *,
    endpoint: str,
    policy_fingerprint: str,
    candidate_commit: str,
    profile_id: str,
    environment_id: str,
) -> dict[str, str]:
    request_body, _ = load_unique_json(request_path)
    try:
        request = SignatureVerificationRequest.from_dict(request_body)
    except ValueError as error:
        raise PromotionBlocked("BLOCK_MERGE_VERIFICATION_REQUEST", str(error)) from error
    signed = request.signed_payload
    if (
        signed.subject_type != "EXTERNAL_ACTIVITY"
        or signed.subject_id != receipt["instance_id"]
        or signed.subject_payload.content != receipt_bytes
        or signed.subject_payload.content_sha256 != sha256_bytes(receipt_bytes)
        or signed.subject_payload.size_bytes != len(receipt_bytes)
        or signed.candidate_commit != candidate_commit
        or signed.profile_id != profile_id
        or signed.environment_id != environment_id
        or signed.purpose != "EXTERNAL_ACTIVITY_RECEIPT"
        or tuple(signed.required_authority_roles) != MERGE_AUTHORITY_ROLES
        or tuple(signed.required_scopes) != MERGE_AUTHORITY_SCOPES
        or signed.policy_fingerprint_sha256 != policy_fingerprint
    ):
        raise PromotionBlocked("BLOCK_MERGE_AUTHORIZATION_BINDING", receipt["instance_id"])
    try:
        attestation = verify_exact_payload(
            request, endpoint=endpoint, policy_fingerprint=policy_fingerprint,
        )
    except TrustedSignatureClientError as error:
        raise PromotionBlocked("BLOCK_MERGE_AUTHORIZATION_VERIFIER", str(error)) from error
    if attestation.signer is None or "ACCEPTANCE_AUTHORITY" not in attestation.signer.verified_roles:
        raise PromotionBlocked("BLOCK_MERGE_AUTHORIZATION_ROLE", receipt["instance_id"])
    return {"attestation_id": attestation.attestation_id, "signer_id": attestation.signer.signer_id}


def validate_promotion_intent_identity(
    intent: dict[str, Any], *, candidate_sha: str, candidate_commit: str,
    evidence_sha: str, profile_id: str,
) -> None:
    if (
        intent["candidate_manifest_sha256"] != candidate_sha
        or intent["promotion_commit_parent"] != candidate_commit
        or intent["profile_id"] != profile_id
        or intent["current_idx_manifest_sha256"] != evidence_sha
    ):
        raise PromotionBlocked("BLOCK_PROMOTION_INTENT_IDENTITY", intent["promotion_id"])
    validate_allowed_paths(intent["allowed_paths"])


def base_result(
    candidate: dict[str, Any],
    candidate_sha: str,
    evidence_sha: str,
    mode: str,
    provenance_attestations: list[dict[str, str]],
) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "artifact_kind": "M02_PROMOTION_EQUIVALENCE_RESULT",
        "mode": mode,
        "status": "PASS",
        "candidate_manifest_sha256": candidate_sha,
        "candidate_commit": candidate["implementation_candidate_commit"],
        "candidate_tree_sha256": candidate["production_tree_content_sha256"],
        "profile_id": candidate.get("profile_id", "T1_M02_MILESTONE_PROFILE"),
        "environment_id": candidate["environment_id"],
        "evidence_index_sha256": evidence_sha,
        "allowed_paths": PROM_ALLOWED_PATHS,
        "promotion_intent_sha256": None,
        "premerge_result_sha256": None,
        "merge_receipt_sha256": None,
        "merge_commit": None,
        "merge_tree_sha256": None,
        "allowed_path_diff_sha256": None,
        "provenance_attestations": provenance_attestations,
        "merge_attestation": None,
        "checks": [],
    }


def premerge(
    candidate_path: Path,
    evidence_path: Path,
    profile_id: str,
    *,
    provenance_request_paths: list[Path],
    verifier_endpoint: str | None,
    policy_fingerprint: str | None,
) -> dict[str, Any]:
    validate_allowed_paths(PROM_ALLOWED_PATHS)
    if profile_id != "T1_M02_MILESTONE_PROFILE":
        raise PromotionBlocked("BLOCK_PROFILE_IDENTITY", profile_id)
    candidate, candidate_sha, provenance = validate_candidate(
        candidate_path,
        profile_id=profile_id,
        provenance_request_paths=provenance_request_paths,
        verifier_endpoint=verifier_endpoint,
        policy_fingerprint=policy_fingerprint,
    )
    _, evidence_sha = validate_evidence_index(
        evidence_path,
        candidate_sha=candidate_sha,
        profile_id=profile_id,
        environment_id=candidate["environment_id"],
    )
    result = base_result(candidate, candidate_sha, evidence_sha, "PREMERGE", provenance)
    result["checks"] = [
        "candidate_schema", "candidate_commit", "candidate_production_tree",
        "candidate_artifact_sets", "candidate_image_digests", "exact_n001_n015_current_indexes",
        "same_candidate_profile_environment", "prom_allowed_path_exact_set",
        f"protected_provenance_attestation_count:{len(provenance)}",
    ]
    validate_against_schema(result, RESULT_SCHEMA)
    return result


def postmerge(
    candidate_path: Path,
    evidence_path: Path,
    intent_path: Path,
    premerge_path: Path,
    receipt_path: Path,
    verification_request_path: Path,
    *,
    provenance_request_paths: list[Path],
    verifier_endpoint: str,
    policy_fingerprint: str,
) -> dict[str, Any]:
    profile_id = "T1_M02_MILESTONE_PROFILE"
    candidate, candidate_sha, provenance = validate_candidate(
        candidate_path,
        profile_id=profile_id,
        provenance_request_paths=provenance_request_paths,
        verifier_endpoint=verifier_endpoint,
        policy_fingerprint=policy_fingerprint,
    )
    _, evidence_sha = validate_evidence_index(
        evidence_path, candidate_sha=candidate_sha, profile_id=profile_id,
        environment_id=candidate["environment_id"],
    )
    intent, intent_bytes = load_unique_json(intent_path, INTENT_SCHEMA)
    premerge_result, premerge_bytes = load_unique_json(premerge_path, RESULT_SCHEMA)
    receipt, receipt_bytes = load_unique_json(receipt_path, EXTERNAL_RECEIPT_SCHEMA)
    validate_external_receipt(receipt)
    intent_sha = sha256_bytes(intent_bytes)
    premerge_sha = sha256_bytes(premerge_bytes)
    receipt_sha = sha256_bytes(receipt_bytes)
    validate_promotion_intent_identity(
        intent,
        candidate_sha=candidate_sha,
        candidate_commit=candidate["implementation_candidate_commit"],
        evidence_sha=evidence_sha,
        profile_id=profile_id,
    )
    if (
        premerge_result["mode"] != "PREMERGE"
        or premerge_result["candidate_manifest_sha256"] != candidate_sha
        or premerge_result["evidence_index_sha256"] != evidence_sha
        or premerge_result["allowed_paths"] != PROM_ALLOWED_PATHS
    ):
        raise PromotionBlocked("BLOCK_PREMERGE_RESULT_IDENTITY", premerge_sha)
    if (
        receipt["activity_id"] != "EXT-T1-M02-N016-MERGE"
        or receipt["activity_type"] != "PROTECTED_MERGE"
        or receipt["candidate_manifest_sha256"] != candidate_sha
        or receipt["profile_id"] != profile_id
        or receipt["result"] != "PASS"
        or receipt["signature_verification"]["status"] != "PASS"
    ):
        raise PromotionBlocked("BLOCK_MERGE_RECEIPT_IDENTITY", receipt["instance_id"])
    authorization = validate_merge_authorization(
        receipt_bytes, receipt, verification_request_path,
        endpoint=verifier_endpoint, policy_fingerprint=policy_fingerprint,
        candidate_commit=candidate["implementation_candidate_commit"],
        profile_id=profile_id, environment_id=candidate["environment_id"],
    )
    payload = receipt["activity_payload"]
    merge_commit = payload["merge_commit"]
    validate_commit(merge_commit)
    excluded = {item["path"] for item in candidate["excluded_paths"]}
    candidate_production = tree_map(candidate["implementation_candidate_commit"], candidate["source_roots"], excluded)
    merge_production = tree_map(merge_commit, candidate["source_roots"], excluded)
    candidate_full = tree_fingerprint(candidate_production)
    merge_full = tree_fingerprint(merge_production)
    if candidate_full != candidate["production_tree_content_sha256"]:
        raise PromotionBlocked("BLOCK_CANDIDATE_TREE_RECOMPUTE", candidate_full)
    if tree_fingerprint(candidate_production, set(PROM_ALLOWED_PATHS)) != tree_fingerprint(merge_production, set(PROM_ALLOWED_PATHS)):
        raise PromotionBlocked("BLOCK_POSTMERGE_PRODUCTION_EQUIVALENCE", merge_commit)
    candidate_all = tree_map(candidate["implementation_candidate_commit"])
    merge_all = tree_map(merge_commit)
    changes, diff_sha = tree_diff(candidate_all, merge_all)
    changed_paths = {item["path"] for item in changes}
    if not changed_paths.issubset(set(PROM_ALLOWED_PATHS)):
        raise PromotionBlocked("BLOCK_POSTMERGE_ALLOWED_PATH_DIFF", repr(sorted(changed_paths)))
    if (
        payload["implementation_commit"] != candidate["implementation_candidate_commit"]
        or payload["implementation_tree_sha256"] != candidate_full
        or payload["merge_tree_sha256"] != merge_full
        or payload["allowed_path_diff_sha256"] != diff_sha
        or payload["premerge_run_sha256"] != premerge_sha
        or payload["promotion_intent_sha256"] != intent_sha
    ):
        raise PromotionBlocked("BLOCK_MERGE_RECEIPT_RECOMPUTED_IDENTITY", receipt["instance_id"])
    result = base_result(candidate, candidate_sha, evidence_sha, "POSTMERGE", provenance)
    result.update({
        "promotion_intent_sha256": intent_sha,
        "premerge_result_sha256": premerge_sha,
        "merge_receipt_sha256": receipt_sha,
        "merge_commit": merge_commit,
        "merge_tree_sha256": merge_full,
        "allowed_path_diff_sha256": diff_sha,
        "merge_attestation": authorization,
        "checks": [
            "candidate_schema", "candidate_production_tree", "exact_n001_n015_current_indexes",
            "promotion_intent_identity", "premerge_result_identity", "protected_merge_authorization",
            "merge_tree_recomputed", "production_content_equivalence", "prom_allowed_path_diff",
            f"merge_attestation:{authorization['attestation_id']}",
            f"protected_provenance_attestation_count:{len(provenance)}",
        ],
    })
    validate_against_schema(result, RESULT_SCHEMA)
    return result


def expect_block(code: str, action: Any) -> None:
    try:
        action()
    except PromotionBlocked as error:
        if error.code != code:
            raise AssertionError(f"expected {code}, observed {error.code}") from error
        return
    raise AssertionError(f"mutation did not block: {code}")


def self_test() -> dict[str, Any]:
    validate_allowed_paths(PROM_ALLOWED_PATHS)
    expect_block(
        "BLOCK_PROM_ALLOWED_PATH_EXACT_SET",
        lambda: validate_allowed_paths(["go/control-plane/cmd/ingest-gateway/main.go"]),
    )
    sample = {
        "candidate_manifest_sha256": "a" * 64,
        "profile_id": "T1_M02_MILESTONE_PROFILE",
        "environment_id": "selftest-env",
        "index_id": "m02-promotion-evidence-selftest",
        "task_indexes": [
            {
                "task_id": task_id,
                "path": f"doc/02_acceptance/topic1/tasks/t1-m02-n{number:03d}/current-evidence-index.json",
                "sha256": f"{number:064x}",
            }
            for number, task_id in enumerate(EXPECTED_TASK_IDS, start=1)
        ],
        "stale_index_sha256s": [],
        "superseded_index_sha256s": [],
        "exclusions": [],
    }
    drift = copy.deepcopy(sample)
    drift["task_indexes"].pop()
    validate_evidence_index_body(
        sample,
        candidate_sha="a" * 64,
        profile_id="T1_M02_MILESTONE_PROFILE",
        environment_id="selftest-env",
    )
    expect_block(
        "BLOCK_EVIDENCE_TASK_EXACT_SET",
        lambda: validate_evidence_index_body(
            drift,
            candidate_sha="a" * 64,
            profile_id="T1_M02_MILESTONE_PROFILE",
            environment_id="selftest-env",
        ),
    )
    stale_overlap = copy.deepcopy(sample)
    stale_overlap["stale_index_sha256s"] = [sample["task_indexes"][0]["sha256"]]
    stale_overlap["exclusions"] = [{
        "path": "doc/02_acceptance/topic1/tasks/t1-m02-n001/old-current-evidence-index.json",
        "sha256": sample["task_indexes"][0]["sha256"],
        "reason": "STALE",
    }]
    expect_block(
        "BLOCK_EVIDENCE_STATE_OVERLAP",
        lambda: validate_evidence_index_body(
            stale_overlap,
            candidate_sha="a" * 64,
            profile_id="T1_M02_MILESTONE_PROFILE",
            environment_id="selftest-env",
        ),
    )
    before = {"go/a": {"mode": "100644", "blob_sha256": "a" * 64}}
    after = copy.deepcopy(before)
    changes, diff_sha = tree_diff(before, after)
    if changes or diff_sha != sha256_bytes(canonical_bytes([])):
        raise AssertionError("empty tree diff is not deterministic")
    git_ref = {
        "artifact_id": "config-a", "artifact_role": "config", "source_kind": "CANDIDATE_GIT_BLOB",
        "path": "deployments/config-a.json", "sha256": "1" * 64,
        "provenance_receipt_path": None, "provenance_receipt_sha256": None,
    }
    validate_artifact_ref_shapes([git_ref], ["1" * 64], "selftest")
    git_with_receipt = copy.deepcopy(git_ref)
    git_with_receipt.update({"provenance_receipt_path": "receipt.json", "provenance_receipt_sha256": "2" * 64})
    expect_block(
        "BLOCK_CANDIDATE_GIT_PROVENANCE",
        lambda: validate_artifact_ref_shapes([git_with_receipt], ["1" * 64], "selftest"),
    )
    delivery_refs: list[dict[str, Any]] = []
    delivery_blobs: dict[str, bytes] = {}
    for role, relative in DELIVERY_REQUIRED.items():
        body = (ROOT / relative).read_bytes()
        delivery_refs.append({"artifact_id": role, "path": relative, "sha256": sha256_bytes(body)})
        delivery_blobs[role] = body
    validate_delivery_artifact_set(delivery_refs, delivery_blobs)
    role_drift = copy.deepcopy(delivery_refs)
    role_drift[0]["path"] = DELIVERY_REQUIRED["preflight-plan"]
    expect_block(
        "BLOCK_CANDIDATE_DELIVERY_EXACT_SET",
        lambda: validate_delivery_artifact_set(role_drift, delivery_blobs),
    )
    intent = {
        "promotion_id": "m02-selftest", "candidate_manifest_sha256": "a" * 64,
        "promotion_commit_parent": "b" * 40, "profile_id": "T1_M02_MILESTONE_PROFILE",
        "current_idx_manifest_sha256": "c" * 64, "allowed_paths": PROM_ALLOWED_PATHS,
    }
    validate_promotion_intent_identity(
        intent, candidate_sha="a" * 64, candidate_commit="b" * 40,
        evidence_sha="c" * 64, profile_id="T1_M02_MILESTONE_PROFILE",
    )
    parent_drift = copy.deepcopy(intent)
    parent_drift["promotion_commit_parent"] = "d" * 40
    expect_block(
        "BLOCK_PROMOTION_INTENT_IDENTITY",
        lambda: validate_promotion_intent_identity(
            parent_drift, candidate_sha="a" * 64, candidate_commit="b" * 40,
            evidence_sha="c" * 64, profile_id="T1_M02_MILESTONE_PROFILE",
        ),
    )
    return {
        "status": "PASS",
        "production_applied": False,
        "mutation_guards": [
            "production-path-in-PROM", "missing-task-index", "active-stale-overlap",
            "candidate-git-provenance", "delivery-role-path-drift", "promotion-parent-drift",
        ],
        "allowed_paths": PROM_ALLOWED_PATHS,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["premerge", "postmerge"])
    parser.add_argument("--candidate-manifest", type=Path)
    parser.add_argument("--evidence-index", type=Path)
    parser.add_argument("--profile-id", default="T1_M02_MILESTONE_PROFILE")
    parser.add_argument("--promotion-intent", type=Path)
    parser.add_argument("--premerge-result", type=Path)
    parser.add_argument("--merge-receipt", type=Path)
    parser.add_argument("--merge-verification-request", type=Path)
    parser.add_argument("--provenance-verification-request", type=Path, action="append", default=[])
    parser.add_argument("--verifier-endpoint")
    parser.add_argument("--policy-fingerprint")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    try:
        if args.self_test:
            if args.mode or args.candidate_manifest or args.evidence_index:
                parser.error("--self-test does not accept execution inputs")
            result = self_test()
        elif args.mode == "premerge":
            if not args.candidate_manifest or not args.evidence_index:
                parser.error("premerge requires --candidate-manifest and --evidence-index")
            result = premerge(
                args.candidate_manifest,
                args.evidence_index,
                args.profile_id,
                provenance_request_paths=args.provenance_verification_request,
                verifier_endpoint=args.verifier_endpoint,
                policy_fingerprint=args.policy_fingerprint,
            )
        elif args.mode == "postmerge":
            required = (
                args.candidate_manifest, args.evidence_index, args.promotion_intent,
                args.premerge_result, args.merge_receipt, args.merge_verification_request,
                args.verifier_endpoint, args.policy_fingerprint,
            )
            if not all(required):
                parser.error("postmerge requires candidate evidence intent premerge merge receipt and protected verifier inputs")
            result = postmerge(
                args.candidate_manifest, args.evidence_index, args.promotion_intent,
                args.premerge_result, args.merge_receipt, args.merge_verification_request,
                provenance_request_paths=args.provenance_verification_request,
                verifier_endpoint=args.verifier_endpoint, policy_fingerprint=args.policy_fingerprint,
            )
        else:
            parser.error("select --self-test or --mode")
        print(json.dumps(result, sort_keys=True, indent=2))
        return 0
    except (PromotionBlocked, ValueError, OSError, json.JSONDecodeError) as error:
        payload = {
            "status": "BLOCKED",
            "production_applied": False,
            "failure": str(error),
        }
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
