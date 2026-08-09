import { appConfig } from '@/config/runtime';
import { api } from '@/services/api';

export type CampaignDetailMetric = {
  label: string;
  value: string;
  delta: string;
  status: 'ok' | 'warn' | 'risk' | 'info';
};

export type CampaignDetailPhase = {
  phase: string;
  time: string;
  alertCount: number;
  evidenceCount: number;
  status: CampaignDetailMetric['status'];
  summary: string;
};

export type CampaignDetailAlertRow = {
  告警时间: string;
  告警ID: string;
  告警名称: string;
  攻击阶段: string;
  影响资产: string;
  风险: string;
  状态: string;
  操作: string;
};

export type CampaignDetailImpactTab = {
  label: string;
  value: string;
  status: CampaignDetailMetric['status'];
};

export type CampaignDetailAssetRow = {
  资产: string;
  类型: string;
  部门: string;
  业务系统: string;
  风险: string;
  证据: string;
};

export type CampaignDetailImpactRiskRow = {
  label: string;
  count: number;
  percent: string;
  status: CampaignDetailMetric['status'];
};

export type CampaignDetailImpactAsset = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailAssetRow[];
};

export type CampaignDetailAccountRow = {
  账号: string;
  账号类型: string;
  权限风险: string;
  登录链路: string;
};

export type CampaignDetailImpactAccount = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailAccountRow[];
};

export type CampaignDetailBusinessSystemRow = {
  业务系统: string;
  关键服务: string;
  风险: string;
  恢复优先级: string;
};

export type CampaignDetailImpactBusinessSystem = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailBusinessSystemRow[];
};

export type CampaignDetailServiceRow = {
  服务名称: string;
  端口协议: string;
  风险: string;
  依赖关系: string;
};

export type CampaignDetailImpactService = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailServiceRow[];
};

export type CampaignDetailDepartmentRow = {
  部门名称: string;
  责任人: string;
  风险: string;
  处置进度: number;
};

export type CampaignDetailImpactDepartment = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailDepartmentRow[];
};

export type CampaignDetailCampusRow = {
  校区楼宇: string;
  覆盖资产: number;
  风险: string;
  链路: string;
};

export type CampaignDetailImpactCampus = {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  rows: CampaignDetailCampusRow[];
};

export type CampaignDetailEvidenceCheck = {
  label: string;
  value: string;
  percent: number;
  status: CampaignDetailMetric['status'];
};

export type CampaignDetailEvidenceSummaryRow = {
  证据类型: string;
  文件记录: string;
  完整度: string;
  状态: string;
};

export type CampaignDetailEvidenceDigestItem = {
  label: string;
  value: string;
};

export type CampaignDetailFlowStep = {
  title: string;
  time: string;
  status: CampaignDetailMetric['status'];
};

export type CampaignDetailEvidenceRailItem = {
  key: string;
  label: string;
  current: number | null;
  expected: number | null;
  available: boolean;
};

export type CampaignDetailStatusTransition = {
  status: string;
  changedAt: string;
  source: string;
};

export type CampaignDetailActionRow = {
  动作: string;
  目标: string;
  负责人: string;
  状态: string;
};

export type CampaignDetailReviewRow = {
  维度: string;
  结论: string;
  状态: string;
};

export type CampaignDetailSnapshot = {
  campaignId: string;
  stateVersion: number;
  snapshotId: string;
  snapshotSHA256: string;
  partial: boolean;
  missingSections: string[];
  sourceWatermarks: Record<string, string>;
  reports: CampaignDetailReport[];
  campaignType: string;
  title: string;
  riskScore: number;
  currentPhase: string;
  duration: string;
  firstSeen: string;
  lastUpdated: string;
  status: string;
  activityStatus: string;
  workflowStatus: string;
  assignee: string;
  alertCount: number;
  assetCount: number;
  tags: string[];
  summary: string;
  profileFacts: Array<{ label: string; value: string; status?: boolean }>;
  metrics: CampaignDetailMetric[];
  phases: CampaignDetailPhase[];
  alerts: CampaignDetailAlertRow[];
  impactTabs: CampaignDetailImpactTab[];
  topAssets: CampaignDetailAssetRow[];
  impactAsset: CampaignDetailImpactAsset;
  impactAccount: CampaignDetailImpactAccount;
  impactBusinessSystem: CampaignDetailImpactBusinessSystem;
  impactService: CampaignDetailImpactService;
  impactDepartment: CampaignDetailImpactDepartment;
  impactCampus: CampaignDetailImpactCampus;
  impactDataBacked: Record<'asset' | 'account' | 'service' | 'department' | 'campus' | 'business-system', boolean>;
  evidenceCompleteness: number;
  evidenceCompletenessAvailable: boolean;
  phaseDataBacked: boolean;
  evidenceRail: CampaignDetailEvidenceRailItem[];
  statusTransitions: CampaignDetailStatusTransition[];
  evidenceChecks: CampaignDetailEvidenceCheck[];
  evidenceSummaryRows: CampaignDetailEvidenceSummaryRow[];
  evidenceDigest: CampaignDetailEvidenceDigestItem[];
  responseFlow: CampaignDetailFlowStep[];
  responseActions: CampaignDetailActionRow[];
  reviewRows: CampaignDetailReviewRow[];
  evidence: CampaignDetailMetric[];
};

export type CampaignDetailReport = {
  reportId: string;
  jobId: string;
  format: string;
  status: string;
  campaignRevision: number;
  sourceSnapshotId: string;
  snapshotId: string;
  snapshotSHA256: string;
  artifactSHA256: string;
  sizeBytes: number;
  attempts: number;
  errorMessage: string;
};

const canonicalPhases = ['初始访问', '执行', '持久化', '横向移动', 'C2通信', '数据外传', '处置闭环'];

function requiredCampaignId(campaignId: string): string {
  const normalized = campaignId.trim();
  if (!normalized) throw new Error('campaign_id 不能为空');
  return normalized;
}

export async function fetchCampaignDetailSnapshot(campaignId: string, snapshotId?: string): Promise<CampaignDetailSnapshot> {
  const normalizedId = requiredCampaignId(campaignId);
  if (appConfig.useMock) return buildMockCampaignDetailSnapshot(normalizedId);

  const response = await api.get(`/v1/campaigns/${encodeURIComponent(normalizedId)}`, {
    params: snapshotId ? { snapshot_id: snapshotId } : undefined,
  });
  return normalizeCampaignDetailSnapshot(normalizedId, response.data);
}

