# 全流量采集分析系统 — 业务逻辑文档

> 版本: 2026-06-14 | 基于 agent.md + 代码实现 + doc/ 设计文档

---

## 目录

1. [系统架构总览](#一系统架构总览)
2. [数据采集层](#二数据采集层)
3. [流计算处理层](#三流计算处理层)
4. [检测与分析层](#四检测与分析层)
5. [告警与处置层](#五告警与处置层)
6. [控制面服务层](#六控制面服务层)
7. [图数据库与关联分析](#七图数据库与关联分析)
8. [MLOps 模型生命周期](#八mlops-模型生命周期)
9. [多租户与安全](#九多租户与安全)
10. [性能与可靠性](#十性能与可靠性)

---

## 一、系统架构总览

### 1.1 系统定位

面向园区网络的全流量采集分析系统，基于多源异构数据融合，实现复杂恶意攻击行为的智能检测。

**课题来源**: 泉城省实验室重大项目 QCL20250108
**考核指标**: 预警准确率 ≥ 95%，误报率 < 5%（CNAS 第三方测试）

### 1.2 核心数据链路

```
Probe Agent (Rust, eBPF/AF_XDP)
  → gRPC + mTLS
Ingest Gateway (Go)
  → Kafka
Flink Jobs (Java, 11 modules)
  → ClickHouse / OpenSearch / PostgreSQL / NebulaGraph / MinIO / Loki
Go Control-Plane APIs (8 services)
  → Web UI (React + TypeScript, 22 pages)
```

### 1.3 子系统组成

```
┌────────────────────────────────────────────────────────────────────┐
│                        全流量采集分析系统                             │
├────────────┬────────────┬────────────┬────────────┬────────────────┤
│ 数据采集    │ 流计算      │ 数据存储    │ 控制面      │ 前端            │
│            │            │            │            │                │
│ Probe Agent│ Session    │ ClickHouse │ Ingest GW  │ Dashboard      │
│ (Rust)     │ Feature    │ (OLAP)     │ Auth Svc   │ AlertList      │
│ AF_XDP     │ Rule       │            │ Alert Svc  │ GraphExplorer  │
│ AF_PACKET  │ CEP        │ PostgreSQL │ Rule Mgr   │ ForensicsPage  │
│ PCAP replay│ Behavior   │ (Metadata) │ Graph Svc  │ RuleMgmt       │
│            │ Alert Gen  │            │ Forensics  │ Settings       │
│ Fluent Bit │ Log Job    │ OpenSearch │ Asset Svc  │ AttackChain    │
│ (Syslog)   │ User Bhv   │ (Search)   │ MockIngest │ CampaignList   │
│            │ PCAP Index │            │            │                │
│ Keycloak   │            │ NebulaGraph│ MLOps      │ Login          │
│ (OIDC)     │            │ (Graph)    │ Orchestrator│               │
│            │            │            │            │                │
│ APISIX     │            │ MinIO      │            │ 22 pages      │
│ (Gateway)  │            │ (Object)   │            │ 15 components │
│            │            │            │            │                │
│            │            │ Redis      │            │                │
│            │            │ (Cache)    │            │                │
│            │            │            │            │                │
│            │            │ Loki       │            │                │
│            │            │ (Log)      │            │                │
└────────────┴────────────┴────────────┴────────────┴────────────────┘
```

---

## 二、数据采集层

### 2.1 四源数据采集

系统采集四类异构数据源，对齐实施方案 "多源异构数据融合" 要求。

#### 2.1.1 流量数据 (FlowEvent)

**采集方**: Probe Agent (Rust)
**传输**: gRPC + mTLS → Ingest Gateway → Kafka `flow.events.v1`

| 字段组 | 字段 | 说明 |
|--------|------|------|
| **元数据** | event_id (UUIDv4) | 幂等键 |
| | tenant_id | 租户隔离 |
| | probe_id | 探针标识 |
| | run_id | `realtime` / `replay_<task_id>` |
| | feature_set_id | 特征集版本 |
| | event_ts / ingest_ts | 时间戳(ms) |
| **会话标识** | community_id | SHA1(五元组排序哈希)，双向会话唯一 |
| **五元组** | src_ip, dst_ip, src_port, dst_port, protocol | TCP=6, UDP=17 |
| **方向** | direction | `fwd` / `bwd` / `unknown` |
| **流量统计** | packets_fwd/bwd, bytes_fwd/bwd | 包数和字节数 |
| | pps, bps | 包速率 / 比特率 |
| | duration_ms | 持续时间 |
| **包特征** | pktlen_stats (min,max,mean,std) | 包长统计 |
| | iat_stats (min,max,mean,std) | 包间隔统计 |
| **TCP 特征** | tcp_flags_fwd/bwd | OR-ed TCP flags |
| | tos | DSCP + ECN |
| **会话特征** | active_stats, idle_stats | 活动/空闲期统计 |
| | subflow_count | 子流数量 |

**采集性能**: AF_XDP 零拷贝，单核 ≥ 5Mpps，线速 100Gbps

#### 2.1.2 资产信息 (Asset)

**采集方**: Go Asset Service + Rust Probe ARP/DHCP/DNS 被动发现

| 字段 | 说明 |
|------|------|
| asset_id (UUID) | 唯一标识 |
| ip_address | 当前 IP |
| mac_address | 规范格式 xx:xx:xx:xx:xx:xx |
| hostname | DHCP/DNS/LLMNR 解析 |
| vendor | OUI 厂商 (IEEE 注册) |
| os_type | OS 指纹 (DHCP option 60, HTTP UA) |
| source | `arp` / `dhcp` / `dns` / `lldp` / `snmp` / `manual` |
| vlan_id, switch_port | 网络位置 |
| first_seen, last_seen | 时间戳 |

**发现机制**:
- ARP 流量 → MAC+IP 绑定 + ARP 欺骗检测 (4 类)
- DHCP 流量 → MAC+IP+Hostname + OS 指纹 (12 种)
- DNS 查询 → Hostname↔IP 映射 + DNS 隧道检测
- LLDP/CDP → 交换机邻居拓扑

#### 2.1.3 设备日志 (DeviceLog)

**采集方**: Fluent Bit DaemonSet (K8s)
**传输**: UDP 514 / TCP 6514 (TLS) → Kafka `device.logs.v1`

| 字段 | 说明 |
|------|------|
| log_id | UUIDv4 |
| device_ip | 日志来源设备 IP |
| device_type | `switch` / `router` / `firewall` / `server` |
| facility | Syslog facility (0-23) |
| severity | Syslog severity (0-7) |
| message | 原始日志文本 |
| parsed | 结构化解析结果 (JSON) |
| source | `syslog` / `snmp_trap` / `netflow` |

#### 2.1.4 用户行为 (UserEvent)

**采集方**: Keycloak Event Listener + APISIX Access Log
**传输**: Kafka `user.events.v1`

| 字段 | 说明 |
|------|------|
| user_id | Keycloak user UUID |
| event_type | `login` / `logout` / `token_refresh` / `api_access` |
| source_ip | 客户端 IP |
| resource | 访问资源路径 |
| action | 操作 |
| result | `success` / `denied` / `error` |

### 2.2 采集业务流

```
实时采集: Probe Agent → AF_XDP 抓包 → 协议解析 → 流聚合(100条/100ms)
         → gRPC UploadFlows → Ingest Gateway → Kafka → Flink

离线回放: Argo Workflow 触发 → PCAP 文件 → 速率控制(Original/2x/5x/Max)
         → run_id="replay_<task_id>" → 同实时 Pipeline

资产发现: Probe ARP/DHCP/DNS → Kafka asset.bindings.v1 → Asset Service
         → UpsertAsset(MAC=唯一键) → asset_events 变更审计

日志采集: Fluent Bit DaemonSet → Kafka device.logs.v1 → Flink Log Job
         → Syslog RFC5424/3164 解析 → 关联 Asset Service → Loki + OpenSearch

用户行为: Keycloak Event → Kafka user.events.v1 → Flink User Behavior Job
         → 异地/爆破/提权检测 → OpenSearch 审计索引
```

---

## 三、流计算处理层

### 3.1 Flink 作业全景 (11 个 Job)

| # | Job | 输入 | 输出 | 业务功能 |
|:--:|------|------|------|---------|
| 1 | **Session** | `flow.events.v1` | `session.events.v1` + CH | 流量会话聚合 (community_id, process/window 双模式) |
| 2 | **Feature** | `session.events.v1` | `feature.stat.v1` | L1/L2/L3 统计特征提取 |
| 3 | **Rule** | `session.events.v1` | `detections.v1` | 规则引擎检测 (Suricata/YARA/Sigma 兼容) |
| 4 | **PCAP Index** | `flow.events.v1` | `pcap.index.v1` | PCAP 元数据索引 (Bloom Filter) |
| 5 | **CEP** | `detections.v1` + `alerts.v1` | campaigns | 复杂事件关联 (5 攻击模式) |
| 6 | **Behavior** | `feature.stat.v1` + `model-updates` | `detections.v1` | ML 行为检测 (XGBoost/LightGBM) + 模型热更新 |
| 7 | **Alert Gen** | `detections.v1` | `alerts.v1` + CH | 告警生成 (去重+证据+三写 CH/OS/Kafka) |
| 8 | **Log** | `device.logs.v1` | Loki + OpenSearch | Syslog 解析富化 (RFC5424/3164) |
| 9 | **User Behavior** | `user.events.v1` | `alerts.v1` + CH | 用户异常检测 (异地/爆破/提权) |
| 10 | Feature (补) | `session.events.v1` | `feature.stat.v1` | 特征工程补充 |
| 11 | CEP (补) | `session.events.v1` | campaigns | CEP 关联补充 |

### 3.2 会话聚合 (Session Job)

```
输入: FlowEvent (单向流记录)
处理:
  1. 按 community_id 分组 (双向会话合并)
  2. process 模式: KeyedProcessFunction + 状态 TTL
  3. window 模式: Session Window + 超时淘汰
输出: SessionEvent (完整会话记录)
  - session_id, community_id, 五元组
  - 双向包数/字节数统计
  - TCP 握手信息 (SYN/ACK/RST 计数)
  - 会话持续时间
```

### 3.3 特征提取 (Feature Job)

```
输入: SessionEvent
提取:
  L1 层 (基础统计): pps, bps, up_down_ratio, duration_ms
  L2 层 (包特征): pktlen (min/max/mean/std), iat (min/max/mean/std)
  L3 层 (行为特征): active/idle 期统计, TCP flags 计数
输出: FeatureStat → ClickHouse + Kafka feature.stat.v1
```

---

## 四、检测与分析层

### 4.1 检测体系 (3 层递进)

```
第 1 层: 规则引擎 (Rule Job)
  ├── 阈值规则 (Threshold): pps > X, bps > Y
  ├── Suricata 规则: 兼容 Suricata rule 语法
  ├── YARA 规则: 载荷特征匹配
  └── Sigma 规则: SIEM 检测规则

第 2 层: 行为检测 (Behavior Job)
  ├── 扫描检测: ScanDetectionModel (端口/IP 扫描)
  ├── 隧道检测: TunnelDetectionModel (DNS/HTTP/ICMP 隧道)
  ├── DGA 检测: DGADetectionModel (域名生成算法)
  ├── 加密流量: EncryptedTrafficModel (TLS/JA3/JA3S 指纹)
  ├── C2 通信: C2DetectionModel (心跳/Jitter 检测)
  ├── 数据外泄: DataExfilDetectionModel
  ├── 僵尸网络: BotnetDetectionModel
  ├── 恶意软件: MalwareDetectionModel
  ├── 钓鱼检测: PhishingDetectionModel
  └── 异常检测: AnomalyDetectionModel (XGBoost ML 模型, 热更新)

第 3 层: 复杂事件关联 (CEP Job)
  ├── 端口扫描: N 分钟内 M 个不同目标端口
  ├── 暴力破解: 连续认证失败 + 最终成功
  ├── C2  Beaconing: 周期性外联 + 固定间隔
  ├── 数据外泄: 大量出站流量 + 非工作时间
  └── 横向移动: 内部 IP 间异常通信链
```

### 4.2 协议异常检测 (6 类)

| 检测类型 | 检测内容 |
|---------|---------|
| TCP 标志位异常 | SYN+FIN 同时设置、NULL 扫描、XMAS 扫描 |
| TCP 状态机异常 | 非标准握手序列、半开连接 |
| UDP 异常 | 异常大包、泛洪模式 |
| ICMP 隧道检测 | ICMP payload 熵值分析 |
| IP 分片异常 | 重叠分片、微小分片、分片洪水 |
| 流异常 | 流量突增/突降、非对称流量 |

### 4.3 TLS/JA3 指纹检测

| 检测维度 | 内容 |
|---------|------|
| JA3 指纹 | 10 个已知恶意 JA3 (Cobalt Strike×3, Metasploit×2, Empire, TrickBot, Dridex, Gozi, Emotet) |
| JA3S 指纹 | 服务端指纹黑名单 |
| TLS 版本 | SSLv2/SSLv3/TLSv1.0/TLSv1.1 废弃版本 |
| Cipher Suite | 16 个弱密码套件 (RC4/DES/NULL/anon) |
| SNI 异常 | DDNS 恶意 SNI (8 个后缀), DGA 高熵 SNI (Shannon), 端口不匹配 |

### 4.4 检测结果合并

```
多个检测源 → 统一 DetectionBatch
  Rule Job     → DetectionBatch { source: "rule", rule_id, score, labels }
  Behavior Job → DetectionBatch { source: "behavior", model_version, score, labels }
  CEP Job      → DetectionBatch { source: "cep", pattern_id, confidence }
  
输出: Kafka detections.v1 → Alert Generator → Alert
```

---

## 五、告警与处置层

### 5.1 告警生成 (Alert Generator)

```
输入: DetectionBatch (多源检测结果)
处理:
  1. 去重: MD5 确定性 UUID v3 生成 alert_id
  2. 聚合: 相同 community_id 的多个 detection 合并
  3. 证据: EvidenceBuilder 构建证据包 (flow stats + PCAP 引用 + 关联告警)
  4. 三写: ClickHouse (OLAP) + OpenSearch (检索) + Kafka (通知)
输出: Alert → Kafka alert.events.v1 → Go Alert Consumer
```

### 5.2 告警富化 (Go Alert Consumer)

```
Alert 消费 → 5 维度富化:
  1. 威胁情报: IP/域名信誉查询 + 自动标记 (malicious/suspicious/clean)
  2. GeoIP: 地理位置风险评估 + 不可能旅行检测 (Haversine 距离)
  3. 资产关联: IP→MAC→Hostname→Vendor 映射
  4. 白名单过滤: IP/域名/指纹/子网 匹配 → 自动关闭
  5. 攻击链: Campaign Correlator 多维度关联
```

### 5.3 告警反馈闭环

```
运营人员 → Web UI 标记 TP/FP
  ↓ POST /api/v1/alerts/{id}/feedback
Go Feedback Handler
  ├── ClickHouse: INSERT alert_feedback (TP/FP + reason_code)
  ├── Kafka: alert.feedback.v1 (下游消费)
  ├── Whitelist: FP 自动添加白名单 (90天 TTL)
  └── Stats: FP rate 统计 + FP 排行 API
  ↓
MLOps Orchestrator 监控
  feedback ≥ 500 条? → 触发重训
  FP rate > 15%? → 触发重训
```

### 5.4 告警生命周期

```
detected ──→ acknowledged ──→ triaged ──→ resolved
                │                 │
                └──→ escalated ───┘
                
状态转换:
  detected    → acknowledged (运营认领)
  acknowledged → triaged (分析中: TP/FP/N/A)
  triaged     → resolved (已处置)
  any         → escalated (升级处理)
```

---

## 六、控制面服务层

### 6.1 Go 服务矩阵 (8 服务 + 1 编排器)

| 服务 | 入口 | 核心功能 |
|------|------|---------|
| **Ingest Gateway** | `cmd/ingest-gateway` | mTLS 双向认证、限流(Token Bucket 100K/s)、去重(LRU 10万)、Kafka 分区写入、DLQ |
| **Auth Service** | `cmd/auth-service` | JWT/OIDC (Keycloak)、RBAC (4 角色)、API Token 管理 |
| **Alert Service** | `cmd/alert-service` | 告警查询、反馈处理、统计、威胁情报、GeoIP、白名单、Campaign 关联 |
| **Rule Manager** | `cmd/rule-manager` | 规则 CRUD、版本化、灰度发布(8态状态机)、**Model Registry API (14 端点)**、**MLOps Orchestrator** |
| **Graph Service** | `cmd/graph-service` | 图查询 (ClickHouse/Redis)、图算法 (Louvain/PageRank/Betweenness)、NebulaGraph 双客户端 |
| **Forensics Service** | `cmd/forensics-service` | PCAP 切片下载 (MinIO/S3)、证据包生成 |
| **Asset Service** | `cmd/asset-service` | 资产管理 (MAC→IP)、被动发现消费、OUI 厂商查询 |
| **Mock Ingest** | `cmd/mock-ingest` | 双模式: gRPC Mock + FlowEvent 生成器 (开发/测试用) |
| **MLOps Orchestrator** | `internal/rules/service/` | 自编排引擎: 5 触发条件, Argo Workflow 自动提交 |

### 6.2 规则管理 (Rule Manager)

```
规则生命周期:
  drafted → validated → deployed → active → deprecated → archived

部署状态机 (8 态):
  planned → gray (灰度 10%) → active (全量)
                              → paused (暂停) → active (恢复)
  planned → cancelled
  gray → rolled_back (回滚)
  active → rolled_back → 恢复旧版本
  active → superseded (被新版本取代)
```

### 6.3 威胁情报体系

```
威胁情报来源:
  1. 内置威胁源 (GeoIP+恶意 ASN+已知 IoC)
  2. GeoIP Service: 10 高风险国家、15 恶意 ASN
  3. 不可能旅行检测: Haversine 公式 (同一 user_id 在时间窗口内跨大洲)

威胁评分:
  score = IP_reputation × 0.3 + Geo_risk × 0.2 + ASN_risk × 0.15 
        + behavior_anomaly × 0.25 + correlation_bonus × 0.1
```

---

## 七、图数据库与关联分析

### 7.1 NebulaGraph 数据模型

```
图空间: traffic_graph (vid_type=FIXED_STRING(32))

5 类 Tags (节点):
  ip_address ──── session ──── alert ──── campaign ──── network_device

7 类 Edge Types (关系):
  communicates (IP→IP)       — 通信关系
  has_session (IP→Session)   — IP 与会话关联
  triggers_alert (Session→Alert) — 会话触发告警
  includes_alert (Campaign→Alert) — Campaign 包含告警
  belongs_to (IP→Device)     — IP 归属设备
  connects_to (Device→Device) — 设备连接关系
  attack_path_hop (IP→IP)    — 攻击路径跳步
```

### 7.2 图算法引擎

| 算法 | 用途 | 参数 |
|------|------|------|
| **Louvain** | 社区检测，识别异常通信集群 | modularity > 0.8 标记异常 |
| **PageRank** | 节点重要性排序，识别关键枢纽 | damping factor 0.85 |
| **Betweenness** | 桥梁节点识别，发现单点瓶颈 | 采样优化大图 |
| **Attack Path** | DFS 多路径攻击链搜索 | 最大 10 跳, 100 路径 |
| **Anomaly Pattern** | Star/Chain/Mesh/Isolated 4 类模式 | 结构异常检测 |

### 7.3 Campaign 关联器

```
4 维关联:
  时间维度: 告警时间窗口内关联
  空间维度: 同源/同目的 IP 关联
  行为维度: 相同 attack_type 关联
  社区维度: 图社区检测结果关联

13 阶段 MITRE ATT&CK 映射:
  Recon → ResourceDev → InitialAccess → Execution → Persistence
  → PrivilegeEscalation → DefenseEvasion → CredentialAccess
  → Discovery → LateralMovement → Collection → C2 → Exfiltration

8 类攻击链识别:
  DDoS攻击链、数据窃取链、勒索软件链、C2通信链、
  扫描侦查链、横向移动链、权限提升链、隐蔽隧道链
```

---

## 八、MLOps 模型生命周期

### 8.1 模型训练流水线

```
触发条件 (5 种):
  1. 手动触发: POST /api/v1/mlops/retrain
  2. 定时调度: CronWorkflow 每周日 02:00
  3. 反馈积累: ClickHouse alert_feedback 24h ≥ 500 条
  4. FP 率超标: FP rate > 15% (样本 ≥ 100)
  5. 数据漂移: PSI > 0.25 (持续检查)

Argo WorkflowTemplate (7 步):
  ┌─── preflight ──→ extract ──→ train ──→ evaluate ──→ drift ──→ register ──→ activate ─┐
  │    数据就绪       特征提取    模型训练     模型评估      漂移检测     模型注册        自动激活    │
  └──────────────────────────────────────────────────────────────────────────────────────────┘
                                     │                          F1>0.85?               auto=true?
```

### 8.2 模型注册与版本管理

```
Go Model Registry API (14 端点):
  POST   /api/v1/models                          — 创建模型
  GET    /api/v1/models                          — 模型列表
  GET    /api/v1/models/{id}                     — 模型详情
  PUT    /api/v1/models/{id}                     — 更新模型
  DELETE /api/v1/models/{id}                     — 删除模型
  GET    /api/v1/models/{id}/summary             — 模型摘要
  POST   /api/v1/models/{id}/versions            — 注册版本 (MLOps pipeline 调用)
  GET    /api/v1/models/{id}/versions            — 版本列表
  GET    /api/v1/models/{id}/versions/{v}        — 版本详情
  GET    /api/v1/models/{id}/versions/active     — 激活版本
  POST   /api/v1/models/{id}/versions/{v}/activate   — 激活 (触发 Kafka → Flink 热更新)
  POST   /api/v1/models/{id}/versions/{v}/deprecate  — 弃用

Kafka 通知链:
  Go PublishModelUpdate → Kafka rule.updates (event_type: model_update)
  → Flink ModelUpdateBroadcastHandler (Broadcast State)
  → MinioModelLoader.download(artifactUri)
  → XGBoostModelWrapper → Booster.loadModel()
  → ModelRegistry.hotSwap() → 下次推理生效
```

### 8.3 模型存储

```
MinIO: traffic-models bucket
  models/{version}/
    ├── model.json              # XGBoost 原生 JSON
    ├── feature_columns.json    # 特征顺序 (Flink 推理必须对齐)
    ├── feature_importance.json # Top 10 特征
    └── metrics.json            # F1/AUC/PR/ROC

Flink 本地缓存: /opt/flink/models/{sha256_of_artifact_uri}/
  → SHA256 版本隔离 + LRU 淘汰 (最大 5 个版本)
```

### 8.4 自编排闭环

```
Flink 推理 → Alert → 运营标注 TP/FP → ClickHouse alert_feedback
                                              ↓
                                   MLOps Orchestrator (每 1h)
                                   ├─ checkFeedbackAccumulation
                                   ├─ checkFPRate
                                   ├─ checkDataDrift (PSI × 7 features)
                                   └─ submitArgoWorkflow if met
                                              ↓
                                   Argo Workflow 自动训练+评估+注册
                                              ↓
                                   Kafka → Flink 热更新 → 新模型上线
                                              ↑
                                   (无需人工介入，全自动闭环)
```

---

## 九、多租户与安全

### 9.1 全链路租户隔离

```
tenant_id 贯穿全链路:
  1. Kafka: partition key = tenant_id + community_id
  2. ClickHouse: ORDER BY (tenant_id, timestamp)
  3. PostgreSQL: 每表含 tenant_id 列
  4. OpenSearch: 索引按 tenant 分片 (audit-{tenant_id}-{date})
  5. MinIO: 路径 /{tenant}/{date}/{hour}/{probe_id}/
  6. API: APISIX JWT 提取 tenant_id → Header 注入 → Go Handler 解析
```

### 9.2 RBAC 权限模型

```
角色定义:
  admin    — 所有权限 (*)
  operator — 规则+部署+模型 读写 + 激活
  analyst  — 规则读写 + 部署创建 + 模型查看
  viewer   — 只读 (rule:read, deploy:read, model:read)
  probe    — 机器账户 (rule:read)

权限分组:
  rule:*    — rule:read/write/delete/enable/export/import
  deploy:*  — deploy:read/create/gray/activate/rollback/cancel
  model:*   — model:read/create/write/delete/activate/export/import
  admin:*   — admin:read/write
  audit:*   — audit:read
```

### 9.3 传输安全

```
Probe → Ingest Gateway: gRPC + mTLS (双向证书认证)
APISIX → Backend: TLS 终结 + JWT 验证
Kafka: SASL/SCRAM (生产)
内部存储 (CH/PG/Redis/Nebula): K8s 内部网络隔离
MinIO: TLS + IAM Policy
```

---

## 十、性能与可靠性

### 10.1 性能基线

| 指标 | 目标值 | 实测值 |
|------|--------|--------|
| 探针采集吞吐 | 10×100Gbps, 512Mpps | 5.65M pps (release+LTO) |
| 采集延迟 (包→Flow) | P95 < 1ms | 实测通过 |
| 端到端延迟 (包→告警) | P95 ≤ 60s | 全链路验证中 |
| Kafka 消费吞吐 | — | 56,179 msg/s |
| ClickHouse 批量写入 | — | 136,986 行/s |
| Redis SET/GET | — | 400K/416K ops/s |
| OpenSearch 并发搜索 | — | 5 并发 200, 7–43ms |
| NebulaGraph nGQL | — | GO 785 QPS, FIND PATH 276 QPS |

### 10.2 可靠性机制

```
消息可靠:
  Kafka: acks=all, retries=5, enable.idempotence=true, DLQ
  Flink: checkpoint 30–60s, RocksDB state backend, savepoint 保留 5 个

幂等性:
  Ingest Gateway: event_id 去重 (Redis SET NX + TTL)
  ClickHouse: ReplacingMergeTree + event_id 主键
  Flink Sink: INSERT ON DUPLICATE KEY

降级容灾:
  MinIO 不可用 → 跳过 PCAP 裁剪，仅提供 Arkime 跳转
  OpenSearch 不可用 → 告警查询降级到 ClickHouse
  PostgreSQL 不可用 → 读操作使用 Replica, 写操作等待
  Redis 不可用 → Sentinel 自动故障转移 < 10s
  Kafka Broker 宕机 → 3 Broker KRaft, 容忍 1 节点故障

数据保留:
  Kafka: flow.events 24h, detections/alerts 72h, DLQ 168h
  ClickHouse: flows_raw 30d TTL, sessions_agg 90d TTL
  OpenSearch: audit 180d (网络安全法合规), 其他 30d
```

## 十一、新增业务逻辑 (2026-06-14 补齐)

### 11.1 告警通知 (Alert Notification)

**文件**: `internal/alert/notification/notification_service.go`

| 渠道 | 条件 | 实现 |
|------|------|------|
| Email (SMTP) | Severity ≥ high | HTML 邮件 + 告警详情表 |
| Slack | Severity ≥ critical | Webhook Attachment 格式 |
| Webhook | 全级别 | HTTP POST JSON |

频率限制: ≤10 条/分钟，按 tenant_id 路由通知。

### 11.2 资产风险评分 (Asset Risk Scoring)

**文件**: `internal/alert/risk/asset_risk.go`

| 维度 | 权重 | 数据 |
|------|:--:|------|
| 告警评分 | 40% | active×15 + critical×25 |
| 漏洞评分 | 15% | 协议异常计数 × 10 |
| 行为异常 | 25% | P95/avg PPS burst 比率 |
| 暴露面 | 20% | 开放端口 + 风险端口(22/3389/23/21) |

等级: 0-30=low, 30-60=medium, 60-80=high, 80-100=critical

### 11.3 SOAR 自动响应 (Playbook Engine)

**文件**: `internal/alert/playbook/playbook_engine.go`

6 个内置剧本:
- **block-scanner**: 扫描源 → block_ip(24h) + tag + notify
- **quarantine-c2**: C2通信 → quarantine(72h) + capture_pcap + enrich + notify
- **throttle-brute-force**: 暴力破解 → rate_limit(1rps) + notify
- **investigate-exfil**: 数据外泄 → capture_pcap(600s) + enrich + escalate + notify
- **log-lateral-movement**: 横向移动 → tag + enrich + notify
- **dns-tunnel-block**: DNS隧道 → block_domain(sinkhole) + capture_pcap + notify

### 11.4 数据质量监控 (Data Quality Monitor)

**文件**: `internal/common/dataquality/monitor.go`

| 检查 | 数据源 | 阈值 |
|------|--------|------|
| 数据流入率 | flows_raw 15min | < 100 flows/min → fail |
| 数据完整性 | sessions vs features | ratio < 0.9 → warn |
| 端到端延迟 | ingest_ts - event_ts P95 | > 60s → fail |
| Schema 漂移 | system.columns | 偏差 > 3 → fail |
| Kafka 积压 | flows_raw 写入率 | < 50% 基线 → warn |

状态: healthy (0 fail) | degraded (≥2 warn) | unhealthy (≥1 fail)

### 11.5 模型可解释性 (SHAP)

**文件**: `mlops/scripts/explain_model.py`

- SHAP TreeExplainer (XGBoost 原生加速)
- Beeswarm + Bar + Waterfall 可视化
- Permutation Importance (模型无关)
- 输出: PNG 图表 + JSON 重要性排名

### 11.6 业务逻辑完整度总览

| 类别 | 已完成 | 本次补齐 | 总计 |
|------|:--:|:--:|:--:|
| 数据采集 | 4/4 | — | 4 |
| 流计算 | 11/11 | — | 11 |
| 检测引擎 | 10/10 | — | 10 |
| 告警处置 | 4/4 | — | 4 |
| 资产管理 | 3/3 | — | 3 |
| 图分析 | 5/5 | — | 5 |
| MLOps | 6/6 | — | 6 |
| **通知** | **0** | **+1** | **1** |
| **风险评估** | **0** | **+1** | **1** |
| **自动响应** | **0** | **+1** | **1** |
| **数据质量** | **0** | **+1** | **1** |
| **可解释性** | **0** | **+1** | **1** |
| **总计** | **43** | **+5** | **48 项业务功能** |
```
