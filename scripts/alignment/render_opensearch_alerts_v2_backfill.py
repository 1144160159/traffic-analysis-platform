#!/usr/bin/env python3
"""Render a default-suspended, plan-bound T-OS-002 backfill Job."""

from __future__ import annotations

import argparse
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from candidate_snapshot import build_snapshot
from plan_opensearch_alerts_v2_backfill import plan_sha256
from render_opensearch_alerts_v2_expand import dns_name, parse_time, sha256, validate_window


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/opensearch/index-governance.v1.json"
MAX_PLAN_AGE_SECONDS = 900
IMAGE = "docker.io/curlimages/curl@sha256:1ab04d023ece37e6ec991bf3306ad04e0ef0084e94a5c6b6563cfcb9563169db"


def canonical_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def validate_plan(plan: dict[str, Any], *, now: datetime, max_age_seconds: int = MAX_PLAN_AGE_SECONDS) -> None:
    if plan.get("remediation_id") != "T-OS-002" or plan.get("mode") != "READ_ONLY_PLAN":
        raise ValueError("backfill plan identity or mode is invalid")
    if plan.get("scoped_evidence_status") != "PASS" or plan.get("execution_readiness") != "READY":
        raise ValueError("backfill plan is not READY")
    if plan.get("production_applied") is not False or plan.get("production_mutations") != []:
        raise ValueError("backfill plan must be read-only evidence")
    binding = plan.get("binding")
    if not isinstance(binding, dict) or plan.get("plan_sha256") != plan_sha256(binding):
        raise ValueError("backfill plan SHA-256 does not match its binding")
    captured = parse_time(str(plan.get("captured_at", "")))
    age = (now - captured).total_seconds()
    if age < -30 or age > max_age_seconds:
        raise ValueError("backfill plan is stale or from the future")
    if binding.get("target_write_index") is None or binding.get("source_count", 0) < 1:
        raise ValueError("backfill plan target or source count is invalid")


def secret_env(name: str, secret_name: str, key: str) -> dict[str, Any]:
    return {
        "name": name,
        "valueFrom": {"secretKeyRef": {"name": secret_name, "key": key}},
    }