export function normalizeCampaignDetailSnapshot(
  requestedCampaignId: string,
  campaignPayload: unknown,
): CampaignDetailSnapshot {
  const envelope = isRecord(campaignPayload) ? campaignPayload : {};
  const meta = isRecord(envelope.meta) ? envelope.meta : {};
  const campaign = unwrapPayload(campaignPayload);
  const record = isRecord(campaign) ? campaign : {};
  const campaignId = textFrom(record, ['campaign_id', 'campaignId', 'id', 'event_id']) || requestedCampaignId;
  const alertIds = stringListFrom(valueAt(record, ['alert_ids', 'alerts']));
  const alertRows = extractList(record, ['alerts']).length ? extractList(record, ['alerts']) : alertIds.map((id) => ({ alert_id: id }));
  const phaseSummaryRows = extractList(record, ['phase_summaries']);
  const entityIds = stringListFrom(valueAt(record, ['entities']));
  const referenceFixture = appConfig.useMock;
  const score = normalizeScore(numberAt(record, ['score', 'risk_score', 'riskScore']));
  const rawAlertCount = Math.max(alertRows.length, alertIds.length);
  const alertCount = rawAlertCount;
  const phases = buildPhaseCards(record, alertCount, phaseSummaryRows, referenceFixture);
  const phaseDataBacked = valueAt(record, ['phase_data_backed']) === true;
  const impactAsset = buildImpactAsset(record, entityIds, referenceFixture);
  const assetCount = impactAsset.total;
  const firstSeen = formatTimestamp(valueAt(record, ['ts_start', 'start_time', 'first_seen'])) || '-';
  const lastUpdated = formatTimestamp(valueAt(record, ['ts_end', 'end_time', 'ingest_ts', 'updated_at'])) || '-';
  const currentPhase = phases.find((item) => item.status === 'risk')?.phase
    || phases[phases.length - 2]?.phase
    || (referenceFixture ? '数据外传' : '未提供');
  const duration = formatDuration(valueAt(record, ['ts_start', 'start_time']), valueAt(record, ['ts_end', 'end_time']));
  const status = campaignStatus(record, score);
  const activityStatus = statusLabel(textFrom(record, ['activity_status']) || textFrom(record, ['status']));
  const workflowStatus = statusLabel(textFrom(record, ['status'])) || activityStatus;
  const impactAccount = buildImpactAccount(record, referenceFixture);
  const impactBusinessSystem = buildImpactBusinessSystem(record, referenceFixture);
  const impactService = buildImpactService(record, referenceFixture);
  const impactDepartment = buildImpactDepartment(record, referenceFixture);
  const impactCampus = buildImpactCampus(record, referenceFixture);
  const impactDataBacked = normalizeImpactDataBacked(record, referenceFixture);
  const summary = textFrom(record, ['summary', 'description'])
    || (referenceFixture ? '园区科研网络定向窃密战役，跨办公区、科研网与数据中心产生多阶段告警和取证证据。' : '未提供战役摘要');
  const tags = [
    textFrom(record, ['campaign_type', 'campaignType']),
    ...stringListFrom(valueAt(record, ['rule_ids'])).slice(0, 2),
    ...stringListFrom(valueAt(record, ['model_ids'])).slice(0, 2),
  ].filter(Boolean).slice(0, 4);
  const rawEvidenceCompleteness = valueAt(record, ['evidence_completeness']);
  const evidenceRail = normalizeEvidenceRail(record, rawAlertCount, phaseSummaryRows, phaseDataBacked);
  const derivedEvidenceCompleteness = deriveEvidenceCompleteness(evidenceRail);
  const explicitEvidenceCompleteness = typeof rawEvidenceCompleteness === 'number'
    ? rawEvidenceCompleteness
    : typeof rawEvidenceCompleteness === 'string' && rawEvidenceCompleteness.trim()
      ? Number(rawEvidenceCompleteness)
      : Number.NaN;
  const evidenceCompletenessAvailable = (Number.isFinite(explicitEvidenceCompleteness)
    && explicitEvidenceCompleteness >= 0) || derivedEvidenceCompleteness.available;
  const evidenceCompleteness = evidenceCompletenessAvailable
    ? Math.min(100, Math.round(
        Number.isFinite(explicitEvidenceCompleteness) && explicitEvidenceCompleteness >= 0
          ? explicitEvidenceCompleteness
          : derivedEvidenceCompleteness.value,
      ))
    : (referenceFixture ? Math.min(96, Math.round(67 + alertCount / 3 + Math.min(6, assetCount / 10))) : 0);
  const campaignType = textFrom(record, ['campaign_type', 'campaignType']) || '未分类';
  const statusTransitions = normalizeStatusTransitions(record);
  const assignee = textFrom(record, ['owner', 'assignee', 'analyst']) || (referenceFixture ? 'sec_analyst' : '未分配');
  const explicitResponseProgress = optionalNumberAt(record, ['response_progress', 'responseProgress', 'disposition_progress', 'dispositionProgress']);
  const responseProgress = status === '已结束'
    ? 100
    : explicitResponseProgress === undefined
      ? undefined
      : Math.max(0, Math.min(100, Math.round(explicitResponseProgress)));

  return {
    campaignId,
    stateVersion: Math.max(0, Math.trunc(numberAt(record, ['state_version', 'stateVersion']))),
    snapshotId: textFrom(record, ['snapshot_id']) || textFrom(meta, ['snapshot_id']),
    snapshotSHA256: textFrom(record, ['snapshot_sha256']),
    partial: valueAt(meta, ['partial']) === true,
    missingSections: stringListFrom(valueAt(meta, ['missing_sections'])),
    sourceWatermarks: stringRecordFrom(valueAt(meta, ['source_watermarks'])),
    reports: normalizeCampaignReports(record),
    campaignType,
    title: summaryTitle(summary, campaignId),
    riskScore: score,
    currentPhase,
    duration: duration || (referenceFixture ? '2天14小时' : '-'),
    firstSeen,
    lastUpdated,
    status,
    activityStatus,
    workflowStatus,
    assignee,
    alertCount,
    assetCount,
    tags: tags.length ? tags : (referenceFixture ? ['APT 定向攻击', 'C2_Tunnel_v3', 'Data_Exfil_v1'] : []),
    summary,
    profileFacts: [
      fact('战役 ID', campaignId),
      fact('风险评分', `${score}/100`),
      fact('当前阶段', currentPhase),
      fact('持续时间', duration || (referenceFixture ? '2天14小时' : '-')),
      fact('首次发现', firstSeen),
      fact('最近活动', lastUpdated),
      fact('当前状态', status, true),
      fact('负责人', assignee),
      fact('关联告警', `${alertCount} 条`),
      fact('影响资产', `${assetCount} 台`),
    ],
    metrics: [
      metric('风险评分', `${score}/100`, currentPhase, score >= 85 ? 'risk' : 'warn'),
      metric('关联告警', `${alertCount}`, 'alert-service', alertCount ? 'warn' : 'info'),
      metric('影响资产', `${assetCount}`, 'asset graph', assetCount ? 'warn' : 'ok'),
      metric('攻击阶段', `${phases.length}`, 'ATT&CK', 'info'),
      metric('证据完整度', `${evidenceCompleteness}%`, 'evidence bundle', evidenceCompleteness >= 80 ? 'ok' : 'warn'),
      metric('处置进度', responseProgress === undefined ? '暂不可用' : `${responseProgress}%`, status, responseProgress === undefined ? 'info' : responseProgress >= 100 ? 'ok' : 'warn'),
    ],
    phases,
    alerts: buildAlertRows(alertRows, alertIds, phases, assetCount, referenceFixture),
    impactTabs: buildImpactTabs(
      assetCount,
      impactAccount.total,
      impactService.total,
      impactDepartment.total,
      impactCampus.total,
      impactBusinessSystem.total,
      impactDataBacked,
    ),
    topAssets: impactAsset.rows,
    impactAsset,
    impactAccount,
    impactBusinessSystem,
    impactService,
    impactDepartment,
    impactCampus,
    impactDataBacked,
    evidenceCompleteness,
    evidenceCompletenessAvailable,
    phaseDataBacked,
    evidenceRail,
    statusTransitions,
    evidenceChecks: referenceFixture ? buildEvidenceChecks(evidenceCompleteness, alertCount) : buildEvidenceChecksFromRail(evidenceRail),
    evidenceSummaryRows: referenceFixture ? buildEvidenceRows(campaignId, evidenceCompleteness) : buildEvidenceRowsFromRail(evidenceRail),
    evidenceDigest: buildEvidenceDigest(record, firstSeen, lastUpdated, referenceFixture),
    responseFlow: referenceFixture ? buildResponseFlow(firstSeen, lastUpdated, status) : buildResponseFlowFromTransitions(statusTransitions),
    responseActions: referenceFixture ? buildResponseActions(currentPhase, campaignId) : buildResponseActionsFromTransitions(statusTransitions),
    reviewRows: referenceFixture
      ? buildReviewRows(score, currentPhase, evidenceCompleteness)
      : buildRuntimeReviewRows(summary, phases, assetCount, alertCount, evidenceCompletenessAvailable, evidenceCompleteness, assignee),
    evidence: [
      metric('Campaign Detail API', `/v1/campaigns/${campaignId}`, 'primary', 'ok'),
      metric('告警聚合', `${alertCount} 条`, 'alerts', alertCount ? 'warn' : 'info'),
      metric('影响实体', `${assetCount} 个`, 'entities', assetCount ? 'warn' : 'ok'),
      metric('攻击阶段', phases.map((item) => item.phase).join(' / '), 'attack_phases', 'info'),
      metric('审计提示', '报告生成需写入 audit_logs', 'audit', 'info'),
    ],
  };
}

