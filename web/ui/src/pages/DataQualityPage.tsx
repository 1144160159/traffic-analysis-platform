import {
  ApiOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  CloseOutlined,
  DatabaseOutlined,
  DownloadOutlined,
  FileSearchOutlined,
  FileDoneOutlined,
  FullscreenOutlined,
  FieldTimeOutlined,
  PrinterOutlined,
  ReloadOutlined,
  LeftOutlined,
  RightOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  SyncOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Form, Input, Modal, Select, Space, Switch, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, MouseEvent, ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { DataQualityDonutChart, DataQualityFieldTrendChart, DataQualityKpiSparklineChart, DataQualityTrendChart } from '@/components/charts';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot, type DataQualityTimeRange } from '@/services/api';
import {
  buildDLQReplayDryRunRequest,
  requestDLQFallbackReplay,
  type DLQReplayFallbackRequest,
  type DLQReplayFallbackResult,
} from '@/services/dlqReplayApi';
import type { DataQualityVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';
import { getPageActionPlan, type ActionEndpointPlan } from '@/services/pageApiPlans';

const fieldRows = [
  ['src_ip', '99.98%', '99.95%', '0.02%', '0.03%', '98.76%', '正常', '查看', 'ok'],
  ['dst_ip', '99.97%', '99.93%', '0.03%', '0.04%', '98.71%', '正常', '查看', 'ok'],
  ['src_port', '99.90%', '99.85%', '0.10%', '0.05%', '96.32%', '正常', '查看', 'ok'],
  ['dst_port', '99.91%', '99.83%', '0.09%', '0.08%', '95.41%', '正常', '查看', 'ok'],
  ['protocol', '99.98%', '99.97%', '0.02%', '0.01%', '2.31%', '正常', '查看', 'ok'],
  ['community_id', '98.76%', '98.62%', '1.24%', '0.14%', '97.21%', '中等', '查看', 'warn'],
  ['tenant', '99.12%', '98.45%', '0.88%', '0.67%', '87.34%', '中等', '查看', 'warn'],
  ['asset_id', '95.84%', '94.11%', '4.16%', '1.73%', '85.22%', '高危', '查看', 'risk'],
];

const anomalyRows = [
  ['DLQ 异常增长', 'dlq_topic', '12,845 条', '03:41:12', 'risk'],
  ['消费延迟提升', 'threat_alerts', '2.3 s', '03:40:55', 'warn'],
  ['分区倾斜严重', 'flow_original（分区 12）', '2.15', '03:40:33', 'warn'],
  ['字段缺失率升高', 'asset_events.asset_id', '4.12%', '03:39:18', 'info'],
  ['延迟数据率升高', 'flow_enriched', '0.67%', '03:37:42', 'info'],
];

const locateRows = [
  ['定位 Flink 作业', 'feature-job', 'Backpressure...'],
  ['定位 Kafka Topic', 'flow_original', 'Lag 10.9K'],
  ['定位探针/采集端', 'probe-dc-01', 'packet loss 0.02%'],
  ['查看 DLQ 详情', 'dlq_topic', '12,845 条'],
];

const consumerRows = [
  ['session-job', '1.23M', '2', '15:30:12', '健康'],
  ['feature-job', '0.86M', '1', '15:30:08', '健康'],
  ['rule-job', '0.54M', '3', '15:29:58', '健康'],
  ['pcap-index-job', '0.43M', '0', '15:30:15', '健康'],
  ['behavior-job', '0.21M', '1', '15:29:55', '健康'],
];

const replayRows = [
  ['DLQ-20260626-001', 'dlq.v1', '12,845', '待重放', '按 offset 分批'],
  ['DLQ-20260626-002', 'threat_alerts', '2,381', '校验中', '先验 schema'],
  ['RC-15M-030', 'flow_original', '7.82M', '已对账', '差异 0.4%'],
  ['RC-24H-114', 'session.events.v1', '2.13M', '已闭环', '幂等通过'],
];

const dlqReplayActionPlan = getPageActionPlan('data-quality', 'dlq-fallback-replay');
const dataQualityContextActionPlan = getPageActionPlan('data-quality', 'data-quality-context-action');

const actionEndpointLabel = (action: ActionEndpointPlan | undefined) =>
  action ? `${action.method} ${action.endpoint}` : 'POST /v1/dlq/replay/fallback';

const actionScopeLabel = (action: ActionEndpointPlan | undefined) =>
  (action?.acceptedScopes ?? action?.requiredScopes ?? ['dlq:replay']).join(' / ');

const dlqSampleDrawerWidth = 'min(720px, calc(var(--taf-window-inner-width, 100dvw) - 40px))';
const dlqReplayModalWidth = 'min(760px, calc(var(--taf-window-inner-width, 100dvw) - 64px))';
const fieldDetailDrawerWidth = 'min(480px, calc(var(--taf-window-inner-width, 100dvw) - 40px))';

const dlqReplayContractRows = [
  ['接口', actionEndpointLabel(dlqReplayActionPlan), 'APISIX /api/v1/dlq*', '已登记'],
  ['权限', actionScopeLabel(dlqReplayActionPlan), 'token scope', '强制'],
  ['预检', 'dry_run=true', '不执行文件回放', '默认'],
  ['幂等', 'idempotency_key', 'Redis 24h TTL', '强制'],
  ['审计', dlqReplayActionPlan?.auditEvent ?? 'dlq_replay_approved', 'audit_trail', '强制'],
];

const dataQualityOverlays: OverlayContract[] = [
  {
    id: 'modal-data-replay-task',
    title: '数据重放任务',
    kind: 'Modal',
    actionLabel: '数据重放',
    description: '按 Topic、offset、时间窗和 schema 校验结果创建重放任务；fallback DLQ 默认先走 dry-run 预检。',
    impact: '影响 Kafka/Flink/ClickHouse 对账链路，需限制批次、审批人和幂等策略。',
    audit: `${dlqReplayActionPlan?.auditEvent ?? 'dlq_replay_approved'} 写入 replay audit trail。`,
    danger: true,
    fields: [
      ['接口', actionEndpointLabel(dlqReplayActionPlan)],
      ['权限', actionScopeLabel(dlqReplayActionPlan)],
      ['默认模式', 'dry_run=true 预检'],
      ['幂等策略', 'idempotency_key Redis-backed 24h TTL'],
      ['审批约束', 'approved_by 必须不同于 requested_by'],
    ],
  },
];

const flinkJobs = [
  ['session-job', '28s', '1.6s', '0.08', '99.2%', 'ok'],
  ['feature-job', '31s', '2.1s', '0.16', '94.6%', 'warn'],
  ['rule-job', '26s', '1.2s', '0.04', '98.7%', 'ok'],
  ['pcap-index-job', '34s', '1.9s', '0.11', '96.3%', 'ok'],
  ['behavior-job', '42s', '3.8s', '0.24', '91.8%', 'warn'],
  ['alert-generator', '24s', '1.1s', '0.03', '99.1%', 'ok'],
];

const reportRows = [
  ['质量基线', '92.6%', '已归档', 'S3://quality/baseline'],
  ['Kafka Topic', '19 项', '已采样', 'offset + lag'],
  ['Flink Checkpoint', '94.6%', '已校验', 'latest checkpoint'],
  ['字段矩阵', '33 项', '待复核', 'missing + duplicate'],
  ['存储写入', '96.6%', '已归档', 'CH/OS/Nebula/MinIO'],
];

const reportMetricLabels = ['日报评分', '验收通过率', '异常归因', '待补证据', '已导出', 'SLA 达成'];

const reportMetrics: PageSnapshot['metrics'] = [
  { label: '日报评分', value: '92/100', delta: '较昨日 ↑ 3 分', status: 'ok' },
  { label: '验收通过率', value: '98.7%', delta: '较昨日 ↑ 0.6%', status: 'ok' },
  { label: '异常归因', value: '14 个', delta: '较昨日 ↓ 2', status: 'risk' },
  { label: '待补证据', value: '5 项', delta: '较昨日 ↓ 1', status: 'warn' },
  { label: '已导出', value: '23 份', delta: '较昨日 ↑ 6', status: 'info' },
  { label: 'SLA 达成', value: '97.8%', delta: '较昨日 ↑ 0.9%', status: 'ok' },
];

const reportChapterRows = [
  ['1', '总览', '完成 100%', 'ok'],
  ['2', 'Topic 健康', '完成 100%', 'ok'],
  ['3', 'Flink 质量', '完成 100%', 'ok'],
  ['4', '字段质量', '完成 100%', 'ok'],
  ['5', '存储质量', '完成 100%', 'ok'],
  ['6', '重放对账', '完成 95%', 'warn'],
  ['7', '验收结论', '完成 100%', 'ok'],
];

const reportAnomalyRows = [
  ['DLQ 增长', 'behavior-job 下游写入超时导致重试失败进入 DLQ', 'sec_analyst', '12,845 条 (69.7%)', '修复中'],
  ['字段缺失', 'tenant / asset_id 映射缺失影响字段完整度', 'data_analyst', '3,214 条 (17.4%)', '修复中'],
  ['存储写入延迟', 'ClickHouse Distributed 队列积压导致写入慢于常态', 'ops_engineer', '全量数据 (100%)', '处理中'],
  ['Flink backpressure', 'feature-job sink 写入校验上游并发超限', 'flink_owner', '6 个作业 (52%)', '已修复'],
  ['Topic 分区倾斜', 'flow_original p23 分区写入流量过高，倾斜明显', 'kafka_owner', 'p23 分区 (4.1%)', '处理中'],
];

const reportExportRows = [
  ['2025-06-26 15:12', 'PDF', 'sec_analyst', '已审批', 'security_team', '查看'],
  ['2025-06-26 10:25', 'JSON', 'data_analyst', '已审批', 'data_team', '查看'],
  ['2025-06-26 08:10', 'CSV', 'ops_engineer', '已审批', 'ops_team', '查看'],
  ['2025-06-25 23:30', 'PDF', 'sec_analyst', '已审批', 'management', '补齐'],
  ['2025-06-25 16:05', 'JSON', 'flink_owner', '待审批', 'flink_team', '复核'],
  ['2025-06-25 16:02', 'PDF', 'data_analyst', '已驳回', 'data_team', '查看'],
];

const reportRailAnomalies = [
  ['高风险异常', '2', 'risk'],
  ['中风险异常', '6', 'warn'],
  ['低风险异常', '6', 'ok'],
  ['已处置异常', '14', 'info'],
];

const reportRailLocateRows = [
  ['定位异常归因', ''],
  ['定位缺失证据', ''],
  ['定位 SLA 失败项', ''],
  ['定位导出失败', ''],
];

const reportRailRepairRows = [
  ['处理 DLQ 增长', ''],
  ['修复字段缺失', ''],
  ['优化存储延迟', ''],
  ['缓解 backpressure', ''],
  ['优化分区倾斜', ''],
];

const settingsRows = [
  ['消费延迟 P95', '3s', '告警', 'Topic 健康'],
  ['字段缺失率', '2%', '阻断', '字段质量'],
  ['Checkpoint 间隔', '60s', '告警', 'Flink 质量'],
  ['DLQ 重放批量', '5000', '审批', '重放对账'],
  ['存储写入成功率', '99%', '告警', '存储质量'],
];

const settingsMetricLabels = ['启用规则', '阈值组', '告警策略', '报告周期', '待审批变更', '最近保存', '审计完整'];

const settingsMetrics: PageSnapshot['metrics'] = [
  { label: '启用规则', value: '42 条', delta: '较昨日 ↑ 3', status: 'ok' },
  { label: '阈值组', value: '8 组', delta: '较昨日 --', status: 'info' },
  { label: '告警策略', value: '12 条', delta: '较昨日 ↑ 1', status: 'ok' },
  { label: '报告周期', value: '6 个', delta: '较昨日 --', status: 'info' },
  { label: '待审批变更', value: '3 项', delta: '较昨日 ↓ 1', status: 'warn' },
  { label: '最近保存', value: '15:20', delta: '保存人 sec_analyst', status: 'info' },
  { label: '审计完整', value: '100%', delta: '较昨日 --', status: 'ok' },
];

const settingsThresholdRows = [
  ['完整率', '全局', '95%', '90%', '>= 95%', '启用', 'data_owner', '编辑'],
  ['及时性', '全局', '2 min', '5 min', '<= 2 min', '启用', 'ops_engineer', '编辑'],
  ['准确性', '全局', '98%', '95%', '>= 98%', '启用', 'data_owner', '编辑'],
  ['重复率', '全局', '1%', '3%', '<= 1%', '启用', 'data_owner', '编辑'],
  ['字段缺失率', '全局', '2%', '5%', '<= 2%', '启用', 'data_owner', '编辑'],
  ['DLQ 数量', 'Topic 级', '1000', '5000', '<= 1K', '启用', 'ops_engineer', '编辑'],
  ['Watermark P95', 'Flink 作业', '3s', '6s', '< 3s', '启用', 'flink_owner', '编辑'],
  ['存储写入 P95', '存储组件', '800 ms', '1500 ms', '<= 800ms', '启用', 'ops_engineer', '编辑'],
];

const settingsRuleGroups = [
  {
    title: 'Topic 健康',
    version: 'v2.3.1',
    rules: [
      ['分区迟滞检测', '启用', '严重', '测试规则', '告警频率 5 分钟'],
      ['副本延迟检测', '启用', '告警', '测试规则', '告警频率 10 分钟'],
    ],
  },
  {
    title: 'Flink 质量',
    version: 'v2.4.0',
    rules: [
      ['Backpressure 检测', '启用', '严重', '测试规则', '告警频率 1 分钟'],
      ['Checkpoint 失败', '启用', '严重', '测试规则', '告警频率 立即'],
    ],
  },
  {
    title: '字段质量',
    version: 'v1.8.2',
    rules: [
      ['字段缺失检测', '启用', '告警', '测试规则', '告警频率 10 分钟'],
      ['格式校验失败', '启用', '中危', '测试规则', '告警频率 5 分钟'],
    ],
  },
  {
    title: '存储质量',
    version: 'v2.1.3',
    rules: [
      ['写入失败检测', '启用', '严重', '测试规则', '告警频率 1 分钟'],
      ['索引校验失败', '启用', '告警', '测试规则', '告警频率 5 分钟'],
    ],
  },
];

const settingsAlertRoutes = [
  ['严重', '短信 / 电话 / 邮件 / 钉钉', 'ops_manager', '5 分钟升级', 'https://alert/webhook/severe', 'P1 自动建单', '0 分钟', '00:39:12'],
  ['告警', '邮件 / 钉钉', 'ops_engineer', '15 分钟升级', 'https://alert/webhook/warning', 'P2 自动建单', '5 分钟', '02:39:12'],
  ['中危', '钉钉 / 飞书', 'data_owner', '30 分钟升级', 'https://alert/webhook/medium', 'P3 自动建单', '10 分钟', '06:39:12'],
  ['低危', '邮件 / 内部系统', 'data_analyst', '不升级', 'https://alert/webhook/info', '不建单', '30 分钟', '24:00:00'],
];

const settingsReportRows = [
  ['日报', '每天 08:00', '0 0 8 * * ?', 'sec_ops,data_owner', '数据质量日报模板 v2', '包含', 'JSON'],
  ['周报', '每周一 09:00', '0 0 9 ? * MON', 'sec_manager,cto,audit', '数据质量周报模板 v1', '包含', 'JSON'],
  ['月报', '每月 1日 10:00', '0 0 10 1 * ?', 'sec_manager,cto,audit', '数据质量月报模板 v1', '包含', 'JSON'],
  ['自定义', '每周三 18:00', '0 0 18 ? * WED', 'sec_ops', '自定义模板 v1', '部分', 'JSON'],
];

const settingsAuditRows = [
  ['2025-06-26 15:20:31', 'sec_analyst', '完整率 告警阈值', '95% -> 96%', '待审批', '详情'],
  ['2025-06-26 15:18:12', 'sec_analyst', 'Watermark P95 告警', '3s -> 2.5s', '待审批', '详情'],
  ['2025-06-26 15:15:44', 'sec_analyst', 'DLQ 数量 阻断阈值', '5000 -> 3000', '待审批', '详情'],
  ['2025-06-26 14:58:09', 'ops_manager', '告警策略 严重升级', '5 分钟 -> 3 分钟', '已批准', '详情'],
  ['2025-06-26 14:42:30', 'ops_engineer', '存储规则', '存储写入延迟检测', '已批准', '详情'],
  ['2025-06-25 10:25:11', 'data_owner', '字段缺失率 告警阈值', '2% -> 2.5%', '已拒绝', '详情'],
];

const settingsRailAnomalies = [
  ['未通过校验', '2', 'risk'],
  ['冲突配置', '1', 'warn'],
  ['阈值越界', '0', 'ok'],
  ['策略路由缺失', '0', 'info'],
];

const settingsRailLocateRows = [
  ['定位阈值配置', ''],
  ['定位规则组', ''],
  ['定位告警策略', ''],
  ['定位报告计划', ''],
  ['定位审批任务', ''],
];

const settingsRailRepairRows = [
  ['修复未通过校验', ''],
  ['合并冲突配置', ''],
  ['优化阈值建议', ''],
  ['补全路由策略', ''],
  ['校验并一键修复', ''],
];

const dataQualityTabs = [
  { label: '质量总览', slug: 'overview' },
  { label: 'Topic 健康', slug: 'topic-health' },
  { label: 'Flink 质量', slug: 'flink-quality' },
  { label: '字段质量', slug: 'field-quality' },
  { label: '存储质量', slug: 'storage-quality' },
  { label: '重放对账', slug: 'replay-reconcile' },
  { label: '质量报告', slug: 'report' },
  { label: '质量设置', slug: 'settings' },
] as const;

const topicHealthFallbackMetrics: PageSnapshot['metrics'] = [
  { label: 'Topic 健康分', value: '88/100', delta: '较昨日 ↑ 3', status: 'ok' },
  { label: '总 offset', value: '412.7M', delta: '24h 变化 ↑ 18.6M', status: 'info' },
  { label: '积压消息', value: '3.21M', delta: '较昨日 ↑ 0.72M', status: 'warn' },
  { label: '消费延迟 P95', value: '1.4s', delta: '较昨日 ↓ 0.6s', status: 'ok' },
  { label: '分区倾斜', value: '2.15', delta: '较昨日 ↑ 0.38', status: 'warn' },
  { label: '平均消息大小', value: '1.6KB', delta: '较昨日 ↑ 0.1KB', status: 'info' },
  { label: '异常 Topic', value: '3', delta: '较昨日 ↑ 1', status: 'risk' },
];

const flinkQualityFallbackMetrics: PageSnapshot['metrics'] = [
  { label: 'Flink 质量分', value: '91/100', delta: '较昨日 ↑ 2', status: 'ok' },
  { label: '运行作业', value: '9', delta: '较昨日 --', status: 'info' },
  { label: 'Checkpoint 成功率', value: '99.2%', delta: '较昨日 ↑ 0.6%', status: 'ok' },
  { label: 'Watermark 延迟 P95', value: '1.6s', delta: '较昨日 ↓ 0.3s', status: 'ok' },
  { label: 'Backpressure', value: '0.38', delta: '较昨日 ↑ 0.08', status: 'warn' },
  { label: '迟到数据率', value: '0.67%', delta: '较昨日 ↓ 0.12%', status: 'ok' },
  { label: '异常事件', value: '312', delta: '较昨日 ↑ 48', status: 'risk' },
];

const fieldQualityFallbackMetrics: PageSnapshot['metrics'] = [
  { label: '字段质量分', value: '94/100', delta: '较昨日 ↑ 3', status: 'ok' },
  { label: '完整率', value: '98.7%', delta: '较昨日 ↑ 0.8%', status: 'ok' },
  { label: '格式合规', value: '97.9%', delta: '较昨日 ↑ 1.2%', status: 'ok' },
  { label: '一致性', value: '96.4%', delta: '较昨日 ↑ 0.6%', status: 'ok' },
  { label: '异常字段', value: '23 项', delta: '较昨日 ↑ 5', status: 'risk' },
  { label: '影响记录', value: '18.4K 条', delta: '较昨日 ↑ 2.1K', status: 'warn' },
  { label: '待修复任务', value: '7 个', delta: '较昨日 ↓ 1', status: 'warn' },
];

const fieldQualityFallbackRows: DataQualityVisuals['fieldQualityRows'] = [
  ['五元组', '99.2%', '98.7%', '99.6%', '97.6%', '99.1%', '98.3%'],
  ['community_id', '98.1%', '97.8%', '--', '95.0%', '99.0%', '98.6%'],
  ['tenant', '94.2%', '99.0%', '99.3%', '96.1%', '--', '96.4%'],
  ['asset_id', '92.3%', '96.4%', '--', '93.5%', '--', '94.0%'],
  ['protocol', '99.6%', '96.2%', '94.1%', '97.5%', '99.2%', '98.1%'],
  ['timestamp', '99.4%', '98.9%', '--', '98.7%', '92.6%', '98.9%'],
  ['direction', '98.7%', '99.1%', '98.2%', '97.6%', '99.0%', '98.4%'],
  ['bytes', '99.3%', '99.2%', '--', '98.8%', '99.1%', '98.7%'],
  ['packets', '99.0%', '99.0%', '--', '98.5%', '99.1%', '98.6%'],
  ['alert_id', '95.6%', '96.7%', '--', '94.0%', '--', '95.2%'],
];

const fieldTrendFallback: DataQualityVisuals['fieldTrend'] = {
  times: ['15:30', '19:30', '23:30', '03:30', '07:30', '11:30', '15:30'],
  missing: [220, 640, 980, 1850, 3650, 4520, 3080, 2610, 2260, 1940, 1820, 2160, 2480, 1880],
  format: [160, 430, 760, 1260, 2260, 3130, 2580, 2320, 2040, 1720, 1540, 1880, 2130, 1760],
  mapping: [120, 360, 590, 980, 1680, 2310, 2180, 1920, 1720, 1460, 1280, 1510, 1830, 1390],
  timeDrift: [70, 160, 270, 440, 790, 1160, 1020, 870, 760, 650, 580, 690, 820, 620],
  unknownProtocol: [40, 90, 150, 230, 410, 560, 520, 470, 420, 350, 310, 380, 470, 360],
};

const fieldTrendSummaryFallback: DataQualityVisuals['fieldTrendSummary'] = [
  ['缺失值', '7,235', 'info'],
  ['格式不合法', '5,134', 'ok'],
  ['映射不一致', '3,126', 'ok'],
  ['时间漂移', '1,738', 'ok'],
  ['未知协议', '1,167', 'ok'],
];

const fieldKpiTrendFallback: DataQualityVisuals['fieldKpiTrends'] = [
  [89, 90, 90, 91, 92, 91, 93, 92, 93, 94, 93, 94],
  [97.2, 97.6, 97.4, 98.1, 97.8, 98.2, 97.9, 98.4, 98.1, 98.5, 98.3, 98.7],
  [96.5, 96.8, 97.1, 96.9, 97.4, 97.2, 97.7, 97.4, 97.9, 97.6, 97.8, 97.9],
  [94.8, 95.2, 95.5, 95.1, 95.6, 95.9, 95.5, 96.1, 95.8, 96.2, 96.0, 96.4],
  [18, 20, 17, 21, 19, 23, 22, 25, 20, 24, 21, 23],
  [15.2, 16.8, 15.9, 17.3, 16.1, 18.7, 17.4, 19.1, 17.8, 18.9, 17.7, 18.4],
  [10, 9, 11, 8, 9, 7, 8, 7, 6, 8, 7, 7],
];

const fieldCommunityCheckFallbackRows: DataQualityVisuals['communityCheckRows'] = [
  ['五元组 → community_id', '124,356', '120,865', '3,491', '97.19%'],
  ['哈希碰撞告警', '124,356', '124,353', '3', '99.99%'],
  ['协议一致性', '124,356', '123,721', '635', '99.49%'],
];

const fieldCommunityMismatchFallbackRows: DataQualityVisuals['communityMismatchRows'] = [
  ['15:29:44', 'sess-71c21d6', '10.12.8.44:321', '172.16.5.10:80', '6', '8a9f3a...', 'a1c9b6...', '原始 cid 缺失'],
  ['15:28:11', 'sess-c4e47e4', '10.23.5.53:911', '192.168.1.20:443', '6', 'c3d4a77...', 'c9d4a77...', '端口反向不一致'],
  ['15:26:55', 'sess-5d4d7e1', '10.0.9.49:83', '8.8.8.8:53', '17', 'e8f2ac...', 'c5f8ac...', '协议异常'],
  ['15:25:37', 'sess-9b0c2f9', '10.14.7.4:0012', '172.16.5.11:80', '6', 'f2b20c...', '5f8a76...', '源目反置'],
  ['15:21:08', 'sess-8efd6a12', '10.12.8.33:544', '172.16.5.12:8080', '6', '7dcd940...', '7dcd940...', '周期 cid 漂移'],
];

const fieldAnomalyFallbackRows: DataQualityVisuals['fieldAnomalyRows'] = [
  ['15:29:44', 'traffic_normal', 'tenant', '缺失值', 'null (空)', '__unknown__', 'ACME-APP-12', '查看证据'],
  ['15:28:11', 'asset_inventory', 'asset_id', '缺失值', 'null (空)', '--', '--', '查看证据'],
  ['15:26:55', 'traffic_normal', 'protocol', '未知枚举', '143', '__unknown__', 'WEB-SRV-07', '创建任务'],
  ['15:25:37', 'traffic_session', 'timestamp', '时间漂移', '2025-06-25 14:05:37', '2025-06-26 15:25:37', 'DB-SRV-02', '查看证据'],
  ['15:21:08', 'traffic_normal', 'community_id', '校验不匹配', 'a1c9d4899f9...', '86f95a21cd...', 'WEB-SRV-07', '创建任务'],
  ['15:18:42', 'traffic_normal', 'src_ip', '格式不合法', '999.1.1.1', '10.255.255.255', '--', '创建任务'],
  ['15:09:12', 'alert_event', 'alert_id', '格式不合法', '@ALERT123', 'ALERT-123', 'FW-01', '查看证据'],
  ['14:42:33', 'traffic_session', 'bytes', '负数值', '-1024', '0', '--', '创建任务'],
];

const fieldLineageFallbackRows: DataQualityVisuals['fieldLineageRows'] = [
  ['traffic_raw', '解析与清洗', '字段映射 A(3)', 'ClickHouse', 'warn'],
  ['traffic_session_raw', '会话构建', '枚举映射 A(5)', 'OpenSearch', 'risk'],
  ['asset_inventory', '资产标准化', '格式校验 A(2)', 'NebulaGraph', 'warn'],
  ['alert_raw', '告警解析', '时间校验 A(4)', 'MinIO', 'warn'],
];

const fieldRepairFallbackRows: DataQualityVisuals['fieldRepairRows'] = [
  ['补全 tenant 缺失值', 'tenant', '映射：机器 src_ip → 租户所属部门', '张三', '进行中', '2025-06-27', '--', '查看'],
  ['asset_id 补全规则', 'asset_id', '映射：src_ip → asset_id', '李四', '待处理', '2025-06-27', '--', '创建'],
  ['protocol 映射补全', 'protocol', '枚举映射：143 → __unknown__', '王五', '待检查', '2025-06-26', '通过', '查看'],
  ['时间同步校正规则', 'timestamp', '校正：统一为 UTC+8', '赵六', '已完成', '2025-06-26', '通过', '查看'],
  ['community_id 校验修复', 'community_id', '重新计算 SHA-1 并回填', '孙七', '进行中', '2025-06-28', '--', '查看'],
  ['src_ip 格式校正', 'src_ip', '非法 IP → 10.255.255.255', '周八', '待处理', '2025-06-27', '--', '创建'],
  ['bytes 负数归零', 'bytes', '值 < 0 → 0', '吴九', '已完成', '2025-06-26', '通过', '查看'],
];

const storageQualityFallbackMetrics: PageSnapshot['metrics'] = [
  { label: '存储质量分', value: '93/100', delta: '较昨日 ↑ 4', status: 'ok' },
  { label: '写入成功率', value: '99.84%', delta: '较昨日 ↑ 0.12%', status: 'ok' },
  { label: '写入延迟 P95', value: '420 ms', delta: '较昨日 ↓ 80 ms', status: 'ok' },
  { label: '失败写入', value: '186 条', delta: '较昨日 ↑ 34', status: 'warn' },
  { label: '索引滞后', value: '2.1 s', delta: '较昨日 ↑ 0.4 s', status: 'warn' },
  { label: '归档成功率', value: '99.7%', delta: '较昨日 ↑ 0.2%', status: 'ok' },
  { label: '容量水位', value: '72.6%', delta: '较昨日 ↑ 1.8%', status: 'info' },
];

const storageComponentFallbackRows: DataQualityVisuals['storageComponentRows'] = [
  ['ClickHouse', '注意', '78.3 K EPS', '99.82%', '380 ms', '12,356 Distributed 队列', '14.2 TB / 20 TB', '2 shard / 2 replica', '详情'],
  ['OpenSearch', '警告', '12.6 K docs/s', '99.71%', '560 ms', '8,912 Bulk 队列', '6.9 TB / 10 TB', '36 index / 72 shard', '索引滞后 2.1s'],
  ['NebulaGraph', '正常', '2.1 K edges/s', '99.46%', '210 ms', '256 写入队列', '420 GB / 1 TB', '3 partition 健康', '详情'],
  ['MinIO', '注意', '1.8 K objects/s', '99.64%', '690 ms', '1,245 Multipart 队列', '72.4 TB / 120 TB', '8 bucket 生命周期正常', '重试'],
];

const storageTrendFallback: DataQualityVisuals['storageTrend'] = {
  times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
  clickhouse: [62, 66, 70, 73, 76, 78, 75, 79, 82, 80, 78, 84, 86, 83, 88, 85],
  opensearch: [32, 35, 37, 39, 41, 45, 43, 44, 48, 46, 47, 50, 52, 49, 55, 53],
  nebula: [18, 20, 22, 21, 24, 25, 23, 27, 26, 28, 29, 30, 32, 31, 34, 33],
  minio: [14, 15, 16, 18, 17, 19, 21, 20, 22, 23, 21, 24, 26, 25, 27, 26],
  latencyP95: [48, 46, 52, 50, 58, 62, 66, 64, 70, 68, 72, 76, 74, 82, 80, 78],
  latencySla: [60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60],
};

const storageCapacityFallback: DataQualityVisuals['storageCapacityTrend'] = {
  days: ['06-20', '06-21', '06-22', '06-23', '06-24', '06-25', '06-26'],
  clickhouse: [54, 56, 58, 61, 64, 67, 70],
  opensearch: [42, 44, 47, 49, 53, 56, 59],
  nebula: [28, 30, 31, 33, 36, 38, 40],
  minio: [60, 62, 64, 67, 69, 71, 73],
  threshold: [80, 80, 80, 80, 80, 80, 80],
};

const storageFailureFallbackRows: DataQualityVisuals['storageFailureRows'] = [
  ['15:29:44', 'ClickHouse', 'traffic.sessions (Distributed)', '分布式队列积压/写入延迟', '3,912,445', '3', '进行中', '清理队列'],
  ['15:28:11', 'OpenSearch', 'traffic-logs-2025.06.26', 'bulk rejected: thread pool queue full', '812,301', '5', '进行中', '扩容索引'],
  ['15:26:55', 'NebulaGraph', 'edge_upsert', 'edge upsert timeout', '54,812', '2', '重试中', '增加超时'],
  ['15:25:37', 'MinIO', 'pcap-archive/2025/06/26', '超时写入，副本不可用', '412,005', '7', '重试中', '重试任务'],
  ['15:18:08', 'OpenSearch', 'asset-inventory-2025.06', 'mapping 冲突', '24,118', '1', '已结束', '修复映射'],
];

const storagePipelineFallbackRows: DataQualityVisuals['storagePipelineRows'] = [
  { from: 'Kafka / Flink', to: 'ClickHouse', label: 'session/events 写入', status: 'warn' },
  { from: 'Kafka / Flink', to: 'OpenSearch', label: 'log/index bulk', status: 'risk' },
  { from: 'Kafka / Flink', to: 'NebulaGraph', label: 'entity/edge upsert', status: 'ok' },
  { from: 'Kafka / Flink', to: 'MinIO', label: 'pcap archive multipart', status: 'warn' },
  { from: 'ClickHouse', to: '写入确认', label: 'ack 99.82%', status: 'warn' },
  { from: 'OpenSearch', to: '重试队列 / DLQ', label: 'bulk reject', status: 'risk' },
  { from: 'NebulaGraph', to: '写入确认', label: 'raft commit', status: 'ok' },
  { from: 'MinIO', to: '归档重试', label: 'multipart retry', status: 'warn' },
];

const storageReplicaFallbackRows: DataQualityVisuals['storageReplicaRows'] = [
  ['ClickHouse 副本', '2 shard / 2 replica', '副本延迟 P95 1.2s', 'Keeper 正常', '注意'],
  ['OpenSearch 分片', '36 index / 72 shard', 'yellow 3 / red 1', 'refresh lag 2.1s', '警告'],
  ['NebulaGraph 分区', '3 partition', 'Raft commit 99.96%', 'leader 均衡', '正常'],
  ['MinIO 生命周期', '8 bucket', '对象总数 1.28 亿', '过期率 0.18%', '注意'],
];

const storageIndexHealthFallback: DataQualityVisuals['storageIndexHealth'] = [
  { label: '正常', value: 68, status: 'ok' },
  { label: '警告', value: 3, status: 'warn' },
  { label: '异常', value: 1, status: 'risk' },
];

const storagePartitionFallbackRows: DataQualityVisuals['storagePartitionRows'] = [
  ['NebulaGraph', 'partition-01', 'leader 正常', 'commit 99.97%'],
  ['NebulaGraph', 'partition-02', 'leader 正常', 'commit 99.96%'],
  ['NebulaGraph', 'partition-03', 'follower lag', 'commit 99.91%'],
];

const storageObjectFallbackRows: DataQualityVisuals['storageObjectRows'] = [
  ['Bucket 数', '8'],
  ['对象总数', '1.28 亿'],
  ['生命周期规则', '6 条'],
  ['24h 过期率', '0.18%'],
];

const storageRailAlertFallbackRows: DataQualityVisuals['storageRailAlerts'] = [
  ['中', 'ClickHouse Distributed 队列积压', '12,356', '03:21:44', 'warn'],
  ['高', 'OpenSearch 索引滞后升高', '2.1s', '03:18:12', 'risk'],
  ['中', 'MinIO Multipart 重试较多', '1,245', '03:09:33', 'warn'],
  ['中', 'ClickHouse 写入延迟升高', '380ms', '02:56:41', 'warn'],
  ['高', 'OpenSearch Bulk 拒绝率升高', '0.67%', '02:41:05', 'risk'],
];

const storageRailLocateFallbackRows: DataQualityVisuals['storageRailLocateRows'] = ['定位失败写入', '刷新索引滞后', '组件健康详情', '容量与水位趋势', '写入链路追踪', '映射与故障队列'];
const storageRailRepairFallbackRows: DataQualityVisuals['storageRailRepairRows'] = ['清理 ClickHouse 分布式队列', '优化 OpenSearch 索引分片', '创建归档重试任务', '检查 MinIO 生命周期策略', '查看修复工单'];
const storageRailEvidenceFallbackRows: DataQualityVisuals['storageRailEvidenceRows'] = ['导出存储质量报告', '导出异常明细', '延迟报告下载', '近期历史报告', '证据包快照'];

const replayReconcileFallbackMetrics: PageSnapshot['metrics'] = [
  { label: '对账通过率', value: '99.12%', delta: '较昨日 ↑ 0.42%', status: 'ok' },
  { label: '待重放 DLQ', value: '12,845', delta: '较昨日 ↓ 843', status: 'warn' },
  { label: '重放成功率', value: '98.6%', delta: '较昨日 ↑ 0.61%', status: 'ok' },
  { label: '重复记录', value: '2,136', delta: '较昨日 ↓ 256', status: 'warn' },
  { label: '幂等冲突', value: '47', delta: '较昨日 ↓ 12', status: 'ok' },
  { label: '窗口差异率', value: '0.31%', delta: '较昨日 ↓ 0.08%', status: 'ok' },
  { label: '验收包', value: '8', delta: '较昨日 ↑ 1', status: 'info' },
];

const replayTaskFallbackRows: DataQualityVisuals['replayTaskRows'] = [
  ['flow_original', 'flow_original.v1', '06-26 00:00 - 15:00', '8,421', '98.92%', '94', '通过', '详情 / 重放'],
  ['flow_enriched', 'flow_enriched.v1', '06-26 00:00 - 15:00', '2,136', '99.31%', '15', '通过', '详情 / 重放'],
  ['dns_logs', 'dns_logs.v1', '06-26 00:00 - 15:00', '1,128', '97.84%', '24', '警告', '详情 / 重放'],
  ['asset_events', 'asset_events.v1', '06-26 00:00 - 15:00', '642', '99.01%', '6', '通过', '详情 / 重放'],
  ['threat_alerts', 'threat_alerts.v1', '06-26 00:00 - 15:00', '311', '96.43%', '11', '警告', '详情 / 重放'],
  ['pcap_index', 'pcap_index.v1', '06-26 00:00 - 15:00', '207', '99.42%', '3', '通过', '详情 / 重放'],
];

const replayReconcileTrendFallback: DataQualityVisuals['replayReconcileTrend'] = {
  times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
  sourceTotal: [64, 68, 66, 72, 78, 74, 70, 76, 82, 78, 80, 84, 86, 83, 88, 90],
  sinkTotal: [63, 67, 65, 71, 77, 73, 69, 75, 81, 77, 79, 83, 85, 82, 87, 89],
  diffCount: [18, 16, 19, 22, 26, 21, 18, 20, 24, 22, 19, 21, 23, 18, 20, 17],
  diffRate: [34, 31, 36, 42, 48, 39, 35, 38, 44, 41, 37, 39, 42, 34, 37, 32],
  diffRateThreshold: [58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58],
};

const replayReconcileSummaryFallback: DataQualityVisuals['replayReconcileSummary'] = [
  ['源端总数', '3.21B', 'info'],
  ['落库总数', '3.20B', 'ok'],
  ['差异数量', '9.95M', 'warn'],
  ['差异率', '0.31%', 'ok'],
];

const replayIdempotencyFallbackRows: DataQualityVisuals['replayIdempotencyRows'] = [
  ['幂等键一致性', 'community_id+five_tuple+ts', '通过', '47', '查看'],
  ['Hash 碰撞检测', 'Murmur3(128)', '通过', '0', '查看'],
  ['重复 session_id', 'session_id', '警告', '1,236', '查看'],
  ['重复 alert_id', 'alert_id', '通过', '342', '查看'],
  ['重放批次重叠', 'batch_id/window', '通过', '0', '查看'],
  ['幂等冲突写入', 'upsert_conflict', '警告', '47', '查看'],
];

const replayDifferenceFallbackRows: DataQualityVisuals['replayDifferenceRows'] = [
  ['06-26 14:00', 'flow_original.v1', 'offset gap', 'f7a3c2e1...', 'offset 15874231', 'offset 15874190', '消息中断与重投', '重放补齐'],
  ['06-26 13:00', 'flow_enriched.v1', 'schema mismatch', 'b1d9a8c2...', 'app_id:102', 'app_id:null', '字段类型变更未兼容', '规则更新'],
  ['06-26 12:00', 'dns_logs.v1', 'late event', 'c45f2a77...', 'ts 12:35:12', 'ts 12:20:05', '迟到数据窗口', '延长窗口'],
  ['06-26 12:00', 'asset_events.v1', 'duplicate key', 'd8a7b3a1...', 'asset_id:9987', 'asset_id:9987', '重复写入', '幂等检查'],
  ['06-26 11:00', 'threat_alerts.v1', 'duplicate key', 'e27b4d91...', 'alert_id:55123', 'alert_id:55123', '主键告警产生', '去重规则'],
  ['06-26 10:00', 'pcap_index.v1', 'sink timeout', 'a9c4e1f2...', '写入 2.1MB/s', '写入超时', 'ClickHouse 超时', '重试写入'],
];

const replayFlowFallbackNodes: DataQualityVisuals['replayFlowNodes'] = [
  { id: 'dlq', label: 'DLQ / Kafka', detail: '待重放 12,845 / Topic 6 个 / 最早 offset 15:26:11', status: 'warn' },
  { id: 'flink', label: '重放作业（Flink）', detail: 'Job: replay-job / 并行度 8 / RUNNING / Checkpoint 3m ago', status: 'ok' },
  { id: 'idempotent', label: '去重过滤（幂等）', detail: '幂等键 6 规则 / 过滤率 0.72% / 冲突 47', status: 'warn' },
  { id: 'sink', label: '落库目标', detail: 'ClickHouse / OpenSearch / NebulaGraph / MinIO', status: 'ok' },
  { id: 'retry', label: '重试队列', detail: '失败任务 2 / 回退策略 offset batch', status: 'risk' },
  { id: 'checkpoint', label: '校验检查点', detail: '对账窗口 24h / 差异率 0.31%', status: 'ok' },
  { id: 'gate', label: '验收门禁', detail: '验收包 8 / 审计记录完整', status: 'ok' },
];

const replayFlowFallbackEdges: DataQualityVisuals['replayFlowEdges'] = [
  { from: 'DLQ / Kafka', to: '重放作业（Flink）', label: '数据流', status: 'ok' },
  { from: '重放作业（Flink）', to: '去重过滤（幂等）', label: '校验流', status: 'info' },
  { from: '去重过滤（幂等）', to: '落库目标', label: '数据流', status: 'ok' },
  { from: '重放作业（Flink）', to: '重试队列', label: '异常/重试', status: 'risk' },
  { from: '落库目标', to: '校验检查点', label: '校验流', status: 'info' },
  { from: '校验检查点', to: '验收门禁', label: '控制流', status: 'warn' },
];

const replayEvidenceFallbackRows: DataQualityVisuals['replayEvidenceRows'] = [
  ['对账报告', 'data_analyst', '06-26 15:20', '已归档', '导出 PDF'],
  ['重放日志', 'ops_engineer', '06-26 15:18', '已归档', '导出日志'],
  ['投递快照摘要', 'qa_engineer', '06-26 15:16', '已归档', '导出 JSON'],
  ['差异样本还原', 'sec_analyst', '06-26 15:14', '已归档', '导出样本'],
  ['审计记录', 'sec_manager', '06-26 15:25', '已归档', '导出记录'],
];

const replayRailAlertFallbackRows: DataQualityVisuals['replayRailAlerts'] = [
  ['高', '差异率超阈值窗口', '3', 'risk'],
  ['中', '重放失败任务', '2', 'warn'],
  ['中', '幂等冲突告警', '1', 'warn'],
  ['中', '重复记录激增', '2', 'warn'],
];

const replayRailLocateFallbackRows: DataQualityVisuals['replayRailLocateRows'] = ['定位 DLQ Topic', '定位重放作业', '定位差异窗口', '查看对账详情'];
const replayRailRepairFallbackRows: DataQualityVisuals['replayRailRepairRows'] = ['重放失败重试', '扩容重放作业', '补齐幂等规则', '优化幂等字段索引', '延长对账时间窗'];
const replayRailEvidenceFallbackRows: DataQualityVisuals['replayRailEvidenceRows'] = ['导出对账报告', '生成验收包', '查看验收历史', '审计操作日志'];

const flinkJobFallbackRows: DataQualityVisuals['flinkJobRows'] = [
  ['session-job', '运行中', '24', '1.3s / 1.1s', '1.2s', '0.21', '0.32%', '12', '正常'],
  ['feature-job', '运行中', '16', '1.4s / 1.2s', '1.4s', '0.25', '0.41%', '5', '正常'],
  ['rule-job', '运行中', '20', '1.2s / 1.0s', '1.1s', '0.22', '0.38%', '8', '正常'],
  ['pcap-index-job', '重启中', '12', '2.1s / 2.0s', '1.8s', '0.45', '0.71%', '18', '正常'],
  ['behavior-job', '背压中', '32', '1.6s / 1.4s', '1.7s', '0.78', '1.42%', '156', '正常'],
  ['alert-generator-job', '运行中', '8', '1.1s / 0.9s', '0.9s', '0.18', '0.21%', '6', '正常'],
  ['log-job', '运行中', '10', '1.2s / 1.0s', '1.0s', '0.19', '0.29%', '9', '正常'],
  ['user-behavior-job', '运行中', '16', '1.5s / 1.3s', '1.3s', '0.31', '0.56%', '24', '正常'],
];

const flinkCheckpointTrendFallback: DataQualityVisuals['flinkCheckpointTrend'] = {
  times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
  checkpointDuration: [60, 57, 58, 55, 52, 18, 54, 50, 22, 56, 54, 49, 51, 47, 50, 43],
  checkpointAge: [70, 68, 66, 64, 61, 58, 55, 20, 58, 56, 54, 52, 48, 45, 19, 50],
  watermarkP95: [76, 74, 72, 75, 70, 77, 72, 65, 76, 73, 75, 70, 69, 74, 56, 71],
  watermarkSla: [44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44],
  checkpointSla: [58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58],
};

const flinkBackpressureFallbackBuckets = ['0-7', '8-15', '16-23', '24-31', '32-39', '40-47'];

const flinkBackpressureFallbackRows: DataQualityVisuals['flinkBackpressureRows'] = [
  { label: 'session-job', values: ['ok', 'ok', 'ok', 'info', 'ok', 'info', 'ok', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok'] },
  { label: 'feature-job', values: ['ok', 'ok', 'info', 'ok', 'warn', 'info', 'ok', 'ok', 'warn', 'warn', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'info', 'ok'] },
  { label: 'rule-job', values: ['ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info'] },
  { label: 'pcap-index-job', values: ['info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info'] },
  { label: 'behavior-job', values: ['warn', 'warn', 'warn', 'risk', 'risk', 'warn', 'warn', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk'] },
  { label: 'alert-generator-job', values: ['warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn'] },
  { label: 'log-job', values: ['ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info'] },
  { label: 'user-behavior-job', values: ['ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok'] },
];

const flinkLateTopicFallbackRows: DataQualityVisuals['flinkLateTopicRows'] = [
  ['flow_original', '1.23M', '14.6K', '2.1K'],
  ['flow_enriched', '1.02M', '11.3K', '1.4K'],
  ['dns_logs', '362K', '6.2K', '0.8K'],
  ['tls_logs', '286K', '4.1K', '0.5K'],
  ['asset_events', '184K', '2.0K', '0.3K'],
  ['threat_alerts', '96K', '1.6K', '0.2K'],
];

const flinkWindowFallbackRows: DataQualityVisuals['flinkWindowRows'] = [
  ['1 min', '3.2s', '0.21%'],
  ['5 min', '4.8s', '0.17%'],
  ['10 min', '6.1s', '0.14%'],
  ['30 min', '8.7s', '0.11%'],
  ['60 min', '11.3s', '0.09%'],
];

const flinkFailureFallbackRows: DataQualityVisuals['flinkFailureRows'] = [
  ['TimeoutException', 'behavior-job', 'map-3b6f', '62', '06-26 11:42', '06-26 15:20', '检查下游处理组'],
  ['BackpressureException', 'behavior-job', 'sink-9a21', '48', '06-26 10:15', '06-26 15:20', '扩容并行度'],
  ['CheckpointException', 'pcap-index-job', 'src-2111', '18', '06-26 08:33', '06-26 15:18', '检查快照存储'],
  ['WatermarkLagAlert', 'user-behavior-job', 'wm-77aa', '15', '06-26 09:41', '06-26 15:05', '优化 watermark 策略'],
  ['OutOfMemoryError', 'log-job', 'proc-1d4c', '9', '06-26 07:58', '06-26 14:55', '调整内存与 TTL'],
  ['SerializationException', 'session-job', 'map-6e12', '7', '06-26 07:12', '06-26 14:31', '修复序列化配置'],
  ['RebalanceInProgress', 'alert-generator-job', 'rebalance', '6', '06-26 06:44', '06-26 13:22', '等待再均衡完成'],
];

const flinkSinkFallbackRows: DataQualityVisuals['flinkSinkRows'] = [
  { name: 'ClickHouse', status: '正常', eps: '18,734', success: '99.95%', p95: '36 ms', retries: '128', trend: [68, 70, 69, 72, 71, 73, 70, 74, 73, 76, 72, 75, 78, 74, 82, 76] },
  { name: 'OpenSearch', status: '正常', eps: '15,962', success: '99.91%', p95: '52 ms', retries: '215', trend: [70, 68, 72, 69, 71, 73, 72, 76, 75, 74, 77, 73, 79, 76, 80, 78] },
  { name: 'NebulaGraph', status: '正常', eps: '8,521', success: '99.97%', p95: '41 ms', retries: '74', trend: [74, 73, 75, 72, 76, 74, 77, 78, 75, 80, 76, 82, 79, 83, 81, 84] },
  { name: 'MinIO', status: '正常', eps: '6,342', success: '99.98%', p95: '28 ms', retries: '31', trend: [78, 76, 79, 77, 80, 79, 82, 81, 83, 82, 85, 83, 86, 84, 87, 86] },
];

type DataQualityTabSlug = (typeof dataQualityTabs)[number]['slug'];

type FieldQualityDetail = {
  title: string;
  description: string;
  columns: string[];
  rows: string[][];
  actionLabel?: string;
  actionSuccessMessage?: string;
};

type OpenFieldQualityDetail = (detail: FieldQualityDetail, focusSelector?: string) => void;

const metricToneClass = {
  ok: 'is-ok',
  warn: 'is-warn',
  risk: 'is-risk',
  info: 'is-info',
};

const metricTrendFallback = (value: string, index: number) => {
  const numeric = Number.parseFloat(value.replace(/[^0-9.]/g, '')) || 1;
  const seed = [0.96, 0.99, 0.97, 1.01, 0.98, 1.02, 1, 1.03];
  return seed.map((factor, offset) => Number((numeric * factor + ((index + offset) % 3 - 1) * Math.max(numeric * 0.008, 0.2)).toFixed(2)));
};

const resolveDataQualityTab = (param: string | null): DataQualityTabSlug => (
  dataQualityTabs.find((item) => item.slug === param)?.slug ?? 'overview'
);

export function DataQualityPage({ route }: { route: NavRoute }) {
  const [dlqReplayForm] = Form.useForm<DLQReplayFallbackRequest>();
  const [searchParams, setSearchParams] = useSearchParams();
  const [dlqSampleOpen, setDlqSampleOpen] = useState(false);
  const [dlqReplayOpen, setDlqReplayOpen] = useState(false);
  const [dlqReplayResult, setDlqReplayResult] = useState<DLQReplayFallbackResult | null>(null);
  const [fieldDetail, setFieldDetail] = useState<FieldQualityDetail | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [timeRange, setTimeRange] = useState<DataQualityTimeRange>('近 24 小时');
  const isVisualBreakdown = searchParams.has('__codex_ui_breakdown_production');
  const activeTab = resolveDataQualityTab(searchParams.get('tab'));
  const isTopicHealthTab = activeTab === 'topic-health';
  const isFlinkQualityTab = activeTab === 'flink-quality';
  const isFieldQualityTab = activeTab === 'field-quality';
  const isStorageQualityTab = activeTab === 'storage-quality';
  const isReplayReconcileTab = activeTab === 'replay-reconcile';
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, timeRange],
    queryFn: () => fetchPageSnapshot(route.id, { dataQualityTimeRange: timeRange }),
    refetchInterval: autoRefresh && !isVisualBreakdown ? 30_000 : false,
    refetchIntervalInBackground: autoRefresh && !isVisualBreakdown,
  });
  const dlqReplayMutation = useMutation({
    mutationFn: requestDLQFallbackReplay,
    onSuccess: async (result) => {
      setDlqReplayResult(result);
      message.success(result.status === 'dry_run' ? 'DLQ dry-run 预检完成' : 'DLQ 重放请求已提交');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(errorText(mutationError));
    },
  });

  const snapshot = data;
  const rows = useMemo(() => snapshot?.rows ?? [], [snapshot?.rows]);
  const dataQualityVisuals = snapshot?.visuals?.dataQuality;
  const activeTabLabel = dataQualityTabs.find((tab) => tab.slug === activeTab)?.label ?? '质量总览';
  const topicMetricSource = dataQualityVisuals?.topicMetrics ?? topicHealthFallbackMetrics;
  const topicMetricLabels = topicMetricSource.map((item) => item.label);
  const flinkMetricSource = dataQualityVisuals?.flinkKpis ?? flinkQualityFallbackMetrics;
  const flinkMetricLabels = flinkMetricSource.map((item) => item.label);
  const fieldMetricSource = dataQualityVisuals?.fieldKpis ?? fieldQualityFallbackMetrics;
  const fieldMetricLabels = fieldMetricSource.map((item) => item.label);
  const fieldKpiTrends = dataQualityVisuals?.fieldKpiTrends ?? fieldKpiTrendFallback;
  const storageMetricSource = dataQualityVisuals?.storageKpis ?? storageQualityFallbackMetrics;
  const storageMetricLabels = storageMetricSource.map((item) => item.label);
  const replayMetricSource = dataQualityVisuals?.replayKpis ?? replayReconcileFallbackMetrics;
  const replayMetricLabels = replayMetricSource.map((item) => item.label);
  const metricLabels = activeTab === 'report' ? reportMetricLabels : activeTab === 'settings' ? settingsMetricLabels : isTopicHealthTab ? topicMetricLabels : isFlinkQualityTab ? flinkMetricLabels : isFieldQualityTab ? fieldMetricLabels : isStorageQualityTab ? storageMetricLabels : isReplayReconcileTab ? replayMetricLabels : route.page.kpis;
  const metricSource = activeTab === 'report' ? reportMetrics : activeTab === 'settings' ? settingsMetrics : isTopicHealthTab ? topicMetricSource : isFlinkQualityTab ? flinkMetricSource : isFieldQualityTab ? fieldMetricSource : isStorageQualityTab ? storageMetricSource : isReplayReconcileTab ? replayMetricSource : snapshot?.metrics ?? [];
  const metrics = metricLabels.map((label) => metricSource.find((item) => item.label === label) ?? fallbackMetric(label));
  const qualityScore = Number.parseFloat(metrics[0]?.value ?? '0') || 92;
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderQualityCell(column, value),
  }));
  const openDlqReplay = () => {
    dlqReplayForm.setFieldsValue(defaultDLQReplayRequest());
    setDlqReplayResult(null);
    setDlqReplayOpen(true);
    if (activeTab !== 'replay-reconcile') {
      setSearchParams({ tab: 'replay-reconcile' });
    }
  };
  const submitDlqReplay = (values: DLQReplayFallbackRequest) => {
    dlqReplayMutation.mutate(buildDLQReplayDryRunRequest({
      ...values,
      requested_at_unix: Math.floor(Date.now() / 1000),
    }));
  };
  const openFieldDetail: OpenFieldQualityDetail = (detail, focusSelector) => {
    setFieldDetail({
      ...detail,
      columns: [...detail.columns, '接口预留', '审计事件'],
      rows: detail.rows.map((row) => [
        ...row,
        actionEndpointLabel(dataQualityContextActionPlan),
        dataQualityContextActionPlan?.auditEvent ?? 'DATA_QUALITY_ACTION_REQUESTED',
      ]),
    });
    if (focusSelector) {
      window.setTimeout(() => {
        document.querySelector(focusSelector)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 0);
    }
  };
  const openReplayReconcile = () => {
    setSearchParams({ tab: 'replay-reconcile' });
    message.info('已切换至重放对账视图');
  };
  const openUnboundBusinessAction = (event: MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement;
    const button = target.closest('button');
    if (!button || button.disabled || button.dataset.dqActionManaged === 'true') return;
    if (button.closest('.ant-drawer, .ant-modal')) return;
    if (button.closest('.taf-data-quality-field-sample-table, .taf-data-quality-field-repair-table, .taf-data-quality-field-lineage, .taf-data-quality-field-rail')) return;
    const label = (button.getAttribute('aria-label') || button.getAttribute('title') || button.textContent || '').replace(/\s+/g, ' ').trim();
    if (!label) return;
    const actionable = /创建|导出|修复|保存|同步|更新|配置|审批|重放|重试|回滚|清理/.test(label);
    setFieldDetail({
      title: label,
      description: actionable
        ? `已加载“${label}”的操作预览；确认后将通过预留接口提交数据质量模拟业务动作并返回审计反馈。`
        : `已打开“${label}”的业务上下文，可继续查看当前 ${activeTabLabel} 数据。`,
      columns: ['操作', '当前视图', '接口预留', '审计事件', '处理结果'],
      rows: [[
        label,
        activeTabLabel,
        actionEndpointLabel(dataQualityContextActionPlan),
        dataQualityContextActionPlan?.auditEvent ?? 'DATA_QUALITY_ACTION_REQUESTED',
        actionable ? '确认后提交模拟任务' : '已定位关联业务数据',
      ]],
      actionLabel: actionable ? '确认提交' : undefined,
      actionSuccessMessage: actionable ? `${label}已提交，审计记录已生成` : undefined,
    });
  };

  return (
    <div className="taf-page taf-data-quality" data-business-action-delegate="data-quality-context" onClick={openUnboundBusinessAction}>
      <section className={`taf-data-quality-shell is-unified-tabs${activeTab === 'report' ? ' is-report-tab' : ''}${activeTab === 'settings' ? ' is-settings-tab' : ''}${isTopicHealthTab ? ' is-topic-health-tab' : ''}${isFlinkQualityTab ? ' is-flink-quality-tab' : ''}${isFieldQualityTab ? ' is-field-quality-tab' : ''}${isStorageQualityTab ? ' is-storage-quality-tab' : ''}${isReplayReconcileTab ? ' is-replay-reconcile-tab' : ''}`}>
        <main className="taf-data-quality-main">
          <header className="taf-data-quality-titlebar">
            <div className="taf-data-quality-heading">
              <h1 title={route.page.title}>{route.page.title}</h1>
              <span title={`当前视图：${activeTabLabel}`}>当前视图：{activeTabLabel}</span>
            </div>
            <nav className="taf-data-quality-tabs" aria-label="数据质量视图">
              {dataQualityTabs.map((tab, index) => (
                <button
                  key={tab.slug}
                  type="button"
                  className={tab.slug === activeTab ? 'is-active' : ''}
                  aria-selected={tab.slug === activeTab}
                  aria-label={tab.label}
                  data-tab-slot={index + 1}
                  data-tab-slug={tab.slug}
                  role="tab"
                  title={tab.label}
                  data-dq-action-managed="true"
                  onClick={() => setSearchParams((current) => {
                    const next = new URLSearchParams(current);
                    next.set('tab', tab.slug);
                    return next;
                  })}
                >
                  {tab.label}
                </button>
              ))}
            </nav>
            <Space className="taf-data-quality-toolbar-actions" size={6}>
              {isTopicHealthTab || isFlinkQualityTab || isFieldQualityTab || isStorageQualityTab || isReplayReconcileTab ? null : activeTab === 'report' ? (
                <>
                  <span className="taf-data-quality-report-toolbar-label">报告版本</span>
                  <Select size="small" value="v2026.06.26" options={[{ value: 'v2026.06.26' }, { value: 'v2026.06.25' }]} />
                </>
              ) : isVisualBreakdown ? (
                <>
                  <span className="taf-data-quality-toolbar-label">时间范围</span>
                  <Select size="small" value="近 24 小时" options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} />
                  <Tooltip title="刷新质量报表">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                  <Select size="small" value="30s" options={[{ value: '30s' }, { value: '60s' }, { value: '5min' }]} />
                  <Tooltip title="全屏查看">
                    <Button size="small" icon={<FullscreenOutlined />} />
                  </Tooltip>
                </>
              ) : (
                <>
                  <Select size="small" value="近 24 小时" options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} />
                  <Select size="small" value="全部管道" options={[{ value: '全部管道' }, { value: '采集链路' }, { value: '检测链路' }]} />
                  <Tooltip title="刷新质量报表">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                </>
              )}
              {activeTab !== 'report' && !isVisualBreakdown && <OverlayContractHost overlays={dataQualityOverlays} compact />}
            </Space>
          </header>

          {!isVisualBreakdown && isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/data-quality、APISIX 路由或 alert-service dataquality monitor。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <DataQualityFilterBar activeTab={activeTab} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} onRefresh={() => void refetch()} timeRange={timeRange} onTimeRangeChange={setTimeRange} />

          <div className={`taf-data-quality-kpis is-unified${activeTab === 'report' ? ' is-report' : ''}${activeTab === 'settings' ? ' is-settings' : ''}${isFlinkQualityTab ? ' is-flink' : ''}${isFieldQualityTab ? ' is-field' : ''}${isStorageQualityTab ? ' is-storage' : ''}${isReplayReconcileTab ? ' is-replay' : ''}`}>
            {metrics.map((metric, index) => (
              <DataQualityMetricTile key={metric.label} metric={metric} index={index} fieldKpiTrend={isFieldQualityTab ? fieldKpiTrends[index] : undefined} />
            ))}
          </div>

          <DataQualityTabView
            activeTab={activeTab}
            columns={columns}
            evidence={snapshot?.evidence ?? []}
            dataQualityVisuals={dataQualityVisuals}
            isLoading={!isVisualBreakdown && isLoading}
            onOpenFieldDetail={openFieldDetail}
            onOpenReplay={openDlqReplay}
            qualityScore={qualityScore}
            rows={rows}
          />
        </main>

        {activeTab === 'report' ? (
          <ReportSideRail onOpenReplay={openDlqReplay} />
        ) : activeTab === 'settings' ? (
          <SettingsSideRail />
        ) : activeTab === 'topic-health' ? (
          <TopicHealthSideRail onOpenDlqSample={() => setDlqSampleOpen(true)} onReplay={openDlqReplay} />
        ) : isFlinkQualityTab ? (
          <FlinkQualitySideRail />
        ) : isFieldQualityTab ? (
          <FieldQualitySideRail onOpenDetail={openFieldDetail} onOpenReplayReconcile={openReplayReconcile} />
        ) : isStorageQualityTab ? (
          <StorageQualitySideRail dataQualityVisuals={dataQualityVisuals} />
        ) : isReplayReconcileTab ? (
          <ReplayReconcileSideRail dataQualityVisuals={dataQualityVisuals} />
        ) : (
          <aside className="taf-data-quality-rail">
            <WorkPanel title="质量异常告警（近 24 小时）">
              <QualityAnomalies />
            </WorkPanel>
            <WorkPanel title="快速定位">
              <QuickLocate onOpenDlqSample={() => setDlqSampleOpen(true)} />
            </WorkPanel>
            <WorkPanel title="质量修复建议">
              <RepairAdvice onReplay={openDlqReplay} />
            </WorkPanel>
            <WorkPanel title="快收证据与报告">
              <EvidenceActions evidence={snapshot?.evidence ?? []} />
            </WorkPanel>
          </aside>
        )}
      </section>
      <Drawer
        className="taf-data-quality-dlq-sample-drawer"
        title="DLQ 样本详情"
        placement="right"
        width={dlqSampleDrawerWidth}
        open={dlqSampleOpen}
        closeIcon={<CloseOutlined title="关闭弹窗" />}
        onClose={() => setDlqSampleOpen(false)}
      >
        <Alert
          type="info"
          showIcon
          message="DLQ 样本只读预检"
          description={`影响范围：dlq.v1 待重放样本；${actionEndpointLabel(dlqReplayActionPlan)} 需要 ${actionScopeLabel(dlqReplayActionPlan)}，执行前必须完成 schema drift 校验、审批确认、幂等 key 和审计 trace。`}
        />
        <DenseRows columns={['任务', '来源', '数量', '状态', '策略']} rows={replayRows} />
        <DenseRows
          columns={['字段', '异常值', '样本窗口', '处置建议']}
          rows={[
            ['schema_version', '缺失', 'offset 12892-13021', '先验 schema'],
            ['tenant_id', '默认值', 'offset 13022-13046', '租户隔离复核'],
            ['event_time', '乱序', 'offset 13047-13092', '按窗口重排'],
          ]}
        />
      </Drawer>
      <Drawer
        className="taf-data-quality-field-detail-drawer"
        title={fieldDetail?.title ?? '字段质量详情'}
        placement="right"
        width={fieldDetailDrawerWidth}
        open={Boolean(fieldDetail)}
        closeIcon={<CloseOutlined title="关闭详情" />}
        onClose={() => setFieldDetail(null)}
      >
        {fieldDetail && (
          <>
            <Alert type="info" showIcon message={fieldDetail.title} description={fieldDetail.description} />
            <DenseRows columns={fieldDetail.columns} rows={fieldDetail.rows} />
            {fieldDetail.actionLabel && (
              <Button
                type="primary"
                block
                className="taf-data-quality-field-detail-action"
                onClick={() => {
                  message.success(fieldDetail.actionSuccessMessage ?? `${fieldDetail.actionLabel}已提交`);
                  setFieldDetail(null);
                }}
              >
                {fieldDetail.actionLabel}
              </Button>
            )}
          </>
        )}
      </Drawer>
      <Modal
        className="taf-data-quality-replay-modal"
        title="DLQ fallback replay dry-run"
        open={dlqReplayOpen}
        width={dlqReplayModalWidth}
        closeIcon={<CloseOutlined title="关闭弹窗" />}
        onCancel={() => setDlqReplayOpen(false)}
        onOk={() => dlqReplayForm.submit()}
        okText="执行 dry-run 预检"
        cancelText="关闭"
        confirmLoading={dlqReplayMutation.isPending}
      >
        <Alert
          type="warning"
          showIcon
          message="高风险重放动作默认只做 dry-run"
          description={`${actionEndpointLabel(dlqReplayActionPlan)} 会验证审批人、修复摘要、幂等键、scope 和审计链；切换执行模式前应先完成 dry-run 证据归档。`}
        />
        <Form
          className="taf-data-quality-replay-form"
          form={dlqReplayForm}
          layout="vertical"
          onFinish={submitDlqReplay}
        >
          <div className="taf-data-quality-replay-form-grid">
            <Form.Item label="审批人" name="approved_by" rules={[{ required: true, message: '请输入审批人' }]}>
              <Input placeholder="operator-2" />
            </Form.Item>
            <Form.Item label="审批单号" name="approval_id" rules={[{ required: true, message: '请输入审批单号' }]}>
              <Input placeholder="APPROVAL-20260629-DQ-001" />
            </Form.Item>
            <Form.Item label="幂等键" name="idempotency_key" rules={[{ required: true, message: '请输入幂等键' }]}>
              <Input placeholder="tenant-a:APPROVAL-20260629-DQ-001:dry-run" />
            </Form.Item>
            <Form.Item label="dry-run" name="dry_run" valuePropName="checked">
              <Switch checkedChildren="预检" unCheckedChildren="执行" />
            </Form.Item>
          </div>
          <Form.Item label="重放原因" name="reason" rules={[{ required: true, message: '请输入重放原因' }]}>
            <Input.TextArea rows={2} placeholder="schema repair 后验证 fallback 文件可安全回放" />
          </Form.Item>
          <Form.Item label="修复摘要" name="repair_summary" rules={[{ required: true, message: '请输入修复摘要' }]}>
            <Input.TextArea rows={2} placeholder="已修复 malformed event payload，先执行 dry-run 预检" />
          </Form.Item>
        </Form>
        {dlqReplayResult && (
          <div className="taf-data-quality-replay-result">
            <Alert
              type={dlqReplayResult.failed_files ? 'warning' : 'success'}
              showIcon
              message={`Replay ${dlqReplayResult.status}`}
              description={`replay_id=${dlqReplayResult.replay_id}，fallback 文件 ${dlqReplayResult.pre_fallback_files}，待重放字节 ${formatBytes(dlqReplayResult.pre_fallback_bytes)}，审计 ${dlqReplayResult.audit_trail.length} 条。`}
            />
          </div>
        )}
      </Modal>
    </div>
  );
}

