import { appConfig } from '@/config/runtime';
import { api } from '@/services/api';
import { alertStatusLabel } from '@/services/alertStatus';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

export type AlertDetailMetric = {
  label: string;
  value: string;
  delta: string;
  status: 'ok' | 'warn' | 'risk' | 'info';
};

export type AlertDetailAsset = {
  title: string;
  role: string;
  ip: string;
  hostname: string;
  service: string;
  business: string;
  risk: string;
  facts?: Array<{ label: string; value: string }>;
};

export type AlertDetailTimelineItem = {
  time: string;
  title: string;
  description: string;
  status: 'ok' | 'warn' | 'risk' | 'info';
};

export type AlertDetailEvidenceRow = {
  证据类型: string;
  文件记录: string;
  内容摘要: string;
  大小: string;
  生成时间: string;
  状态: string;
  操作: string;
  evidenceKind?: string;
  hashValue?: string;
  signedUrl?: string;
  fileTags?: string[];
  pcapEvidence?: AlertDetailPcapEvidence;
  sessionEvidence?: AlertDetailSessionEvidence;
  graphPath?: AlertDetailGraphPathEvidence;
  logEvidence?: AlertDetailLogEvidence;
};

export type AlertDetailPcapEvidence = {
  fileName: string;
  contentSummary: string;
  size: string;
  generatedAt: string;
  statusLines: string[];
  downloadAudit: string;
  objectPath: string;
  sha256: string;
};

export type AlertDetailSessionTimelineItem = {
  time: string;
  label: string;
};

export type AlertDetailSessionEvidence = {
  sessionId: string;
  tupleLines: string[];
  summaryLines: string[];
  bytes: string;
  duration: string;
  status: string;
  actionKind: 'reload' | 'file';
  timeline: AlertDetailSessionTimelineItem[];
  linkedPcap: string;
};

export type AlertDetailGraphPathNode = {
  id: string;
  label: string;
  value: string;
  kind: 'external' | 'gateway' | 'server' | 'account';
};

export type AlertDetailGraphPathEdge = {
  from: string;
  to: string;
  label: string;
};

export type AlertDetailGraphPathEvidence = {
  pathFile: string;
  pathSummary: string;
  edgeWeight: string;
  relationType: string;
  relatedEntities: string[];
  generatedAt: string;
  status: string;
  riskScore: number;
  nodes: AlertDetailGraphPathNode[];
  edges: AlertDetailGraphPathEdge[];
  resources: string[];
};

export type AlertDetailHighlightedField = {
  key: string;
  value: string;
};

export type AlertDetailLogTag = {
  label: string;
  kind: 'device' | 'rule' | 'user';
};

export type AlertDetailLogEvidence = {
  logFile: string;
  source: string;
  hitFields: string[];
  contentSummary: string;
  generatedAt: string;
  status: string;
  highlightedFields: AlertDetailHighlightedField[];
  sourceTags: AlertDetailLogTag[];
};

export type AlertDetailSnapshot = {
  alertId: string;
  title: string;
  severity: string;
  score: number;
  confidence: string;
  status: string;
  stateVersion?: number;
  assignee: string;
  ruleModel: string;
  attackPhase: string;
  firstSeen: string;
  businessSystem: string;
  recommendation: string;
  tags: string[];
  metrics: AlertDetailMetric[];
  assets: AlertDetailAsset[];
  stageTrail: AlertDetailTimelineItem[];
  timeline: AlertDetailTimelineItem[];
  evidenceRows: AlertDetailEvidenceRow[];
  responseActions: Array<{ label: string; risk: string; status: 'ok' | 'warn' | 'risk' | 'info' }>;
  feedback: {
    defaultResult: 'tp' | 'fp' | 'pending';
    reason: string;
    whitelistDraft: string;
    sampleReturn: string;
  };
  evidence: AlertDetailMetric[];
};

export type UpdateAlertStatusResult = {
  alertId: string;
  oldStatus: string;
  newStatus: string;
  reason: string;
  stateVersion?: number;
};

export type AssignAlertResult = {
  alertId: string;
  assignee: string;
  status: string;
};

export type CloseAlertResult = {
  alertId: string;
  status: string;
  reason: string;
};

export type ReopenAlertResult = {
  alertId: string;
  status: string;
};

export type AlertFeedbackLabel = 'TP' | 'FP';

export type AlertFeedbackInput = {
  label: AlertFeedbackLabel;
  reasonCode?: string;
  comment?: string;
  addToWhitelist?: boolean;
};

export type AlertFeedbackResult = {
  feedbackId: string;
  alertId: string;
  label: AlertFeedbackLabel;
  reasonCode: string;
  comment: string;
  addToWhitelist: boolean;
  whitelistDraft?: {
    id: string;
    type: string;
    value: string;
    reason: string;
    status: string;
    sourceAlertId: string;
    url: string;
  };
};

export const buildAssignAlertRequest = (assignee: string) => ({
  assignee: assignee.trim(),
});

export const buildCloseAlertRequest = (reason: string) => ({
  reason: reason.trim(),
});

export const buildUpdateAlertStatusRequest = (status: string, reason: string, stateVersion?: number) => ({
  status,
  reason: reason.trim(),
  ...(isPositiveStateVersion(stateVersion) ? { state_version: Math.trunc(stateVersion) } : {}),
});

export const buildAlertFeedbackRequest = (input: AlertFeedbackInput) => ({
  label: input.label,
  reason_code: input.label === 'FP' ? (input.reasonCode ?? '').trim() : '',
  comment: (input.comment ?? '').trim(),
  add_to_whitelist: input.label === 'FP' && Boolean(input.addToWhitelist),
});

type AlertFeedbackRequestPayload = ReturnType<typeof buildAlertFeedbackRequest>;

export async function fetchAlertDetailSnapshot(alertId: string): Promise<AlertDetailSnapshot> {
  const normalizedId = alertId || 'AL-20260620-000123';
  if (isVisualBreakdownMode()) return buildMockAlertDetailSnapshot('AL-20260620-000123');
  if (appConfig.useMock || !appConfig.enableAlertDetailApi) return buildMockAlertDetailSnapshot(normalizedId);

  const [alertResponse, evidenceResponse, feedbackResponse] = await Promise.all([
    api.get(`/v1/alerts/${encodeURIComponent(normalizedId)}`),
    api
      .get(`/v1/alerts/${encodeURIComponent(normalizedId)}/evidence`)
      .then((response) => response.data)
      .catch((error: unknown) => ({ secondary_error: normalizeError(error) })),
    api
      .get(`/v1/alerts/${encodeURIComponent(normalizedId)}/feedback`)
      .then((response) => response.data)
      .catch((error: unknown) => ({ secondary_error: normalizeError(error) })),
  ]);

  return normalizeAlertDetailSnapshot(normalizedId, alertResponse.data, evidenceResponse, feedbackResponse);
}

