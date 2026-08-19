# User Flow: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Primary Flow
- User enters the feature through an existing route or operational workflow.
- The UI/API validates auth, tenant and data availability before showing business results.
- The task produces evidence for the declared acceptance layer.

## Negative Flow
- Unauthorized, cross-tenant, empty and degraded states are explicit.
- Failure states cannot be hidden by mock data unless `VITE_USE_MOCK=true` is explicitly active for local development.

## Design Guardrail
- No route-specific signal was selected for this task.
