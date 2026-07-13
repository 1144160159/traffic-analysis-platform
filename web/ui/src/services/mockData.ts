import type { PageSpec } from '@/routes/routeManifest';

export type SnapshotRow = Record<string, string | number>;

export type ScreenVisualPoint = {
  name: string;
  x: number;
  y: number;
  value?: number;
  level?: 'low' | 'medium' | 'high';
};

export type ScreenWorldPoint = {
  name: string;
  coord: [number, number];
  value: number;
  level?: 'low' | 'medium' | 'high';
};

export type ScreenWorldFlow = {
  name: string;
  from: [number, number];
  to: [number, number];
  value: number;
  level?: 'low' | 'medium' | 'high';
};

export type ScreenAbnormalLink = {
  name: string;
  linkCount: number;
  assetCount: number;
  level?: 'low' | 'medium' | 'high';
};

export type ScreenEvidenceRing = {
  label: string;
  value: number;
  caption: string;
  href: string;
  level?: 'low' | 'medium' | 'high';
};

export type ScreenVisualNode = {
  id: string;
  label: string;
  meta?: string;
  type?: string;
  x: number;
  y: number;
  tone?: 'ok' | 'warn' | 'info' | 'risk';
  status?: 'online' | 'offline' | 'maintenance';
  probes?: string;
  links?: string;
  assets?: string;
  riskScore?: number;
  bandwidth?: string;
  href?: string;
};

export type ScreenVisualEdge = {
  from: string;
  to: string;
  tone?: 'core' | 'converge' | 'risk';
  width?: number;
};

export type ScreenVisuals = {
  probeMapNodes: ScreenVisualNode[];
  probeMapLinks: Array<[string, string]>;
  topologyNodes: ScreenVisualNode[];
  topologyEdges: ScreenVisualEdge[];
  campaignDensityPoints: ScreenVisualPoint[];
  riskMapPoints: ScreenWorldPoint[];
  egressMapPoints: ScreenWorldPoint[];
  egressMapFlows: ScreenWorldFlow[];
  abnormalLinks: ScreenAbnormalLink[];
  evidenceRings: ScreenEvidenceRing[];
};

export type DashboardHealthGate = {
  component: string;
  status: string;
  reason: string;
  scope: string;
  updated: string;
};

export type DashboardStage = {
  label: string;
  value: string;
  footnote: string;
  status: 'ok' | 'warn' | 'risk' | 'info';
  bars: number[];
  slaPercent?: number;
  pressurePercent?: number;
  action?: string;
};

export type DashboardQualityRing = {
  label: string;
  value: string;
  ringPercent: number;
  status: 'ok' | 'warn' | 'risk' | 'info';
  subtext: string;
};

export type DashboardTalker = {
  label: string;
  value: number;
};

export type DashboardVisuals = {
  kpiSparks: number[][];
  healthGates: DashboardHealthGate[];
  stages: DashboardStage[];
  qualityRings: DashboardQualityRing[];
  topTalkers: DashboardTalker[];
};

