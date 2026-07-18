import {
  ApiOutlined,
  CalendarOutlined,
  CloudServerOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileSearchOutlined,
  GlobalOutlined,
  KeyOutlined,
  LockOutlined,
  RadarChartOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Empty, Input, Modal, Pagination, Select, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { EncryptedJa3ScatterChart, EncryptedProtocolTrendChart, EvidenceClosureRingChart, EvidenceEntropyTrendChart, ExfilGraphChart, ExfilStackedTrendChart, HeartbeatTrendChart, PcapPacketTrendChart, WorldActivityMap } from '@/components/charts';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot, submitEncryptedTrafficEgressAction, submitEncryptedTrafficEvidenceAction } from '@/services/api';
import type { EncryptedTrafficTimeRange } from '@/services/api';
import type { EncryptedTrafficVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';

const encryptedTrafficTabs = [
  { label: '总览', slug: 'overview' },
  { label: '指纹分析', slug: 'fingerprint' },
  { label: '隧道检测', slug: 'tunnel-detection' },
  { label: '外联画像', slug: 'egress-profile' },
  { label: '证据中心', slug: 'evidence-center' },
] as const;

type EncryptedTrafficTabSlug = (typeof encryptedTrafficTabs)[number]['slug'];

type EgressAction = {
  id: 'egress-create-alert' | 'egress-evidence-lookup' | 'egress-entity-graph' | 'egress-audit-write' | 'egress-response-request';
  label: string;
  target: string;
  description: string;
  successMessage: string;
  navigateTo?: string;
};

type EvidenceAction = {
  id:
    | 'evidence-create-task'
    | 'evidence-download-pcap'
    | 'evidence-verify-hash'
    | 'evidence-export-package'
    | 'evidence-associate-analysis'
    | 'evidence-preserve'
    | 'evidence-link-alert'
    | 'evidence-expert-review'
    | 'evidence-gap-mark'
    | 'evidence-submit-recommendation'
    | 'evidence-export-report'
    | 'evidence-write-audit';
  label: string;
  target: string;
  description: string;
  successMessage: string;
};

type EvidenceLocateFilters = {
  query: string;
  protocol: string;
  risk: string;
  sniScope: string;
};

const initialEvidenceLocateFilters: EvidenceLocateFilters = {
  query: '',
  protocol: '全部',
  risk: '全部',
  sniScope: '全部',
};

function createEgressAction(label: string, target: string, description?: string): EgressAction {
  if (label === '创建外联告警') {
    return {
      id: 'egress-create-alert',
      label,
      target,
      description: description ?? `将为 ${target} 提交待审核的外联告警请求，并写入服务端审计。`,
      successMessage: '外联告警请求已写入审计，等待规则服务评估。',
    };
  }
  if (label === '查看目的地证据') {
    return {
      id: 'egress-evidence-lookup',
      label,
      target,
      description: description ?? `将记录对 ${target} 的证据检索，并打开证据中心。`,
      successMessage: '目的地证据检索已写入审计。',
      navigateTo: `/encrypted-traffic?tab=evidence-center&destination=${encodeURIComponent(target)}`,
    };
  }
  if (label === '跳转实体图谱' || label === '查看实体图谱') {
    return {
      id: 'egress-entity-graph',
      label,
      target,
      description: description ?? `将记录对 ${target} 的实体图谱下钻，并打开实体关系视图。`,
      successMessage: '实体图谱下钻已写入审计。',
      navigateTo: `/graph?focus=${encodeURIComponent(target)}`,
    };
  }
  if (label === '写入审计日志') {
    return {
      id: 'egress-audit-write',
      label,
      target,
      description: description ?? `将把 ${target} 的外联研判写入服务端审计日志。`,
      successMessage: '外联研判已写入服务端审计。',
    };
  }
  return {
    id: 'egress-response-request',
    label,
    target,
    description: description ?? `将为 ${target} 提交“${label}”处置请求，并写入服务端审计。`,
    successMessage: '处置请求已写入审计，等待审批或编排服务处理。',
  };
}

function createEvidenceAction(label: string, target: string): EvidenceAction {
  const details: Record<string, Pick<EvidenceAction, 'id' | 'description' | 'successMessage'>> = {
    '创建取证任务': { id: 'evidence-create-task', description: `将为 ${target} 提交加密证据采集任务请求，并写入服务端审计。`, successMessage: '取证任务请求已写入审计，等待取证服务处理。' },
    '下载 PCAP': { id: 'evidence-download-pcap', description: `将为 ${target} 提交 PCAP 下载请求，并写入服务端审计。`, successMessage: 'PCAP 下载请求已写入审计，等待取证服务处理。' },
    '校验证据 Hash': { id: 'evidence-verify-hash', description: `将为 ${target} 提交 Hash 校验请求，并写入服务端审计。`, successMessage: 'Hash 校验请求已写入审计，等待取证服务处理。' },
    '导出证据包': { id: 'evidence-export-package', description: `将为 ${target} 提交证据包导出请求，并写入服务端审计。`, successMessage: '证据包导出请求已写入审计，等待取证服务处理。' },
    '关联证据分析': { id: 'evidence-associate-analysis', description: `将为 ${target} 建立关联证据分析请求，并写入服务端审计。`, successMessage: '关联分析请求已写入审计，等待证据服务处理。' },
    '生成取证任务': { id: 'evidence-create-task', description: `将为 ${target} 生成取证任务请求，并写入服务端审计。`, successMessage: '取证任务请求已写入审计，等待取证服务处理。' },
    '申请证据保全': { id: 'evidence-preserve', description: `将为 ${target} 申请证据保全，并写入服务端审计。`, successMessage: '证据保全请求已写入审计，等待取证服务处理。' },
    '关联告警': { id: 'evidence-link-alert', description: `将为 ${target} 提交告警关联请求，并写入服务端审计。`, successMessage: '告警关联请求已写入审计。' },
    '发起专家复核': { id: 'evidence-expert-review', description: `将为 ${target} 发起专家复核请求，并写入服务端审计。`, successMessage: '专家复核请求已写入审计。' },
    '标记证据缺口': { id: 'evidence-gap-mark', description: `将为 ${target} 标记证据缺口，并写入服务端审计。`, successMessage: '证据缺口标记已写入审计。' },
    '提交处置建议': { id: 'evidence-submit-recommendation', description: `将为 ${target} 提交处置建议，并写入服务端审计。`, successMessage: '处置建议已写入审计。' },
    '导出证据报告': { id: 'evidence-export-report', description: `将为 ${target} 提交证据报告导出请求，并写入服务端审计。`, successMessage: '证据报告导出请求已写入审计。' },
  };
  const detail = details[label] ?? { id: 'evidence-write-audit' as const, description: `将把 ${target} 的证据研判写入服务端审计日志。`, successMessage: '证据研判已写入服务端审计。' };
  return { label, target, ...detail };
}

const resolveEncryptedTrafficTab = (param: string | null): EncryptedTrafficTabSlug => (
  encryptedTrafficTabs.find((item) => item.slug === param)?.slug ?? 'overview'
);

const encryptedTrafficOverlays: OverlayContract[] = [
  {
    id: 'drawer-encrypted-fingerprint',
    title: '加密指纹详情',
    kind: 'Drawer',
    actionLabel: '指纹详情',
    description: '展示 JA3/JA4、SNI、ALPN、目的地信誉、证据样本和命中规则。',
  },
  {
    id: 'drawer-certificate-detail',
    title: '证书详情',
    kind: 'Drawer',
    actionLabel: '证书详情',
    description: '展示证书链、签发机构、有效期、域名绑定和异常证据。',
  },
];

export function EncryptedTrafficPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = resolveEncryptedTrafficTab(searchParams.get('tab'));
  const destinationFromUrl = searchParams.get('destination')?.trim() ?? '';
  const [selectedEgressDestination, setSelectedEgressDestination] = useState(destinationFromUrl);
  const [selectedEvidenceSession, setSelectedEvidenceSession] = useState('');
  const [egressAction, setEgressAction] = useState<EgressAction>();
  const [evidenceAction, setEvidenceAction] = useState<EvidenceAction>();
  const [timeRange, setTimeRange] = useState<EncryptedTrafficTimeRange>('近 24 小时');
  const [evidenceLocateFilters, setEvidenceLocateFilters] = useState<EvidenceLocateFilters>({
    ...initialEvidenceLocateFilters,
    query: destinationFromUrl,
  });
  const [isAnalysisRunning, setIsAnalysisRunning] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, timeRange],
    queryFn: () => fetchPageSnapshot(route.id, { timeRange }),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const encryptedVisuals = data?.visuals?.encryptedTraffic;
  const evidenceSessions = encryptedVisuals?.evidenceCenter.sessions ?? [];
  const hasActiveEvidenceLocateFilters = hasEvidenceLocateFilters(evidenceLocateFilters);
  const locatedEvidenceSessions = useMemo(
    () => evidenceSessions.filter((item) => matchesEvidenceLocateFilters(item, evidenceLocateFilters)),
    [evidenceLocateFilters, evidenceSessions],
  );
  const currentEgressTarget = selectedEgressDestination || encryptedVisuals?.destinationRows[0]?.[0] || '未选择对象';
  const selectedEvidenceTarget = locatedEvidenceSessions.find((item) => item.sessionId === selectedEvidenceSession)?.sessionId;
  const currentEvidenceTarget = selectedEvidenceTarget
    || locatedEvidenceSessions[0]?.sessionId
    || (!hasActiveEvidenceLocateFilters ? evidenceSessions[0]?.sessionId : '')
    || '证据中心-未选择对象';
  useEffect(() => {
    if (selectedEgressDestination && !encryptedVisuals?.destinationRows.some((row) => row[0] === selectedEgressDestination)) {
      setSelectedEgressDestination('');
    }
  }, [encryptedVisuals?.destinationRows, selectedEgressDestination]);
  useEffect(() => {
    if (selectedEvidenceSession && !evidenceSessions.some((item) => item.sessionId === selectedEvidenceSession)) {
      setSelectedEvidenceSession('');
    }
  }, [evidenceSessions, selectedEvidenceSession]);
  const egressActionMutation = useMutation({
    mutationFn: (action: EgressAction) => submitEncryptedTrafficEgressAction({
      actionId: action.id,
      target: action.target,
      dataMode: encryptedVisuals?.egressAvailability.state ?? 'unavailable',
    }),
  });
  const evidenceActionMutation = useMutation({
    mutationFn: (action: EvidenceAction) => submitEncryptedTrafficEvidenceAction({
      actionId: action.id,
      target: action.target,
      dataMode: encryptedVisuals?.evidenceCenter.availability.state ?? 'unavailable',
    }),
  });
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const evidenceKpis = encryptedVisuals?.evidenceCenter.kpis ?? [];
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderEncryptedCell(column, value),
  }));
  const openEgressAction = (label: string, target = currentEgressTarget, description?: string) => {
    setEgressAction(createEgressAction(label, target, description));
  };
  const openEvidenceAction = (label: string, target: string) => setEvidenceAction(createEvidenceAction(label, target));
  const refreshForTimeRange = async (successMessage?: string) => {
    const result = await refetch();
    if (result.isError) {
      message.error('加密流量数据刷新失败，请检查 API 状态。');
      return false;
    }
    if (successMessage) message.success(successMessage);
    return true;
  };
  const handleTimeRangeChange = (value: EncryptedTrafficTimeRange) => {
    setTimeRange(value);
  };
  const runEncryptedTrafficAnalysis = async () => {
    setIsAnalysisRunning(true);
    try {
      await evidenceActionMutation.mutateAsync(createEvidenceAction('关联证据分析', currentEvidenceTarget));
      await refreshForTimeRange(`已按${timeRange}提交关联证据分析请求。`);
    } catch (submissionError) {
      const detail = submissionError instanceof Error ? submissionError.message : '服务端未确认关联分析请求。';
      message.error(`一键分析提交失败：${detail}`);
    } finally {
      setIsAnalysisRunning(false);
    }
  };
  const exportEgressReport = () => {
    const payload = JSON.stringify({
      generatedAt: new Date().toISOString(),
      target: currentEgressTarget,
      availability: encryptedVisuals?.egressAvailability,
      destinations: encryptedVisuals?.destinationRows ?? [],
      domains: encryptedVisuals?.egressDomainCards ?? [],
    }, null, 2);
    const href = URL.createObjectURL(new Blob([payload], { type: 'application/json' }));
    const anchor = document.createElement('a');
    anchor.href = href;
    anchor.download = `encrypted-egress-${Date.now()}.json`;
    anchor.style.display = 'none';
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(href), 0);
    message.success('外联画像 JSON 报告已生成');
  };

  return (
    <div className={`taf-page taf-encrypted taf-encrypted--${activeTab}`} data-tab-slug={activeTab}>
      <section className="taf-encrypted-shell">
        <header className="taf-encrypted-titlebar">
          <h1>{route.page.title}</h1>
          <div className="taf-encrypted-tabs" role="tablist" aria-label="加密流量分析视图">
            {encryptedTrafficTabs.map((tab) => (
              <button
                key={tab.slug}
                type="button"
                className={tab.slug === activeTab ? 'is-active' : ''}
                aria-selected={tab.slug === activeTab}
                role="tab"
                onClick={() => setSearchParams((current) => {
                  const next = new URLSearchParams(current);
                  next.set('tab', tab.slug);
                  return next;
                })}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <div className="taf-encrypted-controls">
            <label>
              <span>时间范围</span>
              <Select size="small" value={timeRange} onChange={handleTimeRangeChange} options={[{ value: '近 1 小时' }, { value: '近 24 小时' }, { value: '近 7 天' }]} />
            </label>
            <Tooltip title="刷新加密流量">
              <Button size="small" icon={<ReloadOutlined />} aria-label="刷新加密流量" onClick={() => void refreshForTimeRange()} />
            </Tooltip>
            <Button size="small" type="primary" icon={<ThunderboltOutlined />} loading={isAnalysisRunning} onClick={() => void runEncryptedTrafficAnalysis()}>一键分析</Button>
            <OverlayContractHost overlays={encryptedTrafficOverlays} compact />
          </div>
        </header>

        {isError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={error instanceof Error ? error.message : '请检查 /v1/encrypted-traffic/*、APISIX 路由或后端服务。'}
            action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
          />
        )}

        <EncryptedTrafficKpis
          activeTab={activeTab}
          metrics={metrics}
          evidenceKpis={evidenceKpis}
          visuals={encryptedVisuals}
        />

        <div className="taf-encrypted-grid">
          <main className="taf-encrypted-main">
            <EncryptedTrafficTabView
              activeTab={activeTab}
              columns={columns}
              encryptedVisuals={encryptedVisuals}
              isLoading={isLoading}
              rows={rows}
              selectedEgressDestination={selectedEgressDestination}
              onSelectEgressDestination={setSelectedEgressDestination}
              selectedEvidenceSession={selectedEvidenceSession}
              onSelectEvidenceSession={setSelectedEvidenceSession}
              onEgressAction={openEgressAction}
              onEvidenceAction={openEvidenceAction}
              evidenceLocateFilters={evidenceLocateFilters}
            />
          </main>

          <aside className="taf-encrypted-rail">
            {activeTab === 'egress-profile' ? (
              <EgressActionRail
                visuals={encryptedVisuals}
                target={currentEgressTarget}
                onAction={openEgressAction}
                onExport={exportEgressReport}
                onNavigate={(path) => navigate(path)}
              />
            ) : activeTab === 'evidence-center' ? (
              <EvidenceCenterRail
                visuals={encryptedVisuals}
                target={currentEvidenceTarget}
                timeRange={timeRange}
                onTimeRangeChange={handleTimeRangeChange}
                onAction={openEvidenceAction}
                onLocate={(filters) => {
                  setEvidenceLocateFilters(filters);
                  const matchingSession = evidenceSessions.find((item) => matchesEvidenceLocateFilters(item, filters));
                  setSelectedEvidenceSession(matchingSession?.sessionId ?? '');
                  if (matchingSession) message.success('已按条件定位证据会话。');
                  else message.warning('当前时间窗没有匹配的证据会话。');
                }}
              />
            ) : activeTab === 'fingerprint' ? (
              <FingerprintRail visuals={encryptedVisuals} target={currentEgressTarget} onAction={openEgressAction} onNavigate={(path) => navigate(path)} />
            ) : activeTab === 'tunnel-detection' ? (
              <TunnelDetectionRail visuals={encryptedVisuals} target={currentEgressTarget} onAction={openEgressAction} onNavigate={(path) => navigate(path)} />
            ) : (
              <EncryptedContextRail
                activeTab={activeTab}
                visuals={encryptedVisuals}
                target={currentEgressTarget}
                onAction={openEgressAction}
                onNavigate={(path) => navigate(path)}
              />
            )}
          </aside>
        </div>
      </section>
      <Modal
        title={egressAction ? `${egressAction.label}确认` : '处置确认'}
        open={Boolean(egressAction)}
        confirmLoading={egressActionMutation.isPending}
        onCancel={() => setEgressAction(undefined)}
        onOk={async () => {
          if (!egressAction) return;
          try {
            await egressActionMutation.mutateAsync(egressAction);
            message.success(egressAction.successMessage);
            if (egressAction.navigateTo) navigate(egressAction.navigateTo);
            setEgressAction(undefined);
          } catch (submissionError) {
            const detail = submissionError instanceof Error ? submissionError.message : '服务端未确认处置请求。';
            message.error(`提交失败：${detail}`);
            throw submissionError;
          }
        }}
        okText="确认提交"
        cancelText="取消"
      >
        <p>{egressAction?.description}</p>
        <p className="taf-egress-action-audit">对象：{egressAction?.target}；确认后将校验 RBAC、租户边界并持久化审计记录，接口失败不会显示成功。</p>
      </Modal>
      <Modal
        title={evidenceAction ? `${evidenceAction.label}确认` : '证据动作确认'}
        open={Boolean(evidenceAction)}
        confirmLoading={evidenceActionMutation.isPending}
        onCancel={() => setEvidenceAction(undefined)}
        onOk={async () => {
          if (!evidenceAction) return;
          try {
            await evidenceActionMutation.mutateAsync(evidenceAction);
            message.success(evidenceAction.successMessage);
            setEvidenceAction(undefined);
          } catch (submissionError) {
            const detail = submissionError instanceof Error ? submissionError.message : '服务端未确认取证请求。';
            message.error(`提交失败：${detail}`);
            throw submissionError;
          }
        }}
        okText="确认提交"
        cancelText="取消"
      >
        <p>{evidenceAction?.description}</p>
        <p className="taf-egress-action-audit">对象：{evidenceAction?.target}；确认后将校验 RBAC、租户边界并持久化审计记录，接口失败不会显示成功。</p>
      </Modal>
    </div>
  );
}

function EncryptedTrafficKpis({
  activeTab,
  metrics,
  evidenceKpis,
  visuals,
}: {
  activeTab: EncryptedTrafficTabSlug;
  metrics: PageSnapshot['metrics'];
  evidenceKpis: string[][];
  visuals?: EncryptedTrafficVisuals;
}) {
  if (activeTab === 'overview') {
    return <div className="taf-encrypted-kpis">{metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}</div>;
  }
  const fingerprintRisk = visuals?.ja3Rows.filter((row) => row[5]?.includes('高')).length ?? 0;
  const weakSuites = visuals?.tlsSuiteRows.filter((row) => row[3]?.includes('risk')).length ?? 0;
  const tabRows: string[][] = activeTab === 'fingerprint'
    ? (visuals?.tabKpis.fingerprint ?? [
      ['指纹总数', String(visuals?.ja3Rows.length ?? 0), '真实 JA3 API'],
      ['可疑 JA3', String(fingerprintRisk), '风险指纹'],
      ['未知 SNI', metrics.find((item) => item.label.includes('SNI'))?.value ?? '0', '会话观测'],
      ['异常 Issuer', String(visuals?.certificateRows.length ?? 0), '证书字段'],
      ['TLS1.0/1.1', String(weakSuites), '弱版本'],
      ['弱密码套件', String(weakSuites), '协议风险'],
      ['关联规则', String(visuals?.tunnelRuleRows.length ?? 0), '检测规则'],
    ])
    : activeTab === 'tunnel-detection'
      ? (visuals?.tabKpis.tunnelDetection ?? [
        ['隧道告警', String(visuals?.tunnelRows.length ?? 0), '当前窗口'],
        ['DoH 会话', String(visuals?.tunnelRows.filter((row) => row[0]?.includes('DoH')).length ?? 0), '隧道候选'],
        ['异常长连接', String(visuals?.tunnelRows.filter((row) => row[0]?.includes('长连接')).length ?? 0), '持续时间'],
        ['高熵流量', String(visuals?.tunnelRows.filter((row) => row[0]?.includes('高熵')).length ?? 0), '载荷熵值'],
        ['低熵心跳', String(visuals?.tunnelRows.filter((row) => row[0]?.includes('心跳')).length ?? 0), '周期通信'],
        ['疑似 VPN', String(visuals?.tunnelRows.filter((row) => row[0]?.includes('VPN')).length ?? 0), '协议候选'],
        ['已创建告警', String(visuals?.tunnelRows.filter((row) => row[6]?.includes('高')).length ?? 0), '待审核'],
      ])
      : activeTab === 'egress-profile'
        ? (visuals?.egressKpis ?? []).slice(0, 7)
        : evidenceKpis;
  return (
    <div className={`taf-encrypted-kpis taf-encrypted-kpis--${activeTab === 'evidence-center' ? 'evidence' : 'tab'}`}>
      {tabRows.map(([label, value, source]) => (
        <div key={label}>
          <span className="taf-evidence-kpi-icon" aria-hidden="true">{evidenceKpiIcon(label)}</span>
          <span>{label}</span>
          <strong>{value}</strong>
          <small>{source || '真实 API'}</small>
        </div>
      ))}
    </div>
  );
}

function EncryptedTrafficTabView({
  activeTab,
  columns,
  encryptedVisuals,
  isLoading,
  rows,
  selectedEgressDestination,
  onSelectEgressDestination,
  selectedEvidenceSession,
  onSelectEvidenceSession,
  onEgressAction,
  onEvidenceAction,
  evidenceLocateFilters,
}: {
  activeTab: EncryptedTrafficTabSlug;
  columns: ColumnsType<SnapshotRow>;
  encryptedVisuals?: EncryptedTrafficVisuals;
  isLoading: boolean;
  rows: SnapshotRow[];
  selectedEgressDestination: string;
  onSelectEgressDestination: (destination: string) => void;
  selectedEvidenceSession: string;
  onSelectEvidenceSession: (sessionId: string) => void;
  onEgressAction: (label: string, target?: string, description?: string) => void;
  onEvidenceAction: (label: string, target: string) => void;
  evidenceLocateFilters: EvidenceLocateFilters;
}) {
  if (activeTab === 'fingerprint') return <FingerprintContent visuals={encryptedVisuals} onAction={onEgressAction} />;
  if (activeTab === 'tunnel-detection') return <TunnelDetectionContent visuals={encryptedVisuals} onAction={onEgressAction} />;
  if (activeTab === 'egress-profile') {
    return (
      <EgressProfileContent
        visuals={encryptedVisuals}
        selectedDestination={selectedEgressDestination}
        onSelectDestination={onSelectEgressDestination}
        onAction={onEgressAction}
      />
    );
  }
  if (activeTab === 'evidence-center') return <EvidenceCenterContent encryptedVisuals={encryptedVisuals} isLoading={isLoading} selectedSession={selectedEvidenceSession} onSelectSession={onSelectEvidenceSession} onAction={onEvidenceAction} locateFilters={evidenceLocateFilters} />;
  return <EncryptedOverviewContent columns={columns} encryptedVisuals={encryptedVisuals} isLoading={isLoading} rows={rows} />;
}

function matchesEvidenceLocateFilters(
  session: EncryptedTrafficVisuals['evidenceCenter']['sessions'][number],
  filters: EvidenceLocateFilters,
) {
  const query = filters.query.trim().toLocaleLowerCase();
  const searchable = [
    session.sessionId,
    session.source,
    session.destination,
    session.protocol,
    session.sni,
    session.ja3,
    session.certificateHash,
    session.pcapIndex,
  ].join(' ').toLocaleLowerCase();
  const matchesQuery = !query || searchable.includes(query);
  const matchesProtocol = filters.protocol === '全部' || session.protocol.includes(filters.protocol);
  const matchesRisk = filters.risk === '全部' || session.risk.includes(filters.risk);
  const matchesSniScope = filters.sniScope === '全部'
    || (filters.sniScope === '未知 SNI' && (!session.sni || session.sni === '-'))
    || (filters.sniScope === '高熵外联' && session.entropy >= 7.5);
  return matchesQuery && matchesProtocol && matchesRisk && matchesSniScope;
}

function hasEvidenceLocateFilters(filters: EvidenceLocateFilters) {
  return Boolean(filters.query.trim()) || filters.protocol !== '全部' || filters.risk !== '全部' || filters.sniScope !== '全部';
}

function EncryptedOverviewContent({
  columns,
  encryptedVisuals,
  isLoading,
  rows,
}: {
  columns: ColumnsType<SnapshotRow>;
  encryptedVisuals?: EncryptedTrafficVisuals;
  isLoading: boolean;
  rows: SnapshotRow[];
}) {
  return (
    <>
      <div className="taf-encrypted-upper">
        <WorkPanel title="协议分布与趋势">
          <ProtocolDistribution rows={encryptedVisuals?.protocolRows} trend={encryptedVisuals?.protocolTrend} />
        </WorkPanel>
        <WorkPanel title="指纹分析（Top JA3）">
          <Ja3Table rows={encryptedVisuals?.ja3Rows} />
        </WorkPanel>
        <WorkPanel title="JA3 分布（流量 vs 会话数）">
          <Ja3Scatter points={encryptedVisuals?.scatterPoints} />
        </WorkPanel>
      </div>
      <div className="taf-encrypted-middle">
        <WorkPanel title="隧道检测与异常特征">
          <TunnelFeatureCards rows={encryptedVisuals?.tunnelCards} />
        </WorkPanel>
        <WorkPanel title="异常隧道列表">
          <TunnelTable rows={encryptedVisuals?.tunnelRows} />
        </WorkPanel>
      </div>
      <OverviewEvidenceTable columns={columns} isLoading={isLoading} rows={rows} />
    </>
  );
}

function FingerprintContent({ visuals, onAction }: { visuals?: EncryptedTrafficVisuals; onAction: (label: string, target?: string, description?: string) => void }) {
  return (
    <div className="taf-fingerprint-board">
      <div className="taf-fingerprint-board-top">
        <WorkPanel title="JA3/JA3S 指纹明细（Top 20）">
          <Ja3Table rows={visuals?.ja3Rows} pageSize={7} />
        </WorkPanel>
        <WorkPanel title="指纹分布与聚类（按 JA3 聚簇）">
          <Ja3Scatter points={visuals?.scatterPoints} />
        </WorkPanel>
        <WorkPanel title="证书 Issuer 与 SNI 分布">
          <FingerprintCertificateOverview visuals={visuals} />
        </WorkPanel>
      </div>
      <div className="taf-fingerprint-board-bottom">
        <WorkPanel title="TLS 版本与密码套件（会话数热力）">
          <TlsSuiteMatrix rows={visuals?.tlsSuiteRows} />
        </WorkPanel>
        <WorkPanel title="指纹关联规则（Top 匹配）">
          <EncryptedDenseRows columns={['规则名称', '匹配指纹', '匹配类型', '置信度', '状态']} rows={visuals?.tunnelRuleRows ?? []} pageSize={5} />
        </WorkPanel>
        <WorkPanel title="证书详情预览（点击左侧行查看）">
          <FingerprintCertificatePreview visuals={visuals} onAction={onAction} />
        </WorkPanel>
      </div>
    </div>
  );
}

function TunnelDetectionContent({ visuals, onAction }: { visuals?: EncryptedTrafficVisuals; onAction: (label: string, target?: string, description?: string) => void }) {
  return (
    <div className="taf-tunnel-board">
      <div className="taf-tunnel-board-top">
        <WorkPanel title="隧道异常列表">
          <TunnelTable rows={visuals?.tunnelRows} pageSize={8} />
        </WorkPanel>
        <WorkPanel title="熵值与会话时长散点图">
          <Ja3Scatter points={visuals?.scatterPoints} />
        </WorkPanel>
        <WorkPanel title="心跳通信时间序列">
          <HeartbeatSeries bars={visuals?.heartbeatBars} summary={visuals?.heartbeatSummary} />
        </WorkPanel>
      </div>
      <div className="taf-tunnel-board-bottom">
        <WorkPanel title="DoH 与隧道特征">
          <TunnelFeatureCards rows={visuals?.tunnelCards} />
        </WorkPanel>
        <WorkPanel title="检测规则命中">
          <EncryptedDenseRows
            columns={['规则名称', '特征', '阈值', '命中数', '置信度', '处置']}
            rows={visuals?.tunnelRuleRows ?? []}
            pageSize={6}
          />
        </WorkPanel>
        <WorkPanel title="会话证据预览">
          <EncryptedDenseRows
            columns={['源 IP', '目的域名', '协议', 'JA3', 'PCAP 索引', '风险']}
            rows={visuals?.evidenceRows ?? []}
            pageSize={6}
          />
        </WorkPanel>
      </div>
      <div className="taf-encrypted-inline-actions">
        <Button size="small" onClick={() => onAction('生成调查规则', visuals?.tunnelRows[0]?.[2])}>生成调查规则</Button>
        <Button size="small" onClick={() => onAction('提交专家复核', visuals?.tunnelRows[0]?.[2])}>提交专家复核</Button>
      </div>
    </div>
  );
}

function EgressProfileContent({
  visuals,
  selectedDestination,
  onSelectDestination,
  onAction,
}: {
  visuals?: EncryptedTrafficVisuals;
  selectedDestination: string;
  onSelectDestination: (destination: string) => void;
  onAction: (label: string, target?: string, description?: string) => void;
}) {
  return (
    <div className="taf-egress-board">
      <div className="taf-egress-board-top">
        <WorkPanel
          title="外联目的地地图"
          extra={<EgressAvailability availability={visuals?.egressAvailability} />}
          className="taf-egress-map-panel"
        >
          <EgressProfile
            visuals={visuals}
            selectedDestination={selectedDestination}
            onSelectDestination={onSelectDestination}
          />
        </WorkPanel>
        <div className="taf-egress-board-stack">
          <WorkPanel title="域名画像卡" extra={<Button size="small" type="link" onClick={() => onAction('查看更多域名画像', selectedDestination || undefined)}>{'更多 >'}</Button>}>
            <DomainCards
              visuals={visuals}
              selectedDestination={selectedDestination}
              onSelectDestination={onSelectDestination}
            />
          </WorkPanel>
        </div>
      </div>
      <div className="taf-egress-board-bottom">
        <WorkPanel title="Top 外联目的地" extra={<Button size="small" type="link" onClick={() => onAction('查看更多外联目的地', selectedDestination || undefined)}>{'更多 >'}</Button>}>
          <EgressDestinationTable
            rows={visuals?.destinationRows ?? []}
            selectedDestination={selectedDestination}
            onSelectDestination={onSelectDestination}
          />
        </WorkPanel>
        <WorkPanel title="首次出现与异常域名趋势" extra={<span className="taf-egress-panel-hint">真实时间桶</span>}>
          <EgressAnomalyTrend trend={visuals?.egressTrend} availability={visuals?.egressAvailability} />
        </WorkPanel>
        <WorkPanel title="实体图谱入口" extra={<Button size="small" type="link" onClick={() => onAction('查看实体图谱', selectedDestination || undefined)}>{'更多 >'}</Button>}>
          <EgressEntityGraph
            visuals={visuals}
            selectedDestination={selectedDestination}
            onSelectDestination={onSelectDestination}
          />
        </WorkPanel>
      </div>
    </div>
  );
}

function OverviewEvidenceTable({ columns, isLoading, rows }: { columns: ColumnsType<SnapshotRow>; isLoading: boolean; rows: SnapshotRow[] }) {
  return (
    <WorkPanel title="证据与握手元数据（最新 200 条）" className="taf-encrypted-evidence-panel">
      <div className="taf-encrypted-ant-table-slot" data-paginated-table="overview-evidence">
        <Table
          className="taf-encrypted-fixed-ant-table"
          rowKey={(record) => String(record['Session 摘要'] ?? JSON.stringify(record))}
          size="small"
          loading={isLoading}
          pagination={{ pageSize: 5, showSizeChanger: false, hideOnSinglePage: false, position: ['bottomRight'], showTotal: (count) => `共 ${count} 条` }}
          columns={columns}
          dataSource={rows}
          scroll={{ x: 1280 }}
        />
      </div>
    </WorkPanel>
  );
}

function EvidenceCenterContent({
  encryptedVisuals,
  isLoading,
  selectedSession,
  onSelectSession,
  onAction,
  locateFilters,
}: {
  encryptedVisuals?: EncryptedTrafficVisuals;
  isLoading: boolean;
  selectedSession: string;
  onSelectSession: (sessionId: string) => void;
  onAction: (label: string, target: string) => void;
  locateFilters: EvidenceLocateFilters;
}) {
  const evidence = encryptedVisuals?.evidenceCenter;
  const allSessions = evidence?.sessions ?? [];
  const visibleSessions = allSessions.filter((item) => matchesEvidenceLocateFilters(item, locateFilters));
  const selectedEvidenceSession = visibleSessions.find((item) => item.sessionId === selectedSession) ?? visibleSessions[0];
  const currentTarget = selectedEvidenceSession?.sessionId || '证据中心-未选择对象';
  const usesAdaptedEvidenceDetails = selectedEvidenceSession?.sessionId === evidence?.sessions[0]?.sessionId;
  const selectedSni = selectedEvidenceSession?.sni && selectedEvidenceSession.sni !== '-' ? selectedEvidenceSession.sni : selectedEvidenceSession?.destination || '未返回';
  const selectedCertificateDetails = [
    { label: 'Subject', value: selectedSni },
    { label: 'JA3', value: selectedEvidenceSession?.ja3 || '未返回' },
    { label: 'Session ID', value: selectedEvidenceSession?.sessionId || '未返回' },
    { label: '协议', value: selectedEvidenceSession?.protocol || '未返回' },
    { label: 'ALPN', value: selectedEvidenceSession?.alpn || '未返回' },
    { label: '证书 Hash', value: selectedEvidenceSession?.certificateHash || '未返回' },
  ];
  const selectedHandshakeTimeline = [
    { time: selectedEvidenceSession?.time || '--:--:--', event: 'Session 观测', detail: `${selectedEvidenceSession?.source || '未返回'} -> ${selectedEvidenceSession?.destination || '未返回'}`, status: selectedEvidenceSession?.risk.includes('高') ? 'risk' as const : 'ok' as const },
    { time: selectedEvidenceSession?.time || '--:--:--', event: '协议识别', detail: selectedEvidenceSession?.protocol || '未返回', status: 'info' as const },
    { time: selectedEvidenceSession?.time || '--:--:--', event: 'SNI / Hash', detail: selectedEvidenceSession?.sni || '未返回', status: 'info' as const },
    { time: selectedEvidenceSession?.time || '--:--:--', event: 'JA3 指纹', detail: selectedEvidenceSession?.ja3 || '未返回', status: 'ok' as const },
    { time: selectedEvidenceSession?.time || '--:--:--', event: '证书 Hash', detail: selectedEvidenceSession?.certificateHash || '未返回', status: 'ok' as const },
    { time: selectedEvidenceSession?.time || '--:--:--', event: 'PCAP 关联', detail: selectedEvidenceSession?.pcapIndex || '未返回', status: 'info' as const },
  ];
  const certificateDetails = usesAdaptedEvidenceDetails && evidence?.certificateDetails.length ? evidence.certificateDetails : selectedCertificateDetails;
  const handshakeTimeline = usesAdaptedEvidenceDetails && evidence?.handshakeTimeline.length ? evidence.handshakeTimeline : selectedHandshakeTimeline;
  const total = evidence?.completeness.reduce((sum, item) => sum + item.total, 0) ?? 0;
  const completed = evidence?.completeness.reduce((sum, item) => sum + item.complete, 0) ?? 0;
  const completenessPercent = total ? Math.round((completed / total) * 100) : 0;
  const currentPcapTarget = evidence?.pcapRows[0]?.[0] ?? '';

  return (
    <div className="taf-evidence-center">
      <div className="taf-evidence-main-grid">
        <WorkPanel
          title="加密会话证据表"
          extra={(
            <span className="taf-evidence-panel-status">
              <EvidenceAvailability availability={evidence?.availability} />
              <span className="taf-evidence-source-hint">证据 API</span>
            </span>
          )}
        >
          <EvidenceSessionTable rows={visibleSessions} loading={isLoading} selectedSession={selectedEvidenceSession?.sessionId ?? ''} onSelect={onSelectSession} />
        </WorkPanel>
        <WorkPanel title="时间窗内 PCAP 索引（独立数据源）" extra={<span className="taf-evidence-source-hint">尚未自动关联当前会话</span>}>
          <EncryptedDenseRows columns={['对象路径', '时间窗', '大小', '包数', '来源', 'sha256', '状态']} rows={evidence?.pcapRows ?? []} />
          <div className="taf-evidence-pcap-trend">
            <PcapPacketTrendChart points={evidence?.pcapTrend ?? []} ariaLabel="PCAP 数据包字节趋势图" />
          </div>
          <div className="taf-evidence-pcap-actions">
            <Button size="small" disabled={!currentPcapTarget} onClick={() => onAction('下载 PCAP', currentPcapTarget)}>申请下载</Button>
            <Button size="small" disabled={!currentPcapTarget} onClick={() => onAction('校验证据 Hash', currentPcapTarget)}>申请校验</Button>
            <Button size="small" type="primary" disabled={!currentPcapTarget} onClick={() => onAction('生成取证任务', currentPcapTarget)}>生成取证任务</Button>
          </div>
        </WorkPanel>
        <EvidenceAnchorPanel
          session={selectedEvidenceSession}
          availability={evidence?.availability}
          entropyTrend={evidence?.entropyTrend ?? []}
          onAction={onAction}
        />
      </div>
      <div className="taf-evidence-bottom-grid">
        <WorkPanel title="证书详情与握手元数据">
          <div className="taf-evidence-certificate-grid">
            <div className="taf-evidence-definition-list">
              {certificateDetails.map((item) => <div key={item.label}><span>{item.label}</span><strong>{item.value}</strong></div>)}
            </div>
            <div className="taf-evidence-handshake-timeline">
              {handshakeTimeline.map((item) => <div key={`${item.time}-${item.event}`} className={`is-${item.status}`}><time>{item.time}</time><strong>{item.event}</strong><span>{item.detail}</span></div>)}
            </div>
          </div>
        </WorkPanel>
        <WorkPanel title="证据完整度">
          <div className="taf-evidence-completeness">
            <div className="taf-evidence-ring"><EvidenceClosureRingChart item={{ label: '证据完整度', value: completenessPercent, level: completenessPercent >= 90 ? 'low' : completenessPercent >= 60 ? 'medium' : 'high' }} ariaLabel="证据完整度环图" /></div>
            <div className="taf-evidence-completeness-list">
              {(evidence?.completeness ?? []).map((item) => {
                const percent = item.total ? Math.round((item.complete / item.total) * 100) : 0;
                return <div key={item.label}><span>{item.label}</span><i><b className={`is-${item.status}`} style={{ width: `${percent}%` }} /></i><em>{item.complete}/{item.total}</em></div>;
              })}
            </div>
          </div>
        </WorkPanel>
        <WorkPanel title="Hash 与审计校验" extra={<Button size="small" onClick={() => onAction('校验证据 Hash', currentTarget)}>申请校验</Button>}>
          <EncryptedDenseRows columns={['Hash 摘要', '对象路径', '时间', '来源', '状态']} rows={evidence?.hashRows ?? []} />
        </WorkPanel>
      </div>
    </div>
  );
}

function EvidenceAnchorPanel({
  session,
  availability,
  entropyTrend,
  onAction,
}: {
  session?: EncryptedTrafficVisuals['evidenceCenter']['sessions'][number];
  availability?: EncryptedTrafficVisuals['evidenceCenter']['availability'];
  entropyTrend: EncryptedTrafficVisuals['evidenceCenter']['entropyTrend'];
  onAction: (label: string, target: string) => void;
}) {
  const target = session?.sessionId || '证据中心-未选择对象';
  const hasTarget = Boolean(session?.sessionId);
  const facts = [
    ['Session ID', target],
    ['时间', session?.time || '未返回'],
    ['源 / 目的', `${session?.source || '未返回'} -> ${session?.destination || '未返回'}`],
    ['协议', `${session?.protocol || '未返回'} / ${session?.alpn || '-'}`],
    ['SNI / 域名', session?.sni || '未返回'],
    ['JA3', session?.ja3 || '未返回'],
    ['证书 Hash', session?.certificateHash || '未返回'],
    ['PCAP 索引', session?.pcapIndex || '未返回'],
  ];
  const sourceHint = entropyTrend.length ? 'Payload entropy API' : 'Payload entropy 未返回';
  return (
    <WorkPanel title="证据锚点概览" extra={<StatusTag value={session?.risk || '未知'} />}>
      <div className="taf-evidence-anchor">
        <div className="taf-evidence-anchor__facts">
          {facts.map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong></div>)}
        </div>
        <div className="taf-evidence-anchor__entropy">
          <span>Payload Entropy</span>
          <em>{sourceHint}</em>
          <div><EvidenceEntropyTrendChart points={entropyTrend} ariaLabel="加密证据会话熵分趋势图" /></div>
        </div>
        <div className="taf-evidence-anchor__actions">
          <Button size="small" disabled={!hasTarget || !session?.certificateHash || session.certificateHash === '-'} onClick={() => onAction('校验证据 Hash', target)}>校验证据 Hash</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('关联证据分析', target)}>关联证据分析</Button>
        </div>
      </div>
    </WorkPanel>
  );
}

