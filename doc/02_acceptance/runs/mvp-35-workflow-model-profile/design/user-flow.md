# User Flow: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
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
