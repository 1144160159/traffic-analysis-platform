# Implementation Brief: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- current_status: `DISCOVERED`
- execution_mode: `plan`
- patch_valid: `True`

## Inputs
- context_pack: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/design`
- plan: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/plan.md`

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

## Local Verification
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh proto`

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
