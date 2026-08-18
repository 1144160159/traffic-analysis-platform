import base64
import copy
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from evaluate_m04_known_attack_accuracy import (  # noqa: E402
    AuthorityVerification,
    EvaluationBlocked,
    VerificationSummary,
    canonical_bytes,
    canonical_method_body,
    compute_body,
    evaluate,
    load_json,
    select_midterm_method,
    sha256_bytes,
    sha256_path,
    verify_result,
)


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


class M04KnownAttackAccuracyTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m04-eval-", dir=ROOT)
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.profile_id = "T1_MIDTERM_SIGNED_METRIC_PROFILE"
        self.environment_id = "k8s-test"
        self.evaluated_at = "2026-08-14T00:20:00Z"
        self.candidate_hash = "1" * 64
        self.model_hash = "2" * 64
        self.feature_hash = "3" * 64
        self.threshold_hash = "4" * 64
        self.policy_fingerprint = "a" * 64
        self.custody_path = self.path / "custody.json"
        self.custody_path.write_text('{"status":"FROZEN"}\n', encoding="utf-8")
        samples = [
            self.sample("attack-1", "known_attack", "known_attack", 0),
            self.sample("attack-2", "known_attack", "known_attack", 1),
            self.sample("attack-3", "known_attack", "known_attack", 2),
            self.sample("normal-1", "normal", "normal", 3),
            self.sample("normal-2", "normal", "normal", 4),
        ]
        label_projection = [
            {key: sample[key] for key in ("sample_id", "stratum", "label", "valid", "invalid_reason")}
            for sample in samples
        ]
        label_projection.sort(key=lambda item: item["sample_id"])
        self.dataset = {
            "schema_version": "1.0.0",
            "artifact_kind": "M04_FROZEN_KNOWN_ATTACK_DATASET",
            "dataset_id": "m04-known-v1",
            "status": "FROZEN",
            "split": "EVALUATION",
            "taxonomy_scope": "KNOWN_ATTACK_AND_NORMAL_ONLY",
            "profile_id": self.profile_id,
            "environment_id": self.environment_id,
            "analysis_unit": "sample-window",
            "dedup_window": "one-entity-window",
            "label_arbitration": "custodian-final-label",
            "candidate_hash": self.candidate_hash,
            "model_hash": self.model_hash,
            "feature_hash": self.feature_hash,
            "threshold_hash": self.threshold_hash,
            "taxonomy_sha256": "5" * 64,
            "labels_sha256": sha256_bytes(canonical_bytes(label_projection)),
            "frozen_at": "2026-08-14T00:00:00Z",
            "labels_released_at": "2026-08-14T00:15:00Z",
            "license": "internal-evaluation-license",
            "custodian": "qa-custodian",
            "custody_receipt": {
                "path": self.relative(self.custody_path),
                "sha256": sha256_path(self.custody_path),
            },
            "samples": samples,
        }
        self.dataset_path = self.path / "dataset.json"
        self.write(self.dataset_path, self.dataset)
        self.method_path = self.path / "methods.json"
        self.method = self.build_method()
        self.method_body = canonical_method_body(self.method)
        self.method_body_hash = sha256_bytes(self.method_body)
        self.method_body_path = self.path / "method-body.json"
        self.method_body_path.write_bytes(self.method_body)
        self.signature_paths = []
        for index in range(4):
            path = self.path / f"method-{index}.sig"
            path.write_bytes(f"signature-{index}".encode())
            self.signature_paths.append(path)
        self.intake_path = self.path / "intake.json"
        intake = self.build_intake()
        self.write(self.intake_path, intake)
        self.method["signed_intake"] = {
            "receipt_path": self.relative(self.intake_path),
            "receipt_sha256": sha256_path(self.intake_path),
            "signed_at": "2026-08-14T00:05:00Z",
            "method_sha256": self.method_body_hash,
            "signature_verification": "PASS",
        }
        self.registry = {
            "schema_version": "2.0.0",
            "registry_status": "ACTIVE",
            "methods": [
                self.method,
                copy.deepcopy(load_json(ROOT / "contracts/quality/topic1-metric-method.v1.json")["methods"][1]),
            ],
        }
        self.write(self.method_path, self.registry)
        self.method_roles = {
            "PROJECT_OWNER": "project-owner",
            "DOMAIN_OWNER": "algorithm-owner",
            "TEST_OWNER": "qa-owner",
            "ACCEPTANCE_AUTHORITY": "acceptance-owner",
        }
        self.method_request_paths = []
        for role, identity in self.method_roles.items():
            request_path = self.path / f"method-request-{role.lower()}.json"
            self.write_request(
                request_path,
                request_id=f"T1-SIGREQ-m04-method-{role.lower()}",
                content=self.method_body,
                subject_type="SIGNED_CONTRACT",
                subject_id="T1-MIDTERM-KNOWN-ALERT-METHOD",
                purpose="SIGNED_CONTRACT_INTAKE",
                roles={role: identity},
                scopes=["PROJECT", "TEST", "ACCEPTANCE", "CONTRACT"],
                evaluation_time="2026-08-14T00:05:00Z",
            )
            self.method_request_paths.append(request_path)
        self.predictions_path = self.path / "predictions.json"
        self.predictions = self.build_predictions({
            "attack-1": "known_attack",
            "attack-2": "known_attack",
            "attack-3": "normal",
            "normal-1": "known_attack",
            "normal-2": "normal",
        })
        self.write(self.predictions_path, self.predictions)
        self.result_roles = {
            "PROJECT_OWNER": "project-owner",
            "TEST_OWNER": "qa-owner",
            "ACCEPTANCE_AUTHORITY": "acceptance-owner",
        }
        self.result_request_paths = []
        self.refresh_result_request()

    def relative(self, path: Path) -> str:
        return path.relative_to(ROOT).as_posix()

    @staticmethod
    def write(path: Path, value: dict) -> None:
        path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    @staticmethod
    def sample(sample_id: str, label: str, stratum: str, minute: int) -> dict:
        return {
            "sample_id": sample_id,
            "entity_id": f"entity-{sample_id}",
            "window_start": f"2026-08-13T23:{minute:02d}:00Z",
            "window_end": f"2026-08-13T23:{minute:02d}:30Z",
            "stratum": stratum,
            "label": label,
            "valid": True,
            "invalid_reason": None,
            "source_sha256": digest(sample_id.encode()),
        }

    def build_method(self) -> dict:
        method = copy.deepcopy(load_json(ROOT / "contracts/quality/topic1-metric-method.v1.json")["methods"][0])
        method["method_status"] = "SIGNED"
        method["authority"] = {
            "project_owner": "project-owner",
            "algorithm_owner": "algorithm-owner",
            "qa_owner": "qa-owner",
            "acceptance_owner": "acceptance-owner",
            "external_lab": None,
            "signature_set": ["project-owner", "algorithm-owner", "qa-owner", "acceptance-owner"],
        }
        method["population"].update({
            "analysis_unit": "sample-window",
            "dedup_window": "one-entity-window",
            "abstain_policy": "count_as_incorrect",
            "invalid_sample_policy": "exclude_manifest_invalid_with_reason",
            "label_arbitration": "custodian-final-label",
            "strata": ["known_attack", "normal"],
            "minimum_sample_rules": ["total>=5", "known_attack>=3", "normal>=2"],
        })
        method["metrics"][0].update({
            "formula_status": "SIGNED",
            "signed_formula": "precision=TP/(TP+FP)",
        })
        method["threshold_lock"] = {
            "status": "LOCKED",
            "candidate_hash": self.candidate_hash,
            "model_hash": self.model_hash,
            "feature_hash": self.feature_hash,
            "threshold_hash": self.threshold_hash,
            "dataset_manifest_hash": sha256_path(self.dataset_path),
        }
        method["signed_intake"] = None
        return method

    def build_intake(self) -> dict:
        authorities = [
            {"role": "PROJECT_OWNER", "identity": "project-owner"},
            {"role": "DOMAIN_OWNER", "identity": "algorithm-owner"},
            {"role": "TEST_OWNER", "identity": "qa-owner"},
            {"role": "ACCEPTANCE_AUTHORITY", "identity": "acceptance-owner"},
        ]
        return {
            "schema_version": "1.0.0",
            "intake_id": "m04-method-intake",
            "intake_type": "METRIC_METHOD_SIGNATURE",
            "subject_path": self.relative(self.method_path),
            "subject_body_sha256": self.method_body_hash,
            "authorities": authorities,
            "signed_payload_artifact": self.relative(self.method_body_path),
            "signed_payload_sha256": self.method_body_hash,
            "signature_artifacts": [
                {
                    "path": self.relative(path),
                    "sha256": sha256_path(path),
                    "algorithm": "ED25519",
                    "key_or_certificate_id": f"key-{index}",
                }
                for index, path in enumerate(self.signature_paths)
            ],
            "signed_at": "2026-08-14T00:05:00Z",
            "verification": {
                "status": "PASS",
                "verifier": "protected-verifier",
                "verifier_version": "1.0.0",
                "verified_at": "2026-08-14T00:05:01Z",
                "revocation_checked": True,
            },
        }

    def build_predictions(self, decisions: dict[str, str]) -> dict:
        return {
            "schema_version": "1.0.0",
            "artifact_kind": "M04_MIDTERM_RAW_PREDICTIONS",
            "run_id": "m04-run-001",
            "profile_id": self.profile_id,
            "environment_id": self.environment_id,
            "candidate_hash": self.candidate_hash,
            "model_hash": self.model_hash,
            "feature_hash": self.feature_hash,
            "threshold_hash": self.threshold_hash,
            "dataset_manifest_hash": sha256_path(self.dataset_path),
            "method_body_hash": self.method_body_hash,
            "generated_at": "2026-08-14T00:10:00Z",
            "labels_visible_to_predictor": False,
            "rows": [
                {
                    "sample_id": sample_id,
                    "predicted_class": decisions[sample_id],
                    "score": 0.9 if decisions[sample_id] == "known_attack" else 0.1,
                    "detector_version": "rule-job:v1",
                    "evidence_ids": [f"evidence:{sample_id}"],
                }
                for sample_id in sorted(decisions)
            ],
        }

    def write_request(
        self, path: Path, *, request_id: str, content: bytes, subject_type: str,
        subject_id: str, purpose: str, roles: dict[str, str], scopes: list[str],
        evaluation_time: str,
    ) -> None:
        certificate = b"test-certificate"
        signature = b"test-signature"
        request = {
            "schema_version": "1.0.0",
            "canonicalization_version": "M01-SIGNATURE-REQUEST-C14N-V1",
            "request_id": request_id,
            "signed_payload": {
                "domain": "traffic-analysis-platform/topic1/signature-verification/v1",
                "subject_type": subject_type,
                "subject_id": subject_id,
                "subject_payload": {
                    "artifact_id": subject_id,
                    "media_type": "application/json",
                    "content_base64": base64.b64encode(content).decode(),
                    "content_sha256": sha256_bytes(content),
                    "size_bytes": len(content),
                },
                "candidate_commit": self.candidate_hash,
                "profile_id": self.profile_id,
                "environment_id": self.environment_id,
                "purpose": purpose,
                "signature_algorithm": "ED25519",
                "signer_certificate_sha256": sha256_bytes(certificate),
                "certificate_chain_sha256": "b" * 64,
                "claimed_authorities": [
                    {"authority_id": identity, "role": role} for role, identity in roles.items()
                ],
                "required_authority_roles": list(roles),
                "required_scopes": scopes,
                "issued_at": "2026-08-14T00:04:00Z",
                "expires_at": "2026-08-14T01:00:00Z",
                "evaluation_time": evaluation_time,
                "policy_id": "T1-SIGNATURE-POLICY-TEST",
                "policy_version": "1.0.0",
                "policy_fingerprint_sha256": self.policy_fingerprint,
                "nonce": hashlib.sha256(request_id.encode()).hexdigest()[:24],
                "cnas_context": None,
            },
            "verification_material": {
                "detached_signature_base64": base64.b64encode(signature).decode(),
                "signature_sha256": sha256_bytes(signature),
                "certificate_chain": [{
                    "certificate_id": "test-certificate",
                    "certificate_sha256": sha256_bytes(certificate),
                    "certificate_der_base64": base64.b64encode(certificate).decode(),
                }],
            },
        }
        self.write(path, request)

    def fake_verifier(self, request, request_hash: str) -> VerificationSummary:
        return VerificationSummary(
            request_id=request.request_id,
            request_sha256=request_hash,
            attestation_id=f"attestation:{request.request_id}",
            attestation_issued_at=request.signed_payload.evaluation_time,
            verifier_id="protected-test-verifier",
        )

    def refresh_result_request(self) -> None:
        method_signatures = [
            AuthorityVerification(
                role=role,
                summary=self.fake_verifier(NoneRequest(path), sha256_path(path)),
            )
            for role, path in zip(self.method_roles, self.method_request_paths)
        ]
        method = select_midterm_method(self.registry)
        samples = {item["sample_id"]: item for item in self.dataset["samples"]}
        rows = {item["sample_id"]: item for item in self.predictions["rows"]}
        body = compute_body(
            method,
            sha256_path(self.method_path),
            self.method_body_hash,
            self.dataset,
            sha256_path(self.dataset_path),
            self.predictions,
            sha256_path(self.predictions_path),
            samples,
            rows,
            self.evaluated_at,
            method_signatures,
        )
        self.result_request_paths = []
        for role, identity in self.result_roles.items():
            request_path = self.path / f"result-request-{role.lower()}.json"
            self.write_request(
                request_path,
                request_id=f"T1-SIGREQ-m04-result-{role.lower()}",
                content=canonical_bytes(body),
                subject_type="WORK_ORDER_EVIDENCE",
                subject_id=body["result_id"],
                purpose="WORK_ORDER_EVIDENCE_RUN",
                roles={role: identity},
                scopes=["PROJECT", "TEST", "ACCEPTANCE", "WORK_ORDER"],
                evaluation_time=self.evaluated_at,
            )
            self.result_request_paths.append(request_path)

    def run_evaluate(self) -> dict:
        return evaluate(
            method_path=self.method_path,
            dataset_path=self.dataset_path,
            predictions_path=self.predictions_path,
            method_request_paths=self.method_request_paths,
            result_request_paths=self.result_request_paths,
            evaluated_at=self.evaluated_at,
            verifier=self.fake_verifier,
        )

    def test_signed_precision_counts_and_strata_pass(self) -> None:
        result = self.run_evaluate()
        self.assertEqual("PASS_KNOWN_ATTACK_MIDTERM", result["body"]["status"])
        self.assertEqual("66.666667", result["body"]["metric"]["percent"])
        self.assertEqual((2, 1, 1, 1), tuple(result["body"]["counts"][key] for key in ("tp", "fp", "tn", "fn")))
        self.assertEqual(["known_attack", "normal"], [item["stratum"] for item in result["body"]["strata"]])
        self.assertEqual("VERIFIED", result["signature"]["status"])
        self.assertEqual(3, len(result["signature"]["signatures"]))
        self.assertEqual(4, len(result["body"]["method_signature"]["signatures"]))

    def test_repository_pending_method_blocks_before_other_inputs(self) -> None:
        pending = ROOT / "contracts/quality/topic1-metric-method.v1.json"
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_METHOD_NOT_SIGNED"):
            evaluate(
                method_path=pending,
                dataset_path=Path("missing"),
                predictions_path=Path("missing"),
                method_request_paths=[Path("missing")],
                result_request_paths=[Path("missing")],
                evaluated_at=self.evaluated_at,
                verifier=self.fake_verifier,
            )

    def test_prediction_exact_set_rejects_missing_and_duplicates(self) -> None:
        self.predictions["rows"].pop()
        self.write(self.predictions_path, self.predictions)
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_PREDICTION_EXACT_SET"):
            self.run_evaluate()
        self.predictions = self.build_predictions({
            item["sample_id"]: "normal" for item in self.dataset["samples"]
        })
        self.predictions["rows"].append(copy.deepcopy(self.predictions["rows"][0]))
        self.write(self.predictions_path, self.predictions)
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_DUPLICATE_PREDICTION"):
            self.run_evaluate()

    def test_threshold_binding_and_label_hash_fail_closed(self) -> None:
        self.predictions["threshold_hash"] = "9" * 64
        self.write(self.predictions_path, self.predictions)
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_PREDICTION_THRESHOLD_BINDING"):
            self.run_evaluate()
        self.predictions["threshold_hash"] = self.threshold_hash
        self.write(self.predictions_path, self.predictions)
        self.dataset["labels_sha256"] = "9" * 64
        self.write(self.dataset_path, self.dataset)
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_DATASET_LOCK_DRIFT"):
            self.run_evaluate()

    def test_below_fifty_is_signed_failure_and_preserved(self) -> None:
        self.predictions = self.build_predictions({
            "attack-1": "known_attack",
            "attack-2": "normal",
            "attack-3": "normal",
            "normal-1": "known_attack",
            "normal-2": "known_attack",
        })
        self.write(self.predictions_path, self.predictions)
        self.refresh_result_request()
        result = self.run_evaluate()
        self.assertEqual("FAIL_THRESHOLD", result["body"]["status"])
        self.assertEqual("33.333333", result["body"]["metric"]["percent"])
        self.assertEqual("VERIFIED", result["signature"]["status"])
        self.assertGreaterEqual(len(result["body"]["failure_samples"]), 4)

    def test_independent_recompute_rejects_tampering(self) -> None:
        result = self.run_evaluate()
        verify_result(copy.deepcopy(result), result)
        tampered = copy.deepcopy(result)
        tampered["body"]["counts"]["tp"] += 1
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_RESULT_BODY_HASH"):
            verify_result(tampered, result)

    def test_method_request_wrong_authority_is_rejected_before_verifier(self) -> None:
        request_path = self.method_request_paths[0]
        request = load_json(request_path)
        request["signed_payload"]["claimed_authorities"][0]["authority_id"] = "intruder"
        self.write(request_path, request)
        with self.assertRaisesRegex(EvaluationBlocked, "BLOCK_SIGNATURE_REQUEST_BINDING"):
            self.run_evaluate()


class NoneRequest:
    """Minimal request-shaped adapter used only to derive deterministic fixture summaries."""

    def __init__(self, path: Path) -> None:
        value = load_json(path)
        self.request_id = value["request_id"]
        self.signed_payload = type("Signed", (), {
            "evaluation_time": value["signed_payload"]["evaluation_time"]
        })()


if __name__ == "__main__":
    unittest.main()
