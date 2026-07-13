#!/usr/bin/env python3
"""Generate the first Codex Loop task pool and architecture map."""

from __future__ import annotations

import argparse
from typing import Any

from lib import SCRIPT_ROOT, repo_path, write_json, write_yaml


def task(
    task_id: str,
    title: str,
    priority: str,
    primary: str,
    dependent: list[str],
    acceptance_type: str,
    subsystems: list[str],
    allowed_paths: list[str],
    local_checks: list[str],
    close_when: list[str],
    sources: list[str],
    risk_level: str = "medium",
    reasons: list[str] | None = None,
    mode: str = "plan",
    data_mode: str = "live_existing",
    contracts: dict[str, bool] | None = None,
) -> dict[str, Any]:
    return {
        "id": task_id,
        "title": title,
        "priority": priority,
        "status": "DISCOVERED",
        "lane": {"primary": primary, "dependent": dependent},
        "source": sources,
        "acceptance_type": acceptance_type,
        "subsystems": subsystems,
        "risk": {"level": risk_level, "reasons": reasons or []},
        "execution": {"mode": mode, "allow_live_write": False},
        "workspace": {"allowed_paths": allowed_paths},
        "contracts": contracts or {
            "proto": False,
            "kafka_topics": False,
            "database_schema": False,
            "apisix_routes": False,
        },
        "data_plan": {"mode": data_mode, "tenant": "default", "cleanup": "none"},
        "verification": {
            "local": local_checks,
            "live_readonly": ["curl --noproxy '*' http://10.0.5.8:30180/login"],
        },
        "review": {
            "required": True,
            "perspectives": [
                "code_correctness",
                "product_logic",
                "technical_design",
                "acceptance_evidence",
            ],
            "design_update_allowed": [
                "doc/01_design/",
                "doc/02_acceptance/",
                "doc/05_status/",
            ],
        },
        "evidence": {
            "run_dir": "doc/02_acceptance/runs/${run_id}",
            "required": [
                "run-summary.json",
                "plan.md",
                "local-report.md",
                "review-report.md",
                "evidence-report.md",
            ],
        },
        "close_when": close_when,
    }


