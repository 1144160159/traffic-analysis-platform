#!/usr/bin/env python3
"""Build the deterministic F-ADAPTER-002 data-guessing risk inventory."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/alignment/adapter-risk-registry.v1.json"
SOURCE_ROOTS = (
    "web/ui/src/pages",
    "web/ui/src/services",
)
EXCLUDED_SOURCE_NAMES = {
    "mockData.ts",
}


def production_sources(root: Path) -> tuple[str, ...]:
    sources: list[str] = []
    for source_root in SOURCE_ROOTS:
        directory = root / source_root
        for path in directory.rglob("*"):
            if not path.is_file() or path.suffix not in {".ts", ".tsx"}:
                continue
            if path.name in EXCLUDED_SOURCE_NAMES or ".test." in path.name or ".spec." in path.name:
                continue
            sources.append(path.relative_to(root).as_posix())
    return tuple(sorted(sources))


RULES: dict[str, dict[str, Any]] = {
    "hardcoded_private_ip_literal": {
        "classification": "prohibited_demo_fill",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"(?:10\.(?:\d{1,3}\.){2}\d{1,3}|192\.168\.(?:\d{1,3}\.)\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.(?:\d{1,3}\.)\d{1,3})"),
    },
    "fabricated_comparison_literal": {
        "classification": "prohibited_demo_fill",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"较昨日\s*[+-]\s*\d"),
    },
    "truthy_numeric_fallback": {
        "classification": "review_required_null_semantics",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"(?:numberAt|numberFrom|ratioAt|optionalNumberAt|optionalRatioAt)\([^\n;]+\)\s*\|\|"),
    },
    "runtime_mock_dependency": {
        "classification": "prohibited_runtime_fixture_dependency",
        "owner": "frontend-build-owner",
        "pattern": re.compile(r"^\s*import(?!\s+type\b)[^\n]+(?:mockData|/mocks?/)", re.IGNORECASE),
    },
    "runtime_visual_fixture_bypass": {
        "classification": "prohibited_runtime_fixture_dependency",
        "owner": "frontend-build-owner",
        "pattern": re.compile(r"buildVisualBreakdownSnapshot|isVisualBreakdownMode\(\).*build", re.IGNORECASE),
    },
    "runtime_static_business_fallback": {
        "classification": "prohibited_runtime_fixture_dependency",
        "owner": "frontend-build-owner",
        "pattern": re.compile(
            r"(?:\?\?\s*\w*Fallback\w*|"
            r"\?\.length\s*\?[^:\n]+:\s*\w*Fallback\w*|"
            r"\bbuildFallback(?:Heatmap|FlinkMetrics)\s*\(|"
            r"\|\|\s*92\b|"
            r"const\s+rangeLabel\s*=\s*['\"]20\d{2}-)"
        ),
    },
    "fabricated_generic_snapshot": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"rows\.length\s*\+\s*index|pageId\.toUpperCase\(\).*padStart"),
    },
    "derived_summary_fallback": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"(?:reportable_count|pending_evidence_count|open_risk_count).*\?\?.*(?:metric|length|filter|completeness|riskOpen)"),
    },
    "derived_business_collection": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"events\.length\s*\?\s*events\s*:\s*users\.length\s*\?\s*users\s*:\s*protocols"),
    },
    "default_demo_resource_id": {
        "classification": "prohibited_demo_fill",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"(?:alertId|campaignId)\s*\|\|\s*['\"](?:AL-|APT-)"),
    },
    "fixed_detail_business_fallback": {
        "classification": "prohibited_demo_fill",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(
            r"(?:\|\||return)\s*['\"](?:"
            r"20\d{2}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}|"
            r"sec_[^'\"]+|C2_Tunnel[^'\"]*|教学区[^'\"]*|办公室[^'\"]*|"
            r"Windows\s+10[^'\"]*|办公区|Example\s+GmbH|德国[^'\"]*|"
            r"AS\d+|TLS/\d+|回流至\s+MLOps|IDS\s*/\s*探针[^'\"]*|"
            r"\d+(?:\.\d+)?\s*(?:MB|KB)|\d+天\d+小时"
            r")[" + "'\"]"
        ),
    },
    "fixed_detail_metric_literal": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"metric\([^\n]*(?:'\d+\s*(?:台|项)'|:\s*'\d+%')"),
    },
    "fixed_detail_timeline_literal": {
        "classification": "prohibited_demo_fill",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"timelineItem\(['\"]\d{2}:\d{2}:\d{2}['\"]"),
    },
    "fixed_detail_collection_fallback": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(
            r"\?[^\n]*:\s*\[(?:[^\n]*(?:C2通信|横向移动|可疑外联|隔离主机|阻断\s*IP|"
            r"DB-SRV-01|svc_backup|PCAP\s+1|Session\s+2|ja3_score=))"
        ),
    },
    "fixed_numeric_business_fallback": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(
            r"^(?!.*\b(?:width|height)\s*:).*"
            r"(?:optional(?:Number|Ratio)(?:At|From)|averageNumbers|modelMetricValue|modelDrift|\.length|\.size)"
            r"[^;\n]*(?:\?\?|\|\|)\s*(?:[1-9]\d*(?:\.\d+)?|0\.\d+)"
        ),
    },
    "runtime_business_fixture_object": {
        "classification": "prohibited_runtime_fixture_dependency",
        "owner": "frontend-build-owner",
        "pattern": re.compile(
            r"(?:\b(?:fallbackScreenVisuals|visualTargetEgressRows)\b|"
            r"^\s*const\s+(?:pipeline|topologyNodes|topologyEdges|probeMapNodes|probeMapLinks|"
            r"evidenceRings|attackStages|abnormalLinks|responseStats|runtimeStats|riskMapPoints|"
            r"egressMapPoints|egressMapFlows|campaignDensityPoints)\b[^=]*=\s*\[\s*$)"
        ),
    },
    "generated_business_trend": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(r"\b(?:rowSeed|buildModelMetricTrend)\b"),
    },
    "fixed_page_metric_fallback": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(
            r"(?:numericRowValue\([^\n]+,\s*-?\d+(?:\.\d+)?\)|"
            r"(?:const|function)\s+fallbackMetric\b)"
        ),
    },
    "fixed_screen_visual_fixture": {
        "classification": "prohibited_business_inference",
        "owner": "frontend-contract-owner",
        "pattern": re.compile(
            r"(?:\b(?:probeMapNodes|probeMapLinks)\s*:[^\n]*:\s*\[|"
            r"\b(?:topologyNodes|topologyEdges|abnormalLinks|evidenceRings)\s*:\s*\[\s*$|"
            r"初始访问簇|资源利用簇|执行脚本簇)"
        ),
    },
}


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    return _sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


def scan_source(relative: str, source: str) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    for line_number, raw_line in enumerate(source.splitlines(), 1):
        line = raw_line.strip()
        if not line:
            continue
        for rule_id, rule in RULES.items():
            if not rule["pattern"].search(raw_line):
                continue
            excerpt_hash = _sha256(line)
            fingerprint = _sha256(f"{relative}\0{line_number}\0{rule_id}\0{line}")[:20]
            findings.append(
                {
                    "finding_id": f"ADAPTER-{fingerprint.upper()}",
                    "rule_id": rule_id,
                    "classification": rule["classification"],
                    "status": "OPEN",
                    "owner": rule["owner"],
                    "path": relative,
                    "line": line_number,
                    "excerpt_sha256": excerpt_hash,
                    "excerpt": line[:240],
                    "required_disposition": "replace with a typed contract field or document an explicit compatibility mapping before closure",
                }
            )
    return findings


def build_registry(root: Path = ROOT) -> dict[str, Any]:
    root = root.resolve()
    findings: list[dict[str, Any]] = []
    authorities = []
    sources = production_sources(root)
    for relative in sources:
        path = root / relative
        source = path.read_text(encoding="utf-8")
        authorities.append({"path": relative, "sha256": _sha256(path.read_bytes())})
        findings.extend(scan_source(relative, source))
    findings.sort(key=lambda item: (item["path"], item["line"], item["rule_id"]))
    counts_by_rule = {
        rule_id: sum(item["rule_id"] == rule_id for item in findings)
        for rule_id in sorted(RULES)
    }
    registry: dict[str, Any] = {
        "schema_version": 1,
        "feature_id": "F-ADAPTER-002",
        "status": "verifying",
        "scope": "all production TypeScript sources under web/ui/src/pages and web/ui/src/services; tests and explicit mock fixture modules are excluded",
        "authorities": authorities,
        "policy": {
            "zero_empty_missing_unavailable_are_distinct": True,
            "runtime_fixture_fallback_allowed": False,
            "hardcoded_business_fact_allowed": False,
            "frontend_derived_topology_or_kpi_allowed": False,
            "new_findings_require_catalog_regeneration_and_owner_review": True,
            "external_routes_fields_controls_operations_removed": False,
        },
        "rules": [
            {
                "rule_id": rule_id,
                "classification": rule["classification"],
                "owner": rule["owner"],
            }
            for rule_id, rule in sorted(RULES.items())
        ],
        "coverage": {
            "source_files": len(sources),
            "open_findings": len(findings),
            "counts_by_rule": counts_by_rule,
            "resolved_in_this_slice": [
                "pageSnapshotAdapters graph hard-coded center IP fallback",
                "topic tunnel fabricated comparison literals",
                "topic tunnel truthy summary fallback and display-list business-row inference",
                "visual-breakdown query parameter no longer bypasses real page APIs with runtime fixture snapshots",
                "untyped page responses render partial and unavailable instead of guessed KPI values rows IDs or statuses",
                "graph landing queries no longer inject a fixed frontend center address",
                "topic delivery summary no longer derives report and evidence counts from completeness or display rows",
                "selected live adapters render missing addresses unavailable and contain no fixed private business address literals",
                "selected adapters preserve authoritative zero values across compatibility aliases",
                "data-quality topic rows require authoritative topic_health payloads instead of frontend-scaled aggregates",
                "production bundle statically excludes MSW worker modules and runtime fixture source graphs",
                "alert campaign and topic pilot fixture/live schema diff and sixteen-field displayed-value reconciliation are read-only and candidate-bound",
            ],
            "remaining": [
                "capture additional live zero empty partial and unavailable pilot windows beyond the current sampled reconciliation",
                "capture zero, empty, partial and unavailable live samples in Windows Chrome",
            ],
        },
        "findings": findings,
    }
    registry["catalog_sha256"] = _canonical_sha256(registry)
    return registry


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args()
    registry = build_registry(args.root)
    rendered = json.dumps(registry, ensure_ascii=False, indent=2) + "\n"
    output = args.root.resolve() / OUTPUT.relative_to(ROOT)
    if args.check:
        current = output.read_text(encoding="utf-8") if output.is_file() else ""
        status = "PASS" if current == rendered else "FAIL"
        print(json.dumps({"status": status, "catalog_sha256": registry["catalog_sha256"], "coverage": registry["coverage"]}, ensure_ascii=False, indent=2))
        return 0 if status == "PASS" else 1
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(rendered, encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": output.relative_to(args.root.resolve()).as_posix(), "catalog_sha256": registry["catalog_sha256"], "coverage": registry["coverage"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