export function buildMockCampaignDetailSnapshot(campaignId: string) {
  return normalizeCampaignDetailSnapshot(campaignId, {
    data: {
      campaign_id: campaignId,
      campaign_type: 'APT 定向窃密',
      score: 92,
      summary: '园区科研网络定向窃密战役',
      ts_start: 1_771_289_520_000,
      ts_end: 1_771_512_300_000,
      ingest_ts: 1_771_512_300_000,
      entities: Array.from({ length: 57 }, (_, index) => `10.12.${Math.floor(index / 20) + 1}.${18 + index}`),
      alert_ids: Array.from({ length: 38 }, (_, index) => `AL-20260620-${String(index + 101).padStart(6, '0')}`),
      alerts: [
        { alert_id: 'AL-20260620-000123', alert_type: 'C2 隧道通信', severity: 'critical', last_seen: 1_771_292_540_000 },
        { alert_id: 'AL-20260620-000119', alert_type: '横向移动 SMB 探测', severity: 'high', last_seen: 1_771_303_900_000 },
        { alert_id: 'AL-20260620-000117', alert_type: '凭证访问异常', severity: 'high', last_seen: 1_771_334_100_000 },
        { alert_id: 'AL-20260620-000112', alert_type: '异常 DNS 隧道', severity: 'medium', last_seen: 1_771_389_600_000 },
        { alert_id: 'AL-20260620-000108', alert_type: '大流量数据外传', severity: 'critical', last_seen: 1_771_452_300_000 },
      ],
      attack_phases: canonicalPhases,
      rule_ids: ['C2_Tunnel_v3', 'Lateral_Move_v2', 'Data_Exfil_v1'],
      model_ids: ['APT_Campaign_Cluster_v2'],
      first_suspicious_file: 'invoice_update.docm',
      file_sha256: 'e1d2f7c3b2...a7b9e5f1c3',
      first_external_domain: 'update-data[.]cloudsync[.]solution',
      resolved_ip: '185.22.14.9',
      first_outbound_at: '2026-06-19 00:18:41',
      last_outbound_at: '2026-06-19 02:58:07',
      __reference_fixture: true,
    },
  });
}

function buildPhaseCards(
  record: Record<string, unknown>,
  alertCount: number,
  phaseSummaryRows: Record<string, unknown>[] = [],
  referenceFixture = false,
): CampaignDetailPhase[] {
  const payloadPhases = stringListFrom(valueAt(record, ['attack_phases', 'phases'])).map(phaseLabel);
  const phases = Array.from(new Set([...(referenceFixture ? canonicalPhases : []), ...payloadPhases])).slice(0, 7);
  const exfiltrationIndex = phases.findIndex((phase) => phase.includes('外传'));
  const activeIndex = exfiltrationIndex >= 0 ? exfiltrationIndex : Math.min(5, phases.length - 1);
  const phaseSummaries = new Map(
    phaseSummaryRows.map((item) => [phaseLabel(textFrom(item, ['phase', 'attack_phase'])), item]),
  );
  return phases.map((phase, index) => ({
    phase,
    time: phaseSummaries.has(phase)
      ? (formatTimestamp(valueAt(phaseSummaries.get(phase), ['last_seen'])) || '-')
      : (referenceFixture ? phaseTime(index) : '-'),
    alertCount: phaseSummaries.size
      ? numberAt(phaseSummaries.get(phase) ?? {}, ['alert_count'])
      : (referenceFixture ? Math.max(1, Math.round(alertCount / phases.length) + (index % 3) - 1) : 0),
    evidenceCount: phaseSummaries.size
      ? numberAt(phaseSummaries.get(phase) ?? {}, ['evidence_count'])
      : (referenceFixture ? 3 + index + (phase.includes('外传') ? 4 : 0) : 0),
    status: phaseStatus(index, activeIndex, phase),
    summary: phaseSummary(phase),
  }));
}

function buildAlertRows(
  alerts: Record<string, unknown>[],
  alertIds: string[],
  phases: CampaignDetailPhase[],
  assetCount: number,
  referenceFixture = false,
): CampaignDetailAlertRow[] {
  const source = alerts.length ? alerts : alertIds.map((id) => ({ alert_id: id }));
  const normalized = source.map((alert, index) => {
    const phase = phases[Math.min(index + 1, Math.max(0, phases.length - 1))];
    const phaseName = phase?.phase || '未提供';
    return {
      告警时间: formatTimestamp(valueAt(alert, ['last_seen', 'timestamp', 'first_seen'])) || (referenceFixture ? `06-20 ${String(3 + index).padStart(2, '0')}:4${index}:18` : '-'),
      告警ID: textFrom(alert, ['alert_id', 'id']) || alertIds[index] || (referenceFixture ? `AL-20260620-${String(123 - index).padStart(6, '0')}` : ''),
      告警名称: textFrom(alert, ['alert_type', 'title', 'name']) || (referenceFixture ? alertNameForPhase(phaseName) : '未提供'),
      攻击阶段: phaseName,
      影响资产: assetCount ? `${Math.max(1, Math.round(assetCount / Math.max(1, index + 6)))} 台` : '0 台',
      风险: severityLabel(textFrom(alert, ['severity', 'risk_level']) || (referenceFixture ? (index < 3 ? 'high' : 'medium') : 'unknown')),
      状态: textFrom(alert, ['status']) || (referenceFixture ? (index < 3 ? '调查中' : '处置中') : '未提供'),
      操作: '下钻',
    };
  });
  while (referenceFixture && normalized.length < 5) {
    const index = normalized.length;
    const phase = phases[Math.min(index + 1, phases.length - 1)];
    normalized.push({
      告警时间: `06-20 ${String(3 + index).padStart(2, '0')}:4${index}:18`,
      告警ID: `AL-20260620-${String(123 - index).padStart(6, '0')}`,
      告警名称: alertNameForPhase(phase.phase),
      攻击阶段: phase.phase,
      影响资产: `${Math.max(2, Math.round(assetCount / (index + 6)))} 台`,
      风险: index < 3 ? '高危' : '中危',
      状态: index < 3 ? '调查中' : '处置中',
      操作: '下钻',
    });
  }
  return normalized;
}

