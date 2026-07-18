import {
  ApiOutlined,
  AuditOutlined,
  CalendarOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  CloudDownloadOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  LinkOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Empty, Input, Select, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ForensicsSessionTimelineChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  cancelForensicsJob,
  createForensicsJob,
  fetchPageSnapshot,
  presignForensicsPcap,
  verifyForensicsPcap,
} from '@/services/api';
import type { ForensicsVisuals, SnapshotRow } from '@/services/mockData';
import { getAuthToken } from '@/services/authStorage';

type ActionKind = 'view' | 'create' | 'cancel' | 'verify' | 'presign';
type ForensicsAction = {
  kind: ActionKind;
  title: string;
  target: string;
  key?: string;
  sha256?: string;
  endpoint: string;
  auditEvent: string;
};

const emptyVisuals: ForensicsVisuals = {
  availability: { jobs: 'unavailable', sessions: 'unavailable', pcap: 'unavailable', audit: 'unavailable' },
  stateCounts: [], jobs: [], pcapIndexes: [], pcapTrend: [], sessions: [], completeness: [], hashRows: [], signedUrls: [], exportRows: [], auditRows: [],
};

export function ForensicsWorkbenchPage({ route }: { route: NavRoute }) {
  const [searchParams] = useSearchParams();
  const sourceAssetId = searchParams.get('assetId') ?? '';
  const [listPage, setListPage] = useState(1);
  const [protocol, setProtocol] = useState('全部');
  const [asset, setAsset] = useState(sourceAssetId || '全部资产');
  const [srcIp, setSrcIp] = useState('');
  const [dstIp, setDstIp] = useState('');
  const [port, setPort] = useState('全部');
  const [tuple, setTuple] = useState('');
  const [taskId, setTaskId] = useState('');
  const [appliedFilters, setAppliedFilters] = useState<Record<string, string>>({});
  const [action, setAction] = useState<ForensicsAction>();
  const pageSize = 5;
  const permissions = readTokenPermissions(getAuthToken());
  const canWrite = hasScope(permissions, 'pcap:write');
  const canDownload = hasScope(permissions, 'pcap:download');

  useEffect(() => {
    if (sourceAssetId) setAsset(sourceAssetId);
  }, [sourceAssetId]);

  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, sourceAssetId, listPage, pageSize, appliedFilters],
    queryFn: () => fetchPageSnapshot(route.id, { sourceAssetId, page: listPage, pageSize, forensicsFilters: appliedFilters }),
  });
  const visuals = data?.visuals?.forensics ?? emptyVisuals;
  const rows = data?.rows ?? [];
  const total = data?.total ?? rows.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderForensicCell(column, value, record, openAction),
  }));

  const actionMutation = useMutation({
    mutationFn: async (current: ForensicsAction) => {
      if (current.kind === 'create') {
        const endTime = Date.now();
        const firstSession = visuals.sessions[0];
        return createForensicsJob({
          assetId: isUuid(sourceAssetId) ? sourceAssetId : undefined,
          srcIp: firstSession?.source !== '-' ? firstSession?.source.split(':')[0] : undefined,
          dstIp: firstSession?.destination.split(':')[0] || undefined,
          startTime: endTime - 60 * 60 * 1_000,
          endTime,
          maxPackets: 100_000,
        });
      }
      if (current.kind === 'cancel') return cancelForensicsJob(current.target);
      if (current.kind === 'verify') return verifyForensicsPcap(current.key || current.target, current.sha256 === '-' ? undefined : current.sha256);
      if (current.kind === 'presign') return presignForensicsPcap(current.key || current.target, 3600);
      return { status: 'loaded', target: current.target };
    },
    onSuccess: () => void refetch(),
  });

  function openAction(title: string, target = String(rows[0]?.['任务 ID'] ?? '-'), options: Partial<ForensicsAction> = {}) {
    actionMutation.reset();
    setAction(createForensicsAction(title, target, options));
  }

  const currentJob = visuals.jobs[0];
  const currentSession = visuals.sessions[0];
  const assetOptions = Array.from(new Set(['全部资产', ...rows.map((row) => String(row['资产'] ?? '')).filter(Boolean), asset])).map((value) => ({ value, label: value }));

  return (
    <div className="taf-page taf-forensics">
      <section className="taf-forensics-shell">
        <main className="taf-forensics-workspace">
          <header className="taf-forensics-titlebar">
            <div className="taf-forensics-heading"><h1>{route.page.title}</h1><span>证据检索、会话复放、完整性校验与受控导出</span></div>
            <div className="taf-forensics-context-row">
              <div className="taf-forensics-source"><b>来源上下文</b><button type="button" onClick={() => openAction('查看关联任务', currentJob?.id || '-')}>任务（{currentJob?.id || '暂无'}）</button><button type="button" onClick={() => openAction('查看关联资产', sourceAssetId || '未指定资产')}>资产（{sourceAssetId || '未指定'}）</button><button type="button" onClick={() => openAction('查看关联会话', currentSession?.sessionId || '-')}>会话（{currentSession?.sessionId || '暂无'}）</button><button type="button" onClick={() => openAction('查看图谱路径', sourceAssetId || currentSession?.source || '-')}>图谱路径 &gt;</button></div>
              <Button size="small" type="link" onClick={() => { setProtocol('全部'); setAsset(sourceAssetId || '全部资产'); setSrcIp(''); setDstIp(''); setPort('全部'); setTuple(''); setTaskId(''); setAppliedFilters({}); setListPage(1); }}>清空条件</Button>
            </div>
          </header>

          {isError && <Alert type="error" showIcon message="真实 API 数据加载失败" description={error instanceof Error ? error.message : '请检查取证、会话、证据和审计 API。'} action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>} />}

          <div className="taf-forensics-filter">
            <label><span>时间窗</span><Button size="small" icon={<CalendarOutlined />} onClick={() => openAction('查看时间窗', '近 24 小时')}>近 24 小时</Button></label>
            <label><span>资产</span><Select size="small" value={asset} options={assetOptions} onChange={setAsset} /></label>
            <label><span>源 IP</span><Input size="small" value={srcIp} placeholder="请输入源 IP" onChange={(event) => setSrcIp(event.target.value)} /></label>
            <label><span>目的 IP</span><Input size="small" value={dstIp} placeholder="请输入目的 IP" onChange={(event) => setDstIp(event.target.value)} /></label>
            <label><span>协议</span><Select size="small" value={protocol} options={['全部', 'TLS', 'DNS', 'HTTP'].map((value) => ({ value }))} onChange={setProtocol} /></label>
            <label><span>端口</span><Select size="small" value={port} options={['全部', '443', '53'].map((value) => ({ value }))} onChange={setPort} /></label>
            <label><span>五元组</span><Input size="small" value={tuple} placeholder="请输入五元组" onChange={(event) => setTuple(event.target.value)} /></label>
            <label><span>任务 ID</span><Input size="small" value={taskId} placeholder="请输入任务 ID" onChange={(event) => setTaskId(event.target.value)} /></label>
            <Button size="small" onClick={() => { setProtocol('全部'); setAsset(sourceAssetId || '全部资产'); setSrcIp(''); setDstIp(''); setPort('全部'); setTuple(''); setTaskId(''); setAppliedFilters({}); setListPage(1); }}>重置</Button>
            <Button size="small" type="primary" icon={<SearchOutlined />} onClick={() => { setAppliedFilters({ assetId: asset !== '全部资产' ? asset : '', srcIp: srcIp.trim(), dstIp: dstIp.trim(), protocol, port, tuple: tuple.trim(), taskId: taskId.trim() }); setListPage(1); }}>查询</Button>
            <Tooltip title="刷新取证数据"><Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} /></Tooltip>
          </div>

          <TaskTab rows={rows} total={total} page={listPage} pageCount={pageCount} pageSize={pageSize} loading={isLoading} columns={columns} visuals={{ ...visuals, sessions: visuals.sessions.filter((item) => protocol === '全部' || item.protocol === protocol) }} sourceAssetId={sourceAssetId} canWrite={canWrite} canDownload={canDownload} onPage={setListPage} onAction={openAction} />
        </main>
        <aside className="taf-forensics-rail">
          <IntegrityPanel rows={visuals.completeness} onAction={openAction} />
          <SignedUrlPanel rows={visuals.signedUrls} onAction={openAction} />
          <WorkPanel title="取证操作"><ActionGrid jobs={visuals.jobs} pcaps={visuals.pcapIndexes} canWrite={canWrite} onAction={openAction} /></WorkPanel>
          <AuditPanel rows={visuals.auditRows} onAction={openAction} />
        </aside>
      </section>

      <Drawer className="taf-forensics-action-drawer" title={action ? `${action.title}${action.kind === 'view' ? '' : '确认'}` : '取证操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); actionMutation.reset(); }} extra={action?.kind === 'view' ? <Button size="small" onClick={() => setAction(undefined)}>关闭</Button> : <Button size="small" type="primary" loading={actionMutation.isPending} disabled={!action || actionMutation.isSuccess} onClick={() => action && actionMutation.mutate(action)}>{actionMutation.isSuccess ? '已完成' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>{action.kind === 'view' ? `当前展示已由取证查询接口加载的“${action.title}”上下文。` : `将对取证对象执行“${action.title}”，并保留授权、证据与审计上下文。`}</p><dl><dt>取证对象</dt><dd>{action.target}</dd><dt>数据/操作接口</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionMutation.isSuccess && action.kind === 'verify' && !isVerifiedResult(actionMutation.data) && <Alert type="error" showIcon message="PCAP 完整性校验不匹配" description={JSON.stringify(actionMutation.data)} />}{actionMutation.isSuccess && (action.kind !== 'verify' || isVerifiedResult(actionMutation.data)) && <Alert type="success" showIcon message="取证操作已完成" description={JSON.stringify(actionMutation.data)} />}{actionMutation.isError && <Alert type="error" showIcon message="取证操作失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : 'unknown error'} />}</div>}
      </Drawer>
    </div>
  );
}

