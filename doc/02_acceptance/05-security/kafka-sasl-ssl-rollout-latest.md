# Kafka SASL_SSL Rollout

Run: `20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live`

Result: `pass`

This is the maintenance-window Kafka rollout gate. It is disruptive only when `ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true`.

## Summary

| Metric | Count |
|---|---:|
| Checks | 11 |
| Passed | 11 |
| Blockers | 0 |
| Warnings | 0 |
| Preflight prerequisite blockers | 0 |
| Post-rollout preflight blockers | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live-summary.json`
- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live.ndjson`
- `preflight/`
- `post-preflight/`
- `kafka-post-rollout-topics.txt`
- `kafka-post-rollout-acls.txt`
- `kafka-post-rollout-broker-api.txt`
