# Codex Loop Sandbox Plan

- run_id: `mvp-21-daemon-quota-sandbox-plan-i1-worker-cle-p0-screen-001-sandbox`
- status: `SANDBOX_PLAN_READY`
- driver: `local-container`
- stage: `prepare`
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
- none

## Local Container Command

```bash
docker run --read-only --rm --network none --workdir /workspace --cap-drop ALL --security-opt no-new-privileges --cpus 1 --memory 1Gi -e PYTHONUNBUFFERED=1 -e CODEX_LOOP_SANDBOX=1 -v /home/wangwt/phase_2/code/traffic-analysis-platform:/workspace --tmpfs /tmp:rw,noexec,nosuid,size=256m traffic-analysis/codex-loop:local python -B scripts/codex_loop/workflow.py --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml --run-id mvp-21-daemon-quota-sandbox-plan-i1-worker-cle-p0-screen-001-sandbox-workflow --stage prepare
```

## Guardrail
- This script only renders isolation plans and optional dry-run validation; it does not run containers or apply Kubernetes resources.
- Default policy denies live write, external Codex execution, service account token automount and network egress.