function TaskTab({ rows, total, page, pageCount, pageSize, loading, columns, visuals, sourceAssetId, canWrite, canDownload, onPage, onAction }: { rows: SnapshotRow[]; total: number; page: number; pageCount: number; pageSize: number; loading: boolean; columns: ColumnsType<SnapshotRow>; visuals: ForensicsVisuals; sourceAssetId: string; canWrite: boolean; canDownload: boolean; onPage: (page: number) => void; onAction: ActionHandler }) {
  const [sessionPage, setSessionPage] = useState(1);
  const [pcapPage, setPcapPage] = useState(1);
  const [exportPage, setExportPage] = useState(1);
  const [hashPage, setHashPage] = useState(1);
  const sessionPageSize = 6;
  const compactPageSize = 3;
  const pcapRows = pageSlice(visuals.pcapIndexes, pcapPage, pageSize);
  const sessionRows = pageSlice(visuals.sessions, sessionPage, sessionPageSize);
  const exportRows = pageSlice(visuals.exportRows, exportPage, compactPageSize);
  const hashRows = pageSlice(visuals.hashRows, hashPage, compactPageSize);
  return (
    <div className="taf-forensics-dashboard">
      <div className="taf-forensics-core">
        <div className="taf-forensics-left-stack">
          <WorkPanel title="取证任务状态机" extra={<Tooltip title={canWrite ? '' : '需要 pcap:write 权限'}><Button size="small" type="primary" disabled={!canWrite} onClick={() => onAction('新建取证任务')}>新建任务</Button></Tooltip>}><StateMachine rows={visuals.stateCounts} /></WorkPanel>
          <WorkPanel title={`取证任务列表（共 ${total} 条）`} className="taf-forensics-task-panel"><div className="taf-forensics-table-frame"><Table rowKey={(record) => String(record['任务 ID'])} size="small" loading={loading} pagination={false} tableLayout="fixed" columns={columns} dataSource={rows} /><ListPager label="取证任务" page={page} pageCount={pageCount} pageSize={pageSize} total={total} onPage={onPage} /></div></WorkPanel>
          <WorkPanel title={`PCAP 索引（共 ${formatCount(visuals.totals?.pcapIndexes ?? visuals.pcapIndexes.length)} 条）`} extra={<Button size="small" type="link" onClick={() => onAction('查看全部 PCAP', `${visuals.totals?.pcapIndexes ?? visuals.pcapIndexes.length} 条`)}>查看全部 PCAP &gt;</Button>}><div className="taf-forensics-mini-table-frame"><PcapRows rows={pcapRows} onAction={onAction} /><ListPager label="PCAP 索引" page={pcapPage} pageCount={countPages(visuals.totals?.pcapIndexes ?? visuals.pcapIndexes.length, pageSize)} pageSize={pageSize} total={visuals.totals?.pcapIndexes ?? visuals.pcapIndexes.length} onPage={setPcapPage} /></div></WorkPanel>
        </div>
        <div className="taf-forensics-right-stack">
          <WorkPanel title="会话复放（Session）" extra={<Button size="small" type="link" onClick={() => onAction('查看全部会话', `${visuals.totals?.sessions ?? visuals.sessions.length} 条`)}>查看全部会话 &gt;</Button>}><div className="taf-forensics-mini-table-frame"><SessionRows rows={sessionRows} onAction={onAction} /><ListPager label="会话复放" page={sessionPage} pageCount={countPages(visuals.totals?.sessions ?? visuals.sessions.length, sessionPageSize)} pageSize={sessionPageSize} total={visuals.totals?.sessions ?? visuals.sessions.length} onPage={setSessionPage} /></div><div className="taf-forensics-packet-echart"><ForensicsSessionTimelineChart ariaLabel="会话数据包时间轴" rows={visuals.sessions} /></div></WorkPanel>
          <WorkPanel title="会话请求 / 响应与协议摘要"><SessionPayload session={sessionRows[0] ?? visuals.sessions[0]} /></WorkPanel>
        </div>
      </div>
      <div className="taf-forensics-lower">
        <WorkPanel title="证据导出包" extra={<Button size="small" type="link" onClick={() => onAction('查看全部导出记录', `${visuals.totals?.exportRows ?? visuals.exportRows.length} 条`)}>查看全部导出 &gt;</Button>}><div className="taf-forensics-mini-table-frame"><ExportRows rows={exportRows} canDownload={canDownload} onAction={onAction} /><ListPager label="证据导出包" page={exportPage} pageCount={countPages(visuals.totals?.exportRows ?? visuals.exportRows.length, compactPageSize)} pageSize={compactPageSize} total={visuals.totals?.exportRows ?? visuals.exportRows.length} onPage={setExportPage} /></div></WorkPanel>
        <WorkPanel title={`Hash 校验结果（最近 ${visuals.totals?.hashRows ?? visuals.hashRows.length} 条）`} extra={<Button size="small" type="link" onClick={() => onAction('查看全部 Hash 校验', `${visuals.totals?.hashRows ?? visuals.hashRows.length} 条`)}>查看全部校验 &gt;</Button>}><div className="taf-forensics-mini-table-frame"><HashRows rows={hashRows} onAction={onAction} /><ListPager label="Hash 校验结果" page={hashPage} pageCount={countPages(visuals.totals?.hashRows ?? visuals.hashRows.length, compactPageSize)} pageSize={compactPageSize} total={visuals.totals?.hashRows ?? visuals.hashRows.length} onPage={setHashPage} /></div></WorkPanel>
        <WorkPanel title="返回来源"><ReturnSources sourceAssetId={sourceAssetId} session={visuals.sessions[0]} onAction={onAction} /></WorkPanel>
      </div>
    </div>
  );
}