export async function updateAlertStatus(alertId: string, status: string, reason: string, stateVersion?: number): Promise<UpdateAlertStatusResult> {
  const normalizedId = alertId || 'AL-20260620-000123';
  const request = buildUpdateAlertStatusRequest(status, reason, stateVersion);
  if (appConfig.useMock) {
    return {
      alertId: normalizedId,
      oldStatus: 'triage',
      newStatus: status,
      reason: request.reason,
      stateVersion: request.state_version,
    };
  }

  const response = await api.put(`/v1/alerts/${encodeURIComponent(normalizedId)}/status`, request);
  const payload = unwrapPayload(response.data);
  return {
    alertId: textFrom(payload, ['alert_id', 'alertId']) || normalizedId,
    oldStatus: textFrom(payload, ['old_status', 'oldStatus']),
    newStatus: textFrom(payload, ['new_status', 'newStatus']) || status,
    reason: textFrom(payload, ['reason']) || request.reason,
    stateVersion: stateVersionFrom(valueAt(payload, ['state_version', 'stateVersion'])),
  };
}

export async function assignAlert(alertId: string, assignee: string): Promise<AssignAlertResult> {
  const normalizedId = alertId || 'AL-20260620-000123';
  const request = buildAssignAlertRequest(assignee);
  if (appConfig.useMock) {
    return {
      alertId: normalizedId,
      assignee: request.assignee,
      status: 'assigned',
    };
  }

  const response = await api.put(`/v1/alerts/${encodeURIComponent(normalizedId)}/assign`, request);
  const payload = unwrapPayload(response.data);
  return {
    alertId: textFrom(payload, ['alert_id', 'alertId']) || normalizedId,
    assignee: textFrom(payload, ['assignee']) || request.assignee,
    status: textFrom(payload, ['status']) || 'assigned',
  };
}

export async function closeAlert(alertId: string, reason: string): Promise<CloseAlertResult> {
  const normalizedId = alertId || 'AL-20260620-000123';
  const request = buildCloseAlertRequest(reason);
  if (appConfig.useMock) {
    return {
      alertId: normalizedId,
      status: 'closed',
      reason: request.reason,
    };
  }

  const response = await api.post(`/v1/alerts/${encodeURIComponent(normalizedId)}/close`, request);
  const payload = unwrapPayload(response.data);
  return {
    alertId: textFrom(payload, ['alert_id', 'alertId']) || normalizedId,
    status: textFrom(payload, ['status']) || 'closed',
    reason: textFrom(payload, ['reason']) || request.reason,
  };
}

export async function reopenAlert(alertId: string): Promise<ReopenAlertResult> {
  const normalizedId = alertId || 'AL-20260620-000123';
  if (appConfig.useMock) {
    return {
      alertId: normalizedId,
      status: 'new',
    };
  }

  const response = await api.post(`/v1/alerts/${encodeURIComponent(normalizedId)}/reopen`);
  const payload = unwrapPayload(response.data);
  return {
    alertId: textFrom(payload, ['alert_id', 'alertId']) || normalizedId,
    status: textFrom(payload, ['status']) || 'new',
  };
}

export async function submitAlertFeedback(alertId: string, input: AlertFeedbackInput): Promise<AlertFeedbackResult> {
  const normalizedId = alertId || 'AL-20260620-000123';
  const request = buildAlertFeedbackRequest(input);
  if (appConfig.useMock) {
    return {
      feedbackId: 'mock-feedback',
      alertId: normalizedId,
      label: request.label as AlertFeedbackLabel,
      reasonCode: request.reason_code,
      comment: request.comment,
      addToWhitelist: request.add_to_whitelist,
      whitelistDraft: request.add_to_whitelist
        ? {
            id: 'mock-whitelist-draft',
            type: 'ip',
            value: '172.16.5.10',
            reason: request.reason_code,
            status: 'draft',
            sourceAlertId: normalizedId,
            url: `/whitelist?source_alert=${encodeURIComponent(normalizedId)}&draft_id=mock-whitelist-draft`,
          }
        : undefined,
    };
  }

  const response = await api.post(`/v1/alerts/${encodeURIComponent(normalizedId)}/feedback`, request);
  return normalizeAlertFeedbackResult(normalizedId, response.data, request);
}

export function normalizeAlertFeedbackResult(
  normalizedId: string,
  payloadData: unknown,
  request: AlertFeedbackRequestPayload,
): AlertFeedbackResult {
  const payload = unwrapPayload(payloadData);
  const draft = valueAt(payload, ['whitelist_draft', 'whitelistDraft']);
  const draftRecord = isRecord(draft) ? draft : {};
  const draftId = textFrom(draftRecord, ['id', 'whitelist_id', 'whitelistId']);
  return {
    feedbackId: textFrom(payload, ['feedback_id', 'feedbackId']) || '',
    alertId: textFrom(payload, ['alert_id', 'alertId']) || normalizedId,
    label: (textFrom(payload, ['label']) || request.label) as AlertFeedbackLabel,
    reasonCode: textFrom(payload, ['reason_code', 'reasonCode']) || request.reason_code,
    comment: textFrom(payload, ['comment']) || request.comment,
    addToWhitelist: booleanFrom(valueAt(payload, ['add_to_whitelist', 'addToWhitelist'])) || request.add_to_whitelist,
    whitelistDraft: draftId
      ? {
          id: draftId,
          type: textFrom(draftRecord, ['type']),
          value: textFrom(draftRecord, ['value']),
          reason: textFrom(draftRecord, ['reason']),
          status: textFrom(draftRecord, ['status']) || 'draft',
          sourceAlertId: textFrom(draftRecord, ['source_alert_id', 'sourceAlertId', 'alert_id']) || normalizedId,
          url:
            textFrom(draftRecord, ['url']) ||
            `/whitelist?source_alert=${encodeURIComponent(normalizedId)}&draft_id=${encodeURIComponent(draftId)}`,
        }
      : undefined,
  };
}

