#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION || 'r722';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.png');
const actualPath = path.join(root, `evidence/ui-image-breakdowns/pages/campaigns/responsive-${revision}/campaigns-1920x1080.png`);
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaigns');
const comparisonPath = path.join(outputDir, `comparison-${revision}.png`);
const diffPath = path.join(outputDir, `diff-${revision}.png`);
const metricsPath = path.join(outputDir, `metrics-${revision}.json`);
const threshold = 64;

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });
const result = await page.evaluate(async ({ source, actual, threshold }) => {
  const load = (data) => new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = reject;
    image.src = data;
  });
  const [sourceImage, actualImage] = await Promise.all([load(source), load(actual)]);
  const width = sourceImage.naturalWidth;
  const height = sourceImage.naturalHeight;
  if (width !== actualImage.naturalWidth || height !== actualImage.naturalHeight) {
    throw new Error(`dimension mismatch ${width}x${height} vs ${actualImage.naturalWidth}x${actualImage.naturalHeight}`);
  }
  const canvas = (canvasWidth, canvasHeight) => {
    const node = document.createElement('canvas');
    node.width = canvasWidth;
    node.height = canvasHeight;
    return node;
  };
  const sourceCanvas = canvas(width, height);
  const actualCanvas = canvas(width, height);
  sourceCanvas.getContext('2d').drawImage(sourceImage, 0, 0);
  actualCanvas.getContext('2d').drawImage(actualImage, 0, 0);
  const sourcePixels = sourceCanvas.getContext('2d').getImageData(0, 0, width, height);
  const actualPixels = actualCanvas.getContext('2d').getImageData(0, 0, width, height);
  const diff = canvas(width, height);
  const diffContext = diff.getContext('2d');
  const diffPixels = diffContext.createImageData(width, height);
  const regionDefs = [
    { id: 'overview', x: 197, y: 128, width: 1238, height: 162 },
    { id: 'campaign-list', x: 197, y: 298, width: 634, height: 665 },
    { id: 'attack-view', x: 838, y: 298, width: 597, height: 665 },
    { id: 'right-rail', x: 1443, y: 128, width: 454, height: 835 },
  ];
  const regions = Object.fromEntries(regionDefs.map((region) => [region.id, { pixels: 0, mismatched: 0 }]));
  let mismatched = 0;
  let deltaSum = 0;
  for (let index = 0; index < sourcePixels.data.length; index += 4) {
    const pixelIndex = index / 4;
    const x = pixelIndex % width;
    const y = Math.floor(pixelIndex / width);
    const red = Math.abs(sourcePixels.data[index] - actualPixels.data[index]);
    const green = Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]);
    const blue = Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]);
    const delta = Math.max(red, green, blue);
    const changed = delta > threshold;
    deltaSum += red + green + blue;
    if (changed) mismatched += 1;
    for (const region of regionDefs) {
      if (x >= region.x && x < region.x + region.width && y >= region.y && y < region.y + region.height) {
        regions[region.id].pixels += 1;
        if (changed) regions[region.id].mismatched += 1;
      }
    }
    diffPixels.data[index] = changed ? 255 : Math.round(actualPixels.data[index] * 0.2);
    diffPixels.data[index + 1] = changed ? 58 : Math.round(actualPixels.data[index + 1] * 0.2);
    diffPixels.data[index + 2] = changed ? 58 : Math.round(actualPixels.data[index + 2] * 0.2);
    diffPixels.data[index + 3] = 255;
  }
  diffContext.putImageData(diffPixels, 0, 0);
  const comparison = canvas(width * 2, height);
  const comparisonContext = comparison.getContext('2d');
  comparisonContext.drawImage(sourceImage, 0, 0);
  comparisonContext.drawImage(actualImage, width, 0);
  return {
    width,
    height,
    mismatched,
    mismatchRatio: mismatched / (width * height),
    meanAbsoluteChannelDelta: deltaSum / (width * height * 3),
    regions: Object.fromEntries(Object.entries(regions).map(([id, region]) => [
      id,
      { ...region, mismatch_ratio: region.mismatched / region.pixels },
    ])),
    diff: diff.toDataURL('image/png'),
    comparison: comparison.toDataURL('image/png'),
  };
}, {
  source: `data:image/png;base64,${fs.readFileSync(sourcePath).toString('base64')}`,
  actual: `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`,
  threshold,
});

fs.mkdirSync(outputDir, { recursive: true });
const writeDataUrl = (file, dataUrl) => fs.writeFileSync(file, Buffer.from(dataUrl.split(',')[1], 'base64'));
writeDataUrl(diffPath, result.diff);
writeDataUrl(comparisonPath, result.comparison);
const metrics = {
  target_id: `campaigns-${revision}`,
  route: '/campaigns',
  generated_at: new Date().toISOString(),
  browser_backend: 'Windows Chrome CDP over Xshell tunnel 9224',
  source_image: path.relative(root, sourcePath),
  actual_screenshot: path.relative(root, actualPath),
  comparison_image: path.relative(root, comparisonPath),
  diff_image: path.relative(root, diffPath),
  viewport: { width: result.width, height: result.height, device_scale_factor: 1 },
  visual_diff: {
    channel_tolerance: threshold,
    mismatch_pixels: result.mismatched,
    pixel_mismatch_ratio: result.mismatchRatio,
    mean_absolute_channel_delta: result.meanAbsoluteChannelDelta,
    regions: result.regions,
  },
};
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
console.log(JSON.stringify(metrics, null, 2));
await page.close().catch(() => {});
