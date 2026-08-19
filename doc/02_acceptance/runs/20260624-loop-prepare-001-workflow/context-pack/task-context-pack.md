# Task Context Pack: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- task_status: `DISCOVERED`
- lane: `Storage / Data Quality`
- acceptance_type: `regression`
- budget: `12000` chars

## Current Objective
- Continue only the bounded task `CLE-P0-DLQ-001` unless a design delta explicitly expands scope.
- Use original source refs for exact details; this pack is a working brief, not a source of truth.

## Scope
- execution_mode: `plan`
- allow_live_write: `False`
- allowed_paths:
- `go/control-plane/`
- `java/flink-jobs/`
- `proto/traffic/v1/`
- `common/`
- `doc/02_acceptance/`

## Close Conditions
- bad message injection path exists
- manual repair, approval, replay and audit are represented
- idempotency key and duplicate-write evidence are recorded

## Verification To Preserve
- local:
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh proto`
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
- `database_schema` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001
- `kafka_topics` impacts: CLE-P0-DLQ-001, CLE-P0-SEC-001
- `proto` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001

## Evidence Signals
- `mvp-45-service-objective-stop-wiring-daemon-i1-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-47-loop-one-round-first-p0-daemon-i1-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-47-post-close-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-47-screen-fix-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-48-prereq-final-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-48-prereq-reopen-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-5-post-workflow-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-50-loop-one-round-first-p0-route-daemon-i1-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-6-post-task-state-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-7-post-implementation-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-8-post-evidence-repair-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`
- `mvp-9-post-scout` [context_scout/regression]: status `CONTEXT_SCOUTED`, missing `0`

## Source Refs
- `task_source` `doc/05_status/代码实证状态核对-2026-06-19.md#3`: task provenance
- `task_source` `doc/03_review/专家深评整改清单.md#QA-05`: task provenance
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-001-scout/context/context.snapshot.json`: bounded global snapshot
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-001-scout/context/dependency-map.json`: route and contract signals
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-001-scout/context/evidence-ledger.json`: prior run evidence state
- `guidance` `doc/02_acceptance/runs/20260624-loop-prepare-001-guidance/guidance/guidance.json`: blockers and scheduling signal
- `design` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/design-summary.json`: product and architecture strategy
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/acceptance-cases.md`: acceptance-cases.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/architecture-evolution.md`: architecture-evolution.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/feature-spec.md`: feature-spec.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/implementation-plan.md`: implementation-plan.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/product-iteration.md`: product-iteration.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design/visual-correction.md`: visual-correction.md

## Working Rules
- Use this pack as the active context, not the whole repository history.
- When a source detail matters, open the referenced file instead of trusting memory.
- If this pack conflicts with current code, current code and fresh evidence win.
- Do not close a task from context-pack evidence alone.
- Keep smoke, regression, acceptance and third-party labels separate.