type ActionHandler = (title: string, target?: string, options?: Partial<ForensicsAction>) => void;

function SessionRows({ rows, onAction }: { rows: ForensicsVisuals['sessions']; onAction: ActionHandler }) {
  return <div className="taf-forensics-session-table"><div><span>开始时间</span><span>协议</span><span>源地址</span><span>目的地址</span><span>字节/包</span><span>持续时间</span></div>{rows.map((item) => <button key={item.sessionId} type="button" onClick={() => onAction('复放会话', item.sessionId)}><span>{item.time}</span><strong>{item.protocol}</strong><span>{item.source}</span><span>{item.destination}</span><span>{formatBytes(item.byteCount)} / {item.packetCount}</span><span>{item.duration}</span></button>)}</div>;
}

function SessionPayload({ session }: { session?: ForensicsVisuals['sessions'][number] }) {
  if (!session) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无会话请求 / 响应数据" />;
  return <div className="taf-forensics-payload"><pre><b>请求（Client → Server）</b>{`\nSession: ${session.sessionId}\nSource: ${session.source}\nProtocol: ${session.protocol}\nStarted: ${session.time}`}</pre><pre><b>响应（Server → Client）</b>{`\nDestination: ${session.destination}\nBytes: ${formatBytes(session.byteCount)}\nPackets: ${session.packetCount}\nDuration: ${session.duration}`}</pre><dl><dt>会话 ID</dt><dd>{session.sessionId}</dd><dt>SNI</dt><dd>{session.sni}</dd><dt>JA3</dt><dd>{session.ja3}</dd><dt>风险</dt><dd>{session.risk}</dd></dl></div>;
}

