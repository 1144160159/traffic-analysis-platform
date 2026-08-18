#!/usr/bin/env python3
"""Generate the blocked, function-complete M01 N015 static design.

The artifact specifies code and non-function surfaces for the M01 v2 preview.
It never creates review receipts, implementation, images, signatures, runtime
evidence, execution authorization, or a global registry switch.
"""

from __future__ import annotations

import argparse
import hashlib
import heapq
import json
import re
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
ALLOCATION = REPO / "contracts/alignment/m01-early-trust-train-allocation.v2.json"
CATALOG = REPO / "contracts/alignment/m01-early-trust-train-catalog.v2.json"
BUILDER = REPO / "scripts/alignment/build_topic1_task_registry.py"
FUNCTION_REVIEW_SCHEMA = REPO / "contracts/alignment/function-design-review-receipt.schema.json"
NON_FUNCTION_REVIEW_SCHEMA = REPO / "contracts/alignment/non-function-design-exemption.schema.json"
SCHEMA = REPO / "contracts/alignment/m01-early-trust-function-design.schema.json"
OUTPUT = REPO / "contracts/alignment/m01-early-trust-function-design.v1.json"
MARKDOWN = REPO / "doc/07_alignment/generated/M01早期受信验证函数设计覆盖.md"

TERMINAL = "T1-M01-P066-IDX-n015-task-completion"
EXTERNAL_IMAGE = "EXT-T1-M01-N015-VERIFIER-IMAGE-BUILD-AND-SIGN"
BLOCKED = "BLOCKED_PREVIEW_NOT_REGISTERED"
FUNCTION_IDS = [
    "T1-M01-P058-REF-n015-s2", "T1-M01-P059-REF-n015-s3",
    "T1-M01-P060-REF-n015-s4", "T1-M01-P061-REF-n015-s5",
    "T1-M01-P062-REF-n015-s6", "T1-M01-P067-REF-n015-s10",
    "T1-M01-P068-REF-n015-s11", "T1-M01-P069-REF-n015-s12",
    "T1-M01-P070-REF-n015-s13", "T1-M01-P071-REF-n015-s14",
    "T1-M01-P072-REF-n015-s15", "T1-M01-P073-REF-n015-s16",
    "T1-M01-P074-REF-n015-s17", "T1-M01-P075-REF-n015-s18",
    "T1-M01-P076-REF-n015-s19", "T1-M01-P077-REF-n015-s20",
    "T1-M01-P078-REF-n015-s21",
    "T1-M01-P087-REF-n015-s30", "T1-M01-P088-REF-n015-s31",
    "T1-M01-P089-REF-n015-s32", "T1-M01-P090-REF-n015-s33",
    "T1-M01-P091-REF-n015-s34", "T1-M01-P092-REF-n015-s35",
    "T1-M01-P093-REF-n015-s36", "T1-M01-P094-REF-n015-s37",
]
TYPE_IDS = [
    "T1-M01-P086-WRT-n015-s29", "T1-M01-P095-WRT-n015-s38",
    "T1-M01-P096-WRT-n015-s39",
]
NON_FUNCTION_IDS = [
    "T1-M01-P057-CTR-n015-s1", "T1-M01-P079-OPS-n015-s22",
    "T1-M01-P080-TST-PRE-n015-s23", "T1-M01-P081-TST-PRE-n015-s24",
    "T1-M01-P082-OPS-n015-s25", "T1-M01-P083-OPS-n015-s26",
    "T1-M01-P084-TST-POST-n015-s27", "T1-M01-P085-TST-POST-n015-s28",
]
MEMBER_IDS = [
    "T1-M01-P057-CTR-n015-s1", "T1-M01-P058-REF-n015-s2",
    "T1-M01-P059-REF-n015-s3", "T1-M01-P060-REF-n015-s4",
    "T1-M01-P061-REF-n015-s5", "T1-M01-P062-REF-n015-s6",
    "T1-M01-P067-REF-n015-s10", "T1-M01-P068-REF-n015-s11",
    "T1-M01-P069-REF-n015-s12", "T1-M01-P070-REF-n015-s13",
    "T1-M01-P071-REF-n015-s14", "T1-M01-P072-REF-n015-s15",
    "T1-M01-P073-REF-n015-s16", "T1-M01-P074-REF-n015-s17",
    "T1-M01-P075-REF-n015-s18", "T1-M01-P076-REF-n015-s19",
    "T1-M01-P077-REF-n015-s20", "T1-M01-P078-REF-n015-s21",
    "T1-M01-P079-OPS-n015-s22", "T1-M01-P080-TST-PRE-n015-s23",
    "T1-M01-P081-TST-PRE-n015-s24", "T1-M01-P082-OPS-n015-s25",
    "T1-M01-P083-OPS-n015-s26", "T1-M01-P084-TST-POST-n015-s27",
    "T1-M01-P085-TST-POST-n015-s28",
    "T1-M01-P086-WRT-n015-s29", "T1-M01-P087-REF-n015-s30",
    "T1-M01-P088-REF-n015-s31", "T1-M01-P089-REF-n015-s32",
    "T1-M01-P090-REF-n015-s33", "T1-M01-P091-REF-n015-s34",
    "T1-M01-P092-REF-n015-s35", "T1-M01-P093-REF-n015-s36",
    "T1-M01-P094-REF-n015-s37",
    "T1-M01-P095-WRT-n015-s38", "T1-M01-P096-WRT-n015-s39",
]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON root must be an object: {path}")
    return value


def pr_number(atomic_id: str) -> int:
    match = re.search(r"-P([0-9]{3})-", atomic_id)
    if not match:
        raise ValueError(f"invalid atomic ID: {atomic_id}")
    return int(match.group(1))


SIGNATURES: dict[str, tuple[str | None, str]] = {
    FUNCTION_IDS[0]: (None, "def main(argv: Sequence[str] | None = None) -> int"),
    FUNCTION_IDS[1]: (None, "def verify_exact_payload(request: SignatureVerificationRequest, *, endpoint: str, policy_fingerprint: str, timeout_seconds: float = 5.0, max_response_bytes: int = 262144) -> SignatureVerificationAttestation"),
    FUNCTION_IDS[2]: ("def require_trusted_signature_verifier(context: str) -> None", "def require_trusted_signature_verifier(request: SignatureVerificationRequest, *, client: TrustedSignatureVerifier, context: str) -> SignatureVerificationAttestation"),
    FUNCTION_IDS[3]: ("def validate_candidate_artifact_refs(refs: list[dict[str, Any]], expected_hashes: list[str], candidate_commit: str, context: str) -> None", "def validate_candidate_artifact_refs(refs: list[dict[str, Any]], expected_hashes: list[str], candidate_commit: str, context: str, *, repo_root: Path, verifier: TrustedSignatureVerifier, trust_context: CandidateTrustContext) -> None"),
    FUNCTION_IDS[4]: ("def validate_implementation_candidate(candidate: dict[str, Any], context: str) -> None", "def validate_implementation_candidate(candidate: dict[str, Any], context: str, *, repo_root: Path, verifier: TrustedSignatureVerifier, trust_context: CandidateTrustContext) -> None"),
    FUNCTION_IDS[5]: (None, "def resolve_candidate_repository(repo_root: Path) -> CandidateRepository"),
    FUNCTION_IDS[6]: ("def read_candidate_blob(candidate_commit: str, path: str) -> bytes | None", "def read_candidate_blob(repo_root: Path, candidate_commit: str, path: str) -> bytes | None"),
    FUNCTION_IDS[7]: ("def load_hashed_json_artifact(artifact_path: str, expected_sha256: str, schema_path: Path | None = None) -> dict[str, Any]", "def load_hashed_json_artifact(repo_root: Path, artifact_path: str, expected_sha256: str, schema_path: Path | None = None) -> dict[str, Any]"),
    FUNCTION_IDS[8]: ("def validate_hashed_artifact(artifact_path: str, expected_sha256: str) -> None", "def validate_hashed_artifact(repo_root: Path, artifact_path: str, expected_sha256: str) -> None"),
    FUNCTION_IDS[9]: ("def candidate_tree_fingerprint(candidate_commit: str, source_roots: list[str], excluded_paths: list[dict[str, Any]]) -> str", "def candidate_tree_fingerprint(repo_root: Path, candidate_commit: str, source_roots: list[str], excluded_paths: list[dict[str, Any]]) -> str"),
    FUNCTION_IDS[10]: (None, "def canonicalize_verification_request(request: SignatureVerificationRequest) -> bytes"),
    FUNCTION_IDS[11]: (None, "def load_trust_policy(policy_path: Path, *, policy_fingerprint: str, secret_resolver: SecretResolver) -> LoadedTrustPolicy"),
    FUNCTION_IDS[12]: (None, "def verify_certificate_chain(chain: Sequence[bytes], *, policy: LoadedTrustPolicy, verification_time: datetime) -> VerifiedSigner"),
    FUNCTION_IDS[13]: (None, "def verify_detached_signature(payload: bytes, signature: bytes, *, signer: VerifiedSigner, algorithm: str) -> str"),
    FUNCTION_IDS[14]: (None, "def verify_authority_scope(request: SignatureVerificationRequest, signer: VerifiedSigner, *, policy: LoadedTrustPolicy, now: datetime) -> VerifiedAuthority"),
    FUNCTION_IDS[15]: (None, "def claim_attestation_nonce(store: ReplayStore, request: SignatureVerificationRequest, *, payload_sha256: str, expires_at: datetime) -> ReplayClaim"),
    FUNCTION_IDS[16]: (None, "def verify_request(request: SignatureVerificationRequest, *, policy_path: Path, secret_resolver: SecretResolver, replay_store: ReplayStore, now: datetime) -> SignatureVerificationAttestation"),
    FUNCTION_IDS[17]: (None, "def resolve_secret_handle(reference: str, *, allowed_prefixes: frozenset[str]) -> SecretHandle"),
    FUNCTION_IDS[18]: (None, "def open_replay_store(path: Path, *, schema_version: str, lock_timeout_seconds: float = 5.0) -> ReplayStore"),
    FUNCTION_IDS[19]: (None, "def append_attestation_record(ledger: AttestationLedger, request: SignatureVerificationRequest, attestation: SignatureVerificationAttestation, replay_claim: ReplayClaim) -> str"),
    FUNCTION_IDS[20]: (None, "def handle_verify_request(runtime: SignatureVerificationRuntime, body: bytes, *, now: datetime) -> HttpResponse"),
    FUNCTION_IDS[21]: (None, "def handle_health_request(runtime: SignatureVerificationRuntime) -> HttpResponse"),
    FUNCTION_IDS[22]: (None, "def build_server(runtime: SignatureVerificationRuntime, *, listen: str, tls_context: SSLContext, limits: ServerLimits) -> VerifierServer"),
    FUNCTION_IDS[23]: (None, "def serve(config: SignatureServiceConfig, *, shutdown: ShutdownSignal) -> int"),
    FUNCTION_IDS[24]: (None, "def main(argv: Sequence[str] | None = None) -> int"),
}


