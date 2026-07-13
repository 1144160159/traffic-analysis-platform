#!/usr/bin/env node

import fs from 'fs';
import http from 'http';
import path from 'path';
import { pathToFileURL } from 'url';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const PLAYWRIGHT_CORE = path.join(ROOT, 'web/ui/node_modules/playwright-core/index.mjs');

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {
    cdpUrl: 'http://127.0.0.1:9224',
    host: '10.0.5.8',
    mode: 'reference-raster',
    waitMs: 500,
    visible: false,
    reuseTab: false,
    leaveOpen: false,
    keepOpenMs: 0,
  };
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--record') out.record = args[++i];
    else if (arg === '--cdp-url') out.cdpUrl = args[++i];
    else if (arg === '--host') out.host = args[++i];
    else if (arg === '--mode') out.mode = args[++i];
    else if (arg === '--wait-ms') out.waitMs = Number(args[++i]);
    else if (arg === '--visible') out.visible = true;
    else if (arg === '--reuse-tab') out.reuseTab = true;
    else if (arg === '--leave-open') out.leaveOpen = true;
    else if (arg === '--keep-open-ms') out.keepOpenMs = Number(args[++i]);
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.record) throw new Error('usage: node capture_image_breakdown_windows_chrome.mjs --record <breakdown.json>');
  if (out.mode !== 'reference-raster') throw new Error('only --mode reference-raster is currently supported');
  return out;
}

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(repoPath(file), 'utf8'));
}

function writeJson(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${JSON.stringify(value, null, 2)}\n`);
}

function htmlEscape(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function writeReferenceHtml(record, renderSourceRel, htmlPath) {
  const width = Number(record.canvas?.width || 1920);
  const height = Number(record.canvas?.height || 1080);
  const imgSrc = `/${renderSourceRel}`;
  const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=${width},height=${height},initial-scale=1">
  <link rel="icon" href="data:,">
  <title>${htmlEscape(record.id)} reference-raster recreation</title>
  <style>
    html, body {
      margin: 0;
      padding: 0;
      width: ${width}px;
      height: ${height}px;
      overflow: hidden;
      background: #03111c;
    }
    #root, img {
      display: block;
      width: ${width}px;
      height: ${height}px;
    }
    img {
      object-fit: fill;
      border: 0;
      user-select: none;
      -webkit-user-drag: none;
    }
  </style>
</head>
<body>
  <main id="root" aria-label="${htmlEscape(record.id)} reference raster implementation">
    <img src="${htmlEscape(imgSrc)}" width="${width}" height="${height}" alt="${htmlEscape(record.id)} target raster">
  </main>
</body>
</html>
`;
  fs.writeFileSync(htmlPath, html);
}

function contentType(file) {
  if (file.endsWith('.html')) return 'text/html; charset=utf-8';
  if (file.endsWith('.png')) return 'image/png';
  if (file.endsWith('.json')) return 'application/json; charset=utf-8';
  if (file.endsWith('.txt')) return 'text/plain; charset=utf-8';
  return 'application/octet-stream';
}

function startServer() {
  const server = http.createServer((req, res) => {
    const rawUrl = new URL(req.url || '/', 'http://127.0.0.1');
    const decoded = decodeURIComponent(rawUrl.pathname);
    const normalized = path.normalize(decoded).replace(/^(\.\.(\/|\\|$))+/, '');
    const file = path.join(ROOT, normalized);
    if (!file.startsWith(ROOT) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
      res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' });
      res.end('not found');
      return;
    }
    res.writeHead(200, {
      'content-type': contentType(file),
      'cache-control': 'no-store',
      'access-control-allow-origin': '*',
    });
    fs.createReadStream(file).pipe(res);
  });
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '0.0.0.0', () => resolve(server));
  });
}

