首先阅读该项目的整体代码，基于doc中的业务文档设计及代码中已提供的接口进行web前端的开发（只开发页面），要使用假数据完成页面验证工作，请学习example中图片样式设计UI风格。
基于 doc/ 产品设计文档与现有 api.ts 接口定义，在 web/ui 中完成全量页面开发：采用「大屏深色可视化 + 深色后台管理」混合 UI 风格（参考 example BigDataView 模板），并通过 MSW 假数据层实现无后端本地验证。
建立 MSW mock 层：fixtures + handlers + browser worker，VITE_USE_MOCK 开关，WebSocket 模拟推送
深色主题 token、global.css、MainLayout/ScreenLayout/PageContainer 改造，Login 页
Dashboard 增强 + SituationalScreen 全屏态势大屏（ECharts 地图/趋势/滚动告警）
AlertList/AlertDetail 深色 UI + 证据卡片/Timeline/TP-FP 反馈交互
CampaignList 增强、TopicPanels 新增、GraphExplorer 换 ECharts 力导向图
RuleManagement 增强、DeploymentManagement/ProbeManagement/WhitelistManagement/AuditLog 新页
ForensicsPage PCAP 任务 mock 流程、Settings 各 Tab mock 持久化
路由/菜单补全、npm run build + dev 全页面验证、测试路径更新