function buildImpactTabs(
  assetCount: number,
  accountCount: number,
  serviceCount: number,
  departmentCount: number,
  campusCount: number,
  businessSystemCount: number,
  dataBacked: CampaignDetailSnapshot['impactDataBacked'],
): CampaignDetailImpactTab[] {
  const value = (available: boolean, count: number, unit: string) => available ? `${count} ${unit}` : '--';
  return [
    { label: '资产', value: value(dataBacked.asset, assetCount, '台'), status: 'risk' },
    { label: '账号', value: value(dataBacked.account, accountCount, '个'), status: 'warn' },
    { label: '服务', value: value(dataBacked.service, serviceCount, '个'), status: 'warn' },
    { label: '部门', value: value(dataBacked.department, departmentCount, '个'), status: 'info' },
    { label: '校区', value: value(dataBacked.campus, campusCount, '个'), status: 'info' },
    { label: '业务系统', value: value(dataBacked['business-system'], businessSystemCount, '个'), status: 'risk' },
  ];
}

function buildImpactService(record: Record<string, unknown>, referenceFixture = false): CampaignDetailImpactService {
  const payloadRows = extractList(record, ['impact_services', 'affected_services', 'services', 'top_services'])
    .map((row) => ({
      服务名称: textFrom(row, ['service_name', 'service', 'name', 'id']) || '',
      端口协议: textFrom(row, ['port_protocol', 'portProtocol', 'port_proto', 'endpoint']) || servicePortProtocol(row),
      风险: severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])),
      依赖关系: textFrom(row, ['dependency', 'depends_on', 'business_system', 'relation', 'dependency_system']) || '',
    }))
    .filter((row) => row.服务名称);
  const defaults: CampaignDetailServiceRow[] = [
    { 服务名称: 'PostgreSQL', 端口协议: '5432/TCP', 风险: '高危', 依赖关系: '科研管理系统' },
    { 服务名称: 'MinIO API', 端口协议: '9000/TCP', 风险: '高危', 依赖关系: '证据归档' },
    { 服务名称: 'LDAP', 端口协议: '389/TCP', 风险: '中危', 依赖关系: '统一认证' },
    { 服务名称: 'NFS', 端口协议: '2049/TCP', 风险: '中危', 依赖关系: '文件共享' },
    { 服务名称: 'Redis', 端口协议: '6379/TCP', 风险: '中危', 依赖关系: '会话缓存' },
  ];
  const usesFallbackRows = referenceFixture && payloadRows.length === 0;
  const rows = usesFallbackRows ? defaults : payloadRows.slice(0, 5);
  const explicitTotal = numberAt(record, ['service_count', 'affected_service_count', 'services_total']);
  const total = explicitTotal || (usesFallbackRows ? 42 : payloadRows.length);
  const high = numberAt(record, ['service_high_risk', 'high_risk_services'])
    || (usesFallbackRows ? 11 : payloadRows.filter((row) => row.风险.includes('高')).length);
  const medium = numberAt(record, ['service_medium_risk', 'medium_risk_services'])
    || (usesFallbackRows ? 18 : payloadRows.filter((row) => row.风险.includes('中')).length);
  const low = numberAt(record, ['service_low_risk', 'low_risk_services'])
    || Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响服务',
    breakdown: usesFallbackRows
      ? [
          { label: '高风险', count: 11, percent: '26.2%', status: 'risk' },
          { label: '中风险', count: 18, percent: '42.9%', status: 'warn' },
          { label: '低风险', count: 13, percent: '30.9%', status: 'ok' },
        ]
      : normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function buildImpactDepartment(record: Record<string, unknown>, referenceFixture = false): CampaignDetailImpactDepartment {
  const payloadRows = extractList(record, ['impact_departments', 'affected_departments', 'departments', 'top_departments'])
    .map((row) => ({
      部门名称: textFrom(row, ['department', 'department_name', 'dept', 'name', 'id']) || '',
      责任人: textFrom(row, ['owner', 'assignee', 'principal', 'responsible_person', 'leader']) || '',
      风险: severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])),
      处置进度: progressPercent(row, ['progress', 'response_progress', 'disposal_progress', 'remediation_progress']),
    }))
    .filter((row) => row.部门名称);
  const defaults: CampaignDetailDepartmentRow[] = [
    { 部门名称: '科研处', 责任人: '王主任', 风险: '高危', 处置进度: 40 },
    { 部门名称: '信息中心', 责任人: 'sec_manager', 风险: '高危', 处置进度: 55 },
    { 部门名称: '财务处', 责任人: '李主任', 风险: '中危', 处置进度: 60 },
    { 部门名称: '教务处', 责任人: '张主任', 风险: '中危', 处置进度: 72 },
    { 部门名称: '图书馆', 责任人: '运维组', 风险: '中危', 处置进度: 80 },
  ];
  const usesFallbackRows = referenceFixture && payloadRows.length === 0;
  const rows = usesFallbackRows ? defaults : payloadRows.slice(0, 5);
  const explicitTotal = numberAt(record, ['department_count', 'affected_department_count', 'departments_total']);
  const total = explicitTotal || (usesFallbackRows ? 7 : payloadRows.length);
  const high = numberAt(record, ['department_high_risk', 'high_risk_departments'])
    || (usesFallbackRows ? 2 : payloadRows.filter((row) => row.风险.includes('高')).length);
  const medium = numberAt(record, ['department_medium_risk', 'medium_risk_departments'])
    || (usesFallbackRows ? 3 : payloadRows.filter((row) => row.风险.includes('中')).length);
  const low = numberAt(record, ['department_low_risk', 'low_risk_departments'])
    || Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响部门',
    breakdown: normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function buildImpactCampus(record: Record<string, unknown>, referenceFixture = false): CampaignDetailImpactCampus {
  const payloadRows = extractList(record, ['impact_campuses', 'affected_campuses', 'campuses', 'top_campuses'])
    .map((row) => ({
      校区楼宇: textFrom(row, ['campus_building', 'campus', 'building', 'name', 'id']) || '',
      覆盖资产: numberAt(row, ['covered_assets', 'asset_count', 'assets', 'affected_assets']),
      风险: severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])),
      链路: textFrom(row, ['link_path', 'network_path', 'path', 'route', 'link']) || '',
    }))
    .filter((row) => row.校区楼宇);
  const defaults: CampaignDetailCampusRow[] = [
    { 校区楼宇: '主校区-数据中心', 覆盖资产: 26, 风险: '高危', 链路: '核心链路' },
    { 校区楼宇: '主校区-科研楼', 覆盖资产: 18, 风险: '中危', 链路: '东西向' },
    { 校区楼宇: '东校区-办公楼', 覆盖资产: 9, 风险: '中危', 链路: 'VPN 链路' },
    { 校区楼宇: '南校区-教学楼', 覆盖资产: 7, 风险: '中危', 链路: '无线网' },
    { 校区楼宇: '西校区-图书馆', 覆盖资产: 5, 风险: '低危', 链路: '出口链路' },
  ];
  const usesFallbackRows = referenceFixture && payloadRows.length === 0;
  const rows = usesFallbackRows ? defaults : payloadRows.slice(0, 5);
  const explicitTotal = numberAt(record, ['campus_count', 'affected_campus_count', 'campuses_total']);
  const total = explicitTotal || (usesFallbackRows ? 4 : payloadRows.length);
  const high = numberAt(record, ['campus_high_risk', 'high_risk_campuses'])
    || (usesFallbackRows ? 1 : payloadRows.filter((row) => row.风险.includes('高')).length);
  const medium = numberAt(record, ['campus_medium_risk', 'medium_risk_campuses'])
    || (usesFallbackRows ? 2 : payloadRows.filter((row) => row.风险.includes('中')).length);
  const low = numberAt(record, ['campus_low_risk', 'low_risk_campuses'])
    || Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响校区',
    breakdown: normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function buildImpactBusinessSystem(record: Record<string, unknown>, referenceFixture = false): CampaignDetailImpactBusinessSystem {
  const payloadRows = extractList(record, ['impact_business_systems', 'affected_business_systems', 'business_systems', 'top_business_systems'])
    .map((row) => ({
      业务系统: textFrom(row, ['business_system', 'system', 'name', 'id']) || '',
      关键服务: textFrom(row, ['key_service', 'service', 'services', 'dependency']) || '',
      风险: severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])),
      恢复优先级: textFrom(row, ['recovery_priority', 'priority', 'recovery']) || '',
    }))
    .filter((row) => row.业务系统);
  const defaults: CampaignDetailBusinessSystemRow[] = [
    { 业务系统: '科研管理系统', 关键服务: 'DB/API', 风险: '高危', 恢复优先级: 'P0' },
    { 业务系统: '数据分析平台', 关键服务: 'Spark/MinIO', 风险: '高危', 恢复优先级: 'P0' },
    { 业务系统: '文件存储系统', 关键服务: 'NFS/SMB', 风险: '中危', 恢复优先级: 'P1' },
    { 业务系统: '统一认证平台', 关键服务: 'LDAP/OIDC', 风险: '中危', 恢复优先级: 'P1' },
    { 业务系统: '教工终端管理', 关键服务: 'Agent API', 风险: '中危', 恢复优先级: 'P2' },
  ];
  const usesFallbackRows = referenceFixture && payloadRows.length === 0;
  const rows = usesFallbackRows ? defaults : payloadRows.slice(0, 5);
  const explicitTotal = numberAt(record, ['business_system_count', 'affected_business_system_count', 'businessSystemsTotal']);
  const total = explicitTotal || (usesFallbackRows ? 9 : payloadRows.length);
  const high = numberAt(record, ['business_system_high_risk', 'high_risk_business_systems'])
    || (usesFallbackRows ? 3 : payloadRows.filter((row) => row.风险.includes('高')).length);
  const medium = numberAt(record, ['business_system_medium_risk', 'medium_risk_business_systems'])
    || (usesFallbackRows ? 4 : payloadRows.filter((row) => row.风险.includes('中')).length);
  const low = numberAt(record, ['business_system_low_risk', 'low_risk_business_systems'])
    || Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响系统',
    breakdown: usesFallbackRows
      ? [
          { label: '高风险', count: 3, percent: '33.3%', status: 'risk' },
          { label: '中风险', count: 4, percent: '44.5%', status: 'warn' },
          { label: '低风险', count: 2, percent: '22.2%', status: 'ok' },
        ]
      : normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function buildImpactAccount(record: Record<string, unknown>, referenceFixture = false): CampaignDetailImpactAccount {
  const accountRows = extractList(record, ['impact_accounts', 'affected_accounts', 'accounts', 'top_accounts'])
    .map((row) => ({
      账号: textFrom(row, ['account', 'account_id', 'username', 'name', 'id']) || '',
      账号类型: textFrom(row, ['account_type', 'type', 'category']) || '',
      权限风险: severityLabel(textFrom(row, ['permission_risk', 'risk', 'risk_level', 'severity'])),
      登录链路: textFrom(row, ['login_path', 'access_path', 'path', 'route']) || '',
    }))
    .filter((row) => row.账号);
  const defaults: CampaignDetailAccountRow[] = [
    { 账号: 'svc_backup', 账号类型: '服务账号', 权限风险: '高危', 登录链路: 'VPN -> DB-SRV-07' },
    { 账号: 'temp_admin', 账号类型: '临时账号', 权限风险: '高危', 登录链路: '跳板机 -> 核心库' },
    { 账号: 'li.ming', 账号类型: '人员账号', 权限风险: '中危', 登录链路: '办公网 -> NAS-02' },
    { 账号: 'ops_reader', 账号类型: '只读账号', 权限风险: '中危', 登录链路: '堡垒机 -> 日志库' },
    { 账号: 'svc_deploy', 账号类型: '服务账号', 权限风险: '中危', 登录链路: 'CI -> K8s API' },
  ];
  const usesFallbackRows = referenceFixture && accountRows.length === 0;
  const rows = usesFallbackRows ? defaults : accountRows.slice(0, 5);
  const total = optionalNumberAt(record, ['account_count', 'affected_account_count', 'affectedAccounts', 'accounts_total'])
    ?? (usesFallbackRows ? 31 : accountRows.length);
  const high = optionalNumberAt(record, ['account_high_risk', 'high_risk_accounts'])
    ?? (usesFallbackRows ? 8 : accountRows.filter((row) => row.权限风险.includes('高')).length);
  const medium = optionalNumberAt(record, ['account_medium_risk', 'medium_risk_accounts'])
    ?? (usesFallbackRows ? 14 : accountRows.filter((row) => row.权限风险.includes('中')).length);
  const low = optionalNumberAt(record, ['account_low_risk', 'low_risk_accounts']) ?? Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响账号',
    breakdown: normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function servicePortProtocol(row: Record<string, unknown>) {
  const port = textFrom(row, ['port', 'listen_port', 'service_port']);
  const protocol = textFrom(row, ['protocol', 'transport', 'proto']).toUpperCase();
  if (port && protocol) return `${port}/${protocol}`;
  return port || protocol;
}

function normalizeAccountRiskBreakdown(total: number, high: number, medium: number, low: number): CampaignDetailImpactRiskRow[] {
  const safeTotal = Math.max(1, total);
  const formatPercent = (value: number) => `${((value / safeTotal) * 100).toFixed(1)}%`;
  return [
    { label: '高风险', count: high, percent: formatPercent(high), status: 'risk' },
    { label: '中风险', count: medium, percent: formatPercent(medium), status: 'warn' },
    { label: '低风险', count: low, percent: formatPercent(low), status: 'ok' },
  ];
}

function progressPercent(source: Record<string, unknown>, keys: string[]) {
  const value = numberAt(source, keys);
  if (!Number.isFinite(value) || value <= 0) return 0;
  const percent = value <= 1 ? value * 100 : value;
  return Math.max(0, Math.min(100, Math.round(percent)));
}

function buildImpactAsset(
  record: Record<string, unknown>,
  entities: string[],
  referenceFixture = false,
): CampaignDetailImpactAsset {
  const payloadRows = extractList(record, ['impact_assets']);
  const rows = buildTopAssets(record, entities, referenceFixture);
  const explicitTotal = numberAt(record, ['asset_count', 'affected_asset_count', 'assets_total']);
  const total = explicitTotal || (payloadRows.length ? payloadRows.length : entities.length);
  const high = referenceFixture
    ? 12
    : payloadRows.filter((row) => severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])).includes('高')).length;
  const medium = referenceFixture
    ? 23
    : payloadRows.filter((row) => severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])).includes('中')).length;
  const low = referenceFixture
    ? Math.max(0, total - high - medium)
    : payloadRows.filter((row) => severityLabel(textFrom(row, ['risk', 'risk_level', 'severity'])).includes('低')).length
      || Math.max(0, total - high - medium);
  return {
    total,
    unit: '受影响资产',
    breakdown: normalizeAccountRiskBreakdown(total, high, medium, low),
    rows,
  };
}

