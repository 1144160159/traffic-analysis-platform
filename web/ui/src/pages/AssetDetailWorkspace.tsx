import {
  ApartmentOutlined,
  ArrowLeftOutlined,
  AuditOutlined,
  BranchesOutlined,
  CloseOutlined,
  DownloadOutlined,
  FileSearchOutlined,
  ProfileOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Alert, Button, Descriptions, Space, Table, Tabs, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { RingChart, TrendChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { assetDetailTabs, type AssetDetailSlug } from './assetInventoryState';

type DetailRow = Record<string, string> & { key: string };

const interfaces: DetailRow[] = [
  { key: 'eth0', '接口': 'eth0', '网卡': 'Intel X710', 'IP': '10.12.4.12', 'MAC': '00:50:56:AA:12:34', 'VLAN': '120', '模式': '业务', '状态': 'Up', '速率': '10G / Full', '入站/出站': '12.8 GB / 6.4 GB', '丢包': '0.02%' },
  { key: 'eth1', '接口': 'eth1', '网卡': 'Intel X710', 'IP': '10.12.4.13', 'MAC': '00:50:56:AA:12:35', 'VLAN': '121', '模式': '业务', '状态': 'Up', '速率': '10G / Full', '入站/出站': '4.1 GB / 2.2 GB', '丢包': '0.01%' },
  { key: 'bond0', '接口': 'bond0', '网卡': 'Bond0(Miix)', 'IP': '10.12.4.14', 'MAC': '-', 'VLAN': '200', '模式': '镜像', '状态': 'Up', '速率': '20G / Full', '入站/出站': '18.2 GB / 9.6 GB', '丢包': '0.03%' },
  { key: 'eth2', '接口': 'eth2', '网卡': 'Mellanox CX5', 'IP': '-', 'MAC': '00:50:56:AA:12:37', 'VLAN': '300', '模式': '监控', '状态': 'Monitor', '速率': '40G / Full', '入站/出站': '2.8 GB / 0 GB', '丢包': '0.00%' },
  { key: 'eth3', '接口': 'eth3', '网卡': 'Intel I350', 'IP': '172.16.22.12', 'MAC': '00:50:56:AA:12:38', 'VLAN': '30', '模式': '业务', '状态': 'Down', '速率': '1G / Full', '入站/出站': '0 B / 0 B', '丢包': '100%' },
  { key: 'mgmt0', '接口': 'mgmt0', '网卡': 'Intel I210', 'IP': '10.12.2.5', 'MAC': '00:50:56:AA:12:39', 'VLAN': '10', '模式': '管理', '状态': 'Up', '速率': '1G / Full', '入站/出站': '320 MB / 180 MB', '丢包': '0.01%' },
];

const services: DetailRow[] = [
  { key: '22', '端口': '22', '协议': 'TCP', '服务': 'SSH', '版本': 'OpenSSH 8.9p1', '暴露范围': '内网 + 外网', '访问来源': '18 个 IP', '风险': '高危', '关联告警': '5' },
  { key: '80', '端口': '80', '协议': 'TCP', '服务': 'HTTP', '版本': 'Nginx 1.20.1', '暴露范围': '外网', '访问来源': '9 个 IP', '风险': '中危', '关联告警': '3' },
  { key: '443', '端口': '443', '协议': 'TCP', '服务': 'HTTPS', '版本': 'Nginx 1.20.1', '暴露范围': '外网', '访问来源': '23 个 IP', '风险': '中危', '关联告警': '6' },
  { key: '3306', '端口': '3306', '协议': 'TCP', '服务': 'MySQL', '版本': '8.0.32', '暴露范围': '内网', '访问来源': '6 个 IP', '风险': '高危', '关联告警': '4' },
  { key: '6379', '端口': '6379', '协议': 'TCP', '服务': 'Redis', '版本': '6.2.6', '暴露范围': '内网', '访问来源': '5 个 IP', '风险': '高危', '关联告警': '2' },
  { key: '9200', '端口': '9200', '协议': 'TCP', '服务': 'OpenSearch', '版本': '2.11.0', '暴露范围': '内网 + 外网', '访问来源': '7 个 IP', '风险': '中危', '关联告警': '3' },
  { key: '9092', '端口': '9092', '协议': 'TCP', '服务': 'Kafka', '版本': '3.4.0', '暴露范围': '内网', '访问来源': '4 个 IP', '风险': '中危', '关联告警': '1' },
];

const ownership: DetailRow[] = [
  { key: 'asset', '责任类型': '资产管理员', '责任主体': '李老师 (138****6688)', '组织': '计算中心', '状态': '已确认', '最近确认': '2026-07-11 20:18' },
  { key: 'security', '责任类型': '安全复核', '责任主体': 'sec_manager', '组织': '安全运营组', '状态': '已确认', '最近确认': '2026-07-11 19:42' },
  { key: 'business', '责任类型': '业务确认', '责任主体': '张老师 (139****1122)', '组织': '教学应用组', '状态': '已确认', '最近确认': '2026-07-10 15:21' },
  { key: 'audit', '责任类型': '取证审批', '责任主体': '合规团队', '组织': '审计配置', '状态': '待审批', '最近确认': '-' },
];

const changes: DetailRow[] = [
  { key: '1', '时间': '2026-07-12 17:48:02', '类型': '开放服务变更', '字段': 'HTTPS/443', '变更前': '内网', '变更后': '内网 + 外网', '操作者': 'probe-12', '审计状态': '高风险待确认' },
  { key: '2', '时间': '2026-07-12 10:12:33', '类型': '归属变更', '字段': '责任人', '变更前': '李老师', '变更后': '张老师', '操作者': 'sec_admin', '审计状态': '已审计' },
  { key: '3', '时间': '2026-07-11 18:05:44', '类型': 'IP 变更', '字段': 'IP 地址', '变更前': '10.12.4.11', '变更后': '10.12.4.12', '操作者': 'dhcp-listener', '审计状态': '已审计' },
  { key: '4', '时间': '2026-07-11 09:31:20', '类型': 'MAC 绑定', '字段': 'MAC', '变更前': '-', '变更后': '00:50:56:AA:12:34', '操作者': 'arp-collector', '审计状态': '确认中' },
  { key: '5', '时间': '2026-07-10 14:22:33', '类型': '资产组变更', '字段': '资产组', '变更前': '通用服务器组', '变更后': '计算服务器组', '操作者': 'admin', '审计状态': '已审计' },
];

const metricSets: Record<AssetDetailSlug, Array<[string, string, string]>> = {
  basic: [['暴露端口', '8', 'risk'], ['弱口令疑似', '2', 'warn'], ['漏洞命中', '5', 'risk'], ['异常外联', '3', 'info']],
  'network-interface': [['网卡', '6', 'info'], ['在线接口', '4', 'ok'], ['镜像口', '2', 'info'], ['IP 绑定', '8', 'info'], ['VLAN', '3', 'warn'], ['异常接口', '1', 'risk']],
  'open-services': [['开放端口', '12', 'risk'], ['外网暴露', '3', 'info'], ['高危服务', '4', 'risk'], ['弱口令疑似', '2', 'warn'], ['关联告警', '19', 'warn'], ['24h 会话', '1,286', 'info']],
  ownership: [['责任角色', '4', 'info'], ['业务系统', '3', 'ok'], ['资产组', '2', 'warn'], ['数据域', '4', 'info'], ['待确认字段', '2', 'warn'], ['近 7 天变更', '5', 'info']],
  history: [['变更总数', '28', 'info'], ['IP/MAC 变更', '6', 'ok'], ['主机名变更', '3', 'info'], ['归属变更', '5', 'warn'], ['开放服务变更', '12', 'warn'], ['高风险变更', '3', 'risk'], ['可回滚', '4', 'ok'], ['未审计', '2', 'risk']],
};

const detailActions: Record<AssetDetailSlug, string[]> = {
  basic: ['打开完整详情', '跳转实体图谱', '生成整改工单', '进入取证分析', '更新资产归属'],
  'network-interface': ['查看通信路径', '查看镜像链路', '接口诊断', '生成整改工单', '更新接口归属'],
  'open-services': ['查看服务详情', '查看告警', '进入取证分析', '生成整改工单', '加入白名单'],
  ownership: ['申请归属变更', '补充责任角色', '查看审批链', '跳转实体图谱', '导出审计'],
  history: ['进入变更详情', '查看证据', '发起回滚', '生成整改工单', '导出审计包'],
};

const iconForAction = (label: string) => {
  if (label.includes('图谱') || label.includes('路径')) return <BranchesOutlined />;
  if (label.includes('审计') || label.includes('审批')) return <AuditOutlined />;
  if (label.includes('导出')) return <DownloadOutlined />;
  if (label.includes('归属') || label.includes('责任')) return <ApartmentOutlined />;
  if (label.includes('取证') || label.includes('证据') || label.includes('告警')) return <FileSearchOutlined />;
  if (label.includes('白名单')) return <SafetyCertificateOutlined />;
  if (label.includes('工单') || label.includes('诊断') || label.includes('回滚')) return <ToolOutlined />;
  return <ProfileOutlined />;
};

function columns(keys: string[]): ColumnsType<DetailRow> {
  return keys.map((key) => ({
    title: key,
    dataIndex: key,
    key,
    ellipsis: true,
    render: (value: string) => /风险|危|审计|确认|Up|Down|Monitor/.test(value) ? <StatusTag value={value} /> : value,
  }));
}

export function AssetDetailWorkspace({
  assetId,
  detail,
  onClose,
  onDetailChange,
}: {
  assetId: string;
  detail: AssetDetailSlug;
  onClose: () => void;
  onDetailChange: (detail: AssetDetailSlug) => void;
}) {
  const navigate = useNavigate();
  const [feedback, setFeedback] = useState<string>();
  const metrics = metricSets[detail];
  const actionButtons = detailActions[detail];
  const assetName = assetId === 'SRV-0007' ? '实验楼-SRV-12' : assetId;
  const title = assetDetailTabs.find((item) => item.slug === detail)?.label ?? '资产详情';

  const runAction = (label: string) => {
    if (label.includes('图谱') || label.includes('路径')) {
      navigate(`/graph?assetId=${encodeURIComponent(assetId)}`);
      return;
    }
    if (label.includes('取证') || label.includes('证据')) {
      navigate(`/forensics?assetId=${encodeURIComponent(assetId)}`);
      return;
    }
    if (label.includes('告警')) {
      navigate(`/alerts?assetId=${encodeURIComponent(assetId)}`);
      return;
    }
    setFeedback(`${label}已提交，任务对象 ${assetId}；后续状态将在资产审计记录中更新。`);
  };

  const content = useMemo(() => {
    if (detail === 'network-interface') return <NetworkInterfaceDetail />;
    if (detail === 'open-services') return <OpenServicesDetail />;
    if (detail === 'ownership') return <OwnershipDetail />;
    if (detail === 'history') return <HistoryDetail />;
    return <BasicDetail assetId={assetId} />;
  }, [assetId, detail]);

  return (
    <section className={`taf-asset-detail-workspace taf-asset-detail-${detail}`} data-breakdown-page-id={`assets-detail-${detail}`}>
      <header className="taf-asset-detail-workspace__title">
        <div>
          <span>资产台账 / {assetName}</span>
          <h1>资产详情</h1>
        </div>
        <Space>
          <Tooltip title="刷新当前资产">
            <Button aria-label="刷新当前资产" icon={<ReloadOutlined />} onClick={() => setFeedback(`已刷新 ${assetId} 的${title}数据。`)} />
          </Tooltip>
          <Tooltip title="返回资产列表">
            <Button aria-label="返回资产列表" icon={<ArrowLeftOutlined />} onClick={onClose} />
          </Tooltip>
          <Tooltip title="关闭详情">
            <Button aria-label="关闭详情" icon={<CloseOutlined />} onClick={onClose} />
          </Tooltip>
        </Space>
      </header>

      <div className="taf-asset-detail-workspace__identity">
        <CloudAssetIcon />
        <strong>{assetName}</strong>
        <StatusTag value="高风险" />
        <StatusTag value="在线" />
        <span>{assetId} · 10.12.4.12 · Ubuntu 22.04</span>
      </div>

      <div className={`taf-asset-detail-workspace__metrics is-${metrics.length}`}>
        {metrics.map(([label, value, tone]) => (
          <div key={label} className={`is-${tone}`}><strong>{value}</strong><span>{label}</span></div>
        ))}
      </div>

      <Tabs
        className="taf-asset-detail-workspace__tabs"
        activeKey={detail}
        onChange={(key) => onDetailChange(key as AssetDetailSlug)}
        items={assetDetailTabs.map((item) => ({ key: item.slug, label: item.label }))}
      />

      {feedback && <Alert closable showIcon type="success" message={feedback} onClose={() => setFeedback(undefined)} />}
      <div className="taf-asset-detail-workspace__content">{content}</div>

      <footer className="taf-asset-detail-workspace__actions">
        {actionButtons.map((label) => (
          <Button key={label} icon={iconForAction(label)} onClick={() => runAction(label)}>{label}</Button>
        ))}
      </footer>
    </section>
  );
}

function CloudAssetIcon() {
  return <span className="taf-asset-detail-workspace__asset-icon"><ProfileOutlined /></span>;
}

function BasicDetail({ assetId }: { assetId: string }) {
  return (
    <div className="taf-asset-detail-basic-grid">
      <WorkPanel title="基础信息" className="taf-asset-detail-span-2">
        <Descriptions size="small" column={2} bordered items={[
          { key: 'id', label: '资产 ID', children: assetId },
          { key: 'type', label: '类型', children: '服务器' },
          { key: 'ip', label: 'IP / MAC', children: '10.12.4.12 / 00:50:56:AA:12:34' },
          { key: 'os', label: '操作系统', children: 'Ubuntu 22.04' },
          { key: 'name', label: '主机名', children: 'SRV-12' },
          { key: 'risk', label: '风险等级', children: <StatusTag value="高风险" /> },
          { key: 'org', label: '园区 / 部门', children: '实验楼 / 计算中心' },
          { key: 'probe', label: '采集探针', children: 'probe-12' },
          { key: 'group', label: '资产组', children: '计算服务器组' },
          { key: 'source', label: '采集来源', children: '流量探针 + SNMP/LLDP' },
          { key: 'first', label: '首次发现', children: '2026-05-11 10:22:33' },
          { key: 'last', label: '最近活跃', children: '2026-07-12 18:44:12' },
        ]} />
      </WorkPanel>
      <WorkPanel title="资产健康度"><RingChart value={82} height={210} ariaLabel="资产健康度" /></WorkPanel>
      <WorkPanel title="标签与责任边界" className="taf-asset-detail-span-3">
        <div className="taf-asset-detail-tags">
          {['生产环境', '数据库服务器', '核心业务', '外网暴露', '计算中心', '教学管理系统'].map((item) => <StatusTag key={item} value={item} />)}
        </div>
      </WorkPanel>
    </div>
  );
}

function NetworkInterfaceDetail() {
  return (
    <div className="taf-asset-detail-layout">
      <WorkPanel title="网络接口清单" className="taf-asset-detail-span-3">
        <Table size="small" rowKey="key" columns={columns(['接口', '网卡', 'IP', 'MAC', 'VLAN', '模式', '状态', '速率', '入站/出站', '丢包'])} dataSource={interfaces} pagination={{ pageSize: 6, size: 'small' }} scroll={{ x: 1180 }} />
      </WorkPanel>
      <WorkPanel title="吞吐趋势（近 24 小时）" className="taf-asset-detail-span-2"><TrendChart title="接口入站 / 出站" /></WorkPanel>
      <WorkPanel title="接口状态"><RingChart value={83} height={210} ariaLabel="接口健康度" /></WorkPanel>
    </div>
  );
}

function OpenServicesDetail() {
  return (
    <div className="taf-asset-detail-layout">
      <WorkPanel title="开放服务表" className="taf-asset-detail-span-2">
        <Table size="small" rowKey="key" columns={columns(['端口', '协议', '服务', '版本', '暴露范围', '访问来源', '风险', '关联告警'])} dataSource={services} pagination={{ pageSize: 5, size: 'small' }} scroll={{ x: 820 }} />
      </WorkPanel>
      <WorkPanel title="暴露面评分"><RingChart value={74} height={250} ariaLabel="开放服务暴露面评分" /></WorkPanel>
      <WorkPanel title="24h 会话统计" className="taf-asset-detail-span-2"><TrendChart title="入站 / 出站会话" /></WorkPanel>
      <WorkPanel title="风险归因">
        <div className="taf-asset-detail-risk-list">{['外网暴露 3', '高危服务 4', '弱口令疑似 2', '关联告警 19'].map((item) => <span key={item}>{item}<StatusTag value="待处置" /></span>)}</div>
      </WorkPanel>
    </div>
  );
}

function OwnershipDetail() {
  return (
    <div className="taf-asset-detail-layout">
      <WorkPanel title="归属信息卡">
        <Descriptions size="small" column={1} bordered items={[
          { key: 'campus', label: '园区', children: '主园区' },
          { key: 'dept', label: '部门', children: '计算中心' },
          { key: 'system', label: '业务系统', children: '教学管理系统' },
          { key: 'group', label: '资产组', children: '计算服务器组' },
          { key: 'domain', label: '数据域', children: '教学业务域 / 运维日志域' },
        ]} />
      </WorkPanel>
      <WorkPanel title="责任角色" className="taf-asset-detail-span-2">
        <Table size="small" rowKey="key" columns={columns(['责任类型', '责任主体', '组织', '状态', '最近确认'])} dataSource={ownership} pagination={false} />
      </WorkPanel>
      <WorkPanel title="依赖归属" className="taf-asset-detail-span-2">
        <Table size="small" rowKey="key" columns={columns(['服务', '依赖关系', '负责人', 'SLA', '状态'])} dataSource={[
          { key: 'a', '服务': '教学管理系统', '依赖关系': '核心/承载系统', '负责人': '张老师', 'SLA': '99.2%', '状态': '已确认' },
          { key: 'b', '服务': '统一身份认证', '依赖关系': '认证依赖', '负责人': '李老师', 'SLA': '99.5%', '状态': '已确认' },
          { key: 'c', '服务': '文件存储系统', '依赖关系': '数据存储依赖', '负责人': '王老师', 'SLA': '98.8%', '状态': '待确认' },
        ]} pagination={false} />
      </WorkPanel>
      <WorkPanel title="归属完整度"><RingChart value={88} height={210} ariaLabel="归属完整度" /></WorkPanel>
    </div>
  );
}

function HistoryDetail() {
  return (
    <div className="taf-asset-detail-layout">
      <WorkPanel title="资产变更时间线" className="taf-asset-detail-span-2">
        <Table size="small" rowKey="key" columns={columns(['时间', '类型', '字段', '操作者', '审计状态'])} dataSource={changes} pagination={{ pageSize: 4, size: 'small' }} />
      </WorkPanel>
      <WorkPanel title="风险变化趋势"><TrendChart title="风险评分 / 暴露资产" /></WorkPanel>
      <WorkPanel title="字段 Diff 对比" className="taf-asset-detail-span-2">
        <Table size="small" rowKey="key" columns={columns(['字段', '变更前', '变更后', '状态'])} dataSource={[
          { key: 'dept', '字段': '部门/归属', '变更前': '网络信息中心', '变更后': '计算中心', '状态': '已审计' },
          { key: 'owner', '字段': '责任人', '变更前': '李老师', '变更后': '张老师', '状态': '已审计' },
          { key: 'group', '字段': '资产组', '变更前': '通用服务器组', '变更后': '计算服务器组', '状态': '已审计' },
          { key: 'risk', '字段': '风险等级', '变更前': '中风险', '变更后': '高风险', '状态': '待确认' },
        ]} pagination={false} />
      </WorkPanel>
      <WorkPanel title="回滚与审批">
        <div className="taf-asset-detail-risk-list"><span>可回滚变更 4 项<StatusTag value="可回滚" /></span><span>审批中变更 1 项<StatusTag value="审批中" /></span><span>未审计变更 2 项<StatusTag value="高风险" /></span></div>
      </WorkPanel>
    </div>
  );
}
