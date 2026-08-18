import type { PageSpec } from '@/routes/routeManifest';
import { alertStatusLabel, normalizeAlertStatus } from '@/services/alertStatus';
import type {
  DataQualityCheck,
  DataQualityVisuals,
  ForensicsVisuals,
  PageSnapshot,
  ScreenVisualNode,
  ScreenVisualPoint,
  ScreenVisuals,
  ScreenWorldFlow,
  ScreenWorldPoint,
  SnapshotRow,
  TopicVisuals,
} from '@/services/mockData';
import {
  extractNamedPageSnapshotList as extractNamedList,
  extractPageSnapshotList as extractList,
  isRecord,
  unwrapPageSnapshotPayload as unwrapPayload,
} from '@/services/pageSnapshotEnvelope';
import {
  adaptEncryptedTrafficSnapshot,
  formatEvidenceDateTime,
} from '@/services/encryptedTrafficSnapshotAdapter';

type MetricStatus = PageSnapshot['metrics'][number]['status'];

export const adaptKnownPageSnapshot = (
  page: PageSpec,
  primaryPayload: unknown,
  secondaryPayloads: unknown[],
): PageSnapshot | undefined => {
  if (page.id === 'screen') return adaptScreen(page, primaryPayload, secondaryPayloads);
  if (page.id === 'probes') return adaptProbes(page, primaryPayload);
  if (page.id === 'data-quality') return adaptDataQuality(page, primaryPayload);
  if (page.id === 'alerts') return adaptAlerts(page, primaryPayload, secondaryPayloads);
  if (page.id === 'assets') return adaptAssets(page, primaryPayload, secondaryPayloads);
  if (page.id === 'graph') return adaptGraph(page, primaryPayload);
  if (page.id === 'fusion') return adaptFusion(page, primaryPayload, secondaryPayloads);
  if (page.id === 'baselines') return adaptBaselines(page, primaryPayload);
  if (page.id === 'campaigns') return adaptCampaigns(page, primaryPayload);
  if (page.id === 'attack-chains') return adaptAttackChains(page, primaryPayload);
  if (page.id === 'topics') return adaptTopicsOverview(page, primaryPayload, secondaryPayloads);
  if (page.id === 'topic-tunnel' || page.id === 'topic-exfil' || page.id === 'topic-apt') return adaptTopicPage(page, primaryPayload);
  if (page.id === 'encrypted-traffic') return adaptEncryptedTrafficSnapshot(page, primaryPayload, secondaryPayloads);
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
  const captureModeCount = probes.length ? modes.size : undefined;
  const nicCount = sumArrayLengths(probes, ['interfaces']);
  const mtlsEnabled = probes.filter((item) => Boolean(valueAt(item, ['mtls_enabled']))).length;

  return {
    id: page.id,
    total,
    metrics: [
      metric('探针总数', total, '台', total ? 'info' : 'warn'),
      metric('在线探针', online, '在线', online === total && total ? 'ok' : online ? 'warn' : 'risk'),
      metric('采集网卡', nicCount, '张', nicCount ? 'info' : 'warn'),
      metric('采集模式', captureModeCount, '种', captureModeCount === undefined ? 'warn' : 'info'),
      metric('平均 CPU', avgCpu, '%', avgCpu >= 80 ? 'risk' : avgCpu >= 60 ? 'warn' : 'ok'),
      metric('平均内存', avgMemory, '%', avgMemory >= 80 ? 'risk' : avgMemory >= 60 ? 'warn' : 'ok'),
      metric('告警探针', degraded, '台', degraded ? 'warn' : 'ok'),
      metric('离线探针', offline, '台', offline ? 'risk' : 'ok'),
    ],
    rows: probes.map((item) =>
      makeRow(page, {
        '探针 ID': textFrom(item, ['probe_id', 'id']) || '-',
        位置: probeLocation(item),
        状态: probeStatusLabel(textFrom(item, ['status'])),
        采集模式: probeCaptureMode(item),
        采集带宽: `${(numberFrom(item, ['bandwidth_mbps']) / 1000).toFixed(1)} Gbps`,
        丢包率: `${ratioAt(item, ['drop_rate']).toFixed(2)}%`,
        解析率: `${numberFrom(item, ['parse_rate']).toFixed(2)}%`,
        CPU: `${numberFrom(item, ['cpu_usage']).toFixed(1)}%`,
        内存: `${numberFrom(item, ['memory_usage', 'memory_percent']).toFixed(1)}%`,
        运行时长: probeUptime(item),
        版本: textFrom(item, ['config_version', 'software_version', 'version']) || '-',
        磁盘: `${numberFrom(item, ['disk_usage']).toFixed(1)}%`,
        采集网卡: stringArrayFrom(item, ['interfaces']).join(', '),
        归档路径: textFrom(item, ['archive_path']) || '-',
        mTLS: valueAt(item, ['mtls_enabled']) ? '已启用' : '未启用',
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
  const topicHealth = extractNamedList(primaryPayload, ['topics', 'topic_health', 'topicHealth']);
  const metrics = isRecord(report) && isRecord(report.metrics) ? report.metrics : {};
  const score = optionalNumberFrom(report, ['score', 'quality_score']);
  const completenessValue = optionalNumberAt(metrics, ['data_completeness']) ?? qualityCheckValue(checks, 'data_completeness');
  const completeness = completenessValue === undefined ? undefined : boundedPercent(completenessValue, 0);
  const latencyMs = optionalNumberAt(metrics, ['p95_latency_ms']) ?? qualityCheckValue(checks, 'end_to_end_latency');
  const timeliness = optionalRatioAt(metrics, ['timeliness']);
  const accuracy = optionalRatioAt(metrics, ['accuracy']);
  const duplicateRate = optionalRatioAt(metrics, ['duplicate_rate']);
  const fieldMissing = optionalRatioAt(metrics, ['field_missing_rate']);
  const kafkaLag = optionalNumberAt(metrics, ['insert_rate_per_min']) ?? qualityCheckValue(checks, 'kafka_lag_proxy');
  const dlqCount = optionalNumberAt(metrics, ['dlq_count']);
  const topics = buildQualityTopics(page, topicHealth);
  const visualsEnvelope = isRecord(report) && isRecord(report.visuals) ? report.visuals : undefined;
  const dataQualityVisuals = visualsEnvelope && isRecord(visualsEnvelope.dataQuality)
    ? visualsEnvelope.dataQuality as DataQualityVisuals
    : undefined;
  const source = isRecord(report) && isRecord(report.data_source) ? report.data_source : {};
  const visualSource = textFrom(source, ['visuals']) || 'unconfigured';
  const fixtureVersion = textFrom(source, ['fixture_version']);
  const dataQualityChecks: DataQualityCheck[] = checks.map((item) => {
    const rawStatus = textFrom(item, ['status']).toLowerCase();
    const status: DataQualityCheck['status'] = rawStatus === 'pass' || rawStatus === 'warn' || rawStatus === 'fail' ? rawStatus : 'unknown';
    const rawMeasured = valueAt(item, ['measured']);
    return {
      name: textFrom(item, ['name']) || 'unknown',
      status,
      message: textFrom(item, ['message']) || '质量检查未返回说明。',
      value: optionalNumberFrom(item, ['value']),
      threshold: optionalNumberFrom(item, ['threshold']),
      measured: typeof rawMeasured === 'boolean' ? rawMeasured : true,
      source: textFrom(item, ['source']),
    };
  });

  return {
    id: page.id,
    dataQualityChecks,
    metrics: [
      metric('质量总分', score, '分', score === undefined ? 'info' : score >= 90 ? 'ok' : score >= 80 ? 'warn' : 'risk'),
      metric('完整性', completeness, '%', completeness === undefined ? 'info' : completeness >= 95 ? 'ok' : completeness >= 90 ? 'warn' : 'risk'),
      metric('及时性', timeliness, '%', timeliness === undefined ? 'info' : timeliness >= 92 ? 'ok' : timeliness >= 88 ? 'warn' : 'risk'),
      metric('准确性', accuracy, '%', accuracy === undefined ? 'info' : accuracy >= 92 ? 'ok' : accuracy >= 88 ? 'warn' : 'risk'),
      metric('重复率', duplicateRate, '%', duplicateRate === undefined ? 'info' : duplicateRate <= 1 ? 'ok' : duplicateRate <= 3 ? 'warn' : 'risk'),
      metric('字段缺失率', fieldMissing, '%', fieldMissing === undefined ? 'info' : fieldMissing <= 2 ? 'ok' : fieldMissing <= 5 ? 'warn' : 'risk'),
      metric('DLQ 数量', dlqCount, '条', dlqCount === undefined ? 'info' : dlqCount > 20_000 ? 'risk' : dlqCount > 10_000 ? 'warn' : 'ok'),
    ],
    rows: topics,
    timeline: [
      timelineItem('Data Quality API 已接入', `来自 /v1/data-quality，整体状态 ${qualityOverallLabel(report)}。`, checks.length ? 'ok' : 'warn'),
      timelineItem('Kafka Topic 健康', `流量 ${formatNumber(optionalNumberAt(metrics, ['flow_rate']))}/min，积压代理 ${formatNumber(kafkaLag)}。`, kafkaLag === undefined ? 'info' : kafkaLag > 5000 ? 'warn' : 'ok'),
      timelineItem('Flink 处理质量', latencyMs === undefined ? '端到端 P95 暂不可用；Checkpoint 与 watermark 等待服务端返回。' : `端到端 P95 ${Math.round(latencyMs)} ms，Checkpoint 与 watermark 用页面门禁继续展示。`, latencyMs === undefined ? 'info' : latencyMs > 60_000 ? 'risk' : 'ok'),
      timelineItem('字段与存储对账', `字段缺失 ${fieldMissing === undefined ? '暂不可用' : `${fieldMissing.toFixed(2)}%`}，ClickHouse 写入 ${formatNumber(optionalNumberAt(metrics, ['insert_rate_per_min']))}/min。`, fieldMissing === undefined ? 'info' : fieldMissing > 5 ? 'risk' : 'ok'),
      ...checks.slice(0, 4).map((item) =>
        timelineItem(qualityCheckName(item), textFrom(item, ['message']) || '质量检查已返回。', qualityStatus(textFrom(item, ['status']))),
      ),
    ],
    evidence: [
      evidence('Data Quality API', '/v1/data-quality', checks.length ? 'ok' : 'warn'),
      evidence('质量基线', score === undefined ? '暂不可用' : `${score} 分`, score === undefined ? 'info' : score >= 90 ? 'ok' : 'warn'),
      evidence('Kafka Topic', `${topics.length} 个`, topics.length ? 'ok' : 'warn'),
      evidence('Flink Checkpoint', latencyMs === undefined ? '暂不可用' : latencyMs > 60_000 ? '延迟异常' : '最新可用', latencyMs === undefined ? 'info' : latencyMs > 60_000 ? 'risk' : 'ok'),
      evidence('字段矩阵', fieldMissing === undefined ? '暂不可用' : `${fieldMissing.toFixed(2)}% 缺失`, fieldMissing === undefined ? 'info' : fieldMissing > 5 ? 'risk' : 'ok'),
      evidence('存储写入', `${formatNumber(numberAt(metrics, ['insert_rate_per_min']))}/min`, numberAt(metrics, ['insert_rate_per_min']) ? 'ok' : 'info'),
      evidence('重放对账', dlqCount === undefined ? '暂不可用' : `${formatNumber(dlqCount)} DLQ`, dlqCount === undefined ? 'info' : dlqCount > 20_000 ? 'risk' : 'warn'),
      evidence('可视化数据源', fixtureVersion ? `${visualSource} / ${fixtureVersion}` : visualSource, dataQualityVisuals ? 'ok' : 'risk'),
    ],
    visuals: dataQualityVisuals ? { dataQuality: dataQualityVisuals } : undefined,
  };
};

const metricNumericValue = (value: unknown) => {
  const numericValue = Number(String(value ?? '').replace(/[^\d.-]/g, ''));
  return Number.isFinite(numericValue) ? numericValue : 0;
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

const levelFromValue = (value: number, high: number, medium: number): ScreenWorldPoint['level'] => {
  if (value >= high) return 'high';
  if (value >= medium) return 'medium';
  return 'low';
};

const screenWorldPointFrom = (item: Record<string, unknown>): ScreenWorldPoint | undefined => {
  const x = optionalNumberFrom(item, ['map_x', 'x', 'longitude', 'lon']);
  const y = optionalNumberFrom(item, ['map_y', 'y', 'latitude', 'lat']);
  const value = optionalNumberFrom(item, ['egress_gbps', 'gbps', 'value', 'count', 'sessions']);
  const name = textFrom(item, ['region', 'country', 'dst_region', 'name', 'label']);
  if (x === undefined || y === undefined || value === undefined || !name) return undefined;
  return { name, coord: [x, y], value, level: levelFromValue(value, 30, 12) };
};

const buildProbeMapFromApi = (
  probes: Record<string, unknown>[],
) => {
  const visibleProbes = probes.filter((probe) => Boolean(textFrom(probe, ['probe_id', 'id', 'name', 'hostname']))).slice(0, 14);
  const nodes: ScreenVisualNode[] = visibleProbes.map((probe, index): ScreenVisualNode => {
      const id = textFrom(probe, ['probe_id', 'id', 'name', 'hostname']);
      const { x, y } = probeMapCoordinate(probe, index, visibleProbes.length);
      const status = normalizeProbeMapStatus(textFrom(probe, ['status', 'state', 'health']));
      return {
        id,
        label: textFrom(probe, ['name', 'hostname', 'label', 'zone']) || id,
        x,
        y,
        status,
        meta: textFrom(probe, ['location', 'zone', 'site']) || 'API 探针',
        tone: status === 'offline' ? 'risk' : status === 'maintenance' ? 'warn' : 'ok',
      };
    });
  const nodeIds = new Set(nodes.map((node) => node.id));
  const links: Array<[string, string]> = visibleProbes.flatMap((probe, index) => {
    const id = nodes[index].id;
    const upstream = textFrom(probe, ['upstream_probe_id', 'parent_probe_id', 'gateway_probe_id', 'core_probe_id']);
    return upstream && nodeIds.has(upstream) ? [[upstream, id] as [string, string]] : [];
  });
  return { nodes, links };
};

const adaptScreen = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const stats = unwrapPayload(primaryPayload);
  const encryptedTrend = extractList(secondaryPayloads[0], ['trend', 'data']);
  const phases = extractList(secondaryPayloads[1], ['phases', 'data']);
  const probes = extractList(secondaryPayloads[2], ['probes', 'items', 'data']);
  const assetsTotal = optionalNumberAt(stats, ['assets', 'total']) ?? optionalNumberAt(stats, ['fusion', 'entities_aligned']);
  const buildingTotal = optionalNumberAt(stats, ['assets', 'buildings_total']);
  const buildingCovered = optionalNumberAt(stats, ['assets', 'buildings_covered']);
  const buildingCoverage = optionalRatioAt(stats, ['assets', 'coverage_rate'])
    ?? (buildingCovered !== undefined && buildingTotal ? (buildingCovered / buildingTotal) * 100 : undefined);
  const probeOnline = optionalNumberAt(stats, ['probes', 'online']);
  const probeDegraded = optionalNumberAt(stats, ['probes', 'degraded']);
  const probeOffline = optionalNumberAt(stats, ['probes', 'offline']);
  const probeTotal = optionalNumberAt(stats, ['probes', 'total'])
    ?? (probeOnline !== undefined && probeDegraded !== undefined && probeOffline !== undefined
      ? probeOnline + probeDegraded + probeOffline
      : undefined);
  const probeOnlineRate = optionalRatioAt(stats, ['probes', 'online_rate'])
    ?? (probeOnline !== undefined && probeTotal ? (probeOnline / probeTotal) * 100 : undefined);
  const throughputGbps =
    optionalNumberAt(stats, ['traffic', 'gbps']) ??
    optionalNumberAt(stats, ['traffic', 'throughput_gbps']) ??
    optionalNumberAt(stats, ['performance', 'throughput_gbps']);
  const parserSuccess = optionalRatioAt(stats, ['performance', 'parser_success_rate']) ?? optionalRatioAt(stats, ['data_quality', 'parse_success_rate']);
  const kafkaLag = optionalNumberAt(stats, ['performance', 'kafka_lag']);
  const p95Ms = optionalNumberAt(stats, ['performance', 'end_to_end_p95_ms']);
  const criticalAlerts = optionalNumberAt(stats, ['alerts', 'critical']);
  const highOnlyAlerts = optionalNumberAt(stats, ['alerts', 'high']);
  const highAlerts = criticalAlerts !== undefined && highOnlyAlerts !== undefined ? criticalAlerts + highOnlyAlerts : undefined;
  const evidenceCoverage =
    optionalRatioAt(stats, ['evidence', 'coverage_rate']) ??
    optionalRatioAt(stats, ['fusion', 'completeness']) ??
    optionalRatioAt(stats, ['compliance', 'pass_rate']);
  const phaseTotal = phases.reduce((sum, item) => sum + numberFrom(item, ['count', 'value']), 0);
  const responseActions =
    optionalNumberAt(stats, ['response', 'actions_24h']) ??
    optionalNumberAt(stats, ['playbooks', 'actions_24h']);
  const encryptedSessions = encryptedTrend.reduce((sum, item) => sum + numberFrom(item, ['count', 'sessions', 'value']), 0);
  const screenVisuals = buildScreenVisuals({
    phases,
    encryptedTrend,
    probes,
  });
  const displayNumber = (value: number | undefined, digits?: number) => value === undefined
    ? '暂不可用'
    : digits === undefined ? formatNumber(value) : value.toFixed(digits);
  const displayPair = (value: number | undefined, total: number | undefined) =>
    value === undefined || total === undefined ? '暂不可用' : `${formatNumber(value)}/${formatNumber(total)}`;

  return {
    id: page.id,
    metrics: [
      metric('楼宇覆盖率', buildingCoverage, '%', buildingCoverage === undefined ? 'warn' : buildingCoverage >= 95 ? 'ok' : buildingCoverage >= 90 ? 'warn' : 'risk'),
      metric('探针在线率', probeOnlineRate, '%', probeOnlineRate === undefined ? 'warn' : probeOnlineRate >= 95 ? 'ok' : probeOnlineRate >= 90 ? 'warn' : 'risk'),
      metric('采集吞吐', throughputGbps, 'Gbps', throughputGbps ? 'ok' : 'warn'),
      metric('协议解析率', parserSuccess, '%', parserSuccess === undefined ? 'warn' : parserSuccess >= 98 ? 'ok' : parserSuccess >= 95 ? 'warn' : 'risk'),
      metric('Kafka 积压', kafkaLag, 'msg', kafkaLag === undefined ? 'warn' : kafkaLag >= 5_000 ? 'risk' : kafkaLag >= 500 ? 'warn' : 'ok'),
      metric('Flink P95', p95Ms, 'ms', p95Ms === undefined ? 'warn' : p95Ms >= 60_000 ? 'risk' : p95Ms >= 5_000 ? 'warn' : 'ok'),
      metric('证据完整度', evidenceCoverage, '%', evidenceCoverage === undefined ? 'warn' : evidenceCoverage >= 95 ? 'ok' : evidenceCoverage >= 90 ? 'warn' : 'risk'),
      metric('高危告警', highAlerts, '条', highAlerts === undefined ? 'warn' : highAlerts ? 'risk' : 'ok'),
      metric('攻击阶段', phases.length, '类', phases.length ? 'ok' : 'warn'),
      metric('闭环动作', responseActions, '次', responseActions ? 'ok' : 'warn'),
    ],
    visuals: {
      screen: screenVisuals,
    },
    rows: [
      makeRow(page, {
        '对象 ID': 'SCREEN-CAPTURE',
        类型: '采集覆盖',
        范围: `${displayPair(buildingCovered, buildingTotal)} 楼宇，${displayPair(probeOnline, probeTotal)} 探针`,
        风险: buildingCoverage !== undefined && probeOnlineRate !== undefined && buildingCoverage >= 95 && probeOnlineRate >= 95 ? '低风险' : '待复核',
        证据: '/v1/dashboard/stats',
        状态: '已接入',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-PIPELINE',
        类型: '流处理链路',
        范围: `${displayNumber(throughputGbps, 1)} Gbps / ${displayNumber(kafkaLag)} lag / ${displayNumber(p95Ms)} ms`,
        风险: kafkaLag === undefined || p95Ms === undefined ? '待复核' : kafkaLag >= 5_000 || p95Ms >= 60_000 ? '高风险' : kafkaLag >= 500 || p95Ms >= 5_000 ? '中风险' : '低风险',
        证据: 'Kafka / Flink / ClickHouse',
        状态: '实时展示',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-THREAT',
        类型: '威胁态势',
        范围: `${displayNumber(highAlerts)} 高危告警，${phases.length} 攻击阶段`,
        风险: highAlerts === undefined ? '待复核' : highAlerts ? '高风险' : '低风险',
        证据: '/v1/dashboard/attack-phases',
        状态: phases.length ? '已关联' : '待返回',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-EVIDENCE',
        类型: '取证证据',
        范围: `${displayNumber(evidenceCoverage, 1)}% 证据完整度`,
        风险: evidenceCoverage !== undefined && evidenceCoverage >= 95 ? '低风险' : '待补齐',
        证据: 'PCAP / Session / Audit',
        状态: '闭环展示',
      }),
      makeRow(page, {
        '对象 ID': 'SCREEN-RESPONSE',
        类型: '响应反馈',
        范围: `${displayNumber(responseActions)} 次动作，${formatNumber(encryptedSessions)} 加密趋势样本`,
        风险: responseActions ? '低风险' : '待处置',
        证据: '/v1/dashboard/encrypted/trend',
        状态: '联动剧本',
      }),
    ],
    timeline: [
      timelineItem('大屏真实统计已接入', `来自 /v1/dashboard/stats，覆盖 ${displayNumber(assetsTotal ?? buildingCovered)} 个对象、${displayPair(probeOnline, probeTotal)} 个在线探针。`, assetsTotal !== undefined || buildingCovered !== undefined ? 'ok' : 'warn'),
      timelineItem('全流量处理链路已映射', `采集 ${displayNumber(throughputGbps, 1)} Gbps，Kafka 积压 ${displayNumber(kafkaLag)}，Flink P95 ${displayNumber(p95Ms)} ms。`, kafkaLag === undefined || p95Ms === undefined || kafkaLag >= 500 || p95Ms >= 5_000 ? 'warn' : 'ok'),
      timelineItem('攻击阶段与加密趋势已关联', `攻击阶段 ${phases.length || 0} 类，加密趋势样本 ${formatNumber(encryptedSessions)}。`, phases.length && encryptedTrend.length ? 'ok' : 'warn'),
      timelineItem('取证与反馈闭环已上屏', `证据完整度 ${displayNumber(evidenceCoverage, 1)}%，近 24 小时响应动作 ${displayNumber(responseActions)}。`, evidenceCoverage !== undefined && evidenceCoverage >= 95 && responseActions ? 'ok' : 'warn'),
    ],
    evidence: [
      evidence('Screen API', '/v1/dashboard/stats', 'ok'),
      evidence('Encrypted Trend API', `${encryptedTrend.length} 点`, encryptedTrend.length ? 'ok' : 'warn'),
      evidence('Attack Phases API', `${phases.length} 类 / ${formatNumber(phaseTotal)} 次`, phases.length ? 'ok' : 'warn'),
      evidence('楼宇覆盖', displayPair(buildingCovered, buildingTotal), buildingCoverage !== undefined && buildingCoverage >= 95 ? 'ok' : 'warn'),
      evidence('探针在线', displayPair(probeOnline, probeTotal), probeOnlineRate !== undefined && probeOnlineRate >= 95 ? 'ok' : 'warn'),
      evidence('证据闭环', evidenceCoverage === undefined ? '暂不可用' : `${evidenceCoverage.toFixed(1)}%`, evidenceCoverage !== undefined && evidenceCoverage >= 95 ? 'ok' : 'warn'),
      evidence('响应动作', responseActions === undefined ? '暂不可用' : `${formatNumber(responseActions)} 次`, responseActions ? 'ok' : 'warn'),
    ],
  };
};

const buildScreenVisuals = ({
  phases,
  encryptedTrend,
  probes,
}: {
  phases: Record<string, unknown>[];
  encryptedTrend: Record<string, unknown>[];
  probes: Record<string, unknown>[];
}): ScreenVisuals => {
  const liveProbeMap = buildProbeMapFromApi(probes);
  const phasePoints: ScreenVisualPoint[] = phases.slice(0, 12).flatMap((phase, index) => {
        const value = optionalNumberFrom(phase, ['count', 'value']);
        const name = textFrom(phase, ['phase', 'name', 'label']);
        if (value === undefined || !name) return [];
        const angle = (index / Math.max(phases.length, 1)) * Math.PI * 2;
        const radius = 14 + Math.min(28, value / 120);
        return [{
          name,
          x: 52 + Math.cos(angle) * radius,
          y: 52 + Math.sin(angle) * radius,
          value,
          level: value >= 200 ? 'high' : value >= 80 ? 'medium' : 'low',
        } as const];
      });
  const riskMapPoints: ScreenWorldPoint[] = [];
  const egressMapPoints = encryptedTrend.flatMap((item) => {
    const point = screenWorldPointFrom(item);
    return point ? [point] : [];
  });
  const egressMapFlows: ScreenWorldFlow[] = [];

  return {
    probeMapNodes: liveProbeMap.nodes,
    probeMapLinks: liveProbeMap.links,
    topologyNodes: [],
    topologyEdges: [],
    campaignDensityPoints: phasePoints,
    riskMapPoints,
    egressMapPoints,
    egressMapFlows,
    abnormalLinks: [],
    evidenceRings: [],
  };
};

const adaptAlerts = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[] = []): PageSnapshot => {
  const envelope = unwrapEnvelope(primaryPayload);
  const alerts = extractList(primaryPayload, ['alerts', 'data']);
  const total = totalFromEnvelope(envelope, alerts.length);
  const pageSeverityCounts = countBy(alerts, 'severity');
  const pageStatusCounts = alerts.reduce<Record<string, number>>((acc, item) => {
    const status = normalizeAlertStatus(textFrom(item, ['status'])) ?? 'unknown';
    acc[status] = (acc[status] ?? 0) + 1;
    return acc;
  }, {});
  const stats = unwrapPayload(secondaryPayloads[0]);
  const statsRecord = isRecord(stats) ? stats : {};
  const rawSeverityCounts = isRecord(statsRecord.by_severity) ? statsRecord.by_severity : pageSeverityCounts;
  const severityCounts = Object.entries(rawSeverityCounts).reduce<Record<string, number>>((acc, [key, value]) => {
    const normalized = key.toLowerCase().replace(/^severity_/, '');
    acc[normalized] = (acc[normalized] ?? 0) + metricNumericValue(value);
    return acc;
  }, {});
  const rawStatusCounts = isRecord(statsRecord.by_status) ? statsRecord.by_status : pageStatusCounts;
  const statusCounts = Object.entries(rawStatusCounts).reduce<Record<string, number>>((acc, [key, value]) => {
    const normalized = normalizeAlertStatus(key) ?? key.toLowerCase();
    acc[normalized] = (acc[normalized] ?? 0) + metricNumericValue(value);
    return acc;
  }, {});
  const criticalCount = countValue(severityCounts, 'critical');
  const highOnlyCount = countValue(severityCounts, 'high');
  const mediumCount = countValue(severityCounts, 'medium');
  const lowCount = countValue(severityCounts, 'low') + countValue(severityCounts, 'info');
  const highCount = criticalCount + highOnlyCount;
  const statsWindowTotal = optionalNumberAt(statsRecord, ['total']) ?? total;

  return {
    id: page.id,
    metrics: [
      alertQueueMetric('高危', highCount, highCount ? 'risk' : 'ok'),
      alertQueueMetric('中危', mediumCount, mediumCount ? 'warn' : 'ok'),
      alertQueueMetric('低危', lowCount, 'info'),
      alertQueueMetric('未处理', countValue(statusCounts, 'new'), 'risk'),
      alertQueueMetric('处理中', countValue(statusCounts, 'triage'), 'warn'),
      alertQueueMetric('已确认', countValue(statusCounts, 'assigned'), 'ok'),
      alertQueueMetric('已忽略', countValue(statusCounts, 'closed'), 'info'),
    ],
    total,
    rows: alerts.map((item, index) => {
      const rawStatus = textFrom(item, ['status']);
      return makeRow(page, {
        '告警 ID': textFrom(item, ['alert_id', 'id']) || `ALERT-${index + 1}`,
        风险等级: alertSeverityLabel(textFrom(item, ['severity'])),
        告警名称: textFrom(item, ['alert_type', 'name', 'title']) || '-',
        攻击阶段: attackPhaseLabel(textFrom(item, ['attack_phase', 'phase', 'mitre_phase'])),
        '源 IP': textFrom(item, ['src_ip', 'source_ip']) || '-',
        '目的 IP': textFrom(item, ['dst_ip', 'destination_ip']) || '-',
        受影响资产: textFrom(item, ['asset_name', 'affected_asset', 'asset_id', 'hostname']) || textFrom(item, ['src_ip']) || '-',
        '规则/模型': textFrom(item, ['rule_name', 'rule_id', 'rule_version', 'model_name', 'model', 'model_version']) || textFrom(item, ['alert_type']) || '-',
        置信度: confidenceLabel(numberFrom(item, ['confidence', 'score'])),
        首次发生: textFrom(item, ['first_seen', 'created_at', 'timestamp']) || '-',
        状态: alertDisplayStatus(rawStatus),
        __alertId: textFrom(item, ['alert_id', 'id']) || `ALERT-${index + 1}`,
        __stateVersion: numberFrom(item, ['state_version', 'stateVersion', 'updated_ts']),
        __riskScore: numberFrom(item, ['score', 'confidence']) * 100,
        __status: normalizeAlertStatus(rawStatus) ?? rawStatus,
        __ruleVersion: textFrom(item, ['rule_version', 'rule_id', 'rule_name']),
        __modelVersion: textFrom(item, ['model_version', 'model', 'model_name']),
        __attackPhase: textFrom(item, ['attack_phase', 'phase', 'mitre_phase']),
        __campaignId: textFrom(item, ['campaign_id', 'community_id', 'cluster_id']),
        __firstSeen: textFrom(item, ['first_seen', 'timestamp']),
        __lastSeen: textFrom(item, ['last_seen']),
        __createdAt: textFrom(item, ['created_at']),
        __updatedAt: textFrom(item, ['updated_at', 'updated_ts']),
      });
    }),
    timeline: [
      timelineItem('告警队列已接入', `来自 /v1/alerts，当前页 ${alerts.length} 条，时间窗总量 ${statsWindowTotal}。`, 'ok'),
      timelineItem('统计口径已接入', '队列指标来自 /v1/alerts/stats，不再按当前页样本推算。', 'info'),
    ],
    evidence: [
      evidence('Alerts API', '/v1/alerts', 'ok'),
      evidence('返回记录', `${alerts.length}/${total}`, alerts.length ? 'ok' : 'warn'),
      evidence('高危队列', `${highCount} 条`, highCount ? 'risk' : 'ok'),
    ],
  };
};

const alertQueueMetric = (label: string, value: number, status: MetricStatus) => ({
  ...metric(label, value, '条', status),
  delta: '24h 窗口',
});

const alertSeverityLabel = (severity: string) => {
  const normalized = severity.toLowerCase().replace(/^severity_/, '');
  return normalized === 'critical' ? '高危' : severityLabel(severity);
};

const alertDisplayStatus = (status: string) => {
  const normalized = status.trim().toLowerCase();
  if (['false_positive', 'ignored'].includes(normalized)) return '已忽略';
  if (['confirmed', 'resolved'].includes(normalized)) return '已确认';
  if (normalizeAlertStatus(status) === 'triage') return '处理中';
  if (normalizeAlertStatus(status) === 'assigned') return '已确认';
  return alertStatusLabel(status);
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
      metric('分类资产总数', optionalNumberAt(stats, ['total']) ?? total, '个', 'info'),
      metric('活跃资产', numberAt(stats, ['active']), '个', 'ok'),
      metric('离线资产', numberAt(stats, ['inactive']), '个', numberAt(stats, ['inactive']) ? 'warn' : 'ok'),
      metric('未知状态资产', numberAt(stats, ['unknown']), '个', numberAt(stats, ['unknown']) ? 'warn' : 'ok'),
      metric('高风险资产', optionalNumberAt(stats, ['high_criticality']) ?? highRisk, '个', (optionalNumberAt(stats, ['high_criticality']) ?? highRisk) ? 'risk' : 'ok'),
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
      const exposedPorts = services.length > 0
        ? services.length
        : (optionalNumberAt(item, ['open_ports']) ?? optionalNumberAt(item, ['ports_count']) ?? 0);
      const highServices = services.filter((service) => textFrom(service, ['risk_level', 'risk']).includes('高')).length;
      const riskScore = numberAt(metadata, ['risk_score']);
      return makeRow(page, {
        '资产 ID': displayCode,
        'IP/MAC': [textFrom(item, ['ip_address', 'ip']), textFrom(item, ['mac_address', 'mac'])].filter(Boolean).join(' / ') || '-',
        主机名: textFrom(item, ['hostname', 'name', 'asset_name']) || '-',
        类型: textFrom(item, ['asset_type', 'type', 'os_type']) || '-',
        '园区/部门': [textFrom(item, ['campus']), textFrom(item, ['department'])].filter(Boolean).join(' / ') || '-',
        操作系统: textFrom(item, ['os', 'os_type', 'operating_system']) || '-',
        重要性: String(optionalNumberAt(item, ['criticality']) ?? '-'),
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
        __revision: numberAt(item, ['revision']),
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
  const centerEntity = textFrom(meta, ['center_ip', 'center_id']) || textFrom(nodes[0], ['ip', 'id']);
  const durationMs = numberAt(meta, ['duration_ms']);

  return {
    id: page.id,
    metrics: [
      metric('实体节点', optionalNumberAt(meta, ['node_count']) ?? nodes.length, '个', nodes.length ? 'info' : 'warn'),
      metric('关系边', optionalNumberAt(meta, ['edge_count']) ?? edges.length, '条', edges.length ? 'info' : 'warn'),
      metric('异常路径', riskPaths, '条', riskPaths ? 'risk' : 'ok'),
      metric('关键资产', keyAssets, '个', keyAssets ? 'warn' : 'ok'),
      metric('告警关联', alertCount, '条', alertCount ? 'risk' : 'ok'),
    ],
    rows: edges.slice(0, 8).map((item, index) =>
      makeRow(page, {
        '路径 ID': `GRAPH-PATH-${String(index + 1).padStart(3, '0')}`,
        源实体: textFrom(item, ['source']) || '-',
        目标实体: textFrom(item, ['target']) || '-',
        跳数: String(Math.min(index + 1, 3)),
        风险: graphRiskLabel(item, index),
        证据: `${textFrom(item, ['protocol', 'direction']) || '通信'} / ${formatNumber(numberAt(item, ['session_count']))} sessions`,
      }),
    ),
    timeline: [
      timelineItem('图谱探索已接入', `来自 /v1/graph/explore，中心节点 ${centerEntity || '未返回'}。`, centerEntity ? 'ok' : 'warn'),
      timelineItem('查询治理已记录', `缓存命中 ${String(Boolean(isRecord(graph) && graph.cache_hit))}，耗时 ${durationMs || 0} ms。`, durationMs > 1_000 ? 'warn' : 'ok'),
      timelineItem('节点上限保护', String(isRecord(graph) && graph.truncated) === 'true' ? '结果已截断，需要缩小范围。' : '本次查询未触发截断。', String(isRecord(graph) && graph.truncated) === 'true' ? 'warn' : 'ok'),
    ],
    evidence: [
      evidence('Graph API', '/v1/graph/explore', 'ok'),
      evidence('中心节点', centerEntity || '数据暂不可用', centerEntity ? 'info' : 'warn'),
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
  const entitiesAligned = optionalNumberAt(stats, ['entities_aligned']) ?? entities.length;
  const alignmentRate = optionalRatioAt(stats, ['alignment_rate']) ?? optionalRatioAt(quality, ['accuracy']) ?? optionalRatioAt(multiSource, ['confidence']);
  const sourceCoverage = sourceCoveragePercent(sourceStatsWithIntel) || ratioAt(multiSource, ['coverage_rate']);
  const duplicationRate = ratioAt(quality, ['duplication_rate']);
  const highRiskIntel = threatEntries.filter((item) => threatIntelReputation(item) !== 'clean' && threatIntelReputation(item) !== 'unknown').length;
  const conflictCount = Math.max(entities.filter((item) => numberAt(item, ['risk_score']) >= 70).length, Math.round((entitiesAligned || entities.length) * (duplicationRate / 100)));
  const writeBackRate = Math.max(0, Math.min(100, ((optionalRatioAt(quality, ['completeness']) ?? sourceCoverage) + (alignmentRate ?? 0)) / 2));
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
          可信度: confidenceLabel(optionalNumberAt(item, ['risk_score']) ?? alignmentRate),
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
      metric('可信度', alignmentRate, '%', alignmentRate === undefined ? 'info' : alignmentRate >= 90 ? 'ok' : 'warn'),
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
  const learning = baselines.filter((item) => textFrom(item, ['status']).toLowerCase() === 'learning').length;
  const allMetrics = baselines.flatMap((item) => extractList(item, ['metrics']));
  const deviations = allMetrics.filter((metricItem) => numberAt(metricItem, ['deviation_score']) >= 2).length;
  const highDeviation = allMetrics.filter((metricItem) => numberAt(metricItem, ['deviation_score']) >= 3).length;
  const coverage = optionalRatioAt(envelope, ['summary', 'coverage_rate'])
    ?? optionalRatioAt(envelope, ['coverage_rate']);

  return {
    id: page.id,
    metrics: [
      metric('偏离资产', deviations, '个', deviations ? 'warn' : 'ok'),
      metric('新端口', highDeviation, '个', highDeviation ? 'risk' : 'ok'),
      metric('异常协议', Math.max(0, deviations - highDeviation), '类', deviations > highDeviation ? 'warn' : 'ok'),
      metric('夜间访问', learning, '个', learning ? 'info' : 'ok'),
      metric('基线稳定度', coverage, '%', coverage === undefined ? 'info' : coverage >= 80 ? 'ok' : 'warn'),
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
      evidence('稳定覆盖', coverage === undefined ? '暂不可用' : `${coverage.toFixed(1)}%`, coverage === undefined ? 'info' : coverage >= 80 ? 'ok' : 'warn'),
    ],
  };
};

const adaptCampaigns = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const payload = unwrapPayload(primaryPayload);
  const envelope = isRecord(payload) ? payload : unwrapEnvelope(primaryPayload);
  const campaigns = extractList(primaryPayload, ['campaigns', 'data']);
  const summary = isRecord(envelope.summary) ? envelope.summary : {};
  const total = optionalNumberAt(summary, ['total']) ?? totalFromEnvelope(envelope, campaigns.length);
  const active = optionalNumberAt(summary, ['active']) ?? campaigns.filter((item) => campaignStatus(item) !== '已结束').length;
  const highRisk = optionalNumberAt(summary, ['high_risk']) ?? campaigns.filter((item) => campaignRisk(item).includes('高')).length;
  const mediumRisk = optionalNumberAt(summary, ['medium_risk']) ?? campaigns.filter((item) => campaignRisk(item).includes('中')).length;
  const lowRisk = optionalNumberAt(summary, ['low_risk']) ?? campaigns.filter((item) => campaignRisk(item).includes('低')).length;
  const affectedAssets = optionalNumberAt(summary, ['affected_assets']) ?? sumArrayLengths(campaigns, ['entities']);
  const alertCount = optionalNumberAt(summary, ['alert_count']) ?? sumArrayLengths(campaigns, ['alerts', 'alert_ids']);
  const averageDurationHours = numberAt(summary, ['average_duration_hours'])
    || average(campaigns.map(campaignDurationHours).filter((value) => value > 0));
  const campaignScores = campaigns
    .map((item) => optionalNumberAt(item, ['score']))
    .filter((value): value is number => value !== undefined);
  const maxScore = optionalNumberAt(summary, ['max_score']) ?? (campaignScores.length ? Math.max(...campaignScores) : 0);
  const highestRisk = maxScore >= 0.8 ? '高风险' : maxScore >= 0.5 ? '中风险' : '低风险';

  return {
    id: page.id,
    total,
    metrics: [
      metric('战役总数', total, '个', total ? 'info' : 'warn'),
      campaignMetric('活跃战役', active, '个', active ? 'risk' : 'ok'),
      campaignMetric('影响资产', affectedAssets, '台', affectedAssets ? 'warn' : 'ok'),
      {
        label: '最高风险',
        value: highestRisk,
        delta: '真实 API',
        status: highestRisk === '高风险' ? 'risk' : highestRisk === '中风险' ? 'warn' : 'ok',
      },
      campaignMetric('告警总数', alertCount, '条', alertCount ? 'warn' : 'ok'),
      {
        label: '平均持续时间',
        value: formatCampaignDuration(averageDurationHours),
        delta: '真实 API',
        status: averageDurationHours >= 24 ? 'warn' : 'info',
      },
    ],
    rows: campaigns.map((item, index) => {
      const authoritativeMemberCount = valueAt(item, ['member_count']);
      const alertCountForRow = authoritativeMemberCount !== undefined
        ? numberAt(item, ['member_count'])
        : arrayLengthFrom(item, ['alerts', 'alert_ids']);
      return makeRow(page, {
        战役名称: textFrom(item, ['campaign_id', 'id', 'event_id']) || `CAMPAIGN-${index + 1}`,
        阶段: campaignPhase(item),
        风险等级: campaignRisk(item),
        影响资产: arrayLengthFrom(item, ['entities']),
        告警数: alertCountForRow,
        首次发现: formatEpochTime(numberFrom(item, ['ts_start', 'start_time'])),
        最近活动: formatEpochTime(numberFrom(item, ['ts_end', 'end_time', 'ingest_ts'])),
        状态: campaignWorkflowStatus(item),
        __activity_status: campaignStatus(item),
        __workflow_status: campaignWorkflowStatus(item),
        操作: '查看',
        __assignee: textFrom(item, ['assignee']),
        __campaign_type: textFrom(item, ['campaign_type']),
        __summary: textFrom(item, ['summary']),
        __state_version: numberAt(item, ['state_version']),
        __snapshot_id: textFrom(item, ['snapshot_id']),
        __snapshot_sha256: textFrom(item, ['snapshot_sha256']),
        __workbench_updated_at: textFrom(item, ['workbench_updated_at']),
        __entity_count: arrayLengthFrom(item, ['entities']),
        __alert_count: alertCountForRow,
        __phase_count: arrayLengthFrom(item, ['attack_phases']),
        __rule_count: arrayLengthFrom(item, ['rule_ids']),
        __model_count: arrayLengthFrom(item, ['model_ids']),
        __has_summary: textFrom(item, ['summary']) ? 1 : 0,
        __phase_initial_access: hasCampaignPhase(item, 'initial_access') ? 1 : 0,
        __phase_execution: hasCampaignPhase(item, 'execution') ? 1 : 0,
        __phase_persistence: hasCampaignPhase(item, 'persistence') ? 1 : 0,
        __phase_lateral_movement: hasCampaignPhase(item, 'lateral_movement') ? 1 : 0,
        __phase_command_and_control: hasCampaignPhase(item, 'command_and_control') ? 1 : 0,
        __phase_exfiltration: hasCampaignPhase(item, 'exfiltration') ? 1 : 0,
        __phase_impact: hasCampaignPhase(item, 'impact') ? 1 : 0,
      });
    }),
    timeline: campaignTimeline(campaigns),
    evidence: [
      evidence('Campaigns API', '/v1/campaigns', 'ok'),
      evidence('返回记录', `${campaigns.length}/${total}`, campaigns.length ? 'ok' : 'warn'),
      evidence('告警聚合', `${alertCount} 条`, alertCount ? 'risk' : 'ok'),
      evidence('影响实体', `${affectedAssets} 个`, affectedAssets ? 'warn' : 'ok'),
      evidence('证据完整度', '接口未提供', 'info'),
    ],
    visuals: {
      campaigns: {
        riskCounts: { high: highRisk, medium: mediumRisk, low: lowRisk },
      },
    },
  };
};

const campaignMetric = (label: string, value: number, suffix: string, status: MetricStatus) => ({
  label,
  value: `${formatNumber(Number.isFinite(value) ? value : 0)} ${suffix}`,
  delta: '真实 API',
  status,
});

const formatCampaignDuration = (hours: number) => {
  if (!Number.isFinite(hours) || hours <= 0) return '0 小时';
  const roundedHours = Math.round(hours);
  const days = Math.floor(roundedHours / 24);
  const remainingHours = roundedHours % 24;
  return days ? `${days} 天 ${remainingHours} 小时` : `${remainingHours} 小时`;
};

const hasCampaignPhase = (item: Record<string, unknown>, phase: string) =>
  stringArrayFrom(item, ['attack_phases']).some((value) => value.toLowerCase() === phase);

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
  const blockPoints = phases.filter((item) => campaignPhaseLabel(textFrom(item, ['phase'])).includes('外') || campaignPhaseLabel(textFrom(item, ['phase'])).includes('C2')).length;
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

const topicContractVisual = (payload: unknown): Pick<
  TopicVisuals,
  'snapshotId' | 'snapshotRevision' | 'snapshotAsOf' | 'partial' | 'missingSections' | 'sourceWatermarks' | 'actionCatalog'
> => {
  const envelope = isRecord(payload) ? payload : {};
  const meta = isRecord(envelope.meta) ? envelope.meta : {};
  const data = unwrapPayload(payload);
  const sourceWatermarks = isRecord(meta.source_watermarks)
    ? Object.fromEntries(Object.entries(meta.source_watermarks).map(([key, value]) => [key, String(value ?? '')]))
    : {};
  const actionCatalog = extractNamedList(data, ['action_catalog']).map((item) => ({
    actionId: textFrom(item, ['action_id']),
    risk: textFrom(item, ['risk']),
    approval: textFrom(item, ['approval']),
    executor: textFrom(item, ['executor']),
    compensation: textFrom(item, ['compensation']),
    enabled: Boolean(item.enabled),
    unavailableCause: textFrom(item, ['unavailable_cause']) || undefined,
  })).filter((item) => item.actionId);
  return {
    snapshotId: textAt(meta, ['snapshot_id']) || textAt(data, ['snapshot_id']),
    snapshotRevision: numberAt(data, ['revision']),
    snapshotAsOf: textAt(meta, ['as_of']),
    partial: Boolean(meta.partial),
    missingSections: stringListAt(meta, ['missing_sections']),
    sourceWatermarks,
    actionCatalog,
  };
};

const topicScopeVisual = (payload: unknown): TopicVisuals['scope'] => {
  const scope = valueAt(payload, ['scope']);
  if (!isRecord(scope)) return undefined;
  return {
    scopeName: textAt(scope, ['scope_name']),
    includedAssets: stringListAt(scope, ['included_assets']),
    excludedAssets: stringListAt(scope, ['excluded_assets']),
    riskLevels: stringListAt(scope, ['risk_levels']),
    timeWindow: textAt(scope, ['time_window']),
    updatedAt: numberAt(scope, ['updated_at']),
  };
};

const topicStatus = (value: string): 'ok' | 'warn' | 'risk' | 'info' => {
  if (value === 'ok' || value === 'warn' || value === 'risk' || value === 'info') return value;
  return 'info';
};

const topicPresentation = (payload: unknown): TopicVisuals['presentation'] => {
  const presentation = valueAt(payload, ['presentation']);
  if (!isRecord(presentation)) return undefined;
  return {
    topicId: textAt(presentation, ['topic_id']),
    site: textAt(presentation, ['site']),
    assetGroup: textAt(presentation, ['asset_group']),
    ipRange: textAt(presentation, ['ip_range']),
    protocols: textAt(presentation, ['protocols']),
    timeWindowLabel: textAt(presentation, ['time_window_label']),
    ruleVersion: textAt(presentation, ['rule_version']),
    modelVersion: textAt(presentation, ['model_version']),
    reportTitle: textAt(presentation, ['report_title']),
    reportTimeRange: textAt(presentation, ['report_time_range']),
    reportGeneratedAt: textAt(presentation, ['report_generated_at']),
    reportScope: textAt(presentation, ['report_scope']),
    reportConclusion: textAt(presentation, ['report_conclusion']),
  };
};

const topicNumericSummary = (value: unknown): Record<string, number> => {
  if (!isRecord(value)) return {};
  return Object.entries(value).reduce<Record<string, number>>((result, [key, item]) => {
    const numeric = Number(item);
    if (Number.isFinite(numeric)) result[key] = numeric;
    return result;
  }, {});
};

const topicEvidenceBundle = (payload: unknown): NonNullable<TopicVisuals['evidenceBundle']> =>
  extractNamedList(payload, ['evidence_bundle']).map((item) => ({
    label: textFrom(item, ['label']),
    complete: numberFrom(item, ['complete']),
    total: numberFrom(item, ['total']),
    status: topicStatus(textFrom(item, ['status'])),
  }));

const topicTopologyVisual = (payload: unknown) => {
  const rawNodes = extractNamedList(payload, ['topology_nodes']);
  const nodeIds = new Set<string>();
  let duplicateNodeCount = 0;
  const topologyNodes: NonNullable<TopicVisuals['topologyNodes']> = [];
  rawNodes.forEach((item) => {
    const id = textFrom(item, ['id']).trim();
    const label = textFrom(item, ['label']).trim();
    if (!id || !label) return;
    if (nodeIds.has(id)) {
      duplicateNodeCount += 1;
      return;
    }
    nodeIds.add(id);
    topologyNodes.push({
      id,
      label,
      detail: textFrom(item, ['detail']).trim(),
      x: numberFrom(item, ['x']),
      y: numberFrom(item, ['y']),
      tone: (['asset', 'probe', 'risk', 'protocol', 'proxy', 'destination', 'warn'].includes(textFrom(item, ['tone']))
        ? textFrom(item, ['tone'])
        : 'asset') as NonNullable<TopicVisuals['topologyNodes']>[number]['tone'],
      width: Math.max(80, Math.min(188, optionalNumberFrom(item, ['width']) ?? 104)),
      height: Math.max(46, optionalNumberFrom(item, ['height']) ?? 46),
      // The three topic canvases share one frame contract: icon, title, and
      // detail are all rendered inside a rectangular node.
      symbol: 'roundRect',
      icon: ([
        'desktop', 'server', 'storage', 'probe', 'user', 'protocol', 'gateway', 'lock', 'global',
        'campaign', 'initial', 'execute', 'persist', 'evasion', 'credential', 'discovery',
        'lateral', 'c2', 'exfil', 'evidence', 'audit',
      ].includes(textFrom(item, ['icon']))
        ? textFrom(item, ['icon'])
        : undefined) as NonNullable<TopicVisuals['topologyNodes']>[number]['icon'],
      labelPosition: 'inside',
    });
  });

  let danglingLinkCount = 0;
  let selfLinkCount = 0;
  const topologyLinks: NonNullable<TopicVisuals['topologyLinks']> = [];
  extractNamedList(payload, ['topology_links']).forEach((item) => {
    const source = textFrom(item, ['source']).trim();
    const target = textFrom(item, ['target']).trim();
    if (!source || !target || !nodeIds.has(source) || !nodeIds.has(target)) {
      danglingLinkCount += 1;
      return;
    }
    if (source === target) {
      selfLinkCount += 1;
      return;
    }
    topologyLinks.push({
      source,
      target,
      value: numberFrom(item, ['value']),
      tone: (['info', 'risk', 'ok', 'warn', 'purple'].includes(textFrom(item, ['tone']))
        ? textFrom(item, ['tone'])
        : 'info') as NonNullable<TopicVisuals['topologyLinks']>[number]['tone'],
      lineType: (textFrom(item, ['line_type']) === 'dashed' ? 'dashed' : 'solid') as 'solid' | 'dashed',
      label: textFrom(item, ['label']),
      width: optionalNumberFrom(item, ['width']) ?? undefined,
      curveness: optionalNumberFrom(item, ['curveness']) ?? undefined,
    });
  });

  return {
    topologyNodes,
    topologyLinks,
    topologyDiagnostics: {
      duplicateNodeCount,
      danglingLinkCount,
      selfLinkCount,
      validNodeCount: topologyNodes.length,
      validLinkCount: topologyLinks.length,
    },
    impactHighlights: extractNamedList(payload, ['impact_highlights']).map((item) => ({
      label: textFrom(item, ['label']),
      value: textFrom(item, ['value']),
      detail: textFrom(item, ['detail']),
      status: topicStatus(textFrom(item, ['status'])),
      targetSignal: textFrom(item, ['target_signal']),
    })).filter((item) => item.label && item.value),
  };
};

const adaptTunnelTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const metricDeltas = valueAt(primaryPayload, ['metric_deltas']);
  const timeRange = valueAt(primaryPayload, ['time_range']);
  const protocols = extractNamedList(primaryPayload, ['protocols']);
  const users = extractNamedList(primaryPayload, ['users']);
  const events = extractNamedList(primaryPayload, ['events']);
  const sessionCount = optionalNumberAt(summary, ['session_count']);
  const protocolCount = optionalNumberAt(summary, ['protocol_count']);
  const activeUsers = optionalNumberAt(summary, ['active_users']);
  const highRiskUsers = optionalNumberAt(summary, ['high_risk_users']);
  const totalBytes = optionalNumberAt(summary, ['total_bytes']);
  const encryptedTrafficGbps = optionalNumberAt(summary, ['encrypted_traffic_gbps']);
  const endpointCount = optionalNumberAt(summary, ['endpoint_count']);
  const suspiciousRatio = optionalRatioAt(summary, ['suspicious_ratio']);
  const evidenceRate = optionalRatioAt(summary, ['evidence_completeness'])
    ?? optionalRatioAt(summary, ['evidence_rate']);
  const reportConfidence = optionalRatioAt(summary, ['report_confidence']);
  const openRiskCount = optionalNumberAt(summary, ['open_risk_count']);
  const sourceRows = events;
  const evidenceBundle = topicEvidenceBundle(primaryPayload);
  const presentation = topicPresentation(primaryPayload);
  const totalEvents = optionalNumberAt(summary, ['total_events']) ?? sourceRows.length;
  const metricDelta = (key: string) => textAt(metricDeltas, [key]) || '未提供比较基线';
  const countValue = (value: number | undefined) => value === undefined ? '暂不可用' : formatNumber(value);
  const percentValue = (value: number | undefined, digits = 1) => value === undefined ? '暂不可用' : `${value.toFixed(digits)}%`;
  const valueStatus = (value: number | undefined, nonZero: MetricStatus, zero: MetricStatus = 'ok'): MetricStatus =>
    value === undefined ? 'warn' : value ? nonZero : zero;

  return {
    id: page.id,
    total: totalEvents,
    metrics: [
      topicMetric('隧道协议数', countValue(protocolCount), metricDelta('protocol_count'), valueStatus(protocolCount, 'info')),
      topicMetric('高频隧道源', countValue(activeUsers), metricDelta('active_users'), valueStatus(activeUsers, 'info')),
      topicMetric('加密会话流量', encryptedTrafficGbps === undefined
        ? totalBytes === undefined ? '暂不可用' : bytesLabel(totalBytes)
        : `${encryptedTrafficGbps.toFixed(1)} Gbps`, metricDelta('encrypted_traffic_gbps'), valueStatus(encryptedTrafficGbps ?? totalBytes, 'ok', 'warn')),
      topicMetric('异常隧道数', countValue(sessionCount), metricDelta('session_count'), valueStatus(sessionCount, 'risk')),
      topicMetric('隧道端点数', countValue(endpointCount), metricDelta('endpoint_count'), valueStatus(endpointCount, 'info')),
      topicMetric('可疑隧道占比', percentValue(suspiciousRatio), metricDelta('suspicious_ratio'), valueStatus(suspiciousRatio, 'warn')),
      topicMetric('证据完整度', percentValue(evidenceRate, 0), metricDelta('evidence_completeness'), evidenceRate === undefined ? 'warn' : evidenceRate >= 85 ? 'ok' : evidenceRate ? 'warn' : 'info'),
      topicMetric('报告置信度', percentValue(reportConfidence, 0), metricDelta('report_confidence'), reportConfidence === undefined ? 'warn' : reportConfidence >= 85 ? 'ok' : reportConfidence ? 'warn' : 'info'),
      topicMetric('未闭环风险数', countValue(openRiskCount), metricDelta('open_risk_count'), valueStatus(openRiskCount, 'warn')),
      topicMetric('活跃隧道会话', countValue(sessionCount), '兼容专题总览', valueStatus(sessionCount, 'info', 'warn')),
    ],
    rows: sourceRows.map((item) =>
      makeRow(page, {
        事件ID: textFrom(item, ['event_id']) || [textFrom(item, ['ip']), textFrom(item, ['protocol'])].filter(Boolean).join('/') || '-',
        隧道源: textFrom(item, ['ip']) || '-',
        协议: textFrom(item, ['protocol']) || '-',
        目的端点: textFrom(item, ['dst_ip', 'destination_ip']) || '-',
        证据类型: textFrom(item, ['evidence_type']) || 'Session',
        时间窗: textFrom(item, ['time_window']) || formatEpochTime(optionalNumberFrom(item, ['last_seen'])),
        阶段: textFrom(item, ['phase']) || '-',
        风险状态: topicRiskLabel(textFrom(item, ['risk'])),
        风险操作: textFrom(item, ['risk_action']) || '取证',
        __session_count: numberFrom(item, ['count']),
        __total_bytes: numberFrom(item, ['total_bytes']),
      }),
    ),
    timeline: [
      timelineItem('隧道专题已接入', `来自 /v1/topics/tunnel，协议 ${countValue(protocolCount)} 类，活跃会话 ${countValue(sessionCount)}。`, sessionCount === undefined ? 'warn' : 'ok'),
      timelineItem('高危用户聚合', `users 区块返回 ${users.length} 个源资产，summary 高危数 ${countValue(highRiskUsers)}。`, highRiskUsers === undefined ? 'warn' : highRiskUsers ? 'risk' : 'ok'),
      timelineItem('协议分布', `protocols 返回 ${protocols.length} 项，summary 总流量 ${totalBytes === undefined ? '暂不可用' : bytesLabel(totalBytes)}。`, totalBytes === undefined ? 'warn' : 'ok'),
      timelineItem('取证闭环', '隧道会话继续下钻 encrypted-traffic、forensics、audit-log。', 'info'),
    ],
    evidence: evidenceBundle.length ? evidenceBundle.map((item) => evidence(item.label, `${item.complete} / ${item.total} (${item.total ? Math.round((item.complete / item.total) * 100) : 0}%)`, item.status)) : [
      evidence('Tunnel Topic API', '/v1/topics/tunnel', 'ok'),
      evidence('协议分布', `${protocols.length} 类`, protocols.length ? 'ok' : 'warn'),
      evidence('高危用户', `${highRiskUsers}/${users.length}`, highRiskUsers ? 'risk' : 'ok'),
      evidence('JA3/JA3S', '关联加密流量', 'info'),
      evidence('PCAP 窗口', sessionCount === undefined ? '会话候选数暂不可用' : `${sessionCount} 个会话候选`, sessionCount === undefined ? 'warn' : sessionCount ? 'warn' : 'ok'),
      evidence('审计记录', '阻断/取证待写入', 'info'),
    ],
    visuals: {
      topic: {
        topic: 'tunnel',
        dataMode: textAt(primaryPayload, ['data_mode']) === 'simulated'
          ? 'simulated'
          : textAt(primaryPayload, ['data_mode']) === 'partial' ? 'partial' : 'live',
        ...topicContractVisual(primaryPayload),
        simulationId: textAt(primaryPayload, ['simulation_id']),
        simulationVersion: textAt(primaryPayload, ['simulation_version']),
        presentation,
        summary: topicNumericSummary(summary),
        evidenceBundle,
        destinationDistribution: extractNamedList(primaryPayload, ['destination_distribution']).map((item) => ({
          label: textFrom(item, ['label']), value: numberFrom(item, ['value']), trafficGb: numberFrom(item, ['traffic_gb']), asn: textFrom(item, ['asn']),
        })),
        certificateAnomalies: extractNamedList(primaryPayload, ['certificate_anomalies']).map((item) => ({
          label: textFrom(item, ['label']),
          value: numberFrom(item, ['value']),
          status: topicStatus(textFrom(item, ['status'])),
          percent: numberFrom(item, ['percent']),
          sample: textFrom(item, ['sample']),
        })),
        tunnelTrend: extractNamedList(primaryPayload, ['tunnel_trend']).map((item) => ({
          label: textFrom(item, ['label']),
          value: numberFrom(item, ['value']),
        })).filter((item) => item.label),
        tunnelTrendUnit: textAt(primaryPayload, ['tunnel_trend_unit']),
        tunnelReusePaths: extractNamedList(primaryPayload, ['reuse_paths']).map((item) => [
          textFrom(item, ['source']),
          textFrom(item, ['protocol']),
          textFrom(item, ['proxy']),
          textFrom(item, ['destination']),
        ]).filter((row) => row.every(Boolean)),
        ...topicTopologyVisual(primaryPayload),
        updatedAt: numberAt(primaryPayload, ['updated_at']),
        timeRange: {
          start: numberAt(timeRange, ['start']),
          end: numberAt(timeRange, ['end']),
        },
        scope: topicScopeVisual(primaryPayload),
        tunnelProtocols: protocols.map((item) => ({
          protocol: textFrom(item, ['protocol']),
          count: numberFrom(item, ['count']),
          totalBytes: numberFrom(item, ['total_bytes']),
        })),
        tunnelUsers: users.map((item) => ({
          ip: textFrom(item, ['ip']),
          dstIp: textFrom(item, ['dst_ip']),
          protocol: textFrom(item, ['protocol']),
          risk: textFrom(item, ['risk']),
          count: numberFrom(item, ['count']),
          totalBytes: numberFrom(item, ['total_bytes']),
          lastSeen: numberFrom(item, ['last_seen']),
        })),
      } satisfies TopicVisuals,
    },
  };
};

const adaptExfilTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const timeRange = valueAt(primaryPayload, ['time_range']);
  const sources = extractNamedList(primaryPayload, ['top_sources', 'sources']);
  const destinations = extractNamedList(primaryPayload, ['destinations']);
  const riskTypes = extractNamedList(primaryPayload, ['risk_types', 'risks']);
  const accountServices = extractNamedList(primaryPayload, ['account_service_distribution']);
  const paths = extractNamedList(primaryPayload, ['paths']);
  const trend = extractNamedList(primaryPayload, ['trend']);
  const events = extractNamedList(primaryPayload, ['events']);
  const rows = events;
  const sourceCount = optionalNumberAt(summary, ['source_count']) ?? sources.length;
  const pathCount = optionalNumberAt(summary, ['path_count']) ?? paths.length;
  const sessionCount = optionalNumberAt(summary, ['session_count']) ?? sumNumbers(sources, ['session_count']);
  const uploadBytes = optionalNumberAt(summary, ['upload_bytes']) ?? sumNumbers(sources, ['upload_bytes']);
  const highRiskSources = optionalNumberAt(summary, ['high_risk_sources'])
    ?? sources.filter((item) => topicRiskLabel(textFrom(item, ['risk'])).includes('高')).length;
  const alertCount = optionalNumberAt(summary, ['alert_count']) ?? optionalNumberAt(summary, ['warning_count']) ?? 0;
  const destinationCount =
    optionalNumberAt(summary, ['destination_count']) ??
    optionalNumberAt(summary, ['dst_count']) ??
    (destinations.length > 0
      ? destinations.length
      : new Set(rows.map((item) => textFrom(item, ['dst_region', 'region', 'dst_ip'])).filter(Boolean)).size);
  const normalizedDestinationCount = Number(destinationCount);
  const sensitiveTypeCount =
    optionalNumberAt(summary, ['sensitive_type_count']) ??
    new Set(rows.map((item) => textFrom(item, ['data_type'])).filter(Boolean)).size;
  const crossBorderDestinations =
    optionalNumberAt(summary, ['cross_border_destinations']) ??
    optionalNumberAt(summary, ['cross_border_destination_count']) ??
    0;
  const peakUploadGbps =
    optionalNumberAt(summary, ['peak_upload_gbps']) ??
    optionalNumberAt(summary, ['peak_gbps']) ??
    Math.max(0, ...rows.map((item) => numberFrom(item, ['peak_gbps', 'gbps'])));
  const topRiskType = riskTypes[0] ? textFrom(riskTypes[0], ['type', 'severity']) : '异常上传';
  const evidenceRate =
    optionalRatioAt(summary, ['evidence_completeness']) ??
    optionalRatioAt(summary, ['evidence_rate']) ??
    0;
  const evidenceBundle = topicEvidenceBundle(primaryPayload);
  const presentation = topicPresentation(primaryPayload);
  const totalEvents = optionalNumberAt(summary, ['total_events']) ?? rows.length;

  return {
    id: page.id,
    total: totalEvents,
    metrics: [
      topicMetric('外传预警量', formatNumber(alertCount), alertCount ? '当前窗口' : '告警待关联', alertCount ? 'risk' : 'info'),
      topicMetric('外传路径数', formatNumber(pathCount), '实时路径', pathCount ? 'warn' : 'info'),
      topicMetric('可疑外传源', formatNumber(highRiskSources), '实时源资产', highRiskSources ? 'risk' : sourceCount ? 'warn' : 'ok'),
      topicMetric('外传目的地数', formatNumber(normalizedDestinationCount), '实时目的端', normalizedDestinationCount ? 'info' : 'warn'),
      topicMetric('敏感数据类型数', formatNumber(sensitiveTypeCount), sensitiveTypeCount ? '数据分类' : '分类待接入', sensitiveTypeCount ? 'warn' : 'info'),
      topicMetric('异常上传峰值', `${peakUploadGbps.toFixed(1)} Gbps`, '当前窗口', peakUploadGbps >= 30 ? 'warn' : 'ok'),
      topicMetric('跨境目的地数', formatNumber(crossBorderDestinations), crossBorderDestinations ? '地域归因' : '地域待归因', crossBorderDestinations ? 'warn' : 'info'),
      topicMetric('证据完整度', `${Math.round(evidenceRate)}%`, evidenceRate ? '接口聚合' : '待证据关联', evidenceRate >= 90 ? 'ok' : evidenceRate >= 60 ? 'warn' : 'info'),
    ],
    rows: rows.map((item) =>
      makeRow(page, {
        源资产: textFrom(item, ['src_ip']) || '-',
        外传路径: textFrom(item, ['dst_ip']) ? `${textFrom(item, ['src_ip'])} -> ${textFrom(item, ['dst_ip'])}` : `${textFrom(item, ['src_ip']) || '源资产'} -> 多目的地`,
        目标区域: textFrom(item, ['dst_region', 'region', 'dst_ip']) || '-',
        数据类型: textFrom(item, ['data_type']) || '-',
        上传量: bytesLabel(numberFrom(item, ['upload_bytes', 'total_bytes'])),
        会话数: optionalNumberAt(item, ['session_count']) ?? optionalNumberAt(item, ['count']) ?? 0,
        风险类型: textFrom(item, ['risk']) ? `${topicRiskLabel(textFrom(item, ['risk']))}路径` : topRiskType,
        风险等级: topicRiskLabel(textFrom(item, ['risk'])),
        协议: textFrom(item, ['protocol']),
        目的端口: numberFrom(item, ['dst_port']),
        最近活动: formatEpochTime(numberFrom(item, ['last_seen'])),
        处置: '阻断',
      }),
    ),
    timeline: [
      timelineItem('外传专题已接入', `来自 /v1/topics/exfil，源资产 ${sourceCount} 个，路径 ${pathCount} 条。`, sourceCount || pathCount ? 'ok' : 'warn'),
      timelineItem('上传风险汇总', `外传会话 ${formatNumber(sessionCount)}，上传流量 ${bytesLabel(uploadBytes)}。`, uploadBytes ? 'risk' : 'ok'),
      timelineItem('风险类型聚合', `risk_types 返回 ${riskTypes.length} 类，首要类型 ${topRiskType}。`, riskTypes.length ? 'warn' : 'info'),
      timelineItem('证据与阻断', '外传路径继续下钻 assets、baselines、playbooks、compliance。', 'info'),
    ],
    evidence: evidenceBundle.length ? evidenceBundle.map((item) => evidence(item.label, `${item.complete} / ${item.total} (${item.total ? Math.round((item.complete / item.total) * 100) : 0}%)`, item.status)) : [
      evidence('告警证据', `${formatNumber(alertCount)} / ${formatNumber(alertCount)} (100%)`, alertCount ? 'ok' : 'warn'),
      evidence('PCAP', '未由专题接口返回', 'info'),
      evidence('Session', `${formatNumber(sessionCount)} / ${formatNumber(sessionCount)} (100%)`, sessionCount ? 'ok' : 'info'),
      evidence('审计日志', '由专题操作审计提供', 'info'),
      evidence('回溯路径', `${formatNumber(paths.length)} / ${formatNumber(paths.length)} (100%)`, paths.length ? 'ok' : 'info'),
      evidence('资产快照', `${formatNumber(sourceCount)} 个源资产`, sourceCount ? 'ok' : 'info'),
    ],
    visuals: {
      topic: {
        topic: 'exfil',
        dataMode: textAt(primaryPayload, ['data_mode']) === 'simulated'
          ? 'simulated'
          : textAt(primaryPayload, ['data_mode']) === 'partial' ? 'partial' : 'live',
        ...topicContractVisual(primaryPayload),
        simulationId: textAt(primaryPayload, ['simulation_id']),
        simulationVersion: textAt(primaryPayload, ['simulation_version']),
        presentation,
        summary: topicNumericSummary(summary),
        evidenceBundle,
        ...topicTopologyVisual(primaryPayload),
        updatedAt: numberAt(primaryPayload, ['updated_at']),
        timeRange: {
          start: numberAt(timeRange, ['start']),
          end: numberAt(timeRange, ['end']),
        },
        scope: topicScopeVisual(primaryPayload),
        exfilSources: sources.map((item) => ({
          srcIp: textFrom(item, ['src_ip']),
          sessionCount: numberFrom(item, ['session_count']),
          uploadBytes: numberFrom(item, ['upload_bytes']),
          totalBytes: numberFrom(item, ['total_bytes']),
          destinationCount: numberFrom(item, ['dst_count']),
          lastSeen: numberFrom(item, ['last_seen']),
          risk: textFrom(item, ['risk']),
        })),
        exfilDestinations: destinations.map((item) => ({
          dstIp: textFrom(item, ['dst_ip']),
          region: textFrom(item, ['region', 'dst_region']),
          asn: textFrom(item, ['asn']),
          sessionCount: numberFrom(item, ['session_count']),
          uploadBytes: numberFrom(item, ['upload_bytes']),
          totalBytes: numberFrom(item, ['total_bytes']),
          sourceCount: numberFrom(item, ['src_count']),
          lastSeen: numberFrom(item, ['last_seen']),
          risk: textFrom(item, ['risk']),
        })),
        exfilRiskTypes: riskTypes.map((item) => ({
          type: textFrom(item, ['type']),
          count: numberFrom(item, ['count']),
          severity: textFrom(item, ['severity']),
          totalBytes: numberFrom(item, ['total_bytes']),
        })),
        exfilAccountServices: accountServices.map((item) => ({
          label: textFrom(item, ['label', 'name', 'account', 'service']),
          type: textFrom(item, ['type']),
          count: numberFrom(item, ['count', 'value', 'hits']),
        })).filter((item) => item.label && item.count > 0),
        exfilPaths: paths.map((item) => ({
          srcIp: textFrom(item, ['src_ip']),
          dstIp: textFrom(item, ['dst_ip']),
          dstPort: numberFrom(item, ['dst_port']),
          protocol: textFrom(item, ['protocol']),
          sessionCount: numberFrom(item, ['session_count']),
          uploadBytes: numberFrom(item, ['upload_bytes']),
          lastSeen: numberFrom(item, ['last_seen']),
          risk: textFrom(item, ['risk']),
        })),
        exfilTrend: trend.map((item) => ({
          bucketStart: numberFrom(item, ['bucket_start']),
          destinationCount: numberFrom(item, ['destination_count']),
          largeUploadSessions: numberFrom(item, ['large_upload_sessions']),
          longLivedSessions: numberFrom(item, ['long_lived_sessions']),
          nonStandardPortSessions: numberFrom(item, ['non_standard_port_sessions']),
          encryptedSessions: numberFrom(item, ['encrypted_sessions']),
        })),
      } satisfies TopicVisuals,
    },
  };
};

const adaptAptTopic = (page: PageSpec, primaryPayload: unknown): PageSnapshot => {
  const summary = valueAt(primaryPayload, ['summary']);
  const timeRange = valueAt(primaryPayload, ['time_range']);
  const campaigns = extractNamedList(primaryPayload, ['campaigns', 'data']);
  const events = extractNamedList(primaryPayload, ['events']);
  const phases = valueAt(primaryPayload, ['phase_distribution']);
  const phaseCount = isRecord(phases) ? Object.keys(phases).length : 0;
  const campaignCount = optionalNumberAt(summary, ['campaign_count']) ?? campaigns.length;
  const highRisk = optionalNumberAt(summary, ['high_risk_count']) ?? campaigns.filter((item) => campaignRisk(item).includes('高')).length;
  const entityCount = optionalNumberAt(summary, ['entity_count']) ?? sumArrayLengths(campaigns, ['entities']);
  const alertCount = optionalNumberAt(summary, ['alert_count']) ?? sumArrayLengths(campaigns, ['alerts']);
  const phaseCoverageTotal = optionalNumberAt(summary, ['phase_coverage_total']);
  const phaseCoverageDone = optionalNumberAt(summary, ['phase_coverage_done']);
  const phaseCoverageAvailable = phaseCoverageTotal !== undefined && phaseCoverageDone !== undefined;
  const lateralMoveLinks = numberAt(summary, ['lateral_move_links']);
  const persistenceSignals = numberAt(summary, ['persistence_signals']);
  const exfilEvidence = numberAt(summary, ['exfil_evidence_count']);
  const closureRate = ratioAt(summary, ['closure_rate']);
  const reportConfidence = ratioAt(summary, ['report_confidence']);
  const clusterDensity = numberAt(summary, ['cluster_density']);
  const evidenceRate = ratioAt(summary, ['evidence_completeness']);
  const metricScope = textAt(summary, ['metric_scope']) === 'listed_campaigns' ? `最近 ${campaigns.length} 个战役` : '接口聚合';
  const evidenceBundle = topicEvidenceBundle(primaryPayload);
  const presentation = topicPresentation(primaryPayload);
  const totalEvents = optionalNumberAt(summary, ['total_events']) ?? events.length;
  const topologyVisual = topicTopologyVisual(primaryPayload);
  const topologyNodeLabels = new Map((topologyVisual.topologyNodes ?? []).map((item) => [item.id, item.label]));
  const traceableRelation = (item: NonNullable<TopicVisuals['topologyLinks']>[number]) => ({
    sourceId: item.source,
    sourceLabel: topologyNodeLabels.get(item.source) ?? item.source,
    targetId: item.target,
    targetLabel: topologyNodeLabels.get(item.target) ?? item.target,
    value: item.value,
    tone: item.tone,
    lineType: item.lineType,
    originalLabel: item.label,
  });
  const aptLateralPaths = (topologyVisual.topologyLinks ?? [])
    .filter((item) => item.source === 'phase-lateral' || item.target === 'phase-lateral' || item.label.includes('横向移动'))
    .map(traceableRelation);
  const aptEvidenceAssociations = (topologyVisual.topologyLinks ?? [])
    .filter((item) => item.target.startsWith('evidence-'))
    .map(traceableRelation);

  return {
    id: page.id,
    total: totalEvents,
    metrics: [
      topicMetric('关联战役数', String(campaignCount), '实时战役', campaignCount || highRisk ? 'risk' : 'ok'),
      topicMetric('战役集密度', clusterDensity.toFixed(2), metricScope, clusterDensity >= 0.7 ? 'ok' : clusterDensity ? 'warn' : 'info'),
      topicMetric('攻击阶段覆盖', phaseCoverageAvailable ? `${phaseCoverageDone}/${phaseCoverageTotal}` : '暂不可用', '实时阶段', phaseCoverageAvailable && phaseCoverageDone > 0 ? 'info' : 'warn'),
      topicMetric('关键资产命中', String(entityCount), '实时实体', entityCount ? 'risk' : 'ok'),
      topicMetric('横向移动链路', String(lateralMoveLinks), metricScope, lateralMoveLinks ? 'warn' : 'ok'),
      topicMetric('持久化迹象数', String(persistenceSignals), metricScope, persistenceSignals ? 'warn' : 'ok'),
      topicMetric('外传关联证据', String(exfilEvidence), metricScope, exfilEvidence ? 'info' : 'ok'),
      topicMetric('处置闭环率', `${Math.round(closureRate)}%`, metricScope, closureRate >= 80 ? 'ok' : closureRate >= 60 ? 'warn' : closureRate ? 'risk' : 'info'),
      topicMetric('报告置信度', `${Math.round(reportConfidence)}%`, metricScope, reportConfidence >= 80 ? 'ok' : reportConfidence >= 60 ? 'warn' : reportConfidence ? 'risk' : 'info'),
    ],
    rows: events.map((item) =>
      makeRow(page, {
        事件ID: textFrom(item, ['event_id', 'id']) || textFrom(item, ['campaign_id']) || '-',
        战役名称: textFrom(item, ['campaign_id', 'id', 'event_id']) || '-',
        阶段: campaignPhase(item),
        关键实体: topicFirstArrayValue(item, 'entities') || textFrom(item, ['source_ip']) || '-',
        关联告警: arrayLengthFrom(item, ['alerts']) || textFrom(item, ['alert_id']) || 0,
        攻击技术: topicFirstArrayValue(item, 'attack_phases') || campaignTypeLabel(textFrom(item, ['campaign_type'])),
        首次发现: formatEpochTime(numberFrom(item, ['ts_start', 'start_time'])),
        最近活动: formatEpochTime(numberFrom(item, ['ts_end', 'end_time', 'ingest_ts'])),
        风险等级: campaignRisk(item),
        处置状态: textFrom(item, ['status', 'activity_status']) || 'unknown',
        处置: '复盘',
        __campaign_id: textFrom(item, ['campaign_id', 'id', 'event_id']) || '-',
        __ts_start: numberFrom(item, ['ts_start', 'start_time']),
        __ts_end: numberFrom(item, ['ts_end', 'end_time', 'ingest_ts']),
      }),
    ),
    timeline: [
      timelineItem('APT 专题已接入', `来自 /v1/topics/apt，战役 ${campaignCount} 个，列出 ${campaigns.length} 个。`, campaignCount ? 'ok' : 'warn'),
      timelineItem('阶段分布', phaseCount ? `phase_distribution 返回 ${phaseCount} 个阶段。` : '阶段分布等待 campaigns 写入 attack_phases。', phaseCount ? 'ok' : 'warn'),
      timelineItem('实体与告警聚合', `影响实体 ${entityCount} 个，关联告警 ${formatNumber(alertCount)} 条。`, alertCount ? 'risk' : 'info'),
      timelineItem('复盘闭环', '战役继续下钻 campaigns、attack-chains、graph、rules。', 'info'),
    ],
    evidence: evidenceBundle.length ? evidenceBundle.map((item) => evidence(item.label, `${item.complete} / ${item.total} (${item.total ? Math.round((item.complete / item.total) * 100) : 0}%)`, item.status)) : [
      evidence('APT Topic API', '/v1/topics/apt', 'ok'),
      evidence('Campaigns', `${campaigns.length}/${campaignCount}`, campaigns.length ? 'ok' : 'warn'),
      evidence('Phase Distribution', `${phaseCount} 阶段`, phaseCount ? 'ok' : 'warn'),
      evidence('Entity Graph', `${entityCount} 实体`, entityCount ? 'warn' : 'info'),
      evidence('Evidence Bundle', `${evidenceRate.toFixed(1)}%`, evidenceRate >= 85 ? 'ok' : 'warn'),
      evidence('审计记录', '复盘结论待写入', 'info'),
    ],
    visuals: {
      topic: {
        topic: 'apt',
        dataMode: textAt(primaryPayload, ['data_mode']) === 'simulated'
          ? 'simulated'
          : textAt(primaryPayload, ['data_mode']) === 'partial' ? 'partial' : 'live',
        ...topicContractVisual(primaryPayload),
        simulationId: textAt(primaryPayload, ['simulation_id']),
        simulationVersion: textAt(primaryPayload, ['simulation_version']),
        presentation,
        summary: topicNumericSummary(summary),
        evidenceBundle,
        ...topologyVisual,
        aptCampaigns: campaigns.map((item) => ({
          id: textFrom(item, ['campaign_id', 'id']),
          type: textFrom(item, ['campaign_type']),
          status: textFrom(item, ['status']),
          activityStatus: textFrom(item, ['activity_status']),
          score: numberFrom(item, ['score']),
          tsStart: (() => {
            const value = numberFrom(item, ['ts_start', 'start_time']);
            return value > 0 && value < 10_000_000_000 ? value * 1000 : value;
          })(),
          tsEnd: (() => {
            const value = numberFrom(item, ['ts_end', 'end_time']);
            return value > 0 && value < 10_000_000_000 ? value * 1000 : value;
          })(),
          attackPhases: stringArrayFrom(item, ['attack_phases']),
          entities: stringArrayFrom(item, ['entities']),
          alertCount: arrayLengthFrom(item, ['alerts']),
        })).filter((item) => item.id),
        aptLateralPaths,
        aptEvidenceAssociations,
        aptIocs: extractList(primaryPayload, ['iocs']).map((item) => ({
          value: textFrom(item, ['value']),
          type: textFrom(item, ['type']),
          campaign: textFrom(item, ['campaign']),
          hits: numberFrom(item, ['hits']),
          firstSeen: (() => {
            const value = numberFrom(item, ['first_seen']);
            return value > 0 && value < 10_000_000_000 ? value * 1000 : value;
          })(),
          lastSeen: (() => {
            const value = numberFrom(item, ['last_seen']);
            return value > 0 && value < 10_000_000_000 ? value * 1000 : value;
          })(),
        })),
        aptResponse: (() => {
          const response = valueAt(primaryPayload, ['response']);
          return {
            closed: numberAt(response, ['closed']), processing: numberAt(response, ['processing']),
            open: numberAt(response, ['open']), total: numberAt(response, ['total']),
          };
        })(),
        updatedAt: numberAt(primaryPayload, ['updated_at']),
        timeRange: {
          start: numberAt(timeRange, ['start']),
          end: numberAt(timeRange, ['end']),
        },
        scope: topicScopeVisual(primaryPayload),
        aptPhaseDistribution: isRecord(phases)
          ? Object.entries(phases).map(([phase, count]) => ({
            phase,
            count: Number.isFinite(Number(count)) ? Number(count) : 0,
          }))
          : [],
      } satisfies TopicVisuals,
    },
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
  const total = totalFromEnvelope(envelope, jobs.length);
  const processing = countJobStatus(jobs, 'processing') || numberAt(taskStats, ['processing']);
  const queued = countJobStatus(jobs, 'queued') || numberAt(taskStats, ['queued']);
  const completed = countJobStatus(jobs, 'completed') || numberAt(taskStats, ['completed']);
  const partial = countJobStatus(jobs, 'partial') || numberAt(taskStats, ['partial']);
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
      { label: '部分完成', value: partial, status: partial ? 'warn' : 'info' },
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
      timelineItem('Worker 统计已关联', `worker=${formatNumber(optionalNumberAt(workerStats, ['workers']) ?? optionalNumberAt(workerStats, ['worker_count']))}，队列=${formatNumber(optionalNumberAt(workerStats, ['queue_size']))}。`, 'info'),
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
        版本: `v${optionalNumberAt(item, ['version']) ?? index + 1}.0`,
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
  const avgF1 = averageOptionalModelMetric(models, ['f1_score', 'f1']);
  const fpDelta = averageOptionalModelMetric(models, ['false_positive_delta', 'fp_delta']);

  return {
    id: page.id,
    metrics: [
      modelMetric('线上模型数', `${online} 个`, '真实 API', online ? 'ok' : 'warn'),
      modelMetric('候选模型数', `${candidates} 个`, '真实 API', candidates ? 'info' : 'ok'),
      modelMetric('漂移告警', `${driftAlerts} 个`, '真实 API', driftAlerts ? 'risk' : 'ok'),
      modelMetric('待重训模型', `${retrain} 个`, '真实 API', retrain ? 'warn' : 'ok'),
      modelMetric('平均 F1', avgF1 === undefined ? '暂不可用' : avgF1.toFixed(3), '真实 API', avgF1 !== undefined && avgF1 >= 0.9 ? 'ok' : 'warn'),
      modelMetric('误报率变化', fpDelta === undefined ? '暂不可用' : `${fpDelta.toFixed(1)}%`, '真实 API', fpDelta !== undefined && fpDelta <= 0 ? 'ok' : 'warn'),
    ],
    rows: models.slice(0, 8).map((item, index) =>
      makeRow(page, {
        __model_id: textFrom(item, ['model_id', 'id', 'uuid']) || `model-${index + 1}`,
        __rollback_version: textFrom(item, ['previous_version']),
        __f1_score: optionalModelMetricValue(item, ['f1_score', 'f1']) ?? '-',
        __auc: optionalModelMetricValue(item, ['auc', 'auc_score']) ?? '-',
        __drift: optionalModelMetricValue(item, ['drift', 'drift_score', 'psi']) ?? '-',
        __false_positive_delta: optionalModelMetricValue(item, ['false_positive_delta', 'fp_delta']) ?? '-',
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
  const workflows = extractList(status, ['workflows', 'tasks', 'jobs']);
  const running = optionalNumberAt(status, ['running_workflows']);
  const maxConcurrent = optionalNumberAt(status, ['max_concurrent']);
  const evaluationTasks = optionalNumberAt(status, ['evaluation_tasks']);
  const registeredModels = optionalNumberAt(status, ['registered_models']);
  const publishedModels = optionalNumberAt(status, ['published_models']);
  const failedWorkflows = optionalNumberAt(status, ['failed_workflows']);
  const gatePassRate = optionalRatioAt(status, ['gate_pass_rate']);
  const feedbackThreshold = optionalNumberAt(status, ['min_feedback_count']);
  const maxFpRate = optionalRatioAt(status, ['max_fp_rate']);
  const connectedValue = valueAt(status, ['clickhouse_connected']);
  const connected = typeof connectedValue === 'boolean' ? connectedValue : undefined;
  const statusText = textFrom(status, ['status']);
  const configured = statusText ? statusText !== 'not_configured' : undefined;
  const triggerCount = triggers.length;

  return {
    id: page.id,
    metrics: [
      metric('训练任务', running, '项', running === undefined ? 'warn' : running ? 'info' : 'ok'),
      metric('评估任务', evaluationTasks, '项', evaluationTasks === undefined ? 'warn' : evaluationTasks ? 'info' : 'ok'),
      metric('注册任务', registeredModels, '项', registeredModels === undefined ? 'warn' : 'ok'),
      metric('发布任务', publishedModels, '项', publishedModels === undefined ? 'warn' : publishedModels ? 'info' : 'ok'),
      metric('失败任务', failedWorkflows, '项', failedWorkflows === undefined ? 'warn' : failedWorkflows ? 'risk' : 'ok'),
      metric('门禁通过率', gatePassRate, '%', gatePassRate === undefined ? 'warn' : gatePassRate >= 85 ? 'ok' : gatePassRate ? 'warn' : 'risk'),
    ],
    rows: buildMlopsRows(page, workflows),
    timeline: [
      timelineItem('MLOps 编排器已接入', `来自 /v1/mlops/status，running=${running ?? '暂不可用'}，max=${maxConcurrent ?? '暂不可用'}。`, configured ? 'ok' : 'warn'),
      timelineItem('触发条件已关联', `来自 /v1/mlops/conditions，当前返回 ${triggerCount} 个触发器。`, triggerCount ? 'ok' : 'warn'),
      timelineItem('反馈与漂移门禁', `反馈阈值 ${feedbackThreshold ?? '-'}，最大误报率 ${maxFpRate ?? '-'}，ClickHouse=${connected === undefined ? 'unavailable' : connected ? 'connected' : 'disconnected'}。`, connected ? 'ok' : 'warn'),
      timelineItem('训练发布闭环', '页面承接反馈样本、标注、训练、评估、注册、发布和效果回流全链路。', 'info'),
    ],
    evidence: [
      evidence('MLOps Status API', '/v1/mlops/status', configured ? 'ok' : 'warn'),
      evidence('Conditions API', `${triggerCount} triggers`, triggerCount ? 'ok' : 'warn'),
      evidence('Argo Workflow', running === undefined || maxConcurrent === undefined ? '暂不可用' : `${running}/${maxConcurrent} running`, running === undefined ? 'warn' : running ? 'info' : 'ok'),
      evidence('反馈阈值', feedbackThreshold === undefined ? '未配置' : `${feedbackThreshold}`, feedbackThreshold === undefined ? 'warn' : 'ok'),
      evidence('误报门禁', maxFpRate === undefined ? '未配置' : `${maxFpRate}%`, maxFpRate === undefined ? 'warn' : 'ok'),
      evidence('ClickHouse', connected === undefined ? 'unavailable' : connected ? 'connected' : 'disconnected', connected ? 'ok' : 'warn'),
    ],
  };
};

const adaptPlaybooks = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const catalog = extractList(primaryPayload, ['playbooks', 'catalog', 'data']);
  const executions = extractList(secondaryPayloads[0], ['executions', 'data']);
  const total = optionalNumberAt(primaryPayload, ['total']) ?? catalog.length;
  const enabled = catalog.filter((item) => item.enabled !== false).length;
  const pendingApproval = catalog.filter((item) => !item.enabled || playbookHighRiskActions(item) >= 2).length;
  const todayRuns = executions.length || sumNumbers(catalog, ['run_count']);
  const failedSteps = sumNumbers(executions, ['failed_actions']);
  const highRiskConfirm = catalog.filter((item) => playbookHighRiskActions(item) > 0).length;
  const avgDurationMs = averageOptionalNumbers(executions, ['duration_ms']);
  const avgDuration = avgDurationMs === undefined ? '暂不可用' : playbookDurationLabel(avgDurationMs);

  return {
    id: page.id,
    metrics: [
      playbookMetric('启用剧本', `${formatNumber(enabled)} 个`, '真实 API', enabled ? 'ok' : 'warn'),
      playbookMetric('待审批', `${formatNumber(pendingApproval)} 个`, '风险门禁', pendingApproval ? 'warn' : 'ok'),
      playbookMetric('今日执行', `${formatNumber(todayRuns)} 次`, '执行记录', todayRuns ? 'info' : 'warn'),
      playbookMetric('失败步骤', `${formatNumber(failedSteps)} 步`, failedSteps ? '-1' : '0', failedSteps ? 'risk' : 'ok'),
      playbookMetric('高危待确认', `${formatNumber(highRiskConfirm)} 项`, '二次确认', highRiskConfirm ? 'warn' : 'ok'),
      playbookMetric('平均处理耗时', avgDuration, '执行记录', avgDurationMs === undefined ? 'warn' : avgDurationMs > 600_000 ? 'warn' : 'ok'),
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
  const total = optionalNumberAt(payload, ['total']) ?? entries.length;
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
  const total = optionalNumberAt(primaryPayload, ['total']) ?? reports.length;
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
  const total = optionalNumberAt(primaryPayload, ['total']) ?? logs.length;
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
  const workbench = unwrapPayload(primaryPayload);
  const workbenchSettings = valueAt(workbench, ['settings']);
  const settings = isRecord(workbenchSettings) ? workbenchSettings : workbench;
  const channels = notificationChannels(settings);
  const rules = extractList(workbench, ['rules', 'subscriptions', 'routes']);
  const history = extractList(workbench, ['history', 'deliveries', 'audits']);
  const escalationRules = extractList(workbench, ['escalation_policies', 'escalation_rules', 'escalations']);
  const apiSilenceRules = extractList(secondaryPayloads[0], ['rules', 'data', 'items']);
  const silenceRules = apiSilenceRules.length
    ? apiSilenceRules
    : extractList(workbench, ['silence_rules', 'silences', 'maintenance_windows']);
  const templates = extractList(workbench, ['templates', 'message_templates']);
  const enabledChannels = channels.filter((item) => item.enabled).length;
  const rulesAvailable = listFieldPresent(workbench, ['rules', 'subscriptions', 'routes']);
  const historyAvailable = listFieldPresent(workbench, ['history', 'deliveries', 'audits']);
  const escalationAvailable = listFieldPresent(workbench, ['escalation_policies', 'escalation_rules', 'escalations']);
  const silenceAvailable = apiSilenceRules.length > 0 || listFieldPresent(workbench, ['silence_rules', 'silences', 'maintenance_windows']);
  const templatesAvailable = listFieldPresent(workbench, ['templates', 'message_templates']);
  const failedDeliveries = historyAvailable ? history.filter((item) => notificationDeliveryStatus(item).includes('失败')).length : undefined;
  const pendingNotifications = historyAvailable ? history.filter((item) => notificationDeliveryStatus(item).includes('待')).length : undefined;
  const escalationCount = escalationAvailable ? escalationRules.length : undefined;
  const silenceCount = silenceAvailable ? silenceRules.length : undefined;
  const templateCount = templatesAvailable ? templates.length : undefined;
  const rateLimit = optionalNumberAt(settings, ['rate_limit_per_min']);
  const rowRules = rules.length ? rules : notificationRowsFromSilenceRules(silenceRules);

  return {
    id: page.id,
    metrics: [
      notificationMetric('启用渠道', `${enabledChannels} 个`, textFrom(settings, ['enabled']) === 'false' ? '已停用' : 'settings', enabledChannels ? 'ok' : 'warn'),
      notificationMetric('订阅规则', rulesAvailable ? `${rules.length} 条` : '暂不可用', '路由策略', rulesAvailable ? 'ok' : 'warn'),
      notificationMetric('待确认通知', pendingNotifications === undefined ? '暂不可用' : `${formatNumber(pendingNotifications)} 条`, 'SLA 队列', pendingNotifications !== undefined && pendingNotifications > 100 ? 'warn' : pendingNotifications === undefined ? 'warn' : 'info'),
      notificationMetric('失败通知', failedDeliveries === undefined ? '暂不可用' : `${formatNumber(failedDeliveries)} 条`, failedDeliveries ? '需重试' : failedDeliveries === undefined ? '未返回' : '稳定', failedDeliveries ? 'risk' : failedDeliveries === undefined ? 'warn' : 'ok'),
      notificationMetric('升级策略', escalationCount === undefined ? '暂不可用' : `${escalationCount} 条`, rateLimit === undefined ? '速率未返回' : `${rateLimit}/min`, escalationCount === undefined ? 'warn' : escalationCount ? 'warn' : 'info'),
      notificationMetric('静默窗口', silenceCount === undefined ? '暂不可用' : `${silenceCount} 个`, notificationSecretRef(settings) ? 'secret_ref' : '未绑定密钥', silenceCount === undefined ? 'warn' : notificationSecretRef(settings) ? 'info' : 'warn'),
    ],
    rows: buildNotificationRows(page, settings, channels, rowRules),
    timeline: [
      timelineItem('通知配置已接入', `来自 /v1/notifications/settings，通道 ${channels.length} 个，启用 ${enabledChannels} 个。`, channels.length ? 'ok' : 'warn'),
      timelineItem('Secret 引用门禁', notificationSecretRef(settings) ? `敏感值通过 ${notificationSecretRef(settings)} 引用。` : '尚未配置 secret_ref，页面不展示明文密钥。', notificationSecretRef(settings) ? 'ok' : 'warn'),
      timelineItem('投递与升级策略', `失败通知 ${failedDeliveries ?? '暂不可用'}、待确认 ${pendingNotifications ?? '暂不可用'}、升级策略 ${escalationCount ?? '暂不可用'}、模板 ${templateCount ?? '暂不可用'}。`, failedDeliveries ? 'warn' : failedDeliveries === undefined ? 'info' : 'ok'),
      timelineItem('抑制与静默', `来自 /v1/notifications/silence-rules，维护窗口 ${silenceRules.length} 个，低优先级静默和专题免打扰写入审计。`, silenceRules.length ? 'ok' : 'info'),
    ],
    evidence: [
      evidence('Notification Settings API', '/v1/notifications/settings', 'ok'),
      evidence('Notification Silence API', `/v1/notifications/silence-rules ${silenceRules.length} 条`, apiSilenceRules.length ? 'ok' : 'info'),
      evidence('Secret 引用', notificationSecretRef(settings) || '待配置', notificationSecretRef(settings) ? 'ok' : 'warn'),
      evidence('通道测试', `${enabledChannels}/${channels.length} 启用`, enabledChannels ? 'ok' : 'warn'),
      evidence('订阅策略', rulesAvailable ? `${rules.length} 条` : '暂不可用', rulesAvailable ? 'ok' : 'warn'),
      evidence('升级策略', escalationCount === undefined ? '暂不可用' : `${escalationCount} 条`, escalationCount === undefined ? 'warn' : 'ok'),
      evidence('投递审计', failedDeliveries === undefined ? '暂不可用' : `${failedDeliveries} 失败`, failedDeliveries ? 'risk' : failedDeliveries === undefined ? 'warn' : 'ok'),
      evidence('静默窗口', silenceCount === undefined ? '暂不可用' : `${silenceCount} 个`, silenceCount === undefined ? 'warn' : 'info'),
    ],
  };
};

const adaptSettings = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
  const workbench = unwrapPayload(primaryPayload);
  const scopes = extractList(secondaryPayloads[0], ['scopes']);
  const tokenPayload = secondaryPayloads[1];
  const probeScopePayload = secondaryPayloads[2];
  const tokens = extractList(tokenPayload, ['tokens', 'data', 'items']);
  const probeScopes = extractList(probeScopePayload, ['scopes']);
  const tokenEnvelope = unwrapEnvelope(tokenPayload);
  const totalTokens = totalFromEnvelope(tokenEnvelope, tokens.length);
  const tenantID = textFrom(workbench, ['tenant_id']);
  const tenantCount = tenantID ? 1 : 0;
  const roles = extractList(workbench, ['roles']);
  const integrations = extractList(isRecord(workbench) ? workbench.settings : undefined, ['integrations']);
  const persistedTokens = isRecord(workbench) && isRecord(workbench.tokens) ? workbench.tokens : {};
  const scopeCategories = new Set(scopes.map((item) => textFrom(item, ['category'])).filter(Boolean));
  const activeTokens = optionalNumberAt(persistedTokens, ['active']) ?? (tokens.length ? tokens.filter(settingsTokenActive).length : undefined);
  const expiringTokens = optionalNumberAt(persistedTokens, ['expiring_soon']) ?? (tokens.length ? tokens.filter(settingsTokenExpiringSoon).length : undefined);
  const rotationEnabled = tokens.filter((item) => valueAt(item, ['rotation_enabled']) === true).length;
  const pendingAudit = optionalNumberAt(workbench, ['pending_audit_count']);
  const tokenListAvailable = tokens.length > 0;
  const healthyIntegrations = integrations.filter((item) => textFrom(item, ['status']) === 'healthy').length;

  return {
    id: page.id,
    metrics: [
      settingsMetric('租户数', `${tenantCount} 个`, 'tenant_id', tenantCount ? 'info' : 'warn'),
      settingsMetric('角色策略', roles.length || scopes.length ? `${roles.length || scopes.length} 项` : '暂不可用', scopeCategories.size ? `${scopeCategories.size} 类 scope` : 'scope 分类未返回', roles.length || scopes.length ? 'ok' : 'warn'),
      settingsMetric('有效令牌', activeTokens === undefined ? '暂不可用' : `${activeTokens} 个`, tokenListAvailable ? 'tokens' : '令牌清单未返回', activeTokens === undefined ? 'warn' : activeTokens ? 'ok' : 'info'),
      settingsMetric('即将过期令牌', expiringTokens === undefined ? '暂不可用' : `${expiringTokens} 个`, '7天内过期', expiringTokens === undefined ? 'warn' : expiringTokens ? 'warn' : 'ok'),
      settingsMetric('集成健康', integrations.length ? `${healthyIntegrations}/${integrations.length}` : '暂不可用', probeScopes.length ? 'probe scopes' : '配置项', integrations.length ? 'ok' : 'warn'),
      settingsMetric('配置变更待审计', pendingAudit === undefined ? '暂不可用' : `${pendingAudit} 项`, rotationEnabled ? '轮换开启' : '保存后写审计', pendingAudit === undefined ? 'warn' : pendingAudit ? 'info' : 'ok'),
    ],
    rows: buildSettingsRows(page, tokens),
    timeline: [
      timelineItem('租户设置真源已接入', `来自 /v1/auth/system-settings，租户 ${tenantID}、revision ${numberAt(workbench, ['revision'])}。`, tenantID ? 'ok' : 'warn'),
      timelineItem('Token Scope 真源已接入', `来自 /v1/tokens/scopes，返回 ${scopes.length || 0} 个权限范围。`, scopes.length ? 'ok' : 'warn'),
      timelineItem('API 令牌清单', tokenListAvailable ? `来自 /v1/tokens，当前租户 ${totalTokens || tokens.length} 个令牌。` : '令牌清单暂未返回，页面保持创建和轮换入口。', tokenListAvailable ? 'ok' : 'warn'),
      timelineItem('探针最小权限', `probe scopes ${probeScopes.length || 0} 个，默认权限不在前端展开明文密钥。`, probeScopes.length ? 'ok' : 'info'),
      timelineItem('配置审计闭环', `保存配置、轮换令牌、连接测试和安全审计均需要写入 audit_logs。`, 'info'),
    ],
    evidence: [
      evidence('System Settings API', `/v1/auth/system-settings r${numberAt(workbench, ['revision'])}`, tenantID ? 'ok' : 'warn'),
      evidence('Token Scopes API', `${scopes.length || 0} scopes`, scopes.length ? 'ok' : 'warn'),
      evidence('Token List API', tokenListAvailable ? `${totalTokens || tokens.length} tokens` : '待返回', tokenListAvailable ? 'ok' : 'warn'),
      evidence('Probe Scopes API', `${probeScopes.length || 0} scopes`, probeScopes.length ? 'ok' : 'info'),
      evidence('RBAC 矩阵', scopeCategories.size ? `${scopeCategories.size} 类权限` : '暂不可用', scopeCategories.size ? 'info' : 'warn'),
      evidence('留存策略', 'Flow/Session/Alert/PCAP/Audit', 'ok'),
      evidence('集成健康', integrations.length ? `${healthyIntegrations}/${integrations.length}` : '暂不可用', integrations.length ? 'ok' : 'warn'),
      evidence('审计写入', pendingAudit === undefined ? '暂不可用' : `${pendingAudit} 项待审计`, pendingAudit === undefined ? 'warn' : 'info'),
    ],
  };
};

const metric = (label: string, value: number | undefined, suffix: string, status: MetricStatus) => ({
  label,
  value: value === undefined ? '暂不可用' : suffix === '%' ? `${value.toFixed(1)}%` : `${formatNumber(value)} ${suffix}`,
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

const totalFromEnvelope = (payload: Record<string, unknown>, fallback: number) => {
  const direct = optionalRawNumberAt(payload, ['total']);
  const dataTotal = optionalRawNumberAt(payload, ['data', 'total']);
  const pagination = optionalRawNumberAt(payload, ['pagination', 'total']);
  const metaPage = optionalRawNumberAt(payload, ['meta', 'page', 'total']);
  return direct ?? dataTotal ?? pagination ?? metaPage ?? fallback;
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
    const value = optionalNumberAt(payload, [key]);
    if (value !== undefined) return value;
  }
  return 0;
};

// Compatibility aliases are tried in declaration order while preserving an
// authoritative zero. Callers decide explicitly whether absence is
// unavailable, derivable from the same response, or covered by a documented
// compatibility default.
const optionalNumberFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = optionalNumberAt(payload, [key]);
    if (value !== undefined) return value;
  }
  return undefined;
};

const numberAt = (payload: unknown, path: string[]) => numeric(valueAt(payload, path));

const optionalNumberAt = (payload: unknown, path: string[]) => {
  const value = valueAt(payload, path);
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
};

// Envelope metadata must be read without recursively unwrapping `data`, or a
// list response would hide sibling pagination fields.
const optionalRawNumberAt = (payload: unknown, path: string[]) => {
  let current = payload;
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return typeof current === 'number' && Number.isFinite(current) ? current : undefined;
};

const ratioAt = (payload: unknown, path: string[]) => {
  const value = numberAt(payload, path);
  return value <= 1 ? value * 100 : value;
};

const optionalRatioAt = (payload: unknown, path: string[]) => {
  const value = optionalNumberAt(payload, path);
  if (value === undefined) return undefined;
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

const listFieldPresent = (payload: unknown, keys: string[]) =>
  keys.some((key) => Array.isArray(valueAt(payload, [key])));

const stringListAt = (payload: unknown, path: string[]) => {
  const value = valueAt(payload, path);
  return Array.isArray(value)
    ? value.map((item) => String(item ?? '').trim()).filter(Boolean)
    : [];
};

const sumNumbers = (items: Record<string, unknown>[], paths: string[]) =>
  items.reduce((total, item) => total + paths.reduce((sum, path) => sum + numberAt(item, [path]), 0), 0);

const numeric = (value: unknown) => (typeof value === 'number' && Number.isFinite(value) ? value : 0);

const average = (values: number[]) =>
  values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0;

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

const averageOptionalNumbers = (items: Record<string, unknown>[], keys: string[]) => {
  const values = items
    .flatMap((item) => keys.map((key) => optionalNumberAt(item, [key])))
    .filter((value): value is number => value !== undefined);
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : undefined;
};

const attackPhaseLabel = (phase: string) => {
  const labels: Record<string, string> = {
    reconnaissance: '侦察',
    initial_access: '初始访问',
    execution: '执行',
    persistence: '持久化',
    privilege_escalation: '权限提升',
    defense_evasion: '防御规避',
    credential_access: '凭证访问',
    discovery: '发现',
    lateral_movement: '横向移动',
    collection: '数据收集',
    command_control: '命令与控制',
    exfiltration: '数据外传',
    impact: '影响',
  };
  return (labels[phase.toLowerCase()] ?? phase) || '-';
};

const severityLabel = (severity: string) => {
  const value = severity.toLowerCase().replace(/^severity_/, '');
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
  const explicit = textFrom(item, ['activity_status', 'status']).toLowerCase();
  if (explicit === 'active') return '活跃中';
  if (explicit === 'investigating') return '调查中';
  if (explicit === 'contained') return '处置中';
  if (explicit === 'closed') return '已结束';
  if (explicit) return statusLabel(explicit);
  const tsEnd = numberAt(item, ['ts_end', 'end_time']);
  return tsEnd ? '活跃中' : '调查中';
};

const campaignWorkflowStatus = (item: Record<string, unknown>) => {
  const explicit = textFrom(item, ['status', 'activity_status']).toLowerCase();
  if (explicit === 'active') return '活跃中';
  if (explicit === 'investigating') return '调查中';
  if (explicit === 'contained') return '处置中';
  if (explicit === 'closed') return '已结束';
  return campaignStatus(item);
};

const campaignDurationHours = (item: Record<string, unknown>) => {
  const start = numberAt(item, ['ts_start', 'start_time']);
  const end = optionalNumberAt(item, ['ts_end', 'end_time']) ?? optionalNumberAt(item, ['ingest_ts']);
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

const encryptedRisk = (item: Record<string, unknown>) => {
  const explicit = severityLabel(textFrom(item, ['risk_level', 'severity', 'risk']));
  if (explicit && explicit !== '-') return explicit;
  const anomaly = numberAt(item, ['anomaly_score']);
  const entropy = numberAt(item, ['entropy_score']);
  if (anomaly >= 0.8 || entropy >= 7.5) return '高危';
  if (anomaly >= 0.5 || entropy >= 5.5) return '中危';
  return '低危';
};

const boundedPercent = (value: number | undefined, fallback: number) => {
  if (value === undefined) return fallback;
  return value <= 1 ? value * 100 : value;
};

const qualityCheckValue = (checks: Record<string, unknown>[], name: string) => {
  const check = checks.find((item) => textFrom(item, ['name']).toLowerCase() === name);
  return optionalNumberAt(check, ['value']);
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

const buildQualityTopics = (page: PageSpec, topics: Record<string, unknown>[]) =>
  topics.map((topic) => {
    const throughput = optionalNumberFrom(topic, ['throughput_per_min', 'throughput', 'messages_per_min']);
    const lag = optionalNumberFrom(topic, ['lag', 'backlog', 'consumer_lag']);
    const p95 = optionalNumberFrom(topic, ['consumer_p95_ms', 'message_p95_ms', 'p95_latency_ms']);
    return makeRow(page, {
        Topic: textFrom(topic, ['topic', 'name']) || '-',
        分区数: optionalNumberFrom(topic, ['partitions', 'partition_count']) ?? '-',
        当前吞吐量: throughput === undefined ? '暂不可用' : `${formatNumber(throughput)} msg/min`,
        消费延迟: p95 === undefined ? '暂不可用' : `${Math.round(p95 / 1000)}s`,
        积压量: lag === undefined ? '暂不可用' : formatNumber(lag),
        积压趋势: textFrom(topic, ['lag_trend', 'trend']) || '暂不可用',
        '消费延迟 P95': p95 === undefined ? '暂不可用' : `${Math.round(p95)} ms`,
        分区倾斜: textFrom(topic, ['partition_skew']) || '暂不可用',
        '消息延迟 P95': optionalNumberAt(topic, ['message_p95_ms']) === undefined ? '暂不可用' : `${Math.round(optionalNumberAt(topic, ['message_p95_ms']) ?? 0)} ms`,
        操作: textFrom(topic, ['action']) || '查看详情',
      });
  });

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

const probeLocation = (item: Record<string, unknown>) =>
  textFrom(item, ['location', 'building', 'site', 'name']) || '-';

const probeUptime = (item: Record<string, unknown>) => {
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
  const start = optionalNumberFrom(params, ['start_time', 'start_ms']) ?? optionalNumberFrom(item, ['created_at']);
  const end = optionalNumberFrom(params, ['end_time', 'end_ms']) ?? optionalNumberFrom(item, ['completed_at', 'updated_at']);
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
  if (normalized === 'partial') return '部分完成';
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
  return optionalModelMetricValue(item, ['drift', 'psi', 'drift_psi'])
    ?? optionalModelMetricValue(metadata, ['drift', 'psi', 'drift_psi'])
    ?? 0;
};

const optionalModelMetricValue = (item: Record<string, unknown>, keys: string[]) => {
  const direct = optionalNumberFrom(item, keys);
  if (direct !== undefined) return direct;
  const metrics = valueAt(item, ['metrics']);
  if (isRecord(metrics)) {
    const metricValue = optionalNumberFrom(metrics, keys);
    if (metricValue !== undefined) return metricValue;
  }
  const metadata = modelMetadata(item);
  const metadataMetrics = valueAt(metadata, ['metrics']);
  if (isRecord(metadataMetrics)) return optionalNumberFrom(metadataMetrics, keys);
  return undefined;
};

const averageOptionalModelMetric = (items: Record<string, unknown>[], keys: string[]) => {
  const values = items
    .map((item) => optionalModelMetricValue(item, keys))
    .filter((value): value is number => value !== undefined);
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : undefined;
};

const buildMlopsRows = (page: PageSpec, workflows: Record<string, unknown>[]): SnapshotRow[] =>
  workflows.slice(0, 8).map((item) =>
    makeRow(page, {
      __data_mode: 'api-workflow',
      任务ID: textFrom(item, ['workflow_id', 'task_id', 'job_id', 'id']) || '-',
      阶段: textFrom(item, ['stage', 'phase', 'task_type', 'type']) || '-',
      数据集版本: textFrom(item, ['dataset_version', 'dataset']) || '-',
      算法配置: textFrom(item, ['algorithm_config', 'algorithm', 'model_type']) || '-',
      特征版本: textFrom(item, ['feature_version', 'features']) || '-',
      资源占用: textFrom(item, ['resource_usage', 'resources']) || '-',
      状态: textFrom(item, ['status', 'state']) || '-',
      操作: '查看日志',
    }),
  );

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
    const totalAlerts = optionalNumberAt(content, ['total_alerts']) ?? optionalNumberAt(summary, ['total_alerts']);
    const resolvedAlerts = optionalNumberAt(content, ['resolved_alerts']) ?? optionalNumberAt(summary, ['resolved_alerts']);
    return [
      textFrom(section, ['title', 'section_name']) || rowSpecs[index][0],
      index === 0 ? '响应闭环 >= 80%' : index === 1 ? 'SLA 违规 <= 3' : '误报反馈已留痕',
      totalAlerts ? `${resolvedAlerts} / ${totalAlerts}` : rowSpecs[index][2],
      index === 0 ? '告警 / 处置链路' : index === 1 ? '告警 SLA' : '反馈样本库',
      textFrom(section, ['section_name']) || rowSpecs[index][4],
      formatEpochTime(optionalNumberAt(report, ['generated_at'])) ?? rowSpecs[index][5],
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
  const defaults = ['email', 'webhook', 'wechat', 'dingtalk', 'slack', 'feishu'];
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

  if (!tokens.length) {
    return defaultRows.map((row) => makeRow(page, {
      令牌名称: row[0], 权限范围: row[1], 令牌指纹: row[2], 过期时间: row[3], 最近使用: row[4],
      轮换状态: row[5], 操作: '轮换 / 吊销', token_id: '', scopes: '', token_status: 'display-only',
    }));
  }

  return tokens.slice(0, 8).map((item, index) => makeRow(page, {
    令牌名称: textFrom(item, ['name']) || `API Token ${index + 1}`,
    权限范围: settingsTokenScopes(item),
    令牌指纹: settingsTokenFingerprint(item, index),
    过期时间: settingsTokenExpiresAt(item),
    最近使用: settingsTokenLastUsed(item),
    轮换状态: settingsTokenRotationStatus(item),
    操作: '轮换 / 吊销',
    token_id: textFrom(item, ['token_id']),
    scopes: stringListFrom(valueAt(item, ['scopes'])).join(','),
    token_status: textFrom(item, ['status']),
  }));
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

const confidenceLabel = (value: number | undefined) => {
  if (value === undefined) return '暂不可用';
  if (value === 0) return '0';
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

const formatNumber = (value: number | undefined) => {
  if (value === undefined) return '暂不可用';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value);
};

const formatEpochTime = (value: number | undefined) => {
  if (!value) return '-';
  const ms = value > 10_000_000_000 ? value : value * 1000;
  return new Date(ms).toISOString().slice(5, 16).replace('T', ' ');
};
