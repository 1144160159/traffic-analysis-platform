#!/usr/bin/env python3
"""Validate and render the versioned least-privilege Kafka ACL plan."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
ACL_CATALOG = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
TOPIC_CATALOG = ROOT / "contracts/events/kafka-topic-catalog.v1.json"
GENERATED_CONFIGMAP = ROOT / "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml"
GENERATED_IDENTITIES = ROOT / "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml"
GENERATED_SCRAM_BOOTSTRAP = ROOT / "deployments/kubernetes/init-jobs/00-kafka-service-principals.yaml"
SAFE_VALUE = re.compile(r"^[A-Za-z0-9._:-]+$")
SAFE_ENV = re.compile(r"^[A-Z][A-Z0-9_]+$")
KINDS = {"service", "consumer", "flink_job", "external_producer", "replayer", "operator"}


def _load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate_documents(acl: dict[str, Any], topics: dict[str, Any]) -> dict[str, Any]:
    errors: list[str] = []
    policy = acl.get("policy") or {}
    if policy.get("resource_pattern_type") != "literal":
        errors.append("ACL resource_pattern_type must be literal")
    if policy.get("forbid_wildcards") is not True:
        errors.append("ACL policy must forbid wildcards")
    forbidden = set(policy.get("forbidden_operations") or [])
    for field in (
        "producer_topic_operations",
        "consumer_topic_operations",
        "consumer_group_operations",
        "producer_cluster_operations",
    ):
        operations = policy.get(field)
        if not isinstance(operations, list) or not operations:
            errors.append(f"ACL policy {field} must be a non-empty array")
        elif forbidden.intersection(operations):
            errors.append(f"ACL policy {field} contains forbidden operations")

    principals: dict[str, dict[str, Any]] = {}
    kafka_principals: set[str] = set()
    credential_secrets: set[tuple[str, str]] = set()
    credential_password_envs: set[str] = set()
    credential_remote_properties: set[str] = set()
    expanded_service_identities = 0
    expanded_workload_identities = 0
    for item in acl.get("principals") or []:
        principal_id = str(item.get("id", ""))
        kafka_principal = str(item.get("principal", ""))
        if not principal_id or principal_id in principals:
            errors.append(f"duplicate or empty principal id: {principal_id!r}")
            continue
        principals[principal_id] = item
        if item.get("kind") not in KINDS:
            errors.append(f"principal {principal_id} has invalid kind")
        if not kafka_principal.startswith("User:") or not SAFE_VALUE.fullmatch(kafka_principal):
            errors.append(f"principal {principal_id} has unsafe Kafka principal")
        if kafka_principal in kafka_principals:
            errors.append(f"duplicate Kafka principal: {kafka_principal}")
        kafka_principals.add(kafka_principal)
        credential = item.get("credential")
        requires_workload_credential = item.get("kind") == "service" or credential is not None
        if requires_workload_credential:
            if item.get("rollout_state") != "expand" or not isinstance(credential, dict):
                errors.append(f"workload principal {principal_id} requires expand credential metadata")
                continue
            expanded_workload_identities += 1
            if item.get("kind") == "service":
                expanded_service_identities += 1
            namespace = str(credential.get("namespace", ""))
            secret_name = str(credential.get("secret_name", ""))
            password_env = str(credential.get("password_env", ""))
            remote_property = str(credential.get("remote_password_property", ""))
            workload = str(credential.get("workload", ""))
            expected_username = kafka_principal.removeprefix("User:")
            expected_namespace = "flink" if item.get("kind") == "flink_job" else "traffic-analysis"
            if namespace != expected_namespace:
                errors.append(
                    f"workload principal {principal_id} must use {expected_namespace} namespace"
                )
            if not secret_name or not SAFE_VALUE.fullmatch(secret_name):
                errors.append(f"workload principal {principal_id} has unsafe credential secret")
            if not password_env or not SAFE_ENV.fullmatch(password_env):
                errors.append(f"workload principal {principal_id} has unsafe password env")
            if remote_property != password_env:
                errors.append(f"workload principal {principal_id} remote property must match password env")
            if workload != principal_id:
                errors.append(f"workload principal {principal_id} workload must match its id")
            deterministic_username = (
                f"traffic-{principal_id.removesuffix('-job')}"
                if item.get("kind") == "flink_job"
                else f"traffic-{principal_id}"
            )
            if expected_username != deterministic_username:
                errors.append(f"workload principal {principal_id} username is not deterministic")
            secret_identity = (namespace, secret_name)
            if secret_identity in credential_secrets:
                errors.append(f"duplicate workload credential secret: {namespace}/{secret_name}")
            credential_secrets.add(secret_identity)
            if password_env in credential_password_envs:
                errors.append(f"duplicate workload password env: {password_env}")
            credential_password_envs.add(password_env)
            if remote_property in credential_remote_properties:
                errors.append(f"duplicate remote password property: {remote_property}")
            credential_remote_properties.add(remote_property)

    canonical = {str(item.get("name")): item for item in topics.get("topics") or []}
    bindings: dict[str, dict[str, Any]] = {}
    producer_ids: set[str] = set()
    consumer_ids: set[str] = set()
    for binding in acl.get("topic_bindings") or []:
        topic = str(binding.get("topic", ""))
        if topic in bindings or topic not in canonical:
            errors.append(f"duplicate or non-canonical ACL topic binding: {topic!r}")
            continue
        bindings[topic] = binding
        producers = binding.get("producers")
        consumers = binding.get("consumers")
        if not isinstance(producers, list) or not isinstance(consumers, list):
            errors.append(f"topic {topic} producers and consumers must be arrays")
            continue
        if bool(canonical[topic].get("producers")) != bool(producers):
            errors.append(f"topic {topic} producer ACL coverage differs from event catalog")
        if bool(canonical[topic].get("consumers")) != bool(consumers):
            errors.append(f"topic {topic} consumer ACL coverage differs from event catalog")
        for principal_id in producers:
            if principal_id not in principals:
                errors.append(f"topic {topic} references unknown producer {principal_id}")
            else:
                producer_ids.add(principal_id)
        seen_consumers: set[str] = set()
        for consumer in consumers:
            principal_id = str(consumer.get("principal", ""))
            groups = consumer.get("groups")
            if principal_id not in principals:
                errors.append(f"topic {topic} references unknown consumer {principal_id}")
            else:
                consumer_ids.add(principal_id)
            if principal_id in seen_consumers:
                errors.append(f"topic {topic} repeats consumer {principal_id}")
            seen_consumers.add(principal_id)
            if not isinstance(groups, list) or not groups:
                errors.append(f"topic {topic} consumer {principal_id} requires groups")
                continue
            for group in groups:
                if not isinstance(group, str) or not SAFE_VALUE.fullmatch(group) or "*" in group:
                    errors.append(f"topic {topic} consumer {principal_id} has unsafe group {group!r}")

    missing = sorted(set(canonical) - set(bindings))
    extra = sorted(set(bindings) - set(canonical))
    if missing:
        errors.append(f"canonical topics missing ACL bindings: {missing}")
    if extra:
        errors.append(f"ACL bindings absent from canonical topics: {extra}")

    allowed_additional = {str(item.get("name")) for item in topics.get("allowed_additional_topics") or []}
    declared_additional: set[str] = set()
    for item in acl.get("additional_topic_bindings") or []:
        name = str(item.get("topic", ""))
        if name in declared_additional:
            errors.append(f"duplicate additional ACL topic binding: {name}")
        declared_additional.add(name)
        if item.get("state") != "blocked" or not str(item.get("reason", "")).strip():
            errors.append(f"additional topic {name} must remain blocked with a reason")
    if declared_additional != allowed_additional:
        errors.append("additional ACL topic bindings must exactly match allowed additional topics")

    replay = acl.get("replay_policy") or {}
    replay_id = str(replay.get("principal", ""))
    if replay_id not in principals or principals.get(replay_id, {}).get("kind") != "replayer":
        errors.append("replay_policy must reference the replayer principal")
    if replay.get("enabled") is not False:
        errors.append("baseline replay principal must be disabled")

    unused = sorted(
        principal_id
        for principal_id, item in principals.items()
        if principal_id not in producer_ids | consumer_ids and item.get("kind") not in {"replayer", "operator"}
    )
    if unused:
        errors.append(f"non-governance principals have no grants: {unused}")

    return {
        "schema_version": 1,
        "result": "pass" if not errors else "blocked",
        "counts": {
            "principals": len(principals),
            "canonical_topics": len(canonical),
            "topic_bindings": len(bindings),
            "additional_topics_blocked": len(declared_additional),
            "producer_principals": len(producer_ids),
            "consumer_principals": len(consumer_ids),
            "expanded_service_identities": expanded_service_identities,
            "expanded_workload_identities": expanded_workload_identities,
        },
        "errors": errors,
    }


def _grant_lines(acl: dict[str, Any]) -> list[str]:
    principals = {item["id"]: item["principal"] for item in acl["principals"]}
    policy = acl["policy"]
    grants: set[tuple[str, str, str, str]] = set()
    producer_ids: set[str] = set()
    for binding in acl["topic_bindings"]:
        topic = binding["topic"]
        for principal_id in binding["producers"]:
            producer_ids.add(principal_id)
            for operation in policy["producer_topic_operations"]:
                grants.add((principals[principal_id], "topic", topic, operation))
        for consumer in binding["consumers"]:
            principal = principals[consumer["principal"]]
            for operation in policy["consumer_topic_operations"]:
                grants.add((principal, "topic", topic, operation))
            for group in consumer["groups"]:
                for operation in policy["consumer_group_operations"]:
                    grants.add((principal, "group", group, operation))
    for principal_id in producer_ids:
        for operation in policy["producer_cluster_operations"]:
            grants.add((principals[principal_id], "cluster", "kafka-cluster", operation))

    lines: list[str] = []
    for principal, resource_type, resource, operation in sorted(grants):
        resource_flag = "--cluster" if resource_type == "cluster" else f"--{resource_type} {resource}"
        lines.append(
            f'apply_acl --allow-principal {principal} --operation {operation} {resource_flag}'
        )
    return lines


def render_shell(acl: dict[str, Any]) -> str:
    lines = _grant_lines(acl)
    body = "\n".join(lines)
    return f'''#!/usr/bin/env bash
set -euo pipefail

BOOTSTRAP=${{1:?bootstrap server required}}
CLIENT_CONFIG=${{2:?client properties required}}
ACL_BIN=${{3:?kafka-acls.sh path required}}

apply_acl() {{
  "$ACL_BIN" --bootstrap-server "$BOOTSTRAP" --command-config "$CLIENT_CONFIG" \\
    --add --resource-pattern-type literal "$@"
}}

# Generated from contracts/events/kafka-acl-catalog.v1.json. Do not edit.
{body}
echo "Applied {len(lines)} literal least-privilege Kafka ACL grants."
'''


def render_configmap(acl: dict[str, Any]) -> str:
    shell = render_shell(acl)
    digest = hashlib.sha256(shell.encode()).hexdigest()
    indented = "\n".join("    " + line for line in shell.splitlines())
    return f'''# Generated by scripts/alignment/generate_kafka_acl_plan.py. Do not edit.
apiVersion: v1
kind: ConfigMap
metadata:
  name: kafka-acl-plan-v1
  namespace: middleware
  annotations:
    traffic.openai.com/source-sha256: "{digest}"
data:
  apply.sh: |
{indented}
'''


def _workload_principals(acl: dict[str, Any]) -> list[dict[str, Any]]:
    return sorted(
        (
            item
            for item in acl["principals"]
            if item.get("rollout_state") == "expand"
            and isinstance(item.get("credential"), dict)
        ),
        key=lambda item: item["id"],
    )


def render_service_identities(acl: dict[str, Any]) -> str:
    workloads = _workload_principals(acl)
    lines = [
        "# Generated by scripts/alignment/generate_kafka_acl_plan.py. Do not edit.",
        "apiVersion: external-secrets.io/v1",
        "kind: ExternalSecret",
        "metadata:",
        "  name: kafka-principal-credentials",
        "  namespace: middleware",
        "spec:",
        "  refreshInterval: 1h",
        "  secretStoreRef:",
        "    name: traffic-platform-secret-store",
        "    kind: ClusterSecretStore",
        "  target:",
        "    name: kafka-principal-credentials",
        "    creationPolicy: Owner",
        "  data:",
    ]
    for item in workloads:
        credential = item["credential"]
        lines.extend(
            [
                f"  - secretKey: {credential['password_env']}",
                "    remoteRef:",
                "      key: traffic-platform-prod-credentials",
                f"      property: {credential['remote_password_property']}",
            ]
        )
    for item in workloads:
        credential = item["credential"]
        username = item["principal"].removeprefix("User:")
        lines.extend(
            [
                "---",
                "apiVersion: external-secrets.io/v1",
                "kind: ExternalSecret",
                "metadata:",
                f"  name: {credential['secret_name']}",
                f"  namespace: {credential['namespace']}",
                "spec:",
                "  refreshInterval: 1h",
                "  secretStoreRef:",
                "    name: traffic-platform-secret-store",
                "    kind: ClusterSecretStore",
                "  target:",
                f"    name: {credential['secret_name']}",
                "    creationPolicy: Owner",
                "    template:",
                "      engineVersion: v2",
                "      data:",
                f'        username: "{username}"',
                '        password: "{{ .password }}"',
                "  data:",
                "  - secretKey: password",
                "    remoteRef:",
                "      key: traffic-platform-prod-credentials",
                f"      property: {credential['remote_password_property']}",
            ]
        )

    provision_entries = " \\\n".join(
        f'      "{item["principal"].removeprefix("User:")}:{item["credential"]["password_env"]}"'
        for item in workloads
    )
    provision_script = f'''#!/usr/bin/env bash
set -euo pipefail
set +x

KAFKA_HOME=${{KAFKA_HOME:-/opt/kafka}}
CONFIG_BIN="$KAFKA_HOME/bin/kafka-configs.sh"
BROKER_API_BIN="$KAFKA_HOME/bin/kafka-broker-api-versions.sh"
BOOTSTRAP=${{KAFKA_BOOTSTRAP:-kafka-bootstrap.middleware.svc:9092}}
CLIENT_CONFIG=/tmp/kafka-admin.properties

safe_java_property() {{
  local value="$1"
  if ! [[ "$value" =~ ^[A-Za-z0-9._@%+=:/-]+$ ]]; then
    return 1
  fi
  printf '%s' "$value"
}}

admin_username="$(safe_java_property "$KAFKA_ADMIN_USERNAME")"
admin_password="$(safe_java_property "$KAFKA_ADMIN_PASSWORD")"
truststore_password="$(safe_java_property "$KAFKA_TLS_TRUSTSTORE_PASSWORD")"
cat >"$CLIENT_CONFIG" <<EOF
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="$admin_username" password="$admin_password";
ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12
ssl.truststore.type=PKCS12
ssl.truststore.password=$truststore_password
EOF
chmod 0600 "$CLIENT_CONFIG"

for attempt in $(seq 1 60); do
  if "$BROKER_API_BIN" --bootstrap-server "$BOOTSTRAP" --command-config "$CLIENT_CONFIG" >/dev/null 2>&1; then
    break
  fi
  if [ "$attempt" -eq 60 ]; then
    echo "Kafka did not become ready for SCRAM principal provisioning" >&2
    exit 1
  fi
  sleep 5
done

for entry in \\
{provision_entries}; do
  username="${{entry%%:*}}"
  password_env="${{entry#*:}}"
  password="${{!password_env:-}}"
  case "$username" in traffic-*) ;; *) echo "unsafe Kafka username" >&2; exit 1;; esac
  if ! [[ "$password" =~ ^[A-Za-z0-9._@%+=:/-]{{32,}}$ ]]; then
    echo "missing or unsafe password for $username" >&2
    exit 1
  fi
  "$CONFIG_BIN" --bootstrap-server "$BOOTSTRAP" --command-config "$CLIENT_CONFIG" \\
    --alter --add-config "SCRAM-SHA-512=[password=$password]" \\
    --entity-type users --entity-name "$username" >/dev/null
  echo "Provisioned SCRAM principal $username"
done
'''
    indented_script = "\n".join("    " + line for line in provision_script.splitlines())
    lines.extend(
        [
            "---",
            "apiVersion: v1",
            "kind: ConfigMap",
            "metadata:",
            "  name: kafka-scram-principal-plan-v1",
            "  namespace: middleware",
            "data:",
            "  provision.sh: |",
            indented_script,
            "---",
            "apiVersion: batch/v1",
            "kind: Job",
            "metadata:",
            "  name: init-kafka-principals",
            "  namespace: middleware",
            "spec:",
            "  backoffLimit: 6",
            "  ttlSecondsAfterFinished: 300",
            "  template:",
            "    spec:",
            "      restartPolicy: OnFailure",
            "      containers:",
            "      - name: provision-principals",
            "        image: docker.io/apache/kafka@sha256:32217f809d3fba75ee46e93b1fdbea4aacc0821139443efb73691a913948c31a",
            "        command: [/opt/traffic/kafka-identities/provision.sh]",
            "        envFrom:",
            "        - secretRef: {name: kafka-principal-credentials}",
            "        env:",
            "        - name: KAFKA_ADMIN_USERNAME",
            "          valueFrom: {secretKeyRef: {name: traffic-credentials, key: KAFKA_INTER_BROKER_USERNAME}}",
            "        - name: KAFKA_ADMIN_PASSWORD",
            "          valueFrom: {secretKeyRef: {name: traffic-credentials, key: KAFKA_INTER_BROKER_PASSWORD}}",
            "        - name: KAFKA_TLS_TRUSTSTORE_PASSWORD",
            "          valueFrom: {secretKeyRef: {name: traffic-credentials, key: KAFKA_TLS_TRUSTSTORE_PASSWORD}}",
            "        volumeMounts:",
            "        - {name: kafka-identity-plan, mountPath: /opt/traffic/kafka-identities, readOnly: true}",
            "        - {name: kafka-client-tls, mountPath: /etc/kafka/tls, readOnly: true}",
            "      volumes:",
            "      - name: kafka-identity-plan",
            "        configMap: {name: kafka-scram-principal-plan-v1, defaultMode: 0555}",
            "      - name: kafka-client-tls",
            "        secret: {secretName: kafka-client-tls, optional: false}",
        ]
    )
    return "\n".join(lines) + "\n"


def split_service_identity_bundle(bundle: str) -> tuple[str, str]:
    marker = "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: kafka-scram-principal-plan-v1\n"
    if marker not in bundle:
        raise ValueError("Kafka service identity bundle lacks the SCRAM ConfigMap marker")
    external_secrets, bootstrap_tail = bundle.split(marker, maxsplit=1)
    return external_secrets.rstrip() + "\n", marker + bootstrap_tail


def validate() -> dict[str, Any]:
    return validate_documents(_load(ACL_CATALOG), _load(TOPIC_CATALOG))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--format", choices=("check", "shell", "configmap", "identities"), default="check")
    parser.add_argument("--check-generated", action="store_true")
    parser.add_argument("--write-generated", action="store_true")
    args = parser.parse_args()
    acl = _load(ACL_CATALOG)
    topics = _load(TOPIC_CATALOG)
    result = validate_documents(acl, topics)
    if result["result"] != "pass":
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 1
    rendered = render_configmap(acl)
    identity_bundle = render_service_identities(acl)
    identities, scram_bootstrap = split_service_identity_bundle(identity_bundle)
    if args.write_generated:
        GENERATED_CONFIGMAP.parent.mkdir(parents=True, exist_ok=True)
        GENERATED_CONFIGMAP.write_text(rendered, encoding="utf-8")
        GENERATED_IDENTITIES.parent.mkdir(parents=True, exist_ok=True)
        GENERATED_IDENTITIES.write_text(identities, encoding="utf-8")
        GENERATED_SCRAM_BOOTSTRAP.parent.mkdir(parents=True, exist_ok=True)
        GENERATED_SCRAM_BOOTSTRAP.write_text(scram_bootstrap, encoding="utf-8")
    if args.check_generated:
        if not GENERATED_CONFIGMAP.is_file() or GENERATED_CONFIGMAP.read_text(encoding="utf-8") != rendered:
            result["result"] = "blocked"
            result["errors"] = ["generated Kafka ACL ConfigMap is stale"]
            print(json.dumps(result, ensure_ascii=False, indent=2))
            return 1
        if not GENERATED_IDENTITIES.is_file() or GENERATED_IDENTITIES.read_text(encoding="utf-8") != identities:
            result["result"] = "blocked"
            result["errors"] = ["generated Kafka service identity manifest is stale"]
            print(json.dumps(result, ensure_ascii=False, indent=2))
            return 1
        if not GENERATED_SCRAM_BOOTSTRAP.is_file() or GENERATED_SCRAM_BOOTSTRAP.read_text(encoding="utf-8") != scram_bootstrap:
            result["result"] = "blocked"
            result["errors"] = ["generated Kafka SCRAM bootstrap manifest is stale"]
            print(json.dumps(result, ensure_ascii=False, indent=2))
            return 1
    if args.format == "shell":
        print(render_shell(acl), end="")
    elif args.format == "configmap":
        print(rendered, end="")
    elif args.format == "identities":
        print(identity_bundle, end="")
    else:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