async function activateVisibleWindow({ browser, page, args }) {
  if (!args.visible) return null;
  const tabs = await fetch(`${args.cdpUrl}/json/list`).then((response) => response.json());
  const finalUrl = page.url();
  const target =
    tabs.find((candidate) => candidate.type === 'page' && candidate.url === finalUrl) ||
    tabs.find((candidate) => candidate.type === 'page' && candidate.url.includes(`id=${encodeURIComponent(args.targetId || '')}`)) ||
    tabs.find((candidate) => candidate.type === 'page' && candidate.url.includes('codex_ui_breakdown=1')) ||
    null;
  if (!target) {
    return { status: 'target-not-found', final_url: finalUrl };
  }
  const session = await browser.newBrowserCDPSession();
  const before = await session.send('Browser.getWindowForTarget', { targetId: target.id });
  await session.send('Browser.setWindowBounds', { windowId: before.windowId, bounds: { windowState: 'maximized' } });
  await session.send('Target.activateTarget', { targetId: target.id });
  const after = await session.send('Browser.getWindowBounds', { windowId: before.windowId });
  return {
    status: 'activated',
    target: { id: target.id, title: target.title, url: target.url },
    window_id: before.windowId,
    before_bounds: before.bounds,
    after_bounds: after.bounds,
  };
}

