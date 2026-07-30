#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const revision = process.env.TOPIC_REVISION ?? 'topic-panel-tunnel-analysis-tab-geometry';
const evidenceRoot = path.join(root, 'evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel');
const outputPath = path.join(
  root,
  'evidence/ui-image-breakdowns/pages',
  `topics-tunnel-analysis-tab-geometry-${revision}.json`,
);
const tabLabels = ['协议分析', '隧道源', '端点国家分布'];

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function smokeToken() {
  const encoded = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-tunnel-tab-geometry-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read'],
    token_type: 'access',
    session_id: `codex-tunnel-tab-geometry-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function compareGeometry(states, viewport) {
  const baseline = states[0];
  const deltas = states.map((state) => ({
    label: state.label,
    panel: {
      width: Math.abs(state.panel.width - baseline.panel.width),
      height: Math.abs(state.panel.height - baseline.panel.height),
    },
    body: {
      width: Math.abs(state.body.width - baseline.body.width),
      height: Math.abs(state.body.height - baseline.body.height),
    },
    grid: {
      width: Math.abs(state.grid.width - baseline.grid.width),
      height: Math.abs(state.grid.height - baseline.grid.height),
    },
    relative: {
      panelLeft: Math.abs(state.relative.panelLeft - baseline.relative.panelLeft),
      panelTop: Math.abs(state.relative.panelTop - baseline.relative.panelTop),
      bodyLeft: Math.abs(state.relative.bodyLeft - baseline.relative.bodyLeft),
      bodyTop: Math.abs(state.relative.bodyTop - baseline.relative.bodyTop),
      gridLeft: Math.abs(state.relative.gridLeft - baseline.relative.gridLeft),
      gridTop: Math.abs(state.relative.gridTop - baseline.relative.gridTop),
    },
  }));
  const maximumDelta = Math.max(
    ...deltas.flatMap((item) => [
      ...Object.values(item.panel),
      ...Object.values(item.body),
      ...Object.values(item.grid),
      ...Object.values(item.relative),
    ]),
  );
  return {
    viewport,
    baseline,
    states,
    deltas,
    maximumDelta,
    passed: states.length === tabLabels.length
      && states.every((state) =>
        state.geometryContract === 'fixed-within-viewport'
        && state.activeTab === state.tabId
        && state.clickTargetMatches)
      && maximumDelta <= 1,
  };
}

async function inspectState(page, label) {
  return page.locator('.taf-topic-tunnel-analysis').evaluate((panel, expectedLabel) => {
    const round = (value) => Math.round(value * 100) / 100;
    const rect = (element) => {
      const value = element.getBoundingClientRect();
      return {
        left: round(value.left),
        top: round(value.top),
        width: round(value.width),
        height: round(value.height),
      };
    };
    const boardline = panel.closest('.taf-topic-tunnel-boardline');
    const body = panel.querySelector(':scope > .taf-panel__body');
    const tabs = panel.querySelector('.taf-topic-tunnel-analysis-tabs');
    const grid = panel.querySelector('.taf-topic-tunnel-analysis-grid');
    const activeButton = tabs.querySelector('button.is-active');
    const boardlineRect = rect(boardline);
    const panelRect = rect(panel);
    const bodyRect = rect(body);
    const gridRect = rect(grid);
    const activeButtonRect = rect(activeButton);
    const hit = document.elementFromPoint(
      activeButtonRect.left + activeButtonRect.width / 2,
      activeButtonRect.top + activeButtonRect.height / 2,
    );
    return {
      label: expectedLabel,
      tabId: activeButton.getAttribute('data-tab-id'),
      activeTab: grid.getAttribute('data-active-tab'),
      geometryContract: grid.getAttribute('data-tab-geometry-contract'),
      panel: panelRect,
      body: bodyRect,
      grid: gridRect,
      relative: {
        panelLeft: round(panelRect.left - boardlineRect.left),
        panelTop: round(panelRect.top - boardlineRect.top),
        bodyLeft: round(bodyRect.left - panelRect.left),
        bodyTop: round(bodyRect.top - panelRect.top),
        gridLeft: round(gridRect.left - panelRect.left),
        gridTop: round(gridRect.top - panelRect.top),
      },
      activeButton: activeButtonRect,
      clickTargetMatches: hit === activeButton || activeButton.contains(hit),
      scrollY: round(window.scrollY),
    };
  }, label);
}

async function inspectViewport(context, token, viewport) {
  const page = await context.newPage();
  const productBadResponses = [];
  const requestFailures = [];
  const consoleErrors = [];
  const pageErrors = [];
  page.on('response', (response) => {
    if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
      productBadResponses.push({ status: response.status(), url: response.url() });
    }
  });
  page.on('requestfailed', (request) => {
    if (request.url().startsWith(baseUrl)) {
      requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
    }
  });
  page.on('console', (entry) => {
    if (entry.type() === 'error' && (!entry.location().url || entry.location().url.startsWith(baseUrl))) {
      consoleErrors.push({ text: entry.text(), location: entry.location() });
    }
  });
  page.on('pageerror', (error) => pageErrors.push(error.message));

  await page.setViewportSize(viewport);
  const url = new URL(`/topics?topic=tunnel&tab=tunnel&tunnelGeometryTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  await page.locator('.taf-topic-tunnel-analysis').waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForFunction(() => {
    const pageRoot = document.querySelector('.taf-topic-page');
    return Boolean(pageRoot
      && pageRoot.textContent?.includes('隧道协议数')
      && !pageRoot.textContent?.includes('真实 API 数据加载失败'));
  });
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
  await page.waitForTimeout(400);

  const states = [];
  for (const label of tabLabels) {
    await page.getByRole('tab', { name: label, exact: true }).click();
    await page.waitForFunction(
      ([selector, expected]) => {
        const activeButton = document.querySelector(`${selector} button.is-active`);
        return activeButton?.textContent?.trim() === expected;
      },
      ['.taf-topic-tunnel-analysis-tabs', label],
    );
    await page.waitForTimeout(120);
    states.push(await inspectState(page, label));
  }

  await page.getByRole('tab', { name: '隧道源', exact: true }).click();
  await page.locator('.taf-topic-tunnel-analysis').scrollIntoViewIfNeeded();
  await page.waitForTimeout(180);
  const screenshotPath = path.join(
    evidenceRoot,
    `latest-tunnel-analysis-fixed-geometry-${viewport.width}x${viewport.height}-${revision}.png`,
  );
  fs.mkdirSync(path.dirname(screenshotPath), { recursive: true });
  await page.locator('.taf-topic-tunnel-analysis').screenshot({
    path: screenshotPath,
    timeout: 120_000,
  });

  const geometry = compareGeometry(states, viewport);
  const runtime = {
    productBadResponses,
    requestFailures,
    consoleErrors,
    pageErrors,
    passed: productBadResponses.length === 0
      && requestFailures.length === 0
      && consoleErrors.length === 0
      && pageErrors.length === 0,
  };
  await page.close();
  return {
    ...geometry,
    screenshot: path.relative(root, screenshotPath),
    runtime,
    passed: geometry.passed && runtime.passed,
  };
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP /json/version preflight failed');
const listResponse = await fetch(`${cdpUrl}/json/list`);
if (!listResponse.ok) throw new Error('Windows Chrome CDP /json/list preflight failed');
const version = await versionResponse.json();
const tabs = await listResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const token = smokeToken();

const results = [];
try {
  results.push(await inspectViewport(context, token, { width: 1920, height: 1080 }));
  results.push(await inspectViewport(context, token, { width: 1366, height: 768 }));
} finally {
  await browser.close();
}

const payload = {
  generated_at: new Date().toISOString(),
  source: 'Windows Chrome via Xshell/CDP 127.0.0.1:9224 -> Windows 9222',
  cdp: {
    browser: version.Browser,
    protocol_version: version['Protocol-Version'],
    tab_count: tabs.length,
  },
  revision,
  target: baseUrl,
  results,
  passed: results.length === 2 && results.every((result) => result.passed),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(payload, null, 2)}\n`);
console.log(JSON.stringify(payload, null, 2));
process.exitCode = payload.passed ? 0 : 1;
