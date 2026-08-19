# Kafka SASL_SSL Rollout

Run: `20260630-kafka-sasl-ssl-rollout-r5-keytool-truststore-live`

Result: `blocked`

This is the maintenance-window Kafka rollout gate. It is disruptive only when `ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true`.

## Summary

| Metric | Count |
|---|---:|
| Checks | 7 |
| Passed | 5 |
| Blockers | 1 |
| Warnings | 1 |
| Preflight prerequisite blockers | 0 |
| Post-rollout preflight blockers | null |

## Key Artifacts

- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r5-keytool-truststore-live/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r5-keytool-truststore-live-summary.json`
- `doc/02_acceptance/runs/20260630-kafka-sasl-ssl-rollout-r5-keytool-truststore-live/live-kafka-sasl-ssl-rollout-20260630-kafka-sasl-ssl-rollout-r5-keytool-truststore-live.ndjson`
- `preflight/`
- `post-preflight/`
- `kafka-post-rollout-topics.txt`
- `kafka-post-rollout-acls.txt`
- `kafka-post-rollout-broker-api.txt`
