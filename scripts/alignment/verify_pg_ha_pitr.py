#!/usr/bin/env python3
"""Verify repository-side safe-hold guards for T-PG-006 PostgreSQL HA/PITR."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/postgres/ha-pitr-fencing.v1.json"


def _load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def _documents(path: Path) -> dict[tuple[str, str], dict[str, Any]]:
    documents = [item for item in yaml.safe_load_all(path.read_text(encoding="utf-8")) if item]
    return {
        (str(item.get("kind", "")), str((item.get("metadata") or {}).get("name", ""))): item
        for item in documents
    }


def _container(stateful_set: dict[str, Any]) -> dict[str, Any]:
    containers = (
        stateful_set.get("spec", {})
        .get("template", {})
        .get("spec", {})
        .get("containers", [])
    )
    if len(containers) != 1:
        raise ValueError(
            f"{stateful_set.get('metadata', {}).get('name')}: expected one PostgreSQL container"
        )
    return containers[0]


def _probe_source(container: dict[str, Any], probe_name: str) -> str:
    return json.dumps(container.get(probe_name) or {}, ensure_ascii=False, sort_keys=True)


def verify(root: Path = ROOT) -> dict[str, Any]:
    contract = _load_json(root / CONTRACT.relative_to(ROOT))
    errors: list[str] = []
    if contract.get("schema_version") != 1 or contract.get("remediation_id") != "T-PG-006":
        errors.append("PostgreSQL HA/PITR contract must be schema v1 for T-PG-006")

    repository_mode = contract.get("repository_mode") or {}
    if repository_mode.get("mode") != "safe-hold-readiness-only":
        errors.append("repository mode must remain safe-hold-readiness-only until one HA controller exists")
    if repository_mode.get("automated_promotion_enabled") is not False:
        errors.append("repository must not claim automated promotion while fencing is absent")
    if repository_mode.get("service_cutover_enabled") is not False:
        errors.append("repository must not claim automatic Service cutover while fencing is absent")

    controller = contract.get("target_ha_controller") or {}
    pitr = contract.get("pitr") or {}
    if controller.get("repository_state") != "NOT_IMPLEMENTED":
        errors.append("HA controller cannot be marked implemented without release-candidate evidence")
    if pitr.get("repository_state") != "NOT_IMPLEMENTED":
        errors.append("PITR cannot be marked implemented without archive and restore evidence")
    if len(controller.get("required_capabilities") or []) < 6:
        errors.append("HA controller contract is missing fencing or endpoint capabilities")
    if len(pitr.get("required_capabilities") or []) < 6:
        errors.append("PITR contract is missing backup, WAL or restore capabilities")

    scope = contract.get("scope") or {}
    manifest_path = root / str(scope.get("manifest", ""))
    if not manifest_path.is_file():
        errors.append("PostgreSQL infrastructure manifest is missing")
        documents: dict[tuple[str, str], dict[str, Any]] = {}
    else:
        documents = _documents(manifest_path)

    primary = documents.get(("StatefulSet", str(scope.get("declared_primary"))))
    replicas = documents.get(("StatefulSet", str(scope.get("declared_replicas"))))
    write_service = documents.get(("Service", str(scope.get("write_service"))))
    read_service = documents.get(("Service", str(scope.get("read_service"))))
    cronjob = documents.get(("CronJob", str(scope.get("readiness_cronjob"))))
    configmap = documents.get(("ConfigMap", str(scope.get("readiness_configmap"))))

    for label, value in (
        ("declared primary StatefulSet", primary),
        ("declared replica StatefulSet", replicas),
        ("write Service", write_service),
        ("read Service", read_service),
        ("readiness CronJob", cronjob),
        ("readiness ConfigMap", configmap),
    ):
        if value is None:
            errors.append(f"manifest is missing {label}")

    if primary:
        if primary.get("spec", {}).get("replicas") != 1:
            errors.append("safe-hold topology must declare exactly one static primary")
        probe = _probe_source(_container(primary), "readinessProbe")
        for token in ("SELECT NOT pg_is_in_recovery()", "traffic_platform"):
            if token not in probe:
                errors.append(f"primary readiness probe missing {token}")
    if replicas:
        if int(replicas.get("spec", {}).get("replicas", 0)) < 2:
            errors.append("safe-hold topology must keep at least two declared replicas")
        probe = _probe_source(_container(replicas), "readinessProbe")
        if "SELECT pg_is_in_recovery()" not in probe:
            errors.append("replica readiness probe must reject a promoted or writable pod")

    if write_service:
        selector = write_service.get("spec", {}).get("selector") or {}
        if selector != {"app": "postgres", "role": "primary"}:
            errors.append("write Service must select only the declared primary role")
    if read_service:
        selector = read_service.get("spec", {}).get("selector") or {}
        if selector != {"app": "postgres", "role": "replica"}:
            errors.append("read Service must select only declared replicas")

    if cronjob:
        spec = cronjob.get("spec") or {}
        annotations = cronjob.get("metadata", {}).get("annotations") or {}
        if spec.get("concurrencyPolicy") != "Forbid":
            errors.append("PostgreSQL readiness CronJob must forbid concurrent runs")
        job_spec = spec.get("jobTemplate", {}).get("spec", {})
        if job_spec.get("backoffLimit") != 0:
            errors.append("readiness job must not create an unbounded retry train")
        deadline = job_spec.get("activeDeadlineSeconds")
        if not isinstance(deadline, int) or deadline > 55:
            errors.append("readiness job must have a bounded deadline no greater than 55 seconds")
        if annotations.get("alignment.traffic-platform.io/automation-mode") != "readiness-only":
            errors.append("CronJob must declare readiness-only automation mode")

    script = str((configmap or {}).get("data", {}).get("failover.sh", ""))
    for token in (
        "automatic promotion is disabled",
        "SELECT NOT pg_is_in_recovery()",
        "SELECT pg_is_in_recovery()",
        "for ordinal in 0 1",
    ):
        if token not in script:
            errors.append(f"readiness script missing {token}")
    lower_script = script.lower()
    forbidden_hits = [
        token for token in contract.get("unsafe_legacy_tokens") or []
        if str(token).lower() in lower_script
    ]
    if forbidden_hits:
        errors.append(f"readiness script contains unsafe promotion tokens: {forbidden_hits}")

    runbook = root / "doc/07_alignment/runbooks/T-PG-006-ha-pitr-fencing.md"
    if not runbook.is_file():
        errors.append("T-PG-006 HA/PITR runbook is missing")
    else:
        runbook_source = runbook.read_text(encoding="utf-8")
        for token in (
            "production_applied=false",
            "pg_is_in_recovery()",
            "primary_epoch",
            "pg_verifybackup",
            "recovery_target_time",
            "旧主只能作为新副本重新加入",
        ):
            if token not in runbook_source:
                errors.append(f"T-PG-006 runbook missing {token}")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": "T-PG-006",
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL_SAFE_HOLD",
        "repository_mode": repository_mode.get("mode"),
        "automated_promotion_enabled": repository_mode.get("automated_promotion_enabled"),
        "pitr_repository_state": pitr.get("repository_state"),
        "declared_primary_replicas": primary.get("spec", {}).get("replicas") if primary else None,
        "declared_standby_replicas": replicas.get("spec", {}).get("replicas") if replicas else None,
        "unsafe_promotion_hits": forbidden_hits,
        "errors": errors,
        "remaining_gates": list((contract.get("remaining_gates") or {}).values()),
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
