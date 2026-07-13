import fs from 'node:fs';
import path from 'node:path';

const ROOT = process.cwd();
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');
const EXPECTED_TOTAL = 181;
const EXPECTED_COUNTS = {
  foundation: 8,
  page: 27,
  overlay: 70,
  component: 48,
  state: 16,
  responsive: 12,
};
const TARGET_CANVAS = { width: 1920, height: 1080 };
const TARGET_APP_SHELL = {
  shellTopbar: '80px',
  shellSidebar: '166px',
  shellBottombar: '83px',
};

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function exists(relPath) {
  return fs.existsSync(path.join(ROOT, relPath));
}

function pngDimensions(relPath) {
  const file = path.join(ROOT, relPath);
  if (!fs.existsSync(file)) return null;
  const buffer = fs.readFileSync(file);
  const isPng = buffer.length > 24 && buffer.toString('hex', 0, 8) === '89504e470d0a1a0a';
  if (!isPng) return null;
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) };
}

function rawTraceFiles(targetFile) {
  const absolute = path.join(ROOT, targetFile);
  const dir = path.dirname(absolute);
  const base = path.basename(absolute, '.png');
  return ['raw-imagegen', 'raw-deterministic']
    .map((kind) => path.join(dir, `${base}.${kind}.png`))
    .filter((file) => fs.existsSync(file));
}

function fail(message, detail) {
  return { level: 'error', message, detail };
}

function warn(message, detail) {
  return { level: 'warn', message, detail };
}

function groupByType(items) {
  return items.reduce((acc, item) => {
    acc[item.type] = (acc[item.type] ?? 0) + 1;
    return acc;
  }, {});
}

function validateManifest(manifest) {
  const issues = [];
  if (manifest.total !== EXPECTED_TOTAL || manifest.items.length !== EXPECTED_TOTAL) {
    issues.push(fail('manifest total must stay at 181', { total: manifest.total, items: manifest.items.length }));
  }
  const counts = groupByType(manifest.items);
  for (const [type, expected] of Object.entries(EXPECTED_COUNTS)) {
    if (manifest.counts?.[type] !== expected || counts[type] !== expected) {
      issues.push(fail(`manifest count mismatch for ${type}`, { expected, manifest: manifest.counts?.[type], actual: counts[type] ?? 0 }));
    }
  }
  const ids = new Set();
  for (const item of manifest.items) {
    if (ids.has(item.id)) issues.push(fail('duplicate manifest id', item.id));
    ids.add(item.id);
    if (!item.targetFile || !exists(item.targetFile)) issues.push(fail('missing target image', item.id));
    if (!item.promptFile || !exists(item.promptFile)) issues.push(fail('missing prompt file', item.id));
    const dimensions = item.targetFile ? pngDimensions(item.targetFile) : null;
    if (!dimensions || dimensions.width !== TARGET_CANVAS.width || dimensions.height !== TARGET_CANVAS.height) {
      issues.push(fail('target image must be 1920x1080 PNG', { id: item.id, dimensions }));
    }
    if (item.targetFile && rawTraceFiles(item.targetFile).length === 0) {
      issues.push(fail('missing raw trace image', item.id));
    }
  }
  return issues;
}

function validateSpecFiles(manifest) {
  const issues = [];
  const required = [
    'README.md',
    'IMPLEMENTATION_PLAYBOOK.md',
    'index.json',
    'tokens.json',
    'app-shell.json',
    'component-map.json',
    'route-page-map.json',
    'visual-acceptance.json',
    'frontend-delta.md',
    'frontend-task-matrix.json',
    'business-flow-acceptance.json',
    'FRONTEND_TASK_MATRIX.md',
    'BUSINESS_FLOW_ACCEPTANCE.md',
    'FRONTEND_DEV_CHECKLIST.md',
    'FRONTEND_IMPLEMENTATION_METHODS.md',
    'frontend-code-gap.json',
    'FRONTEND_CODE_GAP.md',
    'FRONTEND_FIX_QUEUE.md',
  ];
  for (const relPath of required) {
    const file = path.join(SPEC_DIR, relPath);
    if (!fs.existsSync(file)) issues.push(fail('missing generated spec file', path.relative(ROOT, file)));
  }

  for (const item of manifest.items) {
    const layerFile = path.join(SPEC_DIR, 'layers', `${item.id}.json`);
    if (!fs.existsSync(layerFile)) {
      issues.push(fail('missing layer contract', item.id));
      continue;
    }
    const layer = readJson(layerFile);
    if (layer.id !== item.id || layer.source?.targetFile !== item.targetFile) {
      issues.push(fail('layer contract does not match manifest item', item.id));
    }
    if (!Array.isArray(layer.layers) || layer.layers.length === 0) {
      issues.push(fail('layer contract has no layers', item.id));
    }
  }

  const pageItems = manifest.items.filter((item) => item.type === 'page');
  for (const page of pageItems) {
    const contract = path.join(SPEC_DIR, 'page-contracts', `${page.id}.md`);
    if (!fs.existsSync(contract)) issues.push(fail('missing page contract', page.id));
  }
  if (!fs.existsSync(path.join(SPEC_DIR, 'page-contracts', 'topics.md'))) {
    issues.push(fail('missing merged topics page contract', 'topics'));
  }

  const overlayItems = manifest.items.filter((item) => item.type === 'overlay');
  for (const overlay of overlayItems) {
    const contract = path.join(SPEC_DIR, 'overlay-contracts', `${overlay.id}.md`);
    if (!fs.existsSync(contract)) issues.push(fail('missing overlay contract', overlay.id));
  }
  return issues;
}

