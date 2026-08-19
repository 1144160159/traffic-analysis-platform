# 2026-06-29 DLQ replay failure-sample regression report

## Scope

This run closes the DLQ replay failed-sample regression part of the DLQ/replay P0 thread.

It uses the live Kubernetes `ingest-gateway`, PostgreSQL token fallback, Redis idempotency store, and the protected `POST /api/v1/dlq/replay/fallback` endpoint. The script seeds a controlled invalid fallback record with an invalid `payload_base64` value, then verifies that replay returns a partial result and keeps the bad file quarantined.

## Code changes

- Fixed `go/control-plane/internal/ingest/dlq/producer.go` so `ReplayFallbackFiles` returns final fallback stats after the replay attempt.
- Added a unit test in `go/control-plane/internal/ingest/dlq/producer_test.go` proving an invalid fallback file is preserved and reported as `remaining_fallback_files=1`.
- Added `tests/e2e/live_dlq_replay_failure_regression.sh` for the live failure regression drill.
- Rolled `traffic/ingest-gateway:dlq-replay-failure-20260629-r2` into Kubernetes.

## Evidence

| Check | Result | Artifact |
|---|---:|---|
| Go ingest packages and `cmd/ingest-gateway` tests | pass | `go-test-ingest-failure-regression-r2.txt` |
| Live script syntax | pass | `live-dlq-replay-failure-script-bash-n-r2.txt` |
| K8s dry-run/apply/rollout | pass | `go-services-dry-run-r2.txt`, `go-services-apply-r2.txt`, `ingest-gateway-rollout-r2.txt` |
| Deployed image readiness | pass, `2/2` | `ingest-gateway-image-ready-r2.txt`, `ingest-gateway-pods-current-r2.txt` |
| Live failure regression | pass, 10/10 checks | `live-dlq-replay-failure-regression-20260629-dlq-failure-r2-summary.json` |
| Cleanup verification | pass | `cleanup-verification-r2.txt` |

## Live result

Run `20260629-dlq-failure-r2` passed all 10 checks.

- The invalid fallback file was seeded into one live `ingest-gateway` Pod.
- Non-dry-run replay returned HTTP 200 with `status=partial`, `duplicate=false`, `replayed_files=0`, `failed_files=1`, `remaining_fallback_files=1`, and an explicit success-rate error.
- The invalid fallback file remained present immediately after the partial replay, proving it stayed quarantined.
- A duplicate request with the same idempotency key returned `duplicate=true` with the same `replay_id` and the same partial result.
- Temporary token, Redis idempotency key, port-forward, and fallback file were cleaned.

## Discovery notes

Run `20260629-dlq-failure-r1` exposed an observability bug: the partial replay response reported `remaining_fallback_files=0` even though the script proved the invalid file was still present. The root cause was `ReplayFallbackFiles` updating fallback stats in a `defer` while returning an unnamed value. Run `r2` verifies the fix.

## Remaining scope

This evidence does not yet close malformed-message injection through the live Kafka/Flink ingest path or a legal-login Desktop Chrome business-page click through the data-quality modal.
