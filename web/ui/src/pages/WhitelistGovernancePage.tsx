import {
  AuditOutlined,
  BellOutlined,
  ClockCircleOutlined,
  EditOutlined,
  EyeOutlined,
  FieldTimeOutlined,
  FileSearchOutlined,
  FilterOutlined,
  LinkOutlined,
  LockOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const builderTabs = ['IP', '资产', '账号', '域名', '规则', '模型'];

const approvalSteps: Array<{ title: string; caption: string; icon: ReactNode; tone: string }> = [
  { title: '申请', caption: '安全运营', icon: <PlusOutlined />, tone: 'ok' },
  { title: '影响评估', caption: '覆盖 23 资产', icon: <FileSearchOutlined />, tone: 'info' },
  { title: '审批', caption: 'sec_analyst', icon: <AuditOutlined />, tone: 'warn' },
  { title: '定期复核', caption: '30 天', icon: <ClockCircleOutlined />, tone: 'info' },
  { title: '到期策略', caption: '停用并审计', icon: <FieldTimeOutlined />, tone: 'info' },
];

const expiryRows = [
  ['IP', '10.****.88', '办公网', '2026-06-24', '安全运营', '中', '延期 / 停用'],
  ['域名', 'downloads.campus.local', '全网', '2026-06-25', '平台团队', '中', '延期 / 停用'],
  ['账号', 'temp_admin', '管理平台', '2026-06-26', '(未归属)', '高', '指派角色 / 停用'],
  ['规则', 'Rule-100324', '全网', '长期', '安全运营', '高', '转审计'],
];

const feedbackRows = [
  ['AL-20260619-0187', 'DNS 异常', 'R-DNS异常', 'M-登录异常', '域名例外', '影响评估中'],
  ['AL-20260618-0451', '端口扫描', 'R-端口扫描', '-', 'IP例外', '已生效'],
  ['AL-20260617-0322', '登录失败', 'R-登录失败', 'M-登录异常', '账号例外', '待审批'],
];

const impactRows = [
  ['办公网-终端', '12', 'M-登录异常', '142'],
  ['业务系统-应用', '8', 'R-登录失败', '86'],
  ['运维管理-服务', '6', 'R-端口扫描', '54'],
  ['数据库-中间件', '5', 'R-协议异常', '21'],
  ['测试环境-节点', '3', 'R-弱口令', '12'],
];

const whitelistOverlays: OverlayContract[] = [
  {
    id: 'modal-whitelist-add',
    title: '添加白名单',
    kind: 'Modal',
    actionLabel: '添加白名单',
    description: '创建 IP、资产、账号、域名、规则或模型白名单，并绑定有效期和审批人。',
    impact: '影响后续检测命中、误报压降和审计复核。',
    audit: '记录白名单对象、来源告警、审批链和过期策略。',
  },
  {
    id: 'drawer-whitelist-approval',
    title: '白名单审批详情',
    kind: 'Drawer',
    actionLabel: '审批详情',
    description: '展示白名单申请、影响评估、审批记录、复核周期和到期策略。',
  },
];

type WhitelistAction = {
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
};

export function WhitelistGovernancePage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [builderTab, setBuilderTab] = useState('域名');
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [typeFilter, setTypeFilter] = useState('对象类型');
  const [query, setQuery] = useState('');
  const [listPage, setListPage] = useState(1);
  const [expiryTab, setExpiryTab] = useState('即将到期（7天内）');
  const [approvalExpanded, setApprovalExpanded] = useState(false);
  const [action, setAction] = useState<WhitelistAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildWhitelistRows(data?.rows ?? []), [data?.rows]);
  const filteredRows = useMemo(() => rows.filter((row) => {
    const typeMatches = typeFilter === '对象类型' || String(row.对象类型).includes(typeFilter);
    const statusMatches = statusFilter === '全部状态'
      || (statusFilter === '即将到期'
        ? String(row.有效期 ?? '').includes('2026-06')
        : String(row.状态).includes(statusFilter));
    const text = `${row.匹配条件 ?? ''} ${row.来源告警 ?? ''}`.toLowerCase();
    return typeMatches && statusMatches && (!query || text.includes(query.toLowerCase()));
  }), [query, rows, statusFilter, typeFilter]);
  const pageSize = 5;
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const visibleRows = filteredRows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderWhitelistCell(column, value, record, openAction),
  }));
  function openAction(title: string, target = String(selected?.匹配条件 ?? '当前白名单')) {
    setActionSubmitted(false);
    setAction(createWhitelistAction(title, target));
  }

  return (
    <div className="taf-page taf-whitelist">
      <section className="taf-whitelist-shell">
        <main className="taf-whitelist-main">
          <header className="taf-whitelist-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => openAction('新增白名单')}>新增白名单</Button>
              <Button size="small" icon={<BellOutlined />} onClick={() => openAction('从告警生成草案', 'AL-20260619-0187')}>从告警生成草案</Button>
              <Button size="small" icon={<AuditOutlined />} onClick={() => openAction('提交审批')}>提交审批</Button>
              <Button size="small" icon={<ClockCircleOutlined />} onClick={() => openAction('批量延期')}>批量延期</Button>
              <Button size="small" danger ghost icon={<StopOutlined />} onClick={() => openAction('停用')}>停用</Button>
              <Button size="small" icon={<SaveOutlined />} onClick={() => openAction('转审计')}>转审计</Button>
              <Tooltip title="刷新白名单目录">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={whitelistOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/whitelist、APISIX 路由或 alert-service 白名单仓储。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-whitelist-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-whitelist-workbench">
            <section className="taf-whitelist-left">
              <WorkPanel title="A. 白名单列表" extra={<Button size="small" icon={<FilterOutlined />} onClick={() => openAction('保存筛选条件')}>筛选</Button>}>
                <div className="taf-whitelist-filterbar">
                  <Select aria-label="白名单状态筛选" size="small" value={statusFilter} options={[{ value: '全部状态' }, { value: '生效' }, { value: '待审批' }, { value: '即将到期' }]} onChange={(value) => { setStatusFilter(value); setListPage(1); }} />
                  <Select aria-label="白名单对象类型筛选" size="small" value={typeFilter} options={[{ value: '对象类型' }, { value: 'IP' }, { value: '域名' }, { value: '规则' }, { value: '模型' }]} onChange={(value) => { setTypeFilter(value); setListPage(1); }} />
                  <Input aria-label="搜索白名单" size="small" prefix={<SearchOutlined />} value={query} onChange={(event) => { setQuery(event.target.value); setListPage(1); }} placeholder="搜索匹配条件/来源告警ID" />
                  <Button size="small" icon={<SettingOutlined />} aria-label="配置白名单列表" onClick={() => openAction('配置白名单列表')} />
                </div>
                <Table
                  rowKey={rowKey}
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  scroll={{ x: 920 }}
                  columns={columns}
                  dataSource={visibleRows}
                  rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                  onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
                />
                <div className="taf-whitelist-pagination"><span>共 {filteredRows.length} 条</span><button type="button" aria-label="白名单上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`白名单第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="白名单下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
              </WorkPanel>

              <WorkPanel title="E. 到期治理">
                <ExpiryGovernance activeTab={expiryTab} onTabChange={setExpiryTab} onAction={openAction} />
              </WorkPanel>
            </section>

            <section className="taf-whitelist-center">
              <WorkPanel title="B. 条件构造器 / 新增白名单草案" extra={<span className="taf-whitelist-selected">当前：{String(selected?.匹配条件 ?? 'update.campus.local')}</span>}>
                <ConditionBuilder activeTab={builderTab} onTabChange={setBuilderTab} />
              </WorkPanel>
              <WorkPanel title="F. 反馈关联（从告警到白名单草案）">
                <FeedbackLink onAction={openAction} />
              </WorkPanel>
            </section>

            <aside className="taf-whitelist-right">
              <WorkPanel title="C. 审批流程状态机" extra={<Button size="small" type="text" aria-pressed={approvalExpanded} onClick={() => setApprovalExpanded((value) => !value)}>{approvalExpanded ? '收起' : '展开'}</Button>}>
                <ApprovalFlow expanded={approvalExpanded} />
              </WorkPanel>
              <WorkPanel title="D. 命中监控（近7天）">
                <HitMonitor data={data} />
              </WorkPanel>
              <WorkPanel title="G. 影响矩阵 / 来源链路卡">
                <ImpactMatrix onAction={openAction} />
              </WorkPanel>
            </aside>
          </div>
        </main>
      </section>
      <Drawer className="taf-whitelist-action-drawer" title={action ? `${action.title}确认` : '白名单操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为白名单对象创建“{action.title}”仿真任务，并保留审批、租户和审计上下文。</p><dl><dt>操作对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="白名单业务操作已进入仿真任务队列" description={`目标：${action.target}；动作：${action.title}`} />}</div>}
      </Drawer>
    </div>
  );
}

function ConditionBuilder({ activeTab, onTabChange }: { activeTab: string; onTabChange: (value: string) => void }) {
  return (
    <div className="taf-whitelist-builder">
      <div className="taf-whitelist-builder-tabs">
        {builderTabs.map((tab) => (
          <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>
        ))}
      </div>
      <div className="taf-whitelist-form">
        <label><span>{activeTab}</span><Input size="small" value={activeTab === '域名' ? 'update.campus.local' : activeTab === 'IP' ? '10.12.4.23' : `${activeTab}-例外对象`} readOnly /></label>
        <label><span>匹配方式</span><Select size="small" value="等于" options={[{ value: '等于' }, { value: '包含' }, { value: 'CIDR' }]} /></label>
        <label><span>生效范围</span><Select size="small" value="全网" options={[{ value: '全网' }, { value: '办公网' }, { value: '研发网络' }]} /></label>
        <label><span>时间窗</span><Input size="small" value="2026-06-10 ~ 2026-07-10" readOnly /></label>
        <label className="is-wide"><span>例外原因</span><Input size="small" value="业务系统自动更新，触发 DNS 异常告警（误报）" readOnly /></label>
        <label><span>关联来源</span><Input size="small" value="AL-20260619-0187" readOnly /></label>
        <label><span>责任角色</span><Select size="small" value="安全运营" options={[{ value: '安全运营' }, { value: '平台团队' }]} /></label>
      </div>
      <div className="taf-whitelist-chips">
        <span>DNS异常</span><span>系统更新</span><span>误报</span><b>覆盖告警 128</b><b>影响资产 23</b><b className="is-warn">潜在漏报风险 中</b>
      </div>
    </div>
  );
}

function ApprovalFlow({ expanded }: { expanded: boolean }) {
  return (
    <div className="taf-whitelist-approval">
      <div className="taf-whitelist-approval-steps">
        {approvalSteps.map((step, index) => (
          <span key={step.title} className={`taf-whitelist-approval-step is-${step.tone} ${index === 1 ? 'is-current' : ''}`}>
            <i>{step.icon}</i>
            <b>{index + 1}</b>
            <em>{step.title}</em>
            <small>{step.caption}</small>
          </span>
        ))}
      </div>
      {expanded && <div className="taf-whitelist-approval-meta">
        <span>申请角色 <b>安全运营</b></span>
        <span>审批角色 <b>sec_analyst</b></span>
        <span>风险说明 <b>覆盖 DNS 异常告警，域名每日访问同源稳定</b></span>
        <span>到期策略 <b>到期后自动停用并产生复核任务</b></span>
      </div>}
    </div>
  );
}

function HitMonitor({ data }: { data?: PageSnapshot }) {
  const evidence = data?.evidence ?? [];
  return (
    <div className="taf-whitelist-hit">
      <div className="taf-whitelist-hit-metrics">
        <span><em>命中次数</em><b>1,218</b><small>↑ 9.2%</small></span>
        <span><em>覆盖告警</em><b>{evidence.find((item) => item.label === '命中监控')?.value ?? '128 条'}</b><small className="is-risk">↑ 14.6%</small></span>
        <span><em>覆盖资产</em><b>23</b><small>↓ 4.2%</small></span>
        <span><em>潜在漏报风险</em><b className="is-warn">中</b><small>待复核</small></span>
      </div>
      <div className="taf-whitelist-hit-lower">
        <div>
          <strong>命中趋势（次）</strong>
          <div className="taf-whitelist-hit-echart"><DataQualityTrendChart ariaLabel="白名单命中趋势" categories={['D-6', 'D-5', 'D-4', 'D-3', 'D-2', 'D-1', '今天']} series={[{ name: '命中次数', color: '#18a8ff', values: [142, 166, 151, 192, 178, 214, 175], area: true }]} /></div>
        </div>
        <div>
          <strong>覆盖维度（规则 x 模型）</strong>
          <div className="taf-whitelist-hit-echart"><DataQualityTrendChart ariaLabel="白名单覆盖维度" categories={['DNS', '登录', '端口', '协议', '弱口令']} series={[{ name: '规则命中', color: '#ffb020', values: [45, 25, 20, 14, 9], type: 'bar' }, { name: '模型命中', color: '#65d86e', values: [32, 18, 14, 10, 6], type: 'bar' }]} /></div>
        </div>
      </div>
    </div>
  );
}

function ExpiryGovernance({ activeTab, onTabChange, onAction }: { activeTab: string; onTabChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  const tabs = ['即将到期（7天内）', '过期未处理', '长期生效（>180天）', '未归属责任角色'];
  return (
    <div className="taf-whitelist-expiry">
      <div className="taf-whitelist-expiry-tabs">{tabs.map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}</div>
      <div className="taf-whitelist-expiry-table">
        <div><span>对象类型</span><span>匹配条件</span><span>生效范围</span><span>到期时间</span><span>责任角色</span><span>风险等级</span><span>操作</span></div>
        {expiryRows.map((row) => (
          <button key={`${row[0]}-${row[1]}`} type="button" onClick={() => onAction('处理到期白名单', row[1])}>
            {row.map((cell, index) => <span key={`${row[1]}-${index}`} className={index === 5 ? riskClass(cell) : ''}>{cell}</span>)}
          </button>
        ))}
      </div>
      <footer>查看全部 27 项</footer>
    </div>
  );
}

function FeedbackLink({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-whitelist-feedback">
      <div className="taf-whitelist-chain">
        <span className="is-risk"><b>告警</b><em>AL-20260619-0187</em><small>DNS 异常</small></span>
        <LinkOutlined />
        <span className="is-info"><b>规则 / 模型</b><em>R-DNS异常</em><small>M-登录异常</small></span>
        <LinkOutlined />
        <span className="is-warn"><b>白名单草案</b><em>域名</em><small>update.campus.local</small></span>
        <LinkOutlined />
        <span className="is-info"><b>审批</b><em>影响评估中</em><small>待复核</small></span>
      </div>
      <div className="taf-whitelist-feedback-table">
        <div><span>最近关联链路</span><span>告警类型</span><span>规则</span><span>模型</span><span>草案类型</span><span>状态</span></div>
        {feedbackRows.map((row) => (
          <button key={row[0]} type="button" onClick={() => onAction('查看反馈关联', row[0])}>
            {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 5 ? statusClass(cell) : ''}>{cell}</span>)}
          </button>
        ))}
      </div>
      <footer>建议动作：<button type="button" onClick={() => onAction('回到告警', 'AL-20260619-0187')}>回到告警</button><button type="button" onClick={() => onAction('回到规则管理', 'R-DNS异常')}>回到规则管理</button><button type="button" onClick={() => onAction('调整范围')}>调整范围</button><button type="button" onClick={() => onAction('撤销白名单草案')}>撤销</button></footer>
    </div>
  );
}

