# F-COMMON-001 Feature Contract 注册表运行手册

## 当前结论

`contracts/alignment/feature-contract-registry.v1.json` 是功能合同覆盖、owner、工作包、接受用例、回滚标识和合同hash的机器可校验注册表。它是构建与发布门禁，不是生产运行时依赖；注册表服务或生成器故障不得影响在线请求。

当前仓库有54个canonical功能ID，其中38个属于24周标准范围。已有24份正式合同且当前轻量语义校验通过，16个P0功能全部有合同；14个标准范围P1和16个backlog功能仍缺少正式合同。因此本项只能标记 `IMPLEMENTING/PARTIAL`，不得把“ID已登记”写成“合同已冻结”。

## 权威和变更流程

1. `canonical-registry.json` 只决定canonical ID、优先级和主工作包；不得从文件名或页面标题反推ID。
2. `work-packages.json` 决定唯一Accountable、接受用例和回滚手册ID；同一ID不得由两个工作包主责。
3. `features/<feature_id>.json` 定义UI、API、领域、数据、权限、性能、验收、发布和兼容语义。
4. OpenAPI是REST真源，`proto/traffic/v1`是gRPC/Kafka消息真源，版本化migration是Schema真源；合同只引用这些真源，不复制成另一个可独立漂移的定义。
5. 前端只消费生成类型。手写DTO、fixture和adapter不得增加合同之外的生产能力。
6. 新增或修改合同后先运行构建器，再执行验证器、OpenAPI、scope、compatibility diff和相关领域测试。

## 合同最低内容

每个功能必须明确：用户结果和非目标、保留路由/动作/API、UI状态、operation ID、同步/异步边界、状态机和补偿、权威数据与投影、tenant来源和scope、负向测试、性能旅程与停止条件、灰度和回滚。异步操作还必须有稳定action ID、幂等键、revision、202受理语义、状态URL和最终效果类型。

合同状态不得领先于证据。`implemented`只说明候选实现，不代表G2真实服务、G3数据对账、G4性能、G5浏览器、G6发布或G7关闭。合同缺失、validation error、重复operation ID、未知canonical ID、owner缺失或兼容删除都会阻断候选。

## 兼容与回滚

本整改期不得删除既有路由、动作、API、响应字段、scope或审计事件。旧参数只允许通过显式兼容层保留，必须登记owner、调用量、复审日期和未来独立退役建议；不得从多个候选字段“哪个有值用哪个”猜测语义。

合同门禁回滚时恢复上一份带hash的注册表和对应生成物，禁止手工复制旧DTO。若新合同导致生成客户端、OpenAPI或migration不一致，停止发布并保留旧兼容路径；不得通过放宽校验、删除contract gap或把未知字段静默忽略来让门禁变绿。

## 验收证据

- 注册表覆盖54个唯一功能ID和唯一Accountable。
- 38个标准范围功能的合同缺口可计算；P0合同缺失必须为零。
- 资产、专题和告警三条试点的合同、路由、动作、operation、scope和回滚引用可追踪。
- 合同hash、候选hash、生成客户端、OpenAPI diff、Proto breaking和migration检查绑定同一候选。
- 真实候选发布后采集contract version、旧客户端调用量、未知枚举/字段、compatibility adapter命中量和页面HAR；当前这些live telemetry仍未实现。
- G7关闭前，标准范围合同缺失必须为零，且独立QA确认没有隐藏兼容删除。G8仍由外部项目门禁判定。
