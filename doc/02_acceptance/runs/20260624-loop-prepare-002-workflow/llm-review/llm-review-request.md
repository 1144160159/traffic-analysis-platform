# LLM Reviewer Request: CLE-P0-P95-001

## Task
- run_id: `20260624-loop-prepare-002-workflow`
- title: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- acceptance_type: `acceptance-prep`
- execution_mode: `plan`

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
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

## Context Pack Excerpt
```text
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

```

## Design feature-spec.md
```text
# Feature Spec: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Actors
- authenticated operator
- system auditor
- third-view reviewer

## Capability
- Use the documented UI suite as the visual source of truth, then migrate page by page behind regression gates.
- The feature must expose a clear product state for authorized, unauthorized, expired, degraded and empty-data scenarios.
- The frontend must surface loading and error states without fabricating successful business data.

## Functional Requirements
- Keep behavior inside the task's allowed workspace and declared lane.
- Document any change to public UX, API semantics or evidence status before implementation closes.
- Require negative cases for auth, tenant, empty data and degraded upstream dependencies when applicable.

## Acceptance-facing Behavior
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

```

## Design architecture-evolution.md
```text
# Architecture Evolution: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Evolution Principle
- Evolve one contract boundary at a time and keep each step reversible or compatibility-preserving.
- Put product behavior, API contract, data sensitivity and verification gate in writing before code changes.
- Treat this package as design evidence, not as implementation evidence.

## Recommended Architecture Step
- Use the documented UI suite as the visual source of truth, then migrate page by page behind regression gates.

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
# Frontend Visual Correction: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Visual Source Of Truth
- `doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-UI设计套装.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-Tab页功能点与表现形式矩阵.md`
- `doc/04_assets/ui_suite_gpt_v1/README.md`

## Correction Rules
- Use the dark security-operations visual token system from the UI frontend specification.
- Keep the product title and six primary business domains aligned with the documented UI suite.
- Do not add a third-level left menu; do not turn second-level navigation into large cards or topic blocks.
- Keep `/screen`, `/dashboard` and topic/workbench pages differentiated by business purpose, not by random styling.
- For `/screen`, prioritize one-screen closure: campus topology, collection pipeline, threat posture, evidence integrity, response feedback and runtime base.
- Use real API states for loading/error/empty/degraded; visual success must not be backed by hidden mock data in production mode.
- Before broad UI rebuild, run the backup task `CLE-P0-UIBACKUP-001` or explicitly record why it is not needed.

## Visual QA Cases
- 1920x1080 screen baseline does not overlap text or navigation.
- 2K/4K display-wall scaling preserves information hierarchy.
- Unauthorized and read-only states are visibly different from normal live operation.
- Console/pageerror/requestfailed criteria remain clean during browser smoke when implementation occurs.

```

## Design acceptance-cases.md
```text
# Acceptance Cases: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Evidence Layer
- declared_acceptance_type: `acceptance-prep`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

## Local Verification Candidates
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh web`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`

```

## Patch Request Excerpt
```text
# Codex Patch Request: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- acceptance_type: `acceptance-prep`

## Inputs
- context_pack: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design`
- guidance: `doc/02_acceptance/runs/20260624-loop-prepare-002-guidance/guidance/guidance.json`

## Allowed Paths
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

## Verification
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh web`

## Required Codex Output
- Provide a unified diff patch file.
- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.
- Do not edit outside Allowed Paths.
- Do not mark the task closed; evidence_check.py decides closure eligibility.

```
