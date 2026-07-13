import fs from 'node:fs';
import path from 'node:path';

const ROOT = process.cwd();
const SPEC_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs');
const WEB_DIR = path.join(ROOT, 'web/ui/src');
const TARGET_APP_SHELL = {
  shellTopbar: '80px',
  shellSidebar: '166px',
  shellBottombar: '83px',
};

const SPECIAL_PAGE_RULES = {
  login: {
    dataAccessOk: (text) => text.includes('login(') && text.includes('fetchCaptcha'),
    reason: '登录页使用 login/fetchCaptcha，不走 page snapshot。',
  },
  'not-found': {
    dataAccessOk: () => true,
    reason: '404 页不需要业务 API。',
  },
};

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function readText(file) {
  return fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : '';
}

function writeJson(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function writeMd(file, content) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, content.trimEnd() + '\n');
}

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function cssVar(text, name) {
  const match = text.match(new RegExp(`${name}:\\s*([^;]+);`));
  return match ? match[1].trim() : null;
}

function normalizeEndpoint(endpoint) {
  return endpoint.replace(/^\/api/, '').replace(/:[a-zA-Z]+/g, '{id}');
}

function endpointLiterals(text) {
  return new Set([...text.matchAll(/['"`](\/v1\/[^'"`]+)['"`]/g)].map((match) => normalizeEndpoint(match[1])));
}

function countMatches(text, pattern) {
  return [...text.matchAll(pattern)].length;
}

function dangerousActionLines(text) {
  const destructive = /删除|撤销|回滚|停止|停用|隔离|阻断|封禁|下发|吊销| revoke|delete|rollback|stop|disable|deactivate|block|isolate/i;
  return text
    .split('\n')
    .filter((line) => /\bdanger\b/.test(line) && destructive.test(line))
    .map((line) => line.trim());
}

function pageFile(page) {
  if (!page.codeFile) return null;
  return path.join(ROOT, page.codeFile);
}

function pageSignals(page, text, appText) {
  const combined = `${text}\n${appText}`;
  const special = SPECIAL_PAGE_RULES[page.id];
  const usesQuery = /\buseQuery\s*\(/.test(text);
  const usesService =
    /from ['"]@\/services\//.test(text) ||
    /fetchPageSnapshot|fetchAlertDetail|fetchCampaignDetail|login\(|fetchCaptcha/.test(text);
  const directFetch = /\bfetch\s*\(/.test(text);
  const anyUsage = /(^|[^a-zA-Z])any([^a-zA-Z]|$)|as\s+any/.test(text);
  const loading = /isLoading|loading=|Skeleton|Spin|加载中|submitting/.test(text);
  const error = /isError|message\.error|Alert|catch\s*\(|错误|失败/.test(text);
  const empty = /\bEmpty\b|空数据|暂无|rows\.length|data\?/.test(text);
  const forbidden = /403|权限不足|forbidden|AccessDenied/.test(combined);
  const dangerousLines = dangerousActionLines(text);
  const dangerousActionCount = dangerousLines.length;
  const guardedDanger =
    /Popconfirm|Modal|confirm|二次确认|审批|影响范围|audit|trace|审计/.test(text) ||
    ['login', 'not-found'].includes(page.id);
  const dataAccessOk =
    special?.dataAccessOk(text) ??
    (page.apiEndpoints.length === 0 ? true : usesQuery && usesService && !directFetch);

  return {
    usesQuery,
    usesService,
    directFetch,
    anyUsage,
    loading,
    error,
    empty,
    forbidden,
    dangerousActionCount,
    dangerousLines,
    guardedDanger,
    dataAccessOk,
    dataAccessNote: special?.reason ?? null,
  };
}

function overlayKind(id) {
  if (id.startsWith('drawer-')) return 'Drawer';
  if (id.startsWith('dropdown-')) return 'Dropdown/Menu';
  if (id.startsWith('popconfirm-')) return 'Popconfirm';
  return 'Modal';
}

function overlayHostFiles(overlay, pagesByRoute) {
  const host = pagesByRoute.get(overlay.route);
  const files = [];
  if (host?.codeFile) files.push(path.join(ROOT, host.codeFile));
  if (
    /user-menu|quick-entry|global-search|mobile-navigation|notification-center/.test(overlay.id) ||
    overlay.route === '/settings'
  ) {
    files.push(path.join(WEB_DIR, 'layouts/AppShell.tsx'));
  }
  return [...new Set(files)];
}

function semanticTokens(id) {
  return id
    .split('-')
    .filter((token) => !['modal', 'drawer', 'dropdown', 'popconfirm', 'detail', 'edit'].includes(token))
    .filter((token) => token.length >= 4);
}

function overlaySignals(overlay, pagesByRoute) {
  const files = overlayHostFiles(overlay, pagesByRoute);
  const text = files.map(readText).join('\n');
  const kind = overlayKind(overlay.id);
  const kindPattern =
    kind === 'Modal'
      ? /\bModal\b/
      : kind === 'Drawer'
        ? /\bDrawer\b|Drawer\(|drawer/i
        : kind === 'Dropdown/Menu'
          ? /\bDropdown\b|\bMenuProps\b|dropdown/i
          : /\bPopconfirm\b|popconfirm|CloseableConfirmButton/i;
  const tokens = semanticTokens(overlay.id);
  const tokenHits = tokens.filter((token) => new RegExp(token, 'i').test(text));
  const kindFound = kindPattern.test(text);
  const semanticHintFound = tokenHits.length > 0;
  const status = kindFound && semanticHintFound ? 'confirmed' : kindFound || semanticHintFound ? 'partial' : 'missing';
  return {
    id: overlay.id,
    title: overlay.title,
    route: overlay.route,
    kind,
    hostFiles: files.map(rel),
    kindFound,
    semanticHintFound,
    tokenHits,
    status,
  };
}

function batchStatus(pageGaps, overlayGaps, matrix) {
  return matrix.batches.map((batch) => {
    const pages = matrix.pages.filter((page) => page.batchId === batch.id);
    const pageIds = new Set(pages.map((page) => page.id));
    const pageRoutes = new Set(pages.map((page) => page.route));
    const batchPageGaps = pageGaps.filter((gap) => pageIds.has(gap.pageId));
    const batchOverlayGaps = overlayGaps.filter((gap) => pageRoutes.has(gap.route));
    return {
      id: batch.id,
      title: batch.title,
      pageCount: pages.length,
      overlayCount: batchOverlayGaps.length,
      missingOverlayCount: batchOverlayGaps.filter((gap) => gap.status === 'missing').length,
      partialOverlayCount: batchOverlayGaps.filter((gap) => gap.status === 'partial').length,
      pageGapCount: batchPageGaps.length,
      nextAction:
        batch.id === '00-foundation'
          ? '先对齐 AppShell token 和全局浮层入口。'
          : batchPageGaps.length || batchOverlayGaps.some((gap) => gap.status !== 'confirmed')
            ? '按本批次缺口逐页补齐状态、危险动作保护和浮层触发。'
            : '进入截图回归和业务链路验收。',
    };
  });
}

function partialOverlayAction(items) {
  const hostFiles = [...new Set(items.flatMap((item) => item.hostFiles))].join(', ');
  const missingKinds = items
    .filter((item) => !item.kindFound)
    .map((item) => `${item.id} -> ${item.kind}`)
    .join('；');
  const missingSemantics = items
    .filter((item) => !item.semanticHintFound)
    .map((item) => `${item.id} -> ${semanticTokens(item.id).join('/')}`)
    .join('；');
  const parts = [];
  if (missingKinds) parts.push(`补容器：${missingKinds}`);
  if (missingSemantics) parts.push(`补语义触发/标题：${missingSemantics}`);
  parts.push(`目标文件：${hostFiles}`);
  parts.push('完成后做截图验收，并保留权限提示、影响范围和审计 trace。');
  return parts.join('。');
}

function buildFixQueue({ appShellGaps, routeGaps, pageGaps, overlaySignalsList, batchSummaries }) {
  const tasks = [];
  for (const gap of appShellGaps) {
    tasks.push({
      priority: 'P0',
      area: 'AppShell',
      target: 'web/ui/src/styles/tokens.css',
      issue: `${gap.key} 当前 ${gap.current ?? '未设置'}，UI 契约目标 ${gap.target}`,
      action: '先对齐公共尺寸，再进行页面视觉回归。',
    });
  }
  for (const gap of routeGaps) {
    tasks.push({
      priority: 'P0',
      area: 'Route',
      target: gap.pageId,
      issue: gap.message,
      action: '补齐路由、懒加载或页面文件后再开发业务页面。',
    });
  }
  for (const gap of pageGaps) {
    tasks.push({
      priority: gap.severity,
      area: 'Page',
      target: gap.pageId,
      issue: gap.message,
      action: gap.action,
    });
  }
  const missingOverlays = overlaySignalsList.filter((overlay) => overlay.status === 'missing');
  const partialOverlays = overlaySignalsList.filter((overlay) => overlay.status === 'partial');
  for (const group of groupBy(missingOverlays, (item) => item.route)) {
    tasks.push({
      priority: 'P1',
      area: 'Overlay',
      target: group.key,
      issue: `${group.items.length} 个浮层未被静态识别：${group.items.map((item) => item.id).join(', ')}`,
      action: '按 overlay-contracts 实现触发入口、容器组件、权限提示、影响范围和审计 trace。',
    });
  }
  for (const group of groupBy(partialOverlays, (item) => item.route)) {
    tasks.push({
      priority: 'P2',
      area: 'Overlay',
      target: group.key,
      issue: `${group.items.length} 个浮层只有部分静态信号：${group.items.map((item) => item.id).join(', ')}`,
      action: partialOverlayAction(group.items),
    });
  }
  for (const batch of batchSummaries.filter((item) => item.pageGapCount || item.missingOverlayCount || item.partialOverlayCount)) {
    tasks.push({
      priority: 'P3',
      area: 'Batch',
      target: batch.id,
      issue: `批次仍有页面缺口 ${batch.pageGapCount}、缺失浮层 ${batch.missingOverlayCount}、部分浮层 ${batch.partialOverlayCount}`,
      action: batch.nextAction,
    });
  }
  return tasks;
}

function groupBy(items, keyFn) {
  const groups = new Map();
  for (const item of items) {
    const key = keyFn(item);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(item);
  }
  return [...groups.entries()].map(([key, groupItems]) => ({ key, items: groupItems }));
}

function tableRow(cells) {
  return `| ${cells.join(' | ')} |`;
}

function reportMd(report) {
  const stateText = (page) => {
    if (page.id === 'login') return '登录专用';
    if (page.id === 'not-found') return '404 专用';
    return [page.signals.loading ? 'loading' : '', page.signals.error ? 'error' : '', page.signals.empty ? 'empty' : ''].filter(Boolean).join('/') || '不足';
  };
  const overview = [
    tableRow(['类别', '结果']),
    tableRow(['---', '---:']),
    tableRow(['页面文件覆盖', `${report.summary.pageFilesPresent}/${report.summary.pages}`]),
    tableRow(['路由/懒加载覆盖', `${report.summary.routesCovered}/${report.summary.pages}`]),
    tableRow(['契约 API 在 pageApiPlans 覆盖', `${report.summary.apiEndpointsCovered}/${report.summary.apiEndpoints}`]),
    tableRow(['直接 fetch 违规', `${report.summary.directFetchViolations}`]),
    tableRow(['AppShell token 缺口', `${report.summary.appShellTokenGaps}`]),
    tableRow(['浮层 confirmed/partial/missing', `${report.summary.overlaysConfirmed}/${report.summary.overlaysPartial}/${report.summary.overlaysMissing}`]),
  ].join('\n');

  const appShellRows = report.appShell.gaps.length
    ? report.appShell.gaps.map((gap) => `- ${gap.key}：当前 \`${gap.current ?? '未设置'}\`，目标 \`${gap.target}\`。`).join('\n')
    : '- 已与 UI 契约一致。';

  const pageRows = [
    tableRow(['页面', '文件', '数据接入', '状态覆盖', '危险动作保护', '备注']),
    tableRow(['---', '---', '---', '---', '---', '---']),
    ...report.pages.map((page) =>
      tableRow([
        `\`${page.id}\``,
        page.fileExists ? '有' : '缺失',
        page.signals.dataAccessOk ? '通过' : '需修正',
        stateText(page),
        page.signals.dangerousActionCount ? (page.signals.guardedDanger ? '需人工验真' : '缺少保护') : '无危险动作',
        page.signals.dataAccessNote ?? '',
      ]),
    ),
  ].join('\n');

  const overlayRows = [
    tableRow(['状态', '数量', '说明']),
    tableRow(['---', '---:', '---']),
    tableRow(['confirmed', String(report.summary.overlaysConfirmed), '同时发现语义线索和目标容器组件。']),
    tableRow(['partial', String(report.summary.overlaysPartial), '只发现语义线索或容器组件，需人工核查。']),
    tableRow(['missing', String(report.summary.overlaysMissing), '未发现可追踪实现，需按契约补齐。']),
  ].join('\n');

  const topQueue = report.fixQueue
    .slice(0, 20)
    .map((task, index) => `${index + 1}. \`${task.priority}\` ${task.area} ${task.target}：${task.issue} ${task.action}`)
    .join('\n');

  return `# 前端代码差距报告

本报告把 UI 契约与当前 \`web/ui\` 静态代码对齐，输出可执行的修复队列。它不替代浏览器截图和真实 API 验收，但能先挡住明显的开发偏差。

## 总览

${overview}

## AppShell 公共参数

${appShellRows}

## 页面代码覆盖

${pageRows}

## 浮层静态追踪

${overlayRows}

## 优先修复队列 Top 20

${topQueue || '当前无静态缺口。'}

## 使用方式

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/build_frontend_contracts.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_handoff.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
\`\`\`
`;
}

function fixQueueMd(tasks) {
  if (!tasks.length) {
    return `# 前端修复队列

本文件按优先级列出当前代码相对 UI 契约的可执行修复项。P0/P1 先处理，P2/P3 在页面批次内处理。

当前无静态缺口。
`;
  }

  const rows = [
    tableRow(['优先级', '区域', '目标', '问题', '动作']),
    tableRow(['---', '---', '---', '---', '---']),
    ...tasks.map((task) => tableRow([task.priority, task.area, `\`${task.target}\``, task.issue, task.action])),
  ].join('\n');
  return `# 前端修复队列

本文件按优先级列出当前代码相对 UI 契约的可执行修复项。P0/P1 先处理，P2/P3 在页面批次内处理。

${rows}
`;
}

function main() {
  const matrix = readJson(path.join(SPEC_DIR, 'frontend-task-matrix.json'));
  const appShell = readJson(path.join(SPEC_DIR, 'app-shell.json'));
  const appText = readText(path.join(WEB_DIR, 'App.tsx'));
  const routeText = readText(path.join(WEB_DIR, 'routes/routeManifest.tsx'));
  const tokensText = readText(path.join(WEB_DIR, 'styles/tokens.css'));
  const apiPlanText = readText(path.join(WEB_DIR, 'services/pageApiPlans.ts'));
  const plannedEndpoints = endpointLiterals(apiPlanText);

  const appShellGaps = Object.entries(TARGET_APP_SHELL)
    .map(([key, target]) => ({ key, target, current: appShell.currentCodeTokens?.[key] ?? cssVar(tokensText, `--${key.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}`) }))
    .filter((item) => item.current !== item.target);

  const pagesByRoute = new Map(matrix.pages.map((page) => [page.route, page]));
  const routeGaps = [];
  const pageReports = matrix.pages.map((page) => {
    const file = pageFile(page);
    const fileExists = file ? fs.existsSync(file) : false;
    const text = file ? readText(file) : '';
    const lazyCovered = page.pageComponent ? appText.includes(`const ${page.pageComponent}`) || appText.includes(`<${page.pageComponent}`) : false;
    const routeManifestCovered =
      page.route === '*'
        ? appText.includes('path="*"')
        : routeText.includes(`'${page.route}'`) || routeText.includes(`"${page.route}"`) || appText.includes(`path="${page.route}"`);
    if (!fileExists) routeGaps.push({ pageId: page.id, message: `页面文件不存在：${page.codeFile}` });
    if (!lazyCovered && page.pageComponent) routeGaps.push({ pageId: page.id, message: `App.tsx 未发现 ${page.pageComponent} 懒加载或渲染分支` });
    if (!routeManifestCovered && page.route !== '/topics') routeGaps.push({ pageId: page.id, message: `routeManifest/App 未发现路由 ${page.route}` });

    const normalizedEndpoints = page.apiEndpoints.map(normalizeEndpoint);
    const apiCoverage = normalizedEndpoints.map((endpoint) => ({
      endpoint,
      covered: plannedEndpoints.has(endpoint),
    }));
    const missingApi = apiCoverage.filter((item) => !item.covered);
    const signals = pageSignals(page, text, appText);
    return {
      id: page.id,
      title: page.title,
      route: page.route,
      batchId: page.batchId,
      pageComponent: page.pageComponent,
      codeFile: page.codeFile,
      fileExists,
      lazyCovered,
      routeManifestCovered,
      apiCoverage,
      missingApi,
      signals,
    };
  });

  const pageGaps = [];
  for (const page of pageReports) {
    if (page.missingApi.length) {
      pageGaps.push({
        pageId: page.id,
        severity: 'P1',
        message: `契约 API 未在 pageApiPlans.ts 中覆盖：${page.missingApi.map((item) => item.endpoint).join(', ')}`,
        action: '补齐 pageApiPlans.ts，并通过 services/api.ts 统一请求。',
      });
    }
    if (!page.signals.dataAccessOk) {
      pageGaps.push({
        pageId: page.id,
        severity: 'P1',
        message: '页面未能静态证明使用 React Query + service 接入契约 API。',
        action: '补齐 useQuery/service hook，保留 loading/error/empty 状态。',
      });
    }
    const stateRequired = !['login', 'not-found'].includes(page.id);
    if (stateRequired && (!page.signals.loading || !page.signals.error || (!page.signals.empty && page.id !== 'screen'))) {
      pageGaps.push({
        pageId: page.id,
        severity: 'P2',
        message: '页面状态覆盖不足或未能静态识别完整 loading/error/empty。',
        action: '补齐状态组件，必要时使用 state-* 设计图作为参考。',
      });
    }
    if (page.signals.dangerousActionCount && !page.signals.guardedDanger) {
      pageGaps.push({
        pageId: page.id,
        severity: 'P1',
        message: `发现 ${page.signals.dangerousActionCount} 个 danger 动作但缺少二次确认/影响范围/审计线索。`,
        action: '补 Popconfirm/Modal、影响范围说明和审计 trace。',
      });
    }
  }

  const overlaySignalsList = matrix.overlays.map((overlay) => overlaySignals(overlay, pagesByRoute));
  const batchSummaries = batchStatus(pageGaps, overlaySignalsList, matrix);
  const apiEndpoints = pageReports.flatMap((page) => page.apiCoverage);
  const fixQueue = buildFixQueue({
    appShellGaps,
    routeGaps,
    pageGaps,
    overlaySignalsList,
    batchSummaries,
  });
  const report = {
    generatedAt: new Date().toISOString(),
    summary: {
      pages: pageReports.length,
      pageFilesPresent: pageReports.filter((page) => page.fileExists).length,
      routesCovered: pageReports.filter((page) => page.lazyCovered && page.routeManifestCovered).length,
      apiEndpoints: apiEndpoints.length,
      apiEndpointsCovered: apiEndpoints.filter((item) => item.covered).length,
      directFetchViolations: pageReports.filter((page) => page.signals.directFetch).length,
      appShellTokenGaps: appShellGaps.length,
      overlays: overlaySignalsList.length,
      overlaysConfirmed: overlaySignalsList.filter((item) => item.status === 'confirmed').length,
      overlaysPartial: overlaySignalsList.filter((item) => item.status === 'partial').length,
      overlaysMissing: overlaySignalsList.filter((item) => item.status === 'missing').length,
      pageGaps: pageGaps.length,
      routeGaps: routeGaps.length,
      fixQueue: fixQueue.length,
    },
    appShell: {
      target: TARGET_APP_SHELL,
      current: appShell.currentCodeTokens,
      gaps: appShellGaps,
    },
    routeGaps,
    pages: pageReports,
    pageGaps,
    overlays: overlaySignalsList,
    batches: batchSummaries,
    fixQueue,
  };

  writeJson(path.join(SPEC_DIR, 'frontend-code-gap.json'), report);
  writeMd(path.join(SPEC_DIR, 'FRONTEND_CODE_GAP.md'), reportMd(report));
  writeMd(path.join(SPEC_DIR, 'FRONTEND_FIX_QUEUE.md'), fixQueueMd(fixQueue));

  console.log(`frontend code gap generated: ${rel(SPEC_DIR)}`);
  console.log(`pages: ${report.summary.pageFilesPresent}/${report.summary.pages}`);
  console.log(`api endpoints: ${report.summary.apiEndpointsCovered}/${report.summary.apiEndpoints}`);
  console.log(`direct fetch violations: ${report.summary.directFetchViolations}`);
  console.log(`app shell gaps: ${report.summary.appShellTokenGaps}`);
  console.log(
    `overlays confirmed/partial/missing: ${report.summary.overlaysConfirmed}/${report.summary.overlaysPartial}/${report.summary.overlaysMissing}`,
  );
  console.log(`fix queue: ${report.summary.fixQueue}`);
}

main();
