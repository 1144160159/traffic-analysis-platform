# Codex Loop Plan: CLE-P0-ROUTE-001

- run_id: `mvp-0-route-plan`
- title: routeManifest 统一菜单、路由、权限、验收点
- priority: `P0`
- status: `DISCOVERED`
- primary lane: `UI Rebuild`
- dependent lanes: Product Design, Deploy / SRE / Security
- acceptance type: `regression`
- execution mode: `local`

## Source
- doc/05_status/代码实证状态核对-2026-06-19.md#6
- doc/03_review/专家深评整改清单.md#SUB-UI-02

## Scope
- `web/ui/src/`
- `web/ui/e2e/`
- `doc/02_acceptance/`
- `doc/01_design/`

## Required Gates
- G0 Intake: agent.md, doc inputs and git status must be captured
- G1 Scope: all changes must stay inside workspace.allowed_paths
- G2 Contract: changed contracts require producer/consumer/deploy checks
- G3 Security: auth, tenant, audit and secret boundaries must be explicit
- G5 Local: smallest relevant local verification must pass or be explained
- G6 Reviewer: third-view review must have no P0/P1 blockers
- G7 Live: live evidence must match its declared evidence layer
- G8 Evidence: run-summary and required reports must exist

## Local Verification
- `tests/run_tests.sh web`

## Live Readonly Verification
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Evidence
- run directory: `doc/02_acceptance/runs/mvp-0-route-plan`
- required: `run-summary.json`
- required: `plan.md`
- required: `local-report.md`
- required: `review-report.md`
- required: `evidence-report.md`

## Close Conditions
- menu, route and breadcrumb derive from one manifest
- all six primary groups and twenty-four secondary routes are represented
- unauthorized routes have explicit behavior
- route matrix evidence exists

## Safety Notes
- This plan does not grant permission for destructive live operations.
- Smoke/regression/acceptance/third-party evidence must remain separate.
- Any policy failure should produce a gate request instead of continuing execution.
