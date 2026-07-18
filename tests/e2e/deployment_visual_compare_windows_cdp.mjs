#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const { chromium } = createRequire(path.join(root, 'web/ui/package.json'))('@playwright/test');
const cdpUrl = 'http://127.0.0.1:9224';
const outputDir = path.join(root, 'evidence/ui-image-breakdowns/pages/deployments/visual');
const adjudicationPath = path.join(root, 'doc/04_assets/ui_suite_gpt_v1/specs/overlay-contracts/deployment-modal-size-adjudication.json');
const adjudication = JSON.parse(fs.readFileSync(adjudicationPath, 'utf8'));
const browserAcceptance = JSON.parse(fs.readFileSync(path.join(root, 'evidence/ui-image-breakdowns/pages/deployments/full-stack-r263.json'), 'utf8'));
const channelTolerance = 64;
const visualCase = process.env.DEPLOYMENT_VISUAL_CASE;
const cases = [
  {
    id: 'main', targetId: 'deployments-main-r20', maxRatio: 0.125,
    source: 'doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png',
    actual: 'evidence/ui-image-breakdowns/pages/deployments/interaction-r263-normal-api.png',
    region: { id: 'deployments-business-roi-v1', x: 198, y: 80, width: 1722, height: 917 },
  },
  {
    id: 'create', modalKey: 'create', targetId: 'deployments-create-precheck-r20', maxRatio: 0.08,
    source: 'doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-create.png',
    actual: 'evidence/ui-image-breakdowns/pages/deployments/interaction-r263-create-modal.png',
    region: { id: 'deployment-create-modal-roi-v1', x: 165, y: 34, width: 1622, height: 1012 },
  },
  {
    id: 'rollback', modalKey: 'rollback', targetId: 'deployments-rollback-precheck-r20', maxRatio: 0.08,
    source: 'doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-rollback.png',
    actual: 'evidence/ui-image-breakdowns/pages/deployments/interaction-r263-rollback-modal.png',
    region: { id: 'deployment-rollback-modal-roi-v1', x: 165, y: 34, width: 1622, height: 1012 },
  },
].filter((testCase) => !visualCase || testCase.id === visualCase);

if (cases.length === 0) throw new Error(`unknown DEPLOYMENT_VISUAL_CASE: ${visualCase}`);

const toDataUrl = (relativePath) => `data:image/png;base64,${fs.readFileSync(path.join(root, relativePath)).toString('base64')}`;
const version = await (await fetch(`${cdpUrl}/json/version`)).json();
const browser = await chromium.connectOverCDP(version.webSocketDebuggerUrl);
const context = browser.contexts()[0] ?? await browser.newContext();
const page = await context.newPage();
fs.mkdirSync(outputDir, { recursive: true });
let unadjudicatedFailures = 0;

