#!/usr/bin/env python3
"""Generate deployable Codex Loop supervisor manifests without applying them."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_PROFILE = "conservative"
DEFAULT_PROFILES = SCRIPT_ROOT / "policies" / "resource-profiles.yaml"
SYSTEMD_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop.service"
CRONJOB_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-cronjob.yaml"
QUEUE_SERVICE_DEPLOYMENT_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-queue-service-deployment.yaml"
QUEUE_SERVICE_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-queue-service.yaml"
PV_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-pv.yaml"
PVC_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-pvc.yaml"
KUSTOMIZATION_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-kustomization.yaml"


def load_profiles(path: Path) -> dict[str, Any]:
    data = load_yaml_subset(path)
    profiles = data.get("profiles") or {}
    if not isinstance(profiles, dict):
        raise ValueError(f"Invalid profiles file: {path}")
    return profiles


def selected_profile(name: str, profiles_path: Path) -> dict[str, Any]:
    profiles = load_profiles(profiles_path)
    profile = profiles.get(name)
    if not isinstance(profile, dict):
        raise KeyError(f"Unknown resource profile `{name}` in {profiles_path}")
    return profile


def validate_profile(profile: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    queue_backend = str(profile.get("queue_backend") or "repo-json")
    max_workers = int(profile.get("max_concurrent_workers") or 0)
    executor_entry = str(profile.get("executor_entry") or "service")
    runner = str(profile.get("runner") or "workflow")
    create_worktrees = profile.get("create_worktrees") is True
    activate_workspaces = profile.get("activate_workspaces") is True
    if executor_entry not in {"service", "executor_pool", "queue_service"}:
        findings.append({"level": "blocker", "code": "UNKNOWN_EXECUTOR_ENTRY", "message": f"Unsupported executor_entry `{executor_entry}`."})
    if max_workers != 1 and not (executor_entry == "executor_pool" and runner == "sandbox-plan"):
        findings.append({"level": "blocker", "code": "CONCURRENCY_NOT_ISOLATED", "message": "Only executor_pool sandbox-plan profiles may set max_concurrent_workers > 1."})
    if max_workers > 1 and queue_backend != "sqlite":
        findings.append({"level": "blocker", "code": "POOL_REQUIRES_SQLITE", "message": "Executor pool profiles with max_concurrent_workers > 1 must use sqlite queue backend."})
    if max_workers > 1 and not profile.get("workspace_isolation_policy"):
        findings.append({"level": "blocker", "code": "WORKSPACE_ISOLATION_POLICY_REQUIRED", "message": "Executor pool profiles with max_concurrent_workers > 1 must set workspace_isolation_policy."})
    if activate_workspaces and executor_entry != "executor_pool":
        findings.append({"level": "blocker", "code": "WORKSPACE_ACTIVATION_REQUIRES_POOL", "message": "activate_workspaces is only supported for executor_pool profiles."})
    if activate_workspaces and not create_worktrees:
        findings.append({"level": "blocker", "code": "WORKSPACE_ACTIVATION_REQUIRES_CREATE", "message": "activate_workspaces requires create_worktrees because worktree paths are run_id scoped."})
    if executor_entry == "queue_service" and queue_backend != "sqlite":
        findings.append({"level": "blocker", "code": "QUEUE_SERVICE_REQUIRES_SQLITE", "message": "Queue service profiles must use sqlite queue backend."})
    if executor_entry == "queue_service":
        k8s_host = str(profile.get("queue_service_k8s_host") or "0.0.0.0")
        token_env = str(profile.get("queue_service_auth_token_env") or "")
        if k8s_host not in {"127.0.0.1", "localhost", "::1"} and not token_env:
            findings.append({"level": "blocker", "code": "QUEUE_SERVICE_TOKEN_ENV_REQUIRED", "message": "Queue service profiles exposed beyond loopback require queue_service_auth_token_env."})
    if str(profile.get("worker_stage")) != "prepare":
        findings.append({"level": "blocker", "code": "UNSAFE_DEFAULT_STAGE", "message": "Production deployment profile must default to prepare."})
    if profile.get("allow_live_write") is True:
        findings.append({"level": "blocker", "code": "LIVE_WRITE_ENABLED", "message": "Deployment profile cannot enable live writes by default."})
    if profile.get("allow_external_codex") is True:
        findings.append({"level": "blocker", "code": "EXTERNAL_CODEX_ENABLED", "message": "Deployment profile cannot enable external Codex by default."})
    if queue_backend not in {"repo-json", "sqlite", "http"}:
        findings.append({"level": "blocker", "code": "UNKNOWN_QUEUE_BACKEND", "message": f"Unsupported queue backend `{queue_backend}`."})
    if queue_backend == "http":
        queue_path = str(profile.get("queue_path") or "")
        if not queue_path.startswith(("http://", "https://")):
            findings.append({"level": "blocker", "code": "HTTP_QUEUE_URL_INVALID", "message": "HTTP queue backend profiles must set queue_path to an http(s) URL."})
    return findings


def render_template(path: Path, variables: dict[str, Any]) -> str:
    text = path.read_text(encoding="utf-8")
    return text.format(**variables)


def yaml_command(args: list[str]) -> str:
    return "\n".join(f'                - "{item}"' for item in args)


def state_path(path: str | None, state_root: str) -> str | None:
    if not path:
        return None
    if path.startswith(("http://", "https://", "/")):
        return path
    if path.startswith("doc/02_acceptance/runs/"):
        return f"{state_root.rstrip('/')}/{path}"
    return path


def kustomization_resources(resources: list[str]) -> str:
    return "\n".join(f"  - {item}" for item in resources)


def queue_token_env_yaml(enabled: bool) -> str:
    if not enabled:
        return ""
    return "\n".join(
        [
            "                - name: CODEX_LOOP_QUEUE_TOKEN",
            "                  valueFrom:",
            "                    secretKeyRef:",
            "                      name: codex-loop-queue-token",
            "                      key: token",
        ]
    )


def build_variables(profile: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    queue_path = profile.get("queue_path")
    queue_path_arg = f" --queue-path {queue_path}" if queue_path else ""
    app_root = str(args.app_root).rstrip("/") or "/app"
    state_root = str(args.state_root).rstrip("/") or "/workspace"
    runs_root = f"{state_root}/doc/02_acceptance/runs"
    k8s_queue_path = state_path(str(queue_path) if queue_path else None, state_root)
    executor_entry = str(profile.get("executor_entry") or "service")
    runner = str(profile.get("runner") or "workflow")
    max_workers = int(profile.get("max_concurrent_workers") or 1)
    queue_backend = str(profile.get("queue_backend") or "repo-json")
    quota_policy = str(profile.get("quota_policy") or "scripts/codex_loop/policies/resource-quotas.yaml")
    workspace_isolation_policy = str(profile.get("workspace_isolation_policy") or "scripts/codex_loop/policies/workspace-isolation.yaml")
    create_worktrees = profile.get("create_worktrees") is True
    activate_workspaces = profile.get("activate_workspaces") is True
    workspace_flags = ""
    if create_worktrees:
        workspace_flags += " --create-worktrees"
    if activate_workspaces:
        workspace_flags += " --activate-workspaces"
    exec_stop = f"{sys.executable} -B {repo_path('.')}/scripts/codex_loop/service.py stop"
    k8s_resources = ["codex-loop-pv.yaml", "codex-loop-pvc.yaml", "codex-loop-cronjob.yaml"]
    queue_service_port = int(profile.get("queue_service_port") or 8765)
    if executor_entry == "queue_service":
        image_layout = args.image_layout if args.image_layout != "auto" else "control-only"
        host = str(profile.get("queue_service_host") or "127.0.0.1")
        k8s_host = str(profile.get("queue_service_k8s_host") or "0.0.0.0")
        auth_token_env = str(profile.get("queue_service_auth_token_env") or "CODEX_LOOP_QUEUE_TOKEN")
        systemd_exec = (
            f"{sys.executable} -B {repo_path('.')}/scripts/codex_loop/queue_service.py serve"
            f" --host {host}"
            f" --port {queue_service_port}"
            f" --queue-backend {queue_backend}"
            f" --auth-token-env {auth_token_env}"
            f"{queue_path_arg}"
            f" --quiet"
        )
        k8s_args = [
            "python",
            "-B",
            "scripts/codex_loop/queue_service.py",
            "serve",
            "--host",
            k8s_host,
            "--port",
            str(queue_service_port),
            "--queue-backend",
            queue_backend,
            "--auth-token-env",
            auth_token_env,
            "--quiet",
        ]
        if k8s_queue_path:
            k8s_args.extend(["--queue-path", str(k8s_queue_path)])
        exec_stop = "/bin/kill -TERM $MAINPID"
        k8s_resources = ["codex-loop-pv.yaml", "codex-loop-pvc.yaml", "codex-loop-queue-service-deployment.yaml", "codex-loop-queue-service.yaml"]
    elif executor_entry == "executor_pool":
        image_layout = args.image_layout if args.image_layout != "auto" else "full-repo"
        systemd_exec = (
            f"{sys.executable} -B {repo_path('.')}/scripts/codex_loop/executor_pool.py"
            f" --runner {runner}"
            f" --max-workers {max_workers}"
            f" --max-tasks {int(profile.get('max_items_per_cycle') or 1)}"
            f" --stage {str(profile.get('worker_stage') or 'prepare')}"
            f" --queue-backend {queue_backend}"
            f" --quota-policy {quota_policy}"
            f" --workspace-isolation-policy {workspace_isolation_policy}"
            f"{workspace_flags}"
            f"{queue_path_arg}"
        )
        k8s_args = [
            "python",
            "-B",
            "scripts/codex_loop/executor_pool.py",
            "--runner",
            runner,
            "--max-workers",
            str(max_workers),
            "--max-tasks",
            str(int(profile.get("max_items_per_cycle") or 1)),
            "--stage",
            str(profile.get("worker_stage") or "prepare"),
            "--queue-backend",
            queue_backend,
            "--quota-policy",
            quota_policy,
            "--workspace-isolation-policy",
            workspace_isolation_policy,
        ]
        if create_worktrees:
            k8s_args.append("--create-worktrees")
        if activate_workspaces:
            k8s_args.append("--activate-workspaces")
        if k8s_queue_path:
            k8s_args.extend(["--queue-path", str(k8s_queue_path)])
    else:
        image_layout = args.image_layout if args.image_layout != "auto" else "full-repo"
        systemd_exec = (
            f"{sys.executable} -B {repo_path('.')}/scripts/codex_loop/service.py run"
            f" --interval-seconds {int(profile.get('interval_seconds') or 300)}"
            f" --max-cycles 0"
            f" --max-failures {int(profile.get('max_failures') or 3)}"
            f" --max-items {int(profile.get('max_items_per_cycle') or 1)}"
            f" --worker-stage {str(profile.get('worker_stage') or 'prepare')}"
            f" --lease-seconds {int(profile.get('lease_seconds') or 1800)}"
            f" --queue-backend {queue_backend}"
            f" --profile {args.profile}"
            f"{queue_path_arg}"
        )
        k8s_args = [
            "python",
            "-B",
            "scripts/codex_loop/service.py",
            "once",
            "--max-items",
            str(int(profile.get("max_items_per_cycle") or 1)),
            "--worker-stage",
            str(profile.get("worker_stage") or "prepare"),
            "--lease-seconds",
            str(int(profile.get("lease_seconds") or 1800)),
            "--queue-backend",
            queue_backend,
            "--profile",
            args.profile,
        ]
        if k8s_queue_path:
            k8s_args.extend(["--queue-path", str(k8s_queue_path)])
    queue_path_yaml = ""
    if queue_path:
        queue_path_yaml = f'                - --queue-path\n                - "{queue_path}"'
    return {
        "repo_root": str(repo_path(".")),
        "app_root": app_root,
        "state_root": state_root,
        "runs_root": runs_root,
        "script_root": f"{app_root}/scripts/codex_loop",
        "profile_name": args.profile,
        "python": sys.executable,
        "executor_entry": executor_entry,
        "runner": runner,
        "max_workers": max_workers,
        "exec_start": systemd_exec,
        "exec_stop": exec_stop,
        "k8s_command_yaml": yaml_command(k8s_args),
        "k8s_resources_yaml": kustomization_resources(k8s_resources),
        "k8s_resources": k8s_resources,
        "queue_token_env_yaml": queue_token_env_yaml(queue_backend == "http"),
        "queue_service_port": queue_service_port,
        "interval_seconds": int(profile.get("interval_seconds") or 300),
        "max_failures": int(profile.get("max_failures") or 3),
        "max_items": int(profile.get("max_items_per_cycle") or 1),
        "worker_stage": str(profile.get("worker_stage") or "prepare"),
        "lease_seconds": int(profile.get("lease_seconds") or 1800),
        "queue_backend": queue_backend,
        "queue_path": queue_path,
        "k8s_queue_path": k8s_queue_path,
        "queue_path_arg": queue_path_arg,
        "queue_path_yaml": queue_path_yaml,
        "workspace_isolation_policy": workspace_isolation_policy,
        "create_worktrees": create_worktrees,
        "activate_workspaces": activate_workspaces,
        "schedule": args.schedule,
        "image": args.image,
        "image_layout": image_layout,
        "pv_name": args.pv_name,
        "pv_host_path": args.pv_host_path,
        "pv_node_name": args.pv_node_name,
        "storage_class": args.storage_class,
        "cpu_request": args.cpu_request,
        "memory_request": args.memory_request,
        "cpu_limit": args.cpu_limit,
        "memory_limit": args.memory_limit,
        "storage_request": args.storage_request,
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Deployment Plan",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- profile: `{summary['profile_name']}`",
        f"- target: `{summary['target']}`",
        f"- image_layout: `{(summary.get('variables') or {}).get('image_layout')}`",
        "",
        "## Outputs",
    ]
    for item in summary.get("outputs", []):
        lines.append(f"- `{item}`")
    lines.extend(["", "## Findings"])
    findings = summary.get("findings") or []
    if findings:
        for finding in findings:
            lines.append(f"- `{finding['level']}` `{finding['code']}`: {finding['message']}")
    else:
        lines.append("- none")
    validation = summary.get("validation") or {}
    if validation:
        lines.extend(["", "## Validation"])
        for name, result in validation.items():
            lines.append(f"- `{name}` exit `{result.get('exit_code')}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.",
            "- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.",
            "",
        ]
    )
    return "\n".join(lines)


def run_validation(out_dir: Path, target: str) -> dict[str, Any]:
    validation: dict[str, Any] = {}
    if target in {"all", "kubernetes"}:
        env = os.environ.copy()
        for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
            env.pop(key, None)
        yaml_files = [
            out_dir / "codex-loop-pv.yaml",
            out_dir / "codex-loop-pvc.yaml",
            out_dir / "codex-loop-cronjob.yaml",
            out_dir / "codex-loop-queue-service-deployment.yaml",
            out_dir / "codex-loop-queue-service.yaml",
        ]
        command = ["kubectl", "apply", "--dry-run=client"]
        for path in yaml_files:
            if path.exists():
                command.extend(["-f", str(path)])
        proc = subprocess.run(command, cwd=repo_path("."), env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        write_text(out_dir / "kubectl-dry-run.txt", proc.stdout)
        validation["kubectl_dry_run"] = {
            "command": command,
            "exit_code": proc.returncode,
            "output": "deploy/kubectl-dry-run.txt",
            "output_tail": proc.stdout[-4000:],
        }
    if target in {"all", "systemd"}:
        command = ["systemd-analyze", "verify", str(out_dir / "codex-loop.service")]
        proc = subprocess.run(command, cwd=repo_path("."), text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        write_text(out_dir / "systemd-verify.txt", proc.stdout)
        validation["systemd_verify"] = {
            "command": command,
            "exit_code": proc.returncode,
            "output": "deploy/systemd-verify.txt",
            "output_tail": proc.stdout[-4000:],
        }
    return validation


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--target", choices=["all", "systemd", "kubernetes"], default="all")
    parser.add_argument("--profile", default=DEFAULT_PROFILE)
    parser.add_argument("--profiles", default=str(DEFAULT_PROFILES))
    parser.add_argument("--schedule", default="*/15 * * * *")
    parser.add_argument("--image", default="docker.io/traffic-analysis/codex-loop@sha256:8c5b02614836432992780e6a8e7550d73fbffabd13f834150f960f8f374a4ee7")
    parser.add_argument("--cpu-request", default="250m")
    parser.add_argument("--memory-request", default="256Mi")
    parser.add_argument("--cpu-limit", default="1")
    parser.add_argument("--memory-limit", default="1Gi")
    parser.add_argument("--storage-request", default="2Gi")
    parser.add_argument("--storage-class", default="local-hdd")
    parser.add_argument("--pv-name", default="codex-loop-workspace-pv")
    parser.add_argument("--pv-host-path", default="/home/k8s-data/codex-loop/workspace")
    parser.add_argument("--pv-node-name", default="8-2tb")
    parser.add_argument("--app-root", default="/app")
    parser.add_argument("--state-root", default="/workspace")
    parser.add_argument("--image-layout", choices=["auto", "control-only", "full-repo"], default="auto")
    parser.add_argument("--validate", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("deploy")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "deploy"
    out_dir.mkdir(parents=True, exist_ok=True)
    profiles_path = repo_path(args.profiles)
    profile = selected_profile(args.profile, profiles_path)
    findings = validate_profile(profile)
    variables = build_variables(profile, args)
    if args.target in {"all", "kubernetes"} and variables["executor_entry"] != "queue_service" and variables["image_layout"] == "control-only":
        findings.append(
            {
                "level": "blocker",
                "code": "K8S_FULL_REPO_IMAGE_REQUIRED",
                "message": "Kubernetes profiles that execute service/executor workflows require a full-repo image or mounted repo workspace; control-only images are limited to queue service and synthetic remote-pool workers.",
            }
        )
    outputs: list[str] = []

    if args.target in {"all", "systemd"}:
        write_text(out_dir / "codex-loop.service", render_template(SYSTEMD_TEMPLATE, variables))
        outputs.append("deploy/codex-loop.service")
    if args.target in {"all", "kubernetes"}:
        write_text(out_dir / "codex-loop-pv.yaml", render_template(PV_TEMPLATE, variables))
        write_text(out_dir / "codex-loop-pvc.yaml", render_template(PVC_TEMPLATE, variables))
        if variables.get("executor_entry") == "queue_service":
            write_text(out_dir / "codex-loop-queue-service-deployment.yaml", render_template(QUEUE_SERVICE_DEPLOYMENT_TEMPLATE, variables))
            write_text(out_dir / "codex-loop-queue-service.yaml", render_template(QUEUE_SERVICE_TEMPLATE, variables))
            outputs.extend(["deploy/codex-loop-pv.yaml", "deploy/codex-loop-pvc.yaml", "deploy/codex-loop-queue-service-deployment.yaml", "deploy/codex-loop-queue-service.yaml"])
        else:
            write_text(out_dir / "codex-loop-cronjob.yaml", render_template(CRONJOB_TEMPLATE, variables))
            outputs.extend(["deploy/codex-loop-pv.yaml", "deploy/codex-loop-pvc.yaml", "deploy/codex-loop-cronjob.yaml"])
        write_text(out_dir / "kustomization.yaml", render_template(KUSTOMIZATION_TEMPLATE, variables))
        outputs.append("deploy/kustomization.yaml")

    validation = run_validation(out_dir, args.target) if args.validate else {}
    for name, result in validation.items():
        if result.get("exit_code") != 0:
            findings.append({"level": "blocker", "code": f"{name.upper()}_FAILED", "message": f"{name} exited {result.get('exit_code')}."})
    if validation:
        write_json(out_dir / "validation.json", validation)
        outputs.append("deploy/validation.json")
        if "kubectl_dry_run" in validation:
            outputs.append("deploy/kubectl-dry-run.txt")
        if "systemd_verify" in validation:
            outputs.append("deploy/systemd-verify.txt")

    status = "DEPLOY_PLAN_BLOCKED" if any(item["level"] == "blocker" for item in findings) else "DEPLOY_PLAN_READY"
    summary = {
        "run_id": run_id,
        "run_kind": "deploy_plan",
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "profile_name": args.profile,
        "profile_path": rel_path(profiles_path),
        "profile": profile,
        "target": args.target,
        "variables": variables,
        "findings": findings,
        "validation": validation,
        "outputs": ["deploy/deploy-plan.json", "deploy/deploy-report.md", *outputs],
    }
    write_json(out_dir / "deploy-plan.json", summary)
    write_text(out_dir / "deploy-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} outputs={len(outputs)}")
    return 1 if status == "DEPLOY_PLAN_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
