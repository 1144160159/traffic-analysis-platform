#!/usr/bin/env python3
"""Plan or execute a Kubernetes multi-pod stress against the HTTP queue service."""

from __future__ import annotations

import argparse
import os
import subprocess
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from queue_backend import display_path, enqueue_plan, queue_status
from remote_pool_stress import is_loopback_url, stress_plan, target_queue_counts


ALLOW_ENV = "CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS"


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def safe_name(value: str, max_len: int = 63) -> str:
    result = "".join(ch.lower() if ch.isalnum() else "-" for ch in value).strip("-")
    result = "-".join(part for part in result.split("-") if part)
    return (result or "remote-pool")[:max_len].rstrip("-")


def task_id(prefix: str, index: int) -> str:
    return f"{prefix}-{index:03d}"


def yaml_list(items: list[str], indent: int = 10) -> str:
    pad = " " * indent
    return "\n".join(f'{pad}- "{item}"' for item in items)


def env_without_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def run_command(command: list[str], timeout: int = 120) -> dict[str, Any]:
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


def service_findings(args: argparse.Namespace, token: str | None) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    parsed = urlparse(args.service_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_SERVICE_URL_INVALID", "message": "--service-url must be an http(s) base URL."})
        return findings
    if not is_loopback_url(args.service_url) and not args.allow_external_service and os.environ.get("CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS") != "1":
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_EXTERNAL_SERVICE_NOT_ALLOWED", "message": "Non-loopback queue service stress requires --allow-external-service or CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS=1."})
    if args.execute and not token:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_EXECUTE_TOKEN_REQUIRED", "message": "Execution mode requires CODEX_LOOP_QUEUE_TOKEN or --auth-token for seed/finalize operations."})
    if args.execute and os.environ.get(ALLOW_ENV) != "1":
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_EXECUTE_GATE_NOT_SET", "message": f"Set {ALLOW_ENV}=1 before applying Kubernetes stress jobs."})
    return findings


def worker_command(args: argparse.Namespace, run_id: str, task_prefix: str) -> list[str]:
    command = [
        "python",
        "-B",
        "scripts/codex_loop/remote_pool_stress.py",
        "--worker-only",
        "--run-id",
        f"{run_id}-worker-$(JOB_COMPLETION_INDEX)",
        "--service-url",
        args.service_url.rstrip("/"),
        "--allow-external-service",
        "--worker-index",
        "$(JOB_COMPLETION_INDEX)",
        "--task-prefix",
        task_prefix,
        "--tasks",
        str(args.tasks),
        "--rounds",
        str(args.rounds),
        "--lease-seconds",
        str(args.lease_seconds),
        "--completion-delay-ms",
        str(args.completion_delay_ms),
    ]
    return command


def render_worker_job(args: argparse.Namespace, run_id: str, task_prefix: str, job_name: str) -> str:
    command_yaml = yaml_list(worker_command(args, run_id, task_prefix), indent=16)
    return f"""apiVersion: batch/v1
kind: Job
metadata:
  name: {job_name}
  namespace: {args.namespace}
  labels:
    app.kubernetes.io/name: codex-loop
    app.kubernetes.io/component: remote-pool-stress
    codex-loop/run-id: {safe_name(run_id)}
spec:
  completionMode: Indexed
  completions: {args.workers}
  parallelism: {args.workers}
  backoffLimit: 0
  activeDeadlineSeconds: {args.active_deadline_seconds}
  ttlSecondsAfterFinished: {args.ttl_seconds_after_finished}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: codex-loop
        app.kubernetes.io/component: remote-pool-stress
        codex-loop/run-id: {safe_name(run_id)}
    spec:
      restartPolicy: Never
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: remote-pool-worker
          image: {args.image}
          imagePullPolicy: {args.image_pull_policy}
          workingDir: {args.app_root}
          command:
{command_yaml}
          env:
            - name: PYTHONUNBUFFERED
              value: "1"
            - name: CODEX_LOOP_REPO_ROOT
              value: "{args.app_root}"
            - name: CODEX_LOOP_SCRIPT_ROOT
              value: "{args.app_root}/scripts/codex_loop"
            - name: CODEX_LOOP_RUNS_ROOT
              value: "{args.state_root}/doc/02_acceptance/runs"
            - name: JOB_COMPLETION_INDEX
              valueFrom:
                fieldRef:
                  fieldPath: metadata.annotations['batch.kubernetes.io/job-completion-index']
            - name: CODEX_LOOP_QUEUE_TOKEN
              valueFrom:
                secretKeyRef:
                  name: {args.secret_name}
                  key: {args.token_key}
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: false
            capabilities:
              drop:
                - ALL
          resources:
            requests:
              cpu: "{args.cpu_request}"
              memory: "{args.memory_request}"
            limits:
              cpu: "{args.cpu_limit}"
              memory: "{args.memory_limit}"
          volumeMounts:
            - name: workspace
              mountPath: {args.state_root}
      volumes:
        - name: workspace
          persistentVolumeClaim:
            claimName: {args.pvc_name}
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          operator: Exists
"""


def render_commands(args: argparse.Namespace, worker_job: Path, job_name: str, run_id: str) -> str:
    commands = [
        "# Validate only",
        f"kubectl apply --dry-run=client --validate=false -f {worker_job}",
        "",
        "# Execute after seed enqueue",
        f"kubectl apply -f {worker_job}",
        f"kubectl wait --for=condition=complete job/{job_name} -n {args.namespace} --timeout={args.timeout_seconds}s",
        f"kubectl logs job/{job_name} -n {args.namespace} --all-containers=true",
        "",
        "# Cleanup",
        f"kubectl delete job {job_name} -n {args.namespace} --ignore-not-found=true",
        "",
        f"# Run id: {run_id}",
    ]
    return "\n".join(commands) + "\n"


def validate_job(worker_job: Path) -> dict[str, Any]:
    result = run_command(["kubectl", "apply", "--dry-run=client", "--validate=false", "-f", str(worker_job)], timeout=120)
    return {
        "command": result["command"],
        "exit_code": result["exit_code"],
        "timed_out": result["timed_out"],
        "output": "remote-pool-k8s-stress/kubectl-dry-run.txt",
        "output_tail": result["output_tail"],
        "_raw_output": result["output"],
    }


def execute_job(args: argparse.Namespace, worker_job: Path, job_name: str, task_prefix: str, token: str) -> dict[str, Any]:
    old_token = os.environ.get("CODEX_LOOP_QUEUE_TOKEN")
    os.environ["CODEX_LOOP_QUEUE_TOKEN"] = token
    steps: list[dict[str, Any]] = []
    final_queue: dict[str, Any] = {}
    target_counts: dict[str, Any] = {}
    try:
        seed = enqueue_plan(stress_plan(args.run_id_value, task_prefix, args.tasks, args.max_retries), backend="http", path=args.service_url)
        steps.append({"name": "seed_enqueue", "result": seed})
        for name, command, timeout in [
            ("apply_job", ["kubectl", "apply", "-f", str(worker_job)], 120),
            ("wait_complete", ["kubectl", "wait", "--for=condition=complete", f"job/{job_name}", "-n", args.namespace, f"--timeout={args.timeout_seconds}s"], args.timeout_seconds + 30),
            ("logs", ["kubectl", "logs", f"job/{job_name}", "-n", args.namespace, "--all-containers=true"], 300),
        ]:
            result = run_command(command, timeout=timeout)
            steps.append({"name": name, "command": command, "exit_code": result.get("exit_code"), "timed_out": result.get("timed_out"), "output": f"remote-pool-k8s-stress/{name}.txt", "output_tail": result.get("output_tail"), "_raw_output": result.get("output")})
            if name in {"apply_job", "wait_complete"} and result.get("exit_code") != 0:
                break
        final_queue = queue_status(backend="http", path=args.service_url, include_items=True)
        target_ids = [task_id(task_prefix, index + 1) for index in range(args.tasks)]
        target_counts = target_queue_counts(final_queue, target_ids)
        if args.cleanup:
            result = run_command(["kubectl", "delete", "job", job_name, "-n", args.namespace, "--ignore-not-found=true"], timeout=120)
            steps.append({"name": "cleanup_job", "command": result.get("command"), "exit_code": result.get("exit_code"), "timed_out": result.get("timed_out"), "output": "remote-pool-k8s-stress/cleanup-job.txt", "output_tail": result.get("output_tail"), "_raw_output": result.get("output")})
    finally:
        if old_token is None:
            os.environ.pop("CODEX_LOOP_QUEUE_TOKEN", None)
        else:
            os.environ["CODEX_LOOP_QUEUE_TOKEN"] = old_token
    return {"steps": steps, "final_queue": final_queue, "target_counts": target_counts}


def execution_findings(args: argparse.Namespace, execution: dict[str, Any] | None) -> list[dict[str, str]]:
    if not execution:
        return []
    findings: list[dict[str, str]] = []
    for step in execution.get("steps", []):
        if step.get("name") == "seed_enqueue":
            result = step.get("result") or {}
            if not result.get("counts"):
                findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_SEED_FAILED", "message": "Seed enqueue did not return queue counts."})
        elif step.get("exit_code") != 0:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_STEP_FAILED", "message": f"{step.get('name')} exited {step.get('exit_code')}."})
    counts = (execution.get("target_counts") or {}).get("counts") or {}
    if int(counts.get("done") or 0) != args.tasks:
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_TARGET_COUNTS_UNEXPECTED", "message": f"Expected {args.tasks} done target tasks, got {counts}."})
    if (execution.get("target_counts") or {}).get("missing"):
        findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_TARGET_TASKS_MISSING", "message": f"Target tasks missing: {(execution.get('target_counts') or {}).get('missing')}."})
    return findings


def status_for(findings: list[dict[str, str]], validate: bool, validation: dict[str, Any] | None, execute: bool, execution: dict[str, Any] | None) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "REMOTE_POOL_K8S_STRESS_BLOCKED"
    if execute and execution:
        return "REMOTE_POOL_K8S_STRESS_COMPLETED"
    if validate and validation:
        return "REMOTE_POOL_K8S_STRESS_VALIDATED"
    return "REMOTE_POOL_K8S_STRESS_PLANNED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Remote Pool K8s Stress",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- namespace: `{summary['namespace']}`",
        f"- service_url: `{summary['service_url']}`",
        f"- workers: `{summary['workers']}`",
        f"- tasks: `{summary['tasks']}`",
        f"- validate_requested: `{summary['validate_requested']}`",
        f"- execute_requested: `{summary['execute_requested']}`",
        f"- job_name: `{summary['job_name']}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Outputs"])
    for item in summary.get("outputs", []):
        lines.append(f"- `{item}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Default mode only renders manifests; real apply requires --execute and CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS=1.",
            "- Worker pods only claim and complete synthetic queue tasks through the HTTP queue service.",
            "- This evidence does not close product tasks or replace long-running soak.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--service-url", default="http://codex-loop-queue-service.traffic-analysis.svc:8765")
    parser.add_argument("--allow-external-service", action="store_true")
    parser.add_argument("--namespace", default="traffic-analysis")
    parser.add_argument("--image", default="docker.io/traffic-analysis/codex-loop@sha256:8c5b02614836432992780e6a8e7550d73fbffabd13f834150f960f8f374a4ee7")
    parser.add_argument("--image-pull-policy", default="IfNotPresent")
    parser.add_argument("--app-root", default="/app")
    parser.add_argument("--state-root", default="/workspace")
    parser.add_argument("--pvc-name", default="codex-loop-workspace")
    parser.add_argument("--secret-name", default="codex-loop-queue-token")
    parser.add_argument("--token-key", default="token")
    parser.add_argument("--auth-token", default=None)
    parser.add_argument("--workers", type=int, default=3)
    parser.add_argument("--tasks", type=int, default=6)
    parser.add_argument("--rounds", type=int, default=2)
    parser.add_argument("--lease-seconds", type=int, default=60)
    parser.add_argument("--completion-delay-ms", type=int, default=1)
    parser.add_argument("--task-prefix", default=None)
    parser.add_argument("--max-retries", type=int, default=3)
    parser.add_argument("--cpu-request", default="100m")
    parser.add_argument("--memory-request", default="128Mi")
    parser.add_argument("--cpu-limit", default="500m")
    parser.add_argument("--memory-limit", default="512Mi")
    parser.add_argument("--active-deadline-seconds", type=int, default=900)
    parser.add_argument("--ttl-seconds-after-finished", type=int, default=3600)
    parser.add_argument("--timeout-seconds", type=int, default=900)
    parser.add_argument("--validate", action="store_true")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--cleanup", action="store_true")
    args = parser.parse_args()
    args.app_root = args.app_root.rstrip("/") or "/app"
    args.state_root = args.state_root.rstrip("/") or "/workspace"

    run_id = args.run_id or make_run_id("remote-pool-k8s-stress")
    args.run_id_value = run_id
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "remote-pool-k8s-stress"
    out_dir.mkdir(parents=True, exist_ok=True)
    token = args.auth_token or os.environ.get("CODEX_LOOP_QUEUE_TOKEN")
    task_prefix = args.task_prefix or f"REMOTE-POOL-K8S-{run_id}"
    job_name = safe_name(f"codex-loop-rpks-{run_id}", max_len=63)
    worker_job = out_dir / "remote-pool-worker-job.yaml"
    seed = stress_plan(run_id, task_prefix, args.tasks, args.max_retries)

    findings = service_findings(args, token)
    write_text(worker_job, render_worker_job(args, run_id, task_prefix, job_name))
    write_json(out_dir / "seed-plan.json", seed)
    write_text(out_dir / "command-template.txt", render_commands(args, worker_job, job_name, run_id))

    validation = None
    if args.validate:
        validation = validate_job(worker_job)
        write_text(out_dir / "kubectl-dry-run.txt", validation.pop("_raw_output", ""))
        if validation.get("exit_code") != 0:
            findings.append({"level": "blocker", "code": "REMOTE_POOL_K8S_DRY_RUN_FAILED", "message": f"kubectl dry-run exited {validation.get('exit_code')}."})

    execution = None
    if args.execute and not any(item.get("level") == "blocker" for item in findings):
        execution = execute_job(args, worker_job, job_name, task_prefix, token or "")
        for step in execution.get("steps", []):
            if step.get("_raw_output") is not None and step.get("output"):
                write_text(out_dir / Path(step["output"]).name, step.pop("_raw_output") or "")
        findings.extend(execution_findings(args, execution))

    outputs = [
        "remote-pool-k8s-stress/stress-summary.json",
        "remote-pool-k8s-stress/stress-report.md",
        "remote-pool-k8s-stress/seed-plan.json",
        "remote-pool-k8s-stress/remote-pool-worker-job.yaml",
        "remote-pool-k8s-stress/command-template.txt",
    ]
    if validation:
        outputs.append("remote-pool-k8s-stress/kubectl-dry-run.txt")
    if execution:
        for step in execution.get("steps", []):
            if step.get("output"):
                outputs.append(step["output"])

    summary = {
        "run_id": run_id,
        "run_kind": "remote_pool_k8s_stress",
        "status": status_for(findings, args.validate, validation, args.execute, execution),
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "namespace": args.namespace,
        "service_url": args.service_url.rstrip("/"),
        "service_mode": "kubernetes",
        "image": args.image,
        "image_layout": "control-only",
        "app_root": args.app_root,
        "state_root": args.state_root,
        "job_name": job_name,
        "workers": args.workers,
        "tasks": args.tasks,
        "rounds": args.rounds,
        "task_prefix": task_prefix,
        "target_task_ids": [task_id(task_prefix, index + 1) for index in range(args.tasks)],
        "secret_name": args.secret_name,
        "token_key": args.token_key,
        "validate_requested": args.validate,
        "execute_requested": args.execute,
        "cleanup_requested": args.cleanup,
        "execute_gate": {"name": ALLOW_ENV, "present": ALLOW_ENV in os.environ, "accepted": os.environ.get(ALLOW_ENV) == "1"},
        "validation": validation,
        "execution": {
            "steps": [{key: value for key, value in step.items() if key != "_raw_output"} for step in execution.get("steps", [])],
            "target_counts": execution.get("target_counts"),
            "final_counts": (execution.get("final_queue") or {}).get("counts"),
        } if execution else None,
        "queue_path": display_path(args.service_url),
        "findings": findings,
        "outputs": outputs,
    }
    write_json(out_dir / "stress-summary.json", summary)
    write_text(out_dir / "stress-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} workers={args.workers} tasks={args.tasks} findings={len(findings)}")
    return 0 if summary["status"] in {"REMOTE_POOL_K8S_STRESS_PLANNED", "REMOTE_POOL_K8S_STRESS_VALIDATED", "REMOTE_POOL_K8S_STRESS_COMPLETED"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
