import {
  AimOutlined,
  ArrowRightOutlined,
  CheckCircleOutlined,
  ClusterOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  DotChartOutlined,
  ExpandOutlined,
  FileProtectOutlined,
  FundProjectionScreenOutlined,
  HddOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SafetyOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Descriptions, Modal, Tag } from 'antd';
import type { CSSProperties, ReactNode } from 'react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AbnormalImpactPieChart,
  CampaignDensityChart,
  EvidenceClosureRingChart,
  SparklineChart,
  WorldActivityMap,
  type CampaignDensityPoint,
  type WorldActivityFlow,
  type WorldActivityPoint,
} from '@/components/charts';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { ScreenAbnormalLink, ScreenEvidenceRing, ScreenVisualEdge, ScreenVisualNode, ScreenVisuals } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type Tone = 'ok' | 'warn' | 'info' | 'risk';
type TopologyMode = '2d' | '3d';
type PipelineNode = {
  label: string;
  metricLabel: string;
  value: string;
  noteLabel: string;
  note: string;
  icon: ReactNode;
  href: string;
  trend: number[];
  tone: Tone;
  trendTone?: Tone;
};

type TopologyNode = {
  id: string;
  label: string;
  meta: string;
  type: string;
  x: number;
  y: number;
  tone: Tone;
  probes: string;
  links: string;
  assets: string;
  riskScore: number;
  bandwidth: string;
  href: string;
};

type ProbeMapNode = {
  id: string;
  label: string;
  x: number;
  y: number;
  status: 'online' | 'offline' | 'maintenance';
};

type TopologyEdge = {
  from: string;
  to: string;
  tone: 'core' | 'converge' | 'risk';
  width?: number;
};

const pipeline: PipelineNode[] = [
  { label: '探针采集', metricLabel: '在线累计', value: '24 / 25', noteLabel: '采集带宽', note: '78.3 Gbps', icon: <AimOutlined />, href: '/probes', trend: [34, 42, 37, 45, 44, 52, 49, 57], tone: 'ok' },
  { label: '协议解析', metricLabel: '协议识别', value: '58 种', noteLabel: '解析成功率', note: '99.2%', icon: <NodeIndexOutlined />, href: '/data-quality', trend: [24, 28, 31, 30, 36, 40, 39, 45], tone: 'ok', trendTone: 'info' },
  { label: '归一化', metricLabel: '流量标准化', value: '98.1%', noteLabel: '规范化率', note: '98.1%', icon: <DeploymentUnitOutlined />, href: '/data-quality', trend: [42, 39, 44, 47, 46, 51, 49, 55], tone: 'ok' },
  { label: 'Kafka 集群', metricLabel: '分区', value: '48 / 48', noteLabel: '积压', note: '12,324', icon: <ClusterOutlined />, href: '/data-quality', trend: [28, 30, 34, 41, 36, 39, 42, 40], tone: 'warn' },
  { label: 'Flink 处理', metricLabel: '任务', value: '58', noteLabel: '处理延迟', note: '1.2 s', icon: <ThunderboltOutlined />, href: '/data-quality', trend: [32, 34, 31, 38, 36, 42, 45, 43], tone: 'ok' },
  { label: 'ClickHouse', metricLabel: '写入', value: '78.3 Gbps', noteLabel: '查询延迟', note: '0.4 s', icon: <DatabaseOutlined />, href: '/data-quality', trend: [38, 40, 43, 45, 44, 48, 49, 52], tone: 'ok' },
  { label: 'OpenSearch', metricLabel: '写入', value: '12.6 K EPS', noteLabel: '查询延迟', note: '0.6 s', icon: <FileProtectOutlined />, href: '/forensics', trend: [22, 27, 31, 29, 35, 33, 40, 44], tone: 'ok' },
  { label: 'NebulaGraph', metricLabel: '图谱更新', value: '2.1 K/s', noteLabel: '图谱状态', note: '正常', icon: <FundProjectionScreenOutlined />, href: '/graph', trend: [20, 25, 27, 35, 34, 39, 43, 48], tone: 'ok' },
  { label: 'MinIO 存储', metricLabel: '存储使用', value: '72.4 TB', noteLabel: '容量', note: '120 TB', icon: <HddOutlined />, href: '/forensics', trend: [18, 24, 30, 36, 41, 44, 48, 51], tone: 'ok' },
];

const topologyNodes: TopologyNode[] = [
  { id: 'teach-a', label: '教学楼A', meta: '探针在线', type: '教学区', x: 25, y: 32, tone: 'ok', probes: '3 / 3', links: '5 条', assets: '418', riskScore: 18, bandwidth: '8.6 Gbps', href: '/assets' },
  { id: 'teach-b', label: '教学楼B', meta: '汇聚正常', type: '教学区', x: 37, y: 25, tone: 'ok', probes: '4 / 4', links: '6 条', assets: '512', riskScore: 22, bandwidth: '9.8 Gbps', href: '/assets' },
  { id: 'library', label: '图书馆', meta: '核心链路', type: '公共区', x: 49, y: 18, tone: 'info', probes: '2 / 2', links: '8 条', assets: '236', riskScore: 31, bandwidth: '7.2 Gbps', href: '/graph' },
  { id: 'lab', label: '实验楼群', meta: '维护中', type: '实验区', x: 68, y: 31, tone: 'warn', probes: '5 / 6', links: '7 条', assets: '684', riskScore: 64, bandwidth: '16.4 Gbps', href: '/alerts' },
  { id: 'soc', label: '安全运营中心', meta: '高风险区', type: '核心运营', x: 82, y: 39, tone: 'risk', probes: '2 / 3', links: '9 条', assets: '124', riskScore: 87, bandwidth: '11.7 Gbps', href: '/alerts' },
  { id: 'dc', label: '数据中心', meta: '入库正常', type: '数据底座', x: 60, y: 63, tone: 'info', probes: '4 / 4', links: '11 条', assets: '196', riskScore: 38, bandwidth: '78.3 Gbps', href: '/data-quality' },
  { id: 'dorm', label: '宿舍区', meta: '在线', type: '生活区', x: 77, y: 70, tone: 'ok', probes: '3 / 3', links: '5 条', assets: '1,286', riskScore: 42, bandwidth: '14.2 Gbps', href: '/assets' },
  { id: 'admin', label: '行政楼', meta: '在线', type: '办公区', x: 34, y: 69, tone: 'ok', probes: '2 / 2', links: '4 条', assets: '211', riskScore: 27, bandwidth: '5.4 Gbps', href: '/assets' },
  { id: 'canteen', label: '食堂', meta: '汇聚正常', type: '生活服务', x: 20, y: 61, tone: 'info', probes: '2 / 2', links: '3 条', assets: '96', riskScore: 21, bandwidth: '2.8 Gbps', href: '/graph' },
  { id: 'stadium', label: '体育馆', meta: '带宽 57%', type: '活动场馆', x: 74, y: 56, tone: 'warn', probes: '1 / 2', links: '4 条', assets: '162', riskScore: 58, bandwidth: '6.9 Gbps', href: '/data-quality' },
];

