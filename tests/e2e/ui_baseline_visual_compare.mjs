#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const requireFromUi = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = requireFromUi('@playwright/test');
const cdpUrl = process.env.BASELINE_CDP_URL || 'http://127.0.0.1:9224';
const runId = process.env.BASELINE_VISUAL_RUN_ID || 'r658';
const actualDir = path.join(root, process.env.BASELINE_ACTUAL_DIR || 'evidence/learning/baselines/20260723-windows-xshell-r658');
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/baselines');
const threshold = 64;
const maximumRatio = 0.12;

const states = [
  { id: 'asset', target: 'baselines.png', actual: 'baseline-default-1920x1080.png' },
  { id: 'account', target: 'baselines-account.png', actual: 'baseline-account-1920x1080.png' },
  { id: 'port', target: 'baselines-port.png', actual: 'baseline-port-1920x1080.png' },
  { id: 'protocol', target: 'baselines-protocol.png', actual: 'baseline-protocol-1920x1080.png' },
  { id: 'time', target: 'baselines-time-window.png', actual: 'baseline-time-1920x1080.png' },
];

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP unavailable: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
fs.mkdirSync(outputDir, { recursive: true });

const measurements = [];
for (const state of states) {
  const targetPath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages', state.target);
  const actualPath = path.join(actualDir, state.actual);
  if (!fs.existsSync(targetPath) || !fs.existsSync(actualPath)) throw new Error(`missing visual pair for ${state.id}`);
  const targetDataUrl = `data:image/png;base64,${fs.readFileSync(targetPath).toString('base64')}`;
  const actualDataUrl = `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`;
  const result = await page.evaluate(async ({ targetDataUrl, actualDataUrl, threshold }) => {
    const load = (src) => new Promise((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = reject;
      image.src = src;
    });
    const [target, actual] = await Promise.all([load(targetDataUrl), load(actualDataUrl)]);
    if (target.naturalWidth !== actual.naturalWidth || target.naturalHeight !== actual.naturalHeight) throw new Error('visual source dimensions differ');
    const width = target.naturalWidth;
    const height = target.naturalHeight;
    const canvas = (w, h) => Object.assign(document.createElement('canvas'), { width: w, height: h });
    const targetCanvas = canvas(width, height);
    const actualCanvas = canvas(width, height);
    targetCanvas.getContext('2d').drawImage(target, 0, 0);
    actualCanvas.getContext('2d').drawImage(actual, 0, 0);
    const targetPixels = targetCanvas.getContext('2d').getImageData(0, 0, width, height);
    const actualPixels = actualCanvas.getContext('2d').getImageData(0, 0, width, height);
    const measure = (region) => {
      let mismatch = 0;
      let deltaSum = 0;
      const right = Math.min(width, region.x + region.width);
      const bottom = Math.min(height, region.y + region.height);
      for (let y = region.y; y < bottom; y += 1) {
        for (let x = region.x; x < right; x += 1) {
          const index = (y * width + x) * 4;
          const red = Math.abs(targetPixels.data[index] - actualPixels.data[index]);
          const green = Math.abs(targetPixels.data[index + 1] - actualPixels.data[index + 1]);
          const blue = Math.abs(targetPixels.data[index + 2] - actualPixels.data[index + 2]);
          deltaSum += red + green + blue;
          if (Math.max(red, green, blue) > threshold) mismatch += 1;
        }
      }
      const count = (right - region.x) * (bottom - region.y);
      return { ...region, pixel_count: count, mismatch_pixels: mismatch, mismatch_ratio: mismatch / count, mean_absolute_channel_delta: deltaSum / (count * 3) };
    };
    const full = measure({ id: 'full', x: 0, y: 0, width, height });
    const business = measure({ id: 'business', x: 174, y: 80, width: 1726, height: 917 });
    const primary = measure({ id: 'primary_workbench', x: 174, y: 80, width: 1225, height: 917 });
    const rail = measure({ id: 'governance_rail', x: 1406, y: 80, width: 494, height: 917 });
    const comparison = canvas(width * 2, height);
    comparison.getContext('2d').drawImage(target, 0, 0);
    comparison.getContext('2d').drawImage(actual, width, 0);
    const diff = canvas(width, height);
    const diffContext = diff.getContext('2d');
    const diffPixels = diffContext.createImageData(width, height);
    for (let index = 0; index < diffPixels.data.length; index += 4) {
      const delta = Math.max(
        Math.abs(targetPixels.data[index] - actualPixels.data[index]),
        Math.abs(targetPixels.data[index + 1] - actualPixels.data[index + 1]),
        Math.abs(targetPixels.data[index + 2] - actualPixels.data[index + 2]),
      );
      diffPixels.data[index] = delta > threshold ? 255 : Math.round(actualPixels.data[index] * .2);
      diffPixels.data[index + 1] = delta > threshold ? 64 : Math.round(actualPixels.data[index + 1] * .2);
      diffPixels.data[index + 2] = delta > threshold ? 64 : Math.round(actualPixels.data[index + 2] * .2);
      diffPixels.data[index + 3] = 255;
    }
    diffContext.putImageData(diffPixels, 0, 0);
    const png = (value) => value.toDataURL('image/png').split(',')[1];
    return { dimensions: { width, height }, full, regions: [business, primary, rail], comparison: png(comparison), diff: png(diff) };
  }, { targetDataUrl, actualDataUrl, threshold });

  const comparisonPath = path.join(outputDir, `baseline-${state.id}-${runId}-comparison.png`);
  const diffPath = path.join(outputDir, `baseline-${state.id}-${runId}-diff.png`);
  fs.writeFileSync(comparisonPath, Buffer.from(result.comparison, 'base64'));
  fs.writeFileSync(diffPath, Buffer.from(result.diff, 'base64'));
  measurements.push({
    state: state.id,
    target: path.relative(root, targetPath),
    actual: path.relative(root, actualPath),
    comparison: path.relative(root, comparisonPath),
    diff: path.relative(root, diffPath),
    dimensions: result.dimensions,
    full: { ...result.full, maximum_ratio: maximumRatio, passed: result.full.mismatch_ratio <= maximumRatio },
    regions: result.regions.map((region) => ({ ...region, maximum_ratio: maximumRatio, passed: region.mismatch_ratio <= maximumRatio })),
  });
}

await page.close();
await browser.close();
const metrics = {
  result: measurements.every((state) => state.full.passed && state.regions.every((region) => region.passed)) ? 'pass' : 'review_required',
  run_id: runId,
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome CDP',
  browser: version.Browser,
  threshold,
  maximum_ratio: maximumRatio,
  states: measurements,
  generated_at: new Date().toISOString(),
};
const metricsPath = path.join(outputDir, `baseline-${runId}-visual-metrics.json`);
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
console.log(JSON.stringify(metrics, null, 2));
