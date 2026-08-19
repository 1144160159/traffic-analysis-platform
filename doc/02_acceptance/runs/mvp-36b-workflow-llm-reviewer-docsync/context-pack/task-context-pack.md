# Task Context Pack: CLE-P0-REVIEWER-001

- run_id: `mvp-36b-workflow-llm-reviewer-docsync`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- task_status: `DISCOVERED`
- lane: `Mission / Acceptance`
- acceptance_type: `regression`
- budget: `12000` chars

## Current Objective
- Continue only the bounded task `CLE-P0-REVIEWER-001` unless a design delta explicitly expands scope.
- Use original source refs for exact details; this pack is a working brief, not a source of truth.

## Scope
- execution_mode: `review`
- allow_live_write: `False`
- allowed_paths:
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Verification To Preserve
- local:
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`
- live_readonly:
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Guidance Findings
- none

## Design Signal
- status: `DESIGN_READY`
- decision: `design_ready`
- strategy: Keep this as a design-prep package; select a narrower implementation task before changing code.
- route_signal: No route-specific signal was selected for this task.

## Route Signals
- none

## Contract Signals
- none

## Evidence Signals
- `mvp-35-model-profile-smoke` [codex_runner/None]: status `CODEX_RUNNER_PLANNED`, missing `0`
- `mvp-35-workflow-model-profile` [workflow_run/acceptance-prep]: status `WORKFLOW_PREPARED`, missing `0`
- `mvp-36-final-llm-reviewer-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-36-llm-reviewer-smoke` [llm_review/None]: status `LLM_REVIEW_PLANNED`, missing `0`
- `mvp-36-workflow-llm-reviewer` [workflow_run/acceptance-prep]: status `WORKFLOW_PREPARED`, missing `0`
- `mvp-4-context-screen` [context_pack/acceptance-prep]: status `CONTEXT_PACKED`, missing `0`
- `mvp-4-post-context-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-5-post-workflow-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-6-post-task-state-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-7-post-implementation-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-8-post-evidence-repair-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-9-post-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`

## Source Refs
- `task_source` `doc/01_design/自动开发Loop引擎设计.md#9`: task provenance
- `context` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/context/context.snapshot.json`: bounded global snapshot
- `context` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/context/dependency-map.json`: route and contract signals
- `context` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/context/evidence-ledger.json`: prior run evidence state
- `guidance` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/guidance/guidance.json`: blockers and scheduling signal
- `design` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/design-summary.json`: product and architecture strategy
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/acceptance-cases.md`: acceptance-cases.md
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/architecture-evolution.md`: architecture-evolution.md
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/feature-spec.md`: feature-spec.md
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/implementation-plan.md`: implementation-plan.md
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/product-iteration.md`: product-iteration.md
- `design_doc` `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design/visual-correction.md`: visual-correction.md

## Working Rules
- Use this pack as the active context, not the whole repository history.
- When a source detail matters, open the referenced file instead of trusting memory.
- If this pack conflicts with current code, current code and fresh evidence win.
- Do not close a task from context-pack evidence alone.
- Keep smoke, regression, acceptance and third-party labels separate.
