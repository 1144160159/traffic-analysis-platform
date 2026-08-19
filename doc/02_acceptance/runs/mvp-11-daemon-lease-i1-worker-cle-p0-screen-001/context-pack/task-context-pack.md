# Task Context Pack: CLE-P0-SCREEN-001

- run_id: `mvp-11-daemon-lease-i1-worker-cle-p0-screen-001`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- task_status: `DISCOVERED`
- lane: `UI Rebuild`
- acceptance_type: `regression`
- budget: `12000` chars

## Current Objective
- Continue only the bounded task `CLE-P0-SCREEN-001` unless a design delta explicitly expands scope.
- Use original source refs for exact details; this pack is a working brief, not a source of truth.

## Scope
- execution_mode: `local`
- allow_live_write: `False`
- allowed_paths:
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`

## Close Conditions
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented

## Verification To Preserve
- local:
- `tests/run_tests.sh web`
- live_readonly:
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Guidance Findings
- `warning` `HIGH_RISK_LOCAL`: High-risk task is allowed to enter local mode. Suggestion: Keep security and reviewer gates mandatory; consider planning before implementation.
- `blocker` `SCREEN_AUTH_BOUNDARY`: /screen is outside ProtectedLayout. Suggestion: Resolve the /screen public/protected/readonly strategy before claiming UI auth boundary closure.

## Design Signal
- status: `DESIGN_ITERATING`
- decision: `design_iteration_required`
- strategy: Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data.
- route_signal: `/screen` is currently detected outside ProtectedLayout at web/ui/src/App.tsx:75.

## Route Signals
- `/screen` line `75` protected=`False` note=screen outside ProtectedLayout

## Contract Signals
- none

## Evidence Signals
- `mvp-4-context-screen` [context_pack/acceptance-prep]: status `CONTEXT_PACKED`, missing `0`
- `mvp-4-post-context-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-5-post-workflow-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-5-workflow-screen` [workflow_run/acceptance-prep]: status `DESIGN_ITERATING`, missing `0`
- `mvp-6-post-task-state-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-7-implementation-screen` [implementation_guard/acceptance-prep]: status `IMPLEMENTATION_BLOCKED`, missing `0`
- `mvp-7-post-implementation-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-7-workflow-screen` [workflow_run/acceptance-prep]: status `DESIGN_ITERATING`, missing `0`
- `mvp-8-evidence-repair` [workflow_run/acceptance-prep]: status `DESIGN_ITERATING`, missing `0`
- `mvp-8-post-evidence-repair-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-9-patch-review-scheduler` [workflow_run/acceptance-prep]: status `DESIGN_ITERATING`, missing `0`
- `mvp-9-post-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`

## Source Refs
- `task_source` `doc/05_status/代码实证状态核对-2026-06-19.md#5`: task provenance
- `task_source` `doc/03_review/专家深评整改清单.md#SUB-FS-01`: task provenance
- `context` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-scout/context/context.snapshot.json`: bounded global snapshot
- `context` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-scout/context/dependency-map.json`: route and contract signals
- `context` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-scout/context/evidence-ledger.json`: prior run evidence state
- `guidance` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1/guidance/guidance.json`: blockers and scheduling signal
- `design` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/design-summary.json`: product and architecture strategy
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/acceptance-cases.md`: acceptance-cases.md
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/architecture-evolution.md`: architecture-evolution.md
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/feature-spec.md`: feature-spec.md
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/implementation-plan.md`: implementation-plan.md
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/product-iteration.md`: product-iteration.md
- `design_doc` `doc/02_acceptance/runs/mvp-11-daemon-lease-i1-worker-cle-p0-screen-001/design/visual-correction.md`: visual-correction.md

## Working Rules
- Use this pack as the active context, not the whole repository history.
- When a source detail matters, open the referenced file instead of trusting memory.
- If this pack conflicts with current code, current code and fresh evidence win.
- Do not close a task from context-pack evidence alone.
- Keep smoke, regression, acceptance and third-party labels separate.
