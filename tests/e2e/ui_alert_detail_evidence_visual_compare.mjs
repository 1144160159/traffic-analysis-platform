#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const cdpUrl = process.env.TRAFFIC_WINDOWS_CDP_URL || 'http://127.0.0.1:9224';
const outputDir = path.join(
  root,
  'doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-alert-detail-r805-evidence',
);
const cases = [
  { id: 'pcap', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-pcap.png' },
  { id: 'session', source: 'evidence/ui-image-breakdowns/pages/alert-detail-evidence-session/target-business-r160-close-final.png' },
  { id: 'logs', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-logs.png' },
  { id: 'graph-path', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-graph-path.png' },
  { id: 'files', source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-files.png' },
];
const channelTolerance = 28;
const maximumRatio = 0.35;

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost';

const versionResponse = await fetch(`${cdpUrl}/json/version`);
if (!versionResponse.ok) throw new Error(`Windows Chrome CDP preflight failed: ${versionResponse.status}`);
const version = await versionResponse.json();
const browser = await chromium.connectOverCDP(cdpUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();

const results = [];
for (const item of cases) {
  const sourcePath = path.join(root, item.source);
  const actualPath = path.join(outputDir, `${item.id}-business.png`);
  if (!fs.existsSync(sourcePath) || !fs.existsSync(actualPath)) {
    throw new Error(`visual input missing for ${item.id}`);
  }
  const compared = await page.evaluate(async ({ sourceBase64, actualBase64, tolerance }) => {
    const decode = async (base64) => {
      const binary = atob(base64);
      const bytes = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
      return createImageBitmap(new Blob([bytes], { type: 'image/png' }));
    };
    const [sourceImage, actualImage] = await Promise.all([decode(sourceBase64), decode(actualBase64)]);
    const width = actualImage.width;
    const height = actualImage.height;
    const canvasFor = () => {
      const canvas = document.createElement('canvas');
      canvas.width = width;
      canvas.height = height;
      return canvas;
    };
    const sourceCanvas = canvasFor();
    const sourceContext = sourceCanvas.getContext('2d', { willReadFrequently: true });
    sourceContext.drawImage(sourceImage, 0, 0, width, height);
    const actualCanvas = canvasFor();
    const actualContext = actualCanvas.getContext('2d', { willReadFrequently: true });
    actualContext.drawImage(actualImage, 0, 0);
    const sourcePixels = sourceContext.getImageData(0, 0, width, height).data;
    const actualPixels = actualContext.getImageData(0, 0, width, height).data;
    const diffCanvas = canvasFor();
    const diffContext = diffCanvas.getContext('2d');
    const diffPixels = diffContext.createImageData(width, height);
    let mismatched = 0;
    let absoluteDelta = 0;
    for (let index = 0; index < sourcePixels.length; index += 4) {
      const red = Math.abs(sourcePixels[index] - actualPixels[index]);
      const green = Math.abs(sourcePixels[index + 1] - actualPixels[index + 1]);
      const blue = Math.abs(sourcePixels[index + 2] - actualPixels[index + 2]);
      const changed = Math.max(red, green, blue) > tolerance;
      if (changed) mismatched += 1;
      absoluteDelta += red + green + blue;
      diffPixels.data[index] = changed ? 255 : Math.round(actualPixels[index] * 0.2);
      diffPixels.data[index + 1] = changed ? 48 : Math.round(actualPixels[index + 1] * 0.2);
      diffPixels.data[index + 2] = changed ? 80 : Math.round(actualPixels[index + 2] * 0.2);
      diffPixels.data[index + 3] = 255;
    }
    diffContext.putImageData(diffPixels, 0, 0);
    const comparisonCanvas = document.createElement('canvas');
    comparisonCanvas.width = width * 2;
    comparisonCanvas.height = height;
    const comparisonContext = comparisonCanvas.getContext('2d');
    comparisonContext.drawImage(sourceCanvas, 0, 0);
    comparisonContext.drawImage(actualCanvas, width, 0);
    return {
      width,
      height,
      sourceWidth: sourceImage.width,
      sourceHeight: sourceImage.height,
      mismatched,
      meanAbsoluteChannelDelta: absoluteDelta / (width * height * 3),
      normalizedSource: sourceCanvas.toDataURL('image/png'),
      diff: diffCanvas.toDataURL('image/png'),
      comparison: comparisonCanvas.toDataURL('image/png'),
    };
  }, {
    sourceBase64: fs.readFileSync(sourcePath).toString('base64'),
    actualBase64: fs.readFileSync(actualPath).toString('base64'),
    tolerance: channelTolerance,
  });
  const normalizedSourcePath = path.join(outputDir, `${item.id}-source-normalized.png`);
  const diffPath = path.join(outputDir, `${item.id}-diff.png`);
  const comparisonPath = path.join(outputDir, `${item.id}-comparison.png`);
  const writeDataUrl = (target, dataUrl) => fs.writeFileSync(target, Buffer.from(dataUrl.split(',')[1], 'base64'));
  writeDataUrl(normalizedSourcePath, compared.normalizedSource);
  writeDataUrl(diffPath, compared.diff);
  writeDataUrl(comparisonPath, compared.comparison);
  const ratio = compared.mismatched / (compared.width * compared.height);
  results.push({
    id: item.id,
    status: ratio <= maximumRatio ? 'pass' : 'fail',
    source_image: item.source,
    source_original_size: { width: compared.sourceWidth, height: compared.sourceHeight },
    normalized_source: path.relative(root, normalizedSourcePath),
    actual_screenshot: path.relative(root, actualPath),
    comparison_image: path.relative(root, comparisonPath),
    diff_image: path.relative(root, diffPath),
    compared_size: { width: compared.width, height: compared.height },
    channel_tolerance: channelTolerance,
    mismatch_pixels: compared.mismatched,
    pixel_mismatch_ratio: ratio,
    max_pixel_ratio: maximumRatio,
    mean_absolute_channel_delta: compared.meanAbsoluteChannelDelta,
  });
}

const report = {
  result: results.every((item) => item.status === 'pass') ? 'pass' : 'fail',
  browser_backend: 'Windows Chrome through Xshell CDP tunnel',
  browser: version.Browser,
  comparison_contract: 'Reference content is normalized to the fixed 1152x666 modal body crop; comparison images place reference left and implementation right.',
  fixed_modal_contract: { width: 1152, height: 720, tab_geometry_delta: 0 },
  results,
  timestamp: new Date().toISOString(),
};
const reportPath = path.join(outputDir, 'visual-metrics.json');
fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report, null, 2));

await page.close();
await browser.close();
if (report.result !== 'pass') process.exitCode = 1;
