# Redis 缓存 Key 设计

## Key 命名规范
`{prefix}:{tenant_id}:{entity}:{id}`

## 缓存 Key 列表

### 1. 去重缓存 (Dedup)
```
dedup:{tenant_id}:{event_id}     → "1"     TTL=10min   事件去重
```

### 2. 配额限流 (Rate Limit)
```
quota:{tenant_id}:{probe_id}:{minute}  → count   TTL=2min   每分钟事件数
quota:{tenant_id}:{probe_id}:bandwidth → bytes   TTL=1min   带宽配额
```

### 3. Session/Token (Auth)
```
session:{session_id}       → JSON{session}   TTL=24h    用户会话
token:blacklist:{jti}      → "1"             TTL=7d     已撤销JWT
token:refresh:{user_id}    → refresh_token   TTL=30d    刷新令牌
```

### 4. 探针状态 (Probe)
```
probe:status:{probe_id}    → JSON{ProbeStatus}  TTL=5min   探针心跳
probe:config:{probe_id}    → JSON{ProbeConfig}  TTL=1h     探针配置缓存
```

### 5. 告警去重 (Alert)
```
alert:dedup:{fingerprint}  → alert_id       TTL=1h     告警聚合窗口
alert:state:{alert_id}     → state           TTL=30d    告警状态机
```

### 6. 资产缓存 (Asset)
```
asset:oui:{oui_prefix}     → vendor_name     TTL=24h    OUI厂商缓存
asset:mac:{tenant}:{mac}   → JSON{Asset}     TTL=1h     MAC→资产映射
```

### 7. 实时统计 (Dashboard)
```
stats:pps:{tenant}:{probe}   → value   TTL=1min   实时PPS
stats:flows:{tenant}         → count   TTL=1min   活跃Flow数
stats:alerts:{tenant}:{1h}   → count   TTL=1h     最近1h告警数
```

### 8. 分布式锁
```
lock:campaign:{campaign_id}  → owner   TTL=30s    CEP关联锁
lock:config:reload           → owner   TTL=10s    配置热加载锁
```

## 部署状态 (2026-06-06)

### 集群拓扑
```
Master:   redis-replica-0 (10.244.1.56, Node-9)  ← 由 Sentinel 自动选举
Replica:  redis-master-0  (10.244.0.78, Node-8)  ← 实际是 slave
Replica:  redis-replica-1 (10.244.0.84, Node-8)
Sentinel: redis-sentinel-0 (Node-9), sentinel-1 (Node-8), sentinel-2 (Node-9)
Quorum: 2/3
```

### 已初始化基线 Key (6 keys)
```
probe:status:probe-001         → 探针状态缓存
config:feature_set:active      → 当前活跃特征集
asset:oui:18:c0:09             → OUI 厂商缓存 (Broadcom)
asset:oui:00:1a:c5             → OUI 厂商缓存 (Cisco)
stats:flows:default            → 实时 Flow 统计
stats:alerts:default:1h        → 1h 告警计数
```

### 运维命令
```bash
# 查看主节点
kubectl exec -n databases redis-sentinel-0 -- redis-cli -p 26379 SENTINEL GET-MASTER-ADDR-BY-NAME mymaster

# 查看所有 key
kubectl exec -n databases <master-pod> -- redis-cli KEYS '*'

# 查看复制状态
kubectl exec -n databases <any-pod> -- redis-cli INFO REPLICATION
```
