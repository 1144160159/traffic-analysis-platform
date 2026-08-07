#!/usr/bin/env python3
"""Capture immutable repository and redacted read-only live T-CONFIG-001 evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
CATALOG = ROOT / "contracts/configuration/configuration-catalog.v1.json"
COMMANDS = (
    (
        "configuration-catalog-current",
        ["python3", "scripts/alignment/build_configuration_catalog.py", "--check"],
    ),
    (
        "configuration-catalog-verifier",
        ["python3", "scripts/alignment/verify_configuration_catalog.py"],
    ),
    (
        "configuration-catalog-negative-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_configuration_catalog", "-v"],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/configuration/configuration-catalog.v1.json",
    "scripts/alignment/build_configuration_catalog.py",
    "scripts/alignment/verify_configuration_catalog.py",
    "scripts/alignment/capture_configuration_catalog.py",
    "tests/alignment/test_configuration_catalog.py",
    "doc/07_alignment/runbooks/T-CONFIG-001-configuration-catalog.md",
    "Makefile",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def direct_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for key in (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "http_proxy",
        "https_proxy",
        "all_proxy",
    ):
        environment.pop(key, None)
    return environment


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[configuration] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=direct_environment(),
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[configuration] {name}: {result['status']}", flush=True)
    return result


def kubectl_json(arguments: list[str]) -> dict[str, Any]:
    completed = subprocess.run(
        ["kubectl", "--request-timeout=15s", *arguments, "-o", "json"],
        cwd=ROOT,
        env=direct_environment(),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=30,
    )
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or "kubectl failed")
    return json.loads(completed.stdout)


def pod_spec(resource: dict[str, Any]) -> dict[str, Any]:
    kind = resource.get("kind")
    spec = resource.get("spec") or {}
    if kind in {"Deployment", "StatefulSet", "DaemonSet", "Job", "ReplicaSet"}:
        return ((spec.get("template") or {}).get("spec") or {})
    if kind == "CronJob":
        return (
            ((((spec.get("jobTemplate") or {}).get("spec") or {}).get("template") or {}).get("spec"))
            or {}
        )
    return {}


def source_identity(env: dict[str, Any]) -> tuple[str, dict[str, Any]]:
    value_from = env.get("valueFrom") or {}
    if "secretKeyRef" in value_from:
        source = value_from["secretKeyRef"] or {}
        return "secretKeyRef", {
            "name": source.get("name"),
            "key": source.get("key"),
            "optional": bool(source.get("optional", False)),
        }
    if "configMapKeyRef" in value_from:
        source = value_from["configMapKeyRef"] or {}
        return "configMapKeyRef", {
            "name": source.get("name"),
            "key": source.get("key"),
            "optional": bool(source.get("optional", False)),
        }
    if "fieldRef" in value_from:
        source = value_from["fieldRef"] or {}
        return "fieldRef", {"fieldPath": source.get("fieldPath")}
    if "value" in env:
        value = str(env.get("value", ""))
        return "literal", {"value_sha256": hashlib.sha256(value.encode()).hexdigest(), "empty": value == ""}
    return "unknown", {}


def secret_resource_versions(bindings: list[dict[str, Any]]) -> dict[str, Any]:
    references = sorted(
        {
            (item["namespace"], str(item["identity"].get("name")))
            for item in bindings
            if item["source_kind"] == "secretKeyRef" and item["identity"].get("name")
        }
    )
    versions: dict[str, Any] = {}
    for namespace, name in references:
        completed = subprocess.run(
            [
                "kubectl",
                "--request-timeout=8s",
                "-n",
                namespace,
                "get",
                "secret",
                name,
                "-o",
                "jsonpath={.metadata.resourceVersion}",
            ],
            cwd=ROOT,
            env=direct_environment(),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
            timeout=15,
        )
        versions[f"{namespace}/{name}"] = {
            "found": completed.returncode == 0,
            "resource_version": completed.stdout.strip() if completed.returncode == 0 else None,
            "error_class": None if completed.returncode == 0 else "metadata_lookup_failed",
        }
    return versions


def capture_live(catalog: dict[str, Any]) -> dict[str, Any]:
    expected: dict[str, set[str]] = {}
    governed_resources: set[str] = set()
    for entry in catalog["entries"]:
        binding = str(entry["runtime_binding_id"])
        if not binding.startswith("k8s:"):
            continue
        source = (entry.get("sources") or [{}])[0]
        kind = str(source.get("kind", "")).removeprefix("kubernetes_")
        identity = source.get("reference") or {}
        expected.setdefault(binding, set()).add(canonical_sha256({"kind": kind, "identity": identity}))
        governed_resources.add(":".join(binding.split(":")[:5]))

    resources = kubectl_json(
        ["get", "deployments,statefulsets,daemonsets,jobs,cronjobs", "-A"]
    )
    bindings: list[dict[str, Any]] = []
    for resource in resources.get("items") or []:
        kind = str(resource.get("kind", ""))
        metadata = resource.get("metadata") or {}
        namespace = str(metadata.get("namespace") or "default")
        name = str(metadata.get("name") or "unnamed")
        resource_prefix = f"k8s:{namespace}:{kind}:{name}"
        if not any(item.startswith(resource_prefix + ":") for item in expected):
            continue
        spec = pod_spec(resource)
        for container in list(spec.get("initContainers") or []) + list(spec.get("containers") or []):
            container_name = str(container.get("name") or "unnamed")
            for env in container.get("env") or []:
                if not isinstance(env, dict) or not env.get("name"):
                    continue
                key = str(env["name"])
                binding_id = f"{resource_prefix}:{container_name}:{key}"
                source_kind, identity = source_identity(env)
                identity_sha = canonical_sha256({"kind": source_kind, "identity": identity})
                bindings.append(
                    {
                        "runtime_binding_id": binding_id,
                        "namespace": namespace,
                        "resource": f"{kind}/{name}",
                        "container": container_name,
                        "key": key,
                        "source_kind": source_kind,
                        "identity": identity,
                        "identity_sha256": identity_sha,
                        "registered": binding_id in expected,
                        "matches_declared_source": identity_sha in expected.get(binding_id, set()),
                    }
                )
    bindings.sort(key=lambda item: item["runtime_binding_id"])
    secret_versions = secret_resource_versions(bindings)
    drift = [item["runtime_binding_id"] for item in bindings if item["registered"] and not item["matches_declared_source"]]
    unregistered = [item["runtime_binding_id"] for item in bindings if not item["registered"]]
    workload_hashes: dict[str, str] = {}
    for item in bindings:
        workload = f"{item['namespace']}/{item['resource']}"
        workload_hashes.setdefault(workload, "")
    for workload in list(workload_hashes):
        values = [
            {
                "runtime_binding_id": item["runtime_binding_id"],
                "source_kind": item["source_kind"],
                "identity_sha256": item["identity_sha256"],
            }
            for item in bindings
            if f"{item['namespace']}/{item['resource']}" == workload
        ]
        workload_hashes[workload] = canonical_sha256(values)
    return {
        "read_only": True,
        "queried_resource_count": len(resources.get("items") or []),
        "governed_live_binding_count": len(bindings),
        "matching_binding_count": sum(item["matches_declared_source"] for item in bindings),
        "drift_binding_ids": sorted(drift),
        "unregistered_binding_ids": sorted(unregistered),
        "workload_rendered_config_sha256": dict(sorted(workload_hashes.items())),
        "secret_reference_metadata": secret_versions,
        "process_effective_config_sha256": None,
        "process_effective_config_reason": "candidate services do not yet expose the T-CONFIG-001 metric",
        "secret_values_captured": False,
        "production_mutations": [],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or candidate_before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)

    live: dict[str, Any]
    try:
        live = capture_live(json.loads(CATALOG.read_text(encoding="utf-8")))
        live["query_status"] = "PASS"
    except Exception as exc:  # evidence must preserve an unavailable live control plane as partial
        live = {
            "read_only": True,
            "query_status": "FAIL",
            "error_class": type(exc).__name__,
            "production_mutations": [],
            "secret_values_captured": False,
        }

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    scoped_pass = repository_pass and live.get("query_status") == "PASS" and candidate_stable
    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-CONFIG-001",
        "related_ids": ["T-SEC-001", "T-OBS-001", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_CATALOG_AND_REDACTED_LIVE_RENDERED_BINDINGS",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "catalog_summary": {
            "catalog_sha256": catalog.get("catalog_sha256"),
            "counts": catalog.get("counts"),
            "conflicting_runtime_bindings": catalog.get("conflicting_runtime_bindings"),
            "shared_authority_domains": [item["domain"] for item in catalog.get("shared_authorities", [])],
        },
        "live_observation": live,
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_REDACTED_CONSUMER_SCOPED_CATALOG_PRECEDENCE_SOURCE_HASHES_AND_DRIFT_NEGATIVE_TESTS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_READ_ONLY_LIVE_RENDERED_BINDINGS_OPEN_FOR_PROCESS_EFFECTIVE_HASH_AND_CANDIDATE_ROLLOUT",
            "G3": "OPEN_FOR_DECLARED_RENDERED_CLUSTER_AND_PROCESS_HASH_RECONCILIATION",
            "G4": "OPEN_FOR_RELOAD_ROLLING_RESTART_AND_RESOURCE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_CANARY_ROLLBACK_SECRET_ROTATION_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "consumer-scoped Go, Flink and Kubernetes keys carry required governance metadata",
            "secret values and non-empty secret defaults are excluded and rejected",
            "critical Kafka, Flink, ClickHouse, MinIO, IAM and APISIX authorities are hash-bound",
            "multiple Kubernetes declarations cannot silently disagree on one runtime binding",
            "live workload values are represented only by hashes and secret references by metadata versions",
        ],
        "open": [
            "move all runtime values from legacy exact-diff sources to catalog renderers",
            "publish redacted startup summaries and effective_config_sha256 metrics from every process",
            "bind cluster rendered hashes to immutable release artifacts",
            "exercise invalid value, missing required, partial rollout, reload ACK, rollback and secret rotation",
            "complete performance, Windows Chrome, rollout and observation gates",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": str(manifest_path.relative_to(ROOT)),
                "manifest_sha256": sha256(manifest_path),
                "catalog_sha256": catalog.get("catalog_sha256"),
                "live_query_status": live.get("query_status"),
                "live_drift_count": len(live.get("drift_binding_ids") or []),
                "live_unregistered_count": len(live.get("unregistered_binding_ids") or []),
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
