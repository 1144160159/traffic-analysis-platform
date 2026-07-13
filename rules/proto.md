# Protobuf / gRPC 规范

基于 Google API Design Guide、buf 最佳实践。

## 1. 文件组织

```protobuf
// proto/traffic/v1/flow.proto
syntax = "proto3";
package traffic.v1;
option go_package = "go/control-plane/pkg/proto/traffic/v1;trafficv1";
```

## 2. 命名

```protobuf
// 消息: PascalCase, 字段: snake_case
message FlowEvent {
  string flow_id = 1;      // UUID v4
  string probe_id = 2;     // 探针 ID
  string tenant_id = 3;    // 租户 ID
  string src_ip = 4;       // 源 IP (String 而非 bytes)
  uint64 byte_count = 10;  // 字节数
}

// 服务: PascalCase + Service 后缀
service IngestService {
  rpc UploadFlows(UploadFlowsRequest) returns (UploadFlowsResponse);
  rpc RegisterProbe(RegisterProbeRequest) returns (RegisterProbeResponse);
}
```

## 3. 字段规范

```protobuf
// IP 地址: string (支持 IPv4/IPv6, 不用 bytes)
string src_ip = 4;   // ✓
bytes src_ip = 4;    // ✗

// 时间戳: google.protobuf.Timestamp 或 int64 (ms)
int64 start_ts = 11;  // Unix ms
// 或: google.protobuf.Timestamp start_time = 11;

// 枚举: 必须从 0 开始, 0 = UNSPECIFIED
enum AlertStatus {
  ALERT_STATUS_UNSPECIFIED = 0;
  ALERT_STATUS_NEW = 1;
  ALERT_STATUS_ACKNOWLEDGED = 2;
  ALERT_STATUS_RESOLVED = 3;
}

// 禁止:
//   删除字段 (用 reserved)
//   更改字段类型或编号
//   required (proto3 不支持)
```

## 4. 版本兼容

```protobuf
// 删除字段用 reserved
message OldMessage {
  reserved 2, 15, 9 to 11;
  reserved "old_field_name";
}

// 新增字段: 只追加, 不插入
// 字段编号 1-15 用于高频字段 (1 字节编码)
```

## 5. 生成与校验

```bash
cd proto
buf lint                    # 格式检查
buf breaking --against '.git#branch=main'  # 兼容性检查
./scripts/generate.sh       # 生成 Go/Rust/Java
# 影响: go/control-plane/pkg/proto/
#       rust/probe-agent/proto-gen/src/
#       java/flink-jobs/flink-common/src/main/java/
```

## 6. gRPC 规范

```protobuf
// 批量操作: 用 repeated 消息, 不用 stream
message BatchUploadRequest {
  string tenant_id = 1;
  string probe_id = 2;
  repeated FlowEvent events = 3;  // ✓ 批量
}

// 长时间操作: 返回 operation_id
message ReplayPcapResponse {
  string operation_id = 1;  // 用于轮询状态
}

// 错误: 用标准 gRPC status codes
// NOT_FOUND, PERMISSION_DENIED, RESOURCE_EXHAUSTED, INTERNAL
```
