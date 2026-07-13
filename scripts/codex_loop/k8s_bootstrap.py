#!/usr/bin/env python3
"""Plan or apply Kubernetes bootstrap resources for the Codex Loop queue service."""

from __future__ import annotations

import argparse
import os
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text


ALLOW_ENV = "CODEX_LOOP_ALLOW_K8S_BOOTSTRAP"
TOKEN_ENV = "CODEX_LOOP_QUEUE_TOKEN"


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def env_without_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def run_command(command: list[str], timeout: int = 120, input_text: str | None = None) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            env=env_without_proxies(),
            input=input_text,
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


def redact_secret_command(command: list[str]) -> list[str]:
    return ["--from-literal=token=<redacted>" if item.startswith("--from-literal=token=") else item for item in command]


def add_finding(findings: list[dict[str, str]], level: str, code: str, message: str) -> None:
    findings.append({"level": level, "code": code, "message": message})


def required_paths(deploy_dir: Path) -> dict[str, Path]:
    return {
        "pv": deploy_dir / "codex-loop-pv.yaml",
        "pvc": deploy_dir / "codex-loop-pvc.yaml",
        "deployment": deploy_dir / "codex-loop-queue-service-deployment.yaml",
        "service": deploy_dir / "codex-loop-queue-service.yaml",
        "kustomization": deploy_dir / "kustomization.yaml",
    }


def render_commands(args: argparse.Namespace, deploy_dir: Path) -> str:
    paths = required_paths(deploy_dir)
    lines = [
        "# Validate manifests without writing cluster resources",
        f"kubectl apply --dry-run=client -f {paths['pv']} -f {paths['pvc']} -f {paths['deployment']} -f {paths['service']}",
        "kubectl create secret generic codex-loop-queue-token -n "
        f"{args.namespace} --from-literal=token=\"$CODEX_LOOP_QUEUE_TOKEN\" --dry-run=client -o name",
        "",
        "# Apply after CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1 and CODEX_LOOP_QUEUE_TOKEN are set",
        "kubectl create secret generic codex-loop-queue-token -n "
        f"{args.namespace} --from-literal=token=\"$CODEX_LOOP_QUEUE_TOKEN\" --dry-run=client -o yaml | kubectl apply -f -",
        f"kubectl apply -f {paths['pv']} -f {paths['pvc']} -f {paths['deployment']} -f {paths['service']}",
        f"kubectl -n {args.namespace} rollout status deployment/codex-loop-queue-service --timeout={args.timeout_seconds}s",
        f"kubectl -n {args.namespace} get service codex-loop-queue-service",
        "",
        "# Secret values must not be written into evidence files.",
    ]
    return "\n".join(lines) + "\n"


def validate_secret(namespace: str) -> dict[str, Any]:
    token = os.environ.get(TOKEN_ENV)
    literal = token if token else "placeholder-not-a-real-token"
    result = run_command(
        [
            "kubectl",
            "create",
            "secret",
            "generic",
            "codex-loop-queue-token",
            "-n",
            namespace,
            f"--from-literal=token={literal}",
            "--dry-run=client",
            "-o",
            "name",
        ],
        timeout=60,
    )
    result["output"] = result["output"].replace(literal, "<redacted>")
    result["output_tail"] = result["output_tail"].replace(literal, "<redacted>")
    result["command"] = redact_secret_command(result["command"])
    result["token_env_present"] = bool(token)
    return result


def validate_manifests(deploy_dir: Path) -> dict[str, Any]:
    paths = required_paths(deploy_dir)
    command = [
        "kubectl",
        "apply",
        "--dry-run=client",
        "-f",
        str(paths["pv"]),
        "-f",
        str(paths["pvc"]),
        "-f",
        str(paths["deployment"]),
        "-f",
        str(paths["service"]),
    ]
    return run_command(command, timeout=120)


def apply_secret(namespace: str) -> dict[str, Any]:
    token = os.environ.get(TOKEN_ENV) or ""
    create = run_command(
        [
            "kubectl",
            "create",
            "secret",
            "generic",
            "codex-loop-queue-token",
            "-n",
            namespace,
            f"--from-literal=token={token}",
            "--dry-run=client",
            "-o",
            "yaml",
        ],
        timeout=60,
    )
    apply = {"command": ["kubectl", "apply", "-f", "-"], "exit_code": None, "timed_out": False, "output": "", "output_tail": ""}
    if create["exit_code"] == 0:
        apply = run_command(["kubectl", "apply", "-f", "-"], timeout=120, input_text=create["output"])
    create["output"] = "<redacted>"
    create["output_tail"] = "<redacted>"
    create["command"] = redact_secret_command(create["command"])
    apply["output"] = apply.get("output", "").replace(token, "<redacted>")
    apply["output_tail"] = apply.get("output_tail", "").replace(token, "<redacted>")
    return {"create": {key: value for key, value in create.items() if key != "output"}, "apply": {key: value for key, value in apply.items() if key != "output"}}


def apply_manifests(args: argparse.Namespace, deploy_dir: Path) -> list[dict[str, Any]]:
    paths = required_paths(deploy_dir)
    steps: list[dict[str, Any]] = []
    for name in ["pv", "pvc", "deployment", "service"]:
        result = run_command(["kubectl", "apply", "-f", str(paths[name])], timeout=120)
        steps.append({key: value for key, value in result.items() if key != "output"} | {"name": f"apply_{name}", "output": f"k8s-bootstrap/apply-{name}.txt", "_raw_output": result["output"]})
        if result["exit_code"] != 0:
            return steps
    rollout = run_command(["kubectl", "-n", args.namespace, "rollout", "status", "deployment/codex-loop-queue-service", f"--timeout={args.timeout_seconds}s"], timeout=args.timeout_seconds + 30)
    steps.append({key: value for key, value in rollout.items() if key != "output"} | {"name": "rollout_status", "output": "k8s-bootstrap/rollout-status.txt", "_raw_output": rollout["output"]})
    return steps


