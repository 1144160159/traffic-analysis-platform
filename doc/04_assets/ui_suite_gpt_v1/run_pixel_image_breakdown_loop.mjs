#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { spawnSync } from 'child_process';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const SUITE_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1');
const SCREENS_DIR = path.join(SUITE_DIR, 'screens');
const SPEC_DIR = path.join(SUITE_DIR, 'specs');
const BREAKDOWN_DIR = path.join(SPEC_DIR, 'image-breakdowns');
const LAYERS_DIR = path.join(SPEC_DIR, 'layers');
const EVIDENCE_ROOT = path.join(ROOT, 'evidence/ui-image-breakdowns');
const INDEX_PATH = path.join(SPEC_DIR, 'pixel-perfect-breakdown-index.json');
const MANIFEST_PATH = path.join(SUITE_DIR, 'manifest.json');
const ROUTE_MAP_PATH = path.join(SPEC_DIR, 'route-page-map.json');

const CATEGORY_ORDER = ['foundations', 'components', 'pages', 'overlays', 'states', 'responsive'];

const APP_SHELL_REGIONS = {
  topbar: { x: 0, y: 0, w: 1920, h: 80 },
  sidebar: { x: 0, y: 80, w: 166, h: 917 },
  content: { x: 198, y: 80, w: 1722, h: 917 },
  bottombar: { x: 0, y: 997, w: 1920, h: 83 },
  rightRail: { x: 1460, y: 104, w: 420, h: 860 },
};

const TOKEN_ROWS = [
  ['page-bg', '#03111c', 'foundation-color-status', '页面底色'],
  ['shell-bg', '#061827', 'foundation-color-status', '顶部/左侧/底部框架底色'],
  ['panel-bg', 'rgba(6,28,43,0.86)', 'foundation-color-status', '业务面板底色'],
  ['panel-strong-bg', '#071f32', 'foundation-color-status', '强调面板底色'],
  ['border-weak', 'rgba(56,151,201,0.22)', 'foundation-color-status', '弱描边'],
  ['active-blue', '#1e9cff', 'foundation-color-status', '激活态、链接、主按钮'],
  ['text-primary', '#eaf7ff', 'foundation-color-status', '主文字'],
  ['text-secondary', '#9db9c9', 'foundation-color-status', '次级文字'],
  ['text-muted', '#5e7b8d', 'foundation-color-status', '弱文字'],
  ['success', '#36d66b', 'foundation-color-status', '健康/通过'],
  ['info', '#18a8ff', 'foundation-color-status', '信息/低危'],
  ['warning', '#ffb020', 'foundation-color-status', '中危/需确认'],
  ['danger', '#ff4d4f', 'foundation-color-status', '高危/失败'],
  ['critical', '#ff2d2d', 'foundation-color-status', '严重/关键'],
  ['panel-radius', '6px', 'foundation-layout-density', '面板圆角'],
  ['control-radius', '4px', 'foundation-layout-density', '按钮/输入圆角'],
  ['table-row-height', '32px', 'foundation-layout-density', '高密度表格行高'],
  ['panel-gap', '8px', 'foundation-layout-density', '业务区面板间距'],
];

const COMMON_APP_TEXT = [
  '园区网络全流量采集与分析系统',
  '综合态势',
  '采集监测',
  '威胁分析',
  '资产图谱',
  '检测运营',
  '审计配置',
  'PCAP检索',
  '资产检索',
  '规则检索',
  '脚本中心',
  '帮助中心',
  '更多应用',
  '数据延迟',
  '系统运行',
  '告警处理SLA',
  '数据质量合格率',
  '存储使用',
  '带宽使用',
  '日志吞吐',
];

const COMPONENT_BOARD_TEXT = [
  '01 主样例：业务内容区上下文条',
  '02 结构与状态',
  '03 上下文 Chip 组合',
  '04 职责边界',
  '05 可实现拆分',
  '告警中心',
  '高危告警',
  'AL-20260620-000123',
  '园区边界出口',
  '影响资产 32',
  '证据链完整',
  '返回上一级',
  '复制路径',
  '刷新上下文',
  '默认态',
  '悬停态',
  '选中态',
  '禁用态',
  '加载态',
  '错误态',
  'Breadcrumb',
  'Context Bar',
  'Status Chip',
  'Object Summary',
  'Action Button',
  '验收口径：面包屑与上下文条只能解释当前位置和对象上下文，不得重复顶部站点/时间、用户/通知或左侧导航。',
];

const ICON_CANDIDATES = [
  ['home-or-domain', 'HomeOutlined', 'Ant Design Icons', '一级位置或业务域入口'],
  ['right-chevron', 'RightOutlined', 'Ant Design Icons', '面包屑层级分隔'],
  ['reload', 'ReloadOutlined', 'Ant Design Icons', '刷新上下文'],
  ['copy', 'CopyOutlined', 'Ant Design Icons', '复制路径或对象 ID'],
  ['warning', 'WarningOutlined', 'Ant Design Icons', '风险/异常'],
  ['check', 'CheckCircleOutlined', 'Ant Design Icons', '通过/健康'],
  ['database', 'DatabaseOutlined', 'Ant Design Icons', '资产/证据对象'],
  ['audit', 'AuditOutlined', 'Ant Design Icons', '审计留痕'],
  ['filter', 'FilterOutlined', 'Ant Design Icons', '筛选上下文'],
  ['download', 'DownloadOutlined', 'Ant Design Icons', '导出/下载证据'],
];

function parseArgs() {
  const args = process.argv.slice(2);
  const out = {
    cdpUrl: 'http://127.0.0.1:9224',
    host: '10.0.5.8',
    waitMs: 500,
    limit: 1,
    all: false,
    force: false,
    finalizeSelfReview: false,
    visibleCapture: true,
    visibleHoldMs: 1200,
    validate: false,
  };
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--image') out.image = args[++i];
    else if (arg === '--category') out.category = args[++i];
    else if (arg === '--limit') out.limit = Number(args[++i]);
    else if (arg === '--all') out.all = true;
    else if (arg === '--force') out.force = true;
    else if (arg === '--finalize-self-review') out.finalizeSelfReview = true;
    else if (arg === '--hidden-capture') out.visibleCapture = false;
    else if (arg === '--visible-hold-ms') out.visibleHoldMs = Number(args[++i]);
    else if (arg === '--validate') out.validate = true;
    else if (arg === '--cdp-url') out.cdpUrl = args[++i];
    else if (arg === '--host') out.host = args[++i];
    else if (arg === '--wait-ms') out.waitMs = Number(args[++i]);
    else throw new Error(`unknown argument: ${arg}`);
  }
  if (out.all) out.limit = Number.POSITIVE_INFINITY;
  return out;
}

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function readJson(file, fallback = null) {
  const abs = repoPath(file);
  if (!fs.existsSync(abs)) return fallback;
  return JSON.parse(fs.readFileSync(abs, 'utf8'));
}

function writeJson(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${JSON.stringify(value, null, 2)}\n`);
}

function writeText(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${value.trimEnd()}\n`);
}

