import { appConfig } from '@/config/runtime';
import { api } from '@/services/api';
import { alertStatusLabel } from '@/services/alertStatus';

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
  evidenceId?: string;
  evidenceKind?: string;
  hashValue?: string;
  signedUrl?: string;
  viewUrl?: string;
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
  riskScore?: number;
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
  score?: number;
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
  evidenceApiError?: string;
  feedbackApiError?: string;
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

function requiredAlertId(alertId: string): string {
  const normalized = alertId.trim();
  if (!normalized) throw new Error('alert_id 不能为空');
  return normalized;
}

export async function fetchAlertDetailSnapshot(alertId: string): Promise<AlertDetailSnapshot> {
  const normalizedId = requiredAlertId(alertId);
  if (appConfig.useMock) return buildMockAlertDetailSnapshot(normalizedId);
  if (!appConfig.enableAlertDetailApi) {
    throw new Error('告警详情真实 API 未启用；禁止回退到演示快照');
  }

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
  const normalizedId = requiredAlertId(alertId);
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
  const normalizedId = requiredAlertId(alertId);
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
  const normalizedId = requiredAlertId(alertId);
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
  const normalizedId = requiredAlertId(alertId);
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
  const normalizedId = requiredAlertId(alertId);
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
            value: '192.0.2.10',
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
  const evidenceList = evidenceRows;
  const score = normalizeScore(optionalNumberAt(alertRecord, ['score', 'risk_score', 'riskScore']));
  const confidenceValue = optionalNumberAt(alertRecord, ['confidence', 'probability']);
  const alertId = textFrom(alertRecord, ['alert_id', 'alertId', 'id']) || requestedAlertId;
  const severity = severityLabel(textFrom(alertRecord, ['severity', 'risk_level', 'riskLevel']));
  const status = alertStatusLabel(textFrom(alertRecord, ['status']));
  const stateVersion =
    stateVersionFrom(valueAt(alertRecord, ['state_version', 'stateVersion', 'version'])) ??
    stateVersionFrom(valueAt(alertRecord, ['updated_ts', 'updated_at', 'updatedAt']));
  const srcIp = textFrom(alertRecord, ['src_ip', 'source_ip', 'srcIp']) || '暂不可用';
  const dstIp = textFrom(alertRecord, ['dst_ip', 'destination_ip', 'dstIp']) || '暂不可用';
  const title = alertTitle(alertRecord);
  const ruleModel = textFrom(alertRecord, ['rule_name', 'rule_version', 'model_version', 'alert_type', 'alertType']) || '暂不可用';
  const firstSeen = formatDateTime(textFrom(alertRecord, ['first_seen', 'firstSeen'])) || '暂不可用';
  const evidenceAvailable = !isSecondaryError(evidencePayload) && evidenceRows.length > 0;
  const feedbackAvailable = !isSecondaryError(feedbackPayload) && Object.keys(isRecord(feedback) ? feedback : {}).length > 0;
  const feedbackResult = textFrom(feedback, ['result', 'verdict', 'classification']);
  const tags = stringListFrom(valueAt(alertRecord, ['labels', 'tags'])).slice(0, 4);
  const stageTrail = normalizeAlertTimeline(valueAt(alertRecord, ['stage_trail', 'stageTrail', 'attack_stages', 'attackStages']));
  const timeline = normalizeAlertTimeline(valueAt(alertRecord, ['timeline', 'events', 'history']));
  const responseActions = normalizeResponseActions(valueAt(alertRecord, ['response_actions', 'responseActions', 'action_catalog', 'actionCatalog']));
  const confidenceText = confidenceValue === undefined
    ? '暂不可用'
    : confidenceValue > 1
      ? `${confidenceValue}%`
      : confidenceValue.toFixed(2);

  return {
    alertId,
    title,
    severity,
    score,
    confidence: confidenceText,
    status,
    stateVersion,
    assignee: textFrom(alertRecord, ['assignee', 'owner']) || '未分配',
    ruleModel,
    attackPhase: attackPhaseLabel(textFrom(alertRecord, ['attack_phase', 'phase']) || ruleModel),
    firstSeen,
    businessSystem: textFrom(alertRecord, ['business_system', 'businessSystem']) || '暂不可用',
    recommendation: textFrom(alertRecord, ['recommendation', 'suggestion']) || '暂不可用',
    tags,
    metrics: [
      metric('风险评分', score === undefined ? '暂不可用' : `${score}/100`, severity, score === undefined ? 'info' : score >= 85 ? 'risk' : 'warn'),
      metric('置信度', confidenceText, ruleModel, confidenceValue === undefined ? 'info' : 'ok'),
      metric('影响主机', srcIp === '暂不可用' && dstIp === '暂不可用' ? '暂不可用' : `${Number(srcIp !== '暂不可用') + Number(dstIp !== '暂不可用')} 台`, `${srcIp} -> ${dstIp}`, 'warn'),
      metric('证据链', `${evidenceList.length} 项`, evidenceAvailable ? 'API' : '暂无权威证据', evidenceAvailable ? 'ok' : 'warn'),
      metric('处置动作', `${responseActions.length} 项`, responseActions.length ? '服务端目录' : '暂无权威动作', responseActions.length ? 'info' : 'warn'),
      metric('反馈状态', feedbackResultLabel(feedbackResult), feedbackAvailable ? '已读取' : '待提交', feedbackAvailable ? 'ok' : 'warn'),
    ],
    assets: [
      {
        title: '源端资产',
        role: '受控主机',
        ip: srcIp,
        hostname: textFrom(alertRecord, ['src_hostname', 'source_hostname']) || '暂不可用',
        service: textFrom(alertRecord, ['src_service']) || '暂不可用',
        business: textFrom(alertRecord, ['src_business']) || '暂不可用',
        risk: textFrom(alertRecord, ['src_risk', 'source_risk']) || '暂不可用',
        facts: [
          { label: 'IP 地址', value: srcIp },
          { label: 'MAC 地址', value: textFrom(alertRecord, ['src_mac', 'source_mac']) || '暂不可用' },
          { label: '操作系统', value: textFrom(alertRecord, ['src_os', 'source_os']) || '暂不可用' },
          { label: '所属部门', value: textFrom(alertRecord, ['src_department', 'department']) || '暂不可用' },
          { label: '最近风险画像', value: textFrom(alertRecord, ['src_risk', 'source_risk']) || '暂不可用' },
        ],
      },
      {
        title: '目的端资产（外部）',
        role: 'C2 节点',
        ip: dstIp,
        hostname: textFrom(alertRecord, ['dst_hostname', 'destination_hostname']) || '暂不可用',
        service: textFrom(alertRecord, ['dst_service']) || '暂不可用',
        business: textFrom(alertRecord, ['dst_geo']) || '暂不可用',
        risk: textFrom(alertRecord, ['dst_risk', 'destination_risk']) || '暂不可用',
        facts: [
          { label: 'IP 地址', value: dstIp },
          { label: '地理位置', value: textFrom(alertRecord, ['dst_geo']) || '暂不可用' },
          { label: 'ASN', value: textFrom(alertRecord, ['dst_asn']) || '暂不可用' },
          { label: '所属组织', value: textFrom(alertRecord, ['dst_org', 'destination_org']) || '暂不可用' },
          { label: '最近风险画像', value: textFrom(alertRecord, ['dst_risk', 'destination_risk']) || '暂不可用' },
        ],
      },
    ],
    stageTrail,
    timeline,
    evidenceRows: evidenceList.map((item) => evidenceRow(item, alertId)),
    responseActions,
    feedback: {
      defaultResult: feedbackResult === 'fp' || feedbackResult === 'false_positive' ? 'fp' : feedbackResult === 'pending' ? 'pending' : 'tp',
      reason: textFrom(feedback, ['reason', 'false_positive_reason']) || '',
      whitelistDraft: textFrom(feedback, ['whitelist_draft', 'whitelist']),
      sampleReturn: textFrom(feedback, ['sample_return', 'mlops_sample']),
    },
    evidence: [
      metric('Alert Detail API', `/v1/alerts/${alertId}`, 'primary', 'ok'),
      metric('Evidence API', evidenceAvailable ? `${evidenceRows.length} rows` : secondaryErrorText(evidencePayload) || '待返回', 'secondary', evidenceAvailable ? 'ok' : 'warn'),
      metric('Feedback API', feedbackAvailable ? '已读取' : secondaryErrorText(feedbackPayload) || '待提交', 'secondary', feedbackAvailable ? 'ok' : 'info'),
      metric('审计提示', '危险动作需留痕', 'audit_logs', 'info'),
    ],
    evidenceApiError: secondaryErrorText(evidencePayload),
    feedbackApiError: secondaryErrorText(feedbackPayload),
  };
}