function EvidenceAvailability({ availability }: { availability?: EncryptedTrafficVisuals['evidenceCenter']['availability'] }) {
  const state = availability?.state ?? 'unavailable';
  const label = state === 'live'
    ? '证据 API 已接入'
    : state === 'partial'
      ? '真实证据部分返回'
      : '证据数据未返回';
  return <span className={`taf-evidence-availability is-${state}`} title={availability?.detail}>{label}</span>;
}

function EvidenceSessionTable({
  rows,
  loading,
  selectedSession,
  onSelect,
}: {
  rows: EncryptedTrafficVisuals['evidenceCenter']['sessions'];
  loading: boolean;
  selectedSession: string;
  onSelect: (sessionId: string) => void;
}) {
  const columns: ColumnsType<EncryptedTrafficVisuals['evidenceCenter']['sessions'][number]> = [
    { title: '时间', dataIndex: 'time', width: 92, ellipsis: true },
    { title: 'Session ID', dataIndex: 'sessionId', width: 132, ellipsis: true },
    { title: '源 IP', dataIndex: 'source', width: 96, ellipsis: true },
    { title: '目的地址', dataIndex: 'destination', width: 120, ellipsis: true },
    { title: '协议', dataIndex: 'protocol', width: 76, ellipsis: true },
    { title: '风险', dataIndex: 'risk', width: 70, render: (value) => <StatusTag value={String(value)} /> },
  ];
  return (
    <div className="taf-encrypted-ant-table-slot" data-paginated-table="evidence-sessions">
      <Table
        className="taf-encrypted-fixed-ant-table taf-evidence-session-table"
        rowKey="sessionId"
        size="small"
        loading={loading}
        columns={columns}
        dataSource={rows}
        pagination={{ pageSize: 6, showSizeChanger: false, hideOnSinglePage: false, position: ['bottomRight'], showTotal: (count) => `共 ${count} 条` }}
        rowClassName={(record) => record.sessionId === selectedSession ? 'taf-evidence-selected-row' : ''}
        onRow={(record) => ({ onClick: () => onSelect(record.sessionId) })}
      />
    </div>
  );
}