export function normalizeAlertDetailSnapshot(
  requestedAlertId: string,
  alertPayload: unknown,
  evidencePayload: unknown,
  feedbackPayload: unknown,
): AlertDetailSnapshot {
  const alert = unwrapPayload(alertPayload);
  const evidenceRows = extractList(evidencePayload, ['evidences', 'evidence', 'items', 'data']);
  const feedback = unwrapPayload(feedbackPayload);
  const alertRecord = isRecord(alert) ? alert : {};
  const evidenceList = evidenceRows.length ? evidenceRows : derivedEvidence(alertRecord, requestedAlertId);
  const score = normalizeScore(numberAt(alertRecord, ['score', 'risk_score', 'riskScore']) || 92);
  const confidenceValue = numberAt(alertRecord, ['confidence', 'probability']);
  const alertId = textFrom(alertRecord, ['alert_id', 'alertId', 'id']) || requestedAlertId;
  const severity = severityLabel(textFrom(alertRecord, ['severity', 'risk_level', 'riskLevel']) || 'critical');
  const status = alertStatusLabel(textFrom(alertRecord, ['status']) || 'triage');
  const stateVersion =
    stateVersionFrom(valueAt(alertRecord, ['state_version', 'stateVersion', 'version'])) ??
    stateVersionFrom(valueAt(alertRecord, ['updated_ts', 'updated_at', 'updatedAt']));
  const srcIp = textFrom(alertRecord, ['src_ip', 'source_ip', 'srcIp']) || '172.16.5.10';
  const dstIp = textFrom(alertRecord, ['dst_ip', 'destination_ip', 'dstIp']) || '185.22.14.9';
  const title = alertTitle(alertRecord);
  const ruleModel = textFrom(alertRecord, ['rule_name', 'rule_version', 'model_version', 'alert_type', 'alertType']) || 'C2_Tunnel_v3';
  const firstSeen = formatDateTime(textFrom(alertRecord, ['first_seen', 'firstSeen'])) || '2026-06-20 03:42:11';
  const evidenceAvailable = !isSecondaryError(evidencePayload) && evidenceRows.length > 0;
  const feedbackAvailable = !isSecondaryError(feedbackPayload) && Object.keys(isRecord(feedback) ? feedback : {}).length > 0;
  const feedbackResult = textFrom(feedback, ['result', 'verdict', 'classification']);

  return {
    alertId,
    title,
    severity,
    score,
    confidence: confidenceValue ? confidenceValue.toFixed(confidenceValue > 1 ? 0 : 2) : '0.98',
    status,
    stateVersion,
    assignee: textFrom(alertRecord, ['assignee', 'owner']) || 'sec_analyst',
    ruleModel,
    attackPhase: attackPhaseLabel(textFrom(alertRecord, ['attack_phase', 'phase']) || ruleModel),
    firstSeen,
    businessSystem: textFrom(alertRecord, ['business_system', 'businessSystem']) || '教学区核心业务',
    recommendation: textFrom(alertRecord, ['recommendation', 'suggestion']) || '隔离受控主机并阻断 C2 通信',
    tags: stringListFrom(valueAt(alertRecord, ['labels', 'tags'])).length
      ? stringListFrom(valueAt(alertRecord, ['labels', 'tags'])).slice(0, 4)
      : ['C2通信', '横向移动', '可疑外联'],
    metrics: [
      metric('风险评分', `${score}/100`, severity, score >= 85 ? 'risk' : 'warn'),
      metric('置信度', confidenceValue ? (confidenceValue > 1 ? `${confidenceValue}%` : confidenceValue.toFixed(2)) : '0.98', ruleModel, 'ok'),
      metric('影响主机', '2 台', `${srcIp} -> ${dstIp}`, 'warn'),
      metric('证据链', `${evidenceList.length} 项`, evidenceAvailable ? 'API' : '默认证据视图', evidenceAvailable ? 'ok' : 'warn'),
      metric('处置动作', '5 项', '需写审计', 'info'),
      metric('反馈状态', feedbackResultLabel(feedbackResult), feedbackAvailable ? '已读取' : '待提交', feedbackAvailable ? 'ok' : 'warn'),
    ],
    assets: [
      {
        title: '源端资产',
        role: '受控主机',
        ip: srcIp,
        hostname: textFrom(alertRecord, ['src_hostname', 'source_hostname']) || '办公室-WS-1024',
        service: textFrom(alertRecord, ['src_service']) || 'Windows 10 22H2',
        business: textFrom(alertRecord, ['src_business']) || '办公区',
        risk: '异常流量 / 可疑外联',
        facts: [
          { label: 'IP 地址', value: srcIp },
          { label: 'MAC 地址', value: textFrom(alertRecord, ['src_mac', 'source_mac']) || '00:0c:29:3a:7c:1d' },
          { label: '操作系统', value: textFrom(alertRecord, ['src_os', 'source_os']) || 'Windows 10 22H2' },
          { label: '所属部门', value: textFrom(alertRecord, ['src_department', 'department']) || '办公区' },
          { label: '最近风险画像', value: '异常流量 / 可疑外联' },
        ],
      },
      {
        title: '目的端资产（外部）',
        role: 'C2 节点',
        ip: dstIp,
        hostname: textFrom(alertRecord, ['dst_hostname', 'destination_hostname']) || 'Example GmbH',
        service: textFrom(alertRecord, ['dst_service']) || 'TLS/443',
        business: textFrom(alertRecord, ['dst_geo']) || '德国 法兰克福',
        risk: '攻击基础设施',
        facts: [
          { label: 'IP 地址', value: dstIp },
          { label: '地理位置', value: textFrom(alertRecord, ['dst_geo']) || '德国 法兰克福' },
          { label: 'ASN', value: textFrom(alertRecord, ['dst_asn']) || 'AS132203' },
          { label: '所属组织', value: textFrom(alertRecord, ['dst_org', 'destination_org']) || 'Example GmbH' },
          { label: '最近风险画像', value: 'C2节点 / 攻击基础设施' },
        ],
      },
    ],
    stageTrail: [
      timelineItem('03:41:02', '初始访问', '可疑外联建立', 'info'),
      timelineItem('03:42:18', '异常行为', '规则命中', 'ok'),
      timelineItem('03:43:02', '横向移动', 'SMB 扫描迹象', 'warn'),
      timelineItem('03:43:47', 'C2 连接', '高危通信确认', 'risk'),
      timelineItem('03:44:12', '凭证利用', '待进一步验证', 'info'),
    ],
    timeline: [
      timelineItem(firstSeen.slice(11, 19) || '03:42:11', '首次发生', `检测到主机 ${srcIp} 向 ${dstIp} 发起异常外联`, 'info'),
      timelineItem('03:42:18', '规则命中', `命中规则/模型 ${ruleModel}`, 'warn'),
      timelineItem('03:43:02', '证据生成', `生成 ${evidenceList.length} 项证据，含 PCAP / Session / 日志`, 'ok'),
      timelineItem('03:43:47', '横向移动', '检测到内网扫描与会话探测行为', 'risk'),
      timelineItem('03:44:12', '处置动作', '已生成隔离与阻断建议，等待确认执行', 'info'),
    ],
    evidenceRows: evidenceList.slice(0, 6).map((item, index) => evidenceRow(item, alertId, index)),
    responseActions: [
      { label: '隔离主机', risk: '高危', status: 'risk' },
      { label: '阻断 IP', risk: '高危', status: 'risk' },
      { label: '封禁账户', risk: '中危', status: 'warn' },
      { label: '下发脚本', risk: '低危', status: 'info' },
      { label: '创建工单', risk: '低危', status: 'info' },
    ],
    feedback: {
      defaultResult: feedbackResult === 'fp' || feedbackResult === 'false_positive' ? 'fp' : feedbackResult === 'pending' ? 'pending' : 'tp',
      reason: textFrom(feedback, ['reason', 'false_positive_reason']) || '',
      whitelistDraft: textFrom(feedback, ['whitelist_draft', 'whitelist']) || `${srcIp} / ${dstIp}`,
      sampleReturn: textFrom(feedback, ['sample_return', 'mlops_sample']) || '回流至 MLOps',
    },
    evidence: [
      metric('Alert Detail API', `/v1/alerts/${alertId}`, 'primary', 'ok'),
      metric('Evidence API', evidenceAvailable ? `${evidenceRows.length} rows` : secondaryErrorText(evidencePayload) || '待返回', 'secondary', evidenceAvailable ? 'ok' : 'warn'),
      metric('Feedback API', feedbackAvailable ? '已读取' : secondaryErrorText(feedbackPayload) || '待提交', 'secondary', feedbackAvailable ? 'ok' : 'info'),
      metric('审计提示', '危险动作需留痕', 'audit_logs', 'info'),
    ],
  };
}

