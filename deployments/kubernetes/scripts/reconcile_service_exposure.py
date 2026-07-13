#!/usr/bin/env python3
"""Reconcile Kubernetes Service objects so old NodePort fields are pruned."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path

import yaml


PROXY_ENV_KEYS = {
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "http_proxy",
    "https_proxy",
    "all_proxy",
}


def iter_yaml_documents(root: Path):
    paths = [root] if root.is_file() else sorted(root.rglob("*.yaml")) + sorted(root.rglob("*.yml"))
    for path in paths:
        if ".archive" in path.parts:
            continue
        with path.open(encoding="utf-8") as handle:
            for index, document in enumerate(yaml.safe_load_all(handle), 1):
                if isinstance(document, dict):
                    yield path, index, document


def service_documents(roots: list[Path]) -> list[tuple[Path, int, dict]]:
    services: list[tuple[Path, int, dict]] = []
    for root in roots:
        for path, index, document in iter_yaml_documents(root):
            if document.get("kind") == "Service":
                services.append((path, index, document))
    return services


def kubectl_env() -> dict[str, str]:
    env = os.environ.copy()
    for key in PROXY_ENV_KEYS:
        env.pop(key, None)
    return env


def run_kubectl(kubectl: str, args: list[str], stdin: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [kubectl, *args],
        input=stdin,
        text=True,
        capture_output=True,
        env=kubectl_env(),
        check=False,
    )


def trim(value: str, limit: int = 800) -> str:
    value = " ".join(value.split())
    return value[:limit]


def reconcile_service(kubectl: str, service: dict, dry_run: str) -> dict:
    metadata = service.get("metadata") or {}
    spec = service.get("spec") or {}
    namespace = metadata.get("namespace") or ""
    name = metadata.get("name") or ""
    port_summary = [
        {
            "name": port.get("name") or "",
            "port": port.get("port"),
            "node_port": port.get("nodePort"),
            "target_port": port.get("targetPort"),
        }
        for port in spec.get("ports") or []
    ]
    if not namespace:
        return {
            "namespace": "default",
            "service": name,
            "type": spec.get("type") or "ClusterIP",
            "ports": port_summary,
            "action": "skip",
            "dry_run": dry_run,
            "returncode": 1,
            "stdout": "",
            "stderr": "Service manifest is missing metadata.namespace",
            "status": "failed",
        }
    exists = run_kubectl(kubectl, ["get", "service", name, "-n", namespace])
    action = "replace" if exists.returncode == 0 else "apply"
    payload = yaml.safe_dump(service, sort_keys=False)
    command = [action]
    if action == "replace":
        command.append("--save-config")
    if dry_run != "none":
        command.append(f"--dry-run={dry_run}")
    command.extend(["-f", "-"])
    result = run_kubectl(kubectl, command, payload)
    return {
        "namespace": namespace,
        "service": name,
        "type": spec.get("type") or "ClusterIP",
        "ports": port_summary,
        "action": action,
        "dry_run": dry_run,
        "returncode": result.returncode,
        "stdout": trim(result.stdout),
        "stderr": trim(result.stderr),
        "status": "ok" if result.returncode == 0 else "failed",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", action="append", type=Path, required=True)
    parser.add_argument("--kubectl", default=os.environ.get("KUBECTL", "kubectl"))
    parser.add_argument("--dry-run", choices=["none", "client", "server"], default="none")
    parser.add_argument("--out-inventory", type=Path)
    args = parser.parse_args()

    records = []
    for path, index, service in service_documents(args.root):
        record = reconcile_service(args.kubectl, service, args.dry_run)
        record["path"] = str(path)
        record["document"] = index
        records.append(record)

    summary = {
        "roots": [str(root) for root in args.root],
        "dry_run": args.dry_run,
        "services": len(records),
        "failed": len([record for record in records if record["status"] != "ok"]),
    }
    if args.out_inventory:
        args.out_inventory.write_text(json.dumps(records, indent=2, ensure_ascii=True), encoding="utf-8")
    print(json.dumps(summary, indent=2, ensure_ascii=True))
    return 1 if summary["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
