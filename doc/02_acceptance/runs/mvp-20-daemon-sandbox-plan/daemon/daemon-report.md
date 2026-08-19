# Codex Loop Daemon Report

- run_id: `mvp-20-daemon-sandbox-plan`
- status: `DAEMON_COMPLETED`
- iterations: `1`
- worker_runner: `sandbox-plan`
- worker_stage: `prepare`

## Iterations
- iteration `1` status `ITERATION_COMPLETED`
  - `scout.py` exit `0`
  - `guide.py` exit `0`
  - `scheduler.py` exit `0`
  - `sandbox_worker.py` exit `0`
  - `metrics.py` exit `0`

## Guardrail
- Daemon cycles are bounded by --iterations.
- Default worker runner is the local workflow worker and default stage is prepare.
- Sandbox execution requires --worker-runner sandbox-execute plus sandbox_executor.py environment gates.