TASKS: list[dict[str, Any]] = [
    task(
        "CLE-P0-BASELINE-001",
        "生成 baseline/release manifest 草案",
        "P0",
        "Mission / Acceptance",
        ["Deploy / SRE / Security"],
        "regression",
        ["deployments", "doc/02_acceptance"],
        ["doc/02_acceptance/", "scripts/codex_loop/"],
        ["python scripts/codex_loop/collect_evidence.py --run-id ${run_id}"],
        [
            "commit, manifest, DDL, Topic, model and rule versions are listed",
            "release evidence is marked regression, not acceptance",
            "git status snapshot is captured",
        ],
        [
            "doc/01_design/自动开发Loop引擎设计.md#14",
            "doc/02_acceptance/README.md#3",
        ],
        reasons=["release evidence affects acceptance language"],
        mode="plan",
    ),
    task(
        "CLE-P0-REVIEWER-001",
        "开启第三视角 Reviewer Gate",
        "P0",
        "Mission / Acceptance",
        ["Product Design"],
        "regression",
        ["doc/02_acceptance", "scripts/codex_loop"],
        ["doc/02_acceptance/", "scripts/codex_loop/"],
        ["python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}"],
        [
            "review-report.md exists",
            "design-delta.md exists",
            "review decision cannot close P0/P1 blockers silently",
        ],
        ["doc/01_design/自动开发Loop引擎设计.md#9"],
        reasons=["review gate controls task closure"],
        mode="review",
    ),
    task(
        "CLE-P0-UIBACKUP-001",
        "备份现有 web/ui 并生成旧前端清单",
        "P0",
        "UI Rebuild",
        ["Product Design"],
        "regression",
        ["web/ui", "doc/02_acceptance"],
        ["web/ui/", "doc/02_acceptance/", "scripts/codex_loop/"],
        ["cd web/ui && npm run build"],
        [
            "web/ui git status is captured before changes",
            "package version and build command are recorded",
            "rollback path is documented",
        ],
        ["doc/01_design/自动开发Loop引擎设计.md#3.3"],
        reasons=["front-end rebuild must preserve rollback path"],
        mode="backup",
    ),
    task(
        "CLE-P0-ROUTE-001",
        "routeManifest 统一菜单、路由、权限、验收点",
        "P0",
        "UI Rebuild",
        ["Product Design", "Deploy / SRE / Security"],
        "regression",
        ["web/ui", "deployments/kubernetes/configmaps"],
        ["web/ui/src/", "web/ui/e2e/", "doc/02_acceptance/", "doc/01_design/"],
        ["tests/run_tests.sh web"],
        [
            "menu, route and breadcrumb derive from one manifest",
            "all six primary groups and twenty-four secondary routes are represented",
            "unauthorized routes have explicit behavior",
            "route matrix evidence exists",
        ],
        [
            "doc/05_status/代码实证状态核对-2026-06-19.md#6",
            "doc/03_review/专家深评整改清单.md#SUB-UI-02",
        ],
        reasons=["touches auth boundary", "touches route visibility"],
        mode="local",
        contracts={"proto": False, "kafka_topics": False, "database_schema": False, "apisix_routes": True},
    ),
    task(
        "CLE-P0-AUTH-001",
        "启动 /auth/me 鉴权和 WebSocket 延迟连接",
        "P0",
        "UI Rebuild",
        ["Go Control-plane"],
        "regression",
        ["web/ui", "go/control-plane/internal/auth"],
        ["web/ui/src/", "web/ui/e2e/", "go/control-plane/internal/auth/", "doc/02_acceptance/"],
        ["tests/run_tests.sh web", "tests/run_tests.sh go"],
        [
            "fake or expired token cannot enter protected shell",
            "WebSocket connects only after authorized route is active",
            "auth negative cases are recorded as expected negatives",
        ],
        [
            "doc/03_review/专家深评整改清单.md#SUB-FS-02",
            "doc/03_review/专家深评整改清单.md#SUB-UI-01",
        ],
        risk_level="high",
        reasons=["touches authentication and session timing"],
        mode="local",
    ),
    task(
        "CLE-P0-SCREEN-001",
        "/screen 只读 token 或脱敏公开边界",
        "P0",
        "UI Rebuild",
        ["Deploy / SRE / Security", "Product Design"],
        "regression",
        ["web/ui", "go/control-plane/internal/auth"],
        ["web/ui/src/", "web/ui/e2e/", "go/control-plane/internal/auth/", "doc/02_acceptance/"],
        ["tests/run_tests.sh web"],
        [
            "/screen has exactly one public/protected/readonly strategy",
            "unauthorized behavior is verified",
            "sensitive data display policy is documented",
        ],
        [
            "doc/05_status/代码实证状态核对-2026-06-19.md#5",
            "doc/03_review/专家深评整改清单.md#SUB-FS-01",
        ],
        risk_level="high",
        reasons=["screen can expose operational data"],
        mode="local",
    ),
    task(
        "CLE-P0-P95-001",
        "完整 P95 时间戳链设计与埋点计划",
        "P0",
        "Go Control-plane",
        ["Proto / Kafka / Flink", "UI Rebuild", "Mission / Acceptance"],
        "acceptance-prep",
        ["go/control-plane", "java/flink-jobs", "web/ui", "proto"],
        ["go/control-plane/", "java/flink-jobs/", "web/ui/src/", "proto/traffic/v1/", "doc/02_acceptance/", "doc/01_design/"],
        ["tests/run_tests.sh go", "tests/run_tests.sh java", "tests/run_tests.sh web"],
        [
            "event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined",
            "P50/P90/P95/P99 report shape is specified",
            "existing dashboard metric is not mislabeled as full end-to-end P95",
        ],
        [
            "doc/05_status/代码实证状态核对-2026-06-19.md#3",
            "doc/03_review/专家深评整改清单.md#ARCH-04",
        ],
        risk_level="high",
        reasons=["can affect proto, Flink and UI semantics"],
        mode="plan",
        contracts={"proto": True, "kafka_topics": False, "database_schema": True, "apisix_routes": False},
    ),
    task(
        "CLE-P0-DLQ-001",
        "DLQ replay API、审批、审计、幂等验证",
        "P0",
        "Storage / Data Quality",
        ["Go Control-plane", "Proto / Kafka / Flink", "Mission / Acceptance"],
        "regression",
        ["go/control-plane", "java/flink-jobs", "proto", "common"],
        ["go/control-plane/", "java/flink-jobs/", "proto/traffic/v1/", "common/", "doc/02_acceptance/"],
        ["tests/run_tests.sh go", "tests/run_tests.sh java", "tests/run_tests.sh proto"],
        [
            "bad message injection path exists",
            "manual repair, approval, replay and audit are represented",
            "idempotency key and duplicate-write evidence are recorded",
        ],
        [
            "doc/05_status/代码实证状态核对-2026-06-19.md#3",
            "doc/03_review/专家深评整改清单.md#QA-05",
        ],
        risk_level="high",
        reasons=["touches replay and data correctness"],
        mode="plan",
        contracts={"proto": True, "kafka_topics": True, "database_schema": True, "apisix_routes": False},
    ),
    task(
        "CLE-P0-PCAP-001",
        "PCAP hash、签名 URL、跨租户拒绝、下载审计",
        "P0",
        "Go Control-plane",
        ["Rust Probe", "Mission / Acceptance", "Deploy / SRE / Security"],
        "regression",
        ["go/control-plane/internal/forensics", "rust/probe-agent", "proto"],
        ["go/control-plane/internal/forensics/", "rust/probe-agent/", "proto/traffic/v1/", "doc/02_acceptance/"],
        ["tests/run_tests.sh go", "tests/run_tests.sh rust"],
        [
            "downloaded PCAP hash can be verified against index",
            "expired presigned URL fails",
            "cross-tenant access returns expected denial",
            "download audit evidence exists",
        ],
        [
            "doc/05_status/代码实证状态核对-2026-06-19.md#4",
            "doc/03_review/专家深评整改清单.md#SUB-CTO-09",
        ],
        risk_level="high",
        reasons=["touches forensic evidence and tenant boundary"],
        mode="plan",
        contracts={"proto": True, "kafka_topics": False, "database_schema": True, "apisix_routes": False},
    ),
    task(
        "CLE-P0-SEC-001",
        "Kafka TLS/SASL/ACL、ExternalSecret、NetworkPolicy profile",
        "P0",
        "Deploy / SRE / Security",
        ["Proto / Kafka / Flink", "Go Control-plane", "Rust Probe"],
        "security",
        ["deployments/kubernetes", "common", "go/control-plane", "rust/probe-agent", "java/flink-jobs"],
        ["deployments/kubernetes/", "common/", "go/control-plane/", "rust/probe-agent/", "java/flink-jobs/", "doc/02_acceptance/"],
        ["kubectl apply --dry-run=client -f deployments/kubernetes"],
        [
            "production profile separates development credentials",
            "Kafka illegal produce/consume negative is specified",
            "NetworkPolicy default-deny and allow matrix are documented",
            "secret scan result is recorded",
        ],
        [
            "doc/05_status/未开发项梳理-2026-06-19.md#2",
            "doc/03_review/专家深评整改清单.md#CTO-04",
        ],
        risk_level="high",
        reasons=["touches production security boundaries"],
        mode="plan",
        contracts={"proto": False, "kafka_topics": True, "database_schema": False, "apisix_routes": True},
    ),
    task(
        "CLE-P1-FUSION-001",
        "多源融合消融实验框架",
        "P1",
        "MLOps",
        ["Product Design", "Mission / Acceptance"],
        "acceptance-prep",
        ["mlops", "doc/02_acceptance"],
        ["mlops/", "doc/02_acceptance/", "doc/01_design/"],
        ["make python-test"],
        [
            "single-source and multi-source experiment groups are defined",
            "metrics include FPR, recall, MTTR and evidence completeness",
            "report template is present",
        ],
        [
            "doc/05_status/未开发项梳理-2026-06-19.md#4",
            "doc/03_review/专家深评整改清单.md#ALG-04",
        ],
        mode="plan",
    ),
    task(
        "CLE-P1-PILOT-001",
        "试点交付包和 25 分钟演示证据脚本",
        "P1",
        "Mission / Acceptance",
        ["Product Design"],
        "third-party-prep",
        ["doc/02_acceptance", "doc/01_design"],
        ["doc/02_acceptance/", "doc/01_design/"],
        ["python scripts/codex_loop/plan.py --task scripts/codex_loop/tasks/CLE-P1-PILOT-001.yaml"],
        [
            "pilot package directory structure is defined",
            "25 minute demo maps each step to REQ-T1 evidence",
            "economic benefit and user confirmation templates are listed",
        ],
        [
            "doc/01_design/课题一产品与技术总体设计.md#7",
            "doc/05_status/未开发项梳理-2026-06-19.md#4",
        ],
        mode="plan",
    ),
]


