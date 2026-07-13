import {
  ApiOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloudUploadOutlined,
  CodeOutlined,
  DownloadOutlined,
  EditOutlined,
  FilterOutlined,
  HistoryOutlined,
  ImportOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { CloseableConfirmButton } from '@/components/CloseableConfirmButton';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const ruleConditions = [
  ['协议 (proto)', '在', 'TLS_SSH'],
  ['JA3 指纹 (ja3_score)', '大于', '0.82'],
  ['目的IP信誉 (dst_reputation)', '等于', 'high'],
  ['出站流量 P95 (bytes_out_p95)', '大于', '5 MB'],
];

const sampleRows = [
  ['c2_tunnel_01.pcap', '12.4 MB', '探针-01', '06-19 22:18'],
  ['c2_tunnel_02.pcap', '8.7 MB', '探针-07', '06-19 19:41'],
  ['c2_tunnel_03.pcap', '15.2 MB', '探针-03', '06-19 16:02'],
  ['c2_tunnel_04.pcap', '6.3 MB', '探针-12', '06-19 13:37'],
];

const matrixRows = [
  ['C2_Tunnel_v3', '1,142', '4', '18,932', '8', '0.35%'],
  ['Lateral_Move_v2', '837', '3', '19,102', '6', '0.25%'],
  ['DNS_Tunnel_v2', '598', '3', '19,144', '4', '0.33%'],
  ['Data_Exfil_v1', '1,034', '5', '18,706', '7', '0.48%'],
];

const fpRows = [
  ['fp_20260619_001', '7', '正常流量', '探针-05'],
  ['fp_20260619_002', '5', '软件更新', '探针-02'],
  ['fp_20260618_009', '4', '备份同步', '探针-09'],
  ['fp_20260618_015', '3', '远程管理', '探针-11'],
];

const whitelistRows = [
  ['10.12.2.45', '156'],
  ['10.12.3.78', '121'],
  ['172.16.5.23', '98'],
  ['update.campus.local', '86'],
  ['backup.internal.local', '65'],
];

const ruleOverlays: OverlayContract[] = [
  {
    id: 'modal-rule-edit',
    title: '新建/编辑规则',
    kind: 'Modal',
    actionLabel: '规则编辑',
    description: '编辑规则条件、样本验证、命中阈值、标签和回滚版本。',
    impact: '影响实时检测命中和告警生成。',
    audit: '记录规则草稿、测试结果、发布版本和操作者 trace。',
  },
  {
    id: 'drawer-rule-detail',
    title: '规则详情',
    kind: 'Drawer',
    actionLabel: '规则详情',
    description: '展示规则版本、命中趋势、误报样本、白名单建议和审计记录。',
  },
  {
    id: 'modal-rule-publish',
    title: '规则发布确认',
    kind: 'Modal',
    actionLabel: '发布确认',
    description: '确认发布范围、灰度比例、回滚条件和影响评估。',
    impact: '影响 Flink rule job 与在线告警策略。',
    danger: true,
  },
];

type RuleAction = {
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
};

export function RuleManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [editorTab, setEditorTab] = useState('规则定义');
  const [typeFilter, setTypeFilter] = useState('全部类型');
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [query, setQuery] = useState('');
  const [listPage, setListPage] = useState(1);
  const [sampleTab, setSampleTab] = useState('PCAP 样本 32');
  const [action, setAction] = useState<RuleAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildRuleRows(data?.rows ?? []), [data?.rows]);
  const filteredRows = useMemo(() => rows.filter((row) => {
    const typeMatches = typeFilter === '全部类型' || String(row.类型).includes(typeFilter);
    const statusMatches = statusFilter === '全部状态' || String(row.状态).includes(statusFilter);
    const text = `${row.规则ID ?? ''} ${row.规则名称 ?? ''} ${row.MITRE阶段 ?? ''}`.toLowerCase();
    return typeMatches && statusMatches && (!query || text.includes(query.toLowerCase()));
  }), [query, rows, statusFilter, typeFilter]);
  const pageSize = 7;
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const visibleRows = filteredRows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderRuleCell(column, value),
  }));
  function openAction(title: string, target = String(selected?.['规则ID'] ?? 'C2_Tunnel_v3')) {
    setActionSubmitted(false);
    setAction(createRuleAction(title, target));
  }

  return (
    <div className="taf-page taf-rules">
      <section className="taf-rules-shell">
        <main className="taf-rules-main">
          <header className="taf-rules-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              <span>创建、测试、发布、命中、误报、回滚与审计闭环</span>
            </div>
            <Space size={6}>
              <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => openAction('规则库')}>规则库</Button>
              <Button size="small" icon={<ImportOutlined />} onClick={() => openAction('导入规则')}>导入规则</Button>
              <Button size="small" icon={<DownloadOutlined />} onClick={() => openAction('导出规则')}>导出规则</Button>
              <Button size="small" icon={<SettingOutlined />} onClick={() => openAction('规则包管理')}>规则包管理</Button>
              <Button size="small" type="primary" icon={<EditOutlined />} onClick={() => openAction('新建规则')}>新建规则</Button>
              <Tooltip title="刷新规则库">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={ruleOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/rules、APISIX 路由或 rule-manager。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-rules-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-rules-workbench">
            <WorkPanel title={`规则列表（共 ${filteredRows.length} 条）`} className="taf-rules-list-panel">
              <div className="taf-rules-filterbar">
                <label><SearchOutlined /><input aria-label="搜索规则" value={query} onChange={(event) => { setQuery(event.target.value); setListPage(1); }} placeholder="搜索规则名称 / 规则ID" /></label>
                <Select size="small" value={typeFilter} options={[{ value: '全部类型' }, { value: '流量' }, { value: '文件' }]} onChange={(value) => { setTypeFilter(value); setListPage(1); }} />
                <Select size="small" value={statusFilter} options={[{ value: '全部状态' }, { value: '启用' }, { value: '灰度' }]} onChange={(value) => { setStatusFilter(value); setListPage(1); }} />
                <Button size="small" icon={<FilterOutlined />} aria-label="保存规则筛选条件" onClick={() => openAction('保存筛选条件')} />
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </div>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                scroll={{ x: 980, y: 236 }}
                columns={columns}
                dataSource={visibleRows}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
              />
              <div className="taf-rules-pagination"><span>共 {filteredRows.length} 条</span><button type="button" aria-label="规则上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`规则第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="规则下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
            </WorkPanel>

            <WorkPanel title={`规则编辑：${String(selected?.['规则ID'] ?? 'C2_Tunnel_v3')}`} extra={<StatusTag value={selected?.状态 ?? '已启用'} />} className="taf-rules-editor-panel">
              <RuleEditor selected={selected} activeTab={editorTab} onTabChange={setEditorTab} onAction={openAction} />
            </WorkPanel>

            <aside className="taf-rules-rail">
              <WorkPanel title="生命周期">
                <Lifecycle />
              </WorkPanel>
              <WorkPanel title="版本历史">
                <VersionHistory selected={selected} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="审批清单（当前版本 v3.0）">
                <ApprovalList />
              </WorkPanel>
              <WorkPanel title="发布控制">
                <ReleaseControl onAction={openAction} />
              </WorkPanel>
            </aside>
          </div>

          <div className="taf-rules-bottom">
            <WorkPanel title="样本回放验证（近 7 天）">
              <SampleReplay activeTab={sampleTab} onTabChange={setSampleTab} onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="命中结果矩阵（近 7 天）">
              <HitMatrix onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="误报样本 Top5">
              <FalsePositiveTop onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="性能影响（近 7 天）">
              <PerformanceImpact />
            </WorkPanel>
            <WorkPanel title="白名单草案（命中高但低风险）">
              <WhitelistDraft onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="相关操作">
              <RelatedActions onAction={openAction} />
            </WorkPanel>
          </div>
        </main>
      </section>
      <Drawer className="taf-rules-action-drawer" title={action ? `${action.title}确认` : '规则操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为规则对象创建“{action.title}”仿真任务，并保留租户、审批与审计上下文。</p><dl><dt>规则对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="规则业务操作已进入仿真任务队列" description={`目标：${action.target}；动作：${action.title}`} />}</div>}
      </Drawer>
    </div>
  );
}

function RuleEditor({
  selected,
  activeTab,
  onTabChange,
  onAction,
}: {
  selected?: SnapshotRow;
  activeTab: string;
  onTabChange: (value: string) => void;
  onAction: (title: string, target?: string) => void;
}) {
  return (
    <div className="taf-rules-editor">
      <nav className="taf-rules-editor-tabs">
        {['规则定义', '测试验证', '依赖引用'].map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}
      </nav>
      <div className="taf-rules-editor-grid">
        <section>
          <h3>条件构建器</h3>
          <div className="taf-rules-condition-actions"><Button size="small" icon={<BranchesOutlined />} onClick={() => onAction('添加条件组')}>添加条件组</Button><Button size="small" icon={<ApiOutlined />} onClick={() => onAction('添加条件')}>添加条件</Button></div>
          <div className="taf-rules-conditions">
            <strong>且（AND）</strong>
            {ruleConditions.map(([field, op, value]) => (
              <div key={field}><span>{field}</span><em>{op}</em><b>{value}</b><button type="button" aria-label={`移除条件 ${field}`} onClick={() => onAction('移除条件', field)}>×</button></div>
            ))}
          </div>
          <h3>例外条件（任意满足则排除）</h3>
          <div className="taf-rules-exception"><span>目的IP</span><em>在</em><b>信任列表 (Whitelist_IP_Group)</b><button type="button" aria-label="移除规则例外条件" onClick={() => onAction('移除例外条件')}>×</button></div>
        </section>
        <section>
          <div className="taf-rules-dsl-title"><h3>DSL 表达式（自动生成）</h3><Button size="small" onClick={() => onAction('格式化 DSL')}>格式化</Button></div>
          <pre>{`when
  proto in {"TLS", "SSH"}
  and ja3_score > 0.82
  and dst_reputation == "high"
  and bytes_out_p95 > 5 MB
then
  alert("${String(selected?.['规则ID'] ?? 'C2_Tunnel_v3')}")
  level = high
  category = "C2"
  mitre = ["TA0011"]
end`}</pre>
          <div className="taf-rules-mitre"><span>TA0011 指挥与控制</span><Button size="small" onClick={() => onAction('添加 MITRE 阶段')}>添加阶段</Button></div>
        </section>
      </div>
    </div>
  );
}