function ProtocolDistribution({ rows = [], trend }: { rows?: string[][]; trend?: number[] }) {
  const items = rows.map(([label, , ratio, tone]) => ({
    label,
    value: Number.parseFloat(ratio) || 0,
    color: tone === 'is-risk' ? '#ff4d4f' : tone === 'is-warn' ? '#8d66ff' : '#18a8ff',
  }));

  return (
    <div className="taf-encrypted-protocol taf-echarts-protocol">
      <EncryptedProtocolTrendChart items={items} trend={trend ?? []} ariaLabel="加密流量协议占比与趋势图" />
    </div>
  );
}

function Ja3Table({ rows = [], pageSize = 5 }: { rows?: string[][]; pageSize?: number }) {
  const pagination = useFixedTablePagination(rows, pageSize);
  if (!rows.length) return <div className="taf-encrypted-ja3-table" data-paginated-table="ja3"><div className="taf-encrypted-table-empty"><EncryptedEmptyState description="JA3 API 暂无指纹数据" /></div><FixedTablePagination {...pagination} /></div>;
  return (
    <div className="taf-encrypted-ja3-table" data-paginated-table="ja3">
      <div>
        <span>JA3 指纹</span>
        <span>流量占比</span>
        <span>流量 (Gbps)</span>
        <span>SNI 数</span>
        <span>关联告警</span>
        <span>风险等级</span>
      </div>
      {pagination.rows.map(([hash, ratio, flow, sni, alerts, risk]) => (
        <button key={hash} type="button" disabled title="请使用顶部“指纹详情”查看完整证据">
          <strong>{hash}</strong>
          <span>{ratio}</span>
          <span>{flow}</span>
          <span>{sni}</span>
          <span>{alerts}</span>
          <StatusTag value={risk} />
        </button>
      ))}
      <FixedTablePagination {...pagination} />
    </div>
  );
}

