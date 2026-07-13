import {
  AlertOutlined,
  ApiOutlined,
  BlockOutlined,
  BranchesOutlined,
  CalendarOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileSearchOutlined,
  FullscreenOutlined,
  LinkOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo } from 'react';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const phases = [
  ['侦察', 'TA0043', '203.0.113.45', '端口扫描探测', 'DNS 解析记录', '封禁源 IP', 'info'],
  ['初始访问', 'TA0001', '边界防火墙 FW-01', 'Web 漏洞利用', 'HTTP 请求包', 'WAF 规则加固', 'ok'],
  ['执行', 'TA0002', 'WEB 服务器 10.12.5.23', '恶意命令执行', '进程创建日志', '终止恶意进程', 'ok'],
  ['横向移动', 'TA0008', '域控服务器 10.12.1.10', '凭证窃取', 'LSASS 访问', '重置域控凭证', 'warn'],
  ['C2 通信', 'TA0011', '内网主机 10.12.8.45', 'C2 隧道通信', 'TLS 流量会话', '阻断 C2 域名', 'warn'],
  ['数据外传', 'TA0010', '外部域名 c2.example.com', '数据外传尝试', '外传流量样本', '阻断外传通道', 'risk'],
];

const evidenceRows = [
  ['1', 'PCAP', 'dns-20260619-0112.pcap', '01:12:08', '100%'],
  ['2', 'PCAP', 'web-20260619-0114.pcap', '01:14:22', '100%'],
  ['3', '日志', 'sysmon-4688.log', '01:15:03', '100%'],
  ['4', '日志', 'sysmon-10.log', '01:18:47', '95%'],
  ['5', 'Session', 'tls-session-012511.json', '01:25:11', '100%'],
  ['6', 'PCAP', 'exfil-20260619-0143.pcap', '01:43:02', '98%'],
];

const recommendations = [
  ['高', 'c2.example.com', '封禁域名', '低影响'],
  ['高', '198.51.100.27', '阻断 IP', '低影响'],
  ['中', '10.12.8.45', '隔离主机', '中等影响'],
  ['中', '10.12.5.23', '加强访问控制', '低影响'],
  ['低', 'SMB 445', '收紧防火墙策略', '低影响'],
  ['低', 'RDP 3389', '限制管理网段', '低影响'],
];

const attackChainOverlays: OverlayContract[] = [
  {
    id: 'drawer-attack-chain-detail',
    title: '攻击链详情抽屉',
    kind: 'Drawer',
    actionLabel: '链路详情',
    description: '展示攻击阶段、MITRE 技术、证据节点、处置建议和关联图谱路径。',
    audit: '记录攻击链下钻对象、筛选条件和分析 trace。',
  },
];

export function AttackChainAnalysisPage({ route }: { route: NavRoute }) {
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderAttackCell(column, value),
  }));

  return (
    <div className="taf-page taf-attack-chain">
      <section className="taf-attack-shell">
        <header className="taf-attack-toolbar">
          <h1>{route.page.title}</h1>
          <div className="taf-attack-filters">
            <label>
              <span>选择战役</span>
              <Select size="small" value="疑似 C2 隧道通信" options={[{ value: '疑似 C2 隧道通信' }, { value: 'RedLync APT' }]} />
            </label>
            <label>
              <span>时间范围</span>
              <Button size="small" icon={<CalendarOutlined />}>2026-06-19 00:00:00 ~ 2026-06-20 03:45:00</Button>
            </label>
            <label>
              <span>资产范围</span>
              <Select size="small" value="全部资产" options={[{ value: '全部资产' }, { value: '核心区' }, { value: '办公区' }]} />
            </label>
            <label>
              <span>视图模式</span>
              <Select size="small" value="攻击链视图" options={[{ value: '攻击链视图' }, { value: '泳道视图' }, { value: '矩阵视图' }]} />
            </label>
          </div>
          <Space>
            <Button size="small" icon={<DownloadOutlined />}>导出报告</Button>
            <Button size="small" icon={<LinkOutlined />}>下钻图谱</Button>
            <Button size="small" type="primary" icon={<BlockOutlined />}>触发响应</Button>
            <Tooltip title="刷新攻击链">
              <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
            </Tooltip>
            <Button size="small" icon={<FullscreenOutlined />} />
            <OverlayContractHost overlays={attackChainOverlays} compact />
          </Space>
        </header>

        {isError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={error instanceof Error ? error.message : '请检查 /v1/attack-chains、ClickHouse campaigns 或后端服务。'}
            action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
          />
        )}

        <div className="taf-attack-grid">
          <main className="taf-attack-main">
            <WorkPanel title="攻击链画布" className="taf-attack-canvas-panel">
              <AttackCanvas />
            </WorkPanel>
            <div className="taf-attack-bottom">
              <WorkPanel title="ATT&CK 阶段矩阵">
                <PhaseMatrix metrics={data?.metrics ?? []} />
              </WorkPanel>
              <WorkPanel title="路径明细（关键跳转）">
                <PathDetail rows={rows} columns={columns} isLoading={isLoading} />
              </WorkPanel>
            </div>
          </main>
          <aside className="taf-attack-rail">
            <EvidenceAnchorList />
            <ResponseRecommendations />
          </aside>
        </div>
      </section>
    </div>
  );
}

