#!/usr/bin/env python3
"""Build the redacted, consumer-scoped T-CONFIG-001 configuration catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any, Iterable

import yaml


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/configuration/configuration-catalog.v1.json"

GO_ROOTS = (
    ROOT / "go/control-plane/cmd",
    ROOT / "go/control-plane/internal",
)
FLINK_PROPERTIES = ROOT / "java/flink-jobs"
KUBERNETES_GLOBS = (
    "deployments/kubernetes/applications/*.yaml",
    "deployments/kubernetes/flink/*.yaml",
    "deployments/kubernetes/infrastructure/*.yaml",
    "deployments/kubernetes/init-jobs/*.yaml",
    "deployments/kubernetes/observability/*.yaml",
    "deployments/kubernetes/argo-events/mlops-training-template.yaml",
    "go/control-plane/deployments/kubernetes/*.yaml",
)
SHARED_AUTHORITIES = (
    {
        "domain": "kafka_topics",
        "owner": "kafka-platform-owner",
        "path": "contracts/events/kafka-topic-catalog.v1.json",
        "rendered_consumers": [
            "common/kafka/create-topics.sh",
            "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
        ],
    },
    {
        "domain": "kafka_acl",
        "owner": "kafka-platform-owner",
        "path": "contracts/events/kafka-acl-catalog.v1.json",
        "rendered_consumers": [
            "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
        ],
    },
    {
        "domain": "flink_job_topology",
        "owner": "flink-data-owner",
        "path": "contracts/flink/job-registry.v1.json",
        "rendered_consumers": ["deployments/kubernetes/flink"],
    },
    {
        "domain": "clickhouse_schema",
        "owner": "clickhouse-data-owner",
        "path": "contracts/clickhouse/schema-authority.v1.json",
        "rendered_consumers": [
            "common/sql/ch/00-all-tables.sql",
            "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml",
        ],
    },
    {
        "domain": "minio_lifecycle",
        "owner": "minio-platform-owner",
        "path": "deployments/kubernetes/init-jobs/06-minio-lifecycle.yaml",
        "rendered_consumers": ["minio_runtime"],
    },
    {
        "domain": "iam_scopes",
        "owner": "security-platform-owner",
        "path": "go/control-plane/internal/auth/model/scopes.go",
        "rendered_consumers": ["web/ui/src/config/permissions.ts"],
    },
    {
        "domain": "apisix_routes",
        "owner": "security-platform-owner",
        "path": "deployments/kubernetes/configmaps/apisix-routes.yaml",
        "rendered_consumers": ["apisix_standalone_runtime"],
    },
)

GO_FIELD = re.compile(
    r"(?P<field>[A-Za-z][A-Za-z0-9_]*)\s+(?P<type>[^`]+?)\s+`(?P<tags>[^`]*)`"
)
ENV_TAG = re.compile(r'env:"(?P<name>[A-Z0-9_]+)"')
DEFAULT_TAG = re.compile(r'envDefault:"(?P<value>[^"]*)"')
SEPARATOR_TAG = re.compile(r'envSeparator:"(?P<value>[^"]*)"')
PROPERTY = re.compile(r"^(?P<name>[A-Za-z0-9_.-]+)\s*[=:]\s*(?P<value>.*)$")
ENV_REFERENCE = re.compile(r"^\$\{(?P<name>[A-Z0-9_]+)(?::(?P<fallback>.*))?\}$")
SECRET_NAME = re.compile(
    r"(?:PASSWORD|PASSWD|SECRET|TOKEN|PRIVATE_KEY|SIGNING_KEY|API_KEY|ACCESS_KEY|"
    r"SECRET_KEY|CREDENTIAL|WEBHOOK|WEBHOOK_URL)$",
    re.IGNORECASE,
)
NON_SECRET_CONTROL_NAME = re.compile(
    r"_(?:ENABLED|DISABLED|TTL|INTERVAL|TIMEOUT|COUNT|LIMIT|PREFIX|METHOD|TYPE|VERSION|"
    r"TOPIC|GROUP|SIZE|PATH)$",
    re.IGNORECASE,
)

OWNER_BY_DOMAIN = {
    "alert": "alert-domain-owner",
    "asset": "asset-domain-owner",
    "auth": "security-platform-owner",
    "forensics": "forensics-domain-owner",
    "graph": "graph-domain-owner",
    "ingest": "probe-domain-owner",
    "rules": "rule-deployment-domain-owner",
}
OWNER_BY_INFRASTRUCTURE = {
    "01-kafka.yaml": "kafka-platform-owner",
    "02-clickhouse.yaml": "clickhouse-data-owner",
    "03-postgresql.yaml": "postgresql-data-owner",
    "04-redis.yaml": "redis-platform-owner",
    "05-opensearch.yaml": "opensearch-data-owner",
    "06-minio.yaml": "minio-platform-owner",
    "07-flink.yaml": "flink-data-owner",
    "08-gateway.yaml": "security-platform-owner",
    "09-nebula-graph.yaml": "graph-domain-owner",
    "10-observability.yaml": "observability-data-quality-owner",
}


def _relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return _sha256(payload)


def _go_type(value: str) -> str:
    value = value.strip()
    return {
        "bool": "boolean",
        "int": "integer",
        "int32": "integer",
        "int64": "integer",
        "uint": "integer",
        "uint16": "integer",
        "uint32": "integer",
        "uint64": "integer",
        "float32": "number",
        "float64": "number",
        "time.Duration": "duration",
        "[]string": "string_list",
        "string": "string",
    }.get(value, f"go:{value}")


def _go_consumer(path: Path) -> tuple[str, str]:
    parts = path.relative_to(ROOT / "go/control-plane").parts
    if parts[0] == "cmd" and len(parts) > 1:
        return parts[1], "platform-contract-owner"
    if parts[0] == "internal" and len(parts) > 1:
        domain = parts[1]
        return domain, OWNER_BY_DOMAIN.get(domain, "platform-contract-owner")
    return "shared", "platform-contract-owner"


def _secret(name: str, source_kind: str = "") -> bool:
    if source_kind == "secretKeyRef":
        return True
    if name.upper().startswith("ALLOW_NO_"):
        return False
    return bool(SECRET_NAME.search(name)) and not bool(NON_SECRET_CONTROL_NAME.search(name))


def _entry_base(
    *,
    entry_id: str,
    key: str,
    consumer: str,
    owner: str,
    value_type: str,
    required: bool,
    secret: bool,
) -> dict[str, Any]:
    return {
        "id": entry_id,
        "runtime_binding_id": entry_id,
        "key": key,
        "consumer": consumer,
        "owner": owner,
        "type": value_type,
        "default": None,
        "default_present": False,
        "secret_default_nonempty": False,
        "required": required,
        "secret": secret,
        "legal_range": {
            "mode": "consumer_parser",
            "rule": "the referenced consumer must reject values outside its parser or validator",
        },
        "hot_reload": "unsupported",
        "restart_required": True,
        "environment_override": True,
        "deprecation": {"status": "active", "replacement": None},
        "sources": [],
    }


def _go_entries() -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for root in GO_ROOTS:
        for path in sorted(root.rglob("*.go")):
            if path.name.endswith("_test.go"):
                continue
            consumer, owner = _go_consumer(path)
            for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                match = GO_FIELD.search(line)
                if not match:
                    continue
                env = ENV_TAG.search(match.group("tags"))
                if not env:
                    continue
                name = env.group("name")
                default_match = DEFAULT_TAG.search(match.group("tags"))
                separator_match = SEPARATOR_TAG.search(match.group("tags"))
                secret = _secret(name)
                entry = _entry_base(
                    entry_id=(
                        f"go:{_relative(path)}:{line_number}:{match.group('field')}:{name}"
                    ),
                    key=name,
                    consumer=f"go:{consumer}",
                    owner=owner,
                    value_type=_go_type(match.group("type")),
                    required=default_match is None,
                    secret=secret,
                )
                if default_match is not None and not secret:
                    entry["default"] = default_match.group("value")
                entry["default_present"] = default_match is not None
                entry["secret_default_nonempty"] = bool(
                    secret and default_match is not None and default_match.group("value")
                )
                entry["separator"] = separator_match.group("value") if separator_match else None
                entry["sources"] = [
                    {
                        "kind": "go_env_tag",
                        "path": _relative(path),
                        "line": line_number,
                        "field": match.group("field"),
                    }
                ]
                entries.append(entry)
    return entries


def _flink_entries() -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    pattern = "*/src/main/resources/*.properties"
    for path in sorted(FLINK_PROPERTIES.glob(pattern)):
        module = path.parts[-5]
        for line_number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            line = raw.strip()
            if not line or line.startswith(("#", "!")):
                continue
            match = PROPERTY.match(line)
            if not match:
                continue
            name = match.group("name")
            value = match.group("value").strip()
            secret = _secret(name)
            env_reference = ENV_REFERENCE.match(value)
            entry = _entry_base(
                entry_id=f"flink:{module}:{name}",
                key=name,
                consumer=f"flink:{module}",
                owner="flink-data-owner",
                value_type="string",
                required=value == "",
                secret=secret,
            )
            if value and not secret:
                entry["default"] = value
            entry["default_present"] = bool(value and not env_reference)
            entry["secret_default_nonempty"] = bool(secret and value and not env_reference)
            entry["environment_override"] = True
            entry["sources"] = [
                {
                    "kind": "flink_env_reference" if env_reference else "flink_properties",
                    "path": _relative(path),
                    "line": line_number,
                    "reference": (
                        {
                            "environment_variable": env_reference.group("name"),
                            "fallback_present": env_reference.group("fallback") is not None,
                            "fallback_empty": env_reference.group("fallback") == "",
                        }
                        if env_reference
                        else {}
                    ),
                }
            ]
            entries.append(entry)
    return entries


def _pod_specs(document: dict[str, Any]) -> Iterable[tuple[str, dict[str, Any]]]:
    kind = str(document.get("kind", ""))
    spec = document.get("spec") or {}
    if kind in {"Deployment", "StatefulSet", "DaemonSet", "Job", "ReplicaSet"}:
        template = spec.get("template") or {}
        yield kind, template.get("spec") or {}
    elif kind == "CronJob":
        template = (((spec.get("jobTemplate") or {}).get("spec") or {}).get("template") or {})
        yield kind, template.get("spec") or {}


def _kubernetes_owner(path: Path, name: str) -> str:
    if path.parent.name == "infrastructure":
        return OWNER_BY_INFRASTRUCTURE.get(path.name, "security-platform-owner")
    lowered = name.lower()
    for domain, owner in OWNER_BY_DOMAIN.items():
        if domain in lowered:
            return owner
    if "flink" in path.parts or "flink" in lowered:
        return "flink-data-owner"
    return "platform-contract-owner"


def _kubernetes_entries() -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    entries: list[dict[str, Any]] = []
    parse_errors: list[dict[str, Any]] = []
    paths = sorted({path for glob in KUBERNETES_GLOBS for path in ROOT.glob(glob)})
    for path in paths:
        try:
            documents = [item for item in yaml.safe_load_all(path.read_text(encoding="utf-8")) if item]
        except yaml.YAMLError as exc:
            parse_errors.append({"path": _relative(path), "error": str(exc)})
            continue
        for document in documents:
            metadata = document.get("metadata") or {}
            namespace = str(metadata.get("namespace") or "default")
            resource_name = str(metadata.get("name") or "unnamed")
            owner = _kubernetes_owner(path, resource_name)
            for kind, pod_spec in _pod_specs(document):
                containers = list(pod_spec.get("initContainers") or []) + list(
                    pod_spec.get("containers") or []
                )
                for container in containers:
                    container_name = str(container.get("name") or "unnamed")
                    for env in container.get("env") or []:
                        if not isinstance(env, dict) or not env.get("name"):
                            continue
                        name = str(env["name"])
                        value_from = env.get("valueFrom") or {}
                        if "secretKeyRef" in value_from:
                            source_kind = "secretKeyRef"
                            source = value_from["secretKeyRef"] or {}
                            source_identity = {
                                "name": source.get("name"),
                                "key": source.get("key"),
                                "optional": bool(source.get("optional", False)),
                            }
                        elif "configMapKeyRef" in value_from:
                            source_kind = "configMapKeyRef"
                            source = value_from["configMapKeyRef"] or {}
                            source_identity = {
                                "name": source.get("name"),
                                "key": source.get("key"),
                                "optional": bool(source.get("optional", False)),
                            }
                        elif "fieldRef" in value_from:
                            source_kind = "fieldRef"
                            source = value_from["fieldRef"] or {}
                            source_identity = {"fieldPath": source.get("fieldPath")}
                        elif "value" in env:
                            source_kind = "literal"
                            value = str(env.get("value", ""))
                            source_identity = {
                                "value_sha256": _sha256(value),
                                "empty": value == "",
                            }
                        else:
                            source_kind = "unknown"
                            source_identity = {}
                        secret = _secret(name, source_kind)
                        runtime_binding_id = (
                            f"k8s:{namespace}:{kind}:{resource_name}:{container_name}:{name}"
                        )
                        entry = _entry_base(
                            entry_id=f"{runtime_binding_id}:{_relative(path)}",
                            key=name,
                            consumer=f"k8s:{namespace}/{kind}/{resource_name}/{container_name}",
                            owner=owner,
                            value_type="string",
                            required=not bool(source_identity.get("optional", False)),
                            secret=secret,
                        )
                        entry["runtime_binding_id"] = runtime_binding_id
                        entry["environment_override"] = False
                        entry["sources"] = [
                            {
                                "kind": f"kubernetes_{source_kind}",
                                "path": _relative(path),
                                "reference": source_identity,
                            }
                        ]
                        entries.append(entry)
    return entries, parse_errors


def build_catalog() -> dict[str, Any]:
    go_entries = _go_entries()
    flink_entries = _flink_entries()
    kubernetes_entries, parse_errors = _kubernetes_entries()
    entries = sorted(
        go_entries + flink_entries + kubernetes_entries,
        key=lambda item: item["id"],
    )
    runtime_bindings: dict[str, list[dict[str, Any]]] = {}
    for entry in entries:
        runtime_bindings.setdefault(str(entry["runtime_binding_id"]), []).append(entry)
    duplicate_runtime_bindings = {
        binding: [item["id"] for item in declarations]
        for binding, declarations in runtime_bindings.items()
        if len(declarations) > 1
    }
    conflicting_runtime_bindings = []
    for binding, declarations in runtime_bindings.items():
        if len(declarations) < 2:
            continue
        identities = {
            _canonical_sha256(
                {
                    "key": item["key"],
                    "secret": item["secret"],
                    "source": (item.get("sources") or [{}])[0].get("reference", {}),
                }
            )
            for item in declarations
        }
        if len(identities) > 1:
            conflicting_runtime_bindings.append(binding)
    shared_authorities = []
    for authority in SHARED_AUTHORITIES:
        authority_path = ROOT / str(authority["path"])
        shared_authorities.append(
            {
                **authority,
                "source_sha256": _sha256(authority_path.read_bytes()),
            }
        )
    source_files = sorted(
        {
            source["path"]
            for entry in entries
            for source in entry.get("sources", [])
            if source.get("path")
        }
        | {str(item["path"]) for item in SHARED_AUTHORITIES}
    )
    source_hashes = {
        path: _sha256((ROOT / path).read_bytes())
        for path in source_files
    }
    catalog: dict[str, Any] = {
        "schema_version": 1,
        "control_id": "T-CONFIG-001",
        "status": "candidate_default_off",
        "authority": {
            "governance_metadata": "contracts/configuration/configuration-catalog.v1.json",
            "runtime_value_transition": "legacy_exact_diff_until_catalog_renderers_land",
            "secret_values": "external_secret_or_kubernetes_secret_only",
        },
        "precedence": [
            "command_line",
            "environment_variable",
            "secret_reference",
            "configmap_or_file",
            "code_or_properties_default",
        ],
        "effective_hash": {
            "algorithm": "sha256-canonical-json-v1",
            "include": [
                "consumer",
                "key",
                "resolved_non_secret_value",
                "secret_reference_name_key_and_resource_version",
            ],
            "exclude": ["secret_value", "token", "password", "private_key"],
            "required_outputs": [
                "redacted_startup_summary",
                "config_version_metric",
                "effective_config_sha256_metric",
            ],
        },
        "rollout": {
            "hot_reload_requires_ack": True,
            "unacked_instance_action": "remove_from_traffic_and_keep_previous_version",
            "restart_change_strategy": "one_instance_at_a_time_with_readiness",
            "rollback": "restore_complete_immutable_catalog_and_rendered_artifact",
        },
        "shared_authorities": shared_authorities,
        "source_hashes": source_hashes,
        "parse_errors": parse_errors,
        "duplicate_runtime_bindings": duplicate_runtime_bindings,
        "conflicting_runtime_bindings": sorted(conflicting_runtime_bindings),
        "counts": {
            "entries": len(entries),
            "go_env_consumers": len(go_entries),
            "flink_properties": len(flink_entries),
            "kubernetes_env_bindings": len(kubernetes_entries),
            "secret_entries": sum(bool(item["secret"]) for item in entries),
            "source_files": len(source_files),
        },
        "entries": entries,
    }
    catalog["catalog_sha256"] = _canonical_sha256(catalog)
    return catalog


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=OUTPUT)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog = build_catalog()
    payload = json.dumps(catalog, ensure_ascii=False, indent=2) + "\n"
    output = args.output if args.output.is_absolute() else ROOT / args.output
    if args.check:
        if not output.exists() or output.read_text(encoding="utf-8") != payload:
            print(
                json.dumps(
                    {
                        "status": "FAIL",
                        "reason": "configuration catalog is missing or stale",
                        "output": _relative(output),
                        "expected_catalog_sha256": catalog["catalog_sha256"],
                    },
                    ensure_ascii=False,
                    indent=2,
                )
            )
            return 1
    else:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(
        json.dumps(
            {
                "status": "PASS",
                "output": _relative(output),
                "catalog_sha256": catalog["catalog_sha256"],
                "counts": catalog["counts"],
                "parse_errors": catalog["parse_errors"],
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0 if not catalog["parse_errors"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
