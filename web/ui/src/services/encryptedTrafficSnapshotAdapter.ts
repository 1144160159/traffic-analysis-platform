import type { PageSpec } from '@/routes/routeManifest';
import type { EncryptedTrafficVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';
import {
  extractNamedPageSnapshotList as extractNamedList,
  extractPageSnapshotList as extractList,
  isRecord,
  unwrapPageSnapshotPayload as unwrapPayload,
} from '@/services/pageSnapshotEnvelope';

type MetricStatus = PageSnapshot['metrics'][number]['status'];

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
  ...values,
});

const valueAt = (payload: unknown, path: string[]) => {
  let current = unwrapPayload(payload);
  for (const key of path) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return current;
};

const textFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = valueAt(payload, [key]);
    if (typeof value === 'string' || typeof value === 'number') {
      const text = String(value);
      if (text) return text;
    }
  }
  return '';
};

const optionalNumberAt = (payload: unknown, path: string[]) => {
  const value = valueAt(payload, path);
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
};

const numberAt = (payload: unknown, path: string[]) => optionalNumberAt(payload, path) ?? 0;

const optionalNumberFrom = (payload: unknown, keys: string[]) => {
  for (const key of keys) {
    const value = optionalNumberAt(payload, [key]);
    if (value !== undefined) return value;
  }
  return undefined;
};

const numberFrom = (payload: unknown, keys: string[]) => optionalNumberFrom(payload, keys) ?? 0;