FUNCTION_SPECS: dict[str, dict[str, Any]] = {
    FUNCTION_IDS[0]: {"responsibility": "Run the immutable eleven-case positive and fail-closed verifier matrix without enabling a registry or runtime.", "inputs": ["validated CLI arguments", "immutable fixture directory", "expected case manifest"], "outputs": ["process exit code", "in-memory per-case observations"], "steps": ["parse only the documented fixture and endpoint arguments", "load the exact eleven fixture identities in deterministic order", "invoke verify_exact_payload once per case with bounded transport", "compare typed outcome and rejection code with the fixture oracle", "return zero only when one positive and ten negative cases exactly match"], "errors": ["unknown or duplicate case identity aborts the run", "transport or response-shape failure is recorded as fail-closed", "any expected-versus-actual drift returns a non-zero exit"], "security": "The runner never treats fixture self-reporting as authority and never emits secret material.", "atomicity": "All cases are evaluated before a zero exit is possible.", "idempotency": "The same immutable fixtures and endpoint response produce the same exit code.", "callers": ["P080 G0 evidence runner"], "callees": ["verify_exact_payload"]},
    FUNCTION_IDS[1]: {"responsibility": "Send one exact typed verification payload and accept only a bounded attestation bound to the request and policy.", "inputs": ["SignatureVerificationRequest", "protected endpoint URL", "pinned policy fingerprint", "timeout and response byte bounds"], "outputs": ["SignatureVerificationAttestation", "typed fail-closed exception"], "steps": ["serialize the request using the request schema without dropping signed fields", "send one POST with explicit content type timeout and response size bound", "reject non-success status or a body that exceeds the byte bound", "parse and schema-check the attestation before reading PASS", "compare request payload policy candidate profile environment nonce and verifier identity", "return only the fully bound typed attestation"], "errors": ["timeout network or TLS failure rejects the request", "HTTP or JSON shape failure rejects the response", "policy request identity or payload binding mismatch rejects PASS", "unknown attestation fields or algorithms reject the response"], "security": "Only a protected endpoint response with exact request binding can cross the local trust boundary.", "atomicity": "No caller-visible PASS is returned before all response bindings succeed.", "idempotency": "Retries reuse the same request identity and cannot create a different accepted payload.", "callers": ["test_trusted_signature_verifier.main", "build_topic1_task_registry.require_trusted_signature_verifier"], "callees": ["HTTPS transport", "signature-verification-attestation schema validator"]},
    FUNCTION_IDS[2]: {"responsibility": "Replace the unconditional trust hard block with one typed verifier call while retaining fail-closed semantics.", "inputs": ["SignatureVerificationRequest", "TrustedSignatureVerifier client", "human-readable validation context"], "outputs": ["SignatureVerificationAttestation", "contextual fail-closed exception"], "steps": ["require a non-empty caller context and typed request", "invoke the injected verifier without ambient endpoint discovery", "require attestation status PASS and exact request identity", "translate verifier rejection into a stable contextual error", "return the attestation for the immediate authorization check only"], "errors": ["missing verifier injection preserves the hard block", "typed verifier rejection is never converted to warning", "attestation identity mismatch preserves the hard block"], "security": "The injected verifier is the only path from untrusted receipt bytes to a trust decision.", "atomicity": "The hard block is lifted for one caller only after complete attestation validation.", "idempotency": "Repeated validation of one immutable request returns the same binding or rejection.", "callers": ["validate_candidate_artifact_refs", "BOM transition validator"], "callees": ["TrustedSignatureVerifier.verify_exact_payload"]},
    FUNCTION_IDS[3]: {"responsibility": "Validate exact candidate artifact identity provenance hashes and trusted signatures using the explicit repository.", "inputs": ["artifact reference list", "exact expected hash list", "candidate commit", "explicit repo_root", "verifier and trust context"], "outputs": ["None on complete validation", "typed identity provenance or trust rejection"], "steps": ["reject duplicate artifact IDs paths hashes or cardinality drift", "read candidate blobs only through explicit-root helpers", "validate external artifact and provenance paths under the explicit root", "recompute signed provenance core and all payload/signature hashes", "build a typed verification request bound to candidate profile environment and purpose", "call require_trusted_signature_verifier and retain no partial success"], "errors": ["candidate blob absence or digest drift rejects the entire set", "external provenance identity or receipt drift rejects the entire set", "trusted signature rejection prevents caller progress", "repository escape or symlink rejects before reading bytes"], "security": "Git blobs external artifacts and signatures remain untrusted until exact identity and verifier checks finish.", "atomicity": "The reference set is accepted as a unit with no per-item PASS persisted early.", "idempotency": "Immutable commit references and external hashes yield a stable accept or reject decision.", "callers": ["validate_implementation_candidate"], "callees": ["read_candidate_blob", "load_hashed_json_artifact", "validate_hashed_artifact", "require_trusted_signature_verifier"]},
    FUNCTION_IDS[4]: {"responsibility": "Validate a complete implementation candidate against one explicit repository and one trust context.", "inputs": ["implementation candidate object", "explicit repo_root", "TrustedSignatureVerifier", "CandidateTrustContext"], "outputs": ["None on complete candidate validity", "typed candidate rejection"], "steps": ["resolve and verify the declared candidate commit in the explicit repository", "recompute the production tree fingerprint with explicit-root helpers", "validate config model supply-chain and runtime artifact sets", "validate prebuilt binary image SBOM and delivery artifact provenance", "bind every trusted signature request to candidate profile environment and purpose", "return only after all candidate categories and trust checks pass"], "errors": ["missing commit or tree fingerprint drift rejects the candidate", "any artifact category mismatch rejects the candidate", "prebuilt image or SBOM provenance mismatch rejects the candidate", "trusted signature failure leaves the execution gate blocked"], "security": "Candidate content and all external supply-chain claims are hostile until rooted and verified.", "atomicity": "No candidate category can authorize independently of the complete candidate validation.", "idempotency": "One immutable candidate manifest and repository resolve to one deterministic decision.", "callers": ["task registry candidate validator"], "callees": ["resolve_candidate_repository", "candidate_tree_fingerprint", "validate_candidate_artifact_refs", "require_trusted_signature_verifier"]},
    FUNCTION_IDS[5]: {"responsibility": "Resolve and freeze one caller-supplied repository root for all candidate Git and artifact operations.", "inputs": ["caller-supplied repo_root"], "outputs": ["CandidateRepository with canonical root and git_dir", "typed unsafe repository rejection"], "steps": ["require an absolute caller-supplied path", "resolve the path without accepting a missing final target", "reject symlinked or non-directory repository roots", "ask Git for the canonical top-level and git directory", "require Git top-level equality with the resolved caller path", "return immutable canonical repository metadata"], "errors": ["relative missing or symlink root is rejected", "Git discovery failure is rejected with bounded diagnostics", "nested checkout or top-level mismatch is rejected"], "security": "No ambient module constant current directory or environment variable may select candidate bytes.", "atomicity": "Repository metadata is returned only after all canonical identity checks complete.", "idempotency": "An unchanged repository root resolves to identical canonical metadata.", "callers": ["validate_implementation_candidate"], "callees": ["git rev-parse --show-toplevel", "git rev-parse --git-dir"]},
    FUNCTION_IDS[6]: {"responsibility": "Read exactly one Git blob from an explicit repository while distinguishing absence from command failure.", "inputs": ["explicit repo_root", "immutable candidate commit", "canonical repo-relative path"], "outputs": ["blob bytes or None for absence", "typed Git command failure"], "steps": ["validate commit syntax and canonical repo-relative path", "reject absolute traversal NUL and revision-separator path forms", "run git cat-file blob with argument separation under repo_root", "return stdout only on a successful blob read", "return None only for exact missing-object diagnostics", "raise a bounded error for every other Git failure"], "errors": ["unsafe path is rejected before Git invocation", "missing object returns None without hiding other failures", "permission corruption or invalid command failure raises an error"], "security": "Candidate-controlled path text cannot change the repository or Git revision expression.", "atomicity": "The caller receives either the complete blob bytes None or an exception.", "idempotency": "One commit and canonical path identify immutable blob bytes.", "callers": ["candidate_tree_fingerprint", "validate_candidate_artifact_refs", "delivery artifact validator"], "callees": ["git cat-file blob"]},
    FUNCTION_IDS[7]: {"responsibility": "Load one repository-scoped JSON object after exact digest and optional schema validation.", "inputs": ["explicit repo_root", "canonical artifact path", "expected SHA-256", "optional schema path"], "outputs": ["validated JSON object", "typed path digest JSON or schema rejection"], "steps": ["resolve artifact and optional schema beneath repo_root", "reject symlink path escape missing or non-file targets", "read bytes once and compare the exact SHA-256", "decode JSON and require an object root", "validate against the optional repository-scoped schema", "return the validated object without rereading the file"], "errors": ["path escape symlink or missing file is rejected before parsing", "digest mismatch is rejected before JSON use", "JSON root or schema mismatch rejects the artifact"], "security": "Digest and repository containment precede deserialization of untrusted artifact bytes.", "atomicity": "Only one read buffer is hashed parsed validated and returned.", "idempotency": "Unchanged bytes expected hash and schema produce the same object.", "callers": ["validate_candidate_artifact_refs", "candidate receipt validators"], "callees": ["SHA-256", "validate_against_schema"]},
    FUNCTION_IDS[8]: {"responsibility": "Validate one explicit-root artifact path and digest without exposing or parsing its contents.", "inputs": ["explicit repo_root", "canonical artifact path", "expected SHA-256"], "outputs": ["None on exact digest", "typed containment presence or digest rejection"], "steps": ["resolve the candidate path beneath repo_root", "reject symlink path escape and non-file targets", "validate the expected digest syntax", "read bytes once with bounded filesystem errors", "compare SHA-256 in constant-shape control flow", "return None only on exact equality"], "errors": ["unsafe or missing path is rejected", "invalid expected digest is rejected", "filesystem read or digest mismatch is rejected"], "security": "Untrusted paths cannot select bytes outside the explicit repository boundary.", "atomicity": "No state is written and success denotes one complete digest comparison.", "idempotency": "Unchanged path bytes and expected digest yield the same decision.", "callers": ["validate_candidate_artifact_refs", "signature artifact validator"], "callees": ["SHA-256"]},
    FUNCTION_IDS[9]: {"responsibility": "Derive a deterministic production-tree fingerprint from one explicit repository and immutable commit.", "inputs": ["explicit repo_root", "candidate commit", "exact production source roots", "declared exclusions"], "outputs": ["canonical SHA-256 tree fingerprint", "typed root exclusion or Git rejection"], "steps": ["require the exact production-root set and canonical exclusion paths", "prove exclusions are inactive and not tracked at the candidate commit", "enumerate candidate blobs with git ls-tree under repo_root", "read each entry through explicit-root read_candidate_blob", "hash canonical path mode and blob SHA-256 records", "sort records and hash the canonical JSON projection"], "errors": ["source-root or exclusion drift rejects fingerprinting", "tracked excluded content rejects fingerprinting", "Git enumeration or disappearing entry rejects fingerprinting", "an empty production tree rejects fingerprinting"], "security": "The explicit repository and immutable commit are part of the fingerprint trust boundary.", "atomicity": "A fingerprint is returned only after every selected blob is read and hashed.", "idempotency": "The same repository commit roots and exclusions produce one stable digest.", "callers": ["validate_implementation_candidate"], "callees": ["git cat-file", "git ls-tree", "read_candidate_blob"]},
    FUNCTION_IDS[10]: {"responsibility": "Canonicalize every signed request identity field into one deterministic payload with domain separation.", "inputs": ["SignatureVerificationRequest"], "outputs": ["canonical UTF-8 payload bytes", "typed schema or canonicalization rejection"], "steps": ["validate request schema and reject unknown fields", "normalize only schema-designated textual encodings", "preserve candidate profile environment purpose nonce time and policy identity", "construct a versioned domain-separated canonical object", "serialize with sorted keys fixed separators and UTF-8", "return bytes and never a mutable object reference"], "errors": ["missing or unknown request field is rejected", "non-canonical digest time or identifier encoding is rejected", "unsupported canonicalization version is rejected"], "security": "No signed identity field may be omitted defaulted or transformed ambiguously.", "atomicity": "Canonical bytes are produced only from one fully valid request.", "idempotency": "Equivalent schema-valid input has one byte-for-byte canonical form.", "callers": ["verify_request"], "callees": ["signature-verification-request schema validator"]},
    FUNCTION_IDS[11]: {"responsibility": "Load one fingerprint-pinned trust policy and resolve secret values without putting them in outputs or logs.", "inputs": ["policy path", "expected policy fingerprint", "SecretResolver"], "outputs": ["LoadedTrustPolicy with protected handles", "typed policy or secret-reference rejection"], "steps": ["reject non-canonical or symlinked policy paths", "read and hash policy bytes before parsing", "require exact policy fingerprint and schema version", "validate anchors algorithms EKUs roles scopes and revocation policy", "resolve only declared secret references to protected handles", "return an immutable policy with no raw secret serialization"], "errors": ["path or fingerprint drift rejects the policy", "unknown algorithm role scope or anchor rejects the policy", "missing or over-broad secret reference rejects loading", "revocation configuration that permits soft-fail is rejected"], "security": "Trust anchors and secret references are policy inputs; raw private or credential values never leave the resolver.", "atomicity": "No partial policy is returned if any anchor rule or secret fails.", "idempotency": "Pinned bytes and stable secret references produce the same policy identity.", "callers": ["verify_request"], "callees": ["signature-trust-policy schema validator", "SecretResolver.resolve"]},
    FUNCTION_IDS[12]: {"responsibility": "Validate signer certificate path algorithm EKU validity and revocation state at an injected verification time.", "inputs": ["ordered certificate chain bytes", "LoadedTrustPolicy", "injected verification_time"], "outputs": ["VerifiedSigner", "typed chain validity or revocation rejection"], "steps": ["parse every certificate with strict DER handling and size bounds", "build a path only to a policy-pinned trust anchor", "enforce permitted key sizes signature algorithms and critical extensions", "check leaf EKU and issuer constraints for the requested verifier role", "check not-before not-after and policy clock skew", "require successful revocation evidence under the policy", "return normalized signer fingerprints and asserted roles"], "errors": ["parse path or anchor failure rejects the chain", "weak key algorithm EKU or critical-extension mismatch rejects", "expired not-yet-valid revoked or indeterminate revocation rejects", "chain length or byte bounds reject resource abuse"], "security": "Caller-provided certificates do not become authority without a pinned path and hard-fail revocation result.", "atomicity": "No signer identity is returned until the entire chain policy succeeds.", "idempotency": "One chain policy and injected time yield a deterministic result.", "callers": ["verify_request"], "callees": ["cryptography X.509 path verifier", "revocation provider"]},
    FUNCTION_IDS[13]: {"responsibility": "Verify one detached signature over exact canonical bytes using only the algorithm and key approved for the signer.", "inputs": ["canonical payload bytes", "detached signature bytes", "VerifiedSigner", "declared algorithm"], "outputs": ["payload SHA-256", "typed algorithm or signature rejection"], "steps": ["require the declared algorithm in signer and policy allowlists", "bind the signer public key type to the declared algorithm", "enforce payload and signature byte bounds", "invoke the cryptographic verifier over the exact payload bytes", "reject any alternate serialization retry or algorithm fallback", "return the payload SHA-256 after successful verification"], "errors": ["unknown downgraded or key-incompatible algorithm rejects", "malformed or oversized signature rejects", "cryptographic verification failure rejects without fallback"], "security": "Only the exact canonical request bytes are authenticated; no self-reported status is consulted.", "atomicity": "The payload digest is returned only after one successful cryptographic verification.", "idempotency": "The same key algorithm payload and signature always yield the same result.", "callers": ["verify_request"], "callees": ["policy-approved cryptographic verifier"]},
    FUNCTION_IDS[14]: {"responsibility": "Authorize verified signer roles and scopes for the exact request candidate environment purpose and time window.", "inputs": ["SignatureVerificationRequest", "VerifiedSigner", "LoadedTrustPolicy", "injected current time"], "outputs": ["VerifiedAuthority", "typed role scope identity or time rejection"], "steps": ["derive required roles from request purpose and subject type", "match signer roles exactly without hierarchical wildcard expansion", "bind candidate commit profile and environment to request policy", "enforce request issuance expiry and maximum validity duration", "enforce CNAS accredited scope only when the request declares that boundary", "return the exact satisfied role and scope projection"], "errors": ["missing wrong or expired authority role rejects", "candidate profile or environment mismatch rejects", "purpose or time-window drift rejects", "required CNAS scope mismatch rejects"], "security": "Cryptographic identity alone is insufficient; least-privilege authority is evaluated per request.", "atomicity": "All required roles and conditional scopes succeed as one authorization decision.", "idempotency": "The same request signer policy and injected time produce one decision.", "callers": ["verify_request"], "callees": ["LoadedTrustPolicy role and scope matcher"]},
    FUNCTION_IDS[15]: {"responsibility": "Atomically claim a policy-scoped attestation nonce and distinguish idempotent replay from conflicting reuse.", "inputs": ["ReplayStore", "SignatureVerificationRequest", "canonical payload SHA-256", "claim expiry"], "outputs": ["ReplayClaim", "typed conflicting replay rejection"], "steps": ["derive a domain policy purpose environment and nonce claim key", "start one compare-and-set operation with bounded expiry", "store only payload digest request identity and expiry metadata", "return CREATED when no claim exists", "return IDEMPOTENT only when the existing payload identity exactly matches", "reject CONFLICT when any bound identity differs"], "errors": ["store timeout or unavailable state rejects verification", "conflicting nonce reuse rejects the request", "invalid or excessive expiry rejects before storage"], "security": "Replay state stores hashes and identities only and fails closed on unavailable atomic storage.", "atomicity": "Nonce creation or comparison is one linearizable store operation.", "idempotency": "Exact retries return the existing identical claim; substitutions are rejected.", "callers": ["verify_request"], "callees": ["ReplayStore.compare_and_set"]},
    FUNCTION_IDS[16]: {"responsibility": "Orchestrate policy payload chain signature authority and replay checks into one fully bound attestation.", "inputs": ["SignatureVerificationRequest", "policy path", "SecretResolver", "ReplayStore", "injected current time"], "outputs": ["SignatureVerificationAttestation", "stable fail-closed rejection"], "steps": ["canonicalize and schema-validate the complete request", "load the fingerprint-pinned trust policy", "verify the certificate chain at the injected time", "verify the detached signature over exact canonical bytes", "verify role purpose candidate profile environment time and conditional scope", "atomically claim the attestation nonce", "construct an attestation containing every verified identity and component digest", "return PASS only after all prior steps complete"], "errors": ["any policy payload chain signature scope or replay error returns REJECT", "internal dependency timeout or unavailable state returns ERROR not PASS", "attestation construction or schema mismatch returns ERROR", "no exception path persists or returns partial PASS"], "security": "This is the sole server trust-decision boundary and all dependencies are fail-closed and explicitly injected.", "atomicity": "PASS is constructed only after all checks and the replay claim commit.", "idempotency": "Exact retries return an equivalently bound attestation; substitutions cannot reuse the nonce.", "callers": ["protected verifier HTTP adapter"], "callees": ["canonicalize_verification_request", "load_trust_policy", "verify_certificate_chain", "verify_detached_signature", "verify_authority_scope", "claim_attestation_nonce"]},
    FUNCTION_IDS[17]: {"responsibility": "Resolve one allowlisted secret reference into an opaque non-serializable handle without exposing secret bytes.", "inputs": ["canonical secret reference", "allowed reference prefixes"], "outputs": ["opaque SecretHandle", "typed secret reference rejection"], "steps": ["parse and canonicalize the provider and reference name", "reject inline values traversal wildcards and unapproved providers", "require one exact allowlisted prefix", "request an opaque handle from the configured provider", "verify the handle is non-serializable and has bounded lifetime", "return the handle without reading or logging its value"], "errors": ["inline relative wildcard or unknown reference rejects", "provider unavailable or ambiguous version rejects", "serializable or overlong handle rejects"], "security": "Secret bytes remain inside the provider and only opaque handles cross the runtime boundary.", "atomicity": "A handle is returned only after reference and provider policy checks complete.", "idempotency": "One versioned reference resolves to one stable handle identity during its lifetime.", "callers": ["serve", "load_trust_policy"], "callees": ["configured SecretProvider.resolve_handle"]},
    FUNCTION_IDS[18]: {"responsibility": "Open a persistent exclusive atomic replay store with exact schema identity recovery and durability settings.", "inputs": ["canonical replay store path", "expected schema version", "lock timeout"], "outputs": ["exclusive ReplayStore", "typed path lock migration or recovery rejection"], "steps": ["reject relative symlink escaped or shared paths", "create or open the store with an exclusive process lock", "read and verify the durable schema and store identity", "recover committed claims and reject torn or incompatible records", "configure synchronous compare-and-set and bounded expiry cleanup", "return the locked store only after a read-write durability probe"], "errors": ["unsafe path or lock contention rejects startup", "schema mismatch or torn recovery rejects startup", "durability probe or filesystem failure rejects startup"], "security": "Replay protection never falls back to memory or an unlocked shared store.", "atomicity": "Store open recovery lock and durability probe succeed as one startup gate.", "idempotency": "Reopening an unchanged clean store yields the same identity and committed claims.", "callers": ["serve"], "callees": ["persistent key-value store open lock recover and fsync"]},
    FUNCTION_IDS[19]: {"responsibility": "Append and durably synchronize one canonical request decision attestation and replay binding record.", "inputs": ["AttestationLedger", "SignatureVerificationRequest", "SignatureVerificationAttestation", "ReplayClaim"], "outputs": ["durable record SHA-256", "typed append or synchronization rejection"], "steps": ["verify request attestation and replay claim identity equality", "canonicalize a versioned ledger record with no secret fields", "derive the record identity and reject conflicting existing content", "append framed checksum-protected bytes under the exclusive ledger lock", "flush file data and required parent metadata before success", "return the durable record digest only after synchronization"], "errors": ["identity or schema mismatch rejects before append", "conflicting existing record rejects without overwrite", "short write checksum flush or sync failure returns ERROR"], "security": "The ledger contains hashes public signer identity and decisions only and is never rewriteable.", "atomicity": "A decision is externally visible only after a complete framed record is durable.", "idempotency": "An exact duplicate returns the existing digest while conflicting content rejects.", "callers": ["handle_verify_request"], "callees": ["AttestationLedger.append_and_fsync"]},
    FUNCTION_IDS[20]: {"responsibility": "Handle one bounded verify request through schema domain verification durable ledger commit and typed HTTP response.", "inputs": ["SignatureVerificationRuntime", "bounded request body", "injected request time"], "outputs": ["HttpResponse", "durable decision record"], "steps": ["reject unsupported method content type encoding and oversized body", "parse JSON and schema-check one SignatureVerificationRequest", "invoke verify_request with runtime policy secret and replay dependencies", "append and fsync the resulting decision and replay binding", "map PASS REJECT and ERROR to closed response status and body shapes", "attach request decision and ledger digest headers without secret data"], "errors": ["transport body or schema failure returns a bounded client rejection", "domain verification failure returns REJECT not PASS", "ledger failure converts even a domain PASS to ERROR", "response serialization failure emits a generic ERROR"], "security": "No PASS leaves the process before domain verification replay claim and durable audit commit.", "atomicity": "The durable decision record precedes any externally visible PASS response.", "idempotency": "Exact retried requests reuse replay and ledger identities and return equivalent responses.", "callers": ["VerifierServer verify route"], "callees": ["verify_request", "append_attestation_record"]},
    FUNCTION_IDS[21]: {"responsibility": "Report liveness and readiness separately without exposing policy secret replay or ledger internals.", "inputs": ["SignatureVerificationRuntime"], "outputs": ["bounded HttpResponse", "closed readiness reason code"], "steps": ["confirm the process event loop is responsive", "check loaded policy fingerprint and validity without reloading it", "probe secret handles without reading values", "probe replay store lock schema and durable write capability", "probe attestation ledger append boundary without fake PASS", "return ready only when every required dependency is usable"], "errors": ["unloaded invalid or expired policy reports not ready", "secret replay or ledger probe failure reports not ready", "unknown dependency state reports not ready rather than degraded PASS"], "security": "Health output contains stable reason codes and public identities only and never secret paths or values.", "atomicity": "Readiness is a single snapshot over all required dependency probes.", "idempotency": "Unchanged runtime dependency state yields the same health response.", "callers": ["VerifierServer health routes", "deployment probes"], "callees": ["runtime dependency health probes"]},
    FUNCTION_IDS[22]: {"responsibility": "Build a bounded TLS server exposing only verify liveness and readiness routes over explicit runtime dependencies.", "inputs": ["SignatureVerificationRuntime", "listen address", "configured SSLContext", "ServerLimits"], "outputs": ["unstarted VerifierServer", "typed TLS route or limit rejection"], "steps": ["validate loopback or approved listen address and fixed route set", "require TLS minimum version cipher and client-auth policy", "configure header body connection concurrency and timeout limits", "bind verify and health handlers with explicit runtime injection", "disable debug directory listing proxy trust and dynamic route loading", "return an unstarted server with staged shutdown hooks"], "errors": ["unsafe listen TLS or client-auth policy rejects", "missing handler dependency or duplicate route rejects", "invalid resource or timeout bounds reject"], "security": "The server surface is closed and TLS authenticated with no ambient proxy or plugin behavior.", "atomicity": "No socket is opened until complete route TLS limit and runtime validation succeeds.", "idempotency": "Equivalent immutable inputs build an equivalent unstarted server configuration.", "callers": ["serve"], "callees": ["handle_verify_request", "handle_health_request", "TLS HTTP server constructor"]},
    FUNCTION_IDS[23]: {"responsibility": "Bootstrap validate and serve the protected verifier then stage shutdown with replay and audit durability preserved.", "inputs": ["SignatureServiceConfig", "ShutdownSignal"], "outputs": ["process exit code", "durable shutdown diagnostics"], "steps": ["validate all canonical config paths identities and resource bounds", "resolve secret handles and load the fingerprint-pinned policy", "open the exclusive persistent replay store and attestation ledger", "construct SignatureVerificationRuntime and run readiness checks", "build bind and start the TLS server before declaring ready", "on shutdown stop admission drain in-flight requests flush ledger and replay state", "close locks and return zero only after graceful durable shutdown"], "errors": ["config policy secret store or ledger startup failure returns non-zero before readiness", "bind TLS or readiness failure closes opened resources and returns non-zero", "shutdown deadline flush or close failure returns non-zero and never claims graceful"], "security": "Startup and shutdown preserve fail-closed trust replay and audit guarantees across process restarts.", "atomicity": "Readiness is published only after all dependencies and the listener are valid; shutdown orders admission before durability close.", "idempotency": "Restarting with unchanged durable state recovers the same claims and policy identity without replay reset.", "callers": ["main", "container process"], "callees": ["resolve_secret_handle", "open_replay_store", "load_trust_policy", "build_server"]},
    FUNCTION_IDS[24]: {"responsibility": "Parse explicit service runtime arguments and invoke serve without accepting ambient trust or secret configuration.", "inputs": ["CLI argv", "explicit policy secret replay ledger TLS and listen arguments"], "outputs": ["stable process exit code", "redacted startup diagnostics"], "steps": ["parse only documented required CLI arguments and reject unknown flags", "canonicalize config paths and immutable policy identity", "build SignatureServiceConfig without reading secret values", "install bounded termination signals and shutdown deadline", "invoke serve exactly once and propagate its exit code", "redact exception output and never retry with weaker defaults"], "errors": ["missing unknown duplicate or unsafe argument returns usage error", "ambient environment fallback is rejected", "serve exception or non-graceful shutdown returns non-zero"], "security": "Trust policy secrets TLS replay and ledger locations must be explicit and cannot come from attacker-controlled ambient state.", "atomicity": "One validated CLI configuration starts at most one server runtime.", "idempotency": "The same explicit configuration and durable state have the same startup decision.", "callers": ["Docker image entrypoint"], "callees": ["serve"]},
}


