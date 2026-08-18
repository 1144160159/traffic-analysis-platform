#!/usr/bin/env python3
"""Run M09-N017 feedback authority tests against existing K8s PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import uuid
from pathlib import Path
from typing import Any

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT = (
    ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n017"
    / "k8s-model-feedback-latest.json"
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
INTEGRATION_TEST = "^TestModelFeedbackRevisionK8sPostgresIntegration$"
CLEANUP_TEST = "^TestModelFeedbackRevisionK8sCleanupOracle$"
UNIT_TEST = "^Test(ModelFeedback(First|Next|Rejects|Idempotent|Identities)|VerifyModelFeedbackProducerReadiness)"
BUNDLE_CHECK = " && ".join((
    "test -f /usr/share/nginx/html/index.html",
    "test -f /usr/share/nginx/html/.vite/manifest.json",
    "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
    "grep -R -q 'expected_label_revision' /usr/share/nginx/html/assets",
    "grep -R -q 'adjudication_state' /usr/share/nginx/html/assets",
    "grep -R -q '当前仲裁版本' /usr/share/nginx/html/assets",
    "echo PASS",
))


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-model-feedback-canary",
        "traffic.analysis/canary-run": run_id,
    }


def validate(images: list[str], run_id: str, node: str) -> str:
    for image in images:
        if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
            raise base.CanaryError("candidate images must be explicit non-latest references")
    if not NODE_RE.fullmatch(node):
        raise base.CanaryError("invalid Kubernetes node name")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise base.CanaryError("--run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def postgres_env(suffix: str) -> list[dict[str, Any]]:
    return [
        {"name": "MODEL_FEEDBACK_K8S_INTEGRATION", "value": "run-scoped-only"},
        {"name": "MODEL_FEEDBACK_K8S_SUFFIX", "value": suffix},
        {"name": "MODEL_FEEDBACK_K8S_PG_HOST", "value": "postgres-primary.databases.svc"},
        {"name": "MODEL_FEEDBACK_K8S_PG_PASSWORD", "valueFrom": {"secretKeyRef": {
            "name": "traffic-credentials", "key": "PG_PASSWORD",
        }}},
    ]


def test_job(name: str, run_id: str, suffix: str, node: str, image: str, suite: str) -> dict[str, Any]:
    container: dict[str, Any] = {
        "name": suite,
        "image": image,
        "imagePullPolicy": "Never",
        "securityContext": {
            "allowPrivilegeEscalation": False,
            "readOnlyRootFilesystem": True,
            "capabilities": {"drop": ["ALL"]},
        },
        "resources": {
            "requests": {"cpu": "100m", "memory": "128Mi"},
            "limits": {"cpu": "1", "memory": "512Mi"},
        },
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite in ("integration", "cleanup"):
        selected = INTEGRATION_TEST if suite == "integration" else CLEANUP_TEST
        container["args"] = ["-test.v", f"-test.run={selected}", "-test.count=1"]
        container["env"] = postgres_env(suffix)
    elif suite == "unit":
        container["args"] = ["-test.v", f"-test.run={UNIT_TEST}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": f"{name}-{suite}",
            "namespace": base.APP_NAMESPACE,
            "labels": labels(run_id),
            "annotations": {
                "traffic.analysis/shared-postgres-touched": str(suite == "integration").lower(),
                "traffic.analysis/run-scoped-cleanup-oracle": str(suite == "cleanup").lower(),
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "template": {
                "metadata": {"labels": labels(run_id)},
                "spec": {
                    "nodeName": node,
                    "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True,
                        "runAsUser": 1000,
                        "runAsGroup": 1000,
                        "fsGroup": 1000,
                        "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [container],
                    "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--test-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.test_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n017-feedback-{suffix}"
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    integration_error: Exception | None = None
    try:
        base.apply([test_job(name, args.run_id, suffix, args.node, args.test_image, "integration")])
        logs, receipt = base.wait_job(f"{name}-integration", args.timeout)
        logs_by_suite["integration"] = logs
        receipts.append(receipt)
    except Exception as error:
        integration_error = error
    finally:
        try:
            base.apply([test_job(name, args.run_id, suffix, args.node, args.test_image, "cleanup")])
            logs, receipt = base.wait_job(f"{name}-cleanup", args.timeout)
            logs_by_suite["cleanup"] = logs
            receipts.append(receipt)
        finally:
            if not args.keep:
                base.cleanup(args.run_id)
    if integration_error is not None:
        raise integration_error

    for suite, image in (("unit", args.test_image), ("web", args.web_image)):
        try:
            base.apply([test_job(name, args.run_id, suffix, args.node, image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        finally:
            if not args.keep:
                base.cleanup(args.run_id)

    source_paths = [
        ROOT / "go/control-plane/internal/alert/api/handler_model_feedback_revision.go",
        ROOT / "go/control-plane/internal/alert/api/model_feedback_readiness.go",
        ROOT / "go/control-plane/internal/alert/api/handler_feedback_outbox.go",
        ROOT / "go/control-plane/internal/alert/api/handler_feedback_transaction.go",
        ROOT / "go/control-plane/internal/alert/api/handler_feedback.go",
        ROOT / "go/control-plane/internal/alert/api/handler.go",
        ROOT / "go/control-plane/cmd/alert-service/main.go",
        ROOT / "web/ui/src/services/alertDetailApi.ts",
        ROOT / "web/ui/src/pages/AlertDetailPage.tsx",
        ROOT / "contracts/events/model-feedback-event.v1.schema.json",
        ROOT / "contracts/events/kafka-topic-catalog.v1.json",
        ROOT / "contracts/events/kafka-acl-catalog.v1.json",
        ROOT / "contracts/openapi/alignment-v1.openapi.json",
        ROOT / "deployments/kubernetes/applications/go-services.yaml",
    ]
    consumer_schema_present = "consumer_schema_present=true" in logs_by_suite["integration"]
    envelope = {
        "artifact_kind": "M09_MODEL_FEEDBACK_TEST_RESULT",
        "task_id": "T1-M09-N017",
        "run_id": args.run_id,
        "status": "PASS",
        "profile_id": "M09-N017-K8S-POSTGRES-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_POSTGRES_AND_IMMUTABLE_UI_BUNDLE",
        "inputs": {
            "candidate_images": images,
            "source_sha256": {
                str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
                for path in source_paths
            },
            "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode()).hexdigest(),
        },
        "test_output_sha256": {
            suite: hashlib.sha256(log.encode()).hexdigest()
            for suite, log in logs_by_suite.items()
        },
        "kubernetes_jobs": receipts,
        "postgres_feedback_audit_outbox_atomic": True,
        "idempotent_replay_single_revision": True,
        "stale_and_terminal_conflicts_rejected": True,
        "tenant_bound_prediction_aggregate": True,
        "revision_chain_traceable": True,
        "producer_outbox_rows_remain_unpublished": True,
        "k8s_consumer_schema_present": consumer_schema_present,
        "readiness_receipt_join_logic_verified_with_k8s_fixture": consumer_schema_present,
        "readiness_gate_failed_closed_on_missing_schema": not consumer_schema_present,
        "zero_candidate_rejected": True,
        "shared_postgres_touched": True,
        "run_scoped_postgres_rows_removed": True,
        "mock_enabled": False,
        "authority_runtime_flag_enabled_in_candidate_manifest": False,
        "producer_runtime_flag_enabled_in_candidate_manifest": False,
        "real_kafka_consumer_receipt_observed": False,
        "production_applied": False,
        "run_scoped_kubernetes_resources_removed": not args.keep,
        "does_not_prove": [
            "real Kafka consumer receipt", "producer broker ACK", "MLOps training execution",
            "production rollout", "large feedback queue performance", "Windows Chrome acceptance",
            "authorization to enable MODEL_FEEDBACK_REVISION_PRODUCER_V1_ENABLED",
            "global milestone completion",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
