#!/usr/bin/env python3
"""Execute a rendered Codex Loop sandbox plan behind an explicit gate.

Default mode is audit-only. Real Kubernetes Job apply or local container
execution requires --execute and CODEX_LOOP_ALLOW_SANDBOX_EXECUTION=1.
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, rel_path, repo_path, run_git, write_json, write_text


EXECUTE_TRUE_VALUES = {"1", "true", "yes", "allow", "allowed"}


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def env_without_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def execute_allowed(plan: dict[str, Any]) -> tuple[bool, dict[str, Any]]:
    policy_path = repo_path(plan.get("policy_path") or "scripts/codex_loop/policies/execution-sandbox.yaml")
    gate_name = "CODEX_LOOP_ALLOW_SANDBOX_EXECUTION"
    if policy_path.exists():
        text = policy_path.read_text(encoding="utf-8")
        for line in text.splitlines():
            if line.strip().startswith("allow_execute_env:"):
                gate_name = line.split(":", 1)[1].strip() or gate_name
                break
    value = os.environ.get(gate_name, "")
    accepted = value.strip().lower() in EXECUTE_TRUE_VALUES
    return accepted, {"name": gate_name, "present": gate_name in os.environ, "accepted": accepted, "value": "[redacted]" if gate_name in os.environ else None}


def run_command(command: list[str], timeout: int, env: dict[str, str] | None = None) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
            check=False,
        )
        return {
            "command": command,
            "exit_code": proc.returncode,
            "stdout": proc.stdout,
            "stderr": proc.stderr,
            "timed_out": False,
        }
    except subprocess.TimeoutExpired as exc:
        stdout = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
        stderr = exc.stderr if isinstance(exc.stderr, str) else (exc.stderr or b"").decode("utf-8", errors="replace")
        return {
            "command": command,
            "exit_code": None,
            "stdout": stdout,
            "stderr": stderr,
            "timed_out": True,
        }


def append_result(output_dir: Path, name: str, result: dict[str, Any]) -> None:
    write_text(output_dir / f"{name}.stdout.txt", result.get("stdout") or "")
    write_text(output_dir / f"{name}.stderr.txt", result.get("stderr") or "")


def execute_kubernetes(plan: dict[str, Any], plan_dir: Path, out_dir: Path, timeout: int, cleanup: bool) -> dict[str, Any]:
    namespace = str(plan.get("namespace") or "traffic-analysis")
    job_name = str(plan.get("job_name") or "")
    network_policy_name = str(plan.get("network_policy_name") or "")
    job_yaml = plan_dir / "codex-loop-sandbox-job.yaml"
    network_yaml = plan_dir / "codex-loop-sandbox-networkpolicy.yaml"
    results: dict[str, Any] = {"driver": "kubernetes-job", "steps": []}
    if not shutil.which("kubectl"):
        results["steps"].append({"name": "precheck", "exit_code": None, "error": "kubectl not found"})
        return results

    commands = [
        ("apply_network_policy", ["kubectl", "apply", "-f", str(network_yaml)]),
        ("apply_job", ["kubectl", "apply", "-f", str(job_yaml)]),
        ("wait_complete", ["kubectl", "wait", "--for=condition=complete", f"job/{job_name}", "-n", namespace, f"--timeout={timeout}s"]),
        ("logs", ["kubectl", "logs", f"job/{job_name}", "-n", namespace, "--all-containers=true"]),
        ("describe", ["kubectl", "describe", f"job/{job_name}", "-n", namespace]),
    ]
    for name, command in commands:
        result = run_command(command, timeout=timeout, env=env_without_proxies())
        append_result(out_dir, name, result)
        results["steps"].append({"name": name, "command": command, "exit_code": result.get("exit_code"), "timed_out": result.get("timed_out"), "stdout": f"sandbox-executor/{name}.stdout.txt", "stderr": f"sandbox-executor/{name}.stderr.txt"})
        if name in {"apply_network_policy", "apply_job", "wait_complete"} and result.get("exit_code") not in {0}:
            break
    if cleanup:
        for name, command in [
            ("cleanup_job", ["kubectl", "delete", "job", job_name, "-n", namespace, "--ignore-not-found=true"]),
            ("cleanup_network_policy", ["kubectl", "delete", "networkpolicy", network_policy_name, "-n", namespace, "--ignore-not-found=true"]),
        ]:
            result = run_command(command, timeout=120, env=env_without_proxies())
            append_result(out_dir, name, result)
            results["steps"].append({"name": name, "command": command, "exit_code": result.get("exit_code"), "timed_out": result.get("timed_out"), "stdout": f"sandbox-executor/{name}.stdout.txt", "stderr": f"sandbox-executor/{name}.stderr.txt"})
    return results


def execute_local_container(plan: dict[str, Any], out_dir: Path, timeout: int) -> dict[str, Any]:
    command_text = str(plan.get("local_container_command") or "")
    command = shlex.split(command_text)
    result = run_command(command, timeout=timeout)
    append_result(out_dir, "local-container", result)
    return {
        "driver": "local-container",
        "steps": [
            {
                "name": "local_container",
                "command": command,
                "exit_code": result.get("exit_code"),
                "timed_out": result.get("timed_out"),
                "stdout": "sandbox-executor/local-container.stdout.txt",
                "stderr": "sandbox-executor/local-container.stderr.txt",
            }
        ],
    }


def derive_status(execute: bool, findings: list[dict[str, str]], execution: dict[str, Any] | None) -> str:
    if findings:
        return "SANDBOX_EXECUTION_BLOCKED"
    if not execute:
        return "SANDBOX_EXECUTION_PLANNED"
    steps = (execution or {}).get("steps") or []
    if any(step.get("timed_out") for step in steps):
        return "SANDBOX_EXECUTION_TIMEOUT"
    if steps and all(step.get("exit_code") == 0 for step in steps):
        return "SANDBOX_EXECUTION_COMPLETED"
    return "SANDBOX_EXECUTION_FAILED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Sandbox Execution",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- execute_requested: `{summary['execute_requested']}`",
        f"- driver: `{summary['driver']}`",
        f"- plan: `{summary['sandbox_plan']}`",
        f"- gate: `{summary['execute_gate']['name']}` accepted `{summary['execute_gate']['accepted']}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Steps"])
    steps = ((summary.get("execution") or {}).get("steps") or [])
    if steps:
        for step in steps:
            lines.append(f"- `{step['name']}` exit `{step.get('exit_code')}` timed_out `{step.get('timed_out')}`")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Default mode is audit-only; real execution requires --execute and the sandbox execution environment gate.",
            "- This executor does not decide task closure; workflow, review, and evidence_check still own closure.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sandbox-plan", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=1800)
    parser.add_argument("--cleanup", action="store_true")
    args = parser.parse_args()

    plan_path = repo_path(args.sandbox_plan)
    plan = load_json(plan_path)
    run_id = args.run_id or f"{plan.get('run_id', 'sandbox')}-execution"
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "sandbox-executor"
    out_dir.mkdir(parents=True, exist_ok=True)
    plan_dir = plan_path.parent
    gate_allowed, gate = execute_allowed(plan)
    findings: list[dict[str, str]] = []
    if plan.get("status") != "SANDBOX_PLAN_READY":
        findings.append(finding("blocker", "SANDBOX_PLAN_NOT_READY", f"Sandbox plan status is {plan.get('status')}." ))
    if args.execute and not gate_allowed:
        findings.append(finding("blocker", "SANDBOX_EXECUTE_GATE_NOT_SET", f"Set {gate['name']}=1 to allow sandbox execution."))
    driver = str(plan.get("driver") or "")
    if driver not in {"kubernetes-job", "local-container"}:
        findings.append(finding("blocker", "SANDBOX_DRIVER_UNSUPPORTED", f"Unsupported sandbox driver `{driver}`."))

    execution: dict[str, Any] | None = None
    if args.execute and not findings:
        if driver == "kubernetes-job":
            execution = execute_kubernetes(plan, plan_dir, out_dir, args.timeout_seconds, args.cleanup)
        elif driver == "local-container":
            execution = execute_local_container(plan, out_dir, args.timeout_seconds)

    outputs = ["sandbox-executor/execution.json", "sandbox-executor/execution-report.md"]
    if execution:
        for step in execution.get("steps") or []:
            if step.get("stdout"):
                outputs.append(step["stdout"])
            if step.get("stderr"):
                outputs.append(step["stderr"])

    summary = {
        "run_id": run_id,
        "run_kind": "sandbox_execution",
        "status": derive_status(args.execute, findings, execution),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "sandbox_plan": rel_path(plan_path),
        "plan_status": plan.get("status"),
        "task_id": plan.get("task_id"),
        "child_run_id": plan.get("child_run_id"),
        "driver": driver,
        "execute_requested": args.execute,
        "execute_gate": gate,
        "cleanup": args.cleanup,
        "findings": findings,
        "execution": execution,
        "outputs": outputs,
        "warning": "Sandbox execution evidence is operational; it does not close tasks by itself.",
    }
    write_json(out_dir / "execution.json", summary)
    write_text(out_dir / "execution-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} execute={args.execute} driver={driver} findings={len(findings)}")
    return 0 if summary["status"] in {"SANDBOX_EXECUTION_PLANNED", "SANDBOX_EXECUTION_COMPLETED"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