def status_for(findings: list[dict[str, str]], execute: bool) -> str:
    if any(item["level"] == "blocker" for item in findings):
        return "K8S_BOOTSTRAP_BLOCKED"
    if execute:
        return "K8S_BOOTSTRAP_APPLIED"
    return "K8S_BOOTSTRAP_VALIDATED"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop K8s Bootstrap",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- namespace: `{summary['namespace']}`",
        f"- deploy_dir: `{summary['deploy_dir']}`",
        f"- execute_requested: `{summary['execute_requested']}`",
        f"- execute_gate_accepted: `{summary['execute_gate']['accepted']}`",
        f"- token_env_present: `{summary['token_env']['present']}`",
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
            "- Default mode validates manifests and secret creation only; it does not apply resources.",
            "- Real apply requires --execute, CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1, and CODEX_LOOP_QUEUE_TOKEN.",
            "- Secret values are never written to evidence.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--deploy-dir", required=True)
    parser.add_argument("--namespace", default="traffic-analysis")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=180)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("k8s-bootstrap")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "k8s-bootstrap"
    out_dir.mkdir(parents=True, exist_ok=True)
    deploy_dir = repo_path(args.deploy_dir)
    paths = required_paths(deploy_dir)
    findings: list[dict[str, str]] = []
    for name, path in paths.items():
        if not path.exists():
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_MANIFEST_MISSING", f"{name} manifest is missing: {rel_path(path)}.")

    write_text(out_dir / "command-template.txt", render_commands(args, deploy_dir))
    manifest_validation = None
    secret_validation = None
    execution = None
    if not any(item["level"] == "blocker" for item in findings):
        manifest_validation = validate_manifests(deploy_dir)
        write_text(out_dir / "kubectl-dry-run.txt", manifest_validation["output"])
        if manifest_validation["exit_code"] != 0:
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_DRY_RUN_FAILED", f"Manifest dry-run exited {manifest_validation['exit_code']}.")
        secret_validation = validate_secret(args.namespace)
        write_text(out_dir / "secret-dry-run.txt", secret_validation["output"])
        if secret_validation["exit_code"] != 0:
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_SECRET_DRY_RUN_FAILED", f"Secret dry-run exited {secret_validation['exit_code']}.")

    execute_gate = {"name": ALLOW_ENV, "present": ALLOW_ENV in os.environ, "accepted": os.environ.get(ALLOW_ENV) == "1"}
    token_env = {"name": TOKEN_ENV, "present": TOKEN_ENV in os.environ, "accepted": bool(os.environ.get(TOKEN_ENV))}
    if args.execute:
        if not execute_gate["accepted"]:
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_EXECUTE_GATE_NOT_ACCEPTED", f"{ALLOW_ENV}=1 is required before applying bootstrap resources.")
        if not token_env["accepted"]:
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_TOKEN_ENV_MISSING", f"{TOKEN_ENV} is required before applying the queue token Secret.")
    elif not token_env["accepted"]:
        add_finding(findings, "warning", "K8S_BOOTSTRAP_TOKEN_ENV_NOT_SET", f"{TOKEN_ENV} was not set; secret dry-run used a placeholder.")

    if args.execute and not any(item["level"] == "blocker" for item in findings):
        secret_apply = apply_secret(args.namespace)
        steps = apply_manifests(args, deploy_dir)
        for step in steps:
            if step.get("_raw_output") is not None:
                write_text(out_dir / Path(step["output"]).name, step.pop("_raw_output"))
            if step.get("exit_code") != 0:
                add_finding(findings, "blocker", "K8S_BOOTSTRAP_APPLY_FAILED", f"{step.get('name')} exited {step.get('exit_code')}.")
        if (secret_apply.get("create") or {}).get("exit_code") != 0 or (secret_apply.get("apply") or {}).get("exit_code") != 0:
            add_finding(findings, "blocker", "K8S_BOOTSTRAP_SECRET_APPLY_FAILED", "Secret create/apply did not complete.")
        execution = {"secret": secret_apply, "steps": steps}

    outputs = [
        "k8s-bootstrap/bootstrap-summary.json",
        "k8s-bootstrap/bootstrap-report.md",
        "k8s-bootstrap/command-template.txt",
    ]
    if manifest_validation:
        outputs.append("k8s-bootstrap/kubectl-dry-run.txt")
    if secret_validation:
        outputs.append("k8s-bootstrap/secret-dry-run.txt")
    if execution:
        for step in execution.get("steps", []):
            if step.get("output"):
                outputs.append(step["output"])

    summary = {
        "run_id": run_id,
        "run_kind": "k8s_bootstrap",
        "status": status_for(findings, args.execute),
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "namespace": args.namespace,
        "deploy_dir": rel_path(deploy_dir),
        "manifests": {name: rel_path(path) for name, path in paths.items()},
        "execute_requested": args.execute,
        "execute_gate": execute_gate,
        "token_env": token_env,
        "manifest_validation": {key: value for key, value in (manifest_validation or {}).items() if key != "output"} if manifest_validation else None,
        "secret_validation": {key: value for key, value in (secret_validation or {}).items() if key != "output"} if secret_validation else None,
        "execution": execution,
        "findings": findings,
        "outputs": outputs,
    }
    write_json(out_dir / "bootstrap-summary.json", summary)
    write_text(out_dir / "bootstrap-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} findings={len(findings)}")
    return 2 if summary["status"] == "K8S_BOOTSTRAP_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