function buildMockAlertDetailSnapshot(alertId: string): AlertDetailSnapshot {
  return normalizeAlertDetailSnapshot(
    alertId,
    {
      data: {
        alert_id: alertId,
        alert_type: '疑似 C2 隧道通信',
        severity: 'critical',
        score: 92,
        confidence: 0.98,
        status: 'triage',
        assignee: 'sec_analyst',
        src_ip: '172.16.5.10',
        dst_ip: '185.22.14.9',
        rule_version: 'C2_Tunnel_v3',
        first_seen: '2026-06-20 03:42:11',
        business_system: '教学区核心业务',
        labels: ['C2通信', '横向移动', '可疑外联'],
      },
    },
    {
      evidences: [
        {
          type: 'PCAP',
          evidence_id: 'AL-20260620-000123.pcap',
          summary: 'PCAP 切片，TLS over HTTP 隧道，疑似隧道通信',
          size: '24.8 MB',
          timestamp: '2026-06-20 03:43:05',
          status: 'generated',
          pcap_evidence: {
            file_name: 'AL-20260620-000123.pcap',
            content_summary: 'PCAP 切片，TLS over HTTP 隧道，疑似隧道通信',
            size: '24.8 MB',
            generated_at: '2026-06-20 03:43:05',
            status_lines: ['已生成 /', 'SHA256通过'],
            download_audit: 'sec_analyst 03:44 下载',
            object_path: 'minio://traffic-evidence/alerts/2026/06/20/AL-20260620-000123.pcap',
            sha256: '1a2b3c4d5bef79a8h9i0j...',
          },
        },
        {
          type: 'Session',
          evidence_id: 'session-20260620-000123.json',
          summary: '异常长连接，双向持续传输，SNI 缺失',
          size: '1.2 MB',
          timestamp: '2026-06-20 03:43:05',
          status: 'generated',
          session_evidence: {
            session_id: 'session-20260620-000123.json',
            tuple_lines: ['172.16.5.10:443 ->', '185.22.14.9:8443 / TCP'],
            summary_lines: ['异常长连接，双向持续传输，', 'SNI 缺失'],
            bytes: '1.2 MB',
            duration: '12m 38s',
            status_label: '已生成',
            action_kind: 'reload',
            timeline: [
              { time: '03:31', label: '建连' },
              { time: '03:34', label: '心跳' },
              { time: '03:43', label: '切片关联' },
            ],
            linked_pcap: 'AL-20260620-000123.pcap',
          },
        },
        {
          type: 'Session',
          evidence_id: 'session-20260620-000124.json',
          summary: '周期心跳，每 30s 上行小包',
          size: '768 KB',
          timestamp: '2026-06-20 03:43:06',
          status: 'generated',
          session_evidence: {
            session_id: 'session-20260620-000124.json',
            tuple_lines: ['10.20.4.18:51514 ->', '185.22.14.9:443 / TCP'],
            summary_lines: ['周期心跳，每 30s 上行小包'],
            bytes: '768 KB',
            duration: '08m 16s',
            status_label: '已生成',
            action_kind: 'file',
            linked_pcap: 'AL-20260620-000123.pcap',
          },
        },
        {
          type: '日志',
          evidence_id: 'ids-20260620-000123.log',
          summary: '设备日志与规则命中日志，命中 C2_Tunnel_v3',
          size: '183 KB',
          timestamp: '2026-06-20 03:43:05',
          status: 'generated',
          log_evidence: {
            log_file: 'ids-20260620-000123.log',
            source: 'IDS / 探针-07',
            hit_fields: ['rule=C2_Tunnel_v3,', 'ja3_score=0.91'],
            content_summary: '设备日志与规则命中日志，命中 C2_Tunnel_v3',
            generated_at: '2026-06-20 03:43:05',
            status: '已生成',
            highlighted_fields: [
              { key: 'dst_ip', value: '185.22.14.9' },
              { key: 'sni', value: 'null' },
              { key: 'bytes_out_p95', value: '5.8MB' },
              { key: 'user_event', value: 'svc_backup login' },
            ],
            source_tags: [
              { label: '设备日志', kind: 'device' },
              { label: '规则命中', kind: 'rule' },
              { label: '用户事件', kind: 'user' },
            ],
          },
        },
        {
          type: '图谱路径',
          evidence_id: 'path-20260620-000123.json',
          summary: '172.16.5.10 -> 185.22.14.9 路径关系',
          size: '78 KB',
          timestamp: '2026-06-20 03:43:10',
          status: 'generated',
          graph_path: {
            path_file: 'path-20260620-000123.json',
            path_summary: '172.16.5.10 -> 185.22.14.9\n路径关系',
            edge_weight: '0.86',
            relation_type: '横向访问',
            related_entities: ['资产 DB-SRV-01', '账号 svc_backup', '域名 downloads.campus.local'],
            generated_at: '2026-06-20 03:43:10',
            status: '已生成',
            risk_score: 85,
            resources: ['PCAP 1', 'Session 2', '日志 1'],
            nodes: [
              { id: 'external-ip', label: '可疑外部IP', value: '185.22.14.9', kind: 'external' },
              { id: 'gateway', label: '边界网关', value: '10.20.0.1', kind: 'gateway' },
              { id: 'server', label: '核心业务服务器', value: '10.20.4.18', kind: 'server' },
              { id: 'account', label: '账号', value: 'svc_backup', kind: 'account' },
            ],
            edges: [
              { from: 'external-ip', to: 'gateway', label: '通信' },
              { from: 'gateway', to: 'server', label: '登录' },
              { from: 'server', to: 'account', label: '访问' },
            ],
          },
        },
        {
          type: '文件',
          evidence_id: 'hash-1a2b3c4d5bef79a8h9i0j.txt',
          summary: 'SHA256: 1a2b3c4d5bef79a8h9i0j...; signed-url 可用',
          size: '64 B',
          timestamp: '2026-06-20 03:43:04',
          status: 'calculated_accessible',
          evidence_kind: 'hash 清单 / 附件',
          hash: 'SHA256: 1a2b3c4d5bef79a8h9i0j...',
          signed_url: 'https://evidence.campus.local/signed/AL-20260620-000123',
          tags: ['报告附件', '导出脚本', 'hash 校验', '下载审计 sec_analyst 03:45'],
        },
      ],
    },
    { result: 'tp' },
  );
}

