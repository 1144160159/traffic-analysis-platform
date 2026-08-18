import type {
  EncryptedTrafficAvailability,
  EncryptedTrafficSnapshotFact,
  EncryptedTrafficSnapshotSection,
} from '@/services/encryptedTrafficApi';

export function encryptedSnapshotAvailabilityCopy(availability: EncryptedTrafficAvailability) {
  const copies: Record<EncryptedTrafficAvailability, { label: string; description: string; tone: 'ok' | 'info' | 'warn' | 'risk' }> = {
    available: { label: '可用', description: '来源返回了可解释事实', tone: 'ok' },
    zero: { label: '实测为零', description: '方法已执行且测量值为零', tone: 'info' },
    no_sample: { label: '无样本', description: '当前时间窗没有可计算样本', tone: 'info' },
    not_computable: { label: '不可计算', description: '存在事实但缺少方法所需输入', tone: 'warn' },
    unavailable: { label: '来源不可用', description: '权威来源读取失败', tone: 'risk' },
    forbidden: { label: '字段受限', description: '当前身份缺少字段权限', tone: 'warn' },
  };
  return copies[availability];
}

export function encryptedSnapshotDrilldownPath(fact: EncryptedTrafficSnapshotFact): string | undefined {
  const firstString = (value: unknown) => Array.isArray(value)
    ? value.find((item): item is string => typeof item === 'string' && item.length > 0)
    : typeof value === 'string' && value.length > 0 ? value : undefined;
  const pcapIndex = firstString(fact.pcap_index_ids);
  if (pcapIndex) return `/forensics?pcap_index=${encodeURIComponent(pcapIndex)}`;
  const evidence = firstString(fact.evidence_refs);
  if (evidence) return `/forensics?query=${encodeURIComponent(evidence)}`;
  const session = firstString(fact.session_id);
  if (session) return `/forensics?session=${encodeURIComponent(session)}`;
  const event = firstString(fact.source_event_ids) ?? firstString(fact.event_ids);
  if (event) return `/alerts?event_id=${encodeURIComponent(event)}`;
  return undefined;
}

export function encryptedSnapshotSectionPresentation(section: EncryptedTrafficSnapshotSection) {
  const fact = section.facts.find((candidate) => encryptedSnapshotDrilldownPath(candidate));
  const drilldown = fact ? encryptedSnapshotDrilldownPath(fact) : undefined;
  return {
    availability: encryptedSnapshotAvailabilityCopy(section.availability),
    ruleVersions: section.rule_versions.join(', ') || '无规则判定',
    modelVersions: section.model_versions.join(', ') || '无模型判定',
    limitations: encryptedSnapshotLimitations(section),
    drilldown,
    drilldownLabel: drilldown ? '下钻首条事实' : section.availability === 'forbidden' ? '字段权限不足' : '暂无可下钻事实',
  };
}

function encryptedSnapshotLimitations(section: EncryptedTrafficSnapshotSection) {
  if (section.missing_reasons.length) return `限制：${section.missing_reasons.join('；')}`;
  if (section.partial) return '限制：来源声明为部分结果，但未提供具体缺失原因。';
  return '限制：无新增缺失声明；仍需结合证据与规则上下文研判。';
}
