#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const runId = process.env.SETTINGS_VISUAL_RUN_ID || 'r496';
const cdpUrl = process.env.SETTINGS_UI_CDP_URL || 'http://127.0.0.1:9224';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png');
const actualPath = path.join(root, `evidence/ui-image-breakdowns/pages/settings/actual-${runId}-main-1920.png`);
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/settings');
const diffPath = path.join(outputDir, `diff-${runId}-main.png`);
const comparePath = path.join(outputDir, `compare-${runId}-main.png`);
const metricsPath = path.join(outputDir, `metrics-${runId}-main.json`);
const channelTolerance = 90;
const maxRatio = 0.125;

for (const file of [sourcePath, actualPath]) if (!fs.existsSync(file)) throw new Error(`missing visual input: ${file}`);
const toDataUrl = (file) => `data:image/png;base64,${fs.readFileSync(file).toString('base64')}`;
const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const result = await page.evaluate(async ({ sourceUrl, actualUrl, tolerance }) => {
  const load = (url) => new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = reject;
    image.src = url;
  });
  const [source, actual] = await Promise.all([load(sourceUrl), load(actualUrl)]);
  if (source.naturalWidth !== actual.naturalWidth || source.naturalHeight !== actual.naturalHeight) throw new Error('visual inputs have different dimensions');
  const width = source.naturalWidth;
  const height = source.naturalHeight;
  const sourceCanvas = document.createElement('canvas');
  const actualCanvas = document.createElement('canvas');
  const diffCanvas = document.createElement('canvas');
  for (const canvas of [sourceCanvas, actualCanvas, diffCanvas]) { canvas.width = width; canvas.height = height; }
  const sourceContext = sourceCanvas.getContext('2d', { willReadFrequently: true });
  const actualContext = actualCanvas.getContext('2d', { willReadFrequently: true });
  const diffContext = diffCanvas.getContext('2d');
  sourceContext.drawImage(source, 0, 0);
  actualContext.drawImage(actual, 0, 0);
  const sourcePixels = sourceContext.getImageData(0, 0, width, height);
  const actualPixels = actualContext.getImageData(0, 0, width, height);
  const diffPixels = diffContext.createImageData(width, height);
  let mismatch = 0;
  let maxChannelDelta = 0;
  for (let index = 0; index < sourcePixels.data.length; index += 4) {
    const delta = Math.max(
      Math.abs(sourcePixels.data[index] - actualPixels.data[index]),
      Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]),
      Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]),
    );
    maxChannelDelta = Math.max(maxChannelDelta, delta);
    const changed = delta > tolerance;
    if (changed) mismatch += 1;
    if (changed) {
      diffPixels.data[index] = 255; diffPixels.data[index + 1] = 42; diffPixels.data[index + 2] = 164; diffPixels.data[index + 3] = 255;
    } else {
      const gray = Math.round((actualPixels.data[index] + actualPixels.data[index + 1] + actualPixels.data[index + 2]) / 3);
      diffPixels.data[index] = gray; diffPixels.data[index + 1] = gray; diffPixels.data[index + 2] = gray; diffPixels.data[index + 3] = 90;
    }
  }
  diffContext.putImageData(diffPixels, 0, 0);
  const compareCanvas = document.createElement('canvas');
  compareCanvas.width = width * 2; compareCanvas.height = height;
  const compareContext = compareCanvas.getContext('2d');
  compareContext.drawImage(source, 0, 0); compareContext.drawImage(actual, width, 0);
  return { width, height, mismatch, maxChannelDelta, diff: diffCanvas.toDataURL('image/png'), compare: compareCanvas.toDataURL('image/png') };
}, { sourceUrl: toDataUrl(sourcePath), actualUrl: toDataUrl(actualPath), tolerance: channelTolerance });

const decode = (dataUrl) => Buffer.from(dataUrl.slice(dataUrl.indexOf(',') + 1), 'base64');
fs.writeFileSync(diffPath, decode(result.diff));
fs.writeFileSync(comparePath, decode(result.compare));
const total = result.width * result.height;
const ratio = result.mismatch / total;
const metrics = {
  target_id: `settings-main-${runId}`,
  route: '/settings',
  status: ratio <= maxRatio ? 'pass' : 'fail',
  generated_at: new Date().toISOString(),
  desktop_chrome_backend_status: 'pass',
  source_image: path.relative(root, sourcePath),
  actual_screenshot: path.relative(root, actualPath),
  diff_image: path.relative(root, diffPath),
  combined_comparison: path.relative(root, comparePath),
  viewport: { width: result.width, height: result.height },
  visual_diff: {
    size_ok: result.width === 1920 && result.height === 1080,
    comparison_scope: 'full-image',
    compared_pixels: total,
    mismatch_pixels: result.mismatch,
    pixel_mismatch_ratio: ratio,
    max_pixel_ratio: maxRatio,
    channel_tolerance: channelTolerance,
    max_channel_delta: result.maxChannelDelta,
  },
};
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
await page.close();
await browser.close();
console.log(JSON.stringify({ metrics: path.relative(root, metricsPath), status: metrics.status, ratio, max_ratio: maxRatio }));
if (metrics.status !== 'pass') process.exitCode = 1;
