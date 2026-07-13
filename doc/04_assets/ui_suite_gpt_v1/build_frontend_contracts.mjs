import fs from 'node:fs';
import path from 'node:path';

const ROOT = process.cwd();
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');
const TOKENS_CSS = path.join(ROOT, 'web/ui/src/styles/tokens.css');
let manifest;

const TARGET_CANVAS = { width: 1920, height: 1080 };
const TARGET_APP_SHELL = {
  topbar: { x: 0, y: 0, w: 1920, h: 80 },
  sidebar: { x: 0, y: 80, w: 166, h: 917 },
  content: { x: 198, y: 80, w: 1722, h: 917 },
  bottombar: { x: 0, y: 997, w: 1920, h: 83 },
};

const DOMAIN_BY_ROUTE = {
  '/dashboard': 'overview',
  '/screen': 'overview',
  '/topics': 'overview',
  '/probes': 'collection-monitoring',
  '/data-quality': 'collection-monitoring',
  '/alerts': 'threat-analysis',
  '/alerts/:alertId': 'threat-analysis',
  '/campaigns': 'threat-analysis',
  '/campaigns/:campaignId': 'threat-analysis',
  '/attack-chains': 'threat-analysis',
  '/encrypted-traffic': 'threat-analysis',
  '/forensics': 'threat-analysis',
  '/assets': 'asset-graph',
  '/graph': 'asset-graph',
  '/fusion': 'asset-graph',
  '/baselines': 'asset-graph',
  '/rules': 'detection-ops',
  '/deployments': 'detection-ops',
  '/models': 'detection-ops',
  '/mlops': 'detection-ops',
  '/playbooks': 'detection-ops',
  '/whitelist': 'detection-ops',
  '/compliance': 'audit-config',
  '/audit-log': 'audit-config',
  '/notifications': 'audit-config',
  '/settings': 'audit-config',
  '/login': 'auth',
  '*': 'global',
};

const PAGE_COMPONENT_BY_ID = {
  login: 'LoginPage',
  screen: 'SituationalScreen',
  dashboard: 'DashboardOperationsPage',
  topics: 'TopicWorkbenchPage',
  probes: 'ProbesManagementPage',
  'data-quality': 'DataQualityPage',
  alerts: 'AlertTriagePage',
  'alert-detail': 'AlertDetailPage',
  campaigns: 'CampaignWorkbenchPage',
  'campaign-detail': 'CampaignDetailPage',
  'attack-chains': 'AttackChainAnalysisPage',
  'encrypted-traffic': 'EncryptedTrafficPage',
  forensics: 'ForensicsWorkbenchPage',
  assets: 'AssetInventoryPage',
  graph: 'GraphEntityPage',
  fusion: 'FusionWorkbenchPage',
  baselines: 'BaselineWorkbenchPage',
  rules: 'RuleManagementPage',
  deployments: 'DeploymentManagementPage',
  models: 'ModelManagementPage',
  mlops: 'MlopsOrchestrationPage',
  playbooks: 'PlaybookAutomationPage',
  whitelist: 'WhitelistGovernancePage',
  compliance: 'ComplianceAuditPage',
  'audit-log': 'AuditLogPage',
  notifications: 'NotificationConfigPage',
  settings: 'SettingsGovernancePage',
  'not-found': 'NotFoundPage',
};

const PAGE_API = {
  dashboard: ['/api/v1/dashboard/stats', '/api/v1/dashboard/alerts/trend', '/api/v1/dashboard/attack-phases'],
  screen: ['/api/v1/dashboard/stats', '/api/v1/dashboard/encrypted/trend', '/api/v1/dashboard/attack-phases'],
  topics: ['/api/v1/topics/tunnel', '/api/v1/topics/exfil', '/api/v1/topics/apt'],
  probes: ['/api/v1/probes'],
  'data-quality': ['/api/v1/data-quality'],
  alerts: ['/api/v1/alerts'],
  'alert-detail': ['/api/v1/alerts/{id}', '/api/v1/alerts/{id}/evidence', '/api/v1/alerts/{id}/feedback'],
  campaigns: ['/api/v1/campaigns'],
  'campaign-detail': ['/api/v1/campaigns/{id}'],
  'attack-chains': ['/api/v1/attack-chains'],
  'encrypted-traffic': [
    '/api/v1/encrypted-traffic/stats',
    '/api/v1/encrypted-traffic/sessions',
    '/api/v1/encrypted-traffic/ja3',
    '/api/v1/encrypted-traffic/tunnels',
    '/api/v1/encrypted-traffic/exfiltration',
  ],
  forensics: ['/api/v1/pcap/jobs', '/api/v1/pcap/stats'],
  assets: ['/api/v1/assets'],
  graph: ['/api/v1/graph/explore'],
  fusion: ['/api/v1/fusion/stats', '/api/v1/fusion/entities'],
  baselines: ['/api/v1/baselines'],
  rules: ['/api/v1/rules'],
  deployments: ['/api/v1/deployments'],
  models: ['/api/v1/models'],
  mlops: ['/api/v1/mlops/status', '/api/v1/mlops/conditions'],
  playbooks: ['/api/v1/playbooks/catalog', '/api/v1/playbooks/executions'],
  whitelist: ['/api/v1/whitelist'],
  compliance: ['/api/v1/compliance/reports', '/api/v1/compliance/audit-trail'],
  'audit-log': ['/api/v1/audit/logs'],
  notifications: ['/api/v1/notifications/settings'],
  settings: ['/api/v1/tokens/scopes', '/api/v1/tokens', '/api/v1/tokens/scopes/probe'],
};

