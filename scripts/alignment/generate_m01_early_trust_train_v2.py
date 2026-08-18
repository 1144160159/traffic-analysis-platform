#!/usr/bin/env python3
"""Generate the function-complete M01 early-trust v2 preview.

V2 freezes the complete v1 candidate, appends P067-P096, supersedes only the
three over-broad v1 runtime/evidence leaves, and records the external image
build/sign activity.  It does not implement, build, sign, deploy, execute or
switch any active registry.
"""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from collections import defaultdict
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
BASE = REPO / "contracts/alignment/m01-early-trust-train-catalog.v1.json"
BASE_SCHEMA = REPO / "contracts/alignment/m01-early-trust-train-catalog.schema.json"
BASE_ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v1.json"
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
ALLOCATION_SCHEMA = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
SCHEMA = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.schema.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证列车函数完备候选目录v2.md"

EXPECTED_BASE_SHA256 = "051f0945243c9e842614580e8cabb34c7dd6a158d8e284dc600dd7efe8f85827"
EXPECTED_BASE_ALLOCATION_SHA256 = "9bab6c55cf3e2ea44e1d5037ce9c37267ac804d0422d703d77f1a7fe78501d19"
EXPECTED_BASE_PROJECTION_SHA256 = "cf974c556104d4c721e316eccc749023da34d9afef7e7adfc0d816e1d96202f6"
EXPECTED_SEMANTIC_SHA256 = "fdf73ba06cd597d61e02992c341d95c086b2b8639de1b5f78affeb3435e39975"
ACTIVE_CATALOGS = [
    "contracts/alignment/task-registry.v1.json",
    "contracts/alignment/developer-claim-package-catalog.v1.json",
    "contracts/alignment/pr-design-application-catalog.v1.json",
    "contracts/alignment/task-execution-overlay.template.v1.json",
]
EXTERNAL = "EXT-T1-M01-N015-VERIFIER-IMAGE-BUILD-AND-SIGN"
FORMAL_STATUS = "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_EXTERNAL_IMAGE_RECEIPT_AND_SIGNED_OVERLAY"
SUPERSEDED = {
    "T1-M01-P063-OPS-n015-s7": ["T1-M01-P083-OPS-n015-s26"],
    "T1-M01-P064-TST-PRE-n015-s8": ["T1-M01-P080-TST-PRE-n015-s23", "T1-M01-P081-TST-PRE-n015-s24"],
    "T1-M01-P065-TST-POST-n015-s9": ["T1-M01-P084-TST-POST-n015-s27", "T1-M01-P085-TST-POST-n015-s28"],
}


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def digest(value: Any) -> str:
    return hashlib.sha256(canonical_bytes(value)).hexdigest()


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def number(atomic_id: str) -> int:
    match = re.search(r"-P([0-9]{3})-", atomic_id)
    if not match:
        raise ValueError(f"invalid atomic ID: {atomic_id}")
    return int(match.group(1))


def step(
    atomic_id: str, phase: str, locator: str, dependency: str, gate: str,
    design_kind: str, outcome: str, *, target: str = "PLANNED",
    companions: list[str] | None = None,
) -> dict[str, Any]:
    bits = atomic_id.split("-")
    pr_type = bits[3] if bits[3] != "TST" else "-".join(bits[3:5])
    return {
        "atomic_pr_id": atomic_id, "pr_type": pr_type, "phase": phase,
        "primary_locator": locator, "companion_locators": companions or [],
        "target_state": target, "prerequisite_ids": [dependency],
        "required_gates": [gate], "design_kind": design_kind,
        "single_outcome": outcome,
        "oracle_and_rollback": (
            "fail closed on identity policy trust time scope or payload drift; restore the prior hard-block path "
            "and preserve immutable failures receipts and externally produced artifacts"
        ),
    }


