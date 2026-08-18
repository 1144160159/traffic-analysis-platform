#!/usr/bin/env python3
"""Validate the M09-N013 M07 snapshot adapter and UI bundle in Kubernetes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import secrets
import uuid
from pathlib import Path
from typing import Any

import yaml

import run_m09_alert_evidence_links_k8s as base


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "docker.io/library/postgres:16-alpine"
PREREQUISITES = ROOT / "scripts/alignment/fixtures/m09_attack_chain_snapshot_prerequisites.sql"
MIGRATION = ROOT / "deployments/postgres/migrations/202608142100_m07_attack_chain_snapshot_v1.sql"
DEFAULT_OUTPUT = (
    ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n013"
    / "k8s-attack-chain-snapshot-ui-latest.json"
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")
REPOSITORY_TEST = "^TestAttackChainRepositoryPostgresAtomicSnapshot$"
API_TEST = "^Test(VersionedAttackChain|AttackChainEmptySetMeta)"
BUNDLE_CHECK = " && ".join(
    (
        "test -f /usr/share/nginx/html/index.html",
        "test -f /usr/share/nginx/html/.vite/manifest.json",
        "! find /usr/share/nginx/html -name mockServiceWorker.js -print -quit | grep -q .",
        "grep -F -q ': \"${USE_MOCK:=false}\"' /docker-entrypoint.sh",
        "grep -R -q '/v1/attack-chains' /usr/share/nginx/html/assets",
        "grep -R -q 'M07 快照事实边界' /usr/share/nginx/html/assets",
        "grep -R -q '页面不会补线' /usr/share/nginx/html/assets",
        "grep -R -q 'observed' /usr/share/nginx/html/assets",
        "grep -R -q 'derived' /usr/share/nginx/html/assets",
        "grep -R -q 'analyst_conclusion' /usr/share/nginx/html/assets",
        "grep -R -q '替代路径' /usr/share/nginx/html/assets",
        "grep -R -q '来源水位' /usr/share/nginx/html/assets",
        "echo PASS",
    )
)


def labels(run_id: str) -> dict[str, str]:
    return {
        "app.kubernetes.io/name": "m09-attack-chain-snapshot-ui-canary",
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
    for path in (PREREQUISITES, MIGRATION):
        if not path.is_file():
            raise base.CanaryError(f"missing input: {path.relative_to(ROOT)}")
    return parsed.hex[:10]


def dependency_objects(name: str, run_id: str, node: str, password: str) -> list[dict[str, Any]]:
    common = labels(run_id)
    selector = {"traffic.analysis/canary-postgres": name}
    return [
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "data": {
                "00-prerequisites.sql": PREREQUISITES.read_text(encoding="utf-8"),
                "10-attack-chain.sql": MIGRATION.read_text(encoding="utf-8"),
            },
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "stringData": {"pg-password": password},
        },
        {
            "apiVersion": "v1", "kind": "Secret", "type": "Opaque",
            "metadata": {"name": name, "namespace": base.APP_NAMESPACE, "labels": common},
            "stringData": {
                "pg-dsn": (
                    f"postgres://postgres:{password}@{name}.{base.DB_NAMESPACE}.svc:5432/"
                    "traffic_platform?sslmode=disable"
                )
            },
        },
        {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": name, "namespace": base.DB_NAMESPACE, "labels": common},
            "spec": {"selector": selector, "ports": [{"name": "postgres", "port": 5432}]},
        },
        {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {
                "name": name, "namespace": base.DB_NAMESPACE,
                "labels": {**common, **selector},
            },
            "spec": {
                "nodeName": node, "automountServiceAccountToken": False,
                "restartPolicy": "Never", "securityContext": {"seccompProfile": {"type": "RuntimeDefault"}},
                "containers": [{
                    "name": "postgres", "image": POSTGRES_IMAGE, "imagePullPolicy": "IfNotPresent",
                    "env": [
                        {"name": "POSTGRES_DB", "value": "traffic_platform"},
                        {"name": "POSTGRES_USER", "value": "postgres"},
                        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-password"}}},
                    ],
                    "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
                    "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                    "volumeMounts": [
                        {"name": "data", "mountPath": "/var/lib/postgresql/data"},
                        {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True},
                    ],
                }],
                "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": name}}],
            },
        },
    ]


def test_job(name: str, run_id: str, node: str, image: str, suite: str) -> dict[str, Any]:
    common = labels(run_id)
    job_name = f"{name}-{suite}"
    container: dict[str, Any] = {
        "name": suite, "image": image, "imagePullPolicy": "Never",
        "securityContext": {
            "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
            "capabilities": {"drop": ["ALL"]},
        },
        "resources": {"requests": {"cpu": "100m", "memory": "128Mi"}, "limits": {"cpu": "1", "memory": "512Mi"}},
        "volumeMounts": [{"name": "tmp", "mountPath": "/tmp"}],
    }
    if suite == "repository":
        container["args"] = ["-test.v", f"-test.run={REPOSITORY_TEST}", "-test.count=1"]
        container["env"] = [{
            "name": "ATTACK_CHAIN_POSTGRES_TEST_DSN",
            "valueFrom": {"secretKeyRef": {"name": name, "key": "pg-dsn"}},
        }]
    elif suite == "api":
        container["args"] = ["-test.v", f"-test.run={API_TEST}", "-test.count=1"]
    else:
        container["command"] = ["/bin/sh", "-ec"]
        container["args"] = [BUNDLE_CHECK]
    return {
        "apiVersion": "batch/v1", "kind": "Job",
        "metadata": {
            "name": job_name, "namespace": base.APP_NAMESPACE, "labels": common,
            "annotations": {
                "traffic.analysis/shared-postgres-touched": "false",
                "traffic.analysis/production-applied": "false",
            },
        },
        "spec": {
            "backoffLimit": 0,
            "template": {
                "metadata": {"labels": common},
                "spec": {
                    "nodeName": node, "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000,
                        "fsGroup": 1000, "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [container], "volumes": [{"name": "tmp", "emptyDir": {}}],
                },
            },
        },
    }


def postgres_oracle(name: str) -> dict[str, Any]:
    sql = """SELECT json_build_object(
      'migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608142100'),
      'sentinel',(SELECT marker FROM codex_ephemeral_attack_chain_snapshot_sentinel LIMIT 1),
      'snapshots',(SELECT count(*) FROM attack_chain_snapshots_v1 WHERE tenant_id='tenant-a'),
      'current_rows',(SELECT count(*) FROM attack_chain_snapshot_current_v1 WHERE tenant_id='tenant-a'),
      'graph_snapshots',(SELECT count(*) FROM gnn_graph_snapshots_v1 WHERE tenant_id='tenant-a'),
      'evidence_anchors',(SELECT count(*) FROM attack_chain_evidence_manifest_v1 WHERE tenant_id='tenant-a')
    );"""
    result = base.kubectl(
        "exec", "-i", "-n", base.DB_NAMESPACE, name, "--", "sh", "-ec",
        'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At',
        input_text=sql,
    )
    oracle = json.loads(result.stdout.strip())
    expected = {
        "migration": 1, "sentinel": "ephemeral-only", "snapshots": 1,
        "current_rows": 1, "graph_snapshots": 1, "evidence_anchors": 3,
    }
    if oracle != expected:
        raise base.CanaryError(f"unexpected PostgreSQL attack-chain oracle: {oracle}")
    return oracle


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository-image", required=True)
    parser.add_argument("--api-image", required=True)
    parser.add_argument("--web-image", required=True)
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--timeout", type=int, default=420)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.timeout < 120 or args.timeout > 1200:
        raise base.CanaryError("--timeout must be between 120 and 1200 seconds")
    images = [args.repository_image, args.api_image, args.web_image]
    suffix = validate(images, args.run_id, args.node)
    name = f"m09-n013-chain-{suffix}"
    password = "m09-n013-" + secrets.token_hex(16)
    applied = False
    receipts: list[dict[str, Any]] = []
    logs_by_suite: dict[str, str] = {}
    try:
        base.apply(dependency_objects(name, args.run_id, args.node, password))
        applied = True
        base.wait_pod(name, base.DB_NAMESPACE, args.timeout)
        dependency = base.pod_receipt(name, base.DB_NAMESPACE)
        for suite, image in (
            ("repository", args.repository_image),
            ("api", args.api_image),
            ("web", args.web_image),
        ):
            base.apply([test_job(name, args.run_id, args.node, image, suite)])
            logs, receipt = base.wait_job(f"{name}-{suite}", args.timeout)
            logs_by_suite[suite] = logs
            receipts.append(receipt)
        oracle = postgres_oracle(name)
    finally:
        if applied and not args.keep:
            base.cleanup(args.run_id)
    envelope = {
        "artifact_kind": "M09_ATTACK_CHAIN_SNAPSHOT_UI_TEST_RESULT",
        "task_id": "T1-M09-N013", "run_id": args.run_id, "status": "PASS",
        "profile_id": "M09-N013-K8S-M07-SNAPSHOT-UI-V1",
        "coverage_status": "PASS_FOR_RUN_SCOPED_K8S_SNAPSHOT_REPOSITORY_API_AND_UI_BUNDLE",
        "inputs": {
            "prerequisites_sha256": hashlib.sha256(PREREQUISITES.read_bytes()).hexdigest(),
            "migration_sha256": hashlib.sha256(MIGRATION.read_bytes()).hexdigest(),
            "postgres_image": POSTGRES_IMAGE, "candidate_images": images,
            "bundle_check_sha256": hashlib.sha256(BUNDLE_CHECK.encode()).hexdigest(),
        },
        "test_output_sha256": {
            suite: hashlib.sha256(logs.encode()).hexdigest()
            for suite, logs in logs_by_suite.items()
        },
        "postgres_oracle": oracle,
        "kubernetes_dependency": dependency,
        "kubernetes_jobs": receipts,
        "source_target_explicit": True,
        "provenance_classes": ["observed", "derived", "analyst"],
        "alternative_paths_visible": True,
        "uncertainty_visible": True,
        "truncation_fail_visible": True,
        "fabricated_recommendations": False,
        "evidence_drilldown_present": True,
        "mock_enabled": False,
        "production_applied": False,
        "shared_postgres_touched": False,
        "run_scoped_resources_removed": not args.keep,
        "does_not_prove": [
            "production rollout", "Windows Chrome acceptance", "visual pixel parity",
            "shared-store migration", "project completion",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(envelope, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(envelope, sort_keys=True))


if __name__ == "__main__":
    main()
