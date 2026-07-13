# Signoff Checklist Review Template

Run ID: `20260702-third-party-signoff-readiness-r26-ui-r40-receiver-selftest`

This checklist is generated from `doc/02_acceptance/08-third-party` and current latest evidence pointers. It is a review aid only.

## Required Reviews

| Item | Required action | Evidence |
|---|---|---|
| Deployment proof | Fill site, period, topology, continuous-run proof and deployment result | `pilot-deployment-proof.md` |
| Demo walkthrough | Execute against real APISIX/API/UI evidence or archived package | `demo-script.md` |
| Weekly report | Fill pilot week metrics and cases | `pilot-weekly-report-template.md` |
| Economic benefit | Replace TBD inputs with user-confirmed numbers | `economic-benefit.md` |
| User signoff | Fill functional acceptance, exceptions and signatures | `user-acceptance-signoff.md` |
| Third-party quality | Attach signed blind-test or CNAS-equivalent report | `04-detection-quality/` |
| Performance | Attach signed hardware-window 10x100G/512Mpps summaries | `03-performance/` |
| Security and HA | Attach production-security and RTO/RPO pass evidence or signed exceptions | `05-security/`, `06-resilience/` |

## Current Evidence Inputs

| Key | Result | Blockers | Path |
|---|---:|---:|---|
| baseline | pass | 0 | `doc/02_acceptance/00-baseline/release-manifest-latest.json` |
| deployment | pass | 0 | `doc/02_acceptance/07-deployment/deployment-preflight-latest.json` |
| ui_contract | blocked | 1 | `doc/02_acceptance/02-regression/ui-contract-preflight-latest.json` |
| ui_visual_interaction | blocked | 3 | `doc/02_acceptance/02-regression/ui-visual-interaction-preflight-latest.json` |
| ui_visual_evidence_finalization | blocked | 54 | `doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-latest.json` |
| business_flow | pass | 0 | `doc/02_acceptance/02-regression/business-flow-api-preflight-latest.json` |
| oidc_sso | pass | 0 | `doc/02_acceptance/02-regression/oidc-sso/oidc-sso-preflight-latest.json` |
| security | blocked | 1 | `doc/02_acceptance/05-security/production-security-preflight-latest.json` |
| ha | blocked | 1 | `doc/02_acceptance/06-resilience/ha-readiness-preflight-latest.json` |
| performance | blocked | 4 | `doc/02_acceptance/03-performance/capture-performance-preflight-latest.json` |
| detection_quality | blocked | 5 | `doc/02_acceptance/04-detection-quality/detection-quality-preflight-latest.json` |
| asset_discovery_coverage | blocked | 1 | `doc/02_acceptance/02-regression/asset-discovery-coverage-latest.json` |
| completion_snapshot | blocked | 9 | `doc/02_acceptance/09-completion/project-completion-audit-latest.json` |

## Signoff Rule

Do not remove `TBD` placeholders or mark the formal completion gate complete until the user or third-party reviewer fills the final documents and signs the exceptions.
