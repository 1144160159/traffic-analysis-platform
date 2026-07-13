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
    expect(snapshot.topAssets).toHaveLength(5);
    expect(snapshot.impactAccount.total).toBe(31);
    expect(snapshot.impactAccount.breakdown.map((item) => item.count)).toEqual([8, 14, 9]);
    expect(snapshot.impactAccount.breakdown.map((item) => item.percent)).toEqual(['25.8%', '45.2%', '29.0%']);
    expect(snapshot.impactAccount.rows).toHaveLength(5);
    expect(snapshot.impactAccount.rows[0]).toEqual({
      账号: 'svc_backup',
      账号类型: '服务账号',
      权限风险: '高危',
      登录链路: 'VPN -> DB-SRV-07',
    });
    expect(snapshot.impactBusinessSystem.total).toBe(9);
    expect(snapshot.impactBusinessSystem.breakdown.map((item) => item.count)).toEqual([3, 4, 2]);
    expect(snapshot.impactBusinessSystem.breakdown.map((item) => item.percent)).toEqual(['33.3%', '44.5%', '22.2%']);
    expect(snapshot.impactBusinessSystem.rows[0]).toEqual({
      业务系统: '科研管理系统',
      关键服务: 'DB/API',
      风险: '高危',
      恢复优先级: 'P0',
    });
    expect(snapshot.impactService.total).toBe(42);
    expect(snapshot.impactService.breakdown.map((item) => item.count)).toEqual([11, 18, 13]);
    expect(snapshot.impactService.breakdown.map((item) => item.percent)).toEqual(['26.2%', '42.9%', '30.9%']);
    expect(snapshot.impactService.rows[0]).toEqual({
      服务名称: 'PostgreSQL',
      端口协议: '5432/TCP',
      风险: '高危',
      依赖关系: '科研管理系统',
    });
    expect(snapshot.impactDepartment.total).toBe(7);
    expect(snapshot.impactDepartment.breakdown.map((item) => item.count)).toEqual([2, 3, 2]);
    expect(snapshot.impactDepartment.breakdown.map((item) => item.percent)).toEqual(['28.6%', '42.9%', '28.6%']);
    expect(snapshot.impactDepartment.rows[0]).toEqual({
      部门名称: '科研处',
      责任人: '王主任',
      风险: '高危',
      处置进度: 40,
    });
    expect(snapshot.impactCampus.total).toBe(4);
    expect(snapshot.impactCampus.breakdown.map((item) => item.count)).toEqual([1, 2, 1]);
    expect(snapshot.impactCampus.breakdown.map((item) => item.percent)).toEqual(['25.0%', '50.0%', '25.0%']);
    expect(snapshot.impactCampus.rows[0]).toEqual({
      校区楼宇: '主校区-数据中心',
      覆盖资产: 26,
      风险: '高危',
      链路: '核心链路',
    });
    expect(snapshot.evidenceChecks.map((item) => item.label)).toContain('完整度');
    expect(snapshot.evidenceSummaryRows[0].证据类型).toBe('PCAP');
    expect(snapshot.responseFlow).toHaveLength(6);
    expect(snapshot.responseActions).toHaveLength(5);
    expect(snapshot.reviewRows).toHaveLength(6);
    expect(snapshot.evidence.find((item) => item.label === 'Campaign Detail API')?.value).toBe('/v1/campaigns/APT-20260619-001');
  });

  it('keeps a usable empty-state storyboard when optional detail fields are missing', () => {
    const snapshot = normalizeCampaignDetailSnapshot('APT-EMPTY', { data: { campaign_id: 'APT-EMPTY', score: 87 } });

    expect(snapshot.campaignId).toBe('APT-EMPTY');
    expect(snapshot.riskScore).toBe(87);
    expect(snapshot.alertCount).toBe(38);
    expect(snapshot.assetCount).toBe(57);
    expect(snapshot.phases).toHaveLength(7);
    expect(snapshot.alerts).toHaveLength(5);
    expect(snapshot.impactAccount.total).toBe(31);
    expect(snapshot.impactAccount.rows[4].账号).toBe('svc_deploy');
    expect(snapshot.impactBusinessSystem.rows[4].恢复优先级).toBe('P2');
    expect(snapshot.impactService.rows[4]).toEqual({
      服务名称: 'Redis',
      端口协议: '6379/TCP',
      风险: '中危',
      依赖关系: '会话缓存',
    });
    expect(snapshot.impactDepartment.rows[4]).toEqual({
      部门名称: '图书馆',
      责任人: '运维组',
      风险: '中危',
      处置进度: 80,
    });
    expect(snapshot.impactCampus.rows[4]).toEqual({
      校区楼宇: '西校区-图书馆',
      覆盖资产: 5,
      风险: '低危',
      链路: '出口链路',
    });
    expect(snapshot.evidenceSummaryRows).toHaveLength(5);
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
