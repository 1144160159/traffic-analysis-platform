#!/bin/bash

# 配置
SERVER="localhost:50051"
TOKEN="${GRPC_TOKEN:-test-token-001}"
PROBE="${GRPC_PROBE:-probe-default-001}"

# 公共参数
GRPC_OPTS="-plaintext -H x-tenant-token:$TOKEN -H x-probe-id:$PROBE"

echo "🔧 gRPC 测试工具"
echo "服务器: $SERVER"
echo "Token: $TOKEN"
echo "Probe: $PROBE"
echo ""

case "$1" in
  list)
    echo "📋 列出所有服务..."
    grpcurl $GRPC_OPTS $SERVER list
    ;;
  
  describe)
    SERVICE="${2:-traffic.v1.IngestService}"
    echo "📖 查看服务详情: $SERVICE"
    grpcurl $GRPC_OPTS $SERVER describe $SERVICE
    ;;
  
  heartbeat)
    echo "💓 发送心跳..."
    grpcurl $GRPC_OPTS \
      -d '{
        "tenant_id": "default",
        "probe_id": "probe-default-001",
        "status": {
          "cpu_usage": 25.5,
          "memory_usage": 536870912,
          "packets_captured": 10000,
          "packets_dropped": 10,
          "capture_pps": 1000
        }
      }' \
      $SERVER traffic.v1.IngestService.Heartbeat
    ;;
  
  flow)
    echo "📊 上传 Flow 事件..."
    grpcurl $GRPC_OPTS \
      -d '{
        "events": [
          {
            "header": {
              "eventId": "test-'$(date +%s)'",
              "tenantId": "default",
              "probeId": "probe-default-001",
              "eventTs": '$(date +%s000)'
            },
            "communityId": "1:test",
            "tuple": {
              "srcIp": "192.168.1.100",
              "dstIp": "10.0.0.1",
              "srcPort": 12345,
              "dstPort": 80,
              "protocol": 6
            },
            "tsStart": '$(date +%s000)',
            "tsEnd": '$(date +%s000)',
            "packetsFwd": 100,
            "packetsBwd": 50,
            "bytesFwd": 10000,
            "bytesBwd": 5000
          }
        ]
      }' \
      $SERVER traffic.v1.IngestService.UploadFlows
    ;;
  
  session)
    echo "📡 上传 Session 事件..."
    grpcurl $GRPC_OPTS \
      -d '{
        "sessions": [
          {
            "header": {
              "eventId": "session-'$(date +%s)'",
              "tenantId": "default",
              "probeId": "probe-default-001",
              "eventTs": '$(date +%s000)'
            },
            "sessionId": "sess-001",
            "communityId": "1:test",
            "clientIp": "192.168.1.100",
            "serverIp": "10.0.0.1",
            "clientPort": 12345,
            "serverPort": 80,
            "protocol": 6,
            "tsStart": '$(date +%s000)',
            "tsEnd": '$(date +%s000)',
            "packetsTotal": 150,
            "bytesTotal": 15000
          }
        ]
      }' \
      $SERVER traffic.v1.IngestService.UploadSessions
    ;;
  
  pcap)
    echo "💾 上传 PCAP 索引..."
    grpcurl $GRPC_OPTS \
      -d '{
        "index": {
          "tenantId": "default",
          "probeId": "probe-default-001",
          "fileKey": "pcap/test-'$(date +%s)'.pcap.zst",
          "tsStart": '$(date +%s000)',
          "tsEnd": '$(date +%s000)',
          "byteSize": 1048576,
          "zstdLevel": 3,
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "communityId": "1:test"
        }
      }' \
      $SERVER traffic.v1.IngestService.UploadPcapIndex
    ;;
  
  health)
    echo "🏥 健康检查..."
    grpcurl -plaintext $SERVER grpc.health.v1.Health.Check
    ;;
  
  all)
    echo "🚀 运行所有测试..."
    $0 list
    echo ""
    $0 heartbeat
    echo ""
    $0 flow
    echo ""
    $0 session
    echo ""
    $0 pcap
    ;;
  
  *)
    echo "用法: $0 {list|describe|heartbeat|flow|session|pcap|health|all} [参数]"
    echo ""
    echo "示例:"
    echo "  $0 list                    # 列出服务"
    echo "  $0 describe                # 查看 IngestService"
    echo "  $0 heartbeat               # 发送心跳"
    echo "  $0 flow                    # 上传 Flow 事件"
    echo "  $0 session                 # 上传 Session 事件"
    echo "  $0 pcap                    # 上传 PCAP 索引"
    echo "  $0 all                     # 运行所有测试"
    echo ""
    echo "环境变量:"
    echo "  GRPC_TOKEN    - API Token (默认: test-token-001)"
    echo "  GRPC_PROBE    - Probe ID (默认: probe-default-001)"
    exit 1
    ;;
esac
