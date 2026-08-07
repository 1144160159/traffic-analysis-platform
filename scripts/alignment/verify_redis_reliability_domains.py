#!/usr/bin/env python3
"""Verify T-REDIS-001 reliability-domain safe-hold and migration guards."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/redis/reliability-domains.v1.json")


def _load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def _documents(path: Path) -> dict[tuple[str, str], dict[str, Any]]:
    result: dict[tuple[str, str], dict[str, Any]] = {}
    for item in yaml.safe_load_all(path.read_text(encoding="utf-8")):
        if not item:
            continue
        key = (str(item.get("kind", "")), str((item.get("metadata") or {}).get("name", "")))
        if key in result:
            raise ValueError(f"duplicate Kubernetes resource {key}")
        result[key] = item
    return result


def _container(workload: dict[str, Any]) -> dict[str, Any]:
    containers = workload.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
    if len(containers) != 1:
        raise ValueError(f"{workload.get('metadata', {}).get('name')}: expected one container")
    return containers[0]


def _arg_value(container: dict[str, Any], option: str) -> str | None:
    args = [str(value) for value in container.get("args") or []]
    try:
        index = args.index(option)
    except ValueError:
        return None
    return args[index + 1] if index + 1 < len(args) else None


def _env(container: dict[str, Any]) -> dict[str, str]:
    values: dict[str, str] = {}
    for item in container.get("env") or []:
        if "value" in item:
            values[str(item.get("name"))] = str(item.get("value"))
    return values


def verify(root: Path = ROOT) -> dict[str, Any]:
    contract = _load_json(root / CONTRACT)
    errors: list[str] = []
    if contract.get("schema_version") != 1 or contract.get("remediation_id") != "T-REDIS-001":
        errors.append("Redis reliability-domain contract must be schema v1 for T-REDIS-001")
    if contract.get("status") not in {"implementing", "verifying"}:
        errors.append("contract must not claim closed before live migration and fault evidence")
    if contract.get("production_applied") is not False:
        errors.append("repository evidence must not claim the Redis domain migration is applied")
    if contract.get("repository_mode") != "safe-hold-with-isolated-cache-target":
        errors.append("repository mode must preserve safe-hold until mixed clients are split")

    domains = contract.get("domains") or {}
    for name in ("coordination", "session", "cache", "stream_queue"):
        if name not in domains:
            errors.append(f"contract is missing {name} domain")
    if (domains.get("coordination") or {}).get("maxmemory_policy") != "noeviction":
        errors.append("coordination contract must require noeviction")
    if (domains.get("session") or {}).get("database") != 1:
        errors.append("session contract must use a separate logical database")
    if (domains.get("cache") or {}).get("maxmemory_policy") != "allkeys-lru":
        errors.append("cache contract must remain explicitly discardable")
    if (domains.get("stream_queue") or {}).get("repository_state") != "FORBIDDEN_AS_KAFKA_REPLACEMENT":
        errors.append("Redis stream/queue must not silently replace Kafka")
    if len(contract.get("remaining_gates") or []) < 6:
        errors.append("contract must retain live split, failure, security and observation gates")

    infra_path = root / str(contract.get("infrastructure_manifest", ""))
    app_path = root / str(contract.get("application_manifest", ""))
    try:
        infra = _documents(infra_path)
        apps = _documents(app_path)
    except (OSError, ValueError, yaml.YAMLError) as exc:
        return {
            "schema_version": 1,
            "contract_id": contract.get("contract_id"),
            "remediation_id": "T-REDIS-001",
            "status": "FAIL",
            "coverage_status": "PARTIAL_SAFE_HOLD",
            "errors": errors + [str(exc)],
        }

    master = infra.get(("StatefulSet", "redis-master"))
    replicas = infra.get(("StatefulSet", "redis-replica"))
    cache = infra.get(("StatefulSet", "redis-cache"))
    cache_service = infra.get(("Service", "redis-cache"))
    legacy_service = infra.get(("Service", "redis"))
    for label, value in (
        ("coordination master", master),
        ("coordination replicas", replicas),
        ("isolated cache target", cache),
        ("cache Service", cache_service),
        ("legacy compatibility Service", legacy_service),
    ):
        if value is None:
            errors.append(f"infrastructure manifest is missing {label}")

    reliable_policies: dict[str, str | None] = {}
    for name, workload in (("redis-master", master), ("redis-replica", replicas)):
        if not workload:
            continue
        container = _container(workload)
        policy = _arg_value(container, "--maxmemory-policy")
        reliable_policies[name] = policy
        if policy != "noeviction":
            errors.append(f"{name} reliable domain must use noeviction, found {policy}")
        if _arg_value(container, "--appendonly") != "yes":
            errors.append(f"{name} reliable domain must retain AOF")
        labels = workload.get("spec", {}).get("template", {}).get("metadata", {}).get("labels") or {}
        if labels.get("traffic.analysis/redis-domain") != "coordination":
            errors.append(f"{name} pod must declare coordination domain")

    cache_policy = None
    if cache:
        container = _container(cache)
        cache_policy = _arg_value(container, "--maxmemory-policy")
        if cache_policy != "allkeys-lru":
            errors.append("redis-cache must use allkeys-lru")
        if _arg_value(container, "--appendonly") != "no":
            errors.append("redis-cache must not persist AOF")
        if _arg_value(container, "--save") != "":
            errors.append("redis-cache must disable RDB snapshots")
        volumes = cache.get("spec", {}).get("template", {}).get("spec", {}).get("volumes") or []
        if not any("emptyDir" in volume for volume in volumes):
            errors.append("redis-cache must use discardable emptyDir storage")
    if cache_service:
        expected = {"app": "redis-cache", "traffic.analysis/redis-domain": "cache"}
        if (cache_service.get("spec", {}).get("selector") or {}) != expected:
            errors.append("redis-cache Service must select only cache-domain pods")
    if legacy_service:
        annotations = legacy_service.get("metadata", {}).get("annotations") or {}
        if annotations.get("alignment.traffic-platform.io/compatibility-only") != "true":
            errors.append("legacy redis Service must be marked compatibility-only")
        if annotations.get("alignment.traffic-platform.io/new-client-use") != "forbidden":
            errors.append("legacy redis Service must forbid new clients")

    bindings = contract.get("workload_bindings") or []
    seen: set[str] = set()
    for binding in bindings:
        workload_name = str(binding.get("workload", ""))
        if workload_name in seen:
            errors.append(f"duplicate workload binding {workload_name}")
            continue
        seen.add(workload_name)
        workload = apps.get(("Deployment", workload_name))
        if not workload:
            errors.append(f"application manifest is missing bound workload {workload_name}")
            continue
        env = _env(_container(workload))
        domain = str(binding.get("domain", ""))
        database = str(binding.get("database"))
        if env.get("REDIS_RELIABILITY_DOMAIN") != domain:
            errors.append(f"{workload_name} must declare Redis domain {domain}")
        if env.get("REDIS_DB") != database:
            errors.append(f"{workload_name} must use Redis DB {database}")
        if domain in {"coordination", "session"}:
            if env.get("REDIS_SENTINEL_ADDRS") != "redis-sentinel.databases.svc:26379":
                errors.append(f"{workload_name} reliable domain must use Sentinel")
            if env.get("REDIS_SENTINEL_MASTER") != "mymaster":
                errors.append(f"{workload_name} reliable domain must use mymaster")
            if env.get("REDIS_ADDR") in {"redis:6379", "redis.databases.svc:6379"}:
                errors.append(f"{workload_name} must not use the mixed legacy Service")
        elif domain == "cache":
            if env.get("REDIS_ADDR") != "redis-cache.databases.svc:6379":
                errors.append(f"{workload_name} cache domain must use redis-cache")
            if "REDIS_SENTINEL_ADDRS" in env:
                errors.append(f"{workload_name} cache domain must not use coordination Sentinel")
        else:
            errors.append(f"{workload_name} has unknown Redis domain {domain}")

    prefixes = contract.get("keyspace_inventory") or []
    prefix_names = [str(item.get("prefix", "")) for item in prefixes]
    if len(prefix_names) != len(set(prefix_names)) or any(not value for value in prefix_names):
        errors.append("keyspace prefix inventory must be non-empty and unique")
    valid_domains = {"coordination", "session", "cache"}
    for item in prefixes:
        if item.get("domain") not in valid_domains:
            errors.append(f"keyspace prefix {item.get('prefix')} has invalid domain")
        if not item.get("authority"):
            errors.append(f"keyspace prefix {item.get('prefix')} lacks an authority")

    runbook = root / "doc/07_alignment/runbooks/T-REDIS-001-reliability-domain-migration.md"
    if not runbook.is_file():
        errors.append("T-REDIS-001 runbook is missing")
    else:
        source = runbook.read_text(encoding="utf-8")
        for token in (
            "production_applied=false",
            "SAFE_HOLD",
            "noeviction",
            "redis-cache.databases.svc:6379",
            "fail closed",
            "T+0/T+1/T+3/T+7",
        ):
            if token not in source:
                errors.append(f"T-REDIS-001 runbook missing {token}")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": "T-REDIS-001",
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL_SAFE_HOLD",
        "repository_mode": contract.get("repository_mode"),
        "production_applied": contract.get("production_applied"),
        "reliable_policies": reliable_policies,
        "cache_policy": cache_policy,
        "workload_bindings": len(bindings),
        "mixed_clients_remaining": sum(bool(item.get("mixed_client_safe_hold")) for item in bindings),
        "keyspace_prefixes": len(prefixes),
        "errors": errors,
        "remaining_gates": contract.get("remaining_gates") or [],
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
