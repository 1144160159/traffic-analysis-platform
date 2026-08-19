# Codex Loop Plan: CLE-P0-REVIEWER-001

- run_id: `mvp-36-workflow-llm-reviewer`
- title: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary lane: `Mission / Acceptance`
- dependent lanes: Product Design
- acceptance type: `regression`
- execution mode: `review`

## Source
- doc/01_design/自动开发Loop引擎设计.md#9

## Scope
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Required Gates
- G0 Intake: agent.md, doc inputs and git status must be captured
- G1 Scope: all changes must stay inside workspace.allowed_paths
- G5 Local: smallest relevant local verification must pass or be explained
- G6 Reviewer: third-view review must have no P0/P1 blockers
- G7 Live: live evidence must match its declared evidence layer
- G8 Evidence: run-summary and required reports must exist

## Local Verification
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Live Readonly Verification
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Evidence
- run directory: `doc/02_acceptance/runs/mvp-36-workflow-llm-reviewer`
- required: `run-summary.json`
- required: `plan.md`
- required: `local-report.md`
- required: `review-report.md`
- required: `evidence-report.md`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Safety Notes
- This plan does not grant permission for destructive live operations.
- Smoke/regression/acceptance/third-party evidence must remain separate.
- Any policy failure should produce a gate request instead of continuing execution.