function Lifecycle() {
  const steps = ['草稿', '待审', '灰度', '启用', '停用', '回滚'];
  return (
    <div className="taf-rules-lifecycle">
      {steps.map((step, index) => <span key={step} className={index === 2 ? 'is-active' : ''}>{step}</span>)}
      <p>当前状态：<b>灰度</b></p>
      <p>最近状态变更：2026-06-19 17:28:45</p>
    </div>
  );
}

function VersionHistory({ selected, onAction }: { selected?: SnapshotRow; onAction: (title: string, target?: string) => void }) {
  const version = String(selected?.版本 ?? 'v3.0');
  return (
    <div className="taf-rules-version">
      {[version, 'v2.9', 'v2.8', 'v2.7'].map((item, index) => (
        <div key={item}><HistoryOutlined /><strong>{item}</strong><span>{index === 0 ? '当前' : index === 1 ? '启用' : '停用'}</span><em>{index ? 'system' : 'sec_analyst'}</em></div>
      ))}
      <Button size="small" type="link" onClick={() => onAction('查看更多版本', String(selected?.['规则ID'] ?? 'C2_Tunnel_v3'))}>查看更多版本</Button>
    </div>
  );
}

function ApprovalList() {
  return (
    <div className="taf-rules-approval">
      {['语法校验', '逻辑评审', '安全评审', '运营评审', '最终审核'].map((item, index) => (
        <span key={item} className={index < 2 ? 'is-ok' : index < 4 ? 'is-warn' : ''}><CheckCircleOutlined />{item}<b>{index < 2 ? '已通过' : index < 4 ? '待评审' : '待提交'}</b></span>
      ))}
    </div>
  );
}

function ReleaseControl({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-rules-release">
      <Button size="small" onClick={() => onAction('灰度设置')}>灰度设置</Button>
      <Button size="small" type="primary" onClick={() => onAction('全量发布')}>全量发布</Button>
      <Button size="small" danger ghost icon={<RollbackOutlined />} onClick={() => onAction('回滚版本')}>回滚版本</Button>
      <CloseableConfirmButton
        title="删除规则"
        description="影响范围：当前规则草稿及关联测试记录；确认删除后写入 audit trace，可从版本历史追溯。"
        confirmText="确认删除"
        danger
        buttonProps={{ className: 'taf-rules-delete', size: 'small', danger: true }}
      >
        删除规则
      </CloseableConfirmButton>
      <Button size="small" type="link" onClick={() => onAction('进入部署管理')}>进入部署管理</Button>
    </div>
  );
}

