# Learning Episode Evaluation

- run_id: `20260712-campaign-learning-baseline`
- task_id: `UI-RL-P0-CAMPAIGN-001`
- subject: `page:campaigns`
- hard_gates_passed: `False`
- reward_eligible: `False`
- reward_total: `None`
- learning_status: `negative`
- main_thread_decision: `repair`

## Hard Gates
- `business_semantics`: `pass`
- `functional_realization`: `blocked`
- `tenant_rbac_audit`: `blocked`
- `database_and_seed`: `pass`
- `runtime_clean`: `blocked`
- `business_roi`: `pass`
- `required_tests`: `blocked`
- `security_and_secrets`: `blocked`
- `rollout_and_rollback`: `fail`

## Reward Dimensions
- `business`: `70`
- `function`: `45`
- `quality`: `55`
- `visual`: `85`
- `performance`: `40`
- `maintainability`: `65`

## Guardrail
- A reward is computed only after every frozen hard gate passes.
- This report does not deploy, mutate task status, or authorize production changes.
