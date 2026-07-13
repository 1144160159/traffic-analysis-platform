# NebulaGraph 图数据库 — Schema 与运维指南

> 集群名称: `traffic_graph`
> 连接入口: `nebula-graph.middleware.svc:9669` (LoadBalancer `10.0.5.224`)
> HTTP API: `nebula-graph:19669`
> 用户/密码: `root` / `root`

## 一、集群规格

| 组件 | 副本 | CPU Req/Lim | 内存 Req/Lim | 存储 | 端口 |
|------|:--:|-------------|-------------|------|------|
| Meta | 3 | 1-2 / 2-4 | 2Gi / 4Gi | 20Gi | 9559(thrift), 19559(http) |
| Graph | 3 | 2-4 / 4-8 | 4Gi / 8Gi | — | 9669(thrift), 19669(http) |
| Storage | 3 | 2-4 / 4-8 | 8Gi / 16Gi | 100Gi | 9777(thrift), 9778(admin), 19779(http) |

## 二、图空间

```nGQL
CREATE SPACE traffic_graph(
  vid_type=FIXED_STRING(32),
  replica_factor=3,
  partition_num=30
);
```

**VID 策略**: `FIXED_STRING(32)` 要求 VID 精确 32 字节。
- Go 客户端使用 `hashVID()` 函数 (MD5 → 32 hex chars) 确定性生成 VID
- 查询时必须通过 hashVID 转换后方可 FETCH/GO，否则返回空集
- IP 地址、alert_id、campaign_id 等均需 hashVID 映射

## 三、数据模型

### Tags (5 类节点)

```nGQL
CREATE TAG ip_address(
  tenant_id    STRING  NOT NULL, ip STRING NOT NULL,
  mac_address  STRING,  hostname STRING, vendor STRING,
  os_type      STRING,  is_gateway BOOL DEFAULT false,
  risk_score   DOUBLE DEFAULT 0.0, first_seen INT64,
  last_seen    INT64,   alert_count INT64 DEFAULT 0
);

CREATE TAG session(
  tenant_id    STRING  NOT NULL, community_id STRING NOT NULL,
  protocol     INT8,   src_port INT32, dst_port INT32,
  packets_fwd  INT64,  packets_bwd INT64,
  bytes_fwd    INT64,  bytes_bwd INT64,
  duration_ms  INT32,  ts_start INT64, ts_end INT64,
  alert_count  INT64   DEFAULT 0
);

CREATE TAG alert(
  tenant_id    STRING  NOT NULL, alert_type STRING NOT NULL,
  severity     STRING,  score DOUBLE, labels STRING,
  first_seen   INT64,   last_seen INT64,
  status       STRING   DEFAULT 'new'
);

CREATE TAG campaign(
  tenant_id      STRING  NOT NULL, campaign_type STRING,
  title          STRING,  description STRING,
  severity       STRING,  score DOUBLE,
  phase_progress DOUBLE   DEFAULT 0.0,
  start_time     INT64,   end_time INT64
);

CREATE TAG network_device(
  tenant_id     STRING  NOT NULL, device_type STRING,
  mgmt_ip       STRING,  hostname STRING, vendor STRING,
  model         STRING,  software_ver STRING,
  ports_count   INT32   DEFAULT 0
);
```

### Edge Types (7 类关系边)

```nGQL
-- 网络通信 (src → dst)
CREATE EDGE communicates(
  tenant_id STRING NOT NULL, community_id STRING,
  protocol INT8, session_count INT64 DEFAULT 1,
  total_bytes INT64, total_packets INT64,
  first_seen INT64, last_seen INT64, direction STRING
);

-- 会话归属 (IP → Session)
CREATE EDGE has_session(tenant_id STRING NOT NULL, role STRING NOT NULL, ts INT64);

-- 告警触发 (Session → Alert)
CREATE EDGE triggers_alert(tenant_id STRING NOT NULL, alert_id STRING NOT NULL, ts INT64);

-- 活动包含 (Campaign → Alert)
CREATE EDGE includes_alert(tenant_id STRING NOT NULL, alert_id STRING, ts INT64);

-- IP归属 (IP → Subnet/AS)
CREATE EDGE belongs_to(tenant_id STRING NOT NULL, subnet STRING, asn INT32, ts INT64);

-- 拓扑连接 (Switch → IP / Switch → Switch)
CREATE EDGE connects_to(tenant_id STRING NOT NULL, port STRING, vlan_id INT32, bandwidth INT64, ts INT64);

-- 攻击路径跳 (src → hop1 → ... → dst)
CREATE EDGE attack_path_hop(tenant_id STRING NOT NULL, campaign_id STRING, hop_order INT32, ts INT64);
```

### 索引 (12 个)

```nGQL
-- Tag 索引 (9)
CREATE TAG INDEX ip_tenant_idx      ON ip_address(tenant_id(32));
CREATE TAG INDEX ip_address_idx     ON ip_address(ip(32));
CREATE TAG INDEX ip_risk_idx        ON ip_address(risk_score);
CREATE TAG INDEX session_tenant_idx ON session(tenant_id(32));
CREATE TAG INDEX session_comm_idx   ON session(community_id(32));
CREATE TAG INDEX alert_tenant_idx   ON alert(tenant_id(32));
CREATE TAG INDEX alert_type_idx     ON alert(alert_type(32));
CREATE TAG INDEX alert_severity_idx ON alert(severity(16));
CREATE TAG INDEX campaign_tenant_idx ON campaign(tenant_id(32));

-- Edge 索引 (3)
CREATE EDGE INDEX comm_tenant_idx   ON communicates(tenant_id(32));
CREATE EDGE INDEX trigger_alert_idx ON triggers_alert(alert_id(32));
CREATE EDGE INDEX att_path_camp_idx ON attack_path_hop(campaign_id(32));
```

