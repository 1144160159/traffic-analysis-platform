# traffic-analysis-platform Agent Guide

本文档是本仓库的精简工作指南。详细语言规范见 `rules/`，共享 SQL/YAML/Shell 资源见 `common/`，K8s 部署见 `deployments/`。

## 1. 系统总览

主链路:

`probe-agent -> Kafka -> Flink jobs -> ClickHouse/Postgres/OpenSearch/NebulaGraph/Redis/MinIO -> Go control-plane APIs -> Web UI`

子系统:

| 子系统 | 路径 | 重点 |
|------|------|------|
| Go 控制面 | `go/control-plane` | auth, alert, rule, asset, graph, forensics, ingest 等 API 服务 |
| Rust 探针 | `rust/probe-agent` | AF_XDP/AF_PACKET/PCAP 采集, 流聚合, PCAP 归档, DNS/DHCP/ARP 解析 |
| Flink 作业 | `java/flink-jobs` | session, feature, rule, pcap-index, cep, behavior, alert-generator, log, user-behavior |
| Web UI | `web/ui` | Vite + React + Ant Design + ECharts, 真实 API 优先, MSW 仅 `VITE_USE_MOCK=true` |
| Proto | `proto/traffic/v1` | 跨语言契约真源 |
| MLOps | `mlops` + `deployments/kubernetes/argo-events` | Argo Workflows, 模型训练/注册/激活, Flink 热更新 |
| 公共资源 | `common` | CH/PG DDL, Kafka topic, Redis key, OpenSearch/Nebula 配置 |
| 部署 | `deployments` | K8s 清单, APISIX 路由, init jobs, workload 配置 |

核心数据:

- ClickHouse: `flows_raw`, `sessions`, `alerts`, `evidence`, `campaigns`, `pcap_index`, `device_logs`, `user_events`, `dlq_events` 等 OLAP 表, 使用 ReplicatedMergeTree + Distributed。
- PostgreSQL: tenants, users, api_tokens, probes, assets, rules, models, deployments, audit_logs 等元数据表。
- Kafka: `flow.events.v1`, `session.events.v1`, `detections.v1`, `alerts.v1`, `pcap.index.v1`, `asset.bindings.v1`, `device.logs.v1`, `user.events.v1`, `rule.updates`, `model-updates`, `alert.feedback.v1`, `dlq.v1` 等。

## 2. 工作原则

- 先看 `git status --short`，只改当前任务需要的文件。仓库经常有大量未提交或生成产物变更，不要回滚他人改动。
- 优先沿用现有模式: Go `api/service/repository/config`, Rust 模块边界, Flink module POM/job wiring, Web UI Ant Design/React Query 约定。
- `proto/traffic/v1` 是契约真源。除非任务明确要求检查生成物，不要手改 Go/Rust/Java protobuf 生成文件。
- 变更 Kafka topic、DB schema、MinIO 路径、mTLS 证书、环境变量或配置键时，必须同时检查生产者、消费者、部署清单和 init scripts。
- 凭证只从 K8s Secret 或环境变量读取，禁止写入文档、日志或代码。
- Live 验证优先真实链路。不要用 mock 替代 DB/Kafka/API 集成测试。

## 3. 运行环境基线

K8s:

- 2 节点集群: `8-2tb` + `zeus-server`。
- 常用命令需要清理代理:
  `env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl ...`
- 对 NodePort/localhost 做 HTTP 验证时用 `curl --noproxy '*' ...`。

关键 NodePort:

| 组件 | NodePort |
|------|:--:|
| APISIX Gateway | `30180` |
| Kafka | `30092` |
| ClickHouse HTTP / Native | `30023` / `31609` |
| PostgreSQL | `30032` |
| Redis / Sentinel | `30079` / `30279` |
| OpenSearch | `30020` |
| MinIO | `30000` |
| Keycloak | `30080` |
| Argo Server | `30046` |
| StreamPark | `30100` |

ClickHouse:

- 集群模式为 2 shard x 2 replica + 3 Keeper。
- Keeper 必须使用 PVC, 禁止回退到 `emptyDir`。当前基线: `data-clickhouse-keeper-{0,1,2}` 绑定 `clickhouse-keeper-pv-{0,1,2}`, `20Gi`, `local-hdd`。
- CH Pod 使用 init container 写入 `cluster.xml`, `docker_related_config.xml`, `macros.xml` 到共享卷。禁止用 sidecar 写配置。
- `remote_servers` 必须包含 `<user>default</user>` 和 `<password>`。
- `is_local=0` 是已知限制, 不单独判为故障。
- `traffic.sessions` 的 Distributed 短队列可短暂出现。持续增长时优先检查 Flink Session sink 是否退化为单条 INSERT。

Flink:

- 当前 live Job 以 RUNNING job、latest checkpoint 和 exceptions 为空为准；REST 中历史 failed/canceled job 不代表当前故障。
- Flink UI live 入口可能是 ClusterIP, 不要仅凭本机 `127.0.0.1:30082` 失败判定异常。
- 当前 checkpoint 主路径为 MinIO 大盘 `s3://flink-checkpoints/checkpoints/...`，savepoint 路径为 `s3://flink-checkpoints/savepoints/...`；本地 SSD `/home/k8s-data/flink/rocksdb` 只保留 RocksDB 热状态。

APISIX:

- 路由文件: `deployments/kubernetes/configmaps/apisix-routes.yaml`。
- Standalone YAML 要求 `upstream.nodes` 展开格式, 节点权重为整数。
- ConfigMap 用目录挂载 + 启动脚本 copy, 避免 subPath inotify 失效。
- UI 前缀通常需要 `/prefix` 和 `/prefix/*` 两条路由。
- 业务 API 路由目前包含 `/api/v1/encrypted-traffic*`, `/api/v1/fusion*`, `/api/v1/baselines*`, `/api/v1/compliance*`, `/api/v1/topics*`, `/api/v1/audit*`, `/api/v1/feedback*`, `/api/v1/tokens*`。

## 4. 开发规范

Go:

- 目录遵循 `internal/<domain>/{api,service,repository,config,health}`。
- handler 不写业务逻辑；统一使用项目 error types；日志带 `trace_id`/`tenant_id`。
- 禁止 panic、忽略 error、硬编码配置和拼接 SQL。

Rust:

- 模块边界: `capture`, `parser`, `aggregator`, `archiver`, `sender`。
- 使用 `anyhow::Result` + `thiserror`；异步用 tokio。
- 采集路径注意零拷贝、批量发送、分区流表和 backpressure。
- 非测试代码禁止 `unwrap()`、阻塞 async context、硬编码缓冲区。

Java/Flink:

- 每个 operator 设置稳定 `uid`。
- 必须配置 state TTL, RocksDB state backend, checkpoint 30-60s, timeout <= 10min。
- ClickHouse sink 必须批量 `addBatch/executeBatch`; 禁止每条记录新建连接单条 INSERT。
- Session Job 写 `traffic.sessions` Distributed 表, 不直接写 `sessions_local`。
- 线上热修可用 `mvn -pl flink-session-job -am package -Dmaven.test.skip=true`；历史 MiniCluster `testCompile` 问题不阻塞紧急修复。

TypeScript/Web:

- API 统一封装在 `services/api.ts`; React Query 处理 loading/error。
- 组件内禁止直接 `fetch`、硬编码 API 地址、忽略 loading/error、随意使用 `any`。
- 运行时 mock 只能在 `VITE_USE_MOCK=true` 下启用。
- 生产 Web UI nginx 代理指向 `apisix.gateway.svc:9080`; 本地 Vite 开发代理可指向真实 APISIX `10.0.5.8:30180`。
- 桌面端 Modal 必须保持小尺寸，不得铺满或遮住整个浏览器业务区域；业务详情、证据和日志优先使用右侧窄 Drawer，并保持宿主页面上下文可见。验收专用 focus 路由不代表生产弹层形态。

Proto:

- 消息 PascalCase, 字段 snake_case, 字段必须有注释。
- 修改后运行 `cd proto && buf lint && ./scripts/generate.sh`，再检查至少一个受影响语言消费者。

## 5. 验证选择器

使用最小相关检查:

| 范围 | 命令 |
|------|------|
| Go | `cd go/control-plane && go test ./...` 或 `go build ./cmd/<service>` |
| Rust | `cd rust/probe-agent && cargo test --workspace` 或 `cargo build --release --workspace` |
| Java/Flink | `cd java/flink-jobs && mvn test` 或 `mvn -pl <module> package` |
| Web UI | `cd web/ui && npm run test` 或 `npm run build`; 浏览器变更用 Playwright/Chrome 验证 |
| Proto | `cd proto && buf lint && ./scripts/generate.sh` |
| K8s YAML | `kubectl apply --dry-run=client -f <files>` |

DB/集成测试:

- 所有 DB 集成测试基于 K8s live 集群或明确的测试容器执行，禁止用 mock 伪造成功。
- ClickHouse 密码只从 `traffic-credentials` Secret 读取。
- 常用 CH 健康口径: Keeper 三台 `ls /clickhouse/tables` 返回 `01 02`; 四个 CH Pod `readonly=0`, `zk_errors=0`; Distributed 队列可 flush。

前端浏览器冒烟 (Codex Desktop):

- 使用 `codex-desktop-iab-bridge` 时，只调用 `codex-desktop-node-repl` 的封装工具。
- 当前目标浏览器是 VSCode 所在 Windows 机器 `LongShine@10.3.6.59` 的本地 Chrome extension 后端，必须先通过反向隧道把 Linux 本地服务映射到 Windows 本机地址。
- 正式 UI 截图/交互验收使用 Windows 本机隧道 URL：`http://127.0.0.1:25173` 为 Vite UI，`http://127.0.0.1:25174` 为证据接收器，`http://127.0.0.1:25175` 为 smoke redirect helper。
- 打开登录页必须用 `desktop_chrome_open_url(url="http://127.0.0.1:25173/login", keep=true)`。
- 目标浏览器 type 必须是 Chrome extension，禁止切换或回退到 `iab`。
- 禁止手写 `browser-client` / `tabs.new` / `goto`，禁止提交登录表单。
- 若存在残留 `about:blank`，调用 `desktop_chrome_cleanup_blank_tabs` 清理。

浏览器/API 回归判定:

- 真实 APISIX/API 请求无 4xx/5xx。
- 无 `requestfailed`。
- 无非 warning 的 console/pageerror/runtime exception。
- 登录页轻量目标: `http://127.0.0.1:25173/login`，标题应为 `园区网络全流量采集与分析系统`。

## 6. 部署与镜像

- 本环境常用本地 containerd 导入双节点后滚动，不依赖外部 registry。
- Docker Hub 不可用时，Web UI 可用 `web/ui/deployments/Dockerfile.overlay` 基于上一版生产镜像叠加本地 `dist`，但必须以 `npm run build` 产物为准。
- 基础设施拓扑、PV、StorageClass、CNI、证书体系不应由应用代码随意修改。
- APISIX、服务镜像和 init job 变更要同步部署清单，避免 live 与仓库漂移。

## 7. 当前状态摘要

截至 2026-06-19 的仓库记录与本轮验证:

- 2026-06-19 本轮验证前工作树已存在大量未提交内容：152 个已跟踪修改、102 个已跟踪删除、302 个未跟踪条目；本轮只读测试和文档/记忆同步不回滚、不归因这些既有变更。
- 2026-06-19 读取并对齐远端 Codex/agent 上下文：`/root/.codex/skills/traffic-platform/SKILL.md`、`agent.md`、`.claude/skills/traffic-platform.md`、`tests/run_tests.sh` 和 `Makefile`。
- 2026-06-19 `tests/run_tests.sh full` 通过：Go `go test ./...`、Web `npm run lint:check && npm run build && npm run test -- --run`、Java/Flink `mvn test`、Rust `cargo test --workspace`、Proto `buf lint && ./scripts/generate.sh` 均成功；日志 `/tmp/traffic-test-full-20260619081419.log`。已知非阻断项：Web ESLint 9 个 warning、Vite 大 chunk warning、Rust unused/dead-code warnings。
- 2026-06-19 `make python-test` 通过：MLOps `scripts/test_mlops.py` 共 17 passed，8 个 Pandas4Warning；日志 `/tmp/traffic-python-test-20260619081646.log`。Shell 启动时出现 `/etc/bashrc` 的 `BASHRCSOURCED` 未绑定变量提示，但 pytest 命令成功退出。
- 2026-06-19 `tests/run_tests.sh live` 以 `ROUNDS=100 LOG_DIR=/tmp/traffic-e2e-20260619082043` 通过真实 APISIX/API/DB 链路：`7014/7014` checks passed、0 failed、耗时 488s；summary `/tmp/traffic-e2e-20260619082043/live-100-round-smoke-20260619082043-4085535-summary.json`。

- `traffic-analysis` 业务 Pod 全部 Running；`probe-agent` DaemonSet 2/2 Ready。
- ClickHouse Keeper PVC 迁移已完成，ReplicatedMergeTree 只读问题已恢复。
- Flink 9 个 live 业务作业 RUNNING，checkpoint 持续完成。
- Web UI 主页面入口已切真实 API，Chrome/CDP 全路由巡检无 4xx/5xx、request failed 或运行时异常。
- OIDC 回调、登录验证码、PCAP completed 下载、资产发现、Probe-Ingest mTLS、Probe S3 multipart 归档、告警反馈、MLOps 手动重训与模型注册/激活均已通过真实链路验收。
- 仍需专项报告的边界: 10 x 100Gbps 采集压测、端到端 P95 <= 60s 压测、MLOps 模型泛化质量、生产 Kafka SASL/TLS 和 ExternalSecret 加固。

关键历史:

- 2026-06-11: APISIX standalone 修复 subPath inotify、SPA 绝对路径和 YAML 格式问题。
- 2026-06-16: CH 集群配置改为 Keeper DNS + init container 写配置 + 节点间认证，9 张 ReplicatedMergeTree/Distributed 表验证通过。
- 2026-06-17: CH Keeper 从 `emptyDir` 漂移修复为 PVC；Session sink 改为真正批量写入；专题页面和主页面入口补齐真实 API；MLOps 完整训练/评估/注册/激活链路完成。
- 2026-06-19: 完成本轮全项目验证：`tests/run_tests.sh full`、`make python-test`、`ROUNDS=100 LOG_DIR=/tmp/... tests/run_tests.sh live` 均通过；live smoke 为 7014/7014 checks passed。

Community ID 跨语言固定向量:

```text
10.0.0.1->10.0.0.2:12345->80 TCP      => 1:CpuULklTENbGdRpvp7gNcQd5ZqA=
192.168.1.1->192.168.1.100:443->54321 => 1:yvabNgZAlWzo8wcUZ6B9cSRJQ9Q=
10.0.0.1->10.0.0.2:53->12345 UDP      => 1:JrhaqgS2mu6o+Lu2/yWyT0ECe6E=
::1->::2:8080->9090 TCP                => 1:/Q8HrtOQusOw7LFS4Ju3LeGLJu0=
```

## 8. 变更检查清单

- [ ] 改动是否局限于当前任务？
- [ ] Proto 变更是否重新生成并检查消费者？
- [ ] Kafka topic、DB schema、ConfigMap/Secret、环境变量是否同步部署清单？
- [ ] API 是否走 `services/api.ts` 或后端既有 service/repository 层？
- [ ] 错误处理、日志、鉴权、tenant 边界是否符合现有模式？
- [ ] 批量写入、State TTL、缓存 key、索引或 N+1 查询是否检查过？
- [ ] 使用了最小相关测试，并记录无法运行的检查？
- [ ] 前端真实链路是否确认无 4xx/5xx、request failed、非 warning console/pageerror？