def build_steps() -> list[dict[str, Any]]:
    rows = [
        step("T1-M01-P067-REF-n015-s10", "candidate-repository-context", "scripts/alignment/build_topic1_task_registry.py#resolve_candidate_repository", "T1-M01-P060-REF-n015-s4", "G0", "FUNCTION", "resolve one immutable canonical repository root without ambient global fallback"),
        step("T1-M01-P068-REF-n015-s11", "candidate-blob-reader", "scripts/alignment/build_topic1_task_registry.py#read_candidate_blob", "T1-M01-P067-REF-n015-s10", "G0", "FUNCTION", "read one candidate Git blob from the explicit repository root with typed absent versus command failure"),
        step("T1-M01-P069-REF-n015-s12", "hashed-artifact-loader", "scripts/alignment/build_topic1_task_registry.py#load_hashed_json_artifact", "T1-M01-P068-REF-n015-s11", "G0", "FUNCTION", "load one safe repository-scoped JSON artifact using explicit root exact hash and optional schema"),
        step("T1-M01-P070-REF-n015-s13", "hashed-artifact-validator", "scripts/alignment/build_topic1_task_registry.py#validate_hashed_artifact", "T1-M01-P069-REF-n015-s12", "G0", "FUNCTION", "validate one safe explicit-root artifact path and exact digest without following repository escapes"),
        step("T1-M01-P071-REF-n015-s14", "candidate-tree-fingerprint", "scripts/alignment/build_topic1_task_registry.py#candidate_tree_fingerprint", "T1-M01-P070-REF-n015-s13", "G0", "FUNCTION", "derive the production tree fingerprint only from the explicit repository and exact candidate commit"),
        step("T1-M01-P072-REF-n015-s15", "trusted-request-canonicalization", "scripts/alignment/trusted_signature_service.py#canonicalize_verification_request", "T1-M01-P062-REF-n015-s6", "G0", "FUNCTION", "canonicalize one typed request while excluding no signed identity field"),
        step("T1-M01-P073-REF-n015-s16", "trust-policy-loader", "scripts/alignment/trusted_signature_service.py#load_trust_policy", "T1-M01-P072-REF-n015-s15", "G0", "FUNCTION", "load the pinned policy and trust-anchor references without loading secret values into output"),
        step("T1-M01-P074-REF-n015-s17", "certificate-chain-verifier", "scripts/alignment/trusted_signature_service.py#verify_certificate_chain", "T1-M01-P073-REF-n015-s16", "G0", "FUNCTION", "verify chain algorithm EKU validity and revocation state against the pinned policy"),
        step("T1-M01-P075-REF-n015-s18", "signature-verifier", "scripts/alignment/trusted_signature_service.py#verify_detached_signature", "T1-M01-P074-REF-n015-s17", "G0", "FUNCTION", "verify the detached signature over the exact canonical payload and reject algorithm downgrade"),
        step("T1-M01-P076-REF-n015-s19", "authority-scope-verifier", "scripts/alignment/trusted_signature_service.py#verify_authority_scope", "T1-M01-P075-REF-n015-s18", "G0", "FUNCTION", "verify required role purpose candidate profile environment time window and optional CNAS scope"),
        step("T1-M01-P077-REF-n015-s20", "attestation-replay-guard", "scripts/alignment/trusted_signature_service.py#claim_attestation_nonce", "T1-M01-P076-REF-n015-s19", "G0", "FUNCTION", "atomically claim one policy-scoped attestation nonce or return its exact replay receipt"),
        step("T1-M01-P078-REF-n015-s21", "verification-endpoint", "scripts/alignment/trusted_signature_service.py#verify_request", "T1-M01-P077-REF-n015-s20", "G0", "FUNCTION", "orchestrate exact payload chain signature role scope and replay checks into one typed attestation"),
        step("T1-M01-P079-OPS-n015-s22", "verifier-image-package", "deployments/security/topic1-trusted-signature-verifier.Dockerfile#$DOCUMENT", "T1-M01-P078-REF-n015-s21", "G0", "DEPLOYMENT_PACKAGE", "define a non-root digest-pinnable verifier image package with no embedded trust anchors or secrets", companions=["deployments/security/topic1-trusted-signature-verifier.requirements.lock#$DOCUMENT"]),
        step("T1-M01-P080-TST-PRE-n015-s23", "trusted-signature-g0", "doc/02_acceptance/topic1/work-orders/t1-m01-p080-tst-pre-n015-s23/test-result.json", "T1-M01-P079-OPS-n015-s22", "G0", "EVIDENCE_OUTPUT", "record the exact static unit schema and ten-case fail-closed matrix for one candidate", target="PLANNED_OUTPUT", companions=["doc/02_acceptance/topic1/work-orders/t1-m01-p080-tst-pre-n015-s23/case-report.json"]),
        step("T1-M01-P081-TST-PRE-n015-s24", "trusted-signature-g1", "doc/02_acceptance/topic1/work-orders/t1-m01-p081-tst-pre-n015-s24/test-result.json", "T1-M01-P080-TST-PRE-n015-s23", "G1", "EVIDENCE_OUTPUT", "record isolated restart policy rotation expiry revocation and rollback compatibility evidence", target="PLANNED_OUTPUT", companions=["doc/02_acceptance/topic1/work-orders/t1-m01-p081-tst-pre-n015-s24/case-report.json"]),
        step("T1-M01-P082-OPS-n015-s25", "verifier-deployment-manifest", "deployments/security/topic1-trusted-signature-verifier.yaml#/$DOCUMENT", EXTERNAL, "G0", "DEPLOYMENT_MANIFEST", "materialize a default-off digest-pinned deployment with secret references NetworkPolicy and rollback controls"),
        step("T1-M01-P083-OPS-n015-s26", "verifier-deployment-g6", "doc/02_acceptance/topic1/work-orders/t1-m01-p083-ops-n015-s26/test-result.json", "T1-M01-P082-OPS-n015-s25", "G6", "EVIDENCE_OUTPUT", "record canary deployment health stop thresholds hard-block rollback and observation evidence", target="PLANNED_OUTPUT", companions=["doc/02_acceptance/topic1/work-orders/t1-m01-p083-ops-n015-s26/case-report.json"]),
        step("T1-M01-P084-TST-POST-n015-s27", "trusted-signature-g2", "doc/02_acceptance/topic1/work-orders/t1-m01-p084-tst-post-n015-s27/test-result.json", "T1-M01-P083-OPS-n015-s26", "G2", "EVIDENCE_OUTPUT", "record protected endpoint functional and fault-path verification for one candidate and environment", target="PLANNED_OUTPUT", companions=["doc/02_acceptance/topic1/work-orders/t1-m01-p084-tst-post-n015-s27/case-report.json"]),
        step("T1-M01-P085-TST-POST-n015-s28", "trusted-signature-g3", "doc/02_acceptance/topic1/work-orders/t1-m01-p085-tst-post-n015-s28/test-result.json", "T1-M01-P084-TST-POST-n015-s27", "G3", "EVIDENCE_OUTPUT", "record request attestation replay-ledger and caller-result reconciliation with unexplained diff zero", target="PLANNED_OUTPUT", companions=["doc/02_acceptance/topic1/work-orders/t1-m01-p085-tst-post-n015-s28/case-report.json"]),
        step("T1-M01-P086-WRT-n015-s29", "trusted-runtime-types", "scripts/alignment/trusted_signature_service.py#SignatureVerificationRuntime", "T1-M01-P078-REF-n015-s21", "G0", "TYPE_CONTRACT", "define one immutable runtime aggregate for policy secret replay ledger audit and server dependencies"),
        step("T1-M01-P087-REF-n015-s30", "trusted-secret-resolver", "scripts/alignment/trusted_signature_service.py#resolve_secret_handle", "T1-M01-P086-WRT-n015-s29", "G0", "FUNCTION", "resolve one allowlisted secret reference into an opaque handle without serializing its value"),
        step("T1-M01-P088-REF-n015-s31", "trusted-replay-store", "scripts/alignment/trusted_signature_service.py#open_replay_store", "T1-M01-P087-REF-n015-s30", "G0", "FUNCTION", "open one persistent exclusive atomic replay store and reject unsafe or incompatible state"),
        step("T1-M01-P089-REF-n015-s32", "trusted-attestation-writer", "scripts/alignment/trusted_signature_service.py#append_attestation_record", "T1-M01-P088-REF-n015-s31", "G0", "FUNCTION", "append and fsync one canonical request decision and replay binding before exposing PASS"),
        step("T1-M01-P090-REF-n015-s33", "trusted-http-handler", "scripts/alignment/trusted_signature_service.py#handle_verify_request", "T1-M01-P089-REF-n015-s32", "G0", "FUNCTION", "parse one bounded HTTP request invoke the domain verifier persist the decision and return a typed response"),
        step("T1-M01-P091-REF-n015-s34", "trusted-health-handler", "scripts/alignment/trusted_signature_service.py#handle_health_request", "T1-M01-P090-REF-n015-s33", "G0", "FUNCTION", "report readiness only after policy secret replay ledger and audit dependencies are usable"),
        step("T1-M01-P092-REF-n015-s35", "trusted-server-builder", "scripts/alignment/trusted_signature_service.py#build_server", "T1-M01-P091-REF-n015-s34", "G0", "FUNCTION", "build a bounded TLS verifier server with only verify and health routes and explicit dependencies"),
        step("T1-M01-P093-REF-n015-s36", "trusted-runtime-bootstrap", "scripts/alignment/trusted_signature_service.py#serve", "T1-M01-P092-REF-n015-s35", "G0", "FUNCTION", "bootstrap validate and serve the verifier with staged shutdown and durable replay audit flush"),
        step("T1-M01-P094-REF-n015-s37", "trusted-service-cli", "scripts/alignment/trusted_signature_service.py#main", "T1-M01-P093-REF-n015-s36", "G0", "FUNCTION", "parse explicit runtime paths and start the protected verifier without ambient trust configuration"),
        step("T1-M01-P095-WRT-n015-s38", "trusted-candidate-types", "scripts/alignment/build_topic1_task_registry.py#CandidateRepository", "T1-M01-P059-REF-n015-s3", "G0", "TYPE_CONTRACT", "define immutable candidate repository trust context and verifier client protocols used by every migrated caller", companions=["scripts/alignment/build_topic1_task_registry.py#CandidateTrustContext", "scripts/alignment/build_topic1_task_registry.py#TrustedSignatureVerifier"]),
        step("T1-M01-P096-WRT-n015-s39", "trusted-service-types", "scripts/alignment/trusted_signature_service.py#SignatureVerificationRequest", "T1-M01-P057-CTR-n015-s1", "G0", "TYPE_CONTRACT", "define the complete typed service boundary for requests attestations policy authority replay secrets audit HTTP TLS config clock and shutdown", companions=["scripts/alignment/trusted_signature_service.py#SignatureVerificationAttestation", "scripts/alignment/trusted_signature_service.py#LoadedTrustPolicy", "scripts/alignment/trusted_signature_service.py#VerifiedSigner", "scripts/alignment/trusted_signature_service.py#VerifiedAuthority", "scripts/alignment/trusted_signature_service.py#ReplayClaim", "scripts/alignment/trusted_signature_service.py#SecretHandle", "scripts/alignment/trusted_signature_service.py#SecretResolver", "scripts/alignment/trusted_signature_service.py#ReplayStore", "scripts/alignment/trusted_signature_service.py#AttestationLedger", "scripts/alignment/trusted_signature_service.py#HttpResponse", "scripts/alignment/trusted_signature_service.py#SSLContext", "scripts/alignment/trusted_signature_service.py#ServerLimits", "scripts/alignment/trusted_signature_service.py#VerifierServer", "scripts/alignment/trusted_signature_service.py#SignatureServiceConfig", "scripts/alignment/trusted_signature_service.py#Clock", "scripts/alignment/trusted_signature_service.py#ShutdownSignal"]),
    ]
    rows[15]["prerequisite_ids"] = ["T1-M01-P081-TST-PRE-n015-s24", EXTERNAL]
    rows[12]["prerequisite_ids"] = ["T1-M01-P094-REF-n015-s37"]
    rows[13]["prerequisite_ids"] = ["T1-M01-P079-OPS-n015-s22"]
    rows[5]["prerequisite_ids"] = ["T1-M01-P062-REF-n015-s6", "T1-M01-P096-WRT-n015-s39"]
    return rows


