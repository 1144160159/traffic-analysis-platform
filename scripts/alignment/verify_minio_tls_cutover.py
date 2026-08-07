#!/usr/bin/env python3
"""Verify the default-off MinIO TLS cutover bundle without applying it."""

from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/minio/tls-cutover.v1.json")
OVERLAY = Path("deployments/kubernetes/security/minio-tls-cutover-v1")
BASE_SERVER = Path("deployments/kubernetes/infrastructure/06-minio.yaml")

EXPECTED_COMPONENTS = {
    "minio-server",
    "minio-console-proxy",
    "alert-service",
    "asset-service",
    "forensics-service",
    "probe-agent",
    "flink-jobmanager",
    "flink-taskmanager",
    "flink-job-config",
    "argo-artifact-repositories",
    "argo-workflow-controller",
    "mlops-register-model",
    "minio-multipart-cleanup",
    "minio-post-cutover-verifier",
}
EXPECTED_IMAGES = {
    "alert-service",
    "asset-service",
    "forensics-service",
    "probe-agent",
    "mlops-trainer",
}
LOCAL_IMAGE_STATUS = "BUILT_LOCAL_NOT_DISTRIBUTED"
SHA256_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
SOURCE_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def _load_json(root: Path) -> dict[str, Any]:
    value = json.loads((root / CONTRACT).read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("MinIO TLS cutover contract must contain an object")
    return value


def _render(root: Path) -> str:
    result = subprocess.run(
        [
            "kubectl",
            "kustomize",
            "--load-restrictor=LoadRestrictionsNone",
            str(root / OVERLAY),
        ],
        cwd=root,
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode:
        raise RuntimeError(f"MinIO TLS overlay render failed: {result.stderr.strip()}")
    return result.stdout


def _resource_index(rendered: str) -> dict[tuple[str, str, str], dict[str, Any]]:
    index: dict[tuple[str, str, str], dict[str, Any]] = {}
    for item in yaml.safe_load_all(rendered):
        if not item:
            continue
        metadata = item.get("metadata") or {}
        key = (
            str(item.get("kind") or ""),
            str(metadata.get("namespace") or "default"),
            str(metadata.get("name") or ""),
        )
        if key in index:
            raise ValueError(f"duplicate rendered resource: {key}")
        index[key] = item
    return index


def _container(resource: dict[str, Any], name: str) -> dict[str, Any]:
    spec = resource.get("spec") or {}
    pod_spec = ((spec.get("template") or {}).get("spec") or {})
    for container in pod_spec.get("containers") or []:
        if container.get("name") == name:
            return container
    return {}


def _env(container: dict[str, Any]) -> dict[str, Any]:
    return {str(item.get("name") or ""): item.get("value") for item in container.get("env") or []}


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    for required in (CONTRACT, OVERLAY / "kustomization.yaml", BASE_SERVER):
        if not (root / required).exists():
            errors.append(f"missing required cutover artifact: {required}")
    if errors:
        return {"status": "FAIL", "errors": errors}

    contract = _load_json(root)
    if contract.get("schema_version") != 1 or contract.get("contract_id") != "traffic-platform-minio-tls-cutover-v1":
        errors.append("MinIO TLS cutover contract identity drifted")
    if contract.get("status") != "implementing" or contract.get("phase") != "default_off_atomic_bundle":
        errors.append("MinIO TLS cutover must remain in the implementing default-off phase")
    if contract.get("default_off") is not True:
        errors.append("MinIO TLS cutover bundle must remain default-off")
    if contract.get("production_applied") is not False or contract.get("cutover_ready") is not False:
        errors.append("MinIO TLS cutover cannot claim production apply or readiness without live gates")
    if contract.get("maintenance_outage_required") is not True:
        errors.append("MinIO TLS cutover must declare the coordinated outage requirement")
    if set(contract.get("components") or []) != EXPECTED_COMPONENTS:
        errors.append("MinIO TLS cutover component inventory drifted")

    images = contract.get("candidate_images") or []
    if {str(item.get("workload") or "") for item in images} != EXPECTED_IMAGES:
        errors.append("MinIO TLS candidate image inventory drifted")
    candidate_by_workload: dict[str, dict[str, Any]] = {}
    for item in images:
        if not isinstance(item, dict):
            errors.append("MinIO TLS candidate image entry must be an object")
            continue
        workload = str(item.get("workload") or "")
        candidate_by_workload[workload] = item
        source_sha = str(item.get("component_source_sha256") or "")
        expected_image = f"docker.io/traffic/{workload}:minio-tls-{source_sha[:12]}"
        if item.get("status") != LOCAL_IMAGE_STATUS:
            errors.append(f"{workload} must remain explicitly local and undistributed")
        if not SOURCE_SHA256_RE.fullmatch(source_sha):
            errors.append(f"{workload} component source hash is invalid")
        if item.get("image") != expected_image:
            errors.append(f"{workload} image tag is not bound to its component source hash")
        if not SHA256_RE.fullmatch(str(item.get("local_image_id") or "")):
            errors.append(f"{workload} local Docker image ID is invalid")
        if item.get("registry_digest") is not None:
            errors.append(f"{workload} must not claim a registry digest before distribution")
        if item.get("platform") != "linux/amd64":
            errors.append(f"{workload} candidate platform must be linux/amd64")
        if item.get("distributed_to_nodes") is not False or item.get("signed") is not False:
            errors.append(f"{workload} must not claim distribution or signing without external evidence")
        if "digest" in item:
            errors.append(f"{workload} must distinguish local_image_id from registry_digest")
    if len(contract.get("preflight") or []) < 9:
        errors.append("MinIO TLS preflight inventory is incomplete")
    if len(contract.get("cutover_sequence") or []) < 8 or len(contract.get("rollback_sequence") or []) < 7:
        errors.append("MinIO TLS cutover or rollback sequence is incomplete")
    if len(contract.get("stop_conditions") or []) < 8 or len(contract.get("known_blockers") or []) < 5:
        errors.append("MinIO TLS stop-condition or blocker inventory is incomplete")

    try:
        rendered = _render(root)
        resources = _resource_index(rendered)
    except (RuntimeError, ValueError, yaml.YAMLError) as exc:
        errors.append(str(exc))
        rendered = ""
        resources = {}

    forbidden = ("http://minio", "insecure: true", "InsecureSkipVerify", "danger_accept_invalid")
    for token in forbidden:
        if token in rendered:
            errors.append(f"rendered MinIO TLS bundle contains forbidden token: {token}")
    overlay_text = "\n".join(
        path.read_text(encoding="utf-8")
        for path in sorted((root / OVERLAY).glob("*"))
        if path.is_file()
    )
    if "BEGIN PRIVATE KEY" in overlay_text or "BEGIN CERTIFICATE" in overlay_text:
        errors.append("cutover bundle must not contain certificate or private-key values")

    minio = resources.get(("StatefulSet", "minio", "minio"), {})
    minio_container = _container(minio, "minio")
    args = minio_container.get("args") or []
    if "--certs-dir" not in args or "/etc/minio/certs" not in args or not any(str(value).startswith("https://minio-{0...3}") for value in args):
        errors.append("rendered MinIO server is not configured for distributed TLS")
    if "minio-server-tls" not in json.dumps(minio, sort_keys=True):
        errors.append("rendered MinIO server does not mount its TLS Secret")

    proxy_config = resources.get(("ConfigMap", "minio", "minio-proxy-nginx"), {})
    proxy_text = str((proxy_config.get("data") or {}).get("default.conf") or "")
    if "proxy_pass https://minio.minio.svc:9001" not in proxy_text or "proxy_ssl_verify on" not in proxy_text:
        errors.append("MinIO console proxy does not verify the TLS upstream")
    proxy = resources.get(("Deployment", "minio", "minio-proxy"), {})
    if "minio-server-tls" not in json.dumps(proxy, sort_keys=True):
        errors.append("MinIO console proxy does not mount the governed CA")

    for name in ("alert-service", "asset-service", "forensics-service"):
        deployment = resources.get(("Deployment", "traffic-analysis", name), {})
        container = _container(deployment, name)
        expected_image = candidate_by_workload.get(name, {}).get("image")
        if not expected_image or container.get("image") != expected_image:
            errors.append(f"{name} does not render the contracted local TLS candidate image")
        env = _env(container)
        if env.get("S3_USE_SSL") != "true" or env.get("S3_CA_CERT") != "/etc/minio/tls/ca.crt":
            errors.append(f"{name} does not enable verified MinIO TLS")
        if "minio-client-ca" not in json.dumps(deployment, sort_keys=True):
            errors.append(f"{name} does not mount the MinIO client CA")

    probe_config = resources.get(("ConfigMap", "traffic-analysis", "probe-agent-config"), {})
    probe_yaml = str((probe_config.get("data") or {}).get("config.yaml") or "")
    if 's3_endpoint: "https://minio.minio.svc:9000"' not in probe_yaml or 's3_ca_cert: "/etc/minio/tls/ca.crt"' not in probe_yaml:
        errors.append("probe-agent config does not bind HTTPS MinIO to its CA")
    probe = resources.get(("DaemonSet", "traffic-analysis", "probe-agent"), {})
    probe_container = _container(probe, "probe-agent")
    expected_probe_image = candidate_by_workload.get("probe-agent", {}).get("image")
    if not expected_probe_image or probe_container.get("image") != expected_probe_image:
        errors.append("probe-agent does not render the contracted local TLS candidate image")
    if "minio-client-ca" not in json.dumps(probe, sort_keys=True):
        errors.append("probe-agent does not mount the MinIO client CA")

    for name, container_name in (("flink-jobmanager", "jobmanager"), ("flink-taskmanager", "taskmanager")):
        statefulset = resources.get(("StatefulSet", "flink", name), {})
        container = _container(statefulset, container_name)
        env = _env(container)
        if "s3.endpoint: https://minio.minio.svc:9000" not in str(env.get("FLINK_PROPERTIES") or ""):
            errors.append(f"{name} S3 endpoint is not HTTPS")
        if "truststore.p12" not in str(env.get("JAVA_TOOL_OPTIONS") or ""):
            errors.append(f"{name} does not configure the MinIO truststore")
        if "minio-client-ca" not in json.dumps(statefulset, sort_keys=True):
            errors.append(f"{name} does not mount the MinIO truststore")
    flink_config = resources.get(("ConfigMap", "flink", "flink-job-config"), {})
    if (flink_config.get("data") or {}).get("s3.endpoint") != "https://minio.minio.svc:9000":
        errors.append("Flink job ConfigMap S3 endpoint is not HTTPS")

    for name, key in (("artifact-repositories", "default-v1"), ("workflow-controller-configmap", "artifactRepository")):
        config = resources.get(("ConfigMap", "argo", name), {})
        value = str((config.get("data") or {}).get(key) or "")
        if "insecure: false" not in value or "caSecret:" not in value or "minio-client-ca" not in value:
            errors.append(f"Argo {name} does not use the governed MinIO CA")

    workflow = resources.get(("WorkflowTemplate", "argo", "mlops-training-template"), {})
    workflow_parameters = {
        str(item.get("name") or ""): item.get("value")
        for item in ((workflow.get("spec") or {}).get("arguments") or {}).get("parameters") or []
    }
    expected_trainer_image = candidate_by_workload.get("mlops-trainer", {}).get("image")
    if not expected_trainer_image or workflow_parameters.get("trainer-image") != expected_trainer_image:
        errors.append("MLOps workflow does not render the contracted local TLS candidate image")
    templates = (workflow.get("spec") or {}).get("templates") or []
    register = next((item for item in templates if item.get("name") == "register-model"), {})
    register_env = _env(register.get("container") or {})
    if register_env.get("MINIO_SECURE") != "true" or register_env.get("MINIO_CA_FILE") != "/etc/minio/tls/ca.crt":
        errors.append("MLOps register-model does not enable verified MinIO TLS")
    if "minio-client-ca" not in json.dumps(workflow, sort_keys=True):
        errors.append("MLOps WorkflowTemplate does not mount the MinIO client CA")

    cron = resources.get(("CronJob", "minio", "minio-incomplete-multipart-cleanup"), {})
    verify_job = resources.get(("Job", "minio", "minio-tls-post-cutover-verify-v1"), {})
    for name, resource in (("multipart cleanup", cron), ("post-cutover verifier", verify_job)):
        text = json.dumps(resource, sort_keys=True)
        if "https://minio.minio.svc:9000" not in text or "--certs-dir" not in text:
            errors.append(f"MinIO {name} does not use verified TLS")

    base_text = (root / BASE_SERVER).read_text(encoding="utf-8")
    if "--certs-dir" in base_text or "minio-server-tls" in base_text:
        errors.append("default-off bundle must not activate the base MinIO StatefulSet")

    return {
        "status": "PASS" if not errors else "FAIL",
        "contract_id": contract.get("contract_id"),
        "default_off": contract.get("default_off"),
        "production_applied": contract.get("production_applied"),
        "cutover_ready": contract.get("cutover_ready"),
        "component_count": len(contract.get("components") or []),
        "rendered_resource_count": len(resources),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
