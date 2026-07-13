import {
  AlertOutlined,
  ApiOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  DownloadOutlined,
  EditOutlined,
  ExportOutlined,
  FileDoneOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  GlobalOutlined,
  KeyOutlined,
  LockOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SearchOutlined,
  ShareAltOutlined,
  StarOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  DataQualityDonutChart,
  DataQualityTrendChart,
  ExfilBarChart,
  ExfilLineChart,
  ExfilPieChart,
  ExfilSankeyChart,
  TopicTopologyGraph,
  type ExfilBarItem,
  type ExfilDistributionItem,
  type ExfilSankeyLink,
  type ExfilSankeyNode,
  type ExfilTrendPoint,
  type TopicTopologyLink,
  type TopicTopologyNode,
} from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { findRouteById } from '@/routes/routeManifest';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type TopicId = 'topic-tunnel' | 'topic-exfil' | 'topic-apt';
type Tone = PageSnapshot['metrics'][number]['status'];
type SnapshotMetric = PageSnapshot['metrics'][number];

type ExfilTableRow = {
  region: string;
  asn: string;
  traffic: string;
  ratio: string;
};

type ExfilVisualModel = {
  sankeyNodes: ExfilSankeyNode[];
  sankeyLinks: ExfilSankeyLink[];
  destinationRows: ExfilTableRow[];
  sensitiveTypes: ExfilDistributionItem[];
  protocols: ExfilDistributionItem[];
  trend: ExfilTrendPoint[];
  accounts: ExfilBarItem[];
  confidence: number;
  totalUploadGb: number;
  pathCount: number;
};

type AptCampaignNode = {
  name: string;
  meta: string;
  events: number;
  tone: Tone;
};

type AptPhaseNode = {
  id: string;
  label: string;
  value: number;
  confidence: string;
  tone: Tone;
};

type AptEvidenceNode = {
  label: string;
  value: string;
  tone: Tone;
};

type AptTimelinePoint = {
  label: string;
  aptCn: number;
  tempHawk: number;
  unknown: number;
};

type AptIocRow = {
  ioc: string;
  type: string;
  hits: number;
  firstSeen: string;
};

type AptEvidenceEventRow = {
  id: string;
  phase: string;
  assetGroup: string;
  ioc: string;
  evidenceType: string;
  timeWindow: string;
  status: string;
  statusTone: Tone;
  actions: string[];
};

type AptVisualModel = {
  campaigns: AptCampaignNode[];
  phases: AptPhaseNode[];
  evidenceNodes: AptEvidenceNode[];
  assets: AptEvidenceNode[];
  timeline: AptTimelinePoint[];
  iocs: AptIocRow[];
  response: Array<{ label: string; value: number; tone: Tone }>;
  evidenceRows: PageSnapshot['evidence'];
  reportConfidence: number;
  closureRate: number;
  eventTotal: number;
};

type TopicConfig = {
  tone: 'tunnel' | 'exfil' | 'apt';
  topicCode: string;
  site: string;
  assetGroup: string;
  ipRange: string;
  protocol: string;
  timeRange: string;
  rule: string;
  model: string;
  canvasTitle: string;
  canvasMode: string;
  reportTitle: string;
  reportSubject: string;
  eventTotal: number;
  api: string;
  icon: ReactNode;
  focusModes: string[];
  signalTitle: string;
  signals: Array<{ label: string; value: string; detail: string; status: Tone; icon: ReactNode }>;
  laneTitle: string;
  lanes: Array<{ phase: string; target: string; evidence: string; status: Tone; icon: ReactNode }>;
  actionRows: Array<{ label: string; detail: string; status: Tone; icon: ReactNode }>;
  drillLinks: Array<{ label: string; to: string; icon: ReactNode }>;
  score: number;
};

type TopicActionButtonProps = {
  topic: string;
  title: string;
  target?: string;
  className?: string;
  ariaLabel?: string;
  children: ReactNode;
};

function TopicActionButton({ topic, title, target = title, className, ariaLabel, children }: TopicActionButtonProps) {
  const [open, setOpen] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  return (
    <>
      <button
        type="button"
        className={className}
        title={title}
        aria-label={ariaLabel}
        onClick={() => {
          setSubmitted(false);
          setOpen(true);
        }}
      >
        {children}
      </button>
      <Drawer
        className="taf-topic-action-drawer"
        title={`${title}确认`}
        open={open}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onClose={() => {
          setOpen(false);
          setSubmitted(false);
        }}
        extra={<Button size="small" type="primary" disabled={submitted} onClick={() => setSubmitted(true)}>{submitted ? '已写入任务队列' : '确认提交'}</Button>}
      >
        <div className="taf-alert-detail-action-body">
          <p>将为专题“{topic}”创建“{title}”仿真任务，并保留当前对象、操作者和专题审计上下文。</p>
          <dl>
            <dt>专题对象</dt><dd>{topic}</dd>
            <dt>操作目标</dt><dd>{target}</dd>
            <dt>接口预留</dt><dd>/v1/topics/{topic}/actions</dd>
          </dl>
          {submitted && <Alert type="success" showIcon message="专题业务操作已进入仿真任务队列" description={`目标：${target}；动作：${title}`} />}
        </div>
      </Drawer>
    </>
  );
}

