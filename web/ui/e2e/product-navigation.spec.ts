import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    window.__RUNTIME_CONFIG__ = { ...(window.__RUNTIME_CONFIG__ ?? {}), USE_MOCK: true };
    window.localStorage.setItem('traffic-ui-token', 'e2e-token');
  });
});

test('dashboard, alerts, screen and settings routes render', async ({ page }) => {
  for (const route of ['/dashboard', '/alerts', '/screen', '/settings']) {
    await page.goto(route);
    await expect(page.locator('text=园区网络全流量采集与分析系统')).toBeVisible();
    await expect(page.locator('body')).toContainText(route === '/screen' ? '园区数字孪生拓扑' : route === '/settings' ? '系统设置' : route === '/alerts' ? '告警中心' : '仪表盘');
  }
});

test('dashboard renders operations workbench modules', async ({ page }) => {
  await page.goto('/dashboard');
  await expect(page.locator('text=脱敏运营 KPI')).toBeVisible();
  await expect(page.locator('text=优先级待办队列')).toBeVisible();
  await expect(page.locator('text=采集与数据健康门禁')).toBeVisible();
  await expect(page.locator('text=验收缺口与建议动作')).toBeVisible();
  await expect(page.locator('text=Top Talkers 风险贡献')).toBeVisible();
});

test('topic panel renders three topic modes in one menu page', async ({ page }) => {
  const topics = [
    ['加密隧道专题', '隧道信号雷达'],
    ['数据外传专题', '外传风险信号'],
    ['APT 战役专题', '战役态势信号'],
  ];

  await page.goto('/topics');
  await expect(page.getByRole('heading', { name: '专题面板' })).toBeVisible();
  await expect(page.locator('.taf-sidebar__item.is-active')).toContainText('专题面板');

  for (const [title, signalTitle] of topics) {
    await page.getByRole('tab', { name: title }).click();
    await expect(page.getByRole('heading', { name: '专题面板' })).toBeVisible();
    await expect(page.locator('.taf-topic-title-main')).toContainText(title);
    await expect(page.getByRole('heading', { name: signalTitle })).toBeVisible();
    await expect(page.locator('.taf-topic-kpis .taf-metric')).toHaveCount(6);
    await expect(page.locator('.taf-topic-signal-card')).toHaveCount(4);
    await expect(page.locator('.taf-topic-lane-card')).toHaveCount(5);
    await expect(page.locator('.taf-topic-action-row')).toHaveCount(4);
    await expect(page.locator('.taf-topic-evidence-card')).toHaveCount(6);
  }
});

test('legacy topic deep links redirect into the unified topic panel', async ({ page }) => {
  await page.goto('/topics/apt');
  await expect(page).toHaveURL(/\/topics\?topic=apt$/);
  await expect(page.getByRole('heading', { name: '专题面板' })).toBeVisible();
  await expect(page.locator('.taf-topic-title-main')).toContainText('APT 战役专题');
});

test('not found route renders safe recovery workbench modules', async ({ page }) => {
  await page.goto('/missing/not-found');
  await expect(page.getByRole('heading', { name: '404 页面不存在' })).toBeVisible();
  await expect(page.getByText('请求的页面不可用或已被移除。')).toBeVisible();
  await expect(page.locator('.taf-notfound__fact')).toHaveCount(4);
  await expect(page.locator('.taf-notfound__return-grid a')).toHaveCount(3);
  await expect(page.locator('.taf-notfound__return-grid button')).toHaveCount(1);
  await expect(page.getByRole('heading', { name: '最近可用入口' })).toBeVisible();
  await expect(page.locator('.taf-notfound__entry')).toHaveCount(4);
  await expect(page.locator('.taf-notfound__status-row')).toHaveCount(4);
  await expect(page.getByRole('button', { name: '复制追踪 ID' })).toBeVisible();
  await expect(page.locator('body')).not.toContainText('/missing/not-found');
  await expect(page.locator('body')).not.toContainText('stack');
});

