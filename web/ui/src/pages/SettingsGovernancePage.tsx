import {
  ApiOutlined,
  AuditOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  ExperimentOutlined,
  EyeOutlined,
  FileProtectOutlined,
  KeyOutlined,
  LockOutlined,
  NodeIndexOutlined,
  PartitionOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SecurityScanOutlined,
  SettingOutlined,
  TeamOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const tenantNodes = [
  ['华东园区（主租户）', '已隔离', 0, 'site'],
  ['核心园区', '已隔离', 1, 'campus'],
  ['分园区A', '已隔离', 1, 'campus'],
  ['分园区B', '已隔离', 1, 'campus'],
  ['研发网络', '隔离', 1, 'folder'],
  ['教学网络', '隔离', 2, 'folder'],
  ['测试网段', '隔离', 2, 'folder'],
  ['办公网段', '隔离', 2, 'folder'],
  ['生产网段', '已隔离', 1, 'folder'],
  ['华南园区', '部分隔离', 0, 'site'],
  ['海外园区', '未隔离', 0, 'site'],
];

const roleRows = [
  { role: '安全值班员', cells: ['ok', 'locked', 'locked', 'locked', 'locked', 'ok', 'locked'] },
  { role: '研判员', cells: ['ok', 'ok', 'locked', 'locked', 'pending', 'ok', 'locked'] },
  { role: '管理员', cells: ['ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok'] },
  { role: '审计员', cells: ['ok', 'locked', 'ok', 'pending', 'locked', 'ok', 'ok'] },
  { role: '只读大屏账号', cells: ['ok', 'locked', 'locked', 'locked', 'locked', 'locked', 'locked'] },
];

const permissionColumns = ['告警查看', 'PCAP访问', '规则发布', '模型激活', '脚本执行', '证据导出', '系统配置'];

const retentionRows = [
  ['Flow', '90 天', '正常', '2026-09-19 删除'],
  ['Session', '180 天', '正常', '2026-12-18 删除'],
  ['Alert', '365 天', '正常', '2027-06-20 删除'],
  ['Evidence', '3 年', '正常', '2029-06-20 归档'],
  ['PCAP', '30 天', '即将到期', '2026-07-21 删除'],
  ['Audit', '5 年', '正常', '2031-06-20 归档'],
];

const integrations = [
  ['Keycloak', <UserSwitchOutlined key="keycloak" />, '健康', '2026-06-21 14:31'],
  ['APISIX', <ApiOutlined key="apisix" />, '健康', '2026-06-21 14:30'],
  ['Kafka', <PartitionOutlined key="kafka" />, '健康', '2026-06-21 14:32'],
  ['MinIO', <CloudServerOutlined key="minio" />, '健康', '2026-06-21 14:29'],
  ['OpenSearch', <ExperimentOutlined key="opensearch" />, '健康', '2026-06-21 14:33'],
  ['NebulaGraph', <NodeIndexOutlined key="nebula" />, '健康', '2026-06-21 14:28'],
  ['Webhook', <ClusterOutlined key="webhook" />, '健康', '2026-06-21 14:27'],
];

const securityParams = [
  ['登录策略', 'SSO 强制登录', <SafetyCertificateOutlined key="login" />],
  ['密码策略', '强度：高 / 90 天', <LockOutlined key="password" />],
  ['MFA', '已启用', <KeyOutlined key="mfa" />],
  ['IP 访问控制', '已配置 24 条', <SecurityScanOutlined key="ip" />],
  ['脱敏策略', '中等脱敏', <ControlOutlined key="mask" />],
  ['时间窗默认值', '最近 24 小时', <ClockCircleOutlined key="time" />],
  ['告警阈值', '默认策略', <SettingOutlined key="threshold" />],
  ['页面刷新频率', '30 秒', <ReloadOutlined key="refresh" />],
  ['大屏脱敏', '已启用', <EyeOutlined key="screen" />],
  ['功能开关', '12 项已启用', <DeploymentUnitOutlined key="feature" />],
];

const loopActions = [
  ['同步资产和权限范围', <ReloadOutlined key="sync" />],
  ['保存并写审计', <SaveOutlined key="save" />],
  ['创建令牌', <KeyOutlined key="create" />],
  ['轮换令牌', <ControlOutlined key="rotate" />],
  ['吊销令牌', <LockOutlined key="revoke" />],
  ['更新生命周期策略', <DatabaseOutlined key="retention" />],
  ['连接测试', <ApiOutlined key="test" />],
  ['触发安全审计', <AuditOutlined key="audit" />],
  ['提示配置影响范围', <FileProtectOutlined key="impact" />],
];

const settingsOverlays: OverlayContract[] = [
  {
    id: 'modal-settings-token',
    title: '创建 API 令牌',
    kind: 'Modal',
    actionLabel: '令牌配置',
    description: '创建 API 令牌，配置作用域、有效期、IP 访问控制和脱敏策略。',
    impact: '影响系统 API 访问边界，令牌明文仅创建时展示一次。',
    audit: '记录 token scope、租户、创建人、过期时间和 secret_ref。',
  },
  {
    id: 'popconfirm-settings-token-revoke',
    title: 'API 令牌吊销确认',
    kind: 'Popconfirm',
    actionLabel: '吊销确认',
    description: '确认吊销选中的 API 令牌并立即阻断后续访问。',
    impact: '可能影响集成系统、脚本任务和自动化调用。',
    danger: true,
  },
];

export function SettingsGovernancePage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selected = useMemo(() => rows.find((row) => settingsRowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderSettingsCell(column, value),
  }));

  return (
    <div className="taf-page taf-settings-page">
      <section className="taf-settings-shell">
        <main className="taf-settings-main">
          <header className="taf-settings-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={8}>
              <Button size="small" type="primary" icon={<SaveOutlined />}>保存配置</Button>
              <Button size="small" icon={<ApiOutlined />}>连接测试</Button>
              <Button size="small" icon={<KeyOutlined />}>创建令牌</Button>
              <Button size="small" icon={<ControlOutlined />}>轮换令牌</Button>
              <Button size="small" icon={<AuditOutlined />}>触发安全审计</Button>
              <Button size="small" icon={<EyeOutlined />}>查看影响范围</Button>
              <Tooltip title="刷新系统设置">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={settingsOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/tokens/scopes、/v1/tokens、APISIX 路由或 auth-service。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-settings-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-settings-workbench">
            <WorkPanel title="A. 租户与站点" className="taf-settings-tenant-panel" extra={<TeamOutlined />}>
              <TenantTree />
            </WorkPanel>

            <WorkPanel title="B. RBAC 权限矩阵" className="taf-settings-rbac-panel" extra={<SecurityScanOutlined />}>
              <RbacMatrix />
            </WorkPanel>

            <WorkPanel title="C. API 令牌" className="taf-settings-token-panel" extra={<KeyOutlined />}>
              <Table
                rowKey={settingsRowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                columns={columns}
                dataSource={rows.slice(0, 5)}
                rowSelection={{ selectedRowKeys: selected ? [settingsRowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(settingsRowKey(record)) })}
              />
              <div className="taf-settings-token-footer"><span>共 {metricValue(data, '有效令牌', '46')} 个有效令牌</span><span>{selected?.令牌名称 ?? '未选择'} 已关联审计</span><Select size="small" value="10 条/页" options={[{ value: '10 条/页' }]} /></div>
            </WorkPanel>

            <WorkPanel title="D. 数据留存策略" className="taf-settings-retention-panel" extra={<DatabaseOutlined />}>
              <RetentionPolicy />
            </WorkPanel>

            <WorkPanel title="E. 集成配置健康" className="taf-settings-integration-panel" extra={<ApiOutlined />}>
              <IntegrationHealth />
            </WorkPanel>

            <WorkPanel title="F. 安全策略与系统参数" className="taf-settings-security-panel" extra={<SettingOutlined />}>
              <SecurityParams />
            </WorkPanel>

            <WorkPanel title="G. 闭环动作入口" className="taf-settings-loop-panel" extra={<AuditOutlined />}>
              <LoopActions />
            </WorkPanel>
          </div>
        </main>
      </section>
    </div>
  );
}

function TenantTree() {
  return (
    <div className="taf-settings-tenant-tree">
      <header><span>名称</span><span>隔离范围 / 状态</span></header>
      {tenantNodes.map(([name, status, level, type], index) => (
        <button key={`${name}-${index}`} type="button" className={status === '未隔离' ? 'is-risk' : status === '部分隔离' ? 'is-warn' : ''} style={{ '--indent': String(level) } as React.CSSProperties}>
          <span><i>{type === 'site' ? <ClusterOutlined /> : <DatabaseOutlined />}</i>{name}</span>
          <em>{status}</em>
        </button>
      ))}
    </div>
  );
}

function RbacMatrix() {
  return (
    <div className="taf-settings-rbac">
      <div className="taf-settings-rbac-head">
        <span>角色</span>
        {permissionColumns.map((column) => <span key={column}>{column}</span>)}
      </div>
      {roleRows.map((row) => (
        <div key={row.role} className="taf-settings-rbac-row">
          <b>{row.role}</b>
          {row.cells.map((cell, index) => (
            <span key={`${row.role}-${permissionColumns[index]}`} className={`is-${cell}`}>
              {cell === 'ok' ? <CheckCircleOutlined /> : cell === 'pending' ? <ClockCircleOutlined /> : <LockOutlined />}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}

function RetentionPolicy() {
  return (
    <div className="taf-settings-retention">
      <header><span>数据类型</span><span>保留周期</span><span>生命周期状态</span><span>下一步动作</span></header>
      {retentionRows.map((row) => (
        <div key={row[0]} className={row[2].includes('即将') ? 'is-warn' : ''}>
          {row.map((cell) => <span key={`${row[0]}-${cell}`}>{cell}</span>)}
        </div>
      ))}
    </div>
  );
}

function IntegrationHealth() {
  return (
    <div className="taf-settings-integrations">
      <header><span>集成组件</span><span>连接状态</span><span>最近测试</span><span>配置入口</span></header>
      {integrations.map(([name, icon, status, testedAt]) => (
        <button key={String(name)} type="button">
          <span><i>{icon}</i>{name}</span>
          <b>{status}</b>
          <span>{testedAt}</span>
          <em>配置</em>
        </button>
      ))}
    </div>
  );
}

function SecurityParams() {
  return (
    <div className="taf-settings-security">
      {securityParams.map(([label, value, icon]) => (
        <div key={String(label)}>
          <i>{icon}</i>
          <span>{label}</span>
          <b>{value}</b>
        </div>
      ))}
    </div>
  );
}

function LoopActions() {
  return (
    <div className="taf-settings-loop-actions">
      {loopActions.map(([label, icon], index) => (
        <button key={String(label)} type="button" className={index === 4 ? 'is-risk' : index === 7 || index === 8 ? 'is-warn' : ''}>
          <i>{icon}</i>
          <span>{label}</span>
        </button>
      ))}
    </div>
  );
}

const renderSettingsCell = (column: string, value: unknown) => {
  if (column === '轮换状态') return <StatusTag value={value} />;
  if (column === '令牌指纹') return <span className="taf-settings-token-fingerprint">{String(value)}</span>;
  if (column === '操作') return <span className="taf-settings-token-actions">{String(value)}</span>;
  return String(value);
};

const settingsRowKey = (row: SnapshotRow) => String(row.令牌名称 ?? JSON.stringify(row));

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: '0',
  delta: 'API',
  status: 'info',
});

const metricValue = (data: PageSnapshot | undefined, label: string, fallback: string) =>
  data?.metrics.find((metric) => metric.label === label)?.value ?? fallback;
