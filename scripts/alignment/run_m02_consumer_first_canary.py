#!/usr/bin/env python3
"""Execute the M02 canary in consumer-first order with fail-closed rollback.

Validation and dry-run modes never mutate a cluster. Execution additionally
requires an approved rollout body, a separately signed authorization receipt,
current N001-N012/N014 indexes, and two candidate-bound RUNNING consumer
receipts. The runner never disables consumers or deletes durable data.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import math
import re
import subprocess
import sys
import time
from pathlib import Path
from typing import Any
from urllib import error as urlerror
from urllib import parse as urlparse
from urllib import request as urlrequest

import yaml

from build_topic1_task_registry import validate_against_schema
from trusted_signature_service import SignatureVerificationRequest
from validate_m02_delivery_package import DeliveryPackageError, validate_delivery_package
from verify_trusted_signature import TrustedSignatureClientError, verify_exact_payload


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/deployments/m02-rollout-plan.schema.json"
DEFAULT_PLAN = ROOT / "deployments/releases/topic1/m02-canary-rollout.v1.yaml"
ACL_VALIDATOR = ROOT / "scripts/alignment/generate_kafka_acl_plan.py"
REQUIRED_PARENT_NUMBERS = (*range(1, 13), 14)
AUTHORITY_ROLES = ["PROJECT_OWNER", "TEST_OWNER", "ACCEPTANCE_AUTHORITY"]
STAGE_SEQUENCE = [
    (1, "VERIFY_TOPIC_ACL"),
    (2, "VERIFY_CONSUMERS_RUNNING"),
    (3, "ENABLE_GATEWAY_WRITERS"),
    (4, "ENABLE_PROBE_PRODUCER"),
    (5, "OBSERVE_AND_RECONCILE"),
]
ROLLBACK_SEQUENCE = [
    "DISABLE_PROBE_PRODUCER",
    "DISABLE_GATEWAY_WRITERS",
    "RESTORE_IMAGE_CONFIG_ROUTE",
    "RECONCILE_OFFSETS_SPOOL_OBJECTS",
]
STOP_METRICS = {
    "consumer_not_running",
    "checkpoint_failures",
    "kafka_produce_errors",
    "dlq_permanent_records",
    "capture_unexplained_difference",
    "object_hash_mismatches",
    "pcap_metadata_without_object",
}
JOB_NAME_REGEX = r"^(?:Session Aggregation Job V2|PCAP Index (?:Carrier )?Job v2)$"
CONSUMER_TOPICS = {
    "flink-session-job": (["flow.events.v1"], ["session.events.v1", "dlq.v1"]),
    "flink-pcap-index-job": (["pcap.index.v1"], ["dlq.v1"]),
}
M02_TOPIC_PARTITIONS = {
    "flow.events.v1": 16,
    "session.events.v1": 8,
    "pcap.index.v1": 8,
    "dlq.v1": 4,
}
M02_ACL_PRINCIPALS = {
    "ingest-gateway",
    "flink-session-job",
    "flink-pcap-index-job",
}
KAFKA_LIVE_VERIFY_SCRIPT = r'''set -eu
bootstrap=${1:?bootstrap server required}
kafka_home=${KAFKA_HOME:-/opt/kafka}
client_config=$(mktemp)
trap 'rm -f "$client_config"' EXIT HUP INT TERM

escape_property() {
  case "$1" in
    *$'\n'*|*$'\r'*) return 1 ;;
  esac
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

username=$(escape_property "${KAFKA_INTER_BROKER_USERNAME:?missing broker username}")
password=$(escape_property "${KAFKA_INTER_BROKER_PASSWORD:?missing broker password}")
truststore_password=$(escape_property "${KAFKA_TLS_TRUSTSTORE_PASSWORD:?missing truststore password}")
umask 077
{
  printf '%s\n' 'security.protocol=SASL_SSL'
  printf '%s\n' 'sasl.mechanism=SCRAM-SHA-512'
  printf 'sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="%s" password="%s";\n' "$username" "$password"
  printf '%s\n' 'ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12'
  printf '%s\n' 'ssl.truststore.type=PKCS12'
  printf 'ssl.truststore.password=%s\n' "$truststore_password"
} > "$client_config"

for topic in flow.events.v1 session.events.v1 pcap.index.v1 dlq.v1; do
  printf '@@TOPIC:%s\n' "$topic"
  "$kafka_home/bin/kafka-topics.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --describe --topic "$topic"
  printf '@@ACL_TOPIC:%s\n' "$topic"
  "$kafka_home/bin/kafka-acls.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --list --topic "$topic"
done
for group in flink-session-job flink-pcap-index-job; do
  printf '@@ACL_GROUP:%s\n' "$group"
  "$kafka_home/bin/kafka-acls.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --list --group "$group"
  printf '@@GROUP_LAG:%s\n' "$group"
  "$kafka_home/bin/kafka-consumer-groups.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --describe --group "$group"
done
printf '%s\n' '@@ACL_CLUSTER:kafka-cluster'
"$kafka_home/bin/kafka-acls.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --list --cluster
'''
ROLE_SCOPES = {
    "PROJECT_OWNER": ["CANDIDATE", "PROFILE", "ENVIRONMENT", "PURPOSE", "PROJECT", "EXECUTION"],
    "TEST_OWNER": ["CANDIDATE", "PROFILE", "ENVIRONMENT", "PURPOSE", "TEST", "EXECUTION"],
    "ACCEPTANCE_AUTHORITY": ["CANDIDATE", "PROFILE", "ENVIRONMENT", "PURPOSE", "ACCEPTANCE", "EXECUTION"],
}


class CanaryBlocked(RuntimeError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(f"{code}: {detail}")
        self.code = code
        self.detail = detail


class WorkloadMutationFailed(CanaryBlocked):
    def __init__(self, code: str, detail: str, mutation: dict[str, Any]) -> None:
        super().__init__(code, detail)
        self.mutation = mutation


def canonical_sha256(value: Any) -> str:
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def sha256_path(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def repo_path(value: str) -> Path:
    path = ROOT / value
    resolved = path.resolve(strict=False)
    if not resolved.is_relative_to(ROOT.resolve()):
        raise CanaryBlocked("BLOCK_PATH_ESCAPE", value)
    return resolved


def parse_time(value: str) -> dt.datetime:
    parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return parsed


def exact_probe_regex(probe_ids: list[str]) -> str:
    escaped = [re.escape(value).replace(r"\-", "-") for value in probe_ids]
    return "^(?:" + "|".join(escaped) + ")$"


def expected_promql(plan: dict[str, Any]) -> dict[str, str]:
    probe_regex = exact_probe_regex(plan["scope"]["probe_ids"])
    return {
        "consumer_not_running": (
            f'clamp_min(2 - count(flink_jobmanager_job_uptime{{job_name=~"{JOB_NAME_REGEX}"}} > 0), 0)'
        ),
        "checkpoint_failures": (
            f'sum(increase(flink_jobmanager_job_numberOfFailedCheckpoints{{job_name=~"{JOB_NAME_REGEX}"}}[5m]))'
        ),
        "kafka_produce_errors": "sum(increase(ingest_kafka_error_total[5m]))",
        "dlq_permanent_records": (
            'sum(increase(flink_taskmanager_job_task_operator_numRecordsOut'
            f'{{job_name=~"{JOB_NAME_REGEX}",operator_name=~".*DLQ.*"}}[5m]))'
        ),
        "capture_unexplained_difference": (
            'sum(abs(max by(probe_id)(probe_status'
            f'{{probe_id=~"{probe_regex}",metric="packets_dropped"}})'
            ' - on(probe_id) max by(probe_id)(probe_status'
            f'{{probe_id=~"{probe_regex}",metric="capture_allocation_drops"}})'
            ' - on(probe_id) max by(probe_id)(probe_status'
            f'{{probe_id=~"{probe_regex}",metric="capture_kernel_drops"}}))'
            ' + on(probe_id) max by(probe_id)(probe_status'
            f'{{probe_id=~"{probe_regex}",metric="capture_errors"}}))'
        ),
        "object_hash_mismatches": "sum(increase(probe_pcap_object_hash_mismatch_total[5m]))",
        "pcap_metadata_without_object": "sum(increase(probe_pcap_metadata_without_object_total[5m]))",
    }


def load_plan(path: Path) -> dict[str, Any]:
    body = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(body, dict):
        raise CanaryBlocked("BLOCK_ROLLOUT_BODY_INVALID", "YAML root must be an object")
    try:
        validate_against_schema(body, SCHEMA)
    except ValueError as error:
        raise CanaryBlocked("BLOCK_ROLLOUT_SCHEMA", str(error)) from error
    validate_plan_semantics(body)
    return body


def validate_plan_semantics(plan: dict[str, Any]) -> None:
    observed_stages = [(item["order"], item["action"]) for item in plan["stages"]]
    if observed_stages != STAGE_SEQUENCE:
        raise CanaryBlocked("BLOCK_STAGE_ORDER", repr(observed_stages))
    if plan["rollback"]["steps"] != ROLLBACK_SEQUENCE:
        raise CanaryBlocked("BLOCK_ROLLBACK_ORDER", repr(plan["rollback"]["steps"]))
    consumers = [item["consumer"] for item in plan["consumer_receipts"]]
    if consumers != ["flink-session-job", "flink-pcap-index-job"]:
        raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_ORDER", repr(consumers))
    metrics = [item["metric"] for item in plan["stop_thresholds"]]
    if len(metrics) != len(set(metrics)) or set(metrics) != STOP_METRICS:
        raise CanaryBlocked("BLOCK_STOP_THRESHOLD_EXACT_SET", repr(metrics))
    if any(item["limit"] != 0 for item in plan["stop_thresholds"]):
        raise CanaryBlocked("BLOCK_NONZERO_STOP_THRESHOLD", "M02 canary thresholds must all be zero")
    expected_queries = expected_promql(plan)
    conflicts = {
        item["metric"]: {"expected": expected_queries[item["metric"]], "observed": item["query"]}
        for item in plan["stop_thresholds"]
        if item["query"] != expected_queries[item["metric"]]
    }
    if conflicts:
        raise CanaryBlocked("BLOCK_PROMQL_IDENTITY", repr(conflicts))
    authorization = plan["execution_authorization"]
    requests = authorization["verification_requests"]
    observed_roles = [item["role"] for item in requests]
    if authorization["status"] == "APPROVED" and observed_roles != AUTHORITY_ROLES:
        raise CanaryBlocked(
            "BLOCK_AUTHORIZATION_REQUEST_SET",
            "approved plan requires one ordered protected-verifier request for each 3-of-3 role",
        )
    if authorization["status"] == "APPROVED" and (
        not authorization["verifier_endpoint"] or not authorization["policy_fingerprint"]
    ):
        raise CanaryBlocked(
            "BLOCK_TRUSTED_VERIFIER_BINDING",
            "approved plan requires a pinned HTTPS verifier endpoint and policy fingerprint",
        )
    if authorization["status"] == "APPROVED" and (
        not re.fullmatch(r"[0-9a-f]{64}", plan["candidate_id"])
        or not re.fullmatch(r"[0-9a-f]{40}", plan["candidate_commit"])
        or not re.fullmatch(r"[0-9a-f]{64}", plan["delivery_package_sha256"])
    ):
        raise CanaryBlocked(
            "BLOCK_CANDIDATE_IDENTITY",
            "approved plan requires exact candidate manifest, delivery package, and Git identities",
        )
    if authorization["status"] == "PENDING_EXTERNAL_APPROVAL" and requests:
        raise CanaryBlocked(
            "BLOCK_PENDING_AUTHORIZATION_HAS_REQUESTS",
            "pending plan must not claim protected verification requests",
        )
    if authorization["status"] == "PENDING_EXTERNAL_APPROVAL" and (
        authorization["verifier_endpoint"] is not None
        or authorization["policy_fingerprint"] is not None
    ):
        raise CanaryBlocked(
            "BLOCK_PENDING_AUTHORIZATION_HAS_VERIFIER",
            "pending plan must not claim a protected verifier binding",
        )


def require_current_parent_indexes(plan: dict[str, Any]) -> list[dict[str, str]]:
    receipts: list[dict[str, str]] = []
    for number in REQUIRED_PARENT_NUMBERS:
        path = ROOT / "doc/02_acceptance/topic1/tasks" / f"t1-m02-n{number:03d}" / "current-evidence-index.json"
        if not path.is_file():
            raise CanaryBlocked("BLOCK_PARENT_CURRENT_INDEX_MISSING", str(path.relative_to(ROOT)))
        body = json.loads(path.read_text(encoding="utf-8"))
        try:
            validate_against_schema(body, ROOT / "contracts/alignment/current-evidence-index.schema.json")
        except ValueError as error:
            raise CanaryBlocked("BLOCK_PARENT_INDEX_SCHEMA", f"N{number:03d}:{error}") from error
        expected_parent = f"T1-M02-N{number:03d}"
        if body["index_id"] != f"CURRENT-IDX-{expected_parent}":
            raise CanaryBlocked("BLOCK_PARENT_INDEX_IDENTITY", expected_parent)
        if body["candidate_manifest_sha256"] != plan["candidate_id"]:
            raise CanaryBlocked("BLOCK_PARENT_INDEX_CANDIDATE_MISMATCH", expected_parent)
        if body["profile_id"] != plan["profile_id"] or body["environment_id"] != plan["environment_id"]:
            raise CanaryBlocked("BLOCK_PARENT_INDEX_PROFILE_ENVIRONMENT", expected_parent)
        if not body["evidence_runs"] or any(item["result"] != "PASS" for item in body["evidence_runs"]):
            raise CanaryBlocked("BLOCK_PARENT_INDEX_NOT_PASS", expected_parent)
        receipts.append({"path": str(path.relative_to(ROOT)), "sha256": sha256_path(path)})
    return receipts


def validate_consumer_receipts(plan: dict[str, Any]) -> list[dict[str, str]]:
    receipts: list[dict[str, str]] = []
    for binding in plan["consumer_receipts"]:
        path = repo_path(binding["path"])
        if not path.is_file():
            raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_MISSING", binding["path"])
        body = json.loads(path.read_text(encoding="utf-8"))
        expected = {
            "schema_version": 1,
            "artifact_kind": "M02_CONSUMER_READINESS_RECEIPT",
            "rollout_id": plan["rollout_id"],
            "candidate_id": plan["candidate_id"],
            "consumer": binding["consumer"],
            "state": "RUNNING",
            "activation": "CONSUMER_FIRST_IDLE",
            "producer_enabled": False,
        }
        conflicts = {key: (expected_value, body.get(key)) for key, expected_value in expected.items() if body.get(key) != expected_value}
        if conflicts:
            raise CanaryBlocked("BLOCK_CONSUMER_RECEIPT_CONFLICT", f"{binding['consumer']}:{conflicts}")
        if not isinstance(body.get("job_id"), str) or len(body["job_id"]) != 32:
            raise CanaryBlocked("BLOCK_CONSUMER_JOB_ID", binding["consumer"])
        if not re.fullmatch(r"[0-9a-fA-F]{32}", body["job_id"]):
            raise CanaryBlocked("BLOCK_CONSUMER_JOB_ID", binding["consumer"])
        if not isinstance(body.get("completed_checkpoint_id"), int) or body["completed_checkpoint_id"] < 0:
            raise CanaryBlocked("BLOCK_CONSUMER_CHECKPOINT", binding["consumer"])
        expected_inputs, expected_outputs = CONSUMER_TOPICS[binding["consumer"]]
        if body.get("input_topics") != expected_inputs or body.get("output_topics") != expected_outputs:
            raise CanaryBlocked("BLOCK_CONSUMER_TOPIC_BINDING", binding["consumer"])
        if not re.fullmatch(r"[0-9a-f]{64}", str(body.get("jar_sha256", ""))):
            raise CanaryBlocked("BLOCK_CONSUMER_JAR_IDENTITY", binding["consumer"])
        try:
            ready_at = parse_time(body["ready_observed_at"])
        except (KeyError, TypeError, ValueError) as error:
            raise CanaryBlocked("BLOCK_CONSUMER_READY_TIME", binding["consumer"]) from error
        if ready_at > dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=1):
            raise CanaryBlocked("BLOCK_CONSUMER_READY_TIME", binding["consumer"])
        receipts.append({"consumer": binding["consumer"], "path": binding["path"], "sha256": sha256_path(path), "job_id": body["job_id"]})
    return receipts


def validate_authorization(plan: dict[str, Any], plan_path: Path) -> dict[str, Any]:
    authorization = plan["execution_authorization"]
    if authorization["status"] != "APPROVED":
        raise CanaryBlocked("BLOCK_EXTERNAL_APPROVAL_PENDING", authorization["status"])
    rollout_bytes = plan_path.read_bytes()
    rollout_sha256 = hashlib.sha256(rollout_bytes).hexdigest()
    observed: list[dict[str, str]] = []
    signers: set[str] = set()
    for binding in authorization["verification_requests"]:
        path = repo_path(binding["path"])
        if not path.is_file():
            raise CanaryBlocked("BLOCK_AUTHORIZATION_REQUEST_MISSING", binding["path"])
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
            request = SignatureVerificationRequest.from_dict(raw)
        except (OSError, ValueError, TypeError, json.JSONDecodeError) as error:
            raise CanaryBlocked("BLOCK_AUTHORIZATION_REQUEST_INVALID", binding["path"]) from error
        signed = request.signed_payload
        if (
            signed.subject_type != "EXECUTION_OVERLAY"
            or signed.subject_id != plan["rollout_id"]
            or signed.subject_payload.content != rollout_bytes
            or signed.subject_payload.content_sha256 != rollout_sha256
            or signed.subject_payload.size_bytes != len(rollout_bytes)
            or signed.candidate_commit != plan["candidate_commit"]
            or signed.profile_id != plan["profile_id"]
            or signed.environment_id != plan["environment_id"]
            or signed.purpose != "EXECUTION_OVERLAY_ACCEPTANCE"
            or tuple(signed.required_authority_roles) != (binding["role"],)
            or tuple(signed.required_scopes) != tuple(ROLE_SCOPES[binding["role"]])
            or signed.policy_fingerprint_sha256 != authorization["policy_fingerprint"]
        ):
            raise CanaryBlocked("BLOCK_AUTHORIZATION_BINDING", binding["role"])
        try:
            attestation = verify_exact_payload(
                request,
                endpoint=authorization["verifier_endpoint"],
                policy_fingerprint=authorization["policy_fingerprint"],
            )
        except TrustedSignatureClientError as error:
            raise CanaryBlocked("BLOCK_AUTHORIZATION_VERIFIER", f"{binding['role']}:{error}") from error
        if attestation.signer is None or binding["role"] not in attestation.signer.verified_roles:
            raise CanaryBlocked("BLOCK_AUTHORIZATION_ROLE", binding["role"])
        if attestation.signer.signer_id in signers:
            raise CanaryBlocked("BLOCK_AUTHORIZATION_SIGNER_REUSE", attestation.signer.signer_id)
        signers.add(attestation.signer.signer_id)
        observed.append({
            "role": binding["role"],
            "request_path": binding["path"],
            "request_sha256": sha256_path(path),
            "attestation_id": attestation.attestation_id,
            "signer_id": attestation.signer.signer_id,
        })
    if len(observed) != 3 or len(signers) != 3:
        raise CanaryBlocked("BLOCK_AUTHORIZATION_QUORUM", "exact independent 3-of-3 quorum required")
    return {"rollout_sha256": rollout_sha256, "verified_authorities": observed}


def run(
    command: list[str],
    *,
    check: bool = True,
    input_bytes: bytes | None = None,
    timeout_seconds: int = 180,
) -> subprocess.CompletedProcess[bytes]:
    try:
        return subprocess.run(
            command,
            cwd=ROOT,
            input=input_bytes,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=check,
            timeout=timeout_seconds,
        )
    except subprocess.TimeoutExpired as error:
        raise CanaryBlocked("BLOCK_COMMAND_TIMEOUT", " ".join(command[:8])) from error


def kubectl(
    namespace: str,
    *args: str,
    check: bool = True,
    input_bytes: bytes | None = None,
) -> subprocess.CompletedProcess[bytes]:
    return run(["kubectl", "-n", namespace, *args], check=check, input_bytes=input_bytes)


def parse_marked_sections(output: str) -> dict[tuple[str, str], str]:
    sections: dict[tuple[str, str], list[str]] = {}
    active: tuple[str, str] | None = None
    for line in output.splitlines():
        marker = re.fullmatch(r"@@(TOPIC|ACL_TOPIC|ACL_GROUP|ACL_CLUSTER|GROUP_LAG):(.+)", line.strip())
        if marker:
            active = (marker.group(1), marker.group(2))
            if active in sections:
                raise CanaryBlocked("BLOCK_KAFKA_LIVE_DUPLICATE_SECTION", repr(active))
            sections[active] = []
        elif active is not None:
            sections[active].append(line)
    return {key: "\n".join(lines) for key, lines in sections.items()}


def acl_entries(section: str) -> set[tuple[str, str]]:
    return {
        (match.group(1), match.group(2).upper())
        for match in re.finditer(
            r"principal=([^,\s]+),\s*host=[^,]+,\s*operation=([^,\s]+),\s*permissionType=ALLOW",
            section,
            re.IGNORECASE,
        )
    }


def denied_acl_entries(section: str) -> set[tuple[str, str]]:
    return {
        (match.group(1), match.group(2).upper())
        for match in re.finditer(
            r"principal=([^,\s]+),\s*host=[^,]+,\s*operation=([^,\s]+),\s*permissionType=DENY",
            section,
            re.IGNORECASE,
        )
    }


def expected_m02_acls(catalog: dict[str, Any]) -> dict[tuple[str, str], set[tuple[str, str]]]:
    principals = {
        item["id"]: item["principal"]
        for item in catalog["principals"]
        if item["id"] in M02_ACL_PRINCIPALS
    }
    if set(principals) != M02_ACL_PRINCIPALS:
        raise CanaryBlocked("BLOCK_TOPIC_ACL_PRINCIPAL_SET", repr(sorted(principals)))
    expected: dict[tuple[str, str], set[tuple[str, str]]] = {}
    for binding in catalog["topic_bindings"]:
        topic = binding["topic"]
        if topic not in M02_TOPIC_PARTITIONS:
            continue
        topic_entries = expected.setdefault(("ACL_TOPIC", topic), set())
        for producer in binding["producers"]:
            if producer in principals:
                topic_entries.update({(principals[producer], "DESCRIBE"), (principals[producer], "WRITE")})
                expected.setdefault(("ACL_CLUSTER", "kafka-cluster"), set()).add(
                    (principals[producer], "IDEMPOTENTWRITE")
                )
        for consumer in binding["consumers"]:
            consumer_id = consumer["principal"]
            if consumer_id not in principals:
                continue
            topic_entries.update({(principals[consumer_id], "DESCRIBE"), (principals[consumer_id], "READ")})
            for group in consumer["groups"]:
                expected.setdefault(("ACL_GROUP", group), set()).update(
                    {(principals[consumer_id], "DESCRIBE"), (principals[consumer_id], "READ")}
                )
    return expected


def kafka_live_query(plan: dict[str, Any]) -> subprocess.CompletedProcess[bytes]:
    live = plan["kafka_verification"]
    return kubectl(
        live["namespace"],
        "exec",
        "-i",
        live["pod"],
        "-c",
        live["container"],
        "--",
        "bash",
        "-s",
        "--",
        live["bootstrap_server"],
        check=False,
        input_bytes=KAFKA_LIVE_VERIFY_SCRIPT.encode(),
    )


def verify_topic_acl(plan: dict[str, Any]) -> dict[str, str]:
    completed = run([sys.executable, str(ACL_VALIDATOR), "--check-generated"], check=False)
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_TOPIC_ACL_STATIC_DRIFT", completed.stdout.decode(errors="replace")[-4000:])
    topic_catalog_path = ROOT / "contracts/events/kafka-topic-catalog.v1.json"
    acl_catalog_path = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
    topic_catalog = json.loads(topic_catalog_path.read_text(encoding="utf-8"))
    observed_partitions = {
        item["name"]: item["partitions"]
        for item in topic_catalog["topics"]
        if item["name"] in M02_TOPIC_PARTITIONS
    }
    if observed_partitions != M02_TOPIC_PARTITIONS:
        raise CanaryBlocked("BLOCK_TOPIC_CATALOG_CONTRACT", repr(observed_partitions))
    expected_acls = expected_m02_acls(json.loads(acl_catalog_path.read_text(encoding="utf-8")))
    relevant_principals = {principal for entries in expected_acls.values() for principal, _ in entries}
    completed = kafka_live_query(plan)
    output = completed.stdout.decode(errors="replace")
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_TOPIC_ACL_LIVE_QUERY", output[-4000:])
    sections = parse_marked_sections(output)
    for topic, partitions in M02_TOPIC_PARTITIONS.items():
        section = sections.get(("TOPIC", topic), "")
        match = re.search(rf"\bTopic:\s*{re.escape(topic)}\b.*?\bPartitionCount:\s*(\d+)\b", section)
        if match is None or int(match.group(1)) != partitions:
            raise CanaryBlocked("BLOCK_KAFKA_TOPIC_LIVE_CONTRACT", f"{topic}:{section[-1000:]}")
    for resource, required in expected_acls.items():
        section = sections.get(resource, "")
        observed = {entry for entry in acl_entries(section) if entry[0] in relevant_principals}
        denied = {entry for entry in denied_acl_entries(section) if entry[0] in relevant_principals}
        if observed != required or denied:
            raise CanaryBlocked(
                "BLOCK_KAFKA_ACL_LIVE_CONTRACT",
                f"{resource}:missing={sorted(required - observed)} extra={sorted(observed - required)} "
                f"denied={sorted(denied)}",
            )
    return {
        "acl_catalog_sha256": sha256_path(acl_catalog_path),
        "topic_catalog_sha256": sha256_path(topic_catalog_path),
        "live_observation_sha256": hashlib.sha256(completed.stdout).hexdigest(),
    }


def parse_consumer_group_lag(
    sections: dict[tuple[str, str], str],
) -> dict[str, dict[str, int]]:
    expected_topics = {
        "flink-session-job": "flow.events.v1",
        "flink-pcap-index-job": "pcap.index.v1",
    }
    result: dict[str, dict[str, int]] = {}
    for group, topic in expected_topics.items():
        rows: dict[int, int] = {}
        for line in sections.get(("GROUP_LAG", group), "").splitlines():
            columns = line.split()
            if len(columns) < 6 or columns[0] != group or columns[1] != topic:
                continue
            try:
                partition = int(columns[2])
                lag = int(columns[5])
            except ValueError as error:
                raise CanaryBlocked("BLOCK_KAFKA_OFFSET_PARSE", f"{group}:{line}") from error
            if partition in rows:
                raise CanaryBlocked("BLOCK_KAFKA_OFFSET_DUPLICATE_PARTITION", f"{group}:{partition}")
            rows[partition] = lag
        expected_partitions = set(range(M02_TOPIC_PARTITIONS[topic]))
        if set(rows) != expected_partitions:
            raise CanaryBlocked(
                "BLOCK_KAFKA_OFFSET_PARTITION_SET",
                f"{group}:expected={sorted(expected_partitions)} observed={sorted(rows)}",
            )
        result[group] = {"partition_count": len(rows), "total_lag": sum(rows.values())}
    return result


def gateway_env(plan: dict[str, Any], enabled: bool) -> list[str]:
    if enabled:
        return [
            "M02_FLOW_WRITER_V1_ENABLED=true",
            "M02_PCAP_WRITER_V1_ENABLED=true",
            f"M02_CANARY_TENANT_ID={plan['scope']['tenant_id']}",
            f"M02_CANARY_PROBE_IDS={','.join(plan['scope']['probe_ids'])}",
        ]
    return ["M02_FLOW_WRITER_V1_ENABLED=false", "M02_PCAP_WRITER_V1_ENABLED=false", "M02_CANARY_TENANT_ID-", "M02_CANARY_PROBE_IDS-"]


def probe_env(plan: dict[str, Any], enabled: bool) -> list[str]:
    if enabled:
        return ["M02_CAPTURE_PRODUCER_V1_ENABLED=true", f"M02_CAPTURE_CANARY_PROBE_IDS={','.join(plan['scope']['probe_ids'])}"]
    return ["M02_CAPTURE_PRODUCER_V1_ENABLED=false", "M02_CAPTURE_CANARY_PROBE_IDS-"]


def get_workload_template(namespace: str, resource: str) -> dict[str, Any]:
    completed = kubectl(namespace, "get", resource, "-o", "json", check=False)
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_KUBERNETES_GET_WORKLOAD", completed.stdout.decode(errors="replace")[-4000:])
    try:
        body = json.loads(completed.stdout)
        template = body["spec"]["template"]
        if not isinstance(template, dict) or not template:
            raise ValueError("spec.template is empty")
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise CanaryBlocked("BLOCK_KUBERNETES_WORKLOAD_RESPONSE", f"{resource}:{error}") from error
    return template


def mutation_receipt(mutation: dict[str, Any]) -> dict[str, str]:
    return {
        "resource": mutation["resource"],
        "before_template_sha256": canonical_sha256(mutation["before_template"]),
        "applied_template_sha256": canonical_sha256(mutation["applied_template"]),
    }


def begin_workload_env(namespace: str, resource: str, values: list[str]) -> dict[str, Any]:
    before = get_workload_template(namespace, resource)
    completed = kubectl(namespace, "set", "env", resource, *values, check=False)
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_KUBERNETES_SET_ENV", completed.stdout.decode(errors="replace")[-4000:])
    try:
        after = get_workload_template(namespace, resource)
    except CanaryBlocked as error:
        mutation = {"resource": resource, "before_template": before, "applied_template": {}}
        raise WorkloadMutationFailed(error.code, error.detail, mutation) from error
    mutation = {"resource": resource, "before_template": before, "applied_template": after}
    if canonical_sha256(before) == canonical_sha256(after):
        raise CanaryBlocked("BLOCK_KUBERNETES_NO_TEMPLATE_CHANGE", resource)
    return mutation


def wait_workload_rollout(namespace: str, mutation: dict[str, Any]) -> None:
    resource = mutation["resource"]
    completed = kubectl(namespace, "rollout", "status", resource, "--timeout=180s", check=False)
    if completed.returncode != 0:
        raise WorkloadMutationFailed(
            "BLOCK_KUBERNETES_ROLLOUT",
            completed.stdout.decode(errors="replace")[-4000:],
            mutation,
        )


def rollback(plan: dict[str, Any], mutations: list[dict[str, Any]]) -> list[dict[str, Any]]:
    namespace = plan["namespace"]
    actions: list[dict[str, Any]] = []
    for mutation in reversed(mutations):
        resource = mutation["resource"]
        patch = [
            {"op": "test", "path": "/spec/template", "value": mutation["applied_template"]},
            {"op": "replace", "path": "/spec/template", "value": mutation["before_template"]},
        ]
        completed = kubectl(
            namespace,
            "patch",
            resource,
            "--type=json",
            "-p",
            json.dumps(patch, separators=(",", ":")),
            check=False,
        )
        action = {**mutation_receipt(mutation), "patch_exit_code": completed.returncode}
        if completed.returncode == 0:
            rollout = kubectl(namespace, "rollout", "status", resource, "--timeout=180s", check=False)
            action["rollout_exit_code"] = rollout.returncode
            try:
                restored = get_workload_template(namespace, resource)
                action["restored_template_sha256"] = canonical_sha256(restored)
                action["restored_exact"] = restored == mutation["before_template"]
            except CanaryBlocked as error:
                action["restored_exact"] = False
                action["verification_failure"] = str(error)
        else:
            action["rollout_exit_code"] = -1
            action["restored_exact"] = False
            action["patch_failure"] = completed.stdout.decode(errors="replace")[-4000:]
        actions.append(action)
    return actions


def reconcile_after_rollback(plan: dict[str, Any]) -> dict[str, Any]:
    completed = kafka_live_query(plan)
    output = completed.stdout.decode(errors="replace")
    if completed.returncode != 0:
        raise CanaryBlocked("BLOCK_ROLLBACK_KAFKA_RECONCILIATION", output[-4000:])
    offsets = parse_consumer_group_lag(parse_marked_sections(output))
    nonzero_offsets = {group: item for group, item in offsets.items() if item["total_lag"] != 0}
    queries = {
        "pcap_spool_queue_depth": "max(probe_pcap_upload_queue_depth)",
        "object_hash_mismatches": "sum(increase(probe_pcap_object_hash_mismatch_total[5m]))",
        "pcap_metadata_without_object": "sum(increase(probe_pcap_metadata_without_object_total[5m]))",
    }
    observed: dict[str, float] = {}
    for metric, query in queries.items():
        observed[metric] = max(query_prometheus(plan["observation"]["prometheus_url"], query))
    nonzero_metrics = {name: value for name, value in observed.items() if value != 0}
    if nonzero_offsets or nonzero_metrics:
        raise CanaryBlocked(
            "BLOCK_ROLLBACK_DURABLE_RECONCILIATION",
            f"offsets={nonzero_offsets} metrics={nonzero_metrics}",
        )
    return {
        "status": "PASS",
        "consumer_offsets": offsets,
        "durable_metrics": observed,
        "kafka_observation_sha256": hashlib.sha256(completed.stdout).hexdigest(),
    }


def query_prometheus(origin: str, query: str) -> list[float]:
    parsed = urlparse.urlsplit(origin)
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.hostname
        or parsed.username
        or parsed.password
        or parsed.query
        or parsed.fragment
    ):
        raise CanaryBlocked("BLOCK_PROMETHEUS_ORIGIN", origin)
    base_path = parsed.path.rstrip("/")
    endpoint = urlparse.urlunsplit((
        parsed.scheme,
        parsed.netloc,
        f"{base_path}/api/v1/query",
        urlparse.urlencode({"query": query, "time": dt.datetime.now(dt.timezone.utc).timestamp()}),
        "",
    ))
    try:
        with urlrequest.urlopen(endpoint, timeout=10) as response:
            declared = response.headers.get("Content-Length")
            if declared is not None and int(declared) > 1_048_576:
                raise CanaryBlocked("BLOCK_PROMETHEUS_RESPONSE_SIZE", declared)
            payload = response.read(1_048_577)
    except (OSError, ValueError, urlerror.URLError) as error:
        raise CanaryBlocked("BLOCK_PROMETHEUS_TRANSPORT", str(error)) from error
    if len(payload) > 1_048_576:
        raise CanaryBlocked("BLOCK_PROMETHEUS_RESPONSE_SIZE", str(len(payload)))
    try:
        body = json.loads(payload)
        if body.get("status") != "success" or not isinstance(body.get("data"), dict):
            raise ValueError("Prometheus response is not success")
        data = body["data"]
        if data.get("resultType") == "vector":
            result = data.get("result")
            if not isinstance(result, list) or not result:
                raise ValueError("Prometheus vector is empty")
            raw_values = [item["value"][1] for item in result]
        elif data.get("resultType") == "scalar":
            raw_values = [data["result"][1]]
        else:
            raise ValueError(f"unsupported result type {data.get('resultType')!r}")
        values = [float(value) for value in raw_values]
    except (KeyError, IndexError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise CanaryBlocked("BLOCK_PROMETHEUS_RESPONSE", str(error)) from error
    if any(not math.isfinite(value) or value < 0 for value in values):
        raise CanaryBlocked("BLOCK_PROMETHEUS_VALUE", repr(values))
    return values


def observe(plan: dict[str, Any]) -> dict[str, Any]:
    observation = plan["observation"]
    started = time.monotonic()
    deadline = started + observation["minimum_seconds"]
    poll_count = 0
    maxima = {item["metric"]: 0.0 for item in plan["stop_thresholds"]}
    latest = dict(maxima)
    while True:
        poll_count += 1
        for threshold in plan["stop_thresholds"]:
            values = query_prometheus(observation["prometheus_url"], threshold["query"])
            observed = max(values)
            metric = threshold["metric"]
            latest[metric] = observed
            maxima[metric] = max(maxima[metric], observed)
            if observed > threshold["limit"]:
                raise CanaryBlocked(
                    "BLOCK_CANARY_STOP_THRESHOLD",
                    f"{metric} observed={observed} limit={threshold['limit']}",
                )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(observation["poll_seconds"], remaining))
    return {
        "status": "PASS",
        "minimum_seconds": observation["minimum_seconds"],
        "poll_count": poll_count,
        "maxima": maxima,
        "latest": latest,
    }


def execute(plan: dict[str, Any], plan_path: Path) -> dict[str, Any]:
    result: dict[str, Any] = {
        "schema_version": 1,
        "artifact_kind": "M02_CONSUMER_FIRST_CANARY_RESULT",
        "rollout_id": plan["rollout_id"],
        "candidate_id": plan["candidate_id"],
        "status": "FAIL",
        "production_applied": False,
        "plan_sha256": sha256_path(plan_path),
        "stage_results": [],
    }
    mutations: list[dict[str, Any]] = []
    try:
        result["authorization"] = validate_authorization(plan, plan_path)
        try:
            delivery = validate_delivery_package(
                candidate_commit=plan["candidate_commit"],
                candidate_manifest_path=repo_path(plan["candidate_manifest_path"]),
                candidate_manifest_sha256=plan["candidate_id"],
                profile_id=plan["profile_id"],
                environment_id=plan["environment_id"],
            )
        except DeliveryPackageError as error:
            raise CanaryBlocked("BLOCK_DELIVERY_PACKAGE", str(error)) from error
        if delivery["package_sha256"] != plan["delivery_package_sha256"]:
            raise CanaryBlocked(
                "BLOCK_DELIVERY_PACKAGE_IDENTITY",
                f"expected={plan['delivery_package_sha256']} observed={delivery['package_sha256']}",
            )
        result["delivery_package"] = {
            "package_sha256": delivery["package_sha256"],
            "artifacts": delivery["package"]["artifacts"],
        }
        result["parent_indexes"] = require_current_parent_indexes(plan)
        result["stage_results"].append({"stage": "VERIFY_TOPIC_ACL", "status": "PASS", **verify_topic_acl(plan)})
        result["consumer_receipts"] = validate_consumer_receipts(plan)
        result["stage_results"].append({"stage": "VERIFY_CONSUMERS_RUNNING", "status": "PASS"})
        gateway_mutation = begin_workload_env(
            plan["namespace"],
            "deployment/ingest-gateway",
            gateway_env(plan, True),
        )
        mutations.append(gateway_mutation)
        result["production_applied"] = True
        wait_workload_rollout(plan["namespace"], gateway_mutation)
        result["stage_results"].append({
            "stage": "ENABLE_GATEWAY_WRITERS",
            "status": "PASS",
            **mutation_receipt(gateway_mutation),
        })
        probe_mutation = begin_workload_env(
            plan["namespace"],
            "daemonset/probe-agent",
            probe_env(plan, True),
        )
        mutations.append(probe_mutation)
        wait_workload_rollout(plan["namespace"], probe_mutation)
        result["stage_results"].append({
            "stage": "ENABLE_PROBE_PRODUCER",
            "status": "PASS",
            **mutation_receipt(probe_mutation),
        })
        result["observation"] = observe(plan)
        result["stage_results"].append({"stage": "OBSERVE_AND_RECONCILE", "status": "PASS"})
        result["status"] = "PASS"
        return result
    except Exception as error:
        if isinstance(error, WorkloadMutationFailed) and error.mutation not in mutations:
            mutations.append(error.mutation)
            result["production_applied"] = True
        result["failure"] = {
            "code": error.code if isinstance(error, CanaryBlocked) else "CANARY_EXECUTION_ERROR",
            "detail": error.detail if isinstance(error, CanaryBlocked) else str(error),
        }
        if result["production_applied"]:
            try:
                result["rollback"] = rollback(plan, mutations)
                workload_restored = all(
                    item["patch_exit_code"] == 0
                    and item["rollout_exit_code"] == 0
                    and item["restored_exact"] is True
                    for item in result["rollback"]
                )
                if workload_restored:
                    result["durable_reconciliation"] = reconcile_after_rollback(plan)
                    result["rollback_status"] = "PASS"
                else:
                    result["rollback_status"] = "FAIL"
                    result["durable_reconciliation"] = {"status": "NOT_EXECUTED_WORKLOAD_RESTORE_FAILED"}
            except Exception as rollback_error:
                result["rollback_status"] = "FAIL"
                result["rollback_failure"] = str(rollback_error)
            result["status"] = "FAIL"
        else:
            result["status"] = "BLOCKED"
        return result


def dry_run_commands(plan: dict[str, Any]) -> list[list[str]]:
    namespace = plan["namespace"]
    return [
        [sys.executable, str(ACL_VALIDATOR.relative_to(ROOT)), "--check-generated"],
        [
            "kubectl", "-n", plan["kafka_verification"]["namespace"], "exec", "-i",
            plan["kafka_verification"]["pod"], "-c", plan["kafka_verification"]["container"],
            "--", "bash", "-s", "--", plan["kafka_verification"]["bootstrap_server"],
        ],
        ["kubectl", "-n", namespace, "set", "env", "deployment/ingest-gateway", *gateway_env(plan, True)],
        ["kubectl", "-n", namespace, "rollout", "status", "deployment/ingest-gateway", "--timeout=180s"],
        ["kubectl", "-n", namespace, "set", "env", "daemonset/probe-agent", *probe_env(plan, True)],
        ["kubectl", "-n", namespace, "rollout", "status", "daemonset/probe-agent", "--timeout=180s"],
    ]


def self_check() -> dict[str, Any]:
    plan = load_plan(DEFAULT_PLAN)
    return {
        "status": "PASS",
        "stage_sequence": STAGE_SEQUENCE,
        "rollback_sequence": ROLLBACK_SEQUENCE,
        "default_plan_authorization": plan["execution_authorization"]["status"],
        "default_plan_production_applied": False,
        "commands": dry_run_commands(plan),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", type=Path, default=DEFAULT_PLAN)
    parser.add_argument("--validate-only", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--self-check", action="store_true")
    parser.add_argument("--result-output", type=Path)
    args = parser.parse_args()
    modes = sum((args.validate_only, args.dry_run, args.execute, args.self_check))
    if modes != 1:
        parser.error("select exactly one mode")
    try:
        if args.self_check:
            payload = self_check()
        else:
            plan_path = args.plan.resolve(strict=True)
            plan = load_plan(plan_path)
            if args.validate_only:
                payload = {"status": "PASS", "production_applied": False, "plan_sha256": sha256_path(plan_path)}
            elif args.dry_run:
                payload = {"status": "PASS", "production_applied": False, "commands": dry_run_commands(plan)}
            else:
                payload = execute(plan, plan_path)
        if args.result_output:
            args.result_output.parent.mkdir(parents=True, exist_ok=True)
            args.result_output.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 0 if payload["status"] == "PASS" else 1
    except (CanaryBlocked, ValueError, OSError, json.JSONDecodeError) as error:
        payload = {"status": "BLOCKED", "production_applied": False, "failure": str(error)}
        print(json.dumps(payload, sort_keys=True, indent=2))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
