# Codex Loop Workspace Isolation

- run_id: `mvp-32-executor-pool-stress-local-clone-i002`
- status: `WORKSPACE_ISOLATION_READY`
- mode: `local-clone-create`
- workspace_backend: `local-clone`
- workspace_root: `doc/02_acceptance/runs/.loop/worktrees`
- workspaces: `1`
- source_dirty: `True`

## Workspaces
- `CLE-P0-SCREEN-001` -> `doc/02_acceptance/runs/.loop/worktrees/mvp-32-executor-pool-stress-local-clone-i002/cle-p0-screen-001` backend `local-clone` base `e3316aec4ac1d6592e28aefc86853128ecde7408`

## Findings
- `warning` `SOURCE_WORKTREE_DIRTY`: Source worktree is dirty; generated worktrees are based on HEAD and will not include uncommitted changes.

## Guardrail
- Default mode only plans per-task workspaces; it does not create or delete workspaces.
- Workspace creation requires the explicit CODEX_LOOP_ALLOW_WORKTREE_CREATE gate.
- git-worktree writes source .git metadata; local-clone writes only under the workspace root.
- Workspaces are based on HEAD and do not include uncommitted source changes.
