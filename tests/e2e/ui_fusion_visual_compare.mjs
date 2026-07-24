#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost';

const root = process.cwd();
const require = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = require('@playwright/test');
const cdpUrl = process.env.UI_CDP_URL || 'http://127.0.0.1:9224';
const artifactRevision = process.env.FUSION_ARTIFACT_REVISION || 'r603';
const sourcePath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png');
const actualPath = process.env.FUSION_ACTUAL_PATH
  ? path.resolve(root, process.env.FUSION_ACTUAL_PATH)
  : path.join(root, `evidence/ui-image-breakdowns/pages/fusion/fusion-${artifactRevision}-interactions.png`);
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/fusion');
const comparisonPath = path.join(outputDir, `fusion-${artifactRevision}-comparison.png`);
const businessComparisonPath = path.join(outputDir, `fusion-${artifactRevision}-business-comparison.png`);
const focusedComparisonPath = path.join(outputDir, `fusion-${artifactRevision}-source-pipeline-comparison.png`);
const bottomComparisonPath = path.join(outputDir, `fusion-${artifactRevision}-bottom-workbench-comparison.png`);
const diffPath = path.join(outputDir, `fusion-${artifactRevision}-diff.png`);
const metricsPath = path.join(outputDir, `fusion-${artifactRevision}-visual-metrics.json`);
const threshold = 64;
const maximumRatio = 0.125;
const regionalMaximums = {
  business: 0.125,
  source_status: 0.135,
  fusion_pipeline: 0.142,
  rule_table: 0.125,
  bottom_workbench: 0.102,
  conflict_detail: 0.105,
};

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed with ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
await page.setViewportSize({ width: 1920, height: 1080 });

