#!/usr/bin/env python3
"""Audit Codex Loop image distribution across Kubernetes nodes."""

from __future__ import annotations

import argparse
import json
import os
import socket
import subprocess
from datetime import datetime
from typing import Any

from lib import ensure_run_dir, make_run_id, run_git, write_json, write_text


def now_iso() -> str:
    return datetime.now().isoformat(timespec="seconds")


def env_without_proxies() -> dict[str, str]:
    env = os.environ.copy()
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        env.pop(key, None)
    return env


def run_command(command: list[str], timeout: int = 60) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            command,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
            env=env_without_proxies(),
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


def kubectl_nodes() -> tuple[list[dict[str, str]], dict[str, Any]]:
    result = run_command(["kubectl", "get", "nodes", "-o", "json"], timeout=60)
    if result["exit_code"] != 0:
        return [], result
    data = json.loads(result["output"])
    nodes: list[dict[str, str]] = []
    for item in data.get("items", []):
        addresses = {entry.get("type"): entry.get("address") for entry in item.get("status", {}).get("addresses", [])}
        nodes.append(
            {
                "name": item.get("metadata", {}).get("name", ""),
                "internal_ip": addresses.get("InternalIP") or "",
                "hostname": addresses.get("Hostname") or "",
            }
        )
    return nodes, result


def is_local_node(node: dict[str, str]) -> bool:
    hostnames = {socket.gethostname().lower(), socket.getfqdn().lower()}
    node_names = {node.get("name", "").lower(), node.get("hostname", "").lower()}
    if hostnames & node_names:
        return True
    local_ips = set()
    for info in socket.getaddrinfo(socket.gethostname(), None):
        local_ips.add(str(info[4][0]))
    return bool(node.get("internal_ip") and node["internal_ip"] in local_ips)


def image_check_command(image: str) -> str:
    return f"ctr -n k8s.io images ls | grep -F {image!r}"


def check_node(node: dict[str, str], image: str, ssh_user: str | None, timeout: int) -> dict[str, Any]:
    if is_local_node(node):
        command = ["sh", "-lc", image_check_command(image)]
        result = run_command(command, timeout=timeout)
        mode = "local"
    else:
        target = node.get("internal_ip") or node.get("hostname") or node.get("name")
        if ssh_user:
            target = f"{ssh_user}@{target}"
        command = ["ssh", "-o", "BatchMode=yes", "-o", f"ConnectTimeout={min(timeout, 10)}", target, image_check_command(image)]
        result = run_command(command, timeout=timeout)
        mode = "ssh"
    return {
        "node": node,
        "mode": mode,
        "available": result["exit_code"] == 0 and image in result["output"],
        "exit_code": result["exit_code"],
        "timed_out": result["timed_out"],
        "output_tail": result["output_tail"],
        "command": result["command"],
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Image Distribution",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- image: `{summary['image']}`",
        f"- nodes_total: `{len(summary.get('nodes', []))}`",
        "",
        "## Nodes",
    ]
    for item in summary.get("nodes", []):
        node = item.get("node") or {}
        lines.append(f"- `{node.get('name')}` `{node.get('internal_ip')}` available=`{item.get('available')}` mode=`{item.get('mode')}`")
    lines.extend(["", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--image", default="docker.io/traffic-analysis/codex-loop@sha256:8c5b02614836432992780e6a8e7550d73fbffabd13f834150f960f8f374a4ee7")
    parser.add_argument("--ssh-user", default=None)
    parser.add_argument("--timeout-seconds", type=int, default=60)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("image-distribution")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "image-distribution"
    out_dir.mkdir(parents=True, exist_ok=True)
    findings: list[dict[str, str]] = []
    nodes, node_command = kubectl_nodes()
    if node_command["exit_code"] != 0:
        add_finding(findings, "blocker", "IMAGE_DISTRIBUTION_NODE_LIST_FAILED", f"kubectl node list exited {node_command['exit_code']}.")
    checks = [check_node(node, args.image, args.ssh_user, args.timeout_seconds) for node in nodes]
    missing = [item for item in checks if not item.get("available")]
    if missing:
        names = ", ".join((item.get("node") or {}).get("name", "") for item in missing)
        add_finding(findings, "blocker", "IMAGE_DISTRIBUTION_MISSING_ON_NODES", f"Image is missing on nodes: {names}.")
    status = "IMAGE_DISTRIBUTION_READY" if not findings else "IMAGE_DISTRIBUTION_BLOCKED"
    summary = {
        "run_id": run_id,
        "run_kind": "image_distribution",
        "status": status,
        "created_at": now_iso(),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "image": args.image,
        "nodes": checks,
        "node_list": {key: value for key, value in node_command.items() if key != "output"},
        "findings": findings,
        "outputs": [
            "image-distribution/distribution-summary.json",
            "image-distribution/distribution-report.md",
        ],
    }
    write_json(out_dir / "distribution-summary.json", summary)
    write_text(out_dir / "distribution-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} nodes={len(nodes)} findings={len(findings)}")
    return 1 if status == "IMAGE_DISTRIBUTION_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
