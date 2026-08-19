# Codex Adapter Invocation: CLE-P0-REVIEWER-001

- task: 开启第三视角 Reviewer Gate
- patch_request: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/patch-runner/patch-request.md`
- execute: `False`
- command: `none`

## Contract
- External Codex must read the patch request.
- External Codex should produce a unified diff and `codex-output.json` matching patch-runner/codex-output-contract.json.
- The adapter does not trust model output; patch_runner.py must validate it before apply.
