import fs from 'node:fs';
import path from 'node:path';

const ROOT = process.cwd();
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');
const ROUTE_MAP_PATH = path.join(SPEC_DIR, 'route-page-map.json');

const DOMAIN_LABELS = {
  overview: '综合态势',
  'collection-monitoring': '采集监控',
  'threat-analysis': '威胁分析',
  'asset-graph': '资产图谱',
  'detection-ops': '检测运营',
  'audit-config': '审计配置',
  auth: '认证入口',
  global: '全局',
};

const BATCHES = [
  {
    id: '00-foundation',
    title: '公共骨架与组件底座',
    goal: '先统一 AppShell、token、Ant Design 组件封装、状态页和响应式规则，避免每个页面重复返工。',
    pageIds: [],
    sourceContracts: ['tokens.json', 'app-shell.json', 'component-map.json', 'visual-acceptance.json'],
    entryFiles: [
      'web/ui/src/styles/tokens.css',
      'web/ui/src/styles/app-shell.css',
      'web/ui/src/layouts/AppShell.tsx',
      'web/ui/src/components/*.tsx',
    ],
  },
  {
    id: '01-core-entry',
    title: '入口、总览与专题统一页',
    goal: '打通登录、仪表盘、大屏、专题三态，形成所有页面复用的导航和闭环栏样板。',
    pageIds: ['login', 'dashboard', 'screen', 'topics'],
    sourceContracts: ['page-contracts/login.md', 'page-contracts/dashboard.md', 'page-contracts/screen.md', 'page-contracts/topics.md'],
    entryFiles: ['web/ui/src/App.tsx', 'web/ui/src/pages/LoginPage.tsx', 'web/ui/src/pages/DashboardOperationsPage.tsx', 'web/ui/src/pages/TopicWorkbenchPage.tsx'],
  },
  {
    id: '02-threat-forensics',
    title: '告警、战役、攻击链、加密流量与取证闭环',
    goal: '完成核心安全分析闭环：告警研判、详情证据、战役聚类、攻击路径、取证导出和审计留痕。',
    pageIds: ['alerts', 'alert-detail', 'campaigns', 'campaign-detail', 'attack-chains', 'encrypted-traffic', 'forensics'],
    sourceContracts: ['page-contracts/alerts.md', 'page-contracts/alert-detail.md', 'page-contracts/campaigns.md', 'page-contracts/encrypted-traffic.md', 'page-contracts/forensics.md'],
    entryFiles: ['web/ui/src/pages/AlertTriagePage.tsx', 'web/ui/src/pages/AlertDetailPage.tsx', 'web/ui/src/pages/CampaignWorkbenchPage.tsx', 'web/ui/src/pages/EncryptedTrafficPage.tsx', 'web/ui/src/pages/ForensicsWorkbenchPage.tsx'],
  },
  {
    id: '03-collection-asset',
    title: '采集质量、资产图谱与融合基线',
    goal: '证明数据来源可信、资产画像可追踪、图谱/融合/基线能支撑告警解释。',
    pageIds: ['probes', 'data-quality', 'assets', 'graph', 'fusion', 'baselines'],
    sourceContracts: ['page-contracts/probes.md', 'page-contracts/data-quality.md', 'page-contracts/assets.md', 'page-contracts/graph.md', 'page-contracts/fusion.md', 'page-contracts/baselines.md'],
    entryFiles: ['web/ui/src/pages/ProbesManagementPage.tsx', 'web/ui/src/pages/DataQualityPage.tsx', 'web/ui/src/pages/AssetInventoryPage.tsx', 'web/ui/src/pages/GraphEntityPage.tsx'],
  },
  {
    id: '04-detection-ops',
    title: '规则、部署、模型、MLOps、剧本和白名单',
    goal: '把检测能力从规则编辑推进到发布、模型激活、剧本执行和白名单治理。',
    pageIds: ['rules', 'deployments', 'models', 'mlops', 'playbooks', 'whitelist'],
    sourceContracts: ['page-contracts/rules.md', 'page-contracts/deployments.md', 'page-contracts/models.md', 'page-contracts/mlops.md', 'page-contracts/playbooks.md', 'page-contracts/whitelist.md'],
    entryFiles: ['web/ui/src/pages/RuleManagementPage.tsx', 'web/ui/src/pages/DeploymentManagementPage.tsx', 'web/ui/src/pages/ModelManagementPage.tsx', 'web/ui/src/pages/MlopsOrchestrationPage.tsx'],
  },
  {
    id: '05-audit-config',
    title: '合规、审计、通知、系统设置与异常页',
    goal: '补齐验收证据、审计查询、通知升级、租户配置、权限与 API token 管理。',
    pageIds: ['compliance', 'audit-log', 'notifications', 'settings', 'not-found'],
    sourceContracts: ['page-contracts/compliance.md', 'page-contracts/audit-log.md', 'page-contracts/notifications.md', 'page-contracts/settings.md', 'page-contracts/not-found.md'],
    entryFiles: ['web/ui/src/pages/ComplianceAuditPage.tsx', 'web/ui/src/pages/AuditLogPage.tsx', 'web/ui/src/pages/NotificationConfigPage.tsx', 'web/ui/src/pages/SettingsGovernancePage.tsx'],
  },
];

