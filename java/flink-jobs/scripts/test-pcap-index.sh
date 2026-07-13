#!/usr/bin/env bash
# 发送测试 PCAP 索引数据到 Kafka

set -euo pipefail

KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
TOPIC="${TOPIC:-pcap.index.v1}"

# 创建测试 PCAP 索引消息（使用 protobuf 编码）
# 这里用 JSON 格式模拟，实际需要使用 protobuf 序列化

echo "Sending test PCAP index to ${TOPIC}..."

# 使用 Python 发送 Protobuf 消息
python3 << 'EOF'
import sys
sys.path.insert(0, '../../go/control-plane/pkg/proto/traffic/v1')

try:
    from traffic.v1 import pcap_pb2
    from kafka import KafkaProducer
    import time

    producer = KafkaProducer(
        bootstrap_servers='localhost:9092',
        value_serializer=lambda v: v.SerializeToString()
    )

    now_ms = int(time.time() * 1000)
    
    meta = pcap_pb2.PcapIndexMeta()
    meta.tenant_id = "tenant-1"
    meta.probe_id = "probe-1"
    meta.file_key = f"pcap/tenant-1/probe-1/2024/01/15/12/capture-{now_ms}.pcap.zst"
    meta.ts_start = now_ms - 60000
    meta.ts_end = now_ms
    meta.byte_size = 104857600  # 100 MB
    meta.zstd_level = 3
    meta.sha256 = "abc123def456"
    meta.community_id = "1:test=="
    meta.created_ts = now_ms

    producer.send('pcap.index.v1', meta)
    producer.flush()
    print("Test PCAP index sent successfully!")

except ImportError:
    print("Python protobuf modules not found, skipping test")
except Exception as e:
    print(f"Error: {e}")
EOF

echo "Done!"