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
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const stateItems = [
  ['新建', '12', 'info', <FileSearchOutlined key="new" />],
  ['排队中', '5', 'info', <ClockCircleOutlined key="queued" />],
  ['采集中', '8', 'warn', <CloudDownloadOutlined key="collecting" />],
  ['解析中', '6', 'warn', <ApiOutlined key="parsing" />],
  ['完成', '156', 'ok', <CheckCircleOutlined key="done" />],
  ['失败', '3', 'risk', <CloseCircleOutlined key="failed" />],
];

const pcapRows = [
  ['20220620/000123/001', 'evidence/pcap/000123/001.pcap', '24.8 MB', 'a1d27c3b2a...b7be95f1c3', '06-19 00:00 ~ 01:00', 'TLS'],
  ['20220620/000123/002', 'evidence/pcap/000123/002.pcap', '18.7 MB', 'c3f8b6d1e4...a1e6d8f4b2', '06-19 01:00 ~ 02:00', 'TLS'],
  ['20220620/000123/003', 'evidence/pcap/000123/003.pcap', '32.1 MB', '8c19d4a2b9...cafe04d122', '06-19 02:00 ~ 03:00', 'TLS'],
  ['20220620/000123/004', 'evidence/pcap/000123/004.pcap', '27.6 MB', '0f9a3b2d11...9a6b3e7c90', '06-19 03:00 ~ 04:00', 'DNS'],
  ['20220620/000123/005', 'evidence/pcap/000123/005.pcap', '31.9 MB', 'd6c2e703f4...669e2b8a11', '06-19 04:00 ~ 05:00', 'TLS'],
];

const sessionRows = [
  ['03:41:55', 'TLS', '172.16.5.10:44221', '185.22.14.9:443', '1.23 MB', '12.45 s'],
  ['03:41:20', 'TLS', '172.16.5.10:44222', '185.22.14.9:443', '512 KB', '5.16 s'],
  ['03:39:44', 'TLS', '172.16.5.10:44323', '104.16.12.34:443', '845 KB', '8.37 s'],
  ['03:38:31', 'DNS', '172.16.5.10:53513', '8.8.8.8:53', '2.1 KB', '0.21 s'],
  ['03:38:12', 'HTTP', '172.16.5.10:51512', '198.51.100.27:80', '3.21 MB', '14.82 s'],
];

const exportPackages = [
  ['PKG-20260620-0012', '合规证据包', 'PCAP+Session+日志+报告', '24', '1.26 GB', '完成'],
  ['PKG-20260620-0010', '会话与日志', 'Session+日志(CSV)', '12', '256 MB', '完成'],
  ['PKG-20260619-0098', '原始 PCAP', 'PCAP 原始包', '10', '512 MB', '完成'],
];

const hashRows = [
  ['000123_001.pcap', 'SHA256', 'a1d27c3b2a...b7be95f1c3', 'a1d27c3b2a...b7be95f1c3', '匹配', '06-20 03:44:21'],
  ['000123_002.pcap', 'SHA256', 'c3f8b6d1e4...a1e6d8f4b2', 'c3f8b6d1e4...a1e6d8f4b2', '匹配', '06-20 03:44:15'],
  ['000123_003.pcap', 'SHA256', '8c19d4a2b9...cafe04d122', '8c19d4a2b9...cafe04d122', '匹配', '06-20 03:44:15'],
];

const auditRows = [
  ['06-20 03:44:58', 'sec_analyst', '下载 PCAP', 'F-20260620-000199', '成功'],
  ['06-20 03:44:21', 'sec_analyst', '导出 CSV', 'F-20260620-000189', '成功'],
  ['06-20 03:43:50', 'sec_analyst', '校验 hash', 'F-20260620-000189', '成功'],
  ['06-20 03:42:31', 'system', '生成签名 URL', 'F-20260620-000189', '成功'],
  ['06-20 03:41:12', 'sec_analyst', '新建取证', 'F-20260620-000189', '成功'],
];

