# Product Iteration: CLE-P0-SCREEN-001

- run_id: `mvp-10-worker-cle-p0-screen-001`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Product Decision
- Resolve blocker-class design choices before implementation or closure.
- recommended_strategy: Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data.

## Product Value
- Turn the current gap into a visible iteration with owner-readable intent, scope and acceptance boundaries.
- Make the feature safer to discuss: product behavior, data sensitivity, visual target and verification are separated.
- Preserve the option to stop after design review if the next step would widen security, data or deployment scope.

## Source Signals
- doc/05_status/代码实证状态核对-2026-06-19.md#5
- doc/03_review/专家深评整改清单.md#SUB-FS-01

## Guidance Signals
- `warning` `HIGH_RISK_LOCAL`: High-risk task is allowed to enter local mode. Suggestion: Keep security and reviewer gates mandatory; consider planning before implementation.
- `blocker` `SCREEN_AUTH_BOUNDARY`: /screen is outside ProtectedLayout. Suggestion: Resolve the /screen public/protected/readonly strategy before claiming UI auth boundary closure.

## Scheduling Signal
- score `1515` in guidance ranking; mode `local`; lane `UI Rebuild`.

## Product Non-goals
- This package does not mark the feature Done, Acceptance Ready or Third-party Passed.
- This package does not authorize live writes or destructive infrastructure changes.
- This package does not replace PRD/SDD updates when the implementation changes user-facing behavior.