function Ja3Scatter({ points = [] }: { points?: EncryptedTrafficVisuals['scatterPoints'] }) {
  if (!points.length) return <EncryptedEmptyState description="暂无包含流量与会话计数的指纹数据" />;
  return (
    <div className="taf-encrypted-scatter taf-echarts-scatter">
      <EncryptedJa3ScatterChart
        points={points.map((point) => ({ x: point.left, y: 100 - point.top, level: point.tone === 'risk' ? 'high' : point.tone === 'warn' ? 'medium' : point.tone === 'ok' ? 'low' : 'info' }))}
        ariaLabel="JA3 流量与会话散点图"
      />
    </div>
  );
}

function TunnelFeatureCards({ rows = [] }: { rows?: string[][] }) {
  if (!rows.length) return <EncryptedEmptyState description="隧道分析 API 暂无聚合数据" />;
  return (
    <div className="taf-encrypted-tunnel-cards">
      {rows.map(([label, value, delta, tone]) => (
        <div key={label} className={`is-${tone}`}>
          <span>{label}</span>
          <strong>{value}</strong>
          <em>{delta}</em>
        </div>
      ))}
    </div>
  );
}

function TunnelTable({ rows = [], pageSize = 5 }: { rows?: string[][]; pageSize?: number }) {
  const pagination = useFixedTablePagination(rows, pageSize);
  if (!rows.length) return <div className="taf-encrypted-tunnel-table" data-paginated-table="tunnel-candidates"><div className="taf-encrypted-table-empty"><EncryptedEmptyState description="隧道候选 API 暂无待研判记录" /></div><FixedTablePagination {...pagination} /></div>;
  return (
    <div className="taf-encrypted-tunnel-table" data-paginated-table="tunnel-candidates">
      <div>
        <span>类型</span>
        <span>疑征</span>
        <span>源 IP</span>
        <span>目的域名 / IP</span>
        <span>持续时间</span>
        <span>流量 (Gbps)</span>
        <span>风险等级</span>
      </div>
      {pagination.rows.map(([type, feature, src, dst, duration, flow, risk]) => (
        <button key={`${type}-${src}-${dst}`} type="button" disabled title="待后端返回候选会话标识后可下钻">
          <strong>{type}</strong>
          <span>{feature}</span>
          <span>{src}</span>
          <span>{dst}</span>
          <span>{duration}</span>
          <span>{flow}</span>
          <StatusTag value={risk} />
        </button>
      ))}
      <FixedTablePagination {...pagination} />
    </div>
  );
}

