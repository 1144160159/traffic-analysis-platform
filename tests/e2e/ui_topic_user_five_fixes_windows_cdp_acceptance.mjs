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
const revision = process.env.TOPIC_REVISION ?? 'topic-panel-r859-r860-user-five-fixes';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/topics-user-five-fixes-${revision}.json`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
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
    username: 'codex-topic-user-five-fixes-admin',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'topic:write', 'topic:export', 'audit:read'],
    token_type: 'access',
    session_id: `codex-topic-five-fixes-${revision}`,
    iat: now,
    exp: now + 3_600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

function rect(element) {
  const value = element.getBoundingClientRect();
  return {
    left: Math.round(value.left * 100) / 100,
    top: Math.round(value.top * 100) / 100,
    right: Math.round(value.right * 100) / 100,
    bottom: Math.round(value.bottom * 100) / 100,
    width: Math.round(value.width * 100) / 100,
    height: Math.round(value.height * 100) / 100,
  };
}

function intersects(a, b, tolerance = 0.5) {
  return Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left)) > tolerance
    && Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top)) > tolerance;
}

function inside(child, parent, tolerance = 0.5) {
  return child.left >= parent.left - tolerance
    && child.top >= parent.top - tolerance
    && child.right <= parent.right + tolerance
    && child.bottom <= parent.bottom + tolerance;
}

async function openTopic(page, token, topic, viewport) {
  await page.setViewportSize(viewport);
  const url = new URL(`/topics?topic=${topic}&tab=${topic}&fiveFixesTs=${Date.now()}`, baseUrl);
  url.hash = `codex_smoke_token=${token}`;
  await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
  const layout = `.taf-topic-${topic === 'apt' ? 'apt' : 'exfil'}-layout`;
  await page.locator(layout).waitFor({ state: 'visible', timeout: 20_000 });
  await page.waitForFunction((key) => {
    const pageElement = document.querySelector('.taf-topic-page');
    return Boolean(pageElement
      && !pageElement.textContent?.includes('真实 API 数据加载失败')
      && pageElement.textContent?.includes(key));
  }, topic === 'apt' ? '关联战役数' : '外传路径数', { timeout: 20_000 });
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
  await page.waitForTimeout(500);
}

async function capture(page, directory, filename, fullPage = false) {
  const file = path.join(root, `evidence/ui-image-breakdowns/pages/${directory}/${filename}-${revision}.png`);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  await page.screenshot({ path: file, fullPage });
  return path.relative(root, file);
}

async function sampleExfilGeometry(page) {
  return page.evaluate(() => {
    const selectors = {
      main: '.taf-topic-exfil-main',
      board: '.taf-topic-exfil-boardline',
      table: '.taf-topic-exfil-table-panel',
      canvas: '.taf-topic-exfil-canvas',
      topology: '.taf-topic-exfil .taf-api-topology',
      page: '.taf-topic-page',
    };
    const bounds = {};
    for (const [key, selector] of Object.entries(selectors)) {
      const element = document.querySelector(selector);
      bounds[key] = element ? {
        top: Math.round(element.getBoundingClientRect().top * 100) / 100,
        bottom: Math.round(element.getBoundingClientRect().bottom * 100) / 100,
        width: Math.round(element.getBoundingClientRect().width * 100) / 100,
        height: Math.round(element.getBoundingClientRect().height * 100) / 100,
      } : null;
    }
    return {
      timestamp: performance.now(),
      document_height: document.documentElement.scrollHeight,
      body_height: document.body.scrollHeight,
      bounds,
    };
  });
}

function stableSeries(samples, key, tolerance = 2) {
  const values = samples.map((sample) => {
    if (key === 'document_height' || key === 'body_height') return sample[key];
    return sample.bounds[key]?.height ?? Number.NaN;
  });
  const finite = values.filter(Number.isFinite);
  const delta = finite.length ? Math.max(...finite) - Math.min(...finite) : Number.POSITIVE_INFINITY;
  const monotonicGrowth = finite.length >= 3 && finite.every((value, index) => index === 0 || value > finite[index - 1] + tolerance);
  return { values, delta: Math.round(delta * 100) / 100, monotonic_growth: monotonicGrowth, passed: delta <= tolerance && !monotonicGrowth };
}

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error('Windows Chrome CDP preflight failed');
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
for (const stalePage of context.pages()) {
  if (stalePage.url().includes('fiveFixesTs=')) await stalePage.close().catch(() => {});
}
const page = await context.newPage();
const token = smokeToken();
const productBadResponses = [];
const requestFailures = [];
const consoleErrors = [];
const pageErrors = [];

page.on('response', async (response) => {
  if (response.url().startsWith(`${baseUrl}/api/`) && response.status() >= 400) {
    productBadResponses.push({
      status: response.status(),
      url: response.url(),
      body: (await response.text().catch(() => '')).slice(0, 800),
    });
  }
});
page.on('requestfailed', (request) => {
  if (request.url().startsWith(baseUrl)) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
});
page.on('console', (entry) => {
  if (entry.type() === 'error' && (!entry.location().url || entry.location().url.startsWith(baseUrl))) {
    consoleErrors.push({ text: entry.text(), location: entry.location() });
  }
});
page.on('pageerror', (error) => pageErrors.push({ message: error.message }));
await page.route('https://api.yhchj.com/ip', (route) => route.fulfill({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify({ ip: '127.0.0.1', source: 'windows-cdp-five-fixes' }),
}));

let exitCode = 1;
try {
  await openTopic(page, token, 'apt', { width: 1920, height: 1080 });

  const topology = page.locator('.taf-topic-apt-canvas .taf-api-topology');
  const topologyBefore = await topology.evaluate((element) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    return {
      bounds: toRect(element),
      overflow: getComputedStyle(element).overflow,
      zoom: element.getAttribute('data-zoom'),
      canvas: toRect(element.querySelector('canvas')),
    };
  });
  const zoomButton = topology.getByTitle('放大关系图');
  for (let index = 0; index < 5; index += 1) await zoomButton.click();
  await page.waitForTimeout(500);
  const topologyAfter = await topology.evaluate((element) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    return {
      bounds: toRect(element),
      overflow: getComputedStyle(element).overflow,
      zoom: Number(element.getAttribute('data-zoom')),
      canvas: toRect(element.querySelector('canvas')),
    };
  });
  const topologyScreenshot = await capture(page, 'topics-apt-campaign', 'user-fix-1-apt-zoom-label-bounds');
  const topologyGeometryStable = Math.abs(topologyAfter.bounds.width - topologyBefore.bounds.width) <= 1
    && Math.abs(topologyAfter.bounds.height - topologyBefore.bounds.height) <= 1
    && inside(topologyAfter.canvas, topologyAfter.bounds, 1);
  const topologyPass = topologyAfter.zoom === 2
    && topologyAfter.overflow === 'hidden'
    && topologyGeometryStable;

  const donut = await page.locator('.taf-topic-apt-response-chart').evaluate((host) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const isInside = (child, parent, tolerance = 0.5) => child.left >= parent.left - tolerance
      && child.top >= parent.top - tolerance
      && child.right <= parent.right + tolerance
      && child.bottom <= parent.bottom + tolerance;
    const isIntersecting = (left, right, tolerance = 0.5) =>
      Math.max(0, Math.min(left.right, right.right) - Math.max(left.left, right.left)) > tolerance
      && Math.max(0, Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top)) > tolerance;
    const hostBounds = toRect(host);
    const totalBounds = toRect(host.querySelector('strong'));
    const labelBounds = toRect(host.querySelector('span'));
    const chartBounds = toRect(host.querySelector('.taf-topic-apt-response-echart'));
    const centerSafeBounds = {
      left: chartBounds.left + chartBounds.width * 0.22,
      right: chartBounds.right - chartBounds.width * 0.22,
      top: chartBounds.top + chartBounds.height * 0.22,
      bottom: chartBounds.bottom - chartBounds.height * 0.22,
    };
    return {
      host: hostBounds,
      chart: chartBounds,
      total: totalBounds,
      label: labelBounds,
      center_safe_bounds: centerSafeBounds,
      total_label_overlap: isIntersecting(totalBounds, labelBounds),
      total_inside_host: isInside(totalBounds, hostBounds),
      label_inside_host: isInside(labelBounds, hostBounds),
      total_centered_in_hole: isInside(totalBounds, centerSafeBounds),
      label_centered_in_hole: isInside(labelBounds, centerSafeBounds),
    };
  });
  donut.passed = !donut.total_label_overlap
    && donut.total_inside_host
    && donut.label_inside_host
    && donut.total_centered_in_hole
    && donut.label_centered_in_hole;
  const donutScreenshot = await capture(page, 'topics-apt-campaign', 'user-fix-2-apt-response-donut');

  const reportSheet = await page.locator('.taf-topic-apt-report .taf-topic-report-sheet').evaluate((sheet) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const isInside = (child, parent, tolerance = 0.5) => child.left >= parent.left - tolerance
      && child.top >= parent.top - tolerance
      && child.right <= parent.right + tolerance
      && child.bottom <= parent.bottom + tolerance;
    const isIntersecting = (left, right, tolerance = 0.5) =>
      Math.max(0, Math.min(left.right, right.right) - Math.max(left.left, right.left)) > tolerance
      && Math.max(0, Math.min(left.bottom, right.bottom) - Math.max(left.top, right.top)) > tolerance;
    const sheetBounds = toRect(sheet);
    const children = [...sheet.children].map((element) => ({
      tag: element.tagName.toLowerCase(),
      text: element.textContent?.trim() ?? '',
      bounds: toRect(element),
      overflow: getComputedStyle(element).overflow,
      white_space: getComputedStyle(element).whiteSpace,
    }));
    const overlaps = [];
    for (let left = 0; left < children.length; left += 1) {
      for (let right = left + 1; right < children.length; right += 1) {
        if (isIntersecting(children[left].bounds, children[right].bounds)) {
          overlaps.push([children[left].tag, children[right].tag]);
        }
      }
    }
    return {
      sheet: sheetBounds,
      children,
      overlaps,
      all_inside: children.every((child) => isInside(child.bounds, sheetBounds)),
      progress_height: children.find((child) => child.tag === 'i')?.bounds.height ?? 0,
    };
  });
  reportSheet.passed = reportSheet.all_inside && reportSheet.overlaps.length === 0 && reportSheet.progress_height <= 5.5;
  const reportScreenshot = await capture(page, 'topics-apt-campaign', 'user-fix-5-report-thumbnail');

  const responsive = [];
  for (const viewport of [
    { width: 1920, height: 768 },
    { width: 1600, height: 900 },
    { width: 1366, height: 768 },
  ]) {
    await openTopic(page, token, 'exfil', viewport);
    const samples = [];
    for (let index = 0; index < 7; index += 1) {
      samples.push(await sampleExfilGeometry(page));
      await page.waitForTimeout(500);
    }
    const stability = {};
    for (const key of ['document_height', 'body_height', 'main', 'board', 'table', 'canvas', 'topology', 'page']) {
      stability[key] = stableSeries(samples.slice(2), key);
    }
    const screenshot = await capture(
      page,
      'topics-data-exfiltration',
      `user-fix-3-height-stable-${viewport.width}x${viewport.height}`,
    );
    responsive.push({
      viewport,
      samples,
      stability,
      screenshot,
      passed: Object.values(stability).every((item) => item.passed),
    });
  }

  await openTopic(page, token, 'exfil', { width: 1920, height: 1080 });
  const actions = await page.locator('.taf-topic-exfil-row-actions').first().evaluate((host) => {
    const toRect = (target) => {
      const value = target.getBoundingClientRect();
      return {
        left: Math.round(value.left * 100) / 100,
        top: Math.round(value.top * 100) / 100,
        right: Math.round(value.right * 100) / 100,
        bottom: Math.round(value.bottom * 100) / 100,
        width: Math.round(value.width * 100) / 100,
        height: Math.round(value.height * 100) / 100,
      };
    };
    const isInside = (child, parent, tolerance = 0.5) => child.left >= parent.left - tolerance
      && child.top >= parent.top - tolerance
      && child.right <= parent.right + tolerance
      && child.bottom <= parent.bottom + tolerance;
    const hostBounds = toRect(host);
    const buttons = [...host.querySelectorAll('button')].map((button) => {
      const style = getComputedStyle(button);
      return {
        label: button.textContent?.trim() ?? '',
        bounds: toRect(button),
        background: style.backgroundColor,
        border: style.borderColor,
        color: style.color,
      };
    });
    const topSpread = buttons.length
      ? Math.max(...buttons.map((button) => button.bounds.top)) - Math.min(...buttons.map((button) => button.bounds.top))
      : Number.POSITIVE_INFINITY;
    const whiteButtons = buttons.filter((button) => /rgb\(255,\s*255,\s*255\)/u.test(button.background));
    return {
      host: hostBounds,
      buttons,
      labels: buttons.map((button) => button.label),
      top_spread: Math.round(topSpread * 100) / 100,
      white_button_count: whiteButtons.length,
      all_inside: buttons.every((button) => isInside(button.bounds, hostBounds, 1)),
    };
  });
  const expectedLabels = ['PCAP', 'Session', '文件摘要', '回溯路径', '审计日志'];
  actions.passed = actions.buttons.length === 5
    && expectedLabels.every((label) => actions.labels.includes(label))
    && actions.top_spread <= 2
    && actions.white_button_count === 0
    && actions.all_inside;
  const actionsScreenshot = await capture(page, 'topics-data-exfiltration', 'user-fix-4-horizontal-action-controls');

  const checks = {
    apt_topology_zoom_label_bounds: {
      before: topologyBefore,
      after: topologyAfter,
      geometry_stable: topologyGeometryStable,
      screenshot: topologyScreenshot,
      passed: topologyPass,
    },
    apt_response_donut_text: { ...donut, screenshot: donutScreenshot },
    exfil_responsive_height_stability: {
      viewports: responsive,
      passed: responsive.every((item) => item.passed),
    },
    exfil_action_controls: { ...actions, screenshot: actionsScreenshot },
    report_thumbnail_typography: { ...reportSheet, screenshot: reportScreenshot },
  };
  const result = Object.values(checks).every((check) => check.passed)
    && productBadResponses.length === 0
    && requestFailures.length === 0
    && consoleErrors.length === 0
    && pageErrors.length === 0;
  const payload = {
    result: result ? 'pass' : 'fail',
    browser_backend: 'Windows Chrome CDP through Xshell 9224 -> 9222',
    browser: version.Browser,
    revision,
    checks,
    product_bad_responses: productBadResponses,
    request_failures: requestFailures,
    console_errors: consoleErrors,
    page_errors: pageErrors,
    timestamp: new Date().toISOString(),
  };
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(payload, null, 2)}\n`);
  console.log(JSON.stringify(payload, null, 2));
  exitCode = result ? 0 : 1;
} finally {
  await page.close().catch(() => {});
  await browser.close().catch(() => {});
}

process.exit(exitCode);