function PcapRows({ rows, onAction }: { rows: ForensicsVisuals['pcapIndexes']; onAction: ActionHandler }) {
  return <div className="taf-forensics-pcap"><div><span>对象键</span><span>MinIO 路径</span><span>大小</span><span>SHA256</span><span>时间窗</span><span>状态</span></div>{rows.map((item) => <button key={`${item.fileKey}-${item.startTime}`} type="button" onClick={() => onAction('查看 PCAP 索引', item.fileKey)}><strong>{item.fileKey}</strong><span>{item.storagePath}</span><span>{formatBytes(item.sizeBytes)}</span><span>{shortHash(item.sha256)}</span><span>{item.startTime} ~ {item.endTime}</span><StatusTag value={item.status} /></button>)}</div>;
}

function ExportRows({ rows, canDownload, onAction }: { rows: ForensicsVisuals['exportRows']; canDownload: boolean; onAction: ActionHandler }) {
  return <div className="taf-forensics-export"><div><span>任务 ID</span><span>类型</span><span>内容</span><span>文件数</span><span>大小</span><span>状态</span><span>操作</span></div>{rows.map((item) => <button key={item.id} type="button" title={canDownload ? '生成下载签名 URL' : '需要 pcap:download 权限'} onClick={() => canDownload && isValidResultKey(item.resultKey) ? onAction('生成下载签名 URL', item.id, { kind: 'presign', key: item.resultKey }) : onAction('查看证据任务', item.id)}><strong>{item.id}</strong><span>PCAP 证据</span><span>{item.content}</span><span>{item.files}</span><span>{formatBytes(item.sizeBytes)}</span><StatusTag value={item.status} /><DownloadOutlined /></button>)}</div>;
}