const topologyEdges: TopologyEdge[] = [
  { from: 'core', to: 'teach-a', tone: 'core', width: 3 },
  { from: 'core', to: 'teach-b', tone: 'core', width: 2.6 },
  { from: 'core', to: 'library', tone: 'core', width: 2.8 },
  { from: 'core', to: 'dc', tone: 'core', width: 3 },
  { from: 'core', to: 'lab', tone: 'risk', width: 2.6 },
  { from: 'core', to: 'dorm', tone: 'converge', width: 2.2 },
  { from: 'dc', to: 'stadium', tone: 'risk', width: 2 },
  { from: 'dc', to: 'admin', tone: 'converge', width: 1.8 },
  { from: 'teach-a', to: 'canteen', tone: 'converge', width: 1.8 },
  { from: 'soc', to: 'lab', tone: 'risk', width: 2 },
  { from: 'teach-b', to: 'admin', tone: 'converge', width: 1.6 },
  { from: 'library', to: 'teach-b', tone: 'converge', width: 1.6 },
];

const probeMapNodes: ProbeMapNode[] = [
  { id: 'core', label: '核心区', x: 132, y: 118, status: 'online' },
  { id: 'teach-a', label: '教学楼A', x: 84, y: 92, status: 'online' },
  { id: 'teach-b', label: '教学楼B', x: 108, y: 66, status: 'online' },
  { id: 'library', label: '图书馆', x: 155, y: 74, status: 'online' },
  { id: 'lab', label: '实验楼', x: 178, y: 98, status: 'maintenance' },
  { id: 'dc', label: '数据中心', x: 148, y: 143, status: 'online' },
  { id: 'office', label: '办公区', x: 96, y: 142, status: 'online' },
  { id: 'canteen', label: '食堂', x: 64, y: 131, status: 'online' },
  { id: 'dorm', label: '宿舍区', x: 186, y: 158, status: 'online' },
  { id: 'stadium', label: '体育馆', x: 205, y: 130, status: 'online' },
  { id: 'soc', label: '安全运营', x: 202, y: 190, status: 'offline' },
  { id: 'edge', label: '边界', x: 54, y: 176, status: 'online' },
];

const probeMapLinks: Array<[string, string]> = [
  ['core', 'teach-a'],
  ['core', 'teach-b'],
  ['core', 'library'],
  ['core', 'lab'],
  ['core', 'dc'],
  ['core', 'office'],
  ['dc', 'dorm'],
  ['dc', 'stadium'],
  ['office', 'canteen'],
  ['office', 'edge'],
  ['dorm', 'soc'],
  ['teach-a', 'canteen'],
];

const evidenceRings: ScreenEvidenceRing[] = [
  { label: 'PCAP 覆盖率', value: 98.6, caption: '覆盖流量 78.3Gbps', href: '/forensics', level: 'low' },
  { label: 'Session 还原率', value: 95.7, caption: '还原会话 1.23M', href: '/forensics', level: 'low' },
  { label: '日志关联率', value: 93.2, caption: '关联日志 246.5M', href: '/audit-log', level: 'medium' },
  { label: '对象存储归档率', value: 99.1, caption: '归档 72.4TB', href: '/forensics', level: 'low' },
  { label: 'hash 校验通过率', value: 99.8, caption: '校验文件 18.6M', href: '/compliance', level: 'low' },
  { label: '签名 URL 可用率', value: 99.6, caption: '可用链接 12,645', href: '/forensics', level: 'low' },
];

const attackStages = [
  { id: 'recon', label: '侦察', value: 2186, pct: 12, tone: 'danger', detail: '端口扫描与弱服务枚举集中在实验区边界。' },
  { id: 'exploit', label: '资源利用', value: 3276, pct: 18, tone: 'danger', detail: 'Web 服务与 VPN 暴露资产存在异常利用链。' },
  { id: 'initial', label: '初始访问', value: 4932, pct: 27, tone: 'danger', detail: '钓鱼回连和异常登录形成最高热度阶段。' },
  { id: 'exec', label: '执行', value: 3184, pct: 17, tone: 'warn', detail: '脚本执行与横向测试行为正在被剧本阻断。' },
  { id: 'credential', label: '凭证访问', value: 2104, pct: 11, tone: 'ok', detail: '凭证访问已压低，仍需关注宿舍区异常会话。' },
  { id: 'impact', label: '影响达成', value: 2536, pct: 15, tone: 'ok', detail: '核心数据区未出现持续影响达成迹象。' },
];

const abnormalLinks: ScreenAbnormalLink[] = [
  { name: '实验区 - 核心区', linkCount: 1286, assetCount: 432, level: 'high' },
  { name: '宿舍区 - 核心区', linkCount: 923, assetCount: 311, level: 'medium' },
  { name: '办公区 - 核心区', linkCount: 612, assetCount: 207, level: 'medium' },
  { name: '教学区 - 图书馆', linkCount: 484, assetCount: 162, level: 'low' },
  { name: '生活区 - 核心区', linkCount: 371, assetCount: 128, level: 'low' },
];

const responseStats = [
  { label: '隔离动作数', value: '68', icon: <SafetyOutlined />, href: '/playbooks' },
  { label: '阻断动作数', value: '128', icon: <AimOutlined />, href: '/playbooks' },
  { label: '封禁动作数', value: '46', icon: <CheckCircleOutlined />, href: '/playbooks' },
  { label: '下发脚本数', value: '32', icon: <DeploymentUnitOutlined />, href: '/deployments' },
  { label: '反馈标注数', value: '236', icon: <DotChartOutlined />, href: '/mlops' },
];

const runtimeStats = [
  ['大屏刷新间隔', '5 s'],
  ['拓扑渲染延迟', '286 ms'],
  ['链路带宽水位', '57%'],
  ['流向动画帧率', '28 FPS'],
];