function derivedEvidence(alert: Record<string, unknown>, alertId: string): Record<string, unknown>[] {
  const ids = stringListFrom(valueAt(alert, ['evidence_ids']));
  if (ids.length) return ids.map((id) => ({ evidence_id: id, type: '证据', status: 'referenced' }));
  return [
    {
      type: 'PCAP',
      evidence_id: `${alertId}.pcap`,
      summary: 'PCAP 切片，TLS over HTTP 隧道，疑似隧道通信',
      size: '24.8 MB',
      timestamp: '2026-06-20 03:43:05',
      status: 'generated',
      pcap_evidence: {
        file_name: `${alertId}.pcap`,
        content_summary: 'PCAP 切片，TLS over HTTP 隧道，疑似隧道通信',
        size: '24.8 MB',
        generated_at: '2026-06-20 03:43:05',
        status_lines: ['已生成 /', 'SHA256通过'],
        download_audit: 'sec_analyst 03:44 下载',
        object_path: `minio://traffic-evidence/alerts/2026/06/20/${alertId}.pcap`,
        sha256: '1a2b3c4d5bef79a8h9i0j...',
      },
    },
    {
      type: 'Session',
      evidence_id: 'session-20260620-000123.json',
      summary: '异常长连接，双向持续传输，SNI 缺失',
      size: '1.2 MB',
      timestamp: '2026-06-20 03:43:05',
      status: 'generated',
      session_evidence: {
        session_id: 'session-20260620-000123.json',
        tuple_lines: ['172.16.5.10:443 ->', '185.22.14.9:8443 / TCP'],
        summary_lines: ['异常长连接，双向持续传输，', 'SNI 缺失'],
        bytes: '1.2 MB',
        duration: '12m 38s',
        status_label: '已生成',
        action_kind: 'reload',
        timeline: [
          { time: '03:31', label: '建连' },
          { time: '03:34', label: '心跳' },
          { time: '03:43', label: '切片关联' },
        ],
        linked_pcap: 'AL-20260620-000123.pcap',
      },
    },
    {
      type: 'Session',
      evidence_id: 'session-20260620-000124.json',
      summary: '周期心跳，每 30s 上行小包',
      size: '768 KB',
      timestamp: '2026-06-20 03:43:06',
      status: 'generated',
      session_evidence: {
        session_id: 'session-20260620-000124.json',
        tuple_lines: ['10.20.4.18:51514 ->', '185.22.14.9:443 / TCP'],
        summary_lines: ['周期心跳，每 30s 上行小包'],
        bytes: '768 KB',
        duration: '08m 16s',
        status_label: '已生成',
        action_kind: 'file',
        linked_pcap: 'AL-20260620-000123.pcap',
      },
    },
    {
      type: '日志',
      evidence_id: `ids-${alertId}.log`,
      summary: '设备日志与规则命中日志，命中 C2_Tunnel_v3',
      size: '-',
      timestamp: '2026-06-20 03:43:05',
      status: 'generated',
      log_evidence: {
        log_file: 'ids-20260620-000123.log',
        source: 'IDS / 探针-07',
        hit_fields: ['rule=C2_Tunnel_v3,', 'ja3_score=0.91'],
        content_summary: '设备日志与规则命中日志，命中 C2_Tunnel_v3',
        generated_at: '2026-06-20 03:43:05',
        status: '已生成',
        highlighted_fields: [
          { key: 'dst_ip', value: '185.22.14.9' },
          { key: 'sni', value: 'null' },
          { key: 'bytes_out_p95', value: '5.8MB' },
          { key: 'user_event', value: 'svc_backup login' },
        ],
        source_tags: [
          { label: '设备日志', kind: 'device' },
          { label: '规则命中', kind: 'rule' },
          { label: '用户事件', kind: 'user' },
        ],
      },
    },
    {
      type: '图谱路径',
      evidence_id: `path-${alertId}.json`,
      summary: '等待图谱路径证据',
      size: '-',
      status: 'pending',
      graph_path: {
        path_file: `path-${alertId}.json`,
        path_summary: '172.16.5.10 -> 185.22.14.9\n路径关系',
        edge_weight: '0.86',
        relation_type: '横向访问',
        related_entities: ['资产 DB-SRV-01', '账号 svc_backup', '域名 downloads.campus.local'],
        generated_at: '2026-06-20 03:43:10',
        status: '已生成',
        risk_score: 85,
        resources: ['PCAP 1', 'Session 2', '日志 1'],
      },
    },
    {
      type: '文件',
      evidence_id: `hash-${alertId}.txt`,
      summary: '等待 Hash 校验与签名 URL',
      size: '-',
      status: 'pending',
      evidence_kind: 'hash 清单 / 附件',
      signed_url: `https://evidence.campus.local/signed/${alertId}`,
    },
  ];
}