function HashRows({ rows, onAction }: { rows: ForensicsVisuals['hashRows']; onAction: ActionHandler }) {
  return <div className="taf-forensics-hash"><div><span>对象键</span><span>算法</span><span>索引值</span><span>实时校验</span><span>状态</span><span>索引时间</span></div>{rows.map((item) => <button key={`${item.fileKey}-${item.checkedAt}`} type="button" onClick={() => isValidResultKey(item.fileKey) ? onAction('校验 PCAP 完整性', item.fileKey, { kind: 'verify', key: item.fileKey, sha256: item.sha256 }) : onAction('查看 PCAP 索引', item.fileKey)}><strong>{item.fileKey}</strong><span>SHA256</span><span>{shortHash(item.sha256)}</span><span>{isValidResultKey(item.fileKey) ? '调用 /verify' : '索引只读'}</span><StatusTag value={item.status} /><span>{item.checkedAt}</span></button>)}</div>;
}

function ReturnSources({ sourceAssetId, session, onAction }: { sourceAssetId: string; session?: ForensicsVisuals['sessions'][number]; onAction: ActionHandler }) {
  return <div className="taf-forensics-return"><button type="button" onClick={() => onAction('返回告警详情', session?.sessionId || '-')}><LinkOutlined /><span>返回告警详情</span><strong>{session?.sessionId || '暂无关联告警'} &gt;</strong></button><button type="button" onClick={() => onAction('返回战役详情', sourceAssetId || '-')}><LinkOutlined /><span>返回战役详情</span><strong>查看关联战役 &gt;</strong></button><button type="button" onClick={() => onAction('返回资产详情', sourceAssetId || '未指定资产')}><LinkOutlined /><span>返回资产详情</span><strong>{sourceAssetId || session?.source || '未指定资产'} &gt;</strong></button><button type="button" onClick={() => onAction('返回实体图谱', sourceAssetId || session?.source || '-')}><LinkOutlined /><span>返回实体图谱</span><strong>查看路径 &gt;</strong></button><button type="button" onClick={() => onAction('返回取证设置', session?.sessionId || '-')}><LinkOutlined /><span>返回取证设置</span><strong>查看模板 &gt;</strong></button></div>;
}

