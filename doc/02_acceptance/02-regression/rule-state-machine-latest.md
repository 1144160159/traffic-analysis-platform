# Rule state-machine live preflight

- Run ID: `20260629-rule-state-machine-r5-model-version-state`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Enable rule: `4ab44dc7-d39d-4f99-8284-e60c4c20d698`
- Disable rule: `a6a9873a-bb28-4a00-86bb-6b4c742a6ecd`
- Checks: 15/15 passed, blockers=0, warnings=0

## Evidence

- NDJSON: `doc/02_acceptance/runs/20260629-rule-state-machine/live-rule-state-machine-20260629-rule-state-machine-r5-model-version-state.ndjson`
- Summary: `doc/02_acceptance/runs/20260629-rule-state-machine/live-rule-state-machine-20260629-rule-state-machine-r5-model-version-state-summary.json`
- API/DB/Audit responses: `doc/02_acceptance/runs/20260629-rule-state-machine/20260629-rule-state-machine-r5-model-version-state-*.json`, `doc/02_acceptance/runs/20260629-rule-state-machine/20260629-rule-state-machine-r5-model-version-state-*.txt`

## Scope

This report validates the rule enable/disable state machine: a disabled rule can be enabled with `rule:enable`, an active rule can be disabled with `rule:enable`, both actions increment `rules.version`, create `rule_versions`, write `rule_outbox`, and persist queryable `RULE_ENABLE` / `RULE_DISABLE` audit rows. Cross-tenant and read-only requests are rejected and leave rule state unchanged.
