#!/usr/bin/env python3
"""Execute T1-M10-N004 read-only site preflight across Kubernetes nodes."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import ssl
import sys
import tempfile
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import validate_m10_site_values as site_validator


ROOT = Path(__file__).resolve().parents[2]
NAMESPACE = "traffic-analysis"
INPUT = Path("deployments/kubernetes/site-values.v1.template.yaml")
DEFAULT_OUTPUT = ROOT / "doc/02_acceptance/topic1/tasks/t1-m10-n004/k8s-site-preflight-latest.json"
SOURCE_FILES = (
    INPUT,
    Path("contracts/alignment/m10-site-values.schema.json"),
    Path("scripts/alignment/validate_m10_site_values.py"),
    Path("scripts/alignment/run_m10_site_preflight_k8s.py"),
    Path("scripts/alignment/Dockerfile.m10-site-preflight"),
)
IMAGE_RE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._/@:-]+$")
NODE_RE = re.compile(r"^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")


class PreflightError(RuntimeError):
    pass


def run(command: list[str], *, input_text: str | None = None, check: bool = True):
    import subprocess

    environment = os.environ.copy()
    if command and command[0] == "kubectl":
        for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
            environment.pop(key, None)
    result = subprocess.run(
        command, input=input_text, text=True, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, check=False, env=environment,
    )
    if check and result.returncode != 0:
        raise PreflightError(
            f"command failed ({result.returncode}): {' '.join(command)}\n{result.stdout}{result.stderr}"
        )
    return result


def kubectl(*args: str, input_text: str | None = None, check: bool = True):
    return run(["kubectl", *args], input_text=input_text, check=check)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_text(path: Path, default: str = "") -> str:
    try:
        return path.read_text(encoding="utf-8").strip()
    except OSError:
        return default


def host_cpu_count() -> int:
    body = read_text(Path("/host/proc/cpuinfo"))
    return sum(line.startswith("processor") for line in body.splitlines())


def host_memory_gib() -> float:
    match = re.search(r"^MemTotal:\s+(\d+)\s+kB$", read_text(Path("/host/proc/meminfo")), re.MULTILINE)
    return round(int(match.group(1)) / 1024 / 1024, 3) if match else 0.0


def host_numa_nodes() -> list[str]:
    root = Path("/host/sys/devices/system/node")
    return sorted(path.name for path in root.glob("node[0-9]*") if path.is_dir())


def host_nics() -> list[dict[str, Any]]:
    root = Path("/host/sys/class/net")
    ignored = ("lo", "cni", "flannel", "veth", "docker", "br-")
    result = []
    for path in sorted(root.iterdir() if root.is_dir() else []):
        if path.name.startswith(ignored):
            continue
        speed = read_text(path / "speed", "-1")
        result.append({
            "name": path.name,
            "operstate": read_text(path / "operstate", "unknown"),
            "speed_mbps": int(speed) if speed.lstrip("-").isdigit() else -1,
        })
    return result


def host_disk() -> dict[str, Any]:
    try:
        stat = os.statvfs("/host/data")
        return {
            "path": "/home/data", "observed": True,
            "total_gib": round(stat.f_blocks * stat.f_frsize / 1024**3, 3),
            "free_gib": round(stat.f_bavail * stat.f_frsize / 1024**3, 3),
        }
    except OSError as error:
        return {"path": "/home/data", "observed": False, "error": str(error)}


def tls_certificate(dns: str, port: int) -> dict[str, Any]:
    try:
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
        with socket.create_connection((dns, port), timeout=3) as raw:
            with context.wrap_socket(raw, server_hostname=dns) as secured:
                der = secured.getpeercert(binary_form=True)
        pem = ssl.DER_cert_to_PEM_cert(der)
        with tempfile.NamedTemporaryFile("w", suffix=".pem", dir="/tmp") as handle:
            handle.write(pem)
            handle.flush()
            decoded = ssl._ssl._test_decode_cert(handle.name)
        expires = datetime.strptime(decoded["notAfter"], "%b %d %H:%M:%S %Y %Z").replace(tzinfo=timezone.utc)
        return {
            "status": "PASS", "not_after": expires.isoformat(),
            "days_remaining": round((expires - datetime.now(timezone.utc)).total_seconds() / 86400, 3),
            "subject_alt_names": [value for kind, value in decoded.get("subjectAltName", []) if kind == "DNS"],
        }
    except Exception as error:
        return {"status": "BLOCKED", "error": f"{type(error).__name__}: {error}"}


def dependency_probe(item: dict[str, Any]) -> dict[str, Any]:
    dns, port = item["dns"], item["port"]
    try:
        addresses = sorted({entry[4][0] for entry in socket.getaddrinfo(dns, port, type=socket.SOCK_STREAM)})
        dns_status = "PASS"
    except OSError as error:
        addresses, dns_status = [], "BLOCKED"
        dns_error = str(error)
    try:
        with socket.create_connection((dns, port), timeout=3):
            tcp_status, tcp_error = "PASS", None
    except OSError as error:
        tcp_status, tcp_error = "BLOCKED", str(error)
    return {
        "id": item["id"], "dns": dns, "port": port, "tls": item["tls"],
        "dns_status": dns_status, "addresses": addresses,
        "dns_error": dns_error if dns_status != "PASS" else None,
        "tcp_status": tcp_status, "tcp_error": tcp_error,
        "certificate": tls_certificate(dns, port) if item["tls"] and tcp_status == "PASS" else None,
    }


def probe_node(site_values: Path) -> dict[str, Any]:
    values = site_validator.load(site_values)
    validation_errors = site_validator.validate_site_values(values)
    if validation_errors:
        return {"status": "FAIL", "validation_errors": validation_errors}
    return {
        "status": "PASS",
        "node": os.environ.get("NODE_NAME", "unknown"),
        "observed_at_epoch_ms": int(time.time() * 1000),
        "cpu": {"logical_processors": host_cpu_count()},
        "memory": {"total_gib": host_memory_gib()},
        "numa": {"nodes": host_numa_nodes()},
        "nics": host_nics(),
        "disk": host_disk(),
        "clock": {
            "clocksource": read_text(Path("/host/sys/devices/system/clocksource/clocksource0/current_clocksource"), "unknown")
        },
        "dependencies": [dependency_probe(item) for item in values["site"]["externalDependencies"]],
    }


def validate_inputs(image: str, run_id: str, timeout: int) -> str:
    if not IMAGE_RE.fullmatch(image) or image.endswith(":latest"):
        raise PreflightError("image must be an explicit non-latest reference")
    if timeout < 30 or timeout > 900:
        raise PreflightError("timeout must be between 30 and 900 seconds")
    parsed = uuid.UUID(run_id)
    if str(parsed) != run_id:
        raise PreflightError("run-id must be a canonical lowercase UUID")
    return parsed.hex[:10]


def cluster_nodes() -> list[str]:
    body = json.loads(kubectl("get", "nodes", "-o", "json").stdout)
    nodes = sorted(item["metadata"]["name"] for item in body["items"])
    if not nodes or any(not NODE_RE.fullmatch(node) for node in nodes):
        raise PreflightError("cluster node inventory is empty or invalid")
    return nodes


def objects(name: str, image: str, run_id: str, nodes: list[str]) -> list[dict[str, Any]]:
    labels = {"app.kubernetes.io/name": "m10-site-preflight", "traffic.analysis/canary-run": run_id}
    result: list[dict[str, Any]] = [{
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": name, "namespace": NAMESPACE, "labels": labels},
        "data": {"site-values.yaml": (ROOT / INPUT).read_text(encoding="utf-8")},
    }]
    for index, node in enumerate(nodes):
        job_name = f"{name}-{index}"
        result.append({
            "apiVersion": "batch/v1", "kind": "Job",
            "metadata": {
                "name": job_name, "namespace": NAMESPACE, "labels": labels,
                "annotations": {
                    "traffic.analysis/shared-infrastructure-touched": "false",
                    "traffic.analysis/production-applied": "false",
                    "traffic.analysis/host-mounts": "read-only",
                },
            },
            "spec": {"backoffLimit": 0, "ttlSecondsAfterFinished": 300, "template": {
                "metadata": {"labels": labels},
                "spec": {
                    "nodeName": node, "automountServiceAccountToken": False,
                    "restartPolicy": "Never",
                    "securityContext": {
                        "runAsNonRoot": True, "runAsUser": 65532, "runAsGroup": 65532,
                        "fsGroup": 65532, "seccompProfile": {"type": "RuntimeDefault"},
                    },
                    "containers": [{
                        "name": "preflight", "image": image, "imagePullPolicy": "Never",
                        "args": ["--probe", "--site-values", "/workspace/site-values.yaml"],
                        "env": [{"name": "NODE_NAME", "value": node}],
                        "resources": {
                            "requests": {"cpu": "25m", "memory": "32Mi"},
                            "limits": {"cpu": "250m", "memory": "128Mi"},
                        },
                        "securityContext": {
                            "allowPrivilegeEscalation": False, "readOnlyRootFilesystem": True,
                            "capabilities": {"drop": ["ALL"]},
                        },
                        "volumeMounts": [
                            {"name": "workspace", "mountPath": "/workspace", "readOnly": True},
                            {"name": "host-proc", "mountPath": "/host/proc", "readOnly": True},
                            {"name": "host-sys", "mountPath": "/host/sys", "readOnly": True},
                            {"name": "host-data", "mountPath": "/host/data", "readOnly": True},
                            {"name": "tmp", "mountPath": "/tmp"},
                        ],
                    }],
                    "volumes": [
                        {"name": "workspace", "configMap": {"name": name}},
                        {"name": "host-proc", "hostPath": {"path": "/proc", "type": "Directory"}},
                        {"name": "host-sys", "hostPath": {"path": "/sys", "type": "Directory"}},
                        {"name": "host-data", "hostPath": {"path": "/home/data", "type": "Directory"}},
                        {"name": "tmp", "emptyDir": {}},
                    ],
                },
            }},
        })
    return result


def wait_job(name: str, timeout: int) -> tuple[dict[str, Any], dict[str, Any]]:
    deadline = time.time() + timeout
    job: dict[str, Any] = {}
    while time.time() < deadline:
        response = kubectl("get", "job", name, "-n", NAMESPACE, "-o", "json", check=False)
        if response.returncode == 0:
            job = json.loads(response.stdout)
            if job.get("status", {}).get("succeeded") == 1:
                break
            if job.get("status", {}).get("failed", 0):
                logs = kubectl("logs", "-n", NAMESPACE, f"job/{name}", check=False)
                raise PreflightError(f"node preflight failed:\n{logs.stdout}{logs.stderr}")
        time.sleep(1)
    else:
        raise PreflightError(f"timed out waiting for {name}")
    pods = json.loads(kubectl("get", "pod", "-n", NAMESPACE, "-l", f"job-name={name}", "-o", "json").stdout)["items"]
    if len(pods) != 1:
        raise PreflightError(f"expected one pod for {name}")
    pod = pods[0]
    logs = kubectl("logs", "-n", NAMESPACE, pod["metadata"]["name"]).stdout.strip()
    receipt = json.loads(logs.splitlines()[-1])
    if receipt.get("status") != "PASS":
        raise PreflightError(f"invalid node receipt: {receipt}")
    container = pod["status"]["containerStatuses"][0]
    return receipt, {
        "job_name": name, "job_uid": job["metadata"]["uid"],
        "pod_name": pod["metadata"]["name"], "pod_uid": pod["metadata"]["uid"],
        "node": pod["spec"]["nodeName"], "image": container.get("image"),
        "image_id": container.get("imageID"), "container_id": container.get("containerID"),
        "started_at": pod.get("status", {}).get("startTime"),
        "completed_at": job.get("status", {}).get("completionTime"),
    }


def rbac_checks() -> list[dict[str, Any]]:
    checks = []
    for verb, resource, namespace in (
        ("get", "nodes", None), ("get", "secrets", "traffic-analysis"),
        ("create", "deployments.apps", "traffic-analysis"),
        ("patch", "deployments.apps", "traffic-analysis"),
        ("delete", "jobs.batch", "traffic-analysis"),
    ):
        args = ["auth", "can-i", verb, resource]
        if namespace:
            args.extend(["-n", namespace])
        response = kubectl(*args, check=False)
        checks.append({"verb": verb, "resource": resource, "namespace": namespace, "allowed": response.returncode == 0 and response.stdout.strip() == "yes"})
    return checks


def secret_ref_checks(values: dict[str, Any]) -> list[dict[str, Any]]:
    refs: set[tuple[str, str, str]] = set()
    network = values["site"]["network"]["caBundleSecretRef"]
    refs.add((network["namespace"], network["name"], network["key"]))
    for dependency in values["site"]["externalDependencies"]:
        if dependency["caSecretRef"]:
            ref = dependency["caSecretRef"]
            refs.add((ref["namespace"], ref["name"], ref["key"]))
    for ref in values["secretRefs"]["required"]:
        refs.update((ref["namespace"], ref["name"], key) for key in ref["keys"])
    checks = []
    template = '{{range $k,$v := .data}}{{$k}}{{"\\n"}}{{end}}'
    for namespace, name, key in sorted(refs):
        response = kubectl("get", "secret", name, "-n", namespace, "-o", f"go-template={template}", check=False)
        keys = set(response.stdout.splitlines()) if response.returncode == 0 else set()
        checks.append({"namespace": namespace, "name": name, "key": key, "secret_exists": response.returncode == 0, "key_exists": key in keys})
    return checks


def evaluate(values: dict[str, Any], probes: list[dict[str, Any]], rbac: list[dict[str, Any]], secrets: list[dict[str, Any]]) -> dict[str, Any]:
    blockers: list[str] = []
    warnings: list[str] = []
    required_nodes = values["site"]["cluster"]["minNodes"]
    if len(probes) < required_nodes:
        blockers.append("NODE_COUNT_BELOW_SITE_MINIMUM")
    total_cpu = sum(item["cpu"]["logical_processors"] for item in probes)
    total_memory = sum(item["memory"]["total_gib"] for item in probes)
    total_free_disk = sum(item["disk"].get("free_gib", 0) for item in probes)
    if total_cpu < values["site"]["quota"]["cpuCores"]:
        blockers.append("CPU_CAPACITY_BELOW_SITE_QUOTA")
    if total_memory < values["site"]["quota"]["memoryGi"]:
        blockers.append("MEMORY_CAPACITY_BELOW_SITE_QUOTA")
    if total_free_disk < values["site"]["quota"]["storageGi"]:
        blockers.append("DISK_CAPACITY_BELOW_SITE_QUOTA")
    if any(not item["numa"]["nodes"] for item in probes):
        blockers.append("NUMA_TOPOLOGY_UNOBSERVED")
    fastest = [max((nic["speed_mbps"] for nic in item["nics"] if nic["operstate"] == "up"), default=-1) for item in probes]
    if any(speed < 0 for speed in fastest):
        blockers.append("ACTIVE_NIC_SPEED_UNOBSERVED")
    elif sum(fastest) < values["site"]["quota"]["ingressMbps"]:
        blockers.append("NIC_CAPACITY_BELOW_SITE_QUOTA")
    clocks = [item["observed_at_epoch_ms"] for item in probes]
    clock_skew_ms = max(clocks) - min(clocks) if clocks else 0
    if clock_skew_ms > 2000:
        blockers.append("NODE_CLOCK_SKEW_EXCEEDS_2S")
    dependency_results = [dependency for probe in probes for dependency in probe["dependencies"]]
    if any(item["dns_status"] != "PASS" for item in dependency_results):
        blockers.append("DEPENDENCY_DNS_UNRESOLVED")
    if any(item["tcp_status"] != "PASS" for item in dependency_results):
        blockers.append("DEPENDENCY_TCP_UNREACHABLE")
    certificates = [item["certificate"] for item in dependency_results if item["tls"]]
    if any(not item or item.get("status") != "PASS" for item in certificates):
        blockers.append("TLS_CERTIFICATE_UNOBSERVED")
    elif any(item.get("days_remaining", 0) < 30 for item in certificates):
        blockers.append("TLS_CERTIFICATE_EXPIRES_WITHIN_30_DAYS")
    if any(not item["allowed"] for item in rbac):
        blockers.append("DEPLOYMENT_RBAC_INSUFFICIENT")
    if any(not item["secret_exists"] or not item["key_exists"] for item in secrets):
        blockers.append("SECRET_OR_CA_REFERENCE_MISSING")
    return {
        "status": "PASS" if not blockers else "BLOCKED",
        "blocking_codes": sorted(set(blockers)), "warnings": sorted(set(warnings)),
        "capacity": {
            "nodes": len(probes), "logical_processors": total_cpu,
            "memory_gib": round(total_memory, 3), "free_disk_gib": round(total_free_disk, 3),
            "fastest_active_nic_mbps_by_node": fastest, "clock_skew_ms": clock_skew_ms,
        },
    }


def cleanup(run_id: str) -> None:
    kubectl("delete", "job,configmap", "-n", NAMESPACE, "-l", f"traffic.analysis/canary-run={run_id}", "--ignore-not-found=true", "--wait=true", "--timeout=120s", check=False)


def orchestrate(args: argparse.Namespace) -> None:
    suffix = validate_inputs(args.image, args.run_id, args.timeout)
    nodes = cluster_nodes()
    name = f"m10-n004-preflight-{suffix}"
    resources = objects(name, args.image, args.run_id, nodes)
    try:
        kubectl("apply", "-f", "-", input_text=yaml.safe_dump_all(resources, sort_keys=False))
        receipts, identities = [], []
        for index in range(len(nodes)):
            receipt, identity = wait_job(f"{name}-{index}", args.timeout)
            receipts.append(receipt)
            identities.append(identity)
        values = site_validator.load(ROOT / INPUT)
        rbac = rbac_checks()
        secrets = secret_ref_checks(values)
        evaluation = evaluate(values, receipts, rbac, secrets)
    finally:
        if not args.keep:
            cleanup(args.run_id)
    evidence = {
        "artifact_kind": "M10_SITE_PREFLIGHT_RESULT", "task_id": "T1-M10-N004",
        "run_id": args.run_id, "status": "PASS", "engineering_status": "PASS",
        "acceptance_status": evaluation["status"],
        "profile_id": "M10-N004-K8S-READONLY-SITE-PREFLIGHT-V1",
        "candidate_id": None, "environment_kind": "KUBERNETES",
        "site_values_sha256": sha256(ROOT / INPUT),
        "source_sha256": {str(path): sha256(ROOT / path) for path in SOURCE_FILES},
        "nodes": receipts, "kubernetes_jobs": identities,
        "rbac_checks": rbac, "secret_ref_checks": secrets, "evaluation": evaluation,
        "required_gates": {"G0": "BLOCKED_BY_N002", "G6": evaluation["status"]},
        "shared_infrastructure_touched": False, "production_applied": False,
        "run_scoped_resources_removed": not args.keep,
        "allowed_claims": ["The declared read-only site preflight executed across every current Kubernetes node"],
        "does_not_prove": [
            "the application candidate was deployed", "performance or HA acceptance passed",
            "the site is promotion-ready", "production promotion is authorized",
        ],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_name(f".{args.output.name}.tmp")
    temporary.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(args.output)
    print(json.dumps(evidence, sort_keys=True))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--probe", action="store_true")
    parser.add_argument("--site-values", type=Path)
    parser.add_argument("--image")
    parser.add_argument("--run-id", default=str(uuid.uuid4()))
    parser.add_argument("--timeout", type=int, default=240)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--keep", action="store_true")
    args = parser.parse_args()
    if args.probe:
        if args.site_values is None:
            raise SystemExit("--site-values is required in probe mode")
        result = probe_node(args.site_values)
        print(json.dumps(result, sort_keys=True))
        return 0 if result["status"] == "PASS" else 1
    if not args.image:
        raise SystemExit("--image is required in orchestrator mode")
    orchestrate(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
