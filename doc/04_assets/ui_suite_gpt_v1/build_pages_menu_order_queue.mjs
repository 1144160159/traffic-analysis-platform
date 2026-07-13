#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const SPEC_DIR = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs');
const INDEX_PATH = path.join(SPEC_DIR, 'pixel-perfect-breakdown-index.json');
const OUT_JSON = path.join(SPEC_DIR, 'pages-menu-order-queue.json');
const OUT_MD = path.join(SPEC_DIR, 'PAGES_MENU_ORDER_QUEUE.md');

const menuPlan = [
  {
    group: 'auth',
    title: '登录入口',
    entries: [
      { id: 'login', route: '/login', title: '登录' },
    ],
  },
  {
    group: 'overview',
    title: '综合态势',
    entries: [
      { id: 'dashboard', route: '/dashboard', title: '仪表盘' },
      { id: 'screen', route: '/screen', title: '态势大屏' },
      {
        id: 'topics',
        route: '/topics',
        title: '专题面板',
        children: ['topics-encrypted-tunnel', 'topics-data-exfiltration', 'topics-apt-campaign'],
      },
    ],
  },
  {
    group: 'collection-monitoring',
    title: '采集监测',
    entries: [
      { id: 'probes', route: '/probes', title: '探针管理' },
      {
        id: 'data-quality',
        route: '/data-quality',
        title: '数据质量',
        children: [
          'data-quality-topic-health',
          'data-quality-flink-quality',
          'data-quality-field-quality',
          'data-quality-storage-quality',
          'data-quality-replay-reconcile',
          'data-quality-report',
          'data-quality-settings',
        ],
      },
    ],
  },
  {
    group: 'threat-analysis',
    title: '威胁分析',
    entries: [
      {
        id: 'alerts',
        route: '/alerts',
        title: '告警中心',
        children: [
          'alert-detail',
          'alert-detail-evidence-files',
          'alert-detail-evidence-graph-path',
          'alert-detail-evidence-logs',
          'alert-detail-evidence-pcap',
          'alert-detail-evidence-session',
        ],
      },
      {
        id: 'campaigns',
        route: '/campaigns',
        title: '战役列表',
        children: [
          'campaign-detail',
          'campaign-detail-impact-account',
          'campaign-detail-impact-business-system',
          'campaign-detail-impact-campus',
          'campaign-detail-impact-department',
          'campaign-detail-impact-service',
        ],
      },
      { id: 'attack-chains', route: '/attack-chains', title: '攻击链分析' },
      {
        id: 'encrypted-traffic',
        route: '/encrypted-traffic',
        title: '加密流量',
        children: [
          'encrypted-traffic-fingerprint',
          'encrypted-traffic-tunnel-detection',
          'encrypted-traffic-egress-profile',
          'encrypted-traffic-evidence-center',
        ],
      },
      { id: 'forensics', route: '/forensics', title: '取证分析' },
    ],
  },
  {
    group: 'asset-graph',
    title: '资产图谱',
    entries: [
      {
        id: 'assets',
        route: '/assets',
        title: '资产台账',
        children: [
          'assets-network-device',
          'assets-server',
          'assets-unknown',
          'assets-business-system',
          'assets-detail-basic',
          'assets-detail-network-interface',
          'assets-detail-open-services',
          'assets-detail-ownership',
          'assets-detail-history',
        ],
      },
      {
        id: 'graph',
        route: '/graph',
        title: '实体图谱',
        children: ['graph-account-access-path', 'graph-attack-path', 'graph-communication-path'],
      },
      { id: 'fusion', route: '/fusion', title: '数据融合' },
      {
        id: 'baselines',
        route: '/baselines',
        title: '行为基准',
        children: ['baselines-account', 'baselines-port', 'baselines-protocol', 'baselines-time-window'],
      },
    ],
  },
  {
    group: 'detection-ops',
    title: '检测运营',
    entries: [
      {
        id: 'rules',
        route: '/rules',
        title: '规则管理',
        children: [
          'rules-editor-dependencies',
          'rules-editor-test-validation',
          'rules-sample-logs',
          'rules-sample-session',
        ],
      },
      { id: 'deployments', route: '/deployments', title: '部署管理' },
      {
        id: 'models',
        route: '/models',
        title: '模型管理',
        children: [
          'models-activation-audit-gate',
          'models-feature-anomaly-explanation',
          'models-feature-rule-contribution',
          'models-feature-sample-examples',
        ],
      },
      { id: 'mlops', route: '/mlops', title: 'MLOps 编排' },
      { id: 'playbooks', route: '/playbooks', title: 'SOAR 剧本' },
      {
        id: 'whitelist',
        route: '/whitelist',
        title: '白名单',
        children: [
          'whitelist-condition-account',
          'whitelist-condition-asset',
          'whitelist-condition-ip',
          'whitelist-condition-model',
          'whitelist-condition-rule',
          'whitelist-expiry-expired-unhandled',
          'whitelist-expiry-long-lived',
          'whitelist-expiry-unassigned-owner',
        ],
      },
    ],
  },
  {
    group: 'audit-config',
    title: '审计配置',
    entries: [
      { id: 'compliance', route: '/compliance', title: '合规审计' },
      {
        id: 'audit-log',
        route: '/audit-log',
        title: '审计日志',
        children: ['audit-log-operation-context', 'audit-log-related-chain'],
      },
      { id: 'notifications', route: '/notifications', title: '通知配置' },
      { id: 'settings', route: '/settings', title: '系统设置' },
    ],
  },
  {
    group: 'fallback',
    title: '兜底页面',
    entries: [
      { id: 'not-found', route: '/__codex_visual_not_found__', title: '404' },
    ],
  },
];

