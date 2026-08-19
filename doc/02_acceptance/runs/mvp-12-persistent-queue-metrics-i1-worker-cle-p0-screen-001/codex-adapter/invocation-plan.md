# Codex Adapter Invocation: CLE-P0-SCREEN-001

- task: /screen 只读 token 或脱敏公开边界
- patch_request: `doc/02_acceptance/runs/mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001/patch-runner/patch-request.md`
- execute: `False`
- command: `none`

## Contract
- External Codex must read the patch request.
- External Codex should produce a unified diff and `codex-output.json` matching patch-runner/codex-output-contract.json.
- The adapter does not trust model output; patch_runner.py must validate it before apply.