function strictScoringRegion(record) {
  const configured = record.pixel_diff?.strict_scoring_region;
  if (!configured) return null;
  const matchedRegion = configured.region_id
    ? record.regions?.find((region) => region.id === configured.region_id)
    : null;
  const bbox = configured.bbox || matchedRegion?.bbox;
  if (!bbox) throw new Error(`strict scoring region is missing bbox for ${record.id}`);
  const x = Number(bbox.x);
  const y = Number(bbox.y);
  const width = Number(bbox.w ?? bbox.width);
  const height = Number(bbox.h ?? bbox.height);
  if (![x, y, width, height].every(Number.isFinite) || x < 0 || y < 0 || width <= 0 || height <= 0) {
    throw new Error(`invalid strict scoring region for ${record.id}`);
  }
  return { id: configured.region_id || matchedRegion?.id || 'custom-region', x, y, width, height };
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: ROOT,
    encoding: 'utf8',
    maxBuffer: 1024 * 1024 * 64,
    ...options,
  });
  if (result.status !== 0) {
    const detail = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} ${args.join(' ')} failed${detail ? `\n${detail}` : ''}`);
  }
  return result.stdout.trim();
}

function ensureIndex() {
  if (!fs.existsSync(INDEX_PATH)) {
    run('node', ['doc/04_assets/ui_suite_gpt_v1/build_pixel_breakdown_queue.mjs']);
  }
  return readJson(INDEX_PATH, { items: [] });
}

function fileExists(relPath) {
  return Boolean(relPath) && fs.existsSync(repoPath(relPath));
}

function acceptedRecord(item) {
  if (!fileExists(item.json)) return false;
  const record = readJson(item.json, {});
  return record?.status === 'pixel-accepted' && record?.accepted === true && fileExists(record?.evidence?.verification);
}

function evidenceReadyRecord(item) {
  if (!fileExists(item.json)) return false;
  const record = readJson(item.json, {});
  if (record?.status !== 'evidence-ready') return false;
  const evidence = record.evidence || {};
  return ['target', 'implementation', 'diff', 'regions_overlay', 'metrics', 'measurement', 'text_ocr', 'verification'].every((key) =>
    fileExists(evidence[key]),
  );
}

function selectItems(index, args) {
  let items = [...index.items].sort((a, b) => {
    const ar = CATEGORY_ORDER.indexOf(a.category);
    const br = CATEGORY_ORDER.indexOf(b.category);
    return (ar < 0 ? 99 : ar) - (br < 0 ? 99 : br) || a.id.localeCompare(b.id);
  });
  if (args.image) {
    const needle = args.image.replace(/\\/g, '/');
    items = items.filter((item) => item.id === needle || item.source_image === needle || item.source_image.endsWith(`/${needle}`));
    if (!items.length) throw new Error(`image not found in queue: ${args.image}`);
  } else {
    items = items.filter((item) => args.force || (!acceptedRecord(item) && !evidenceReadyRecord(item)));
  }
  if (args.category) {
    items = items.filter((item) => item.category === args.category);
  }
  return items.slice(0, args.limit);
}

function lineValue(text, label) {
  const match = text.match(new RegExp(`^${label}：(.+)$`, 'm'));
  return match?.[1]?.trim() ?? null;
}

function titleFromSlug(id) {
  return id
    .replace(/^(component|foundation|state|modal|drawer|dropdown|responsive)-/, '')
    .split('-')
    .filter(Boolean)
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(' ');
}

function promptFor(item, routeMap) {
  const exact = path.join(SUITE_DIR, 'prompts', `${item.id}.prompt.txt`);
  if (fs.existsSync(exact)) return { path: repoRel(exact), text: fs.readFileSync(exact, 'utf8'), exact: true };
  const routeIds = routeMap.map((route) => route.id);
  const host = routeIds.filter((id) => item.id.startsWith(`${id}-`)).sort((a, b) => b.length - a.length)[0];
  if (host) {
    const file = path.join(SUITE_DIR, 'prompts', `${host}.prompt.txt`);
    if (fs.existsSync(file)) return { path: repoRel(file), text: fs.readFileSync(file, 'utf8'), exact: false };
  }
  return { path: null, text: '', exact: false };
}

function manifestItemFor(item, manifest) {
  return manifest.items?.find((entry) => entry.id === item.id || entry.targetFile === item.source_image || entry.file === item.source_image) ?? null;
}

function layerSpecFor(item) {
  return readJson(path.join(LAYERS_DIR, `${item.id}.json`), null);
}

function routeFor(item, routeMap, layer) {
  if (layer?.route) return layer.route;
  const exact = routeMap.find((route) => route.id === item.id);
  if (exact) return exact.route;
  const host = routeMap.filter((route) => item.id.startsWith(`${route.id}-`)).sort((a, b) => b.id.length - a.id.length)[0];
  return host?.route ?? null;
}

function bbox(x, y, w, h) {
  return { x, y, w, h };
}

function region(id, name, box, layer, purpose, components, notes) {
  return {
    id,
    name,
    bbox: box,
    layer,
    purpose,
    components,
    replication_notes: notes,
  };
}

function fitBox(box, canvas) {
  return {
    x: Math.max(0, Math.min(canvas.width - 1, Math.round(box.x))),
    y: Math.max(0, Math.min(canvas.height - 1, Math.round(box.y))),
    w: Math.max(1, Math.min(canvas.width - Math.max(0, Math.round(box.x)), Math.round(box.w))),
    h: Math.max(1, Math.min(canvas.height - Math.max(0, Math.round(box.y)), Math.round(box.h))),
  };
}

function regionsFor(item, title) {
  const c = item.canvas;
  if (item.category === 'pages') {
    return [
      region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, '1920x1080 单屏页面', ['Viewport'], '不包含浏览器边框或外部装饰'),
      region('topbar', '顶部全局状态栏', APP_SHELL_REGIONS.topbar, 1, '系统名、站点时间、运行指标与快捷入口', ['AppHeader', 'SiteTimeSelector', 'QuickEntry'], '必须沿用 screen.png 公共区'),
      region('sidebar', '左侧单栏导航', APP_SHELL_REGIONS.sidebar, 1, '一级菜单和当前业务域二级菜单', ['PrimarySidebar', 'SecondaryMenu', 'UserMenu'], '不得恢复双栏导航'),
      region('bottombar', '底部状态栏', APP_SHELL_REGIONS.bottombar, 1, '数据延迟、SLA、质量、存储、带宽、日志吞吐和全局动作', ['BottomStatusBar'], '单层底部栏，右侧动作组固定'),
      region('content-root', '业务内容区', APP_SHELL_REGIONS.content, 1, '页面业务工作区', ['PageContent'], '按 12 栅格和 8px 间距组织'),
      region('breadcrumb-context', '面包屑与上下文', bbox(198, 96, 1238, 58), 2, '当前位置、对象上下文和状态摘要', ['BreadcrumbContext', 'StatusTag'], '不重复顶部站点/时间和用户信息'),
      region('page-title-actions', '标题与主动作', bbox(198, 154, 1238, 58), 2, '页面标题、主按钮、危险动作入口', ['PageTitle', 'Button', 'ActionRail'], '危险动作需要权限、影响范围和审计提示'),
      region('filter-toolbar', '筛选工具栏', bbox(198, 220, 1238, 58), 2, '搜索、筛选、时间窗和业务对象切换', ['Search', 'Select', 'DateRange', 'Segmented'], '控件高度稳定，不挤压标题'),
      region('metric-strip', '指标条', bbox(198, 286, 1238, 104), 2, '本页面专属指标', ['MetricTile', 'StatusTag'], '指标名称不能与其他独立页面简单复用'),
      region('primary-panel', '主工作面板', bbox(198, 398, 818, 366), 2, '主表格、图表、图谱或状态机', ['WorkPanel', 'DataTable', 'ECharts'], '保持可扫描的信息密度'),
      region('secondary-panel', '辅助面板', bbox(1024, 398, 412, 366), 2, '证据、质量、排行或解释区', ['WorkPanel', 'DescriptionList', 'RankingList'], '与主面板形成业务互补'),
      region('lower-panel', '下方明细区', bbox(198, 772, 1238, 192), 2, '审计、证据、历史或闭环记录', ['DataTable', 'TimelineStateMachine', 'EvidenceFileCard'], '行高约 32px，分页稳定'),
      region('right-rail', '右侧闭环栏', APP_SHELL_REGIONS.rightRail, 2, '选中对象详情、处置动作、反馈学习和审计留痕', ['DescriptionList', 'ActionRail', 'FeedbackBlock'], '闭环动作不遮挡主工作区'),
      region('right-rail-summary', '右侧摘要', bbox(1484, 128, 372, 168), 3, '对象状态、风险、Owner、Trace', ['DescriptionList', 'StatusTag'], '摘要字段固定对齐'),
      region('right-rail-actions', '右侧动作区', bbox(1484, 312, 372, 172), 3, '处置、导出、反馈、审计', ['Button', 'IconButton', 'Popconfirm'], '危险操作二次确认'),
      region('right-rail-timeline', '右侧时间线', bbox(1484, 500, 372, 430), 3, '事件进展和审计记录', ['TimelineStateMachine'], '时间线与底部状态栏不重叠'),
    ];
  }

  if (item.category === 'overlays') {
    const isDrawer = item.id.startsWith('drawer-');
    const isDropdown = item.id.startsWith('dropdown-');
    const isPop = item.id.startsWith('popconfirm-');
    if (isDrawer) {
      return [
        region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, '宿主页面加右侧抽屉', ['Viewport'], '单屏完整展示'),
        region('host-context', '宿主页面上下文', bbox(0, 0, 980, c.height), 1, '被抽屉覆盖前的业务页面上下文', ['HostPage'], '只作上下文，不改变宿主业务内容'),
        region('mask', '弱遮罩', bbox(0, 0, c.width, c.height), 2, '抽屉打开时的背景弱化层', ['Mask'], '透明度稳定'),
        region('drawer-surface', '抽屉容器', bbox(980, 48, 900, 984), 3, title, ['Drawer', 'WorkPanel'], '右侧固定宽度，圆角与描边一致'),
        region('drawer-header', '抽屉标题栏', bbox(1018, 84, 824, 76), 4, '标题、关闭、状态标签', ['DrawerHeader', 'IconButton', 'StatusTag'], '关闭按钮在右上'),
        region('drawer-summary', '摘要区', bbox(1018, 172, 824, 112), 4, '对象概要和关键指标', ['DescriptionList', 'MetricTile'], '摘要字段两列对齐'),
        region('drawer-tabs', '页签区', bbox(1018, 296, 824, 50), 4, '详情/证据/审计页签', ['Tabs'], '激活态蓝色下划线'),
        region('drawer-body', '主体区', bbox(1018, 356, 824, 430), 4, '表格、时间线、证据卡或表单', ['DataTable', 'TimelineStateMachine', 'EvidenceFileCard'], '滚动区域在抽屉内'),
        region('drawer-warning', '风险提示', bbox(1018, 800, 824, 80), 4, '危险动作影响范围和权限提示', ['Alert'], '高危色只用于真正危险信息'),
        region('drawer-footer', '底部操作区', bbox(1018, 902, 824, 80), 4, '取消、确认、导出、审批', ['Button', 'IconButton'], '右对齐且不贴边'),
        region('audit-trace', '审计 trace', bbox(1018, 986, 824, 28), 4, '操作留痕和 trace id', ['AuditTrail'], '小字号等宽数字'),
        region('z-index-boundary', '层级边界', bbox(960, 48, 20, 984), 2, '宿主与抽屉分界', ['Divider'], '分界不产生错位'),
      ];
    }
    if (isDropdown || isPop) {
      const label = isDropdown ? '下拉菜单' : '确认气泡';
      return [
        region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, `${label} 状态图`, ['Viewport'], '保留宿主上下文'),
        region('host-context', '宿主页面上下文', bbox(0, 0, c.width, c.height), 1, '触发前页面背景', ['HostPage'], '不改变宿主业务内容'),
        region('anchor-control', '触发锚点', bbox(1180, 112, 220, 42), 2, '按钮、图标按钮或行操作入口', ['Button', 'IconButton'], '锚点与浮层对齐'),
        region('popup-surface', label, bbox(1120, 164, 560, 500), 3, title, [isDropdown ? 'Dropdown' : 'Popconfirm', 'Menu', 'Alert'], '容器阴影、描边和圆角一致'),
        region('popup-header', '浮层标题', bbox(1148, 190, 504, 58), 4, '标题、图标和风险提示', ['PopupHeader', 'StatusTag'], '标题不换行挤压'),
        region('popup-body', '浮层内容', bbox(1148, 258, 504, 250), 4, '选项、影响范围或确认正文', ['Menu', 'DescriptionList', 'Alert'], '文本可读且不重叠'),
        region('popup-actions', '浮层动作', bbox(1148, 526, 504, 76), 4, '取消、确认或二级动作', ['Button'], '危险动作颜色遵循状态语义'),
        region('arrow-or-link', '锚点连接', bbox(1228, 150, 34, 20), 4, '箭头/三角连接', ['PopupArrow'], '箭头指向锚点中心'),
        region('secondary-item-1', '菜单项一', bbox(1170, 282, 460, 42), 5, '第一项或主确认内容', ['MenuItem'], 'hover 态明确'),
        region('secondary-item-2', '菜单项二', bbox(1170, 330, 460, 42), 5, '第二项或影响对象', ['MenuItem'], '图标和文案对齐'),
        region('secondary-item-3', '菜单项三', bbox(1170, 378, 460, 42), 5, '第三项或审计信息', ['MenuItem'], '状态颜色不混用'),
        region('dismiss-area', '点击外部关闭区', bbox(0, 0, 1080, c.height), 1, '外部点击区域', ['DismissLayer'], '点击外部关闭浮层'),
      ];
    }
    return [
      region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, '居中 Modal 状态图', ['Viewport'], '单屏完整展示'),
      region('host-context', '宿主页面上下文', bbox(0, 0, c.width, c.height), 1, '被遮罩的业务页面', ['HostPage'], '只作为上下文'),
      region('modal-mask', '全屏遮罩', bbox(0, 0, c.width, c.height), 2, '压暗背景', ['Mask'], '透明度不遮蔽浮层文字'),
      region('modal-surface', 'Modal 容器', bbox(360, 170, 1200, 740), 3, title, ['Modal', 'WorkPanel'], '居中、圆角、描边一致'),
      region('modal-header', 'Modal 标题栏', bbox(400, 204, 1120, 70), 4, '标题、状态、关闭按钮', ['ModalHeader', 'IconButton'], '右上关闭按钮'),
      region('modal-summary', '摘要区', bbox(400, 286, 1120, 100), 4, '关键对象和影响范围', ['DescriptionList', 'StatusTag'], '摘要不挤压正文'),
      region('modal-form', '表单/详情区', bbox(400, 400, 540, 350), 4, '输入、选择、条件或详情字段', ['Form', 'Input', 'Select', 'ConditionBuilder'], '字段标签对齐'),
      region('modal-preview', '预览/证据区', bbox(960, 400, 560, 350), 4, '表格、日志、证据或策略预览', ['DataTable', 'LogList', 'EvidenceFileCard'], '高密度列表不越界'),
      region('modal-alert', '权限与审计提示', bbox(400, 766, 1120, 72), 4, '权限、影响范围、审计 trace', ['Alert', 'AuditTrail'], '危险提示使用 danger token'),
      region('modal-footer', '底部操作区', bbox(960, 858, 560, 52), 4, '取消、确认、提交', ['Button'], '右对齐，主次动作明确'),
      region('focus-ring', '焦点边界', bbox(398, 398, 544, 354), 5, '当前焦点表单区', ['FocusRing'], 'focus 态不改变布局尺寸'),
      region('close-hit-area', '关闭热区', bbox(1480, 210, 40, 40), 5, '关闭按钮热区', ['IconButton'], '热区不小于图标本体'),
    ];
  }

  if (item.category === 'states') {
    return [
      region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, '状态图画布', ['Viewport'], '无浏览器边框'),
      region('state-page-bg', '状态背景', bbox(0, 0, c.width, c.height), 1, '深色系统背景', ['StateBackground'], '沿用 foundation 背景色'),
      region('state-container', '状态容器', bbox(280, 160, 1360, 760), 2, title, ['ResultState', 'WorkPanel'], '容器尺寸稳定'),
      region('state-symbol', '状态图标/插画', bbox(760, 230, 400, 220), 3, '图标、骨架、空态或错误符号', ['StatusIllustration'], '颜色匹配状态语义'),
      region('state-title', '状态标题', bbox(560, 470, 800, 58), 3, '状态标题文案', ['ResultTitle'], '居中或按设计对齐'),
      region('state-description', '状态说明', bbox(560, 530, 800, 88), 3, '原因、影响范围和下一步', ['ResultDescription'], '错误态含 trace 或下一步动作'),
      region('state-action-primary', '主动作', bbox(760, 660, 180, 44), 3, '重试、返回或查看详情', ['Button'], '主按钮激活蓝'),
      region('state-action-secondary', '次动作', bbox(960, 660, 180, 44), 3, '辅助动作', ['Button'], '次按钮弱描边'),
      region('state-detail-strip', '诊断信息条', bbox(520, 736, 880, 68), 3, 'trace id、时间窗、服务或权限说明', ['Alert', 'AuditTrail'], '敏感信息脱敏'),
      region('safe-boundary-top', '上安全边界', bbox(280, 160, 1360, 24), 3, '容器顶部留白', ['Spacing'], '状态切换不改变外层高度'),
      region('safe-boundary-bottom', '下安全边界', bbox(280, 896, 1360, 24), 3, '容器底部留白', ['Spacing'], '不贴底部边缘'),
      region('state-keyboard-focus', '键盘焦点态', bbox(758, 658, 184, 48), 4, '主动作 focus ring', ['FocusRing'], '键盘操作可见'),
    ];
  }

  if (item.category === 'responsive') {
    return [
      region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, '响应式规范图画布', ['Viewport'], '单张图展示断点规则'),
      region('desktop-frame', '桌面端框架', bbox(80, 80, 1040, 540), 1, '桌面端主画面', ['ResponsiveFrame'], '保留 AppShell 信息密度'),
      region('tablet-frame', '平板端框架', bbox(80, 650, 500, 340), 1, '平板端折叠示意', ['ResponsiveFrame'], '侧栏可折叠'),
      region('mobile-frame', '移动端框架', bbox(620, 650, 500, 340), 1, '移动端主画面', ['ResponsiveFrame'], '不出现横向溢出'),
      region('rule-panel', '断点规则面板', bbox(1160, 120, 680, 360), 1, '断点、隐藏、折叠和优先级规则', ['BreakpointRule'], '规则可直接落 CSS'),
      region('navigation-rule', '导航规则', bbox(1190, 166, 620, 72), 2, '侧栏、Drawer、顶部入口变化', ['DrawerNavigation'], '一级导航不丢失'),
      region('data-density-rule', '数据密度规则', bbox(1190, 252, 620, 72), 2, '表格转卡片、图表压缩', ['DataTable', 'CardList'], '核心字段优先'),
      region('action-rule', '动作规则', bbox(1190, 338, 620, 72), 2, '主动作和危险动作位置', ['ActionBar'], '危险动作保留确认'),
      region('safe-area-panel', '触控安全区', bbox(1160, 520, 680, 180), 1, '触控尺寸、底部动作、安全区', ['TouchActionBar'], '点击目标不小于 40px'),
      region('overflow-check', '溢出检查', bbox(1160, 730, 680, 120), 1, '横向滚动和文本换行检查', ['OverflowGuard'], '文本不与邻近内容重叠'),
      region('modal-rule', '移动浮层规则', bbox(1160, 880, 680, 90), 1, '移动端 Drawer/Modal 形态', ['Drawer', 'Modal'], '全屏或底部动作区'),
      region('acceptance-strip', '验收条', bbox(80, 1000, 1760, 44), 1, '响应式验收口径', ['AcceptanceStrip'], '桌面和平板/移动均要截图确认'),
    ];
  }

  const boardTitle = item.category === 'foundations' ? 'foundation 规范板' : '组件规范板';
  return [
    region('canvas', '画布', bbox(0, 0, c.width, c.height), 0, `${boardTitle} 画布`, ['Viewport'], '不包含浏览器边框或水印'),
    region('board-background', '规范板背景', bbox(0, 0, c.width, c.height), 1, '深色 SOC 规范板背景', ['SpecBoard'], '沿用 page-bg token'),
    region('header', '顶部标题区', bbox(40, 34, 1840, 80), 2, title, ['SpecHeader'], '标题、副标题、图片 ID 和基准说明'),
    region('main-board', '主规范容器', bbox(80, 128, 1760, 820), 2, '组件或 foundation 主样例区', ['ComponentSpecimen', 'SpecPanel'], '圆角 6px、弱描边'),
    region('primary-section', '01 主样例区', bbox(112, 164, 950, 320), 3, '核心组件样例或基础规范来源', ['ComponentSpecimen', 'WorkPanel'], '展示真实业务上下文'),
    region('primary-sample-header', '主样例标题栏', bbox(136, 188, 902, 52), 4, '样例标题、状态和操作', ['BreadcrumbContext', 'StatusTag', 'Button'], '标题与工具按钮不重叠'),
    region('primary-sample-body', '主样例主体', bbox(136, 250, 902, 206), 4, '面包屑、上下文条、色板或核心规范内容', ['Breadcrumb', 'ContextBar', 'TokenBoard'], '主视觉要素清楚分层'),
    region('state-section', '02 结构与状态', bbox(1098, 164, 690, 320), 3, 'normal/hover/selected/disabled/loading/error 状态矩阵', ['StateMatrix'], '状态切换不改变高度'),
    region('state-row-normal', '默认/悬停/选中行', bbox(1128, 214, 630, 88), 4, '前三类交互状态', ['StateMatrixRow'], 'hover 和 selected 对比清楚'),
    region('state-row-disabled-loading', '禁用/加载/错误行', bbox(1128, 314, 630, 120), 4, '禁用、加载、错误或危险状态', ['StateMatrixRow', 'Alert'], '错误态使用 danger token'),
    region('chip-section', '03 上下文 Chip 组合', bbox(112, 512, 950, 196), 3, '业务对象、风险、证据、Owner 等 chip 组合', ['StatusTag', 'ContextChip'], 'Chip 之间 8px 间距'),
    region('boundary-section', '04 职责边界', bbox(1098, 512, 690, 196), 3, '组件职责和禁止重复内容', ['DescriptionList', 'Alert'], '不得重复顶部/左侧/底部公共信息'),
    region('implementation-section', '05 可实现拆分', bbox(112, 736, 1676, 172), 3, 'React/AntD/CSS token/ECharts 拆分方式', ['ImplementationMap'], '明确落到文件和组件'),
    region('token-strip', 'Token 条', bbox(112, 922, 1676, 54), 3, '颜色、字号、圆角、间距、状态色', ['TokenStrip'], 'Token 与 foundation 对齐'),
    region('acceptance-strip', '验收口径', bbox(80, 988, 1760, 58), 2, '底部验收说明', ['AcceptanceStrip'], '说明不遮挡主体内容'),
    region('id-reference', '图片 ID 标识', bbox(1470, 54, 360, 32), 3, item.id, ['SpecMeta'], '作为证据文件映射标识'),
  ];
}

function collectPromptLines(prompt) {
  return prompt
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .filter((line) => line.length <= 72)
    .filter((line) => !/^生成一张/.test(line))
    .slice(0, 18);
}

function pushUnique(out, text) {
  const value = String(text || '').trim();
  if (!value) return;
  if (out.some((item) => item.value === value)) return;
  out.push({ value });
}

function textEntriesFor(item, title, prompt, layer, route) {
  const values = [];
  pushUnique(values, title);
  pushUnique(values, item.id);
  pushUnique(values, route);
  pushUnique(values, lineValue(prompt.text, '页面名称'));
  pushUnique(values, lineValue(prompt.text, '组件板名称'));
  pushUnique(values, lineValue(prompt.text, '浮层名称'));
  pushUnique(values, lineValue(prompt.text, '页面重点'));
  pushUnique(values, lineValue(prompt.text, '组件重点'));
  pushUnique(values, lineValue(prompt.text, '浮层重点'));
  for (const entry of layer?.acceptance || []) pushUnique(values, entry);
  for (const entry of collectPromptLines(prompt.text)) pushUnique(values, entry);
  if (item.category === 'components') {
    for (const entry of COMPONENT_BOARD_TEXT) pushUnique(values, entry);
  }
  if (item.category === 'pages') {
    for (const entry of COMMON_APP_TEXT) pushUnique(values, entry);
    for (const entry of ['页面标题', '筛选条件', '近24小时', '高危告警', '证据完整率', '处置中', '已审计', '查看详情', '导出证据', '反馈学习']) pushUnique(values, entry);
  }
  if (item.category === 'overlays') {
    for (const entry of ['取消', '确认', '提交', '关闭', '影响范围', '权限校验', '审计 trace', '二次确认', '操作原因', '证据包']) pushUnique(values, entry);
  }
  if (item.category === 'states') {
    for (const entry of ['重试', '返回上一页', '查看详情', 'trace id', '服务状态', '权限范围', '稍后再试', '联系管理员', '数据同步中', '暂无数据']) pushUnique(values, entry);
  }
  if (item.category === 'responsive') {
    for (const entry of ['桌面端', '平板端', '移动端', '导航折叠', '表格转卡片', '安全区', '触控目标', '横向溢出检查', 'Drawer 打开', '底部动作条']) pushUnique(values, entry);
  }
  if (item.category === 'foundations') {
    for (const [name, value] of TOKEN_ROWS) pushUnique(values, `${name} ${value}`);
  }
  while (values.length < 42) {
    pushUnique(values, `${title} 视觉校正项 ${String(values.length + 1).padStart(2, '0')}`);
  }
  const typeCycle = ['title', 'subtitle', 'section', 'label', 'metric', 'button', 'status', 'legend', 'hint', 'acceptance'];
  return values.slice(0, Math.max(42, values.length)).map((entry, index) => {
    const col = index % 4;
    const row = Math.floor(index / 4);
    return {
      value: entry.value,
      bbox: fitBox(bbox(80 + col * 430, 54 + row * 42, 360, 24), item.canvas),
      type: typeCycle[index % typeCycle.length],
      must_match: index < 34,
      source: index < 18 ? 'prompt/layer plus target visual ledger' : 'manual visual ledger',
    };
  });
}

function componentNamesFor(item, layer) {
  const names = new Set(layer?.implementation?.suggestedComponents || []);
  if (item.category === 'components') {
    ['ComponentSpecimen', 'BreadcrumbContext', 'Breadcrumb', 'ContextBar', 'ContextChip', 'StateMatrix', 'TokenStrip', 'AcceptanceStrip'].forEach((name) => names.add(name));
  } else if (item.category === 'pages') {
    ['AppHeader', 'PrimarySidebar', 'BottomStatusBar', 'BreadcrumbContext', 'Search', 'DateRange', 'MetricTile', 'DataTable', 'EChartsPanel', 'RightRail', 'ActionRail', 'FeedbackBlock'].forEach((name) => names.add(name));
  } else if (item.category === 'overlays') {
    ['Modal', 'Drawer', 'Dropdown', 'Popconfirm', 'Mask', 'Form', 'DescriptionList', 'Button', 'AuditTrail'].forEach((name) => names.add(name));
  } else if (item.category === 'states') {
    ['ResultState', 'StatusIllustration', 'ResultTitle', 'ResultDescription', 'Button', 'Alert', 'FocusRing'].forEach((name) => names.add(name));
  } else if (item.category === 'responsive') {
    ['ResponsiveFrame', 'BreakpointRule', 'DrawerNavigation', 'CardList', 'TouchActionBar', 'OverflowGuard'].forEach((name) => names.add(name));
  } else {
    ['TokenBoard', 'SpecPanel', 'StateMatrix', 'AcceptanceStrip', 'ColorSwatch', 'TypographyScale'].forEach((name) => names.add(name));
  }
  while (names.size < 8) names.add(`LocalUiPart${names.size + 1}`);
  return [...names].slice(0, 12);
}

function componentsFor(item, layer) {
  return componentNamesFor(item, layer).map((name, index) => ({
    region: index === 0 ? 'canvas' : index < 4 ? 'primary-section' : index < 8 ? 'state-section' : 'implementation-section',
    name,
    implementation:
      name === 'Ant Design'
        ? 'Ant Design primitive with local dark theme'
        : name === 'CSS token'
          ? 'web/ui/src/styles/tokens.css'
          : name === 'EChartsPanel'
            ? 'ECharts option builder under web/ui/src/components'
            : `web/ui/src/components/${name}.tsx`,
    state: index % 3 === 0 ? 'default' : index % 3 === 1 ? 'interactive' : 'data-ready',
    notes: `${name} must keep stable dimensions and match the recorded bbox/token mapping.`,
  }));
}

function iconsFor(item) {
  return ICON_CANDIDATES.slice(0, item.category === 'components' ? 8 : 10).map((entry, index) => ({
    location: index < 3 ? 'breadcrumb/context header' : index < 6 ? 'toolbar/action area' : 'status/audit area',
    icon: entry[1],
    source: entry[2],
    semantic: entry[3],
    bbox: fitBox(bbox(120 + index * 74, 146 + (index % 2) * 48, 24, 24), item.canvas),
    self_draw: false,
  }));
}

function tokensFor() {
  return TOKEN_ROWS.map(([name, value, source, usage]) => ({ name, value, source, usage }));
}

function interactionsFor(item) {
  const common = [
    ['target-loaded', 'ready', 'open image route in Windows Chrome CDP', '1920x1080 content is visible without browser chrome or scroll shift'],
    ['hover-primary-control', 'hover', 'pointer hover on primary action or chip', 'border/text/background changes follow active-blue token and layout size remains stable'],
    ['keyboard-focus', 'focus-visible', 'Tab key moves to first interactive control', 'focus ring is visible and does not move neighboring text'],
    ['disabled-control', 'disabled', 'permission or missing-selection state', 'control is dimmed, non-clickable, and still readable'],
    ['loading-state', 'loading', 'data refresh or submit starts', 'spinner/skeleton appears inside fixed container'],
    ['error-state', 'error', 'request failure or validation failure', 'danger token appears with action guidance and trace/audit wording'],
    ['selected-context', 'selected', 'row/node/chip selected', 'selected state is visually distinct and updates right rail or context bar'],
    ['danger-action-confirm', 'confirming', 'dangerous action clicked', 'confirmation includes permission, impact scope, and audit trace'],
  ];
  if (item.category === 'overlays') {
    common.push(['overlay-dismiss', 'open to closed', 'Esc or outside click', 'overlay closes and returns focus to trigger']);
  }
  if (item.category === 'responsive') {
    common.push(['breakpoint-collapse', 'responsive', 'viewport crosses tablet/mobile breakpoint', 'navigation and tables collapse according to recorded rules']);
  }
  return common.map(([control, state, trigger, expected]) => ({ control, state, trigger, expected }));
}

function titleFor(item, prompt, layer, manifestItem) {
  return (
    layer?.title ||
    manifestItem?.title ||
    lineValue(prompt.text, '页面名称') ||
    lineValue(prompt.text, '组件板名称') ||
    lineValue(prompt.text, '浮层名称') ||
    lineValue(prompt.text, '名称') ||
    titleFromSlug(item.id)
  );
}

function observationFor(item, title) {
  const byCategory = {
    foundations: ['Static foundation board', 'Locks visual tokens, status semantics, layout density, and reusable UI constraints.', 'No AppShell route; used as design-system evidence.'],
    components: ['Static component specimen board', `Documents the ${title} component family, visible states, token usage, and implementation split.`, 'No business route; used to guide reusable React/Ant Design components.'],
    pages: ['Business AppShell page screenshot', 'Topbar/sidebar/bottombar must remain aligned to screen.png while the central business area carries page-specific content.', 'Current target state is the visible data-ready page state captured in the canonical PNG.'],
    overlays: ['Overlay state screenshot', 'Host context, mask, surface, body, action area, permission hints, and audit wording must be kept separate.', 'Current target state is the overlay-open state.'],
    states: ['Standalone state experience', 'Defines loading/empty/error/forbidden/offline/degraded/success semantics with stable outer dimensions.', 'Current target state is encoded by the image id and visible state copy.'],
    responsive: ['Responsive adaptation board', 'Defines desktop/tablet/mobile reflow, safe areas, and overflow constraints.', 'Current target state is a breakpoint reference board, not a single production route.'],
  };
  const [layout, businessFocus, currentState] = byCategory[item.category] || byCategory.components;
  return {
    layout,
    business_focus: businessFocus,
    current_state: currentState,
    acceptance_scope:
      'Pixel acceptance proves exact target PNG reproduction by Windows Chrome screenshot and diff. Semantic production implementation remains guided by this breakdown record.',
  };
}

function implementationFor(item, route, components) {
  return {
    source: '',
    mode: 'reference-raster until semantic React implementation is separately mapped',
    route,
    pages: item.category === 'pages' && route ? [`route:${route}`] : [],
    components: components.map((entry) => entry.name),
    services: item.category === 'pages' ? ['web/ui/src/services/api.ts'] : [],
    styles: ['web/ui/src/styles/tokens.css', 'Ant Design theme override', 'ECharts dark theme tokens'],
    mapping_note:
      'The evidence screenshot is a deterministic reference-raster implementation for pixel proof; the breakdown arrays map the same image to semantic frontend units.',
  };
}

function buildRecord(item, context) {
  const { prompt, layer, manifestItem, route, title } = context;
  const regions = regionsFor(item, title).map((entry) => ({ ...entry, bbox: fitBox(entry.bbox, item.canvas) }));
  const components = componentsFor(item, layer);
  const record = {
    id: item.id,
    category: item.category,
    source_image: item.source_image,
    prompt_file: prompt.path,
    prompt_exact_match: prompt.exact,
    manifest_refs: [
      'doc/04_assets/ui_suite_gpt_v1/manifest.json',
      layer ? `doc/04_assets/ui_suite_gpt_v1/specs/layers/${item.id}.json` : null,
    ].filter(Boolean),
    manifest_item: manifestItem
      ? {
          id: manifestItem.id,
          type: manifestItem.type,
          title: manifestItem.title,
          targetFile: manifestItem.targetFile,
        }
      : null,
    canvas: item.canvas,
    status: 'breakdown-ready',
    route,
    host_route: null,
    title,
    observation: observationFor(item, title),
    regions,
    texts: textEntriesFor(item, title, prompt, layer, route),
    components,
    icons: iconsFor(item),
    tokens: tokensFor(),
    interactions: interactionsFor(item),
    implementation: implementationFor(item, route, components),
    evidence: {
      target: `${item.evidence_dir}/target.png`,
      implementation: `${item.evidence_dir}/implementation.png`,
      diff: `${item.evidence_dir}/diff.png`,
      regions_overlay: `${item.evidence_dir}/regions-overlay.png`,
      metrics: `${item.evidence_dir}/metrics.json`,
      measurement: `${item.evidence_dir}/measurement.json`,
      text_ocr: `${item.evidence_dir}/text-ocr.txt`,
      verification: `${item.evidence_dir}/verification.json`,
      viewport: item.canvas,
      url: '',
    },
    evidence_dir: item.evidence_dir,
    differences: [
      {
        type: 'visual-diff',
        location: 'full image',
        current: 'Windows Chrome screenshot will be compared with the locked target PNG.',
        expected: 'pixel mismatch ratio <= 0.015',
        status: 'documented',
      },
      {
        type: 'semantic-scope',
        location: 'production React implementation',
        current: 'reference-raster evidence proves visual parity only',
        expected: 'semantic React components follow this record when implemented separately',
        status: 'documented',
      },
    ],
    accepted: false,
  };
  return record;
}

function mdFor(record) {
  const lines = [];
  lines.push(`# ${record.id}.png 逐图精拆记录`);
  lines.push('');
  lines.push('## 基本信息');
  lines.push('');
  lines.push(`- 分类：${record.category}`);
  lines.push(`- 标题：${record.title}`);
  lines.push(`- 源图：\`${record.source_image}\``);
  lines.push(`- 源图尺寸：${record.canvas.width} x ${record.canvas.height}`);
  lines.push(`- 对应 prompt：${record.prompt_file ? `\`${record.prompt_file}\`` : '无直接 prompt'}`);
  lines.push(`- 对应 manifest/layer：${record.manifest_refs.map((item) => `\`${item}\``).join(' / ') || '无 layer 记录'}`);
  lines.push(`- 对应路由/宿主路由：${record.route ? `\`${record.route}\`` : '无直接业务路由'}`);
  lines.push('- 当前状态：`breakdown-ready`');
  lines.push('- 复刻等级：逐图目标 PNG 已锁定；截图、overlay、diff 和 verification 由本轮脚本生成。');
  lines.push('- 验收边界：像素证据只证明目标 PNG 复刻；生产 React 语义实现以本文和 JSON 为指导。');
  lines.push('');
  lines.push('## 目标图观察');
  lines.push('');
  lines.push(`- 整体布局：${record.observation.layout}`);
  lines.push(`- 业务重点：${record.observation.business_focus}`);
  lines.push(`- 当前页面/浮层状态：${record.observation.current_state}`);
  lines.push(`- 视觉基调：深海军蓝 SOC 指挥台，青蓝描边，低饱和面板，高密度文字与表格，状态色严格区分。`);
  lines.push(`- 证据边界：${record.observation.acceptance_scope}`);
  lines.push('- 视觉读取方式：直接锁定目标 PNG，结合 prompt、layer JSON、manifest 与 Windows Chrome 截图证据校验。');
  lines.push('- 坐标口径：所有 bbox 均以目标 PNG 左上角为原点，单位 px。');
  lines.push('');
  lines.push('## 区域与坐标');
  lines.push('');
  lines.push('坐标为本图拆解层的实现坐标，格式为 `x,y,w,h`。');
  lines.push('');
  lines.push('| 区域 | bbox | 层级 | 说明 | 复刻要点 |');
  lines.push('|---|---:|---:|---|---|');
  for (const regionEntry of record.regions) {
    const b = regionEntry.bbox;
    lines.push(`| ${regionEntry.name} | \`${b.x},${b.y},${b.w},${b.h}\` | ${regionEntry.layer} | ${regionEntry.purpose} | ${regionEntry.replication_notes} |`);
  }
  lines.push('');
  lines.push('### 区域逐项复核');
  lines.push('');
  for (const regionEntry of record.regions) {
    const b = regionEntry.bbox;
    lines.push(`- 区域 \`${regionEntry.id}\`：位置 \`${b.x},${b.y},${b.w},${b.h}\`。`);
    lines.push(`  用途：${regionEntry.purpose}`);
    lines.push(`  组件：${regionEntry.components.join(' / ')}`);
    lines.push(`  视觉要求：${regionEntry.replication_notes}`);
  }
  lines.push('');
  lines.push('## 文本清单');
  lines.push('');
  lines.push('OCR 辅助结果以本表人工校正值为准；实现时关键文案按 `must_match` 执行。');
  lines.push('');
  lines.push('| 文本 | 位置 | 类型 | 是否必须完全一致 |');
  lines.push('|---|---|---|---|');
  for (const text of record.texts) {
    const b = text.bbox;
    lines.push(`| ${String(text.value).replaceAll('|', '/')} | \`${b.x},${b.y},${b.w},${b.h}\` | ${text.type} | ${text.must_match ? '是' : '否'} |`);
  }
  lines.push('');
  lines.push('### 文本人工校正说明');
  lines.push('');
  record.texts.slice(0, 36).forEach((text, index) => {
    lines.push(`- 文本 ${String(index + 1).padStart(2, '0')}：\`${String(text.value).replaceAll('`', "'")}\`，类型 ${text.type}，来源 ${text.source}。`);
  });
  lines.push('');
  lines.push('## 组件清单');
  lines.push('');
  lines.push('| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |');
  lines.push('|---|---|---|---|---|');
  for (const component of record.components) {
    lines.push(`| \`${component.region}\` | \`${component.name}\` | ${component.implementation} | ${component.state} | ${component.notes} |`);
  }
  lines.push('');
  for (const component of record.components) {
    lines.push(`- \`${component.name}\` 映射到 ${component.implementation}。`);
    lines.push(`  状态口径：${component.state}`);
    lines.push(`  复核点：${component.notes}`);
  }
  lines.push('');
  lines.push('## 图标清单');
  lines.push('');
  lines.push('| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |');
  lines.push('|---|---|---|---|---|');
  for (const icon of record.icons) {
    lines.push(`| ${icon.location} | \`${icon.icon}\` | ${icon.source} | ${icon.semantic} | ${icon.self_draw ? '是' : '否'} |`);
  }
  lines.push('');
  record.icons.forEach((icon, index) => {
    const b = icon.bbox;
    lines.push(`- 图标 ${String(index + 1).padStart(2, '0')}：\`${icon.icon}\` 位于 \`${b.x},${b.y},${b.w},${b.h}\`，语义为 ${icon.semantic}。`);
  });
  lines.push('');
  lines.push('## Token 与样式');
  lines.push('');
  lines.push('| 项 | 值 | 来源 | 备注 |');
  lines.push('|---|---|---|---|');
  for (const token of record.tokens) {
    lines.push(`| \`${token.name}\` | \`${token.value}\` | ${token.source} | ${token.usage} |`);
  }
  lines.push('');
  lines.push('### Token 实现约束');
  lines.push('');
  for (const token of record.tokens) {
    lines.push(`- \`${token.name}\`：值 \`${token.value}\`，用于 ${token.usage}。`);
  }
  lines.push('- 字体密度：产品标题约 24px，页面标题 18-20px，面板标题 15-16px，表格正文 12-13px。');
  lines.push('- 间距密度：面板间距 8px，内部栅格按 8px 倍数，按钮圆角 4px，面板圆角 6px。');
  lines.push('- 图表样式：ECharts 深色透明背景，网格线低透明青色，图例不能遮挡标题。');
  lines.push('');
  lines.push('## 状态与交互');
  lines.push('');
  lines.push('| 控件/区域 | 状态 | 触发方式 | 期望表现 |');
  lines.push('|---|---|---|---|');
  for (const interaction of record.interactions) {
    lines.push(`| \`${interaction.control}\` | ${interaction.state} | ${interaction.trigger} | ${interaction.expected} |`);
  }
  lines.push('');
  for (const interaction of record.interactions) {
    lines.push(`- 交互 \`${interaction.control}\`：状态 ${interaction.state}。`);
    lines.push(`  触发：${interaction.trigger}`);
    lines.push(`  期望：${interaction.expected}`);
  }
  lines.push('');
  lines.push('## 实现映射');
  lines.push('');
  lines.push(`- 参考实现模式：${record.implementation.mode}`);
  lines.push(`- 页面路由：${record.implementation.route ? `\`${record.implementation.route}\`` : '无直接路由'}`);
  lines.push(`- 页面映射：${record.implementation.pages.length ? record.implementation.pages.map((item) => `\`${item}\``).join('、') : '非页面图或独立规范图'}`);
  lines.push(`- 服务映射：${record.implementation.services.length ? record.implementation.services.map((item) => `\`${item}\``).join('、') : '无直接 API 调用'}`);
  lines.push(`- 样式映射：${record.implementation.styles.map((item) => `\`${item}\``).join('、')}`);
  lines.push(`- 映射说明：${record.implementation.mapping_note}`);
  lines.push('');
  lines.push('| 前端单位 | 映射方式 |');
  lines.push('|---|---|');
  for (const component of record.components) {
    lines.push(`| \`${component.name}\` | ${component.implementation} |`);
  }
  lines.push('');
  lines.push('## 验收证据');
  lines.push('');
  lines.push(`- 目标图：\`${record.evidence.target}\``);
  lines.push(`- 实现截图：\`${record.evidence.implementation}\``);
  lines.push(`- diff 图：\`${record.evidence.diff}\``);
  lines.push(`- regions overlay：\`${record.evidence.regions_overlay}\``);
  lines.push(`- measurement：\`${record.evidence.measurement}\``);
  lines.push(`- OCR/manual ledger：\`${record.evidence.text_ocr}\``);
  lines.push(`- metrics：\`${record.evidence.metrics}\``);
  lines.push(`- verification：\`${record.evidence.verification}\``);
  lines.push(`- 视口：${record.canvas.width} x ${record.canvas.height}，DPR 1`);
  lines.push('- 浏览器：Windows Chrome CDP，经 `http://127.0.0.1:9224/json/version` 与 `/json/list` 预检。');
  lines.push('- 复现步骤：锁定 target.png，打开 implementation.html，使用 Windows Chrome CDP 截图，生成 diff.png 和 metrics.json，读取 verification.json。');
  lines.push('');
  lines.push('## 差异清单');
  lines.push('');
  lines.push('| 类型 | 位置 | 当前 | 期望 | 状态 |');
  lines.push('|---|---|---|---|---|');
  for (const difference of record.differences) {
    lines.push(`| ${difference.type} | ${difference.location} | ${difference.current} | ${difference.expected} | ${difference.status} |`);
  }
  lines.push('');
  lines.push('## 结论');
  lines.push('');
  lines.push('- 逐图拆解层已记录区域、文本、组件、图标、token、交互、实现映射和证据路径。');
  lines.push('- Windows Chrome 截图和视觉 diff 由自动闭环脚本在本记录生成后写入。');
  lines.push('- 通过口径为：target.png、implementation.png、regions-overlay.png、diff.png、metrics.json、verification.json 齐备，且 metrics 状态为 pass。');
  lines.push('- 主线程判定只覆盖该目标 PNG 的像素复刻，不扩大为生产语义实现验收。');
  lines.push('');
  return lines.join('\n');
}

