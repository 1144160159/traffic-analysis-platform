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
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/attack-chains');
const reportPath = path.join(
  root,
  `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-attack-chain-view-modes-${revision}.json`,
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
  const encode = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const header = encode({ alg: 'HS256', typ: 'JWT' });
  const claims = encode({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-attack-chain-view-acceptance',
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

const modes = [
  {
    name: '泳道视图',
    slug: 'swimlane',
    board: '.taf-attack-swimlane-board',
    phaseTrigger: '.taf-attack-swimlane-row',
    expected: {
      headers: ['攻击阶段', '实体 / 资产', '告警事件', '证据锚点', '处置动作'],
      rows: 6,
      arrows: 24,
    },
  },
  {
    name: '矩阵视图',
    slug: 'matrix',
    board: '.taf-attack-object-matrix',
    phaseTrigger: '.taf-attack-object-matrix-head',
    expected: {
      phaseHeaders: 6,
      laneLabels: ['实体 / 资产', '告警事件', '证据锚点', '处置动作'],
      cells: 24,
    },
  },
];

fs.mkdirSync(evidenceDir, { recursive: true });
fs.mkdirSync(path.dirname(reportPath), { recursive: true });

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const token = createSmokeToken();
const attackChainResponse = await fetch(`${baseUrl}/api/v1/attack-chains?limit=1`, {
  headers: { Authorization: `Bearer ${token}` },
});
const attackChainPayload = await attackChainResponse.json().catch(() => ({}));
const attackChainEnvelope = attackChainPayload?.data ?? attackChainPayload;
const firstAttackChain = Array.isArray(attackChainEnvelope?.chains) ? attackChainEnvelope.chains[0] : null;
const apiSummary = {
  status: attackChainResponse.status,
  total: Number(attackChainEnvelope?.total ?? 0),
  first_chain_id: firstAttackChain?.chain_id ?? null,
  first_phase_count: Array.isArray(firstAttackChain?.phases) ? firstAttackChain.phases.length : 0,
};
const results = [];

try {
  for (const mode of modes) {
    const page = await context.newPage();
    const errors = { responses: [], console: [], page: [], requests: [] };
    page.on('response', (response) => {
      if (response.status() >= 400 && response.url().startsWith(baseUrl)) {
        errors.responses.push({ status: response.status(), url: response.url().replace(/[?#].*$/, '') });
      }
    });
    page.on('console', (entry) => {
      if (entry.type() === 'error') errors.console.push(entry.text());
    });
    page.on('pageerror', (error) => {
      if (!error.message.includes("Cannot read properties of null (reading 'disconnect')")) {
        errors.page.push(error.message);
      }
    });
    page.on('requestfailed', (request) => {
      errors.requests.push({
        method: request.method(),
        url: request.url().replace(/[?#].*$/, ''),
        error: request.failure()?.errorText ?? '',
      });
    });

    try {
      await page.setViewportSize({ width: 1920, height: 1080 });
      const url = new URL('/attack-chains', baseUrl);
      url.searchParams.set('view', mode.name);
      url.searchParams.set('__codex_ui_breakdown_production', '1');
      url.searchParams.set('viewAcceptanceTs', String(Date.now()));
      url.hash = `codex_smoke_token=${token}`;
      await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
      await page.locator('.taf-attack-chain').waitFor({ state: 'visible', timeout: 20_000 });
      await page.waitForLoadState('networkidle', { timeout: 12_000 }).catch(() => {});
      if (new URL(page.url()).pathname !== '/attack-chains') {
        const authenticatedUrl = new URL('/attack-chains', baseUrl);
        authenticatedUrl.searchParams.set('view', mode.name);
        authenticatedUrl.searchParams.set('__codex_ui_breakdown_production', '1');
        authenticatedUrl.searchParams.set('viewAcceptanceTs', String(Date.now()));
        await page.goto(authenticatedUrl.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
        await page.locator('.taf-attack-chain').waitFor({ state: 'visible', timeout: 20_000 });
      }

      await page.locator(mode.phaseTrigger).first().waitFor({ state: 'visible', timeout: 30_000 });
      await page.locator('.taf-attack-evidence-table > button').first().waitFor({ state: 'visible', timeout: 15_000 });
      await page.waitForTimeout(300);

      const firstPhase = page.locator(mode.phaseTrigger).first();
      await firstPhase.click();
      const selected = await firstPhase.getAttribute('aria-pressed');
      await firstPhase.click();
      await page.waitForFunction((selector) => (
        document.querySelector(selector)?.getAttribute('aria-pressed') === 'false'
      ), mode.phaseTrigger);

      const canvas = page.locator('.taf-attack-canvas');
      const board = page.locator(mode.board);
      const scaleBefore = await canvas.evaluate((node) => getComputedStyle(node).getPropertyValue('--attack-chain-scale').trim());
      const boardBefore = await board.boundingBox();
      await page.getByRole('button', { name: '放大攻击链画布' }).click();
      await page.waitForTimeout(250);
      const scaleAfter = await canvas.evaluate((node) => getComputedStyle(node).getPropertyValue('--attack-chain-scale').trim());
      const boardAfter = await board.boundingBox();
      await page.getByRole('button', { name: '缩小攻击链画布' }).click();
      await page.waitForTimeout(250);

      await page.setViewportSize({ width: 1100, height: 900 });
      await page.waitForTimeout(350);
      const narrow = await canvas.evaluate((node) => {
        const style = getComputedStyle(node);
        return {
          clientWidth: node.clientWidth,
          scrollWidth: node.scrollWidth,
          overflowX: style.overflowX,
          pageClientWidth: document.documentElement.clientWidth,
          pageScrollWidth: document.documentElement.scrollWidth,
        };
      });

      await page.setViewportSize({ width: 1920, height: 1080 });
      await page.waitForTimeout(350);
      await page.evaluate(() => {
        window.scrollTo(0, 0);
        const shell = document.querySelector('.taf-attack-shell');
        const canvasNode = document.querySelector('.taf-attack-canvas');
        if (shell instanceof HTMLElement) shell.scrollTo({ top: 0, left: 0, behavior: 'instant' });
        if (canvasNode instanceof HTMLElement) canvasNode.scrollTo({ top: 0, left: 0, behavior: 'instant' });
      });

      const screenshotPath = path.join(evidenceDir, `implementation-${revision}-${mode.slug}.png`);
      await page.screenshot({ path: screenshotPath, fullPage: false });
      const geometry = await page.evaluate(({ boardSelector, modeName }) => {
        const rect = (selector) => {
          const node = document.querySelector(selector);
          if (!node) return null;
          const box = node.getBoundingClientRect();
          return {
            left: box.left,
            right: box.right,
            top: box.top,
            bottom: box.bottom,
            width: box.width,
            height: box.height,
          };
        };
        const canvasNode = document.querySelector('.taf-attack-canvas');
        return {
          mode: canvasNode?.getAttribute('data-view-mode') ?? '',
          selectedMode: document.querySelector('.taf-attack-filters label:last-of-type .ant-select-selection-item')?.textContent?.trim() ?? '',
          canvas: rect('.taf-attack-canvas'),
          board: rect(boardSelector),
          rail: rect('.taf-attack-rail'),
          bottom: rect('.taf-attack-bottom'),
          canvasClientWidth: canvasNode instanceof HTMLElement ? canvasNode.clientWidth : 0,
          canvasScrollWidth: canvasNode instanceof HTMLElement ? canvasNode.scrollWidth : 0,
          bodyClientWidth: document.documentElement.clientWidth,
          bodyScrollWidth: document.documentElement.scrollWidth,
          swimlaneHeaders: [...document.querySelectorAll('.taf-attack-swimlane-header strong')].map((node) => node.textContent?.trim()),
          swimlaneRows: document.querySelectorAll('.taf-attack-swimlane-row').length,
          swimlaneArrows: document.querySelectorAll('.taf-attack-swimlane-arrow').length,
          matrixHeaders: document.querySelectorAll('.taf-attack-object-matrix-head').length,
          matrixLabels: [...document.querySelectorAll('.taf-attack-object-matrix-label')].map((node) => node.textContent?.trim()),
          matrixCells: document.querySelectorAll('.taf-attack-object-matrix-cell').length,
          visibleMode: modeName,
        };
      }, { boardSelector: mode.board, modeName: mode.name });

      const checks = [
        { name: `${mode.name} is the selected and rendered view`, passed: geometry.mode === mode.name && geometry.selectedMode === mode.name },
        { name: `${mode.name} keeps the canvas, bottom panels and right rail visible`, passed: Boolean(geometry.canvas && geometry.board && geometry.rail && geometry.bottom && geometry.canvas.height >= 480 && geometry.rail.width >= 360) },
        { name: `${mode.name} phase selection state is interactive`, passed: selected === 'true' },
        { name: `${mode.name} is backed by a six-phase database chain`, passed: apiSummary.status === 200 && apiSummary.first_phase_count === 6 },
        { name: `${mode.name} zoom changes the board geometry`, passed: Number(scaleAfter) > Number(scaleBefore) && Boolean(boardBefore && boardAfter && boardAfter.width > boardBefore.width) },
        { name: `${mode.name} supports canvas-only horizontal scrolling at narrow width`, passed: ['auto', 'scroll'].includes(narrow.overflowX) && narrow.scrollWidth > narrow.clientWidth && narrow.pageScrollWidth <= narrow.pageClientWidth + 2 },
        { name: `${mode.name} has no page-level horizontal overflow at 1920`, passed: geometry.bodyScrollWidth <= geometry.bodyClientWidth + 2 },
        { name: `${mode.name} has no runtime errors`, passed: Object.values(errors).every((items) => items.length === 0) },
      ];
      if (mode.slug === 'swimlane') {
        checks.push(
          { name: 'swimlane has the five source-UI business columns', passed: JSON.stringify(geometry.swimlaneHeaders) === JSON.stringify(mode.expected.headers) },
          { name: 'swimlane has six database-backed phase rows', passed: geometry.swimlaneRows === mode.expected.rows },
          { name: 'swimlane arrows connect all five business columns', passed: geometry.swimlaneArrows === mode.expected.arrows },
        );
      } else {
        checks.push(
          { name: 'matrix has six source-UI phase headers', passed: geometry.matrixHeaders === mode.expected.phaseHeaders },
          { name: 'matrix has the four source-UI object lanes', passed: JSON.stringify(geometry.matrixLabels) === JSON.stringify(mode.expected.laneLabels) },
          { name: 'matrix has a complete 4 x 6 object-stage grid', passed: geometry.matrixCells === mode.expected.cells },
        );
      }

      results.push({
        mode: mode.name,
        screenshot: path.relative(root, screenshotPath),
        geometry,
        interaction: {
          selected,
          scaleBefore,
          scaleAfter,
          boardBefore,
          boardAfter,
          narrow,
        },
        errors,
        checks,
        result: checks.every((check) => check.passed) ? 'pass' : 'fail',
      });
    } catch (error) {
      const diagnosticPath = path.join(evidenceDir, `implementation-${revision}-${mode.slug}-failure.png`);
      await page.screenshot({ path: diagnosticPath, fullPage: false }).catch(() => {});
      results.push({
        mode: mode.name,
        screenshot: path.relative(root, diagnosticPath),
        route: page.url(),
        visible_text: (await page.locator('body').innerText().catch(() => '')).slice(0, 4000),
        errors,
        checks: [{ name: `${mode.name} acceptance completed`, passed: false }],
        failure: error instanceof Error ? error.message : String(error),
        result: 'fail',
      });
    } finally {
      await page.close().catch(() => {});
    }
  }
} finally {
  const report = {
    run_id: `attack-chain-view-modes-${revision}`,
    generated_at: new Date().toISOString(),
    browser_backend: 'Windows Chrome CDP over Xshell 9224 -> 9222',
    browser: version.Browser,
    route: '/attack-chains',
    source_visual_truth: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png',
    api_summary: apiSummary,
    results,
    result: results.length === modes.length && results.every((item) => item.result === 'pass') ? 'pass' : 'fail',
  };
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(JSON.stringify({ report: path.relative(root, reportPath), ...report }, null, 2));
}

process.exit(results.length === modes.length && results.every((item) => item.result === 'pass') ? 0 : 1);