function repoRel(file) {
  return path.relative(ROOT, file).replaceAll(path.sep, '/');
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function appendIfPresent({ out, itemById, seen, id, parent, group, groupTitle, route, title, kind }) {
  const item = itemById.get(id);
  if (!item || seen.has(id)) return false;
  seen.add(id);
  out.push({
    order: out.length + 1,
    id,
    category: item.category,
    group,
    group_title: groupTitle,
    parent_id: parent || '',
    title: title || id,
    route: route || '',
    kind,
    source_image: item.source_image,
    breakdown: item.breakdown,
    json: item.json,
    review: item.review,
    evidence_dir: item.evidence_dir,
  });
  return true;
}

function buildQueue(index) {
  const pageItems = index.items.filter((item) => item.category === 'pages');
  const itemById = new Map(pageItems.map((item) => [item.id, item]));
  const seen = new Set();
  const ordered = [];
  const missingPlanned = [];

  for (const section of menuPlan) {
    for (const entry of section.entries) {
      const parentPresent = appendIfPresent({
        out: ordered,
        itemById,
        seen,
        id: entry.id,
        parent: '',
        group: section.group,
        groupTitle: section.title,
        route: entry.route,
        title: entry.title,
        kind: 'menu-route',
      });
      if (!parentPresent && !entry.children?.length) missingPlanned.push(entry.id);

      for (const child of entry.children || []) {
        const childPresent = appendIfPresent({
          out: ordered,
          itemById,
          seen,
          id: child,
          parent: entry.id,
          group: section.group,
          groupTitle: section.title,
          route: entry.route,
          title: child,
          kind: 'menu-state',
        });
        if (!childPresent) missingPlanned.push(child);
      }
    }
  }

  const leftovers = pageItems
    .filter((item) => !seen.has(item.id))
    .sort((left, right) => left.id.localeCompare(right.id));

  for (const item of leftovers) {
    appendIfPresent({
      out: ordered,
      itemById,
      seen,
      id: item.id,
      parent: '',
      group: 'unmapped',
      groupTitle: '未映射页面',
      route: '',
      title: item.id,
      kind: 'unmapped-index-page',
    });
  }

  return { ordered, missingPlanned, leftovers: leftovers.map((item) => item.id), totalPages: pageItems.length };
}

function markdown(report) {
  const lines = [];
  lines.push('# Pages 菜单顺序闭环队列');
  lines.push('');
  lines.push('本队列用于 pages 分类重新闭环：先登录页，再按应用菜单顺序处理主页面和挂靠状态页。`pixel-perfect-breakdown-index.json` 只作为全集来源，不作为执行顺序。');
  lines.push('');
  lines.push(`- pages 总数：${report.total_pages}`);
  lines.push(`- 队列总数：${report.items.length}`);
  lines.push(`- 未映射队列项：${report.unmapped_items.length}`);
  lines.push('');
  lines.push('| 顺序 | 分组 | 图片 ID | 父页面 | 类型 | 路由 |');
  lines.push('|---:|---|---|---|---|---|');
  for (const item of report.items) {
    lines.push(`| ${item.order} | ${item.group_title} | \`${item.id}\` | ${item.parent_id ? `\`${item.parent_id}\`` : '-'} | \`${item.kind}\` | \`${item.route || '-'}\` |`);
  }
  if (report.unmapped_items.length) {
    lines.push('');
    lines.push('## 未映射项');
    lines.push('');
    for (const id of report.unmapped_items) lines.push(`- \`${id}\``);
  }
  return lines.join('\n');
}

function main() {
  const index = readJson(INDEX_PATH);
  const queue = buildQueue(index);
  const report = {
    generated_by: repoRel(__filename),
    source_index: repoRel(INDEX_PATH),
    ordering_rule: 'login first, then routeManifest menu order with child visual states attached to their parent menu route',
    total_pages: queue.totalPages,
    items: queue.ordered,
    missing_planned_items: queue.missingPlanned,
    unmapped_items: queue.leftovers,
  };
  fs.writeFileSync(OUT_JSON, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(OUT_MD, `${markdown(report)}\n`);
  console.log(JSON.stringify({
    total_pages: report.total_pages,
    queued: report.items.length,
    first: report.items.slice(0, 8).map((item) => item.id),
    unmapped_items: report.unmapped_items,
    outputs: [repoRel(OUT_JSON), repoRel(OUT_MD)],
  }, null, 2));
}

main();
