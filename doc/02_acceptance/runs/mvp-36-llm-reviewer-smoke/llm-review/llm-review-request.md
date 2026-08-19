# LLM Reviewer Request: CLE-P0-REVIEWER-001

## Task
- run_id: `mvp-36-llm-reviewer-smoke`
- title: 开启第三视角 Reviewer Gate
- priority: `P0`
- acceptance_type: `regression`
- execution_mode: `review`

## Required Decision
- Return JSON only.
- Use decision `pass` only when product logic, technical design, code scope and evidence are all sufficient.
- Use `repair_required` for implementation or verification gaps.
- Use `design_update_required` for product, architecture, API, data, or visual-design mismatch.
- Use `human_gate_required` for secrets, destructive data action, auth boundary risk, production safety risk, or unclear ownership.

## Static Reviewer
- decision: `pending`
- local_status: `missing`
- changed_paths: `0`
- non_blocking `NO_DIFF_TO_REVIEW` `git diff`: No patch or changed paths were available for diff-aware review.

## Semantic Reviewer
- status: `SEMANTIC_REVIEW_HELD`
- review_decision: `pending`

## Patch Intake
- patch_path: `none`
- touched_paths: `0`
- patch_valid: `True`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Context Pack Excerpt
```text
# Task Context Pack: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
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
- `mvp-3-post-design-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-33-final-executor-pool-stress-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-34-final-soak-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-35-model-profile-smoke` [codex_runner/None]: status `CODEX_RUNNER_PLANNED`, missing `0`
- `mvp-4-context-screen` [context_pack/acceptance-prep]: status `CONTEXT_PACKED`, missing `0`
- `mvp-4-post-context-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-5-post-workflow-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-6-post-task-state-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-7-post-implementation-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-8-post-evidence-repair-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-9-post-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`

## Source Refs
- `task_source` `doc/01_design/自动开发Loop引擎设计.md#9`: task provenance
- `context` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/context/context.snapshot.json`: bounded global snapshot
- `context` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/context/dependency-map.json`: route and contract signals
- `context` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/context/evidence-ledger.json`: prior run evidence state
- `guidance` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/guidance/guidance.json`: blockers and scheduling signal
- `design` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/design-summary.json`: product and architecture strategy
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/acceptance-cases.md`: acceptance-cases.md
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/architecture-evolution.md`: architecture-evolution.md
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/feature-spec.md`: feature-spec.md
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/implementation-plan.md`: implementation-plan.md
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/product-iteration.md`: product-iteration.md
- `design_doc` `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design/visual-correction.md`: visual-correction.md

## Working Rules
- Use this pack as the active context, not the whole repository history.
- When a source detail matters, open the referenced file instead of trusting memory.
- If this pack conflicts with current code, current code and fresh evidence win.
- Do not close a task from context-pack evidence alone.
- Keep smoke, regression, acceptance and third-party labels separate.

```

## Design feature-spec.md
```text
# Feature Spec: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Actors
- authenticated operator
- system auditor
- third-view reviewer

## Capability
- Keep this as a design-prep package; select a narrower implementation task before changing code.
- The feature must expose a clear product state for authorized, unauthorized, expired, degraded and empty-data scenarios.
- The frontend must surface loading and error states without fabricating successful business data.

## Functional Requirements
- Keep behavior inside the task's allowed workspace and declared lane.
- Document any change to public UX, API semantics or evidence status before implementation closes.
- Require negative cases for auth, tenant, empty data and degraded upstream dependencies when applicable.

## Acceptance-facing Behavior
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

```

## Design architecture-evolution.md
```text
# Architecture Evolution: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Evolution Principle
- Evolve one contract boundary at a time and keep each step reversible or compatibility-preserving.
- Put product behavior, API contract, data sensitivity and verification gate in writing before code changes.
- Treat this package as design evidence, not as implementation evidence.

## Recommended Architecture Step
- Keep this as a design-prep package; select a narrower implementation task before changing code.

## Dependency Signals
- `apisix_routes` impacts: CLE-P0-ROUTE-001, CLE-P0-SEC-001
- `database_schema` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001
- `kafka_topics` impacts: CLE-P0-DLQ-001, CLE-P0-SEC-001
- `proto` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001

## Architecture Stop Conditions
- Do not close the task from this package alone.
- Keep smoke, regression, acceptance and third-party evidence as separate evidence layers.
- Prefer existing repository contracts and UI documents over newly invented behavior.

```

## Design visual-correction.md
```text
# Frontend Visual Correction: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Applicability
- This task is not primarily a frontend visual task.
- Keep this file as a reminder to check visual impact only if UI surfaces change.

```

## Design acceptance-cases.md
```text
# Acceptance Cases: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Evidence Layer
- declared_acceptance_type: `regression`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Local Verification Candidates
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`

```

## Patch Request Excerpt
```text
# Codex Patch Request: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- acceptance_type: `regression`

## Inputs
- context_pack: `doc/02_acceptance/runs/mvp-35-workflow-model-profile/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/mvp-35-workflow-model-profile/design`
- guidance: `doc/02_acceptance/runs/mvp-35-workflow-model-profile/guidance/guidance.json`

## Allowed Paths
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Verification
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Required Codex Output
- Provide a unified diff patch file.
- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.
- Do not edit outside Allowed Paths.
- Do not mark the task closed; evidence_check.py decides closure eligibility.

```