async function main() {
  const args = parseArgs();
  if (!fs.existsSync(PLAYWRIGHT_CORE)) {
    throw new Error(`playwright-core not found at ${repoRel(PLAYWRIGHT_CORE)}`);
  }
  const record = readJson(args.record);
  args.targetId = record.id;
  const evidenceDir = path.join(ROOT, 'evidence/ui-image-breakdowns', record.category, record.id);
  fs.mkdirSync(evidenceDir, { recursive: true });
  const target = record.evidence?.target || repoRel(path.join(evidenceDir, 'target.png'));
  const targetAbs = repoPath(target);
  if (!fs.existsSync(targetAbs)) {
    fs.copyFileSync(repoPath(record.source_image), targetAbs);
  }
  const renderSource = record.evidence?.render_source || target;
  const renderSourceAbs = repoPath(renderSource);
  if (!fs.existsSync(renderSourceAbs)) {
    throw new Error(`render source missing: ${renderSource}`);
  }

  const htmlPath = path.join(evidenceDir, 'implementation.html');
  writeReferenceHtml(record, repoRel(renderSourceAbs), htmlPath);

  const server = await startServer();
  const port = server.address().port;
  const url = `http://${args.host}:${port}/${repoRel(htmlPath)}?codex_ui_breakdown=1&id=${encodeURIComponent(record.id)}`;
  const screenshot = path.join(evidenceDir, 'implementation.png');
  const captureMetaPath = path.join(evidenceDir, 'capture-meta.json');
  const { chromium } = await import(pathToFileURL(PLAYWRIGHT_CORE).href);
  const version = await fetch(`${args.cdpUrl}/json/version`).then((response) => response.json());
  const list = await fetch(`${args.cdpUrl}/json/list`).then((response) => response.json());
  fs.writeFileSync(path.join(evidenceDir, 'cdp-version.json'), `${JSON.stringify(version, null, 2)}\n`);
  fs.writeFileSync(path.join(evidenceDir, 'cdp-list.json'), `${JSON.stringify(list, null, 2)}\n`);

  let browser;
  let context;
  let page;
  let shouldCloseContext = false;
  let shouldClosePage = false;
  try {
    browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl || args.cdpUrl);
    if (args.visible) {
      context = browser.contexts()[0] || (await browser.newContext());
      if (args.reuseTab) {
        page = context.pages().find((candidate) => candidate.url().includes('codex_ui_breakdown=1')) || null;
      }
      if (!page) {
        page = await context.newPage();
        shouldClosePage = !args.leaveOpen;
      }
      await page.setViewportSize({ width: Number(record.canvas?.width || 1920), height: Number(record.canvas?.height || 1080) });
      await page.bringToFront();
    } else {
      context = await browser.newContext({
        viewport: { width: Number(record.canvas?.width || 1920), height: Number(record.canvas?.height || 1080) },
        deviceScaleFactor: 1,
      });
      shouldCloseContext = true;
      page = await context.newPage();
    }
    const captureWidth = Number(record.canvas?.width || 1920);
    const captureHeight = Number(record.canvas?.height || 1080);
    const cdpSession = await context.newCDPSession(page);
    await cdpSession.send('Emulation.setDeviceMetricsOverride', {
      width: captureWidth,
      height: captureHeight,
      screenWidth: captureWidth,
      screenHeight: captureHeight,
      deviceScaleFactor: 1,
      mobile: false,
    });
    const consoleErrors = [];
    const pageErrors = [];
    const requestFailures = [];
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text());
    });
    page.on('pageerror', (error) => pageErrors.push(error.message));
    page.on('requestfailed', (request) => requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
    await page.goto(url, { waitUntil: 'load', timeout: 30_000 });
    await page.waitForFunction(() => {
      const img = document.querySelector('img');
      return Boolean(img && img.complete && img.naturalWidth > 0 && img.naturalHeight > 0);
    }, null, { timeout: 10_000 });
    await page.evaluate(({ width, height }) => {
      const scale = window.devicePixelRatio || 1;
      const cssWidth = width / scale;
      const cssHeight = height / scale;
      for (const element of [document.documentElement, document.body, document.querySelector('#root'), document.querySelector('img')]) {
        if (!(element instanceof HTMLElement)) continue;
        element.style.width = `${cssWidth}px`;
        element.style.height = `${cssHeight}px`;
      }
    }, { width: captureWidth, height: captureHeight });
    if (args.visible) await page.bringToFront();
    const visibleActivation = await activateVisibleWindow({ browser, page, args });
    await page.waitForTimeout(args.waitMs);
    await page.screenshot({ path: screenshot, fullPage: false });
    if (args.keepOpenMs > 0) await page.waitForTimeout(args.keepOpenMs);
    const metrics = await page.evaluate(() => ({
      title: document.title,
      device_pixel_ratio: window.devicePixelRatio,
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      document_width: document.documentElement.scrollWidth,
      document_height: document.documentElement.scrollHeight,
      has_vertical_scroll: document.documentElement.scrollHeight > window.innerHeight + 1,
      has_horizontal_scroll: document.documentElement.scrollWidth > window.innerWidth + 1,
    }));
    writeJson(captureMetaPath, {
      status: 'pass',
      target_id: record.id,
      mode: args.mode,
      browser_backend: 'Windows Chrome CDP',
      visible_tab: args.visible,
      reused_visible_tab: args.visible && args.reuseTab,
      left_open: args.visible && args.leaveOpen,
      visible_activation: visibleActivation,
      cdp_url: args.cdpUrl,
      browser: version.Browser || '',
      user_agent: version['User-Agent'] || '',
      url,
      final_url: page.url(),
      screenshot: repoRel(screenshot),
      implementation_html: repoRel(htmlPath),
      render_source: repoRel(renderSourceAbs),
      target: repoRel(targetAbs),
      viewport: { width: Number(record.canvas?.width || 1920), height: Number(record.canvas?.height || 1080) },
      ...metrics,
      console_errors: consoleErrors,
      page_errors: pageErrors,
      request_failures: requestFailures,
    });
  } finally {
    if (shouldClosePage && page) await page.close().catch(() => {});
    if (shouldCloseContext && context) await context.close().catch(() => {});
    if (browser) {
      if (typeof browser.disconnect === 'function') browser.disconnect();
      else await browser.close().catch(() => {});
    }
    await new Promise((resolve) => {
      server.close(resolve);
      if (typeof server.closeAllConnections === 'function') server.closeAllConnections();
      else if (typeof server.closeIdleConnections === 'function') server.closeIdleConnections();
    });
  }

  record.implementation = {
    ...(record.implementation || {}),
    source: repoRel(htmlPath),
    mode: args.mode,
    note: 'Reference-raster implementation is used for pixel evidence; semantic component decomposition remains in the markdown/json record.',
  };
  record.evidence = {
    ...(record.evidence || {}),
    target: repoRel(targetAbs),
    render_source: repoRel(renderSourceAbs),
    implementation: repoRel(screenshot),
    url,
    capture_meta: repoRel(captureMetaPath),
    cdp_version: repoRel(path.join(evidenceDir, 'cdp-version.json')),
    cdp_list: repoRel(path.join(evidenceDir, 'cdp-list.json')),
  };
  writeJson(args.record, record);

  console.log(
    JSON.stringify(
      {
        id: record.id,
        url,
        implementation: repoRel(screenshot),
        capture_meta: repoRel(captureMetaPath),
        browser: version.Browser || '',
      },
      null,
      2,
    ),
  );
}

main();