function DataQualityMetricTile({
  fieldKpiTrend,
  metric,
  index,
}: {
  fieldKpiTrend?: number[];
  metric: PageSnapshot['metrics'][number];
  index: number;
}) {
  const up = !metric.delta.includes('↓') && !metric.delta.startsWith('-');
  const isScore = index === 0;
  const isReportScore = metric.label === '日报评分';
  const isSettingsScore = metric.label === '启用规则';
  const isTopicScore = metric.label === 'Topic 健康分';
  const isFlinkScore = metric.label === 'Flink 质量分';
  const isFieldScore = metric.label === '字段质量分';
  const isStorageScore = metric.label === '存储质量分';
  const isReplayScore = metric.label === '对账通过率';
  const trendValues = fieldKpiTrend ?? metricTrendFallback(metric.value, index);
  const scoreParts = isScore ? metric.value.match(/^(\d+(?:\.\d+)?)(.*)$/) : null;
  return (
    <div className={`taf-metric taf-data-quality-metric ${metricToneClass[metric.status]}${isScore ? ' is-score' : ''}${isReportScore ? ' is-report-score' : ''}${isSettingsScore ? ' is-settings-score' : ''}${isFlinkScore ? ' is-flink-score' : ''}${isFieldScore ? ' is-field-score' : ''}${isStorageScore ? ' is-storage-score' : ''}${isReplayScore ? ' is-replay-score' : ''}`} title={`${metric.label} ${metric.value} ${metric.delta}`}>
      <span>{metric.label}</span>
      <strong>
        {scoreParts ? (
          <>
            <b>{scoreParts[1]}</b>
            <em>{scoreParts[2].trim()}</em>
          </>
        ) : metric.value}
      </strong>
      <small>
        {up ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
        {metric.delta}
      </small>
      {!isScore && <DataQualityKpiSparklineChart ariaLabel={`${metric.label}趋势`} className="taf-data-quality-field-kpi-echart taf-data-quality-kpi-echart" tone={metric.status} values={trendValues} />}
      {isTopicScore && <SafetyCertificateOutlined className="taf-data-quality-topic-score-icon" />}
      {isFlinkScore && <SafetyCertificateOutlined className="taf-data-quality-flink-score-icon" />}
      {isFieldScore && <SafetyCertificateOutlined className="taf-data-quality-field-score-icon" />}
      {isStorageScore && <SafetyCertificateOutlined className="taf-data-quality-storage-score-icon" />}
      {isReplayScore && <SafetyCertificateOutlined className="taf-data-quality-replay-score-icon" />}
      {isReportScore && <SafetyCertificateOutlined className="taf-data-quality-report-score-icon" />}
      {isSettingsScore && <SafetyCertificateOutlined className="taf-data-quality-settings-score-icon" />}
    </div>
  );
}

function DataQualityFilterBar({
  activeTab,
  autoRefresh,
  onAutoRefreshChange,
  onRefresh,
  onTimeRangeChange,
  timeRange,
}: {
  activeTab: DataQualityTabSlug;
  autoRefresh: boolean;
  onAutoRefreshChange: (enabled: boolean) => void;
  onRefresh: () => void;
  onTimeRangeChange: (range: DataQualityTimeRange) => void;
  timeRange: DataQualityTimeRange;
}) {
  const activeTabLabel = dataQualityTabs.find((tab) => tab.slug === activeTab)?.label ?? '质量总览';
  const rangeLabel = '2025-06-25 15:30:45 ~ 2025-06-26 15:30:45';
  return (
    <div className="taf-data-quality-filterbar" data-tab={activeTab}>
      <span title="时间范围">时间范围</span>
      <Select<DataQualityTimeRange> size="small" value={timeRange} options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} onChange={onTimeRangeChange} />
      <button type="button" className="taf-data-quality-filter-range" title={`${activeTabLabel} ${rangeLabel}`}>{rangeLabel} <FieldTimeOutlined /></button>
      <span className="taf-data-quality-filter-spacer" />
      <span title="自动刷新">自动刷新</span>
      <button
        type="button"
        className={`taf-data-quality-auto-toggle${autoRefresh ? '' : ' is-off'}`}
        title={`自动刷新 已${autoRefresh ? '开启' : '关闭'}`}
        aria-label={`自动刷新 已${autoRefresh ? '开启' : '关闭'}`}
        aria-pressed={autoRefresh}
        data-dq-action-managed="true"
        onClick={() => onAutoRefreshChange(!autoRefresh)}
      ><span /></button>
      <Tooltip title={`刷新${activeTabLabel}数据`}>
        <Button size="small" icon={<ReloadOutlined />} data-dq-action-managed="true" onClick={onRefresh}>刷新</Button>
      </Tooltip>
    </div>
  );
}

