# Model Profile: CLE-P0-REVIEWER-001

- status: `MODEL_PROFILE_SELECTED`
- selected_profile: `gpt5_high_risk_patch`
- model: `gpt-5`
- sandbox: `read-only`
- timeout_seconds: `2400`
- patch_request: `doc/02_acceptance/runs/mvp-35-model-profile-smoke/patch-runner/patch-request.md`
- output_schema: `doc/02_acceptance/runs/mvp-35-model-profile-smoke/patch-runner/codex-output-schema.json`
- codex_policy: `scripts/codex_loop/policies/codex-execution.yaml`
- findings: `0`

## Selection Reasons
- priority P0 requires high-risk profile

## Command Template

```bash
codex exec --model gpt-5 --sandbox read-only --output-schema doc/02_acceptance/runs/mvp-35-model-profile-smoke/patch-runner/codex-output-schema.json 'Read and follow the high-risk Codex patch request at {prompt}. Return only output matching the provided schema. Highlight contract, security, evidence, and rollback risks.'
```

## Findings
- none

## Guardrail
- This profile only selects a model command template; it does not call Codex.
- `codex_runner.py` must still validate policy, environment gate, redaction and execution status.
- Generated patches must still return through `patch_runner.py`, `review.py` and `evidence_check.py`.
