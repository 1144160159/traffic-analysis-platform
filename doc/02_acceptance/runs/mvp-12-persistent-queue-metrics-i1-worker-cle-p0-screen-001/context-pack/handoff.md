# Handoff: CLE-P0-SCREEN-001

- Current task: /screen 只读 token 或脱敏公开边界
- Current context pack: `doc/02_acceptance/runs/mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001/context-pack/task-context-pack.md`
- Status to assume: `DISCOVERED` with `1` blocker(s) from guidance.

## Resume Steps
- Read this handoff and `task-context-pack.md` first.
- Refresh `git status --short` before any edit.
- If implementing, open exact source refs before relying on summarized text.
- Preserve the declared evidence layer and close_when conditions.

## Current Stop Conditions
- `SCREEN_AUTH_BOUNDARY`: /screen is outside ProtectedLayout.

## Next Likely Action
- Continue from design strategy: Keep `/screen` protected by default; allow display-wall usage only through an explicit read-only token with scoped tenant/site/time-window claims, expiry, audit, and desensitized fallback data.
