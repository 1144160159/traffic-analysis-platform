import {
  AuditOutlined,
  BellOutlined,
  CalendarOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  ControlOutlined,
  FileTextOutlined,
  MailOutlined,
  MessageOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SendOutlined,
  SettingOutlined,
  ToolOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Space, Switch, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const channelCards = [
  { name: '邮件', icon: <MailOutlined />, success: '99.32%', latency: '0.8s', failures: '3', enabled: true },
  { name: '短信', icon: <MessageOutlined />, success: '98.12%', latency: '1.6s', failures: '7', enabled: true },
  { name: 'Webhook', icon: <ControlOutlined />, success: '99.06%', latency: '0.5s', failures: '7', enabled: true },
  { name: '企业微信', icon: <BellOutlined />, success: '98.76%', latency: '0.5s', failures: '2', enabled: true },
  { name: '钉钉', icon: <SendOutlined />, success: '98.45%', latency: '1.0s', failures: '6', enabled: true },
  { name: '工单系统', icon: <FileTextOutlined />, success: '97.66%', latency: '2.1s', failures: '9', enabled: true },
];

const escalationSteps: Array<[string, string, ReactNode]> = [
  ['SLA 超时', '15 分钟', <ClockCircleOutlined key="sla" />],
  ['未确认', '30 分钟', <BellOutlined key="unack" />],
  ['处置失败', '30 分钟', <SafetyCertificateOutlined key="fail" />],
  ['重复告警', '3 次 / 30 分钟', <WarningOutlined key="repeat" />],
  ['验收缺口', '24 小时', <FileTextOutlined key="gap" />],
];

const templateRows = [
  ['告警模板', '入侵告警通知模板', 'v2.3', '通用', '2026-06-20 10:21', '启用'],
  ['取证模板', '取证任务通知模板', 'v1.8', '通过', '2026-06-19 16:35', '启用'],
  ['数据质量模板', '数据质量日报模板', 'v2.1', '告警(1)', '2026-06-19 09:14', '启用'],
  ['合规模板', '合规缺口报告模板', 'v1.5', '通过', '2026-06-18 22:47', '启用'],
];

const historyRows = [
  ['2026-06-21 15:34:22', '安全值班组', '邮件', '攻击告警', '成功', '-', '0', 'tr-7c1e3f9b3d4e'],
  ['2026-06-21 15:32:11', '运维管理组', '钉钉', '任务失败', '失败', '渠道限流', '3', 'tr-2b9d6a7e8f11'],
  ['2026-06-21 15:30:55', '三级值班组', 'Webhook', '数据质量', '成功', '-', '0', 'tr-3d2a7b6c9e45'],
  ['2026-06-21 15:28:44', '审计管理员', '工单', '验收缺口', '失败', '凭据失效', '2', 'tr-df4b8d1c2a77'],
  ['2026-06-21 15:26:15', '安全值班组', '企业微信', '系统异常', '成功', '-', '0', 'tr-1a9b2c3d6e66'],
];

const silenceRows = [
  ['核心交换机维护', '主园区', '2026-06-22 22:00', '2026-06-23 02:00', '交换机 / 网络设备', '夜间升级策略', '启用'],
  ['安全平台升级', '主园区', '2026-06-25 00:00', '2026-06-25 04:00', '平台服务 / 探针', '全部策略', '启用'],
  ['分园区维护', '分园区 A', '2026-06-27 01:00', '2026-06-27 05:00', '全部资产', '非紧急策略', '启用'],
];

const notificationOverlays: OverlayContract[] = [
  {
    id: 'modal-notification-channel-edit',
    title: '通知渠道编辑',
    kind: 'Modal',
    actionLabel: '渠道编辑',
    description: '编辑邮件、短信、Webhook、企业微信、钉钉或工单渠道参数。',
    impact: '影响告警、取证、合规和数据质量通知送达。',
    audit: '记录 secret_ref、渠道参数变更和连接测试结果。',
  },
  {
    id: 'modal-notification-template-preview-test',
    title: '通知模板预览测试',
    kind: 'Modal',
    actionLabel: '模板测试',
    description: '预览模板变量、样例数据和测试发送结果。',
  },
  {
    id: 'drawer-notification-silence-rule',
    title: '通知静默规则',
    kind: 'Drawer',
    actionLabel: '静默规则',
    description: '配置维护窗口、资产范围、策略范围和升级例外。',
    impact: '影响指定时间窗内的通知抑制和升级策略。',
  },
];

type NotificationAction = { title: string; target: string; endpoint: string; auditEvent: string };

export function NotificationConfigPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [listPage, setListPage] = useState(1);
  const [action, setAction] = useState<NotificationAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildNotificationRows(data?.rows ?? []), [data?.rows]);
  const pageSize = 6;
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const visibleRows = rows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderNotificationCell(column, value),
  }));
  function openAction(title: string, target = String(selected?.规则 ?? '攻击告警订阅规则')) {
    setActionSubmitted(false);
    setAction(createNotificationAction(title, target));
  }

  return (
    <div className="taf-page taf-notifications">
      <section className="taf-notifications-shell">
        <main className="taf-notifications-main">
          <header className="taf-notifications-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={8}>
              <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => openAction('新增渠道')}>新增渠道</Button>
              <Button size="small" icon={<SendOutlined />} onClick={() => openAction('测试发送')}>测试发送</Button>
              <Button size="small" icon={<CheckCircleOutlined />} onClick={() => openAction('保存订阅策略')}>保存订阅策略</Button>
              <Button size="small" icon={<ClockCircleOutlined />} onClick={() => openAction('新建升级策略')}>新建升级策略</Button>
              <Button size="small" icon={<CalendarOutlined />} onClick={() => openAction('静默窗口')}>静默窗口</Button>
              <Button size="small" icon={<AuditOutlined />} onClick={() => openAction('导入审计')}>导入审计</Button>
              <Tooltip title="刷新通知配置">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={notificationOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/notifications/settings、APISIX 路由、secret_ref 或 alert-service。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-notifications-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-notifications-workbench">
            <WorkPanel title="A. 通知渠道健康" className="taf-notifications-channel-panel" extra={<MailOutlined />}>
              <ChannelHealth onAction={openAction} />
            </WorkPanel>

            <WorkPanel title="B. 订阅规则" className="taf-notifications-rules-panel" extra={<SettingOutlined />}>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                scroll={{ x: 940, y: 160 }}
                columns={columns}
                dataSource={visibleRows}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
              />
              <div className="taf-notifications-pagination"><span>共 {rows.length} 条</span><button type="button" aria-label="订阅规则上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`订阅规则第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="订阅规则下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
            </WorkPanel>

            <WorkPanel title="C. 条件构造器" className="taf-notifications-builder-panel" extra={<ToolOutlined />}>
              <ConditionBuilder selected={selected} onAction={openAction} />
            </WorkPanel>

            <WorkPanel title="D. 升级策略流程" className="taf-notifications-escalation-panel" extra={<WarningOutlined />}>
              <EscalationFlow onAction={openAction} />
            </WorkPanel>

            <WorkPanel title="E. 模板管理" className="taf-notifications-templates-panel" extra={<Button size="small" type="primary" onClick={() => openAction('新建模板')}>新建模板</Button>}>
              <TemplateManager onAction={openAction} />
            </WorkPanel>

            <WorkPanel title="F. 发送历史" className="taf-notifications-history-panel" extra={<HistoryFilters onAction={openAction} />}>
              <SendHistory onAction={openAction} />
            </WorkPanel>

            <WorkPanel title="G. 抑制与静默" className="taf-notifications-silence-panel" extra={<Space size={6}><Button size="small" type="primary" onClick={() => openAction('新建维护窗口')}>新建维护窗口</Button><Button size="small" onClick={() => openAction('导入日历')}>导入日历</Button></Space>}>
              <SilenceWindows onAction={openAction} />
            </WorkPanel>
          </div>
        </main>
      </section>
      <Drawer className="taf-notifications-action-drawer" title={action ? `${action.title}确认` : '通知操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为通知对象创建“{action.title}”仿真任务，并保留渠道、租户与审计上下文。</p><dl><dt>通知对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="通知业务操作已进入仿真任务队列" />}</div>}
      </Drawer>
    </div>
  );
}

