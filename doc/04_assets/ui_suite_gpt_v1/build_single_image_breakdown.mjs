import fs from 'node:fs';
import path from 'node:path';

const ROOT = process.cwd();
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SCREENS_DIR = path.join(SUITE_DIR, 'screens');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const BREAKDOWN_DIR = path.join(SPEC_DIR, 'image-breakdowns');
const ROUTE_MAP_PATH = path.join(SPEC_DIR, 'route-page-map.json');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');

const APP_SHELL = {
  topbar: { x: 0, y: 0, w: 1920, h: 80 },
  sidebar: { x: 0, y: 80, w: 166, h: 917 },
  content: { x: 198, y: 80, w: 1722, h: 917 },
  bottombar: { x: 0, y: 997, w: 1920, h: 83 },
  rightRail: { x: 1460, y: 104, w: 420, h: 860 },
};

const PAGE_STATE_TITLES = {
  'alert-detail-evidence-files': '告警详情 / 证据文件',
  'alert-detail-evidence-graph-path': '告警详情 / 图谱路径证据',
  'alert-detail-evidence-logs': '告警详情 / 日志证据',
  'alert-detail-evidence-pcap': '告警详情 / PCAP 证据',
  'alert-detail-evidence-session': '告警详情 / Session 证据',
  'assets-business-system': '资产台账 / 业务系统',
  'assets-detail-basic': '资产台账 / 资产基础详情',
  'assets-detail-history': '资产台账 / 资产历史',
  'assets-detail-network-interface': '资产台账 / 网卡接口',
  'assets-detail-open-services': '资产台账 / 开放服务',
  'assets-detail-ownership': '资产台账 / 归属信息',
  'assets-network-device': '资产台账 / 网络设备',
  'assets-server': '资产台账 / 服务器',
  'assets-unknown': '资产台账 / 未知资产',
  'audit-log-operation-context': '审计日志 / 操作上下文',
  'audit-log-related-chain': '审计日志 / 关联链路',
  'baselines-account': '行为基准 / 账号基线',
  'baselines-port': '行为基准 / 端口基线',
  'baselines-protocol': '行为基准 / 协议基线',
  'baselines-time-window': '行为基准 / 时间窗基线',
  'campaign-detail-impact-account': '战役详情 / 账号影响面',
  'campaign-detail-impact-business-system': '战役详情 / 业务系统影响面',
  'campaign-detail-impact-campus': '战役详情 / 园区影响面',
  'campaign-detail-impact-department': '战役详情 / 部门影响面',
  'campaign-detail-impact-service': '战役详情 / 服务影响面',
  'data-quality-field-quality': '数据质量 / 字段质量',
  'data-quality-flink-quality': '数据质量 / Flink 处理质量',
  'data-quality-replay-reconcile': '数据质量 / 重放与对账',
  'data-quality-report': '数据质量 / 质量报告',
  'data-quality-settings': '数据质量 / 质量规则设置',
  'data-quality-storage-quality': '数据质量 / 存储质量',
  'data-quality-topic-health': '数据质量 / Topic 健康',
  'encrypted-traffic-egress-profile': '加密流量 / 外联画像',
  'encrypted-traffic-evidence-center': '加密流量 / 证据中心',
  'encrypted-traffic-fingerprint': '加密流量 / 指纹分析',
  'encrypted-traffic-tunnel-detection': '加密流量 / 隧道检测',
  'graph-account-access-path': '实体图谱 / 账号访问路径',
  'graph-attack-path': '实体图谱 / 攻击路径',
  'graph-communication-path': '实体图谱 / 通信路径',
  'models-activation-audit-gate': '模型管理 / 激活审计门禁',
  'models-feature-anomaly-explanation': '模型管理 / 特征异常解释',
  'models-feature-rule-contribution': '模型管理 / 特征规则贡献',
  'models-feature-sample-examples': '模型管理 / 样本示例',
  'rules-editor-dependencies': '规则管理 / 依赖引用',
  'rules-editor-test-validation': '规则管理 / 测试验证',
  'rules-sample-logs': '规则管理 / 样本日志',
  'rules-sample-session': '规则管理 / 样本 Session',
  'topics-apt-campaign': '专题面板 / APT 战役',
  'topics-data-exfiltration': '专题面板 / 数据外传',
  'topics-encrypted-tunnel': '专题面板 / 加密隧道',
  'whitelist-condition-account': '白名单 / 账号条件',
  'whitelist-condition-asset': '白名单 / 资产条件',
  'whitelist-condition-ip': '白名单 / IP 条件',
  'whitelist-condition-model': '白名单 / 模型条件',
  'whitelist-condition-rule': '白名单 / 规则条件',
  'whitelist-expiry-expired-unhandled': '白名单 / 过期未处理',
  'whitelist-expiry-long-lived': '白名单 / 长期有效',
  'whitelist-expiry-unassigned-owner': '白名单 / 未分配 Owner',
};

