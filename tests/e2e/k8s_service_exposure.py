#!/usr/bin/env python3
"""Validate the repo Service exposure profile for production preflight."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml


ALLOWED_EXTERNAL = {
    ("gateway", "apisix", "http", 9080, 30180),
}


def iter_yaml_documents(root: Path):
    for path in sorted(root.rglob("*.yaml")) + sorted(root.rglob("*.yml")):
        if ".archive" in path.parts:
            continue
        with path.open(encoding="utf-8") as handle:
            for index, document in enumerate(yaml.safe_load_all(handle), 1):
                if isinstance(document, dict):
                    yield path, index, document


def service_records(root: Path) -> list[dict]:
    return records_from_documents(iter_yaml_documents(root))


def iter_kubectl_payload_documents(path: Path):
    with path.open(encoding="utf-8") as handle:
        payload = json.load(handle)

    if isinstance(payload, dict) and payload.get("kind") == "List":
        items = payload.get("items") or []
    elif isinstance(payload, list):
        items = payload
    else:
        items = [payload]

    for index, document in enumerate(items, 1):
        if isinstance(document, dict):
            yield path, index, document


def records_from_documents(documents) -> list[dict]:
    records: list[dict] = []
    for path, document_index, document in documents:
        if document.get("kind") != "Service":
            continue
        metadata = document.get("metadata") or {}
        spec = document.get("spec") or {}
        namespace_missing = not metadata.get("namespace")
        namespace = metadata.get("namespace") or "default"
        service = metadata.get("name") or ""
        service_type = spec.get("type") or "ClusterIP"
        for port in spec.get("ports") or []:
            port_name = port.get("name") or ""
            port_number = int(port.get("port") or 0)
            node_port = port.get("nodePort")
            if node_port is not None:
                node_port = int(node_port)
            external = service_type in {"NodePort", "LoadBalancer"} or node_port is not None
            allowed = (namespace, service, port_name, port_number, node_port) in ALLOWED_EXTERNAL
            blocked_reason = ""
            if namespace_missing:
                blocked_reason = "missing_namespace"
            elif external and not allowed:
                blocked_reason = "non_business_external"
            records.append(
                {
                    "path": str(path),
                    "document": document_index,
                    "namespace": namespace,
                    "namespace_missing": namespace_missing,
                    "service": service,
                    "type": service_type,
                    "port_name": port_name,
                    "port": port_number,
                    "node_port": node_port,
                    "external": external,
                    "allowed_public_business_port": allowed,
                    "blocked_reason": blocked_reason,
                    "status": "blocked" if blocked_reason else "ok",
                }
            )
    return records


def write_lines(path: Path, records: list[dict]) -> None:
    lines = []
    for record in records:
        lines.append(
            "{path}: {namespace}/{service} {type} {port_name}:{port} nodePort={node_port} reason={blocked_reason}".format(
                **record
            )
        )
    path.write_text("".join(f"{line}\n" for line in lines), encoding="utf-8")


def validate_records(args: argparse.Namespace, records: list[dict], source: dict) -> int:
    blockers = [record for record in records if record["status"] == "blocked"]
    external = [record for record in records if record["external"]]
    summary = {
        **source,
        "service_ports": len(records),
        "external_service_ports": len(external),
        "blocked_external_service_ports": len(blockers),
        "missing_namespace_service_ports": len([record for record in records if record["namespace_missing"]]),
        "allowed_external_service_ports": len([record for record in external if record["allowed_public_business_port"]]),
    }

    if args.out_inventory:
        args.out_inventory.write_text(json.dumps(records, indent=2, ensure_ascii=True), encoding="utf-8")
    if args.out_blockers:
        args.out_blockers.write_text(json.dumps(blockers, indent=2, ensure_ascii=True), encoding="utf-8")
    if args.out_blocker_lines:
        write_lines(args.out_blocker_lines, blockers)

    print(json.dumps(summary, indent=2, ensure_ascii=True))
    return 1 if blockers else 0


def validate(args: argparse.Namespace) -> int:
    records = service_records(args.root)
    return validate_records(args, records, {"root": str(args.root)})


def validate_kubectl_json(args: argparse.Namespace) -> int:
    records = records_from_documents(iter_kubectl_payload_documents(args.input))
    return validate_records(args, records, {"input": str(args.input)})


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    validate_parser = subparsers.add_parser("validate")
    validate_parser.add_argument("--root", type=Path, required=True)
    validate_parser.add_argument("--out-inventory", type=Path)
    validate_parser.add_argument("--out-blockers", type=Path)
    validate_parser.add_argument("--out-blocker-lines", type=Path)
    validate_parser.set_defaults(func=validate)

    kubectl_parser = subparsers.add_parser("validate-kubectl-json")
    kubectl_parser.add_argument("--input", type=Path, required=True)
    kubectl_parser.add_argument("--out-inventory", type=Path)
    kubectl_parser.add_argument("--out-blockers", type=Path)
    kubectl_parser.add_argument("--out-blocker-lines", type=Path)
    kubectl_parser.set_defaults(func=validate_kubectl_json)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