function StateMachine({ rows }: { rows: ForensicsVisuals['stateCounts'] }) {
  const icons = [<FileSearchOutlined key="new" />, <ClockCircleOutlined key="queued" />, <CloudDownloadOutlined key="collecting" />, <ApiOutlined key="parsing" />, <CheckCircleOutlined key="done" />, <CloseCircleOutlined key="failed" />];
  return <div className="taf-forensics-state">{rows.map((item, index) => <div key={item.label} className={`is-${item.status}`}><span>{icons[index]}</span><strong>{item.label}</strong><em>{item.value}</em>{index < rows.length - 1 && <i />}</div>)}</div>;
}

function IntegrityPanel({ rows, onAction }: { rows: ForensicsVisuals['completeness']; onAction: ActionHandler }) {
  const total = rows.reduce((sum, item) => sum + item.total, 0);
  const complete = rows.reduce((sum, item) => sum + item.complete, 0);
  const score = total ? Math.round(complete / total * 100) : 0;
  return <WorkPanel title="证据完整性" extra={<Button size="small" type="link" onClick={() => onAction('查看完整性详情')}>查看详情 &gt;</Button>}><div className="taf-forensics-integrity"><div className="taf-forensics-integrity-score"><SafetyCertificateOutlined /><span>完整性评分</span><strong>{score}<small>/100</small></strong><em>来自 /encrypted-traffic/evidence</em></div>{rows.slice(0, 5).map((item) => <div key={item.label}><CheckCircleOutlined /><span>{item.label}</span><strong>{item.complete} / {item.total}</strong><em>{item.status === 'ok' ? '通过' : '关注'}</em></div>)}</div></WorkPanel>;
}

function SignedUrlPanel({ rows, onAction }: { rows: ForensicsVisuals['signedUrls']; onAction: ActionHandler }) {
  return <WorkPanel title="签名 URL 与有效期" extra={<Button size="small" type="link" onClick={() => onAction('查看全部签名记录')}>查看全部签名 &gt;</Button>}><div className="taf-forensics-signed"><div><span>类型</span><span>签名 URL</span><span>过期时间</span><span>状态</span></div>{rows.slice(0, 4).map((item) => <button key={`${item.key}-${item.expiresAt}`} type="button" onClick={() => onAction('查看签名 URL', item.key)}><strong>{item.type || 'PCAP'}</strong><span>{item.url || item.key}</span><span>{item.expiresAt}</span><StatusTag value={item.status} /></button>)}</div>{!rows.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无有效签名 URL" />}</WorkPanel>;
}

function ActionGrid({ jobs, pcaps, canWrite, onAction }: { jobs: ForensicsVisuals['jobs']; pcaps: ForensicsVisuals['pcapIndexes']; canWrite: boolean; onAction: ActionHandler }) {
  const target = jobs[0]?.id || pcaps[0]?.fileKey || '-';
  return <div className="taf-forensics-actions"><Tooltip title={canWrite ? '' : '需要 pcap:write 权限'}><Button size="small" icon={<FileProtectOutlined />} disabled={!canWrite} onClick={() => onAction('新建取证任务')}>新建取证</Button></Tooltip><Button size="small" icon={<CalendarOutlined />} disabled={!canWrite} title={canWrite ? '' : '需要 pcap:write 权限'} onClick={() => onAction('追加取证时间窗', target)}>追加时间窗</Button><Button size="small" icon={<ApiOutlined />} onClick={() => onAction('关联战役', target)}>关联战役</Button><Button size="small" icon={<FileSearchOutlined />} onClick={() => onAction('关联告警', target)}>关联告警</Button><Button size="small" icon={<LinkOutlined />} onClick={() => onAction('进入实体图谱', target)}>进入图谱</Button><Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => onAction('进入取证分析', target)}>进入取证</Button></div>;
}

function AuditPanel({ rows, onAction }: { rows: ForensicsVisuals['auditRows']; onAction: ActionHandler }) {
  return <WorkPanel title="审计日志（近 24 小时）" extra={<Button size="small" type="link" onClick={() => onAction('查看全部取证审计')}>查看全部 &gt;</Button>}><div className="taf-forensics-audit"><div><span>时间</span><span>操作人</span><span>操作</span><span>对象</span><span>结果</span></div>{rows.slice(0, 5).map((item, index) => <button key={`${item.time}-${item.action}-${index}`} type="button" onClick={() => onAction('查看取证审计', item.target)}><span>{item.time}</span><strong>{item.user}</strong><span>{item.action}</span><span>{item.target}</span><StatusTag value={item.result} /></button>)}</div>{!rows.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="近 24 小时无 PCAP 审计" />}</WorkPanel>;
}

