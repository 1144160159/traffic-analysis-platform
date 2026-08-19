# Learning Episode Evaluation

- run_id: `20260712-campaign-r298-r210`
- task_id: `UI-RL-P0-CAMPAIGN-001`
- subject: `page:campaigns`
- hard_gates_passed: `False`
- reward_eligible: `False`
- reward_total: `None`
- learning_status: `negative`
- main_thread_decision: `repair`

## Hard Gates
- `business_semantics`: `blocked`
- `functional_realization`: `blocked`
- `tenant_rbac_audit`: `pass`
- `database_and_seed`: `pass`
- `runtime_clean`: `fail`
- `business_roi`: `pass`
- `required_tests`: `fail`
- `security_and_secrets`: `fail`
- `rollout_and_rollback`: `fail`

## Reward Dimensions
- `business`: `72`
- `function`: `76`
- `quality`: `70`
- `visual`: `92`
- `performance`: `42`
- `maintainability`: `82`

## Guardrail
- A reward is computed only after every frozen hard gate passes.
- This report does not deploy, mutate task status, or authorize production changes.
