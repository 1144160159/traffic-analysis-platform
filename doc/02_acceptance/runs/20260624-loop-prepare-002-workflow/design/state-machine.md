# State Machine: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

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
