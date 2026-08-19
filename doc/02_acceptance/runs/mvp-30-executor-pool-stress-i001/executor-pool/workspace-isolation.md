# Codex Loop Workspace Isolation

- run_id: `mvp-30-executor-pool-stress-i001`
- status: `WORKSPACE_ISOLATION_BLOCKED`
- mode: `worktree-create`
- workspace_root: `doc/02_acceptance/runs/.loop/worktrees`
- workspaces: `1`
- source_dirty: `True`

## Workspaces
- `CLE-P0-SCREEN-001` -> `doc/02_acceptance/runs/.loop/worktrees/mvp-30-executor-pool-stress-i001/cle-p0-screen-001` base `e3316aec4ac1d6592e28aefc86853128ecde7408`

## Findings
- `warning` `SOURCE_WORKTREE_DIRTY`: Source worktree is dirty; generated worktrees are based on HEAD and will not include uncommitted changes.

## Guardrail
- Default mode only plans per-task worktrees; it does not create or delete git worktrees.
- Worktree creation requires the explicit CODEX_LOOP_ALLOW_WORKTREE_CREATE gate.
- Worktrees are based on HEAD and do not include uncommitted source changes.
