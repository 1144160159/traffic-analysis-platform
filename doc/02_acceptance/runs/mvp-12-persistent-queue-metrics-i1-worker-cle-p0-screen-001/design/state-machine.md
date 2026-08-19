# State Machine: CLE-P0-SCREEN-001

- run_id: `mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## States
- `UNINITIALIZED`: route has mounted, no auth decision yet
- `AUTH_CHECKING`: session or read-only token is being verified
- `AUTHORIZED_LIVE`: authenticated operator can view live screen data
- `AUTHORIZED_READONLY`: scoped display token can view scoped/desensitized read-only data
- `DEMO_DESENSITIZED`: explicit demo mode uses non-sensitive fixture or generated-safe data
- `DENIED`: missing/invalid/expired/cross-tenant access
- `DEGRADED`: auth ok, but one or more upstream data APIs are unavailable

## Required Transitions
- Any blocker from guidance keeps the task in `DESIGN_ITERATING` or equivalent; it cannot close.
- Any auth, tenant, contract or evidence-layer change requires reviewer confirmation before closure.
- Any failed verification moves the task to repair/planning state, not to closed.
