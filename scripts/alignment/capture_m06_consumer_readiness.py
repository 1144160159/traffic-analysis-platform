#!/usr/bin/env python3
"""Capture candidate-bound M06 consumer-first readiness from Kubernetes.

The command is read-only. It refuses to issue a PASS receipt unless the exact
consumer workload is ready on the approved image, every producer guard is off,
and the Kafka group owns the complete input-topic partition set for a bounded
stability window.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import re
import subprocess
import time
from pathlib import Path
from typing import Any

from run_m06_producer_rail_canary import (
    DEFAULT_PLAN,
    PHASES,
    ROOT,
    CanaryBlocked,
    canonical_sha256,
    load_plan,
)


SHA_RE = re.compile(r"^[0-9a-f]{64}$")
KAFKA_NAME_RE = re.compile(r"^[A-Za-z0-9._-]{1,249}$")
IMAGE_DIGEST_RE = re.compile(r"^[^\s]+@sha256:[0-9a-f]{64}$")
KAFKA_GROUP_SCRIPT = r'''set -eu
group=${1:?group required}
bootstrap=${2:?bootstrap required}
kafka_home=${KAFKA_HOME:-/opt/kafka}
client_config=$(mktemp)
trap 'rm -f "$client_config"' EXIT HUP INT TERM

case "$group" in (*[!A-Za-z0-9._-]*|'') exit 64;; esac
umask 077
{
  printf '%s\n' 'security.protocol=SASL_SSL'
  printf '%s\n' 'sasl.mechanism=SCRAM-SHA-512'
  printf 'sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="%s" password="%s";\n' "$KAFKA_INTER_BROKER_USERNAME" "$KAFKA_INTER_BROKER_PASSWORD"
  printf '%s\n' 'ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12'
  printf '%s\n' 'ssl.truststore.type=PKCS12'
  printf 'ssl.truststore.password=%s\n' "$KAFKA_TLS_TRUSTSTORE_PASSWORD"
} > "$client_config"
"$kafka_home/bin/kafka-consumer-groups.sh" \
  --bootstrap-server "$bootstrap" --command-config "$client_config" \
  --describe --group "$group"
'''


def run(command: list[str], *, input_bytes: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    try:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            input=input_bytes,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=180,
            check=False,
        )
    except subprocess.TimeoutExpired as error:
        raise CanaryBlocked("BLOCK_READINESS_COMMAND_TIMEOUT", " ".join(command[:8])) from error
    return completed


def kubectl(namespace: str, *args: str, input_bytes: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    return run(["kubectl", "-n", namespace, *args], input_bytes=input_bytes)


def get_resource(namespace: str, resource: str, *, absent_allowed: bool = False) -> dict[str, Any] | None:
    completed = kubectl(namespace, "get", resource, "-o", "json")
    if completed.returncode != 0:
        output = completed.stdout.decode(errors="replace")
        if absent_allowed and "NotFound" in output:
            return None
        raise CanaryBlocked("BLOCK_READINESS_RESOURCE", f"{namespace}/{resource}: {output[-2000:]}")
    try:
        body = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        raise CanaryBlocked("BLOCK_READINESS_RESOURCE_JSON", f"{namespace}/{resource}") from error
    if not isinstance(body, dict):
        raise CanaryBlocked("BLOCK_READINESS_RESOURCE_JSON", f"{namespace}/{resource}")
    return body


def container(body: dict[str, Any], container_name: str) -> dict[str, Any]:
    containers = body.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
    matches = [item for item in containers if item.get("name") == container_name]
    if len(matches) != 1:
        raise CanaryBlocked("BLOCK_READINESS_CONTAINER", container_name)
    return matches[0]


def exact_env(container_body: dict[str, Any], expected: dict[str, str]) -> dict[str, str]:
    observed: dict[str, str] = {}
    for item in container_body.get("env", []):
        name = item.get("name")
        if name not in expected:
            continue
        if set(item) != {"name", "value"} or not isinstance(item.get("value"), str):
            raise CanaryBlocked("BLOCK_READINESS_ENV_VALUE_FROM", str(name))
        if name in observed:
            raise CanaryBlocked("BLOCK_READINESS_ENV_DUPLICATE", str(name))
        observed[name] = item["value"]
    if observed != expected:
        raise CanaryBlocked("BLOCK_READINESS_ENV", repr({"expected": expected, "observed": observed}))
    return observed


def ready_workload(body: dict[str, Any], resource: str) -> dict[str, Any]:
    kind = body.get("kind")
    metadata = body.get("metadata", {})
    spec, status = body.get("spec", {}), body.get("status", {})
    generation = metadata.get("generation")
    if status.get("observedGeneration", generation) != generation:
        raise CanaryBlocked("BLOCK_READINESS_GENERATION", resource)
    if kind == "Deployment":
        desired = int(spec.get("replicas", 0))
        counts = {name: int(status.get(name, 0)) for name in ("readyReplicas", "updatedReplicas", "availableReplicas")}
        if desired < 1 or any(value != desired for value in counts.values()):
            raise CanaryBlocked("BLOCK_READINESS_REPLICAS", repr({"desired": desired, **counts}))
    elif kind == "DaemonSet":
        desired = int(status.get("desiredNumberScheduled", 0))
        counts = {name: int(status.get(name, 0)) for name in ("numberReady", "updatedNumberScheduled", "numberAvailable")}
        if desired < 1 or any(value != desired for value in counts.values()):
            raise CanaryBlocked("BLOCK_READINESS_REPLICAS", repr({"desired": desired, **counts}))
    elif kind == "Job":
        if int(status.get("succeeded", 0)) != 1 or int(status.get("failed", 0)) != 0:
            raise CanaryBlocked("BLOCK_READINESS_JOB", repr(status))
        desired, counts = 1, {"succeeded": 1}
    else:
        raise CanaryBlocked("BLOCK_READINESS_KIND", repr(kind))
    return {
        "kind": kind,
        "name": metadata.get("name"),
        "uid": metadata.get("uid"),
        "resource_version": metadata.get("resourceVersion"),
        "generation": generation,
        "desired": desired,
        "counts": counts,
    }


def observe_workload(binding: dict[str, Any]) -> dict[str, Any]:
    expected_image = str(binding.get("expected_image", ""))
    if not IMAGE_DIGEST_RE.fullmatch(expected_image):
        raise CanaryBlocked("BLOCK_READINESS_IMAGE_BINDING", expected_image)
    body = get_resource(binding["namespace"], binding["resource"])
    assert body is not None
    selected = container(body, binding["container"])
    if selected.get("image") != expected_image:
        raise CanaryBlocked(
            "BLOCK_READINESS_IMAGE",
            repr({"expected": expected_image, "observed": selected.get("image")}),
        )
    return {**ready_workload(body, binding["resource"]), "image": expected_image}


def observe_producer_guards(guards: list[dict[str, Any]]) -> list[dict[str, Any]]:
    receipts: list[dict[str, Any]] = []
    for guard in guards:
        absent_allowed = guard.get("absent_allowed") is True
        body = get_resource(guard["namespace"], guard["resource"], absent_allowed=absent_allowed)
        if body is None:
            receipts.append({"namespace": guard["namespace"], "resource": guard["resource"], "state": "ABSENT"})
            continue
        if "env" in guard:
            value = exact_env(container(body, guard["container"]), {guard["env"]: guard["expected"]})
            receipts.append({
                "namespace": guard["namespace"], "resource": guard["resource"],
                "container": guard["container"], "env": value,
                "uid": body.get("metadata", {}).get("uid"),
                "generation": body.get("metadata", {}).get("generation"),
            })
        else:
            replicas = int(body.get("spec", {}).get("replicas", 0))
            if replicas > int(guard["replicas_at_most"]):
                raise CanaryBlocked("BLOCK_PRODUCER_ALREADY_ENABLED", f"{guard['resource']} replicas={replicas}")
            receipts.append({
                "namespace": guard["namespace"], "resource": guard["resource"],
                "replicas": replicas, "uid": body.get("metadata", {}).get("uid"),
                "generation": body.get("metadata", {}).get("generation"),
            })
    return receipts


def parse_group_rows(output: str, *, group: str, topic: str, partitions: int) -> dict[str, Any]:
    rows: dict[int, dict[str, Any]] = {}
    for line in output.splitlines():
        columns = line.split()
        if len(columns) < 8 or columns[0] != group or columns[1] != topic:
            continue
        try:
            partition = int(columns[2])
            log_end_offset = int(columns[4])
            lag = None if columns[5] == "-" else int(columns[5])
        except ValueError as error:
            raise CanaryBlocked("BLOCK_READINESS_KAFKA_ROW", line) from error
        if partition in rows:
            raise CanaryBlocked("BLOCK_READINESS_KAFKA_DUPLICATE_PARTITION", str(partition))
        if columns[6] == "-":
            raise CanaryBlocked("BLOCK_READINESS_KAFKA_UNASSIGNED", str(partition))
        rows[partition] = {
            "partition": partition,
            "current_offset": None if columns[3] == "-" else int(columns[3]),
            "log_end_offset": log_end_offset,
            "lag": lag,
            "consumer_id": columns[6],
            "host": columns[7],
        }
    expected = set(range(partitions))
    if set(rows) != expected:
        raise CanaryBlocked(
            "BLOCK_READINESS_KAFKA_PARTITION_SET",
            repr({"expected": sorted(expected), "observed": sorted(rows)}),
        )
    members = sorted({item["consumer_id"] for item in rows.values()})
    return {"partition_count": partitions, "members": members, "rows": [rows[index] for index in sorted(rows)]}


def observe_group(admin: dict[str, Any], binding: dict[str, Any]) -> dict[str, Any]:
    group, topic = binding["group_id"], binding["topic"]
    if not KAFKA_NAME_RE.fullmatch(group) or not KAFKA_NAME_RE.fullmatch(topic):
        raise CanaryBlocked("BLOCK_READINESS_KAFKA_IDENTITY", repr((group, topic)))
    completed = kubectl(
        admin["namespace"], "exec", "-i", admin["pod"], "-c", admin["container"], "--",
        "bash", "-s", "--", group, admin["bootstrap_server"], input_bytes=KAFKA_GROUP_SCRIPT.encode(),
    )
    output = completed.stdout.decode(errors="replace")
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_READINESS_KAFKA_GROUP", output[-3000:])
    parsed = parse_group_rows(output, group=group, topic=topic, partitions=binding["partitions"])
    parsed["observation_sha256"] = hashlib.sha256(completed.stdout).hexdigest()
    return parsed


def capture(plan: dict[str, Any], phase: str) -> dict[str, Any]:
    if not SHA_RE.fullmatch(str(plan.get("candidate_id", ""))):
        raise CanaryBlocked("BLOCK_CANDIDATE_ID", str(plan.get("candidate_id")))
    observed_context = run(["kubectl", "config", "current-context"])
    context = observed_context.stdout.decode().strip()
    if observed_context.returncode != 0 or context != plan["cluster_context"]:
        raise CanaryBlocked("BLOCK_CLUSTER_CONTEXT", f"expected={plan['cluster_context']} observed={context}")
    observation = plan["consumer_observation"]
    binding = observation["consumers"][phase]
    workload_first = observe_workload(binding["workload"])
    guards_first = observe_producer_guards(binding["producer_guards"])
    kafka_first = observe_group(observation["kafka_admin"], binding)
    time.sleep(observation["stability_seconds"])
    workload_second = observe_workload(binding["workload"])
    guards_second = observe_producer_guards(binding["producer_guards"])
    kafka_second = observe_group(observation["kafka_admin"], binding)
    stable = {
        "workload": workload_first == workload_second,
        "producer_guards": guards_first == guards_second,
        "kafka_members": kafka_first["members"] == kafka_second["members"],
        "kafka_partition_count": kafka_first["partition_count"] == kafka_second["partition_count"],
    }
    if not all(stable.values()):
        raise CanaryBlocked("BLOCK_READINESS_UNSTABLE", repr(stable))
    now = dt.datetime.now(dt.timezone.utc)
    expires = now + dt.timedelta(seconds=observation["receipt_ttl_seconds"])
    return {
        "schema_version": 1,
        "artifact_kind": "M06_CONSUMER_READINESS_RECEIPT",
        "phase": phase,
        "candidate_id": plan["candidate_id"],
        "profile_id": plan["profile_id"],
        "environment_id": plan["environment_id"],
        "cluster_context": context,
        "consumer": binding["consumer"],
        "topic": binding["topic"],
        "group_id": binding["group_id"],
        "state": "RUNNING",
        "observed_before_producer": True,
        "producer_enabled": False,
        "workload": workload_second,
        "producer_guards": guards_second,
        "producer_guards_sha256": canonical_sha256(guards_second),
        "kafka_group": kafka_second,
        "stability_seconds": observation["stability_seconds"],
        "observed_at": now.isoformat().replace("+00:00", "Z"),
        "expires_at": expires.isoformat().replace("+00:00", "Z"),
        "production_applied": True,
        "claim": "candidate-bound consumer-first readiness only",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", type=Path, default=DEFAULT_PLAN)
    parser.add_argument("--phase", choices=PHASES, required=True)
    modes = parser.add_mutually_exclusive_group(required=True)
    modes.add_argument("--validate-only", action="store_true")
    modes.add_argument("--observe", action="store_true")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        plan_path = args.plan.resolve(strict=True)
        if not plan_path.is_relative_to(ROOT.resolve()):
            raise CanaryBlocked("BLOCK_PLAN_PATH_ESCAPE", str(plan_path))
        plan = load_plan(plan_path)
        if args.validate_only:
            payload = {"status": "PASS", "phase": args.phase, "production_applied": False}
        else:
            if args.output is None:
                raise CanaryBlocked("BLOCK_RECEIPT_OUTPUT_REQUIRED", "--observe requires --output")
            payload = capture(plan, args.phase)
        if args.output:
            output = args.output.resolve()
            if not output.is_relative_to(ROOT.resolve()):
                raise CanaryBlocked("BLOCK_RECEIPT_PATH_ESCAPE", str(output))
            if output.exists():
                raise CanaryBlocked("BLOCK_RECEIPT_OVERWRITE", str(output))
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 0
    except (CanaryBlocked, OSError, ValueError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "BLOCKED", "production_applied": False, "failure": str(error)}, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
