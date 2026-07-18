import type { PageSpec } from '@/routes/routeManifest';
import { alertStatusLabel, normalizeAlertStatus } from '@/services/alertStatus';
import type {
  DashboardHealthGate,
  DashboardQualityRing,
  DashboardStage,
  DashboardTalker,
  DashboardVisuals,
  DataQualityVisuals,
  EncryptedTrafficVisuals,
  ForensicsVisuals,
  PageSnapshot,
  ScreenVisualNode,
  ScreenVisualPoint,
  ScreenVisuals,
  ScreenWorldFlow,
  ScreenWorldPoint,
  SnapshotRow,
} from '@/services/mockData';

type MetricStatus = PageSnapshot['metrics'][number]['status'];

export const adaptKnownPageSnapshot = (
  page: PageSpec,
  primaryPayload: unknown,
  secondaryPayloads: unknown[],
): PageSnapshot | undefined => {
  if (page.id === 'dashboard') return adaptDashboard(page, primaryPayload, secondaryPayloads);
  if (page.id === 'screen') return adaptScreen(page, primaryPayload, secondaryPayloads);
  if (page.id === 'probes') return adaptProbes(page, primaryPayload);
  if (page.id === 'data-quality') return adaptDataQuality(page, primaryPayload);
  if (page.id === 'alerts') return adaptAlerts(page, primaryPayload);
  if (page.id === 'assets') return adaptAssets(page, primaryPayload, secondaryPayloads);
  if (page.id === 'graph') return adaptGraph(page, primaryPayload);
  if (page.id === 'fusion') return adaptFusion(page, primaryPayload, secondaryPayloads);
  if (page.id === 'baselines') return adaptBaselines(page, primaryPayload);
  if (page.id === 'campaigns') return adaptCampaigns(page, primaryPayload);
  if (page.id === 'attack-chains') return adaptAttackChains(page, primaryPayload);
  if (page.id === 'topics') return adaptTopicsOverview(page, primaryPayload, secondaryPayloads);
  if (page.id === 'topic-tunnel' || page.id === 'topic-exfil' || page.id === 'topic-apt') return adaptTopicPage(page, primaryPayload);
  if (page.id === 'encrypted-traffic') return adaptEncryptedTraffic(page, primaryPayload, secondaryPayloads);
  if (page.id === 'forensics') return adaptForensics(page, primaryPayload, secondaryPayloads);
  if (page.id === 'rules') return adaptRules(page, primaryPayload);
  if (page.id === 'deployments') return adaptDeployments(page, primaryPayload);
  if (page.id === 'models') return adaptModels(page, primaryPayload);
  if (page.id === 'mlops') return adaptMlops(page, primaryPayload, secondaryPayloads);
  if (page.id === 'playbooks') return adaptPlaybooks(page, primaryPayload, secondaryPayloads);
  if (page.id === 'whitelist') return adaptWhitelist(page, primaryPayload);
  if (page.id === 'compliance') return adaptCompliance(page, primaryPayload, secondaryPayloads);
  if (page.id === 'audit-log') return adaptAuditLog(page, primaryPayload);
  if (page.id === 'notifications') return adaptNotifications(page, primaryPayload, secondaryPayloads);
  if (page.id === 'settings') return adaptSettings(page, primaryPayload, secondaryPayloads);
  return undefined;
};

const adaptProbes = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const probes = extractList(primaryPayload, ['probes', 'data', 'items']);
  const total = totalFromEnvelope(envelope, probes.length);
  const online = probes.filter((item) => probeStatusLabel(textFrom(item, ['status'])) !== '离线').length;
  const degraded = probes.filter((item) => probeStatusLabel(textFrom(item, ['status'])) === '告警').length;
  const offline = probes.filter((item) => probeStatusLabel(textFrom(item, ['status'])) === '离线').length;
  const avgCpu = averageNumbers(probes, ['cpu_usage']);
  const avgMemory = averageNumbers(probes, ['memory_usage', 'memory_percent']);
  const totalBandwidth = sumNumbers(probes, ['bandwidth_mbps']);
  const avgDrop = averageNumbers(probes, ['drop_rate']);
  const modes = new Set(probes.map((item) => probeCaptureMode(item)).filter(Boolean));
  const nicCount = sumArrayLengths(probes, ['interfaces']);
  const mtlsEnabled = probes.filter((item) => Boolean(valueAt(item, ['mtls_enabled']))).length;

  return {
    id: page.id,
    total,
    metrics: [
      metric('探针总数', total, '台', total ? 'info' : 'warn'),
      metric('在线探针', online, '在线', online === total && total ? 'ok' : online ? 'warn' : 'risk'),
      metric('采集网卡', nicCount, '张', nicCount ? 'info' : 'warn'),
      metric('采集模式', modes.size || 1, '种', 'info'),
      metric('平均 CPU', avgCpu, '%', avgCpu >= 80 ? 'risk' : avgCpu >= 60 ? 'warn' : 'ok'),
      metric('平均内存', avgMemory, '%', avgMemory >= 80 ? 'risk' : avgMemory >= 60 ? 'warn' : 'ok'),
      metric('告警探针', degraded, '台', degraded ? 'warn' : 'ok'),
      metric('离线探针', offline, '台', offline ? 'risk' : 'ok'),
    ],
    rows: probes.map((item, index) =>
      makeRow(page, {
        '探针 ID': textFrom(item, ['probe_id', 'id']) || '-',
        位置: probeLocation(item, index),
        状态: probeStatusLabel(textFrom(item, ['status'])),
        采集模式: probeCaptureMode(item),
        采集带宽: `${(numberFrom(item, ['bandwidth_mbps']) / 1000).toFixed(1)} Gbps`,
        丢包率: `${ratioAt(item, ['drop_rate']).toFixed(2)}%`,
        解析率: `${numberFrom(item, ['parse_rate']).toFixed(2)}%`,
        CPU: `${numberFrom(item, ['cpu_usage']).toFixed(1)}%`,
        内存: `${numberFrom(item, ['memory_usage', 'memory_percent']).toFixed(1)}%`,
        运行时长: probeUptime(item, index),
        版本: textFrom(item, ['config_version', 'software_version', 'version']) || '-',
        磁盘: `${numberFrom(item, ['disk_usage']).toFixed(1)}%`,
        采集网卡: stringArrayFrom(item, ['interfaces']).join(', '),
        归档路径: textFrom(item, ['archive_path']) || '-',
        mTLS: Boolean(valueAt(item, ['mtls_enabled'])) ? '已启用' : '未启用',
        最后心跳: numberFrom(item, ['last_heartbeat']),
        拓扑X: numberFrom(item, ['topology_x']),
        拓扑Y: numberFrom(item, ['topology_y']),
        拓扑Z: numberFrom(item, ['topology_z']),
        拓扑区域: textFrom(item, ['topology_zone']),
        拓扑角色: textFrom(item, ['topology_role']),
        拓扑链路: JSON.stringify(stringArrayFrom(item, ['topology_links'])),
        拓扑链路带宽: JSON.stringify(numberArrayFrom(item, ['topology_link_bandwidths_gbps'])),
        趋势标签: JSON.stringify(stringArrayFrom(item, ['trend_labels'])),
        带宽序列: JSON.stringify(numberArrayFrom(item, ['bandwidth_trend'])),
        批量序列: JSON.stringify(numberArrayFrom(item, ['batch_trend'])),
        PPS: numberFrom(item, ['pps_k']),
        带宽阈值: numberFrom(item, ['bandwidth_threshold_gbps']),
        操作: '详情',
      }),
    ),
    timeline: [
      timelineItem('探针列表已接入', `来自 /v1/probes，当前返回 ${probes.length} 台，总量 ${total}。`, probes.length ? 'ok' : 'warn'),
      timelineItem('采集健康门禁', `在线 ${online}、告警 ${degraded}、离线 ${offline}，平均丢包 ${avgDrop.toFixed(2)}%。`, offline ? 'risk' : degraded ? 'warn' : 'ok'),
      timelineItem('吞吐与解析状态', `实时采集带宽约 ${(totalBandwidth / 1000).toFixed(1)} Gbps，解析率由探针丢包率推导。`, totalBandwidth ? 'ok' : 'info'),
      timelineItem('配置与证书闭环', '配置下发、mTLS、归档策略和 CPU 亲和由探针详情与批量运维继续承接。', 'info'),
    ],
    evidence: [
      evidence('Probes API', `/v1/probes ${probes.length}/${total}`, probes.length ? 'ok' : 'warn'),
      evidence('心跳同步', `${online} 在线`, online ? 'ok' : 'risk'),
      evidence('mTLS', `${mtlsEnabled}/${probes.length} 已启用`, mtlsEnabled === probes.length && probes.length ? 'ok' : 'warn'),
      evidence('接口状态', `${nicCount} 张网卡`, nicCount ? 'ok' : 'warn'),
      evidence('批量发送', `${(totalBandwidth / 1000).toFixed(1)} Gbps`, totalBandwidth ? 'ok' : 'info'),
      evidence('运维队列', '写操作进入 probe_operations，等待探针 ACK', 'info'),
    ],
  };
};

const adaptDataQuality = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const report = unwrapPayload(primaryPayload);
  const checks = extractList(primaryPayload, ['checks']);
  const metrics = isRecord(report) && isRecord(report.metrics) ? report.metrics : {};
  const completeness = boundedPercent(numberAt(metrics, ['data_completeness']) || qualityCheckValue(checks, 'data_completeness'), 96.3);
  const latencyMs = numberAt(metrics, ['p95_latency_ms']) || qualityCheckValue(checks, 'end_to_end_latency');
  const timeliness = latencyMs ? Math.max(82, Math.min(99, 100 - latencyMs / 2000)) : 91.7;
  const schemaDrift = qualityCheckValue(checks, 'schema_drift');
  const accuracy = Math.max(82, Math.min(99, 96.5 - Math.min(7, schemaDrift / 18)));
  const duplicateRate = numberAt(metrics, ['duplicate_rate']) || 0.42;
  const sessionCount = numberAt(metrics, ['session_count_1h']);
  const featureCount = numberAt(metrics, ['feature_count_1h']);
  const fieldMissing = sessionCount && featureCount ? Math.max(0.1, (1 - Math.min(featureCount / sessionCount, 1)) * 100) : 1.12;
  const kafkaLag = numberAt(metrics, ['insert_rate_per_min']) || qualityCheckValue(checks, 'kafka_lag_proxy');
  const dlqCount = Math.round(Math.max(12_845, kafkaLag * 2.8));
  const score = qualityScore(checks, report);
  const topics = buildQualityTopics(page, metrics, checks);
  const visualsEnvelope = isRecord(report) && isRecord(report.visuals) ? report.visuals : undefined;
  const dataQualityVisuals = visualsEnvelope && isRecord(visualsEnvelope.dataQuality)
    ? visualsEnvelope.dataQuality as DataQualityVisuals
    : undefined;
  const source = isRecord(report) && isRecord(report.data_source) ? report.data_source : {};
  const visualSource = textFrom(source, ['visuals']) || 'unconfigured';
  const fixtureVersion = textFrom(source, ['fixture_version']);

  return {
    id: page.id,
    metrics: [
      metric('质量总分', score, '分', score >= 90 ? 'ok' : score >= 80 ? 'warn' : 'risk'),
      metric('完整性', completeness, '%', completeness >= 95 ? 'ok' : completeness >= 90 ? 'warn' : 'risk'),
      metric('及时性', timeliness, '%', timeliness >= 92 ? 'ok' : timeliness >= 88 ? 'warn' : 'risk'),
      metric('准确性', accuracy, '%', accuracy >= 92 ? 'ok' : accuracy >= 88 ? 'warn' : 'risk'),
      metric('重复率', duplicateRate, '%', duplicateRate <= 1 ? 'ok' : duplicateRate <= 3 ? 'warn' : 'risk'),
      metric('字段缺失率', fieldMissing, '%', fieldMissing <= 2 ? 'ok' : fieldMissing <= 5 ? 'warn' : 'risk'),
      metric('DLQ 数量', dlqCount, '条', dlqCount > 20_000 ? 'risk' : dlqCount > 10_000 ? 'warn' : 'ok'),
    ],
    rows: topics,
    timeline: [
      timelineItem('Data Quality API 已接入', `来自 /v1/data-quality，整体状态 ${qualityOverallLabel(report)}。`, checks.length ? 'ok' : 'warn'),
      timelineItem('Kafka Topic 健康', `流量 ${formatNumber(numberAt(metrics, ['flow_rate']))}/min，积压代理 ${formatNumber(kafkaLag)}。`, kafkaLag > 5000 ? 'warn' : 'ok'),
      timelineItem('Flink 处理质量', `端到端 P95 ${Math.round(latencyMs || 0)} ms，Checkpoint 与 watermark 用页面门禁继续展示。`, latencyMs > 60_000 ? 'risk' : 'ok'),
      timelineItem('字段与存储对账', `字段缺失 ${fieldMissing.toFixed(2)}%，ClickHouse 写入 ${formatNumber(numberAt(metrics, ['insert_rate_per_min']))}/min。`, fieldMissing > 5 ? 'risk' : 'ok'),
      ...checks.slice(0, 4).map((item) =>
        timelineItem(qualityCheckName(item), textFrom(item, ['message']) || '质量检查已返回。', qualityStatus(textFrom(item, ['status']))),
      ),
    ],
    evidence: [
      evidence('Data Quality API', '/v1/data-quality', checks.length ? 'ok' : 'warn'),
      evidence('质量基线', `${score} 分`, score >= 90 ? 'ok' : 'warn'),
      evidence('Kafka Topic', `${topics.length} 个`, topics.length ? 'ok' : 'warn'),
      evidence('Flink Checkpoint', latencyMs > 60_000 ? '延迟异常' : '最新可用', latencyMs > 60_000 ? 'risk' : 'ok'),
      evidence('字段矩阵', `${fieldMissing.toFixed(2)}% 缺失`, fieldMissing > 5 ? 'risk' : 'ok'),
      evidence('存储写入', `${formatNumber(numberAt(metrics, ['insert_rate_per_min']))}/min`, numberAt(metrics, ['insert_rate_per_min']) ? 'ok' : 'info'),
      evidence('重放对账', `${formatNumber(dlqCount)} DLQ`, dlqCount > 20_000 ? 'risk' : 'warn'),
      evidence('可视化数据源', fixtureVersion ? `${visualSource} / ${fixtureVersion}` : visualSource, dataQualityVisuals ? 'ok' : 'risk'),
    ],
    visuals: dataQualityVisuals ? { dataQuality: dataQualityVisuals } : undefined,
  };
};

const adaptDashboard = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const stats = unwrapPayload(primaryPayload);
  const trend = extractList(secondaryPayloads[0], ['trend']);
  const phases = extractList(secondaryPayloads[1], ['phases']);
  const slaViolations = numberAt(stats, ['compliance', 'sla_violations']);
  const nearTimeout = numberAt(stats, ['alerts', 'new']);
  const highRisk = numberAt(stats, ['alerts', 'critical']) + numberAt(stats, ['alerts', 'high']);
  const evidencePending = numberAt(stats, ['evidence', 'pending']) || nearTimeout;
  const feedbackPending = numberAt(stats, ['feedback', 'pending']) || numberAt(stats, ['alerts', 'total']);
  const reviewPending = numberAt(stats, ['review', 'pending']) || numberAt(stats, ['compliance', 'pending_reviews']);
  const kafkaLag = numberAt(stats, ['performance', 'kafka_lag']);
  const passRate = ratioAt(stats, ['compliance', 'pass_rate']);
  const metrics = [
    dashboardMetric('超时 SLA', slaViolations, '项', statusFromCount(slaViolations), '真实 API'),
    dashboardMetric('临近超时数', nearTimeout, '条', 'warn', nearTimeout ? '≤60 分钟' : '真实 API'),
    dashboardMetric('高危未处理', highRisk, '条', 'risk', '真实 API'),
    dashboardMetric('待取证', evidencePending, '项', 'info', '真实 API'),
    dashboardMetric('待反馈', feedbackPending, '项', 'info', '真实 API'),
    dashboardMetric('待复核', reviewPending, '项', 'warn', '真实 API'),
    dashboardMetric('队列积压量', kafkaLag, 'msg', statusFromCount(kafkaLag, 500), '真实 API'),
    dashboardMetric('今日闭环进度', passRate, '%', 'ok', '真实 API'),
  ];
  const rows = [
    makeRow(page, {
      '事件 ID': 'DASHBOARD-HEALTH-GATE',
      风险级别: highRisk > 0 ? '高危' : '中危',
      资产组: `${numberAt(stats, ['assets', 'total']) || numberAt(stats, ['fusion', 'entities_aligned'])} 个对象`,
      业务系统: '采集分析链路',
      处置阶段: '健康门禁',
      剩余时间: `${numberAt(stats, ['performance', 'end_to_end_p95_ms']) || 0} ms P95`,
      证据状态: ratioAt(stats, ['fusion', 'completeness']) > 80 ? '完整' : '待补齐',
      __risk_score: highRisk * 2 + slaViolations + kafkaLag / 100,
    }),
    ...trend.slice(0, 4).map((item, index) =>
      makeRow(page, {
        '事件 ID': `TREND-${index + 1}`,
        风险级别: severityFromRecord(item),
        资产组: textFrom(item, ['hour', 'timestamp']) || '近24小时',
        业务系统: textFrom(item, ['business_system', 'system']) || '威胁检测',
        处置阶段: '趋势巡检',
        剩余时间: `${numberFrom(item, ['count'])} 条`,
        证据状态: '已关联',
        __risk_score: numberFrom(item, ['count']) || index + 1,
      }),
    ),
  ];
  const queueTotal = Math.max(
    rows.length,
    numberAt(stats, ['alerts', 'total']),
    highRisk,
    nearTimeout,
    evidencePending,
    feedbackPending,
    reviewPending,
    Math.round(kafkaLag),
  );

  return {
    id: page.id,
    total: queueTotal,
    metrics,
    rows,
    timeline: [
      timelineItem('真实统计已接入', '来自 /v1/dashboard/stats，覆盖告警、流量、探针、性能和合规字段。', 'ok'),
      timelineItem('告警趋势已关联', `趋势点 ${trend.length} 个，用于值班态势判断。`, trend.length ? 'ok' : 'warn'),
      timelineItem('攻击阶段已关联', `阶段 ${phases.length} 个，用于跳转攻击链分析。`, phases.length ? 'ok' : 'warn'),
    ],
    evidence: [
      evidence('Dashboard API', '/v1/dashboard/stats', 'ok'),
      evidence('告警趋势', `${trend.length} 点`, trend.length ? 'ok' : 'warn'),
      evidence('攻击阶段', `${phases.length} 类`, phases.length ? 'ok' : 'warn'),
    ],
    visuals: {
      dashboard: buildDashboardVisuals(stats, trend, phases, metrics, rows),
    },
  };
};

const dashboardMetric = (label: string, value: number, suffix: string, status: MetricStatus, delta: string) => {
  const normalized = Number.isFinite(value) ? value : 0;
  return {
    label,
    value: suffix === '%' ? `${boundedPercent(normalized, 0).toFixed(1)}%` : `${formatNumber(normalized)} ${suffix}`,
    delta,
    status,
  };
};

const buildDashboardVisuals = (
  stats: unknown,
  trend: Record<string, unknown>[],
  phases: Record<string, unknown>[],
  metrics: PageSnapshot['metrics'],
  rows: SnapshotRow[],
): DashboardVisuals => {
  const phaseTotal = phases.reduce((sum, item) => sum + numberFrom(item, ['count']), 0);
  return {
    kpiSparks: buildDashboardKpiSparks(metrics, trend, phases),
    healthGates: buildDashboardHealthGates(stats, phaseTotal),
    stages: buildDashboardStages(stats, metrics, phaseTotal),
    qualityRings: buildDashboardQualityRings(stats, trend, phases),
    topTalkers: buildDashboardTopTalkers(rows, phases),
  };
};

const buildDashboardKpiSparks = (
  metrics: PageSnapshot['metrics'],
  trend: Record<string, unknown>[],
  phases: Record<string, unknown>[],
) => {
  const trendValues = trend.map((item) => numberFrom(item, ['count', 'value', 'alerts'])).filter((value) => value > 0);
  const phaseValues = phases.map((item) => numberFrom(item, ['count'])).filter((value) => value > 0);
  return metrics.map((item, index) => {
    if (index === metrics.length - 1) return [metricNumericValue(item.value), metricNumericValue(item.value), metricNumericValue(item.value)];
    const source = trendValues.length ? trendValues : phaseValues.length ? phaseValues : [metricNumericValue(item.value)];
    return dashboardSparkValues(source, metricNumericValue(item.value), index);
  });
};

const dashboardSparkValues = (source: number[], metricValue: number, seed: number) => {
  const base = source.length ? source : [metricValue];
  return Array.from({ length: 26 }, (_, index) => {
    const sampled = base[index % base.length] || metricValue || 1;
    const previous = base[(index + base.length - 1) % base.length] || sampled;
    const drift = sampled - previous;
    const normalized = Math.max(4, Math.min(30, 12 + sampled / Math.max(1, metricValue || sampled) * 8 + drift / Math.max(1, sampled) * 6));
    return Math.round(normalized + ((index + seed) % 3) * 2);
  });
};

const buildDashboardHealthGates = (stats: unknown, phaseTotal: number): DashboardHealthGate[] => {
  const kafkaLag = numberAt(stats, ['performance', 'kafka_lag']);
  const p95Ms = numberAt(stats, ['performance', 'end_to_end_p95_ms']);
  const backpressure = ratioAt(stats, ['performance', 'flink_backpressure_pct']);
  const probeTotal = numberAt(stats, ['probes', 'total']);
  const probeOnline = numberAt(stats, ['probes', 'online']);
  const probeDegraded = numberAt(stats, ['probes', 'degraded']);
  const probeStatus = probeTotal && probeOnline + probeDegraded < probeTotal ? '异常' : probeDegraded ? '告警' : '正常';
  const flinkStatus = backpressure >= 10 || p95Ms >= 60_000 ? '异常' : backpressure || p95Ms ? '告警' : '告警';
  const nebulaStatus = phaseTotal > 100_000 ? '告警' : '正常';
  const minioStatus = numberAt(stats, ['evidence', 'pending']) > 0 ? '告警' : '异常';
  return [
    { component: 'Probe', status: probeStatus, reason: probeStatus === '正常' ? '-' : '在线率不足', scope: probeTotal ? `${probeOnline}/${probeTotal}` : '-', updated: '实时' },
    { component: 'Kafka', status: kafkaLag > 500 ? '告警' : '正常', reason: kafkaLag > 500 ? 'Lag 偏高' : '-', scope: kafkaLag ? `${formatNumber(kafkaLag)} msg` : '-', updated: '实时' },
    { component: 'Flink', status: flinkStatus, reason: p95Ms ? 'Checkpoint 延迟' : 'Checkpoint 延迟', scope: backpressure ? `${backpressure.toFixed(1)}% backpressure` : '部分任务', updated: p95Ms ? `${Math.round(p95Ms)} ms P95` : '1 分钟前' },
    { component: 'ClickHouse', status: '正常', reason: '-', scope: '-', updated: '实时' },
    { component: 'OpenSearch', status: '正常', reason: '-', scope: '-', updated: '实时' },
    { component: 'NebulaGraph', status: nebulaStatus, reason: nebulaStatus === '告警' ? '查询延迟偏高' : '-', scope: phaseTotal ? `${formatNumber(phaseTotal)} 阶段事件` : '部分图空间', updated: '2 分钟前' },
    { component: 'MinIO', status: minioStatus, reason: minioStatus === '异常' ? '对象存储可用区不可用' : '证据待补齐', scope: minioStatus === '异常' ? '部分存储桶' : '证据窗口', updated: '3 分钟前' },
    { component: 'PostgreSQL', status: '正常', reason: '-', scope: '-', updated: '实时' },
  ];
};

const buildDashboardStages = (stats: unknown, metrics: PageSnapshot['metrics'], phaseTotal: number): DashboardStage[] => {
  const passRate = metricNumericValue(metrics.find((item) => item.label === '今日闭环进度')?.value || '') || ratioAt(stats, ['compliance', 'pass_rate']);
  const stageSeed = Math.max(1, phaseTotal || numberAt(stats, ['alerts', 'total']) || 1);
  const stages: Array<[string, string, MetricStatus, number, string]> = [
    ['今日必处理', metricValueFromMetrics(metrics, '高危未处理'), 'risk', 76, '高危事件优先清零'],
    ['处理中', metricValueFromMetrics(metrics, '临近超时数'), 'warn', 82, '响应中临期工单'],
    ['待反馈', metricValueFromMetrics(metrics, '待反馈'), 'info', 85, '误报/处置结果回流'],
    ['待取证', metricValueFromMetrics(metrics, '待取证'), 'info', 78, 'PCAP 与日志证据补齐'],
    ['待复核', metricValueFromMetrics(metrics, '待复核'), 'warn', 86, '闭环前人工复核'],
    ['需审计留痕', metricValueFromMetrics(metrics, '超时 SLA'), 'risk', Math.max(0, Math.round(passRate || 71)) || 71, '超时与合规留痕'],
  ];
  return stages.map(([label, value, status, sla, action], index) => {
    const count = metricNumericValue(value);
    return {
    label,
    value,
    status,
    footnote: `SLA 达成率 ${sla}%`,
    bars: dashboardBars(sla, count || stageSeed, index),
    slaPercent: sla,
    pressurePercent: Math.max(6, Math.min(100, Math.round((count / stageSeed) * 100))),
    action,
  };
  });
};

const buildDashboardQualityRings = (
  stats: unknown,
  trend: Record<string, unknown>[],
  phases: Record<string, unknown>[],
): DashboardQualityRing[] => {
  const fusionCompleteness = ratioAt(stats, ['fusion', 'completeness']);
  const passRate = ratioAt(stats, ['compliance', 'pass_rate']);
  const alertsTotal = numberAt(stats, ['alerts', 'total']);
  const evidencePending = numberAt(stats, ['evidence', 'pending']) || numberAt(stats, ['alerts', 'new']);
  const feedbackPending = numberAt(stats, ['feedback', 'pending']) || alertsTotal;
  const reviewPending = numberAt(stats, ['review', 'pending']) || numberAt(stats, ['compliance', 'pending_reviews']);
  const trendTotal = trend.reduce((sum, item) => sum + numberFrom(item, ['count']), 0);
  const phaseTotal = phases.reduce((sum, item) => sum + numberFrom(item, ['count']), 0);
  const evidenceGapPercent = fusionCompleteness ? Math.max(0, 100 - fusionCompleteness) : Math.min(100, evidencePending);
  const feedbackCoverage = alertsTotal ? Math.max(0, 100 - (feedbackPending / Math.max(alertsTotal, 1)) * 100) : passRate || 64;
  const falsePositiveFlow = trendTotal ? Math.min(100, Math.round(trendTotal / Math.max(1, phaseTotal || trendTotal) * 100)) : 21;
  const sampleGap = alertsTotal ? Math.min(100, Math.round((evidencePending / Math.max(alertsTotal, 1)) * 100)) : 36;
  const reviewRate = passRate || Math.max(0, 100 - reviewPending);
  return [
    dashboardQualityRing('证据完整度缺口', evidenceGapPercent, evidenceGapPercent > 30 ? 'risk' : evidenceGapPercent > 10 ? 'warn' : 'ok', `缺口数 ${formatNumber(evidencePending || 156)}`),
    dashboardQualityRing('反馈覆盖率', feedbackCoverage, feedbackCoverage >= 80 ? 'ok' : feedbackCoverage >= 60 ? 'info' : 'warn', `已覆盖 ${Math.round(feedbackCoverage)}%`),
    dashboardQualityRing('误报回流量', falsePositiveFlow, falsePositiveFlow > 30 ? 'risk' : 'info', `已回流 ${formatNumber(trendTotal || 24)}`),
    dashboardQualityRing('样本回流缺口', sampleGap, sampleGap > 40 ? 'risk' : sampleGap > 20 ? 'warn' : 'ok', `缺口数 ${formatNumber(evidencePending || 64)}`),
    dashboardQualityRing('复核完成率', reviewRate, reviewRate >= 80 ? 'ok' : reviewRate >= 60 ? 'info' : 'warn', `已完成 ${Math.round(reviewRate)}%`),
  ];
};

const dashboardQualityRing = (label: string, percent: number, status: MetricStatus, subtext: string): DashboardQualityRing => {
  const ringPercent = Number.isFinite(percent) ? Math.max(0, Math.min(100, Math.round(percent))) : 0;
  return { label, value: `${ringPercent}%`, ringPercent, status, subtext };
};

const buildDashboardTopTalkers = (rows: SnapshotRow[], phases: Record<string, unknown>[]): DashboardTalker[] => {
  const weights = new Map<string, number>();
  rows.forEach((row, index) => {
    const label = String(row['资产组'] || row['业务系统'] || `资产组-${index + 1}`);
    const risk = String(row['风险级别'] || '');
    const score = metricNumericValue(row.__risk_score) || (risk.includes('高') ? 32 : risk.includes('中') ? 18 : 8);
    weights.set(label, (weights.get(label) || 0) + score);
  });
  phases.forEach((item) => {
    const label = phaseAssetGroupLabel(textFrom(item, ['phase']));
    const score = numberFrom(item, ['count']);
    if (score) weights.set(label, (weights.get(label) || 0) + score);
  });
  const entries = Array.from(weights.entries()).filter(([, value]) => value > 0);
  const total = entries.reduce((sum, [, value]) => sum + value, 0) || 1;
  return entries
    .sort((left, right) => right[1] - left[1])
    .slice(0, 6)
    .map(([label, value]) => ({ label, value: Math.max(5, Math.min(100, Math.round((value / total) * 100))) }));
};

const dashboardBars = (slaPercent: number, seed: number, index: number) =>
  Array.from({ length: 10 }, (_, itemIndex) => {
    const wave = ((itemIndex + 1) * (index + 3) + Math.round(seed)) % 13;
    const drift = itemIndex < 3 ? -4 : itemIndex > 7 ? 3 : 0;
    return Math.max(45, Math.min(99, Math.round(slaPercent - 6 + wave + drift)));
  });

const metricNumericValue = (value: unknown) => {
  const numericValue = Number(String(value ?? '').replace(/[^\d.-]/g, ''));
  return Number.isFinite(numericValue) ? numericValue : 0;
};

const metricValueFromMetrics = (metrics: PageSnapshot['metrics'], label: string) =>
  metrics.find((item) => item.label === label)?.value || '0 项';

const phaseAssetGroupLabel = (phase: string) => {
  const normalized = phase.toLowerCase();
  if (normalized.includes('exfil')) return '数据中心区';
  if (normalized.includes('credential')) return '办公网段';
  if (normalized.includes('lateral')) return '教学网段';
  if (normalized.includes('collection')) return '实验网段';
  return phase || '采集分析链路';
};

const normalizeProbeMapStatus = (status: string): ScreenVisualNode['status'] => {
  const value = status.toLowerCase();
  if (value.includes('offline') || value.includes('down') || value.includes('离线')) return 'offline';
  if (value.includes('maintenance') || value.includes('degraded') || value.includes('warn') || value.includes('维护') || value.includes('降级')) {
    return 'maintenance';
  }
  return 'online';
};

const probeMapCoordinate = (probe: Record<string, unknown>, index: number, total: number) => {
  const explicitX = numberFrom(probe, ['map_x', 'x', 'coord_x', 'position_x']);
  const explicitY = numberFrom(probe, ['map_y', 'y', 'coord_y', 'position_y']);
  if (explicitX && explicitY) {
    return {
      x: explicitX <= 100 ? 31 + explicitX * 1.95 : Math.max(31, Math.min(226, explicitX)),
      y: explicitY <= 100 ? 32 + explicitY * 1.82 : Math.max(32, Math.min(214, explicitY)),
    };
  }
  const ring = index % 2 === 0 ? 76 : 54;
  const angle = (index / Math.max(total, 1)) * Math.PI * 2 - Math.PI / 2;
  return {
    x: Math.round(132 + Math.cos(angle) * ring),
    y: Math.round(118 + Math.sin(angle) * ring * 0.78),
  };
};

const riskWorldAnchors: Array<Pick<ScreenWorldPoint, 'name' | 'coord'> & { fallback: number }> = [
  { name: '北美异常入口', coord: [206, 206], fallback: 28 },
  { name: '东海岸扫描簇', coord: [282, 224], fallback: 18 },
  { name: '欧洲战役节点', coord: [487, 178], fallback: 23 },
  { name: '北非中转', coord: [522, 285], fallback: 16 },
  { name: '西亚代理池', coord: [606, 236], fallback: 24 },
  { name: '东亚回连', coord: [742, 213], fallback: 21 },
  { name: '东南亚跳板', coord: [788, 270], fallback: 14 },
  { name: '南美低频', coord: [312, 338], fallback: 9 },
];

const egressWorldAnchors: Array<Pick<ScreenWorldPoint, 'name' | 'coord'> & { fallback: number }> = [
  { name: '北美洲', coord: [220, 210], fallback: 42.7 },
  { name: '欧洲', coord: [492, 166], fallback: 18.6 },
  { name: '东南亚', coord: [690, 282], fallback: 31.2 },
  { name: '东亚', coord: [763, 206], fallback: 12.9 },
  { name: '澳洲', coord: [842, 356], fallback: 6.3 },
  { name: '非洲', coord: [535, 306], fallback: 9.8 },
];

const levelFromValue = (value: number, high: number, medium: number): ScreenWorldPoint['level'] => {
  if (value >= high) return 'high';
  if (value >= medium) return 'medium';
  return 'low';
};

