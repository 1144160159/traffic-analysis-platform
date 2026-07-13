#!/usr/bin/env python3
"""Audit Kubernetes readiness for remote-pool multi-pod stress execution."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text


ALLOW_ENV = "CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS"
QUEUE_TOKEN_ENV = "CODEX_LOOP_QUEUE_TOKEN"


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def env_without_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def read_json(path: str | None) -> dict[str, Any]:
    if not path:
        return {}
    target = repo_path(path)
    if not target.exists():
        return {"missing": True, "path": rel_path(target)}
    try:
        return json.loads(target.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return {"parse_error": str(exc), "path": rel_path(target)}


def run_command(command: list[str], timeout: int = 60) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            env=env_without_proxies(),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
        )
        return {
            "command": command,
            "exit_code": proc.returncode,
            "timed_out": False,
            "output": proc.stdout,
            "output_tail": proc.stdout[-4000:],
        }
    except subprocess.TimeoutExpired as exc:
        output = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
        return {
            "command": command,
            "exit_code": None,
            "timed_out": True,
            "output": output,
            "output_tail": output[-4000:],
        }


def kubectl_json(args: list[str], timeout: int = 60) -> dict[str, Any]:
    result = run_command(["kubectl", *args, "-o", "json"], timeout=timeout)
    data: dict[str, Any] = {}
    if result["exit_code"] == 0 and result["output"].strip():
        try:
            data = json.loads(result["output"])
        except json.JSONDecodeError as exc:
            result["parse_error"] = str(exc)
    return {"result": result, "data": data}


def redact_secret(data: dict[str, Any]) -> dict[str, Any]:
    if not data:
        return {}
    redacted = json.loads(json.dumps(data))
    if isinstance(redacted.get("data"), dict):
        redacted["data"] = {key: "<redacted>" for key in redacted["data"]}
    if isinstance(redacted.get("stringData"), dict):
        redacted["stringData"] = {key: "<redacted>" for key in redacted["stringData"]}
    return redacted


def add_finding(findings: list[dict[str, str]], level: str, code: str, message: str) -> None:
    findings.append({"level": level, "code": code, "message": message})


def status_for(findings: list[dict[str, str]]) -> str:
    if any(item["level"] == "blocker" for item in findings):
        return "REMOTE_POOL_K8S_READINESS_BLOCKED"
    if findings:
        return "REMOTE_POOL_K8S_READINESS_DEGRADED"
    return "REMOTE_POOL_K8S_READINESS_READY"


def service_name_from_url(service_url: str) -> str | None:
    host = urlparse(service_url).hostname or ""
    if host.endswith(".svc") or ".svc." in host:
        return host.split(".")[0]
    return None


def service_port_from_url(service_url: str) -> int | None:
    try:
        return urlparse(service_url).port
    except ValueError:
        return None


def manifest_path(args: argparse.Namespace, stress_summary: dict[str, Any]) -> Path:
    if args.worker_job:
        return repo_path(args.worker_job)
    if args.stress_summary:
        summary_path = repo_path(args.stress_summary)
        candidate = summary_path.parent / "remote-pool-worker-job.yaml"
        if candidate.exists():
            return candidate
    for output in stress_summary.get("outputs", []):
        if output.endswith("remote-pool-worker-job.yaml"):
            return repo_path(Path(args.stress_summary or ".").parent / Path(output).name)
    return repo_path("doc/02_acceptance/runs/unknown/remote-pool-k8s-stress/remote-pool-worker-job.yaml")


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Remote Pool K8s Readiness",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- namespace: `{summary['namespace']}`",
        f"- service_name: `{summary['service_name']}`",
        f"- service_url: `{summary['service_url']}`",
        f"- pvc_name: `{summary['pvc_name']}`",
        f"- secret_name: `{summary['secret_name']}`",
        f"- token_key_present: `{((summary.get('secret') or {}).get('token_key_present'))}`",
        f"- worker_job: `{summary['worker_job']}`",
        f"- execute_gate_present: `{(summary.get('execute_gate') or {}).get('present')}`",
        f"- execute_gate_accepted: `{(summary.get('execute_gate') or {}).get('accepted')}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Outputs",
        ]
    )
    for item in summary.get("outputs", []):
        lines.append(f"- `{item}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This audit is read-only and does not create, patch, delete, or execute Kubernetes workloads.",
            "- Secret values are never written to evidence; only key presence is recorded.",
            "- READY means the remote-pool K8s stress can be executed after the explicit execution gate is accepted.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--stress-summary", default=None)
    parser.add_argument("--service-url", default=None)
    parser.add_argument("--namespace", default=None)
    parser.add_argument("--service-name", default=None)
    parser.add_argument("--service-port", type=int, default=None)
    parser.add_argument("--pvc-name", default=None)
    parser.add_argument("--secret-name", default=None)
    parser.add_argument("--token-key", default=None)
    parser.add_argument("--worker-job", default=None)
    parser.add_argument("--require-execute-gate", action="store_true")
    parser.add_argument("--require-local-token", action="store_true")
    args = parser.parse_args()

    stress_summary = read_json(args.stress_summary)
    service_url = (args.service_url or stress_summary.get("service_url") or "http://codex-loop-queue-service.traffic-analysis.svc:8765").rstrip("/")
    namespace = args.namespace or stress_summary.get("namespace") or "traffic-analysis"
    service_name = args.service_name or service_name_from_url(service_url) or "codex-loop-queue-service"
    service_port = args.service_port or service_port_from_url(service_url)
    pvc_name = args.pvc_name or stress_summary.get("pvc_name") or "codex-loop-workspace"
    secret_name = args.secret_name or stress_summary.get("secret_name") or "codex-loop-queue-token"
    token_key = args.token_key or stress_summary.get("token_key") or "token"
    worker_job = manifest_path(args, stress_summary)
    run_id = args.run_id or make_run_id("remote-pool-k8s-readiness")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "remote-pool-k8s-readiness"
    out_dir.mkdir(parents=True, exist_ok=True)

    findings: list[dict[str, str]] = []
    checks: dict[str, Any] = {}
    if stress_summary.get("missing"):
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_STRESS_SUMMARY_MISSING", f"Stress summary is missing: {stress_summary.get('path')}.")
    elif stress_summary.get("parse_error"):
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_STRESS_SUMMARY_PARSE_FAILED", f"Stress summary could not be parsed: {stress_summary.get('parse_error')}.")
    elif args.stress_summary and stress_summary.get("status") == "REMOTE_POOL_K8S_STRESS_BLOCKED":
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_STRESS_BLOCKED", "Stress summary has blocker findings.")
    elif args.stress_summary and stress_summary.get("status") not in {"REMOTE_POOL_K8S_STRESS_PLANNED", "REMOTE_POOL_K8S_STRESS_VALIDATED", "REMOTE_POOL_K8S_STRESS_COMPLETED"}:
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_STRESS_NOT_READY", f"Stress summary status is {stress_summary.get('status')}.")

    kubectl_path = shutil.which("kubectl")
    checks["kubectl"] = {"available": bool(kubectl_path), "path": kubectl_path}
    if not kubectl_path:
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_KUBECTL_MISSING", "kubectl is not available on PATH.")

    namespace_data: dict[str, Any] = {}
    service_data: dict[str, Any] = {}
    pvc_data: dict[str, Any] = {}
    secret_data: dict[str, Any] = {}
    dry_run: dict[str, Any] | None = None

    if kubectl_path:
        version = run_command(["kubectl", "version", "--client=true"], timeout=30)
        checks["kubectl"]["client_version"] = {key: value for key, value in version.items() if key != "output"}
        if version["exit_code"] != 0:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_KUBECTL_CLIENT_FAILED", f"kubectl client check exited {version['exit_code']}.")

        namespace_check = kubectl_json(["get", "namespace", namespace], timeout=30)
        namespace_data = namespace_check["data"]
        checks["namespace"] = {"exit_code": namespace_check["result"]["exit_code"], "timed_out": namespace_check["result"]["timed_out"], "output_tail": namespace_check["result"]["output_tail"], "name": namespace_data.get("metadata", {}).get("name")}
        if namespace_check["result"]["exit_code"] != 0:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_NAMESPACE_MISSING", f"Namespace {namespace} is not readable.")

        service_check = kubectl_json(["-n", namespace, "get", "service", service_name], timeout=30)
        service_data = service_check["data"]
        service_ports = service_data.get("spec", {}).get("ports") or []
        port_values = [item.get("port") for item in service_ports]
        checks["service"] = {"exit_code": service_check["result"]["exit_code"], "timed_out": service_check["result"]["timed_out"], "output_tail": service_check["result"]["output_tail"], "name": service_data.get("metadata", {}).get("name"), "cluster_ip": service_data.get("spec", {}).get("clusterIP"), "ports": service_ports}
        if service_check["result"]["exit_code"] != 0:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_SERVICE_MISSING", f"Service {namespace}/{service_name} is not readable.")
        elif service_port is not None and service_port not in port_values:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_SERVICE_PORT_MISMATCH", f"Service {namespace}/{service_name} does not expose port {service_port}; ports={port_values}.")

        pvc_check = kubectl_json(["-n", namespace, "get", "pvc", pvc_name], timeout=30)
        pvc_data = pvc_check["data"]
        pvc_phase = pvc_data.get("status", {}).get("phase")
        checks["pvc"] = {"exit_code": pvc_check["result"]["exit_code"], "timed_out": pvc_check["result"]["timed_out"], "output_tail": pvc_check["result"]["output_tail"], "name": pvc_data.get("metadata", {}).get("name"), "phase": pvc_phase, "storage": pvc_data.get("spec", {}).get("resources", {}).get("requests", {}).get("storage")}
        if pvc_check["result"]["exit_code"] != 0:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_PVC_MISSING", f"PVC {namespace}/{pvc_name} is not readable.")
        elif pvc_phase != "Bound":
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_PVC_NOT_BOUND", f"PVC {namespace}/{pvc_name} phase is {pvc_phase}.")

        secret_check = kubectl_json(["-n", namespace, "get", "secret", secret_name], timeout=30)
        secret_data = redact_secret(secret_check["data"])
        raw_secret_data = secret_check["data"].get("data") or {}
        token_key_present = token_key in raw_secret_data and bool(raw_secret_data.get(token_key))
        secret_output_tail = secret_check["result"]["output_tail"] if secret_check["result"]["exit_code"] != 0 else "<redacted>"
        checks["secret"] = {"exit_code": secret_check["result"]["exit_code"], "timed_out": secret_check["result"]["timed_out"], "output_tail": secret_output_tail, "redacted": secret_data, "token_key_present": token_key_present}
        if secret_check["result"]["exit_code"] != 0:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_SECRET_MISSING", f"Secret {namespace}/{secret_name} is not readable.")
        elif not token_key_present:
            add_finding(findings, "blocker", "REMOTE_POOL_K8S_SECRET_TOKEN_KEY_MISSING", f"Secret {namespace}/{secret_name} does not contain non-empty key {token_key}.")

        if worker_job.exists():
            dry_run = run_command(["kubectl", "apply", "--dry-run=client", "--validate=false", "-f", str(worker_job)], timeout=120)
            checks["worker_job_dry_run"] = {key: value for key, value in dry_run.items() if key != "output"}
            write_text(out_dir / "kubectl-dry-run.txt", dry_run["output"])
            if dry_run["exit_code"] != 0:
                add_finding(findings, "blocker", "REMOTE_POOL_K8S_WORKER_JOB_DRY_RUN_FAILED", f"Worker Job dry-run exited {dry_run['exit_code']}.")

    if not worker_job.exists():
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_WORKER_JOB_MISSING", f"Worker Job manifest is missing: {rel_path(worker_job)}.")

    parsed_service = service_name_from_url(service_url)
    if parsed_service and parsed_service != service_name:
        add_finding(findings, "warning", "REMOTE_POOL_K8S_SERVICE_URL_NAME_MISMATCH", f"Service URL host points at {parsed_service}, but readiness checked {service_name}.")

    execute_gate = {"name": ALLOW_ENV, "present": ALLOW_ENV in os.environ, "accepted": os.environ.get(ALLOW_ENV) == "1"}
    if not execute_gate["accepted"]:
        level = "blocker" if args.require_execute_gate else "warning"
        add_finding(findings, level, "REMOTE_POOL_K8S_EXECUTE_GATE_NOT_ACCEPTED", f"{ALLOW_ENV}=1 is required before real K8s stress execution.")
    local_token = {"name": QUEUE_TOKEN_ENV, "present": QUEUE_TOKEN_ENV in os.environ, "accepted": bool(os.environ.get(QUEUE_TOKEN_ENV))}
    if args.require_local_token and not local_token["accepted"]:
        add_finding(findings, "blocker", "REMOTE_POOL_K8S_LOCAL_TOKEN_MISSING", f"{QUEUE_TOKEN_ENV} is required for local seed/finalize operations.")

    outputs = [
        "remote-pool-k8s-readiness/readiness-summary.json",
        "remote-pool-k8s-readiness/readiness-report.md",
        "remote-pool-k8s-readiness/kubectl-checks.json",
    ]
    if dry_run:
        outputs.append("remote-pool-k8s-readiness/kubectl-dry-run.txt")

    summary = {
        "run_id": run_id,
        "run_kind": "remote_pool_k8s_readiness",
        "status": status_for(findings),
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "stress_summary_path": rel_path(repo_path(args.stress_summary)) if args.stress_summary else None,
        "stress_summary_status": stress_summary.get("status"),
        "namespace": namespace,
        "service_url": service_url,
        "service_name": service_name,
        "service_port": service_port,
        "pvc_name": pvc_name,
        "secret_name": secret_name,
        "token_key": token_key,
        "worker_job": rel_path(worker_job),
        "execute_gate": execute_gate,
        "local_token": local_token,
        "namespace_resource": {"name": namespace_data.get("metadata", {}).get("name"), "phase": (namespace_data.get("status") or {}).get("phase")},
        "service": {"name": service_data.get("metadata", {}).get("name"), "cluster_ip": service_data.get("spec", {}).get("clusterIP"), "ports": service_data.get("spec", {}).get("ports") or []},
        "pvc": {"name": pvc_data.get("metadata", {}).get("name"), "phase": (pvc_data.get("status") or {}).get("phase"), "storage": pvc_data.get("spec", {}).get("resources", {}).get("requests", {}).get("storage")},
        "secret": {"name": secret_data.get("metadata", {}).get("name"), "type": secret_data.get("type"), "token_key_present": (checks.get("secret") or {}).get("token_key_present", False)},
        "worker_job_dry_run": {"exit_code": (dry_run or {}).get("exit_code"), "timed_out": (dry_run or {}).get("timed_out")} if dry_run else None,
        "checks": checks,
        "findings": findings,
        "outputs": outputs,
    }
    write_json(out_dir / "kubectl-checks.json", checks)
    write_json(out_dir / "readiness-summary.json", summary)
    write_text(out_dir / "readiness-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} findings={len(findings)}")
    return 2 if summary["status"] == "REMOTE_POOL_K8S_READINESS_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
