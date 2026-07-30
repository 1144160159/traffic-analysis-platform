#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const baseUrl = process.env.UI_BASE_URL ?? 'http://10.0.5.8:30180';
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const revision = process.env.ATTACK_CHAIN_REVISION ?? 'current';
const viewportWidth = Number(process.env.UI_VIEWPORT_WIDTH ?? '1920');
const viewportHeight = Number(process.env.UI_VIEWPORT_HEIGHT ?? '1080');
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/attack-chains');
const screenshotPath = path.join(evidenceDir, `implementation-${revision}.png`);
const reportPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-attack-chains-${revision}.json`,
);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function createSmokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const base64url = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = base64url({ alg: 'HS256', typ: 'JWT' });
  const claims = base64url({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-attack-chain-acceptance',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'alert:*', 'graph:read', 'playbook:execute'],
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1_800,
  });
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

fs.mkdirSync(evidenceDir, { recursive: true });
fs.mkdirSync(path.dirname(reportPath), { recursive: true });

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const token = createSmokeToken();
const errors = { responses: [], console: [], page: [], requests: [] };

page.on('response', (response) => {
  if (response.status() >= 400 && response.url().startsWith(baseUrl)) {
    errors.responses.push({ status: response.status(), url: response.url().replace(/[?#].*$/, '') });
  }
});
page.on('console', (entry) => {
  if (entry.type() === 'error') errors.console.push(entry.text());
});
page.on('pageerror', (error) => errors.page.push(error.message));
page.on('requestfailed', (request) => {
  errors.requests.push({
    method: request.method(),
    url: request.url().replace(/[?#].*$/, ''),
    error: request.failure()?.errorText ?? '',
  });
});

let exitCode = 1;
try {
  await page.setViewportSize({ width: viewportWidth, height: viewportHeight });
  const url = new URL('/attack-chains', baseUrl);
  url.searchParams.set('acceptanceTs', String(Date.now()));
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-attack-chain').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  if (new URL(page.url()).pathname !== '/attack-chains') {
    const authenticatedUrl = new URL('/attack-chains', baseUrl);
    authenticatedUrl.searchParams.set('acceptanceTs', String(Date.now()));
    await page.goto(authenticatedUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
    await page.locator('.taf-attack-chain').waitFor({ state: 'visible', timeout: 20_000 });
    await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
  }
  try {
    await page.locator('.taf-attack-column').first().waitFor({ state: 'visible', timeout: 30_000 });
  } catch (error) {
    const failurePath = path.join(evidenceDir, `implementation-${revision}-load-failure.png`);
    await page.screenshot({ path: failurePath, fullPage: false }).catch(() => {});
    console.error(JSON.stringify({
      stage: 'attack-chain-phase-load',
      route: page.url(),
      visible_text: (await page.locator('body').innerText().catch(() => '')).slice(0, 4000),
      errors,
      screenshot: path.relative(root, failurePath),
    }, null, 2));
    throw error;
  }
  await page.locator('.taf-attack-evidence-table > button').first().waitFor({ state: 'visible', timeout: 10_000 });
  await page.waitForTimeout(300);
  if (viewportWidth < 1600) {
    await page.screenshot({ path: path.join(evidenceDir, `implementation-${revision}-preinteraction.png`), fullPage: false });
  }

  const interaction = {};
  const firstPhase = page.locator('.taf-attack-column').first();
  const filteredEvidenceResponse = page.waitForResponse(
    (response) => response.url().includes('/evidence?')
      && response.url().includes('phase=')
      && response.status() === 200,
  );
  const filteredRecommendationResponse = page.waitForResponse(
    (response) => response.url().includes('/recommendations?')
      && response.url().includes('phase=')
      && response.status() === 200,
  );
  await firstPhase.click();
  await Promise.all([filteredEvidenceResponse, filteredRecommendationResponse]);
  await page.waitForFunction(() => (
    document.querySelectorAll('.taf-attack-evidence-table > button').length === 1
    && document.querySelectorAll('.taf-attack-recommendations > button').length === 1
  ));
  interaction.phase_selected = await firstPhase.getAttribute('aria-pressed');
  interaction.filtered_evidence_rows = await page.locator('.taf-attack-evidence-table > button').count();
  interaction.filtered_recommendation_rows = await page.locator('.taf-attack-recommendations > button').count();
  await firstPhase.click();
  await page.waitForFunction(() => (
    document.querySelector('.taf-attack-column:first-of-type')?.getAttribute('aria-pressed') === 'false'
    && document.querySelectorAll('.taf-attack-evidence-table > button').length === 3
    && document.querySelectorAll('.taf-attack-recommendations > button').length === 6
  ));
  interaction.phase_cleared = await firstPhase.getAttribute('aria-pressed');

  const zoomTarget = page.locator('.taf-attack-canvas');
  interaction.scale_before = await zoomTarget.evaluate((node) => getComputedStyle(node).getPropertyValue('--attack-chain-scale').trim());
  await page.getByRole('button', { name: '放大攻击链画布' }).click();
  interaction.scale_after = await zoomTarget.evaluate((node) => getComputedStyle(node).getPropertyValue('--attack-chain-scale').trim());
  await page.getByRole('button', { name: '缩小攻击链画布' }).click();

  const pcapResponse = page.waitForResponse((response) => response.url().includes('/evidence?') && response.url().includes('type=pcap') && response.status() === 200);
  await page.locator('.taf-attack-tabs button', { hasText: 'PCAP' }).click();
  await pcapResponse;
  interaction.pcap_rows = await page.locator('.taf-attack-evidence-table > button').count();
  interaction.pcap_total = await page.locator('.taf-attack-pagination .ant-pagination-total-text').innerText();
  await page.locator('.taf-attack-tabs button', { hasText: '全部' }).click();
  await page.locator('.taf-attack-evidence-table > button').first().waitFor({ state: 'visible' });
  await page.waitForTimeout(100);
  interaction.all_evidence_total = await page.locator('.taf-attack-pagination .ant-pagination-total-text').innerText();

  const evidenceFirstPage = await page.locator('.taf-attack-evidence-table > button strong').allTextContents();
  const evidenceSecondResponse = page.waitForResponse((response) => response.url().includes('/evidence?') && response.url().includes('offset=3') && response.status() === 200);
  await page.locator('.taf-attack-pagination .ant-pagination-item-2').click();
  await evidenceSecondResponse;
  const evidenceSecondPage = await page.locator('.taf-attack-evidence-table > button strong').allTextContents();
  interaction.evidence_pagination = {
    first_page: evidenceFirstPage,
    second_page: evidenceSecondPage,
    changed: JSON.stringify(evidenceFirstPage) !== JSON.stringify(evidenceSecondPage),
  };
  await page.locator('.taf-attack-pagination .ant-pagination-item-1').click();
  await page.waitForTimeout(100);

  const pathFirstPage = await page.locator('.taf-attack-paged-table .ant-table-tbody > tr').allTextContents();
  const pathSecondResponse = page.waitForResponse((response) => response.url().includes('/paths?') && response.url().includes('offset=3') && response.status() === 200);
  await page.locator('.taf-attack-paged-table > .ant-pagination .ant-pagination-item-2').click();
  await pathSecondResponse;
  const pathSecondPage = await page.locator('.taf-attack-paged-table .ant-table-tbody > tr').allTextContents();
  interaction.path_pagination = {
    first_page: pathFirstPage,
    second_page: pathSecondPage,
    changed: JSON.stringify(pathFirstPage) !== JSON.stringify(pathSecondPage),
  };
  await page.locator('.taf-attack-paged-table > .ant-pagination .ant-pagination-item-1').click();
  await page.waitForTimeout(100);

  interaction.recommendation_tabs = {};
  interaction.recommendation_tabs.block = {
    rows: await page.locator('.taf-attack-recommendations > button').count(),
    first: (await page.locator('.taf-attack-recommendations > button').first().innerText()).replace(/\s+/g, ' ').trim(),
  };
  for (const [label, category] of [['隔离建议', 'isolate'], ['白名单风险', 'allowlist'], ['剧本推荐', 'playbook']]) {
    const responsePromise = page.waitForResponse(
      (response) => response.url().includes('/recommendations?')
        && response.url().includes(`category=${category}`)
        && response.status() === 200,
    );
    await page.locator('.taf-attack-suggestion-tabs button', { hasText: label }).click();
    await responsePromise;
    await page.locator('.taf-attack-recommendations > button').first().waitFor({ state: 'visible' });
    interaction.recommendation_tabs[category] = {
      rows: await page.locator('.taf-attack-recommendations > button').count(),
      first: (await page.locator('.taf-attack-recommendations > button').first().innerText()).replace(/\s+/g, ' ').trim(),
    };
  }
  await page.locator('.taf-attack-suggestion-tabs button', { hasText: '阻断点' }).click();
  await page.waitForFunction(() => document.querySelector('.taf-attack-recommendations > button')?.textContent?.includes('阻断'));
  await page.evaluate(() => {
    window.scrollTo(0, 0);
    for (const selector of ['.taf-attack-chain', '.taf-attack-canvas', '.taf-attack-shell']) {
      const node = document.querySelector(selector);
      if (node instanceof HTMLElement) node.scrollTo({ top: 0, left: 0, behavior: 'instant' });
    }
  });
  await page.waitForTimeout(200);
  const apiSummary = await page.evaluate(async () => {
    const token = localStorage.getItem('traffic-ui-token') ?? '';
    const summarize = async (url) => {
      const response = await fetch(url, { headers: { Authorization: `Bearer ${token}` } });
      const payload = await response.json().catch(() => ({}));
      const envelope = payload?.data ?? payload;
      const records = Array.isArray(envelope?.chains)
        ? envelope.chains
        : Array.isArray(envelope?.campaigns)
          ? envelope.campaigns
          : [];
      return {
        status: response.status,
        count: records.length,
        total: Number(envelope?.total ?? records.length),
        first_id: records[0]?.chain_id ?? records[0]?.campaign_id ?? null,
        first_phase_count: Array.isArray(records[0]?.phases) ? records[0].phases.length : null,
      };
    };
    return {
      attack_chains: await summarize('/api/v1/attack-chains?limit=3'),
      campaigns: await summarize('/api/v1/campaigns?limit=3'),
    };
  });

  const selectors = {
    shell: '.taf-attack-shell',
    toolbar: '.taf-attack-toolbar',
    canvas: '.taf-attack-canvas',
    phaseColumns: '.taf-attack-column',
    evidenceRows: '.taf-attack-evidence-table > button',
    recommendationRows: '.taf-attack-recommendations > button',
    matrixItems: '.taf-attack-matrix > button',
    pathRows: '.taf-attack-bottom .ant-table-tbody > tr',
  };
  const counts = {};
  for (const [name, selector] of Object.entries(selectors)) {
    counts[name] = await page.locator(selector).count();
  }

  const canvasBox = await page.locator(selectors.canvas).boundingBox();
  const railBox = await page.locator('.taf-attack-rail').boundingBox();
  const canvasGeometry = await page.evaluate(() => {
    const rectPayload = (rect) => ({
      left: rect.left,
      right: rect.right,
      top: rect.top,
      bottom: rect.bottom,
      width: rect.width,
      height: rect.height,
      centerX: rect.left + rect.width / 2,
      centerY: rect.top + rect.height / 2,
    });
    const box = (selector) => {
      const node = document.querySelector(selector);
      if (!node) return null;
      return rectPayload(node.getBoundingClientRect());
    };
    const iconAndTextGroupBox = (selector) => {
      const card = document.querySelector(selector);
      if (!card) return null;
      const childRects = [...card.children]
        .map((child) => child.getBoundingClientRect())
        .filter((rect) => rect.width > 0 && rect.height > 0);
      if (!childRects.length) return null;
      const left = Math.min(...childRects.map((rect) => rect.left));
      const right = Math.max(...childRects.map((rect) => rect.right));
      const top = Math.min(...childRects.map((rect) => rect.top));
      const bottom = Math.max(...childRects.map((rect) => rect.bottom));
      return rectPayload({
        left,
        right,
        top,
        bottom,
        width: right - left,
        height: bottom - top,
      });
    };
    const laneSelectors = [
      '.taf-attack-lane-head',
      '.taf-attack-lane-labels > strong:nth-of-type(1)',
      '.taf-attack-lane-labels > strong:nth-of-type(2)',
      '.taf-attack-lane-labels > strong:nth-of-type(3)',
      '.taf-attack-lane-labels > strong:nth-of-type(4)',
    ];
    const contentSelectors = [
      '.taf-attack-column:first-of-type .taf-attack-phase-card',
      '.taf-attack-column:first-of-type .taf-attack-entity-card',
      '.taf-attack-column:first-of-type .taf-attack-alert-card',
      '.taf-attack-column:first-of-type .taf-attack-evidence-card',
      '.taf-attack-column:first-of-type .taf-attack-action-card',
    ];
    const laneCenters = laneSelectors.map(box);
    const iconAndTextGroups = contentSelectors.map(iconAndTextGroupBox);
    const centerDeltas = laneCenters.map((lane, index) => (
      lane && iconAndTextGroups[index] ? Math.abs(lane.centerY - iconAndTextGroups[index].centerY) : null
    ));
    const verticalGroups = [
      ['.taf-attack-phase-card', '.is-phase-entity', '.taf-attack-entity-card'],
      ['.taf-attack-entity-card', '.is-entity-alert', '.taf-attack-alert-card'],
      ['.taf-attack-alert-card', '.is-alert-evidence', '.taf-attack-evidence-card'],
      ['.taf-attack-evidence-card', '.is-evidence-action', '.taf-attack-action-card'],
    ].map(([beforeSelector, arrowSelector, afterSelector]) => {
      const before = box(`.taf-attack-column:first-of-type ${beforeSelector}`);
      const arrow = box(`.taf-attack-column:first-of-type ${arrowSelector}`);
      const after = box(`.taf-attack-column:first-of-type ${afterSelector}`);
      return {
        before,
        arrow,
        after,
        clear: Boolean(before && arrow && after && arrow.top >= before.bottom && arrow.bottom <= after.top),
      };
    });
    const entities = [...document.querySelectorAll('.taf-attack-entity-card')].map((node) => {
      const rect = node.getBoundingClientRect();
      return { left: rect.left, right: rect.right, centerY: rect.top + rect.height / 2 };
    });
    const horizontalArrows = [...document.querySelectorAll('.taf-attack-horizontal-link')].map((node, index) => {
      const rect = node.getBoundingClientRect();
      const before = entities[index];
      const after = entities[index + 1];
      return {
        left: rect.left,
        right: rect.right,
        centerY: rect.top + rect.height / 2,
        aligned: Boolean(before && after
          && rect.left >= before.right
          && rect.right <= after.left
          && Math.abs((rect.top + rect.height / 2) - before.centerY) <= 2),
      };
    });
    return {
      laneCenters,
      iconAndTextGroups,
      centerDeltas,
      verticalGroups,
      horizontalArrows,
      horizontalArrowCount: horizontalArrows.length,
      verticalArrowCount: document.querySelectorAll('.taf-attack-vertical-link').length,
    };
  });
  const pageMetrics = await page.evaluate(() => ({
    viewport: { width: window.innerWidth, height: window.innerHeight },
    document: {
      width: document.documentElement.scrollWidth,
      height: document.documentElement.scrollHeight,
    },
    horizontal_overflow: document.documentElement.scrollWidth > window.innerWidth + 2,
    body_title: document.querySelector('.taf-attack-toolbar h1')?.textContent?.trim() ?? '',
    selected_chain_text: document.querySelector('.taf-attack-filters .ant-select-selection-item')?.textContent?.trim() ?? '',
    attack_chain_resources: performance.getEntriesByType('resource')
      .map((entry) => entry.name)
      .filter((name) => name.includes('/attack-chains'))
      .map((name) => name.replace(/[?#].*$/, '')),
  }));
  await page.screenshot({ path: screenshotPath, fullPage: false });

  const checks = [
    { name: `Windows Chrome viewport is ${viewportWidth}x${viewportHeight}`, passed: pageMetrics.viewport.width === viewportWidth && pageMetrics.viewport.height === viewportHeight },
    { name: 'attack-chain business title is visible', passed: pageMetrics.body_title === '攻击链分析' },
    { name: 'attack-chain canvas is visible', passed: Boolean(canvasBox && canvasBox.width >= (viewportWidth >= 1600 ? 800 : 620) && canvasBox.height >= 420) },
    { name: 'right evidence and response rail is visible', passed: Boolean(railBox && railBox.width >= 360 && railBox.height >= (viewportWidth >= 1600 ? 500 : 300)) },
    { name: 'six API-driven attack phases are present', passed: counts.phaseColumns === 6 },
    { name: 'database-backed evidence page contains three rows', passed: counts.evidenceRows === 3 },
    { name: 'six response recommendations are present', passed: counts.recommendationRows === 6 },
    { name: 'ATT&CK matrix follows phase count', passed: counts.matrixItems === counts.phaseColumns },
    { name: 'database-backed path page contains three rows', passed: counts.pathRows === 3 },
    { name: 'phase selection filters the linked rail', passed: interaction.phase_selected === 'true' && interaction.filtered_evidence_rows === 1 && interaction.filtered_recommendation_rows === 1 && interaction.phase_cleared === 'false' },
    { name: 'canvas zoom changes the rendered scale', passed: Number(interaction.scale_after) > Number(interaction.scale_before) },
    { name: 'PCAP evidence tab filters database evidence', passed: interaction.pcap_rows > 0 && interaction.pcap_total.includes('3') && interaction.all_evidence_total.includes('6') },
    { name: 'evidence anchors use backend pagination', passed: interaction.evidence_pagination.changed && interaction.evidence_pagination.first_page.length === 3 && interaction.evidence_pagination.second_page.length === 3 },
    { name: 'path details use backend pagination', passed: interaction.path_pagination.changed && interaction.path_pagination.first_page.length === 3 && interaction.path_pagination.second_page.length === 3 },
    { name: 'all recommendation tabs use distinct database rows', passed: new Set(Object.values(interaction.recommendation_tabs).map((item) => item.first)).size === 4 && Object.values(interaction.recommendation_tabs).every((item) => item.rows === 6) },
    { name: 'five icon-and-text groups align to their left label centerlines', passed: canvasGeometry.centerDeltas.length === 5 && canvasGeometry.centerDeltas.every((delta) => delta !== null && delta <= 2) },
    { name: 'vertical bidirectional arrows stay between rows', passed: canvasGeometry.verticalArrowCount === 24 && canvasGeometry.verticalGroups.every((group) => group.clear) },
    { name: 'horizontal arrows stay centered between entity columns', passed: canvasGeometry.horizontalArrowCount === 5 && canvasGeometry.horizontalArrows.every((arrow) => arrow.aligned) },
    { name: 'no horizontal browser overflow', passed: !pageMetrics.horizontal_overflow },
    { name: 'no runtime errors', passed: Object.values(errors).every((items) => items.length === 0) },
  ];
  const report = {
    run_id: `attack-chains-${revision}`,
    generated_at: new Date().toISOString(),
    browser_backend: 'Windows Chrome CDP over Xshell 9224 -> 9222',
    browser: version.Browser,
    route: '/attack-chains',
    viewport: pageMetrics.viewport,
    screenshot: path.relative(root, screenshotPath),
    counts,
    geometry: { canvas: canvasBox, rail: railBox, canvas_alignment: canvasGeometry },
    document: pageMetrics.document,
    selected_chain_text: pageMetrics.selected_chain_text,
    attack_chain_resources: pageMetrics.attack_chain_resources,
    api_summary: apiSummary,
    interaction,
    errors,
    checks,
    result: checks.every((check) => check.passed) ? 'pass' : 'fail',
  };
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(JSON.stringify({ report: path.relative(root, reportPath), ...report }, null, 2));
  exitCode = report.result === 'pass' ? 0 : 1;
} finally {
  if (process.env.UI_KEEP_PAGE_OPEN !== '1') {
    await page.close().catch(() => {});
  }
}

process.exit(exitCode);
