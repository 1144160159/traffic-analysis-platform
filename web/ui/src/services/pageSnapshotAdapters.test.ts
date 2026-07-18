import { describe, expect, it } from 'vitest';
import { findRouteById } from '@/routes/routeManifest';
import { adaptKnownPageSnapshot } from '@/services/pageSnapshotAdapters';

describe('pageSnapshotAdapters', () => {
  it('maps dashboard stats into duty metrics and evidence rows', () => {
    const route = findRouteById('dashboard');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          alerts: { total: 128, new: 12, critical: 3, high: 9 },
          performance: { kafka_lag: 42, end_to_end_p95_ms: 1280 },
          compliance: { sla_violations: 2, pass_rate: 0.982, pending_reviews: 5 },
          fusion: { completeness: 0.91, entities_aligned: 2860 },
        },
      },
      [{ data: { trend: [{ hour: '09:00', count: 8, severity: 'high' }] } }, { data: { phases: [{ phase: '执行', count: 4 }] } }],
    );

    expect(snapshot?.metrics.find((item) => item.label === '高危未处理')?.value).toBe('12 条');
    expect(snapshot?.metrics.find((item) => item.label === '待复核')?.value).toBe('5 项');
    expect(snapshot?.rows[0]['事件 ID']).toBe('DASHBOARD-HEALTH-GATE');
    expect(snapshot?.rows[0]['业务系统']).toBe('采集分析链路');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Dashboard API');
  });

  it('maps situational screen stats into full-loop screen metrics', () => {
    const route = findRouteById('screen');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          assets: { total: 2860, buildings_total: 30, buildings_covered: 29 },
          probes: { total: 20, online: 19, degraded: 1 },
          traffic: { gbps: 78.3 },
          performance: { parser_success_rate: 0.992, kafka_lag: 640, end_to_end_p95_ms: 860 },
          alerts: { critical: 2, high: 7 },
          evidence: { coverage_rate: 0.968 },
          response: { actions_24h: 242 },
        },
      },
      [
        { data: { trend: [{ hour: '09:00', sessions: 41 }, { hour: '10:00', sessions: 52 }] } },
        { data: { phases: [{ phase: '初始访问', count: 12 }, { phase: '执行', count: 8 }] } },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '楼宇覆盖率')?.value).toBe('96.7%');
    expect(snapshot?.metrics.find((item) => item.label === '探针在线率')?.value).toBe('95.0%');
    expect(snapshot?.metrics.find((item) => item.label === '采集吞吐')?.value).toBe('78.3 Gbps');
    expect(snapshot?.metrics.find((item) => item.label === 'Kafka 积压')?.status).toBe('warn');
    expect(snapshot?.metrics.find((item) => item.label === '高危告警')?.value).toBe('9 条');
    expect(snapshot?.rows[0]['对象 ID']).toBe('SCREEN-CAPTURE');
    expect(snapshot?.rows[1]['对象 ID']).toBe('SCREEN-PIPELINE');
    expect(snapshot?.timeline.map((item) => item.title)).toContain('全流量处理链路已映射');
    expect(snapshot?.evidence.find((item) => item.label === 'Attack Phases API')?.value).toBe('2 类 / 20 次');
    expect(snapshot?.evidence.find((item) => item.label === '响应动作')?.value).toBe('242 次');
  });

  it('maps probe list payload into probe management matrix and gates', () => {
    const route = findRouteById('probes');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            probe_id: 'PROBE-DC-01',
            hostname: 'probe-dc-01',
            ip_address: '10.12.0.11',
            location: '数据中心机房 A',
            status: 'online',
            health_score: 100,
            cpu_usage: 28.7,
            memory_usage: 38.2,
            drop_rate: 0.0002,
            bandwidth_mbps: 18600,
            capture_mode: 'hybrid_l2_l3',
            interfaces: ['eth2', 'eth3'],
            parse_rate: 99.21,
            disk_usage: 56.1,
            archive_path: 's3://pcap-archive/probe-dc-01/',
            mtls_enabled: true,
            config_version: 'v3.4.7',
            last_heartbeat: 1792886700000,
          },
          {
            probe_id: 'PROBE-SPORT-01',
            hostname: 'probe-sport-01',
            ip_address: '10.12.8.51',
            location: '体育馆',
            status: 'degraded',
            health_score: 60,
            cpu_usage: 72.6,
            memory_usage: 68.7,
            drop_rate: 0.0112,
            bandwidth_mbps: 3800,
            capture_mode: 'l2',
            interfaces: ['eth2', 'eth3'],
            config_version: 'v3.4.5',
            last_heartbeat: 1792886699000,
          },
          {
            probe_id: 'PROBE-DORM-01',
            hostname: 'probe-dorm-01',
            ip_address: '10.12.9.22',
            location: '宿舍区',
            status: 'offline',
            health_score: 0,
            cpu_usage: 0,
            memory_usage: 0,
            drop_rate: 0,
            bandwidth_mbps: 0,
            capture_mode: '',
            config_version: 'v3.4.4',
            last_heartbeat: 0,
          },
        ],
        total: 25,
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '探针总数')?.value).toBe('25 台');
    expect(snapshot?.metrics.find((item) => item.label === '在线探针')?.value).toBe('2 在线');
    expect(snapshot?.metrics.find((item) => item.label === '采集网卡')?.value).toBe('4 张');
    expect(snapshot?.metrics.find((item) => item.label === '告警探针')?.value).toBe('1 台');
    expect(snapshot?.metrics.find((item) => item.label === '离线探针')?.value).toBe('1 台');
    expect(snapshot?.rows[0]['探针 ID']).toBe('PROBE-DC-01');
    expect(snapshot?.rows[0]['位置']).toBe('数据中心机房 A');
    expect(snapshot?.rows[0]['状态']).toBe('在线');
    expect(snapshot?.rows[0]['采集模式']).toBe('混合 (L2+L3)');
    expect(snapshot?.rows[0]['采集带宽']).toBe('18.6 Gbps');
    expect(snapshot?.rows[0]['采集网卡']).toBe('eth2, eth3');
    expect(snapshot?.rows[0]['归档路径']).toBe('s3://pcap-archive/probe-dc-01/');
    expect(snapshot?.rows[1]['状态']).toBe('告警');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Probes API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('mTLS');
  });

  it('maps data quality report into quality gates and topic health rows', () => {
    const route = findRouteById('data-quality');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          timestamp: '2026-06-20T03:45:00Z',
          overall: 'degraded',
          metrics: {
            flow_rate: 4200,
            session_count_1h: 1000,
            feature_count_1h: 963,
            p95_latency_ms: 1600,
            flows_raw_columns: 86,
            insert_rate_per_min: 3900,
          },
          checks: [
            { name: 'flow_rate', status: 'pass', message: 'Flow rate is healthy', value: 4200, threshold: 100 },
            { name: 'data_completeness', status: 'pass', message: 'Completeness is healthy', value: 0.963, threshold: 0.9 },
            { name: 'end_to_end_latency', status: 'warn', message: 'Latency is above target', value: 1600, threshold: 60000 },
            { name: 'schema_drift', status: 'warn', message: 'Schema columns changed', value: 86, threshold: 3 },
            { name: 'kafka_lag_proxy', status: 'pass', message: 'Kafka lag proxy is stable', value: 3900, threshold: 50 },
          ],
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '质量总分')?.value).toBe('84 分');
    expect(snapshot?.metrics.find((item) => item.label === '完整性')?.value).toBe('96.3%');
    expect(snapshot?.metrics.find((item) => item.label === 'DLQ 数量')?.value).toBe('12.8K 条');
    expect(snapshot?.rows[0].Topic).toBe('flow_original');
    expect(snapshot?.rows[0]['当前吞吐量']).toBe('4.2K msg/min');
    expect(snapshot?.rows[6].Topic).toBe('dlq.v1');
    expect(snapshot?.timeline.map((item) => item.title)).toContain('Data Quality API 已接入');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Data Quality API');
  });

  it('maps alert list payload into alert workbench columns', () => {
    const route = findRouteById('alerts');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            alert_id: 'AL-20260625-0001',
            severity: 'critical',
            alert_type: 'C2 Tunnel',
            attack_phase: '命令与控制',
            src_ip: '10.12.1.20',
            dst_ip: '185.22.14.9',
            asset_name: '办公区-WS-1024',
            rule_name: 'C2_Tunnel_v3',
            confidence: 0.98,
            first_seen: '2026-06-25T09:42:11Z',
            status: 'ALERT_STATUS_NEW',
            state_version: 1782712345678,
          },
          {
            alert_id: 'AL-20260625-0002',
            severity: 'high',
            alert_type: 'Lateral Movement',
            attack_phase: '横向移动',
            src_ip: '10.12.1.21',
            dst_ip: '10.12.8.9',
            asset_name: '核心区-SRV-08',
            rule_name: 'Lateral_Move_v2',
            confidence: 0.88,
            first_seen: '2026-06-25T09:45:11Z',
            status: 'triage',
          },
          {
            alert_id: 'AL-20260625-0003',
            severity: 'medium',
            alert_type: 'Credential Access',
            attack_phase: '凭证访问',
            src_ip: '10.12.1.22',
            dst_ip: '10.12.8.10',
            asset_name: '认证服务',
            rule_name: 'Credential_Abuse_v1',
            confidence: 0.79,
            first_seen: '2026-06-25T09:48:11Z',
            status: 'assigned',
          },
          {
            alert_id: 'AL-20260625-0004',
            severity: 'low',
            alert_type: 'Benign Scanner',
            attack_phase: '侦察',
            src_ip: '10.12.1.23',
            dst_ip: '10.12.8.11',
            asset_name: '扫描器',
            rule_name: 'Scanner_v1',
            confidence: 0.61,
            first_seen: '2026-06-25T09:51:11Z',
            status: 'false_positive',
          },
        ],
        total: 4,
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '高危')?.value).toBe('2 条');
    expect(snapshot?.metrics.find((item) => item.label === '未处理')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '研判中')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '已指派')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '已关闭')?.value).toBe('1 条');
    expect(snapshot?.rows[0]['告警 ID']).toBe('AL-20260625-0001');
    expect(snapshot?.rows[0]['受影响资产']).toBe('办公区-WS-1024');
    expect(snapshot?.rows[0]['规则/模型']).toBe('C2_Tunnel_v3');
    expect(snapshot?.rows[0]['置信度']).toBe('0.98');
    expect(snapshot?.rows[0]['状态']).toBe('未处理');
    expect(snapshot?.rows[0].__alertId).toBe('AL-20260625-0001');
    expect(snapshot?.rows[0].__stateVersion).toBe(1782712345678);
    expect(snapshot?.rows[0].__status).toBe('new');
    expect(snapshot?.rows[1]['状态']).toBe('研判中');
    expect(snapshot?.rows[2]['状态']).toBe('已指派');
    expect(snapshot?.rows[3]['状态']).toBe('已关闭');
  });

  it('maps asset list payload into asset inventory columns', () => {
    const route = findRouteById('assets');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            asset_id: 'ASSET-001',
            ip_address: '10.12.1.33',
            mac_address: '00:11:22:33:44:55',
            hostname: 'SRV-001',
            vendor: 'Huawei',
            os_type: 'Linux',
            department: '数据中心',
            criticality: 91,
            open_ports: 8,
            risk_level: 'high',
            status: 'inactive',
            last_seen: '2026-06-25T09:40:00Z',
            metadata: {
              device_role: '核心交换',
              network_interfaces: [{ name: 'GE0/1', status: 'up' }],
              business_domain: '教学教务',
              system_level: '核心',
              key_services: [{ name: '教务 API' }],
              dependency_health: [{ type: '服务器', total: 12 }],
              sla_current: '99.2%',
              suspected_type: '临时主机',
              confidence: 72,
              ticket_status: '待确认',
            },
          },
        ],
        pagination: { total: 1 },
      },
      [
        {
          data: {
            total: 1,
            active: 0,
            inactive: 1,
            unknown: 0,
            high_criticality: 1,
            unowned: 1,
            open_services: 8,
            network_interfaces: 0,
            context_records: 8,
          },
        },
        {
          data: [
            {
              run_id: 'run-snmp-lldp-001',
              status: 'completed',
              discovered_assets: 2,
              discovered_links: 1,
            },
          ],
        },
        {
          data: [
            {
              source_asset_id: 'ASSET-001',
              neighbor_asset_id: 'ASSET-EDGE-001',
              neighbor_ip: '10.12.1.1',
              protocol: 'lldp',
            },
          ],
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '分类资产总数')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '活跃资产')?.value).toBe('0 个');
    expect(snapshot?.metrics.find((item) => item.label === '暴露服务数')?.value).toBe('8 条');
    expect(snapshot?.metrics.find((item) => item.label === '高风险资产')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '未归属资产')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '分类观测记录')?.value).toBe('8 条');
    expect(snapshot?.rows[0]['资产 ID']).toBe('ASSET-001');
    expect(snapshot?.rows[0]['IP/MAC']).toContain('10.12.1.33');
    expect(snapshot?.rows[0]['主机名']).toBe('SRV-001');
    expect(snapshot?.rows[0]['操作系统']).toBe('Linux');
    expect(snapshot?.rows[0]['厂商']).toBe('Huawei');
    expect(snapshot?.rows[0]['设备角色']).toBe('核心交换');
    expect(snapshot?.rows[0]['接口数']).toBe(1);
    expect(snapshot?.rows[0]['业务域']).toBe('教学教务');
    expect(snapshot?.rows[0]['系统等级']).toBe('核心');
    expect(snapshot?.rows[0]['关键服务']).toBe(1);
    expect(snapshot?.rows[0]['依赖资产']).toBe(12);
    expect(snapshot?.rows[0]['SLA']).toBe('99.2%');
    expect(snapshot?.rows[0]['疑似类型']).toBe('临时主机');
    expect(snapshot?.rows[0]['置信度']).toBe('72%');
    expect(snapshot?.rows[0]['工单状态']).toBe('待确认');
    expect(snapshot?.rows[0].__discoveryRunId).toBe('run-snmp-lldp-001');
    expect(snapshot?.rows[0].__discoveryRunStatus).toBe('已完成');
    expect(snapshot?.rows[0].__topologyNeighborCount).toBe(1);
    expect(snapshot?.rows[0].__topologyNeighbor).toBe('10.12.1.1');
    expect(snapshot?.timeline.some((item) => item.title === '主动发现任务已接入')).toBe(true);
    expect(snapshot?.evidence.find((item) => item.label === 'LLDP 拓扑')?.value).toBe('1 条');
  });

  it('maps graph explore payload into graph metrics and path rows', () => {
    const route = findRouteById('graph');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          query_id: 'graph-query-001',
          graph: {
            nodes: [
              { ip: '10.20.4.18', session_count: 180, total_bytes: 4294967296, alert_count: 3 },
              { ip: '185.234.15.23', session_count: 96, total_bytes: 1200000000, alert_count: 1 },
            ],
            edges: [
              { source: '185.234.15.23', target: '10.20.4.18', session_count: 128, total_bytes: 860000000, direction: 'inbound', protocol: 'TCP' },
            ],
            truncated: false,
            cache_hit: true,
          },
          meta: {
            center_ip: '10.20.4.18',
            node_count: 2,
            edge_count: 1,
            duration_ms: 278,
          },
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '实体节点')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '关系边')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '告警关联')?.value).toBe('4 条');
    expect(snapshot?.rows[0]['源实体']).toBe('185.234.15.23');
    expect(snapshot?.rows[0]['目标实体']).toBe('10.20.4.18');
    expect(snapshot?.rows[0]['风险']).toBe('高危');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Graph API');
  });

  it('maps fusion stats and aligned entities into fusion workbench rows', () => {
    const route = findRouteById('fusion');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          total_events: 12846,
          entities_aligned: 2817,
          alignment_rate: 0.913,
          data_source_stats: {
            traffic: { count: 1, records_per_min: 860 },
            asset: { count: 1, records_per_min: 24 },
            log: { count: 1, records_per_min: 121 },
          },
          quality_metrics: {
            completeness: 0.947,
            accuracy: 0.913,
            freshness: 0.988,
            duplication_rate: 0.006,
          },
        },
      },
      [
        {
          data: {
            entities: [
              {
                entity_id: 'ASSET-001',
                entity_type: 'ip',
                identifiers: { asset_id: 'ASSET-001', ip: '10.12.4.12' },
                risk_score: 86,
                asset_criticality: '高',
                last_updated: 1792902000000,
              },
            ],
            total: 1,
          },
        },
        {
          data: [
            {
              type: 'ip',
              value: '185.130.5.253',
              reputation: 'c2',
              category: 'c2',
              source: 'builtin',
              description: 'Known Cobalt Strike C2',
            },
          ],
          meta: { page: { total: 1 } },
        },
        {
          data: {
            formula_version: 'fusion-value-ablation-v1',
            active_source_count: 4,
            multi_source: {
              coverage_rate: 0.92,
              confidence: 0.913,
            },
            delta: {
              lead_time_minutes: 18.4,
              false_positive_reduction_pct: 31.7,
              mttr_reduction_pct: 24.5,
            },
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '融合实体')?.value).toBe('2.8K 个');
    expect(snapshot?.metrics.find((item) => item.label === '可信度')?.value).toBe('91.3%');
    expect(snapshot?.metrics.find((item) => item.label === '来源覆盖')?.value).toBe('100.0%');
    expect(snapshot?.metrics.find((item) => item.label === '检出提前量')?.value).toBe('18.4 分钟');
    expect(snapshot?.metrics.find((item) => item.label === '误报下降')?.value).toBe('31.7%');
    expect(snapshot?.metrics.find((item) => item.label === 'MTTR 下降')?.value).toBe('24.5%');
    expect(snapshot?.metrics.find((item) => item.label === '情报命中')?.value).toBe('1 条');
    expect(snapshot?.rows[0]['对象']).toBe('185.130.5.253');
    expect(snapshot?.rows[0]['来源 A']).toBe('Threat Intel 威胁情报');
    expect(snapshot?.rows[0]['冲突字段']).toBe('C2 情报命中');
    expect(snapshot?.rows[1]['对象']).toBe('ASSET-001');
    expect(snapshot?.rows[1]['来源 B']).toBe('CMDB 资产库');
    expect(snapshot?.rows[1]['处理状态']).toBe('待确认');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Fusion Stats API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Threat Intel API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Fusion Value API');
  });

  it('maps behavior baseline payload into baseline workbench rows', () => {
    const route = findRouteById('baselines');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          baselines: [
            {
              baseline_id: 'ip_10.12.4.12',
              name: 'IP行为基线 10.12.4.12',
              entity_type: 'ip',
              entity_id: '10.12.4.12',
              baseline_type: 'dynamic',
              status: 'active',
              version: 3,
              metrics: [
                {
                  metric_name: 'bytes_per_session',
                  unit: 'bytes',
                  normal_range: [100, 500],
                  mean: 300,
                  std_dev: 20,
                  current_value: 460,
                  deviation_score: 3.6,
                  threshold_config: { warning_multiplier: 2, alert_multiplier: 3 },
                },
              ],
            },
            {
              baseline_id: 'ip_10.12.4.13',
              name: 'IP行为基线 10.12.4.13',
              entity_type: 'ip',
              entity_id: '10.12.4.13',
              baseline_type: 'dynamic',
              status: 'learning',
              metrics: [],
            },
          ],
          total: 2933709,
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '偏离资产')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '新端口')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '基线稳定度')?.value).toBe('50.0%');
    expect(snapshot?.rows[0]['对象']).toBe('IP行为基线 10.12.4.12');
    expect(snapshot?.rows[0]['基线类型']).toBe('动态基线');
    expect(snapshot?.rows[0]['偏离值']).toBe('3.6x');
    expect(snapshot?.rows[0]['状态']).toBe('稳定');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Baselines API');
  });

  it('maps campaign payload into campaign workbench rows and evidence', () => {
    const route = findRouteById('campaigns');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          campaigns: [
            {
              campaign_id: 'campaign-tenant-a1b2c3d4',
              campaign_type: 'apt',
              attack_phases: ['initial_access', 'execution', 'lateral_movement', 'exfiltration'],
              entities: ['10.12.4.12', '10.12.4.13', 'db-srv-01'],
              alerts: ['AL-001', 'AL-002', 'AL-003', 'AL-004'],
              score: 0.91,
              ts_start: 1792886400,
              ts_end: 1792972800,
              summary: 'RedLync APT lateral movement',
            },
            {
              campaign_id: 'campaign-tenant-e5f6g7h8',
              campaign_type: 'brute_force',
              attack_phases: ['reconnaissance', 'initial_access'],
              entities: ['10.12.5.20'],
              alerts: ['AL-005'],
              score: 0.56,
              ts_start: 1792886400,
              ts_end: 1792890000,
            },
          ],
          total: 2933709,
        },
      },
      [],
    );

    expect(snapshot?.total).toBe(2933709);
    expect(snapshot?.metrics.find((item) => item.label === '战役总数')?.value).toBe('2.9M 个');
    expect(snapshot?.metrics.find((item) => item.label === '当前页活跃')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '当前页活跃')?.delta).toBe('当前页 API');
    expect(snapshot?.metrics.find((item) => item.label === '当前页影响资产')?.value).toBe('4 台');
    expect(snapshot?.rows[0]['战役名称']).toBe('campaign-tenant-a1b2c3d4');
    expect(snapshot?.rows[0]['阶段']).toBe('数据外传');
    expect(snapshot?.rows[0]['风险等级']).toBe('高风险');
    expect(snapshot?.rows[0]['影响资产']).toBe(3);
    expect(snapshot?.rows[0]['告警数']).toBe(4);
    expect(snapshot?.rows[0]['首次发现']).toBe('10-25 00:00');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Campaigns API');
    expect(snapshot?.evidence.find((item) => item.label === '证据完整度')?.value).toBe('接口未提供');
  });

  it('preserves every campaign returned by the requested API page', () => {
    const route = findRouteById('campaigns');
    expect(route).toBeTruthy();
    const campaigns = Array.from({ length: 20 }, (_, index) => ({
      campaign_id: `campaign-${index + 1}`,
      attack_phases: ['execution'],
      entities: [`asset-${index + 1}`],
      alerts: [`alert-${index + 1}`],
      score: 0.5,
      ts_start: 1792886400,
      ts_end: 1792890000,
    }));

    const snapshot = adaptKnownPageSnapshot(route!.page, { data: { campaigns, total: 2933709 } }, []);

    expect(snapshot?.rows).toHaveLength(20);
    expect(snapshot?.rows[19]['战役名称']).toBe('campaign-20');
    expect(snapshot?.total).toBe(2933709);
  });

  it('maps attack chain payload into attack chain rows and evidence', () => {
    const route = findRouteById('attack-chains');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          chains: [
            {
              chain_id: 'chain-001',
              title: 'C2 隧道攻击链',
              risk_score: 91,
              root_alert_id: 'AL-20260619-001',
              source_ip: '203.0.113.45',
              entity_count: 6,
              alert_count: 12,
              status: 'active',
              mitre_techniques: ['TA0043', 'TA0001', 'TA0002'],
              phases: [
                {
                  phase: 'initial_access',
                  confidence: 0.92,
                  key_events: [
                    {
                      event_id: 'evt-001',
                      description: 'Web 漏洞利用',
                      src_ip: '203.0.113.45',
                      dst_ip: '10.12.5.23',
                      technique: 'TA0001',
                      severity: 'high',
                    },
                  ],
                },
                {
                  phase: 'command_and_control',
                  confidence: 0.88,
                  key_events: [
                    {
                      event_id: 'evt-002',
                      description: 'C2 隧道通信',
                      src_ip: '10.12.8.45',
                      dst_ip: 'c2.example.com',
                      technique: 'TA0011',
                      severity: 'high',
                    },
                  ],
                },
              ],
            },
          ],
          total: 1,
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '阶段节点')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '实体节点')?.value).toBe('6 个');
    expect(snapshot?.metrics.find((item) => item.label === '置信度')?.value).toBe('91.0%');
    expect(snapshot?.rows[0]['阶段']).toBe('初始访问');
    expect(snapshot?.rows[0]['实体']).toBe('203.0.113.45');
    expect(snapshot?.rows[0]['告警']).toBe('Web 漏洞利用');
    expect(snapshot?.rows[1]['处置建议']).toBe('阻断 C2 域名');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Attack Chains API');
  });

  it('maps split topic payloads into tunnel, exfiltration and apt snapshots', () => {
    const tunnel = findRouteById('topic-tunnel');
    const exfil = findRouteById('topic-exfil');
    const apt = findRouteById('topic-apt');
    expect(tunnel).toBeTruthy();
    expect(exfil).toBeTruthy();
    expect(apt).toBeTruthy();

    const tunnelSnapshot = adaptKnownPageSnapshot(
      tunnel!.page,
      {
        data: {
          summary: { protocol_count: 2, active_users: 2, session_count: 1280, total_bytes: 2_147_483_648, high_risk_users: 1 },
          protocols: [{ protocol: 'TLS', count: 900, total_bytes: 1_600_000_000 }],
          users: [{ ip: '10.12.2.36', count: 320, protocol: 'DoH/TLS', risk: 'high', total_bytes: 900_000_000, last_seen: 1792886400 }],
        },
      },
      [],
    );

    const exfilSnapshot = adaptKnownPageSnapshot(
      exfil!.page,
      {
        data: {
          summary: { source_count: 3, path_count: 4, session_count: 680, upload_bytes: 1_073_741_824, high_risk_sources: 2 },
          top_sources: [{ src_ip: '10.12.8.45', session_count: 210, upload_bytes: 486_000_000, dst_count: 8, risk: 'high' }],
          risk_types: [{ type: 'cloud_storage', count: 12, severity: 'high', total_bytes: 680_000_000 }],
          paths: [{ src_ip: '10.12.8.45', dst_ip: '198.51.100.27', session_count: 88, upload_bytes: 420_000_000, risk: 'high' }],
        },
      },
      [],
    );

    const aptSnapshot = adaptKnownPageSnapshot(
      apt!.page,
      {
        data: {
          summary: { campaign_count: 2, listed_campaigns: 1, high_risk_count: 1, entity_count: 4, alert_count: 18 },
          phase_distribution: { initial_access: 1, command_and_control: 1 },
          campaigns: [
            {
              campaign_id: 'APT-20260619-001',
              campaign_type: 'apt',
              score: 0.91,
              entities: ['WEB-SRV-02', 'DC-01'],
              alerts: ['AL-1', 'AL-2'],
              attack_phases: ['initial_access', 'command_and_control'],
              ts_start: 1792886400,
              ts_end: 1792890000,
            },
          ],
        },
      },
      [],
    );

    expect(tunnelSnapshot?.metrics.find((item) => item.label === '活跃隧道会话')?.value).toBe('1.3K');
    expect(tunnelSnapshot?.rows[0]['风险状态']).toBe('高危');
    expect(tunnelSnapshot?.evidence.map((item) => item.label)).toContain('Tunnel Topic API');
    expect(exfilSnapshot?.metrics.find((item) => item.label === '外传路径数')?.value).toBe('4');
    expect(exfilSnapshot?.rows[0]['外传路径']).toBe('10.12.8.45 -> 198.51.100.27');
    expect(exfilSnapshot?.evidence.map((item) => item.label)).toContain('告警证据');
    expect(aptSnapshot?.metrics.find((item) => item.label === '关联战役数')?.value).toBe('2');
    expect(aptSnapshot?.rows[0]['战役名称']).toBe('APT-20260619-001');
    expect(aptSnapshot?.evidence.map((item) => item.label)).toContain('APT Topic API');
  });

  it('maps encrypted traffic payload into encrypted workbench rows and evidence', () => {
    const route = findRouteById('encrypted-traffic');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          total_sessions: 100,
          encrypted_ratio: 0.82,
          tls_sessions: 64,
          quic_sessions: 18,
          ja3_fingerprints: 32,
          malicious_ja3_matches: 7,
        },
      },
      [
        {
          data: {
            sessions: [
              {
                session_id: 'sess-001',
                src_ip: '10.12.2.36',
                dst_ip: '203.0.113.45',
                dst_port: 443,
                protocol: 'TLS',
                sni: '',
                ja3_fingerprint: '771,4865-4866',
                ja3s_fingerprint: '8f9e3d7a',
                cipher_suite: 'TLS_AES_128_GCM_SHA256',
                tls_version: 'TLS 1.3',
                certificate_issuer: 'Cloudflare Inc ECC CA-3',
                certificate_valid_until: 1893456000,
                risk_level: 'high',
                entropy_score: 7.8,
                anomaly_score: 0.91,
                start_time: 1792886400,
              },
              {
                session_id: 'sess-002',
                src_ip: '10.10.8.45',
                dst_ip: '198.51.100.27',
                dst_port: 443,
                protocol: 'QUIC',
                sni: 'sync.example.net',
                ja3_fingerprint: 'cbd52c1e',
                ja3s_fingerprint: 'a1b2c3d4',
                tls_version: 'TLS 1.3',
                certificate_issuer: '',
                risk_level: 'medium',
                start_time: 1792886460,
              },
            ],
            total: 2,
          },
        },
        { data: { fingerprints: [{ ja3: '771,4865-4866', risk_level: 'high' }] } },
        { data: { protocols: [{ protocol: 'DoH', count: 12 }], users: [{ user: 'sec_analyst', count: 4 }] } },
        { data: { top_sources: [{ src_ip: '10.12.2.36', bytes: 1024 }], risk_types: [{ type: 'exfil', count: 1 }], paths: [{ path: '10.12.2.36->203.0.113.45' }] } },
        {
          data: {
            sessions: [{ session_id: 'evidence-001', src_ip: '10.12.2.36', dst_ip: '203.0.113.45', protocol: 'TLS', sni_hash: 'update.example.com', ja3_fingerprint: 'ja3-evidence', certificate_hash: '90ab', alpn: 'h2', risk_level: 'high', entropy_score: 7.8, start_time: 1792886400000 }],
            pcap_indexes: [{ file_key: 'pcap/001.pcap', probe_id: 'probe-gw-01', ts_start: 1792886400000, ts_end: 1792886460000, byte_count: 2048, packet_count: 24, sha256: 'abc123def456' }],
            pcap_trend: [{ bucket_start: 1792886400000, byte_count: 2048, packet_count: 24 }],
            entropy_trend: [{ bucket_start: 1792886400000, entropy_score: 7.8 }],
            completeness: [{ label: 'Session', complete: 1, total: 1 }, { label: 'PCAP', complete: 1, total: 1 }],
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '加密流量总量')?.value).toBe('100 会话');
    expect(snapshot?.metrics.find((item) => item.label === 'TLS 流量占比')?.value).toBe('64.0%');
    expect(snapshot?.metrics.find((item) => item.label === 'QUIC 流量占比')?.value).toBe('18.0%');
    expect(snapshot?.metrics.find((item) => item.label === '未知 SNI 比例')?.value).toBe('50.0%');
    expect(snapshot?.rows[0]['协议']).toBe('TLS');
    expect(snapshot?.rows[0]['Session 摘要']).toBe('10.12.2.36 -> 203.0.113.45:443');
    expect(snapshot?.rows[0]['证书详情']).toBe('有效');
    expect(snapshot?.rows[0]['风险等级']).toBe('高危');
    expect(snapshot?.rows[1]['证书详情']).toBe('缺失证书');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Tunnel Analytics API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Exfiltration API');
    expect(snapshot?.visuals?.encryptedTraffic?.protocolRows[0]).toEqual(['TLS', '0.0 Gbps', '64.0%', 'is-info']);
    expect(snapshot?.visuals?.encryptedTraffic?.ja3Rows[0][0]).toBe('771,4865-4866');
    expect(snapshot?.visuals?.encryptedTraffic?.tunnelCards[0][0]).toBe('高频 DNS 候选');
    expect(snapshot?.visuals?.encryptedTraffic?.tunnelRuleRows).toHaveLength(1);
    expect(snapshot?.visuals?.encryptedTraffic?.tunnelRuleRows.map((row) => row[0])).toContain('高频 DNS 候选规则');
    expect(snapshot?.visuals?.encryptedTraffic?.heartbeatBars).toHaveLength(0);
    expect(snapshot?.visuals?.encryptedTraffic?.destinationRows[0][0]).toBe('203.0.113.45');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceRows[0][5]).toBe('高危');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.availability.state).toBe('partial');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.sessions[0].sessionId).toBe('evidence-001');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.sessions[0].sni).toBe('update.example.com');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.sessions[0].certificateHash).toBe('90ab');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.sessions[0].entropy).toBe(7.8);
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.pcapRows[0][0]).toBe('pcap/001.pcap');
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.pcapTrend[0].value).toBe(2048);
    expect(snapshot?.visuals?.encryptedTraffic?.evidenceCenter.entropyTrend[0].value).toBe(7.8);
  });

  it('keeps encrypted traffic empty when all source APIs are empty', () => {
    const route = findRouteById('encrypted-traffic');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      { data: { total_sessions: 0, tls_sessions: 0, quic_sessions: 0 } },
      [
        { data: { sessions: [] } },
        { data: { fingerprints: [] } },
        { data: { protocols: [], users: [] } },
        { data: { top_sources: [], risk_types: [], paths: [] } },
      ],
    );

    const visuals = snapshot?.visuals?.encryptedTraffic;
    expect(visuals?.egressAvailability.state).toBe('unavailable');
    expect(visuals?.egressAvailability.detail).toContain('未生成任何替代数据');
    expect(visuals?.egressMapNodes).toHaveLength(0);
    expect(visuals?.egressDomainCards).toHaveLength(0);
    expect(visuals?.egressTrend.labels).toHaveLength(0);
    expect(visuals?.egressTrend.series).toHaveLength(0);
    expect(visuals?.egressKpis[0]).toEqual(['公网目的地', '—', '等待外传 API']);
    expect(visuals?.evidenceCenter.availability.state).toBe('unavailable');
    expect(visuals?.evidenceCenter.kpis[0]).toEqual(['会话证据', '0', '证据 API']);
    expect(visuals?.evidenceCenter.sessions).toHaveLength(0);
    expect(visuals?.evidenceCenter.pcapRows).toHaveLength(0);
    expect(visuals?.evidenceCenter.pcapTrend).toHaveLength(0);
    expect(visuals?.evidenceCenter.completeness.find((item) => item.label === 'Session')).toMatchObject({ complete: 0, total: 0 });
  });

  it('uses real destination and time-bucket fields without synthesizing egress trend categories', () => {
    const route = findRouteById('encrypted-traffic');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      { data: { total_sessions: 2, tls_sessions: 2, quic_sessions: 0 } },
      [
        { data: { sessions: [] } },
        { data: { fingerprints: [] } },
        { data: { protocols: [], users: [] } },
        {
          data: {
            top_sources: [{ src_ip: '10.12.2.36', session_count: 9 }],
            top_destinations: [{ dst_ip: '203.0.113.45', session_count: 9, upload_bytes: 1024, risk: 'high' }],
            risk_types: [],
            paths: [{ src_ip: '10.12.2.36', dst_ip: '203.0.113.45', session_count: 9 }],
            trend: [{ bucket_start: 1792886400000, destination_count: 3, large_upload_sessions: 2, long_lived_sessions: 1, non_standard_port_sessions: 4, encrypted_sessions: 9 }],
          },
        },
      ],
    );

    const visuals = snapshot?.visuals?.encryptedTraffic;
    expect(visuals?.egressAvailability.state).toBe('live');
    expect(visuals?.destinationRows[0][0]).toBe('203.0.113.45');
    expect(visuals?.egressMapNodes[0].label).toBe('203.0.113.45');
    expect(visuals?.egressTrend.labels).toHaveLength(1);
    expect(visuals?.egressTrend.series.map((series) => series.values[0])).toEqual([3, 2, 1, 4, 9]);
  });

  it('maps forensic pcap jobs into forensics workbench rows and evidence', () => {
    const route = findRouteById('forensics');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            job_id: 'F-20260620-000189',
            status: 'completed',
            progress: 100,
            total_packets: 128800,
            total_bytes: 134217728,
            files_scanned: 5,
            result_file_key: 'results/default/2026-06-20/F-20260620-000189.pcap',
            download_url: 'https://minio.local/signed/F-20260620-000189',
            expires_at: 1792976400000,
            params: {
              alert_id: 'AL-20260620-000123',
              asset_id: '办公区-WS-1024',
              src_ip: '172.16.5.10',
              src_port: 44221,
              dst_ip: '185.22.14.9',
              dst_port: 443,
              protocol: 'TLS',
              start_time: 1792886400000,
              end_time: 1792887000000,
            },
          },
          {
            job_id: 'F-20260620-000190',
            status: 'processing',
            progress: 65,
            total_packets: 0,
            total_bytes: 0,
            files_scanned: 2,
            params: {
              campaign_id: 'APT-20260619-001',
              asset_id: '核心区-DC-01',
              src_ip: '10.12.8.45',
              dst_ip: '10.12.9.33',
              protocol: 'SMB',
            },
          },
        ],
        total: 2,
      },
      [
        {
          data: {
            task_stats: { queued: 1, processing: 1, completed: 1, failed: 0, cancelled: 0 },
            worker_stats: { workers: 3, queue_size: 1 },
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '取证任务')?.value).toBe('2 项');
    expect(snapshot?.metrics.find((item) => item.label === '处理中')?.value).toBe('2 项');
    expect(snapshot?.metrics.find((item) => item.label === '已完成')?.value).toBe('1 项');
    expect(snapshot?.metrics.find((item) => item.label === '签名 URL')?.value).toBe('1 个');
    expect(snapshot?.rows[0]['任务 ID']).toBe('F-20260620-000189');
    expect(snapshot?.rows[0]['告警/战役 ID']).toBe('AL-20260620-000123');
    expect(snapshot?.rows[0]['资产']).toBe('办公区-WS-1024');
    expect(snapshot?.rows[0]['五元组']).toBe('172.16.5.10:44221 -> 185.22.14.9:443 TLS');
    expect(snapshot?.rows[0]['证据包']).toBe('F-20260620-000189.pcap');
    expect(snapshot?.rows[0]['状态']).toBe('完成');
    expect(snapshot?.rows[1]['状态']).toBe('采集中');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('PCAP Jobs API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('下载审计');
  });

  it('maps rule manager payload into rule lifecycle workbench rows and gates', () => {
    const route = findRouteById('rules');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            rule_id: 'C2_Tunnel_v3',
            name: 'C2 隧道通信检测',
            type: 'signature',
            engine: 'suricata',
            severity: 'high',
            enabled: true,
            priority: 90,
            version: 3,
            status: 'active',
            labels: ['TA0011', 'c2'],
            updated_at: '2026-06-19T17:28:45Z',
          },
          {
            rule_id: 'APT_ToolDrop_v1',
            name: 'APT 工具投递检测',
            type: 'yara',
            engine: 'yara',
            severity: 'critical',
            enabled: false,
            priority: 80,
            version: 2,
            status: 'draft',
            labels: ['TA0002'],
          },
          {
            rule_id: 'Port_Scan_v1',
            name: '端口扫描检测',
            type: 'threshold',
            engine: 'internal',
            severity: 'medium',
            enabled: false,
            priority: 40,
            version: 4,
            status: 'disabled',
          },
        ],
        pagination: { total: 736, limit: 8, offset: 0 },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '启用规则')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '规则草稿')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '回滚候选')?.value).toBe('1 条');
    expect(snapshot?.rows[0].规则ID).toBe('C2_Tunnel_v3');
    expect(snapshot?.rows[0].规则名称).toBe('C2 隧道通信检测');
    expect(snapshot?.rows[0].类型).toBe('特征');
    expect(snapshot?.rows[0].严重级别).toBe('高危');
    expect(snapshot?.rows[0].MITRE阶段).toBe('TA0011');
    expect(snapshot?.rows[0].状态).toBe('启用');
    expect(snapshot?.rows[0].最近状态变更).toBe('2026-06-19 17:28');
    expect(snapshot?.rows[0].状态操作人).toBe('system');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Rules API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('发布门禁');
  });

  it('maps deployment manager payload into release workbench rows and gates', () => {
    const route = findRouteById('deployments');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            deployment_id: 'DEP-001',
            name: '规则包-APT检测增强',
            rule_version: 'v2.3.1',
            scope: { tenant: '租户A', region: '华东园区', probes: 12, asset_group: '核心业务资产组', percentage: 20 },
            status: 'gray',
            created_by: '安全运营组',
            created_at: '2025-05-27T14:15:00Z',
            updated_at: '2025-05-27T14:16:00Z',
          },
          {
            deployment_id: 'DEP-002',
            name: '异常流量检测模型',
            model_version: 'v1.8.0',
            scope: { tenant: '租户B', probes: 8 },
            status: 'planned',
            created_by: '算法平台组',
            created_at: '2025-05-27T13:40:00Z',
            updated_at: '2025-05-27T13:40:00Z',
          },
          {
            deployment_id: 'DEP-003',
            name: '配置模板-告警阈值',
            feature_set_id: 'config-v1.2.0',
            scope: { tenant: '租户A', probes: 2 },
            status: 'failed',
            created_by: '安全运营组',
            created_at: '2025-05-27T09:22:00Z',
            updated_at: '2025-05-27T09:25:00Z',
          },
          {
            deployment_id: 'DEP-004',
            name: '规则包-僵木马C2检测',
            rule_version: 'v2.1.4',
            scope: { tenant: '租户A' },
            status: 'rolled_back',
            created_by: '安全运营组',
            created_at: '2025-05-26T22:10:00Z',
            updated_at: '2025-05-26T22:11:00Z',
          },
        ],
        pagination: { total: 48, limit: 8, offset: 0 },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '待发布对象')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '灰度中')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '失败/阻断')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '可回滚版本')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '平均生效延迟')?.value).toBe('100 s');
    expect(snapshot?.rows[0].发布对象).toBe('规则包-APT检测增强');
    expect(snapshot?.rows[0].版本).toBe('v2.3.1');
    expect(snapshot?.rows[0].环境).toBe('canary');
    expect(snapshot?.rows[0].状态).toBe('灰度中');
    expect(snapshot?.rows[0].影响范围).toContain('租户A');
    expect(snapshot?.rows[0].影响范围).toContain('20% 流量');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Deployments API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('manifest');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('规则版本');
  });

  it('maps model registry payload into model management workbench rows and gates', () => {
    const route = findRouteById('models');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: [
          {
            model_id: 'MODEL-UEBA',
            name: 'UEBA 行为分析',
            model_type: 'classification',
            metadata: {
              status: 'online',
              current_version: 'v1.8.0',
              online_version: 'v1.8.0',
              owner: '安全运营组',
              trained_at: '2026-06-19T22:10:00Z',
              metrics: { f1_score: 0.948, auc: 0.982, drift: 0.12, false_positive_delta: -6.2 },
            },
            created_at: '2026-06-01T09:00:00Z',
            updated_at: '2026-06-19T22:10:00Z',
          },
          {
            model_id: 'MODEL-TUNNEL',
            name: '加密隧道检测',
            model_type: 'detection',
            metadata: {
              status: 'challenger',
              candidate_version: 'v2.3.1',
              owner: '网络安全组',
              trained_at: '2026-06-19T18:40:00Z',
              metrics: { f1_score: 0.963, drift: 0.18 },
            },
          },
          {
            model_id: 'MODEL-EXFIL',
            name: '数据外传识别',
            model_type: 'classification',
            metadata: {
              status: 'drift',
              current_version: 'v1.5.2',
              owner: '数据安全组',
              trained_at: '2026-06-19T16:05:00Z',
              metrics: { f1_score: 0.882, drift: 0.38 },
            },
          },
        ],
        pagination: { total: 28, limit: 8, offset: 0 },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '线上模型数')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '候选模型数')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '漂移告警')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '待重训模型')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '平均 F1')?.value).toBe('0.931');
    expect(snapshot?.rows[0].模型名).toBe('UEBA 行为分析');
    expect(snapshot?.rows[0].__model_id).toBe('MODEL-UEBA');
    expect(snapshot?.rows[0].类型).toBe('分类');
    expect(snapshot?.rows[0].版本).toBe('v1.8.0');
    expect(snapshot?.rows[0].状态).toBe('线上');
    expect(snapshot?.rows[0].线上版本).toBe('v1.8.0');
    expect(snapshot?.rows[0].负责人).toBe('安全运营组');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Models API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('激活门禁');
  });

  it('maps mlops orchestrator payload into orchestration rows and gates', () => {
    const route = findRouteById('mlops');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          last_retrain_time: '2026-06-19T22:10:00Z',
          running_workflows: 2,
          max_concurrent: 6,
          min_retrain_interval: '6h0m0s',
          check_interval: '5m0s',
          min_feedback_count: 500,
          max_fp_rate: 5,
          clickhouse_connected: true,
        },
      },
      [
        {
          data: {
            triggers: [
              { name: 'feedback', description: 'Feedback accumulation threshold reached' },
              { name: 'fp_rate', description: 'False positive rate exceeds limit' },
              { name: 'drift', description: 'Feature distribution drift detected' },
            ],
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '训练任务')?.value).toBe('2 项');
    expect(snapshot?.metrics.find((item) => item.label === '评估任务')?.value).toBe('3 项');
    expect(snapshot?.metrics.find((item) => item.label === '注册任务')?.value).toBe('3 项');
    expect(snapshot?.metrics.find((item) => item.label === '发布任务')?.value).toBe('3 项');
    expect(snapshot?.metrics.find((item) => item.label === '失败任务')?.value).toBe('0 项');
    expect(snapshot?.metrics.find((item) => item.label === '门禁通过率')?.value).toBe('86.7%');
    expect(snapshot?.rows[0].任务ID).toBe('TR-20250527-006');
    expect(snapshot?.rows[0].阶段).toBe('训练任务');
    expect(snapshot?.rows[0].资源占用).toContain('GPU 70%');
    expect(snapshot?.rows[6].阶段).toBe('反馈触发');
    expect(snapshot?.rows[7].阶段).toBe('误报触发');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('MLOps Status API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Conditions API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('ClickHouse');
  });

  it('maps playbook catalog and executions into SOAR automation workbench rows and evidence', () => {
    const route = findRouteById('playbooks');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          playbooks: [
            {
              name: 'block-scanner',
              description: '自动封禁扫描源 IP (临时 24h)',
              enabled: true,
              trigger: { alert_type: 'scan', severity_min: 'high', score_min: 0.8 },
              actions: [
                { type: 'block_ip', parameters: { duration: '24h' } },
                { type: 'notify', parameters: { channel: 'slack' } },
              ],
              max_runs: 10,
              run_count: 4,
              updated_at: '2026-06-19T22:10:00Z',
            },
            {
              name: 'quarantine-c2',
              description: '隔离 C2 通信主机',
              enabled: true,
              trigger: { alert_type: 'c2', severity_min: 'critical', score_min: 0.9 },
              actions: [
                { type: 'quarantine', parameters: { target: 'source_ip' } },
                { type: 'capture_pcap', parameters: { duration: '300s' } },
                { type: 'notify', parameters: { channel: 'email+slack' } },
              ],
              max_runs: 3,
              run_count: 0,
            },
            {
              name: 'investigate-exfil',
              description: '数据外泄取证 + 升级',
              enabled: false,
              trigger: { alert_type: 'data_exfil', severity_min: 'high', score_min: 0.8 },
              actions: [
                { type: 'capture_pcap', parameters: { duration: '600s' } },
                { type: 'escalate', parameters: { level: 'L2' } },
                { type: 'notify', parameters: { channel: 'email' } },
              ],
              max_runs: 5,
              run_count: 1,
            },
          ],
          total: 3,
        },
      },
      [
        {
          data: {
            executions: [
              {
                execution_id: 'exec-001',
                playbook: 'block-scanner',
                alert_id: 'AL-001',
                success_actions: 2,
                failed_actions: 0,
                duration_ms: 384000,
                created_at: '2026-06-20T03:45:00Z',
              },
              {
                execution_id: 'exec-002',
                playbook: 'investigate-exfil',
                alert_id: 'AL-002',
                success_actions: 1,
                failed_actions: 2,
                duration_ms: 120000,
                created_at: '2026-06-20T03:40:00Z',
              },
            ],
            total: 2,
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '启用剧本')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '待审批')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '今日执行')?.value).toBe('2 次');
    expect(snapshot?.metrics.find((item) => item.label === '失败步骤')?.value).toBe('2 步');
    expect(snapshot?.metrics.find((item) => item.label === '高危待确认')?.value).toBe('3 项');
    expect(snapshot?.metrics.find((item) => item.label === '平均处理耗时')?.value).toBe('4分12秒');
    expect(snapshot?.rows[0].剧本名称).toBe('自动封禁扫描源 IP');
    expect(snapshot?.rows[0].适用告警).toBe('扫描告警');
    expect(snapshot?.rows[0].动作类型).toContain('阻断');
    expect(snapshot?.rows[0].风险级别).toBe('高危');
    expect(snapshot?.rows[0].启用状态).toBe('已启用');
    expect(snapshot?.rows[2].启用状态).toBe('已停用');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Playbook Catalog API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Executions API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('审计日志');
  });

  it('maps whitelist entries into governance rows and evidence', () => {
    const route = findRouteById('whitelist');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          entries: [
            {
              id: 'WL-001',
              tenant_id: 'default',
              type: 'domain',
              value: 'update.campus.local',
              reason: 'DNS 异常误报',
              description: 'Auto-whitelist from FP feedback: dns_noise (alert_id=AL-20260619-0187)',
              created_by: '安全运营',
              expires_at: '2026-08-10T00:00:00Z',
              created_at: '2026-06-10T00:00:00Z',
              covered_alerts: 128,
            },
            {
              id: 'WL-002',
              tenant_id: 'default',
              type: 'ip',
              value: '10.12.4.23',
              reason: '备份系统例外',
              created_by: '平台团队',
              expires_at: '2026-07-01T00:00:00Z',
              created_at: '2026-06-01T00:00:00Z',
              status: 'pending',
            },
            {
              id: 'WL-003',
              tenant_id: 'default',
              type: 'subnet',
              value: '10.12.0.0/16',
              reason: '长期业务例外',
              created_by: '安全运营',
              created_at: '2025-01-01T00:00:00Z',
            },
          ],
          total: 3,
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '生效白名单')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '待审批')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '长期生效')?.value).toBe('1 个');
    expect(snapshot?.metrics.find((item) => item.label === '覆盖告警')?.value).toBe('128 条');
    expect(snapshot?.rows[0].对象类型).toBe('域名');
    expect(snapshot?.rows[0].匹配条件).toBe('update.campus.local');
    expect(snapshot?.rows[0].来源告警).toBe('AL-20260619-0187');
    expect(snapshot?.rows[1].状态).toBe('待审批');
    expect(snapshot?.rows[2].状态).toBe('高风险覆盖');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Whitelist API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('审批状态');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('命中监控');
  });

  it('maps compliance reports into audit gate rows and evidence package fields', () => {
    const route = findRouteById('compliance');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          reports: [
            {
              report_id: 'RPT-202606-00023',
              tenant_id: 'default',
              report_type: 'weekly',
              time_range: { start: 1781308800000, end: 1781913600000 },
              generated_at: 1781995200000,
              status: 'completed',
              summary: {
                total_alerts: 100,
                critical_alerts: 12,
                resolved_alerts: 90,
                false_positives: 7,
                avg_response_time_min: 42,
                sla_violations: 2,
              },
              sections: [
                {
                  section_name: 'alert_response',
                  title: '告警响应闭环',
                  content: { resolved_alerts: 90, total_alerts: 100, resolution_rate: 0.9 },
                  status: 'pass',
                },
                {
                  section_name: 'critical_alerts',
                  title: '严重风险处置',
                  content: { critical_alerts: 12, sla_violations: 2 },
                  status: 'warn',
                },
                {
                  section_name: 'deployment_baseline',
                  title: '部署基线一致性',
                  content: { passed: 11, total: 16 },
                  status: 'fail',
                },
              ],
            },
          ],
          total: 1,
        },
      },
      [
        {
          data: {
            trails: [
              { log_id: 'LOG-1', action: 'COMPLIANCE_REPORT_GENERATED', result: 'success' },
              { log_id: 'LOG-2', action: 'EVIDENCE_EXPORTED', result: 'success' },
            ],
            total: 2,
          },
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '门禁通过率')?.value).toBe('33.3%');
    expect(snapshot?.metrics.find((item) => item.label === '未达标项')?.value).toBe('4 项');
    expect(snapshot?.metrics.find((item) => item.label === '证据完整度')?.value).toBe('84.0%');
    expect(snapshot?.metrics.find((item) => item.label === '复验通过率')?.value).toBe('90.0%');
    expect(snapshot?.metrics.find((item) => item.label === '报告生成数')?.value).toBe('1 份');
    expect(snapshot?.rows[0].维度).toBe('告警响应闭环');
    expect(snapshot?.rows[0].结果).toBe('通过');
    expect(snapshot?.rows[1].结果).toBe('待整改');
    expect(snapshot?.rows[2].结果).toBe('未达标');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Compliance API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('审计日志');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('部署 manifest');
  });

  it('maps audit logs into trace rows, risk metrics and retention evidence', () => {
    const route = findRouteById('audit-log');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          trails: [
            {
              log_id: 'LOG-001',
              tenant_id: 'default',
              user_id: 'sec_admin',
              action: 'PCAP_DOWNLOAD',
              resource_type: 'pcap',
              resource_id: 'pcap-20260621-001',
              details: { request_id: 'req-audit-001', trace_id: 'trace-audit-001', role: '安全管理员' },
              ip_address: '10.22.33.44',
              timestamp: 1782041515000,
              result: 'success',
            },
            {
              log_id: 'LOG-002',
              tenant_id: 'default',
              user_id: 'ml_admin',
              action: 'MODEL_ACTIVATED',
              resource_type: 'model',
              resource_id: 'model-v2',
              details: { request_id: 'req-audit-002', trace_id: 'trace-audit-002' },
              ip_address: '10.22.33.45',
              timestamp: 1782041455000,
              result: 'success',
            },
            {
              log_id: 'LOG-003',
              tenant_id: 'default',
              user_id: 'ops_admin',
              action: 'DEPLOYMENT_ROLLBACK',
              resource_type: 'deployment',
              resource_id: 'deploy-v1',
              details: { request_id: 'req-audit-003', trace_id: 'trace-audit-003' },
              ip_address: '10.22.33.46',
              timestamp: 1782041395000,
              result: 'failed',
            },
          ],
          total: 3,
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '今日操作')?.value).toBe('3 条');
    expect(snapshot?.metrics.find((item) => item.label === '失败操作')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '高风险操作')?.value).toBe('3 条');
    expect(snapshot?.metrics.find((item) => item.label === '导出下载')?.value).toBe('1 次');
    expect(snapshot?.metrics.find((item) => item.label === 'PCAP 访问')?.value).toBe('1 次');
    expect(snapshot?.rows[0]['用户/角色']).toBe('sec_admin / 安全管理员');
    expect(snapshot?.rows[0].对象类型).toBe('PCAP');
    expect(snapshot?.rows[0].动作类型).toBe('PCAP 访问');
    expect(snapshot?.rows[0].请求ID).toBe('req-audit-001');
    expect(snapshot?.rows[2].结果).toBe('失败');
    expect(snapshot?.rows[2].风险标签).toBe('高风险');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Audit Logs API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('高风险审计');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('留存状态');
  });

  it('maps notification settings into channel, subscription and delivery evidence', () => {
    const route = findRouteById('notifications');
    expect(route).toBeTruthy();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        data: {
          enabled: true,
          min_severity: 'high',
          rate_limit_per_min: 12,
          channels: {
            email: true,
            webhook: true,
            wechat: false,
            dingtalk: true,
            feishu: false,
          },
          secret_ref: 'traffic-analysis/notification-secret',
          rules: [
            {
              name: '严重告警',
              severity: 'critical',
              alert_type: '攻击告警',
              asset_group: '核心资产',
              time_window: '全天',
              recipient: '安全值班组',
              escalation: '夜间升级策略',
              silence: '低优先级静默',
              status: 'enabled',
            },
          ],
          history: [
            { status: 'failed' },
            { status: 'pending' },
            { status: 'success' },
          ],
          escalation_rules: [{ name: 'night' }, { name: 'sla' }],
          silence_rules: [{ name: 'maint' }],
          templates: [{ name: '告警模板' }],
        },
      },
      [],
    );

    expect(snapshot?.metrics.find((item) => item.label === '启用渠道')?.value).toBe('3 个');
    expect(snapshot?.metrics.find((item) => item.label === '失败通知')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '待确认通知')?.value).toBe('1 条');
    expect(snapshot?.metrics.find((item) => item.label === '升级策略')?.value).toBe('2 条');
    expect(snapshot?.metrics.find((item) => item.label === '静默窗口')?.value).toBe('1 个');
    expect(snapshot?.rows[0].规则).toBe('严重告警');
    expect(snapshot?.rows[0].严重级别).toBe('高危');
    expect(snapshot?.rows[0].渠道).toBe('邮件 / Webhook / 钉钉');
    expect(snapshot?.rows[0].状态).toBe('启用');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Notification Settings API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Secret 引用');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('投递审计');
  });

  it('maps settings token scopes and token list into governance rows', () => {
    const route = findRouteById('settings');
    expect(route).toBeTruthy();
    const activeTokenExpiresAt = new Date(Date.now() + 45 * 86_400_000).toISOString();
    const expiringTokenExpiresAt = new Date(Date.now() + 3 * 86_400_000).toISOString();

    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      {
        scopes: [
          { name: 'alert:read', description: 'Read alerts', category: 'alert' },
          { name: 'token:write', description: 'Manage API tokens', category: 'admin' },
          { name: 'probe:ingest', description: 'Upload flow events', category: 'probe' },
          { name: 'pcap:download', description: 'Download PCAP files', category: 'pcap' },
        ],
      },
      [
        {
          tokens: [
            {
              token_id: '11111111-2222-3333-4444-555555555555',
              tenant_id: 'default',
              name: 'SOAR-Executor',
              scopes: ['alert:read', 'token:write'],
              token_prefix: 'abcd',
              status: 'active',
              expires_at: activeTokenExpiresAt,
              last_used_at: '2026-06-21T11:23:00Z',
              rotation_enabled: true,
            },
            {
              token_id: '22222222-3333-4444-5555-666666666666',
              tenant_id: 'campus-a',
              name: 'PCAP-Export',
              scopes: ['pcap:download'],
              token_prefix: 'pcap',
              status: 'active',
              expires_at: expiringTokenExpiresAt,
              last_used_at: '2026-06-20T16:02:00Z',
            },
            {
              token_id: '33333333-4444-5555-6666-777777777777',
              tenant_id: 'campus-a',
              name: 'Revoked-Token',
              scopes: ['probe:ingest'],
              token_prefix: 'rvkd',
              status: 'revoked',
              expires_at: '2026-12-01T00:00:00Z',
            },
          ],
          total: 3,
        },
        {
          scopes: [
            { name: 'probe:ingest', category: 'probe' },
            { name: 'probe:metrics', category: 'probe' },
          ],
          default_scopes: ['probe:ingest', 'probe:metrics'],
        },
      ],
    );

    expect(snapshot?.metrics.find((item) => item.label === '租户数')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '角色策略')?.value).toBe('4 项');
    expect(snapshot?.metrics.find((item) => item.label === '有效令牌')?.value).toBe('2 个');
    expect(snapshot?.metrics.find((item) => item.label === '即将过期令牌')?.value).toBe('1 个');
    expect(snapshot?.rows[0].令牌名称).toBe('SOAR-Executor');
    expect(snapshot?.rows[0].权限范围).toBe('告警查看、令牌管理');
    expect(snapshot?.rows[0].令牌指纹).toBe('abcd****17');
    expect(snapshot?.rows[1].轮换状态).toBe('即将过期');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Token Scopes API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Token List API');
    expect(snapshot?.evidence.map((item) => item.label)).toContain('Probe Scopes API');
  });

  it('maps forensics jobs, sessions, PCAP indexes and audit without invented fallbacks', () => {
    const route = findRouteById('forensics');
    expect(route).toBeTruthy();
    const snapshot = adaptKnownPageSnapshot(
      route!.page,
      { data: [{ job_id: 'job-live-1', status: 'completed', progress: 100, result_file_key: 'default/result/job-live-1.pcap', sha256: 'a'.repeat(64), total_bytes: 4096, total_packets: 12, files_scanned: 1, params: { src_ip: '10.0.0.8', dst_ip: '8.8.8.8', dst_port: 443, asset_id: 'asset-live' }, created_at: 1784100000000, completed_at: 1784100060000 }], pagination: { total: 11 } },
      [
        { data: { task_stats: { completed: 11 }, worker_stats: { workers: 2 } } },
        { data: { sessions: [{ session_id: 'session-live-1', src_ip: '10.0.0.8', dst_ip: '8.8.8.8', dst_port: 443, protocol: 'TLS', byte_count: 2048, packet_count: 8, start_time: 1784100000000, end_time: 1784100005000, risk_level: 'high' }] } },
        { data: { pcap_indexes: [{ file_key: 'default/result/job-live-1.pcap', storage_path: 's3://pcap/default/result/job-live-1.pcap', probe_id: 'probe-live', byte_count: 4096, packet_count: 12, sha256: 'a'.repeat(64), start_time: 1784100000000, end_time: 1784100060000 }], pcap_trend: [{ bucket_start: 1784100000000, byte_count: 4096 }], completeness: [{ label: '索引Hash', complete: 1, total: 1 }] } },
        { data: { trails: [{ log_id: 'audit-live-1', user_id: 'analyst', action: 'PCAP_DOWNLOAD', resource_type: 'pcap', resource_id: 'default/result/job-live-1.pcap', result: 'success', timestamp: 1784100060000 }], total: 1 } },
      ],
    );

    expect(snapshot?.total).toBe(11);
    expect(snapshot?.rows[0]['任务 ID']).toBe('job-live-1');
    expect(snapshot?.rows[0]['告警/战役 ID']).toBe('-');
    expect(snapshot?.rows[0].资产).toBe('asset-live');
    expect(snapshot?.visuals?.forensics?.sessions[0].sessionId).toBe('session-live-1');
    expect(snapshot?.visuals?.forensics?.pcapIndexes[0].fileKey).toBe('default/result/job-live-1.pcap');
    expect(snapshot?.visuals?.forensics?.hashRows[0].sha256).toHaveLength(64);
    expect(snapshot?.visuals?.forensics?.auditRows[0].target).toBe('default/result/job-live-1.pcap');
    expect(JSON.stringify(snapshot)).not.toContain('办公区-WS-1024');
    expect(JSON.stringify(snapshot)).not.toContain('AL-20260620-');
  });
});