function ListPager({ label, page, pageCount, pageSize, total, onPage }: { label: string; page: number; pageCount: number; pageSize: number; total: number; onPage: (page: number) => void }) {
  const pages = Array.from({ length: Math.min(pageCount, 7) }, (_, index) => index + 1);
  return <div className="taf-forensics-pagination" data-pager-label={label}><button type="button" aria-label={`${label}上一页`} disabled={page === 1} onClick={() => onPage(Math.max(1, page - 1))}>‹</button>{pages.map((item) => <button key={item} type="button" className={item === page ? 'is-active' : ''} aria-label={`${label}第 ${item} 页`} onClick={() => onPage(item)}>{item}</button>)}<button type="button" aria-label={`${label}下一页`} disabled={page === pageCount} onClick={() => onPage(Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页，共 {total} 条</span></div>;
}

function renderForensicCell(column: string, value: unknown, row: SnapshotRow, onAction: ActionHandler) {
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return <Button size="small" type="link" onClick={() => onAction('查看取证任务', String(row['任务 ID'] ?? value))}>{String(value)}</Button>;
  if (column === '任务 ID' || column === '告警/战役 ID') return <span className="taf-forensics-link-cell"><EyeOutlined />{String(value)}</span>;
  if (column === '证据包') return <span className="taf-forensics-package-cell"><FileSearchOutlined />{String(value)}</span>;
  return String(value ?? '-');
}

const createForensicsAction = (title: string, target: string, options: Partial<ForensicsAction> = {}): ForensicsAction => {
  const kind: ActionKind = options.kind ?? ((title === '新建取证任务' || title === '新建取证') ? 'create' : title === '取消取证任务' ? 'cancel' : 'view');
  const endpoint = kind === 'create' ? '/v1/pcap/jobs' : kind === 'cancel' ? '/v1/pcap/jobs/{id}/cancel' : kind === 'verify' ? '/v1/pcap/verify' : kind === 'presign' ? '/v1/pcap/presign' : '/v1/pcap/jobs/{id}';
  const auditEvent = kind === 'create' ? 'PCAP_CUT' : kind === 'cancel' ? 'PCAP_CANCEL' : kind === 'verify' ? 'PCAP_INTEGRITY_VERIFY' : kind === 'presign' ? 'PCAP_DOWNLOAD' : 'READ_ONLY_DETAIL';
  return { kind, title, target, key: options.key, sha256: options.sha256, endpoint, auditEvent };
};

const isUuid = (value: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
const isValidResultKey = (value: string) => /^results\/[^/]+\/[^/]+\/.+/.test(value);
const countPages = (total: number, pageSize: number) => Math.max(1, Math.ceil(total / pageSize));
const formatCount = (value: number) => value.toLocaleString('en-US');
const pageSlice = <T,>(rows: T[], page: number, pageSize: number) => rows.slice((page - 1) * pageSize, page * pageSize);
const shortHash = (value: string) => value && value !== '-' ? `${value.slice(0, 12)}…${value.slice(-8)}` : '-';
const formatBytes = (value: number) => value >= 1024 ** 3 ? `${(value / 1024 ** 3).toFixed(2)} GB` : value >= 1024 ** 2 ? `${(value / 1024 ** 2).toFixed(2)} MB` : value >= 1024 ? `${(value / 1024).toFixed(1)} KB` : `${value} B`;
const readTokenPermissions = (token: string | null): string[] => {
  if (!token) return [];
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))) as { permissions?: unknown };
    return Array.isArray(payload.permissions) ? payload.permissions.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
};
const hasScope = (permissions: string[], required: string) => permissions.some((permission) => permission === '*' || permission === 'admin:*' || permission === 'pcap:*' || permission === required);
const isVerifiedResult = (value: unknown) => typeof value === 'object' && value !== null && 'verified' in value && (value as { verified?: unknown }).verified === true;