const riskMapPoints: WorldActivityPoint[] = [
  { name: '北美异常入口', coord: [206, 206], value: 28, level: 'high' },
  { name: '东海岸扫描簇', coord: [282, 224], value: 18, level: 'medium' },
  { name: '欧洲战役节点', coord: [487, 178], value: 23, level: 'high' },
  { name: '北非中转', coord: [522, 285], value: 16, level: 'medium' },
  { name: '西亚代理池', coord: [606, 236], value: 24, level: 'high' },
  { name: '东亚回连', coord: [742, 213], value: 21, level: 'high' },
  { name: '东南亚跳板', coord: [788, 270], value: 14, level: 'medium' },
  { name: '南美低频', coord: [312, 338], value: 9, level: 'low' },
];

const egressMapPoints: WorldActivityPoint[] = [
  { name: '北美洲', coord: [220, 210], value: 42.7, level: 'high' },
  { name: '欧洲', coord: [492, 166], value: 18.6, level: 'medium' },
  { name: '东南亚', coord: [690, 282], value: 31.2, level: 'high' },
  { name: '东亚', coord: [763, 206], value: 12.9, level: 'medium' },
  { name: '澳洲', coord: [842, 356], value: 6.3, level: 'low' },
  { name: '非洲', coord: [535, 306], value: 9.8, level: 'medium' },
];

const egressMapFlows: WorldActivityFlow[] = [
  { name: '园区 -> 北美洲', from: [630, 235], to: [220, 210], value: 42.7, level: 'high' },
  { name: '园区 -> 东南亚', from: [630, 235], to: [690, 282], value: 31.2, level: 'high' },
  { name: '园区 -> 欧洲', from: [630, 235], to: [492, 166], value: 18.6, level: 'medium' },
  { name: '园区 -> 东亚', from: [630, 235], to: [763, 206], value: 12.9, level: 'medium' },
  { name: '园区 -> 澳洲', from: [630, 235], to: [842, 356], value: 6.3, level: 'low' },
  { name: '园区 -> 非洲', from: [630, 235], to: [535, 306], value: 9.8, level: 'medium' },
  { name: '欧洲 -> 北美洲', from: [492, 166], to: [220, 210], value: 14.2, level: 'medium' },
  { name: '欧洲 -> 东亚', from: [492, 166], to: [763, 206], value: 11.6, level: 'medium' },
  { name: '东亚 -> 澳洲', from: [763, 206], to: [842, 356], value: 7.5, level: 'low' },
  { name: '非洲 -> 欧洲', from: [535, 306], to: [492, 166], value: 8.8, level: 'medium' },
  { name: '北美洲 -> 欧洲', from: [220, 210], to: [492, 166], value: 18.4, level: 'high' },
];

const campaignDensityPoints: CampaignDensityPoint[] = [
  { name: '钓鱼回连簇', x: 52, y: 48, value: 47, level: 'high' },
  { name: 'VPN 弱口令簇', x: 42, y: 36, value: 35, level: 'high' },
  { name: '实验区扫描簇', x: 60, y: 31, value: 28, level: 'medium' },
  { name: '凭证访问簇', x: 36, y: 58, value: 22, level: 'medium' },
  { name: '脚本执行簇', x: 55, y: 66, value: 26, level: 'medium' },
  { name: '边界代理簇', x: 70, y: 48, value: 19, level: 'low' },
  { name: '横向探测簇', x: 31, y: 45, value: 18, level: 'low' },
  { name: '数据打包簇', x: 67, y: 70, value: 24, level: 'medium' },
  { name: '异常登录簇', x: 46, y: 72, value: 17, level: 'low' },
  { name: '外联跳板簇', x: 75, y: 58, value: 31, level: 'high' },
  { name: '办公区噪声簇', x: 28, y: 67, value: 13, level: 'low' },
  { name: '宿舍区风险簇', x: 61, y: 82, value: 20, level: 'medium' },
];

const fallbackScreenVisuals: ScreenVisuals = {
  probeMapNodes,
  probeMapLinks,
  topologyNodes,
  topologyEdges,
  campaignDensityPoints,
  riskMapPoints,
  egressMapPoints,
  egressMapFlows,
  abnormalLinks,
  evidenceRings,
};

const normalizeTone = (tone?: ScreenVisualNode['tone']): Tone => tone ?? 'info';

const normalizeTopologyNode = (node: ScreenVisualNode): TopologyNode => ({
  id: node.id,
  label: node.label,
  meta: node.meta ?? '在线',
  type: node.type ?? '园区节点',
  x: node.x,
  y: node.y,
  tone: normalizeTone(node.tone),
  probes: node.probes ?? '2 / 2',
  links: node.links ?? '3 条',
  assets: node.assets ?? '128',
  riskScore: node.riskScore ?? 38,
  bandwidth: node.bandwidth ?? '6.8 Gbps',
  href: node.href ?? '/assets',
});

const normalizeTopologyEdge = (edge: ScreenVisualEdge): TopologyEdge => ({
  from: edge.from,
  to: edge.to,
  tone: edge.tone ?? 'converge',
  width: edge.width,
});

const normalizeCampaignPoint = (point: ScreenVisuals['campaignDensityPoints'][number]): CampaignDensityPoint => ({
  name: point.name,
  x: point.x,
  y: point.y,
  value: point.value ?? 12,
  level: point.level,
});

function ProbeCoverageMap({ nodes: probeNodes, links }: { nodes: ScreenVisualNode[]; links: Array<[string, string]> }) {
  const nodeById = new Map(probeNodes.map((node) => [node.id, node]));
  return (
    <svg className="taf-probe-map__svg" viewBox="0 0 260 240" role="img" aria-label="探针覆盖地图">
      <path className="taf-probe-map__outline" d="M42 74 L76 47 L112 50 L145 32 L197 48 L226 86 L217 125 L226 166 L196 204 L151 214 L118 197 L82 214 L45 186 L31 141 Z" />
      <path className="taf-probe-map__district-line" d="M76 47 L95 94 L42 74" />
      <path className="taf-probe-map__district-line" d="M112 50 L132 118 L95 94" />
      <path className="taf-probe-map__district-line" d="M145 32 L155 74 L132 118" />
      <path className="taf-probe-map__district-line" d="M197 48 L178 98 L226 86" />
      <path className="taf-probe-map__district-line" d="M132 118 L148 143 L96 142 L64 131" />
      <path className="taf-probe-map__district-line" d="M148 143 L186 158 L202 190" />
      {links.map(([sourceId, targetId]) => {
        const source = nodeById.get(sourceId);
        const target = nodeById.get(targetId);
        return !source || !target ? null : <line key={`${sourceId}-${targetId}`} className="taf-probe-map__link" x1={source.x} y1={source.y} x2={target.x} y2={target.y} />;
      })}
      {probeNodes.map((node) => (
        <g key={node.id} className={`taf-probe-map__node ${probeMapStatusClass(node.status ?? 'online')}`} transform={`translate(${node.x} ${node.y})`}>
          <circle className="taf-probe-map__node-halo" r="11" />
          <circle r="5.4" />
          <circle className="taf-probe-map__node-core" r="2.4" />
          {['core', 'teach-a', 'dc', 'dorm', 'soc'].includes(node.id) && <text x="7" y="-7">{node.label}</text>}
        </g>
      ))}
    </svg>
  );
}