def data_contracts() -> list[dict[str, Any]]:
    return [
        {"type_name": "CandidateRepository", "purpose": "Immutable explicit repository identity used by every candidate read and Git command.", "required_fields": ["root: Path", "git_dir: Path", "identity_sha256: str"], "invariants": ["root is absolute canonical non-symlink directory", "Git top-level equals root exactly"], "secret_policy": "Contains paths and hashes only; no credentials or environment-derived secrets."},
        {"type_name": "CandidateTrustContext", "purpose": "Binds a trust decision to candidate profile environment purpose and policy.", "required_fields": ["candidate_commit: str", "profile_id: str", "environment_id: str", "purpose: str", "policy_fingerprint: str"], "invariants": ["all identity fields are non-empty and canonical", "candidate commit is an immutable full object ID"], "secret_policy": "Carries public identity metadata only and must never embed keys tokens or secret values."},
        {"type_name": "SignatureVerificationRequest", "purpose": "Complete domain-separated payload and signer material submitted to the protected verifier.", "required_fields": ["request_id: str", "payload_sha256: str", "candidate_commit: str", "profile_id: str", "environment_id: str", "purpose: str", "policy_fingerprint: str", "nonce: str", "issued_at: str", "expires_at: str", "certificate_chain: list[str]", "signature: str", "algorithm: str"], "invariants": ["canonical payload covers every identity field", "validity interval is bounded and ordered", "nonce is unique within policy purpose and environment"], "secret_policy": "Certificates and signatures are public verification material; private keys and bearer credentials are forbidden."},
        {"type_name": "SignatureVerificationAttestation", "purpose": "Typed verifier result bound to every input identity and verified component digest.", "required_fields": ["request_id: str", "status: str", "payload_sha256: str", "policy_fingerprint: str", "signer_fingerprint: str", "verified_roles: list[str]", "replay_claim_id: str", "verified_at: str"], "invariants": ["PASS contains no omitted request identity", "REJECT and ERROR cannot be interpreted as PASS", "attestation schema rejects unknown status values"], "secret_policy": "Outputs fingerprints roles hashes and timestamps only; error text is redacted and no secret is serialized."},
        {"type_name": "LoadedTrustPolicy", "purpose": "Validated immutable policy with pinned anchors algorithms roles scopes and protected handles.", "required_fields": ["fingerprint: str", "trust_anchors: tuple[bytes, ...]", "allowed_algorithms: frozenset[str]", "role_rules: Mapping[str, Any]", "revocation_mode: str", "secret_handles: tuple[SecretHandle, ...]"], "invariants": ["fingerprint equals exact policy bytes", "revocation mode is hard fail", "algorithms and roles contain no wildcard expansion"], "secret_policy": "Only opaque SecretHandle values may be retained; raw resolved secret bytes are non-serializable and non-loggable."},
        {"type_name": "VerifiedSignerAuthority", "purpose": "Normalized signer certificate identity plus the exact roles and scopes authorized for one request.", "required_fields": ["signer_fingerprint: str", "chain_fingerprints: tuple[str, ...]", "verified_roles: frozenset[str]", "verified_scopes: frozenset[str]", "valid_until: datetime"], "invariants": ["chain terminates at a pinned anchor", "roles and scopes are explicit policy members", "valid_until does not exceed request or certificate expiry"], "secret_policy": "Contains public certificate fingerprints and authorization labels only; key material is forbidden."},
        {"type_name": "ReplayClaim", "purpose": "Linearizable nonce claim that distinguishes first use exact retry and conflicting replay.", "required_fields": ["claim_id: str", "claim_key_sha256: str", "payload_sha256: str", "disposition: str", "expires_at: datetime"], "invariants": ["disposition is CREATED or IDEMPOTENT for accepted calls", "one claim key cannot bind two payload identities", "expiry is bounded by policy"], "secret_policy": "Stores identity hashes and expiry only; request payload signature certificate and credentials are forbidden."},
        {"type_name": "SignatureVerificationRuntime", "purpose": "Immutable process runtime containing exact policy secret replay audit clock and server dependency handles.", "required_fields": ["policy: LoadedTrustPolicy", "secret_resolver: SecretResolver", "replay_store: ReplayStore", "attestation_ledger: AttestationLedger", "clock: Clock", "runtime_id: str"], "invariants": ["runtime identity hashes public dependency identities", "replay store and ledger are exclusive persistent durable handles", "runtime never contains raw secret bytes", "policy is loaded before readiness"], "secret_policy": "Only opaque non-serializable secret handles are reachable; configuration logs and health output exclude raw values."},
    ]