const sourceDataUrl = `data:image/png;base64,${fs.readFileSync(sourcePath).toString('base64')}`;
const actualDataUrl = `data:image/png;base64,${fs.readFileSync(actualPath).toString('base64')}`;
const result = await page.evaluate(async ({ sourceDataUrl, actualDataUrl, threshold }) => {
  const load = (src) => new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error('PNG decode failed'));
    image.src = src;
  });
  const [source, actual] = await Promise.all([load(sourceDataUrl), load(actualDataUrl)]);
  if (source.naturalWidth !== actual.naturalWidth || source.naturalHeight !== actual.naturalHeight) {
    throw new Error(`dimension mismatch: ${source.naturalWidth}x${source.naturalHeight} vs ${actual.naturalWidth}x${actual.naturalHeight}`);
  }
  const width = source.naturalWidth;
  const height = source.naturalHeight;
  const makeCanvas = (canvasWidth, canvasHeight) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvasWidth;
    canvas.height = canvasHeight;
    return canvas;
  };
  const sourceCanvas = makeCanvas(width, height);
  const actualCanvas = makeCanvas(width, height);
  sourceCanvas.getContext('2d').drawImage(source, 0, 0);
  actualCanvas.getContext('2d').drawImage(actual, 0, 0);
  const sourcePixels = sourceCanvas.getContext('2d').getImageData(0, 0, width, height);
  const actualPixels = actualCanvas.getContext('2d').getImageData(0, 0, width, height);
  const diffCanvas = makeCanvas(width, height);
  const diffContext = diffCanvas.getContext('2d');
  const diffPixels = diffContext.createImageData(width, height);
  let mismatched = 0;
  let channelDeltaSum = 0;
  let maximumChannelDelta = 0;
  for (let index = 0; index < sourcePixels.data.length; index += 4) {
    const red = Math.abs(sourcePixels.data[index] - actualPixels.data[index]);
    const green = Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]);
    const blue = Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]);
    const delta = Math.max(red, green, blue);
    channelDeltaSum += red + green + blue;
    maximumChannelDelta = Math.max(maximumChannelDelta, delta);
    if (delta > threshold) mismatched += 1;
    const alpha = Math.max(34, delta);
    diffPixels.data[index] = delta > threshold ? 255 : actualPixels.data[index] * 0.24;
    diffPixels.data[index + 1] = delta > threshold ? 64 : actualPixels.data[index + 1] * 0.24;
    diffPixels.data[index + 2] = delta > threshold ? 64 : actualPixels.data[index + 2] * 0.24;
    diffPixels.data[index + 3] = alpha;
  }
  diffContext.putImageData(diffPixels, 0, 0);

  const regions = [
    { id: 'business', x: 174, y: 74, width: 1726, height: 912 },
    { id: 'source_status', x: 174, y: 118, width: 1233, height: 235 },
    { id: 'fusion_pipeline', x: 174, y: 330, width: 1233, height: 286 },
    { id: 'rule_table', x: 174, y: 600, width: 1233, height: 230 },
    { id: 'bottom_workbench', x: 174, y: 800, width: 1726, height: 186 },
    { id: 'conflict_detail', x: 1414, y: 74, width: 486, height: 734 },
  ];
  const diagnosticRegions = [
    { id: 'bottom_conflicts', x: 174, y: 800, width: 510, height: 186 },
    { id: 'bottom_audit', x: 691, y: 800, width: 652, height: 186 },
    { id: 'bottom_quality', x: 1350, y: 800, width: 550, height: 186 },
  ];
  const measureRegions = (items) => items.map((region) => {
    let regionMismatch = 0;
    let regionDelta = 0;
    for (let y = region.y; y < Math.min(height, region.y + region.height); y += 1) {
      for (let x = region.x; x < Math.min(width, region.x + region.width); x += 1) {
        const index = (y * width + x) * 4;
        const red = Math.abs(sourcePixels.data[index] - actualPixels.data[index]);
        const green = Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]);
        const blue = Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]);
        regionDelta += red + green + blue;
        if (Math.max(red, green, blue) > threshold) regionMismatch += 1;
      }
    }
    const pixels = region.width * region.height;
    return { ...region, mismatchPixels: regionMismatch, pixelCount: pixels, mismatchRatio: regionMismatch / pixels, meanAbsoluteChannelDelta: regionDelta / (pixels * 3) };
  });
  const regionMetrics = measureRegions(regions);
  const diagnosticRegionMetrics = measureRegions(diagnosticRegions);
  const colorHistogram = (pixels, region) => {
    const counts = new Map();
    for (let y = region.y; y < region.y + region.height; y += 1) {
      for (let x = region.x; x < region.x + region.width; x += 1) {
        const index = (y * width + x) * 4;
        const key = `${pixels.data[index]},${pixels.data[index + 1]},${pixels.data[index + 2]}`;
        counts.set(key, (counts.get(key) ?? 0) + 1);
      }
    }
    return [...counts.entries()].sort((left, right) => right[1] - left[1]).slice(0, 16).map(([rgb, count]) => ({ rgb, count }));
  };
  const bottomColorHistograms = {
    source: colorHistogram(sourcePixels, regions[4]),
    actual: colorHistogram(actualPixels, regions[4]),
  };
  const bottomMismatchPairs = (() => {
    const counts = new Map();
    const region = regions[4];
    for (let y = region.y; y < region.y + region.height; y += 1) {
      for (let x = region.x; x < region.x + region.width; x += 1) {
        const index = (y * width + x) * 4;
        const sourceRGB = [sourcePixels.data[index], sourcePixels.data[index + 1], sourcePixels.data[index + 2]];
        const actualRGB = [actualPixels.data[index], actualPixels.data[index + 1], actualPixels.data[index + 2]];
        if (Math.max(...sourceRGB.map((value, channel) => Math.abs(value - actualRGB[channel]))) <= threshold) continue;
        const key = `${sourceRGB.join(',')} -> ${actualRGB.join(',')}`;
        counts.set(key, (counts.get(key) ?? 0) + 1);
      }
    }
    return [...counts.entries()].sort((left, right) => right[1] - left[1]).slice(0, 24).map(([pair, count]) => ({ pair, count }));
  })();
  const bottomMismatchDistribution = (() => {
    const region = regions[4];
    const byY = [];
    const byX = [];
    for (let offsetY = 0; offsetY < region.height; offsetY += 6) {
      let count = 0;
      for (let y = region.y + offsetY; y < Math.min(region.y + region.height, region.y + offsetY + 6); y += 1) {
        for (let x = region.x; x < region.x + region.width; x += 1) {
          const index = (y * width + x) * 4;
          if (Math.max(
            Math.abs(sourcePixels.data[index] - actualPixels.data[index]),
            Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]),
            Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]),
          ) > threshold) count += 1;
        }
      }
      byY.push({ y: region.y + offsetY, count });
    }
    for (let offsetX = 0; offsetX < region.width; offsetX += 50) {
      let count = 0;
      for (let y = region.y; y < region.y + region.height; y += 1) {
        for (let x = region.x + offsetX; x < Math.min(region.x + region.width, region.x + offsetX + 50); x += 1) {
          const index = (y * width + x) * 4;
          if (Math.max(
            Math.abs(sourcePixels.data[index] - actualPixels.data[index]),
            Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]),
            Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]),
          ) > threshold) count += 1;
        }
      }
      byX.push({ x: region.x + offsetX, count });
    }
    return { byY, byX };
  })();
  const bottomOpacitySweep = [0.45, 0.55, 0.65, 0.75, 0.85, 0.95, 1].map((opacity) => {
    let mismatched = 0;
    const background = [3, 17, 28];
    for (let y = 834; y < 986; y += 1) {
      for (let x = 174; x < 1900; x += 1) {
        const index = (y * width + x) * 4;
        const deltas = [0, 1, 2].map((channel) => Math.abs(
          sourcePixels.data[index + channel]
          - Math.round(background[channel] + (actualPixels.data[index + channel] - background[channel]) * opacity),
        ));
        if (Math.max(...deltas) > threshold) mismatched += 1;
      }
    }
    return { opacity, mismatchPixels: mismatched, pixelCount: 1726 * 152, mismatchRatio: mismatched / (1726 * 152) };
  });

  const comparisonCanvas = makeCanvas(width * 2, height);
  const comparisonContext = comparisonCanvas.getContext('2d');
  comparisonContext.drawImage(source, 0, 0);
  comparisonContext.drawImage(actual, width, 0);

  const businessLeft = 174;
  const businessWidth = 1726;
  const businessCanvas = makeCanvas(businessWidth * 2, height);
  const businessContext = businessCanvas.getContext('2d');
  businessContext.drawImage(source, businessLeft, 0, businessWidth, height, 0, 0, businessWidth, height);
  businessContext.drawImage(actual, businessLeft, 0, businessWidth, height, businessWidth, 0, businessWidth, height);

  const focusTop = 80;
  const focusWidth = 1218;
  const focusHeight = 500;
  const focusedCanvas = makeCanvas(focusWidth * 2, focusHeight);
  const focusedContext = focusedCanvas.getContext('2d');
  focusedContext.drawImage(source, businessLeft, focusTop, focusWidth, focusHeight, 0, 0, focusWidth, focusHeight);
  focusedContext.drawImage(actual, businessLeft, focusTop, focusWidth, focusHeight, focusWidth, 0, focusWidth, focusHeight);

  const bottomTop = 800;
  const bottomHeight = 186;
  const bottomCanvas = makeCanvas(businessWidth * 2, bottomHeight);
  const bottomContext = bottomCanvas.getContext('2d');
  bottomContext.drawImage(source, businessLeft, bottomTop, businessWidth, bottomHeight, 0, 0, businessWidth, bottomHeight);
  bottomContext.drawImage(actual, businessLeft, bottomTop, businessWidth, bottomHeight, businessWidth, 0, businessWidth, bottomHeight);

  return {
    dimensions: { width, height },
    mismatchPixels: mismatched,
    pixelCount: width * height,
    mismatchRatio: mismatched / (width * height),
    meanAbsoluteChannelDelta: channelDeltaSum / (width * height * 3),
    maximumChannelDelta,
    regionMetrics,
    diagnosticRegionMetrics,
    bottomOpacitySweep,
    bottomColorHistograms,
    bottomMismatchPairs,
    bottomMismatchDistribution,
    comparison: comparisonCanvas.toDataURL('image/png').split(',')[1],
    businessComparison: businessCanvas.toDataURL('image/png').split(',')[1],
    focusedComparison: focusedCanvas.toDataURL('image/png').split(',')[1],
    bottomComparison: bottomCanvas.toDataURL('image/png').split(',')[1],
    diff: diffCanvas.toDataURL('image/png').split(',')[1],
  };
}, { sourceDataUrl, actualDataUrl, threshold });