const FLOW_DEFINITIONS = [
  {
    id: 'auth-access',
    title: '登录、会话、权限与审计入口',
    pages: ['login', 'dashboard', 'screen', 'settings', 'audit-log'],
    success: ['未登录跳转登录页', '权限不足展示 403 且列出 requiredScopes', '只读大屏受 screen:view 或 masked-demo 门禁约束', '用户菜单能进入设置/审计并可退出', '敏感操作写入审计线索'],
  },
  {
    id: 'daily-soc-loop',
    title: '日常值班闭环：仪表盘 -> 告警 -> 证据 -> 反馈',
    pages: ['dashboard', 'alerts', 'alert-detail', 'forensics', 'whitelist', 'audit-log'],
    success: ['仪表盘待办可定位到告警队列', '告警详情展示证据链和状态机', '处置/反馈必须经过二次确认或权限提示', '取证下载和反馈操作留痕'],
  },
  {
    id: 'topic-investigation',
    title: '专题研判闭环：隧道、外传、APT 三态',
    pages: ['topics', 'encrypted-traffic', 'campaigns', 'campaign-detail', 'attack-chains', 'forensics'],
    success: ['专题切换保留筛选上下文', '专题报告/证据包导出有权限与影响范围', '可下钻到加密流量、战役、攻击链和取证', '审计 trace 可回放'],
  },
  {
    id: 'asset-evidence-loop',
    title: '资产证据闭环：资产 -> 图谱 -> 融合 -> 基线',
    pages: ['assets', 'graph', 'fusion', 'baselines', 'alerts'],
    success: ['资产详情、实体图谱和告警上下文互跳', '融合冲突可解释并可审计', '基线阈值调整有预览和回滚', '风险分数与证据来源对应'],
  },
  {
    id: 'data-quality-loop',
    title: '采集质量闭环：探针 -> 数据质量 -> DLQ/重放',
    pages: ['probes', 'data-quality', 'mlops', 'compliance'],
    success: ['探针离线、背压、字段缺失有专门状态', 'DLQ 样本可查看和重放', '质量指标进入验收/合规视图', '所有修复动作记录操作者和时间窗'],
  },
  {
    id: 'detection-release-loop',
    title: '检测发布闭环：规则 -> 部署 -> 模型 -> 剧本 -> 白名单',
    pages: ['rules', 'deployments', 'models', 'mlops', 'playbooks', 'whitelist'],
    success: ['规则发布前必须测试验证', '部署支持灰度、回滚和审计', '模型激活可回退', '剧本触发有风险控制', '白名单有审批和到期治理'],
  },
  {
    id: 'governance-loop',
    title: '治理闭环：合规、审计、通知、系统配置',
    pages: ['compliance', 'audit-log', 'notifications', 'settings'],
    success: ['合规证据包可导出', '审计日志可按用户/对象/动作追踪', '通知升级和静默规则可测试', 'API token 创建/撤销必须权限提示和二次确认'],
  },
];

