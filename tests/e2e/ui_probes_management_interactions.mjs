#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const baselinePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png');
const acceptanceDir = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/probes');
const screenshotPath = path.join(acceptanceDir, 'actual-1920.png');
const bottomScreenshotPath = path.join(acceptanceDir, 'actual-bottom-1920.png');
const topology2DScreenshotPath = path.join(acceptanceDir, 'topology-2d-1920.png');
const diffPath = path.join(acceptanceDir, 'diff-1920.png');
const metricsPath = path.join(acceptanceDir, 'metrics.json');
const sideBySidePath = path.join(acceptanceDir, 'canonical-side-by-side-1920.png');
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-probes-latest.json');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';
fs.mkdirSync(acceptanceDir, { recursive: true });

function smokeToken({ tenantId = 'default', permissions = ['*', 'admin:*', 'probe:read', 'probe:write', 'screen:view'] } = {}) {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: tenantId, username: 'codex-probes-windows-cdp', roles: permissions.includes('probe:write') ? ['admin'] : ['viewer'],
    permissions, token_type: 'access', iat: now, exp: now + 1_800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', Buffer.from(encoded, 'base64').toString('utf8')).update(input).digest('base64url')}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/g, 'codex_smoke_token=<redacted>');
}

async function writeSideBySide(page) {
  const dataUrl = await page.evaluate(async ({ source, actual }) => {
    const decode = async (base64) => {
      const binary = atob(base64);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
      return createImageBitmap(new Blob([bytes], { type: 'image/png' }));
    };
    const [left, right] = await Promise.all([decode(source), decode(actual)]);
    const width = Math.max(left.width, right.width);
    const height = Math.max(left.height, right.height);
    const canvas = document.createElement('canvas');
    canvas.width = width * 2;
    canvas.height = height;
    const context = canvas.getContext('2d');
    context.drawImage(left, 0, 0, width, height);
    context.drawImage(right, width, 0, width, height);
    return canvas.toDataURL('image/png');
  }, { source: fs.readFileSync(baselinePath).toString('base64'), actual: fs.readFileSync(screenshotPath).toString('base64') });
  fs.writeFileSync(sideBySidePath, Buffer.from(dataUrl.split(',')[1], 'base64'));
}

function check(checks, id, pass, details = {}) {
  checks.push({ id, ...details, status: pass ? 'pass' : 'fail' });
}

function pngDimensions(filePath) {
  const buffer = fs.readFileSync(filePath);
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) };
}

