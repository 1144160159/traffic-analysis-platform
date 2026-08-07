#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');

function parseArgs(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    if (!key.startsWith('--') || index + 1 >= argv.length) throw new Error(`invalid argument: ${key}`);
    result[key.slice(2).replace(/-([a-z])/gu, (_, letter) => letter.toUpperCase())] = argv[index + 1];
    index += 1;
  }
  return result;
}

const args = {
  baseUrl: 'http://10.0.5.8:30185',
  cdpUrl: 'http://127.0.0.1:9224',
  runId: '',
  outputJson: '',
  evidenceDir: '',
  ...parseArgs(process.argv.slice(2)),
};
if (!args.runId || !args.outputJson || !args.evidenceDir) throw new Error('--run-id, --output-json and --evidence-dir are required');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = process.env.NO_PROXY || '127.0.0.1,localhost,10.0.5.8';

const outputPath = path.resolve(root, args.outputJson);
const evidenceDirectory = path.resolve(root, args.evidenceDir);
if (fs.existsSync(outputPath) || fs.existsSync(evidenceDirectory)) throw new Error('refusing to overwrite immutable Product Design evidence');
fs.mkdirSync(evidenceDirectory, { recursive: true });

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

function makeSmokeToken() {
  const encoded = execFileSync('kubectl', [
    '-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials',
    '-o', 'jsonpath={.data.JWT_SECRET}',
  ], { encoding: 'utf8', env: process.env, timeout: 15_000 });
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: 'default',
    username: 'codex-product-design-topic-auditor',
    roles: ['admin'],
    permissions: ['*', 'admin:*', 'topic:read', 'audit:read'],
    token_type: 'access',
    session_id: `topic-product-design-${args.runId}`,
    iat: now,
    exp: now + 3600,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const signature = crypto.createHmac('sha256', secret).update(input).digest('base64url');
  return `${input}.${signature}`;
}