function SampleReplay({ activeTab, onTabChange, onAction }: { activeTab: string; onTabChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  const tabs = ['PCAP 样本 32', 'Session 样本 128', '日志样本 256'];
  return (
    <div className="taf-rules-samples">
      <div>{tabs.map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}</div>
      {sampleRows.map(([name, size, source, time]) => <span key={name}><b>{name}</b><em>{size}</em><em>{source}</em><em>{time}</em><Button type="text" size="small" icon={<PlayCircleOutlined />} aria-label={`回放样本 ${name}`} onClick={() => onAction('回放样本', name)} /><Button type="text" size="small" icon={<DownloadOutlined />} aria-label={`下载样本 ${name}`} onClick={() => onAction('下载样本', name)} /></span>)}
    </div>
  );
}

function HitMatrix({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-rules-matrix">
      <div><span>规则</span><span>TP</span><span>FP</span><span>TN</span><span>FN</span><span>误报率</span></div>
      {matrixRows.map((row) => <button key={row[0]} type="button" onClick={() => onAction('查看命中矩阵', row[0])}>{row.map((cell) => <b key={cell}>{cell}</b>)}</button>)}
    </div>
  );
}

function FalsePositiveTop({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-rules-fp">
      {fpRows.map(([id, count, type, source]) => <button key={id} type="button" onClick={() => onAction('查看误报样本', id)}><b>{id}</b><em>{count}</em><em>{type}</em><em>{source}</em></button>)}
      <Button size="small" type="link" onClick={() => onAction('导出 Top5')}>导出 Top5</Button>
    </div>
  );
}

function PerformanceImpact() {
  const items = [
    ['平均延时', '18 ms', '+2 ms', 'info' as const, [14, 16, 15, 17, 16, 18, 18]],
    ['P95 延时', '46 ms', '+5 ms', 'warn' as const, [36, 39, 42, 40, 44, 41, 46]],
    ['CPU占用', '7.4%', '-0.6%', 'ok' as const, [8.1, 8, 7.8, 7.6, 7.9, 7.5, 7.4]],
    ['内存占用', '1.2 GB', '+0.1 GB', 'info' as const, [1, 1.1, 1.1, 1.2, 1.1, 1.2, 1.2]],
  ];
  return <div className="taf-rules-performance">{items.map(([label, value, delta, tone, values]) => <div key={label as string}><span>{label as string}</span><strong>{value as string}</strong><em>{delta as string}</em><div className="taf-rules-performance-echart"><DataQualityKpiSparklineChart ariaLabel={`规则${label as string}趋势`} tone={tone as 'ok' | 'info' | 'warn' | 'risk'} values={values as number[]} /></div></div>)}</div>;
}

function WhitelistDraft({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-rules-whitelist">
      {whitelistRows.map(([entity, count]) => <button key={entity} type="button" onClick={() => onAction('查看白名单建议', entity)}><b>{entity}</b><em>{count}</em></button>)}
      <Button size="small" type="link" onClick={() => onAction('生成白名单草案')}>生成白名单草案</Button>
    </div>
  );
}

function RelatedActions({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const actions: Array<[string, ReactNode]> = [
    ['查看命中规则', <SearchOutlined key="search" />],
    ['进入部署管理', <CloudUploadOutlined key="deploy" />],
    ['生成白名单草案', <SafetyCertificateOutlined key="white" />],
    ['回滚版本', <RollbackOutlined key="rollback" />],
    ['导出规则包', <DownloadOutlined key="download" />],
  ];
  return <div className="taf-rules-actions">{actions.map(([label, icon]) => <Button key={label} size="small" icon={icon} onClick={() => onAction(label)}>{label}</Button>)}</div>;
}

const renderRuleCell = (column: string, value: unknown) => {
  if (column === '规则ID') return <span className="taf-rules-id-cell"><CodeOutlined />{String(value)}</span>;
  if (column === '严重级别' || column === '状态') return <StatusTag value={value} />;
  if (column === '误报率' || column === '平均延时') return <span className="taf-rules-warn">{String(value)}</span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row['规则ID'] ?? row['规则名称'] ?? JSON.stringify(row));

const buildRuleRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 规则ID: 'C2_Tunnel_v3', 规则名称: 'C2 隧道通信检测', 类型: '流量', 严重级别: '高', MITRE阶段: '指挥与控制', 状态: '启用', 版本: 'v3.0', 命中数: '1.3K', 误报率: '0.38%', 平均延时: '18 ms' }];
  if (source.length >= 21) return source;
  return Array.from({ length: 21 }, (_, index) => index < source.length ? source[index] : {
    ...source[index % source.length],
    规则ID: `${String(source[index % source.length]['规则ID'] ?? 'RULE')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}`,
  });
};

const createRuleAction = (title: string, target: string): RuleAction => {
  const actions = pageApiPlans.rules.actions ?? [];
  const plan = actions.find((item) =>
    (title.includes('发布') || title.includes('启用')) && item.id === 'rule-enable'
    || (title.includes('停用') || title.includes('回滚')) && item.id === 'rule-disable',
  );
  return { title, target, endpoint: plan?.endpoint ?? '/v1/rules/{id}', auditEvent: plan?.auditEvent ?? 'RULE_ACTION_SIMULATED' };
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '92.0%' : '0',
  delta: 'API',
  status: 'info',
});
