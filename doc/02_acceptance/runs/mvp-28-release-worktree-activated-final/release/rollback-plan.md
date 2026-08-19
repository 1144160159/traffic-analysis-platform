# Codex Loop Rollback Plan

- run_id: `mvp-28-release-worktree-activated-final`
- commit: `e3316aec4ac1d6592e28aefc86853128ecde7408`
- status: `RELEASE_FROZEN`

## Immediate Stop

```bash
python -B scripts/codex_loop/service.py stop
python -B scripts/codex_loop/lock_manager.py release --force
python -B scripts/codex_loop/service.py health --run-id rollback-health
```

## Revert Scope

The release manifest records the current diff and untracked loop evidence. Revert only loop-engine files after review:

```bash
git status --short -- scripts/codex_loop doc/01_design/自动开发Loop引擎设计.md doc/README.md doc/02_acceptance/runs/.loop
```

## Guardrail
- Do not run destructive git reset or checkout without explicit human approval.
- Do not delete acceptance evidence until a replacement manifest exists.