const probeMapStatusClass = (status: string) => status === 'offline' ? 'is-risk' : status === 'maintenance' ? 'is-warn' : 'is-ok';

function topologyPoint(id: string, nodes: TopologyNode[]) {
  const node = nodes.find((item) => item.id === id);
  if (!node) return { x: 470, y: 250 };
  return { x: node.x * 10, y: node.y * 5 };
}

const topologyCampusBoundary = [
  [85, 364],
  [150, 242],
  [206, 176],
  [332, 128],
  [498, 104],
  [696, 128],
  [870, 196],
  [928, 306],
  [830, 398],
  [604, 424],
  [388, 416],
  [214, 392],
] as const;

const topologyCampusBoundaryPath = `M ${topologyCampusBoundary.map(([x, y]) => `${x} ${y}`).join(' L ')} Z`;

function isPointInTopologyBoundary(x: number, y: number) {
  let inside = false;
  for (let i = 0, j = topologyCampusBoundary.length - 1; i < topologyCampusBoundary.length; j = i, i += 1) {
    const [xi, yi] = topologyCampusBoundary[i];
    const [xj, yj] = topologyCampusBoundary[j];
    const intersects = yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi;
    if (intersects) inside = !inside;
  }
  return inside;
}

function isTopologyBlockInside(block: { x: number; y: number; w: number; h: number }) {
  const footprint = [
    [block.x + block.w * 0.5 + 12, block.y + block.h * 0.5],
    [block.x + 8, block.y + 2],
    [block.x + block.w + 18, block.y + 2],
    [block.x + 18, block.y + block.h + 10],
    [block.x + block.w + 18, block.y + block.h + 8],
  ];
  return footprint.every(([x, y]) => isPointInTopologyBoundary(x, y));
}

function topologyLinkPath(from: { x: number; y: number }, to: { x: number; y: number }, mode: TopologyMode) {
  if (mode === '2d') return `M ${from.x} ${from.y} L ${to.x} ${to.y}`;
  const dx = to.x - from.x;
  const bend = Math.max(-80, Math.min(80, dx * 0.12));
  const c1x = from.x + dx * 0.38;
  const c2x = from.x + dx * 0.68;
  const c1y = from.y - 36 + bend;
  const c2y = to.y - 28 - bend;
  return `M ${from.x} ${from.y} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${to.x} ${to.y}`;
}

function topologyBuildingHeight(node: TopologyNode) {
  if (node.id === 'core') return 66;
  if (node.tone === 'risk') return 58;
  if (node.tone === 'warn') return 52;
  if (node.id === 'dc') return 62;
  return 46;
}

function topologyCityBlocks(nodes: TopologyNode[]) {
  const blocks = nodes.flatMap((node, index) => {
    const point = topologyPoint(node.id, nodes);
    if (node.id === 'core') return [];
    const width = node.id === 'dc' || node.id === 'lab' ? 58 : 42 + (index % 3) * 8;
    const height = node.id === 'dc' ? 36 : 20 + (index % 3) * 7;
    return [
      {
        id: `${node.id}-a`,
        x: Math.max(120, Math.min(815, point.x - 58 + (index % 3) * 14)),
        y: Math.max(150, Math.min(356, point.y + 42 + (index % 2) * 10)),
        w: width,
        h: height,
        tone: node.tone,
      },
      {
        id: `${node.id}-b`,
        x: Math.max(120, Math.min(815, point.x + 34 - (index % 2) * 14)),
        y: Math.max(150, Math.min(356, point.y + 24 - (index % 3) * 5)),
        w: Math.max(32, width - 10),
        h: Math.max(18, height - 8),
        tone: node.tone,
      },
    ];
  });
  return blocks.filter(isTopologyBlockInside);
}

function topologyZones(nodes: TopologyNode[]) {
  return nodes
    .filter((node) => node.id !== 'core')
    .map((node, index) => {
      const point = topologyPoint(node.id, nodes);
      const width = node.id === 'dc' || node.id === 'lab' ? 168 : node.id === 'soc' ? 142 : 126;
      const height = node.id === 'dorm' ? 94 : node.id === 'library' ? 76 : 86;
      const skew = index % 2 === 0 ? 28 : -22;
      const x1 = Math.max(42, point.x - width / 2);
      const y1 = Math.max(62, point.y - height / 2);
      const x2 = Math.min(948, point.x + width / 2);
      const y2 = Math.min(444, point.y + height / 2);
      return {
        id: `${node.id}-zone`,
        tone: node.tone,
        label: node.type,
        points: `${x1 + 18},${y1 + 8} ${x2 - 22},${y1 + Math.max(0, skew / 3)} ${x2},${y1 + height * 0.52} ${x2 - 36},${y2} ${x1 + 22},${y2 - 4} ${x1},${y1 + height * 0.46}`,
      };
    });
}