function minimumGraphDistance(nodes, key) {
  let minimum = Number.POSITIVE_INFINITY;
  for (let left = 0; left < nodes.length; left += 1) {
    for (let right = left + 1; right < nodes.length; right += 1) {
      minimum = Math.min(minimum, Math.hypot(nodes[left][key].x - nodes[right][key].x, nodes[left][key].y - nodes[right][key].y));
    }
  }
  return minimum;
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
const targetsResponse = await fetch(`${cdpUrl}/json/list`);
if (!versionResponse.ok || !targetsResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const targets = await targetsResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
page.setDefaultTimeout(15_000);

const runtime = { bad_responses: [], console_errors: [], page_errors: [], request_failures: [] };
page.on('response', (response) => { if (response.status() >= 400) runtime.bad_responses.push({ status: response.status(), url: redact(response.url()) }); });
page.on('console', (entry) => { if (entry.type() === 'error' && !entry.text().includes('net::ERR_CONNECTION_CLOSED')) runtime.console_errors.push(entry.text()); });
page.on('pageerror', (error) => { if (error.message !== 'Object') runtime.page_errors.push(error.message); });
page.on('requestfailed', (request) => { if (request.url().startsWith(baseUrl) || request.url().includes('/api/')) runtime.request_failures.push(`${request.method()} ${redact(request.url())} ${request.failure()?.errorText ?? ''}`); });

const routeUrl = new URL(`/probes?windowsCdpAcceptanceTs=${Date.now()}`, baseUrl);
routeUrl.hash = `codex_smoke_token=${smokeToken()}`;
await page.goto(routeUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
await page.locator('.taf-probes').waitFor({ state: 'visible', timeout: 20_000 });
await page.locator('.taf-probes-topology-svg .taf-probe-deployment-svg').waitFor({ state: 'visible', timeout: 15_000 });
await page.locator('.taf-probes-trend-echart canvas').first().waitFor({ state: 'visible', timeout: 15_000 });
await page.waitForTimeout(500);
await page.bringToFront();
await page.keyboard.press('Control+0').catch(() => {});
await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false });
await cdp.send('Emulation.setPageScaleFactor', { pageScaleFactor: 1 });
await page.waitForTimeout(150);
const observedViewport = await page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
const zoomCalibration = 1920 / observedViewport.width;
if (observedViewport.width !== 1920 || observedViewport.height !== 1080) {
  await cdp.send('Emulation.setDeviceMetricsOverride', { width: Math.round(1920 * zoomCalibration), height: Math.round(1080 * zoomCalibration), deviceScaleFactor: 1 / zoomCalibration, mobile: false });
  await page.waitForTimeout(150);
}

const checks = [];
const acceptanceStartedAt = new Date().toISOString();
check(checks, 'xshell-windows-cdp-preflight', version.Browser?.startsWith('Chrome/') && targets.length > 0, { browser: version.Browser, target_count: targets.length });

const apiSnapshot = await page.evaluate(async () => {
  const token = window.localStorage.getItem('traffic-ui-token');
  const headers = token ? { Authorization: `Bearer ${token}` } : {};
  const [probeResponse, topologyResponse] = await Promise.all([
    fetch('/api/v1/probes?limit=50&offset=0', { headers }),
    fetch('/api/v1/probes/topology?mode=3d', { headers }),
  ]);
  const [probeBody, topologyBody] = await Promise.all([probeResponse.json(), topologyResponse.json()]);
  const payload = probeBody.data ?? probeBody;
  return {
    status: probeResponse.status,
    total: payload.total,
    probes: payload.probes ?? [],
    topologyStatus: topologyResponse.status,
    topology: topologyBody.data ?? topologyBody,
  };
});
const ids = apiSnapshot.probes.map((probe) => String(probe.probe_id));
check(checks, 'real-probe-api-25-canonical-rows', apiSnapshot.status === 200 && apiSnapshot.total === 25 && apiSnapshot.probes.length === 25 && ids.every((id) => !id.includes('-SIM')), { status: apiSnapshot.status, total: apiSnapshot.total, returned: apiSnapshot.probes.length, simulated_ids: ids.filter((id) => id.includes('-SIM')) });
const topologyApiNodes = apiSnapshot.topology.nodes ?? [];
const topologyApiEdges = apiSnapshot.topology.edges ?? [];
const topologyApiZones = apiSnapshot.topology.zones ?? [];
const dualLayoutNodes = topologyApiNodes.filter((node) => Number.isFinite(node.position_2d?.x) && Number.isFinite(node.position_2d?.y) && Number.isFinite(node.position_3d?.x) && Number.isFinite(node.position_3d?.y));
check(checks, 'dedicated-topology-api-contract', apiSnapshot.topologyStatus === 200 && apiSnapshot.topology.source === 'postgres.probes.hardware_info' && dualLayoutNodes.length >= 8 && topologyApiEdges.length >= 8 && topologyApiZones.length >= 1, { status: apiSnapshot.topologyStatus, source: apiSnapshot.topology.source, nodes: topologyApiNodes.length, dual_layout_nodes: dualLayoutNodes.length, edges: topologyApiEdges.length, zones: topologyApiZones.length });
const min2D = minimumGraphDistance(topologyApiNodes, 'position_2d');
const min3D = minimumGraphDistance(topologyApiNodes, 'position_3d');
check(checks, 'topology-api-minimum-node-spacing', min2D >= 6.8 && min3D >= 6.8, { minimum_2d: min2D, minimum_3d: min3D });

const metricText = await page.locator('.taf-probes-kpis').innerText();
const apiWarning = apiSnapshot.probes.filter((probe) => probe.status === 'degraded' || probe.status === 'warning' || probe.status === '告警').length;
const apiOffline = apiSnapshot.probes.filter((probe) => probe.status === 'offline' || probe.status === '离线').length;
const apiOnline = apiSnapshot.probes.length - apiOffline;
for (const [id, text] of [['metric-total', `${apiSnapshot.total} 台`], ['metric-online', `${apiOnline} 在线`], ['metric-nic', '48 张'], ['metric-modes', '4 种'], ['metric-cpu', '32.6%'], ['metric-memory', '41.3%'], ['metric-warning', `${apiWarning} 台`], ['metric-offline', `${apiOffline} 台`]]) {
  check(checks, id, metricText.includes(text), { expected: text });
}

const topologySvg = page.locator('.taf-probes-topology-svg .taf-probe-deployment-svg');
const topologySvgCount = await topologySvg.count();
const topologyNodeCount = await topologySvg.locator('.taf-probe-deployment-svg__node').count();
const topologyLinkCount = await topologySvg.locator('.taf-probe-deployment-svg__links > g').count();
const trendCanvasCount = await page.locator('.taf-probes-trend-echart canvas').count();
const campusImageCount = await page.locator('.taf-probes-campus-image').count();
check(checks, 'api-svg-topology-and-echarts-rendered', topologySvgCount === 1 && topologyNodeCount === topologyApiNodes.length && topologyLinkCount === topologyApiEdges.length && campusImageCount === 0 && trendCanvasCount === 2, { topology_svg_count: topologySvgCount, topology_node_count: topologyNodeCount, topology_api_node_count: topologyApiNodes.length, topology_link_count: topologyLinkCount, topology_api_edge_count: topologyApiEdges.length, static_campus_image_count: campusImageCount, trend_canvas_count: trendCanvasCount });

const threeDimensionalPath = await topologySvg.locator('.taf-probe-deployment-svg__links path').first().getAttribute('d');
const topology2DResponse = page.waitForResponse((response) => response.url().includes('/api/v1/probes/topology?mode=2d') && response.status() === 200, { timeout: 15_000 });
await page.getByRole('button', { name: '2D', exact: true }).click();
await topology2DResponse;
const twoDimensionalPath = await topologySvg.locator('.taf-probe-deployment-svg__links path').first().getAttribute('d');
check(checks, 'topology-2d-switch', await page.getByRole('button', { name: '2D', exact: true }).getAttribute('aria-pressed') === 'true' && await topologySvg.getAttribute('data-topology-mode') === '2d' && twoDimensionalPath !== threeDimensionalPath && await topologySvg.getAttribute('data-topology-source') === 'postgres.probes.hardware_info', { path_3d: threeDimensionalPath, path_2d: twoDimensionalPath, source: await topologySvg.getAttribute('data-topology-source') });
await page.waitForTimeout(250);
const topology2DScreenshot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
fs.writeFileSync(topology2DScreenshotPath, Buffer.from(topology2DScreenshot.data, 'base64'));
const topology2DSize = pngDimensions(topology2DScreenshotPath);
check(checks, 'topology-2d-screenshot-1920x1080', topology2DSize.width === 1920 && topology2DSize.height === 1080, topology2DSize);
const topology2DBytes = fs.statSync(topology2DScreenshotPath).size;
const topology2DContent = await page.evaluate(() => ({
  shell: document.querySelector('.taf-shell')?.getBoundingClientRect().width ?? 0,
  kpis: document.querySelectorAll('.taf-probes-kpis > *').length,
  panels: document.querySelectorAll('.taf-probes .taf-panel').length,
  nodes: document.querySelectorAll('.taf-probe-deployment-svg__node').length,
  detail: document.querySelector('.taf-probes-detail')?.textContent?.includes('探针 ID') ?? false,
}));
check(checks, 'topology-2d-screenshot-content-complete', topology2DBytes > 500_000 && topology2DContent.shell >= 1900 && topology2DContent.kpis === 8 && topology2DContent.panels >= 5 && topology2DContent.nodes === topologyApiNodes.length && topology2DContent.detail, { bytes: topology2DBytes, ...topology2DContent });

const selectedBefore = await topologySvg.locator('.taf-probe-deployment-svg__node.is-selected').getAttribute('data-probe-id');
await topologySvg.locator('.taf-probe-deployment-svg__node').nth(1).click();
const selectedAfter = await topologySvg.locator('.taf-probe-deployment-svg__node.is-selected').getAttribute('data-probe-id');
const selectedDetail = await page.locator('.taf-probes-detail').innerText();
check(checks, 'topology-node-selection-detail', Boolean(selectedAfter) && selectedAfter !== selectedBefore && selectedDetail.includes(selectedAfter) && await page.locator('.taf-probe-topology-card').count() === 0, { selected_before: selectedBefore, selected_after: selectedAfter, detail: selectedDetail.slice(0, 240) });

const initialViewBox = await topologySvg.getAttribute('viewBox');
await page.getByRole('button', { name: '放大部署拓扑' }).click();
const zoomedViewBox = await topologySvg.getAttribute('viewBox');
await page.getByRole('button', { name: '重置部署拓扑' }).click();
const resetViewBox = await topologySvg.getAttribute('viewBox');
check(checks, 'topology-zoom-reset', initialViewBox !== zoomedViewBox && resetViewBox === '0 0 1000 420', { initial_view_box: initialViewBox, zoomed_view_box: zoomedViewBox, reset_view_box: resetViewBox });
await page.locator('.taf-probes-trends-panel .ant-select').click();
await page.getByText('近 24 小时', { exact: true }).last().click();
check(checks, 'trend-range-switch', (await page.locator('.taf-probes-trends-panel .ant-select-selection-item').textContent()) === '近 24 小时');

const pagination = page.locator('.taf-probes-pagination');
await page.locator('.taf-probes').evaluate((business) => { business.scrollTop = 0; });
const paginationBefore = await pagination.boundingBox();
const firstPageId = await page.locator('.taf-probes-status-row .taf-probes-id-cell').first().innerText();
await page.getByRole('button', { name: '探针状态第 2 页' }).click();
await page.locator('.taf-probes').evaluate((business) => { business.scrollTop = 0; });
const paginationAfter = await pagination.boundingBox();
const secondPageId = await page.locator('.taf-probes-status-row .taf-probes-id-cell').first().innerText();
check(checks, 'pagination-fixed-position-and-real-page', paginationBefore && paginationAfter && Math.abs(paginationBefore.y - paginationAfter.y) <= 1 && firstPageId !== secondPageId && !secondPageId.includes('-SIM'), { before: paginationBefore, after: paginationAfter, first_page_id: firstPageId, second_page_id: secondPageId });
await page.getByRole('button', { name: '探针状态第 1 页' }).click();

const operationCases = [
  { id: 'batch-upgrade', label: '批量升级', modal: true, path: /\/api\/v1\/probes\/batch-upgrade$/ },
  { id: 'batch-state', label: '批量启停', modal: false, path: /\/api\/v1\/probes\/batch-state$/ },
  { id: 'config-push', label: '策略下发', modal: false, path: /\/api\/v1\/probes\/[^/]+\/config$/ },
  { id: 'connectivity', label: '连通性测试', modal: false, path: /\/api\/v1\/probes\/[^/]+\/connectivity-test$/ },
  { id: 'cert-rotate', label: '证书轮换', modal: true, path: /\/api\/v1\/probes\/[^/]+\/certificates\/rotate$/ },
  { id: 'restart', label: '重启探针', modal: false, path: /\/api\/v1\/probes\/[^/]+\/restart$/ },
];
const queuedOperationIds = [];
for (const operation of operationCases) {
  const responsePromise = page.waitForResponse((response) => response.request().method() === 'POST' && operation.path.test(new URL(response.url()).pathname));
  await page.locator('.taf-probes-actions button').filter({ hasText: operation.label }).click();
  const panel = page.locator(operation.modal ? '.ant-modal-content:visible' : '.ant-drawer-content-wrapper:visible');
  await panel.waitFor({ state: 'visible' });
  await panel.getByRole('button', { name: '确认提交' }).click();
  const response = await responsePromise;
  const body = await response.json();
  const data = body.data ?? body;
  await panel.locator('.ant-alert-success').waitFor({ state: 'visible' });
  const operationIds = [data.operation_id, ...(data.operation_ids ?? [])].filter(Boolean);
  queuedOperationIds.push(...operationIds);
  check(checks, `${operation.id}-real-backend-operation`, response.ok() && data.status === 'queued' && operationIds.length > 0, { http_status: response.status(), operation_ids: operationIds, status: data.status ?? '' });
  await panel.locator(operation.modal ? '.ant-modal-close' : '.ant-drawer-close').click();
  await panel.waitFor({ state: 'hidden' });
}

const viewerToken = smokeToken({ permissions: ['probe:read', 'screen:view'] });
const viewerWriteResponse = await fetch(`${baseUrl}/api/v1/probes/${encodeURIComponent(ids[0])}/restart`, { method: 'POST', headers: { Authorization: `Bearer ${viewerToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ reason: 'viewer permission boundary check' }) });
check(checks, 'viewer-write-forbidden', viewerWriteResponse.status === 403, { http_status: viewerWriteResponse.status });
const metricsOnlyToken = smokeToken({ permissions: ['probe:metrics'] });
const metricsOnlyTopologyResponse = await fetch(`${baseUrl}/api/v1/probes/topology?mode=3d`, { headers: { Authorization: `Bearer ${metricsOnlyToken}` } });
check(checks, 'metrics-only-topology-forbidden', metricsOnlyTopologyResponse.status === 403, { http_status: metricsOnlyTopologyResponse.status });
const invalidTopologyResponse = await fetch(`${baseUrl}/api/v1/probes/topology?mode=4d`, { headers: { Authorization: `Bearer ${viewerToken}` } });
check(checks, 'invalid-topology-mode-rejected', invalidTopologyResponse.status === 400, { http_status: invalidTopologyResponse.status });
const otherTenantToken = smokeToken({ tenantId: 'tenant-isolation-check', permissions: ['probe:read', 'probe:write', 'screen:view'] });
const crossTenantResponse = await fetch(`${baseUrl}/api/v1/probes/${encodeURIComponent(ids[0])}/restart`, { method: 'POST', headers: { Authorization: `Bearer ${otherTenantToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ reason: 'cross tenant boundary check' }) });
check(checks, 'cross-tenant-probe-hidden', crossTenantResponse.status === 404, { http_status: crossTenantResponse.status });
const crossTenantTopologyResponse = await fetch(`${baseUrl}/api/v1/probes/topology?mode=3d`, { headers: { Authorization: `Bearer ${otherTenantToken}` } });
const crossTenantTopologyBody = await crossTenantTopologyResponse.json();
const crossTenantTopology = crossTenantTopologyBody.data ?? crossTenantTopologyBody;
check(checks, 'cross-tenant-topology-isolated', crossTenantTopologyResponse.status === 200 && (crossTenantTopology.nodes ?? []).length === 0, { http_status: crossTenantTopologyResponse.status, node_count: (crossTenantTopology.nodes ?? []).length });

const operationDbCount = Number(execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', `SELECT count(*) FROM probe_operations WHERE operation_id = ANY(ARRAY[${queuedOperationIds.map((id) => `'${String(id).replaceAll("'", "''")}'::uuid`).join(',')}]) AND status='queued';`], { encoding: 'utf8', env: process.env, timeout: 20_000 }).trim());
check(checks, 'queued-operations-persisted', queuedOperationIds.length > 0 && operationDbCount === queuedOperationIds.length, { expected: queuedOperationIds.length, persisted: operationDbCount });
const queuedAuditActions = ['PROBE_BATCH_UPGRADE_QUEUED', 'PROBE_BATCH_STATE_QUEUED', 'PROBE_CONFIG_PUSH_QUEUED', 'PROBE_CONNECTIVITY_TEST_QUEUED', 'PROBE_CERT_ROTATE_QUEUED', 'PROBE_RESTART_QUEUED'];
const auditDbCount = Number(execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', `SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND action = ANY(ARRAY[${queuedAuditActions.map((action) => `'${action}'`).join(',')}]) AND created_at >= '${acceptanceStartedAt}'::timestamptz;`], { encoding: 'utf8', env: process.env, timeout: 20_000 }).trim());
check(checks, 'queued-operation-audits-persisted', auditDbCount === queuedAuditActions.length, { expected: queuedAuditActions.length, persisted: auditDbCount, actions: queuedAuditActions });

await page.locator('button[aria-label="配置探针"]').first().click();
const configModal = page.locator('.ant-modal-content:visible');
await configModal.waitFor({ state: 'visible' });
check(checks, 'row-config-dialog-exposes-real-endpoint', (await configModal.innerText()).includes('/v1/probes/') && (await configModal.innerText()).includes('/config'));
await configModal.locator('.ant-modal-close').click();

await page.getByRole('button', { name: '全屏查看矩阵' }).click();
const matrixDrawer = page.locator('.ant-drawer-content-wrapper:visible');
await matrixDrawer.waitFor({ state: 'visible' });
check(checks, 'fullscreen-matrix-reachable', await matrixDrawer.isVisible());
await matrixDrawer.locator('.ant-drawer-close').click();

await page.getByRole('button', { name: '3D', exact: true }).click();
await page.locator('.taf-probes-trends-panel .ant-select').click();
await page.getByText('近 6 小时', { exact: true }).last().click();
check(checks, 'canonical-state-restored', await page.getByRole('button', { name: '3D', exact: true }).getAttribute('aria-pressed') === 'true' && (await page.locator('.taf-probes-trends-panel .ant-select-selection-item').textContent()) === '近 6 小时');

const layout = await page.evaluate(() => {
  const rect = (selector) => {
    const element = document.querySelector(selector);
    if (!element) return null;
    const box = element.getBoundingClientRect();
    return { x: box.x, y: box.y, width: box.width, height: box.height, right: box.right, bottom: box.bottom };
  };
  const business = document.querySelector('.taf-probes');
  const businessBox = business?.getBoundingClientRect();
  const businessStyle = business ? getComputedStyle(business) : null;
  const visible = [...document.querySelectorAll('.taf-probes button, .taf-probes canvas, .taf-probes .taf-panel')].filter((element) => {
    const box = element.getBoundingClientRect(); const style = getComputedStyle(element);
    return box.width > 1 && box.height > 1 && style.visibility !== 'hidden' && style.display !== 'none';
  });
  const horizontallyClipped = visible.filter((element) => {
    const box = element.getBoundingClientRect();
    return businessBox && (box.x < businessBox.x - 2 || box.right > businessBox.right + 2);
  }).map((element) => element.textContent?.replace(/\s+/g, ' ').trim().slice(0, 40) || element.tagName);
  if (business) business.scrollTop = business.scrollHeight;
  const trendPanelBox = document.querySelector('.taf-probes-trends-panel')?.getBoundingClientRect();
  const heartbeatPanelBox = document.querySelector('.taf-probes-rail .taf-panel:last-child')?.getBoundingClientRect();
  const thresholdBox = document.querySelector('.taf-probes-threshold')?.getBoundingClientRect();
  const businessBottomBox = business?.getBoundingClientRect();
  return {
    viewport: { width: innerWidth, height: innerHeight }, root_horizontal_overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 2,
    business: rect('.taf-probes'), shell: rect('.taf-probes-shell'), main: rect('.taf-probes-main'), rail: rect('.taf-probes-rail'),
    pagination: rect('.taf-probes-pagination'), matrix: rect('.taf-probes-status-matrix'), horizontally_clipped: horizontallyClipped,
    overflow_y: businessStyle?.overflowY ?? 'missing', client_height: business?.clientHeight ?? 0, scroll_height: business?.scrollHeight ?? 0,
    scroll_top: business?.scrollTop ?? 0, bottom_reachable: Boolean(business && business.scrollTop + business.clientHeight >= business.scrollHeight - 2),
    trend_bottom_visible: Boolean(trendPanelBox && businessBottomBox && trendPanelBox.bottom <= businessBottomBox.bottom + 2),
    heartbeat_bottom_visible: Boolean(heartbeatPanelBox && businessBottomBox && heartbeatPanelBox.bottom <= businessBottomBox.bottom + 2),
    threshold_visible: Boolean(thresholdBox && businessBottomBox && thresholdBox.bottom <= businessBottomBox.bottom + 2),
    text: business?.textContent?.replace(/\s+/g, ' ').trim().slice(0, 12_000) || '',
  };
});
check(checks, 'layout-scrollable-contained-and-aligned', layout.viewport.width === 1920 && layout.viewport.height === 1080 && !layout.root_horizontal_overflow && layout.horizontally_clipped.length === 0 && ['auto', 'scroll'].includes(layout.overflow_y) && layout.scroll_height > layout.client_height && layout.bottom_reachable && layout.trend_bottom_visible && layout.heartbeat_bottom_visible && layout.threshold_visible && layout.main && layout.rail && Math.abs(layout.main.bottom - layout.rail.bottom) <= 2, { ...layout, observed_viewport_before_calibration: observedViewport, zoom_calibration: zoomCalibration });

await page.waitForTimeout(160);
const bottomScreenshot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
fs.writeFileSync(bottomScreenshotPath, Buffer.from(bottomScreenshot.data, 'base64'));
await page.locator('.taf-probes').evaluate((business) => { business.scrollTop = 0; });

await page.evaluate(() => { const main = document.querySelector('.taf-main'); if (main) main.scrollTop = 0; });
await page.waitForTimeout(200);
const screenshot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true });
fs.writeFileSync(screenshotPath, Buffer.from(screenshot.data, 'base64'));
await writeSideBySide(page);
const diffResult = spawnSync('python3', [
  'tests/e2e/ui_visual_diff_metrics.py', '--target-id', 'probes', '--route', '/probes',
  '--source', path.relative(root, baselinePath), '--actual', path.relative(root, screenshotPath), '--diff', path.relative(root, diffPath), '--metrics', path.relative(root, metricsPath),
  '--max-pixel-ratio', '0.12', '--channel-tolerance', '64', '--scoring-region', '198,78,1712,920', '--scoring-region-id', 'probes-business-roi-v1', '--desktop-status', 'pass',
], { cwd: root, encoding: 'utf8' });
const visual = JSON.parse(fs.readFileSync(metricsPath, 'utf8'));
check(checks, 'canonical-visual-diff', diffResult.status === 0 && visual.status === 'pass', { pixel_mismatch_ratio: visual.visual_diff.pixel_mismatch_ratio, max_pixel_ratio: visual.visual_diff.max_pixel_ratio, stdout: diffResult.stdout.trim(), stderr: diffResult.stderr.trim() });
check(checks, 'runtime-clean', runtime.bad_responses.length === 0 && runtime.console_errors.length === 0 && runtime.page_errors.length === 0 && runtime.request_failures.length === 0, runtime);

const failures = checks.filter((item) => item.status !== 'pass');
const result = {
  package_id: 'probes_windows_chrome_xshell_acceptance', generated_at: new Date().toISOString(), result: failures.length ? 'fail' : 'pass',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome -> direct APISIX', browser: version.Browser,
  route: redact(routeUrl.toString()), viewport: { width: 1920, height: 1080, device_scale_factor: 1 },
  summary: { checks: checks.length, pass: checks.length - failures.length, fail: failures.length, visual_status: visual.status },
  failures, checks, layout, runtime_errors: runtime,
  artifacts: { baseline: path.relative(root, baselinePath), actual: path.relative(root, screenshotPath), actual_bottom: path.relative(root, bottomScreenshotPath), topology_2d: path.relative(root, topology2DScreenshotPath), diff: path.relative(root, diffPath), metrics: path.relative(root, metricsPath), side_by_side: path.relative(root, sideBySidePath) },
  token_material_redacted: true,
};
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`);
fs.writeFileSync(path.join(acceptanceDir, 'acceptance-report.json'), `${JSON.stringify(result, null, 2)}\n`);
console.log(JSON.stringify({ result: result.result, summary: result.summary, failures: failures.map((item) => item.id), output: path.relative(root, outputPath) }, null, 2));
await page.close().catch(() => {});
process.exit(result.result === 'pass' ? 0 : 1);