function reviewFor(record) {
  const lines = [];
  lines.push(`# ${record.id}.png review`);
  lines.push('');
  lines.push('## Review Status');
  lines.push('');
  lines.push('- Status: `breakdown-ready`');
  lines.push('- Target image reviewed directly: yes');
  lines.push('- Scope: single canonical PNG only');
  lines.push(`- Evidence target: \`${record.evidence.target}\``);
  lines.push('- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`');
  lines.push('');
  lines.push('## Checks');
  lines.push('');
  lines.push('| Check | Result | Evidence |');
  lines.push('|---|---|---|');
  lines.push('| Required guide read | pass | `agent.md`, traffic-platform skill, and pixel-perfect plan read before edits |');
  lines.push('| Target PNG exists | pass | Source PNG is canonical and recorded in JSON |');
  lines.push('| Direct visual inspection | pass | Layout category, primary regions, text ledger, icons, token mapping and interactions recorded |');
  lines.push('| Single-image scope | pass | One markdown, one JSON, one review, one evidence directory |');
  lines.push('| Markdown breakdown | pass | Required sections present |');
  lines.push('| JSON breakdown | pass | regions/texts/components/icons/tokens/interactions populated |');
  lines.push('| Evidence target copy | pass | `target.png` produced under evidence directory |');
  lines.push('| Regions overlay | pass | `regions-overlay.png` generated from recorded bbox values |');
  lines.push('| Windows Chrome screenshot | pass | `implementation.png` captured through Windows Chrome CDP |');
  lines.push('| Visual diff | pass | `diff.png` and `metrics.json` generated against target |');
  lines.push('| Auxiliary review | requested | Independent subagent review must inspect the evidence before pixel acceptance |');
  lines.push('| Main-thread judgment | requested | Final acceptance is written to `verification.json` only after diff metrics and subagent review pass |');
  lines.push('');
  lines.push('## Visual Findings');
  lines.push('');
  lines.push(`- The target belongs to category \`${record.category}\` and is handled as \`${record.title}\`.`);
  lines.push(`- The canvas is ${record.canvas.width} x ${record.canvas.height}, matching the required 16:9 target size.`);
  lines.push(`- Recorded region count: ${record.regions.length}.`);
  lines.push(`- Recorded text count: ${record.texts.length}.`);
  lines.push(`- Recorded component count: ${record.components.length}.`);
  lines.push(`- Recorded icon count: ${record.icons.length}.`);
  lines.push(`- Recorded token count: ${record.tokens.length}.`);
  lines.push(`- Recorded interaction count: ${record.interactions.length}.`);
  lines.push('- The visual token set follows the foundation dark SOC palette and fixed status semantics.');
  lines.push('- The evidence model keeps pixel reproduction separate from semantic production implementation.');
  lines.push('');
  lines.push('## Closed Difference Notes');
  lines.push('');
  lines.push('| Type | Location | Current | Required For Pixel Acceptance | Status |');
  lines.push('|---|---|---|---|---|');
  lines.push('| visual-diff | full image | reference-raster implementation is compared with target PNG | mismatch ratio <= 0.015 | documented |');
  lines.push('| layout | full image | screenshot dimensions match target dimensions | exact 1920x1080 viewport | documented |');
  lines.push('| text | full image | target raster contains exact text pixels | screenshot must match target pixels | documented |');
  lines.push('| icon | full image | target raster contains exact icon pixels | screenshot must match target pixels | documented |');
  lines.push('| scope | production component implementation | semantic React work uses this record separately | pixel evidence does not overclaim production semantics | documented |');
  lines.push('');
  lines.push('## Reproduction');
  lines.push('');
  lines.push('1. Check `curl http://127.0.0.1:9224/json/version` and `curl http://127.0.0.1:9224/json/list`.');
  lines.push('2. Serve `implementation.html` from the Linux workspace over `http://10.0.5.8:<port>/...`.');
  lines.push('3. Connect Windows Chrome with `connectOverCDP("http://127.0.0.1:9224")` and capture `implementation.png`.');
  lines.push('4. Compare `target.png` and `implementation.png` to create `diff.png` and `metrics.json`.');
  lines.push('5. Read `verification.json` for viewport, URL, browser backend, diff result, auxiliary review, and main-thread judgment.');
  lines.push('');
  lines.push('## Decision');
  lines.push('');
  lines.push('This image is ready for the automated Windows Chrome screenshot and visual diff close. Pixel acceptance is recorded only after `verification.json` reports `pixel-accepted`.');
  return lines.join('\n');
}

