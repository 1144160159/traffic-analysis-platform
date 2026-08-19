# Codex Loop Plan: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- title: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary lane: `Go Control-plane`
- dependent lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance type: `acceptance-prep`
- execution mode: `plan`

## Source
- doc/05_status/代码实证状态核对-2026-06-19.md#3
- doc/03_review/专家深评整改清单.md#ARCH-04

## Scope
- `go/control-plane/`
- `java/flink-jobs/`
- `web/ui/src/`
- `proto/traffic/v1/`
- `doc/02_acceptance/`
- `doc/01_design/`

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
- `tests/run_tests.sh web`

## Live Readonly Verification
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Evidence
- run directory: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow`
- required: `run-summary.json`
- required: `plan.md`
- required: `local-report.md`
- required: `review-report.md`
- required: `evidence-report.md`

## Close Conditions
- event_ts, ingest_ts, kafka_ts, flink_out_ts, api_seen_ts and ui_seen_ts are defined
- P50/P90/P95/P99 report shape is specified
- existing dashboard metric is not mislabeled as full end-to-end P95

## Safety Notes
- This plan does not grant permission for destructive live operations.
- Smoke/regression/acceptance/third-party evidence must remain separate.
- Any policy failure should produce a gate request instead of continuing execution.