function normalizeImpactDataBacked(
  record: Record<string, unknown>,
  referenceFixture: boolean,
): CampaignDetailSnapshot['impactDataBacked'] {
  const explicit = valueAt(record, ['impact_data_backed']);
  const source = isRecord(explicit) ? explicit : {};
  const read = (key: string, fallbackKeys: string[]) => {
    const value = valueAt(source, [key]);
    if (typeof value === 'boolean') return value;
    return referenceFixture || fallbackKeys.some((fallbackKey) => extractList(record, [fallbackKey]).length > 0);
  };
  return {
    asset: read('assets', ['impact_assets']),
    account: read('accounts', ['impact_accounts', 'affected_accounts', 'top_accounts']),
    service: read('services', ['impact_services', 'affected_services', 'top_services']),
    department: read('departments', ['impact_departments', 'affected_departments', 'top_departments']),
    campus: read('campuses', ['impact_campuses', 'affected_campuses', 'top_campuses']),
    'business-system': read('business_systems', ['impact_business_systems', 'affected_business_systems', 'top_business_systems']),
  };
}

function buildTopAssets(record: Record<string, unknown>, entities: string[], referenceFixture = false): CampaignDetailAssetRow[] {
  const defaults = [
    ['科研网-SRV-021', '服务器', '科研处', '科研数据平台', '高危', 'PCAP / Session'],
    ['办公区-WS-1024', '终端', '信息中心', '办公协同', '高危', 'EDR / 日志'],
    ['核心区-DC-01', '域控', '信息中心', '统一认证', '高危', '账号访问'],
    ['图书馆-NAS-03', '存储', '图书馆', '文献库', '中危', '流量画像'],
    ['教学区-PC-0421', '终端', '教务处', '教务系统', '中危', 'Session'],
  ];
  if (!referenceFixture) {
    const impactRows = extractList(record, ['impact_assets']).map((row) => ({
      资产: textFrom(row, ['asset', 'asset_name', 'display_code', 'hostname', 'ip']) || '',
      类型: textFrom(row, ['type', 'asset_type']) || '未提供',
      部门: textFrom(row, ['department']) || '未提供',
      业务系统: textFrom(row, ['business_system', 'system']) || '未提供',
      风险: textFrom(row, ['risk', 'risk_level']) || '未提供',
      证据: textFrom(row, ['evidence', 'source']) || '未提供',
    })).filter((row) => row.资产);
    if (impactRows.length) return impactRows.slice(0, 5);
    return entities.slice(0, 5).map((entity) => ({
      资产: entity,
      类型: '未提供',
      部门: '未提供',
      业务系统: '未提供',
      风险: '未提供',
      证据: '未提供',
    }));
  }
  return defaults.map((row, index) => ({
    资产: entities[index] || row[0],
    类型: row[1],
    部门: row[2],
    业务系统: row[3],
    风险: row[4],
    证据: row[5],
  }));
}