function EncryptedDenseRows({ columns, rows, pageSize = 5 }: { columns: string[]; rows: string[][]; pageSize?: number }) {
  const pagination = useFixedTablePagination(rows, pageSize);
  return (
    <div className="taf-encrypted-dense-rows" data-paginated-table="dense" style={{ '--encrypted-columns': columns.length } as CSSProperties}>
      <div className="taf-encrypted-dense-head">
        {columns.map((column) => <span key={column}>{column}</span>)}
      </div>
      {!rows.length ? <div className="taf-encrypted-table-empty"><EncryptedEmptyState description="当前接口未返回可展示记录" /></div> : pagination.rows.map((row) => (
        <div key={row.join('-')} className="taf-encrypted-dense-row">
          {row.map((cell, index) => <span key={`${cell}-${index}`}>{cell}</span>)}
        </div>
      ))}
      <FixedTablePagination {...pagination} />
    </div>
  );
}

function TlsSuiteMatrix({ rows = [] }: { rows?: string[][] }) {
  if (!rows.length) return <EncryptedEmptyState description="会话 API 暂无 TLS 版本或密码套件字段" />;
  return (
    <div className="taf-encrypted-suite-matrix">
      {rows.map(([version, suite, ratio, tone]) => (
        <div key={`${version}-${suite}`} className={`is-${tone}`}>
          <strong>{version}</strong>
          <span>{suite}</span>
          <em>{ratio}</em>
        </div>
      ))}
    </div>
  );
}

function FingerprintCertificateOverview({ visuals }: { visuals?: EncryptedTrafficVisuals }) {
  const certificates = visuals?.certificateRows ?? [];
  const known = certificates.filter((row) => row[0] && row[0] !== '-').length;
  const unknown = Math.max(0, certificates.length - known);
  const weak = visuals?.tlsSuiteRows.filter((row) => row[3]?.includes('risk')).length ?? 0;
  const items = [
    ['Issuer 覆盖', `${known}/${certificates.length || 0}`, 'is-info'],
    ['未知 Issuer', String(unknown), 'is-warn'],
    ['弱协议', String(weak), 'is-risk'],
    ['证书样本', String(certificates.length), 'is-ok'],
  ];
  return (
    <div className="taf-fingerprint-cert-overview">
      {items.map(([label, value, tone]) => <div key={label} className={tone}><span>{label}</span><strong>{value}</strong></div>)}
      <EncryptedDenseRows columns={['Issuer', '目的 IP', '风险']} rows={certificates.map((row) => [row[0] || '-', row[1] || '-', row[5] || '正常'])} pageSize={3} />
    </div>
  );
}