## 四、部署与运维

### 初始化

```bash
# 打标签 (主节点)
kubectl label node zeus-server nebula-primary=true --overwrite

# 部署集群
kubectl apply -f deployments/kubernetes/infrastructure/09-nebula-graph.yaml

# 等待各组件就绪
kubectl wait --for=condition=ready pod -l app=nebula,component=meta -n middleware --timeout=300s
kubectl wait --for=condition=ready pod -l app=nebula,component=storage -n middleware --timeout=300s
kubectl wait --for=condition=ready pod -l app=nebula,component=graph -n middleware --timeout=120s

# 初始化 Schema (或使用 init-nebula-schema Job)
kubectl apply -f deployments/kubernetes/init-jobs/05-nebula-schema.yaml
```

### 健康检查

```bash
# 查看所有 Pod
kubectl get pods -n middleware -l app=nebula -o wide

# 查看 Hosts 状态 (所有 Storage 应 ONLINE)
kubectl run nebula-check --rm -it --restart=Never \
  --image=vesoft/nebula-console:v3.6.0 -n middleware -- \
  -addr nebula-graph.middleware.svc -port 9669 -u root -p root \
  -e "SHOW HOSTS;"

# 查看分区分布
kubectl run nebula-parts --rm -it --restart=Never \
  --image=vesoft/nebula-console:v3.6.0 -n middleware -- \
  -addr nebula-graph.middleware.svc -port 9669 -u root -p root \
  -e "USE traffic_graph; SHOW PARTS;"

# 查看 Tags 和 Edges
kubectl run nebula-meta-check --rm -it --restart=Never \
  --image=vesoft/nebula-console:v3.6.0 -n middleware -- \
  -addr nebula-graph.middleware.svc -port 9669 -u root -p root \
  -e "USE traffic_graph; SHOW TAGS; SHOW EDGES;"
```

### Storage 故障恢复

如果 Storage Pod CrashLoopBackOff，按以下步骤排查：

```bash
# 1. 检查 Meta 中的 Host 注册
SHOW HOSTS;  -- 确认 Storage IP 和状态

# 2. 如果 Host OFFLINE 或 IP 变更
DROP HOSTS "<old_ip>":9777;
ADD HOSTS "<new_ip>":9777;

# 3. 检查 META_ADDRS 环境变量
kubectl get statefulset nebula-storage -n middleware -o yaml | grep META_ADDRS

# 4. 如缺失，注入
kubectl set env statefulset/nebula-storage -n middleware \
  META_ADDRS="nebula-meta-0.nebula-meta.middleware.svc:9559,nebula-meta-1.nebula-meta.middleware.svc:9559,nebula-meta-2.nebula-meta.middleware.svc:9559"

# 5. 重建 Pod 使配置生效
kubectl delete pod -n middleware nebula-storage-0 nebula-storage-1 nebula-storage-2
```

## 五、Go 客户端使用

### 双客户端架构

| 客户端 | 文件 | 适用场景 |
|--------|------|---------|
| HTTP Client | `client_http.go` | 生产环境（需 nebula-http-gateway） |
| Console Client | `client_console.go` | K8s Pod 内（需 nebula-console 二进制） |
| TCP Client | `client.go` | 开发/测试（简化实现，生产需 nebula-go SDK） |

### 使用示例

```go
// === HTTP Client ===
httpCfg := nebula.DefaultHTTPConfig()
httpClient, err := nebula.NewHTTPClient(httpCfg, logger)
defer httpClient.Close()

// 健康检查
err = httpClient.Ping(ctx)

// 查看集群
hosts, _ := httpClient.ShowHosts(ctx)
spaces, _ := httpClient.ShowSpaces(ctx)

// === Console Client (K8s Pod 内) ===
consoleCfg := nebula.DefaultConsoleConfig()
consoleClient, _ := nebula.NewConsoleClient(consoleCfg, logger)

// 执行 nGQL
result, err := consoleClient.Execute(ctx, "SHOW HOSTS;")

// === 通用操作 (所有客户端) ===
// 插入 IP 节点 (VID 自动 hash)
err = client.InsertIPNode(ctx, "tenant-1", "192.168.1.1",
    "aa:bb:cc:dd:ee:ff", "web-server", "Dell", "Linux",
    false, 0.15, firstSeen, lastSeen)

// 插入通信边
err = client.InsertSessionEdge(ctx, "tenant-1",
    "192.168.1.1", "10.0.0.5", "community-id-123",
    6, 42, 1048576, 5000, firstSeen, lastSeen, "outbound")

// 查询邻居
neighbors, err := client.GetNeighbors(ctx, "tenant-1", "192.168.1.1", 50)
```

## 六、图算法引擎

Go 图算法引擎提供 5 类分析能力：

| 算法 | 功能 | 应用场景 |
|------|------|---------|
| Louvain 社区检测 | 识别功能子网/攻击群组 | 网络分段分析、僵尸网络发现 |
| PageRank | 识别关键节点 | 核心资产识别 |
| Betweenness Centrality | 桥梁节点识别 | 网络瓶颈、横向移动关键节点 |
| Attack Path Analysis | 攻击路径重建 | DFIR 取证 |
| Anomaly Pattern Detection | Star/Chain/Mesh/Isolated | C2/横向移动/P2P/隐蔽通道检测 |