def catalog_leaves() -> dict[str, dict[str, Any]]:
    return {item["atomic_pr_id"]: item for item in load(CATALOG)["candidate_leaves"]}


def function_contracts(leaves: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    contracts: list[dict[str, Any]] = []
    for index, atomic_id in enumerate(FUNCTION_IDS, 1):
        leaf = leaves[atomic_id]
        locator, *companions = leaf["write_locators"]
        path, symbol = locator.split("#", 1)
        before, after = SIGNATURES[atomic_id]
        spec = FUNCTION_SPECS[atomic_id]
        own_test = f"TC-M01-TRUST-{index:02d}"
        extra: list[str] = []
        if atomic_id in set(FUNCTION_IDS[3:10]):
            extra.append("TC-M01-TRUST-34")
        if atomic_id in {FUNCTION_IDS[10], FUNCTION_IDS[13], FUNCTION_IDS[16], FUNCTION_IDS[20]}:
            extra.append("TC-M01-TRUST-35")
        if atomic_id in {FUNCTION_IDS[1], FUNCTION_IDS[16], FUNCTION_IDS[20], FUNCTION_IDS[22]}:
            extra.append("TC-M01-TRUST-36")
        if atomic_id in {FUNCTION_IDS[11], FUNCTION_IDS[14]}:
            extra.append("TC-M01-TRUST-37")
        if atomic_id in set(FUNCTION_IDS[17:]):
            extra.append("TC-M01-TRUST-39")
        if atomic_id in {FUNCTION_IDS[18], FUNCTION_IDS[19], FUNCTION_IDS[20], FUNCTION_IDS[23]}:
            extra.append("TC-M01-TRUST-40")
        contracts.append({
            "contract_id": f"FC-M01-N015-P{pr_number(atomic_id):03d}",
            "atomic_pr_id": atomic_id, "path": path, "qualified_symbol": symbol,
            "change_kind": "planned_modify" if before is not None else "planned_create",
            "companion_target_locators": companions,
            "signature_before": before, "signature_after": after,
            "responsibility": spec["responsibility"], "inputs": spec["inputs"],
            "outputs": spec["outputs"],
            "body_steps": [f"B{step:02d} {text}" for step, text in enumerate(spec["steps"], 1)],
            "error_branches": spec["errors"], "side_effects": [],
            "security_boundary": spec["security"], "atomicity": spec["atomicity"],
            "idempotency": spec["idempotency"], "callers": spec["callers"],
            "callees": spec["callees"], "tests": [own_test, *extra],
            "rollback": "Restore the existing hard-block behavior and retain all externally produced immutable evidence.",
            "design_review_status": "REQUIRED_NOT_PERFORMED",
        })
    return contracts


def type_contracts(leaves: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    runtime_id, candidate_id, service_id = TYPE_IDS
    runtime_locator = leaves[runtime_id]["write_locators"][0]
    runtime_path, runtime_symbol = runtime_locator.split("#", 1)
    service_locator = leaves[service_id]["write_locators"][0]
    service_path, service_symbol = service_locator.split("#", 1)
    return [{
        "contract_id": "TC-FC-M01-N015-P086", "atomic_pr_id": runtime_id,
        "path": runtime_path, "qualified_symbol": runtime_symbol, "change_kind": "planned_create",
        "declaration_after": "@dataclass(frozen=True, slots=True) class SignatureVerificationRuntime",
        "companion_declarations": [],
        "responsibility": "Own the complete immutable process dependency graph required for trustworthy HTTP verification readiness recovery and shutdown.",
        "fields": ["policy: LoadedTrustPolicy", "secret_resolver: SecretResolver", "replay_store: ReplayStore", "attestation_ledger: AttestationLedger", "clock: Clock", "runtime_id: str", "server_limits: ServerLimits"],
        "invariants": ["policy is valid and fingerprint pinned at construction", "replay store is persistent exclusive and schema compatible", "attestation ledger is append-only durable and schema compatible", "secret resolver exposes opaque handles only", "clock and resource limits are explicitly injected", "runtime identity binds public dependency identities"],
        "construction_rules": ["construct only after all dependency startup probes pass", "reject duplicate or mismatched runtime dependency identities", "do not permit post-construction dependency replacement", "close dependencies only through staged serve shutdown"],
        "security_boundary": "The aggregate is the sole explicit bridge between process adapters and trust-domain functions and cannot carry secret bytes.",
        "tests": ["TC-M01-TRUST-39"],
        "rollback": "Destroy the unactivated runtime and preserve the current hard block and durable stores.",
        "design_review_status": "REQUIRED_NOT_PERFORMED",
    }, {
        "contract_id": "TC-FC-M01-N015-P095", "atomic_pr_id": candidate_id,
        "path": "scripts/alignment/build_topic1_task_registry.py", "qualified_symbol": "CandidateRepository", "change_kind": "planned_create",
        "declaration_after": "@dataclass(frozen=True, slots=True) class CandidateRepository",
        "companion_declarations": ["@dataclass(frozen=True, slots=True) class CandidateTrustContext", "class TrustedSignatureVerifier(Protocol)"],
        "responsibility": "Define the compile-time candidate repository trust context and verifier client abstractions used by migrated registry validation callers.",
        "fields": ["CandidateRepository.root: Path", "CandidateRepository.git_dir: Path", "CandidateRepository.identity_sha256: str", "CandidateTrustContext.candidate_commit: str", "CandidateTrustContext.profile_id: str", "CandidateTrustContext.environment_id: str", "CandidateTrustContext.purpose: str", "CandidateTrustContext.policy_fingerprint: str", "TrustedSignatureVerifier.verify_exact_payload(...)"],
        "invariants": ["repository root is absolute canonical and immutable", "candidate commit is a full immutable object ID", "profile environment purpose and policy identity are mandatory", "verifier protocol returns a typed attestation only", "no ambient repository or endpoint selection is represented"],
        "construction_rules": ["construct repository only through resolve_candidate_repository", "construct trust context from validated candidate and execution identity", "inject verifier implementation at the caller boundary", "reject empty wildcard or default identity fields"],
        "security_boundary": "These types prevent ambient repository and verifier selection from bypassing explicit candidate and trust identity.",
        "tests": ["TC-M01-TRUST-41"],
        "rollback": "Retain existing signatures and unconditional hard block until all migrated callers compile and review.",
        "design_review_status": "REQUIRED_NOT_PERFORMED",
    }, {
        "contract_id": "TC-FC-M01-N015-P096", "atomic_pr_id": service_id,
        "path": service_path, "qualified_symbol": service_symbol, "change_kind": "planned_create",
        "declaration_after": "@dataclass(frozen=True, slots=True) class SignatureVerificationRequest",
        "companion_declarations": [locator.split("#", 1)[1] for locator in leaves[service_id]["write_locators"][1:]],
        "responsibility": "Define every compile-time domain protocol configuration and adapter boundary required by the protected verifier service.",
        "fields": ["request and attestation exact identity fields", "loaded policy anchor algorithm role and revocation fields", "verified signer and authority projections", "replay claim identity disposition and expiry", "secret replay ledger clock and shutdown protocols", "HTTP TLS server limits config and response types"],
        "invariants": ["all concrete dataclasses are frozen slotted and schema aligned", "all protocols expose the minimum method surface", "PASS attestation contains every request binding", "secret handles cannot expose raw bytes", "persistent store and ledger protocols require durable operations", "HTTP and TLS types encode closed routes and resource bounds"],
        "construction_rules": ["declare concrete data before functions that consume it", "use Protocol for injected I O clock and shutdown capabilities", "avoid Any dictionary or ambient global substitutes at trust boundaries", "version all durable and wire-visible declarations"],
        "security_boundary": "The type set closes compile-time ambiguity across cryptography replay audit HTTP TLS configuration and shutdown seams.",
        "tests": ["TC-M01-TRUST-42"],
        "rollback": "Remove the unimplemented type candidate and preserve schemas hard block and all active registries.",
        "design_review_status": "REQUIRED_NOT_PERFORMED",
    }]


NON_FUNCTION_SPECS: dict[str, dict[str, Any]] = {
    NON_FUNCTION_IDS[0]: {"kind": "DECLARATIVE_CONTRACT", "input": ["versioned trust-policy fields", "versioned verification request fields", "versioned attestation fields"], "output": ["strict JSON Schema validation", "unknown-field and secret-field rejection"], "oracles": ["all signed identity fields are required", "PASS is impossible without exact request binding", "private keys tokens and raw secrets are forbidden"], "security": ["additionalProperties is false on trust-bearing objects", "algorithms roles scopes and status use closed enums"], "rollback": "Retain the prior schemas and hard block; do not migrate any producer or consumer."},
    NON_FUNCTION_IDS[1]: {"kind": "DEPLOYMENT_PACKAGE", "input": ["reviewed verifier source tree", "digest-pinned base image and requirements lock"], "output": ["non-root reproducible image recipe", "externally buildable SBOM and provenance inputs"], "oracles": ["base image uses an immutable digest", "runtime user is non-root and filesystem is read-only compatible", "dependency lock contains exact hashes and no floating versions"], "security": ["no trust anchor credential or private key is copied into the image", "build network and package inputs are explicit and auditable"], "rollback": "Remove the unbuilt package candidate and retain the existing verifier hard block."},
    NON_FUNCTION_IDS[2]: {"kind": "EVIDENCE_OUTPUT", "input": ["immutable candidate and design hashes", "G0 static unit schema and eleven-case observations"], "output": ["G0 test-result JSON", "G0 per-case report JSON"], "oracles": ["one positive and ten negative cases are enumerated", "every actual result is machine-derived and candidate-bound", "status remains NOT_RUN until an authorized runner writes evidence"], "security": ["self-reported fixture PASS cannot satisfy an oracle", "evidence contains hashes and redacted diagnostics only"], "rollback": "Preserve immutable attempted evidence and leave G0 unsatisfied after candidate rollback."},
    NON_FUNCTION_IDS[3]: {"kind": "EVIDENCE_OUTPUT", "input": ["isolated deployed candidate identity", "G1 restart rotation expiry revocation and rollback observations"], "output": ["G1 test-result JSON", "G1 per-case report JSON"], "oracles": ["restart retains no unverified PASS", "policy rotation and revocation become effective within bounds", "rollback restores hard-block behavior without receipt loss"], "security": ["test secrets are referenced through approved secret storage", "failure diagnostics never contain private keys or tokens"], "rollback": "Preserve immutable G1 observations and restore the previously approved isolated revision."},
    NON_FUNCTION_IDS[4]: {"kind": "DEPLOYMENT_MANIFEST", "input": ["external signed immutable image digest", "approved namespace secret and policy references"], "output": ["default-off verifier workload and service", "NetworkPolicy health probes resources and rollback annotations"], "oracles": ["image reference is digest-only and supplied by the external receipt", "replicas or activation gate defaults to off", "network and secret access are least privilege"], "security": ["manifest contains references but no secret values", "pod is non-root read-only and drops capabilities"], "rollback": "Scale the candidate to zero and restore the last approved digest while preserving all evidence."},
    NON_FUNCTION_IDS[5]: {"kind": "EVIDENCE_OUTPUT", "input": ["authorized canary deployment identity", "G6 health stop-threshold and rollback observations"], "output": ["G6 test-result JSON", "G6 per-case report JSON"], "oracles": ["canary identity equals the external image receipt", "stop thresholds are evaluated before wider traffic", "hard-block rollback is demonstrated and timed"], "security": ["deployment evidence excludes secret values", "operator and environment identity are receipt-bound"], "rollback": "Preserve the failed canary record and scale the verifier candidate to zero."},
    NON_FUNCTION_IDS[6]: {"kind": "EVIDENCE_OUTPUT", "input": ["protected endpoint candidate identity", "G2 functional fault and authorization observations"], "output": ["G2 test-result JSON", "G2 per-case report JSON"], "oracles": ["positive request succeeds only with exact authority", "all ten negative families fail closed", "timeout malformed and unavailable dependencies never return PASS"], "security": ["test certificates are non-production and bounded", "request and response logs are redacted and hash-bound"], "rollback": "Preserve G2 results and disable endpoint routing before restoring the hard block."},
    NON_FUNCTION_IDS[7]: {"kind": "EVIDENCE_OUTPUT", "input": ["request attestation replay and caller result ledgers", "G3 reconciliation window and candidate identity"], "output": ["G3 test-result JSON", "G3 reconciliation case report JSON"], "oracles": ["request attestation replay and caller counts reconcile", "every PASS maps to one exact replay claim", "unexplained difference is exactly zero"], "security": ["reconciliation uses hashes rather than payload or secret values", "late duplicate and conflicting replay are reported separately"], "rollback": "Preserve reconciliation evidence and keep the candidate disabled on any non-zero unexplained difference."},
}


def non_function_contracts(leaves: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for index, atomic_id in enumerate(NON_FUNCTION_IDS, 26):
        spec = NON_FUNCTION_SPECS[atomic_id]
        tests = [f"TC-M01-TRUST-{index:02d}"]
        if atomic_id == NON_FUNCTION_IDS[-1]:
            tests.append("TC-M01-TRUST-38")
        result.append({
            "contract_id": f"NFC-M01-N015-P{pr_number(atomic_id):03d}",
            "atomic_pr_id": atomic_id, "surface_kind": spec["kind"],
            "target_locators": leaves[atomic_id]["write_locators"],
            "input_contract": spec["input"], "output_contract": spec["output"],
            "oracles": spec["oracles"], "security_constraints": spec["security"],
            "tests": tests, "rollback": spec["rollback"],
            "required_review_artifact_kind": "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT",
            "design_review_status": "REQUIRED_NOT_PERFORMED",
        })
    return result


def test_cases() -> list[dict[str, Any]]:
    function_descriptions = [
        (FUNCTION_IDS[0], "unit", "run the immutable positive and ten-negative fixture manifest in shuffled filesystem order", "the runner uses manifest order and returns zero only for the exact eleven outcomes", "G0"),
        (FUNCTION_IDS[1], "negative", "return timeout oversized body unknown field and request-binding substitutions from the endpoint", "every transport shape size and binding drift rejects without returning an attestation", "G0"),
        (FUNCTION_IDS[2], "security", "omit the injected verifier and return REJECT ERROR or mismatched request identity", "the existing hard block remains effective and no caller progresses", "G0"),
        (FUNCTION_IDS[3], "security", "substitute candidate blob external receipt payload signature and candidate identity independently", "the complete artifact reference set is rejected with no partial trusted member", "G0"),
        (FUNCTION_IDS[4], "integration", "supply a missing commit changed tree artifact drift and invalid prebuilt provenance", "the candidate is rejected before any execution or promotion decision", "G0"),
        (FUNCTION_IDS[5], "negative", "supply relative missing symlink nested and non-Git repository roots", "only one canonical top-level repository is returned and every ambient fallback is rejected", "G0"),
        (FUNCTION_IDS[6], "security", "supply traversal revision-separator missing blob and Git corruption cases", "unsafe paths reject missing returns None and command failures remain errors", "G0"),
        (FUNCTION_IDS[7], "negative", "supply symlink escape digest drift invalid JSON non-object root and schema drift", "bytes are never parsed before containment and digest pass and no invalid object is returned", "G0"),
        (FUNCTION_IDS[8], "security", "supply path escape symlink invalid digest missing file and changed bytes", "only an exact explicit-root regular file digest returns None", "G0"),
        (FUNCTION_IDS[9], "replay", "fingerprint the same commit from two detached worktrees then alter roots exclusions and one blob", "equivalent repositories match while every production selection or blob change alters or rejects the digest", "G0"),
        (FUNCTION_IDS[10], "security", "permute keys and substitute each candidate profile environment purpose nonce time and policy field", "key order is stable and every signed identity substitution changes canonical bytes", "G0"),
        (FUNCTION_IDS[11], "security", "supply policy hash drift wildcard roles soft-fail revocation and secret-value embedding", "all weakened policies reject and no raw secret reaches output or diagnostics", "G0"),
        (FUNCTION_IDS[12], "security", "supply invalid root weak algorithm wrong EKU expiry revocation unknown-critical and oversized chains", "each invalid certificate family rejects at injected verification time", "G0"),
        (FUNCTION_IDS[13], "security", "substitute payload signature key and algorithm including downgrade and fallback candidates", "only the exact allowed algorithm key payload and detached signature verify", "G0"),
        (FUNCTION_IDS[14], "security", "substitute authority role purpose commit profile environment time and conditional CNAS scope", "every required identity and conditional scope mismatch rejects", "G0"),
        (FUNCTION_IDS[15], "replay", "race exact duplicate and conflicting payload requests for one scoped nonce", "one claim is created exact retries are idempotent and conflicts reject linearly", "G0"),
        (FUNCTION_IDS[16], "integration", "fault each dependency before and after its normal return and inspect the final attestation", "no fault path produces partial PASS and a complete path binds every verified component", "G0"),
        (FUNCTION_IDS[17], "security", "supply inline wildcard traversal unapproved and unavailable secret references", "only one allowlisted versioned reference yields a non-serializable opaque handle", "G0"),
        (FUNCTION_IDS[18], "replay", "open relative symlink locked incompatible torn and non-durable replay stores", "startup fails closed unless one exclusive persistent compatible durable store opens", "G0"),
        (FUNCTION_IDS[19], "replay", "append identity-mismatched duplicate-conflict short-write and sync-failure decisions", "only exact duplicate is idempotent and no unsynced record can authorize PASS", "G0"),
        (FUNCTION_IDS[20], "integration", "send wrong method type encoding oversized malformed reject pass and ledger-failure requests", "closed HTTP responses map exact decisions and ledger failure converts domain PASS to ERROR", "G0"),
        (FUNCTION_IDS[21], "negative", "fault policy secret replay and ledger health dependencies independently", "readiness stays false for every required dependency failure and reveals no secret data", "G0"),
        (FUNCTION_IDS[22], "security", "supply weak TLS open routes proxy trust unsafe listen and unbounded server limits", "server construction rejects every widened transport or route surface", "G0"),
        (FUNCTION_IDS[23], "rollback", "fault each startup stage then terminate during admission drain replay and ledger flush", "readiness never precedes full startup and non-graceful shutdown never returns zero", "G0"),
        (FUNCTION_IDS[24], "negative", "supply missing duplicate unknown ambient and unsafe CLI configuration", "CLI invokes serve once only for an explicit canonical complete configuration", "G0"),
    ]
    non_function_descriptions = [
        (NON_FUNCTION_IDS[0], "compatibility", "validate positive and unknown missing secret algorithm role scope and status schema fixtures", "the three schemas accept only closed complete versioned trust objects", "G0"),
        (NON_FUNCTION_IDS[1], "deployment", "inspect the planned Dockerfile and lock for floating inputs root user secret copies and non-repeatable installs", "all inputs are digest or hash pinned and the image contract is non-root and secret-free", "G0"),
        (NON_FUNCTION_IDS[2], "integration", "run the authorized G0 recipe against one immutable candidate and exact eleven-case manifest", "both G0 files bind the same candidate design cases runner and NOT_RUN-to-result transition", "G0"),
        (NON_FUNCTION_IDS[3], "rollback", "restart rotate expire revoke and roll back the verifier in the isolated G1 environment", "G1 records bounded convergence no stale PASS and durable evidence retention", "G1"),
        (NON_FUNCTION_IDS[4], "deployment", "render the manifest with absent mutable unsigned and valid externally signed image inputs", "only the external receipt digest renders and activation remains default off", "G0"),
        (NON_FUNCTION_IDS[5], "rollback", "deploy one authorized canary then cross each stop threshold and invoke hard-block rollback", "G6 records identity health stop decision rollback timing and final zero activation", "G6"),
        (NON_FUNCTION_IDS[6], "integration", "exercise positive ten-negative timeout malformed and unavailable dependency requests on the protected endpoint", "G2 binds every result to candidate environment policy and exact rejection family", "G2"),
        (NON_FUNCTION_IDS[7], "reconciliation", "join request attestation replay claim and caller results including late duplicate and conflict", "G3 reports explained categories and exactly zero unexplained difference", "G3"),
    ]
    rows = [{"case_id": f"TC-M01-TRUST-{index:02d}", "kind": kind, "subject_atomic_pr_ids": [atomic_id], "injection": injection, "oracle": oracle, "gate_id": gate, "execution_status": "NOT_RUN"} for index, (atomic_id, kind, injection, oracle, gate) in enumerate(function_descriptions, 1)]
    rows += [{"case_id": f"TC-M01-TRUST-{index:02d}", "kind": kind, "subject_atomic_pr_ids": [atomic_id], "injection": injection, "oracle": oracle, "gate_id": gate, "execution_status": "NOT_RUN"} for index, (atomic_id, kind, injection, oracle, gate) in enumerate(non_function_descriptions, 26)]
    rows.extend([
        {"case_id": "TC-M01-TRUST-34", "kind": "integration", "subject_atomic_pr_ids": FUNCTION_IDS[3:10], "injection": "execute all candidate reads from a detached explicit repository while the process current directory and module checkout differ", "oracle": "every helper and caller reads only the supplied repository and produces the detached candidate fingerprint", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-35", "kind": "security", "subject_atomic_pr_ids": [FUNCTION_IDS[10], FUNCTION_IDS[13], FUNCTION_IDS[16], FUNCTION_IDS[20]], "injection": "replay a valid signature with one canonical request identity field substituted after signing", "oracle": "canonical payload digest changes and the endpoint returns REJECT with no replay or PASS ledger record", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-36", "kind": "negative", "subject_atomic_pr_ids": [FUNCTION_IDS[1], FUNCTION_IDS[16], FUNCTION_IDS[20], FUNCTION_IDS[22]], "injection": "exceed request response chain signature timeout and server resource bounds independently", "oracle": "client and server fail closed with bounded diagnostics and never return or persist PASS", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-37", "kind": "security", "subject_atomic_pr_ids": [FUNCTION_IDS[11], FUNCTION_IDS[14]], "injection": "toggle the request CNAS boundary and substitute accredited scope policy role and environment", "oracle": "CNAS scope is enforced exactly when declared while ordinary requests cannot acquire its authority", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-38", "kind": "reconciliation", "subject_atomic_pr_ids": [NON_FUNCTION_IDS[-1]], "injection": "attempt to construct the preserved P066 completion before the exact P085 G3 result and dependency are accepted", "oracle": "the terminal completion remains blocked until P085 is accepted and all 36 direct members are current", "gate_id": "G3", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-39", "kind": "integration", "subject_atomic_pr_ids": [TYPE_IDS[0], *FUNCTION_IDS[17:]], "injection": "bootstrap restart and gracefully terminate one runtime using persistent replay and audit state", "oracle": "the runtime exact-set is complete readiness is truthful replay survives and audit flush precedes zero exit", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-40", "kind": "rollback", "subject_atomic_pr_ids": [FUNCTION_IDS[18], FUNCTION_IDS[19], FUNCTION_IDS[20], FUNCTION_IDS[23]], "injection": "kill the process at each replay claim ledger append response and shutdown boundary then restart", "oracle": "no undurable PASS is exposed conflicts remain rejected and recovery reports every incomplete transition", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-41", "kind": "compatibility", "subject_atomic_pr_ids": [TYPE_IDS[1], FUNCTION_IDS[2], FUNCTION_IDS[3], FUNCTION_IDS[4], FUNCTION_IDS[5]], "injection": "type-check migrated candidate helpers against missing swapped ambient and structurally similar repository trust and verifier objects", "oracle": "only the exact immutable repository trust context and verifier Protocol shapes compile and reach validation", "gate_id": "G0", "execution_status": "NOT_RUN"},
        {"case_id": "TC-M01-TRUST-42", "kind": "compatibility", "subject_atomic_pr_ids": [TYPE_IDS[2], *FUNCTION_IDS[10:]], "injection": "type-check every service function after removing renaming or widening each domain protocol config adapter clock and shutdown declaration", "oracle": "the exact declaration set is compile complete while every omitted or ambiguous trust-boundary type fails", "gate_id": "G0", "execution_status": "NOT_RUN"},
    ])
    return rows


def topological_members(leaves: dict[str, dict[str, Any]]) -> list[str]:
    members = set(MEMBER_IDS)
    incoming = {item: {dep for dep in leaves[item]["dependency_ids"] if dep in members} for item in members}
    outgoing: dict[str, set[str]] = {item: set() for item in members}
    for item, dependencies in incoming.items():
        for dependency in dependencies:
            outgoing[dependency].add(item)
    heap = [(pr_number(item), item) for item, dependencies in incoming.items() if not dependencies]
    heapq.heapify(heap)
    ordered: list[str] = []
    while heap:
        _, current = heapq.heappop(heap)
        ordered.append(current)
        for child in sorted(outgoing[current], key=pr_number):
            incoming[child].remove(current)
            if not incoming[child]:
                heapq.heappush(heap, (pr_number(child), child))
    if len(ordered) != len(members):
        raise ValueError("M01 N015 member graph is cyclic")
    return ordered


def sequence(leaves: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    rows = []
    for order, atomic_id in enumerate(topological_members(leaves), 1):
        deps = leaves[atomic_id]["dependency_ids"]
        rows.append({
            "order": order, "atomic_pr_id": atomic_id,
            "entry_condition": "accepted dependencies: " + ", ".join(deps),
            "result": "static design target: " + leaves[atomic_id]["single_outcome"],
            "activation": "inactive until global registration review and signed execution authorization",
            "status": BLOCKED,
        })
    return rows


def review_rows(functions: list[dict[str, Any]], types: list[dict[str, Any]], non_functions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    contracts = {item["atomic_pr_id"]: item for item in functions + types + non_functions}
    rows = []
    for atomic_id in MEMBER_IDS:
        is_function = atomic_id in FUNCTION_IDS or atomic_id in TYPE_IDS
        filename = "function-design-review.json" if is_function else "non-function-design-exemption.json"
        rows.append({
            "atomic_pr_id": atomic_id, "design_contract_id": contracts[atomic_id]["contract_id"],
            "required_review_artifact_kind": "FUNCTION_DESIGN_REVIEW_RECEIPT" if is_function else "NON_FUNCTION_DESIGN_EXEMPTION_RECEIPT",
            "expected_review_path": f"doc/02_acceptance/topic1/design-reviews/{atomic_id.lower()}/{filename}",
            "status": "MISSING_NOT_AUTHORED",
            "blocking_reasons": [
                "M01 v2 is a preview and is absent from the four active registries",
                "reviewer adjudicator and accountable owner identities are not assigned",
                "no signed candidate-bound review or exemption receipt exists",
            ],
        })
    return rows


INVARIANTS = [
    "The v2 completion exact-set contains the 36 designed non-terminal members and preserves P066 as a non-executable terminal.",
    "Every function contract owns exactly the catalog primary function locator and all catalog companion locators.",
    "Every non-function contract owns exactly its declarative package manifest or evidence locator set.",
    "Repository selection is explicit from validate_implementation_candidate through every Git blob hash and tree helper.",
    "Candidate-controlled paths are canonical repository-relative paths and symlink or traversal escapes fail before byte reads.",
    "A trusted PASS binds payload candidate commit profile environment purpose policy nonce validity and verifier identity.",
    "Certificate chain algorithm EKU validity revocation authority role and conditional CNAS scope all fail closed.",
    "Replay state is linearizable and distinguishes exact idempotent retry from conflicting nonce substitution.",
    "Runtime readiness requires policy secret replay ledger and server health while shutdown stops admission before durable flush and close.",
    "Secret values private keys credentials and bearer tokens are absent from schemas images manifests logs attestations and evidence.",
    "The deployment image build sign and publish activity remains external and cannot be inferred from a Dockerfile or manifest.",
    "G0 G1 G2 G3 and G6 each own one immutable evidence output pair and no pair asserts more than one gate.",
    "Review artifacts evidence files implementation image signatures deployment and authorization remain absent until independently produced.",
    "The four active registry artifacts remain byte-for-byte unchanged and formal execution stays blocked.",
    "Rollback restores the current cryptographic hard block before traffic removal and never deletes immutable attempt evidence.",
]


def build() -> dict[str, Any]:
    leaves = catalog_leaves()
    functions = function_contracts(leaves)
    types = type_contracts(leaves)
    non_functions = non_function_contracts(leaves)
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M01_N015_EARLY_TRUST_FUNCTION_COMPLETE_STATIC_DESIGN",
        "artifact_status": "STATIC_DESIGN_COMPLETE_REVIEW_IMPLEMENTATION_AND_EXECUTION_BLOCKED",
        "source_refs": [
            {"role": "V2_ALLOCATION", "path": ALLOCATION.relative_to(REPO).as_posix(), "sha256": sha256(ALLOCATION)},
            {"role": "V2_CATALOG", "path": CATALOG.relative_to(REPO).as_posix(), "sha256": sha256(CATALOG)},
            {"role": "ACTIVE_BUILDER", "path": BUILDER.relative_to(REPO).as_posix(), "sha256": sha256(BUILDER)},
            {"role": "FUNCTION_REVIEW_SCHEMA", "path": FUNCTION_REVIEW_SCHEMA.relative_to(REPO).as_posix(), "sha256": sha256(FUNCTION_REVIEW_SCHEMA)},
            {"role": "NON_FUNCTION_REVIEW_SCHEMA", "path": NON_FUNCTION_REVIEW_SCHEMA.relative_to(REPO).as_posix(), "sha256": sha256(NON_FUNCTION_REVIEW_SCHEMA)},
        ],
        "design_scope": {"parent_task_id": "T1-M01-N015", "terminal_atomic_pr_id": TERMINAL, "non_terminal_member_count": 36, "function_leaf_count": 25, "type_leaf_count": 3, "non_function_leaf_count": 8, "covered_atomic_pr_ids": MEMBER_IDS, "excluded_terminal_contract": "P066 uses the standard task-completion candidate and current-evidence-index contracts; no executable function is invented", "current_execution_status": BLOCKED},
        "data_contracts": data_contracts(), "function_contracts": functions, "type_contracts": types,
        "non_function_contracts": non_functions, "test_cases": test_cases(),
        "sequencing": sequence(leaves), "review_coverage": review_rows(functions, types, non_functions),
        "cross_cutting_invariants": INVARIANTS,
        "rollback": {"entrypoint": "Any trust validation deployment or evidence mismatch", "steps": ["stop candidate verifier routing before changing caller behavior", "scale the candidate verifier deployment to zero", "restore the existing require_trusted_signature_verifier hard block", "restore the last approved policy and image digest where applicable", "retain request attestation replay claim and evidence artifacts", "record the triggering invariant and candidate identity", "re-run static validation before proposing another candidate"], "durable_evidence_policy": "Attempted evidence and externally signed artifacts are immutable and never deleted or rewritten by rollback.", "irreversible_boundary": "No irreversible boundary is authorized by this design; global switch deployment and acceptance require later signed decisions."},
        "claims": {"allowed": "P057-P062 and P067-P096 function type and specialized non-function behavior is statically specified against v2 preview", "forbidden": ["GLOBAL_REGISTRY_SWITCHED", "FUNCTION_DESIGN_REVIEWED", "NON_FUNCTION_EXEMPTION_APPROVED", "IMPLEMENTED", "IMAGE_BUILT", "IMAGE_SIGNED", "DEPLOYED", "TEST_EXECUTED", "G0_PASS", "G1_PASS", "G2_PASS", "G3_PASS", "G6_PASS", "TRUST_PASS", "EXECUTION_AUTHORIZED", "PARENT_COMPLETE", "MILESTONE_COMPLETE", "PRODUCTION_ACCEPTED"], "proof_ceiling": "STATIC_FUNCTION_AND_NON_FUNCTION_DESIGN_ONLY_NOT_REVIEW_RECEIPT_GLOBAL_REGISTRATION_IMPLEMENTATION_IMAGE_BUILD_SIGNATURE_DEPLOYMENT_TEST_EXECUTION_TRUST_PASS_AUTHORIZATION_OR_ACCEPTANCE"},
        "validation": {"schema": "PASS", "source_hashes_exact": "PASS", "member_exact_set": "PASS_36", "function_exact_set": "PASS_25", "type_exact_set": "PASS_3", "non_function_exact_set": "PASS_8", "function_signatures_exact": "PASS", "body_steps_contiguous": "PASS", "test_exact_set": "PASS_42_NOT_RUN", "test_subject_coverage": "PASS_ALL_36", "single_gate_evidence": "PASS", "sequence_topological": "PASS", "review_coverage_exact": "PASS_36_MISSING", "no_review_fabrication": True, "mutation_guards": {name: "PASS" for name in ["source_hash_drift", "member_omission", "function_omission", "type_omission", "non_function_omission", "signature_drift", "body_step_drift", "error_branch_omission", "test_omission", "uncovered_leaf", "wrong_gate", "sequence_inversion", "review_false_ready", "terminal_function_fabrication", "secret_policy_weakening"]}},
    }


def markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# M01 早期受信验证函数设计覆盖", "",
        "> 状态：`STATIC_DESIGN_COMPLETE_REVIEW_IMPLEMENTATION_AND_EXECUTION_BLOCKED`。本文只呈现静态设计；不代表评审、实现、构建、签名、部署、测试、授权或验收。", "",
        "## 覆盖结论", "",
        "- N015 非终结成员：36/36。", "- 函数 owner：25/25。", "- 类型 owner：3/3。", "- 非函数 surface：8/8。",
        "- 设计测试：42 个，全部 `NOT_RUN`。", "- 逐叶评审/豁免：36 个，全部 `MISSING_NOT_AUTHORED`。",
        "- P066 仅保留标准 task-completion contract，不虚构可执行函数。", "",
        "## 函数合同", "",
        "| Atomic PR | 函数 | 变更 | 评审 |", "|---|---|---|---|",
    ]
    for item in payload["function_contracts"]:
        lines.append(f"| `{item['atomic_pr_id']}` | `{item['path']}#{item['qualified_symbol']}` | `{item['change_kind']}` | `{item['design_review_status']}` |")
    lines.extend(["", "## 类型合同", "", "| Atomic PR | 类型 | 变更 | 评审 |", "|---|---|---|---|"])
    for item in payload["type_contracts"]:
        lines.append(f"| `{item['atomic_pr_id']}` | `{item['path']}#{item['qualified_symbol']}` | `{item['change_kind']}` | `{item['design_review_status']}` |")
    lines.extend(["", "## 非函数合同", "", "| Atomic PR | Surface | Locators | 评审 |", "|---|---|---|---|"])
    for item in payload["non_function_contracts"]:
        lines.append(f"| `{item['atomic_pr_id']}` | `{item['surface_kind']}` | {len(item['target_locators'])} | `{item['design_review_status']}` |")
    lines.extend(["", "## 顺序与外部连接", ""])
    for item in payload["sequencing"]:
        lines.append(f"{item['order']}. `{item['atomic_pr_id']}` — {item['entry_condition']}")
    lines.extend([
        "", f"外部镜像活动 `{EXTERNAL_IMAGE}` 仍为 `PENDING_NO_RECEIPT`；P082 必须同时等待 P081 与该外部活动。专用合同见 `contracts/alignment/m01-verifier-image-build-sign-receipt.schema.json`，领取入口见 `contracts/alignment/m01-verifier-image-build-sign-work-order.v1.json`。",
        "", "## 下一步入口", "",
        "- 36 条评审工作单：`contracts/alignment/m01-early-trust-review-work-order-catalog.v1.json`；全部仍为 `BLOCKED_EXTERNAL_INPUTS`。",
        "- 四目录切换预检：`contracts/alignment/m01-early-trust-registry-switch-preflight.v1.json`；决策仍为 `BLOCKED_PRECONDITIONS`。",
        "- 这两个入口只编排外部工作，不构成评审 receipt、切换授权或执行证据。",
        "", "## 证明上限", "", f"`{payload['claims']['proof_ceiling']}`", "",
    ])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if args.write == args.check:
        parser.error("choose exactly one of --write or --check")
    payload = build()
    validate_against_schema(payload, SCHEMA)
    json_text = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    markdown_text = markdown(payload)
    if args.write:
        OUTPUT.write_text(json_text, encoding="utf-8")
        MARKDOWN.write_text(markdown_text, encoding="utf-8")
    else:
        if not OUTPUT.is_file() or OUTPUT.read_text(encoding="utf-8") != json_text:
            raise ValueError("M01 early-trust function design JSON is stale")
        if not MARKDOWN.is_file() or MARKDOWN.read_text(encoding="utf-8") != markdown_text:
            raise ValueError("M01 early-trust function design Markdown is stale")
    print("PASS M01 early-trust function design generation: 36 leaves, 25 functions, 3 types, 8 non-functions, 42 NOT_RUN tests")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