const COMPONENT_SOURCE = {
  Button: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-button.png',
  IconButton: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-icon-button.png',
  StatusTag: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-status-chip.png',
  Tabs: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-tabs.png',
  Segmented: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-segmented.png',
  Dropdown: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-dropdown.png',
  Pagination: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-pagination.png',
  Input: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-input.png',
  Search: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-search.png',
  Select: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-select.png',
  DateRange: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-date-range.png',
  ConditionBuilder: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-condition-builder.png',
  DataTable: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-data-table.png',
  DescriptionList: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-description-list.png',
  MetricTile: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-kpi-tile.png',
  HealthCard: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-health-card.png',
  RankingList: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-ranking-list.png',
  LogList: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-log-list.png',
  EvidenceFileCard: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-file-card.png',
  LineAreaChart: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-line-area-chart.png',
  DonutChart: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-donut-chart.png',
  BarRankingChart: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png',
  SankeyFlow: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-sankey-flow.png',
  RadarQuality: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-radar-quality.png',
  Heatmap: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-heatmap.png',
  TopologyGraph: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-topology-graph.png',
  TimelineStateMachine: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-timeline-state-machine.png',
  ActionRail: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-action-rail.png',
  FeedbackBlock: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-feedback-block.png',
  AcceptanceGateMatrix: 'doc/04_assets/ui_suite_gpt_v1/screens/components/component-acceptance-gate-matrix.png',
};

const BUSINESS_ICON_MAP = [
  ['搜索/检索', ['搜索', '检索', '查询'], 'SearchOutlined'],
  ['筛选', ['筛选', '过滤'], 'FilterOutlined'],
  ['时间窗', ['时间', '日期', '窗口'], 'CalendarOutlined'],
  ['刷新/同步', ['刷新', '同步', '更新'], 'ReloadOutlined'],
  ['导出/报告', ['导出', '报告', '证据包'], 'ExportOutlined'],
  ['下载/PCAP', ['下载', 'PCAP'], 'DownloadOutlined'],
  ['新增', ['新增', '创建', '新建', '添加'], 'PlusOutlined'],
  ['编辑/配置', ['编辑', '配置', '设置'], 'EditOutlined'],
  ['危险动作', ['删除', '吊销', '回滚', '失败'], 'DeleteOutlined'],
  ['风险/告警', ['告警', '高危', '风险', '异常'], 'WarningOutlined'],
  ['健康/通过', ['健康', '通过', '成功'], 'CheckCircleOutlined'],
  ['审计留痕', ['审计', '留痕'], 'AuditOutlined'],
  ['资产/设备', ['资产', '主机', '设备', '业务系统'], 'DatabaseOutlined'],
  ['关系图谱', ['图谱', '拓扑', '关系', '路径', '节点'], 'ApartmentOutlined'],
  ['流程/状态机', ['状态机', '阶段', '时间线', 'DAG'], 'BranchesOutlined'],
  ['模型/MLOps', ['模型', 'MLOps', '训练'], 'DeploymentUnitOutlined'],
  ['规则文档', ['规则', 'DSL'], 'FileTextOutlined'],
  ['通知/订阅', ['通知', '订阅'], 'BellOutlined'],
  ['权限/令牌', ['权限', 'RBAC', '令牌'], 'KeyOutlined'],
];

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {};
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--image') out.image = args[++i];
    else if (arg === '--force') out.force = true;
    else if (arg === '--quiet') out.quiet = true;
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (!out.image) throw new Error('usage: node build_single_image_breakdown.mjs --image <one PNG>');
  return out;
}