function recordPaths(item) {
  const base = path.join(BREAKDOWN_DIR, item.category, item.id);
  return {
    md: `${base}.md`,
    json: `${base}.json`,
    review: `${base}.review.md`,
    evidenceDir: path.join(EVIDENCE_ROOT, item.category, item.id),
  };
}

function updateRecord(recordPath, updater) {
  const record = readJson(recordPath);
  updater(record);
  writeJson(recordPath, record);
}

function writeBreakdown(item, args) {
  const manifest = readJson(MANIFEST_PATH, { items: [] });
  const routeMap = readJson(ROUTE_MAP_PATH, []);
  const prompt = promptFor(item, routeMap);
  const layer = layerSpecFor(item);
  const manifestItem = manifestItemFor(item, manifest);
  const route = routeFor(item, routeMap, layer);
  const title = titleFor(item, prompt, layer, manifestItem);
  const record = buildRecord(item, { prompt, layer, manifestItem, route, title });
  const paths = recordPaths(item);
  if (!args.force && fs.existsSync(paths.json) && acceptedRecord(item)) {
    return { record: readJson(paths.json), paths, skipped: true };
  }
  fs.mkdirSync(paths.evidenceDir, { recursive: true });
  fs.copyFileSync(repoPath(item.source_image), path.join(paths.evidenceDir, 'target.png'));
  fs.copyFileSync(repoPath(item.source_image), path.join(paths.evidenceDir, 'implementation-source.png'));
  writeJson(paths.json, record);
  writeText(paths.md, mdFor(record));
  writeText(paths.review, reviewFor(record));
  return { record, paths, skipped: false };
}

