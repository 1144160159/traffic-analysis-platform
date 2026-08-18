#!/usr/bin/env python3
"""Build and validate the Topic One parent-task/atomic-PR registry.

The detailed design remains the review view. This generator turns its 212 task
table rows into a deterministic machine-readable registry. A structurally valid
registry does not make a task READY: unresolved owner/symbol/candidate fields
remain explicit readiness blockers.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import Any, Protocol

from trusted_signature_service import (
    SignatureVerificationAttestation,
    SignatureVerificationRequest,
)


REPO_ROOT = Path(__file__).resolve().parents[2]
DOC_REL = Path("doc/07_alignment/课题一PR里程碑代码级详细设计.md")
DOC_PATH = REPO_ROOT / DOC_REL
REGISTRY_PATH = REPO_ROOT / "contracts/alignment/task-registry.v1.json"
MILESTONE_PATH = REPO_ROOT / "contracts/alignment/milestone-registry.v1.json"
CANONICAL_PATH = REPO_ROOT / "contracts/alignment/canonical-registry.json"
REMEDIATION_LEDGER_PATH = REPO_ROOT / "contracts/alignment/remediation-ledger.json"
FEATURE_REGISTRY_PATH = REPO_ROOT / "contracts/alignment/feature-contract-registry.v1.json"
TASK_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/task-registry.schema.json"
MILESTONE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/milestone-registry.schema.json"
EXECUTION_OVERLAY_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/task-execution-overlay.schema.json"
EXECUTION_OVERLAY_PATH = REPO_ROOT / "contracts/alignment/task-execution-overlay.template.v1.json"
REQUIREMENT_PATH = REPO_ROOT / "contracts/requirements/topic1-system-requirements.v1.json"
REQUIREMENT_SCHEMA_PATH = REPO_ROOT / "contracts/requirements/topic1-system-requirements.schema.json"
EVIDENCE_CONTRACT_PATH = REPO_ROOT / "contracts/requirements/topic1-evidence-contracts.v1.json"
EVIDENCE_CONTRACT_SCHEMA_PATH = REPO_ROOT / "contracts/requirements/topic1-evidence-contracts.schema.json"
METRIC_METHOD_PATH = REPO_ROOT / "contracts/quality/topic1-metric-method.v1.json"
METRIC_METHOD_SCHEMA_PATH = REPO_ROOT / "contracts/quality/topic1-metric-method.schema.json"
EXTERNAL_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/external-activity-receipt.schema.json"
SIGNED_CONTRACT_INTAKE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/signed-contract-intake.schema.json"
EXECUTION_ACCEPTANCE_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/execution-acceptance-receipt.schema.json"
IMPLEMENTATION_CANDIDATE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/implementation-candidate.schema.json"
CANDIDATE_ARTIFACT_PROVENANCE_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/candidate-artifact-provenance-receipt.schema.json"
EVIDENCE_RUN_BINDING_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/evidence-run-binding.schema.json"
EVIDENCE_RUN_MANIFEST_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/evidence-run-manifest.schema.json"
EVIDENCE_CASE_REPORT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/evidence-case-report.schema.json"
EVIDENCE_CASE_FIXTURE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/evidence-case-fixture.schema.json"
CURRENT_EVIDENCE_INDEX_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/current-evidence-index.schema.json"
PROMOTION_INTENT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/promotion-intent.schema.json"
PROMOTION_RESULT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/promotion-result.schema.json"
ATOMIC_EXECUTION_PACKAGE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/atomic-pr-execution-package.schema.json"
ATOMIC_PLAN_MANIFEST_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/atomic-pr-plan-manifest.schema.json"
INTEGRATED_BOM_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/integrated-system-bom.schema.json"
INTEGRATED_BOM_TRANSITION_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/integrated-system-bom-transition.schema.json"
BOM_TRANSITION_AUTHORITY_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/bom-transition-authority-receipt.schema.json"
REQUIREMENT_SATISFACTION_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/requirement-satisfaction.schema.json"
REQUIREMENT_SATISFACTION_AUTHORITY_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/requirement-satisfaction-authority-receipt.schema.json"
MILESTONE_COMPLETION_CANDIDATE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/milestone-completion-candidate.schema.json"
MILESTONE_PROMOTION_CLOSURE_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/milestone-promotion-closure.schema.json"
DEVELOPER_CLAIM_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/developer-claim-package.schema.json"
DEVELOPER_CLAIM_CATALOG_PATH = REPO_ROOT / "contracts/alignment/developer-claim-package-catalog.v1.json"
CODE_TARGET_BINDING_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/canonical-pr-target-binding.schema.json"
TASK_COMPLETION_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/task-completion-candidate.schema.json"
TASK_CURRENT_INDEX_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/task-current-evidence-index.schema.json"
ATOMIC_IMPLEMENTATION_RESULT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/atomic-pr-implementation-result.schema.json"
LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/locator-resolution-receipt.schema.json"
PYTHON_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/python-locator-resolution-receipt.schema.json"
RUST_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/rust-locator-resolution-receipt.schema.json"
PROTO_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/proto-descriptor-locator-resolution-receipt.schema.json"
STRUCTURED_CONFIG_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json"
SHELL_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH = REPO_ROOT / "contracts/alignment/shell-ast-locator-resolution-receipt.schema.json"

TASK_ROW_RE = re.compile(
    r"^\| (T1-M(?P<milestone>\d{2})-N(?P<number>\d{3})) \| "
    r"(?P<pr>[^|]+) \| (?P<paths>[^|]*) \| (?P<action>[^|]*) \| "
    r"(?P<verify>[^|]*) \|$"
)
ID_RE = re.compile(r"^(?:F|T)-[A-Z]+-[0-9]{3}$")
CANONICAL_REF_RE = re.compile(r"(?:F|T)-[A-Z]+-[0-9]{3}")
BACKTICK_RE = re.compile(r"`([^`]+)`")
EXTERNAL_ACTIVITY_RE = re.compile(r"external_activity:(CUSTODY|EXECUTE|ATTEST|APPROVAL)")


@dataclass(frozen=True, slots=True)
class CandidateRepository:
    root: Path
    git_dir: Path
    identity_sha256: str


@dataclass(frozen=True, slots=True)
class CandidateTrustContext:
    candidate_commit: str
    profile_id: str
    environment_id: str
    purpose: str
    policy_fingerprint: str

    def __post_init__(self) -> None:
        if not re.fullmatch(r"(?:[0-9a-f]{40}|[0-9a-f]{64})", self.candidate_commit):
            raise ValueError("candidate trust context requires a full immutable commit ID")
        for label, value in (
            ("profile_id", self.profile_id),
            ("environment_id", self.environment_id),
            ("purpose", self.purpose),
        ):
            if not value or value == "*":
                raise ValueError(f"candidate trust context {label} must be exact")
        if not re.fullmatch(r"[0-9a-f]{64}", self.policy_fingerprint):
            raise ValueError("candidate trust context policy fingerprint must be SHA-256")


class TrustedSignatureVerifier(Protocol):
    def verify_exact_payload(
        self, request: SignatureVerificationRequest
    ) -> SignatureVerificationAttestation: ...

EXTERNAL_ACTIVITY_INPUTS = {
    "CUSTODY": [
        "implementation-candidate-identity-manifest",
        "blind-dataset-manifest",
        "label-custody-policy",
        "signed-final-metric-method",
        "sealed-transfer-protocol-manifest",
    ],
    "EXECUTE": [
        "implementation-candidate-identity-manifest",
        "signed-final-metric-method",
        "sealed-blind-dataset",
        "custody-activity-receipt",
        "threshold-lock",
        "runtime-environment-manifest",
    ],
    "ATTEST": [
        "execute-activity-receipt",
        "raw-predictions",
        "blind-labels",
        "signed-final-metric-method",
        "laboratory-accreditation-scope",
    ],
    "APPROVAL": [
        "implementation-candidate-identity-manifest",
        "contract-closure-manifest",
        "current-evidence-index",
        "cnas-attestation-receipt",
        "promotion-premerge-equivalence-result",
    ],
}

EXTERNAL_ACTIVITY_OUTPUTS = {
    "CUSTODY": ["sealed-blind-dataset", "custody-access-log", "custody-handoff-manifest"],
    "EXECUTE": ["raw-predictions", "runtime-logs", "runtime-environment-manifest"],
    "ATTEST": ["cnas-metric-result", "cnas-signed-report"],
    "APPROVAL": ["signed-go-no-go-decision"],
}

EXTERNAL_ACTIVITY_RECEIPT_FIELDS = {
    "CUSTODY": ["activity_payload.payload_type", "activity_payload.sealed_dataset_sha256", "activity_payload.access_log_sha256", "activity_payload.handoff_manifest_sha256", "activity_payload.handoff_started_at", "activity_payload.handoff_finished_at", "activity_payload.handed_to", "activity_payload.label_separation_verified"],
    "EXECUTE": ["activity_payload.payload_type", "activity_payload.raw_predictions_sha256", "activity_payload.runtime_logs_sha256", "activity_payload.environment_manifest_sha256", "activity_payload.threshold_lock_sha256", "activity_payload.labels_absent_verified"],
    "ATTEST": ["activity_payload.payload_type", "activity_payload.accreditation_scope", "activity_payload.method_sha256", "activity_payload.result_sha256", "activity_payload.report_sha256", "activity_payload.signed_at"],
    "APPROVAL": ["activity_payload.payload_type", "activity_payload.approver_roles", "activity_payload.promotion_profile", "activity_payload.decision", "activity_payload.manifest_sha256", "activity_payload.claims_sha256", "activity_payload.decision_receipt_sha256"],
}
SLICE_ROW_RE = re.compile(
    r"^\| (?P<slice>R\d{2}) \| (?P<ids>[^|]+) \| "
    r"(?P<surface>[^|]+) \| (?P<minimum>[^|]+) \|$"
)
PR_TYPE_RE = re.compile(
    r"TST-PRE|TST-POST|CTR|EXP|PRJ|WRT|UI|OPS|REF|IDX|PROM|TST"
)
VALID_PR_TYPES = {
    "CTR",
    "EXP",
    "PRJ",
    "WRT",
    "UI",
    "OPS",
    "REF",
    "TST-PRE",
    "TST-POST",
    "IDX",
    "PROM",
}

EXPECTED_COUNTS = {
    "M00": 8,
    "M01": 14,
    "M02": 16,
    "M03": 18,
    "M04": 12,
    "M05": 8,
    "M06": 18,
    "M07": 20,
    "M08": 18,
    "M09": 24,
    "M10": 16,
    "M11": 12,
    "M12": 8,
    "M13": 20,
}

MILESTONE_TITLES = {
    "M00": "任务书真源、边界和声明治理",
    "M01": "候选身份、基线、合同与早期护栏",
    "M02": "实时/离线采集与PCAP耐久写链",
    "M03": "深度解析、会话、特征与离线文件还原",
    "M04": "已知攻击检测与中期预警准确率",
    "M05": "2026-10-30中期系统证据点",
    "M06": "四源接入、实体身份和事件时间",
    "M07": "质量、三级融合、基准、图和攻击链",
    "M08": "已知/未知攻击AI、GNN与模型治理",
    "M09": "分析产品、证据、取证、反馈和typed UI",
    "M10": "最小现场部署、安全物化与限定恢复",
    "M11": "冻结盲测与CNAS外部硬门",
    "M12": "课题一系统合同最小发布",
    "M13": "整改收敛与自设强化工程门",
}

MILESTONE_DEPS = {
    "M00": [],
    "M01": ["M00"],
    "M02": ["M01"],
    "M03": ["M02"],
    "M04": ["M03"],
    "M05": ["M04"],
    "M06": ["M01", "M03"],
    "M07": ["M04", "M06"],
    "M08": ["M03", "M06", "M07"],
    "M09": ["M04", "M07", "M08"],
    "M10": ["M01", "M09"],
    "M11": ["M08", "M10"],
    "M12": ["M09", "M10", "M11"],
    "M13": ["M12"],
}

MILESTONE_REQUIREMENTS = {
    "M00": ["REQ-T1-SYS-001", "REQ-T1-EVI-001"],
    "M01": ["REQ-T1-SYS-001", "REQ-T1-EVI-001"],
    "M02": ["REQ-T1-DATA-CAPTURE-001", "REQ-T1-EVI-001"],
    "M03": ["REQ-T1-DATA-PARSE-001", "REQ-T1-FILE-RESTORE-001", "REQ-T1-ENCRYPTED-001"],
    "M04": ["REQ-T1-DET-MIDTERM-001"],
    "M05": ["REQ-T1-RELEASE-MIDTERM-001"],
    "M06": ["REQ-T1-DATA-FOUR-SOURCE-001"],
    "M07": ["REQ-T1-FUSION-001", "REQ-T1-BASELINE-001", "REQ-T1-ATTACKCHAIN-001"],
    "M08": ["REQ-T1-AI-001", "REQ-T1-GNN-001"],
    "M09": [
        "REQ-T1-SYS-001",
        "REQ-T1-EVI-001",
        "REQ-T1-ENCRYPTED-001",
        "REQ-T1-FILE-RESTORE-001",
    ],
    "M10": ["REQ-T1-SYS-DEPLOY-001"],
    "M11": ["REQ-T1-QUAL-001"],
    "M12": ["REQ-T1-SYS-001", "REQ-T1-QUAL-001"],
    "M13": ["REQ-T1-INTERNAL-STRENGTHENING-001"],
}

# A deliberately conservative total order. It over-serializes safe preparation
# work, while critical interleaved consumer-first leaves are rewired below.
EXECUTION_ORDER = {
    "M00": [1, 2, 3, 4, 5, 7, 6, 8],
    "M01": [2, 1, 3, 5, 6, 7, 8, 9, 10, 11, 4, 12, 13, 14],
    "M02": [1, 2, 3, 9, 10, 4, 5, 6, 7, 8, 11, 12, 14, 13, 15, 16],
    "M03": [1, 2, 11, 15, 8, 5, 6, 7, 9, 10, 3, 4, 16, 12, 17, 13, 14, 18],
    "M04": [1, 2, 3, 8, 6, 4, 5, 7, 9, 10, 11, 12],
    "M05": list(range(1, 9)),
    "M06": [1, 2, 8, 16, 3, 5, 6, 7, 9, 11, 12, 4, 10, 13, 17, 15, 14, 18],
    "M07": [1, 3, 2, 4, 5, 7, 6, 8, 10, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20],
    "M08": list(range(1, 19)),
    "M09": [1, 2, 3, 4, 5, 6, 7, 8, 10, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24],
    "M10": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 11, 13, 14, 15, 16],
    "M11": list(range(1, 13)),
    "M12": [2, 4, 1, 5, 3, 6, 7, 8],
    "M13": list(range(1, 21)),
}

PROTO_TOPIC_AUTHORITY_INPUTS = [
    "proto/buf.yaml",
    "proto/buf.gen.yaml",
    "proto/traffic/v1/alert.proto",
    "proto/traffic/v1/asset.proto",
    "proto/traffic/v1/audit.proto",
    "proto/traffic/v1/campaign.proto",
    "proto/traffic/v1/common.proto",
    "proto/traffic/v1/detection.proto",
    "proto/traffic/v1/feature.proto",
    "proto/traffic/v1/flow.proto",
    "proto/traffic/v1/graph.proto",
    "proto/traffic/v1/ingest.proto",
    "proto/traffic/v1/pcap.proto",
    "proto/traffic/v1/session.proto",
    "contracts/events/kafka-topic-catalog.v1.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "contracts/events/kafka-json-events-v1.schema.json",
]

DDL_SOURCE_PATTERN = re.compile(
    r"\b(?:CREATE|ALTER|DROP|TRUNCATE)\s+(?:TABLE|INDEX|VIEW|DATABASE|SCHEMA)\b",
    re.IGNORECASE,
)
SCHEMA_AUTHORITY_INPUTS = sorted({
    path.relative_to(REPO_ROOT).as_posix()
    for root in (
        REPO_ROOT / "common/sql/pg",
        REPO_ROOT / "common/sql/ch",
        REPO_ROOT / "deployments/postgres/migrations",
        REPO_ROOT / "deployments/clickhouse/migrations",
        REPO_ROOT / "go/control-plane/deployments/docker/init",
        REPO_ROOT / "java/flink-jobs",
    )
    for path in root.rglob("*.sql")
    if path.is_file()
} | {
    "go/control-plane/scripts/postgres-schema.sql",
} | {
    path.relative_to(REPO_ROOT).as_posix()
    for root in (
        REPO_ROOT / "go/control-plane/cmd",
        REPO_ROOT / "go/control-plane/internal",
        REPO_ROOT / "java/flink-jobs",
        REPO_ROOT / "rust/probe-agent",
    )
    for path in root.rglob("*")
    if path.is_file()
    and path.suffix.lower() in {".go", ".java", ".rs"}
    and not path.name.endswith(("_test.go", "Test.java"))
    and DDL_SOURCE_PATTERN.search(path.read_text(encoding="utf-8", errors="ignore"))
})

PR_TYPE_OVERRIDES = {
    "T1-M00-N006": ["REF", "TST-PRE"],
    "T1-M01-N003": ["REF", "TST-PRE"],
    "T1-M01-N004": ["REF", "REF", "REF", "TST-PRE"],
    "T1-M01-N005": ["CTR", "PRJ", "REF", "TST-PRE"],
    "T1-M01-N008": ["CTR", "PRJ", "REF", "TST-PRE"],
    "T1-M01-N009": ["CTR", "PRJ", "REF", "TST-PRE"],
    "T1-M01-N010": [
        "CTR", "REF", "REF",
        "REF", "REF", "REF", "REF", "REF", "REF", "REF",
        "REF", "REF", "REF", "TST-PRE", "OPS", "TST-POST",
    ],
    "T1-M04-N008": ["EXP", "PRJ", "EXP", "PRJ", "EXP", "PRJ"],
    "T1-M06-N012": ["PRJ", "PRJ", "PRJ", "PRJ", "PRJ", "PRJ"],
    "T1-M06-N016": ["CTR", "PRJ", "WRT", "WRT", "CTR", "WRT"],
    "T1-M06-N017": ["OPS", "TST-POST", "OPS", "OPS", "TST-POST", "OPS", "TST-POST"],
    "T1-M07-N003": ["PRJ", "OPS"],
    "T1-M07-N018": [
        "CTR", "EXP", "PRJ", "REF", "TST-PRE", "WRT", "PRJ", "WRT",
        "CTR", "EXP", "PRJ", "OPS", "OPS", "OPS", "OPS", "TST-POST",
    ],
    "T1-M08-N010": ["WRT", "WRT", "OPS"],
    "T1-M09-N010": ["PRJ", "OPS"],
    "T1-M09-N023": ["TST-POST"] * 7,
    "T1-M10-N012": ["OPS", "TST-PRE", "TST-POST"],
    "T1-M10-N013": ["OPS"] + ["TST-POST"] * 9,
    "T1-M13-N013": [item for _ in range(5) for item in ("REF", "OPS", "TST-POST")],
    "T1-M13-N014": [item for _ in range(5) for item in ("REF", "OPS", "TST-POST")],
    "T1-M13-N015": [item for _ in range(6) for item in ("REF", "OPS", "TST-POST")],
    "T1-M13-N016": ["TST-POST"] * 3,
    "T1-M13-N017": [item for _ in range(6) for item in ("OPS", "TST-POST")],
    "T1-M13-N018": [item for _ in range(8) for item in ("OPS", "TST-POST")],
    "T1-M13-N019": ["TST-POST"] * 10,
}

PHASE_OVERRIDES = {
    "T1-M00-N006": [
        "traceability-validator-fixture",
        "traceability-validator-run",
    ],
    "T1-M01-N003": [
        "candidate-provenance-fixture",
        "candidate-provenance-run",
    ],
    "T1-M01-N004": [
        "candidate-freeze-fixture",
        "candidate-git-identity",
        "candidate-main-fail-closed",
        "candidate-freeze-run",
    ],
    "T1-M01-N005": [
        "contract-inventory-schema",
        "contract-inventory-builder",
        "contract-inventory-mutation-oracle",
        "contract-inventory-verification",
    ],
    "T1-M01-N008": [
        "proto-topic-matrix-schema",
        "proto-topic-matrix-builder",
        "proto-topic-matrix-mutation-oracle",
        "proto-topic-matrix-verification",
    ],
    "T1-M01-N009": [
        "schema-authority-contract",
        "schema-authority-builder",
        "schema-authority-mutation-oracle",
        "schema-authority-verification",
    ],
    "T1-M01-N010": [
        "trusted-signature-contracts",
        "trusted-signature-fixture",
        "trusted-verifier-adapter",
        "trusted-verifier-wrapper",
        "caller-candidate-artifact-refs",
        "caller-implementation-candidate",
        "caller-requirement-satisfaction",
        "caller-bom-transition",
        "caller-external-signature",
        "caller-signed-contract-intake",
        "caller-execution-overlay",
        "caller-fail-closed-selftest",
        "caller-work-order-evidence-run",
        "trusted-signature-negative-run",
        "trusted-verifier-protected-backend",
        "trusted-signature-positive-run",
    ],
    "T1-M04-N008": [
        "clickhouse-expand", "clickhouse-projection",
        "opensearch-expand", "opensearch-projection",
        "postgres-receipt-expand", "postgres-receipt-projection",
    ],
    "T1-M06-N004": [
        "source-precedence-contract",
        "source-precedence-validator",
        "source-precedence-verification",
        "authority-transaction",
        "http-commit-unknown-mapping",
        "http-commit-unknown-test-fixture",
        "http-commit-unknown-verification",
        "grpc-commit-unknown-mapping",
        "grpc-commit-unknown-test-fixture",
        "grpc-commit-unknown-verification",
        "asset-event-topic-rail",
        "asset-event-topic-rail-test-fixture",
        "asset-event-topic-rail-verification",
        "authority-transaction-test-fixture",
        "authority-transaction-fault-matrix",
        "asset-event-real-broker-fixture",
        "asset-event-real-broker-ack",
        "asset-authority-live-reconcile-runner",
        "asset-authority-live-reconcile",
    ],
    "T1-M06-N012": [
        "clickhouse-flow-projection", "clickhouse-asset-projection",
        "clickhouse-device-log-projection", "clickhouse-user-event-projection",
        "opensearch-asset-projection", "opensearch-device-log-projection",
    ],
    "T1-M06-N016": [
        "asset-binding-contract", "asset-binding-consumer-ready",
        "asset-binding-ingest-bridge-default-off",
        "asset-binding-probe-grpc-sender-default-off", "device-log-contract",
        "device-log-producer-default-off",
    ],
    "T1-M06-N017": [
        "asset-events-canary", "asset-events-acceptance",
        "asset-binding-ingest-bridge-canary", "asset-binding-probe-sender-canary",
        "asset-bindings-acceptance",
        "device-logs-canary", "device-logs-acceptance",
    ],
    "T1-M07-N003": ["repair-replay-consumer-ready", "repair-replay-ops-enable"],
    "T1-M07-N018": [
        "campaign-contract",
        "campaign-expand",
        "campaigns-v1-protobuf-consumer-ready",
        "cep-tenant-key-isolation-refactor",
        "cep-tenant-key-isolation-negative-test",
        "cep-publisher-default-off",
        "campaign-authority-json-v2-consumer-ready",
        "campaign-authority-json-v2-command-writer",
        "campaign-rail-correlation-contract",
        "campaign-rail-correlation-expand",
        "campaign-rail-correlation-projection",
        "cep-publisher-canary",
        "campaign-authority-consumer-canary",
        "campaign-authority-dispatcher-canary",
        "campaign-correlation-canary",
        "campaign-reconcile",
    ],
    "T1-M08-N010": ["metadata-registration", "activation-outbox-publish", "shadow-activation"],
    "T1-M09-N010": ["forensics-worker-idle-ready", "forensics-worker-enable"],
    "T1-M09-N023": [
        "alert-query-journey", "alert-mutation-journey", "permission-negative-journey",
        "failure-recovery-journey", "forensics-download-journey",
        "attack-chain-campaign-journey", "search-export-final-fact-journey",
    ],
    "T1-M10-N012": ["telemetry-oncall-ready", "signal-injection", "observation-verification"],
    "T1-M10-N013": [
        "recovery-plan", "postgres-authority-restore", "kafka-offset-retention-restore",
        "flink-savepoint-restore", "clickhouse-replay-reconcile",
        "opensearch-rebuild", "nebula-rebuild", "redis-rebuild",
        "minio-manifest-restore", "cross-domain-business-oracle",
    ],
    "T1-M13-N013": [f"{domain}-{phase}" for domain in ("capture", "parser", "aggregator", "archiver", "sender") for phase in ("refactor", "canary", "profile")],
    "T1-M13-N014": [f"{domain}-{phase}" for domain in ("partition-key", "batch-compression", "checkpoint-state", "operator-uid", "sink-backpressure") for phase in ("refactor", "canary", "profile")],
    "T1-M13-N015": [f"{domain}-{phase}" for domain in ("postgres", "clickhouse", "opensearch", "nebula", "redis", "minio") for phase in ("refactor", "canary", "profile")],
    "T1-M13-N016": ["10x100g-run", "512mpps-run", "p95-60s-run"],
    "T1-M13-N017": [f"{domain}-{phase}" for domain in ("network-policy", "egress", "management-plane", "image-signing", "secret-rotation", "security-negative") for phase in ("apply", "verify")],
    "T1-M13-N018": [f"{domain}-{phase}" for domain in ("postgres", "clickhouse", "opensearch", "nebula", "redis", "kafka", "flink", "minio") for phase in ("fault-window", "restore-oracle")],
    "T1-M13-N019": [
        "login-auth", "dashboard", "assets", "alerts", "campaigns",
        "attack-chain", "topics", "forensics", "model-governance", "deployment-rollback",
    ],
}

# Candidate paths are phase-specific review inputs, not write authorization.
# Planned paths remain visible so a leaf cannot look executable while lacking
# the file it is supposed to create. allowed_paths stays empty until an
# execution overlay selects exact paths/symbols and an owner approves them.
PHASE_PATH_OVERRIDES = {
    "T1-M00-N006": {
        "traceability-validator-fixture": [
            "scripts/alignment/test_topic1_traceability.py",
            "scripts/alignment/build_topic1_task_registry.py",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/work-packages.json",
        ],
        "traceability-validator-run": [
            "scripts/alignment/test_topic1_traceability.py",
            "scripts/alignment/build_topic1_task_registry.py",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/work-packages.json",
        ],
    },
    "T1-M00-N003": {
        "step-1": [
            "contracts/requirements/topic1-system-requirements.schema.json",
            "contracts/requirements/topic1-system-requirements.v1.json",
        ],
    },
    "T1-M06-N004": {
        "source-precedence-contract": [
            "contracts/alignment/asset-upsert-source-precedence.schema.json",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
        ],
        "source-precedence-validator": [
            "scripts/alignment/validate_asset_upsert_source_precedence.py",
            "contracts/alignment/asset-upsert-source-precedence.schema.json",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
        ],
        "source-precedence-verification": [
            "scripts/alignment/validate_asset_upsert_source_precedence.py",
            "contracts/alignment/asset-upsert-source-precedence.schema.json",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
        ],
        "source-precedence-approval": [
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
            "contracts/alignment/asset-upsert-source-precedence-approval.schema.json",
            "contracts/alignment/asset-upsert-source-precedence-signature-receipt.schema.json",
            "contracts/alignment/asset-upsert-source-precedence-test-result.schema.json",
            "scripts/alignment/validate_asset_upsert_source_precedence_approval.py",
            "scripts/alignment/validate_asset_upsert_source_precedence.py",
            "scripts/alignment/build_topic1_task_registry.py",
            "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/candidate-manifest.json",
        ],
        "authority-transaction": [
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
            "go/control-plane/internal/asset/repository/postgres.go",
            "go/control-plane/internal/asset/service/asset_service.go",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
        ],
        "http-commit-unknown-mapping": [
            "go/control-plane/internal/asset/api/http_handler.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
        ],
        "http-commit-unknown-test-fixture": [
            "go/control-plane/internal/asset/api/auth_test.go",
            "go/control-plane/internal/asset/api/http_handler.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
        ],
        "http-commit-unknown-verification": [
            "go/control-plane/internal/asset/api/auth_test.go",
            "go/control-plane/internal/asset/api/http_handler.go",
            "scripts/alignment/run_exact_go_tests.py",
        ],
        "grpc-commit-unknown-mapping": [
            "go/control-plane/internal/asset/api/grpc_handler.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
        ],
        "grpc-commit-unknown-test-fixture": [
            "go/control-plane/internal/asset/api/grpc_handler_test.go",
            "go/control-plane/internal/asset/api/grpc_handler.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
        ],
        "grpc-commit-unknown-verification": [
            "go/control-plane/internal/asset/api/grpc_handler_test.go",
            "go/control-plane/internal/asset/api/grpc_handler.go",
            "scripts/alignment/run_exact_go_tests.py",
        ],
        "asset-event-topic-rail": [
            "go/control-plane/internal/asset/config/loader.go",
            "go/control-plane/internal/asset/config/loader_test.go",
            "go/control-plane/internal/asset/config/config.go",
        ],
        "asset-event-topic-rail-test-fixture": [
            "go/control-plane/internal/asset/config/loader_test.go",
            "go/control-plane/internal/asset/config/loader.go",
            "go/control-plane/internal/asset/config/config.go",
        ],
        "asset-event-topic-rail-verification": [
            "go/control-plane/internal/asset/config/loader_test.go",
            "go/control-plane/internal/asset/config/loader.go",
            "scripts/alignment/run_exact_go_tests.py",
        ],
        "authority-transaction-fault-matrix": [
            "go/control-plane/internal/asset/repository/atomic_upsert_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
            "scripts/alignment/verify_asset_atomic_ephemeral.py",
            "scripts/alignment/run_exact_go_tests.py",
        ],
        "authority-transaction-test-fixture": [
            "go/control-plane/internal/asset/repository/atomic_upsert_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
            "contracts/alignment/asset-upsert-source-precedence.v1.json",
        ],
        "asset-event-real-broker-fixture": [
            "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
            "go/control-plane/internal/asset/repository/outbox_dispatcher.go",
        ],
        "asset-event-real-broker-ack": [
            "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go",
            "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
            "go/control-plane/internal/asset/repository/outbox_dispatcher.go",
            "scripts/alignment/verify_asset_projection_kafka_ephemeral.py",
            "scripts/alignment/run_exact_go_tests.py",
        ],
        "asset-authority-live-reconcile-runner": [
            "scripts/alignment/reconcile_asset_authority_live.py",
            "tests/alignment/test_reconcile_asset_authority_live.py",
            "contracts/alignment/asset-authority-live-run-manifest.schema.json",
            "contracts/alignment/asset-authority-live-receipt.schema.json",
            "contracts/alignment/asset-authority-live-reconcile-result.schema.json",
            "contracts/alignment/evidence-run-manifest.schema.json",
            "contracts/alignment/evidence-run-binding.schema.json",
            "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/candidate-manifest.json",
        ],
        "asset-authority-live-reconcile": [
            "scripts/alignment/reconcile_asset_authority_live.py",
            "contracts/alignment/asset-authority-live-run-manifest.schema.json",
            "contracts/alignment/asset-authority-live-receipt.schema.json",
            "contracts/alignment/asset-authority-live-reconcile-result.schema.json",
            "contracts/alignment/evidence-run-manifest.schema.json",
            "contracts/alignment/evidence-run-binding.schema.json",
            "go/control-plane/internal/asset/repository/atomic_upsert.go",
            "go/control-plane/internal/asset/repository/outbox_dispatcher.go",
            "go/control-plane/internal/asset/consumer/asset_projection_event.go",
            "go/control-plane/internal/asset/consumer/asset_projection_worker.go",
            "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/candidate-manifest.json",
        ],
    },
    "T1-M01-N003": {
        "candidate-provenance-fixture": [
            "scripts/alignment/test_implementation_candidate.py",
            "contracts/alignment/implementation-candidate.schema.json",
            "contracts/alignment/candidate-artifact-provenance-receipt.schema.json",
            "go/control-plane/deployments/docker/Dockerfile.alert-service.prebuilt.overlay",
            "scripts/alignment/build_topic1_task_registry.py",
        ],
        "candidate-provenance-run": [
            "scripts/alignment/test_implementation_candidate.py",
            "contracts/alignment/implementation-candidate.schema.json",
            "contracts/alignment/candidate-artifact-provenance-receipt.schema.json",
            "go/control-plane/deployments/docker/Dockerfile.alert-service.prebuilt.overlay",
            "scripts/alignment/build_topic1_task_registry.py",
        ],
    },
    "T1-M01-N004": {
        "candidate-git-identity": [
            "scripts/alignment/test_candidate_freeze.py",
            "scripts/alignment/capture_g0.py",
            "scripts/alignment/candidate_snapshot.py",
        ],
        "candidate-main-fail-closed": [
            "scripts/alignment/test_candidate_freeze.py",
            "scripts/alignment/capture_g0.py",
            "scripts/alignment/candidate_snapshot.py",
        ],
        "candidate-freeze-fixture": [
            "scripts/alignment/test_candidate_freeze.py",
            "scripts/alignment/capture_g0.py",
            "scripts/alignment/candidate_snapshot.py",
        ],
        "candidate-freeze-run": [
            "scripts/alignment/test_candidate_freeze.py",
            "scripts/alignment/capture_g0.py",
            "scripts/alignment/candidate_snapshot.py",
        ],
    },
    "T1-M01-N005": {
        "contract-inventory-schema": [
            "contracts/alignment/topic1-contract-inventory.schema.json",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/feature-contract-registry.v1.json",
        ],
        "contract-inventory-builder": [
            "scripts/alignment/build_topic1_contract_inventory.py",
            "contracts/alignment/topic1-contract-inventory.schema.json",
            "contracts/alignment/topic1-contract-inventory.v1.json",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/feature-contract-registry.v1.json",
        ],
        "contract-inventory-mutation-oracle": [
            "scripts/alignment/test_topic1_contract_inventory.py",
            "scripts/alignment/build_topic1_contract_inventory.py",
            "contracts/alignment/topic1-contract-inventory.schema.json",
            "contracts/alignment/topic1-contract-inventory.v1.json",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/feature-contract-registry.v1.json",
        ],
        "contract-inventory-verification": [
            "scripts/alignment/test_topic1_contract_inventory.py",
            "scripts/alignment/build_topic1_contract_inventory.py",
            "contracts/alignment/topic1-contract-inventory.schema.json",
            "contracts/alignment/topic1-contract-inventory.v1.json",
            "contracts/alignment/canonical-registry.json",
            "contracts/alignment/feature-contract-registry.v1.json",
        ],
    },
    "T1-M01-N008": {
        "proto-topic-matrix-schema": [
            "contracts/events/proto-topic-compatibility-matrix.schema.json",
            *PROTO_TOPIC_AUTHORITY_INPUTS,
        ],
        "proto-topic-matrix-builder": [
            "scripts/alignment/build_proto_topic_compatibility_matrix.py",
            "contracts/events/proto-topic-compatibility-matrix.schema.json",
            "contracts/events/proto-topic-compatibility-matrix.v1.json",
            *PROTO_TOPIC_AUTHORITY_INPUTS,
        ],
        "proto-topic-matrix-mutation-oracle": [
            "scripts/alignment/test_proto_topic_compatibility_matrix.py",
            "scripts/alignment/build_proto_topic_compatibility_matrix.py",
            "contracts/events/proto-topic-compatibility-matrix.schema.json",
            "contracts/events/proto-topic-compatibility-matrix.v1.json",
            *PROTO_TOPIC_AUTHORITY_INPUTS,
        ],
        "proto-topic-matrix-verification": [
            "scripts/alignment/test_proto_topic_compatibility_matrix.py",
            "scripts/alignment/build_proto_topic_compatibility_matrix.py",
            "contracts/events/proto-topic-compatibility-matrix.schema.json",
            "contracts/events/proto-topic-compatibility-matrix.v1.json",
            *PROTO_TOPIC_AUTHORITY_INPUTS,
        ],
    },
    "T1-M01-N009": {
        "schema-authority-contract": [
            "contracts/alignment/schema-authority-registry.schema.json",
            *SCHEMA_AUTHORITY_INPUTS,
        ],
        "schema-authority-builder": [
            "scripts/alignment/build_schema_authority_registry.py",
            "contracts/alignment/schema-authority-registry.v1.json",
            "contracts/alignment/schema-authority-registry.schema.json",
            *SCHEMA_AUTHORITY_INPUTS,
        ],
        "schema-authority-mutation-oracle": [
            "scripts/alignment/test_schema_authority_registry.py",
            "scripts/alignment/build_schema_authority_registry.py",
            "contracts/alignment/schema-authority-registry.v1.json",
            "contracts/alignment/schema-authority-registry.schema.json",
            *SCHEMA_AUTHORITY_INPUTS,
        ],
        "schema-authority-verification": [
            "scripts/alignment/test_schema_authority_registry.py",
            "scripts/alignment/build_schema_authority_registry.py",
            "contracts/alignment/schema-authority-registry.v1.json",
            "contracts/alignment/schema-authority-registry.schema.json",
            *SCHEMA_AUTHORITY_INPUTS,
        ],
    },
    "T1-M01-N010": {
        "trusted-signature-contracts": [
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "trusted-verifier-adapter": [
            "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py",
            "scripts/alignment/build_topic1_task_registry.py",
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "trusted-verifier-wrapper": [
            "scripts/alignment/build_topic1_task_registry.py",
            "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py",
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-candidate-artifact-refs": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-implementation-candidate": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-requirement-satisfaction": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-bom-transition": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-external-signature": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-signed-contract-intake": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-execution-overlay": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-fail-closed-selftest": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "caller-work-order-evidence-run": [
            "scripts/alignment/build_topic1_task_registry.py", "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/test_trusted_signature_verifier.py", "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json", "contracts/alignment/signature-verification-attestation.schema.json",
            "contracts/alignment/evidence-case-report.schema.json", "contracts/alignment/evidence-run-manifest.schema.json",
        ],
        "trusted-signature-fixture": [
            "scripts/alignment/test_trusted_signature_verifier.py",
            "scripts/alignment/build_topic1_task_registry.py",
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "trusted-signature-negative-run": [
            "scripts/alignment/test_trusted_signature_verifier.py",
            "scripts/alignment/verify_trusted_signature.py",
            "scripts/alignment/build_topic1_task_registry.py",
        ],
        "trusted-verifier-protected-backend": [
            "deployments/security/topic1-trusted-signature-verifier.yaml",
            "scripts/alignment/test_trusted_signature_verifier.py",
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
        "trusted-signature-positive-run": [
            "scripts/alignment/test_trusted_signature_verifier.py",
            "deployments/security/topic1-trusted-signature-verifier.yaml",
            "scripts/alignment/verify_trusted_signature.py",
            "contracts/alignment/signature-trust-policy.schema.json",
            "contracts/alignment/signature-verification-request.schema.json",
            "contracts/alignment/signature-verification-attestation.schema.json",
        ],
    },
    "T1-M01-N011": {
        "step-1": [
            "contracts/alignment/common-response-protocol.v1.json",
            "contracts/alignment/adapter-risk-registry.v1.json",
            "web/ui/src/services/api.ts",
        ],
    },
    "T1-M02-N013": {
        "step-1": [
            "deployments/kubernetes/applications/probe-agent.yaml",
            "deployments/kubernetes/applications/go-services.yaml",
            "deployments/kubernetes/infrastructure/07-flink.yaml",
        ],
    },
    "T1-M02-N014": {
        "step-1": [
            "rust/probe-agent/probe-agent/tests/pcap_offline_test.rs",
            "rust/probe-agent/probe-agent/tests/integration_test.rs",
            "scripts/alignment/verify_pcap_metadata_ack.py",
            "scripts/alignment/verify_kafka_dlq_commit_barrier.py",
        ],
    },
    "T1-M06-N012": {
        "clickhouse-flow-projection": [
            "common/sql/ch/00-all-tables.sql",
            "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/SessionJob.java",
        ],
        "clickhouse-asset-projection": [
            "common/sql/ch/01-extended.sql",
            "go/control-plane/internal/asset/consumer/asset_projection_worker.go",
            "go/control-plane/internal/asset/consumer/asset_clickhouse_projection.go",
        ],
        "clickhouse-device-log-projection": [
            "common/sql/ch/01-extended.sql",
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java",
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/ClickHouseSinkFactory.java",
        ],
        "clickhouse-user-event-projection": [
            "common/sql/ch/01-extended.sql",
            "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java",
            "java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/sink/ClickHouseUserEventSink.java",
        ],
        "opensearch-asset-projection": [
            "common/opensearch/assets-v2/index-template.json",
            "go/control-plane/internal/asset/consumer/asset_projection_targets.go",
            "go/control-plane/internal/asset/consumer/asset_projection_worker.go",
        ],
        "opensearch-device-log-projection": [
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java",
            "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/sink/OpenSearchSinkFactory.java",
        ],
    },
    "T1-M06-N016": {
        "asset-binding-contract": [
            "proto/traffic/v1/asset.proto",
            "proto/traffic/v1/ingest.proto",
            "contracts/events/kafka-topic-catalog.v1.json",
            "contracts/events/kafka-acl-catalog.v1.json",
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
        ],
        "asset-binding-consumer-ready": [
            "go/control-plane/internal/asset/consumer/binding_consumer.go",
            "go/control-plane/internal/asset/consumer/binding_consumer_test.go",
            "go/control-plane/cmd/asset-service/main.go",
            "go/control-plane/internal/asset/config/config.go",
        ],
        "asset-binding-ingest-bridge-default-off": [
            "proto/traffic/v1/ingest.proto",
            "go/control-plane/internal/ingest/server/handler.go",
            "go/control-plane/internal/ingest/queue/producer.go",
            "go/control-plane/internal/ingest/config/config.go",
            "go/control-plane/cmd/ingest-gateway/main.go",
        ],
        "asset-binding-probe-grpc-sender-default-off": [
            "rust/probe-agent/probe-agent/src/parser/arp.rs",
            "rust/probe-agent/probe-agent/src/parser/dhcp.rs",
            "rust/probe-agent/probe-agent/src/aggregator/packet_processor.rs",
            "rust/probe-agent/probe-agent/src/sender/grpc.rs",
            "rust/probe-agent/probe-agent/src/sender/mod.rs",
            "rust/probe-agent/probe-agent/src/main.rs",
        ],
        "device-log-contract": [
            "proto/traffic/v1/audit.proto",
            "contracts/events/kafka-topic-catalog.v1.json",
            "contracts/events/kafka-acl-catalog.v1.json",
            "contracts/events/device-logs-producer.v1.json",
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
        ],
        "device-log-producer-default-off": [
            "deployments/log-collectors/device-logs.yaml",
            "deployments/log-collectors/device-logs-config.yaml",
            "deployments/log-collectors/device-logs-secret-ref.yaml",
        ],
    },
    "T1-M06-N017": {
        "asset-events-canary": ["go/control-plane/internal/asset/repository/outbox_dispatcher.go", "go/control-plane/cmd/asset-service/main.go"],
        "asset-events-acceptance": ["go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go"],
        "asset-binding-ingest-bridge-canary": ["go/control-plane/internal/ingest/server/handler.go", "go/control-plane/internal/ingest/queue/producer.go", "go/control-plane/cmd/ingest-gateway/main.go"],
        "asset-binding-probe-sender-canary": ["rust/probe-agent/probe-agent/src/sender/grpc.rs", "rust/probe-agent/probe-agent/src/main.rs"],
        "asset-bindings-acceptance": ["go/control-plane/internal/asset/consumer/binding_consumer_test.go", "scripts/alignment/"],
        "device-logs-canary": ["deployments/log-collectors/device-logs.yaml", "java/flink-jobs/flink-log-job/src/main/java/com/traffic/flink/log/LogJob.java"],
        "device-logs-acceptance": ["java/flink-jobs/flink-log-job/src/test/", "scripts/alignment/"],
    },
    "T1-M07-N018": {
        "campaign-contract": [
            "proto/traffic/v1/alert.proto",
            "contracts/events/kafka-topic-catalog.v1.json",
            "contracts/events/kafka-acl-catalog.v1.json",
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
            "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
        ],
        "campaign-expand": [
            "deployments/postgres/migrations/202608010800_campaign_event_delivery_projection_v2.sql",
            "deployments/postgres/migrations/202608101000_campaign_detection_projection_v1.sql",
        ],
        "campaigns-v1-protobuf-consumer-ready": [
            "go/control-plane/internal/alert/consumer/campaign_detection_consumer.go",
            "go/control-plane/internal/alert/api/campaign_detection_projection.go",
            "go/control-plane/internal/alert/config/config.go",
            "go/control-plane/cmd/alert-service/main.go",
        ],
        "cep-tenant-key-isolation-refactor": [
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/CepJob.java",
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/ScanExploitSelector.java",
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/CampaignBuilderUtils.java",
        ],
        "cep-tenant-key-isolation-negative-test": [
            "java/flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/CepJobIntegrationTest.java",
            "java/flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/select/CampaignSelectorTest.java",
            "java/flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/select/CampaignBuilderUtilsTest.java",
        ],
        "cep-publisher-default-off": [
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/CepJob.java",
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/sink/KafkaSinkFactory.java",
            "java/flink-jobs/flink-cep-job/src/main/resources/cep-job.properties",
            "java/flink-jobs/flink-cep-job/deployments/k8s/deployment.yaml",
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
        ],
        "campaign-authority-json-v2-consumer-ready": [
            "go/control-plane/internal/alert/consumer/campaign_event_consumer.go",
            "go/control-plane/internal/alert/api/campaign_event_projection.go",
            "go/control-plane/internal/alert/config/config.go",
            "go/control-plane/cmd/alert-service/main.go",
            "go/control-plane/internal/alert/config/config.go",
        ],
        "campaign-authority-json-v2-command-writer": [
            "go/control-plane/internal/alert/api/campaign_aggregate_v2.go",
            "go/control-plane/internal/alert/api/campaign_membership_v2.go",
            "go/control-plane/internal/alert/api/handler_campaign_outbox.go",
            "go/control-plane/internal/alert/config/config.go",
            "go/control-plane/cmd/alert-service/main.go",
            "go/control-plane/deployments/kubernetes/alert-service.yaml",
            "deployments/kubernetes/applications/go-services.yaml",
        ],
        "campaign-rail-correlation-contract": [
            "contracts/alignment/features/F-CAMPAIGN-001.json",
            "contracts/events/campaign-rail-correlation.v1.json",
        ],
        "campaign-rail-correlation-expand": [
            "deployments/postgres/migrations/202608101030_campaign_rail_correlation_v1.sql",
        ],
        "campaign-rail-correlation-projection": [
            "go/control-plane/internal/alert/api/campaign_rail_correlation.go",
            "go/control-plane/internal/alert/api/campaign_detection_projection.go",
        ],
        "cep-publisher-canary": [
            "java/flink-jobs/flink-cep-job/deployments/k8s/deployment.yaml",
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
        ],
        "campaign-authority-consumer-canary": [
            "go/control-plane/internal/alert/config/config.go",
            "go/control-plane/cmd/alert-service/main.go",
            "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ],
        "campaign-authority-dispatcher-canary": [
            "go/control-plane/internal/alert/config/config.go",
            "go/control-plane/cmd/alert-service/main.go",
            "go/control-plane/deployments/kubernetes/alert-service.yaml",
        ],
        "campaign-correlation-canary": [
            "go/control-plane/cmd/alert-service/main.go",
            "java/flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/CepJob.java",
        ],
        "campaign-reconcile": [
            "go/control-plane/internal/alert/api/campaign_event_pipeline_integration_test.go",
            "go/control-plane/internal/alert/api/campaign_four_store_trace_integration_test.go",
        ],
    },
}

TASK_REQUIRED_GATES = {
    "T1-M02-N015": ["G4"],
    "T1-M03-N014": ["G4"],
    "T1-M09-N023": ["G5"],
    "T1-M10-N015": ["G2", "G3", "G5", "G6"],
    "T1-M13-N009": ["G7"],
    "T1-M13-N016": ["G4"],
    "T1-M13-N017": ["G6"],
    "T1-M13-N018": ["G6"],
    "T1-M13-N019": ["G5"],
}

TASK_REQUIRED_PROFILES = {
    "T1-M02-N015": ["APPROVED_T1_TEN_GIGABIT_OR_HIGHER_PROFILE"],
    "T1-M03-N014": ["APPROVED_T1_TEN_GIGABIT_OR_HIGHER_PROFILE"],
    "T1-M04-N010": ["T1_MIDTERM_SIGNED_METRIC_PROFILE"],
    "T1-M05-N004": ["T1_MIDTERM_SIGNED_METRIC_PROFILE"],
    "T1-M11-N011": ["PROPOSED_T1_CNAS_QUALITY_PROFILE"],
    "T1-M11-N012": ["PROPOSED_T1_CNAS_QUALITY_PROFILE"],
    "T1-M12-N006": ["PROPOSED_T1_CONTRACT_PROFILE"],
    "T1-M12-N007": ["PROPOSED_T1_CONTRACT_PROFILE"],
    "T1-M13-N020": ["PROPOSED_T1_ENGINEERING_PROFILE"],
}

CLOSURE_SLICE_PR_TYPES = {
    "R00": ["CTR", "TST-PRE", "IDX"],
    "R01": ["CTR", "REF", "TST-PRE", "IDX"],
    "R02": ["CTR", "OPS", "TST-POST", "IDX"],
    "R03": ["CTR", "EXP", "WRT", "OPS", "TST-POST", "IDX"],
    "R04": ["CTR", "EXP", "PRJ", "WRT", "OPS", "TST-POST", "IDX"],
    "R05": ["CTR", "EXP", "WRT", "UI", "TST-POST", "IDX"],
    "R06": ["CTR", "EXP", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R07": ["CTR", "EXP", "PRJ", "OPS", "TST-POST", "IDX"],
    "R08": ["CTR", "EXP", "PRJ", "OPS", "TST-POST", "IDX"],
    "R09": ["CTR", "PRJ", "OPS", "TST-POST", "IDX"],
    "R10": ["CTR", "PRJ", "OPS", "TST-POST", "IDX"],
    "R11": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R12": ["CTR", "WRT", "OPS", "TST-POST", "IDX"],
    "R13": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R14": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R15": ["CTR", "EXP", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R16": ["CTR", "EXP", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R17": ["CTR", "EXP", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R18": ["CTR", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R19": ["CTR", "EXP", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R20": ["CTR", "EXP", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R21": ["CTR", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R22": ["CTR", "PRJ", "WRT", "UI", "OPS", "TST-POST", "IDX"],
    "R23": ["CTR", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R24": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R25": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R26": ["CTR", "WRT", "PRJ", "UI", "TST-POST", "IDX"],
    "R27": ["CTR", "EXP", "WRT", "PRJ", "UI", "OPS", "TST-POST", "IDX"],
    "R28": ["CTR", "EXP", "OPS", "TST-POST", "IDX"],
    "R29": ["CTR", "IDX"],
}

# Review-time navigation aids for table cells that intentionally name a logical
# surface rather than one file. They are candidates, not READY-time ownership.
TARGET_ALIASES = {
    "Kafka兼容consumer、Flink common": [
        "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/ProtoDeserializer.java",
        "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/FlowEventDeserializer.java",
    ],
    "MinIO object governance、PCAP manifest": [
        "contracts/kafka/pcap-metadata-ack.v1.json",
        "scripts/alignment/verify_pcap_metadata_ack.py",
    ],
    "Probe注册/heartbeat/control ACK": [
        "proto/traffic/v1/ingest.proto",
        "go/control-plane/internal/alert/consumer/probe_ack_consumer.go",
        "go/control-plane/internal/alert/consumer/probe_operation_event_consumer.go",
    ],
    "Rust aggregator": [
        "rust/probe-agent/probe-agent/src/aggregator/mod.rs",
        "rust/probe-agent/probe-agent/src/aggregator/flow_table.rs",
    ],
    "Feature fingerprint相关代码": [
        "proto/traffic/v1/feature.proto",
        "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/FeatureJob.java",
        "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/calculator/FeatureCalculator.java",
    ],
    "ClickHouse批量sink": [
        "java/flink-jobs/flink-session-job/src/main/java/com/traffic/flink/session/SessionJob.java",
        "java/flink-jobs/flink-feature-job/src/main/java/com/traffic/flink/feature/FeatureJob.java",
    ],
    "asset Kafka consumer/projection": [
        "go/control-plane/internal/asset/consumer/asset_projection_event.go",
        "go/control-plane/internal/asset/consumer/asset_projection_worker.go",
    ],
    "model-updates consumer": [
        "java/flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/BehaviorDetectionJob.java",
        "go/control-plane/internal/rules/service/model_service.go",
    ],
    "feedback consumer/inbox ready": [
        "go/control-plane/internal/rules/consumer/alert_feedback_event_consumer.go",
        "go/control-plane/internal/rules/consumer/model_feedback_inbox_worker.go",
    ],
    "forensics task PG command": [
        "go/control-plane/internal/forensics/repository/task_command_atomic.go",
        "go/control-plane/internal/forensics/api/handler.go",
    ],
    "forensics worker/MinIO": [
        "go/control-plane/internal/forensics/task/async_cutter.go",
        "go/control-plane/internal/forensics/index/clickhouse.go",
        "go/control-plane/internal/forensics/s3client/client.go",
        "go/control-plane/internal/forensics/cutter/pcap_cutter.go",
    ],
    "scoped backup/restore and rebuild": [
        "scripts/alignment/build_dr_recovery_catalog.py",
        "scripts/alignment/capture_dr_recovery_catalog.py",
        "scripts/alignment/verify_dr_recovery_catalog.py",
        "scripts/alignment/capture_flink_state_recovery.py",
        "scripts/alignment/verify_flink_state_recovery.py",
    ],
    "candidate provenance guard": [
        "scripts/alignment/capture_g0.py",
        "scripts/alignment/candidate_snapshot.py",
    ],
    "APISIX/routes/services": [
        "deployments/kubernetes/configmaps/apisix-routes.yaml",
        "contracts/gateway/route-catalog.v1.json",
    ],
}

CLOSURE_SLICE_PATH_HINTS = {
    "R00": ["contracts/alignment/", "contracts/openapi/", "proto/traffic/v1/", "go/control-plane/internal/auth/", "web/ui/src/services/"],
    "R01": ["contracts/alignment/", "go/control-plane/internal/httpx/", "web/ui/src/services/"],
    "R02": ["deployments/kubernetes/security/", "deployments/kubernetes/configmaps/", "go/control-plane/internal/auth/"],
    "R03": ["common/sql/pg/", "deployments/postgres/migrations/", "go/control-plane/internal/"],
    "R04": ["contracts/events/", "deployments/kubernetes/init-jobs/00-kafka-", "deployments/kubernetes/init-jobs/01-kafka-topics.yaml", "go/control-plane/internal/", "java/flink-jobs/"],
    "R05": ["go/control-plane/internal/audit/", "go/control-plane/internal/alert/audit/", "web/ui/src/pages/AuditLogPage.tsx", "deployments/postgres/migrations/202607302100_audit_schema_authority.sql"],
    "R06": ["go/control-plane/internal/forensics/", "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml", "web/ui/src/pages/ForensicsWorkbenchPage.tsx"],
    "R07": ["common/sql/ch/", "deployments/clickhouse/migrations/", "java/flink-jobs/"],
    "R08": ["common/opensearch/", "go/control-plane/internal/alert/persistence/opensearch.go", "go/control-plane/internal/alert/repository/opensearch.go"],
    "R09": ["java/flink-jobs/", "deployments/kubernetes/flink/"],
    "R10": ["common/redis/", "go/control-plane/internal/"],
    "R11": ["go/control-plane/internal/alert/api/handler_data_quality", "deployments/kubernetes/observability/", "scripts/alignment/"],
    "R12": ["rust/probe-agent/", "go/control-plane/internal/ingest/", "deployments/kubernetes/applications/probe-agent.yaml"],
    "R13": ["go/control-plane/internal/asset/", "web/ui/src/pages/Asset", "deployments/postgres/migrations/"],
    "R14": ["go/control-plane/internal/alert/", "java/flink-jobs/flink-alert-generator-job/", "web/ui/src/pages/Alert"],
    "R15": ["go/control-plane/internal/alert/campaign/", "go/control-plane/internal/alert/consumer/campaign_event_consumer.go", "java/flink-jobs/flink-cep-job/", "web/ui/src/pages/Campaign", "web/ui/src/pages/AttackChain"],
    "R16": ["go/control-plane/internal/alert/api/handler_data_quality", "java/flink-jobs/flink-behavior-job/", "web/ui/src/pages/Baseline"],
    "R17": ["mlops/", "go/control-plane/internal/rules/", "java/flink-jobs/flink-behavior-job/", "web/ui/src/pages/ModelManagementPage.tsx"],
    "R18": ["go/control-plane/internal/alert/api/dashboard", "web/ui/src/pages/Dashboard", "web/ui/src/pages/Screen"],
    "R19": ["go/control-plane/internal/graph/", "common/nebula/", "deployments/kubernetes/init-jobs/05-nebula-schema.yaml", "web/ui/src/pages/AttackChainAnalysisPage.tsx"],
    "R20": ["go/control-plane/internal/asset/", "go/control-plane/internal/graph/", "web/ui/src/pages/AssetDetailWorkspace.tsx"],
    "R21": ["go/control-plane/internal/alert/repository/opensearch", "go/control-plane/internal/asset/api/asset_exports.go", "web/ui/src/services/assetExportApi.ts"],
    "R22": ["go/control-plane/internal/alert/api/handler_topic", "web/ui/src/pages/TopicWorkbenchPage.tsx", "web/ui/src/services/topic"],
    "R23": ["java/flink-jobs/flink-feature-job/", "mlops/", "go/control-plane/internal/alert/api/handler_product_pages.go", "web/ui/src/pages/EncryptedTrafficPage.tsx"],
    "R24": ["go/control-plane/internal/rules/", "web/ui/src/pages/Rule", "web/ui/src/pages/Deployment"],
    "R25": ["go/control-plane/internal/alert/playbook/", "go/control-plane/internal/alert/whitelist/", "web/ui/src/pages/Playbook", "web/ui/src/pages/Whitelist"],
    "R26": ["go/control-plane/internal/alert/api/handler_compliance_workflows.go", "web/ui/src/pages/Compliance"],
    "R27": ["go/control-plane/internal/alert/api/notification", "go/control-plane/internal/auth/api/system_settings_handler.go", "web/ui/src/pages/Notification", "web/ui/src/pages/System"],
    "R28": ["scripts/alignment/build_dr_recovery_catalog.py", "scripts/alignment/capture_dr_recovery_catalog.py", "scripts/alignment/verify_dr_recovery_catalog.py", "deployments/kubernetes/infrastructure/"],
    "R29": ["go/control-plane/internal/alert/api/notification", "web/ui/src/pages/Notification", "common/redis/"],
}


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def normalize_pr_types(raw: str) -> list[str]:
    if raw.startswith("external_activity:"):
        return []
    if raw in PR_TYPE_OVERRIDES:
        return list(PR_TYPE_OVERRIDES[raw])
    result = []
    for match in PR_TYPE_RE.findall(raw):
        value = "TST-POST" if match == "TST" else match
        if value not in VALID_PR_TYPES:
            raise ValueError(f"unsupported PR type {value!r} from {raw!r}")
        result.append(value)
    if not result:
        raise ValueError(f"no atomic PR type in {raw!r}")
    return result


def gates_for(pr_type: str) -> list[str]:
    return {
        "CTR": ["G0"],
        "EXP": ["G1"],
        "PRJ": ["G2", "G3"],
        "WRT": ["G2", "G3"],
        "UI": ["G5"],
        "OPS": ["G6"],
        "REF": ["G0"],
        "TST-PRE": ["G0", "G1"],
        "TST-POST": ["G2", "G3"],
        "IDX": ["G0"],
        "PROM": ["G0"],
    }[pr_type]


def runtime_state(pr_type: str) -> str:
    if pr_type in {"PRJ", "WRT", "UI", "OPS"}:
        return "off"
    if pr_type in {"CTR", "EXP", "REF", "IDX", "PROM", "TST-PRE", "TST-POST"}:
        return "no_runtime_change"
    return "not_applicable"


def required_gates(task_id: str, pr_type: str) -> list[str]:
    return sorted(set(gates_for(pr_type)) | set(TASK_REQUIRED_GATES.get(task_id, [])))


def proof_boundary(pr_type: str) -> tuple[list[str], list[str]]:
    proves = {
        "CTR": ["contract and compatibility definition"],
        "EXP": ["additive schema/config preparation"],
        "PRJ": ["consumer or projection path on its declared dependency profile"],
        "WRT": ["authority write/outbox/publisher path on its declared dependency profile"],
        "UI": ["typed browser-facing path for declared journeys"],
        "OPS": ["declared rollout, canary, rollback, or observation operation"],
        "REF": ["behavior-preserving refactor under characterization tests"],
        "TST-PRE": ["pre-enable static/compatibility test result"],
        "TST-POST": ["post-enable scoped runtime or reconciliation result"],
        "IDX": ["append-only indexing of pre-existing evidence"],
        "PROM": ["promotion intent and allowed-path equivalence only"],
    }[pr_type]
    does_not_prove = [
        "any undeclared gate, environment, profile, time window, downstream final fact, or external attestation"
    ]
    if pr_type == "PROM":
        does_not_prove.append("new runtime evidence or a changed production candidate")
    return proves, does_not_prove


def milestone_profile(milestone: str) -> str:
    return (
        "PROPOSED_T1_CNAS_QUALITY_PROFILE" if milestone == "M11"
        else "PROPOSED_T1_CONTRACT_PROFILE" if milestone == "M12"
        else "T1_REMEDIATION_AND_ENGINEERING_SPLIT_PROFILES" if milestone == "M13"
        else f"T1_{milestone}_MILESTONE_PROFILE"
    )


def iter_objects(value: Any):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from iter_objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from iter_objects(child)


@lru_cache(maxsize=1)
def canonical_ids() -> list[str]:
    payload = json.loads(CANONICAL_PATH.read_text(encoding="utf-8"))
    values = {
        obj["id"]
        for obj in iter_objects(payload)
        if isinstance(obj.get("id"), str) and ID_RE.fullmatch(obj["id"])
    }
    if not values:
        raise ValueError("canonical registry yielded an empty ID set; schema drift is fail-closed")
    return sorted(values)


def canonical_slices(lines: list[str]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for line in lines:
        match = SLICE_ROW_RE.match(line)
        if not match:
            continue
        slice_id = match.group("slice")
        result.append(
            {
                "slice_id": f"T1-M13-{slice_id}",
                "short_id": slice_id,
                "canonical_ids": CANONICAL_REF_RE.findall(match.group("ids")),
                "surface": match.group("surface").strip(),
                "minimum_closure_rule": match.group("minimum").strip(),
            }
        )
    if not result:
        raise ValueError("R00-R29 slice table is missing or empty")
    expected = [f"R{index:02d}" for index in range(30)]
    actual = [item["short_id"] for item in result]
    if actual != expected:
        raise ValueError(f"closure slices are missing or out of order: {actual}")
    return result


def canonical_slice_ids(lines: list[str]) -> list[str]:
    return sorted(
        canonical_id
        for item in canonical_slices(lines)
        for canonical_id in item["canonical_ids"]
    )


@lru_cache(maxsize=1)
def requirement_ids() -> set[str]:
    if not REQUIREMENT_PATH.exists():
        return set()
    payload = json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8"))
    return {
        item["requirement_id"]
        for item in payload.get("requirements", [])
        if isinstance(item.get("requirement_id"), str)
    }


@lru_cache(maxsize=1)
def repository_files() -> tuple[str, ...]:
    ignored = {".git", "node_modules", "target", "dist", "build", ".venv", "venv"}
    result = []
    for path in REPO_ROOT.rglob("*"):
        if not path.is_file() or any(part in ignored for part in path.relative_to(REPO_ROOT).parts):
            continue
        result.append(path.relative_to(REPO_ROOT).as_posix())
    return tuple(sorted(result))


def discover_symbol_candidates(paths: list[str]) -> list[dict[str, Any]]:
    patterns = {
        ".go": re.compile(r"^(?:func\s+(?:\([^)]*\)\s*)?|type\s+)([A-Za-z_]\w*)"),
        ".rs": re.compile(r"^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait)\s+([A-Za-z_]\w*)"),
        ".java": re.compile(r"^(?:public\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+([A-Za-z_]\w*)"),
        ".ts": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".tsx": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".py": re.compile(r"^(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)"),
        ".proto": re.compile(r"^(?:message|enum|service)\s+([A-Za-z_]\w*)"),
    }
    result: list[dict[str, Any]] = []
    for relative in paths[:20]:
        path = REPO_ROOT / relative
        pattern = patterns.get(path.suffix)
        if not pattern or not path.is_file():
            continue
        try:
            lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
        except OSError:
            continue
        for line_number, line in enumerate(lines, start=1):
            match = pattern.match(line.strip())
            if match:
                result.append({"path": relative, "symbol": match.group(1), "line": line_number})
                if len(result) >= 20:
                    return result
    return result


def exact_declaration_signature(
    path: str, symbol: str, blob: bytes,
) -> str | None:
    """Resolve one declaration line, never an arbitrary textual occurrence."""
    patterns = {
        ".go": re.compile(r"^(?:func\s+(?:\([^)]*\)\s*)?|type\s+)([A-Za-z_]\w*)"),
        ".rs": re.compile(r"^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait)\s+([A-Za-z_]\w*)"),
        ".java": re.compile(r"^(?:public\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+([A-Za-z_]\w*)"),
        ".ts": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".tsx": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".py": re.compile(r"^(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)"),
        ".proto": re.compile(r"^(?:message|enum|service)\s+([A-Za-z_]\w*)"),
    }
    pattern = patterns.get(Path(path).suffix.lower())
    if pattern is None:
        return None
    matches = []
    lines = blob.decode("utf-8", errors="replace").splitlines()
    for line_index, line in enumerate(lines):
        normalized = " ".join(line.strip().split())
        match = pattern.match(normalized)
        wanted = symbol.rsplit(".", 1)[-1] if Path(path).suffix.lower() == ".go" else symbol
        if match and match.group(1) == wanted:
            if Path(path).suffix.lower() == ".go" and normalized.startswith("func "):
                declaration_lines = [line.strip()]
                cursor = line_index
                while "{" not in " ".join(declaration_lines) and cursor + 1 < len(lines):
                    cursor += 1
                    declaration_lines.append(lines[cursor].strip())
                declaration = " ".join(" ".join(declaration_lines).split())
                declaration = declaration.split("{", 1)[0].rstrip()
                matches.append(declaration)
            else:
                matches.append(normalized)
    return matches[0] if len(matches) == 1 else None


def path_candidates(value: str) -> list[str]:
    normalized = value.strip().strip("/")
    if not normalized or any(mark in normalized for mark in ("<", ">")):
        return []
    if normalized in TARGET_ALIASES:
        return [path for path in TARGET_ALIASES[normalized] if (REPO_ROOT / path).is_file()]
    files = repository_files()
    if "/.../" in normalized:
        prefix, suffix = normalized.split("/.../", 1)
        return [item for item in files if item.startswith(prefix + "/") and item.endswith("/" + suffix)]
    if "/" in normalized:
        direct = [item for item in files if item == normalized or item.endswith("/" + normalized)]
        if direct:
            return direct
        wanted = normalized.split("/")
        basename = wanted[-1]
        flexible = []
        for item in files:
            parts = item.split("/")
            if parts[-1] != basename:
                continue
            position = 0
            for part in parts:
                if position < len(wanted) and part == wanted[position]:
                    position += 1
            if position == len(wanted):
                flexible.append(item)
        return flexible
    if Path(normalized).suffix:
        return [item for item in files if Path(item).name == normalized]
    return []


def resolve_targets(raw: str) -> list[dict[str, Any]]:
    tokens = BACKTICK_RE.findall(raw)
    if not tokens:
        tokens = [token.strip() for token in re.split(r"[;；]", raw) if token.strip()]
    if not tokens:
        tokens = ["unresolved-target"]
    targets = []
    proposed_context = any(marker in raw.lower() for marker in ("拟", "planned", "proposed", "新增"))
    for token in tokens:
        value = token.strip()
        candidates: list[str] = []
        symbols: list[dict[str, Any]] = []
        if ID_RE.fullmatch(value):
            kind = "CONTRACT_ID"
            exists = value in set(canonical_ids())
            resolution_status = "REGISTRY_MATCH" if exists else "UNRESOLVED"
        elif value.startswith("REQ-T1-"):
            kind = "CONTRACT_ID"
            exists = value in requirement_ids()
            resolution_status = "REGISTRY_MATCH" if exists else "UNRESOLVED"
        else:
            candidate = REPO_ROOT / value
            if "*" in value:
                kind = "EXISTING_GLOB"
                candidates = sorted(
                    path.relative_to(REPO_ROOT).as_posix()
                    for path in REPO_ROOT.glob(value)
                    if path.is_file()
                )
                exists = bool(candidates)
                resolution_status = "GLOB_MATCH" if exists else "UNRESOLVED"
            elif candidate.exists() and candidate.is_file():
                kind = "EXISTING_FILE"
                exists = True
                candidates = [candidate.relative_to(REPO_ROOT).as_posix()]
                resolution_status = "EXACT_FILE"
            elif candidate.exists() and candidate.is_dir():
                kind = "EXISTING_DIRECTORY"
                exists = True
                candidates = [candidate.relative_to(REPO_ROOT).as_posix()]
                resolution_status = "EXACT_DIRECTORY"
            elif matched := path_candidates(value):
                candidates = matched[:20]
                exists = True
                kind = "EXISTING_CANDIDATES"
                resolution_status = "UNIQUE_CANDIDATE" if len(matched) == 1 else "AMBIGUOUS_CANDIDATES"
            elif proposed_context or "<" in value or value.startswith("contracts/"):
                kind = "PROPOSED_FILE"
                exists = candidate.exists()
                if (
                    not any(mark in value for mark in ("<", ">", "*", ",", "，", "、"))
                    and (Path(value).suffix or Path(value).name.startswith("Dockerfile"))
                ):
                    candidates = [value]
                resolution_status = "PLANNED" if not exists else "EXACT_FILE"
            else:
                kind = "LOGICAL_ARTIFACT"
                exists = False
                resolution_status = "LOGICAL_ONLY"
        if candidates:
            symbols = discover_symbol_candidates(candidates)
        targets.append(
            {
                "kind": kind,
                "value": value,
                "exists_in_snapshot": exists,
                "resolution_status": resolution_status,
                "candidate_paths": candidates,
                "symbol_candidates": symbols,
                "resolution_note": (
                    "exact path exists; select task-specific symbol and compatibility entrypoint before READY"
                    if exists
                    else "planned or logical target; create an approved target contract before READY"
                ),
            }
        )
    return targets


MILESTONE_OWNER_ROLES = {
    "M00": "requirements-owner",
    "M01": "platform-contract-owner",
    "M02": "capture-ingest-owner",
    "M03": "traffic-parser-feature-owner",
    "M04": "detection-quality-owner",
    "M05": "midterm-release-owner",
    "M06": "multi-source-data-owner",
    "M07": "fusion-graph-owner",
    "M08": "ml-model-governance-owner",
    "M09": "analysis-product-owner",
    "M10": "site-release-sre-owner",
    "M11": "independent-quality-gate-owner",
    "M12": "contract-release-owner",
    "M13": "remediation-and-engineering-owner",
}


def task_requirements(milestone: str, number: int) -> list[str]:
    if milestone == "M13" and number <= 10:
        return []
    if milestone == "M03":
        values = list(MILESTONE_REQUIREMENTS[milestone])
        if number not in {15, 16, 17, 18}:
            values = [
                item for item in values
                if item != "REQ-T1-FILE-RESTORE-001"
            ]
        return values
    if milestone == "M09":
        values = list(MILESTONE_REQUIREMENTS[milestone])
        file_restore = "REQ-T1-FILE-RESTORE-001"
        if number not in {1, 9, 10, 11, 23, 24}:
            values = [item for item in values if item != file_restore]
        return values
    return list(MILESTONE_REQUIREMENTS[milestone])


def task_requirement_links(milestone: str, number: int) -> list[dict[str, str]]:
    links = [
        {"requirement_id": requirement_id, "relation": "SUPPORT"}
        for requirement_id in task_requirements(milestone, number)
    ]
    file_restore = "REQ-T1-FILE-RESTORE-001"
    if milestone == "M03" and number in {15, 16, 17, 18}:
        for link in links:
            if link["requirement_id"] == file_restore:
                link["relation"] = "DEPENDENCY"
    if milestone == "M09":
        relation_by_task = {
            1: "DIRECT_DELIVERY", 9: "DIRECT_DELIVERY", 10: "DIRECT_DELIVERY",
            11: "DIRECT_DELIVERY", 23: "VERIFICATION", 24: "AGGREGATE",
        }
        if number in relation_by_task:
            for link in links:
                if link["requirement_id"] == file_restore:
                    link["relation"] = relation_by_task[number]
        elif number in {2, 3, 4, 5}:
            links.append({"requirement_id": file_restore, "relation": "AFFECTED"})
    return links


def primary_identity(task_id: str, milestone: str, number: int) -> tuple[str, str]:
    if milestone == "M00":
        return "requirement", MILESTONE_REQUIREMENTS[milestone][0]
    if milestone == "M05":
        return "release", "topic1-system-v0.5-midterm"
    if milestone == "M11":
        return "external_gate", "REQ-T1-QUAL-001"
    if milestone == "M12":
        return "release", "topic1-contract-min-v1.0"
    if milestone == "M13" and number <= 10:
        return "technical", f"T1-REMEDIATION-{number:03d}"
    if milestone == "M13":
        return "technical", f"T1-ENGINEERING-{number:03d}"
    return "technical", task_id


def claim_templates(milestone: str, task_id: str) -> tuple[str, str]:
    allowed = (
        "candidate=<hash>; profile=<profile>; environment=<environment>; "
        "time_window=<window>; capability=<bounded task result>"
    )
    forbidden = "No claim beyond this task, its exact candidate/environment/window, or required gates"
    if milestone == "M00":
        return "requirements/source/boundary design only", "No runtime capability is implemented or verified"
    if milestone == "M04":
        return (
            "known attacks; alert accuracy under the pre-signed midterm method >=50%; exact candidate/dataset/environment",
            "No unknown-attack, final 95%/<5%, CNAS, or production-completion claim",
        )
    if milestone == "M11":
        return (
            "external CNAS result only when signed method, scope, candidate, raw outputs and attestation all match",
            "Repository authors cannot manufacture, repair, overwrite, or self-attest external evidence",
        )
    if milestone == "M12":
        return (
            "topic-one contract-scope integrated system for the exact candidate/profile/environment/window",
            "No global G8, five-topic project completion, or internal-strengthening claim",
        )
    if milestone == "M13" and int(task_id[-3:]) >= 11:
        return allowed, "Internal strengthening is not a task-book KPI and does not replace CNAS"
    return allowed, forbidden


def parse_tasks(lines: list[str]) -> tuple[list[dict[str, Any]], dict[str, str]]:
    rows = []
    raw_pr_cells: dict[str, str] = {}
    for line in lines:
        match = TASK_ROW_RE.match(line)
        if not match:
            continue
        task_id = match.group(1)
        milestone = f"M{match.group('milestone')}"
        number = int(match.group("number"))
        pr_cell = match.group("pr").strip()
        raw_pr_cells[task_id] = pr_cell
        refs = sorted(set(CANONICAL_REF_RE.findall(line)))
        external = []
        for activity_type in EXTERNAL_ACTIVITY_RE.findall(pr_cell):
            external.append(
                {
                    "activity_id": f"EXT-{task_id}-{activity_type}",
                    "activity_type": activity_type,
                    "status": "PENDING",
                    "authority": (
                        "independent blind-dataset custodian separated from model/runtime operators"
                        if activity_type == "CUSTODY"
                        else
                        "CNAS-accredited third-party laboratory with valid accreditation "
                        "whose recognized scope covers the test object and pre-signed method"
                        if milestone == "M11"
                        else "independent authorized approver set"
                    ),
                    "mutable_by_repository_authors": False,
                    "depends_on_prs": [],
                    "depends_on_external_activities": [],
                    "run_id": None,
                    "instance_id": None,
                    "candidate_manifest_sha256": None,
                    "profile_id": milestone_profile(milestone),
                    "required_inputs": [
                        {"artifact_id": artifact_id, "sha256": None}
                        for artifact_id in EXTERNAL_ACTIVITY_INPUTS[activity_type]
                    ],
                    "receipt_artifact": f"doc/02_acceptance/external/{task_id.lower()}-<run>/receipt.json",
                    "receipt_sha256": None,
                    "receipt_schema": EXTERNAL_RECEIPT_SCHEMA_PATH.relative_to(REPO_ROOT).as_posix(),
                    "receipt_required_fields": EXTERNAL_ACTIVITY_RECEIPT_FIELDS[activity_type],
                    "signature_required": True,
                    "failure_route": "BLOCK current task; preserve immutable failure receipt; return to accountable milestone",
                }
            )
        primary_kind, primary_id = primary_identity(task_id, milestone, number)
        allowed_claim, forbidden_claim = claim_templates(milestone, task_id)
        resolved_targets = resolve_targets(match.group("paths"))
        path_statuses = {target["resolution_status"] for target in resolved_targets}
        path_blocker = any(
            status in {"UNRESOLVED", "LOGICAL_ONLY", "PLANNED", "AMBIGUOUS_CANDIDATES"}
            for status in path_statuses
        )
        rows.append(
            {
                "task_id": task_id,
                "milestone_id": f"T1-{milestone}",
                "status": "DRAFT",
                "track": (
                    "remediation" if milestone == "M13" and number <= 10
                    else "engineering_strict" if milestone == "M13"
                    else "contract_delivery"
                ),
                "primary_kind": primary_kind,
                "primary_id": primary_id,
                "accountable_milestone": f"T1-{milestone}",
                "accountable_ids": [],
                "requirement_ids": sorted(
                    {item["requirement_id"] for item in task_requirement_links(milestone, number)}
                ),
                "requirement_links": task_requirement_links(milestone, number),
                "secondary_ids": [],
                "affected_ids": refs,
                "depends_on_tasks": [],
                "target_hints": [target["value"] for target in resolved_targets],
                "resolved_targets": resolved_targets,
                "pr_sequence": [],
                "external_activities": external,
                "action": match.group("action").strip(),
                "verification_and_rollback": match.group("verify").strip(),
                "allowed_claim_template": allowed_claim,
                "forbidden_claim_template": forbidden_claim,
                "responsibility": {
                    "owner_role": MILESTONE_OWNER_ROLES[milestone],
                    "owner": None,
                    "reviewers": [],
                    "approvers": [],
                },
                "readiness_blockers": [
                    "owner/reviewer/approver unresolved",
                    *( ["exact symbols and compatibility entrypoints unresolved"] if path_blocker else [] ),
                    *( ["external activity input hashes and receipt run identity unresolved"] if external else [] ),
                    "clean implementation candidate not frozen",
                ],
            }
        )
    return rows, raw_pr_cells


def task_key(milestone: str, number: int) -> str:
    return f"T1-{milestone}-N{number:03d}"


def add_atomic_prs(tasks: list[dict[str, Any]], raw_pr_cells: dict[str, str]) -> None:
    by_milestone: dict[str, list[dict[str, Any]]] = {}
    for task in tasks:
        by_milestone.setdefault(task["milestone_id"], []).append(task)
    for milestone_id, group in sorted(by_milestone.items()):
        counter = 1
        for task in sorted(group, key=lambda item: item["task_id"]):
            task_id = task["task_id"]
            if task_id in PR_TYPE_OVERRIDES:
                types = PR_TYPE_OVERRIDES[task_id]
            else:
                types = normalize_pr_types(raw_pr_cells[task_id])
            phases = PHASE_OVERRIDES.get(task_id, [])
            for step, pr_type in enumerate(types, start=1):
                phase = phases[step - 1] if step <= len(phases) else f"step-{step}"
                candidate_paths = PHASE_PATH_OVERRIDES.get(task_id, {}).get(
                    phase,
                    [
                        path
                        for target in task["resolved_targets"]
                        for path in target["candidate_paths"]
                    ],
                )
                slug = f"n{int(task_id[-3:]):03d}-s{step}"
                pr_id = f"{milestone_id}-P{counter:03d}-{pr_type}-{slug}"
                if pr_type == "IDX":
                    candidate_paths = list(candidate_paths) + [
                        "contracts/alignment/evidence-index.json",
                        f"doc/02_acceptance/topic1/{pr_id.lower()}/evidence-index.json",
                    ]
                elif pr_type == "PROM":
                    candidate_paths = [
                        f"contracts/releases/topic1/{milestone_id.lower()}-release-pointer.json",
                    ]
                proves, does_not_prove = proof_boundary(pr_type)
                task["pr_sequence"].append(
                    {
                        "pr_id": pr_id,
                        "pr_type": pr_type,
                        "status": "DRAFT",
                        "primary_id": task["primary_id"],
                        "depends_on_prs": [],
                        "depends_on_external_activities": [],
                        "required_gates": required_gates(task_id, pr_type),
                        "required_profiles": TASK_REQUIRED_PROFILES.get(task_id, []),
                        "rollback_runbook_id": f"RB-{pr_id}",
                        "default_runtime_state": runtime_state(pr_type),
                        "phase": phase,
                        "candidate_paths": sorted(set(candidate_paths)),
                        "selected_targets": [],
                        "allowed_paths": [],
                        "candidate_manifest_path": None,
                        "candidate_manifest_sha256": None,
                        "profile_id": None,
                        "current_idx_manifest_path": None,
                        "current_idx_manifest_sha256": None,
                        "promotion_intent_manifest_path": None,
                        "promotion_intent_manifest_sha256": None,
                        "postmerge_result_manifest_path": None,
                        "postmerge_result_manifest_sha256": None,
                        "evidence_run_bindings": [],
                        "max_handwritten_loc": 800,
                        "max_production_files": 25,
                        "max_expand_migrations": 1 if pr_type == "EXP" else 0,
                        "max_event_or_api_versions": 1 if pr_type == "CTR" else 0,
                        "produces_new_evidence": pr_type in {"TST-PRE", "TST-POST", "OPS"},
                        "proves": proves,
                        "does_not_prove": does_not_prove,
                    }
                )
                counter += 1
            # Every parent task has a dedicated terminal TASK-IDX.  A domain
            # evidence IDX and a task-completion IDX have different subjects;
            # reusing the former leaves the parent leaf/result/rollback set
            # unbound and makes downstream task dependencies ambiguous.
            pr_type = "IDX"
            pr_id = f"{milestone_id}-P{counter:03d}-IDX-n{int(task_id[-3:]):03d}-task-completion"
            proves, does_not_prove = proof_boundary(pr_type)
            task["pr_sequence"].append(
                {
                        "pr_id": pr_id,
                        "pr_type": pr_type,
                        "status": "DRAFT",
                        "primary_id": task["primary_id"],
                        "depends_on_prs": [],
                        "depends_on_external_activities": [],
                        "required_gates": ["G0"],
                        "required_profiles": TASK_REQUIRED_PROFILES.get(task_id, []),
                        "rollback_runbook_id": f"RB-{pr_id}",
                        "default_runtime_state": "no_runtime_change",
                        "phase": "task-completion",
                        "candidate_paths": [
                            "contracts/alignment/task-completion-candidate.schema.json",
                            "contracts/alignment/task-current-evidence-index.schema.json",
                            f"doc/02_acceptance/topic1/tasks/{task_id.lower()}/completion-candidate.json",
                            f"doc/02_acceptance/topic1/tasks/{task_id.lower()}/current-evidence-index.json",
                        ],
                        "selected_targets": [],
                        "allowed_paths": [],
                        "candidate_manifest_path": None,
                        "candidate_manifest_sha256": None,
                        "profile_id": None,
                        "current_idx_manifest_path": None,
                        "current_idx_manifest_sha256": None,
                        "promotion_intent_manifest_path": None,
                        "promotion_intent_manifest_sha256": None,
                        "postmerge_result_manifest_path": None,
                        "postmerge_result_manifest_sha256": None,
                        "evidence_run_bindings": [],
                        "max_handwritten_loc": 800,
                        "max_production_files": 25,
                        "max_expand_migrations": 0,
                        "max_event_or_api_versions": 0,
                        "produces_new_evidence": False,
                        "proves": proves + ["the parent task's exact leaf/result/rollback closure"],
                        "does_not_prove": does_not_prove + ["requirement satisfaction or milestone completion"],
                }
            )
            counter += 1
            task["completion_contract"] = {
                "schema_path": "contracts/alignment/task-completion-candidate.schema.json",
                "expected_atomic_pr_ids": [pr["pr_id"] for pr in task["pr_sequence"][:-1]],
                "terminal_task_idx_pr_id": task["pr_sequence"][-1]["pr_id"],
                "external_activity_ids": [
                    activity["activity_id"] for activity in task["external_activities"]
                ],
                "required_rollback_runbook_ids": [
                    pr["rollback_runbook_id"] for pr in task["pr_sequence"][:-1]
                ],
                "status": "PENDING",
            }


def add_non_reusing_m06_n004_development_train(tasks: list[dict[str, Any]]) -> None:
    """Add the N004 closure leaves without changing any legacy atomic PR ID.

    Existing execution/evidence artifacts already name M06-P007 and M06-P008.
    The development-readiness deepening therefore uses the permanently
    disjoint P901+ epoch.  P007 keeps its original identity and P008 remains
    the unique terminal TASK-IDX; only their explicit dependency sets change.
    """
    task = next(item for item in tasks if item["task_id"] == "T1-M06-N004")
    by_id = {item["pr_id"]: item for item in task["pr_sequence"]}
    authority = by_id["T1-M06-P007-WRT-n004-s1"]
    terminal = by_id["T1-M06-P008-IDX-n004-task-completion"]
    upstream = list(authority["depends_on_prs"])

    specs = [
        (
            "T1-M06-P901-CTR-n004-source-precedence-contract", "CTR",
            "source-precedence-contract", [*upstream],
        ),
        (
            "T1-M06-P915-REF-n004-source-precedence-validator", "REF",
            "source-precedence-validator", ["T1-M06-P901-CTR-n004-source-precedence-contract"],
        ),
        (
            "T1-M06-P916-TST-PRE-n004-source-precedence-verification", "TST-PRE",
            "source-precedence-verification", ["T1-M06-P915-REF-n004-source-precedence-validator"],
        ),
        (
            "T1-M06-P917-IDX-n004-source-precedence-approval", "IDX",
            "source-precedence-approval", [
                "T1-M06-P916-TST-PRE-n004-source-precedence-verification",
                "T1-M01-P048-IDX-n010-task-completion",
            ],
        ),
        (
            "T1-M06-P902-WRT-n004-http-commit-unknown-mapping", "WRT",
            "http-commit-unknown-mapping", [authority["pr_id"]],
        ),
        (
            "T1-M06-P909-REF-n004-http-commit-unknown-test-fixture", "REF",
            "http-commit-unknown-test-fixture", ["T1-M06-P902-WRT-n004-http-commit-unknown-mapping"],
        ),
        (
            "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification", "TST-PRE",
            "http-commit-unknown-verification", ["T1-M06-P909-REF-n004-http-commit-unknown-test-fixture"],
        ),
        (
            "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping", "WRT",
            "grpc-commit-unknown-mapping", ["T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification"],
        ),
        (
            "T1-M06-P911-REF-n004-grpc-commit-unknown-test-fixture", "REF",
            "grpc-commit-unknown-test-fixture", ["T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping"],
        ),
        (
            "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification", "TST-PRE",
            "grpc-commit-unknown-verification", ["T1-M06-P911-REF-n004-grpc-commit-unknown-test-fixture"],
        ),
        (
            "T1-M06-P904-WRT-n004-asset-event-topic-rail", "WRT",
            "asset-event-topic-rail", ["T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification"],
        ),
        (
            "T1-M06-P913-REF-n004-asset-event-topic-rail-test-fixture", "REF",
            "asset-event-topic-rail-test-fixture", ["T1-M06-P904-WRT-n004-asset-event-topic-rail"],
        ),
        (
            "T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification", "TST-PRE",
            "asset-event-topic-rail-verification", ["T1-M06-P913-REF-n004-asset-event-topic-rail-test-fixture"],
        ),
        (
            "T1-M06-P905-REF-n004-authority-transaction-test-fixture", "REF",
            "authority-transaction-test-fixture", ["T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification"],
        ),
        (
            "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix", "TST-PRE",
            "authority-transaction-fault-matrix", ["T1-M06-P905-REF-n004-authority-transaction-test-fixture"],
        ),
        (
            "T1-M06-P907-REF-n004-asset-event-real-broker-fixture", "REF",
            "asset-event-real-broker-fixture", ["T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix"],
        ),
        (
            "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack", "TST-PRE",
            "asset-event-real-broker-ack", ["T1-M06-P907-REF-n004-asset-event-real-broker-fixture"],
        ),
        (
            "T1-M06-P918-REF-n004-asset-authority-live-reconcile-runner", "REF",
            "asset-authority-live-reconcile-runner", ["T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack"],
        ),
        (
            "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile", "TST-POST",
            "asset-authority-live-reconcile", ["T1-M06-P918-REF-n004-asset-authority-live-reconcile-runner"],
        ),
    ]

    def make_leaf(pr_id: str, pr_type: str, phase: str, dependencies: list[str]) -> dict[str, Any]:
        proves, does_not_prove = proof_boundary(pr_type)
        return {
            "pr_id": pr_id,
            "pr_type": pr_type,
            "status": "DRAFT",
            "primary_id": task["primary_id"],
            "depends_on_prs": dependencies,
            "depends_on_external_activities": [],
            "required_gates": required_gates(task["task_id"], pr_type),
            "required_profiles": TASK_REQUIRED_PROFILES.get(task["task_id"], []),
            "rollback_runbook_id": f"RB-{pr_id}",
            "default_runtime_state": runtime_state(pr_type),
            "phase": phase,
            "candidate_paths": sorted(set(PHASE_PATH_OVERRIDES[task["task_id"]][phase])),
            "selected_targets": [],
            "allowed_paths": [],
            "candidate_manifest_path": None,
            "candidate_manifest_sha256": None,
            "profile_id": None,
            "current_idx_manifest_path": None,
            "current_idx_manifest_sha256": None,
            "promotion_intent_manifest_path": None,
            "promotion_intent_manifest_sha256": None,
            "postmerge_result_manifest_path": None,
            "postmerge_result_manifest_sha256": None,
            "evidence_run_bindings": [],
            "max_handwritten_loc": 400,
            "max_production_files": 1,
            "max_expand_migrations": 0,
            "max_event_or_api_versions": 1 if pr_type == "CTR" else 0,
            "produces_new_evidence": pr_type in {"TST-PRE", "TST-POST"},
            "proves": proves,
            "does_not_prove": does_not_prove,
        }

    source_contract = make_leaf(*specs[0])
    source_validator = make_leaf(*specs[1])
    source_verification = make_leaf(*specs[2])
    authority["phase"] = "authority-transaction"
    authority["candidate_paths"] = sorted(set(PHASE_PATH_OVERRIDES[task["task_id"]]["authority-transaction"]))
    # P007 owns exactly one production method in atomic_upsert.go.  A generic
    # 25-file ceiling would contradict the candidate-bound writable exact set
    # and let a claimant expand the implementation beyond the reviewed seam.
    authority["max_handwritten_loc"] = 800
    authority["max_production_files"] = 1
    source_approval = make_leaf(*specs[3])
    source_approval["required_gates"] = ["G0"]
    authority["depends_on_prs"] = [source_approval["pr_id"]]
    leaves = [source_contract, source_validator, source_verification, source_approval, authority]
    leaves.extend(make_leaf(*spec) for spec in specs[4:])
    # Implementation leaves close only their exact after-source/after-test
    # artifact.  Runtime truth is produced by the downstream evidence leaves;
    # assigning G2/G3 to both WRT and TST-POST creates two gate authorities and
    # an impossible backwards dependency from implementation to its tests.
    # Each evidence leaf therefore owns exactly the gate it can prove.
    gate_overrides = {
        "T1-M06-P007-WRT-n004-s1": ["G0"],
        "T1-M06-P902-WRT-n004-http-commit-unknown-mapping": ["G0"],
        "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping": ["G0"],
        "T1-M06-P904-WRT-n004-asset-event-topic-rail": ["G0"],
        "T1-M06-P918-REF-n004-asset-authority-live-reconcile-runner": ["G0"],
        "T1-M06-P916-TST-PRE-n004-source-precedence-verification": ["G0"],
        "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification": ["G0"],
        "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification": ["G0"],
        "T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification": ["G0"],
        "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix": ["G1"],
        "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack": ["G1"],
        "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile": ["G2", "G3"],
    }
    for leaf in leaves:
        if leaf["pr_id"] in gate_overrides:
            leaf["required_gates"] = gate_overrides[leaf["pr_id"]]
    terminal["depends_on_prs"] = [specs[-1][0]]
    task["pr_sequence"] = [*leaves, terminal]


@lru_cache(maxsize=1)
def feature_owner_map() -> dict[str, str]:
    if not FEATURE_REGISTRY_PATH.exists():
        return {}
    payload = json.loads(FEATURE_REGISTRY_PATH.read_text(encoding="utf-8"))
    return {
        item["feature_id"]: item.get("accountable", "unresolved-feature-owner")
        for item in payload.get("features", [])
        if isinstance(item.get("feature_id"), str)
    }


@lru_cache(maxsize=1)
def canonical_existing_paths() -> dict[str, list[str]]:
    """Return exact repository files already attributed to each canonical ID.

    The remediation ledger is historical/current-state input, not acceptance
    evidence for this design.  We only reuse its existing repository paths as
    code-location candidates; hash fragments and missing paths are discarded.
    """
    if not REMEDIATION_LEDGER_PATH.exists():
        return {}
    payload = json.loads(REMEDIATION_LEDGER_PATH.read_text(encoding="utf-8"))
    result: dict[str, list[str]] = {}
    for item in payload.get("items", []):
        canonical_id = item.get("id")
        if not isinstance(canonical_id, str):
            continue
        paths: list[str] = []
        for reference in item.get("evidence", []):
            if not isinstance(reference, str):
                continue
            relative = reference.split("#", 1)[0]
            path = REPO_ROOT / relative
            if path.is_file():
                paths.append(relative)
        result[canonical_id] = sorted(set(paths))
    return result


def add_closure_slice_prs(lines: list[str]) -> list[dict[str, Any]]:
    owners = feature_owner_map()
    slices = canonical_slices(lines)
    for item in slices:
        short_id = item["short_id"]
        number = int(short_id[-2:])
        parent_number = 6 if number <= 9 else 7 if number <= 19 else 8
        item.update(
            {
                "status": "DRAFT",
                "track": "remediation",
                "accountable_parent_task_id": f"T1-M13-N{parent_number:03d}",
                "depends_on_slices": [] if number == 0 else [f"T1-M13-R{number - 1:02d}"],
                "contract_impact": "UNCLASSIFIED",
                "eligibility": "BLOCKED_PENDING_M12_RESIDUAL_CLASSIFICATION",
                "owner_roles": sorted(
                    {
                        owners.get(canonical_id, "platform-technical-owner")
                        for canonical_id in item["canonical_ids"]
                    }
                ),
                "target_hints": CLOSURE_SLICE_PATH_HINTS[short_id],
                "closure_decisions": [
                    {
                        "canonical_id": canonical_id,
                        "status": "PENDING",
                        "evidence_manifest_sha256": None,
                        "decision_authority": None,
                    }
                    for canonical_id in item["canonical_ids"]
                ],
                "pr_sequence": [],
                "readiness_blockers": [
                    "M12 residual classification missing or contract impact not proven absent",
                    "per-PR exact paths/symbols/owner approvals unresolved",
                    "clean implementation candidate not frozen",
                ],
            }
        )
        pr_number = 1
        for canonical_id in item["canonical_ids"]:
            canonical_candidates = ["contracts/alignment/canonical-registry.json"]
            feature_path = f"contracts/alignment/features/{canonical_id}.json"
            if canonical_id.startswith("F-"):
                canonical_candidates.append(feature_path)
            for pr_type in CLOSURE_SLICE_PR_TYPES[short_id]:
                slug = f"{canonical_id.lower()}-{pr_type.lower()}"
                pr_id = f"T1-M13-{short_id}-P{pr_number:03d}-{pr_type}-{slug}"
                proves, does_not_prove = proof_boundary(pr_type)
                candidate_paths = (
                    [
                        "contracts/alignment/evidence-index.json",
                        f"doc/02_acceptance/topic1/m13/{short_id.lower()}/{canonical_id.lower()}/evidence-index.json",
                    ]
                    if pr_type == "IDX"
                    else sorted(set(canonical_candidates + item["target_hints"]))
                )
                item["pr_sequence"].append(
                    {
                        "pr_id": pr_id,
                        "pr_type": pr_type,
                        "status": "DRAFT",
                        "primary_id": canonical_id,
                        "canonical_ids": [canonical_id],
                        "depends_on_prs": [],
                        "depends_on_external_activities": [],
                        "required_gates": gates_for(pr_type),
                        "required_profiles": ["T1_REMEDIATION_PROFILE"],
                        "rollback_runbook_id": f"RB-{pr_id}",
                        "default_runtime_state": runtime_state(pr_type),
                        "phase": f"{short_id.lower()}-{canonical_id.lower()}-{pr_type.lower()}",
                        "candidate_paths": candidate_paths,
                        "selected_targets": [],
                        "allowed_paths": [],
                        "candidate_manifest_path": None,
                        "candidate_manifest_sha256": None,
                        "profile_id": None,
                        "current_idx_manifest_path": None,
                        "current_idx_manifest_sha256": None,
                        "promotion_intent_manifest_path": None,
                        "promotion_intent_manifest_sha256": None,
                        "postmerge_result_manifest_path": None,
                        "postmerge_result_manifest_sha256": None,
                        "evidence_run_bindings": [],
                        "max_handwritten_loc": 800,
                        "max_production_files": 25,
                        "max_expand_migrations": 1 if pr_type == "EXP" else 0,
                        "max_event_or_api_versions": 1 if pr_type == "CTR" else 0,
                        "produces_new_evidence": pr_type in {"TST-PRE", "TST-POST", "OPS"},
                        "proves": proves,
                        "does_not_prove": does_not_prove,
                    }
                )
                pr_number += 1
        for index in range(1, len(item["pr_sequence"])):
            item["pr_sequence"][index]["depends_on_prs"] = [
                item["pr_sequence"][index - 1]["pr_id"]
            ]
    return slices


def wire_closure_slices(tasks: list[dict[str, Any]], slices: list[dict[str, Any]]) -> None:
    by_task = {task["task_id"]: task for task in tasks}
    by_slice = {item["short_id"]: item for item in slices}

    def set_first_dep(item: dict[str, Any], dependency: str) -> None:
        item["pr_sequence"][0]["depends_on_prs"] = [dependency]

    set_first_dep(by_slice["R00"], last_pr(by_task["T1-M13-N005"]))  # type: ignore[arg-type]
    for number in range(1, 10):
        set_first_dep(by_slice[f"R{number:02d}"], by_slice[f"R{number - 1:02d}"]["pr_sequence"][-1]["pr_id"])
    by_task["T1-M13-N006"]["pr_sequence"][0]["depends_on_prs"] = [
        by_slice["R09"]["pr_sequence"][-1]["pr_id"]
    ]

    set_first_dep(by_slice["R10"], last_pr(by_task["T1-M13-N006"]))  # type: ignore[arg-type]
    for number in range(11, 20):
        set_first_dep(by_slice[f"R{number:02d}"], by_slice[f"R{number - 1:02d}"]["pr_sequence"][-1]["pr_id"])
    by_task["T1-M13-N007"]["pr_sequence"][0]["depends_on_prs"] = [
        by_slice["R19"]["pr_sequence"][-1]["pr_id"]
    ]

    set_first_dep(by_slice["R20"], last_pr(by_task["T1-M13-N007"]))  # type: ignore[arg-type]
    for number in range(21, 30):
        set_first_dep(by_slice[f"R{number:02d}"], by_slice[f"R{number - 1:02d}"]["pr_sequence"][-1]["pr_id"])
    by_task["T1-M13-N008"]["pr_sequence"][0]["depends_on_prs"] = [
        by_slice["R29"]["pr_sequence"][-1]["pr_id"]
    ]


def last_pr(task: dict[str, Any]) -> str | None:
    return task["pr_sequence"][-1]["pr_id"] if task["pr_sequence"] else None


def first_pr(task: dict[str, Any]) -> str | None:
    return task["pr_sequence"][0]["pr_id"] if task["pr_sequence"] else None


def first_execution_node(task: dict[str, Any]) -> tuple[str, str] | None:
    if task["external_activities"]:
        return "external", task["external_activities"][0]["activity_id"]
    if task["pr_sequence"]:
        return "pr", task["pr_sequence"][0]["pr_id"]
    return None


def last_execution_node(task: dict[str, Any]) -> tuple[str, str] | None:
    if task["pr_sequence"]:
        return "pr", task["pr_sequence"][-1]["pr_id"]
    if task["external_activities"]:
        return "external", task["external_activities"][-1]["activity_id"]
    return None


def wire_dependencies(tasks: list[dict[str, Any]]) -> None:
    by_id = {task["task_id"]: task for task in tasks}
    milestone_last: dict[str, tuple[str, str]] = {}

    def add_node_dependency(node: dict[str, Any], dependency: tuple[str, str]) -> None:
        kind, node_id = dependency
        key = "depends_on_prs" if kind == "pr" else "depends_on_external_activities"
        node[key] = sorted(set(node[key] + [node_id]))

    def wire_task_nodes(task: dict[str, Any], predecessors: list[tuple[str, str]]) -> None:
        activities = task["external_activities"]
        prs = task["pr_sequence"]
        if activities:
            for predecessor in predecessors:
                add_node_dependency(activities[0], predecessor)
            for index in range(1, len(activities)):
                add_node_dependency(activities[index], ("external", activities[index - 1]["activity_id"]))
            if prs:
                add_node_dependency(prs[0], ("external", activities[-1]["activity_id"]))
        elif prs:
            for predecessor in predecessors:
                add_node_dependency(prs[0], predecessor)
        for index in range(1, len(prs)):
            add_node_dependency(prs[index], ("pr", prs[index - 1]["pr_id"]))

    for milestone in [f"M{index:02d}" for index in range(14)]:
        ordered_ids = [task_key(milestone, number) for number in EXECUTION_ORDER[milestone]]
        previous_task: dict[str, Any] | None = None
        for index, current_id in enumerate(ordered_ids):
            current = by_id[current_id]
            dependency_tasks = []
            if index == 0:
                dependency_tasks.extend(
                    task_key(dep, EXECUTION_ORDER[dep][-1]) for dep in MILESTONE_DEPS[milestone]
                )
            elif previous_task is not None:
                dependency_tasks.append(previous_task["task_id"])
            current["depends_on_tasks"] = dependency_tasks
            predecessors: list[tuple[str, str]] = []
            if previous_task is not None:
                terminal = last_execution_node(previous_task)
                if terminal:
                    predecessors.append(terminal)
            elif index == 0:
                predecessors.extend(milestone_last[dep] for dep in MILESTONE_DEPS[milestone])
            wire_task_nodes(current, predecessors)
            previous_task = current
        final_task = by_id[ordered_ids[-1]]
        terminal = last_execution_node(final_task)
        if not terminal:
            raise ValueError(f"milestone {milestone} has no final execution node")
        milestone_last[milestone] = terminal

    # Interleaved parent tasks cannot be represented by a task-only total order.
    # The atomic PR graph is authoritative for these consumer-first bridges.
    def set_dep(task_id: str, step: int, dependencies: list[str]) -> None:
        node = by_id[task_id]["pr_sequence"][step - 1]
        node["depends_on_prs"] = dependencies
        node["depends_on_external_activities"] = []

    # M06 has two independent event rails. Asset authority publishes asset.events.v2;
    # passive ARP/DHCP binding and device-log publishers are prepared consumer-first.
    m06_n16 = by_id["T1-M06-N016"]["pr_sequence"]
    set_dep("T1-M06-N003", 1, [m06_n16[4]["pr_id"]])
    set_dep("T1-M06-N006", 1, [last_pr(by_id["T1-M06-N005"]), m06_n16[4]["pr_id"]])  # type: ignore[list-item]
    set_dep("T1-M06-N016", 6, [last_pr(by_id["T1-M06-N006"])])  # type: ignore[list-item]
    set_dep("T1-M06-N007", 1, [m06_n16[5]["pr_id"]])

    # M06 producer canary -> ordinary source canary -> producer acceptance.
    m06_n17 = by_id["T1-M06-N017"]["pr_sequence"]
    set_dep("T1-M06-N015", 1, [m06_n17[0]["pr_id"]])
    set_dep("T1-M06-N017", 2, [last_pr(by_id["T1-M06-N015"])])  # type: ignore[list-item]
    set_dep(
        "T1-M06-N014",
        1,
        [m06_n17[1]["pr_id"], m06_n17[4]["pr_id"], m06_n17[6]["pr_id"]],
    )
    by_id["T1-M06-N015"]["depends_on_tasks"] = ["T1-M06-N013", "T1-M06-N016"]
    by_id["T1-M06-N017"]["depends_on_tasks"] = ["T1-M06-N013", "T1-M06-N016"]
    by_id["T1-M06-N014"]["depends_on_tasks"] = ["T1-M06-N015", "T1-M06-N017"]

    # M08 metadata -> consumer ready -> activation publish -> shadow activation.
    m08_n10 = by_id["T1-M08-N010"]["pr_sequence"]
    m08_n11 = by_id["T1-M08-N011"]["pr_sequence"]
    set_dep("T1-M08-N011", 1, [m08_n10[0]["pr_id"]])
    set_dep("T1-M08-N010", 2, [m08_n11[-1]["pr_id"]])
    set_dep("T1-M08-N010", 3, [m08_n10[1]["pr_id"]])
    set_dep("T1-M08-N012", 1, [m08_n10[2]["pr_id"]])
    by_id["T1-M08-N010"]["depends_on_tasks"] = ["T1-M08-N009"]
    by_id["T1-M08-N011"]["depends_on_tasks"] = ["T1-M08-N009"]
    by_id["T1-M08-N012"]["depends_on_tasks"] = ["T1-M08-N010", "T1-M08-N011"]

    # M09 idle worker -> command writer -> worker enable -> UI.
    m09_n10 = by_id["T1-M09-N010"]["pr_sequence"]
    m09_n09 = by_id["T1-M09-N009"]["pr_sequence"]
    set_dep("T1-M09-N009", 1, [m09_n10[0]["pr_id"]])
    set_dep("T1-M09-N010", 2, [m09_n09[-1]["pr_id"]])
    set_dep("T1-M09-N011", 1, [m09_n10[1]["pr_id"]])
    by_id["T1-M09-N009"]["depends_on_tasks"] = ["T1-M09-N008"]
    by_id["T1-M09-N010"]["depends_on_tasks"] = ["T1-M09-N008"]
    by_id["T1-M09-N011"]["depends_on_tasks"] = ["T1-M09-N009", "T1-M09-N010"]

    # M10 observability ready/injection -> canary -> observation verification.
    m10_n12 = by_id["T1-M10-N012"]["pr_sequence"]
    m10_n11 = by_id["T1-M10-N011"]["pr_sequence"]
    set_dep("T1-M10-N011", 1, [m10_n12[1]["pr_id"]])
    set_dep("T1-M10-N012", 3, [m10_n11[-1]["pr_id"]])
    set_dep("T1-M10-N013", 1, [m10_n12[2]["pr_id"]])
    by_id["T1-M10-N011"]["depends_on_tasks"] = ["T1-M10-N010"]
    by_id["T1-M10-N012"]["depends_on_tasks"] = ["T1-M10-N010"]
    by_id["T1-M10-N013"]["depends_on_tasks"] = ["T1-M10-N011", "T1-M10-N012"]

    # M12 promotion directly references both the pre-merge guard and the signed
    # current-profile IDX; a merely historical IDX ancestor is insufficient.
    set_dep(
        "T1-M12-N007",
        1,
        [last_pr(by_id["T1-M12-N003"]), last_pr(by_id["T1-M12-N006"])],  # type: ignore[list-item]
    )

    # The repository IDX remains between custody and execution, but EXECUTE also
    # carries a direct immutable dependency on the custody receipt/output set so
    # a sealed dataset cannot be exchanged behind an IDX-only edge.
    custody_activity = by_id["T1-M11-N007"]["external_activities"][0]
    execute_activity = by_id["T1-M11-N008"]["external_activities"][0]
    add_node_dependency(
        execute_activity, ("external", custody_activity["activity_id"])
    )


def execution_has_ancestor(
    nodes: dict[str, dict[str, Any]], node_id: str, ancestor_id: str
) -> bool:
    pending = list(
        nodes[node_id].get("depends_on_prs", [])
        + nodes[node_id].get("depends_on_external_activities", [])
    )
    seen: set[str] = set()
    while pending:
        current = pending.pop()
        if current == ancestor_id:
            return True
        if current in seen or current not in nodes:
            continue
        seen.add(current)
        pending.extend(nodes[current].get("depends_on_prs", []))
        pending.extend(nodes[current].get("depends_on_external_activities", []))
    return False


def task_completion_dependency_sets(
    task: dict[str, Any], tasks: list[dict[str, Any]],
    nodes: dict[str, dict[str, Any]],
) -> tuple[list[str], list[str]]:
    by_id = {item["task_id"]: item for item in tasks}
    terminal_by_pr = {
        item["pr_sequence"][-1]["pr_id"]: item["task_id"]
        for item in tasks
    }
    terminal_id = task["pr_sequence"][-1]["pr_id"]
    own_ids = {pr["pr_id"] for pr in task["pr_sequence"]}
    direct_external_leaf_dependencies = {
        dependency
        for pr in task["pr_sequence"][:-1]
        for dependency in pr["depends_on_prs"]
        if dependency not in own_ids
    }
    dependency_indexes = {
        dependency
        for dependency in direct_external_leaf_dependencies
        if dependency in terminal_by_pr
    }
    for dependency_task_id in task["depends_on_tasks"]:
        dependency_terminal = by_id[dependency_task_id]["pr_sequence"][-1]["pr_id"]
        if execution_has_ancestor(nodes, terminal_id, dependency_terminal):
            dependency_indexes.add(dependency_terminal)
    interleaved = direct_external_leaf_dependencies - dependency_indexes
    return sorted(dependency_indexes), sorted(interleaved)


def finalize_task_completion_contracts(
    tasks: list[dict[str, Any]], slices: list[dict[str, Any]]
) -> None:
    """Freeze exact parent-task closure metadata after all interleaved edges."""
    by_id = {task["task_id"]: task for task in tasks}
    nodes = execution_nodes(tasks, slices)
    for task in tasks:
        terminal = task["pr_sequence"][-1]
        leaf_prs = task["pr_sequence"][:-1]
        dependency_task_indexes, interleaved_dependencies = (
            task_completion_dependency_sets(task, tasks, nodes)
        )
        task["completion_contract"] = {
            "schema_path": "contracts/alignment/task-completion-candidate.schema.json",
            "current_index_schema_path": "contracts/alignment/task-current-evidence-index.schema.json",
            "expected_atomic_pr_ids": [pr["pr_id"] for pr in leaf_prs],
            "terminal_task_idx_pr_id": terminal["pr_id"],
            "dependency_task_idx_pr_ids": dependency_task_indexes,
            "interleaved_leaf_dependency_pr_ids": interleaved_dependencies,
            "external_activity_ids": [
                activity["activity_id"] for activity in task["external_activities"]
            ],
            "required_rollback_runbook_ids": [
                pr["rollback_runbook_id"] for pr in leaf_prs
                if pr["pr_type"] not in {"IDX", "PROM", "TST-PRE", "TST-POST"}
            ],
            "status": "PENDING",
        }


def execution_nodes(
    tasks: list[dict[str, Any]], slices: list[dict[str, Any]]
) -> dict[str, dict[str, Any]]:
    nodes: dict[str, dict[str, Any]] = {}
    for task in tasks:
        for pr in task["pr_sequence"]:
            nodes[pr["pr_id"]] = {"node_kind": "pr", **pr}
        for activity in task["external_activities"]:
            nodes[activity["activity_id"]] = {"node_kind": "external_activity", **activity}
    for item in slices:
        for pr in item["pr_sequence"]:
            nodes[pr["pr_id"]] = {"node_kind": "pr", **pr}
    return nodes


def assert_acyclic(nodes: dict[str, dict[str, Any]]) -> None:
    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(node_id: str) -> None:
        if node_id in visited:
            return
        if node_id in visiting:
            raise ValueError(f"execution dependency cycle at {node_id}")
        visiting.add(node_id)
        node = nodes[node_id]
        dependencies = node.get("depends_on_prs", []) + node.get("depends_on_external_activities", [])
        for dependency in dependencies:
            visit(dependency)
        visiting.remove(node_id)
        visited.add(node_id)

    for node_id in nodes:
        visit(node_id)


def validate(
    tasks: list[dict[str, Any]], slices: list[dict[str, Any]], raw_pr_cells: dict[str, str], lines: list[str]
) -> dict[str, Any]:
    task_ids = [task["task_id"] for task in tasks]
    if len(tasks) != 212 or len(set(task_ids)) != 212:
        raise ValueError(f"expected 212 unique tasks, got {len(tasks)}/{len(set(task_ids))}")
    for milestone, expected in EXPECTED_COUNTS.items():
        actual = sorted(
            int(task["task_id"][-3:])
            for task in tasks
            if task["milestone_id"] == f"T1-{milestone}"
        )
        if actual != list(range(1, expected + 1)):
            raise ValueError(f"{milestone} task IDs are not contiguous: {actual}")

    pr_list = [pr for task in tasks for pr in task["pr_sequence"]] + [
        pr for item in slices for pr in item["pr_sequence"]
    ]
    pr_by_id = {pr["pr_id"]: pr for pr in pr_list}
    if len(pr_by_id) != len(pr_list):
        raise ValueError("duplicate atomic PR IDs")
    for pr in pr_list:
        if pr["pr_type"] not in VALID_PR_TYPES:
            raise ValueError(f"invalid atomic PR type in {pr['pr_id']}")
        missing = sorted(
            set(pr["depends_on_prs"])
            - set(pr_by_id)
        )
        external_ids = {
            activity["activity_id"] for task in tasks for activity in task["external_activities"]
        }
        missing_external = sorted(set(pr["depends_on_external_activities"]) - external_ids)
        if missing:
            raise ValueError(f"{pr['pr_id']} has missing PR dependencies {missing}")
        if missing_external:
            raise ValueError(f"{pr['pr_id']} has missing external dependencies {missing_external}")
        if pr["pr_type"] == "PROM":
            if not any(pr_by_id[dependency]["pr_type"] == "IDX" for dependency in pr["depends_on_prs"]):
                raise ValueError(f"{pr['pr_id']} must directly depend on a current IDX PR")
            if pr["produces_new_evidence"]:
                raise ValueError(f"{pr['pr_id']} cannot produce new evidence")

    task_id_set = set(task_ids)
    task_by_id = {task["task_id"]: task for task in tasks}
    nodes = execution_nodes(tasks, slices)
    accountable_claims: dict[str, list[str]] = {}
    for task in tasks:
        missing = sorted(set(task["depends_on_tasks"]) - task_id_set)
        if missing:
            raise ValueError(f"{task['task_id']} has missing task dependencies {missing}")
        types = [pr["pr_type"] for pr in task["pr_sequence"]]
        raw_types = normalize_pr_types(raw_pr_cells[task["task_id"]])
        if len(raw_types) > 1 and len(types) < 2:
            raise ValueError(f"compound parent {task['task_id']} was not expanded")
        if task["accountable_milestone"] != task["milestone_id"]:
            raise ValueError(f"{task['task_id']} accountable milestone differs from its owner milestone")
        linked_requirement_ids = sorted(
            {item["requirement_id"] for item in task["requirement_links"]}
        )
        if linked_requirement_ids != task["requirement_ids"]:
            raise ValueError(f"{task['task_id']} requirement links drifted from requirement_ids")
        if len(task["requirement_links"]) != len(
            {(item["requirement_id"], item["relation"]) for item in task["requirement_links"]}
        ):
            raise ValueError(f"{task['task_id']} contains duplicate requirement links")
        if not task["pr_sequence"] or task["pr_sequence"][-1]["pr_type"] != "IDX":
            raise ValueError(f"{task['task_id']} lacks a unique terminal task IDX")
        completion = task["completion_contract"]
        expected_leaf_ids = [pr["pr_id"] for pr in task["pr_sequence"][:-1]]
        expected_dependency_task_indexes, expected_interleaved_dependencies = (
            task_completion_dependency_sets(task, tasks, nodes)
        )
        expected_rollback_runbooks = [
            pr["rollback_runbook_id"] for pr in task["pr_sequence"][:-1]
            if pr["pr_type"] not in {"IDX", "PROM", "TST-PRE", "TST-POST"}
        ]
        if (
            completion["terminal_task_idx_pr_id"] != task["pr_sequence"][-1]["pr_id"]
            or completion["schema_path"]
            != "contracts/alignment/task-completion-candidate.schema.json"
            or completion["current_index_schema_path"]
            != "contracts/alignment/task-current-evidence-index.schema.json"
            or completion["expected_atomic_pr_ids"] != expected_leaf_ids
            or completion["dependency_task_idx_pr_ids"] != expected_dependency_task_indexes
            or completion["interleaved_leaf_dependency_pr_ids"]
            != expected_interleaved_dependencies
            or completion["external_activity_ids"]
            != [activity["activity_id"] for activity in task["external_activities"]]
            or completion["required_rollback_runbook_ids"]
            != expected_rollback_runbooks
        ):
            raise ValueError(f"{task['task_id']} task completion contract is not an exact leaf closure")
        for canonical_id in task["accountable_ids"]:
            if canonical_id not in canonical_ids():
                raise ValueError(f"{task['task_id']} claims unknown canonical ID {canonical_id}")
            accountable_claims.setdefault(canonical_id, []).append(task["task_id"])

    if set(accountable_claims) != set(canonical_ids()) or any(
        len(owners) != 1 for owners in accountable_claims.values()
    ):
        missing = sorted(set(canonical_ids()) - set(accountable_claims))
        duplicates = {
            canonical_id: owners for canonical_id, owners in accountable_claims.items()
            if len(owners) != 1
        }
        raise ValueError(
            f"canonical accountability must be total and unique: missing={missing[:10]} "
            f"duplicates={dict(list(duplicates.items())[:10])}"
        )

    file_restore_links = {
        task["task_id"]: next(
            (
                link["relation"] for link in task["requirement_links"]
                if link["requirement_id"] == "REQ-T1-FILE-RESTORE-001"
            ),
            None,
        )
        for task in tasks
    }
    expected_file_restore_links = {
        "T1-M03-N015": "DEPENDENCY", "T1-M03-N016": "DEPENDENCY",
        "T1-M03-N017": "DEPENDENCY", "T1-M03-N018": "DEPENDENCY",
        "T1-M09-N001": "DIRECT_DELIVERY", "T1-M09-N002": "AFFECTED",
        "T1-M09-N003": "AFFECTED", "T1-M09-N004": "AFFECTED",
        "T1-M09-N005": "AFFECTED", "T1-M09-N009": "DIRECT_DELIVERY",
        "T1-M09-N010": "DIRECT_DELIVERY", "T1-M09-N011": "DIRECT_DELIVERY",
        "T1-M09-N023": "VERIFICATION", "T1-M09-N024": "AGGREGATE",
    }
    if {
        task_id: relation for task_id, relation in file_restore_links.items()
        if relation is not None
    } != expected_file_restore_links:
        raise ValueError("REQ-T1-FILE-RESTORE-001 task-level traceability drifted")

    for node_id, node in nodes.items():
        dependencies = node.get("depends_on_prs", []) + node.get("depends_on_external_activities", [])
        missing = sorted(set(dependencies) - set(nodes))
        if missing:
            raise ValueError(f"{node_id} has missing execution dependencies {missing}")
    assert_acyclic(nodes)
    augmented_nodes = json.loads(json.dumps(nodes))
    for task in tasks:
        terminal_id = task["completion_contract"]["terminal_task_idx_pr_id"]
        terminal = augmented_nodes[terminal_id]
        terminal["depends_on_prs"] = sorted(
            set(terminal.get("depends_on_prs", []))
            | set(task["completion_contract"]["dependency_task_idx_pr_ids"])
            | set(task["completion_contract"]["interleaved_leaf_dependency_pr_ids"])
        )
    assert_acyclic(augmented_nodes)

    def has_idx_ancestor(pr_id: str, seen: set[str] | None = None) -> bool:
        if seen is None:
            seen = set()
        if pr_id in seen:
            return False
        seen.add(pr_id)
        node = nodes[pr_id]
        dependencies = node.get("depends_on_prs", []) + node.get("depends_on_external_activities", [])
        for dependency in dependencies:
            if nodes[dependency].get("pr_type") == "IDX":
                return True
            if has_idx_ancestor(dependency, seen):
                return True
        return False

    for pr in pr_list:
        if pr["pr_type"] == "PROM" and not has_idx_ancestor(pr["pr_id"]):
            raise ValueError(f"{pr['pr_id']} has no IDX dependency ancestor")

    by_task = {task["task_id"]: task for task in tasks}
    m11_attest_id = by_task["T1-M11-N009"]["external_activities"][0]["activity_id"]
    m11_attest_task_idx = by_task["T1-M11-N009"]["pr_sequence"][-1]
    if (
        m11_attest_id not in m11_attest_task_idx["depends_on_external_activities"]
        or m11_attest_task_idx["pr_id"]
        not in by_task["T1-M11-N010"]["pr_sequence"][0]["depends_on_prs"]
    ):
        raise ValueError("M11 evidence intake is not gated by the attested task IDX")
    m12_approval = by_task["T1-M12-N006"]["external_activities"]
    if len(m12_approval) != 1 or m12_approval[0]["activity_type"] != "APPROVAL":
        raise ValueError("M12 signed Go/No-Go approval node is missing")
    if m12_approval[0]["activity_id"] not in by_task["T1-M12-N006"]["pr_sequence"][0]["depends_on_external_activities"]:
        raise ValueError("M12 evidence IDX is not directly gated by signed approval")

    registry_ids = canonical_ids()
    slice_ids = canonical_slice_ids(lines)
    if len(slice_ids) != len(set(slice_ids)):
        raise ValueError("R00-R29 contains duplicate canonical IDs")
    if registry_ids != slice_ids:
        missing = sorted(set(registry_ids) - set(slice_ids))
        extra = sorted(set(slice_ids) - set(registry_ids))
        raise ValueError(f"canonical slice mismatch missing={missing} extra={extra}")
    slice_bytes = ("\n".join(slice_ids) + "\n").encode("utf-8")

    if len(slices) != 30:
        raise ValueError("expected 30 executable closure slices")
    for item in slices:
        if not item["pr_sequence"] or item["pr_sequence"][-1]["pr_type"] != "IDX":
            raise ValueError(f"{item['slice_id']} is not expanded through an IDX leaf")
        if not item["canonical_ids"]:
            raise ValueError(f"{item['slice_id']} has no canonical IDs")
        for canonical_id in item["canonical_ids"]:
            canonical_prs = [
                pr for pr in item["pr_sequence"]
                if pr.get("canonical_ids") == [canonical_id]
            ]
            if not canonical_prs or canonical_prs[-1]["pr_type"] != "IDX":
                raise ValueError(
                    f"{item['slice_id']} canonical {canonical_id} lacks an independent terminal IDX"
                )
            if any(pr["primary_id"] != canonical_id for pr in canonical_prs):
                raise ValueError(
                    f"{item['slice_id']} canonical {canonical_id} has a cross-canonical primary"
                )

    resolution_counts: dict[str, int] = {}
    for task in tasks:
        for target in task["resolved_targets"]:
            status = target["resolution_status"]
            resolution_counts[status] = resolution_counts.get(status, 0) + 1
    fully_unresolved = sum(
        1 for task in tasks if not any(target["exists_in_snapshot"] for target in task["resolved_targets"])
    )

    return {
        "structure_status": "PASS",
        "dor_status": "BLOCKED",
        "candidate_status": "BLOCKED",
        "promotion_status": "BLOCKED",
        "task_ids_unique": True,
        "task_ids_contiguous": True,
        "pr_ids_unique": True,
        "dependencies_exist": True,
        "dependency_graph_acyclic": True,
        "single_pr_type": True,
        "compound_parent_tasks_expanded": True,
        "external_activity_dependencies_complete": True,
        "closure_slices_expanded": True,
        "idx_before_prom": True,
        "canonical_slice_count": len(slice_ids),
        "canonical_slice_unique": len(set(slice_ids)),
        "canonical_slice_matches_registry": True,
        "canonical_slice_sha256": sha256_bytes(slice_bytes),
        "target_resolution_counts": resolution_counts,
        "fully_unresolved_task_count": fully_unresolved,
    }


def validate_milestone_registry(payload: dict[str, Any], task_ids: set[str]) -> None:
    milestones = payload["milestones"]
    milestone_ids = {item["milestone_id"] for item in milestones}
    expected_ids = {f"T1-M{index:02d}" for index in range(14)}
    if len(milestones) != 14 or milestone_ids != expected_ids:
        raise ValueError("milestone registry must contain exactly T1-M00 through T1-M13")
    graph = {item["milestone_id"]: item["depends_on_milestones"] for item in milestones}
    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(milestone_id: str) -> None:
        if milestone_id in visited:
            return
        if milestone_id in visiting:
            raise ValueError(f"milestone dependency cycle at {milestone_id}")
        visiting.add(milestone_id)
        for dependency in graph[milestone_id]:
            if dependency not in graph:
                raise ValueError(f"{milestone_id} has missing milestone dependency {dependency}")
            visit(dependency)
        visiting.remove(milestone_id)
        visited.add(milestone_id)

    for milestone_id in graph:
        visit(milestone_id)
    ordered_tasks = [task for item in milestones for task in item["review_order"]]
    if len(ordered_tasks) != len(set(ordered_tasks)) or set(ordered_tasks) != task_ids:
        raise ValueError("milestone review orders must cover every task exactly once")


def validate_against_schema(instance: Any, schema_path: Path) -> None:
    root = json.loads(schema_path.read_text(encoding="utf-8"))
    known_schema_keywords = {
        "$schema", "$id", "$defs", "$ref", "title", "description",
        "type", "const", "enum", "required", "properties", "additionalProperties",
        "items", "minItems", "maxItems", "uniqueItems", "minLength", "maxLength",
        "pattern", "minimum", "maximum", "allOf", "anyOf", "oneOf", "not",
        "if", "then", "else",
    }

    def resolve_ref(ref: str) -> dict[str, Any]:
        if not ref.startswith("#/"):
            raise ValueError(f"unsupported non-local schema ref {ref}")
        value: Any = root
        for part in ref[2:].split("/"):
            value = value[part.replace("~1", "/").replace("~0", "~")]
        return value

    def matches(value: Any, schema: dict[str, Any], location: str) -> bool:
        try:
            walk(value, schema, location)
            return True
        except ValueError:
            return False

    def walk(value: Any, schema: dict[str, Any], location: str) -> None:
        unknown_keywords = sorted(set(schema) - known_schema_keywords)
        if unknown_keywords:
            raise ValueError(
                f"unsupported schema keywords at {location}: {unknown_keywords}"
            )
        if "$ref" in schema:
            walk(value, resolve_ref(schema["$ref"]), location)
            return
        for child_schema in schema.get("allOf", []):
            walk(value, child_schema, location)
        if "anyOf" in schema and not any(
            matches(value, child_schema, location) for child_schema in schema["anyOf"]
        ):
            raise ValueError(f"schema anyOf mismatch at {location}")
        if "oneOf" in schema and sum(
            1 for child_schema in schema["oneOf"] if matches(value, child_schema, location)
        ) != 1:
            raise ValueError(f"schema oneOf mismatch at {location}")
        if "not" in schema and matches(value, schema["not"], location):
            raise ValueError(f"schema not constraint failed at {location}")
        if "if" in schema:
            branch = "then" if matches(value, schema["if"], location) else "else"
            if branch in schema:
                walk(value, schema[branch], location)
        if "const" in schema and value != schema["const"]:
            raise ValueError(f"schema const mismatch at {location}")
        if "enum" in schema and value not in schema["enum"]:
            raise ValueError(f"schema enum mismatch at {location}: {value!r}")
        expected_type = schema.get("type")
        allowed_types = expected_type if isinstance(expected_type, list) else [expected_type]
        type_ok = expected_type is None or any(
            (
                (item == "object" and isinstance(value, dict))
                or (item == "array" and isinstance(value, list))
                or (item == "string" and isinstance(value, str))
                or (item == "integer" and isinstance(value, int) and not isinstance(value, bool))
                or (item == "number" and isinstance(value, (int, float)) and not isinstance(value, bool))
                or (item == "boolean" and isinstance(value, bool))
                or (item == "null" and value is None)
            )
            for item in allowed_types
        )
        if not type_ok:
            raise ValueError(f"schema type mismatch at {location}: expected {expected_type}")
        if isinstance(value, dict):
            required = set(schema.get("required", []))
            missing = sorted(required - set(value))
            if missing:
                raise ValueError(f"schema missing required fields at {location}: {missing}")
            properties = schema.get("properties", {})
            if schema.get("additionalProperties") is False:
                extra = sorted(set(value) - set(properties))
                if extra:
                    raise ValueError(f"schema extra fields at {location}: {extra}")
            for key, child in value.items():
                if key in properties:
                    walk(child, properties[key], f"{location}.{key}")
        elif isinstance(value, list):
            if len(value) < schema.get("minItems", 0):
                raise ValueError(f"schema minItems failed at {location}")
            if "maxItems" in schema and len(value) > schema["maxItems"]:
                raise ValueError(f"schema maxItems failed at {location}")
            if schema.get("uniqueItems"):
                serialized = [json.dumps(item, ensure_ascii=False, sort_keys=True) for item in value]
                if len(serialized) != len(set(serialized)):
                    raise ValueError(f"schema uniqueItems failed at {location}")
            if "items" in schema:
                for index, child in enumerate(value):
                    walk(child, schema["items"], f"{location}[{index}]")
        elif isinstance(value, str):
            if len(value) < schema.get("minLength", 0):
                raise ValueError(f"schema minLength failed at {location}")
            if "maxLength" in schema and len(value) > schema["maxLength"]:
                raise ValueError(f"schema maxLength failed at {location}")
            if "pattern" in schema and not re.search(schema["pattern"], value):
                raise ValueError(f"schema pattern failed at {location}: {value!r}")
        elif isinstance(value, (int, float)):
            if "minimum" in schema and value < schema["minimum"]:
                raise ValueError(f"schema minimum failed at {location}")
            if "maximum" in schema and value > schema["maximum"]:
                raise ValueError(f"schema maximum failed at {location}")

    walk(instance, root, "$")


def _resolve_repository_file(repo_root: Path, path: str | Path, label: str) -> Path:
    if not repo_root.is_absolute():
        raise ValueError(f"{label} repository root must be absolute")
    canonical_root = repo_root.resolve(strict=True)
    if canonical_root != repo_root or repo_root.is_symlink() or not repo_root.is_dir():
        raise ValueError(f"{label} repository root is not canonical")
    relative = Path(path)
    if relative.is_absolute() or ".." in relative.parts or not relative.parts:
        raise ValueError(f"{label} path is not canonical repo-relative")
    candidate = canonical_root.joinpath(relative)
    try:
        resolved = candidate.resolve(strict=True)
    except OSError as exc:
        raise ValueError(f"{label} is missing") from exc
    if (
        not resolved.is_relative_to(canonical_root)
        or resolved != candidate
        or candidate.is_symlink()
        or not resolved.is_file()
    ):
        raise ValueError(f"{label} escapes repository scope or uses a symlink")
    return resolved


def load_hashed_json_artifact(
    repo_root: Path,
    artifact_path: str,
    expected_sha256: str,
    schema_path: Path | None = None,
) -> dict[str, Any]:
    resolved = _resolve_repository_file(repo_root, artifact_path, "hashed JSON artifact")
    if not re.fullmatch(r"[0-9a-f]{64}", expected_sha256):
        raise ValueError("hashed JSON artifact expected digest is not SHA-256")
    if not resolved.is_file():
        raise ValueError(f"hashed artifact is outside repository scope or missing: {artifact_path}")
    payload_bytes = resolved.read_bytes()
    if sha256_bytes(payload_bytes) != expected_sha256:
        raise ValueError(f"hashed artifact digest mismatch: {artifact_path}")
    payload = json.loads(payload_bytes)
    if not isinstance(payload, dict):
        raise ValueError(f"hashed JSON artifact root is not an object: {artifact_path}")
    if schema_path is not None:
        if not schema_path.is_absolute():
            schema_path = _resolve_repository_file(repo_root, schema_path, "JSON schema")
        elif not schema_path.resolve(strict=True).is_relative_to(repo_root):
            raise ValueError("JSON schema is outside repository scope")
        validate_against_schema(payload, schema_path.resolve(strict=True))
    return payload


def validate_hashed_artifact(
    repo_root: Path, artifact_path: str, expected_sha256: str
) -> None:
    resolved = _resolve_repository_file(repo_root, artifact_path, "hashed artifact")
    if not re.fullmatch(r"[0-9a-f]{64}", expected_sha256):
        raise ValueError("hashed artifact expected digest is not SHA-256")
    if sha256_bytes(resolved.read_bytes()) != expected_sha256:
        raise ValueError(f"hashed artifact digest mismatch: {artifact_path}")


def validate_task_completion_candidate_semantics(
    completion: dict[str, Any], task: dict[str, Any], registry_sha256: str,
    *,
    pr_bindings: dict[str, dict[str, Any]] | None = None,
    external_bindings: dict[str, dict[str, Any]] | None = None,
    tasks_by_id: dict[str, dict[str, Any]] | None = None,
    _validated_task_completions: dict[str, str] | None = None,
) -> None:
    """Validate the exact task leaf/dependency/external/rollback closure."""
    contract = task["completion_contract"]
    if _validated_task_completions is None:
        _validated_task_completions = {}
    completion_sha = sha256_bytes(canonical_json(completion).encode("utf-8"))
    prior_completion_sha = _validated_task_completions.get(task["task_id"])
    if prior_completion_sha is not None:
        if prior_completion_sha != completion_sha:
            raise ValueError(f"{task['task_id']} appears with two task completion candidates")
        return
    _validated_task_completions[task["task_id"]] = completion_sha
    if (
        completion["task_registry_sha256"] != registry_sha256
        or completion["task_definition_sha256"]
        != sha256_bytes(canonical_json(task).encode("utf-8"))
        or completion["completion_contract_sha256"]
        != sha256_bytes(canonical_json(contract).encode("utf-8"))
        or completion["task_id"] != task["task_id"]
        or completion["milestone_id"] != task["milestone_id"]
        or completion["terminal_task_idx_pr_id"] != contract["terminal_task_idx_pr_id"]
    ):
        raise ValueError(f"{task['task_id']} task completion identity mismatch")

    def exact_unique_ids(
        items: list[dict[str, Any]], field: str, expected: list[str], label: str,
    ) -> dict[str, dict[str, Any]]:
        ids = [item[field] for item in items]
        if len(ids) != len(set(ids)) or set(ids) != set(expected):
            raise ValueError(f"{task['task_id']} {label} is not the exact registry set")
        return {item[field]: item for item in items}

    leaves = exact_unique_ids(
        completion["leaf_results"], "pr_id", contract["expected_atomic_pr_ids"],
        "leaf result set",
    )
    dependencies = exact_unique_ids(
        completion["dependency_task_indexes"], "terminal_task_idx_pr_id",
        contract["dependency_task_idx_pr_ids"], "dependency task IDX set",
    )
    interleaved = exact_unique_ids(
        completion["interleaved_leaf_results"], "pr_id",
        contract["interleaved_leaf_dependency_pr_ids"], "interleaved leaf set",
    )
    external = exact_unique_ids(
        completion["external_results"], "activity_id",
        contract["external_activity_ids"], "external activity set",
    )
    rollbacks = exact_unique_ids(
        completion["rollback_coverage"], "runbook_id",
        contract["required_rollback_runbook_ids"], "rollback coverage set",
    )
    identity_records = [
        *leaves.values(), *dependencies.values(), *interleaved.values(), *external.values(),
    ]
    if any(
        item["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
        or item["profile_id"] != completion["profile_id"]
        or (
            "environment_id" in item
            and item["environment_id"] != completion["environment_id"]
        )
        for item in identity_records
    ):
        raise ValueError(f"{task['task_id']} task completion crosses candidate/profile/environment")
    if any(item["status"] != "PASS" for item in [*leaves.values(), *interleaved.values()]):
        raise ValueError(f"{task['task_id']} task completion contains a non-PASS leaf")
    if any(item["result"] != "PASS" for item in rollbacks.values()):
        raise ValueError(f"{task['task_id']} task completion has incomplete rollback coverage")

    leaf_packages: dict[str, tuple[dict[str, Any], dict[str, Any]]] = {}
    for pr_id, item in {**leaves, **interleaved}.items():
        package = load_hashed_json_artifact(REPO_ROOT, 
            item["execution_package"]["path"], item["execution_package"]["sha256"],
            ATOMIC_EXECUTION_PACKAGE_SCHEMA_PATH,
        )
        receipt = load_hashed_json_artifact(REPO_ROOT, 
            item["acceptance_receipt"]["path"], item["acceptance_receipt"]["sha256"],
            EXECUTION_ACCEPTANCE_RECEIPT_SCHEMA_PATH,
        )
        if (
            package["atomic_pr_id"] != pr_id
            or package["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
            or package["profile_id"] != completion["profile_id"]
            or pr_id not in receipt["atomic_pr_ids"]
            or receipt["decision"] != "ACCEPTED_FOR_SCOPED_EXECUTION"
        ):
            raise ValueError(f"{task['task_id']} leaf {pr_id} package/receipt identity mismatch")
        if pr_bindings is not None:
            binding = pr_bindings[pr_id]
            expected_package_ref = {
                "path": binding["execution_package_ref"]["path"],
                "sha256": binding["execution_package_ref"]["sha256"],
            }
            expected_postmerge = (
                None
                if binding["postmerge_result_manifest_path"] is None
                else {
                    "path": binding["postmerge_result_manifest_path"],
                    "sha256": binding["postmerge_result_manifest_sha256"],
                }
            )
            if (
                item["execution_package"] != expected_package_ref
                or item["postmerge_result"] != expected_postmerge
                or binding["readiness_status"] != "PASS"
            ):
                raise ValueError(f"{task['task_id']} leaf {pr_id} differs from its PASS overlay binding")
        if item["postmerge_result"] is not None:
            validate_hashed_artifact(REPO_ROOT, 
                item["postmerge_result"]["path"], item["postmerge_result"]["sha256"]
            )
        leaf_packages[pr_id] = (package, item)

    for terminal_pr_id, item in dependencies.items():
        dependency_index = load_hashed_json_artifact(REPO_ROOT, 
            item["index"]["path"], item["index"]["sha256"], TASK_CURRENT_INDEX_SCHEMA_PATH,
        )
        if (
            dependency_index["task_id"] != item["task_id"]
            or dependency_index["terminal_task_idx_pr_id"] != terminal_pr_id
            or dependency_index["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
            or dependency_index["profile_id"] != completion["profile_id"]
            or dependency_index["environment_id"] != completion["environment_id"]
            or dependency_index["status"] != "PASS"
        ):
            raise ValueError(f"{task['task_id']} dependency task index {terminal_pr_id} is not matching PASS")
        dependency_completion_ref = dependency_index["completion_candidate"]
        dependency_completion = load_hashed_json_artifact(REPO_ROOT, 
            dependency_completion_ref["path"], dependency_completion_ref["sha256"],
            TASK_COMPLETION_SCHEMA_PATH,
        )
        if tasks_by_id is not None:
            dependency_task = tasks_by_id.get(item["task_id"])
            if dependency_task is None:
                raise ValueError(f"{task['task_id']} dependency task definition is missing")
            validate_task_completion_candidate_semantics(
                dependency_completion, dependency_task, registry_sha256,
                pr_bindings=pr_bindings, external_bindings=external_bindings,
                tasks_by_id=tasks_by_id,
                _validated_task_completions=_validated_task_completions,
            )
        validate_task_current_index_semantics(
            dependency_index, dependency_completion, dependency_completion_ref
        )

    for activity_id, item in external.items():
        receipt = load_hashed_json_artifact(REPO_ROOT, 
            item["receipt"]["path"], item["receipt"]["sha256"], EXTERNAL_RECEIPT_SCHEMA_PATH,
        )
        validate_hashed_artifact(REPO_ROOT, 
            item["trusted_verification_receipt"]["path"],
            item["trusted_verification_receipt"]["sha256"],
        )
        if (
            receipt["activity_id"] != activity_id
            or receipt["run_id"] != item["run_id"]
            or receipt["instance_id"] != item["instance_id"]
            or receipt["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
            or receipt["profile_id"] != completion["profile_id"]
            or receipt["result"] != "PASS"
            or receipt["signature_verification"]["status"] != "PASS"
        ):
            raise ValueError(f"{task['task_id']} external activity {activity_id} receipt mismatch")
        if external_bindings is not None:
            binding = external_bindings[activity_id]
            if (
                binding["status"] != "PASS"
                or binding["run_id"] != item["run_id"]
                or binding["instance_id"] != item["instance_id"]
                or binding["receipt_artifact"] != item["receipt"]["path"]
                or binding["receipt_sha256"] != item["receipt"]["sha256"]
            ):
                raise ValueError(f"{task['task_id']} external activity {activity_id} differs from overlay")

    run_ids = [item["run_id"] for item in completion["evidence_runs"]]
    if len(run_ids) != len(set(run_ids)) or any(
        item["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
        or item["profile_id"] != completion["profile_id"]
        or item["environment_id"] != completion["environment_id"]
        or item["result"] != "PASS"
        for item in completion["evidence_runs"]
    ):
        raise ValueError(f"{task['task_id']} task completion evidence runs are not unique PASS runs")
    evidence_manifests: dict[str, dict[str, Any]] = {}
    for item in completion["evidence_runs"]:
        manifest = load_hashed_json_artifact(REPO_ROOT, 
            item["manifest"]["path"], item["manifest"]["sha256"],
            EVIDENCE_RUN_MANIFEST_SCHEMA_PATH,
        )
        comparable_fields = (
            "run_id", "subject_pr_id", "gate_id", "result",
            "candidate_manifest_sha256", "profile_id", "environment_id",
            "execution_package_sha256", "plan_id", "plan_sha256",
        )
        if any(manifest[field] != item[field] for field in comparable_fields):
            raise ValueError(f"{task['task_id']} evidence run {item['run_id']} differs from manifest")
        if item["subject_pr_id"] not in leaf_packages:
            raise ValueError(f"{task['task_id']} evidence run has an unrelated subject PR")
        package, leaf = leaf_packages[item["subject_pr_id"]]
        plan_key_by_purpose = {
            "VERIFICATION": "test", "ROLLBACK_REHEARSAL": "rollback",
            "OBSERVATION": "observation", "RECONCILIATION": "evidence",
            "EXTERNAL_ATTESTATION": "evidence",
        }
        plan_ref = package["plan_refs"][plan_key_by_purpose[manifest["run_purpose"]]]
        if (
            item["execution_package_sha256"] != leaf["execution_package"]["sha256"]
            or plan_ref is None
            or item["plan_id"] != plan_ref["plan_id"]
            or item["plan_sha256"] != plan_ref["sha256"]
        ):
            raise ValueError(f"{task['task_id']} evidence run package/plan origin mismatch")
        evidence_manifests[item["run_id"]] = manifest

    if pr_bindings is not None:
        expected_runs = sorted(
            (
                run["run_id"], run["subject_pr_id"], run["gate_id"],
                run["manifest_path"], run["manifest_sha256"],
            )
            for pr_id in leaf_packages
            for run in pr_bindings[pr_id]["evidence_run_bindings"]
        )
        actual_runs = sorted(
            (
                run["run_id"], run["subject_pr_id"], run["gate_id"],
                run["manifest"]["path"], run["manifest"]["sha256"],
            )
            for run in completion["evidence_runs"]
        )
        if actual_runs != expected_runs:
            raise ValueError(f"{task['task_id']} evidence runs differ from exact leaf overlay closure")

    for runbook_id, item in rollbacks.items():
        if item["pr_id"] not in leaf_packages:
            raise ValueError(f"{task['task_id']} rollback {runbook_id} has an unrelated PR")
        package, _ = leaf_packages[item["pr_id"]]
        rollback_plan = package["plan_refs"]["rollback"]
        if item["coverage_mode"] == "EXECUTED_ROLLBACK":
            if (
                not item["run_id"] or item["run_id"] not in evidence_manifests
                or item["run_manifest"] is None
                or item["plan_id"] != rollback_plan["plan_id"]
                or item["plan_sha256"] != rollback_plan["sha256"]
            ):
                raise ValueError(f"{task['task_id']} rollback {runbook_id} lacks its approved run")
            manifest = evidence_manifests[item["run_id"]]
            if (
                manifest["subject_pr_id"] != item["pr_id"]
                or manifest["run_purpose"] != "ROLLBACK_REHEARSAL"
                or item["run_manifest"] != next(
                    run["manifest"] for run in completion["evidence_runs"]
                    if run["run_id"] == item["run_id"]
                )
            ):
                raise ValueError(f"{task['task_id']} rollback {runbook_id} run identity mismatch")
        elif any(
            item[field] is not None
            for field in ("plan_id", "plan_sha256", "run_id", "run_manifest")
        ):
            raise ValueError(f"{task['task_id']} non-executed rollback {runbook_id} carries a fake run")

    artifact_ids = [item["artifact_id"] for item in completion["output_artifacts"]]
    if len(artifact_ids) != len(set(artifact_ids)):
        raise ValueError(f"{task['task_id']} task completion output artifact IDs are not unique")
    for artifact in completion["output_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, artifact["path"], artifact["sha256"])
    expected_outputs = sorted(
        (
            artifact["direction"], artifact["artifact_id"], artifact["path"],
            artifact["sha256"], artifact["schema_ref"],
        )
        for manifest in evidence_manifests.values()
        for artifact in manifest["artifacts"]
        if artifact["direction"] == "OUTPUT"
    )
    actual_outputs = sorted(
        (
            artifact["direction"], artifact["artifact_id"], artifact["path"],
            artifact["sha256"], artifact["schema_ref"],
        )
        for artifact in completion["output_artifacts"]
    )
    if actual_outputs != expected_outputs:
        raise ValueError(f"{task['task_id']} task completion output artifact closure mismatch")
    if completion["result"] == "PASS" and completion["blockers"]:
        raise ValueError(f"{task['task_id']} PASS task completion retains blockers")


def validate_task_current_index_semantics(
    current: dict[str, Any], completion: dict[str, Any], completion_ref: dict[str, Any],
    execution_acceptance_ref: dict[str, Any] | None = None,
) -> None:
    context = completion["task_id"]
    if (
        current["task_id"] != completion["task_id"]
        or current["milestone_id"] != completion["milestone_id"]
        or current["terminal_task_idx_pr_id"] != completion["terminal_task_idx_pr_id"]
        or current["candidate_manifest_sha256"] != completion["candidate_manifest_sha256"]
        or current["profile_id"] != completion["profile_id"]
        or current["environment_id"] != completion["environment_id"]
        or current["status"] != completion["result"]
        or current["completion_candidate"]["path"] != completion_ref["path"]
        or current["completion_candidate"]["sha256"] != completion_ref["sha256"]
    ):
        raise ValueError(f"{context} task current index identity mismatch")
    expected_dependency_refs = sorted(
        (item["index"]["path"], item["index"]["sha256"])
        for item in completion["dependency_task_indexes"]
    )
    actual_dependency_refs = sorted(
        (item["path"], item["sha256"]) for item in current["dependency_task_indexes"]
    )
    exact_sets = (
        (actual_dependency_refs, expected_dependency_refs, "dependency task indexes"),
        (
            current["leaf_result_sha256s"],
            sorted(
                sha256_bytes(canonical_json(item).encode("utf-8"))
                for item in completion["leaf_results"]
            ),
            "leaf results",
        ),
        (
            current["external_receipt_sha256s"],
            sorted(item["receipt"]["sha256"] for item in completion["external_results"]),
            "external receipts",
        ),
        (
            current["evidence_manifest_sha256s"],
            sorted(item["manifest"]["sha256"] for item in completion["evidence_runs"]),
            "evidence manifests",
        ),
        (
            current["rollback_manifest_sha256s"],
            sorted(
                item["run_manifest"]["sha256"]
                for item in completion["rollback_coverage"]
                if item["run_manifest"] is not None
            ),
            "rollback manifests",
        ),
        (
            current["output_artifact_sha256s"],
            sorted(item["sha256"] for item in completion["output_artifacts"]),
            "output artifacts",
        ),
    )
    for actual, expected, label in exact_sets:
        if actual != expected:
            raise ValueError(f"{context} task current index differs from {label}")
    validate_hashed_artifact(REPO_ROOT, 
        current["task_idx_acceptance_receipt"]["path"],
        current["task_idx_acceptance_receipt"]["sha256"],
    )
    receipt = load_hashed_json_artifact(REPO_ROOT, 
        current["task_idx_acceptance_receipt"]["path"],
        current["task_idx_acceptance_receipt"]["sha256"],
        EXECUTION_ACCEPTANCE_RECEIPT_SCHEMA_PATH,
    )
    if (
        completion["terminal_task_idx_pr_id"] not in receipt["atomic_pr_ids"]
        or receipt["decision"] != "ACCEPTED_FOR_SCOPED_EXECUTION"
        or (
            execution_acceptance_ref is not None
            and current["task_idx_acceptance_receipt"] != execution_acceptance_ref
        )
    ):
        raise ValueError(f"{context} task current index uses a different execution receipt")


REQUIRED_CANDIDATE_SOURCE_ROOTS = {
    "go/control-plane", "java/flink-jobs", "rust/probe-agent", "web/ui",
    "proto/traffic/v1", "common", "deployments", "mlops", "contracts",
}


def resolve_candidate_repository(repo_root: Path) -> CandidateRepository:
    if not repo_root.is_absolute():
        raise ValueError("candidate repository root must be absolute")
    try:
        resolved = repo_root.resolve(strict=True)
    except OSError as exc:
        raise ValueError("candidate repository root does not exist") from exc
    if resolved != repo_root or repo_root.is_symlink() or not repo_root.is_dir():
        raise ValueError("candidate repository root must be canonical and non-symlinked")
    top_level = subprocess.run(
        ["git", "-C", str(resolved), "rev-parse", "--show-toplevel"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    git_dir = subprocess.run(
        ["git", "-C", str(resolved), "rev-parse", "--absolute-git-dir"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if top_level.returncode != 0 or git_dir.returncode != 0:
        diagnostic = (top_level.stderr + git_dir.stderr).decode(
            "utf-8", errors="replace"
        ).strip()[:512]
        raise ValueError(f"candidate repository Git discovery failed: {diagnostic}")
    try:
        discovered_root = Path(top_level.stdout.decode("utf-8").strip()).resolve(strict=True)
        discovered_git_dir = Path(git_dir.stdout.decode("utf-8").strip()).resolve(strict=True)
    except (OSError, UnicodeDecodeError) as exc:
        raise ValueError("candidate repository returned invalid canonical metadata") from exc
    if discovered_root != resolved or not discovered_git_dir.is_dir():
        raise ValueError("candidate repository is nested or differs from the Git top-level")
    identity = sha256_bytes(
        canonical_json(
            {"schema_version": "1.0.0", "root": str(resolved), "git_dir": str(discovered_git_dir)}
        ).encode("utf-8")
    )
    return CandidateRepository(resolved, discovered_git_dir, identity)


def candidate_tree_fingerprint(
    repo_root: Path,
    candidate_commit: str,
    source_roots: list[str],
    excluded_paths: list[dict[str, Any]],
) -> str:
    repository = resolve_candidate_repository(repo_root)
    if not re.fullmatch(r"(?:[0-9a-f]{40}|[0-9a-f]{64})", candidate_commit):
        raise ValueError("candidate commit is not a full immutable object ID")
    if set(source_roots) != REQUIRED_CANDIDATE_SOURCE_ROOTS:
        raise ValueError("candidate source roots do not exactly cover the production repository surface")
    exclusion_values = [item["path"] for item in excluded_paths]
    if len(exclusion_values) != len(set(exclusion_values)):
        raise ValueError("candidate repeats an excluded path")
    for path in source_roots + exclusion_values:
        if path.startswith("/") or ".." in Path(path).parts or path.endswith("/"):
            raise ValueError("candidate source/exclusion path is not canonical repo-relative")
    for exclusion in excluded_paths:
        path = exclusion["path"]
        if not exclusion["referenced_by_active_build"]:
            raise ValueError("inactive paths are explanatory metadata, not fingerprint exclusions")
        if path in REQUIRED_CANDIDATE_SOURCE_ROOTS or not any(
            path.startswith(root + "/") for root in REQUIRED_CANDIDATE_SOURCE_ROOTS
        ):
            raise ValueError("candidate exclusion is a production root or outside approved roots")
        object_type = subprocess.run(
            ["git", "cat-file", "-t", f"{candidate_commit}:{path}"],
            cwd=repository.root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )
        if object_type.returncode == 0:
            raise ValueError(
                "tracked candidate blobs/directories cannot be removed from the production fingerprint"
            )
    result = subprocess.run(
        ["git", "ls-tree", "-r", "-z", "--full-tree", candidate_commit, "--", *source_roots],
        cwd=repository.root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if result.returncode != 0:
        raise ValueError(
            "candidate source tree cannot be enumerated: "
            + result.stderr.decode("utf-8", errors="replace").strip()
        )
    entries: list[dict[str, str]] = []
    for record in result.stdout.split(b"\0"):
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        mode, object_type, _git_object_id = metadata.decode("ascii").split(" ")
        path = raw_path.decode("utf-8")
        if object_type != "blob" or any(
            path == excluded or path.startswith(excluded + "/")
            for excluded in exclusion_values
        ):
            continue
        blob = read_candidate_blob(repository.root, candidate_commit, path)
        if blob is None:
            raise ValueError(f"candidate tree entry disappeared: {path}")
        entries.append({"path": path, "mode": mode, "blob_sha256": sha256_bytes(blob)})
    if not entries:
        raise ValueError("candidate production tree fingerprint is empty")
    return sha256_bytes(canonical_json(sorted(entries, key=lambda item: item["path"])).encode("utf-8"))


def validate_candidate_artifact_refs(
    refs: list[dict[str, Any]], expected_hashes: list[str], candidate_commit: str,
    context: str,
) -> None:
    ids = [item["artifact_id"] for item in refs]
    paths = [item["path"] for item in refs]
    if (
        len(ids) != len(set(ids))
        or len(paths) != len(set(paths))
        or len(refs) != len(expected_hashes)
        or len({item["sha256"] for item in refs}) != len(refs)
        or {item["sha256"] for item in refs} != set(expected_hashes)
    ):
        raise ValueError(f"{context} artifact identity/path/hash set is not exact")
    for item in refs:
        if item["source_kind"] == "CANDIDATE_GIT_BLOB":
            if (
                item["provenance_receipt_path"] is not None
                or item["provenance_receipt_sha256"] is not None
            ):
                raise ValueError(f"{context} Git artifact unexpectedly carries an external receipt")
            blob = read_candidate_blob(REPO_ROOT, candidate_commit, item["path"])
            if blob is None or sha256_bytes(blob) != item["sha256"]:
                raise ValueError(f"{context} artifact is not the declared candidate Git blob")
            continue
        if (
            item["source_kind"] != "TRUSTED_EXTERNAL_ARTIFACT"
            or item["provenance_receipt_path"] is None
            or item["provenance_receipt_sha256"] is None
        ):
            raise ValueError(f"{context} external artifact lacks a provenance receipt")
        validate_hashed_artifact(REPO_ROOT, item["path"], item["sha256"])
        receipt = load_hashed_json_artifact(REPO_ROOT, 
            item["provenance_receipt_path"], item["provenance_receipt_sha256"],
            CANDIDATE_ARTIFACT_PROVENANCE_RECEIPT_SCHEMA_PATH,
        )
        signed_core = {
            key: value for key, value in receipt.items()
            if key not in {
                "signed_payload_artifact", "signed_payload_sha256",
                "signature_artifacts", "verification",
            }
        }
        signed_core_sha256 = sha256_bytes(canonical_json(signed_core).encode("utf-8"))
        if (
            receipt["artifact_id"] != item["artifact_id"]
            or receipt["artifact_role"] != item["artifact_role"]
            or receipt["artifact_path"] != item["path"]
            or receipt["artifact_sha256"] != item["sha256"]
            or receipt["candidate_commit"] != candidate_commit
            or receipt["signed_payload_sha256"] != signed_core_sha256
            or receipt["verification"]["status"] != "PASS"
        ):
            raise ValueError(f"{context} external artifact provenance identity mismatch")
        validate_hashed_artifact(REPO_ROOT, 
            receipt["signed_payload_artifact"], receipt["signed_payload_sha256"]
        )
        for signature in receipt["signature_artifacts"]:
            validate_hashed_artifact(REPO_ROOT, signature["path"], signature["sha256"])
        require_trusted_signature_verifier(f"{context} external artifact provenance")


def validate_implementation_candidate(candidate: dict[str, Any], context: str) -> None:
    commit = candidate["implementation_candidate_commit"]
    commit_check = subprocess.run(
        ["git", "cat-file", "-e", f"{commit}^{{commit}}"], cwd=REPO_ROOT,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if commit_check.returncode != 0:
        raise ValueError(f"{context} implementation candidate commit does not exist")
    if candidate["production_tree_content_sha256"] != candidate_tree_fingerprint(REPO_ROOT, 
        commit, candidate["source_roots"], candidate["excluded_paths"]
    ):
        raise ValueError(f"{context} production tree fingerprint was not recomputed from Git")
    validate_candidate_artifact_refs(
        candidate["config_schema_migration_artifacts"],
        candidate["config_schema_migration_hashes"], commit,
        f"{context} config/schema/migration",
    )
    validate_candidate_artifact_refs(
        candidate["model_threshold_dataset_artifacts"],
        candidate["model_threshold_dataset_hashes"], commit,
        f"{context} model/threshold/dataset",
    )
    validate_candidate_artifact_refs(
        candidate["supply_chain_artifacts"],
        candidate["supply_chain_artifact_hashes"], commit, f"{context} supply-chain",
    )
    validate_candidate_artifact_refs(
        candidate["runtime_artifacts"],
        candidate["runtime_artifact_hashes"], commit, f"{context} runtime",
    )
    image_digests = [item["image_digest"] for item in candidate["image_attestations"]]
    if len(image_digests) != len(set(image_digests)) or set(image_digests) != set(
        candidate["image_digests"]
    ):
        raise ValueError(f"{context} image digest/attestation set is not exact")
    for image in candidate["image_attestations"]:
        if image["deployed_image_digest"] != image["image_digest"]:
            raise ValueError(f"{context} deployed image is not the attested immutable digest")
        manifest_blob = read_candidate_blob(REPO_ROOT, commit, image["manifest_path"])
        manifest_ref = next(
            (
                item for item in candidate["config_schema_migration_artifacts"]
                if item["path"] == image["manifest_path"]
                and item["sha256"] == image["manifest_sha256"]
                and item["source_kind"] == "CANDIDATE_GIT_BLOB"
            ),
            None,
        )
        if (
            manifest_ref is None
            or manifest_blob is None
            or sha256_bytes(manifest_blob) != image["manifest_sha256"]
        ):
            raise ValueError(f"{context} image manifest is not a candidate Git blob")
        supply_chain_by_path = {
            item["path"]: item for item in candidate["supply_chain_artifacts"]
        }
        attestation_ref = supply_chain_by_path.get(image["attestation_path"])
        if (
            attestation_ref is None
            or attestation_ref["sha256"] != image["attestation_sha256"]
            or attestation_ref["source_kind"] != "TRUSTED_EXTERNAL_ARTIFACT"
            or attestation_ref["artifact_role"]
            != f"image-provenance-attestation:{image['image_digest']}"
            or image["attestation_sha256"] not in candidate["supply_chain_artifact_hashes"]
        ):
            raise ValueError(f"{context} image attestation is outside supply-chain closure")
    excluded_active_paths = {
        item["path"] for item in candidate["excluded_paths"]
        if item["referenced_by_active_build"]
    }
    prebuilt_path_values = [
        item["path"] for item in candidate["external_or_prebuilt_artifacts"]
    ]
    prebuilt_paths = set(prebuilt_path_values)
    if len(prebuilt_path_values) != len(prebuilt_paths):
        raise ValueError(f"{context} repeats a prebuilt path/provenance identity")
    if excluded_active_paths != prebuilt_paths:
        raise ValueError(f"{context} active excluded artifact/prebuilt set is not exact")
    image_attestation_map = {
        item["image_digest"]: item for item in candidate["image_attestations"]
    }
    for item in candidate["external_or_prebuilt_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, item["path"], item["binary_sha256"])
        recipe_blob = read_candidate_blob(REPO_ROOT, commit, item["build_recipe_path"])
        if recipe_blob is None or sha256_bytes(recipe_blob) != item["build_recipe_sha256"]:
            raise ValueError(f"{context} prebuilt build recipe is not a candidate Git blob")
        validate_hashed_artifact(REPO_ROOT, 
            item["sbom_or_attestation_path"], item["sbom_or_attestation_sha256"]
        )
        image = image_attestation_map.get(item["image_digest"])
        prebuilt_provenance = next(
            (
                artifact for artifact in candidate["supply_chain_artifacts"]
                if artifact["path"] == item["sbom_or_attestation_path"]
                and artifact["sha256"] == item["sbom_or_attestation_sha256"]
                and artifact["artifact_role"]
                == (
                    "prebuilt-binary-provenance:"
                    f"{item['image_digest']}:{item['binary_sha256']}"
                )
                and artifact["source_kind"] == "TRUSTED_EXTERNAL_ARTIFACT"
            ),
            None,
        )
        prebuilt_receipt = (
            load_hashed_json_artifact(REPO_ROOT, 
                prebuilt_provenance["provenance_receipt_path"],
                prebuilt_provenance["provenance_receipt_sha256"],
                CANDIDATE_ARTIFACT_PROVENANCE_RECEIPT_SCHEMA_PATH,
            )
            if prebuilt_provenance is not None
            else None
        )
        if (
            image is None
            or prebuilt_provenance is None
            or prebuilt_receipt is None
            or prebuilt_receipt["source_or_builder_sha"] != item["source_or_builder_sha"]
            or prebuilt_receipt["recipe_or_toolchain"] != item["recipe_or_toolchain"]
            or item["deployed_image_digest"] != image["deployed_image_digest"]
            or item["image_internal_binary_sha256"] != item["binary_sha256"]
            or item["sbom_or_attestation_sha256"] not in candidate["supply_chain_artifact_hashes"]
        ):
            raise ValueError(f"{context} prebuilt binary/image/SBOM provenance mismatch")
    delivery_ids = [item["artifact_id"] for item in candidate["delivery_artifacts"]]
    delivery_paths = [item["path"] for item in candidate["delivery_artifacts"]]
    if len(delivery_ids) != len(set(delivery_ids)) or len(delivery_paths) != len(set(delivery_paths)):
        raise ValueError(f"{context} delivery roles must use five distinct candidate files")
    for item in candidate["delivery_artifacts"]:
        if not item["path"].startswith(("deployments/", "contracts/releases/", "doc/02_acceptance/")):
            raise ValueError(f"{context} delivery artifact is outside approved release surfaces")
        blob = read_candidate_blob(REPO_ROOT, commit, item["path"])
        if blob is None or sha256_bytes(blob) != item["sha256"]:
            raise ValueError(f"{context} delivery artifact is not a candidate Git blob")
    require_trusted_signature_verifier(f"{context} candidate supply-chain attestations")


def read_candidate_blob(repo_root: Path, candidate_commit: str, path: str) -> bytes | None:
    repository = resolve_candidate_repository(repo_root)
    if not re.fullmatch(r"(?:[0-9a-f]{40}|[0-9a-f]{64})", candidate_commit):
        raise ValueError("candidate commit is not a full immutable object ID")
    if (
        not path
        or path.startswith("/")
        or ".." in Path(path).parts
        or "\x00" in path
        or ":" in path
        or "\\" in path
        or Path(path).as_posix() != path
    ):
        raise ValueError("candidate blob path is not canonical repo-relative")
    result = subprocess.run(
        ["git", "cat-file", "blob", f"{candidate_commit}:{path}"],
        cwd=repository.root,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if result.returncode == 0:
        return result.stdout
    if b"does not exist" in result.stderr or b"Not a valid object name" in result.stderr:
        return None
    raise ValueError(
        f"cannot read candidate Git blob {candidate_commit}:{path}: "
        f"{result.stderr.decode('utf-8', errors='replace').strip()[:512]}"
    )


def resolve_json_pointer_from_blob(blob: bytes, pointer: str) -> Any:
    if not pointer.startswith("/"):
        raise ValueError("JSON locator lacks an RFC6901 pointer")
    value: Any = json.loads(blob)
    for raw_part in pointer[1:].split("/"):
        if re.search(r"~(?![01])", raw_part):
            raise ValueError("JSON locator contains an invalid RFC6901 escape")
        part = raw_part.replace("~1", "/").replace("~0", "~")
        if isinstance(value, list):
            if not re.fullmatch(r"0|[1-9][0-9]*", part):
                raise ValueError("JSON array pointer is not a canonical non-negative index")
            index = int(part)
            if index >= len(value):
                raise ValueError("JSON array pointer index is out of range")
            value = value[index]
        elif isinstance(value, dict):
            if part not in value:
                raise ValueError("JSON object pointer key does not exist")
            value = value[part]
        else:
            raise ValueError("JSON pointer traverses a scalar value")
    return value


def is_production_surface_path(path: str) -> bool:
    exact_kind = exact_locator_kind_for_path(path)
    source_or_schema_kind = exact_kind in {
        "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
        "python_symbol", "proto_fqn", "sql_object",
    }
    return source_or_schema_kind or path.startswith(
        (
            "go/", "java/", "rust/", "web/ui/", "proto/", "common/",
            "deployments/", "mlops/", "contracts/", "scripts/",
        )
    )


def exact_locator_kind_for_path(path: str) -> str | None:
    suffix_kinds = (
        (".go", "go_symbol"), (".rs", "rust_symbol"),
        (".java", "java_symbol"), (".tsx", "ts_symbol"), (".ts", "ts_symbol"),
        (".py", "python_symbol"),
        (".proto", "proto_fqn"), (".sql", "sql_object"),
        (".yaml", "yaml_path"), (".yml", "yaml_path"), (".json", "json_pointer"),
    )
    return next((kind for suffix, kind in suffix_kinds if path.endswith(suffix)), None)


def validate_atomic_locator(
    locator: dict[str, Any], locators: dict[str, dict[str, Any]], pr_id: str,
    candidate_commit: str,
) -> None:
    path = locator["path"]
    resolved = (REPO_ROOT / path).resolve()
    if (
        not path
        or path.startswith("/")
        or ".." in Path(path).parts
        or any(token in path for token in ("*", "?", "[", "]", "{", "}"))
        or not resolved.is_relative_to(REPO_ROOT)
        or path.endswith("/")
    ):
        raise ValueError(f"{pr_id} locator {locator['locator_id']} is not a concrete repository file")
    exact_kind = exact_locator_kind_for_path(path)
    derived_production_surface = is_production_surface_path(path)
    if locator["production_surface"] is not derived_production_surface:
        raise ValueError(
            f"{pr_id} locator production_surface differs from validator-derived path classification"
        )
    if exact_kind is not None and locator["locator_kind"] != exact_kind:
        raise ValueError(
            f"{pr_id} typed source/contract path {path} requires {exact_kind}, "
            f"not {locator['locator_kind']}"
        )
    candidate_blob = read_candidate_blob(REPO_ROOT, candidate_commit, path)
    if locator["target_state"] == "PLANNED":
        if candidate_blob is not None:
            raise ValueError(f"{pr_id} planned locator already exists and must be declared EXISTING")
        if locator["created_by_atomic_pr_id"] != pr_id or not locator["creation_reason"]:
            raise ValueError(f"{pr_id} planned locator lacks creator/reason identity")
        if locator["candidate_blob_sha256"] is not None:
            raise ValueError(f"{pr_id} planned locator cannot claim a candidate blob")
        if locator["locator_kind"] != "file" and (
            not locator["symbol_or_pointer"] or not locator["signature"]
        ):
            raise ValueError(f"{pr_id} planned code/contract locator lacks expected symbol/signature")
        if locator["production_surface"]:
            compatibility_id = locator["compatibility_entrypoint_locator_id"]
            compatibility = locators.get(compatibility_id or "")
            guard_id = locator["activation_guard_locator_id"]
            guard = locators.get(guard_id or "")
            if (
                compatibility is None
                or compatibility["target_state"] != "EXISTING"
                or compatibility["production_surface"] is not True
                or compatibility["role"] != "compatibility_entrypoint"
                or guard is None
                or guard["target_state"] != "EXISTING"
                or guard["locator_kind"] != "json_pointer"
                or guard["role"] != "activation_guard_default"
                or guard["path"] != "contracts/configuration/configuration-catalog.v1.json"
                or not re.fullmatch(r"/entries/(0|[1-9][0-9]*)/default", guard["symbol_or_pointer"] or "")
            ):
                raise ValueError(
                    f"{pr_id} planned production locator lacks an existing compatibility entrypoint/default-off guard"
                )
            guard_blob = read_candidate_blob(REPO_ROOT, candidate_commit, guard["path"])
            if guard_blob is None or not isinstance(guard["symbol_or_pointer"], str):
                raise ValueError(f"{pr_id} planned production guard is not candidate-bound")
            try:
                guard_value = resolve_json_pointer_from_blob(
                    guard_blob, guard["symbol_or_pointer"]
                )
                entry_pointer = guard["symbol_or_pointer"].rsplit("/", 1)[0]
                guard_entry = resolve_json_pointer_from_blob(guard_blob, entry_pointer)
            except (ValueError, KeyError, IndexError, TypeError, json.JSONDecodeError) as exc:
                raise ValueError(f"{pr_id} planned production guard cannot be resolved") from exc
            if (
                guard_value not in (False, 0, "off", "false", "disabled")
                or not isinstance(guard_entry, dict)
                or guard_entry.get("default_present") is not True
                or guard_entry.get("secret") is not False
                or not guard_entry.get("id")
                or not guard_entry.get("owner")
            ):
                raise ValueError(f"{pr_id} planned production activation guard is not default-off")
            raise ValueError(
                f"{pr_id} planned production target is blocked until the trusted guard-to-target "
                "and compatibility-seam resolver is installed"
            )
        return
    if candidate_blob is None:
        raise ValueError(f"{pr_id} existing locator path is missing")
    if locator["candidate_blob_sha256"] != sha256_bytes(candidate_blob):
        raise ValueError(f"{pr_id} existing locator candidate blob hash mismatch")
    kind = locator["locator_kind"]
    pointer = locator["symbol_or_pointer"]
    if kind == "file":
        if pointer is not None or locator["signature"] is not None:
            raise ValueError(f"{pr_id} file locator cannot self-declare a symbol/signature")
        return
    if kind == "json_pointer":
        if not isinstance(pointer, str) or not pointer.startswith("/"):
            raise ValueError(f"{pr_id} JSON locator lacks an RFC6901 pointer")
        try:
            resolve_json_pointer_from_blob(candidate_blob, pointer)
        except (ValueError, KeyError, IndexError, TypeError, json.JSONDecodeError) as exc:
            raise ValueError(f"{pr_id} JSON pointer does not resolve exactly") from exc
        return
    # AST/descriptor resolvers are deliberately fail-closed.  T1-M01 must install
    # pinned go/parser, buf descriptor, TypeScript, javac, Python ast and syn-based resolvers;
    # regex/ctags/string search is not accepted as symbol identity proof.
    raise ValueError(f"{pr_id} locator kind {kind} has no trusted exact resolver installed")


def validate_atomic_plan(
    plan_ref: dict[str, Any], expected_kind: str, pr_id: str,
    candidate_sha256: str, profile_id: str,
) -> dict[str, Any]:
    plan = load_hashed_json_artifact(REPO_ROOT, 
        plan_ref["path"], plan_ref["sha256"], ATOMIC_PLAN_MANIFEST_SCHEMA_PATH
    )
    if (
        plan["plan_id"] != plan_ref["plan_id"]
        or plan["plan_kind"] != expected_kind
        or plan["status"] != "APPROVED"
        or plan["atomic_pr_id"] != pr_id
        or plan["candidate_manifest_sha256"] != candidate_sha256
        or plan["profile_id"] != profile_id
    ):
        raise ValueError(f"{pr_id} {expected_kind} plan identity/status mismatch")
    content = plan["content"]
    if expected_kind == "TEST" and (
        not content["commands"] or not content["oracles"]
    ):
        raise ValueError(f"{pr_id} TEST plan lacks commands/oracles")
    if expected_kind == "EVIDENCE" and not content["output_artifacts"]:
        raise ValueError(f"{pr_id} EVIDENCE plan lacks output artifacts")
    if expected_kind == "ROLLBACK" and (
        not content["triggers"] or not content["steps"] or not content["recovery_oracles"]
    ):
        raise ValueError(f"{pr_id} ROLLBACK plan lacks triggers/steps/recovery oracles")
    if expected_kind == "OBSERVATION" and (
        not content["signals"] or not content["queries"] or not content["window"]
        or not content["thresholds"] or not content["stop_action"] or not content["oncall_owner"]
    ):
        raise ValueError(f"{pr_id} OBSERVATION plan is incomplete")
    oracle_ids = {item["oracle_id"] for item in content["oracles"]}
    for fixture in content["fixtures"]:
        validate_hashed_artifact(REPO_ROOT, fixture["path"], fixture["content_sha256"])
        if not set(fixture["oracle_ids"]).issubset(oracle_ids):
            raise ValueError(f"{pr_id} fixture references an unknown oracle")
    return plan


def validate_atomic_implementation_result(
    result: dict[str, Any], *, pr_id: str, pr_type: str,
    candidate_manifest_sha256: str, profile_id: str, environment_id: str,
    execution_package_sha256: str, selected_targets: list[dict[str, Any]],
    resolve_files: bool = True, approved_verification_commands: list[str] | None = None,
) -> None:
    """Validate a WRT/REF after-state as the exact reviewed target set.

    Source functions require a candidate-bound language-AST receipt.  Non-symbol
    fixtures/contracts require the exact current bytes.  A command exit code is
    insufficient unless the union of its matched target IDs covers every target
    exactly once; this prevents an implementation leaf from becoming PASS while
    one planned test function or fixture is still absent.
    """
    validate_against_schema(result, ATOMIC_IMPLEMENTATION_RESULT_SCHEMA_PATH)
    if (
        result["atomic_pr_id"] != pr_id
        or result["pr_type"] != pr_type
        or result["candidate_manifest_sha256"] != candidate_manifest_sha256
        or result["profile_id"] != profile_id
        or result["environment_id"] != environment_id
        or result["execution_package_sha256"] != execution_package_sha256
        or result["result"] != "PASS"
        or result["blockers"]
    ):
        raise ValueError(f"{pr_id} after-state implementation result identity/status mismatch")

    delta_ref = result["implementation_delta_ref"]
    if pr_type == "WRT" and delta_ref is None:
        raise ValueError(f"{pr_id} WRT PASS lacks a reviewed before-to-after delta")
    if resolve_files and delta_ref is not None:
        delta = load_hashed_json_artifact(REPO_ROOT, 
            delta_ref["path"], delta_ref["sha256"],
            REPO_ROOT / "contracts/alignment/function-implementation-delta.schema.json",
        )
        if (
            delta["artifact_status"] != "REVIEWED"
            or delta["atomic_pr_id"] != pr_id
            or delta["candidate_manifest_sha256"] != candidate_manifest_sha256
            or delta["blockers"]
        ):
            raise ValueError(f"{pr_id} implementation delta is not reviewed for this candidate")

    expected_by_id = {
        f"TARGET-{index:03d}": target
        for index, target in enumerate(selected_targets, start=1)
    }
    actual_items = [*result["after_units"], *result["after_artifacts"]]
    actual_ids = [item["target_id"] for item in actual_items]
    if len(actual_ids) != len(set(actual_ids)) or set(actual_ids) != set(expected_by_id):
        raise ValueError(f"{pr_id} after-state target ID set differs from selected targets")
    actual_by_id = {item["target_id"]: item for item in actual_items}
    symbol_kinds = {"go_symbol", "rust_symbol", "java_symbol", "ts_symbol", "python_symbol"}
    for target_id, target in expected_by_id.items():
        item = actual_by_id[target_id]
        expected_kind = claim_locator_kind(
            target["path"], claim_surface_kind(target["path"])
        )
        if (
            item["path"] != target["path"]
            or item["locator_kind"] != expected_kind
        ):
            raise ValueError(f"{pr_id} {target_id} after-state path/kind differs from selected target")
        if expected_kind in symbol_kinds:
            if item["qualified_symbol"] != target["symbol"]:
                raise ValueError(f"{pr_id} {target_id} after-state symbol differs from selected target")
            if not resolve_files:
                continue
            resolver_schema = (
                LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH
                if expected_kind == "go_symbol"
                else PYTHON_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH
                if expected_kind == "python_symbol"
                else RUST_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH
                if expected_kind == "rust_symbol"
                else None
            )
            if resolver_schema is None:
                raise ValueError(
                    f"{pr_id} {target_id} has no trusted {expected_kind} after-state resolver installed"
                )
            receipt = load_hashed_json_artifact(REPO_ROOT, 
                item["resolver_receipt"]["path"],
                item["resolver_receipt"]["sha256"],
                resolver_schema,
            )
            locator = receipt["locator"]
            if (
                locator["path"] != item["path"]
                or locator["qualified_symbol"] != item["qualified_symbol"]
                or locator["signature"] != item["signature_after"]
                or locator["candidate_blob_sha256"] != item["candidate_blob_sha256"]
                or locator["normalized_ast_sha256"] != item["ast_node_sha256"]
                or receipt["candidate"]["manifest_sha256"] != candidate_manifest_sha256
            ):
                raise ValueError(f"{pr_id} {target_id} after-state AST receipt differs from implementation result")
        else:
            if item["symbol_or_pointer"] != target["symbol"]:
                raise ValueError(f"{pr_id} {target_id} after-state pointer differs from selected target")
            if resolve_files:
                validate_hashed_artifact(REPO_ROOT, item["path"], item["content_sha256"])

    covered_ids = [
        target_id
        for check in result["verification_checks"]
        for target_id in check["matched_target_ids"]
    ]
    if len(covered_ids) != len(set(covered_ids)) or set(covered_ids) != set(expected_by_id):
        raise ValueError(f"{pr_id} verification checks do not cover the exact after-state target set")
    command_ids = [item["command_id"] for item in result["verification_checks"]]
    if len(command_ids) != len(set(command_ids)):
        raise ValueError(f"{pr_id} verification command IDs are not unique")
    for check in result["verification_checks"]:
        if sha256_bytes(check["command"].encode("utf-8")) != check["command_sha256"]:
            raise ValueError(f"{pr_id} verification command hash differs from its exact command")
    if approved_verification_commands is not None and (
        sorted(item["command"] for item in result["verification_checks"])
        != sorted(approved_verification_commands)
    ):
        raise ValueError(f"{pr_id} verification command set differs from the signed execution package")


def validate_atomic_execution_package(
    package: dict[str, Any], binding: dict[str, Any], template: dict[str, Any],
    parent_work_id: str, candidate: dict[str, Any],
) -> None:
    pr_id = binding["pr_id"]
    if (
        package["artifact_status"] != "READY_BINDING"
        or package["readiness"]["status"] != "READY"
        or package["readiness"]["blockers"]
        or package["proof_ceiling"] != "SCOPED_EXECUTION_AUTHORIZATION_ONLY"
        or set(package["does_not_prove"])
        != {"implemented", "merged", "tested", "deployed", "accepted"}
    ):
        raise ValueError(f"{pr_id} execution package is not a fail-closed READY binding")
    if (
        package["atomic_pr_id"] != pr_id
        or package["parent_work_id"] != parent_work_id
        or package["pr_type"] != template["pr_type"]
        or package["candidate_manifest_path"] != binding["candidate_manifest_path"]
        or package["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
        or package["profile_id"] != binding["profile_id"]
        or package["bom_transition_ref"] != binding["bom_transition_ref"]
        or package["responsibility"]["owner"] != binding["owner"]
        or sorted(package["responsibility"]["reviewers"]) != sorted(binding["reviewers"])
        or sorted(package["responsibility"]["approvers"]) != sorted(binding["approvers"])
    ):
        raise ValueError(f"{pr_id} execution package identity differs from overlay/registry")
    design_schemas = {
        "code_unit_contract": (
            "CODE_UNIT_CONTRACT",
            REPO_ROOT / "contracts/alignment/code-unit-contract.schema.json",
        ),
        "implementation_delta": (
            "FUNCTION_IMPLEMENTATION_DELTA",
            REPO_ROOT / "contracts/alignment/function-implementation-delta.schema.json",
        ),
        "source_precedence_contract": (
            "ASSET_UPSERT_SOURCE_PRECEDENCE",
            REPO_ROOT / "contracts/alignment/asset-upsert-source-precedence.schema.json",
        ),
    }
    for key, (expected_kind, schema_path) in design_schemas.items():
        ref = package["design_refs"][key]
        if ref["artifact_kind"] != expected_kind:
            raise ValueError(f"{pr_id} {key} design reference has the wrong artifact kind")
        design = load_hashed_json_artifact(REPO_ROOT, ref["path"], ref["sha256"], schema_path)
        if design.get("artifact_kind") != expected_kind:
            raise ValueError(f"{pr_id} {key} design artifact kind differs from its typed reference")
        if design.get("atomic_pr_id", pr_id) != pr_id:
            raise ValueError(f"{pr_id} {key} design artifact crosses atomic PR identity")
        design_candidate = design.get("candidate_manifest_sha256")
        if design_candidate is None and isinstance(design.get("candidate"), dict):
            design_candidate = design["candidate"].get("manifest_sha256")
        if design_candidate != package["candidate_manifest_sha256"]:
            raise ValueError(f"{pr_id} {key} design artifact crosses candidate identity")
    writable_locators = {
        item["locator_id"]: item for item in package["selected_targets"]
    }
    context_locators = {
        item["locator_id"]: item for item in package["context_locators"]
    }
    locators = {**writable_locators, **context_locators}
    if (
        len(writable_locators) != len(package["selected_targets"])
        or len(context_locators) != len(package["context_locators"])
        or set(writable_locators) & set(context_locators)
        or {item["path"] for item in package["selected_targets"]}
        & {item["path"] for item in package["context_locators"]}
    ):
        raise ValueError(f"{pr_id} execution package repeats a locator ID")
    for locator in package["selected_targets"]:
        if locator["target_state"] == "PLANNED" and locator["production_surface"]:
            if (
                locator["compatibility_entrypoint_locator_id"] not in context_locators
                or locator["activation_guard_locator_id"] not in context_locators
            ):
                raise ValueError(
                    f"{pr_id} planned production target must use read-only context locators"
                )
    package_paths = {item["path"] for item in package["selected_targets"]}
    binding_targets = {(item["path"], item["symbol"]) for item in binding["selected_targets"]}
    package_targets = {
        (item["path"], item["symbol_or_pointer"])
        for item in package["selected_targets"]
    }
    if (
        package_paths != set(package["allowed_paths"])
        or package_paths != set(binding["allowed_paths"])
        or package_targets != binding_targets
    ):
        raise ValueError(f"{pr_id} execution package locator/allowed-path closure mismatch")
    for locator in package["selected_targets"] + package["context_locators"]:
        validate_atomic_locator(
            locator, locators, pr_id, candidate["implementation_candidate_commit"]
        )
    plans = package["plan_refs"]
    loaded_plans: dict[str, dict[str, Any]] = {}
    for key, kind in (("test", "TEST"), ("evidence", "EVIDENCE"), ("rollback", "ROLLBACK")):
        loaded_plans[key] = validate_atomic_plan(
            plans[key], kind, pr_id,
            binding["candidate_manifest_sha256"], binding["profile_id"],
        )
    if plans["observation"] is not None:
        loaded_plans["observation"] = validate_atomic_plan(
            plans["observation"], "OBSERVATION", pr_id,
            binding["candidate_manifest_sha256"], binding["profile_id"],
        )
    if (
        binding["test_plan_id"] != plans["test"]["plan_id"]
        or binding["evidence_plan_id"] != plans["evidence"]["plan_id"]
        or binding["rollback_plan_id"] != plans["rollback"]["plan_id"]
    ):
        raise ValueError(f"{pr_id} overlay free-form plan IDs differ from immutable package plans")
    for key, plan in loaded_plans.items():
        if (
            plan["owner"] != package["responsibility"]["owner"]
            or sorted(plan["reviewers"]) != sorted(package["responsibility"]["reviewers"])
            or sorted(plan["approvers"]) != sorted(package["responsibility"]["approvers"])
        ):
            raise ValueError(f"{pr_id} {key} plan responsibility differs from execution package")
    if (
        "observation" in loaded_plans
        and loaded_plans["observation"]["content"]["rollback_plan_id"]
        != plans["rollback"]["plan_id"]
    ):
        raise ValueError(f"{pr_id} OBSERVATION plan references a different rollback plan")
    pr_type = package["pr_type"]
    production_files = {
        item["path"] for item in package["selected_targets"]
        if is_production_surface_path(item["path"])
    }
    if len(production_files) > template["max_production_files"]:
        raise ValueError(f"{pr_id} exceeds the registry production-file limit")
    if pr_type == "CTR":
        impacts = package["contract_impacts"]
        if (
            not impacts
            or any(
                item["contract_kind"] == "NONE"
                or item["runtime_enablement"]
                or not item["version"]
                or item["locator_id"] not in writable_locators
                for item in impacts
            )
        ):
            raise ValueError(f"{pr_id} CTR lacks a typed, located, non-runtime contract")
        for item in impacts:
            kind = item["contract_kind"]
            required_by_kind = {
                "OPENAPI": ("operation_id", "route_id"),
                "PROTO": ("proto_fqn",),
                "KAFKA": ("topic", "key_contract", "schema_ref"),
                "DDL": ("schema_ref",),
                "UI": ("route_id",),
            }
            if any(not item[field] for field in required_by_kind[kind]):
                raise ValueError(f"{pr_id} {kind} contract impact lacks its typed identity")
            if kind == "PROTO" and not item["field_numbers"]:
                raise ValueError(f"{pr_id} PROTO contract impact lacks field numbers")
        versioned_contracts = {
            (item["contract_id"], item["version"])
            for item in impacts
            if item["contract_kind"] in {"OPENAPI", "PROTO", "KAFKA"}
        }
        if len(versioned_contracts) > template["max_event_or_api_versions"]:
            raise ValueError(f"{pr_id} exceeds the registry event/API contract limit")
        raise ValueError(
            f"{pr_id} CTR is blocked until the trusted OpenAPI/Proto/Kafka/DDL/UI contract resolver is installed"
        )
    if pr_type == "EXP":
        migration = package["migration"]
        if template["max_expand_migrations"] != 1 or migration is None:
            raise ValueError(f"{pr_id} EXP lacks the single registry-approved migration")
        if not migration["additive"] or not migration["reentrant"]:
            raise ValueError(f"{pr_id} EXP migration is not additive and reentrant")
        migration_blob = read_candidate_blob(REPO_ROOT, 
            candidate["implementation_candidate_commit"], migration["path"]
        )
        if migration_blob is None or sha256_bytes(migration_blob) != migration["content_sha256"]:
            raise ValueError(f"{pr_id} EXP migration path/hash is not candidate-bound")
        migration_locators = [
            item for item in package["selected_targets"]
            if item["path"] == migration["path"] and item["locator_kind"] == "sql_object"
        ]
        if len(migration_locators) != 1:
            raise ValueError(f"{pr_id} EXP migration is not bound to one SQL-object locator")
        raise ValueError(
            f"{pr_id} EXP is blocked until the trusted SQL additive-DDL resolver is installed"
        )
    if pr_type != "EXP" and package["migration"] is not None:
        raise ValueError(f"{pr_id} non-EXP package includes a migration")
    if pr_type not in {"CTR", "REF"} and package["contract_impacts"]:
        raise ValueError(f"{pr_id} non-contract package changes a contract")
    if pr_type in {"WRT", "PRJ"} and package["transaction"] is None:
        raise ValueError(f"{pr_id} {pr_type} lacks transaction/crash semantics")
    if pr_type in {"WRT", "PRJ", "UI"} and (
        not package["security"]["tenant_source"]
        or not package["security"]["scopes"]
        or not package["security"]["object_tenant_predicates"]
        or not package["error_mappings"]
        or (pr_type in {"WRT", "UI"} and not package["security"]["actions"])
    ):
        raise ValueError(f"{pr_id} {pr_type} lacks tenant/error semantics")
    if pr_type == "OPS":
        production_prefixes = ("go/", "java/", "rust/", "web/ui/src/", "proto/", "common/sql/")
        if plans["observation"] is None or any(
            path.startswith(production_prefixes) for path in package["allowed_paths"]
        ):
            raise ValueError(f"{pr_id} OPS lacks observation or includes production source")
    if pr_type == "REF" and (
        not package["contract_impacts"]
        or any(item["contract_kind"] != "NONE" or item["runtime_enablement"] for item in package["contract_impacts"])
    ):
        raise ValueError(f"{pr_id} REF changes an external/runtime contract")
    test_plan = loaded_plans["test"]
    test_gates = set(test_plan["content"]["required_gates"])
    if test_gates != set(template["required_gates"]):
        raise ValueError(f"{pr_id} TEST plan gate set differs from registry leaf")
    proof_ceiling = test_plan["content"]["proof_ceiling"]
    if pr_type not in {"TST-PRE", "TST-POST"}:
        # For implementation leaves required_gates are applicability/parent
        # exit gates, not evidence ownership.  The plan declares the maximum
        # future gate the change must survive; the non-evidence leaf remains
        # forbidden from attaching any run in validate_evidence_gate_state.
        expected_ceiling = max(test_gates, key=lambda gate: int(gate[1:]))
        if proof_ceiling != expected_ceiling:
            raise ValueError(f"{pr_id} TEST applicability ceiling differs from registry gates")
        maximum_gate_rank = int(expected_ceiling[1:])
    else:
        if not test_gates:
            raise ValueError(f"{pr_id} evidence leaf lacks a required gate")
        if pr_type == "TST-PRE" and not test_gates.issubset({"G0", "G1"}):
            raise ValueError(f"{pr_id} TST-PRE attempts to authorize a post-deploy gate")
        if pr_type == "TST-POST" and not any(int(gate[1:]) >= 2 for gate in test_gates):
            raise ValueError(f"{pr_id} TST-POST lacks a G2+ gate")
        if pr_type == "TST-POST" and not test_plan["content"]["environment_constraints"]:
            raise ValueError(f"{pr_id} TST-POST lacks environment constraints")
        expected_ceiling = max(test_gates, key=lambda gate: int(gate[1:]))
        if proof_ceiling != expected_ceiling:
            raise ValueError(f"{pr_id} TEST proof ceiling differs from its required-gate ceiling")
        maximum_gate_rank = int(expected_ceiling[1:])
    for key, plan in loaded_plans.items():
        ceiling = plan["content"]["proof_ceiling"]
        if key in {"rollback", "observation"} and ceiling != "DESIGN_ONLY":
            raise ValueError(f"{pr_id} {key} plan cannot itself claim an executed gate")
        if ceiling != "DESIGN_ONLY" and (
            maximum_gate_rank < 0 or int(ceiling[1:]) > maximum_gate_rank
        ):
            raise ValueError(f"{pr_id} {key} plan proof ceiling exceeds the registry leaf")
    for fixture in test_plan["content"]["fixtures"]:
        if maximum_gate_rank >= 0 and int(fixture["proof_ceiling"][1:]) > maximum_gate_rank:
            raise ValueError(f"{pr_id} fixture proof ceiling exceeds the TEST plan")


BOM_TRANSITIONS_BY_WORK_AND_TYPE = {
    ("T1-M09-N024", "IDX"): ["BOM_DRAFT", "ASSEMBLED"],
    ("T1-M10-N016", "IDX"): ["ASSEMBLED", "DEPLOYED_VERIFIED"],
    ("T1-M11-N003", "IDX"): ["DEPLOYED_VERIFIED", "CNAS_FROZEN"],
    ("T1-M12-N007", "PROM"): ["CNAS_FROZEN", "CONTRACT_RELEASED", "RELEASED_OBSERVING"],
    ("T1-M12-N008", "IDX"): ["RELEASED_OBSERVING", "STABLE"],
}

MILESTONE_COMPLETION_BY_WORK_AND_TYPE = {
    ("T1-M00-N008", "IDX"), ("T1-M01-N013", "IDX"),
    ("T1-M02-N016", "IDX"), ("T1-M03-N018", "IDX"),
    ("T1-M04-N012", "IDX"), ("T1-M05-N007", "IDX"),
    ("T1-M06-N018", "IDX"), ("T1-M07-N020", "IDX"),
    ("T1-M08-N018", "IDX"), ("T1-M09-N024", "IDX"),
    ("T1-M10-N016", "IDX"), ("T1-M11-N012", "IDX"),
    ("T1-M12-N006", "IDX"), ("T1-M12-N008", "IDX"),
    ("T1-M13-N010", "IDX"), ("T1-M13-N020", "IDX"),
}
CONTRACT_EVIDENCE_ANCHOR_PR_ID = "T1-M12-P006-IDX-n006-s1"


def contract_requirement_id_set() -> set[str]:
    payload = json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8"))
    return {
        item["requirement_id"]
        for item in payload["requirements"]
        if item["claim_class"] in {"contract_scope", "formal_kpi", "enabling_engineering"}
    }


def accountable_requirement_id_set(milestone_id: str) -> set[str]:
    payload = json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8"))
    return {
        item["requirement_id"]
        for item in payload["requirements"]
        if item["accountable_milestone"] == milestone_id
    }


REQUIREMENT_REQUIRED_GATES = {
    "REQ-T1-SYS-001": {"G0", "G2", "G3", "G5", "G6", "G8"},
    "REQ-T1-DATA-CAPTURE-001": {"G0", "G1", "G2", "G3", "G4"},
    "REQ-T1-DATA-PARSE-001": {"G0", "G1", "G2", "G3"},
    "REQ-T1-FILE-RESTORE-001": {"G0", "G1", "G2", "G3", "G5"},
    "REQ-T1-ENCRYPTED-001": {"G0", "G2", "G3", "G5"},
    "REQ-T1-DATA-FOUR-SOURCE-001": {"G0", "G1", "G2", "G3"},
    "REQ-T1-FUSION-001": {"G0", "G2", "G3"},
    "REQ-T1-BASELINE-001": {"G0", "G2", "G3", "G6"},
    "REQ-T1-ATTACKCHAIN-001": {"G0", "G2", "G3", "G5"},
    "REQ-T1-AI-001": {"G0", "G2", "G3", "G4", "G6"},
    "REQ-T1-GNN-001": {"G0", "G2", "G3", "G4"},
    "REQ-T1-DET-MIDTERM-001": {"G0", "G2", "G3"},
    "REQ-T1-QUAL-001": {"G0", "G2", "G3", "G8"},
    "REQ-T1-EVI-001": {"G0", "G2", "G3", "G5"},
    "REQ-T1-SYS-DEPLOY-001": {"G0", "G2", "G3", "G6"},
    "REQ-T1-RELEASE-MIDTERM-001": {"G0", "G2", "G3", "G6"},
    "REQ-T1-INTERNAL-STRENGTHENING-001": {"G0", "G4", "G5", "G6", "G7"},
}


def evidence_set_sha256(items: list[dict[str, Any]]) -> str:
    normalized = sorted(
        (
            item["run_id"], item["subject_pr_id"], item["subject_work_id"],
            item["subject_milestone_id"], item["execution_package_sha256"],
            item["plan_kind"], item["plan_id"], item["plan_sha256"],
            item["bom_transition_sha256"], item["run_purpose"], item["gate_id"],
            item["manifest_sha256"], item["result"], item["profile_id"],
            sha256_bytes(canonical_json(sorted(
                (
                    artifact["direction"], artifact["artifact_id"], artifact["path"],
                    artifact["sha256"], artifact["schema_ref"],
                )
                for artifact in item["artifacts"]
            )).encode("utf-8")),
        )
        for item in items
    )
    return sha256_bytes(canonical_json(normalized).encode("utf-8"))


def normalized_evidence_runs(
    bindings: list[dict[str, Any]], context: str,
) -> list[dict[str, Any]]:
    evidence_by_run: dict[str, dict[str, Any]] = {}
    for binding in bindings:
        for run in binding["evidence_run_bindings"]:
            previous = evidence_by_run.get(run["run_id"])
            if previous is not None and previous != run:
                raise ValueError(f"{context} upstream evidence run identity conflicts")
            evidence_by_run[run["run_id"]] = run
    return list(evidence_by_run.values())


def validate_requirement_method_identity(
    requirement_id: str, required_metric_ids: list[str], method_ref_id: str,
    method: dict[str, Any], context: str,
) -> None:
    expected_method_by_requirement = {
        "REQ-T1-DET-MIDTERM-001": "T1-MIDTERM-KNOWN-ALERT-METHOD",
        "REQ-T1-QUAL-001": "T1-FINAL-CNAS-QUALITY-METHOD",
    }
    if (
        expected_method_by_requirement.get(requirement_id) != method_ref_id
        or method["method_id"] != method_ref_id
        or {item["metric_id"] for item in method["metrics"]} != set(required_metric_ids)
    ):
        raise ValueError(f"{context} requirement is bound to the wrong stage/metric method")


def validate_requirement_satisfaction_refs(
    refs: list[dict[str, Any]], binding: dict[str, Any], candidate: dict[str, Any],
    expected_requirement_ids: set[str], available_evidence_runs: list[dict[str, Any]],
    pr_id: str,
) -> list[dict[str, Any]]:
    ref_ids = [item["requirement_id"] for item in refs]
    if len(ref_ids) != len(set(ref_ids)) or set(ref_ids) != expected_requirement_ids:
        raise ValueError(f"{pr_id} requirement satisfaction set is not its exact required set")
    requirement_payload = json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8"))
    requirement_registry = {
        item["requirement_id"]: item
        for item in requirement_payload["requirements"]
    }
    evidence_contract_payload = json.loads(
        EVIDENCE_CONTRACT_PATH.read_text(encoding="utf-8")
    )
    evidence_contracts = validate_evidence_contract_registry(
        evidence_contract_payload, requirement_payload,
    )
    evidence_contract_registry_sha256 = sha256_bytes(EVIDENCE_CONTRACT_PATH.read_bytes())
    available_evidence_by_id = {item["run_id"]: item for item in available_evidence_runs}
    if len(available_evidence_by_id) != len(available_evidence_runs):
        raise ValueError(f"{pr_id} available evidence contains duplicate run IDs")
    loaded: list[dict[str, Any]] = []
    for ref in refs:
        manifest = load_hashed_json_artifact(REPO_ROOT, 
            ref["path"], ref["sha256"], REQUIREMENT_SATISFACTION_SCHEMA_PATH
        )
        registry_item = requirement_registry[ref["requirement_id"]]
        if requirement_payload["status"] != "APPROVED" or registry_item["status"] != "APPROVED":
            raise ValueError(f"{pr_id} cannot satisfy an unapproved requirement")
        signed_contract = evidence_contracts[ref["requirement_id"]]
        required_gates = REQUIREMENT_REQUIRED_GATES[ref["requirement_id"]]
        selected_run_ids = manifest["closure"]["evidence_run_ids"]
        if evidence_contract_payload["status"] != "SIGNED":
            raise ValueError(f"{pr_id} requirement evidence contract is not independently signed")
        if any(run_id not in available_evidence_by_id for run_id in selected_run_ids):
            raise ValueError(f"{pr_id} requirement cites an unavailable evidence run")
        expected_runs = [available_evidence_by_id[run_id] for run_id in selected_run_ids]
        if (
            {item["gate_id"] for item in expected_runs} != required_gates
            or any(item["result"] != "PASS" for item in expected_runs)
        ):
            raise ValueError(f"{pr_id} requirement lacks one or more mandatory gate runs")
        expected_evidence_set_sha256 = evidence_set_sha256(expected_runs)
        contract_artifacts = {
            (direction, item["artifact_id"]): item
            for direction, values in (
                ("INPUT", signed_contract["required_inputs"]),
                ("OUTPUT", signed_contract["required_outputs"]),
            )
            for item in values
        }
        manifest_artifacts = {
            (direction, item["artifact_id"]): item
            for direction, values in (("INPUT", manifest["inputs"]), ("OUTPUT", manifest["outputs"]))
            for item in values
        }
        if len(manifest_artifacts) != len(manifest["inputs"]) + len(manifest["outputs"]):
            raise ValueError(f"{pr_id} requirement repeats a directional artifact identity")
        run_artifacts: dict[tuple[str, str], dict[str, Any]] = {}
        contract_artifact_ids = {item_id for _direction, item_id in contract_artifacts}
        for run in expected_runs:
            for artifact in run["artifacts"]:
                key = (artifact["direction"], artifact["artifact_id"])
                if artifact["artifact_id"] not in contract_artifact_ids:
                    continue
                if key not in contract_artifacts:
                    raise ValueError(f"{pr_id} evidence reverses an artifact direction")
                previous = run_artifacts.get(key)
                if previous is not None and previous != artifact:
                    raise ValueError(f"{pr_id} evidence conflicts on one artifact identity")
                run_artifacts[key] = artifact
        canonical_manifest_artifacts = sorted(
            (
                direction, artifact_id, item["path"], item["sha256"], item["schema_ref"]
            )
            for (direction, artifact_id), item in manifest_artifacts.items()
        )
        if (
            set(manifest_artifacts) != set(contract_artifacts)
            or set(run_artifacts) != set(contract_artifacts)
            or any(
                manifest_artifacts[key]["schema_ref"] != contract_artifacts[key]["schema_ref"]
                or run_artifacts[key] != {
                    "direction": key[0],
                    "artifact_id": key[1],
                    "path": manifest_artifacts[key]["path"],
                    "sha256": manifest_artifacts[key]["sha256"],
                    "schema_ref": manifest_artifacts[key]["schema_ref"],
                }
                for key in contract_artifacts
            )
            or manifest["failure_conditions"] != signed_contract["failure_semantics"]
            or set(manifest["evidence_contract"]["required_gates"])
            != set(signed_contract["required_gates"])
            or set(manifest["evidence_contract"]["required_artifact_ids"])
            != contract_artifact_ids
        ):
            raise ValueError(f"{pr_id} requirement evidence/artifact contract is not exact")
        if (
            ref["schema_version"] != manifest["schema_version"]
            or manifest["requirement_id"] != ref["requirement_id"]
            or manifest["requirement_class"] != registry_item["claim_class"]
            or manifest["accountable_milestone"] != registry_item["accountable_milestone"]
            or manifest["evidence_contract_ref"] != {
                "requirement_id": ref["requirement_id"],
                "registry_path": EVIDENCE_CONTRACT_PATH.relative_to(REPO_ROOT).as_posix(),
                "registry_sha256": evidence_contract_registry_sha256,
                "registry_status": "SIGNED",
            }
            or manifest["satisfaction_state"] != "SATISFIED"
            or manifest["closure"]["result"] != "SATISFIED"
            or manifest["closure"]["candidate_manifest_sha256"]
            != binding["candidate_manifest_sha256"]
            or manifest["closure"]["profile_id"] != binding["profile_id"]
            or manifest["closure"]["environment_id"] != candidate["environment_id"]
            or manifest["closure"]["evidence_set_manifest_sha256"]
            != expected_evidence_set_sha256
        ):
            raise ValueError(f"{pr_id} requirement satisfaction identity/state mismatch")
        for artifact in manifest["inputs"] + manifest["outputs"]:
            validate_hashed_artifact(REPO_ROOT, artifact["path"], artifact["sha256"])
        canonical_inputs = [item for item in canonical_manifest_artifacts if item[0] == "INPUT"]
        canonical_outputs = [item for item in canonical_manifest_artifacts if item[0] == "OUTPUT"]
        if (
            manifest["closure"]["input_set_sha256"]
            != sha256_bytes(canonical_json(canonical_inputs).encode("utf-8"))
            or manifest["closure"]["output_set_sha256"]
            != sha256_bytes(canonical_json(canonical_outputs).encode("utf-8"))
        ):
            raise ValueError(f"{pr_id} requirement satisfaction artifact-set closure mismatch")
        decision_body = {
            "requirement_id": manifest["requirement_id"],
            "requirement_class": manifest["requirement_class"],
            "candidate_manifest_sha256": manifest["closure"]["candidate_manifest_sha256"],
            "profile_id": manifest["closure"]["profile_id"],
            "environment_id": manifest["closure"]["environment_id"],
            "time_window": manifest["closure"]["time_window"],
            "method_manifest_sha256": manifest["closure"]["method_manifest_sha256"],
            "evidence_contract_registry_sha256": evidence_contract_registry_sha256,
            "artifact_tuples": canonical_manifest_artifacts,
            "input_set_sha256": manifest["closure"]["input_set_sha256"],
            "output_set_sha256": manifest["closure"]["output_set_sha256"],
            "evidence_set_manifest_sha256": manifest["closure"]["evidence_set_manifest_sha256"],
            "evidence_run_ids": sorted(manifest["closure"]["evidence_run_ids"]),
            "result": manifest["closure"]["result"],
            "allowed_claim": manifest["allowed_claim"],
        }
        decision_body_sha256 = sha256_bytes(canonical_json(decision_body).encode("utf-8"))
        receipt_ids: set[str] = set()
        for receipt_ref in manifest["closure"]["authority_receipts"]:
            receipt = load_hashed_json_artifact(REPO_ROOT, 
                receipt_ref["path"], receipt_ref["sha256"],
                REQUIREMENT_SATISFACTION_AUTHORITY_RECEIPT_SCHEMA_PATH,
            )
            required_authority_roles = {"system_owner", "qa_owner"}
            if registry_item["claim_class"] == "formal_kpi":
                required_authority_roles.add("quality_owner")
            if manifest["requirement_id"] == "REQ-T1-QUAL-001":
                required_authority_roles.add("cnas_authorized_signatory")
            if (
                receipt_ref["schema_version"] != receipt["schema_version"]
                or receipt_ref["receipt_id"] != receipt["receipt_id"]
                or receipt["receipt_id"] in receipt_ids
                or receipt["requirement_id"] != manifest["requirement_id"]
                or receipt["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
                or receipt["profile_id"] != binding["profile_id"]
                or receipt["environment_id"] != candidate["environment_id"]
                or receipt["evidence_set_manifest_sha256"] != expected_evidence_set_sha256
                or receipt["decision_body_sha256"] != decision_body_sha256
                or receipt["signed_payload_sha256"] != decision_body_sha256
                or receipt["decision"] != "SATISFIED"
                or receipt["verification"]["status"] != "PASS"
                or not required_authority_roles.issubset(
                    {item["role"] for item in receipt["authorities"]}
                )
                or len(receipt["signature_artifacts"]) < len(receipt["authorities"])
            ):
                raise ValueError(f"{pr_id} requirement authority receipt identity mismatch")
            receipt_ids.add(receipt["receipt_id"])
            validate_hashed_artifact(REPO_ROOT, 
                receipt["signed_payload_artifact"], receipt["signed_payload_sha256"]
            )
            for signature in receipt["signature_artifacts"]:
                validate_hashed_artifact(REPO_ROOT, signature["path"], signature["sha256"])
            require_trusted_signature_verifier(
                f"{pr_id} requirement satisfaction authority receipt"
            )
        if registry_item["claim_class"] == "formal_kpi":
            method_ref = manifest["method_ref"]
            if (
                method_ref is None
                or method_ref["status"] != "SIGNED"
                or manifest["closure"]["method_manifest_sha256"] != method_ref["sha256"]
            ):
                raise ValueError(f"{pr_id} formal KPI satisfaction lacks a signed method")
            method_registry = load_hashed_json_artifact(REPO_ROOT, 
                method_ref["path"], method_ref["sha256"], METRIC_METHOD_SCHEMA_PATH
            )
            validate_metric_method_semantics(method_registry)
            methods = {item["method_id"]: item for item in method_registry["methods"]}
            if (
                method_ref["method_id"] not in methods
                or methods[method_ref["method_id"]]["method_status"] != "SIGNED"
            ):
                raise ValueError(f"{pr_id} formal KPI method identity/status mismatch")
            validate_requirement_method_identity(
                ref["requirement_id"], registry_item["metric_method_ids"],
                method_ref["method_id"], methods[method_ref["method_id"]], pr_id,
            )
        loaded.append(manifest)
    return loaded


def unique_result_map(items: list[dict[str, Any]], context: str) -> dict[str, dict[str, Any]]:
    identities = [item["id"] for item in items]
    if len(identities) != len(set(identities)):
        raise ValueError(f"{context} repeats a result identity")
    return {item["id"]: item for item in items}


def validate_completion_run_bindings(
    rollback_run_id: str, rollback_plan_ref: dict[str, Any],
    observation: dict[str, Any], observation_required: bool,
    evidence: list[dict[str, Any]], milestone_id: str,
    upstream_pr_ids: set[str], context: str,
) -> None:
    def plan_identity(run: dict[str, Any]) -> dict[str, Any]:
        return {
            "subject_pr_id": run["subject_pr_id"],
            "execution_package_sha256": run["execution_package_sha256"],
            "plan_id": run["plan_id"],
            "plan_sha256": run["plan_sha256"],
            "bom_transition_sha256": run["bom_transition_sha256"],
        }

    rollback_runs = [
        item for item in evidence
        if item["run_id"] == rollback_run_id
        and item["subject_milestone_id"] == milestone_id
        and item["subject_pr_id"] in upstream_pr_ids
        and item["run_purpose"] == "ROLLBACK_REHEARSAL"
        and item["result"] == "PASS"
        and plan_identity(item) == rollback_plan_ref
    ]
    if len(rollback_runs) != 1:
        raise ValueError(f"{context} lacks one bound PASS rollback rehearsal")
    observation_runs = [
        item for item in evidence
        if item["run_id"] == observation["run_id"]
        and item["subject_milestone_id"] == milestone_id
        and item["subject_pr_id"] in upstream_pr_ids
        and item["run_purpose"] == "OBSERVATION"
        and item["result"] == "PASS"
        and item["time_window"] == observation["window"]
        and plan_identity(item) == observation["plan_ref"]
    ]
    if observation_required:
        if observation["status"] != "PASS" or len(observation_runs) != 1:
            raise ValueError(f"{context} requires one bound PASS observation run")
    elif observation["status"] == "NOT_REQUIRED":
        if observation["run_id"] is not None or observation["plan_ref"] is not None or observation_runs:
            raise ValueError(f"{context} NOT_REQUIRED observation cannot cite a run")
    elif observation["status"] != "PASS" or len(observation_runs) != 1:
        raise ValueError(f"{context} optional observation is neither NOT_REQUIRED nor bound PASS")


def validate_milestone_completion_candidate(
    ref: dict[str, Any], binding: dict[str, Any], candidate: dict[str, Any],
    milestone_id: str, upstream_pr_ids: set[str], upstream_external_ids: set[str],
    upstream_bindings: list[dict[str, Any]], external_bindings: dict[str, dict[str, Any]],
    requirement_manifests: list[dict[str, Any]],
    expected_accountable_canonical_ids: set[str], observation_required: bool,
    pr_id: str,
) -> dict[str, Any]:
    manifest = load_hashed_json_artifact(REPO_ROOT, 
        ref["path"], ref["sha256"], MILESTONE_COMPLETION_CANDIDATE_SCHEMA_PATH
    )
    evidence = normalized_evidence_runs(upstream_bindings, pr_id)
    evidence_ids = [item["run_id"] for item in evidence]
    expected_evidence_sha = evidence_set_sha256(evidence)
    expected_requirement_results = {
        item["requirement_id"]: item for item in requirement_manifests
    }
    actual_requirement_results = unique_result_map(
        manifest["requirement_results"], f"{pr_id} requirement results"
    )
    if (
        ref["schema_version"] != manifest["schema_version"]
        or manifest["milestone_id"] != milestone_id
        or manifest["promotion_profile"] != binding["profile_id"]
        or manifest["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
        or manifest["environment_id"] != candidate["environment_id"]
        or manifest["result"] != "READY_FOR_IDX"
        or manifest["blockers"]
        or set(manifest["required_atomic_prs"]) != upstream_pr_ids
        or set(manifest["required_external_activity_ids"]) != upstream_external_ids
        or set(manifest["expected_evidence_run_ids"]) != set(evidence_ids)
        or manifest["expected_evidence_set_sha256"] != expected_evidence_sha
        or set(manifest["accountable_requirements"]) != set(expected_requirement_results)
        or set(manifest["accountable_canonical_ids"])
        != expected_accountable_canonical_ids
        or set(actual_requirement_results) != set(expected_requirement_results)
        or any(
            actual_requirement_results[item_id]["manifest_sha256"]
            != next(
                item["sha256"] for item in binding["requirement_satisfaction_refs"]
                if item["requirement_id"] == item_id
            )
            or actual_requirement_results[item_id]["result"] != "SATISFIED"
            for item_id in actual_requirement_results
        )
    ):
        raise ValueError(f"{pr_id} milestone completion candidate is not an exact READY closure")
    expected_external_results = {
        item: external_bindings[item]["receipt_sha256"] for item in upstream_external_ids
    }
    actual_external_results = unique_result_map(
        manifest["external_results"], f"{pr_id} external results"
    )
    if (
        set(actual_external_results) != set(expected_external_results)
        or any(
        actual_external_results[item]["manifest_sha256"] != expected_external_results[item]
        or actual_external_results[item]["result"] != "PASS"
        for item in actual_external_results
        )
    ):
        raise ValueError(f"{pr_id} milestone completion external-result closure mismatch")
    actual_gate_runs = {
        (item["gate_id"], item["run_id"], item["manifest_sha256"], item["result"])
        for item in manifest["gate_results"]
    }
    expected_gate_runs = {
        (item["gate_id"], item["run_id"], item["manifest_sha256"], item["result"])
        for item in evidence
    }
    if (
        len(manifest["gate_results"]) != len(actual_gate_runs)
        or len(evidence) != len(expected_gate_runs)
        or actual_gate_runs != expected_gate_runs
    ):
        raise ValueError(f"{pr_id} milestone completion gate-result closure mismatch")
    validate_completion_run_bindings(
        manifest["rollback_run_id"], manifest["rollback_plan_ref"],
        manifest["observation"], observation_required, evidence,
        milestone_id, upstream_pr_ids, pr_id,
    )
    return manifest


def validate_milestone_promotion_closure(
    ref: dict[str, Any], binding: dict[str, Any], candidate: dict[str, Any],
    milestone_id: str, idx_dependencies: list[dict[str, Any]], bom_ref: dict[str, Any] | None,
    pr_id: str,
) -> dict[str, Any]:
    closure = load_hashed_json_artifact(REPO_ROOT, 
        ref["path"], ref["sha256"], MILESTONE_PROMOTION_CLOSURE_SCHEMA_PATH
    )
    completion_refs = [
        item["milestone_completion_candidate_ref"] for item in idx_dependencies
        if item["milestone_completion_candidate_ref"] is not None
    ]
    if len(completion_refs) != 1:
        raise ValueError(f"{pr_id} PROM must have one direct milestone completion candidate")
    idx_sha = idx_dependencies[0]["current_idx_manifest_sha256"]
    idx = load_hashed_json_artifact(REPO_ROOT, 
        idx_dependencies[0]["current_idx_manifest_path"], idx_sha,
        CURRENT_EVIDENCE_INDEX_SCHEMA_PATH,
    )
    if (
        ref["schema_version"] != closure["schema_version"]
        or closure["milestone_id"] != milestone_id
        or closure["promotion_profile"] != binding["profile_id"]
        or closure["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
        or closure["environment_id"] != candidate["environment_id"]
        or closure["completion_candidate_manifest_sha256"] != completion_refs[0]["sha256"]
        or closure["current_idx_manifest_sha256"] != idx_sha
        or closure["current_evidence_set_sha256"] != evidence_set_sha256(idx["evidence_runs"])
        or closure["bom_transition_sha256"]
        != (bom_ref["sha256"] if bom_ref is not None else None)
        or closure["result"] != "PASS"
        or closure["blockers"]
    ):
        raise ValueError(f"{pr_id} milestone promotion closure mismatch")
    return closure


def validate_integrated_bom_semantics(
    bom: dict[str, Any], candidate: dict[str, Any], context: str,
) -> None:
    component_ids = [item["component_id"] for item in bom["components"]]
    edge_ids = [item["edge_id"] for item in bom["dependency_edges"]]
    route_ids = [item["route_id"] for item in bom["routes"]]
    topic_names = [item["name"] for item in bom["topics"]]
    ui_route_ids = [item["route_id"] for item in bom["ui_routes"]]
    requirement_ids_in_bom = [
        item["requirement_id"] for item in bom["requirement_evidence_mapping"]
    ]
    for label, values in (
        ("component", component_ids), ("edge", edge_ids), ("route", route_ids),
        ("topic", topic_names), ("UI route", ui_route_ids),
        ("requirement", requirement_ids_in_bom),
    ):
        if len(values) != len(set(values)):
            raise ValueError(f"{context} BOM repeats a {label} identity")
    if set(requirement_ids_in_bom) != contract_requirement_id_set():
        raise ValueError(f"{context} BOM does not close the exact 15 contract requirements")
    storage_names = [item["store"] for item in bom["storage_dependencies"]]
    if len(storage_names) != 6 or set(storage_names) != {
        "postgresql", "clickhouse", "opensearch", "nebulagraph", "redis", "minio",
    }:
        raise ValueError(f"{context} BOM does not bind the exact six storage domains")
    component_set = set(component_ids)
    role_source_prefixes = {
        "probe": ("rust/probe-agent/",),
        "ingest": ("go/control-plane/cmd/ingest-gateway/", "go/control-plane/internal/ingest/"),
        "stream_processing": ("java/flink-jobs/",),
        "control_plane": ("go/control-plane/",),
        "web_ui": ("web/ui/",),
        "model_training": ("mlops/",),
        "model_registry": ("mlops/", "go/control-plane/internal/model/", "go/control-plane/cmd/rule-manager/"),
        "gateway": ("deployments/apisix/", "go/control-plane/cmd/ingest-gateway/"),
        "storage": ("deployments/", "common/sql/"),
        "observability": ("deployments/", "common/observability/"),
        "installer": ("deployments/", "contracts/releases/"),
    }
    if any(
        not component["source_path"].startswith(role_source_prefixes[component["role"]])
        for component in bom["components"]
    ):
        raise ValueError(f"{context} BOM component role is not bound to an approved source surface")
    if any(
        edge["source_component"] not in component_set
        or edge["target_component"] not in component_set
        for edge in bom["dependency_edges"]
    ):
        raise ValueError(f"{context} BOM contains an orphan dependency edge")
    if any(
        route["source_component"] not in component_set
        or route["target_component"] not in component_set
        for route in bom["routes"]
    ):
        raise ValueError(f"{context} BOM route references a missing component")
    if any(
        topic["producer"] not in component_set
        or any(consumer not in component_set for consumer in topic["consumers"])
        for topic in bom["topics"]
    ):
        raise ValueError(f"{context} BOM topic references a missing producer/consumer")
    if bom["status"] != "BOM_DRAFT":
        required_roles = {
            "probe", "ingest", "stream_processing", "control_plane", "web_ui",
            "model_registry", "gateway", "storage", "observability", "installer",
        }
        actual_required_roles = {
            item["role"] for item in bom["components"] if item["required"]
        }
        required_source_paths = [
            item["source_path"] for item in bom["components"] if item["required"]
        ]
        if not required_roles.issubset(actual_required_roles) or not bom["routes"] or not bom["topics"]:
            raise ValueError(f"{context} BOM lacks mandatory roles/routes/topics")
        if len(required_source_paths) != len(set(required_source_paths)):
            raise ValueError(f"{context} BOM reuses one source blob as multiple required components")
        if any(not value for value in bom["model_bundle"].values()):
            raise ValueError(f"{context} assembled BOM has an incomplete model bundle")
        adjacency: dict[str, set[str]] = {component_id: set() for component_id in component_ids}
        for edge in bom["dependency_edges"]:
            adjacency[edge["source_component"]].add(edge["target_component"])
            adjacency[edge["target_component"]].add(edge["source_component"])
        for route in bom["routes"]:
            adjacency[route["source_component"]].add(route["target_component"])
            adjacency[route["target_component"]].add(route["source_component"])
        for topic in bom["topics"]:
            for consumer in topic["consumers"]:
                adjacency[topic["producer"]].add(consumer)
                adjacency[consumer].add(topic["producer"])
        required_component_ids = {
            item["component_id"] for item in bom["components"] if item["required"]
        }
        reached: set[str] = set()
        pending = [next(iter(required_component_ids))]
        while pending:
            component_id = pending.pop()
            if component_id in reached:
                continue
            reached.add(component_id)
            pending.extend(adjacency[component_id] - reached)
        if not required_component_ids.issubset(reached):
            raise ValueError(f"{context} BOM mandatory component graph is disconnected")
        image_digests = set(candidate["image_digests"])
        config_hashes = set(candidate["config_schema_migration_hashes"])
        model_hashes = set(candidate["model_threshold_dataset_hashes"])
        supply_chain_hashes = set(candidate["supply_chain_artifact_hashes"])
        runtime_hashes = set(candidate["runtime_artifact_hashes"])
        if bom["environment_manifest_sha256"] not in runtime_hashes:
            raise ValueError(f"{context} BOM environment manifest is outside candidate runtime closure")
        for prebuilt in candidate["external_or_prebuilt_artifacts"]:
            if (
                prebuilt["image_digest"] not in image_digests
                or prebuilt["sbom_or_attestation_sha256"] not in supply_chain_hashes
            ):
                raise ValueError(
                    f"{context} candidate prebuilt artifact is outside its image/SBOM closure"
                )
        for component in bom["components"]:
            source_blob = read_candidate_blob(REPO_ROOT, 
                candidate["implementation_candidate_commit"], component["source_path"]
            )
            if source_blob is None or sha256_bytes(source_blob) != component["source_sha256"]:
                raise ValueError(
                    f"{context} BOM component {component['component_id']} source is not a candidate Git blob"
                )
            if component["required"] and any(
                component[field] is None
                for field in (
                    "image_digest", "sbom_sha256", "provenance_attestation_sha256",
                    "config_sha256",
                )
            ):
                raise ValueError(
                    f"{context} required assembled component {component['component_id']} lacks provenance"
                )
            if component["image_digest"] is not None and component["image_digest"] not in image_digests:
                raise ValueError(f"{context} BOM component image is outside candidate image digests")
            if (
                component["deployed_image_digest"] is not None
                and (
                    component["deployed_image_digest"] != component["image_digest"]
                    or component["deployed_image_digest"] not in image_digests
                )
            ):
                raise ValueError(f"{context} BOM deployed image is not the candidate immutable image")
            if any(
                value is not None and value not in supply_chain_hashes
                for value in (
                    component["sbom_sha256"],
                    component["provenance_attestation_sha256"],
                )
            ):
                raise ValueError(f"{context} BOM component SBOM/provenance is outside candidate closure")
            if component["config_sha256"] is not None and component["config_sha256"] not in config_hashes:
                raise ValueError(f"{context} BOM component config is outside candidate closure")
            if not set(component["contract_sha256s"]).issubset(config_hashes):
                raise ValueError(f"{context} BOM component contract is outside candidate closure")
            if not set(component["schema_migration_sha256s"]).issubset(config_hashes):
                raise ValueError(f"{context} BOM component migration is outside candidate closure")
            if component["model_sha256"] is not None and component["model_sha256"] not in model_hashes:
                raise ValueError(f"{context} BOM component model is outside candidate closure")
        if any(edge["contract_sha256"] not in config_hashes for edge in bom["dependency_edges"]):
            raise ValueError(f"{context} BOM dependency contract is outside candidate closure")
        if any(
            route["auth_policy_sha256"] not in config_hashes
            or route["config_sha256"] not in config_hashes
            for route in bom["routes"]
        ):
            raise ValueError(f"{context} BOM route authority/config is outside candidate closure")
        if any(
            topic["acl_sha256"] not in config_hashes
            or topic["retention_sha256"] not in config_hashes
            for topic in bom["topics"]
        ):
            raise ValueError(f"{context} BOM topic ACL/retention is outside candidate closure")
        if any(
            storage["schema_sha256"] not in config_hashes
            or storage["retention_sha256"] not in config_hashes
            for storage in bom["storage_dependencies"]
        ):
            raise ValueError(f"{context} BOM storage schema/retention is outside candidate closure")
        if any(route["bundle_sha256"] not in runtime_hashes for route in bom["ui_routes"]):
            raise ValueError(f"{context} BOM UI bundle is outside candidate runtime closure")
        model_bundle = bom["model_bundle"]
        if any(
            model_bundle[field] not in model_hashes
            for field in (
                "model_sha256", "feature_sha256", "threshold_sha256",
                "dataset_manifest_sha256",
            )
        ) or any(
            model_bundle[field] not in runtime_hashes
            for field in ("registry_receipt_sha256", "runtime_ack_set_sha256")
        ):
            raise ValueError(f"{context} BOM model bundle is outside candidate closure")
        delivery_by_id = {item["artifact_id"]: item for item in candidate["delivery_artifacts"]}
        if len(delivery_by_id) != 5:
            raise ValueError(f"{context} candidate delivery artifact identities are not unique")
        for artifact in delivery_by_id.values():
            blob = read_candidate_blob(REPO_ROOT, 
                candidate["implementation_candidate_commit"], artifact["path"]
            )
            if blob is None or sha256_bytes(blob) != artifact["sha256"]:
                raise ValueError(f"{context} delivery artifact is not a candidate Git blob")
        install_expected = {
            "manifest_sha256": delivery_by_id["install-manifest"]["sha256"],
            "preflight_plan_sha256": delivery_by_id["preflight-plan"]["sha256"],
            "upgrade_plan_sha256": delivery_by_id["upgrade-plan"]["sha256"],
            "rollback_plan_sha256": delivery_by_id["rollback-plan"]["sha256"],
            "restore_plan_sha256": delivery_by_id["restore-plan"]["sha256"],
        }
        if any(bom["install_package"][key] not in {None, value} for key, value in install_expected.items()):
            raise ValueError(f"{context} BOM install package differs from candidate delivery closure")
    if bom["status"] in {
        "DEPLOYED_VERIFIED", "CNAS_FROZEN", "CONTRACT_RELEASED",
        "RELEASED_OBSERVING", "STABLE",
    }:
        for component in bom["components"]:
            if component["required"] and any(
                not component[field]
                for field in (
                    "image_digest", "deployed_image_digest", "sbom_sha256",
                    "provenance_attestation_sha256", "config_sha256",
                )
            ):
                raise ValueError(f"{context} deployed BOM has an unbound required component")
        if any(not value for value in bom["install_package"].values()):
            raise ValueError(f"{context} deployed BOM has an incomplete install package")
    if bom["status"] in {"CONTRACT_RELEASED", "RELEASED_OBSERVING", "STABLE"} and any(
        item["result"] != "SATISFIED"
        or not item["closure_manifest_sha256"]
        or not item["current_idx_manifest_sha256"]
        for item in bom["requirement_evidence_mapping"]
    ):
        raise ValueError(f"{context} released BOM does not close all 15 contract requirements")


def bom_transition_evidence_refs(
    binding: dict[str, Any], include_current_transition: bool,
    include_promotion_closure: bool = True, include_current_idx: bool = True,
    include_evidence_runs: bool = True,
) -> list[tuple[str, str]]:
    refs: list[tuple[str, str]] = []
    if include_evidence_runs:
        refs.extend(
            (item["manifest_path"], item["manifest_sha256"])
            for item in binding["evidence_run_bindings"]
        )
    for path_key, sha_key in (
        ("current_idx_manifest_path", "current_idx_manifest_sha256"),
    ):
        if include_current_idx and binding[path_key] and binding[sha_key]:
            refs.append((binding[path_key], binding[sha_key]))
    for key in (
        "milestone_completion_candidate_ref", "milestone_promotion_closure_ref",
    ):
        if key == "milestone_promotion_closure_ref" and not include_promotion_closure:
            continue
        item = binding[key]
        if item is not None:
            refs.append((item["path"], item["sha256"]))
    refs.extend((item["path"], item["sha256"]) for item in binding["requirement_satisfaction_refs"])
    if include_current_transition and binding["bom_transition_ref"] is not None:
        refs.append(
            (binding["bom_transition_ref"]["path"], binding["bom_transition_ref"]["sha256"])
        )
    return refs


def validate_bom_transition(
    ref: dict[str, Any], binding: dict[str, Any], candidate: dict[str, Any],
    expected_state_path: list[str], upstream_refs: list[dict[str, Any]],
    upstream_bindings: list[dict[str, Any]], pr_id: str,
) -> dict[str, Any]:
    transition = load_hashed_json_artifact(REPO_ROOT, 
        ref["path"], ref["sha256"], INTEGRATED_BOM_TRANSITION_SCHEMA_PATH
    )
    input_bom = load_hashed_json_artifact(REPO_ROOT, 
        transition["input_bom_path"], transition["input_bom_sha256"],
        INTEGRATED_BOM_SCHEMA_PATH,
    )
    output_bom = load_hashed_json_artifact(REPO_ROOT, 
        transition["output_bom_path"], transition["output_bom_sha256"],
        INTEGRATED_BOM_SCHEMA_PATH,
    )
    validate_integrated_bom_semantics(input_bom, candidate, pr_id)
    validate_integrated_bom_semantics(output_bom, candidate, pr_id)
    if (
        transition["state_path"] != expected_state_path
        or transition["input_state"] != expected_state_path[0]
        or transition["output_state"] != expected_state_path[-1]
        or input_bom["status"] != expected_state_path[0]
        or output_bom["status"] != expected_state_path[-1]
        or output_bom["predecessor_bom_sha256"] != transition["input_bom_sha256"]
        or transition["bom_id"] != input_bom["bom_id"]
        or transition["bom_id"] != output_bom["bom_id"]
    ):
        raise ValueError(f"{pr_id} BOM transition state/hash chain mismatch")
    permitted_changes = {
        ("BOM_DRAFT", "ASSEMBLED"): {
            "version", "components", "dependency_edges", "routes", "topics",
            "storage_dependencies", "ui_routes", "requirement_evidence_mapping",
            "model_bundle", "install_package", "explicit_exclusions",
        },
        ("ASSEMBLED", "DEPLOYED_VERIFIED"): {
            "environment_manifest_sha256", "components", "routes", "topics",
            "storage_dependencies", "ui_routes", "install_package", "explicit_exclusions",
        },
        ("DEPLOYED_VERIFIED", "CNAS_FROZEN"): {
            "requirement_evidence_mapping", "model_bundle", "explicit_exclusions",
        },
        ("CNAS_FROZEN", "RELEASED_OBSERVING"): {
            "requirement_evidence_mapping", "install_package", "explicit_exclusions",
        },
        ("RELEASED_OBSERVING", "STABLE"): set(),
    }
    state_pair = (transition["input_state"], transition["output_state"])
    if state_pair == ("CNAS_FROZEN", "RELEASED_OBSERVING") and transition["state_path"] != [
        "CNAS_FROZEN", "CONTRACT_RELEASED", "RELEASED_OBSERVING"
    ]:
        raise ValueError(f"{pr_id} contract release must record the intermediate release state")
    changed_fields = {
        field for field in input_bom
        if field not in {"status", "predecessor_bom_sha256", "created_at"}
        and input_bom[field] != output_bom[field]
    }
    if state_pair not in permitted_changes or not changed_fields.issubset(
        permitted_changes[state_pair]
    ):
        raise ValueError(f"{pr_id} BOM transition mutates fields outside its state contract")
    for artifact in (transition, input_bom, output_bom):
        if (
            artifact["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
            or artifact["profile_id"] != binding["profile_id"]
            or artifact["environment_id"] != candidate["environment_id"]
        ):
            raise ValueError(f"{pr_id} BOM artifact differs from candidate/profile/environment")
    if state_pair == ("CNAS_FROZEN", "RELEASED_OBSERVING"):
        expected_requirement_hashes = {
            item["requirement_id"]: item["sha256"]
            for item in binding["requirement_satisfaction_refs"]
        }
        actual_requirement_hashes = {
            item["requirement_id"]: item["closure_manifest_sha256"]
            for item in output_bom["requirement_evidence_mapping"]
        }
        if expected_requirement_hashes:
            if (
                actual_requirement_hashes != expected_requirement_hashes
                or any(
                    item["current_idx_manifest_sha256"]
                    != binding["current_idx_manifest_sha256"]
                    for item in output_bom["requirement_evidence_mapping"]
                )
            ):
                raise ValueError(f"{pr_id} released BOM differs from requirement/IDX closure")
    matching_predecessors = []
    for upstream_ref in upstream_refs:
        upstream = load_hashed_json_artifact(REPO_ROOT, 
            upstream_ref["path"], upstream_ref["sha256"],
            INTEGRATED_BOM_TRANSITION_SCHEMA_PATH,
        )
        if upstream["output_state"] == expected_state_path[0]:
            matching_predecessors.append((upstream_ref, upstream))
    if expected_state_path[0] == "BOM_DRAFT":
        if transition["predecessor_transition_sha256"] is not None:
            raise ValueError(f"{pr_id} initial BOM transition unexpectedly has a predecessor")
    elif (
        len(matching_predecessors) != 1
        or transition["predecessor_transition_sha256"] != matching_predecessors[0][0]["sha256"]
        or transition["input_bom_sha256"] != matching_predecessors[0][1]["output_bom_sha256"]
        or transition["bom_id"] != matching_predecessors[0][1]["bom_id"]
    ):
        raise ValueError(f"{pr_id} BOM transition does not continue one exact predecessor")
    evidence_refs = bom_transition_evidence_refs(
        binding, include_current_transition=False, include_promotion_closure=False,
        include_current_idx=False, include_evidence_runs=False,
    )
    for upstream_binding in upstream_bindings:
        evidence_refs.extend(
            bom_transition_evidence_refs(upstream_binding, include_current_transition=True)
        )
    evidence_ref_map: dict[str, str] = {}
    for path, artifact_sha256 in evidence_refs:
        previous_path = evidence_ref_map.get(artifact_sha256)
        if previous_path is not None and previous_path != path:
            raise ValueError(f"{pr_id} BOM transition evidence hash aliases two paths")
        evidence_ref_map[artifact_sha256] = path
    if (
        not evidence_ref_map
        or transition["evidence_manifest_sha256s"] != sorted(evidence_ref_map)
    ):
        raise ValueError(f"{pr_id} BOM transition evidence set is not its exact upstream closure")
    for artifact_sha256, path in evidence_ref_map.items():
        validate_hashed_artifact(REPO_ROOT, path, artifact_sha256)
    receipt = load_hashed_json_artifact(REPO_ROOT, 
        transition["authority_receipt_path"], transition["authority_receipt_sha256"],
        BOM_TRANSITION_AUTHORITY_RECEIPT_SCHEMA_PATH,
    )
    transition_body = {
        key: value for key, value in transition.items()
        if key not in {"authority_receipt_path", "authority_receipt_sha256"}
    }
    transition_body_sha256 = sha256_bytes(
        canonical_json(transition_body).encode("utf-8")
    )
    required_authority_roles = {
        "ASSEMBLED": {"system_architect", "system_owner"},
        "DEPLOYED_VERIFIED": {"site_owner", "security_owner"},
        "CNAS_FROZEN": {"quality_owner", "dataset_custodian"},
        "RELEASED_OBSERVING": {"project_owner", "qa_owner"},
        "STABLE": {"product_owner", "operations_owner"},
    }[transition["output_state"]]
    if (
        receipt["transition_id"] != transition["transition_id"]
        or receipt["bom_id"] != transition["bom_id"]
        or receipt["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
        or receipt["profile_id"] != binding["profile_id"]
        or receipt["environment_id"] != candidate["environment_id"]
        or receipt["transition_body_sha256"] != transition_body_sha256
        or receipt["signed_payload_sha256"] != transition_body_sha256
        or receipt["decision"] != "APPROVED"
        or receipt["verification"]["status"] != "PASS"
        or not required_authority_roles.issubset(
            {item["role"] for item in receipt["authorities"]}
        )
    ):
        raise ValueError(f"{pr_id} BOM transition authority receipt mismatch")
    validate_hashed_artifact(REPO_ROOT, 
        receipt["signed_payload_artifact"], receipt["signed_payload_sha256"]
    )
    for signature in receipt["signature_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, signature["path"], signature["sha256"])
    require_trusted_signature_verifier(f"{pr_id} BOM transition authority receipt")
    return transition


def require_trusted_signature_verifier(
    request: SignatureVerificationRequest | str,
    *,
    client: TrustedSignatureVerifier | None = None,
    context: str | None = None,
) -> SignatureVerificationAttestation:
    if isinstance(request, str):
        legacy_context = request.strip()
        if not legacy_context:
            raise ValueError("trusted signature validation context must be non-empty")
        raise ValueError(
            f"{legacy_context} is blocked because an exact typed verification request "
            "and protected client were not supplied"
        )
    if context is None:
        raise ValueError("trusted signature validation context must be supplied")
    if not isinstance(request, SignatureVerificationRequest):
        raise ValueError(f"{context} is blocked because the verifier request is not typed")
    if not context.strip():
        raise ValueError("trusted signature validation context must be non-empty")
    if client is None or not callable(getattr(client, "verify_exact_payload", None)):
        raise ValueError(
            f"{context} is blocked because a protected trusted-signature client was not injected"
        )
    try:
        attestation = client.verify_exact_payload(request)
    except Exception as exc:
        raise ValueError(f"{context} trusted signature verifier rejected or failed") from exc
    if not isinstance(attestation, SignatureVerificationAttestation):
        raise ValueError(f"{context} verifier returned an untyped attestation")
    binding = attestation.request_binding
    signed = request.signed_payload
    if (
        attestation.decision != "PASS"
        or attestation.decision_code != "VERIFIED"
        or binding.request_id != request.request_id
        or binding.subject_payload_sha256 != signed.subject_payload.content_sha256
        or binding.candidate_commit != signed.candidate_commit
        or binding.profile_id != signed.profile_id
        or binding.environment_id != signed.environment_id
        or binding.purpose != signed.purpose
        or binding.policy_fingerprint_sha256 != signed.policy_fingerprint_sha256
        or binding.nonce != signed.nonce
        or binding.signature_sha256 != request.verification_material.signature_sha256
        or tuple(binding.required_authority_roles) != tuple(signed.required_authority_roles)
        or tuple(binding.required_scopes) != tuple(signed.required_scopes)
    ):
        raise ValueError(f"{context} verifier attestation identity does not exactly match the request")
    return attestation


def validate_evidence_gate_state(
    pr_id: str,
    pr_type: str,
    produces_new_evidence: bool,
    required_gates: list[str],
    readiness_status: str,
    evidence_bindings: list[dict[str, Any]],
) -> None:
    actual_gates = [item["gate_id"] for item in evidence_bindings]
    required_gate_set = set(required_gates)
    if readiness_status == "READY" and produces_new_evidence:
        if evidence_bindings:
            raise ValueError(
                f"{pr_id} READY evidence-producing leaf cannot claim a run before execution"
            )
        return
    if readiness_status == "PASS" and produces_new_evidence:
        if (
            not evidence_bindings
            or any(item["result"] != "PASS" for item in evidence_bindings)
            or set(actual_gates) != required_gate_set
            or len(actual_gates) != len(set(actual_gates))
        ):
            raise ValueError(
                f"{pr_id} PASS evidence leaf must have exactly one same-candidate PASS "
                f"run for each required gate {sorted(required_gate_set)}"
            )
        return
    if pr_type == "IDX":
        if (
            not evidence_bindings
            or any(item["result"] != "PASS" for item in evidence_bindings)
            or not required_gate_set.issubset(set(actual_gates))
        ):
            raise ValueError(
                f"{pr_id} IDX requires current PASS runs covering {sorted(required_gate_set)}"
            )
        return
    if evidence_bindings:
        raise ValueError(f"{pr_id} non-evidence/non-IDX leaf cannot attach evidence runs")


def validate_external_predecessor_states(
    activity_id: str,
    template: dict[str, Any],
    pr_bindings: dict[str, dict[str, Any]],
    external_bindings: dict[str, dict[str, Any]],
) -> None:
    non_pass_pr_dependencies = [
        dependency_id
        for dependency_id in template["depends_on_prs"]
        if pr_bindings[dependency_id]["readiness_status"] != "PASS"
    ]
    non_pass_external_dependencies = [
        dependency_id
        for dependency_id in template["depends_on_external_activities"]
        if external_bindings[dependency_id]["status"] != "PASS"
    ]
    if non_pass_pr_dependencies or non_pass_external_dependencies:
        raise ValueError(
            f"external activity {activity_id} has non-PASS predecessors: "
            f"prs={non_pass_pr_dependencies}, external={non_pass_external_dependencies}"
        )


def validate_current_evidence_index_state(
    pr_id: str,
    required_gates: list[str],
    binding: dict[str, Any],
    candidate: dict[str, Any],
    current_idx: dict[str, Any],
) -> None:
    if (
        current_idx["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
        or current_idx["profile_id"] != binding["profile_id"]
        or current_idx["environment_id"] != candidate["environment_id"]
    ):
        raise ValueError(f"{pr_id} IDX is bound to a different candidate/profile/environment")
    indexed_runs = sorted(
        (
            item["run_id"], item["subject_pr_id"], item["subject_work_id"],
            item["subject_milestone_id"], item["execution_package_sha256"],
            item["plan_kind"], item["plan_id"], item["plan_sha256"],
            item["bom_transition_sha256"], item["profile_id"], item["run_purpose"],
            item["gate_id"], canonical_json(item["artifacts"]), item["manifest_path"],
            item["manifest_sha256"], item["result"],
        )
        for item in current_idx["evidence_runs"]
    )
    bound_runs = sorted(
        (
            item["run_id"], item["subject_pr_id"], item["subject_work_id"],
            item["subject_milestone_id"], item["execution_package_sha256"],
            item["plan_kind"], item["plan_id"], item["plan_sha256"],
            item["bom_transition_sha256"], item["profile_id"], item["run_purpose"],
            item["gate_id"], canonical_json(item["artifacts"]), item["manifest_path"],
            item["manifest_sha256"], item["result"],
        )
        for item in binding["evidence_run_bindings"]
    )
    indexed_run_ids = [item["run_id"] for item in current_idx["evidence_runs"]]
    bound_run_ids = [item["run_id"] for item in binding["evidence_run_bindings"]]
    if (
        not binding["evidence_run_bindings"]
        or indexed_runs != bound_runs
        or len(indexed_run_ids) != len(set(indexed_run_ids))
        or len(bound_run_ids) != len(set(bound_run_ids))
        or any(item["result"] != "PASS" for item in current_idx["evidence_runs"])
        or not set(required_gates).issubset(
            {item["gate_id"] for item in current_idx["evidence_runs"]}
        )
        or set(current_idx["superseded_run_ids"])
        & {item["run_id"] for item in current_idx["evidence_runs"]}
    ):
        raise ValueError(
            f"{pr_id} IDX does not exactly close its current non-superseded PASS runs "
            "or required gates"
        )


def validate_current_idx_upstream_closure(
    pr_id: str,
    current_idx: dict[str, Any],
    upstream_bindings: list[dict[str, Any]],
) -> None:
    expected_by_run_id: dict[str, tuple[Any, ...]] = {}
    for upstream in upstream_bindings:
        for item in upstream["evidence_run_bindings"]:
            identity = (
                item["run_id"], item["subject_pr_id"], item["subject_work_id"],
                item["subject_milestone_id"], item["execution_package_sha256"],
                item["plan_kind"], item["plan_id"], item["plan_sha256"],
                item["bom_transition_sha256"], item["profile_id"], item["run_purpose"],
                item["gate_id"], canonical_json(item["artifacts"]), item["manifest_path"],
                item["manifest_sha256"], item["result"],
            )
            previous = expected_by_run_id.setdefault(item["run_id"], identity)
            if previous != identity:
                raise ValueError(
                    f"{pr_id} upstream evidence reuses run_id {item['run_id']} with a different identity"
                )
    indexed_by_run_id = {
        item["run_id"]: (
            item["run_id"], item["subject_pr_id"], item["subject_work_id"],
            item["subject_milestone_id"], item["execution_package_sha256"],
            item["plan_kind"], item["plan_id"], item["plan_sha256"],
            item["bom_transition_sha256"], item["profile_id"], item["run_purpose"],
            item["gate_id"], canonical_json(item["artifacts"]), item["manifest_path"],
            item["manifest_sha256"], item["result"],
        )
        for item in current_idx["evidence_runs"]
    }
    missing_or_changed = sorted(
        run_id
        for run_id, identity in expected_by_run_id.items()
        if indexed_by_run_id.get(run_id) != identity
    )
    if missing_or_changed:
        raise ValueError(
            f"{pr_id} current IDX omits or changes upstream PASS runs {missing_or_changed[:10]}"
        )


def validate_execution_scope_membership(
    scope: dict[str, Any],
    scoped_pr_ids: set[str],
    pr_parent_work_id: dict[str, str],
) -> None:
    declared_work_ids = set(scope["task_ids"]) | set(scope["closure_slice_ids"])
    derived_work_ids = {pr_parent_work_id[pr_id] for pr_id in scoped_pr_ids}
    declared_milestone_ids = set(scope["milestone_ids"])
    derived_milestone_ids = {pr_id[:6] for pr_id in scoped_pr_ids}
    if declared_work_ids != derived_work_ids:
        raise ValueError(
            "execution scope work IDs do not exactly match the selected atomic PR parents"
        )
    if declared_milestone_ids != derived_milestone_ids:
        raise ValueError(
            "execution scope milestone IDs do not exactly match the selected atomic PRs"
        )


def validate_milestone_profile_identity(
    milestone_id: str, actual_profile_id: str | None, expected_profile_id: str
) -> None:
    if actual_profile_id != expected_profile_id:
        raise ValueError(
            f"{milestone_id} execution profile {actual_profile_id!r} differs from "
            f"the milestone registry profile {expected_profile_id!r}"
        )


def validate_pr_parent_binding_identity(
    pr_id: str,
    parent_binding: dict[str, Any],
    pr_binding: dict[str, Any],
) -> None:
    parent_paths = {item["path"] for item in parent_binding["selected_targets"]}
    pr_paths = {item["path"] for item in pr_binding["selected_targets"]}
    if parent_binding["candidate_manifest_sha256"] != pr_binding["candidate_manifest_sha256"]:
        raise ValueError(f"{pr_id} candidate differs from its parent work binding")
    if not pr_paths.issubset(parent_paths):
        raise ValueError(f"{pr_id} selected paths are outside its parent work binding")


def validate_pr_dependency_identity(
    pr_id: str,
    pr_binding: dict[str, Any],
    dependency_bindings: list[dict[str, Any]],
) -> None:
    mismatched = [
        item["pr_id"]
        for item in dependency_bindings
        if item["candidate_manifest_sha256"] != pr_binding["candidate_manifest_sha256"]
        or item["profile_id"] != pr_binding["profile_id"]
    ]
    if mismatched:
        raise ValueError(
            f"{pr_id} differs from PR dependency candidate/profile {mismatched}"
        )


def validate_external_signature_artifacts(receipt: dict[str, Any]) -> None:
    signed_core = {
        key: value
        for key, value in receipt.items()
        if key not in {
            "signed_payload_artifact", "signed_payload_sha256",
            "signature_artifact", "signature_sha256", "signature_verification",
        }
    }
    if sha256_bytes(canonical_json(signed_core).encode("utf-8")) != receipt["signed_payload_sha256"]:
        raise ValueError("external receipt signed payload does not match receipt core")
    validate_hashed_artifact(REPO_ROOT, 
        receipt["signed_payload_artifact"], receipt["signed_payload_sha256"]
    )
    validate_hashed_artifact(REPO_ROOT, receipt["signature_artifact"], receipt["signature_sha256"])
    require_trusted_signature_verifier("external receipt authorization")


def validate_signed_contract_intake(
    intake_path: str,
    intake_sha256: str,
    expected_type: str,
    expected_subject_path: str,
    expected_subject_body_sha256: str,
) -> dict[str, Any]:
    receipt = load_hashed_json_artifact(REPO_ROOT, 
        intake_path, intake_sha256, SIGNED_CONTRACT_INTAKE_SCHEMA_PATH
    )
    if (
        receipt["intake_type"] != expected_type
        or receipt["subject_path"] != expected_subject_path
        or receipt["subject_body_sha256"] != expected_subject_body_sha256
    ):
        raise ValueError(f"signed intake subject mismatch: {intake_path}")
    if receipt["signed_payload_sha256"] != expected_subject_body_sha256:
        raise ValueError(f"signed intake payload does not match subject body: {intake_path}")
    validate_hashed_artifact(REPO_ROOT, 
        receipt["signed_payload_artifact"], receipt["signed_payload_sha256"]
    )
    for signature in receipt["signature_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, signature["path"], signature["sha256"])
    require_trusted_signature_verifier("signed contract intake authorization")
    return receipt


def validate_status_conditions(registry: dict[str, Any], milestones: dict[str, Any]) -> None:
    node_status: dict[str, str] = {}
    for task in registry["tasks"]:
        node_status.update({pr["pr_id"]: pr["status"] for pr in task["pr_sequence"]})
        node_status.update({activity["activity_id"]: activity["status"] for activity in task["external_activities"]})
    for item in registry["closure_slices"]:
        node_status.update({pr["pr_id"]: pr["status"] for pr in item["pr_sequence"]})

    for task in registry["tasks"]:
        for activity in task["external_activities"]:
            expected_native_profile = milestone_profile(task["milestone_id"].removeprefix("T1-"))
            if activity["profile_id"] != expected_native_profile:
                raise ValueError(
                    f"{activity['activity_id']} native profile differs from its milestone"
                )
            input_ids = [item["artifact_id"] for item in activity["required_inputs"]]
            if (
                input_ids != EXTERNAL_ACTIVITY_INPUTS[activity["activity_type"]]
                or len(input_ids) != len(set(input_ids))
            ):
                raise ValueError(
                    f"{activity['activity_id']} required input identity/order drift"
                )
            if activity["status"] in {"READY", "RUNNING", "PASS"}:
                missing_hashes = [
                    item["artifact_id"] for item in activity["required_inputs"]
                    if not isinstance(item["sha256"], str) or not re.fullmatch(r"[0-9a-f]{64}", item["sha256"])
                ]
                if missing_hashes:
                    raise ValueError(f"{activity['activity_id']} has unfrozen typed inputs {missing_hashes}")
                for field in ("run_id", "instance_id", "candidate_manifest_sha256", "profile_id"):
                    if not activity[field]:
                        raise ValueError(f"{activity['activity_id']} {activity['status']} misses {field}")
                expected_instance = f"{activity['activity_id']}-{activity['run_id']}"
                if activity["instance_id"] != expected_instance:
                    raise ValueError(f"{activity['activity_id']} instance_id is not run-scoped")
            if activity["status"] == "PASS":
                dependencies = activity["depends_on_prs"] + activity["depends_on_external_activities"]
                not_pass = [node_id for node_id in dependencies if node_status.get(node_id) != "PASS"]
                if not_pass:
                    raise ValueError(f"{activity['activity_id']} PASS has non-PASS dependencies {not_pass}")
                if "<run>" in activity["receipt_artifact"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt still contains a run placeholder")
                receipt_path = (REPO_ROOT / activity["receipt_artifact"]).resolve()
                if not receipt_path.is_relative_to(REPO_ROOT) or not receipt_path.is_file():
                    raise ValueError(f"{activity['activity_id']} PASS receipt does not exist in repository scope")
                receipt_bytes = receipt_path.read_bytes()
                if sha256_bytes(receipt_bytes) != activity["receipt_sha256"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt hash mismatch")
                receipt = json.loads(receipt_bytes)
                validate_against_schema(receipt, EXTERNAL_RECEIPT_SCHEMA_PATH)
                validate_external_signature_artifacts(receipt)
                if receipt["activity_id"] != activity["activity_id"] or receipt["run_id"] != activity["run_id"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt identity mismatch")
                if receipt["activity_type"] != activity["activity_type"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt type mismatch")
                if receipt["instance_id"] != activity["instance_id"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt instance mismatch")
                if receipt["candidate_manifest_sha256"] != activity["candidate_manifest_sha256"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt candidate mismatch")
                if receipt["profile_id"] != activity["profile_id"]:
                    raise ValueError(f"{activity['activity_id']} PASS receipt profile mismatch")
                expected_inputs = sorted(
                    (item["artifact_id"], item["sha256"])
                    for item in activity["required_inputs"]
                )
                actual_inputs = sorted(
                    (item["artifact_id"], item["sha256"])
                    for item in receipt["input_hashes"]
                )
                if expected_inputs != actual_inputs:
                    raise ValueError(f"{activity['activity_id']} PASS receipt input closure mismatch")
                if receipt["result"] != "PASS" or receipt["signature_verification"]["status"] != "PASS":
                    raise ValueError(f"{activity['activity_id']} PASS receipt is not signed PASS")

    # Design registries can only be DRAFT or an accepted design baseline.
    # Actual execution authority lives in the separately generated overlay.
    if registry["status"] == "ACCEPTED_DESIGN_BASELINE" and registry["validation"]["structure_status"] != "PASS":
        raise ValueError("accepted design baseline lacks structural PASS")
    if milestones["status"] == "ACCEPTED_DESIGN_BASELINE" and any(
        item["completion_checklist"]["status"] not in {"BLOCKED", "PASS"}
        for item in milestones["milestones"]
    ):
        raise ValueError("accepted design baseline has invalid milestone checklist state")


def build_execution_overlay(
    registry: dict[str, Any], milestones: dict[str, Any]
) -> dict[str, Any]:
    milestone_bindings = [
        {
            "milestone_id": item["milestone_id"],
            "readiness_status": "DRAFT",
            "owner": None,
            "reviewers": [],
            "approvers": [],
            "candidate_manifest_sha256": None,
            "profile_id": item["promotion_requirements"]["profile"],
            "blockers": list(item["completion_checklist"]["missing_fields"]),
        }
        for item in milestones["milestones"]
    ]
    task_bindings = []
    for task in registry["tasks"]:
        task_bindings.append(
            {
                "work_id": task["task_id"],
                "readiness_status": "DRAFT",
                "owner": task["responsibility"]["owner"],
                "reviewers": task["responsibility"]["reviewers"],
                "approvers": task["responsibility"]["approvers"],
                "selected_targets": [],
                "candidate_manifest_sha256": None,
                "test_plan_id": None,
                "evidence_plan_id": None,
                "rollback_plan_id": None,
                "blockers": task["readiness_blockers"],
            }
        )
    slice_bindings = []
    for item in registry["closure_slices"]:
        slice_bindings.append(
            {
                "work_id": item["slice_id"],
                "readiness_status": "DRAFT",
                "owner": None,
                "reviewers": [],
                "approvers": [],
                "selected_targets": [],
                "candidate_manifest_sha256": None,
                "test_plan_id": None,
                "evidence_plan_id": None,
                "rollback_plan_id": None,
                "blockers": item["readiness_blockers"],
            }
        )
    atomic_pr_bindings = []
    task_by_pr: dict[str, dict[str, Any]] = {}
    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            task_by_pr[pr["pr_id"]] = task
    slice_by_pr: dict[str, dict[str, Any]] = {}
    for item in registry["closure_slices"]:
        for pr in item["pr_sequence"]:
            slice_by_pr[pr["pr_id"]] = item
    for pr_id in sorted(set(task_by_pr) | set(slice_by_pr)):
        parent = task_by_pr.get(pr_id) or slice_by_pr[pr_id]
        pr = next(
            item for item in parent["pr_sequence"] if item["pr_id"] == pr_id
        )
        responsibility = parent.get("responsibility", {})
        atomic_pr_bindings.append(
            {
                "pr_id": pr_id,
                "readiness_status": "DRAFT",
                "owner": responsibility.get("owner"),
                "reviewers": responsibility.get("reviewers", []),
                "approvers": responsibility.get("approvers", []),
                "selected_targets": [],
                "allowed_paths": [],
                "candidate_manifest_path": None,
                "candidate_manifest_sha256": None,
                "profile_id": None,
                "execution_package_ref": None,
                "bom_transition_ref": None,
                "requirement_satisfaction_refs": [],
                "milestone_completion_candidate_ref": None,
                "milestone_promotion_closure_ref": None,
                "task_completion_candidate_ref": None,
                "task_current_index_ref": None,
                "current_idx_manifest_path": None,
                "current_idx_manifest_sha256": None,
                "promotion_intent_manifest_path": None,
                "promotion_intent_manifest_sha256": None,
                "postmerge_result_manifest_path": None,
                "postmerge_result_manifest_sha256": None,
                "evidence_run_bindings": [],
                "test_plan_id": None,
                "evidence_plan_id": None,
                "rollback_plan_id": pr["rollback_runbook_id"],
                "blockers": [
                    "per-PR exact path/symbol selection and approval unresolved",
                    "clean implementation candidate and profile not frozen",
                    "test/evidence plan not accepted",
                ],
            }
        )
    external_activity_bindings = []
    for task in registry["tasks"]:
        for activity in task["external_activities"]:
            external_activity_bindings.append(
                {
                    "activity_id": activity["activity_id"],
                    "status": "PENDING",
                    "run_id": None,
                    "instance_id": None,
                    "candidate_manifest_sha256": None,
                    "profile_id": activity["profile_id"],
                    "required_inputs": activity["required_inputs"],
                    "receipt_artifact": activity["receipt_artifact"],
                    "receipt_sha256": None,
                    "blockers": ["typed input hashes and signed receipt unresolved"],
                }
            )
    return {
        "schema_version": "1.1.0",
        "instance_id": None,
        "task_registry_sha256": sha256_bytes(canonical_json(registry).encode("utf-8")),
        "milestone_registry_sha256": sha256_bytes(canonical_json(milestones).encode("utf-8")),
        "status": "TEMPLATE_EXECUTION_NO_GO",
        "scope": {"milestone_ids": [], "task_ids": [], "closure_slice_ids": [], "atomic_pr_ids": []},
        "milestone_bindings": milestone_bindings,
        "task_bindings": task_bindings,
        "closure_slice_bindings": slice_bindings,
        "atomic_pr_bindings": atomic_pr_bindings,
        "external_activity_bindings": external_activity_bindings,
        "acceptance": {
            "dor_status": "BLOCKED",
            "candidate_status": "BLOCKED",
            "checklist_status": "BLOCKED",
            "accepted_by": [],
            "accepted_at": None,
            "decision_receipt_path": None,
            "decision_receipt_sha256": None,
        },
    }


def validate_execution_overlay(
    overlay: dict[str, Any], registry: dict[str, Any], milestones: dict[str, Any]
) -> None:
    if overlay["task_registry_sha256"] != sha256_bytes(
        canonical_json(registry).encode("utf-8")
    ):
        raise ValueError("execution overlay is bound to a different task registry")
    if overlay["milestone_registry_sha256"] != sha256_bytes(
        canonical_json(milestones).encode("utf-8")
    ):
        raise ValueError("execution overlay is bound to a different milestone registry")
    task_ids = {task["task_id"] for task in registry["tasks"]}
    slice_ids = {item["slice_id"] for item in registry["closure_slices"]}
    milestone_ids = {item["milestone_id"] for item in milestones["milestones"]}
    task_by_id = {task["task_id"]: task for task in registry["tasks"]}
    slice_by_id = {item["slice_id"]: item for item in registry["closure_slices"]}
    milestone_by_id = {
        item["milestone_id"]: item for item in milestones["milestones"]
    }
    all_canonical_ids = {
        item["id"]
        for item in json.loads(CANONICAL_PATH.read_text(encoding="utf-8"))["items"]
    }
    if {item["milestone_id"] for item in overlay["milestone_bindings"]} != milestone_ids:
        raise ValueError("execution overlay milestone bindings do not exactly cover milestone registry")
    if {item["work_id"] for item in overlay["task_bindings"]} != task_ids:
        raise ValueError("execution overlay task bindings do not exactly cover task registry")
    if {item["work_id"] for item in overlay["closure_slice_bindings"]} != slice_ids:
        raise ValueError("execution overlay closure bindings do not exactly cover closure slices")
    pr_by_id = {
        pr["pr_id"]: pr
        for task in registry["tasks"]
        for pr in task["pr_sequence"]
    }
    pr_by_id.update(
        {
            pr["pr_id"]: pr
            for item in registry["closure_slices"]
            for pr in item["pr_sequence"]
        }
    )
    pr_parent_work_id = {
        pr["pr_id"]: task["task_id"]
        for task in registry["tasks"]
        for pr in task["pr_sequence"]
    }
    pr_parent_work_id.update(
        {
            pr["pr_id"]: item["slice_id"]
            for item in registry["closure_slices"]
            for pr in item["pr_sequence"]
        }
    )
    task_terminal_by_pr = {
        task["completion_contract"]["terminal_task_idx_pr_id"]: task
        for task in registry["tasks"]
    }
    if {item["pr_id"] for item in overlay["atomic_pr_bindings"]} != set(pr_by_id):
        raise ValueError("execution overlay atomic PR bindings do not exactly cover registry leaves")
    activity_ids = {
        activity["activity_id"]
        for task in registry["tasks"]
        for activity in task["external_activities"]
    }
    if {item["activity_id"] for item in overlay["external_activity_bindings"]} != activity_ids:
        raise ValueError("execution overlay external activity bindings do not exactly cover registry nodes")
    if overlay["status"] == "TEMPLATE_EXECUTION_NO_GO":
        if overlay["instance_id"] is not None or overlay["scope"]["atomic_pr_ids"] or any(
            item["readiness_status"] != "DRAFT" for item in overlay["atomic_pr_bindings"]
        ):
            raise ValueError("generated execution template cannot grant execution authority")
        return
    if overlay["status"] != "ACCEPTED_FOR_SCOPED_EXECUTION":
        raise ValueError("unknown execution overlay status")
    if overlay["schema_version"] != "1.1.0":
        raise ValueError("execution overlay v1.0 is historical NO-GO only and cannot authorize execution")
    if not overlay["instance_id"]:
        raise ValueError("accepted execution instance lacks a unique instance_id")
    scope = overlay["scope"]
    scoped_work_ids = set(scope["task_ids"]) | set(scope["closure_slice_ids"])
    scoped_pr_ids = set(scope["atomic_pr_ids"])
    if not scoped_pr_ids or not scope["milestone_ids"]:
        raise ValueError("scoped execution acceptance has an empty PR or milestone scope")
    unknown_prs = sorted(scoped_pr_ids - set(pr_by_id))
    if unknown_prs:
        raise ValueError(f"execution overlay has unknown scoped PR IDs {unknown_prs}")
    validate_execution_scope_membership(scope, scoped_pr_ids, pr_parent_work_id)
    bindings = {
        item["work_id"]: item
        for item in overlay["task_bindings"] + overlay["closure_slice_bindings"]
    }
    unknown = sorted(scoped_work_ids - set(bindings))
    if unknown:
        raise ValueError(f"execution overlay has unknown scoped work IDs {unknown}")
    invalid = []
    for work_id in sorted(scoped_work_ids):
        item = bindings[work_id]
        if (
            item["readiness_status"] != "READY"
            or item["blockers"]
            or not item["owner"]
            or not item["reviewers"]
            or not item["approvers"]
            or not item["selected_targets"]
            or not isinstance(item["candidate_manifest_sha256"], str)
            or not re.fullmatch(r"[0-9a-f]{64}", item["candidate_manifest_sha256"])
            or not item["test_plan_id"]
            or not item["evidence_plan_id"]
            or not item["rollback_plan_id"]
        ):
            invalid.append(work_id)
    if invalid:
        raise ValueError(f"scoped execution acceptance has non-ready bindings {invalid[:10]}")
    pr_bindings = {item["pr_id"]: item for item in overlay["atomic_pr_bindings"]}
    activity_template_by_id = {
        activity["activity_id"]: activity
        for task in registry["tasks"]
        for activity in task["external_activities"]
    }

    def upstream_pr_ids_for(pr_id: str) -> set[str]:
        result: set[str] = set()
        seen_external: set[str] = set()
        pending: list[tuple[str, str]] = [
            *(('pr', item) for item in pr_by_id[pr_id]["depends_on_prs"]),
            *(("external", item) for item in pr_by_id[pr_id]["depends_on_external_activities"]),
        ]
        while pending:
            kind, node_id = pending.pop()
            if kind == "pr":
                if node_id in result:
                    continue
                result.add(node_id)
                node = pr_by_id[node_id]
            else:
                if node_id in seen_external:
                    continue
                seen_external.add(node_id)
                node = activity_template_by_id[node_id]
            pending.extend(("pr", item) for item in node["depends_on_prs"])
            pending.extend(
                ("external", item) for item in node["depends_on_external_activities"]
            )
        return result

    def upstream_external_ids_for(pr_id: str) -> set[str]:
        result: set[str] = set()
        seen_prs: set[str] = set()
        pending: list[tuple[str, str]] = [
            *(('pr', item) for item in pr_by_id[pr_id]["depends_on_prs"]),
            *(('external', item) for item in pr_by_id[pr_id]["depends_on_external_activities"]),
        ]
        while pending:
            kind, node_id = pending.pop()
            if kind == "pr":
                if node_id in seen_prs:
                    continue
                seen_prs.add(node_id)
                node = pr_by_id[node_id]
            else:
                if node_id in result:
                    continue
                result.add(node_id)
                node = activity_template_by_id[node_id]
            pending.extend(("pr", item) for item in node["depends_on_prs"])
            pending.extend(
                ("external", item) for item in node["depends_on_external_activities"]
            )
        return result

    validation_pr_ids: set[str] = set()
    required_external_ids: set[str] = set()
    pending_nodes: list[tuple[str, str]] = [("pr", pr_id) for pr_id in scoped_pr_ids]
    while pending_nodes:
        node_kind, node_id = pending_nodes.pop()
        if node_kind == "pr":
            if node_id in validation_pr_ids:
                continue
            validation_pr_ids.add(node_id)
            node = pr_by_id[node_id]
        else:
            if node_id in required_external_ids:
                continue
            required_external_ids.add(node_id)
            node = activity_template_by_id[node_id]
        pending_nodes.extend(("pr", item) for item in node["depends_on_prs"])
        pending_nodes.extend(
            ("external", item) for item in node["depends_on_external_activities"]
        )
    invalid_prs = []
    broad_roots = {
        "go", "java", "rust", "web-ui", "proto", "common", "contracts",
        "deployments", "doc", "scripts", "tests",
    }

    def is_concrete_selected_path(path: str) -> bool:
        if not path or path.startswith("/") or ".." in Path(path).parts:
            return False
        if path.rstrip("/") in broad_roots or path.endswith("/"):
            return False
        if any(token in path for token in ("*", "?", "[", "]", "{", "}")):
            return False
        resolved = REPO_ROOT / path
        return not resolved.is_dir()

    def selected_path_has_candidate(path: str, candidate_paths: set[str]) -> bool:
        if path in candidate_paths:
            return True
        for candidate in candidate_paths:
            candidate_resolved = REPO_ROOT / candidate
            if (candidate.endswith("/") or candidate_resolved.is_dir()) and path.startswith(
                candidate.rstrip("/") + "/"
            ):
                return True
        return False

    for pr_id in sorted(validation_pr_ids):
        item = pr_bindings[pr_id]
        candidate_paths = set(pr_by_id[pr_id]["candidate_paths"])
        selected_paths = {target["path"] for target in item["selected_targets"]}
        expected_status = "READY" if pr_id in scoped_pr_ids else "PASS"
        if (
            item["readiness_status"] != expected_status
            or item["blockers"]
            or not item["owner"]
            or not item["reviewers"]
            or not item["approvers"]
            or not selected_paths
            or not item["allowed_paths"]
            or selected_paths != set(item["allowed_paths"])
            or not all(is_concrete_selected_path(path) for path in selected_paths)
            or not all(selected_path_has_candidate(path, candidate_paths) for path in selected_paths)
            or not item["candidate_manifest_path"]
            or not isinstance(item["candidate_manifest_sha256"], str)
            or not re.fullmatch(r"[0-9a-f]{64}", item["candidate_manifest_sha256"])
            or not item["profile_id"]
            or not item["execution_package_ref"]
            or not item["test_plan_id"]
            or not item["evidence_plan_id"]
            or not item["rollback_plan_id"]
        ):
            invalid_prs.append(pr_id)
    if invalid_prs:
        raise ValueError(f"scoped execution acceptance has non-ready atomic PR bindings {invalid_prs[:10]}")
    external_bindings = {
        item["activity_id"]: item for item in overlay["external_activity_bindings"]
    }
    for activity_id in sorted(required_external_ids):
        binding = external_bindings[activity_id]
        template = activity_template_by_id[activity_id]
        template_input_ids = [
            item["artifact_id"] for item in template["required_inputs"]
        ]
        binding_input_ids = [
            item["artifact_id"] for item in binding["required_inputs"]
        ]
        if (
            len(binding_input_ids) != len(set(binding_input_ids))
            or binding_input_ids != template_input_ids
        ):
            raise ValueError(
                f"external activity {activity_id} changed required input identity/order"
            )
        if (
            binding["status"] != "PASS"
            or binding["blockers"]
            or not binding["run_id"]
            or binding["instance_id"] != f"{activity_id}-{binding['run_id']}"
            or not isinstance(binding["candidate_manifest_sha256"], str)
            or not re.fullmatch(r"[0-9a-f]{64}", binding["candidate_manifest_sha256"])
            or not binding["profile_id"]
            or not isinstance(binding["receipt_sha256"], str)
            or not re.fullmatch(r"[0-9a-f]{64}", binding["receipt_sha256"])
        ):
            raise ValueError(f"scoped execution depends on non-PASS external activity {activity_id}")
        validate_external_predecessor_states(
            activity_id, template, pr_bindings, external_bindings
        )
        if any(
            not isinstance(item["sha256"], str)
            or not re.fullmatch(r"[0-9a-f]{64}", item["sha256"])
            for item in binding["required_inputs"]
        ):
            raise ValueError(f"external activity {activity_id} has unfrozen input hashes")
        required_input_map = {
            item["artifact_id"]: item["sha256"] for item in binding["required_inputs"]
        }
        if template["activity_type"] == "CUSTODY":
            metric_payload_bytes = METRIC_METHOD_PATH.read_bytes()
            if required_input_map.get("signed-final-metric-method") != sha256_bytes(
                metric_payload_bytes
            ):
                raise ValueError(
                    f"external activity {activity_id} does not bind the repository signed metric method"
                )
            metric_payload = json.loads(metric_payload_bytes)
            validate_against_schema(metric_payload, METRIC_METHOD_SCHEMA_PATH)
            validate_metric_method_semantics(metric_payload)
            final_methods = [
                item for item in metric_payload["methods"]
                if item["method_id"] == "T1-FINAL-CNAS-QUALITY-METHOD"
                and item["method_status"] == "SIGNED"
            ]
            if (
                len(final_methods) != 1
                or final_methods[0]["threshold_lock"]["dataset_manifest_hash"]
                != required_input_map.get("blind-dataset-manifest")
            ):
                raise ValueError(
                    f"external activity {activity_id} blind dataset differs from the signed final method lock"
                )
        candidate_input_hash = required_input_map.get(
            "implementation-candidate-identity-manifest"
        )
        if (
            candidate_input_hash is not None
            and candidate_input_hash != binding["candidate_manifest_sha256"]
        ):
            raise ValueError(
                f"external activity {activity_id} candidate input differs from activity candidate"
            )
        mismatched_pr_candidates = [
            pr_id
            for pr_id in template["depends_on_prs"]
            if pr_bindings[pr_id]["candidate_manifest_sha256"]
            != binding["candidate_manifest_sha256"]
            or pr_bindings[pr_id]["profile_id"] != binding["profile_id"]
        ]
        if mismatched_pr_candidates:
            raise ValueError(
                f"external activity {activity_id} differs from PR dependency candidates {mismatched_pr_candidates}"
            )
        mismatched_external_candidates = [
            dependency_id
            for dependency_id in template["depends_on_external_activities"]
            if external_bindings[dependency_id]["candidate_manifest_sha256"]
            != binding["candidate_manifest_sha256"]
            or external_bindings[dependency_id]["profile_id"] != binding["profile_id"]
        ]
        if mismatched_external_candidates:
            raise ValueError(
                f"external activity {activity_id} differs from external dependency candidates {mismatched_external_candidates}"
            )
        receipt = load_hashed_json_artifact(REPO_ROOT, 
            binding["receipt_artifact"], binding["receipt_sha256"], EXTERNAL_RECEIPT_SCHEMA_PATH
        )
        validate_external_signature_artifacts(receipt)
        if (
            receipt["activity_id"] != activity_id
            or receipt["activity_type"] != template["activity_type"]
            or receipt["run_id"] != binding["run_id"]
            or receipt["instance_id"] != binding["instance_id"]
            or receipt["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
            or receipt["profile_id"] != binding["profile_id"]
            or receipt["result"] != "PASS"
            or receipt["signature_verification"]["status"] != "PASS"
        ):
            raise ValueError(f"external activity {activity_id} receipt is not matching signed PASS")
        expected_inputs = sorted(
            (item["artifact_id"], item["sha256"])
            for item in binding["required_inputs"]
        )
        actual_inputs = sorted(
            (item["artifact_id"], item["sha256"])
            for item in receipt["input_hashes"]
        )
        if expected_inputs != actual_inputs:
            raise ValueError(f"external activity {activity_id} receipt input closure mismatch")
        expected_output_ids = EXTERNAL_ACTIVITY_OUTPUTS[template["activity_type"]]
        output_map = {item["artifact_id"]: item["sha256"] for item in receipt["output_hashes"]}
        if list(output_map) != expected_output_ids or len(output_map) != len(receipt["output_hashes"]):
            raise ValueError(f"external activity {activity_id} output identity/order drift")
        payload = receipt["activity_payload"]
        payload_output_fields = {
            "CUSTODY": {
                "sealed-blind-dataset": "sealed_dataset_sha256",
                "custody-access-log": "access_log_sha256",
                "custody-handoff-manifest": "handoff_manifest_sha256",
            },
            "EXECUTE": {
                "raw-predictions": "raw_predictions_sha256",
                "runtime-logs": "runtime_logs_sha256",
                "runtime-environment-manifest": "environment_manifest_sha256",
            },
            "ATTEST": {
                "cnas-metric-result": "result_sha256",
                "cnas-signed-report": "report_sha256",
            },
            "APPROVAL": {
                "signed-go-no-go-decision": "decision_receipt_sha256",
            },
        }
        if any(
            output_map[artifact_id] != payload[field]
            for artifact_id, field in payload_output_fields[template["activity_type"]].items()
        ):
            raise ValueError(f"external activity {activity_id} payload/output hash mismatch")
        payload_input_fields = {
            "CUSTODY": {},
            "EXECUTE": {
                "signed-final-metric-method": None,
                "sealed-blind-dataset": None,
                "threshold-lock": "threshold_lock_sha256",
                "runtime-environment-manifest": "environment_manifest_sha256",
            },
            "ATTEST": {
                "signed-final-metric-method": "method_sha256",
                "raw-predictions": None,
            },
            "APPROVAL": {
                "contract-closure-manifest": "manifest_sha256",
            },
        }
        for artifact_id, payload_field in payload_input_fields[template["activity_type"]].items():
            if artifact_id not in required_input_map:
                raise ValueError(f"external activity {activity_id} lacks typed input {artifact_id}")
            if payload_field is not None and payload[payload_field] != required_input_map[artifact_id]:
                raise ValueError(
                    f"external activity {activity_id} payload differs from input {artifact_id}"
                )
        if (
            template["activity_type"] == "APPROVAL"
            and payload["promotion_profile"] != binding["profile_id"]
        ):
            raise ValueError(f"external activity {activity_id} approval profile mismatch")
        receipt_input_id_by_type = {
            "CUSTODY": "custody-activity-receipt",
            "EXECUTE": "execute-activity-receipt",
            "ATTEST": "attest-activity-receipt",
            "APPROVAL": "approval-activity-receipt",
        }
        for dependency_id in template["depends_on_external_activities"]:
            dependency_template = activity_template_by_id[dependency_id]
            dependency_binding = external_bindings[dependency_id]
            dependency_receipt = load_hashed_json_artifact(REPO_ROOT, 
                dependency_binding["receipt_artifact"],
                dependency_binding["receipt_sha256"],
                EXTERNAL_RECEIPT_SCHEMA_PATH,
            )
            dependency_outputs = {
                item["artifact_id"]: item["sha256"]
                for item in dependency_receipt["output_hashes"]
            }
            receipt_input_id = receipt_input_id_by_type[dependency_template["activity_type"]]
            if required_input_map.get(receipt_input_id) != dependency_binding["receipt_sha256"]:
                raise ValueError(
                    f"external activity {activity_id} does not bind predecessor receipt {dependency_id}"
                )
            shared_ids = set(dependency_outputs) & set(required_input_map)
            if not shared_ids or any(
                required_input_map[item] != dependency_outputs[item] for item in shared_ids
            ):
                raise ValueError(
                    f"external activity {activity_id} does not preserve predecessor output hashes"
                )

    prom_allowed_prefixes = (
        "contracts/alignment/",
        "contracts/releases/",
        "doc/02_acceptance/",
        "doc/07_alignment/",
        "deployments/releases/",
    )
    for pr_id in sorted(validation_pr_ids):
        template = pr_by_id[pr_id]
        binding = pr_bindings[pr_id]
        if pr_id in scoped_pr_ids:
            validate_pr_parent_binding_identity(
                pr_id, bindings[pr_parent_work_id[pr_id]], binding
            )
        mismatched_external_candidates = [
            activity_id
            for activity_id in template["depends_on_external_activities"]
            if external_bindings[activity_id]["candidate_manifest_sha256"]
            != binding["candidate_manifest_sha256"]
            or external_bindings[activity_id]["profile_id"] != binding["profile_id"]
        ]
        if mismatched_external_candidates:
            raise ValueError(
                f"{pr_id} differs from external dependency candidates {mismatched_external_candidates}"
            )
        candidate = load_hashed_json_artifact(REPO_ROOT, 
            binding["candidate_manifest_path"],
            binding["candidate_manifest_sha256"],
            IMPLEMENTATION_CANDIDATE_SCHEMA_PATH,
        )
        validate_implementation_candidate(candidate, pr_id)
        if candidate["environment_id"] == "":
            raise ValueError(f"{pr_id} candidate environment is empty")
        package_ref = binding["execution_package_ref"]
        package = load_hashed_json_artifact(REPO_ROOT, 
            package_ref["path"], package_ref["sha256"],
            ATOMIC_EXECUTION_PACKAGE_SCHEMA_PATH,
        )
        if package_ref["schema_version"] != package["schema_version"]:
            raise ValueError(f"{pr_id} execution package schema version mismatch")
        validate_atomic_execution_package(
            package, binding, template, pr_parent_work_id[pr_id], candidate
        )
        expected_bom_state_path = BOM_TRANSITIONS_BY_WORK_AND_TYPE.get(
            (pr_parent_work_id[pr_id], template["pr_type"])
        )
        bom_ref = binding["bom_transition_ref"]
        if (expected_bom_state_path is None) != (bom_ref is None):
            raise ValueError(f"{pr_id} BOM transition reference presence differs from its task role")
        if expected_bom_state_path is not None:
            upstream_bom_refs = [
                pr_bindings[item]["bom_transition_ref"]
                for item in sorted(upstream_pr_ids_for(pr_id))
                if pr_bindings[item]["bom_transition_ref"] is not None
            ]
            validate_bom_transition(
                bom_ref, binding, candidate, expected_bom_state_path,
                upstream_bom_refs,
                [pr_bindings[item] for item in sorted(upstream_pr_ids_for(pr_id))],
                pr_id,
            )
        evidence_bindings = binding["evidence_run_bindings"]
        for evidence in evidence_bindings:
            validate_against_schema(evidence, EVIDENCE_RUN_BINDING_SCHEMA_PATH)
            if (
                evidence["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
                or evidence["profile_id"] != binding["profile_id"]
                or evidence["environment_id"] != candidate["environment_id"]
            ):
                raise ValueError(
                    f"{pr_id} evidence is bound to a different candidate/profile/environment"
                )
            evidence_manifest = load_hashed_json_artifact(REPO_ROOT, 
                evidence["manifest_path"], evidence["manifest_sha256"],
                EVIDENCE_RUN_MANIFEST_SCHEMA_PATH,
            )
            identity_fields = (
                "schema_version", "run_id", "subject_pr_id", "subject_work_id",
                "subject_milestone_id", "execution_package_sha256", "plan_kind",
                "plan_id", "plan_sha256", "bom_transition_sha256",
                "candidate_manifest_sha256", "profile_id", "environment_id",
                "time_window", "run_purpose", "gate_id", "result", "artifacts",
                "production_applied", "exclusions",
            )
            if any(evidence_manifest[field] != evidence[field] for field in identity_fields):
                raise ValueError(f"{pr_id} evidence binding differs from its run manifest")
            plan_key_by_purpose = {
                "VERIFICATION": "test",
                "ROLLBACK_REHEARSAL": "rollback",
                "OBSERVATION": "observation",
                "RECONCILIATION": "evidence",
                "EXTERNAL_ATTESTATION": "evidence",
            }
            plan_kind_by_key = {
                "test": "TEST", "evidence": "EVIDENCE",
                "rollback": "ROLLBACK", "observation": "OBSERVATION",
            }
            plan_key = plan_key_by_purpose[evidence["run_purpose"]]
            plan_ref = package["plan_refs"][plan_key]
            artifact_keys = [
                (item["direction"], item["artifact_id"])
                for item in evidence["artifacts"]
            ]
            if (
                evidence["subject_pr_id"] != pr_id
                or evidence["subject_work_id"] != pr_parent_work_id[pr_id]
                or evidence["subject_milestone_id"] != pr_id[:6]
                or evidence["execution_package_sha256"] != package_ref["sha256"]
                or plan_ref is None
                or evidence["plan_kind"] != plan_kind_by_key[plan_key]
                or evidence["plan_id"] != plan_ref["plan_id"]
                or evidence["plan_sha256"] != plan_ref["sha256"]
                or evidence["bom_transition_sha256"]
                != (bom_ref["sha256"] if bom_ref is not None else None)
                or len(artifact_keys) != len(set(artifact_keys))
            ):
                raise ValueError(f"{pr_id} evidence run origin/package/plan/artifact identity mismatch")
            for artifact in evidence["artifacts"]:
                validate_hashed_artifact(REPO_ROOT, artifact["path"], artifact["sha256"])
        validate_evidence_gate_state(
            pr_id,
            template["pr_type"],
            template["produces_new_evidence"],
            template["required_gates"],
            binding["readiness_status"],
            evidence_bindings,
        )
        if binding["readiness_status"] == "PASS" and template["pr_type"] in {"WRT", "REF"}:
            if (
                not binding["postmerge_result_manifest_path"]
                or not binding["postmerge_result_manifest_sha256"]
            ):
                raise ValueError(f"{pr_id} PASS {template['pr_type']} lacks an after-state implementation result")
            implementation_result = load_hashed_json_artifact(REPO_ROOT, 
                binding["postmerge_result_manifest_path"],
                binding["postmerge_result_manifest_sha256"],
                ATOMIC_IMPLEMENTATION_RESULT_SCHEMA_PATH,
            )
            validate_atomic_implementation_result(
                implementation_result,
                pr_id=pr_id,
                pr_type=template["pr_type"],
                candidate_manifest_sha256=binding["candidate_manifest_sha256"],
                profile_id=binding["profile_id"],
                environment_id=candidate["environment_id"],
                execution_package_sha256=package_ref["sha256"],
                selected_targets=binding["selected_targets"],
                approved_verification_commands=validate_atomic_plan_ref(
                    package["plan_refs"]["test"], "TEST", pr_id,
                    binding["candidate_manifest_sha256"], binding["profile_id"],
                )["content"]["commands"],
            )
        elif template["pr_type"] in {"WRT", "REF"} and binding["postmerge_result_manifest_path"] is not None:
            raise ValueError(f"{pr_id} non-PASS implementation leaf cannot publish an after-state result")
        dependency_bindings = [pr_bindings[item] for item in template["depends_on_prs"]]
        if any(item["readiness_status"] != "PASS" for item in dependency_bindings):
            raise ValueError(f"{pr_id} has a non-PASS PR dependency")
        validate_pr_dependency_identity(pr_id, binding, dependency_bindings)
        parent_and_type = (pr_parent_work_id[pr_id], template["pr_type"])
        completion_expected = parent_and_type in MILESTONE_COMPLETION_BY_WORK_AND_TYPE
        closes_milestone = completion_expected or template["pr_type"] == "PROM"
        expected_requirement_ids = (
            contract_requirement_id_set()
            if closes_milestone and pr_id[:6] == "T1-M12"
            else set()
            if closes_milestone and pr_parent_work_id[pr_id] == "T1-M13-N010"
            else accountable_requirement_id_set(pr_id[:6]) if closes_milestone else set()
        )
        upstream_bindings = [
            pr_bindings[item] for item in sorted(upstream_pr_ids_for(pr_id))
        ]
        requirement_evidence_runs = normalized_evidence_runs(upstream_bindings, pr_id)
        anchor_pr_id: str | None = None
        if expected_requirement_ids and template["pr_type"] == "PROM":
            direct_idx_ids = [
                item for item in template["depends_on_prs"]
                if pr_by_id[item]["pr_type"] == "IDX"
                and pr_bindings[item]["milestone_completion_candidate_ref"] is not None
            ]
            if len(direct_idx_ids) != 1:
                raise ValueError(f"{pr_id} requirement closure lacks one direct completion IDX")
            anchor_pr_id = direct_idx_ids[0]
        elif expected_requirement_ids and pr_id[:6] == "T1-M12" and pr_id != CONTRACT_EVIDENCE_ANCHOR_PR_ID:
            anchor_pr_id = CONTRACT_EVIDENCE_ANCHOR_PR_ID
        if anchor_pr_id is not None:
            anchor_binding = pr_bindings[anchor_pr_id]
            if (
                anchor_pr_id not in upstream_pr_ids_for(pr_id)
                or anchor_binding["readiness_status"] != "PASS"
                or not anchor_binding["current_idx_manifest_path"]
                or not anchor_binding["current_idx_manifest_sha256"]
            ):
                raise ValueError(f"{pr_id} contract closure lacks its immutable M12 IDX anchor")
            anchor_idx = load_hashed_json_artifact(REPO_ROOT, 
                anchor_binding["current_idx_manifest_path"],
                anchor_binding["current_idx_manifest_sha256"],
                CURRENT_EVIDENCE_INDEX_SCHEMA_PATH,
            )
            requirement_evidence_runs = anchor_idx["evidence_runs"]
        requirement_manifests = validate_requirement_satisfaction_refs(
            binding["requirement_satisfaction_refs"], binding, candidate,
            expected_requirement_ids, requirement_evidence_runs,
            pr_id,
        )
        completion_ref = binding["milestone_completion_candidate_ref"]
        if completion_expected != (completion_ref is not None):
            raise ValueError(
                f"{pr_id} milestone completion candidate presence differs from its task role"
            )
        if completion_ref is not None:
            accountable_work_ids = {
                pr_parent_work_id[item]
                for item in upstream_pr_ids_for(pr_id) | {pr_id}
            }
            expected_accountable_canonical_ids: set[str] = set()
            for work_id in accountable_work_ids:
                if work_id in task_by_id:
                    expected_accountable_canonical_ids.update(
                        task_by_id[work_id]["accountable_ids"]
                    )
                elif work_id in slice_by_id:
                    expected_accountable_canonical_ids.update(
                        slice_by_id[work_id]["canonical_ids"]
                    )
            if pr_id[:6] == "T1-M12" or pr_parent_work_id[pr_id] == "T1-M13-N010":
                expected_accountable_canonical_ids = set(all_canonical_ids)
            if (
                pr_id[:6] == "T1-M12"
                or pr_parent_work_id[pr_id] == "T1-M13-N010"
            ) and not expected_accountable_canonical_ids:
                raise ValueError(
                    f"{pr_id} cannot complete before canonical accountability is assigned"
                )
            validate_milestone_completion_candidate(
                completion_ref, binding, candidate, pr_id[:6],
                upstream_pr_ids_for(pr_id), upstream_external_ids_for(pr_id),
                upstream_bindings, external_bindings, requirement_manifests,
                expected_accountable_canonical_ids,
                milestone_by_id[pr_id[:6]]["promotion_requirements"]["observation_required"]
                and pr_parent_work_id[pr_id] != "T1-M12-N006",
                pr_id,
            )
        promotion_closure_ref = binding["milestone_promotion_closure_ref"]
        if (template["pr_type"] == "PROM") != (promotion_closure_ref is not None):
            raise ValueError(
                f"{pr_id} milestone promotion closure presence differs from its PR type"
            )
        if template["pr_type"] == "IDX":
            if not binding["current_idx_manifest_path"] or not binding["current_idx_manifest_sha256"]:
                raise ValueError(f"{pr_id} IDX lacks current evidence index artifact")
            current_idx = load_hashed_json_artifact(REPO_ROOT, 
                binding["current_idx_manifest_path"],
                binding["current_idx_manifest_sha256"],
                CURRENT_EVIDENCE_INDEX_SCHEMA_PATH,
            )
            validate_current_evidence_index_state(
                pr_id, template["required_gates"], binding, candidate, current_idx
            )
            expected_bom_sha = bom_ref["sha256"] if bom_ref is not None else None
            if current_idx["bom_transition_manifest_sha256"] != expected_bom_sha:
                raise ValueError(f"{pr_id} current IDX differs from its BOM transition")
            if (
                current_idx["requirement_satisfaction_manifest_sha256s"]
                != sorted(item["sha256"] for item in binding["requirement_satisfaction_refs"])
                or current_idx["milestone_completion_candidate_sha256"]
                != (completion_ref["sha256"] if completion_ref is not None else None)
            ):
                raise ValueError(
                    f"{pr_id} current IDX differs from its requirement/milestone closure"
                )
            validate_current_idx_upstream_closure(
                pr_id,
                current_idx,
                [pr_bindings[item] for item in sorted(upstream_pr_ids_for(pr_id))],
            )
            for item in current_idx["evidence_runs"]:
                load_hashed_json_artifact(REPO_ROOT, item["manifest_path"], item["manifest_sha256"])
        terminal_task = task_terminal_by_pr.get(pr_id)
        completion_candidate_ref = binding["task_completion_candidate_ref"]
        task_current_index_ref = binding["task_current_index_ref"]
        if terminal_task is None:
            if completion_candidate_ref is not None or task_current_index_ref is not None:
                raise ValueError(f"{pr_id} non-terminal PR cannot carry task completion artifacts")
        else:
            if completion_candidate_ref is None:
                raise ValueError(f"{pr_id} terminal TASK-IDX lacks a completion candidate")
            completion_candidate = load_hashed_json_artifact(REPO_ROOT, 
                completion_candidate_ref["path"], completion_candidate_ref["sha256"],
                TASK_COMPLETION_SCHEMA_PATH,
            )
            validate_task_completion_candidate_semantics(
                completion_candidate, terminal_task, overlay["task_registry_sha256"],
                pr_bindings=pr_bindings,
                external_bindings=external_bindings,
                tasks_by_id=task_by_id,
            )
            if (
                completion_candidate["candidate_manifest_sha256"]
                != binding["candidate_manifest_sha256"]
                or completion_candidate["profile_id"] != binding["profile_id"]
                or completion_candidate["environment_id"] != candidate["environment_id"]
            ):
                raise ValueError(f"{pr_id} task completion differs from candidate/profile/environment")
            if binding["readiness_status"] == "PASS":
                if completion_candidate["result"] != "PASS" or task_current_index_ref is None:
                    raise ValueError(f"{pr_id} PASS terminal TASK-IDX lacks current task index")
                task_current_index = load_hashed_json_artifact(REPO_ROOT, 
                    task_current_index_ref["path"], task_current_index_ref["sha256"],
                    TASK_CURRENT_INDEX_SCHEMA_PATH,
                )
                validate_task_current_index_semantics(
                    task_current_index, completion_candidate, completion_candidate_ref,
                    {
                        "path": overlay["acceptance"]["decision_receipt_path"],
                        "sha256": overlay["acceptance"]["decision_receipt_sha256"],
                    },
                )
                if task_current_index["status"] != "PASS":
                    raise ValueError(f"{pr_id} PASS terminal TASK-IDX published a non-PASS current index")
            elif task_current_index_ref is not None:
                raise ValueError(f"{pr_id} non-PASS terminal TASK-IDX cannot publish current task index")
        if template["pr_type"] == "PROM":
            idx_dependencies = [
                item for item in dependency_bindings
                if pr_by_id[item["pr_id"]]["pr_type"] == "IDX"
            ]
            if not idx_dependencies:
                raise ValueError(f"{pr_id} has no direct PASS IDX binding")
            if not all(
                item["candidate_manifest_sha256"] == binding["candidate_manifest_sha256"]
                and item["profile_id"] == binding["profile_id"]
                and item["current_idx_manifest_sha256"] == binding["current_idx_manifest_sha256"]
                for item in idx_dependencies
            ):
                raise ValueError(f"{pr_id} PROM does not match its direct current IDX")
            validate_milestone_promotion_closure(
                promotion_closure_ref, binding, candidate, pr_id[:6],
                idx_dependencies, bom_ref, pr_id,
            )
            if not binding["promotion_intent_manifest_path"] or not binding["promotion_intent_manifest_sha256"]:
                raise ValueError(f"{pr_id} PROM lacks pre-merge intent")
            intent = load_hashed_json_artifact(REPO_ROOT, 
                binding["promotion_intent_manifest_path"],
                binding["promotion_intent_manifest_sha256"],
                PROMOTION_INTENT_SCHEMA_PATH,
            )
            if (
                intent["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
                or intent["profile_id"] != binding["profile_id"]
                or intent["current_idx_manifest_sha256"] != binding["current_idx_manifest_sha256"]
                or intent["bom_transition_manifest_sha256"]
                != (bom_ref["sha256"] if bom_ref is not None else None)
                or intent["milestone_promotion_closure_sha256"]
                != promotion_closure_ref["sha256"]
                or set(intent["allowed_paths"]) != set(binding["allowed_paths"])
            ):
                raise ValueError(f"{pr_id} PROM intent closure mismatch")
            if any(
                not any(path.startswith(prefix) for prefix in prom_allowed_prefixes)
                for path in binding["allowed_paths"]
            ):
                raise ValueError(f"{pr_id} PROM includes a production implementation path")
            if binding["evidence_run_bindings"]:
                raise ValueError(f"{pr_id} PROM cannot generate or attach a new run")
            if binding["readiness_status"] == "PASS":
                if not binding["postmerge_result_manifest_path"] or not binding["postmerge_result_manifest_sha256"]:
                    raise ValueError(f"{pr_id} PASS lacks post-merge result")
                result = load_hashed_json_artifact(REPO_ROOT, 
                    binding["postmerge_result_manifest_path"],
                    binding["postmerge_result_manifest_sha256"],
                    PROMOTION_RESULT_SCHEMA_PATH,
                )
                if (
                    result["promotion_intent_manifest_sha256"] != binding["promotion_intent_manifest_sha256"]
                    or result["candidate_manifest_sha256"] != binding["candidate_manifest_sha256"]
                    or result["bom_transition_manifest_sha256"]
                    != (bom_ref["sha256"] if bom_ref is not None else None)
                    or result["milestone_promotion_closure_sha256"]
                    != promotion_closure_ref["sha256"]
                ):
                    raise ValueError(f"{pr_id} post-merge result closure mismatch")
    acceptance = overlay["acceptance"]
    if any(acceptance[field] != "PASS" for field in ("dor_status", "candidate_status", "checklist_status")):
        raise ValueError("scoped execution acceptance has a BLOCKED acceptance dimension")
    if (
        not acceptance["accepted_by"]
        or not acceptance["accepted_at"]
        or not acceptance["decision_receipt_path"]
        or not acceptance["decision_receipt_sha256"]
    ):
        raise ValueError("scoped execution acceptance lacks signed decision identity")
    decision_receipt = load_hashed_json_artifact(REPO_ROOT, 
        acceptance["decision_receipt_path"],
        acceptance["decision_receipt_sha256"],
        EXECUTION_ACCEPTANCE_RECEIPT_SCHEMA_PATH,
    )
    authorization_body = json.loads(canonical_json(overlay))
    authorization_body["acceptance"]["decision_receipt_path"] = None
    authorization_body["acceptance"]["decision_receipt_sha256"] = None
    authorization_body_sha256 = sha256_bytes(
        canonical_json(authorization_body).encode("utf-8")
    )
    if (
        decision_receipt["instance_id"] != overlay["instance_id"]
        or
        decision_receipt["task_registry_sha256"] != overlay["task_registry_sha256"]
        or decision_receipt["milestone_registry_sha256"] != overlay["milestone_registry_sha256"]
        or decision_receipt["scope_sha256"]
        != sha256_bytes(canonical_json(scope).encode("utf-8"))
        or set(decision_receipt["atomic_pr_ids"]) != scoped_pr_ids
        or sorted(decision_receipt["accepted_by"]) != sorted(acceptance["accepted_by"])
        or decision_receipt["accepted_at"] != acceptance["accepted_at"]
        or decision_receipt["execution_authorization_body_sha256"]
        != authorization_body_sha256
        or decision_receipt["signed_payload_sha256"] != authorization_body_sha256
    ):
        raise ValueError("scoped execution acceptance receipt identity/scope mismatch")
    validate_hashed_artifact(REPO_ROOT, 
        decision_receipt["signed_payload_artifact"],
        decision_receipt["signed_payload_sha256"],
    )
    for signature in decision_receipt["signature_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, signature["path"], signature["sha256"])
    require_trusted_signature_verifier("scoped execution acceptance")
    milestone_binding_by_id = {
        item["milestone_id"]: item for item in overlay["milestone_bindings"]
    }
    milestone_profile_by_id = {
        item["milestone_id"]: item["promotion_requirements"]["profile"]
        for item in milestones["milestones"]
    }
    for milestone_id in scope["milestone_ids"]:
        validate_milestone_profile_identity(
            milestone_id,
            milestone_binding_by_id[milestone_id]["profile_id"],
            milestone_profile_by_id[milestone_id],
        )
    invalid_milestones = [
        milestone_id
        for milestone_id in scope["milestone_ids"]
        if milestone_id not in milestone_binding_by_id
        or milestone_binding_by_id[milestone_id]["readiness_status"] != "READY"
        or milestone_binding_by_id[milestone_id]["blockers"]
        or not milestone_binding_by_id[milestone_id]["owner"]
        or not milestone_binding_by_id[milestone_id]["reviewers"]
        or not milestone_binding_by_id[milestone_id]["approvers"]
        or not isinstance(milestone_binding_by_id[milestone_id]["candidate_manifest_sha256"], str)
        or not re.fullmatch(
            r"[0-9a-f]{64}", milestone_binding_by_id[milestone_id]["candidate_manifest_sha256"]
        )
        or not milestone_binding_by_id[milestone_id]["profile_id"]
        or milestone_binding_by_id[milestone_id]["profile_id"]
        != milestone_profile_by_id[milestone_id]
    ]
    if invalid_milestones:
        raise ValueError(f"scoped execution acceptance has non-ready milestones {invalid_milestones}")
    for pr_id in sorted(scoped_pr_ids):
        milestone_id = pr_id[:6]
        milestone_binding = milestone_binding_by_id[milestone_id]
        pr_binding = pr_bindings[pr_id]
        if (
            pr_binding["candidate_manifest_sha256"]
            != milestone_binding["candidate_manifest_sha256"]
            or pr_binding["profile_id"] != milestone_binding["profile_id"]
        ):
            raise ValueError(
                f"{pr_id} candidate/profile differs from scoped milestone binding"
            )


def validate_requirement_source_and_approval(payload: dict[str, Any]) -> None:
    for source in payload["source_documents"]:
        source_path = (REPO_ROOT / source["path"]).resolve()
        if not source_path.is_relative_to(REPO_ROOT) or not source_path.is_file():
            raise ValueError(f"requirement source is missing or outside repository: {source['path']}")
        if sha256_bytes(source_path.read_bytes()) != source["sha256"]:
            raise ValueError(f"requirement source hash drift: {source['path']}")
    requirement_ids_seen = [item["requirement_id"] for item in payload["requirements"]]
    if len(requirement_ids_seen) != len(set(requirement_ids_seen)):
        raise ValueError("requirement registry has duplicate requirement IDs")
    expected_accountability = {
        "REQ-T1-SYS-001": "T1-M12",
        "REQ-T1-DATA-CAPTURE-001": "T1-M02",
        "REQ-T1-DATA-PARSE-001": "T1-M03",
        "REQ-T1-FILE-RESTORE-001": "T1-M09",
        "REQ-T1-ENCRYPTED-001": "T1-M09",
        "REQ-T1-DATA-FOUR-SOURCE-001": "T1-M06",
        "REQ-T1-FUSION-001": "T1-M07",
        "REQ-T1-BASELINE-001": "T1-M07",
        "REQ-T1-ATTACKCHAIN-001": "T1-M07",
        "REQ-T1-AI-001": "T1-M08",
        "REQ-T1-GNN-001": "T1-M08",
        "REQ-T1-DET-MIDTERM-001": "T1-M04",
        "REQ-T1-QUAL-001": "T1-M11",
        "REQ-T1-EVI-001": "T1-M09",
        "REQ-T1-SYS-DEPLOY-001": "T1-M10",
        "REQ-T1-RELEASE-MIDTERM-001": "T1-M05",
        "REQ-T1-INTERNAL-STRENGTHENING-001": "T1-M13",
    }
    actual_accountability = {
        item["requirement_id"]: item["accountable_milestone"]
        for item in payload["requirements"]
    }
    if actual_accountability != expected_accountability or any(
        item["accountable_milestone"] not in item["related_milestones"]
        or item["satisfaction_manifest_schema"]
        != "contracts/alignment/requirement-satisfaction.schema.json"
        for item in payload["requirements"]
    ):
        raise ValueError("requirement accountability/satisfaction-contract mapping drift")
    if any(
        item["status"] != "APPROVED"
        and item["satisfaction_state"] != "NOT_STARTED"
        for item in payload["requirements"]
    ):
        raise ValueError("unapproved requirement cannot enter implementation/satisfaction state")
    if payload["status"] == "DRAFT_PENDING_SIGNATURE":
        if payload["approval_intake"] is not None:
            raise ValueError("draft requirement registry cannot carry an approval intake")
        return
    if payload["status"] != "APPROVED":
        raise ValueError("active requirement registry cannot be SUPERSEDED")
    intake = payload["approval_intake"]
    if not isinstance(intake, dict):
        raise ValueError("approved requirement registry lacks a signed intake")
    body = dict(payload)
    body["approval_intake"] = None
    body_sha256 = sha256_bytes(canonical_json(body).encode("utf-8"))
    receipt = validate_signed_contract_intake(
        intake["receipt_path"],
        intake["receipt_sha256"],
        "REQUIREMENTS_APPROVAL",
        REQUIREMENT_PATH.relative_to(REPO_ROOT).as_posix(),
        body_sha256,
    )
    if (
        intake["signature_verification"] != "PASS"
        or len(intake["authorities"]) < 2
        or sorted(intake["authorities"])
        != sorted(item["identity"] for item in receipt["authorities"])
    ):
        raise ValueError("approved requirement registry lacks verified authorities")
    if any(item["status"] != "APPROVED" for item in payload["requirements"]):
        raise ValueError("approved requirement registry contains a non-approved requirement")


def validate_evidence_contract_registry(
    payload: dict[str, Any], requirement_payload: dict[str, Any],
) -> dict[str, dict[str, Any]]:
    validate_against_schema(payload, EVIDENCE_CONTRACT_SCHEMA_PATH)
    if payload["subject_requirements_sha256"] != sha256_bytes(REQUIREMENT_PATH.read_bytes()):
        raise ValueError("evidence contract registry is bound to a different requirement registry")
    contracts = payload["contracts"]
    contract_ids = [item["requirement_id"] for item in contracts]
    expected_ids = {item["requirement_id"] for item in requirement_payload["requirements"]}
    if len(contract_ids) != len(set(contract_ids)) or set(contract_ids) != expected_ids:
        raise ValueError("evidence contract registry does not exactly cover the 17 requirements")
    by_id = {item["requirement_id"]: item for item in contracts}
    for requirement_id, contract in by_id.items():
        if set(contract["required_gates"]) != REQUIREMENT_REQUIRED_GATES[requirement_id]:
            raise ValueError(f"{requirement_id} evidence contract gate set drift")
        input_ids = [item["artifact_id"] for item in contract["required_inputs"]]
        output_ids = [item["artifact_id"] for item in contract["required_outputs"]]
        if (
            len(input_ids) != len(set(input_ids))
            or len(output_ids) != len(set(output_ids))
            or set(input_ids) & set(output_ids)
        ):
            raise ValueError(f"{requirement_id} evidence artifact identities are not directional/unique")
        for artifact in contract["required_inputs"] + contract["required_outputs"]:
            schema_path = artifact["schema_ref"].split("#", 1)[0]
            resolved = (REPO_ROOT / schema_path).resolve()
            if not resolved.is_relative_to(REPO_ROOT) or not resolved.is_file():
                raise ValueError(f"{requirement_id} evidence artifact schema is missing: {schema_path}")
    if payload["status"] == "DRAFT_PENDING_SIGNATURE":
        if payload["approval_intake"] is not None:
            raise ValueError("draft evidence contract registry cannot carry an approval intake")
        return by_id
    if (
        requirement_payload["status"] != "APPROVED"
        or any(item["status"] != "APPROVED" for item in requirement_payload["requirements"])
    ):
        raise ValueError("evidence contracts cannot be signed before requirements approval")
    body = dict(payload)
    body["approval_intake"] = None
    body_sha256 = sha256_bytes(canonical_json(body).encode("utf-8"))
    intake = payload["approval_intake"]
    if not isinstance(intake, dict):
        raise ValueError("signed evidence contract registry lacks an approval intake")
    validate_signed_contract_intake(
        intake["receipt_path"], intake["receipt_sha256"],
        "EVIDENCE_CONTRACT_SIGNATURE",
        EVIDENCE_CONTRACT_PATH.relative_to(REPO_ROOT).as_posix(), body_sha256,
    )
    return by_id


def validate_metric_method_semantics(payload: dict[str, Any]) -> None:
    expected = {
        "T1-MIDTERM-ALERT-ACCURACY": ("MIDTERM", ">=", 50),
        "T1-FINAL-ALERT-ACCURACY": ("FINAL", ">=", 95),
        "T1-FINAL-FALSE-ALARM-RATE": ("FINAL", "<", 5),
    }
    methods = payload["methods"]
    method_by_id = {item["method_id"]: item for item in methods}
    if len(method_by_id) != 2 or set(method_by_id) != {
        "T1-MIDTERM-KNOWN-ALERT-METHOD", "T1-FINAL-CNAS-QUALITY-METHOD",
    }:
        raise ValueError("metric registry must contain the independent midterm and final methods")
    midterm = method_by_id["T1-MIDTERM-KNOWN-ALERT-METHOD"]
    final = method_by_id["T1-FINAL-CNAS-QUALITY-METHOD"]
    if midterm["stage"] != "MIDTERM" or final["stage"] != "FINAL":
        raise ValueError("metric method stage identity drift")
    flattened_metrics = [item for method in methods for item in method["metrics"]]
    actual = {
        item["metric_id"]: (item["stage"], item["operator"], item["target_percent"])
        for item in flattened_metrics
    }
    if len(flattened_metrics) != 3 or len(actual) != 3 or actual != expected:
        raise ValueError(f"metric identity/stage/operator/target drift: {actual}")
    if len(midterm["metrics"]) != 1 or len(final["metrics"]) != 2 or {item["metric_id"] for item in midterm["metrics"]} != {
        "T1-MIDTERM-ALERT-ACCURACY"
    } or {item["metric_id"] for item in final["metrics"]} != {
        "T1-FINAL-ALERT-ACCURACY", "T1-FINAL-FALSE-ALARM-RATE"
    }:
        raise ValueError("midterm and final metric sets are not stage-isolated")
    for method in methods:
        status = method["method_status"]
        if status == "PENDING_SIGNATURE":
            if method["signed_intake"] is not None:
                raise ValueError("pending metric method cannot carry a signed intake")
            if any(
                item["formula_status"] != "UNRESOLVED" or item["signed_formula"] is not None
                for item in method["metrics"]
            ):
                raise ValueError("pending metric method contains a signed formula")
            if method["threshold_lock"]["status"] != "PENDING" or any(
                value is not None
                for key, value in method["threshold_lock"].items()
                if key != "status"
            ):
                raise ValueError("pending metric method contains a locked candidate/threshold")
            continue
        if status != "SIGNED":
            raise ValueError("active metric registry cannot use a SUPERSEDED method")
        authority = method["authority"]
        role_names = ["project_owner", "algorithm_owner", "qa_owner"]
        role_names.append(
            "acceptance_owner" if method["stage"] == "MIDTERM" else "external_lab"
        )
        authority_identities = [authority[field] for field in role_names]
        population = method["population"]
        locked_hashes = [
            method["threshold_lock"][field]
            for field in (
                "candidate_hash", "model_hash", "feature_hash", "threshold_hash",
                "dataset_manifest_hash",
            )
        ]
        if (
            any(not isinstance(identity, str) or not identity for identity in authority_identities)
            or len(set(authority_identities)) != len(authority_identities)
            or not set(authority_identities).issubset(set(authority["signature_set"]))
            or any(
                not isinstance(population[field], str) or not population[field]
                for field in (
                    "analysis_unit", "dedup_window", "abstain_policy",
                    "invalid_sample_policy", "label_arbitration",
                )
            )
            or not population["classes"]
            or not population["strata"]
            or not population["minimum_sample_rules"]
            or any(
                item["formula_status"] != "SIGNED"
                or not isinstance(item["signed_formula"], str)
                or not item["signed_formula"]
                for item in method["metrics"]
            )
            or method["threshold_lock"]["status"] != "LOCKED"
            or any(
                not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{64}", value)
                for value in locked_hashes
            )
        ):
            raise ValueError(
                f"SIGNED {method['stage']} method lacks complete stage authorities, "
                "population, formulas, sample rules, or locked hashes"
            )
        intake = method["signed_intake"]
        if not isinstance(intake, dict):
            raise ValueError("signed metric method lacks signed intake")
        body = dict(method)
        body["signed_intake"] = None
        body_sha256 = sha256_bytes(canonical_json(body).encode("utf-8"))
        if body_sha256 != intake["method_sha256"]:
            raise ValueError("signed metric method body hash mismatch")
        validate_signed_contract_intake(
            intake["receipt_path"],
            intake["receipt_sha256"],
            "METRIC_METHOD_SIGNATURE",
            METRIC_METHOD_PATH.relative_to(REPO_ROOT).as_posix(),
            body_sha256,
        )


def run_fail_closed_validator_self_tests(metric_method_payload: dict[str, Any]) -> None:
    def must_reject(label: str, action: Any) -> None:
        try:
            action()
        except ValueError:
            return
        raise ValueError(f"fail-closed validator self-test unexpectedly accepted {label}")

    implementation_result = {
        "artifact_kind": "ATOMIC_PR_IMPLEMENTATION_RESULT",
        "schema_version": "1.0.0",
        "atomic_pr_id": "T1-M00-P001-WRT-selftest",
        "pr_type": "WRT",
        "candidate_manifest_sha256": "a" * 64,
        "profile_id": "SELFTEST",
        "environment_id": "selftest",
        "execution_package_sha256": "b" * 64,
        "implementation_delta_ref": {"path": "doc/selftest-delta.json", "sha256": "9" * 64},
        "after_units": [{
            "target_id": "TARGET-001",
            "path": "go/control-plane/internal/selftest/example.go",
            "locator_kind": "go_symbol",
            "qualified_symbol": "selftest.Example",
            "signature_after": "func Example() error",
            "candidate_blob_sha256": "c" * 64,
            "ast_node_sha256": "d" * 64,
            "resolver_receipt": {"path": "doc/selftest-locator.json", "sha256": "e" * 64},
        }],
        "after_artifacts": [],
        "verification_checks": [{
            "command_id": "CHECK-COMPILE",
            "command": "go test ./go/control-plane/internal/selftest",
            "command_sha256": sha256_bytes(
                b"go test ./go/control-plane/internal/selftest"
            ),
            "exit_code": 0,
            "matched_target_ids": ["TARGET-001"],
            "status": "PASS",
        }],
        "result": "PASS",
        "blockers": [],
        "proof_ceiling": "EXACT_AFTER_IMPLEMENTATION_ONLY_NOT_TEST_RUNTIME_PARENT_OR_PRODUCTION_ACCEPTANCE",
    }
    implementation_args = {
        "pr_id": "T1-M00-P001-WRT-selftest",
        "pr_type": "WRT",
        "candidate_manifest_sha256": "a" * 64,
        "profile_id": "SELFTEST",
        "environment_id": "selftest",
        "execution_package_sha256": "b" * 64,
        "selected_targets": [{
            "path": "go/control-plane/internal/selftest/example.go",
            "symbol": "selftest.Example",
            "approved_by": "selftest",
        }],
        "resolve_files": False,
        "approved_verification_commands": [
            "go test ./go/control-plane/internal/selftest"
        ],
    }
    validate_atomic_implementation_result(implementation_result, **implementation_args)
    missing_target = json.loads(canonical_json(implementation_result))
    missing_target["after_units"] = []
    must_reject(
        "PASS WRT result dropping its selected after-state target",
        lambda: validate_atomic_implementation_result(missing_target, **implementation_args),
    )
    uncovered_target = json.loads(canonical_json(implementation_result))
    uncovered_target["verification_checks"][0]["matched_target_ids"] = ["TARGET-999"]
    must_reject(
        "PASS WRT result with a compile check unrelated to its target",
        lambda: validate_atomic_implementation_result(uncovered_target, **implementation_args),
    )
    duplicate_coverage = json.loads(canonical_json(implementation_result))
    duplicate_coverage["verification_checks"].append({
        **duplicate_coverage["verification_checks"][0],
        "command_id": "CHECK-COMPILE-AGAIN",
    })
    must_reject(
        "PASS WRT result claiming the same target from two checks",
        lambda: validate_atomic_implementation_result(duplicate_coverage, **implementation_args),
    )
    forged_command = json.loads(canonical_json(implementation_result))
    forged_command["verification_checks"][0]["command"] = "true"
    must_reject(
        "PASS WRT result carrying a command hash unrelated to its command",
        lambda: validate_atomic_implementation_result(forged_command, **implementation_args),
    )

    must_reject(
        "bare PASS evidence report with zero declared cases",
        lambda: validate_case_report_semantics(
            {
                "cases": [],
                "summary": {
                    "expected_case_count": 0,
                    "passed_case_count": 0,
                    "failed_case_count": 0,
                },
                "result": "PASS",
                "positive_attestation": None,
            },
            ({
                "case_id": "required-case",
                "outcome": "BLOCKED",
                "fixture_path": "agent.md",
                "fixture_payload": {},
            },),
            require_positive_attestation=False,
        ),
    )

    runner_path = "agent.md"
    runner_sha = sha256_bytes((REPO_ROOT / runner_path).read_bytes())
    case_id = "required-case"
    rejection_code = registered_case_rejection_code(case_id, "BLOCKED")
    subject_pr = "T1-M00-P001-TST-PRE-selftest"
    with tempfile.TemporaryDirectory(
        prefix=".topic1-evidence-selftest-", dir=REPO_ROOT,
    ) as temp_dir:
        suite_id = "T1-M00-N001::selftest-negative-run::TST-PRE"
        fixture_file = Path(temp_dir) / "required-case.json"
        fixture_rel = fixture_file.relative_to(REPO_ROOT).as_posix()
        fixture_payload = registered_case_fixture_payload(
            suite_id, case_id, "BLOCKED", (runner_path,),
        )
        fixture_file.write_text(
            json.dumps(fixture_payload, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        fixture_sha = sha256_bytes(fixture_file.read_bytes())
        expected_case_specs = ({
            "case_id": case_id,
            "outcome": "BLOCKED",
            "fixture_path": fixture_rel,
            "fixture_payload": fixture_payload,
        },)
        case_input = [fixture_sha]
        case_output_fact = {
            "case_id": case_id,
            "expected_outcome": "BLOCKED",
            "actual_outcome": "BLOCKED",
            "status": "PASS",
            "rejection_code": rejection_code,
            "fixture_artifact_id": f"FIXTURE-{case_id}",
            "input_sha256s": case_input,
        }
        case_output_sha = sha256_bytes(
            canonical_json(case_output_fact).encode("utf-8")
        )
        report_path = Path(temp_dir) / "case-report.json"
        manifest_path = Path(temp_dir) / "test-result.json"
        report_rel = report_path.relative_to(REPO_ROOT).as_posix()
        report = {
            "schema_version": "1.0.0",
            "subject_pr_id": subject_pr,
            "subject_work_id": "T1-M00-N001",
            "subject_milestone_id": "T1-M00",
            "candidate_manifest_sha256": "a" * 64,
            "profile_id": "SELFTEST",
            "environment_id": "selftest",
            "runner_artifact": {"path": runner_path, "sha256": runner_sha},
            "fixture_artifacts": [{
                "artifact_id": f"FIXTURE-{case_id}",
                "path": fixture_rel,
                "sha256": fixture_sha,
            }],
            "cases": [{
                "case_id": case_id,
                "expected_outcome": "BLOCKED",
                "actual_outcome": "BLOCKED",
                "status": "PASS",
                "rejection_code": rejection_code,
                "input_sha256s": case_input,
                "output_sha256s": [case_output_sha],
            }],
            "summary": {
                "expected_case_count": 1,
                "passed_case_count": 1,
                "failed_case_count": 0,
            },
            "result": "PASS",
            "positive_attestation": None,
        }
        report_path.write_text(
            json.dumps(report, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        report_sha = sha256_bytes(report_path.read_bytes())
        manifest = {
            "schema_version": "1.0.0",
            "run_id": "SELFTEST-WORK-ORDER-EVIDENCE",
            "subject_pr_id": subject_pr,
            "subject_work_id": "T1-M00-N001",
            "subject_milestone_id": "T1-M00",
            "execution_package_sha256": "b" * 64,
            "plan_kind": "TEST",
            "plan_id": "SELFTEST-PLAN",
            "plan_sha256": "c" * 64,
            "bom_transition_sha256": None,
            "candidate_manifest_sha256": "a" * 64,
            "profile_id": "SELFTEST",
            "environment_id": "selftest",
            "time_window": "selftest",
            "run_purpose": "VERIFICATION",
            "gate_id": "G0",
            "result": "PASS",
            "artifacts": [
                {
                    "direction": "INPUT",
                    "artifact_id": f"RUNNER-{subject_pr}",
                    "path": runner_path,
                    "sha256": runner_sha,
                    "schema_ref": None,
                },
                {
                    "direction": "INPUT",
                    "artifact_id": f"FIXTURE-{case_id}",
                    "path": fixture_rel,
                    "sha256": fixture_sha,
                    "schema_ref": None,
                },
                {
                    "direction": "OUTPUT",
                    "artifact_id": f"CASE-REPORT-{subject_pr}",
                    "path": report_rel,
                    "sha256": report_sha,
                    "schema_ref": "contracts/alignment/evidence-case-report.schema.json",
                },
            ],
            "production_applied": False,
            "exclusions": [],
        }
        manifest_path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        validate_work_order_evidence_run(
            manifest_path,
            report_path,
            {
                "atomic_pr_id": subject_pr,
                "parent_work_id": "T1-M00-N001",
                "milestone_id": "T1-M00",
                "pr_type": "TST-PRE",
                "required_gates": ["G0"],
                "outcome": {"subject": "selftest-negative-run"},
                "generated_outputs": [{
                    "artifact_id": f"CASE-REPORT-{subject_pr}",
                    "path": report_rel,
                }],
            },
            expected_case_specs,
        )
        wrong_rejection_report = {
            **report,
            "cases": [{
                **report["cases"][0],
                "rejection_code": "REJECT_TOTALLY_WRONG",
            }],
        }
        must_reject(
            "case report with a non-registered rejection code",
            lambda: validate_case_report_semantics(
                wrong_rejection_report,
                expected_case_specs,
                require_positive_attestation=False,
            ),
        )
        wrong_output_report = {
            **report,
            "cases": [{
                **report["cases"][0],
                "output_sha256s": ["0" * 64],
            }],
        }
        must_reject(
            "case report whose output hash is not bound to its canonical result",
            lambda: validate_case_report_semantics(
                wrong_output_report,
                expected_case_specs,
                require_positive_attestation=False,
            ),
        )
        wrong_fixture_report = json.loads(json.dumps(report))
        wrong_fixture_report["fixture_artifacts"][0]["path"] = runner_path
        wrong_fixture_report["fixture_artifacts"][0]["sha256"] = runner_sha
        wrong_fixture_report["cases"][0]["input_sha256s"] = [runner_sha]
        wrong_fixture_fact = {
            **case_output_fact,
            "input_sha256s": [runner_sha],
        }
        wrong_fixture_report["cases"][0]["output_sha256s"] = [
            sha256_bytes(canonical_json(wrong_fixture_fact).encode("utf-8"))
        ]
        must_reject(
            "unrelated repository file relabelled as a registered case fixture",
            lambda: validate_case_report_semantics(
                wrong_fixture_report,
                expected_case_specs,
                require_positive_attestation=False,
            ),
        )

    incomplete_signed_method = json.loads(json.dumps(metric_method_payload))
    incomplete_signed_method["methods"][0]["method_status"] = "SIGNED"
    must_reject(
        "SIGNED metric method with unresolved formula/authority/threshold",
        lambda: validate_metric_method_semantics(incomplete_signed_method),
    )
    duplicate_metric_method = json.loads(json.dumps(metric_method_payload))
    duplicate_metric_method["methods"][0]["metrics"].append(
        {**duplicate_metric_method["methods"][0]["metrics"][0], "display_name": "duplicate"}
    )
    must_reject(
        "duplicate metric ID hidden behind a different display name",
        lambda: validate_metric_method_semantics(duplicate_metric_method),
    )
    must_reject(
        "milestone result list using last-wins duplicate identities",
        lambda: unique_result_map(
            [
                {"id": "REQ-T1-SYS-001", "manifest_sha256": "a" * 64, "result": "FAIL"},
                {"id": "REQ-T1-SYS-001", "manifest_sha256": "b" * 64, "result": "SATISFIED"},
            ],
            "SELFTEST-COMPLETION-DUPLICATE",
        ),
    )
    must_reject(
        "observation-required milestone declaring NOT_REQUIRED",
        lambda: validate_completion_run_bindings(
            "rollback-run",
            {
                "subject_pr_id": "T1-M12-P008-OPS-n008-s1",
                "execution_package_sha256": "a" * 64,
                "plan_id": "rollback-plan", "plan_sha256": "b" * 64,
                "bom_transition_sha256": None,
            },
            {
                "window": "T+0..T+1", "status": "NOT_REQUIRED", "run_id": None,
                "plan_ref": None,
            },
            True,
            [{
                "run_id": "rollback-run", "run_purpose": "ROLLBACK_REHEARSAL",
                "result": "PASS", "time_window": "rollback-window",
                "subject_milestone_id": "T1-M12",
                "subject_pr_id": "T1-M12-P008-OPS-n008-s1",
                "execution_package_sha256": "a" * 64,
                "plan_id": "rollback-plan", "plan_sha256": "b" * 64,
                "bom_transition_sha256": None,
            }],
            "T1-M12",
            {"T1-M12-P008-OPS-n008-s1"},
            "SELFTEST-COMPLETION-OBSERVATION",
        ),
    )
    must_reject(
        "milestone completion borrowing rollback and observation from unrelated PRs",
        lambda: validate_completion_run_bindings(
            "old-rollback",
            {
                "subject_pr_id": "T1-M01-P001-OPS-old",
                "execution_package_sha256": "c" * 64,
                "plan_id": "old-rollback-plan", "plan_sha256": "d" * 64,
                "bom_transition_sha256": None,
            },
            {
                "window": "old-window", "status": "PASS", "run_id": "old-observe",
                "plan_ref": {
                    "subject_pr_id": "T1-M02-P001-OPS-old",
                    "execution_package_sha256": "e" * 64,
                    "plan_id": "old-observe-plan", "plan_sha256": "f" * 64,
                    "bom_transition_sha256": None,
                },
            },
            True,
            [
                {
                    "run_id": "old-rollback", "run_purpose": "ROLLBACK_REHEARSAL",
                    "result": "PASS", "time_window": "old-window",
                    "subject_milestone_id": "T1-M01",
                    "subject_pr_id": "T1-M01-P001-OPS-old",
                    "execution_package_sha256": "c" * 64,
                    "plan_id": "old-rollback-plan", "plan_sha256": "d" * 64,
                    "bom_transition_sha256": None,
                },
                {
                    "run_id": "old-observe", "run_purpose": "OBSERVATION",
                    "result": "PASS", "time_window": "old-window",
                    "subject_milestone_id": "T1-M02",
                    "subject_pr_id": "T1-M02-P001-OPS-old",
                    "execution_package_sha256": "e" * 64,
                    "plan_id": "old-observe-plan", "plan_sha256": "f" * 64,
                    "bom_transition_sha256": None,
                },
            ],
            "T1-M12", {"T1-M12-P008-OPS-n008-s1"},
            "SELFTEST-COMPLETION-OLD-RUN",
        ),
    )
    method_by_id = {
        item["method_id"]: item for item in metric_method_payload["methods"]
    }
    must_reject(
        "midterm requirement bound to the final CNAS metric method",
        lambda: validate_requirement_method_identity(
            "REQ-T1-DET-MIDTERM-001",
            ["T1-MIDTERM-ALERT-ACCURACY"],
            "T1-FINAL-CNAS-QUALITY-METHOD",
            method_by_id["T1-FINAL-CNAS-QUALITY-METHOD"],
            "SELFTEST-METRIC-STAGE",
        ),
    )
    head = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=REPO_ROOT,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
    ).stdout.decode("ascii").strip()
    must_reject(
        "candidate fingerprint excluding an entire production source root",
        lambda: candidate_tree_fingerprint(REPO_ROOT, 
            head, sorted(REQUIRED_CANDIDATE_SOURCE_ROOTS),
            [{
                "path": "go/control-plane", "reason": "invalid whole-root exclusion",
                "referenced_by_active_build": False,
            }],
        ),
    )
    signed_contract_over_draft_requirements = json.loads(
        EVIDENCE_CONTRACT_PATH.read_text(encoding="utf-8")
    )
    signed_contract_over_draft_requirements["status"] = "SIGNED"
    signed_contract_over_draft_requirements["approval_intake"] = {
        "receipt_path": "contracts/alignment/not-reached.json",
        "receipt_sha256": "0" * 64,
    }
    must_reject(
        "signed evidence contract over a draft requirement registry",
        lambda: validate_evidence_contract_registry(
            signed_contract_over_draft_requirements,
            json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8")),
        ),
    )
    inspected_path = "agent.md"
    inspected_blob = read_candidate_blob(REPO_ROOT, head, inspected_path)
    if inspected_blob is None:
        raise ValueError("self-test cannot resolve tracked agent.md from HEAD")
    inspected_sha = sha256_bytes(inspected_blob)
    validate_atomic_locator(
        {
            "locator_id": "SELFTEST-FILE-POSITIVE",
            "locator_kind": "file",
            "path": inspected_path,
            "target_state": "EXISTING",
            "symbol_or_pointer": None,
            "signature": None,
            "role": "selftest",
            "production_surface": False,
            "candidate_blob_sha256": inspected_sha,
            "created_by_atomic_pr_id": None,
            "creation_reason": None,
            "compatibility_entrypoint_locator_id": None,
            "activation_guard_locator_id": None,
        },
        {},
        "T1-M01-P001-CTR-selftest",
        head,
    )
    must_reject(
        "non-file locator using an uninstalled/fake symbol resolver",
        lambda: validate_atomic_locator(
            {
                "locator_id": "SELFTEST-FAKE-SYMBOL",
                "locator_kind": "go_symbol",
                "path": "go/control-plane/cmd/alert-projection-reconcile/main.go",
                "target_state": "EXISTING",
                "symbol_or_pointer": "Fake.Type",
                "signature": "func Fake()",
                "role": "selftest",
                "production_surface": True,
                "candidate_blob_sha256": sha256_bytes(
                    read_candidate_blob(REPO_ROOT, 
                        head, "go/control-plane/cmd/alert-projection-reconcile/main.go"
                    ) or b""
                ),
                "created_by_atomic_pr_id": None,
                "creation_reason": None,
                "compatibility_entrypoint_locator_id": None,
                "activation_guard_locator_id": None,
            },
            {},
            "T1-M01-P001-CTR-selftest",
            head,
        ),
    )
    must_reject(
        "production Go source disguised as a file locator",
        lambda: validate_atomic_locator(
            {
                "locator_id": "SELFTEST-GO-AS-FILE",
                "locator_kind": "file",
                "path": "go/control-plane/cmd/alert-projection-reconcile/main.go",
                "target_state": "EXISTING",
                "symbol_or_pointer": None,
                "signature": None,
                "role": "selftest",
                "production_surface": True,
                "candidate_blob_sha256": sha256_bytes(
                    read_candidate_blob(REPO_ROOT, 
                        head, "go/control-plane/cmd/alert-projection-reconcile/main.go"
                    ) or b""
                ),
                "created_by_atomic_pr_id": None,
                "creation_reason": None,
                "compatibility_entrypoint_locator_id": None,
                "activation_guard_locator_id": None,
            },
            {},
            "T1-M01-P001-CTR-selftest",
            head,
        ),
    )
    planned_without_guard = {
        "locator_id": "SELFTEST-PLANNED",
        "locator_kind": "go_symbol",
        "path": "go/control-plane/internal/selftest/planned_does_not_exist.go",
        "target_state": "PLANNED",
        "symbol_or_pointer": "selftest.Planned",
        "signature": "func Planned()",
        "role": "selftest",
        "production_surface": True,
        "candidate_blob_sha256": None,
        "created_by_atomic_pr_id": "T1-M01-P001-CTR-selftest",
        "creation_reason": "selftest",
        "compatibility_entrypoint_locator_id": None,
        "activation_guard_locator_id": None,
    }
    must_reject(
        "planned production locator without compatibility entrypoint/activation guard",
        lambda: validate_atomic_locator(
            planned_without_guard,
            {planned_without_guard["locator_id"]: planned_without_guard},
            "T1-M01-P001-CTR-selftest",
            head,
        ),
    )
    planned_top_level_source = dict(planned_without_guard)
    planned_top_level_source.update({
        "locator_id": "SELFTEST-PLANNED-TOPLEVEL",
        "path": "planned_top_level_escape.go",
        "locator_kind": "file",
        "symbol_or_pointer": None,
        "signature": None,
        "production_surface": False,
    })
    must_reject(
        "planned top-level Go source disguised as a non-production file",
        lambda: validate_atomic_locator(
            planned_top_level_source,
            {planned_top_level_source["locator_id"]: planned_top_level_source},
            "T1-M01-P001-CTR-selftest",
            head,
        ),
    )
    must_reject(
        "negative JSON array index locator",
        lambda: resolve_json_pointer_from_blob(b'{"items":["first","last"]}', "/items/-1"),
    )
    must_reject(
        "free-form or missing plan artifact",
        lambda: validate_atomic_plan(
            {
                "plan_id": "x",
                "path": "contracts/alignment/does-not-exist-plan.json",
                "sha256": "0" * 64,
            },
            "TEST",
            "T1-M01-P001-CTR-selftest",
            "a" * 64,
            "SELFTEST",
        ),
    )
    pass_run = {"gate_id": "G2", "result": "PASS"}
    fail_run = {"gate_id": "G2", "result": "FAIL"}
    wrong_gate_run = {"gate_id": "G1", "result": "PASS"}
    must_reject(
        "FAIL evidence satisfying a PASS leaf",
        lambda: validate_evidence_gate_state(
            "SELFTEST-PASS-FAIL", "TST-POST", True, ["G2"], "PASS", [fail_run]
        ),
    )
    must_reject(
        "wrong gate satisfying required_gates",
        lambda: validate_evidence_gate_state(
            "SELFTEST-WRONG-GATE", "TST-POST", True, ["G2"], "PASS", [wrong_gate_run]
        ),
    )
    must_reject(
        "READY evidence producer claiming an already completed run",
        lambda: validate_evidence_gate_state(
            "SELFTEST-READY-RUN", "TST-POST", True, ["G2"], "READY", [pass_run]
        ),
    )
    must_reject(
        "external PASS bridging over a READY PR predecessor",
        lambda: validate_external_predecessor_states(
            "SELFTEST-EXTERNAL",
            {"depends_on_prs": ["SELFTEST-PR"], "depends_on_external_activities": []},
            {"SELFTEST-PR": {"readiness_status": "READY"}},
            {},
        ),
    )
    must_reject(
        "IDX omitting the evidence run bound by the execution instance",
        lambda: validate_current_evidence_index_state(
            "SELFTEST-IDX",
            ["G2"],
            {
                "candidate_manifest_sha256": "a" * 64,
                "profile_id": "SELFTEST",
                "evidence_run_bindings": [{
                    "run_id": "run-1", "subject_pr_id": "T1-M01-P001-TST-PRE-selftest",
                    "subject_work_id": "T1-M01-N001", "subject_milestone_id": "T1-M01",
                    "execution_package_sha256": "c" * 64, "plan_kind": "TEST",
                    "plan_id": "plan-test", "plan_sha256": "d" * 64,
                    "bom_transition_sha256": None, "artifacts": [],
                    "profile_id": "SELFTEST", "run_purpose": "VERIFICATION", "gate_id": "G2", "manifest_path": "run-1.json",
                    "manifest_sha256": "b" * 64, "result": "PASS",
                }],
            },
            {"environment_id": "selftest"},
            {
                "candidate_manifest_sha256": "a" * 64,
                "profile_id": "SELFTEST", "environment_id": "selftest",
                "evidence_runs": [], "superseded_run_ids": [],
            },
        ),
    )
    current_run = {
        "run_id": "run-1", "subject_pr_id": "T1-M01-P001-TST-PRE-selftest",
        "subject_work_id": "T1-M01-N001", "subject_milestone_id": "T1-M01",
        "execution_package_sha256": "c" * 64, "plan_kind": "TEST",
        "plan_id": "plan-test", "plan_sha256": "d" * 64,
        "bom_transition_sha256": None, "artifacts": [],
        "profile_id": "SELFTEST", "run_purpose": "VERIFICATION", "gate_id": "G2", "manifest_path": "run-1.json",
        "manifest_sha256": "b" * 64, "result": "PASS",
    }
    must_reject(
        "IDX duplicating one current evidence run",
        lambda: validate_current_evidence_index_state(
            "SELFTEST-IDX-DUPLICATE",
            ["G2"],
            {
                "candidate_manifest_sha256": "a" * 64,
                "profile_id": "SELFTEST", "evidence_run_bindings": [current_run],
            },
            {"environment_id": "selftest"},
            {
                "candidate_manifest_sha256": "a" * 64,
                "profile_id": "SELFTEST", "environment_id": "selftest",
                "evidence_runs": [current_run, current_run], "superseded_run_ids": [],
            },
        ),
    )
    must_reject(
        "IDX omitting a PASS run from its transitive PR ancestry",
        lambda: validate_current_idx_upstream_closure(
            "SELFTEST-IDX-UPSTREAM",
            {"evidence_runs": []},
            [{"evidence_run_bindings": [current_run]}],
        ),
    )
    must_reject(
        "execution scope declaring M00 while selecting an M12 PR",
        lambda: validate_execution_scope_membership(
            {
                "milestone_ids": ["T1-M00"],
                "task_ids": ["T1-M12-N007"],
                "closure_slice_ids": [],
            },
            {"T1-M12-P007-PROM-n007-s1"},
            {"T1-M12-P007-PROM-n007-s1": "T1-M12-N007"},
        ),
    )
    must_reject(
        "execution scope replacing the declared milestone promotion profile",
        lambda: validate_milestone_profile_identity(
            "T1-M12", "FAKE_PROFILE", "PROPOSED_T1_CONTRACT_PROFILE"
        ),
    )
    must_reject(
        "atomic PR candidate differing from its scoped parent task candidate",
        lambda: validate_pr_parent_binding_identity(
            "SELFTEST-PR-PARENT",
            {
                "candidate_manifest_sha256": "a" * 64,
                "selected_targets": [{"path": "contracts/example.json"}],
            },
            {
                "candidate_manifest_sha256": "b" * 64,
                "selected_targets": [{"path": "contracts/example.json"}],
            },
        ),
    )
    must_reject(
        "PR dependency carrying evidence from a different candidate/profile",
        lambda: validate_pr_dependency_identity(
            "SELFTEST-PR-DEPENDENCY",
            {"candidate_manifest_sha256": "a" * 64, "profile_id": "PROFILE-A"},
            [{
                "pr_id": "SELFTEST-UPSTREAM",
                "candidate_manifest_sha256": "b" * 64,
                "profile_id": "PROFILE-B",
            }],
        ),
    )
    must_reject(
        "self-declared signature verification without trusted verifier",
        lambda: require_trusted_signature_verifier("SELFTEST"),
    )
    selftest_contract = {
        "schema_path": "contracts/alignment/task-completion-candidate.schema.json",
        "current_index_schema_path": "contracts/alignment/task-current-evidence-index.schema.json",
        "expected_atomic_pr_ids": ["SELFTEST-EXPECTED-PR"],
        "terminal_task_idx_pr_id": "SELFTEST-TASK-IDX",
        "dependency_task_idx_pr_ids": [],
        "interleaved_leaf_dependency_pr_ids": [],
        "external_activity_ids": [],
        "required_rollback_runbook_ids": [],
        "status": "PENDING",
    }
    selftest_task = {
        "task_id": "T1-M00-N001", "milestone_id": "T1-M00",
        "completion_contract": selftest_contract,
    }
    selftest_registry_sha = "a" * 64
    must_reject(
        "task completion substituting an unrelated passed leaf",
        lambda: validate_task_completion_candidate_semantics(
            {
                "task_registry_sha256": selftest_registry_sha,
                "task_definition_sha256": sha256_bytes(
                    canonical_json(selftest_task).encode("utf-8")
                ),
                "completion_contract_sha256": sha256_bytes(
                    canonical_json(selftest_contract).encode("utf-8")
                ),
                "task_id": "T1-M00-N001", "milestone_id": "T1-M00",
                "terminal_task_idx_pr_id": "SELFTEST-TASK-IDX",
                "candidate_manifest_sha256": "b" * 64, "profile_id": "SELFTEST",
                "environment_id": "selftest", "result": "PASS",
                "leaf_results": [{"pr_id": "SELFTEST-UNRELATED-PR"}],
                "dependency_task_indexes": [], "interleaved_leaf_results": [],
                "external_results": [], "evidence_runs": [], "rollback_coverage": [],
                "output_artifacts": [], "blockers": [],
            },
            selftest_task, selftest_registry_sha,
        ),
    )
    must_reject(
        "task completion referencing nonexistent execution and receipt artifacts",
        lambda: validate_task_completion_candidate_semantics(
            {
                "task_registry_sha256": selftest_registry_sha,
                "task_definition_sha256": sha256_bytes(
                    canonical_json(selftest_task).encode("utf-8")
                ),
                "completion_contract_sha256": sha256_bytes(
                    canonical_json(selftest_contract).encode("utf-8")
                ),
                "task_id": "T1-M00-N001", "milestone_id": "T1-M00",
                "terminal_task_idx_pr_id": "SELFTEST-TASK-IDX",
                "candidate_manifest_sha256": "b" * 64, "profile_id": "SELFTEST",
                "environment_id": "selftest", "result": "PASS",
                "leaf_results": [{
                    "pr_id": "SELFTEST-EXPECTED-PR", "status": "PASS",
                    "candidate_manifest_sha256": "b" * 64,
                    "profile_id": "SELFTEST", "environment_id": "selftest",
                    "execution_package": {
                        "path": "contracts/alignment/does-not-exist-execution-package.json",
                        "sha256": "c" * 64,
                    },
                    "acceptance_receipt": {
                        "path": "contracts/alignment/does-not-exist-execution-receipt.json",
                        "sha256": "d" * 64,
                    },
                    "postmerge_result": None,
                }],
                "dependency_task_indexes": [], "interleaved_leaf_results": [],
                "external_results": [], "evidence_runs": [], "rollback_coverage": [],
                "output_artifacts": [], "blockers": [],
            },
            selftest_task, selftest_registry_sha,
        ),
    )
    external_only_contract = {
        **selftest_contract,
        "expected_atomic_pr_ids": [],
        "external_activity_ids": [],
    }
    external_only_task = {
        "task_id": "T1-M11-N008", "milestone_id": "T1-M11",
        "completion_contract": external_only_contract,
    }
    validate_task_completion_candidate_semantics(
        {
            "task_registry_sha256": selftest_registry_sha,
            "task_definition_sha256": sha256_bytes(
                canonical_json(external_only_task).encode("utf-8")
            ),
            "completion_contract_sha256": sha256_bytes(
                canonical_json(external_only_contract).encode("utf-8")
            ),
            "task_id": "T1-M11-N008", "milestone_id": "T1-M11",
            "terminal_task_idx_pr_id": "SELFTEST-TASK-IDX",
            "candidate_manifest_sha256": "b" * 64, "profile_id": "SELFTEST",
            "environment_id": "selftest", "result": "PASS",
            "leaf_results": [], "dependency_task_indexes": [],
            "interleaved_leaf_results": [], "external_results": [],
            "evidence_runs": [], "rollback_coverage": [], "output_artifacts": [],
            "blockers": [],
        },
        external_only_task, selftest_registry_sha,
    )


SOURCE_SUFFIXES = {".go", ".rs", ".java", ".ts", ".tsx", ".py"}


def claim_review_key(parent: dict[str, Any], pr: dict[str, Any]) -> str:
    parent_id = parent.get("task_id", parent.get("slice_id"))
    return f"{parent_id}::{pr['phase']}::{pr['pr_type']}"


CLAIM_TEST_RUNNER_OVERRIDES = {
    "T1-M00-N006::traceability-validator-run::TST-PRE": "scripts/alignment/test_topic1_traceability.py",
    "T1-M01-N003::candidate-provenance-run::TST-PRE": "scripts/alignment/test_implementation_candidate.py",
    "T1-M01-N004::candidate-freeze-run::TST-PRE": "scripts/alignment/test_candidate_freeze.py",
    "T1-M01-N005::contract-inventory-verification::TST-PRE": "scripts/alignment/test_topic1_contract_inventory.py",
    "T1-M01-N008::proto-topic-matrix-verification::TST-PRE": "scripts/alignment/test_proto_topic_compatibility_matrix.py",
    "T1-M01-N009::schema-authority-verification::TST-PRE": "scripts/alignment/test_schema_authority_registry.py",
    "T1-M01-N010::trusted-signature-negative-run::TST-PRE": "scripts/alignment/test_trusted_signature_verifier.py",
    "T1-M01-N010::trusted-signature-positive-run::TST-POST": "scripts/alignment/test_trusted_signature_verifier.py",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": "scripts/alignment/run_exact_go_tests.py",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": "scripts/alignment/run_exact_go_tests.py",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": "scripts/alignment/run_exact_go_tests.py",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": "scripts/alignment/verify_asset_atomic_ephemeral.py",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": "scripts/alignment/verify_asset_projection_kafka_ephemeral.py",
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST": "scripts/alignment/reconcile_asset_authority_live.py",
}

# These Go files are read-only test sources for evidence leaves.  They are not
# Python matrix runners and therefore must never enter
# CLAIM_TEST_RUNNER_OVERRIDES.
CLAIM_TEST_SOURCE_OVERRIDES = {
    "T1-M06-N004::source-precedence-verification::TST-PRE":
        "scripts/alignment/validate_asset_upsert_source_precedence.py",
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST":
        "scripts/alignment/reconcile_asset_authority_live.py",
}

CLAIM_NO_CASE_REPORT_KEYS = {
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST",
}

CLAIM_NATIVE_EVIDENCE_MANIFEST_KEYS = {
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST",
}

CLAIM_GENERIC_EVIDENCE_MANIFEST_GATES = {
    "T1-M06-N004::source-precedence-verification::TST-PRE": "G0",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": "G0",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": "G0",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": "G0",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": "G1",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": "G1",
}

CLAIM_GENERIC_RESULT_SCHEMAS = {
    "T1-M06-N004::source-precedence-verification::TST-PRE":
        "contracts/alignment/asset-upsert-source-precedence-test-result.schema.json",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE":
        "contracts/alignment/exact-go-test-result.schema.json",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE":
        "contracts/alignment/exact-go-test-result.schema.json",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE":
        "contracts/alignment/exact-go-test-result.schema.json",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE":
        "contracts/alignment/asset-atomic-ephemeral-test-result.schema.json",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE":
        "contracts/alignment/asset-projection-kafka-ephemeral-test-result.schema.json",
}

CLAIM_TEST_COMMAND_OVERRIDES = {
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": (
        "python3 scripts/alignment/verify_asset_atomic_ephemeral.py "
        "--suite asset-upsert-only "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        "--candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p906-tst-pre-n004-authority-transaction-fault-matrix/"
        "test-result.json"
    ),
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": (
        "python3 scripts/alignment/verify_asset_projection_kafka_ephemeral.py "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        "--candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p908-tst-pre-n004-asset-event-real-broker-ack/test-result.json"
    ),
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": (
        "python3 scripts/alignment/run_exact_go_tests.py --self-test "
        "--package ./internal/asset/api "
        "--subject-pr-id T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification "
        "--test TestAtomicAssetUpsertCommitUnknownReturnsSafePending "
        "--source go/control-plane/internal/asset/api/auth_test.go "
        "--source go/control-plane/internal/asset/api/http_handler.go "
        "--candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p910-tst-pre-n004-http-commit-unknown-verification/test-result.json"
    ),
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": (
        "python3 scripts/alignment/run_exact_go_tests.py --self-test "
        "--package ./internal/asset/api "
        "--subject-pr-id T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification "
        "--test TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage "
        "--source go/control-plane/internal/asset/api/grpc_handler_test.go "
        "--source go/control-plane/internal/asset/api/grpc_handler.go "
        "--candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p912-tst-pre-n004-grpc-commit-unknown-verification/test-result.json"
    ),
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": (
        "python3 scripts/alignment/run_exact_go_tests.py --self-test "
        "--package ./internal/asset/config "
        "--subject-pr-id T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification "
        "--test TestAssetEventTopicRailFailsClosed "
        "--source go/control-plane/internal/asset/config/loader_test.go "
        "--source go/control-plane/internal/asset/config/loader.go "
        "--candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p914-tst-pre-n004-asset-event-topic-rail-verification/test-result.json"
    ),
    "T1-M06-N004::source-precedence-verification::TST-PRE": (
        "test -n \"$TOPIC1_DESIGN_CANDIDATE_MANIFEST\" && test -n \"$TOPIC1_PROFILE_ID\" && "
        "test -n \"$TOPIC1_ENVIRONMENT_ID\" && "
        "python3 scripts/alignment/validate_asset_upsert_source_precedence.py --self-test "
        "--candidate-manifest \"$TOPIC1_DESIGN_CANDIDATE_MANIFEST\" "
        "--profile-id \"$TOPIC1_PROFILE_ID\" --environment-id \"$TOPIC1_ENVIRONMENT_ID\" "
        "--run-id \"${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required}\" "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p916-tst-pre-n004-source-precedence-verification/test-result.json"
    ),
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST": (
        "python3 scripts/alignment/reconcile_asset_authority_live.py "
        "--candidate-manifest ${TOPIC1_CANDIDATE_MANIFEST:?TOPIC1_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--run-manifest ${TOPIC1_RUN_MANIFEST:?TOPIC1_RUN_MANIFEST is required} "
        "--authority-receipt ${TOPIC1_AUTHORITY_RECEIPT:?TOPIC1_AUTHORITY_RECEIPT is required} "
        "--broker-receipt ${TOPIC1_BROKER_RECEIPT:?TOPIC1_BROKER_RECEIPT is required} "
        "--projection-receipt ${TOPIC1_PROJECTION_RECEIPT:?TOPIC1_PROJECTION_RECEIPT is required} "
        "--output doc/02_acceptance/topic1/work-orders/"
        "t1-m06-p919-tst-post-n004-asset-authority-live-reconcile/test-result.json"
    ),
}

# Leaf-specific runners decide business truth; this common adapter only binds
# an already-PASS immutable result to the signed TEST plan/package identity.
# The chain uses `&&`, so a failed, skipped, zero-match, or blocked runner can
# never emit a gate manifest.
_GENERIC_EVIDENCE_PR_IDS = {
    "T1-M06-N004::source-precedence-verification::TST-PRE": "T1-M06-P916-TST-PRE-n004-source-precedence-verification",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": "T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack",
}
for _review_key, _gate_id in CLAIM_GENERIC_EVIDENCE_MANIFEST_GATES.items():
    _pr_id = _GENERIC_EVIDENCE_PR_IDS[_review_key]
    _work_dir = f"doc/02_acceptance/topic1/work-orders/{_pr_id.lower()}"
    _result_schema = CLAIM_GENERIC_RESULT_SCHEMAS[_review_key]
    CLAIM_TEST_COMMAND_OVERRIDES[_review_key] += (
        " && python3 scripts/alignment/write_evidence_run_manifest.py "
        f"--subject-pr-id {_pr_id} --subject-work-id T1-M06-N004 --milestone-id T1-M06 "
        "--run-id ${TOPIC1_RUN_ID:?TOPIC1_RUN_ID is required} "
        f"--gate-id {_gate_id} "
        "--candidate-manifest ${TOPIC1_CANDIDATE_MANIFEST:?TOPIC1_CANDIDATE_MANIFEST is required} "
        "--design-candidate-manifest ${TOPIC1_DESIGN_CANDIDATE_MANIFEST:?TOPIC1_DESIGN_CANDIDATE_MANIFEST is required} "
        "--profile-id ${TOPIC1_PROFILE_ID:?TOPIC1_PROFILE_ID is required} "
        "--environment-id ${TOPIC1_ENVIRONMENT_ID:?TOPIC1_ENVIRONMENT_ID is required} "
        "--execution-package-sha256 ${TOPIC1_EXECUTION_PACKAGE_SHA256:?TOPIC1_EXECUTION_PACKAGE_SHA256 is required} "
        "--plan ${TOPIC1_TEST_PLAN:?TOPIC1_TEST_PLAN is required} "
        "--time-window ${TOPIC1_TIME_WINDOW:?TOPIC1_TIME_WINDOW is required} "
        f"--result-artifact {_work_dir}/test-result.json "
        f"--result-schema {_result_schema} "
        f"--output {_work_dir}/evidence-{_gate_id.lower()}.json"
    )


# Function- or artifact-specific implementation bodies for the M06-N004
# golden train.  These replace generic S1-S3 prose in the developer catalog;
# execution remains blocked by the signed-overlay ceiling.
CLAIM_IMPLEMENTATION_STEP_OVERRIDES: dict[str, list[dict[str, str]]] = {
    "T1-M06-N004::source-precedence-contract::CTR": [
        {"step_id": "S1", "action": "freeze AssetRecord field exact-set and the two approved action IDs from candidate sources", "target_path": "contracts/alignment/asset-upsert-source-precedence.schema.json", "expected_effect": "schema permits exactly one explicit rule per source field and keeps approval lifecycle fail closed"},
        {"step_id": "S2", "action": "write all 22 field rules with class, manual behavior, observation behavior, stale-observation behavior and oracle IDs", "target_path": "contracts/alignment/asset-upsert-source-precedence.v1.json", "expected_effect": "identity, governance, observation and server-managed fields have no implicit fallback"},
        {"step_id": "S3", "action": "retain status PROPOSED_DOMAIN_REVIEW until the two required owners sign the frozen defaults and stale-observation revision rule", "target_path": "contracts/alignment/asset-upsert-source-precedence.v1.json", "expected_effect": "a structurally valid proposal cannot self-authorize the WRT leaf"},
    ],
    "T1-M06-N004::source-precedence-validator::REF": [
        {"step_id": "S1", "action": "parse AssetRecord from config.go and compute the candidate field exact-set", "target_path": "scripts/alignment/validate_asset_upsert_source_precedence.py", "expected_effect": "renamed, missing or additional source fields invalidate the contract"},
        {"step_id": "S2", "action": "validate action, field, class, oracle and lifecycle exact-sets without mutating the contract", "target_path": "scripts/alignment/validate_asset_upsert_source_precedence.py", "expected_effect": "APPROVED with blockers, missing fields and duplicate fields are rejected"},
        {"step_id": "S3", "action": "emit deterministic positive and malicious negative outcomes for the evidence-only child leaf", "target_path": "scripts/alignment/validate_asset_upsert_source_precedence.py", "expected_effect": "validator output has a bounded proof ceiling and never claims domain approval"},
    ],
    "T1-M06-N004::source-precedence-approval::IDX": [
        {"step_id": "S1", "action": "load the exact source-precedence body, candidate hash and P916 semantic result; compute the subject body SHA-256 without rewriting the proposal", "target_path": "doc/02_acceptance/topic1/work-orders/t1-m06-p917-idx-n004-source-precedence-approval/current-index.json", "expected_effect": "approval input is immutable, candidate-bound and cannot silently resolve a remaining contract blocker"},
        {"step_id": "S2", "action": "invoke the M01 protected trusted-verifier separately for multi-source-data-owner and asset-service-owner; require two typed signature receipts with distinct signer identities, same candidate/profile/environment, canonical payload, purpose and policy", "target_path": "doc/02_acceptance/topic1/work-orders/t1-m06-p917-idx-n004-source-precedence-approval/current-index.json", "expected_effect": "a proposed contract, one signer, duplicate signer/role, role mismatch, stale payload, fake receipt, changed body or active blocker remains BLOCKED"},
        {"step_id": "S3", "action": "publish only a same-candidate APPROVED_DOMAIN_CONTRACT current index that carries the signed approval receipt hash and proof ceiling; never mutate source policy status on behalf of the owners", "target_path": "doc/02_acceptance/topic1/work-orders/t1-m06-p917-idx-n004-source-precedence-approval/current-index.json", "expected_effect": "P007 may start only from a trusted domain decision while the approval leaf grants no implementation or execution authority"},
    ],
    "T1-M06-N004::authority-transaction::WRT": [
        {"step_id": "S1", "action": "load the candidate-bound code-unit contract, implementation delta, source-precedence contract and all seven P007 Go AST receipts; require P917 APPROVED with 2/2 distinct owner signatures", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "the writable exact-set is one method, every direct repository caller/callee is candidate-bound and no developer invents an unresolved domain rule"},
        {"step_id": "S2", "action": "implement B01-B02: copy input, normalize action/reason and preserve the current v1 assetUpsertIdentity canonical bytes and SHA-256", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "same logical intent retains exact replay compatibility with existing ledger rows"},
        {"step_id": "S3", "action": "implement B03-B07: BeginTx, idempotency advisory lock, ledger replay/conflict, asset advisory lock and tenant+MAC FOR UPDATE read in fixed order", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "same-key and same-asset races serialize without a durable effect before decision"},
        {"step_id": "S4", "action": "implement B08 from the approved 22-field source matrix and reject unknown action or revision conflict before authority write", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "observation input cannot overwrite governance and stale observations cannot regress current facts"},
        {"step_id": "S5", "action": "implement B09-B14 with checked Tags/Metadata/history/audit/outbox serialization and five same-transaction effects", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "assets, history, audit, pending outbox and stored receipt share tenant, revision, event and transaction identity"},
        {"step_id": "S6", "action": "implement B15-B16 with typed ErrAssetCommitUnknown and same-key recovery semantics; never retry with a new key or claim broker publication", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "commit ambiguity remains unknown and response loss converges to exactly one stored result"},
        {"step_id": "S7", "action": "regenerate the after-state AST/call receipt, compile the affected package and publish only this WRT implementation receipt; P905/P906 remain downstream verification leaves", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "expected_effect": "after-state source, function contract and call graph remain one candidate-bound truth without depending on future test receipts"},
    ],
    "T1-M06-N004::http-commit-unknown-mapping::WRT": [
        {"step_id": "H01", "action": "on POST, call h.requireAssetDiscoveryWrite before reading or mutating authority state; return the existing safe auth response on denial", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "viewer, missing principal and wrong-scope requests stop before service or database I/O"},
        {"step_id": "H02", "action": "derive request_id, trace_id and Idempotency-Key from bounded headers, decode the body once, and reject body/authenticated-tenant mismatch with the existing safe 409 path", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "tenant and logical-intent identity are stable inputs and malformed requests have zero authority effect"},
        {"step_id": "H03", "action": "construct AssetUpsertCommand with action asset-upsert, original tenant/key/request/trace/actor and expected revision, then call h.svc.UpsertAssetAtomic exactly once", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "the HTTP adapter neither retries nor manufactures a second logical intent"},
        {"step_id": "H04", "action": "preserve the existing ErrAssetRevisionConflict 409 mapping and its stable revision_conflict envelope", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "stale-write compatibility is unchanged"},
        {"step_id": "H05", "action": "preserve the existing ErrAssetIdempotencyConflict 409 mapping and prohibit automatic retry with a new key", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "same-key/different-payload remains a deterministic nonretryable conflict"},
        {"step_id": "H06", "action": "when errors.Is(err, ErrAssetCommitUnknown), return HTTP 503 with exact code asset_upsert_outcome_unknown, exact safe message asset upsert outcome is unknown; retry with the client-held original Idempotency-Key, retryable=true, retry_after_ms=1000 and meta trace_id/request_id only; never echo the key", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "outcome ambiguity is not called failed or succeeded; the caller retains its original logical-intent key without the public response disclosing it"},
        {"step_id": "H07", "action": "for every other internal error, log the protected cause with trace correlation and return exact code asset_upsert_failed plus exact message asset upsert request failed; never serialize err.Error", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "SQL, payload, tenant, MAC, key and secret markers cannot reach the response body"},
        {"step_id": "H08", "action": "on a known result, preserve the existing response schema and state only PostgreSQL authority success; compile the API package and emit this WRT receipt without waiting for P909/P910", "target_path": "go/control-plane/internal/asset/api/http_handler.go", "expected_effect": "the leaf has an exact after-state and no reverse dependency on downstream tests or Kafka projection"},
    ],
    "T1-M06-N004::grpc-commit-unknown-mapping::WRT": [
        {"step_id": "G01", "action": "read req.GetAsset, reject nil or missing MAC with existing codes.InvalidArgument, and perform no service call", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "structural input failure stops before authority I/O"},
        {"step_id": "G02", "action": "call assetUpsertCommandFromGRPC to derive authenticated tenant, actor, idempotency key, request ID, trace ID and expected revision from the approved metadata contract", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "missing auth, scope or key fails before database access"},
        {"step_id": "G03", "action": "convert the proto asset to AssetRecord, require body tenant to equal authenticated tenant, and preserve existing source/default compatibility", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "cross-tenant input is PermissionDenied and cannot reach the repository"},
        {"step_id": "G04", "action": "call h.svc.UpsertAssetAtomic exactly once with the original logical-intent identity and no adapter-level retry", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "the transport receives only a typed result or typed/wrapped error"},
        {"step_id": "G05", "action": "preserve ErrAssetRevisionConflict to codes.Aborted and ErrAssetIdempotencyConflict to codes.AlreadyExists", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "existing concurrency and payload-conflict semantics do not regress"},
        {"step_id": "G06", "action": "map errors.Is(err, ErrAssetCommitUnknown) to codes.Unavailable with exact message asset upsert outcome is unknown; retry with the same idempotency key and emit no RetryInfo or ad hoc metadata", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "unknown remains retryable only through the original key and no unversioned protocol is invented"},
        {"step_id": "G07", "action": "for every other internal error, log the protected cause and return codes.Internal with exact message asset upsert request failed; never pass err.Error to status.Error", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "SQL, payload and secret markers cannot enter gRPC status text"},
        {"step_id": "G08", "action": "on known success preserve AssetId and Created fields, compile the API package and emit this WRT receipt without waiting for P911/P912", "target_path": "go/control-plane/internal/asset/api/grpc_handler.go", "expected_effect": "the leaf has an exact after-state and does not claim Kafka or projection finality"},
    ],
    "T1-M06-N004::asset-event-topic-rail::WRT": [
        {"step_id": "C01", "action": "after environment parsing and existing server/PostgreSQL validation, read Kafka.Enabled, EventOutboxEnabled, ProjectionEnabled, Topic and EventTopic before producer or consumer construction", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "rail identity is decided at startup and keeps existing validation ordering explicit"},
        {"step_id": "C02", "action": "if Kafka.Enabled is true, require strings.TrimSpace(Kafka.Topic) to equal asset.bindings.v1; otherwise return a stable binding-rail configuration error", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "an enabled binding consumer cannot read the event rail or an arbitrary topic"},
        {"step_id": "C03", "action": "if EventOutboxEnabled or ProjectionEnabled is true, require strings.TrimSpace(Kafka.EventTopic) to equal asset.events.v2; otherwise return exact error asset event topic must be asset.events.v2 when EventOutboxEnabled or ProjectionEnabled", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "outbox-only and projection-only independently pin every enabled event producer or consumer to the canonical event contract"},
        {"step_id": "C04", "action": "when both rail families are enabled, reject equal normalized Topic and EventTopic before any client is created with exact error asset binding and event topics must differ when both rails are enabled", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "two incompatible schemas cannot collapse onto one rail"},
        {"step_id": "C05", "action": "implement and document the exact four-combination enablement table: all disabled validates neither rail; binding-only validates Topic; outbox-only validates EventTopic; projection-only validates EventTopic; never weaken a still-enabled rail", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "feature-off compatibility remains additive and every enabled combination is deterministic"},
        {"step_id": "C06", "action": "continue the existing projection/detail/export validations, compile the config package and emit this WRT receipt without waiting for P913/P914", "target_path": "go/control-plane/internal/asset/config/loader.go", "expected_effect": "the exact predicate is frozen, existing checks remain reachable and there is no reverse dependency"},
    ],
    "T1-M06-N004::http-commit-unknown-test-fixture::REF": [
        {"step_id": "S1", "action": "construct sqlmock repository, AssetService, HTTPHandler, signed asset:discover identity and fixed idempotency/request/trace inputs", "target_path": "go/control-plane/internal/asset/api/auth_test.go", "expected_effect": "the request reaches the real HTTP-service-repository path without a production database"},
        {"step_id": "S2", "action": "make tx.Commit return an internal cause containing SQL and secret marker bytes after all expected writes", "target_path": "go/control-plane/internal/asset/api/auth_test.go", "expected_effect": "the typed ErrAssetCommitUnknown branch is exercised rather than a generic validation failure"},
        {"step_id": "S3", "action": "assert HTTP 503, exact asset_upsert_outcome_unknown code/message, retryable=true, retry_after_ms=1000, trace/request metadata, absence of idempotency_key and absence of cause bytes; retry guidance refers to the caller-held original key", "target_path": "go/control-plane/internal/asset/api/auth_test.go", "expected_effect": "HTTP outcome unknown is safe, stable, does not disclose the key and is not misreported as a definitive failure"},
    ],
    "T1-M06-N004::grpc-commit-unknown-test-fixture::REF": [
        {"step_id": "S1", "action": "construct sqlmock repository, AssetService, AssetHandler and authenticated gRPC metadata with fixed tenant, key, trace, request and revision", "target_path": "go/control-plane/internal/asset/api/grpc_handler_test.go", "expected_effect": "the test reaches the real gRPC-service-repository path"},
        {"step_id": "S2", "action": "inject a commit error whose cause contains forbidden SQL/secret bytes after expected writes", "target_path": "go/control-plane/internal/asset/api/grpc_handler_test.go", "expected_effect": "errors.Is identifies typed commit ambiguity"},
        {"step_id": "S3", "action": "assert codes.Unavailable, exact message asset upsert outcome is unknown; retry with the same idempotency key, no unversioned RetryInfo, and absence of forbidden cause bytes while Aborted/AlreadyExists remain unchanged", "target_path": "go/control-plane/internal/asset/api/grpc_handler_test.go", "expected_effect": "gRPC unknown mapping is safe and transport-specific"},
    ],
    "T1-M06-N004::asset-event-topic-rail-test-fixture::REF": [
        {"step_id": "S1", "action": "build one otherwise-valid Config fixture and an exact table covering all-disabled, binding-only, outbox-only, projection-only, both-enabled, swapped and equal Kafka rail cases", "target_path": "go/control-plane/internal/asset/config/loader_test.go", "expected_effect": "unrelated validation fields cannot mask any one of the four enablement combinations"},
        {"step_id": "S2", "action": "invoke Config.validate for every case and compare the stable rail-specific error predicate", "target_path": "go/control-plane/internal/asset/config/loader_test.go", "expected_effect": "enabled noncanonical or collapsed rails fail before producer construction"},
        {"step_id": "S3", "action": "assert all-disabled, binding-only, outbox-only, projection-only and both-enabled canonical cases pass; assert the three exact stable error strings for noncanonical binding, noncanonical event and rail collision", "target_path": "go/control-plane/internal/asset/config/loader_test.go", "expected_effect": "fail-closed isolation remains backward compatible and no branch depends on developer-chosen wording"},
    ],
    "T1-M06-N004::authority-transaction-test-fixture::REF": [
        {"step_id": "S1", "action": "add TestAssetUpsertIdentityV1GoldenAndBeginFailure with literal canonical JSON bytes, SHA-256 and zero-effect begin failure", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "expected_effect": "the existing ledger hash format cannot drift silently"},
        {"step_id": "S2", "action": "add TestUpsertAtomicSameKeyDifferentPayloadZeroWrite with only the idempotency lock/ledger read expectations", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "expected_effect": "same key with a different hash rejects before asset lock or write"},
        {"step_id": "S3", "action": "add TestUpsertAtomicActionClassSourcePolicy for manual, fresh observation and stale observation rows across all 22 contract fields", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "expected_effect": "B08 field rules are executable oracles rather than prose"},
        {"step_id": "S4", "action": "add table-driven TestUpsertAtomicCrashMatrix with independent B09/B11/B12/B13/B14 sqlmock failures, Rollback and unmet-expectation checks", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "expected_effect": "mock control-flow proves every injected pre-commit error reaches Rollback but does not claim live PostgreSQL visibility"},
        {"step_id": "S5", "action": "add sentinel-gated TestUpsertAtomicPostgresPreCommitFaultMatrix; install run-scoped PostgreSQL failure triggers at assets/history/audit/outbox/ledger, execute one intent per point, then use a new connection to reconcile zero rows for the same tenant/event across all five tables", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go", "expected_effect": "B09/B11/B12/B13/B14 rollback is independently proven against the real authority store and every scoped trigger is removed in cleanup"},
        {"step_id": "S6", "action": "add sentinel-gated TestUpsertAtomicCommitUnknownSameKeyRecovery using the owned ephemeral PostgreSQL fixture and original idempotency key", "target_path": "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go", "expected_effect": "response loss and commit ambiguity converge to exactly one result in a real authority store"},
    ],
    "T1-M06-N004::asset-event-real-broker-fixture::REF": [
        {"step_id": "S1", "action": "retain sentinel checks for owned loopback PostgreSQL and Kafka, fixed one-partition asset.events.v2 and unique run identity", "target_path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "expected_effect": "the test cannot run against a shared or production target"},
        {"step_id": "S2", "action": "extend TestAssetProjectionRealKafkaDurableInbox to query pending/published_at=NULL before DispatchNext, use RequireAll/Async=false, assert the canonical event_id/event_type/schema_version/aggregate_version/tenant_id/asset_id/trace_id headers and payload, then reconcile published_at only after successful dispatch", "target_path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "expected_effect": "the successful real-broker path proves pending-before-send, canonical message identity, broker-return-before-published and durable consumption"},
        {"step_id": "S3", "action": "add TestAssetProjectionKafkaPublishFailureKeepsOutboxPending with a deterministic failing publisher and a new PostgreSQL connection that asserts status remains pending and published_at remains NULL", "target_path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "expected_effect": "publisher failure cannot be relabelled as broker ACK or a published outbox row"},
        {"step_id": "S4", "action": "commit durable inbox/offset, replay the same event and reconcile exactly one logical projection; expose explicit assertion markers consumed by the G1 runner", "target_path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "expected_effect": "ACK, publication, durable consumption and replay identities close as separate oracles"},
    ],
    "T1-M06-N004::asset-authority-live-reconcile-runner::REF": [
        {"step_id": "R01", "action": "implement reconcile_receipts as a pure function: validate the run schema, parse the UTC window and reject reversed windows without reading files or clocks", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "pure comparison starts from one explicit authorized interval"},
        {"step_id": "R02", "action": "inside reconcile_receipts require receipt kind, run/candidate/profile/environment and tenant/trace/event/asset/revision exact equality; reject stale issued_at and duplicate receipt IDs", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "unrelated, duplicate or stale facts cannot enter the chain"},
        {"step_id": "R03", "action": "inside reconcile_receipts verify the five authority counts, ACK-qualified published status, stable hashes and run-authorized required projection target exact-set", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "G2 begins only after the exact authority effect set exists"},
        {"step_id": "R04", "action": "derive expected_projection_fact_sha256 from tenant/asset/revision/event/result hash, then compare broker RequireAll/Async=false/header/payload/partition/offset and projection inbox/watermark/final hash/target order and uniqueness", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "G3 zero difference is computed, never copied from a converged flag"},
        {"step_id": "R05", "action": "implement write_reconciliation_outputs to validate and immutably write one result plus exactly ordered G2 and G3 EVIDENCE manifests bound to the evidence plan", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "write set is exactly test-result.json, evidence-g2.json and evidence-g3.json"},
        {"step_id": "R06", "action": "implement main as the only I/O adapter: safe repo paths, candidate/profile/environment/evidence-plan checks, receipt hash loading and exact artifact context passed to the M01 protected verifier", "target_path": "scripts/alignment/reconcile_asset_authority_live.py", "expected_effect": "pure reconciliation cannot be bypassed by self-reported trusted receipt fields"},
        {"step_id": "R07", "action": "run tests/alignment/test_reconcile_asset_authority_live.py and require one positive plus ten named fail-closed mutations including duplicate, stale, final-hash and target exact-set failures", "target_path": "tests/alignment/test_reconcile_asset_authority_live.py", "expected_effect": "every nontrivial function branch has an executable negative oracle"},
        {"step_id": "R08", "action": "freeze the exact positive and six registered mutation recipes without treating fixture shape as runtime evidence", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/exact-authority-broker-projection-chain.json", "expected_effect": "fixture ownership is exact and the formal run still requires protected receipts"},
        {"step_id": "R09", "action": "freeze the cross-candidate receipt mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/cross-candidate-receipt.json", "expected_effect": "a valid receipt from another candidate is rejected"},
        {"step_id": "R10", "action": "freeze the missing-ledger-effect mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/missing-ledger-effect.json", "expected_effect": "four-of-five authority effects cannot close G2"},
        {"step_id": "R11", "action": "freeze the broker-not-acked mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/broker-not-acked.json", "expected_effect": "delivery without RequireAll ACK cannot become published"},
        {"step_id": "R12", "action": "freeze the projection-offset-mismatch mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/projection-offset-mismatch.json", "expected_effect": "an unrelated inbox offset cannot close the event identity"},
        {"step_id": "R13", "action": "freeze the required-target-diverged mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/required-target-diverged.json", "expected_effect": "a required projection divergence blocks G3"},
        {"step_id": "R14", "action": "freeze the self-reported-untrusted-receipt mutation recipe", "target_path": "scripts/alignment/fixtures/topic1/t1-m06-n004/asset-authority-live-reconcile/tst-post/self-reported-untrusted-receipt.json", "expected_effect": "well-shaped arbitrary hashes never replace protected signature verification"},
    ],
}

CLAIM_REQUIRED_CASES: dict[str, tuple[tuple[str, str], ...]] = {
    "T1-M00-N006::traceability-validator-run::TST-PRE": (
        ("complete-mapping", "PASS"), ("missing-canonical", "BLOCKED"),
        ("duplicate-accountability", "BLOCKED"), ("wrong-milestone", "BLOCKED"),
        ("missing-task-leaf", "BLOCKED"), ("enhanced-dag-cycle", "BLOCKED"),
    ),
    "T1-M01-N003::candidate-provenance-run::TST-PRE": (
        ("active-prebuilt-without-provenance", "BLOCKED"),
        ("duplicate-prebuilt-path", "BLOCKED"),
        ("binary-image-sha-mismatch", "BLOCKED"),
        ("builder-recipe-mismatch", "BLOCKED"),
        ("image-deployed-digest-mismatch", "BLOCKED"),
        ("supply-chain-artifact-missing", "BLOCKED"),
        ("tracked-or-root-exclusion", "BLOCKED"),
        ("self-reported-signature-pass", "BLOCKED"),
    ),
    "T1-M01-N004::candidate-freeze-run::TST-PRE": (
        ("clean-worktree", "PASS"), ("tracked-dirty", "BLOCKED"),
        ("untracked-dirty", "BLOCKED"), ("wrong-parent", "BLOCKED"),
        ("moving-head", "BLOCKED"), ("source-roots-drift", "BLOCKED"),
        ("exclusion-set-drift", "BLOCKED"), ("run-id-overwrite", "BLOCKED"),
    ),
    "T1-M01-N005::contract-inventory-verification::TST-PRE": (
        ("exact-inventory", "PASS"), ("missing-canonical", "BLOCKED"),
        ("duplicate-canonical", "BLOCKED"), ("wrong-canonical-kind", "BLOCKED"),
        ("wrong-owner", "BLOCKED"), ("missing-contract-path", "BLOCKED"),
        ("backlog-without-next-task", "BLOCKED"),
        ("nondeterministic-rebuild", "BLOCKED"),
    ),
    "T1-M01-N008::proto-topic-matrix-verification::TST-PRE": (
        ("exact-compatible-matrix", "PASS"), ("topic-without-schema", "BLOCKED"),
        ("schema-without-topic", "BLOCKED"), ("producer-only", "BLOCKED"),
        ("consumer-only", "BLOCKED"), ("incompatible-proto-fqn", "BLOCKED"),
        ("missing-key-contract", "BLOCKED"), ("missing-dlq", "BLOCKED"),
        ("wildcard-acl", "BLOCKED"), ("producer-before-consumer", "BLOCKED"),
        ("duplicate-event-version", "BLOCKED"),
    ),
    "T1-M01-N009::schema-authority-verification::TST-PRE": (
        ("exact-authority-registry", "PASS"), ("file-without-registry", "BLOCKED"),
        ("registry-without-blob", "BLOCKED"), ("duplicate-authority", "BLOCKED"),
        ("broken-predecessor", "BLOCKED"), ("checksum-drift", "BLOCKED"),
        ("runtime-ddl-callsite", "BLOCKED"), ("init-migration-drift", "BLOCKED"),
        ("non-reentrant-replay", "BLOCKED"),
    ),
    "T1-M01-N010::trusted-signature-negative-run::TST-PRE": (
        ("payload-substitution", "BLOCKED"), ("wrong-authority-role", "BLOCKED"),
        ("certificate-time-revocation-eku", "BLOCKED"),
        ("policy-fingerprint-drift", "BLOCKED"),
        ("chain-root-algorithm-invalid", "BLOCKED"),
        ("attestation-candidate-profile-environment-mismatch", "BLOCKED"),
        ("cnas-scope-mismatch", "BLOCKED"),
        ("verifier-transport-or-shape-failure", "BLOCKED"),
        ("self-reported-pass-random-signature", "BLOCKED"),
        ("attestation-replay", "BLOCKED"),
    ),
    "T1-M01-N010::trusted-signature-positive-run::TST-POST": (
        ("protected-positive-attestation", "PASS"),
    ),
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST": (
        ("exact-authority-broker-projection-chain", "PASS"),
        ("cross-candidate-receipt", "BLOCKED"),
        ("missing-ledger-effect", "BLOCKED"),
        ("broker-not-acked", "BLOCKED"),
        ("projection-offset-mismatch", "BLOCKED"),
        ("required-target-diverged", "BLOCKED"),
        ("self-reported-untrusted-receipt", "BLOCKED"),
    ),
}

CLAIM_FIXTURE_OWNER_KEYS = {
    "T1-M00-N006::traceability-validator-run::TST-PRE":
        "T1-M00-N006::traceability-validator-fixture::REF",
    "T1-M01-N003::candidate-provenance-run::TST-PRE":
        "T1-M01-N003::candidate-provenance-fixture::REF",
    "T1-M01-N004::candidate-freeze-run::TST-PRE":
        "T1-M01-N004::candidate-freeze-fixture::REF",
    "T1-M01-N005::contract-inventory-verification::TST-PRE":
        "T1-M01-N005::contract-inventory-mutation-oracle::REF",
    "T1-M01-N008::proto-topic-matrix-verification::TST-PRE":
        "T1-M01-N008::proto-topic-matrix-mutation-oracle::REF",
    "T1-M01-N009::schema-authority-verification::TST-PRE":
        "T1-M01-N009::schema-authority-mutation-oracle::REF",
    "T1-M01-N010::trusted-signature-negative-run::TST-PRE":
        "T1-M01-N010::trusted-signature-fixture::REF",
    "T1-M01-N010::trusted-signature-positive-run::TST-POST":
        "T1-M01-N010::trusted-signature-fixture::REF",
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST":
        "T1-M06-N004::asset-authority-live-reconcile-runner::REF",
}


def registered_case_fixture_path(suite_id: str, case_id: str) -> str:
    task_id, phase, pr_type = suite_id.split("::")
    return (
        "scripts/alignment/fixtures/topic1/"
        f"{task_id.lower()}/{phase}/{pr_type.lower()}/{case_id}.json"
    )


def registered_case_fixture_payload(
    suite_id: str, case_id: str, outcome: str,
    authority_input_paths: tuple[str, ...],
) -> dict[str, Any]:
    normalized_case = case_id.replace("-", "_").upper()
    normalized_suite = re.sub(r"[^A-Z0-9]+", "_", suite_id.upper()).strip("_")
    return {
        "schema_version": "1.0.0",
        "suite_id": suite_id,
        "case_id": case_id,
        "recipe_id": f"T1-FIXTURE-{normalized_suite}-{normalized_case}-V1",
        "recipe_version": 1,
        "mutation_operator": (
            f"ASSERT_{normalized_case}" if outcome == "PASS"
            else f"MUTATE_{normalized_case}"
        ),
        "expected_outcome": outcome,
        "expected_rejection_code": (
            None if outcome == "PASS"
            else "REJECT_" + normalized_case
        ),
        "authority_input_paths": list(authority_input_paths),
        "parameters": {},
    }


def claim_case_specs(suite_id: str) -> tuple[dict[str, Any], ...]:
    expected_cases = CLAIM_REQUIRED_CASES[suite_id]
    task_id, phase, _ = suite_id.split("::")
    authority_inputs = tuple(sorted(set(PHASE_PATH_OVERRIDES[task_id][phase])))
    return tuple({
        "case_id": case_id,
        "outcome": outcome,
        "fixture_path": registered_case_fixture_path(suite_id, case_id),
        "fixture_payload": registered_case_fixture_payload(
            suite_id, case_id, outcome, authority_inputs,
        ),
    } for case_id, outcome in expected_cases)


CLAIM_CASE_SPECS = {
    suite_id: claim_case_specs(suite_id)
    for suite_id in CLAIM_REQUIRED_CASES
}

# Stable review keys are parent+phase+type, not generated PR numbers.  This
# keeps the reviewed code locator attached to its semantic leaf when an earlier
# task is split and every subsequent PR number shifts.
CLAIM_EXACT_TARGET_OVERRIDES = {
    "T1-M00-N006::traceability-validator-fixture::REF": {"targets": [{"path": "scripts/alignment/test_topic1_traceability.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "dedicated requirement/canonical/WP/accountability fixture runner"}]},
    "T1-M01-N003::candidate-provenance-fixture::REF": {"targets": [{"path": "scripts/alignment/test_implementation_candidate.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "dedicated prebuilt-artifact provenance negative-fixture runner"}]},
    "T1-M01-N004::candidate-freeze-fixture::REF": {"targets": [{"path": "scripts/alignment/test_candidate_freeze.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "dedicated clean/dirty/parent/range candidate-freeze fixture runner"}]},
    "T1-M01-N004::candidate-git-identity::REF": {"targets": [{"path": "scripts/alignment/capture_g0.py", "symbol_or_pointer": "_git_snapshot", "surface_kind": "TEST_TOOL", "reason": "candidate cleanliness and parent identity are captured at the G0 git-snapshot seam"}]},
    "T1-M01-N004::candidate-main-fail-closed::REF": {"targets": [{"path": "scripts/alignment/capture_g0.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "CLI rejects dirty, moving-HEAD and mismatched parent/range inputs before publishing a run"}]},
    "T1-M01-N005::contract-inventory-schema::CTR": {"targets": [{"path": "contracts/alignment/topic1-contract-inventory.schema.json", "symbol_or_pointer": "/", "reason": "typed schema for the derived contract inventory"}]},
    "T1-M01-N005::contract-inventory-builder::PRJ": {"targets": [{"path": "scripts/alignment/build_topic1_contract_inventory.py", "symbol_or_pointer": "main", "surface_kind": "SOURCE", "reason": "deterministic builder for the derived inventory and exact-set diff"}]},
    "T1-M01-N005::contract-inventory-mutation-oracle::REF": {"targets": [{"path": "scripts/alignment/test_topic1_contract_inventory.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "independent mutation and exact-set oracle for the derived contract inventory"}]},
    "T1-M01-N008::proto-topic-matrix-schema::CTR": {"targets": [{"path": "contracts/events/proto-topic-compatibility-matrix.schema.json", "symbol_or_pointer": "/", "reason": "typed schema for the Proto-to-Topic compatibility matrix"}]},
    "T1-M01-N008::proto-topic-matrix-builder::PRJ": {"targets": [{"path": "scripts/alignment/build_proto_topic_compatibility_matrix.py", "symbol_or_pointer": "main", "surface_kind": "SOURCE", "reason": "descriptor/catalog/ACL exact-set builder with consumer-first compatibility checks"}]},
    "T1-M01-N008::proto-topic-matrix-mutation-oracle::REF": {"targets": [{"path": "scripts/alignment/test_proto_topic_compatibility_matrix.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "independent descriptor, catalog, ACL and consumer-first mutation oracle"}]},
    "T1-M01-N009::schema-authority-contract::CTR": {"targets": [{"path": "contracts/alignment/schema-authority-registry.schema.json", "symbol_or_pointer": "/", "reason": "typed schema for storage-object DDL authority records"}]},
    "T1-M01-N009::schema-authority-builder::PRJ": {"targets": [{"path": "scripts/alignment/build_schema_authority_registry.py", "symbol_or_pointer": "main", "surface_kind": "SOURCE", "reason": "deterministic init/migration/runtime-DDL authority scanner"}]},
    "T1-M01-N009::schema-authority-mutation-oracle::REF": {"targets": [{"path": "scripts/alignment/test_schema_authority_registry.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "independent duplicate-authority, predecessor, checksum and runtime-DDL mutation oracle"}]},
    "T1-M01-N010::trusted-signature-contracts::CTR": {"targets": [
        {"path": "contracts/alignment/signature-trust-policy.schema.json", "symbol_or_pointer": "/", "reason": "versioned trust-policy reference contract"},
        {"path": "contracts/alignment/signature-verification-request.schema.json", "symbol_or_pointer": "/", "reason": "exact payload, role, purpose, time and scope request contract"},
        {"path": "contracts/alignment/signature-verification-attestation.schema.json", "symbol_or_pointer": "/", "reason": "protected-verifier attestation response contract"},
    ]},
    "T1-M01-N010::trusted-verifier-adapter::REF": {"targets": [{"path": "scripts/alignment/verify_trusted_signature.py", "symbol_or_pointer": "verify_exact_payload", "surface_kind": "TEST_TOOL", "reason": "bounded structured adapter to the independently protected signature verifier"}]},
    "T1-M01-N010::trusted-verifier-wrapper::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "require_trusted_signature_verifier", "surface_kind": "SOURCE", "reason": "typed exact-request wrapper that remains fail closed"}]},
    "T1-M01-N010::trusted-signature-fixture::REF": {"targets": [{"path": "scripts/alignment/test_trusted_signature_verifier.py", "symbol_or_pointer": "main", "surface_kind": "TEST_TOOL", "reason": "dedicated ten-case fail-closed and protected-positive test harness"}]},
    "T1-M01-N010::caller-candidate-artifact-refs::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_candidate_artifact_refs", "surface_kind": "SOURCE", "reason": "pass exact provenance payload and artifact role into the typed verifier request"}]},
    "T1-M01-N010::caller-implementation-candidate::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_implementation_candidate", "surface_kind": "SOURCE", "reason": "bind candidate supply-chain attestations to candidate/profile/environment and policy identity"}]},
    "T1-M01-N010::caller-requirement-satisfaction::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_requirement_satisfaction_refs", "surface_kind": "SOURCE", "reason": "bind authority decisions to the exact requirement evidence body and roles"}]},
    "T1-M01-N010::caller-bom-transition::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_bom_transition", "surface_kind": "SOURCE", "reason": "bind BOM transition authority to exact state, evidence and profile"}]},
    "T1-M01-N010::caller-external-signature::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_external_signature_artifacts", "surface_kind": "SOURCE", "reason": "verify external activity payload, signer role, purpose, time and scope"}]},
    "T1-M01-N010::caller-signed-contract-intake::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_signed_contract_intake", "surface_kind": "SOURCE", "reason": "verify signed requirement, metric and evidence-contract bodies"}]},
    "T1-M01-N010::caller-execution-overlay::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_execution_overlay", "surface_kind": "SOURCE", "reason": "verify scoped execution acceptance before READY or PASS authority"}]},
    "T1-M01-N010::caller-fail-closed-selftest::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "run_fail_closed_validator_self_tests", "surface_kind": "SOURCE", "reason": "migrate the built-in hard-block self-test to the typed verifier request without weakening its rejection oracle"}]},
    "T1-M01-N010::caller-work-order-evidence-run::REF": {"targets": [{"path": "scripts/alignment/build_topic1_task_registry.py", "symbol_or_pointer": "validate_work_order_evidence_run", "surface_kind": "SOURCE", "reason": "verify the positive work-order attestation with the exact payload, trust policy, role, purpose and candidate identity"}]},
    "T1-M01-N010::trusted-verifier-protected-backend::OPS": {"targets": [{"path": "deployments/security/topic1-trusted-signature-verifier.yaml", "symbol_or_pointer": "/", "reason": "default-off protected verifier endpoint, pinned policy fingerprint and secret references"}]},
    "T1-M06-N004::source-precedence-contract::CTR": {"targets": [
        {"path": "contracts/alignment/asset-upsert-source-precedence.schema.json", "symbol_or_pointer": "/", "surface_kind": "CONTRACT", "reason": "shape and fail-closed lifecycle for the action-class field precedence contract"},
        {"path": "contracts/alignment/asset-upsert-source-precedence.v1.json", "symbol_or_pointer": "/", "surface_kind": "CONTRACT", "reason": "one explicit field-by-action precedence matrix; remains proposed until the domain owner approves it"},
    ]},
    "T1-M06-N004::authority-transaction::WRT": {"targets": [{"path": "go/control-plane/internal/asset/repository/atomic_upsert.go", "symbol_or_pointer": "repository.(*AssetRepository).UpsertAtomic", "surface_kind": "SOURCE", "reason": "reviewed authoritative transaction method; assetUpsertIdentity is request-hash data, not the implementation seam"}]},
    "T1-M06-N004::http-commit-unknown-mapping::WRT": {"targets": [{"path": "go/control-plane/internal/asset/api/http_handler.go", "symbol_or_pointer": "api.(*HTTPHandler).upsertAsset", "surface_kind": "SOURCE", "reason": "HTTP behavior-changing compatibility entrypoint must map typed commit ambiguity to unknown without leaking err.Error"}]},
    "T1-M06-N004::grpc-commit-unknown-mapping::WRT": {"targets": [{"path": "go/control-plane/internal/asset/api/grpc_handler.go", "symbol_or_pointer": "api.(*AssetHandler).UpsertAsset", "surface_kind": "SOURCE", "reason": "gRPC behavior-changing compatibility entrypoint must map typed commit ambiguity to unknown without leaking err.Error"}]},
    "T1-M06-N004::asset-event-topic-rail::WRT": {"targets": [{"path": "go/control-plane/internal/asset/config/loader.go", "symbol_or_pointer": "config.(*Config).validate", "surface_kind": "SOURCE", "reason": "startup behavior-changing fail-closed seam for canonical asset.events.v2 and asset.bindings.v1 rail separation"}]},
    "T1-M06-N004::authority-transaction-test-fixture::REF": {"targets": [
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "symbol_or_pointer": "repository.TestAssetUpsertIdentityV1GoldenAndBeginFailure", "planned_signature": "func TestAssetUpsertIdentityV1GoldenAndBeginFailure(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact v1 intent-hash golden and begin-failure test"},
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "symbol_or_pointer": "repository.TestUpsertAtomicSameKeyDifferentPayloadZeroWrite", "planned_signature": "func TestUpsertAtomicSameKeyDifferentPayloadZeroWrite(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact idempotency conflict zero-write test"},
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "symbol_or_pointer": "repository.TestUpsertAtomicActionClassSourcePolicy", "planned_signature": "func TestUpsertAtomicActionClassSourcePolicy(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact manual/observation/stale-observation source-policy test"},
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_test.go", "symbol_or_pointer": "repository.TestUpsertAtomicCrashMatrix", "planned_signature": "func TestUpsertAtomicCrashMatrix(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact B09/B11/B12/B13/B14 rollback matrix test"},
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go", "symbol_or_pointer": "repository_test.TestUpsertAtomicCommitUnknownSameKeyRecovery", "planned_signature": "func TestUpsertAtomicCommitUnknownSameKeyRecovery(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact ephemeral PostgreSQL commit-ambiguity and response-loss recovery test"},
        {"path": "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go", "symbol_or_pointer": "repository_test.TestUpsertAtomicPostgresPreCommitFaultMatrix", "planned_signature": "func TestUpsertAtomicPostgresPreCommitFaultMatrix(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact live PostgreSQL B09/B11/B12/B13/B14 rollback and five-table reconciliation test"},
    ]},
    "T1-M06-N004::asset-event-real-broker-fixture::REF": {"targets": [
        {"path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "symbol_or_pointer": "consumer.TestAssetProjectionRealKafkaDurableInbox", "surface_kind": "TEST", "reason": "owned sentinel-gated real Kafka seam for broker ACK, canonical headers, published transition and exact replay evidence"},
        {"path": "go/control-plane/internal/asset/consumer/asset_projection_real_kafka_integration_test.go", "symbol_or_pointer": "consumer.TestAssetProjectionKafkaPublishFailureKeepsOutboxPending", "planned_signature": "func TestAssetProjectionKafkaPublishFailureKeepsOutboxPending(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact publisher-failure negative proving pending and published_at NULL"},
    ]},
    "T1-M06-N004::http-commit-unknown-test-fixture::REF": {"targets": [{"path": "go/control-plane/internal/asset/api/auth_test.go", "symbol_or_pointer": "api.TestAtomicAssetUpsertCommitUnknownReturnsSafePending", "planned_signature": "func TestAtomicAssetUpsertCommitUnknownReturnsSafePending(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact HTTP test function for safe transport-unknown response and no err.Error leakage"}]},
    "T1-M06-N004::grpc-commit-unknown-test-fixture::REF": {"targets": [{"path": "go/control-plane/internal/asset/api/grpc_handler_test.go", "symbol_or_pointer": "api.TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage", "planned_signature": "func TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact gRPC test function for typed commit-unknown status and safe message"}]},
    "T1-M06-N004::asset-event-topic-rail-test-fixture::REF": {"targets": [{"path": "go/control-plane/internal/asset/config/loader_test.go", "symbol_or_pointer": "config.TestAssetEventTopicRailFailsClosed", "planned_signature": "func TestAssetEventTopicRailFailsClosed(t *testing.T)", "surface_kind": "TEST", "reason": "planned exact table test for canonical event and binding rail separation"}]},
    "T1-M06-N004::source-precedence-validator::REF": {"targets": [{"path": "scripts/alignment/validate_asset_upsert_source_precedence.py", "symbol_or_pointer": "validate_contract", "planned_signature": "def validate_contract(payload: dict[str, Any]) -> None:", "surface_kind": "TEST_TOOL", "reason": "planned exact Python validator seam; current baseline has no candidate-bound implementation blob"}]},
    "T1-M06-N004::asset-authority-live-reconcile-runner::REF": {"targets": [
        {"path": "scripts/alignment/reconcile_asset_authority_live.py", "symbol_or_pointer": "reconcile_receipts", "planned_signature": "def reconcile_receipts(run_manifest: dict[str, Any], authority: dict[str, Any], broker: dict[str, Any], projection: dict[str, Any]) -> dict[str, Any]:", "surface_kind": "TEST_TOOL", "reason": "pure exact G2/G3 authority-broker-projection reconciliation function"},
        {"path": "scripts/alignment/reconcile_asset_authority_live.py", "symbol_or_pointer": "write_reconciliation_outputs", "planned_signature": "def write_reconciliation_outputs(run_manifest: dict[str, Any], input_sha256: dict[str, str], reconciled: dict[str, Any], output: Path) -> tuple[Path, Path, Path]:", "surface_kind": "TEST_TOOL", "reason": "immutable exact result and dual evidence-manifest writer"},
        {"path": "scripts/alignment/reconcile_asset_authority_live.py", "symbol_or_pointer": "main", "planned_signature": "def main() -> int:", "surface_kind": "TEST_TOOL", "reason": "safe CLI and protected verifier orchestration boundary"},
        {"path": "tests/alignment/test_reconcile_asset_authority_live.py", "symbol_or_pointer": "main", "planned_signature": "def main() -> int:", "surface_kind": "TEST", "reason": "positive plus ten fail-closed semantic branch oracle"}
    ]},
}

# Each verification case has one immutable, suite-owned recipe file.  The
# fixture implementation leaf writes it; the later TST leaf can only consume
# that exact dependency artifact.  This prevents arbitrary repository files
# from being relabelled as fixtures by a self-consistent report.
for _suite_id, _specs in CLAIM_CASE_SPECS.items():
    _task_id, _run_phase, _ = _suite_id.split("::")
    _owner_key = CLAIM_FIXTURE_OWNER_KEYS[_suite_id]
    _owner_task, _owner_phase, _owner_type = _owner_key.split("::")
    if _owner_task != _task_id:
        raise ValueError(f"fixture owner crosses tasks: {_suite_id} -> {_owner_key}")
    _fixture_paths = [item["fixture_path"] for item in _specs]
    PHASE_PATH_OVERRIDES[_task_id][_run_phase].extend(_fixture_paths)
    PHASE_PATH_OVERRIDES[_owner_task][_owner_phase].extend(_fixture_paths)
    CLAIM_EXACT_TARGET_OVERRIDES[_owner_key]["targets"].extend({
        "path": item["fixture_path"],
        "symbol_or_pointer": "/",
        "surface_kind": "TEST",
        "reason": (
            f"immutable registered recipe for case {item['case_id']} in {_suite_id}"
        ),
    } for item in _specs)

REGISTERED_JSON_CONTRACT_PAIRS = {
    (
        "contracts/alignment/m02-code-direct-leaf-allocation.v2.json",
        "contracts/alignment/m02-code-direct-leaf-allocation.v2.schema.json",
    ),
    (
        "contracts/alignment/m02-code-direct-leaf-catalog.v2.json",
        "contracts/alignment/m02-code-direct-leaf-catalog.v2.schema.json",
    ),
    (
        "contracts/alignment/m02-code-direct-leaf-catalog.v1.json",
        "contracts/alignment/m02-code-direct-leaf-catalog.schema.json",
    ),
    (
        "contracts/alignment/m02-partial-ack-function-design.v1.json",
        "contracts/alignment/m02-partial-ack-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-pcap-spool-function-design.v1.json",
        "contracts/alignment/m02-pcap-spool-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-pcap-consumer-function-design.v1.json",
        "contracts/alignment/m02-pcap-consumer-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-pcap-projection-function-design.v1.json",
        "contracts/alignment/m02-pcap-projection-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-pcap-metadata-receipt-function-design.v1.json",
        "contracts/alignment/m02-pcap-metadata-receipt-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-capture-flow-identity-function-design.v1.json",
        "contracts/alignment/m02-capture-flow-identity-function-design.schema.json",
    ),
    (
        "contracts/alignment/m02-probe-control-ack-function-design.v1.json",
        "contracts/alignment/m02-probe-control-ack-function-design.schema.json",
    ),
    (
        "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/candidate-manifest.json",
        "contracts/alignment/design-candidate-manifest.schema.json",
    ),
    ("contracts/alignment/topic1-contract-inventory.v1.json", "contracts/alignment/topic1-contract-inventory.schema.json"),
    ("contracts/events/proto-topic-compatibility-matrix.v1.json", "contracts/events/proto-topic-compatibility-matrix.schema.json"),
    ("contracts/alignment/schema-authority-registry.v1.json", "contracts/alignment/schema-authority-registry.schema.json"),
    (
        "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/t1-m06-p007-wrt-n004-s1/locator-resolver-receipt.json",
        "contracts/alignment/locator-resolution-receipt.schema.json",
    ),
    *(
        (
            "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/"
            f"{leaf}/locator-resolver-receipt.json",
            "contracts/alignment/locator-resolution-receipt.schema.json",
        )
        for leaf in (
            "t1-m06-p902-wrt-n004-http-commit-unknown-mapping",
            "t1-m06-p903-wrt-n004-grpc-commit-unknown-mapping",
            "t1-m06-p904-wrt-n004-asset-event-topic-rail",
        )
    ),
    (
        "contracts/alignment/asset-upsert-source-precedence.v1.json",
        "contracts/alignment/asset-upsert-source-precedence.schema.json",
    ),
    (
        "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/t1-m06-p007-wrt-n004-s1/implementation-delta.v1.json",
        "contracts/alignment/function-implementation-delta.schema.json",
    ),
    *(
        (
            f"doc/02_acceptance/topic1/tasks/t1-m06-n004/design/t1-m06-p007-wrt-n004-s1/{name}-plan-manifest.json",
            "contracts/alignment/atomic-pr-plan-manifest.schema.json",
        )
        for name in ("test", "evidence", "rollback", "observation")
    ),
    (
        "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/t1-m06-p007-wrt-n004-s1/atomic-pr-execution-package.draft.json",
        "contracts/alignment/atomic-pr-execution-package.schema.json",
    ),
}
CLAIM_REVIEWED_DIRECT_KEYS = (
    set(CLAIM_EXACT_TARGET_OVERRIDES)
    | set(CLAIM_TEST_RUNNER_OVERRIDES)
    | set(CLAIM_TEST_SOURCE_OVERRIDES)
)
CLAIM_ALLOWED_CLAIM_OVERRIDES = {
    key: f"bounded implementation result for exact leaf {key.split('::')[1]}"
    for key in CLAIM_REVIEWED_DIRECT_KEYS
}
CLAIM_ALLOWED_CLAIM_OVERRIDES.update({
    "T1-M00-N006::traceability-validator-fixture::REF": "traceability fixture and validator assets are implemented; no test PASS is claimed",
    "T1-M00-N006::traceability-validator-run::TST-PRE": "same-candidate structural traceability matrix PASS for the declared cases",
    "T1-M01-N003::candidate-provenance-fixture::REF": "candidate provenance negative-fixture assets are implemented; no test PASS is claimed",
    "T1-M01-N003::candidate-provenance-run::TST-PRE": "same-candidate prebuilt provenance rejection matrix PASS for the declared cases",
    "T1-M01-N004::candidate-freeze-fixture::REF": "candidate-freeze fixture assets are implemented; no freeze PASS is claimed",
    "T1-M01-N004::candidate-freeze-run::TST-PRE": "same-candidate clean/dirty/parent/range freeze matrix PASS for the declared cases",
    "T1-M01-N005::contract-inventory-schema::CTR": "versioned contract-inventory schema is defined without a derived-instance claim",
    "T1-M01-N005::contract-inventory-builder::PRJ": "deterministic contract-inventory builder and exact-set semantics are implemented",
    "T1-M01-N005::contract-inventory-verification::TST-PRE": "same-candidate contract inventory rebuild, schema and exact-set checks PASS",
    "T1-M01-N008::proto-topic-matrix-schema::CTR": "versioned Proto/Topic matrix schema is defined without a compatibility PASS claim",
    "T1-M01-N008::proto-topic-matrix-builder::PRJ": "deterministic descriptor/catalog/ACL matrix builder is implemented",
    "T1-M01-N008::proto-topic-matrix-verification::TST-PRE": "same-candidate Proto/Topic/ACL exact-set and compatibility checks PASS",
    "T1-M01-N009::schema-authority-contract::CTR": "versioned schema-authority contract is defined without a runtime DDL claim",
    "T1-M01-N009::schema-authority-builder::PRJ": "deterministic init/migration/runtime-DDL authority scanner is implemented",
    "T1-M01-N009::schema-authority-verification::TST-PRE": "same-candidate schema-authority exact-set and replay checks PASS",
    "T1-M01-N010::trusted-signature-negative-run::TST-PRE": "same-candidate trusted-signature fail-closed negative matrix PASS",
    "T1-M01-N010::trusted-verifier-protected-backend::OPS": "default-off verifier deployment manifest and its static security contract are implemented; no protected-backend availability or trust PASS is claimed",
    "T1-M01-N010::trusted-signature-positive-run::TST-POST": "protected-environment exact-payload positive attestation path PASS for this technical leaf",
    "T1-M06-N004::source-precedence-contract::CTR": "proposed action-class source-precedence contract validates structurally; domain approval is not claimed",
    "T1-M06-N004::source-precedence-validator::REF": "source-precedence semantic validator is bound to the exact contract and AssetRecord field set; approval is not claimed",
    "T1-M06-N004::source-precedence-verification::TST-PRE": "source-precedence exact-set and fail-closed negative matrix may be claimed only for the recorded candidate run",
    "T1-M06-N004::source-precedence-approval::IDX": "distinct trusted owners approved this exact candidate-bound source-precedence body; no implementation or execution authority is claimed",
    "T1-M06-N004::authority-transaction::WRT": "the exact UpsertAtomic target and candidate-bound function-design inputs are identified; implementation or test PASS is not claimed",
    "T1-M06-N004::http-commit-unknown-mapping::WRT": "the exact HTTP compatibility seam for commit-unknown mapping is identified; implementation is not claimed",
    "T1-M06-N004::grpc-commit-unknown-mapping::WRT": "the exact gRPC compatibility seam for commit-unknown mapping is identified; implementation is not claimed",
    "T1-M06-N004::asset-event-topic-rail::WRT": "the exact startup configuration validation seam is identified; runtime rail isolation is not claimed",
    "T1-M06-N004::authority-transaction-test-fixture::REF": "the exact Go test seam for planned source, fault and recovery cases is identified; test PASS is not claimed",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": "the exact candidate-bound fault-matrix command may produce a leaf result only after all no-SKIP oracles pass",
    "T1-M06-N004::asset-event-real-broker-fixture::REF": "the exact sentinel-gated real-broker fixture seam is identified; broker evidence is not claimed",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": "the exact ephemeral Kafka command may prove ACK-before-published and reconciliation only for its declared run",
    "T1-M06-N004::http-commit-unknown-test-fixture::REF": "the exact HTTP test seam is bound for planned safe-unknown assertions; test implementation or PASS is not claimed",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": "the exact HTTP safe-unknown test may prove only its same-candidate no-leak transport oracle",
    "T1-M06-N004::grpc-commit-unknown-test-fixture::REF": "the exact gRPC test seam is bound for planned unavailable/safe-message assertions; test implementation or PASS is not claimed",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": "the exact gRPC safe-unknown test may prove only its same-candidate status and no-leak oracle",
    "T1-M06-N004::asset-event-topic-rail-test-fixture::REF": "the exact config test seam is bound for planned canonical-rail negatives; test implementation or PASS is not claimed",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": "the exact config rail test may prove only startup fail-closed separation for its candidate",
})
CLAIM_FORBIDDEN_CLAIM_OVERRIDES = {
    key: (
        "Only this exact leaf is in scope; it does not close the parent task, satisfy a requirement, "
        "complete a milestone, prove CNAS acceptance, authorize production rollout, or publish a release"
    )
    for key in CLAIM_REVIEWED_DIRECT_KEYS
}

CLAIM_SEMANTIC_BUILDERS: dict[str, tuple[str, str, str]] = {}

CLAIM_BUILDER_COMMANDS = {
    "T1-M06-N004::source-precedence-approval::IDX": (
        "python3 scripts/alignment/validate_asset_upsert_source_precedence_approval.py --self-test && "
        "python3 scripts/alignment/validate_asset_upsert_source_precedence_approval.py --instance "
        "doc/02_acceptance/topic1/work-orders/t1-m06-p917-idx-n004-source-precedence-approval/current-index.json"
    ),
    "T1-M01-N005::contract-inventory-builder::PRJ": "python3 scripts/alignment/build_topic1_contract_inventory.py --write && python3 scripts/alignment/build_topic1_contract_inventory.py --check",
    "T1-M01-N008::proto-topic-matrix-builder::PRJ": "python3 scripts/alignment/build_proto_topic_compatibility_matrix.py --write && python3 scripts/alignment/build_proto_topic_compatibility_matrix.py --check",
    "T1-M01-N009::schema-authority-builder::PRJ": "python3 scripts/alignment/build_schema_authority_registry.py --write && python3 scripts/alignment/build_schema_authority_registry.py --check",
    "T1-M06-N004::asset-authority-live-reconcile-runner::REF": (
        "python3 tests/alignment/test_reconcile_asset_authority_live.py"
    ),
}

CLAIM_COMPONENT_TEST_CASES = {
    "T1-M01-N004::candidate-git-identity::REF": "git-identity",
    "T1-M01-N004::candidate-main-fail-closed::REF": "main-fail-closed",
    "T1-M01-N010::trusted-verifier-adapter::REF": "adapter",
    "T1-M01-N010::trusted-verifier-wrapper::REF": "wrapper",
}

CLAIM_DERIVED_OUTPUTS = {
    "T1-M01-N005::contract-inventory-builder::PRJ": (
        "TOPIC1-CONTRACT-INVENTORY",
        "contracts/alignment/topic1-contract-inventory.v1.json",
    ),
    "T1-M01-N008::proto-topic-matrix-builder::PRJ": (
        "PROTO-TOPIC-COMPATIBILITY-MATRIX",
        "contracts/events/proto-topic-compatibility-matrix.v1.json",
    ),
    "T1-M01-N009::schema-authority-builder::PRJ": (
        "SCHEMA-AUTHORITY-REGISTRY",
        "contracts/alignment/schema-authority-registry.v1.json",
    ),
}

CLAIM_ROLLBACK_OVERRIDES = {
    key: [
        "revert only the unpromoted fixture or derived-contract implementation from this leaf",
        "preserve every historical PASS/FAIL result and keep the parent TASK-IDX unchanged",
    ]
    for key in CLAIM_REVIEWED_DIRECT_KEYS
}
for _key in CLAIM_REVIEWED_DIRECT_KEYS:
    if "trusted-" in _key or "caller-" in _key:
        CLAIM_ROLLBACK_OVERRIDES[_key] = [
            "restore unconditional BLOCKED behavior at the affected verifier seam",
            "remove only the unpromoted backend reference or code path; preserve signed receipts and failed attestations",
        ]
    elif "candidate-" in _key:
        CLAIM_ROLLBACK_OVERRIDES[_key] = [
            "stop candidate capture and restore the prior fail-closed implementation",
            "do not clean, stash, reset or otherwise mutate the user worktree; preserve historical run artifacts",
        ]
PLACEHOLDER_TEXT_RE = re.compile(
    r"^(?:true|noop|none|n/?a|todo|tbd|do something|looks okay)$", re.IGNORECASE
)


def contains_placeholder_semantics(value: str) -> bool:
    normalized = value.strip().lower()
    return bool(
        PLACEHOLDER_TEXT_RE.fullmatch(normalized)
        or re.search(r"\b(?:noop|looks okay|do something|tbd|todo)\b", normalized)
        or re.match(r"^(?:true|:)(?:\s|$|[;&|])", normalized)
    )


def claim_surface_kind(path: str) -> str:
    """Classify a path for write-policy purposes, not by historical use."""
    lower = path.lower()
    name = Path(path).name
    suffix = Path(path).suffix.lower()
    if path.startswith("scripts/alignment/fixtures/"):
        return "TEST"
    if (
        path.startswith("doc/02_acceptance/")
        or path.startswith("contracts/releases/")
        or "evidence-index" in lower
        or "current-index" in lower
        or "promotion" in lower
        or "release-pointer" in lower
    ):
        return "EVIDENCE"
    if (
        "/test/" in lower or "/tests/" in lower
        or lower.endswith(("_test.go", "_test.rs", "test.py", ".test.ts", ".test.tsx"))
    ):
        return "TEST"
    if path.startswith("scripts/") and re.search(
        r"(?:^|/)(?:check|verify|validate|capture|reconcile|audit|smoke|test)[^/]*\.py$",
        lower,
    ):
        return "TEST_TOOL"
    if suffix in SOURCE_SUFFIXES:
        return "SOURCE"
    if suffix == ".sql":
        return "MIGRATION"
    if suffix in {".yaml", ".yml"} or name.startswith("Dockerfile"):
        return "DEPLOYMENT"
    if suffix in {".proto", ".json"}:
        return "CONTRACT"
    return "DOCUMENT"


def claim_locator_kind(path: str, surface_kind: str) -> str:
    suffix = Path(path).suffix.lower()
    return {
        ".go": "go_symbol", ".rs": "rust_symbol", ".java": "java_symbol",
        ".ts": "ts_symbol", ".tsx": "ts_symbol", ".py": "python_symbol",
        ".proto": "proto_fqn", ".sql": "sql_object",
        ".json": "json_pointer", ".yaml": "yaml_path", ".yml": "yaml_path",
    }.get(suffix, "file" if surface_kind != "TARGET_BINDING" else "json_pointer")


CLAIM_WRITE_POLICY = {
    "CTR": {"CONTRACT"},
    "EXP": {"MIGRATION"},
    "PRJ": {"SOURCE"},
    "WRT": {"SOURCE"},
    "UI": {"SOURCE"},
    "OPS": {"DEPLOYMENT", "TEST_TOOL"},
    "REF": {"SOURCE", "TEST", "TEST_TOOL"},
    "TST-PRE": {"EVIDENCE"},
    "TST-POST": {"EVIDENCE"},
    "IDX": {"EVIDENCE"},
    "PROM": {"EVIDENCE"},
}

CLAIM_ROLE_TOKENS = {
    "CTR": {"contract", "schema", "proto", "openapi", "catalog"},
    "EXP": {"migration", "expand", "table", "column", "index"},
    "PRJ": {"consumer", "project", "projection", "reconcile", "sink", "reader"},
    "WRT": {"repository", "writer", "outbox", "dispatcher", "service", "handler", "command"},
    "UI": {"page", "component", "view", "form", "client", "api"},
    "OPS": {"deploy", "config", "canary", "rollout", "restore", "observe"},
    "REF": {"refactor", "seam", "adapter", "job", "service", "handler"},
    "TST-PRE": {"test", "verify", "check", "contract", "negative"},
    "TST-POST": {"test", "verify", "check", "live", "reconcile", "browser"},
    "IDX": {"index", "evidence", "current"},
    "PROM": {"release", "pointer", "promotion"},
}

CLAIM_ORACLES = {
    "CTR": "versioned contract validates and the compatibility diff contains no unapproved removal",
    "EXP": "the single additive migration is reentrant and preserves the old reader/writer path",
    "PRJ": "the consumer/projector is idempotent, handles duplicate/late input, and reconciles to authority",
    "WRT": "authority state, audit and outbox commit atomically and deterministic retry returns the same result",
    "UI": "the typed user action reaches the real API and its receipt resolves to the final authoritative fact",
    "OPS": "default-off rollout, stop threshold, rollback and post-rollback health checks all pass",
    "REF": "characterization tests remain unchanged while the named seam moves without contract drift",
    "TST-PRE": "the exact positive and negative checks pass on the frozen candidate without runtime enablement",
    "TST-POST": "the required gate passes on the declared environment and final facts reconcile without unexplained diff",
    "IDX": "the current index contains the exact non-superseded artifact/run set and every hash resolves",
    "PROM": "pre/post-merge production content is equivalent and the release pointer references the signed current IDX",
}

CLAIM_ORACLE_OVERRIDES = {
    "T1-M00-N006::traceability-validator-fixture::REF": "the dedicated runner contains named positive and orphan/duplicate/wrong-milestone/missing-leaf/cycle fixtures and compiles",
    "T1-M00-N006::traceability-validator-run::TST-PRE": "every declared traceability case records expected/actual result and the same-candidate result is PASS",
    "T1-M01-N003::candidate-provenance-fixture::REF": "the dedicated runner contains the complete prebuilt/provenance rejection matrix and compiles",
    "T1-M01-N003::candidate-provenance-run::TST-PRE": "every declared provenance case records the required rejection code and the same-candidate result is PASS",
    "T1-M01-N004::candidate-freeze-fixture::REF": "the dedicated runner covers clean, tracked dirty, untracked, wrong parent, moving HEAD, source-root, exclusion and overwrite cases",
    "T1-M01-N004::candidate-git-identity::REF": "_git_snapshot returns porcelain-v2 identity without cleaning or hiding any tracked or untracked change",
    "T1-M01-N004::candidate-main-fail-closed::REF": "main blocks before publishing when dirty, parent/range mismatched, HEAD moves or the run identity changes",
    "T1-M01-N004::candidate-freeze-run::TST-PRE": "all eight freeze cases record expected/actual state and the same-candidate matrix is PASS",
    "T1-M01-N005::contract-inventory-schema::CTR": "the schema fixes item identity, source hash, owner, status, blocker and next-task fields without creating an inventory instance",
    "T1-M01-N005::contract-inventory-builder::PRJ": "the builder deterministically derives 54 feature, 48 technical, 38 standard and 16 backlog records from read-only authorities",
    "T1-M01-N005::contract-inventory-mutation-oracle::REF": "the independent oracle rejects missing, duplicate, relabelled and wrong-owner records without modifying the derived inventory",
    "T1-M01-N005::contract-inventory-verification::TST-PRE": "schema validation, exact cardinalities, path resolution and deterministic rebuild diff all PASS",
    "T1-M01-N008::proto-topic-matrix-schema::CTR": "the schema fixes topic/version/FQN/key/producer/consumer/DLQ/ACL/readiness fields without asserting compatibility",
    "T1-M01-N008::proto-topic-matrix-builder::PRJ": "the builder resolves every catalog-referenced descriptor and computes producer-consumer-ACL-DLQ compatibility deterministically",
    "T1-M01-N008::proto-topic-matrix-mutation-oracle::REF": "the independent oracle rejects descriptor, key, ACL, DLQ and consumer-first mutations without modifying the matrix",
    "T1-M01-N008::proto-topic-matrix-verification::TST-PRE": "descriptor exact set, buf lint, catalog/ACL diff and consumer-first negative matrix all PASS",
    "T1-M01-N009::schema-authority-contract::CTR": "the schema fixes storage/object/authority/init/migration/checksum/predecessor/runtime-callsite fields without applying DDL",
    "T1-M01-N009::schema-authority-builder::PRJ": "the builder scans every approved init/migration file and runtime DDL callsite and emits a deterministic authority registry",
    "T1-M01-N009::schema-authority-mutation-oracle::REF": "the independent oracle rejects duplicate authority, predecessor, checksum, init drift and runtime-DDL mutations without applying DDL",
    "T1-M01-N009::schema-authority-verification::TST-PRE": "file-to-registry and registry-to-blob diffs, ordering, checksum, replay and duplicate-authority cases all PASS",
    "T1-M01-N010::trusted-signature-contracts::CTR": "policy, exact request and attestation schemas are versioned and forbid secret values and self-reported trust",
    "T1-M01-N010::trusted-verifier-adapter::REF": "the adapter sends a bounded structured request, pins policy identity, enforces timeout/output limits and fails closed",
    "T1-M01-N010::trusted-verifier-wrapper::REF": "the wrapper accepts the typed request instead of a context string and retains unconditional blocking when the protected verifier is unavailable",
    "T1-M01-N010::trusted-signature-fixture::REF": "the dedicated runner implements all ten negative cases plus the protected positive case without embedding trust anchors",
    "T1-M01-N010::trusted-verifier-protected-backend::OPS": "the deployment stays default off, uses only secret references and pinned policy identity, and rollback restores hard-block mode",
    "T1-M01-N010::trusted-signature-negative-run::TST-PRE": "all ten fail-closed cases record the expected rejection and no positive trust claim is emitted",
    "T1-M01-N010::trusted-signature-positive-run::TST-POST": "a protected verifier returns a cryptographically verifiable same-payload same-role same-policy attestation in the declared environment",
}
CLAIM_ORACLE_OVERRIDES.update({
    f"T1-M01-N010::{phase}::REF": (
        f"{phase.removeprefix('caller-')} passes exact payload/hash, role, purpose, time, policy, "
        "candidate/profile/environment and applicable scope to the typed verifier request; unavailable trust remains BLOCKED"
    )
    for phase in (
        "caller-candidate-artifact-refs",
        "caller-implementation-candidate",
        "caller-requirement-satisfaction",
        "caller-bom-transition",
        "caller-external-signature",
        "caller-signed-contract-intake",
        "caller-execution-overlay",
        "caller-fail-closed-selftest",
    )
})
CLAIM_ORACLE_OVERRIDES.update({
    "T1-M06-N004::source-precedence-contract::CTR": "the proposal contains one rule for every exact AssetRecord field, remains PROPOSED_DOMAIN_REVIEW, and grants no domain approval",
    "T1-M06-N004::source-precedence-validator::REF": "the validator rejects missing, duplicate, unknown-action and APPROVED-with-blocker mutations against the exact candidate field set",
    "T1-M06-N004::source-precedence-verification::TST-PRE": "the immutable result records every source-precedence positive and malicious negative case and preserves the proposed-only proof ceiling",
    "T1-M06-N004::source-precedence-approval::IDX": "the current index binds two distinct trusted owner signatures to the exact candidate, 22-field body and frozen default/revision decisions; unsigned or changed bodies remain BLOCKED",
    "T1-M06-N004::authority-transaction::WRT": "the one-method after-state compiles, preserves the v1 intent hash, applies the approved source matrix and stages exactly assets/history/audit/outbox/ledger in one PostgreSQL transaction with typed commit ambiguity",
    "T1-M06-N004::http-commit-unknown-mapping::WRT": "the HTTP after-state compiles and maps only typed commit ambiguity to the frozen safe 503 error envelope while preserving existing conflict and success envelopes",
    "T1-M06-N004::grpc-commit-unknown-mapping::WRT": "the gRPC after-state compiles and maps typed commit ambiguity to codes.Unavailable plus the frozen safe message without changing existing conflict mappings",
    "T1-M06-N004::asset-event-topic-rail::WRT": "the config after-state compiles and fails startup unless every enabled binding, event producer or projection rail uses its canonical topic",
    "T1-M06-N004::http-commit-unknown-test-fixture::REF": "the exact planned HTTP fixture drives real handler-service-repository code to commit ambiguity and asserts the frozen 503 envelope and no-leak boundary",
    "T1-M06-N004::http-commit-unknown-verification::TST-PRE": "the exact runner records one run and one pass event for the named HTTP test; zero-match, skip, fail or duplicate events are rejected",
    "T1-M06-N004::grpc-commit-unknown-test-fixture::REF": "the exact planned gRPC fixture drives real handler-service-repository code and asserts Unavailable, the frozen safe message and no-leak boundary",
    "T1-M06-N004::grpc-commit-unknown-verification::TST-PRE": "the exact runner records one run and one pass event for the named gRPC test; zero-match, skip, fail or duplicate events are rejected",
    "T1-M06-N004::asset-event-topic-rail-test-fixture::REF": "the exact planned config table covers canonical, swapped, equal, projection-only and independently disabled rail predicates with stable errors",
    "T1-M06-N004::asset-event-topic-rail-verification::TST-PRE": "the exact runner records one run and one pass event for the named config test; zero-match, skip, fail or duplicate events are rejected",
    "T1-M06-N004::authority-transaction-test-fixture::REF": "the six planned test functions separate v1 hash, same-key conflict, 22-field source policy, sqlmock control flow, live PostgreSQL pre-commit reconciliation and same-key commit-unknown recovery",
    "T1-M06-N004::authority-transaction-fault-matrix::TST-PRE": "the owned PostgreSQL runner requires one run and one pass event for every registered unit and integration test and reconciles zero-or-exactly-one durable effects",
    "T1-M06-N004::asset-event-real-broker-fixture::REF": "the sentinel-gated fixture uses RequireAll synchronous Kafka, verifies published only after the producer returns, durably commits inbox/offset and preserves one logical replay result",
    "T1-M06-N004::asset-event-real-broker-ack::TST-PRE": "the owned loopback Kafka/PostgreSQL runner requires the exact integration test run+pass and emits G1-only ACK/published/inbox/replay evidence with production_applied=false",
    "T1-M06-N004::asset-authority-live-reconcile-runner::REF": "the exact reconciliation function, schemas and malicious receipt fixtures are implemented; no G2/G3 PASS is claimed",
    "T1-M06-N004::asset-authority-live-reconcile::TST-POST": "one same-candidate authorized real-dependency run emits exactly one G2 and one G3 PASS manifest after authority, broker and projection facts reconcile with zero unexplained difference",
})


def claim_leaf_oracle(parent: dict[str, Any], pr: dict[str, Any]) -> str:
    return CLAIM_ORACLE_OVERRIDES.get(
        claim_review_key(parent, pr), CLAIM_ORACLES[pr["pr_type"]]
    )


def claim_tokens(parent: dict[str, Any], pr: dict[str, Any]) -> set[str]:
    raw = " ".join(
        [pr["phase"], pr["primary_id"], parent.get("action", ""), parent.get("minimum_closure_rule", "")]
    ).lower()
    return {
        token for token in re.findall(r"[a-z][a-z0-9]+", raw)
        if len(token) >= 4 and token not in {"step", "topic", "task"}
    } | CLAIM_ROLE_TOKENS[pr["pr_type"]]


def claim_target_discovery_command(parent: dict[str, Any], pr: dict[str, Any]) -> str:
    """Return a bounded, read-only locator search for an unresolved leaf."""
    roots_by_type = {
        "CTR": ["contracts", "proto", "common", "scripts/alignment"],
        "EXP": ["go/control-plane", "deployments", "common"],
        "PRJ": ["go/control-plane", "java/flink-jobs", "rust/probe-agent"],
        "WRT": ["go/control-plane", "java/flink-jobs", "rust/probe-agent"],
        "UI": ["web/ui/src"],
        "OPS": ["deployments", "scripts"],
        "REF": ["go/control-plane", "java/flink-jobs", "rust/probe-agent", "web/ui/src"],
        "TST-PRE": ["tests", "scripts", "go/control-plane", "java/flink-jobs", "rust/probe-agent", "web/ui"],
        "TST-POST": ["tests", "scripts", "go/control-plane", "java/flink-jobs", "rust/probe-agent", "web/ui"],
        "IDX": ["contracts/alignment", "doc/02_acceptance"],
        "PROM": ["contracts/releases", "deployments/releases", "doc/02_acceptance"],
    }
    tokens = sorted(claim_tokens(parent, pr))[:12]
    pattern = "|".join(re.escape(token) for token in tokens)
    roots = " ".join(
        root for root in roots_by_type[pr["pr_type"]]
        if (REPO_ROOT / root).exists()
    )
    if not roots:
        roots = "contracts/alignment"
    return f"rg -n --hidden --glob '!*.evidence.json' '({pattern})' {roots}"


def exact_claim_path(path: str) -> bool:
    return bool(
        path and "*" not in path and not path.endswith("/")
        and not (REPO_ROOT / path).is_dir()
        and (Path(path).suffix or Path(path).name.startswith("Dockerfile"))
    )


def path_score(path: str, tokens: set[str]) -> int:
    lower = path.lower().replace("_", "-")
    return sum(1 for token in tokens if token.replace("_", "-") in lower)


def choose_symbol(path: str, tokens: set[str]) -> str | None:
    candidates = sorted(
        {item["symbol"] for item in discover_symbol_candidates([path])}
    )
    if len(candidates) == 1:
        return candidates[0]
    if not candidates:
        return None
    scores = {
        symbol: sum(token in symbol.lower() for token in tokens)
        for symbol in candidates
    }
    best = max(scores.values())
    winners = [symbol for symbol, score in scores.items() if score == best]
    return winners[0] if best > 0 and len(winners) == 1 else None


def json_identity_pointer(path: str, identity: str) -> str:
    blob = claim_baseline_blob(path)
    if blob is None:
        return "/"
    try:
        payload = json.loads(blob.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return "/"
    identity_fields = {"id", "feature_id", "requirement_id", "canonical_id"}
    if isinstance(payload, dict):
        for key, values in payload.items():
            if not isinstance(values, list):
                continue
            for index, value in enumerate(values):
                if isinstance(value, dict) and any(
                    value.get(field) == identity for field in identity_fields
                ):
                    escaped_key = key.replace("~", "~0").replace("/", "~1")
                    return f"/{escaped_key}/{index}"
    return "/"


@lru_cache(maxsize=1)
def claim_baseline_commit() -> str:
    return subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=REPO_ROOT,
        check=True, capture_output=True, text=True,
    ).stdout.strip()


@lru_cache(maxsize=None)
def claim_baseline_blob(path: str) -> bytes | None:
    return read_candidate_blob(REPO_ROOT, claim_baseline_commit(), path)


def choose_symbol_from_blob(path: str, tokens: set[str], blob: bytes) -> str | None:
    pattern_by_suffix = {
        ".go": re.compile(r"^(?:func\s+(?:\([^)]*\)\s*)?|type\s+)([A-Za-z_]\w*)"),
        ".rs": re.compile(r"^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait)\s+([A-Za-z_]\w*)"),
        ".java": re.compile(r"^(?:public\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+([A-Za-z_]\w*)"),
        ".ts": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".tsx": re.compile(r"^export\s+(?:async\s+)?(?:function|const|class|interface|type)\s+([A-Za-z_]\w*)"),
        ".py": re.compile(r"^(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)"),
        ".proto": re.compile(r"^(?:message|enum|service)\s+([A-Za-z_]\w*)"),
    }
    pattern = pattern_by_suffix.get(Path(path).suffix.lower())
    if pattern is None:
        return None
    candidates = sorted({
        match.group(1)
        for line in blob.decode("utf-8", errors="replace").splitlines()
        if (match := pattern.match(line.strip()))
    })
    if len(candidates) == 1:
        return candidates[0]
    scores = {item: sum(token in item.lower() for token in tokens) for item in candidates}
    if not scores:
        return None
    best = max(scores.values())
    winners = [item for item, score in scores.items() if score == best]
    return winners[0] if best > 0 and len(winners) == 1 else None


def make_claim_target(
    path: str, reason: str, tokens: set[str], identity: str, *, planned_output: bool = False,
) -> dict[str, Any] | None:
    if not exact_claim_path(path):
        return None
    baseline_blob = None if planned_output else claim_baseline_blob(path)
    state = "PLANNED_OUTPUT" if planned_output else "EXISTING" if baseline_blob is not None else "PLANNED"
    surface = claim_surface_kind(path)
    locator = claim_locator_kind(path, surface)
    symbol: str | None = None
    if surface in {"CONTRACT", "TARGET_BINDING"} and Path(path).suffix == ".json":
        symbol = json_identity_pointer(path, identity)
    elif surface == "MIGRATION":
        symbol = Path(path).stem
    elif surface == "DEPLOYMENT":
        symbol = "/"
    elif Path(path).suffix == ".proto":
        symbol = choose_symbol_from_blob(path, tokens, baseline_blob) if baseline_blob is not None else Path(path).stem.title()
    elif surface in {"SOURCE", "TEST", "TEST_TOOL"}:
        if state == "PLANNED":
            return None
        symbol = choose_symbol_from_blob(path, tokens, baseline_blob) if baseline_blob is not None else None
        if surface == "SOURCE" and symbol is None:
            return None
    if locator in {
        "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
        "python_symbol", "proto_fqn",
    } and symbol is None:
        return None
    signature_before = None
    signature_after = None
    symbol_state = "NOT_APPLICABLE"
    candidate_blob_sha256 = None
    if baseline_blob is not None:
        candidate_blob_sha256 = sha256_bytes(baseline_blob)
        if locator in {
            "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
            "python_symbol", "proto_fqn",
        } and symbol is not None:
            signature_before = exact_declaration_signature(path, symbol, baseline_blob)
            signature_after = signature_before
            symbol_state = "EXISTING" if signature_before is not None else "PLANNED"
    return {
        "path": path,
        "target_state": state,
        "surface_kind": surface,
        "locator_kind": locator,
        "symbol_or_pointer": symbol,
        "symbol_state": symbol_state,
        "signature_before": signature_before,
        "signature_after": signature_after,
        "candidate_blob_sha256": candidate_blob_sha256,
        "selection_reason": reason,
    }


def exact_override_claim_target(
    parent: dict[str, Any], pr: dict[str, Any]
) -> tuple[list[dict[str, Any]], list[str]] | None:
    """Resolve one reviewed design-time target without weakening execution gates."""
    override = CLAIM_EXACT_TARGET_OVERRIDES.get(claim_review_key(parent, pr))
    if override is None:
        return None
    result = []
    for target_override in override["targets"]:
        path = target_override["path"]
        if path not in pr["candidate_paths"]:
            return [], ["reviewed exact target is absent from the leaf candidate surface"]
        if not exact_claim_path(path):
            return [], ["reviewed exact target is not a file-level repo path"]
        baseline_blob = claim_baseline_blob(path)
        state = "EXISTING" if baseline_blob is not None else "PLANNED"
        surface = target_override.get("surface_kind", claim_surface_kind(path))
        if surface not in CLAIM_WRITE_POLICY[pr["pr_type"]]:
            return [], ["reviewed exact target violates the PR-type write policy"]
        symbol = target_override["symbol_or_pointer"]
        planned_signature = target_override.get("planned_signature")
        if state == "EXISTING" and planned_signature is None and claim_locator_kind(path, surface) in {
            "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
            "python_symbol", "proto_fqn",
        }:
            if exact_declaration_signature(path, symbol, baseline_blob) is None:
                return [], ["reviewed exact target symbol is absent from the source declaration inventory"]
        signature_before = (
            exact_declaration_signature(path, symbol, baseline_blob)
            if baseline_blob is not None and planned_signature is None and claim_locator_kind(path, surface) in {
                "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
                "python_symbol", "proto_fqn",
            }
            else None
        )
        signature_after = planned_signature or target_override.get("signature_after") or signature_before
        symbol_state = (
            "PLANNED" if planned_signature is not None
            else "EXISTING" if signature_before is not None
            else "NOT_APPLICABLE"
        )
        result.append({
            "path": path,
            "target_state": state,
            "surface_kind": surface,
            "locator_kind": claim_locator_kind(path, surface),
            "symbol_or_pointer": symbol,
            "symbol_state": symbol_state,
            "signature_before": signature_before,
            "signature_after": signature_after,
            "candidate_blob_sha256": (
                sha256_bytes(baseline_blob) if baseline_blob is not None else None
            ),
            "selection_reason": target_override["reason"],
        })
    return result, []


def unique_direct_claim_target(
    parent: dict[str, Any], pr: dict[str, Any]
) -> tuple[list[dict[str, Any]], list[str]]:
    pr_type = pr["pr_type"]
    tokens = claim_tokens(parent, pr)
    reviewed_override = exact_override_claim_target(parent, pr)
    if reviewed_override is not None:
        return reviewed_override
    if pr_type == "IDX":
        if pr.get("phase") == "task-completion" and parent.get("task_id"):
            task_slug = parent["task_id"].lower()
            targets = [
                make_claim_target(
                    f"doc/02_acceptance/topic1/tasks/{task_slug}/completion-candidate.json",
                    "immutable parent-task completion candidate output", tokens,
                    pr["primary_id"], planned_output=True,
                ),
                make_claim_target(
                    f"doc/02_acceptance/topic1/tasks/{task_slug}/current-evidence-index.json",
                    "immutable parent-task current evidence index output", tokens,
                    pr["primary_id"], planned_output=True,
                ),
            ]
            return [item for item in targets if item is not None], []
        path = f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/current-index.json"
        target = make_claim_target(path, "dedicated immutable current-index output", tokens, pr["primary_id"], planned_output=True)
        return ([target] if target else []), []
    if pr_type == "PROM":
        paths = [path for path in pr["candidate_paths"] if exact_claim_path(path)]
        path = paths[0] if len(paths) == 1 else f"contracts/releases/topic1/{pr['pr_id'].lower()}-release-pointer.json"
        target = make_claim_target(path, "dedicated release-pointer output", tokens, pr["primary_id"], planned_output=True)
        return ([target] if target else []), []

    if pr_type in {"TST-PRE", "TST-POST"}:
        review_key = claim_review_key(parent, pr)
        runner_override = (
            CLAIM_TEST_RUNNER_OVERRIDES.get(review_key)
            or CLAIM_TEST_SOURCE_OVERRIDES.get(review_key)
        )
        if runner_override is not None:
            runnable = [runner_override] if runner_override in pr["candidate_paths"] else []
        else:
            runnable = [
                path for path in sorted(set(pr["candidate_paths"]))
                if exact_claim_path(path) and claim_baseline_blob(path) is not None
                and claim_surface_kind(path) in {"TEST", "TEST_TOOL"}
            ]
        if not runnable:
            return [], ["no exact runnable test/tool target is bound"]
        best = max(path_score(path, tokens) for path in runnable)
        winners = [path for path in runnable if path_score(path, tokens) == best]
        if len(winners) != 1:
            return [], ["multiple runnable tests/tools match this leaf; bind one exact command target"]
        path = f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/test-result.json"
        target = make_claim_target(
            path, f"evidence output for read-only test target {winners[0]}", tokens,
            pr["primary_id"], planned_output=True,
        )
        targets = [target] if target else []
        generic_gate = CLAIM_GENERIC_EVIDENCE_MANIFEST_GATES.get(review_key)
        if generic_gate is not None:
            gate_target = make_claim_target(
                f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/evidence-{generic_gate.lower()}.json",
                f"immutable {generic_gate} evidence-run manifest bound to the leaf-specific PASS result",
                tokens, pr["primary_id"], planned_output=True,
            )
            if gate_target is not None:
                targets.append(gate_target)
        if review_key in CLAIM_NATIVE_EVIDENCE_MANIFEST_KEYS:
            for gate in ("g2", "g3"):
                gate_target = make_claim_target(
                    f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/evidence-{gate}.json",
                    f"immutable native {gate.upper()} evidence-run manifest produced by the reconciler",
                    tokens, pr["primary_id"], planned_output=True,
                )
                if gate_target is not None:
                    targets.append(gate_target)
        if review_key in CLAIM_REQUIRED_CASES and review_key not in CLAIM_NO_CASE_REPORT_KEYS:
            case_target = make_claim_target(
                f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/case-report.json",
                f"typed per-case output for read-only test target {winners[0]}", tokens,
                pr["primary_id"], planned_output=True,
            )
            if case_target is not None:
                targets.append(case_target)
        return targets, []

    candidates: list[dict[str, Any]] = []
    for path in sorted(set(pr["candidate_paths"])):
        target = make_claim_target(
            path, f"unique phase-compatible target for {pr['phase']}", tokens,
            pr["primary_id"],
        )
        if target and target["surface_kind"] in CLAIM_WRITE_POLICY[pr_type]:
            if pr_type == "UI" and not path.startswith("web/ui/"):
                continue
            if pr_type in {"PRJ", "WRT", "REF"} and path.startswith("web/ui/"):
                continue
            lower = f"{path} {target.get('symbol_or_pointer') or ''}".lower()
            if pr_type == "PRJ" and not any(
                token in lower for token in ("consumer", "project", "reconcile", "sink", "job", "reader")
            ):
                continue
            if pr_type == "WRT" and not any(
                token in lower for token in (
                    "repository", "writer", "outbox", "dispatcher", "service",
                    "handler", "command", "sender", "publisher", "api",
                )
            ):
                continue
            candidates.append(target)
    if not candidates:
        return [], ["no exact PR-type-compatible code/config/test target is bound"]
    scores = {item["path"]: path_score(item["path"], tokens) for item in candidates}
    best = max(scores.values())
    winners = [item for item in candidates if scores[item["path"]] == best]
    if len(winners) != 1:
        return [], [
            "candidate target is ambiguous; create a reviewed target binding with one write locator"
        ]
    chosen = winners[0]
    if chosen["target_state"] == "PLANNED" and chosen["surface_kind"] in {"SOURCE", "TEST", "TEST_TOOL"}:
        return [], ["planned code target lacks a reviewed signature, compatibility seam and default-off guard"]
    return [chosen], []


def claim_read_context(parent: dict[str, Any], pr: dict[str, Any], writes: list[dict[str, Any]]) -> list[str]:
    write_paths = {item["path"] for item in writes}
    review_key = claim_review_key(parent, pr)
    derived_output = CLAIM_DERIVED_OUTPUTS.get(review_key)
    own_generated_paths = {derived_output[1]} if derived_output is not None else set()
    candidates = {
        path for path in pr["candidate_paths"]
        if exact_claim_path(path) and (
            claim_baseline_blob(path) is not None
            or review_key in CLAIM_REVIEWED_DIRECT_KEYS
        )
    }
    canonical_ids = pr.get("canonical_ids", []) or [pr["primary_id"]]
    historical = canonical_existing_paths()
    for canonical_id in canonical_ids:
        candidates.update(historical.get(canonical_id, []))
    if review_key in CLAIM_REVIEWED_DIRECT_KEYS and pr["pr_type"] in {"TST-PRE", "TST-POST"}:
        candidates.add("contracts/alignment/evidence-run-manifest.schema.json")
    if review_key in CLAIM_REQUIRED_CASES:
        candidates.add("contracts/alignment/evidence-case-report.schema.json")
        candidates.add("contracts/alignment/evidence-case-fixture.schema.json")
    resolved = sorted(
        path for path in candidates
        if path not in write_paths and path not in own_generated_paths
    )
    # Reviewed exact leaves must expose their complete authority input set.
    # The generic cap is only a discovery safeguard for unresolved leaves; it
    # must not silently truncate migrations, Proto descriptors or callsites
    # after a task has been promoted to a direct developer work order.
    return (
        resolved
        if review_key in CLAIM_REVIEWED_DIRECT_KEYS
        else resolved[:80]
    )


def claim_dependency_contracts(
    pr: dict[str, Any], *, target_binding: bool = False
) -> list[dict[str, str]]:
    required_status = "DECLARED" if target_binding else "PASS"
    blocking_mode = "ASSIGNMENT_NONBLOCKING" if target_binding else "START_BLOCKED"
    result = [
        {
            "dependency_id": dependency,
            "dependency_kind": "ATOMIC_PR",
            "required_status": required_status,
            "consumed_artifact": (
                "registry-leaf" if target_binding else
                "task-current-evidence-index" if dependency.endswith("-task-completion")
                else "atomic-pr-receipt"
            ),
            "blocking_mode": blocking_mode,
        }
        for dependency in pr["depends_on_prs"]
    ]
    result.extend(
        {
            "dependency_id": dependency,
            "dependency_kind": "EXTERNAL_ACTIVITY",
            "required_status": required_status,
            "consumed_artifact": "registry-leaf" if target_binding else "external-activity-receipt",
            "blocking_mode": blocking_mode,
        }
        for dependency in pr["depends_on_external_activities"]
    )
    return result


def claim_checks(
    parent: dict[str, Any], pr: dict[str, Any], paths: list[str], evidence_output: str,
    case_report_output: str | None = None,
) -> list[dict[str, str]]:
    review_key = claim_review_key(parent, pr)
    commands: list[tuple[str, str]] = []
    exact_test_command = CLAIM_TEST_COMMAND_OVERRIDES.get(review_key)
    if exact_test_command is not None:
        commands.append(("go-exact-test", exact_test_command))
        if evidence_output is not None:
            commands.append(("result-json", f"python3 -m json.tool {evidence_output}"))
    elif any(path.startswith("go/control-plane/") for path in paths):
        commands.append(("go", "cd go/control-plane && go test ./..."))
    if any(path.startswith("rust/probe-agent/") for path in paths):
        commands.append(("rust", "cd rust/probe-agent && cargo test --workspace"))
    if any(path.startswith("java/flink-jobs/") for path in paths):
        commands.append(("java", "cd java/flink-jobs && mvn test"))
    if any(path.startswith("web/ui/") for path in paths):
        commands.extend([
            ("web-test", "cd web/ui && npm run test -- --run"),
            ("web-build", "cd web/ui && npm run build"),
        ])
    if any(path.startswith("proto/") for path in paths):
        commands.append(("proto", "cd proto && buf lint"))
    python_paths = sorted(
        path for path in set(paths)
        if path.endswith(".py") and (
            claim_baseline_blob(path) is not None
            or review_key in CLAIM_REVIEWED_DIRECT_KEYS
        )
    )
    if python_paths:
        commands.append((
            "python-compile",
            "python3 -m py_compile " + " ".join(python_paths),
        ))
    for index, path in enumerate(paths, start=1):
        if path.startswith("deployments/") and path.endswith((".yaml", ".yml")):
            commands.append((f"manifest-{index}", f"kubectl apply --dry-run=client -f {path}"))
        if path.endswith(".json") and path not in {
            evidence_output, case_report_output,
        }:
            commands.append((f"json-{index}", f"python3 -m json.tool {path}"))
    builder_command = CLAIM_BUILDER_COMMANDS.get(review_key)
    if builder_command is not None:
        commands.append(("builder-self-check", builder_command))
    semantic_builder = CLAIM_SEMANTIC_BUILDERS.get(review_key)
    if semantic_builder is not None:
        builder_path, instance_path, schema_path = semantic_builder
        commands.append((
            "deterministic-rebuild",
            f"python3 {builder_path} --check --result {evidence_output}",
        ))
        commands.append((
            "json-contract",
            "python3 scripts/alignment/build_topic1_task_registry.py "
            f"--check-json-contract {instance_path} {schema_path}",
        ))
        commands.append((
            "result-json",
            f"python3 -m json.tool {evidence_output}",
        ))
        if case_report_output is not None:
            commands.append((
                "case-report-json",
                f"python3 -m json.tool {case_report_output}",
            ))
    runner = CLAIM_TEST_RUNNER_OVERRIDES.get(review_key)
    if runner is not None and semantic_builder is None and exact_test_command is None:
        mode = (
            "protected-positive" if pr["pr_type"] == "TST-POST"
            else "fail-closed" if "trusted-signature" in review_key
            else "matrix"
        )
        commands.append((
            "declared-case-matrix",
            f"python3 {runner} --mode {mode} --result {evidence_output}"
            + (f" --case-report {case_report_output}" if case_report_output else ""),
        ))
        commands.append((
            "result-json",
            f"python3 -m json.tool {evidence_output}",
        ))
        if case_report_output is None:
            raise ValueError(f"{review_key} test runner lacks an exact case-report output")
        commands.append((
            "case-report-json",
            f"python3 -m json.tool {case_report_output}",
        ))
        commands.append((
            "evidence-run-contract",
            "python3 scripts/alignment/build_topic1_task_registry.py "
            f"--check-evidence-run-manifest {evidence_output} {case_report_output}",
        ))
    component_case = CLAIM_COMPONENT_TEST_CASES.get(review_key)
    if component_case is not None:
        runner_path = (
            "scripts/alignment/test_candidate_freeze.py"
            if review_key.startswith("T1-M01-N004::")
            else "scripts/alignment/test_trusted_signature_verifier.py"
        )
        commands.append((
            "component-contract",
            f"python3 {runner_path} --mode component --case {component_case}",
        ))
    if review_key == "T1-M01-N010::trusted-verifier-protected-backend::OPS":
        commands.append((
            "deployment-security-contract",
            "python3 scripts/alignment/test_trusted_signature_verifier.py "
            "--mode deployment --manifest deployments/security/topic1-trusted-signature-verifier.yaml",
        ))
    if review_key.startswith("T1-M01-N010::caller-"):
        case_id = pr["phase"].removeprefix("caller-")
        commands.append((
            "caller-contract",
            "python3 scripts/alignment/test_trusted_signature_verifier.py "
            f"--mode caller --case {case_id}",
        ))
    commands.append(("registry", "python3 scripts/alignment/build_topic1_task_registry.py --check"))
    return [
        {
            "check_id": f"{pr['pr_id']}-{name}",
            "command": command,
            "oracle": f"command exits 0 and {claim_leaf_oracle(parent, pr)}",
            "evidence_output": evidence_output,
        }
        for name, command in dict(commands).items()
    ]


def registered_case_rejection_code(case_id: str, outcome: str) -> str | None:
    return (
        None if outcome == "PASS"
        else "REJECT_" + case_id.replace("-", "_").upper()
    )


def validate_case_report_semantics(
    report: dict[str, Any], expected_case_specs: tuple[dict[str, Any], ...],
    *, require_positive_attestation: bool,
) -> None:
    expected = {item["case_id"]: item for item in expected_case_specs}
    case_ids = [item["case_id"] for item in report["cases"]]
    if len(case_ids) != len(set(case_ids)) or set(case_ids) != set(expected):
        raise ValueError("case report does not contain the exact registered case set")
    fixture_by_id = {item["artifact_id"]: item for item in report["fixture_artifacts"]}
    expected_fixture_ids = {f"FIXTURE-{case_id}" for case_id in expected}
    fixture_paths = [item["path"] for item in report["fixture_artifacts"]]
    if (
        len(fixture_by_id) != len(report["fixture_artifacts"])
        or set(fixture_by_id) != expected_fixture_ids
        or len(fixture_paths) != len(set(fixture_paths))
    ):
        raise ValueError("case report fixtures do not match the exact registered case set")
    for artifact in report["fixture_artifacts"]:
        validate_hashed_artifact(REPO_ROOT, artifact["path"], artifact["sha256"])
    for item in report["cases"]:
        spec = expected[item["case_id"]]
        expected_outcome = spec["outcome"]
        expected_rejection = registered_case_rejection_code(
            item["case_id"], expected_outcome,
        )
        fixture = fixture_by_id[f"FIXTURE-{item['case_id']}"]
        if fixture["path"] != spec["fixture_path"]:
            raise ValueError(
                f"case {item['case_id']} fixture path differs from its registered recipe"
            )
        fixture_payload = json.loads(
            (REPO_ROOT / fixture["path"]).read_text(encoding="utf-8")
        )
        validate_against_schema(fixture_payload, EVIDENCE_CASE_FIXTURE_SCHEMA_PATH)
        if fixture_payload != spec["fixture_payload"]:
            raise ValueError(
                f"case {item['case_id']} fixture body differs from its registered recipe"
            )
        if (
            item["expected_outcome"] != expected_outcome
            or item["actual_outcome"] != expected_outcome
            or item["status"] != "PASS"
            or item["rejection_code"] != expected_rejection
            or item["input_sha256s"] != [fixture["sha256"]]
        ):
            raise ValueError(f"case {item['case_id']} did not meet its registered oracle")
        output_fact = {
            "case_id": item["case_id"],
            "expected_outcome": item["expected_outcome"],
            "actual_outcome": item["actual_outcome"],
            "status": item["status"],
            "rejection_code": item["rejection_code"],
            "fixture_artifact_id": fixture["artifact_id"],
            "input_sha256s": item["input_sha256s"],
        }
        expected_output_sha = sha256_bytes(
            canonical_json(output_fact).encode("utf-8")
        )
        if item["output_sha256s"] != [expected_output_sha]:
            raise ValueError(f"case {item['case_id']} output hash is not bound to its canonical result")
    summary = report["summary"]
    if (
        summary["expected_case_count"] != len(expected_case_specs)
        or summary["passed_case_count"] != len(expected_case_specs)
        or summary["failed_case_count"] != 0
        or report["result"] != "PASS"
    ):
        raise ValueError("case report summary is not an exact PASS closure")
    if require_positive_attestation != (report["positive_attestation"] is not None):
        raise ValueError("positive attestation presence differs from the registered test leaf")


def validate_work_order_evidence_run(
    manifest_path: Path, report_path: Path,
    package: dict[str, Any], expected_case_specs: tuple[dict[str, Any], ...],
) -> None:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    report = json.loads(report_path.read_text(encoding="utf-8"))
    validate_against_schema(manifest, EVIDENCE_RUN_MANIFEST_SCHEMA_PATH)
    validate_against_schema(report, EVIDENCE_CASE_REPORT_SCHEMA_PATH)
    expected_pr = package["atomic_pr_id"]
    expected_work = package["parent_work_id"]
    expected_milestone = package["milestone_id"]
    identity_fields = ("candidate_manifest_sha256", "profile_id", "environment_id")
    if (
        manifest["subject_pr_id"] != expected_pr
        or manifest["subject_work_id"] != expected_work
        or manifest["subject_milestone_id"] != expected_milestone
        or report["subject_pr_id"] != expected_pr
        or report["subject_work_id"] != expected_work
        or report["subject_milestone_id"] != expected_milestone
        or any(report[field] != manifest[field] for field in identity_fields)
        or manifest["result"] != "PASS"
        or manifest["run_purpose"] != "VERIFICATION"
        or manifest["gate_id"] not in package["required_gates"]
        or manifest["production_applied"]
    ):
        raise ValueError("evidence run identity/result differs from its exact work order")
    review_key = (
        f"{expected_work}::{package['outcome']['subject']}::"
        f"{package['pr_type']}"
    )
    registered_runner = CLAIM_TEST_RUNNER_OVERRIDES.get(review_key)
    if (
        registered_runner is not None
        and report["runner_artifact"]["path"] != registered_runner
    ):
        raise ValueError("case report runner differs from the exact registered test runner")
    expected_report_rel = next(
        item["path"] for item in package["generated_outputs"]
        if item["artifact_id"] == f"CASE-REPORT-{expected_pr}"
    )
    actual_report_rel = report_path.relative_to(REPO_ROOT).as_posix()
    if actual_report_rel != expected_report_rel:
        raise ValueError("case report path differs from the registered work-order output")
    report_sha = sha256_bytes(report_path.read_bytes())
    expected_artifact_refs = [{
        "direction": "INPUT",
        "artifact_id": f"RUNNER-{expected_pr}",
        "path": report["runner_artifact"]["path"],
        "sha256": report["runner_artifact"]["sha256"],
        "schema_ref": None,
    }, *[
        {
            "direction": "INPUT",
            "artifact_id": artifact["artifact_id"],
            "path": artifact["path"],
            "sha256": artifact["sha256"],
            "schema_ref": None,
        }
        for artifact in report["fixture_artifacts"]
    ], {
        "direction": "OUTPUT",
        "artifact_id": f"CASE-REPORT-{expected_pr}",
        "path": expected_report_rel,
        "sha256": report_sha,
        "schema_ref": "contracts/alignment/evidence-case-report.schema.json",
    }]
    require_positive = package["outcome"]["subject"] == "trusted-signature-positive-run"
    if require_positive:
        attestation = report["positive_attestation"]
        expected_artifact_refs.append({
            "direction": "INPUT",
            "artifact_id": f"POSITIVE-ATTESTATION-{expected_pr}",
            "path": attestation["artifact"]["path"],
            "sha256": attestation["artifact"]["sha256"],
            "schema_ref": "contracts/alignment/signature-verification-attestation.schema.json",
        })
    actual_ref_ids = [item["artifact_id"] for item in manifest["artifacts"]]
    if (
        len(actual_ref_ids) != len(set(actual_ref_ids))
        or {
            canonical_json(item) for item in manifest["artifacts"]
        } != {
            canonical_json(item) for item in expected_artifact_refs
        }
    ):
        raise ValueError("evidence run does not bind the exact runner, fixture, case-report and attestation closure")
    validate_hashed_artifact(REPO_ROOT, 
        report["runner_artifact"]["path"], report["runner_artifact"]["sha256"]
    )
    validate_case_report_semantics(
        report, expected_case_specs, require_positive_attestation=require_positive,
    )
    if require_positive:
        attestation = report["positive_attestation"]
        if any(attestation[field] != report[field] for field in identity_fields):
            raise ValueError("positive attestation identity differs from the case report")
        validate_hashed_artifact(REPO_ROOT, 
            attestation["artifact"]["path"], attestation["artifact"]["sha256"]
        )
        attestation_path = REPO_ROOT / attestation["artifact"]["path"]
        attestation_payload = json.loads(attestation_path.read_text(encoding="utf-8"))
        validate_against_schema(
            attestation_payload,
            REPO_ROOT / "contracts/alignment/signature-verification-attestation.schema.json",
        )
        require_trusted_signature_verifier(
            f"{expected_pr} protected positive work-order attestation"
        )


def convert_claim_package_to_target_binding(
    package: dict[str, Any], pr: dict[str, Any], reason: str
) -> None:
    prior_target_paths = [
        item["path"] for item in package["change_targets"]
        if claim_baseline_blob(item["path"]) is not None
    ]
    binding_path = (
        f"contracts/alignment/code-target-bindings/{package['milestone_id'].lower()}/"
        f"{package['atomic_pr_id'].lower()}.json"
    )
    package["claim_status"] = "TARGET_BINDING_CLAIMABLE"
    package["claim_mode"] = "TARGET_BINDING"
    package["direct_target_bound"] = False
    package["dependency_contracts"] = claim_dependency_contracts(
        pr, target_binding=True
    )
    package["outcome"]["state_change"] = (
        "replace unresolved or conflicting targets with one reviewed code-target binding"
    )
    package["outcome"]["acceptance_oracle"] = (
        "the binding checker resolves one exact non-conflicting target and preserves formal execution blockers"
    )
    package["change_targets"] = [
        {
            "path": binding_path,
            "target_state": "PLANNED_OUTPUT",
            "surface_kind": "TARGET_BINDING",
            "locator_kind": "json_pointer",
            "symbol_or_pointer": "/",
            "symbol_state": "NOT_APPLICABLE",
            "signature_before": None,
            "signature_after": None,
            "candidate_blob_sha256": None,
            "selection_reason": "dedicated target-binding artifact for this atomic PR",
        }
    ]
    package["read_context_paths"] = sorted(
        set(package["read_context_paths"] + prior_target_paths)
    )
    package["generated_outputs"] = [
        {
            "artifact_id": f"ARTIFACT-{package['atomic_pr_id']}",
            "path": binding_path,
            "artifact_kind": "TARGET_BINDING",
        }
    ]
    package["implementation_steps"] = [
        {
            "step_id": "S1",
            "action": (
                "run the bounded read-only locator search `"
                + claim_target_discovery_command({"action": ""}, pr)
                + "`; inspect the candidates and identify one authoritative, non-conflicting write seam"
            ),
            "target_path": binding_path,
            "expected_effect": "one exact repo path and symbol/pointer are selected for this PR type",
        },
        {
            "step_id": "S2",
            "action": "create the target-binding JSON with candidate, responsibility, signature, test oracle and rollback",
            "target_path": binding_path,
            "expected_effect": "the binding validates against canonical-pr-target-binding.schema.json",
        },
        {
            "step_id": "S3",
            "action": "run the target-binding checker without modifying domain production code",
            "target_path": binding_path,
            "expected_effect": "the checker accepts one type-compatible locator and rejects broad or historical evidence paths",
        },
    ]
    package["verification_checks"] = [
        {
            "check_id": f"{package['atomic_pr_id']}-target-binding",
            "command": (
                "python3 scripts/alignment/build_topic1_task_registry.py "
                f"--check-target-binding {binding_path}"
            ),
            "oracle": "checker exits 0 with REVIEWABLE_TARGET_BINDING and no domain file is changed",
            "evidence_output": binding_path,
        },
        {
            "check_id": f"{package['atomic_pr_id']}-registry",
            "command": "python3 scripts/alignment/build_topic1_task_registry.py --check",
            "oracle": "all generated registries remain structurally current",
            "evidence_output": binding_path,
        },
    ]
    package["rollback"] = {
        "runbook_id": pr["rollback_runbook_id"],
        "trigger": "binding validation fails or the selected locator conflicts with another work order",
        "steps": [
            "revert only the unapproved target-binding artifact",
            "regenerate the claim catalog and confirm the leaf remains in target-binding mode",
        ],
        "oracle": "the prior catalog validates and no domain or durable runtime state changes",
        "durable_data_policy": "target binding never deletes or mutates runtime facts, offsets, objects or evidence",
    }
    package["allowed_claim"] = "one reviewable target binding exists for this atomic PR"
    if "target binding is not domain implementation" not in package["forbidden_claim"]:
        package["forbidden_claim"] += "; target binding is not domain implementation"
    package["implementation_blockers"] = sorted(
        set(package["implementation_blockers"] + [reason])
    )


def build_developer_claim_catalog(registry: dict[str, Any]) -> dict[str, Any]:
    packages: list[dict[str, Any]] = []
    registry_sha = sha256_bytes(canonical_json(registry).encode("utf-8"))
    baseline_commit = claim_baseline_commit()

    def add(parent: dict[str, Any], pr: dict[str, Any], owner_role: str) -> None:
        parent_id = parent.get("task_id", parent.get("slice_id"))
        milestone_id = parent.get("milestone_id", "T1-M13")
        direct_targets, implementation_blockers = unique_direct_claim_target(parent, pr)
        direct = bool(direct_targets) and not implementation_blockers
        terminal_completion = (
            pr["pr_type"] == "IDX" and pr.get("phase") == "task-completion"
            and parent.get("task_id") is not None
        )
        binding_path = (
            f"contracts/alignment/code-target-bindings/{milestone_id.lower()}/"
            f"{pr['pr_id'].lower()}.json"
        )
        if direct:
            writes = direct_targets
            status = "DIRECT_TARGET_BOUND_CLAIMABLE"
            mode = "DIRECT_TARGET_BOUND"
            output_path = (
                writes[-1]["path"] if terminal_completion else
                writes[-1]["path"] if pr["pr_type"] in {"IDX", "PROM"} else
                writes[0]["path"] if pr["pr_type"] in {"TST-PRE", "TST-POST"} else
                f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/claim-result.json"
            )
            output_kind = (
                "CURRENT_INDEX" if pr["pr_type"] == "IDX"
                else "RELEASE_POINTER" if pr["pr_type"] == "PROM" else "EVIDENCE"
            )
            case_report_path = (
                f"doc/02_acceptance/topic1/work-orders/{pr['pr_id'].lower()}/case-report.json"
                if claim_review_key(parent, pr) in CLAIM_REQUIRED_CASES
                and claim_review_key(parent, pr) not in CLAIM_NO_CASE_REPORT_KEYS
                else None
            )
            first_target = writes[0]["path"]
            steps = (
                [
                    {
                        "step_id": "S1",
                        "action": "load the exact completion contract, every PASS leaf receipt, dependency TASK-IDX, external receipt, evidence run and rollback result",
                        "target_path": writes[0]["path"],
                        "expected_effect": "all referenced artifacts resolve under one candidate/profile/environment and match the parent-task contract",
                    },
                    {
                        "step_id": "S2",
                        "action": "write the immutable task completion candidate with exact leaf, run, output, rollback and claim-ceiling closure",
                        "target_path": writes[0]["path"],
                        "expected_effect": "completion result is PASS only when the exact artifact closure has no blockers",
                    },
                    {
                        "step_id": "S3",
                        "action": "run the task-completion bundle checker and publish the current task index only after the completion candidate passes",
                        "target_path": writes[1]["path"],
                        "expected_effect": "one PASS current task index points to the immutable completion candidate and signed TASK-IDX execution receipt",
                    },
                ]
                if terminal_completion else [
                    {
                        "step_id": "S1",
                        "action": "read the parent contract and all PASS dependency receipts before editing",
                        "target_path": first_target,
                        "expected_effect": "the next action remains blocked until the declared candidate, dependencies and signed execution overlay are all valid",
                    },
                    *[
                        {
                            "step_id": f"S{index + 2}",
                            "action": (
                                (
                                    f"run the declared test tool to produce only bound {pr['pr_type']} "
                                    f"output {target['path']}"
                                    if pr["pr_type"] in {"TST-PRE", "TST-POST"}
                                    else f"write only bound {pr['pr_type']} output {target['path']}"
                                    if pr["pr_type"] in {"IDX", "PROM"}
                                    else f"modify only bound {pr['pr_type']} target {target['path']} "
                                    "and preserve compatibility/default-off constraints"
                                )
                            ),
                            "target_path": target["path"],
                            "expected_effect": claim_leaf_oracle(parent, pr),
                        }
                        for index, target in enumerate(writes)
                    ],
                    {
                        "step_id": f"S{len(writes) + 2}",
                        "action": "run every verification check and write the immutable result artifact",
                        "target_path": output_path,
                        "expected_effect": "the evidence output records command, candidate identity, result and artifact hashes",
                    },
                ]
            )
        else:
            writes = [
                {
                    "path": binding_path,
                    "target_state": "PLANNED_OUTPUT",
                    "surface_kind": "TARGET_BINDING",
                    "locator_kind": "json_pointer",
                    "symbol_or_pointer": "/",
                    "symbol_state": "NOT_APPLICABLE",
                    "signature_before": None,
                    "signature_after": None,
                    "candidate_blob_sha256": None,
                    "selection_reason": "dedicated target-binding artifact for this atomic PR",
                }
            ]
            status = "TARGET_BINDING_CLAIMABLE"
            mode = "TARGET_BINDING"
            output_path = binding_path
            output_kind = "TARGET_BINDING"
            steps = [
                {
                    "step_id": "S1",
                    "action": (
                        "run the bounded read-only locator search `"
                        + claim_target_discovery_command(parent, pr)
                        + "`; inspect every candidate and identify the single authoritative write seam"
                    ),
                    "target_path": binding_path,
                    "expected_effect": "one exact repo path and symbol/pointer are selected for this PR type",
                },
                {
                    "step_id": "S2",
                    "action": "create the target-binding JSON with candidate commit, owner, expected signature, test oracle and rollback",
                    "target_path": binding_path,
                    "expected_effect": "the binding validates against canonical-pr-target-binding.schema.json",
                },
                {
                    "step_id": "S3",
                    "action": "run the target-binding checker; do not modify domain production code in this binding PR",
                    "target_path": binding_path,
                    "expected_effect": "the checker accepts one unambiguous type-compatible target and rejects broad or historical evidence paths",
                },
            ]
        read_context = claim_read_context(parent, pr, writes)
        reviewed_steps = CLAIM_IMPLEMENTATION_STEP_OVERRIDES.get(
            claim_review_key(parent, pr)
        )
        if direct and reviewed_steps is not None:
            steps = reviewed_steps
        outcome = {
            "result_id": f"RESULT-{pr['pr_id']}",
            "subject": pr["phase"],
            "state_change": (
                "materialize the exact PASS parent-task completion candidate and its immutable current evidence index"
                if terminal_completion else
                f"deliver the bounded {pr['pr_type']} leaf on its exact write target after the signed overlay authorizes execution"
                if direct else "replace unresolved target candidates with one reviewed code-target binding"
            ),
            "acceptance_oracle": (
                "every declared leaf/dependency/external/evidence/rollback/output artifact resolves and one same-identity PASS current task index is published"
                if terminal_completion else
                claim_leaf_oracle(parent, pr)
                if direct else "the binding checker resolves one exact target and preserves all formal execution blockers"
            ),
            "non_goals": (
                [
                    "closes only this parent task; it does not satisfy a requirement or close a milestone",
                    "does not author external or CNAS attestation",
                    "does not authorize production code or runtime changes",
                ]
                if terminal_completion else [
                    "does not close the parent task or milestone",
                    "does not authorize execution without a signed overlay",
                    "does not treat historical evidence as a writable target",
                ]
            ),
        }
        if direct:
            check_paths = [item["path"] for item in writes]
            if pr["pr_type"] in {"TST-PRE", "TST-POST"}:
                check_paths.extend(read_context)
            checks = claim_checks(
                parent, pr, check_paths, output_path,
                case_report_path if direct else None,
            )
            if (
                pr["pr_type"] == "IDX" and pr.get("phase") == "task-completion"
                and parent.get("task_id") is not None
            ):
                checks.insert(
                    0,
                    {
                        "check_id": f"{pr['pr_id']}-task-completion-bundle",
                        "command": (
                            "python3 scripts/alignment/build_topic1_task_registry.py "
                            f"--check-task-completion-bundle {writes[0]['path']} {writes[1]['path']} "
                            f"doc/02_acceptance/topic1/tasks/{parent['task_id'].lower()}/signed-execution-instance.json"
                        ),
                        "oracle": (
                            "checker first validates the signed execution instance with the trusted verifier, "
                            "then loads every leaf/dependency/external/evidence/rollback/output artifact and "
                            "accepts one same-candidate PASS task completion/current-index bundle"
                        ),
                        "evidence_output": writes[1]["path"],
                    },
                )
        else:
            checks = [
                {
                    "check_id": f"{pr['pr_id']}-target-binding",
                    "command": (
                        "python3 scripts/alignment/build_topic1_task_registry.py "
                        f"--check-target-binding {binding_path}"
                    ),
                    "oracle": "checker exits 0 with REVIEWABLE_TARGET_BINDING and no domain file is changed",
                    "evidence_output": binding_path,
                },
                {
                    "check_id": f"{pr['pr_id']}-registry",
                    "command": "python3 scripts/alignment/build_topic1_task_registry.py --check",
                    "oracle": "all generated registries remain structurally current",
                    "evidence_output": binding_path,
                },
            ]
        generated_outputs = (
            [
                {
                    "artifact_id": f"TASK-COMPLETION-{parent_id}",
                    "path": writes[0]["path"],
                    "artifact_kind": "EVIDENCE",
                },
                {
                    "artifact_id": f"TASK-CURRENT-INDEX-{parent_id}",
                    "path": writes[1]["path"],
                    "artifact_kind": "CURRENT_INDEX",
                },
            ]
            if direct and pr["pr_type"] == "IDX"
            and pr.get("phase") == "task-completion"
            and parent.get("task_id") is not None
            else [
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}",
                    "path": output_path,
                    "artifact_kind": output_kind,
                }
            ]
        )
        if direct and claim_review_key(parent, pr) in CLAIM_NATIVE_EVIDENCE_MANIFEST_KEYS:
            generated_outputs = [
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}-RESULT",
                    "path": writes[0]["path"],
                    "artifact_kind": "EVIDENCE",
                },
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}-G2",
                    "path": writes[1]["path"],
                    "artifact_kind": "EVIDENCE",
                },
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}-G3",
                    "path": writes[2]["path"],
                    "artifact_kind": "EVIDENCE",
                },
            ]
        generic_gate = CLAIM_GENERIC_EVIDENCE_MANIFEST_GATES.get(
            claim_review_key(parent, pr)
        )
        if direct and generic_gate is not None:
            generated_outputs = [
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}-RESULT",
                    "path": writes[0]["path"],
                    "artifact_kind": "EVIDENCE",
                },
                {
                    "artifact_id": f"ARTIFACT-{pr['pr_id']}-{generic_gate}",
                    "path": writes[1]["path"],
                    "artifact_kind": "EVIDENCE",
                },
            ]
        if direct and case_report_path is not None:
            generated_outputs.append({
                "artifact_id": f"CASE-REPORT-{pr['pr_id']}",
                "path": case_report_path,
                "artifact_kind": "EVIDENCE",
            })
        derived_output = CLAIM_DERIVED_OUTPUTS.get(claim_review_key(parent, pr))
        if derived_output is not None:
            artifact_id, artifact_path = derived_output
            generated_outputs.append({
                "artifact_id": f"{artifact_id}-{pr['pr_id']}",
                "path": artifact_path,
                "artifact_kind": "DERIVED_CONTRACT",
            })
        packages.append(
            {
                "work_order_id": f"WO-{pr['pr_id']}",
                "source_task_registry_sha256": registry_sha,
                "baseline_candidate_commit": baseline_commit,
                "atomic_pr_id": pr["pr_id"],
                "parent_work_id": parent_id,
                "milestone_id": milestone_id,
                "pr_type": pr["pr_type"],
                "claim_status": status,
                "claim_mode": mode,
                "direct_target_bound": direct,
                "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
                "owner_role": owner_role,
                "primary_id": pr["primary_id"],
                "dependency_contracts": claim_dependency_contracts(
                    pr, target_binding=not direct
                ),
                "outcome": outcome,
                "change_targets": writes,
                "read_context_paths": read_context,
                "generated_outputs": generated_outputs,
                "implementation_steps": steps,
                "verification_checks": checks,
                "rollback": {
                    "runbook_id": pr["rollback_runbook_id"],
                    "trigger": "any required check fails, compatibility drifts, or a stop threshold is crossed",
                    "steps": (
                        [
                            "revert only the unapproved target-binding artifact",
                            "regenerate the claim catalog and confirm the leaf returns to target-binding mode",
                        ]
                        if not direct else [
                            "do not publish the new task current index; retain the failed completion candidate as immutable evidence",
                            "restore the previous signed task current-index pointer when one exists",
                            "re-run exact leaf/dependency/evidence/rollback reconciliation before a new completion candidate",
                        ]
                        if terminal_completion else
                        CLAIM_ROLLBACK_OVERRIDES.get(claim_review_key(parent, pr), [
                            "stop new intake or keep the new path default-off before reverting",
                            "restore the prior compatible code/configuration without deleting durable facts",
                            "reconcile in-flight work and verify the prior path on the same candidate profile",
                        ])
                    ),
                    "oracle": (
                        "the previous signed task current index remains authoritative and no immutable completion/evidence artifact is deleted"
                        if terminal_completion else
                        "the prior compatible path passes its checks and no durable fact is deleted or regressed"
                    ),
                    "durable_data_policy": "preserve authority rows, audit, offsets, objects and immutable evidence; use compensating/reconcile work",
                },
                "required_gates": pr["required_gates"],
                "allowed_claim": (
                    CLAIM_ALLOWED_CLAIM_OVERRIDES[claim_review_key(parent, pr)]
                    if claim_review_key(parent, pr) in CLAIM_ALLOWED_CLAIM_OVERRIDES
                    else "the exact parent task leaf, dependency, evidence, output and rollback closure is indexed for the same candidate/profile/environment"
                    if direct and pr["pr_type"] == "IDX" and pr.get("phase") == "task-completion"
                    else parent.get("allowed_claim_template", "bounded release promotion only")
                    if direct and pr["pr_type"] == "PROM"
                    else "; ".join(pr.get("proves", []))
                    if direct and pr.get("proves")
                    else f"the exact {pr['pr_type']} target is bound and the bounded leaf result may be claimed only after signed execution and evidence validation"
                    if direct
                    else "one reviewable target binding exists for this atomic PR"
                ),
                "forbidden_claim": (
                    CLAIM_FORBIDDEN_CLAIM_OVERRIDES[claim_review_key(parent, pr)]
                    if claim_review_key(parent, pr) in CLAIM_FORBIDDEN_CLAIM_OVERRIDES
                    else parent.get("forbidden_claim_template", "no milestone or project completion claim")
                    + ("" if direct else "; target binding is not domain implementation")
                ),
                "limits": {
                    "max_handwritten_loc": pr["max_handwritten_loc"],
                    "max_production_files": pr["max_production_files"],
                    "max_expand_migrations": pr["max_expand_migrations"],
                    "max_event_or_api_versions": pr["max_event_or_api_versions"],
                },
                "implementation_blockers": implementation_blockers,
            }
        )

    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            add(task, pr, task["responsibility"]["owner_role"])
    for item in registry["closure_slices"]:
        for pr in item["pr_sequence"]:
            add(item, pr, "+".join(item["owner_roles"]))

    nodes = execution_nodes(registry["tasks"], registry["closure_slices"])
    packages_by_id = {item["atomic_pr_id"]: item for item in packages}
    pr_by_id = {
        pr["pr_id"]: pr
        for parent in [*registry["tasks"], *registry["closure_slices"]]
        for pr in parent["pr_sequence"]
    }
    locator_users: dict[tuple[str, str | None], list[str]] = {}
    for package in packages:
        if not package["direct_target_bound"]:
            continue
        for target in package["change_targets"]:
            locator_users.setdefault(
                (target["path"], target["symbol_or_pointer"]), []
            ).append(package["atomic_pr_id"])
    conflicting_ids: set[str] = set()
    for users in locator_users.values():
        for index, left in enumerate(users):
            for right in users[index + 1:]:
                if not execution_has_ancestor(nodes, left, right) and not execution_has_ancestor(
                    nodes, right, left
                ):
                    conflicting_ids.update((left, right))
    for pr_id in sorted(conflicting_ids):
        convert_claim_package_to_target_binding(
            packages_by_id[pr_id], pr_by_id[pr_id],
            "exact write locator is shared by unordered work orders; bind a distinct symbol/pointer",
        )
    direct_count = sum(item["direct_target_bound"] for item in packages)
    return {
        "schema_version": "2.2.0",
        "source_task_registry_sha256": registry_sha,
        "baseline_candidate_commit": baseline_commit,
        "package_count": len(packages),
        "next_action_claimable_count": len(packages),
        "direct_target_count": direct_count,
        "target_binding_count": len(packages) - direct_count,
        "next_action_unclaimable_count": 0,
        "formal_execution_blocked_count": len(packages),
        "packages": packages,
    }


def validate_developer_claim_catalog(catalog: dict[str, Any], registry: dict[str, Any]) -> None:
    expected: dict[str, tuple[dict[str, Any], dict[str, Any]]] = {}
    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            expected[pr["pr_id"]] = (task, pr)
    for item in registry["closure_slices"]:
        for pr in item["pr_sequence"]:
            expected[pr["pr_id"]] = (item, pr)
    registry_sha = sha256_bytes(canonical_json(registry).encode("utf-8"))
    actual_ids = [item["atomic_pr_id"] for item in catalog["packages"]]
    if len(actual_ids) != len(set(actual_ids)) or set(actual_ids) != set(expected):
        raise ValueError("developer claim catalog must cover every atomic PR exactly once")
    packages_by_id = {item["atomic_pr_id"]: item for item in catalog["packages"]}

    @lru_cache(maxsize=None)
    def dependency_artifact_paths(pr_id: str) -> frozenset[str]:
        _, pr = expected[pr_id]
        result: set[str] = set()
        for dependency_id in pr.get("depends_on_prs", []):
            dependency_package = packages_by_id[dependency_id]
            result.update(
                item["path"] for item in dependency_package["change_targets"]
            )
            result.update(
                item["path"] for item in dependency_package["generated_outputs"]
            )
            result.update(dependency_artifact_paths(dependency_id))
        return frozenset(result)

    reviewed_packages = {
        f"{item['parent_work_id']}::{item['outcome']['subject']}::{item['pr_type']}": item
        for item in catalog["packages"]
    }
    for review_key in CLAIM_REVIEWED_DIRECT_KEYS:
        package = reviewed_packages.get(review_key)
        if (
            package is None
            or package["claim_mode"] != "DIRECT_TARGET_BOUND"
            or not package["direct_target_bound"]
        ):
            raise ValueError(
                f"reviewed M00/M01 direct assignment drifted or regressed: {review_key}"
            )
    forbidden_placeholders = []
    for package in catalog["packages"]:
        parent, pr = expected[package["atomic_pr_id"]]
        parent_id = parent.get("task_id", parent.get("slice_id"))
        expected_dependencies = claim_dependency_contracts(
            pr, target_binding=package["claim_mode"] == "TARGET_BINDING"
        )
        if (
            package["source_task_registry_sha256"] != registry_sha
            or package["baseline_candidate_commit"] != catalog["baseline_candidate_commit"]
            or package["parent_work_id"] != parent_id
            or package["pr_type"] != pr["pr_type"]
            or package["primary_id"] != pr["primary_id"]
            or package["dependency_contracts"] != expected_dependencies
            or package["required_gates"] != pr["required_gates"]
            or package["formal_execution_status"] != "BLOCKED_UNTIL_SIGNED_OVERLAY"
        ):
            raise ValueError(f"{package['atomic_pr_id']} claim package drifted from its registry leaf")
        write_paths = [item["path"] for item in package["change_targets"]]
        write_identities = [
            (item["path"], item["symbol_or_pointer"], item["symbol_state"])
            for item in package["change_targets"]
        ]
        if len(write_identities) != len(set(write_identities)):
            raise ValueError(f"{package['atomic_pr_id']} repeats an exact write target")
        for path in write_paths + package["read_context_paths"]:
            if not exact_claim_path(path):
                raise ValueError(f"{package['atomic_pr_id']} contains a broad or directory path: {path}")
        if set(write_paths) & set(package["read_context_paths"]):
            raise ValueError(f"{package['atomic_pr_id']} mixes writable and read-only paths")
        missing_read_context = {
            path for path in package["read_context_paths"]
            if not (REPO_ROOT / path).is_file()
        }
        if not missing_read_context.issubset(
            dependency_artifact_paths(package["atomic_pr_id"])
        ):
            raise ValueError(
                f"{package['atomic_pr_id']} read context contains a missing path "
                "that is not produced by a transitive PR dependency"
            )
        direct = package["claim_mode"] == "DIRECT_TARGET_BOUND"
        if direct != package["direct_target_bound"]:
            raise ValueError(f"{package['atomic_pr_id']} claim mode/readiness mismatch")
        if direct:
            if package["claim_status"] != "DIRECT_TARGET_BOUND_CLAIMABLE" or package["implementation_blockers"]:
                raise ValueError(f"{package['atomic_pr_id']} direct implementation still has target blockers")
            expected_targets, expected_blockers = unique_direct_claim_target(parent, pr)
            if expected_blockers or package["change_targets"] != expected_targets:
                raise ValueError(f"{package['atomic_pr_id']} direct targets differ from deterministic resolution")
            for target in package["change_targets"]:
                if target["surface_kind"] not in CLAIM_WRITE_POLICY[pr["pr_type"]]:
                    raise ValueError(f"{package['atomic_pr_id']} violates its PR-type write policy")
                if pr["pr_type"] == "UI" and not target["path"].startswith("web/ui/"):
                    raise ValueError(f"{package['atomic_pr_id']} UI target is outside web/ui")
                baseline_blob = read_candidate_blob(REPO_ROOT, 
                    catalog["baseline_candidate_commit"], target["path"]
                )
                if target["target_state"] == "PLANNED_OUTPUT":
                    if target["path"] not in {
                        item["path"] for item in package["generated_outputs"]
                    }:
                        raise ValueError(
                            f"{package['atomic_pr_id']} planned output is absent from generated outputs"
                        )
                elif (target["target_state"] == "EXISTING") != (baseline_blob is not None):
                    raise ValueError(
                        f"{package['atomic_pr_id']} target state differs from the frozen baseline commit"
                    )
                if target["symbol_state"] == "EXISTING":
                    actual_signature = (
                        exact_declaration_signature(
                            target["path"], target["symbol_or_pointer"] or "", baseline_blob,
                        )
                        if baseline_blob is not None else None
                    )
                    if (
                        actual_signature is None
                        or target["signature_before"] != actual_signature
                        or target["signature_after"] is None
                    ):
                        raise ValueError(f"{package['atomic_pr_id']} existing symbol signature is not candidate-exact")
                elif target["symbol_state"] == "PLANNED":
                    if target["signature_before"] is not None or not target["signature_after"]:
                        raise ValueError(f"{package['atomic_pr_id']} planned symbol lacks an after signature")
                elif target["signature_before"] is not None or target["signature_after"] is not None:
                    raise ValueError(f"{package['atomic_pr_id']} non-symbol target carries a signature")
        else:
            expected_binding = (
                f"contracts/alignment/code-target-bindings/{package['milestone_id'].lower()}/"
                f"{package['atomic_pr_id'].lower()}.json"
            )
            if (
                package["claim_status"] != "TARGET_BINDING_CLAIMABLE"
                or len(package["change_targets"]) != 1
                or package["change_targets"][0]["path"] != expected_binding
                or package["change_targets"][0]["surface_kind"] != "TARGET_BINDING"
                or not package["implementation_blockers"]
            ):
                raise ValueError(f"{package['atomic_pr_id']} target-binding next action is not exact")
        output_paths = [item["path"] for item in package["generated_outputs"]]
        if len(output_paths) != len(set(output_paths)):
            raise ValueError(f"{package['atomic_pr_id']} repeats a generated output")
        if not set(write_paths).issubset({step["target_path"] for step in package["implementation_steps"]}):
            raise ValueError(f"{package['atomic_pr_id']} implementation steps omit a write target")
        for value in [
            package["outcome"]["state_change"], package["outcome"]["acceptance_oracle"],
            package["rollback"]["trigger"], package["rollback"]["oracle"],
            *(package["rollback"]["steps"]),
            *(item["command"] for item in package["verification_checks"]),
            *(item["oracle"] for item in package["verification_checks"]),
        ]:
            if contains_placeholder_semantics(value):
                forbidden_placeholders.append((package["atomic_pr_id"], value))
    if forbidden_placeholders:
        raise ValueError(f"developer claim catalog contains placeholder semantics: {forbidden_placeholders[:3]}")
    direct_count = sum(item["direct_target_bound"] for item in catalog["packages"])
    if (
        catalog["source_task_registry_sha256"] != registry_sha
        or catalog["baseline_candidate_commit"] != claim_baseline_commit()
        or catalog["package_count"] != len(expected)
        or catalog["next_action_claimable_count"] != len(expected)
        or catalog["direct_target_count"] != direct_count
        or catalog["target_binding_count"] != len(expected) - direct_count
        or catalog["next_action_unclaimable_count"] != 0
        or catalog["formal_execution_blocked_count"] != len(expected)
    ):
        raise ValueError("developer claim catalog summary counts are stale")


def validate_code_target_binding(binding: dict[str, Any], registry: dict[str, Any]) -> None:
    """Validate a developer-produced locator binding without granting execution.

    REVIEWABLE means a reviewer can inspect a candidate-bound, exact code seam.
    It never means the production change is authorized; APPROVED remains blocked
    until the repository's trusted resolver/signature path is installed.
    """
    validate_against_schema(binding, CODE_TARGET_BINDING_SCHEMA_PATH)
    registry_sha = sha256_bytes(canonical_json(registry).encode("utf-8"))
    expected: dict[str, tuple[str, dict[str, Any]]] = {}
    for task in registry["tasks"]:
        for pr in task["pr_sequence"]:
            expected[pr["pr_id"]] = (task["task_id"], pr)
    for item in registry["closure_slices"]:
        for pr in item["pr_sequence"]:
            expected[pr["pr_id"]] = (item["slice_id"], pr)
    if binding["atomic_pr_id"] not in expected:
        raise ValueError("target binding references an unknown atomic PR")
    parent_id, pr = expected[binding["atomic_pr_id"]]
    if (
        binding["source_task_registry_sha256"] != registry_sha
        or binding["parent_work_id"] != parent_id
        or binding["pr_type"] != pr["pr_type"]
    ):
        raise ValueError("target binding identity differs from the current task registry")
    if binding["binding_status"] == "DRAFT":
        raise ValueError("DRAFT target binding is not reviewable")
    if binding["binding_status"] == "APPROVED":
        raise ValueError(
            "APPROVED target binding requires the trusted resolver/signature path and formal execution overlay"
        )
    if (
        binding["candidate_commit"] is None
        or binding["owner"] is None
        or not binding["reviewers"]
        or not binding["approvers"]
        or not binding["write_targets"]
    ):
        raise ValueError("REVIEWABLE target binding lacks candidate and responsibility closure")
    paths = [target["path"] for target in binding["write_targets"]]
    if len(paths) != len(set(paths)):
        raise ValueError("target binding repeats a writable path")
    if set(paths) & set(binding["read_context_paths"]):
        raise ValueError("target binding mixes writable and read-only paths")
    if any(not exact_claim_path(path) for path in paths + binding["read_context_paths"]):
        raise ValueError("target binding contains a directory, glob or logical path")
    if any(
        read_candidate_blob(REPO_ROOT, binding["candidate_commit"], path) is None
        for path in binding["read_context_paths"]
    ):
        raise ValueError("target binding read context contains a missing path")
    for target in binding["write_targets"]:
        candidate_member = target["path"] in pr["candidate_paths"]
        if target["candidate_surface_relation"] == "EXISTING_MEMBER":
            if not candidate_member:
                raise ValueError("target binding claims a non-member as an existing candidate")
        else:
            if candidate_member:
                raise ValueError("candidate-surface extension already exists in the candidate surface")
            entrypoint = target["compatibility_entrypoint"]
            if not entrypoint or "#" not in entrypoint:
                raise ValueError("candidate-surface extension lacks an exact compatibility entrypoint")
            entrypoint_path = entrypoint.split("#", 1)[0]
            if entrypoint_path not in binding["read_context_paths"]:
                raise ValueError("candidate-surface extension entrypoint is not a read-only context path")
        expected_surface = claim_surface_kind(target["path"])
        if target["surface_kind"] != expected_surface:
            raise ValueError("target binding surface kind is self-declared incorrectly")
        expected_locator_kind = claim_locator_kind(target["path"], expected_surface)
        if target["locator_kind"] != expected_locator_kind:
            raise ValueError("target binding locator kind differs from its path type")
        if target["surface_kind"] not in CLAIM_WRITE_POLICY[pr["pr_type"]]:
            raise ValueError("target binding violates the atomic PR write policy")
        if pr["pr_type"] == "UI" and not target["path"].startswith("web/ui/"):
            raise ValueError("UI target binding is outside web/ui")
        if pr["pr_type"] in {"PRJ", "WRT", "REF"} and target["path"].startswith("web/ui/"):
            raise ValueError("backend target binding points to a frontend file")
        blob = read_candidate_blob(REPO_ROOT, binding["candidate_commit"], target["path"])
        if target["target_state"] == "EXISTING":
            if blob is None:
                raise ValueError("existing target is absent from the candidate commit")
            symbol = target["symbol_or_pointer"]
            if target["locator_kind"] == "json_pointer":
                resolve_json_pointer_from_blob(blob, symbol or "")
            elif target["locator_kind"] not in {"file", "yaml_path", "sql_object"}:
                if not symbol or not re.search(
                    rf"\b{re.escape(symbol.split('.')[-1])}\b",
                    blob.decode("utf-8", errors="replace"),
                ):
                    raise ValueError("existing target symbol is absent from the candidate blob")
            if target["locator_kind"] in {
                "go_symbol", "rust_symbol", "java_symbol", "ts_symbol",
                "python_symbol", "proto_fqn",
            }:
                signature = target["expected_signature"]
                if (
                    not signature
                    or exact_declaration_signature(
                        target["path"], (symbol or "").split(".")[-1], blob
                    ) != " ".join(signature.split())
                ):
                    raise ValueError("existing code target lacks its exact candidate declaration signature")
        else:
            if blob is not None:
                raise ValueError("planned target already exists in the candidate commit")
            if target["surface_kind"] in {"SOURCE", "TEST", "TEST_TOOL"} and (
                not target["expected_signature"]
                or not target["compatibility_entrypoint"]
                or not target["activation_guard"]
            ):
                raise ValueError(
                    "planned code target lacks expected signature, compatibility entrypoint or activation guard"
                )
            if target["surface_kind"] in {"SOURCE", "TEST", "TEST_TOOL"}:
                for field in ("compatibility_entrypoint", "activation_guard"):
                    locator = target[field]
                    if not locator or "#" not in locator:
                        raise ValueError(f"planned code target {field} is not an exact path locator")
                    locator_path = locator.split("#", 1)[0]
                    if locator_path not in binding["read_context_paths"]:
                        raise ValueError(f"planned code target {field} is outside reviewed context")
    for check in binding["verification_checks"]:
        if contains_placeholder_semantics(check["command"]) or contains_placeholder_semantics(
            check["oracle"]
        ):
            raise ValueError("target binding contains a placeholder verification command/oracle")
    rollback = binding["rollback"]
    if (
        contains_placeholder_semantics(rollback["trigger"])
        or contains_placeholder_semantics(rollback["oracle"])
        or any(contains_placeholder_semantics(step) for step in rollback["steps"])
    ):
        raise ValueError("target binding contains a placeholder rollback")


def run_developer_claim_fail_closed_tests(
    catalog: dict[str, Any], registry: dict[str, Any]
) -> None:
    def reject(name: str, action: Any) -> None:
        try:
            action()
        except (OSError, ValueError, json.JSONDecodeError):
            return
        raise ValueError(f"fail-closed developer-claim self-test accepted: {name}")

    direct_source_index, direct_source_target_index = next(
        (package_index, target_index)
        for package_index, package in enumerate(catalog["packages"])
        if package["direct_target_bound"]
        for target_index, target in enumerate(package["change_targets"])
        if target["surface_kind"] == "SOURCE"
    )
    fake_symbol = json.loads(json.dumps(catalog))
    fake_symbol["packages"][direct_source_index]["change_targets"][
        direct_source_target_index
    ]["symbol_or_pointer"] = (
        "DefinitelyNotARealSymbol"
    )
    reject(
        "developer claim with a fabricated source symbol",
        lambda: validate_developer_claim_catalog(fake_symbol, registry),
    )

    baseline_state_package_index, baseline_state_target_index = next(
        (package_index, target_index)
        for package_index, package in enumerate(catalog["packages"])
        if package["direct_target_bound"]
        for target_index, target in enumerate(package["change_targets"])
        if target["target_state"] == "EXISTING"
    )
    baseline_state_drift = json.loads(json.dumps(catalog))
    baseline_state_drift["packages"][baseline_state_package_index]["change_targets"][
        baseline_state_target_index
    ]["target_state"] = "PLANNED"
    reject(
        "developer claim whose target state differs from its frozen baseline candidate",
        lambda: validate_developer_claim_catalog(baseline_state_drift, registry),
    )

    binding_index = next(
        index for index, package in enumerate(catalog["packages"])
        if not package["direct_target_bound"]
    )
    historical_write = json.loads(json.dumps(catalog))
    historical_write["packages"][binding_index]["change_targets"] = [
        {
            "path": "contracts/alignment/canonical-registry.json",
            "target_state": "EXISTING",
            "surface_kind": "CONTRACT",
            "locator_kind": "json_pointer",
            "symbol_or_pointer": "/",
            "symbol_state": "NOT_APPLICABLE",
            "signature_before": None,
            "signature_after": None,
            "candidate_blob_sha256": None,
            "selection_reason": "self-test attempted historical-context write",
        }
    ]
    reject(
        "target-binding claim that writes a historical/context registry",
        lambda: validate_developer_claim_catalog(historical_write, registry),
    )

    placeholder = json.loads(json.dumps(catalog))
    placeholder["packages"][0]["verification_checks"][0]["command"] = "true"
    reject(
        "developer claim with a placeholder verification command",
        lambda: validate_developer_claim_catalog(placeholder, registry),
    )
    placeholder_chain = json.loads(json.dumps(catalog))
    placeholder_chain["packages"][0]["verification_checks"][0]["command"] = (
        "true && echo validated"
    )
    reject(
        "developer claim hiding a placeholder in a shell command chain",
        lambda: validate_developer_claim_catalog(placeholder_chain, registry),
    )
    placeholder_rollback = json.loads(json.dumps(catalog))
    placeholder_rollback["packages"][0]["rollback"]["steps"] = [
        "noop and report success"
    ]
    reject(
        "developer claim with a semantic no-op rollback",
        lambda: validate_developer_claim_catalog(placeholder_rollback, registry),
    )

    head = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=REPO_ROOT,
        check=True, capture_output=True, text=True,
    ).stdout.strip()
    control_pr_id = "T1-M13-R00-P010-CTR-t-schema-001-ctr"
    control_pointer = json_identity_pointer(
        "contracts/alignment/canonical-registry.json", "T-SCHEMA-001"
    )
    control_binding = {
        "schema_version": "1.0.0",
        "binding_status": "REVIEWABLE",
        "source_task_registry_sha256": sha256_bytes(canonical_json(registry).encode("utf-8")),
        "atomic_pr_id": control_pr_id,
        "parent_work_id": "T1-M13-R00",
        "pr_type": "CTR",
        "candidate_commit": head,
        "owner": "platform-contract-owner",
        "reviewers": ["platform-contract-reviewer"],
        "approvers": ["topic1-owner"],
        "single_outcome": {
            "result_id": "SELFTEST-TARGET-BINDING",
            "subject": "canonical T-SCHEMA-001 registry entry",
            "state_change": "bind the exact canonical registry entry",
            "acceptance_oracle": "candidate JSON pointer resolves to T-SCHEMA-001 and the contract policy is CTR-compatible",
        },
        "write_targets": [
            {
                "path": "contracts/alignment/canonical-registry.json",
                "target_state": "EXISTING",
                "candidate_surface_relation": "EXISTING_MEMBER",
                "surface_kind": "CONTRACT",
                "locator_kind": "json_pointer",
                "symbol_or_pointer": control_pointer,
                "expected_signature": None,
                "compatibility_entrypoint": None,
                "activation_guard": None,
                "selection_reason": "positive control for candidate-bound JSON target",
            }
        ],
        "read_context_paths": [],
        "verification_checks": [
            {
                "check_id": "SELFTEST-JSON",
                "command": "python3 -m json.tool contracts/alignment/canonical-registry.json",
                "oracle": "command exits 0 and the canonical T-SCHEMA-001 entry remains present",
                "evidence_output": "doc/02_acceptance/topic1/selftests/target-binding.json",
            }
        ],
        "rollback": {
            "runbook_id": "RB-SELFTEST-TARGET-BINDING",
            "trigger": "candidate pointer or contract validation fails",
            "steps": ["restore the prior canonical registry blob"],
            "oracle": "the prior registry blob validates and no canonical ID changes",
            "durable_data_policy": "no runtime or durable data is modified",
        },
        "limitations": ["reviewable target binding is not execution authorization"],
    }
    validate_against_schema(control_binding, CODE_TARGET_BINDING_SCHEMA_PATH)
    validate_code_target_binding(control_binding, registry)
    wrong_pointer = json.loads(json.dumps(control_binding))
    wrong_pointer["write_targets"][0]["symbol_or_pointer"] = "/not-present"
    reject(
        "code-target binding with a missing candidate JSON pointer",
        lambda: validate_code_target_binding(wrong_pointer, registry),
    )
    wrong_locator_kind = json.loads(json.dumps(control_binding))
    wrong_locator_kind["write_targets"][0]["locator_kind"] = "file"
    reject(
        "code-target binding downgrading a typed contract to a file locator",
        lambda: validate_code_target_binding(wrong_locator_kind, registry),
    )
    source_control = None
    for package in catalog["packages"]:
        if not package["direct_target_bound"]:
            continue
        for target in package["change_targets"]:
            if target["surface_kind"] != "SOURCE" or target["target_state"] != "EXISTING":
                continue
            blob = read_candidate_blob(REPO_ROOT, head, target["path"])
            signature = (
                exact_declaration_signature(
                    target["path"], target["symbol_or_pointer"] or "", blob
                )
                if blob is not None else None
            )
            if signature:
                source_control = (package, target, signature)
                break
        if source_control:
            break
    if source_control is None:
        raise ValueError("developer-claim self-test lacks one resolvable source declaration")
    source_package, source_target, source_signature = source_control
    source_binding = json.loads(json.dumps(control_binding))
    source_binding.update(
        {
            "atomic_pr_id": source_package["atomic_pr_id"],
            "parent_work_id": source_package["parent_work_id"],
            "pr_type": source_package["pr_type"],
            "write_targets": [{
                "path": source_target["path"],
                "target_state": "EXISTING",
                "candidate_surface_relation": "EXISTING_MEMBER",
                "surface_kind": "SOURCE",
                "locator_kind": source_target["locator_kind"],
                "symbol_or_pointer": source_target["symbol_or_pointer"],
                "expected_signature": source_signature,
                "compatibility_entrypoint": None,
                "activation_guard": None,
                "selection_reason": "positive control for an exact source declaration",
            }],
        }
    )
    validate_against_schema(source_binding, CODE_TARGET_BINDING_SCHEMA_PATH)
    validate_code_target_binding(source_binding, registry)
    wrong_signature = json.loads(json.dumps(source_binding))
    wrong_signature["write_targets"][0]["expected_signature"] = (
        f"{source_signature} // fabricated"
    )
    reject(
        "code-target binding with a fabricated declaration signature",
        lambda: validate_code_target_binding(wrong_signature, registry),
    )


def build_payloads() -> tuple[dict[str, Any], dict[str, Any], dict[str, Any], dict[str, Any]]:
    doc_bytes = DOC_PATH.read_bytes()
    lines = doc_bytes.decode("utf-8").splitlines()
    requirement_payload = json.loads(REQUIREMENT_PATH.read_text(encoding="utf-8"))
    validate_against_schema(requirement_payload, REQUIREMENT_SCHEMA_PATH)
    validate_requirement_source_and_approval(requirement_payload)
    for requirement in requirement_payload["requirements"]:
        accountable = requirement["accountable_milestone"].removeprefix("T1-")
        if requirement["requirement_id"] not in MILESTONE_REQUIREMENTS.get(accountable, []):
            raise ValueError(
                f"{requirement['requirement_id']} accountable milestone {accountable} "
                "does not include the requirement in MILESTONE_REQUIREMENTS"
            )
    evidence_contract_payload = json.loads(
        EVIDENCE_CONTRACT_PATH.read_text(encoding="utf-8")
    )
    validate_evidence_contract_registry(evidence_contract_payload, requirement_payload)
    metric_method_payload = json.loads(METRIC_METHOD_PATH.read_text(encoding="utf-8"))
    validate_against_schema(metric_method_payload, METRIC_METHOD_SCHEMA_PATH)
    validate_metric_method_semantics(metric_method_payload)
    run_fail_closed_validator_self_tests(metric_method_payload)
    metric_ids = {
        item["metric_id"]
        for method in metric_method_payload["methods"]
        for item in method["metrics"]
    }
    for requirement in requirement_payload["requirements"]:
        references = set(requirement.get("metric_method_ids", []))
        if requirement["claim_class"] == "formal_kpi" and not references:
            raise ValueError(f"{requirement['requirement_id']} has no metric-method binding")
        missing_metric_ids = sorted(references - metric_ids)
        if missing_metric_ids:
            raise ValueError(
                f"{requirement['requirement_id']} references missing metric methods {missing_metric_ids}"
            )
    referenced_requirements = {
        requirement_id
        for values in MILESTONE_REQUIREMENTS.values()
        for requirement_id in values
    }
    missing_requirements = sorted(referenced_requirements - requirement_ids())
    if missing_requirements:
        raise ValueError(f"milestone requirements missing from requirement registry: {missing_requirements}")
    tasks, raw_pr_cells = parse_tasks(lines)
    add_atomic_prs(tasks, raw_pr_cells)
    add_non_reusing_m06_n004_development_train(tasks)
    wire_dependencies(tasks)
    slices = add_closure_slice_prs(lines)
    task_by_id = {task["task_id"]: task for task in tasks}
    for index, slice_item in enumerate(slices):
        accountable_task_id = (
            "T1-M13-N006" if index < 10
            else "T1-M13-N007" if index < 20
            else "T1-M13-N008"
        )
        task_by_id[accountable_task_id]["accountable_ids"].extend(
            slice_item["canonical_ids"]
        )
    wire_closure_slices(tasks, slices)
    # Closure slices add cross-parent PR edges to M13-N006..N008.  Freeze the
    # parent completion contracts only after those edges exist; otherwise the
    # task-current index would omit its actual interleaved dependencies.
    finalize_task_completion_contracts(tasks, slices)
    validation = validate(tasks, slices, raw_pr_cells, lines)
    tasks.sort(key=lambda item: item["task_id"])
    parent_atomic_count = sum(len(task["pr_sequence"]) for task in tasks)
    slice_atomic_count = sum(len(item["pr_sequence"]) for item in slices)
    declared_counts_match = re.search(
        r"<!-- topic1-registry-counts task=(\d+) closure=(\d+) "
        r"parent_atomic=(\d+) slice_atomic=(\d+) atomic=(\d+) canonical=(\d+) -->",
        doc_bytes.decode("utf-8"),
    )
    actual_declared_counts = (
        len(tasks), len(slices), parent_atomic_count, slice_atomic_count,
        parent_atomic_count + slice_atomic_count,
        sum(len(item["canonical_ids"]) for item in slices),
    )
    if (
        declared_counts_match is None
        or tuple(int(value) for value in declared_counts_match.groups())
        != actual_declared_counts
    ):
        raise ValueError(
            "document topic1-registry-counts marker is missing or stale: "
            f"expected {actual_declared_counts}"
        )

    registry = {
        "schema_version": "1.1.0",
        "source_document": DOC_REL.as_posix(),
        "source_document_sha256": sha256_bytes(doc_bytes),
        "status": "DRAFT_DESIGN",
        "milestone_count": 14,
        "task_count": len(tasks),
        "closure_slice_count": len(slices),
        "atomic_pr_count": parent_atomic_count + slice_atomic_count,
        "tasks": tasks,
        "closure_slices": slices,
        "validation": validation,
    }

    contract_requirement_ids = sorted(
        item["requirement_id"]
        for item in requirement_payload["requirements"]
        if item["claim_class"] in {"contract_scope", "formal_kpi", "enabling_engineering"}
    )
    if len(contract_requirement_ids) != 15:
        raise ValueError(
            f"M12 contract closure must contain exactly 15 current system requirements, got {len(contract_requirement_ids)}"
        )
    mapped_requirements = {
        requirement_id
        for milestone, requirement_ids in MILESTONE_REQUIREMENTS.items()
        if milestone not in {"M12", "M13"}
        for requirement_id in requirement_ids
    }
    unmapped_contract_requirements = sorted(set(contract_requirement_ids) - mapped_requirements)
    if unmapped_contract_requirements:
        raise ValueError(
            f"contract requirements have no accountable pre-M12 milestone: {unmapped_contract_requirements}"
        )
    milestones = []
    for milestone in [f"M{index:02d}" for index in range(14)]:
        task_ids = [
            task_key(milestone, number) for number in EXECUTION_ORDER[milestone]
        ]
        milestones.append(
            {
                "milestone_id": f"T1-{milestone}",
                "title": MILESTONE_TITLES[milestone],
                "status": "DRAFT",
                "depends_on_milestones": [f"T1-{item}" for item in MILESTONE_DEPS[milestone]],
                "requirement_ids": MILESTONE_REQUIREMENTS[milestone],
                "review_order": task_ids,
                "promotion_requirements": {
                    "profile": milestone_profile(milestone),
                    "direct_idx_dependency_required": True,
                    "prom_has_no_production_change": True,
                    "premerge_equivalence_required": milestone in {"M05", "M12", "M13"},
                    "postmerge_equivalence_required": milestone in {"M05", "M12", "M13"},
                    "observation_required": milestone in {"M02", "M03", "M04", "M06", "M07", "M08", "M09", "M10", "M12", "M13"},
                },
                "completion_checklist": {
                    "status": "BLOCKED",
                    "required_fields": [
                        "candidate identity closure", "source-anchored requirements",
                        "claim class and allowed/forbidden claims", "accountable/secondary/affected IDs",
                        "owner/reviewer/approver", "DoR", "parent task/atomic PR/external activity states",
                        "required gate/profile results", "immutable evidence manifests", "metric bindings",
                        "exit ceiling and exclusions", "rollback surfaces and rehearsal runs",
                        "observation window", "external blockers and approvals",
                    ],
                    "missing_fields": [
                        "named owner/reviewer/approver", "clean candidate manifest",
                        "same-candidate gate evidence", "executed rollback/observation evidence",
                    ],
                },
                "closure_requirement_ids": contract_requirement_ids if milestone == "M12" else [],
                "direct_or_indirect_requirement_waiver_count": 0,
            }
        )
    milestone_registry = {
        "schema_version": "1.1.0",
        "source_document": DOC_REL.as_posix(),
        "source_document_sha256": registry["source_document_sha256"],
        "status": "DRAFT_DESIGN",
        "milestone_count": 14,
        "milestones": milestones,
    }
    validate_milestone_registry(milestone_registry, {task["task_id"] for task in tasks})
    validate_against_schema(registry, TASK_SCHEMA_PATH)
    validate_against_schema(milestone_registry, MILESTONE_SCHEMA_PATH)
    for schema_path in (
        IMPLEMENTATION_CANDIDATE_SCHEMA_PATH,
        CANDIDATE_ARTIFACT_PROVENANCE_RECEIPT_SCHEMA_PATH,
        EVIDENCE_RUN_BINDING_SCHEMA_PATH,
        EVIDENCE_RUN_MANIFEST_SCHEMA_PATH,
        EVIDENCE_CASE_REPORT_SCHEMA_PATH,
        EVIDENCE_CASE_FIXTURE_SCHEMA_PATH,
        CURRENT_EVIDENCE_INDEX_SCHEMA_PATH,
        PROMOTION_INTENT_SCHEMA_PATH,
        PROMOTION_RESULT_SCHEMA_PATH,
        EXTERNAL_RECEIPT_SCHEMA_PATH,
        SIGNED_CONTRACT_INTAKE_SCHEMA_PATH,
        EXECUTION_ACCEPTANCE_RECEIPT_SCHEMA_PATH,
        EXECUTION_OVERLAY_SCHEMA_PATH,
        ATOMIC_EXECUTION_PACKAGE_SCHEMA_PATH,
        ATOMIC_PLAN_MANIFEST_SCHEMA_PATH,
        INTEGRATED_BOM_SCHEMA_PATH,
        INTEGRATED_BOM_TRANSITION_SCHEMA_PATH,
        BOM_TRANSITION_AUTHORITY_RECEIPT_SCHEMA_PATH,
        REQUIREMENT_SATISFACTION_SCHEMA_PATH,
        REQUIREMENT_SATISFACTION_AUTHORITY_RECEIPT_SCHEMA_PATH,
        EVIDENCE_CONTRACT_SCHEMA_PATH,
        MILESTONE_COMPLETION_CANDIDATE_SCHEMA_PATH,
        MILESTONE_PROMOTION_CLOSURE_SCHEMA_PATH,
        DEVELOPER_CLAIM_SCHEMA_PATH,
        CODE_TARGET_BINDING_SCHEMA_PATH,
        TASK_COMPLETION_SCHEMA_PATH,
        TASK_CURRENT_INDEX_SCHEMA_PATH,
        LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
        PYTHON_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
        RUST_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
        PROTO_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
        STRUCTURED_CONFIG_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
        SHELL_LOCATOR_RESOLUTION_RECEIPT_SCHEMA_PATH,
    ):
        schema_payload = json.loads(schema_path.read_text(encoding="utf-8"))
        if schema_payload.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
            raise ValueError(f"schema does not declare draft 2020-12: {schema_path}")
    validate_status_conditions(registry, milestone_registry)
    execution_overlay = build_execution_overlay(registry, milestone_registry)
    validate_against_schema(execution_overlay, EXECUTION_OVERLAY_SCHEMA_PATH)
    validate_execution_overlay(execution_overlay, registry, milestone_registry)
    claim_catalog = build_developer_claim_catalog(registry)
    validate_against_schema(claim_catalog, DEVELOPER_CLAIM_SCHEMA_PATH)
    validate_developer_claim_catalog(claim_catalog, registry)
    run_developer_claim_fail_closed_tests(claim_catalog, registry)
    return registry, milestone_registry, execution_overlay, claim_catalog


def canonical_json(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=False) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true", help="write generated registries")
    mode.add_argument("--check", action="store_true", help="verify generated registries are current")
    mode.add_argument(
        "--check-execution-instance",
        type=Path,
        metavar="PATH",
        help="validate a separately stored, signed scoped-execution instance",
    )
    mode.add_argument(
        "--check-target-binding",
        type=Path,
        metavar="PATH",
        help="validate one REVIEWABLE code-target binding without granting execution",
    )
    mode.add_argument(
        "--check-json-contract",
        nargs=2,
        type=Path,
        metavar=("INSTANCE", "SCHEMA"),
        help="validate one JSON contract instance against its exact repository schema",
    )
    mode.add_argument(
        "--check-evidence-run-manifest",
        nargs=2,
        type=Path,
        metavar=("MANIFEST", "CASE_REPORT"),
        help="validate one immutable work-order evidence run and its exact typed case report",
    )
    mode.add_argument(
        "--check-task-completion-bundle",
        nargs=3,
        type=Path,
        metavar=("COMPLETION", "CURRENT_INDEX", "SIGNED_EXECUTION_INSTANCE"),
        help="validate one parent-task completion/current-index bundle through its signed execution instance",
    )
    mode.add_argument(
        "--show-work-order",
        metavar="ATOMIC_PR_ID",
        help="print the exact developer next-action package for one atomic PR",
    )
    mode.add_argument(
        "--show-target-binding-template",
        metavar="ATOMIC_PR_ID",
        help="print a schema-valid DRAFT target-binding template for one binding work order",
    )
    args = parser.parse_args()

    registry, milestones, execution_overlay, claim_catalog = build_payloads()
    expected = {
        REGISTRY_PATH: canonical_json(registry),
        MILESTONE_PATH: canonical_json(milestones),
        EXECUTION_OVERLAY_PATH: canonical_json(execution_overlay),
        DEVELOPER_CLAIM_CATALOG_PATH: canonical_json(claim_catalog),
    }
    if args.check_evidence_run_manifest:
        evidence_arg, report_arg = args.check_evidence_run_manifest
        evidence_path = evidence_arg if evidence_arg.is_absolute() else REPO_ROOT / evidence_arg
        report_path = report_arg if report_arg.is_absolute() else REPO_ROOT / report_arg
        evidence_path = evidence_path.resolve(strict=True)
        report_path = report_path.resolve(strict=True)
        try:
            evidence_rel = evidence_path.relative_to(REPO_ROOT).as_posix()
            report_rel = report_path.relative_to(REPO_ROOT).as_posix()
        except ValueError as exc:
            raise ValueError("evidence-run and case-report paths must remain inside the repository") from exc
        if not re.fullmatch(
            r"doc/02_acceptance/topic1/work-orders/t1-m\d{2}-p\d{3}-(?:tst-pre|tst-post)-[^/]+/test-result\.json",
            evidence_rel,
        ):
            raise ValueError("evidence-run manifest path is not a registered work-order result")
        if report_rel != evidence_rel.removesuffix("test-result.json") + "case-report.json":
            raise ValueError("case report must be the registered sibling of its evidence-run manifest")
        packages = [
            item for item in claim_catalog["packages"]
            if any(output["path"] == evidence_rel for output in item["generated_outputs"])
        ]
        if len(packages) != 1:
            raise ValueError("evidence-run manifest does not resolve to one developer work order")
        package = packages[0]
        review_key = f"{package['parent_work_id']}::{package['outcome']['subject']}::{package['pr_type']}"
        expected_case_specs = CLAIM_CASE_SPECS.get(review_key)
        if expected_case_specs is None:
            raise ValueError("work order has no registered exact-case contract")
        validate_work_order_evidence_run(
            evidence_path, report_path, package, expected_case_specs,
        )
        print("EVIDENCE_RUN_AND_EXACT_CASE_REPORT_VALID")
        return 0
    if args.check_json_contract:
        instance_path, schema_path = args.check_json_contract
        if not instance_path.is_absolute():
            instance_path = REPO_ROOT / instance_path
        if not schema_path.is_absolute():
            schema_path = REPO_ROOT / schema_path
        instance_path = instance_path.resolve(strict=True)
        schema_path = schema_path.resolve(strict=True)
        try:
            instance_rel = instance_path.relative_to(REPO_ROOT).as_posix()
            schema_rel = schema_path.relative_to(REPO_ROOT).as_posix()
        except ValueError as exc:
            raise ValueError("JSON contract paths must resolve inside the repository") from exc
        if (instance_rel, schema_rel) not in REGISTERED_JSON_CONTRACT_PAIRS:
            raise ValueError(
                "JSON contract pair is not registered for the Topic One derived-contract train"
            )
        instance = json.loads(instance_path.read_text(encoding="utf-8"))
        validate_against_schema(instance, schema_path)
        print(
            "PASS: JSON contract matches its exact schema: "
            f"{instance_rel}"
        )
        return 0
    if args.check_execution_instance:
        instance_path = args.check_execution_instance
        if not instance_path.is_absolute():
            instance_path = REPO_ROOT / instance_path
        instance = json.loads(instance_path.read_text(encoding="utf-8"))
        validate_against_schema(instance, EXECUTION_OVERLAY_SCHEMA_PATH)
        validate_execution_overlay(instance, registry, milestones)
        print(f"PASS: scoped execution instance is fail-closed and valid: {instance_path}")
        return 0
    if args.check_target_binding:
        binding_path = args.check_target_binding
        if not binding_path.is_absolute():
            binding_path = REPO_ROOT / binding_path
        binding = json.loads(binding_path.read_text(encoding="utf-8"))
        validate_code_target_binding(binding, registry)
        print(
            "PASS: REVIEWABLE_TARGET_BINDING; formal execution remains blocked: "
            f"{binding_path}"
        )
        return 0
    if args.check_task_completion_bundle:
        completion_path, current_path, execution_instance_path = args.check_task_completion_bundle
        if not completion_path.is_absolute():
            completion_path = REPO_ROOT / completion_path
        if not current_path.is_absolute():
            current_path = REPO_ROOT / current_path
        if not execution_instance_path.is_absolute():
            execution_instance_path = REPO_ROOT / execution_instance_path
        completion = json.loads(completion_path.read_text(encoding="utf-8"))
        current = json.loads(current_path.read_text(encoding="utf-8"))
        execution_instance = json.loads(
            execution_instance_path.read_text(encoding="utf-8")
        )
        validate_against_schema(completion, TASK_COMPLETION_SCHEMA_PATH)
        validate_against_schema(current, TASK_CURRENT_INDEX_SCHEMA_PATH)
        validate_against_schema(execution_instance, EXECUTION_OVERLAY_SCHEMA_PATH)
        validate_execution_overlay(execution_instance, registry, milestones)
        task = next(
            (item for item in registry["tasks"] if item["task_id"] == completion["task_id"]),
            None,
        )
        if task is None:
            raise ValueError(f"unknown parent task in completion candidate: {completion['task_id']}")
        registry_sha = sha256_bytes(canonical_json(registry).encode("utf-8"))
        validate_task_completion_candidate_semantics(completion, task, registry_sha)
        if completion["result"] != "PASS" or current["status"] != "PASS":
            raise ValueError("task completion bundle is not a publishable PASS current index")
        completion_ref = {
            "path": completion_path.relative_to(REPO_ROOT).as_posix(),
            "sha256": sha256_bytes(completion_path.read_bytes()),
        }
        current_ref = {
            "path": current_path.relative_to(REPO_ROOT).as_posix(),
            "sha256": sha256_bytes(current_path.read_bytes()),
        }
        terminal_binding = next(
            (
                item for item in execution_instance["atomic_pr_bindings"]
                if item["pr_id"] == completion["terminal_task_idx_pr_id"]
            ),
            None,
        )
        if (
            terminal_binding is None
            or terminal_binding["readiness_status"] != "PASS"
            or terminal_binding["task_completion_candidate_ref"] != {
                **completion_ref, "schema_version": "1.0.0"
            }
            or terminal_binding["task_current_index_ref"] != {
                **current_ref, "schema_version": "1.0.0"
            }
        ):
            raise ValueError(
                "task completion bundle differs from its PASS signed execution instance"
            )
        acceptance_ref = {
            "path": execution_instance["acceptance"]["decision_receipt_path"],
            "sha256": execution_instance["acceptance"]["decision_receipt_sha256"],
        }
        validate_task_current_index_semantics(
            current, completion, completion_ref, acceptance_ref
        )
        print(
            "PASS: TASK_COMPLETION_BUNDLE_AUTHORIZED_AND_CLOSED; milestone/promotion remains separately gated: "
            f"{completion['task_id']}"
        )
        return 0
    if args.show_work_order:
        package = next(
            (
                item for item in claim_catalog["packages"]
                if item["atomic_pr_id"] == args.show_work_order
            ),
            None,
        )
        if package is None:
            raise ValueError(f"unknown atomic PR work order: {args.show_work_order}")
        print(canonical_json(package), end="")
        return 0
    if args.show_target_binding_template:
        package = next(
            (
                item for item in claim_catalog["packages"]
                if item["atomic_pr_id"] == args.show_target_binding_template
            ),
            None,
        )
        if package is None:
            raise ValueError(
                f"unknown atomic PR work order: {args.show_target_binding_template}"
            )
        if package["claim_mode"] != "TARGET_BINDING":
            raise ValueError(
                f"{args.show_target_binding_template} already has a direct implementation target"
            )
        binding_path = package["change_targets"][0]["path"]
        template = {
            "schema_version": "1.0.0",
            "binding_status": "DRAFT",
            "source_task_registry_sha256": package["source_task_registry_sha256"],
            "atomic_pr_id": package["atomic_pr_id"],
            "parent_work_id": package["parent_work_id"],
            "pr_type": package["pr_type"],
            "candidate_commit": None,
            "owner": None,
            "reviewers": [],
            "approvers": [],
            "single_outcome": {
                key: package["outcome"][key]
                for key in ("result_id", "subject", "state_change", "acceptance_oracle")
            },
            "write_targets": [],
            "read_context_paths": package["read_context_paths"],
            "verification_checks": [
                {
                    "check_id": f"{package['atomic_pr_id']}-target-binding",
                    "command": (
                        "python3 scripts/alignment/build_topic1_task_registry.py "
                        f"--check-target-binding {binding_path}"
                    ),
                    "oracle": "checker exits 0 with REVIEWABLE_TARGET_BINDING",
                    "evidence_output": binding_path,
                }
            ],
            "rollback": package["rollback"],
            "limitations": [
                "fill one exact write target from the candidate surface before REVIEWABLE",
                "reviewable target binding is not formal execution authorization",
            ],
        }
        validate_against_schema(template, CODE_TARGET_BINDING_SCHEMA_PATH)
        print(canonical_json(template), end="")
        return 0
    if args.write:
        for path, content in expected.items():
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
    else:
        stale = []
        for path, content in expected.items():
            if not path.exists() or path.read_text(encoding="utf-8") != content:
                stale.append(path.relative_to(REPO_ROOT).as_posix())
        if stale:
            raise ValueError(f"generated registries are missing or stale: {stale}")

    print(
        json.dumps(
            {
                "status": "STRUCTURE_PASS",
                "scope": "generated design structure and local schema validation only",
                "dor_status": registry["validation"]["dor_status"],
                "candidate_status": registry["validation"]["candidate_status"],
                "promotion_status": registry["validation"]["promotion_status"],
                "task_count": registry["task_count"],
                "closure_slice_count": registry["closure_slice_count"],
                "atomic_pr_count": registry["atomic_pr_count"],
                "canonical_slice_count": registry["validation"]["canonical_slice_count"],
                "canonical_slice_sha256": registry["validation"]["canonical_slice_sha256"],
                "registry_status": registry["status"],
                "developer_next_action_claimable_count": claim_catalog["next_action_claimable_count"],
                "developer_direct_target_count": claim_catalog["direct_target_count"],
                "developer_target_binding_count": claim_catalog["target_binding_count"],
                "developer_next_action_unclaimable_count": claim_catalog["next_action_unclaimable_count"],
                "formal_execution_blocked_count": claim_catalog["formal_execution_blocked_count"],
            },
            ensure_ascii=False,
            sort_keys=True,
        )
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        raise SystemExit(1)