function DataQualityTabView({
  activeTab,
  columns,
  dataQualityVisuals,
  evidence,
  isLoading,
  onOpenFieldDetail,
  onOpenReplay,
  qualityScore,
  rows,
}: {
  activeTab: DataQualityTabSlug;
  columns: ColumnsType<SnapshotRow>;
  dataQualityVisuals?: DataQualityVisuals;
  evidence: PageSnapshot['evidence'];
  isLoading: boolean;
  onOpenFieldDetail: OpenFieldQualityDetail;
  onOpenReplay: () => void;
  qualityScore: number;
  rows: SnapshotRow[];
}) {
  if (activeTab === 'topic-health') {
    return <TopicHealthContent columns={columns} dataQualityVisuals={dataQualityVisuals} isLoading={isLoading} qualityScore={qualityScore} rows={rows} />;
  }
  if (activeTab === 'flink-quality') {
    return <FlinkQualityContent dataQualityVisuals={dataQualityVisuals} />;
  }
  if (activeTab === 'field-quality') {
    return <FieldQualityContent dataQualityVisuals={dataQualityVisuals} onOpenDetail={onOpenFieldDetail} />;
  }
  if (activeTab === 'storage-quality') {
    return <StorageQualityContent dataQualityVisuals={dataQualityVisuals} />;
  }
  if (activeTab === 'replay-reconcile') {
    return <ReplayReconcileContent dataQualityVisuals={dataQualityVisuals} onOpenReplay={onOpenReplay} />;
  }
  if (activeTab === 'report') {
    return <ReportContent evidence={evidence} />;
  }
  if (activeTab === 'settings') {
    return <SettingsContent />;
  }
  return <QualityOverviewContent columns={columns} dataQualityVisuals={dataQualityVisuals} evidence={evidence} isLoading={isLoading} qualityScore={qualityScore} rows={rows} />;
}