function normalizeEvidenceRail(
  record: Record<string, unknown>,
  rawAlertCount: number,
  phaseSummaryRows: Record<string, unknown>[],
  phaseDataBacked: boolean,
): CampaignDetailEvidenceRailItem[] {
  const rows = extractList(record, ['evidence_summary']);
  if (rows.length) {
    return rows.slice(0, 5).map((row) => {
      const currentValue = valueAt(row, ['current']);
      const expectedValue = valueAt(row, ['expected']);
      const current = typeof currentValue === 'number' && Number.isFinite(currentValue) ? currentValue : null;
      const expected = typeof expectedValue === 'number' && Number.isFinite(expectedValue) ? expectedValue : null;
      return {
        key: textFrom(row, ['key']) || textFrom(row, ['label']),
        label: textFrom(row, ['label']) || '未命名证据',
        current,
        expected,
        available: valueAt(row, ['available']) === true,
      };
    });
  }

  const evidenceCount = phaseDataBacked
    ? phaseSummaryRows.reduce((sum, row) => sum + numberAt(row, ['evidence_count']), 0)
    : 0;
  return [
    { key: 'alerts', label: '告警', current: rawAlertCount, expected: null, available: true },
    { key: 'packet_session', label: 'PCAP / Session', current: phaseDataBacked ? evidenceCount : null, expected: null, available: phaseDataBacked },
    { key: 'logs', label: '日志', current: null, expected: null, available: false },
    { key: 'graph_paths', label: '图谱路径', current: null, expected: null, available: false },
    { key: 'response_records', label: '处置记录', current: null, expected: null, available: false },
  ];
}

function normalizeStatusTransitions(record: Record<string, unknown>): CampaignDetailStatusTransition[] {
  return extractList(record, ['status_transitions']).map((row) => ({
    status: textFrom(row, ['status']),
    changedAt: textFrom(row, ['changed_at', 'changedAt']),
    source: textFrom(row, ['source']) || 'runtime',
  })).filter((row) => row.status);
}

function buildEvidenceChecksFromRail(rows: CampaignDetailEvidenceRailItem[]): CampaignDetailEvidenceCheck[] {
  return rows.filter((row) => row.available).map((row) => ({
    label: row.label,
    value: row.expected === null ? String(row.current ?? 0) : `${row.current ?? 0} / ${row.expected}`,
    percent: row.expected !== null && row.expected > 0
      ? Math.min(100, Math.round(((row.current ?? 0) / row.expected) * 100))
      : 0,
    status: row.expected === null ? 'info' : (row.current ?? 0) < row.expected ? 'warn' : 'ok',
  }));
}

function deriveEvidenceCompleteness(rows: CampaignDetailEvidenceRailItem[]) {
  const measurable = rows.filter((row) => row.available && row.current !== null && row.expected !== null && row.expected > 0);
  if (!measurable.length) return { available: false, value: 0 };
  const current = measurable.reduce((sum, row) => sum + Math.max(0, row.current ?? 0), 0);
  const expected = measurable.reduce((sum, row) => sum + Math.max(0, row.expected ?? 0), 0);
  return {
    available: expected > 0,
    value: expected > 0 ? Math.min(100, (current / expected) * 100) : 0,
  };
}

function buildEvidenceRowsFromRail(rows: CampaignDetailEvidenceRailItem[]): CampaignDetailEvidenceSummaryRow[] {
  return rows.filter((row) => row.available).map((row) => {
    const current = row.current ?? 0;
    const completeness = row.expected && row.expected > 0 ? Math.round((current / row.expected) * 100) : null;
    return {
      证据类型: row.label,
      文件记录: String(current),
      完整度: completeness === null ? '--' : `${Math.min(100, completeness)}%`,
      状态: completeness === null ? '已采集' : completeness >= 100 ? '完整' : '待补齐',
    };
  });
}

function buildResponseFlowFromTransitions(rows: CampaignDetailStatusTransition[]): CampaignDetailFlowStep[] {
  const normalized = rows.map((row) => ({
    ...row,
    status: row.status.toLowerCase(),
    time: compactFlowTime(formatTimestamp(row.changedAt)),
  }));
  const byStatus = (statuses: string[]) => normalized.find((row) => statuses.includes(row.status));
  const discovered = normalized[0];
  const investigating = byStatus(['investigating', 'active']);
  const contained = byStatus(['contained']);
  const closed = byStatus(['closed']);
  const currentIndex = closed ? 5 : contained ? 2 : investigating ? 1 : discovered ? 0 : -1;
  const definitions = [
    { title: '发现', row: discovered },
    { title: '研判', row: investigating },
    { title: '遏制', row: contained },
    { title: '根除', row: undefined },
    { title: '恢复', row: closed },
    { title: '复盘', row: undefined },
  ];
  return definitions.map((definition, index) => ({
    title: definition.title,
    time: definition.row?.time ?? '-',
    status: index < currentIndex ? 'ok' : index === currentIndex ? 'warn' : 'info',
  }));
}