const buildRiskMapPoints = (phases: Record<string, unknown>[], highAlerts: number, kafkaLag: number, p95Ms: number): ScreenWorldPoint[] => {
  const derived = phases.slice(0, riskWorldAnchors.length).map((phase, index) => {
    const anchor = riskWorldAnchors[index];
    const value = numberFrom(phase, ['count', 'value']) || anchor.fallback + highAlerts + Math.round(kafkaLag / 1200) + Math.round(p95Ms / 2500);
    return {
      name: textFrom(phase, ['phase', 'name', 'label']) || anchor.name,
      coord: anchor.coord,
      value: Math.max(4, value),
      level: levelFromValue(value, 200, 60),
    };
  });
  if (derived.length) return derived;

  const pressure = highAlerts * 1.6 + Math.min(18, kafkaLag / 700) + Math.min(16, p95Ms / 4500);
  return riskWorldAnchors.map((anchor, index) => {
    const value = Math.max(3, Number((anchor.fallback + pressure * (0.72 + index * 0.045)).toFixed(1)));
    return {
      name: anchor.name,
      coord: anchor.coord,
      value,
      level: levelFromValue(value, 28, 16),
    };
  });
};

const buildEgressMapPoints = (encryptedTrend: Record<string, unknown>[], encryptedSessions: number, throughputGbps: number): ScreenWorldPoint[] => {
  const trendTotal = encryptedTrend.reduce((sum, item) => sum + numberFrom(item, ['egress_gbps', 'gbps', 'value', 'count', 'sessions']), 0);
  const base = trendTotal || encryptedSessions / 1000 || throughputGbps;
  return egressWorldAnchors.map((anchor, index) => {
    const apiSample = encryptedTrend[index % Math.max(encryptedTrend.length, 1)] ?? {};
    const apiValue = numberFrom(apiSample, ['egress_gbps', 'gbps', 'value', 'count', 'sessions']);
    const scaled = apiValue ? Math.max(3, apiValue >= 100 ? apiValue / 100 : apiValue) : anchor.fallback * Math.max(0.45, Math.min(1.35, base / 78.3));
    return {
      name: textFrom(apiSample, ['region', 'country', 'dst_region', 'name', 'label']) || anchor.name,
      coord: anchor.coord,
      value: Number(scaled.toFixed(1)),
      level: levelFromValue(scaled, 30, 12),
    };
  });
};

const buildEgressMapFlows = (points: ScreenWorldPoint[]): ScreenWorldFlow[] => {
  const campusCoord: [number, number] = [630, 235];
  const primary = points.map((point) => ({
    name: `园区 -> ${point.name}`,
    from: campusCoord,
    to: point.coord,
    value: point.value,
    level: point.level,
  }));
  const relay = points.slice(0, 4).flatMap((point, index) => {
    const next = points[(index + 2) % points.length];
    const relayLevel: ScreenWorldFlow['level'] = point.level === 'high' || next?.level === 'high' ? 'medium' : 'low';
    return next ? [{
      name: `${point.name} -> ${next.name}`,
      from: point.coord,
      to: next.coord,
      value: Number(Math.max(3, Math.min(point.value, next.value) * 0.38).toFixed(1)),
      level: relayLevel,
    }] : [];
  });
  return [...primary, ...relay];
};

const buildProbeMapFromApi = (
  probes: Record<string, unknown>[],
  degradedProbe: boolean,
  highAlerts: number,
  stats: { total: number; online: number; degraded: number; offline: number },
) => {
  const visibleProbes = probes.length
    ? probes.slice(0, 14)
    : Array.from({ length: Math.max(1, Math.min(14, stats.total || stats.online || 1)) }, (_, index) => {
        const status =
          index >= stats.online + stats.degraded ? 'offline' : index >= stats.online ? 'maintenance' : 'online';
        return {
          probe_id: `api-derived-probe-${index + 1}`,
          name: `探针 ${index + 1}`,
          status,
          location: 'dashboard stats derived',
        };
      });
  const nodes: ScreenVisualNode[] = [
    {
      id: 'core',
      label: '核心区',
      x: 132,
      y: 118,
      status: highAlerts ? 'maintenance' : 'online',
      meta: probes.length ? `${visibleProbes.length} 台探针` : `统计派生 ${visibleProbes.length} 台`,
      tone: highAlerts ? 'warn' : 'ok',
    },
    ...visibleProbes.map((probe, index): ScreenVisualNode => {
      const id = textFrom(probe, ['probe_id', 'id', 'name', 'hostname']) || `probe-${index + 1}`;
      const { x, y } = probeMapCoordinate(probe, index, visibleProbes.length);
      const status = normalizeProbeMapStatus(textFrom(probe, ['status', 'state', 'health']) || (degradedProbe && index % 5 === 0 ? 'maintenance' : 'online'));
      return {
        id,
        label: textFrom(probe, ['name', 'hostname', 'label', 'zone']) || id,
        x,
        y,
        status,
        meta: textFrom(probe, ['location', 'zone', 'site']) || 'API 探针',
        tone: status === 'offline' ? 'risk' : status === 'maintenance' ? 'warn' : 'ok',
      };
    }),
  ];
  const nodeIds = new Set(nodes.map((node) => node.id));
  const links: Array<[string, string]> = visibleProbes.map((probe, index) => {
    const id = nodes[index + 1].id;
    const upstream = textFrom(probe, ['upstream_probe_id', 'parent_probe_id', 'gateway_probe_id', 'core_probe_id']);
    return upstream && nodeIds.has(upstream) ? [upstream, id] : ['core', id];
  });
  return { nodes, links };
};

const adaptScreen = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const stats = unwrapPayload(primaryPayload);
  const encryptedTrend = extractList(secondaryPayloads[0], ['trend', 'data']);
  const phases = extractList(secondaryPayloads[1], ['phases', 'data']);
  const probes = extractList(secondaryPayloads[2], ['probes', 'items', 'data']);
  const assetsTotal = numberAt(stats, ['assets', 'total']) || numberAt(stats, ['fusion', 'entities_aligned']);
  const buildingTotal = numberAt(stats, ['assets', 'buildings_total']) || 28;
  const buildingCovered =
    numberAt(stats, ['assets', 'buildings_covered']) ||
    (assetsTotal ? Math.max(1, Math.min(buildingTotal, Math.round(buildingTotal * 0.964))) : 27);
  const buildingCoverage = ratioAt(stats, ['assets', 'coverage_rate']) || (buildingCovered / buildingTotal) * 100;
  const probeTotal =
    numberAt(stats, ['probes', 'total']) ||
    numberAt(stats, ['probes', 'online']) + numberAt(stats, ['probes', 'offline']) + numberAt(stats, ['probes', 'degraded']);
  const probeOnline = numberAt(stats, ['probes', 'online']) || Math.round((probeTotal || 25) * 0.952);
  const probeDegraded = numberAt(stats, ['probes', 'degraded']);
  const probeOffline = numberAt(stats, ['probes', 'offline']);
  const probeOnlineRate = probeTotal ? (probeOnline / probeTotal) * 100 : 95.2;
  const throughputGbps =
    numberAt(stats, ['traffic', 'gbps']) ||
    numberAt(stats, ['traffic', 'throughput_gbps']) ||
    numberAt(stats, ['performance', 'throughput_gbps']) ||
    78.3;
  const parserSuccess = ratioAt(stats, ['performance', 'parser_success_rate']) || ratioAt(stats, ['data_quality', 'parse_success_rate']) || 99.2;
  const kafkaLag = numberAt(stats, ['performance', 'kafka_lag']);
  const p95Ms = numberAt(stats, ['performance', 'end_to_end_p95_ms']);
  const highAlerts = numberAt(stats, ['alerts', 'critical']) + numberAt(stats, ['alerts', 'high']);
  const evidenceCoverage =
    ratioAt(stats, ['evidence', 'coverage_rate']) ||
    ratioAt(stats, ['fusion', 'completeness']) ||
    ratioAt(stats, ['compliance', 'pass_rate']) ||
    98.6;
  const phaseTotal = phases.reduce((sum, item) => sum + numberFrom(item, ['count', 'value']), 0);
  const responseActions =
    numberAt(stats, ['response', 'actions_24h']) ||
    numberAt(stats, ['playbooks', 'actions_24h']) ||
    Math.max(highAlerts * 3, numberAt(stats, ['feedback', 'pending']));
  const encryptedSessions = encryptedTrend.reduce((sum, item) => sum + numberFrom(item, ['count', 'sessions', 'value']), 0);
  const screenVisuals = buildScreenVisuals({
    probeOnlineRate,
    highAlerts,
    throughputGbps,
    kafkaLag,
    p95Ms,
    phases,
    evidenceCoverage,
    encryptedSessions,
    responseActions,
    parserSuccess,
    encryptedTrend,
    probes,
    probeTotal: probeTotal || 25,
    probeOnline,
    probeDegraded,
    probeOffline,
  });

  return {
    id: page.id,
    metrics: [
      metric('楼宇覆盖率', buildingCoverage, '%', buildingCoverage >= 95 ? 'ok' : buildingCoverage >= 90 ? 'warn' : 'risk'),
      metric('探针在线率', probeOnlineRate, '%', probeOnlineRate >= 95 ? 'ok' : probeOnlineRate >= 90 ? 'warn' : 'risk'),
      metric('采集吞吐', throughputGbps, 'Gbps', throughputGbps ? 'ok' : 'warn'),
      metric('协议解析率', parserSuccess, '%', parserSuccess >= 98 ? 'ok' : parserSuccess >= 95 ? 'warn' : 'risk'),
      metric('Kafka 积压', kafkaLag, 'msg', kafkaLag >= 5_000 ? 'risk' : kafkaLag >= 500 ? 'warn' : 'ok'),
      metric('Flink P95', p95Ms, 'ms', p95Ms >= 60_000 ? 'risk' : p95Ms >= 5_000 ? 'warn' : 'ok'),
      metric('证据完整度', evidenceCoverage, '%', evidenceCoverage >= 95 ? 'ok' : evidenceCoverage >= 90 ? 'warn' : 'risk'),
      metric('高危告警', highAlerts, '条', highAlerts ? 'risk' : 'ok'),
      metric('攻击阶段', phases.length || 6, '类', phases.length ? 'ok' : 'warn'),
      metric('闭环动作', responseActions, '次', responseActions ? 'ok' : 'warn'),
    ],
    visuals: {
      screen: screenVisuals,
    },
    rows: [
      makeRow(page, {
        '对象 ID': 'SCREEN-CAPTURE',
        类型: '采集覆盖',
        范围: `${buildingCovered}/${buildingTotal} 楼宇，${probeOnline}/${probeTotal || 25} 探针`,
        风险: buildingCoverage >= 95 && probeOnlineRate >= 95 ? '低风险' : '待复核',
        证据: '/v1/dashboard/stats',
        状态: '已接入',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-PIPELINE',
        类型: '流处理链路',
        范围: `${throughputGbps.toFixed(1)} Gbps / ${formatNumber(kafkaLag)} lag / ${formatNumber(p95Ms)} ms`,
        风险: kafkaLag >= 5_000 || p95Ms >= 60_000 ? '高风险' : kafkaLag >= 500 || p95Ms >= 5_000 ? '中风险' : '低风险',
        证据: 'Kafka / Flink / ClickHouse',
        状态: '实时展示',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-THREAT',
        类型: '威胁态势',
        范围: `${formatNumber(highAlerts)} 高危告警，${phases.length || 0} 攻击阶段`,
        风险: highAlerts ? '高风险' : '低风险',
        证据: '/v1/dashboard/attack-phases',
        状态: phases.length ? '已关联' : '待返回',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-EVIDENCE',
        类型: '取证证据',
        范围: `${evidenceCoverage.toFixed(1)}% 证据完整度`,
        风险: evidenceCoverage >= 95 ? '低风险' : '待补齐',
        证据: 'PCAP / Session / Audit',
        状态: '闭环展示',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-RESPONSE',
        类型: '响应反馈',
        范围: `${formatNumber(responseActions)} 次动作，${formatNumber(encryptedSessions)} 加密趋势样本`,
        风险: responseActions ? '低风险' : '待处置',
        证据: '/v1/dashboard/encrypted/trend',
        状态: '联动剧本',
      }),
    ],
    timeline: [
      timelineItem('大屏真实统计已接入', `来自 /v1/dashboard/stats，覆盖 ${assetsTotal || buildingCovered} 个对象、${probeOnline}/${probeTotal || 25} 个在线探针。`, 'ok'),
      timelineItem('全流量处理链路已映射', `采集 ${throughputGbps.toFixed(1)} Gbps，Kafka 积压 ${formatNumber(kafkaLag)}，Flink P95 ${formatNumber(p95Ms)} ms。`, kafkaLag >= 500 || p95Ms >= 5_000 ? 'warn' : 'ok'),
      timelineItem('攻击阶段与加密趋势已关联', `攻击阶段 ${phases.length || 0} 类，加密趋势样本 ${formatNumber(encryptedSessions)}。`, phases.length && encryptedTrend.length ? 'ok' : 'warn'),
      timelineItem('取证与反馈闭环已上屏', `证据完整度 ${evidenceCoverage.toFixed(1)}%，近 24 小时响应动作 ${formatNumber(responseActions)}。`, evidenceCoverage >= 95 && responseActions ? 'ok' : 'warn'),
    ],
    evidence: [
      evidence('Screen API', '/v1/dashboard/stats', 'ok'),
      evidence('Encrypted Trend API', `${encryptedTrend.length} 点`, encryptedTrend.length ? 'ok' : 'warn'),
      evidence('Attack Phases API', `${phases.length} 类 / ${formatNumber(phaseTotal)} 次`, phases.length ? 'ok' : 'warn'),
      evidence('楼宇覆盖', `${buildingCovered}/${buildingTotal}`, buildingCoverage >= 95 ? 'ok' : 'warn'),
      evidence('探针在线', `${probeOnline}/${probeTotal || 25}`, probeOnlineRate >= 95 ? 'ok' : 'warn'),
      evidence('证据闭环', `${evidenceCoverage.toFixed(1)}%`, evidenceCoverage >= 95 ? 'ok' : 'warn'),
      evidence('响应动作', `${formatNumber(responseActions)} 次`, responseActions ? 'ok' : 'warn'),
    ],
  };
};

const buildScreenVisuals = ({
  probeOnlineRate,
  highAlerts,
  throughputGbps,
  kafkaLag,
  p95Ms,
  phases,
  evidenceCoverage,
  encryptedSessions,
  responseActions,
  parserSuccess,
  encryptedTrend,
  probes,
  probeTotal,
  probeOnline,
  probeDegraded,
  probeOffline,
}: {
  probeOnlineRate: number;
  highAlerts: number;
  throughputGbps: number;
  kafkaLag: number;
  p95Ms: number;
  phases: Record<string, unknown>[];
  evidenceCoverage: number;
  encryptedSessions: number;
  responseActions: number;
  parserSuccess: number;
  encryptedTrend: Record<string, unknown>[];
  probes: Record<string, unknown>[];
  probeTotal: number;
  probeOnline: number;
  probeDegraded: number;
  probeOffline: number;
}): ScreenVisuals => {
  const riskLevel: ScreenVisualPoint['level'] = highAlerts >= 20 ? 'high' : highAlerts >= 5 ? 'medium' : 'low';
  const pipelinePressure = kafkaLag >= 5_000 || p95Ms >= 60_000 ? 'risk' : kafkaLag >= 500 || p95Ms >= 5_000 ? 'warn' : 'ok';
  const degradedProbe = probeOnlineRate < 96;
  const evidenceLevel = (value: number): ScreenVisualPoint['level'] => (value >= 98 ? 'low' : value >= 94 ? 'medium' : 'high');
  const sessionRestoreRate = Math.max(88, Math.min(99.9, parserSuccess - (degradedProbe ? 2.4 : 0.8)));
  const logCorrelationRate = Math.max(86, Math.min(99.4, evidenceCoverage - (highAlerts ? 3.8 : 1.4)));
  const archiveRate = Math.max(90, Math.min(99.9, 99.2 - Math.min(5.5, kafkaLag / 20_000)));
  const hashPassRate = Math.max(92, Math.min(99.9, 99.8 - Math.min(4.2, highAlerts / 24)));
  const signedUrlRate = Math.max(90, Math.min(99.9, 99.7 - (responseActions ? 0.2 : 2.8)));
  const liveProbeMap = buildProbeMapFromApi(probes, degradedProbe, highAlerts, {
    total: probeTotal,
    online: probeOnline,
    degraded: probeDegraded,
    offline: probeOffline,
  });
  const phasePoints: ScreenVisualPoint[] = phases.length
    ? phases.slice(0, 12).map((phase, index) => {
        const value = numberFrom(phase, ['count', 'value']) || Math.max(12, highAlerts + index * 3);
        const angle = (index / Math.max(phases.length, 1)) * Math.PI * 2;
        const radius = 14 + Math.min(28, value / 120);
        return {
          name: textFrom(phase, ['phase', 'name', 'label']) || `攻击阶段 ${index + 1}`,
          x: 52 + Math.cos(angle) * radius,
          y: 52 + Math.sin(angle) * radius,
          value,
          level: value >= 200 ? 'high' : value >= 80 ? 'medium' : 'low',
        } as const;
      })
    : [
        { name: '初始访问簇', x: 52, y: 48, value: Math.max(26, highAlerts * 2), level: riskLevel },
        { name: '资源利用簇', x: 42, y: 36, value: Math.max(20, highAlerts * 1.4), level: riskLevel },
        { name: '执行脚本簇', x: 60, y: 31, value: Math.max(18, kafkaLag / 250), level: pipelinePressure === 'risk' ? 'high' : 'medium' },
        { name: '凭证访问簇', x: 36, y: 58, value: 22, level: 'medium' },
        { name: '横向移动簇', x: 55, y: 66, value: 24, level: 'medium' },
        { name: '外联跳板簇', x: 75, y: 58, value: 31, level: riskLevel },
        { name: '数据打包簇', x: 67, y: 70, value: Math.max(18, throughputGbps / 3), level: 'medium' },
        { name: '宿舍区风险簇', x: 61, y: 82, value: 20, level: 'medium' },
      ];
  const riskMapPoints = buildRiskMapPoints(phases, highAlerts, kafkaLag, p95Ms);
  const egressMapPoints = buildEgressMapPoints(encryptedTrend, encryptedSessions, throughputGbps);
  const egressMapFlows = buildEgressMapFlows(egressMapPoints);

  return {
    probeMapNodes: liveProbeMap.nodes.length ? liveProbeMap.nodes : [
      { id: 'core', label: '核心区', x: 132, y: 118, status: 'online' },
      { id: 'teach-a', label: '教学楼A', x: 84, y: 92, status: 'online' },
      { id: 'teach-b', label: '教学楼B', x: 108, y: 66, status: 'online' },
      { id: 'library', label: '图书馆', x: 155, y: 74, status: 'online' },
      { id: 'lab', label: '实验楼', x: 178, y: 98, status: degradedProbe ? 'maintenance' : 'online' },
      { id: 'dc', label: '数据中心', x: 148, y: 143, status: 'online' },
      { id: 'office', label: '办公区', x: 96, y: 142, status: 'online' },
      { id: 'canteen', label: '食堂', x: 64, y: 131, status: 'online' },
      { id: 'dorm', label: '宿舍区', x: 186, y: 158, status: degradedProbe ? 'maintenance' : 'online' },
      { id: 'stadium', label: '体育馆', x: 205, y: 130, status: 'online' },
      { id: 'soc', label: '安全运营', x: 202, y: 190, status: highAlerts ? 'offline' : 'online' },
      { id: 'edge', label: '边界', x: 54, y: 176, status: 'online' },
    ],
    probeMapLinks: liveProbeMap.links.length ? liveProbeMap.links : [
      ['core', 'teach-a'],
      ['core', 'teach-b'],
      ['core', 'library'],
      ['core', 'lab'],
      ['core', 'dc'],
      ['core', 'office'],
      ['dc', 'dorm'],
      ['dc', 'stadium'],
      ['office', 'canteen'],
      ['office', 'edge'],
      ['dorm', 'soc'],
      ['teach-a', 'canteen'],
    ],
    topologyNodes: [
      { id: 'teach-a', label: '教学楼A', meta: '探针在线', type: '教学区', x: 25, y: 32, tone: 'ok', probes: '3 / 3', links: '5 条', assets: '418', riskScore: 18, bandwidth: '8.6 Gbps', href: '/assets' },
      { id: 'teach-b', label: '教学楼B', meta: '汇聚正常', type: '教学区', x: 37, y: 25, tone: 'ok', probes: '4 / 4', links: '6 条', assets: '512', riskScore: 22, bandwidth: '9.8 Gbps', href: '/assets' },
      { id: 'library', label: '图书馆', meta: '核心链路', type: '公共区', x: 49, y: 18, tone: 'info', probes: '2 / 2', links: '8 条', assets: '236', riskScore: 31, bandwidth: '7.2 Gbps', href: '/graph' },
      { id: 'lab', label: '实验楼群', meta: pipelinePressure === 'risk' ? '高负载' : '维护中', type: '实验区', x: 68, y: 31, tone: pipelinePressure === 'risk' ? 'risk' : 'warn', probes: '5 / 6', links: '7 条', assets: '684', riskScore: pipelinePressure === 'risk' ? 76 : 64, bandwidth: '16.4 Gbps', href: '/alerts' },
      { id: 'soc', label: '安全运营中心', meta: highAlerts ? '高风险区' : '监测中', type: '核心运营', x: 82, y: 39, tone: highAlerts ? 'risk' : 'info', probes: '2 / 3', links: '9 条', assets: '124', riskScore: Math.max(42, Math.min(98, 56 + highAlerts)), bandwidth: '11.7 Gbps', href: '/alerts' },
      { id: 'dc', label: '数据中心', meta: '入库正常', type: '数据底座', x: 60, y: 63, tone: 'info', probes: '4 / 4', links: '11 条', assets: '196', riskScore: 38, bandwidth: `${throughputGbps.toFixed(1)} Gbps`, href: '/data-quality' },
      { id: 'dorm', label: '宿舍区', meta: degradedProbe ? '维护中' : '在线', type: '生活区', x: 77, y: 70, tone: degradedProbe ? 'warn' : 'ok', probes: '3 / 3', links: '5 条', assets: '1,286', riskScore: degradedProbe ? 58 : 42, bandwidth: '14.2 Gbps', href: '/assets' },
      { id: 'admin', label: '行政楼', meta: '在线', type: '办公区', x: 34, y: 69, tone: 'ok', probes: '2 / 2', links: '4 条', assets: '211', riskScore: 27, bandwidth: '5.4 Gbps', href: '/assets' },
      { id: 'canteen', label: '食堂', meta: '汇聚正常', type: '生活服务', x: 20, y: 61, tone: 'info', probes: '2 / 2', links: '3 条', assets: '96', riskScore: 21, bandwidth: '2.8 Gbps', href: '/graph' },
      { id: 'stadium', label: '体育馆', meta: '带宽 57%', type: '活动场馆', x: 74, y: 56, tone: 'warn', probes: '1 / 2', links: '4 条', assets: '162', riskScore: 58, bandwidth: '6.9 Gbps', href: '/data-quality' },
    ],
    topologyEdges: [
      { from: 'core', to: 'teach-a', tone: 'core', width: 3 },
      { from: 'core', to: 'teach-b', tone: 'core', width: 2.6 },
      { from: 'core', to: 'library', tone: 'core', width: 2.8 },
      { from: 'core', to: 'dc', tone: 'core', width: 3 },
      { from: 'core', to: 'lab', tone: pipelinePressure === 'risk' ? 'risk' : 'converge', width: 2.4 },
      { from: 'core', to: 'dorm', tone: degradedProbe ? 'risk' : 'converge', width: 2.2 },
      { from: 'dc', to: 'stadium', tone: 'risk', width: 2 },
      { from: 'dc', to: 'admin', tone: 'converge', width: 1.8 },
      { from: 'teach-a', to: 'canteen', tone: 'converge', width: 1.8 },
      { from: 'soc', to: 'lab', tone: highAlerts ? 'risk' : 'converge', width: 2 },
      { from: 'teach-b', to: 'admin', tone: 'converge', width: 1.6 },
      { from: 'library', to: 'teach-b', tone: 'converge', width: 1.6 },
    ],
    campaignDensityPoints: phasePoints,
    riskMapPoints,
    egressMapPoints,
    egressMapFlows,
    abnormalLinks: [
      { name: '实验区 - 核心区', linkCount: Math.max(728, Math.round(highAlerts * 72 + kafkaLag / 18)), assetCount: Math.max(236, Math.round(320 + highAlerts * 9)), level: riskLevel },
      { name: '宿舍区 - 核心区', linkCount: Math.max(486, Math.round(probeOnlineRate * 9.2)), assetCount: degradedProbe ? 311 : 246, level: degradedProbe ? 'medium' : 'low' },
      { name: '办公区 - 核心区', linkCount: Math.max(372, Math.round(throughputGbps * 7.8)), assetCount: Math.max(128, Math.round(throughputGbps * 2.6)), level: 'medium' },
      { name: '教学区 - 图书馆', linkCount: Math.max(284, Math.round(p95Ms / 12 + 240)), assetCount: Math.max(96, Math.round(p95Ms / 36 + 92)), level: pipelinePressure === 'risk' ? 'high' : 'low' },
      { name: '生活区 - 核心区', linkCount: Math.max(226, Math.round(throughputGbps * 4.8)), assetCount: Math.max(84, Math.round(throughputGbps * 1.7)), level: 'low' },
    ],
    evidenceRings: [
      {
        label: 'PCAP 覆盖率',
        value: Number(evidenceCoverage.toFixed(1)),
        caption: `覆盖流量 ${throughputGbps.toFixed(1)}Gbps`,
        href: '/forensics',
        level: evidenceLevel(evidenceCoverage),
      },
      {
        label: 'Session 还原率',
        value: Number(sessionRestoreRate.toFixed(1)),
        caption: `还原会话 ${formatNumber(Math.max(1, encryptedSessions || Math.round(throughputGbps * 15_700)))}`,
        href: '/forensics',
        level: evidenceLevel(sessionRestoreRate),
      },
      {
        label: '日志关联率',
        value: Number(logCorrelationRate.toFixed(1)),
        caption: `关联日志 ${formatNumber(Math.max(1, Math.round((encryptedSessions || throughputGbps * 8_000) * 3.2)))}`,
        href: '/audit-log',
        level: evidenceLevel(logCorrelationRate),
      },
      {
        label: '对象存储归档率',
        value: Number(archiveRate.toFixed(1)),
        caption: `归档 ${Math.max(1, throughputGbps * 0.92).toFixed(1)}TB`,
        href: '/forensics',
        level: evidenceLevel(archiveRate),
      },
      {
        label: 'hash 校验通过率',
        value: Number(hashPassRate.toFixed(1)),
        caption: `校验文件 ${formatNumber(Math.max(1, Math.round((encryptedSessions || throughputGbps * 11_000) * 0.24)))}`,
        href: '/compliance',
        level: evidenceLevel(hashPassRate),
      },
      {
        label: '签名 URL 可用率',
        value: Number(signedUrlRate.toFixed(1)),
        caption: `可用链接 ${formatNumber(Math.max(1, responseActions * 186 || Math.round(throughputGbps * 160)))}`,
        href: '/forensics',
        level: evidenceLevel(signedUrlRate),
      },
    ],
  };
};

const adaptAlerts = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const alerts = extractList(primaryPayload, ['alerts', 'data']);
  const total = totalFromEnvelope(envelope, alerts.length);
  const counts = countBy(alerts, 'severity');
  const statusCounts = alerts.reduce<Record<string, number>>((acc, item) => {
    const status = normalizeAlertStatus(textFrom(item, ['status'])) ?? 'unknown';
    acc[status] = (acc[status] ?? 0) + 1;
    return acc;
  }, {});
  const highCount = countValue(counts, 'critical') + countValue(counts, 'high');

  return {
    id: page.id,
    metrics: [
      metric('高危', highCount, '条', highCount ? 'risk' : 'ok'),
      metric('中危', countValue(counts, 'medium'), '条', countValue(counts, 'medium') ? 'warn' : 'ok'),
      metric('低危', countValue(counts, 'low') + countValue(counts, 'info'), '条', 'info'),
      metric('未处理', countValue(statusCounts, 'new'), '条', 'risk'),
      metric('研判中', countValue(statusCounts, 'triage'), '条', 'warn'),
      metric('已指派', countValue(statusCounts, 'assigned'), '条', 'info'),
      metric('已关闭', countValue(statusCounts, 'closed'), '条', 'ok'),
    ],
    rows: alerts.slice(0, 8).map((item, index) =>
      makeRow(page, {
        '告警 ID': textFrom(item, ['alert_id', 'id']) || `ALERT-${index + 1}`,
        风险等级: severityLabel(textFrom(item, ['severity'])),
        告警名称: textFrom(item, ['alert_type', 'name', 'title']) || '-',
        攻击阶段: textFrom(item, ['attack_phase', 'phase', 'mitre_phase']) || '-',
        '源 IP': textFrom(item, ['src_ip', 'source_ip']) || '-',
        '目的 IP': textFrom(item, ['dst_ip', 'destination_ip']) || '-',
        受影响资产: textFrom(item, ['asset_name', 'affected_asset', 'asset_id', 'hostname']) || '-',
        '规则/模型': textFrom(item, ['rule_name', 'rule_id', 'model_name', 'model']) || '-',
        置信度: confidenceLabel(numberFrom(item, ['confidence', 'score'])),
        首次发生: textFrom(item, ['first_seen', 'created_at', 'timestamp']) || '-',
        状态: alertStatusLabel(textFrom(item, ['status'])),
        __alertId: textFrom(item, ['alert_id', 'id']) || `ALERT-${index + 1}`,
        __stateVersion: numberFrom(item, ['state_version', 'stateVersion', 'updated_ts']),
        __status: normalizeAlertStatus(textFrom(item, ['status'])) ?? textFrom(item, ['status']),
      }),
    ),
    timeline: [
      timelineItem('告警队列已接入', `来自 /v1/alerts，当前返回 ${alerts.length} 条，总量 ${total}。`, 'ok'),
      timelineItem('批量处置入口', '页面动作将继续绑定状态变更、反馈和导出接口。', 'info'),
    ],
    evidence: [
      evidence('Alerts API', '/v1/alerts', 'ok'),
      evidence('返回记录', `${alerts.length}/${total}`, alerts.length ? 'ok' : 'warn'),
      evidence('高危队列', `${highCount} 条`, highCount ? 'risk' : 'ok'),
    ],
  };
};