function QualityOverviewContent({
  columns,
  dataQualityVisuals,
  evidence,
  isLoading,
  qualityScore,
  rows,
}: {
  columns: ColumnsType<SnapshotRow>;
  dataQualityVisuals?: DataQualityVisuals;
  evidence: PageSnapshot['evidence'];
  isLoading: boolean;
  qualityScore: number;
  rows: SnapshotRow[];
}) {
  return (
    <>
      <div className="taf-data-quality-overview-grid">
        <WorkPanel title="Kafka Topic 健康 (Top 10)" className="taf-data-quality-topic-panel taf-data-quality-overview-topic">
          <DataQualityTopicGrid isLoading={isLoading} rows={rows.slice(0, 7)} />
        </WorkPanel>
        <WorkPanel title="Topic 分区倾斜热力图" className="taf-data-quality-overview-heat" extra={<span className="taf-data-quality-score">{qualityScore.toFixed(0)} 分</span>}>
          <TopicHeatmap rows={rows} visuals={dataQualityVisuals} />
        </WorkPanel>
        <WorkPanel title="Flink 处理质量概览" className="taf-data-quality-overview-flink">
          <FlinkQuality score={qualityScore} evidence={evidence} visuals={dataQualityVisuals} />
        </WorkPanel>
        <WorkPanel title="字段质量矩阵（近 24 小时）" className="taf-data-quality-overview-field">
          <FieldQuality />
        </WorkPanel>
        <WorkPanel title="存储写入质量" className="taf-data-quality-overview-storage">
          <StorageQualityOverview rows={dataQualityVisuals?.storageComponentRows} />
        </WorkPanel>
        <WorkPanel title="对账报告（近 24 小时）" className="taf-data-quality-overview-reconcile">
          <ReconciliationReport />
        </WorkPanel>
      </div>
    </>
  );
}

const overviewTopicColumns = [
  'Topic',
  '分区数',
  '当前吞吐量',
  '消费延迟',
  '积压量',
  '积压趋势',
  '消费延迟 P95',
  '分区倾斜',
  '消息延迟 P95',
  '操作',
];

function DataQualityTopicGrid({ isLoading, rows }: { isLoading: boolean; rows: SnapshotRow[] }) {
  if (isLoading && rows.length === 0) {
    return <div className="taf-data-quality-topic-grid is-loading">加载 Topic 健康数据...</div>;
  }

  return (
    <div className="taf-data-quality-topic-grid" style={{ '--dq-topic-columns': overviewTopicColumns.length } as CSSProperties}>
      <div className="taf-data-quality-topic-grid-head">
        {overviewTopicColumns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {rows.map((row, index) => (
        <div key={String(row.Topic ?? index)} className="taf-data-quality-topic-grid-row">
          {overviewTopicColumns.map((column) => {
            const value = String(row[column] ?? '--');
            if (column === 'Topic') {
              return <strong key={column} title={value}>{value}</strong>;
            }
            if (column === '积压趋势') {
              return (
                <span key={column} className={`taf-data-quality-topic-trend ${value === '上升' ? 'is-risk' : value === '波动' ? 'is-warn' : 'is-ok'}`} title={value}>
                  <TopicSparkline index={index} tone={value} />
                  <em>{value}</em>
                </span>
              );
            }
            if (column === '操作') {
              return <span key={column} className={`taf-data-quality-topic-state ${value === '危急' ? 'is-risk' : value === '中等' ? 'is-warn' : 'is-ok'}`} title={value}>{value}</span>;
            }
            return <span key={column} title={value}>{value}</span>;
          })}
        </div>
      ))}
    </div>
  );
}

function TopicSparkline({ index, tone }: { index: number; tone: string }) {
  const base = [
    [58, 62, 59, 66, 63, 70, 65, 68, 64],
    [52, 49, 51, 46, 42, 39, 35, 32, 29],
    [61, 65, 63, 68, 71, 75, 79, 83, 87],
  ];
  const values = tone === '上升' ? base[2] : tone === '下降' ? base[1] : base[index % base.length];
  const status = tone === '上升' ? 'risk' : tone === '波动' ? 'warn' : 'ok';
  return <DataQualityKpiSparklineChart ariaLabel="Topic 趋势" className="taf-data-quality-topic-echart" tone={status} values={values} />;
}

function TopicHealthContent({
  columns,
  compact = false,
  dataQualityVisuals,
  isLoading,
  qualityScore,
  rows,
}: {
  columns: ColumnsType<SnapshotRow>;
  compact?: boolean;
  dataQualityVisuals?: DataQualityVisuals;
  isLoading: boolean;
  qualityScore: number;
  rows: SnapshotRow[];
}) {
  return (
    <>
      <div className="taf-data-quality-upper">
        <WorkPanel title="Kafka Topic 健康明细" className="taf-data-quality-topic-panel taf-data-quality-topic-health-table-panel">
          {isLoading && rows.length === 0 ? <div className="taf-data-quality-topic-grid is-loading">加载 Topic 健康数据...</div> : <TopicHealthTable rows={rows} />}
        </WorkPanel>

        <WorkPanel
          title="消费延迟趋势"
          className="taf-data-quality-trend-panel"
          extra={<span className="taf-data-quality-trend-legend">P50 / P95 / 阈值</span>}
        >
          <LatencyTrend />
        </WorkPanel>

        <WorkPanel title="分区倾斜热力图（flow_original）" className="taf-data-quality-topic-health-heat-panel" extra={<span className="taf-data-quality-score">{qualityScore.toFixed(0)} 分</span>}>
          <TopicHeatmap rows={rows} visuals={dataQualityVisuals} />
        </WorkPanel>
      </div>
      {!compact && (
        <div className="taf-data-quality-tab-grid">
          <WorkPanel title="Consumer Group 健康">
            <DenseRows
              columns={['Consumer Group', '当前 Lag', 'Rebalance', '最后提交', '状态']}
              rows={dataQualityVisuals?.consumerRows ?? consumerRows}
            />
          </WorkPanel>
          <WorkPanel title="消息大小吞吐分布（24h）">
            <MessageSizeDistribution visuals={dataQualityVisuals} />
          </WorkPanel>
          <WorkPanel title="异常分区处置队列">
            <PartitionQueue rows={dataQualityVisuals?.partitionQueueRows ?? [
              ['dlq_topic', '2', '消费延迟 18.2s', '下游消息异常积压', '消息组复位', '定位'],
              ['dlq_topic', '5', '消费延迟 15.1s', '消费链路处理堆积', '扩容消费者并优化处理', '修复'],
              ['threat_alerts', '7', '倾斜度 3.12', '分区数据分布不均', '评估扩分区重新分配数据', '评估'],
            ]} />
          </WorkPanel>
        </div>
      )}
    </>
  );
}

const topicHealthColumns = ['Topic', '分区数', '当前 offset', '积压', '消费延迟P95', '分区倾斜', '消息大小', '状态', '操作'];

function TopicHealthTable({ rows }: { rows: SnapshotRow[] }) {
  const pageSize = 7;
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const [page, setPage] = useState(1);
  const currentPage = Math.min(page, totalPages);
  const visibleRows = rows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return (
    <div className="taf-data-quality-topic-health-table" style={{ '--dq-topic-health-columns': topicHealthColumns.length } as CSSProperties}>
      <div className="taf-data-quality-topic-health-head">
        {topicHealthColumns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {visibleRows.map((row) => (
        <div key={String(row.Topic)} className="taf-data-quality-topic-health-row">
          {topicHealthColumns.map((column) => {
            const rawValue = column === '分区倾斜' ? row.分区倾斜度 : row[column];
            const value = String(rawValue ?? '--');
            if (column === 'Topic') return <strong key={column} title={value}>{value}</strong>;
            if (column === '状态') return <span key={column} className={`taf-data-quality-topic-health-state ${value === '严重' ? 'is-risk' : value === '告警' ? 'is-warn' : 'is-ok'}`} title={value}>{value}</span>;
            if (column === '操作') return <span key={column} className="taf-data-quality-topic-health-op" title={`查看 ${String(row.Topic)}`}><FileSearchOutlined /></span>;
            return <span key={column} className={(column.includes('延迟') || column === '分区倾斜') && (value.includes('15') || value.includes('2.') || value.includes('5.')) ? 'is-warn' : ''} title={value}>{value}</span>;
          })}
        </div>
      ))}
      <div className="taf-data-quality-topic-health-footer">
        <span>共 {rows.length} 条</span>
        <button type="button" title="上一页" aria-label="Topic 健康上一页" data-dq-action-managed="true" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)}><LeftOutlined /></button>
        {Array.from({ length: totalPages }, (_, index) => index + 1).map((item) => <button key={item} type="button" data-dq-action-managed="true" className={item === currentPage ? 'is-active' : ''} aria-current={item === currentPage ? 'page' : undefined} onClick={() => setPage(item)}>{item}</button>)}
        <button type="button" title="下一页" aria-label="Topic 健康下一页" data-dq-action-managed="true" disabled={currentPage === totalPages} onClick={() => setPage(currentPage + 1)}><RightOutlined /></button>
        <span>{pageSize} 条/页</span>
      </div>
    </div>
  );
}

function FlinkQualityContent({
  dataQualityVisuals,
}: {
  dataQualityVisuals?: DataQualityVisuals;
}) {
  return (
    <>
      <div className="taf-data-quality-flink-upper">
        <WorkPanel title="Flink 作业健康明细">
          <FlinkJobHealthTable rows={dataQualityVisuals?.flinkJobRows} />
        </WorkPanel>
        <WorkPanel title="Checkpoint 与 Watermark 趋势">
          <FlinkCheckpointWatermarkTrend trend={dataQualityVisuals?.flinkCheckpointTrend} />
        </WorkPanel>
        <WorkPanel title="Backpressure 热力图 (按作业 / Subtask)">
          <FlinkBackpressureHeatmap buckets={dataQualityVisuals?.flinkBackpressureBuckets} rows={dataQualityVisuals?.flinkBackpressureRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-flink-lower">
        <WorkPanel title="迟到数据与窗口闭合（按来源 Topic）">
          <LateWindowClosure topicRows={dataQualityVisuals?.flinkLateTopicRows} windowRows={dataQualityVisuals?.flinkWindowRows} />
        </WorkPanel>
        <WorkPanel title="异常与失败原因（Top 10）">
          <FlinkFailureTable rows={dataQualityVisuals?.flinkFailureRows} />
        </WorkPanel>
        <WorkPanel title="Sink 写入质量（近 24h）">
          <SinkQualityCards rows={dataQualityVisuals?.flinkSinkRows} />
        </WorkPanel>
      </div>
    </>
  );
}

const flinkJobColumns = ['作业', '状态', '并行度', 'Checkpoint', 'Watermark P95', 'Backpressure', '迟到率', '异常数', 'Sink 状态', '操作'];

function FlinkJobHealthTable({ rows }: { rows?: DataQualityVisuals['flinkJobRows'] }) {
  const jobRows = rows?.length ? rows : flinkJobFallbackRows;
  const pageSize = 5;
  const totalPages = Math.max(1, Math.ceil(jobRows.length / pageSize));
  const [page, setPage] = useState(1);
  const currentPage = Math.min(page, totalPages);
  const visibleRows = jobRows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return (
    <div className="taf-data-quality-flink-job-table">
      <div className="taf-data-quality-flink-job-head">
        {flinkJobColumns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {visibleRows.map((row) => {
        const tone = row[1] === '背压中' ? 'warn' : row[1] === '重启中' ? 'info' : 'ok';
        return (
          <div key={row[0]} className={`taf-data-quality-flink-job-row is-${tone}`} title={row.join(' ')}>
            {row.map((cell, index) => {
              if (index === 0) return <strong key={`${row[0]}-${index}`} title={cell}>{cell}</strong>;
              if (index === 1) return <span key={`${row[0]}-${index}`} className="taf-data-quality-flink-job-state" title={cell}>{cell}</span>;
              return <span key={`${row[0]}-${index}`} title={cell}>{cell}</span>;
            })}
            <span className="taf-data-quality-flink-job-actions" title={`查看 ${row[0]}`}>
              <FileSearchOutlined />
              <SearchOutlined />
            </span>
          </div>
        );
      })}
      <div className="taf-data-quality-flink-job-footer">
        <span>共 {jobRows.length} 条</span>
        <button type="button" title="上一页" aria-label="Flink 作业上一页" data-dq-action-managed="true" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)}><LeftOutlined /></button>
        {Array.from({ length: totalPages }, (_, index) => index + 1).map((item) => <button key={item} type="button" data-dq-action-managed="true" className={item === currentPage ? 'is-active' : ''} aria-current={item === currentPage ? 'page' : undefined} onClick={() => setPage(item)}>{item}</button>)}
        <button type="button" title="下一页" aria-label="Flink 作业下一页" data-dq-action-managed="true" disabled={currentPage === totalPages} onClick={() => setPage(currentPage + 1)}><RightOutlined /></button>
        <span>{pageSize} 条/页</span>
      </div>
    </div>
  );
}

function FlinkCheckpointWatermarkTrend({ trend }: { trend?: DataQualityVisuals['flinkCheckpointTrend'] }) {
  const chart = trend ?? flinkCheckpointTrendFallback;
  return (
    <div className="taf-data-quality-flink-checkpoint-trend">
      <div className="taf-data-quality-flink-trend-legend">
        {[
          ['Checkpoint 时长 (s)', 'duration'],
          ['Checkpoint Age (s)', 'age'],
          ['Watermark 延迟 P95 (s)', 'watermark'],
          ['Watermark SLA 阈值 (s)', 'watermark-sla'],
          ['Checkpoint SLA 阈值 (s)', 'checkpoint-sla'],
        ].map(([label, tone]) => <span key={label} className={`is-${tone}`} title={label}>{label}</span>)}
      </div>
      <DataQualityTrendChart
        ariaLabel="Checkpoint 与 Watermark 趋势"
        className="taf-data-quality-flink-checkpoint-echart"
        categories={chart.times}
        series={[
          { name: 'Checkpoint 时长', color: '#18a8ff', values: chart.checkpointDuration },
          { name: 'Checkpoint Age', color: '#40d98a', values: chart.checkpointAge },
          { name: 'Watermark P95', color: '#ffb020', values: chart.watermarkP95 },
          { name: 'Watermark SLA', color: '#ff4d4f', dashed: true, values: chart.watermarkSla },
          { name: 'Checkpoint SLA', color: '#a78bfa', dashed: true, values: chart.checkpointSla },
        ]}
        valueFormatter={(value) => `${value}s`}
      />
    </div>
  );
}