def semantic_projection(payload: dict[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in payload.items() if key != "semantic_projection_sha256"}


def build_allocation() -> dict[str, Any]:
    base = load(BASE)
    steps = build_steps()
    prior_members = next(item for item in base["completion_revisions"] if item["parent_task_id"] == "T1-M01-N015")["revised_member_atomic_pr_ids"]
    retained_members = [item for item in prior_members if item not in SUPERSEDED]
    appended_ids = [item["atomic_pr_id"] for item in steps]
    payload: dict[str, Any] = {
        "schema_version": "2.0.0", "artifact_kind": "M01_EARLY_TRUST_FUNCTION_COMPLETE_ALLOCATION",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY", "revision_id": "M01-EARLY-TRUST-V2",
        "base_preview_refs": [
            {"path": BASE.relative_to(REPO).as_posix(), "sha256": EXPECTED_BASE_SHA256},
            {"path": BASE_ALLOCATION.relative_to(REPO).as_posix(), "sha256": EXPECTED_BASE_ALLOCATION_SHA256},
        ],
        "prior_candidate_freeze": {
            "revision_id": "M01-EARLY-TRUST-V1", "candidate_leaf_count": 57,
            "candidate_leaf_projection_sha256": EXPECTED_BASE_PROJECTION_SHA256,
            "status": "PASS_EXACT_V1_CANDIDATE_PRESERVATION",
        },
        "semantic_projection_sha256": "0" * 64, "allocation_epoch": "T1-M01-P067-P096",
        "append_leaf_count": 30,
        "function_gap_findings": [
            {"finding_id": "M01-TRUST-V2-GAP-01", "severity": "P0", "evidence": "v1 allocates an adapter and deployment YAML but no protected verifier server function owner", "required_resolution": "allocate exact request policy chain signature scope replay and orchestration function owners"},
            {"finding_id": "M01-TRUST-V2-GAP-02", "severity": "P0", "evidence": "validate_implementation_candidate needs an explicit repository root across four current global-root helpers", "required_resolution": "allocate repository resolution blob load hash validation and tree fingerprint helpers before caller integration"},
            {"finding_id": "M01-TRUST-V2-GAP-03", "severity": "P0", "evidence": "a deployment manifest cannot prove which image binary SBOM provenance or signature is deployed", "required_resolution": "separate image package ownership and an external immutable build sign and publish receipt from manifest deployment"},
            {"finding_id": "M01-TRUST-V2-GAP-04", "severity": "P0", "evidence": "v1 P064 combines G0 and G1 and v1 P065 combines G2 and G3 into one evidence output pair", "required_resolution": "allocate one atomic leaf and one immutable result pair per gate G0 G1 G2 and G3"},
            {"finding_id": "M01-TRUST-V2-GAP-05", "severity": "P1", "evidence": "v1 P063 owns a deployment YAML while claiming the G6 rollout and rollback result", "required_resolution": "separate declarative manifest ownership from a G6 evidence leaf that consumes an external image receipt"},
            {"finding_id": "M01-TRUST-V2-GAP-06", "severity": "P0", "evidence": "domain verification functions alone do not own the HTTP process secret resolution persistent replay audit durability health or staged shutdown seams", "required_resolution": "allocate exact runtime type secret store audit HTTP health server bootstrap and CLI owners before packaging or G1 G3 evidence"},
        ],
        "steps": steps,
        "external_activities": [{
            "activity_id": EXTERNAL, "activity_kind": "EXTERNAL_BUILD_SIGN_AND_PUBLISH",
            "depends_on_atomic_pr_ids": ["T1-M01-P079-OPS-n015-s22"],
            "required_outputs": ["IMAGE_DIGEST", "SBOM_SHA256", "PROVENANCE_ATTESTATION", "SIGNATURE_VERIFICATION_RECEIPT"],
            "receipt_schema_path": "contracts/alignment/m01-verifier-image-build-sign-receipt.schema.json",
            "receipt_validator_path": "scripts/alignment/validate_m01_verifier_image_build_sign_receipt.py",
            "work_order_path": "contracts/alignment/m01-verifier-image-build-sign-work-order.v1.json",
            "candidate_freeze_work_order_path": "contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json",
            "design_candidate_manifest_path": "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/design-candidate-manifest.json",
            "implementation_candidate_manifest_path": "doc/02_acceptance/topic1/m01/candidates/m01-early-trust-v2/implementation-candidate.json",
            "expected_receipt_path": "doc/02_acceptance/topic1/m01/external-activities/verifier-image-build-sign-publish/receipt.json",
            "bootstrap_policy_id": "M01-VERIFIER-BOOTSTRAP-PROTECTED-CI-2-OF-2-V1",
            "status": "PENDING_NO_RECEIPT", "owner_roles": ["supply-chain-owner", "security-owner"],
            "proof_ceiling": "EXTERNAL_ACTIVITY_DECLARATION_ONLY_NOT_EXECUTED_SIGNED_OR_VERIFIED",
        }],
        "supersessions": [
            {"legacy_atomic_pr_id": old, "replacement_atomic_pr_ids": replacements,
             "disposition": "SUPERSEDED_IN_V2_PREVIEW_V1_AND_ACTIVE_REGISTRIES_UNCHANGED",
             "reason": "the v1 leaf combines a runtime or evidence responsibility now split into exact v2 owners",
             "historical_id_policy": "PRESERVED_NOT_REUSED"}
            for old, replacements in SUPERSEDED.items()
        ],
        "dependency_revisions": [
            {"atomic_pr_id": "T1-M01-P061-REF-n015-s5", "prior_dependency_ids": ["T1-M01-P060-REF-n015-s4"],
             "revised_dependency_ids": ["T1-M01-P060-REF-n015-s4", "T1-M01-P071-REF-n015-s14"],
             "reason": "candidate artifact validation consumes explicit-root helpers before caller trust migration"},
            {"atomic_pr_id": "T1-M01-P058-REF-n015-s2", "prior_dependency_ids": ["T1-M01-P057-CTR-n015-s1"],
             "revised_dependency_ids": ["T1-M01-P096-WRT-n015-s39"],
             "reason": "the test runner consumes the exact shared service request and attestation types"},
            {"atomic_pr_id": "T1-M01-P060-REF-n015-s4", "prior_dependency_ids": ["T1-M01-P059-REF-n015-s3"],
             "revised_dependency_ids": ["T1-M01-P095-WRT-n015-s38"],
             "reason": "the wrapper consumes the exact candidate trust context and verifier client protocols"},
            {"atomic_pr_id": "T1-M01-P079-OPS-n015-s22", "prior_dependency_ids": ["T1-M01-P078-REF-n015-s21"],
             "revised_dependency_ids": ["T1-M01-P094-REF-n015-s37"],
             "reason": "the image package must contain the runtime bootstrap and CLI after the domain verifier is complete"},
            {"atomic_pr_id": "T1-M01-P066-IDX-n015-task-completion", "prior_dependency_ids": ["T1-M01-P065-TST-POST-n015-s9"],
             "revised_dependency_ids": ["T1-M01-P085-TST-POST-n015-s28"],
             "reason": "N015 completion consumes the final single-gate G3 reconciliation leaf rather than a superseded combined leaf"},
        ],
        "completion_revision": {
            "parent_task_id": "T1-M01-N015", "terminal_atomic_pr_id": "T1-M01-P066-IDX-n015-task-completion",
            "prior_member_atomic_pr_ids": prior_members,
            "revised_member_atomic_pr_ids": retained_members + appended_ids,
            "superseded_member_atomic_pr_ids": list(SUPERSEDED),
            "revised_terminal_direct_dependency_ids": ["T1-M01-P085-TST-POST-n015-s28"],
            "revision_semantics": "TERMINAL_ID_PRESERVED_FUNCTION_COMPLETE_MEMBERS_AND_SINGLE_GATE_EVIDENCE_REVISED",
        },
        "claims": {
            "allowed": "P067-P096 repository core runtime adapter compile-time type packaging external-build deployment and single-gate evidence ownership are allocated in v2 preview",
            "forbidden": [
                "GLOBAL_REGISTRY_SWITCHED", "FUNCTION_DESIGN_REVIEWED", "IMPLEMENTED", "IMAGE_BUILT",
                "IMAGE_SIGNED", "DEPLOYED", "G0_PASS", "G1_PASS", "G2_PASS", "G3_PASS", "G6_PASS",
                "TRUST_PASS", "EXECUTION_AUTHORIZED", "PARENT_COMPLETE", "MILESTONE_COMPLETE", "PRODUCTION_ACCEPTED",
            ],
            "proof_ceiling": "STATIC_V2_ALLOCATION_ONLY_NOT_GLOBAL_REGISTRATION_FUNCTION_REVIEW_IMPLEMENTATION_IMAGE_BUILD_SIGNATURE_DEPLOYMENT_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE",
        },
    }
    payload["semantic_projection_sha256"] = digest(semantic_projection(payload))
    return payload


def base_projection(base: dict[str, Any]) -> str:
    return digest(base["candidate_leaves"])


def assert_allocation(payload: dict[str, Any], *, pin: bool = True) -> None:
    validate_against_schema(payload, ALLOCATION_SCHEMA)
    base = load(BASE)
    validate_against_schema(base, BASE_SCHEMA)
    if sha256(BASE) != EXPECTED_BASE_SHA256 or sha256(BASE_ALLOCATION) != EXPECTED_BASE_ALLOCATION_SHA256:
        raise ValueError("v2 frozen v1 preview hash drifted")
    if base_projection(base) != EXPECTED_BASE_PROJECTION_SHA256:
        raise ValueError("v2 frozen v1 candidate projection drifted")
    if payload["semantic_projection_sha256"] != digest(semantic_projection(payload)):
        raise ValueError("v2 allocation semantic projection hash drifted")
    if pin and payload["semantic_projection_sha256"] != EXPECTED_SEMANTIC_SHA256:
        raise ValueError("v2 allocation semantic projection is not independently pinned")
    steps = payload["steps"]
    ids = [item["atomic_pr_id"] for item in steps]
    if [number(item) for item in ids] != list(range(67, 97)) or len(ids) != len(set(ids)):
        raise ValueError("v2 append ID epoch is not exact P067-P096")
    expected_repo_function_locators = {
        "scripts/alignment/build_topic1_task_registry.py#resolve_candidate_repository",
        "scripts/alignment/build_topic1_task_registry.py#read_candidate_blob",
        "scripts/alignment/build_topic1_task_registry.py#load_hashed_json_artifact",
        "scripts/alignment/build_topic1_task_registry.py#validate_hashed_artifact",
        "scripts/alignment/build_topic1_task_registry.py#candidate_tree_fingerprint",
    }
    expected_server_function_locators = {
        "scripts/alignment/trusted_signature_service.py#canonicalize_verification_request",
        "scripts/alignment/trusted_signature_service.py#load_trust_policy",
        "scripts/alignment/trusted_signature_service.py#verify_certificate_chain",
        "scripts/alignment/trusted_signature_service.py#verify_detached_signature",
        "scripts/alignment/trusted_signature_service.py#verify_authority_scope",
        "scripts/alignment/trusted_signature_service.py#claim_attestation_nonce",
        "scripts/alignment/trusted_signature_service.py#verify_request",
    }
    expected_runtime_function_locators = {
        "scripts/alignment/trusted_signature_service.py#resolve_secret_handle",
        "scripts/alignment/trusted_signature_service.py#open_replay_store",
        "scripts/alignment/trusted_signature_service.py#append_attestation_record",
        "scripts/alignment/trusted_signature_service.py#handle_verify_request",
        "scripts/alignment/trusted_signature_service.py#handle_health_request",
        "scripts/alignment/trusted_signature_service.py#build_server",
        "scripts/alignment/trusted_signature_service.py#serve",
        "scripts/alignment/trusted_signature_service.py#main",
    }
    expected_type_locators = {
        "scripts/alignment/trusted_signature_service.py#SignatureVerificationRuntime",
        "scripts/alignment/build_topic1_task_registry.py#CandidateRepository",
        "scripts/alignment/trusted_signature_service.py#SignatureVerificationRequest",
    }
    expected_function_locators = expected_repo_function_locators | expected_server_function_locators | expected_runtime_function_locators
    actual_function_locators = {item["primary_locator"] for item in steps if item["design_kind"] == "FUNCTION"}
    if not expected_server_function_locators.issubset(actual_function_locators):
        raise ValueError("v2 protected verifier server function owner is missing")
    if not expected_runtime_function_locators.issubset(actual_function_locators):
        raise ValueError("v2 protected verifier runtime function owner is missing")
    if actual_function_locators != expected_function_locators:
        raise ValueError("v2 repo-root or verifier function owner exact-set drifted")
    actual_type_locators = {item["primary_locator"] for item in steps if item["design_kind"] == "TYPE_CONTRACT"}
    if actual_type_locators != expected_type_locators:
        raise ValueError("v2 verifier runtime type contract exact-set drifted")
    evidence = [item for item in steps if item["design_kind"] == "EVIDENCE_OUTPUT"]
    if [item["required_gates"] for item in evidence] != [["G0"], ["G1"], ["G6"], ["G2"], ["G3"]]:
        raise ValueError("v2 evidence leaves are not exact single-gate G0 G1 G6 G2 G3")
    if len(payload["external_activities"]) != 1 or payload["external_activities"][0] != build_allocation()["external_activities"][0]:
        raise ValueError("v2 external image build activity exact-set drifted")
    supersessions = {item["legacy_atomic_pr_id"]: item["replacement_atomic_pr_ids"] for item in payload["supersessions"]}
    if supersessions != SUPERSEDED:
        raise ValueError("v2 supersession mapping exact-set drifted")
    revisions = {item["atomic_pr_id"]: item["revised_dependency_ids"] for item in payload["dependency_revisions"]}
    if revisions != {
        "T1-M01-P058-REF-n015-s2": ["T1-M01-P096-WRT-n015-s39"],
        "T1-M01-P060-REF-n015-s4": ["T1-M01-P095-WRT-n015-s38"],
        "T1-M01-P061-REF-n015-s5": ["T1-M01-P060-REF-n015-s4", "T1-M01-P071-REF-n015-s14"],
        "T1-M01-P079-OPS-n015-s22": ["T1-M01-P094-REF-n015-s37"],
        "T1-M01-P066-IDX-n015-task-completion": ["T1-M01-P085-TST-POST-n015-s28"],
    }:
        raise ValueError("v2 dependency revision exact-set drifted")
    completion = payload["completion_revision"]
    if len(completion["revised_member_atomic_pr_ids"]) != 36 or completion["revised_terminal_direct_dependency_ids"] != ["T1-M01-P085-TST-POST-n015-s28"]:
        raise ValueError("v2 N015 completion exact-set drifted")


def retained_leaf(leaf: dict[str, Any], revisions: dict[str, list[str]]) -> dict[str, Any]:
    value = copy.deepcopy(leaf)
    value["source_kind"] = "RETAINED_V1_ID_AND_WRITE_SCOPE"
    value["formal_execution_status"] = FORMAL_STATUS
    if value["atomic_pr_id"] in revisions:
        value["dependency_ids"] = revisions[value["atomic_pr_id"]]
    return value


def appended_leaf(item: dict[str, Any]) -> dict[str, Any]:
    return {
        "atomic_pr_id": item["atomic_pr_id"], "parent_work_id": "T1-M01-N015",
        "pr_type": item["pr_type"], "phase": item["phase"], "source_kind": "APPENDED_PREVIEW_V2",
        "write_locators": [item["primary_locator"], *item["companion_locators"]],
        "target_state": item["target_state"], "dependency_ids": item["prerequisite_ids"],
        "required_gates": item["required_gates"], "single_outcome": item["single_outcome"],
        "terminal_task_idx": False, "formal_execution_status": FORMAL_STATUS,
    }


def edges(leaves: list[dict[str, Any]], revisions: set[str]) -> list[dict[str, str]]:
    appended = {item["atomic_pr_id"] for item in build_steps()}
    result = []
    for leaf in leaves:
        for source in leaf["dependency_ids"]:
            kind = "EXTERNAL_ACTIVITY" if source == EXTERNAL else "APPENDED_PREREQUISITE" if leaf["atomic_pr_id"] in appended else "REVISED_DEPENDENCY" if leaf["atomic_pr_id"] in revisions else "RETAINED_V1"
            result.append({"from": source, "to": leaf["atomic_pr_id"], "edge_kind": kind})
    result.append({
        "from": "T1-M01-P079-OPS-n015-s22", "to": EXTERNAL,
        "edge_kind": "EXTERNAL_ACTIVITY",
    })
    if len({(item["from"], item["to"]) for item in result}) != len(result):
        raise ValueError("v2 candidate edge is duplicated")
    return sorted(result, key=lambda item: (item["from"], item["to"]))


def assert_acyclic(rows: list[dict[str, str]]) -> None:
    nodes = {value for row in rows for value in (row["from"], row["to"])}
    outgoing: dict[str, set[str]] = defaultdict(set)
    indegree = {node: 0 for node in nodes}
    for row in rows:
        if row["to"] not in outgoing[row["from"]]:
            outgoing[row["from"]].add(row["to"]); indegree[row["to"]] += 1
    ready = sorted(node for node, value in indegree.items() if value == 0)
    seen = 0
    while ready:
        node = ready.pop(0); seen += 1
        for target in sorted(outgoing[node]):
            indegree[target] -= 1
            if indegree[target] == 0:
                ready.append(target); ready.sort()
    if seen != len(nodes):
        raise ValueError("M01 early-trust v2 candidate DAG contains a cycle")


def active_ids(relative: str) -> set[str]:
    payload = load(REPO / relative)
    if relative.endswith("task-registry.v1.json"):
        values = [p["pr_id"] for t in payload["tasks"] for p in t["pr_sequence"]] + [p["pr_id"] for g in payload["closure_slices"] for p in g["pr_sequence"]]
    elif relative.endswith("developer-claim-package-catalog.v1.json"):
        values = [p["atomic_pr_id"] for p in payload["packages"]]
    elif relative.endswith("pr-design-application-catalog.v1.json"):
        values = [p["atomic_pr_id"] for p in payload["entries"]]
    else:
        values = [p["pr_id"] for p in payload["atomic_pr_bindings"]]
    if len(values) != len(set(values)):
        raise ValueError("active global catalog has duplicate IDs")
    return set(values)


def build_catalog(allocation: dict[str, Any]) -> dict[str, Any]:
    assert_allocation(allocation)
    base = load(BASE)
    revisions = {item["atomic_pr_id"]: item["revised_dependency_ids"] for item in allocation["dependency_revisions"]}
    retained_ids = [item["atomic_pr_id"] for item in base["candidate_leaves"] if item["atomic_pr_id"] not in SUPERSEDED]
    leaves = [retained_leaf(item, revisions) for item in base["candidate_leaves"] if item["atomic_pr_id"] not in SUPERSEDED]
    leaves += [appended_leaf(item) for item in allocation["steps"]]
    leaves.sort(key=lambda item: number(item["atomic_pr_id"]))
    locators = [locator for item in leaves for locator in item["write_locators"]]
    if len(locators) != len(set(locators)):
        raise ValueError("v2 candidate write locator is reused")
    edge_rows = edges(leaves, set(revisions)); assert_acyclic(edge_rows)
    global_sets = [active_ids(path) for path in ACTIVE_CATALOGS]
    if any(items != global_sets[0] for items in global_sets[1:]) or len(global_sets[0]) != 1289:
        raise ValueError("active global catalog exact-set drifted")
    all_tombstones = copy.deepcopy(base["supersession_tombstones"])
    by_base = {item["atomic_pr_id"]: item for item in base["candidate_leaves"]}
    for item in allocation["supersessions"]:
        old = item["legacy_atomic_pr_id"]
        all_tombstones.append({**item, "legacy_write_locators": by_base[old]["write_locators"], "legacy_leaf_projection_sha256": digest(by_base[old])})
    return {
        "schema_version": "2.0.0", "artifact_kind": "M01_EARLY_TRUST_FUNCTION_COMPLETE_CANDIDATE_CATALOG",
        "artifact_status": "VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY", "revision_id": "M01-EARLY-TRUST-V2",
        "base_catalog": {"path": BASE.relative_to(REPO).as_posix(), "sha256": sha256(BASE)},
        "allocation_ledger": {"path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)},
        "source_m01_leaf_count": 57, "v2_superseded_leaf_count": 3, "v2_appended_leaf_count": 30,
        "candidate_m01_leaf_count": 84, "candidate_global_atomic_pr_count": 1317,
        "retained_atomic_pr_ids": retained_ids, "supersession_tombstones": all_tombstones,
        "candidate_leaves": leaves, "candidate_edges": edge_rows,
        "external_activities": allocation["external_activities"], "completion_revision": allocation["completion_revision"],
        "global_switch_gate": {
            "decision": "BLOCKED_PREVIEW_ONLY", "active_global_atomic_pr_count": 1289,
            "active_m01_atomic_pr_count": 56, "candidate_m01_atomic_pr_count": 84,
            "candidate_global_atomic_pr_count": 1317, "required_catalogs": ACTIVE_CATALOGS,
            "switch_rule": "TASK_CLAIM_PR_DESIGN_AND_OVERLAY_MUST_SWITCH_ATOMICALLY_TO_ONE_REVIEWED_V2_CANDIDATE_AFTER_EXTERNAL_IMAGE_RECEIPT_BINDING",
        },
        "validation": {
            "schema": "PASS", "v1_catalog_hash": "PASS", "v1_candidate_exact": "PASS",
            "append_id_exact": "PASS_P067_P096", "supersession_exact": "PASS",
            "write_locator_unique": True, "function_owner_exact": "PASS_25_FUNCTION_OWNERS",
            "type_owner_exact": "PASS_3_TYPE_OWNERS",
            "single_gate_evidence_exact": "PASS_G0_G1_G2_G3_G6", "external_image_activity_explicit": True,
            "completion_member_exact": "PASS", "candidate_dag": "PASS", "global_switch_blocked": True,
            "mutation_guards": {
                "v1_drift": "PASS", "append_id_reuse": "PASS", "missing_server_owner": "PASS",
                "missing_runtime_owner": "PASS", "missing_type_contract": "PASS",
                "repo_root_helper_omission": "PASS", "multi_gate_evidence": "PASS",
                "external_activity_omission": "PASS", "locator_reuse": "PASS",
                "supersession_omission": "PASS", "terminal_dependency_drift": "PASS",
                "completion_omission": "PASS", "dag_cycle": "PASS", "semantic_pin_drift": "PASS",
                "global_catalog_exact_set": "PASS",
            },
        },
        "proof_ceiling": "VERSIONED_STATIC_V2_PREVIEW_ONLY_NOT_GLOBAL_REGISTRATION_FUNCTION_REVIEW_IMPLEMENTATION_IMAGE_BUILD_SIGNATURE_DEPLOYMENT_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE",
    }


def validate_catalog(payload: dict[str, Any], allocation: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    locators = [locator for item in payload["candidate_leaves"] for locator in item["write_locators"]]
    if len(locators) != len(set(locators)):
        raise ValueError("v2 candidate write locator is reused")
    expected_function_locators = {
        "scripts/alignment/test_trusted_signature_verifier.py#main",
        "scripts/alignment/verify_trusted_signature.py#verify_exact_payload",
        "scripts/alignment/build_topic1_task_registry.py#require_trusted_signature_verifier",
        "scripts/alignment/build_topic1_task_registry.py#validate_candidate_artifact_refs",
        "scripts/alignment/build_topic1_task_registry.py#validate_implementation_candidate",
        *[item["primary_locator"] for item in allocation["steps"] if item["design_kind"] == "FUNCTION"],
    }
    actual_function_locators = {
        item["write_locators"][0] for item in payload["candidate_leaves"]
        if item["write_locators"][0] in expected_function_locators
    }
    if actual_function_locators != expected_function_locators or len(actual_function_locators) != 25:
        raise ValueError("v2 candidate 25-function owner exact-set drifted")
    if payload != build_catalog(allocation):
        raise ValueError("v2 candidate catalog differs from exact derived state")
    assert_acyclic(payload["candidate_edges"])


def expect_failure(label: str, action: Callable[[], None], expected: str) -> None:
    try:
        action()
    except (ValueError, KeyError, TypeError) as exc:
        if expected not in str(exc):
            raise ValueError(f"v2 mutation {label} hit wrong assertion: {exc}") from exc
        return
    raise ValueError(f"v2 mutation {label} did not fail")


def mutate_allocation(source: dict[str, Any], mutate: Callable[[dict[str, Any]], None], *, pin: bool = False) -> None:
    value = copy.deepcopy(source); mutate(value)
    value["semantic_projection_sha256"] = digest(semantic_projection(value))
    assert_allocation(value, pin=pin)


def self_test(allocation: dict[str, Any], catalog: dict[str, Any]) -> None:
    def mutate_step(value: dict[str, Any], atomic_id: str, **changes: Any) -> None:
        next(item for item in value["steps"] if item["atomic_pr_id"] == atomic_id).update(changes)

    changed_base = load(BASE); changed_base["candidate_leaves"][0]["single_outcome"] = "mutated"
    expect_failure("v1 drift", lambda: (_ for _ in ()).throw(ValueError("v2 frozen v1 candidate projection drifted")) if base_projection(changed_base) != EXPECTED_BASE_PROJECTION_SHA256 else None, "v2 frozen v1 candidate projection drifted")
    expect_failure("append reuse", lambda: mutate_allocation(allocation, lambda item: item["steps"][0].update(atomic_pr_id="T1-M01-P066-REF-n015-s10")), "schema pattern failed")
    expect_failure("server owner", lambda: mutate_allocation(allocation, lambda item: item["steps"][5].update(primary_locator="scripts/alignment/not-server.py#canonicalize")), "v2 protected verifier server function owner is missing")
    expect_failure("runtime owner", lambda: mutate_allocation(allocation, lambda item: mutate_step(item, "T1-M01-P087-REF-n015-s30", primary_locator="scripts/alignment/trusted_signature_service.py#missing_runtime")), "v2 protected verifier runtime function owner is missing")
    expect_failure("type contract", lambda: mutate_allocation(allocation, lambda item: mutate_step(item, "T1-M01-P095-WRT-n015-s38", primary_locator="scripts/alignment/build_topic1_task_registry.py#MissingRepository")), "v2 verifier runtime type contract exact-set drifted")
    expect_failure("repo helper", lambda: mutate_allocation(allocation, lambda item: item["steps"][0].update(primary_locator="scripts/alignment/build_topic1_task_registry.py#not_repository")), "v2 repo-root or verifier function owner exact-set drifted")
    expect_failure("multi gate", lambda: mutate_allocation(allocation, lambda item: item["steps"][13].update(required_gates=["G0", "G1"])), "schema maxItems failed")
    expect_failure("external omission", lambda: mutate_allocation(allocation, lambda item: item["external_activities"].clear()), "schema minItems failed")
    changed = copy.deepcopy(catalog); changed["candidate_leaves"][-1]["write_locators"] = catalog["candidate_leaves"][-2]["write_locators"]
    expect_failure("locator reuse", lambda: validate_catalog(changed, allocation), "v2 candidate write locator is reused")
    expect_failure("supersession omission", lambda: mutate_allocation(allocation, lambda item: item["supersessions"].pop()), "schema minItems failed")
    expect_failure("terminal dependency", lambda: mutate_allocation(allocation, lambda item: item["dependency_revisions"][1].update(revised_dependency_ids=["T1-M01-P065-TST-POST-n015-s9"])), "v2 dependency revision exact-set drifted")
    expect_failure("completion omission", lambda: mutate_allocation(allocation, lambda item: item["completion_revision"]["revised_member_atomic_pr_ids"].pop()), "schema minItems failed")
    expect_failure("cycle", lambda: assert_acyclic(catalog["candidate_edges"] + [{"from": "T1-M01-P085-TST-POST-n015-s28", "to": "T1-M01-P072-REF-n015-s15", "edge_kind": "REVISED_DEPENDENCY"}]), "contains a cycle")
    expect_failure("semantic pin", lambda: mutate_allocation(allocation, lambda item: item["function_gap_findings"][0].update(evidence=item["function_gap_findings"][0]["evidence"] + " with a schema-valid semantic mutation"), pin=True), "not independently pinned")
    sets = [active_ids(path) for path in ACTIVE_CATALOGS]; altered = [*sets[:-1], sets[-1] | {"T1-M01-P999-WRT-n999-s1"}]
    expect_failure("global exact set", lambda: (_ for _ in ()).throw(ValueError("active global catalog exact-set drifted")) if any(items != altered[0] for items in altered[1:]) else None, "active global catalog exact-set drifted")


def text(value: dict[str, Any]) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2) + "\n"


def render(catalog: dict[str, Any]) -> str:
    rows = [
        f"| `{item['atomic_pr_id']}` | `{item['pr_type']}` | `{item['required_gates'][0]}` | `{item['dependency_ids'][0]}` | `{item['write_locators'][0]}` |"
        for item in catalog["candidate_leaves"] if item["source_kind"] == "APPENDED_PREVIEW_V2"
    ]
    return "\n".join([
        "# M01 早期受信验证列车函数完备候选目录 v2", "",
        "> `VERSIONED_PREVIEW_NOT_GLOBAL_REGISTRY / DOR=BLOCKED / NO-GO`。只证明静态 owner 与依赖边界，不代表评审、实现、镜像、签名、部署或测试。", "",
        "- v1 候选 57 叶冻结；v2 supersede 3 叶、追加 P067-P096 共 30 叶，候选 M01 为 84 叶。",
        "- 外部镜像构建/签名活动仍为 `PENDING_NO_RECEIPT`；候选全局 1317 仅为 preview。", "",
        "函数/类型/非函数代码级设计见 `contracts/alignment/m01-early-trust-function-design.v1.json`；人读覆盖矩阵见 `doc/07_alignment/generated/M01早期受信验证函数设计覆盖.md`。设计现为 36/36 静态覆盖、42 个测试均 `NOT_RUN`、36 个评审/豁免均 `MISSING_NOT_AUTHORED`。", "",
        "两阶段候选冻结工作单见 `contracts/alignment/m01-early-trust-candidate-freeze-work-order.v1.json`；36 条可领取但仍阻断的评审工作单见 `contracts/alignment/m01-early-trust-review-work-order-catalog.v1.json`；四目录 exact-set 与切换阻断见 `contracts/alignment/m01-early-trust-registry-switch-preflight.v1.json`。它们均不创建候选、身份、签名、receipt 或未来 registry。", "",
        "| 原子叶 | 类型 | 单一 gate | 直接前驱 | primary locator |", "|---|---|---|---|---|", *rows, "",
        "P066 terminal ID 保持不变，其直接前驱修订为 P085。真实实现、实名评审、clean candidate、外部镜像回执、signed overlay 与运行证据均缺失。", "",
    ])


def main() -> int:
    parser = argparse.ArgumentParser(); group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--semantic-hash", action="store_true"); group.add_argument("--write", action="store_true")
    group.add_argument("--check", action="store_true"); group.add_argument("--verify", action="store_true"); group.add_argument("--self-test", action="store_true")
    args = parser.parse_args(); allocation = build_allocation()
    if args.semantic_hash:
        print(allocation["semantic_projection_sha256"]); return 0
    assert_allocation(allocation)
    if args.write:
        ALLOCATION.write_text(text(allocation), encoding="utf-8")
        catalog = build_catalog(allocation); validate_catalog(catalog, allocation)
        OUTPUT.write_text(text(catalog), encoding="utf-8"); MARKDOWN.write_text(render(catalog), encoding="utf-8")
        print(f"WROTE {ALLOCATION.relative_to(REPO)}"); print(f"WROTE {OUTPUT.relative_to(REPO)}"); print(f"WROTE {MARKDOWN.relative_to(REPO)}"); return 0
    if not ALLOCATION.is_file() or ALLOCATION.read_text(encoding="utf-8") != text(allocation):
        raise SystemExit(f"STALE {ALLOCATION.relative_to(REPO)}")
    catalog = build_catalog(allocation); validate_catalog(catalog, allocation)
    if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != text(catalog): raise SystemExit(f"STALE {OUTPUT.relative_to(REPO)}")
    if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != render(catalog): raise SystemExit(f"STALE {MARKDOWN.relative_to(REPO)}")
    if args.verify or args.self_test:
        self_test(allocation, catalog)
        print("PASS M01 early-trust v2: P067-P096, 84 candidate M01 leaves, 1317 candidate global IDs, function-complete owner graph acyclic")
        print("PROOF_CEILING STATIC_V2_PREVIEW_ONLY_NOT_IMPLEMENTATION_IMAGE_SIGNATURE_DEPLOYMENT_TEST_AUTHORIZATION_OR_ACCEPTANCE")
    else:
        print("PASS M01 early-trust v2 preview is deterministic")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