const PAGE_WORKFLOWS = {
  dashboard: ['脱敏运营 KPI', '优先级待办队列', '采集与数据健康门禁', '证据与反馈质量摘要', '验收缺口看板'],
  screen: ['园区拓扑总览', '采集流处理管道', '威胁态势', '证据取证', '响应与反馈', '运行底座'],
  topics: ['专题范围定义', '专题专属 KPI', '局部影响面', '专题分析', '关联事件与证据', '专题报告与交付'],
  probes: ['探针总览', '部署拓扑', '吞吐与丢包', '探针配置', '心跳与日志', '批量运维'],
  'data-quality': ['质量总分', 'Topic 健康', 'Flink 处理质量', '字段质量', '存储质量', '重放与对账'],
  alerts: ['告警队列', '筛选检索', '选中告警摘要', '研判时间线', '关联告警簇', '处置与反馈'],
  'alert-detail': ['告警摘要', '状态机', '证据链', '处置反馈', '上下文跳转'],
  campaigns: ['战役总览', '战役列表', '战役时间线', '影响范围', '证据汇总'],
  'campaign-detail': ['战役画像', '攻击阶段', '影响范围', '证据链', '报告导出'],
  'attack-chains': ['攻击链画布', '阶段识别', '路径分析', '证据锚点', '处置建议'],
  'encrypted-traffic': ['加密流量总览', '指纹分析', '隧道检测', '外联画像', '证据提取'],
  forensics: ['取证任务', 'PCAP 索引', '会话复放', '证据完整性', '证据导出', '跨页上下文'],
  assets: ['资产列表', '风险画像', '资产详情', '流量画像', '证据关联'],
  graph: ['实体关系图', '路径分析', '邻居扩展', '风险边解释', '证据跳转'],
  fusion: ['融合总览', '冲突队列', '来源可信度', '回写审计', '冲突处理'],
  baselines: ['资产基线', '账号基线', '端口基线', '协议基线', '时间段基线'],
  rules: ['规则定义', '测试验证', '依赖引用', '样本回放', '发布门禁'],
  deployments: ['发布计划', '灰度状态', '回滚窗口', '运行健康', '审计记录'],
  models: ['模型版本', '特征解释', '异常解释', '样本示例', '激活回滚'],
  mlops: ['任务 DAG', '标注回流', '训练评估', '模型注册', '在线效果'],
  playbooks: ['剧本列表', '剧本编排', '触发策略', '执行历史', '风险控制', '审计证据'],
  whitelist: ['条件构造', '审批流程', '到期治理', '命中监控', '风险解释'],
  compliance: ['验收门禁', '指标映射', '证据包', '运行报告', '缺口治理', '第三方评测'],
  'audit-log': ['日志检索', '操作详情', '高风险审计', '关联链路', '留存状态', '导出取证'],
  notifications: ['通知渠道', '订阅规则', '升级策略', '模板管理', '发送历史', '抑制静默'],
  settings: ['租户站点', '权限矩阵', 'API 令牌', '留存策略', '集成配置', '安全策略', '系统参数'],
};

const TOKENS = {
  canvas: TARGET_CANVAS,
  appShell: TARGET_APP_SHELL,
  colors: {
    pageBg: '#03111c',
    shellBg: '#061827',
    panelBg: 'rgba(6, 28, 43, 0.88)',
    panelStrong: '#071f32',
    borderSubtle: 'rgba(56, 151, 201, 0.24)',
    active: '#1e9cff',
    info: '#18a8ff',
    success: '#36d66b',
    warning: '#ffb020',
    danger: '#ff4d4f',
    critical: '#ff2d2d',
    textPrimary: '#eaf7ff',
    textSecondary: '#9db9c9',
    textMuted: '#5e7b8d',
  },
  typography: {
    fontFamily: '"Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", "Segoe UI", sans-serif',
    pageTitle: 20,
    panelTitle: 14,
    tableText: 12,
    metricValue: 22,
    helperText: 11,
    letterSpacing: 0,
  },
  density: {
    panelRadius: 6,
    buttonRadius: 4,
    tableRowHeight: 32,
    panelGap: 8,
    iconButton: 32,
    compactControlHeight: 28,
  },
};