function FlinkBackpressureHeatmap({
  buckets,
  rows,
}: {
  buckets?: string[];
  rows?: DataQualityVisuals['flinkBackpressureRows'];
}) {
  const heatRows = rows?.length ? rows : flinkBackpressureFallbackRows;
  const bucketLabels = buckets?.length ? buckets : flinkBackpressureFallbackBuckets;
  return (
    <div className="taf-data-quality-flink-backpressure">
      <div className="taf-data-quality-flink-backpressure-head">
        <span>Subtask</span>
        {bucketLabels.map((bucket) => <em key={bucket} title={bucket}>{bucket}</em>)}
      </div>
      {heatRows.map((row) => (
        <div key={row.label} className="taf-data-quality-flink-backpressure-row" title={`${row.label} Backpressure`}>
          <strong title={row.label}>{row.label}</strong>
          <div>
            {row.values.map((value, index) => <i key={`${row.label}-${index}`} className={`is-${value}`} title={`${row.label} subtask ${index} ${value}`} />)}
          </div>
        </div>
      ))}
      <div className="taf-data-quality-flink-backpressure-legend">
        <span><i className="is-ok" />空闲 (0-0.1)</span>
        <span><i className="is-warn" />轻度背压 (0.1-0.5)</span>
        <span><i className="is-risk" />严重背压 (&gt;0.5)</span>
      </div>
    </div>
  );
}

function LateWindowClosure({
  topicRows,
  windowRows,
}: {
  topicRows?: DataQualityVisuals['flinkLateTopicRows'];
  windowRows?: DataQualityVisuals['flinkWindowRows'];
}) {
  const topics = topicRows?.length ? topicRows : flinkLateTopicFallbackRows;
  const windows = windowRows?.length ? windowRows : flinkWindowFallbackRows;
  return (
    <div className="taf-data-quality-flink-late-window">
      <div className="taf-data-quality-flink-late-bars">
        <div className="taf-data-quality-flink-late-legend">
          <span><i className="is-normal" />正常事件</span>
          <span><i className="is-late" />迟到事件 (side-output)</span>
          <span><i className="is-severe" />丢弃事件</span>
        </div>
        {topics.map(([topic, normal, late, dropped], index) => {
          const normalWidth = [60, 61, 58, 59, 57, 56][index] ?? 58;
          const lateWidth = [28, 27, 29, 28, 30, 31][index] ?? 29;
          const droppedWidth = Math.max(8, 100 - normalWidth - lateWidth);
          return (
            <div key={topic} className="taf-data-quality-flink-late-row" title={`${topic} 正常 ${normal} 迟到 ${late} 丢弃 ${dropped}`}>
              <strong title={topic}>{topic}</strong>
              <div>
                <span className="is-normal" style={{ width: `${normalWidth}%` }}>{normal}</span>
                <span className="is-late" style={{ width: `${lateWidth}%` }}>{late}</span>
                <span className="is-severe" style={{ width: `${droppedWidth}%` }}>{dropped}</span>
              </div>
            </div>
          );
        })}
      </div>
      <div className="taf-data-quality-flink-window-table">
        <div>
          <span title="窗口大小">窗口大小</span>
          <span title="窗口闭合延迟 P95">窗口闭合延迟 P95</span>
          <span title="丢弃率">丢弃率</span>
        </div>
        {windows.map((row) => (
          <div key={row[0]} title={row.join(' ')}>
            {row.map((cell) => <span key={cell} title={cell}>{cell}</span>)}
          </div>
        ))}
      </div>
    </div>
  );
}

function FlinkFailureTable({ rows }: { rows?: DataQualityVisuals['flinkFailureRows'] }) {
  const failureRows = rows?.length ? rows : flinkFailureFallbackRows;
  const columns = ['异常类型', '作业', '算子 UID', '次数', '首次发生', '最近发生', '建议处理'];
  return (
    <div className="taf-data-quality-flink-failure-table">
      <div>
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {failureRows.map((row) => (
        <button key={`${row[0]}-${row[2]}`} type="button" title={row.join(' ')}>
          {row.map((cell) => <span key={cell} title={cell}>{cell}</span>)}
        </button>
      ))}
      <footer>
        <button type="button" title="查看更多异常与失败原因">查看更多 <ArrowUpOutlined /></button>
      </footer>
    </div>
  );
}

function SinkQualityCards({ rows }: { rows?: DataQualityVisuals['flinkSinkRows'] }) {
  const sinkRows = rows?.length ? rows : flinkSinkFallbackRows;
  return (
    <div className="taf-data-quality-flink-sinks">
      {sinkRows.map((sink) => (
        <section key={sink.name} title={`${sink.name} ${sink.status} 写入 EPS ${sink.eps} 成功率 ${sink.success} P95 写入延迟 ${sink.p95} 重试次数 ${sink.retries}`}>
          <header>
            <strong title={sink.name}>{sink.name}</strong>
            <span><CheckCircleOutlined /> {sink.status}</span>
          </header>
          <p><span>写入 EPS</span><b>{sink.eps}</b></p>
          <p><span>成功率</span><b>{sink.success}</b></p>
          <p><span>P95 写入延迟</span><b>{sink.p95}</b></p>
          <p><span>重试次数</span><b>{sink.retries}</b></p>
          <MiniTrend values={sink.trend} />
        </section>
      ))}
    </div>
  );
}

function MiniTrend({ values }: { values: number[] }) {
  return <DataQualityKpiSparklineChart ariaLabel="Sink 写入趋势" className="taf-data-quality-flink-mini-echart" tone="ok" values={values} />;
}

function FieldQualityContent({
  dataQualityVisuals,
  onOpenDetail,
}: {
  dataQualityVisuals?: DataQualityVisuals;
  onOpenDetail: OpenFieldQualityDetail;
}) {
  return (
    <>
      <div className="taf-data-quality-field-upper">
        <WorkPanel title="关键字段质量矩阵">
          <FieldQualityMatrix rows={dataQualityVisuals?.fieldQualityRows} />
        </WorkPanel>
        <WorkPanel title="字段异常趋势（近 24 小时）">
          <FieldAnomalyTrend trend={dataQualityVisuals?.fieldTrend} summary={dataQualityVisuals?.fieldTrendSummary} />
        </WorkPanel>
        <WorkPanel title="五元组与 community_id 校验">
          <CommunityIdCheck rows={dataQualityVisuals?.communityCheckRows} mismatches={dataQualityVisuals?.communityMismatchRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-field-lower">
        <WorkPanel className="taf-data-quality-field-samples-panel" title="异常样本表（按影响时间排序）">
          <FieldAnomalySampleTable rows={dataQualityVisuals?.fieldAnomalyRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
        <WorkPanel title="字段血缘与映射">
          <FieldLineageMapping rows={dataQualityVisuals?.fieldLineageRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
        <WorkPanel className="taf-data-quality-field-repairs-panel" title="修复任务与规则建议">
          <FieldRepairTasks rows={dataQualityVisuals?.fieldRepairRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
      </div>
    </>
  );
}

function fieldQualityTone(value: string) {
  if (value === '--') return 'na';
  const numeric = Number.parseFloat(value);
  if (Number.isNaN(numeric)) return 'info';
  if (numeric < 95) return 'risk';
  if (numeric < 98) return 'warn';
  return 'ok';
}

function fieldTaskStatusClass(value: string) {
  if (value.includes('已完成')) return 'is-ok';
  if (value.includes('进行中')) return 'is-info';
  if (value.includes('待检查')) return 'is-warn';
  return 'is-risk';
}

function FieldQualityMatrix({ rows }: { rows?: DataQualityVisuals['fieldQualityRows'] }) {
  const matrixRows = rows?.length ? rows : fieldQualityFallbackRows;
  const columns = ['字段', '完整性', '格式', '枚举', '跨表一致', '时序', '来源血缘'];
  return (
    <div className="taf-data-quality-field-matrix">
      <div className="taf-data-quality-field-matrix-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {matrixRows.map((row) => (
        <div key={row[0]} className="taf-data-quality-field-matrix-row" title={row.join(' ')}>
          <strong title={row[0]}>{row[0]}</strong>
          {row.slice(1).map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={`is-${fieldQualityTone(cell)}`} title={`${columns[index + 1]} ${cell}`}>{cell}</span>
          ))}
        </div>
      ))}
      <footer>
        <span><i className="is-ok" />优秀 (&gt;=98%)</span>
        <span><i className="is-warn" />中等 (95%-98%)</span>
        <span><i className="is-risk" />较差 (&lt;95%)</span>
        <span><i className="is-na" />-- 不适用</span>
      </footer>
    </div>
  );
}

const fieldTrendSeries = [
  ['缺失值', 'missing', '#1890ff'],
  ['格式不合法', 'format', '#35d06f'],
  ['映射不一致', 'mapping', '#f59e0b'],
  ['时间漂移', 'timeDrift', '#ff4d4f'],
  ['未知协议', 'unknownProtocol', '#2f80ed'],
] as const;

function FieldAnomalyTrend({
  summary,
  trend,
}: {
  summary?: DataQualityVisuals['fieldTrendSummary'];
  trend?: DataQualityVisuals['fieldTrend'];
}) {
  const chart = trend ?? fieldTrendFallback;
  const summaryRows = summary?.length ? summary : fieldTrendSummaryFallback;
  return (
    <div className="taf-data-quality-field-trend-panel">
      <div className="taf-data-quality-field-trend-legend">
        {fieldTrendSeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}><i />{label}</span>
        ))}
      </div>
      <DataQualityFieldTrendChart
        ariaLabel="字段异常趋势"
        threshold={2000}
        times={chart.times}
        series={fieldTrendSeries.map(([name, key, color]) => ({ name, color, values: chart[key] }))}
      />
      <footer>
        {chart.times.map((time) => <span key={time} title={time}>{time}</span>)}
      </footer>
      <div className="taf-data-quality-field-trend-summary">
        {summaryRows.map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`} title={`${label} ${value}`}><em>{label}</em><strong>{value}</strong></span>
        ))}
      </div>
    </div>
  );
}

function CommunityIdCheck({
  mismatches,
  rows,
}: {
  mismatches?: DataQualityVisuals['communityMismatchRows'];
  rows?: DataQualityVisuals['communityCheckRows'];
}) {
  const checkRows = rows?.length ? rows : fieldCommunityCheckFallbackRows;
  const mismatchRows = mismatches?.length ? mismatches : fieldCommunityMismatchFallbackRows;
  return (
    <div className="taf-data-quality-community-check">
      <div className="taf-data-quality-community-summary">
        {[
          ['校验状态', '正常', 'ok'],
          ['匹配率', '97.1%', 'ok'],
          ['不匹配数', '512', 'risk'],
          ['哈希碰撞告警', '3', 'risk'],
        ].map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`} title={`${label} ${value}`}><em>{label}</em><strong>{value}</strong></span>
        ))}
      </div>
      <div className="taf-data-quality-community-flow">
        <span title="五元组 src_ip dst_ip src_port dst_port protocol">五元组<small>src_ip<br />dst_ip<br />src_port<br />dst_port<br />protocol</small></span>
        <i />
        <span title="社区 ID 计算 SHA-1">社区 ID 计算<small>社区哈希 (SHA-1)</small></span>
        <i />
        <span title="community_id">community_id</span>
      </div>
      <div className="taf-data-quality-community-detail">
        <div>{['校验明细', '总记录数', '匹配数', '不匹配数', '匹配率'].map((column) => <span key={column} title={column}>{column}</span>)}</div>
        {checkRows.map((row) => (
          <div key={row[0]} title={row.join(' ')}>
            {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 3 ? 'is-risk' : ''} title={cell}>{cell}</span>)}
          </div>
        ))}
      </div>
      <div className="taf-data-quality-community-mismatch">
        <h4>不匹配样例（Top 5）</h4>
        <div>{['时间', 'session_id', 'src_ip:src_port', 'dst_ip:dst_port', 'protocol', '计算 cid', '原始 cid', '原因'].map((column) => <span key={column} title={column}>{column}</span>)}</div>
        {mismatchRows.map((row) => (
          <div key={`${row[0]}-${row[1]}`} title={row.join(' ')}>
            {row.map((cell, index) => <span key={`${row[1]}-${index}`} className={index === 7 ? 'is-risk' : ''} title={cell}>{cell}</span>)}
          </div>
        ))}
      </div>
    </div>
  );
}

function FieldAnomalySampleTable({
  onOpenDetail,
  rows,
}: {
  onOpenDetail: OpenFieldQualityDetail;
  rows?: DataQualityVisuals['fieldAnomalyRows'];
}) {
  const sampleRows = rows?.length ? rows : fieldAnomalyFallbackRows;
  const columns = ['时间', 'Topic', '字段', '异常类型', '原始值', '归一化值', '影响资产', '处置'];
  const pageSize = 5;
  const totalPages = Math.max(1, Math.ceil(sampleRows.length / pageSize));
  const [page, setPage] = useState(1);
  const currentPage = Math.min(page, totalPages);
  const visibleRows = sampleRows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return (
    <div className="taf-data-quality-field-sample-table">
      <div className="taf-data-quality-field-table-head">{columns.map((column) => <span key={column} title={column}>{column}</span>)}</div>
      <div className="taf-data-quality-field-table-rows" aria-label="字段异常样本">
        {visibleRows.map((row) => (
          <button
            key={`${row[0]}-${row[2]}-${row[4]}`}
            type="button"
            className="taf-data-quality-field-table-row"
            title={row.join(' ')}
            onClick={() => onOpenDetail({
              title: `字段异常详情：${row[2]}`,
              description: `${row[1]} 在 ${row[0]} 发现 ${row[3]}；当前处置建议为 ${row[7]}。`,
              columns,
              rows: [row],
              actionLabel: row[7] === '创建任务' ? '创建字段修复任务' : undefined,
              actionSuccessMessage: row[7] === '创建任务' ? `已为 ${row[2]} 创建字段修复任务` : undefined,
            })}
          >
            {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 3 ? 'is-risk' : index === 7 ? 'is-link' : ''} title={cell}>{cell}</span>)}
          </button>
        ))}
      </div>
      <FieldTablePagination currentPage={currentPage} label="异常样本" onChange={setPage} total={sampleRows.length} totalPages={totalPages} />
    </div>
  );
}

function FieldLineageMapping({
  onOpenDetail,
  rows,
}: {
  onOpenDetail: OpenFieldQualityDetail;
  rows?: DataQualityVisuals['fieldLineageRows'];
}) {
  const lineageRows = rows?.length ? rows : fieldLineageFallbackRows;
  const columns = ['数据源（Kafka Topic）', '处理链路（Flink）', '归一化映射', '数据落地（Sink）'];
  return (
    <div className="taf-data-quality-field-lineage">
      <div className="taf-data-quality-field-lineage-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {lineageRows.map(([source, flink, mapping, sink, tone]) => (
        <div key={source} className={`is-${tone}`} title={`${source} ${flink} ${mapping} ${sink}`}>
          {[source, flink, mapping, sink].map((cell) => <span key={cell} title={cell}>{cell}</span>)}
        </div>
      ))}
      <footer>
        <span><i className="is-ok" />正常</span>
        <span><i className="is-warn" />警告</span>
        <span><i className="is-risk" />异常</span>
        <button
          type="button"
          onClick={() => onOpenDetail({
            title: '字段映射修复任务',
            description: '已识别到异常映射链路；创建后将进入字段质量修复队列并保留审计信息。',
            columns: ['数据源', '处理链路', '建议动作'],
            rows: [['traffic_session_raw', '会话构建', '创建字段映射修复任务']],
            actionLabel: '创建字段修复任务',
            actionSuccessMessage: '字段映射修复任务已创建',
          })}
        >当前链路存在异常映射，建议创建修复任务</button>
      </footer>
    </div>
  );
}

function FieldRepairTasks({
  onOpenDetail,
  rows,
}: {
  onOpenDetail: OpenFieldQualityDetail;
  rows?: DataQualityVisuals['fieldRepairRows'];
}) {
  const repairRows = rows?.length ? rows : fieldRepairFallbackRows;
  const columns = ['任务名称', '异常字段', '建议映射 / 修复规则', '负责人', '状态', 'SLA', '验证结果', '操作'];
  const pageSize = 5;
  const totalPages = Math.max(1, Math.ceil(repairRows.length / pageSize));
  const [page, setPage] = useState(1);
  const currentPage = Math.min(page, totalPages);
  const visibleRows = repairRows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return (
    <div className="taf-data-quality-field-repair-table">
      <div className="taf-data-quality-field-table-head">{columns.map((column) => <span key={column} title={column}>{column}</span>)}</div>
      <div className="taf-data-quality-field-table-rows" aria-label="字段修复任务">
        {visibleRows.map((row) => (
          <button
            key={`${row[0]}-${row[1]}`}
            type="button"
            className="taf-data-quality-field-table-row"
            title={row.join(' ')}
            onClick={() => onOpenDetail({
              title: `字段修复任务：${row[0]}`,
              description: `${row[1]} 的修复规则由 ${row[3]} 负责，当前状态为 ${row[4]}。`,
              columns,
              rows: [row],
              actionLabel: row[7] === '创建' ? '创建修复任务' : undefined,
              actionSuccessMessage: row[7] === '创建' ? `${row[0]} 已创建并进入待处理队列` : undefined,
            })}
          >
            {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 4 ? fieldTaskStatusClass(cell) : index === 6 && cell === '通过' ? 'is-ok' : index === 7 ? 'is-link' : ''} title={cell}>{cell}</span>)}
          </button>
        ))}
      </div>
      <FieldTablePagination currentPage={currentPage} label="修复任务" onChange={setPage} total={repairRows.length} totalPages={totalPages} />
    </div>
  );
}

function FieldTablePagination({
  currentPage,
  label,
  onChange,
  total,
  totalPages,
}: {
  currentPage: number;
  label: string;
  onChange: (page: number) => void;
  total: number;
  totalPages: number;
}) {
  return (
    <footer className="taf-data-quality-field-pagination" aria-label={`${label}分页`}>
      <span>共 {total} 条</span>
      <div>
        <Tooltip title="上一页">
          <button type="button" aria-label={`${label}上一页`} disabled={currentPage === 1} onClick={() => onChange(currentPage - 1)}><LeftOutlined /></button>
        </Tooltip>
        {Array.from({ length: totalPages }, (_, index) => index + 1).map((page) => (
          <button key={page} type="button" aria-current={page === currentPage ? 'page' : undefined} onClick={() => onChange(page)}>{page}</button>
        ))}
        <Tooltip title="下一页">
          <button type="button" aria-label={`${label}下一页`} disabled={currentPage === totalPages} onClick={() => onChange(currentPage + 1)}><RightOutlined /></button>
        </Tooltip>
      </div>
    </footer>
  );
}

function StorageQualityContent({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  return (
    <>
      <div className="taf-data-quality-storage-upper">
        <WorkPanel title="存储组件健康总览">
          <StorageComponentHealthTable rows={dataQualityVisuals?.storageComponentRows} />
        </WorkPanel>
        <WorkPanel title="写入速率与延迟趋势（近 24 小时）">
          <StorageWriteTrend trend={dataQualityVisuals?.storageTrend} />
        </WorkPanel>
        <WorkPanel title="容量与水位趋势（近 7 天）">
          <StorageCapacityTrend trend={dataQualityVisuals?.storageCapacityTrend} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-storage-lower">
        <WorkPanel title="失败写入与原因列表（近 24 小时）">
          <StorageFailureTable rows={dataQualityVisuals?.storageFailureRows} />
        </WorkPanel>
        <WorkPanel title="索引与归档链路（写入链路全景）">
          <StoragePipelineFlow rows={dataQualityVisuals?.storagePipelineRows} />
        </WorkPanel>
        <WorkPanel title="副本、分片与对象健康">
          <StorageReplicaHealth
            indexHealth={dataQualityVisuals?.storageIndexHealth}
            objectRows={dataQualityVisuals?.storageObjectRows}
            partitionRows={dataQualityVisuals?.storagePartitionRows}
            replicaRows={dataQualityVisuals?.storageReplicaRows}
          />
        </WorkPanel>
      </div>
    </>
  );
}

function storageStatusClass(value: string | undefined) {
  if (!value) return 'info';
  if (value.includes('警告') || value.includes('高') || value.includes('异常') || value.includes('red')) return 'risk';
  if (value.includes('注意') || value.includes('重试') || value.includes('进行中') || value.includes('lag')) return 'warn';
  if (value.includes('正常') || value.includes('已结束')) return 'ok';
  return 'info';
}

function StorageComponentHealthTable({ rows }: { rows?: DataQualityVisuals['storageComponentRows'] }) {
  const tableRows = rows?.length ? rows : storageComponentFallbackRows;
  const columns = ['组件', '状态', '写入速率', '成功率', 'P95 延迟', '积压/队列', '容量', '副本/分片', '操作'];
  return (
    <div className="taf-data-quality-storage-component-table">
      <div className="taf-data-quality-storage-component-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {tableRows.map((row) => (
        <button key={row[0]} type="button" className={`is-${storageStatusClass(row[1])}`} title={row.join(' ')}>
          <strong title={row[0]}>{row[0]}</strong>
          {row.slice(1).map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 0 ? `is-${storageStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>{cell}</span>
          ))}
        </button>
      ))}
    </div>
  );
}

const storageTrendSeries = [
  ['ClickHouse EPS', 'clickhouse', '#2f80ed'],
  ['OpenSearch docs/s', 'opensearch', '#00d6d6'],
  ['NebulaGraph edges/s', 'nebula', '#4ade80'],
  ['MinIO objects/s', 'minio', '#b85cff'],
  ['P95 延迟(毫秒)', 'latencyP95', '#ff7875'],
  ['延迟 SLA', 'latencySla', '#faad14'],
] as const;

function buildStorageLinePoints(values: number[], maxValue: number, width = 420, height = 142, left = 34, top = 12) {
  return values.map((value, index) => {
    const x = left + (index / Math.max(values.length - 1, 1)) * width;
    const y = top + height - (value / Math.max(maxValue, 1)) * height;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(' ');
}

function StorageWriteTrend({ trend }: { trend?: DataQualityVisuals['storageTrend'] }) {
  const chart = trend ?? storageTrendFallback;
  return (
    <div className="taf-data-quality-storage-trend">
      <div className="taf-data-quality-storage-trend-legend">
        {storageTrendSeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}><i />{label}</span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="写入速率与延迟趋势"
        className="taf-data-quality-storage-echart"
        categories={chart.times}
        series={storageTrendSeries.map(([name, key, color]) => ({ name, color, values: chart[key], dashed: key === 'latencySla' }))}
      />
    </div>
  );
}

const storageCapacitySeries = [
  ['ClickHouse 容量', 'clickhouse', '#2f80ed'],
  ['OpenSearch 索引', 'opensearch', '#00d6d6'],
  ['NebulaGraph 分区', 'nebula', '#4ade80'],
  ['MinIO Bucket', 'minio', '#b85cff'],
  ['容量阈值', 'threshold', '#faad14'],
] as const;

function buildStorageAreaPoints(values: number[], maxValue: number) {
  const left = 34;
  const top = 12;
  const width = 420;
  const height = 142;
  const linePoints = values.map((value, index) => {
    const x = left + (index / Math.max(values.length - 1, 1)) * width;
    const y = top + height - (value / Math.max(maxValue, 1)) * height;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  return `${left},${top + height} ${linePoints.join(' ')} ${left + width},${top + height}`;
}

function StorageCapacityTrend({ trend }: { trend?: DataQualityVisuals['storageCapacityTrend'] }) {
  const chart = trend ?? storageCapacityFallback;
  return (
    <div className="taf-data-quality-storage-capacity">
      <div className="taf-data-quality-storage-trend-legend">
        {storageCapacitySeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}><i />{label}</span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="容量与水位趋势"
        className="taf-data-quality-storage-echart"
        categories={chart.days}
        series={storageCapacitySeries.map(([name, key, color]) => ({ name, color, values: chart[key], dashed: key === 'threshold', area: key !== 'threshold' }))}
        valueFormatter={(value) => `${value}%`}
      />
    </div>
  );
}

function StorageFailureTable({ rows }: { rows?: DataQualityVisuals['storageFailureRows'] }) {
  const failureRows = rows?.length ? rows : storageFailureFallbackRows;
  const columns = ['时间', '组件', '目标表/索引/Bucket', '失败原因', '影响记录', '重试', '状态', '处置'];
  return (
    <div className="taf-data-quality-storage-failure-table">
      <div>{columns.map((column) => <span key={column} title={column}>{column}</span>)}</div>
      {failureRows.map((row) => (
        <button key={`${row[0]}-${row[1]}-${row[2]}`} type="button" className={`is-${storageStatusClass(row[6])}`} title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 6 ? `is-${storageStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>{cell}</span>
          ))}
        </button>
      ))}
    </div>
  );
}

