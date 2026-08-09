# MinIO TLS cutover v1

This directory is a default-off candidate overlay. It is not referenced by the
normal deployment path and must not be applied until
`contracts/minio/tls-cutover.v1.json` records `cutover_ready=true` with an
approved maintenance window and immutable image digests.

The overlay currently references five locally built `linux/amd64` candidate
tags recorded in the contract. Their local Docker image IDs are evidence of a
successful local build only; they are not registry repo digests. The bundle
therefore remains `cutover_ready=false` until the images are signed, pinned by
registry digest, distributed to both target nodes and verified in the approved
window.

Render and validate without changing the cluster:

```bash
kubectl kustomize --load-restrictor=LoadRestrictionsNone \
  deployments/kubernetes/security/minio-tls-cutover-v1 \
  >/tmp/minio-tls-cutover-v1-rendered.yaml
kubectl apply --dry-run=client --validate=false \
  -f /tmp/minio-tls-cutover-v1-rendered.yaml
python3 scripts/alignment/verify_minio_tls_cutover.py
make alignment-capture-minio-tls-candidate-images \
  RUN_ID=<immutable-run-id> \
  G0_MANIFEST=<matching-g0-manifest>
```

Do not use `kubectl apply -k` directly: the overlay intentionally references
reviewed sibling baselines and therefore requires the explicit local render
command. The bundle requires a coordinated outage because a distributed MinIO
cluster must not run mixed HTTP and HTTPS members. Follow the contract's
quiesce, cutover, stop and rollback sequences as one change record.

The checked-in overlay contains no certificate or private-key values. Server
material and client trust are delivered only by ExternalSecret. The Flink
PKCS12 file contains the public CA only; its conventional `changeit` integrity
password is not a private-key credential.
