#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const revision = process.env.CAMPAIGN_EVIDENCE_REVISION || 'r762';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.png');
const actualPath = path.join(root, `evidence/ui-image-breakdowns/pages/campaign-detail/implementation-${revision}.png`);
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/campaign-detail');
const comparisonPath = path.join(outputDir, `comparison-${revision}.png`);
const diffPath = path.join(outputDir, `diff-${revision}.png`);
const metricsPath = path.join(outputDir, `metrics-${revision}.json`);
const threshold = Number(process.env.CAMPAIGN_DETAIL_DIFF_TOLERANCE || '64');
const maxPixelMismatchRatio = Number(process.env.CAMPAIGN_DETAIL_MAX_MISMATCH_RATIO || '0.015');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

if (!fs.existsSync(sourcePath) || !fs.existsSync(actualPath)) {
  throw new Error('Campaign detail source or implementation screenshot is missing');
}

const version = await fetch(`${cdpUrl}/json/version`).then((response) => response.json());
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
let exitCode = 1;

try {
  await page.setViewportSize({ width: 1920, height: 1080 });
  const result = await page.evaluate(async ({ source, actual, threshold: channelTolerance }) => {
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
    const makeCanvas = (canvasWidth, canvasHeight) => {
      const canvas = document.createElement('canvas');
      canvas.width = canvasWidth;
      canvas.height = canvasHeight;
      return canvas;
    };
    const sourceCanvas = makeCanvas(width, height);
    const actualCanvas = makeCanvas(width, height);
    sourceCanvas.getContext('2d').drawImage(sourceImage, 0, 0);
    actualCanvas.getContext('2d').drawImage(actualImage, 0, 0);
    const sourcePixels = sourceCanvas.getContext('2d').getImageData(0, 0, width, height);
    const actualPixels = actualCanvas.getContext('2d').getImageData(0, 0, width, height);
    const diff = makeCanvas(width, height);
    const diffContext = diff.getContext('2d');
    const diffPixels = diffContext.createImageData(width, height);
    const regionDefs = [
      { id: 'title-profile', x: 198, y: 80, width: 1712, height: 167 },
      { id: 'attack-timeline', x: 198, y: 255, width: 1207, height: 264 },
      { id: 'alerts-impact-evidence', x: 198, y: 527, width: 1207, height: 471 },
      { id: 'response-rail', x: 1413, y: 255, width: 497, height: 743 },
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
      const changed = delta > channelTolerance;
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
    const comparison = makeCanvas(width * 2, height);
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
  const writeDataUrl = (filePath, dataUrl) => {
    fs.writeFileSync(filePath, Buffer.from(dataUrl.split(',')[1], 'base64'));
  };
  writeDataUrl(diffPath, result.diff);
  writeDataUrl(comparisonPath, result.comparison);
  const metrics = {
    target_id: `campaign-detail-${revision}`,
    route: '/campaigns/:campaignId',
    generated_at: new Date().toISOString(),
    browser_backend: 'Windows Chrome CDP over Xshell tunnel 9224',
    browser: version.Browser,
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
    acceptance: {
      max_pixel_mismatch_ratio: maxPixelMismatchRatio,
      status: result.mismatchRatio <= maxPixelMismatchRatio ? 'pass' : 'fail',
      note: 'Metric is advisory because the user-directed title and inline impact behavior intentionally supersede the older source image in those regions.',
    },
  };
  fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
  console.log(JSON.stringify(metrics, null, 2));
  exitCode = metrics.acceptance.status === 'pass' ? 0 : 1;
} finally {
  await page.close().catch(() => {});
}
process.exit(exitCode);