function FingerprintCertificatePreview({ visuals, onAction }: { visuals?: EncryptedTrafficVisuals; onAction: (label: string, target?: string, description?: string) => void }) {
  const certificate = visuals?.certificateRows[0] ?? [];
  const target = certificate[1] || visuals?.ja3Rows[0]?.[0] || '未选择证书';
  return (
    <div className="taf-fingerprint-cert-preview">
      <div className="taf-fingerprint-cert-preview__title"><SafetyCertificateOutlined /><strong>{target}</strong><StatusTag value={certificate[5] || '未知'} /></div>
      <div className="taf-fingerprint-cert-preview__facts">
        <span>Issuer</span><strong>{certificate[0] || '未返回'}</strong>
        <span>TLS / ALPN</span><strong>{`${certificate[2] || '-'} / ${certificate[3] || '-'}`}</strong>
        <span>关联告警</span><strong>{certificate[4] || '0'}</strong>
        <span>证书状态</span><strong>{certificate[5] || '未知'}</strong>
      </div>
      <div className="taf-fingerprint-cert-preview__actions">
        <Button size="small" onClick={() => onAction('查看完整证书', target)}>查看完整证书</Button>
        <Button size="small" type="primary" onClick={() => onAction('创建证据', target)}>创建证据</Button>
      </div>
    </div>
  );
}

function HeartbeatSeries({ bars, summary }: { bars?: number[]; summary?: EncryptedTrafficVisuals['heartbeatSummary'] }) {
  const heartbeatBars = bars ?? [];
  if (!heartbeatBars.length) return <EncryptedEmptyState description="会话 API 暂无可计算的持续时间数据" />;
  const sorted = [...heartbeatBars].sort((left, right) => left - right);
  const p95 = sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))] ?? 0;
  const average = heartbeatBars.reduce((sum, value) => sum + value, 0) / heartbeatBars.length;

  return (
    <div className="taf-encrypted-heartbeat">
      <div className="taf-encrypted-heartbeat-kpis">
        {[
          ['P95 间隔', summary ? `${summary.intervalP95Seconds.toFixed(1)}s` : `${p95.toFixed(1)} min`],
          ['抖动 (P95)', summary ? `${summary.jitterP95Seconds.toFixed(2)}s` : `${average.toFixed(1)} min`],
          ['包数', summary ? summary.packetCount.toLocaleString('zh-CN') : String(heartbeatBars.length)],
        ].map(([label, value]) => (
          <span key={label}>
            <b>{label}</b>
            <strong>{value}</strong>
          </span>
        ))}
      </div>
      <div className="taf-encrypted-heartbeat-bars">
        <HeartbeatTrendChart values={heartbeatBars} ariaLabel="心跳通信时间序列图" />
      </div>
    </div>
  );
}

function EncryptedEmptyState({ description }: { description: string }) {
  return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description} />;
}

function useFixedTablePagination<T>(sourceRows: T[], pageSize = 5) {
  const [page, setPage] = useState(1);
  const pageCount = Math.max(1, Math.ceil(sourceRows.length / pageSize));
  useEffect(() => setPage((current) => Math.min(current, pageCount)), [pageCount]);
  const offset = (page - 1) * pageSize;
  return {
    rows: sourceRows.slice(offset, offset + pageSize),
    page,
    pageSize,
    total: sourceRows.length,
    onChange: setPage,
  };
}

function FixedTablePagination({
  page,
  pageSize,
  total,
  onChange,
}: {
  rows: unknown[];
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number) => void;
}) {
  return (
    <div className="taf-encrypted-table-pagination">
      <Pagination
        size="small"
        current={page}
        pageSize={pageSize}
        total={total}
        showSizeChanger={false}
        hideOnSinglePage={false}
        onChange={onChange}
      />
    </div>
  );
}

function DomainCards({
  visuals,
  selectedDestination,
  onSelectDestination,
}: {
  visuals?: EncryptedTrafficVisuals;
  selectedDestination: string;
  onSelectDestination?: (destination: string) => void;
}) {
  const cards = visuals?.egressDomainCards ?? [];
  if (!cards.length) {
    return <EgressEmptyState message="外传分析接口未返回域名或目的地址画像。" />;
  }
  return (
    <div className="taf-encrypted-domain-cards">
      {cards.map(([domain, desc, sessions, risk], index) => {
        const destination = visuals?.egressMapNodes[index]?.label ?? domain;
        return (
          <button
            key={domain}
            type="button"
            className={destination === selectedDestination ? 'is-selected' : ''}
            onClick={() => onSelectDestination?.(destination)}
          >
            <strong>{domain}</strong>
            <span>{desc}</span>
            <em>{sessions}</em>
            <StatusTag value={risk} />
          </button>
        );
      })}
    </div>
  );
}

function EgressProfile({
  visuals,
  selectedDestination,
  onSelectDestination,
}: {
  visuals?: EncryptedTrafficVisuals;
  selectedDestination?: string;
  onSelectDestination?: (destination: string) => void;
}) {
  const nodes = visuals?.egressMapNodes ?? [];
  const mapPoints = [
    { name: '园区出口', coord: [745, 225] as [number, number], value: 1, level: 'low' as const },
    ...nodes.map((node) => ({
      name: node.label,
      coord: [Math.round(node.x * 10), Math.round(node.y * 5)] as [number, number],
      value: Math.max(1, Number.parseFloat(node.flow) || Number.parseFloat(node.sessions) || 1),
      level: egressWorldLevel(node.risk),
      selected: node.label === selectedDestination,
    })),
  ];
  const mapFlows = nodes.map((node) => ({
    name: `园区出口 -> ${node.label}`,
    from: [745, 225] as [number, number],
    to: [Math.round(node.x * 10), Math.round(node.y * 5)] as [number, number],
    value: Math.max(1, Number.parseFloat(node.flow) || Number.parseFloat(node.sessions) || 1),
    level: egressWorldLevel(node.risk),
    selected: node.label === selectedDestination,
  }));

  return (
    <div className="taf-encrypted-egress">
      <div className="taf-encrypted-map">
        <WorldActivityMap
          variant="egress"
          points={mapPoints}
          flows={mapFlows}
          ariaLabel={`外联目的地全球流向图，当前选中 ${selectedDestination || '未选择对象'}`}
          onNodeClick={(name) => onSelectDestination?.(name)}
        />
        {!nodes.length && <EgressEmptyState message="暂无可定位的外联目的地。" />}
        <div className="taf-egress-map-legend">
          <span className="is-risk">高风险目的地</span>
          <span className="is-warn">待确认目的地</span>
          <span className="is-info">已观测目的地</span>
        </div>
      </div>
    </div>
  );
}

function egressWorldLevel(risk: string): 'low' | 'medium' | 'high' {
  if (risk.includes('高') || risk.includes('严重')) return 'high';
  if (risk.includes('中') || risk.includes('待确认')) return 'medium';
  return 'low';
}

function EgressAvailability({ availability }: { availability?: EncryptedTrafficVisuals['egressAvailability'] }) {
  const state = availability?.state ?? 'unavailable';
  const label = state === 'live'
    ? '外传 API 已接入'
    : state === 'partial'
      ? '会话补全中'
      : '外传数据未返回';
  return <span className={`taf-egress-availability is-${state}`} title={availability?.detail}>{label}</span>;
}

function EgressEmptyState({ message }: { message: string }) {
  return <div className="taf-egress-empty-state"><GlobalOutlined /><span>{message}</span></div>;
}

function EgressDestinationTable({
  rows,
  selectedDestination,
  onSelectDestination,
}: {
  rows: string[][];
  selectedDestination: string;
  onSelectDestination: (destination: string) => void;
}) {
  const pagination = useFixedTablePagination(rows);
  if (!rows.length) return <div className="taf-encrypted-destinations" data-paginated-table="egress-destinations"><div className="taf-encrypted-table-empty"><EgressEmptyState message="外传分析接口未返回目的地排行。" /></div><FixedTablePagination {...pagination} /></div>;
  return (
    <div className="taf-encrypted-destinations" data-paginated-table="egress-destinations">
      <div>
        <span>目的 IP / 域名</span>
        <span>位置 / ASN</span>
        <span>流量</span>
        <span>会话数</span>
        <span>风险</span>
      </div>
      {pagination.rows.map(([ip, location, flow, sessions, risk]) => (
        <button key={`${ip}-${location}`} type="button" className={ip === selectedDestination ? 'is-selected' : ''} onClick={() => onSelectDestination(ip)}>
          <strong>{ip}</strong>
          <span>{location}</span>
          <span>{flow}</span>
          <span>{sessions}</span>
          <StatusTag value={risk} />
        </button>
      ))}
      <FixedTablePagination {...pagination} />
    </div>
  );
}

function EgressAnomalyTrend({
  trend,
  availability,
}: {
  trend?: EncryptedTrafficVisuals['egressTrend'];
  availability?: EncryptedTrafficVisuals['egressAvailability'];
}) {
  if (!trend?.labels.length || !trend.series.length) return <EgressEmptyState message={availability?.detail ?? '外传分析接口未返回趋势时间桶。'} />;
  return (
    <div className="taf-egress-anomaly-trend" title="当前流量/会话样本由外传 API 或已标识的仿真数据生成。">
      <ExfilStackedTrendChart
        labels={trend.labels}
        series={trend.series}
        ariaLabel="外联会话风险趋势图"
      />
    </div>
  );
}