const forensicsOverlays: OverlayContract[] = [
  {
    id: 'modal-forensics-task',
    title: '取证任务详情',
    kind: 'Modal',
    actionLabel: '任务详情',
    description: '展示取证任务条件、采集进度、PCAP 切片、hash 校验与失败原因。',
  },
  {
    id: 'popconfirm-pcap-download',
    title: 'PCAP 下载确认',
    kind: 'Popconfirm',
    actionLabel: '下载确认',
    description: '确认下载对象、有效期、签名 URL 和授权边界。',
    impact: '下载原始证据文件，必须留存审计记录。',
    danger: true,
  },
  {
    id: 'drawer-session-replay',
    title: '会话复放抽屉',
    kind: 'Drawer',
    actionLabel: '会话复放',
    description: '按五元组和时间窗复放会话摘要、协议解析和证据链。',
  },
  {
    id: 'modal-forensics-evidence-export',
    title: '取证证据导出',
    kind: 'Modal',
    actionLabel: '证据导出',
    description: '导出 PCAP、Session、日志、hash 与审计记录组成的证据包。',
    impact: '生成可下载证据包并写入 MinIO 与审计日志。',
  },
];

type ForensicsAction = { title: string; target: string; endpoint: string; auditEvent: string };

export function ForensicsWorkbenchPage({ route }: { route: NavRoute }) {
  const [listPage, setListPage] = useState(1);
  const [protocol, setProtocol] = useState('全部');
  const [asset, setAsset] = useState('请选择资产');
  const [action, setAction] = useState<ForensicsAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildForensicsRows(data?.rows ?? []), [data?.rows]);
  const pageSize = 5;
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const visibleRows = rows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderForensicCell(column, value, record, openAction),
  }));
  function openAction(title: string, target = String(rows[0]?.['任务 ID'] ?? 'F-20260620-000189')) {
    setActionSubmitted(false);
    setAction(createForensicsAction(title, target));
  }

  return (
    <div className="taf-page taf-forensics">
      <section className="taf-forensics-shell">
        <header className="taf-forensics-titlebar">
          <div>
            <h1>{route.page.title}</h1>
            <span>证据检索、会话复放与导出</span>
          </div>
          <div className="taf-forensics-source">
            <b>来源上下文</b>
            <button type="button" onClick={() => openAction('查看关联告警', 'AL-20260620-000123')}>告警(AL-20260620-000123)</button>
            <button type="button" onClick={() => openAction('查看关联战役', 'APT-20260619-001')}>战役(APT-20260619-001)</button>
            <button type="button" onClick={() => openAction('查看关联资产', '办公区-WS-1024')}>资产(办公区-WS-1024)</button>
            <button type="button" onClick={() => openAction('查看图谱路径')}>图谱路径(点击查看)</button>
          </div>
          <Button size="small" type="link" onClick={() => { setProtocol('全部'); setAsset('请选择资产'); setListPage(1); }}>清空条件</Button>
          <OverlayContractHost overlays={forensicsOverlays} compact />
        </header>

        {isError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={error instanceof Error ? error.message : '请检查 /v1/pcap/jobs、/v1/pcap/stats、APISIX 路由或 forensics-service。'}
            action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
          />
        )}

        <div className="taf-forensics-filter">
          <label>
            <span>时间窗</span>
            <Button size="small" icon={<CalendarOutlined />} onClick={() => openAction('调整时间窗')}>2026-06-19 00:00:00 ~ 2026-06-20 03:45:00</Button>
          </label>
          <label><span>资产</span><Select size="small" value={asset} options={[{ value: '请选择资产' }, { value: '办公区-WS-1024' }]} onChange={setAsset} /></label>
          <label><span>源 IP</span><Input size="small" placeholder="请输入源 IP" onChange={() => setListPage(1)} /></label>
          <label><span>目的 IP</span><Input size="small" placeholder="请输入目的 IP" onChange={() => setListPage(1)} /></label>
          <label><span>协议</span><Select size="small" value={protocol} options={[{ value: '全部' }, { value: 'TLS' }, { value: 'DNS' }, { value: 'HTTP' }]} onChange={setProtocol} /></label>
          <label><span>端口</span><Select size="small" value="全部" options={[{ value: '全部' }, { value: '443' }, { value: '53' }]} onChange={() => setListPage(1)} /></label>
          <label><span>五元组</span><Input size="small" placeholder="请输入五元组" onChange={() => setListPage(1)} /></label>
          <label><span>告警 ID</span><Input size="small" placeholder="请输入告警 ID" onChange={() => setListPage(1)} /></label>
          <Button size="small" onClick={() => { setProtocol('全部'); setAsset('请选择资产'); setListPage(1); }}>重置</Button>
          <Button size="small" type="primary" icon={<SearchOutlined />} onClick={() => openAction('执行取证查询')}>查询</Button>
          <Tooltip title="刷新取证数据">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
          </Tooltip>
        </div>

        <div className="taf-forensics-grid">
          <main className="taf-forensics-main">
            <div className="taf-forensics-upper">
              <WorkPanel title="取证任务状态机" extra={<Button size="small" type="primary" onClick={() => openAction('新建取证任务')}>新建任务</Button>}>
                <StateMachine />
              </WorkPanel>
              <WorkPanel title="会话复放 (Session)">
                <SessionReplay onAction={openAction} />
              </WorkPanel>
            </div>

            <div className="taf-forensics-middle">
              <WorkPanel title="取证任务列表（共 190 条）" className="taf-forensics-task-panel">
                <Table
                  rowKey={(record) => String(record['任务 ID'] ?? JSON.stringify(record))}
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  columns={columns}
                scroll={{ x: 900, y: 174 }}
                dataSource={visibleRows}
              />
              <div className="taf-forensics-pagination"><button type="button" aria-label="取证任务上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`取证任务第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="取证任务下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页，共 {rows.length} 条</span></div>
              </WorkPanel>
              <WorkPanel title="PCAP 索引（共 1,256 条）">
                <PcapIndex onAction={openAction} />
              </WorkPanel>
            </div>

            <div className="taf-forensics-lower">
              <WorkPanel title="证据导出包">
                <ExportPackages onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="hash 校验结果（最近 20 条）">
                <HashResults onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="返回来源">
                <ReturnSources onAction={openAction} />
              </WorkPanel>
            </div>
          </main>

          <aside className="taf-forensics-rail">
            <IntegrityPanel evidence={data?.evidence ?? []} onAction={openAction} />
            <SignedUrlPanel onAction={openAction} />
            <WorkPanel title="取证操作">
              <ActionGrid onAction={openAction} />
            </WorkPanel>
            <AuditPanel onAction={openAction} />
          </aside>
        </div>
      </section>
      <Drawer className="taf-forensics-action-drawer" title={action ? `${action.title}确认` : '取证操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为取证对象创建“{action.title}”仿真任务，并保留授权、证据与审计上下文。</p><dl><dt>取证对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="取证业务操作已进入仿真任务队列" />}</div>}
      </Drawer>
    </div>
  );
}