function evidenceRow(item: Record<string, unknown>, alertId: string, index: number): AlertDetailEvidenceRow {
  const type = textFrom(item, ['type', 'evidence_type']) || ['PCAP', 'Session', '日志', '图谱路径', 'Hash', '签名 URL'][index % 6];
  const id = textFrom(item, ['evidence_id', 'id', 'file_key', 'path']) || `${type}-${alertId}-${index + 1}`;
  const status = evidenceStatusLabel(textFrom(item, ['status']) || 'generated');
  const hashValue = textFrom(item, ['hash', 'sha256', 'checksum']) || (type.includes('文件') ? 'SHA256: 1a2b3c4d5bef79a8h9i0j...' : '');
  const signedUrl = textFrom(item, ['signed_url', 'signedUrl', 'url']) || (type.includes('文件') ? `https://evidence.campus.local/signed/${alertId}` : '');
  const fileTags = stringListFrom(valueAt(item, ['tags', 'labels'])).length
    ? stringListFrom(valueAt(item, ['tags', 'labels']))
    : type.includes('文件')
      ? ['报告附件', '导出脚本', 'hash 校验', '下载审计 sec_analyst 03:45']
      : [];
  const pcapEvidence = pcapEvidenceFrom(item, alertId, type, id, status);
  const sessionEvidence = sessionEvidenceFrom(item, alertId, type, id, status);
  const graphPath = graphPathFrom(item, alertId, type, id, status);
  const logEvidence = logEvidenceFrom(item, alertId, type, id, status);
  return {
    证据类型: type,
    文件记录: id,
    内容摘要: textFrom(item, ['summary', 'description']) || `${type} 证据已关联告警上下文`,
    大小: textFrom(item, ['size', 'bytes']) || '-',
    生成时间: formatDateTime(textFrom(item, ['timestamp', 'created_at', 'generated_at'])) || '2026-06-20 03:43:05',
    状态: status,
    操作: status === '待生成' ? '等待' : '下载 / 查看',
    evidenceKind: textFrom(item, ['evidence_kind', 'evidenceKind', 'kind']) || (type.includes('文件') ? 'hash 清单 / 附件' : type),
    hashValue,
    signedUrl,
    fileTags,
    pcapEvidence,
    sessionEvidence,
    graphPath,
    logEvidence,
  };
}

function pcapEvidenceFrom(
  item: Record<string, unknown>,
  alertId: string,
  type: string,
  evidenceId: string,
  status: string,
): AlertDetailPcapEvidence | undefined {
  const sourceValue = valueAt(item, ['pcap_evidence', 'pcapEvidence', 'pcap']);
  const source = isRecord(sourceValue) ? sourceValue : item;
  const typeText = `${type} ${evidenceId}`.toLowerCase();
  if (!typeText.includes('pcap')) return undefined;
  const generatedAt =
    formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) ||
    '2026-06-20 03:43:05';
  const statusLines = stringListFrom(valueAt(source, ['status_lines', 'statusLines', 'check_status', 'checkStatus']));
  const fileName = textFrom(source, ['file_name', 'fileName', 'evidence_id', 'id']) || evidenceId || `${alertId}.pcap`;
  return {
    fileName,
    contentSummary:
      textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description']) ||
      'PCAP 切片，TLS over HTTP 隧道，疑似隧道通信',
    size: textFrom(source, ['size', 'bytes']) || '24.8 MB',
    generatedAt,
    statusLines: statusLines.length ? statusLines : [`${status || '已生成'} /`, 'SHA256通过'],
    downloadAudit: textFrom(source, ['download_audit', 'downloadAudit', 'audit']) || 'sec_analyst 03:44 下载',
    objectPath:
      textFrom(source, ['object_path', 'objectPath', 'minio_path', 'minioPath', 'path']) ||
      `minio://traffic-evidence/alerts/2026/06/20/${fileName}`,
    sha256: textFrom(source, ['sha256', 'hash', 'checksum']) || '1a2b3c4d5bef79a8h9i0j...',
  };
}

