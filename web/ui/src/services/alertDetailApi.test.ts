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
          },
          {
            type: 'Session',
            evidence_id: 'session-20260620-000123.json',
            summary: '异常长连接',
            size: '1.2 MB',
            timestamp: '2026-06-20T03:43:05Z',
            status: 'generated',
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
    expect(snapshot.evidenceRows[1].sessionEvidence?.sessionId).toBe('session-20260620-000123.json');
    expect(snapshot.evidenceRows[1].sessionEvidence?.tupleLines).toContain('172.16.5.10:443 ->');
    expect(snapshot.evidence.map((item) => item.label)).toContain('Evidence API');
  });

  it('keeps secondary API failures visible without failing the primary alert detail', () => {
    const snapshot = normalizeAlertDetailSnapshot(
      'AL-1',
      { data: { alert_id: 'AL-1', severity: 'high', score: 87, src_ip: '10.0.0.1', dst_ip: '10.0.0.2' } },
      { secondary_error: 'HTTP 404' },
      { secondary_error: 'HTTP 404' },
    );

    expect(snapshot.alertId).toBe('AL-1');
    expect(snapshot.evidenceRows).toHaveLength(6);
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
  });
});