function AttackCanvas() {
  return (
    <div className="taf-attack-canvas">
      <div className="taf-attack-lane-head">
        <span>攻击阶段</span>
        <small>MITRE ATT&CK</small>
      </div>
      <div className="taf-attack-lanes">
        {['攻击阶段', '实体 / 资产', '告警事件', '证据锚点', '处置动作'].map((lane) => (
          <strong key={lane}>{lane}</strong>
        ))}
      </div>
      <div className="taf-attack-chain-columns">
        {phases.map(([phase, technique, entity, alert, evidence, action, tone], index) => (
          <div key={phase} className={`taf-attack-column is-${tone}`}>
            <div className="taf-attack-phase-card">
              <b>{index + 1}</b>
              <span>{phase}</span>
              <small>{technique}</small>
            </div>
            <div className="taf-attack-entity-card">
              <NodeIcon tone={tone} />
              <span>{entity}</span>
            </div>
            <div className="taf-attack-alert-card">
              <AlertOutlined />
              <span>{alert}</span>
              <small>06-19 01:{String(12 + index * 6).padStart(2, '0')}:08</small>
            </div>
            <div className="taf-attack-evidence-card">
              <FileSearchOutlined />
              <span>{evidence}</span>
              <small>pcap / sysmon / session</small>
            </div>
            <div className="taf-attack-action-card">
              <SafetyCertificateOutlined />
              <span>{action}</span>
              <small>{index < 2 ? '低影响' : index < 4 ? '中影响' : '需审批'}</small>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function PhaseMatrix({ metrics }: { metrics: PageSnapshot['metrics'] }) {
  const confidence = metrics.find((item) => item.label === '置信度')?.value ?? '92%';
  return (
    <div className="taf-attack-matrix">
      {phases.map(([phase, technique, , , , , tone]) => (
        <button key={phase} type="button" className={`is-${tone}`}>
          <strong>{phase}</strong>
          <span>{technique}</span>
          <i />
          <small>已发生</small>
        </button>
      ))}
      <div className="taf-attack-confidence">
        <span>链路置信度</span>
        <strong>{confidence}</strong>
      </div>
    </div>
  );
}

function PathDetail({ rows, columns, isLoading }: { rows: SnapshotRow[]; columns: ColumnsType<SnapshotRow>; isLoading: boolean }) {
  return (
    <Table
      rowKey={(record) => String(record['阶段'] ?? JSON.stringify(record))}
      size="small"
      loading={isLoading}
      pagination={false}
      columns={columns}
      dataSource={rows.slice(0, 5)}
    />
  );
}

function EvidenceAnchorList() {
  return (
    <WorkPanel title="证据锚点">
      <div className="taf-attack-tabs">
        {['全部', '告警', 'PCAP', 'Session', '日志', '图谱', '规则/模型'].map((tab, index) => (
          <button key={tab} type="button" className={index === 0 ? 'is-active' : ''}>{tab}</button>
        ))}
      </div>
      <div className="taf-attack-evidence-table">
        <div>
          <span>阶段</span>
          <span>类型</span>
          <span>名称</span>
          <span>时间</span>
          <span>完整度</span>
        </div>
        {evidenceRows.map(([phase, type, name, time, integrity]) => (
          <button key={name} type="button">
            <StatusTag value={phase} />
            <span>{type}</span>
            <strong>{name}</strong>
            <em>{time}</em>
            <em>{integrity}</em>
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function ResponseRecommendations() {
  return (
    <WorkPanel title="处置建议">
      <div className="taf-attack-suggestion-tabs">
        {['阻断点', '隔离建议', '白名单风险', '剧本推荐'].map((item, index) => (
          <button key={item} type="button" className={index === 0 ? 'is-active' : ''}>{item}</button>
        ))}
      </div>
      <div className="taf-attack-recommendations">
        {recommendations.map(([priority, target, action, impact], index) => (
          <button key={`${target}-${action}`} type="button">
            <b>{index + 1}</b>
            <StatusTag value={priority} />
            <span>{target}</span>
            <strong>{action}</strong>
            <em>{impact}</em>
            <EyeOutlined />
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function NodeIcon({ tone }: { tone: unknown }) {
  if (tone === 'risk') return <ApiOutlined />;
  if (tone === 'warn') return <BranchesOutlined />;
  if (tone === 'ok') return <SafetyCertificateOutlined />;
  return <NodeIndexOutlined />;
}

const renderAttackCell = (column: string, value: unknown): ReactNode => {
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '证据') return <span className="taf-attack-evidence-cell">{String(value ?? '')}</span>;
  return String(value ?? '');
};
