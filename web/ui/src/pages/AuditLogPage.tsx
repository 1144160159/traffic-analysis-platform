import {
  AuditOutlined,
  CheckCircleOutlined,
  DownloadOutlined,
  ExportOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  HistoryOutlined,
  LinkOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, DatePicker, Drawer, Input, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { auditDetailTabSlug, resolveAuditDetailTab } from '@/routes/pageRouteState';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const { RangePicker } = DatePicker;

const diffRows = [
  ['规则名称', '禁止外联-高风险端口', '禁止外联-高风险端口-v1.3'],
  ['优先级', '100', '80'],
  ['动作', '告警', '阻断'],
  ['目的端口', '22,23,3389,445', '22,23,3389,445,5985'],
  ['有效时间', '全天', '全天'],
  ['备注', '-', '新增高风险端口监控'],
];

const chainNodes = [
  ['告警', 'A-2026-0621-7781', '15:10:21', 'risk'],
  ['PCAP', 'P-2026-0621-3342', '15:10:45', 'info'],
  ['规则', 'rule-7f2c19b3d4e', '15:11:02', 'warn'],
  ['模型', 'model-d1ef38b5c', '15:12:18', 'info'],
  ['部署', 'deploy-9a8b7c6e5d4f', '15:13:03', 'ok'],
  ['白名单', 'white-3b2a1c0d9e8f', '15:13:55', 'warn'],
];

const retentionRows = [
  ['日志保留周期', '已用 96 天', '96%'],
  ['归档位置', 'archive-audit / 2026/06/21', '100%'],
  ['完整性校验', '99.67%', '99.67%'],
  ['脱敏状态', '已脱敏 100%', '100%'],
];

const auditLogOverlays: OverlayContract[] = [
  {
    id: 'drawer-audit-operation-detail',
    title: '审计操作详情',
    kind: 'Drawer',
    actionLabel: '操作详情',
    description: '展示审计事件主体、对象、差异、证据链、trace 和保留策略。',
  },
  {
    id: 'modal-audit-export',
    title: '审计材料导出',
    kind: 'Modal',
    actionLabel: '材料导出',
    description: '按查询条件导出审计日志、diff、证据链和脱敏材料。',
    impact: '生成审计导出记录并校验权限、时间窗和脱敏策略。',
  },
];

type AuditAction = {
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
};

export function AuditLogPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedKey, setSelectedKey] = useState<string>();
  const [userFilter, setUserFilter] = useState('全部用户/角色');
  const [tenantFilter, setTenantFilter] = useState('全部租户');
  const [objectFilter, setObjectFilter] = useState('全部');
  const [actionFilter, setActionFilter] = useState('全部');
  const [resultFilter, setResultFilter] = useState('全部');
  const [requestQuery, setRequestQuery] = useState('');
  const [traceQuery, setTraceQuery] = useState('');
  const detailTab = resolveAuditDetailTab(searchParams.get('detail'));
  const [exportFormat, setExportFormat] = useState('PDF');
  const [listPage, setListPage] = useState(1);
  const [action, setAction] = useState<AuditAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildAuditRows(data?.rows ?? []), [data?.rows]);
  const filteredRows = useMemo(() => rows.filter((row) => {
    const objectMatches = objectFilter === '全部' || String(row.对象类型).includes(objectFilter);
    const actionMatches = actionFilter === '全部' || String(row.动作类型).includes(actionFilter);
    const resultMatches = resultFilter === '全部' || String(row.结果).includes(resultFilter);
    const userMatches = userFilter === '全部用户/角色' || String(row['用户/角色']).includes(userFilter.replace('安全管理员', '管理员'));
    const tenantMatches = tenantFilter === '全部租户' || String(row.租户 ?? '').includes(tenantFilter);
    const requestMatches = !requestQuery || String(row.请求ID).toLowerCase().includes(requestQuery.toLowerCase());
    const traceMatches = !traceQuery || String(row.trace_id).toLowerCase().includes(traceQuery.toLowerCase());
    return objectMatches && actionMatches && resultMatches && userMatches && tenantMatches && requestMatches && traceMatches;
  }), [actionFilter, objectFilter, requestQuery, resultFilter, rows, tenantFilter, traceQuery, userFilter]);
  const pageSize = 10;
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const visibleRows = filteredRows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderAuditCell(column, value),
  }));
  function openAction(title: string, target = String(selected?.请求ID ?? 'req-5c7e1ab2f8d3')) {
    setActionSubmitted(false);
    setAction(createAuditAction(title, target));
  }
  function resetSearch() {
    setUserFilter('全部用户/角色');
    setTenantFilter('全部租户');
    setObjectFilter('全部');
    setActionFilter('全部');
    setResultFilter('全部');
    setRequestQuery('');
    setTraceQuery('');
    setListPage(1);
  }
  function setDetailTab(tab: string) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set('detail', auditDetailTabSlug(tab));
      return next;
    });
  }

  return (
    <div className="taf-page taf-auditlog">
      <section className="taf-auditlog-shell">
        <main className="taf-auditlog-main">
          <header className="taf-auditlog-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={8}>
              <Button size="small" icon={<FileSearchOutlined />} onClick={() => openAction('保存查询')}>保存查询</Button>
              <Button size="small" icon={<DownloadOutlined />} onClick={() => openAction('导出取证')}>导出取证</Button>
              <Button size="small" icon={<FileProtectOutlined />} onClick={() => openAction('生成合规证据')}>生成合规证据</Button>
              <Button size="small" type="primary" danger icon={<WarningOutlined />} onClick={() => openAction('触发复核')}>触发复核</Button>
              <Button size="small" type="primary" ghost icon={<SafetyCertificateOutlined />} onClick={() => openAction('归档校验')}>归档校验</Button>
              <Tooltip title="刷新审计日志">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={auditLogOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/audit/logs、APISIX 路由或 alert-service audit_logs 查询。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-auditlog-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-auditlog-workbench">
            <WorkPanel title="日志检索" className="taf-auditlog-filter-panel" extra={<SearchOutlined />}>
              <AuditSearchBar userFilter={userFilter} tenantFilter={tenantFilter} objectFilter={objectFilter} actionFilter={actionFilter} resultFilter={resultFilter} requestQuery={requestQuery} traceQuery={traceQuery} onUserChange={(value) => { setUserFilter(value); setListPage(1); }} onTenantChange={(value) => { setTenantFilter(value); setListPage(1); }} onObjectChange={(value) => { setObjectFilter(value); setListPage(1); }} onActionChange={(value) => { setActionFilter(value); setListPage(1); }} onResultChange={(value) => { setResultFilter(value); setListPage(1); }} onRequestChange={(value) => { setRequestQuery(value); setListPage(1); }} onTraceChange={(value) => { setTraceQuery(value); setListPage(1); }} onReset={resetSearch} onSearch={() => openAction('执行审计查询')} />
            </WorkPanel>

            <WorkPanel title={`审计日志（共 ${filteredRows.length} 条）`} className="taf-auditlog-table-panel" extra={<AuditOutlined />}>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                scroll={{ x: 980, y: 270 }}
                columns={columns}
                dataSource={visibleRows}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
              />
              <div className="taf-auditlog-pagination">
                <span>共 {filteredRows.length} 条</span><button type="button" aria-label="审计日志上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`审计日志第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="审计日志下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span>
              </div>
            </WorkPanel>

            <WorkPanel title="操作详情 / Diff 视图" className="taf-auditlog-detail-panel" extra={<HistoryOutlined />}>
              <AuditDetail selected={selected} activeTab={detailTab} onTabChange={setDetailTab} onAction={openAction} />
            </WorkPanel>

            <div className="taf-auditlog-bottom">
              <WorkPanel title="关联链路（从当前操作追溯业务链路）" extra={<LinkOutlined />}>
                <RelatedChain selected={selected} />
              </WorkPanel>
              <WorkPanel title="操作时间线" extra={<HistoryOutlined />}>
                <OperationTimeline data={data} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="导出取证" extra={<ExportOutlined />}>
                <ExportEvidence format={exportFormat} onFormatChange={setExportFormat} onAction={openAction} />
              </WorkPanel>
            </div>
          </div>
        </main>
      </section>
      <Drawer className="taf-auditlog-action-drawer" title={action ? `${action.title}确认` : '审计操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为审计对象创建“{action.title}”仿真任务，并保留查询条件、租户与审计上下文。</p><dl><dt>审计对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="审计业务操作已进入仿真任务队列" description={`目标：${action.target}；动作：${action.title}`} />}</div>}
      </Drawer>
    </div>
  );
}

function AuditSearchBar({
  userFilter,
  tenantFilter,
  objectFilter,
  actionFilter,
  resultFilter,
  requestQuery,
  traceQuery,
  onUserChange,
  onTenantChange,
  onObjectChange,
  onActionChange,
  onResultChange,
  onRequestChange,
  onTraceChange,
  onReset,
  onSearch,
}: {
  userFilter: string;
  tenantFilter: string;
  objectFilter: string;
  actionFilter: string;
  resultFilter: string;
  requestQuery: string;
  traceQuery: string;
  onUserChange: (value: string) => void;
  onTenantChange: (value: string) => void;
  onObjectChange: (value: string) => void;
  onActionChange: (value: string) => void;
  onResultChange: (value: string) => void;
  onRequestChange: (value: string) => void;
  onTraceChange: (value: string) => void;
  onReset: () => void;
  onSearch: () => void;
}) {
  return (
    <div className="taf-auditlog-search">
      <label><span>用户/角色</span><Select size="small" value={userFilter} options={[{ value: '全部用户/角色' }, { value: '安全管理员' }, { value: '审计员' }, { value: '自动化账号' }]} onChange={onUserChange} /></label>
      <label><span>租户</span><Select size="small" value={tenantFilter} options={[{ value: '全部租户' }, { value: '主园区' }, { value: '实验网' }]} onChange={onTenantChange} /></label>
      <label className="is-wide"><span>时间</span><RangePicker size="small" showTime onChange={onSearch} /></label>
      <label><span>对象类型</span><Select size="small" value={objectFilter} options={[{ value: '全部' }, { value: 'PCAP' }, { value: '规则' }, { value: '模型' }, { value: '令牌' }]} onChange={onObjectChange} /></label>
      <label><span>动作类型</span><Select size="small" value={actionFilter} options={[{ value: '全部' }, { value: '访问' }, { value: '发布' }, { value: '激活' }, { value: '导出' }]} onChange={onActionChange} /></label>
      <label><span>结果</span><Select size="small" value={resultFilter} options={[{ value: '全部' }, { value: '成功' }, { value: '失败' }, { value: '待复核' }]} onChange={onResultChange} /></label>
      <label><span>请求 ID</span><Input size="small" value={requestQuery} onChange={(event) => onRequestChange(event.target.value)} placeholder="请输入请求 ID" /></label>
      <label><span>trace_id</span><Input size="small" value={traceQuery} onChange={(event) => onTraceChange(event.target.value)} placeholder="请输入 trace_id" /></label>
      <div className="taf-auditlog-search-actions">
        <Button size="small" onClick={onReset}>重置</Button>
        <Button size="small" type="primary" icon={<SearchOutlined />} onClick={onSearch}>查询</Button>
        <Button size="small" type="text" onClick={onSearch}>收起</Button>
      </div>
    </div>
  );
}

function AuditDetail({ selected, activeTab, onTabChange, onAction }: { selected?: SnapshotRow; activeTab: string; onTabChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  const resource = String(selected?.对象类型 ?? '规则');
  const action = String(selected?.动作类型 ?? '发布');
  const result = String(selected?.结果 ?? '成功');

  return (
    <div className="taf-auditlog-detail">
      <div className="taf-auditlog-detail-meta">
        <span>对象类型：<b>{resource}</b></span>
        <span>动作类型：<b>{action}</b></span>
        <span>结果：<b className={resultClass(result)}>{result}</b></span>
        <span>对象 ID：<b>{resource.toLowerCase()}-7f2c1a9b3d4e</b></span>
        <span>时间：<b>{String(selected?.时间 ?? '2026-06-21 15:31:55')}</b></span>
      </div>

      <div className="taf-auditlog-detail-tabs">
        {['字段变更对比', '操作上下文', '关联链路'].map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}
      </div>

      {activeTab === '字段变更对比' && (
        <>
          <div className="taf-auditlog-diff">
            <div><span>字段</span><span>操作前（v1.2.3）</span><span>操作后（v1.3.0）</span></div>
            {diffRows.map(([field, before, after]) => (
              <button key={field} type="button" onClick={() => onAction('查看字段变更', field)}>
                <span>{field}</span>
                <span>{before}</span>
                <span className={field === '动作' || field === '优先级' ? 'is-risk' : field === '目的端口' || field === '备注' ? 'is-ok' : ''}>{after}</span>
              </button>
            ))}
          </div>
          <div className="taf-auditlog-sidecards">
            <HighRiskAudit onAction={onAction} />
            <RetentionStatus onAction={onAction} />
          </div>
        </>
      )}
      {activeTab === '操作上下文' && <AuditOperationContext selected={selected} result={result} />}
      {activeTab === '关联链路' && <AuditRelatedChainDetail selected={selected} onAction={onAction} />}
    </div>
  );
}

function AuditOperationContext({ selected, result }: { selected?: SnapshotRow; result: string }) {
  return (
    <div className="taf-auditlog-operation-context" data-audit-detail-state="operation-context">
      <div className="taf-auditlog-context">
        <span>用户：<b>{String(selected?.['用户/角色'] ?? 'sec_analyst')}</b></span>
        <span>租户：<b>{String(selected?.租户 ?? 'campus-main')}</b></span>
        <span>角色：<b>安全运营</b></span>
        <span>来源 IP：<b>10.22.33.44</b></span>
        <span>User-Agent：<b>WebConsole/1.0</b></span>
        <span>请求 ID：<b>{String(selected?.请求ID ?? 'req-5c7e1ab2f8d3')}</b></span>
        <span>trace_id：<b>{String(selected?.trace_id ?? 'trace-b1c642a7d5f8e9f')}</b></span>
        <span>会话 ID：<b>sid-20260621-8842</b></span>
      </div>
      <div className="taf-auditlog-request-chain" aria-label="审计请求链路">
        {['登录态校验', 'RBAC 权限校验', '规则发布 API', 'Kafka rule.updates', '审计落库'].map((step) => <span key={step}>{step}</span>)}
      </div>
      <div className="taf-auditlog-context-summary">
        <span>来源页面：<b>规则管理 / 规则定义 / 发布</b></span>
        <span>上下文校验：<b className={result.includes('失败') ? 'is-risk' : 'is-ok'}>{result.includes('失败') ? '权限门禁拒绝或对象版本冲突' : '同一用户、同一租户、可信办公网段，未触发异常登录'}</b></span>
      </div>
    </div>
  );
}

function AuditRelatedChainDetail({ selected, onAction }: { selected?: SnapshotRow; onAction: (title: string, target?: string) => void }) {
  const relations = [
    ['AL-20260620-000123', '触发规则', '已关联', '06-20 03:43', '查看告警'],
    ['PCAP-20260620-0156', '证据引用', '已归档', '06-20 03:44', '查看证据'],
    ['deploy-rule-20260621', '发布工单', '已发布', '06-21 15:31', '查看部署'],
    ['WL-20260619-0187', '例外影响', '需复审', '06-21 15:32', '查看白名单'],
  ];
  return (
    <div className="taf-auditlog-related-detail" data-audit-detail-state="related-chain">
      <RelatedChain selected={selected} />
      <div className="taf-auditlog-related-table">
        <div><span>关联对象</span><span>关系</span><span>状态</span><span>时间</span><span>跳转</span></div>
        {relations.map(([object, relation, status, time, action]) => (
          <button key={object} type="button" onClick={() => onAction(action, object)}>
            <span>{object}</span><span>{relation}</span><StatusTag value={status} /><span>{time}</span><span>{action}</span>
          </button>
        ))}
      </div>
      <Alert type="info" showIcon message="审计提示" description="本次发布影响 1 条告警、3 份证据、1 个部署任务和 1 条白名单复审项。" />
    </div>
  );
}

function HighRiskAudit({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const rows = [
    ['15:31:02', '模型 / 激活', '高风险', '已复核'],
    ['15:30:41', '脚本 / 执行', '高风险', '待复核'],
    ['15:30:15', '令牌 / 变更', '高风险', '待复核'],
    ['15:28:59', '部署 / 回滚', '高风险', '待复核'],
    ['15:27:41', 'PCAP / 下载', '高风险', '修复'],
  ];
  return (
    <div className="taf-auditlog-risk">
      <h3>高风险审计（近 24h）</h3>
      <div><span>时间</span><span>对象/动作</span><span>风险等级</span><span>复核状态</span></div>
      {rows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button" onClick={() => onAction('查看高风险审计', row[1])}>
          <span>{row[0]}</span>
          <span>{row[1]}</span>
          <span className="is-risk">{row[2]}</span>
          <span className={row[3] === '已复核' ? 'is-ok' : 'is-warn'}>{row[3]}</span>
        </button>
      ))}
    </div>
  );
}

function RetentionStatus({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const trendValues = [[84, 88, 90, 93, 95, 95, 96], [72, 78, 80, 86, 90, 96, 100], [98.8, 99.1, 99.4, 99.2, 99.6, 99.7, 99.67], [92, 94, 96, 98, 99, 100, 100]];
  return (
    <div className="taf-auditlog-retention">
      <h3>留存状态</h3>
      {retentionRows.map(([label, value], index) => (
        <span key={label}>
          <em>{label}</em>
          <b>{value}</b>
          <div className="taf-auditlog-retention-echart"><DataQualityKpiSparklineChart ariaLabel={`审计${label}趋势`} tone={index === 0 ? 'warn' : index === 1 || index === 3 ? 'ok' : 'info'} values={trendValues[index]} /></div>
        </span>
      ))}
      <footer>最后校验：2026-06-21 15:30:10 <button type="button" onClick={() => onAction('查看留存状态')}>详情</button></footer>
    </div>
  );
}

function RelatedChain({ selected }: { selected?: SnapshotRow }) {
  return (
    <div className="taf-auditlog-chain">
      {chainNodes.map(([type, id, time, tone], index) => (
        <span key={id} className={`is-${tone}`}>
          <i>{chainIcon(type)}</i>
          <b>{type}</b>
          <em>{index === 2 ? String(selected?.对象类型 ?? type) : id}</em>
          <small>{time}</small>
        </span>
      ))}
    </div>
  );
}

function OperationTimeline({ data, onAction }: { data?: PageSnapshot; onAction: (title: string, target?: string) => void }) {
  const timeline = data?.timeline?.length ? data.timeline : [];
  const entries = timeline.slice(0, 6);
  return (
    <div className="taf-auditlog-timeline">
      {entries.map((item, index) => (
        <button key={`${item.title}-${index}`} type="button" className={`is-${item.status}`} onClick={() => onAction('查看操作时间线', item.title)}>
          <i />
          <span>{`15:${String(10 + index * 2).padStart(2, '0')}:21`}</span>
          <b>{item.title}</b>
          <em>{item.description}</em>
        </button>
      ))}
    </div>
  );
}

function ExportEvidence({ format, onFormatChange, onAction }: { format: string; onFormatChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-auditlog-export">
      <label><span>时间范围</span><Input size="small" value="2026-06-21 00:00:00 ~ 2026-06-21 23:59:59" readOnly /></label>
      <label><span>对象类型</span><Select size="small" value="全部" options={[{ value: '全部' }, { value: 'PCAP' }, { value: '规则' }, { value: '模型' }]} onChange={(value) => onAction('更新导出对象类型', value)} /></label>
      <label><span>用户/角色</span><Select size="small" value="全部" options={[{ value: '全部' }, { value: '审计员' }, { value: '管理员' }]} onChange={(value) => onAction('更新导出用户角色', value)} /></label>
      <div className="taf-auditlog-export-format">
        {['PDF', 'CSV', 'JSON'].map((item) => <button key={item} type="button" className={item === format ? 'is-active' : ''} onClick={() => onFormatChange(item)}>{item}</button>)}
      </div>
      <Button size="small" type="primary" block icon={<DownloadOutlined />} onClick={() => onAction('导出审计材料', format)}>导出审计材料</Button>
    </div>
  );
}

const renderAuditCell = (column: string, value: unknown) => {
  if (column === '结果' || column === '风险标签') return <StatusTag value={value} />;
  if (column === '用户/角色') return <span className="taf-auditlog-user"><AuditOutlined />{String(value)}</span>;
  if (column === '请求ID' || column === 'trace_id') return <span className="taf-auditlog-code">{String(value)}</span>;
  if (column === '操作') return <span className="taf-auditlog-row-actions">{String(value)}</span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row['请求ID'] ?? row.trace_id ?? JSON.stringify(row));

const buildAuditRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 时间: '2026-06-21 15:31:55', '用户/角色': 'sec_analyst', 对象类型: '规则', 动作类型: '发布', 结果: '成功', 请求ID: 'req-5c7e1ab2f8d3', trace_id: 'trace-b1c642a7d5f8e9f', 风险标签: '中危', 操作: '详情 / 关联 / 复核' }];
  if (source.length >= 30) return source;
  return Array.from({ length: 30 }, (_, index) => index < source.length ? source[index] : {
    ...source[index % source.length],
    请求ID: `${String(source[index % source.length].请求ID ?? 'req')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}`,
    trace_id: `${String(source[index % source.length].trace_id ?? 'trace')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}`,
  });
};

const createAuditAction = (title: string, target: string): AuditAction => ({
  title,
  target,
  endpoint: title.includes('导出') ? '/v1/audit/logs/export' : title.includes('复核') ? '/v1/audit/logs/{id}/review' : '/v1/audit/logs/{id}',
  auditEvent: title.includes('导出') ? 'AUDIT_EVIDENCE_EXPORTED' : title.includes('复核') ? 'AUDIT_REVIEW_TRIGGERED' : 'AUDIT_ACTION_SIMULATED',
});

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '0.00%' : '0',
  delta: 'API',
  status: 'info',
});

const metricText = (data: PageSnapshot | undefined, label: string, fallback: string) =>
  data?.metrics.find((item) => item.label === label)?.value ?? fallback;

const resultClass = (value: string) => {
  if (value.includes('失败')) return 'is-risk';
  if (value.includes('待')) return 'is-warn';
  return 'is-ok';
};

const chainIcon = (type: string) => {
  if (type === '告警') return <WarningOutlined />;
  if (type === 'PCAP') return <FileProtectOutlined />;
  if (type === '规则') return <AuditOutlined />;
  if (type === '模型') return <FileSearchOutlined />;
  if (type === '部署') return <SafetyCertificateOutlined />;
  return <CheckCircleOutlined />;
};