export type DataQualityVisuals = {
  topicMetrics: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  heatmap: Array<{ label: string; values: Array<'ok' | 'info' | 'warn' | 'risk'> }>;
  heatmapTimes: string[];
  heatmapLegend: Array<{ label: string; status: 'ok' | 'info' | 'warn' | 'risk' }>;
  consumerRows: string[][];
  messageSizeDistribution: Array<{ label: string; value: number }>;
  messageSizeTopicRows: string[][];
  partitionQueueRows: string[][];
  fieldKpis: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  fieldKpiTrends: number[][];
  fieldQualityRows: string[][];
  fieldTrend: {
    times: string[];
    missing: number[];
    format: number[];
    mapping: number[];
    timeDrift: number[];
    unknownProtocol: number[];
  };
  fieldTrendSummary: string[][];
  communityCheckRows: string[][];
  communityMismatchRows: string[][];
  fieldAnomalyRows: string[][];
  fieldLineageRows: string[][];
  fieldRepairRows: string[][];
  storageKpis: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  storageComponentRows: string[][];
  storageTrend: {
    times: string[];
    clickhouse: number[];
    opensearch: number[];
    nebula: number[];
    minio: number[];
    latencyP95: number[];
    latencySla: number[];
  };
  storageCapacityTrend: {
    days: string[];
    clickhouse: number[];
    opensearch: number[];
    nebula: number[];
    minio: number[];
    threshold: number[];
  };
  storageFailureRows: string[][];
  storagePipelineRows: Array<{ from: string; to: string; label: string; status: 'ok' | 'info' | 'warn' | 'risk' }>;
  storageReplicaRows: string[][];
  storageIndexHealth: Array<{ label: string; value: number; status: 'ok' | 'info' | 'warn' | 'risk' }>;
  storagePartitionRows: string[][];
  storageObjectRows: string[][];
  storageRailAlerts: string[][];
  storageRailLocateRows: string[];
  storageRailRepairRows: string[];
  storageRailEvidenceRows: string[];
  replayKpis: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  replayTaskRows: string[][];
  replayReconcileTrend: {
    times: string[];
    sourceTotal: number[];
    sinkTotal: number[];
    diffCount: number[];
    diffRate: number[];
    diffRateThreshold: number[];
  };
  replayReconcileSummary: string[][];
  replayIdempotencyRows: string[][];
  replayDifferenceRows: string[][];
  replayFlowNodes: Array<{ id: string; label: string; detail: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  replayFlowEdges: Array<{ from: string; to: string; label: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  replayEvidenceRows: string[][];
  replayRailAlerts: string[][];
  replayRailLocateRows: string[];
  replayRailRepairRows: string[];
  replayRailEvidenceRows: string[];
  flinkKpis: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  flinkJobRows: string[][];
  flinkCheckpointTrend: {
    times: string[];
    checkpointDuration: number[];
    checkpointAge: number[];
    watermarkP95: number[];
    watermarkSla: number[];
    checkpointSla: number[];
  };
  flinkBackpressureBuckets: string[];
  flinkBackpressureRows: Array<{ label: string; values: Array<'ok' | 'info' | 'warn' | 'risk'> }>;
  flinkLateTopicRows: string[][];
  flinkWindowRows: string[][];
  flinkFailureRows: string[][];
  flinkSinkRows: Array<{ name: string; status: string; eps: string; success: string; p95: string; retries: string; trend: number[] }>;
  flinkMetrics: Array<{ label: string; value: string; description: string; status: 'ok' | 'info' | 'warn' | 'risk' }>;
  flinkTrend: {
    times: string[];
    p50: number[];
    p95: number[];
    threshold: number[];
  };
};

export type EncryptedTrafficVisuals = {
  protocolRows: string[][];
  protocolTrend: number[];
  ja3Rows: string[][];
  scatterPoints: Array<{ left: number; top: number; tone: 'ok' | 'warn' | 'risk' | 'info' }>;
  tunnelCards: string[][];
  tunnelRows: string[][];
  destinationRows: string[][];
  adviceRows: string[][];
  certificateRows: string[][];
  tunnelRuleRows: string[][];
  evidenceRows: string[][];
  egressKpis: string[][];
  egressDomainCards: string[][];
  egressMapNodes: Array<{
    id: string;
    label: string;
    location: string;
    flow: string;
    sessions: string;
    risk: string;
    x: number;
    y: number;
  }>;
  egressTrend: {
    labels: string[];
    series: Array<{
      name: string;
      color: string;
      values: number[];
    }>;
  };
  egressAvailability: {
    state: 'live' | 'partial' | 'simulated' | 'unavailable';
    detail: string;
  };
  heartbeatBars: number[];
  evidenceCenter: {
    availability: {
      state: 'live' | 'partial' | 'simulated' | 'unavailable';
      detail: string;
    };
    kpis: string[][];
    sessions: Array<{
      time: string;
      sessionId: string;
      source: string;
      destination: string;
      protocol: string;
      sni: string;
      ja3: string;
      alpn: string;
      certificateHash: string;
      pcapIndex: string;
      risk: string;
      entropy: number;
    }>;
    pcapRows: string[][];
    pcapTrend: Array<{ label: string; value: number }>;
    entropyTrend: Array<{ label: string; value: number }>;
    certificateDetails: Array<{ label: string; value: string }>;
    handshakeTimeline: Array<{ time: string; event: string; detail: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
    completeness: Array<{ label: string; complete: number; total: number; status: 'ok' | 'warn' | 'risk' | 'info' }>;
    hashRows: string[][];
  };
};

export type PageSnapshot = {
  id: string;
  total?: number;
  metrics: Array<{ label: string; value: string; delta: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  rows: SnapshotRow[];
  timeline: Array<{ title: string; description: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  evidence: Array<{ label: string; value: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  visuals?: {
    dashboard?: DashboardVisuals;
    screen?: ScreenVisuals;
    dataQuality?: DataQualityVisuals;
    encryptedTraffic?: EncryptedTrafficVisuals;
  };
};

const statusCycle: PageSnapshot['metrics'][number]['status'][] = ['risk', 'warn', 'info', 'ok'];

export const buildVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  if (page.id === 'dashboard') return buildDashboardVisualBreakdownSnapshot(page);
  if (page.id === 'alerts') return buildAlertsVisualBreakdownSnapshot(page);
  if (page.id === 'campaigns') return buildCampaignsVisualBreakdownSnapshot(page);
  if (page.id === 'probes') return buildProbesVisualBreakdownSnapshot(page);
  if (page.id === 'data-quality') return buildDataQualityVisualBreakdownSnapshot(page);
  if (page.id === 'topic-exfil') return buildTopicExfilVisualBreakdownSnapshot(page);
  if (page.id === 'topic-apt') return buildTopicAptVisualBreakdownSnapshot(page);
  return buildPageSnapshot(page);
};

export const buildPageSnapshot = (page: PageSpec): PageSnapshot => {
  const metrics = page.kpis.slice(0, 8).map((label, index) => ({
    label,
    value: metricValue(page.id, label, index),
    delta: index % 3 === 0 ? '+12' : index % 3 === 1 ? '-3' : '+4.8%',
    status: statusCycle[index % statusCycle.length],
  }));

  const rows = Array.from({ length: 8 }, (_, index) =>
    Object.fromEntries(
      page.tableColumns.map((column, columnIndex) => [
        column,
        cellValue(page.id, column, index, columnIndex),
      ]),
    ),
  );

  const timeline = ['首次发现', '异常行为', '证据生成', '处置动作', '审计留痕'].map((title, index) => ({
    title,
    description: `${page.title} ${title}已关联上下文，Trace-${index + 1}${page.id.slice(0, 3).toUpperCase()}`,
    status: statusCycle[(index + 1) % statusCycle.length],
  }));

  const evidence = (page.evidence.length ? page.evidence : ['PCAP', 'Session', '日志', '图谱路径']).map((label, index) => ({
    label,
    value: index % 2 === 0 ? `${92 + index}.6%` : `${12 + index * 7} 项`,
    status: statusCycle[(index + 2) % statusCycle.length],
  }));

  return { id: page.id, total: rows.length, metrics, rows, timeline, evidence };
};

const buildProbesVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  const targetMetrics: Record<string, { value: string; delta: string; status: PageSnapshot['metrics'][number]['status'] }> = {
    探针总数: { value: '25', delta: '↑ 2', status: 'info' },
    在线探针: { value: '24', delta: '在线率 96.0%', status: 'ok' },
    采集网卡: { value: '48', delta: '↑ 3', status: 'info' },
    采集模式: { value: '4', delta: '混合采集', status: 'warn' },
    '平均 CPU': { value: '32.6%', delta: '', status: 'ok' },
    平均内存: { value: '41.3%', delta: '', status: 'info' },
    告警探针: { value: '3', delta: '', status: 'risk' },
    离线探针: { value: '1', delta: '', status: 'risk' },
  };
  const metrics = page.kpis.slice(0, 8).map((label) => ({
    label,
    ...(targetMetrics[label] ?? { value: metricValue(page.id, label, 0), delta: '实时', status: 'info' as const }),
  }));
  const rows = Array.from({ length: 7 }, (_, index) =>
    Object.fromEntries(
      page.tableColumns.map((column, columnIndex) => [
        column,
        cellValue(page.id, column, index, columnIndex),
      ]),
    ),
  );
  return {
    id: page.id,
    total: 25,
    metrics,
    rows,
    timeline: heartbeatProbeTimeline(),
    evidence: [
      { label: '心跳同步', value: '正常 1s 前', status: 'ok' },
      { label: 'mTLS', value: '已启用', status: 'ok' },
      { label: '接口状态', value: '94.6%', status: 'ok' },
      { label: '批量发送', value: '33 项', status: 'info' },
      { label: '审计记录', value: '96.6%', status: 'ok' },
    ],
  };
};

const buildDataQualityVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  const targetMetrics: Record<string, { value: string; delta: string; status: PageSnapshot['metrics'][number]['status'] }> = {
    质量总分: { value: '92', delta: '较昨日 ↑ 2 分  评估时间 2026-06-20 03:40', status: 'ok' },
    完整性: { value: '96.3%', delta: '较昨日 ↑ 1.8%', status: 'ok' },
    及时性: { value: '91.7%', delta: '较昨日 ↓ 0.6%', status: 'warn' },
    准确性: { value: '93.8%', delta: '较昨日 ↑ 1.8%', status: 'ok' },
    重复率: { value: '0.42%', delta: '较昨日 ↑ 0.08%', status: 'ok' },
    字段缺失率: { value: '1.12%', delta: '较昨日 ↓ 0.09%', status: 'risk' },
    'DLQ 数量': { value: '12,845', delta: '较昨日 ↑ 843', status: 'risk' },
  };
  const rows = [
    qualityTopicSnapshotRow('flow_original', '48', '98.7 M', '1.2 s', '3.21 M', '波动', '780 ms', '1.12', '1.6 KB', '正常', '142.3M', '1.23M', '1.1s', '1.32', '1.5KB', '健康'),
    qualityTopicSnapshotRow('flow_enriched', '48', '96.3 M', '0.9 s', '1.84 M', '下降', '650 ms', '1.08', '2.1 KB', '正常', '128.6M', '0.86M', '1.3s', '1.45', '1.7KB', '健康'),
    qualityTopicSnapshotRow('dns_logs', '24', '45.6 M', '1.6 s', '2.36 M', '下降', '890 ms', '1.35', '0.9 KB', '正常', '36.7M', '0.21M', '0.6s', '1.12', '0.9KB', '健康'),
    qualityTopicSnapshotRow('tls_logs', '24', '32.8 M', '1.1 s', '1.21 M', '下降', '710 ms', '1.07', '1.2 KB', '正常', '28.9M', '0.18M', '0.7s', '1.08', '1.2KB', '健康'),
    qualityTopicSnapshotRow('asset_events', '16', '18.2 M', '0.8 s', '0.52 M', '下降', '540 ms', '1.03', '0.7 KB', '正常', '18.4M', '0.12M', '0.5s', '1.05', '0.8KB', '健康'),
    qualityTopicSnapshotRow('threat_alerts', '12', '12.6 M', '2.3 s', '0.84 M', '波动', '1.40 s', '1.22', '1.8 KB', '中等', '8.6M', '0.43M', '2.8s', '2.48', '1.1KB', '告警'),
    qualityTopicSnapshotRow('dlq_topic', '6', '--', '--', '12,845', '上升', '--', '--', '1.1 KB', '危急', '1.2M', '0.18M', '15.6s', '5.83', '1.0KB', '严重'),
  ];

  return {
    id: page.id,
    total: rows.length,
    metrics: page.kpis.map((label) => ({
      label,
      ...(targetMetrics[label] ?? { value: metricValue(page.id, label, 0), delta: '实时', status: 'info' as const }),
    })),
    rows,
    timeline: [
      { title: 'Kafka Topic 健康', description: 'Topic 吞吐、消费延迟、积压、分区倾斜和消息延迟 P95 已按目标图映射。', status: 'ok' },
      { title: 'Flink 处理质量', description: '运行作业、checkpoint、backpressure、watermark、迟到数据和错误事件由 typed fallback 驱动。', status: 'ok' },
      { title: '字段质量矩阵', description: '字段完整率、准确率、缺失率、异常率和唯一值占比进入组件化表格。', status: 'ok' },
      { title: '存储写入质量', description: 'ClickHouse、OpenSearch、NebulaGraph、MinIO 写入质量进入密集明细。', status: 'ok' },
      { title: '重放对账', description: 'DLQ 与重放对账动作通过页面 API 契约和 dry-run 门禁关联。', status: 'warn' },
    ],
    evidence: [
      { label: '质量基线', value: '92.6%', status: 'info' },
      { label: 'Kafka Topic', value: '19 项', status: 'ok' },
      { label: 'Flink Checkpoint', value: '94.6%', status: 'risk' },
      { label: '字段矩阵', value: '33 项', status: 'warn' },
      { label: '存储写入', value: '96.6%', status: 'info' },
      { label: '重放对账', value: '99.12%', status: 'ok' },
    ],
    visuals: {
      dataQuality: buildDataQualityVisuals(),
    },
  };
};

const buildCampaignsVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  const targetMetrics: Record<string, { value: string; delta: string; status: PageSnapshot['metrics'][number]['status'] }> = {
    战役总数: { value: '58', delta: '', status: 'info' },
    活跃战役: { value: '12', delta: '', status: 'ok' },
    影响资产: { value: '236 台', delta: '', status: 'info' },
    最高风险: { value: '高风险', delta: '', status: 'risk' },
    告警总数: { value: '1,246 条', delta: '', status: 'warn' },
    平均持续时间: { value: '3 天 14 小时', delta: '', status: 'info' },
  };
  const rows = Array.from({ length: 8 }, (_, index) =>
    Object.fromEntries(page.tableColumns.map((column) => [column, campaignCellValue(column, index)])),
  );

  return {
    id: page.id,
    total: 58,
    metrics: page.kpis.map((label) => ({
      label,
      ...(targetMetrics[label] ?? { value: metricValue(page.id, label, 0), delta: '', status: 'info' as const }),
    })),
    rows,
    timeline: [
      { title: '首次发现', description: '06-19 09:12 发现 RedLync 初始访问与多条告警簇。', status: 'info' },
      { title: '异常行为', description: '执行、横向移动与外联通信进入关联分析。', status: 'warn' },
      { title: '证据生成', description: '告警、PCAP、Session、日志与图谱路径持续补齐。', status: 'ok' },
      { title: '处置动作', description: '阻断外联、下钻攻击链并生成 SOAR 处置上下文。', status: 'risk' },
      { title: '审计留痕', description: '战役状态、负责人和处置动作写入审计 trace。', status: 'warn' },
    ],
    evidence: [
      { label: '告警', value: '234 / 312', status: 'ok' },
      { label: 'PCAP / Session', value: '86 / 128', status: 'warn' },
      { label: '日志', value: '1,432 / 2,150', status: 'ok' },
      { label: '图谱路径', value: '12 / 18', status: 'warn' },
      { label: '处置记录', value: '8 / 10', status: 'risk' },
    ],
  };
};

const qualityTopicSnapshotRow = (
  topic: string,
  partitions: string,
  throughput: string,
  latency: string,
  backlog: string,
  trend: string,
  p95: string,
  skew: string,
  messageP95: string,
  action: string,
  currentOffset: string,
  backlogValue: string,
  p95Value: string,
  partitionSkewValue: string,
  messageSize: string,
  state: string,
): SnapshotRow => ({
  Topic: topic,
  分区数: partitions,
  当前吞吐量: throughput,
  消费延迟: latency,
  积压量: backlog,
  积压趋势: trend,
  '消费延迟 P95': p95,
  分区倾斜: skew,
  '消息延迟 P95': messageP95,
  操作: action,
  '当前 offset': currentOffset,
  积压: backlogValue,
  '消费延迟P95': p95Value,
  分区倾斜度: partitionSkewValue,
  消息大小: messageSize,
  状态: state,
});

const buildDataQualityVisuals = (): DataQualityVisuals => {
  const heatmapValues: DataQualityVisuals['heatmap'][number]['values'][] = [
    ['info', 'info', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'info', 'info', 'ok', 'info', 'warn', 'ok', 'info', 'info'],
    ['info', 'info', 'info', 'ok', 'warn', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'warn', 'risk', 'info', 'ok', 'info'],
    ['ok', 'ok', 'ok', 'ok', 'info', 'ok', 'info', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'info'],
    ['warn', 'warn', 'risk', 'warn', 'warn', 'warn', 'risk', 'warn', 'risk', 'risk', 'risk', 'warn', 'risk', 'warn', 'warn', 'warn'],
    ['risk', 'risk', 'warn', 'warn', 'warn', 'warn', 'warn', 'risk', 'warn', 'warn', 'risk', 'risk', 'warn', 'risk', 'warn', 'warn'],
    ['info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info'],
  ];

  return {
    topicMetrics: [
      { label: 'Topic 健康分', value: '88/100', delta: '较昨日 ↑ 3', status: 'ok' },
      { label: '总 offset', value: '412.7M', delta: '24h 变化 ↑ 18.6M', status: 'info' },
      { label: '积压消息', value: '3.21M', delta: '较昨日 ↑ 0.72M', status: 'warn' },
      { label: '消费延迟 P95', value: '1.4s', delta: '较昨日 ↓ 0.6s', status: 'ok' },
      { label: '分区倾斜', value: '2.15', delta: '较昨日 ↑ 0.38', status: 'warn' },
      { label: '平均消息大小', value: '1.6KB', delta: '较昨日 ↑ 0.1KB', status: 'info' },
      { label: '异常 Topic', value: '3', delta: '较昨日 ↑ 1', status: 'risk' },
    ],
    heatmap: ['0-7', '8-15', '16-23', '24-31', '32-39', '40-47'].map((label, index) => ({
      label,
      values: heatmapValues[index],
    })),
    heatmapTimes: ['15:30', '19:30', '23:30', '03:30', '07:30', '11:30', '15:30'],
    heatmapLegend: [
      { label: '均衡 (<1.2)', status: 'info' },
      { label: '轻度倾斜 (1.2-2)', status: 'ok' },
      { label: '严重倾斜 (>2)', status: 'risk' },
    ],
    consumerRows: [
      ['session-job', '1.23M', '2', '15:30:12', '健康'],
      ['feature-job', '0.86M', '1', '15:30:08', '健康'],
      ['rule-job', '0.54M', '3', '15:29:58', '健康'],
      ['pcap-index-job', '0.43M', '0', '15:30:15', '健康'],
      ['behavior-job', '0.21M', '1', '15:29:55', '健康'],
    ],
    messageSizeDistribution: [
      { label: '<0.5KB', value: 12 },
      { label: '0.5-1KB', value: 21 },
      { label: '1-2KB', value: 36 },
      { label: '2-4KB', value: 28 },
      { label: '4-8KB', value: 14 },
      { label: '>8KB', value: 6 },
      { label: '>9KB', value: 2 },
    ],
    messageSizeTopicRows: [
      ['flow_original', '1.5', '14.2', '18,734', '3.2x'],
      ['flow_enriched', '1.7', '16.8', '15,962', '3.1x'],
      ['rule_logs', '0.9', '4.3', '8,521', '2.8x'],
      ['tls_logs', '1.2', '6.7', '6,342', '2.9x'],
      ['asset_events', '0.8', '3.6', '3,421', '2.6x'],
      ['threat_alerts', '1.1', '8.5', '1,842', '2.7x'],
      ['dlq_topic', '1.0', '5.1', '256', '1.0x'],
    ],
    partitionQueueRows: [
      ['dlq_topic', '2', '消费延迟 18.2s', '下游消息异常积压', '消息组复位', '定位'],
      ['dlq_topic', '5', '消费延迟 15.1s', '消费链路处理堆积', '扩容消费者并优化处理', '修复'],
      ['threat_alerts', '7', '倾斜度 3.12', '分区数据分布不均', '评估扩分区重新分配数据', '评估'],
    ],
    fieldKpis: [
      { label: '字段质量分', value: '94/100', delta: '较昨日 ↑ 3', status: 'ok' },
      { label: '完整率', value: '98.7%', delta: '较昨日 ↑ 0.8%', status: 'ok' },
      { label: '格式合规', value: '97.9%', delta: '较昨日 ↑ 1.2%', status: 'ok' },
      { label: '一致性', value: '96.4%', delta: '较昨日 ↑ 0.6%', status: 'ok' },
      { label: '异常字段', value: '23 项', delta: '较昨日 ↑ 5', status: 'risk' },
      { label: '影响记录', value: '18.4K 条', delta: '较昨日 ↑ 2.1K', status: 'warn' },
      { label: '待修复任务', value: '7 个', delta: '较昨日 ↓ 1', status: 'warn' },
    ],
    fieldKpiTrends: [
      [89, 90, 90, 91, 92, 91, 93, 92, 93, 94, 93, 94],
      [97.2, 97.6, 97.4, 98.1, 97.8, 98.2, 97.9, 98.4, 98.1, 98.5, 98.3, 98.7],
      [96.5, 96.8, 97.1, 96.9, 97.4, 97.2, 97.7, 97.4, 97.9, 97.6, 97.8, 97.9],
      [94.8, 95.2, 95.5, 95.1, 95.6, 95.9, 95.5, 96.1, 95.8, 96.2, 96.0, 96.4],
      [18, 20, 17, 21, 19, 23, 22, 25, 20, 24, 21, 23],
      [15.2, 16.8, 15.9, 17.3, 16.1, 18.7, 17.4, 19.1, 17.8, 18.9, 17.7, 18.4],
      [10, 9, 11, 8, 9, 7, 8, 7, 6, 8, 7, 7],
    ],
    fieldQualityRows: [
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
    ],
    fieldTrend: {
      times: ['15:30', '19:30', '23:30', '03:30', '07:30', '11:30', '15:30'],
      missing: [220, 640, 980, 1850, 3650, 4520, 3080, 2610, 2260, 1940, 1820, 2160, 2480, 1880],
      format: [160, 430, 760, 1260, 2260, 3130, 2580, 2320, 2040, 1720, 1540, 1880, 2130, 1760],
      mapping: [120, 360, 590, 980, 1680, 2310, 2180, 1920, 1720, 1460, 1280, 1510, 1830, 1390],
      timeDrift: [70, 160, 270, 440, 790, 1160, 1020, 870, 760, 650, 580, 690, 820, 620],
      unknownProtocol: [40, 90, 150, 230, 410, 560, 520, 470, 420, 350, 310, 380, 470, 360],
    },
    fieldTrendSummary: [
      ['缺失值', '7,235', 'info'],
      ['格式不合法', '5,134', 'ok'],
      ['映射不一致', '3,126', 'ok'],
      ['时间漂移', '1,738', 'ok'],
      ['未知协议', '1,167', 'ok'],
    ],
    communityCheckRows: [
      ['五元组 → community_id', '124,356', '120,865', '3,491', '97.19%'],
      ['哈希碰撞告警', '124,356', '124,353', '3', '99.99%'],
      ['协议一致性', '124,356', '123,721', '635', '99.49%'],
    ],
    communityMismatchRows: [
      ['15:29:44', 'sess-71c21d6', '10.12.8.44:321', '172.16.5.10:80', '6', '8a9f3a...', 'a1c9b6...', '原始 cid 缺失'],
      ['15:28:11', 'sess-c4e47e4', '10.23.5.53:911', '192.168.1.20:443', '6', 'c3d4a77...', 'c9d4a77...', '端口反向不一致'],
      ['15:26:55', 'sess-5d4d7e1', '10.0.9.49:83', '8.8.8.8:53', '17', 'e8f2ac...', 'c5f8ac...', '协议异常'],
      ['15:25:37', 'sess-9b0c2f9', '10.14.7.4:0012', '172.16.5.11:80', '6', 'f2b20c...', '5f8a76...', '源目反置'],
      ['15:21:08', 'sess-8efd6a12', '10.12.8.33:544', '172.16.5.12:8080', '6', '7dcd940...', '7dcd940...', '周期 cid 漂移'],
    ],
    fieldAnomalyRows: [
      ['15:29:44', 'traffic_normal', 'tenant', '缺失值', 'null (空)', '__unknown__', 'ACME-APP-12', '查看证据'],
      ['15:28:11', 'asset_inventory', 'asset_id', '缺失值', 'null (空)', '--', '--', '查看证据'],
      ['15:26:55', 'traffic_normal', 'protocol', '未知枚举', '143', '__unknown__', 'WEB-SRV-07', '创建任务'],
      ['15:25:37', 'traffic_session', 'timestamp', '时间漂移', '2025-06-25 14:05:37', '2025-06-26 15:25:37', 'DB-SRV-02', '查看证据'],
      ['15:21:08', 'traffic_normal', 'community_id', '校验不匹配', 'a1c9d4899f9...', '86f95a21cd...', 'WEB-SRV-07', '创建任务'],
      ['15:18:42', 'traffic_normal', 'src_ip', '格式不合法', '999.1.1.1', '10.255.255.255', '--', '创建任务'],
      ['15:09:12', 'alert_event', 'alert_id', '格式不合法', '@ALERT123', 'ALERT-123', 'FW-01', '查看证据'],
      ['14:42:33', 'traffic_session', 'bytes', '负数值', '-1024', '0', '--', '创建任务'],
    ],
    fieldLineageRows: [
      ['traffic_raw', '解析与清洗', '字段映射 A(3)', 'ClickHouse', 'warn'],
      ['traffic_session_raw', '会话构建', '枚举映射 A(5)', 'OpenSearch', 'risk'],
      ['asset_inventory', '资产标准化', '格式校验 A(2)', 'NebulaGraph', 'warn'],
      ['alert_raw', '告警解析', '时间校验 A(4)', 'MinIO', 'warn'],
    ],
    fieldRepairRows: [
      ['补全 tenant 缺失值', 'tenant', '映射：机器 src_ip → 租户所属部门', '张三', '进行中', '2025-06-27', '--', '查看'],
      ['asset_id 补全规则', 'asset_id', '映射：src_ip → asset_id', '李四', '待处理', '2025-06-27', '--', '创建'],
      ['protocol 映射补全', 'protocol', '枚举映射：143 → __unknown__', '王五', '待检查', '2025-06-26', '通过', '查看'],
      ['时间同步校正规则', 'timestamp', '校正：统一为 UTC+8', '赵六', '已完成', '2025-06-26', '通过', '查看'],
      ['community_id 校验修复', 'community_id', '重新计算 SHA-1 并回填', '孙七', '进行中', '2025-06-28', '--', '查看'],
      ['src_ip 格式校正', 'src_ip', '非法 IP → 10.255.255.255', '周八', '待处理', '2025-06-27', '--', '创建'],
      ['bytes 负数归零', 'bytes', '值 < 0 → 0', '吴九', '已完成', '2025-06-26', '通过', '查看'],
    ],
    storageKpis: [
      { label: '存储质量分', value: '93/100', delta: '较昨日 ↑ 4', status: 'ok' },
      { label: '写入成功率', value: '99.84%', delta: '较昨日 ↑ 0.12%', status: 'ok' },
      { label: '写入延迟 P95', value: '420 ms', delta: '较昨日 ↓ 80 ms', status: 'ok' },
      { label: '失败写入', value: '186 条', delta: '较昨日 ↑ 34', status: 'warn' },
      { label: '索引滞后', value: '2.1 s', delta: '较昨日 ↑ 0.4 s', status: 'warn' },
      { label: '归档成功率', value: '99.7%', delta: '较昨日 ↑ 0.2%', status: 'ok' },
      { label: '容量水位', value: '72.6%', delta: '较昨日 ↑ 1.8%', status: 'info' },
    ],
    storageComponentRows: [
      ['ClickHouse', '注意', '78.3 K EPS', '99.82%', '380 ms', '12,356 Distributed 队列', '14.2 TB / 20 TB', '2 shard / 2 replica', '详情'],
      ['OpenSearch', '警告', '12.6 K docs/s', '99.71%', '560 ms', '8,912 Bulk 队列', '6.9 TB / 10 TB', '36 index / 72 shard', '索引滞后 2.1s'],
      ['NebulaGraph', '正常', '2.1 K edges/s', '99.46%', '210 ms', '256 写入队列', '420 GB / 1 TB', '3 partition 健康', '详情'],
      ['MinIO', '注意', '1.8 K objects/s', '99.64%', '690 ms', '1,245 Multipart 队列', '72.4 TB / 120 TB', '8 bucket 生命周期正常', '重试'],
    ],
    storageTrend: {
      times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
      clickhouse: [62, 66, 70, 73, 76, 78, 75, 79, 82, 80, 78, 84, 86, 83, 88, 85],
      opensearch: [32, 35, 37, 39, 41, 45, 43, 44, 48, 46, 47, 50, 52, 49, 55, 53],
      nebula: [18, 20, 22, 21, 24, 25, 23, 27, 26, 28, 29, 30, 32, 31, 34, 33],
      minio: [14, 15, 16, 18, 17, 19, 21, 20, 22, 23, 21, 24, 26, 25, 27, 26],
      latencyP95: [48, 46, 52, 50, 58, 62, 66, 64, 70, 68, 72, 76, 74, 82, 80, 78],
      latencySla: [60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60],
    },
    storageCapacityTrend: {
      days: ['06-20', '06-21', '06-22', '06-23', '06-24', '06-25', '06-26'],
      clickhouse: [54, 56, 58, 61, 64, 67, 70],
      opensearch: [42, 44, 47, 49, 53, 56, 59],
      nebula: [28, 30, 31, 33, 36, 38, 40],
      minio: [60, 62, 64, 67, 69, 71, 73],
      threshold: [80, 80, 80, 80, 80, 80, 80],
    },
    storageFailureRows: [
      ['15:29:44', 'ClickHouse', 'traffic.sessions (Distributed)', '分布式队列积压/写入延迟', '3,912,445', '3', '进行中', '清理队列'],
      ['15:28:11', 'OpenSearch', 'traffic-logs-2025.06.26', 'bulk rejected: thread pool queue full', '812,301', '5', '进行中', '扩容索引'],
      ['15:26:55', 'NebulaGraph', 'edge_upsert', 'edge upsert timeout', '54,812', '2', '重试中', '增加超时'],
      ['15:25:37', 'MinIO', 'pcap-archive/2025/06/26', '超时写入，副本不可用', '412,005', '7', '重试中', '重试任务'],
      ['15:18:08', 'OpenSearch', 'asset-inventory-2025.06', 'mapping 冲突', '24,118', '1', '已结束', '修复映射'],
    ],
    storagePipelineRows: [
      { from: 'Kafka / Flink', to: 'ClickHouse', label: 'session/events 写入', status: 'warn' },
      { from: 'Kafka / Flink', to: 'OpenSearch', label: 'log/index bulk', status: 'risk' },
      { from: 'Kafka / Flink', to: 'NebulaGraph', label: 'entity/edge upsert', status: 'ok' },
      { from: 'Kafka / Flink', to: 'MinIO', label: 'pcap archive multipart', status: 'warn' },
      { from: 'ClickHouse', to: '写入确认', label: 'ack 99.82%', status: 'warn' },
      { from: 'OpenSearch', to: '重试队列 / DLQ', label: 'bulk reject', status: 'risk' },
      { from: 'NebulaGraph', to: '写入确认', label: 'raft commit', status: 'ok' },
      { from: 'MinIO', to: '归档重试', label: 'multipart retry', status: 'warn' },
    ],
    storageReplicaRows: [
      ['ClickHouse 副本', '2 shard / 2 replica', '副本延迟 P95 1.2s', 'Keeper 正常', '注意'],
      ['OpenSearch 分片', '36 index / 72 shard', 'yellow 3 / red 1', 'refresh lag 2.1s', '警告'],
      ['NebulaGraph 分区', '3 partition', 'Raft commit 99.96%', 'leader 均衡', '正常'],
      ['MinIO 生命周期', '8 bucket', '对象总数 1.28 亿', '过期率 0.18%', '注意'],
    ],
    storageIndexHealth: [
      { label: '正常', value: 68, status: 'ok' },
      { label: '警告', value: 3, status: 'warn' },
      { label: '异常', value: 1, status: 'risk' },
    ],
    storagePartitionRows: [
      ['NebulaGraph', 'partition-01', 'leader 正常', 'commit 99.97%'],
      ['NebulaGraph', 'partition-02', 'leader 正常', 'commit 99.96%'],
      ['NebulaGraph', 'partition-03', 'follower lag', 'commit 99.91%'],
    ],
    storageObjectRows: [
      ['Bucket 数', '8'],
      ['对象总数', '1.28 亿'],
      ['生命周期规则', '6 条'],
      ['24h 过期率', '0.18%'],
    ],
    storageRailAlerts: [
      ['中', 'ClickHouse Distributed 队列积压', '12,356', '03:21:44', 'warn'],
      ['高', 'OpenSearch 索引滞后升高', '2.1s', '03:18:12', 'risk'],
      ['中', 'MinIO Multipart 重试较多', '1,245', '03:09:33', 'warn'],
      ['中', 'ClickHouse 写入延迟升高', '380ms', '02:56:41', 'warn'],
      ['高', 'OpenSearch Bulk 拒绝率升高', '0.67%', '02:41:05', 'risk'],
    ],
    storageRailLocateRows: ['定位失败写入', '刷新索引滞后', '组件健康详情', '容量与水位趋势', '写入链路追踪', '映射与故障队列'],
    storageRailRepairRows: ['清理 ClickHouse 分布式队列', '优化 OpenSearch 索引分片', '创建归档重试任务', '检查 MinIO 生命周期策略', '查看修复工单'],
    storageRailEvidenceRows: ['导出存储质量报告', '导出异常明细', '延迟报告下载', '近期历史报告', '证据包快照'],
    replayKpis: [
      { label: '对账通过率', value: '99.12%', delta: '较昨日 ↑ 0.42%', status: 'ok' },
      { label: '待重放 DLQ', value: '12,845', delta: '较昨日 ↓ 843', status: 'warn' },
      { label: '重放成功率', value: '98.6%', delta: '较昨日 ↑ 0.61%', status: 'ok' },
      { label: '重复记录', value: '2,136', delta: '较昨日 ↓ 256', status: 'warn' },
      { label: '幂等冲突', value: '47', delta: '较昨日 ↓ 12', status: 'ok' },
      { label: '窗口差异率', value: '0.31%', delta: '较昨日 ↓ 0.08%', status: 'ok' },
      { label: '验收包', value: '8', delta: '较昨日 ↑ 1', status: 'info' },
    ],
    replayTaskRows: [
      ['flow_original', 'flow_original.v1', '06-26 00:00 - 15:00', '8,421', '98.92%', '94', '通过', '详情 / 重放'],
      ['flow_enriched', 'flow_enriched.v1', '06-26 00:00 - 15:00', '2,136', '99.31%', '15', '通过', '详情 / 重放'],
      ['dns_logs', 'dns_logs.v1', '06-26 00:00 - 15:00', '1,128', '97.84%', '24', '警告', '详情 / 重放'],
      ['asset_events', 'asset_events.v1', '06-26 00:00 - 15:00', '642', '99.01%', '6', '通过', '详情 / 重放'],
      ['threat_alerts', 'threat_alerts.v1', '06-26 00:00 - 15:00', '311', '96.43%', '11', '警告', '详情 / 重放'],
      ['pcap_index', 'pcap_index.v1', '06-26 00:00 - 15:00', '207', '99.42%', '3', '通过', '详情 / 重放'],
    ],
    replayReconcileTrend: {
      times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
      sourceTotal: [64, 68, 66, 72, 78, 74, 70, 76, 82, 78, 80, 84, 86, 83, 88, 90],
      sinkTotal: [63, 67, 65, 71, 77, 73, 69, 75, 81, 77, 79, 83, 85, 82, 87, 89],
      diffCount: [18, 16, 19, 22, 26, 21, 18, 20, 24, 22, 19, 21, 23, 18, 20, 17],
      diffRate: [34, 31, 36, 42, 48, 39, 35, 38, 44, 41, 37, 39, 42, 34, 37, 32],
      diffRateThreshold: [58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58],
    },
    replayReconcileSummary: [
      ['源端总数', '3.21B', 'info'],
      ['落库总数', '3.20B', 'ok'],
      ['差异数量', '9.95M', 'warn'],
      ['差异率', '0.31%', 'ok'],
    ],
    replayIdempotencyRows: [
      ['幂等键一致性', 'community_id+five_tuple+ts', '通过', '47', '查看'],
      ['Hash 碰撞检测', 'Murmur3(128)', '通过', '0', '查看'],
      ['重复 session_id', 'session_id', '警告', '1,236', '查看'],
      ['重复 alert_id', 'alert_id', '通过', '342', '查看'],
      ['重放批次重叠', 'batch_id/window', '通过', '0', '查看'],
      ['幂等冲突写入', 'upsert_conflict', '警告', '47', '查看'],
    ],
    replayDifferenceRows: [
      ['06-26 14:00', 'flow_original.v1', 'offset gap', 'f7a3c2e1...', 'offset 15874231', 'offset 15874190', '消息中断与重投', '重放补齐'],
      ['06-26 13:00', 'flow_enriched.v1', 'schema mismatch', 'b1d9a8c2...', 'app_id:102', 'app_id:null', '字段类型变更未兼容', '规则更新'],
      ['06-26 12:00', 'dns_logs.v1', 'late event', 'c45f2a77...', 'ts 12:35:12', 'ts 12:20:05', '迟到数据窗口', '延长窗口'],
      ['06-26 12:00', 'asset_events.v1', 'duplicate key', 'd8a7b3a1...', 'asset_id:9987', 'asset_id:9987', '重复写入', '幂等检查'],
      ['06-26 11:00', 'threat_alerts.v1', 'duplicate key', 'e27b4d91...', 'alert_id:55123', 'alert_id:55123', '主键告警产生', '去重规则'],
      ['06-26 10:00', 'pcap_index.v1', 'sink timeout', 'a9c4e1f2...', '写入 2.1MB/s', '写入超时', 'ClickHouse 超时', '重试写入'],
    ],
    replayFlowNodes: [
      { id: 'dlq', label: 'DLQ / Kafka', detail: '待重放 12,845 / Topic 6 个 / 最早 offset 15:26:11', status: 'warn' },
      { id: 'flink', label: '重放作业（Flink）', detail: 'Job: replay-job / 并行度 8 / RUNNING / Checkpoint 3m ago', status: 'ok' },
      { id: 'idempotent', label: '去重过滤（幂等）', detail: '幂等键 6 规则 / 过滤率 0.72% / 冲突 47', status: 'warn' },
      { id: 'sink', label: '落库目标', detail: 'ClickHouse / OpenSearch / NebulaGraph / MinIO', status: 'ok' },
      { id: 'retry', label: '重试队列', detail: '失败任务 2 / 回退策略 offset batch', status: 'risk' },
      { id: 'checkpoint', label: '校验检查点', detail: '对账窗口 24h / 差异率 0.31%', status: 'ok' },
      { id: 'gate', label: '验收门禁', detail: '验收包 8 / 审计记录完整', status: 'ok' },
    ],
    replayFlowEdges: [
      { from: 'DLQ / Kafka', to: '重放作业（Flink）', label: '数据流', status: 'ok' },
      { from: '重放作业（Flink）', to: '去重过滤（幂等）', label: '校验流', status: 'info' },
      { from: '去重过滤（幂等）', to: '落库目标', label: '数据流', status: 'ok' },
      { from: '重放作业（Flink）', to: '重试队列', label: '异常/重试', status: 'risk' },
      { from: '落库目标', to: '校验检查点', label: '校验流', status: 'info' },
      { from: '校验检查点', to: '验收门禁', label: '控制流', status: 'warn' },
    ],
    replayEvidenceRows: [
      ['对账报告', 'data_analyst', '06-26 15:20', '已归档', '导出 PDF'],
      ['重放日志', 'ops_engineer', '06-26 15:18', '已归档', '导出日志'],
      ['投递快照摘要', 'qa_engineer', '06-26 15:16', '已归档', '导出 JSON'],
      ['差异样本还原', 'sec_analyst', '06-26 15:14', '已归档', '导出样本'],
      ['审计记录', 'sec_manager', '06-26 15:25', '已归档', '导出记录'],
    ],
    replayRailAlerts: [
      ['高', '差异率超阈值窗口', '3', 'risk'],
      ['中', '重放失败任务', '2', 'warn'],
      ['中', '幂等冲突告警', '1', 'warn'],
      ['中', '重复记录激增', '2', 'warn'],
    ],
    replayRailLocateRows: ['定位 DLQ Topic', '定位重放作业', '定位差异窗口', '查看对账详情'],
    replayRailRepairRows: ['重放失败重试', '扩容重放作业', '补齐幂等规则', '优化幂等字段索引', '延长对账时间窗'],
    replayRailEvidenceRows: ['导出对账报告', '生成验收包', '查看验收历史', '审计操作日志'],
    flinkKpis: [
      { label: 'Flink 质量分', value: '91/100', delta: '较昨日 ↑ 2', status: 'ok' },
      { label: '运行作业', value: '9', delta: '较昨日 --', status: 'info' },
      { label: 'Checkpoint 成功率', value: '99.2%', delta: '较昨日 ↑ 0.6%', status: 'ok' },
      { label: 'Watermark 延迟 P95', value: '1.6s', delta: '较昨日 ↓ 0.3s', status: 'ok' },
      { label: 'Backpressure', value: '0.38', delta: '较昨日 ↑ 0.08', status: 'warn' },
      { label: '迟到数据率', value: '0.67%', delta: '较昨日 ↓ 0.12%', status: 'ok' },
      { label: '异常事件', value: '312', delta: '较昨日 ↑ 48', status: 'risk' },
    ],
    flinkJobRows: [
      ['session-job', '运行中', '24', '1.3s / 1.1s', '1.2s', '0.21', '0.32%', '12', '正常'],
      ['feature-job', '运行中', '16', '1.4s / 1.2s', '1.4s', '0.25', '0.41%', '5', '正常'],
      ['rule-job', '运行中', '20', '1.2s / 1.0s', '1.1s', '0.22', '0.38%', '8', '正常'],
      ['pcap-index-job', '重启中', '12', '2.1s / 2.0s', '1.8s', '0.45', '0.71%', '18', '正常'],
      ['behavior-job', '背压中', '32', '1.6s / 1.4s', '1.7s', '0.78', '1.42%', '156', '正常'],
      ['alert-generator-job', '运行中', '8', '1.1s / 0.9s', '0.9s', '0.18', '0.21%', '6', '正常'],
      ['log-job', '运行中', '10', '1.2s / 1.0s', '1.0s', '0.19', '0.29%', '9', '正常'],
      ['user-behavior-job', '运行中', '16', '1.5s / 1.3s', '1.3s', '0.31', '0.56%', '24', '正常'],
    ],
    flinkCheckpointTrend: {
      times: ['15:30', '18:30', '21:30', '00:30', '03:30', '06:30', '09:30', '12:30', '15:30'],
      checkpointDuration: [60, 57, 58, 55, 52, 18, 54, 50, 22, 56, 54, 49, 51, 47, 50, 43],
      checkpointAge: [70, 68, 66, 64, 61, 58, 55, 20, 58, 56, 54, 52, 48, 45, 19, 50],
      watermarkP95: [76, 74, 72, 75, 70, 77, 72, 65, 76, 73, 75, 70, 69, 74, 56, 71],
      watermarkSla: [44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44, 44],
      checkpointSla: [58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58, 58],
    },
    flinkBackpressureBuckets: ['0-7', '8-15', '16-23', '24-31', '32-39', '40-47'],
    flinkBackpressureRows: [
      { label: 'session-job', values: ['ok', 'ok', 'ok', 'info', 'ok', 'info', 'ok', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok'] },
      { label: 'feature-job', values: ['ok', 'ok', 'info', 'ok', 'warn', 'info', 'ok', 'ok', 'warn', 'warn', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'info', 'ok'] },
      { label: 'rule-job', values: ['ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'info', 'ok', 'ok', 'info'] },
      { label: 'pcap-index-job', values: ['info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info', 'info'] },
      { label: 'behavior-job', values: ['warn', 'warn', 'warn', 'risk', 'risk', 'warn', 'warn', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk', 'risk'] },
      { label: 'alert-generator-job', values: ['warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn', 'warn'] },
      { label: 'log-job', values: ['ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info'] },
      { label: 'user-behavior-job', values: ['ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok', 'info', 'ok', 'ok', 'ok'] },
    ],
    flinkLateTopicRows: [
      ['flow_original', '1.23M', '14.6K', '2.1K'],
      ['flow_enriched', '1.02M', '11.3K', '1.4K'],
      ['dns_logs', '362K', '6.2K', '0.8K'],
      ['tls_logs', '286K', '4.1K', '0.5K'],
      ['asset_events', '184K', '2.0K', '0.3K'],
      ['threat_alerts', '96K', '1.6K', '0.2K'],
    ],
    flinkWindowRows: [
      ['1 min', '3.2s', '0.21%'],
      ['5 min', '4.8s', '0.17%'],
      ['10 min', '6.1s', '0.14%'],
      ['30 min', '8.7s', '0.11%'],
      ['60 min', '11.3s', '0.09%'],
    ],
    flinkFailureRows: [
      ['TimeoutException', 'behavior-job', 'map-3b6f', '62', '06-26 11:42', '06-26 15:20', '检查下游处理组'],
      ['BackpressureException', 'behavior-job', 'sink-9a21', '48', '06-26 10:15', '06-26 15:20', '扩容并行度'],
      ['CheckpointException', 'pcap-index-job', 'src-2111', '18', '06-26 08:33', '06-26 15:18', '检查快照存储'],
      ['WatermarkLagAlert', 'user-behavior-job', 'wm-77aa', '15', '06-26 09:41', '06-26 15:05', '优化 watermark 策略'],
      ['OutOfMemoryError', 'log-job', 'proc-1d4c', '9', '06-26 07:58', '06-26 14:55', '调整内存与 TTL'],
      ['SerializationException', 'session-job', 'map-6e12', '7', '06-26 07:12', '06-26 14:31', '修复序列化配置'],
      ['RebalanceInProgress', 'alert-generator-job', 'rebalance', '6', '06-26 06:44', '06-26 13:22', '等待再均衡完成'],
    ],
    flinkSinkRows: [
      { name: 'ClickHouse', status: '正常', eps: '18,734', success: '99.95%', p95: '36 ms', retries: '128', trend: [68, 70, 69, 72, 71, 73, 70, 74, 73, 76, 72, 75, 78, 74, 82, 76] },
      { name: 'OpenSearch', status: '正常', eps: '15,962', success: '99.91%', p95: '52 ms', retries: '215', trend: [70, 68, 72, 69, 71, 73, 72, 76, 75, 74, 77, 73, 79, 76, 80, 78] },
      { name: 'NebulaGraph', status: '正常', eps: '8,521', success: '99.97%', p95: '41 ms', retries: '74', trend: [74, 73, 75, 72, 76, 74, 77, 78, 75, 80, 76, 82, 79, 83, 81, 84] },
      { name: 'MinIO', status: '正常', eps: '6,342', success: '99.98%', p95: '28 ms', retries: '31', trend: [78, 76, 79, 77, 80, 79, 82, 81, 83, 82, 85, 83, 86, 84, 87, 86] },
    ],
    flinkMetrics: [
      { label: '运行作业', value: '9', description: '全部 RUNNING', status: 'ok' },
      { label: 'Checkpoint 成功率', value: '99.2%', description: '最近 15 分钟', status: 'ok' },
      { label: 'Backpressure', value: '0.38', description: '6 个 task 观察', status: 'warn' },
      { label: 'Watermark 延迟 P95', value: '1.6s', description: '阈值 3s', status: 'ok' },
      { label: '迟到数据率', value: '0.67%', description: '较昨日 ↓ 0.09%', status: 'warn' },
      { label: '异常事件', value: '312', description: '近 24 小时', status: 'risk' },
    ],
    flinkTrend: {
      times: ['03:40', '07:40', '11:40', '15:40', '19:40'],
      p50: [68, 64, 66, 60, 62, 56, 58, 53, 55, 50, 52],
      p95: [52, 48, 55, 44, 50, 38, 42, 34, 40, 30, 36],
      threshold: [46, 46, 46, 46, 46, 46, 46, 46, 46, 46, 46],
    },
  };
};

const heartbeatProbeTimeline = (): PageSnapshot['timeline'] => [
  { title: '03:45:00 PROBE-DC-01', description: '心跳同步，延迟 1s', status: 'ok' },
  { title: '03:44:59 PROBE-DC-02', description: '心跳同步，延迟 1s', status: 'ok' },
  { title: '03:44:58 PROBE-BUILD-01', description: '心跳同步，延迟 2s', status: 'ok' },
  { title: '03:44:54 PROBE-SPORT-01', description: '丢包率 1.12%', status: 'warn' },
  { title: '03:44:49 PROBE-DORM-01', description: '超时 3m21s', status: 'risk' },
];

const buildDashboardVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => ({
  id: page.id,
  total: 1286,
  metrics: [
    { label: '超时 SLA', value: '23', delta: '较昨日 +5', status: 'risk' },
    { label: '临近超时数', value: '47', delta: '≤60 分钟 / 较昨日 +8', status: 'warn' },
    { label: '高危未处理', value: '92', delta: '较昨日 +12', status: 'risk' },
    { label: '待取证', value: '156', delta: '较昨日 -3', status: 'info' },
    { label: '待反馈', value: '64', delta: '较昨日 -7', status: 'info' },
    { label: '待复核', value: '38', delta: '较昨日 +2', status: 'warn' },
    { label: '队列积压量', value: '1,286', delta: '较昨日 +96', status: 'info' },
    { label: '今日闭环进度', value: '62%', delta: '目标 80% / 剩余 17h', status: 'ok' },
  ],
  rows: [
    dashboardRow('EVT-20260620-001', '高危', '教学网段', '认证门户', '检测分析', '00:18:32', '缺失'),
    dashboardRow('EVT-20260620-002', '高危', '办公网段', '财务系统', '响应处置', '00:24:55', '不完整'),
    dashboardRow('EVT-20260620-003', '高危', '数据中心区', '文件服务', '响应处置', '00:28:11', '缺失'),
    dashboardRow('EVT-20260620-004', '中危', '实验网段', '教务系统', '检测分析', '00:42:37', '完整'),
    dashboardRow('EVT-20260620-005', '中危', '办公网段', '资产目录', '检测分析', '00:48:06', '不完整'),
    dashboardRow('EVT-20260620-006', '中危', '教学网段', '邮件系统', '响应处置', '00:55:21', '缺失'),
    dashboardRow('EVT-20260620-007', '低危', '数据中心区', '内部网站', '监控观察', '01:12:09', '完整'),
    dashboardRow('EVT-20260620-008', '低危', '实验网段', '代码仓库', '监控观察', '01:35:44', '不完整'),
  ],
  timeline: [
    { title: '今日必处理', description: '92 条高危未处理事件进入优先级工作篮。', status: 'risk' },
    { title: '处理中', description: '156 条事件正在处置，SLA 达成率 82%。', status: 'warn' },
    { title: '待反馈', description: '64 条样本反馈等待回流，较昨日 -7。', status: 'info' },
    { title: '待取证', description: '156 条证据窗口待补齐，需优先拉取 PCAP/Session。', status: 'info' },
    { title: '需审计留痕', description: '23 条工单逾期，需补齐影响范围与审计记录。', status: 'risk' },
  ],
  evidence: [
    { label: '证据完整度缺口', value: '18%', status: 'warn' },
    { label: '反馈覆盖率', value: '64%', status: 'info' },
    { label: '误报回流量', value: '21%', status: 'risk' },
    { label: '样本回流缺口', value: '36%', status: 'warn' },
    { label: '复核完成率', value: '72%', status: 'ok' },
  ],
  visuals: {
    dashboard: {
      kpiSparks: [
        [18, 12, 16, 10, 15, 13, 17, 11, 14, 12, 16, 13, 15, 10, 18, 12, 15, 11, 17, 13, 16, 12, 18, 10, 16, 14],
        [20, 14, 18, 13, 16, 15, 19, 12, 17, 15, 18, 16, 20, 13, 19, 14, 18, 13, 17, 15, 19, 12, 18, 14, 17, 15],
        [17, 11, 15, 12, 14, 13, 18, 10, 16, 12, 15, 11, 17, 13, 16, 10, 18, 12, 14, 11, 17, 13, 15, 12, 16, 14],
        [15, 10, 16, 11, 18, 12, 17, 10, 16, 12, 18, 11, 17, 13, 16, 12, 15, 11, 17, 10, 18, 12, 16, 11, 17, 13],
        [14, 10, 16, 11, 15, 12, 17, 11, 16, 13, 15, 11, 17, 12, 16, 13, 18, 10, 17, 12, 16, 10, 18, 12, 15, 13],
        [15, 11, 17, 12, 16, 13, 18, 10, 17, 12, 15, 11, 18, 12, 16, 10, 17, 13, 16, 11, 18, 12, 17, 10, 16, 13],
        [14, 10, 16, 11, 18, 12, 15, 10, 17, 12, 16, 11, 18, 13, 15, 10, 17, 11, 16, 12, 18, 10, 15, 11, 17, 13],
        [62, 62, 62, 62, 62],
      ],
      healthGates: [
        { component: 'Probe', status: '正常', reason: '-', scope: '-', updated: '03:44:32' },
        { component: 'Kafka', status: '正常', reason: '-', scope: '-', updated: '03:44:18' },
        { component: 'Flink', status: '告警', reason: 'Checkpoint 延迟', scope: '部分任务', updated: '03:43:51' },
        { component: 'ClickHouse', status: '正常', reason: '-', scope: '-', updated: '03:44:27' },
        { component: 'OpenSearch', status: '正常', reason: '-', scope: '-', updated: '03:44:21' },
        { component: 'NebulaGraph', status: '告警', reason: '查询延迟偏高', scope: '部分图空间', updated: '03:43:29' },
        { component: 'MinIO', status: '异常', reason: '对象存储可用区不可用', scope: '部分存储桶', updated: '03:42:16' },
        { component: 'PostgreSQL', status: '正常', reason: '-', scope: '-', updated: '03:44:11' },
      ],
      stages: [
        { label: '今日必处理', value: '92', footnote: 'SLA 达成率 76%', status: 'risk', bars: [18, 24, 31, 42, 27, 21, 34, 46] },
        { label: '处理中', value: '156', footnote: 'SLA 达成率 82%', status: 'warn', bars: [21, 29, 38, 47, 26, 31, 42, 51] },
        { label: '待反馈', value: '64', footnote: 'SLA 达成率 85%', status: 'info', bars: [20, 28, 36, 44, 25, 30, 39, 48] },
        { label: '待取证', value: '156', footnote: 'SLA 达成率 78%', status: 'info', bars: [18, 24, 34, 44, 22, 29, 37, 47] },
        { label: '待复核', value: '38', footnote: 'SLA 达成率 86%', status: 'warn', bars: [20, 27, 35, 43, 24, 31, 40, 49] },
        { label: '需审计留痕', value: '23', footnote: 'SLA 达成率 71%', status: 'risk', bars: [18, 26, 34, 43, 22, 28, 36, 45] },
      ],
      qualityRings: [
        { label: '证据完整度缺口', value: '18%', ringPercent: 18, status: 'warn', subtext: '缺口数 156' },
        { label: '反馈覆盖率', value: '64%', ringPercent: 64, status: 'info', subtext: '已覆盖 64%' },
        { label: '误报回流量', value: '21%', ringPercent: 21, status: 'risk', subtext: '已回流 24%' },
        { label: '样本回流缺口', value: '36%', ringPercent: 36, status: 'warn', subtext: '缺口数 64' },
        { label: '复核完成率', value: '72%', ringPercent: 72, status: 'ok', subtext: '已完成 72%' },
      ],
      topTalkers: [
        { label: '办公网段', value: 32 },
        { label: '教学网段', value: 25 },
        { label: '数据中心区', value: 19 },
        { label: '实验网段', value: 13 },
        { label: '访客网段', value: 6 },
        { label: '其他', value: 5 },
      ],
    },
  },
});

const buildTopicExfilVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  const base = buildPageSnapshot(page);
  return {
    ...base,
    metrics: [
      { label: '外传预警量', value: '64', delta: '较昨日 +8', status: 'risk' },
      { label: '外传路径数', value: '112', delta: '较昨日 +15', status: 'warn' },
      { label: '可疑外传源', value: '23', delta: '较昨日 +3', status: 'risk' },
      { label: '外传目的地数', value: '87', delta: '较昨日 +11', status: 'info' },
      { label: '敏感数据类型数', value: '12', delta: '较昨日 +2', status: 'warn' },
      { label: '异常上传峰值', value: '38.6 Gbps', delta: '较昨日 +27%', status: 'warn' },
      { label: '跨境目的地数', value: '32', delta: '较昨日 +6', status: 'warn' },
      { label: '证据完整度', value: '62%', delta: '较昨日 +6%', status: 'ok' },
    ],
    evidence: [
      { label: '告警证据', value: '64 / 64 (100%)', status: 'ok' },
      { label: 'PCAP', value: '132 / 156 (84%)', status: 'warn' },
      { label: 'Session', value: '198 / 204 (97%)', status: 'ok' },
      { label: '审计日志', value: '38 / 38 (100%)', status: 'ok' },
      { label: '回溯路径', value: '18 / 18 (100%)', status: 'ok' },
      { label: '资产快照', value: '23 / 23 (100%)', status: 'ok' },
    ],
  };
};

const buildTopicAptVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => {
  const base = buildPageSnapshot(page);
  return {
    ...base,
    metrics: [
      { label: '关联战役数', value: '7', delta: '较昨日 +1', status: 'risk' },
      { label: '战役集密度', value: '0.72', delta: '较昨日 +0.08', status: 'ok' },
      { label: '攻击阶段覆盖', value: '5/7', delta: '较昨日 +1', status: 'info' },
      { label: '关键资产命中', value: '46', delta: '较昨日 +6', status: 'risk' },
      { label: '横向移动链路', value: '23', delta: '较昨日 +4', status: 'warn' },
      { label: '持久化迹象数', value: '18', delta: '较昨日 +2', status: 'warn' },
      { label: '外传关联证据', value: '32', delta: '较昨日 +5', status: 'info' },
      { label: '处置闭环率', value: '68%', delta: '较昨日 +7%', status: 'warn' },
      { label: '报告置信度', value: '62%', delta: '较昨日 +8%', status: 'ok' },
    ],
    evidence: [
      { label: 'APT 专题接口', value: '92.6%', status: 'ok' },
      { label: '战役聚类', value: '19 项', status: 'ok' },
      { label: '阶段分布', value: '94.6%', status: 'ok' },
      { label: '实体图谱', value: '33 项', status: 'warn' },
      { label: '证据包', value: '96.6%', status: 'ok' },
      { label: '审计记录', value: '47 项', status: 'ok' },
    ],
  };
};

const buildAlertsVisualBreakdownSnapshot = (page: PageSpec): PageSnapshot => ({
  id: page.id,
  total: 425,
  metrics: [
    { label: '高危', value: '38', delta: '+12', status: 'risk' },
    { label: '中危', value: '57', delta: '+8', status: 'warn' },
    { label: '低危', value: '33', delta: '-5', status: 'info' },
    { label: '未处理', value: '86', delta: '待研判', status: 'risk' },
    { label: '处理中', value: '19', delta: '研判中', status: 'info' },
    { label: '已确认', value: '156', delta: '+20', status: 'ok' },
    { label: '已忽略', value: '42', delta: '-3', status: 'info' },
  ],
  rows: [
    alertVisualRow('AL-20260620-000123', '高危', '疑似 C2 隧道通信', '命令与控制', '172.16.5.10:44321', '185.22.14.9:443', '办公区-WS-1024', 'C2_Tunnel_v3', '0.98', '06-20 03:42:11', '未处理', 'new', 1001, 92),
    alertVisualRow('AL-20260620-000122', '高危', '横向移动-凭证滥用', '横向移动', '10.12.8.45:49872', '10.12.9.33:445', '教学区-SRV-2003', 'Lateral_Move_v2', '0.96', '06-20 03:41:08', '处理中', 'triage', 1002, 89),
    alertVisualRow('AL-20260620-000121', '中危', '数据外传-大量外发', '数据泄露', '192.168.3.55:52444', '203.0.113.45:443', '宿舍区-PC-3056', 'Data_Exfil_v1', '0.89', '06-20 03:39:47', '未处理', 'new', 1003, 78),
    alertVisualRow('AL-20260620-000119', '高危', '疑似 APT-工具投递', '执行', '172.16.1.77:51322', '198.51.100.27:80', '核心区-DC-01', 'APT_ToolDrop_v1', '0.94', '06-20 03:39:47', '未处理', 'new', 1004, 91),
    alertVisualRow('AL-20260620-000118', '中危', '异常 DNS 隧道', '命令与控制', '10.12.2.36:53513', '8.8.8.8:53', '办公区-WS-2011', 'DNS_Tunnel_v2', '0.72', '06-20 03:39:11', '处理中', 'triage', 1005, 74),
    alertVisualRow('AL-20260620-000117', '中危', '异常登录-暴力破解', '侦察', '192.168.1.210:52111', '172.16.3.0:24', '公网接入区-GW-01', 'Port_Scan_v1', '0.65', '06-20 03:36:53', '已确认', 'closed', 1006, 67),
    alertVisualRow('AL-20260620-000116', '低危', '协议异常-HTTP 错误', '执行', '172.16.6.15:50233', '10.12.5.20:8080', '办公区-LNX-07', 'HTTP_Anomaly_v1', '0.48', '06-20 03:36:41', '已忽略', 'closed', 1007, 48),
  ],
  timeline: [
    { title: '首次发生', description: '检测到异常外连，建立 TLS 会话', status: 'info' },
    { title: '异常行为', description: '心跳包特征匹配 C2 通信模式', status: 'warn' },
    { title: '横向移动', description: '内网主机发起 SMB 探测', status: 'warn' },
    { title: '证据生成', description: '已生成 PCAP 证据 (3.2 MB)', status: 'ok' },
    { title: '处置动作', description: '已隔离主机（自动化处置）', status: 'ok' },
  ],
  evidence: [
    { label: 'Alerts API', value: '/v1/alerts visual fallback', status: 'ok' },
    { label: '记录返回', value: '7/425', status: 'ok' },
    { label: '高危队列', value: '38 条', status: 'risk' },
    { label: '刷新节奏', value: '15s', status: 'info' },
  ],
});

const alertVisualRow = (
  id: string,
  severity: string,
  name: string,
  phase: string,
  source: string,
  destination: string,
  asset: string,
  model: string,
  confidence: string,
  firstSeen: string,
  status: string,
  statusCode: string,
  stateVersion: number,
  risk: number,
): SnapshotRow => ({
  '告警 ID': id,
  风险等级: severity,
  告警名称: name,
  攻击阶段: phase,
  '源 IP': source,
  '目的 IP': destination,
  受影响资产: asset,
  '规则/模型': model,
  置信度: confidence,
  首次发生: firstSeen,
  状态: status,
  操作: '查看',
  __alertId: id,
  __stateVersion: stateVersion,
  __status: statusCode,
  __riskScore: risk,
});

const dashboardRow = (
  id: string,
  risk: string,
  assetGroup: string,
  system: string,
  stage: string,
  remaining: string,
  evidenceState: string,
): SnapshotRow => ({
  '事件 ID': id,
  风险级别: risk,
  资产组: assetGroup,
  业务系统: system,
  处置阶段: stage,
  剩余时间: remaining,
  证据状态: evidenceState,
});

const metricValue = (pageId: string, label: string, index: number) => {
  if (pageId === 'whitelist') {
    if (label === '生效白名单') return '482';
    if (label === '待审批') return '38';
    if (label === '即将到期') return '27';
    if (label === '长期生效') return '153';
    if (label === '覆盖告警') return '3.2K';
    if (label === '潜在漏报风险') return '7/19';
  }
  if (pageId === 'compliance') {
    if (label === '门禁通过率') return '84.6%';
    if (label === '未达标项') return '12';
    if (label === '证据完整度') return '92.3%';
    if (label === '复验通过率') return '78.9%';
    if (label === '第三方批次') return '5';
    if (label === '报告生成数') return '23';
  }
  if (pageId === 'audit-log') {
    if (label === '今日操作') return '12,842';
    if (label === '失败操作') return '132';
    if (label === '高风险操作') return '56';
    if (label === '导出下载') return '218';
    if (label === 'PCAP 访问') return '124';
    if (label === '完整性校验通过率') return '99.67%';
  }
  if (pageId === 'notifications') {
    if (label === '启用渠道') return '6';
    if (label === '订阅规则') return '28';
    if (label === '待确认通知') return '82';
    if (label === '失败通知') return '21';
    if (label === '升级策略') return '5';
    if (label === '静默窗口') return '4';
  }
  if (pageId === 'settings') {
    if (label === '租户数') return '12';
    if (label === '角色策略') return '28';
    if (label === '有效令牌') return '46';
    if (label === '即将过期令牌') return '8';
    if (label === '集成健康') return '7/7';
    if (label === '配置变更待审计') return '3';
  }
  if (pageId === 'topic-tunnel') {
    if (label === '活跃隧道会话') return '8.4K';
    if (label === '隧道协议') return '6';
    if (label === '高危用户') return '18';
    if (label === '总流量') return '2.7 TB';
    if (label === '异常长连接') return '412';
    if (label === '证据完整度') return '92.4%';
  }
  if (pageId === 'topic-exfil') {
    if (label === '外传预警量') return '64';
    if (label === '外传路径数') return '112';
    if (label === '可疑外传源') return '23';
    if (label === '外传目的地数') return '87';
    if (label === '敏感数据类型数') return '12';
    if (label === '异常上传峰值') return '38.6 Gbps';
    if (label === '跨境目的地数') return '32';
    if (label === '证据完整度') return '62%';
  }
  if (pageId === 'topic-apt') {
    if (label === '关联战役数') return '7';
    if (label === '战役集密度') return '0.72';
    if (label === '攻击阶段覆盖') return '5/7';
    if (label === '关键资产命中') return '46';
    if (label === '横向移动链路') return '23';
    if (label === '持久化迹象数') return '18';
    if (label === '外传关联证据') return '32';
    if (label === '处置闭环率') return '68%';
    if (label === '报告置信度') return '62%';
  }
  if (label === '质量总分') return '92';
  if (label === '完整性') return '96.3%';
  if (label === '及时性') return '91.7%';
  if (label === '准确性') return '93.8%';
  if (label === '重复率') return '0.42%';
  if (label === '字段缺失率') return '1.12%';
  if (label === 'DLQ 数量') return '12.8K';
  if (label === '规则草稿') return '46';
  if (label === '待审核规则') return '18';
  if (label === '灰度规则') return '27';
  if (label === '启用规则') return '312';
  if (label === '回滚候选') return '7';
  if (label === '高耗时规则') return '9';
  if (label === '待发布对象') return '18';
  if (label === '灰度中') return '7';
  if (label === '失败/阻断') return '2';
  if (label === '可回滚版本') return '23';
  if (label === '发布成功率') return '98.2%';
  if (label === '平均生效延迟') return '58s';
  if (label === '线上模型数') return '18';
  if (label === '候选模型数') return '7';
  if (label === '漂移告警') return '3';
  if (label === '待重训模型') return '5';
  if (label === '平均 F1') return '0.947';
  if (label === '误报率变化') return '-6.2%';
  if (label === '训练任务') return '32';
  if (label === '评估任务') return '18';
  if (label === '注册任务') return '9';
  if (label === '发布任务') return '7';
  if (label === '失败任务') return '5';
  if (label === '门禁通过率') return '86.7%';
  if (label === '启用剧本') return '18';
  if (label === '待审批') return '5';
  if (label === '今日执行') return '23';
  if (label === '失败步骤') return '7';
  if (label === '高危待确认') return '3';
  if (label === '平均处理耗时') return '6分24秒';
  if (label.includes('率') || label.includes('健康') || label.includes('完整') || label.includes('通过')) {
    return `${96 - index * 3}.${index}%`;
  }
  if (label.includes('带宽')) return `${78 - index * 2}.3 Gbps`;
  if (label.includes('延迟')) return `${1 + index * 0.2}s`;
  if (label.includes('模型') || label.includes('规则') || label.includes('任务')) return `${12 + index * 6}`;
  return `${38 + index * 17}`;
};

const cellValue = (pageId: string, column: string, row: number, columnIndex: number) => {
  if (pageId === 'alerts') return alertCellValue(column, row);
  if (pageId === 'probes') return probeCellValue(column, row);
  if (pageId === 'data-quality') return dataQualityCellValue(column, row);
  if (pageId === 'assets') return assetCellValue(column, row);
  if (pageId === 'graph') return graphCellValue(column, row);
  if (pageId === 'fusion') return fusionCellValue(column, row);
  if (pageId === 'baselines') return baselineCellValue(column, row);
  if (pageId === 'campaigns') return campaignCellValue(column, row);
  if (pageId === 'attack-chains') return attackChainCellValue(column, row);
  if (pageId === 'topic-tunnel') return topicTunnelCellValue(column, row);
  if (pageId === 'topic-exfil') return topicExfilCellValue(column, row);
  if (pageId === 'topic-apt') return topicAptCellValue(column, row);
  if (pageId === 'encrypted-traffic') return encryptedTrafficCellValue(column, row);
  if (pageId === 'forensics') return forensicCellValue(column, row);
  if (pageId === 'rules') return ruleCellValue(column, row);
  if (pageId === 'deployments') return deploymentCellValue(column, row);
  if (pageId === 'models') return modelCellValue(column, row);
  if (pageId === 'mlops') return mlopsCellValue(column, row);
  if (pageId === 'playbooks') return playbookCellValue(column, row);
  if (pageId === 'whitelist') return whitelistCellValue(column, row);
  if (pageId === 'compliance') return complianceCellValue(column, row);
  if (pageId === 'audit-log') return auditLogCellValue(column, row);
  if (pageId === 'notifications') return notificationCellValue(column, row);
  if (pageId === 'settings') return settingsCellValue(column, row);
  if (column.includes('ID') || column.includes('批次')) return `${pageId.toUpperCase()}-${String(row + 1).padStart(4, '0')}`;
  if (column.includes('风险') || column.includes('级别')) return ['高危', '中危', '低危', '高危'][row % 4];
  if (column.includes('状态') || column.includes('结果') || column.includes('健康')) return ['未处理', '处理中', '已确认', '正常'][row % 4];
  if (column.includes('时间') || column.includes('更新')) return `2026-06-25 09:${String(10 + row).padStart(2, '0')}:32`;
  if (column.includes('IP')) return `10.12.${row + 1}.${45 + columnIndex}`;
  if (column.includes('Hash')) return `sha256:${pageId.slice(0, 4)}${row}c9e`;
  if (column.includes('大小')) return `${(row + 1) * 2.4} MB`;
  if (column.includes('证据')) return ['完整', '缺 PCAP', '缺日志', '已归档'][row % 4];
  if (column.includes('进度')) return `${42 + row * 7}%`;
  return `${column}-${row + 1}`;
};

const alertCellValue = (column: string, row: number) => {
  const names = ['疑似 C2 隧道通信', '横向移动 SMB 探测', '数据外传大流量异常', 'APT 工具投递', '异常 DNS 隧道', '端口扫描'];
  const phases = ['命令与控制', '横向移动', '数据泄露', '执行', '命令与控制', '侦察'];
  const statuses = ['未处理', '处理中', '已确认', '已忽略'];
  if (column === '告警 ID') return `AL-${String(202606250001 + row)}`;
  if (column === '风险等级') return ['高危', '高危', '中危', '高危', '中危', '低危'][row % 6];
  if (column === '告警名称') return names[row % names.length];
  if (column === '攻击阶段') return phases[row % phases.length];
  if (column === '源 IP') return `10.12.${row + 1}.${20 + row}`;
  if (column === '目的 IP') return ['185.22.14.9', '10.12.9.33', '203.0.113.45', '198.51.100.27'][row % 4];
  if (column === '受影响资产') return ['办公区-WS-1024', '教学区-SRV-2003', '宿舍区-PC-3056', '核心区-DC-01'][row % 4];
  if (column === '规则/模型') return ['C2_Tunnel_v3', 'Lateral_Move_v2', 'Data_Exfil_v1', 'APT_ToolDrop_v1'][row % 4];
  if (column === '置信度') return (0.98 - row * 0.05).toFixed(2);
  if (column === '首次发生') return `2026-06-25 09:${String(42 - row).padStart(2, '0')}:11`;
  if (column === '状态') return statuses[row % statuses.length];
  return `${column}-${row + 1}`;
};

const probeCellValue = (column: string, row: number) => {
  const ids = ['PROBE-DC-01', 'PROBE-DC-02', 'PROBE-BUILD-01', 'PROBE-BUILD-02', 'PROBE-OFFICE-01', 'PROBE-SPORT-01', 'PROBE-DORM-01'];
  const locations = ['数据中心机房 A', '数据中心机房 B', '教学区 1 栋', '教学区 2 栋', '办公区 A 栋', '体育馆', '宿舍区 1 栋'];
  const statuses = ['在线', '在线', '在线', '在线', '在线', '告警', '离线'];
  const modes = ['混合 (L2+L3)', 'L2 全量', '混合 (L2+L3)', 'L2 全量', '混合 (L2+L3)', 'L2 全量', '-'];
  const bandwidth = ['18.6 Gbps', '9.8 Gbps', '6.7 Gbps', '5.1 Gbps', '4.2 Gbps', '3.8 Gbps', '-'];
  const drops = ['0.02%', '0.01%', '0.03%', '0.00%', '0.04%', '1.12%', '-'];
  const parseRates = ['99.21%', '99.34%', '98.92%', '99.41%', '98.71%', '92.15%', '-'];
  if (column === '探针 ID') return ids[row % ids.length];
  if (column === '位置') return locations[row % locations.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '采集模式') return modes[row % modes.length];
  if (column === '采集带宽') return bandwidth[row % bandwidth.length];
  if (column === '丢包率') return drops[row % drops.length];
  if (column === '解析率') return parseRates[row % parseRates.length];
  if (column === 'CPU') return row === 5 ? '72.6%' : row === 6 ? '-' : `${(25.9 + row * 1.8).toFixed(1)}%`;
  if (column === '内存') return row === 5 ? '68.7%' : row === 6 ? '-' : `${(36.3 + row * 1.6).toFixed(1)}%`;
  if (column === '运行时长') return ['12d 14h', '9d 22h', '11d 3h', '8d 18h', '7d 12h', '5d 6h', '-'][row % 7];
  if (column === '版本') return ['v3.4.7', 'v3.4.7', 'v3.4.6', 'v3.4.6', 'v3.4.7', 'v3.4.5', '-'][row % 7];
  if (column === '操作') return '详情';
  return `${column}-${row + 1}`;
};

const dataQualityCellValue = (column: string, row: number) => {
  const topics = ['flow_original', 'flow_enriched', 'session.events.v1', 'feature.events.v1', 'alerts.v1', 'pcap.index.v1', 'dlq.v1', 'asset.bindings.v1'];
  const partitions = [36, 24, 18, 18, 12, 12, 6, 8];
  const throughput = ['4.2K msg/min', '3.4K msg/min', '2.1K msg/min', '2.0K msg/min', '520 msg/min', '336 msg/min', '128 msg/min', '210 msg/min'];
  const lag = ['10.9K', '7.0K', '3.7K', '2.8K', '1.9K', '1.1K', '12.8K', '654'];
  const trends = ['波动', '下降', '下降', '下降', '波动', '下降', '上升', '下降'];
  if (column === 'Topic') return topics[row % topics.length];
  if (column === '分区数') return partitions[row % partitions.length];
  if (column === '当前吞吐量') return throughput[row % throughput.length];
  if (column === '消费延迟') return ['2s', '1s', '1s', '1s', '1s', '1s', '3s', '1s'][row % 8];
  if (column === '积压量') return lag[row % lag.length];
  if (column === '积压趋势') return trends[row % trends.length];
  if (column === '消费延迟 P95') return ['1600 ms', '1344 ms', '1152 ms', '1248 ms', '1472 ms', '1024 ms', '1920 ms', '928 ms'][row % 8];
  if (column === '分区倾斜') return `${(1.08 + row * 0.07).toFixed(2)}x`;
  if (column === '消息延迟 P95') return ['1152 ms', '968 ms', '829 ms', '899 ms', '1060 ms', '737 ms', '1382 ms', '668 ms'][row % 8];
  if (column === '操作') return row === 6 ? '重放 DLQ' : row % 3 === 0 ? '定位 Flink' : '查看';
  return `${column}-${row + 1}`;
};

const assetCellValue = (column: string, row: number) => {
  const hostnames = ['实验楼-PC-0082', 'SRV-12', 'SW-07', 'NAS-03', 'AP-15', 'FIN-PC-0082', 'DB-SRV-07', 'CAM-022'];
  const types = ['终端', '服务器', '网络设备', '服务器', '网络设备', '终端', '服务器', '终端'];
  const departments = ['实验楼 / 计算中心', '实验楼 / 计算中心', '核心区', '图书馆', '办公区', '财务部', '计算中心 / 数据库组', '安防系统'];
  const systems = ['Windows 11', 'Ubuntu 22.04', 'Huawei VRP', 'Synology DSM 7', 'ArubaOS 8.7', 'Windows 11', 'CentOS 7.9', 'Linux 4.x'];
  const risks = ['弱口令', '漏洞 / 暴露服务', '高危端口', '暴露服务', '弱口令', '未打补丁', '漏洞 / 高危端口', '默认凭据'];
  if (column === '资产 ID') return row === 0 ? 'PC-0082' : `ASSET-${String(420 + row).padStart(4, '0')}`;
  if (column === 'IP/MAC') return row === 0 ? '10.12.8.82 / 00:50:56:AA:08:82' : `10.12.${row + 3}.${45 + row} / 00:50:56:AA:${String(12 + row).padStart(2, '0')}:34`;
  if (column === '主机名') return hostnames[row % hostnames.length];
  if (column === '类型') return types[row % types.length];
  if (column === '园区/部门') return departments[row % departments.length];
  if (column === '操作系统') return systems[row % systems.length];
  if (column === '重要性') return ['中', '高', '高', '中', '低', '中', '高', '低'][row % 8];
  if (column === '最近活跃') return `2026-06-25 09:${String(38 + row).padStart(2, '0')}:21`;
  if (column === '暴露端口') return [3, 8, 2, 5, 1, 2, 6, 3][row % 8];
  if (column === '风险标签') return risks[row % risks.length];
  return `${column}-${row + 1}`;
};

const graphCellValue = (column: string, row: number) => {
  const sources = ['185.234.15.23', '10.20.0.1', 'biz_admin', 'erp.corp.edu.cn', 'WEB-SRV-02', '10.20.4.18'];
  const targets = ['边界网关 10.20.0.1', '核心业务服务器 10.20.4.18', '核心业务服务器 10.20.4.18', '核心业务服务器 10.20.4.18', 'DB-SRV-01 10.20.4.20', 'ALERT-20260620-1287'];
  const risks = ['高危', '中危', '中危', '低危', '中危', '高危'];
  const evidence = ['PCAP-20260620-0156', 'SESSION-20260620-3381', 'AUDIT-GRAPH-0042', 'DNS-QUERY-7712', 'FLOW-EDGE-4420', 'ALERT-20260620-1287'];
  if (column === '路径 ID') return `GRAPH-PATH-${String(row + 1).padStart(3, '0')}`;
  if (column === '源实体') return sources[row % sources.length];
  if (column === '目标实体') return targets[row % targets.length];
  if (column === '跳数') return [3, 2, 1, 1, 2, 1][row % 6];
  if (column === '风险') return risks[row % risks.length];
  if (column === '证据') return evidence[row % evidence.length];
  return `${column}-${row + 1}`;
};

const fusionCellValue = (column: string, row: number) => {
  const objects = ['IP_MAC_BIND_V3', 'ACCOUNT_HOST_LINK', 'ASSET_DEPT_COMPLETION', 'DOMAIN_IP_RESOLVE', 'ALERT_ASSET_JOIN', 'CVE_SERVICE_MATCH'];
  const sourcesA = ['Flow 流量', 'AD 登录', 'CMDB 资产', 'DNS 解析', 'SIEM 告警', 'Vuln 漏洞'];
  const sourcesB = ['DHCP / ARP', 'EDR 终端', 'HR 部门', 'Passive DNS', 'Asset 资产', 'Service 服务'];
  const fields = ['IP-MAC', '账号-主机', '资产-部门', '域名-IP', '告警-资产', '漏洞-服务'];
  if (column === '对象') return objects[row % objects.length];
  if (column === '来源 A') return sourcesA[row % sourcesA.length];
  if (column === '来源 B') return sourcesB[row % sourcesB.length];
  if (column === '冲突字段') return fields[row % fields.length];
  if (column === '可信度') return ['0.86', '0.75', '0.70', '0.85', '0.65', '0.60'][row % 6];
  if (column === '处理状态') return ['待确认', '已对齐', '已对齐', '待复核', '待确认', '已对齐'][row % 6];
  return `${column}-${row + 1}`;
};

const baselineCellValue = (column: string, row: number) => {
  const objects = ['实验楼-SRV-12', '图书馆-NAS-03', '教学区-PC-0421', '汇聚交换机-07', '办公区-WS-1024', '核心区-DC-01'];
  const types = ['资产基线', '端口基线', '流量基线', '协议基线', '账号基线', '时间段基线'];
  const deviations = ['7.5x', '3.2x', '6.8x', '2.4x', '1.2x', '4.1x'];
  const evidence = ['Flow / DNS / TLS / PCAP', 'Session / Flow', 'Flow / PCAP', '协议分布 / Session', 'AD / User Event', 'Flow / Audit'];
  const explanations = ['新的目的地偏离', '新端口扫描偏离', '出站流量偏离', '协议分布偏离', '夜间访问偏离', '会话长度偏离'];
  const statuses = ['待解释', '已解释', '待解释', '已解释', '观察中', '待重建'];
  if (column === '对象') return objects[row % objects.length];
  if (column === '基线类型') return types[row % types.length];
  if (column === '偏离值') return deviations[row % deviations.length];
  if (column === '证据') return evidence[row % evidence.length];
  if (column === '解释') return explanations[row % explanations.length];
  if (column === '状态') return statuses[row % statuses.length];
  return `${column}-${row + 1}`;
};

const campaignCellValue = (column: string, row: number) => {
  const names = [
    'APT-20260619-RedLync',
    'DataExfil-20260618-Office',
    'Ransom-20260617-LocalShare',
    'Recon-20260616-ScanWave',
    'Lateral-20260615-PSEXEC',
    'BruteForce-20260614-SSH',
    'DNS-Tunnel-20260614-Iodine',
    'MalDoc-20260613-Macro',
  ];
  const phases = ['横向移动', '数据外传', '执行活动', '信息收集', '横向移动', '初始访问', '外联通信', '执行活动'];
  const risks = ['高风险', '高风险', '高风险', '中风险', '中风险', '中风险', '低风险', '低风险'];
  const statuses = ['活跃中', '活跃中', '活跃中', '观察中', '观察中', '已结束', '已结束', '已结束'];
  const assets = [42, 31, 18, 56, 27, 12, 8, 16];
  const alerts = [234, 187, 96, 145, 102, 78, 34, 56];
  const firstSeen = ['06-19 09:12:45', '06-18 14:32:11', '06-17 21:18:04', '06-16 11:07:52', '06-15 19:43:18', '06-14 22:14:36', '06-14 15:36:21', '06-13 10:11:09'];
  const lastSeen = ['06-20 03:22:11', '06-20 01:45:33', '06-19 23:12:17', '06-18 17:33:21', '06-18 10:22:05', '06-17 08:11:54', '06-16 20:43:33', '06-15 16:22:40'];
  if (column === '战役名称') return names[row % names.length];
  if (column === '阶段') return phases[row % phases.length];
  if (column === '风险等级') return risks[row % risks.length];
  if (column === '影响资产') return assets[row % assets.length];
  if (column === '告警数') return alerts[row % alerts.length];
  if (column === '首次发现') return firstSeen[row % firstSeen.length];
  if (column === '最近活动') return lastSeen[row % lastSeen.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '操作') return '查看';
  return `${column}-${row + 1}`;
};

const attackChainCellValue = (column: string, row: number) => {
  const phases = ['侦察', '初始访问', '执行', '横向移动', 'C2 通信', '数据外传'];
  const entities = ['203.0.113.45', '边界防火墙 FW-01', 'WEB 服务器 10.12.5.23', '域控服务器 10.12.1.10', '内网主机 10.12.8.45', 'c2.example.com'];
  const alerts = ['端口扫描探测', 'Web 漏洞利用', '恶意命令执行', '凭证窃取', 'C2 隧道通信', '数据外传尝试'];
  const evidence = ['DNS 解析记录', 'HTTP 请求包', '进程创建日志', 'LSASS 访问', 'TLS 流量会话', '外传流量样本'];
  const actions = ['封禁源 IP', 'WAF 规则加固', '终止恶意进程', '重置域控凭证', '阻断 C2 域名', '阻断外传通道'];
  if (column === '阶段') return phases[row % phases.length];
  if (column === '实体') return entities[row % entities.length];
  if (column === '告警') return alerts[row % alerts.length];
  if (column === '证据') return evidence[row % evidence.length];
  if (column === '处置建议') return actions[row % actions.length];
  if (column === '状态') return row % 4 === 0 ? '待确认' : '已确认';
  return `${column}-${row + 1}`;
};

const topicTunnelCellValue = (column: string, row: number) => {
  const sessions = [
    '10.12.2.36 -> cloudflare-dns.com',
    '10.10.8.45 -> 203.0.113.45',
    '172.16.5.10 -> api.update.server',
    '10.12.9.33 -> 198.51.100.27',
    '10.11.3.22 -> 37.120.196.12',
    '10.12.6.77 -> 2606:4700::6810',
  ];
  const protocols = ['DoH/TLS', 'TLS 隧道', 'QUIC', 'VPN over 443', '未知加密', 'TLS 1.3'];
  const features = ['未知 SNI', '长连接 > 1h', '高熵流量', '固定心跳', '低频黑噪', 'JA3 异常'];
  if (column === '会话摘要') return sessions[row % sessions.length];
  if (column === '协议族') return protocols[row % protocols.length];
  if (column === '源资产') return ['办公区-WS-1024', '财务-SRV-2003', '核心区-DC-01', '宿舍区-PC-3056'][row % 4];
  if (column === '目标对象') return ['cloudflare-dns.com', '203.0.113.45', 'api.update.server', 'cdn-sync.example'][row % 4];
  if (column === '指纹/特征') return features[row % features.length];
  if (column === '持续时间') return ['3h 24m', '6h 12m', '2h 53m', '1h 41m', '4h 30m', '5h 18m'][row % 6];
  if (column === '流量') return ['428 GB', '312 GB', '221 GB', '164 GB', '92 GB', '76 GB'][row % 6];
  if (column === '风险等级') return ['高危', '高危', '中危', '中危', '低危', '中危'][row % 6];
  if (column === '处置') return '取证';
  return `${column}-${row + 1}`;
};

const topicExfilCellValue = (column: string, row: number) => {
  const sources = ['财务-SRV-2003', '科研-NAS-07', '办公区-WS-1024', '教学区-PC-0402', '核心区-DB-01', '宿舍区-PC-3056'];
  const paths = [
    '10.12.8.45 -> object-store.example',
    '10.12.4.18 -> 198.51.100.27',
    '10.10.9.33 -> 203.0.113.45',
    '10.11.3.22 -> cloud-drive.example',
    '10.12.2.36 -> backup-cloud.example',
    '172.16.5.10 -> 185.22.14.9',
  ];
  const riskTypes = ['异常上传', '跨境外联', '云存储', '未知 ASN', '敏感库访问', '白名单复核'];
  if (column === '源资产') return sources[row % sources.length];
  if (column === '外传路径') return paths[row % paths.length];
  if (column === '目标区域') return ['境外云服务', '对象存储', '未知 ASN', '跨境 CDN'][row % 4];
  if (column === '数据类型') return ['数据库备份', '压缩包', '源代码', '文档集合'][row % 4];
  if (column === '上传量') return ['486 GB', '312 GB', '218 GB', '176 GB', '98 GB', '72 GB'][row % 6];
  if (column === '会话数') return [326, 211, 184, 142, 98, 76][row % 6];
  if (column === '风险类型') return riskTypes[row % riskTypes.length];
  if (column === '风险等级') return ['高危', '高危', '中危', '中危', '中危', '低危'][row % 6];
  if (column === '处置') return '阻断';
  return `${column}-${row + 1}`;
};

const topicAptCellValue = (column: string, row: number) => {
  const campaigns = ['APT-20260619-RedLync', 'APT-20260618-NightOwl', 'APT-20260617-BlueLance', 'APT-20260616-ShadowLab', 'APT-20260615-EastGate', 'APT-20260614-Dropper'];
  const phases = ['初始访问', '执行活动', '横向移动', 'C2 通信', '数据外传', '影响达成'];
  const entities = ['WEB-SRV-02', '域控 DC-01', '财务-SRV-2003', '10.12.8.45', 'c2.example.net', '科研-NAS-07'];
  if (column === '战役名称') return campaigns[row % campaigns.length];
  if (column === '阶段') return phases[row % phases.length];
  if (column === '关键实体') return entities[row % entities.length];
  if (column === '关联告警') return [234, 187, 156, 112, 96, 78][row % 6];
  if (column === '攻击技术') return ['T1190', 'T1059', 'T1021', 'T1071', 'T1041', 'T1486'][row % 6];
  if (column === '首次发现') return `06-${String(19 - (row % 6)).padStart(2, '0')} 09:12`;
  if (column === '最近活动') return `06-${String(20 - (row % 5)).padStart(2, '0')} 03:22`;
  if (column === '风险等级') return ['高风险', '高风险', '中风险', '高风险', '中风险', '低风险'][row % 6];
  if (column === '处置') return '复盘';
  return `${column}-${row + 1}`;
};

const encryptedTrafficCellValue = (column: string, row: number) => {
  const protocols = ['TLS', 'QUIC', 'TLS', 'TLS', '未知加密', 'TLS', 'QUIC', 'TLS'];
  const sessions = [
    '10.12.2.36:56321 -> 104.12.12.34:443',
    '10.10.8.45:61234 -> 203.0.113.45:443',
    '172.16.5.10:55211 -> 185.22.14.9:443',
    '10.12.9.33:46822 -> 198.51.100.27:443',
    '10.11.3.22:53242 -> 37.120.196.12:443',
    '10.12.6.77:51021 -> 2606:4700::6810:84e5:443',
  ];
  const snis = ['cdn.example.com', '-', 'api.update.server', 'sync.example.net', '-', 'cloudflare-dns.com'];
  const ja3 = ['771,4865-4866...', 'cbd52c1eb670...', 'e7d70S342S8a...', '4d7a28f00056...', '598c8ab3943e...', 'a1b2c3d4e5f6...'];
  const ja3s = ['8f9e3d7a1c2b...', 'a1b2c3d4e5f6...', 'd4d3f2b1a7c6...', 'c1d2e3f4e5b6...', '0f1e2d3c4b5a...', '90ab12cd34ef...'];
  const issuers = ['Cloudflare Inc ECC CA-3', 'Amazon RSA 2048 M01', "Let's Encrypt R3", 'DigiCert TLS RSA SHA256 2020 CA1', 'Sectigo RSA Domain Validation Secure', '未知'];
  const risks = ['中危', '高危', '中危', '中危', '低危', '高危'];
  if (column === '时间') return `06-20 03:${String(46 - row).padStart(2, '0')}:59`;
  if (column === '协议') return protocols[row % protocols.length];
  if (column === 'Session 摘要') return sessions[row % sessions.length];
  if (column === '证书详情') return row % 5 === 1 ? '异常' : row % 5 === 4 ? '缺失' : '有效';
  if (column === 'SNI') return snis[row % snis.length];
  if (column === 'JA3') return ja3[row % ja3.length];
  if (column === 'JA3S') return ja3s[row % ja3s.length];
  if (column === 'ALPN') return protocols[row % protocols.length] === 'QUIC' ? 'h3' : row % 2 === 0 ? 'h2' : 'http/1.1';
  if (column === 'TLS 版本') return row % 4 === 1 ? 'TLS 1.2' : 'TLS 1.3';
  if (column === '密码套件') return row % 2 === 0 ? 'TLS_AES_128_GCM_SHA256' : 'ECDHE-RSA-AES128-GCM-SHA256';
  if (column === '证书 Issuer') return issuers[row % issuers.length];
  if (column === '风险等级') return risks[row % risks.length];
  if (column === '操作') return '下钻';
  return `${column}-${row + 1}`;
};

const forensicCellValue = (column: string, row: number) => {
  const taskIds = ['F-20260620-000189', 'F-20260620-000188', 'F-20260620-000187', 'F-20260620-000186', 'F-20260620-000185', 'F-20260620-000184'];
  const sources = ['AL-20260620-000123', 'AL-20260620-000122', 'AL-20260620-000119', 'APT-20260619-001', 'AL-20260619-001', 'AL-20260618-014'];
  const assets = ['办公区-WS-1024', '财务-SRV-2003', '核心区-DC-01', '宿舍区-PC-3056', '办公区-WS-2011', '教学区-PC-0402'];
  const tuples = [
    '172.16.5.10:44221 -> 185.22.14.9:443 TLS',
    '10.12.8.45:49872 -> 10.12.9.33:445 SMB',
    '172.16.1.77:51322 -> 198.51.100.27:80 HTTP',
    '192.168.3.55:5544 -> 200.0.113.45:443 TLS',
    '10.12.2.36:53513 -> 8.8.8.8:53 DNS',
    '10.12.4.18:41221 -> 203.0.113.45:443 TLS',
  ];
  const packages = ['000123_001.pcap', '000123_002.pcap', '000123_003.pcap', 'APT-001-session.zip', 'evidence-bundle.tar', 'pcap-window-014.pcap'];
  const statuses = ['完成', '采集中', '排队中', '解析中', '失败', '完成'];
  if (column === '任务 ID') return taskIds[row % taskIds.length];
  if (column === '告警/战役 ID') return sources[row % sources.length];
  if (column === '资产') return assets[row % assets.length];
  if (column === '五元组') return tuples[row % tuples.length];
  if (column === '时间窗') return `06-19 00:${String(row * 10).padStart(2, '0')} ~ 06-19 0${row + 1}:00`;
  if (column === '证据包') return packages[row % packages.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '操作') return row % 3 === 0 ? '下载' : '查看';
  return `${column}-${row + 1}`;
};

const ruleCellValue = (column: string, row: number) => {
  const ids = ['C2_Tunnel_v3', 'Lateral_Move_v2', 'DNS_Tunnel_v2', 'Data_Exfil_v1', 'APT_ToolDrop_v1', 'Port_Scan_v1', 'WebShell_Detect_v1'];
  const names = ['C2 隧道通信检测', '横向移动检测', 'DNS 隧道检测', '数据外发检测', 'APT 工具投递检测', '端口扫描检测', 'WebShell 检测'];
  const types = ['流量', '流量', '流量', '流量', '文件', '流量', '流量'];
  const severities = ['高', '高', '中', '高', '高', '中', '高'];
  const phases = ['指挥与控制', '横向移动', '指挥与控制', '数据泄露', '执行', '侦察', '持久化'];
  const statuses = ['启用', '启用', '启用', '启用', '灰度', '启用', '启用'];
  if (column === '规则ID') return ids[row % ids.length];
  if (column === '规则名称') return names[row % names.length];
  if (column === '类型') return types[row % types.length];
  if (column === '严重级别') return severities[row % severities.length];
  if (column === 'MITRE阶段') return phases[row % phases.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '版本') return ['v3.0', 'v2.9', 'v2.6', 'v1.8', 'v1.5', 'v2.3', 'v1.7'][row % 7];
  if (column === '命中数') return ['1.3K', '892', '643', '1.1K', '412', '2.4K', '318'][row % 7];
  if (column === '误报率') return ['0.38%', '0.21%', '0.31%', '0.25%', '0.47%', '0.19%', '0.42%'][row % 7];
  if (column === '平均延时') return ['18 ms', '24 ms', '16 ms', '22 ms', '31 ms', '21 ms', '28 ms'][row % 7];
  return `${column}-${row + 1}`;
};

const deploymentCellValue = (column: string, row: number) => {
  const objects = ['规则包-APT检测增强', '异常流量检测模型', '采集策略-办公区', 'Flink作业-流量聚合', '配置模板-告警阈值', '规则包-僵木马C2检测', '模型-UEBA行为分析', '采集策略-数据中心'];
  const versions = ['v2.3.1', 'v1.8.0', 'v3.0.5', 'job-20250527.1', 'config-v1.2.0', 'v2.1.4', 'model-v1.6.8', 'policy-v4.0.2'];
  const environments = ['canary', 'stage', 'prod', 'prod', 'canary', 'prod', 'stage', 'prod'];
  const statuses = ['灰度中 20%', '待确认', '已发布', '灰度中 50%', '阻断', '可回滚', '待发布', '已发布'];
  const owners = ['安全运营组', '算法平台组', '采集平台组', 'Flink 平台组', '安全运营组', '安全运营组', '算法平台组', '采集平台组'];
  const scopes = ['租户A / 华东园区 / 12 台探针 / 20% 流量', '租户B / 测试园区 / 8 台探针', '办公区资产组 / 全量', '流量聚合作业 / 50% 分区', '核心业务资产组 / 5% 流量', '租户A / 全量', '行为分析资产组 / 离线评估', '数据中心 / 全量探针'];
  if (column === '发布对象') return objects[row % objects.length];
  if (column === '版本') return versions[row % versions.length];
  if (column === '环境') return environments[row % environments.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '负责人') return owners[row % owners.length];
  if (column === '发布时间') return `2026-06-25 ${String(14 - (row % 5)).padStart(2, '0')}:${String(10 + row * 4).padStart(2, '0')}`;
  if (column === '影响范围') return scopes[row % scopes.length];
  if (column === '操作') return row % 5 === 4 ? '回滚' : '查看 / 灰度 / 回滚';
  return `${column}-${row + 1}`;
};

const modelCellValue = (column: string, row: number) => {
  const names = ['UEBA 行为分析', '加密隧道检测', '数据外传识别', 'APT 战役聚类', '异常 DNS 模型', 'DGA 域名检测', '资产指纹识别', '横向移动检测'];
  const types = ['分类', '检测', '分类', '聚类', '检测', '检测', '分类', '检测'];
  const versions = ['v1.8.0', 'v2.3.1', 'v1.5.2', 'v1.2.4', 'v1.6.7', 'v1.3.0', 'v2.0.3', 'v1.1.8'];
  const statuses = ['线上', '候选', '漂移', '待评估', '停用', '线上', '候选', '漂移'];
  const onlineVersions = ['v1.8.0', 'v2.2.0', 'v1.4.1', '-', 'v1.5.0', 'v1.3.0', 'v1.9.1', 'v1.1.2'];
  const owners = ['安全运营组', '网络安全组', '数据安全组', '威胁分析组', '网络安全组', '威胁分析组', '资产管理组', '安全运营组'];
  if (column === '模型名') return names[row % names.length];
  if (column === '类型') return types[row % types.length];
  if (column === '版本') return versions[row % versions.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '线上版本') return onlineVersions[row % onlineVersions.length];
  if (column === '训练时间') return `2026-06-${String(19 - (row % 4)).padStart(2, '0')} ${String(22 - row).padStart(2, '0')}:${String(10 + row * 5).padStart(2, '0')}`;
  if (column === '负责人') return owners[row % owners.length];
  if (column === '操作') return '详情 / 激活 / 回滚';
  return `${column}-${row + 1}`;
};

const mlopsCellValue = (column: string, row: number) => {
  const ids = ['TR-20250527-006', 'TR-20250527-005', 'TR-20250527-004', 'TR-20250527-003', 'TR-20250527-002', 'TR-20250527-001', 'TR-20250526-018', 'TR-20250526-017'];
  const stages = ['训练任务', '评估门禁', '标注管理', '注册模型', '灰度发布', '效果回流', '反馈样本', '特征构建'];
  const datasets = ['ds_v1.6.3', 'ds_v1.6.2', 'ds_v1.6.1', 'ds_v1.6.0', 'ds_v1.5.9', 'ds_v1.5.8', 'feedback_q', 'feature_q'];
  const algos = ['xgb_v2.4', 'lightgbm_v1.3', 'manual_review', 'xgb_v2.4', 'isolation_forest', 'lof_v1.2', 'labeler', 'spark-feat'];
  const features = ['feat_v1.8.7', 'feat_v1.8.7', 'feat_v1.8.6', 'feat_v1.8.6', 'feat_v1.8.5', 'feat_v1.8.5', 'raw_feedback', 'feat_v1.8.7'];
  const resources = ['GPU 70% / CPU 42%', 'GPU 35% / CPU 24%', 'CPU 90% / MEM 78%', 'CPU 5% / MEM 8%', 'CPU 0% / MEM 0%', 'CPU 0% / MEM 0%', 'CPU 12% / MEM 20%', 'CPU 48% / MEM 39%'];
  const statuses = ['运行中', '运行中', '运行中', '排队中', '排队中', '已完成', '待处理', '运行中'];
  if (column === '任务ID') return ids[row % ids.length];
  if (column === '阶段') return stages[row % stages.length];
  if (column === '数据集版本') return datasets[row % datasets.length];
  if (column === '算法配置') return algos[row % algos.length];
  if (column === '特征版本') return features[row % features.length];
  if (column === '资源占用') return resources[row % resources.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '操作') return row % 3 === 0 ? '查看日志' : row % 3 === 1 ? '失败重试' : '停止';
  return `${column}-${row + 1}`;
};

const playbookCellValue = (column: string, row: number) => {
  const names = ['高危主机隔离', 'C2 连接阻断剧本', '异常账号封禁', '恶意脚本下发', '外联域名封禁', '数据外传取证'];
  const alerts = ['高危主机告警', 'C2 连接告警', '账号异常告警', '恶意行为告警', '域名风险告警', '数据外泄告警'];
  const actions = ['隔离 / 通知', '阻断 / 取证', '封禁 / 通知', '脚本 / 回滚', '封禁 / Sinkhole', '取证 / 升级'];
  const risks = ['高危', '高危', '中危', '中危', '中危', '高危'];
  const statuses = ['已启用', '已启用', '已启用', '草稿', '待审批', '已启用'];
  if (column === '剧本名称') return names[row % names.length];
  if (column === '适用告警') return alerts[row % alerts.length];
  if (column === '动作类型') return actions[row % actions.length];
  if (column === '风险级别') return risks[row % risks.length];
  if (column === '启用状态') return statuses[row % statuses.length];
  if (column === '最近执行') return `2025-05-27 ${String(13 + row).padStart(2, '0')}:${String(52 - row * 3).padStart(2, '0')}`;
  if (column === '操作') return '执行 / 编辑 / 审计';
  return `${column}-${row + 1}`;
};

const whitelistCellValue = (column: string, row: number) => {
  const types = ['IP', '资产', '域名', '账号', '规则', '模型'];
  const values = ['10.****.23', '服务器组-高速缓存', 'update.campus.local', 'svc_backup', 'Rule-100324', '模型-异常登录'];
  const scopes = ['研发网络', '测试环境', '全网', '备份系统', '全网', '办公网'];
  const periods = ['2026-06-01 ~ 2026-07-01', '2026-05-20 ~ 2026-06-20', '2026-06-10 ~ 2026-07-10', '2026-05-15 ~ 2026-06-15', '2026-04-01 ~ 2026-10-01', '2026-05-01 ~ 2026-11-01'];
  const owners = ['安全运营', '平台团队', '安全运营', '平台团队', '安全运营', '数据科学'];
  const alerts = ['AL-20260618-0451', 'AL-20260617-0322', 'AL-20260619-0187', 'AL-20260614-0059', 'AL-20260530-0011', 'AL-20260528-0114'];
  const statuses = ['生效', '即将到期', '待审批', '过期', '生效', '高风险覆盖'];
  if (column === '对象类型') return types[row % types.length];
  if (column === '匹配条件') return values[row % values.length];
  if (column === '生效范围') return scopes[row % scopes.length];
  if (column === '有效期') return periods[row % periods.length];
  if (column === '责任角色') return owners[row % owners.length];
  if (column === '来源告警') return alerts[row % alerts.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '操作') return '查看 / 编辑 / 延期';
  return `${column}-${row + 1}`;
};

const complianceCellValue = (column: string, row: number) => {
  const dimensions = ['采集覆盖', '数据质量', '告警链路', 'PCAP 证据', 'MLOps', '审计留痕', '部署基线'];
  const taskRates = ['96.7%', '93.2%', '88.1%', '91.3%', '82.0%', '97.4%', '78.6%'];
  const tests = ['24 / 25', '28 / 30', '15 / 18', '22 / 24', '18 / 22', '12 / 12', '11 / 16'];
  const sourceRates = ['98.5%', '95.2%', '90.7%', '94.6%', '83.3%', '100%', '76.1%'];
  const evidenceRates = ['100%', '96%', '89%', '95%', '81%', '100%', '72%'];
  const dates = ['2026-06-18', '2026-06-19', '2026-06-17', '2026-06-18', '2026-06-16', '2026-06-19', '2026-06-15'];
  const results = ['通过', '通过', '待整改', '通过', '待整改', '通过', '未达标'];
  if (column === '维度') return dimensions[row % dimensions.length];
  if (column === '任务书指标(覆盖率)') return taskRates[row % taskRates.length];
  if (column === '测试项(通过/总数)') return tests[row % tests.length];
  if (column === '数据源(覆盖率)') return sourceRates[row % sourceRates.length];
  if (column === '证据状态(完整度)') return evidenceRates[row % evidenceRates.length];
  if (column === '最近复验(日期间)') return dates[row % dates.length];
  if (column === '结果') return results[row % results.length];
  return `${column}-${row + 1}`;
};

const auditLogCellValue = (column: string, row: number) => {
  const users = ['sec_admin / 安全管理员', 'ops_admin / 运维管理员', 'ml_admin / 模型管理员', 'soar_bot / 自动化账号', 'iam_admin / 身份管理员', 'audit_user / 审计员'];
  const resources = ['PCAP', '规则', '模型', '脚本', '令牌', '合规报告', '白名单', '部署'];
  const actions = ['访问', '发布', '激活', '执行', '变更', '导出', '新增', '回滚'];
  const results = ['成功', '成功', '成功', '成功', '成功', '待复核', '失败', '成功'];
  const risks = ['中风险', '中风险', '高风险', '中风险', '高风险', '高风险', '高风险', '低风险'];
  if (column === '时间') return `2026-06-21 15:${String(32 - row).padStart(2, '0')}:21`;
  if (column === '用户/角色') return users[row % users.length];
  if (column === '对象类型') return resources[row % resources.length];
  if (column === '动作类型') return actions[row % actions.length];
  if (column === '结果') return results[row % results.length];
  if (column === '请求ID') return `req-${String(row + 1).padStart(2, '0')}-${['7f8a2c91b3d4', '5c7e1ab2f8d3', '2b9f664a1c7e', '9a1d3e7c4b2f'][row % 4]}`;
  if (column === 'trace_id') return `trace-${String(row + 1).padStart(2, '0')}-${['3a9f1d7c2b6e4f2d', 'b1c642a7d5f8e9f', '6d2a9e3b7f1c4d8b'][row % 3]}`;
  if (column === '风险标签') return risks[row % risks.length];
  if (column === '操作') return '详情 / 关联 / 复核';
  return `${column}-${row + 1}`;
};

const notificationCellValue = (column: string, row: number) => {
  const rules = ['严重告警', '高危告警', '中危告警', '低危告警', '验收缺口', '任务失败', '系统异常', '数据质量'];
  const severities = ['高危', '高危', '中危', '低危', '高危', '中危', '中危', '高危'];
  const alertTypes = ['攻击告警', '数据泄露', '异常登录', '扫描告警', '合规缺口', '任务失败', '系统异常', '数据质量'];
  const scopes = ['核心资产 / 主园区', '财务系统 / 主园区', '终端设备 / 分园区A', '网络设备 / 分园区B', '全部资产', '全部资产', '平台服务 / 主园区', 'Kafka / Flink'];
  const windows = ['夜间 00:00-08:00', '全天', '工作日 08:00-20:00', '全天', '工作日 09:00-18:00', '全天', '全天', '工作日 08:00-22:00'];
  const channels = ['邮件 / Webhook / 企业微信', '邮件 / 钉钉 / 工单系统', '邮件 / 短信', 'Webhook', '邮件 / 工单系统', '钉钉 / 飞书', '企业微信 / Webhook', '邮件 / 工单系统'];
  const escalation = ['夜间升级策略', '安全值班升级', '运维升级策略', '普通提醒', '验收升级策略', '运维升级策略', '平台升级策略', '质量升级策略'];
  const silence = ['低优先级静默', '重复合并', '专题免打扰', '低优先级静默', '无', '重复合并', '维护窗口', '专题免打扰'];
  const statuses = ['启用', '启用', '启用', '启用', '启用', '停用', '启用', '草稿'];
  if (column === '规则') return rules[row % rules.length];
  if (column === '严重级别') return severities[row % severities.length];
  if (column === '告警类型') return alertTypes[row % alertTypes.length];
  if (column === '资产组/园区') return scopes[row % scopes.length];
  if (column === '时间窗') return windows[row % windows.length];
  if (column === '渠道') return channels[row % channels.length];
  if (column === '升级策略') return escalation[row % escalation.length];
  if (column === '静默') return silence[row % silence.length];
  if (column === '状态') return statuses[row % statuses.length];
  if (column === '操作') return '规则 / 更多';
  return `${column}-${row + 1}`;
};

const settingsCellValue = (column: string, row: number) => {
  const names = ['SOAR-Executor', 'Model-Service', 'PCAP-Export', 'Webhook-Alert', 'ReadOnly-Dashboard', 'Probe-Ingest', 'Audit-Exporter', 'Screen-Readonly'];
  const scopes = ['脚本执行、证据导出', '模型激活、规则查询', 'PCAP访问、证据导出', '告警触达', '只读访问', '探针接入、探针指标', '审计导出、合规报告', '大屏只读、脱敏查看'];
  const prefixes = ['c1a7****9f2e', 'f3b8****0d11', '9e7d****21b4', '6a2c****e0f0', 'a7d9****3c18', 'd2f4****8b22', 'b93c****1a70', 'e61f****7c09'];
  const expires = ['2026-07-15', '2026-08-10', '2026-06-28', '2026-07-01', '2026-12-31', '2026-09-30', '2027-01-15', '2026-11-20'];
  const used = ['2026-06-21 11:23', '2026-06-21 09:41', '2026-06-20 16:02', '2026-06-18 14:10', '2026-06-21 08:15', '2026-06-21 10:35', '2026-06-20 21:44', '2026-06-21 07:55'];
  const statuses = ['正常', '正常', '即将过期', '正常', '正常', '自动轮换', '正常', '正常'];
  if (column === '令牌名称') return names[row % names.length];
  if (column === '权限范围') return scopes[row % scopes.length];
  if (column === '令牌指纹') return prefixes[row % prefixes.length];
  if (column === '过期时间') return expires[row % expires.length];
  if (column === '最近使用') return used[row % used.length];
  if (column === '轮换状态') return statuses[row % statuses.length];
  if (column === '操作') return '轮换 / 吊销';
  return `${column}-${row + 1}`;
};
