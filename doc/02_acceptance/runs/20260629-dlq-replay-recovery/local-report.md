# 2026-06-29 DLQ fallback replay recovery live report

## Scope

This run closes the controlled fallback-file replay and cross-Pod idempotency part of the DLQ/replay P0 thread.

It uses the existing Kubernetes environment, APISIX/Kafka/PostgreSQL/Redis services, and two live `ingest-gateway` Pods. The run writes one controlled replay message to Kafka topic `dlq.v1` with key `codex-dlq-replay-20260629-dlq-recovery-r5`.

## Code changes

- Fixed `go/control-plane/internal/ingest/dlq/producer.go` so replay Kafka writers do not set a writer-level topic while replay messages set `kafka.Message.Topic`.
- Added `go/control-plane/internal/ingest/dlq/producer_test.go` to lock that writer contract.
- Added `tests/e2e/live_dlq_replay_recovery.sh` for a repeatable live recovery drill:
  - create a temporary `api_tokens` row with `dlq:replay`;
  - seed controlled fallback files into two running `ingest-gateway` Pods;
  - execute non-dry-run replay on Pod A;
  - verify the replayed key/payload from Kafka `dlq.v1`;
  - repeat the same idempotency key on Pod B and require `duplicate:true`;
  - verify Pod A fallback file was consumed, Pod B duplicate file was preserved, and temporary data was cleaned.
- Rolled `traffic/ingest-gateway:dlq-replay-recovery-20260629-r1` into Kubernetes.

## Evidence

| Check | Result | Artifact |
|---|---:|---|
| Go ingest packages and `cmd/ingest-gateway` tests | pass | `go-test-ingest-dlq-recovery-r1.txt` |
| Live script syntax | pass | `live-dlq-replay-script-bash-n.txt` |
| K8s dry-run/apply/rollout | pass | `go-services-dry-run-r1.txt`, `go-services-apply-r1.txt`, `ingest-gateway-rollout-r1.txt` |
| Deployed image readiness | pass, `2/2` | `ingest-gateway-image-ready-r1.txt`, `ingest-gateway-pods-current-r1.txt` |
| Live fallback replay run | pass, 11/11 checks | `live-dlq-replay-recovery-20260629-dlq-recovery-r5-summary.json` |
| Kafka replay verification | pass | `20260629-dlq-recovery-r5-kafka-consumer.log` |
| Cleanup verification | pass | `cleanup-verification-r5.txt` |

## Live result

Run `20260629-dlq-recovery-r5` passed all 11 checks.

- Pod A replay returned HTTP 200 with `duplicate:false`, `replayed_files:1`, `failed_files:0`, and `remaining_fallback_files:0`.
- Kafka lookup found `codex-dlq-replay-20260629-dlq-recovery-r5` with the expected controlled payload.
- Pod B replay with the same idempotency key returned HTTP 200 with `duplicate:true` and the same `replay_id`.
- Pod A's fallback file was removed by replay.
- Pod B's fallback file was preserved during duplicate handling.
- Temporary token count returned to `0`.
- Redis idempotency keys for r1 through r5 returned `0`.
- No `kubectl port-forward` process remained on 18080/18081.
- No `dlq-fallback-codex-*` files remained in either live `ingest-gateway` Pod.

## Discovery notes

Earlier exploratory runs were useful failures:

- r2 proved `requested_by` is required for a non-dry-run request.
- r3 exposed the Kafka writer contract bug: kafka-go rejects setting topic at both writer and message level.
- r4 proved the fixed image replayed successfully, then r5 switched Kafka verification to a post-facto lookup from the beginning to avoid a race with a background consumer.

## Remaining scope

This evidence does not yet close malformed-message injection through the live Kafka/Flink ingest path or a legal-login Desktop Chrome business-page click through the data-quality modal. The failed-sample regression item is covered separately by `../20260629-dlq-replay-failure-regression/`.
