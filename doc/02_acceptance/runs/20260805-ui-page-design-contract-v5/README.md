# UI page design contract candidate v5

This evidence package binds the versioned 28-page UI contract to the deployed
`remediation-ui-c2975d525e67` candidate. The candidate is immutable by image ID,
containerd manifest digest, source composition hash, and exact static-bundle
reconciliation.

The Windows Chrome read gate covers six representative business pages with mock
mode disabled: assets, alerts, topics, graph, models, and forensics. The browser
used the Xshell-forwarded CDP endpoint at `127.0.0.1:9224`; the report retains
screenshots, request observations, runtime errors, and source-reference checks.

The alert report export/cancel write journey is recorded separately under the
actual historical candidate that executed it. That candidate and the current
candidate have identical reconciled static content, but the evidence identity is
not rewritten. The write journey is a scoped pass and does not prove every UI
write interaction.

This package is not full UI acceptance. Twenty-two page journeys, 28/28 visual
comparison, remaining write-side effects, accessibility, multi-viewport
behavior, and embedding the contract registry in the production bundle remain
explicitly open.