function sessionEvidenceFrom(
  item: Record<string, unknown>,
  alertId: string,
  type: string,
  evidenceId: string,
  status: string,
): AlertDetailSessionEvidence | undefined {
  const sourceValue = valueAt(item, ['session_evidence', 'sessionEvidence', 'session']);
  const source = isRecord(sourceValue) ? sourceValue : item;
  const typeText = `${type} ${evidenceId}`.toLowerCase();
  if (!typeText.includes('session')) return undefined;
  const timeline = recordsFrom(valueAt(source, ['timeline', 'events', 'session_timeline', 'sessionTimeline'])).map((event, index) => ({
    time: textFrom(event, ['time', 'at']) || ['03:31', '03:34', '03:43'][index] || '',
    label: textFrom(event, ['label', 'title']) || ['建连', '心跳', '切片关联'][index] || '事件',
  }));
  const tupleLines = stringListFrom(valueAt(source, ['tuple_lines', 'tupleLines', 'five_tuple', 'fiveTuple']));
  const summaryLines = stringListFrom(valueAt(source, ['summary_lines', 'summaryLines']));
  const fallbackIndex = evidenceId.includes('000124') || evidenceId.includes('flow') ? 1 : 0;
  const fallbackRows = [
    {
      sessionId: 'session-20260620-000123.json',
      tupleLines: ['172.16.5.10:443 ->', '185.22.14.9:8443 / TCP'],
      summaryLines: ['异常长连接，双向持续传输，', 'SNI 缺失'],
      bytes: '1.2 MB',
      duration: '12m 38s',
      actionKind: 'reload' as const,
    },
    {
      sessionId: 'session-20260620-000124.json',
      tupleLines: ['10.20.4.18:51514 ->', '185.22.14.9:443 / TCP'],
      summaryLines: ['周期心跳，每 30s 上行小包'],
      bytes: '768 KB',
      duration: '08m 16s',
      actionKind: 'file' as const,
    },
  ];
  const fallback = fallbackRows[fallbackIndex];
  const actionKindText = textFrom(source, ['action_kind', 'actionKind', 'action']).toLowerCase();
  return {
    sessionId: textFrom(source, ['session_id', 'sessionId', 'evidence_id', 'id']) || evidenceId || fallback.sessionId,
    tupleLines: tupleLines.length ? tupleLines : fallback.tupleLines,
    summaryLines: summaryLines.length
      ? summaryLines
      : (textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description'])
          ? [textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description'])]
          : fallback.summaryLines),
    bytes: textFrom(source, ['bytes', 'size']) || fallback.bytes,
    duration: textFrom(source, ['duration', 'duration_text', 'durationText']) || fallback.duration,
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '已生成',
    actionKind: actionKindText.includes('file') || actionKindText.includes('doc') ? 'file' : fallback.actionKind,
    timeline: timeline.length
      ? timeline
      : [
          { time: '03:31', label: '建连' },
          { time: '03:34', label: '心跳' },
          { time: '03:43', label: '切片关联' },
        ],
    linkedPcap:
      textFrom(source, ['linked_pcap', 'linkedPcap', 'pcap', 'pcap_file', 'pcapFile']) ||
      `AL-20260620-000123.pcap`.replace('AL-20260620-000123', alertId || 'AL-20260620-000123'),
  };
}

function logEvidenceFrom(
  item: Record<string, unknown>,
  alertId: string,
  type: string,
  evidenceId: string,
  status: string,
): AlertDetailLogEvidence | undefined {
  const logSourceValue = valueAt(item, ['log_evidence', 'logEvidence', 'log_record', 'logRecord']);
  const source = isRecord(logSourceValue) ? logSourceValue : item;
  const typeText = `${type} ${evidenceId}`.toLowerCase();
  if (!type.includes('日志') && !typeText.includes('log')) return undefined;
  const highlightedFields = recordsFrom(valueAt(source, ['highlighted_fields', 'highlightedFields', 'fields'])).map((field) => ({
    key: textFrom(field, ['key', 'name']) || 'field',
    value: textFrom(field, ['value']) || '-',
  }));
  const sourceTags = recordsFrom(valueAt(source, ['source_tags', 'sourceTags', 'tags'])).map((tag, index) => ({
    label: textFrom(tag, ['label', 'name']) || ['设备日志', '规则命中', '用户事件'][index] || '日志',
    kind: logTagKind(textFrom(tag, ['kind', 'type']), index),
  }));
  return {
    logFile: textFrom(source, ['log_file', 'logFile', 'evidence_id', 'id']) || evidenceId || `ids-${alertId}.log`,
    source: textFrom(source, ['source', 'origin']) || 'IDS / 探针-07',
    hitFields: stringListFrom(valueAt(source, ['hit_fields', 'hitFields', 'match_fields', 'matchFields'])).length
      ? stringListFrom(valueAt(source, ['hit_fields', 'hitFields', 'match_fields', 'matchFields']))
      : ['rule=C2_Tunnel_v3,', 'ja3_score=0.91'],
    contentSummary:
      textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description']) ||
      '设备日志与规则命中日志，命中 C2_Tunnel_v3',
    generatedAt: formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) || '2026-06-20 03:43:05',
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '已生成',
    highlightedFields: highlightedFields.length
      ? highlightedFields
      : [
          { key: 'dst_ip', value: '185.22.14.9' },
          { key: 'sni', value: 'null' },
          { key: 'bytes_out_p95', value: '5.8MB' },
          { key: 'user_event', value: 'svc_backup login' },
        ],
    sourceTags: sourceTags.length
      ? sourceTags
      : [
          { label: '设备日志', kind: 'device' },
          { label: '规则命中', kind: 'rule' },
          { label: '用户事件', kind: 'user' },
        ],
  };
}

function logTagKind(value: string, index: number): AlertDetailLogTag['kind'] {
  const lower = value.toLowerCase();
  if (lower.includes('rule')) return 'rule';
  if (lower.includes('user') || lower.includes('account')) return 'user';
  if (lower.includes('device') || lower.includes('log')) return 'device';
  return (['device', 'rule', 'user'] as const)[index] ?? 'device';
}

