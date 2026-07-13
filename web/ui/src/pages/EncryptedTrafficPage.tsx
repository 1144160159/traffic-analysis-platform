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
import { Alert, Button, Input, Modal, Select, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties } from 'react';
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

const protocolRows = [
  ['TLS', '49.8 Gbps', '63.7%', 'is-info'],
  ['QUIC', '14.4 Gbps', '18.4%', 'is-warn'],
  ['未知加密', '14.1 Gbps', '17.9%', 'is-risk'],
];

const ja3Rows = [
  ['771,4865-4866-4867-4919...', '12.8%', '10.0', '312', '18', '高危'],
  ['cbd52c1eb6700091a3e2d4c...', '8.7%', '6.8', '198', '9', '中危'],
  ['e7d70S342S8a2c0d0e0a71d1...', '7.6%', '6.0', '145', '12', '中危'],
  ['4d7a28f00056bS87f06a4e3c2...', '6.2%', '4.9', '112', '6', '低危'],
  ['598c8ab3943e1e61065c2e8e...', '4.9%', '3.8', '98', '7', '中危'],
  ['f5a3d7c0eb2fe4e3c6fb101e9...', '3.8%', '3.0', '76', '3', '低危'],
];

const tunnelCards = [
  ['DNS over HTTPS 会话', '412', '+36', 'warn'],
  ['异常长连接（> 1h）', '287', '+21', 'risk'],
  ['高熵流量（> 7.5）', '531', '+58', 'risk'],
  ['低频流量（< 3.0）', '76', '+9', 'info'],
  ['低流量心跳（疑似）', '193', '+14', 'warn'],
  ['疑似 VPN 会话', '118', '+13', 'warn'],
];

const tunnelRows = [
  ['DNS over HTTPS', 'DoH over TLS', '10.12.2.36', 'cloudflare-dns.com', '3h 24m', '2.18', '高危'],
  ['异常长连接', '长连接 > 1h', '172.16.5.10', '203.0.113.45', '6h 12m', '3.47', '高危'],
  ['心跳隧道', '固定周期 60s', '10.10.8.45', '198.51.100.27', '2h 53m', '0.96', '中危'],
  ['高熵流量', '熵值 > 7.8', '10.10.9.33', '198.16.12.34', '1h 41m', '9.66', '中危'],
  ['低频黑噪', '频率 < 1/min', '10.11.3.22', '185.22.14.9', '4h 30m', '0.42', '低危'],
  ['疑似 VPN', 'TLS over 443', '10.12.6.77', '37.120.196.12', '5h 18m', '1.02', '中危'],
];

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

const certificateRows = [
  ['DigiCert Global Root', '203.0.113.45', 'TLS 1.3', 'h2', '18', '高危'],
  ['Cloudflare Inc ECC', '104.16.12.34', 'TLS 1.3', 'h3', '9', '中危'],
  ['Unknown Issuer', '185.22.14.9', 'TLS 1.2', 'http/1.1', '12', '中危'],
  ['Let’s Encrypt R3', '198.51.100.27', 'TLS 1.3', 'h2', '3', '低危'],
];

const tunnelRuleRows = [
  ['DoH over HTTPS', 'SNI=DNS, ALPN=h2/h3', '会话 > 3', '287', '95%', '创建告警'],
  ['DoH over QUIC', 'ALPN=h3, 端口=443/853', '会话 > 2', '121', '93%', '创建告警'],
  ['异常长连接', '持续时间 > 2h', '> 2h', '531', '90%', '创建告警'],
  ['低熵心跳通信', '熵值 < 3.5 且周期稳定', '周期 +-20%', '76', '91%', '调整规则'],
  ['高熵可疑流量', '熵值 > 7.0 & 流量 >100MB', '>100MB', '193', '88%', '调整规则'],
  ['可疑 VPN / Proxy', '特征端口/协议指纹', '会话 > 2', '118', '89%', '创建告警'],
];

const evidenceRows = [
  ['10.10.10.23:52344', 'cloudflare-dns.com', 'TLS 1.3', '771,4865...', 'pcap-20250626-000512', '高危'],
  ['172.16.5.18:44710', 'unknown-sni', 'TLS 1.2', 'cbd52c1e...', 'pcap-20250626-000624', '高危'],
  ['10.10.40.12:55320', 'dns.google', 'QUIC', 'e7d70S34...', 'pcap-20250626-000701', '中危'],
  ['10.12.6.77:40112', 'vpn.example.net', 'UDP', '4d7a28f0...', 'pcap-20250626-000744', '中危'],
];

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

