#!/usr/bin/env python3
"""Capture fail-closed SBOM and vulnerability evidence for projection tools.

The local mode is intentionally non-authorizing: it can prove that a locally
built image matches a G0 source snapshot and that its Go binaries were scanned,
but it can never substitute a pullable, signed repository manifest.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import tempfile
import urllib.parse
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
BINARIES = ("alert-projection-shadow", "alert-projection-reconcile")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
REPOSITORY_IMAGE_RE = re.compile(r"^[a-z0-9][a-z0-9./_:-]*@sha256:[0-9a-f]{64}$")
DB_UPDATED_RE = re.compile(r"^DB updated:\s+(.+)$", re.MULTILINE)
SCANNER_RE = re.compile(r"^Scanner:\s+govulncheck@([^\s]+)$", re.MULTILINE)
NONREACHABLE_RE = re.compile(r"found\s+\d+\s+vulnerabilit(?:y|ies).*?and\s+(\d+)\s+vulnerabilit", re.DOTALL)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_rfc3339(value: str) -> datetime:
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return parsed.astimezone(timezone.utc)


def run(command: list[str], *, cwd: Path = ROOT) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command, cwd=cwd, text=True, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, check=False,
    )


def load_object(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected a JSON object: {path}")
    return value


def inspect_image(image: str) -> dict[str, Any]:
    completed = run(["docker", "image", "inspect", image])
    if completed.returncode:
        raise RuntimeError(f"docker image inspect failed: {completed.stdout.strip()}")
    payload = json.loads(completed.stdout)
    if not isinstance(payload, list) or len(payload) != 1 or not isinstance(payload[0], dict):
        raise RuntimeError("docker image inspect returned an unexpected payload")
    return payload[0]


def validate_image_binding(
    *, inspection: dict[str, Any], image: str, mode: str,
    g0_content_sha256: str, g0_head: str,
) -> list[str]:
    labels = (inspection.get("Config") or {}).get("Labels") or {}
    errors: list[str] = []
    if labels.get("org.opencontainers.image.source-content-sha256") != g0_content_sha256:
        errors.append("image source-content label does not match G0")
    if labels.get("org.opencontainers.image.revision") != g0_head:
        errors.append("image revision label does not match G0")
    repo_digests = inspection.get("RepoDigests") or []
    if mode == "local":
        errors.extend([
            "local image has no approval-eligible repository manifest digest",
            "registry signature verification is absent",
            "SLSA provenance verification is absent",
        ])
    elif not REPOSITORY_IMAGE_RE.fullmatch(image) or image not in repo_digests:
        errors.append("published image is not the exact inspected repository@sha256 digest")
    return errors


def extract_binaries(image: str, destination: Path) -> dict[str, Path]:
    created = run(["docker", "create", image])
    if created.returncode:
        raise RuntimeError(f"docker create failed: {created.stdout.strip()}")
    container_id = created.stdout.strip()
    paths: dict[str, Path] = {}
    try:
        for name in BINARIES:
            target = destination / name
            copied = run(["docker", "cp", f"{container_id}:/usr/local/bin/{name}", str(target)])
            if copied.returncode:
                raise RuntimeError(f"docker cp failed for {name}: {copied.stdout.strip()}")
            paths[name] = target
    finally:
        run(["docker", "rm", container_id])
    return paths


def parse_go_version_modules(output: str) -> dict[str, Any]:
    go_version: str | None = None
    command_path: str | None = None
    main_module: dict[str, str] | None = None
    modules: list[dict[str, str]] = []
    for raw in output.splitlines():
        line = raw.strip()
        if not line:
            continue
        if not raw.startswith("\t") and ": go" in line:
            go_version = line.rsplit(": ", 1)[-1]
            continue
        fields = line.split("\t")
        if fields[0] == "path" and len(fields) >= 2:
            command_path = fields[1]
        elif fields[0] == "mod" and len(fields) >= 3:
            main_module = {"path": fields[1], "version": fields[2]}
        elif fields[0] == "dep" and len(fields) >= 3:
            item = {"path": fields[1], "version": fields[2]}
            if len(fields) >= 4 and fields[3]:
                item["go_sum"] = fields[3]
            modules.append(item)
    if not go_version or not command_path or not main_module:
        raise ValueError("go version -m output is missing Go, command or main-module metadata")
    modules.sort(key=lambda item: (item["path"], item["version"]))
    return {
        "go_version": go_version,
        "command_path": command_path,
        "main_module": main_module,
        "modules": modules,
    }


def module_ref(path: str, version: str) -> str:
    return f"pkg:golang/{urllib.parse.quote(path, safe='/')}@{urllib.parse.quote(version, safe='')}"


def build_cyclonedx(binary_metadata: dict[str, dict[str, Any]], binary_hashes: dict[str, str]) -> dict[str, Any]:
    components_by_ref: dict[str, dict[str, Any]] = {}
    dependencies: list[dict[str, Any]] = []
    for binary_name in sorted(binary_metadata):
        metadata = binary_metadata[binary_name]
        binary_ref = f"binary:{binary_name}:sha256:{binary_hashes[binary_name]}"
        components_by_ref[binary_ref] = {
            "type": "application", "bom-ref": binary_ref, "name": binary_name,
            "version": metadata["go_version"],
            "hashes": [{"alg": "SHA-256", "content": binary_hashes[binary_name]}],
            "properties": [{"name": "go.command.path", "value": metadata["command_path"]}],
        }
        module_items = [metadata["main_module"], *metadata["modules"]]
        refs: list[str] = []
        for item in module_items:
            ref = module_ref(item["path"], item["version"])
            refs.append(ref)
            component = {
                "type": "library", "bom-ref": ref, "name": item["path"],
                "version": item["version"], "purl": ref,
            }
            if item.get("go_sum"):
                component["properties"] = [{"name": "go.module.sum", "value": item["go_sum"]}]
            components_by_ref[ref] = component
        dependencies.append({"ref": binary_ref, "dependsOn": sorted(set(refs))})
    identity = "\n".join(f"{name}:{binary_hashes[name]}" for name in sorted(binary_hashes))
    serial = uuid.uuid5(uuid.NAMESPACE_URL, "traffic-analysis/alert-projection-tools/" + identity)
    return {
        "bomFormat": "CycloneDX", "specVersion": "1.5", "serialNumber": f"urn:uuid:{serial}",
        "version": 1,
        "metadata": {"component": {"type": "container", "name": "alert-projection-tools"}},
        "components": [components_by_ref[key] for key in sorted(components_by_ref)],
        "dependencies": sorted(dependencies, key=lambda item: item["ref"]),
    }


def sarif_error_count(payload: dict[str, Any]) -> int:
    count = 0
    for run_item in payload.get("runs") or []:
        if not isinstance(run_item, dict):
            continue
        for result in run_item.get("results") or []:
            if isinstance(result, dict) and result.get("level") == "error":
                count += 1
    return count


def assess_vulnerability_scan(
    *, text_returncode: int, text_output: str,
    sarif_returncode: int, sarif_output: str,
) -> dict[str, Any]:
    errors: list[str] = []
    if text_returncode not in (0, 3):
        errors.append(f"govulncheck text scan failed with exit code {text_returncode}")
    try:
        sarif = json.loads(sarif_output)
        if not isinstance(sarif, dict):
            raise ValueError("SARIF root is not an object")
        reachable = sarif_error_count(sarif)
    except (json.JSONDecodeError, ValueError):
        sarif = None
        reachable = 0
        errors.append("govulncheck SARIF output is invalid")
    if sarif_returncode not in (0, 3):
        errors.append(f"govulncheck SARIF scan failed with exit code {sarif_returncode}")
    if text_returncode == 3 and reachable == 0:
        errors.append("text scan reports reachable vulnerabilities but SARIF has no error result")
    if reachable > 0:
        errors.append(f"govulncheck found {reachable} reachable vulnerability result(s)")
    match = NONREACHABLE_RE.search(text_output)
    nonreachable = int(match.group(1)) if match else 0
    return {
        "status": "PASS" if not errors else "FAIL",
        "reachable_known_vulnerabilities": reachable,
        "nonreachable_module_vulnerabilities": nonreachable,
        "text_exit_code": text_returncode,
        "sarif_exit_code": sarif_returncode,
        "errors": errors,
    }


def scanner_metadata(version_output: str, *, now: datetime, maximum_db_age_seconds: int) -> dict[str, Any]:
    scanner_match = SCANNER_RE.search(version_output)
    db_match = DB_UPDATED_RE.search(version_output)
    errors: list[str] = []
    scanner_version = scanner_match.group(1) if scanner_match else None
    if not scanner_version:
        errors.append("govulncheck scanner version is unavailable")
    db_updated_at: str | None = None
    age_seconds: int | None = None
    if not db_match:
        errors.append("govulncheck database timestamp is unavailable")
    else:
        try:
            raw = db_match.group(1).replace(" +0000 UTC", "+00:00")
            updated = parse_rfc3339(raw)
            age_seconds = int((now.astimezone(timezone.utc) - updated).total_seconds())
            db_updated_at = updated.isoformat()
            if age_seconds < 0 or age_seconds > maximum_db_age_seconds:
                errors.append("govulncheck database is stale or dated in the future")
        except ValueError:
            errors.append("govulncheck database timestamp is invalid")
    return {
        "name": "govulncheck", "version": scanner_version,
        "database": "https://vuln.go.dev", "database_updated_at": db_updated_at,
        "database_age_seconds": age_seconds,
        "maximum_database_age_seconds": maximum_db_age_seconds,
        "status": "PASS" if not errors else "FAIL", "errors": errors,
    }


def verify_cosign(
    *, cosign: Path, image: str, signature_bundle: Path, provenance_bundle: Path,
    identity: str, issuer: str,
) -> list[str]:
    errors: list[str] = []
    commands = (
        [str(cosign), "verify", "--bundle", str(signature_bundle),
         "--certificate-identity", identity, "--certificate-oidc-issuer", issuer, image],
        [str(cosign), "verify-attestation", "--type", "slsaprovenance",
         "--bundle", str(provenance_bundle), "--certificate-identity", identity,
         "--certificate-oidc-issuer", issuer, image],
    )
    for command in commands:
        completed = run(command)
        if completed.returncode:
            errors.append(f"cosign verification failed: {completed.stdout.strip()}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--image", required=True)
    parser.add_argument("--govulncheck", type=Path, required=True)
    parser.add_argument("--mode", choices=("local", "published"), default="local")
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    parser.add_argument("--maximum-db-age-seconds", type=int, default=86_400)
    parser.add_argument("--nonreachable-adjudication", type=Path)
    parser.add_argument("--cosign", type=Path)
    parser.add_argument("--signature-bundle", type=Path)
    parser.add_argument("--provenance-bundle", type=Path)
    parser.add_argument("--certificate-identity")
    parser.add_argument("--certificate-issuer")
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    g0_path = args.g0_manifest.resolve()
    govulncheck = args.govulncheck.resolve()
    if not g0_path.is_file() or not govulncheck.is_file():
        raise SystemExit("G0 manifest and pinned govulncheck executable must exist")
    g0 = load_object(g0_path)
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("G0 manifest is not a passing G0 result")
    candidate = build_snapshot()
    g0_content = (g0.get("candidate_source") or {}).get("content_sha256")
    if candidate["content_sha256"] != g0_content:
        raise SystemExit("current source snapshot does not match the G0 manifest")

    inspection = inspect_image(args.image)
    labels = (inspection.get("Config") or {}).get("Labels") or {}
    head = (g0.get("candidate_before") or {}).get("head")
    repo_digests = inspection.get("RepoDigests") or []
    blockers = validate_image_binding(
        inspection=inspection, image=args.image, mode=args.mode,
        g0_content_sha256=g0_content, g0_head=head,
    )

    now = datetime.now(timezone.utc)
    version = run([str(govulncheck), "-version"])
    scanner = scanner_metadata(
        version.stdout, now=now,
        maximum_db_age_seconds=args.maximum_db_age_seconds,
    )
    blockers.extend(scanner["errors"])

    artifacts: dict[str, str] = {}
    scan_results: dict[str, Any] = {}
    with tempfile.TemporaryDirectory(prefix="alert-projection-supply-chain-") as temp_name:
        temp = Path(temp_name)
        binaries = extract_binaries(args.image, temp)
        binary_metadata: dict[str, dict[str, Any]] = {}
        binary_hashes: dict[str, str] = {}
        raw_artifacts: dict[str, str] = {}
        for name, binary in binaries.items():
            binary_hashes[name] = sha256(binary)
            metadata = run(["/usr/local/go/bin/go", "version", "-m", str(binary)], cwd=temp)
            if metadata.returncode:
                raise SystemExit(f"go version -m failed for {name}: {metadata.stdout.strip()}")
            binary_metadata[name] = parse_go_version_modules(metadata.stdout)
            text_scan = run([str(govulncheck), "-mode=binary", str(binary)], cwd=temp)
            sarif_scan = run([str(govulncheck), "-mode=binary", "-format=sarif", str(binary)], cwd=temp)
            assessment = assess_vulnerability_scan(
                text_returncode=text_scan.returncode, text_output=text_scan.stdout,
                sarif_returncode=sarif_scan.returncode, sarif_output=sarif_scan.stdout,
            )
            scan_results[name] = assessment
            blockers.extend(f"{name}: {item}" for item in assessment["errors"])
            if assessment["nonreachable_module_vulnerabilities"] and args.nonreachable_adjudication is None:
                blockers.append(f"{name}: non-reachable module finding lacks security adjudication")
            raw_artifacts[f"{name}.govulncheck.txt"] = text_scan.stdout
            raw_artifacts[f"{name}.govulncheck.sarif.json"] = sarif_scan.stdout

        sbom = build_cyclonedx(binary_metadata, binary_hashes)
        signature = {"verified": False, "bundle_sha256": None, "identity": None, "issuer": None}
        provenance = {"verified": False, "bundle_sha256": None, "type": "slsaprovenance"}
        if args.mode == "published":
            required = (
                args.cosign, args.signature_bundle, args.provenance_bundle,
                args.certificate_identity, args.certificate_issuer,
            )
            if not all(required):
                blockers.append("published mode requires cosign, signature, provenance, identity and issuer")
            else:
                cosign_errors = verify_cosign(
                    cosign=args.cosign.resolve(), image=args.image,
                    signature_bundle=args.signature_bundle.resolve(),
                    provenance_bundle=args.provenance_bundle.resolve(),
                    identity=args.certificate_identity, issuer=args.certificate_issuer,
                )
                blockers.extend(cosign_errors)
                signature = {
                    "verified": not cosign_errors,
                    "bundle_sha256": sha256(args.signature_bundle.resolve()),
                    "identity": args.certificate_identity, "issuer": args.certificate_issuer,
                }
                provenance = {
                    "verified": not cosign_errors,
                    "bundle_sha256": sha256(args.provenance_bundle.resolve()),
                    "type": "slsaprovenance",
                }

        output.mkdir(parents=True)
        sbom_path = output / "alert-projection-tools.cdx.json"
        sbom_path.write_text(json.dumps(sbom, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        artifacts["sbom"] = sha256(sbom_path)
        for filename, content in raw_artifacts.items():
            path = output / filename
            path.write_text(content, encoding="utf-8")
            artifacts[filename] = sha256(path)

    adjudication_sha = None
    if args.nonreachable_adjudication is not None:
        adjudication_path = args.nonreachable_adjudication.resolve()
        if not adjudication_path.is_file():
            blockers.append("non-reachable security adjudication file does not exist")
        else:
            adjudication_sha = sha256(adjudication_path)
    approval_eligible = args.mode == "published" and not blockers
    manifest = {
        "schema_version": 1, "run_id": args.run_id,
        "gate": "G1_ALERT_PROJECTION_TOOL_SUPPLY_CHAIN",
        "status": "PASS" if approval_eligible else "HOLD",
        "coverage_status": "PUBLISHED_SIGNED_SCANNED" if approval_eligible else "PREPUBLISH_NON_AUTHORIZING",
        "captured_at": now.isoformat(),
        "candidate_source": {"head": head, "content_sha256": candidate["content_sha256"], "file_count": candidate["file_count"]},
        "g0": {"run_id": g0.get("run_id"), "manifest": str(g0_path.relative_to(ROOT)), "manifest_sha256": sha256(g0_path)},
        "image": {
            "reference": args.image, "local_image_id": inspection.get("Id"),
            "repository_digests": repo_digests, "labels": labels,
        },
        "sbom": {"format": "CycloneDX", "spec_version": "1.5", "sha256": artifacts["sbom"]},
        "scanner": scanner, "binary_scans": scan_results,
        "signature": signature, "provenance": provenance,
        "nonreachable_security_adjudication_sha256": adjudication_sha,
        "artifact_sha256": artifacts,
        "approval_eligible": approval_eligible, "blockers": sorted(set(blockers)),
        "production_applied": False, "production_mutations": [],
        "registry_push_attempted": False, "cluster_access_attempted": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"], "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path), "approval_eligible": approval_eligible,
        "blocker_count": len(manifest["blockers"]),
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
