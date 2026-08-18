# T1-M10-N006 APISIX route materialization and rollback

The repository candidate contains 58 explicit standalone APISIX routes. Every route has `request-id` with `X-Request-ID`; every upstream has an explicit timeout and `retries=0`; all 52 protected routes have bearer-only Keycloak OIDC, a positive route-specific body limit, request header validation, and rate limiting. The OIDC client secret is referenced only as `$ENV://APISIX_OIDC_CLIENT_SECRET`. The APISIX StatefulSet reads it from `gateway/traffic-credentials` and mounts `gateway/traffic-platform-ca/ca.crt` for verified Keycloak TLS.

The candidate remains default-off and is not applied. `capture_m10_apisix_route_diff.py` performs read-only Kubernetes queries and records route IDs/hashes, StatefulSet policy, Secret key names, and the exact rollback baseline without reading Secret values.

## Apply gate

Do not patch the ConfigMap or StatefulSet until N005 is authorized for the same candidate, `gateway/traffic-credentials` contains `OIDC_CLIENT_SECRET`, `gateway/traffic-platform-ca` contains `ca.crt`, and the site preflight passes. Apply first to an internal canary partition. Verify anonymous 401, valid bearer success, wrong issuer/audience failure, body limit rejection, 429 behavior, upstream timeout, WebSocket continuity, `X-Request-ID` propagation, and service audit correlation.

## Rollback

The read-only evidence stores the previous live route content hash, ConfigMap UID/resourceVersion, and StatefulSet generation. On any auth loop, route loss, upstream error, latency regression, missing trace, or Secret/CA failure, restore the previous ConfigMap content and StatefulSet template, roll the partition back, wait for both replicas ready, and re-run the route diff and legacy route smoke. Do not delete route history or hide a half-applied generation.

## Current blockers

The live ConfigMap still has 57 routes and lacks all candidate route policies; its content hash differs from the candidate. The live APISIX StatefulSet lacks OIDC env and CA mount. `gateway/traffic-credentials` lacks `OIDC_CLIENT_SECRET`, `gateway/traffic-platform-ca` is absent, and N005 is not authorized. Therefore the expected acceptance state is `BLOCKED_ROUTE_DIFF`, not rollout success.