function EgressEntityGraph({
  visuals,
  selectedDestination,
  onSelectDestination,
}: {
  visuals?: EncryptedTrafficVisuals;
  selectedDestination: string;
  onSelectDestination: (destination: string) => void;
}) {
  const nodes = visuals?.egressMapNodes.slice(0, 4) ?? [];
  if (!nodes.length) return <EgressEmptyState message="暂无可关联的外联实体。" />;
  const graphNodes = [
    { name: '园区出口', x: 18, y: 52, value: 6, level: 'low' as const },
    ...nodes.map((node, index) => ({
      name: node.label,
      x: 45 + (index % 2) * 18,
      y: 22 + Math.floor(index / 2) * 58,
      value: Math.max(2, Number.parseFloat(node.sessions) / 3000 || 2),
      level: egressWorldLevel(node.risk),
      selected: node.label === selectedDestination,
    })),
    { name: '证据/处置', x: 87, y: 52, value: 5, level: 'high' as const },
  ];
  const links = nodes.flatMap((node) => [
    { source: '园区出口', target: node.label, value: Math.max(1, Number.parseFloat(node.flow) || 1), selected: node.label === selectedDestination },
    { source: node.label, target: '证据/处置', value: node.risk.includes('高') ? 2 : 1, selected: node.label === selectedDestination },
  ]);
  return (
    <div className="taf-egress-entity-chart">
      <ExfilGraphChart
        nodes={graphNodes}
        links={links}
        ariaLabel={`外联实体关联图，当前选中 ${selectedDestination || '未选择对象'}`}
        onNodeClick={(name) => {
          if (nodes.some((node) => node.label === name)) onSelectDestination(name);
        }}
      />
    </div>
  );
}

function AdviceList({ rows = [] as string[][], onAction }: { rows?: string[][]; onAction?: (label: string, target?: string) => void }) {
  if (!rows.length) return <EgressEmptyState message="暂无可执行的处置建议。" />;
  return (
    <div className="taf-encrypted-advice">
      {rows.map(([text, action], index) => (
        <div key={text}>
          <span>{text}</span>
          <Button size="small" type={index === 0 ? 'primary' : 'default'} onClick={() => onAction?.(action)}>{action}</Button>
        </div>
      ))}
    </div>
  );
}

function EgressActionRail({
  visuals,
  target,
  onAction,
  onExport,
  onNavigate,
}: {
  visuals?: EncryptedTrafficVisuals;
  target: string;
  onAction: (label: string, target?: string, description?: string) => void;
  onExport: () => void;
  onNavigate: (path: string) => void;
}) {
  const highRisk = visuals?.egressRiskScore ?? visuals?.destinationRows.filter((row) => row[4]?.includes('高')).length ?? 0;
  return (
    <div className="taf-egress-action-rail">
      <WorkPanel title="外联风险">
        <div className="taf-egress-risk-gauge">
          <strong>{highRisk}</strong>
          <span>{visuals?.egressRiskScore ? '高风险' : '高风险目的地'}</span>
          <em>{visuals?.egressRiskDelta || (visuals?.egressAvailability.state === 'live' ? '外传 API 已接入' : '数据状态待补全')}</em>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <div className="taf-egress-quick-actions">
          <Button size="small" danger onClick={() => onAction('创建外联告警', target)}>创建外联告警</Button>
          <Button size="small" onClick={() => onAction('查看目的地证据', target)}>查看目的地证据</Button>
          <Button size="small" onClick={() => onAction('跳转实体图谱', target)}>跳转实体图谱</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="处置建议">
        <AdviceList rows={visuals?.adviceRows} onAction={(label) => onAction(label, target)} />
      </WorkPanel>
      <WorkPanel title="关联与导出">
        <div className="taf-egress-rail-actions">
          <Button size="small" onClick={() => onNavigate(`/alerts?destination=${encodeURIComponent(target)}`)}>关联告警</Button>
          <Button size="small" onClick={() => onNavigate(`/graph?focus=${encodeURIComponent(target)}`)}>实体图谱</Button>
          <Button size="small" icon={<DownloadOutlined />} onClick={onExport}>导出画像</Button>
          <Button size="small" onClick={() => onAction('写入审计日志', target)}>写入审计</Button>
        </div>
      </WorkPanel>
    </div>
  );
}

function EvidenceCenterRail({
  visuals,
  target,
  timeRange,
  onTimeRangeChange,
  onAction,
  onLocate,
}: {
  visuals?: EncryptedTrafficVisuals;
  target: string;
  timeRange: EncryptedTrafficTimeRange;
  onTimeRangeChange: (value: EncryptedTrafficTimeRange) => void;
  onAction: (label: string, target: string) => void;
  onLocate: (filters: EvidenceLocateFilters) => void;
}) {
  const evidence = visuals?.evidenceCenter;
  const total = evidence?.completeness.reduce((sum, item) => sum + item.total, 0) ?? 0;
  const complete = evidence?.completeness.reduce((sum, item) => sum + item.complete, 0) ?? 0;
  const completion = total ? Math.round((complete / total) * 100) : 0;
  const [query, setQuery] = useState(target);
  const [protocol, setProtocol] = useState('全部');
  const [risk, setRisk] = useState('全部');
  const [sniScope, setSniScope] = useState('全部');
  const hasTarget = Boolean(target && !target.includes('未选择对象'));

  useEffect(() => setQuery(target), [target]);

  return (
    <div className="taf-evidence-action-rail">
      <WorkPanel title="证据概览">
        <div className="taf-evidence-rail-summary">
          <div className="taf-evidence-rail-summary__ring">
            <EvidenceClosureRingChart item={{ label: '证据完整度', value: completion, level: completion >= 90 ? 'low' : completion >= 60 ? 'medium' : 'high' }} ariaLabel="证据概览完整度环图" />
          </div>
          <div className="taf-evidence-rail-summary__details">
            <strong>{completion}%</strong>
            <span>证据完整度</span>
            <em>{evidence?.availability.state === 'live' ? '真实证据 API 已接入' : evidence?.availability.state === 'partial' ? '真实证据部分返回' : '证据数据未返回'}</em>
          </div>
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <div className="taf-evidence-locate-form">
          <label><span>检索 Session ID / 域名 / IP</span><Input size="small" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
          <label><span>时间范围</span><Select size="small" value={timeRange} onChange={onTimeRangeChange} options={[{ value: '近 1 小时' }, { value: '近 24 小时' }, { value: '近 7 天' }]} /></label>
          <label><span>协议类型</span><Select size="small" value={protocol} onChange={setProtocol} options={[{ value: '全部' }, { value: 'TLS' }, { value: 'QUIC' }]} /></label>
          <label><span>风险等级</span><Select size="small" value={risk} onChange={setRisk} options={[{ value: '全部' }, { value: '高危' }, { value: '中危' }]} /></label>
          <label><span>SNI / 域名</span><Select size="small" value={sniScope} onChange={setSniScope} options={[{ value: '全部' }, { value: '未知 SNI' }, { value: '高熵外联' }]} /></label>
          <Button size="small" type="primary" onClick={() => onLocate({ query: query.trim(), protocol, risk, sniScope })}>定位</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="证据动作">
        <div className="taf-evidence-quick-actions">
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('生成取证任务', target)}>生成取证任务</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('申请证据保全', target)}>申请证据保全</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('关联告警', target)}>关联告警</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('发起专家复核', target)}>发起专家复核</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('标记证据缺口', target)}>标记证据缺口</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('提交处置建议', target)}>提交处置建议</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="报告与审计">
        <div className="taf-evidence-quick-actions">
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('导出证据报告', target)}>导出证据报告</Button>
          <Button size="small" disabled={!hasTarget} onClick={() => onAction('写入审计日志', target)}>写入审计日志</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="证据缺口 Top5">
        <div className="taf-evidence-gap-list">
          {(evidence?.completeness ?? []).map((item) => <div key={item.label}><span>{item.label}</span><strong>{Math.max(0, item.total - item.complete)}</strong></div>)}
        </div>
      </WorkPanel>
    </div>
  );
}

function FingerprintRail({
  visuals,
  target,
  onAction,
  onNavigate,
}: {
  visuals?: EncryptedTrafficVisuals;
  target: string;
  onAction: (label: string, target?: string, description?: string) => void;
  onNavigate: (path: string) => void;
}) {
  const fingerprintKpis = new Map((visuals?.tabKpis.fingerprint ?? []).map((row) => [row[0], row[1]]));
  const suspicious = fingerprintKpis.get('可疑 JA3') ?? visuals?.ja3Rows.filter((row) => row[5]?.includes('高')).length ?? 0;
  const anomalyRows = [
    ['可疑 JA3', suspicious],
    ['未知 SNI', fingerprintKpis.get('未知 SNI') ?? 0],
    ['异常 Issuer', fingerprintKpis.get('异常 Issuer') ?? 0],
    ['TLS1.0/1.1 会话', fingerprintKpis.get('TLS1.0/1.1') ?? 0],
    ['弱密码套件', fingerprintKpis.get('弱密码套件') ?? 0],
  ];
  return (
    <div className="taf-fingerprint-action-rail">
      <WorkPanel title="指纹异常（近 24 小时）" extra={<Button size="small" type="link" onClick={() => onNavigate('/encrypted-traffic?tab=fingerprint')}>{'查看全部异常 >'}</Button>}>
        <div className="taf-encrypted-rail-stat-list">{anomalyRows.map(([label, value]) => <div key={String(label)}><span>{label}</span><strong>{value}</strong></div>)}</div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <div className="taf-encrypted-rail-actions">
          <Button size="small" onClick={() => onAction('创建 JA3 规则', visuals?.ja3Rows[0]?.[0] || target)}>创建 JA3 规则</Button>
          <Button size="small" onClick={() => onNavigate(`/encrypted-traffic?tab=evidence-center&destination=${encodeURIComponent(target)}`)}>查看证书证据</Button>
          <Button size="small" onClick={() => onAction('导出指纹报告', target)}>导出指纹报告</Button>
          <Button size="small" onClick={() => onAction('加入观察名单', target)}>加入观察名单</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="修复建议">
        <AdviceList rows={visuals?.adviceRows} onAction={(label) => onAction(label, target)} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <div className="taf-encrypted-rail-actions">
          <Button size="small" onClick={() => onAction('指纹分析报告', target)}>指纹分析报告</Button>
          <Button size="small" onClick={() => onNavigate(`/forensics?query=${encodeURIComponent(target)}`)}>PCAP 证据检索</Button>
          <Button size="small" onClick={() => onAction('写入审计日志', target)}>审计与证据导出</Button>
        </div>
      </WorkPanel>
    </div>
  );
}

