#!/usr/bin/env python3
"""Verify the default-off T-OS-003 bounded search_after and PIT candidate."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/search-pagination.v1.json")
FEATURE = Path("contracts/alignment/features/F-SEARCH-001.json")
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
REPOSITORY = Path("go/control-plane/internal/alert/repository/opensearch.go")
CURSOR = Path("go/control-plane/internal/alert/repository/opensearch_cursor.go")
REPOSITORY_TEST = Path("go/control-plane/internal/alert/repository/opensearch_cursor_test.go")
HANDLER = Path("go/control-plane/internal/alert/api/handler.go")
HANDLER_TEST = Path("go/control-plane/internal/alert/api/handler_search_cursor_test.go")
SERVICE = Path("go/control-plane/internal/alert/service/alert_service.go")
CONFIG = Path("go/control-plane/internal/alert/config/config.go")
MAIN = Path("go/control-plane/cmd/alert-service/main.go")
DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
RUNBOOK = Path("doc/07_alignment/runbooks/T-OS-003-search-pagination-pit.md")
WEB_RUNTIME = Path("web/ui/src/config/runtime.ts")
WEB_CLIENT = Path("web/ui/src/services/alertSearchCursorApi.ts")
WEB_PAGE = Path("web/ui/src/pages/AlertTriagePage.tsx")
WEB_DEPLOYMENT = Path("deployments/kubernetes/applications/web-ui.yaml")
K8S_EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n015/k8s-opensearch-cursor-latest.json")


def load_json(root: Path, relative: Path) -> dict[str, Any]:
    return json.loads((root / relative).read_text(encoding="utf-8"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = (
        CONTRACT, FEATURE, OPENAPI, REPOSITORY, CURSOR, REPOSITORY_TEST,
        HANDLER, HANDLER_TEST, SERVICE, CONFIG, MAIN, DEPLOYMENT, RUNBOOK,
        WEB_RUNTIME, WEB_CLIENT, WEB_PAGE, WEB_DEPLOYMENT, K8S_EVIDENCE,
    )
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("remediation_id") != "T-OS-003" or contract.get("feature_id") != "F-SEARCH-001":
        errors.append("contract must bind T-OS-003 to F-SEARCH-001")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial candidate must not claim T-OS-003 closure")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False:
        errors.append("candidate must remain PARTIAL with production_applied=false")
    if not contract.get("closure_blockers"):
        errors.append("live, performance, browser, rollout and observation blockers must remain explicit")

    modes = contract.get("modes", {})
    if modes.get("legacy_shallow", {}).get("maximum_result_window") != 1000:
        errors.append("legacy from/size must be limited to a 1000-result shallow window")
    if modes.get("live", {}).get("pagination") != "search_after":
        errors.append("live traversal must use search_after")
    pit = modes.get("pit", {})
    if pit.get("pagination") != "point_in_time_plus_search_after" or pit.get("explicit_close") is not True:
        errors.append("consistent traversal must combine PIT, search_after and explicit close")

    cursor = contract.get("cursor", {})
    if cursor.get("domain_separation") != "traffic.alert.search.cursor.v1":
        errors.append("cursor HMAC domain separation drifted")
    for guard in ("unknown_claims_rejected", "cross_tenant_replay_rejected", "query_drift_rejected"):
        if cursor.get(guard) is not True:
            errors.append(f"cursor guard must remain true: {guard}")
    if "resolved_target_sha256" not in cursor.get("bound_claims", []):
        errors.append("cursor must bind the resolved physical target digest")
    if cursor.get("live_alias_target_drift_rejected_before_search") is not True or cursor.get("pit_alias_switch_preserves_frozen_shards") is not True:
        errors.append("alias-switch live/PIT behavior is not explicit")

    guards = contract.get("query_guards", {})
    expected = {
        "maximum_page_size": 200,
        "maximum_query_characters": 256,
        "maximum_values_per_filter": 20,
        "maximum_total_filter_values": 64,
        "query_timeout": "2s",
        "allow_partial_search_results": False,
        "track_total_hits_up_to": 10000,
        "stable_tie_breaker": "alert_id",
        "tenant_and_permission_filter_context": True,
        "wildcard_query_type_exposed": False,
        "regexp_query_type_exposed": False,
        "timed_out_response_is_failure": True,
        "failed_shard_response_is_failure": True,
    }
    for key, value in expected.items():
        if guards.get(key) != value:
            errors.append(f"query guard drifted: {key}={guards.get(key)!r}")
    if len(guards.get("source_field_allowlist", [])) < 20:
        errors.append("OpenSearch _source allowlist is incomplete")

    runtime = contract.get("runtime_guards", {})
    if runtime.get("feature_flag") != "OPENSEARCH_SEARCH_CURSOR_V1_ENABLED":
        errors.append("runtime feature flag name drifted")
    if runtime.get("feature_flag_default_enabled") is not False:
        errors.append("cursor traffic must remain disabled by default")
    if runtime.get("legacy_route_preserved") is not True or runtime.get("legacy_fields_preserved") is not True:
        errors.append("legacy route and from/size fields must remain additive compatibility")
    if runtime.get("production_mutations_in_repository_candidate") != []:
        errors.append("repository candidate must not claim production mutation")

    feature = load_json(root, FEATURE)
    if feature.get("feature_id") != "F-SEARCH-001" or feature.get("api", {}).get("operation_id") != "searchAlertsCursorV1":
        errors.append("F-SEARCH-001 Feature Contract operation is missing")
    if feature.get("rollout", {}).get("feature_flag") != "OPENSEARCH_SEARCH_CURSOR_V1_ENABLED":
        errors.append("Feature Contract rollout flag differs from technology contract")

    openapi = load_json(root, OPENAPI)
    paths = openapi.get("paths", {})
    search = paths.get("/v1/alerts/search", {}).get("post", {})
    close = paths.get("/v1/alerts/search/cursor", {}).get("delete", {})
    if search.get("operationId") != "searchAlertsCursorV1":
        errors.append("OpenAPI searchAlertsCursorV1 operation is missing")
    if close.get("operationId") != "closeAlertSearchCursorV1":
        errors.append("OpenAPI closeAlertSearchCursorV1 operation is missing")
    request_schema = openapi.get("components", {}).get("schemas", {}).get("AlertSearchRequest", {})
    properties = request_schema.get("properties", {})
    for field in ("from", "size", "cursor", "cursor_mode", "sort_field", "sort_order", "asset_ip", "rule_version", "model_version", "attack_phase", "min_score"):
        if field not in properties:
            errors.append(f"OpenAPI search request missing compatibility/additive field: {field}")

    repository = (root / REPOSITORY).read_text(encoding="utf-8")
    for token in (
        'SearchCursorModeLive = "live"', 'SearchCursorModePIT  = "pit"',
        '"search_after"', '"pit"', '"_source"', '"track_total_hits"',
        "WithAllowPartialSearchResults(false)", "WithTimeout(timeout)",
        '"alert_id": map[string]interface{}{"order": sortOrder}',
        'filter := []map[string]interface{}{{"term": map[string]interface{}{"tenant_id": query.TenantID}}}',
        "response.TimedOut || response.Shards.Failed > 0",
        "CloseSearchCursor", "bestEffortClosePIT", "context.WithoutCancel",
        "resolveSearchTargets", "Indices.ResolveIndex", "claims.TargetSHA256",
    ):
        if token not in repository:
            errors.append(f"OpenSearch repository guard missing: {token}")
    if 'boolQuery["must"]' in repository and '"tenant_id": query.TenantID' not in repository:
        errors.append("tenant filter disappeared from the query builder")

    cursor_code = (root / CURSOR).read_text(encoding="utf-8")
    for token in (
        "traffic.alert.search.cursor.v1", "hmac.Equal", "DisallowUnknownFields",
        "QuerySHA256", "TargetSHA256", "SnapshotUnixMilli", "ExpiresAtUnixSecond",
        "claims.TenantID", "validSearchSortValues", "searchQuerySHA256",
    ):
        if token not in cursor_code:
            errors.append(f"signed cursor guard missing: {token}")

    handler = (root / HANDLER).read_text(encoding="utf-8")
    for token in (
        'HandleFunc("/alerts/search", h.SearchAlerts).Methods("POST")',
        'HandleFunc("/alerts/search/cursor", h.CloseSearchCursor).Methods("DELETE")',
        "requireAlertReadPermission", "validateSearchAlertsRequest", "utf8.RuneCountInString",
        "JSONContractSuccess", "cursor_mode must be live or pit",
    ):
        if token not in handler:
            errors.append(f"HTTP contract or validation guard missing: {token}")

    config = (root / CONFIG).read_text(encoding="utf-8")
    deployment = (root / DEPLOYMENT).read_text(encoding="utf-8")
    main = (root / MAIN).read_text(encoding="utf-8")
    for token in (
        'env:"OPENSEARCH_SEARCH_CURSOR_V1_ENABLED" envDefault:"false"',
        'env:"OPENSEARCH_SEARCH_SHALLOW_RESULT_LIMIT" envDefault:"1000"',
        'env:"OPENSEARCH_SEARCH_MAX_PAGE_SIZE" envDefault:"200"',
        'env:"OPENSEARCH_SEARCH_QUERY_TIMEOUT" envDefault:"2s"',
        'env:"OPENSEARCH_SEARCH_CURSOR_TTL" envDefault:"2m"',
        'env:"OPENSEARCH_SEARCH_TRACK_TOTAL_HITS_UP_TO" envDefault:"10000"',
    ):
        if token not in config:
            errors.append(f"Go config budget missing: {token}")
    if 'OPENSEARCH_SEARCH_CURSOR_V1_ENABLED, value: "false"' not in deployment:
        errors.append("production candidate must explicitly keep cursor traffic disabled")
    if "CursorSigningKey:   cfg.Auth.JWTSecretKey" not in main:
        errors.append("cursor signing must reuse the Secret-backed JWT root with domain separation")

    web_runtime = (root / WEB_RUNTIME).read_text(encoding="utf-8")
    web_client = (root / WEB_CLIENT).read_text(encoding="utf-8")
    web_page = (root / WEB_PAGE).read_text(encoding="utf-8")
    web_deployment = (root / WEB_DEPLOYMENT).read_text(encoding="utf-8")
    if "enableAlertSearchCursorV1" not in web_runtime or "runtime.ALERT_SEARCH_CURSOR_V1_ENABLED ?? import.meta.env.VITE_ALERT_SEARCH_CURSOR_V1_ENABLED,\n    false," not in web_runtime:
        errors.append("Web cursor runtime must remain default-off")
    if 'ALERT_SEARCH_CURSOR_V1_ENABLED, value: "false"' not in web_deployment:
        errors.append("Web deployment must explicitly keep cursor traversal disabled")
    for token in ("fetchAlertSearchCursorSnapshot", "closeAlertSearchCursor", "opensearch.alerts.target_sha256", "throw new Error"):
        if token not in web_client:
            errors.append(f"typed Web cursor client guard missing: {token}")
    for token in ("cursorSearchEnabled", "PIT 一致性分页", "restartSearchTraversal", "simple: cursorSearchEnabled"):
        if token not in web_page:
            errors.append(f"alert page cursor lifecycle guard missing: {token}")

    tests = (root / REPOSITORY_TEST).read_text(encoding="utf-8") + (root / HANDLER_TEST).read_text(encoding="utf-8")
    for token in (
        "TenantTamperAndQueryDrift", "PITCursorCreatesRotatesAndCloses", "FailsClosedOnTimeoutOrShardFailure",
        "LegacyCompatibilityAndEnabledShallowBound", "RejectsUnknownClaims", "RejectsUnboundedOrAmbiguousInput",
        "LiveCursorFailsClosedAfterAliasSwitch",
    ):
        if token not in tests:
            errors.append(f"negative or compatibility test missing: {token}")

    k8s = load_json(root, K8S_EVIDENCE)
    if k8s.get("status") != "PASS" or k8s.get("production_applied") is not False:
        errors.append("run-scoped Kubernetes OpenSearch evidence is not a non-production PASS")
    for field in (
        "live_cursor_alias_switch_fails_closed", "pit_alias_switch_keeps_frozen_snapshot",
        "opensearch_unavailable_fails_closed", "run_scoped_indices_and_alias_removed",
    ):
        if k8s.get(field) is not True:
            errors.append(f"Kubernetes OpenSearch evidence missing: {field}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "feature_id": contract.get("feature_id"),
        "remediation_id": contract.get("remediation_id"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "feature_flag_default_enabled": runtime.get("feature_flag_default_enabled"),
        "maximum_page_size": guards.get("maximum_page_size"),
        "shallow_result_window": modes.get("legacy_shallow", {}).get("maximum_result_window"),
        "source_allowlist_fields": len(guards.get("source_field_allowlist", [])),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
