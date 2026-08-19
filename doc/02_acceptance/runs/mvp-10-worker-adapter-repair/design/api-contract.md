# API Contract Sketch: CLE-P0-SCREEN-001

- run_id: `mvp-10-worker-adapter-repair`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Contract Impact Declared By Task
- `proto`: `False`
- `kafka_topics`: `False`
- `database_schema`: `False`
- `apisix_routes`: `False`

## API Shape To Confirm Before Implementation
- `GET /api/v1/screen/summary`: returns scoped KPI, collection health and evidence integrity.
- `GET /api/v1/screen/topology`: returns scoped campus/topology view without mutation affordances.
- `GET /api/v1/screen/alerts`: returns scoped high-level alert posture, not raw sensitive details by default.
- `GET /api/v1/auth/me` or equivalent session probe remains the default auth check.
- Read-only token verification is an auth capability, not a frontend-only bypass.

## Negative Contract Cases
- 401 for missing/expired auth.
- 403 for cross-tenant, cross-site or mutation attempts under read-only token.
- 5xx/degraded upstream must be visible as degraded state and cannot silently fabricate live data.