ARCHITECTURE_MAP: dict[str, Any] = {
    "chain": [
        "rust/probe-agent",
        "go/control-plane/cmd/ingest-gateway",
        "kafka topics",
        "java/flink-jobs",
        "ClickHouse/PostgreSQL/OpenSearch/NebulaGraph/Redis/MinIO",
        "go/control-plane APIs",
        "web/ui",
        "feedback/whitelist/rule-review/mlops",
    ],
    "lanes": {
        "Rust Probe": ["rust/probe-agent"],
        "Go Control-plane": ["go/control-plane"],
        "Proto / Kafka / Flink": ["proto/traffic/v1", "common/kafka", "java/flink-jobs"],
        "UI Rebuild": ["web/ui", "doc/04_assets/ui_suite_gpt_v1"],
        "Deploy / SRE / Security": ["deployments/kubernetes", "common"],
        "MLOps": ["mlops", "deployments/kubernetes/argo-events"],
        "Mission / Acceptance": ["doc/02_acceptance", "doc/05_status", "doc/03_review"],
    },
    "evidence_policy": ["smoke", "regression", "acceptance", "third-party"],
}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--priority", default="all", choices=["all", "P0", "P1", "P2"])
    parser.add_argument("--out", default=None, help="Directory for generated task YAML files.")
    parser.add_argument("--architecture-map", default=None, help="Write architecture-map.json to this path.")
    args = parser.parse_args()

    selected = [item for item in TASKS if args.priority == "all" or item["priority"] == args.priority]

    if args.out:
        out_dir = repo_path(args.out)
        out_dir.mkdir(parents=True, exist_ok=True)
        for item in selected:
            write_yaml(out_dir / f"{item['id']}.yaml", item)

    if args.architecture_map:
        write_json(args.architecture_map, ARCHITECTURE_MAP)

    print(f"selected={len(selected)}")
    if args.out:
        print(f"tasks_dir={repo_path(args.out).relative_to(SCRIPT_ROOT.parents[1])}")
    if args.architecture_map:
        print(f"architecture_map={repo_path(args.architecture_map)}")
    for item in selected:
        print(f"{item['id']} {item['priority']} {item['lane']['primary']} {item['title']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