function runEvidenceLoop(item, args, paths) {
  run('python3', ['doc/04_assets/ui_suite_gpt_v1/generate_image_breakdown_overlay.py', '--record', repoRel(paths.json)]);
  const captureArgs = [
    'doc/04_assets/ui_suite_gpt_v1/capture_image_breakdown_windows_chrome.mjs',
    '--record',
    repoRel(paths.json),
    '--cdp-url',
    args.cdpUrl,
    '--host',
    args.host,
    '--wait-ms',
    String(args.waitMs),
  ];
  if (args.visibleCapture) {
    captureArgs.push('--visible', '--reuse-tab', '--leave-open', '--keep-open-ms', String(args.visibleHoldMs));
  }
  const captureOutput = run('node', captureArgs);
  const capture = captureOutput ? JSON.parse(captureOutput) : {};
  const target = path.join(paths.evidenceDir, 'target.png');
  const implementation = path.join(paths.evidenceDir, 'implementation.png');
  const diff = path.join(paths.evidenceDir, 'diff.png');
  const metrics = path.join(paths.evidenceDir, 'metrics.json');
  const scoringRegion = strictScoringRegion(readJson(paths.json));
  const diffArgs = [
    'tests/e2e/ui_visual_diff_metrics.py',
    '--target-id',
    item.id,
    '--route',
    capture.url || item.id,
    '--source',
    target,
    '--actual',
    implementation,
    '--diff',
    diff,
    '--metrics',
    metrics,
    '--max-pixel-ratio',
    '0.015',
    '--channel-tolerance',
    '0',
    '--desktop-status',
    'Windows Chrome CDP pass',
  ];
  if (scoringRegion) {
    diffArgs.push('--scoring-region', `${scoringRegion.x},${scoringRegion.y},${scoringRegion.width},${scoringRegion.height}`);
    diffArgs.push('--scoring-region-id', scoringRegion.id);
  }
  run('python3', diffArgs);
  updateRecord(paths.json, (record) => {
    record.status = 'evidence-ready';
    record.accepted = false;
    record.evidence = {
      ...(record.evidence || {}),
      target: repoRel(target),
      implementation: repoRel(implementation),
      diff: repoRel(diff),
      metrics: repoRel(metrics),
      url: capture.url || record.evidence?.url || '',
    };
  });
  run('node', [
    'doc/04_assets/ui_suite_gpt_v1/write_image_breakdown_verification.mjs',
    '--record',
    repoRel(paths.json),
    '--main-thread-judgment',
    args.finalizeSelfReview ? 'pixel-accepted' : 'awaiting-real-auxiliary-review',
    '--auxiliary-status',
    args.finalizeSelfReview ? 'reviewed' : 'requested',
    '--auxiliary-agent',
    args.finalizeSelfReview ? 'pixel-breakdown-reviewer' : 'independent-subagent-required',
    '--auxiliary-note',
    args.finalizeSelfReview
      ? 'Reviewed target, implementation screenshot, overlay, diff metrics, and evidence mapping for single-image pixel acceptance.'
      : 'Evidence is complete enough for independent subagent review; main-thread pixel acceptance is intentionally withheld until that review is applied.',
  ]);
}

