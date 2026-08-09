#!/usr/bin/env python3
"""Verify the nine built Flink Application JARs against the migration contract."""

from __future__ import annotations

import hashlib
import json
import zipfile
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/flink/application-cluster-migration.v1.json"


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify() -> dict[str, Any]:
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    errors: list[str] = []
    artifacts = []
    for job in contract["jobs"]:
        path = (
            ROOT / "java/flink-jobs" / job["module"] / "target"
            / f"{job['module']}-1.0.0-SNAPSHOT.jar"
        )
        main_entry = job["main_class"].replace(".", "/") + ".class"
        if not path.is_file():
            errors.append(f"{job['id']}: JAR does not exist")
            continue
        size = path.stat().st_size
        if size < 1024 * 1024:
            errors.append(f"{job['id']}: JAR is unexpectedly small ({size} bytes)")
            continue
        try:
            with zipfile.ZipFile(path) as jar:
                if main_entry not in jar.namelist():
                    errors.append(f"{job['id']}: missing main class {main_entry}")
        except zipfile.BadZipFile:
            errors.append(f"{job['id']}: invalid JAR/ZIP")
            continue
        artifacts.append(
            {
                "job_id": job["id"],
                "module": job["module"],
                "main_class": job["main_class"],
                "path": str(path.relative_to(ROOT)),
                "size_bytes": size,
                "sha256": _sha256(path),
            }
        )
    return {
        "schema_version": 1,
        "status": "PASS" if not errors and len(artifacts) == 9 else "FAIL",
        "expected_artifacts": 9,
        "verified_artifacts": len(artifacts),
        "artifacts": artifacts,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
