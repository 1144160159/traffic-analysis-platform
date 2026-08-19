# Codex Loop Plan: CLE-P0-SCREEN-001

- run_id: `mvp-16-workflow-runner-prepare`
- title: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary lane: `UI Rebuild`
- dependent lanes: Deploy / SRE / Security, Product Design
- acceptance type: `regression`
- execution mode: `local`

## Source
- doc/05_status/代码实证状态核对-2026-06-19.md#5
- doc/03_review/专家深评整改清单.md#SUB-FS-01

## Scope
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`

## Required Gates
- G0 Intake: agent.md, doc inputs and git status must be captured
- G1 Scope: all changes must stay inside workspace.allowed_paths
- G3 Security: auth, tenant, audit and secret boundaries must be explicit
- G5 Local: smallest relevant local verification must pass or be explained
- G6 Reviewer: third-view review must have no P0/P1 blockers
- G7 Live: live evidence must match its declared evidence layer
- G8 Evidence: run-summary and required reports must exist

## Local Verification
- `tests/run_tests.sh web`

## Live Readonly Verification
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Evidence
- run directory: `doc/02_acceptance/runs/mvp-16-workflow-runner-prepare`
- required: `run-summary.json`
- required: `plan.md`
- required: `local-report.md`
- required: `review-report.md`
- required: `evidence-report.md`

## Close Conditions
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented

## Safety Notes
- This plan does not grant permission for destructive live operations.
- Smoke/regression/acceptance/third-party evidence must remain separate.
- Any policy failure should produce a gate request instead of continuing execution.