const IMPLEMENTATION_METHODS = [
  {
    id: 'layer-contract-first',
    title: '分层 JSON 契约法',
    priority: '主方法',
    input: ['layers/<id>.json', 'tokens.json', 'app-shell.json', '目标 PNG'],
    output: ['页面区域坐标', '组件角色', '验收 checklist'],
    risk: '需要把 bbox 转成响应式 CSS 约束，不能只写固定像素。',
  },
  {
    id: 'component-system-first',
    title: '组件库优先法',
    priority: '主方法',
    input: ['component-map.json', '48 张 component 图', '现有 Ant Design/ECharts 封装'],
    output: ['可复用 WorkPanel/MetricTile/StatusTag/Chart/Table/Form 模块'],
    risk: '组件抽象过早会拖慢页面，必须从真实页面复用点反推。',
  },
  {
    id: 'route-api-contract',
    title: '路由与 API 契约法',
    priority: '主方法',
    input: ['route-page-map.json', 'web/ui/src/routes/routeManifest.tsx', 'web/ui/src/services/*.ts'],
    output: ['页面数据 hook', 'loading/error/empty 状态', '权限 requiredScopes'],
    risk: '不能在页面中直接 fetch，也不能用 mock 掩盖真实 API 缺口。',
  },
  {
    id: 'visual-regression',
    title: '截图回归法',
    priority: '验收方法',
    input: ['visual-acceptance.json', '目标 PNG', 'Playwright 1920x1080 截图'],
    output: ['公共区像素级差异', '主工作区结构差异', '交互状态截图证据'],
    risk: '截图差异只能证明视觉接近，业务动作仍需 API 和审计验证。',
  },
  {
    id: 'annotated-review',
    title: '人工标注评审法',
    priority: '补充方法',
    input: ['BUSINESS_FLOW_ACCEPTANCE.md', '页面 PR 截图', '产品/安全专家评审意见'],
    output: ['业务语义修正', '缺失动作或危险动作提示', '专家验收记录'],
    risk: '不能替代自动校验，评审意见必须回写到契约或代码任务。',
  },
];

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function writeJson(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function writeMd(file, content) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, content.trimEnd() + '\n');
}

function loadLayer(id) {
  return readJson(path.join(SPEC_DIR, 'layers', `${id}.json`));
}

function overlayKind(id) {
  if (id.startsWith('drawer-')) return 'Drawer';
  if (id.startsWith('dropdown-')) return 'Dropdown/Menu';
  if (id.startsWith('popconfirm-')) return 'Popconfirm';
  return 'Modal';
}

function batchForPage(id) {
  return BATCHES.find((batch) => batch.pageIds.includes(id)) ?? BATCHES[0];
}

function pageCodeFile(pageComponent) {
  return pageComponent ? `web/ui/src/pages/${pageComponent}.tsx` : null;
}

function sourceImageFor(page, layer) {
  return page.sourceImage ?? layer?.source?.targetFile ?? null;
}

function compactList(values, fallback = '无') {
  const list = values.filter(Boolean);
  return list.length ? list.join('、') : fallback;
}

function tableRow(cells) {
  return `| ${cells.join(' | ')} |`;
}