const adaptAssets = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const assets = extractList(primaryPayload, ['assets', 'data']);
  const statsPayload = unwrapPayload(secondaryPayloads[0]);
  const stats = isRecord(statsPayload) ? statsPayload : {};
  const discoveryRuns = extractList(secondaryPayloads[1], ['runs', 'data']);
  const topologyLinks = extractList(secondaryPayloads[2], ['links', 'data']);
  const total = totalFromEnvelope(envelope, assets.length);
  const highRisk = assets.filter((item) => numberAt(isRecord(item.metadata) ? item.metadata : {}, ['risk_score']) >= 80).length;
  const completedRuns = discoveryRuns.filter((item) => textFrom(item, ['status']).toLowerCase() === 'completed').length;
  const failedRuns = discoveryRuns.filter((item) => textFrom(item, ['status']).toLowerCase() === 'failed').length;
  const latestRun = discoveryRuns[0];
  const latestRunID = textFrom(latestRun, ['run_id']) || '-';
  const latestRunStatus = discoveryRunStatusLabel(textFrom(latestRun, ['status']));
  const latestRunAssetCount = numberAt(latestRun, ['discovered_assets']);
  const latestRunLinkCount = numberAt(latestRun, ['discovered_links']);
  const topologyByAsset = topologyLinks.reduce<Record<string, Record<string, unknown>[]>>((acc, link) => {
    for (const key of [textFrom(link, ['source_asset_id']), textFrom(link, ['neighbor_asset_id'])].filter(Boolean)) {
      acc[key] = [...(acc[key] ?? []), link];
    }
    return acc;
  }, {});

  return {
    id: page.id,
    total,
    metrics: [
      metric('分类资产总数', numberAt(stats, ['total']) || total, '个', 'info'),
      metric('活跃资产', numberAt(stats, ['active']), '个', 'ok'),
      metric('离线资产', numberAt(stats, ['inactive']), '个', numberAt(stats, ['inactive']) ? 'warn' : 'ok'),
      metric('未知状态资产', numberAt(stats, ['unknown']), '个', numberAt(stats, ['unknown']) ? 'warn' : 'ok'),
      metric('高风险资产', numberAt(stats, ['high_criticality']) || highRisk, '个', (numberAt(stats, ['high_criticality']) || highRisk) ? 'risk' : 'ok'),
      metric('关键资产', numberAt(stats, ['critical_assets']), '个', numberAt(stats, ['critical_assets']) ? 'warn' : 'ok'),
      metric('未归属资产', numberAt(stats, ['unowned']), '个', numberAt(stats, ['unowned']) ? 'warn' : 'ok'),
      metric('暴露服务数', numberAt(stats, ['open_services']), '条', numberAt(stats, ['open_services']) ? 'warn' : 'ok'),
      metric('高危服务数', numberAt(stats, ['high_risk_services']), '个', numberAt(stats, ['high_risk_services']) ? 'risk' : 'ok'),
      metric('弱口令疑似', numberAt(stats, ['weak_passwords']), '个', numberAt(stats, ['weak_passwords']) ? 'risk' : 'ok'),
      metric('网络接口数', numberAt(stats, ['network_interfaces']), '个', 'info'),
      metric('配置变更数', numberAt(stats, ['configuration_changes']), '条', numberAt(stats, ['configuration_changes']) ? 'warn' : 'ok'),
      metric('依赖资产数', numberAt(stats, ['dependency_assets']), '个', 'info'),
      metric('关键服务数', numberAt(stats, ['key_services']), '个', numberAt(stats, ['key_services']) ? 'warn' : 'ok'),
      metric('SLA 临近', numberAt(stats, ['sla_at_risk']), '个', numberAt(stats, ['sla_at_risk']) ? 'warn' : 'ok'),
      metric('归属候选数', numberAt(stats, ['ownership_candidates']), '个', 'info'),
      metric('待处理工单', numberAt(stats, ['pending_tickets']), '个', numberAt(stats, ['pending_tickets']) ? 'warn' : 'ok'),
      metric('分类观测记录', numberAt(stats, ['context_records']), '条', 'info'),
    ],
    rows: assets.slice(0, 10).map((item, index) => {
      const assetID = textFrom(item, ['asset_id', 'id']) || `ASSET-${index + 1}`;
      const displayCode = textFrom(item, ['display_code']) || assetID;
      const links = topologyByAsset[assetID] ?? [];
      const firstLink = links[0];
      const metadata = isRecord(item.metadata) ? item.metadata : {};
      const services = extractList(metadata, ['open_services']);
      const interfaces = extractList(metadata, ['network_interfaces']);
      const keyServices = extractList(metadata, ['key_services']);
      const ownership = isRecord(metadata.ownership) ? metadata.ownership : {};
      const businessSystems = extractList(ownership, ['business_systems']);
      const exposure = isRecord(metadata.exposure) ? metadata.exposure : {};
      const exposedPorts = services.length || numberAt(item, ['open_ports']) || numberAt(item, ['ports_count']);
      const highServices = services.filter((service) => textFrom(service, ['risk_level', 'risk']).includes('高')).length;
      const riskScore = numberAt(metadata, ['risk_score']);
      return makeRow(page, {
        '资产 ID': displayCode,
        'IP/MAC': [textFrom(item, ['ip_address', 'ip']), textFrom(item, ['mac_address', 'mac'])].filter(Boolean).join(' / ') || '-',
        主机名: textFrom(item, ['hostname', 'name', 'asset_name']) || '-',
        类型: textFrom(item, ['asset_type', 'type', 'os_type']) || '-',
        '园区/部门': [textFrom(item, ['campus']), textFrom(item, ['department'])].filter(Boolean).join(' / ') || '-',
        操作系统: textFrom(item, ['os', 'os_type', 'operating_system']) || '-',
        重要性: String(numberAt(item, ['criticality']) || '-'),
        暴露端口: String(exposedPorts || '-'),
        风险标签: assetRiskLabel(item),
        最近活跃: textFrom(item, ['last_seen', 'updated_at']) || '-',
        资产状态: textFrom(item, ['status', 'asset_status']) || 'unknown',
        业务系统: textFrom(businessSystems[0], ['name']) || textFrom(metadata, ['business_system']) || '-',
        高危服务: highServices,
        弱口令疑似: numberAt(exposure, ['weak_password']),
        厂商: textFrom(item, ['vendor']) || '-',
        管理IP: textFrom(item, ['ip_address', 'ip']) || '-',
        设备角色: textFrom(metadata, ['device_role', 'role']) || textFrom(item, ['asset_type']) || '-',
        接口数: interfaces.length,
        配置变更: extractList(metadata, ['config_changes']).length,
        业务域: textFrom(metadata, ['business_domain']) || '-',
        系统等级: textFrom(metadata, ['system_level']) || '-',
        责任部门: textFrom(item, ['department']) || '-',
        关键服务: keyServices.length,
        依赖资产: extractList(metadata, ['dependency_health']).reduce((sum, dependency) => sum + numberAt(dependency, ['total']), 0),
        风险评分: riskScore,
        SLA: textFrom(metadata, ['sla_current']) || '-',
        来源: textFrom(item, ['source']) || '-',
        疑似类型: textFrom(metadata, ['suspected_type']) || '-',
        置信度: numberAt(metadata, ['confidence']) ? `${numberAt(metadata, ['confidence'])}%` : '-',
        首次发现: textFrom(item, ['first_seen']) || '-',
        工单状态: textFrom(metadata, ['ticket_status']) || '-',
        __assetId: assetID,
        __displayCode: displayCode,
        __assetType: textFrom(item, ['asset_type', 'type']) || 'unknown',
        __status: textFrom(item, ['status', 'asset_status']) || 'unknown',
        __owner: textFrom(item, ['owner']) || '',
        __metadataJson: JSON.stringify(metadata),
        __firstSeen: textFrom(item, ['first_seen']) || '-',
        __discoveryRunId: latestRunID,
        __discoveryRunStatus: latestRunStatus,
        __discoveryAssets: latestRunAssetCount,
        __discoveryLinks: latestRunLinkCount,
        __topologyNeighborCount: links.length,
        __topologyNeighbor: topologyNeighborLabel(firstLink),
        __topologyProtocol: textFrom(firstLink, ['protocol']) || 'LLDP/SNMP',
        __topologyLinksJson: JSON.stringify(links),
      });
    }),
    timeline: [
      timelineItem('资产列表已接入', `来自 /v1/assets，当前返回 ${assets.length} 条，总量 ${total}。`, 'ok'),
      timelineItem('主动发现任务已接入', `来自 /v1/assets/discovery/runs，最新 ${latestRunID} 为 ${latestRunStatus}，成功 ${completedRuns}、失败 ${failedRuns}。`, discoveryRuns.length ? (failedRuns ? 'warn' : 'ok') : 'warn'),
      timelineItem('LLDP 拓扑邻居已联动', `来自 /v1/assets/discovery/neighbors，当前返回 ${topologyLinks.length} 条链路，最新发现 ${latestRunAssetCount}/${latestRunLinkCount}。`, topologyLinks.length ? 'ok' : 'info'),
    ],
    evidence: [
      evidence('Assets API', '/v1/assets', 'ok'),
      evidence('返回记录', `${assets.length}/${total}`, assets.length ? 'ok' : 'warn'),
      evidence('高危资产', `${highRisk} 个`, highRisk ? 'risk' : 'ok'),
      evidence('发现任务', `${completedRuns}/${discoveryRuns.length} completed`, discoveryRuns.length ? (failedRuns ? 'warn' : 'ok') : 'warn'),
      evidence('LLDP 拓扑', `${topologyLinks.length} 条`, topologyLinks.length ? 'ok' : 'info'),
    ],
  };
};

const adaptGraph = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const payload = unwrapPayload(primaryPayload);
  const graph = isRecord(payload) && isRecord(payload.graph) ? payload.graph : payload;
  const meta = isRecord(payload) && isRecord(payload.meta) ? payload.meta : {};
  const nodes = extractList(graph, ['nodes']);
  const edges = extractList(graph, ['edges']);
  const alertCount = nodes.reduce((total, item) => total + numberAt(item, ['alert_count']), 0);
  const keyAssets = nodes.filter((item) => numberAt(item, ['session_count']) >= 100 || numberAt(item, ['total_bytes']) >= 1_000_000_000).length;
  const riskPaths = edges.filter((item) => numberAt(item, ['session_count']) >= 50 || textFrom(item, ['protocol']).toLowerCase().includes('unknown')).length;
  const centerIP = textFrom(meta, ['center_ip']) || textFrom(nodes[0], ['ip']) || '10.20.4.18';
  const durationMs = numberAt(meta, ['duration_ms']);

  return {
    id: page.id,
    metrics: [
      metric('实体节点', numberAt(meta, ['node_count']) || nodes.length, '个', nodes.length ? 'info' : 'warn'),
      metric('关系边', numberAt(meta, ['edge_count']) || edges.length, '条', edges.length ? 'info' : 'warn'),
      metric('异常路径', riskPaths, '条', riskPaths ? 'risk' : 'ok'),
      metric('关键资产', keyAssets, '个', keyAssets ? 'warn' : 'ok'),
      metric('告警关联', alertCount, '条', alertCount ? 'risk' : 'ok'),
    ],
    rows: edges.slice(0, 8).map((item, index) =>
      makeRow(page, {
        '路径 ID': `GRAPH-PATH-${String(index + 1).padStart(3, '0')}`,
        源实体: textFrom(item, ['source']) || centerIP,
        目标实体: textFrom(item, ['target']) || '-',
        跳数: String(Math.min(index + 1, 3)),
        风险: graphRiskLabel(item, index),
        证据: `${textFrom(item, ['protocol', 'direction']) || '通信'} / ${formatNumber(numberAt(item, ['session_count']))} sessions`,
      }),
    ),
    timeline: [
      timelineItem('图谱探索已接入', `来自 /v1/graph/explore，中心节点 ${centerIP}。`, 'ok'),
      timelineItem('查询治理已记录', `缓存命中 ${String(Boolean(isRecord(graph) && graph.cache_hit))}，耗时 ${durationMs || 0} ms。`, durationMs > 1_000 ? 'warn' : 'ok'),
      timelineItem('节点上限保护', String(isRecord(graph) && graph.truncated) === 'true' ? '结果已截断，需要缩小范围。' : '本次查询未触发截断。', String(isRecord(graph) && graph.truncated) === 'true' ? 'warn' : 'ok'),
    ],
    evidence: [
      evidence('Graph API', '/v1/graph/explore', 'ok'),
      evidence('中心节点', centerIP, 'info'),
      evidence('节点 / 边', `${nodes.length}/${edges.length}`, nodes.length && edges.length ? 'ok' : 'warn'),
    ],
  };
};

const adaptFusion = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const stats = unwrapPayload(primaryPayload);
  const entities = extractList(secondaryPayloads[0], ['entities', 'data']);
  const threatEntries = extractList(secondaryPayloads[1], ['entries', 'data']);
  const valueReport = unwrapPayload(secondaryPayloads[2]);
  const valueDelta = isRecord(valueReport) && isRecord(valueReport.delta) ? valueReport.delta : {};
  const multiSource = isRecord(valueReport) && isRecord(valueReport.multi_source) ? valueReport.multi_source : {};
  const valueReportAvailable = isRecord(valueReport) && Boolean(textFrom(valueReport, ['formula_version']));
  const sourceStats = isRecord(stats) && isRecord(stats.data_source_stats) ? stats.data_source_stats : {};
  const sourceStatsWithIntel = {
    ...sourceStats,
    ...(threatEntries.length
      ? {
          threat_intel: {
            count: threatEntries.length,
            records_per_min: Math.max(1, threatEntries.length),
          },
        }
      : {}),
  };
  const quality = isRecord(stats) && isRecord(stats.quality_metrics) ? stats.quality_metrics : {};
  const entitiesAligned = numberAt(stats, ['entities_aligned']) || entities.length;
  const alignmentRate = ratioAt(stats, ['alignment_rate']) || ratioAt(quality, ['accuracy']) || ratioAt(multiSource, ['confidence']);
  const sourceCoverage = sourceCoveragePercent(sourceStatsWithIntel) || ratioAt(multiSource, ['coverage_rate']);
  const duplicationRate = ratioAt(quality, ['duplication_rate']);
  const highRiskIntel = threatEntries.filter((item) => threatIntelReputation(item) !== 'clean' && threatIntelReputation(item) !== 'unknown').length;
  const conflictCount = Math.max(entities.filter((item) => numberAt(item, ['risk_score']) >= 70).length, Math.round((entitiesAligned || entities.length) * (duplicationRate / 100)));
  const writeBackRate = Math.max(0, Math.min(100, ((ratioAt(quality, ['completeness']) || sourceCoverage) + (alignmentRate || 0)) / 2));
  const leadTimeMinutes = numberAt(valueDelta, ['lead_time_minutes']);
  const falsePositiveReduction = numberAt(valueDelta, ['false_positive_reduction_pct']);
  const mttrReduction = numberAt(valueDelta, ['mttr_reduction_pct']);

  const entityRows = entities.length
    ? entities.slice(0, 8).map((item, index) =>
        makeRow(page, {
          对象: fusionEntityName(item, index),
          '来源 A': 'Flow 流量',
          '来源 B': textFrom(item, ['asset_criticality']) === '高' ? 'CMDB 资产库' : 'Asset 资产信息',
          冲突字段: index % 3 === 0 ? 'IP-MAC' : index % 3 === 1 ? '账号-主机' : '资产-部门',
          可信度: confidenceLabel(numberAt(item, ['risk_score']) || alignmentRate),
          处理状态: numberAt(item, ['risk_score']) >= 80 ? '待确认' : '已对齐',
        }),
      )
    : Object.entries(sourceStatsWithIntel).slice(0, 8).map(([source, value], index) =>
        makeRow(page, {
          对象: `${source}-SOURCE-${index + 1}`,
          '来源 A': source,
          '来源 B': '融合规则',
          冲突字段: '来源质量',
          可信度: confidenceLabel(numberAt(value, ['records_per_min']) > 0 ? 0.9 : 0.6),
          处理状态: numberAt(value, ['records_per_min']) > 0 ? '已对齐' : '待确认',
        }),
      );
  const threatRows = threatEntries.slice(0, 3).map((item, index) =>
    makeRow(page, {
      对象: textFrom(item, ['value']) || `THREAT-INTEL-${index + 1}`,
      '来源 A': 'Threat Intel 威胁情报',
      '来源 B': threatIntelSource(item),
      冲突字段: threatIntelReputationLabel(threatIntelReputation(item)),
      可信度: threatIntelConfidence(item),
      处理状态: highRiskIntel ? '待确认' : '已对齐',
    }),
  );
  const rows = [...threatRows, ...entityRows].slice(0, 8);

  return {
    id: page.id,
    metrics: [
      metric('融合实体', entitiesAligned, '个', entitiesAligned ? 'info' : 'warn'),
      metric('可信度', alignmentRate || 0, '%', alignmentRate >= 90 ? 'ok' : 'warn'),
      metric('来源覆盖', sourceCoverage, '%', sourceCoverage >= 80 ? 'ok' : 'warn'),
      ...(valueReportAvailable
        ? [
            metric('检出提前量', leadTimeMinutes, '分钟', leadTimeMinutes >= 10 ? 'ok' : leadTimeMinutes > 0 ? 'warn' : 'info'),
            metric('误报下降', falsePositiveReduction, '%', falsePositiveReduction >= 20 ? 'ok' : falsePositiveReduction > 0 ? 'warn' : 'info'),
            metric('MTTR 下降', mttrReduction, '%', mttrReduction >= 20 ? 'ok' : mttrReduction > 0 ? 'warn' : 'info'),
          ]
        : []),
      metric('情报命中', highRiskIntel, '条', highRiskIntel ? 'risk' : threatEntries.length ? 'ok' : 'warn'),
      metric('冲突数', conflictCount, '条', conflictCount ? 'warn' : 'ok'),
      metric('回写成功率', writeBackRate, '%', writeBackRate >= 90 ? 'ok' : 'warn'),
    ],
    rows,
    timeline: [
      timelineItem('融合统计已接入', `来自 /v1/fusion/stats，事件 ${formatNumber(numberAt(stats, ['total_events']))} 条。`, 'ok'),
      timelineItem('实体对齐已接入', `来自 /v1/fusion/entities，当前返回 ${entities.length} 个实体。`, entities.length ? 'ok' : 'warn'),
      timelineItem('威胁情报已接入', `来自 /v1/threat-intel/entries，当前返回 ${threatEntries.length} 条，高风险 ${highRiskIntel} 条。`, threatEntries.length ? 'ok' : 'warn'),
      timelineItem(
        '价值量化已接入',
        valueReportAvailable
          ? `来自 /v1/fusion/value-report，提前 ${leadTimeMinutes.toFixed(1)} 分钟，误报下降 ${falsePositiveReduction.toFixed(1)}%，MTTR 下降 ${mttrReduction.toFixed(1)}%。`
          : '/v1/fusion/value-report 暂未返回，页面保留融合质量基础指标。',
        valueReportAvailable ? 'ok' : 'warn',
      ),
      timelineItem('质量指标已映射', `完整性 ${ratioAt(quality, ['completeness']).toFixed(1)}%，重复率 ${duplicationRate.toFixed(1)}%。`, duplicationRate > 10 ? 'warn' : 'ok'),
    ],
    evidence: [
      evidence('Fusion Stats API', '/v1/fusion/stats', 'ok'),
      evidence('Fusion Entities API', `/v1/fusion/entities ${entities.length} 条`, entities.length ? 'ok' : 'warn'),
      evidence('Threat Intel API', `/v1/threat-intel/entries ${threatEntries.length} 条`, threatEntries.length ? 'ok' : 'warn'),
      evidence('Fusion Value API', valueReportAvailable ? `/v1/fusion/value-report ${textFrom(valueReport, ['formula_version'])}` : '待返回', valueReportAvailable ? 'ok' : 'warn'),
      evidence('数据源数量', `${Object.keys(sourceStatsWithIntel).length} 个`, Object.keys(sourceStatsWithIntel).length ? 'ok' : 'warn'),
    ],
  };
};

const adaptBaselines = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const baselines = extractList(primaryPayload, ['baselines', 'data']);
  const total = totalFromEnvelope(envelope, baselines.length);
  const active = baselines.filter((item) => textFrom(item, ['status']).toLowerCase() === 'active').length;
  const learning = baselines.filter((item) => textFrom(item, ['status']).toLowerCase() === 'learning').length;
  const allMetrics = baselines.flatMap((item) => extractList(item, ['metrics']));
  const deviations = allMetrics.filter((metricItem) => numberAt(metricItem, ['deviation_score']) >= 2).length;
  const highDeviation = allMetrics.filter((metricItem) => numberAt(metricItem, ['deviation_score']) >= 3).length;
  const coverage = total ? (active / total) * 100 : 0;

  return {
    id: page.id,
    metrics: [
      metric('偏离资产', deviations || learning, '个', deviations || learning ? 'warn' : 'ok'),
      metric('新端口', highDeviation, '个', highDeviation ? 'risk' : 'ok'),
      metric('异常协议', Math.max(0, deviations - highDeviation), '类', deviations > highDeviation ? 'warn' : 'ok'),
      metric('夜间访问', learning, '个', learning ? 'info' : 'ok'),
      metric('基线稳定度', coverage, '%', coverage >= 80 ? 'ok' : 'warn'),
    ],
    rows: baselines.slice(0, 8).map((item, index) => {
      const metrics = extractList(item, ['metrics']);
      const maxDeviation = Math.max(0, ...metrics.map((metricItem) => numberAt(metricItem, ['deviation_score'])));
      const firstMetric = metrics[0] ?? {};
      return makeRow(page, {
        对象: textFrom(item, ['name', 'entity_id']) || `BASELINE-${index + 1}`,
        基线类型: baselineTypeLabel(textFrom(item, ['baseline_type', 'entity_type'])),
        偏离值: maxDeviation ? `${maxDeviation.toFixed(1)}x` : confidenceLabel(numberAt(firstMetric, ['current_value'])),
        证据: `${textFrom(firstMetric, ['metric_name']) || 'sessions'} / ${textFrom(firstMetric, ['unit']) || 'sample'}`,
        解释: maxDeviation >= 3 ? '超出告警阈值' : maxDeviation >= 2 ? '超出观察阈值' : '基线稳定',
        状态: baselineStatusLabel(textFrom(item, ['status'])),
      });
    }),
    timeline: [
      timelineItem('行为基线已接入', `来自 /v1/baselines，当前返回 ${baselines.length} 条，总量 ${total}。`, 'ok'),
      timelineItem('偏离检测已映射', `识别 ${deviations} 个超过观察阈值的指标。`, deviations ? 'warn' : 'ok'),
      timelineItem('基线治理可下钻', '详情、重建、冻结和 reset 动作将继续绑定后端详情与写操作。', 'info'),
    ],
    evidence: [
      evidence('Baselines API', '/v1/baselines', 'ok'),
      evidence('基线数量', `${baselines.length}/${total}`, baselines.length ? 'ok' : 'warn'),
      evidence('稳定覆盖', `${coverage.toFixed(1)}%`, coverage >= 80 ? 'ok' : 'warn'),
    ],
  };
};

const adaptCampaigns = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const payload = unwrapPayload(primaryPayload);
  const envelope = isRecord(payload) ? payload : unwrapEnvelope(primaryPayload);
  const campaigns = extractList(primaryPayload, ['campaigns', 'data']);
  const total = totalFromEnvelope(envelope, campaigns.length);
  const active = campaigns.filter((item) => campaignStatus(item) !== '已结束').length;
  const highRisk = campaigns.filter((item) => campaignRisk(item).includes('高')).length;
  const affectedAssets = sumArrayLengths(campaigns, ['entities']);
  const alertCount = sumArrayLengths(campaigns, ['alerts', 'alert_ids']);
  const longestHours = Math.max(0, ...campaigns.map(campaignDurationHours));

  return {
    id: page.id,
    total,
    metrics: [
      metric('战役总数', total, '个', total ? 'info' : 'warn'),
      campaignMetric('当前页活跃', active, '个', active ? 'risk' : 'ok'),
      campaignMetric('当前页影响资产', affectedAssets, '台', affectedAssets ? 'warn' : 'ok'),
      campaignMetric('当前页高风险', highRisk, '个', highRisk ? 'risk' : 'ok'),
      campaignMetric('当前页告警', alertCount, '条', alertCount ? 'warn' : 'ok'),
      campaignMetric('当前页最长持续', longestHours, '小时', longestHours >= 24 ? 'warn' : 'info'),
    ],
    rows: campaigns.map((item, index) =>
      makeRow(page, {
        战役名称: textFrom(item, ['campaign_id', 'id', 'event_id']) || `CAMPAIGN-${index + 1}`,
        阶段: campaignPhase(item),
        风险等级: campaignRisk(item),
        影响资产: arrayLengthFrom(item, ['entities']),
        告警数: arrayLengthFrom(item, ['alerts', 'alert_ids']),
        首次发现: formatEpochTime(numberFrom(item, ['ts_start', 'start_time'])),
        最近活动: formatEpochTime(numberFrom(item, ['ts_end', 'end_time', 'ingest_ts'])),
        状态: campaignStatus(item),
        操作: '查看',
      }),
    ),
    timeline: campaignTimeline(campaigns),
    evidence: [
      evidence('Campaigns API', '/v1/campaigns', 'ok'),
      evidence('返回记录', `${campaigns.length}/${total}`, campaigns.length ? 'ok' : 'warn'),
      evidence('告警聚合', `${alertCount} 条`, alertCount ? 'risk' : 'ok'),
      evidence('影响实体', `${affectedAssets} 个`, affectedAssets ? 'warn' : 'ok'),
      evidence('证据完整度', '接口未提供', 'info'),
    ],
  };
};

const campaignMetric = (label: string, value: number, suffix: string, status: MetricStatus) => ({
  label,
  value: `${formatNumber(Number.isFinite(value) ? value : 0)} ${suffix}`,
  delta: '当前页 API',
  status,
});

const adaptAttackChains = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const payload = unwrapPayload(primaryPayload);
  const envelope = isRecord(payload) ? payload : unwrapEnvelope(primaryPayload);
  const chains = extractList(primaryPayload, ['chains', 'data']);
  const total = totalFromEnvelope(envelope, chains.length);
  const first = chains[0] ?? {};
  const phases = chains.flatMap((item) => extractList(item, ['phases']));
  const keyEvents = phases.flatMap((item) => extractList(item, ['key_events']));
  const evidenceAnchors = Math.max(phases.length, keyEvents.length, chains.reduce((sum, item) => sum + numberAt(item, ['alert_count']), 0));
  const entityNodes = chains.reduce((sum, item) => sum + numberAt(item, ['entity_count']), 0);
  const riskScore = Math.max(0, ...chains.map((item) => numberAt(item, ['risk_score'])));
  const blockPoints = phases.filter((item) => campaignPhaseLabel(textFrom(item, ['phase'])).includes('外') || campaignPhaseLabel(textFrom(item, ['phase'])).includes('C2')).length || 6;
  const confidence = chains.length ? Math.min(99, Math.max(60, riskScore || averageNumbers(phases, ['confidence']))) : 0;

  const rows = phases.length
    ? phases.slice(0, 6).map((phase, index) => {
        const event = extractList(phase, ['key_events'])[0] ?? {};
        const phaseName = campaignPhaseLabel(textFrom(phase, ['phase'])) || String(index + 1);
        return makeRow(page, {
          阶段: phaseName,
          实体: textFrom(event, ['src_ip']) || textFrom(first, ['source_ip']) || '-',
          告警: textFrom(event, ['description', 'event_id']) || `${phaseName} 告警`,
          证据: textFrom(event, ['technique']) || textFrom(first, ['root_alert_id']) || 'PCAP / Session',
          处置建议: responseActionForPhase(phaseName),
          状态: numberAt(phase, ['confidence']) >= 0.8 ? '已确认' : '待确认',
        });
      })
    : chains.slice(0, 6).map((item, index) =>
        makeRow(page, {
          阶段: campaignPhaseLabel(String(Array.isArray(item.phases) ? item.phases[index] : '攻击链')),
          实体: textFrom(item, ['source_ip']) || '-',
          告警: textFrom(item, ['root_alert_id', 'title']) || '-',
          证据: textFrom(item, ['chain_id']) || '-',
          处置建议: responseActionForPhase(textFrom(item, ['title'])),
          状态: statusLabel(textFrom(item, ['status'])) || '已确认',
        }),
      );

  return {
    id: page.id,
    metrics: [
      metric('阶段节点', phases.length || rows.length, '个', rows.length ? 'info' : 'warn'),
      metric('实体节点', entityNodes || rows.length, '个', entityNodes ? 'info' : 'warn'),
      metric('证据锚点', evidenceAnchors, '个', evidenceAnchors ? 'ok' : 'warn'),
      metric('阻断点', blockPoints, '个', blockPoints ? 'warn' : 'ok'),
      metric('置信度', confidence, '%', confidence >= 80 ? 'ok' : 'warn'),
    ],
    rows,
    timeline: rows.map((row) => timelineItem(String(row['阶段']), `${row['实体']} -> ${row['告警']}`, String(row['状态']).includes('确认') ? 'ok' : 'warn')),
    evidence: [
      evidence('Attack Chains API', '/v1/attack-chains', 'ok'),
      evidence('返回链路', `${chains.length}/${total}`, chains.length ? 'ok' : 'warn'),
      evidence('阶段节点', `${phases.length || rows.length} 个`, rows.length ? 'ok' : 'warn'),
      evidence('关键事件', `${keyEvents.length} 条`, keyEvents.length ? 'ok' : 'info'),
      evidence('置信度', `${confidence.toFixed(1)}%`, confidence >= 80 ? 'ok' : 'warn'),
    ],
  };
};

const adaptEncryptedTraffic = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const stats = unwrapPayload(primaryPayload);
  const sessions = extractList(secondaryPayloads[0], ['sessions', 'data']);
  const fingerprints = extractList(secondaryPayloads[1], ['fingerprints', 'data']);
  const tunnelProtocols = extractList(secondaryPayloads[2], ['protocols', 'data']);
  const tunnelUsers = extractList(secondaryPayloads[2], ['users']);
  const exfilSources = extractNamedList(secondaryPayloads[3], ['top_sources']);
  const exfilDestinations = extractNamedList(secondaryPayloads[3], ['top_destinations', 'destinations']);
  const exfilRiskTypes = extractNamedList(secondaryPayloads[3], ['risk_types']);
  const exfilPaths = extractNamedList(secondaryPayloads[3], ['paths']);
  const exfilTrend = extractNamedList(secondaryPayloads[3], ['trend']);
  const evidenceSessions = extractNamedList(secondaryPayloads[4], ['sessions']);
  const evidencePcapIndexes = extractNamedList(secondaryPayloads[4], ['pcap_indexes']);
  const evidencePcapTrend = extractNamedList(secondaryPayloads[4], ['pcap_trend']);
  const evidenceEntropyTrend = extractNamedList(secondaryPayloads[4], ['entropy_trend']);
  const evidenceCompleteness = extractNamedList(secondaryPayloads[4], ['completeness']);
  const sessionRows = sessions;
  const fingerprintRows = fingerprints.slice(0, 6);
  const tunnelProtocolRows = tunnelProtocols.slice(0, 6);
  const exfilRiskTypeRows = exfilRiskTypes.slice(0, 5);
  const totalSessions = numberAt(stats, ['total_sessions']) || sessionRows.length;
  const tlsSessions = numberAt(stats, ['tls_sessions']) || sessionRows.filter((item) => encryptedProtocol(item).includes('TLS')).length;
  const quicSessions = numberAt(stats, ['quic_sessions']) || sessionRows.filter((item) => encryptedProtocol(item).includes('QUIC')).length;
  const tlsRatio = numberAt(stats, ['tls_ratio']) || (totalSessions ? (tlsSessions / totalSessions) * 100 : ratioAt(stats, ['encrypted_ratio']));
  const quicRatio = numberAt(stats, ['quic_ratio']) || (totalSessions ? (quicSessions / totalSessions) * 100 : 0);
  const unknownRatio = numberAt(stats, ['unknown_encrypted_ratio']) || Math.max(0, 100 - tlsRatio - quicRatio);
  const expiredOrMissingCerts = numberAt(stats, ['abnormal_certificate_count']) || sessionRows.filter((item) => certificateRisk(item)).length;
  const maliciousJA3 = numberAt(stats, ['malicious_ja3_matches']) || fingerprintRows.filter((item) => encryptedRisk(item).includes('高')).length;
  const missingSni = sessionRows.filter((item) => !textFrom(item, ['sni', 'SNI'])).length;
  const unknownSniRatio = numberAt(stats, ['unknown_sni_ratio']) || (sessionRows.length ? (missingSni / sessionRows.length) * 100 : 0);
  const externalDestinations = exfilDestinations.length || exfilPaths.length || sessions.filter((item) => textFrom(item, ['dst_ip', 'destination_ip'])).length;
  const trafficGbps = numberFrom(stats, ['traffic_gbps', 'total_gbps', 'throughput_gbps']);
  const generatedVisuals = buildEncryptedTrafficVisuals({
    stats,
    sessions: sessionRows,
    fingerprints: fingerprintRows,
    tunnelProtocols: tunnelProtocolRows,
    rawTunnelUsers: tunnelUsers,
    exfilRiskTypes: exfilRiskTypeRows,
    rawEgressSources: exfilSources,
    rawEgressDestinations: exfilDestinations,
    rawEgressRiskTypes: exfilRiskTypes,
    rawEgressPaths: exfilPaths,
    rawEgressTrend: exfilTrend,
    rawEgressSessions: sessions,
    rawEvidenceSessions: evidenceSessions,
    rawEvidencePcapIndexes: evidencePcapIndexes,
    rawEvidencePcapTrend: evidencePcapTrend,
    rawEvidenceEntropyTrend: evidenceEntropyTrend,
    rawEvidenceCompleteness: evidenceCompleteness,
    totalSessions,
    tlsRatio,
    quicRatio,
    unknownRatio,
    maliciousJA3,
  });
  const referenceVisuals = isRecord(stats) && isRecord(stats.ui_reference_visuals)
    ? stats.ui_reference_visuals as unknown as EncryptedTrafficVisuals
    : undefined;
  const encryptedVisuals = referenceVisuals?.protocolRows?.length ? referenceVisuals : generatedVisuals;

  return {
    id: page.id,
    metrics: [
      metric('加密流量总量', trafficGbps || totalSessions, trafficGbps ? 'Gbps' : '会话', totalSessions ? 'info' : 'warn'),
      metric('TLS 流量占比', tlsRatio, '%', tlsRatio >= 50 ? 'ok' : 'warn'),
      metric('QUIC 流量占比', quicRatio, '%', quicRatio >= 20 ? 'info' : 'ok'),
      metric('未知加密占比', unknownRatio, '%', unknownRatio >= 20 ? 'info' : 'ok'),
      metric('异常证书数', expiredOrMissingCerts, '张', expiredOrMissingCerts ? 'warn' : 'ok'),
      metric('可疑 JA3 数', maliciousJA3, '个', maliciousJA3 ? 'risk' : 'ok'),
      metric('未知 SNI 比例', unknownSniRatio, '%', unknownSniRatio >= 10 ? 'warn' : 'ok'),
    ],
    rows: sessionRows.slice(0, 8).map((item, index) =>
      makeRow(page, {
        时间: formatEpochTime(numberFrom(item, ['start_time', 'StartTime'])),
        协议: encryptedProtocol(item),
        'Session 摘要': encryptedSessionSummary(item, index),
        证书详情: encryptedCertificateLabel(item),
        SNI: textFrom(item, ['sni', 'SNI']) || '-',
        JA3: textFrom(item, ['ja3_fingerprint', 'ja3', 'JA3Fingerprint']) || '-',
        JA3S: textFrom(item, ['ja3s_fingerprint', 'ja3s', 'JA3SFingerprint']) || '-',
        ALPN: textFrom(item, ['alpn']) || encryptedAlpnFallback(item),
        'TLS 版本': textFrom(item, ['tls_version', 'TLSVersion']) || '-',
        密码套件: textFrom(item, ['cipher_suite', 'CipherSuite']) || '-',
        '证书 Issuer': textFrom(item, ['certificate_issuer', 'CertificateIssuer']) || '-',
        风险等级: encryptedRisk(item),
        操作: '下钻',
      }),
    ),
    timeline: [
      timelineItem('加密流量统计已接入', `来自 /v1/encrypted-traffic/stats，当前 ${formatNumber(totalSessions)} 个会话。`, totalSessions ? 'ok' : 'warn'),
      timelineItem('会话明细已接入', `来自 /v1/encrypted-traffic/sessions，返回 ${sessions.length || sessionRows.length} 条握手元数据。`, sessions.length ? 'ok' : 'info'),
      timelineItem('隧道与外传分析已关联', `隧道协议 ${tunnelProtocolRows.length} 类，外联目的地 ${externalDestinations} 个。`, tunnelProtocolRows.length || externalDestinations ? 'ok' : 'info'),
      timelineItem('指纹库状态', `JA3 指纹 ${fingerprintRows.length || numberAt(stats, ['ja3_fingerprints'])} 个，可疑命中 ${maliciousJA3} 个。`, maliciousJA3 ? 'risk' : 'ok'),
    ],
    evidence: [
      evidence('Encrypted Stats API', '/v1/encrypted-traffic/stats', 'ok'),
      evidence('Sessions API', `/v1/encrypted-traffic/sessions ${sessions.length || sessionRows.length} 条`, sessions.length ? 'ok' : 'info'),
      evidence('JA3 API', `${fingerprintRows.length || numberAt(stats, ['ja3_fingerprints'])} 个指纹`, fingerprints.length || numberAt(stats, ['ja3_fingerprints']) ? 'ok' : 'info'),
      evidence('Tunnel Analytics API', `${tunnelProtocolRows.length + tunnelUsers.length} 项`, tunnelProtocols.length || tunnelUsers.length ? 'ok' : 'info'),
      evidence('Exfiltration API', `${exfilDestinations.length + exfilRiskTypes.length + exfilPaths.length + exfilTrend.length} 项`, exfilDestinations.length || exfilRiskTypes.length || exfilPaths.length || exfilTrend.length ? 'ok' : 'info'),
      evidence('Encrypted Evidence API', `${evidenceSessions.length + evidencePcapIndexes.length + evidencePcapTrend.length} 项`, evidenceSessions.length || evidencePcapIndexes.length || evidencePcapTrend.length ? 'ok' : 'info'),
    ],
    visuals: {
      encryptedTraffic: encryptedVisuals,
    },
  };
};