function ImpactMatrix({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-whitelist-impact">
      <div className="taf-whitelist-impact-table">
        <div><span>覆盖资产 Top5</span><span>数量</span><span>影响规则 Top5</span><span>覆盖告警</span></div>
        {impactRows.map((row) => (
          <button key={row[0]} type="button" onClick={() => onAction('查看影响矩阵', row[0])}>{row.map((cell, index) => <span key={`${row[0]}-${index}`}>{cell}</span>)}</button>
        ))}
      </div>
      <div className="taf-whitelist-donut">
        <i><b>总计</b><strong>128</strong></i>
        <span><em className="is-risk" />DNS(45%)</span>
        <span><em className="is-info" />登录(25%)</span>
        <span><em className="is-ok" />端口(20%)</span>
        <span><em />其他(10%)</span>
      </div>
    </div>
  );
}

const renderWhitelistCell = (column: string, value: unknown, row: SnapshotRow, onAction: (title: string, target?: string) => void) => {
  if (column === '对象类型') return <span className="taf-whitelist-type"><SafetyCertificateOutlined />{String(value)}</span>;
  if (column === '匹配条件') return <span className="taf-whitelist-match"><LockOutlined />{String(value)}</span>;
  if (column === '来源告警') return <span className="taf-whitelist-source"><BellOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return <span className="taf-whitelist-row-actions"><Button type="text" size="small" icon={<EyeOutlined />} aria-label="查看白名单详情" onClick={(event) => { event.stopPropagation(); onAction('查看白名单详情', rowKey(row)); }} /><Button type="text" size="small" icon={<EditOutlined />} aria-label="编辑白名单" onClick={(event) => { event.stopPropagation(); onAction('编辑白名单', rowKey(row)); }} /><Button type="text" size="small" icon={<ClockCircleOutlined />} aria-label="延期白名单" onClick={(event) => { event.stopPropagation(); onAction('延期白名单', rowKey(row)); }} /></span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.匹配条件 ?? row.对象 ?? JSON.stringify(row));

const buildWhitelistRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 对象类型: '域名', 匹配条件: 'update.campus.local', 生效范围: '全网', 有效期: '2026-06-10 ~ 2026-07-10', 责任角色: '安全运营', 来源告警: 'AL-20260619-0187', 状态: '生效', 操作: '查看 / 编辑 / 延期' }];
  if (source.length >= 20) return source;
  return Array.from({ length: 20 }, (_, index) => index < source.length ? source[index] : { ...source[index % source.length], 匹配条件: `${String(source[index % source.length].匹配条件)}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}` });
};

const createWhitelistAction = (title: string, target: string): WhitelistAction => {
  const actions = pageApiPlans.whitelist.actions ?? [];
  const plan = actions.find((item) =>
    (title.includes('新增') || title.includes('草案')) && item.id === 'whitelist-create'
    || title.includes('审批') && item.id === 'whitelist-submit-approval'
    || title.includes('延期') && item.id === 'whitelist-extend'
    || (title.includes('停用') || title.includes('撤销')) && item.id === 'whitelist-disable',
  );
  return { title, target, endpoint: plan?.endpoint ?? '/v1/whitelist/{id}', auditEvent: plan?.auditEvent ?? 'WHITELIST_ACTION_SIMULATED' };
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('风险') ? '0 项' : '0',
  delta: 'API',
  status: 'info',
});

const statusClass = (value: string) => {
  if (value.includes('待')) return 'is-warn';
  if (value.includes('生效')) return 'is-ok';
  return 'is-info';
};

const riskClass = (value: string) => {
  if (value.includes('高')) return 'is-risk';
  if (value.includes('中')) return 'is-warn';
  return 'is-ok';
};