function TopologyTwinLayer({ mode, nodes, edges, selectedNodeId }: { mode: TopologyMode; nodes: TopologyNode[]; edges: TopologyEdge[]; selectedNodeId: string }) {
  const blocks = topologyCityBlocks(nodes);
  const zones = topologyZones(nodes);
  return (
    <div className="taf-topology__svg">
      <svg viewBox="0 0 1000 500" role="img" aria-label={`${mode.toUpperCase()} 园区数字孪生拓扑`}>
        <defs><linearGradient id="topologyGridLine" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stopColor="rgba(127, 212, 255, 0.12)" /><stop offset="100%" stopColor="rgba(127, 212, 255, 0.34)" /></linearGradient></defs>
        <path className="taf-topology__campus-boundary" d={topologyCampusBoundaryPath} />
        <g className="taf-topology__svg-zones">{zones.map((zone) => <g key={zone.id} className={`taf-topology__svg-zone is-${zone.tone}`}><polygon points={zone.points} />{mode === '3d' && <polyline points={zone.points.split(' ').slice(0, 3).join(' ')} />}</g>)}</g>
        <path className="taf-topology__campus-road road-a" d="M120 316 C270 270 370 250 470 250 C620 250 748 216 884 178" />
        <path className="taf-topology__campus-road road-b" d="M158 386 C300 332 410 306 530 328 C652 350 742 390 858 430" />
        <path className="taf-topology__campus-road road-c" d="M318 132 C378 208 430 244 470 250 C545 262 632 302 740 362" />
        {mode === '3d' && <g className="taf-topology__svg-city">{blocks.map((block) => <g key={block.id} className={`taf-topology__svg-city-block is-${block.tone}`} transform={`translate(${block.x} ${block.y})`}><path d={`M 0 0 L ${block.w} -12 L ${block.w + 24} 0 L 24 12 Z`} /><path d={`M 0 0 L 24 12 L 24 ${block.h + 12} L 0 ${block.h} Z`} /><path d={`M 24 12 L ${block.w + 24} 0 L ${block.w + 24} ${block.h} L 24 ${block.h + 12} Z`} /></g>)}</g>}
        {edges.map((edge) => {
          const source = topologyPoint(edge.from, nodes);
          const target = topologyPoint(edge.to, nodes);
          const path = topologyLinkPath(source, target, mode);
          return <g key={`${edge.from}-${edge.to}`} className={`taf-topology__svg-link-group is-${edge.tone}`}><path className="taf-topology__svg-link-shadow" d={path} strokeWidth={(edge.width ?? 2) + 5} /><path className={`taf-topology__svg-link is-${edge.tone}`} d={path} strokeWidth={edge.width ?? 2} /><path className="taf-topology__svg-link-pulse" d={path} strokeWidth={Math.max(1.4, (edge.width ?? 2) - 0.4)} /></g>;
        })}
        {nodes.map((node) => {
          const point = topologyPoint(node.id, nodes);
          const height = topologyBuildingHeight(node);
          return mode === '2d' ? <g key={node.id} className={`taf-topology__svg-site is-${node.tone} ${selectedNodeId === node.id ? 'is-selected' : ''}`} transform={`translate(${point.x} ${point.y})`}><circle className="taf-topology__svg-site-halo" r={node.id === 'core' ? 30 : 22} /><circle className="taf-topology__svg-site-ring" r={node.id === 'core' ? 18 : 12} /><circle className="taf-topology__svg-site-dot" r={node.id === 'core' ? 6 : 4.5} /><text x={node.id === 'core' ? 24 : 16} y="-9">{node.label}</text><text className="taf-topology__svg-site-meta" x={node.id === 'core' ? 24 : 16} y="8">{node.meta}</text></g> : <g key={node.id} className={`taf-topology__svg-building is-${node.tone} ${selectedNodeId === node.id ? 'is-selected' : ''}`} transform={`translate(${point.x} ${point.y})`}><ellipse className="taf-topology__svg-pad" cx="0" cy="18" rx="44" ry="17" /><path className="taf-topology__svg-roof" d={`M -30 ${-height} L 0 ${-height - 12} L 30 ${-height} L 0 ${-height + 12} Z`} /><path className="taf-topology__svg-tower" d={`M -30 ${-height} L 0 ${-height + 12} L 0 20 L -30 7 Z`} /><path className="taf-topology__svg-front" d={`M 0 ${-height + 12} L 30 ${-height} L 30 7 L 0 20 Z`} /><circle className="taf-topology__svg-beacon-halo" cx="0" cy={-height - 14} r="13" /><circle className="taf-topology__svg-beacon" cx="0" cy={-height - 14} r="5" /><text className="taf-topology__svg-label" x="38" y={-height - 3}>{node.label}</text></g>;
        })}
      </svg>
    </div>
  );
}