function TunnelDetectionRail({
  visuals,
  target,
  onAction,
  onNavigate,
}: {
  visuals?: EncryptedTrafficVisuals;
  target: string;
  onAction: (label: string, target?: string, description?: string) => void;
  onNavigate: (path: string) => void;
}) {
  const distribution = visuals?.tunnelRiskDistribution ?? [];
  const highRisk = distribution.find((item) => item.status === 'risk')?.value ?? visuals?.tunnelRows.filter((row) => row[6]?.includes('高')).length ?? 0;
  return (
    <div className="taf-tunnel-action-rail">
      <WorkPanel title="隧道异常">
        <div className="taf-egress-risk-gauge"><strong>{highRisk}</strong><span>高风险候选</span><em>{visuals?.tabKpis.tunnelDetection?.[0]?.[1] ?? visuals?.tunnelRows.length ?? 0} 条待研判</em></div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <div className="taf-encrypted-rail-actions">
          <Button size="small" danger onClick={() => onAction('创建隧道告警', target)}>创建隧道告警</Button>
          <Button size="small" onClick={() => onNavigate(`/encrypted-traffic?tab=evidence-center&destination=${encodeURIComponent(target)}`)}>查看会话证据</Button>
          <Button size="small" onClick={() => onAction('加入观察名单', target)}>加入观察名单</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="修复建议">
        <AdviceList rows={visuals?.adviceRows} onAction={(label) => onAction(label, target)} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <div className="taf-encrypted-rail-actions">
          <Button size="small" onClick={() => onAction('导出隧道检测报告', target)}>导出隧道检测报告</Button>
          <Button size="small" onClick={() => onNavigate(`/forensics?query=${encodeURIComponent(target)}`)}>跳转证据中心</Button>
        </div>
      </WorkPanel>
    </div>
  );
}

function EncryptedContextRail({
  activeTab,
  visuals,
  target,
  onAction,
  onNavigate,
}: {
  activeTab: EncryptedTrafficTabSlug;
  visuals?: EncryptedTrafficVisuals;
  target: string;
  onAction: (label: string, target?: string, description?: string) => void;
  onNavigate: (path: string) => void;
}) {
  const fingerprintTarget = visuals?.ja3Rows[0]?.[0] || target;
  const tunnelTarget = visuals?.tunnelRows[0]?.[2] || target;
  const contextTarget = activeTab === 'fingerprint' ? fingerprintTarget : activeTab === 'tunnel-detection' ? tunnelTarget : target;
  if (activeTab === 'overview') {
    return <OverviewEgressRail visuals={visuals} target={contextTarget} onAction={onAction} onNavigate={onNavigate} />;
  }
  const title = activeTab === 'fingerprint' ? '指纹分析摘要' : activeTab === 'tunnel-detection' ? '隧道候选摘要' : '外联画像';
  const summary = activeTab === 'fingerprint'
    ? `${visuals?.ja3Rows.length ?? 0} 个当前返回指纹，点击指纹详情查看字段证据。`
    : activeTab === 'tunnel-detection'
      ? `${visuals?.tunnelRows.length ?? 0} 个待研判候选；候选不等同于已确认告警。`
      : `${visuals?.destinationRows.length ?? 0} 个公网外联候选目的地。`;
  return (
    <div className={`taf-encrypted-context-rail taf-encrypted-context-rail--${activeTab}`}>
      <WorkPanel title={title} extra={<Button size="small" type="link" onClick={() => onNavigate(`/encrypted-traffic?tab=${activeTab}`)}>查看详情</Button>}>
        <div className="taf-encrypted-context-summary"><strong>{contextTarget || '未选择对象'}</strong><span>{summary}</span></div>
      </WorkPanel>
      <WorkPanel title="处置与分析建议">
        <AdviceList rows={visuals?.adviceRows} onAction={(label) => onAction(label, contextTarget)} />
      </WorkPanel>
      <WorkPanel title="关联与下钻">
        <LinkActions target={contextTarget} onNavigate={onNavigate} />
      </WorkPanel>
      <WorkPanel title="生成与导出">
        <ExportActions target={contextTarget} onAction={onAction} onNavigate={onNavigate} />
      </WorkPanel>
    </div>
  );
}

function OverviewEgressRail({
  visuals,
  target,
  onAction,
  onNavigate,
}: {
  visuals?: EncryptedTrafficVisuals;
  target: string;
  onAction: (label: string, target?: string, description?: string) => void;
  onNavigate: (path: string) => void;
}) {
  const nodes = visuals?.egressMapNodes ?? [];
  const mapPoints = [
    { name: '园区出口', coord: [745, 225] as [number, number], value: 1, level: 'low' as const },
    ...nodes.map((node) => ({
      name: node.label,
      coord: [Math.round(node.x * 10), Math.round(node.y * 5)] as [number, number],
      value: Math.max(1, Number.parseFloat(node.flow) || 1),
      level: egressWorldLevel(node.risk),
    })),
  ];
  const mapFlows = nodes.map((node) => ({
    name: `园区出口 -> ${node.label}`,
    from: [745, 225] as [number, number],
    to: [Math.round(node.x * 10), Math.round(node.y * 5)] as [number, number],
    value: Math.max(1, Number.parseFloat(node.flow) || 1),
    level: egressWorldLevel(node.risk),
  }));
  return (
    <div className="taf-encrypted-context-rail taf-encrypted-overview-rail">
      <WorkPanel title="外联画像" extra={<Button size="small" type="link" onClick={() => onNavigate('/encrypted-traffic?tab=egress-profile')}>{'查看详情 >'}</Button>}>
        <div className="taf-encrypted-overview-rail__kpis">
          {(visuals?.egressKpis ?? []).slice(0, 4).map(([label, value, delta]) => <div key={label}><span>{label}</span><strong>{value}</strong><em>{delta}</em></div>)}
        </div>
        <div className="taf-encrypted-overview-rail__map">
          <WorldActivityMap variant="egress" points={mapPoints} flows={mapFlows} ariaLabel="总览外联目的地地图" />
        </div>
        <EgressDestinationTable rows={(visuals?.destinationRows ?? []).slice(0, 7)} selectedDestination="" onSelectDestination={() => {}} />
      </WorkPanel>
      <WorkPanel title="处置与分析建议"><AdviceList rows={visuals?.adviceRows} onAction={(label) => onAction(label, target)} /></WorkPanel>
      <WorkPanel title="关联与下钻"><LinkActions target={target} onNavigate={onNavigate} /></WorkPanel>
      <WorkPanel title="生成与导出"><ExportActions target={target} onAction={onAction} onNavigate={onNavigate} /></WorkPanel>
    </div>
  );
}

function LinkActions({ target, onNavigate }: { target: string; onNavigate: (path: string) => void }) {
  const actions: Array<[string, ReactNode, string]> = [
    ['关联告警（18）', <ApiOutlined />, `/alerts?encrypted=${encodeURIComponent(target)}`],
    ['关联战役（2）', <RadarChartOutlined />, `/campaigns?encrypted=${encodeURIComponent(target)}`],
    ['攻击链分析', <ThunderboltOutlined />, `/attack-chains?encrypted=${encodeURIComponent(target)}`],
    ['实体图谱', <GlobalOutlined />, `/graph?focus=${encodeURIComponent(target)}`],
    ['取证分析', <FileSearchOutlined />, `/forensics?encrypted=${encodeURIComponent(target)}`],
    ['PCAP 检索', <EyeOutlined />, `/forensics?query=${encodeURIComponent(target)}`],
  ];
  return (
    <div className="taf-encrypted-action-grid">
      {actions.map(([label, icon, path]) => <Button key={String(label)} size="small" icon={icon} onClick={() => onNavigate(String(path))}>{label}</Button>)}
    </div>
  );
}

function ExportActions({ target, onAction, onNavigate }: { target: string; onAction: (label: string, target?: string, description?: string) => void; onNavigate: (path: string) => void }) {
  const actions: Array<[string, ReactNode, () => void]> = [
    ['创建告警', <LockOutlined />, () => onAction('创建外联告警', target)],
    ['创建战役', <RadarChartOutlined />, () => onNavigate(`/campaigns?createFrom=${encodeURIComponent(target)}`)],
    ['生成报告', <FileSearchOutlined />, () => onAction('生成分析报告', target)],
    ['导出 PCAP 索引', <DownloadOutlined />, () => onNavigate(`/forensics?query=${encodeURIComponent(target)}`)],
    ['导出证书', <SafetyCertificateOutlined />, () => onNavigate(`/encrypted-traffic?tab=evidence-center&destination=${encodeURIComponent(target)}`)],
    ['写入审计日志', <CalendarOutlined />, () => onAction('写入审计日志', target)],
  ];
  return (
    <div className="taf-encrypted-action-grid">
      {actions.map(([label, icon, handler]) => <Button key={String(label)} size="small" icon={icon} onClick={handler}>{label}</Button>)}
    </div>
  );
}

function renderEncryptedCell(column: string, value: unknown) {
  if (column === '风险等级') return <StatusTag value={value} />;
  if (column === '操作') return <Button size="small" type="link" disabled title="请在证据中心选择会话后下钻">待关联</Button>;
  if (column === '协议') return <span className="taf-encrypted-protocol-cell"><CloudServerOutlined />{String(value)}</span>;
  if (column === 'JA3' || column === 'JA3S') return <span className="taf-encrypted-hash"><KeyOutlined />{String(value)}</span>;
  if (column === '证书详情') return <StatusTag value={value} />;
  return String(value ?? '-');
}

function evidenceKpiIcon(label: string) {
  if (label.includes('Session')) return <CloudServerOutlined />;
  if (label.includes('PCAP')) return <FileSearchOutlined />;
  if (label.includes('证书')) return <SafetyCertificateOutlined />;
  if (label.includes('握手')) return <ApiOutlined />;
  if (label.includes('Hash')) return <KeyOutlined />;
  if (label.includes('任务')) return <CalendarOutlined />;
  return <LockOutlined />;
}

function fallbackMetric(label: string): PageSnapshot['metrics'][number] {
  const value = label.includes('占比') || label.includes('比例') ? '0.0%' : '0';
  return { label, value, delta: '等待 API', status: 'info' };
}
