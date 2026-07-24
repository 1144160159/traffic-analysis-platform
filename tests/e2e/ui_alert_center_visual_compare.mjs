#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png');
const actualPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/interaction-r651.png');
const comparisonPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/comparison-r651.png');
const diffPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/diff-r651.png');
const metricsPath = path.join(root, 'evidence/ui-image-breakdowns/pages/alerts/metrics-r651.json');
const threshold = 64;
const maximumRatio = 0.13;
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const result = await page.evaluate(async ({ source, actual, threshold }) => {
  const load = (data) => new Promise((resolve, reject) => { const image = new Image(); image.onload = () => resolve(image); image.onerror = reject; image.src = data; });
  const [sourceImage, actualImage] = await Promise.all([load(source), load(actual)]);
  const width = sourceImage.naturalWidth;
  const height = sourceImage.naturalHeight;
  if (width !== actualImage.naturalWidth || height !== actualImage.naturalHeight) throw new Error(`dimension mismatch ${width}x${height} vs ${actualImage.naturalWidth}x${actualImage.naturalHeight}`);
  const canvas = (w, h) => { const node = document.createElement('canvas'); node.width = w; node.height = h; return node; };
  const left = canvas(width, height); const right = canvas(width, height);
  left.getContext('2d').drawImage(sourceImage, 0, 0); right.getContext('2d').drawImage(actualImage, 0, 0);
  const sourcePixels = left.getContext('2d').getImageData(0, 0, width, height);
  const actualPixels = right.getContext('2d').getImageData(0, 0, width, height);
  const diff = canvas(width, height); const diffContext = diff.getContext('2d'); const diffPixels = diffContext.createImageData(width, height);
  const regionDefs = [
    { id: 'queue-and-filters', x: 200, y: 74, width: 1160, height: 338, maxRatio: 0.17 },
    { id: 'alert-table', x: 200, y: 412, width: 1160, height: 620, maxRatio: 0.17 },
    { id: 'detail-and-feedback', x: 1360, y: 74, width: 548, height: 958, maxRatio: 0.19 },
  ];
  const regionCounters = Object.fromEntries(regionDefs.map((region) => [region.id, { mismatched: 0, pixels: 0, maxRatio: region.maxRatio }]));
  let mismatched = 0; let deltaSum = 0;
  for (let index = 0; index < sourcePixels.data.length; index += 4) {
    const pixelIndex = index / 4; const x = pixelIndex % width; const y = Math.floor(pixelIndex / width);
    const red = Math.abs(sourcePixels.data[index] - actualPixels.data[index]);
    const green = Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]);
    const blue = Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]);
    const delta = Math.max(red, green, blue); deltaSum += red + green + blue;
    if (delta > threshold) mismatched += 1;
    for (const region of regionDefs) {
      if (x >= region.x && x < region.x + region.width && y >= region.y && y < region.y + region.height) {
        regionCounters[region.id].pixels += 1;
        if (delta > threshold) regionCounters[region.id].mismatched += 1;
      }
    }
    diffPixels.data[index] = delta > threshold ? 255 : Math.round(actualPixels.data[index] * 0.22);
    diffPixels.data[index + 1] = delta > threshold ? 58 : Math.round(actualPixels.data[index + 1] * 0.22);
    diffPixels.data[index + 2] = delta > threshold ? 58 : Math.round(actualPixels.data[index + 2] * 0.22);
    diffPixels.data[index + 3] = 255;
  }
  diffContext.putImageData(diffPixels, 0, 0);
  const comparison = canvas(width * 2, height); const comparisonContext = comparison.getContext('2d');
  comparisonContext.drawImage(sourceImage, 0, 0); comparisonContext.drawImage(actualImage, width, 0);
  const regions = Object.fromEntries(Object.entries(regionCounters).map(([id, counter]) => [id, { ...counter, mismatchRatio: counter.mismatched / counter.pixels, status: counter.mismatched / counter.pixels <= counter.maxRatio ? 'pass' : 'fail' }]));
  return { width, height, mismatched, mismatchRatio: mismatched / (width * height), meanAbsoluteChannelDelta: deltaSum / (width * height * 3), regions, diff: diff.toDataURL('image/png'), comparison: comparison.toDataURL('image/png') };
}, {
  source: `data:image/png;base64,${fs.readFileSync(sourcePath).toString('base64')}`,
  actual: `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`,
  threshold,
});
const writeDataUrl = (file, dataUrl) => fs.writeFileSync(file, Buffer.from(dataUrl.split(',')[1], 'base64'));
writeDataUrl(diffPath, result.diff); writeDataUrl(comparisonPath, result.comparison);
const regionsPass = Object.values(result.regions).every((region) => region.status === 'pass');
const metrics = { target_id: 'alerts-r651', route: '/alerts', status: result.mismatchRatio <= maximumRatio && regionsPass ? 'pass' : 'fail', generated_at: new Date().toISOString(), browser_backend: 'Windows Chrome CDP over Xshell tunnel', source_image: path.relative(root, sourcePath), actual_screenshot: path.relative(root, actualPath), comparison_image: path.relative(root, comparisonPath), diff_image: path.relative(root, diffPath), viewport: { width: result.width, height: result.height }, visual_diff: { channel_tolerance: threshold, mismatch_pixels: result.mismatched, pixel_mismatch_ratio: result.mismatchRatio, max_pixel_ratio: maximumRatio, mean_absolute_channel_delta: result.meanAbsoluteChannelDelta, regions: result.regions } };
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`); console.log(JSON.stringify(metrics, null, 2));
await page.close().catch(() => {}); process.exit(metrics.status === 'pass' ? 0 : 1);