test('probes renders collection management workbench modules', async ({ page }) => {
  await page.goto('/probes');
  await expect(page.getByRole('heading', { name: '探针管理' })).toBeVisible();
  await expect(page.locator('.taf-probes-kpis .taf-metric')).toHaveCount(8);
  await expect(page.getByRole('heading', { name: '部署拓扑' })).toBeVisible();
  await expect(page.locator('.taf-probes-node')).toHaveCount(8);
  await expect(page.getByRole('heading', { name: '探针状态矩阵' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '吞吐与丢包趋势' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '选中探针详情' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '批量运维' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '心跳与日志' })).toBeVisible();
});

test('data quality renders quality gate workbench modules', async ({ page }) => {
  await page.goto('/data-quality');
  await expect(page.getByRole('heading', { name: '数据质量' })).toBeVisible();
  await expect(page.locator('.taf-data-quality-kpis .taf-metric')).toHaveCount(7);
  await expect(page.getByRole('heading', { name: 'Kafka Topic 健康 (Top 10)' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Topic 分区倾斜热力图' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Flink 处理质量概览' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '字段质量矩阵（近 24 小时）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '存储写入质量' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '对账报告（近 24 小时）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '质量异常告警（近 24 小时）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '快速定位' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '质量修复建议' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '快收证据与报告' })).toBeVisible();
});

test('alerts renders triage workbench modules', async ({ page }) => {
  await page.goto('/alerts');
  await expect(page.locator('text=告警队列').first()).toBeVisible();
  await expect(page.locator('text=筛选检索')).toBeVisible();
  await expect(page.locator('text=告警详情')).toBeVisible();
  await expect(page.locator('text=研判时间线')).toBeVisible();
  await expect(page.locator('text=关联告警簇')).toBeVisible();
  await expect(page.locator('text=处置与反馈')).toBeVisible();
});

test('alerts expose backend canonical status options', async ({ page }) => {
  await page.goto('/alerts');
  await page.getByText('全部状态').click();

  await expect(page.getByText('未处理', { exact: true })).toBeVisible();
  await expect(page.getByText('研判中', { exact: true })).toBeVisible();
  await expect(page.getByText('已指派', { exact: true })).toBeVisible();
  await expect(page.getByText('已关闭', { exact: true })).toBeVisible();
  await expect(page.locator('body')).not.toContainText('处理中');
  await expect(page.locator('body')).not.toContainText('已确认');
  await expect(page.locator('body')).not.toContainText('已忽略');
});

test('alert detail renders evidence and response closed loop modules', async ({ page }) => {
  await page.goto('/alerts/AL-20260620-000123');
  await expect(page.getByRole('heading', { name: '告警详情', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '研判摘要' })).toBeVisible();
  await expect(page.locator('.taf-alert-detail-summary-fact')).toHaveCount(12);
  await expect(page.locator('.taf-alert-detail-metrics .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: '资产上下文' })).toBeVisible();
  await expect(page.locator('.taf-alert-detail-asset-card')).toHaveCount(2);
  await expect(page.locator('.taf-alert-detail-timeline-item')).toHaveCount(5);
  await expect(page.getByRole('heading', { name: '攻击阶段轨迹' })).toBeVisible();
  await expect(page.locator('.taf-alert-detail-stage-node')).toHaveCount(5);
  await expect(page.getByRole('heading', { name: '处置与响应' })).toBeVisible();
  await expect(page.locator('.taf-alert-detail-response button')).toHaveCount(5);
  await expect(page.getByRole('heading', { name: '状态流转门禁' })).toBeVisible();
  await expect(page.getByRole('button', { name: /未处理 后端状态机禁止/ })).toBeDisabled();
  await expect(page.getByRole('button', { name: /研判中 当前状态/ })).toBeDisabled();
  await expect(page.getByRole('button', { name: /已指派 允许迁移/ })).toBeEnabled();
  await expect(page.getByRole('button', { name: /已关闭 允许迁移/ })).toBeEnabled();
  await expect(page.getByRole('button', { name: '提交状态变更' })).toBeDisabled();
  await page.getByRole('button', { name: /已指派 允许迁移/ }).click();
  await page.getByPlaceholder(/填写状态变更原因/).fill('确认责任人接手处置');
  await expect(page.getByRole('button', { name: '提交状态变更' })).toBeEnabled();
  await page.getByRole('button', { name: '提交状态变更' }).click();
  await expect(page.getByText(/告警状态已提交/)).toBeVisible();
  await expect(page.getByRole('heading', { name: '反馈与学习' })).toBeVisible();
  await expect(page.locator('.taf-alert-detail-evidence-panel .ant-table-tbody tr.ant-table-row')).toHaveCount(6);
});

test('assets renders inventory workbench modules', async ({ page }) => {
  await page.goto('/assets');
  await expect(page.locator('text=资产台账').first()).toBeVisible();
  await expect(page.getByRole('heading', { name: '资产详情' })).toBeVisible();
  await expect(page.locator('text=风险画像')).toBeVisible();
  await expect(page.locator('text=流量画像')).toBeVisible();
  await expect(page.locator('text=协议分布')).toBeVisible();
  await expect(page.locator('text=关联证据与上下文')).toBeVisible();
});

test('graph renders entity graph workbench modules', async ({ page }) => {
  await page.goto('/graph');
  await expect(page.getByRole('heading', { name: '邻居图谱' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '实体详情' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '路径分析结果' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '查询治理' })).toBeVisible();
  await expect(page.locator('text=关联证据')).toBeVisible();
  await expect(page.locator('text=核心业务服务器').first()).toBeVisible();
});

test('fusion renders data fusion workbench modules', async ({ page }) => {
  await page.goto('/fusion');
  await expect(page.getByRole('heading', { name: '数据源状态' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '多源融合编排（映射与对齐流程）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '融合规则管理（共 26 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '冲突队列（待处理 18 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '融合事件审计（近 50 条）' })).toBeVisible();
  await expect(page.locator('text=冲突处理').first()).toBeVisible();
});

test('baselines renders behavior baseline workbench modules', async ({ page }) => {
  await page.goto('/baselines');
  await expect(page.getByRole('heading', { name: '基线状态机' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /行为分布分析/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '偏离列表（共 42 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '基线版本管理' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '偏离解释' })).toBeVisible();
  await expect(page.locator('text=治理与操作')).toBeVisible();
});

test('campaigns renders campaign workbench modules', async ({ page }) => {
  await page.goto('/campaigns');
  await expect(page.getByRole('heading', { name: '战役列表', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '当前页风险分布' })).toBeVisible();
  await expect(page.locator('.taf-campaign-list-panel h2')).toContainText('战役列表');
  await expect(page.getByRole('heading', { name: '战役阶段视图（ATT&CK）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '当前选中战役' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '影响范围' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '证据完整度' })).toBeVisible();
  await expect(page.locator('text=状态流转')).toBeVisible();
});

test('campaign detail renders campaign storyboard closed loop modules', async ({ page }) => {
  await page.goto('/campaigns/APT-20260619-001');
  await expect(page.getByRole('heading', { name: '战役详情', exact: true })).toBeVisible();
  await expect(page.locator('.taf-campaign-detail-profile-fact')).toHaveCount(10);
  await expect(page.locator('.taf-campaign-detail-metrics .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: '攻击时间轴（从发现到闭环）' })).toBeVisible();
  await expect(page.locator('.taf-campaign-detail-phase-card')).toHaveCount(7);
  await expect(page.locator('.taf-campaign-detail-phase-dot')).toHaveCount(7);
  await expect(page.getByRole('heading', { name: '关联告警（38）' })).toBeVisible();
  await expect(page.locator('.taf-campaign-detail-alerts .ant-table-tbody tr.ant-table-row')).toHaveCount(5);
  await expect(page.locator('.taf-campaign-detail-impact-tab')).toHaveCount(6);
  await expect(page.locator('.taf-campaign-detail-top-asset')).toHaveCount(5);
  await expect(page.locator('.taf-campaign-detail-evidence-check')).toHaveCount(6);
  await expect(page.locator('.taf-campaign-detail-evidence-panel .ant-table-tbody tr.ant-table-row')).toHaveCount(5);
  await expect(page.locator('.taf-campaign-detail-response-step')).toHaveCount(6);
  await expect(page.locator('.taf-campaign-detail-action-row')).toHaveCount(5);
  await expect(page.locator('.taf-campaign-detail-review-row')).toHaveCount(6);
});

test('attack chains renders analysis canvas modules', async ({ page }) => {
  await page.goto('/attack-chains');
  await expect(page.getByRole('heading', { name: '攻击链分析' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '攻击链画布' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'ATT&CK 阶段矩阵' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '路径明细（关键跳转）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '证据锚点' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '处置建议' })).toBeVisible();
  await expect(page.locator('.taf-attack-column').filter({ hasText: '数据外传' }).first()).toBeVisible();
});

test('encrypted traffic renders encrypted analysis workbench modules', async ({ page }) => {
  await page.goto('/encrypted-traffic');
  await expect(page.getByRole('heading', { name: '加密流量' })).toBeVisible();
  await expect(page.locator('.taf-encrypted-kpis .taf-metric')).toHaveCount(7);
  await expect(page.getByRole('heading', { name: '协议分布与趋势' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '指纹分析（Top JA3）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'JA3 分布（流量 vs 会话数）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '隧道检测与异常特征' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '异常隧道列表' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '证据与握手元数据（最新 200 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '外联画像' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '生成与导出' })).toBeVisible();
});

test('forensics renders evidence workbench modules', async ({ page }) => {
  await page.goto('/forensics');
  await expect(page.getByRole('heading', { name: '取证分析' })).toBeVisible();
  await expect(page.locator('.taf-forensics-state > div')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: '取证任务状态机' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '会话复放 (Session)' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '取证任务列表（共 190 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'PCAP 索引（共 1,256 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '证据导出包' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'hash 校验结果（最近 20 条）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '证据完整性' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '审计日志（近 24 小时）' })).toBeVisible();
});

test('rules renders rule lifecycle workbench modules', async ({ page }) => {
  await page.goto('/rules');
  await expect(page.getByRole('heading', { name: '规则管理' })).toBeVisible();
  await expect(page.locator('.taf-rules-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: /规则列表/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: /规则编辑/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '生命周期' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '版本历史' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '样本回放验证（近 7 天）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '命中结果矩阵（近 7 天）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '误报样本 Top5' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '性能影响（近 7 天）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '白名单草案（命中高但低风险）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '相关操作' })).toBeVisible();
});

test('deployments renders deployment management workbench modules', async ({ page }) => {
  await page.goto('/deployments');
  await expect(page.getByRole('heading', { name: '部署管理' })).toBeVisible();
  await expect(page.locator('.taf-deployments-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: '发布清单' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '灰度策略' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '发布健康' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '版本对比 / 变更摘要' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '回滚管理' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '发布证据' })).toBeVisible();
});

test('models renders model management workbench modules', async ({ page }) => {
  await page.goto('/models');
  await expect(page.getByRole('heading', { name: '模型管理' })).toBeVisible();
  await expect(page.locator('.taf-models-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: /模型列表/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Champion / Challenger 状态机' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /数据集与样本/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: /模型指标/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '解释与特征' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '激活与回滚' })).toBeVisible();
});

test('mlops renders orchestration workbench modules', async ({ page }) => {
  await page.goto('/mlops');
  await expect(page.getByRole('heading', { name: 'MLOps 编排' })).toBeVisible();
  await expect(page.locator('.taf-mlops-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: /MLOps 闭环编排 DAG/ })).toBeVisible();
  await expect(page.locator('.taf-mlops-dag-node')).toHaveCount(8);
  await expect(page.getByRole('heading', { name: /反馈样本池/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '训练任务队列' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /评估与门禁/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '注册与发布' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /效果回流/ })).toBeVisible();
});

test('playbooks renders SOAR automation workbench modules', async ({ page }) => {
  await page.goto('/playbooks');
  await expect(page.getByRole('heading', { name: 'SOAR 剧本' })).toBeVisible();
  await expect(page.locator('.taf-playbooks-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: 'A. 剧本列表' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /B. 剧本编排/ })).toBeVisible();
  await expect(page.locator('.taf-playbooks-flow-node')).toHaveCount(8);
  await expect(page.getByRole('heading', { name: /C. 节点配置/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'D. 风险控制' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'E. 执行历史' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /F. 处置效果/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'G. 审计与证据' })).toBeVisible();
});

test('whitelist renders governance workbench modules', async ({ page }) => {
  await page.goto('/whitelist');
  await expect(page.getByRole('heading', { name: '白名单', exact: true })).toBeVisible();
  await expect(page.locator('.taf-whitelist-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: 'A. 白名单列表' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'B. 条件构造器 / 新增白名单草案' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'C. 审批流程状态机' })).toBeVisible();
  await expect(page.locator('.taf-whitelist-approval-step')).toHaveCount(5);
  await expect(page.getByRole('heading', { name: 'D. 命中监控（近7天）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'E. 到期治理' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'F. 反馈关联（从告警到白名单草案）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'G. 影响矩阵 / 来源链路卡' })).toBeVisible();
});

test('compliance renders audit gate workbench modules', async ({ page }) => {
  await page.goto('/compliance');
  await expect(page.getByRole('heading', { name: '合规审计', exact: true })).toBeVisible();
  await expect(page.locator('.taf-compliance-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: 'A. 验收门禁矩阵' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'B. 指标映射追踪表' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'C. 证据包完整度' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /D. 运行报告预览/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'E. 缺口治理看板' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'F. 第三方评测批次' })).toBeVisible();
  await expect(page.locator('.taf-compliance-indicators button')).toHaveCount(6);
  await expect(page.locator('.taf-compliance-batches button')).toHaveCount(5);
});

test('audit log renders trace and evidence workbench modules', async ({ page }) => {
  await page.goto('/audit-log');
  await expect(page.getByRole('heading', { name: '审计日志', exact: true })).toBeVisible();
  await expect(page.locator('.taf-auditlog-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: '日志检索' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /审计日志（共/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '操作详情 / Diff 视图' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '关联链路（从当前操作追溯业务链路）' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '操作时间线' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '导出取证' })).toBeVisible();
  await expect(page.locator('.taf-auditlog-diff button')).toHaveCount(6);
  await expect(page.locator('.taf-auditlog-chain > span')).toHaveCount(6);
  await expect(page.locator('.taf-auditlog-risk button')).toHaveCount(5);
});

test('notifications renders notification governance workbench modules', async ({ page }) => {
  await page.goto('/notifications');
  await expect(page.getByRole('heading', { name: '通知配置', exact: true })).toBeVisible();
  await expect(page.locator('.taf-notifications-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: 'A. 通知渠道健康' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'B. 订阅规则' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'C. 条件构造器' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'D. 升级策略流程' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'E. 模板管理' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'F. 发送历史' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'G. 抑制与静默' })).toBeVisible();
  await expect(page.locator('.taf-notifications-channel-card')).toHaveCount(6);
  await expect(page.locator('.taf-notifications-builder label')).toHaveCount(6);
  await expect(page.locator('.taf-notifications-steps > span')).toHaveCount(5);
  await expect(page.locator('.taf-notifications-templates button')).toHaveCount(4);
  await expect(page.locator('.taf-notifications-history button')).toHaveCount(5);
  await expect(page.locator('.taf-notifications-silence button')).toHaveCount(3);
});

test('settings renders system governance workbench modules', async ({ page }) => {
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: '系统设置', exact: true })).toBeVisible();
  await expect(page.locator('.taf-settings-kpis .taf-metric')).toHaveCount(6);
  await expect(page.getByRole('heading', { name: 'A. 租户与站点' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'B. RBAC 权限矩阵' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'C. API 令牌' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'D. 数据留存策略' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'E. 集成配置健康' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'F. 安全策略与系统参数' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'G. 闭环动作入口' })).toBeVisible();
  await expect(page.locator('.taf-settings-tenant-tree button')).toHaveCount(11);
  await expect(page.locator('.taf-settings-rbac-row')).toHaveCount(5);
  await expect(page.locator('.taf-settings-token-panel .ant-table-tbody tr.ant-table-row')).toHaveCount(5);
  await expect(page.locator('.taf-settings-retention div')).toHaveCount(6);
  await expect(page.locator('.taf-settings-integrations button')).toHaveCount(7);
  await expect(page.locator('.taf-settings-security > div')).toHaveCount(10);
  await expect(page.locator('.taf-settings-loop-actions button')).toHaveCount(9);
});
