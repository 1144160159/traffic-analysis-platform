#!/usr/bin/env python3
"""Verify the default-off T1-M09-N016 alert report job candidate."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/reporting/alert-report-job.v1.json")
FEATURE = Path("contracts/alignment/features/F-ALERT-001.json")
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
HANDLER = Path("go/control-plane/internal/alert/api/handler_alert_reports.go")
HANDLER_ROOT = Path("go/control-plane/internal/alert/api/handler.go")
HANDLER_TEST = Path("go/control-plane/internal/alert/api/handler_alert_reports_test.go")
K8S_TEST = Path("go/control-plane/internal/alert/api/alert_report_k8s_integration_test.go")
MAIN = Path("go/control-plane/cmd/alert-service/main.go")
DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
WEB_CLIENT = Path("web/ui/src/services/alertDetailActionApi.ts")
WEB_PAGE = Path("web/ui/src/pages/AlertDetailPage.tsx")
RUNBOOK = Path("doc/07_alignment/runbooks/T1-M09-N016-alert-report-jobs.md")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n016/k8s-alert-report-latest.json")


def load_json(root: Path, path: Path) -> dict[str, Any]:
    return json.loads((root / path).read_text(encoding="utf-8"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = (
        CONTRACT, FEATURE, OPENAPI, HANDLER, HANDLER_ROOT, HANDLER_TEST, K8S_TEST,
        MAIN, DEPLOYMENT, WEB_CLIENT, WEB_PAGE, RUNBOOK, EVIDENCE,
    )
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("task_id") != "T1-M09-N016" or contract.get("feature_id") != "F-ALERT-001":
        errors.append("report contract must bind T1-M09-N016 to F-ALERT-001")
    runtime = contract.get("runtime", {})
    if runtime.get("feature_flag") != "ALERT_REPORT_JOBS_V1_ENABLED" or runtime.get("default_enabled") is not False:
        errors.append("report runtime must remain explicitly default-off")
    if (runtime.get("artifact_ttl_minimum"), runtime.get("artifact_ttl_default"), runtime.get("artifact_ttl_maximum")) != ("5m", "24h", "720h"):
        errors.append("report artifact TTL bounds drifted")
    object_contract = contract.get("object", {})
    if object_contract.get("same_key_retry") is not True or object_contract.get("key_template") != "{safe_tenant_id}/alerts/{safe_alert_id}/{job_id}.{format}":
        errors.append("same-key retry object identity drifted")
    manifest = contract.get("manifest", {})
    if manifest.get("manifest_version") != 1 or manifest.get("object_format_version") != 1:
        errors.append("manifest and object format versions must remain v1")
    expired = manifest.get("expired_behavior", {})
    if expired.get("status_endpoint") != "200_queryable_without_download_url" or expired.get("download_endpoint") != "410_REPORT_EXPIRED":
        errors.append("expired report must stay queryable and fail closed on download")
    closure = contract.get("closure", {})
    if closure.get("status") != "PARTIAL" or closure.get("production_applied") is not False or not closure.get("open_items"):
        errors.append("candidate must remain PARTIAL and non-production with explicit blockers")

    feature = load_json(root, FEATURE)
    rollout = feature.get("rollout", {})
    if rollout.get("feature_flag") != "ALERT_REPORT_JOBS_V1_ENABLED" or rollout.get("default_runtime_state") != "off":
        errors.append("Feature Contract report rollout must remain default-off")
    if "artifact_expired" not in feature.get("ui", {}).get("states", []):
        errors.append("Feature Contract UI must expose artifact_expired")

    openapi = load_json(root, OPENAPI)
    paths = openapi.get("paths", {})
    if paths.get("/v1/alerts/{id}/reports/export", {}).get("post", {}).get("operationId") != "createAlertReport":
        errors.append("OpenAPI createAlertReport operation is missing")
    download_responses = paths.get("/v1/alerts/{id}/reports/{job_id}/download", {}).get("get", {}).get("responses", {})
    if "410" not in download_responses:
        errors.append("OpenAPI expired report download 410 is missing")

    handler = (root / HANDLER).read_text(encoding="utf-8")
    for token in (
        "alertReportManifestVersion", "alertReportObjectFormatVersion", "alertReportDefaultArtifactTTL",
        'http.StatusGone, "REPORT_EXPIRED"', "alertReportArtifactExpiry", 'manifestStatus = "expired"',
        "FOR UPDATE SKIP LOCKED", "job.JobID+\".\"+extension", "sha256.Sum256(canonicalSnapshot)",
        '"postgresql.assets.updated_at"', '"postgresql.alert_response_actions.updated_at"',
        '"postgresql.audit_logs.created_at"',
    ):
        if token not in handler:
            errors.append(f"report worker/manifest guard missing: {token}")
    main = (root / MAIN).read_text(encoding="utf-8")
    deployment = (root / DEPLOYMENT).read_text(encoding="utf-8")
    if 'getBoolEnv("ALERT_REPORT_JOBS_V1_ENABLED", false)' not in main:
        errors.append("application report feature flag is not default-off")
    if 'ALERT_REPORT_ARTIFACT_TTL", "24h"' not in main or "5*time.Minute" not in main or "30*24*time.Hour" not in main:
        errors.append("application report TTL validation is missing")
    if 'ALERT_REPORT_JOBS_V1_ENABLED, value: "false"' not in deployment or 'ALERT_REPORT_ARTIFACT_TTL, value: "24h"' not in deployment:
        errors.append("Kubernetes candidate must explicitly keep reports off with a 24h TTL")

    tests = (root / HANDLER_TEST).read_text(encoding="utf-8") + (root / K8S_TEST).read_text(encoding="utf-8")
    for token in (
        "ManifestExposesExpiryAndFailsClosedAfterDeadline", "RejectsExpiredArtifactBeforeObjectAccess",
        "WorkerUploadsArtifactThenCommitsManifestAndCompletionEvent", "CancellationCleanupRemovesExactObjectBeforeTerminalState",
        "same-key retry created", "cross-tenant status", "CleanupOracle",
    ):
        if token not in tests:
            errors.append(f"report negative/integration test missing: {token}")

    web_client = (root / WEB_CLIENT).read_text(encoding="utf-8")
    web_page = (root / WEB_PAGE).read_text(encoding="utf-8")
    for token in ("artifactExpired", "manifestVersion", "objectFormatVersion", "snapshotSHA256", "artifactSHA256"):
        if token not in web_client:
            errors.append(f"typed report manifest field missing: {token}")
    for token in ("报告对象下载权限已过期", "任务、冻结快照、水位和 manifest 仍可查询", "activeBusinessActionResult.downloadUrl"):
        if token not in web_page:
            errors.append(f"Web report lifecycle guard missing: {token}")

    evidence = load_json(root, EVIDENCE)
    if evidence.get("task_id") != "T1-M09-N016" or evidence.get("status") != "PASS" or evidence.get("production_applied") is not False:
        errors.append("Kubernetes report evidence must be a task-bound non-production PASS")
    for field in (
        "frozen_snapshot_and_source_watermarks", "versioned_object_manifest", "same_key_retry_single_object",
        "cooperative_cancel_removes_exact_object", "expired_job_queryable_without_download_authority",
        "expired_download_fails_closed", "tenant_isolation", "run_scoped_postgres_rows_and_bucket_removed",
    ):
        if evidence.get(field) is not True:
            errors.append(f"Kubernetes report evidence missing: {field}")
    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = root / relative
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            errors.append(f"Kubernetes report source hash drifted: {relative}")
    validation = contract.get("kubernetes_validation", {})
    if validation.get("status") != "PASS" or validation.get("run_id") != evidence.get("run_id"):
        errors.append("report contract K8s validation does not bind the current run")

    return {
        "status": "PASS" if not errors else "FAIL",
        "task_id": contract.get("task_id"),
        "feature_id": contract.get("feature_id"),
        "coverage_status": closure.get("status"),
        "production_applied": closure.get("production_applied"),
        "feature_flag_default_enabled": runtime.get("default_enabled"),
        "manifest_version": manifest.get("manifest_version"),
        "object_format_version": manifest.get("object_format_version"),
        "kubernetes_run_id": validation.get("run_id"),
        "closure_blockers": closure.get("open_items", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
