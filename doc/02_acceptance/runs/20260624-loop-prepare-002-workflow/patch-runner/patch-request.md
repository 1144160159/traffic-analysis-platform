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