def render_documents(
    *,
    plan: dict[str, Any],
    run_id: str,
    approval_id: str,
    approved_by: str,
    not_before_epoch: int,
    expires_at_epoch: int,
    g0_candidate_sha256: str,
    g0_manifest_sha256: str,
    contract_file_sha256: str,
) -> list[dict[str, Any]]:
    binding = plan["binding"]
    plan_hash = plan["plan_sha256"]
    plan_text = canonical_json(plan)
    plan_file_hash = hashlib.sha256(plan_text.encode()).hexdigest()
    secret_name = dns_name("os-v2-backfill-approval", run_id)
    config_name = dns_name("os-v2-backfill-plan", run_id)
    job_name = dns_name("backfill-os-alerts-v2", run_id)
    labels = {
        "traffic.io/remediation-id": "T-OS-002",
        "traffic.io/migration-phase": "backfill",
        "traffic.io/run-id": run_id,
    }
    config = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": config_name, "namespace": "middleware", "labels": labels},
        "immutable": True,
        "data": {
            "plan.json": plan_text,
            "source-count-query.json": canonical_json(binding["source_count_query"]),
            "target-count-query.json": canonical_json(binding["target_count_query"]),
            "reindex-request.json": canonical_json(binding["request"]),
        },
    }
    secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {
            "name": secret_name,
            "namespace": "middleware",
            "labels": labels,
            "annotations": {"traffic.io/approval-expires-at-epoch": str(expires_at_epoch)},
        },
        "immutable": True,
        "type": "Opaque",
        "stringData": {
            "approval_id": approval_id,
            "approved_by": approved_by,
            "approval_nonce": run_id,
            "not_before_epoch": str(not_before_epoch),
            "expires_at_epoch": str(expires_at_epoch),
            "cluster_uuid": str(binding["cluster_uuid"]),
            "g0_candidate_sha256": g0_candidate_sha256,
            "g0_manifest_sha256": g0_manifest_sha256,
            "contract_file_sha256": contract_file_sha256,
            "plan_sha256": plan_hash,
            "plan_file_sha256": plan_file_hash,
        },
    }
    expected = {
        "EXPECTED_APPROVAL_NONCE": run_id,
        "EXPECTED_CLUSTER_UUID": str(binding["cluster_uuid"]),
        "EXPECTED_G0_CANDIDATE_SHA256": g0_candidate_sha256,
        "EXPECTED_G0_MANIFEST_SHA256": g0_manifest_sha256,
        "EXPECTED_CONTRACT_FILE_SHA256": contract_file_sha256,
        "EXPECTED_PLAN_SHA256": plan_hash,
        "EXPECTED_PLAN_FILE_SHA256": plan_file_hash,
        "SOURCE_INDEX": str(binding["source_index"]),
        "TARGET_ALIAS": str(binding["target_alias"]),
        "TARGET_WRITE_INDEX": str(binding["target_write_index"]),
        "EXPECTED_SOURCE_COUNT": str(binding["source_count"]),
        "REINDEX_PATH": str(plan["execution"]["path"]),
    }
    approval_env = [
        ("CHANGE_APPROVAL_ID", "approval_id"),
        ("CHANGE_APPROVED_BY", "approved_by"),
        ("CHANGE_APPROVAL_NONCE", "approval_nonce"),
        ("CHANGE_WINDOW_NOT_BEFORE_EPOCH", "not_before_epoch"),
        ("CHANGE_WINDOW_EXPIRES_AT_EPOCH", "expires_at_epoch"),
        ("APPROVED_CLUSTER_UUID", "cluster_uuid"),
        ("APPROVED_G0_CANDIDATE_SHA256", "g0_candidate_sha256"),
        ("APPROVED_G0_MANIFEST_SHA256", "g0_manifest_sha256"),
        ("APPROVED_CONTRACT_FILE_SHA256", "contract_file_sha256"),
        ("APPROVED_PLAN_SHA256", "plan_sha256"),
        ("APPROVED_PLAN_FILE_SHA256", "plan_file_sha256"),
    ]
    env = [
        secret_env("OPENSEARCH_ADMIN_PASSWORD", "traffic-credentials", "OPENSEARCH_ADMIN_PASSWORD"),
        *[secret_env(name, secret_name, key) for name, key in approval_env],
        *[{"name": name, "value": value} for name, value in expected.items()],
    ]
    script = r'''
OS="http://opensearch-0.opensearch.middleware.svc:9200"
PLAN_DIR="/etc/opensearch-backfill-plan"
MAX_APPROVAL_WINDOW_SECONDS=14400
test -n "$CHANGE_APPROVAL_ID"
test -n "$CHANGE_APPROVED_BY"
test "$CHANGE_APPROVAL_NONCE" = "$EXPECTED_APPROVAL_NONCE"
test "$APPROVED_CLUSTER_UUID" = "$EXPECTED_CLUSTER_UUID"
test "$APPROVED_G0_CANDIDATE_SHA256" = "$EXPECTED_G0_CANDIDATE_SHA256"
test "$APPROVED_G0_MANIFEST_SHA256" = "$EXPECTED_G0_MANIFEST_SHA256"
test "$APPROVED_CONTRACT_FILE_SHA256" = "$EXPECTED_CONTRACT_FILE_SHA256"
test "$APPROVED_PLAN_SHA256" = "$EXPECTED_PLAN_SHA256"
test "$APPROVED_PLAN_FILE_SHA256" = "$EXPECTED_PLAN_FILE_SHA256"
echo "$CHANGE_WINDOW_NOT_BEFORE_EPOCH" | grep -Eq '^[0-9]+$'
echo "$CHANGE_WINDOW_EXPIRES_AT_EPOCH" | grep -Eq '^[0-9]+$'
test "$CHANGE_WINDOW_EXPIRES_AT_EPOCH" -gt "$CHANGE_WINDOW_NOT_BEFORE_EPOCH"
test "$((CHANGE_WINDOW_EXPIRES_AT_EPOCH - CHANGE_WINDOW_NOT_BEFORE_EPOCH))" -le "$MAX_APPROVAL_WINDOW_SECONDS"
CURRENT_EPOCH="$(date -u +%s)"
test "$CURRENT_EPOCH" -ge "$CHANGE_WINDOW_NOT_BEFORE_EPOCH"
test "$CURRENT_EPOCH" -le "$CHANGE_WINDOW_EXPIRES_AT_EPOCH"
ACTUAL_PLAN_FILE_SHA256="$(sha256sum "$PLAN_DIR/plan.json" | cut -d' ' -f1)"
test "$ACTUAL_PLAN_FILE_SHA256" = "$EXPECTED_PLAN_FILE_SHA256"

ROOT_RESPONSE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" "$OS/")"
echo "$ROOT_RESPONSE" | grep -Fq "\"cluster_uuid\":\"${EXPECTED_CLUSTER_UUID}\""
curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" "$OS/_cluster/health" | grep -Eq '"status":"(green|yellow)"'
ALIAS_RESPONSE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" "$OS/_alias/$TARGET_ALIAS")"
echo "$ALIAS_RESPONSE" | grep -Fq "\"${TARGET_WRITE_INDEX}\""
echo "$ALIAS_RESPONSE" | grep -Fq '"is_write_index":true'

SOURCE_COUNT_BEFORE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" -X POST "$OS/$SOURCE_INDEX/_count" -H 'Content-Type: application/json' --data-binary @"$PLAN_DIR/source-count-query.json" | sed -n 's/.*"count":\([0-9][0-9]*\).*/\1/p')"
TARGET_COUNT_BEFORE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" -X POST "$OS/$TARGET_ALIAS/_count" -H 'Content-Type: application/json' --data-binary @"$PLAN_DIR/target-count-query.json" | sed -n 's/.*"count":\([0-9][0-9]*\).*/\1/p')"
test "$SOURCE_COUNT_BEFORE" = "$EXPECTED_SOURCE_COUNT"
test "$TARGET_COUNT_BEFORE" = "0"

TASK_RESPONSE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" -X POST "$OS$REINDEX_PATH" -H 'Content-Type: application/json' --data-binary @"$PLAN_DIR/reindex-request.json")"
TASK_ID="$(echo "$TASK_RESPONSE" | sed -n 's/.*"task":"\([^"]*\)".*/\1/p')"
test -n "$TASK_ID"
echo "task_id=$TASK_ID plan_sha256=$EXPECTED_PLAN_SHA256"
while :; do
  CURRENT_EPOCH="$(date -u +%s)"
  if [ "$CURRENT_EPOCH" -gt "$CHANGE_WINDOW_EXPIRES_AT_EPOCH" ]; then
    curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" -X POST "$OS/_tasks/$TASK_ID/_cancel" >/dev/null || true
    echo "approval window expired; task cancellation requested" >&2
    exit 1
  fi
  TASK_STATE="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" "$OS/_tasks/$TASK_ID?wait_for_completion=true&timeout=30s")"
  if echo "$TASK_STATE" | grep -Fq '"completed":true'; then
    break
  fi
  sleep 1
done
echo "$TASK_STATE" | grep -Fq '"failures":[]'
echo "$TASK_STATE" | grep -Fq '"version_conflicts":0'
TARGET_COUNT_AFTER="$(curl -fsS -u "admin:${OPENSEARCH_ADMIN_PASSWORD}" -X POST "$OS/$TARGET_ALIAS/_count" -H 'Content-Type: application/json' --data-binary @"$PLAN_DIR/target-count-query.json" | sed -n 's/.*"count":\([0-9][0-9]*\).*/\1/p')"
test "$TARGET_COUNT_AFTER" = "$EXPECTED_SOURCE_COUNT"
echo "backfill_complete source_count=$EXPECTED_SOURCE_COUNT target_count=$TARGET_COUNT_AFTER"
'''.strip()
    job = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": job_name,
            "namespace": "middleware",
            "labels": labels,
            "annotations": {
                "traffic.io/execution-mode": "approval-required",
                "traffic.io/approval-id": approval_id,
                "traffic.io/approval-expires-at-epoch": str(expires_at_epoch),
                "traffic.io/g0-candidate-sha256": g0_candidate_sha256,
                "traffic.io/g0-manifest-sha256": g0_manifest_sha256,
                "traffic.io/contract-file-sha256": contract_file_sha256,
                "traffic.io/plan-sha256": plan_hash,
            },
        },
        "spec": {
            "suspend": True,
            "backoffLimit": 0,
            "activeDeadlineSeconds": 14_400,
            "ttlSecondsAfterFinished": 604_800,
            "template": {
                "metadata": {"labels": labels},
                "spec": {
                    "restartPolicy": "Never",
                    "containers": [{
                        "name": "backfill-alerts-v2",
                        "image": IMAGE,
                        "command": ["sh", "-ceu"],
                        "env": env,
                        "volumeMounts": [{"name": "plan", "mountPath": "/etc/opensearch-backfill-plan", "readOnly": True}],
                        "args": [script],
                    }],
                    "volumes": [{"name": "plan", "configMap": {"name": config_name}}],
                },
            },
        },
    }
    return [config, secret, job]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--approval-id", required=True)
    parser.add_argument("--approved-by", required=True)
    parser.add_argument("--not-before", required=True)
    parser.add_argument("--expires-at", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite rendered backfill: {output}")
    now = datetime.now(timezone.utc)
    try:
        plan = json.loads(args.plan.resolve().read_text(encoding="utf-8"))
        validate_plan(plan, now=now)
        not_before_epoch, expires_at_epoch = validate_window(
            parse_time(args.not_before), parse_time(args.expires_at), now,
        )
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        raise SystemExit(str(exc)) from exc

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("G0 manifest must be PASS")
    g0_candidate = (g0.get("candidate_source") or {}).get("content_sha256", "")
    if g0_candidate != build_snapshot()["content_sha256"]:
        raise SystemExit("G0 candidate does not match the current source snapshot")
    if plan["binding"]["cluster_uuid"] != plan["observation"]["cluster_uuid"]:
        raise SystemExit("plan cluster UUID observation and binding differ")

    rendered = render_documents(
        plan=plan,
        run_id=args.run_id,
        approval_id=args.approval_id,
        approved_by=args.approved_by,
        not_before_epoch=not_before_epoch,
        expires_at_epoch=expires_at_epoch,
        g0_candidate_sha256=g0_candidate,
        g0_manifest_sha256=sha256(g0_path),
        contract_file_sha256=sha256(CONTRACT),
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(yaml.safe_dump_all(rendered, sort_keys=False), encoding="utf-8")
    print(json.dumps({
        "status": "RENDERED_SUSPENDED",
        "run_id": args.run_id,
        "plan_sha256": plan["plan_sha256"],
        "output": str(output),
        "output_sha256": sha256(output),
        "g0_candidate_sha256": g0_candidate,
        "production_mutations": [],
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