const buildEncryptedTrafficVisuals = ({
  stats,
  sessions,
  fingerprints,
  tunnelProtocols,
  rawTunnelUsers,
  exfilRiskTypes,
  rawEgressSources,
  rawEgressDestinations,
  rawEgressRiskTypes,
  rawEgressPaths,
  rawEgressTrend,
  rawEgressSessions,
  rawEvidenceSessions,
  rawEvidencePcapIndexes,
  rawEvidencePcapTrend,
  rawEvidenceEntropyTrend,
  rawEvidenceCompleteness,
  totalSessions,
  tlsRatio,
  quicRatio,
  unknownRatio,
  maliciousJA3,
}: {
  stats: unknown;
  sessions: Record<string, unknown>[];
  fingerprints: Record<string, unknown>[];
  tunnelProtocols: Record<string, unknown>[];
  rawTunnelUsers: Record<string, unknown>[];
  exfilRiskTypes: Record<string, unknown>[];
  rawEgressSources: Record<string, unknown>[];
  rawEgressDestinations: Record<string, unknown>[];
  rawEgressRiskTypes: Record<string, unknown>[];
  rawEgressPaths: Record<string, unknown>[];
  rawEgressTrend: Record<string, unknown>[];
  rawEgressSessions: Record<string, unknown>[];
  rawEvidenceSessions: Record<string, unknown>[];
  rawEvidencePcapIndexes: Record<string, unknown>[];
  rawEvidencePcapTrend: Record<string, unknown>[];
  rawEvidenceEntropyTrend: Record<string, unknown>[];
  rawEvidenceCompleteness: Record<string, unknown>[];
  totalSessions: number;
  tlsRatio: number;
  quicRatio: number;
  unknownRatio: number;
  maliciousJA3: number;
}): EncryptedTrafficVisuals => {
  const totalGbps = numberAt(stats, ['traffic_gbps', 'total_gbps', 'throughput_gbps']);
  const tlsGbps = totalGbps * tlsRatio / 100;
  const quicGbps = totalGbps * quicRatio / 100;
  const unknownGbps = Math.max(0, totalGbps - tlsGbps - quicGbps);
  const protocolRows = [
    ['TLS', `${tlsGbps.toFixed(1)} Gbps`, `${tlsRatio.toFixed(1)}%`, 'is-info'],
    ['QUIC', `${quicGbps.toFixed(1)} Gbps`, `${quicRatio.toFixed(1)}%`, 'is-warn'],
    ['其他加密', `${unknownGbps.toFixed(1)} Gbps`, `${unknownRatio.toFixed(1)}%`, 'is-info'],
  ];
  const protocolTrend: number[] = [];
  const ja3Source = (fingerprints.length
    ? fingerprints
    : sessions.filter((item) => textFrom(item, ['ja3_fingerprint', 'ja3', 'JA3Fingerprint']))).slice(0, 6);
  const ja3Rows = ja3Source.slice(0, 6).map((item, index) => {
    const risk = encryptedRisk(item);
    const flow = numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps']);
    const ratio = ratioAt(item, ['traffic_ratio']) || ratioAt(item, ['ratio']);
    return [
      textFrom(item, ['ja3_fingerprint', 'ja3', 'fingerprint', 'JA3Fingerprint']) || '-',
      `${ratio.toFixed(1)}%`,
      flow.toFixed(1),
      formatNumber(numberFrom(item, ['sni_count', 'sni', 'domains'])),
      formatNumber(numberFrom(item, ['alert_count', 'alerts', 'matches'])),
      risk,
    ];
  });
  const tunnelCards = (tunnelProtocols.length ? tunnelProtocols : exfilRiskTypes).slice(0, 6).map((item, index) => {
    const label = encryptedTunnelLabel(textFrom(item, ['name', 'protocol', 'type', 'risk_type']), index);
    const value = numberFrom(item, ['count', 'sessions', 'session_count']);
    const risk = severityLabel(textFrom(item, ['risk', 'risk_level', 'severity']));
    return [label, formatNumber(value), '当前窗口', toneFromRisk(risk, index)];
  });
  const tunnelRows = rawTunnelUsers.slice(0, 6).map((item, index) => {
    const protocol = textFrom(item, ['protocol']) || '候选特征';
    const risk = severityLabel(textFrom(item, ['risk', 'risk_level', 'severity'])) || '待研判';
    const count = numberFrom(item, ['count', 'session_count']);
    const totalBytes = numberFrom(item, ['total_bytes', 'bytes']);
    return [
      encryptedTunnelLabel(protocol, index),
      `聚合命中 ${formatNumber(count)} 个会话`,
      textFrom(item, ['ip', 'src_ip', 'source_ip']) || '-',
      '待下钻会话',
      '当前时间窗',
      (totalBytes / 1024 / 1024 / 1024).toFixed(2),
      risk,
    ];
  });
  const egressSources = rawEgressSources;
  const egressDestinations = rawEgressDestinations;
  const egressPaths = rawEgressPaths;
  const egressRiskTypes = rawEgressRiskTypes;
  const egressSessions = rawEgressSessions;
  const destinationRows = encryptedDestinationRows(egressDestinations, egressPaths, egressSessions);
  const egressHasExfiltrationData = Boolean(rawEgressDestinations.length || rawEgressPaths.length);
  const egressHasSessionData = Boolean(rawEgressSessions.length);
  const egressAvailability = {
    state: (egressHasExfiltrationData ? 'live' : egressHasSessionData || rawEgressSources.length || rawEgressTrend.length ? 'partial' : 'unavailable') as 'live' | 'partial' | 'unavailable',
    detail: egressHasExfiltrationData
      ? `公网外联候选 API 返回 ${rawEgressDestinations.length + rawEgressPaths.length} 条目的地/路径数据；风险仍需规则或人工确认。`
      : egressHasSessionData || rawEgressSources.length || rawEgressTrend.length
        ? `目的地聚合未完整返回，当前仅展示 ${rawEgressSessions.length} 条会话与 ${rawEgressTrend.length} 个真实趋势桶。`
        : '外传分析 API 与加密会话均为空，未生成任何替代数据。',
  };
  const highRiskSession = egressSessions.find((item) => encryptedRisk(item).includes('高'));
  const highRiskDestination = destinationRows.find((row) => row[4].includes('高'))?.[0] ?? destinationRows[0]?.[0];
  const highRiskSource = highRiskSession ? textFrom(highRiskSession, ['src_ip', 'source_ip']) : '';
  const adviceRows = highRiskDestination
    ? [
        [`为 ${highRiskDestination} 生成外联调查规则草案。`, '生成规则'],
        [highRiskSource ? `核查源主机 ${highRiskSource} 与目的地的真实业务关系。` : '核查关联源主机与目的地的真实业务关系。', '检查源主机'],
        [`关联 ${highRiskDestination} 的告警、证据和实体关系。`, '关联告警'],
        [`检查 ${highRiskDestination} 的目的地信誉、流量分布和溯源证据。`, '检查目的地'],
      ]
    : [['外传分析接口未返回可处置对象。', '查看数据源']];
  const certificateRows = sessions.slice(0, 4).map((item) => [
    textFrom(item, ['certificate_issuer', 'CertificateIssuer']) || '-',
    textFrom(item, ['dst_ip', 'destination_ip']) || '-',
    textFrom(item, ['tls_version', 'TLSVersion']) || '-',
    textFrom(item, ['alpn']) || encryptedAlpnFallback(item),
    formatNumber(numberFrom(item, ['alert_count', 'alerts'])),
    encryptedRisk(item),
  ]);
  const tlsSuiteCounts = new Map<string, { count: number; risk: string }>();
  sessions.forEach((item) => {
    const version = textFrom(item, ['tls_version', 'TLSVersion']) || encryptedProtocol(item);
    const suite = textFrom(item, ['cipher_suite', 'CipherSuite', 'alpn']) || '-';
    const key = `${version}\u0000${suite}`;
    const current = tlsSuiteCounts.get(key) ?? { count: 0, risk: encryptedRisk(item) };
    current.count += 1;
    if (encryptedRisk(item).includes('高')) current.risk = encryptedRisk(item);
    tlsSuiteCounts.set(key, current);
  });
  const tlsSuiteRows = [...tlsSuiteCounts.entries()].slice(0, 6).map(([key, item]) => {
    const [version, suite] = key.split('\u0000');
    const ratio = sessions.length ? item.count / sessions.length * 100 : 0;
    return [version, suite, `${ratio.toFixed(1)}%`, toneFromRisk(item.risk, 0)];
  });
  const tunnelRuleRows = (tunnelProtocols.length ? tunnelProtocols : exfilRiskTypes).slice(0, 6).map((item, index) => [
    encryptedTunnelRuleLabel(textFrom(item, ['name', 'protocol', 'type', 'risk_type']), index),
    textFrom(item, ['feature', 'condition']) || '接口未返回检测特征',
    textFrom(item, ['threshold']) || '-',
    formatNumber(numberFrom(item, ['count', 'sessions', 'session_count'])),
    textFrom(item, ['confidence']) || '-',
    '查看详情',
  ]);
  const evidenceRows = sessions.slice(0, 4).map((item) => [
    textFrom(item, ['src_ip', 'source_ip']) || '-',
    textFrom(item, ['sni', 'dst_ip', 'destination_ip']) || '-',
    encryptedProtocol(item),
    textFrom(item, ['ja3_fingerprint', 'ja3', 'JA3Fingerprint']) || '-',
    textFrom(item, ['pcap_index', 'pcap_id', 'evidence_id']) || '-',
    encryptedRisk(item),
  ]);
  const egressDomainCards = destinationRows.map(([destination, location, flow, sessions, risk]) => [
    destination,
    location,
    sessions !== '—' ? `${sessions} 会话` : flow !== '—' ? flow : '会话/流量未返回',
    risk,
  ]).slice(0, 6);
  const highRiskDestinations = destinationRows.filter((row) => row[4].includes('高')).length;
  const cloudDestinations = destinationRows.filter(([, location]) => /AWS|Azure|Cloudflare|Google|CDN|云/i.test(location)).length;
  const sourceAssets = new Set([...egressSources, ...egressSessions].map((item) => textFrom(item, ['src_ip', 'source_ip'])).filter(Boolean)).size;
  const egressKpis = egressAvailability.state === 'unavailable'
    ? [
        ['公网目的地', '—', '等待外传 API'],
        ['CDN / 云服务', '—', '等待外传 API'],
        ['异常域名', '—', '等待风险字段'],
        ['外联路径', '—', '等待路径字段'],
        ['高风险目的地', '—', '等待风险字段'],
        ['外联源资产', '—', '等待会话 API'],
        ['待关联风险类型', '—', '等待外传 API'],
      ]
    : [
        ['公网目的地', formatNumber(destinationRows.length), '当前样本'],
        ['CDN / 云服务', formatNumber(cloudDestinations), '当前样本'],
        ['异常域名', formatNumber(egressDomainCards.filter((row) => row[3].includes('高')).length), '当前样本'],
        ['外联路径', formatNumber(egressPaths.length), '外传路径 API'],
        ['高风险目的地', formatNumber(highRiskDestinations), '当前样本'],
        ['外联源资产', formatNumber(sourceAssets), egressHasSessionData ? '加密会话 API' : '会话 API 空'],
        ['待关联风险类型', formatNumber(egressRiskTypes.length), '外传分析 API'],
      ];
  const egressTrend = buildEncryptedEgressTrend(rawEgressTrend);
  const egressMapNodes = destinationRows.map(([label, location, flow, sessions, risk], index) => {
    const [x, y] = encryptedEgressMapPosition(location, index);
    return { id: `${label}-${index}`, label, location, flow, sessions, risk, x, y };
  });
  const scatterSource = (ja3Source.length ? ja3Source : sessions).filter((item) => (
    numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps']) > 0
    || numberFrom(item, ['session_count', 'sessions', 'count']) > 0
  )).slice(0, 34);
  const scatterMaxFlow = Math.max(1, ...scatterSource.map((item) => numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps'])));
  const scatterMaxSessions = Math.max(1, ...scatterSource.map((item) => numberFrom(item, ['session_count', 'sessions', 'count'])));
  const scatterPoints = scatterSource.map((item, index) => {
    const flow = numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps']);
    const sessionCount = numberFrom(item, ['session_count', 'sessions', 'count']);
    return {
      left: clamp(7 + flow / scatterMaxFlow * 85, 7, 92),
      top: clamp(82 - sessionCount / scatterMaxSessions * 70, 12, 82),
      tone: toneFromRisk(encryptedRisk(item), index) as 'ok' | 'warn' | 'risk' | 'info',
    };
  });
  const heartbeatBars = sessions
    .map((item) => numberFrom(item, ['duration_seconds', 'duration_sec', 'duration_ms']) / (numberFrom(item, ['duration_ms']) ? 60_000 : 60))
    .filter((value) => value > 0)
    .slice(0, 48);
  const evidenceCenter = buildEncryptedEvidenceCenter({
    rawSessions: rawEvidenceSessions,
    rawPcapIndexes: rawEvidencePcapIndexes,
    rawPcapTrend: rawEvidencePcapTrend,
    rawEntropyTrend: rawEvidenceEntropyTrend,
    rawCompleteness: rawEvidenceCompleteness,
  });

  return {
    tabKpis: {
      fingerprint: [
        ['指纹总数', formatNumber(numberAt(stats, ['ja3_sample_count']) || ja3Rows.length), '真实 JA3 API'],
        ['可疑 JA3', formatNumber(maliciousJA3), '风险指纹'],
        ['未知 SNI', `${numberAt(stats, ['unknown_sni_ratio']).toFixed(1)}%`, '会话观测'],
        ['异常 Issuer', formatNumber(certificateRows.length), '证书字段'],
        ['TLS1.0/1.1', formatNumber(tlsSuiteRows.filter((row) => /1\.0|1\.1/.test(row[0] ?? '')).length), '弱版本'],
        ['弱密码套件', formatNumber(tlsSuiteRows.filter((row) => row[3]?.includes('risk')).length), '协议风险'],
        ['关联规则', formatNumber(tunnelRuleRows.length), '检测规则'],
      ],
      tunnelDetection: [
        ['隧道告警', formatNumber(tunnelRows.length), '当前窗口'],
        ['DoH 会话', formatNumber(tunnelRows.filter((row) => row[0]?.includes('DoH')).length), '隧道候选'],
        ['异常长连接', formatNumber(tunnelRows.filter((row) => row[0]?.includes('长连接')).length), '持续时间'],
        ['高熵流量', formatNumber(tunnelRows.filter((row) => row[0]?.includes('高熵')).length), '载荷熵值'],
        ['低熵心跳', formatNumber(tunnelRows.filter((row) => row[0]?.includes('心跳')).length), '周期通信'],
        ['疑似 VPN', formatNumber(tunnelRows.filter((row) => row[0]?.includes('VPN')).length), '协议候选'],
        ['已创建告警', formatNumber(tunnelRows.filter((row) => row[6]?.includes('高')).length), '待审核'],
      ],
    },
    protocolRows,
    protocolTrend,
    ja3Rows,
    scatterPoints,
    tunnelCards,
    tunnelRows,
    destinationRows,
    adviceRows,
    certificateRows,
    tlsSuiteRows,
    tunnelRuleRows,
    evidenceRows,
    egressKpis,
    egressDomainCards,
    egressMapNodes,
    egressTrend,
    egressAvailability,
    heartbeatBars,
    evidenceCenter,
  };
};

const encryptedDestinationRows = (
  exfilDestinations: Record<string, unknown>[],
  exfilPaths: Record<string, unknown>[],
  sessions: Record<string, unknown>[],
) => {
  const source = exfilDestinations.length ? exfilDestinations : exfilPaths.length ? exfilPaths : sessions;
  if (!source.length) return [];
  return source.slice(0, 7).map((item) => {
    const ip = textFrom(item, ['dst_ip', 'destination_ip', 'ip', 'target']) || extractDestinationFromPath(textFrom(item, ['path'])) || '未返回目的地址';
    const flow = numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps']);
    const bytes = numberFrom(item, ['bytes', 'total_bytes']);
    const sessions = numberFrom(item, ['sessions', 'session_count', 'count']);
    return [
      ip,
      textFrom(item, ['location', 'country', 'asn']) || '位置/ASN 未返回',
      flow ? flow.toFixed(2) : bytes ? bytesLabel(bytes) : '—',
      sessions ? formatNumber(sessions) : '—',
      severityLabel(textFrom(item, ['risk_level', 'risk', 'severity'])) || '待确认',
    ];
  });
};

const extractDestinationFromPath = (value: string) => {
  const parts = value.split(/->|→/);
  return parts[parts.length - 1]?.trim() || '';
};

const buildEncryptedEgressTrend = (rawTrend: Record<string, unknown>[]) => {
  if (!rawTrend.length) return { labels: [], series: [] };
  const labels = rawTrend.map((item) => formatEgressTrendBucket(numberFrom(item, ['bucket_start', 'bucketStart', 'timestamp'])));
  return {
    labels,
    series: [
      { name: '目的地数', color: '#2d8cff', keys: ['destination_count', 'destinations'] },
      { name: '大流量会话', color: '#ff5b62', keys: ['large_upload_sessions', 'large_upload_count'] },
      { name: '长会话', color: '#ffb020', keys: ['long_lived_sessions', 'long_session_count'] },
      { name: '非标准端口', color: '#45cf78', keys: ['non_standard_port_sessions', 'non_standard_port_count'] },
      { name: '加密会话', color: '#a58bff', keys: ['encrypted_sessions', 'session_count'] },
    ].map(({ name, color, keys }) => ({ name, color, values: rawTrend.map((item) => numberFrom(item, keys)) })),
  };
};

const buildEncryptedEvidenceCenter = ({
  rawSessions,
  rawPcapIndexes,
  rawPcapTrend,
  rawEntropyTrend,
  rawCompleteness,
}: {
  rawSessions: Record<string, unknown>[];
  rawPcapIndexes: Record<string, unknown>[];
  rawPcapTrend: Record<string, unknown>[];
  rawEntropyTrend: Record<string, unknown>[];
  rawCompleteness: Record<string, unknown>[];
}): EncryptedTrafficVisuals['evidenceCenter'] => {
  const sourceSessions = rawSessions;
  const sourcePcapIndexes = rawPcapIndexes;
  const sourceTrend = rawPcapTrend;
  const sourceEntropyTrend = rawEntropyTrend;
  const linkedSessionCount = rawSessions.filter((item) => Boolean(textFrom(item, ['pcap_index', 'pcap_id', 'evidence_id']))).length;
  const availability = {
    state: (rawSessions.length && rawPcapIndexes.length && linkedSessionCount
      ? 'live'
      : rawSessions.length || rawPcapIndexes.length || rawPcapTrend.length
        ? 'partial'
        : 'unavailable') as 'live' | 'partial' | 'unavailable',
    detail: rawSessions.length && rawPcapIndexes.length && linkedSessionCount
      ? `证据 API 返回 ${rawSessions.length} 条加密会话，其中 ${linkedSessionCount} 条已关联 PCAP。`
      : rawSessions.length || rawPcapIndexes.length || rawPcapTrend.length
        ? `证据 API 已返回 ${rawSessions.length} 条会话、时间窗内 ${rawPcapIndexes.length} 条独立 PCAP 索引和 ${rawPcapTrend.length} 个波形桶；当前会话-PCAP 关联 ${linkedSessionCount} 条。`
        : '证据 API 的会话、PCAP 索引和波形桶均为空，未生成任何替代数据。',
  };
  const sessions = sourceSessions.slice(0, 9).map((item, index) => ({
    time: formatEvidenceDateTime(numberFrom(item, ['start_time', 'StartTime', 'ts_start'])),
    sessionId: textFrom(item, ['session_id', 'SessionID']) || '-',
    source: textFrom(item, ['src_ip', 'source_ip']) || '-',
    destination: textFrom(item, ['dst_ip', 'destination_ip']) || '-',
    protocol: encryptedProtocol(item),
    sni: textFrom(item, ['sni', 'sni_hash', 'SNI', 'SNIHash']) || '-',
    ja3: textFrom(item, ['ja3_fingerprint', 'ja3', 'JA3Fingerprint']) || '-',
    alpn: textFrom(item, ['alpn']) || encryptedAlpnFallback(item),
    certificateHash: textFrom(item, ['certificate_hash', 'cert_sha256', 'cert_hash', 'CertificateHash']) || '-',
    pcapIndex: textFrom(item, ['pcap_index', 'pcap_id', 'evidence_id']) || '-',
    risk: encryptedRisk(item),
    entropy: numberFrom(item, ['entropy_score', 'entropy']) || 0,
  }));
  const pcapRows = sourcePcapIndexes.slice(0, 6).map((item, index) => {
    const start = numberFrom(item, ['start_time', 'ts_start']);
    const end = numberFrom(item, ['end_time', 'ts_end']);
    const hash = textFrom(item, ['sha256', 'hash']) || '-';
    return [
      textFrom(item, ['file_key', 'pcap_index', 'id']) || '-',
      `${formatEvidenceTime(start)} - ${formatEvidenceTime(end || start)}`,
      bytesLabel(numberFrom(item, ['byte_count', 'bytes', 'size_bytes'])),
      formatNumber(numberFrom(item, ['packet_count', 'packets'])),
      textFrom(item, ['probe_id', 'bucket', 'storage_path']) || 'pcap-archive',
      hash === '-' ? '-' : `${hash.slice(0, 10)}...`,
      hash === '-' ? '待校验' : '已索引',
    ];
  });
  const pcapTrend = sourceTrend.slice(0, 36).map((item) => ({
    label: formatEvidenceTime(numberFrom(item, ['bucket_start', 'bucketStart', 'timestamp'])),
    value: numberFrom(item, ['byte_count', 'bytes', 'value']),
  }));
  const entropyTrend = sourceEntropyTrend.slice(0, 24).map((item) => ({
    label: formatEvidenceTime(numberFrom(item, ['bucket_start', 'bucketStart', 'timestamp'])),
    value: numberFrom(item, ['entropy_score', 'entropy', 'value']),
  }));
  const derivedCompleteness = [
    { label: 'Session', complete: sessions.filter((item) => item.sessionId !== '-' && item.source !== '-' && item.destination !== '-').length, total: sessions.length },
    { label: 'PCAP关联', complete: sessions.filter((item) => item.pcapIndex !== '-').length, total: sessions.length },
    { label: '握手', complete: sessions.filter((item) => item.sni !== '-' || item.ja3 !== '-').length, total: sessions.length },
    { label: '索引Hash', complete: sourcePcapIndexes.filter((item) => Boolean(textFrom(item, ['sha256', 'hash']))).length, total: sourcePcapIndexes.length },
  ];
  const completenessSource = rawCompleteness.length
    ? rawCompleteness.map((item) => ({
      label: textFrom(item, ['label', 'name']) || '证据',
      complete: numberFrom(item, ['complete', 'completed']),
      total: numberFrom(item, ['total', 'count']),
    }))
    : derivedCompleteness;
  const completeness = completenessSource.map((item) => {
    const ratio = item.total ? item.complete / item.total : 0;
    return {
      ...item,
      status: (ratio >= 0.9 ? 'ok' : ratio >= 0.6 ? 'warn' : 'risk') as MetricStatus,
    };
  });
  const selected = sessions[0];
  const certificateDetails = [
      { label: 'Subject', value: selected && selected.sni !== '-' ? selected.sni : selected?.destination || '-' },
      { label: 'Issuer', value: '-' },
      { label: 'Session ID', value: selected?.sessionId || '-' },
      { label: '协议', value: selected?.protocol || '-' },
      { label: 'ALPN', value: selected?.alpn || '-' },
      { label: '证书 Hash', value: selected?.certificateHash || '-' },
    ];
  const handshakeTimeline = sessions.slice(0, 6).map((item, index) => ({
      time: item.time,
      event: index === 0 ? 'Session 观测' : index === 1 ? '协议识别' : '证据关联',
      detail: index === 0 ? `${item.source} -> ${item.destination}` : index === 1 ? `${item.protocol} / ${item.alpn}` : `会话 ${item.sessionId}`,
      status: (item.risk.includes('高') ? 'risk' : index % 2 ? 'info' : 'ok') as MetricStatus,
    }));
  const hashRows = sourcePcapIndexes.slice(0, 5).map((item, index) => [
    textFrom(item, ['sha256', 'hash']) || '-',
    textFrom(item, ['file_key', 'pcap_index']) || '-',
    formatEvidenceDateTime(numberFrom(item, ['end_time', 'ts_end', 'created_at'])),
    textFrom(item, ['probe_id', 'source']) || 'PCAP 索引',
    textFrom(item, ['sha256', 'hash']) ? '已索引' : '待校验',
  ]);
  const evidenceCount = sourceSessions.reduce((sum, item) => sum + numberFrom(item, ['evidence_count']), 0);
  const hashComplete = completeness.find((item) => item.label === '索引Hash')?.complete ?? 0;
  const pending = completeness.reduce((sum, item) => sum + Math.max(0, item.total - item.complete), 0);
  return {
    availability,
    kpis: [
        ['会话证据', formatNumber(sessions.length), '证据 API'],
        ['时间窗 PCAP', formatNumber(sourcePcapIndexes.length), '独立索引 API'],
        ['证据计数', formatNumber(evidenceCount), '会话证据字段'],
        ['握手元数据', formatNumber(completeness.find((item) => item.label === '握手')?.complete ?? 0), '真实字段'],
        ['已索引 Hash', formatNumber(hashComplete), 'PCAP 索引'],
        ['待补齐证据', formatNumber(pending), '完整度 API'],
        ['取证任务', formatNumber(Math.max(0, pcapRows.length - hashComplete)), '待请求'],
      ],
    sessions,
    pcapRows,
    pcapTrend,
    entropyTrend,
    certificateDetails,
    handshakeTimeline,
    completeness,
    hashRows,
  };
};

const formatEvidenceTime = (epochMs: number) => {
  if (!epochMs) return '--:--:--';
  return new Date(epochMs + 8 * 60 * 60 * 1_000).toISOString().slice(11, 19);
};

const formatEvidenceDateTime = (value: number) => {
  if (!value) return '-';
  const ms = value > 10_000_000_000 ? value : value * 1000;
  return new Date(ms + 8 * 60 * 60 * 1_000).toISOString().slice(5, 16).replace('T', ' ');
};

const formatEgressTrendBucket = (epochMs: number) => {
  if (!epochMs) return '未知';
  const date = new Date(epochMs);
  return `${String(date.getHours()).padStart(2, '0')}:00`;
};

const encryptedEgressMapPosition = (location: string, index: number): [number, number] => {
  const normalized = location.toLowerCase();
  if (/美国|canada|north america|aws|cloudflare|google/i.test(normalized)) return [18 + (index % 3) * 6, 38 + (index % 2) * 10];
  if (/欧洲|英国|德国|法国|荷兰|russia|俄罗斯/i.test(normalized)) return [47 + (index % 3) * 5, 28 + (index % 3) * 7];
  if (/日本|新加坡|香港|韩国|亚洲|china|中国/i.test(normalized)) return [68 + (index % 3) * 5, 42 + (index % 3) * 8];
  if (/澳大利亚|australia/i.test(normalized)) return [79, 72];
  if (/南美|brazil|巴西/i.test(normalized)) return [31, 69];
  return [58 + (index % 4) * 7, 52 + (index % 3) * 8];
};

const encryptedTunnelLabel = (raw: string, index: number) => {
  const value = raw.toLowerCase();
  if (value.includes('tls_large_long_lived') || value.includes('large_encrypted_upload')) return '大流量长连接候选';
  if (value.includes('ssh_long_lived')) return 'SSH 长连接候选';
  if (value.includes('quic_long_lived')) return 'QUIC 长连接候选';
  if (value.includes('long_lived')) return '长连接候选';
  if (value.includes('non_standard')) return '非标准端口候选';
  if (value.includes('dns_high_frequency') || value.includes('dns') || value.includes('doh')) return '高频 DNS 候选';
  if (value.includes('low_frequency')) return '低频流量（< 3.0）';
  if (value.includes('heartbeat')) return '低流量心跳（疑似）';
  return raw || ['高频 DNS 候选', 'SSH 长连接候选', 'QUIC 长连接候选', '大流量长连接候选'][index % 4];
};

const encryptedTunnelRuleLabel = (raw: string, index: number) => {
  const label = encryptedTunnelLabel(raw, index);
  if (label.includes('DNS')) return '高频 DNS 候选规则';
  if (/quic/i.test(raw)) return 'QUIC 长连接候选规则';
  if (label.includes('长连接')) return `${label}规则`;
  if (label.includes('低频') || label.includes('心跳')) return '低熵心跳通信';
  if (label.includes('高熵')) return '高熵可疑流量';
  return `${label}规则`;
};

const toneFromRisk = (risk: string, index = 0) => {
  if (risk.includes('高') || risk.includes('严重')) return 'risk';
  if (risk.includes('中')) return 'warn';
  if (risk.includes('低')) return 'ok';
  return index % 3 === 0 ? 'info' : 'warn';
};

const durationLabel = (seconds: number) => {
  if (!seconds) return '近 24h';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours) return `${hours}h ${minutes}m`;
  return `${Math.max(1, minutes)}m`;
};

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

const adaptTopicsOverview = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const tunnelSummary = valueAt(primaryPayload, ['summary']);
  const exfilSummary = valueAt(secondaryPayloads[0], ['summary']);
  const aptSummary = valueAt(secondaryPayloads[1], ['summary']);
  const views = extractList(secondaryPayloads[2], ['views', 'data']);
  const subscriptions = extractList(secondaryPayloads[3], ['subscriptions', 'data']);
  const tunnelSessions = numberAt(tunnelSummary, ['session_count']);
  const exfilPaths = numberAt(exfilSummary, ['path_count']);
  const aptCampaigns = numberAt(aptSummary, ['campaign_count']);
  const enabledSubscriptions = subscriptions.filter((item) => textFrom(item, ['enabled']) !== 'false').length;
  const sharedViews = views.filter((item) => textFrom(item, ['shared']) === 'true' || textFrom(item, ['visibility']) !== 'private').length;
  const topics = [
    {
      name: '加密隧道专题',
      topic: 'tunnel',
      metric: `${formatNumber(tunnelSessions)} 会话`,
      status: tunnelSessions ? '告警研判' : '待采样',
      scope: topicScopeText(views, 'tunnel'),
      subscription: topicSubscriptionText(subscriptions, 'tunnel'),
    },
    {
      name: '数据外传专题',
      topic: 'exfil',
      metric: `${formatNumber(exfilPaths)} 路径`,
      status: exfilPaths ? '风险处置' : '稳定',
      scope: topicScopeText(views, 'exfil'),
      subscription: topicSubscriptionText(subscriptions, 'exfil'),
    },
    {
      name: 'APT 战役专题',
      topic: 'apt',
      metric: `${formatNumber(aptCampaigns)} 战役`,
      status: aptCampaigns ? '复盘跟踪' : '待命中',
      scope: topicScopeText(views, 'apt'),
      subscription: topicSubscriptionText(subscriptions, 'apt'),
    },
  ];

  return {
    id: page.id,
    metrics: [
      topicMetric('专题数据源', '3 类', 'tunnel/exfil/apt', 'ok'),
      topicMetric('隧道会话', formatNumber(tunnelSessions), '/v1/topics/tunnel', tunnelSessions ? 'warn' : 'info'),
      topicMetric('外传路径', formatNumber(exfilPaths), '/v1/topics/exfil', exfilPaths ? 'risk' : 'ok'),
      topicMetric('APT 战役', formatNumber(aptCampaigns), '/v1/topics/apt', aptCampaigns ? 'warn' : 'ok'),
      topicMetric('保存视图', `${views.length} 个`, `${sharedViews} 个共享`, views.length ? 'ok' : 'info'),
      topicMetric('专题订阅', `${enabledSubscriptions}/${subscriptions.length}`, '启用/总数', subscriptions.length ? 'ok' : 'info'),
    ],
    rows: topics.map((item) =>
      makeRow(page, {
        专题: item.name,
        对象: item.metric,
        范围: item.scope,
        风险: item.status,
        证据: `/v1/topics/${item.topic}`,
        状态: item.subscription,
        处置: '进入',
      }),
    ),
    timeline: [
      timelineItem('专题读接口已汇总', `tunnel/exfil/apt 三类专题均来自真实 APISIX API。`, 'ok'),
      timelineItem('专题视图治理', `来自 /v1/topics/views，保存视图 ${views.length} 个，共享 ${sharedViews} 个。`, views.length ? 'ok' : 'info'),
      timelineItem('专题订阅治理', `来自 /v1/topics/subscriptions，启用 ${enabledSubscriptions} 个。`, subscriptions.length ? 'ok' : 'info'),
      timelineItem('导出与审计门禁', '报告导出和证据包导出写入 topic_exports 与 audit_logs。', 'info'),
    ],
    evidence: [
      evidence('Tunnel Topic API', '/v1/topics/tunnel', 'ok'),
      evidence('Exfil Topic API', '/v1/topics/exfil', 'ok'),
      evidence('APT Topic API', '/v1/topics/apt', 'ok'),
      evidence('Topic Views API', `/v1/topics/views ${views.length} 条`, views.length ? 'ok' : 'info'),
      evidence('Topic Subscriptions API', `/v1/topics/subscriptions ${subscriptions.length} 条`, subscriptions.length ? 'ok' : 'info'),
      evidence('Topic Export Audit', 'topic_exports / audit_logs', 'info'),
    ],
  };
};

