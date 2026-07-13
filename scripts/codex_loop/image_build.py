#!/usr/bin/env python3
"""Plan or run the Codex Loop control image build and record evidence."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def env_without_client_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def run_command(command: list[str], timeout: int) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            env=env_without_client_proxies(),
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


def add_finding(findings: list[dict[str, str]], level: str, code: str, message: str) -> None:
    findings.append({"level": level, "code": code, "message": message})


def status_for(findings: list[dict[str, str]], execute: bool, build: dict[str, Any] | None) -> str:
    if any(item["level"] == "blocker" for item in findings):
        return "IMAGE_BUILD_BLOCKED"
    if execute and build:
        return "IMAGE_BUILD_COMPLETED"
    return "IMAGE_BUILD_PLANNED"


def render_command(args: argparse.Namespace) -> str:
    command = [
        "docker",
        "build",
        "-f",
        args.dockerfile,
        "--build-arg",
        f"BASE_IMAGE={args.base_image}",
        "--build-arg",
        f"INSTALL_OS_PACKAGES={args.install_os_packages}",
        "-t",
        args.image,
        args.context,
    ]
    return " ".join(command) + "\n"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Image Build",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- image: `{summary['image']}`",
        f"- image_layout: `{summary['image_layout']}`",
        f"- base_image: `{summary['base_image']}`",
        f"- dockerfile: `{summary['dockerfile']}`",
        f"- context: `{summary['context']}`",
        f"- execute_requested: `{summary['execute_requested']}`",
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
            "- The default mode records a build plan only; it does not run Docker.",
            "- The control-only image is for queue service and synthetic remote-pool workers, not full project code execution.",
            "- Production Secret values must not be baked into images.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--dockerfile", default="scripts/codex_loop/Dockerfile")
    parser.add_argument("--context", default="scripts/codex_loop")
    parser.add_argument("--image", default="traffic-analysis/codex-loop:local")
    parser.add_argument("--base-image", default="python:3.11-slim")
    parser.add_argument("--install-os-packages", choices=["0", "1"], default="0")
    parser.add_argument("--image-layout", choices=["control-only", "full-repo"], default="control-only")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=300)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("image-build")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "image-build"
    out_dir.mkdir(parents=True, exist_ok=True)
    findings: list[dict[str, str]] = []
    dockerfile = repo_path(args.dockerfile)
    context = repo_path(args.context)
    docker_path = shutil.which("docker")
    if not docker_path:
        add_finding(findings, "blocker", "IMAGE_BUILD_DOCKER_MISSING", "docker is not available on PATH.")
    if not dockerfile.exists():
        add_finding(findings, "blocker", "IMAGE_BUILD_DOCKERFILE_MISSING", f"Dockerfile is missing: {rel_path(dockerfile)}.")
    if not context.exists() or not context.is_dir():
        add_finding(findings, "blocker", "IMAGE_BUILD_CONTEXT_MISSING", f"Build context is missing: {rel_path(context)}.")
    if args.image_layout == "control-only" and rel_path(context) != "scripts/codex_loop":
        add_finding(findings, "warning", "IMAGE_BUILD_CONTEXT_UNEXPECTED", "control-only image normally uses scripts/codex_loop as Docker context.")

    write_text(out_dir / "command-template.txt", render_command(args))
    build = None
    if args.execute and not any(item["level"] == "blocker" for item in findings):
        build = run_command(
            [
                "docker",
                "build",
                "-f",
                args.dockerfile,
                "--build-arg",
                f"BASE_IMAGE={args.base_image}",
                "--build-arg",
                f"INSTALL_OS_PACKAGES={args.install_os_packages}",
                "-t",
                args.image,
                args.context,
            ],
            timeout=args.timeout_seconds,
        )
        write_text(out_dir / "docker-build.txt", build["output"])
        if build["exit_code"] != 0:
            add_finding(findings, "blocker", "IMAGE_BUILD_FAILED", f"docker build exited {build['exit_code']}.")
        if build["timed_out"]:
            add_finding(findings, "blocker", "IMAGE_BUILD_TIMED_OUT", f"docker build timed out after {args.timeout_seconds}s.")

    outputs = [
        "image-build/build-summary.json",
        "image-build/build-report.md",
        "image-build/command-template.txt",
    ]
    if build:
        outputs.append("image-build/docker-build.txt")
    summary = {
        "run_id": run_id,
        "run_kind": "image_build",
        "status": status_for(findings, args.execute, build),
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "docker": {"available": bool(docker_path), "path": docker_path},
        "dockerfile": rel_path(dockerfile),
        "context": rel_path(context),
        "image": args.image,
        "base_image": args.base_image,
        "install_os_packages": args.install_os_packages,
        "image_layout": args.image_layout,
        "execute_requested": args.execute,
        "build": {key: value for key, value in (build or {}).items() if key != "output"} if build else None,
        "findings": findings,
        "outputs": outputs,
    }
    write_json(out_dir / "build-summary.json", summary)
    write_text(out_dir / "build-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} findings={len(findings)}")
    return 1 if summary["status"] == "IMAGE_BUILD_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
