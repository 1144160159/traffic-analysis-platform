#!/usr/bin/env python3
"""Generate a slow-evolution design package from God View and Guidance outputs."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    copy_task_snapshot,
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    make_run_id,
    rel_path,
    repo_path,
    run_git,
    write_json,
    write_text,
)


CORE_DESIGN_OUTPUTS = [
    "product-iteration.md",
    "feature-spec.md",
    "user-flow.md",
    "state-machine.md",
    "api-contract.md",
    "data-contract.md",
    "visual-correction.md",
    "architecture-evolution.md",
    "acceptance-cases.md",
    "implementation-plan.md",
]

UI_DESIGN_SOURCES = [
    "doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md",
    "doc/01_design/面向园区网络的全流量采集分析系统-UI设计套装.md",
    "doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md",
    "doc/01_design/面向园区网络的全流量采集分析系统-Tab页功能点与表现形式矩阵.md",
    "doc/04_assets/ui_suite_gpt_v1/README.md",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        raise FileNotFoundError(path)
    return json.loads(path.read_text(encoding="utf-8"))


def load_context(context_dir: Path | None) -> dict[str, Any]:
    if not context_dir:
        return {}
    files = {
        "context": "context.snapshot.json",
        "gaps": "gap-index.json",
        "deps": "dependency-map.json",
        "evidence": "evidence-ledger.json",
    }
    loaded: dict[str, Any] = {}
    for key, filename in files.items():
        path = context_dir / filename
        if path.exists():
            loaded[key] = load_json(path)
    return loaded


def md_list(items: list[Any], fallback: str = "none") -> list[str]:
    values = [str(item) for item in items if item not in (None, "")]
    return [f"- {item}" for item in values] if values else [f"- {fallback}"]


def md_code_list(items: list[Any], fallback: str = "none") -> list[str]:
    values = [str(item) for item in items if item not in (None, "")]
    return [f"- `{item}`" for item in values] if values else [f"- {fallback}"]


def task_label(task: dict[str, Any]) -> str:
    return f"{task.get('id')}: {task.get('title')}"


def is_ui_task(task: dict[str, Any]) -> bool:
    text = " ".join(
        [
            str(task.get("id", "")),
            str(task.get("title", "")),
            str((task.get("lane") or {}).get("primary", "")),
            " ".join(str(item) for item in list_of(task.get("subsystems"))),
        ]
    ).lower()
    return any(token in text for token in ["screen", "route", "ui", "web/ui", "frontend", "layout"])


def is_screen_task(task: dict[str, Any]) -> bool:
    text = f"{task.get('id', '')} {task.get('title', '')}".lower()
    return "screen" in text or "/screen" in text


def relevant_findings(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    task_id = str(task.get("id"))
    return [item for item in guidance.get("findings", []) if str(item.get("target")) == task_id]


def relevant_recommendations(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    task_id = str(task.get("id"))
    return [item for item in guidance.get("recommended_next", []) if str(item.get("id")) == task_id]


def route_signal(task: dict[str, Any], context: dict[str, Any]) -> str:
    if not is_screen_task(task):
        return "No route-specific signal was selected for this task."
    for route in ((context.get("deps") or {}).get("routes") or {}).get("routes", []):
        if route.get("path") == "/screen":
            if route.get("note"):
                return f"`/screen` is currently detected outside ProtectedLayout at {route.get('source', 'web/ui/src/App.tsx')}:{route.get('line')}."
            return "`/screen` is detected and currently appears to be inside the protected shell."
    return "`/screen` route was not found in the current dependency map."


def design_strategy(task: dict[str, Any], context: dict[str, Any], guidance: dict[str, Any]) -> dict[str, Any]:
    blockers = [item for item in relevant_findings(task, guidance) if item.get("level") == "blocker"]
    ui_task = is_ui_task(task)
    screen_task = is_screen_task(task)
    strategy = {
        "decision": "design_ready",
        "headline": "Use a slow-evolution design package before implementation.",
        "guardrails": [
            "Do not close the task from this package alone.",
            "Keep smoke, regression, acceptance and third-party evidence as separate evidence layers.",
            "Prefer existing repository contracts and UI documents over newly invented behavior.",
        ],
        "product_principles": [
            "Make the next iteration explainable to product, frontend, backend, SRE and acceptance reviewers.",
            "Design one reversible step at a time, then validate with the smallest relevant gate.",
            "Avoid hidden scope expansion: a task may recommend adjacent work, but implementation still follows task boundaries.",
        ],
        "architecture_principles": [
            "Change observable contracts before internal rewrites only when consumers and deploy manifests are named.",
            "For auth, tenant, audit, Kafka, DB, Proto and APISIX boundaries, require explicit negative cases.",
            "A slow evolution step must have a rollback or compatibility story before local implementation starts.",
        ],
    }
    if blockers:
        strategy["decision"] = "design_iteration_required"
        strategy["headline"] = "Resolve blocker-class design choices before implementation or closure."
    if screen_task:
        strategy["recommended_strategy"] = (
            "Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token "
            "with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data."
        )
        strategy["product_principles"].extend(
            [
                "Treat `/screen` as a command display surface, not an unauthenticated operations data leak.",
                "Separate wall-display/read-only/demo modes in product copy, API claims and frontend state.",
            ]
        )
        strategy["architecture_principles"].extend(
            [
                "Do not bypass auth inside the React route tree; model exceptions as scoped capability checks.",
                "Realtime channels for `/screen` must inherit the same token and tenant boundary as initial data fetches.",
            ]
        )
    elif ui_task:
        strategy["recommended_strategy"] = (
            "Use the documented UI suite as the visual source of truth, then migrate page by page behind regression gates."
        )
    else:
        strategy["recommended_strategy"] = (
            "Keep this as a design-prep package; select a narrower implementation task before changing code."
        )
    strategy["route_signal"] = route_signal(task, context)
    return strategy


def render_header(title: str, task: dict[str, Any], run_id: str) -> list[str]:
    lane = task.get("lane") or {}
    return [
        f"# {title}: {task.get('id')}",
        "",
        f"- run_id: `{run_id}`",
        f"- task: {task.get('title')}",
        f"- priority: `{task.get('priority')}`",
        f"- status: `{task.get('status')}`",
        f"- primary_lane: `{lane.get('primary')}`",
        f"- dependent_lanes: {', '.join(list_of(lane.get('dependent'))) or 'none'}",
        f"- acceptance_type: `{task.get('acceptance_type')}`",
        "",
    ]


def render_product_iteration(task: dict[str, Any], run_id: str, strategy: dict[str, Any], guidance: dict[str, Any]) -> str:
    lines = render_header("Product Iteration", task, run_id)
    lines.extend(
        [
            "## Product Decision",
            f"- {strategy['headline']}",
            f"- recommended_strategy: {strategy['recommended_strategy']}",
            "",
            "## Product Value",
            "- Turn the current gap into a visible iteration with owner-readable intent, scope and acceptance boundaries.",
            "- Make the feature safer to discuss: product behavior, data sensitivity, visual target and verification are separated.",
            "- Preserve the option to stop after design review if the next step would widen security, data or deployment scope.",
            "",
            "## Source Signals",
        ]
    )
    lines.extend(md_list(list_of(task.get("source"))))
    lines.extend(["", "## Guidance Signals"])
    findings = relevant_findings(task, guidance)
    if findings:
        for item in findings:
            lines.append(f"- `{item.get('level')}` `{item.get('code')}`: {item.get('message')} Suggestion: {item.get('suggestion')}")
    else:
        lines.append("- none")
    recs = relevant_recommendations(task, guidance)
    if recs:
        lines.extend(["", "## Scheduling Signal"])
        for item in recs:
            lines.append(f"- score `{item.get('score')}` in guidance ranking; mode `{item.get('mode')}`; lane `{item.get('lane')}`.")
    lines.extend(
        [
            "",
            "## Product Non-goals",
            "- This package does not mark the feature Done, Acceptance Ready or Third-party Passed.",
            "- This package does not authorize live writes or destructive infrastructure changes.",
            "- This package does not replace PRD/SDD updates when the implementation changes user-facing behavior.",
            "",
        ]
    )
    return "\n".join(lines)


def render_feature_spec(task: dict[str, Any], run_id: str, strategy: dict[str, Any]) -> str:
    screen_task = is_screen_task(task)
    actors = ["authenticated operator", "system auditor", "third-view reviewer"]
    if screen_task:
        actors.extend(["display-wall viewer with read-only token", "demo viewer with desensitized data"])
    lines = render_header("Feature Spec", task, run_id)
    lines.extend(["## Actors"])
    lines.extend(md_list(actors))
    lines.extend(
        [
            "",
            "## Capability",
            f"- {strategy['recommended_strategy']}",
            "- The feature must expose a clear product state for authorized, unauthorized, expired, degraded and empty-data scenarios.",
            "- The frontend must surface loading and error states without fabricating successful business data.",
            "",
            "## Functional Requirements",
        ]
    )
    if screen_task:
        lines.extend(
            [
                "- `/screen` default access path uses the same protected shell/auth decision as other operational pages.",
                "- Read-only token mode may read only scoped screen data and cannot call mutation endpoints.",
                "- Expired, missing or invalid token state must fail closed with a clear non-sensitive state.",
                "- Demo/desensitized mode must be explicit and visually distinguishable from live operations data.",
                "- WebSocket or polling refresh must reuse the same auth, tenant and site boundary as initial queries.",
            ]
        )
    else:
        lines.extend(
            [
                "- Keep behavior inside the task's allowed workspace and declared lane.",
                "- Document any change to public UX, API semantics or evidence status before implementation closes.",
                "- Require negative cases for auth, tenant, empty data and degraded upstream dependencies when applicable.",
            ]
        )
    lines.extend(
        [
            "",
            "## Acceptance-facing Behavior",
        ]
    )
    lines.extend(md_list(list_of(task.get("close_when"))))
    lines.extend([""])
    return "\n".join(lines)


def render_user_flow(task: dict[str, Any], run_id: str, strategy: dict[str, Any]) -> str:
    lines = render_header("User Flow", task, run_id)
    if is_screen_task(task):
        lines.extend(
            [
                "## Primary Flow",
                "- Operator opens `/screen` from an authenticated session.",
                "- Frontend resolves tenant, site and time window.",
                "- Frontend requests screen summary, topology, collection health, alert posture and evidence integrity from real APIs.",
                "- Realtime refresh starts only after the same auth boundary is confirmed.",
                "- Display shows a one-screen command view with no mutation controls.",
                "",
                "## Read-only Token Flow",
                "- Admin provisions a scoped display token outside this task's default implementation path.",
                "- Display wall opens `/screen` with token context.",
                "- Backend verifies token scope, expiry, tenant and site.",
                "- Frontend shows read-only data and a visible read-only mode marker.",
                "- Expiry moves the screen to a closed, non-sensitive state.",
                "",
                "## Negative Flow",
                "- Missing auth or token is rejected.",
                "- Cross-tenant or cross-site claims are rejected.",
                "- API 401/403/5xx states do not fall back to fake success.",
                "",
            ]
        )
    else:
        lines.extend(
            [
                "## Primary Flow",
                "- User enters the feature through an existing route or operational workflow.",
                "- The UI/API validates auth, tenant and data availability before showing business results.",
                "- The task produces evidence for the declared acceptance layer.",
                "",
                "## Negative Flow",
                "- Unauthorized, cross-tenant, empty and degraded states are explicit.",
                "- Failure states cannot be hidden by mock data unless `VITE_USE_MOCK=true` is explicitly active for local development.",
                "",
            ]
        )
    lines.extend(["## Design Guardrail", f"- {strategy['route_signal']}", ""])
    return "\n".join(lines)


def render_state_machine(task: dict[str, Any], run_id: str) -> str:
    lines = render_header("State Machine", task, run_id)
    if is_screen_task(task):
        states = [
            ("UNINITIALIZED", "route has mounted, no auth decision yet"),
            ("AUTH_CHECKING", "session or read-only token is being verified"),
            ("AUTHORIZED_LIVE", "authenticated operator can view live screen data"),
            ("AUTHORIZED_READONLY", "scoped display token can view scoped/desensitized read-only data"),
            ("DEMO_DESENSITIZED", "explicit demo mode uses non-sensitive fixture or generated-safe data"),
            ("DENIED", "missing/invalid/expired/cross-tenant access"),
            ("DEGRADED", "auth ok, but one or more upstream data APIs are unavailable"),
        ]
    else:
        states = [
            ("DISCOVERED", "task exists but no implementation plan is approved"),
            ("DESIGN_ITERATING", "product, visual or technical design still has open decisions"),
            ("DESIGN_READY", "design package is reviewable and implementation can be planned"),
            ("LOCAL_VERIFIED", "smallest relevant local gate passed"),
            ("REVIEW_REQUIRED", "third-view review still pending"),
            ("CLOSED", "all close_when and evidence files are satisfied"),
        ]
    lines.extend(["## States"])
    for name, desc in states:
        lines.append(f"- `{name}`: {desc}")
    lines.extend(
        [
            "",
            "## Required Transitions",
            "- Any blocker from guidance keeps the task in `DESIGN_ITERATING` or equivalent; it cannot close.",
            "- Any auth, tenant, contract or evidence-layer change requires reviewer confirmation before closure.",
            "- Any failed verification moves the task to repair/planning state, not to closed.",
            "",
        ]
    )
    return "\n".join(lines)


def render_api_contract(task: dict[str, Any], run_id: str) -> str:
    lines = render_header("API Contract Sketch", task, run_id)
    contracts = task.get("contracts") or {}
    lines.extend(["## Contract Impact Declared By Task"])
    for key in ["proto", "kafka_topics", "database_schema", "apisix_routes"]:
        lines.append(f"- `{key}`: `{bool(contracts.get(key))}`")
    if is_screen_task(task):
        lines.extend(
            [
                "",
                "## API Shape To Confirm Before Implementation",
                "- `GET /api/v1/screen/summary`: returns scoped KPI, collection health and evidence integrity.",
                "- `GET /api/v1/screen/topology`: returns scoped campus/topology view without mutation affordances.",
                "- `GET /api/v1/screen/alerts`: returns scoped high-level alert posture, not raw sensitive details by default.",
                "- `GET /api/v1/auth/me` or equivalent session probe remains the default auth check.",
                "- Read-only token verification is an auth capability, not a frontend-only bypass.",
                "",
                "## Negative Contract Cases",
                "- 401 for missing/expired auth.",
                "- 403 for cross-tenant, cross-site or mutation attempts under read-only token.",
                "- 5xx/degraded upstream must be visible as degraded state and cannot silently fabricate live data.",
            ]
        )
    else:
        lines.extend(
            [
                "",
                "## API Shape To Confirm Before Implementation",
                "- Name the existing API/service/repository layer before creating a new endpoint.",
                "- If new API is needed, define auth, tenant, audit, pagination and error semantics first.",
                "- If no API is changed, record that this package is UI/doc/verification-only.",
            ]
        )
    lines.append("")
    return "\n".join(lines)


def render_data_contract(task: dict[str, Any], run_id: str) -> str:
    data_plan = task.get("data_plan") or {}
    lines = render_header("Data Contract Sketch", task, run_id)
    lines.extend(
        [
            "## Data Plan",
            f"- mode: `{data_plan.get('mode')}`",
            f"- tenant: `{data_plan.get('tenant')}`",
            f"- cleanup: `{data_plan.get('cleanup')}`",
            "",
            "## Data Rules",
            "- Prefer real API/DB/Kafka paths for verification; mock data cannot prove live integration.",
            "- Generated live data requires run_id, tenant scoping and cleanup before execution.",
            "- Sensitive data policy must be explicit for screenshots, browser reports and acceptance artifacts.",
        ]
    )
    if is_screen_task(task):
        lines.extend(
            [
                "- `/screen` may aggregate live operational metrics, but public/demo views must be desensitized.",
                "- Display-wall tokens must not expose raw PCAP, user identity, secrets, high-cardinality logs or tenant-crossing data.",
                "- Screenshot evidence must avoid leaking secrets or credentials.",
            ]
        )
    lines.append("")
    return "\n".join(lines)


def render_visual_correction(task: dict[str, Any], run_id: str) -> str:
    lines = render_header("Frontend Visual Correction", task, run_id)
    if not is_ui_task(task):
        lines.extend(
            [
                "## Applicability",
                "- This task is not primarily a frontend visual task.",
                "- Keep this file as a reminder to check visual impact only if UI surfaces change.",
                "",
            ]
        )
        return "\n".join(lines)

    lines.extend(
        [
            "## Visual Source Of Truth",
        ]
    )
    lines.extend(md_code_list(UI_DESIGN_SOURCES))
    lines.extend(
        [
            "",
            "## Correction Rules",
            "- Use the dark security-operations visual token system from the UI frontend specification.",
            "- Keep the product title and six primary business domains aligned with the documented UI suite.",
            "- Do not add a third-level left menu; do not turn second-level navigation into large cards or topic blocks.",
            "- Keep `/screen`, `/dashboard` and topic/workbench pages differentiated by business purpose, not by random styling.",
            "- For `/screen`, prioritize one-screen closure: campus topology, collection pipeline, threat posture, evidence integrity, response feedback and runtime base.",
            "- Use real API states for loading/error/empty/degraded; visual success must not be backed by hidden mock data in production mode.",
            "- Before broad UI rebuild, run the backup task `CLE-P0-UIBACKUP-001` or explicitly record why it is not needed.",
            "",
            "## Visual QA Cases",
            "- 1920x1080 screen baseline does not overlap text or navigation.",
            "- 2K/4K display-wall scaling preserves information hierarchy.",
            "- Unauthorized and read-only states are visibly different from normal live operation.",
            "- Console/pageerror/requestfailed criteria remain clean during browser smoke when implementation occurs.",
            "",
        ]
    )
    return "\n".join(lines)


def render_architecture_evolution(task: dict[str, Any], run_id: str, strategy: dict[str, Any], context: dict[str, Any]) -> str:
    lines = render_header("Architecture Evolution", task, run_id)
    lines.extend(
        [
            "## Evolution Principle",
            "- Evolve one contract boundary at a time and keep each step reversible or compatibility-preserving.",
            "- Put product behavior, API contract, data sensitivity and verification gate in writing before code changes.",
            "- Treat this package as design evidence, not as implementation evidence.",
            "",
            "## Recommended Architecture Step",
            f"- {strategy['recommended_strategy']}",
            "",
            "## Dependency Signals",
        ]
    )
    deps = context.get("deps") or {}
    contract_to_tasks = deps.get("contract_to_tasks") or {}
    if contract_to_tasks:
        for contract, tasks in sorted(contract_to_tasks.items()):
            if task.get("id") in tasks or contract in {"proto", "database_schema", "kafka_topics", "apisix_routes"}:
                lines.append(f"- `{contract}` impacts: {', '.join(tasks)}")
    else:
        lines.append("- no context dependency map available")
    if is_screen_task(task):
        lines.extend(
            [
                "",
                "## Slow-evolution Slices For `/screen`",
                "- Slice 1: make auth strategy explicit and verify unauthorized behavior.",
                "- Slice 2: centralize route/menu metadata so `/screen` is not a hidden exception.",
                "- Slice 3: add scoped read-only display-token capability only if product owners confirm display-wall need.",
                "- Slice 4: align realtime refresh and API polling with the same auth/tenant boundary.",
                "- Slice 5: run visual/browser regression after the old frontend has been backed up.",
            ]
        )
    lines.extend(["", "## Architecture Stop Conditions"])
    for guardrail in strategy["guardrails"]:
        lines.append(f"- {guardrail}")
    lines.append("")
    return "\n".join(lines)


def render_acceptance_cases(task: dict[str, Any], run_id: str) -> str:
    verification = task.get("verification") or {}
    lines = render_header("Acceptance Cases", task, run_id)
    lines.extend(
        [
            "## Evidence Layer",
            f"- declared_acceptance_type: `{task.get('acceptance_type')}`",
            "- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.",
            "",
            "## Close Conditions To Preserve",
        ]
    )
    lines.extend(md_list(list_of(task.get("close_when"))))
    lines.extend(["", "## Local Verification Candidates"])
    lines.extend(md_code_list(list_of(verification.get("local"))))
    lines.extend(["", "## Live-readonly Verification Candidates"])
    lines.extend(md_code_list(list_of(verification.get("live_readonly"))))
    if is_screen_task(task):
        lines.extend(
            [
                "",
                "## Required Negative Cases",
                "- missing session/token",
                "- expired read-only token",
                "- cross-tenant or cross-site token claim",
                "- mutation attempt under read-only token",
                "- API degraded state without fake success data",
                "- browser smoke: no 4xx/5xx except expected auth negatives, no requestfailed, no non-warning console/pageerror",
            ]
        )
    lines.append("")
    return "\n".join(lines)


def render_implementation_plan(task: dict[str, Any], run_id: str, strategy: dict[str, Any], guidance: dict[str, Any]) -> str:
    workspace = task.get("workspace") or {}
    lines = render_header("Implementation Plan", task, run_id)
    lines.extend(
        [
            "## Phase 0: Review Design Package",
            "- Confirm product behavior, data sensitivity, API boundary and visual source of truth.",
            "- Keep any unresolved blocker in DESIGN_ITERATING; do not start closure work.",
            "",
            "## Phase 1: Protect Existing Work",
        ]
    )
    if is_ui_task(task):
        lines.append("- Run or explicitly account for `CLE-P0-UIBACKUP-001` before broad visual rebuild.")
    else:
        lines.append("- Capture git status and task snapshot before implementation.")
    lines.extend(
        [
            "",
            "## Phase 2: Implement Narrow Slice",
            "- Change only files inside the task allowed paths unless a new task/design-delta expands scope.",
        ]
    )
    lines.extend(md_code_list(list_of(workspace.get("allowed_paths"))))
    if is_screen_task(task):
        lines.extend(
            [
                "- First slice should settle `/screen` auth/read-only/desensitized behavior before visual polish.",
                "- Keep read-only token server-verified if implemented; do not rely on frontend-only checks.",
            ]
        )
    lines.extend(
        [
            "",
            "## Phase 3: Verify",
            "- Run the smallest declared local gate first.",
            "- Add browser/API negative checks when auth or visual behavior changes.",
            "- Record failures as repair input, not as acceptance evidence.",
            "",
            "## Phase 4: Review And Evidence",
            "- Run third-view review.",
            "- Update `design-delta.md` if implementation changes product or architecture decisions.",
            "- Keep evidence type honest: design package, smoke, regression, acceptance and third-party are not interchangeable.",
            "",
            "## Guidance Status Suggestions",
        ]
    )
    suggestions = [item for item in guidance.get("status_suggestions", []) if item.get("target") == task.get("id")]
    if suggestions:
        for item in suggestions:
            lines.append(f"- `{item.get('from')}` -> `{item.get('to')}` because {item.get('reason')}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def build_outputs(task: dict[str, Any], run_id: str, context: dict[str, Any], guidance: dict[str, Any]) -> dict[str, str]:
    strategy = design_strategy(task, context, guidance)
    return {
        "product-iteration.md": render_product_iteration(task, run_id, strategy, guidance),
        "feature-spec.md": render_feature_spec(task, run_id, strategy),
        "user-flow.md": render_user_flow(task, run_id, strategy),
        "state-machine.md": render_state_machine(task, run_id),
        "api-contract.md": render_api_contract(task, run_id),
        "data-contract.md": render_data_contract(task, run_id),
        "visual-correction.md": render_visual_correction(task, run_id),
        "architecture-evolution.md": render_architecture_evolution(task, run_id, strategy, context),
        "acceptance-cases.md": render_acceptance_cases(task, run_id),
        "implementation-plan.md": render_implementation_plan(task, run_id, strategy, guidance),
    }


def build_summary(
    task: dict[str, Any],
    run_id: str,
    design_dir: Path,
    context_dir: Path | None,
    guidance_path: Path | None,
    context: dict[str, Any],
    guidance: dict[str, Any],
) -> dict[str, Any]:
    strategy = design_strategy(task, context, guidance)
    findings = relevant_findings(task, guidance)
    blockers = [item for item in findings if item.get("level") == "blocker"]
    return {
        "run_id": run_id,
        "run_kind": "design_package",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": "DESIGN_ITERATING" if blockers else "DESIGN_READY",
        "evidence_type": "acceptance-prep",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "context_dir": rel_path(context_dir) if context_dir else None,
        "guidance_path": rel_path(guidance_path) if guidance_path else None,
        "design_dir": rel_path(design_dir),
        "decision": strategy["decision"],
        "recommended_strategy": strategy["recommended_strategy"],
        "route_signal": strategy["route_signal"],
        "blockers": blockers,
        "warnings": [item for item in findings if item.get("level") == "warning"],
        "outputs": [f"design/{name}" for name in CORE_DESIGN_OUTPUTS],
        "notes": [
            "This package is design and acceptance-prep evidence only.",
            "It does not execute local checks, browser checks, live-readonly checks or live writes.",
            "Task closure still requires close_when, verification, reviewer and evidence gates.",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True, help="Task YAML path.")
    parser.add_argument("--context-dir", default=None, help="Context Scout output directory.")
    parser.add_argument("--guidance", default=None, help="guidance.json path.")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "design")))
    run_dir = ensure_run_dir(run_id)
    design_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "design"
    design_dir.mkdir(parents=True, exist_ok=True)

    context_dir = repo_path(args.context_dir) if args.context_dir else None
    guidance_path = repo_path(args.guidance) if args.guidance else None
    context = load_context(context_dir)
    guidance = load_json(guidance_path)

    copy_task_snapshot(task_path, run_dir)
    outputs = build_outputs(task, run_id, context, guidance)
    for filename, content in outputs.items():
        write_text(design_dir / filename, content)

    summary = build_summary(task, run_id, design_dir, context_dir, guidance_path, context, guidance)
    write_json(design_dir / "design-summary.json", summary)
    write_json(run_dir / "run-summary.json", summary)
    print(design_dir)
    print(f"status={summary['status']} outputs={len(outputs)} task={task_label(task)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
