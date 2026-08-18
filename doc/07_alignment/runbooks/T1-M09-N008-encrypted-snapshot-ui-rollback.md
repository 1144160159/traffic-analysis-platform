# T1-M09-N008 encrypted snapshot UI rollback

## Scope

This change makes `EncryptedTrafficPage` issue a typed, real request to
`GET /v1/encrypted-traffic/snapshot` and renders the five versioned snapshot
sections. It preserves the existing page tabs, six compatibility reads and all
existing action entrypoints. Mock data is not used for the new snapshot path.

## Rollback trigger

Rollback the UI candidate if the page hides partial, unavailable, forbidden,
no-sample or not-computable states; collapses measured zero into missing data;
hides source/rule/model versions or limitations; exposes a denied PCAP field;
or prevents the six legacy reads from rendering while the snapshot flag is
off.

## Procedure

1. Stop UI rollout expansion and retain the current approved web image digest.
2. Restore the previous `web-ui` digest, or remove only the typed snapshot
   query and `EncryptedSnapshotContractPanel` while retaining the existing
   compatibility adapter, tabs and action handlers.
3. Keep `ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED=false` on the API unless its
   separate server rollout is explicitly approved.
4. Verify the encrypted-traffic page with runtime `USE_MOCK=false`, then verify
   the legacy statistics, sessions, JA3, tunnel, exfiltration and evidence
   requests still reach APISIX.

## Data safety

- UI rollback has no database, broker or object-store action.
- Do not fabricate values to fill a missing snapshot section.
- Do not convert a tunnel, exfiltration or encrypted-transport indicator into
  a malicious verdict during fallback.
- Do not retain denied raw-reference values in client state or navigation URLs.

## Forward recovery

Re-run the UI unit tests and production build, create an immutable web image,
and validate its real endpoint string, availability states, limitation copy,
drilldown copy and `USE_MOCK=false` default in a run-scoped K8s Job. Browser
journey evidence remains a later M09-N023 gate and is not implied by this
bundle check.
