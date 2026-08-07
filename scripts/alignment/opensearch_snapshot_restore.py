#!/usr/bin/env python3
"""Plan, execute and verify guarded OpenSearch projection snapshots/restores.

Every mutating operation is plan-only unless --execute is supplied. Restore
execution additionally requires a hash-pinned, unexpired approval manifest and
an isolated target whose UUID differs from the source cluster.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


SAFE_NAME = re.compile(r"^[a-z0-9][a-z0-9._-]{0,254}$")
REQUIRED_VERIFICATION = {
    "mapping_sha256",
    "settings_sha256",
    "aliases",
    "document_count",
    "sample_document_ids",
    "sample_versions",
    "sample_content_sha256",
    "query_oracle",
}


class GuardError(RuntimeError):
    pass


def canonical_sha256(value: Any) -> str:
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def file_sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def endpoint_sha256(endpoint: str) -> str:
    normalized = endpoint.rstrip("/").lower()
    return hashlib.sha256(normalized.encode()).hexdigest()


def parse_timestamp(value: str) -> datetime:
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise GuardError(f"invalid ISO-8601 timestamp: {value}") from exc
    if parsed.tzinfo is None:
        raise GuardError("approval expiry must include a timezone")
    return parsed.astimezone(timezone.utc)


@dataclass
class OpenSearchClient:
    endpoint: str
    ca_file: str | None = None
    cert_file: str | None = None
    key_file: str | None = None
    username: str | None = None
    password: str | None = None
    timeout: float = 20.0

    def __post_init__(self) -> None:
        parsed = urllib.parse.urlsplit(self.endpoint)
        if parsed.scheme != "https":
            raise GuardError("OpenSearch endpoint must use https")
        if not parsed.hostname:
            raise GuardError("OpenSearch endpoint must contain a hostname")
        if bool(self.cert_file) != bool(self.key_file):
            raise GuardError("client certificate and key must be supplied together")
        self.endpoint = self.endpoint.rstrip("/")
        self.context = ssl.create_default_context(cafile=self.ca_file)
        if self.cert_file and self.key_file:
            self.context.load_cert_chain(self.cert_file, self.key_file)

    def request(self, method: str, path: str, body: Any | None = None) -> Any:
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body, separators=(",", ":")).encode()
            headers["Content-Type"] = "application/json"
        if self.username is not None:
            token = base64.b64encode(f"{self.username}:{self.password or ''}".encode()).decode()
            headers["Authorization"] = f"Basic {token}"
        request = urllib.request.Request(self.endpoint + path, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, context=self.context, timeout=self.timeout) as response:
                payload = response.read()
        except urllib.error.HTTPError as exc:
            detail = exc.read(2048).decode(errors="replace")
            raise GuardError(f"OpenSearch {method} {path} returned HTTP {exc.code}: {detail}") from exc
        except (urllib.error.URLError, TimeoutError) as exc:
            raise GuardError(f"OpenSearch {method} {path} failed: {exc}") from exc
        if not payload:
            return {}
        try:
            return json.loads(payload)
        except json.JSONDecodeError as exc:
            raise GuardError(f"OpenSearch {method} {path} returned non-JSON data") from exc

    def identity(self) -> dict[str, str]:
        root = self.request("GET", "/")
        uuid = str(root.get("cluster_uuid", ""))
        name = str(root.get("cluster_name", ""))
        if not uuid or uuid == "_na_" or not name:
            raise GuardError("cluster identity is incomplete")
        return {"cluster_uuid": uuid, "cluster_name": name}


def validate_name(kind: str, value: str) -> None:
    if not SAFE_NAME.fullmatch(value):
        raise GuardError(f"unsafe {kind}: {value!r}")


def validate_indices(indices: list[str]) -> None:
    if not indices:
        raise GuardError("at least one explicit index is required")
    for index in indices:
        validate_name("index", index)
        if "*" in index or index.startswith("."):
            raise GuardError("wildcard and system indices are forbidden")


def load_approval(path: Path, expected_sha256: str | None, require_hash: bool) -> dict[str, Any]:
    if not path.is_file():
        raise GuardError(f"approval manifest does not exist: {path}")
    actual = file_sha256(path)
    if require_hash and not expected_sha256:
        raise GuardError("restore execution requires --approved-manifest-sha256")
    if expected_sha256 and actual != expected_sha256.lower():
        raise GuardError("approval manifest SHA-256 mismatch")
    try:
        manifest = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise GuardError("approval manifest is not valid JSON") from exc
    if manifest.get("approval_status") != "APPROVED" or manifest.get("operation") != "opensearch_isolated_restore":
        raise GuardError("manifest is not an approved isolated restore")
    for field in ("approval_id", "approved_by", "expires_at", "source_endpoint_sha256", "target_endpoint_sha256",
                  "source_cluster_uuid", "target_cluster_uuid", "repository", "snapshot", "indices",
                  "rename_pattern", "rename_replacement", "verification"):
        if field not in manifest:
            raise GuardError(f"approval manifest missing {field}")
    if parse_timestamp(str(manifest["expires_at"])) <= datetime.now(timezone.utc):
        raise GuardError("approval manifest is expired")
    if manifest.get("target_isolated") is not True:
        raise GuardError("approval does not attest an isolated target")
    validate_name("repository", str(manifest["repository"]))
    validate_name("snapshot", str(manifest["snapshot"]))
    validate_indices([str(item) for item in manifest["indices"]])
    verification = manifest.get("verification") or {}
    declared = set(verification.get("required", []))
    if declared != REQUIRED_VERIFICATION:
        raise GuardError("approval verification requirements are incomplete")
    if not isinstance(verification.get("indices"), list) or not verification["indices"]:
        raise GuardError("approval must contain per-index verification oracles")
    if manifest["rename_pattern"] != "^(.+)$":
        raise GuardError("restore rename_pattern must be the bounded whole-index capture")
    replacement = str(manifest["rename_replacement"])
    if not re.fullmatch(r"restore-[a-z0-9][a-z0-9-]{0,40}-\$1", replacement):
        raise GuardError("restore rename_replacement must use an approved restore-<run>-$1 prefix")
    expected_pairs = {
        (str(item.get("source_index", "")), str(item.get("restored_index", "")))
        for item in verification["indices"]
    }
    approved_pairs = {(index, replacement.replace("$1", index)) for index in manifest["indices"]}
    if expected_pairs != approved_pairs:
        raise GuardError("per-index verification targets do not match the approved rename rule")
    for _, restored in approved_pairs:
        validate_name("restored index", restored)
    return manifest


def snapshot_state(client: OpenSearchClient, repository: str, snapshot: str) -> dict[str, Any]:
    payload = client.request("GET", f"/_snapshot/{urllib.parse.quote(repository)}/{urllib.parse.quote(snapshot)}")
    snapshots = payload.get("snapshots") or []
    if len(snapshots) != 1:
        raise GuardError("snapshot lookup did not return exactly one snapshot")
    return snapshots[0]


def snapshot_operation(args: argparse.Namespace, client: OpenSearchClient) -> dict[str, Any]:
    validate_name("repository", args.repository)
    validate_name("snapshot", args.snapshot)
    validate_indices(args.indices)
    identity = client.identity()
    plan = {
        "mode": "execute" if args.execute else "plan",
        "operation": "snapshot",
        "cluster": identity,
        "endpoint_sha256": endpoint_sha256(args.endpoint),
        "repository": args.repository,
        "snapshot": args.snapshot,
        "indices": args.indices,
        "include_global_state": False,
        "partial": False,
        "mutations": [],
    }
    try:
        existing = snapshot_state(client, args.repository, args.snapshot)
    except GuardError as exc:
        if "HTTP 404" not in str(exc):
            raise
        existing = None
    if existing:
        raise GuardError("snapshot already exists; immutable snapshots are never overwritten")
    client.request("GET", f"/_snapshot/{urllib.parse.quote(args.repository)}")
    if not args.execute:
        return plan
    if not args.operator or not args.reason or not args.approval_id:
        raise GuardError("snapshot execution requires --operator, --reason and --approval-id")
    body = {
        "indices": ",".join(args.indices),
        "ignore_unavailable": False,
        "include_global_state": False,
        "partial": False,
        "metadata": {"operator": args.operator, "reason": args.reason, "approval_id": args.approval_id},
    }
    response = client.request("PUT", f"/_snapshot/{args.repository}/{args.snapshot}?wait_for_completion=true", body)
    state = (response.get("snapshot") or {}).get("state")
    if state != "SUCCESS":
        raise GuardError(f"snapshot did not complete successfully: {state}")
    plan["mutations"] = ["snapshot_created"]
    plan["result"] = response
    return plan


def validate_restore_guards(args: argparse.Namespace, source: OpenSearchClient, target: OpenSearchClient,
                            manifest: dict[str, Any]) -> tuple[dict[str, str], dict[str, str], dict[str, Any]]:
    if endpoint_sha256(args.source_endpoint) != manifest["source_endpoint_sha256"]:
        raise GuardError("source endpoint does not match approval")
    if endpoint_sha256(args.target_endpoint) != manifest["target_endpoint_sha256"]:
        raise GuardError("target endpoint does not match approval")
    source_id = source.identity()
    target_id = target.identity()
    if source_id["cluster_uuid"] == target_id["cluster_uuid"]:
        raise GuardError("same-cluster restore is forbidden")
    if source_id["cluster_uuid"] != manifest["source_cluster_uuid"]:
        raise GuardError("source cluster UUID does not match approval")
    if target_id["cluster_uuid"] != manifest["target_cluster_uuid"]:
        raise GuardError("target cluster UUID does not match approval")
    state = snapshot_state(source, manifest["repository"], manifest["snapshot"])
    if state.get("state") != "SUCCESS":
        raise GuardError("only a SUCCESS snapshot may be restored")
    if sorted(state.get("indices") or []) != sorted(manifest["indices"]):
        raise GuardError("snapshot indices differ from approval")
    for oracle in manifest["verification"]["indices"]:
        restored = urllib.parse.quote(str(oracle["restored_index"]))
        try:
            target.request("GET", f"/{restored}/_settings")
        except GuardError as exc:
            if "HTTP 404" not in str(exc):
                raise
        else:
            raise GuardError(f"restore target index already exists: {oracle['restored_index']}")
    return source_id, target_id, state


def index_payload(client: OpenSearchClient, index: str) -> dict[str, Any]:
    quoted = urllib.parse.quote(index)
    mapping = client.request("GET", f"/{quoted}/_mapping")
    settings = client.request("GET", f"/{quoted}/_settings?flat_settings=true&include_defaults=false")
    aliases = client.request("GET", f"/{quoted}/_alias")
    count = client.request("GET", f"/{quoted}/_count")
    return {
        "mapping": (mapping.get(index) or {}).get("mappings", {}),
        "settings": (settings.get(index) or {}).get("settings", {}),
        "aliases": (aliases.get(index) or {}).get("aliases", {}),
        "count": int(count.get("count", -1)),
    }


def verify_restore(target: OpenSearchClient, manifest: dict[str, Any]) -> dict[str, Any]:
    failures: list[dict[str, Any]] = []
    checked: list[dict[str, Any]] = []
    for oracle in manifest["verification"]["indices"]:
        source_index = str(oracle.get("source_index", ""))
        restored_index = str(oracle.get("restored_index", ""))
        validate_name("source index", source_index)
        validate_name("restored index", restored_index)
        actual = index_payload(target, restored_index)
        comparisons = {
            "mapping_sha256": canonical_sha256(actual["mapping"]),
            "settings_sha256": canonical_sha256(actual["settings"]),
            "aliases": sorted(actual["aliases"].keys()),
            "document_count": actual["count"],
        }
        expected = oracle.get("expected") or {}
        for field in ("mapping_sha256", "settings_sha256", "aliases", "document_count"):
            if comparisons[field] != expected.get(field):
                failures.append({"index": restored_index, "field": field, "expected": expected.get(field), "actual": comparisons[field]})
        samples = []
        for sample in oracle.get("samples", []):
            document_id = str(sample.get("id", ""))
            document = target.request("GET", f"/{urllib.parse.quote(restored_index)}/_doc/{urllib.parse.quote(document_id)}")
            observed = {
                "id": document_id,
                "version": document.get("_version"),
                "content_sha256": canonical_sha256(document.get("_source")),
            }
            samples.append(observed)
            for field in ("version", "content_sha256"):
                if observed[field] != sample.get(field):
                    failures.append({"index": restored_index, "id": document_id, "field": field,
                                     "expected": sample.get(field), "actual": observed[field]})
        query_results = []
        for query in oracle.get("queries", []):
            body = query.get("body")
            if not isinstance(body, dict) or int(body.get("size", 0)) > 1000:
                raise GuardError("query oracle must be a bounded JSON search with size <= 1000")
            response = target.request("POST", f"/{urllib.parse.quote(restored_index)}/_search", body)
            hits = response.get("hits") or {}
            total = hits.get("total", 0)
            total_value = total.get("value", 0) if isinstance(total, dict) else total
            ids = sorted(str(hit.get("_id")) for hit in hits.get("hits", []))
            observed = {"name": query.get("name"), "total": total_value, "ids": ids}
            query_results.append(observed)
            if total_value != query.get("expected_total") or ids != sorted(query.get("expected_ids", [])):
                failures.append({"index": restored_index, "query": query.get("name"), "expected_total": query.get("expected_total"),
                                 "actual_total": total_value, "expected_ids": sorted(query.get("expected_ids", [])), "actual_ids": ids})
        checked.append({"source_index": source_index, "restored_index": restored_index,
                        "comparisons": comparisons, "samples": samples, "queries": query_results})
    return {"status": "PASS" if not failures else "FAIL", "checked": checked, "failures": failures}


def restore_operation(args: argparse.Namespace, source: OpenSearchClient, target: OpenSearchClient) -> dict[str, Any]:
    manifest = load_approval(args.approved_manifest.resolve(), args.approved_manifest_sha256, args.execute)
    source_id, target_id, state = validate_restore_guards(args, source, target, manifest)
    plan = {
        "mode": "execute" if args.execute else "plan",
        "operation": "isolated_restore",
        "approval_id": manifest["approval_id"],
        "source_cluster": source_id,
        "target_cluster": target_id,
        "repository": manifest["repository"],
        "snapshot": manifest["snapshot"],
        "snapshot_state": state.get("state"),
        "indices": manifest["indices"],
        "rename_pattern": manifest["rename_pattern"],
        "rename_replacement": manifest["rename_replacement"],
        "mutations": [],
    }
    if not args.execute:
        return plan
    if not args.operator or not args.reason:
        raise GuardError("restore execution requires --operator and --reason")
    target.request("GET", f"/_snapshot/{urllib.parse.quote(manifest['repository'])}")
    body = {
        "indices": ",".join(manifest["indices"]),
        "ignore_unavailable": False,
        "include_global_state": False,
        "include_aliases": True,
        "rename_pattern": manifest["rename_pattern"],
        "rename_replacement": manifest["rename_replacement"],
        "index_settings": {"index.blocks.read_only": True},
    }
    response = target.request("POST", f"/_snapshot/{manifest['repository']}/{manifest['snapshot']}/_restore?wait_for_completion=true", body)
    if response.get("accepted") is False:
        raise GuardError("restore was not accepted")
    verification = verify_restore(target, manifest)
    if verification["status"] != "PASS":
        raise GuardError(f"restore verification failed: {json.dumps(verification['failures'], ensure_ascii=False)}")
    plan["mutations"] = ["isolated_restore_created"]
    plan["result"] = response
    plan["verification"] = verification
    return plan


def add_client_flags(parser: argparse.ArgumentParser, prefix: str = "") -> None:
    label = f"{prefix}-" if prefix else ""
    parser.add_argument(f"--{label}ca-file")
    parser.add_argument(f"--{label}cert-file")
    parser.add_argument(f"--{label}key-file")
    parser.add_argument(f"--{label}username")
    parser.add_argument(f"--{label}password-env")


def client_from_args(args: argparse.Namespace, endpoint_attr: str, prefix: str = "") -> OpenSearchClient:
    key = f"{prefix}_" if prefix else ""
    password_env = getattr(args, key + "password_env", None)
    password = os.environ.get(password_env) if password_env else None
    if password_env and password is None:
        raise GuardError(f"credential environment variable is absent: {password_env}")
    return OpenSearchClient(
        endpoint=getattr(args, endpoint_attr),
        ca_file=getattr(args, key + "ca_file", None),
        cert_file=getattr(args, key + "cert_file", None),
        key_file=getattr(args, key + "key_file", None),
        username=getattr(args, key + "username", None),
        password=password,
        timeout=args.timeout,
    )


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser()
    root.add_argument("--timeout", type=float, default=20.0)
    commands = root.add_subparsers(dest="command", required=True)
    snapshot = commands.add_parser("snapshot")
    snapshot.add_argument("--endpoint", required=True)
    snapshot.add_argument("--repository", required=True)
    snapshot.add_argument("--snapshot", required=True)
    snapshot.add_argument("--indices", nargs="+", required=True)
    snapshot.add_argument("--execute", action="store_true")
    snapshot.add_argument("--operator")
    snapshot.add_argument("--reason")
    snapshot.add_argument("--approval-id")
    add_client_flags(snapshot)
    restore = commands.add_parser("restore")
    restore.add_argument("--source-endpoint", required=True)
    restore.add_argument("--target-endpoint", required=True)
    restore.add_argument("--approved-manifest", type=Path, required=True)
    restore.add_argument("--approved-manifest-sha256")
    restore.add_argument("--execute", action="store_true")
    restore.add_argument("--operator")
    restore.add_argument("--reason")
    add_client_flags(restore, "source")
    add_client_flags(restore, "target")
    return root


def main() -> int:
    args = parser().parse_args()
    try:
        if args.command == "snapshot":
            result = snapshot_operation(args, client_from_args(args, "endpoint"))
        else:
            source = client_from_args(args, "source_endpoint", "source")
            target = client_from_args(args, "target_endpoint", "target")
            result = restore_operation(args, source, target)
        print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
        return 0
    except GuardError as exc:
        print(json.dumps({"status": "BLOCKED", "error": str(exc), "mutations": []}, ensure_ascii=False), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
