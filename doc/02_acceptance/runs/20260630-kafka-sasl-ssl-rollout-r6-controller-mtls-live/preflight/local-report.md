# Kafka Security Rollout Preflight

Run: `20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live-preflight`

Result: `blocked`

This preflight is non-rolling. It checks whether the live plaintext Kafka cluster has the prerequisites needed for a later SASL_SSL/SCRAM/TLS/ACL maintenance-window rollout. Set `SEED_SCRAM=true` to seed SCRAM users without rolling Kafka.

## Summary

| Metric | Count |
|---|---:|
| Checks | 10 |
| Passed | 7 |
| Blockers | 2 |
| Warnings | 1 |
| Prerequisite blockers | 0 |
| Rollout blockers | 2 |
| Missing Secret/TLS keys | 0 |
| TLS validation blockers | 0 |
| SCRAM client user present | 1 |
| SCRAM broker user present | 1 |
| ACL authorizer disabled | 1 |
| Live Kafka plaintext markers | 1 |
| Live Kafka SASL_SSL markers | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live/preflight/live-kafka-security-rollout-preflight-20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live-preflight-summary.json`
- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live/preflight/live-kafka-security-rollout-preflight-20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live-preflight.ndjson`
- `kafka-security-secret-readiness.json`
- `kafka-tls-material-validation.json`
- `kafka-scram-readiness.json`
- `kafka-acl-live-summary.json`
- `live-kafka-listener-summary.json`

## Interpretation

The repo Kafka profile, client manifests, Secret/TLS material, SCRAM users, live ACL authorizer state, and live listener state are checked separately. A later Kafka rollout remains blocked while live listener markers are plaintext or ACL authorizer is disabled. Prerequisite blockers should be zero before rolling the StatefulSet.
