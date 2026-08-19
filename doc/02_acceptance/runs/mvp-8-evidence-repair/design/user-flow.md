# User Flow: CLE-P0-SCREEN-001

- run_id: `mvp-8-evidence-repair`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Primary Flow
- Operator opens `/screen` from an authenticated session.
- Frontend resolves tenant, site and time window.
- Frontend requests screen summary, topology, collection health, alert posture and evidence integrity from real APIs.
- Realtime refresh starts only after the same auth boundary is confirmed.
- Display shows a one-screen command view with no mutation controls.

## Read-only Token Flow
- Admin provisions a scoped display token outside this task's default implementation path.
- Display wall opens `/screen` with token context.
- Backend verifies token scope, expiry, tenant and site.
- Frontend shows read-only data and a visible read-only mode marker.
- Expiry moves the screen to a closed, non-sensitive state.

## Negative Flow
- Missing auth or token is rejected.
- Cross-tenant or cross-site claims are rejected.
- API 401/403/5xx states do not fall back to fake success.

## Design Guardrail
- `/screen` is currently detected outside ProtectedLayout at web/ui/src/App.tsx:75.
