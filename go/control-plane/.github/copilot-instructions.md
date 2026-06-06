# Copilot 指南 — go/control-plane

目的：帮助 AI 代理快速理解控制平面（control-plane）后端的结构、常见变更点与可执行命令。

速览
- 目录：`cmd/`（每个 service 单独二进制），`internal/`（领域实现），`pkg/`（生成代码如 `pkg/proto`），`scripts/`（实用脚本）。
- 主要服务示例：`ingest-gateway`、`rule-manager`、`auth-service` 等，均位于 `cmd/<service>`。

立即要查的文件
- 服务入口：`cmd/<service>/main.go`（wiring、依赖注入、配置加载）。
- 公共库：`internal/common`、`internal/ingest`、`internal/rules` 等。
- Protobuf 产物：`pkg/proto/`（由 `proto/scripts/generate.sh` 生成，修改 proto 后必须同步）。
- 配置：`config.env`、`scripts/*.sh`（如 `create-kafka-topics.sh`、`generate-certs.sh`）。

常用命令（复制可用）
- 构建单个服务：
  ```bash
  cd go/control-plane && go build ./cmd/<service>
  ```
- 全仓检查：
  ```bash
  ./scripts/check-compile.sh
  ```
- 运行单元测试（整个 control-plane）：
  ```bash
  go test ./...
  ```

代码变更注意点（AI 代理专用）
- 修改 proto：修改 `proto/` 下源文件后，运行 `proto/scripts/generate.sh` 并把生成的代码提交到 `go/control-plane/pkg/proto/`。
- 新增/重命名 Kafka topic、DB 表：同步更新 `scripts/create-kafka-topics.sh` 和 `deployments/docker-compose/init/` 的初始化 SQL/脚本。
- 改动 wiring：如果变更 `main.go` 中依赖注入（例如添加全局 client、otel、metrics），检查所有 `cmd/*` 是否需要相同更改。

测试与调试
- 日志位置：开发模式下使用 `docker-compose logs <service>`；本地二进制会写 `go/control-plane/logs/`。
- 本地集成：使用仓库根 `deployments/docker-compose/docker-compose.yml` 启动整个环境。
- 快速生成测试数据：`scripts/test-data.sql`。

PR/Review 清单（AI 可自动检查）
- Proto 变化是否伴随生成文件更新？
- 新增依赖是否更新 `go.mod` 与 CI 配置？
- 是否影响 Kafka topic 或初始化脚本？

快速示例
- 在 `cmd/ingest-gateway/main.go` 添加新的依赖注入：复制其他服务的 wiring 模式，并在 `scripts/check-compile.sh` 下运行构建验证。

更多上下文：参见仓库根的 `.github/copilot-instructions.md` 获取跨模块数据流与集成点。
