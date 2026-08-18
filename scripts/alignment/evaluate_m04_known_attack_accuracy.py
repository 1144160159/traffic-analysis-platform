#!/usr/bin/env python3
"""Evaluate the M04 frozen known-attack set with the pre-signed method only."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import tempfile
from dataclasses import dataclass
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
from typing import Any, Callable

from build_topic1_task_registry import validate_against_schema
from trusted_signature_service import SignatureVerificationRequest
from verify_trusted_signature import verify_exact_payload


ROOT = Path(__file__).resolve().parents[2]
METHOD_SCHEMA = ROOT / "contracts/quality/topic1-metric-method.schema.json"
DATASET_SCHEMA = ROOT / "contracts/quality/m04-known-attack-dataset.schema.json"
PREDICTIONS_SCHEMA = ROOT / "contracts/quality/m04-midterm-predictions.schema.json"
RESULT_SCHEMA = ROOT / "contracts/quality/m04-midterm-metric-result.schema.json"
INTAKE_SCHEMA = ROOT / "contracts/alignment/signed-contract-intake.schema.json"
MIDTERM_METHOD_ID = "T1-MIDTERM-KNOWN-ALERT-METHOD"
MIDTERM_METRIC_ID = "T1-MIDTERM-ALERT-ACCURACY"
SUPPORTED_FORMULAS = {
    "precision=TP/(TP+FP)",
    "generic_accuracy=(TP+TN)/N",
}
SUPPORTED_INVALID_POLICIES = {
    "reject_any_invalid",
    "exclude_manifest_invalid_with_reason",
}
SUPPORTED_ABSTAIN_POLICIES = {"count_as_incorrect", "reject_any_abstain"}
METHOD_ROLE_BINDINGS = {
    "PROJECT_OWNER": "project_owner",
    "DOMAIN_OWNER": "algorithm_owner",
    "TEST_OWNER": "qa_owner",
    "ACCEPTANCE_AUTHORITY": "acceptance_owner",
}
RESULT_ROLE_BINDINGS = {
    "PROJECT_OWNER": "project_owner",
    "TEST_OWNER": "qa_owner",
    "ACCEPTANCE_AUTHORITY": "acceptance_owner",
}


class EvaluationBlocked(ValueError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


@dataclass(frozen=True, slots=True)
class VerificationSummary:
    request_id: str
    request_sha256: str
    attestation_id: str
    attestation_issued_at: str
    verifier_id: str

    def to_dict(self) -> dict[str, str]:
        return {
            "request_id": self.request_id,
            "request_sha256": self.request_sha256,
            "attestation_id": self.attestation_id,
            "attestation_issued_at": self.attestation_issued_at,
            "verifier_id": self.verifier_id,
        }


@dataclass(frozen=True, slots=True)
class AuthorityVerification:
    role: str
    summary: VerificationSummary

    def to_dict(self) -> dict[str, str]:
        return {"role": self.role, **self.summary.to_dict()}


def signature_set(signatures: list[AuthorityVerification]) -> dict[str, Any]:
    return {
        "status": "VERIFIED",
        "signatures": [item.to_dict() for item in sorted(signatures, key=lambda item: item.role)],
    }


Verifier = Callable[[SignatureVerificationRequest, str], VerificationSummary]


def _unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise EvaluationBlocked("BLOCK_DUPLICATE_JSON_KEY", key)
        result[key] = value
    return result


def load_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_unique_object)
    except EvaluationBlocked:
        raise
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise EvaluationBlocked("BLOCK_JSON_INPUT", f"{path}: {error}") from error
    if not isinstance(value, dict):
        raise EvaluationBlocked("BLOCK_JSON_ROOT", str(path))
    return value


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_path(path: Path) -> str:
    try:
        return sha256_bytes(path.read_bytes())
    except OSError as error:
        raise EvaluationBlocked("BLOCK_ARTIFACT_READ", f"{path}: {error}") from error


def canonical_bytes(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")


def canonical_method_body(method: dict[str, Any]) -> bytes:
    body = dict(method)
    body["signed_intake"] = None
    return (json.dumps(body, ensure_ascii=False, indent=2, sort_keys=False) + "\n").encode("utf-8")


def parse_utc(value: str, label: str) -> dt.datetime:
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except (TypeError, ValueError) as error:
        raise EvaluationBlocked("BLOCK_TIME_FORMAT", f"{label}={value!r}") from error
    if parsed.tzinfo is None or parsed.utcoffset() != dt.timedelta(0):
        raise EvaluationBlocked("BLOCK_TIME_ZONE", f"{label} must be UTC")
    return parsed


def repo_artifact(path_value: str, expected_sha256: str, label: str) -> Path:
    path = (ROOT / path_value).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()):
        raise EvaluationBlocked("BLOCK_PATH_ESCAPE", f"{label}: {path_value}")
    if not path.is_file() or sha256_path(path) != expected_sha256:
        raise EvaluationBlocked("BLOCK_ARTIFACT_DRIFT", f"{label}: {path_value}")
    return path


def schema_check(value: dict[str, Any], schema: Path, code: str) -> None:
    try:
        validate_against_schema(value, schema)
    except ValueError as error:
        raise EvaluationBlocked(code, str(error)) from error


def select_midterm_method(registry: dict[str, Any]) -> dict[str, Any]:
    schema_check(registry, METHOD_SCHEMA, "BLOCK_METHOD_SCHEMA")
    matches = [item for item in registry["methods"] if item["method_id"] == MIDTERM_METHOD_ID]
    if len(matches) != 1:
        raise EvaluationBlocked("BLOCK_METHOD_IDENTITY", repr([item["method_id"] for item in matches]))
    method = matches[0]
    if registry["registry_status"] != "ACTIVE" or method["method_status"] != "SIGNED":
        raise EvaluationBlocked(
            "BLOCK_METHOD_NOT_SIGNED",
            f"registry={registry['registry_status']} method={method['method_status']}",
        )
    metrics = method["metrics"]
    if len(metrics) != 1 or metrics[0]["metric_id"] != MIDTERM_METRIC_ID:
        raise EvaluationBlocked("BLOCK_METRIC_IDENTITY", repr(metrics))
    metric = metrics[0]
    if (
        metric["stage"] != "MIDTERM"
        or metric["operator"] != ">="
        or metric["target_percent"] != 50
        or metric["formula_status"] != "SIGNED"
        or metric["signed_formula"] not in SUPPORTED_FORMULAS
    ):
        raise EvaluationBlocked("BLOCK_SIGNED_FORMULA", repr(metric))
    population = method["population"]
    if set(population["classes"]) != {"known_attack", "normal", "abstain"}:
        raise EvaluationBlocked("BLOCK_POPULATION_CLASSES", repr(population["classes"]))
    if population["invalid_sample_policy"] not in SUPPORTED_INVALID_POLICIES:
        raise EvaluationBlocked("BLOCK_INVALID_SAMPLE_POLICY", str(population["invalid_sample_policy"]))
    if population["abstain_policy"] not in SUPPORTED_ABSTAIN_POLICIES:
        raise EvaluationBlocked("BLOCK_ABSTAIN_POLICY", str(population["abstain_policy"]))
    if any(not population[field] for field in ("analysis_unit", "dedup_window", "label_arbitration")):
        raise EvaluationBlocked("BLOCK_POPULATION_METHOD", repr(population))
    lock = method["threshold_lock"]
    if lock["status"] != "LOCKED" or any(not lock[field] for field in (
        "candidate_hash", "model_hash", "feature_hash", "threshold_hash", "dataset_manifest_hash"
    )):
        raise EvaluationBlocked("BLOCK_THRESHOLD_NOT_LOCKED", repr(lock))
    authority = method["authority"]
    expected_identities = {authority[field] for field in METHOD_ROLE_BINDINGS.values()}
    if None in expected_identities or len(expected_identities) != len(METHOD_ROLE_BINDINGS):
        raise EvaluationBlocked("BLOCK_METHOD_AUTHORITIES", repr(authority))
    if set(authority["signature_set"]) != expected_identities:
        raise EvaluationBlocked("BLOCK_METHOD_SIGNATURE_SET", repr(authority["signature_set"]))
    return method


def validate_signed_intake(
    method: dict[str, Any], method_path: Path, method_body: bytes
) -> tuple[dict[str, Any], dt.datetime]:
    intake_ref = method.get("signed_intake")
    if not isinstance(intake_ref, dict):
        raise EvaluationBlocked("BLOCK_SIGNED_INTAKE_MISSING", MIDTERM_METHOD_ID)
    body_hash = sha256_bytes(method_body)
    if intake_ref["method_sha256"] != body_hash or intake_ref["signature_verification"] != "PASS":
        raise EvaluationBlocked("BLOCK_METHOD_BODY_HASH", body_hash)
    receipt_path = repo_artifact(intake_ref["receipt_path"], intake_ref["receipt_sha256"], "signed intake")
    receipt = load_json(receipt_path)
    schema_check(receipt, INTAKE_SCHEMA, "BLOCK_SIGNED_INTAKE_SCHEMA")
    try:
        subject_path = method_path.resolve().relative_to(ROOT.resolve()).as_posix()
    except ValueError as error:
        raise EvaluationBlocked("BLOCK_METHOD_OUTSIDE_REPOSITORY", str(method_path)) from error
    if (
        receipt["intake_type"] != "METRIC_METHOD_SIGNATURE"
        or receipt["subject_path"] != subject_path
        or receipt["subject_body_sha256"] != body_hash
        or receipt["signed_payload_sha256"] != body_hash
        or receipt["signed_at"] != intake_ref["signed_at"]
        or receipt["verification"]["status"] != "PASS"
        or not receipt["verification"]["revocation_checked"]
    ):
        raise EvaluationBlocked("BLOCK_SIGNED_INTAKE_BINDING", str(receipt_path))
    payload_path = repo_artifact(receipt["signed_payload_artifact"], body_hash, "signed method body")
    if payload_path.read_bytes() != method_body:
        raise EvaluationBlocked("BLOCK_SIGNED_PAYLOAD_BYTES", str(payload_path))
    for item in receipt["signature_artifacts"]:
        repo_artifact(item["path"], item["sha256"], "method signature")
    authority = method["authority"]
    expected_authorities = {
        (role, authority[field]) for role, field in METHOD_ROLE_BINDINGS.items()
    }
    actual_authorities = {(item["role"], item["identity"]) for item in receipt["authorities"]}
    if actual_authorities != expected_authorities or len(receipt["signature_artifacts"]) < 4:
        raise EvaluationBlocked("BLOCK_SIGNED_INTAKE_AUTHORITIES", repr(actual_authorities))
    return receipt, parse_utc(receipt["signed_at"], "signed_at")


def load_verification_request(path: Path) -> tuple[SignatureVerificationRequest, str]:
    raw = load_json(path)
    try:
        request = SignatureVerificationRequest.from_dict(raw)
    except Exception as error:
        raise EvaluationBlocked("BLOCK_SIGNATURE_REQUEST_SCHEMA", f"{path}: {error}") from error
    return request, sha256_path(path)


def verify_request(
    request_path: Path,
    verifier: Verifier,
    *,
    expected_content: bytes,
    subject_type: str,
    subject_id: str,
    purpose: str,
    candidate_hash: str,
    profile_id: str,
    environment_id: str,
    role_bindings: dict[str, str],
    method: dict[str, Any],
    required_scopes: set[str],
) -> tuple[VerificationSummary, SignatureVerificationRequest]:
    request, request_hash = load_verification_request(request_path)
    signed = request.signed_payload
    payload = signed.subject_payload
    expected_authorities = {
        (role, method["authority"][field]) for role, field in role_bindings.items()
    }
    actual_authorities = {(item.role, item.authority_id) for item in signed.claimed_authorities}
    mismatches = {
        "subject_type": signed.subject_type != subject_type,
        "subject_id": signed.subject_id != subject_id,
        "purpose": signed.purpose != purpose,
        "payload_bytes": payload.content != expected_content,
        "payload_sha256": payload.content_sha256 != sha256_bytes(expected_content),
        "payload_size": payload.size_bytes != len(expected_content),
        "candidate": signed.candidate_commit != candidate_hash,
        "profile": signed.profile_id != profile_id,
        "environment": signed.environment_id != environment_id,
        "roles": set(signed.required_authority_roles) != set(role_bindings),
        "authorities": actual_authorities != expected_authorities,
        "scopes": set(signed.required_scopes) != required_scopes,
        "cnas": signed.cnas_context is not None,
    }
    if any(mismatches.values()):
        raise EvaluationBlocked(
            "BLOCK_SIGNATURE_REQUEST_BINDING",
            repr({key: value for key, value in mismatches.items() if value}),
        )
    summary = verifier(request, request_hash)
    if summary.request_id != request.request_id or summary.request_sha256 != request_hash:
        raise EvaluationBlocked("BLOCK_VERIFIER_SUMMARY_BINDING", request.request_id)
    return summary, request


def verify_request_set(
    request_paths: list[Path],
    verifier: Verifier,
    *,
    expected_content: bytes,
    subject_type: str,
    subject_id: str,
    purpose: str,
    candidate_hash: str,
    profile_id: str,
    environment_id: str,
    role_bindings: dict[str, str],
    method: dict[str, Any],
    required_scopes: set[str],
) -> tuple[list[AuthorityVerification], dict[str, SignatureVerificationRequest]]:
    if len(request_paths) != len(role_bindings) or len(set(request_paths)) != len(request_paths):
        raise EvaluationBlocked(
            "BLOCK_SIGNATURE_REQUEST_COUNT",
            f"expected={len(role_bindings)} actual={len(request_paths)}",
        )
    signatures: list[AuthorityVerification] = []
    requests: dict[str, SignatureVerificationRequest] = {}
    for path in request_paths:
        request, _ = load_verification_request(path)
        roles = tuple(request.signed_payload.required_authority_roles)
        if len(roles) != 1 or roles[0] not in role_bindings or roles[0] in requests:
            raise EvaluationBlocked("BLOCK_SIGNATURE_ROLE_EXACT_SET", f"{path}: {roles}")
        role = roles[0]
        summary, verified_request = verify_request(
            path,
            verifier,
            expected_content=expected_content,
            subject_type=subject_type,
            subject_id=subject_id,
            purpose=purpose,
            candidate_hash=candidate_hash,
            profile_id=profile_id,
            environment_id=environment_id,
            role_bindings={role: role_bindings[role]},
            method=method,
            required_scopes=required_scopes,
        )
        requests[role] = verified_request
        signatures.append(AuthorityVerification(role=role, summary=summary))
    if set(requests) != set(role_bindings):
        raise EvaluationBlocked("BLOCK_SIGNATURE_ROLE_EXACT_SET", repr(sorted(requests)))
    return signatures, requests


def validate_dataset(
    dataset: dict[str, Any], dataset_path: Path, method: dict[str, Any], signed_at: dt.datetime
) -> tuple[dict[str, dict[str, Any]], dt.datetime]:
    schema_check(dataset, DATASET_SCHEMA, "BLOCK_DATASET_SCHEMA")
    dataset_hash = sha256_path(dataset_path)
    lock = method["threshold_lock"]
    if dataset_hash != lock["dataset_manifest_hash"]:
        raise EvaluationBlocked("BLOCK_DATASET_LOCK_DRIFT", dataset_hash)
    for field in ("candidate_hash", "model_hash", "feature_hash", "threshold_hash"):
        if dataset[field] != lock[field]:
            raise EvaluationBlocked("BLOCK_DATASET_THRESHOLD_BINDING", field)
    population = method["population"]
    for field in ("analysis_unit", "dedup_window", "label_arbitration"):
        if dataset[field] != population[field]:
            raise EvaluationBlocked("BLOCK_DATASET_METHOD_BINDING", field)
    receipt = dataset["custody_receipt"]
    repo_artifact(receipt["path"], receipt["sha256"], "dataset custody receipt")
    frozen_at = parse_utc(dataset["frozen_at"], "dataset.frozen_at")
    labels_released_at = parse_utc(dataset["labels_released_at"], "dataset.labels_released_at")
    if not frozen_at <= signed_at <= labels_released_at:
        raise EvaluationBlocked("BLOCK_METHOD_DATASET_CHRONOLOGY", repr((frozen_at, signed_at, labels_released_at)))
    samples: dict[str, dict[str, Any]] = {}
    intervals: dict[str, list[tuple[dt.datetime, dt.datetime, str]]] = {}
    strata: set[str] = set()
    label_projection: list[dict[str, Any]] = []
    for sample in dataset["samples"]:
        sample_id = sample["sample_id"]
        if sample_id in samples:
            raise EvaluationBlocked("BLOCK_DUPLICATE_SAMPLE_ID", sample_id)
        start = parse_utc(sample["window_start"], f"{sample_id}.window_start")
        end = parse_utc(sample["window_end"], f"{sample_id}.window_end")
        if end <= start:
            raise EvaluationBlocked("BLOCK_SAMPLE_WINDOW", sample_id)
        for previous_start, previous_end, previous_id in intervals.setdefault(sample["entity_id"], []):
            if max(start, previous_start) < min(end, previous_end):
                raise EvaluationBlocked("BLOCK_ENTITY_WINDOW_LEAKAGE", f"{previous_id},{sample_id}")
        intervals[sample["entity_id"]].append((start, end, sample_id))
        if sample["label"] == "normal" and sample["stratum"] != "normal":
            raise EvaluationBlocked("BLOCK_NORMAL_STRATUM", sample_id)
        if sample["label"] == "known_attack" and not (
            sample["stratum"] == "known_attack" or sample["stratum"].startswith("known_attack/")
        ):
            raise EvaluationBlocked("BLOCK_ATTACK_STRATUM", sample_id)
        if sample["valid"] and sample["invalid_reason"] is not None:
            raise EvaluationBlocked("BLOCK_INVALID_REASON_ON_VALID_SAMPLE", sample_id)
        if not sample["valid"] and not str(sample["invalid_reason"] or "").strip():
            raise EvaluationBlocked("BLOCK_INVALID_SAMPLE_WITHOUT_REASON", sample_id)
        samples[sample_id] = sample
        strata.add(sample["stratum"])
        label_projection.append({
            key: sample[key]
            for key in ("sample_id", "stratum", "label", "valid", "invalid_reason")
        })
    if strata != set(population["strata"]):
        raise EvaluationBlocked("BLOCK_STRATA_EXACT_SET", repr(sorted(strata)))
    label_projection.sort(key=lambda item: item["sample_id"])
    if sha256_bytes(canonical_bytes(label_projection)) != dataset["labels_sha256"]:
        raise EvaluationBlocked("BLOCK_LABEL_HASH_DRIFT", dataset["labels_sha256"])
    return samples, labels_released_at


def validate_predictions(
    predictions: dict[str, Any], predictions_path: Path, dataset: dict[str, Any],
    samples: dict[str, dict[str, Any]], method: dict[str, Any], method_body_hash: str,
    dataset_hash: str, signed_at: dt.datetime, labels_released_at: dt.datetime,
) -> tuple[dict[str, dict[str, Any]], dt.datetime]:
    schema_check(predictions, PREDICTIONS_SCHEMA, "BLOCK_PREDICTIONS_SCHEMA")
    lock = method["threshold_lock"]
    for field in ("candidate_hash", "model_hash", "feature_hash", "threshold_hash"):
        if predictions[field] != lock[field] or predictions[field] != dataset[field]:
            raise EvaluationBlocked("BLOCK_PREDICTION_THRESHOLD_BINDING", field)
    bindings = {
        "dataset": predictions["dataset_manifest_hash"] == dataset_hash,
        "method": predictions["method_body_hash"] == method_body_hash,
        "profile": predictions["profile_id"] == dataset["profile_id"],
        "environment": predictions["environment_id"] == dataset["environment_id"],
    }
    if not all(bindings.values()):
        raise EvaluationBlocked("BLOCK_PREDICTION_INPUT_BINDING", repr(bindings))
    generated_at = parse_utc(predictions["generated_at"], "predictions.generated_at")
    if generated_at < signed_at or generated_at > labels_released_at:
        raise EvaluationBlocked("BLOCK_PREDICTION_CHRONOLOGY", predictions["generated_at"])
    rows: dict[str, dict[str, Any]] = {}
    for row in predictions["rows"]:
        if row["sample_id"] in rows:
            raise EvaluationBlocked("BLOCK_DUPLICATE_PREDICTION", row["sample_id"])
        rows[row["sample_id"]] = row
    missing = sorted(set(samples) - set(rows))
    extra = sorted(set(rows) - set(samples))
    if missing or extra:
        raise EvaluationBlocked("BLOCK_PREDICTION_EXACT_SET", f"missing={missing} extra={extra}")
    if method["population"]["abstain_policy"] == "reject_any_abstain":
        abstained = sorted(sample_id for sample_id, row in rows.items() if row["predicted_class"] == "abstain")
        if abstained:
            raise EvaluationBlocked("BLOCK_ABSTAIN_FORBIDDEN", repr(abstained))
    if method["population"]["invalid_sample_policy"] == "reject_any_invalid":
        invalid = sorted(sample_id for sample_id, sample in samples.items() if not sample["valid"])
        if invalid:
            raise EvaluationBlocked("BLOCK_INVALID_SAMPLE_FORBIDDEN", repr(invalid))
    return rows, generated_at


def blank_counts() -> dict[str, int]:
    return {
        "total": 0,
        "evaluated": 0,
        "excluded": 0,
        "tp": 0,
        "fp": 0,
        "tn": 0,
        "fn": 0,
        "abstain_known_attack": 0,
        "abstain_normal": 0,
    }


def increment_counts(counts: dict[str, int], sample: dict[str, Any], row: dict[str, Any]) -> str | None:
    counts["total"] += 1
    if not sample["valid"]:
        counts["excluded"] += 1
        return f"excluded:{sample['invalid_reason']}"
    counts["evaluated"] += 1
    truth = sample["label"]
    predicted = row["predicted_class"]
    if predicted == "abstain":
        counts[f"abstain_{truth}"] += 1
        return "abstain_counted_as_incorrect"
    confusion = {
        ("known_attack", "known_attack"): "tp",
        ("normal", "known_attack"): "fp",
        ("normal", "normal"): "tn",
        ("known_attack", "normal"): "fn",
    }[(truth, predicted)]
    counts[confusion] += 1
    return None if confusion in {"tp", "tn"} else f"classification_{confusion}"


def enforce_minimum_rules(rules: list[str], counts: dict[str, int], strata: dict[str, dict[str, int]]) -> None:
    values = {
        "total": counts["evaluated"],
        "known_attack": counts["tp"] + counts["fn"] + counts["abstain_known_attack"],
        "normal": counts["fp"] + counts["tn"] + counts["abstain_normal"],
    }
    for rule in rules:
        match = re.fullmatch(r"(total|known_attack|normal|stratum:[A-Za-z0-9._/-]+)>=(\d+)", rule)
        if not match:
            raise EvaluationBlocked("BLOCK_UNKNOWN_MINIMUM_RULE", rule)
        key, minimum_text = match.groups()
        actual = (
            strata.get(key.removeprefix("stratum:"), blank_counts())["evaluated"]
            if key.startswith("stratum:") else values[key]
        )
        if actual < int(minimum_text):
            raise EvaluationBlocked("BLOCK_MINIMUM_SAMPLE_RULE", f"{rule} actual={actual}")


def compute_body(
    method: dict[str, Any], method_registry_hash: str, method_body_hash: str,
    dataset: dict[str, Any], dataset_hash: str, predictions: dict[str, Any],
    predictions_hash: str, samples: dict[str, dict[str, Any]],
    rows: dict[str, dict[str, Any]], evaluated_at: str,
    method_signatures: list[AuthorityVerification],
) -> dict[str, Any]:
    total = blank_counts()
    by_stratum = {name: blank_counts() for name in sorted(method["population"]["strata"])}
    failure_samples: list[dict[str, str]] = []
    for sample_id in sorted(samples):
        sample = samples[sample_id]
        row = rows[sample_id]
        reason = increment_counts(total, sample, row)
        increment_counts(by_stratum[sample["stratum"]], sample, row)
        if reason is not None:
            failure_samples.append({
                "sample_id": sample_id,
                "stratum": sample["stratum"],
                "truth": sample["label"],
                "prediction": row["predicted_class"],
                "reason": reason,
            })
    enforce_minimum_rules(method["population"]["minimum_sample_rules"], total, by_stratum)
    formula = method["metrics"][0]["signed_formula"]
    if formula == "precision=TP/(TP+FP)":
        numerator, denominator = total["tp"], total["tp"] + total["fp"]
    elif formula == "generic_accuracy=(TP+TN)/N":
        numerator, denominator = total["tp"] + total["tn"], total["evaluated"]
    else:  # guarded by select_midterm_method; retained as a local exhaustive check
        raise EvaluationBlocked("BLOCK_SIGNED_FORMULA", str(formula))
    if denominator == 0:
        raise EvaluationBlocked("BLOCK_ZERO_DENOMINATOR", str(formula))
    percent_decimal = (Decimal(numerator) * Decimal(100) / Decimal(denominator)).quantize(
        Decimal("0.000001"), rounding=ROUND_HALF_UP
    )
    threshold_pass = percent_decimal >= Decimal("50")
    return {
        "result_id": f"M04-MIDTERM:{predictions['run_id']}:{predictions_hash[:12]}",
        "run_id": predictions["run_id"],
        "method_id": MIDTERM_METHOD_ID,
        "dataset_id": dataset["dataset_id"],
        "profile_id": dataset["profile_id"],
        "environment_id": dataset["environment_id"],
        "evaluated_at": evaluated_at,
        "status": "PASS_KNOWN_ATTACK_MIDTERM" if threshold_pass else "FAIL_THRESHOLD",
        "input_hashes": {
            "method_registry_sha256": method_registry_hash,
            "method_body_sha256": method_body_hash,
            "dataset_manifest_sha256": dataset_hash,
            "predictions_sha256": predictions_hash,
            "candidate_hash": dataset["candidate_hash"],
            "model_hash": dataset["model_hash"],
            "feature_hash": dataset["feature_hash"],
            "threshold_hash": dataset["threshold_hash"],
            "taxonomy_sha256": dataset["taxonomy_sha256"],
            "labels_sha256": dataset["labels_sha256"],
        },
        "method_signature": signature_set(method_signatures),
        "counts": total,
        "strata": [
            {"stratum": name, "counts": by_stratum[name]} for name in sorted(by_stratum)
        ],
        "minimum_sample_rules": list(method["population"]["minimum_sample_rules"]),
        "metric": {
            "metric_id": MIDTERM_METRIC_ID,
            "signed_formula": formula,
            "numerator": numerator,
            "denominator": denominator,
            "percent": f"{percent_decimal:.6f}",
            "operator": ">=",
            "target_percent": 50,
            "threshold_pass": threshold_pass,
        },
        "failure_samples": failure_samples,
        "allowed_claim": "only the signed-method M04 midterm metric for the frozen known-attack and normal evaluation set",
        "forbidden_claims": [
            "unknown-attack detection claim",
            "final 95 percent accuracy or less-than-5-percent false-alarm claim",
            "CNAS or project-completion claim",
        ],
    }


def prepare_evaluation(
    *,
    method_path: Path,
    dataset_path: Path,
    predictions_path: Path,
    method_request_paths: list[Path],
    evaluated_at: str,
    verifier: Verifier,
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any], dt.datetime]:
    method_registry = load_json(method_path)
    method = select_midterm_method(method_registry)
    method_body = canonical_method_body(method)
    method_body_hash = sha256_bytes(method_body)
    _, signed_at = validate_signed_intake(method, method_path, method_body)
    dataset = load_json(dataset_path)
    samples, labels_released_at = validate_dataset(dataset, dataset_path, method, signed_at)
    method_signatures, method_requests = verify_request_set(
        method_request_paths,
        verifier,
        expected_content=method_body,
        subject_type="SIGNED_CONTRACT",
        subject_id=MIDTERM_METHOD_ID,
        purpose="SIGNED_CONTRACT_INTAKE",
        candidate_hash=dataset["candidate_hash"],
        profile_id=dataset["profile_id"],
        environment_id=dataset["environment_id"],
        role_bindings=METHOD_ROLE_BINDINGS,
        method=method,
        required_scopes={"PROJECT", "TEST", "ACCEPTANCE", "CONTRACT"},
    )
    method_signature_times = {
        parse_utc(request.signed_payload.evaluation_time, f"{role} method signature evaluation_time")
        for role, request in method_requests.items()
    }
    if method_signature_times != {signed_at}:
        raise EvaluationBlocked("BLOCK_METHOD_SIGNATURE_TIME", repr(method_signature_times))
    dataset_hash = sha256_path(dataset_path)
    predictions = load_json(predictions_path)
    rows, generated_at = validate_predictions(
        predictions, predictions_path, dataset, samples, method, method_body_hash,
        dataset_hash, signed_at, labels_released_at,
    )
    evaluation_time = parse_utc(evaluated_at, "evaluated_at")
    if evaluation_time < generated_at:
        raise EvaluationBlocked("BLOCK_EVALUATION_CHRONOLOGY", evaluated_at)
    body = compute_body(
        method,
        sha256_path(method_path),
        method_body_hash,
        dataset,
        dataset_hash,
        predictions,
        sha256_path(predictions_path),
        samples,
        rows,
        evaluated_at,
        method_signatures,
    )
    return body, method, dataset, evaluation_time


def evaluate(
    *,
    method_path: Path,
    dataset_path: Path,
    predictions_path: Path,
    method_request_paths: list[Path],
    result_request_paths: list[Path],
    evaluated_at: str,
    verifier: Verifier,
) -> dict[str, Any]:
    body, method, dataset, evaluation_time = prepare_evaluation(
        method_path=method_path,
        dataset_path=dataset_path,
        predictions_path=predictions_path,
        method_request_paths=method_request_paths,
        evaluated_at=evaluated_at,
        verifier=verifier,
    )
    body_bytes = canonical_bytes(body)
    result_signatures, result_requests = verify_request_set(
        result_request_paths,
        verifier,
        expected_content=body_bytes,
        subject_type="WORK_ORDER_EVIDENCE",
        subject_id=body["result_id"],
        purpose="WORK_ORDER_EVIDENCE_RUN",
        candidate_hash=dataset["candidate_hash"],
        profile_id=dataset["profile_id"],
        environment_id=dataset["environment_id"],
        role_bindings=RESULT_ROLE_BINDINGS,
        method=method,
        required_scopes={"PROJECT", "TEST", "ACCEPTANCE", "WORK_ORDER"},
    )
    result_signature_times = {
        parse_utc(request.signed_payload.evaluation_time, f"{role} result signature evaluation_time")
        for role, request in result_requests.items()
    }
    if result_signature_times != {evaluation_time}:
        raise EvaluationBlocked("BLOCK_RESULT_SIGNATURE_TIME", repr(result_signature_times))
    result = {
        "schema_version": "1.0.0",
        "artifact_kind": "M04_SIGNED_MIDTERM_METRIC_RESULT",
        "result_body_sha256": sha256_bytes(body_bytes),
        "body": body,
        "signature": signature_set(result_signatures),
    }
    schema_check(result, RESULT_SCHEMA, "BLOCK_RESULT_SCHEMA")
    return result


def verify_result(result: dict[str, Any], recomputed: dict[str, Any]) -> None:
    schema_check(result, RESULT_SCHEMA, "BLOCK_RESULT_SCHEMA")
    if result["result_body_sha256"] != sha256_bytes(canonical_bytes(result["body"])):
        raise EvaluationBlocked("BLOCK_RESULT_BODY_HASH", result["result_body_sha256"])
    if canonical_bytes(result) != canonical_bytes(recomputed):
        raise EvaluationBlocked("BLOCK_INDEPENDENT_RECOMPUTE", "stored result differs from exact recomputation")


def endpoint_verifier(endpoint: str, policy_fingerprint: str) -> Verifier:
    def verify(request: SignatureVerificationRequest, request_hash: str) -> VerificationSummary:
        try:
            attestation = verify_exact_payload(
                request,
                endpoint=endpoint,
                policy_fingerprint=policy_fingerprint,
            )
        except Exception as error:
            raise EvaluationBlocked("BLOCK_TRUSTED_SIGNATURE_VERIFIER", str(error)) from error
        return VerificationSummary(
            request_id=request.request_id,
            request_sha256=request_hash,
            attestation_id=attestation.attestation_id,
            attestation_issued_at=attestation.issued_at,
            verifier_id=attestation.verifier.service_id,
        )
    return verify


def failure_document(error: Exception, args: argparse.Namespace) -> dict[str, Any]:
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "M04_EVALUATION_FAILURE",
        "status": "BLOCKED",
        "code": getattr(error, "code", "BLOCK_UNEXPECTED"),
        "detail": str(error),
        "inputs": {
            key: str(getattr(args, key))
            for key in ("method_registry", "dataset", "predictions")
            if getattr(args, key, None) is not None
        },
    }


def write_new_bytes(path: Path, value: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=".m04-evaluation-", dir=path.parent)
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as output:
            output.write(value)
            output.flush()
            os.fsync(output.fileno())
        try:
            os.link(temporary_path, path)
        except FileExistsError as error:
            raise EvaluationBlocked("BLOCK_OUTPUT_EXISTS", str(path)) from error
        directory_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        temporary_path.unlink(missing_ok=True)


def write_new(path: Path, value: dict[str, Any]) -> None:
    write_new_bytes(path, (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8"))


def add_input_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--method-registry", type=Path, required=True)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--predictions", type=Path, required=True)
    parser.add_argument(
        "--method-verification-request", type=Path, action="append", required=True,
        help="repeat exactly once for PROJECT_OWNER, DOMAIN_OWNER, TEST_OWNER and ACCEPTANCE_AUTHORITY",
    )
    parser.add_argument("--evaluated-at", required=True)
    parser.add_argument("--verifier-endpoint", required=True)
    parser.add_argument("--policy-fingerprint", required=True)


def add_signed_result_arguments(parser: argparse.ArgumentParser) -> None:
    add_input_arguments(parser)
    parser.add_argument(
        "--result-verification-request", type=Path, action="append", required=True,
        help="repeat exactly once for PROJECT_OWNER, TEST_OWNER and ACCEPTANCE_AUTHORITY",
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prepare_parser = commands.add_parser("prepare", help="write the canonical result body that must be signed")
    add_input_arguments(prepare_parser)
    prepare_parser.add_argument("--output", type=Path, required=True)
    evaluate_parser = commands.add_parser("evaluate")
    add_signed_result_arguments(evaluate_parser)
    evaluate_parser.add_argument("--output", type=Path, required=True)
    verify_parser = commands.add_parser("verify")
    add_signed_result_arguments(verify_parser)
    verify_parser.add_argument("--result", type=Path, required=True)
    args = parser.parse_args()
    verifier = endpoint_verifier(args.verifier_endpoint, args.policy_fingerprint)
    try:
        if args.command == "prepare":
            body, _, _, _ = prepare_evaluation(
                method_path=args.method_registry,
                dataset_path=args.dataset,
                predictions_path=args.predictions,
                method_request_paths=args.method_verification_request,
                evaluated_at=args.evaluated_at,
                verifier=verifier,
            )
            write_new_bytes(args.output, canonical_bytes(body))
            print(json.dumps({
                "status": "PENDING_RESULT_SIGNATURE",
                "result_body": str(args.output),
                "result_body_sha256": sha256_bytes(canonical_bytes(body)),
                "metric_status_after_signature": body["status"],
            }, ensure_ascii=False, indent=2))
            return 0 if body["status"] == "PASS_KNOWN_ATTACK_MIDTERM" else 1
        recomputed = evaluate(
            method_path=args.method_registry,
            dataset_path=args.dataset,
            predictions_path=args.predictions,
            method_request_paths=args.method_verification_request,
            result_request_paths=args.result_verification_request,
            evaluated_at=args.evaluated_at,
            verifier=verifier,
        )
        if args.command == "verify":
            verify_result(load_json(args.result), recomputed)
            print(json.dumps({"status": "VERIFIED", "result": str(args.result)}, ensure_ascii=False))
            return 0
        write_new(args.output, recomputed)
        print(json.dumps(recomputed, ensure_ascii=False, indent=2))
        return 0 if recomputed["body"]["status"] == "PASS_KNOWN_ATTACK_MIDTERM" else 1
    except Exception as error:
        failure = failure_document(error, args)
        if args.command in {"prepare", "evaluate"}:
            try:
                write_new(args.output, failure)
            except EvaluationBlocked as write_error:
                print(json.dumps(failure_document(write_error, args), ensure_ascii=False, indent=2))
                return 2
        print(json.dumps(failure, ensure_ascii=False, indent=2))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