function compactFlowTime(value: string) {
  if (!value || value === '-') return '-';
  const match = value.match(/(?:\d{4}-)?(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
  return match ? `${match[1]}-${match[2]} ${match[3]}:${match[4]}` : value;
}

function buildEvidenceChecks(completeness: number, alertCount: number): CampaignDetailEvidenceCheck[] {
  return [
    { label: 'PCAP', value: '18 个窗口', percent: 96, status: 'ok' },
    { label: 'Session', value: `${Math.max(42, alertCount * 3)} 条`, percent: 92, status: 'ok' },
    { label: '日志', value: 'IDS / EDR / Audit', percent: 88, status: 'ok' },
    { label: '图谱路径', value: '12 条关键路径', percent: 82, status: 'warn' },
    { label: '处置记录', value: '5 个动作', percent: completeness, status: 'info' },
    { label: '完整度', value: `${completeness}%`, percent: completeness, status: completeness >= 80 ? 'ok' : 'warn' },
  ];
}

function buildEvidenceRows(campaignId: string, completeness: number): CampaignDetailEvidenceSummaryRow[] {
  return [
    { 证据类型: 'PCAP', 文件记录: `${campaignId}.pcap.zip`, 完整度: '96%', 状态: '已归档' },
    { 证据类型: 'Session', 文件记录: `${campaignId}-sessions.json`, 完整度: '92%', 状态: '已归档' },
    { 证据类型: '日志', 文件记录: `${campaignId}-logs.bundle`, 完整度: '88%', 状态: '已关联' },
    { 证据类型: '图谱路径', 文件记录: `${campaignId}-graph-paths.json`, 完整度: '82%', 状态: '待补充' },
    { 证据类型: '处置记录', 文件记录: `${campaignId}-response.audit`, 完整度: `${completeness}%`, 状态: '写入审计' },
  ];
}

function buildEvidenceDigest(
  record: Record<string, unknown>,
  firstSeen: string,
  lastUpdated: string,
  referenceFixture: boolean,
): CampaignDetailEvidenceDigestItem[] {
  const firstSuspiciousFile = textFrom(record, ['first_suspicious_file', 'suspicious_file', 'file_name']);
  const sha256 = textFrom(record, ['file_sha256', 'sha256']);
  const firstExternalDomain = textFrom(record, ['first_external_domain', 'external_domain', 'domain']);
  const resolvedIp = textFrom(record, ['resolved_ip', 'external_ip', 'destination_ip']);
  const firstOutboundAt = formatTimestamp(valueAt(record, ['first_outbound_at', 'first_external_at']))
    || textFrom(record, ['first_outbound_at', 'first_external_at']);
  const lastOutboundAt = formatTimestamp(valueAt(record, ['last_outbound_at', 'last_external_at']))
    || textFrom(record, ['last_outbound_at', 'last_external_at']);
  const outboundWindow = [firstOutboundAt, lastOutboundAt].filter(Boolean).join('\n');
  return [
    { label: '首个可疑文件', value: firstSuspiciousFile || (referenceFixture ? 'invoice_update.docm' : '--') },
    { label: 'SHA256', value: sha256 || (referenceFixture ? 'e1d2f7c3b2...a7b9e5f1c3' : '--') },
    { label: '首次外联域名', value: firstExternalDomain || (referenceFixture ? 'update-data[.]cloudsync[.]solution' : '--') },
    { label: '解析 IP', value: resolvedIp || (referenceFixture ? '185.22.14.9' : '--') },
    {
      label: '首次外联时间',
      value: outboundWindow || (referenceFixture ? '2026-06-19 00:18:41\n2026-06-19 02:58:07' : [firstSeen, lastUpdated].filter((item) => item !== '-').join('\n') || '--'),
    },
  ];
}

function buildResponseFlow(firstSeen: string, lastUpdated: string, status: string): CampaignDetailFlowStep[] {
  return [
    { title: '发现', time: firstSeen.slice(5, 16), status: 'ok' },
    { title: '研判', time: '06-18 17:30', status: 'info' },
    { title: '遏制', time: '06-18 20:10', status: 'risk' },
    { title: '根除', time: '06-19 01:30', status: 'info' },
    { title: '恢复', time: lastUpdated.slice(5, 16), status: status === '已结束' ? 'ok' : 'info' },
    { title: '复盘', time: status === '已结束' ? '进行中' : '待完成', status: status === '已结束' ? 'info' : 'info' },
  ];
}

function buildResponseActions(currentPhase: string, campaignId: string): CampaignDetailActionRow[] {
  return [
    { 动作: '隔离受控主机', 目标: '57 台影响资产', 负责人: 'sec_analyst', 状态: '待确认' },
    { 动作: '阻断 C2 出口', 目标: '185.22.14.9/443', 负责人: 'net_ops', 状态: '执行中' },
    { 动作: '重置凭证', 目标: '18 个账号', 负责人: 'iam_admin', 状态: '排队中' },
    { 动作: '生成取证包', 目标: campaignId, 负责人: 'forensics', 状态: '已完成' },
    { 动作: '同步攻击链', 目标: currentPhase, 负责人: 'threat_hunter', 状态: '已完成' },
  ];
}

function buildResponseActionsFromTransitions(rows: CampaignDetailStatusTransition[]): CampaignDetailActionRow[] {
  return rows.slice(-8).reverse().map((row, index) => ({
    动作: statusTransitionLabel(row.status),
    目标: compactFlowTime(formatTimestamp(row.changedAt)),
    负责人: row.source || 'runtime',
    状态: index === 0 ? '最新记录' : '已记录',
  }));
}

function statusTransitionLabel(status: string) {
  const normalized = status.toLowerCase();
  if (normalized === 'new') return '发现战役';
  if (['investigating', 'active'].includes(normalized)) return '研判更新';
  if (normalized === 'contained') return '完成遏制';
  if (normalized === 'closed') return '关闭战役';
  return `状态：${status}`;
}

function buildRuntimeReviewRows(
  summary: string,
  phases: CampaignDetailPhase[],
  assetCount: number,
  alertCount: number,
  evidenceAvailable: boolean,
  evidenceCompleteness: number,
  assignee: string,
): CampaignDetailReviewRow[] {
  return [
    { 维度: '战役摘要', 结论: summary, 状态: '已记录' },
    { 维度: '攻击阶段', 结论: phases.map((item) => item.phase).join(' → ') || '未提供', 状态: phases.length ? '已关联' : '待补充' },
    { 维度: '影响范围', 结论: `${assetCount} 台资产 / ${alertCount} 条告警`, 状态: assetCount || alertCount ? '已关联' : '待补充' },
    { 维度: '证据状态', 结论: evidenceAvailable ? `证据包完整度 ${evidenceCompleteness}%` : '接口尚未提供可计算完整度', 状态: evidenceAvailable ? '已采集' : '待补充' },
    { 维度: '负责人', 结论: assignee, 状态: assignee === '未分配' ? '待分配' : '已分配' },
  ];
}

function buildReviewRows(score: number, currentPhase: string, completeness: number): CampaignDetailReviewRow[] {
  return [
    { 维度: '根因', 结论: 'VPN 弱口令与邮件投递共同触发初始访问', 状态: score >= 85 ? '高风险' : '中风险' },
    { 维度: '阻断点', 结论: `${currentPhase} 阶段已形成可操作阻断点`, 状态: '待确认' },
    { 维度: '遗留风险', 结论: '部分横向移动痕迹仍需补齐主机侧日志', 状态: '中风险' },
    { 维度: '整改建议', 结论: '收紧外联策略，轮换高权账号，补齐 EDR 覆盖', 状态: '执行中' },
    { 维度: '样本回流', 结论: '误报/真实样本进入 MLOps 反馈池', 状态: '已就绪' },
    { 维度: '证据验收', 结论: `证据包完整度 ${completeness}%`, 状态: completeness >= 80 ? '通过' : '待补' },
  ];
}

function metric(
  label: string,
  value: string,
  delta: string,
  status: CampaignDetailMetric['status'],
): CampaignDetailMetric {
  return { label, value, delta, status };
}

function fact(label: string, value: string, status = false) {
  return { label, value, status };
}

function summaryTitle(summary: string, campaignId: string) {
  if (summary && summary.length <= 24) return summary;
  if (summary) return `${summary.slice(0, 23)}...`;
  return `${campaignId} 战役`;
}

function phaseLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('initial') || value.includes('初始')) return '初始访问';
  if (lower.includes('execution') || value.includes('执行')) return '执行';
  if (lower.includes('persist') || value.includes('持久')) return '持久化';
  if (lower.includes('lateral') || value.includes('横向')) return '横向移动';
  if (lower.includes('c2') || lower.includes('command') || value.includes('外联')) return 'C2通信';
  if (lower.includes('exfil') || value.includes('外传')) return '数据外传';
  if (value.includes('处置') || value.includes('闭环')) return '处置闭环';
  return value || '未知阶段';
}

