# Copilot 指南 — rust/probe-agent

目的：帮助 AI 代理理解探针（probe-agent）代码组织、运行/测试命令与常见改动风险点。

速览
- 主要位置：`rust/probe-agent/`（`Cargo.toml`、`src/`、`config.yaml`、`scripts/`）。
- 关键模块：`src/main.rs`（启动与 shutdown 管理）、`src/sender/`（上报与重试逻辑）、`src/aggregator/`（聚合逻辑）。

立即要查的文件
- 启动入口：`src/main.rs`（组件注册、shutdown 顺序、metrics）。
- 配置文件样例：`config.yaml`（运行时参数、上报目标、Kafka 相关配置）。
- 测试脚本：`scripts/run_all_tests.sh`（集成测试 harness，可在容器中运行）。

常用命令
- 构建 release：
  ```bash
  cd rust/probe-agent && cargo build --release
  ```
- 运行单元测试：
  ```bash
  cargo test
  ```
- 修复警告（建议）：
  ```bash
  cargo fix --bin "probe-agent" -p probe-agent
  ```

AI 改动注意点
- shutdown/register 模式：`main.rs` 中注册顺序会影响优雅停机，修改注册名称或超时时间前请查阅 `shutdown_manager` 的实现并在集成测试中验证停机行为。
- 上报/重试：`src/sender/retry.rs` 与 `src/sender/grpc_sender.rs` 管理重试策略与批处理，改动需覆盖边界条件（超时、最大重试、持久化失败）。
- 配置驱动：多数行为通过 `config.yaml` 参数控制，优先添加配置项并保持向后兼容。

集成测试
- 启动 mock 服务并运行 `scripts/run_all_tests.sh`。失败时通常会在 `/tmp/probe-test-*.yaml` 生成配置和日志，阅读脚本输出定位断言失败。

常见问题与修复
- 警告（unused_mut 等）：可用 `cargo fix` 自动修复或手动移除多余的 `mut`，但确保不打断预期可变性语义。
- 集成测试失败（例如 Community ID 不唯一）：复现失败的测试场景并使用 pcap/logs 分析五元组和 hashing 路径。
