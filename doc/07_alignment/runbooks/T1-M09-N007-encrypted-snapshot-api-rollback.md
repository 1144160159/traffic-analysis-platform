# T1-M09-N007 encrypted snapshot API rollback

## Scope

This change adds `GET /api/v1/encrypted-traffic/snapshot` behind
`ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED=false`. The handler reads ClickHouse
only, emits signed tenant-bound continuations, keeps PCAP object references
field-authorized, and preserves all six legacy encrypted-traffic reads. It
does not add a writer, migration, Kafka consumer, object mutation or rollout.

## Rollback trigger

Rollback the candidate if tenant isolation, field permissions, signed
continuations, explicit availability states, the 2-second server budget, the
4 MiB response budget, or the rule limitation that encryption alone is never
a malicious verdict regresses.

## Procedure

1. Set `ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED=false` and stop rollout
   expansion. This is the primary rollback and leaves the registered route
   fail-closed with `503 FEATURE_DISABLED`.
2. If binary rollback is required, restore the previously approved
   `alert-service` digest. Do not change APISIX routes for the six legacy GET
   endpoints.
3. Verify `/api/v1/encrypted-traffic/stats`, `/sessions`, `/ja3`, `/tunnels`,
   `/exfiltration` and `/evidence` through APISIX for one authenticated tenant.
4. Record the failed candidate digest, request window, snapshot ID, trace ID,
   source watermarks, missing sections and latency before closing the attempt.

## Data safety

- Do not alter or delete `traffic.sessions`, `traffic.feature_fp` or
  `traffic.pcap_index_v2` during rollback.
- Do not apply the additive M03 ClickHouse migration merely to make this API
  appear healthy. The reader detects legacy schemas and reports missing
  persisted fields as partial facts.
- Do not expose PCAP file keys to callers without `pcap:read`, and do not turn
  references into download URLs without `pcap:download`.
- No data rollback is required because N007 is read-only.

## Forward recovery

Rebuild the candidate test binary, run the isolated K8s contract Job with the
shared ClickHouse connection in read-only mode, and retain the resulting image
ID and test-binary SHA-256. Production enablement remains a separate canary and
observation decision; a passing Job does not enable the flag.
