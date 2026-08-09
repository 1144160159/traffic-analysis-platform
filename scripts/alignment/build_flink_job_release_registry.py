#!/usr/bin/env python3
"""Materialize a digest-bound T-FLINK-005 release registry for nine Flink jobs."""

from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from verify_flink_application_artifacts import verify as verify_artifacts
from verify_flink_job_registry import (
    APPLICATION_PATH,
    CHECKPOINT_PATH,
    IMAGE_RE,
    REGISTRY_PATH,
    SINK_PATH,
    SHA_RE,
    STATE_PATH,
    contract_sha256,
    expected_jobs,
    load,
    verify,
)


ROOT = Path(__file__).resolve().parents[2]


def _by_job(items: Any, key: str, label: str) -> dict[str, dict[str, Any]]:
    if not isinstance(items, list):
        raise ValueError(f"{label}.jobs must be a list")
    result: dict[str, dict[str, Any]] = {}
    for item in items:
        if not isinstance(item, dict) or not isinstance(item.get(key), str):
            raise ValueError(f"{label}.jobs contains an invalid record")
        value = item[key]
        if value in result:
            raise ValueError(f"{label}.jobs contains duplicate {value}")
        result[value] = item
    return result


def build(
    image_manifest: dict[str, Any],
    savepoint_manifest: dict[str, Any],
    g0_manifest: dict[str, Any],
    artifacts: dict[str, Any],
) -> dict[str, Any]:
    registry = load(REGISTRY_PATH)
    application = load(APPLICATION_PATH)
    state = load(STATE_PATH)
    checkpoint = load(CHECKPOINT_PATH)
    sink = load(SINK_PATH)
    expected = expected_jobs(registry, application, state)
    canonical = set(expected)

    if g0_manifest.get("gate") != "G0" or g0_manifest.get("status") != "PASS":
        raise ValueError("G0 manifest must be PASS")
    candidate_hash = (g0_manifest.get("candidate_source") or {}).get("content_sha256")
    if not isinstance(candidate_hash, str) or len(candidate_hash) != 64:
        raise ValueError("G0 manifest has no candidate content sha256")
    if artifacts.get("status") != "PASS":
        raise ValueError("all nine Flink artifacts must pass verification")

    images = _by_job(image_manifest.get("jobs"), "job_id", "image manifest")
    artifact_by_id = _by_job(artifacts.get("artifacts"), "job_id", "artifact manifest")
    savepoints = savepoint_manifest.get("savepoints") or {}
    if savepoint_manifest.get("schema_version") != 1:
        raise ValueError("savepoint manifest must use schema_version 1")
    if savepoint_manifest.get("source_cluster_id") != application.get("source_session_cluster_id"):
        raise ValueError("savepoint manifest source cluster does not match")
    if set(images) != canonical or set(artifact_by_id) != canonical or set(savepoints) != canonical:
        raise ValueError("image, artifact and savepoint job sets must exactly match the registry")

    jobs = []
    for job_id in [item["id"] for item in registry["jobs"]]:
        image = images[job_id].get("image_digest")
        if not IMAGE_RE.fullmatch(str(image or "")):
            raise ValueError(f"{job_id}: image_digest must be immutable repository@sha256")
        savepoint = savepoints[job_id]
        savepoint_uri = str(savepoint.get("uri", ""))
        savepoint_sha256 = str(savepoint.get("sha256", ""))
        expected_prefix = f"{application['state']['savepoint_root']}/{job_id}/"
        if not savepoint_uri.startswith(expected_prefix) or ".." in savepoint_uri:
            raise ValueError(f"{job_id}: savepoint URI is outside the immutable job prefix")
        if not SHA_RE.fullmatch(savepoint_sha256):
            raise ValueError(f"{job_id}: savepoint sha256 is invalid")
        item = dict(expected[job_id])
        item.update(
            {
                "artifact_sha256": artifact_by_id[job_id]["sha256"],
                "image_digest": image,
                "savepoint_uri": savepoint_uri,
                "savepoint_sha256": savepoint_sha256,
            }
        )
        jobs.append(item)

    release = {
        "schema_version": 1,
        "registry_id": registry["contract_id"],
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate_source_sha256": candidate_hash,
        "contract_sha256": contract_sha256(registry),
        "source_g0_run_id": g0_manifest.get("run_id"),
        "production_applied": False,
        "jobs": jobs,
    }
    validation = verify(
        registry, application, state, checkpoint, sink, release=release
    )
    if validation["status"] != "PASS":
        raise ValueError("release registry validation failed: " + "; ".join(validation["errors"]))
    release["validation"] = validation
    return release


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--image-manifest", type=Path, required=True)
    parser.add_argument("--savepoint-manifest", type=Path, required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable release registry: {output}")
    release = build(
        load(args.image_manifest.resolve()),
        load(args.savepoint_manifest.resolve()),
        load(args.g0_manifest.resolve()),
        verify_artifacts(),
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(release, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": str(output), "jobs": len(release["jobs"])}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
