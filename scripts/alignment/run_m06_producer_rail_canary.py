#!/usr/bin/env python3
"""Run one M06 producer rail canary with exact scope and reversible K8s state.

Validation and dry-run never mutate Kubernetes. Execution is deliberately
blocked by the repository plan until an external authorization receipt and the
phase's consumer/prerequisite receipts are bound to an immutable candidate.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import math
import re
import subprocess
import time
from pathlib import Path
from typing import Any
from urllib import parse as urlparse
from urllib import request as urlrequest

import yaml


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_PLAN = ROOT / "deployments/releases/topic1/m06-producer-rails-canary.v1.yaml"
PHASES = ["asset-events", "asset-binding-ingest", "asset-binding-probe", "device-logs"]
PHASE_ENV = {
    "asset-events": {
        "ASSET_EVENT_OUTBOX_TENANT_ID": "{tenant_id}",
        "ASSET_EVENT_OUTBOX_ENABLED": "true",
    },
    "asset-binding-ingest": {
        "M02_CANARY_TENANT_ID": "{tenant_id}",
        "M02_CANARY_PROBE_IDS": "{probe_ids}",
        "M06_ASSET_BINDING_WRITER_V1_ENABLED": "true",
    },
    "asset-binding-probe": {
        "M06_ASSET_BINDING_CANARY_TENANT_ID": "{tenant_id}",
        "M06_ASSET_BINDING_CANARY_PROBE_IDS": "{probe_ids}",
        "M06_ASSET_BINDING_UPLOAD_V1_ENABLED": "true",
    },
}
TENANT_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
PROBE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


class CanaryBlocked(RuntimeError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


def sha256_path(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical_sha256(value: Any) -> str:
    return hashlib.sha256(
        json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()


def repo_path(value: str) -> Path:
    path = (ROOT / value).resolve(strict=False)
    if not path.is_relative_to(ROOT.resolve()):
        raise CanaryBlocked("BLOCK_PATH_ESCAPE", value)
    return path


def load_plan(path: Path) -> dict[str, Any]:
    plan = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(plan, dict):
        raise CanaryBlocked("BLOCK_PLAN_SHAPE", "YAML root must be an object")
    if plan.get("schema_version") != 1 or plan.get("artifact_kind") != "M06_PRODUCER_RAILS_CANARY_PLAN":
        raise CanaryBlocked("BLOCK_PLAN_IDENTITY", repr((plan.get("schema_version"), plan.get("artifact_kind"))))
    if plan.get("phases") != PHASES:
        raise CanaryBlocked("BLOCK_PHASE_ORDER", repr(plan.get("phases")))
    if plan.get("production_applied") is not False:
        raise CanaryBlocked("BLOCK_REPOSITORY_PLAN_MUTATION_CLAIM", repr(plan.get("production_applied")))
    if not isinstance(plan.get("profile_id"), str) or not plan["profile_id"].strip():
        raise CanaryBlocked("BLOCK_PROFILE_ID", repr(plan.get("profile_id")))
    expected_workloads = set(PHASES)
    if set(plan.get("workloads", {})) != expected_workloads:
        raise CanaryBlocked("BLOCK_WORKLOAD_EXACT_SET", repr(sorted(plan.get("workloads", {}))))
    if set(plan.get("consumer_readiness", {})) != expected_workloads:
        raise CanaryBlocked("BLOCK_CONSUMER_BINDING_EXACT_SET", repr(sorted(plan.get("consumer_readiness", {}))))
    observation = plan.get("consumer_observation", {})
    consumers = observation.get("consumers", {}) if isinstance(observation, dict) else {}
    if set(consumers) != expected_workloads:
        raise CanaryBlocked("BLOCK_CONSUMER_OBSERVATION_EXACT_SET", repr(sorted(consumers)))
    if not isinstance(observation.get("stability_seconds"), int) or observation["stability_seconds"] < 10:
        raise CanaryBlocked("BLOCK_CONSUMER_STABILITY_WINDOW", repr(observation.get("stability_seconds")))
    if not isinstance(observation.get("receipt_ttl_seconds"), int) or not 60 <= observation["receipt_ttl_seconds"] <= 3600:
        raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_TTL", repr(observation.get("receipt_ttl_seconds")))
    for phase, consumer in consumers.items():
        if consumer.get("consumer") != plan["consumer_readiness"][phase].get("consumer"):
            raise CanaryBlocked("BLOCK_CONSUMER_IDENTITY", phase)
        if not consumer.get("topic") or not consumer.get("group_id") or not isinstance(consumer.get("partitions"), int) or consumer["partitions"] < 1:
            raise CanaryBlocked("BLOCK_CONSUMER_KAFKA_BINDING", phase)
        if not isinstance(consumer.get("required_env"), dict) or not consumer["required_env"]:
            raise CanaryBlocked("BLOCK_CONSUMER_ENV_BINDING", phase)
        if not isinstance(consumer.get("producer_guards"), list) or not consumer["producer_guards"]:
            raise CanaryBlocked("BLOCK_PRODUCER_GUARD_SET", phase)
    if set(plan.get("prerequisite_acceptance", {})) != expected_workloads:
        raise CanaryBlocked("BLOCK_PREREQUISITE_EXACT_SET", repr(sorted(plan.get("prerequisite_acceptance", {}))))
    thresholds = plan.get("observation", {}).get("stop_thresholds", {})
    if set(thresholds) != expected_workloads:
        raise CanaryBlocked("BLOCK_THRESHOLD_PHASE_EXACT_SET", repr(sorted(thresholds)))
    for phase, entries in thresholds.items():
        metrics = [entry.get("metric") for entry in entries]
        if not entries or len(metrics) != len(set(metrics)):
            raise CanaryBlocked("BLOCK_THRESHOLD_METRIC_SET", phase)
        if any(not isinstance(entry.get("limit"), (int, float)) or entry["limit"] < 0 or not entry.get("query") for entry in entries):
            raise CanaryBlocked("BLOCK_THRESHOLD_SHAPE", phase)
    authorization = plan.get("execution_authorization", {})
    if authorization.get("status") not in {"PENDING_EXTERNAL_APPROVAL", "APPROVED"}:
        raise CanaryBlocked("BLOCK_AUTHORIZATION_STATE", repr(authorization.get("status")))
    return plan


def validate_scope(plan: dict[str, Any], phase: str) -> tuple[str, list[str], dict[str, str]]:
    scope = plan.get("scope", {})
    tenant_id = str(scope.get("tenant_id", "")).strip()
    probe_ids = [str(value).strip() for value in scope.get("probe_ids", [])]
    device_map = scope.get("device_ip_tenant_map", {})
    if not TENANT_RE.fullmatch(tenant_id) or tenant_id.startswith("PENDING_"):
        raise CanaryBlocked("BLOCK_TENANT_SCOPE", tenant_id)
    if phase in {"asset-binding-ingest", "asset-binding-probe"}:
        if not probe_ids or len(probe_ids) != len(set(probe_ids)) or any(not PROBE_RE.fullmatch(value) for value in probe_ids):
            raise CanaryBlocked("BLOCK_PROBE_SCOPE", repr(probe_ids))
    if not isinstance(device_map, dict):
        raise CanaryBlocked("BLOCK_DEVICE_MAP_SHAPE", repr(device_map))
    normalized_map = {str(ip).strip(): str(value).strip() for ip, value in device_map.items()}
    if phase == "device-logs":
        if not normalized_map or any(not ip or value != tenant_id for ip, value in normalized_map.items()):
            raise CanaryBlocked("BLOCK_DEVICE_MAP_SCOPE", repr(normalized_map))
    return tenant_id, probe_ids, normalized_map


def parse_time(value: str) -> dt.datetime:
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timezone is required")
    return parsed


def validate_bound_receipt(
    plan: dict[str, Any], binding: dict[str, Any] | None, *, kind: str, identity: str,
    expected_status: str = "PASS",
) -> dict[str, str]:
    if not isinstance(binding, dict) or not binding.get("path") or not binding.get("sha256"):
        raise CanaryBlocked("BLOCK_RECEIPT_BINDING_MISSING", identity)
    path = repo_path(binding["path"])
    if not path.is_file() or sha256_path(path) != binding["sha256"]:
        raise CanaryBlocked("BLOCK_RECEIPT_HASH", identity)
    body = json.loads(path.read_text(encoding="utf-8"))
    if body.get("artifact_kind") != kind or body.get("status") != expected_status:
        raise CanaryBlocked("BLOCK_RECEIPT_RESULT", identity)
    expected_binding = (plan["candidate_id"], plan["profile_id"], plan["environment_id"])
    observed_binding = (body.get("candidate_id"), body.get("profile_id"), body.get("environment_id"))
    if observed_binding != expected_binding:
        raise CanaryBlocked("BLOCK_RECEIPT_CANDIDATE_PROFILE_ENVIRONMENT", identity)
    return {"path": binding["path"], "sha256": binding["sha256"]}


def validate_execution_authority(plan: dict[str, Any], phase: str) -> dict[str, Any]:
    authorization = plan["execution_authorization"]
    if authorization.get("status") != "APPROVED":
        raise CanaryBlocked("BLOCK_EXTERNAL_APPROVAL_PENDING", str(authorization.get("status")))
    if not SHA_RE.fullmatch(str(plan.get("candidate_id", ""))):
        raise CanaryBlocked("BLOCK_CANDIDATE_ID", str(plan.get("candidate_id")))
    binding = {"path": authorization.get("receipt_path"), "sha256": authorization.get("receipt_sha256")}
    receipt = validate_bound_receipt(
        plan, binding, kind="M06_CANARY_EXECUTION_AUTHORIZATION", identity="execution-authorization",
        expected_status="APPROVED",
    )
    body = json.loads(repo_path(receipt["path"]).read_text(encoding="utf-8"))
    tenant_id, probe_ids, device_map = validate_scope(plan, phase)
    expected = {
        "rollout_id": plan["rollout_id"],
        "phase": phase,
        "cluster_context": plan["cluster_context"],
        "tenant_id": tenant_id,
        "probe_ids": probe_ids,
        "device_map_sha256": canonical_sha256(device_map),
    }
    conflicts = {key: (value, body.get(key)) for key, value in expected.items() if body.get(key) != value}
    if conflicts:
        raise CanaryBlocked("BLOCK_AUTHORIZATION_SCOPE", repr(conflicts))
    roles = [item.get("role") for item in body.get("approvers", [])]
    signers = [item.get("signer_id") for item in body.get("approvers", [])]
    if roles != ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"] or len(signers) != len(set(signers)):
        raise CanaryBlocked("BLOCK_AUTHORIZATION_QUORUM", repr(roles))
    try:
        approved_at = parse_time(body["approved_at"])
        expires_at = parse_time(body["expires_at"])
    except (KeyError, TypeError, ValueError) as error:
        raise CanaryBlocked("BLOCK_AUTHORIZATION_TIME", str(error)) from error
    now = dt.datetime.now(dt.timezone.utc)
    if approved_at > now or expires_at <= now or expires_at - approved_at > dt.timedelta(hours=8):
        raise CanaryBlocked("BLOCK_AUTHORIZATION_TIME", f"approved={approved_at} expires={expires_at}")
    return receipt


def validate_phase_receipts(plan: dict[str, Any], phase: str) -> dict[str, Any]:
    readiness_binding = plan.get("consumer_readiness", {}).get(phase)
    readiness = validate_bound_receipt(
        plan, readiness_binding, kind="M06_CONSUMER_READINESS_RECEIPT", identity=f"{phase}-consumer"
    )
    readiness_body = json.loads(repo_path(readiness["path"]).read_text(encoding="utf-8"))
    observation = plan["consumer_observation"]["consumers"][phase]
    if (
        readiness_body.get("phase") != phase
        or readiness_body.get("consumer") != readiness_binding.get("consumer")
        or readiness_body.get("topic") != observation["topic"]
        or readiness_body.get("group_id") != observation["group_id"]
        or readiness_body.get("state") != "RUNNING"
        or readiness_body.get("observed_before_producer") is not True
        or readiness_body.get("producer_enabled") is not False
        or readiness_body.get("production_applied") is not True
    ):
        raise CanaryBlocked("BLOCK_CONSUMER_NOT_RUNNING", phase)
    try:
        observed_at = parse_time(readiness_body["observed_at"])
        expires_at = parse_time(readiness_body["expires_at"])
    except (KeyError, TypeError, ValueError) as error:
        raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_TIME", phase) from error
    now = dt.datetime.now(dt.timezone.utc)
    if observed_at > now + dt.timedelta(minutes=1) or expires_at <= now or expires_at - observed_at > dt.timedelta(hours=1):
        raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_TIME", phase)
    result: dict[str, Any] = {"consumer_readiness": readiness}
    prerequisite = plan.get("prerequisite_acceptance", {}).get(phase)
    if phase != "asset-events":
        result["prerequisite_acceptance"] = validate_bound_receipt(
            plan, prerequisite, kind="M06_PHASE_ACCEPTANCE_RECEIPT", identity=f"{phase}-prerequisite"
        )
    return result


def run(command: list[str], *, check: bool = True, input_bytes: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    completed = subprocess.run(
        command, cwd=ROOT, input=input_bytes, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, timeout=240, check=False,
    )
    if check and completed.returncode != 0:
        raise CanaryBlocked("BLOCK_COMMAND_FAILED", f"{' '.join(command[:8])}: {completed.stdout.decode(errors='replace')[-3000:]}")
    return completed


def kubectl(namespace: str, *args: str, check: bool = True, input_bytes: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    return run(["kubectl", "-n", namespace, *args], check=check, input_bytes=input_bytes)


def phase_values(plan: dict[str, Any], phase: str) -> dict[str, str]:
    tenant_id, probe_ids, _ = validate_scope(plan, phase)
    substitutions = {"tenant_id": tenant_id, "probe_ids": ",".join(probe_ids)}
    return {key: value.format(**substitutions) for key, value in PHASE_ENV.get(phase, {}).items()}


def resource_json(namespace: str, resource: str) -> dict[str, Any]:
    return json.loads(kubectl(namespace, "get", resource, "-o", "json").stdout)


def current_context() -> str:
    return run(["kubectl", "config", "current-context"]).stdout.decode().strip()


def required_rbac(phase: str, resource: str) -> list[tuple[str, str]]:
    kind = resource.split("/", 1)[0]
    permissions = [("get", kind), ("patch", kind)]
    if phase == "device-logs":
        permissions.extend(
            [
                ("get", "configmaps"),
                ("patch", "configmaps"),
                ("get", "externalsecrets.external-secrets.io"),
            ]
        )
    return permissions


def verify_rbac(namespace: str, phase: str, resource: str) -> None:
    for verb, kind in required_rbac(phase, resource):
        allowed = kubectl(namespace, "auth", "can-i", verb, kind).stdout.decode().strip()
        if allowed != "yes":
            raise CanaryBlocked("BLOCK_KUBERNETES_RBAC", f"{verb} {kind}={allowed}")


def container_env(resource: dict[str, Any], container_name: str, keys: list[str]) -> dict[str, Any]:
    containers = resource["spec"]["template"]["spec"]["containers"]
    matches = [item for item in containers if item.get("name") == container_name]
    if len(matches) != 1:
        raise CanaryBlocked("BLOCK_CONTAINER_IDENTITY", container_name)
    env = {item["name"]: item for item in matches[0].get("env", []) if item.get("name") in keys}
    result: dict[str, Any] = {}
    for key in keys:
        if key not in env:
            result[key] = {"present": False}
        elif set(env[key]) == {"name", "value"}:
            result[key] = {"present": True, "value": env[key]["value"]}
        else:
            raise CanaryBlocked("BLOCK_TARGET_ENV_VALUE_FROM", key)
    return result


def set_env(namespace: str, resource: str, values: dict[str, str]) -> None:
    kubectl(namespace, "set", "env", resource, *[f"{key}={value}" for key, value in values.items()])


def restore_env(namespace: str, resource: str, before: dict[str, Any]) -> None:
    arguments = [f"{key}={state['value']}" if state["present"] else f"{key}-" for key, state in before.items()]
    kubectl(namespace, "set", "env", resource, *arguments)


def wait_rollout(namespace: str, resource: str) -> None:
    kubectl(namespace, "rollout", "status", resource, "--timeout=180s")


def begin_phase(plan: dict[str, Any], phase: str) -> dict[str, Any]:
    target = plan["workloads"][phase]
    namespace, resource = target["namespace"], target["resource"]
    if phase != "device-logs":
        values = phase_values(plan, phase)
        before = container_env(resource_json(namespace, resource), target["container"], list(values))
        mutation = {"kind": "env", "namespace": namespace, "resource": resource, "before": before, "after": values}
        try:
            set_env(namespace, resource, values)
            wait_rollout(namespace, resource)
        except Exception as error:
            try:
                rollback_mutation(mutation, require_after_state=False)
            except Exception as rollback_error:
                raise CanaryBlocked("BLOCK_PARTIAL_MUTATION_ROLLBACK", f"apply={error}; rollback={rollback_error}") from error
            raise
    else:
        _, _, device_map = validate_scope(plan, phase)
        workload = resource_json(namespace, resource)
        config = resource_json(namespace, "configmap/device-log-collector-config-v1")
        before = {
            "replicas": workload["spec"].get("replicas", 0),
            "device_tenant_map_json": config.get("data", {}).get("device-tenant-map.json", "{}"),
        }
        encoded_map = json.dumps(device_map, sort_keys=True, separators=(",", ":"))
        mutation = {"kind": "device-logs", "namespace": namespace, "resource": resource, "before": before, "after": {"replicas": 1, "device_tenant_map_json": encoded_map}}
        patch = json.dumps({"data": {"device-tenant-map.json": encoded_map}}, separators=(",", ":"))
        try:
            kubectl(namespace, "patch", "configmap/device-log-collector-config-v1", "--type=merge", "-p", patch)
            kubectl(namespace, "scale", resource, "--replicas=1")
            wait_rollout(namespace, resource)
        except Exception as error:
            try:
                rollback_mutation(mutation, require_after_state=False)
            except Exception as rollback_error:
                raise CanaryBlocked("BLOCK_PARTIAL_MUTATION_ROLLBACK", f"apply={error}; rollback={rollback_error}") from error
            raise
    return mutation


def rollback_mutation(mutation: dict[str, Any], *, require_after_state: bool = True) -> dict[str, Any]:
    namespace, resource = mutation["namespace"], mutation["resource"]
    if mutation["kind"] == "env":
        if require_after_state:
            current = container_env(resource_json(namespace, resource), mutation.get("container", resource.split("/", 1)[1]), list(mutation["after"]))
            observed = {key: state.get("value") if state.get("present") else None for key, state in current.items()}
            if observed != mutation["after"]:
                raise CanaryBlocked("BLOCK_ROLLBACK_STATE_DRIFT", repr(observed))
        restore_env(namespace, resource, mutation["before"])
    else:
        if require_after_state:
            workload = resource_json(namespace, resource)
            config = resource_json(namespace, "configmap/device-log-collector-config-v1")
            observed = {"replicas": workload["spec"].get("replicas", 0), "device_tenant_map_json": config.get("data", {}).get("device-tenant-map.json", "{}")}
            if observed != mutation["after"]:
                raise CanaryBlocked("BLOCK_ROLLBACK_STATE_DRIFT", repr(observed))
        kubectl(namespace, "scale", resource, f"--replicas={mutation['before']['replicas']}")
        patch = json.dumps({"data": {"device-tenant-map.json": mutation["before"]["device_tenant_map_json"]}}, separators=(",", ":"))
        kubectl(namespace, "patch", "configmap/device-log-collector-config-v1", "--type=merge", "-p", patch)
    wait_rollout(namespace, resource)
    return {"status": "PASS", "resource": resource, "restored": True}


def query_prometheus(base_url: str, query: str) -> list[float]:
    url = f"{base_url.rstrip('/')}/api/v1/query?{urlparse.urlencode({'query': query})}"
    try:
        with urlrequest.urlopen(url, timeout=10) as response:
            body = json.loads(response.read())
        results = body["data"]["result"]
        values = [float(item["value"][1]) for item in results]
    except Exception as error:
        raise CanaryBlocked("BLOCK_PROMETHEUS_QUERY", f"{query}: {error}") from error
    if not values or any(not math.isfinite(value) or value < 0 for value in values):
        raise CanaryBlocked("BLOCK_PROMETHEUS_RESULT", f"{query}: {values}")
    return values


def observe(plan: dict[str, Any], phase: str) -> dict[str, Any]:
    observation = plan["observation"]
    deadline = time.monotonic() + observation["minimum_seconds"]
    maxima = {item["metric"]: 0.0 for item in observation["stop_thresholds"][phase]}
    polls = 0
    while True:
        polls += 1
        for threshold in observation["stop_thresholds"][phase]:
            observed = max(query_prometheus(observation["prometheus_url"], threshold["query"]))
            maxima[threshold["metric"]] = max(maxima[threshold["metric"]], observed)
            if observed > threshold["limit"]:
                raise CanaryBlocked("BLOCK_STOP_THRESHOLD", f"{threshold['metric']}={observed}>{threshold['limit']}")
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(observation["poll_seconds"], remaining))
    return {"status": "PASS", "polls": polls, "maxima": maxima, "minimum_seconds": observation["minimum_seconds"]}


def dry_run_commands(plan: dict[str, Any], phase: str) -> list[list[str]]:
    target = plan["workloads"][phase]
    namespace, resource = target["namespace"], target["resource"]
    if phase != "device-logs":
        values = PHASE_ENV[phase]
        scope = plan["scope"]
        substitutions = {"tenant_id": scope["tenant_id"], "probe_ids": ",".join(scope["probe_ids"])}
        rendered = {key: value.format(**substitutions) for key, value in values.items()}
        return [["kubectl", "-n", namespace, "set", "env", resource, *[f"{key}={value}" for key, value in rendered.items()]], ["kubectl", "-n", namespace, "rollout", "status", resource, "--timeout=180s"]]
    return [["kubectl", "-n", namespace, "patch", "configmap/device-log-collector-config-v1", "--type=merge", "-p", "<approved-device-map>"], ["kubectl", "-n", namespace, "scale", resource, "--replicas=1"], ["kubectl", "-n", namespace, "rollout", "status", resource, "--timeout=180s"]]


def execute(plan: dict[str, Any], plan_path: Path, phase: str) -> dict[str, Any]:
    result: dict[str, Any] = {
        "schema_version": 1, "artifact_kind": "M06_CANARY_PHASE_RESULT",
        "rollout_id": plan["rollout_id"], "candidate_id": plan["candidate_id"],
        "profile_id": plan["profile_id"], "environment_id": plan["environment_id"], "phase": phase,
        "plan_sha256": sha256_path(plan_path), "status": "BLOCKED", "production_applied": False,
    }
    mutation: dict[str, Any] | None = None
    try:
        result["authorization"] = validate_execution_authority(plan, phase)
        result["prerequisites"] = validate_phase_receipts(plan, phase)
        observed_context = current_context()
        if observed_context != plan["cluster_context"]:
            raise CanaryBlocked("BLOCK_CLUSTER_CONTEXT", f"expected={plan['cluster_context']} observed={observed_context}")
        target = plan["workloads"][phase]
        if phase == "device-logs":
            secret = resource_json(target["namespace"], "externalsecret/kafka-device-log-collector-credentials")
            ready = any(item.get("type") == "Ready" and item.get("status") == "True" for item in secret.get("status", {}).get("conditions", []))
            if not ready:
                raise CanaryBlocked("BLOCK_DEVICE_LOG_CREDENTIAL_NOT_READY", "ExternalSecret Ready=True is required")
        verify_rbac(target["namespace"], phase, target["resource"])
        mutation = begin_phase(plan, phase)
        mutation["container"] = target["container"]
        result["production_applied"] = True
        result["mutation"] = mutation
        result["observation"] = observe(plan, phase)
        result["status"] = "PASS"
        result["activation_retained_for_acceptance"] = True
    except Exception as error:
        result["failure"] = {"code": error.code if isinstance(error, CanaryBlocked) else "CANARY_EXECUTION_ERROR", "detail": error.detail if isinstance(error, CanaryBlocked) else str(error)}
        if mutation is not None:
            try:
                result["rollback"] = rollback_mutation(mutation)
                result["status"] = "FAIL"
            except Exception as rollback_error:
                result["rollback"] = {"status": "FAIL", "detail": str(rollback_error)}
                result["status"] = "FAIL"
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", type=Path, default=DEFAULT_PLAN)
    parser.add_argument("--phase", choices=PHASES, required=True)
    modes = parser.add_mutually_exclusive_group(required=True)
    modes.add_argument("--validate-only", action="store_true")
    modes.add_argument("--dry-run", action="store_true")
    modes.add_argument("--execute", action="store_true")
    modes.add_argument("--rollback-from", type=Path)
    parser.add_argument("--rollback-sha256")
    parser.add_argument("--result-output", type=Path)
    args = parser.parse_args()
    try:
        plan_path = args.plan.resolve(strict=True)
        plan = load_plan(plan_path)
        if args.validate_only:
            payload = {"status": "PASS", "production_applied": False, "plan_sha256": sha256_path(plan_path), "phase": args.phase}
        elif args.dry_run:
            payload = {"status": "PASS", "production_applied": False, "phase": args.phase, "commands": dry_run_commands(plan, args.phase)}
        elif args.execute:
            if args.result_output is None:
                raise CanaryBlocked("BLOCK_RESULT_OUTPUT_REQUIRED", "--execute requires an immutable result output")
            payload = execute(plan, plan_path, args.phase)
        else:
            if args.result_output is None:
                raise CanaryBlocked("BLOCK_RESULT_OUTPUT_REQUIRED", "--rollback-from requires an immutable result output")
            rollback_path = args.rollback_from.resolve(strict=True)
            if not rollback_path.is_relative_to(ROOT.resolve()):
                raise CanaryBlocked("BLOCK_ROLLBACK_PATH_ESCAPE", str(rollback_path))
            if not SHA_RE.fullmatch(str(args.rollback_sha256 or "")) or sha256_path(rollback_path) != args.rollback_sha256:
                raise CanaryBlocked("BLOCK_ROLLBACK_RECEIPT_HASH", str(args.rollback_sha256))
            prior = json.loads(rollback_path.read_text(encoding="utf-8"))
            if (
                prior.get("artifact_kind") != "M06_CANARY_PHASE_RESULT"
                or prior.get("phase") != args.phase
                or prior.get("status") != "PASS"
                or prior.get("production_applied") is not True
                or prior.get("activation_retained_for_acceptance") is not True
                or not isinstance(prior.get("mutation"), dict)
            ):
                raise CanaryBlocked("BLOCK_ROLLBACK_RECEIPT", args.phase)
            expected_binding = (plan["candidate_id"], plan["profile_id"], plan["environment_id"], sha256_path(plan_path))
            observed_binding = (prior.get("candidate_id"), prior.get("profile_id"), prior.get("environment_id"), prior.get("plan_sha256"))
            if observed_binding != expected_binding:
                raise CanaryBlocked("BLOCK_ROLLBACK_BINDING", repr(observed_binding))
            if current_context() != plan["cluster_context"]:
                raise CanaryBlocked("BLOCK_CLUSTER_CONTEXT", "rollback context differs from plan")
            payload = {
                "schema_version": 1,
                "artifact_kind": "M06_CANARY_ROLLBACK_RESULT",
                "rollout_id": plan["rollout_id"],
                "candidate_id": plan["candidate_id"],
                "profile_id": plan["profile_id"],
                "environment_id": plan["environment_id"],
                "phase": args.phase,
                "plan_sha256": sha256_path(plan_path),
                "activation_receipt_sha256": args.rollback_sha256,
                "status": "PASS",
                "production_applied": False,
                "rollback": rollback_mutation(prior["mutation"]),
            }
        if args.result_output:
            output = args.result_output.resolve()
            if output.exists():
                raise CanaryBlocked("BLOCK_RESULT_OVERWRITE", str(output))
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 0 if payload["status"] == "PASS" else 1
    except (CanaryBlocked, OSError, ValueError, json.JSONDecodeError) as error:
        payload = {"status": "BLOCKED", "production_applied": False, "failure": str(error)}
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
