#!/usr/bin/env python3
"""Capture immutable T-SEC-001 repository and redacted read-only live evidence."""

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
CATALOG = ROOT / "contracts/security/service-identity-catalog.v1.json"
COMMANDS = (
    (
        "service-identity-catalog-current",
        ["python3", "scripts/alignment/build_service_identity_catalog.py", "--check"],
    ),
    (
        "service-identity-catalog-verifier",
        ["python3", "scripts/alignment/verify_service_identity_catalog.py"],
    ),
    (
        "service-identity-negative-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_service_identity_catalog", "-v"],
    ),
    (
        "service-accounts-dry-run",
        ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/security/go-service-identities.v1.yaml"],
    ),
    (
        "go-workloads-dry-run",
        ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/applications/go-services.yaml"],
    ),
    (
        "web-ui-dry-run",
        ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/applications/web-ui.yaml"],
    ),
    (
        "probe-agent-dry-run",
        ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/applications/probe-agent.yaml"],
    ),
    (
        "flink-log-job-dry-run",
        ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/flink/flink-log-job.yaml"],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/security/service-identity-catalog.v1.json",
    "scripts/alignment/build_service_identity_catalog.py",
    "scripts/alignment/verify_service_identity_catalog.py",
    "scripts/alignment/capture_service_identity_catalog.py",
    "tests/alignment/test_service_identity_catalog.py",
    "deployments/kubernetes/security/go-service-identities.v1.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "deployments/kubernetes/applications/web-ui.yaml",
    "deployments/kubernetes/applications/probe-agent.yaml",
    "deployments/kubernetes/flink/flink-log-job.yaml",
    "contracts/events/kafka-acl-catalog.v1.json",
    "deployments/kubernetes/security/external-secrets-template.yaml",
    "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml",
    "doc/07_alignment/runbooks/T-SEC-001-service-identity-secret-governance.md",
    "Makefile",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def direct_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    return environment


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[service-identity] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[service-identity] {name}: {result['status']}", flush=True)
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


def _pod_spec(resource: dict[str, Any]) -> dict[str, Any]:
    spec = resource.get("spec") or {}
    if resource.get("kind") == "CronJob":
        return (((((spec.get("jobTemplate") or {}).get("spec") or {}).get("template") or {}).get("spec")) or {})
    return ((spec.get("template") or {}).get("spec") or {})


def _security(container: dict[str, Any]) -> dict[str, Any]:
    security = container.get("securityContext") or {}
    capabilities = security.get("capabilities") or {}
    return {
        "run_as_non_root": security.get("runAsNonRoot"),
        "run_as_user": security.get("runAsUser"),
        "run_as_group": security.get("runAsGroup"),
        "allow_privilege_escalation": security.get("allowPrivilegeEscalation"),
        "capabilities_drop": sorted(str(value) for value in capabilities.get("drop") or []),
        "capabilities_add": sorted(str(value) for value in capabilities.get("add") or []),
        "seccomp_profile": (security.get("seccompProfile") or {}).get("type"),
        "privileged": bool(security.get("privileged", False)),
    }


def _secret_metadata(catalog: dict[str, Any]) -> dict[str, Any]:
    references = sorted(
        {
            (str(reference["namespace"]), str(reference["secret_name"]))
            for workload in catalog.get("workloads") or []
            for reference in workload.get("secret_references") or []
        }
    )
    result: dict[str, Any] = {}
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
        result[f"{namespace}/{name}"] = {
            "found": completed.returncode == 0,
            "resource_version": completed.stdout.strip() if completed.returncode == 0 else None,
            "error_class": None if completed.returncode == 0 else "metadata_lookup_failed",
        }
    return result


def capture_live(catalog: dict[str, Any]) -> dict[str, Any]:
    resources = kubectl_json(["get", "deployments,daemonsets,jobs", "-A"])
    service_accounts = kubectl_json(["get", "serviceaccounts", "-A"])
    pods = kubectl_json(["get", "pods", "-A"])
    resource_index = {
        (
            str(item.get("kind") or ""),
            str((item.get("metadata") or {}).get("namespace") or "default"),
            str((item.get("metadata") or {}).get("name") or ""),
        ): item
        for item in resources.get("items") or []
    }
    account_index = {
        (
            str((item.get("metadata") or {}).get("namespace") or "default"),
            str((item.get("metadata") or {}).get("name") or ""),
        ): item
        for item in service_accounts.get("items") or []
    }
    image_ids: dict[tuple[str, str], set[str]] = {}
    for pod in pods.get("items") or []:
        metadata = pod.get("metadata") or {}
        namespace = str(metadata.get("namespace") or "default")
        labels = metadata.get("labels") or {}
        names = {
            str(labels.get("app") or ""),
            str(labels.get("app.kubernetes.io/name") or ""),
            str(labels.get("job-name") or ""),
        }
        for status in (pod.get("status") or {}).get("containerStatuses") or []:
            image_id = str(status.get("imageID") or "")
            if not image_id:
                continue
            for name in names - {""}:
                image_ids.setdefault((namespace, name), set()).add(image_id)

    observations: list[dict[str, Any]] = []
    for expected in catalog.get("workloads") or []:
        key = (str(expected["kind"]), str(expected["namespace"]), str(expected["name"]))
        resource = resource_index.get(key)
        candidate_account = expected.get("service_account") or {}
        candidate_containers = {
            str(item.get("name") or ""): item for item in expected.get("containers") or []
        }
        observation: dict[str, Any] = {
            "workload_id": expected.get("workload_id"),
            "kind": key[0],
            "namespace": key[1],
            "name": key[2],
            "found": resource is not None,
            "resource_version": (resource or {}).get("metadata", {}).get("resourceVersion"),
            "candidate_service_account": candidate_account.get("name"),
            "candidate_token_automount": candidate_account.get("pod_token_automount"),
            "service_account_match": False,
            "token_automount_match": False,
            "container_security_matches": {},
            "images": [],
            "running_image_ids": sorted(image_ids.get((key[1], key[2]), set())),
        }
        if resource is not None:
            spec = _pod_spec(resource)
            live_account_name = str(spec.get("serviceAccountName") or "default")
            live_automount = spec.get("automountServiceAccountToken")
            observation.update(
                {
                    "live_service_account": live_account_name,
                    "live_token_automount": live_automount,
                    "service_account_match": live_account_name == candidate_account.get("name"),
                    "token_automount_match": live_automount == candidate_account.get("pod_token_automount"),
                    "service_account_resource_found": (key[1], live_account_name) in account_index,
                }
            )
            for container in spec.get("containers") or []:
                name = str(container.get("name") or "")
                live_security = _security(container)
                candidate = candidate_containers.get(name)
                comparable = {
                    field: candidate.get(field)
                    for field in live_security
                } if candidate else None
                observation["container_security_matches"][name] = (
                    live_security == comparable if comparable is not None else False
                )
                observation["images"].append({"container": name, "image": container.get("image")})
        observations.append(observation)

    drift = []
    for item in observations:
        if not item["found"]:
            drift.append(f"{item['namespace']}/{item['kind']}/{item['name']}:missing")
            continue
        if not item["service_account_match"]:
            drift.append(f"{item['namespace']}/{item['kind']}/{item['name']}:service_account")
        if not item["token_automount_match"]:
            drift.append(f"{item['namespace']}/{item['kind']}/{item['name']}:token_automount")
        for container, matches in item["container_security_matches"].items():
            if not matches:
                drift.append(f"{item['namespace']}/{item['kind']}/{item['name']}:{container}:security_context")
    secret_metadata = _secret_metadata(catalog)
    return {
        "read_only": True,
        "workloads": observations,
        "workload_observation_count": len(observations),
        "found_workload_count": sum(item["found"] for item in observations),
        "live_candidate_drift": sorted(drift),
        "secret_reference_metadata": secret_metadata,
        "secret_reference_found_count": sum(item["found"] for item in secret_metadata.values()),
        "kafka_acl_live_status": "OPEN_NOT_CAPTURED",
        "database_and_object_policy_live_status": "OPEN_NOT_CAPTURED",
        "certificate_identity_live_status": "OPEN_FOR_T_PKI_001",
        "secret_values_captured": False,
        "response_payloads_captured": False,
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

    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    try:
        live = capture_live(catalog)
        live["query_status"] = "PASS"
    except Exception as exc:
        live = {
            "read_only": True,
            "query_status": "FAIL",
            "error_class": type(exc).__name__,
            "secret_values_captured": False,
            "production_mutations": [],
        }

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    scoped_pass = (
        repository_pass
        and live.get("query_status") == "PASS"
        and live.get("workload_observation_count") == len(catalog.get("workloads") or [])
        and candidate_stable
    )
    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-SEC-001",
        "related_ids": ["T-PKI-001", "T-IAM-002", "T-KAFKA-001", "T-CONFIG-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_IDENTITY_SECRET_TENANT_INVENTORY_AND_REDACTED_LIVE_BINDINGS",
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
            "security_compliance": "PARTIAL",
        },
        "live_observation": live,
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_VERSIONED_REDACTED_IDENTITY_SECRET_KAFKA_AND_TENANT_RISK_INVENTORY_WITH_NEGATIVE_TESTS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_READ_ONLY_LIVE_WORKLOAD_SERVICE_ACCOUNT_SECURITY_CONTEXT_IMAGE_AND_SECRET_METADATA",
            "G3": "OPEN_FOR_TENANT_NEGATIVES_CREDENTIAL_ROTATION_CERTIFICATE_AND_CROSS_STORE_POLICY_RECONCILIATION",
            "G4": "OPEN_FOR_AUTHENTICATION_AUTHORIZATION_AND_ROTATION_LOAD_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_CROSS_TENANT_AND_UNDER_SCOPED_NEGATIVES",
            "G6": "HOLD_FOR_IDENTITY_SECRET_AND_CERTIFICATE_CANARY_ROLLBACK_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all governed workloads have unique candidate Kubernetes ServiceAccounts",
            "business workload token automount and Go runtime container hardening fail closed",
            "Secret references are inventoried by metadata without serializing values",
            "shared sensitive credentials and untrusted tenant fallbacks cannot be hidden",
            "Kafka workload principals reject wildcard anonymous and shared traffic credentials",
            "live observation is read-only and candidate drift remains explicit",
        ],
        "open": [
            "deploy unique ServiceAccounts and hardened workload templates through canary rollout",
            "replace shared datastore and JWT credentials with per-service identities and rotation evidence",
            "remove request header and query tenant authority fallbacks in favor of trusted auth context",
            "bind service-to-service mTLS certificate identity and T-PKI-001 rotation evidence",
            "capture live Kafka ACL database OpenSearch Nebula and MinIO least-privilege policy reconciliation",
            "complete cross-tenant under-scoped rotation rollback performance and observation gates",
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
                "live_candidate_drift_count": len(live.get("live_candidate_drift") or []),
                "secret_values_captured": False,
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
