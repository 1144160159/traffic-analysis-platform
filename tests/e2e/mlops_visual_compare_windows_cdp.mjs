#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const revision = process.env.MLOPS_EVIDENCE_REVISION?.trim() || 'r336';
const evidenceDir = path.join(root, 'evidence/ui-image-breakdowns/pages/mlops');
const sourcePath = path.join(evidenceDir, 'target.png');
const actualPath = path.join(evidenceDir, `implementation-${revision}.png`);
const diffPath = path.join(evidenceDir, `diff-business-${revision}.png`);
const sidePath = path.join(evidenceDir, `side-by-side-business-${revision}.png`);
const metricsPath = path.join(evidenceDir, `metrics-business-${revision}.json`);
const cdpUrl = 'http://127.0.0.1:9224';
const region = { id: 'content-root', x: 198, y: 80, width: 1722, height: 917 };
const channelTolerance = 90;
const maxRatio = 0.125;
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const dataUrl = (filePath) => `data:image/png;base64,${fs.readFileSync(filePath).toString('base64')}`;
const decode = (value) => Buffer.from(value.slice(value.indexOf(',') + 1), 'base64');

const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
const compared = await page.evaluate(async ({ sourceUrl, actualUrl, scoringRegion, tolerance }) => {
  const load = (url) => new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = reject;
    image.src = url;
  });
  const [source, actual] = await Promise.all([load(sourceUrl), load(actualUrl)]);
  if (source.naturalWidth !== actual.naturalWidth || source.naturalHeight !== actual.naturalHeight) {
    throw new Error('MLOps visual inputs have different dimensions');
  }
  const width = source.naturalWidth;
  const height = source.naturalHeight;
  const canvases = Array.from({ length: 3 }, () => Object.assign(document.createElement('canvas'), { width, height }));
  const [sourceCanvas, actualCanvas, diffCanvas] = canvases;
  const sourceContext = sourceCanvas.getContext('2d', { willReadFrequently: true });
  const actualContext = actualCanvas.getContext('2d', { willReadFrequently: true });
  const diffContext = diffCanvas.getContext('2d');
  sourceContext.drawImage(source, 0, 0);
  actualContext.drawImage(actual, 0, 0);
  const sourcePixels = sourceContext.getImageData(0, 0, width, height);
  const actualPixels = actualContext.getImageData(0, 0, width, height);
  const diffPixels = diffContext.createImageData(width, height);
  let roiMismatch = 0;
  let fullMismatch = 0;
  let maxChannelDelta = 0;
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const index = (y * width + x) * 4;
      const delta = Math.max(
        Math.abs(sourcePixels.data[index] - actualPixels.data[index]),
        Math.abs(sourcePixels.data[index + 1] - actualPixels.data[index + 1]),
        Math.abs(sourcePixels.data[index + 2] - actualPixels.data[index + 2]),
      );
      maxChannelDelta = Math.max(maxChannelDelta, delta);
      const mismatch = delta > tolerance;
      if (mismatch) fullMismatch += 1;
      if (mismatch && x >= scoringRegion.x && x < scoringRegion.x + scoringRegion.width
        && y >= scoringRegion.y && y < scoringRegion.y + scoringRegion.height) roiMismatch += 1;
      if (mismatch) {
        diffPixels.data.set([255, 42, 164, 255], index);
      } else {
        const gray = Math.round((actualPixels.data[index] + actualPixels.data[index + 1] + actualPixels.data[index + 2]) / 3);
        diffPixels.data.set([gray, gray, gray, 90], index);
      }
    }
  }
  diffContext.putImageData(diffPixels, 0, 0);
  const sideCanvas = Object.assign(document.createElement('canvas'), { width: width * 2, height });
  const sideContext = sideCanvas.getContext('2d');
  sideContext.drawImage(source, 0, 0);
  sideContext.drawImage(actual, width, 0);
  return {
    width, height, roiMismatch, fullMismatch, maxChannelDelta,
    diffUrl: diffCanvas.toDataURL('image/png'), sideUrl: sideCanvas.toDataURL('image/png'),
  };
}, {
  sourceUrl: dataUrl(sourcePath), actualUrl: dataUrl(actualPath), scoringRegion: region, tolerance: channelTolerance,
});

const comparedPixels = region.width * region.height;
const totalPixels = compared.width * compared.height;
const ratio = compared.roiMismatch / comparedPixels;
const metrics = {
  target_id: 'mlops', route: '/mlops', status: ratio <= maxRatio ? 'pass' : 'fail',
  generated_at: new Date().toISOString(), desktop_chrome_backend_status: 'windows-chrome-cdp-xshell-tunnel',
  source_image: path.relative(root, sourcePath), actual_screenshot: path.relative(root, actualPath),
  diff_image: path.relative(root, diffPath), viewport: { width: compared.width, height: compared.height },
  visual_diff: {
    size_ok: compared.width === 1920 && compared.height === 1080,
    comparison_scope: 'scoring-region', scoring_region: region,
    source_width: compared.width, source_height: compared.height,
    actual_width: compared.width, actual_height: compared.height,
    compared_pixels: comparedPixels, total_pixels: comparedPixels, mismatch_pixels: compared.roiMismatch,
    pixel_mismatch_ratio: ratio, max_pixel_ratio: maxRatio, channel_tolerance: channelTolerance,
    max_channel_delta: compared.maxChannelDelta,
    full_image_diagnostic: {
      compared_pixels: totalPixels, total_pixels: totalPixels, mismatch_pixels: compared.fullMismatch,
      pixel_mismatch_ratio: compared.fullMismatch / totalPixels, max_channel_delta: compared.maxChannelDelta,
    },
  },
};
fs.writeFileSync(diffPath, decode(compared.diffUrl));
fs.writeFileSync(sidePath, decode(compared.sideUrl));
fs.writeFileSync(metricsPath, `${JSON.stringify(metrics, null, 2)}\n`);
await page.close();
console.log(JSON.stringify({ status: metrics.status, ratio, max_ratio: maxRatio, metrics: path.relative(root, metricsPath) }, null, 2));
process.exit(metrics.status === 'pass' ? 0 : 1);