function StateMachine() {
  return (
    <div className="taf-forensics-state">
      {stateItems.map(([label, value, tone, icon], index) => (
        <div key={String(label)} className={`is-${tone}`}>
          <span>{icon}</span>
          <strong>{label}</strong>
          <em>{value}</em>
          {index < stateItems.length - 1 && <i />}
        </div>
      ))}
    </div>
  );
}

function SessionReplay({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-session">
      <div className="taf-forensics-session-filter">
        <Select size="small" value="全部协议" options={[{ value: '全部协议' }, { value: 'TLS' }]} onChange={(value) => onAction('筛选会话协议', value)} />
        <Select size="small" value="全部方向" options={[{ value: '全部方向' }, { value: '出站' }]} onChange={(value) => onAction('筛选会话方向', value)} />
        <Input size="small" placeholder="搜索域名 / URL / 内容" onChange={() => onAction('筛选会话内容')} />
        <Button size="small" onClick={() => onAction('导出会话')}>导出会话</Button>
      </div>
      <div className="taf-forensics-session-table">
        <div><span>开始时间</span><span>协议</span><span>源 IP:端口</span><span>目的 IP:端口</span><span>字节数</span><span>持续时间</span></div>
        {sessionRows.map(([time, protocol, src, dst, bytes, duration]) => (
          <button key={`${time}-${src}`} type="button" onClick={() => onAction('复放会话', `${src} -> ${dst}`)}>
            <span>{time}</span><strong>{protocol}</strong><span>{src}</span><span>{dst}</span><span>{bytes}</span><span>{duration}</span>
          </button>
        ))}
      </div>
      <div className="taf-forensics-packet-echart"><DataQualityKpiSparklineChart ariaLabel="会话数据包趋势" tone="info" values={[24, 38, 31, 46, 42, 56, 48, 61, 52, 67, 58, 72]} /></div>
      <div className="taf-forensics-payload">
        <pre>{'GET /update/check HTTP/1.1\nHost: update.example.com\nUser-Agent: Mozilla/5.0\nAccept: */*\nConnection: keep-alive'}</pre>
        <pre>{'HTTP/1.1 200 OK\nDate: Fri, 20 Jun 2026 03:41:55 GMT\nContent-Type: application/json\n{"status":"ok","ts":1750388515}'}</pre>
        <dl>
          <dt>域名</dt><dd>update.example.com</dd>
          <dt>SNI</dt><dd>update.example.com</dd>
          <dt>证书</dt><dd>有效</dd>
          <dt>持续时间</dt><dd>12.45 s</dd>
        </dl>
      </div>
    </div>
  );
}