function StoragePipelineFlow({ rows }: { rows?: DataQualityVisuals['storagePipelineRows'] }) {
  const flowRows = rows?.length ? rows : storagePipelineFallbackRows;
  const storageNodes = flowRows.filter((row) => row.from === 'Kafka / Flink').map((row) => row.to);
  const resultNodes = Array.from(new Set(flowRows.filter((row) => row.from !== 'Kafka / Flink').map((row) => row.to)));
  const nodeStatus = (label: string) => flowRows.find((row) => row.to === label || row.from === label)?.status ?? 'info';
  return (
    <div className="taf-data-quality-storage-flow" title="Kafka/Flink 到 ClickHouse、OpenSearch、NebulaGraph、MinIO 的写入链路全景">
      <section className="is-source">
        <StoragePipelineNode label="Kafka / Flink" detail="批量 Sink / Exactly-once" status="info" />
      </section>
      <section>
        {storageNodes.map((node) => (
          <StoragePipelineNode key={node} label={node} detail={flowRows.find((row) => row.to === node)?.label ?? '写入'} status={nodeStatus(node)} />
        ))}
      </section>
      <section>
        {resultNodes.map((node) => (
          <StoragePipelineNode key={node} label={node} detail={flowRows.find((row) => row.to === node)?.label ?? '状态'} status={nodeStatus(node)} />
        ))}
      </section>
      <footer>
        {flowRows.map((edge) => (
          <span key={`${edge.from}-${edge.to}`} className={`is-${edge.status}`} title={`${edge.from} → ${edge.to} ${edge.label}`}>
            {edge.from} → {edge.to}
          </span>
        ))}
      </footer>
    </div>
  );
}

function StoragePipelineNode({ detail, label, status }: { detail: string; label: string; status: 'ok' | 'info' | 'warn' | 'risk' }) {
  return (
    <span className={`taf-data-quality-storage-flow-node is-${status}`} title={`${label} ${detail}`}>
      <b>{label}</b>
      <em>{detail}</em>
    </span>
  );
}

function StorageReplicaHealth({
  indexHealth,
  objectRows,
  partitionRows,
  replicaRows,
}: {
  indexHealth?: DataQualityVisuals['storageIndexHealth'];
  objectRows?: DataQualityVisuals['storageObjectRows'];
  partitionRows?: DataQualityVisuals['storagePartitionRows'];
  replicaRows?: DataQualityVisuals['storageReplicaRows'];
}) {
  const replicas = replicaRows?.length ? replicaRows : storageReplicaFallbackRows;
  const indexes = indexHealth?.length ? indexHealth : storageIndexHealthFallback;
  const partitions = partitionRows?.length ? partitionRows : storagePartitionFallbackRows;
  const objects = objectRows?.length ? objectRows : storageObjectFallbackRows;
  return (
    <div className="taf-data-quality-storage-health">
      <div className="taf-data-quality-storage-replica-list">
        {replicas.map((row) => (
          <button key={row[0]} type="button" className={`is-${storageStatusClass(row[4])}`} title={row.join(' ')}>
            <strong>{row[0]}</strong>
            <span>{row[1]}</span>
            <span>{row[2]}</span>
            <em>{row[3]}</em>
          </button>
        ))}
      </div>
      <div className="taf-data-quality-storage-donut-block">
        <StorageIndexDonut rows={indexes} />
        <div>
          {indexes.map((item) => (
            <span key={item.label} className={`is-${item.status}`} title={`${item.label} ${item.value}`}>
              <i />
              {item.label}
              <b>{item.value}</b>
            </span>
          ))}
        </div>
      </div>
      <div className="taf-data-quality-storage-health-tables">
        <section>
          <h4>分区健康</h4>
          {partitions.map((row) => <p key={row[1]} title={row.join(' ')}>{row.map((cell) => <span key={cell}>{cell}</span>)}</p>)}
        </section>
        <section>
          <h4>对象生命周期</h4>
          {objects.map(([label, value]) => <p key={label} title={`${label} ${value}`}><span>{label}</span><b>{value}</b></p>)}
        </section>
      </div>
    </div>
  );
}

function StorageIndexDonut({ rows }: { rows: DataQualityVisuals['storageIndexHealth'] }) {
  const colorMap = { ok: '#52c41a', info: '#1890ff', warn: '#faad14', risk: '#ff4d4f' };
  return (
    <DataQualityDonutChart
      ariaLabel="OpenSearch 索引健康"
      className="taf-data-quality-storage-donut"
      rows={rows.map((item) => ({ label: item.label, value: item.value, color: colorMap[item.status] }))}
    />
  );
}

function StorageQualityOverview({ rows }: { rows?: DataQualityVisuals['storageComponentRows'] }) {
  const overviewRows = (rows?.length ? rows : storageComponentFallbackRows).map((row) => [row[0], row[1], row[2], row[3], row[4], row[5]]);
  return (
    <DenseRows columns={['组件', '状态', '写入速率', '成功率', 'P95', '积压']} rows={overviewRows} />
  );
}

