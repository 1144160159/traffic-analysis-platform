import { describe, expect, it } from 'vitest';
import { normalizeCampaignDetailSnapshot } from '@/services/campaignDetailApi';

describe('campaignDetailApi', () => {
  it('maps campaign detail payload into the campaign storyboard model', () => {
    const snapshot = normalizeCampaignDetailSnapshot('APT-20260619-001', {
      data: {
        campaign_id: 'APT-20260619-001',
        campaign_type: 'APT 定向窃密',
        score: 0.92,
        summary: '园区科研网络定向窃密战役',
        ts_start: 1_771_289_520_000,
        ts_end: 1_771_512_300_000,
        entities: ['科研网-SRV-021', '办公区-WS-1024', '核心区-DC-01'],
        alert_ids: ['AL-20260620-000123', 'AL-20260620-000119'],
        alerts: [
          { alert_id: 'AL-20260620-000123', alert_type: 'C2 隧道通信', severity: 'critical', last_seen: 1_771_292_540_000 },
          { alert_id: 'AL-20260620-000119', alert_type: '横向移动 SMB 探测', severity: 'high', last_seen: 1_771_303_900_000 },
        ],
        phase_data_backed: true,
        evidence_summary: [
          { key: 'alerts', label: '告警', current: 2, expected: 4, available: true },
          { key: 'packet_session', label: 'PCAP / Session', current: 6, available: true },
        ],
        status_transitions: [
          { status: 'new', changed_at: '2026-06-19T09:15:00Z', source: 'campaign' },
          { status: 'investigating', changed_at: '2026-06-19T10:02:00Z', source: 'audit_log' },
        ],
        attack_phases: ['初始访问', '执行', '持久化', '横向移动', 'C2通信', '数据外传', '处置闭环'],
        rule_ids: ['C2_Tunnel_v3', 'Data_Exfil_v1'],
        model_ids: ['APT_Campaign_Cluster_v2'],
      },
    });

    expect(snapshot.campaignId).toBe('APT-20260619-001');
    expect(snapshot.title).toBe('园区科研网络定向窃密战役');
    expect(snapshot.riskScore).toBe(92);
    expect(snapshot.alertCount).toBe(2);
    expect(snapshot.assetCount).toBe(3);
    expect(snapshot.profileFacts).toHaveLength(10);
    expect(snapshot.phases.map((item) => item.phase)).toEqual(['初始访问', '执行', '持久化', '横向移动', 'C2通信', '数据外传', '处置闭环']);
    expect(snapshot.alerts[0].告警ID).toBe('AL-20260620-000123');
    expect(snapshot.impactTabs).toHaveLength(6);
    expect(snapshot.topAssets).toHaveLength(3);
    expect(snapshot.impactAccount.total).toBe(0);
    expect(snapshot.impactAccount.rows).toEqual([]);
    expect(snapshot.impactBusinessSystem.total).toBe(0);
    expect(snapshot.impactBusinessSystem.rows).toEqual([]);
    expect(snapshot.impactService.total).toBe(0);
    expect(snapshot.impactService.rows).toEqual([]);
    expect(snapshot.impactDepartment.total).toBe(0);
    expect(snapshot.impactDepartment.rows).toEqual([]);
    expect(snapshot.impactCampus.total).toBe(0);
    expect(snapshot.impactCampus.rows).toEqual([]);
    expect(snapshot.evidenceChecks.map((item) => item.label)).toEqual(['告警', 'PCAP / Session']);
    expect(snapshot.phaseDataBacked).toBe(true);
    expect(snapshot.evidenceRail[0]).toEqual({
      key: 'alerts',
      label: '告警',
      current: 2,
      expected: 4,
      available: true,
    });
    expect(snapshot.statusTransitions[1]).toEqual({
      status: 'investigating',
      changedAt: '2026-06-19T10:02:00Z',
      source: 'audit_log',
    });
    expect(snapshot.evidenceSummaryRows[0].证据类型).toBe('告警');
    expect(snapshot.responseFlow).toHaveLength(6);
    expect(snapshot.responseFlow.map((step) => step.title)).toEqual(['发现', '研判', '遏制', '根除', '恢复', '复盘']);
    expect(snapshot.responseActions).toEqual([]);
    expect(snapshot.reviewRows).toEqual([]);
    expect(snapshot.evidence.find((item) => item.label === 'Campaign Detail API')?.value).toBe('/v1/campaigns/APT-20260619-001');
  });

  it('keeps an honest empty state when optional detail fields are missing', () => {
    const snapshot = normalizeCampaignDetailSnapshot('APT-EMPTY', { data: { campaign_id: 'APT-EMPTY', score: 87 } });

    expect(snapshot.campaignId).toBe('APT-EMPTY');
    expect(snapshot.riskScore).toBe(87);
    expect(snapshot.alertCount).toBe(0);
    expect(snapshot.assetCount).toBe(0);
    expect(snapshot.phases).toEqual([]);
    expect(snapshot.alerts).toEqual([]);
    expect(snapshot.impactAccount.total).toBe(0);
    expect(snapshot.impactAccount.rows).toEqual([]);
    expect(snapshot.impactBusinessSystem.rows).toEqual([]);
    expect(snapshot.impactService.rows).toEqual([]);
    expect(snapshot.impactDepartment.rows).toEqual([]);
    expect(snapshot.impactCampus.rows).toEqual([]);
    expect(snapshot.evidenceSummaryRows).toHaveLength(1);
    expect(snapshot.evidenceRail[0].current).toBe(0);
    expect(snapshot.evidenceRail[1].available).toBe(false);
    expect(snapshot.statusTransitions).toEqual([]);
    expect(snapshot.status).toBe('进行中');
  });

  it('maps department impact payload rows and progress percentages', () => {
    const snapshot = normalizeCampaignDetailSnapshot('APT-DEPT', {
      data: {
        campaign_id: 'APT-DEPT',
        department_count: 8,
        department_high_risk: 3,
        department_medium_risk: 4,
        department_low_risk: 1,
        impact_departments: [
          { department_name: '研究院', owner: 'chen.pi', severity: 'critical', response_progress: 0.41 },
          { dept: '数据中心', responsible_person: 'ops_lead', risk_level: 'medium', disposal_progress: 76 },
        ],
      },
    });

    expect(snapshot.impactTabs.find((item) => item.label === '部门')?.value).toBe('8 个');
    expect(snapshot.impactDepartment.breakdown.map((item) => item.count)).toEqual([3, 4, 1]);
    expect(snapshot.impactDepartment.rows).toEqual([
      { 部门名称: '研究院', 责任人: 'chen.pi', 风险: '高危', 处置进度: 41 },
      { 部门名称: '数据中心', 责任人: 'ops_lead', 风险: '中危', 处置进度: 76 },
    ]);
  });

  it('maps service impact payload rows with port protocol and dependencies', () => {
    const snapshot = normalizeCampaignDetailSnapshot('APT-SVC', {
      data: {
        campaign_id: 'APT-SVC',
        service_count: 44,
        service_high_risk: 12,
        service_medium_risk: 20,
        service_low_risk: 12,
        top_services: [
          { service_name: 'Kafka broker', port: 9092, protocol: 'tcp', severity: 'high', dependency: '事件总线' },
          { service: 'OIDC', port_protocol: '443/TCP', risk_level: 'low', business_system: '统一登录' },
        ],
      },
    });

    expect(snapshot.impactTabs.find((item) => item.label === '服务')?.value).toBe('44 个');
    expect(snapshot.impactService.breakdown.map((item) => item.count)).toEqual([12, 20, 12]);
    expect(snapshot.impactService.rows).toEqual([
      { 服务名称: 'Kafka broker', 端口协议: '9092/TCP', 风险: '高危', 依赖关系: '事件总线' },
      { 服务名称: 'OIDC', 端口协议: '443/TCP', 风险: '低危', 依赖关系: '统一登录' },
    ]);
  });
});