function PcapIndex({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-pcap">
      <div><span>pcap_index</span><span>对象路径 (MinIO)</span><span>大小</span><span>hash (SHA256)</span><span>时间窗</span><span>协议</span></div>
      {pcapRows.map(([id, path, size, hash, range, protocol]) => (
        <button key={id} type="button" onClick={() => onAction('查看 PCAP 索引', id)}>
          <strong>{id}</strong><span>{path}</span><span>{size}</span><span>{hash}</span><span>{range}</span><StatusTag value={protocol} />
        </button>
      ))}
    </div>
  );
}

function ExportPackages({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-export">
      <div><span>包 ID</span><span>类型</span><span>内容</span><span>文件数</span><span>大小</span><span>状态</span><span>操作</span></div>
      {exportPackages.map(([id, type, content, files, size, status]) => (
        <button key={id} type="button" onClick={() => onAction('下载证据包', id)}>
          <strong>{id}</strong><span>{type}</span><span>{content}</span><span>{files}</span><span>{size}</span><StatusTag value={status} /><DownloadOutlined />
        </button>
      ))}
    </div>
  );
}

function HashResults({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-hash">
      <div><span>文件名</span><span>算法</span><span>计算值</span><span>参考值</span><span>结果</span><span>时间</span></div>
      {hashRows.map(([file, algo, value, ref, result, time]) => (
        <button key={file} type="button" onClick={() => onAction('查看 hash 校验', file)}>
          <strong>{file}</strong><span>{algo}</span><span>{value}</span><span>{ref}</span><StatusTag value={result} /><span>{time}</span>
        </button>
      ))}
    </div>
  );
}

function ReturnSources({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-return">
      {[
        ['返回告警详情', 'AL-20260620-000123'],
        ['返回战役详情', 'APT-20260619-001'],
        ['返回资产详情', '办公区-WS-1024'],
        ['返回实体图谱', '查看路径'],
      ].map(([label, value]) => (
        <button key={label} type="button" onClick={() => onAction(label, value)}><LinkOutlined /><span>{label}</span><strong>{value}</strong></button>
      ))}
    </div>
  );
}

function IntegrityPanel({ evidence, onAction }: { evidence: PageSnapshot['evidence']; onAction: (title: string, target?: string) => void }) {
  const score = evidence.find((item) => item.label.includes('Hash'))?.status === 'warn' ? '92' : '100';
  return (
    <WorkPanel title="证据完整性" extra={<Button size="small" type="text" aria-label="关闭证据完整性面板" onClick={() => onAction('关闭证据完整性面板')}>×</Button>}>
      <div className="taf-forensics-integrity">
        <div className="taf-forensics-integrity-score">
          <SafetyCertificateOutlined />
          <span>完整性评分</span>
          <strong>{score}<small>/100</small></strong>
          <em>校验时间 2026-06-20 03:45:00</em>
        </div>
        {['原始文件校验', '文件 hash 校验 (SHA256)', '数字签名验证', '签名人', '证书有效期'].map((item, index) => (
          <div key={item}><CheckCircleOutlined /><span>{item}</span><strong>{index === 1 ? 'a1d27c3b2a...b7be95f1c3' : index === 3 ? '流量采集平台' : index === 4 ? '2026-01-01 ~ 2027-01-01' : '通过'}</strong><em>通过</em></div>
        ))}
      </div>
    </WorkPanel>
  );
}

