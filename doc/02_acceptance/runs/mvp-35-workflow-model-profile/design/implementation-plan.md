# Implementation Plan: CLE-P0-REVIEWER-001

- run_id: `mvp-35-workflow-model-profile`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Phase 0: Review Design Package
- Confirm product behavior, data sensitivity, API boundary and visual source of truth.
- Keep any unresolved blocker in DESIGN_ITERATING; do not start closure work.

## Phase 1: Protect Existing Work
- Capture git status and task snapshot before implementation.

## Phase 2: Implement Narrow Slice
- Change only files inside the task allowed paths unless a new task/design-delta expands scope.
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Phase 3: Verify
- Run the smallest declared local gate first.
- Add browser/API negative checks when auth or visual behavior changes.
- Record failures as repair input, not as acceptance evidence.

## Phase 4: Review And Evidence
- Run third-view review.
- Update `design-delta.md` if implementation changes product or architecture decisions.
- Keep evidence type honest: design package, smoke, regression, acceptance and third-party are not interchangeable.

## Guidance Status Suggestions
- none
