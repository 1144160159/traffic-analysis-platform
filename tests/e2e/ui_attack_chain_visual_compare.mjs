#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL ?? 'http://127.0.0.1:9224';
const revision = process.env.ATTACK_CHAIN_REVISION ?? 'r780-final';
const sourcePath = path.join(root, 'evidence/ui-image-breakdowns/pages/attack-chains/target.png');
const actualPath = path.join(root, `evidence/ui-image-breakdowns/pages/attack-chains/implementation-${revision}.png`);
const outputDir = path.dirname(sourcePath);
const comparisonPath = path.join(outputDir, `comparison-${revision}.png`);
const diffPath = path.join(outputDir, `diff-${revision}.png`);
const metricsPath = path.join(outputDir, `metrics-${revision}.json`);
const channelTolerance = Number(process.env.ATTACK_CHAIN_DIFF_TOLERANCE ?? '64');
const maximumRatio = Number(process.env.ATTACK_CHAIN_MAX_MISMATCH_RATIO ?? '0.15');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
if (!fs.existsSync(sourcePath) || !fs.existsSync(actualPath)) {
  throw new Error('Attack-chain target or implementation screenshot is missing');
}

const version = await fetch(`${cdpUrl}/json/version`).then((response) => response.json());
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();

try {
  const result = await page.evaluate(async ({ source, actual, tolerance }) => {
    const load = (url) => new Promise((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = reject;
      image.src = url;
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
    const diffCanvas = canvas(width, height);
    const diffContext = diffCanvas.getContext('2d');
    const diffPixels = diffContext.createImageData(width, height);
    const regionDefs = [
      { id: 'business-region', x: 166, y: 80, width: 1754, height: 918 },
      { id: 'chain-workspace', x: 183, y: 126, width: 1198, height: 616 },
      { id: 'right-rail', x: 1384, y: 126, width: 528, height: 858 },
      { id: 'bottom-detail', x: 183, y: 746, width: 1198, height: 238 },
    ];
    const regions = Object.fromEntries(regionDefs.map(({ id }) => [id, { pixels: 0, mismatched: 0 }]));
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
      const changed = delta > tolerance;
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
    const comparisonCanvas = canvas(width * 2, height);
    const comparisonContext = comparisonCanvas.getContext('2d');
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
      comparison: comparisonCanvas.toDataURL('image/png'),
      diff: diffCanvas.toDataURL('image/png'),
    };
  }, {
    source: `data:image/png;base64,${fs.readFileSync(sourcePath).toString('base64')}`,
    actual: `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`,
    tolerance: channelTolerance,
  });

  const writeDataUrl = (filePath, dataUrl) => {
    fs.writeFileSync(filePath, Buffer.from(dataUrl.split(',')[1], 'base64'));
  };
  writeDataUrl(comparisonPath, result.comparison);
  writeDataUrl(diffPath, result.diff);
  const metrics = {
    target_id: `attack-chains-${revision}`,
    route: '/attack-chains',
    generated_at: new Date().toISOString(),
    browser_backend: 'Windows Chrome CDP over Xshell 9224 -> 9222',
    browser: version.Browser,
    source_image: path.relative(root, sourcePath),
    actual_screenshot: path.relative(root, actualPath),
    comparison_image: path.relative(root, comparisonPath),
    diff_image: path.relative(root, diffPath),
    viewport: { width: result.width, height: result.height, device_scale_factor: 1 },
    visual_diff: {
      channel_tolerance: channelTolerance,
      mismatch_pixels: result.mismatched,
      pixel_mismatch_ratio: result.mismatchRatio,
      mean_absolute_channel_delta: result.meanAbsoluteChannelDelta,
      regions: result.regions,
    },
    acceptance: {
      max_pixel_mismatch_ratio: maximumRatio,
      status: result.mismatchRatio <= maximumRatio ? 'pass' : 'fail',
    },
  };
  fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
  console.log(JSON.stringify(metrics, null, 2));
  process.exitCode = metrics.acceptance.status === 'pass' ? 0 : 1;
} finally {
  await page.close().catch(() => {});
}
process.exit(process.exitCode ?? 1);