export function SituationalScreen({ route, maskedDemo = false }: { route: NavRoute; maskedDemo?: boolean }) {
  const [topologyMode, setTopologyMode] = useState<TopologyMode>('3d');
  const [selectedNodeId, setSelectedNodeId] = useState('soc');
  const [selectedStageId, setSelectedStageId] = useState('initial');
  const [readonlyTokenOpen, setReadonlyTokenOpen] = useState(false);
  const visualBreakdownMode = isVisualBreakdownMode();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    enabled: !maskedDemo && !visualBreakdownMode,
    refetchInterval: maskedDemo || visualBreakdownMode ? false : 5000,
    refetchIntervalInBackground: true,
  });
  const selectedStage = attackStages.find((stage) => stage.id === selectedStageId) ?? attackStages[0];
  const snapshotRows = visualBreakdownMode ? [] : data?.rows ?? [];
  const displayValue = (value: string | number) => (maskedDemo ? '脱敏' : value);
  const screenMetric = (label: string, fallback: string) => (visualBreakdownMode ? fallback : data?.metrics.find((item) => item.label === label)?.value ?? fallback);
  const screenEvidence = (label: string, fallback: string) => (visualBreakdownMode ? fallback : data?.evidence.find((item) => item.label === label)?.value ?? fallback);
  const screenVisuals = data?.visuals?.screen ?? fallbackScreenVisuals;
  const liveTopologyNodes = screenVisuals.topologyNodes.map(normalizeTopologyNode);
  const liveTopologyEdges = screenVisuals.topologyEdges.map(normalizeTopologyEdge);
  const metricNumber = (label: string, fallback: number) => {
    const raw = screenMetric(label, String(fallback));
    const parsed = Number.parseFloat(String(raw).replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : fallback;
  };
  const topologyGraphNodes: TopologyNode[] = [
    {
      id: 'core',
      label: '核心区',
      meta: `${liveTopologyEdges.length} 条链路`,
      type: '核心交换',
      x: 47,
      y: 50,
      tone: 'info',
      probes: screenEvidence('探针在线', '24/25'),
      links: `${liveTopologyEdges.length} 条`,
      assets: screenEvidence('楼宇覆盖', '27/28'),
      riskScore: metricNumber('高危告警', 12) ? 55 : 24,
      bandwidth: screenMetric('采集吞吐', '78.3 Gbps'),
      href: '/graph',
    },
    ...liveTopologyNodes,
  ];
  const liveCampaignDensityPoints = screenVisuals.campaignDensityPoints.map(normalizeCampaignPoint);
  const liveRiskMapPoints: WorldActivityPoint[] = screenVisuals.riskMapPoints?.length ? screenVisuals.riskMapPoints : riskMapPoints;
  const liveEgressMapPoints: WorldActivityPoint[] = screenVisuals.egressMapPoints?.length ? screenVisuals.egressMapPoints : egressMapPoints;
  const liveEgressMapFlows: WorldActivityFlow[] = screenVisuals.egressMapFlows?.length ? screenVisuals.egressMapFlows : egressMapFlows;
  const liveAbnormalLinks = screenVisuals.abnormalLinks ?? abnormalLinks;
  const abnormalAssetTotal = liveAbnormalLinks.reduce((sum, item) => sum + item.assetCount, 0);
  const riskLevelCounts = liveRiskMapPoints.reduce(
    (acc, point) => {
      if (point.level === 'high') acc.high += 1;
      else if (point.level === 'medium') acc.medium += 1;
      else acc.low += 1;
      return acc;
    },
    { high: 0, medium: 0, low: 0 },
  );
  const visualTargetEgressRows: WorldActivityPoint[] = [
    { name: '北美洲', coord: [220, 210], value: 42.7, level: 'high' },
    { name: '东南亚', coord: [690, 282], value: 31.2, level: 'high' },
    { name: '欧洲', coord: [492, 166], value: 18.6, level: 'medium' },
    { name: '东亚', coord: [763, 206], value: 12.9, level: 'medium' },
    { name: '其他', coord: [842, 356], value: 6.3, level: 'low' },
  ];
  const liveEgressRows = visualBreakdownMode
    ? visualTargetEgressRows
    : liveEgressMapPoints
        .slice()
        .sort((left, right) => right.value - left.value)
        .slice(0, 5);
  const riskLegendValues = visualBreakdownMode
    ? { high: '12 条', medium: '23', low: '41' }
    : {
        high: screenMetric('高危告警', `${riskLevelCounts.high} 条`),
        medium: String(riskLevelCounts.medium || 1),
        low: String(riskLevelCounts.low || 1),
      };
  const selectedNode = topologyGraphNodes.find((node) => node.id === selectedNodeId) ?? topologyGraphNodes[0];
  const trendFromMetric = (label: string, fallbackTrend: number[], spread = 18) => {
    const seed = Math.max(1, metricNumber(label, fallbackTrend[fallbackTrend.length - 1] ?? 45));
    return fallbackTrend.map((value, index) => {
      const wave = Math.sin(index * 0.95 + seed / 17) * spread;
      return Math.max(8, Math.min(96, value * 0.45 + seed * 0.55 + wave));
    });
  };
  const livePipeline: PipelineNode[] = pipeline.map((item): PipelineNode => {
    if (item.label === '探针采集') return { ...item, value: screenEvidence('探针在线', item.value), note: screenMetric('采集吞吐', item.note), trend: trendFromMetric('采集吞吐', item.trend, 12) };
    if (item.label === '协议解析') return { ...item, note: screenMetric('协议解析率', item.note), trend: trendFromMetric('协议解析率', item.trend, 5) };
    if (item.label === 'Kafka 集群') return { ...item, note: screenMetric('Kafka 积压', item.note), tone: metricNumber('Kafka 积压', 0) >= 500 ? 'warn' : 'ok', trend: trendFromMetric('Kafka 积压', item.trend, 22) };
    if (item.label === 'Flink 处理') return { ...item, note: screenMetric('Flink P95', item.note), tone: metricNumber('Flink P95', 0) >= 5000 ? 'warn' : 'ok', trend: trendFromMetric('Flink P95', item.trend, 14) };
    if (item.label === 'ClickHouse') return { ...item, value: screenMetric('采集吞吐', item.value), trend: trendFromMetric('采集吞吐', item.trend, 10) };
    return item;
  });
  const liveEvidenceRings = screenVisuals.evidenceRings ?? evidenceRings;
  const liveResponseStats = responseStats.map((item, index) =>
    index === 0 ? { ...item, value: screenMetric('闭环动作', item.value) } : item,
  );
  const liveRuntimeStats = runtimeStats.map(([label, value]) =>
    label === '大屏刷新间隔' ? [label, snapshotRows.length ? `${snapshotRows.length} 条` : value] : [label, value],
  );

  return (
    <div className={`taf-screen ${maskedDemo ? 'is-masked-demo' : ''}`}>
      {maskedDemo && (
        <div className="taf-screen__mode" role="status" aria-live="polite">
          <Tag color="blue">脱敏公开演示</Tag>
          <span>仅展示聚合态势、模糊化指标和只读跳转，不暴露资产、用户、IP、证据文件或处置动作明细。</span>
          <Button size="small" onClick={() => setReadonlyTokenOpen(true)}>只读令牌配置</Button>
        </div>
      )}
      {isLoading && !maskedDemo && (
        <Alert type="info" showIcon message="实时态势加载中" description="正在读取采集、告警、资产、图谱和数据质量聚合结果，外层大屏布局保持稳定。" />
      )}
      {isError && !maskedDemo && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/dashboard/*、APISIX 路由或告警服务。'}
          action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
        />
      )}
      {!isLoading && !isError && !maskedDemo && data && data.rows.length === 0 && (
        <Alert type="warning" showIcon message="暂无实时态势数据" description="当前时间窗尚无聚合记录，可刷新或检查采集链路；页面继续展示 typed fallback 结构。" action={<Button size="small" onClick={() => void refetch()}>刷新</Button>} />
      )}
      <Modal
        className="taf-screen-readonly-token-modal"
        title="态势大屏只读令牌 / 脱敏配置"
        open={readonlyTokenOpen}
        width="min(620px, calc(var(--taf-window-inner-width, 100dvw) - 64px))"
        onCancel={() => setReadonlyTokenOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setReadonlyTokenOpen(false)}>取消</Button>,
          <Button key="confirm" type="primary" onClick={() => setReadonlyTokenOpen(false)}>确认并写入审计</Button>,
        ]}
      >
        <Alert type="info" showIcon message="只读访问边界" description="令牌仅允许大屏聚合指标与脱敏跳转，不授予资产明细、证据下载或处置权限。" />
        <Descriptions size="small" bordered column={1}>
          <Descriptions.Item label="权限范围">screen:view / aggregate:read</Descriptions.Item>
          <Descriptions.Item label="脱敏策略">IP、账号、资产标识和证据文件名模糊化</Descriptions.Item>
          <Descriptions.Item label="有效期">30 分钟，到期自动失效</Descriptions.Item>
          <Descriptions.Item label="审计 trace">记录签发人、租户、使用来源、时间和撤销结果</Descriptions.Item>
        </Descriptions>
      </Modal>
      <section className="taf-screen__left">
        <WorkPanel
          title="园区覆盖与链路状态"
          className="taf-screen-card taf-screen-coverage"
          extra={<Link to="/probes">查看详情 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-screen-coverage__summary">
            <Link to="/assets">
              <strong>{displayValue(screenMetric('楼宇覆盖率', '96.4%'))}</strong>
              楼宇覆盖率
              <small>建筑 {screenEvidence('楼宇覆盖', '27/28')}</small>
            </Link>
            <Link to="/probes">
              <strong>{displayValue(screenMetric('探针在线率', '95.2%'))}</strong>
              校区在线覆盖
              <small>校区 {screenEvidence('校区覆盖', '4/4')}</small>
            </Link>
          </div>
          <div className="taf-link-grid">
            {[
              ['核心链路', '正常', '8 / 8', 'ok', '/graph'],
              ['汇聚链路', '正常', '24 / 26', 'ok', '/data-quality'],
              ['异常链路', '3 处', '需复核', 'risk', '/alerts'],
            ].map(([label, value, note, tone, href]) => (
              <Link key={label} to={href} className={`is-${tone}`}>
                <em>{label}</em>
                <strong>{displayValue(value)}</strong>
                <small>{note}</small>
              </Link>
            ))}
          </div>
          <div className="taf-probe-map" aria-label="探针覆盖地图">
            <div className="taf-probe-map__legend">
              <span className="is-ok">在线</span>
              <span className="is-risk">离线</span>
              <span className="is-warn">维护中</span>
            </div>
            <ProbeCoverageMap nodes={screenVisuals.probeMapNodes} links={screenVisuals.probeMapLinks} />
            <Link to="/probes" className="taf-probe-map__drill">探针部署图 <ArrowRightOutlined /></Link>
          </div>
        </WorkPanel>
        <WorkPanel
          title="证据与取证闭环"
          className="taf-screen-card taf-screen-evidence"
          extra={<Link to="/forensics">进入取证分析 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-evidence-rings">
            {liveEvidenceRings.map((ring) => (
              <Link key={ring.label} to={ring.href} className="taf-evidence-ring">
                <span className="taf-evidence-ring__chart">
                  <EvidenceClosureRingChart item={ring} masked={maskedDemo} />
                </span>
                <em>{ring.label}</em>
                <small>{displayValue(ring.caption)}</small>
              </Link>
            ))}
          </div>
        </WorkPanel>
      </section>

      <section className="taf-screen__center">
        <WorkPanel
          title="园区数字孪生拓扑"
          className="taf-screen-card taf-screen-topology-panel"
          extra={
            <div className="taf-screen-topology-tools">
              <Button
                size="small"
                type={topologyMode === '2d' ? 'primary' : 'default'}
                aria-pressed={topologyMode === '2d'}
                onClick={() => setTopologyMode('2d')}
              >
                2D
              </Button>
              <Button
                size="small"
                type={topologyMode === '3d' ? 'primary' : 'default'}
                aria-pressed={topologyMode === '3d'}
                onClick={() => setTopologyMode('3d')}
              >
                3D
              </Button>
              <Link to="/graph" className="taf-screen-topology-tools__icon" aria-label="进入拓扑详情">
                <ExpandOutlined />
              </Link>
            </div>
          }
        >
          <div className={`taf-topology is-${topologyMode}`}>
            <TopologyTwinLayer mode={topologyMode} nodes={topologyGraphNodes} edges={liveTopologyEdges} selectedNodeId={selectedNode.id} />
            <div className="taf-topology__legend">
              <span className="is-core">核心链路</span>
              <span className="is-converge">汇聚链路</span>
              <span className="is-risk">异常链路</span>
              <span className="is-probe">探针位置</span>
            </div>
            <div className="taf-topology__frame">
              <span>园区边界</span>
              <span>{visualBreakdownMode ? '实时重算 5s' : `API 节点 ${topologyGraphNodes.length}`}</span>
              <span>建筑 {screenEvidence('楼宇覆盖', `${topologyGraphNodes.length}/${topologyGraphNodes.length}`)}</span>
            </div>
            {topologyGraphNodes.map((node) => (
              <button
                key={node.id}
                type="button"
                className={`${node.id === 'core' ? 'taf-topology__core' : 'taf-topology__node'} is-${node.tone} ${selectedNode.id === node.id ? 'is-active' : ''}`}
                style={{ left: `${node.x}%`, top: `${node.y}%` } as CSSProperties}
                aria-pressed={selectedNode.id === node.id}
                onClick={() => setSelectedNodeId(node.id)}
              >
                {node.id === 'core' ? (
                  <span className="taf-topology__core-symbol" aria-hidden="true">
                    <NodeIndexOutlined />
                  </span>
                ) : (
                  <span className="taf-topology__node-building" aria-hidden="true">
                    <i />
                  </span>
                )}
                <strong>{node.label}</strong>
                <small>{node.meta}</small>
              </button>
            ))}
            <div className="taf-topology__details">
              <header>
                <span>{selectedNode.type}</span>
                <strong>{selectedNode.label}</strong>
              </header>
              <dl>
                <div><dt>在线探针</dt><dd>{displayValue(selectedNode.probes)}</dd></div>
                <div><dt>链路</dt><dd>{displayValue(selectedNode.links)}</dd></div>
                <div><dt>资产</dt><dd>{displayValue(selectedNode.assets)}</dd></div>
                <div><dt>风险</dt><dd className={`is-${selectedNode.tone}`}>{displayValue(selectedNode.riskScore)}</dd></div>
                <div><dt>吞吐</dt><dd>{displayValue(selectedNode.bandwidth)}</dd></div>
              </dl>
              <Link to={selectedNode.href}>下钻建筑详情 <ArrowRightOutlined /></Link>
            </div>
            <div className="taf-topology__abnormal">
              <strong>异常链路位置</strong>
              <Link to="/alerts">实验楼群 - 核心区链路</Link>
              <Link to="/alerts">安全运营中心外联边界</Link>
              <Link to="/data-quality">体育馆 - 汇聚区A链路</Link>
            </div>
            <div className="taf-topology__compass" aria-hidden="true">
              <span>N</span>
              <i />
              <span>S</span>
            </div>
            <Link to="/graph" className="taf-topology__drill">
              进入拓扑详情 <ArrowRightOutlined />
            </Link>
          </div>
        </WorkPanel>

        <WorkPanel
          title="采集与流处理管道（全流量处理链路）"
          className="taf-screen-card taf-screen-pipeline-panel"
          extra={
            <div className="taf-pipeline-toolbar">
              <div className="taf-pipeline-status">
                <span className="is-ok">正常</span>
                <span className="is-warn">繁忙</span>
                <span className="is-risk">异常</span>
              </div>
              <Link to="/data-quality" className="taf-pipeline__drill">进入数据管道详情 <ArrowRightOutlined /></Link>
            </div>
          }
        >
          <div className="taf-pipeline">
            {livePipeline.map((node, index) => (
              <div key={node.label} className="taf-pipeline__item">
                <Link to={node.href} className={`taf-pipeline__node is-${node.tone}`}>
                  <header>
                    <em>{node.icon}</em>
                    <span>{node.label}</span>
                  </header>
                  <dl>
                    <div>
                      <dt>{node.metricLabel}</dt>
                      <dd>{displayValue(node.value)}</dd>
                    </div>
                    <div>
                      <dt>{node.noteLabel}</dt>
                      <dd>{displayValue(node.note)}</dd>
                    </div>
                  </dl>
                  <SparklineChart trend={node.trend} tone={node.trendTone ?? node.tone} />
                </Link>
                {index < pipeline.length - 1 && <span className="taf-pipeline__connector" aria-hidden="true" />}
              </div>
            ))}
          </div>
        </WorkPanel>

        <WorkPanel
          title="响应与反馈闭环（近 24 小时）"
          className="taf-screen-card taf-screen-response-panel"
          extra={<Link to="/playbooks">查看剧本 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-action-stat">
            {liveResponseStats.map((item) => (
              <Link key={item.label} to={item.href} className="taf-action-link">
                {item.icon}
                <strong>{displayValue(item.value)}</strong>
                {item.label}
                <small>24h</small>
              </Link>
            ))}
          </div>
          <div className="taf-learning-strip">
            <span>
              <CheckCircleOutlined />
              模型学习批次数
            </span>
            <strong>{displayValue('8 批')}</strong>
            <Link to="/mlops">
              查看学习任务 <ArrowRightOutlined />
            </Link>
          </div>
        </WorkPanel>
      </section>

      <section className="taf-screen__right">
        <WorkPanel
          title="威胁态势总览"
          className="taf-screen-card taf-screen-threat-panel"
          extra={<Link to="/alerts">近24小时 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-threat-grid">
            <div className="taf-attack-bars">
              <header>
                <span>攻击阶段热度</span>
                <Link to="/alerts">查看告警中心 <ArrowRightOutlined /></Link>
              </header>
              {attackStages.map((stage) => (
                <button
                  type="button"
                  key={stage.id}
                  className={`is-${stage.tone} ${selectedStage.id === stage.id ? 'is-active' : ''}`}
                  aria-pressed={selectedStage.id === stage.id}
                  onClick={() => setSelectedStageId(stage.id)}
                >
                  <em>{stage.label}</em>
                  <i><b style={{ width: `${Math.max(stage.pct * 3, 22)}%` }} /></i>
                  <strong>{displayValue(stage.value)}</strong>
                  <small>{stage.pct}%</small>
                </button>
              ))}
              <p>{selectedStage.detail}</p>
            </div>
            <div className="taf-campaign-radar">
              <CampaignDensityChart points={liveCampaignDensityPoints} ariaLabel="战役簇密度图" />
              <span>战役簇密度</span>
              <em className="is-risk">高</em>
              <em className="is-warn">中</em>
              <em className="is-ok">低</em>
              <Link to="/campaigns">查看战役列表</Link>
            </div>
          </div>
        </WorkPanel>
        <WorkPanel
          title="风险区域密度"
          className="taf-screen-card taf-screen-risk-panel"
          extra={<Link to="/graph">查看风险地图 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-risk-map" aria-label="风险区域密度地图">
            <WorldActivityMap
              variant="risk"
              points={liveRiskMapPoints}
              ariaLabel="世界风险区域密度图"
            />
          </div>
          <div className="taf-risk-legend">
            <Link to="/alerts" className="is-risk">高风险 <strong>{displayValue(riskLegendValues.high)}</strong></Link>
            <Link to="/alerts" className="is-warn">中风险 <strong>{displayValue(riskLegendValues.medium)}</strong></Link>
            <Link to="/assets" className="is-ok">低风险 <strong>{displayValue(riskLegendValues.low)}</strong></Link>
          </div>
        </WorkPanel>
        <WorkPanel
          title="异常链路影响面（Top 5）"
          className="taf-screen-card taf-screen-impact-panel"
          extra={<Link to="/data-quality">查看影响详情 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-impact-grid">
            <div className="taf-impact-table">
              <div className="taf-impact-table__head">
                <span>链路位置</span>
                <span>影响链路数</span>
                <span>影响资产数</span>
              </div>
              {liveAbnormalLinks.map((item) => (
                <Link key={item.name} to="/graph" className="taf-impact-table__row">
                  <strong>{item.name}</strong>
                  <em>{displayValue(item.linkCount.toLocaleString('zh-CN'))}</em>
                  <em>{displayValue(item.assetCount.toLocaleString('zh-CN'))}</em>
                </Link>
              ))}
            </div>
            <Link to="/assets" className="taf-impact-donut">
              <AbnormalImpactPieChart
                items={liveAbnormalLinks.map((item) => ({ name: item.name, value: item.assetCount, level: item.level }))}
                total={abnormalAssetTotal}
              />
            </Link>
          </div>
        </WorkPanel>
        <WorkPanel
          title="外联流向强度（近24小时）"
          className="taf-screen-card taf-screen-egress-panel"
          extra={<Link to="/topics/exfil">查看流向详情 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-egress-map">
            <WorldActivityMap
              variant="egress"
              points={liveEgressMapPoints}
              flows={liveEgressMapFlows}
              ariaLabel="世界外联流向强度图"
            />
          </div>
          <div className="taf-egress-list">
            {liveEgressRows.map((point) => (
              <Link key={point.name} to="/topics/exfil" title={`${point.name} ${point.value.toFixed(1)} Gbps`}>
                {point.name}
                <strong>{displayValue(`${point.value.toFixed(1)} Gbps`)}</strong>
              </Link>
            ))}
          </div>
        </WorkPanel>
        <WorkPanel
          title="运行底座（大屏性能与渲染）"
          className="taf-screen-card taf-screen-runtime-panel"
          extra={<Link to="/compliance">验收状态 <ArrowRightOutlined /></Link>}
        >
          <div className="taf-runtime-grid">
            {liveRuntimeStats.map(([label, value]) => (
              <span key={label}>
                <strong>{displayValue(value)}</strong>
                {label}
              </span>
            ))}
          </div>
          <div className="taf-runtime-status">
            <RadarChartOutlined />
            <span>展示脱敏状态</span>
            <strong>{maskedDemo ? '已脱敏' : '已脱敏'}</strong>
          </div>
        </WorkPanel>
      </section>
    </div>
  );
}