function ChannelHealth({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [enabledChannels, setEnabledChannels] = useState(() => Object.fromEntries(channelCards.map((channel) => [channel.name, channel.enabled])));

  return (
    <div className="taf-notifications-channels">
      {channelCards.map((channel) => (
        <div key={channel.name} className="taf-notifications-channel-card">
          <header><i>{channel.icon}</i><b>{channel.name}</b><Switch size="small" checked={enabledChannels[channel.name]} onChange={(checked) => { setEnabledChannels((channels) => ({ ...channels, [channel.name]: checked })); onAction('切换通知渠道', channel.name); }} /></header>
          <span>成功率 <strong>{channel.success}</strong></span>
          <span>延迟 <strong>{channel.latency}</strong></span>
          <span>失败数 <strong>{channel.failures}</strong></span>
          <div className="taf-notifications-channel-echart"><DataQualityKpiSparklineChart ariaLabel={`${channel.name}送达趋势`} tone={Number(channel.failures) > 7 ? 'warn' : 'ok'} values={buildChannelTrend(channel.name, Boolean(enabledChannels[channel.name]))} /></div>
          <footer><button type="button" onClick={() => onAction('测试通知渠道', channel.name)}>测试发送</button></footer>
        </div>
      ))}
    </div>
  );
}

function ConditionBuilder({ selected, onAction }: { selected?: SnapshotRow; onAction: (title: string, target?: string) => void }) {
  const [windowMode, setWindowMode] = useState('时间段');
  const [moreConditions, setMoreConditions] = useState(true);
  const [assetGroup, setAssetGroup] = useState(String(selected?.['资产组/园区'] ?? '核心资产'));
  const [campus, setCampus] = useState('主园区 / 分园区');
  const [startsAt, setStartsAt] = useState('00:00');
  const [endsAt, setEndsAt] = useState('08:00');

  return (
    <div className="taf-notifications-builder">
      <label><span>严重级别</span><div><b>严重</b><b>高危</b><button type="button" onClick={() => onAction('添加严重级别条件')}>+</button></div></label>
      <label><span>告警类型</span><Select size="small" value={String(selected?.告警类型 ?? '攻击告警')} options={[{ value: '攻击告警' }, { value: '数据泄露' }, { value: '异常登录' }]} onChange={(value) => onAction('更新告警类型条件', value)} /></label>
      <label><span>资产组</span><Input size="small" value={assetGroup} onChange={(event) => setAssetGroup(event.target.value)} onBlur={() => onAction('更新资产组条件', assetGroup)} /></label>
      <label><span>园区</span><Input size="small" value={campus} onChange={(event) => setCampus(event.target.value)} onBlur={() => onAction('更新园区条件', campus)} /></label>
      <label><span>时间窗</span><div className="taf-notifications-radio">{['全天', '时间段', '自定义'].map((mode) => <button key={mode} type="button" className={windowMode === mode ? 'is-active' : ''} onClick={() => { setWindowMode(mode); onAction('更新通知时间窗', mode); }}>{mode}</button>)}</div></label>
      <div className="taf-notifications-time"><Input size="small" value={startsAt} onChange={(event) => setStartsAt(event.target.value)} onBlur={() => onAction('更新通知开始时间', startsAt)} /><Input size="small" value={endsAt} onChange={(event) => setEndsAt(event.target.value)} onBlur={() => onAction('更新通知结束时间', endsAt)} /></div>
      <label><span>接收角色</span><div><b>安全值班组</b><button type="button" onClick={() => onAction('选择接收角色')}>+ 选择角色</button></div></label>
      <footer><span>更多条件</span><Switch size="small" checked={moreConditions} onChange={(checked) => { setMoreConditions(checked); onAction('切换更多通知条件'); }} /></footer>
    </div>
  );
}

function EscalationFlow({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [enabled, setEnabled] = useState(true);

  return (
    <div className="taf-notifications-escalation">
      <div className="taf-notifications-policy"><span>策略：夜间升级策略</span><Select size="small" value={enabled ? '启用' : '停用'} options={[{ value: '启用' }, { value: '停用' }]} onChange={(value) => { const nextEnabled = value === '启用'; setEnabled(nextEnabled); onAction('更新升级策略状态', value); }} /><Switch size="small" checked={enabled} onChange={(checked) => { setEnabled(checked); onAction('切换升级策略'); }} /></div>
      <div className="taf-notifications-steps">
        {escalationSteps.map(([title, time, icon]) => (
          <span key={String(title)}>
            <i>{icon}</i>
            <b>{title}</b>
            <em>{time}</em>
            <small>升级至 安全值班组</small>
          </span>
        ))}
      </div>
      <footer><span>策略说明：满足任一阶段条件后，按时间间隔升级，直至人工确认或处置完成。</span><button type="button" onClick={() => onAction('编辑升级策略')}>编辑策略</button></footer>
    </div>
  );
}

function TemplateManager({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [pageSize, setPageSize] = useState('10 条/页');

  return (
    <div className="taf-notifications-templates">
      <div><span>模板类型</span><span>模板名称</span><span>版本</span><span>变量校验</span><span>最近修改</span><span>状态</span><span>操作</span></div>
      {templateRows.map((row) => (
        <button key={row[1]} type="button" onClick={() => onAction('编辑通知模板', row[1])}>
          {row.map((cell, index) => <span key={`${row[1]}-${index}`} className={index === 5 ? 'is-ok' : index === 3 && cell.includes('告警') ? 'is-warn' : ''}>{cell}</span>)}
          <em>编辑 / 预览 / 更多</em>
        </button>
      ))}
      <footer><span>共 4 条</span><Select size="small" value={pageSize} options={[{ value: '10 条/页' }, { value: '20 条/页' }, { value: '50 条/页' }]} onChange={(value) => { setPageSize(value); onAction('更新模板分页', value); }} /></footer>
    </div>
  );
}

function HistoryFilters({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [channel, setChannel] = useState('全部渠道');
  const [status, setStatus] = useState('全部状态');

  return <Space size={6}><Select size="small" value={channel} options={[{ value: '全部渠道' }, { value: '邮件' }, { value: 'Webhook' }, { value: '钉钉' }]} onChange={(value) => { setChannel(value); onAction('筛选发送历史渠道', value); }} /><Select size="small" value={status} options={[{ value: '全部状态' }, { value: '成功' }, { value: '失败' }]} onChange={(value) => { setStatus(value); onAction('筛选发送历史状态', value); }} /></Space>;
}

function SendHistory({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-notifications-history">
      <div><span>时间</span><span>通知对象</span><span>渠道</span><span>告警类型</span><span>状态</span><span>失败原因</span><span>重试次数</span><span>通知ID</span><span>操作</span></div>
      {historyRows.map((row) => (
        <button key={row[7]} type="button" onClick={() => onAction('查看发送历史', row[7])}>
          {row.map((cell, index) => <span key={`${row[7]}-${index}`} className={index === 4 ? statusClass(cell) : ''}>{cell}</span>)}
          <em>详情 / 重试 / 更多</em>
        </button>
      ))}
    </div>
  );
}

function SilenceWindows({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [pageSize, setPageSize] = useState('10 条/页');

  return (
    <div className="taf-notifications-silence">
      <div><span>窗口名称</span><span>园区/范围</span><span>开始时间</span><span>结束时间</span><span>影响范围</span><span>关联策略</span><span>状态</span><span>操作</span></div>
      {silenceRows.map((row) => (
        <button key={row[0]} type="button" onClick={() => onAction('编辑静默窗口', row[0])}>
          {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 6 ? 'is-ok' : ''}>{cell}</span>)}
          <em>编辑 / 禁用 / 更多</em>
        </button>
      ))}
      <footer><span>共 3 条</span><span>已写入 audit_logs</span><Select size="small" value={pageSize} options={[{ value: '10 条/页' }, { value: '20 条/页' }, { value: '50 条/页' }]} onChange={(value) => { setPageSize(value); onAction('更新静默窗口分页', value); }} /></footer>
    </div>
  );
}

const renderNotificationCell = (column: string, value: unknown) => {
  if (column === '状态' || column === '严重级别') return <StatusTag value={value} />;
  if (column === '渠道') return <span className="taf-notifications-channel-cell"><MailOutlined />{String(value)}</span>;
  if (column === '升级策略') return <span className="taf-notifications-policy-cell"><WarningOutlined />{String(value)}</span>;
  if (column === '操作') return <span className="taf-notifications-row-actions">{String(value)}</span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.规则 ?? JSON.stringify(row));

const buildNotificationRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 规则: '攻击告警订阅规则', 告警类型: '攻击告警', '资产组/园区': '核心资产', 渠道: '邮件', 状态: '启用', 操作: '查看 / 编辑' }];
  if (source.length >= 18) return source;
  return Array.from({ length: 18 }, (_, index) => index < source.length ? source[index] : { ...source[index % source.length], 规则: `${String(source[index % source.length].规则 ?? '订阅规则')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}` });
};

