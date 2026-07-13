import {
  AlertOutlined,
  ApiOutlined,
  AppstoreOutlined,
  AuditOutlined,
  BarChartOutlined,
  BellOutlined,
  BranchesOutlined,
  BugOutlined,
  BuildOutlined,
  ClusterOutlined,
  ControlOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  DotChartOutlined,
  ExperimentOutlined,
  FileDoneOutlined,
  FileProtectOutlined,
  FilterOutlined,
  ForkOutlined,
  FundProjectionScreenOutlined,
  GoldOutlined,
  HddOutlined,
  HistoryOutlined,
  LockOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  ShareAltOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';

export type RouteDomain =
  | 'overview'
  | 'collection-monitoring'
  | 'threat-analysis'
  | 'asset-graph'
  | 'detection-ops'
  | 'audit-config';

export type PageVariant =
  | 'dashboard'
  | 'screen'
  | 'queue'
  | 'pipeline'
  | 'graph'
  | 'governance'
  | 'quality'
  | 'detail';

export type PageSpec = {
  id: string;
  title: string;
  subtitle: string;
  variant: PageVariant;
  background: string;
  tabs: string[];
  kpis: string[];
  tableColumns: string[];
  tableTitle: string;
  rightRailTitle: string;
  actions: string[];
  evidence: string[];
  apiHints: string[];
};

export type NavRoute = {
  id: string;
  title: string;
  path: string;
  icon: ReactNode;
  domain: RouteDomain;
  authMode: 'public' | 'protected';
  accessMode: 'interactive' | 'readonly';
  requiredScopes: string[];
  acceptance: string[];
  badge?: string;
  activeNavId?: string;
  page: PageSpec;
};

export type NavGroup = {
  id: RouteDomain;
  title: string;
  icon: ReactNode;
  children: NavRoute[];
};

const backgroundByDomain: Record<RouteDomain, string> = {
  overview: 'overview',
  'collection-monitoring': 'collection',
  'threat-analysis': 'threat',
  'asset-graph': 'asset',
  'detection-ops': 'detection',
  'audit-config': 'audit',
};

const pageOverrides: Record<string, Partial<PageSpec>> = {
  dashboard: {
    variant: 'dashboard',
    subtitle: '安全值班首页，聚焦今日待办、SLA、取证、反馈和验收缺口。',
    background: 'dashboard',
    tabs: ['运营总览', '待办队列', '健康门禁', '验收缺口'],
    kpis: ['超时 SLA', '临近超时数', '高危未处理', '待取证', '待反馈', '待复核', '队列积压量', '今日闭环进度'],
    tableTitle: '优先级待办队列',
    tableColumns: ['事件 ID', '风险级别', '资产组', '业务系统', '处置阶段', '剩余时间', '证据状态'],
    rightRailTitle: '验收缺口与建议动作',
    actions: ['补齐证据', '回流样本', '完善留痕', '跟进处理'],
    evidence: ['PCAP 覆盖率', 'Session 还原率', '日志关联率', '审计留痕'],
    apiHints: ['/api/v1/dashboard/stats', '/api/v1/alerts', '/ws/events'],
  },
  screen: {
    variant: 'screen',
    subtitle: '领导汇报、售前演示和值班展示，一屏讲清采集分析闭环。',
    background: 'screen-campus',
  },
  topics: {
    variant: 'graph',
    background: 'topic-tunnel',
    subtitle: '统一承载加密隧道、数据外传和 APT 战役三类专题研判。',
    tabs: ['加密隧道专题', '数据外传专题', 'APT 战役专题'],
    kpis: ['专题总数', '高危专题', '关联告警', '影响实体', '取证窗口', '证据完整度'],
    tableTitle: '专题线索',
    tableColumns: ['专题', '对象', '范围', '风险', '证据', '状态', '处置'],
    rightRailTitle: '专题闭环建议',
    actions: ['切换专题', '保存视图', '生成报告', '写入审计'],
    evidence: ['隧道专题接口', '外传专题接口', 'APT 专题接口', 'PCAP/会话', '审计记录'],
    apiHints: ['/api/v1/topics/tunnel', '/api/v1/topics/exfil', '/api/v1/topics/apt', '/api/v1/topics/views', '/api/v1/topics/subscriptions'],
  },
  'topic-tunnel': {
    variant: 'graph',
    background: 'topic-tunnel',
    subtitle: '围绕 TLS、QUIC、VPN、DoH 和未知加密通道组织隧道专题研判。',
    tabs: ['专题总览', '协议族', '高危用户', '指纹证据', '处置闭环'],
    kpis: ['隧道协议数', '高频隧道源', '加密会话流量', '异常隧道数', '隧道端点数', '可疑隧道占比', '证据完整度', '报告置信度', '未闭环风险数'],
    tableTitle: '加密隧道关联事件与证据',
    tableColumns: ['事件ID', '隧道源', '协议', '目的端点', '证据类型', '时间窗', '风险状态', '风险操作'],
    rightRailTitle: '专题交付摘要',
    actions: ['提取 PCAP', '阻断隧道', '关联 JA3', '下钻资产', '生成专题报告'],
    evidence: ['隧道专题接口', '协议分布', '高危用户', 'JA3/JA3S 指纹', 'PCAP 窗口', '审计记录'],
    apiHints: ['/api/v1/topics/tunnel'],
  },
  'topic-exfil': {
    variant: 'quality',
    background: 'topic-exfil',
    subtitle: '聚合大流量上传、异常目的地、风险类型和外传路径，形成数据外传专题。',
    tabs: ['专题总览', '源资产', '外传路径', '风险类型', '证据包'],
    kpis: ['外传预警量', '外传路径数', '可疑外传源', '外传目的地数', '敏感数据类型数', '异常上传峰值', '跨境目的地数', '证据完整度'],
    tableTitle: '数据外传关联事件与证据',
    tableColumns: ['源资产', '外传路径', '目标区域', '数据类型', '上传量', '会话数', '风险类型', '风险等级', '处置'],
    rightRailTitle: '专题交付摘要',
    actions: ['阻断路径', '隔离源资产', '提取样本', '复核白名单', '生成外传报告'],
    evidence: ['外传专题接口', '源资产排行', '风险类型', '路径分析', 'PCAP/会话', '审计记录'],
    apiHints: ['/api/v1/topics/exfil'],
  },
  'topic-apt': {
    variant: 'graph',
    background: 'topic-apt',
    subtitle: '以战役为主线聚合阶段、实体、告警、证据和处置复盘。',
    tabs: ['专题总览', '战役阶段', '影响实体', '证据链', '复盘结论'],
    kpis: ['关联战役数', '战役集密度', '攻击阶段覆盖', '关键资产命中', '横向移动链路', '持久化迹象数', '外传关联证据', '处置闭环率', '报告置信度'],
    tableTitle: 'APT 战役线索',
    tableColumns: ['战役名称', '阶段', '关键实体', '关联告警', '攻击技术', '首次发现', '最近活动', '风险等级', '处置'],
    rightRailTitle: '战役复盘与演变',
    actions: ['下钻攻击链', '导出战役包', '生成复盘', '关联规则', '写入审计'],
    evidence: ['APT 专题接口', '战役聚类', '阶段分布', '实体图谱', '证据包', '审计记录'],
    apiHints: ['/api/v1/topics/apt'],
  },
  probes: {
    variant: 'pipeline',
    subtitle: '证明系统看得见、采得全、采得稳，覆盖部署、心跳、吞吐和证书状态。',
    tabs: ['探针总览', '部署拓扑', '吞吐丢包', '配置下发', '心跳日志'],
    kpis: ['探针总数', '在线探针', '采集网卡', '采集模式', '平均 CPU', '平均内存', '告警探针', '离线探针'],
    tableTitle: '探针状态矩阵',
    tableColumns: ['探针 ID', '位置', '状态', '采集模式', '采集带宽', '丢包率', '解析率', 'CPU', '内存', '运行时长', '版本', '操作'],
    rightRailTitle: '探针运维闭环',
    actions: ['批量升级', '下发策略', '连通测试', '轮换证书'],
    evidence: ['心跳同步', 'mTLS', '接口状态', '批量发送', '审计记录'],
  },
  'data-quality': {
    variant: 'quality',
    subtitle: '证明数据可信，定位采集、解析、传输、处理和入库质量问题。',
    tabs: ['质量总览', 'Topic 健康', 'Flink 质量', '字段质量', '存储质量', '重放对账', '质量报告', '质量设置'],
    kpis: ['质量总分', '完整性', '及时性', '准确性', '重复率', '字段缺失率', 'DLQ 数量'],
    tableTitle: 'Kafka Topic 健康 (Top 10)',
    tableColumns: ['Topic', '分区数', '当前吞吐量', '消费延迟', '积压量', '积压趋势', '消费延迟 P95', '分区倾斜', '消息延迟 P95', '操作'],
    rightRailTitle: '质量修复建议',
    actions: ['定位 Flink', '重放 DLQ', '生成报告', '调整阈值'],
    evidence: ['质量基线', 'Kafka Topic', 'Flink Checkpoint', '字段矩阵', '存储写入', '重放对账'],
    apiHints: ['/api/v1/data-quality', '/api/v1/dlq/replay/fallback'],
  },
  alerts: {
    variant: 'queue',
    background: 'alerts',
    subtitle: '告警研判主工作台，完成筛选、取证、处置、反馈和审计闭环。',
    tabs: ['告警队列', '筛选检索', '研判时间线', '关联告警簇', '处置反馈'],
    kpis: ['高危', '中危', '低危', '未处理', '处理中', '已确认', '已忽略'],
    tableTitle: '告警列表',
    tableColumns: ['告警 ID', '风险等级', '告警名称', '攻击阶段', '源 IP', '目的 IP', '受影响资产', '规则/模型', '置信度', '首次发生', '状态'],
    rightRailTitle: '告警详情与处置反馈',
    actions: ['隔离主机', '阻断连接', '封禁 IP', '下发脚本', '加入白名单'],
    apiHints: ['/api/v1/alerts', '/api/v1/alerts/batch/status items[].state_version', '/api/v1/alerts/{id}/assign', '/api/v1/alerts/{id}/feedback'],
  },
  campaigns: {
    variant: 'queue',
    subtitle: '把多条告警聚合成战役，支持跨时间、跨资产、跨阶段调查。',
    tabs: ['战役总览', '战役列表', 'ATT&CK 时间线', '影响范围', '证据汇总'],
    kpis: ['战役总数', '活跃战役', '影响资产', '最高风险', '告警总数', '平均持续时间'],
    tableTitle: '战役列表',
    tableColumns: ['战役名称', '阶段', '风险等级', '影响资产', '告警数', '首次发现', '最近活动', '状态', '操作'],
    rightRailTitle: '战役影响范围',
    actions: ['变更状态', '生成报告', '下钻攻击链', '导出证据包'],
  },
  'attack-chains': {
    variant: 'graph',
    subtitle: '解释攻击如何发生、经过哪些阶段、影响哪些实体和资产。',
    tabs: ['攻击链画布', '阶段识别', '路径分析', '证据锚点', '处置建议'],
    kpis: ['阶段节点', '实体节点', '证据锚点', '阻断点', '置信度'],
    tableTitle: '攻击阶段泳道',
    tableColumns: ['阶段', '实体', '告警', '证据', '处置建议', '状态'],
    rightRailTitle: '处置建议',
    actions: ['跳转告警', '查看证据', '关联规则', '触发剧本'],
  },
  'encrypted-traffic': {
    variant: 'quality',
    background: 'threat',
    subtitle: '对 TLS、QUIC、VPN、隧道和未知加密外联进行可解释分析。',
    tabs: ['总览', '指纹分析', '隧道检测', '外联画像', '证据中心'],
    kpis: ['加密流量总量', 'TLS 流量占比', 'QUIC 流量占比', '未知加密占比', '异常证书数', '可疑 JA3 数', '未知 SNI 比例'],
    tableTitle: '加密会话风险列表',
    tableColumns: ['时间', '协议', 'Session 摘要', '证书详情', 'SNI', 'JA3', 'JA3S', 'ALPN', 'TLS 版本', '密码套件', '证书 Issuer', '风险等级', '操作'],
    rightRailTitle: '证据与规则关联',
    actions: ['创建告警', '提取 PCAP', '关联模型', '进入图谱'],
    evidence: ['Session', 'PCAP 索引', '证书详情', '握手元数据'],
  },
  forensics: {
    variant: 'queue',
    subtitle: '围绕告警、资产、时间窗完成 PCAP、Session、日志证据检索与下载审计。',
    tabs: ['取证任务', 'PCAP 索引', '会话复放', '完整性', '证据导出'],
    kpis: ['取证任务', '处理中', '已完成', 'PCAP 文件', 'Hash 通过', '签名 URL', '审计成功'],
    tableTitle: '取证任务队列',
    tableColumns: ['任务 ID', '告警/战役 ID', '资产', '五元组', '时间窗', '证据包', '状态', '操作'],
    rightRailTitle: '证据完整性',
    actions: ['新建任务', '校验 Hash', '下载证据', '写入审计'],
    evidence: ['原始文件校验', '文件 hash 校验', '签名 URL', '租户隔离', '下载审计'],
  },
  assets: {
    variant: 'queue',
    background: 'assets',
    subtitle: '管理终端、服务器、网络设备、业务系统和风险画像。',
    tabs: ['终端', '服务器', '网络设备', '业务系统', '未知资产'],
    kpis: ['已识别资产', '未知资产', '漂移资产', '长期离线资产', '暴露服务数'],
    tableTitle: '资产台账',
    tableColumns: ['资产 ID', 'IP/MAC', '主机名', '类型', '园区/部门', '操作系统', '重要性', '最近活跃', '暴露端口', '风险标签'],
    rightRailTitle: '资产风险画像',
    actions: ['进入详情', '登记发现凭据', '启动 SNMP/LLDP 发现', '生成工单', '跳转告警', '打开图谱'],
  },
  graph: {
    variant: 'graph',
    background: 'graph',
    subtitle: '围绕 IP、账号、主机、服务、域名、告警构建实体关系和路径分析。',
    tabs: ['最短路径', '攻击路径', '通信路径', '账号访问路径'],
    kpis: ['实体节点', '关系边', '异常路径', '关键资产', '告警关联'],
    tableTitle: '路径分析结果',
    tableColumns: ['路径 ID', '源实体', '目标实体', '跳数', '风险', '证据'],
    rightRailTitle: '实体上下文',
    actions: ['展开邻居', '保存路径', '跳转资产', '生成证据'],
  },
  fusion: {
    variant: 'quality',
    subtitle: '融合资产源、流量源、日志源、漏洞源，治理冲突和可信度。',
    tabs: ['融合总览', '冲突队列', '可信度', '来源质量', '回写审计'],
    kpis: ['融合实体', '冲突数', '可信度', '来源覆盖', '回写成功率'],
    tableTitle: '融合冲突队列',
    tableColumns: ['对象', '来源 A', '来源 B', '冲突字段', '可信度', '处理状态'],
    rightRailTitle: '冲突处理',
    actions: ['接受来源', '人工修正', '回写资产', '查看审计'],
    apiHints: [
      '/api/v1/fusion/stats',
      '/api/v1/fusion/entities',
      '/api/v1/fusion/value-report',
      '/api/v1/threat-intel/entries',
      '/api/v1/fusion/conflicts/{id}/resolve',
      '/api/v1/fusion/rules/{id}',
    ],
  },
  baselines: {
    variant: 'quality',
    subtitle: '建立资产、账号、端口、协议和时间段行为基线并解释偏离。',
    tabs: ['资产基线', '账号基线', '端口基线', '协议基线', '时间段基线'],
    kpis: ['偏离资产', '新端口', '异常协议', '夜间访问', '基线稳定度'],
    tableTitle: '行为偏离列表',
    tableColumns: ['对象', '基线类型', '偏离值', '证据', '解释', '状态'],
    rightRailTitle: '偏离解释',
    actions: ['生成告警', '更新基线', '查看证据', '加入观察'],
  },
  rules: {
    variant: 'governance',
    background: 'rules',
    subtitle: '检测规则生命周期，覆盖定义、验证、依赖、发布和审计。',
    tabs: ['规则定义', '测试验证', '依赖引用', 'PCAP 样本', 'Session 样本', '日志样本'],
    kpis: ['规则草稿', '待审核规则', '灰度规则', '启用规则', '回滚候选', '高耗时规则'],
    tableTitle: '规则清单',
    tableColumns: ['规则ID', '规则名称', '类型', '严重级别', 'MITRE阶段', '状态', '版本', '命中数', '误报率', '平均延时'],
    rightRailTitle: '规则发布门禁',
    actions: ['测试验证', '灰度发布', '回滚版本', '写入审计'],
    evidence: ['规则库', '样本回放', '命中矩阵', '误报反馈', '发布门禁', '版本审计'],
  },
  deployments: {
    variant: 'governance',
    subtitle: '规则、模型、采集策略的发布、灰度、回滚和运行态追踪。',
    tabs: ['发布计划', '灰度状态', '回滚窗口', '运行健康', '审计记录'],
    kpis: ['待发布对象', '灰度中', '失败/阻断', '可回滚版本', '发布成功率', '平均生效延迟'],
    tableTitle: '部署批次',
    tableColumns: ['发布对象', '版本', '环境', '状态', '负责人', '发布时间', '影响范围', '操作'],
    rightRailTitle: '发布状态机',
    actions: ['创建发布', '暂停批次', '执行回滚', '查看审计'],
    evidence: ['manifest', '镜像', 'DDL', 'topic', '规则版本', '模型版本'],
  },
  models: {
    variant: 'governance',
    background: 'mlops',
    subtitle: '模型版本、指标、数据集、解释、激活和回滚治理。',
    tabs: ['重要特征', '规则贡献', '异常解释', '样本示例', '激活流程', '审计门禁'],
    kpis: ['线上模型数', '候选模型数', '漂移告警', '待重训模型', '平均 F1', '误报率变化'],
    tableTitle: '模型版本列表',
    tableColumns: ['模型名', '类型', '版本', '状态', '线上版本', '训练时间', '负责人', '操作'],
    rightRailTitle: '模型治理闭环',
    actions: ['激活模型', '回滚版本', '启动评估', '查看审计'],
  },
  mlops: {
    variant: 'pipeline',
    background: 'mlops',
    subtitle: '标注、训练、评估、发布、效果回流的 MLOps 编排。',
    tabs: ['任务 DAG', '标注回流', '训练评估', '模型注册', '在线效果'],
    kpis: ['训练任务', '评估任务', '注册任务', '发布任务', '失败任务', '门禁通过率'],
    tableTitle: 'MLOps 任务队列',
    tableColumns: ['任务ID', '阶段', '数据集版本', '算法配置', '特征版本', '资源占用', '状态', '操作'],
    rightRailTitle: '训练发布闭环',
    actions: ['启动训练', '注册模型', '灰度发布', '回流样本'],
  },
  playbooks: {
    variant: 'governance',
    subtitle: '隔离、阻断、封禁、工单同步和自动化处置编排。',
    tabs: ['剧本列表', '剧本编排', '触发策略', '执行历史', '风险控制', '审计证据'],
    kpis: ['启用剧本', '待审批', '今日执行', '失败步骤', '高危待确认', '平均处理耗时'],
    tableTitle: 'SOAR 剧本执行',
    tableColumns: ['剧本名称', '适用告警', '动作类型', '风险级别', '启用状态', '最近执行', '操作'],
    rightRailTitle: '响应动作链',
    actions: ['执行剧本', '审批动作', '回滚动作', '生成工单'],
    evidence: ['剧本目录', '执行记录', '审批单', '回滚记录', '审计日志', '合规证据'],
  },
  whitelist: {
    variant: 'governance',
    subtitle: '业务例外、规则豁免、审批、生效范围和到期治理。',
    tabs: ['域名', 'IP', '资产', '账号', '规则', '模型', '即将到期', '过期未处理', '长期生效'],
    kpis: ['生效白名单', '待审批', '即将到期', '长期生效', '覆盖告警', '潜在漏报风险'],
    tableTitle: '白名单列表',
    tableColumns: ['对象类型', '匹配条件', '生效范围', '有效期', '责任角色', '来源告警', '状态', '操作'],
    rightRailTitle: '审批与到期治理',
    actions: ['新增白名单', '从告警生成草案', '提交审批', '批量延期', '停用', '转审计'],
    evidence: ['白名单目录', '审批状态', '到期治理', '命中监控', '来源告警', '审计记录'],
    apiHints: [
      '/api/v1/whitelist',
      '/api/v1/whitelist/{id}',
      '/api/v1/whitelist/check',
      '/api/v1/alerts/{id}/feedback add_to_whitelist -> /api/v1/whitelist draft',
      '/api/v1/audit/logs?action=WHITELIST_*',
    ],
  },
  compliance: {
    variant: 'governance',
    background: 'audit',
    subtitle: '任务书指标、证据包、验收达标状态和审计门禁。',
    tabs: ['验收门禁', '指标映射', '证据包', '运行报告', '缺口治理', '第三方评测'],
    kpis: ['门禁通过率', '未达标项', '证据完整度', '复验通过率', '第三方批次', '报告生成数'],
    tableTitle: '验收门禁矩阵',
    tableColumns: ['维度', '任务书指标(覆盖率)', '测试项(通过/总数)', '数据源(覆盖率)', '证据状态(完整度)', '最近复验(日期间)', '结果'],
    rightRailTitle: '证据包导出',
    actions: ['生成验收报告', '导出证据包', '导出 PDF', '导出 Word', '创建整改任务', '固化验收记录'],
    evidence: ['测试报告', 'PCAP hash', '审计日志', '模型版本', '规则版本', '部署 manifest'],
  },
  'audit-log': {
    variant: 'queue',
    background: 'audit',
    subtitle: '关键操作全链路留痕，覆盖检索、详情 Diff、高风险复核、关联链路、留存校验和取证导出。',
    tabs: ['日志检索', '操作详情', '高风险审计', '关联链路', '留存状态', '导出取证'],
    kpis: ['今日操作', '失败操作', '高风险操作', '导出下载', 'PCAP 访问', '完整性校验通过率'],
    tableTitle: '审计日志',
    tableColumns: ['时间', '用户/角色', '对象类型', '动作类型', '结果', '请求ID', 'trace_id', '风险标签', '操作'],
    rightRailTitle: '操作详情 / Diff 视图',
    actions: ['保存查询', '导出取证', '生成合规证据', '触发复核', '归档校验'],
    evidence: ['Audit Logs API', '操作详情', '高风险审计', '关联链路', '留存状态', '导出取证'],
  },
  notifications: {
    variant: 'governance',
    background: 'settings',
    subtitle: '配置告警、系统异常、验收缺口和运营任务的通知渠道、订阅规则、升级策略和静默窗口。',
    tabs: ['通知渠道', '订阅规则', '升级策略', '模板管理', '发送历史', '抑制静默'],
    kpis: ['启用渠道', '订阅规则', '待确认通知', '失败通知', '升级策略', '静默窗口'],
    tableTitle: '通知路由规则',
    tableColumns: ['规则', '严重级别', '告警类型', '资产组/园区', '时间窗', '渠道', '升级策略', '静默', '状态', '操作'],
    rightRailTitle: '通知策略预览',
    actions: ['新增渠道', '测试发送', '保存订阅策略', '新建升级策略', '静默窗口', '导入审计'],
    evidence: ['Notification Settings API', 'Secret 引用', '通道测试', '订阅策略', '升级策略', '投递审计', '静默窗口'],
  },
  settings: {
    variant: 'governance',
    background: 'settings',
    subtitle: '管理租户、角色权限、令牌、数据留存、集成凭据和系统级参数。',
    tabs: ['租户站点', '权限矩阵', 'API 令牌', '留存策略', '集成配置', '安全策略', '系统参数'],
    kpis: ['租户数', '角色策略', '有效令牌', '即将过期令牌', '集成健康', '配置变更待审计'],
    tableTitle: 'API 令牌',
    tableColumns: ['令牌名称', '权限范围', '令牌指纹', '过期时间', '最近使用', '轮换状态', '操作'],
    rightRailTitle: '闭环动作入口',
    actions: ['保存配置', '连接测试', '创建令牌', '轮换令牌', '触发安全审计', '查看影响范围'],
    evidence: ['Token Scopes API', 'Token List API', 'Probe Scopes API', 'RBAC 矩阵', '留存策略', '集成健康', '审计写入'],
  },
};

const defaultSpec = (id: string, title: string, domain: RouteDomain, variant: PageVariant = 'queue'): PageSpec => ({
  id,
  title,
  subtitle: `${title}围绕全流量采集、研判、取证、响应和验收形成闭环工作台。`,
  variant,
  background: backgroundByDomain[domain],
  tabs: ['总览', '明细', '趋势', '证据', '审计'],
  kpis: ['总量', '高风险', '处理中', '健康度', '证据完整度'],
  tableTitle: `${title}明细`,
  tableColumns: ['对象 ID', '类型', '范围', '风险', '证据', '状态'],
  rightRailTitle: `${title}闭环动作`,
  actions: ['查看详情', '下钻证据', '生成任务', '写入审计'],
  evidence: ['PCAP', 'Session', '日志', '图谱路径', '审计记录'],
  apiHints: [`/api/v1/${id}`],
});

const domainScopes: Record<RouteDomain, string[]> = {
  overview: ['alert:read'],
  'collection-monitoring': ['admin:*', 'probe:metrics'],
  'threat-analysis': ['alert:read'],
  'asset-graph': ['graph:read'],
  'detection-ops': ['rule:read'],
  'audit-config': ['admin:*', 'user:read', 'token:read'],
};

const routeScopes: Record<string, string[]> = {
  screen: ['screen:view'],
  forensics: ['pcap:read'],
  deployments: ['deploy:read'],
  settings: ['admin:*', 'token:read'],
  'audit-log': ['admin:*', 'user:read'],
  compliance: ['admin:*', 'user:read'],
  notifications: ['admin:*', 'user:read'],
  'alert-detail': ['alert:read'],
  'campaign-detail': ['alert:read'],
};

const routeAcceptance = (id: string, title: string) => [
  `${title} route is registered in routeManifest`,
  `${title} requires authenticated /api/v1/auth/me session`,
  `${title} API evidence is mapped through services/api.ts`,
  `${title} route is covered by product-navigation smoke where applicable`,
];

const makeRoute = (
  domain: RouteDomain,
  id: string,
  title: string,
  path: string,
  icon: ReactNode,
  variant: PageVariant = 'queue',
  badge?: string,
  activeNavId?: string,
): NavRoute => {
  const base = defaultSpec(id, title, domain, variant);
  return {
    id,
    title,
    path,
    icon,
    domain,
    authMode: 'protected',
    accessMode: id === 'screen' ? 'readonly' : 'interactive',
    requiredScopes: routeScopes[id] ?? domainScopes[domain],
    acceptance: routeAcceptance(id, title),
    badge,
    activeNavId,
    page: { ...base, ...pageOverrides[id], id, title },
  };
};

export const navGroups: NavGroup[] = [
  {
    id: 'overview',
    title: '综合态势',
    icon: <AppstoreOutlined />,
    children: [
      makeRoute('overview', 'dashboard', '仪表盘', '/dashboard', <DashboardOutlined />, 'dashboard'),
      makeRoute('overview', 'screen', '态势大屏', '/screen', <FundProjectionScreenOutlined />, 'screen'),
      makeRoute('overview', 'topics', '专题面板', '/topics', <BranchesOutlined />, 'graph'),
    ],
  },
  {
    id: 'collection-monitoring',
    title: '采集监测',
    icon: <RadarChartOutlined />,
    children: [
      makeRoute('collection-monitoring', 'probes', '探针管理', '/probes', <ApiOutlined />, 'pipeline'),
      makeRoute('collection-monitoring', 'data-quality', '数据质量', '/data-quality', <DatabaseOutlined />, 'quality', '2'),
    ],
  },
  {
    id: 'threat-analysis',
    title: '威胁分析',
    icon: <AlertOutlined />,
    children: [
      makeRoute('threat-analysis', 'alerts', '告警中心', '/alerts', <BellOutlined />, 'queue', '128'),
      makeRoute('threat-analysis', 'campaigns', '战役列表', '/campaigns', <BuildOutlined />, 'queue'),
      makeRoute('threat-analysis', 'attack-chains', '攻击链分析', '/attack-chains', <ForkOutlined />, 'graph'),
      makeRoute('threat-analysis', 'encrypted-traffic', '加密流量', '/encrypted-traffic', <LockOutlined />, 'quality'),
      makeRoute('threat-analysis', 'forensics', '取证分析', '/forensics', <FileProtectOutlined />, 'queue'),
    ],
  },
  {
    id: 'asset-graph',
    title: '资产图谱',
    icon: <ShareAltOutlined />,
    children: [
      makeRoute('asset-graph', 'assets', '资产台账', '/assets', <GoldOutlined />, 'queue'),
      makeRoute('asset-graph', 'graph', '实体图谱', '/graph', <NodeIndexOutlined />, 'graph'),
      makeRoute('asset-graph', 'fusion', '数据融合', '/fusion', <ClusterOutlined />, 'quality'),
      makeRoute('asset-graph', 'baselines', '行为基准', '/baselines', <DotChartOutlined />, 'quality'),
    ],
  },
  {
    id: 'detection-ops',
    title: '检测运营',
    icon: <BugOutlined />,
    children: [
      makeRoute('detection-ops', 'rules', '规则管理', '/rules', <FilterOutlined />, 'governance'),
      makeRoute('detection-ops', 'deployments', '部署管理', '/deployments', <DeploymentUnitOutlined />, 'governance'),
      makeRoute('detection-ops', 'models', '模型管理', '/models', <ExperimentOutlined />, 'governance'),
      makeRoute('detection-ops', 'mlops', 'MLOps 编排', '/mlops', <BranchesOutlined />, 'pipeline'),
      makeRoute('detection-ops', 'playbooks', 'SOAR 剧本', '/playbooks', <ThunderboltOutlined />, 'governance'),
      makeRoute('detection-ops', 'whitelist', '白名单', '/whitelist', <SafetyCertificateOutlined />, 'governance'),
    ],
  },
  {
    id: 'audit-config',
    title: '审计配置',
    icon: <AuditOutlined />,
    children: [
      makeRoute('audit-config', 'compliance', '合规审计', '/compliance', <FileDoneOutlined />, 'governance'),
      makeRoute('audit-config', 'audit-log', '审计日志', '/audit-log', <HistoryOutlined />, 'queue'),
      makeRoute('audit-config', 'notifications', '通知配置', '/notifications', <ControlOutlined />, 'governance'),
      makeRoute('audit-config', 'settings', '系统设置', '/settings', <SettingOutlined />, 'governance'),
    ],
  },
];

export const navRoutes = navGroups.flatMap((group) => group.children);

export const legacyTopicRoutes: NavRoute[] = [
  makeRoute('overview', 'topic-tunnel', '加密隧道专题', '/topics/tunnel', <LockOutlined />, 'graph'),
  makeRoute('overview', 'topic-exfil', '数据外传专题', '/topics/exfil', <BarChartOutlined />, 'quality'),
  makeRoute('overview', 'topic-apt', 'APT 战役专题', '/topics/apt', <BranchesOutlined />, 'graph'),
];

export const detailRoutes: NavRoute[] = [
  makeRoute('threat-analysis', 'alert-detail', '告警详情', '/alerts/:alertId', <FileProtectOutlined />, 'detail', undefined, 'alerts'),
  makeRoute('threat-analysis', 'campaign-detail', '战役详情', '/campaigns/:campaignId', <HddOutlined />, 'detail', undefined, 'campaigns'),
].map((route) => ({
  ...route,
  page: {
    ...route.page,
    variant: 'detail',
    tabs:
      route.id === 'alert-detail'
        ? ['全部', 'PCAP', 'Session', '日志', '图谱路径', '文件']
        : ['战役画像', '攻击时间轴', '关联告警', '影响范围', '证据包', '复盘结论'],
    kpis:
      route.id === 'alert-detail'
        ? ['风险评分', '证据完整度', '处置动作', '反馈状态', '审计记录']
        : ['风险评分', '关联告警', '影响资产', '攻击阶段', '证据完整度', '处置进度'],
    tableTitle: route.id === 'alert-detail' ? '证据清单' : '关联告警',
    tableColumns:
      route.id === 'alert-detail'
        ? ['证据类型', '文件记录', '内容摘要', '大小', '生成时间', '状态', '操作']
        : ['告警时间', '告警 ID', '告警名称', '攻击阶段', '影响资产', '风险', '状态', '操作'],
    rightRailTitle: route.id === 'alert-detail' ? '处置与反馈' : '处置流程与复盘结论',
    actions:
      route.id === 'alert-detail'
        ? ['隔离主机', '阻断 IP', '封禁账户', '下发脚本', '创建工单']
        : ['生成战役报告', '导出战役包', '下钻攻击链', '查看资产', '写入审计'],
    evidence:
      route.id === 'alert-detail'
        ? ['Alert Detail API', 'Evidence API', 'Feedback API', '审计提示']
        : ['Campaign Detail API', '关联告警', '影响实体', '证据包完整度', '处置流程', '复盘结论'],
    apiHints:
      route.id === 'alert-detail'
        ? [
            '/api/v1/alerts/{id}',
            '/api/v1/alerts/{id}/evidence',
            '/api/v1/alerts/{id}/status',
            '/api/v1/alerts/{id}/assign',
            '/api/v1/alerts/{id}/close',
            '/api/v1/alerts/{id}/reopen',
            '/api/v1/alerts/{id}/feedback',
            '/api/v1/alerts/{id}/feedback add_to_whitelist -> /api/v1/whitelist draft',
          ]
        : route.page.apiHints,
  },
}));

export const allRoutes = [...navRoutes, ...detailRoutes, ...legacyTopicRoutes];

export const findRouteByPath = (pathname: string) => {
  const withoutQuery = pathname.split('?')[0];
  const direct = allRoutes.find((route) => route.path === withoutQuery);
  if (direct) return direct;
  if (withoutQuery.startsWith('/alerts/')) return detailRoutes.find((route) => route.id === 'alert-detail');
  if (withoutQuery.startsWith('/campaigns/')) return detailRoutes.find((route) => route.id === 'campaign-detail');
  return undefined;
};

export const findRouteById = (id: string) => allRoutes.find((route) => route.id === id);