function redact(value) {
  return String(value).replace(/codex_smoke_token=[^&#]+/gu, 'codex_smoke_token=<redacted>');
}

const references = {
  tunnel: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-encrypted-tunnel.png',
  exfil: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-data-exfiltration.png',
  apt: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-apt-campaign.png',
};
const viewports = [
  { width: 1920, height: 1080 },
  { width: 1366, height: 768 },
];

const versionResponse = await fetch(`${args.cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(args.cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const token = makeSmokeToken();
const routes = [];

try {
  for (const topic of ['tunnel', 'exfil', 'apt']) {
    for (const viewport of viewports) {
      const badResponses = [];
      const requestFailures = [];
      const consoleErrors = [];
      const pageErrors = [];
      let snapshotPayload = null;
      const onResponse = async (response) => {
        const url = response.url();
        if (url.startsWith(`${args.baseUrl}/api/`) && response.status() >= 400) {
          badResponses.push({ method: response.request().method(), status: response.status(), url });
        }
        if (response.request().method() === 'GET' && url.includes(`/api/v1/topics/${topic}/snapshot`) && response.ok()) {
          snapshotPayload = await response.json().catch(() => null);
        }
      };
      const onRequestFailed = (request) => {
        if (request.url().startsWith(args.baseUrl)) requestFailures.push({ url: request.url(), error: request.failure()?.errorText ?? '' });
      };
      const onConsole = (entry) => {
        if (entry.type() === 'error' && entry.location().url?.startsWith(args.baseUrl)) {
          consoleErrors.push({ text: entry.text(), location: entry.location() });
        }
      };
      const onPageError = (error) => pageErrors.push({ message: error.message });
      page.on('response', onResponse);
      page.on('requestfailed', onRequestFailed);
      page.on('console', onConsole);
      page.on('pageerror', onPageError);

      await page.setViewportSize(viewport);
      const url = new URL(`/topics?topic=${topic}&productDesignAuditTs=${Date.now()}`, args.baseUrl);
      url.hash = `codex_smoke_token=${token}`;
      await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 45_000 });
      await page.locator(`.taf-topic-${topic}-layout`).waitFor({ state: 'visible', timeout: 20_000 });
      await page.waitForFunction((expected) => {
        const host = document.querySelector('.taf-topic-page');
        return Boolean(host && !host.textContent?.includes('真实 API 数据加载失败') && host.textContent?.includes(expected));
      }, topic === 'tunnel' ? '隧道协议数' : topic === 'exfil' ? '外传路径数' : '关联战役数', { timeout: 20_000 });
      await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
      await page.waitForTimeout(500);

      const geometry = await page.evaluate(() => {
        const visible = (element) => {
          const rect = element.getBoundingClientRect();
          const style = getComputedStyle(element);
          return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
        };
        const metricRoots = [...document.querySelectorAll('.taf-topic-kpis .taf-metric, .taf-topic-tunnel-kpis .taf-topic-tunnel-kpi')]
          .filter(visible);
        const metricText = metricRoots.map((element) => element.textContent?.replace(/\s+/gu, ' ').trim() ?? '');
        const clippedMetricText = metricRoots.flatMap((root) => [...root.querySelectorAll('strong, b, span, small')]
          .filter((element) => visible(element) && (element.textContent?.trim() ?? '') !== '' && element.scrollWidth > element.clientWidth + 1)
          .map((element) => ({
            text: element.textContent?.replace(/\s+/gu, ' ').trim() ?? '',
            client_width: element.clientWidth,
            scroll_width: element.scrollWidth,
          })));
        const topology = document.querySelector('.taf-topic-page .taf-api-topology[data-api-dynamic="true"]');
        const root = document.scrollingElement ?? document.documentElement;
        const errorAlerts = [...document.querySelectorAll('.taf-topic-page .ant-alert-error, .taf-topic-page [role="alert"]')]
          .filter(visible)
          .map((element) => element.textContent?.replace(/\s+/gu, ' ').trim() ?? '');
        return {
          metric_text: metricText,
          clipped_metric_text: clippedMetricText,
          document_horizontal_overflow: root.scrollWidth > root.clientWidth + 2,
          error_alerts: errorAlerts,
          topology: topology ? {
            node_count: Number(topology.getAttribute('data-node-count')),
            link_count: Number(topology.getAttribute('data-link-count')),
            duplicate_node_count: Number(topology.getAttribute('data-duplicate-node-count')),
            dangling_link_count: Number(topology.getAttribute('data-dangling-link-count')),
            self_link_count: Number(topology.getAttribute('data-self-link-count')),
            node_overlap_count: Number(topology.getAttribute('data-node-overlap-count')),
            node_content_overflow_count: Number(topology.getAttribute('data-node-content-overflow-count')),
            node_inset_violation_count: Number(topology.getAttribute('data-node-inset-violation-count')),
            node_layout_capacity_violation_count: Number(topology.getAttribute('data-node-layout-capacity-violation-count')),
            chart_size: topology.getAttribute('data-chart-size'),
            visual_profile: topology.getAttribute('data-visual-profile'),
          } : null,
        };
      });

      const screenshot = path.join(evidenceDirectory, `${topic}-${viewport.width}x${viewport.height}.png`);
      await page.screenshot({ path: screenshot, fullPage: false, timeout: 60_000 });
      const data = snapshotPayload?.data ?? null;
      const contract = {
        snapshot_id: snapshotPayload?.meta?.snapshot_id ?? '',
        trace_id: snapshotPayload?.meta?.trace_id ?? '',
        partial: snapshotPayload?.meta?.partial ?? null,
        missing_sections: snapshotPayload?.meta?.missing_sections ?? [],
        topology_nodes_array: topic === 'tunnel' ? true : Array.isArray(data?.topology_nodes),
        topology_links_array: topic === 'tunnel' ? true : Array.isArray(data?.topology_links),
      };
      const topologyHealthy = !geometry.topology || (
        geometry.topology.node_count > 0
        && geometry.topology.link_count > 0
        && geometry.topology.duplicate_node_count === 0
        && geometry.topology.dangling_link_count === 0
        && geometry.topology.self_link_count === 0
        && geometry.topology.node_overlap_count === 0
        && geometry.topology.node_content_overflow_count === 0
        && geometry.topology.node_inset_violation_count === 0
        && geometry.topology.node_layout_capacity_violation_count === 0
      );
      const passed = badResponses.length === 0
        && requestFailures.length === 0
        && consoleErrors.length === 0
        && pageErrors.length === 0
        && geometry.clipped_metric_text.length === 0
        && !geometry.document_horizontal_overflow
        && geometry.error_alerts.length === 0
        && geometry.metric_text.length > 0
        && Boolean(contract.snapshot_id)
        && Boolean(contract.trace_id)
        && contract.topology_nodes_array
        && contract.topology_links_array
        && topologyHealthy;
      routes.push({
        topic,
        viewport,
        url: redact(page.url()),
        screenshot: path.relative(root, screenshot),
        screenshot_sha256: sha256(screenshot),
        reference_image: references[topic],
        reference_sha256: sha256(path.resolve(root, references[topic])),
        bad_responses: badResponses,
        request_failures: requestFailures,
        console_errors: consoleErrors,
        page_errors: pageErrors,
        geometry,
        contract,
        passed,
      });
      page.off('response', onResponse);
      page.off('requestfailed', onRequestFailed);
      page.off('console', onConsole);
      page.off('pageerror', onPageError);
    }
  }
} finally {
  await page.setViewportSize({ width: 1920, height: 1080 }).catch(() => {});
  await page.close().catch(() => {});
  await browser.close().catch(() => {});
}

const result = routes.length === 6 && routes.every((route) => route.passed) ? 'PASS' : 'FAIL';
const report = {
  schema_version: 1,
  run_id: args.runId,
  gate: 'PRODUCT_DESIGN_TOPIC_READONLY_AUDIT',
  result,
  generated_at: new Date().toISOString(),
  browser: version.Browser,
  browser_path: 'Windows Chrome via Xshell/CDP 127.0.0.1:9224',
  base_url: args.baseUrl,
  read_only: true,
  production_mutations: [],
  routes,
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify({ result, output: path.relative(root, outputPath), routes: routes.map(({ topic, viewport, passed }) => ({ topic, viewport, passed })) }, null, 2));
process.exit(result === 'PASS' ? 0 : 1);