function validateRouteAndAcceptance() {
  const issues = [];
  const routeMap = readJson(path.join(SPEC_DIR, 'route-page-map.json'));
  if (routeMap.length !== EXPECTED_COUNTS.page + 1) {
    issues.push(fail('route-page-map must cover 27 page images plus /topics', routeMap.length));
  }
  const routes = new Set(routeMap.map((item) => item.route));
  for (const route of ['/login', '/dashboard', '/topics', '/alerts', '/settings', '*']) {
    if (!routes.has(route)) issues.push(fail('route-page-map missing route', route));
  }

  const acceptance = readJson(path.join(SPEC_DIR, 'visual-acceptance.json'));
  const strictPages = acceptance.global?.appShellPixelStrictPages ?? [];
  if (strictPages.length !== EXPECTED_COUNTS.page - 2) {
    issues.push(fail('app shell strict page count mismatch', strictPages.length));
  }
  for (const forbidden of ['login', 'screen']) {
    if (strictPages.includes(forbidden)) issues.push(fail('login/screen must not be in strict app shell pages', forbidden));
  }
  return issues;
}

function validateAppShellDelta() {
  const appShell = readJson(path.join(SPEC_DIR, 'app-shell.json'));
  const issues = [];
  for (const [key, target] of Object.entries(TARGET_APP_SHELL)) {
    const current = appShell.currentCodeTokens?.[key];
    if (current !== target) issues.push(warn('frontend token differs from UI suite target', { key, target, current }));
  }
  return issues;
}

function validateHandoffFiles() {
  const issues = [];
  const matrixFile = path.join(SPEC_DIR, 'frontend-task-matrix.json');
  const flowsFile = path.join(SPEC_DIR, 'business-flow-acceptance.json');
  if (!fs.existsSync(matrixFile) || !fs.existsSync(flowsFile)) return issues;

  const matrix = readJson(matrixFile);
  const flows = readJson(flowsFile);
  if (matrix.summary?.pages !== EXPECTED_COUNTS.page + 1 || matrix.pages?.length !== EXPECTED_COUNTS.page + 1) {
    issues.push(fail('frontend task matrix page count mismatch', matrix.summary?.pages ?? matrix.pages?.length));
  }
  if (matrix.summary?.overlays !== EXPECTED_COUNTS.overlay || matrix.overlays?.length !== EXPECTED_COUNTS.overlay) {
    issues.push(fail('frontend task matrix overlay count mismatch', matrix.summary?.overlays ?? matrix.overlays?.length));
  }
  if (matrix.summary?.methods !== 5 || matrix.implementationMethods?.length !== 5) {
    issues.push(fail('frontend implementation methods must stay at 5', matrix.summary?.methods ?? matrix.implementationMethods?.length));
  }
  if (!Array.isArray(flows) || flows.length < 6) {
    issues.push(fail('business flow acceptance must cover core business loops', flows.length));
  }
  return issues;
}

function validateCodeGapFiles() {
  const issues = [];
  const gapFile = path.join(SPEC_DIR, 'frontend-code-gap.json');
  if (!fs.existsSync(gapFile)) return issues;
  const gap = readJson(gapFile);
  if (gap.summary?.pages !== EXPECTED_COUNTS.page + 1) {
    issues.push(fail('frontend code gap page count mismatch', gap.summary?.pages));
  }
  if (gap.summary?.pageFilesPresent !== EXPECTED_COUNTS.page + 1) {
    issues.push(fail('frontend code gap found missing page files', gap.summary?.pageFilesPresent));
  }
  if (gap.summary?.routeGaps !== 0) {
    issues.push(fail('frontend code gap found route coverage gaps', gap.summary?.routeGaps));
  }
  if (gap.summary?.directFetchViolations !== 0) {
    issues.push(fail('frontend code gap found direct fetch violations', gap.summary?.directFetchViolations));
  }
  if (gap.summary?.apiEndpointsCovered !== gap.summary?.apiEndpoints) {
    issues.push(fail('frontend code gap found API plan coverage mismatch', { covered: gap.summary?.apiEndpointsCovered, total: gap.summary?.apiEndpoints }));
  }
  if (gap.summary?.appShellTokenGaps) {
    issues.push(warn('frontend code gap keeps AppShell token mismatches', gap.summary.appShellTokenGaps));
  }
  if (gap.summary?.overlaysMissing) {
    issues.push(warn('frontend code gap has overlay contracts not statically traceable', gap.summary.overlaysMissing));
  }
  return issues;
}

function main() {
  const manifest = readJson(MANIFEST_PATH);
  const issues = [
    ...validateManifest(manifest),
    ...validateSpecFiles(manifest),
    ...validateRouteAndAcceptance(),
    ...validateAppShellDelta(),
    ...validateHandoffFiles(),
    ...validateCodeGapFiles(),
  ];
  const errors = issues.filter((issue) => issue.level === 'error');
  const warnings = issues.filter((issue) => issue.level === 'warn');

  for (const issue of issues) {
    const detail = issue.detail === undefined ? '' : ` ${JSON.stringify(issue.detail)}`;
    console.log(`${issue.level.toUpperCase()}: ${issue.message}${detail}`);
  }
  console.log(
    `validated UI frontend contracts: ${manifest.items.length} manifest items, ${EXPECTED_COUNTS.page + 1} route contracts, ${EXPECTED_COUNTS.overlay} overlay contracts`,
  );
  console.log(`errors: ${errors.length}, warnings: ${warnings.length}`);
  if (errors.length) process.exit(1);
}

main();