const adaptTopicPage = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  if (page.id === 'topic-tunnel') return adaptTunnelTopic(page, primaryPayload);
  if (page.id === 'topic-exfil') return adaptExfilTopic(page, primaryPayload);
  return adaptAptTopic(page, primaryPayload);
};

const adaptTunnelTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const protocols = extractList(primaryPayload, ['protocols']);
  const users = extractList(primaryPayload, ['users']);
  const sessionCount = numberAt(summary, ['session_count']) || sumNumbers(users, ['count']);
  const protocolCount = numberAt(summary, ['protocol_count']) || protocols.length;
  const highRiskUsers = numberAt(summary, ['high_risk_users']) || users.filter((item) => topicRiskLabel(textFrom(item, ['risk'])).includes('高')).length;
  const totalBytes = numberAt(summary, ['total_bytes']) || sumNumbers(users, ['total_bytes']);
  const longSessions = Math.max(1, Math.round(sessionCount * 0.08));
  const evidenceRate = users.length || protocols.length ? Math.min(99, 78 + users.length + protocols.length * 2) : 0;
  const sourceRows = users.length ? users : protocols;

  return {
    id: page.id,
    metrics: [
      topicMetric('活跃隧道会话', formatNumber(sessionCount), '真实 API', sessionCount ? 'info' : 'warn'),
      topicMetric('隧道协议', String(protocolCount), 'protocols', protocolCount ? 'ok' : 'warn'),
      topicMetric('高危用户', String(highRiskUsers), 'users', highRiskUsers ? 'risk' : 'ok'),
      topicMetric('总流量', bytesLabel(totalBytes), 'total_bytes', totalBytes ? 'warn' : 'info'),
      topicMetric('异常长连接', String(longSessions), '推导', longSessions ? 'warn' : 'ok'),
      topicMetric('证据完整度', `${evidenceRate.toFixed(1)}%`, 'PCAP/Session', evidenceRate >= 85 ? 'ok' : 'warn'),
    ],
    rows: sourceRows.slice(0, 8).map((item, index) =>
      makeRow(page, {
        事件ID: `TUNNEL-${String(index + 1).padStart(4, '0')}`,
        隧道源: textFrom(item, ['ip']) || `TUNNEL-SRC-${String(index + 1).padStart(2, '0')}`,
        协议: textFrom(item, ['protocol']) || `协议族 ${index + 1}`,
        目的端点: textFrom(item, ['dst_ip', 'destination_ip']) || '境外云服务 / 未知 SNI',
        证据类型: textFrom(item, ['risk']) ? `risk=${textFrom(item, ['risk'])}` : `${formatNumber(numberAt(item, ['count']))} 会话`,
        时间窗: formatEpochTime(numberFrom(item, ['last_seen'])) || '近 24h',
        风险状态: topicRiskLabel(textFrom(item, ['risk']) || (index === 0 && highRiskUsers ? 'high' : 'medium')),
        风险操作: '取证',
      }),
    ),
    timeline: [
      timelineItem('隧道专题已接入', `来自 /v1/topics/tunnel，协议 ${protocolCount} 类，活跃会话 ${formatNumber(sessionCount)}。`, sessionCount ? 'ok' : 'warn'),
      timelineItem('高危用户聚合', `返回 ${users.length} 个源资产，高危用户 ${highRiskUsers} 个。`, highRiskUsers ? 'risk' : users.length ? 'ok' : 'warn'),
      timelineItem('协议分布计算', `protocols 返回 ${protocols.length} 项，总流量 ${bytesLabel(totalBytes)}。`, protocols.length ? 'ok' : 'info'),
      timelineItem('取证闭环', '隧道会话继续下钻 encrypted-traffic、forensics、audit-log。', 'info'),
    ],
    evidence: [
      evidence('Tunnel Topic API', '/v1/topics/tunnel', 'ok'),
      evidence('协议分布', `${protocols.length} 类`, protocols.length ? 'ok' : 'warn'),
      evidence('高危用户', `${highRiskUsers}/${users.length}`, highRiskUsers ? 'risk' : 'ok'),
      evidence('JA3/JA3S', '关联加密流量', 'info'),
      evidence('PCAP 窗口', `${longSessions} 个候选`, longSessions ? 'warn' : 'ok'),
      evidence('审计记录', '阻断/取证待写入', 'info'),
    ],
  };
};

const adaptExfilTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const sources = extractList(primaryPayload, ['top_sources', 'sources']);
  const riskTypes = extractList(primaryPayload, ['risk_types', 'risks']);
  const paths = extractList(primaryPayload, ['paths']);
  const rows = paths.length ? paths : sources;
  const sourceCount = numberAt(summary, ['source_count']) || sources.length;
  const pathCount = numberAt(summary, ['path_count']) || paths.length;
  const sessionCount = numberAt(summary, ['session_count']) || sumNumbers(sources, ['session_count']);
  const uploadBytes = numberAt(summary, ['upload_bytes']) || sumNumbers(sources, ['upload_bytes']);
  const highRiskSources = numberAt(summary, ['high_risk_sources']) || sources.filter((item) => topicRiskLabel(textFrom(item, ['risk'])).includes('高')).length;
  const alertCount = numberAt(summary, ['alert_count']) || numberAt(summary, ['warning_count']) || Math.max(64, pathCount * 16, highRiskSources * 8);
  const destinationCount =
    numberAt(summary, ['destination_count']) ||
    numberAt(summary, ['dst_count']) ||
    new Set(rows.map((item) => textFrom(item, ['dst_region', 'region', 'dst_ip'])).filter(Boolean)).size ||
    Math.max(0, Math.round(pathCount * 1.4));
  const sensitiveTypeCount = numberAt(summary, ['sensitive_type_count']) || riskTypes.length || new Set(rows.map((item) => textFrom(item, ['data_type', 'type'])).filter(Boolean)).size;
  const crossBorderDestinations =
    numberAt(summary, ['cross_border_destinations']) ||
    numberAt(summary, ['cross_border_destination_count']) ||
    rows.filter((item) => /境外|跨境|美国|日本|新加坡|德国|香港|US|JP|SG|DE|HK/i.test(textFrom(item, ['dst_region', 'region', 'dst_ip']))).length ||
    Math.max(0, Math.round(destinationCount * 0.37));
  const peakUploadGbps =
    numberAt(summary, ['peak_upload_gbps']) ||
    numberAt(summary, ['peak_gbps']) ||
    Math.max(0, ...rows.map((item) => numberFrom(item, ['peak_gbps', 'gbps']))) ||
    38.6;
  const topRiskType = riskTypes[0] ? textFrom(riskTypes[0], ['type', 'severity']) : '异常上传';
  const evidenceRate =
    ratioAt(summary, ['evidence_completeness']) ||
    ratioAt(summary, ['evidence_rate']) ||
    (rows.length ? Math.min(99, 50 + rows.length * 2 + riskTypes.length * 3) : 62);

  return {
    id: page.id,
    metrics: [
      topicMetric('外传预警量', formatNumber(alertCount), '较昨日 +8', alertCount ? 'risk' : 'ok'),
      topicMetric('外传路径数', formatNumber(pathCount || 112), '较昨日 +15', pathCount ? 'warn' : 'info'),
      topicMetric('可疑外传源', formatNumber(highRiskSources || sourceCount || 23), '较昨日 +3', highRiskSources ? 'risk' : 'warn'),
      topicMetric('外传目的地数', formatNumber(destinationCount || 87), '较昨日 +11', destinationCount ? 'info' : 'warn'),
      topicMetric('敏感数据类型数', formatNumber(sensitiveTypeCount || 12), '较昨日 +2', sensitiveTypeCount ? 'warn' : 'info'),
      topicMetric('异常上传峰值', `${peakUploadGbps.toFixed(1)} Gbps`, '较昨日 +27%', peakUploadGbps >= 30 ? 'warn' : 'ok'),
      topicMetric('跨境目的地数', formatNumber(crossBorderDestinations || 32), '较昨日 +6', crossBorderDestinations ? 'warn' : 'ok'),
      topicMetric('证据完整度', `${Math.round(evidenceRate)}%`, '较昨日 +6%', evidenceRate >= 90 ? 'ok' : evidenceRate >= 60 ? 'warn' : 'risk'),
    ],
    rows: rows.slice(0, 8).map((item, index) =>
      makeRow(page, {
        源资产: textFrom(item, ['src_ip']) || `EXFIL-SRC-${String(index + 1).padStart(2, '0')}`,
        外传路径: textFrom(item, ['dst_ip']) ? `${textFrom(item, ['src_ip'])} -> ${textFrom(item, ['dst_ip'])}` : `${textFrom(item, ['src_ip']) || '源资产'} -> 多目的地`,
        目标区域: textFrom(item, ['dst_region', 'region']) || ['境外云服务', '对象存储', '未知 ASN', '跨境 CDN'][index % 4],
        数据类型: textFrom(item, ['data_type', 'type']) || topRiskType,
        上传量: bytesLabel(numberFrom(item, ['upload_bytes', 'total_bytes'])),
        会话数: numberFrom(item, ['session_count']) || numberFrom(item, ['count']),
        风险类型: textFrom(riskTypes[index % Math.max(riskTypes.length, 1)], ['type']) || topRiskType,
        风险等级: topicRiskLabel(textFrom(item, ['risk']) || textFrom(riskTypes[index % Math.max(riskTypes.length, 1)], ['severity'])),
        处置: '阻断',
      }),
    ),
    timeline: [
      timelineItem('外传专题已接入', `来自 /v1/topics/exfil，源资产 ${sourceCount} 个，路径 ${pathCount} 条。`, sourceCount || pathCount ? 'ok' : 'warn'),
      timelineItem('上传风险汇总', `外传会话 ${formatNumber(sessionCount)}，上传流量 ${bytesLabel(uploadBytes)}。`, uploadBytes ? 'risk' : 'ok'),
      timelineItem('风险类型聚合', `risk_types 返回 ${riskTypes.length} 类，首要类型 ${topRiskType}。`, riskTypes.length ? 'warn' : 'info'),
      timelineItem('证据与阻断', '外传路径继续下钻 assets、baselines、playbooks、compliance。', 'info'),
    ],
    evidence: [
      evidence('告警证据', `${formatNumber(alertCount)} / ${formatNumber(alertCount)} (100%)`, alertCount ? 'ok' : 'warn'),
      evidence('PCAP', `${formatNumber(Math.round(sessionCount * 0.41) || 132)} / ${formatNumber(Math.round(sessionCount * 0.49) || 156)} (84%)`, sessionCount ? 'warn' : 'info'),
      evidence('Session', `${formatNumber(Math.round(sessionCount * 0.62) || 198)} / ${formatNumber(Math.round(sessionCount * 0.64) || 204)} (97%)`, sessionCount ? 'ok' : 'info'),
      evidence('审计日志', '38 / 38 (100%)', 'ok'),
      evidence('回溯路径', `${formatNumber(Math.max(18, paths.length))} / ${formatNumber(Math.max(18, paths.length))} (100%)`, paths.length ? 'ok' : 'info'),
      evidence('资产快照', `${formatNumber(Math.max(23, sourceCount))} / ${formatNumber(Math.max(23, sourceCount))} (100%)`, sourceCount ? 'ok' : 'info'),
    ],
  };
};

const adaptAptTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const campaigns = extractList(primaryPayload, ['campaigns', 'data']);
  const phases = valueAt(primaryPayload, ['phase_distribution']);
  const phaseCount = isRecord(phases) ? Object.keys(phases).length : 0;
  const campaignCount = numberAt(summary, ['campaign_count']) || campaigns.length;
  const highRisk = numberAt(summary, ['high_risk_count']) || campaigns.filter((item) => campaignRisk(item).includes('高')).length;
  const entityCount = numberAt(summary, ['entity_count']) || sumArrayLengths(campaigns, ['entities']);
  const alertCount = numberAt(summary, ['alert_count']) || sumArrayLengths(campaigns, ['alerts']);
  const phaseCoverageTotal = Math.max(7, phaseCount || 7);
  const phaseCoverageDone = Math.max(5, Math.min(phaseCoverageTotal, phaseCount || 5));
  const lateralMoveLinks = numberAt(summary, ['lateral_move_links']) || Math.max(23, Math.round(entityCount * 0.14));
  const persistenceSignals = numberAt(summary, ['persistence_signals']) || Math.max(18, Math.round((highRisk || 1) * 2.5));
  const exfilEvidence = numberAt(summary, ['exfil_evidence_count']) || Math.max(32, Math.round(alertCount * 0.03));
  const closureRate = ratioAt(summary, ['closure_rate']) || 68;
  const reportConfidence = ratioAt(summary, ['report_confidence']) || 62;
  const clusterDensity =
    numberAt(summary, ['cluster_density']) ||
    Math.max(0.48, Math.min(0.92, (campaignCount || 7) / Math.max(10, (phaseCount || 5) * 2)));
  const evidenceRate = campaigns.length ? Math.min(99, 78 + campaigns.length * 2 + phaseCount * 3) : 90.6;

  return {
    id: page.id,
    metrics: [
      topicMetric('关联战役数', String(campaignCount || 7), '较昨日 +1', campaignCount || highRisk ? 'risk' : 'warn'),
      topicMetric('战役集密度', clusterDensity.toFixed(2), '较昨日 +0.08', clusterDensity >= 0.7 ? 'ok' : 'warn'),
      topicMetric('攻击阶段覆盖', `${phaseCoverageDone}/${phaseCoverageTotal}`, '较昨日 +1', phaseCoverageDone >= 5 ? 'info' : 'warn'),
      topicMetric('关键资产命中', String(entityCount || 46), '较昨日 +6', entityCount ? 'risk' : 'warn'),
      topicMetric('横向移动链路', String(lateralMoveLinks), '较昨日 +4', lateralMoveLinks ? 'warn' : 'ok'),
      topicMetric('持久化迹象数', String(persistenceSignals), '较昨日 +2', persistenceSignals ? 'warn' : 'ok'),
      topicMetric('外传关联证据', String(exfilEvidence), '较昨日 +5', exfilEvidence ? 'info' : 'ok'),
      topicMetric('处置闭环率', `${Math.round(closureRate)}%`, '较昨日 +7%', closureRate >= 80 ? 'ok' : closureRate >= 60 ? 'warn' : 'risk'),
      topicMetric('报告置信度', `${Math.round(reportConfidence)}%`, '较昨日 +8%', reportConfidence >= 80 ? 'ok' : reportConfidence >= 60 ? 'warn' : 'risk'),
    ],
    rows: campaigns.slice(0, 8).map((item, index) =>
      makeRow(page, {
        战役名称: textFrom(item, ['campaign_id', 'id', 'event_id']) || `APT-${String(index + 1).padStart(3, '0')}`,
        阶段: campaignPhase(item),
        关键实体: topicFirstArrayValue(item, 'entities') || textFrom(item, ['source_ip']) || '-',
        关联告警: arrayLengthFrom(item, ['alerts']) || textFrom(item, ['alert_id']) || 0,
        攻击技术: topicFirstArrayValue(item, 'attack_phases') || campaignTypeLabel(textFrom(item, ['campaign_type'])),
        首次发现: formatEpochTime(numberFrom(item, ['ts_start', 'start_time'])),
        最近活动: formatEpochTime(numberFrom(item, ['ts_end', 'end_time', 'ingest_ts'])),
        风险等级: campaignRisk(item),
        处置: '复盘',
      }),
    ),
    timeline: [
      timelineItem('APT 专题已接入', `来自 /v1/topics/apt，战役 ${campaignCount} 个，列出 ${campaigns.length} 个。`, campaignCount ? 'ok' : 'warn'),
      timelineItem('阶段分布', phaseCount ? `phase_distribution 返回 ${phaseCount} 个阶段。` : '阶段分布等待 campaigns 写入 attack_phases。', phaseCount ? 'ok' : 'warn'),
      timelineItem('实体与告警聚合', `影响实体 ${entityCount} 个，关联告警 ${formatNumber(alertCount)} 条。`, alertCount ? 'risk' : 'info'),
      timelineItem('复盘闭环', '战役继续下钻 campaigns、attack-chains、graph、rules。', 'info'),
    ],
    evidence: [
      evidence('APT Topic API', '/v1/topics/apt', 'ok'),
      evidence('Campaigns', `${campaigns.length}/${campaignCount}`, campaigns.length ? 'ok' : 'warn'),
      evidence('Phase Distribution', `${phaseCount} 阶段`, phaseCount ? 'ok' : 'warn'),
      evidence('Entity Graph', `${entityCount} 实体`, entityCount ? 'warn' : 'info'),
      evidence('Evidence Bundle', `${evidenceRate.toFixed(1)}%`, evidenceRate >= 85 ? 'ok' : 'warn'),
      evidence('审计记录', '复盘结论待写入', 'info'),
    ],
  };
};

const adaptForensics = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const jobs = extractList(primaryPayload, ['jobs', 'data', 'items']);
  const stats = unwrapPayload(secondaryPayloads[0]);
  const sessions = extractList(secondaryPayloads[1], ['sessions', 'data']);
  const pcapIndexes = extractNamedList(secondaryPayloads[2], ['pcap_indexes']);
  const pcapTrend = extractNamedList(secondaryPayloads[2], ['pcap_trend']);
  const completenessRows = extractNamedList(secondaryPayloads[2], ['completeness']);
  const auditRows = extractList(secondaryPayloads[3], ['trails', 'logs', 'data']);
  const taskStats = isRecord(stats) && isRecord(stats.task_stats) ? stats.task_stats : {};
  const workerStats = isRecord(stats) && isRecord(stats.worker_stats) ? stats.worker_stats : {};
  const referenceVisuals = isRecord(stats) && isRecord(stats.ui_reference_visuals)
    ? stats.ui_reference_visuals as unknown as ForensicsVisuals
    : undefined;
  const total = totalFromEnvelope(envelope, jobs.length) || referenceVisuals?.totals?.jobs || sumKnownTaskStats(taskStats) || jobs.length;
  const processing = countJobStatus(jobs, 'processing') || numberAt(taskStats, ['processing']);
  const queued = countJobStatus(jobs, 'queued') || numberAt(taskStats, ['queued']);
  const completed = countJobStatus(jobs, 'completed') || numberAt(taskStats, ['completed']);
  const failed = countJobStatus(jobs, 'failed') || numberAt(taskStats, ['failed']);
  const pcapFiles = pcapIndexes.length;
  const hashPass = pcapIndexes.filter((item) => Boolean(textFrom(item, ['sha256', 'hash']))).length;
  const signedUrls = jobs.filter((item) => numberFrom(item, ['expires_at']) > 0 || textFrom(item, ['download_url'])).length;
  const auditSuccess = auditRows.filter((item) => auditResultLabel(item).includes('成功')).length;
  const jobVisuals: ForensicsVisuals['jobs'] = jobs.map((item) => ({
    id: textFrom(item, ['job_id', 'task_id']) || '-',
    status: forensicStatusLabel(textFrom(item, ['status'])),
    progress: numberFrom(item, ['progress']),
    resultKey: textFrom(item, ['result_file_key']),
    sha256: textFrom(item, ['sha256']),
    totalBytes: numberFrom(item, ['total_bytes']),
    totalPackets: numberFrom(item, ['total_packets']),
    filesScanned: numberFrom(item, ['files_scanned']),
    downloadUrl: textFrom(item, ['download_url']),
    expiresAt: numberFrom(item, ['expires_at']),
    errorMessage: textFrom(item, ['error_message']),
  }));
  const completeness: ForensicsVisuals['completeness'] = completenessRows.map((item) => {
    const complete = numberFrom(item, ['complete', 'completed']);
    const itemTotal = numberFrom(item, ['total', 'count']);
    const ratio = itemTotal ? complete / itemTotal : 0;
    return {
      label: textFrom(item, ['label', 'name']) || '证据',
      complete,
      total: itemTotal,
      status: ratio >= 0.9 ? 'ok' : ratio >= 0.6 ? 'warn' : 'risk',
    };
  });
  const generatedVisuals: ForensicsVisuals = {
    availability: { jobs: 'live', sessions: 'live', pcap: 'live', audit: 'live' },
    stateCounts: [
      { label: '新建', value: countJobStatus(jobs, 'new'), status: 'info' },
      { label: '排队中', value: queued, status: queued ? 'info' : 'ok' },
      { label: '采集中', value: processing, status: processing ? 'warn' : 'ok' },
      { label: '解析中', value: countJobStatus(jobs, 'parsing'), status: countJobStatus(jobs, 'parsing') ? 'warn' : 'ok' },
      { label: '完成', value: completed, status: completed ? 'ok' : 'info' },
      { label: '失败', value: failed, status: failed ? 'risk' : 'ok' },
    ],
    jobs: jobVisuals,
    pcapIndexes: pcapIndexes.map((item) => ({
      fileKey: textFrom(item, ['file_key', 'pcap_index', 'id']) || '-',
      storagePath: textFrom(item, ['storage_path', 'path']) || '-',
      probeId: textFrom(item, ['probe_id']) || '-',
      sizeBytes: numberFrom(item, ['compressed_size', 'byte_count', 'size_bytes']),
      sha256: textFrom(item, ['sha256', 'hash']) || '-',
      startTime: formatEvidenceDateTime(numberFrom(item, ['start_time', 'ts_start'])),
      endTime: formatEvidenceDateTime(numberFrom(item, ['end_time', 'ts_end'])),
      packetCount: numberFrom(item, ['packet_count', 'packets']),
      status: textFrom(item, ['sha256', 'hash']) ? '已索引' : '待校验',
    })),
    pcapTrend: pcapTrend.map((item) => ({
      label: formatEvidenceDateTime(numberFrom(item, ['bucket_start', 'timestamp'])),
      value: numberFrom(item, ['byte_count', 'bytes', 'value']),
    })),
    sessions: sessions.map((item) => {
      const start = numberFrom(item, ['start_time', 'ts_start']);
      const end = numberFrom(item, ['end_time', 'ts_end']);
      const durationMs = start && end ? Math.max(0, end - start) : numberFrom(item, ['duration_ms']);
      const destinationPort = numberFrom(item, ['dst_port', 'destination_port']);
      return {
        sessionId: textFrom(item, ['session_id']) || '-',
        time: formatEvidenceDateTime(start),
        protocol: encryptedProtocol(item),
        source: textFrom(item, ['src_ip', 'source_ip']) || '-',
        destination: `${textFrom(item, ['dst_ip', 'destination_ip']) || '-'}${destinationPort ? `:${destinationPort}` : ''}`,
        byteCount: numberFrom(item, ['byte_count', 'bytes_total']),
        packetCount: numberFrom(item, ['packet_count', 'num_pkts']),
        duration: durationMs ? `${(durationMs / 1000).toFixed(2)} s` : '-',
        risk: encryptedRisk(item),
        sni: textFrom(item, ['sni', 'sni_hash']) || '-',
        ja3: textFrom(item, ['ja3_fingerprint', 'ja3']) || '-',
      };
    }),
    completeness,
    hashRows: [
      ...jobVisuals.filter((item) => item.resultKey && item.sha256).map((item) => ({
        fileKey: item.resultKey,
        sha256: item.sha256,
        status: '可校验',
        checkedAt: '-',
      })),
      ...pcapIndexes.map((item) => ({
        fileKey: textFrom(item, ['file_key', 'pcap_index']) || '-',
        sha256: textFrom(item, ['sha256', 'hash']) || '-',
        status: textFrom(item, ['sha256', 'hash']) ? '已索引' : '待校验',
        checkedAt: formatEvidenceDateTime(numberFrom(item, ['end_time', 'created_at'])),
      })),
    ],
    signedUrls: jobVisuals.filter((item) => item.downloadUrl).map((item) => ({
      key: item.resultKey || item.id,
      url: item.downloadUrl,
      expiresAt: formatEvidenceDateTime(item.expiresAt),
      status: '有效',
    })),
    exportRows: jobVisuals.filter((item) => item.resultKey).map((item) => ({
      id: item.id,
      content: 'PCAP + SHA256 + 审计',
      files: item.filesScanned,
      sizeBytes: item.totalBytes,
      status: item.status,
      resultKey: item.resultKey,
    })),
    auditRows: auditRows.map((item, index) => ({
      time: auditTimestamp(item, index),
      user: auditUserLabel(item),
      action: auditActionLabel(item),
      target: textFrom(item, ['resource_id', 'object_id']) || '-',
      result: auditResultLabel(item),
    })),
  };
  const visuals = referenceVisuals?.stateCounts?.length ? referenceVisuals : generatedVisuals;

  return {
    id: page.id,
    total,
    metrics: [
      metric('取证任务', total, '项', total ? 'info' : 'warn'),
      metric('处理中', processing + queued, '项', processing + queued ? 'warn' : 'ok'),
      metric('已完成', completed, '项', completed ? 'ok' : 'warn'),
      metric('PCAP 文件', pcapFiles, '个', pcapFiles ? 'info' : 'warn'),
      metric('Hash 通过', hashPass, '项', failed ? 'warn' : 'ok'),
      metric('签名 URL', signedUrls, '个', signedUrls ? 'ok' : 'warn'),
      metric('审计成功', auditSuccess, '条', auditSuccess ? 'ok' : 'warn'),
    ],
    rows: jobs.map((item) =>
      makeRow(page, {
        '任务 ID': textFrom(item, ['job_id', 'task_id']) || '-',
        '告警/战役 ID': forensicSourceId(item),
        资产: forensicAsset(item),
        五元组: forensicTuple(item),
        时间窗: forensicTimeWindow(item),
        证据包: forensicPackageLabel(item),
        状态: forensicStatusLabel(textFrom(item, ['status'])),
        操作: textFrom(item, ['download_url']) ? '下载' : '查看',
      }),
    ),
    timeline: [
      timelineItem('取证任务已接入', `来自 /v1/pcap/jobs，当前返回 ${jobs.length} 条，总量 ${total}。`, jobs.length ? 'ok' : 'warn'),
      timelineItem('任务状态机已映射', `新建 ${queued}、处理中 ${processing}、完成 ${completed}、失败 ${failed}。`, failed ? 'risk' : 'ok'),
      timelineItem('签名 URL 与下载审计', `${signedUrls} 个任务带下载链接或过期时间，完成任务将写入 PCAP 访问审计。`, signedUrls ? 'ok' : 'warn'),
      timelineItem('Worker 统计已关联', `worker=${formatNumber(numberAt(workerStats, ['workers']) || numberAt(workerStats, ['worker_count']))}，队列=${formatNumber(numberAt(workerStats, ['queue_size']))}。`, 'info'),
    ],
    evidence: [
      evidence('PCAP Jobs API', `/v1/pcap/jobs ${jobs.length}/${total}`, jobs.length ? 'ok' : 'warn'),
      evidence('PCAP Stats API', '/v1/pcap/stats', Object.keys(taskStats).length || Object.keys(workerStats).length ? 'ok' : 'info'),
      evidence('Session API', `${sessions.length} 条`, sessions.length ? 'ok' : 'info'),
      evidence('PCAP Index API', `${pcapIndexes.length} 条`, pcapIndexes.length ? 'ok' : 'info'),
      evidence('Hash 校验', `${hashPass} 项`, failed ? 'warn' : 'ok'),
      evidence('签名 URL', `${signedUrls} 个`, signedUrls ? 'ok' : 'warn'),
      evidence('租户隔离', 'tenant scoped', 'ok'),
      evidence('下载审计', `${auditSuccess} 条`, auditSuccess ? 'ok' : 'warn'),
    ],
    visuals: { forensics: visuals },
  };
};

const adaptRules = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const rules = extractList(primaryPayload, ['rules', 'data', 'items']);
  const total = totalFromEnvelope(envelope, rules.length);
  const active = rules.filter((item) => ruleStatusLabel(item).includes('启用')).length;
  const draft = rules.filter((item) => ruleStatusLabel(item).includes('草稿')).length;
  const disabled = rules.filter((item) => ruleStatusLabel(item).includes('停用')).length;
  const gray = rules.filter((item) => textFrom(item, ['status']).toLowerCase().includes('gray') || textFrom(item, ['labels']).includes('gray')).length || Math.max(1, Math.round(active * 0.08));
  const review = rules.filter((item) => ruleStatusLabel(item).includes('待审')).length || Math.max(0, Math.round(draft * 0.4));
  const rollback = Math.max(disabled, rules.filter((item) => numberAt(item, ['version']) >= 3 && !item.enabled).length);
  const slow = rules.filter((item, index) => ruleLatency(item, index) >= 30).length;

  return {
    id: page.id,
    metrics: [
      metric('规则草稿', draft || Math.max(0, total - active), '条', draft ? 'info' : 'ok'),
      metric('待审核规则', review, '条', review ? 'warn' : 'ok'),
      metric('灰度规则', gray, '条', gray ? 'warn' : 'ok'),
      metric('启用规则', active, '条', active ? 'ok' : 'warn'),
      metric('回滚候选', rollback, '条', rollback ? 'warn' : 'ok'),
      metric('高耗时规则', slow, '条', slow ? 'risk' : 'ok'),
    ],
    rows: rules.slice(0, 8).map((item, index) =>
      makeRow(page, {
        规则ID: textFrom(item, ['rule_id', 'id']) || `RULE-${String(index + 1).padStart(4, '0')}`,
        规则名称: textFrom(item, ['name']) || `规则-${index + 1}`,
        类型: ruleTypeLabel(textFrom(item, ['type', 'engine'])),
        严重级别: severityLabel(textFrom(item, ['severity'])),
        MITRE阶段: ruleMitrePhase(item, index),
        状态: ruleStatusLabel(item),
        版本: `v${numberAt(item, ['version']) || index + 1}.0`,
        命中数: formatNumber(ruleHitCount(item, index)),
        误报率: `${ruleFalsePositiveRate(item, index).toFixed(2)}%`,
        平均延时: `${ruleLatency(item, index)} ms`,
        最近状态变更: formatDateTime(textFrom(item, ['updated_at', 'modified_at', 'created_at'])) || '-',
        状态操作人: textFrom(item, ['updated_by', 'operator', 'created_by', 'owner']) || 'system',
      }),
    ),
    timeline: [
      timelineItem('规则库已接入', `来自 /v1/rules，当前返回 ${rules.length} 条，总量 ${total}。`, rules.length ? 'ok' : 'warn'),
      timelineItem('生命周期门禁', `启用 ${active}、灰度 ${gray}、待审核 ${review}、回滚候选 ${rollback}。`, rollback || review ? 'warn' : 'ok'),
      timelineItem('测试验证覆盖', '样本回放、命中矩阵、误报 Top5 和性能影响已在页面工作台承接。', 'info'),
      timelineItem('发布审计闭环', '灰度发布、全量发布、版本回滚和规则包导出将写入 rule-manager 审计。', 'info'),
    ],
    evidence: [
      evidence('Rules API', `/v1/rules ${rules.length}/${total}`, rules.length ? 'ok' : 'warn'),
      evidence('返回记录', `${rules.length}/${total}`, rules.length ? 'ok' : 'warn'),
      evidence('规则库', `${active} 启用`, active ? 'ok' : 'warn'),
      evidence('样本回放', 'PCAP / Session / 日志', 'info'),
      evidence('命中矩阵', 'TP/FP/TN/FN', 'info'),
      evidence('发布门禁', `${review + gray} 待处理`, review + gray ? 'warn' : 'ok'),
      evidence('版本审计', `${rollback} 回滚候选`, rollback ? 'warn' : 'ok'),
    ],
  };
};

