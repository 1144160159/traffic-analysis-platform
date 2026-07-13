# Copilot 指南 — java/flink-jobs

目的：为 AI 代理说明 Flink 作业的构建、运行、调试流程以及规则/DLQ 的处理要点。

速览
- 模块位置：`java/flink-jobs/` 下每个子模块为独立 Maven 模块（例如 `flink-rule-job`、`flink-feature-job`）。
- 关注点：规则解析、事件聚合、DLQ（死信队列）写入与消费。

立即要查的文件
- `pom.xml`（根级与模块级，依赖管理和 plugin 配置）。
- 具体作业：`java/flink-jobs/flink-rule-job/src/main/java/.../RuleJob.java`（或等效主类）。
- DLQ/配置示例：`java/flink-jobs/flink-rule-job/README.md`。

常用命令
- 构建特定 job：
  ```bash
  mvn -pl java/flink-jobs/flink-rule-job package
  ```
- 在本地 Flink cluster 运行：
  ```bash
  ./bin/flink run -c com.traffic.flink.rule.RuleJob target/flink-rule-job-*.jar
  ```
- 模块单元测试：
  ```bash
  mvn -pl java/flink-jobs/flink-rule-job test
  ```

规则与 DLQ 说明（AI 指令）
- 规则格式通常在模块 README 或 resources 中定义；若处理规则失败，作业会将事件写入 `dlq.rule-job` 主题。AI 改动规则解析时：
  - 更新规则解析器代码并增加单元测试覆盖异常/边界情形；
  - 在 README 更新示例规则与 schema；
  - 验证 DLQ 写入逻辑（可通过本地 Kafka consumer 检查）。

常见修复场景
- 依赖升级（Flink/connector）：确认 `pom.xml` 的兼容性，并在 CI 中运行构建和单元测试。
- 性能调优：针对窗口/并行度调整 `env.setParallelism(...)` 与 checkpoint 配置。

调试建议
- 本地提交 jar 到 Flink standalone 或 local cluster；在 IDE 中断点调试 `RuleJob.main` 的本地运行模式。
- 使用 Kafka 的 Console Consumer 检查输入/输出主题与 DLQ。

PR 检查点
- 是否更新了模块 `pom.xml` 且 CI 通过？
- DLQ/Topic 名是否变更，并同步更新 `deployments` 初始化脚本？
