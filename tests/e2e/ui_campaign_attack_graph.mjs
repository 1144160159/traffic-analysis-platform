#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = process.env.UI_BASE_URL ?? 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION ?? 'r725';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/campaigns/attack-graph-${revision}.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const encoded = execFileSync(
  'kubectl',
  ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
  { encoding: 'utf8', env: process.env, timeout: 30_000 },
);
const now = Math.floor(Date.now() / 1_000);
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service',
  sub: crypto.randomUUID(),
  jti: crypto.randomUUID(),
  user_id: crypto.randomUUID(),
  tenant_id: 'default',
  username: 'codex-campaign-attack-graph',
  roles: ['admin'],
  permissions: ['*', 'admin:*', 'alert:*', 'graph:read'],
  token_type: 'access',
  session_id: crypto.randomUUID(),
  iat: now,
  exp: now + 1_800,
})).toString('base64url');
const tokenInput = `${header}.${claims}`;
const secret = Buffer.from(encoded, 'base64').toString('utf8');
const accessToken = `${tokenInput}.${crypto.createHmac('sha256', secret).update(tokenInput).digest('base64url')}`;

const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const url = new URL('/campaigns', baseUrl);
url.searchParams.set('attackGraphTs', String(Date.now()));
url.hash = `codex_smoke_token=${accessToken}`;
await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});

const graph = page.locator('[data-chart-engine="echarts"][data-series-type="graph"]');
await graph.waitFor({ state: 'visible', timeout: 20_000 });
const canvas = graph.locator('canvas');
const graphMetrics = await graph.evaluate((element) => {
  const rect = element.getBoundingClientRect();
  const chartHost = element.firstElementChild;
  const chartInstanceId = chartHost?.getAttribute('_echarts_instance_') ?? '';
  return {
    engine: element.getAttribute('data-chart-engine'),
    series_type: element.getAttribute('data-series-type'),
    width: rect.width,
    height: rect.height,
    canvas_count: element.querySelectorAll('canvas').length,
    chart_instance_id: chartInstanceId,
  };
});

const beforeZoom = crypto.createHash('sha256').update(await canvas.screenshot()).digest('hex');
await canvas.hover({ position: { x: 290, y: 190 } });
await page.mouse.wheel(0, -420);
await page.waitForTimeout(450);
const afterZoom = crypto.createHash('sha256').update(await canvas.screenshot()).digest('hex');

await page.reload({ waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
const reloadedCanvas = page.locator('[data-chart-engine="echarts"][data-series-type="graph"] canvas');
await reloadedCanvas.waitFor({ state: 'visible', timeout: 20_000 });
const actionResponsePromise = page.waitForResponse((response) =>
  response.request().method() === 'POST'
    && /\/v1\/campaigns\/[^/]+\/actions/.test(response.url()),
);
await reloadedCanvas.click({ position: { x: 60, y: 114 } });
const actionResponse = await actionResponsePromise;
const actionPayload = await actionResponse.json().catch(() => ({}));
const actionData = actionPayload?.data ?? actionPayload;
const drawer = page.locator('.taf-campaign-action-drawer:visible');
await drawer.waitFor({ state: 'visible', timeout: 10_000 });
const drawerText = await drawer.textContent();

const checks = [
  { name: 'real ECharts graph host', passed: graphMetrics.engine === 'echarts' && graphMetrics.series_type === 'graph' },
  { name: 'ECharts canvas and instance mounted', passed: graphMetrics.canvas_count === 1 && Boolean(graphMetrics.chart_instance_id) },
  { name: 'graph has production viewport size', passed: graphMetrics.width >= 500 && graphMetrics.height >= 380 },
  { name: 'roam zoom changes rendered graph', passed: beforeZoom !== afterZoom },
  { name: 'phase node click reaches campaign action API', passed: actionResponse.ok() && actionData?.audit_event === 'CAMPAIGN_PHASE_VIEWED' },
  { name: 'phase node click opens audited action drawer', passed: Boolean(drawerText?.includes('CAMPAIGN_PHASE_VIEWED') && drawerText?.includes('/api/v1/campaigns/')) },
];
const result = {
  run_id: `campaign-attack-graph-${revision}`,
  result: checks.every((check) => check.passed) ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome CDP over Xshell 9224',
  base_url: baseUrl,
  graph_metrics: graphMetrics,
  zoom_render_hash_before: beforeZoom,
  zoom_render_hash_after: afterZoom,
  action_http_status: actionResponse.status(),
  action_audit_event: actionData?.audit_event ?? null,
  check_count: checks.length,
  passed: checks.filter((check) => check.passed).length,
  failed: checks.filter((check) => !check.passed).length,
  checks,
  timestamp: new Date().toISOString(),
};

fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify(result, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);