function SignedUrlPanel({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <WorkPanel title="签名 URL 与有效期" extra={<Button size="small" type="text" aria-label="关闭签名URL面板" onClick={() => onAction('关闭签名URL面板')}>×</Button>}>
      <div className="taf-forensics-signed">
        <div><span>类型</span><span>签名 URL</span><span>过期时间</span><span>状态</span></div>
        {['PCAP', 'Session', '日志'].map((type) => (
          <button key={type} type="button" onClick={() => onAction('查看签名 URL', type)}><strong>{type}</strong><span>https://minio.local/signed/...</span><span>2026-06-27 03:45</span><StatusTag value="有效" /></button>
        ))}
      </div>
    </WorkPanel>
  );
}

function ActionGrid({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-forensics-actions">
      {[
        ['新建取证任务', <FileProtectOutlined key="new" />],
        ['追加时间窗', <CalendarOutlined key="time" />],
        ['关联到战役', <ApiOutlined key="campaign" />],
        ['关联告警', <AuditOutlined key="alert" />],
        ['进入图谱', <LinkOutlined key="graph" />],
        ['进入取证分析', <FileSearchOutlined key="forensics" />],
      ].map(([label, icon]) => <Button key={String(label)} size="small" icon={icon} onClick={() => onAction(String(label))}>{label}</Button>)}
    </div>
  );
}

function AuditPanel({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <WorkPanel title="审计日志（近 24 小时）" extra={<Button size="small" type="link" onClick={() => onAction('查看全部取证审计')}>查看全部</Button>}>
      <div className="taf-forensics-audit">
        <div><span>时间</span><span>操作人</span><span>操作</span><span>对象</span><span>结果</span></div>
        {auditRows.map(([time, user, action, target, result]) => (
          <button key={`${time}-${action}`} type="button" onClick={() => onAction('查看取证审计', target)}><span>{time}</span><strong>{user}</strong><span>{action}</span><span>{target}</span><StatusTag value={result} /></button>
        ))}
      </div>
    </WorkPanel>
  );
}

function renderForensicCell(column: string, value: unknown, row: SnapshotRow, onAction: (title: string, target?: string) => void) {
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return <Button size="small" type="link" onClick={() => onAction('查看取证任务', String(row['任务 ID'] ?? value))}>{String(value)}</Button>;
  if (column === '任务 ID' || column === '告警/战役 ID') return <span className="taf-forensics-link-cell"><EyeOutlined />{String(value)}</span>;
  if (column === '证据包') return <span className="taf-forensics-package-cell"><FileSearchOutlined />{String(value)}</span>;
  return String(value ?? '-');
}

const buildForensicsRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ '任务 ID': 'F-20260620-000189', '告警/战役 ID': 'AL-20260620-000123', 状态: '完成', 操作: '查看' }];
  if (source.length >= 15) return source;
  return Array.from({ length: 15 }, (_, index) => index < source.length ? source[index] : { ...source[index % source.length], '任务 ID': `${String(source[index % source.length]['任务 ID'] ?? 'F')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}` });
};

const createForensicsAction = (title: string, target: string): ForensicsAction => {
  const cancel = pageApiPlans.forensics.actions?.find((item) => item.id === 'forensics-cancel-job');
  const plan = title.includes('取消') ? cancel : undefined;
  return { title, target, endpoint: plan?.endpoint ?? (title.includes('下载') || title.includes('导出') ? '/v1/pcap/evidence/export' : '/v1/pcap/jobs/{id}'), auditEvent: plan?.auditEvent ?? 'FORENSICS_ACTION_SIMULATED' };
};
