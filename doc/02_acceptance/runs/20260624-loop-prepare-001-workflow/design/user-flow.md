# User Flow: CLE-P0-DLQ-001

- run_id: `20260624-loop-prepare-001-workflow`
- task: DLQ replay API、审批、审计、幂等验证
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Storage / Data Quality`
- dependent_lanes: Go Control-plane, Proto / Kafka / Flink, Mission / Acceptance
- acceptance_type: `regression`

## Primary Flow
- User enters the feature through an existing route or operational workflow.
- The UI/API validates auth, tenant and data availability before showing business results.
- The task produces evidence for the declared acceptance layer.

## Negative Flow
- Unauthorized, cross-tenant, empty and degraded states are explicit.
- Failure states cannot be hidden by mock data unless `VITE_USE_MOCK=true` is explicitly active for local development.

## Design Guardrail
- No route-specific signal was selected for this task.