function graphPathFrom(
  item: Record<string, unknown>,
  alertId: string,
  type: string,
  evidenceId: string,
  status: string,
): AlertDetailGraphPathEvidence | undefined {
  const graphSourceValue = valueAt(item, ['graph_path', 'graphPath', 'path_graph', 'pathGraph']);
  const source = isRecord(graphSourceValue) ? graphSourceValue : item;
  const typeText = `${type} ${evidenceId}`.toLowerCase();
  if (!type.includes('图谱') && !typeText.includes('graph') && !typeText.includes('path')) return undefined;
  const nodes = recordsFrom(valueAt(source, ['nodes', 'path_nodes', 'pathNodes'])).map((node, index) => ({
    id: textFrom(node, ['id']) || `node-${index}`,
    label: textFrom(node, ['label', 'name']) || ['可疑外部IP', '边界网关', '核心业务服务器', '账号'][index] || `节点 ${index + 1}`,
    value: textFrom(node, ['value', 'ip', 'account']) || ['185.22.14.9', '10.20.0.1', '10.20.4.18', 'svc_backup'][index] || '-',
    kind: graphNodeKind(textFrom(node, ['kind', 'type']), index),
  }));
  const edges = recordsFrom(valueAt(source, ['edges', 'path_edges', 'pathEdges'])).map((edge, index) => ({
    from: textFrom(edge, ['from', 'source']) || (nodes[index]?.id ?? `node-${index}`),
    to: textFrom(edge, ['to', 'target']) || (nodes[index + 1]?.id ?? `node-${index + 1}`),
    label: textFrom(edge, ['label', 'relation']) || ['通信', '登录', '访问'][index] || '访问',
  }));
  return {
    pathFile: textFrom(source, ['path_file', 'pathFile', 'evidence_id', 'id']) || evidenceId || `path-${alertId}.json`,
    pathSummary:
      textFrom(source, ['path_summary', 'pathSummary', 'summary', 'description']) ||
      '172.16.5.10 -> 185.22.14.9\n路径关系',
    edgeWeight: textFrom(source, ['edge_weight', 'edgeWeight', 'weight']) || '0.86',
    relationType: textFrom(source, ['relation_type', 'relationType', 'relation']) || '横向访问',
    relatedEntities: stringListFrom(valueAt(source, ['related_entities', 'relatedEntities', 'entities'])).length
      ? stringListFrom(valueAt(source, ['related_entities', 'relatedEntities', 'entities']))
      : ['资产 DB-SRV-01', '账号 svc_backup', '域名 downloads.campus.local'],
    generatedAt: formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) || '2026-06-20 03:43:10',
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '已生成',
    riskScore: normalizeScore(numberAt(source, ['risk_score', 'riskScore']) || 85),
    nodes: nodes.length
      ? nodes
      : [
          { id: 'external-ip', label: '可疑外部IP', value: '185.22.14.9', kind: 'external' },
          { id: 'gateway', label: '边界网关', value: '10.20.0.1', kind: 'gateway' },
          { id: 'server', label: '核心业务服务器', value: '10.20.4.18', kind: 'server' },
          { id: 'account', label: '账号', value: 'svc_backup', kind: 'account' },
        ],
    edges: edges.length
      ? edges
      : [
          { from: 'external-ip', to: 'gateway', label: '通信' },
          { from: 'gateway', to: 'server', label: '登录' },
          { from: 'server', to: 'account', label: '访问' },
        ],
    resources: stringListFrom(valueAt(source, ['resources', 'related_resources', 'relatedResources'])).length
      ? stringListFrom(valueAt(source, ['resources', 'related_resources', 'relatedResources']))
      : ['PCAP 1', 'Session 2', '日志 1'],
  };
}

function graphNodeKind(value: string, index: number): AlertDetailGraphPathNode['kind'] {
  const lower = value.toLowerCase();
  if (lower.includes('gateway')) return 'gateway';
  if (lower.includes('server') || lower.includes('database') || lower.includes('asset')) return 'server';
  if (lower.includes('account') || lower.includes('user')) return 'account';
  if (lower.includes('external') || lower.includes('ip')) return 'external';
  return (['external', 'gateway', 'server', 'account'] as const)[index] ?? 'external';
}

function metric(label: string, value: string, delta: string, status: AlertDetailMetric['status']): AlertDetailMetric {
  return { label, value, delta, status };
}

function timelineItem(
  time: string,
  title: string,
  description: string,
  status: AlertDetailTimelineItem['status'],
): AlertDetailTimelineItem {
  return { time, title, description, status };
}

function alertTitle(alert: Record<string, unknown>) {
  const type = textFrom(alert, ['name', 'title', 'alert_type', 'alertType']);
  if (!type) return '疑似 C2 隧道通信';
  if (type.toLowerCase().includes('c2')) return '疑似 C2 隧道通信';
  return type;
}

function attackPhaseLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('c2') || lower.includes('command')) return 'C2 连接';
  if (lower.includes('lateral')) return '横向移动';
  if (lower.includes('exfil')) return '数据外传';
  return value || 'C2 连接';
}

function severityLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('critical') || lower.includes('high') || value.includes('高')) return '高危';
  if (lower.includes('medium') || value.includes('中')) return '中危';
  if (lower.includes('low') || value.includes('低')) return '低危';
  return value || '高危';
}

function evidenceStatusLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('pending') || lower.includes('waiting')) return '待生成';
  if (lower.includes('calcul')) return '已计算';
  if (lower.includes('access')) return '可访问';
  if (lower.includes('fail')) return '失败';
  return '已生成';
}

function feedbackResultLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower === 'tp' || lower.includes('true')) return 'TP';
  if (lower === 'fp' || lower.includes('false')) return 'FP';
  if (lower.includes('pending')) return '待确认';
  return '待确认';
}

function normalizeScore(value: number) {
  if (!Number.isFinite(value)) return 92;
  if (value <= 1) return Math.round(value * 100);
  return Math.max(0, Math.min(100, Math.round(value)));
}

function isPositiveStateVersion(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0;
}

function stateVersionFrom(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return Math.trunc(value);
  if (typeof value !== 'string' || !value.trim()) return undefined;
  const trimmed = value.trim();
  if (/^\d+$/.test(trimmed)) {
    const numeric = Number(trimmed);
    return Number.isFinite(numeric) && numeric > 0 ? Math.trunc(numeric) : undefined;
  }
  const parsedTime = Date.parse(trimmed);
  return Number.isFinite(parsedTime) && parsedTime > 0 ? parsedTime : undefined;
}

function normalizeError(error: unknown) {
  if (isRecord(error) && isRecord(error.response)) {
    const status = valueAt(error.response, ['status']);
    return `HTTP ${String(status || 'error')}`;
  }
  if (error instanceof Error) return error.message;
  return String(error);
}

function secondaryErrorText(payload: unknown) {
  const data = unwrapPayload(payload);
  return textFrom(data, ['secondary_error']);
}

function isSecondaryError(payload: unknown) {
  return Boolean(secondaryErrorText(payload));
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

function booleanFrom(value: unknown) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value !== 'string') return false;
  const normalized = value.trim().toLowerCase();
  return normalized === 'true' || normalized === '1' || normalized === 'yes';
}

function numberAt(source: Record<string, unknown>, keys: string[]) {
  const value = valueAt(source, keys);
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function stringListFrom(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean);
  if (typeof value === 'string' && value.includes(',')) return value.split(',').map((item) => item.trim()).filter(Boolean);
  if (typeof value === 'string' && value) return [value];
  return [];
}

function recordsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? value.filter(isRecord) : [];
}

function formatDateTime(value: string) {
  if (!value) return '';
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
    .format(new Date(parsed))
    .replace(/\//g, '-');
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