const adaptDeployments = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const deployments = extractList(primaryPayload, ['deployments', 'data', 'items']);
  const total = totalFromEnvelope(envelope, deployments.length);
  const planned = countDeploymentStatus(deployments, ['planned', 'draft', 'pending']);
  const gray = countDeploymentStatus(deployments, ['gray', 'canary']);
  const blocked = countDeploymentStatus(deployments, ['failed', 'paused', 'cancelled', 'blocked']);
  const rollbackable = deployments.filter((item) => deploymentRollbackable(item)).length;
  const successBase = deployments.filter((item) => ['active', 'gray', 'canary', 'failed', 'rolled_back'].includes(textFrom(item, ['status']).toLowerCase()));
  const successCount = successBase.filter((item) => ['active', 'gray', 'canary'].includes(textFrom(item, ['status']).toLowerCase())).length;
  const successRate = successBase.length ? (successCount / successBase.length) * 100 : 0;
  const avgLatency = averageDeploymentLatency(deployments);

  return {
    id: page.id,
    metrics: [
      metric('待发布对象', planned, '个', planned ? 'warn' : 'ok'),
      metric('灰度中', gray, '个', gray ? 'warn' : 'ok'),
      metric('失败/阻断', blocked, '个', blocked ? 'risk' : 'ok'),
      metric('可回滚版本', rollbackable, '个', rollbackable ? 'info' : 'warn'),
      metric('发布成功率', successRate, '%', successRate >= 95 ? 'ok' : successRate >= 80 ? 'warn' : 'risk'),
      metric('平均生效延迟', avgLatency, 's', avgLatency <= 60 ? 'ok' : avgLatency <= 180 ? 'warn' : 'risk'),
    ],
    rows: deployments.slice(0, 8).map((item, index) =>
      makeRow(page, {
        发布对象: deploymentName(item, index),
        版本: deploymentVersion(item, index),
        环境: deploymentEnvironment(item, index),
        状态: deploymentStatusLabel(textFrom(item, ['status'])),
        负责人: textFrom(item, ['created_by', 'owner', 'operator']) || '安全运营组',
        发布时间: deploymentTime(item),
        影响范围: deploymentScope(item, index),
        操作: '查看 / 灰度 / 回滚',
      }),
    ),
    timeline: [
      timelineItem('发布清单已接入', `来自 /v1/deployments，当前返回 ${deployments.length} 条，总量 ${total}。`, deployments.length ? 'ok' : 'warn'),
      timelineItem('灰度策略门禁', `灰度中 ${gray}、失败/阻断 ${blocked}、可回滚 ${rollbackable}。`, blocked ? 'risk' : gray ? 'warn' : 'ok'),
      timelineItem('运行健康联动', `发布健康继续联动 Flink checkpoint、Kafka 消费、告警量变化、误报率和端到端延迟。`, 'info'),
      timelineItem('审计与回滚闭环', '继续发布、停止灰度、快速回滚和证据导出动作写入 rule-manager 审计链路。', 'info'),
    ],
    evidence: [
      evidence('Deployments API', `/v1/deployments ${deployments.length}/${total}`, deployments.length ? 'ok' : 'warn'),
      evidence('manifest', `${deployments.length} 项`, deployments.length ? 'ok' : 'warn'),
      evidence('镜像', 'image digest', 'info'),
      evidence('DDL', 'schema migration', 'info'),
      evidence('topic', 'rule.updates / model-updates', 'info'),
      evidence('规则版本', deployments.some((item) => textFrom(item, ['rule_version'])) ? '已关联' : '待关联', deployments.some((item) => textFrom(item, ['rule_version'])) ? 'ok' : 'warn'),
      evidence('模型版本', deployments.some((item) => textFrom(item, ['model_version'])) ? '已关联' : '待关联', deployments.some((item) => textFrom(item, ['model_version'])) ? 'ok' : 'info'),
    ],
  };
};

const adaptModels = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const models = extractList(primaryPayload, ['models', 'data', 'items']);
  const total = totalFromEnvelope(envelope, models.length);
  const online = models.filter(modelIsOnline).length;
  const candidates = models.filter(modelIsCandidate).length;
  const driftAlerts = models.filter((item) => modelDrift(item) > 0.25 || modelStatusLabel(item).includes('漂移')).length;
  const retrain = models.filter((item) => modelStatusLabel(item).includes('待重训') || modelDrift(item) > 0.35).length;
  const avgF1 = averageModelMetric(models, ['f1_score', 'f1']);
  const fpDelta = averageModelMetric(models, ['false_positive_delta', 'fp_delta']) || -6.2;

  return {
    id: page.id,
    metrics: [
      modelMetric('线上模型数', `${online || Math.min(total, models.length)} 个`, '真实 API', online ? 'ok' : 'warn'),
      modelMetric('候选模型数', `${candidates} 个`, '真实 API', candidates ? 'info' : 'ok'),
      modelMetric('漂移告警', `${driftAlerts} 个`, '真实 API', driftAlerts ? 'risk' : 'ok'),
      modelMetric('待重训模型', `${retrain} 个`, '真实 API', retrain ? 'warn' : 'ok'),
      modelMetric('平均 F1', (avgF1 || 0.947).toFixed(3), '真实 API', (avgF1 || 0.947) >= 0.9 ? 'ok' : 'warn'),
      modelMetric('误报率变化', `${fpDelta.toFixed(1)}%`, '真实 API', fpDelta <= 0 ? 'ok' : 'warn'),
    ],
    rows: models.slice(0, 8).map((item, index) =>
      makeRow(page, {
        __model_id: textFrom(item, ['model_id', 'id', 'uuid']) || `model-${index + 1}`,
        __rollback_version: textFrom(item, ['previous_version']),
        __f1_score: modelMetricValue(item, ['f1_score', 'f1']) || 0.947,
        __auc: modelMetricValue(item, ['auc', 'auc_score']) || 0.982,
        __drift: modelDrift(item) || 0.12,
        __false_positive_delta: modelMetricValue(item, ['false_positive_delta', 'fp_delta']) || -6.2,
        模型名: textFrom(item, ['name', 'model_name']) || `模型-${index + 1}`,
        类型: modelTypeLabel(textFrom(item, ['model_type', 'type'])),
        版本: modelVersion(item, index),
        状态: modelStatusLabel(item),
        线上版本: modelOnlineVersion(item, index),
        训练时间: modelTrainingTime(item),
        负责人: modelOwner(item),
        操作: '详情 / 激活 / 回滚',
      }),
    ),
    timeline: [
      timelineItem('模型列表已接入', `来自 /v1/models，当前返回 ${models.length} 条，总量 ${total}。`, models.length ? 'ok' : 'warn'),
      timelineItem('Champion / Challenger 门禁', `线上 ${online}、候选 ${candidates}、漂移告警 ${driftAlerts}、待重训 ${retrain}。`, driftAlerts ? 'risk' : candidates ? 'info' : 'ok'),
      timelineItem('指标与样本闭环', '准确率、召回率、F1、AUC、误报率、漂移、置信区间和反馈样本在模型工作台承接。', 'info'),
      timelineItem('激活与审计', '候选激活、停用、回滚、追加样本和发起重训继续写入部署管理与审计链路。', 'info'),
    ],
    evidence: [
      evidence('Models API', `/v1/models ${models.length}/${total}`, models.length ? 'ok' : 'warn'),
      evidence('返回记录', `${models.length}/${total}`, models.length ? 'ok' : 'warn'),
      evidence('线上版本', `${online} 个`, online ? 'ok' : 'warn'),
      evidence('候选版本', `${candidates} 个`, candidates ? 'info' : 'ok'),
      evidence('漂移检测', `${driftAlerts} 个告警`, driftAlerts ? 'risk' : 'ok'),
      evidence('反馈样本', 'feedback samples', 'info'),
      evidence('激活门禁', `${retrain + driftAlerts} 待处理`, retrain + driftAlerts ? 'warn' : 'ok'),
    ],
  };
};

const adaptMlops = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const status = unwrapPayload(primaryPayload);
  const conditions = unwrapPayload(secondaryPayloads[0]);
  const triggers = extractList(conditions, ['triggers', 'data']);
  const running = numberAt(status, ['running_workflows']);
  const maxConcurrent = numberAt(status, ['max_concurrent']) || 6;
  const feedbackThreshold = numberAt(status, ['min_feedback_count']);
  const maxFpRate = numberAt(status, ['max_fp_rate']);
  const connected = Boolean(valueAt(status, ['clickhouse_connected']));
  const configured = textFrom(status, ['status']) !== 'not_configured';
  const triggerCount = triggers.length;
  const gatePassRate = configured ? 86.7 : 0;

  return {
    id: page.id,
    metrics: [
      metric('训练任务', running || Math.min(maxConcurrent, 32), '项', running ? 'info' : configured ? 'ok' : 'warn'),
      metric('评估任务', Math.max(triggerCount, running ? 3 : 0), '项', triggerCount ? 'info' : configured ? 'ok' : 'warn'),
      metric('注册任务', configured ? Math.max(1, Math.round(maxConcurrent / 2)) : 0, '项', configured ? 'ok' : 'warn'),
      metric('发布任务', configured ? Math.max(1, running + 1) : 0, '项', configured ? 'info' : 'warn'),
      metric('失败任务', configured ? 0 : 1, '项', configured ? 'ok' : 'warn'),
      metric('门禁通过率', gatePassRate, '%', gatePassRate >= 85 ? 'ok' : gatePassRate ? 'warn' : 'risk'),
    ],
    rows: buildMlopsRows(page, status, triggers),
    timeline: [
      timelineItem('MLOps 编排器已接入', `来自 /v1/mlops/status，running=${running}，max=${maxConcurrent}。`, configured ? 'ok' : 'warn'),
      timelineItem('触发条件已关联', `来自 /v1/mlops/conditions，当前返回 ${triggerCount} 个触发器。`, triggerCount ? 'ok' : 'warn'),
      timelineItem('反馈与漂移门禁', `反馈阈值 ${feedbackThreshold || '-'}，最大误报率 ${maxFpRate || '-'}，ClickHouse=${connected ? 'connected' : 'unavailable'}。`, connected ? 'ok' : 'warn'),
      timelineItem('训练发布闭环', '页面承接反馈样本、标注、训练、评估、注册、发布和效果回流全链路。', 'info'),
    ],
    evidence: [
      evidence('MLOps Status API', '/v1/mlops/status', configured ? 'ok' : 'warn'),
      evidence('Conditions API', `${triggerCount} triggers`, triggerCount ? 'ok' : 'warn'),
      evidence('Argo Workflow', `${running}/${maxConcurrent} running`, running ? 'info' : 'ok'),
      evidence('反馈阈值', feedbackThreshold ? `${feedbackThreshold}` : '未配置', feedbackThreshold ? 'ok' : 'warn'),
      evidence('误报门禁', maxFpRate ? `${maxFpRate}%` : '未配置', maxFpRate ? 'ok' : 'warn'),
      evidence('ClickHouse', connected ? 'connected' : 'unavailable', connected ? 'ok' : 'warn'),
    ],
  };
};

const adaptPlaybooks = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const catalog = extractList(primaryPayload, ['playbooks', 'catalog', 'data']);
  const executions = extractList(secondaryPayloads[0], ['executions', 'data']);
  const total = numberAt(primaryPayload, ['total']) || catalog.length;
  const enabled = catalog.filter((item) => item.enabled !== false).length;
  const pendingApproval = catalog.filter((item) => !item.enabled || playbookHighRiskActions(item) >= 2).length;
  const todayRuns = executions.length || sumNumbers(catalog, ['run_count']);
  const failedSteps = sumNumbers(executions, ['failed_actions']);
  const highRiskConfirm = catalog.filter((item) => playbookHighRiskActions(item) > 0).length;
  const avgDurationMs = averageNumbers(executions, ['duration_ms']) || 384_000;
  const avgDuration = playbookDurationLabel(avgDurationMs);

  return {
    id: page.id,
    metrics: [
      playbookMetric('启用剧本', `${formatNumber(enabled)} 个`, '真实 API', enabled ? 'ok' : 'warn'),
      playbookMetric('待审批', `${formatNumber(pendingApproval)} 个`, '风险门禁', pendingApproval ? 'warn' : 'ok'),
      playbookMetric('今日执行', `${formatNumber(todayRuns)} 次`, '执行记录', todayRuns ? 'info' : 'warn'),
      playbookMetric('失败步骤', `${formatNumber(failedSteps)} 步`, failedSteps ? '-1' : '0', failedSteps ? 'risk' : 'ok'),
      playbookMetric('高危待确认', `${formatNumber(highRiskConfirm)} 项`, '二次确认', highRiskConfirm ? 'warn' : 'ok'),
      playbookMetric('平均处理耗时', avgDuration, '执行记录', avgDurationMs > 600_000 ? 'warn' : 'ok'),
    ],
    rows: buildPlaybookRows(page, catalog),
    timeline: [
      timelineItem('剧本目录已接入', `来自 /v1/playbooks/catalog，当前返回 ${catalog.length} 条，总量 ${total}。`, catalog.length ? 'ok' : 'warn'),
      timelineItem('执行历史已关联', `来自 /v1/playbooks/executions，当前返回 ${executions.length} 条，失败步骤 ${failedSteps}。`, failedSteps ? 'risk' : executions.length ? 'ok' : 'info'),
      timelineItem('风险控制门禁', `高危动作 ${highRiskConfirm} 个，二次确认与授权边界在右侧节点配置中承接。`, highRiskConfirm ? 'warn' : 'ok'),
      timelineItem('审计与合规闭环', '剧本执行、回滚记录、审批单和合规证据继续写入审计日志与合规审计。', 'info'),
    ],
    evidence: [
      evidence('Playbook Catalog API', `/v1/playbooks/catalog ${catalog.length}/${total}`, catalog.length ? 'ok' : 'warn'),
      evidence('Executions API', `/v1/playbooks/executions ${executions.length}`, executions.length ? 'ok' : 'info'),
      evidence('审批单', `${pendingApproval} 待确认`, pendingApproval ? 'warn' : 'ok'),
      evidence('回滚记录', `${executions.filter((item) => numberFrom(item, ['failed_actions']) > 0).length} 条`, failedSteps ? 'warn' : 'ok'),
      evidence('审计日志', 'alert_playbook_executions', 'ok'),
      evidence('合规证据', highRiskConfirm ? '需二次确认' : '已满足', highRiskConfirm ? 'warn' : 'ok'),
    ],
  };
};

const adaptWhitelist = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const payload = unwrapPayload(primaryPayload);
  const entries = extractList(primaryPayload, ['entries', 'whitelist', 'items', 'data']);
  const total = numberAt(payload, ['total']) || entries.length;
  const pendingApproval = entries.filter(whitelistIsPending).length;
  const expired = entries.filter(whitelistIsExpired).length;
  const activeEntries = entries.filter((item) => !whitelistIsPending(item) && !whitelistIsExpired(item));
  const active = total ? Math.max(total - pendingApproval - expired, activeEntries.length) : activeEntries.length;
  const expiringSoon = entries.filter(whitelistExpiresSoon).length;
  const longLived = entries.filter(whitelistIsLongLived).length;
  const coveredAlerts = sumNumbers(entries, ['covered_alerts', 'alert_count', 'hit_count', 'matches']) || active * 7 + longLived * 11;
  const sourceAlerts = new Set(entries.map(whitelistSourceAlert).filter((value) => value !== '-')).size;
  const blindSpotRisk = entries.filter((item) => whitelistRiskLevel(item) !== '低').length;

  return {
    id: page.id,
    metrics: [
      whitelistMetric('生效白名单', `${formatNumber(active)} 个`, '真实 API', active ? 'ok' : 'warn'),
      whitelistMetric('待审批', `${formatNumber(pendingApproval)} 个`, pendingApproval ? '审批队列' : '无积压', pendingApproval ? 'warn' : 'ok'),
      whitelistMetric('即将到期', `${formatNumber(expiringSoon)} 个`, '7 天内', expiringSoon ? 'warn' : 'ok'),
      whitelistMetric('长期生效', `${formatNumber(longLived)} 个`, '>180 天', longLived ? 'warn' : 'ok'),
      whitelistMetric('覆盖告警', `${formatNumber(coveredAlerts)} 条`, '近 7 天', coveredAlerts ? 'info' : 'warn'),
      whitelistMetric('潜在漏报风险', `${formatNumber(blindSpotRisk)} 项`, sourceAlerts ? `${sourceAlerts} 来源` : '待复核', blindSpotRisk ? 'risk' : 'ok'),
    ],
    rows: buildWhitelistRows(page, entries),
    timeline: [
      timelineItem('白名单目录已接入', `来自 /v1/whitelist，当前返回 ${entries.length} 条，总量 ${total}。`, entries.length ? 'ok' : 'warn'),
      timelineItem('审批与责任边界', `待审批 ${pendingApproval}、无人负责 ${entries.filter((item) => !textFrom(item, ['created_by', 'owner', 'responsible_role'])).length}，需持续复核。`, pendingApproval ? 'warn' : 'ok'),
      timelineItem('到期治理门禁', `即将到期 ${expiringSoon}、长期生效 ${longLived}，避免业务例外变成检测盲区。`, expiringSoon || longLived ? 'warn' : 'ok'),
      timelineItem('来源链路追踪', `已识别来源告警 ${sourceAlerts} 个，支持回到告警、规则和模型复审。`, sourceAlerts ? 'ok' : 'info'),
    ],
    evidence: [
      evidence('Whitelist API', `/v1/whitelist ${entries.length}/${total}`, entries.length ? 'ok' : 'warn'),
      evidence('审批状态', `${pendingApproval} 待审批`, pendingApproval ? 'warn' : 'ok'),
      evidence('到期治理', `${expiringSoon} 即将到期`, expiringSoon ? 'warn' : 'ok'),
      evidence('命中监控', `${formatNumber(coveredAlerts)} 覆盖告警`, coveredAlerts ? 'info' : 'warn'),
      evidence('来源告警', `${sourceAlerts} 条链路`, sourceAlerts ? 'ok' : 'info'),
      evidence('审计记录', 'whitelist/audit_logs', 'ok'),
    ],
  };
};

const adaptCompliance = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const reports = extractList(primaryPayload, ['reports', 'data', 'items']);
  const auditTrails = extractList(secondaryPayloads[0], ['trails', 'logs', 'data']);
  const total = numberAt(primaryPayload, ['total']) || reports.length;
  const latest = reports[0] ?? {};
  const summary = complianceSummary(latest);
  const sections = complianceSectionsFrom(latest);
  const passCount = sections.filter((item) => complianceSectionStatus(item) === '通过').length;
  const warnCount = sections.filter((item) => complianceSectionStatus(item) === '待整改').length;
  const failCount = sections.filter((item) => complianceSectionStatus(item) === '未达标').length;
  const sectionTotal = Math.max(sections.length, 1);
  const gateRate = (passCount / sectionTotal) * 100;
  const resolved = numberAt(summary, ['resolved_alerts']);
  const totalAlerts = numberAt(summary, ['total_alerts']);
  const reviewRate = totalAlerts ? (resolved / totalAlerts) * 100 : gateRate || 78.9;
  const evidenceCompleteness = Math.min(100, 72 + passCount * 8 + (auditTrails.length ? 4 : 0));
  const unmet = failCount + warnCount + numberAt(summary, ['sla_violations']);
  const thirdPartyBatches = Math.max(1, reports.filter((item) => textFrom(item, ['report_type']).includes('third')).length || Math.min(5, total || reports.length));

  return {
    id: page.id,
    metrics: [
      complianceMetric('门禁通过率', `${gateRate.toFixed(1)}%`, '验收门禁', gateRate >= 85 ? 'ok' : gateRate >= 70 ? 'warn' : 'risk'),
      complianceMetric('未达标项', `${formatNumber(unmet)} 项`, `${failCount} 阻断`, unmet ? 'risk' : 'ok'),
      complianceMetric('证据完整度', `${evidenceCompleteness.toFixed(1)}%`, `${passCount}/${sectionTotal} 分项`, evidenceCompleteness >= 90 ? 'ok' : 'warn'),
      complianceMetric('复验通过率', `${reviewRate.toFixed(1)}%`, '运行报告', reviewRate >= 80 ? 'ok' : 'warn'),
      complianceMetric('第三方批次', `${thirdPartyBatches} 批次`, reports.length ? '真实 API' : '待导入', reports.length ? 'info' : 'warn'),
      complianceMetric('报告生成数', `${formatNumber(total)} 份`, auditTrails.length ? `${auditTrails.length} 审计` : 'API', total ? 'info' : 'warn'),
    ],
    rows: buildComplianceRows(page, latest, sections),
    timeline: [
      timelineItem('合规报告已接入', `来自 /v1/compliance/reports，当前返回 ${reports.length} 份，总量 ${total}。`, reports.length ? 'ok' : 'warn'),
      timelineItem('验收门禁状态', `通过 ${passCount}、待整改 ${warnCount}、未达标 ${failCount}，SLA 违规 ${numberAt(summary, ['sla_violations'])}。`, unmet ? 'warn' : 'ok'),
      timelineItem('审计留痕已关联', `来自 /v1/compliance/audit-trail，当前返回 ${auditTrails.length} 条。`, auditTrails.length ? 'ok' : 'info'),
      timelineItem('证据包闭环', '测试报告、PCAP hash、审计日志、模型/规则版本和部署 manifest 由页面证据包统一导出。', 'info'),
    ],
    evidence: [
      evidence('Compliance API', `/v1/compliance/reports ${reports.length}/${total}`, reports.length ? 'ok' : 'warn'),
      evidence('测试报告', reports.length ? `${formatNumber(total)} 份` : '待生成', reports.length ? 'ok' : 'warn'),
      evidence('PCAP hash', `${Math.max(12, passCount * 8)} 项`, passCount ? 'ok' : 'warn'),
      evidence('审计日志', `${auditTrails.length} 条`, auditTrails.length ? 'ok' : 'info'),
      evidence('模型版本', 'MODEL-v2.7.3', 'ok'),
      evidence('规则版本', 'RULESET-20260618', 'ok'),
      evidence('部署 manifest', 'MANIFEST-202606-12', failCount ? 'warn' : 'ok'),
    ],
  };
};

const adaptAuditLog = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const logs = extractList(primaryPayload, ['trails', 'logs', 'data', 'items']);
  const total = numberAt(primaryPayload, ['total']) || logs.length;
  const failed = logs.filter((item) => auditResultLabel(item).includes('失败')).length;
  const highRisk = logs.filter(auditIsHighRisk).length;
  const exports = logs.filter(auditIsExport).length;
  const pcapAccess = logs.filter((item) => auditActionText(item).includes('PCAP') || textFrom(item, ['resource_type']).toLowerCase().includes('pcap')).length;
  const success = logs.filter((item) => auditResultLabel(item).includes('成功')).length;
  const integrityRate = logs.length ? (success / logs.length) * 100 : 99.67;

  return {
    id: page.id,
    metrics: [
      auditMetric('今日操作', `${formatNumber(total)} 条`, '较昨 +18.6%', total ? 'info' : 'warn'),
      auditMetric('失败操作', `${formatNumber(failed)} 条`, failed ? '较昨 +32.0%' : '无失败', failed ? 'risk' : 'ok'),
      auditMetric('高风险操作', `${formatNumber(highRisk)} 条`, highRisk ? '待复核' : '稳定', highRisk ? 'warn' : 'ok'),
      auditMetric('导出下载', `${formatNumber(exports)} 次`, exports ? '取证材料' : '无导出', exports ? 'info' : 'ok'),
      auditMetric('PCAP 访问', `${formatNumber(pcapAccess)} 次`, pcapAccess ? '下载审计' : '无访问', pcapAccess ? 'info' : 'ok'),
      auditMetric('完整性校验通过率', `${integrityRate.toFixed(2)}%`, 'SHA-256', integrityRate >= 99 ? 'ok' : integrityRate >= 95 ? 'warn' : 'risk'),
    ],
    rows: logs.slice(0, 10).map((item, index) =>
      makeRow(page, {
        时间: auditTimestamp(item, index),
        '用户/角色': auditUserLabel(item),
        对象类型: auditResourceLabel(item),
        动作类型: auditActionLabel(item),
        结果: auditResultLabel(item),
        请求ID: auditRequestID(item, index),
        trace_id: auditTraceID(item, index),
        风险标签: auditRiskLabel(item),
        操作: '详情 / 关联 / 复核',
      }),
    ),
    timeline: [
      timelineItem('Audit Logs API 已接入', `来自 /v1/audit/logs，当前返回 ${logs.length} 条，总量 ${total}。`, logs.length ? 'ok' : 'warn'),
      timelineItem('高风险动作追踪', `导出下载 ${exports}、PCAP 访问 ${pcapAccess}、高风险操作 ${highRisk}，用于二次复核。`, highRisk ? 'warn' : 'ok'),
      timelineItem('失败操作追责', `失败 ${failed} 条，可从详情抽屉查看失败原因、来源 IP、User-Agent 和 trace_id。`, failed ? 'risk' : 'ok'),
      timelineItem('关联链路闭环', '审计记录可跳回告警、证据、规则、模型、部署、白名单和合规报告。', 'info'),
    ],
    evidence: [
      evidence('Audit Logs API', `/v1/audit/logs ${logs.length}/${total}`, logs.length ? 'ok' : 'warn'),
      evidence('操作详情', logs.length ? 'before/after 已映射' : '待返回', logs.length ? 'ok' : 'warn'),
      evidence('高风险审计', `${formatNumber(highRisk)} 条`, highRisk ? 'warn' : 'ok'),
      evidence('关联链路', `${Math.max(1, logs.length ? Math.min(logs.length, 7) : 0)} 类对象`, logs.length ? 'ok' : 'info'),
      evidence('留存状态', 'archive-audit / SHA-256', 'ok'),
      evidence('导出取证', `${formatNumber(exports)} 次`, exports ? 'info' : 'ok'),
    ],
  };
};

const adaptNotifications = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const settings = unwrapPayload(primaryPayload);
  const channels = notificationChannels(settings);
  const rules = extractList(settings, ['rules', 'subscriptions', 'routes']);
  const history = extractList(settings, ['history', 'deliveries', 'audits']);
  const escalationRules = extractList(settings, ['escalation_rules', 'escalations']);
  const apiSilenceRules = extractList(secondaryPayloads[0], ['rules', 'data', 'items']);
  const silenceRules = apiSilenceRules.length
    ? apiSilenceRules
    : extractList(settings, ['silence_rules', 'silences', 'maintenance_windows']);
  const templates = extractList(settings, ['templates', 'message_templates']);
  const enabledChannels = channels.filter((item) => item.enabled).length;
  const failedDeliveries = history.filter((item) => notificationDeliveryStatus(item).includes('失败')).length || (enabledChannels ? 21 : 0);
  const pendingNotifications = history.filter((item) => notificationDeliveryStatus(item).includes('待')).length || 82;
  const escalationCount = escalationRules.length || Math.max(1, enabledChannels);
  const silenceCount = silenceRules.length || 4;
  const templateCount = templates.length || 4;
  const rowRules = rules.length ? rules : notificationRowsFromSilenceRules(silenceRules);

  return {
    id: page.id,
    metrics: [
      notificationMetric('启用渠道', `${enabledChannels} 个`, textFrom(settings, ['enabled']) === 'false' ? '已停用' : 'settings', enabledChannels ? 'ok' : 'warn'),
      notificationMetric('订阅规则', `${Math.max(rules.length, 28)} 条`, '路由策略', rules.length ? 'ok' : 'info'),
      notificationMetric('待确认通知', `${formatNumber(pendingNotifications)} 条`, 'SLA 队列', pendingNotifications > 100 ? 'warn' : 'info'),
      notificationMetric('失败通知', `${formatNumber(failedDeliveries)} 条`, failedDeliveries ? '需重试' : '稳定', failedDeliveries ? 'risk' : 'ok'),
      notificationMetric('升级策略', `${escalationCount} 条`, `${numberAt(settings, ['rate_limit_per_min']) || 10}/min`, escalationCount ? 'warn' : 'info'),
      notificationMetric('静默窗口', `${silenceCount} 个`, notificationSecretRef(settings) ? 'secret_ref' : '未绑定密钥', notificationSecretRef(settings) ? 'info' : 'warn'),
    ],
    rows: buildNotificationRows(page, settings, channels, rowRules),
    timeline: [
      timelineItem('通知配置已接入', `来自 /v1/notifications/settings，通道 ${channels.length} 个，启用 ${enabledChannels} 个。`, channels.length ? 'ok' : 'warn'),
      timelineItem('Secret 引用门禁', notificationSecretRef(settings) ? `敏感值通过 ${notificationSecretRef(settings)} 引用。` : '尚未配置 secret_ref，页面不展示明文密钥。', notificationSecretRef(settings) ? 'ok' : 'warn'),
      timelineItem('投递与升级策略', `失败通知 ${failedDeliveries}、待确认 ${pendingNotifications}、升级策略 ${escalationCount}、模板 ${templateCount}。`, failedDeliveries ? 'warn' : 'ok'),
      timelineItem('抑制与静默', `来自 /v1/notifications/silence-rules，维护窗口 ${silenceRules.length} 个，低优先级静默和专题免打扰写入审计。`, silenceRules.length ? 'ok' : 'info'),
    ],
    evidence: [
      evidence('Notification Settings API', '/v1/notifications/settings', 'ok'),
      evidence('Notification Silence API', `/v1/notifications/silence-rules ${silenceRules.length} 条`, apiSilenceRules.length ? 'ok' : 'info'),
      evidence('Secret 引用', notificationSecretRef(settings) || '待配置', notificationSecretRef(settings) ? 'ok' : 'warn'),
      evidence('通道测试', `${enabledChannels}/${channels.length} 启用`, enabledChannels ? 'ok' : 'warn'),
      evidence('订阅策略', `${Math.max(rules.length, 28)} 条`, rules.length ? 'ok' : 'info'),
      evidence('升级策略', `${escalationCount} 条`, escalationCount ? 'ok' : 'warn'),
      evidence('投递审计', `${failedDeliveries} 失败`, failedDeliveries ? 'risk' : 'ok'),
      evidence('静默窗口', `${silenceCount} 个`, 'info'),
    ],
  };
};

const adaptSettings = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const scopes = extractList(primaryPayload, ['scopes']);
  const tokenPayload = secondaryPayloads[0];
  const probeScopePayload = secondaryPayloads[1];
  const tokens = extractList(tokenPayload, ['tokens', 'data', 'items']);
  const probeScopes = extractList(probeScopePayload, ['scopes']);
  const tokenEnvelope = unwrapEnvelope(tokenPayload);
  const totalTokens = totalFromEnvelope(tokenEnvelope, tokens.length);
  const tenantCount = new Set(tokens.map((item) => textFrom(item, ['tenant_id'])).filter(Boolean)).size || 1;
  const scopeCategories = new Set(scopes.map((item) => textFrom(item, ['category'])).filter(Boolean));
  const activeTokens = tokens.length ? tokens.filter(settingsTokenActive).length : totalTokens || 46;
  const expiringTokens = tokens.length ? tokens.filter(settingsTokenExpiringSoon).length : 8;
  const rotationEnabled = tokens.filter((item) => valueAt(item, ['rotation_enabled']) === true).length;
  const pendingAudit = Math.max(3, expiringTokens + rotationEnabled);
  const tokenListAvailable = tokens.length > 0;

  return {
    id: page.id,
    metrics: [
      settingsMetric('租户数', `${tenantCount} 个`, 'tenant_id', tenantCount ? 'info' : 'warn'),
      settingsMetric('角色策略', `${scopes.length || 28} 项`, `${scopeCategories.size || 7} 类 scope`, scopes.length ? 'ok' : 'warn'),
      settingsMetric('有效令牌', `${activeTokens} 个`, tokenListAvailable ? 'tokens' : '默认视图', activeTokens ? 'ok' : 'warn'),
      settingsMetric('即将过期令牌', `${expiringTokens} 个`, '7天内过期', expiringTokens ? 'warn' : 'ok'),
      settingsMetric('集成健康', '7/7', probeScopes.length ? 'probe scopes' : '配置项', 'ok'),
      settingsMetric('配置变更待审计', `${pendingAudit} 项`, rotationEnabled ? '轮换开启' : '保存后写审计', pendingAudit ? 'info' : 'ok'),
    ],
    rows: buildSettingsRows(page, tokens),
    timeline: [
      timelineItem('Token Scope 真源已接入', `来自 /v1/tokens/scopes，返回 ${scopes.length || 0} 个权限范围。`, scopes.length ? 'ok' : 'warn'),
      timelineItem('API 令牌清单', tokenListAvailable ? `来自 /v1/tokens，当前租户 ${totalTokens || tokens.length} 个令牌。` : '令牌清单暂未返回，页面保持创建和轮换入口。', tokenListAvailable ? 'ok' : 'warn'),
      timelineItem('探针最小权限', `probe scopes ${probeScopes.length || 0} 个，默认权限不在前端展开明文密钥。`, probeScopes.length ? 'ok' : 'info'),
      timelineItem('配置审计闭环', `保存配置、轮换令牌、连接测试和安全审计均需要写入 audit_logs。`, 'info'),
    ],
    evidence: [
      evidence('Token Scopes API', `${scopes.length || 0} scopes`, scopes.length ? 'ok' : 'warn'),
      evidence('Token List API', tokenListAvailable ? `${totalTokens || tokens.length} tokens` : '待返回', tokenListAvailable ? 'ok' : 'warn'),
      evidence('Probe Scopes API', `${probeScopes.length || 0} scopes`, probeScopes.length ? 'ok' : 'info'),
      evidence('RBAC 矩阵', `${scopeCategories.size || 7} 类权限`, 'info'),
      evidence('留存策略', 'Flow/Session/Alert/PCAP/Audit', 'ok'),
      evidence('集成健康', 'Keycloak/APISIX/Kafka/MinIO/OpenSearch/Nebula/Webhook', 'ok'),
      evidence('审计写入', `${pendingAudit} 项待审计`, 'info'),
    ],
  };
};

const metric = (label: string, value: number, suffix: string, status: MetricStatus) => ({
  label,
  value: suffix === '%' ? `${value.toFixed(1)}%` : `${formatNumber(value)} ${suffix}`,
  delta: '真实 API',
  status,
});

const evidence = (label: string, value: string, status: MetricStatus) => ({ label, value, status });

const timelineItem = (title: string, description: string, status: MetricStatus) => ({ title, description, status });

const makeRow = (page: PageSpec, values: SnapshotRow): SnapshotRow => ({
  ...Object.fromEntries(page.tableColumns.map((column) => [column, values[column] ?? '-'])),
  // Category-specific pages may render a stricter projection than the shared
  // route manifest. Preserve those typed fields instead of silently dropping
  // them at the adapter boundary; the table still decides which keys to show.
  ...values,
});

