# Codex Loop Plan: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- title: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary lane: `Storage / Data Quality`
- dependent lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance type: `regression`
- execution mode: `plan`

## Source
- doc/05_status/代码实证状态核对-2026-06-19.md#3
- doc/03_review/专家深评整改清单.md#QA-05

## Scope
- `go/control-plane/`
- `java/flink-jobs/`
- `proto/traffic/v1/`
- `common/`
- `doc/02_acceptance/`

## Required Gates
- G0 Intake: agent.md, doc inputs and git status must be captured
- G1 Scope: all changes must stay inside workspace.allowed_paths
- G2 Contract: changed contracts require producer/consumer/deploy checks
- G3 Security: auth, tenant, audit and secret boundaries must be explicit
- G5 Local: smallest relevant local verification must pass or be explained
- G6 Reviewer: third-view review must have no P0/P1 blockers
- G7 Live: live evidence must match its declared evidence layer
- G8 Evidence: run-summary and required reports must exist

## Local Verification
- `tests/run_tests.sh go`
- `tests/run_tests.sh java`
- `tests/run_tests.sh proto`

## Live Readonly Verification
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Evidence
- run directory: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow`
- required: `run-summary.json`
- required: `plan.md`
- required: `local-report.md`
- required: `review-report.md`
- required: `evidence-report.md`

## Close Conditions
- bad message injection path exists
- manual repair, approval, replay and audit are represented
- idempotency key and duplicate-write evidence are recorded

## Safety Notes
- This plan does not grant permission for destructive live operations.
- Smoke/regression/acceptance/third-party evidence must remain separate.
- Any policy failure should produce a gate request instead of continuing execution.
