import {
  AuditOutlined,
  CheckCircleOutlined,
  FileDoneOutlined,
  FilePdfOutlined,
  FileSearchOutlined,
  FileWordOutlined,
  FolderOpenOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const indicatorRows = [
  ['采集覆盖 >= 95%', '采集探针覆盖率', '采集监测 / 探针管理', '探针元数据', '采集感知', '通过'],
  ['数据质量 >= 90%', '数据完整性、去重率', '采集监测 / 数据质量', 'Kafka / Flink / ClickHouse', '采集监测', '通过'],
  ['告警链路 <= 5 分钟', '告警闭环、关键证据率', '威胁分析 / 告警中心', '告警 / 关联引擎', '威胁分析', '待整改'],
  ['PCAP 证据覆盖 >= 90%', 'PCAP hash 命中率', '威胁分析 / PCAP 检索', 'PCAP 存储', '威胁分析', '通过'],
  ['模型效果 F1 >= 0.80', '模型效果评估', '检测运营 / 模型管理', '模型服务 / 反馈池', '检测运营', '待整改'],
  ['审计链路完整', '操作留痕完整性', '审计配置 / 审计日志', '审计日志 / 操作日志', '审计配置', '通过'],
];

const gapRows = [
  ['阻断', '部署基线一致性', '部分节点基础镜像差异', '检测运营', '2026-06-25', '未复验'],
  ['阻断', '告警链路时延', '关联引擎负载偏高', '威胁分析', '2026-06-23', '整改中'],
  ['一般', 'MLOps 闭环完整性', '反馈回流未全量', '检测运营', '2026-06-26', '未开始'],
  ['一般', '证据链路-规则版本', '历史版本缺失', '检测运营', '2026-06-22', '待复验'],
  ['信息', '数据质量波动', '周末流量波动大', '采集监测', '2026-06-21', '通过'],
];

const batchRows = [
  ['PT-202506-02', '第三方评测平台', '全量样本', '86.4%', '1/1 通过', '上升'],
  ['PT-202505-01', '第三方评测平台', '全量样本', '82.1%', '2/2 通过', '上升'],
  ['PT-202504-03', '第三方评测平台', '增量样本', '79.3%', '1/2 通过', '上升'],
  ['PT-202503-02', '第三方评测平台', '全量样本', '81.7%', '1/2 通过', '持平'],
  ['PT-202502-01', '第三方评测平台', '基线样本', '76.5%', '-', '持平'],
];

const complianceOverlays: OverlayContract[] = [
  {
    id: 'drawer-compliance-gate-detail',
    title: '合规门禁详情',
    kind: 'Drawer',
    actionLabel: '门禁详情',
    description: '展示门禁指标、证据来源、整改状态和验收结论。',
  },
  {
    id: 'modal-compliance-evidence-package-export',
    title: '合规证据包导出',
    kind: 'Modal',
    actionLabel: '证据包导出',
    description: '导出合规证据包，包含门禁、审计、运行报告和整改记录。',
    impact: '生成可供验收的证据材料并写入审计。',
  },
  {
    id: 'modal-compliance-report-export',
    title: '合规运行报告导出',
    kind: 'Modal',
    actionLabel: '运行报告导出',
    description: '导出指定时间窗内的运行指标、合规状态和第三方评测记录。',
  },
];

export function ComplianceAuditPage({ route }: { route: NavRoute }) {
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderComplianceCell(column, value),
  }));

  return (
    <div className="taf-page taf-compliance">
      <section className="taf-compliance-shell">
        <main className="taf-compliance-main">
          <header className="taf-compliance-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={8}>
              <Button size="small" type="primary" icon={<FileDoneOutlined />}>生成验收报告</Button>
              <Button size="small" icon={<FolderOpenOutlined />}>导出证据包</Button>
              <Button size="small" icon={<FilePdfOutlined />}>导出 PDF</Button>
              <Button size="small" icon={<FileWordOutlined />}>导出 Word</Button>
              <Button size="small" icon={<ToolOutlined />}>创建整改任务</Button>
              <Button size="small" icon={<SafetyCertificateOutlined />}>固化验收记录</Button>
              <Tooltip title="刷新合规报告">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={complianceOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/compliance/reports、/v1/compliance/audit-trail、APISIX 路由或 alert-service。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-compliance-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-compliance-workbench">
            <section className="taf-compliance-left">
              <WorkPanel title="A. 验收门禁矩阵" extra={<AuditOutlined />}>
                <Table
                  rowKey={rowKey}
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  columns={columns}
                  dataSource={rows.slice(0, 7)}
                />
              </WorkPanel>

              <WorkPanel title="B. 指标映射追踪表">
                <IndicatorTrace />
              </WorkPanel>
            </section>

            <section className="taf-compliance-righttop">
              <WorkPanel title="C. 证据包完整度" extra={<FileSearchOutlined />}>
                <EvidencePackage data={data} />
              </WorkPanel>
              <WorkPanel title="D. 运行报告预览（时间窗：2026-06-14 ~ 2026-06-21）">
                <ReportPreview data={data} />
              </WorkPanel>
            </section>

            <WorkPanel title="E. 缺口治理看板" className="taf-compliance-gap-panel">
              <GapBoard />
            </WorkPanel>

            <WorkPanel title="F. 第三方评测批次" className="taf-compliance-batch-panel">
              <ThirdPartyBatches />
            </WorkPanel>
          </div>
        </main>
      </section>
    </div>
  );
}

