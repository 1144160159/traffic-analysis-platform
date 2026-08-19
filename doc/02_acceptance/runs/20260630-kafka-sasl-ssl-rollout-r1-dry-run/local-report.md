# Kafka SASL_SSL Rollout

Run: `20260630-kafka-sasl-ssl-rollout-r1-dry-run`

Result: `blocked`

This is the maintenance-window Kafka rollout gate. It is disruptive only when `ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true`.

## Summary

| Metric | Count |
|---|---:|
| Checks | 3 |
| Passed | 2 |
| Blockers | 1 |
| Warnings | 0 |
| Preflight prerequisite blockers | 0 |
| Post-rollout preflight blockers | null |

## Key Artifacts

- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r1-dry-run/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r1-dry-run-summary.json`
- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r1-dry-run/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r1-dry-run.ndjson`
- `preflight/`
- `post-preflight/`
- `kafka-post-rollout-topics.txt`
- `kafka-post-rollout-acls.txt`
- `kafka-post-rollout-broker-api.txt`