const optionalRatioAt = (payload: unknown, path: string[]) => {
  const value = optionalNumberAt(payload, path);
  if (value === undefined) return undefined;
  return value <= 1 ? value * 100 : value;
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

const bytesLabel = (value: number) => {
  if (!value) return '0 B';
  if (value >= 1024 ** 4) return `${(value / 1024 ** 4).toFixed(2)} TB`;
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GB`;
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${Math.round(value)} B`;
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

const encryptedProtocol = (item: Record<string, unknown>) => {
  const protocol = textFrom(item, ['protocol']).toUpperCase();
  if (protocol.includes('QUIC')) return 'QUIC';
  if (protocol.includes('TLS')) return 'TLS';
  if (protocol.includes('HTTPS')) return 'TLS';
  return protocol || '未知加密';
};

const encryptedSessionSummary = (item: Record<string, unknown>) => {
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

const encryptedAlpnFallback = (item: Record<string, unknown>) =>
  textFrom(item, ['application_protocol', 'next_protocol']) || '-';

const encryptedRisk = (item: Record<string, unknown>) => {
  const explicit = severityLabel(textFrom(item, ['risk_level', 'severity', 'risk']));
  if (explicit && explicit !== '-') return explicit;
  const anomaly = numberAt(item, ['anomaly_score']);
  const entropy = numberAt(item, ['entropy_score']);
  if (anomaly >= 0.8 || entropy >= 7.5) return '高危';
  if (anomaly >= 0.5 || entropy >= 5.5) return '中危';
  return '低危';
};

export const adaptEncryptedTrafficSnapshot = (page: PageSpec, primaryPayload: unknown, secondaryPayloads: unknown[]): PageSnapshot => {
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
  const totalSessions = optionalNumberAt(stats, ['total_sessions']) ?? sessionRows.length;
  const tlsSessions = optionalNumberAt(stats, ['tls_sessions']) ?? sessionRows.filter((item) => encryptedProtocol(item).includes('TLS')).length;
  const quicSessions = optionalNumberAt(stats, ['quic_sessions']) ?? sessionRows.filter((item) => encryptedProtocol(item).includes('QUIC')).length;
  const tlsRatio = optionalNumberAt(stats, ['tls_ratio']) ?? (totalSessions ? (tlsSessions / totalSessions) * 100 : optionalRatioAt(stats, ['encrypted_ratio']));
  const quicRatio = optionalNumberAt(stats, ['quic_ratio']) ?? (totalSessions ? (quicSessions / totalSessions) * 100 : 0);
  const unknownRatio = optionalNumberAt(stats, ['unknown_encrypted_ratio'])
    ?? (tlsRatio === undefined ? undefined : Math.max(0, 100 - tlsRatio - quicRatio));
  const expiredOrMissingCerts = optionalNumberAt(stats, ['abnormal_certificate_count']) ?? sessionRows.filter((item) => certificateRisk(item)).length;
  const maliciousJA3 = optionalNumberAt(stats, ['malicious_ja3_matches']) ?? fingerprintRows.filter((item) => encryptedRisk(item).includes('高')).length;
  const missingSni = sessionRows.filter((item) => !textFrom(item, ['sni', 'SNI'])).length;
  const unknownSniRatio = optionalNumberAt(stats, ['unknown_sni_ratio']) ?? (sessionRows.length ? (missingSni / sessionRows.length) * 100 : 0);
  const externalDestinations = exfilDestinations.length || exfilPaths.length || sessions.filter((item) => textFrom(item, ['dst_ip', 'destination_ip'])).length;
  const trafficGbps = optionalNumberFrom(stats, ['traffic_gbps', 'total_gbps', 'throughput_gbps']);
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
      metric('加密流量总量', trafficGbps ?? totalSessions, trafficGbps === undefined ? '会话' : 'Gbps', totalSessions ? 'info' : 'warn'),
      metric('TLS 流量占比', tlsRatio, '%', tlsRatio === undefined ? 'info' : tlsRatio >= 50 ? 'ok' : 'warn'),
      metric('QUIC 流量占比', quicRatio, '%', quicRatio >= 20 ? 'info' : 'ok'),
      metric('未知加密占比', unknownRatio, '%', unknownRatio === undefined ? 'info' : unknownRatio >= 20 ? 'info' : 'ok'),
      metric('异常证书数', expiredOrMissingCerts, '张', expiredOrMissingCerts ? 'warn' : 'ok'),
      metric('可疑 JA3 数', maliciousJA3, '个', maliciousJA3 ? 'risk' : 'ok'),
      metric('未知 SNI 比例', unknownSniRatio, '%', unknownSniRatio >= 10 ? 'warn' : 'ok'),
    ],
    rows: sessionRows.slice(0, 8).map((item) =>
      makeRow(page, {
        时间: formatEpochTime(numberFrom(item, ['start_time', 'StartTime'])),
        协议: encryptedProtocol(item),
        'Session 摘要': encryptedSessionSummary(item),
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
  tlsRatio: number | undefined;
  quicRatio: number;
  unknownRatio: number | undefined;
  maliciousJA3: number;
}): EncryptedTrafficVisuals => {
  const totalGbps = numberAt(stats, ['traffic_gbps', 'total_gbps', 'throughput_gbps']);
  const tlsGbps = tlsRatio === undefined ? undefined : totalGbps * tlsRatio / 100;
  const quicGbps = totalGbps * quicRatio / 100;
  const unknownGbps = tlsGbps === undefined ? undefined : Math.max(0, totalGbps - tlsGbps - quicGbps);
  const protocolRows = [
    ['TLS', tlsGbps === undefined ? '暂不可用' : `${tlsGbps.toFixed(1)} Gbps`, tlsRatio === undefined ? '暂不可用' : `${tlsRatio.toFixed(1)}%`, 'is-info'],
    ['QUIC', `${quicGbps.toFixed(1)} Gbps`, `${quicRatio.toFixed(1)}%`, 'is-warn'],
    ['其他加密', unknownGbps === undefined ? '暂不可用' : `${unknownGbps.toFixed(1)} Gbps`, unknownRatio === undefined ? '暂不可用' : `${unknownRatio.toFixed(1)}%`, 'is-info'],
  ];
  const protocolTrend: number[] = [];
  const ja3Source = (fingerprints.length
    ? fingerprints
    : sessions.filter((item) => textFrom(item, ['ja3_fingerprint', 'ja3', 'JA3Fingerprint']))).slice(0, 6);
  const ja3Rows = ja3Source.slice(0, 6).map((item) => {
    const risk = encryptedRisk(item);
    const flow = numberFrom(item, ['traffic_gbps', 'flow_gbps', 'gbps']);
    const ratio = optionalRatioAt(item, ['traffic_ratio']) ?? optionalRatioAt(item, ['ratio']);
    return [
      textFrom(item, ['ja3_fingerprint', 'ja3', 'fingerprint', 'JA3Fingerprint']) || '-',
      ratio === undefined ? '暂不可用' : `${ratio.toFixed(1)}%`,
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
        ['指纹总数', formatNumber(optionalNumberAt(stats, ['ja3_sample_count']) ?? ja3Rows.length), '真实 JA3 API'],
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
  const sessions = sourceSessions.slice(0, 9).map((item) => ({
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
    entropy: optionalNumberFrom(item, ['entropy_score', 'entropy']) ?? 0,
  }));
  const pcapRows = sourcePcapIndexes.slice(0, 6).map((item) => {
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
  const hashRows = sourcePcapIndexes.slice(0, 5).map((item) => [
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

export const formatEvidenceDateTime = (value: number) => {
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

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));