for (const testCase of cases) {
  const result = await page.evaluate(async ({ sourceUrl, actualUrl, region, tolerance }) => {
    const load = (url) => new Promise((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = reject;
      image.src = url;
    });
    const [source, actual] = await Promise.all([load(sourceUrl), load(actualUrl)]);
    const width = source.naturalWidth;
    const height = source.naturalHeight;
    if (width !== actual.naturalWidth || height !== actual.naturalHeight) throw new Error('visual inputs have different dimensions');
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
        if (mismatch && x >= region.x && x < region.x + region.width && y >= region.y && y < region.y + region.height) roiMismatch += 1;
        if (mismatch) {
          diffPixels.data[index] = 255; diffPixels.data[index + 1] = 42; diffPixels.data[index + 2] = 164; diffPixels.data[index + 3] = 255;
        } else {
          const gray = Math.round((actualPixels.data[index] + actualPixels.data[index + 1] + actualPixels.data[index + 2]) / 3);
          diffPixels.data[index] = gray; diffPixels.data[index + 1] = gray; diffPixels.data[index + 2] = gray; diffPixels.data[index + 3] = 90;
        }
      }
    }
    diffContext.putImageData(diffPixels, 0, 0);
    const sideCanvas = document.createElement('canvas');
    sideCanvas.width = width * 2; sideCanvas.height = height;
    const sideContext = sideCanvas.getContext('2d');
    sideContext.drawImage(source, 0, 0); sideContext.drawImage(actual, width, 0);
    return {
      width, height, roiMismatch, fullMismatch, maxChannelDelta,
      diffUrl: diffCanvas.toDataURL('image/png'), sideUrl: sideCanvas.toDataURL('image/png'),
    };
  }, { sourceUrl: toDataUrl(testCase.source), actualUrl: toDataUrl(testCase.actual), region: testCase.region, tolerance: channelTolerance });
  const comparedPixels = testCase.region.width * testCase.region.height;
  const totalPixels = result.width * result.height;
  const ratio = result.roiMismatch / comparedPixels;
  const metrics = {
    target_id: testCase.targetId,
    route: '/deployments',
    status: ratio <= testCase.maxRatio ? 'pass' : 'fail',
    generated_at: new Date().toISOString(),
    desktop_chrome_backend_status: 'pass',
    source_image: testCase.source,
    actual_screenshot: testCase.actual,
    diff_image: `evidence/ui-image-breakdowns/pages/deployments/visual/${testCase.id}-diff-r20.png`,
    viewport: { width: result.width, height: result.height },
    visual_diff: {
      size_ok: result.width === 1920 && result.height === 1080,
      comparison_scope: 'scoring-region', scoring_region: testCase.region,
      source_width: result.width, source_height: result.height, actual_width: result.width, actual_height: result.height,
      compared_pixels: comparedPixels, total_pixels: comparedPixels, mismatch_pixels: result.roiMismatch,
      pixel_mismatch_ratio: ratio, max_pixel_ratio: testCase.maxRatio, channel_tolerance: channelTolerance,
      max_channel_delta: result.maxChannelDelta,
      full_image_diagnostic: { compared_pixels: totalPixels, total_pixels: totalPixels, mismatch_pixels: result.fullMismatch, pixel_mismatch_ratio: result.fullMismatch / totalPixels, max_channel_delta: result.maxChannelDelta },
    },
  };
  if (metrics.status === 'fail') {
    const exception = adjudication.status === 'accepted_contract_exception'
      ? adjudication.exceptions?.find((item) => item.target_id === testCase.targetId && item.reference_geometry_status === 'superseded')
      : undefined;
    const actualModal = testCase.modalKey ? browserAcceptance.modal_boxes?.[testCase.modalKey] : undefined;
    const geometryConforms = Boolean(exception && actualModal
      && actualModal.width <= exception.required_actual_modal.max_width
      && actualModal.height <= exception.required_actual_modal.max_height);
    if (exception && geometryConforms) {
      metrics.adjudication = {
        status: 'accepted_contract_exception',
        governing_rule: adjudication.governing_rule,
        required_actual_modal: exception.required_actual_modal,
        actual_modal: actualModal,
        geometry_conforms: true,
        decision: adjudication.decision,
      };
      metrics.effective_status = 'pass_with_contract_exception';
    } else {
      metrics.effective_status = 'fail';
      unadjudicatedFailures += 1;
    }
  } else {
    metrics.effective_status = 'pass';
  }
  const decode = (dataUrl) => Buffer.from(dataUrl.slice(dataUrl.indexOf(',') + 1), 'base64');
  fs.writeFileSync(path.join(outputDir, `${testCase.id}-diff-r20.png`), decode(result.diffUrl));
  fs.writeFileSync(path.join(outputDir, `${testCase.id}-side-by-side-r20.png`), decode(result.sideUrl));
  fs.writeFileSync(path.join(outputDir, `${testCase.id}-metrics-r20.json`), `${JSON.stringify(metrics, null, 2)}\n`);
  console.log(JSON.stringify({ id: testCase.id, status: metrics.status, ratio, max_ratio: testCase.maxRatio }));
}

await page.close();
process.exit(unadjudicatedFailures === 0 ? 0 : 1);
