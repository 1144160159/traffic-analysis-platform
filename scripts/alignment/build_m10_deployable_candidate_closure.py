#!/usr/bin/env python3
"""Build the fail-closed T1-M10-N001 deployable-candidate closure."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from scripts.alignment.candidate_snapshot import build_snapshot


OUTPUT_RELATIVE = Path("deployments/releases/topic1/m10-deployable-candidate-closure.v1.json")
M01_POINTER = Path("contracts/releases/topic1/t1-m01-release-pointer.json")
M09_POINTER = Path("contracts/releases/topic1/t1-m09-release-pointer.json")
M09_BOM = Path("contracts/alignment/m09-integrated-bom.v1.json")
DIMENSION_PATHS: dict[str, tuple[str, ...]] = {
    "source_tree": (
        "scripts/alignment/candidate_snapshot.py",
        "contracts/alignment/task-registry.v1.json",
    ),
    "image_digests": (
        "deployments/kubernetes/image-digests.lock.json",
    ),
    "config": (
        "deployments/kubernetes/site-values.template.yaml",
        "deployments/kubernetes/site-values.v1.template.yaml",
        "deployments/kubernetes/applications/go-services.yaml",
        "deployments/kubernetes/applications/probe-agent.yaml",
        "deployments/kubernetes/applications/web-ui.yaml",
    ),
    "schema": (
        "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
        "deployments/kubernetes/init-jobs/03a-clickhouse-authoritative-migrations.yaml",
        "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml",
        "deployments/kubernetes/init-jobs/05-nebula-schema.yaml",
    ),
    "topics": (
        "contracts/events/kafka-topic-catalog.v1.json",
        "common/kafka/create-topics.sh",
        "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
    ),
    "models": (
        "contracts/mlops/model-artifact-manifest.schema.json",
        "contracts/mlops/model-consumer-profile.v1.json",
    ),
    "thresholds": (
        "contracts/mlops/m08-model-inference-parity-internal.v1.json",
        "contracts/mlops/tenant-model-canary-policy.schema.json",
    ),
    "runbooks": (
        "deployments/kubernetes/deploy.sh",
        "deployments/kubernetes/security/README.md",
        "doc/07_alignment/runbooks/T1-M09-N024-integrated-bom-no-go.md",
    ),
}


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def canonical_sha256(value: Any) -> str:
    return sha256_bytes(
        json.dumps(value, sort_keys=True, separators=(",", ":")).encode("utf-8")
    )


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON object required: {path}")
    return value


def probe(root: Path, relative: Path) -> dict[str, Any]:
    path = root / relative
    if not path.is_file():
        return {"path": str(relative), "exists": False, "sha256": None}
    return {"path": str(relative), "exists": True, "sha256": sha256_file(path)}


def git_state(root: Path) -> dict[str, Any]:
    def run(*args: str) -> str:
        return subprocess.run(
            ["git", *args], cwd=root, check=True, text=True, capture_output=True
        ).stdout.strip()

    raw = run("status", "--porcelain=v1", "--untracked-files=all").splitlines()
    ignored_suffix = str(OUTPUT_RELATIVE)
    status = sorted(line for line in raw if not line.endswith(ignored_suffix))
    return {
        "head": run("rev-parse", "HEAD"),
        "branch": run("branch", "--show-current"),
        "dirty_count": len(status),
        "status_sha256": sha256_bytes("\n".join(status).encode("utf-8")),
    }


def image_lock_summary(root: Path) -> dict[str, int]:
    path = root / DIMENSION_PATHS["image_digests"][0]
    if not path.is_file():
        return {"entry_count": 0, "pullable_digest_count": 0, "mutable_count": 0}
    images = load_json(path).get("images", [])
    if not isinstance(images, list):
        raise ValueError("image digest lock images must be an array")
    return {
        "entry_count": len(images),
        "pullable_digest_count": sum(
            isinstance(item, dict) and bool(item.get("repo_digest")) for item in images
        ),
        "mutable_count": sum(
            isinstance(item, dict) and item.get("mutable_tag") is True for item in images
        ),
    }


def dimension(
    root: Path,
    name: str,
    *,
    status: str,
    blocking_codes: list[str],
    detail: dict[str, Any] | None = None,
) -> dict[str, Any]:
    refs = [probe(root, Path(relative)) for relative in DIMENSION_PATHS[name]]
    if any(not item["exists"] for item in refs):
        status = "MISSING"
        blocking_codes = sorted(set([*blocking_codes, f"{name.upper()}_INPUT_REQUIRED"]))
    return {
        "name": name,
        "status": status,
        "refs": refs,
        "detail": detail or {},
        "blocking_codes": blocking_codes,
    }


def upstream_state(root: Path) -> tuple[dict[str, Any], dict[str, Any], list[str]]:
    m01 = probe(root, M01_POINTER)
    m09 = probe(root, M09_POINTER)
    blockers: list[str] = []
    if not m01["exists"]:
        blockers.append("UPSTREAM_M01_RELEASE_POINTER_REQUIRED")
    if not m09["exists"]:
        blockers.append("UPSTREAM_M09_RELEASE_POINTER_REQUIRED")
    else:
        pointer = load_json(root / M09_POINTER)
        m09["status"] = pointer.get("status")
        m09["candidate_id"] = pointer.get("candidate_id")
        if pointer.get("status") != "GO" or not pointer.get("candidate_id"):
            blockers.append("UPSTREAM_M09_SAME_CANDIDATE_REQUIRED")
    return m01, m09, blockers


def build(root: Path = ROOT) -> dict[str, Any]:
    source = build_snapshot()
    git = git_state(root)
    m01, m09, blockers = upstream_state(root)
    if git["dirty_count"]:
        blockers.append("CLEAN_SOURCE_COMMIT_REQUIRED")
    dimensions = [
        dimension(
            root,
            "source_tree",
            status="PRESENT_UNBOUND",
            blocking_codes=["CLEAN_SOURCE_COMMIT_REQUIRED"] if git["dirty_count"] else [],
            detail={
                "content_sha256": source["content_sha256"],
                "file_count": source["file_count"],
                "algorithm": source["algorithm"],
                "git": git,
            },
        ),
        dimension(
            root,
            "image_digests",
            status="PRESENT_UNBOUND",
            blocking_codes=["SINGLE_CANDIDATE_IMAGE_SET_REQUIRED"],
            detail=image_lock_summary(root),
        ),
        dimension(root, "config", status="PRESENT_UNBOUND", blocking_codes=["SITE_VALUES_BINDING_REQUIRED"]),
        dimension(root, "schema", status="PRESENT_UNBOUND", blocking_codes=["SCHEMA_MIGRATION_SET_BINDING_REQUIRED"]),
        dimension(root, "topics", status="PRESENT_UNBOUND", blocking_codes=["TOPIC_CATALOG_BINDING_REQUIRED"]),
        dimension(root, "models", status="PRESENT_UNBOUND", blocking_codes=["APPROVED_MODEL_MANIFEST_REQUIRED"]),
        dimension(root, "thresholds", status="PRESENT_UNBOUND", blocking_codes=["APPROVED_THRESHOLD_SET_REQUIRED"]),
        dimension(root, "runbooks", status="PRESENT_UNBOUND", blocking_codes=["CANDIDATE_RUNBOOK_SET_REQUIRED"]),
    ]
    blockers.extend(code for item in dimensions for code in item["blocking_codes"])
    blocking_codes = sorted(set(blockers))
    basis = {
        "source_tree_content_sha256": source["content_sha256"],
        "git_head": git["head"],
        "upstream": [m01, m09, probe(root, M09_BOM)],
        "dimensions": dimensions,
    }
    all_bound = all(item["status"] == "BOUND" for item in dimensions)
    candidate_id = canonical_sha256(basis) if all_bound and not blocking_codes else None
    return {
        "schema_version": 1,
        "artifact_kind": "M10_DEPLOYABLE_CANDIDATE_CLOSURE",
        "task_id": "T1-M10-N001",
        "atomic_pr_id": "T1-M10-P001-CTR-n001-s1",
        "status": "FROZEN" if candidate_id else "BLOCKED_INCOMPLETE",
        "candidate_id": candidate_id,
        "closure_basis_sha256": canonical_sha256(basis),
        "environment_kind": "KUBERNETES",
        "source_tree": {
            "content_sha256": source["content_sha256"],
            "file_count": source["file_count"],
            "algorithm": source["algorithm"],
            "git": git,
        },
        "upstream": {
            "m01_release_pointer": m01,
            "m09_release_pointer": m09,
            "m09_integrated_bom": probe(root, M09_BOM),
        },
        "dimensions": dimensions,
        "closure": {
            "required_dimension_count": len(DIMENSION_PATHS),
            "bound_dimension_count": sum(item["status"] == "BOUND" for item in dimensions),
            "all_dimensions_bound": all_bound,
            "fail_closed": candidate_id is None,
        },
        "blocking_codes": blocking_codes,
        "allowed_claims": [
            "The eight deployable-candidate dimensions were inventoried with exact hashes",
            "The current closure basis is deterministic and fail-closed",
        ],
        "forbidden_claims": [
            "A clean implementation candidate is frozen",
            "The indexed images and configuration form one deployable candidate",
            "M10 deployment, canary, site acceptance, or production promotion is authorized",
        ],
        "production_applied": False,
    }


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / OUTPUT_RELATIVE)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    output = args.output.resolve(strict=False)
    if output != (ROOT / OUTPUT_RELATIVE).resolve(strict=False) and output != Path("/tmp/m10-deployable-candidate-closure.json"):
        raise SystemExit("output must be the repository closure or the documented /tmp path")
    rendered = render(build(ROOT))
    if args.check:
        if not output.is_file() or output.read_text(encoding="utf-8") != rendered:
            print("FAIL: M10 deployable-candidate closure is stale")
            return 1
        print("PASS: M10 deployable-candidate closure is deterministic and current")
        return 0
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
