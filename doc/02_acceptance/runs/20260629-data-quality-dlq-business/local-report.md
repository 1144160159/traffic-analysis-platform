# 2026-06-29 Data Quality DLQ business-page dry-run report

## Scope

This run closes the Data Quality business-page dry-run path for a legal user session: a signed access JWT enters `/data-quality?tab=replay-reconcile`, opens the DLQ fallback replay modal, submits the default dry-run form, and receives HTTP 200 from the live APISIX route `/api/v1/dlq/replay/fallback`.

## Code changes

- Added `go/control-plane/internal/ingest/auth/replay_token_validator.go` so the DLQ replay HTTP endpoint accepts both existing `api_tokens` and signed user access JWTs.
- Wired the replay validator into `go/control-plane/cmd/ingest-gateway/main.go`.
- Added `JWT_SIGNING_KEY` to the live ingest-gateway Deployment and rolled image `traffic/ingest-gateway:dq-dlq-jwt-20260629-r1`.
- Added `tests/e2e/live_data_quality_dlq_business.sh` for repeatable live UI/API evidence.

## Evidence

| Check | Result | Artifact |
|---|---:|---|
| Go ingest auth/DLQ/gateway tests | pass | `go-test-dq-dlq-jwt-r1.txt` |
| Web UI targeted Vitest | pass, 13/13 | `web-ui-vitest-dq-dlq-business.txt` |
| Live script syntax | pass | `live-data-quality-dlq-business-bash-n.txt` |
| Image build/import/rollout | pass | `ingest-gateway-docker-build-r1.txt`, `ingest-gateway-import-local-r1.txt`, `ingest-gateway-import-10.0.5.9-r1.txt`, `ingest-gateway-rollout-r1.txt` |
| Live Pod readiness | pass, 2/2 | `ingest-gateway-image-ready-r1.txt`, `ingest-gateway-jwt-env-r1.txt` |
| Live business-page dry-run | pass, 4/4 | `live-data-quality-dlq-business-20260629-dq-dlq-business-r2-summary.json` |
| Desktop Chrome login gate | pass | `desktop-chrome-data-quality-login-gate-r2.json` |

## Live result

Run `20260629-dq-dlq-business-r2` passed all 4 checks:

- `/api/v1/auth/me` accepted the signed user access JWT and exposed `dlq:replay`.
- `/api/v1/data-quality` returned 200 through APISIX.
- Playwright rendered `/data-quality?tab=replay-reconcile` with `DLQ 重放队列` and `DLQ Replay API 契约`.
- The modal submitted `POST /api/v1/dlq/replay/fallback` and received status 200 with `status: dry_run`, `duplicate: false`, `requested_by: codex-data-quality-operator`, no console errors, no page errors, no request failures, and no HTTP 4xx/5xx.

No fallback file replay was executed. The run created only a dry-run idempotency record with a 24h Redis TTL and `pre_fallback_files: 0`.

## Desktop Chrome note

The Codex Desktop Chrome extension wrapper can open the live login page and confirms that an anonymous direct open of `/data-quality?tab=replay-reconcile` correctly redirects to `/login`. `agent.md` forbids raw `browser-client`/tab scripting and login-form submission, so this run does not claim a Desktop Chrome logged-in click. The business-page click closure is covered by the legal JWT Playwright session above.