function IndicatorTrace() {
  return (
    <div className="taf-compliance-indicators">
      <div><span>任务书指标</span><span>测试项</span><span>对应页面</span><span>对应数据源</span><span>责任模块</span><span>状态</span></div>
      {indicatorRows.map((row) => (
        <button key={row[0]} type="button">
          {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 5 ? statusClass(cell) : ''}>{cell}</span>)}
        </button>
      ))}
    </div>
  );
}

function EvidencePackage({ data }: { data?: PageSnapshot }) {
  const evidence = data?.evidence?.length ? data.evidence : [];
  return (
    <div className="taf-compliance-evidence">
      <div><span>证据项</span><span>编号 / hash（前12位）</span><span>校验状态</span><span>最后更新时间</span></div>
      {evidence.slice(1, 7).map((item, index) => (
        <button key={item.label} type="button">
          <span>{item.label}</span>
          <span>{evidenceCode(item.label, index)}</span>
          <span className={item.status === 'warn' || item.status === 'risk' ? 'is-warn' : 'is-ok'}>{item.status === 'warn' ? '待复核' : '通过'}</span>
          <span>2026-06-{String(21 - index).padStart(2, '0')} {String(10 + index).padStart(2, '0')}:24:11</span>
        </button>
      ))}
    </div>
  );
}

function ReportPreview({ data }: { data?: PageSnapshot }) {
  const reportMetrics = [
    ['总告警数', '12,456', '较上周 ↓ 8.7%'],
    ['处置完成', '11,203', '处置率 90.0%'],
    ['数据质量', metricValue(data, '证据完整度', '95.1%'), '较上周 ↑ 1.6%'],
    ['系统健康', '98.7%', '健康度'],
    ['模型 F1', '0.872', '较上周 ↑ 0.031'],
  ];
  return (
    <div className="taf-compliance-report">
      <div className="taf-compliance-report-metrics">
        {reportMetrics.map(([label, value, note]) => (
          <span key={label}><em>{label}</em><b>{value}</b><small>{note}</small></span>
        ))}
      </div>
      <div className="taf-compliance-report-trends">
        {['告警趋势（条/日）', '处置趋势（条/日）', '数据质量趋势（%）', '系统健康趋势（%）'].map((title, index) => (
          <span key={title}><b>{title}</b><i className={`is-${index}`} /></span>
        ))}
      </div>
      <footer>摘要：本时间窗内系统运行总体稳定，告警处置及时率 90.0%，数据质量与模型效果持续提升。</footer>
    </div>
  );
}

function GapBoard() {
  return (
    <div className="taf-compliance-gap">
      <div><span>未达标项</span><span>原因</span><span>责任模块</span><span>计划完成</span><span>复验状态</span><span>操作</span></div>
      {gapRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button">
          <span className={riskClass(row[0])}>{row[0]}</span>
          {row.slice(1).map((cell, index) => <span key={`${row[1]}-${index}`} className={index === 3 ? statusClass(cell) : ''}>{cell}</span>)}
          <em>创建任务 / 复验</em>
        </button>
      ))}
      <footer><span>共 12 项</span><span>阻断 3</span><span>一般 7</span><span>信息 2</span></footer>
    </div>
  );
}

function ThirdPartyBatches() {
  return (
    <div className="taf-compliance-batches">
      <div><span>批次号</span><span>样本来源</span><span>测试范围</span><span>通过率</span><span>复测记录</span><span>趋势</span></div>
      {batchRows.map((row) => (
        <button key={row[0]} type="button">
          {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 4 ? statusClass(cell) : ''}>{cell}</span>)}
          <i />
        </button>
      ))}
      <footer><span>共 5 条</span><Select size="small" value="10 / 页" options={[{ value: '10 / 页' }]} /></footer>
    </div>
  );
}

const renderComplianceCell = (column: string, value: unknown) => {
  if (column.includes('覆盖率') || column.includes('完整度')) return <span className="taf-compliance-bar"><em style={{ width: percentWidth(value) }} /><b>{String(value)}</b></span>;
  if (column === '维度') return <span className="taf-compliance-dimension"><CheckCircleOutlined />{String(value)}</span>;
  if (column === '结果') return <StatusTag value={value} />;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.维度 ?? JSON.stringify(row));

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') || label.includes('完整度') ? '0.0%' : '0',
  delta: 'API',
  status: 'info',
});

const statusClass = (value: string) => {
  if (value.includes('未') || value.includes('阻断')) return 'is-risk';
  if (value.includes('待') || value.includes('整改') || value.includes('一般')) return 'is-warn';
  return 'is-ok';
};

const riskClass = (value: string) => {
  if (value.includes('阻断')) return 'is-risk';
  if (value.includes('一般')) return 'is-warn';
  return 'is-info';
};

const percentWidth = (value: unknown) => {
  const match = String(value).match(/(\d+(?:\.\d+)?)%/);
  const percent = match ? Number(match[1]) : 82;
  return `${Math.max(8, Math.min(100, percent))}%`;
};

const evidenceCode = (label: string, index: number) => {
  const prefix = label.includes('PCAP') ? 'PCAP-SET' : label.includes('审计') ? 'LOG' : label.includes('模型') ? 'MODEL' : label.includes('规则') ? 'RULESET' : label.includes('部署') ? 'MANIFEST' : 'RPT';
  return `${prefix}-202606-${String(index + 12).padStart(2, '0')}`;
};

const metricValue = (data: PageSnapshot | undefined, label: string, fallback: string) =>
  data?.metrics.find((metric) => metric.label === label)?.value ?? fallback;
