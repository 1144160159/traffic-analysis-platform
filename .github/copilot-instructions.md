
# Copilot / AI Agent 指南 — traffic-analysis-platform

目的：让 AI 编码代理在 5–10 分钟内获得可操作上下文，能安全修改与构建主要模块。

要点速览
- 主要组件：`go/control-plane`（后端服务），`java/flink-jobs`（实时流处理），`rust/probe-agent`（探针/采集器），`web/ui`（前端），`deployments/`（本地/集群部署）。
- 典型数据流：probe → Kafka (`feature.stat.v1`) → Flink jobs → ClickHouse / Kafka → 后端服务。

必须优先检查的文件和位置
- Go 服务入口与 wiring：`go/control-plane/cmd/<service>/main.go`。
- 公共域与实现：`go/control-plane/internal/`、`go/control-plane/common/`。
- Protobuf 源与生成：`proto/`；生成脚本 `proto/scripts/generate.sh`；Go 产物位于 `go/control-plane/pkg/proto/`。
- Flink 作业：`java/flink-jobs/*`，尤其 `flink-rule-job`（规则、DLQ、示例配置）。
- 探针：`rust/probe-agent`（`Cargo.toml`、`config.yaml`、`README.md`）。
- 本地集成脚本：`deployments/docker-compose/init/` 与 `go/control-plane/scripts/`（`create-kafka-topics.sh`、`generate-certs.sh`）。

常用开发命令（可直接复制运行）
- 构建单个 Go 服务：`cd go/control-plane && go build ./cmd/<service>`
- 全仓快速编译检查（control-plane）：`go/control-plane/scripts/check-compile.sh`
- 运行 Go 测试：`go test ./...`（在 `go/control-plane` 目录内有更多子模块测试）
- 构建 Flink job：`mvn -pl java/flink-jobs/flink-rule-job package`
- 运行 Flink job（本地 cluster）：`./bin/flink run -c com.traffic.flink.rule.RuleJob target/flink-rule-job-*.jar`
- 构建 Rust probe：`cd rust/probe-agent && cargo build --release`
- 本地集成（容器）：`docker-compose -f deployments/docker-compose/docker-compose.yml up --build`

注意的项目规范与约定
- 每个后端服务为独立二进制，位于 `go/control-plane/cmd/<service>`，init 与 wiring 放在对应 `main.go`。
- 领域代码放 `internal/<domain>`，使用 `api/service/repository` 或类似分层。
- Protobuf 是单一真源：任何 proto 改动后必须运行 `proto/scripts/generate.sh` 并提交相应语言的生成文件。
- 配置与 secret：`go/control-plane/config.env` 与各服务的 `config.yaml`/`application.*` 为主要运行时配置位置。
- 日志与遥测：遵循 `common/logging` 与 `common/otel` 的结构化日志与 trace 约定。

常见集成点（修改时要注意）
- Kafka：主题如 `feature.stat.v1`、`rule.updates`、`detections.v1`、`dlq.rule-job` 在多个模块中被硬编码或配置引用，改名需更新 producer/consumer 与 init 脚本（参见 `deployments/docker-compose/init/init-kafka-topics.sh`）。
- 存储：ClickHouse、Postgres，初始化 SQL 在 `deployments/init/` 与 `go/control-plane/init.sql`。
- 证书：mTLS 相关脚本在 `go/control-plane/scripts/generate-certs.sh`，服务证书放在 `go/control-plane/certs/`。

快速审查清单（提交 PR 前）
- Proto 修改：运行并提交 `proto/scripts/generate.sh` 的输出到对应语言目录（`go/control-plane/pkg/proto/`、`rust/proto-gen/` 等）。
- 依赖变更（mvn/cargo/go mod）：确保构建脚本/CI 能正常运行，更新对应 module README/CHANGELOG。
- 配置与部署：若引入新 Kafka topic、DB 表或外部服务，更新 `deployments/docker-compose/init/` 与文档。

调试与本地故障排查要点
- 查看服务日志：`docker-compose logs <service>` 或控制平面本地 logs 目录 `go/control-plane/logs/`。
- Flink 作业失败：检查作业的 DLQ 主题（`dlq.rule-job`）并查看 `java/flink-jobs/flink-rule-job/README.md` 的示例处理。
- 本地模拟数据：`go/control-plane/scripts/test-data.sql` 和相关脚本可用于填充测试数据。

如果你是 AI 代理要开始工作
1. 先在 `go/control-plane`、`proto/`、`rust/probe-agent` 各读一遍入口文件（`main.go`、`proto/scripts/generate.sh`、`Cargo.toml`）。
2. 若改 proto：运行 `proto/scripts/generate.sh`，提交生成输出并在 CI 上验证构建。
3. 若新增 Kafka topic 或 DB：更新初始化脚本（`deployments/docker-compose/init/`）并在 PR 说明中列出变更。

变更记录与反馈
- 我已将此文件精简为 Agent 可直接使用的要点。若需要我可以将其拆成“按模块的 Agent 指南”（例如单独的 `go/control-plane/.github/copilot-instructions.md`）。

——
如需我把某个子模块（Flink、Rust probe、Go 服务）的细化任务写成 TODO 并逐条实施，请回复要优先处理的模块名称。

