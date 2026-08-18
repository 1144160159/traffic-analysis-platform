#!/usr/bin/env python3
"""Run the M07 campaign dual-rail canary on isolated K8s infrastructure.

The runner never patches the production alert-service or Flink deployments.
It creates run-scoped PostgreSQL, consumers, fixture jobs, dispatcher and
correlation workloads, uses the canonical production topics with canary-only
tenant identities, verifies durable PostgreSQL/broker outcomes, and deletes
only its exact run-scoped resources unless --keep is requested.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import secrets
import subprocess
import sys
import time
import uuid
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
DB_NAMESPACE = "databases"
APP_NAMESPACE = "traffic-analysis"
FLINK_NAMESPACE = "flink"
CONTRACT_SHA256 = "f4f564d6d22084c1202634af99fabe29067bd51f5748dfd30b9fd48f0541980b"
INIT_FILES = [
    ("00-prerequisites.sql", ROOT / "scripts/alignment/fixtures/m07_campaign_rail_canary_prerequisites.sql"),
    ("10-alert-links.sql", ROOT / "deployments/postgres/migrations/202607301930_alert_campaign_links.sql"),
    ("20-campaign-aggregate.sql", ROOT / "deployments/postgres/migrations/202608010700_campaign_aggregate_v2.sql"),
    ("30-campaign-membership.sql", ROOT / "deployments/postgres/migrations/202608010730_campaign_membership_aggregate_v2.sql"),
    ("40-campaign-delivery.sql", ROOT / "deployments/postgres/migrations/202608010800_campaign_event_delivery_projection_v2.sql"),
    ("50-campaign-correlation.sql", ROOT / "deployments/postgres/migrations/202608101030_campaign_rail_correlation_v1.sql"),
]
CANONICAL_GROUPS = {
    "proto": "alert-service-campaigns-proto-projection-v1",
    "aggregate": "alert-service-campaign-event-projection-v2",
    "membership": "alert-service-campaign-membership-projection-v2",
}


class CanaryError(RuntimeError):
    pass


def run(command: list[str], *, input_text: str | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(command, input=input_text, text=True, capture_output=True)
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise CanaryError(f"command failed ({' '.join(command[:4])}): {detail}")
    return result


def kubectl(*args: str, input_text: str | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    return run(["kubectl", *args], input_text=input_text, check=check)


def apply(objects: list[dict[str, Any]]) -> None:
    body = "\n---\n".join(yaml.safe_dump(item, sort_keys=False) for item in objects)
    kubectl("apply", "-f", "-", input_text=body)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_inputs(image: str, candidate: str, run_id: str, node: str) -> tuple[str, str]:
    if not image or image.endswith(":latest") or "@sha256:" in image:
        raise CanaryError("--image must be a non-latest local candidate tag; image identity is checked separately")
    if not re.fullmatch(r"[0-9a-f]{64}", candidate):
        raise CanaryError("--candidate-sha256 must be lowercase SHA-256")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise CanaryError("--run-id must be a canonical lowercase UUID")
    if not re.fullmatch(r"[a-zA-Z0-9.-]+", node):
        raise CanaryError("invalid Kubernetes node name")
    contract = ROOT / "contracts/events/campaign-rail-correlation.v1.json"
    if sha256(contract) != CONTRACT_SHA256:
        raise CanaryError("campaign rail correlation contract hash drifted")
    for _, path in INIT_FILES:
        if not path.is_file():
            raise CanaryError(f"missing canary migration input: {path.relative_to(ROOT)}")
    suffix = run_id.replace("-", "")[:8]
    return suffix, f"canary-m07-{run_id.replace('-', '')[:12]}"


def labels(run_id: str) -> dict[str, str]:
    return {"app.kubernetes.io/name": "m07-campaign-rail-canary", "traffic.analysis/canary-run": run_id}


def postgres_objects(names: dict[str, str], password: str, run_id: str, candidate: str, node: str) -> list[dict[str, Any]]:
    data = {name: path.read_text(encoding="utf-8") for name, path in INIT_FILES}
    data["90-canary-sentinel.sql"] = f"""