function phaseTime(index: number) {
  return ['06-19 09:12', '06-19 10:42', '06-19 13:18', '06-19 18:33', '06-20 02:16', '06-20 03:42', '06-20 10:08'][index] || '06-20';
}

function phaseStatus(index: number, activeIndex: number, phase: string): CampaignDetailMetric['status'] {
  if (phase.includes('外传') || index === activeIndex) return 'risk';
  if (phase.includes('横向') || phase.includes('C2')) return 'warn';
  if (phase.includes('闭环')) return 'info';
  return 'ok';
}

function phaseSummary(phase: string) {
  if (phase.includes('初始')) return '邮件投递与 VPN 异常登录';
  if (phase.includes('执行')) return '脚本执行与工具落地';
  if (phase.includes('持久')) return '计划任务与服务注册';
  if (phase.includes('横向')) return 'SMB / RDP 横向移动';
  if (phase.includes('C2')) return 'TLS 隧道与 DNS Beacon';
  if (phase.includes('外传')) return '科研数据压缩外传';
  return '阻断、取证、反馈学习';
}

function alertNameForPhase(phase: string) {
  if (phase.includes('横向')) return '横向移动 SMB 探测';
  if (phase.includes('C2')) return 'C2 隧道通信';
  if (phase.includes('外传')) return '大流量数据外传';
  if (phase.includes('持久')) return '异常计划任务创建';
  return '异常登录与工具投递';
}

function severityLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('critical') || lower.includes('high') || value.includes('高')) return '高危';
  if (lower.includes('medium') || value.includes('中')) return '中危';
  if (lower.includes('low') || value.includes('低')) return '低危';
  return value || '中危';
}

function statusLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower === 'new' || value === '新建') return '新建';
  if (lower === 'active' || value === '活跃中') return '活跃中';
  if (lower === 'investigating' || value === '调查中') return '调查中';
  if (lower === 'contained' || value === '处置中') return '处置中';
  if (lower === 'closed' || value === '已结束') return '已结束';
  return value || '未设置';
}

function campaignStatus(record: Record<string, unknown>, score: number) {
  const explicit = textFrom(record, ['status', 'state']);
  if (explicit) return statusLabel(explicit);
  return score >= 85 ? '进行中' : '观察中';
}

function formatDuration(start: unknown, end: unknown) {
  const startMs = normalizeTimeMs(start);
  const endMs = normalizeTimeMs(end);
  if (!startMs || !endMs || endMs < startMs) return '';
  const hours = Math.max(1, Math.round((endMs - startMs) / 3_600_000));
  const days = Math.floor(hours / 24);
  const restHours = hours % 24;
  if (days > 0) return `${days}天${restHours}小时`;
  return `${hours}小时`;
}

function normalizeScore(value: number) {
  if (!Number.isFinite(value)) return 92;
  if (value <= 1) return Math.round(value * 100);
  return Math.max(0, Math.min(100, Math.round(value)));
}

function formatTimestamp(value: unknown) {
  const ms = normalizeTimeMs(value);
  if (!ms) return typeof value === 'string' ? value : '';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
    .format(new Date(ms))
    .replace(/\//g, '-');
}

function normalizeTimeMs(value: unknown) {
  if (value === undefined || value === null || value === '') return 0;
  if (typeof value === 'string') {
    const numeric = Number(value);
    if (Number.isFinite(numeric) && numeric > 0) return numeric > 100_000_000_000 ? numeric : numeric * 1000;
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return 0;
  return value > 100_000_000_000 ? value : value * 1000;
}

function unwrapPayload(payload: unknown): unknown {
  if (!isRecord(payload)) return payload;
  if ('data' in payload) return unwrapPayload(payload.data);
  return payload;
}

function extractList(payload: unknown, keys: string[]): Record<string, unknown>[] {
  const data = unwrapPayload(payload);
  if (Array.isArray(data)) return data.filter(isRecord);
  if (!isRecord(data)) return [];
  for (const key of keys) {
    const value = data[key];
    if (Array.isArray(value)) return value.filter(isRecord);
    if (isRecord(value)) {
      const nested = extractList(value, keys);
      if (nested.length) return nested;
    }
  }
  return [];
}

function normalizeCampaignReports(record: Record<string, unknown>): CampaignDetailReport[] {
  return extractList(record, ['reports']).map((item) => ({
    reportId: textFrom(item, ['report_id']),
    jobId: textFrom(item, ['job_id']),
    format: textFrom(item, ['format']),
    status: textFrom(item, ['status']),
    campaignRevision: Math.max(0, Math.trunc(numberAt(item, ['campaign_revision']))),
    sourceSnapshotId: textFrom(item, ['source_snapshot_id']),
    snapshotId: textFrom(item, ['snapshot_id']),
    snapshotSHA256: textFrom(item, ['snapshot_sha256']),
    artifactSHA256: textFrom(item, ['artifact_sha256']),
    sizeBytes: Math.max(0, Math.trunc(numberAt(item, ['size_bytes']))),
    attempts: Math.max(0, Math.trunc(numberAt(item, ['attempts']))),
    errorMessage: textFrom(item, ['error_message']),
  })).filter((item) => item.reportId);
}

function stringRecordFrom(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(
    Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === 'string'),
  );
}

function valueAt(source: unknown, keys: string[]) {
  if (!isRecord(source)) return undefined;
  for (const key of keys) {
    if (key in source) return source[key];
  }
  return undefined;
}

function textFrom(source: unknown, keys: string[]) {
  const value = valueAt(source, keys);
  if (value === undefined || value === null) return '';
  return String(value);
}

function numberAt(source: Record<string, unknown>, keys: string[]) {
  const value = valueAt(source, keys);
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function optionalNumberAt(source: Record<string, unknown>, keys: string[]) {
  const value = valueAt(source, keys);
  if (value === undefined || value === null || value === '') return undefined;
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : undefined;
}

function stringListFrom(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value
      .map((item) => {
        if (typeof item === 'string' || typeof item === 'number') return String(item);
        if (isRecord(item)) return textFrom(item, ['alert_id', 'id', 'entity_id', 'name', 'ip']);
        return '';
      })
      .filter(Boolean);
  }
  if (typeof value === 'string' && value.includes(',')) return value.split(',').map((item) => item.trim()).filter(Boolean);
  if (typeof value === 'string' && value) return [value];
  return [];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
