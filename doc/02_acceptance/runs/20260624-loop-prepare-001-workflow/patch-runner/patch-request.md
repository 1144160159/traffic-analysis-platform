# Codex Patch Request: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- acceptance_type: `regression`

## Inputs
- context_pack: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design`
- guidance: `doc/02_acceptance/runs/20260624-loop-prepare-001-guidance/guidance/guidance.json`

## Allowed Paths
- `go/control-plane/`
- `java/flink-jobs/`
- `proto/traffic/v1/`
- `common/`
- `doc/02_acceptance/`

## Close Conditions
- bad message injection path exists
- manual repair, approval, replay and audit are represented
- idempotency key and duplicate-write evidence are recorded

## Verification
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh proto`

## Required Codex Output
- Provide a unified diff patch file.
- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.
- Do not edit outside Allowed Paths.
- Do not mark the task closed; evidence_check.py decides closure eligibility.
