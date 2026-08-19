# Codex Adapter Invocation: CLE-P0-DLQ-001

- task: DLQ replay API、审批、审计、幂等验证
- patch_request: `doc/02_acceptance/runs/20260624-loop-prepare-001-workflow/patch-runner/patch-request.md`
- execute: `False`
- command: `none`

## Contract
- External Codex must read the patch request.
- External Codex should produce a unified diff and `codex-output.json` matching patch-runner/codex-output-contract.json.
- The adapter does not trust model output; patch_runner.py must validate it before apply.