function pageRows(routeMap, layers, overlays) {
  return routeMap.map((page) => {
    const layer = page.id === 'topics' ? null : layers.get(page.id);
    const batch = batchForPage(page.id);
    const relatedOverlays = overlays.filter((overlay) => overlay.route === page.route);
    return {
      id: page.id,
      title: layer?.title ?? page.title,
      route: page.route,
      domain: page.domain,
      domainLabel: DOMAIN_LABELS[page.domain] ?? page.domain,
      batchId: batch.id,
      batchTitle: batch.title,
      pageComponent: page.pageComponent,
      codeFile: pageCodeFile(page.pageComponent),
      sourceImage: sourceImageFor(page, layer),
      markdownContract: page.contract,
      layerContract: page.id === 'topics' ? null : `doc/04_assets/ui_suite_gpt_v1/specs/layers/${page.id}.json`,
      apiEndpoints: page.apiEndpoints,
      workflows: layer?.implementation?.workflows ?? [],
      overlayIds: relatedOverlays.map((overlay) => overlay.id),
      overlayCount: relatedOverlays.length,
      acceptance: layer?.acceptance ?? [],
    };
  });
}

function overlayRows(overlays) {
  return overlays.map((overlay) => ({
    id: overlay.id,
    title: overlay.title,
    route: overlay.route ?? 'global',
    kind: overlayKind(overlay.id),
    sourceImage: overlay.source?.targetFile,
    markdownContract: `doc/04_assets/ui_suite_gpt_v1/specs/overlay-contracts/${overlay.id}.md`,
    layerContract: `doc/04_assets/ui_suite_gpt_v1/specs/layers/${overlay.id}.json`,
    apiEndpoints: overlay.implementation?.apiEndpoints ?? [],
    acceptance: overlay.acceptance ?? [],
  }));
}

function batchRows(pages, overlays, items) {
  const foundationCounts = {
    foundation: items.filter((item) => item.type === 'foundation').length,
    component: items.filter((item) => item.type === 'component').length,
    state: items.filter((item) => item.type === 'state').length,
    responsive: items.filter((item) => item.type === 'responsive').length,
  };
  return BATCHES.map((batch) => {
    const pageItems = pages.filter((page) => page.batchId === batch.id);
    const pageRoutes = new Set(pageItems.map((page) => page.route));
    return {
      ...batch,
      pages: pageItems.map((page) => page.id),
      routes: pageItems.map((page) => page.route),
      overlayCount: overlays.filter((overlay) => pageRoutes.has(overlay.route)).length,
      foundationCounts: batch.id === '00-foundation' ? foundationCounts : undefined,
      doneWhen: [
        '契约自检无 error',
        '相关页面 npm run build 通过',
        '页面 loading/error/empty/403 状态可触发',
        '危险动作具备权限提示、影响范围、审计 trace 和二次确认',
      ],
    };
  });
}

function flowRows(flows, pages, overlays) {
  const pagesById = new Map(pages.map((page) => [page.id, page]));
  return flows.map((flow) => {
    const flowPages = flow.pages.map((id) => pagesById.get(id)).filter(Boolean);
    const routeSet = new Set(flowPages.map((page) => page.route));
    const flowOverlays = overlays.filter((overlay) => routeSet.has(overlay.route));
    return {
      ...flow,
      routes: flowPages.map((page) => page.route),
      apiEndpoints: [...new Set(flowPages.flatMap((page) => page.apiEndpoints))],
      overlays: flowOverlays.map((overlay) => overlay.id),
      contracts: flowPages.map((page) => page.markdownContract),
    };
  });
}

