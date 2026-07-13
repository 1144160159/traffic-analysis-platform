# Model Version State Machine Live Regression

- Run ID: `20260629-model-version-state-machine-r1`
- Result: `pass`
- Passed: `20/20`
- Blockers: `0`
- Tenant: `codex-model-state`
- Model: `2c65ca3e-95bb-4856-944c-8c1f45efb383`
- NDJSON: `doc/02_acceptance/runs/20260629-model-version-state-machine/live-model-version-state-machine-20260629-model-version-state-machine-r1.ndjson`
- Summary: `doc/02_acceptance/runs/20260629-model-version-state-machine/live-model-version-state-machine-20260629-model-version-state-machine-r1-summary.json`
- API/DB/Audit responses: `doc/02_acceptance/runs/20260629-model-version-state-machine/20260629-model-version-state-machine-r1-*.json`, `doc/02_acceptance/runs/20260629-model-version-state-machine/20260629-model-version-state-machine-r1-*.txt`

This report validates the model registry state machine: model versions register into `registered`, registered versions can activate, activation deprecates any previous active version for the same model, active versions can deprecate, registered deprecate returns 409, cross-tenant and read-only requests return 403, and `MODEL_VERSION_CREATE` / `MODEL_VERSION_ACTIVATE` / `MODEL_VERSION_DEPRECATE` plus failure audit rows are persisted in `audit_logs`.