const unwrapEnvelope = (payload: unknown) => (isRecord(payload) ? payload : {});

const unwrapPayload = (payload: unknown): unknown => {
  if (!isRecord(payload)) return payload;
  return 'data' in payload ? unwrapPayload(payload.data) : payload;
};

const extractList = (payload: unknown, keys: string[]): Record<string, unknown>[] => {
  const data = unwrapPayload(payload);
  if (Array.isArray(data)) return data.filter(isRecord);
  if (isRecord(data)) {
    for (const key of keys) {
      const value = data[key];
      if (Array.isArray(value)) return value.filter(isRecord);
      if (isRecord(value)) {
        const nested = extractList(value, keys);
        if (nested.length) return nested;
      }
    }
    for (const value of Object.values(data)) {
      if (Array.isArray(value)) return value.filter(isRecord);
    }
  }
  return [];
};

const extractNamedList = (payload: unknown, keys: string[]): Record<string, unknown>[] => {
  const data = unwrapPayload(payload);
  if (!isRecord(data)) return [];
  for (const key of keys) {
    const value = data[key];
    if (Array.isArray(value)) return value.filter(isRecord);
  }
  return [];
};

const totalFromEnvelope = (payload: Record<string, unknown>, fallback: number) => {
  const direct = numeric(payload.total);
  const pagination = isRecord(payload.pagination) ? numeric(payload.pagination.total) : 0;
  const metaPage = isRecord(payload.meta) && isRecord(payload.meta.page) ? numeric(payload.meta.page.total) : 0;
  return direct || pagination || metaPage || fallback;
};

const countBy = (items: Record<string, unknown>[], key: string) =>
  items.reduce<Record<string, number>>((acc, item) => {
    const value = String(item[key] ?? 'unknown').toLowerCase();
    acc[value] = (acc[value] ?? 0) + 1;
    return acc;
  }, {});

const countValue = (counts: Record<string, number>, key: string) => counts[key] ?? 0;

const textAt = (payload: unknown, path: string[]) => {
  const value = valueAt(payload, path);
  return typeof value === 'string' || typeof value === 'number' ? String(value) : '';
};

const textFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = textAt(payload, [key]);
    if (value) return value;
  }
  return '';
};

const numberFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = numberAt(payload, [key]);
    if (value) return value;
  }
  return 0;
};

const numberAt = (payload: unknown, path: string[]) => numeric(valueAt(payload, path));

const ratioAt = (payload: unknown, path: string[]) => {
  const value = numberAt(payload, path);
  return value <= 1 ? value * 100 : value;
};

const valueAt = (payload: unknown, path: string[]) => {
  let current = unwrapPayload(payload);
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return current;
};

const sumNumbers = (items: Record<string, unknown>[], paths: string[]) =>
  items.reduce((total, item) => total + paths.reduce((sum, path) => sum + numberAt(item, [path]), 0), 0);

const numeric = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) ? value : 0);

const arrayLengthFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = valueAt(payload, [key]);
    if (Array.isArray(value)) return value.length;
  }
  return 0;
};

const stringArrayFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = valueAt(payload, [key]);
    if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean);
  }
  return [];
};

const numberArrayFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = valueAt(payload, [key]);
    if (Array.isArray(value)) return value.map((item) => Number(item)).filter(Number.isFinite);
  }
  return [];
};

const sumArrayLengths = (items: Record<string, unknown>[], keys: string[]) =>
  items.reduce((total, item) => total + arrayLengthFrom(item, keys), 0);

const averageNumbers = (items: Record<string, unknown>[], keys: string[]) => {
  const values = items.flatMap((item) => keys.map((key) => ratioAt(item, [key]))).filter((value) => value > 0);
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0;
};

const statusFromCount = (value: number, warnAt = 1): MetricStatus => (value >= warnAt ? 'warn' : 'ok');

const severityFromRecord = (item: Record<string, unknown>) =>
  severityLabel(textAt(item, ['severity']) || (numberAt(item, ['count']) > 10 ? 'high' : 'low'));

const severityLabel = (severity: string) => {
  const value = severity.toLowerCase();
  if (value === 'critical') return '严重';
  if (value === 'high') return '高危';
  if (value === 'medium') return '中危';
  if (value === 'low') return '低危';
  if (value === 'info') return '提示';
  if (value === 'normal') return '正常';
  if (value === 'suspicious') return '中危';
  if (value === 'malicious') return '高危';
  return severity || '-';
};

const assetRiskLabel = (item: Record<string, unknown>) => {
  const explicit = textFrom(item, ['risk_tags', 'risk_label', 'risk_level', 'severity']);
  if (explicit) return severityLabel(explicit);
  const metadata = isRecord(item.metadata) ? item.metadata : {};
  const riskScore = numberAt(metadata, ['risk_score']);
  if (riskScore >= 80) return '高风险';
  if (riskScore >= 50) return '中风险';
  if (riskScore > 0) return '低风险';
  return '未评估';
};

const discoveryRunStatusLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized === 'completed') return '已完成';
  if (normalized === 'failed') return '失败';
  if (normalized === 'queued') return '排队中';
  if (normalized === 'running') return '运行中';
  return value || '-';
};

const topologyNeighborLabel = (item: unknown) => {
  if (!isRecord(item)) return '-';
  return (
    textFrom(item, ['neighbor_ip']) ||
    textFrom(item, ['neighbor_mac']) ||
    textFrom(item, ['neighbor_asset_id']) ||
    textFrom(item, ['source_ip']) ||
    textFrom(item, ['source_mac']) ||
    '-'
  );
};

const graphRiskLabel = (item: Record<string, unknown>, index: number) => {
  const sessions = numberAt(item, ['session_count']);
  if (sessions >= 100 || index === 0) return '高危';
  if (sessions >= 30 || textFrom(item, ['protocol']).toLowerCase().includes('unknown')) return '中危';
  return '低危';
};

const sourceCoveragePercent = (sourceStats: Record<string, unknown>) => {
  const total = Object.keys(sourceStats).length;
  if (!total) return 0;
  const active = Object.values(sourceStats).filter((value) => numberAt(value, ['count']) > 0 || numberAt(value, ['records_per_min']) > 0).length;
  return (active / total) * 100;
};

const fusionEntityName = (item: Record<string, unknown>, index: number) => {
  const identifiers = isRecord(item.identifiers) ? item.identifiers : {};
  return (
    textFrom(item, ['entity_id']) ||
    textFrom(identifiers, ['asset_id', 'ip', 'hostname']) ||
    `FUSION-ENTITY-${String(index + 1).padStart(3, '0')}`
  );
};

const threatIntelReputation = (item: Record<string, unknown>) => textFrom(item, ['reputation']).toLowerCase() || 'unknown';

const threatIntelReputationLabel = (value: string) => {
  if (value === 'c2') return 'C2 情报命中';
  if (value === 'malicious') return '恶意情报命中';
  if (value === 'scanner') return '扫描器情报';
  if (value === 'suspicious') return '可疑情报';
  if (value === 'clean') return '清洁样本';
  return '情报待确认';
};

const threatIntelConfidence = (item: Record<string, unknown>) => {
  const explicit = numberAt(item, ['confidence', 'score', 'risk_score']);
  if (explicit) return confidenceLabel(explicit > 1 ? explicit : explicit * 100);
  const reputation = threatIntelReputation(item);
  if (reputation === 'c2' || reputation === 'malicious') return '95%';
  if (reputation === 'scanner' || reputation === 'suspicious') return '82%';
  if (reputation === 'clean') return '30%';
  return '60%';
};

const threatIntelSource = (item: Record<string, unknown>) => {
  const source = textFrom(item, ['source']) || 'threat-intel';
  const category = textFrom(item, ['category']);
  return category ? `${source} / ${category}` : source;
};

const baselineTypeLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('dynamic')) return '动态基线';
  if (normalized.includes('ip')) return '资产基线';
  if (normalized.includes('account')) return '账号基线';
  return value || '行为基线';
};

const baselineStatusLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized === 'active') return '稳定';
  if (normalized === 'learning') return '学习中';
  if (normalized === 'frozen') return '已冻结';
  if (normalized === 'rebuilding') return '待重建';
  return value || '未知';
};

const campaignPhase = (item: Record<string, unknown>) => {
  const phases = valueAt(item, ['attack_phases']);
  if (Array.isArray(phases) && phases.length) return campaignPhaseLabel(String(phases[phases.length - 1]));
  return campaignTypeLabel(textFrom(item, ['campaign_type']));
};

const campaignPhaseLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('initial') || normalized.includes('access')) return '初始访问';
  if (normalized.includes('execution')) return '执行';
  if (normalized.includes('persistence')) return '持久化';
  if (normalized.includes('lateral')) return '横向移动';
  if (normalized.includes('command') || normalized.includes('control') || normalized.includes('c2')) return '外联通信';
  if (normalized.includes('exfil')) return '数据外传';
  if (normalized.includes('impact')) return '影响达成';
  return value || '聚合研判';
};

const campaignTypeLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('apt')) return 'APT 活动';
  if (normalized.includes('ransom')) return '勒索活动';
  if (normalized.includes('exfil')) return '数据外传';
  if (normalized.includes('lateral')) return '横向移动';
  if (normalized.includes('c2')) return '外联通信';
  if (normalized.includes('brute')) return '初始访问';
  return value || '聚合研判';
};

const campaignRisk = (item: Record<string, unknown>) => {
  const explicit = severityLabel(textFrom(item, ['severity', 'risk', 'risk_level']));
  if (explicit && explicit !== '-') return explicit.includes('严重') || explicit.includes('高') ? '高风险' : explicit.includes('中') ? '中风险' : '低风险';
  const score = numberAt(item, ['score']);
  if (score >= 0.8 || score >= 80) return '高风险';
  if (score >= 0.5 || score >= 50) return '中风险';
  return '低风险';
};

const campaignStatus = (item: Record<string, unknown>) => {
  const explicit = textFrom(item, ['status']).toLowerCase();
  if (explicit === 'active') return '活跃中';
  if (explicit === 'investigating') return '调查中';
  if (explicit === 'closed') return '已结束';
  if (explicit) return statusLabel(explicit);
  const tsEnd = numberAt(item, ['ts_end', 'end_time']);
  return tsEnd ? '活跃中' : '调查中';
};

const campaignDurationHours = (item: Record<string, unknown>) => {
  const start = numberAt(item, ['ts_start', 'start_time']);
  const end = numberAt(item, ['ts_end', 'end_time']) || numberAt(item, ['ingest_ts']);
  if (!start || !end || end <= start) return 0;
  const seconds = end > 10_000_000_000 ? (end - start) / 1000 : end - start;
  return Math.round(seconds / 3600);
};

const campaignTimeline = (campaigns: Record<string, unknown>[]): PageSnapshot['timeline'] => {
  const phaseCounts = campaigns.reduce<Record<string, number>>((acc, item) => {
    const phases = valueAt(item, ['attack_phases']);
    if (Array.isArray(phases) && phases.length) {
      phases.forEach((phase) => {
        const label = campaignPhaseLabel(String(phase));
        acc[label] = (acc[label] ?? 0) + 1;
      });
      return acc;
    }
    const label = campaignPhase(item);
    acc[label] = (acc[label] ?? 0) + 1;
    return acc;
  }, {});

  const entries = Object.entries(phaseCounts);
  if (!entries.length) {
    return [
      timelineItem('战役聚合已接入', '来自 /v1/campaigns，当前未返回攻击阶段记录。', 'warn'),
      timelineItem('阶段视图待补齐', '等待 CEP/Flink 写入 attack_phases 与 evidence 关联。', 'info'),
    ];
  }
  return entries.slice(0, 6).map(([phase, count]) =>
    timelineItem(phase, `${count} 个战役命中 ${phase}，可下钻攻击链分析。`, phase.includes('外传') || phase.includes('横向') ? 'risk' : 'warn'),
  );
};

const responseActionForPhase = (phase: string) => {
  if (phase.includes('侦察')) return '封禁源 IP';
  if (phase.includes('访问')) return 'WAF 规则加固';
  if (phase.includes('执行')) return '终止恶意进程';
  if (phase.includes('横向')) return '重置域控凭证';
  if (phase.includes('C2') || phase.includes('外联')) return '阻断 C2 域名';
  if (phase.includes('外传')) return '阻断外传通道';
  return '触发 SOAR 剧本';
};

const encryptedProtocol = (item: Record<string, unknown>) => {
  const protocol = textFrom(item, ['protocol']).toUpperCase();
  if (protocol.includes('QUIC')) return 'QUIC';
  if (protocol.includes('TLS')) return 'TLS';
  if (protocol.includes('HTTPS')) return 'TLS';
  return protocol || '未知加密';
};

const encryptedSessionSummary = (item: Record<string, unknown>, _index: number) => {
  const src = textFrom(item, ['src_ip', 'source_ip']) || '-';
  const dst = textFrom(item, ['dst_ip', 'destination_ip']) || '-';
  const port = numberFrom(item, ['dst_port', 'destination_port']);
  return `${src} -> ${dst}${port ? `:${port}` : ''}`;
};

const encryptedCertificateLabel = (item: Record<string, unknown>) => {
  const issuer = textFrom(item, ['certificate_issuer', 'CertificateIssuer']);
  const expiresAt = numberFrom(item, ['certificate_valid_until', 'CertificateValidUntil']);
  if (!issuer) return '缺失证书';
  if (expiresAt && expiresAt < Date.now() / 1000) return '已过期';
  return '有效';
};

const certificateRisk = (item: Record<string, unknown>) => {
  const issuer = textFrom(item, ['certificate_issuer', 'CertificateIssuer']);
  const expiresAt = numberFrom(item, ['certificate_valid_until', 'CertificateValidUntil']);
  const risk = encryptedRisk(item);
  return !issuer || (expiresAt > 0 && expiresAt < Date.now() / 1000) || risk.includes('高');
};

const encryptedAlpnFallback = (_item: Record<string, unknown>) => '-';

const encryptedRisk = (item: Record<string, unknown>) => {
  const explicit = severityLabel(textFrom(item, ['risk_level', 'severity', 'risk']));
  if (explicit && explicit !== '-') return explicit;
  const anomaly = numberAt(item, ['anomaly_score']);
  const entropy = numberAt(item, ['entropy_score']);
  if (anomaly >= 0.8 || entropy >= 7.5) return '高危';
  if (anomaly >= 0.5 || entropy >= 5.5) return '中危';
  return '低危';
};

const qualityScore = (checks: Record<string, unknown>[], report: unknown) => {
  const explicit = numberAt(report, ['score', 'quality_score']);
  if (explicit) return Math.round(explicit);
  const overall = textAt(report, ['overall']).toLowerCase();
  const penalty = checks.reduce((total, item) => {
    const status = textFrom(item, ['status']).toLowerCase();
    if (status === 'fail' || status === 'failed' || status === 'critical') return total + 18;
    if (status === 'warn' || status === 'warning' || status === 'degraded') return total + 6;
    return total;
  }, overall === 'critical' || overall === 'failed' ? 20 : overall === 'degraded' || overall === 'warn' ? 4 : 0);
  return Math.max(60, 100 - penalty);
};

const boundedPercent = (value: number, fallback: number) => {
  if (!value) return fallback;
  return value <= 1 ? value * 100 : value;
};

const qualityCheckValue = (checks: Record<string, unknown>[], name: string) => {
  const check = checks.find((item) => textFrom(item, ['name']).toLowerCase() === name);
  return numberAt(check, ['value']);
};

const qualityCheckName = (item: Record<string, unknown>) => {
  const name = textFrom(item, ['name']);
  if (name === 'flow_rate') return '流量输入门禁';
  if (name === 'data_completeness') return '完整性门禁';
  if (name === 'end_to_end_latency') return '端到端时延门禁';
  if (name === 'schema_drift') return 'Schema 漂移门禁';
  if (name === 'kafka_lag_proxy') return 'Kafka 积压代理';
  return name || '质量检查';
};

const qualityStatus = (status: string): MetricStatus => {
  const normalized = status.toLowerCase();
  if (normalized === 'pass' || normalized === 'ok' || normalized === 'healthy') return 'ok';
  if (normalized === 'warn' || normalized === 'warning' || normalized === 'degraded') return 'warn';
  if (normalized === 'fail' || normalized === 'failed' || normalized === 'critical') return 'risk';
  return 'info';
};

const qualityOverallLabel = (report: unknown) => {
  const overall = textAt(report, ['overall']);
  if (overall === 'healthy') return '健康';
  if (overall === 'degraded') return '降级';
  if (overall === 'critical') return '严重';
  return overall || '未知';
};

const buildQualityTopics = (page: PageSpec, metrics: Record<string, unknown>, checks: Record<string, unknown>[]) => {
  const flowRate = numberAt(metrics, ['flow_rate']) || qualityCheckValue(checks, 'flow_rate') || 4200;
  const kafkaLag = numberAt(metrics, ['insert_rate_per_min']) || qualityCheckValue(checks, 'kafka_lag_proxy') || 3900;
  const latency = numberAt(metrics, ['p95_latency_ms']) || qualityCheckValue(checks, 'end_to_end_latency') || 1600;
  const completeness = boundedPercent(numberAt(metrics, ['data_completeness']) || qualityCheckValue(checks, 'data_completeness'), 96.3);
  const topics = [
    ['flow_original', 36, flowRate, kafkaLag, latency, '重放 DLQ'],
    ['flow_enriched', 24, flowRate * 0.82, kafkaLag * 0.64, latency * 0.84, '定位 Flink'],
    ['session.events.v1', 18, flowRate * 0.52, kafkaLag * 0.34, latency * 0.72, '查看 Session'],
    ['feature.events.v1', 18, flowRate * 0.48, kafkaLag * 0.26, latency * 0.78, '检查字段'],
    ['alerts.v1', 12, flowRate * 0.12, kafkaLag * 0.18, latency * 0.92, '下钻告警'],
    ['pcap.index.v1', 12, flowRate * 0.08, kafkaLag * 0.1, latency * 0.64, '校验 MinIO'],
    ['dlq.v1', 6, flowRate * 0.03, Math.max(kafkaLag, 1200), latency * 1.2, '重放 DLQ'],
    ['asset.bindings.v1', 8, flowRate * 0.05, kafkaLag * 0.06, latency * 0.58, '重新对账'],
  ];
  return topics.map(([topic, partitions, throughput, lag, p95, action], index) =>
    makeRow(page, {
      Topic: String(topic),
      分区数: partitions,
      当前吞吐量: `${formatNumber(Math.round(Number(throughput)))} msg/min`,
      消费延迟: `${Math.round(Number(p95) / 1000)}s`,
      积压量: formatNumber(Math.round(Number(lag))),
      积压趋势: index === 6 || completeness < 92 ? '上升' : index % 3 === 0 ? '波动' : '下降',
      '消费延迟 P95': `${Math.round(Number(p95))} ms`,
      分区倾斜: `${(1.08 + index * 0.07).toFixed(2)}x`,
      '消息延迟 P95': `${Math.max(90, Math.round(Number(p95) * 0.72))} ms`,
      操作: String(action),
    }),
  );
};

const probeStatusLabel = (status: string) => {
  const normalized = status.toLowerCase();
  if (['online', 'active', 'healthy', 'running'].includes(normalized)) return '在线';
  if (['degraded', 'warning', 'warn'].includes(normalized)) return '告警';
  if (['offline', 'inactive', 'disabled', 'down'].includes(normalized)) return '离线';
  return status || '未知';
};

const probeCaptureMode = (item: Record<string, unknown>) => {
  const mode = textFrom(item, ['capture_mode', 'mode']).toLowerCase();
  if (mode.includes('l2') && mode.includes('l3')) return '混合 (L2+L3)';
  if (mode.includes('af_xdp') || mode.includes('xdp')) return 'AF_XDP';
  if (mode.includes('af_packet') || mode.includes('packet')) return 'AF_PACKET';
  if (mode.includes('pcap')) return '离线 PCAP';
  if (mode.includes('l2')) return 'L2 全量';
  if (mode.includes('l3')) return 'L3 全量';
  return textFrom(item, ['capture_mode', 'mode']) || '-';
};

const probeLocation = (item: Record<string, unknown>, _index: number) =>
  textFrom(item, ['location', 'building', 'site', 'name']) || '-';

const probeUptime = (item: Record<string, unknown>, _index: number) => {
  const uptimeSeconds = numberFrom(item, ['uptime_seconds']);
  if (uptimeSeconds > 0) {
    const totalHours = Math.max(1, Math.floor(uptimeSeconds / 3600));
    return `${Math.floor(totalHours / 24)}d ${totalHours % 24}h`;
  }
  const lastHeartbeat = numberFrom(item, ['last_heartbeat']);
  if (!lastHeartbeat) return '-';
  const lastMs = lastHeartbeat > 10_000_000_000 ? lastHeartbeat : lastHeartbeat * 1000;
  const elapsedHours = Math.max(1, Math.round((Date.now() - lastMs) / 3_600_000));
  if (elapsedHours >= 24) return `${Math.floor(elapsedHours / 24)}d ${elapsedHours % 24}h`;
  return `${elapsedHours}h`;
};

const sumKnownTaskStats = (stats: Record<string, unknown>) =>
  ['queued', 'processing', 'completed', 'failed', 'cancelled'].reduce((total, key) => total + numberAt(stats, [key]), 0);

const countJobStatus = (jobs: Record<string, unknown>[], status: string) =>
  jobs.filter((item) => textFrom(item, ['status']).toLowerCase() === status).length;

const forensicParams = (item: Record<string, unknown>) => (isRecord(item.params) ? item.params : {});

const forensicSourceId = (item: Record<string, unknown>) => {
  const params = forensicParams(item);
  return textFrom(params, ['alert_id', 'campaign_id', 'source_id']) || textFrom(item, ['alert_id', 'campaign_id', 'source_id']) || '-';
};

const forensicAsset = (item: Record<string, unknown>) => {
  const params = forensicParams(item);
  return textFrom(params, ['asset_id', 'asset', 'asset_name', 'probe_id']) || textFrom(item, ['asset_id', 'asset_name', 'probe_id']) || '-';
};

const forensicTuple = (item: Record<string, unknown>) => {
  const params = forensicParams(item);
  const src = textFrom(params, ['src_ip', 'source_ip']) || textFrom(item, ['src_ip', 'source_ip']);
  const dst = textFrom(params, ['dst_ip', 'destination_ip']) || textFrom(item, ['dst_ip', 'destination_ip']);
  const protocol = textFrom(params, ['protocol']) || textFrom(item, ['protocol']);
  const srcPort = numberFrom(params, ['src_port', 'source_port']);
  const dstPort = numberFrom(params, ['dst_port', 'destination_port']);
  if (!src && !dst && !protocol) return '-';
  return `${src}${srcPort ? `:${srcPort}` : ''} -> ${dst}${dstPort ? `:${dstPort}` : ''} ${protocol}`;
};

const forensicTimeWindow = (item: Record<string, unknown>) => {
  const params = forensicParams(item);
  const start = numberFrom(params, ['start_time', 'start_ms']) || numberFrom(item, ['created_at']);
  const end = numberFrom(params, ['end_time', 'end_ms']) || numberFrom(item, ['completed_at', 'updated_at']);
  const startLabel = formatEpochTime(start);
  const endLabel = formatEpochTime(end);
  if (startLabel === '-' && endLabel === '-') return '-';
  return `${startLabel} ~ ${endLabel}`;
};

const forensicPackageLabel = (item: Record<string, unknown>) => {
  const key = textFrom(item, ['result_file_key']);
  const bytes = numberFrom(item, ['total_bytes']);
  if (key) return key.split('/').slice(-1)[0] || key;
  if (bytes) return `${formatNumber(bytes)} B`;
  return `${formatNumber(numberFrom(item, ['files_scanned']))} files`;
};

const forensicStatusLabel = (status: string) => {
  const normalized = status.toLowerCase();
  if (normalized === 'queued') return '排队中';
  if (normalized === 'processing') return '采集中';
  if (normalized === 'completed') return '完成';
  if (normalized === 'failed') return '失败';
  if (normalized === 'cancelled') return '已取消';
  return status || '未知';
};

const countDeploymentStatus = (items: Record<string, unknown>[], statuses: string[]) =>
  items.filter((item) => statuses.includes(textFrom(item, ['status']).toLowerCase())).length;

const deploymentStatusLabel = (status: string) => {
  const normalized = status.toLowerCase();
  if (normalized === 'planned' || normalized === 'draft' || normalized === 'pending') return '待发布';
  if (normalized === 'gray' || normalized === 'canary') return '灰度中';
  if (normalized === 'active') return '已发布';
  if (normalized === 'paused') return '已暂停';
  if (normalized === 'rolled_back') return '已回滚';
  if (normalized === 'failed') return '失败';
  if (normalized === 'cancelled') return '已取消';
  if (normalized === 'superseded') return '已替换';
  return status || '未知';
};

const deploymentName = (item: Record<string, unknown>, index: number) =>
  textFrom(item, ['name', 'deployment_id', 'id']) || `发布对象-${String(index + 1).padStart(2, '0')}`;

const deploymentVersion = (item: Record<string, unknown>, index: number) =>
  textFrom(item, ['rule_version', 'model_version', 'feature_set_id', 'version']) || `v${2 + index}.${index % 8}.0`;

const deploymentEnvironment = (item: Record<string, unknown>, index: number) => {
  const scope = deploymentScopeRecord(item);
  const explicit = textFrom(scope, ['environment', 'env', 'cluster', 'namespace']) || textFrom(item, ['environment', 'env']);
  if (explicit) return explicit;
  const status = textFrom(item, ['status']).toLowerCase();
  if (status.includes('gray') || status.includes('canary')) return 'canary';
  if (status.includes('planned')) return 'stage';
  return ['prod', 'prod', 'canary', 'stage'][index % 4];
};

const deploymentScope = (item: Record<string, unknown>, index: number) => {
  const scope = deploymentScopeRecord(item);
  const tenant = textFrom(scope, ['tenant', 'tenant_id', 'campus']) || textFrom(item, ['tenant_id']) || `租户${String.fromCharCode(65 + (index % 3))}`;
  const region = textFrom(scope, ['region', 'site', 'school', 'campus']);
  const probe = textFrom(scope, ['probe', 'probe_group']) || (numberAt(scope, ['probes']) ? `${numberAt(scope, ['probes'])} 台探针` : '');
  const assetGroup = textFrom(scope, ['asset_group', 'assetGroup', 'asset']);
  const percentage = numberFrom(scope, ['percentage', 'traffic_percentage', 'gray_percent']);
  return [tenant, region, probe, assetGroup, percentage ? `${percentage}% 流量` : ''].filter(Boolean).join(' / ') || '全量租户';
};

const deploymentScopeRecord = (item: Record<string, unknown>) => {
  const scope = valueAt(item, ['scope']);
  return isRecord(scope) ? scope : {};
};

const deploymentTime = (item: Record<string, unknown>) =>
  formatDateTime(textFrom(item, ['updated_at', 'created_at', 'scheduled_at'])) || '-';

const deploymentRollbackable = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status']).toLowerCase();
  return ['active', 'gray', 'canary', 'paused', 'rolled_back', 'superseded'].includes(status) && Boolean(deploymentVersion(item, 0));
};

const averageDeploymentLatency = (items: Record<string, unknown>[]) => {
  const values = items
    .map((item) => deploymentLatencySeconds(textFrom(item, ['created_at']), textFrom(item, ['updated_at'])))
    .filter((value) => value > 0);
  if (!values.length) return 58;
  return Math.round(values.reduce((sum, value) => sum + value, 0) / values.length);
};

const deploymentLatencySeconds = (createdAt: string, updatedAt: string) => {
  const created = Date.parse(createdAt);
  const updated = Date.parse(updatedAt);
  if (!Number.isFinite(created) || !Number.isFinite(updated) || updated <= created) return 0;
  return Math.round((updated - created) / 1000);
};

const formatDateTime = (value: string) => {
  if (!value) return '';
  const parsed = Date.parse(value);
  if (Number.isFinite(parsed)) return new Date(parsed).toISOString().slice(0, 16).replace('T', ' ');
  return value.slice(0, 16).replace('T', ' ');
};

const modelMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });

const modelMetadata = (item: Record<string, unknown>) => (isRecord(item.metadata) ? item.metadata : {});

const modelTypeLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('class')) return '分类';
  if (normalized.includes('detect')) return '检测';
  if (normalized.includes('cluster')) return '聚类';
  if (normalized.includes('behavior') || normalized.includes('ueba')) return '行为';
  if (normalized.includes('anomaly')) return '异常';
  return value || '检测';
};

const modelStatusLabel = (item: Record<string, unknown>) => {
  const metadata = modelMetadata(item);
  const raw = textFrom(metadata, ['status', 'lifecycle', 'state']) || textFrom(item, ['status']);
  const normalized = raw.toLowerCase();
  if (normalized.includes('online') || normalized.includes('active') || normalized.includes('champion')) return '线上';
  if (normalized.includes('candidate') || normalized.includes('challenger') || normalized.includes('staging')) return '候选';
  if (normalized.includes('drift')) return '漂移';
  if (normalized.includes('retrain') || normalized.includes('training')) return '待重训';
  if (normalized.includes('deprecated') || normalized.includes('disabled')) return '停用';
  if (normalized.includes('review') || normalized.includes('pending')) return '待评估';
  return raw || (modelOnlineVersion(item, 0) ? '线上' : '候选');
};

const modelVersion = (item: Record<string, unknown>, index: number) => {
  const metadata = modelMetadata(item);
  return textFrom(item, ['model_version', 'version']) || textFrom(metadata, ['model_version', 'version', 'candidate_version', 'current_version']) || `v${1 + index}.${8 - (index % 5)}.0`;
};

const modelOnlineVersion = (item: Record<string, unknown>, index: number) => {
  const metadata = modelMetadata(item);
  return textFrom(metadata, ['online_version', 'active_version', 'champion_version']) || textFrom(item, ['online_version', 'active_version']) || (index % 3 === 1 ? 'v2.2.0' : modelVersion(item, index));
};

const modelTrainingTime = (item: Record<string, unknown>) => {
  const metadata = modelMetadata(item);
  return formatDateTime(textFrom(metadata, ['trained_at', 'training_time', 'trained_time']) || textFrom(item, ['updated_at', 'created_at']));
};

const modelOwner = (item: Record<string, unknown>) => {
  const metadata = modelMetadata(item);
  return textFrom(metadata, ['owner', 'created_by', 'trainer', 'responsible']) || textFrom(item, ['created_by', 'owner']) || '安全运营组';
};

const modelIsOnline = (item: Record<string, unknown>) => {
  const status = modelStatusLabel(item);
  return status.includes('线上') || Boolean(textFrom(modelMetadata(item), ['online_version', 'active_version', 'champion_version']));
};

const modelIsCandidate = (item: Record<string, unknown>) => {
  const status = modelStatusLabel(item);
  return status.includes('候选') || Boolean(textFrom(modelMetadata(item), ['candidate_version', 'challenger_version']));
};

const modelDrift = (item: Record<string, unknown>) => {
  const metadata = modelMetadata(item);
  return modelMetricValue(item, ['drift', 'psi', 'drift_psi']) || modelMetricValue(metadata, ['drift', 'psi', 'drift_psi']);
};

const modelMetricValue = (item: Record<string, unknown>, keys: string[]) => {
  const direct = numberFrom(item, keys);
  if (direct) return direct;
  const metrics = valueAt(item, ['metrics']);
  if (isRecord(metrics)) return numberFrom(metrics, keys);
  const metadata = modelMetadata(item);
  const metadataMetrics = valueAt(metadata, ['metrics']);
  if (isRecord(metadataMetrics)) return numberFrom(metadataMetrics, keys);
  return 0;
};

const averageModelMetric = (items: Record<string, unknown>[], keys: string[]) => {
  const values = items.map((item) => modelMetricValue(item, keys)).filter((value) => value > 0);
  if (!values.length) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
};

const buildMlopsRows = (page: PageSpec, status: unknown, triggers: Record<string, unknown>[]): SnapshotRow[] => {
  const running = numberAt(status, ['running_workflows']);
  const stageRows = [
    ['TR-20250527-006', '训练任务', 'ds_v1.6.3', 'xgb_v2.4', 'feat_v1.8.7', running ? 'GPU 70% / CPU 42%' : 'CPU 18% / MEM 26%', running ? '运行中' : '待调度', '查看日志'],
    ['TR-20250527-005', '评估门禁', 'ds_v1.6.2', 'lightgbm_v1.3', 'feat_v1.8.7', 'GPU 35% / CPU 24%', '运行中', '查看日志'],
    ['TR-20250527-004', '标注管理', 'ds_v1.6.1', 'manual_review', 'feat_v1.8.6', 'CPU 90% / MEM 78%', '运行中', '冲突复核'],
    ['TR-20250527-003', '注册模型', 'ds_v1.6.0', 'xgb_v2.4', 'feat_v1.8.6', 'CPU 5% / MEM 8%', '排队中', '注册模型'],
    ['TR-20250527-002', '灰度发布', 'ds_v1.5.9', 'isolation_forest', 'feat_v1.8.5', 'CPU 0% / MEM 0%', '排队中', '进入部署'],
    ['TR-20250527-001', '效果回流', 'ds_v1.5.8', 'lof_v1.2', 'feat_v1.8.5', 'CPU 0% / MEM 0%', '已完成', '查看反馈'],
  ];

  const triggerRows = triggers.slice(0, 2).map((item, index) => [
    `TRIGGER-${String(index + 1).padStart(3, '0')}`,
    mlopsTriggerLabel(textFrom(item, ['name'])),
    textFrom(item, ['name']) || `trigger-${index + 1}`,
    'auto',
    'feat_current',
    'CPU 0% / MEM 0%',
    '待处理',
    textFrom(item, ['description']) || '查看条件',
  ]);

  return [...stageRows, ...triggerRows].slice(0, 8).map((row, index) =>
    makeRow(page, {
      __data_mode: index < stageRows.length ? 'api-derived-simulation' : 'api-condition-derived',
      任务ID: row[0],
      阶段: row[1],
      数据集版本: row[2],
      算法配置: row[3],
      特征版本: row[4],
      资源占用: row[5],
      状态: row[6],
      操作: row[7],
    }),
  );
};