const topicConfigs: Record<TopicId, TopicConfig> = {
  'topic-tunnel': {
    tone: 'tunnel',
    topicCode: 'tunnel-20260620-01',
    site: '主校区',
    assetGroup: '办公终端 / 服务群组',
    ipRange: '10.12.0.0/16',
    protocol: 'SSH / TLS / HTTPS / RDP / SOCKS',
    timeRange: '近 7 天 (2026-06-13 00:00:00 ~ 2026-06-20 03:45:00)',
    rule: '加密隧道识别规则集 v2.1',
    model: '加密隧道识别模型 v1.3',
    canvasTitle: '加密隧道局部影响面',
    canvasMode: '布局：径向',
    reportTitle: '加密隧道专题汇总周报',
    reportSubject: '办公终端 / 服务群组',
    eventTotal: 128,
    api: '/v1/topics/tunnel',
    icon: <LockOutlined />,
    focusModes: ['高危会话', '未知 SNI', 'DoH', '长连接'],
    signalTitle: '隧道信号雷达',
    signals: [
      { label: '协议族识别', value: 'TLS / QUIC / VPN', detail: '按协议、SNI、ALPN 和指纹聚合', status: 'info', icon: <CloudServerOutlined /> },
      { label: '高危用户', value: '风险 Top 20', detail: '源资产会话数、总流量、最近命中', status: 'risk', icon: <AlertOutlined /> },
      { label: '指纹证据', value: 'JA3 / JA3S', detail: '与加密流量页共享指纹证据', status: 'warn', icon: <KeyOutlined /> },
      { label: '取证窗口', value: 'PCAP / 会话', detail: '按隧道会话回收证据包', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: '隧道研判路径',
    lanes: [
      { phase: '识别', target: '协议族和未知加密通道', evidence: '专题接口', status: 'ok', icon: <ApiOutlined /> },
      { phase: '聚合', target: '源资产、目的对象和长连接', evidence: '用户/协议', status: 'info', icon: <NodeIndexOutlined /> },
      { phase: '解释', target: 'JA3、SNI、ALPN、证书链', evidence: '加密流量', status: 'warn', icon: <KeyOutlined /> },
      { phase: '取证', target: 'PCAP 时间窗和会话摘要', evidence: '取证分析', status: 'ok', icon: <FileSearchOutlined /> },
      { phase: '处置', target: '阻断、白名单复核、审计', evidence: '审计日志', status: 'risk', icon: <ThunderboltOutlined /> },
    ],
    actionRows: [
      { label: '提取隧道 PCAP', detail: '按源资产和目的对象生成取证任务', status: 'ok', icon: <DownloadOutlined /> },
      { label: '阻断高危通道', detail: '联动规则、SOAR 和边界策略', status: 'risk', icon: <LockOutlined /> },
      { label: '复核业务例外', detail: '对 CDN、VPN、备份流量做白名单复核', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '沉淀专题报告', detail: '输出隧道趋势、证据和处置复盘', status: 'info', icon: <AuditOutlined /> },
    ],
    drillLinks: [
      { label: '加密流量', to: '/encrypted-traffic', icon: <LockOutlined /> },
      { label: '取证分析', to: '/forensics', icon: <FileSearchOutlined /> },
      { label: '实体图谱', to: '/graph', icon: <NodeIndexOutlined /> },
      { label: '审计日志', to: '/audit-log', icon: <AuditOutlined /> },
    ],
    score: 92,
  },
  'topic-exfil': {
    tone: 'exfil',
    topicCode: 'exfil-20260620-01',
    site: '主校区',
    assetGroup: '科研文件服务 / 办公终端',
    ipRange: '10.14.0.0/16',
    protocol: 'HTTPS / S3 / WebDAV / DNS',
    timeRange: '近 24 小时 (2026-06-19 03:45:00 ~ 2026-06-20 03:45:00)',
    rule: '数据外传识别模型 v3.2',
    model: '外传路径识别模型 v2.0',
    canvasTitle: '数据外传路径分析 (Sankey)',
    canvasMode: '风险路径 TOP',
    reportTitle: '数据外传专题汇总周报',
    reportSubject: '科研文件服务 / 办公终端',
    eventTotal: 128,
    api: '/v1/topics/exfil',
    icon: <DatabaseOutlined />,
    focusModes: ['高危源资产', '跨境目的地', '云存储', '异常上传'],
    signalTitle: '外传风险信号',
    signals: [
      { label: '源资产排行', value: '源资产 Top', detail: '上传量、会话数、目的地数量', status: 'risk', icon: <DatabaseOutlined /> },
      { label: '路径分叉', value: '外传路径', detail: '源 IP 到外联目的地的风险路径', status: 'warn', icon: <BranchesOutlined /> },
      { label: '风险类型', value: '类型聚合', detail: '云盘、境外、异常端口、未知协议', status: 'info', icon: <RadarChartOutlined /> },
      { label: '证据汇聚', value: 'PCAP / 会话', detail: '关联上传窗口、目的地和审计动作', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: '数据外传闭环路径',
    lanes: [
      { phase: '发现', target: '上传突增和异常目的地', evidence: '源资产排行', status: 'risk', icon: <AlertOutlined /> },
      { phase: '定位', target: '源资产、账号、业务系统', evidence: '资产图谱', status: 'warn', icon: <SearchOutlined /> },
      { phase: '分类', target: '云存储、跨境、敏感库', evidence: '风险类型', status: 'info', icon: <DatabaseOutlined /> },
      { phase: '阻断', target: '路径、账号和目的地策略', evidence: 'SOAR 剧本', status: 'risk', icon: <ThunderboltOutlined /> },
      { phase: '固化', target: '报告、审计和复验', evidence: '合规审计', status: 'ok', icon: <AuditOutlined /> },
    ],
    actionRows: [
      { label: '阻断外传路径', detail: '按目的地、端口和源资产生成策略', status: 'risk', icon: <LockOutlined /> },
      { label: '隔离源资产', detail: '对高危源资产触发 SOAR 审批', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '提取样本证据', detail: '生成上传窗口 PCAP 和 Session 包', status: 'ok', icon: <DownloadOutlined /> },
      { label: '复核白名单', detail: '业务备份与云服务例外进入白名单治理', status: 'info', icon: <FileProtectOutlined /> },
    ],
    drillLinks: [
      { label: '资产台账', to: '/assets', icon: <DatabaseOutlined /> },
      { label: '行为基准', to: '/baselines', icon: <RadarChartOutlined /> },
      { label: 'SOAR 剧本', to: '/playbooks', icon: <ThunderboltOutlined /> },
      { label: '合规审计', to: '/compliance', icon: <AuditOutlined /> },
    ],
    score: 88,
  },
  'topic-apt': {
    tone: 'apt',
    topicCode: 'campaign-20260620-apt01',
    site: '主园区',
    assetGroup: '办公终端 / 数据中心',
    ipRange: '10.12.0.0/16',
    protocol: '初始访问 / 执行 / 横向移动 / 数据外传',
    timeRange: '近 30 天 (2026-05-21 ~ 2026-06-20 03:45:00)',
    rule: '战役关联规则 v2.4',
    model: '战役聚类模型 v1.8',
    canvasTitle: 'APT/战役攻击链画布',
    canvasMode: '布局：分层',
    reportTitle: 'APT/战役分析报告',
    reportSubject: '办公终端 / 数据中心',
    eventTotal: 156,
    api: '/v1/topics/apt',
    icon: <RadarChartOutlined />,
    focusModes: ['活跃战役', '横向移动', '高危实体', '阶段复盘'],
    signalTitle: '战役态势信号',
    signals: [
      { label: '阶段分布', value: 'ATT&CK 阶段', detail: '从初始访问到影响达成的阶段聚合', status: 'warn', icon: <BranchesOutlined /> },
      { label: '实体影响', value: '实体图谱', detail: '主机、账号、服务和目的地串联', status: 'risk', icon: <NodeIndexOutlined /> },
      { label: '关联告警', value: '战役聚类', detail: '多告警聚类成战役视角', status: 'info', icon: <AlertOutlined /> },
      { label: '复盘证据', value: '证据包', detail: '阶段、实体、PCAP、审计闭环', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: 'APT 战役阶段线',
    lanes: [
      { phase: '初始访问', target: '漏洞利用、账号异常、恶意投递', evidence: '告警证据', status: 'warn', icon: <AlertOutlined /> },
      { phase: '执行活动', target: '脚本、进程、远控工具', evidence: '行为事件', status: 'risk', icon: <ThunderboltOutlined /> },
      { phase: '横向移动', target: '账号、SMB、RDP、服务跳转', evidence: '实体图谱', status: 'risk', icon: <NodeIndexOutlined /> },
      { phase: 'C2 通信', target: '加密隧道、DGA、异常外联', evidence: '加密流量', status: 'warn', icon: <GlobalOutlined /> },
      { phase: '影响达成', target: '数据外传、破坏、持久化', evidence: '战役复盘', status: 'ok', icon: <AuditOutlined /> },
    ],
    actionRows: [
      { label: '下钻攻击链', detail: '进入阶段画布复核关键节点', status: 'risk', icon: <BranchesOutlined /> },
      { label: '导出战役包', detail: '合并关联告警、实体和取证证据', status: 'ok', icon: <DownloadOutlined /> },
      { label: '关联检测规则', detail: '把复盘结论回写规则和模型治理', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '写入审计', detail: '固化战役处理、复盘结论和责任链', status: 'info', icon: <AuditOutlined /> },
    ],
    drillLinks: [
      { label: '战役列表', to: '/campaigns', icon: <RadarChartOutlined /> },
      { label: '攻击链分析', to: '/attack-chains', icon: <BranchesOutlined /> },
      { label: '实体图谱', to: '/graph', icon: <NodeIndexOutlined /> },
      { label: '规则管理', to: '/rules', icon: <SafetyCertificateOutlined /> },
    ],
    score: 90,
  },
};

const topicOptions: Array<{ id: TopicId; label: string; param: string }> = [
  { id: 'topic-tunnel', label: '加密隧道专题', param: 'tunnel' },
  { id: 'topic-exfil', label: '数据外传专题', param: 'exfil' },
  { id: 'topic-apt', label: 'APT/战役专题', param: 'apt' },
];

const topicOverlays: OverlayContract[] = [
  {
    id: 'modal-topic-save-view',
    title: '专题保存视图',
    kind: 'Modal',
    actionLabel: '保存视图',
    description: '保存当前专题、筛选条件、图层、时间窗和关注信号。',
  },
  {
    id: 'drawer-topic-scope-edit',
    title: '专题范围编辑',
    kind: 'Drawer',
    actionLabel: '范围编辑',
    description: '编辑专题资产组、IP 段、协议、规则、模型和时间范围。',
    impact: '影响专题聚合结果、报告和订阅推送范围。',
  },
  {
    id: 'modal-topic-report-export',
    title: '专题报告导出',
    kind: 'Modal',
    actionLabel: '报告导出',
    description: '导出专题趋势、信号雷达、处置建议和业务复盘报告。',
  },
  {
    id: 'modal-topic-evidence-package-export',
    title: '专题证据包导出',
    kind: 'Modal',
    actionLabel: '证据包导出',
    description: '导出 PCAP、Session、日志、规则和模型证据包。',
    impact: '生成证据包下载记录并写入审计。',
  },
  {
    id: 'drawer-topic-subscription',
    title: '专题订阅配置',
    kind: 'Drawer',
    actionLabel: '订阅配置',
    description: '配置专题日报、周报、阈值触发和通知渠道。',
  },
  {
    id: 'dropdown-topic-share-favorite',
    title: '专题分享收藏菜单',
    kind: 'Dropdown/Menu',
    actionLabel: '分享收藏',
    description: '提供收藏、复制链接、共享给角色和加入常用入口。',
  },
];

const topicHeaderOverlays: OverlayContract[] = [
  topicOverlays[1],
  topicOverlays[0],
];

type TunnelKpi = {
  label: string;
  value: string;
  delta: string;
  status: Tone;
  icon: ReactNode;
};

type TunnelEvidenceEvent = {
  id: string;
  source: string;
  protocol: string;
  destination: string;
  evidenceType: string;
  timeRange: string;
  evidence: string[];
};

const tunnelKpis: TunnelKpi[] = [
  { label: '隧道协议数', value: '7', delta: '较昨日 +1', status: 'risk', icon: <GlobalOutlined /> },
  { label: '高频隧道源', value: '23', delta: '较昨日 +3', status: 'info', icon: <NodeIndexOutlined /> },
  { label: '加密会话流量', value: '78.3 Gbps', delta: '较昨日 +12.6%', status: 'ok', icon: <ThunderboltOutlined /> },
  { label: '异常隧道数', value: '64', delta: '较昨日 +7', status: 'risk', icon: <AlertOutlined /> },
  { label: '隧道端点数', value: '312', delta: '较昨日 +11', status: 'info', icon: <RadarChartOutlined /> },
  { label: '可疑隧道占比', value: '18.6%', delta: '较昨日 +4.2%', status: 'warn', icon: <LockOutlined /> },
  { label: '证据完整度', value: '62%', delta: '较昨日 +8%', status: 'ok', icon: <SafetyCertificateOutlined /> },
  { label: '报告置信度', value: '62%', delta: '较昨日 +8%', status: 'ok', icon: <FileDoneOutlined /> },
  { label: '未闭环风险数', value: '18', delta: '较昨日 -2', status: 'warn', icon: <FileProtectOutlined /> },
];

const tunnelProtocols: ExfilDistributionItem[] = [
  { label: 'SSH', value: 32, color: '#58bfff' },
  { label: 'TLS', value: 28, color: '#8bd85e' },
  { label: 'HTTPS', value: 20, color: '#ff6b4a' },
  { label: 'RDP', value: 10, color: '#ffb020' },
  { label: 'SOCKS', value: 6, color: '#b685ff' },
  { label: '其他', value: 4, color: '#7f8fb5' },
];

const tunnelProtocolRows = [
  { label: 'SSH', percent: '32%', traffic: '25.1 Gbps' },
  { label: 'TLS', percent: '28%', traffic: '21.9 Gbps' },
  { label: 'HTTPS', percent: '20%', traffic: '15.6 Gbps' },
  { label: 'RDP', percent: '10%', traffic: '7.8 Gbps' },
  { label: 'SOCKS', percent: '6%', traffic: '4.7 Gbps' },
  { label: '其他', percent: '4%', traffic: '3.2 Gbps' },
];

const tunnelSourceTop: ExfilBarItem[] = [
  { label: '10.12.8.45', value: 18.2 },
  { label: '10.12.6.78', value: 14.7 },
  { label: '10.12.9.33', value: 9.6 },
  { label: '10.12.3.67', value: 6.8 },
  { label: '10.12.2.55', value: 4.3 },
];

const tunnelAsnRows = [
  ['美国 (US)', '2,134', 'AS15169', '28.0'],
  ['新加坡 (SG)', '1,421', 'AS133481', '16.5'],
  ['德国 (DE)', '1,098', 'AS3320', '9.2'],
  ['荷兰 (NL)', '987', 'AS6830', '6.4'],
  ['香港 (HK)', '652', 'AS4760', '3.6'],
];

const tunnelEndpointCountryTop: ExfilBarItem[] = [
  { label: '美国', value: 2134 },
  { label: '新加坡', value: 1421 },
  { label: '德国', value: 1098 },
  { label: '荷兰', value: 987 },
  { label: '香港', value: 652 },
];

const tunnelTrend: ExfilTrendPoint[] = [
  { label: '06-13', value: 18 },
  { label: '06-14', value: 42 },
  { label: '06-15', value: 58 },
  { label: '06-16', value: 64 },
  { label: '06-17', value: 72 },
  { label: '06-18', value: 66 },
  { label: '06-19', value: 88 },
  { label: '06-20', value: 78 },
];

const tunnelJa3Rows = [
  ['JA3 指纹异常', '28', '39.4%', '771,acdc...'],
  ['自签名证书', '18', '25.4%', 'Self-Signed/CN=*'],
  ['证书过期', '12', '16.9%', 'expired / 2024-*'],
  ['域名不匹配', '13', '18.3%', 'example.com'],
];

const tunnelReuseRows = [
  ['10.12.8.45', 'SSH', '跳板机(SG)', '203.0.113.45(US)'],
  ['10.12.6.78', 'TLS', '代理(US)', '198.51.100.77(US)'],
  ['10.12.9.33', 'SOCKS', 'VPN(NL)', '45.77.34.12(NL)'],
];

const tunnelEvidenceEvents: TunnelEvidenceEvent[] = [
  { id: 'TN-20260620-0001', source: '10.12.8.45', protocol: 'SSH', destination: '203.0.113.45(SG)', evidenceType: 'PCAP', timeRange: '06-19 22:14:32 ~ 22:18:47', evidence: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'] },
  { id: 'TN-20260620-0002', source: '10.12.6.78', protocol: 'TLS', destination: '198.51.100.77(US)', evidenceType: 'Session', timeRange: '06-19 21:05:11 ~ 21:15:09', evidence: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'] },
  { id: 'TN-20260620-0003', source: '10.12.9.33', protocol: 'HTTPS', destination: '104.16.24.34(US)', evidenceType: '证书', timeRange: '06-18 08:32:00 ~ 08:42:15', evidence: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'] },
  { id: 'TN-20260620-0004', source: '10.12.3.67', protocol: 'RDP', destination: '45.77.34.12(NL)', evidenceType: 'PCAP', timeRange: '06-18 17:26:55 ~ 17:32:20', evidence: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'] },
  { id: 'TN-20260620-0005', source: '10.12.2.55', protocol: 'SOCKS', destination: '23.227.38.65(US)', evidenceType: '回溯路径', timeRange: '06-17 23:10:21 ~ 23:22:47', evidence: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'] },
];

const tunnelEvidenceCompleteness = [
  { label: '告警证据', value: '64 / 64 (100%)', status: 'ok' as const },
  { label: 'PCAP', value: '132 / 156 (84%)', status: 'warn' as const },
  { label: 'Session', value: '198 / 204 (97%)', status: 'ok' as const },
  { label: '审计日志', value: '38 / 38 (100%)', status: 'ok' as const },
  { label: '回溯路径', value: '18 / 18 (100%)', status: 'ok' as const },
  { label: '资产快照', value: '23 / 23 (100%)', status: 'ok' as const },
];

const topicIdByParam: Record<string, TopicId> = {
  tunnel: 'topic-tunnel',
  exfil: 'topic-exfil',
  apt: 'topic-apt',
};

const resolveTopicId = (topicParam: string | null, tabParam: string | null): TopicId => {
  const param = topicParam ?? tabParam;
  return param && topicIdByParam[param] ? topicIdByParam[param] : 'topic-tunnel';
};

export function TopicWorkbenchPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedTopic = resolveTopicId(searchParams.get('topic'), searchParams.get('tab'));
  const selectedRoute = findRouteById(selectedTopic);
  const topicPage = selectedRoute?.page ?? route.page;
  const config = topicConfigs[selectedTopic];
  const [focusMode, setFocusMode] = useState(config.focusModes[0]);
  const [selectedSignal, setSelectedSignal] = useState(config.signals[0].label);

  useEffect(() => {
    setFocusMode(config.focusModes[0]);
    setSelectedSignal(config.signals[0].label);
  }, [config]);

  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', selectedTopic],
    queryFn: () => fetchPageSnapshot(selectedTopic),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const metrics = topicPage.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const evidenceRows = data?.evidence.length ? data.evidence : topicPage.evidence.map((label) => ({ label, value: '待返回', status: 'info' as const }));
  const columns: ColumnsType<SnapshotRow> = topicPage.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderTopicCell(column, value),
  }));

  if (selectedTopic === 'topic-tunnel') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`}>
          <div className="taf-topic-tunnel-layout">
            <div className="taf-topic-tunnel-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <OverlayContractHost overlays={topicHeaderOverlays} compact />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', config.topicCode],
                  ['站点', config.site],
                  ['资产组', config.assetGroup],
                  ['IP 段', config.ipRange],
                  ['协议', config.protocol],
                  ['时间窗', config.timeRange],
                  ['规则', config.rule],
                  ['模型', config.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <TunnelKpiStrip metrics={metrics} />

              <main className="taf-topic-main taf-topic-tunnel-main">
                <div className="taf-topic-boardline taf-topic-tunnel-boardline">
                  <WorkPanel
                    title={config.canvasTitle}
                    className="taf-topic-canvas-panel taf-topic-tunnel-impact-panel"
                    extra={(
                      <span className="taf-topic-tunnel-panel-actions">
                        <TopicActionButton topic={config.topicCode} title="布局：径向" target={config.canvasTitle}>布局：径向</TopicActionButton>
                        <TopicActionButton topic={config.topicCode} title="全屏" target={config.canvasTitle}>全屏</TopicActionButton>
                      </span>
                    )}
                  >
                    <TunnelImpactMap rows={rows} />
                    <div className="taf-topic-alert-strip taf-topic-tunnel-alert-strip">
                      {config.signals.slice(0, 3).map((signal) => (
                        <button
                          key={signal.label}
                          type="button"
                          className={`taf-topic-alert-chip is-${signal.status} ${selectedSignal === signal.label ? 'is-selected' : ''}`}
                          onClick={() => setSelectedSignal(signal.label)}
                        >
                          <span>{signal.icon}</span>
                          <strong>{signal.label}</strong>
                          <em>{signal.value}</em>
                        </button>
                      ))}
                    </div>
                  </WorkPanel>

                  <TunnelAnalysisPanel rows={rows} metrics={metrics} />
                </div>

                <WorkPanel title={`加密隧道关联事件与证据 / topic: ${config.topicCode}`} className="taf-topic-table-panel taf-topic-tunnel-table-panel" extra={<TunnelTableToolbar topic={config.topicCode} />}>
                  <TunnelEvidenceTable rows={rows} isLoading={isLoading} />
                </WorkPanel>
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-tunnel-rail">
              <TunnelRightRail config={config} metrics={metrics} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  if (selectedTopic === 'topic-exfil') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`}>
          <div className="taf-topic-exfil-layout">
            <div className="taf-topic-exfil-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <OverlayContractHost overlays={topicHeaderOverlays} compact />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', config.topicCode],
                  ['站点', config.site],
                  ['资产组', config.assetGroup],
                  ['IP 段', config.ipRange],
                  ['协议', config.protocol],
                  ['时间窗', config.timeRange],
                  ['规则', config.rule],
                  ['模型', config.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <div className="taf-topic-kpis">
                {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
              </div>

              <main className="taf-topic-main taf-topic-exfil-main">
                <div className="taf-topic-boardline taf-topic-exfil-boardline">
                  <WorkPanel
                    title={config.canvasTitle}
                    className="taf-topic-canvas-panel taf-topic-exfil-canvas-panel"
                    extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
                  >
                    <ExfilCanvas rows={rows} metrics={metrics} />
                  </WorkPanel>

                  <ExfilAnalysisDashboard rows={rows} metrics={metrics} focusMode={focusMode} />
                </div>

                <WorkPanel title={topicPage.tableTitle} className="taf-topic-table-panel taf-topic-exfil-table-panel" extra={<span>专题总览</span>}>
                  <Table
                    rowKey={(record) => String(record[topicPage.tableColumns[0]] ?? JSON.stringify(record))}
                    size="small"
                    loading={isLoading}
                    columns={columns}
                    dataSource={rows}
                    pagination={{ pageSize: 5, size: 'small' }}
                    scroll={{ x: 980, y: 142 }}
                  />
                </WorkPanel>
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-exfil-rail">
              <ExfilRightRail config={config} metrics={metrics} evidenceRows={evidenceRows} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  if (selectedTopic === 'topic-apt') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`}>
          <div className="taf-topic-apt-layout">
            <div className="taf-topic-apt-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <OverlayContractHost overlays={topicHeaderOverlays} compact />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', config.topicCode],
                  ['站点', config.site],
                  ['资产组', config.assetGroup],
                  ['IP 段', config.ipRange],
                  ['协议', config.protocol],
                  ['时间窗', config.timeRange],
                  ['规则', config.rule],
                  ['模型', config.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <div className="taf-topic-kpis">
                {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
              </div>

              <main className="taf-topic-main taf-topic-apt-main">
                <div className="taf-topic-boardline taf-topic-apt-boardline">
                  <WorkPanel
                    title={config.canvasTitle}
                    className="taf-topic-canvas-panel taf-topic-apt-canvas-panel"
                    extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
                  >
                    <TopicCanvas topicId={selectedTopic} config={config} rows={rows} metrics={metrics} selectedSignal={selectedSignal} />
                  </WorkPanel>

                  <AptAnalysisDashboard rows={rows} metrics={metrics} focusMode={focusMode} />
                </div>

                <div className="taf-topic-apt-bottomline">
                  <WorkPanel title={`战役关联事件与证据 / ${config.topicCode}`} className="taf-topic-table-panel taf-topic-apt-table-panel" extra={<AptEvidenceToolbar />}>
                    <AptEvidenceTable rows={rows} isLoading={isLoading} />
                  </WorkPanel>

                  <AptResponsePanel rows={rows} metrics={metrics} />
                </div>
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-apt-rail">
              <AptRightRail config={config} metrics={metrics} evidenceRows={evidenceRows} rows={rows} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  return (
    <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
      <section className={`taf-topic-shell bg-${topicPage.background}`}>
        <header className="taf-topic-titlebar">
          <div className="taf-topic-title-main">
            <h1>{route.page.title}</h1>
          </div>
          <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
            {topicOptions.map((option) => (
              <button
                key={option.id}
                type="button"
                role="tab"
                aria-selected={option.id === selectedTopic}
                className={option.id === selectedTopic ? 'is-active' : ''}
                onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div className="taf-topic-controls">
            <OverlayContractHost overlays={topicHeaderOverlays} compact />
          </div>
        </header>

        <div className="taf-topic-facts" aria-label="专题筛选条件">
          {[
            ['专题ID', config.topicCode],
            ['站点', config.site],
            ['资产组', config.assetGroup],
            ['IP 段', config.ipRange],
            ['协议', config.protocol],
            ['时间窗', config.timeRange],
            ['规则', config.rule],
            ['模型', config.model],
          ].map(([label, value]) => (
            <span key={label}>
              <b>{label}：</b>
              <em>{value}</em>
            </span>
          ))}
        </div>

        {isError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
            action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
          />
        )}

        <div className="taf-topic-kpis">
          {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
        </div>

        <div className="taf-topic-grid">
          <main className="taf-topic-main">
            <div className="taf-topic-boardline">
              <WorkPanel
                title={config.canvasTitle}
                className="taf-topic-canvas-panel"
                extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
              >
                <TopicCanvas topicId={selectedTopic} config={config} rows={rows} metrics={metrics} selectedSignal={selectedSignal} />
                <div className="taf-topic-alert-strip">
                  {config.signals.slice(0, 3).map((signal) => (
                    <button
                      key={signal.label}
                      type="button"
                      className={`taf-topic-alert-chip is-${signal.status} ${selectedSignal === signal.label ? 'is-selected' : ''}`}
                      onClick={() => setSelectedSignal(signal.label)}
                    >
                      <span>{signal.icon}</span>
                      <strong>{signal.label}</strong>
                      <em>{signal.value}</em>
                    </button>
                  ))}
                </div>
              </WorkPanel>

              <AptAnalysisDashboard rows={rows} metrics={metrics} focusMode={focusMode} />
            </div>

            <WorkPanel title={topicPage.tableTitle} className="taf-topic-table-panel" extra={<span>{topicPage.tabs[0]}</span>}>
              <Table
                rowKey={(record) => String(record[topicPage.tableColumns[0]] ?? JSON.stringify(record))}
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={rows}
                pagination={{ pageSize: 5, size: 'small' }}
                scroll={{ x: 980, y: 142 }}
              />
            </WorkPanel>
          </main>

          <aside className="taf-topic-rail">
            <AptRightRail config={config} metrics={metrics} evidenceRows={evidenceRows} rows={rows} />
          </aside>
        </div>
      </section>
    </div>
  );
}

function TunnelKpiStrip({ metrics }: { metrics: SnapshotMetric[] }) {
  const useTargetValues = isVisualBreakdownMode();
  return (
    <div className="taf-topic-kpis taf-topic-tunnel-kpis" aria-label="加密隧道专题指标">
      {tunnelKpis.map((item) => {
        const metric = metrics.find((candidate) => candidate.label === item.label);
        const value = useTargetValues ? item.value : metric?.value || item.value;
        const delta = useTargetValues ? item.delta : metric?.delta || item.delta;
        const status = useTargetValues ? item.status : metric?.status || item.status;
        return (
          <div key={item.label} className={`taf-topic-tunnel-kpi is-${status}`} title={`${item.label}: ${value}, ${delta}`}>
            <span>{item.icon}</span>
            <b>{item.label}</b>
            <strong>{value}</strong>
            <em>{delta}</em>
          </div>
        );
      })}
    </div>
  );
}

function TunnelImpactMap({ rows }: { rows: SnapshotRow[] }) {
  const [selectedNode, setSelectedNode] = useState('risk-01');
  const rowIps = rows
    .map((row) => rowText(row, '源资产') || rowText(row, '隧道源'))
    .filter((value) => /^\d{1,3}(?:\.\d{1,3}){3}$/.test(value));
  const riskIps = ['10.12.8.45', '10.12.6.78', '10.12.9.33'].map((fallback, index) => rowIps[index] ?? fallback);
  const baseNodes: TopicTopologyNode[] = [
    { id: 'asset-office', label: '办公终端组', detail: '668 资产', tone: 'asset', x: 7, y: 23 },
    { id: 'asset-server', label: '服务群组', detail: '284 资产', tone: 'asset', x: 7, y: 49 },
    { id: 'asset-storage', label: '数据存储', detail: '76 资产', tone: 'asset', x: 7, y: 74 },
    { id: 'probe-01', label: 'Probe-01', detail: '10.12.1.11', tone: 'probe', x: 25, y: 37 },
    { id: 'probe-02', label: 'Probe-02', detail: '10.12.1.12', tone: 'probe', x: 25, y: 67 },
    { id: 'risk-01', label: riskIps[0], detail: '高风险隧道源', tone: 'risk', x: 42, y: 28 },
    { id: 'risk-02', label: riskIps[1], detail: '高风险隧道源', tone: 'risk', x: 42, y: 51 },
    { id: 'risk-03', label: riskIps[2], detail: '高风险隧道源', tone: 'risk', x: 42, y: 74 },
    { id: 'protocol-ssh', label: 'SSH 隧道', detail: '203 会话', tone: 'protocol', x: 58, y: 22 },
    { id: 'protocol-tls', label: 'TLS 隧道', detail: '165.1M 会话', tone: 'protocol', x: 58, y: 38 },
    { id: 'protocol-https', label: 'HTTPS 隧道', detail: '104.3M 会话', tone: 'protocol', x: 58, y: 54 },
    { id: 'protocol-rdp', label: 'RDP 隧道', detail: '8.6M 会话', tone: 'protocol', x: 58, y: 70 },
    { id: 'protocol-socks', label: 'SOCKS 隧道', detail: '6.2M 会话', tone: 'protocol', x: 58, y: 86 },
    { id: 'proxy-sg', label: '跳板机', detail: 'SG.ASN 45102', tone: 'proxy', x: 75, y: 38 },
    { id: 'proxy-nl', label: 'VPN/中转', detail: 'NL.ASN 6830', tone: 'proxy', x: 75, y: 67 },
    { id: 'dest-us', label: '美国', detail: '45 节点', tone: 'destination', x: 92, y: 17 },
    { id: 'dest-sg', label: '新加坡', detail: '28 节点', tone: 'destination', x: 92, y: 34 },
    { id: 'dest-hk', label: '香港', detail: '79 节点', tone: 'destination', x: 92, y: 51 },
    { id: 'dest-nl', label: '荷兰', detail: '12 节点', tone: 'destination', x: 92, y: 68 },
    { id: 'dest-de', label: '德国', detail: '9 节点', tone: 'destination', x: 92, y: 84 },
  ];
  const nodes = baseNodes.map((node) => ({ ...node, selected: node.id === selectedNode }));
  const links: TopicTopologyLink[] = [
    ['asset-office', 'probe-01', 'info'], ['asset-server', 'probe-01', 'info'], ['asset-storage', 'probe-02', 'info'],
    ['probe-01', 'risk-01', 'info'], ['probe-01', 'risk-02', 'info'], ['probe-02', 'risk-02', 'info'], ['probe-02', 'risk-03', 'info'],
    ['risk-01', 'protocol-ssh', 'risk'], ['risk-01', 'protocol-tls', 'risk'], ['risk-02', 'protocol-https', 'risk'], ['risk-02', 'protocol-rdp', 'risk'], ['risk-03', 'protocol-socks', 'risk'],
    ['protocol-ssh', 'proxy-sg', 'ok'], ['protocol-tls', 'proxy-sg', 'ok'], ['protocol-https', 'proxy-nl', 'ok'], ['protocol-rdp', 'proxy-nl', 'ok'], ['protocol-socks', 'proxy-nl', 'ok'],
    ['proxy-sg', 'dest-us', 'purple'], ['proxy-sg', 'dest-sg', 'purple'], ['proxy-sg', 'dest-hk', 'purple'], ['proxy-nl', 'dest-nl', 'purple'], ['proxy-nl', 'dest-de', 'purple'],
  ].map(([source, target, tone]) => ({ source, target, tone: tone as TopicTopologyLink['tone'] }));

  return (
    <div className="taf-topic-canvas taf-topic-tunnel-impact">
      <div className="taf-topic-canvas-legend taf-topic-tunnel-legend">
        {['主机/资产', '探针', '隧道协议', '代理/跳板', '外部端点', '告警', '战役'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <TopicTopologyGraph ariaLabel="加密隧道局部影响面关系图" nodes={nodes} links={links} onNodeClick={setSelectedNode} />
    </div>
  );
}

function TunnelAnalysisPanel({ rows, metrics }: { rows: SnapshotRow[]; metrics: SnapshotMetric[] }) {
  const topSources = buildTunnelTopSources(rows);
  const completeness = Math.round(metricValueNumber(metrics, '证据完整度') || 62);
  const tabs = ['协议分析', '隧道源TOP5', '端点国家分布'];
  const [activeTab, setActiveTab] = useState(tabs[0]);
  return (
    <WorkPanel
      title="加密隧道分析"
      className="taf-topic-tunnel-analysis"
      extra={(
        <span className="taf-topic-tunnel-analysis-tabs">
          {tabs.map((item) => (
            <button key={item} type="button" className={activeTab === item ? 'is-active' : ''} aria-selected={activeTab === item} onClick={() => setActiveTab(item)}>{item}</button>
          ))}
        </span>
      )}
    >
      <div className="taf-topic-tunnel-analysis-grid">
        <section className="taf-topic-tunnel-card is-protocol">
          <header>
            <strong>协议</strong>
            <span>{completeness}% 证据完整</span>
          </header>
          <div className="taf-topic-tunnel-protocol-body">
            <ExfilPieChart items={tunnelProtocols} ariaLabel="加密隧道协议占比" />
            <div className="taf-topic-tunnel-protocol-table">
              {tunnelProtocolRows.map((row) => (
                <span key={row.label} title={`${row.label} ${row.percent} ${row.traffic}`}>
                  <b>{row.label}</b>
                  <em>{row.percent}</em>
                  <strong>{row.traffic}</strong>
                </span>
              ))}
            </div>
          </div>
        </section>

        <section className="taf-topic-tunnel-card is-source">
          <header>
            <strong>高频隧道源 TOP5</strong>
            <span>流量 (Gbps)</span>
          </header>
          <ExfilBarChart items={topSources} ariaLabel="高频隧道源 TOP5" />
        </section>

        <section className="taf-topic-tunnel-card is-asn">
          <header>
            <strong>端点国家 / ASN TOP5</strong>
            <span>外部目的端点数 / ASN</span>
          </header>
          <div className="taf-topic-tunnel-asn-body">
            <div className="taf-topic-tunnel-asn-chart">
              <ExfilBarChart items={tunnelEndpointCountryTop} ariaLabel="外部隧道端点国家分布 TOP5" />
            </div>
            <div className="taf-topic-tunnel-mini-table is-asn">
              <b>国家/地区</b><b>端点数</b><b>ASN</b><b>流量</b>
              {tunnelAsnRows.flatMap((row) => row.map((cell, index) => <span key={`${row[0]}-${index}`} title={cell}>{cell}</span>))}
            </div>
          </div>
        </section>

        <section className="taf-topic-tunnel-card is-trend">
          <header>
            <strong>加密流量趋势 (Gbps)</strong>
            <span>06-13 ~ 06-20</span>
          </header>
          <ExfilLineChart points={tunnelTrend} ariaLabel="加密流量趋势" />
        </section>

        <section className="taf-topic-tunnel-card is-ja3">
          <header>
            <strong>JA3 / 证书异常疑点</strong>
            <span>类型</span>
          </header>
          <div className="taf-topic-tunnel-mini-table is-ja3">
            <b>类型</b><b>数量</b><b>占比</b><b>示例</b>
            {tunnelJa3Rows.flatMap((row) => row.map((cell, index) => <span key={`${row[0]}-${index}`} title={cell}>{cell}</span>))}
          </div>
        </section>

        <section className="taf-topic-tunnel-card is-reuse">
          <header>
            <strong>隧道复用路径（示例）</strong>
            <span>源主机 / 协议 / 代理 / 目的端点</span>
          </header>
          <div className="taf-topic-tunnel-reuse">
            {tunnelReuseRows.map((row) => (
              <span key={row.join('-')} title={row.join(' -> ')}>
                {row.map((cell, index) => <b key={cell}>{cell}{index < row.length - 1 ? <i /> : null}</b>)}
              </span>
            ))}
          </div>
        </section>
      </div>
    </WorkPanel>
  );
}

function TunnelTableToolbar({ topic }: { topic: string }) {
  return (
    <span className="taf-topic-tunnel-table-toolbar">
      <TopicActionButton topic={topic} title="证据类型：全部">证据类型：全部</TopicActionButton>
      <TopicActionButton topic={topic} title="阶段：全部">阶段：全部</TopicActionButton>
      <TopicActionButton topic={topic} title="风险等级：全部">风险等级：全部</TopicActionButton>
      <TopicActionButton topic={topic} title="搜索隧道证据" ariaLabel="搜索"><SearchOutlined /></TopicActionButton>
    </span>
  );
}

function TunnelEvidenceTable({ rows, isLoading }: { rows: SnapshotRow[]; isLoading: boolean }) {
  const hasRealTunnelRows = !isVisualBreakdownMode() && rows.some((row) => rowText(row, '事件ID').startsWith('TN-'));
  const events = hasRealTunnelRows ? rows.map((row, index) => ({
    id: rowText(row, '事件ID') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].id,
    source: rowText(row, '隧道源') || rowText(row, '源资产') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].source,
    protocol: rowText(row, '协议') || rowText(row, '协议族') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].protocol,
    destination: rowText(row, '目的端点') || rowText(row, '目标对象') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].destination,
    evidenceType: rowText(row, '证据类型') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].evidenceType,
    timeRange: rowText(row, '时间窗') || tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].timeRange,
    evidence: tunnelEvidenceEvents[index % tunnelEvidenceEvents.length].evidence,
  })) : tunnelEvidenceEvents;

  const pageSize = 5;
  const pagedEvents = events.length > pageSize
    ? events
    : Array.from({ length: 12 }, (_, index) => ({
      ...events[index % Math.max(events.length, 1)],
      id: `${events[index % Math.max(events.length, 1)]?.id ?? `TN-${index + 1}`}-${String(index + 1).padStart(2, '0')}`,
    }));
  const [page, setPage] = useState(1);
  const pageCount = Math.max(1, Math.ceil(pagedEvents.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const visibleEvents = pagedEvents.slice((currentPage - 1) * pageSize, currentPage * pageSize);

  return (
    <div className="taf-topic-tunnel-table" aria-busy={isLoading}>
      <div className="taf-topic-tunnel-table-head">
        {['事件ID', '隧道源', '协议', '目的端点', '证据类型', '时间窗', '风险状态', '风险操作'].map((label) => <b key={label}>{label}</b>)}
      </div>
      <div className="taf-topic-tunnel-table-body">
        {visibleEvents.map((row) => (
          <div key={row.id} className="taf-topic-tunnel-table-row">
            <span title={row.id}>{row.id}</span>
            <span title={row.source}>{row.source}</span>
            <span title={row.protocol}>{row.protocol}</span>
            <span title={row.destination}>{row.destination}</span>
            <span title={row.evidenceType}>{row.evidenceType}</span>
            <span title={row.timeRange}>{row.timeRange}</span>
            <span className="taf-topic-tunnel-evidence-tags">
              {row.evidence.slice(0, 3).map((item) => <i key={item}>{item}</i>)}
            </span>
            <span className="taf-topic-tunnel-evidence-tags">
              {row.evidence.slice(3).map((item) => <i key={item}>{item}</i>)}
            </span>
          </div>
        ))}
      </div>
      <div className="taf-topic-tunnel-table-footer">
        <span>共 {pagedEvents.length} 条</span>
        <button type="button" aria-label="隧道证据上一页" title="上一页" disabled={currentPage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>‹</button>
        {Array.from({ length: pageCount }, (_, index) => index + 1).map((value) => (
          <button key={value} type="button" className={currentPage === value ? 'is-active' : ''} aria-current={currentPage === value ? 'page' : undefined} title={`第 ${value} 页`} onClick={() => setPage(value)}>{value}</button>
        ))}
        <button type="button" aria-label="隧道证据下一页" title="下一页" disabled={currentPage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>›</button>
        <span>{pageSize} 条/页</span>
      </div>
    </div>
  );
}

function TunnelRightRail({ config, metrics }: { config: TopicConfig; metrics: SnapshotMetric[] }) {
  const completeness = isVisualBreakdownMode() ? 62 : Math.round(metricValueNumber(metrics, '证据完整度') || 62);
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出总报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['试点周报导出', <ExportOutlined key="export" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];

  return (
    <>
      <WorkPanel title={`专题交付摘要 / ${config.topicCode}`} className="taf-topic-tunnel-delivery">
        <div className="taf-topic-tunnel-delivery-grid">
          <div className="taf-topic-tunnel-ring" style={{ '--value': completeness } as CSSProperties}>
            <span>报告就绪度</span>
            <strong>{completeness}%</strong>
            <em>较昨日 +8%</em>
          </div>
          <div className="taf-topic-tunnel-delivery-stats">
            {[
              ['可生成报告', '7', 'ok'],
              ['待补证据', '3', 'warn'],
              ['未闭环风险', '18', 'risk'],
            ].map(([label, value, tone]) => (
              <span key={label} className={`is-${tone}`} title={`${label}: ${value}`}>
                <i />
                <b>{label}</b>
                <strong>{value}</strong>
              </span>
            ))}
          </div>
        </div>
        <div className="taf-topic-tunnel-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出报告" className="ant-btn ant-btn-default ant-btn-sm"><DownloadOutlined />导出报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm"><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / 加密隧道专题" className="taf-topic-tunnel-evidence-panel">
        <div className="taf-topic-tunnel-evidence-list">
          {tunnelEvidenceCompleteness.map((item) => (
            <span key={item.label} className={`is-${item.status}`} title={`${item.label}: ${item.value}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="报告预览 / 当前保存视图" className="taf-topic-tunnel-report-panel">
        <div className="taf-topic-report-preview taf-topic-tunnel-report-preview">
          <div className="taf-topic-report-sheet">
            <span />
            <span />
            <span />
            <i />
          </div>
          <div>
            <strong>报告类型：加密隧道专题_试点周报</strong>
            <span>时间窗：2026-06-13 ~ 2026-06-20</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：2026-06-20 03:40:12</span>
            <TopicActionButton topic={config.topicCode} title="预览报告" className="ant-btn ant-btn-link ant-btn-sm">预览报告</TopicActionButton>
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-tunnel-action-panel">
        <div className="taf-topic-exfil-action-grid taf-topic-tunnel-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function buildTunnelTopSources(rows: SnapshotRow[]): ExfilBarItem[] {
  const values = rows
    .map((row) => ({
      label: rowText(row, '隧道源') || rowText(row, '源资产'),
      value: rowNumber(row, '流量') || rowNumber(row, '总流量') || rowNumber(row, '会话数'),
    }))
    .filter((item) => /^\d{1,3}(?:\.\d{1,3}){3}$/.test(item.label) && item.value > 0)
    .slice(0, 5);
  return values.length >= 5 ? values : tunnelSourceTop;
}

function TopicCanvas({
  topicId,
  config,
  rows,
  metrics,
  selectedSignal,
}: {
  topicId: TopicId;
  config: TopicConfig;
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  selectedSignal: string;
}) {
  if (topicId === 'topic-exfil') return <ExfilCanvas rows={rows} metrics={metrics} />;
  if (topicId === 'topic-apt') return <AptCanvas config={config} rows={rows} metrics={metrics} selectedSignal={selectedSignal} />;
  return <TunnelCanvas config={config} rows={rows} />;
}

function ExfilRightRail({
  config,
  metrics,
  evidenceRows,
}: {
  config: TopicConfig;
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
}) {
  const completeness = Math.max(0, Math.min(100, Math.round(metricValueNumber(metrics, '证据完整度') || 62)));
  const summaryStats: Array<[string, string]> = [
    ['可生成报告', '7'],
    ['待补证据', '3'],
    ['未闭环风险', '18'],
  ];
  const evidence = evidenceRows.length ? evidenceRows : [
    { label: '告警证据', value: '64 / 64 (100%)', status: 'ok' as const },
    { label: 'PCAP', value: '132 / 156 (84%)', status: 'warn' as const },
    { label: 'Session', value: '198 / 204 (97%)', status: 'ok' as const },
    { label: '审计日志', value: '38 / 38 (100%)', status: 'ok' as const },
    { label: '回溯路径', value: '18 / 18 (100%)', status: 'ok' as const },
    { label: '资产快照', value: '23 / 23 (100%)', status: 'ok' as const },
  ];
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出总报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['试点周报导出', <ExportOutlined key="export" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];

  return (
    <>
      <WorkPanel title={`专题交付摘要 / ${config.topicCode}`} className="taf-topic-exfil-delivery">
        <div className="taf-topic-exfil-delivery-grid">
          <div className="taf-topic-exfil-delivery-ring" style={{ '--value': completeness } as CSSProperties}>
            <strong>{completeness}%</strong>
            <span>较昨日 +6%</span>
          </div>
          <div className="taf-topic-exfil-delivery-stats">
            {summaryStats.map(([label, value]) => (
              <span key={label}>
                <i />
                <b>{label}</b>
                <strong>{value}</strong>
              </span>
            ))}
          </div>
        </div>
        <div className="taf-topic-exfil-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出总报告" className="ant-btn ant-btn-default ant-btn-sm"><DownloadOutlined />导出总报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm"><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / 数据外传专题" className="taf-topic-exfil-evidence">
        <div className="taf-topic-exfil-evidence-list">
          {evidence.slice(0, 6).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="报告预览 / 当前保存视图" className="taf-topic-exfil-report">
        <div className="taf-topic-report-preview">
          <div className="taf-topic-report-sheet">
            <span />
            <span />
            <span />
            <i />
          </div>
          <div>
            <strong>{config.reportTitle}</strong>
            <span>时间窗：近 24 小时</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：2026-06-20 03:40:12</span>
            <TopicActionButton topic={config.topicCode} title="预览报告" className="ant-btn ant-btn-link ant-btn-sm">预览报告</TopicActionButton>
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-exfil-action-panel">
        <div className="taf-topic-exfil-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function TunnelCanvas({ config, rows }: { config: TopicConfig; rows: SnapshotRow[] }) {
  const sources = rows.slice(0, 3).map((row, index) => String(row['源资产'] ?? `10.12.${index + 6}.${45 + index}`));
  const destinations = ['美国 45 节点', '新加坡 28 节点', '香港 79 节点', '德国 12 节点'];
  return (
    <div className="taf-topic-canvas taf-topic-tunnel-canvas">
      <div className="taf-topic-canvas-legend">
        {['主机/资产', '探针', '隧道协议', '代理/跳板', '外部端点', '告警'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <div className="taf-topic-flow-map">
        <div className="taf-topic-flow-col">
          {sources.map((source, index) => <FlowNode key={source} tone="source" title={source} detail={`${668 - index * 212} 资产`} />)}
        </div>
        <div className="taf-topic-flow-col is-probe">
          {['Probe-01 10.12.1.11', 'Probe-02 10.12.1.12'].map((item) => <FlowNode key={item} tone="probe" title={item} detail="在线采集" />)}
        </div>
        <div className="taf-topic-flow-col is-risk">
          {['10.12.8.45', '10.12.6.78', '10.12.9.33'].map((item) => <FlowNode key={item} tone="risk" title={item} detail="高风险隧道源" />)}
        </div>
        <div className="taf-topic-flow-col is-protocol">
          {['SSH 隧道 203 会话', 'TLS 隧道 165.1M 会话', 'HTTPS 隧道 104.3M 会话', 'SOCKS 隧道 6.2M 会话'].map((item) => <FlowNode key={item} tone="protocol" title={item} detail={config.protocol} />)}
        </div>
        <div className="taf-topic-flow-col is-destination">
          {destinations.map((item) => <FlowNode key={item} tone="destination" title={item} detail="外部端点" />)}
        </div>
      </div>
    </div>
  );
}

function ExfilCanvas({ rows, metrics }: { rows: SnapshotRow[]; metrics: SnapshotMetric[] }) {
  const model = buildExfilVisualModel(rows, metrics);
  return (
    <div className="taf-topic-canvas taf-topic-exfil-canvas">
      <div className="taf-topic-canvas-legend">
        {['内部源', '文件服务', '代理/中转', '外部目的地', '风险路径'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <div className="taf-topic-sankey">
        <ExfilSankeyChart nodes={model.sankeyNodes} links={model.sankeyLinks} ariaLabel="数据外传路径 Sankey" />
        <div className="taf-topic-sankey-summary">
          <span>总外传流量：{model.totalUploadGb.toFixed(1)} GB</span>
          <span>涉及路径：{model.pathCount}</span>
          <span>闭环可信度：{model.confidence}%</span>
        </div>
      </div>
    </div>
  );
}

function ExfilAnalysisDashboard({ rows, metrics, focusMode }: { rows: SnapshotRow[]; metrics: SnapshotMetric[]; focusMode: string }) {
  const model = buildExfilVisualModel(rows, metrics);

  return (
    <div className="taf-topic-exfil-dashboard" aria-label="数据外传分析">
      <div className="taf-topic-exfil-card is-table">
        <header>
          <strong>目的地国家/ASN TOP 5</strong>
          <span>{focusMode}</span>
        </header>
        <div className="taf-topic-exfil-table">
          <b>国家/地区</b><b>ASN</b><b>流量</b><b>占比</b>
          {model.destinationRows.map((row) => [
            <span key={`${row.region}-region`}>{row.region}</span>,
            <span key={`${row.region}-asn`}>{row.asn}</span>,
            <span key={`${row.region}-traffic`}>{row.traffic}</span>,
            <span key={`${row.region}-ratio`}>{row.ratio}</span>,
          ])}
        </div>
      </div>

      <ExfilDistributionCard title="敏感数据类型分布" items={model.sensitiveTypes} />

      <div className="taf-topic-exfil-card is-trend">
        <header>
          <strong>异常上传峰值趋势 (Gbps)</strong>
          <span>峰值 {Math.max(...model.trend.map((point) => point.value)).toFixed(1)} GB</span>
        </header>
        <ExfilLineChart points={model.trend} ariaLabel="异常上传峰值趋势" />
      </div>

      <ExfilDistributionCard title="外传协议占比" items={model.protocols} />

      <div className="taf-topic-exfil-card is-score">
        <header>
          <strong>路径置信度评分</strong>
          <span>{model.pathCount} 条路径</span>
        </header>
        <div className="taf-topic-exfil-score-ring" style={{ '--value': model.confidence } as CSSProperties}>
          <strong>{model.confidence}</strong>
          <span>/100</span>
        </div>
      </div>

      <div className="taf-topic-exfil-card is-bars">
        <header>
          <strong>可疑账号/服务分布 TOP 5</strong>
          <span>命中 {model.accounts.reduce((sum, item) => sum + item.value, 0)}</span>
        </header>
        <ExfilBarChart items={model.accounts} ariaLabel="可疑账号和服务分布" />
      </div>
    </div>
  );
}

function ExfilDistributionCard({ title, items }: { title: string; items: ExfilDistributionItem[] }) {
  const total = items.reduce((sum, item) => sum + item.value, 0);
  const primaryValue = total ? Math.round((items[0]?.value ?? 0) / total * 100) : 0;
  return (
    <div className="taf-topic-exfil-card is-donut">
      <header>
        <strong>{title}</strong>
        <span>{primaryValue}%</span>
      </header>
      <div className="taf-topic-exfil-donut-layout">
        <ExfilPieChart items={items} ariaLabel={title} center={['50%', '50%']} radius={['30%', '54%']} />
        <div className="taf-topic-exfil-legend">
          {items.map((item) => (
            <span key={item.label} style={{ '--color': item.color } as CSSProperties}>
              <b>{item.label}</b>
              <em>{item.value.toFixed(1)}%</em>
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

function buildExfilVisualModel(rows: SnapshotRow[], metrics: SnapshotMetric[]): ExfilVisualModel {
  const sourceRows = rows.length ? rows : buildExfilFallbackRows();
  const uploadByType = groupRows(sourceRows, '数据类型', '上传量');
  const uploadByDestination = groupRows(sourceRows, '目标区域', '上传量');
  const uploadByRisk = groupRows(sourceRows, '风险类型', '上传量');
  const sessionsBySource = groupRows(sourceRows, '源资产', '会话数');
  const totalUploadGb = sourceRows.reduce((sum, row) => sum + rowUploadGb(row), 0);
  const sourceCount = metricNumber(metrics, '可疑外传源') || sessionsBySource.length || sourceRows.length;
  const pathCount = metricNumber(metrics, '外传路径数') || sourceRows.length;
  const evidenceRate = metricNumber(metrics, '证据完整度') || 88;
  const confidence = Math.max(58, Math.min(99, Math.round(evidenceRate)));
  const topSources = sessionsBySource.slice(0, 5);
  const topTypes = uploadByType.slice(0, 5);
  const topRiskTypes = uploadByRisk.slice(0, 4);
  const topDestinations = uploadByDestination.slice(0, 5);

  const sankeyNodes = [
    ...topSources.map((item) => ({ name: item.label, depth: 0 })),
    ...topTypes.map((item) => ({ name: `类型:${item.label}`, depth: 1 })),
    ...topRiskTypes.map((item) => ({ name: `风险:${item.label}`, depth: 2 })),
    ...topDestinations.map((item) => ({ name: item.label, depth: 3 })),
    ...sourceRows.slice(0, 4).map((row, index) => ({ name: `路径-${String(index + 1).padStart(2, '0')}`, depth: 4 })),
  ];

  const sankeyLinks: ExfilSankeyLink[] = [];
  sourceRows.slice(0, 8).forEach((row, index) => {
    const source = firstMatchingGroup(rowText(row, '源资产'), topSources);
    const dataType = firstMatchingGroup(rowText(row, '数据类型'), topTypes);
    const riskType = firstMatchingGroup(rowText(row, '风险类型'), topRiskTypes);
    const destination = firstMatchingGroup(rowText(row, '目标区域'), topDestinations);
    const riskPath = `路径-${String((index % 4) + 1).padStart(2, '0')}`;
    const value = Math.max(1, rowUploadGb(row));

    if (source && dataType) sankeyLinks.push({ source, target: `类型:${dataType}`, value });
    if (dataType && riskType) sankeyLinks.push({ source: `类型:${dataType}`, target: `风险:${riskType}`, value: value * 0.82 });
    if (riskType && destination) sankeyLinks.push({ source: `风险:${riskType}`, target: destination, value: value * 0.72 });
    if (destination) sankeyLinks.push({ source: destination, target: riskPath, value: value * 0.56 });
  });

  const destinationRows = topDestinations.map((item, index) => ({
    region: item.label,
    asn: destinationAsn(item.label, index),
    traffic: `${item.value.toFixed(1)}`,
    ratio: percentOf(item.value, totalUploadGb),
  }));

  return {
    sankeyNodes: uniqueSankeyNodes(sankeyNodes),
    sankeyLinks: mergeSankeyLinks(sankeyLinks),
    destinationRows,
    sensitiveTypes: normalizeDistribution(topTypes, ['#65d86e', '#ffb020', '#ff8a3d', '#ff4d4f', '#b685ff']),
    protocols: buildProtocolDistribution(sourceRows),
    trend: buildExfilTrend(sourceRows, totalUploadGb),
    accounts: topSources.map((item) => ({ label: serviceAccountLabel(item.label), value: Math.max(1, Math.round(item.value)) })),
    confidence,
    totalUploadGb,
    pathCount: Math.max(pathCount, sourceRows.length, sourceCount ? Math.round(sourceCount * 1.6) : 1),
  };
}

function buildExfilFallbackRows(): SnapshotRow[] {
  return [
    { 源资产: '10.14.2.35', 外传路径: '10.14.2.35 -> 203.0.113.45', 目标区域: '美国 (US)', 数据类型: '数据库备份', 上传量: '486 GB', 会话数: 326, 风险类型: '异常上传', 风险等级: '高危', 处置: '阻断' },
    { 源资产: '10.14.5.21', 外传路径: '10.14.5.21 -> 198.51.100.77', 目标区域: '日本 (JP)', 数据类型: '源代码/文档', 上传量: '312 GB', 会话数: 211, 风险类型: '跨境外联', 风险等级: '高危', 处置: '阻断' },
    { 源资产: '10.14.3.18', 外传路径: '10.14.3.18 -> 8.8.8.8', 目标区域: '新加坡 (SG)', 数据类型: '设计图纸/模型', 上传量: '218 GB', 会话数: 184, 风险类型: '云存储', 风险等级: '中危', 处置: '阻断' },
    { 源资产: '10.14.7.66', 外传路径: '10.14.7.66 -> 185.199.111.153', 目标区域: '德国 (DE)', 数据类型: '个人信息', 上传量: '176 GB', 会话数: 142, 风险类型: '未知 ASN', 风险等级: '中危', 处置: '阻断' },
    { 源资产: '10.14.9.10', 外传路径: '10.14.9.10 -> 104.16.24.34', 目标区域: '香港 (HK)', 数据类型: '财务数据', 上传量: '98 GB', 会话数: 98, 风险类型: '对象存储', 风险等级: '中危', 处置: '阻断' },
  ];
}

function groupRows(rows: SnapshotRow[], labelColumn: string, valueColumn: string) {
  const groups = new Map<string, number>();
  rows.forEach((row) => {
    const label = rowText(row, labelColumn) || '未知';
    groups.set(label, (groups.get(label) ?? 0) + rowNumber(row, valueColumn));
  });
  return [...groups.entries()]
    .map(([label, value]) => ({ label, value }))
    .filter((item) => item.value > 0)
    .sort((a, b) => b.value - a.value);
}

function rowText(row: SnapshotRow, column: string) {
  const value = row[column];
  return typeof value === 'number' ? String(value) : String(value ?? '').trim();
}

function rowNumber(row: SnapshotRow, column: string) {
  const value = row[column];
  if (typeof value === 'number') return value;
  return parseNumericValue(String(value ?? ''));
}

function rowUploadGb(row: SnapshotRow) {
  const value = row['上传量'];
  if (typeof value === 'number') return value;
  const raw = String(value ?? '');
  const numeric = parseNumericValue(raw);
  if (/tb/i.test(raw)) return numeric * 1024;
  if (/mb/i.test(raw)) return numeric / 1024;
  if (/kb/i.test(raw)) return numeric / (1024 * 1024);
  return numeric;
}

function parseNumericValue(value: string) {
  const match = value.replace(/,/g, '').match(/-?\d+(?:\.\d+)?/);
  return match ? Number(match[0]) : 0;
}

function metricNumber(metrics: SnapshotMetric[], label: string) {
  const metric = metrics.find((item) => item.label === label);
  return metric ? parseNumericValue(metric.value) : 0;
}

function metricValueNumber(metrics: SnapshotMetric[], label: string) {
  return metricNumber(metrics, label);
}

function firstMatchingGroup(value: string, groups: Array<{ label: string }>) {
  if (!groups.length) return '';
  return groups.find((item) => item.label === value)?.label ?? groups[0].label;
}

function uniqueSankeyNodes(nodes: ExfilSankeyNode[]) {
  const seen = new Set<string>();
  return nodes.filter((node) => {
    if (seen.has(node.name)) return false;
    seen.add(node.name);
    return true;
  });
}

function mergeSankeyLinks(links: ExfilSankeyLink[]) {
  const merged = new Map<string, ExfilSankeyLink>();
  links.forEach((link) => {
    const key = `${link.source}->${link.target}`;
    const previous = merged.get(key);
    merged.set(key, previous ? { ...previous, value: previous.value + link.value } : { ...link });
  });
  return [...merged.values()].map((link) => ({ ...link, value: Number(link.value.toFixed(2)) }));
}

function normalizeDistribution(groups: Array<{ label: string; value: number }>, colors: string[]): ExfilDistributionItem[] {
  const total = groups.reduce((sum, item) => sum + item.value, 0) || 1;
  return groups.slice(0, 5).map((item, index) => ({
    label: item.label,
    value: Number((item.value / total * 100).toFixed(1)),
    color: colors[index % colors.length],
  }));
}

function buildProtocolDistribution(rows: SnapshotRow[]): ExfilDistributionItem[] {
  const protocols = ['HTTPS', 'S3', 'WebDAV', 'DNS', '其他'];
  const total = rows.reduce((sum, row) => sum + rowUploadGb(row), 0) || 1;
  return protocols.map((label, index) => {
    const value = rows.reduce((sum, row, rowIndex) => {
      const path = rowText(row, '外传路径');
      const matched = path.toUpperCase().includes(label.toUpperCase()) || rowIndex % protocols.length === index;
      return matched ? sum + rowUploadGb(row) : sum;
    }, 0);
    return {
      label,
      value: Number((value / total * 100).toFixed(1)),
      color: ['#1ea8ff', '#ffb020', '#65d86e', '#ff8a3d', '#b685ff'][index],
    };
  }).filter((item) => item.value > 0);
}

function buildExfilTrend(rows: SnapshotRow[], totalUploadGb: number): ExfilTrendPoint[] {
  const base = totalUploadGb || 320;
  return Array.from({ length: 12 }, (_, index) => {
    const row = rows[index % rows.length];
    const pulse = row ? rowUploadGb(row) / Math.max(base, 1) : 0.08;
    const wave = 0.42 + Math.sin(index * 0.86) * 0.14 + (index % 4 === 2 ? 0.18 : 0);
    return {
      label: `${String(index * 2).padStart(2, '0')}:00`,
      value: Number(Math.max(2, base / 18 * (wave + pulse)).toFixed(1)),
    };
  });
}

function percentOf(value: number, total: number) {
  if (!total) return '0.0%';
  return `${(value / total * 100).toFixed(1)}%`;
}

function destinationAsn(label: string, index: number) {
  const known: Record<string, string> = {
    '美国 (US)': 'AS15169',
    '日本 (JP)': 'AS17676',
    '新加坡 (SG)': 'AS133481',
    '德国 (DE)': 'AS3320',
    '香港 (HK)': 'AS4760',
  };
  return known[label] ?? `AS${15169 + index * 817}`;
}

function serviceAccountLabel(source: string) {
  const labels = ['svc_backup', 'svc_share', 'user_test01', 'gitlab-ci', 'anonymous'];
  if (source.includes('NAS')) return 'svc_backup';
  if (source.includes('Share')) return 'svc_share';
  const tail = source.split(/[.-]/).filter(Boolean).slice(-1)[0];
  return tail && /\d+/.test(tail) ? `svc_${tail}` : labels[Math.abs(source.length) % labels.length];
}

function AptCanvas({
  config,
  rows,
  metrics,
  selectedSignal,
}: {
  config: TopicConfig;
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  selectedSignal: string;
}) {
  const [selectedNode, setSelectedNode] = useState('campaign-0');
  const model = buildAptVisualModel(rows, metrics, []);
  const nodes: TopicTopologyNode[] = [
    ...model.campaigns.map((item, index) => ({
      id: `campaign-${index}`,
      label: item.name,
      detail: `${item.meta} / 事件 ${item.events}`,
      tone: 'risk' as const,
      x: 9,
      y: 22 + index * 29,
      size: [112, 40] as [number, number],
    })),
    ...model.phases.map((item, index) => ({
      id: `phase-${item.id}`,
      label: `${index + 1} ${item.label}`,
      detail: `${item.value} / ${item.confidence}`,
      tone: item.tone === 'risk' ? 'risk' as const : item.tone === 'warn' ? 'proxy' as const : item.tone === 'ok' ? 'destination' as const : 'probe' as const,
      x: 38 + (index % 3) * 18,
      y: 22 + Math.floor(index / 3) * 29,
      size: [92, 36] as [number, number],
    })),
    ...model.evidenceNodes.map((item, index) => ({
      id: `evidence-${index}`,
      label: item.label,
      detail: item.value,
      tone: item.tone === 'risk' ? 'risk' as const : item.tone === 'warn' ? 'proxy' as const : 'destination' as const,
      x: 88,
      y: 17 + index * 15,
      size: [106, 36] as [number, number],
    })),
    ...model.assets.map((item, index) => ({
      id: `asset-${index}`,
      label: item.label,
      detail: item.value,
      tone: 'destination' as const,
      x: 30 + index * 19,
      y: 88,
      size: [106, 32] as [number, number],
    })),
  ].map((node) => ({ ...node, selected: node.id === selectedNode }));
  const links: TopicTopologyLink[] = [
    ...model.campaigns.flatMap((_, campaignIndex) => model.phases.slice(campaignIndex * 2, campaignIndex * 2 + 3).map((phase) => ({ source: `campaign-${campaignIndex}`, target: `phase-${phase.id}`, tone: 'purple' as const }))),
    ...model.phases.map((phase, index) => ({ source: `phase-${phase.id}`, target: `evidence-${index % model.evidenceNodes.length}`, tone: phase.tone === 'risk' ? 'risk' as const : 'warn' as const })),
    ...model.phases.slice(0, model.assets.length).map((phase, index) => ({ source: `phase-${phase.id}`, target: `asset-${index}`, tone: 'ok' as const })),
  ];
  return (
    <div className="taf-topic-canvas taf-topic-apt-canvas">
      <div className="taf-topic-canvas-legend">
        {['战役簇', '攻击阶段', '资产/账号', 'C2/外联', '证据节点'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <div className="taf-topic-attack-map taf-topic-apt-attack-map">
        <div className="taf-topic-apt-topology-svg">
          <TopicTopologyGraph ariaLabel="APT 战役攻击关系图" nodes={nodes} links={links} onNodeClick={setSelectedNode} />
        </div>
      </div>
    </div>
  );
}

function AptAnalysisDashboard({ rows, metrics, focusMode }: { rows: SnapshotRow[]; metrics: SnapshotMetric[]; focusMode: string }) {
  const model = buildAptVisualModel(rows, metrics, []);
  const analysisTabs = ['ATT&CK阶段覆盖', '战役耗时线', '关键 IoC 命中', '横向移动路径', '处置动作状态', '证据关联强度'];
  const [activeTab, setActiveTab] = useState(analysisTabs[0]);

  return (
    <WorkPanel
      title="战役分析"
      className="taf-topic-apt-analysis-panel"
      extra={<span className="taf-topic-focus">{focusMode}</span>}
    >
      <div className="taf-topic-apt-tabs" role="tablist" aria-label="APT 战役分析维度">
        {analysisTabs.map((item) => (
          <button
            key={item}
            type="button"
            className={activeTab === item ? 'is-active' : ''}
            aria-selected={activeTab === item}
            onClick={() => setActiveTab(item)}
          >
            {item}
          </button>
        ))}
      </div>
      <div className="taf-topic-apt-analysis-grid">
        <div className="taf-topic-apt-matrix" aria-label="ATT&CK 阶段覆盖矩阵">
          <b />
          {model.phases.map((phase) => <b key={phase.id}>{phase.id}<small>{phase.label}</small></b>)}
          {model.campaigns.map((campaign, rowIndex) => [
            <strong key={`${campaign.name}-name`}>{campaign.name}</strong>,
            ...model.phases.map((phase, phaseIndex) => (
              <span
                key={`${campaign.name}-${phase.id}`}
                className={`is-${(rowIndex + phaseIndex) % 5 === 1 ? 'warn' : (rowIndex + phaseIndex) % 6 === 2 ? 'risk' : 'ok'}`}
              />
            )),
          ])}
        </div>

        <div className="taf-topic-apt-trend" aria-label="战役时间线事件数">
          <header>
            <strong>战役时间线（事件数）</strong>
            <span>{model.eventTotal} 事件</span>
          </header>
          <DataQualityTrendChart
            ariaLabel="APT 战役事件趋势"
            className="taf-topic-apt-trend-echart"
            categories={model.timeline.map((point) => point.label)}
            series={[
              { name: 'APT-CN', color: '#ff5b3d', values: model.timeline.map((point) => point.aptCn), area: true },
              { name: 'TEMP.HAWK', color: '#ffb020', values: model.timeline.map((point) => point.tempHawk) },
              { name: 'UNKNOWN-07', color: '#65d86e', values: model.timeline.map((point) => point.unknown), dashed: true },
            ]}
          />
        </div>

        <div className="taf-topic-apt-ioc" aria-label="关键 IoC 命中 TOP5">
          <header>
            <strong>关键 IoC 命中 TOP5</strong>
            <span>复盘证据</span>
          </header>
          <div>
            <b title="IoC">IoC</b><b title="类型">类型</b><b title="命中次数">命中次数</b><b title="首次命中">首次命中</b>
            {model.iocs.map((item) => [
              <span key={`${item.ioc}-ioc`} title={item.ioc}>{item.ioc}</span>,
              <span key={`${item.ioc}-type`} title={item.type}>{item.type}</span>,
              <span key={`${item.ioc}-hits`} title={`${item.hits}`}>{item.hits}</span>,
              <span key={`${item.ioc}-time`} title={item.firstSeen}>{item.firstSeen}</span>,
            ])}
          </div>
        </div>

      </div>
    </WorkPanel>
  );
}

function AptResponsePanel({ rows, metrics }: { rows: SnapshotRow[]; metrics: SnapshotMetric[] }) {
  const model = buildAptVisualModel(rows, metrics, []);
  const total = model.response.reduce((sum, item) => sum + item.value, 0);
  return (
    <WorkPanel title="处置动作状态（近30天）" className="taf-topic-apt-response-panel" extra={<span className="taf-topic-focus">总计 {total}</span>}>
      <div className="taf-topic-apt-response" aria-label="处置动作状态">
        <div className="taf-topic-apt-response-chart">
          <DataQualityDonutChart
            ariaLabel="APT 处置动作状态分布"
            className="taf-topic-apt-response-echart"
            rows={model.response.map((item) => ({
              label: item.label,
              value: item.value,
              color: item.tone === 'risk' ? '#ff5b3d' : item.tone === 'warn' ? '#ffb020' : '#65d86e',
            }))}
          />
          <strong>{total}</strong>
          <span>总计</span>
        </div>
        <div>
          {model.response.map((item) => (
            <span key={item.label} className={`is-${item.tone}`}>
              <b>{item.label}</b>
              <em>{item.value} ({Math.round(item.value / Math.max(total, 1) * 100)}%)</em>
            </span>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function AptRightRail({
  config,
  metrics,
  evidenceRows,
  rows,
}: {
  config: TopicConfig;
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
  rows: SnapshotRow[];
}) {
  const model = buildAptVisualModel(rows, metrics, evidenceRows);
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出战役报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];
  const riskOpen = Math.max(3, Math.round((100 - model.closureRate) / 2));

  return (
    <>
      <WorkPanel title={`战役交付摘要 / ${config.topicCode}`} className="taf-topic-apt-delivery">
        <span className="taf-topic-apt-delivery-scope">(APT/战役专题)</span>
        <div className="taf-topic-exfil-delivery-grid">
          <div className="taf-topic-exfil-delivery-ring" style={{ '--value': model.reportConfidence } as CSSProperties}>
            <strong>{Math.round(model.reportConfidence)}%</strong>
            <span>较昨日 +8%</span>
          </div>
          <div className="taf-topic-exfil-delivery-stats">
            <span><i /><b>可生成报告</b><strong>{Math.max(7, model.campaigns.length + 4)}</strong></span>
            <span><i /><b>待补证据</b><strong>{model.evidenceRows.filter((item) => item.status === 'warn').length || 3}</strong></span>
            <span><i /><b>未闭环风险</b><strong>{riskOpen}</strong></span>
          </div>
        </div>
        <div className="taf-topic-exfil-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出总报告" className="ant-btn ant-btn-default ant-btn-sm"><DownloadOutlined />导出总报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm"><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / APT/战役专题" className="taf-topic-apt-evidence">
        <div className="taf-topic-exfil-evidence-list">
          {model.evidenceRows.slice(0, 6).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="战役报告预览 / 当前保存视图" className="taf-topic-apt-report">
        <div className="taf-topic-report-preview">
          <div className="taf-topic-report-sheet">
            <span />
            <span />
            <span />
            <i />
          </div>
          <div>
            <strong>{config.reportTitle}</strong>
            <span>时间窗：{config.timeRange.split('(')[0].trim()}</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：2026-06-20 03:40:12</span>
            <TopicActionButton topic={config.topicCode} title="预览报告" className="ant-btn ant-btn-link ant-btn-sm">预览报告</TopicActionButton>
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-apt-action-panel">
        <div className="taf-topic-exfil-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function AptEvidenceToolbar() {
  return (
    <div className="taf-topic-apt-table-toolbar" aria-label="APT 证据表筛选">
      {['视图：分层', '发现域：全部', '证据类型：全部', '处置状态：全部'].map((item) => (
        <span key={item} title={item}>{item}</span>
      ))}
    </div>
  );
}

function AptEvidenceTable({ rows, isLoading }: { rows: SnapshotRow[]; isLoading: boolean }) {
  const tableRows = buildAptEvidenceEventRows(rows);
  const pageSize = 5;
  const [page, setPage] = useState(1);
  const [selectedAction, setSelectedAction] = useState<{ action: string; row: AptEvidenceEventRow }>();
  const [submittedAction, setSubmittedAction] = useState(false);
  const pageCount = Math.max(1, Math.ceil(tableRows.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const visibleRows = tableRows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const columns: Array<[keyof AptEvidenceEventRow, string]> = [
    ['id', '事件ID'],
    ['phase', '阶段'],
    ['assetGroup', '资产组'],
    ['ioc', 'IoC'],
    ['evidenceType', '证据类型'],
    ['timeWindow', '时间窗'],
    ['status', '处置状态'],
  ];

  return (
    <div className="taf-topic-apt-evidence-table" aria-busy={isLoading} aria-label="战役关联事件与证据">
      {columns.map(([, label]) => <b key={label} title={label}>{label}</b>)}
      <b title="操作">操作</b>
      {isLoading ? (
        <span className="taf-topic-apt-table-loading">加载中...</span>
      ) : visibleRows.map((row) => (
        <div key={row.id} className="taf-topic-apt-table-row">
          {columns.map(([key]) => (
            <span
              key={`${row.id}-${String(key)}`}
              className={key === 'status' ? `is-${row.statusTone}` : undefined}
              title={String(row[key])}
            >
              {String(row[key])}
            </span>
          ))}
          <span className="taf-topic-apt-table-actions" title={row.actions.join(' / ')}>
            {row.actions.map((action) => (
              <button
                key={`${row.id}-${action}`}
                type="button"
                title={action}
                onClick={() => {
                  setSubmittedAction(false);
                  setSelectedAction({ action, row });
                }}
              >
                {action}
              </button>
            ))}
          </span>
        </div>
      ))}
      {!isLoading && (
        <div className="taf-topic-apt-table-footer">
          <span>共 {tableRows.length} 条</span>
          <button type="button" title="上一页" aria-label="APT 证据上一页" disabled={currentPage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>‹</button>
          {Array.from({ length: pageCount }, (_, index) => index + 1).map((value) => (
            <button
              key={value}
              type="button"
              className={currentPage === value ? 'is-active' : ''}
              title={`第 ${value} 页`}
              aria-current={currentPage === value ? 'page' : undefined}
              onClick={() => setPage(value)}
            >
              {value}
            </button>
          ))}
          <button type="button" title="下一页" aria-label="APT 证据下一页" disabled={currentPage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>›</button>
          <span>{pageSize} 条/页</span>
        </div>
      )}
      <Drawer
        className="taf-topic-action-drawer"
        title={selectedAction ? `${selectedAction.action}确认` : 'APT 证据操作'}
        open={Boolean(selectedAction)}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onClose={() => {
          setSelectedAction(undefined);
          setSubmittedAction(false);
        }}
        extra={<Button size="small" type="primary" disabled={submittedAction} onClick={() => setSubmittedAction(true)}>{submittedAction ? '已写入任务队列' : '确认提交'}</Button>}
      >
        <div className="taf-alert-detail-action-body">
          <p>将为当前 APT 专题事件创建“{selectedAction?.action}”仿真任务，并保留对象、动作和审计上下文。</p>
          <dl>
            <dt>事件 ID</dt><dd>{selectedAction?.row.id}</dd>
            <dt>IoC</dt><dd>{selectedAction?.row.ioc}</dd>
            <dt>接口预留</dt><dd>/v1/topics/apt/evidence-actions</dd>
          </dl>
          {submittedAction && <Alert type="success" showIcon message="APT 证据操作已进入仿真任务队列" description={`事件 ${selectedAction?.row.id}；动作 ${selectedAction?.action}`} />}
        </div>
      </Drawer>
    </div>
  );
}

function buildAptVisualModel(rows: SnapshotRow[], metrics: SnapshotMetric[], evidenceRows: PageSnapshot['evidence']): AptVisualModel {
  if (isVisualBreakdownMode()) return buildAptTargetVisualModel();

  const sourceRows = rows.length ? rows : buildAptFallbackRows();
  const campaignNames = sourceRows.slice(0, 3).map((row, index) => rowText(row, '战役名称') || ['APT-CN-2026', 'TEMP.HAWK', 'UNKNOWN-07'][index]);
  const alertTotal = metricNumber(metrics, '关联告警') || sourceRows.reduce((sum, row) => sum + rowNumber(row, '关联告警'), 0) || 156;
  const eventTotal = Math.max(64, Math.round(alertTotal * 0.14));
  const campaigns = campaignNames.map((name, index) => ({
    name,
    meta: index === 0 ? '高置信' : index === 1 ? '中置信' : '低置信',
    events: Math.max(12, Math.round(eventTotal / (index + 1.6))),
    tone: (index === 0 ? 'risk' : index === 1 ? 'warn' : 'info') as Tone,
  }));
  const labels = ['初始访问', '执行', '持久化', '防御规避', '凭证访问', '发现', '横向移动', '命令控制', '数据外传'];
  const phases = labels.map((label, index) => ({
    id: `TA${String(index + 1).padStart(4, '0')}`,
    label,
    value: phaseValue(label, sourceRows, metrics, index),
    confidence: index % 3 === 0 ? '高置信' : index % 3 === 1 ? '中覆盖' : '低覆盖',
    tone: (index % 4 === 0 ? 'risk' : index % 4 === 1 ? 'warn' : index % 4 === 2 ? 'info' : 'ok') as Tone,
  }));
  const evidenceNodes = [
    { label: 'C2 域名', value: iocValue(sourceRows, 0, 'c2-apt.ltop'), tone: 'risk' as Tone },
    { label: 'C2 IP', value: iocValue(sourceRows, 1, '185.199.111.153'), tone: 'risk' as Tone },
    { label: '外联地址', value: iocValue(sourceRows, 2, '195.110.10.77'), tone: 'warn' as Tone },
    { label: 'PCAP', value: `${Math.max(56, Math.round(eventTotal * 0.36))} 证据`, tone: 'warn' as Tone },
    { label: 'Session', value: `${Math.max(72, Math.round(eventTotal * 0.46))} 会话`, tone: 'ok' as Tone },
  ];
  const assets = [
    { label: '资产/组', value: `办公终端 命中 ${Math.max(32, metricNumber(metrics, '关键资产命中'))}`, tone: 'ok' as Tone },
    { label: '账号', value: `CORP.LOCAL 命中 ${Math.max(27, Math.round(eventTotal * 0.17))}`, tone: 'ok' as Tone },
    { label: '资产/后门', value: `PowerShell 命中 ${Math.max(18, metricNumber(metrics, '持久化迹象数'))}`, tone: 'ok' as Tone },
    { label: '关键班弱码', value: `弱特征服务 命中 ${Math.max(14, Math.round(eventTotal * 0.09))}`, tone: 'ok' as Tone },
  ];
  const reportConfidence = metricNumber(metrics, '报告置信度') || 62;
  const closureRate = metricNumber(metrics, '处置闭环率') || 68;
  const normalizedEvidenceRows = evidenceRows.length ? evidenceRows : [
    { label: '告警证据', value: '64 / 64 (100%)', status: 'ok' as const },
    { label: 'PCAP', value: '132 / 156 (84%)', status: 'warn' as const },
    { label: 'Session', value: '198 / 204 (97%)', status: 'ok' as const },
    { label: '审计日志', value: '38 / 38 (100%)', status: 'ok' as const },
    { label: '回溯路径', value: '18 / 18 (100%)', status: 'ok' as const },
    { label: '资产快照', value: '23 / 23 (100%)', status: 'ok' as const },
  ];

  return {
    campaigns,
    phases,
    evidenceNodes,
    assets,
    timeline: buildAptTimeline(sourceRows, eventTotal),
    iocs: evidenceNodes.slice(0, 5).map((item, index) => ({
      ioc: item.value.replace(/\s+(证据|会话)$/u, ''),
      type: index === 0 ? '域名' : index === 1 ? 'IP' : index === 4 ? '会话' : 'Hash',
      hits: Math.max(18, Math.round(eventTotal / (index + 2))),
      firstSeen: `06-${String(23 + index).padStart(2, '0')} ${String(10 + index).padStart(2, '0')}:21`,
    })),
    response: [
      { label: '已完成', value: Math.round(closureRate), tone: 'ok' as Tone },
      { label: '进行中', value: Math.max(12, Math.round((100 - closureRate) * 0.56)), tone: 'warn' as Tone },
      { label: '待处置', value: Math.max(8, Math.round((100 - closureRate) * 0.44)), tone: 'risk' as Tone },
    ],
    evidenceRows: normalizedEvidenceRows,
    reportConfidence,
    closureRate,
    eventTotal,
  };
}

function buildAptTargetVisualModel(): AptVisualModel {
  return {
    campaigns: [
      { name: 'APT-CN-2026', meta: '高置信', events: 156, tone: 'risk' },
      { name: 'TEMP.HAWK', meta: '中置信', events: 98, tone: 'warn' },
      { name: 'UNKNOWN-07', meta: '低置信', events: 64, tone: 'info' },
    ],
    phases: [
      { id: 'TA0001', label: '初始访问', value: 7, confidence: '高置信', tone: 'risk' },
      { id: 'TA0002', label: '执行', value: 7, confidence: '高置信', tone: 'warn' },
      { id: 'TA0003', label: '持久化', value: 6, confidence: '中覆盖', tone: 'info' },
      { id: 'TA0004', label: '防御规避', value: 6, confidence: '中覆盖', tone: 'ok' },
      { id: 'TA0005', label: '凭证访问', value: 5, confidence: '中覆盖', tone: 'risk' },
      { id: 'TA0007', label: '发现', value: 6, confidence: '中覆盖', tone: 'warn' },
      { id: 'TA0008', label: '横向移动', value: 23, confidence: '链路', tone: 'risk' },
      { id: 'TA0011', label: '命令控制', value: 8, confidence: '命中', tone: 'warn' },
      { id: 'TA0010', label: '数据外传', value: 32, confidence: '证据', tone: 'risk' },
    ],
    evidenceNodes: [
      { label: 'C2 域名', value: 'c2-apt.ltop 命中 8', tone: 'risk' },
      { label: 'C2 IP', value: '185.199.111.153 命中 6', tone: 'risk' },
      { label: '外联地址', value: '195.110.10.77 命中 5', tone: 'warn' },
      { label: 'PCAP', value: '56 证据', tone: 'warn' },
      { label: 'Session', value: '72 会话', tone: 'ok' },
      { label: '日志/审计', value: '134 条', tone: 'ok' },
    ],
    assets: [
      { label: '资产/组', value: '办公终端 命中 32', tone: 'ok' },
      { label: '账号', value: 'CORP.LOCAL 命中 27', tone: 'ok' },
      { label: '资产/后门', value: 'PowerShell 命中 18', tone: 'ok' },
      { label: '关键班弱码', value: '弱特征服务 命中 14', tone: 'ok' },
    ],
    timeline: [
      { label: '05-21', aptCn: 12, tempHawk: 8, unknown: 5 },
      { label: '05-26', aptCn: 22, tempHawk: 13, unknown: 7 },
      { label: '05-31', aptCn: 38, tempHawk: 18, unknown: 11 },
      { label: '06-05', aptCn: 46, tempHawk: 27, unknown: 14 },
      { label: '06-10', aptCn: 64, tempHawk: 31, unknown: 20 },
      { label: '06-15', aptCn: 58, tempHawk: 39, unknown: 24 },
      { label: '06-20', aptCn: 78, tempHawk: 46, unknown: 28 },
    ],
    iocs: [
      { ioc: 'c2-apt.ltop', type: '域名', hits: 32, firstSeen: '05-23 10:21' },
      { ioc: '195.110.10.77', type: 'IP', hits: 28, firstSeen: '05-24 14:11' },
      { ioc: '185.199.111.153', type: 'IP', hits: 21, firstSeen: '05-25 09:33' },
      { ioc: 'updatel.javroc-dol.com', type: '域名', hits: 18, firstSeen: '06-02 11:07' },
      { ioc: 'a1b2c3d4e5f6a7b8', type: 'Hash', hits: 18, firstSeen: '06-03 16:42' },
    ],
    response: [
      { label: '已完成', value: 68, tone: 'ok' },
      { label: '进行中', value: 18, tone: 'warn' },
      { label: '待处置', value: 14, tone: 'risk' },
    ],
    evidenceRows: [
      { label: '告警证据', value: '64 / 64 (100%)', status: 'ok' },
      { label: 'PCAP', value: '132 / 156 (84%)', status: 'warn' },
      { label: 'Session', value: '198 / 204 (97%)', status: 'ok' },
      { label: '审计日志', value: '38 / 38 (100%)', status: 'ok' },
      { label: '回溯路径', value: '18 / 18 (100%)', status: 'ok' },
      { label: '资产快照', value: '23 / 23 (100%)', status: 'ok' },
    ],
    reportConfidence: 62,
    closureRate: 68,
    eventTotal: 180,
  };
}

function buildAptEvidenceEventRows(rows: SnapshotRow[]): AptEvidenceEventRow[] {
  if (isVisualBreakdownMode()) return buildAptTargetEvidenceRows();

  const directRows = rows
    .map((row, index) => {
      const id = rowText(row, '事件ID');
      if (!id) return null;
      const status = rowText(row, '处置状态') || '进行中';
      return {
        id,
        phase: rowText(row, '阶段') || '初始访问',
        assetGroup: rowText(row, '资产组') || rowText(row, '关键实体') || '办公终端',
        ioc: rowText(row, 'IoC') || rowText(row, '关键实体') || 'c2-apt.ltop',
        evidenceType: rowText(row, '证据类型') || 'Session',
        timeWindow: rowText(row, '时间窗') || rowText(row, '最近活动') || `2026-06-${String(index + 1).padStart(2, '0')} 10:00 ~ 10:30`,
        status,
        statusTone: aptStatusTone(status),
        actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
      } satisfies AptEvidenceEventRow;
    })
    .filter((row): row is AptEvidenceEventRow => Boolean(row));

  return directRows.length ? directRows : buildAptTargetEvidenceRows();
}

function buildAptTargetEvidenceRows(): AptEvidenceEventRow[] {
  return [
    {
      id: '20260601-0001',
      phase: '初始访问',
      assetGroup: '办公终端',
      ioc: 'c2-apt.ltop',
      evidenceType: 'PCAP',
      timeWindow: '2026-05-23 10:21 ~ 10:42',
      status: '已完成',
      statusTone: 'ok',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260602-0005',
      phase: '执行',
      assetGroup: '办公终端',
      ioc: 'PowerShell - Encoded',
      evidenceType: 'Session',
      timeWindow: '2026-05-23 10:43 ~ 11:05',
      status: '已完成',
      statusTone: 'ok',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260603-0012',
      phase: '持久化',
      assetGroup: '办公终端',
      ioc: 'a1b2c3d4e5f6a7b8',
      evidenceType: '文件摘要',
      timeWindow: '2026-05-24 14:11 ~ 14:35',
      status: '进行中',
      statusTone: 'warn',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260604-0020',
      phase: '防御规避',
      assetGroup: '办公网段',
      ioc: 'regsvr32.exe',
      evidenceType: '日志',
      timeWindow: '2026-05-25 09:22 ~ 09:35',
      status: '已完成',
      statusTone: 'ok',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260610-0051',
      phase: '凭证访问',
      assetGroup: '数据中心',
      ioc: '10.12.3.55 > 10.12.5.21',
      evidenceType: 'Session',
      timeWindow: '2026-05-30 21:03 ~ 21:28',
      status: '进行中',
      statusTone: 'warn',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260612-0068',
      phase: '命令控制',
      assetGroup: '数据中心',
      ioc: '185.199.111.153',
      evidenceType: 'PCAP',
      timeWindow: '2026-06-02 11:07 ~ 11:29',
      status: '已完成',
      statusTone: 'ok',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
    {
      id: '20260615-0079',
      phase: '数据外传',
      assetGroup: '数据中心',
      ioc: '195.110.10.77',
      evidenceType: '日志/审计',
      timeWindow: '2026-06-18 02:18 ~ 02:46',
      status: '待处置',
      statusTone: 'risk',
      actions: ['全量详情', '溯源分析', 'PCAP', 'Session', '关联告警', '停止BGP'],
    },
  ];
}

function aptStatusTone(status: string): Tone {
  if (status.includes('完成')) return 'ok';
  if (status.includes('待') || status.includes('未')) return 'risk';
  return 'warn';
}

function buildAptFallbackRows(): SnapshotRow[] {
  return [
    { 战役名称: 'APT-CN-2026', 阶段: '初始访问', 关键实体: 'WEB-SRV-02', 关联告警: 234, 攻击技术: 'T1190', 首次发现: '06-19 09:12', 最近活动: '06-20 03:22', 风险等级: '高风险', 处置: '下钻' },
    { 战役名称: 'TEMP.HAWK', 阶段: '执行活动', 关键实体: '域控 DC-01', 关联告警: 187, 攻击技术: 'T1059', 首次发现: '06-18 09:12', 最近活动: '06-19 03:22', 风险等级: '高风险', 处置: '下钻' },
    { 战役名称: 'UNKNOWN-07', 阶段: '横向移动', 关键实体: '财务-SRV-2003', 关联告警: 156, 攻击技术: 'T1021', 首次发现: '06-17 09:12', 最近活动: '06-18 03:22', 风险等级: '中风险', 处置: '下钻' },
  ];
}

function phaseValue(label: string, rows: SnapshotRow[], metrics: SnapshotMetric[], index: number) {
  const byRow = rows.filter((row) => rowText(row, '阶段').includes(label.slice(0, 2))).length;
  if (byRow) return byRow + 4;
  if (label === '横向移动') return metricNumber(metrics, '横向移动链路') || 23;
  if (label === '持久化') return metricNumber(metrics, '持久化迹象数') || 18;
  if (label === '数据外传') return metricNumber(metrics, '外传关联证据') || 32;
  return [7, 7, 6, 6, 5, 6, 23, 8, 32][index] ?? 6;
}

function iocValue(rows: SnapshotRow[], index: number, fallback: string) {
  const row = rows[index % rows.length] ?? {};
  const entity = rowText(row, '关键实体');
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(entity)) return entity;
  if (index === 0 && entity.includes('.')) return entity;
  return fallback;
}

function buildAptTimeline(rows: SnapshotRow[], eventTotal: number): AptTimelinePoint[] {
  const labels = ['05-21', '05-26', '05-31', '06-05', '06-10', '06-15', '06-20'];
  return labels.map((label, index) => {
    const seed = rows[index % rows.length] ? rowNumber(rows[index % rows.length], '关联告警') : eventTotal;
    return {
      label,
      aptCn: Math.max(8, Math.round(seed / 4 + Math.sin(index * 0.9) * 12)),
      tempHawk: Math.max(6, Math.round(seed / 6 + Math.cos(index * 0.7) * 8)),
      unknown: Math.max(3, Math.round(seed / 10 + Math.sin(index * 1.2) * 5)),
    };
  });
}

function FlowNode({ tone, title, detail }: { tone: string; title: string; detail: string }) {
  return (
    <span className={`taf-topic-flow-node is-${tone}`}>
      <strong>{title}</strong>
      <em>{detail}</em>
    </span>
  );
}

function renderTopicCell(column: string, value: unknown) {
  if (column.includes('风险')) return <StatusTag value={value} />;
  if (column === '处置') return <TopicActionButton topic="专题下钻" title="下钻" className="ant-btn ant-btn-link ant-btn-sm">下钻</TopicActionButton>;
  if (column.includes('流量') || column.includes('上传量') || column.includes('告警')) {
    return <strong className="taf-topic-strong-cell">{String(value ?? '-')}</strong>;
  }
  if (column.includes('源') || column.includes('实体') || column.includes('会话')) {
    return <span className="taf-topic-entity-cell"><GlobalOutlined />{String(value ?? '-')}</span>;
  }
  return String(value ?? '-');
}

function fallbackMetric(label: string): PageSnapshot['metrics'][number] {
  return { label, value: label.includes('完整') ? '0.0%' : '0', delta: '等待 API', status: 'info' };
}
