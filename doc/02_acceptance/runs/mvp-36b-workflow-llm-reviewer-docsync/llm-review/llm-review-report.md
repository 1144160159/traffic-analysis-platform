# LLM Review: CLE-P0-REVIEWER-001

- status: `LLM_REVIEW_PLANNED`
- selected_profile: `gpt5_high_risk_llm_review`
- model: `gpt-5`
- sandbox: `read-only`
- timeout_seconds: `2400`
- decision: `none`
- review_request: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/llm-review/llm-review-request.md`
- output_schema: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/llm-review/llm-review-schema.json`
- llm_output: `none`
- findings: `0`

## Selection Reasons
- priority P0 requires high-risk LLM reviewer

## Command Template

```bash
codex exec --model gpt-5 --sandbox read-only --output-schema doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/llm-review/llm-review-schema.json 'Review the high-risk Codex Loop reviewer request at {prompt}. Return only JSON matching the provided schema. Focus on security, contracts, evidence, rollback, and product correctness.'
```

## Findings
- none

## Guardrail
- Planned LLM review evidence does not close a task.
- Actual LLM output is advisory unless `evidence_check.py` and the required reviewer gates also pass.
- Non-pass LLM decisions must become repair, design iteration, or human gate evidence.