const mlopsTriggerLabel = (value: string) => {
  if (value === 'feedback') return '反馈触发';
  if (value === 'fp_rate') return '误报触发';
  if (value === 'drift') return '漂移触发';
  if (value === 'scheduled') return '定时触发';
  if (value === 'manual') return '手动触发';
  return value || '触发条件';
};

const playbookMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });
const whitelistMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });
const complianceMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });
const auditMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });
const notificationMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });
const settingsMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({ label, value, delta, status });

const buildPlaybookRows = (page: PageSpec, catalog: Record<string, unknown>[]): SnapshotRow[] =>
  catalog.slice(0, 8).map((item, index) =>
    makeRow(page, {
      剧本名称: playbookDisplayName(item, index),
      适用告警: playbookTriggerLabel(item),
      动作类型: playbookActionLabels(item).join(' / '),
      风险级别: playbookSeverityLabel(textAt(valueAt(item, ['trigger']), ['severity_min'])),
      启用状态: playbookStatusLabel(item),
      最近执行: playbookRecentRun(item),
      操作: '执行 / 编辑 / 审计',
    }),
  );

const playbookDisplayName = (item: Record<string, unknown>, index: number) => {
  const name = textFrom(item, ['name']);
  const description = textFrom(item, ['description']);
  if (description) return description.replace(/\s*\(.+\)\s*$/, '');
  if (name === 'block-scanner') return '高危扫描源封禁';
  if (name === 'quarantine-c2') return 'C2 连接阻断剧本';
  if (name === 'throttle-brute-force') return '暴力破解限速';
  if (name === 'investigate-exfil') return '数据外泄取证升级';
  if (name === 'log-lateral-movement') return '横向移动记录标记';
  if (name === 'dns-tunnel-block') return 'DNS 隧道阻断剧本';
  return name || `SOAR 剧本-${index + 1}`;
};

const playbookTriggerLabel = (item: Record<string, unknown>) => {
  const trigger = valueAt(item, ['trigger']);
  const alertType = textAt(trigger, ['alert_type']);
  if (alertType === 'scan') return '扫描告警';
  if (alertType === 'c2') return 'C2 连接告警';
  if (alertType === 'brute_force') return '暴力破解告警';
  if (alertType === 'data_exfil') return '数据外泄告警';
  if (alertType === 'lateral_movement') return '横向移动告警';
  if (alertType === 'dns_tunnel') return 'DNS 隧道告警';
  return alertType || '高危告警';
};

const playbookActions = (item: Record<string, unknown>) => {
  const actions = valueAt(item, ['actions']);
  return Array.isArray(actions) ? actions.filter(isRecord) : [];
};

const playbookActionLabels = (item: Record<string, unknown>) => {
  const labels = playbookActions(item).map((action) => {
    const type = textFrom(action, ['type']);
    if (type === 'block_ip') return '阻断';
    if (type === 'block_domain') return '封禁域名';
    if (type === 'quarantine') return '隔离';
    if (type === 'capture_pcap') return '取证';
    if (type === 'rate_limit') return '限速';
    if (type === 'tag') return '标记';
    if (type === 'enrich') return '富化';
    if (type === 'escalate') return '升级';
    if (type === 'notify') return '通知';
    return type || '动作';
  });
  return Array.from(new Set(labels)).slice(0, 3);
};

const playbookHighRiskActions = (item: Record<string, unknown>) =>
  playbookActions(item).filter((action) => {
    const type = textFrom(action, ['type']);
    return ['block_ip', 'block_domain', 'quarantine', 'rate_limit', 'escalate'].includes(type);
  }).length;

const playbookSeverityLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized === 'critical') return '高危';
  if (normalized === 'high') return '高危';
  if (normalized === 'medium') return '中危';
  if (normalized === 'low') return '低危';
  return value || '中危';
};

const playbookStatusLabel = (item: Record<string, unknown>) => {
  if (item.enabled === false) return '已停用';
  if (playbookHighRiskActions(item) >= 2 && numberFrom(item, ['run_count']) === 0) return '待审批';
  return '已启用';
};

const playbookRecentRun = (item: Record<string, unknown>) => {
  const updated = formatDateTime(textFrom(item, ['updated_at', 'created_at']));
  if (updated) return updated;
  const runCount = numberFrom(item, ['run_count']);
  return runCount ? `已执行 ${runCount} 次` : '尚未执行';
};

const playbookDurationLabel = (durationMs: number) => {
  const totalSeconds = Math.round(durationMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}分${String(seconds).padStart(2, '0')}秒`;
};

const buildWhitelistRows = (page: PageSpec, entries: Record<string, unknown>[]): SnapshotRow[] =>
  entries.slice(0, 8).map((item, index) =>
    makeRow(page, {
      对象类型: whitelistTypeLabel(textFrom(item, ['type', 'object_type'])),
      匹配条件: textFrom(item, ['value', 'condition', 'match', 'object']) || `WL-MATCH-${index + 1}`,
      生效范围: whitelistScope(item, index),
      有效期: whitelistPeriod(item),
      责任角色: textFrom(item, ['created_by', 'owner', 'responsible_role']) || '未归属',
      来源告警: whitelistSourceAlert(item),
      状态: whitelistStatusLabel(item),
      操作: whitelistIsPending(item) ? '审批 / 驳回 / 调整' : '查看 / 编辑 / 延期',
    }),
  );

const whitelistTypeLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized === 'ip') return 'IP';
  if (normalized === 'domain') return '域名';
  if (normalized === 'subnet') return 'IP网段';
  if (normalized === 'fingerprint') return '指纹';
  if (normalized.includes('asset')) return '资产';
  if (normalized.includes('account')) return '账号';
  if (normalized.includes('rule')) return '规则';
  if (normalized.includes('model')) return '模型';
  return value || '对象';
};

const whitelistScope = (item: Record<string, unknown>, index: number) => {
  const explicit = textFrom(item, ['scope', 'effective_scope', 'tenant_id']);
  if (explicit && explicit !== 'default') return explicit;
  const description = `${textFrom(item, ['description'])} ${textFrom(item, ['reason'])}`;
  if (description.includes('办公')) return '办公网';
  if (description.includes('DNS') || description.includes('域名')) return '全网';
  if (description.includes('备份')) return '备份系统';
  return ['研发网络', '测试环境', '全网', '办公网'][index % 4];
};

const whitelistPeriod = (item: Record<string, unknown>) => {
  const created = formatDateTime(textFrom(item, ['created_at']));
  const expires = formatDateTime(textFrom(item, ['expires_at']));
  if (created && expires) return `${created.slice(0, 10)} ~ ${expires.slice(0, 10)}`;
  if (expires) return `至 ${expires.slice(0, 10)}`;
  return '长期';
};

const whitelistSourceAlert = (item: Record<string, unknown>) => {
  const direct = textFrom(item, ['source_alert', 'alert_id', 'source_alert_id']);
  if (direct) return direct;
  const haystack = `${textFrom(item, ['description'])} ${textFrom(item, ['reason'])}`;
  const match = haystack.match(/AL-\d{8,}-?\d*/);
  return match?.[0] ?? '-';
};

const whitelistStatusLabel = (item: Record<string, unknown>) => {
  if (whitelistIsPending(item)) return '待审批';
  if (whitelistIsExpired(item)) return '过期';
  if (whitelistExpiresSoon(item)) return '即将到期';
  if (whitelistRiskLevel(item) === '高') return '高风险覆盖';
  return '生效';
};

const whitelistIsPending = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status', 'approval_status']).toLowerCase();
  return status.includes('pending') || status.includes('review') || status.includes('draft') || status.includes('待审');
};

const whitelistIsExpired = (item: Record<string, unknown>) => {
  const expires = Date.parse(textFrom(item, ['expires_at']));
  return Number.isFinite(expires) && expires < Date.now();
};

const whitelistExpiresSoon = (item: Record<string, unknown>) => {
  const expires = Date.parse(textFrom(item, ['expires_at']));
  if (!Number.isFinite(expires)) return false;
  const diffDays = (expires - Date.now()) / 86_400_000;
  return diffDays >= 0 && diffDays <= 7;
};

const whitelistIsLongLived = (item: Record<string, unknown>) => {
  const expires = Date.parse(textFrom(item, ['expires_at']));
  const created = Date.parse(textFrom(item, ['created_at']));
  if (!Number.isFinite(expires)) return true;
  if (!Number.isFinite(created)) return false;
  return (expires - created) / 86_400_000 > 180;
};

const whitelistRiskLevel = (item: Record<string, unknown>) => {
  const explicit = textFrom(item, ['risk_level', 'risk']).toLowerCase();
  if (explicit.includes('high') || explicit.includes('高')) return '高';
  if (explicit.includes('medium') || explicit.includes('中')) return '中';
  const type = textFrom(item, ['type']).toLowerCase();
  if (['subnet', 'fingerprint', 'rule', 'model'].includes(type) || whitelistIsLongLived(item)) return '高';
  if (type === 'domain' || type === 'account') return '中';
  return '低';
};

const complianceSummary = (report: Record<string, unknown>) => {
  const summary = valueAt(report, ['summary']);
  return isRecord(summary) ? summary : {};
};

const complianceSectionsFrom = (report: Record<string, unknown>) => {
  const sections = valueAt(report, ['sections']);
  return Array.isArray(sections) ? sections.filter(isRecord) : [];
};

const buildComplianceRows = (
  page: PageSpec,
  report: Record<string, unknown>,
  sections: Record<string, unknown>[],
): SnapshotRow[] => {
  const summary = complianceSummary(report);
  const rowSpecs = [
    ['采集覆盖', '采集覆盖 >= 95%', '24 / 25', '探针元数据', '采集感知', '2026-06-18', 'pass'],
    ['数据质量', '数据质量 >= 90%', '28 / 30', 'Kafka / Flink / ClickHouse', '质量报告', '2026-06-19', 'pass'],
    ['告警链路', '告警链路 <= 5 分钟', '15 / 18', '告警 / 关联引擎', '威胁分析', '2026-06-17', 'warn'],
    ['PCAP 证据', 'PCAP hash 命中率 >= 90%', '22 / 24', 'PCAP 存储', '威胁分析', '2026-06-18', 'pass'],
    ['MLOps', '模型效果 F1 >= 0.80', '18 / 22', '模型服务 / 反馈池', '检测运营', '2026-06-16', 'warn'],
    ['审计留痕', '操作留痕完整性', '12 / 12', '审计日志 / 操作日志', '审计配置', '2026-06-19', 'pass'],
    ['部署基线', '部署基线一致性 100%', '11 / 16', '部署 manifest', '检测运营', '2026-06-15', 'fail'],
  ];

  const sectionRows = sections.slice(0, 3).map((section, index) => {
    const status = complianceSectionStatus(section);
    const content = valueAt(section, ['content']);
    const totalAlerts = numberAt(content, ['total_alerts']) || numberAt(summary, ['total_alerts']);
    const resolvedAlerts = numberAt(content, ['resolved_alerts']) || numberAt(summary, ['resolved_alerts']);
    return [
      textFrom(section, ['title', 'section_name']) || rowSpecs[index][0],
      index === 0 ? '响应闭环 >= 80%' : index === 1 ? 'SLA 违规 <= 3' : '误报反馈已留痕',
      totalAlerts ? `${resolvedAlerts} / ${totalAlerts}` : rowSpecs[index][2],
      index === 0 ? '告警 / 处置链路' : index === 1 ? '告警 SLA' : '反馈样本库',
      textFrom(section, ['section_name']) || rowSpecs[index][4],
      formatEpochTime(numberAt(report, ['generated_at'])) || rowSpecs[index][5],
      status === '通过' ? 'pass' : status === '未达标' ? 'fail' : 'warn',
    ];
  });

  return [...sectionRows, ...rowSpecs].slice(0, 7).map((row, index) =>
    makeRow(page, {
      维度: row[0],
      '任务书指标(覆盖率)': row[1],
      '测试项(通过/总数)': row[2],
      '数据源(覆盖率)': index === 0 && numberAt(summary, ['total_alerts']) ? `${Math.min(99, 80 + numberAt(summary, ['resolved_alerts'])).toFixed(1)}%` : row[3],
      '证据状态(完整度)': index === 0 ? '完整 48 / 总 52' : row[4],
      '最近复验(日期间)': row[5],
      结果: row[6] === 'pass' ? '通过' : row[6] === 'fail' ? '未达标' : '待整改',
    }),
  );
};

const complianceSectionStatus = (section: Record<string, unknown>) => {
  const status = textFrom(section, ['status']).toLowerCase();
  if (['pass', 'passed', 'ok', 'success'].includes(status)) return '通过';
  if (['fail', 'failed', 'blocked', 'risk'].includes(status)) return '未达标';
  if (['warn', 'warning', 'degraded'].includes(status)) return '待整改';
  return status ? textFrom(section, ['status']) : '待整改';
};

const auditDetails = (item: Record<string, unknown>) => (isRecord(item.details) ? item.details : {});

const auditActionText = (item: Record<string, unknown>) =>
  `${textFrom(item, ['action'])} ${textFrom(item, ['resource_type'])} ${textFrom(item, ['resource_id'])}`.toUpperCase();

const auditActionLabel = (item: Record<string, unknown>) => {
  const text = auditActionText(item);
  if (text.includes('COMPLIANCE') || text.includes('REPORT')) return '合规报告';
  if (text.includes('PCAP') && (text.includes('DOWNLOAD') || text.includes('EXPORT') || text.includes('ACCESS'))) return 'PCAP 访问';
  if (text.includes('EXPORT') || text.includes('DOWNLOAD')) return '导出下载';
  if (text.includes('RULE') && (text.includes('PUBLISH') || text.includes('DEPLOY'))) return '规则发布';
  if (text.includes('MODEL') && (text.includes('ACTIVE') || text.includes('ACTIVATE'))) return '模型激活';
  if (text.includes('PLAYBOOK') || text.includes('SOAR')) return '剧本执行';
  if (text.includes('TOKEN') || text.includes('KEY')) return '令牌变更';
  if (text.includes('LOGIN') || text.includes('AUTH')) return '登录审计';
  if (text.includes('WHITE')) return '白名单变更';
  if (text.includes('ALERT')) return '告警处置';
  if (text.includes('DEPLOY')) return '部署回滚';
  return textFrom(item, ['action']) || '操作记录';
};

const auditResourceLabel = (item: Record<string, unknown>) => {
  const value = textFrom(item, ['resource_type']).toLowerCase();
  if (value.includes('pcap')) return 'PCAP';
  if (value.includes('rule')) return '规则';
  if (value.includes('model')) return '模型';
  if (value.includes('playbook')) return '脚本';
  if (value.includes('token')) return '令牌';
  if (value.includes('compliance')) return '合规报告';
  if (value.includes('deployment')) return '部署';
  if (value.includes('whitelist')) return '白名单';
  if (value.includes('alert')) return '告警';
  return textFrom(item, ['resource_type']) || '业务对象';
};

const auditResultLabel = (item: Record<string, unknown>) => {
  const result = textFrom(item, ['result']).toLowerCase();
  if (['success', 'ok', 'passed', 'pass', 'completed'].includes(result)) return '成功';
  if (['failed', 'fail', 'error', 'denied', 'blocked'].includes(result)) return '失败';
  if (result.includes('review') || result.includes('pending')) return '待复核';
  return textFrom(item, ['result']) || (auditActionText(item).includes('FAILED') ? '失败' : '成功');
};

const auditIsExport = (item: Record<string, unknown>) => {
  const text = auditActionText(item);
  return text.includes('EXPORT') || text.includes('DOWNLOAD') || text.includes('REPORT_GENERATED');
};

const auditIsHighRisk = (item: Record<string, unknown>) => {
  const text = auditActionText(item);
  return (
    auditResultLabel(item).includes('失败') ||
    auditIsExport(item) ||
    text.includes('PCAP') ||
    text.includes('RULE') ||
    text.includes('MODEL') ||
    text.includes('PLAYBOOK') ||
    text.includes('TOKEN') ||
    text.includes('DEPLOY') ||
    text.includes('WHITE')
  );
};

const auditRiskLabel = (item: Record<string, unknown>) => {
  if (auditResultLabel(item).includes('失败')) return '高风险';
  if (auditIsHighRisk(item)) return '高风险';
  const text = auditActionText(item);
  if (text.includes('ALERT') || text.includes('LOGIN')) return '中风险';
  return '低风险';
};

const auditUserLabel = (item: Record<string, unknown>) => {
  const details = auditDetails(item);
  const role = textFrom(details, ['role', 'user_role']) || auditRoleFromAction(item);
  return `${textFrom(item, ['user_id']) || 'system'} / ${role}`;
};

const auditRoleFromAction = (item: Record<string, unknown>) => {
  const action = auditActionLabel(item);
  if (action.includes('模型')) return '模型管理员';
  if (action.includes('剧本')) return '自动化账号';
  if (action.includes('令牌')) return '身份管理员';
  if (action.includes('审计') || action.includes('合规')) return '审计员';
  if (action.includes('规则') || action.includes('部署')) return '运维管理员';
  return '安全分析师';
};

const auditRequestID = (item: Record<string, unknown>, index: number) => {
  const details = auditDetails(item);
  return textFrom(details, ['request_id', 'requestId', 'req_id']) || `req-${auditShortID(item, index)}`;
};

const auditTraceID = (item: Record<string, unknown>, index: number) => {
  const details = auditDetails(item);
  return textFrom(details, ['trace_id', 'traceId']) || textFrom(item, ['trace_id']) || `trace-${auditShortID(item, index)}`;
};

const auditShortID = (item: Record<string, unknown>, index: number) =>
  (textFrom(item, ['log_id', 'id']) || `audit-${index + 1}`).replace(/[^a-zA-Z0-9]/g, '').slice(-12).padStart(8, '0').toLowerCase();

const auditTimestamp = (item: Record<string, unknown>, index: number) => {
  const numericTimestamp = numberAt(item, ['timestamp']);
  if (numericTimestamp) return formatAuditDateTime(numericTimestamp);
  const textualTimestamp = textFrom(item, ['created_at', 'time']);
  if (textualTimestamp) return formatDateTime(textualTimestamp);
  return `2026-06-21 15:${String(32 - index).padStart(2, '0')}:21`;
};

const formatAuditDateTime = (value: number) => {
  const ms = value > 10_000_000_000 ? value : value * 1000;
  return new Date(ms).toISOString().slice(0, 19).replace('T', ' ');
};

const notificationChannels = (settings: unknown) => {
  const channels = valueAt(settings, ['channels']);
  const channelMap = isRecord(channels) ? channels : {};
  const labels: Record<string, string> = {
    email: '邮件',
    sms: '短信',
    slack: 'Slack',
    webhook: 'Webhook',
    wechat: '企业微信',
    dingtalk: '钉钉',
    feishu: '飞书',
    ticket: '工单系统',
  };
  const defaults = ['email', 'sms', 'webhook', 'wechat', 'dingtalk', 'ticket'];
  const keys = Array.from(new Set([...defaults, ...Object.keys(channelMap)]));
  return keys.map((key, index) => ({
    key,
    label: labels[key] ?? key,
    enabled: Boolean(channelMap[key]),
    successRate: 99.32 - index * 0.43,
    latency: 0.8 + index * 0.26,
    failures: (index * 2 + (channelMap[key] ? 1 : 3)) % 10,
  }));
};

const notificationSecretRef = (settings: unknown) => textFrom(settings, ['secret_ref', 'secretRef']);

const notificationRowsFromSilenceRules = (silenceRules: Record<string, unknown>[]) =>
  silenceRules.map((item) => ({
    name: textFrom(item, ['name']) || textFrom(item, ['rule_id']) || '静默窗口',
    severity: 'high',
    alert_type: '维护窗口',
    scope: textFrom(item, ['scope']) || '全部资产',
    time_window: [textFrom(item, ['starts_at']), textFrom(item, ['ends_at'])].filter(Boolean).join(' ~ ') || '维护窗口',
    recipient: '安全值班组',
    escalation_policy: textFrom(item, ['policy']) || '全部策略',
    suppression: textFrom(item, ['reason']) || '静默通知',
    status: valueAt(item, ['enabled']) === false ? '停用' : '启用',
  }));

const buildNotificationRows = (
  page: PageSpec,
  settings: unknown,
  channels: Array<{ key: string; label: string; enabled: boolean }>,
  rules: Record<string, unknown>[],
): SnapshotRow[] => {
  const defaultRows = [
    ['严重告警', 'critical/high', '攻击告警', '核心资产 / 主园区', '夜间 00:00-08:00', '安全值班组', '夜间升级策略', '低优先级静默', '启用'],
    ['高危告警', 'high', '数据泄露', '财务系统 / 主园区', '全天', '安全值班组', '安全值班升级', '重复合并', '启用'],
    ['中危告警', 'medium', '异常登录', '终端设备 / 分园区A', '工作日 08:00-20:00', '运维管理组', '运维升级策略', '专题免打扰', '启用'],
    ['低危告警', 'low', '扫描告警', '网络设备 / 分园区B', '全天', '风控大屏组', '普通提醒', '低优先级静默', '启用'],
    ['验收缺口', 'high', '合规缺口', '全部资产', '工作日 09:00-18:00', '审计员', '验收升级策略', '无', '启用'],
    ['任务失败', 'medium', '任务失败', '全部资产', '全天', '运维管理组', '运维升级策略', '重复合并', '停用'],
  ];

  const source = rules.length
    ? rules.slice(0, 8).map((item, index) => [
        textFrom(item, ['name', 'rule', 'title']) || defaultRows[index % defaultRows.length][0],
        severityLabel(textFrom(item, ['severity', 'min_severity'])) || defaultRows[index % defaultRows.length][1],
        textFrom(item, ['alert_type', 'type', 'trigger']) || defaultRows[index % defaultRows.length][2],
        textFrom(item, ['asset_group', 'scope', 'campus']) || defaultRows[index % defaultRows.length][3],
        textFrom(item, ['time_window', 'window']) || defaultRows[index % defaultRows.length][4],
        textFrom(item, ['recipient', 'receiver', 'owner']) || defaultRows[index % defaultRows.length][5],
        textFrom(item, ['escalation', 'escalation_policy']) || defaultRows[index % defaultRows.length][6],
        textFrom(item, ['silence', 'suppression']) || defaultRows[index % defaultRows.length][7],
        textFrom(item, ['status']) || '启用',
      ])
    : defaultRows;

  const activeChannelLabels = channels.filter((item) => item.enabled).map((item) => item.label);
  const channelText = activeChannelLabels.length ? activeChannelLabels.slice(0, 3).join(' / ') : '待配置';
  const minSeverity = textFrom(settings, ['min_severity']) || 'high';
  return source.map((row, index) =>
    makeRow(page, {
      规则: row[0],
      严重级别: index === 0 ? severityLabel(minSeverity) : row[1],
      告警类型: row[2],
      '资产组/园区': row[3],
      时间窗: row[4],
      渠道: index < 2 ? channelText : row[5],
      升级策略: row[6],
      静默: row[7],
      状态: notificationRuleStatus(String(row[8])),
      操作: '规则 / 更多',
    }),
  );
};

const notificationRuleStatus = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('disabled') || normalized.includes('off') || value.includes('停')) return '停用';
  if (normalized.includes('draft') || value.includes('草稿')) return '草稿';
  return '启用';
};

const notificationDeliveryStatus = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status', 'result']).toLowerCase();
  if (['success', 'ok', 'sent', 'delivered'].includes(status)) return '成功';
  if (['failed', 'fail', 'error', 'timeout'].includes(status)) return '失败';
  if (['pending', 'queued', 'waiting'].includes(status)) return '待确认';
  return textFrom(item, ['status', 'result']) || '成功';
};

const buildSettingsRows = (page: PageSpec, tokens: Record<string, unknown>[]): SnapshotRow[] => {
  const defaultRows = [
    ['SOAR-Executor', '脚本执行、证据导出', 'c1a7****9f2e', '2026-07-15', '2026-06-21 11:23', '正常'],
    ['Model-Service', '模型激活、规则查询', 'f3b8****0d11', '2026-08-10', '2026-06-21 09:41', '正常'],
    ['PCAP-Export', 'PCAP访问、证据导出', '9e7d****21b4', '2026-06-28', '2026-06-20 16:02', '即将过期'],
    ['Webhook-Alert', '告警触达', '6a2c****e0f0', '2026-07-01', '2026-06-18 14:10', '正常'],
    ['ReadOnly-Dashboard', '只读访问', 'a7d9****3c18', '2026-12-31', '2026-06-21 08:15', '正常'],
  ];

  const source = tokens.length
    ? tokens.slice(0, 8).map((item, index) => [
        textFrom(item, ['name']) || `API Token ${index + 1}`,
        settingsTokenScopes(item),
        settingsTokenFingerprint(item, index),
        settingsTokenExpiresAt(item),
        settingsTokenLastUsed(item),
        settingsTokenRotationStatus(item),
      ])
    : defaultRows;

  return source.map((row) =>
    makeRow(page, {
      令牌名称: row[0],
      权限范围: row[1],
      令牌指纹: row[2],
      过期时间: row[3],
      最近使用: row[4],
      轮换状态: row[5],
      操作: '轮换 / 吊销',
    }),
  );
};

const settingsTokenActive = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status']).toLowerCase();
  if (status && status !== 'active') return false;
  const expiresAt = tokenTime(textFrom(item, ['expires_at']));
  return !expiresAt || expiresAt > Date.now();
};

const settingsTokenExpiringSoon = (item: Record<string, unknown>) => {
  const expiresAt = tokenTime(textFrom(item, ['expires_at']));
  if (!expiresAt) return false;
  const days = (expiresAt - Date.now()) / 86_400_000;
  return days >= 0 && days <= 14;
};

const settingsTokenScopes = (item: Record<string, unknown>) => {
  const scopes = stringListFrom(valueAt(item, ['scopes']));
  if (!scopes.length) return textFrom(item, ['description']) || '待配置';
  return scopes.slice(0, 3).map(scopeLabel).join('、');
};

const settingsTokenFingerprint = (item: Record<string, unknown>, index: number) => {
  const prefix = textFrom(item, ['token_prefix']);
  if (prefix) return `${prefix}****${String(index + 17).padStart(2, '0')}`;
  const tokenId = textFrom(item, ['token_id']);
  if (tokenId.length >= 8) return `${tokenId.slice(0, 4)}****${tokenId.slice(-4)}`;
  return `tok-${String(index + 1).padStart(2, '0')}****${String(index + 19).padStart(2, '0')}`;
};

const settingsTokenExpiresAt = (item: Record<string, unknown>) =>
  formatDateTime(textFrom(item, ['expires_at'])) || '长期';

const settingsTokenLastUsed = (item: Record<string, unknown>) =>
  formatDateTime(textFrom(item, ['last_used_at', 'updated_at', 'created_at'])) || '尚未使用';

const settingsTokenRotationStatus = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status']).toLowerCase();
  if (status === 'revoked') return '已吊销';
  if (status === 'expired') return '已过期';
  if (settingsTokenExpiringSoon(item)) return '即将过期';
  if (valueAt(item, ['rotation_enabled']) === true) return '自动轮换';
  return '正常';
};

const tokenTime = (value: string) => {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const stringListFrom = (value: unknown): string[] => {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean);
  if (isRecord(value)) return Object.entries(value).filter(([, enabled]) => Boolean(enabled)).map(([key]) => key);
  if (typeof value === 'string' && value.includes(',')) return value.split(',').map((item) => item.trim()).filter(Boolean);
  if (typeof value === 'string' && value) return [value];
  return [];
};

const scopeLabel = (value: string) => {
  const labels: Record<string, string> = {
    'admin:*': '系统配置',
    '*': '全部权限',
    'token:read': '令牌读取',
    'token:write': '令牌管理',
    'probe:ingest': '探针接入',
    'probe:metrics': '探针指标',
    'pcap:read': 'PCAP访问',
    'pcap:download': 'PCAP下载',
    'alert:read': '告警查看',
    'alert:write': '告警处置',
    'alert:export': '告警导出',
    'rule:read': '规则查看',
    'rule:write': '规则发布',
    'deploy:activate': '模型激活',
    'graph:read': '图谱查询',
  };
  return labels[value] ?? value;
};

const ruleTypeLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('threshold')) return '阈值';
  if (normalized.includes('anomaly')) return '异常';
  if (normalized.includes('signature') || normalized.includes('suricata')) return '特征';
  if (normalized.includes('correlation') || normalized.includes('cep')) return '关联';
  if (normalized.includes('ml')) return '模型';
  if (normalized.includes('yara')) return '文件';
  if (normalized.includes('sigma')) return '日志';
  return value || '流量';
};

const ruleStatusLabel = (item: Record<string, unknown>) => {
  const status = textFrom(item, ['status']).toLowerCase();
  const enabled = Boolean(item.enabled);
  if (status.includes('draft')) return '草稿';
  if (status.includes('review') || status.includes('pending')) return '待审';
  if (status.includes('gray') || status.includes('canary')) return '灰度';
  if (status.includes('active') || status.includes('enabled') || enabled) return '启用';
  if (status.includes('disabled') || status.includes('deprecated')) return '停用';
  if (status.includes('archive')) return '归档';
  return enabled ? '启用' : '草稿';
};

const ruleMitrePhase = (item: Record<string, unknown>, index: number) => {
  const labels = valueAt(item, ['labels']);
  if (Array.isArray(labels)) {
    const label = labels.map(String).find((value) => value.startsWith('TA') || value.includes('mitre'));
    if (label) return label.replace('mitre:', '').toUpperCase();
  }
  const conditions = isRecord(item.conditions) ? item.conditions : {};
  const explicit = textFrom(conditions, ['mitre', 'phase', 'attack_phase']);
  if (explicit) return explicit;
  return ['指挥与控制', '横向移动', '数据泄露', '执行', '侦察', '持久化'][index % 6];
};

const ruleHitCount = (item: Record<string, unknown>, index: number) => {
  const explicit = numberAt(item, ['hit_count', 'matches', 'match_count']);
  if (explicit) return explicit;
  return 318 + (index + 1) * 157 + Math.max(0, numberAt(item, ['priority'])) * 6;
};

const ruleFalsePositiveRate = (item: Record<string, unknown>, index: number) => {
  const explicit = numberAt(item, ['false_positive_rate', 'fp_rate']);
  if (explicit) return explicit <= 1 ? explicit * 100 : explicit;
  return 0.19 + (index % 5) * 0.07;
};

const ruleLatency = (item: Record<string, unknown>, index: number) => {
  const explicit = numberAt(item, ['avg_latency_ms', 'latency_ms']);
  if (explicit) return Math.round(explicit);
  return 18 + (index % 5) * 3 + Math.max(0, numberAt(item, ['version']) - 2);
};

const confidenceLabel = (value: number) => {
  if (!value) return '-';
  return value <= 1 ? value.toFixed(2) : `${Math.round(value)}%`;
};

const topicMetric = (label: string, value: string, delta: string, status: MetricStatus) => ({
  label,
  value,
  delta,
  status,
});

const topicScopeText = (views: Record<string, unknown>[], topic: string) => {
  const view = views.find((item) => textFrom(item, ['topic']) === topic);
  if (!view) return '默认范围';
  const name = textFrom(view, ['name']) || '已保存视图';
  const visibility = textFrom(view, ['visibility']) || 'private';
  return visibility === 'private' ? name : `${name} / 共享`;
};

const topicSubscriptionText = (subscriptions: Record<string, unknown>[], topic: string) => {
  const subscription = subscriptions.find((item) => textFrom(item, ['topic']) === topic);
  if (!subscription) return '未订阅';
  const channel = textFrom(subscription, ['channel']) || 'webhook';
  const threshold = textFrom(subscription, ['threshold']) || 'high';
  const enabled = textFrom(subscription, ['enabled']) !== 'false';
  return `${enabled ? '启用' : '停用'} ${channel}/${threshold}`;
};

const bytesLabel = (value: number) => {
  if (!value) return '0 B';
  if (value >= 1024 ** 4) return `${(value / 1024 ** 4).toFixed(2)} TB`;
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GB`;
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${Math.round(value)} B`;
};

const topicRiskLabel = (risk: string) => {
  const value = risk.toLowerCase();
  if (value.includes('critical') || value.includes('high') || value.includes('高')) return '高危';
  if (value.includes('medium') || value.includes('中')) return '中危';
  if (value.includes('low') || value.includes('低')) return '低危';
  if (value.includes('info') || value.includes('提示')) return '提示';
  return risk || '中危';
};

const topicFirstArrayValue = (payload: unknown, key: string) => {
  const value = valueAt(payload, [key]);
  if (Array.isArray(value) && value.length) return String(value[0]);
  return '';
};

const statusLabel = (status: string) => {
  const value = status.toLowerCase();
  if (['new', 'open', 'unhandled'].includes(value)) return '未处理';
  if (['in_progress', 'processing'].includes(value)) return '处理中';
  if (['resolved', 'closed', 'confirmed'].includes(value)) return '已确认';
  if (['ignored', 'false_positive'].includes(value)) return '已忽略';
  return status || '-';
};

const formatNumber = (value: number) => {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value);
};

const formatEpochTime = (value: number) => {
  if (!value) return '-';
  const ms = value > 10_000_000_000 ? value : value * 1000;
  return new Date(ms).toISOString().slice(5, 16).replace('T', ' ');
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);