export function buildMockAlertDetailSnapshot(alertId: string): AlertDetailSnapshot {
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
        src_ip: '192.0.2.10',
        dst_ip: '198.51.100.9',
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
            tuple_lines: ['192.0.2.10:443 ->', '198.51.100.9:8443 / TCP'],
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
            tuple_lines: ['192.0.2.18:51514 ->', '198.51.100.9:443 / TCP'],
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
              { key: 'dst_ip', value: '198.51.100.9' },
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
          summary: '192.0.2.10 -> 198.51.100.9 路径关系',
          size: '78 KB',
          timestamp: '2026-06-20 03:43:10',
          status: 'generated',
          graph_path: {
            path_file: 'path-20260620-000123.json',
            path_summary: '192.0.2.10 -> 198.51.100.9\n路径关系',
            edge_weight: '0.86',
            relation_type: '横向访问',
            related_entities: ['资产 DB-SRV-01', '账号 svc_backup', '域名 downloads.campus.local'],
            generated_at: '2026-06-20 03:43:10',
            status: '已生成',
            risk_score: 85,
            resources: ['PCAP 1', 'Session 2', '日志 1'],
            nodes: [
              { id: 'external-ip', label: '可疑外部IP', value: '198.51.100.9', kind: 'external' },
              { id: 'gateway', label: '边界网关', value: '192.0.2.1', kind: 'gateway' },
              { id: 'server', label: '核心业务服务器', value: '192.0.2.18', kind: 'server' },
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

function evidenceRow(item: Record<string, unknown>, alertId: string): AlertDetailEvidenceRow {
  const metrics = valueAt(item, ['metrics']);
  const snippetRef = valueAt(item, ['snippet_ref', 'snippetRef']);
  const source = {
    ...(isRecord(metrics) ? metrics : {}),
    ...(isRecord(snippetRef) ? snippetRef : {}),
    ...item,
  };
  const type = textFrom(source, ['type', 'evidence_type']) || '未知证据';
  const id = textFrom(source, ['evidence_id', 'id', 'file_key', 'path']) || '暂不可用';
  const status = evidenceStatusLabel(textFrom(source, ['status']));
  const hashValue = textFrom(source, ['hash', 'sha256', 'checksum']) || '';
  const signedUrl = textFrom(source, ['signed_url', 'signedUrl', 'url']) || '';
  const viewUrl = textFrom(source, ['redirect_url', 'redirectUrl', 'view_url', 'viewUrl']) || '';
  const fileTags = stringListFrom(valueAt(source, ['tags', 'labels'])).length
    ? stringListFrom(valueAt(source, ['tags', 'labels']))
    : [];
  const pcapEvidence = pcapEvidenceFrom(source, alertId, type, id);
  const sessionEvidence = sessionEvidenceFrom(source, alertId, type, id, status);
  const graphPath = graphPathFrom(source, alertId, type, id, status);
  const logEvidence = logEvidenceFrom(source, alertId, type, id, status);
  return {
    证据类型: type,
    文件记录: id,
    内容摘要: textFrom(source, ['summary', 'description']) || '暂不可用',
    大小: textFrom(source, ['size', 'bytes']) || '-',
    生成时间: formatDateTime(textFrom(source, ['timestamp', 'created_at', 'generated_at'])) || '-',
    状态: status,
    操作: status === '待生成' ? '等待' : status === '暂不可用' ? '不可用' : '下载 / 查看',
    evidenceId: id,
    evidenceKind: textFrom(source, ['evidence_kind', 'evidenceKind', 'kind']) || '暂不可用',
    hashValue,
    signedUrl,
    viewUrl,
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
): AlertDetailPcapEvidence | undefined {
  const sourceValue = valueAt(item, ['pcap_evidence', 'pcapEvidence', 'pcap']);
  const source = isRecord(sourceValue) ? sourceValue : item;
  const typeText = `${type} ${evidenceId}`.toLowerCase();
  if (!typeText.includes('pcap')) return undefined;
  const generatedAt = formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) || '暂不可用';
  const statusLines = stringListFrom(valueAt(source, ['status_lines', 'statusLines', 'check_status', 'checkStatus']));
  const fileName = textFrom(source, ['file_name', 'fileName', 'evidence_id', 'id']) || evidenceId || `${alertId}.pcap`;
  return {
    fileName,
    contentSummary: textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description']) || '暂不可用',
    size: textFrom(source, ['size', 'bytes']) || '暂不可用',
    generatedAt,
    statusLines,
    downloadAudit: textFrom(source, ['download_audit', 'downloadAudit', 'audit']) || '暂不可用',
    objectPath: textFrom(source, ['object_path', 'objectPath', 'minio_path', 'minioPath', 'path']),
    sha256: textFrom(source, ['sha256', 'hash', 'checksum']),
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
  const timeline = recordsFrom(valueAt(source, ['timeline', 'events', 'session_timeline', 'sessionTimeline'])).map((event) => ({
    time: textFrom(event, ['time', 'at']) || '暂不可用',
    label: textFrom(event, ['label', 'title']) || '暂不可用',
  }));
  const tupleLines = stringListFrom(valueAt(source, ['tuple_lines', 'tupleLines', 'five_tuple', 'fiveTuple']));
  const summaryLines = stringListFrom(valueAt(source, ['summary_lines', 'summaryLines']));
  const actionKindText = textFrom(source, ['action_kind', 'actionKind', 'action']).toLowerCase();
  return {
    sessionId: textFrom(source, ['session_id', 'sessionId', 'evidence_id', 'id']) || evidenceId || '暂不可用',
    tupleLines: tupleLines.length ? tupleLines : ['会话五元组暂不可用'],
    summaryLines: summaryLines.length
      ? summaryLines
      : (textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description'])
          ? [textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description'])]
          : ['Session 摘要暂不可用']),
    bytes: textFrom(source, ['bytes', 'size']) || '暂不可用',
    duration: textFrom(source, ['duration', 'duration_text', 'durationText']) || '暂不可用',
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '暂不可用',
    actionKind: actionKindText.includes('file') || actionKindText.includes('doc') ? 'file' : 'reload',
    timeline,
    linkedPcap: textFrom(source, ['linked_pcap', 'linkedPcap', 'pcap', 'pcap_file', 'pcapFile']),
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
    label: textFrom(tag, ['label', 'name']) || '暂不可用',
    kind: logTagKind(textFrom(tag, ['kind', 'type']), index),
  }));
  return {
    logFile: textFrom(source, ['log_file', 'logFile', 'evidence_id', 'id']) || evidenceId || `ids-${alertId}.log`,
    source: textFrom(source, ['source', 'origin']) || '暂不可用',
    hitFields: stringListFrom(valueAt(source, ['hit_fields', 'hitFields', 'match_fields', 'matchFields'])).length
      ? stringListFrom(valueAt(source, ['hit_fields', 'hitFields', 'match_fields', 'matchFields']))
      : [],
    contentSummary: textFrom(source, ['content_summary', 'contentSummary', 'summary', 'description']) || '暂不可用',
    generatedAt: formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) || '暂不可用',
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '暂不可用',
    highlightedFields,
    sourceTags,
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
    label: textFrom(node, ['label', 'name']) || '暂不可用',
    value: textFrom(node, ['value', 'ip', 'account']) || '暂不可用',
    kind: graphNodeKind(textFrom(node, ['kind', 'type']), index),
  }));
  const edges = recordsFrom(valueAt(source, ['edges', 'path_edges', 'pathEdges'])).map((edge, index) => ({
    from: textFrom(edge, ['from', 'source']) || (nodes[index]?.id ?? `node-${index}`),
    to: textFrom(edge, ['to', 'target']) || (nodes[index + 1]?.id ?? `node-${index + 1}`),
    label: textFrom(edge, ['label', 'relation']) || '暂不可用',
  }));
  return {
    pathFile: textFrom(source, ['path_file', 'pathFile', 'evidence_id', 'id']) || evidenceId || `path-${alertId}.json`,
    pathSummary:
      textFrom(source, ['path_summary', 'pathSummary', 'summary', 'description']) ||
      '路径关系暂不可用',
    edgeWeight: textFrom(source, ['edge_weight', 'edgeWeight', 'weight']) || '暂不可用',
    relationType: textFrom(source, ['relation_type', 'relationType', 'relation']) || '暂不可用',
    relatedEntities: stringListFrom(valueAt(source, ['related_entities', 'relatedEntities', 'entities'])).length
      ? stringListFrom(valueAt(source, ['related_entities', 'relatedEntities', 'entities']))
      : [],
    generatedAt: formatDateTime(textFrom(source, ['generated_at', 'generatedAt', 'timestamp', 'created_at'])) || '暂不可用',
    status: textFrom(source, ['status_label', 'statusLabel']) || status || '暂不可用',
    riskScore: normalizeScore(optionalNumberAt(source, ['risk_score', 'riskScore'])),
    nodes,
    edges,
    resources: stringListFrom(valueAt(source, ['resources', 'related_resources', 'relatedResources'])).length
      ? stringListFrom(valueAt(source, ['resources', 'related_resources', 'relatedResources']))
      : [],
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

function normalizeAlertTimeline(value: unknown): AlertDetailTimelineItem[] {
  return recordsFrom(value).map((item) => {
    const rawStatus = textFrom(item, ['status', 'severity', 'level']).toLowerCase();
    const status: AlertDetailTimelineItem['status'] = rawStatus.includes('critical') || rawStatus.includes('high') || rawStatus.includes('risk')
      ? 'risk'
      : rawStatus.includes('medium') || rawStatus.includes('warn')
        ? 'warn'
        : rawStatus.includes('success') || rawStatus.includes('ok')
          ? 'ok'
          : 'info';
    return timelineItem(
      formatDateTime(textFrom(item, ['time', 'timestamp', 'created_at', 'createdAt'])) || '暂不可用',
      textFrom(item, ['title', 'label', 'phase', 'name']) || '暂不可用',
      textFrom(item, ['description', 'summary', 'detail', 'message']) || '暂不可用',
      status,
    );
  });
}

function normalizeResponseActions(value: unknown): AlertDetailSnapshot['responseActions'] {
  return recordsFrom(value).flatMap((item) => {
    const label = textFrom(item, ['label', 'name', 'title']);
    if (!label) return [];
    const rawStatus = textFrom(item, ['status', 'severity', 'risk_level', 'riskLevel']).toLowerCase();
    const status: AlertDetailSnapshot['responseActions'][number]['status'] = rawStatus.includes('critical') || rawStatus.includes('high') || rawStatus.includes('risk')
      ? 'risk'
      : rawStatus.includes('medium') || rawStatus.includes('warn')
        ? 'warn'
        : rawStatus.includes('success') || rawStatus.includes('ok')
          ? 'ok'
          : 'info';
    return [{
      label,
      risk: textFrom(item, ['risk', 'risk_label', 'riskLabel', 'severity']) || '未提供',
      status,
    }];
  });
}

function alertTitle(alert: Record<string, unknown>) {
  const type = textFrom(alert, ['name', 'title', 'alert_type', 'alertType']);
  if (!type) return '暂不可用';
  if (type.toLowerCase().includes('c2')) return '疑似 C2 隧道通信';
  return type;
}

function attackPhaseLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('c2') || lower.includes('command')) return 'C2 连接';
  if (lower.includes('lateral')) return '横向移动';
  if (lower.includes('exfil')) return '数据外传';
  return value || '暂不可用';
}

function severityLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower.includes('critical') || lower.includes('high') || value.includes('高')) return '高危';
  if (lower.includes('medium') || value.includes('中')) return '中危';
  if (lower.includes('low') || value.includes('低')) return '低危';
  return value || '暂不可用';
}

function evidenceStatusLabel(value: string) {
  const lower = value.toLowerCase();
  if (!lower) return '暂不可用';
  if (lower.includes('pending') || lower.includes('waiting')) return '待生成';
  if (lower.includes('generated') || lower.includes('ready') || lower.includes('complete')) return '已生成';
  if (lower.includes('calcul')) return '已计算';
  if (lower.includes('access')) return '可访问';
  if (lower.includes('fail')) return '失败';
  return value;
}

function feedbackResultLabel(value: string) {
  const lower = value.toLowerCase();
  if (lower === 'tp' || lower.includes('true')) return 'TP';
  if (lower === 'fp' || lower.includes('false')) return 'FP';
  if (lower.includes('pending')) return '待确认';
  return '待确认';
}

function normalizeScore(value: number | undefined) {
  if (value === undefined || !Number.isFinite(value)) return undefined;
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

function optionalNumberAt(source: Record<string, unknown>, keys: string[]) {
  const value = valueAt(source, keys);
  if (value === undefined || value === null || value === '') return undefined;
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : undefined;
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