function matrixMd(matrix) {
  const pageTable = [
    tableRow(['批次', '页面', '路由', 'React 页面', 'API', '浮层', '契约']),
    tableRow(['---', '---', '---', '---', '---:', '---:', '---']),
    ...matrix.pages.map((page) =>
      tableRow([
        page.batchId,
        page.title,
        `\`${page.route}\``,
        `\`${page.pageComponent}\``,
        String(page.apiEndpoints.length),
        String(page.overlayCount),
        `\`${page.markdownContract}\``,
      ]),
    ),
  ].join('\n');

  const batchList = matrix.batches
    .map((batch) => {
      const foundation = batch.foundationCounts
        ? ` foundation=${batch.foundationCounts.foundation}, component=${batch.foundationCounts.component}, state=${batch.foundationCounts.state}, responsive=${batch.foundationCounts.responsive}`
        : '';
      return `- \`${batch.id}\` ${batch.title}：${batch.goal} 页面=${batch.pages.length}，浮层=${batch.overlayCount}${foundation}`;
    })
    .join('\n');

  return `# 前端任务矩阵

本文件把 UI 图契约整理成前端可派工的批次、页面、路由、API 和浮层依赖。前端开发以批次推进，不按 PNG 零散推进。

## 批次顺序

${batchList}

## 页面矩阵

${pageTable}

## 浮层实现规则

- Modal：承载少量表单、导出、发布、确认前预览等交互；桌面端必须使用小尺寸弹窗，不得铺满或遮住整个浏览器业务区域。
- Drawer：优先承载详情、证据、日志、路径分析等上下文下钻；从侧面滑出并保留宿主业务上下文可见，不得做成全屏覆盖层。
- Dropdown/Menu：承载行操作、快捷入口和分享收藏等轻量操作。
- Popconfirm：只用于删除、撤销、下载等需要二次确认的短动作。
- 业务详情内容较少时优先使用窄 Drawer，其次使用小 Modal；只有独立页面级工作流才允许占据完整业务区，验收专用 focus 截图状态不等同于生产弹层。

## 完成定义

- 当前批次所有页面契约已实现，相关浮层均可触发。
- 页面使用 \`services/api.ts\` 或现有 service，不直接 \`fetch\`。
- 每个页面覆盖 loading、error、empty、403/401 中的适用状态。
- 运行 \`node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs\` 无 error。
- 运行 \`cd web/ui && npm run build\`，页面级变更再补 Playwright 截图证据。
`;
}

function flowMd(flows) {
  const sections = flows
    .map(
      (flow) => `## ${flow.title}

- 路由：${flow.routes.map((route) => `\`${route}\``).join(' -> ')}
- API：${compactList(flow.apiEndpoints.map((api) => `\`${api}\``))}
- 浮层：${compactList(flow.overlays.map((id) => `\`${id}\``))}
- 契约：${flow.contracts.map((file) => `\`${file}\``).join('、')}
- 验收：
${flow.success.map((item) => `  - ${item}`).join('\n')}
`,
    )
    .join('\n');

  return `# 业务链路验收清单

本文件从业务闭环角度约束前端实现，防止页面视觉完成但业务动作断链。

${sections}

## 通用业务底线

- 所有危险动作必须包含权限提示、影响范围、审计 trace 和取消/确认动作。
- 页面内状态流转必须能从数据驱动，不能只写静态卡片。
- 同一业务对象跨页面跳转时必须保留对象 ID、时间窗、租户/站点上下文。
- 导出、下载、撤销、发布、回滚、封禁、隔离等动作必须可追溯。
`;
}

function checklistMd() {
  return `# 前端开发 Checklist

## 开发前

- 读取 \`FRONTEND_TASK_MATRIX.md\`，确认所在批次和前置依赖。
- 读取 \`page-contracts/<id>.md\` 和 \`layers/<id>.json\`。
- 打开对应目标 PNG，对照 \`app-shell.json\` 和 \`tokens.json\`。
- 检查 \`route-page-map.json\` 中的 React 页面、API endpoint 和契约路径。
- 检查关联 \`overlay-contracts/\`，不要漏掉行操作、批量操作和详情抽屉。

## 开发中

- 先补 service/hook，再写页面视图。
- 表格、图表、KPI、时间线、证据卡片优先复用 \`component-map.json\` 对应组件。
- 页面状态至少覆盖 loading、error、empty；鉴权页覆盖 401/403。
- 危险操作先做不可误触状态，再接入真实动作。
- AppShell 公共区不要在单页内重写。

## 提交前

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
cd web/ui && npm run build
\`\`\`

浏览器验收页面时需确认：

- 无 4xx/5xx API response。
- 无 requestfailed。
- 无 pageerror 或非 warning console error。
- 1920x1080 截图公共区与目标 AppShell 参数一致。
- 相关浮层能从页面真实入口触发。
`;
}

