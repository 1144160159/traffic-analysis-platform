# Security Evidence

This directory keeps stable security gate evidence for GATE-P0-07 and GATE-P0-10.

Latest Kafka SASL_SSL rollout:

- Run: `20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live`
- Result: `pass`, `11/11` checks passed, `0` blockers, `0` warnings
- Closed rollout evidence: Kafka StatefulSet rolled to `SASL_SSL`/`SCRAM-SHA-512`, ACL authorizer is active, init topic/ACL job completed over secure client config, broker API/topic/ACL checks passed, and post-rollout security preflight blockers are `0`
- Superseded failed rollout evidence: r4 exposed ExternalSecret source truststores with `0` trusted cert entries; r5 exposed KRaft controller principal `ANONYMOUS` and StandardAuthorizer denial. Both are closed by source-secret truststore regeneration plus controller mTLS / broker cert DN super user.

Latest Kafka security rollout preflight:

- Run: `20260630-kafka-security-rollout-preflight-r9-post-sasl-ssl`
- Result: `pass`, `9/10` checks passed, `0` blockers, `1` warning
- Closed evidence: required Kafka Secret/TLS keys exist, TLS material parses with live passwords, SCRAM client and broker users exist, live ACLs are listable, live listener has `SASL_SSL` markers and no plaintext markers, and repo clients/init jobs are configured for `SASL_SSL`
- Remaining warning: SCRAM seeding step is skipped unless `SEED_SCRAM=true`; this is non-blocking after the live users have been verified

Latest production security preflight:

- Run: `20260630-production-security-preflight-r49-waiver-registry`
- Result: `blocked`, `20/21` checks passed, `1` blocker, `0` warnings
- Closed since the previous production-security run: live Kafka TLS/SASL listener profile is enabled with no plaintext markers; External Secrets Operator CRD is established, all `3` ESO controller pods are Ready, all `13/13` production ExternalSecrets are live and Ready, live ExternalSecret reconciliation is `14/14`, and live workload unpinned image count remains `0`
- Remaining blocker: NetworkPolicy-capable CNI count `0` on the current Flannel-only cluster
- Waiver registry: `production-security-waivers.yaml` is a reviewable acceptance artifact for local/dev Kafka plaintext fixtures, the placeholder raw Secret template, and required host-level infrastructure workloads. Latest r49 proves all waiver-backed categories have `0` unwaived findings. It does not waive the NetworkPolicy-capable CNI blocker.

Latest NetworkPolicy enforcement preflight:

- Run: `20260630-network-policy-enforcement-preflight-r1-flannel-blocked`
- Result: `blocked`, `2/4` checks passed, `2` blockers, `0` warnings
- Closed evidence: repo NetworkPolicy profile client dry-run passed, and `20` live NetworkPolicy objects are present
- Remaining blocker: policy-capable CNI pods are `0`; the default-deny and allow-list negative probe is intentionally skipped because running it on Flannel-only networking would be a false pass

Latest NetworkPolicy enforcement readiness package:

- Run: `20260630-network-policy-enforcement-readiness-r1`
- Result: `pass`, `7/9` checks passed, `0` blockers, `2` warnings
- Stable package: `network-policy-readiness/latest/`
- Package contents: CNI migration runbook template, CNI selection review template, rollback checklist, enforcement probe review template, post-CNI preflight command, evidence manifest, and input snapshots for the latest formal preflight, CNI capability summary, live NetworkPolicy inventory and repo `00-network-policies.yaml`
- Boundary: this package does not install a CNI and does not prove NetworkPolicy enforcement; formal closure still requires `ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh` to pass after policy-capable CNI pods exist

Stable artifacts:

- `kafka-sasl-ssl-rollout-latest.json`
- `kafka-sasl-ssl-rollout-latest.md`
- `kafka-security-rollout-preflight-latest.json`
- `kafka-security-rollout-preflight-latest.md`
- `external-secret-operator-canary-latest.json`
- `external-secret-operator-canary-latest.md`
- `production-security-preflight-latest.json`
- `production-security-preflight-latest.md`
- `production-security-waivers.yaml`
- `production-security-waivers-latest.json`
- `network-policy-enforcement-preflight-latest.json`
- `network-policy-enforcement-preflight-latest.md`
- `network-policy-enforcement-probe-latest.json`
- `network-policy-readiness/network-policy-enforcement-readiness-latest.json`
- `network-policy-readiness/network-policy-enforcement-readiness-latest.md`
- `network-policy-readiness/latest/`
- `external-secret-production-reconciliation-latest.json`
- `external-secret-production-reconciliation-latest.md`
- `live-production-secretstores-latest.json`
- `expected-production-externalsecrets-latest.json`
- `live-production-externalsecret-readiness-latest.json`
- `external-secret-production-post-reconcile-key-readiness-latest.json`
- `live-external-secret-reconciliation-summary-latest.json`
- `live-cni-policy-capability-summary-latest.json`
- `live-cni-policy-capability-latest.json`
- `network-policy-live-latest.json`
