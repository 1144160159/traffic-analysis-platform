# T-MINIO-002/003/004 对象治理实施与回滚手册

## 1. 当前结论

本手册只证明仓库候选的 bucket、生命周期、安全和恢复基线已经建立，不代表生产已应用。当前合同固定为 `status=implementing`、`production_applied=false`，G2 至 G8 继续开放。

权威合同为 `contracts/minio/object-governance.v1.json`，覆盖：

- T-MINIO-002：对象类型生命周期、引用与删除协议。
- T-MINIO-003：逐服务身份、TLS、最小 policy、拓扑和下载权限。
- T-MINIO-004：bucket registry、版本锁、复制、scrub 和恢复。

## 2. 已实现的仓库护栏

1. 登记 `pcap-archive`、`report-artifacts`、`traffic-models`、`flink-checkpoints`、`argo-artifacts` 五个 bucket，覆盖 PCAP、证据、报告、导出、模型、checkpoint、savepoint 和 Argo artifact 八类对象。
2. 每个 bucket 固定 owner、key 规则、manifest 权威源、版本、Object Lock、legal hold、生命周期、加密、复制、配额、policy 和恢复优先级。
3. 删除协议固定为墓碑、引用检查、合规检查、受限审批、对象版本删除、审计和对账；业务应用不得直接删除。
4. 模型注册脚本不再回退共享管理员凭据；缺少 endpoint、凭据、bucket 或显式安全模式时失败。
5. 模型注册脚本不得自动创建 bucket，bucket 必须由受控 bootstrap 预创建。
6. 受控 bootstrap 已显式创建登记的五个 bucket，并在初始化结束前逐一执行 `mc stat`；运行时模型和 Argo 凭据不得承担建桶职责。
7. 四份训练 Workflow 均显式声明 MinIO endpoint、Secret 引用、bucket 和安全模式；明文仍作为当前迁移缺口保留，不能解释为目标完成。
8. verifier 拒绝伪关闭、生产已应用声明、对象类型遗漏、bootstrap bucket 漏项、凭据默认值、`secure=False` 字面量和应用侧 `make_bucket` 回归。
9. `contracts/minio/tls-material.v1.json` 与 `deployments/kubernetes/security/minio-tls-material.v1.yaml` 已固定服务端证书三件套、八个 DNS SAN 和三个客户端 CA 命名空间；当前只允许材料 expand，`cutover_ready=false`。
10. `contracts/minio/tls-cutover.v1.json` 与默认关闭 overlay 已把 MinIO server/proxy、Go、Rust、Flink、Python、Argo 和运维任务共 14 个组件纳入同一切换/回滚边界；五个 `linux/amd64` 客户端候选镜像已在本机完成构建并绑定组件源码哈希。

## 3. 当前明确缺口

- 正常部署路径仍使用 HTTP；TLS bundle 保持默认关闭，尚未应用生产。
- 五个候选镜像只有本地 Docker image ID，尚无 registry 签名、repo digest、双节点分发和批准窗口内的 live 运行证据。
- 仓库生命周期只覆盖 `pcap-archive` 和 `report-artifacts`。
- versioning、Object Lock、legal hold、复制、配额和服务端加密尚未应用。
- `minio-proxy` 只有一个副本且没有 TLS 身份。
- 五个 bucket 的受控 bootstrap 仅存在于仓库候选，尚未应用并与 live MinIO 精确对账。
- live bucket/policy exact diff、凭据轮换、scrub、隔离恢复和跨区域恢复均缺失。

## 4. 迁移顺序

1. Expand：创建逐服务身份、TLS 双信任、bucket/prefix policy、版本与保留配置；不切换业务流量。
2. Inventory：只读采集 bucket、版本、lifecycle、policy、quota、replication、encryption 和对象数量/字节，不读取对象正文或输出凭据。
3. Shadow：新身份先做 HEAD/LIST 负例与受限写入；旧身份保持兼容。
4. Scrub：按 tenant、bucket、prefix、时间和最大对象数执行 manifest—object 对账，输出 missing、extra、version、size、hash、retention 和 temporary 差异。
5. Cutover：按 model、report、PCAP、Argo、Flink 的低风险到高风险顺序串行切换；每次只切一个身份和一个 prefix。
6. Contract：共享凭据活跃连接清零后撤销旧 policy；保留回滚所需双信任和旧只读路径至观察窗结束。

## 5. 停止扩大条件

出现任一情况立即停止：

- 对象或 manifest 丢失、hash/size/version 不一致扩大。
- 跨 tenant 或越 prefix 权限成功。
- 预签名 URL 超过批准 TTL 或未绑定权限。
- versioning/Object Lock/retention 与合同不一致。
- checkpoint/savepoint 清理互相污染。
- 复制滞后、容量、错误率或 P99 超预算。
- 任一服务只能依靠 root/shared credential 恢复。

## 6. 回滚

回滚只恢复路由、身份和读写选择，不删除新对象、版本、manifest 或审计：

1. 停止新身份扩大，保留失败 run 和差异清单。
2. 将 writer 切回上一批准身份与 endpoint；新身份改为只读或禁用。
3. 保持新旧 CA 双信任，禁止通过关闭证书校验回滚。
4. 对在途 multipart、临时 key、outbox 和 manifest 执行有界对账。
5. checkpoint/savepoint 使用登记的旧路径与 artifact 恢复；不得清空 bucket。
6. 只有业务 oracle、manifest/object hash 和授权负例恢复后才结束回滚。

## 7. 验收证据

- G0：完整 alignment、全仓和 Python/MLOps 三项。
- G1：`verify_minio_object_governance.py`、`verify_minio_service_identities.py`、`verify_minio_tls_material.py`、`verify_minio_tls_cutover.py`、镜像元数据采集器与对应负向测试。
- G2：真实 TLS、逐服务身份、policy allow/deny、版本和生命周期读取。
- G3：PG/CH manifest 与对象 missing/extra/version/size/hash/retention 对账。
- G4：固定对象规模、并发、冷热缓存下的 P50/P95/P99、吞吐和资源。
- G5：Windows Chrome 受控下载、对象不可用/过期/部分结果；节点、磁盘、代理、凭据和区域故障。
- G6：灰度、回滚、旧身份撤销和 T+0/T+1/T+3/T+7 观察。
- G7：独立 QA/SRE/安全签字；G8 仍独立判定。