function methodsMd(methods) {
  const sections = methods
    .map(
      (method, index) => `## ${index + 1}. ${method.title}

- 优先级：${method.priority}
- 输入：${method.input.map((item) => `\`${item}\``).join('、')}
- 输出：${method.output.join('、')}
- 风险：${method.risk}
`,
    )
    .join('\n');
  return `# 前端准确实现 UI 图的 5 种方法

本项目推荐把 5 种方法组合使用：前三种指导开发，第四种做自动验收，第五种处理业务语义和专家意见。

${sections}

## 推荐组合

- 新页面：分层 JSON 契约法 + 路由与 API 契约法。
- 公共控件：组件库优先法 + 截图回归法。
- 关键业务闭环：路由与 API 契约法 + 人工标注评审法。
- 视觉争议：截图回归法作为证据，人工评审决定是否修改契约。
`;
}

function main() {
  const manifest = readJson(MANIFEST_PATH);
  const routeMap = readJson(ROUTE_MAP_PATH);
  const layers = new Map(manifest.items.map((item) => [item.id, loadLayer(item.id)]));
  const layerList = [...layers.values()];
  const overlays = overlayRows(layerList.filter((item) => item.type === 'overlay'));
  const pages = pageRows(routeMap, layers, overlays);
  const matrix = {
    generatedAt: new Date().toISOString(),
    summary: {
      manifestItems: manifest.items.length,
      pages: pages.length,
      overlays: overlays.length,
      components: manifest.items.filter((item) => item.type === 'component').length,
      states: manifest.items.filter((item) => item.type === 'state').length,
      responsive: manifest.items.filter((item) => item.type === 'responsive').length,
      methods: IMPLEMENTATION_METHODS.length,
    },
    batches: batchRows(pages, overlays, manifest.items),
    pages,
    overlays,
    componentBacklog: manifest.items.filter((item) => item.type === 'component').map((item) => item.id),
    stateBacklog: manifest.items.filter((item) => item.type === 'state').map((item) => item.id),
    responsiveBacklog: manifest.items.filter((item) => item.type === 'responsive').map((item) => item.id),
    implementationMethods: IMPLEMENTATION_METHODS,
  };
  const flows = flowRows(FLOW_DEFINITIONS, pages, overlays);

  writeJson(path.join(SPEC_DIR, 'frontend-task-matrix.json'), matrix);
  writeJson(path.join(SPEC_DIR, 'business-flow-acceptance.json'), flows);
  writeMd(path.join(SPEC_DIR, 'FRONTEND_TASK_MATRIX.md'), matrixMd(matrix));
  writeMd(path.join(SPEC_DIR, 'BUSINESS_FLOW_ACCEPTANCE.md'), flowMd(flows));
  writeMd(path.join(SPEC_DIR, 'FRONTEND_DEV_CHECKLIST.md'), checklistMd());
  writeMd(path.join(SPEC_DIR, 'FRONTEND_IMPLEMENTATION_METHODS.md'), methodsMd(IMPLEMENTATION_METHODS));

  console.log(`frontend handoff generated: ${path.relative(ROOT, SPEC_DIR)}`);
  console.log(`batches: ${matrix.batches.length}`);
  console.log(`pages: ${matrix.pages.length}`);
  console.log(`overlays: ${matrix.overlays.length}`);
  console.log(`business flows: ${flows.length}`);
}

main();
