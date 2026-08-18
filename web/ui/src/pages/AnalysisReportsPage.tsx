import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Space, Table, Tag } from 'antd';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchAnalysisReports, type AnalysisReportView , friendlyAnalysisError } from '@/services/analysisApi';

/** 报告状态确定性文案(§18.2);未知枚举 fail-closed。 */
const REPORT_STATE: Record<string, { label: string; color: string }> = {
  NOT_REQUESTED: { label: '未申请', color: 'default' },
  QUEUED: { label: '排队中', color: 'default' },
  GENERATING: { label: '生成中', color: 'processing' },
  VERIFYING: { label: '对象校验中', color: 'processing' },
  AVAILABLE: { label: '可下载', color: 'green' },
  FAILED: { label: '生成失败', color: 'red' },
  CANCELLED: { label: '已取消', color: 'default' },
};

function reportStateView(state: string) {
  const known = REPORT_STATE[state];
  if (!known) {
    return <Tag color="default">状态无法确认({state})</Tag>;
  }
  return <Tag color={known.color}>{known.label}</Tag>;
}

/**
 * 报告中心(§7.5 六轴正交):人读报告列表独立 ReportState;
 * 机器摘要随 Run 终态冻结(经运行详情查询),报告失败不回退 Run。
 */
export function AnalysisReportsPage({ route }: { route: NavRoute }) {
  const [rows, setRows] = useState<AnalysisReportView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRows(await fetchAnalysisReports());
    } catch (e) {
      setError(friendlyAnalysisError(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const columns = [
    { title: '运行', dataIndex: 'RunID', ellipsis: true },
    { title: '机器摘要哈希', dataIndex: 'SummarySHA256', render: (v: string) => v.slice(0, 16) + '…' },
    { title: '模板/语言', key: 'tpl', render: (_: unknown, r: AnalysisReportView) => `${r.TemplateRevision} / ${r.Locale}` },
    { title: '状态', dataIndex: 'State', render: (v: string) => reportStateView(v) },
    { title: '大小', dataIndex: 'ObjectSize', render: (v: number) => (v > 0 ? `${v} B` : '—') },
    {
      title: '对象(仅 AVAILABLE 可下载)', dataIndex: 'ObjectKey', ellipsis: true,
      render: (v: string, r: AnalysisReportView) =>
        r.State === 'AVAILABLE' && v ? (
          <span title={`对象 sha256: ${r.ObjectSHA256}`}>{v}</span>
        ) : '—',
    },
  ];

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <Space style={{ marginBottom: 12 }}>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      {error ? <Alert type="error" message={error} style={{ marginBottom: 12 }} /> : null}
      <Table
        rowKey="ReportID"
        loading={loading}
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: '暂无人读报告;在运行详情对已终态 Run 请求生成' }}
      />
    </div>
  );
}
