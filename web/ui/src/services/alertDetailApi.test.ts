import { describe, expect, it } from 'vitest';
import {
  buildAssignAlertRequest,
  buildAlertFeedbackRequest,
  buildCloseAlertRequest,
  buildUpdateAlertStatusRequest,
  normalizeAlertFeedbackResult,
  normalizeAlertDetailSnapshot,
} from '@/services/alertDetailApi';

describe('alertDetailApi', () => {
  it('maps alert detail, evidence and feedback payloads into the detail workbench model', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-20260620-000123',
      {
        data: {
          alert_id: 'AL-20260620-000123',
          alert_type: 'C2 Tunnel',
          severity: 'critical',
          score: 0.92,
          confidence: 0.98,
          status: 'triage',
          state_version: 1782712345678,
          assignee: 'sec_analyst',
          src_ip: '172.16.5.10',
          dst_ip: '185.22.14.9',
          rule_version: 'C2_Tunnel_v3',
          first_seen: '2026-06-20T03:42:11Z',
          business_system: '教学区核心业务',
          labels: ['C2通信', '横向移动'],
        },
      },
      {
        evidences: [
          {
            type: 'PCAP',
            evidence_id: 'AL-20260620-000123.pcap',
            summary: '包含 TLS over HTTP 流量',
            size: '24.8 MB',
            timestamp: '2026-06-20T03:43:05Z',
            status: 'generated',
            redirect_url: '/forensics?evidence=AL-20260620-000123.pcap',
          },
          {
            type: 'Session',
            evidence_id: 'session-20260620-000123.json',
            summary: '异常长连接',
            size: '1.2 MB',
            timestamp: '2026-06-20T03:43:05Z',
            status: 'generated',
            session_evidence: {
              tuple_lines: ['172.16.5.10:443 ->', '185.22.14.9:8443 / TCP'],
            },
          },
        ],
      },
      { result: 'tp' },
    );

    expect(snapshot.alertId).toBe('AL-20260620-000123');
    expect(snapshot.title).toBe('疑似 C2 隧道通信');
    expect(snapshot.score).toBe(92);
    expect(snapshot.severity).toBe('高危');
    expect(snapshot.status).toBe('研判中');
    expect(snapshot.stateVersion).toBe(1782712345678);
    expect(snapshot.metrics.find((item) => item.label === '证据链')?.value).toBe('2 项');
    expect(snapshot.metrics.find((item) => item.label === '反馈状态')?.value).toBe('TP');
    expect(snapshot.assets[0].ip).toBe('172.16.5.10');
    expect(snapshot.assets[1].ip).toBe('185.22.14.9');
    expect(snapshot.evidenceRows[0].证据类型).toBe('PCAP');
    expect(snapshot.evidenceRows[0].状态).toBe('已生成');
    expect(snapshot.evidenceRows[0].viewUrl).toBe('/forensics?evidence=AL-20260620-000123.pcap');
    expect(snapshot.evidenceRows[1].sessionEvidence?.sessionId).toBe('session-20260620-000123.json');
    expect(snapshot.evidenceRows[1].sessionEvidence?.tupleLines).toContain('172.16.5.10:443 ->');
    expect(snapshot.evidence.map((item) => item.label)).toContain('Evidence API');
  });

  it('renders missing session tuple data as unavailable instead of a fixed business address', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-2',
      { data: { alert_id: 'AL-2', severity: 'high', score: 87 } },
      { evidences: [{ type: 'Session', evidence_id: 'session-missing-tuple', status: 'generated' }] },
      {},
    );

    expect(snapshot.assets[0].ip).toBe('暂不可用');
    expect(snapshot.assets[1].ip).toBe('暂不可用');
    expect(snapshot.evidenceRows[0].sessionEvidence?.tupleLines).toEqual(['会话五元组暂不可用']);
  });

  it('keeps an explicit zero score and renders a missing score as unavailable', () => {
    const zero = normalizeAlertDetailSnapshot('AL-ZERO', { data: { alert_id: 'AL-ZERO', score: 0 } }, {}, {});
    const missing = normalizeAlertDetailSnapshot('AL-MISSING', { data: { alert_id: 'AL-MISSING' } }, {}, {});

    expect(zero.score).toBe(0);
    expect(zero.metrics.find((item) => item.label === '风险评分')?.value).toBe('0/100');
    expect(missing.score).toBeUndefined();
    expect(missing.metrics.find((item) => item.label === '风险评分')?.value).toBe('暂不可用');
  });

  it('does not invent missing alert detail facts, timelines, actions or evidence metadata', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-MINIMAL',
      { data: { alert_id: 'AL-MINIMAL', confidence: 0 } },
      { evidences: [{ type: 'PCAP' }] },
      {},
    );

    expect(snapshot.confidence).toBe('0.00');
    expect(snapshot.title).toBe('暂不可用');
    expect(snapshot.severity).toBe('暂不可用');
    expect(snapshot.assignee).toBe('未分配');
    expect(snapshot.ruleModel).toBe('暂不可用');
    expect(snapshot.firstSeen).toBe('暂不可用');
    expect(snapshot.tags).toEqual([]);
    expect(snapshot.stageTrail).toEqual([]);
    expect(snapshot.timeline).toEqual([]);
    expect(snapshot.responseActions).toEqual([]);
    expect(snapshot.evidenceRows[0].文件记录).toBe('暂不可用');
    expect(snapshot.evidenceRows[0].状态).toBe('暂不可用');
    expect(snapshot.evidenceRows[0].pcapEvidence).toMatchObject({
      contentSummary: '暂不可用',
      size: '暂不可用',
      generatedAt: '暂不可用',
      statusLines: [],
      objectPath: '',
      sha256: '',
    });
  });

  it('renders timeline and response actions only from authoritative payload fields', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-AUTHORITY',
      {
        data: {
          alert_id: 'AL-AUTHORITY',
          timeline: [{ timestamp: '2026-08-04T01:02:03Z', title: '规则命中', description: '服务端事件', severity: 'high' }],
          response_actions: [{ name: '隔离主机', risk: '高危', status: 'high' }],
        },
      },
      {},
      {},
    );

    expect(snapshot.timeline).toEqual([
      { time: '2026-08-04 09:02:03', title: '规则命中', description: '服务端事件', status: 'risk' },
    ]);
    expect(snapshot.responseActions).toEqual([{ label: '隔离主机', risk: '高危', status: 'risk' }]);
  });

  it('keeps secondary API failures visible without failing the primary alert detail', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-1',
      { data: { alert_id: 'AL-1', severity: 'high', score: 87, src_ip: '10.0.0.1', dst_ip: '10.0.0.2' } },
      { secondary_error: 'HTTP 404' },
      { secondary_error: 'HTTP 404' },
    );

    expect(snapshot.alertId).toBe('AL-1');
    expect(snapshot.evidenceRows).toHaveLength(0);
    expect(snapshot.evidence.find((item) => item.label === 'Evidence API')?.status).toBe('warn');
    expect(snapshot.evidence.find((item) => item.label === 'Feedback API')?.value).toBe('HTTP 404');
  });

  it('trims state-machine action requests before sending them to alert-service', () => {
    expect(buildUpdateAlertStatusRequest('assigned', '  verified evidence and owner  ', 1782712345678)).toEqual({
      status: 'assigned',
      reason: 'verified evidence and owner',
      state_version: 1782712345678,
    });
    expect(buildAssignAlertRequest('  sec_analyst  ')).toEqual({ assignee: 'sec_analyst' });
    expect(buildCloseAlertRequest('  verified evidence and audit note  ')).toEqual({
      reason: 'verified evidence and audit note',
    });
  });

  it('builds canonical TP/FP feedback requests for alert-service', () => {
    expect(
      buildAlertFeedbackRequest({
        label: 'FP',
        reasonCode: ' FALSE_ALARM ',
        comment: '  business scanner  ',
        addToWhitelist: true,
      }),
    ).toEqual({
      label: 'FP',
      reason_code: 'FALSE_ALARM',
      comment: 'business scanner',
      add_to_whitelist: true,
      adjudication_state: 'ADJUDICATED',
      expected_label_revision: 0,
    });
    expect(
      buildAlertFeedbackRequest({
        label: 'TP',
        reasonCode: 'FALSE_ALARM',
        addToWhitelist: true,
      }),
    ).toEqual({
      label: 'TP',
      reason_code: '',
      comment: '',
      add_to_whitelist: false,
      adjudication_state: 'ADJUDICATED',
      expected_label_revision: 0,
    });
  });

  it('normalizes feedback response whitelist drafts into navigable links', () => {
    const request = buildAlertFeedbackRequest({
      label: 'FP',
      reasonCode: 'FALSE_ALARM',
      comment: 'scanner exception',
      addToWhitelist: true,
    });
    const result = normalizeAlertFeedbackResult(
      'AL-20260629-0001',
      {
        data: {
          feedback_id: 'FB-1',
          event_id: 'EV-2',
          prediction_id: 'PR-1',
          label_revision: 2,
          adjudication_state: 'ADJUDICATED',
          previous_event_id: 'EV-1',
          alert_id: 'AL-20260629-0001',
          label: 'FP',
          reason_code: 'FALSE_ALARM',
          comment: 'scanner exception',
          add_to_whitelist: true,
          whitelist_draft: {
            id: 'WL-1',
            type: 'ip',
            value: '10.12.4.23',
            reason: 'FALSE_ALARM',
            status: 'draft',
            source_alert_id: 'AL-20260629-0001',
          },
        },
      },
      request,
    );

    expect(result.whitelistDraft).toEqual({
      id: 'WL-1',
      type: 'ip',
      value: '10.12.4.23',
      reason: 'FALSE_ALARM',
      status: 'draft',
      sourceAlertId: 'AL-20260629-0001',
      url: '/whitelist?source_alert=AL-20260629-0001&draft_id=WL-1',
    });
    expect(result).toMatchObject({
      eventId: 'EV-2', predictionId: 'PR-1', labelRevision: 2,
      adjudicationState: 'ADJUDICATED', previousEventId: 'EV-1',
    });
  });
});
