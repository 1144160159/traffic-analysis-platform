# Codex Loop Sandbox Plan

- run_id: `mvp-18-sandbox-execute-local-blocked`
- status: `SANDBOX_PLAN_BLOCKED`
- driver: `kubernetes-job`
- stage: `execute-local`
- task: `CLE-P0-SCREEN-001`
- image: `traffic-analysis/codex-loop:local`
- network_allowed: `False`

## Outputs
- `sandbox/sandbox-plan.json`
- `sandbox/sandbox-report.md`
- `sandbox/codex-loop-sandbox-job.yaml`
- `sandbox/codex-loop-sandbox-networkpolicy.yaml`
- `sandbox/local-container-command.txt`

## Findings
- `blocker` `SANDBOX_STAGE_NOT_ALLOWED`: Stage `execute-local` is not allowed by the sandbox policy.

## Local Container Command

```bash
docker run --read-only --rm --network none --workdir /workspace --cap-drop ALL --security-opt no-new-privileges --cpus 1 --memory 1Gi -e PYTHONUNBUFFERED=1 -e CODEX_LOOP_SANDBOX=1 -v /home/wangwt/phase_2/code/traffic-analysis-platform:/workspace --tmpfs /tmp:rw,noexec,nosuid,size=256m traffic-analysis/codex-loop:local python -B scripts/codex_loop/workflow.py --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml --run-id mvp-18-sandbox-execute-local-blocked-workflow --stage execute-local
```

## Guardrail
- This script only renders isolation plans and optional dry-run validation; it does not run containers or apply Kubernetes resources.
- Default policy denies live write, external Codex execution, service account token automount and network egress.