const COMPONENT_KIND = {
  modal: { component: 'Modal', antD: 'Modal', defaultBox: { x: 360, y: 170, w: 1200, h: 740 } },
  drawer: { component: 'Drawer', antD: 'Drawer', defaultBox: { x: 980, y: 48, w: 900, h: 984 } },
  dropdown: { component: 'Dropdown', antD: 'Dropdown/Menu', defaultBox: { x: 1280, y: 96, w: 480, h: 720 } },
  popconfirm: { component: 'Popconfirm', antD: 'Popconfirm', defaultBox: { x: 640, y: 330, w: 640, h: 400 } },
};

const COMPONENT_MAP = {
  appShell: {
    files: ['web/ui/src/layouts/AppShell.tsx', 'web/ui/src/styles/app-shell.css', 'web/ui/src/styles/tokens.css'],
    sourceImages: [
      'screens/pages/screen.png',
      'screens/components/component-app-header.png',
      'screens/components/component-primary-sidebar.png',
      'screens/components/component-secondary-menu.png',
      'screens/components/component-bottom-status-bar.png',
    ],
    required: ['topbar', 'sidebar', 'bottombar', 'quickEntries', 'userMenu'],
  },
  primitives: {
    Button: ['component-button', 'component-icon-button'],
    Tag: ['component-status-chip'],
    Tooltip: ['component-tooltip'],
    Tabs: ['component-tabs'],
    Segmented: ['component-segmented'],
    Dropdown: ['component-dropdown'],
    Pagination: ['component-pagination'],
    Input: ['component-input', 'component-search'],
    Select: ['component-select'],
    DatePicker: ['component-date-range'],
    Switch: ['component-switch-checkbox-radio'],
    Table: ['component-data-table'],
    Form: ['component-condition-builder'],
  },
  composites: {
    MetricTile: ['component-kpi-tile', 'web/ui/src/components/MetricTile.tsx'],
    WorkPanel: ['component-empty-card', 'component-permission-card', 'web/ui/src/components/WorkPanel.tsx'],
    StatusTag: ['component-status-chip', 'web/ui/src/components/StatusTag.tsx'],
    Charts: [
      'component-line-area-chart',
      'component-donut-chart',
      'component-bar-ranking-chart',
      'component-sankey-flow',
      'component-radar-quality',
      'component-heatmap',
      'component-topology-graph',
      'component-timeline-state-machine',
      'web/ui/src/components/charts.tsx',
    ],
  },
};

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function writeJson(file, value) {
  ensureDir(path.dirname(file));
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function writeMd(file, content) {
  ensureDir(path.dirname(file));
  fs.writeFileSync(file, content.trimEnd() + '\n');
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function pngDimensions(file) {
  if (!fs.existsSync(file)) return null;
  const buf = fs.readFileSync(file);
  const isPng = buf.length > 24 && buf.toString('hex', 0, 8) === '89504e470d0a1a0a';
  if (!isPng) return null;
  return { width: buf.readUInt32BE(16), height: buf.readUInt32BE(20) };
}

function rawTraceFiles(targetFile) {
  const dir = path.dirname(targetFile);
  const base = path.basename(targetFile, '.png');
  return ['raw-imagegen', 'raw-deterministic']
    .map((kind) => path.join(dir, `${base}.${kind}.png`))
    .filter((file) => fs.existsSync(file))
    .map(rel);
}

function overlayKind(id) {
  if (id.startsWith('drawer-')) return 'drawer';
  if (id.startsWith('dropdown-')) return 'dropdown';
  if (id.startsWith('popconfirm-')) return 'popconfirm';
  return 'modal';
}

function componentSuggestions(item) {
  if (item.type === 'overlay') {
    const kind = overlayKind(item.id);
    return [COMPONENT_KIND[kind].antD, 'Button', 'Form', 'Alert', 'Tag'];
  }
  if (item.type === 'page') return ['AppShell', 'WorkPanel', 'MetricTile', 'Table', 'Tabs', 'ECharts', 'StatusTag'];
  if (item.type === 'component') {
    const id = item.id;
    if (id.includes('table')) return ['Table', 'Pagination', 'Dropdown', 'Tag'];
    if (id.includes('chart') || id.includes('heatmap') || id.includes('sankey') || id.includes('topology')) return ['ECharts'];
    if (id.includes('input') || id.includes('select') || id.includes('date') || id.includes('condition')) return ['Form'];
    return ['Ant Design', 'CSS token', 'React component'];
  }
  if (item.type === 'state') return ['Result', 'Skeleton', 'Empty', 'Alert', 'Button'];
  if (item.type === 'responsive') return ['CSS media query', 'AppShell breakpoint', 'Drawer navigation'];
  return ['CSS token'];
}

function layersFor(item) {
  if (item.type === 'page') {
    if (item.id === 'login') {
      return [
        { id: 'login-background', role: 'auth-background', bbox: { x: 0, y: 0, w: 1920, h: 1080 } },
        { id: 'login-panel', role: 'auth-form', bbox: { x: 1160, y: 210, w: 520, h: 620 } },
      ];
    }
    return [
      { id: 'topbar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.topbar },
      { id: 'sidebar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.sidebar },
      { id: 'content', role: 'page-workspace', bbox: TARGET_APP_SHELL.content },
      { id: 'bottombar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.bottombar },
      { id: 'right-rail', role: 'closed-loop-rail', bbox: { x: 1460, y: 104, w: 420, h: 860 } },
    ];
  }
  if (item.type === 'overlay') {
    const kind = overlayKind(item.id);
    return [
      { id: `${kind}-surface`, role: 'interaction-container', bbox: COMPONENT_KIND[kind].defaultBox },
      { id: 'action-bar', role: 'cancel-confirm-actions', bbox: { x: 1240, y: 950, w: 560, h: 52 } },
      { id: 'audit-strip', role: 'audit-and-risk-hint', bbox: { x: COMPONENT_KIND[kind].defaultBox.x, y: 950, w: 760, h: 52 } },
    ];
  }
  if (item.type === 'component') {
    return [
      { id: 'component-board', role: 'component-specimen', bbox: { x: 80, y: 80, w: 1760, h: 920 } },
      { id: 'states-matrix', role: 'normal-hover-active-disabled-error-loading', bbox: { x: 1120, y: 140, w: 660, h: 760 } },
    ];
  }
  if (item.type === 'state') {
    return [
      { id: 'state-canvas', role: 'reusable-state-pattern', bbox: { x: 280, y: 170, w: 1360, h: 740 } },
      { id: 'primary-action', role: 'state-action', bbox: { x: 980, y: 780, w: 240, h: 44 } },
    ];
  }
  if (item.type === 'responsive') {
    return [
      { id: 'breakpoint-frame', role: 'responsive-reference', bbox: { x: 80, y: 80, w: 1760, h: 920 } },
      { id: 'folding-rules', role: 'module-priority-and-collapse', bbox: { x: 1160, y: 140, w: 620, h: 760 } },
    ];
  }
  return [{ id: 'foundation-board', role: 'design-system-foundation', bbox: { x: 80, y: 80, w: 1760, h: 920 } }];
}

function acceptanceFor(item) {
  const common = [
    '最终 PNG 必须为 1920x1080',
    '中文为主，只保留必要英文技术词和单位',
    '状态色必须遵守 success/info/warning/danger/critical token',
    '危险动作必须具备影响范围、权限提示和审计留痕',
  ];
  if (item.type === 'page') {
    return [
      ...common,
      item.id === 'login' ? '登录页不展示常规 AppShell' : '公共 AppShell 必须与 screen.png 目标参数一致',
      '页面主工作区不得复用相邻页面的业务组件组合',
      '所有 API 调用必须经 services/api.ts 或现有服务封装',
      'React Query 必须覆盖 loading/error/empty 状态',
    ];
  }
  if (item.type === 'overlay') {
    const kind = overlayKind(item.id);
    return [
      ...common,
      `必须实现为 ${COMPONENT_KIND[kind].antD} 或等价语义组件`,
      '浮层只承载当前交互容器本体，不恢复完整宿主 AppShell',
      '确认类动作必须出现取消/确认，危险确认默认不可误触',
    ];
  }
  if (item.type === 'component') return [...common, '必须展示 normal/hover/active/disabled/error/loading 等状态矩阵'];
  if (item.type === 'state') return [...common, '401 与 403 必须分离，不能共用重试主动作'];
  if (item.type === 'responsive') return [...common, '必须说明核心业务保留、次要区域折叠、危险动作位置和上下文传递'];
  return [...common, 'foundation 只能作为 token 和设计系统来源，不作为业务页面直接实现'];
}

function itemSpec(item) {
  const targetAbs = path.join(ROOT, item.targetFile);
  const dims = pngDimensions(targetAbs);
  const pageId = item.type === 'page' ? item.id : routeToPageId(item.route);
  return {
    id: item.id,
    title: item.title,
    type: item.type,
    route: item.route ?? null,
    domain: DOMAIN_BY_ROUTE[item.route] ?? DOMAIN_BY_ROUTE[pageIdToRoute(pageId)] ?? 'global',
    source: {
      targetFile: item.targetFile,
      promptFile: item.promptFile,
      rawTraceFiles: rawTraceFiles(targetAbs),
      dimensions: dims,
    },
    implementation: {
      pageComponent: PAGE_COMPONENT_BY_ID[pageId] ?? null,
      suggestedComponents: componentSuggestions(item),
      apiEndpoints: PAGE_API[pageId] ?? [],
      workflows: PAGE_WORKFLOWS[pageId] ?? [],
    },
    layers: layersFor(item),
    acceptance: acceptanceFor(item),
  };
}

function routeToPageId(route) {
  if (!route) return null;
  if (route === '*') return 'not-found';
  if (route === '/alerts/:alertId') return 'alert-detail';
  if (route === '/campaigns/:campaignId') return 'campaign-detail';
  return route.replace(/^\//, '') || 'dashboard';
}

function pageIdToRoute(id) {
  const item = manifest.items.find((entry) => entry.type === 'page' && entry.id === id);
  return item?.route ?? null;
}

function currentCssShellTokens() {
  if (!fs.existsSync(TOKENS_CSS)) return {};
  const css = fs.readFileSync(TOKENS_CSS, 'utf8');
  const pick = (name) => {
    const match = css.match(new RegExp(`${name}:\\s*([^;]+);`));
    return match ? match[1].trim() : null;
  };
  return {
    shellTopbar: pick('--shell-topbar'),
    shellSidebar: pick('--shell-sidebar'),
    shellBottombar: pick('--shell-bottombar'),
    panelRadius: pick('--panel-radius'),
  };
}

function pageContractMd(spec, relatedOverlays) {
  const imageLine = spec.source.targetFile ? `- 目标图：\`${spec.source.targetFile}\`` : '- 目标图：无单张页面主图，使用专题状态输入图。';
  return `# ${spec.title} 前端实现契约

## 基本信息

- ID：\`${spec.id}\`
- 路由：\`${spec.route ?? pageIdToRoute(spec.id) ?? '/topics'}\`
- 领域：\`${spec.domain}\`
- React 页面：\`${spec.implementation.pageComponent ?? '待映射'}\`
${imageLine}
- API：${spec.implementation.apiEndpoints.length ? spec.implementation.apiEndpoints.map((api) => `\`${api}\``).join('、') : '按页面服务计划补齐'}

## 必须实现的业务层

${(spec.implementation.workflows.length ? spec.implementation.workflows : ['页面主工作区', '右侧闭环栏', '审计与证据入口']).map((item) => `- ${item}`).join('\n')}

## 分层参数

${spec.layers.map((layer) => `- \`${layer.id}\`：${layer.role}，bbox=\`${JSON.stringify(layer.bbox)}\``).join('\n')}

## 组件映射

${spec.implementation.suggestedComponents.map((item) => `- ${item}`).join('\n')}

## 关联浮层

${relatedOverlays.length ? relatedOverlays.map((overlay) => `- \`${overlay.id}\`：${overlay.title}，${overlay.implementation.suggestedComponents[0]}`).join('\n') : '- 暂无专属浮层，按页面内交互实现。'}

## 验收清单

${spec.acceptance.map((item) => `- [ ] ${item}`).join('\n')}
`;
}

function overlayContractMd(spec) {
  return `# ${spec.title} 浮层实现契约

## 基本信息

- ID：\`${spec.id}\`
- 宿主路由：\`${spec.route ?? 'global'}\`
- 推荐组件：\`${spec.implementation.suggestedComponents[0]}\`
- 目标图：\`${spec.source.targetFile}\`
- Prompt：\`${spec.source.promptFile}\`

## 分层参数

${spec.layers.map((layer) => `- \`${layer.id}\`：${layer.role}，bbox=\`${JSON.stringify(layer.bbox)}\``).join('\n')}

## 数据与动作

- API 继承：${spec.implementation.apiEndpoints.length ? spec.implementation.apiEndpoints.map((api) => `\`${api}\``).join('、') : '按宿主页面服务计划补齐'}
- 必须包含：权限提示、影响范围、审计 trace、取消/确认动作。
- 危险动作：默认要求二次确认，确认按钮在必填条件未满足时禁用。

## 验收清单

${spec.acceptance.map((item) => `- [ ] ${item}`).join('\n')}
`;
}

function implementationPlaybook() {
  return `# UI Suite 前端实现手册

本目录把 manifest 中的 181 个 UI 契约项转换为前端实现契约。前端开发必须以这里的 JSON/Markdown 为入口，而不是只凭 PNG 目测实现。

## 执行顺序

1. 先实现 \`tokens.json\` 和 \`app-shell.json\`：统一顶部栏、左侧单栏、底部栏、颜色、字号、密度。
2. 再实现 \`component-map.json\`：把 48 张 component 图映射到 Ant Design、ECharts 和现有 React 组件。
3. 按 \`route-page-map.json\` 逐页实现 27 张页面主图和 \`/topics\` 合并页。
4. 按 \`overlay-contracts/\` 实现 70 张 Modal/Drawer/Dropdown/Popconfirm。
5. 按 \`visual-acceptance.json\` 做 Playwright 截图回归和业务状态验收。
6. 每轮开发前后运行 \`validate_frontend_contracts.mjs\`，确保契约、图、prompt、trace、路由映射未断链。
7. 运行 \`build_frontend_handoff.mjs\` 生成任务矩阵、业务链路验收和开发 checklist，用于派工和 PR 验收。
8. 运行 \`build_frontend_code_gap.mjs\` 对比当前 \`web/ui\` 与契约，生成代码缺口报告和修复队列。

## 前端代码落点

- 路由：\`web/ui/src/routes/routeManifest.tsx\`
- AppShell：\`web/ui/src/layouts/AppShell.tsx\`
- 全局样式：\`web/ui/src/styles/tokens.css\`、\`web/ui/src/styles/app-shell.css\`
- 页面：\`web/ui/src/pages/*.tsx\`
- 组件：\`web/ui/src/components/*.tsx\`
- API：\`web/ui/src/services/*.ts\`

## 不可偏离项

- 不允许恢复双栏左侧导航。
- 不允许把通知、用户、设置、电源放到顶部栏。
- 不允许页面组件直接 \`fetch\`；必须走 \`services/api.ts\` 或既有 service 封装。
- 不允许忽略 loading/error/empty/401/403。
- 不允许危险动作缺少影响范围、权限提示和审计留痕。

## 每页开发完成定义

- 路由可访问，权限与 \`requiredScopes\` 一致。
- 页面 AppShell 与 \`app-shell.json\` 对齐。
- 主工作区实现 \`page-contracts/<id>.md\` 的业务层。
- API endpoint 与 \`route-page-map.json\` 对齐。
- 关联浮层全部可触发。
- Playwright 截图与目标图做视觉 diff，公共区必须严格一致。

## 契约自检

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
\`\`\`

## 继续梳理交付物

- \`FRONTEND_TASK_MATRIX.md\`：批次、页面、路由、API、浮层依赖。
- \`BUSINESS_FLOW_ACCEPTANCE.md\`：从登录到业务闭环的验收链路。
- \`FRONTEND_DEV_CHECKLIST.md\`：前端开发前、中、提交前 checklist。
- \`FRONTEND_IMPLEMENTATION_METHODS.md\`：分层、组件、路由/API、截图回归、人工标注 5 种方法。
- \`FRONTEND_CODE_GAP.md\`：当前前端代码与 UI 契约的静态差距。
- \`FRONTEND_FIX_QUEUE.md\`：按优先级排序的前端修复队列。
`;
}

function frontendDeltaMd(cssTokens) {
  const target = {
    shellTopbar: `${TARGET_APP_SHELL.topbar.h}px`,
    shellSidebar: `${TARGET_APP_SHELL.sidebar.w}px`,
    shellBottombar: `${TARGET_APP_SHELL.bottombar.h}px`,
  };
  const rows = Object.entries(target)
    .map(([key, value]) => {
      const current = cssTokens[key] ?? '未读取';
      const status = current === value ? '一致' : '需对齐';
      return `| ${key} | \`${value}\` | \`${current}\` | ${status} |`;
    })
    .join('\n');
  return `# 前端实现差异清单

本文件只指出 UI 图契约与当前前端 token 的可见差异，不自动修改前端代码。

| 参数 | UI 图目标 | 当前代码 | 状态 |
|---|---:|---:|---|
${rows}

## 处理原则

- 若实现目标是复刻 UI 图，以上 \`需对齐\` 项必须优先修正。
- 修正后运行 \`npm run build\` 和 Playwright 截图回归。
- 如产品确认以前端现状为准，必须反向更新 UI suite 的 AppShell 目标参数和所有验收文档。
`;
}

function routeMap(specs) {
  const pageSpecs = specs.filter((item) => item.type === 'page');
  const topicsContract = {
    id: 'topics',
    title: '专题面板',
    type: 'page',
    route: '/topics',
    domain: 'overview',
    source: {
      targetFile: null,
      stateImages: [
        'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-encrypted-tunnel.png',
        'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-data-exfiltration.png',
        'doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-apt-campaign.png',
      ].filter((file) => fs.existsSync(path.join(ROOT, file))),
    },
    implementation: {
      pageComponent: 'TopicWorkbenchPage',
      apiEndpoints: PAGE_API.topics,
      workflows: PAGE_WORKFLOWS.topics,
      suggestedComponents: ['AppShell', 'Segmented', 'Tabs', 'MetricTile', 'Table', 'Graph', 'Modal', 'Drawer'],
    },
    layers: [
      { id: 'topbar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.topbar },
      { id: 'sidebar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.sidebar },
      { id: 'topic-content', role: 'topic-workspace', bbox: TARGET_APP_SHELL.content },
      { id: 'bottombar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.bottombar },
    ],
    acceptance: acceptanceFor({ type: 'page', id: 'topics' }),
  };
  return [...pageSpecs, topicsContract].map((spec) => ({
    id: spec.id,
    title: spec.title,
    route: spec.route,
    domain: spec.domain,
    pageComponent: spec.implementation.pageComponent,
    apiEndpoints: spec.implementation.apiEndpoints,
    sourceImage: spec.source.targetFile ?? spec.source.stateImages?.[0] ?? null,
    sourceImages: spec.source.stateImages ?? (spec.source.targetFile ? [spec.source.targetFile] : []),
    contract: `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/${spec.id}.md`,
  }));
}

function visualAcceptance(specs) {
  return {
    global: {
      imageSize: TARGET_CANVAS,
      maxAllowedMissingTargets: 0,
      maxAllowedMissingPrompts: 0,
      requiredRawTrace: true,
      appShellPixelStrictPages: specs.filter((item) => item.type === 'page' && !['login', 'screen'].includes(item.id)).map((item) => item.id),
    },
    playwright: {
      viewports: [
        { name: 'desktop-1920', width: 1920, height: 1080 },
        { name: 'desktop-1440', width: 1440, height: 900 },
        { name: 'tablet', width: 1024, height: 768 },
        { name: 'mobile', width: 390, height: 844 },
      ],
      failOn: ['4xx/5xx API response', 'requestfailed', 'pageerror', 'console error except known warnings', 'missing loading/error/empty state'],
    },
    routes: routeMap(specs),
  };
}

function main() {
  ensureDir(SPEC_DIR);
  ensureDir(path.join(SPEC_DIR, 'layers'));
  ensureDir(path.join(SPEC_DIR, 'page-contracts'));
  ensureDir(path.join(SPEC_DIR, 'overlay-contracts'));

  manifest = readJson(MANIFEST_PATH);
  const specs = manifest.items.map(itemSpec);
  const byRoute = new Map();
  for (const spec of specs) {
    if (spec.route) {
      if (!byRoute.has(spec.route)) byRoute.set(spec.route, []);
      byRoute.get(spec.route).push(spec);
    }
  }

  for (const spec of specs) writeJson(path.join(SPEC_DIR, 'layers', `${spec.id}.json`), spec);

  const pageSpecs = specs.filter((item) => item.type === 'page');
  const overlaySpecs = specs.filter((item) => item.type === 'overlay');
  for (const page of pageSpecs) {
    const related = overlaySpecs.filter((overlay) => overlay.route === page.route);
    writeMd(path.join(SPEC_DIR, 'page-contracts', `${page.id}.md`), pageContractMd(page, related));
  }

  const topicRelated = overlaySpecs.filter((overlay) => overlay.route === '/topics');
  writeMd(
    path.join(SPEC_DIR, 'page-contracts', 'topics.md'),
    pageContractMd(
      {
        id: 'topics',
        title: '专题面板',
        type: 'page',
        route: '/topics',
        domain: 'overview',
        source: { targetFile: null },
        implementation: {
          pageComponent: 'TopicWorkbenchPage',
          apiEndpoints: PAGE_API.topics,
          workflows: PAGE_WORKFLOWS.topics,
          suggestedComponents: ['AppShell', 'Segmented', 'Tabs', 'MetricTile', 'Table', 'Graph', 'Modal', 'Drawer'],
        },
        layers: [
          { id: 'topbar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.topbar },
          { id: 'sidebar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.sidebar },
          { id: 'topic-content', role: 'topic-workspace', bbox: TARGET_APP_SHELL.content },
          { id: 'bottombar', role: 'global-app-shell', bbox: TARGET_APP_SHELL.bottombar },
        ],
        acceptance: acceptanceFor({ type: 'page', id: 'topics' }),
      },
      topicRelated,
    ),
  );

  for (const overlay of overlaySpecs) {
    writeMd(path.join(SPEC_DIR, 'overlay-contracts', `${overlay.id}.md`), overlayContractMd(overlay));
  }

  const cssTokens = currentCssShellTokens();
  writeJson(path.join(SPEC_DIR, 'index.json'), {
    generatedAt: new Date().toISOString(),
    sourceManifest: rel(MANIFEST_PATH),
    total: manifest.total,
    counts: manifest.counts,
    contractFiles: {
      tokens: 'tokens.json',
      appShell: 'app-shell.json',
      componentMap: 'component-map.json',
      routePageMap: 'route-page-map.json',
      visualAcceptance: 'visual-acceptance.json',
      frontendDelta: 'frontend-delta.md',
      implementationPlaybook: 'IMPLEMENTATION_PLAYBOOK.md',
      frontendTaskMatrix: 'FRONTEND_TASK_MATRIX.md',
      businessFlowAcceptance: 'BUSINESS_FLOW_ACCEPTANCE.md',
      frontendDevChecklist: 'FRONTEND_DEV_CHECKLIST.md',
      frontendImplementationMethods: 'FRONTEND_IMPLEMENTATION_METHODS.md',
      frontendCodeGap: 'FRONTEND_CODE_GAP.md',
      frontendFixQueue: 'FRONTEND_FIX_QUEUE.md',
      imageBreakdowns: 'image-breakdowns/',
    },
  });
  writeJson(path.join(SPEC_DIR, 'tokens.json'), TOKENS);
  writeJson(path.join(SPEC_DIR, 'app-shell.json'), {
    target: TARGET_APP_SHELL,
    currentCodeTokens: cssTokens,
    rules: [
      '顶部栏不得承载通知铃铛、用户头像、用户菜单、设置或电源动作组',
      '左侧必须为单栏展开式导航，禁止窄一级栏 + 独立二级栏',
      '底部右侧承载通知、设置、全局配置、电源',
      '除 login 和明确独立浮层外，公共区按 screen.png 目标参数验收',
    ],
  });
  writeJson(path.join(SPEC_DIR, 'component-map.json'), COMPONENT_MAP);
  writeJson(path.join(SPEC_DIR, 'route-page-map.json'), routeMap(specs));
  writeJson(path.join(SPEC_DIR, 'visual-acceptance.json'), visualAcceptance(specs));
  writeMd(path.join(SPEC_DIR, 'frontend-delta.md'), frontendDeltaMd(cssTokens));
  writeMd(path.join(SPEC_DIR, 'IMPLEMENTATION_PLAYBOOK.md'), implementationPlaybook());
  writeMd(
    path.join(SPEC_DIR, 'README.md'),
    `# UI Suite 前端实现契约包

本目录由 \`build_frontend_contracts.mjs\` 从 \`manifest.json\`、现有 UI 图和前端代码约束生成，用于指导 React + Ant Design + ECharts 前端准确实现 manifest 中的 181 个 UI 契约项，并配合 \`image-breakdowns/\` 覆盖 \`screens/\` 下全部 canonical PNG。

逐图拆解要求：\`screens/\` 下每一张页面、浮层、组件、状态或响应式图片都必须有独立拆解记录。拆解记录只能逐张新增或更新，禁止一次性批量生成全量清单。

## 入口文件

- \`IMPLEMENTATION_PLAYBOOK.md\`：前端执行顺序和完成定义。
- \`tokens.json\`：颜色、字号、密度、AppShell 坐标参数。
- \`app-shell.json\`：公共顶部栏、左侧菜单和底部栏契约。
- \`route-page-map.json\`：路由、页面组件、API、目标图和契约文件映射。
- \`component-map.json\`：48 张组件板到前端组件/Ant Design/ECharts 的映射。
- \`visual-acceptance.json\`：Playwright 与视觉回归验收规则。
- \`frontend-delta.md\`：当前前端 token 与 UI 图目标参数差异。
- \`FRONTEND_TASK_MATRIX.md\`：前端批次、页面、API、浮层派工矩阵。
- \`BUSINESS_FLOW_ACCEPTANCE.md\`：关键业务闭环验收链路。
- \`FRONTEND_DEV_CHECKLIST.md\`：逐页开发与提交前检查清单。
- \`FRONTEND_IMPLEMENTATION_METHODS.md\`：准确实现 UI 图的 5 种方法。
- \`FRONTEND_CODE_GAP.md\`：当前前端代码与 UI 契约的静态差距。
- \`FRONTEND_FIX_QUEUE.md\`：按优先级排序的前端修复队列。
- \`PIXEL_PERFECT_IMAGE_BREAKDOWN_PLAN.md\`：逐张精拆、Windows Chrome 截图、视觉 diff 和差异清零的 100% 复刻验收方案。
- \`image-breakdowns/\`：逐图拆解记录；每个 PNG 对应一个 Markdown 和可选 JSON，不允许用批量汇总替代。
- \`page-contracts/\`：逐页面开发契约。
- \`overlay-contracts/\`：逐浮层开发契约。
- \`layers/\`：181 个 manifest 契约项的机器可读分层 JSON。

## 重新生成

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/build_frontend_contracts.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_handoff.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs
\`\`\`

## 逐图拆解

先生成待办索引；该索引只用于排队和统计，不是拆解记录：

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/build_pixel_breakdown_queue.mjs
\`\`\`

每次只允许启动一张图片的精拆记录：

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/start_pixel_image_breakdown.mjs --image doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png
\`\`\`

产物固定为：

- \`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.md\`
- \`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.json\`
- \`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.review.md\`
- \`evidence/ui-image-breakdowns/<分类>/<图片ID>/\`

## 全流程门禁

拆解记录门禁只判断每张图是否有完整的视觉拆解记录，不判断实现截图和 pixel diff：

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/validate_image_breakdown_records.mjs
\`\`\`

状态文件：

- \`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdown-record-status.json\`
- \`doc/04_assets/ui_suite_gpt_v1/specs/IMAGE_BREAKDOWN_RECORD_STATUS.md\`

逐图记录不能停在 \`review-ready\`。每张图都必须继续推进到实现、Windows Chrome 截图、视觉 diff 和最终状态：

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/validate_pixel_breakdown_pipeline.mjs
\`\`\`

状态文件：

- \`doc/04_assets/ui_suite_gpt_v1/specs/pixel-perfect-pipeline-status.json\`
- \`doc/04_assets/ui_suite_gpt_v1/specs/PIXEL_PERFECT_PIPELINE_STATUS.md\`

只有 \`pixel-accepted\` 才表示该图完整走完全部流程；\`review-ready\`、\`diff-pending\`、\`blocked\` 都不是完成。

## 契约自检

\`\`\`bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
\`\`\`
`,
  );

  console.log(`frontend contracts generated: ${rel(SPEC_DIR)}`);
  console.log(`items: ${specs.length}`);
  console.log(`pages: ${pageSpecs.length} + topics contract`);
  console.log(`overlays: ${overlaySpecs.length}`);
}

main();