function ReplayReconcileContent({
  dataQualityVisuals,
  onOpenReplay,
}: {
  dataQualityVisuals?: DataQualityVisuals;
  onOpenReplay: () => void;
}) {
  return (
    <>
      <div className="taf-data-quality-replay-upper">
        <WorkPanel
          title="DLQ 重放任务表"
          extra={<Button size="small" icon={<SyncOutlined />} onClick={onOpenReplay}>重放</Button>}
        >
          <ReplayTaskTable rows={dataQualityVisuals?.replayTaskRows} />
        </WorkPanel>
        <WorkPanel
          title="时间窗对账报告（近 24 小时）"
          extra={(
            <Space className="taf-data-quality-replay-panel-tools" size={4}>
              <Button size="small">按小时</Button>
              <Button size="small" icon={<DownloadOutlined />}>导出图表</Button>
            </Space>
          )}
        >
          <ReplayReconcileTrend
            summary={dataQualityVisuals?.replayReconcileSummary}
            trend={dataQualityVisuals?.replayReconcileTrend}
          />
        </WorkPanel>
        <WorkPanel title="幂等检查与重复检测">
          <ReplayIdempotencyTable rows={dataQualityVisuals?.replayIdempotencyRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-replay-lower">
        <WorkPanel title="差异样本与原因（近 24 小时）">
          <ReplayDifferenceTable rows={dataQualityVisuals?.replayDifferenceRows} />
        </WorkPanel>
        <WorkPanel title="重放链路状态">
          <ReplayFlowStatus edges={dataQualityVisuals?.replayFlowEdges} nodes={dataQualityVisuals?.replayFlowNodes} />
        </WorkPanel>
        <WorkPanel title="验收证据与导出">
          <ReplayEvidenceExport rows={dataQualityVisuals?.replayEvidenceRows} />
        </WorkPanel>
      </div>
    </>
  );
}

const replayTaskColumns = ['任务', '来源 Topic', '时间窗', '待重放', '成功率', '失败数', '幂等状态', '操作'];

function replayStatusClass(value: string | undefined) {
  if (!value) return 'info';
  if (value.includes('高') || value.includes('失败') || value.includes('异常')) return 'risk';
  if (value.includes('警告') || value.includes('待重放') || value.includes('冲突') || value.includes('重试')) return 'warn';
  if (value.includes('通过') || value.includes('已归档') || value.includes('RUNNING') || value.includes('正常')) return 'ok';
  return 'info';
}

function ReplayTaskTable({ rows }: { rows?: DataQualityVisuals['replayTaskRows'] }) {
  const taskRows = rows?.length ? rows : replayTaskFallbackRows;
  return (
    <div className="taf-data-quality-replay-task-table">
      <div>{replayTaskColumns.map((column) => <span key={column} title={column}>{column}</span>)}</div>
      {taskRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button" className={`is-${replayStatusClass(row[6])}`} title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 6 ? `is-${replayStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
    </div>
  );
}

function ReplayReconcileTrend({
  summary,
  trend,
}: {
  summary?: DataQualityVisuals['replayReconcileSummary'];
  trend?: DataQualityVisuals['replayReconcileTrend'];
}) {
  const chart = trend ?? replayReconcileTrendFallback;
  const summaryRows = summary?.length ? summary : replayReconcileSummaryFallback;
  return (
    <div className="taf-data-quality-replay-trend" title="源端总数、落库总数、差异数量、差异率（%）">
      <div className="taf-data-quality-replay-trend-legend">
        {[
          ['源端总数', 'source'],
          ['落库总数', 'sink'],
          ['差异数量', 'diff'],
          ['差异率（%）', 'rate'],
          ['阈值 1.00%', 'threshold'],
        ].map(([label, tone]) => <span key={label} className={`is-${tone}`} title={label}><i />{label}</span>)}
      </div>
      <DataQualityTrendChart
        ariaLabel="时间窗对账报告趋势"
        className="taf-data-quality-replay-echart"
        categories={chart.times}
        series={[
          { name: '源端总数', color: '#18a8ff', values: chart.sourceTotal },
          { name: '落库总数', color: '#4ade80', values: chart.sinkTotal },
          { name: '差异数量', color: '#ffb020', type: 'bar', values: chart.diffCount },
          { name: '差异率', color: '#ff4d4f', values: chart.diffRate },
          { name: '阈值', color: '#a78bfa', dashed: true, values: chart.diffRateThreshold },
        ]}
      />
      <div className="taf-data-quality-replay-summary">
        <strong title="汇总（近24小时）">汇总（近24小时）</strong>
        {summaryRows.map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`} title={`${label} ${value}`}>{label}<b>{value}</b></span>
        ))}
      </div>
    </div>
  );
}

function ReplayIdempotencyTable({ rows }: { rows?: DataQualityVisuals['replayIdempotencyRows'] }) {
  const checkRows = rows?.length ? rows : replayIdempotencyFallbackRows;
  return (
    <div className="taf-data-quality-replay-idempotency">
      <div>{['检查项', '规则', '状态', '命中', '操作'].map((column) => <span key={column} title={column}>{column}</span>)}</div>
      {checkRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button" className={`is-${replayStatusClass(row[2])}`} title={row.join(' ')}>
          {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 2 ? `is-${replayStatusClass(cell)}` : index === 4 ? 'is-link' : ''} title={cell}>{cell}</span>)}
        </button>
      ))}
      <footer><button type="button" title="查看全部检查项">查看全部检查项 <ArrowUpOutlined /></button></footer>
    </div>
  );
}

function ReplayDifferenceTable({ rows }: { rows?: DataQualityVisuals['replayDifferenceRows'] }) {
  const sampleRows = rows?.length ? rows : replayDifferenceFallbackRows;
  return (
    <div className="taf-data-quality-replay-difference">
      <div>{['时间窗', 'Topic', '差异类型', 'Trace ID', '源端值', '落库值', '原因', '操作'].map((column) => <span key={column} title={column}>{column}</span>)}</div>
      {sampleRows.map((row) => (
        <button key={`${row[0]}-${row[1]}-${row[3]}`} type="button" title={row.join(' ')}>
          {row.map((cell, index) => <span key={`${row[3]}-${index}`} className={index === 7 ? 'is-link' : index === 2 && (cell.includes('duplicate') || cell.includes('timeout')) ? 'is-warn' : ''} title={cell}>{cell}</span>)}
        </button>
      ))}
      <footer><button type="button" title="查看更多差异样本">查看更多差异样本 <ArrowUpOutlined /></button></footer>
    </div>
  );
}

function ReplayFlowStatus({
  edges,
  nodes,
}: {
  edges?: DataQualityVisuals['replayFlowEdges'];
  nodes?: DataQualityVisuals['replayFlowNodes'];
}) {
  const flowNodes = nodes?.length ? nodes : replayFlowFallbackNodes;
  const flowEdges = edges?.length ? edges : replayFlowFallbackEdges;
  const topNodes = flowNodes.slice(0, 4);
  const bottomNodes = flowNodes.slice(4);
  return (
    <div className="taf-data-quality-replay-flow" title="重放链路状态">
      <div className="taf-data-quality-replay-flow-chain">
        {topNodes.map((node, index) => <ReplayFlowNode key={node.id} node={node} step={index} />)}
      </div>
      <div className="taf-data-quality-replay-flow-bottom">
        {bottomNodes.map((node, index) => <ReplayFlowNode key={node.id} node={node} step={index + topNodes.length} compact />)}
      </div>
      <footer>
        {flowEdges.map((edge) => (
          <span key={`${edge.from}-${edge.to}`} className={`is-${edge.status}`} title={`${edge.from} → ${edge.to} ${edge.label}`}>
            <i />
            {edge.label}
          </span>
        ))}
        <button type="button" title="查看链路详情">查看链路详情 <ArrowUpOutlined /></button>
      </footer>
    </div>
  );
}

function ReplayFlowNode({
  compact = false,
  node,
  step,
}: {
  compact?: boolean;
  node: DataQualityVisuals['replayFlowNodes'][number];
  step: number;
}) {
  const details = node.detail.split('/').map((item) => item.trim()).filter(Boolean).slice(0, compact ? 2 : 3);
  return (
    <section className={`taf-data-quality-replay-flow-node is-${node.status}${compact ? ' is-compact' : ''}`} title={`${node.label} ${node.detail}`} style={{ '--replay-step': step } as CSSProperties}>
      <strong title={node.label}>{node.label}</strong>
      {details.map((detail) => <span key={detail} title={detail}>{detail}</span>)}
    </section>
  );
}

function ReplayEvidenceExport({ rows }: { rows?: DataQualityVisuals['replayEvidenceRows'] }) {
  const evidenceRows = rows?.length ? rows : replayEvidenceFallbackRows;
  return (
    <div className="taf-data-quality-replay-evidence">
      {evidenceRows.map((row) => (
        <button key={row[0]} type="button" className={`is-${replayStatusClass(row[3])}`} title={row.join(' ')}>
          <CheckCircleOutlined />
          <strong title={row[0]}>{row[0]}</strong>
          <span title={`${row[1]} ${row[2]}`}>{row[1]} · {row[2]}</span>
          <em title={row[4]}>{row[4]}</em>
        </button>
      ))}
      <footer><button type="button" title="查看验收历史">查看验收历史 <ArrowUpOutlined /></button></footer>
    </div>
  );
}

function ReplayReconcileSideRail({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-replay-rail">
      <WorkPanel title="重放对账异常">
        <ReplayRailAlerts rows={dataQualityVisuals?.replayRailAlerts} />
      </WorkPanel>
      <WorkPanel title="快速定位">
        <ReplayRailLinks icon="search" rows={dataQualityVisuals?.replayRailLocateRows} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <ReplayRailLinks icon="sync" rows={dataQualityVisuals?.replayRailRepairRows} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <ReplayRailLinks icon="download" rows={dataQualityVisuals?.replayRailEvidenceRows} />
      </WorkPanel>
    </aside>
  );
}

function ReplayRailAlerts({ rows }: { rows?: DataQualityVisuals['replayRailAlerts'] }) {
  const alertRows = rows?.length ? rows : replayRailAlertFallbackRows;
  return (
    <div className="taf-data-quality-replay-rail-alerts">
      {alertRows.map(([level, title, value, tone]) => (
        <button key={title} type="button" className={`is-${tone}`} title={`${level} ${title} ${value}`}>
          <span title={level}>{level}</span>
          <strong title={title}>{title}</strong>
          <b title={value}>{value}</b>
        </button>
      ))}
      <a href="#replay-all-alerts" title="查看全部异常">查看全部异常 <ArrowUpOutlined /></a>
    </div>
  );
}

function ReplayRailLinks({
  icon,
  rows,
}: {
  icon: 'download' | 'search' | 'sync';
  rows?: string[];
}) {
  const fallbackRows = icon === 'download' ? replayRailEvidenceFallbackRows : icon === 'sync' ? replayRailRepairFallbackRows : replayRailLocateFallbackRows;
  const items = rows?.length ? rows : fallbackRows;
  const iconNode = icon === 'download' ? <DownloadOutlined /> : icon === 'sync' ? <SyncOutlined /> : <SearchOutlined />;
  return (
    <div className="taf-data-quality-replay-rail-links">
      {items.map((label) => (
        <button key={label} type="button" title={label}>
          {iconNode}
          <span title={label}>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function ReportContent({ evidence }: { evidence: PageSnapshot['evidence'] }) {
  return (
    <div className="taf-data-quality-report-workspace">
      <WorkPanel title="质量报告预览" className="taf-data-quality-report-preview-panel">
        <QualityReportPreview />
      </WorkPanel>
      <WorkPanel title="报告章节" className="taf-data-quality-report-chapters">
        <ReportChapters />
      </WorkPanel>
      <WorkPanel title="异常归因摘要（近 24 小时）" className="taf-data-quality-report-anomaly-panel">
        <DenseRows
          columns={['异常类型', '根因分析', '负责人', '影响范围', '修复状态']}
          rows={reportAnomalyRows}
        />
        <div className="taf-data-quality-report-more">查看全部异常归因 <ArrowUpOutlined /></div>
      </WorkPanel>
      <WorkPanel title="导出记录" className="taf-data-quality-report-export-panel">
        <DenseRows
          columns={['导出时间', '来源', '申请人', '审批状态', '接收团队', '操作']}
          rows={reportExportRows}
        />
        <div className="taf-data-quality-report-more">查看全部导出记录 <ArrowUpOutlined /></div>
      </WorkPanel>
      <WorkPanel title="验收报告与审批" className="taf-data-quality-report-approval-panel">
        <ReportApproval evidence={evidence} />
      </WorkPanel>
    </div>
  );
}

function QualityReportPreview() {
  return (
    <div className="taf-data-quality-report-preview">
      <article className="taf-data-quality-report-sheet">
        <header>
          <SafetyCertificateOutlined />
          <div>
            <h2 title="数据质量日报">数据质量日报</h2>
            <p>统计时间：2025-06-25 15:30:45 ~ 2025-06-26 15:30:45</p>
          </div>
          <span>版本：v2026.06.26<br />生成时间：2025-06-26 15:10:12</span>
        </header>
        <section className="taf-data-quality-report-score-strip">
          {[
            ['质量评分', '92/100'],
            ['完整性', '96.3%'],
            ['及时性', '91.7%'],
            ['一致性', '95.8%'],
            ['可用性', '98.2%'],
            ['安全合规', '100%'],
          ].map(([label, value]) => (
            <div key={label}>
              <span>{label}</span>
              <strong>{value}</strong>
            </div>
          ))}
        </section>
        <div className="taf-data-quality-report-sheet-grid">
          <section className="taf-data-quality-report-line">
            <h3>二、质量趋势（近 24 小时）</h3>
            <svg viewBox="0 0 340 142" preserveAspectRatio="none" aria-label="质量趋势">
              {[24, 50, 76, 102, 128].map((y) => <line key={y} x1="0" x2="340" y1={y} y2={y} />)}
              <polyline className="is-blue" points="0,40 28,42 56,35 84,39 112,31 140,37 168,29 196,34 224,32 252,38 280,30 308,35 340,28" />
              <polyline className="is-green" points="0,58 28,55 56,61 84,52 112,56 140,50 168,55 196,47 224,51 252,49 280,45 308,48 340,43" />
              <polyline className="is-orange" points="0,82 28,78 56,84 84,80 112,75 140,79 168,73 196,76 224,72 252,74 280,70 308,73 340,68" />
              <polyline className="is-purple" points="0,101 28,96 56,104 84,99 112,93 140,97 168,91 196,94 224,90 252,92 280,86 308,89 340,84" />
            </svg>
          </section>
          <section className="taf-data-quality-report-donut">
            <h3>三、异常概览</h3>
            <div>
              <strong>23</strong>
              <span>异常总数</span>
            </div>
            <ul>
              <li><i className="is-blue" />字段质量 <b>9 (39%)</b></li>
              <li><i className="is-orange" />Flink 质量 <b>6 (26%)</b></li>
              <li><i className="is-green" />存储质量 <b>5 (22%)</b></li>
              <li><i className="is-purple" />Topic 健康 <b>2 (9%)</b></li>
            </ul>
          </section>
          <section>
            <h3>四、关键指标对比</h3>
            <ReportMiniTable rows={[
              ['事件总量', '18.4K', '17.2K', '↑ 6.9%'],
              ['DLQ 数量', '12.8K', '13.6K', '↓ 5.8%'],
              ['写入成功率', '99.84%', '99.71%', '↑ 0.13%'],
              ['写入延迟 P95', '420ms', '460ms', '↓ 40ms'],
              ['Backpressure', '0.38', '0.46', '↓ 0.08'],
            ]} />
          </section>
          <section>
            <h3>五、存储写入质量</h3>
            <ReportMiniTable rows={[
              ['ClickHouse', '42.7K EPS', '99.82%', '380ms'],
              ['OpenSearch', '12.6K docs/s', '99.71%', '560ms'],
              ['NebulaGraph', '2.1K edges/s', '99.46%', '210ms'],
              ['MinIO', '1.8K objects/s', '99.64%', '690ms'],
            ]} />
          </section>
        </div>
        <footer>
          <section>
            <h3>六、重放对账结果</h3>
            <div className="taf-data-quality-report-conclusion-grid">
              <span title="对账通过率 99.12%">对账通过率 <b title="99.12%">99.12%</b></span>
              <span title="窗口错序 0.31%">窗口错序 <b title="0.31%">0.31%</b></span>
              <span title="重复记录 2,136">重复记录 <b title="2,136">2,136</b></span>
              <span title="异常冲突 47">异常冲突 <b title="47">47</b></span>
            </div>
          </section>
          <section>
            <h3>七、验收结论</h3>
            <div className="taf-data-quality-report-conclusion">
              <strong>通过</strong>
              <span title="SLA 97.8% 达成，数据质量总体健康。">SLA 97.8% 达成，数据质量总体健康。</span>
              <small title="建议：字段缺失与存储延迟需持续优化跟踪。">建议：字段缺失与存储延迟需持续优化跟踪。</small>
            </div>
          </section>
        </footer>
      </article>
      <div className="taf-data-quality-report-viewerbar">
        <Button size="small" type="text">‹</Button>
        <span className="is-page">1</span>
        <span>/ 16</span>
        <Button size="small" type="text">›</Button>
        <i />
        <span>100%</span>
        <Button size="small" type="text">+</Button>
        <Button size="small" type="text" icon={<DownloadOutlined />} />
        <Button size="small" type="text" icon={<PrinterOutlined />} />
        <Button size="small" type="text" icon={<FullscreenOutlined />} />
      </div>
    </div>
  );
}

function ReportMiniTable({ rows }: { rows: string[][] }) {
  return (
    <div className="taf-data-quality-report-mini-table">
      {rows.map((row) => (
        <span key={row.join('-')}>
          {row.map((cell) => <b key={cell}>{cell}</b>)}
        </span>
      ))}
    </div>
  );
}

function ReportChapters() {
  return (
    <div className="taf-data-quality-report-chapter-list">
      {reportChapterRows.map(([index, label, value, tone]) => (
        <button key={label} type="button" className={`is-${tone}`}>
          <b>{index}</b>
          <span>{label}</span>
          <em>{value}</em>
          <CheckCircleOutlined />
        </button>
      ))}
    </div>
  );
}

function ReportApproval({ evidence }: { evidence: PageSnapshot['evidence'] }) {
  return (
    <div className="taf-data-quality-report-approval">
      <section>
        <h3>验收包信息</h3>
        <p>验收包：数据质量验收包 #20250626</p>
        <p>版本：v2026.06.26</p>
        <p>生成时间：2025-06-26 15:10:12</p>
        <p>内容：报告 + 证据清单 + 对账报告 + 日志快照</p>
      </section>
      <section className="taf-data-quality-report-sla">
        <span>SLA Gate</span>
        <strong title="97.8%">97.8%</strong>
        <em>达成（&gt;= 95%）</em>
      </section>
      <section className="taf-data-quality-report-audit">
        <h3>审批流转</h3>
        <p><CheckCircleOutlined />提交 sec_analyst 2025-06-26 15:12</p>
        <p><SyncOutlined />审核中 data_manager 2025-06-26 15:18</p>
        <p><FieldTimeOutlined />终审 security_manager 待处理</p>
      </section>
      <section>
        <h3>风控 / 例外</h3>
        <p>存储延迟未完全恢复</p>
        <p>影响 ClickHouse Distributed 队列 <b>低</b></p>
      </section>
      <div className="taf-data-quality-report-approval-evidence">
        {evidence.slice(0, 3).map((item) => <span key={item.label}>{item.label}<b>{item.value}</b></span>)}
      </div>
    </div>
  );
}

const topicHealthRailAlerts = [
  ['严重', 'dlq_topic 消费延迟过高', 'P95 15.6s > 阈值 3s', '15:25', 'risk'],
  ['告警', 'threat_alerts 分区倾斜', '倾斜度 2.48 > 阈值 2', '15:20', 'warn'],
  ['提示', 'flow_enriched 积压增长', '24h 积压增长 0.45M', '15:18', 'info'],
];

const topicHealthLocateRows: Array<[string, ReactNode]> = [
  ['定位异常 Topic', <FileSearchOutlined key="topic" />],
  ['定位异常分区', <SearchOutlined key="partition" />],
  ['定位异常消费组', <ApiOutlined key="group" />],
  ['查看相关 Flink 作业', <DatabaseOutlined key="flink" />],
];

const topicHealthRepairRows: Array<[string, ReactNode]> = [
  ['优化消费者并行度', <SettingOutlined key="parallelism" />],
  ['调整 Topic 分区数', <ApiOutlined key="partition" />],
  ['清理或消费 DLQ', <DatabaseOutlined key="dlq" />],
  ['扩容 Kafka 集群', <SafetyCertificateOutlined key="kafka" />],
];

const topicHealthEvidenceRows: Array<[string, ReactNode]> = [
  ['生成 Topic 健康报告', <FileDoneOutlined key="report" />],
  ['导出 offset 对账', <DownloadOutlined key="offset" />],
  ['创建分区评估', <SafetyCertificateOutlined key="partition-report" />],
  ['查看消费组详情', <FileSearchOutlined key="consumer" />],
  ['查看 DLQ 详情', <DatabaseOutlined key="dlq-detail" />],
];

const flinkQualityRailAlerts = [
  ['严重', 'behavior-job 背压过高', 'Backpressure 0.78', '15:25', 'risk'],
  ['告警', 'Watermark 延迟升高', 'P95 3.4s > 阈值 2s', '15:20', 'warn'],
  ['告警', 'Checkpoint 耗时异常', 'P95 2.1s > 阈值 1.5s', '15:18', 'warn'],
];

const flinkQualityLocateRows: Array<[string, ReactNode]> = [
  ['定位 Flink 作业', <FileSearchOutlined key="flink-job" />],
  ['查看 Checkpoint', <DatabaseOutlined key="checkpoint" />],
  ['查看 Watermark', <FieldTimeOutlined key="watermark" />],
  ['查看 Backpressure', <ApiOutlined key="backpressure" />],
  ['查看作业日志', <FileDoneOutlined key="job-log" />],
];

const flinkQualityRepairRows: Array<[string, ReactNode]> = [
  ['优化并行度与资源', <SettingOutlined key="parallelism" />],
  ['处理反压根因', <ApiOutlined key="root-cause" />],
  ['降低 Watermark 延迟', <FieldTimeOutlined key="watermark-delay" />],
  ['清理异常数据源', <DatabaseOutlined key="source-cleanup" />],
  ['升级 Flink 版本', <SafetyCertificateOutlined key="flink-version" />],
];

const flinkQualityEvidenceRows: Array<[string, ReactNode]> = [
  ['导出 Watermark 报告', <DownloadOutlined key="watermark-report" />],
  ['导出 Checkpoint 报告', <DownloadOutlined key="checkpoint-report" />],
  ['导出 Backpressure 报告', <DownloadOutlined key="backpressure-report" />],
  ['生成 Flink 质量报告', <FileDoneOutlined key="flink-report" />],
  ['跳转部署管理', <DatabaseOutlined key="deployment" />],
];

const fieldQualityRailStats = [
  ['异常字段数', '23'],
  ['影响记录数', '18.4K'],
  ['高影响字段', '5'],
  ['严重异常数', '2'],
];

const fieldQualityLocateRows: Array<[string, ReactNode]> = [
  ['查看异常样本', <FileSearchOutlined key="field-sample" />],
  ['按字段分析趋势', <ApiOutlined key="field-trend" />],
  ['按资产查看影响', <DatabaseOutlined key="field-asset" />],
  ['按 Topic 查看异常', <FileDoneOutlined key="field-topic" />],
];

const fieldQualityRepairRows: Array<[string, ReactNode]> = [
  ['创建字段修复任务', <SettingOutlined key="field-task" />],
  ['同步资产映射', <ApiOutlined key="asset-mapping" />],
  ['更新枚举映射规则', <DatabaseOutlined key="enum" />],
  ['配置格式校验规则', <FileSearchOutlined key="format-rule" />],
  ['时间校准策略', <FieldTimeOutlined key="time-sync" />],
  ['修复任务列表', <FileDoneOutlined key="task-list" />],
];

const fieldQualityEvidenceRows: Array<[string, ReactNode]> = [
  ['字段质量证据中心', <FileSearchOutlined key="field-evidence-center" />],
  ['导出字段质量报告', <DownloadOutlined key="field-report" />],
  ['导出异常样本', <DownloadOutlined key="field-sample-export" />],
  ['字段质量周报', <FileDoneOutlined key="field-weekly" />],
  ['跳转重放对账', <DatabaseOutlined key="field-reconcile" />],
];

function TopicHealthSideRail({ onOpenDlqSample, onReplay }: { onOpenDlqSample: () => void; onReplay: () => void }) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-topic-health-rail">
      <WorkPanel title="质量异常告警">
        <div className="taf-data-quality-topic-rail-alerts">
          {topicHealthRailAlerts.map(([level, title, detail, time, tone]) => (
            <button key={title} type="button" className={`is-${tone}`} title={`${level} ${title} ${detail} ${time}`}>
              <b>{level}</b>
              <strong>{title}</strong>
              <span>{detail}</span>
              <em>{time}</em>
            </button>
          ))}
          <button type="button" className="is-link" title="查看全部告警（3）">查看全部告警（3） <ArrowUpOutlined /></button>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <TopicHealthRailLinks rows={topicHealthLocateRows} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <TopicHealthRailLinks rows={topicHealthRepairRows} onPrimary={onReplay} />
      </WorkPanel>
      <WorkPanel title="快收证据与报告">
        <TopicHealthRailLinks rows={topicHealthEvidenceRows} onPrimary={onOpenDlqSample} />
      </WorkPanel>
    </aside>
  );
}

function FieldQualitySideRail({
  onOpenDetail,
  onOpenReplayReconcile,
}: {
  onOpenDetail: OpenFieldQualityDetail;
  onOpenReplayReconcile: () => void;
}) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-field-rail">
      <WorkPanel title="字段质量异常（近 24 小时）">
        <div className="taf-data-quality-field-rail-stats">
          {fieldQualityRailStats.map(([label, value], index) => (
            <button
              key={label}
              type="button"
              className={index > 1 ? 'is-risk' : 'is-info'}
              title={`${label} ${value}`}
              onClick={() => onOpenDetail({
                title: `${label}详情`,
                description: '当前统计来自字段质量实时快照，可继续查看关联异常样本与修复任务。',
                columns: ['指标', '当前值', '建议入口'],
                rows: [[label, value, index > 1 ? '查看修复任务' : '查看异常样本']],
              })}
            >
              <span>{label}</span>
              <b>{value}</b>
            </button>
          ))}
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <FieldQualityRailLinks rows={fieldQualityLocateRows} onOpenDetail={onOpenDetail} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <FieldQualityRailLinks rows={fieldQualityRepairRows} onOpenDetail={onOpenDetail} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <FieldQualityRailLinks rows={fieldQualityEvidenceRows} onOpenDetail={onOpenDetail} onOpenReplayReconcile={onOpenReplayReconcile} />
      </WorkPanel>
    </aside>
  );
}

function FieldQualityRailLinks({
  onOpenDetail,
  onOpenReplayReconcile,
  rows,
}: {
  onOpenDetail: OpenFieldQualityDetail;
  onOpenReplayReconcile?: () => void;
  rows: Array<[string, ReactNode]>;
}) {
  const openLink = (label: string) => {
    if (label === '跳转重放对账' && onOpenReplayReconcile) {
      onOpenReplayReconcile();
      return;
    }
    const focusSelector = label.includes('样本') || label.includes('Topic') || label.includes('资产')
      ? '.taf-data-quality-field-samples-panel'
      : label.includes('趋势')
        ? '.taf-data-quality-field-upper > .taf-panel:nth-child(2)'
        : label.includes('修复') || label.includes('映射') || label.includes('校验') || label.includes('时间校准')
          ? '.taf-data-quality-field-repairs-panel'
          : undefined;
    const actionable = label.startsWith('创建') || label.startsWith('导出') || label.startsWith('同步') || label.startsWith('更新') || label.startsWith('配置');
    onOpenDetail({
      title: label,
      description: actionable
        ? '已加载字段质量操作预览；确认后将写入模拟任务队列，并保留操作审计。'
        : '已定位到对应字段质量业务区域，可继续查看实时数据与关联记录。',
      columns: ['操作', '当前视图', '处理建议'],
      rows: [[label, '字段质量', actionable ? '确认后提交模拟任务' : '查看关联业务数据']],
      actionLabel: actionable ? (label.startsWith('导出') ? '生成导出任务' : '提交操作') : undefined,
      actionSuccessMessage: actionable ? `${label}已提交` : undefined,
    }, focusSelector);
  };
  return (
    <div className="taf-data-quality-topic-rail-links">
      {rows.map(([label, icon]) => (
        <button key={label} type="button" onClick={() => openLink(label)} title={label}>
          {icon}
          <span>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function StorageQualitySideRail({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  const alerts = dataQualityVisuals?.storageRailAlerts?.length ? dataQualityVisuals.storageRailAlerts : storageRailAlertFallbackRows;
  const locateRowsForRail = dataQualityVisuals?.storageRailLocateRows?.length ? dataQualityVisuals.storageRailLocateRows : storageRailLocateFallbackRows;
  const repairRowsForRail = dataQualityVisuals?.storageRailRepairRows?.length ? dataQualityVisuals.storageRailRepairRows : storageRailRepairFallbackRows;
  const evidenceRowsForRail = dataQualityVisuals?.storageRailEvidenceRows?.length ? dataQualityVisuals.storageRailEvidenceRows : storageRailEvidenceFallbackRows;
  return (
    <aside className="taf-data-quality-rail taf-data-quality-storage-rail">
      <WorkPanel title="存储质量异常（近 24 小时）">
        <div className="taf-data-quality-topic-rail-alerts taf-data-quality-storage-rail-alerts">
          {alerts.map(([level, title, detail, time, tone]) => (
            <button key={`${title}-${time}`} type="button" className={`is-${tone}`} title={`${level} ${title} ${detail} ${time}`}>
              <b>{level}</b>
              <strong>{title}</strong>
              <span>{detail}</span>
              <em>{time}</em>
            </button>
          ))}
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <StorageRailLinks rows={locateRowsForRail} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <StorageRailLinks rows={repairRowsForRail} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <StorageRailLinks rows={evidenceRowsForRail} />
      </WorkPanel>
    </aside>
  );
}

function StorageRailLinks({ rows }: { rows: string[] }) {
  const icons = [<SearchOutlined />, <SyncOutlined />, <DatabaseOutlined />, <ApiOutlined />, <FileSearchOutlined />, <DownloadOutlined />];
  return (
    <div className="taf-data-quality-topic-rail-links taf-data-quality-storage-rail-links">
      {rows.map((label, index) => (
        <button key={label} type="button" title={label}>
          {icons[index % icons.length]}
          <span>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function FlinkQualitySideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-flink-rail">
      <WorkPanel title="Flink 质量异常">
        <div className="taf-data-quality-topic-rail-alerts taf-data-quality-flink-rail-alerts">
          {flinkQualityRailAlerts.map(([level, title, detail, time, tone]) => (
            <button key={title} type="button" className={`is-${tone}`} title={`${level} ${title} ${detail} ${time}`}>
              <b>{level}</b>
              <strong>{title}</strong>
              <span>{detail}</span>
              <em>{time}</em>
            </button>
          ))}
          <button type="button" className="is-link" title="查看全部告警（12）">查看全部告警（12） <ArrowUpOutlined /></button>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <TopicHealthRailLinks rows={flinkQualityLocateRows} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <TopicHealthRailLinks rows={flinkQualityRepairRows} />
      </WorkPanel>
      <WorkPanel title="快收证据与报告">
        <TopicHealthRailLinks rows={flinkQualityEvidenceRows} />
      </WorkPanel>
    </aside>
  );
}

function TopicHealthRailLinks({ rows, onPrimary }: { rows: Array<[string, ReactNode]>; onPrimary?: () => void }) {
  return (
    <div className="taf-data-quality-topic-rail-links">
      {rows.map(([label, icon], index) => (
        <button key={label} type="button" onClick={index === 0 ? onPrimary : undefined} title={label}>
          {icon}
          <span>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function ReportSideRail({ onOpenReplay }: { onOpenReplay: () => void }) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-report-rail">
      <WorkPanel title="报告异常（近 24 小时）">
        <div className="taf-data-quality-report-rail-anomalies">
          {reportRailAnomalies.map(([label, value, tone]) => (
            <button key={label} type="button" className={`is-${tone}`}>
              <WarningOutlined />
              <span>{label}</span>
              <b>{value}</b>
            </button>
          ))}
          <button type="button" className="is-link">查看全部异常 <ArrowUpOutlined /></button>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <ReportRailButtons rows={reportRailLocateRows} icon={<FileSearchOutlined />} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <ReportRailButtons rows={reportRailRepairRows} icon={<SettingOutlined />} onPrimary={onOpenReplay} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <div className="taf-data-quality-report-rail-actions">
          {[
            ['生成质量日报', <FileDoneOutlined key="daily" />],
            ['导出验收报告', <DownloadOutlined key="download" />],
            ['补齐证据清单', <FileSearchOutlined key="evidence" />],
            ['提交审批', <SafetyCertificateOutlined key="approve" />],
            ['查看导出历史', <DatabaseOutlined key="history" />],
          ].map(([label, icon]) => (
            <Button key={String(label)} size="small" icon={icon}>{label}</Button>
          ))}
        </div>
      </WorkPanel>
    </aside>
  );
}

function SettingsSideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-settings-rail">
      <WorkPanel title="设置异常（近 24 小时）">
        <div className="taf-data-quality-report-rail-anomalies">
          {settingsRailAnomalies.map(([label, value, tone]) => (
            <button key={label} type="button" className={`is-${tone}`}>
              <WarningOutlined />
              <span>{label}</span>
              <b>{value}</b>
            </button>
          ))}
          <button type="button" className="is-link">查看全部异常 <ArrowUpOutlined /></button>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <ReportRailButtons rows={settingsRailLocateRows} icon={<FileSearchOutlined />} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <ReportRailButtons rows={settingsRailRepairRows} icon={<SettingOutlined />} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <div className="taf-data-quality-report-rail-actions">
          {[
            ['校验规则配置', <FileSearchOutlined key="validate" />],
            ['提交审批', <SafetyCertificateOutlined key="approve" />],
            ['导出设置 JSON', <DownloadOutlined key="download" />],
            ['查看审计记录', <DatabaseOutlined key="audit" />],
            ['回滚上个版本', <SyncOutlined key="rollback" />],
          ].map(([label, icon]) => (
            <Button key={String(label)} size="small" icon={icon}>{label}</Button>
          ))}
        </div>
      </WorkPanel>
    </aside>
  );
}

function ReportRailButtons({
  icon,
  onPrimary,
  rows,
}: {
  icon: ReactNode;
  onPrimary?: () => void;
  rows: string[][];
}) {
  return (
    <div className="taf-data-quality-report-rail-buttons">
      {rows.map(([label], index) => (
        <button key={label} type="button" onClick={index === 0 ? onPrimary : undefined}>
          {icon}
          <span>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function SettingsContent() {
  return (
    <div className="taf-data-quality-settings-workspace">
      <WorkPanel title="质量阈值配置" className="taf-data-quality-settings-threshold-panel" extra={<span className="taf-data-quality-settings-mini-select">默认阈值组</span>}>
        <SettingsThresholdTable />
      </WorkPanel>
      <WorkPanel title="检测规则分组" className="taf-data-quality-settings-rules-panel" extra={<Button size="small" type="text">+ 新建规则组</Button>}>
        <SettingsRuleGroups />
      </WorkPanel>
      <WorkPanel title="告警策略与路由" className="taf-data-quality-settings-strategy-panel" extra={<Button size="small" type="text">+ 新建策略</Button>}>
        <SettingsAlertStrategy />
      </WorkPanel>
      <WorkPanel title="报告周期与模板" className="taf-data-quality-settings-template-panel">
        <SettingsReportTemplates />
      </WorkPanel>
      <WorkPanel title="保存确认与影响评估" className="taf-data-quality-settings-impact-panel">
        <SettingsImpactAssessment />
      </WorkPanel>
      <WorkPanel title="审计记录" className="taf-data-quality-settings-audit-panel">
        <SettingsAuditRecords />
      </WorkPanel>
    </div>
  );
}

function SettingsThresholdTable() {
  return (
    <div className="taf-data-quality-settings-threshold">
      <div>
        {['指标', '适用范围', '告警阈值', '阻断阈值', 'SLA', '启用', '负责人', '操作'].map((item) => <span key={item}>{item}</span>)}
      </div>
      {settingsThresholdRows.map((row) => (
        <button key={row[0]} type="button">
          <strong>{row[0]}</strong>
          <span>{row[1]}</span>
          <em>{row[2]}</em>
          <em className="is-risk">{row[3]}</em>
          <span>{row[4]}</span>
          <i />
          <span>{row[6]}</span>
          <b>{row[7]}</b>
        </button>
      ))}
      <footer>
        <span>共 8 条</span>
        <b>‹</b>
        <strong>1</strong>
        <b>›</b>
        <span>10 条/页</span>
      </footer>
    </div>
  );
}

function SettingsRuleGroups() {
  return (
    <div className="taf-data-quality-settings-rules">
      {settingsRuleGroups.map((group) => (
        <section key={group.title}>
          <header>
            <strong>{group.title}</strong>
            <span>{group.version}</span>
            <b>启用</b>
            <em>规则数 {group.rules.length * 4}</em>
          </header>
          {group.rules.map(([name, enabled, severity, action, freq]) => (
            <button key={`${group.title}-${name}`} type="button" className={severity === '严重' ? 'is-risk' : severity === '中危' ? 'is-warn' : 'is-info'}>
              <span>{name}</span>
              <i />
              <b>{severity}</b>
              <em>{action}</em>
              <small>{freq}</small>
            </button>
          ))}
        </section>
      ))}
    </div>
  );
}

function SettingsAlertStrategy() {
  return (
    <div className="taf-data-quality-settings-strategy">
      <div>
        {['严重级别', '通知渠道', '负责人', '升级策略', 'Webhook / 抄送', '工单规则', '静默期', 'SLA 倒计时'].map((item) => <span key={item}>{item}</span>)}
      </div>
      {settingsAlertRoutes.map((row) => (
        <button key={row[0]} type="button" className={row[0] === '严重' ? 'is-risk' : row[0] === '告警' ? 'is-warn' : row[0] === '中危' ? 'is-mid' : 'is-ok'}>
          {row.map((cell, index) => index === 0 ? <strong key={cell}>{cell}</strong> : <span key={`${cell}-${index}`}>{cell}</span>)}
        </button>
      ))}
    </div>
  );
}

function SettingsReportTemplates() {
  return (
    <div className="taf-data-quality-settings-template">
      <div>
        {['类型', '时间', 'cron 表达式', '接收人组', '模板', '证据包', '导出格式'].map((item) => <span key={item}>{item}</span>)}
      </div>
      {settingsReportRows.map((row) => (
        <button key={row[0]} type="button">
          {row.map((cell, index) => <span key={`${cell}-${index}`}>{cell}</span>)}
        </button>
      ))}
    </div>
  );
}

function SettingsImpactAssessment() {
  return (
    <div className="taf-data-quality-settings-impact">
      <section>
        <h3>变更摘要（本次变更 12 项）</h3>
        {[
          '1. 完整率 告警阈值 95% -> 96%',
          '2. Watermark P95 告警 3s -> 2.5s',
          '3. DLQ 数量 阻断 5000 -> 3000',
          '4. 新增规则：存储写入延迟检测',
          '5. 告警策略：严重级别升级5分钟',
          '6. 报告：周报接收人组增加 ciso',
        ].map((item) => <p key={item}>{item}</p>)}
      </section>
      <section>
        <h3>影响评估</h3>
        <p>预计告警量变化 <b className="is-warn">+12.6%（1 个中等）</b></p>
        <p>预计阻断次数变化 <b className="is-ok">-8.3%（↓降低）</b></p>
        <p>受影响模块 <b>5 个</b></p>
        <p>需要审批 <b>是（变更较大）</b></p>
      </section>
      <section>
        <h3>校验结果</h3>
        <p><CheckCircleOutlined />通过 配置校验全部通过</p>
      </section>
      <footer>
        <Button size="small" type="primary">保存并提交审批</Button>
        <Button size="small">仅保存草稿</Button>
      </footer>
    </div>
  );
}

function SettingsAuditRecords() {
  return (
    <div className="taf-data-quality-settings-audit">
      <DenseRows
        columns={['时间', '操作者', '变更项', '变更内容（前 -> 后）', '审批状态', '操作']}
        rows={settingsAuditRows}
      />
      <footer>
        <span>共 28 条</span>
        <b>‹</b>
        <strong>1</strong>
        <span>2</span>
        <span>3</span>
        <span>7</span>
        <b>›</b>
        <span>10 条/页</span>
      </footer>
    </div>
  );
}

function TopicHeatmap({ rows = [], visuals }: { rows?: SnapshotRow[]; visuals?: DataQualityVisuals }) {
  const heatmap = visuals?.heatmap ?? buildFallbackHeatmap(rows);
  const times = visuals?.heatmapTimes ?? ['03:40', '07:40', '11:40', '15:40', '19:40', '23:40', '03:40'];
  const legend = visuals?.heatmapLegend ?? [
    { label: 'S0', status: 'ok' as const },
    { label: '均衡', status: 'info' as const },
    { label: '轻度倾斜', status: 'warn' as const },
    { label: '严重倾斜', status: 'risk' as const },
  ];
  return (
    <div className="taf-data-quality-heatmap" style={{ '--dq-heat-columns': heatmap[0]?.values.length ?? 12 } as CSSProperties}>
      <div className="taf-data-quality-heatmap-body">
        {heatmap.map((row) => (
          <div key={row.label} className="taf-data-quality-heatmap-row">
            <b title={`分区 ${row.label}`}>{row.label}</b>
            {row.values.map((tone, columnIndex) => (
              <span key={`${row.label}-${columnIndex}`} className={`is-${tone}`} title={`分区 ${row.label} / ${times[columnIndex % times.length]} / ${tone}`} />
            ))}
          </div>
        ))}
        <div className="taf-data-quality-heatmap-axis">
          <i />
          {times.map((time, index) => <span key={`${time}-${index}`}>{time}</span>)}
        </div>
      </div>
      <div className="taf-data-quality-heatmap-legend">
        {legend.map(({ label, status }) => (
          <span key={label}>
            <i className={`is-${status}`} />
            {label}
          </span>
        ))}
      </div>
    </div>
  );
}

function buildFallbackHeatmap(rows: SnapshotRow[]): DataQualityVisuals['heatmap'] {
  const labels = ['0-7', '8-15', '16-23', '24-31', '32-39', '40-47'];
  return labels.map((label, rowIndex) => ({
    label,
    values: Array.from({ length: 12 }, (_, columnIndex) => {
      const source = rows[(rowIndex + columnIndex) % Math.max(rows.length, 1)];
      const skew = Number.parseFloat(String(source?.分区倾斜 ?? '1'));
      const trend = String(source?.积压趋势 ?? '');
      if (trend.includes('上升') || skew > 1.25 || (rowIndex * 12 + columnIndex) % 17 === 0) return 'risk';
      if (trend.includes('波动') || skew > 1.12 || (rowIndex * 12 + columnIndex) % 11 === 0) return 'warn';
      if ((rowIndex * 12 + columnIndex) % 7 === 0) return 'info';
      return 'ok';
    }),
  }));
}

function LatencyTrend() {
  const p50 = [82, 79, 83, 76, 84, 80, 77, 85, 72, 81, 79, 83, 78, 86, 73, 82, 80, 78, 85, 74, 81, 79, 84, 77];
  const p95 = [70, 66, 72, 64, 73, 68, 63, 71, 60, 68, 65, 69, 61, 34, 58, 66, 63, 67, 59, 70, 64, 41, 67, 62];
  const threshold = [62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62];

  return (
    <div className="taf-data-quality-trend">
      <DataQualityTrendChart
        ariaLabel="消费延迟 P50 P95 趋势"
        className="taf-data-quality-latency-echart"
        categories={['15:30', '19:30', '23:30', '03:30', '07:30', '11:30', '15:30']}
        series={[
          { name: 'P50', color: '#18a8ff', values: p50 },
          { name: 'P95', color: '#ffb020', values: p95 },
          { name: '阈值', color: '#ff4d4f', dashed: true, values: threshold },
        ]}
        valueFormatter={(value) => `${(value / 10).toFixed(0)}s`}
      />
    </div>
  );
}

function DenseRows({ columns, rows }: { columns: string[]; rows: string[][] }) {
  return (
    <div className="taf-data-quality-dense-rows" style={{ '--dq-columns': columns.length } as CSSProperties}>
      <div className="taf-data-quality-dense-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {rows.map((row) => (
        <div key={row.join('-')} className="taf-data-quality-dense-row">
          {row.map((cell, index) => <span key={`${cell}-${index}`} title={cell}>{cell}</span>)}
        </div>
      ))}
    </div>
  );
}

function MessageSizeDistribution({ visuals }: { visuals?: DataQualityVisuals }) {
  const bars = visuals?.messageSizeDistribution ?? [
    { label: '<0.5KB', value: 12 },
    { label: '0.5-1KB', value: 21 },
    { label: '1-2KB', value: 36 },
    { label: '2-4KB', value: 28 },
    { label: '4-8KB', value: 14 },
    { label: '>8KB', value: 6 },
  ];
  const topicRows = visuals?.messageSizeTopicRows ?? [
    ['flow_original', '1.5', '14.2', '18,734', '3.2x'],
    ['flow_enriched', '1.7', '16.8', '15,962', '3.1x'],
    ['rule_logs', '0.9', '4.3', '8,521', '2.8x'],
    ['tls_logs', '1.2', '6.7', '6,342', '2.9x'],
    ['asset_events', '0.8', '3.6', '3,421', '2.6x'],
    ['threat_alerts', '1.1', '8.5', '1,842', '2.7x'],
    ['dlq_topic', '1.0', '5.1', '256', '1.0x'],
  ];
  return (
    <div className="taf-data-quality-message-size">
      <div className="taf-data-quality-bars">
        {bars.map(({ label, value }) => (
          <div key={label} title={`${label} ${value}%`}>
            <span>{label}</span>
            <i style={{ height: `${Math.min(value * 2.25, 100)}%` }} />
            <em>{value}%</em>
          </div>
        ))}
      </div>
      <DenseRows
        columns={['Topic', '平均大小(KB)', '最大大小(KB)', '吞吐 EPS', '压缩比']}
        rows={topicRows}
      />
    </div>
  );
}

function PartitionQueue({ rows }: { rows: string[][] }) {
  return (
    <div className="taf-data-quality-partition-queue">
      <div className="taf-data-quality-partition-head">
        {['Topic', '分区', '异常指标', '根因分析', '建议动作', '操作'].map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {rows.map(([topic, partition, metric, reason, action, operation]) => (
        <div key={`${topic}-${partition}`} className="taf-data-quality-partition-row">
          <strong title={topic}>{topic}</strong>
          <span title={partition}>{partition}</span>
          <span title={metric}>{metric}</span>
          <span title={reason}>{reason}</span>
          <span title={action}>{action}</span>
          <em title={`${operation} ${topic} 分区 ${partition}`}>{operation}</em>
        </div>
      ))}
      <footer>
        <span title="定位 Kafka Topic"><SearchOutlined /> 定位 Kafka Topic</span>
        <span title="定位 Flink 作业"><ApiOutlined /> 定位 Flink 作业</span>
        <span title="创建修复任务"><SafetyCertificateOutlined /> 创建修复任务</span>
      </footer>
    </div>
  );
}

function FlinkJobCards() {
  return (
    <div className="taf-data-quality-job-cards">
      {flinkJobs.map(([job, checkpoint, watermark, backpressure, pass, tone]) => (
        <div key={job} className={`is-${tone}`}>
          <strong>{job}</strong>
          <span>Checkpoint <b>{checkpoint}</b></span>
          <span>Watermark <b>{watermark}</b></span>
          <span>Backpressure <b>{backpressure}</b></span>
          <em>{pass}</em>
        </div>
      ))}
    </div>
  );
}

function HeatBands() {
  return (
    <div className="taf-data-quality-heat-bands">
      {Array.from({ length: 42 }, (_, index) => {
        const tone = index % 13 === 0 ? 'risk' : index % 8 === 0 ? 'warn' : index % 5 === 0 ? 'info' : 'ok';
        return <span key={index} className={`is-${tone}`} />;
      })}
    </div>
  );
}

function FieldDriftMatrix() {
  const fields = ['五元组', 'community_id', 'tenant', 'asset_id', 'protocol', 'timestamp', 'ja3', 'campaign_id'];
  return (
    <div className="taf-data-quality-field-drift">
      {fields.map((field, index) => (
        <div key={field} className={index % 7 === 0 ? 'is-risk' : index % 4 === 0 ? 'is-warn' : 'is-ok'}>
          <strong>{field}</strong>
          <span>缺失 {(0.18 + index * 0.41).toFixed(2)}%</span>
          <span>异常 {(0.05 + index * 0.13).toFixed(2)}%</span>
        </div>
      ))}
    </div>
  );
}

function GateList() {
  const gates = [
    ['基线样本覆盖', '通过', 'ok'],
    ['Schema Drift', '观察', 'warn'],
    ['幂等键校验', '通过', 'ok'],
    ['跨租户隔离', '通过', 'ok'],
    ['DLQ 回放审批', '待审批', 'info'],
  ];
  return (
    <div className="taf-data-quality-gates">
      {gates.map(([label, value, tone]) => (
        <span key={label} className={`is-${tone}`}>
          <CheckCircleOutlined />
          <strong>{label}</strong>
          <em>{value}</em>
        </span>
      ))}
    </div>
  );
}

function FlinkQuality({ score, evidence, visuals }: { score: number; evidence: PageSnapshot['evidence']; visuals?: DataQualityVisuals }) {
  const items = visuals?.flinkMetrics ?? buildFallbackFlinkMetrics(score, evidence);
  return (
    <div className="taf-data-quality-flink">
      <div className="taf-data-quality-flink-list">
        {items.map(({ description, label, status, value }) => (
          <div key={label} className={`is-${status}`} title={`${label} ${value} ${description}`}>
            <span title={label}>{label}</span>
            <strong title={value}>{value}</strong>
            <em title={description}>{description}</em>
          </div>
        ))}
      </div>
      <FlinkWatermarkTrend evidence={evidence} trend={visuals?.flinkTrend} />
    </div>
  );
}

function buildFallbackFlinkMetrics(score: number, evidence: PageSnapshot['evidence']): DataQualityVisuals['flinkMetrics'] {
  const checkpoint = evidence.find((item) => item.label.includes('Checkpoint'))?.value ?? '99.2%';
  return [
    { label: '运行作业', value: '58', description: score >= 90 ? '全部 RUNNING' : '存在告警', status: score >= 90 ? 'ok' : 'warn' },
    { label: 'Checkpoint 成功率', value: checkpoint, description: '最近 15 分钟', status: checkpoint.includes('异常') ? 'risk' : 'ok' },
    { label: 'Backpressure', value: '0.38', description: '6 个 task 观察', status: 'warn' },
    { label: 'Watermark 延迟 P95', value: '1.6s', description: '阈值 3s', status: 'ok' },
    { label: '迟到数据率', value: '0.67%', description: '较昨日 ↓ 0.09%', status: 'warn' },
    { label: '错误事件数', value: '312', description: '近 24 小时', status: 'risk' },
  ];
}

function FlinkWatermarkTrend({ evidence, trend }: { evidence: PageSnapshot['evidence']; trend?: DataQualityVisuals['flinkTrend'] }) {
  const chart = trend ?? {
    times: ['03:40', '07:40', '11:40', '15:40', '19:40'],
    p50: [68, 64, 66, 60, 62, 56, 58, 53, 55, 50, 52],
    p95: [52, 48, 55, 44, 50, 38, 42, 34, 40, 30, 36],
    threshold: [46, 46, 46, 46, 46, 46, 46, 46, 46, 46, 46],
  };
  return (
    <div className="taf-data-quality-flink-trend" title={evidence.map((item) => `${item.label} ${item.value}`).join(' / ')}>
      <header>
        <span>Watermark 延迟趋势</span>
        <em>P50 / P95 / 阈值</em>
      </header>
      <DataQualityTrendChart
        ariaLabel="Watermark 延迟 P50 P95 阈值趋势"
        className="taf-data-quality-watermark-echart"
        categories={chart.times}
        series={[
          { name: 'P50', color: '#18a8ff', values: chart.p50 },
          { name: 'P95', color: '#ffb020', values: chart.p95 },
          { name: '阈值', color: '#ff4d4f', dashed: true, values: chart.threshold },
        ]}
      />
    </div>
  );
}

function FieldQuality() {
  return (
    <div className="taf-data-quality-field">
      <div>
        <span title="字段">字段</span>
        <span title="完整率">完整率</span>
        <span title="准确率">准确率</span>
        <span title="缺失率">缺失率</span>
        <span title="异常率">异常率</span>
        <span title="唯一值占比">唯一值占比</span>
        <span title="趋势">趋势</span>
        <span title="状态">状态</span>
        <span title="操作">操作</span>
      </div>
      {fieldRows.map(([field, completeness, accuracy, missing, abnormal, uniqueRate, status, action, tone], index) => (
        <button key={field} type="button" className={`is-${tone}`} title={`${field} 完整率 ${completeness} 准确率 ${accuracy} 缺失率 ${missing} 异常率 ${abnormal} 唯一值占比 ${uniqueRate} 状态 ${status}`}>
          <strong title={field}>{field}</strong>
          <span title={completeness}>{completeness}</span>
          <span title={accuracy}>{accuracy}</span>
          <span title={missing}>{missing}</span>
          <span title={abnormal}>{abnormal}</span>
          <span title={uniqueRate}>{uniqueRate}</span>
          <span className="taf-data-quality-field-trend" title={`${field} 趋势`}>
            <TopicSparkline index={index} tone={tone === 'risk' ? '上升' : tone === 'warn' ? '波动' : '下降'} />
          </span>
          <span title={status}>{status}</span>
          <span className="taf-data-quality-field-action" title={action}><FileSearchOutlined /></span>
        </button>
      ))}
    </div>
  );
}

function ReconciliationReport() {
  return (
    <DenseRows
      columns={['时间窗', '采集总量', '入库总量', '差异量', '差异率', '状态']}
      rows={[
        ['06-20 02:00 - 03:00', '1.28 B', '1.27 B', '12.6 M', '0.98%', '通过'],
        ['06-20 01:00 - 02:00', '1.31 B', '1.30 B', '13.2 M', '1.01%', '通过'],
        ['06-20 00:00 - 01:00', '1.29 B', '1.28 B', '14.1 M', '1.09%', '通过'],
      ]}
    />
  );
}

function QualityAnomalies() {
  return (
    <div className="taf-data-quality-anomalies">
      {anomalyRows.map(([title, target, value, time, tone]) => (
        <button key={`${title}-${target}`} type="button" className={`is-${tone}`} title={`${title} ${target} ${value} ${time}`}>
          <i aria-hidden="true" />
          <strong title={title}>{title}</strong>
          <span title={target}>{target}</span>
          <em title={`${value} ${time}`}>{value}<b title={time}>{time}</b></em>
        </button>
      ))}
    </div>
  );
}

function QuickLocate({ onOpenDlqSample }: { onOpenDlqSample: () => void }) {
  return (
    <div className="taf-data-quality-locate">
      {locateRows.map(([type, target, value]) => (
        <button key={target} type="button" title={`${type} ${target} ${value}`} onClick={type.includes('DLQ') ? onOpenDlqSample : undefined}>
          <SearchOutlined />
          <strong title={type}>{type}</strong>
        </button>
      ))}
    </div>
  );
}

function RepairAdvice({ onReplay }: { onReplay?: () => void }) {
  const items: Array<[string, string, string]> = [
    ['建议对 dlq_topic 执行重放 (12,845 条)', '先校验 schema drift 与幂等 key', '创建任务'],
    ['检查 threat_alerts 消费组性能瓶颈', 'consumer lag 与 checkpoint 关联复核', '处理'],
    ['优化 flow_original 分区分布策略', '对严重倾斜分区做重分配评估', '优化'],
    ['补齐 asset_id 字段映射规则', '字段缺失样本回流到融合规则', '配置'],
  ];
  return (
    <div className="taf-data-quality-advice">
      {items.map(([title, desc, action]) => (
        <button key={String(title)} type="button" title={`${title} ${desc} ${action}`} onClick={title.includes('dlq_topic') ? onReplay : undefined}>
          <i aria-hidden="true" />
          <strong title={title}>{title}</strong>
          <span title={desc}>{desc}</span>
          <em title={action}>{action}</em>
        </button>
      ))}
      <a className="taf-data-quality-advice-more" href="#quality-advice-all" title="查看全部建议">查看全部建议 <ArrowUpOutlined /></a>
    </div>
  );
}

function EvidenceActions({ evidence }: { evidence: PageSnapshot['evidence'] }) {
  const actions: Array<[string, ReactNode]> = [
    ['生成质量报告', <FileDoneOutlined key="report" />],
    ['导出对账报告', <DownloadOutlined key="download" />],
    ['合规审计证据', <FileSearchOutlined key="evidence" />],
    ['SLA 验收包', <SafetyCertificateOutlined key="sla" />],
  ];
  return (
    <div className="taf-data-quality-actions" title={evidence.map((item) => `${item.label} ${item.value}`).join(' / ')}>
      <div className="taf-data-quality-action-grid">
        {actions.map(([label, icon]) => (
          <Button key={String(label)} size="small" icon={icon} title={String(label)}>{label}</Button>
        ))}
      </div>
    </div>
  );
}

const renderQualityCell = (column: string, value: unknown) => {
  if (column === 'Topic') return <span className="taf-data-quality-topic" title={String(value)}><ApiOutlined />{String(value)}</span>;
  if (column === '积压趋势' || column === '操作') return <StatusTag value={value} />;
  if (column.includes('延迟') || column.includes('倾斜')) return <span className="taf-data-quality-warn" title={String(value)}>{String(value)}</span>;
  return <span title={String(value)}>{String(value)}</span>;
};

const defaultDLQReplayRequest = (): DLQReplayFallbackRequest => {
  const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
  const approvalId = `APPROVAL-${stamp}-DQ-DLQ`;
  return buildDLQReplayDryRunRequest({
    approved_by: 'operator-2',
    approval_id: approvalId,
    idempotency_key: `tenant-a:${approvalId}:dry-run`,
    reason: 'schema repair 后验证 fallback 文件可安全回放',
    repair_summary: '已完成 schema drift 和字段缺失修复，先执行 dry-run 预检',
  });
};

const errorText = (value: unknown) => (value instanceof Error ? value.message : 'DLQ replay dry-run 请求失败');

const formatBytes = (value: number) => {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${value} B`;
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label === '质量总分' ? '92 分' : label.includes('率') || label.includes('性') ? '92.0%' : '0',
  delta: 'API',
  status: 'info',
});