function rel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function readJson(file, fallback = null) {
  if (!fs.existsSync(file)) return fallback;
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function writeJson(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function writeText(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, value.trimEnd() + '\n');
}

function pngDimensions(file) {
  const buf = fs.readFileSync(file);
  if (buf.length < 24 || buf.toString('hex', 0, 8) !== '89504e470d0a1a0a') {
    throw new Error(`${file} is not a PNG`);
  }
  return { width: buf.readUInt32BE(16), height: buf.readUInt32BE(20) };
}

function clean(value) {
  return value ? value.replace(/[。；;，,]+$/u, '').trim() : value;
}

function lineValue(text, label) {
  const match = text.match(new RegExp(`^${label}：(.+)$`, 'm'));
  return clean(match?.[1]);
}

function extractBullets(text, heading) {
  const start = text.indexOf(`${heading}：`);
  if (start < 0) return [];
  const bullets = [];
  for (const line of text.slice(start).split('\n').slice(1)) {
    if (/^[\u4e00-\u9fa5A-Za-z0-9 /]+：/.test(line) && bullets.length) break;
    const match = line.match(/^- (.+)$/);
    if (match) bullets.push(clean(match[1]));
    if (!line.trim() && bullets.length) break;
  }
  return bullets;
}

function focusedPrompt(text) {
  const marker = '\n本图类型：';
  const index = text.indexOf(marker);
  return index >= 0 ? text.slice(index + 1) : text;
}

function titleFromSlug(id) {
  return PAGE_STATE_TITLES[id] ?? id.replace(/-/g, ' ');
}

function categoryFromImage(imageAbs) {
  const relative = rel(imageAbs);
  const match = relative.match(/screens\/([^/]+)\//);
  if (!match) throw new Error(`image must be under ${rel(SCREENS_DIR)}: ${relative}`);
  return match[1];
}

function outputDirFor(category) {
  return path.join(BREAKDOWN_DIR, category);
}

function canonicalImagePath(input) {
  const abs = path.isAbsolute(input) ? input : path.join(ROOT, input);
  const base = path.basename(abs);
  if (!base.endsWith('.png')) throw new Error('only PNG images are supported');
  if (base.includes('.raw-') || base.includes('.before-')) throw new Error('raw/before images are not canonical breakdown targets');
  return abs;
}

function routeIds(routeMap) {
  return routeMap.map((item) => item.id);
}

function longestHostPrefix(id, hostIds) {
  if (hostIds.includes(id)) return id;
  if (id.startsWith('topics-')) return 'topics';
  const candidates = hostIds.filter((host) => id.startsWith(`${host}-`));
  return candidates.sort((a, b) => b.length - a.length)[0] ?? null;
}

function findRoute(id, routeMap) {
  const hostId = longestHostPrefix(id, routeIds(routeMap));
  return routeMap.find((item) => item.id === hostId) ?? null;
}

function promptFor(id, hostId = null) {
  const exact = path.join(SUITE_DIR, 'prompts', `${id}.prompt.txt`);
  if (fs.existsSync(exact)) return { file: rel(exact), text: fs.readFileSync(exact, 'utf8'), exact: true };
  if (hostId) {
    const host = path.join(SUITE_DIR, 'prompts', `${hostId}.prompt.txt`);
    if (fs.existsSync(host)) return { file: rel(host), text: fs.readFileSync(host, 'utf8'), exact: false };
  }
  return { file: null, text: '', exact: false };
}

function componentsFromText(id, text, category) {
  const source = `${id} ${text}`;
  const items = new Set();
  const add = (...values) => values.forEach((value) => items.add(value));
  if (category === 'components') return componentsForComponentBoard(id);
  if (category === 'states') return componentsForState(id);
  if (category === 'foundations') add('TokenBoard', 'SpecPanel');
  else if (category === 'responsive') add('ResponsiveFrame', 'BreakpointRule', 'DrawerNavigation');
  else add('WorkPanel', 'Button', 'StatusTag');
  if (/KPI|指标|总览|总分|评分|健康|覆盖率|完整度|通过率|SLA/.test(source)) add('MetricTile', 'HealthCard');
  if (/表格|列表|队列|清单|台账|日志|历史/.test(source)) add('DataTable', 'Pagination');
  if (/搜索|检索|筛选|时间窗|查询|选择|租户/.test(source)) add('Search', 'Select', 'DateRange', 'Input');
  if (/图谱|拓扑|路径|链|DAG|节点|关系/.test(source)) add('TopologyGraph', 'TimelineStateMachine');
  if (/折线|趋势|柱状|排行|热力|雷达|环|桑基|图表/.test(source)) add('LineAreaChart', 'DonutChart', 'BarRankingChart', 'Heatmap');
  if (/表单|编辑|配置|设置|条件|DSL|规则|权限/.test(source)) add('ConditionBuilder', 'Input');
  if (/证据|PCAP|Session|文件|hash|取证/.test(source)) add('EvidenceFileCard');
  if (/反馈|回流|学习/.test(source)) add('FeedbackBlock');
  if (/详情|属性|上下文/.test(source)) add('DescriptionList');
  if (/动作|处置|审批|确认|发布|回滚|删除/.test(source)) add('ActionRail');
  return [...items];
}

function unique(values) {
  return [...new Set(values.filter(Boolean))];
}

function componentsForComponentBoard(id) {
  return unique([...specificComponentForBoard(id), 'ComponentSpecimen', 'StateMatrix', 'TokenStrip']);
}

function componentsForState(id) {
  const key = id.replace(/^state-/, '');
  const base = ['ResultState', 'WorkPanel', 'StatusIllustration', 'ResultTitle', 'ResultDescription', 'Button', 'IconButton'];
  const extras = [];
  if (/(loading|skeleton)/.test(key)) extras.push('Skeleton');
  if (/(empty|no-data|zero)/.test(key)) extras.push('Empty');
  if (/(error|failed|forbidden|unauthorized|offline|degraded|timeout)/.test(key)) extras.push('Alert');
  if (/(success|toast|complete|healthy)/.test(key)) extras.push('StatusTag');
  if (/(table|list|result)/.test(key)) extras.push('DataTable');
  return unique([...base, ...extras]);
}

function specificComponentForBoard(id) {
  const map = [
    ['acceptance-gate-matrix', ['AcceptanceGateMatrix']],
    ['action-rail', ['ActionRail']],
    ['alert-queue', ['DataTable', 'StatusTag']],
    ['alert-timeline', ['TimelineStateMachine']],
    ['app-header', ['AppHeader']],
    ['asset-context', ['DescriptionList', 'StatusTag']],
    ['bar-ranking-chart', ['BarRankingChart']],
    ['batch-action-bar', ['Button', 'Dropdown', 'StatusTag']],
    ['bottom-status-bar', ['BottomStatusBar']],
    ['breadcrumb-context', ['BreadcrumbContext']],
    ['button', ['Button']],
    ['condition-builder', ['ConditionBuilder']],
    ['data-table', ['DataTable', 'Pagination']],
    ['date-range', ['DateRange']],
    ['description-list', ['DescriptionList']],
    ['donut-chart', ['DonutChart']],
    ['dropdown', ['Dropdown']],
    ['empty-card', ['WorkPanel', 'Empty']],
    ['evidence-drawer', ['EvidenceDrawer']],
    ['evidence-file-card', ['EvidenceFileCard']],
    ['feedback-block', ['FeedbackBlock']],
    ['health-card', ['HealthCard']],
    ['heatmap', ['Heatmap']],
    ['icon-button', ['IconButton']],
    ['input', ['Input']],
    ['kpi-tile', ['MetricTile']],
    ['line-area-chart', ['LineAreaChart']],
    ['log-list', ['LogList']],
    ['pagination', ['Pagination']],
    ['permission-card', ['PermissionCard']],
    ['primary-sidebar', ['PrimarySidebar']],
    ['quick-entry', ['QuickEntry']],
    ['radar-quality', ['RadarQuality']],
    ['ranking-list', ['RankingList']],
    ['risk-score', ['RiskScore']],
    ['sankey-flow', ['SankeyFlow']],
    ['search', ['Search']],
    ['secondary-menu', ['SecondaryMenu']],
    ['segmented', ['Segmented']],
    ['select', ['Select']],
    ['site-time-selector', ['SiteTimeSelector']],
    ['status-chip', ['StatusTag']],
    ['switch-checkbox-radio', ['Switch', 'Checkbox', 'Radio']],
    ['tabs', ['Tabs']],
    ['timeline-state-machine', ['TimelineStateMachine']],
    ['tooltip', ['Tooltip']],
    ['topology-graph', ['TopologyGraph']],
    ['user-menu', ['UserMenu']],
  ];
  const key = id.replace(/^component-/, '');
  const found = map.find(([needle]) => key.includes(needle));
  return found ? found[1] : ['ComponentSpecimen'];
}

function iconsFromText(id, text) {
  const source = `${id} ${text}`;
  const icons = [];
  for (const [semantic, keywords, icon] of BUSINESS_ICON_MAP) {
    if (keywords.some((keyword) => source.includes(keyword))) icons.push({ icon, semantic });
  }
  if (!icons.length) icons.push({ icon: 'InfoCircleOutlined', semantic: '信息/说明' });
  return icons;
}

function overlayKind(id) {
  if (id.startsWith('drawer-')) return 'drawer';
  if (id.startsWith('dropdown-')) return 'dropdown';
  if (id.startsWith('popconfirm-')) return 'popconfirm';
  return 'modal';
}

function normalizeRoute(route) {
  route = clean(route);
  return route;
}

function routeToHostId(route, routeMap) {
  return routeMap.find((item) => normalizeRoute(item.route) === normalizeRoute(route))?.id ?? null;
}

function commonMeta({ id, imageRel, dims, category, title, prompt, manifestItem }) {
  return {
    id,
    record_type: 'single-image-breakdown',
    batch_generated: false,
    source_image: imageRel,
    image_type: category.slice(0, -1),
    title,
    canvas: dims,
    prompt: prompt.file,
    prompt_exact_match: prompt.exact,
    manifest_item: manifestItem
      ? {
          id: manifestItem.id,
          type: manifestItem.type,
          title: manifestItem.title,
          targetFile: manifestItem.targetFile,
        }
      : null,
    scope: {
      covers: [path.basename(imageRel)],
      rule: 'one PNG has one detailed breakdown record; no bulk summary may replace this record',
    },
  };
}

function pageRecord(context) {
  const { id, route, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const modules = extractBullets(text, '必须包含的业务模块');
  const focus = lineValue(text, '页面重点') ?? titleFromSlug(id);
  const layout = lineValue(text, '表现形式与布局') ?? '按目标图主体区域拆分为标题/筛选/指标/主工作区/详情或闭环区。';
  const isLogin = id === 'login';
  const title = PAGE_STATE_TITLES[id] ?? lineValue(text, '页面名称') ?? route?.title ?? titleFromSlug(id);
  const components = isLogin
    ? ['AuthLayout', 'AuthBackground', 'AuthBrandHero', 'AuthLoginCard', 'AuthTabs', 'Form', 'Input', 'Select', 'CaptchaChallenge', 'Button']
    : componentsFromText(id, `${tail} ${modules.join(' ')}`, 'pages');
  const regions = isLogin
    ? [
        region('auth-page-root', { x: 0, y: 0, w: dims.width, h: dims.height }, '全屏认证页面', ['AuthLayout'], ['不渲染业务 AppShell']),
        region('auth-background', { x: 0, y: 0, w: dims.width, h: dims.height }, '深色园区/链路认证背景', ['AuthBackground'], ['CSS/背景资产分层，不贴整图']),
        region('left-hero', { x: 0, y: 70, w: 932, h: 850 }, '左侧品牌、安全能力和认证说明', ['AuthBrandHero', 'HologramShield', 'CapabilityPill'], ['盾牌、产品名、能力标签、安全提示']),
        region('login-card', { x: 1017, y: 131, w: 719, h: 765 }, '右侧账号密码登录面板', ['AuthLoginCard', 'Form', 'Tabs'], ['Tab、租户、账号、密码、验证码、记住登录、辅助链接、登录按钮']),
      ]
    : [
        region('topbar', APP_SHELL.topbar, '顶部全局状态栏', ['AppHeader', 'SiteTimeSelector', 'QuickEntry'], ['品牌、站点、时间、风险、告警、采集健康、数据质量、快捷入口']),
        region('sidebar', APP_SHELL.sidebar, '左侧单栏导航', ['PrimarySidebar', 'SecondaryMenu', 'UserMenu'], ['一级菜单、当前域二级菜单、底部用户区']),
        region('content-header', { x: 198, y: 96, w: 1238, h: 70 }, '页面标题、面包屑、状态摘要', ['BreadcrumbContext', 'PageTitle', 'StatusTag'], [focus]),
        region('toolbar-or-tabs', { x: 198, y: 166, w: 1238, h: 64 }, '筛选、搜索、时间窗、Tab 或主动作栏', ['Search', 'Select', 'DateRange', 'Tabs', 'Button'], [layout]),
        region('primary-workspace', { x: 198, y: 238, w: 1238, h: 526 }, '页面主视觉和核心业务区', components, modules.slice(0, 4)),
        region('secondary-workspace', { x: 198, y: 772, w: 1238, h: 192 }, '辅助列表、证据、质量、审计或下钻区', components, modules.slice(4)),
        region('right-rail', APP_SHELL.rightRail, '右侧详情/闭环栏', ['DescriptionList', 'ActionRail', 'FeedbackBlock', 'TimelineStateMachine'], ['选中对象详情、闭环动作、反馈学习、审计留痕']),
        region('bottombar', APP_SHELL.bottombar, '底部全局状态栏', ['BottomStatusBar', 'GlobalActions'], ['数据延迟、系统运行、SLA、质量、存储、带宽、日志吞吐、全局动作']),
      ];
  return {
    ...commonMeta({ id, imageRel, dims, category: 'pages', title, prompt, manifestItem }),
    route: route?.route ?? lineValue(text, '路由') ?? null,
    host_page: route?.id ?? null,
    react_page: route?.pageComponent ? `web/ui/src/pages/${route.pageComponent}.tsx` : null,
    focus,
    modules: modules.length ? modules : ['按目标图和文件名表达的页内状态实现专属业务模块'],
    layout,
    regions,
    component_inventory: components.map(componentInfo),
    icon_inventory: iconsFromText(id, `${tail} ${modules.join(' ')}`),
    states: isLogin
      ? ['default', 'input-focus', 'required-error', 'captcha-loading', 'captcha-error', 'submit-loading', 'login-success', 'login-failed', 'sso-tab']
      : ['loading', 'empty', 'error', 'permission-denied', 'data-ready', 'selected-row-or-node', 'drawer-or-modal-open'],
    implementation_notes: isLogin
      ? ['登录页不套业务 AppShell', '验证码和登录请求必须走 services/api.ts', '登录异常弹窗单独以 modal-login-error-captcha.png 拆解']
      : ['公共 AppShell 按 screen.png 固定', '主工作区不得贴图', '表格/筛选/图表/浮层需真实组件化', '危险动作需影响范围、权限提示和审计 trace'],
  };
}

function overlayRecord(context, routeMap) {
  const { id, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const hostRoute = normalizeRoute(lineValue(text, '基准路由'));
  const hostPage = hostRoute ? routeToHostId(hostRoute, routeMap) : null;
  const kind = overlayKind(id);
  const title = lineValue(text, '浮层名称') ?? titleFromSlug(id);
  const focus = lineValue(text, '浮层重点') ?? title;
  const layout = lineValue(text, '浮层布局') ?? '浮层本体为主，背景轻微压暗，动作区清晰。';
  const regions = overlayRegions(kind, dims, focus, layout);
  return {
    ...commonMeta({ id, imageRel, dims, category: 'overlays', title, prompt, manifestItem }),
    overlay_kind: kind,
    host_route: hostRoute,
    host_page: hostPage,
    recommended_ant_design: kind === 'drawer' ? 'Drawer' : kind === 'dropdown' ? 'Dropdown/Menu' : kind === 'popconfirm' ? 'Popconfirm' : 'Modal',
    focus,
    layout,
    trigger_location: triggerLocation(id),
    regions,
    component_inventory: componentsFromText(id, tail, 'overlays').map(componentInfo),
    icon_inventory: iconsFromText(id, tail),
    states: ['closed', 'opening', 'open', 'validation-error', 'submitting', 'success', 'failed'],
    implementation_notes: ['只实现当前浮层本体，不恢复完整宿主截图', '危险/提交动作必须包含取消、确认、权限提示、影响范围和审计留痕', '浮层尺寸、Header、Body、Footer 操作区保持稳定'],
  };
}

function componentRecord(context) {
  const { id, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const title = lineValue(text, '组件名称') ?? manifestItem?.title ?? titleFromSlug(id);
  const components = componentsFromText(id, tail, 'components');
  return {
    ...commonMeta({ id, imageRel, dims, category: 'components', title, prompt, manifestItem }),
    purpose: lineValue(text, '组件重点') ?? '作为前端复刻时的组件形态、密度、状态矩阵和 token 参考。',
    regions: [
      region('component-title', { x: 80, y: 58, w: 1760, h: 72 }, '组件名称、用途和使用说明', ['ComponentTitle'], [title]),
      region('primary-specimen', { x: 80, y: 140, w: 980, h: 780 }, '组件主样例和布局结构', components, ['按目标图重建结构、间距、文字、图标和状态色']),
      region('state-matrix', { x: 1100, y: 140, w: 740, h: 780 }, 'normal/hover/active/disabled/loading/error 等状态矩阵', ['StateMatrix'], ['每个状态必须有稳定尺寸，不能影响布局']),
      region('token-strip', { x: 80, y: 940, w: 1760, h: 88 }, '颜色、字号、边框、圆角、阴影、间距 token', ['TokenStrip'], ['落到 CSS token 或 Ant Design theme']),
    ],
    component_inventory: components.map(componentInfo),
    icon_inventory: iconsFromText(id, tail),
    states: ['normal', 'hover', 'active', 'selected', 'disabled', 'loading', 'error', 'empty'],
    implementation_notes: ['组件板不直接作为页面截图使用', '必须抽象为可复用 React/Ant Design/CSS/ECharts 组件', '所有状态尺寸保持稳定'],
  };
}

function foundationRecord(context) {
  const { id, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const title = manifestItem?.title ?? lineValue(text, '名称') ?? titleFromSlug(id);
  return {
    ...commonMeta({ id, imageRel, dims, category: 'foundations', title, prompt, manifestItem }),
    purpose: '设计系统基础板，用于约束页面、浮层、组件、状态和响应式图，不作为业务页面直接实现。',
    regions: [
      region('foundation-header', { x: 64, y: 48, w: 1792, h: 90 }, '规范板标题和适用范围', ['SpecHeader'], [title]),
      region('token-area-left', { x: 80, y: 150, w: 840, h: 760 }, '布局、颜色、字体、图标或表格等规范样例', ['TokenBoard'], ['提取为 CSS token、Ant Design theme、ECharts theme']),
      region('token-area-right', { x: 960, y: 150, w: 880, h: 760 }, '状态矩阵、组件状态或验收规则', ['SpecPanel', 'StateMatrix'], ['供后续单图拆解引用']),
      region('acceptance-strip', { x: 80, y: 930, w: 1760, h: 90 }, '禁止项和验收约束', ['AcceptanceStrip'], ['不得在业务页中偏离 foundations']),
    ],
    component_inventory: ['TokenBoard', 'SpecPanel', 'StateMatrix', 'AcceptanceStrip'].map(componentInfo),
    icon_inventory: iconsFromText(id, tail),
    states: ['reference-only'],
    implementation_notes: ['从该图抽取 token 和规范', '不要把 foundation 图贴进页面', '如与 screen.png 公共 AppShell 冲突，以 screen.png 公共区为准'],
  };
}

function stateRecord(context) {
  const { id, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const title = manifestItem?.title ?? titleFromSlug(id);
  return {
    ...commonMeta({ id, imageRel, dims, category: 'states', title, prompt, manifestItem }),
    purpose: '通用页面或局部组件状态图，用于统一 loading/empty/error/forbidden/offline/degraded/success 等体验。',
    regions: [
      region('state-container', { x: 280, y: 170, w: 1360, h: 740 }, '状态容器和背景面板', ['ResultState', 'WorkPanel'], ['深色面板、细边框、低透明背景']),
      region('state-symbol', { x: 760, y: 230, w: 400, h: 220 }, '状态图标、加载骨架或空态符号', ['StatusIllustration'], ['图标语义和颜色必须匹配状态']),
      region('state-copy', { x: 560, y: 470, w: 800, h: 160 }, '状态标题、说明、trace 或影响范围', ['ResultTitle', 'ResultDescription'], ['错误态必须有可诊断信息但不泄露敏感内容']),
      region('state-actions', { x: 760, y: 680, w: 400, h: 90 }, '主动作和次动作', ['Button', 'IconButton'], ['重试、返回、查看详情等动作清晰']),
    ],
    component_inventory: componentsFromText(id, tail, 'states').map(componentInfo),
    icon_inventory: iconsFromText(id, tail),
    states: [id.replace(/^state-/, '')],
    implementation_notes: ['401 与 403 不共用同一文案', '错误态带 trace id 或下一步动作', 'loading/empty/error 不改变外层布局尺寸'],
  };
}

function responsiveRecord(context) {
  const { id, prompt, dims, imageRel, manifestItem } = context;
  const text = prompt.text;
  const tail = focusedPrompt(text);
  const title = manifestItem?.title ?? titleFromSlug(id);
  return {
    ...commonMeta({ id, imageRel, dims, category: 'responsive', title, prompt, manifestItem }),
    purpose: '响应式断点参考图，用于说明桌面/平板/移动端下模块折叠、保留和优先级。',
    regions: [
      region('breakpoint-frame', { x: 80, y: 80, w: 1040, h: 900 }, '目标断点主画面', ['ResponsiveFrame'], ['保持核心业务可见，不出现横向溢出']),
      region('folding-rules', { x: 1160, y: 120, w: 680, h: 760 }, '模块折叠、导航形态和危险动作位置说明', ['BreakpointRule'], ['侧栏可折叠为 Drawer，主动作不丢失']),
      region('safe-area-actions', { x: 1160, y: 900, w: 680, h: 100 }, '安全区、底部动作和触控尺寸', ['TouchActionBar'], ['移动端按钮不小于可点击目标']),
    ],
    component_inventory: componentsFromText(id, tail, 'responsive').map(componentInfo),
    icon_inventory: iconsFromText(id, tail),
    states: ['desktop', 'tablet', 'mobile', 'drawer-open', 'content-collapsed'],
    implementation_notes: ['禁止固定 1920px 宽导致移动端横向滚动', '表格移动端改卡片/横向容器必须保持上下文', '浮层在移动端使用全屏 Drawer 或底部操作区'],
  };
}

function region(id, bbox, description, components, elements) {
  return { id, bbox, description, components, elements: elements.filter(Boolean) };
}

function componentInfo(component) {
  return {
    component,
    reference_image: COMPONENT_SOURCE[component] ?? null,
    implementation: implementationFor(component),
  };
}

function implementationFor(component) {
  if (['MetricTile', 'StatusTag', 'WorkPanel'].includes(component)) return `web/ui/src/components/${component}.tsx`;
  if (['LineAreaChart', 'DonutChart', 'BarRankingChart', 'Heatmap', 'TopologyGraph'].includes(component)) return 'web/ui/src/components/charts.tsx or ECharts option builder';
  if (['Button', 'Input', 'Select', 'DateRange', 'DataTable', 'Pagination', 'Tabs', 'Dropdown'].includes(component)) return 'Ant Design with local dark tokens';
  return 'local React/CSS component';
}

function overlayRegions(kind, dims, focus, layout) {
  if (kind === 'drawer') {
    return [
      region('host-backdrop', { x: 0, y: 0, w: dims.width, h: dims.height }, '宿主页面弱化背景', ['HostPage', 'Mask'], ['只作上下文，不重绘完整宿主页']),
      region('drawer-surface', { x: 980, y: 48, w: 900, h: 984 }, '右侧 Drawer 容器', ['Drawer', 'WorkPanel'], [focus]),
      region('drawer-summary', { x: 1018, y: 96, w: 824, h: 148 }, '顶部摘要、状态和关键指标', ['DescriptionList', 'StatusTag', 'MetricTile'], [layout]),
      region('drawer-content', { x: 1018, y: 252, w: 824, h: 640 }, '详情 Tab、表格、时间线或证据列表', ['Tabs', 'DataTable', 'TimelineStateMachine', 'EvidenceFileCard'], ['主体信息区']),
      region('drawer-actions', { x: 1320, y: 934, w: 520, h: 48 }, '底部操作区', ['Button', 'IconButton'], ['取消、确认、导出、审批等动作']),
    ];
  }
  if (kind === 'dropdown') {
    return [
      region('dropdown-anchor', { x: 1280, y: 96, w: 180, h: 36 }, '锚点按钮或行操作入口', ['Button', 'IconButton'], ['More/Down 图标']),
      region('dropdown-menu', { x: 1280, y: 136, w: 480, h: 520 }, '下拉菜单内容', ['Dropdown', 'Menu', 'StatusTag'], [focus, layout]),
    ];
  }
  if (kind === 'popconfirm') {
    return [
      region('popconfirm-anchor', { x: 1060, y: 360, w: 96, h: 32 }, '危险动作或下载动作锚点', ['Button', 'IconButton'], ['Delete/Download 图标']),
      region('popconfirm-card', { x: 640, y: 330, w: 640, h: 400 }, '确认卡片、影响范围和二次确认', ['Popconfirm', 'Alert', 'Button'], [focus, layout]),
    ];
  }
  return [
    region('modal-backdrop', { x: 0, y: 0, w: dims.width, h: dims.height }, '全屏压暗背景', ['Mask'], ['宿主页面只作上下文']),
    region('modal-surface', { x: 360, y: 170, w: 1200, h: 740 }, '居中 Modal 容器', ['Modal', 'WorkPanel'], [focus]),
    region('modal-body', { x: 400, y: 250, w: 1120, h: 540 }, '表单、详情、状态机、表格或确认内容', ['Form', 'DescriptionList', 'DataTable', 'Alert'], [layout]),
    region('modal-actions', { x: 1160, y: 900, w: 360, h: 48 }, '底部取消/确认操作区', ['Button', 'IconButton'], ['危险动作需二次确认']),
  ];
}

function triggerLocation(id) {
  if (id.includes('global-search')) return '顶部快捷入口或全局搜索动作';
  if (id.includes('quick-entry')) return '顶部快捷入口组';
  if (id.includes('user-menu')) return '左侧底部用户区';
  if (id.includes('row-actions')) return '表格行操作 More 按钮';
  if (id.includes('batch')) return '表格批量操作栏';
  if (id.includes('delete') || id.includes('revoke')) return '危险操作按钮旁';
  if (id.includes('download') || id.includes('export')) return '导出/下载按钮旁';
  if (id.startsWith('drawer-')) return '主表格行、图谱节点、证据项或右侧详情入口';
  return '页面主操作按钮或详情动作区';
}

function markdown(record) {
  const lines = [];
  lines.push(`# ${record.id}.png 单图拆解记录`);
  lines.push('');
  lines.push('## 记录信息');
  lines.push('');
  lines.push(`- 图片 ID：\`${record.id}\``);
  lines.push(`- 图片类型：\`${record.image_type}\``);
  lines.push(`- 标题：${record.title}`);
  lines.push(`- 源图：\`${record.source_image}\``);
  lines.push(`- 源图尺寸：\`${record.canvas.width}x${record.canvas.height}\``);
  if (record.route) lines.push(`- 路由：\`${record.route}\``);
  if (record.host_route) lines.push(`- 宿主路由：\`${record.host_route}\``);
  if (record.react_page) lines.push(`- React 页面：\`${record.react_page}\``);
  if (record.prompt) lines.push(`- Prompt：\`${record.prompt}\`${record.prompt_exact_match ? '' : '（复用宿主页面 prompt）'}`);
  lines.push('- 拆解方式：单张图片独立记录；不允许用批量汇总替代。');
  lines.push('');
  if (record.focus) {
    lines.push('## 业务/视觉重点');
    lines.push('');
    lines.push(`- ${record.focus}`);
    if (record.layout) lines.push(`- 布局：${record.layout}`);
    if (record.modules?.length) {
      lines.push('- 模块：');
      for (const item of record.modules) lines.push(`  - ${item}`);
    }
    lines.push('');
  }
  lines.push('## 区域拆解');
  lines.push('');
  lines.push('| 区域 ID | 位置/尺寸 | 说明 | 组件 | 元素/复刻要点 |');
  lines.push('|---|---:|---|---|---|');
  for (const item of record.regions) {
    lines.push(`| \`${item.id}\` | \`${bbox(item.bbox)}\` | ${item.description} | ${item.components.join(', ')} | ${item.elements.join(' / ')} |`);
  }
  lines.push('');
  lines.push('## 组件清单');
  lines.push('');
  lines.push('| 组件 | 参考图 | 实现落点 |');
  lines.push('|---|---|---|');
  for (const item of record.component_inventory) {
    lines.push(`| \`${item.component}\` | ${item.reference_image ? `\`${item.reference_image}\`` : '-'} | ${item.implementation} |`);
  }
  lines.push('');
  lines.push('## 图标清单');
  lines.push('');
  lines.push('| 图标 | 语义 |');
  lines.push('|---|---|');
  for (const item of record.icon_inventory) lines.push(`| \`${item.icon}\` | ${item.semantic} |`);
  lines.push('');
  lines.push('## 状态与交互');
  lines.push('');
  for (const state of record.states) lines.push(`- \`${state}\``);
  lines.push('');
  lines.push('## 实现注意事项');
  lines.push('');
  for (const note of record.implementation_notes) lines.push(`- ${note}`);
  return lines.join('\n');
}

function bbox(value) {
  return `x=${value.x} y=${value.y} w=${value.w} h=${value.h}`;
}

function main() {
  const args = parseArgs();
  const imageAbs = canonicalImagePath(args.image);
  const imageRel = rel(imageAbs);
  const id = path.basename(imageAbs, '.png');
  const category = categoryFromImage(imageAbs);
  const dims = pngDimensions(imageAbs);
  const routeMap = readJson(ROUTE_MAP_PATH, []);
  const manifest = readJson(MANIFEST_PATH, { items: [] });
  const manifestItem = manifest.items.find((item) => item.targetFile === imageRel || item.file === imageRel || item.id === id) ?? null;
  const route = category === 'pages' ? findRoute(id, routeMap) : null;
  const hostId = route?.id ?? longestHostPrefix(id, routeIds(routeMap));
  const prompt = promptFor(id, hostId);
  const context = { id, category, dims, imageRel, prompt, route, manifestItem };
  let record;
  if (category === 'pages') record = pageRecord(context);
  else if (category === 'overlays') record = overlayRecord(context, routeMap);
  else if (category === 'components') record = componentRecord(context);
  else if (category === 'foundations') record = foundationRecord(context);
  else if (category === 'states') record = stateRecord(context);
  else if (category === 'responsive') record = responsiveRecord(context);
  else throw new Error(`unsupported image category: ${category}`);
  const outBase = path.join(outputDirFor(category), id);
  const mdFile = `${outBase}.md`;
  const jsonFile = `${outBase}.json`;
  if (!args.force && (fs.existsSync(mdFile) || fs.existsSync(jsonFile))) {
    if (!args.quiet) console.log(`skip existing: ${rel(mdFile)}`);
    return;
  }
  writeJson(jsonFile, record);
  writeText(mdFile, markdown(record));
  if (!args.quiet) console.log(`wrote ${rel(mdFile)}`);
}

main();
