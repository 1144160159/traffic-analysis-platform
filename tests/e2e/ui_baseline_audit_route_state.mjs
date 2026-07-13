#!/usr/bin/env node

import { createRequire } from 'node:module';
import path from 'node:path';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = String(process.env.UI_BASE_URL || 'http://127.0.0.1:5173').replace(/\/+$/, '');

const baselineStates = [
  ['account', '账号基线'],
  ['port', '端口基线'],
  ['protocol', '协议基线'],
  ['time-window', '时间段基线'],
];
const auditStates = [
  ['operation-context', '操作上下文'],
  ['related-chain', '关联链路'],
];

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
const page = await context.newPage();
const consoleErrors = [];
const pageErrors = [];
const requestFailures = [];
const badResponses = [];

page.on('console', (message) => {
  if (message.type() === 'error') consoleErrors.push(message.text());
});
page.on('pageerror', (error) => pageErrors.push(error.message));
page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
page.on('response', (response) => {
  if (response.status() >= 400) badResponses.push(`${response.status()} ${response.url()}`);
});

const checks = [];
for (const [slug, label] of baselineStates) {
  await page.goto(`${baseUrl}/baselines?tab=${slug}`, { waitUntil: 'networkidle' });
  const activeLabel = (await page.locator('.taf-baseline-tabs .ant-tabs-tab-active').innerText()).trim();
  checks.push({ id: `baselines-${slug}`, expected: label, actual: activeLabel, passed: activeLabel === label });
}

for (const [slug, label] of auditStates) {
  await page.goto(`${baseUrl}/audit-log?detail=${slug}`, { waitUntil: 'networkidle' });
  const activeLabel = (await page.locator('.taf-auditlog-detail-tabs button.is-active').innerText()).trim();
  const stateVisible = await page.locator(`[data-audit-detail-state="${slug}"]`).isVisible();
  checks.push({ id: `audit-log-${slug}`, expected: label, actual: activeLabel, state_visible: stateVisible, passed: activeLabel === label && stateVisible });
}

await browser.close();

const report = {
  status: checks.every((item) => item.passed) && consoleErrors.length === 0 && pageErrors.length === 0 && requestFailures.length === 0 && badResponses.length === 0 ? 'pass' : 'fail',
  base_url: baseUrl,
  checks,
  console_errors: consoleErrors,
  page_errors: pageErrors,
  request_failures: requestFailures,
  bad_responses: badResponses,
};
console.log(JSON.stringify(report, null, 2));
if (report.status !== 'pass') process.exitCode = 1;
