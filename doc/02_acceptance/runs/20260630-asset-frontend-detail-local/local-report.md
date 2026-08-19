# Asset Frontend Detail Linkage Local Report

Run ID: `20260630-asset-frontend-detail-local`
Date: 2026-06-30
Scope: local Web UI adapter/page contract only. This is not a live APISIX rollout, Desktop Chrome browser pass, real campus scan coverage report, or device discovery-rate acceptance.

## Change

- `/assets` snapshot adapter now consumes `/v1/assets/discovery/runs` and `/v1/assets/discovery/neighbors` secondary payloads.
- Asset rows carry discovery context fields for latest run id, run status, discovered asset/link counts, topology neighbor count, neighbor label, and topology protocol.
- `AssetInventoryPage` shows latest discovery run and LLDP/SNMP neighbor context in the asset detail rail and asset-context tabs.

## Verification

```bash
npm --prefix web/ui run test -- --run src/services/pageSnapshotAdapters.test.ts src/services/pageApiPlans.test.ts src/routes/routeManifest.test.ts
```

Result: `3` test files passed, `49` tests passed.

```bash
npm --prefix web/ui run build
```

Result: TypeScript and Vite production build passed. Vite reported only the existing large chunk warning.

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```

Result: `181` manifest items, `28` route contracts, `70` overlay contracts, `errors: 0, warnings: 0`.

## Browser Status

Codex Desktop Chrome bridge is still not usable in this session. `desktop_chrome_open_url(url="http://10.0.5.8:30180/login", keep=true)` and `js_reset` both returned `Transport closed`, so this report does not claim browser-facing completion.
