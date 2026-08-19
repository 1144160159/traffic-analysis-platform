# Feature Spec: CLE-P0-SCREEN-001

- run_id: `mvp-8-evidence-repair`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Actors
- authenticated operator
- system auditor
- third-view reviewer
- display-wall viewer with read-only token
- demo viewer with desensitized data

## Capability
- Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data.
- The feature must expose a clear product state for authorized, unauthorized, expired, degraded and empty-data scenarios.
- The frontend must surface loading and error states without fabricating successful business data.

## Functional Requirements
- `/screen` default access path uses the same protected shell/auth decision as other operational pages.
- Read-only token mode may read only scoped screen data and cannot call mutation endpoints.
- Expired, missing or invalid token state must fail closed with a clear non-sensitive state.
- Demo/desensitized mode must be explicit and visually distinguishable from live operations data.
- WebSocket or polling refresh must reuse the same auth, tenant and site boundary as initial queries.

## Acceptance-facing Behavior
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented
