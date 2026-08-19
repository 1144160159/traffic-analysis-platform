# State Machine: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## States
- `DISCOVERED`: task exists but no implementation plan is approved
- `DESIGN_ITERATING`: product, visual or technical design still has open decisions
- `DESIGN_READY`: design package is reviewable and implementation can be planned
- `LOCAL_VERIFIED`: smallest relevant local gate passed
- `REVIEW_REQUIRED`: third-view review still pending
- `CLOSED`: all close_when and evidence files are satisfied

## Required Transitions
- Any blocker from guidance keeps the task in `DESIGN_ITERATING` or equivalent; it cannot close.
- Any auth, tenant, contract or evidence-layer change requires reviewer confirmation before closure.
- Any failed verification moves the task to repair/planning state, not to closed.
