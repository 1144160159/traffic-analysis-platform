#!/usr/bin/env python3
"""Static and Kubernetes-evidence verifier for T1-M09-N019."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/rule-model-review.v1.json")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n019/k8s-rule-model-review-latest.json")
TEXT_PATHS = (
    "go/control-plane/internal/rules/model/deployment_runtime_gate.go",
    "go/control-plane/internal/rules/model/deployment_workbench.go",
    "go/control-plane/internal/rules/service/deployment_runtime_gate.go",
    "go/control-plane/internal/rules/service/deployment_service.go",
    "go/control-plane/internal/rules/config/config.go",
    "go/control-plane/cmd/rule-manager/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/rule-manager.yaml",
    "web/ui/src/pages/DeploymentManagementWorkspace.tsx",
    "web/ui/src/pages/deploymentManagementLogic.ts",
    "web/ui/src/services/api.ts",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def load_texts() -> dict[str, str]:
    return {relative: (ROOT / relative).read_text(encoding="utf-8") for relative in TEXT_PATHS}


def validate_snapshot(texts: dict[str, str], contract: dict[str, Any], evidence: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    rail = contract.get("runtime_rail", {})
    if contract.get("task_id") != "T1-M09-N019" or contract.get("status") != "PARTIAL":
        errors.append("N019 contract identity/status must remain PARTIAL")
    if rail.get("flag") != "DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED" or rail.get("default_enabled") is not False:
        errors.append("deployment runtime ACK gate contract is not default-off")

    gate = texts["go/control-plane/internal/rules/service/deployment_runtime_gate.go"]
    for token in (
        "discoverDeploymentRuntimeGate", "rule_update_applied_acks", "model_update_applied_acks",
        "deployment_event_projection", "COUNT(DISTINCT a.subtask_index)",
        "minSuccess == 0", "maxSuccess == receipt.ExpectedAcks-1",
        "sameDeploymentRuntimeReceipt", "runtime ACK event changed after approval",
    ):
        if token not in gate:
            errors.append(f"runtime ACK gate implementation missing {token}")

    service = texts["go/control-plane/internal/rules/service/deployment_service.go"]
    for token in (
        "configuration[deploymentRuntimeGateConfigurationKey] = runtimeGate",
        "deploymentRuntimeGatePrecheckResult(runtimeGate)",
        "runtime ACK gated deployment must pass through gray observation",
        "lockedApprovedConfiguration, true",
        "RuntimeGate: runtimeGate",
        "RuntimeGate: workbench.RuntimeGate",
    ):
        if token not in service:
            errors.append(f"deployment workflow gate/evidence integration missing {token}")

    model = texts["go/control-plane/internal/rules/model/deployment_runtime_gate.go"]
    for token in ("ExpectedAcks", "SuccessfulAcks", "FailedAcks", "KafkaPartition", "KafkaOffset", "ExpansionAllowed"):
        if token not in model:
            errors.append(f"deployment runtime receipt model missing {token}")

    config = texts["go/control-plane/internal/rules/config/config.go"]
    main = texts["go/control-plane/cmd/rule-manager/main.go"]
    if 'env:"DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED" envDefault:"false"' not in config:
        errors.append("runtime ACK gate config default is not false")
    for token in ("RuntimeAckGateEnabled", "RuleAppliedExpectedParallelism", "ModelAppliedExpectedParallelism"):
        if token not in main:
            errors.append(f"rule-manager deployment gate wiring missing {token}")
    for relative in (
        "deployments/kubernetes/applications/go-services.yaml",
        "go/control-plane/deployments/kubernetes/rule-manager.yaml",
    ):
        manifest = texts[relative]
        at = manifest.find("DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED")
        if at < 0 or "false" not in manifest[at:at + 100]:
            errors.append(f"runtime ACK gate is not explicitly default-off in {relative}")

    client = texts["web/ui/src/services/api.ts"]
    page = texts["web/ui/src/pages/DeploymentManagementWorkspace.tsx"]
    logic = texts["web/ui/src/pages/deploymentManagementLogic.ts"]
    for token in ("DeploymentRuntimeReceipt", "DeploymentRuntimeGate", "runtime_gate"):
        if token not in client:
            errors.append(f"typed deployment client missing {token}")
    for token in ("规则 / 模型运行时 ACK", "data-runtime-ack-status", "ACK 不完整，已停止灰度扩展", "runtimeExpansionBlocked"):
        if token not in page:
            errors.append(f"deployment runtime ACK UI missing {token}")
    if "status === 'gray'" not in logic or "!gate?.expansion_allowed" not in logic:
        errors.append("UI expansion blocking is not limited to enabled gray runtime gate")

    latest = contract.get("latest_evidence", {})
    if evidence.get("task_id") != "T1-M09-N019" or evidence.get("status") != "PASS" or evidence.get("run_id") != latest.get("run_id"):
        errors.append("N019 Kubernetes evidence identity/status mismatch")
    for field in (
        "partial_ack_stops_expansion", "exact_rule_model_receipts", "approval_binds_event_ids",
        "event_drift_requires_reapproval", "gray_projection_ack_required",
        "old_version_recoverable", "ui_runtime_gate_present", "run_scoped_resources_removed",
    ):
        if evidence.get(field) is not True:
            errors.append(f"N019 Kubernetes evidence missing {field}=true")
    for field in ("runtime_gate_default_enabled", "mock_enabled", "shared_postgres_touched", "production_applied"):
        if evidence.get(field) is not False:
            errors.append(f"N019 Kubernetes evidence must keep {field}=false")
    if len(evidence.get("kubernetes_jobs", [])) != 3:
        errors.append("N019 Kubernetes evidence does not contain three successful jobs")
    oracle = evidence.get("postgres_cleanup_oracle", {})
    for field in (
        "tenants", "rule_versions", "rule_outbox", "rule_acks", "model_versions",
        "model_outbox", "model_acks", "deployments", "deployment_outbox", "deployment_projection",
    ):
        if oracle.get(field) != 0:
            errors.append(f"N019 PostgreSQL cleanup oracle {field} is not zero")
    if not any("broker-generated rule/model ACK delivery" in item for item in evidence.get("does_not_prove", [])):
        errors.append("N019 evidence overclaims broker-generated rule/model ACK delivery")
    return errors


def main() -> int:
    contract, evidence, texts = load_json(CONTRACT), load_json(EVIDENCE), load_texts()
    errors = validate_snapshot(texts, contract, evidence)
    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = ROOT / relative
        actual = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else "missing"
        if actual != expected:
            errors.append(f"Kubernetes evidence source hash drifted: {relative}")
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N019 rule/model review gate and Kubernetes evidence are current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
