# Acceptance Cases: CLE-P0-SCREEN-001

- run_id: `mvp-8-evidence-repair`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Evidence Layer
- declared_acceptance_type: `regression`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- /screen has exactly one public/protected/readonly strategy
- unauthorized behavior is verified
- sensitive data display policy is documented

## Local Verification Candidates
- `tests/run_tests.sh web`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`

## Required Negative Cases
- missing session/token
- expired read-only token
- cross-tenant or cross-site token claim
- mutation attempt under read-only token
- API degraded state without fake success data
- browser smoke: no 4xx/5xx except expected auth negatives, no requestfailed, no non-warning console/pageerror
