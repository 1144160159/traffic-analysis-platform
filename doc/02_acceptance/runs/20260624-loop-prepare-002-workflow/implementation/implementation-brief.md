# Implementation Brief: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- current_status: `DISCOVERED`
- execution_mode: `plan`
- patch_valid: `True`

## Inputs
- context_pack: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/design`
- plan: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/plan.md`

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

## Local Verification
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh web`

## Implementation Rules
- Refresh `git status --short` before editing.
- Read the context pack and exact source refs before relying on summarized text.
- Keep changes inside workspace.allowed_paths unless a design delta expands scope.
- If touching Proto, Kafka, DB schema or APISIX routes, the task contract must declare it and consumers must be checked.
- Do not treat this brief, patch validation or context evidence as task closure.
- After patching, run the task's smallest relevant verification and third-view review.

## Patch Scope
- no patch supplied

## Blocking Findings
- none
