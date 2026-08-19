# Task Context Pack: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- task_status: `DISCOVERED`
- lane: `Go Control-plane`
- acceptance_type: `acceptance-prep`
- budget: `12000` chars

## Current Objective
- Continue only the bounded task `CLE-P0-P95-001` unless a design delta explicitly expands scope.
- Use original source refs for exact details; this pack is a working brief, not a source of truth.

## Scope
- execution_mode: `plan`
- allow_live_write: `False`
- allowed_paths:
- `go/control-plane/`
- `java/flink-jobs/`
- `web/ui/src/`
- `proto/traffic/v1/`
- `doc/02_acceptance/`
- `doc/01_design/`

## Close Conditions
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

## Verification To Preserve
- local:
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh web`
- live_readonly:
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Guidance Findings
- none

## Design Signal
- status: `DESIGN_READY`
- decision: `design_ready`
- strategy: Use the documented UI suite as the visual source of truth, then migrate page by page behind regression gates.
- route_signal: No route-specific signal was selected for this task.

## Route Signals
- `/login` line `74` protected=`False` note=none
- `/` line `75` protected=`False` note=none
- `/dashboard` line `77` protected=`True` note=none
- `/screen` line `78` protected=`True` note=none
- `/topics` line `79` protected=`True` note=none
- `/probes` line `80` protected=`True` note=none
- `/data-quality` line `81` protected=`True` note=none
- `/alerts` line `82` protected=`True` note=none
- `/alerts/:alertId` line `83` protected=`True` note=none
- `/campaigns` line `84` protected=`True` note=none
- `/campaigns/:campaignId` line `85` protected=`True` note=none
- `/attack-chains` line `86` protected=`True` note=none
- `/encrypted-traffic` line `87` protected=`True` note=none
- `/forensics` line `88` protected=`True` note=none
- `/assets` line `89` protected=`True` note=none
- `/graph` line `90` protected=`True` note=none
- `/fusion` line `91` protected=`True` note=none
- `/baselines` line `92` protected=`True` note=none
- `/rules` line `93` protected=`True` note=none
- `/deployments` line `94` protected=`True` note=none
- `/models` line `95` protected=`True` note=none
- `/mlops` line `96` protected=`True` note=none
- `/playbooks` line `97` protected=`True` note=none
- `/whitelist` line `98` protected=`True` note=none
- `/compliance` line `99` protected=`True` note=none
- `/audit-log` line `100` protected=`True` note=none
- `/notifications` line `101` protected=`True` note=none
- `/settings` line `102` protected=`True` note=none
- `/*` line `104` protected=`False` note=none

## Contract Signals
- `database_schema` impacts: CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001
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
- `task_source` `doc/03_review/专家深评整改清单.md#ARCH-04`: task provenance
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-002-scout/context/context.snapshot.json`: bounded global snapshot
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-002-scout/context/dependency-map.json`: route and contract signals
- `context` `doc/02_acceptance/runs/20260624-loop-prepare-002-scout/context/evidence-ledger.json`: prior run evidence state
- `guidance` `doc/02_acceptance/runs/20260624-loop-prepare-002-guidance/guidance/guidance.json`: blockers and scheduling signal
- `design` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/design-summary.json`: product and architecture strategy
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/acceptance-cases.md`: acceptance-cases.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/architecture-evolution.md`: architecture-evolution.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/feature-spec.md`: feature-spec.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/implementation-plan.md`: implementation-plan.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/product-iteration.md`: product-iteration.md
- `design_doc` `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design/visual-correction.md`: visual-correction.md

## Working Rules
- Use this pack as the active context, not the whole repository history.
- When a source detail matters, open the referenced file instead of trusting memory.
- If this pack conflicts with current code, current code and fresh evidence win.
- Do not close a task from context-pack evidence alone.
- Keep smoke, regression, acceptance and third-party labels separate.