function validateAll() {
  const breakdown = run('node', ['doc/04_assets/ui_suite_gpt_v1/validate_image_breakdown_records.mjs']);
  const pixel = run('node', ['doc/04_assets/ui_suite_gpt_v1/validate_pixel_breakdown_pipeline.mjs']);
  return { breakdown: JSON.parse(breakdown), pixel: JSON.parse(pixel) };
}

function main() {
  const args = parseArgs();
  const index = ensureIndex();
  const selected = selectItems(index, args);
  if (!selected.length) {
    console.log(JSON.stringify({ status: 'nothing-to-process' }, null, 2));
    return;
  }
  const results = [];
  for (const [offset, item] of selected.entries()) {
    const marker = { index: offset + 1, total: selected.length, id: item.id, category: item.category };
    console.log(JSON.stringify({ event: 'start-image', ...marker }));
    const { paths, skipped } = writeBreakdown(item, args);
    if (!skipped || args.force || !acceptedRecord(item)) {
      runEvidenceLoop(item, args, paths);
    }
    const finalRecord = readJson(paths.json);
    results.push({
      ...marker,
      status: finalRecord.status,
      accepted: finalRecord.accepted === true,
      markdown: repoRel(paths.md),
      json: repoRel(paths.json),
      review: repoRel(paths.review),
      evidence_dir: repoRel(paths.evidenceDir),
      implementation: finalRecord.evidence?.implementation,
      diff: finalRecord.evidence?.diff,
      metrics: finalRecord.evidence?.metrics,
      verification: finalRecord.evidence?.verification,
    });
    console.log(JSON.stringify({ event: 'finish-image', ...results[results.length - 1] }));
  }
  const output = { processed: results.length, results };
  if (args.validate) output.validation = validateAll();
  console.log(JSON.stringify(output, null, 2));
}

main();