CREATE TABLE campaign_rail_canary_sentinel (
  run_id UUID PRIMARY KEY,
  candidate_sha256 TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{{64}}$'),
  expires_at TIMESTAMPTZ NOT NULL
);
INSERT INTO campaign_rail_canary_sentinel(run_id,candidate_sha256,expires_at)
VALUES ('{run_id}'::uuid,'{candidate}',now()+interval '2 hours');
""".strip() + "\n"
    common_labels = labels(run_id)
    return [
        {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": names["init"], "namespace": DB_NAMESPACE, "labels": common_labels}, "data": data},
        {"apiVersion": "v1", "kind": "Secret", "metadata": {"name": names["secret"], "namespace": DB_NAMESPACE, "labels": common_labels},
         "type": "Opaque", "stringData": {"password": password}},
        {"apiVersion": "v1", "kind": "Secret", "metadata": {"name": names["secret"], "namespace": APP_NAMESPACE, "labels": common_labels},
         "type": "Opaque", "stringData": {"password": password}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": names["postgres"], "namespace": DB_NAMESPACE, "labels": common_labels},
         "spec": {"selector": {"traffic.analysis/canary-postgres": names["postgres"]}, "ports": [{"name": "postgres", "port": 5432, "targetPort": 5432}]}},
        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": names["postgres"], "namespace": DB_NAMESPACE, "labels": {**common_labels, "traffic.analysis/canary-postgres": names["postgres"]}},
         "spec": {"nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                  "containers": [{"name": "postgres", "image": "docker.io/library/postgres:16-alpine", "imagePullPolicy": "IfNotPresent",
                                  "env": [{"name": "POSTGRES_DB", "value": "traffic_platform"}, {"name": "POSTGRES_USER", "value": "postgres"},
                                          {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": names["secret"], "key": "password"}}}],
                                  "ports": [{"containerPort": 5432}],
                                  "readinessProbe": {"exec": {"command": ["sh", "-ec", "pg_isready -h 127.0.0.1 -U postgres -d traffic_platform"]}, "periodSeconds": 2, "failureThreshold": 90},
                                  "resources": {"requests": {"cpu": "100m", "memory": "192Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                                  "volumeMounts": [{"name": "data", "mountPath": "/var/lib/postgresql/data"}, {"name": "init", "mountPath": "/docker-entrypoint-initdb.d", "readOnly": True}]}],
                  "volumes": [{"name": "data", "emptyDir": {}}, {"name": "init", "configMap": {"name": names["init"]}}]}}
    ]


def base_env(stage: str, candidate: str, run_id: str, tenant: str) -> list[dict[str, Any]]:
    return [
        {"name": "CAMPAIGN_RAIL_CANARY_STAGE", "value": stage},
        {"name": "CAMPAIGN_RAIL_CANARY_RUN_ID", "value": run_id},
        {"name": "CAMPAIGN_RAIL_CANARY_CANDIDATE_SHA256", "value": candidate},
        {"name": "CAMPAIGN_RAIL_CANARY_TENANT_ID", "value": tenant},
        {"name": "AUTH_ENABLED", "value": "false"},
        {"name": "API_LISTEN_ADDR", "value": ":8082"},
        {"name": "KAFKA_BROKERS", "value": "kafka-bootstrap.middleware.svc:9092"},
        {"name": "KAFKA_SECURITY_PROTOCOL", "value": "SASL_SSL"},
        {"name": "KAFKA_SASL_MECHANISM", "value": "SCRAM-SHA-512"},
        {"name": "KAFKA_TLS_CA_FILE", "value": "/etc/kafka/tls/ca.crt"},
        {"name": "KAFKA_TLS_SERVER_NAME", "value": "kafka-bootstrap.middleware.svc"},
        {"name": "KAFKA_CAMPAIGNS_PROTO_TOPIC", "value": "campaigns.v1"},
        {"name": "KAFKA_CAMPAIGNS_PROTO_GROUP", "value": CANONICAL_GROUPS["proto"]},
        {"name": "KAFKA_CAMPAIGN_EVENT_TOPIC", "value": "campaign.events.v2"},
        {"name": "KAFKA_CAMPAIGN_EVENT_GROUP", "value": CANONICAL_GROUPS["aggregate"]},
        {"name": "KAFKA_CAMPAIGN_MEMBERSHIP_TOPIC", "value": "campaign.membership.events.v2"},
        {"name": "KAFKA_CAMPAIGN_MEMBERSHIP_GROUP", "value": CANONICAL_GROUPS["membership"]},
        {"name": "CAMPAIGNS_PROTO_CONSUMER_V1_ENABLED", "value": "false"},
        {"name": "CAMPAIGN_EVENT_CONSUMER_V2_ENABLED", "value": "false"},
        {"name": "CAMPAIGN_EVENT_DISPATCHER_V2_ENABLED", "value": "false"},
        {"name": "CAMPAIGN_RAIL_CORRELATION_V1_ENABLED", "value": "false"},
        {"name": "CAMPAIGNS_PROTO_CANDIDATE_SHA256", "value": candidate},
        {"name": "CAMPAIGN_JSON_V2_CANDIDATE_SHA256", "value": candidate},
        {"name": "CAMPAIGN_RAIL_CORRELATION_CONTRACT_SHA256", "value": CONTRACT_SHA256},
        {"name": "LOG_LEVEL", "value": "info"},
    ]


def credential_env(secret_name: str) -> list[dict[str, Any]]:
    return [
        {"name": "KAFKA_SASL_USERNAME", "valueFrom": {"secretKeyRef": {"name": secret_name, "key": "username"}}},
        {"name": "KAFKA_SASL_PASSWORD", "valueFrom": {"secretKeyRef": {"name": secret_name, "key": "password"}}},
    ]


def database_env(names: dict[str, str]) -> list[dict[str, Any]]:
    return [
        {"name": "CAMPAIGN_RAIL_CANARY_ISOLATED_DATABASE", "value": "true"},
        {"name": "AUTH_POSTGRES_HOST", "value": f"{names['postgres']}.{DB_NAMESPACE}.svc"},
        {"name": "AUTH_POSTGRES_PORT", "value": "5432"},
        {"name": "AUTH_POSTGRES_DATABASE", "value": "traffic_platform"},
        {"name": "AUTH_POSTGRES_USERNAME", "value": "postgres"},
        {"name": "AUTH_POSTGRES_SSL_MODE", "value": "disable"},
        {"name": "AUTH_POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": names["secret"], "key": "password"}}},
    ]


def enable_stage_switch(env: list[dict[str, Any]], stage: str) -> None:
    mapping = {
        "proto-consumer": "CAMPAIGNS_PROTO_CONSUMER_V1_ENABLED",
        "json-consumer": "CAMPAIGN_EVENT_CONSUMER_V2_ENABLED",
        "json-dispatcher": "CAMPAIGN_EVENT_DISPATCHER_V2_ENABLED",
        "correlation": "CAMPAIGN_RAIL_CORRELATION_V1_ENABLED",
    }
    target = mapping[stage]
    matched = [item for item in env if item.get("name") == target]
    if len(matched) != 1:
        raise CanaryError(f"stage switch {target} is not unique in the generated environment")
    matched[0]["value"] = "true"


def pod_container(image: str, env: list[dict[str, Any]], *, readiness: bool) -> dict[str, Any]:
    body: dict[str, Any] = {
        "name": "canary", "image": image, "imagePullPolicy": "Never",
        "securityContext": {"runAsNonRoot": True, "runAsUser": 1000, "runAsGroup": 1000, "allowPrivilegeEscalation": False,
                            "capabilities": {"drop": ["ALL"]}, "seccompProfile": {"type": "RuntimeDefault"}},
        "env": env,
        "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "500m", "memory": "256Mi"}},
        "volumeMounts": [{"name": "kafka-tls", "mountPath": "/etc/kafka/tls", "readOnly": True}],
    }
    if readiness:
        body["ports"] = [{"name": "http", "containerPort": 8082}]
        body["livenessProbe"] = {"httpGet": {"path": "/health", "port": "http"}, "periodSeconds": 5, "failureThreshold": 6}
        body["readinessProbe"] = {"httpGet": {"path": "/health/ready", "port": "http"}, "periodSeconds": 2, "failureThreshold": 120}
    return body


def stage_deployment(name: str, stage: str, image: str, candidate: str, run_id: str, tenant: str, node: str, names: dict[str, str]) -> dict[str, Any]:
    env = base_env(stage, candidate, run_id, tenant) + credential_env("kafka-alert-service-credentials") + database_env(names)
    enable_stage_switch(env, stage)
    selector = {"traffic.analysis/canary-stage": name}
    return {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": name, "namespace": APP_NAMESPACE, "labels": labels(run_id)},
            "spec": {"replicas": 1, "selector": {"matchLabels": selector}, "template": {"metadata": {"labels": {**labels(run_id), **selector},
                      "annotations": {"traffic.analysis/candidate-sha256": candidate}},
                      "spec": {"nodeName": node, "serviceAccountName": "alert-service", "automountServiceAccountToken": False,
                               "terminationGracePeriodSeconds": 20, "containers": [pod_container(image, env, readiness=True)],
                               "volumes": [{"name": "kafka-tls", "secret": {"secretName": "kafka-client-tls"}}]}}}}


def fixture_job(name: str, stage: str, namespace: str, credential: str, image: str, candidate: str, run_id: str, tenant: str, node: str, names: dict[str, str], needs_db: bool) -> dict[str, Any]:
    env = base_env(stage, candidate, run_id, tenant) + credential_env(credential)
    if needs_db:
        env += database_env(names)
    return {"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": name, "namespace": namespace, "labels": labels(run_id)},
            "spec": {"backoffLimit": 0, "template": {"metadata": {"labels": labels(run_id), "annotations": {"traffic.analysis/candidate-sha256": candidate}},
                     "spec": {"nodeName": node, "automountServiceAccountToken": False, "restartPolicy": "Never",
                              "containers": [pod_container(image, env, readiness=False)],
                              "volumes": [{"name": "kafka-tls", "secret": {"secretName": "kafka-client-tls"}}]}}}}


def wait_pod_running(namespace: str, selector: str, timeout: int = 180) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        result = kubectl("get", "pod", "-n", namespace, "-l", selector, "-o", "json", check=False)
        if result.returncode == 0:
            pods = json.loads(result.stdout).get("items", [])
            if pods:
                phase = pods[0].get("status", {}).get("phase")
                if phase == "Running":
                    return
                if phase in {"Failed", "Succeeded"}:
                    raise CanaryError(f"canary pod entered unexpected phase {phase}")
        time.sleep(2)
    raise CanaryError(f"timed out waiting for running pod: {namespace}/{selector}")


def wait_consumer_group_active(group: str, expected_members: int = 1, timeout: int = 120) -> None:
    if group not in CANONICAL_GROUPS.values() or expected_members != 1:
        raise CanaryError("refusing an unbound consumer-group readiness query")
    script = r'''set -euo pipefail
group=$1
expected=$2
kafka_home=${KAFKA_HOME:-/opt/kafka}
client_config=$(mktemp)
trap 'rm -f "$client_config"' EXIT HUP INT TERM
umask 077
cat > "$client_config" <<EOF
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="${KAFKA_INTER_BROKER_USERNAME}" password="${KAFKA_INTER_BROKER_PASSWORD}";
ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12
ssl.truststore.type=PKCS12
ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}
EOF
state=$("$kafka_home/bin/kafka-consumer-groups.sh" --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
  --command-config "$client_config" --describe --group "$group" --state 2>/dev/null || true)
if printf '%s\n' "$state" | awk -v group="$group" -v expected="$expected" \
  '$1==group && $(NF-1)=="Stable" && $NF+0>=expected {found=1} END {exit(found?0:1)}'; then
  printf '@@GROUP_ACTIVE:%s:%s\n' "$group" "$expected"
fi
'''
    deadline = time.time() + timeout
    marker = f"@@GROUP_ACTIVE:{group}:{expected_members}"
    while time.time() < deadline:
        result = kubectl("exec", "-i", "-n", "middleware", "kafka-0", "--", "bash", "-s", "--",
                         group, str(expected_members), input_text=script, check=False)
        if result.returncode == 0 and marker in result.stdout.splitlines():
            return
        time.sleep(2)
    raise CanaryError(f"consumer group did not expose {expected_members} stable member: {group}")


def wait_job(namespace: str, name: str, timeout: int = 180) -> None:
    result = kubectl("wait", "--for=condition=complete", f"job/{name}", "-n", namespace, f"--timeout={timeout}s", check=False)
    if result.returncode != 0:
        logs = kubectl("logs", f"job/{name}", "-n", namespace, check=False).stdout.strip()
        raise CanaryError(f"job {namespace}/{name} failed or timed out: {logs}")


def wait_deployment(name: str, timeout: int = 180) -> None:
    result = kubectl("wait", "--for=condition=available", f"deployment/{name}", "-n", APP_NAMESPACE, f"--timeout={timeout}s", check=False)
    if result.returncode != 0:
        logs = kubectl("logs", f"deployment/{name}", "-n", APP_NAMESPACE, "--tail=100", check=False).stdout.strip()
        raise CanaryError(f"deployment {name} did not become ready: {logs}")


def psql(names: dict[str, str], sql: str) -> str:
    # Stream SQL over stdin. Passing json.dumps(sql) through `sh -c ... -c`
    # preserves escaped newlines as literal backslashes, and also needlessly
    # creates a second quoting surface for evidence queries.
    command = ["exec", "-i", "-n", DB_NAMESPACE, names["postgres"], "--", "sh", "-ec",
               'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -v ON_ERROR_STOP=1 -U postgres -d traffic_platform -At']
    return kubectl(*command, input_text=sql).stdout.strip()


def wait_result(names: dict[str, str], tenant: str, candidate: str, timeout: int = 180) -> dict[str, Any]:
    query = f"""SELECT json_build_object(
      'ready_receipts',(SELECT count(*) FROM campaign_consumer_readiness_v1 WHERE candidate_sha256='{candidate}' AND state='ready'),
      'proto_inbox',(SELECT count(*) FROM campaign_proto_projection_inbox_v1 WHERE tenant_id='{tenant}' AND state='applied'),
      'json_inbox',(SELECT count(*) FROM campaign_event_projection_inbox WHERE tenant_id='{tenant}'),
      'json_deliveries',(SELECT count(*) FROM campaign_event_projection_deliveries d JOIN campaign_event_projection_inbox i ON i.stream=d.stream AND i.event_id=d.event_id WHERE i.tenant_id='{tenant}'),
      'published_outboxes',((SELECT count(*) FROM campaign_aggregate_outbox WHERE tenant_id='{tenant}' AND published=true)+(SELECT count(*) FROM campaign_alert_link_outbox WHERE tenant_id='{tenant}' AND published=true)),
      'correlated',(SELECT count(*) FROM campaign_rail_correlation_v1 WHERE tenant_id='{tenant}' AND state='correlated' AND confidence=1),
      'reconcile_exact',(SELECT count(*) FROM campaign_rail_reconcile_runs_v1 WHERE tenant_id='{tenant}' AND state='exact' AND missing_count=0 AND extra_count=0)
    );"""
    deadline = time.time() + timeout
    last: dict[str, Any] = {}
    while time.time() < deadline:
        raw = psql(names, query)
        if raw:
            last = json.loads(raw)
            if (last.get("ready_receipts") == 3 and last.get("proto_inbox") == 1 and
                    last.get("json_inbox") == 2 and last.get("json_deliveries", 0) >= 4 and
                    last.get("published_outboxes") == 2 and last.get("correlated") == 1 and
                    last.get("reconcile_exact", 0) >= 1):
                return last
        time.sleep(2)
    raise CanaryError(f"campaign rail result did not converge: {last}")


def resource_names(suffix: str) -> dict[str, str]:
    prefix = f"m07-campaign-rail-{suffix}"
    return {"init": prefix + "-init", "secret": prefix + "-pg", "postgres": prefix + "-pg",
            "proto": prefix + "-proto", "proto_fixture": prefix + "-proto-fixture",
            "json": prefix + "-json", "json_projection": prefix + "-json-projection",
            "json_fixture": prefix + "-json-fixture", "dispatcher": prefix + "-dispatcher",
            "correlation": prefix + "-correlation"}


def ensure_absent(names: dict[str, str]) -> None:
    checks = [(DB_NAMESPACE, "pod", names["postgres"]), (APP_NAMESPACE, "deployment", names["proto"]),
              (APP_NAMESPACE, "deployment", names["json"]), (APP_NAMESPACE, "deployment", names["dispatcher"]),
              (APP_NAMESPACE, "deployment", names["correlation"])]
    for namespace, kind, name in checks:
        if kubectl("get", kind, name, "-n", namespace, check=False).returncode == 0:
            raise CanaryError(f"refusing to reuse existing canary resource {namespace}/{kind}/{name}")


def verify_kafka_preflight() -> None:
    script = r'''set -euo pipefail
kafka_home=${KAFKA_HOME:-/opt/kafka}
bootstrap=kafka-bootstrap.middleware.svc:9092
client_config=$(mktemp)
trap 'rm -f "$client_config"' EXIT HUP INT TERM
case "${KAFKA_INTER_BROKER_USERNAME}${KAFKA_INTER_BROKER_PASSWORD}${KAFKA_TLS_TRUSTSTORE_PASSWORD}" in
  *$'\n'*|*$'\r'*) exit 91 ;;
esac
umask 077
cat > "$client_config" <<EOF
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="${KAFKA_INTER_BROKER_USERNAME}" password="${KAFKA_INTER_BROKER_PASSWORD}";
ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12
ssl.truststore.type=PKCS12
ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}
EOF
require_acl() {
  output=$1 principal=$2 operation=$3
  printf '%s' "$output" | grep -F "principal=${principal}" | grep -Fq "operation=${operation}" || return 1
}
for topic in campaigns.v1 campaign.events.v2 campaign.membership.events.v2; do
  description=$("$kafka_home/bin/kafka-topics.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --describe --topic "$topic")
  printf '%s' "$description" | grep -Eq 'PartitionCount:[[:space:]]*6'
  acl=$("$kafka_home/bin/kafka-acls.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --list --topic "$topic")
  require_acl "$acl" User:traffic-alert-service DESCRIBE
  require_acl "$acl" User:traffic-alert-service READ
  if [ "$topic" = campaigns.v1 ]; then
    require_acl "$acl" User:traffic-flink-cep DESCRIBE
    require_acl "$acl" User:traffic-flink-cep WRITE
  else
    require_acl "$acl" User:traffic-alert-service WRITE
  fi
  printf '@@TOPIC:%s:PASS\n' "$topic"
done
for group in alert-service-campaigns-proto-projection-v1 alert-service-campaign-event-projection-v2 alert-service-campaign-membership-projection-v2; do
  acl=$("$kafka_home/bin/kafka-acls.sh" --bootstrap-server "$bootstrap" --command-config "$client_config" --list --group "$group")
  require_acl "$acl" User:traffic-alert-service DESCRIBE
  require_acl "$acl" User:traffic-alert-service READ
  printf '@@GROUP:%s:PASS\n' "$group"
done
'''
    result = kubectl("exec", "-i", "-n", "middleware", "kafka-0", "--", "bash", "-s", input_text=script, check=False)
    markers = {line.strip() for line in result.stdout.splitlines() if line.startswith("@@")}
    expected = {
        "@@TOPIC:campaigns.v1:PASS",
        "@@TOPIC:campaign.events.v2:PASS",
        "@@TOPIC:campaign.membership.events.v2:PASS",
        "@@GROUP:alert-service-campaigns-proto-projection-v1:PASS",
        "@@GROUP:alert-service-campaign-event-projection-v2:PASS",
        "@@GROUP:alert-service-campaign-membership-projection-v2:PASS",
    }
    if result.returncode != 0 or markers != expected:
        raise CanaryError(f"Kafka topic/ACL preflight failed: returncode={result.returncode} markers={sorted(markers)}")


def cleanup(names: dict[str, str]) -> None:
    for namespace, kind, name in [
        (APP_NAMESPACE, "deployment", names["correlation"]), (APP_NAMESPACE, "deployment", names["dispatcher"]),
        (APP_NAMESPACE, "deployment", names["json"]), (APP_NAMESPACE, "deployment", names["proto"]),
        (APP_NAMESPACE, "job", names["json_fixture"]), (APP_NAMESPACE, "job", names["json_projection"]),
        (FLINK_NAMESPACE, "job", names["proto_fixture"]), (DB_NAMESPACE, "pod", names["postgres"]),
        (DB_NAMESPACE, "service", names["postgres"]), (DB_NAMESPACE, "configmap", names["init"]),
        (DB_NAMESPACE, "secret", names["secret"]), (APP_NAMESPACE, "secret", names["secret"]),
    ]:
        kubectl("delete", kind, name, "-n", namespace, "--ignore-not-found=true", "--wait=true", check=False)


def execute(args: argparse.Namespace, suffix: str, tenant: str) -> dict[str, Any]:
    names = resource_names(suffix)
    password = secrets.token_urlsafe(32)
    ensure_absent(names)
    verify_kafka_preflight()
    try:
        apply(postgres_objects(names, password, args.run_id, args.candidate_sha256, args.node))
        kubectl("wait", "--for=condition=ready", f"pod/{names['postgres']}", "-n", DB_NAMESPACE, "--timeout=300s")

        apply([stage_deployment(names["proto"], "proto-consumer", args.image, args.candidate_sha256, args.run_id, tenant, args.node, names)])
        wait_pod_running(APP_NAMESPACE, f"traffic.analysis/canary-stage={names['proto']}")
        wait_consumer_group_active(CANONICAL_GROUPS["proto"])
        apply([fixture_job(names["proto_fixture"], "proto-fixture", FLINK_NAMESPACE, "kafka-flink-cep-job-credentials",
                           args.image, args.candidate_sha256, args.run_id, tenant, args.node, names, False)])
        wait_job(FLINK_NAMESPACE, names["proto_fixture"])
        wait_deployment(names["proto"])

        apply([stage_deployment(names["json"], "json-consumer", args.image, args.candidate_sha256, args.run_id, tenant, args.node, names)])
        wait_pod_running(APP_NAMESPACE, f"traffic.analysis/canary-stage={names['json']}")
        wait_consumer_group_active(CANONICAL_GROUPS["aggregate"])
        wait_consumer_group_active(CANONICAL_GROUPS["membership"])

        # The projection inbox has a tenant FK and the consumers deliberately do
        # not commit an offset when that authority boundary rejects a message.
        # Seed the run-scoped tenant and outboxes before making either JSON event
        # visible on Kafka; otherwise a correct consumer becomes a poison-message
        # retry loop and the readiness gate can never be satisfied.
        apply([fixture_job(names["json_fixture"], "json-fixture", APP_NAMESPACE, "kafka-alert-service-credentials",
                           args.image, args.candidate_sha256, args.run_id, tenant, args.node, names, True)])
        wait_job(APP_NAMESPACE, names["json_fixture"])
        apply([fixture_job(names["json_projection"], "json-projection-fixture", APP_NAMESPACE, "kafka-alert-service-credentials",
                           args.image, args.candidate_sha256, args.run_id, tenant, args.node, names, False)])
        wait_job(APP_NAMESPACE, names["json_projection"])
        wait_deployment(names["json"])

        apply([stage_deployment(names["dispatcher"], "json-dispatcher", args.image, args.candidate_sha256, args.run_id, tenant, args.node, names)])
        wait_deployment(names["dispatcher"])

        apply([stage_deployment(names["correlation"], "correlation", args.image, args.candidate_sha256, args.run_id, tenant, args.node, names)])
        wait_deployment(names["correlation"])
        result = wait_result(names, tenant, args.candidate_sha256)
        return {"status": "PASS", "production_applied": False, "infrastructure": "kubernetes",
                "run_id": args.run_id, "tenant_id": tenant, "candidate_sha256": args.candidate_sha256,
                "image": args.image, "node": args.node, "contract_sha256": CONTRACT_SHA256,
                "result": result, "resources_retained": bool(args.keep)}
    finally:
        if not args.keep:
            cleanup(names)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", required=True)
    parser.add_argument("--candidate-sha256", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--node", default="8-2tb")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    try:
        suffix, tenant = validate_inputs(args.image, args.candidate_sha256, args.run_id, args.node)
        if not args.execute:
            print(json.dumps({"status": "VALIDATED_NOT_EXECUTED", "run_id": args.run_id, "tenant_id": tenant,
                              "candidate_sha256": args.candidate_sha256, "image": args.image,
                              "contract_sha256": CONTRACT_SHA256}, sort_keys=True))
            return 0
        result = execute(args, suffix, tenant)
        print(json.dumps(result, sort_keys=True))
        return 0
    except (CanaryError, ValueError) as error:
        print(json.dumps({"status": "FAIL", "error": str(error)}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
