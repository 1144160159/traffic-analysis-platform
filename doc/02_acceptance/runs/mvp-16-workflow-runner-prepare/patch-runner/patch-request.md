# Codex Patch Request: CLE-P0-SCREEN-001

- run_id: `mvp-16-workflow-runner-prepare`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- acceptance_type: `regression`

## Inputs
- context_pack: `doc/02_acceptance/runs/mvp-16-workflow-runner-prepare/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/mvp-16-workflow-runner-prepare/design`
- guidance: `doc/02_acceptance/runs/mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001/guidance/guidance.json`

## Allowed Paths
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`

## Close Conditions
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented

## Verification
- `tests/run_tests.sh web`

## Required Codex Output
- Provide a unified diff patch file.
- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.
- Do not edit outside Allowed Paths.
- Do not mark the task closed; evidence_check.py decides closure eligibility.
