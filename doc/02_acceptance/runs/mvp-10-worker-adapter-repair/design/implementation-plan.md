# Implementation Plan: CLE-P0-SCREEN-001

- run_id: `mvp-10-worker-adapter-repair`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Phase 0: Review Design Package
- Confirm product behavior, data sensitivity, API boundary and visual source of truth.
- Keep any unresolved blocker in DESIGN_ITERATING; do not start closure work.

## Phase 1: Protect Existing Work
- Run or explicitly account for `CLE-P0-UIBACKUP-001` before broad visual rebuild.

## Phase 2: Implement Narrow Slice
- Change only files inside the task allowed paths unless a new task/design-delta expands scope.
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`
- First slice should settle `/screen` auth/read-only/desensitized behavior before visual polish.
- Keep read-only token server-verified if implemented; do not rely on frontend-only checks.

## Phase 3: Verify
- Run the smallest declared local gate first.
- Add browser/API negative checks when auth or visual behavior changes.
- Record failures as repair input, not as acceptance evidence.

## Phase 4: Review And Evidence
- Run third-view review.
- Update `design-delta.md` if implementation changes product or architecture decisions.
- Keep evidence type honest: design package, smoke, regression, acceptance and third-party are not interchangeable.

## Guidance Status Suggestions
- `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