const scatterPoints: EncryptedTrafficVisuals['scatterPoints'] = Array.from({ length: 34 }, (_, index) => ({
  left: 7 + ((index * 11) % 86),
  top: 12 + ((index * 17) % 70),
  tone: index % 7 === 0 ? 'risk' : index % 5 === 0 ? 'warn' : index % 3 === 0 ? 'info' : 'ok',
}));

export function EncryptedTrafficPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = resolveEncryptedTrafficTab(searchParams.get('tab'));
  const [selectedEgressDestination, setSelectedEgressDestination] = useState('');
  const [selectedEvidenceSession, setSelectedEvidenceSession] = useState('');
  const [egressAction, setEgressAction] = useState<EgressAction>();
  const [evidenceAction, setEvidenceAction] = useState<EvidenceAction>();
  const [timeRange, setTimeRange] = useState<EncryptedTrafficTimeRange>('近 24 小时');
  const [evidenceLocateFilters, setEvidenceLocateFilters] = useState<EvidenceLocateFilters>(initialEvidenceLocateFilters);
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
    <div className={`taf-page taf-encrypted taf-encrypted--${activeTab}`}>
      <section className="taf-encrypted-shell">
        <header className="taf-encrypted-titlebar">
          <h1>{route.page.title}</h1>
          <div className="taf-encrypted-tabs">
            {encryptedTrafficTabs.map((tab) => (
              <button
                key={tab.slug}
                type="button"
                className={tab.slug === activeTab ? 'is-active' : ''}
                aria-selected={tab.slug === activeTab}
                role="tab"
                onClick={() => setSearchParams({ tab: tab.slug })}
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

        {activeTab === 'evidence-center' ? (
          <div className="taf-encrypted-kpis taf-encrypted-kpis--evidence">
            {evidenceKpis.map(([label, value, source]) => (
              <div key={label}>
                <span className="taf-evidence-kpi-icon" aria-hidden="true">{evidenceKpiIcon(label)}</span>
                <span>{label}</span>
                <strong>{value}</strong>
                <small>{source}</small>
              </div>
            ))}
          </div>
        ) : (
          <div className="taf-encrypted-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>
        )}

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
                  message.success('已按条件定位证据会话。');
                }}
              />
            ) : (
              <>
                <WorkPanel title="外联画像" extra={<Button size="small" type="link">查看详情</Button>}>
                  <EgressProfile visuals={encryptedVisuals} />
                </WorkPanel>
                <WorkPanel title="处置与分析建议">
                  <AdviceList rows={encryptedVisuals?.adviceRows} />
                </WorkPanel>
                <WorkPanel title="关联与下钻">
                  <LinkActions />
                </WorkPanel>
                <WorkPanel title="生成与导出">
                  <ExportActions />
                </WorkPanel>
              </>
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
  if (activeTab === 'fingerprint') return <FingerprintContent visuals={encryptedVisuals} />;
  if (activeTab === 'tunnel-detection') return <TunnelDetectionContent visuals={encryptedVisuals} />;
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

function FingerprintContent({ visuals }: { visuals?: EncryptedTrafficVisuals }) {
  return (
    <>
      <div className="taf-encrypted-fingerprint-layout">
        <WorkPanel title="JA3 / JA3S 指纹排行">
          <Ja3Table rows={visuals?.ja3Rows} />
        </WorkPanel>
        <WorkPanel title="JA3 分布（流量 vs 会话数）">
          <Ja3Scatter points={visuals?.scatterPoints} />
        </WorkPanel>
      </div>
      <div className="taf-encrypted-tab-grid">
        <WorkPanel title="证书 Issuer 异常">
          <EncryptedDenseRows
            columns={['Issuer', '目的 IP', 'TLS', 'ALPN', '告警', '风险']}
            rows={visuals?.certificateRows ?? certificateRows}
          />
        </WorkPanel>
        <WorkPanel title="TLS 版本与密码套件">
          <TlsSuiteMatrix />
        </WorkPanel>
        <WorkPanel title="指纹处置建议">
          <AdviceList rows={visuals?.adviceRows} />
        </WorkPanel>
      </div>
    </>
  );
}

function TunnelDetectionContent({ visuals }: { visuals?: EncryptedTrafficVisuals }) {
  return (
    <>
      <div className="taf-encrypted-middle">
        <WorkPanel title="隧道告警与异常特征">
          <TunnelFeatureCards rows={visuals?.tunnelCards} />
        </WorkPanel>
        <WorkPanel title="隧道异常列表">
          <TunnelTable rows={visuals?.tunnelRows} />
        </WorkPanel>
      </div>
      <div className="taf-encrypted-tunnel-layout">
        <WorkPanel title="熵值与会话时长散点图">
          <Ja3Scatter points={visuals?.scatterPoints} />
        </WorkPanel>
        <WorkPanel title="心跳通信时间序列">
          <HeartbeatSeries bars={visuals?.heartbeatBars} />
        </WorkPanel>
      </div>
      <div className="taf-encrypted-tab-grid">
        <WorkPanel title="DoH 与隧道特征">
          <TunnelFeatureCards rows={visuals?.tunnelCards} />
        </WorkPanel>
        <WorkPanel title="检测规则命中">
          <EncryptedDenseRows
            columns={['规则名称', '特征', '阈值', '命中数', '置信度', '处置']}
            rows={visuals?.tunnelRuleRows ?? tunnelRuleRows}
          />
        </WorkPanel>
        <WorkPanel title="会话证据预览">
          <EncryptedDenseRows
            columns={['源 IP', '目的域名', '协议', 'JA3', 'PCAP 索引', '风险']}
            rows={(visuals?.evidenceRows ?? evidenceRows).slice(0, 3)}
          />
        </WorkPanel>
      </div>
    </>
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
          <WorkPanel title="域名画像卡" extra={<span className="taf-egress-panel-hint">实时对象</span>}>
            <DomainCards
              visuals={visuals}
              selectedDestination={selectedDestination}
              onSelectDestination={onSelectDestination}
            />
          </WorkPanel>
          <WorkPanel title="外联会话风险趋势" extra={<span className="taf-egress-panel-hint">真实时间桶</span>}>
            <EgressAnomalyTrend trend={visuals?.egressTrend} availability={visuals?.egressAvailability} />
          </WorkPanel>
        </div>
      </div>
      <div className="taf-egress-board-bottom">
        <WorkPanel title="Top 外联目的地" extra={<span className="taf-egress-panel-hint">点击行以定位地图</span>}>
          <EgressDestinationTable
            rows={visuals?.destinationRows ?? []}
            selectedDestination={selectedDestination}
            onSelectDestination={onSelectDestination}
          />
        </WorkPanel>
        <WorkPanel title="实体图谱入口" extra={<Button size="small" type="link" onClick={() => onAction('查看实体图谱', selectedDestination || undefined)}>查看详情</Button>}>
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
      <Table
        rowKey={(record) => String(record['Session 摘要'] ?? JSON.stringify(record))}
        size="small"
        loading={isLoading}
        pagination={false}
        columns={columns}
        dataSource={rows.slice(0, 5)}
        scroll={{ x: 1280 }}
      />
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
  const [evidenceTab, setEvidenceTab] = useState<'tls' | 'quic' | 'session'>('tls');
  const evidence = encryptedVisuals?.evidenceCenter;
  const allSessions = evidence?.sessions ?? [];
  const sessions = allSessions.filter((item) => (
    evidenceTab === 'tls' ? item.protocol.includes('TLS') : evidenceTab === 'quic' ? item.protocol.includes('QUIC') : true
  ));
  const activeSessions = sessions.length ? sessions : allSessions;
  const visibleSessions = activeSessions.filter((item) => matchesEvidenceLocateFilters(item, locateFilters));
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

  return (
    <div className="taf-evidence-center">
      <div className="taf-evidence-main-grid">
        <WorkPanel
          title="加密会话证据表"
          extra={(
            <span className="taf-evidence-panel-status">
              <EvidenceAvailability availability={evidence?.availability} />
              <span className="taf-evidence-source-hint">{evidence?.availability.state === 'simulated' ? '仿真证据样本' : '证据 API'}</span>
            </span>
          )}
        >
          <div className="taf-encrypted-evidence-tabs" role="tablist" aria-label="证据会话类型">
            {[
              ['tls', 'TLS 握手'],
              ['quic', 'QUIC 握手'],
              ['session', 'Session 摘要'],
            ].map(([value, label]) => (
              <button key={value} type="button" role="tab" aria-selected={evidenceTab === value} className={evidenceTab === value ? 'is-active' : ''} onClick={() => { setEvidenceTab(value as 'tls' | 'quic' | 'session'); onSelectSession(''); }}>{label}</button>
            ))}
          </div>
          <EvidenceSessionTable rows={visibleSessions} loading={isLoading} selectedSession={selectedEvidenceSession?.sessionId ?? ''} onSelect={onSelectSession} />
        </WorkPanel>
        <WorkPanel title="PCAP 索引与切片" extra={<span className="taf-evidence-source-hint">{evidence?.availability.state === 'simulated' ? '仿真波形' : 'PCAP 索引 API'}</span>}>
          <EncryptedDenseRows columns={['对象路径', '时间窗', '大小', '包数', '来源', 'sha256', '状态']} rows={evidence?.pcapRows ?? []} />
          <div className="taf-evidence-pcap-trend">
            <PcapPacketTrendChart points={evidence?.pcapTrend ?? []} ariaLabel="PCAP 数据包字节趋势图" />
          </div>
          <div className="taf-evidence-pcap-actions">
            <Button size="small" onClick={() => onAction('下载 PCAP', currentTarget)}>下载</Button>
            <Button size="small" onClick={() => onAction('校验证据 Hash', currentTarget)}>校验</Button>
            <Button size="small" type="primary" onClick={() => onAction('生成取证任务', currentTarget)}>生成取证任务</Button>
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
  const sourceHint = availability?.state === 'simulated'
    ? '仿真熵分'
    : entropyTrend.length
      ? 'Payload entropy API'
      : 'Payload entropy 未返回';
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
          <Button size="small" onClick={() => onAction('校验证据 Hash', target)}>校验证据 Hash</Button>
          <Button size="small" onClick={() => onAction('关联证据分析', target)}>关联证据分析</Button>
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
      : state === 'simulated'
        ? '仿真数据（API 空）'
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
    { title: '时间', dataIndex: 'time', width: 126, ellipsis: true },
    { title: 'Session ID', dataIndex: 'sessionId', width: 120, ellipsis: true },
    { title: '源 IP', dataIndex: 'source', width: 104, ellipsis: true },
    { title: '目的地址', dataIndex: 'destination', width: 120, ellipsis: true },
    { title: '协议', dataIndex: 'protocol', width: 72, ellipsis: true },
    { title: 'SNI/Hash', dataIndex: 'sni', width: 138, ellipsis: true },
    { title: 'JA3', dataIndex: 'ja3', width: 114, ellipsis: true },
    { title: 'ALPN', dataIndex: 'alpn', width: 60, ellipsis: true },
    { title: 'PCAP 索引', dataIndex: 'pcapIndex', width: 126, ellipsis: true },
    { title: '风险', dataIndex: 'risk', width: 66, render: (value) => <StatusTag value={String(value)} /> },
  ];
  return (
    <Table
      rowKey="sessionId"
      size="small"
      loading={loading}
      columns={columns}
      dataSource={rows}
      pagination={false}
      scroll={{ x: 1_050, y: 300 }}
      rowClassName={(record) => record.sessionId === selectedSession ? 'taf-evidence-selected-row' : ''}
      onRow={(record) => ({ onClick: () => onSelect(record.sessionId) })}
    />
  );
}

function ProtocolDistribution({ rows = protocolRows, trend }: { rows?: string[][]; trend?: number[] }) {
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

function Ja3Table({ rows = ja3Rows }: { rows?: string[][] }) {
  return (
    <div className="taf-encrypted-ja3-table">
      <div>
        <span>JA3 指纹</span>
        <span>流量占比</span>
        <span>流量 (Gbps)</span>
        <span>SNI 数</span>
        <span>关联告警</span>
        <span>风险等级</span>
      </div>
      {rows.map(([hash, ratio, flow, sni, alerts, risk]) => (
        <button key={hash} type="button">
          <strong>{hash}</strong>
          <span>{ratio}</span>
          <span>{flow}</span>
          <span>{sni}</span>
          <span>{alerts}</span>
          <StatusTag value={risk} />
        </button>
      ))}
    </div>
  );
}

function Ja3Scatter({ points = scatterPoints }: { points?: EncryptedTrafficVisuals['scatterPoints'] }) {
  return (
    <div className="taf-encrypted-scatter taf-echarts-scatter">
      <EncryptedJa3ScatterChart
        points={points.map((point) => ({ x: point.left, y: 100 - point.top, level: point.tone === 'risk' ? 'high' : point.tone === 'warn' ? 'medium' : point.tone === 'ok' ? 'low' : 'info' }))}
        ariaLabel="JA3 流量与会话散点图"
      />
    </div>
  );
}

function TunnelFeatureCards({ rows = tunnelCards }: { rows?: string[][] }) {
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

function TunnelTable({ rows = tunnelRows }: { rows?: string[][] }) {
  return (
    <div className="taf-encrypted-tunnel-table">
      <div>
        <span>类型</span>
        <span>疑征</span>
        <span>源 IP</span>
        <span>目的域名 / IP</span>
        <span>持续时间</span>
        <span>流量 (Gbps)</span>
        <span>风险等级</span>
      </div>
      {rows.map(([type, feature, src, dst, duration, flow, risk]) => (
        <button key={`${type}-${src}-${dst}`} type="button">
          <strong>{type}</strong>
          <span>{feature}</span>
          <span>{src}</span>
          <span>{dst}</span>
          <span>{duration}</span>
          <span>{flow}</span>
          <StatusTag value={risk} />
        </button>
      ))}
    </div>
  );
}

function EncryptedDenseRows({ columns, rows }: { columns: string[]; rows: string[][] }) {
  return (
    <div className="taf-encrypted-dense-rows" style={{ '--encrypted-columns': columns.length } as CSSProperties}>
      <div className="taf-encrypted-dense-head">
        {columns.map((column) => <span key={column}>{column}</span>)}
      </div>
      {rows.map((row) => (
        <div key={row.join('-')} className="taf-encrypted-dense-row">
          {row.map((cell, index) => <span key={`${cell}-${index}`}>{cell}</span>)}
        </div>
      ))}
    </div>
  );
}

function TlsSuiteMatrix() {
  const items = [
    ['TLS 1.3', '4865 / AES128-GCM', '48.2%', 'ok'],
    ['TLS 1.3', '4867 / CHACHA20', '21.6%', 'ok'],
    ['TLS 1.2', '49195 / ECDHE', '18.4%', 'warn'],
    ['TLS 1.2', 'RSA legacy', '7.2%', 'risk'],
    ['QUIC', 'h3 / 443', '14.9%', 'warn'],
    ['Unknown', 'unknown-sni', '6.8%', 'risk'],
  ];
  return (
    <div className="taf-encrypted-suite-matrix">
      {items.map(([version, suite, ratio, tone]) => (
        <div key={`${version}-${suite}`} className={`is-${tone}`}>
          <strong>{version}</strong>
          <span>{suite}</span>
          <em>{ratio}</em>
        </div>
      ))}
    </div>
  );
}

function HeartbeatSeries({ bars }: { bars?: number[] }) {
  const fallbackBars = Array.from({ length: 48 }, (_, index) => 18 + ((index * 19) % 70));
  const heartbeatBars = bars && bars.length >= 24 ? bars : fallbackBars;

  return (
    <div className="taf-encrypted-heartbeat">
      <div className="taf-encrypted-heartbeat-kpis">
        {[
          ['P95 间隔', '60.4s'],
          ['抖动 P95', '0.82s'],
          ['包数', '2,486'],
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
  const kpis = visuals?.egressKpis ?? [];
  const nodes = visuals?.egressMapNodes ?? [];
  const mapPoints = [
    { name: '园区出口', coord: [500, 265] as [number, number], value: 1, level: 'low' as const },
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
    from: [500, 265] as [number, number],
    to: [Math.round(node.x * 10), Math.round(node.y * 5)] as [number, number],
    value: Math.max(1, Number.parseFloat(node.flow) || Number.parseFloat(node.sessions) || 1),
    level: egressWorldLevel(node.risk),
    selected: node.label === selectedDestination,
  }));

  return (
    <div className="taf-encrypted-egress">
      <div className="taf-encrypted-egress-kpis">
        {kpis.map(([label, value, delta]) => (
          <div key={label}>
            <span>{label}</span>
            <strong>{value}</strong>
            <em>{delta}</em>
          </div>
        ))}
      </div>
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
      : state === 'simulated'
        ? '仿真数据（API 空）'
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
  if (!rows.length) return <EgressEmptyState message="外传分析接口未返回目的地排行。" />;
  return (
    <div className="taf-encrypted-destinations">
      <div>
        <span>目的 IP / 域名</span>
        <span>位置 / ASN</span>
        <span>流量</span>
        <span>会话数</span>
        <span>风险</span>
      </div>
      {rows.map(([ip, location, flow, sessions, risk]) => (
        <button key={`${ip}-${location}`} type="button" className={ip === selectedDestination ? 'is-selected' : ''} onClick={() => onSelectDestination(ip)}>
          <strong>{ip}</strong>
          <span>{location}</span>
          <span>{flow}</span>
          <span>{sessions}</span>
          <StatusTag value={risk} />
        </button>
      ))}
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
  const highRisk = visuals?.destinationRows.filter((row) => row[4]?.includes('高')).length ?? 0;
  return (
    <div className="taf-egress-action-rail">
      <WorkPanel title="外联风险">
        <div className="taf-egress-risk-gauge">
          <strong>{highRisk}</strong>
          <span>高风险目的地</span>
          <em>{visuals?.egressAvailability.state === 'live' ? '外传 API 已接入' : '数据状态待补全'}</em>
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
            <em>{evidence?.availability.state === 'live' ? '真实证据 API 已接入' : evidence?.availability.state === 'simulated' ? '仿真证据样本' : '证据数据补全中'}</em>
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
          <Button size="small" onClick={() => onAction('生成取证任务', target)}>生成取证任务</Button>
          <Button size="small" onClick={() => onAction('申请证据保全', target)}>申请证据保全</Button>
          <Button size="small" onClick={() => onAction('关联告警', target)}>关联告警</Button>
          <Button size="small" onClick={() => onAction('发起专家复核', target)}>发起专家复核</Button>
          <Button size="small" onClick={() => onAction('标记证据缺口', target)}>标记证据缺口</Button>
          <Button size="small" onClick={() => onAction('提交处置建议', target)}>提交处置建议</Button>
        </div>
      </WorkPanel>
      <WorkPanel title="报告与审计">
        <div className="taf-evidence-quick-actions">
          <Button size="small" onClick={() => onAction('导出证据报告', target)}>导出证据报告</Button>
          <Button size="small" onClick={() => onAction('写入审计日志', target)}>写入审计日志</Button>
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

function LinkActions() {
  const actions = [
    ['关联告警 (18)', <ApiOutlined />],
    ['关联战役 (2)', <RadarChartOutlined />],
    ['攻击链分析', <ThunderboltOutlined />],
    ['实体图谱', <GlobalOutlined />],
    ['取证分析', <FileSearchOutlined />],
    ['PCAP 检索', <EyeOutlined />],
  ];
  return (
    <div className="taf-encrypted-action-grid">
      {actions.map(([label, icon]) => <Button key={String(label)} size="small" icon={icon}>{label}</Button>)}
    </div>
  );
}

function ExportActions() {
  const actions = [
    ['创建告警', <LockOutlined />],
    ['创建战役', <RadarChartOutlined />],
    ['生成报告', <FileSearchOutlined />],
    ['导出 PCAP 索引', <DownloadOutlined />],
    ['导出证书', <SafetyCertificateOutlined />],
    ['写入审计日志', <CalendarOutlined />],
  ];
  return (
    <div className="taf-encrypted-action-grid">
      {actions.map(([label, icon]) => <Button key={String(label)} size="small" icon={icon}>{label}</Button>)}
    </div>
  );
}

function renderEncryptedCell(column: string, value: unknown) {
  if (column === '风险等级') return <StatusTag value={value} />;
  if (column === '操作') return <Button size="small" type="link">下钻</Button>;
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