fs.mkdirSync(outputDir, { recursive: true });
fs.writeFileSync(comparisonPath, Buffer.from(result.comparison, 'base64'));
fs.writeFileSync(businessComparisonPath, Buffer.from(result.businessComparison, 'base64'));
fs.writeFileSync(focusedComparisonPath, Buffer.from(result.focusedComparison, 'base64'));
fs.writeFileSync(bottomComparisonPath, Buffer.from(result.bottomComparison, 'base64'));
fs.writeFileSync(diffPath, Buffer.from(result.diff, 'base64'));
const regionMetrics = result.regionMetrics.map((region) => ({
  ...region,
  maximumRatio: regionalMaximums[region.id],
  passed: region.mismatchRatio <= regionalMaximums[region.id],
}));
const metrics = {
  result: result.mismatchRatio <= maximumRatio && regionMetrics.every((region) => region.passed) ? 'pass' : 'fail',
  browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome CDP',
  browser: version.Browser,
  source: path.relative(root, sourcePath),
  actual: path.relative(root, actualPath),
  comparison: path.relative(root, comparisonPath),
  business_comparison: path.relative(root, businessComparisonPath),
  focused_comparison: path.relative(root, focusedComparisonPath),
  bottom_comparison: path.relative(root, bottomComparisonPath),
  diff: path.relative(root, diffPath),
  dimensions: result.dimensions,
  threshold,
  maximum_ratio: maximumRatio,
  regional_maximums: regionalMaximums,
  mismatch_pixels: result.mismatchPixels,
  pixel_count: result.pixelCount,
  mismatch_ratio: result.mismatchRatio,
  mean_absolute_channel_delta: result.meanAbsoluteChannelDelta,
  maximum_channel_delta: result.maximumChannelDelta,
  regions: regionMetrics,
  diagnostic_regions: result.diagnosticRegionMetrics,
  bottom_opacity_sweep: result.bottomOpacitySweep,
  bottom_color_histograms: result.bottomColorHistograms,
  bottom_mismatch_pairs: result.bottomMismatchPairs,
  bottom_mismatch_distribution: result.bottomMismatchDistribution,
  generated_at: new Date().toISOString(),
};
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
console.log(JSON.stringify(metrics, null, 2));
await page.close();
process.exit(metrics.result === 'pass' ? 0 : 1);
