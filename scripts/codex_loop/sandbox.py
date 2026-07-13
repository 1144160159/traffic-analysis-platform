#!/usr/bin/env python3
"""Render isolated execution plans for Codex Loop workflows.

The sandbox planner does not run containers or apply Kubernetes resources. It
turns one task workflow into an auditable isolation contract that can be
validated before a production executor is allowed to run it.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, copy_task_snapshot, ensure_run_dir, list_of, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_POLICY = SCRIPT_ROOT / "policies" / "execution-sandbox.yaml"
JOB_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-sandbox-job.yaml"
NETWORK_POLICY_TEMPLATE = SCRIPT_ROOT / "templates" / "codex-loop-sandbox-networkpolicy.yaml"


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def load_policy(path: Path) -> dict[str, Any]:
    policy = load_yaml_subset(path)
    policy.setdefault("default_driver", "kubernetes-job")
    policy.setdefault("allowed_drivers", ["kubernetes-job", "local-container"])
    policy.setdefault("allowed_stages", ["prepare", "dry-run"])
    policy.setdefault("default_namespace", "traffic-analysis")
    policy.setdefault("default_image", "docker.io/traffic-analysis/codex-loop@sha256:8c5b02614836432992780e6a8e7550d73fbffabd13f834150f960f8f374a4ee7")
    policy.setdefault("repo_mount_path", "/workspace")
    policy.setdefault("allow_live_write", False)
    policy.setdefault("allow_external_codex", False)
    policy.setdefault("allow_network", False)
    policy.setdefault("automount_service_account_token", False)
    policy.setdefault("read_only_root_filesystem", True)
    policy.setdefault("allow_privilege_escalation", False)
    policy.setdefault("run_as_non_root", True)
    policy.setdefault("run_as_user", 1000)
    policy.setdefault("run_as_group", 1000)
    policy.setdefault("fs_group", 1000)
    policy.setdefault("seccomp_profile", "RuntimeDefault")
    policy.setdefault("drop_capabilities", ["ALL"])
    policy.setdefault("active_deadline_seconds", 1800)
    policy.setdefault("ttl_seconds_after_finished", 86400)
    policy.setdefault("local_container_runtime", "docker")
    policy.setdefault("local_network", "none")
    return policy


def safe_name(value: str, limit: int = 63) -> str:
    text = re.sub(r"[^a-z0-9-]+", "-", value.lower()).strip("-")
    text = re.sub(r"-+", "-", text) or "codex-loop"
    return text[:limit].rstrip("-") or "codex-loop"


def yaml_bool(value: Any) -> str:
    return "true" if bool(value) else "false"


def shell_join(args: list[str]) -> str:
    return " ".join(shlex.quote(str(item)) for item in args)


def capability_drop_yaml(policy: dict[str, Any]) -> str:
    drops = [str(item) for item in list_of(policy.get("drop_capabilities"))]
    return "\n".join(f"                - {item}" for item in drops) if drops else "                - ALL"


def build_workflow_command(task_path: Path, child_run_id: str, stage: str) -> list[str]:
    return [
        "python",
        "-B",
        "scripts/codex_loop/workflow.py",
        "--task",
        rel_path(task_path),
        "--run-id",
        child_run_id,
        "--stage",
        stage,
    ]


def build_local_container_command(policy: dict[str, Any], image: str, task_path: Path, child_run_id: str, stage: str) -> list[str]:
    runtime = str(policy.get("local_container_runtime") or "docker")
    repo_mount = str(policy.get("repo_mount_path") or "/workspace")
    command = [
        runtime,
        "run",
        "--rm",
        "--network",
        str(policy.get("local_network") or "none"),
        "--workdir",
        repo_mount,
        "--cap-drop",
        "ALL",
        "--security-opt",
        "no-new-privileges",
        "--cpus",
        str(policy.get("cpu_limit") or "1"),
        "--memory",
        str(policy.get("memory_limit") or "1Gi"),
        "-e",
        "PYTHONUNBUFFERED=1",
        "-e",
        "CODEX_LOOP_SANDBOX=1",
        "-v",
        f"{repo_path('.')}:{repo_mount}",
    ]
    if policy.get("read_only_root_filesystem") is True:
        command.insert(2, "--read-only")
        command.extend(["--tmpfs", "/tmp:rw,noexec,nosuid,size=256m"])
    command.extend([image, *build_workflow_command(task_path, child_run_id, stage)])
    return command


def render_job(policy: dict[str, Any], variables: dict[str, Any]) -> str:
    template_vars = dict(variables)
    template_vars.update(
        {
            "automount_service_account_token": yaml_bool(policy.get("automount_service_account_token")),
            "run_as_non_root": yaml_bool(policy.get("run_as_non_root")),
            "allow_privilege_escalation": yaml_bool(policy.get("allow_privilege_escalation")),
            "read_only_root_filesystem": yaml_bool(policy.get("read_only_root_filesystem")),
            "capability_drop_yaml": capability_drop_yaml(policy),
        }
    )
    return JOB_TEMPLATE.read_text(encoding="utf-8").format(**template_vars)


def render_network_policy(policy: dict[str, Any], variables: dict[str, Any]) -> str:
    return NETWORK_POLICY_TEMPLATE.read_text(encoding="utf-8").format(**variables)


def validate_policy(policy: dict[str, Any], driver: str, stage: str) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    allowed_drivers = {str(item) for item in list_of(policy.get("allowed_drivers"))}
    allowed_stages = {str(item) for item in list_of(policy.get("allowed_stages"))}
    if driver not in allowed_drivers:
        findings.append(finding("blocker", "SANDBOX_DRIVER_NOT_ALLOWED", f"Driver `{driver}` is not allowed."))
    if stage not in allowed_stages:
        findings.append(finding("blocker", "SANDBOX_STAGE_NOT_ALLOWED", f"Stage `{stage}` is not allowed by the sandbox policy."))
    if policy.get("allow_live_write") is True:
        findings.append(finding("blocker", "SANDBOX_LIVE_WRITE_ENABLED", "Sandbox policy cannot enable live write by default."))
    if policy.get("allow_external_codex") is True:
        findings.append(finding("blocker", "SANDBOX_EXTERNAL_CODEX_ENABLED", "Sandbox policy cannot enable external Codex by default."))
    if policy.get("allow_privilege_escalation") is True:
        findings.append(finding("blocker", "SANDBOX_PRIVILEGE_ESCALATION", "Privilege escalation must be disabled."))
    if policy.get("automount_service_account_token") is True:
        findings.append(finding("blocker", "SANDBOX_SERVICE_ACCOUNT_TOKEN", "Service account token automount must be disabled."))
    if "ALL" not in {str(item) for item in list_of(policy.get("drop_capabilities"))}:
        findings.append(finding("blocker", "SANDBOX_CAPABILITIES_NOT_DROPPED", "Sandbox must drop all Linux capabilities."))
    if policy.get("read_only_root_filesystem") is not True:
        findings.append(finding("warning", "SANDBOX_ROOT_WRITABLE", "readOnlyRootFilesystem is disabled."))
    if policy.get("allow_network") is True:
        findings.append(finding("warning", "SANDBOX_NETWORK_ENABLED", "Network is enabled for the sandbox plan."))
    return findings


def run_validation(out_dir: Path, driver: str) -> dict[str, Any]:
    validation: dict[str, Any] = {}
    if driver == "kubernetes-job":
        command = [
            "kubectl",
            "apply",
            "--dry-run=client",
            "-f",
            str(out_dir / "codex-loop-sandbox-job.yaml"),
            "-f",
            str(out_dir / "codex-loop-sandbox-networkpolicy.yaml"),
        ]
        env = os.environ.copy()
        for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
            env.pop(key, None)
        if not shutil.which("kubectl"):
            validation["kubectl_dry_run"] = {"command": command, "exit_code": None, "skipped": True, "reason": "kubectl not found"}
            return validation
        proc = subprocess.run(command, cwd=repo_path("."), env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        write_text(out_dir / "kubectl-dry-run.txt", proc.stdout)
        validation["kubectl_dry_run"] = {
            "command": command,
            "exit_code": proc.returncode,
            "output": "sandbox/kubectl-dry-run.txt",
            "output_tail": proc.stdout[-4000:],
        }
    return validation


def status_for(findings: list[dict[str, str]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "SANDBOX_PLAN_BLOCKED"
    if findings:
        return "SANDBOX_PLAN_DEGRADED"
    return "SANDBOX_PLAN_READY"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Sandbox Plan",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- driver: `{summary['driver']}`",
        f"- stage: `{summary['stage']}`",
        f"- task: `{summary['task_id']}`",
        f"- image: `{summary['image']}`",
        f"- network_allowed: `{summary['network_allowed']}`",
        "",
        "## Outputs",
    ]
    for item in summary.get("outputs", []):
        lines.append(f"- `{item}`")
    lines.extend(["", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    if summary.get("local_container_command"):
        lines.extend(["", "## Local Container Command", "", "```bash", summary["local_container_command"], "```"])
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This script only renders isolation plans and optional dry-run validation; it does not run containers or apply Kubernetes resources.",
            "- Default policy denies live write, external Codex execution, service account token automount and network egress.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--child-run-id", default=None)
    parser.add_argument("--stage", choices=["prepare", "dry-run", "execute-local"], default="prepare")
    parser.add_argument("--driver", choices=["kubernetes-job", "local-container"], default=None)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--namespace", default=None)
    parser.add_argument("--image", default=None)
    parser.add_argument("--validate", action="store_true")
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    policy_path = repo_path(args.policy)
    policy = load_policy(policy_path)
    driver = args.driver or str(policy.get("default_driver") or "kubernetes-job")
    run_id = args.run_id or make_run_id(f"sandbox-{task.get('id')}")
    child_run_id = args.child_run_id or f"{run_id}-workflow"
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "sandbox"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    namespace = args.namespace or str(policy.get("default_namespace") or "traffic-analysis")
    image = args.image or str(policy.get("default_image") or "docker.io/traffic-analysis/codex-loop@sha256:8c5b02614836432992780e6a8e7550d73fbffabd13f834150f960f8f374a4ee7")
    safe_run_id = safe_name(run_id, 50)
    job_name = safe_name(f"codex-loop-{run_id}", 63)
    network_policy_name = safe_name(f"codex-loop-net-{run_id}", 63)
    variables = {
        "job_name": job_name,
        "network_policy_name": network_policy_name,
        "namespace": namespace,
        "safe_run_id": safe_run_id,
        "image": image,
        "repo_mount_path": str(policy.get("repo_mount_path") or "/workspace"),
        "task_path": rel_path(task_path),
        "child_run_id": child_run_id,
        "stage": args.stage,
        "run_as_user": int(policy.get("run_as_user") or 1000),
        "run_as_group": int(policy.get("run_as_group") or 1000),
        "fs_group": int(policy.get("fs_group") or 1000),
        "seccomp_profile": str(policy.get("seccomp_profile") or "RuntimeDefault"),
        "cpu_request": str(policy.get("cpu_request") or "250m"),
        "memory_request": str(policy.get("memory_request") or "256Mi"),
        "cpu_limit": str(policy.get("cpu_limit") or "1"),
        "memory_limit": str(policy.get("memory_limit") or "1Gi"),
        "active_deadline_seconds": int(policy.get("active_deadline_seconds") or 1800),
        "ttl_seconds_after_finished": int(policy.get("ttl_seconds_after_finished") or 86400),
    }

    findings = validate_policy(policy, driver, args.stage)
    if not task_path.exists():
        findings.append(finding("blocker", "TASK_NOT_FOUND", f"Task file not found: {rel_path(task_path)}"))

    local_command = build_local_container_command(policy, image, task_path, child_run_id, args.stage)
    write_text(out_dir / "codex-loop-sandbox-job.yaml", render_job(policy, variables))
    write_text(out_dir / "codex-loop-sandbox-networkpolicy.yaml", render_network_policy(policy, variables))
    write_text(out_dir / "local-container-command.txt", shell_join(local_command) + "\n")

    validation = run_validation(out_dir, driver) if args.validate else {}
    for name, result in validation.items():
        if result.get("exit_code") not in {0, None}:
            findings.append(finding("blocker", f"{name.upper()}_FAILED", f"{name} exited {result.get('exit_code')}."))
    if validation:
        write_json(out_dir / "validation.json", validation)

    outputs = [
        "sandbox/sandbox-plan.json",
        "sandbox/sandbox-report.md",
        "sandbox/codex-loop-sandbox-job.yaml",
        "sandbox/codex-loop-sandbox-networkpolicy.yaml",
        "sandbox/local-container-command.txt",
    ]
    if validation:
        outputs.append("sandbox/validation.json")
        if "kubectl_dry_run" in validation and validation["kubectl_dry_run"].get("output"):
            outputs.append("sandbox/kubectl-dry-run.txt")

    summary = {
        "run_id": run_id,
        "run_kind": "sandbox_plan",
        "status": status_for(findings),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "task_path": rel_path(task_path),
        "child_run_id": child_run_id,
        "driver": driver,
        "stage": args.stage,
        "policy_path": rel_path(policy_path),
        "namespace": namespace,
        "image": image,
        "job_name": job_name,
        "network_policy_name": network_policy_name,
        "network_allowed": bool(policy.get("allow_network")),
        "local_container_command": shell_join(local_command),
        "validation": validation,
        "findings": findings,
        "outputs": outputs,
        "warning": "Sandbox plan is isolation evidence only; it does not run the workflow or close tasks.",
    }
    write_json(out_dir / "sandbox-plan.json", summary)
    write_text(out_dir / "sandbox-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} driver={driver} stage={args.stage} findings={len(findings)}")
    return 1 if summary["status"] == "SANDBOX_PLAN_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
