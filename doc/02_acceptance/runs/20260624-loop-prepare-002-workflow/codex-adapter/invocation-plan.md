# Codex Adapter Invocation: CLE-P0-P95-001

- task: 完整 P95 时间戳链设计与埋点计划
- patch_request: `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/patch-runner/patch-request.md`
- execute: `False`
- command: `none`

## Contract
- External Codex must read the patch request.
- External Codex should produce a unified diff and `codex-output.json` matching patch-runner/codex-output-contract.json.
- The adapter does not trust model output; patch_runner.py must validate it before apply.