const buildChannelTrend = (channel: string, enabled: boolean) => {
  const seed = Array.from(channel).reduce((total, character) => total + character.charCodeAt(0), 0);
  const baseline = enabled ? 94 + seed % 4 : 60 + seed % 8;
  return [baseline - 2, baseline + 1, baseline - 1, baseline + 2, baseline, baseline + 3, baseline + 1];
};

const createNotificationAction = (title: string, target: string): NotificationAction => {
  const actions = pageApiPlans.notifications.actions ?? [];
  const targetsSilenceRule = /静默|维护|日历/.test(title);
  const plan = actions.find((item) => (
    (targetsSilenceRule && item.id.includes('silence')) ||
    (title.includes('测试') && item.id.includes('test'))
  )) ?? actions.find((item) => item.id === 'notification-settings-update');
  return { title, target, endpoint: title.includes('模板') ? '/v1/notifications/templates/{id}' : plan?.endpoint ?? '/v1/notifications/settings', auditEvent: plan?.auditEvent ?? 'NOTIFICATION_ACTION_SIMULATED' };
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: '0',
  delta: 'API',
  status: 'info',
});

const metricValue = (data: PageSnapshot | undefined, label: string, fallback: string) =>
  data?.metrics.find((metric) => metric.label === label)?.value ?? fallback;

const statusClass = (value: string) => {
  if (value.includes('失败')) return 'is-risk';
  if (value.includes('待') || value.includes('告警')) return 'is-warn';
  return 'is-ok';
};
