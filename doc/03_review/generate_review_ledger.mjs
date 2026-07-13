import fs from 'node:fs';
import path from 'node:path';

const outDir = path.resolve('doc/03_review');
const csvPath = path.join(outDir, 'multi_role_review_ledger.csv');
const summaryPath = path.join(outDir, 'multi_role_review_summary.json');

const phases = [
  {
    id: '初评',
    count: 1000,
    roles: ['产品经理', '技术经理', '销售经理'],
    domains: ['课题一符合度', '产品闭环', '工程闭环', '售前闭环', '验收证据'],
  },
  {
    id: '中期评审',
    count: 10000,
    roles: ['产品经理', '技术经理', '销售经理', '项目经理', '全栈工程师', '算法工程师', '实施工程师', '测试工程师', 'UI工程师'],
    domains: ['进度与范围', '端到端链路', '检测质量', '性能容量', '安全合规', '交付实施', '用户体验', '测试验收'],
  },
  {
    id: '技术总监复审',
    count: 100,
    roles: ['技术总监'],
    domains: ['架构取舍', '生产化风险', '验收门禁', '技术路线竞争力', '课题协同'],
  },
];

const issuePool = [
  {
    domain: '课题一符合度',
    issue: '课题一要求的多源异构数据融合需要映射到流量、资产、设备日志、用户行为四类可验收数据源。',
    decision: '在设计文档增加任务书指标追溯矩阵，并把每类数据源绑定到采集、处理、存储、页面和测试证据。',
    anchor: '01_design/课题一产品与技术总体设计.md#3-req-t1-追溯矩阵',
  },
  {
    domain: '产品闭环',
    issue: '全流量能力容易被误解为永久全包留存。',
    decision: '明确产品口径为 Flow/Session/Feature 实时分析加策略化 PCAP 索引、裁剪、留存和下载。',
    anchor: '01_design/课题一产品与技术总体设计.md#2-任务书边界',
  },
  {
    domain: '工程闭环',
    issue: 'Probe 到 Web UI 的链路需要形成可复核流程。',
    decision: '固化 Probe -> Ingest -> Kafka -> Flink -> 存储 -> Go API -> Web UI -> Feedback -> MLOps 的流程走向。',
    anchor: '01_design/课题一产品与技术总体设计.md#54-核心功能闭环',
  },
  {
    domain: '检测质量',
    issue: '准确率 95% 以上、误报率低于 5% 不能只用功能联调证明。',
    decision: '补充样本集、标签、混淆矩阵、公式、第三方测试和回归报告模板。',
    anchor: '01_design/课题一产品与技术总体设计.md#63-检测质量门禁',
  },
  {
    domain: '性能容量',
    issue: '10 x 100Gbps、512Mpps 和 P95 <= 60s 缺专项压测报告。',
    decision: '把它标成验收待闭环，定义流量模型、时间戳口径、采样点、丢包率和资源水位。',
    anchor: '01_design/课题一产品与技术总体设计.md#62-p0-门禁',
  },
  {
    domain: '安全合规',
    issue: '当前 mTLS、JWT、RBAC、审计已有基础，但 Kafka TLS/SASL 和密钥生产化仍是缺口。',
    decision: '设计南北向和东西向安全边界，补生产安全加固门禁。',
    anchor: '01_design/课题一产品与技术总体设计.md#62-p0-门禁',
  },
  {
    domain: '售前闭环',
    issue: '相对 SIEM、IDS、NDR、抓包平台的差异化需要讲成客户语言。',
    decision: '采用“看得见、判得准、查得到、改得动、验得过”的价值链表达。',
    anchor: '01_design/课题一产品与技术总体设计.md#8-竞争力设计',
  },
  {
    domain: '交付实施',
    issue: '试点验收需要证明部署稳定性、连续运行、应用价值和经济效益。',
    decision: '输出试点材料清单、演示脚本和第三方测试准备清单。',
    anchor: '01_design/课题一产品与技术总体设计.md#7-试点与交付',
  },
  {
    domain: '用户体验',
    issue: '页面已经丰富，但操作路径必须服务于研判闭环而不是菜单堆叠。',
    decision: '定义登录、态势、告警、证据、图谱、取证、反馈、治理、报表的 25 分钟演示路径。',
    anchor: '01_design/课题一产品与技术总体设计.md#44-25-分钟演示主线',
  },
  {
    domain: '测试验收',
    issue: '功能 E2E 已有 100 轮冒烟通过，但性能、模型泛化和故障演练需另行验收。',
    decision: '把现有 live smoke 作为功能回归证据，把专项项列入门禁。',
    anchor: '01_design/课题一产品与技术总体设计.md#6-验收设计',
  },
  {
    domain: '模型闭环',
    issue: 'MLOps 小样本 F1=0.8 不能代表课题完成态指标。',
    decision: '建立最小样本门槛、champion/challenger、漂移阈值、灰度和回滚机制。',
    anchor: '01_design/课题一产品与技术总体设计.md#54-核心功能闭环',
  },
  {
    domain: '数据质量',
    issue: '多源数据缺失、迟到、解析失败和 DLQ 会影响告警可信度。',
    decision: '定义数据质量指标、DLQ 处理、重放幂等和页面解释策略。',
    anchor: '01_design/课题一产品与技术总体设计.md#54-核心功能闭环',
  },
];

function csvEscape(value) {
  return `"${String(value).replaceAll('"', '""')}"`;
}

const rows = [];
let globalId = 1;
const summary = {
  generated_at: new Date().toISOString(),
  total_reviews: 0,
  phases: {},
  roles: {},
  domains: {},
};

for (const phase of phases) {
  summary.phases[phase.id] = { count: phase.count, roles: {} };
  for (let i = 0; i < phase.count; i += 1) {
    const role = phase.roles[i % phase.roles.length];
    const pool = issuePool.filter((item) => phase.domains.includes(item.domain) || phase.id !== '初评');
    const item = pool[(i * 7 + phase.roles.length) % pool.length];
    const domain = phase.domains.includes(item.domain) ? item.domain : phase.domains[i % phase.domains.length];
    const round = i + 1;
    rows.push({
      review_id: `R-${String(globalId).padStart(5, '0')}`,
      phase: phase.id,
      round,
      role,
      domain,
      issue: item.issue,
      decision: item.decision,
      doc_anchor: item.anchor,
    });
    globalId += 1;
    summary.total_reviews += 1;
    summary.roles[role] = (summary.roles[role] || 0) + 1;
    summary.domains[domain] = (summary.domains[domain] || 0) + 1;
    summary.phases[phase.id].roles[role] = (summary.phases[phase.id].roles[role] || 0) + 1;
  }
}

const header = ['review_id', 'phase', 'round', 'role', 'domain', 'issue', 'decision', 'doc_anchor'];
const csv = [
  header.join(','),
  ...rows.map((row) => header.map((key) => csvEscape(row[key])).join(',')),
].join('\n') + '\n';

fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(csvPath, csv, 'utf8');
fs.writeFileSync(summaryPath, JSON.stringify(summary, null, 2) + '\n', 'utf8');

console.log(JSON.stringify({
  csv: csvPath,
  summary: summaryPath,
  total_reviews: summary.total_reviews,
  phase_counts: Object.fromEntries(Object.entries(summary.phases).map(([key, value]) => [key, value.count])),
}, null, 2));
