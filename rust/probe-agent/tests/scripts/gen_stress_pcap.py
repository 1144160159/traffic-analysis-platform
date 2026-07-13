#!/usr/bin/env python3
"""
Probe Agent 压力测试 PCAP 生成器。
生成指定数量的合法 Ethernet + IPv4 + TCP 报文用于吞吐量测试。

用法:
    python3 gen_stress_pcap.py [包数量] [输出文件]

默认: 500000 包 → /tmp/stress_test.pcap
"""

import struct
import random
import sys
import os

NUM_PACKETS = int(sys.argv[1]) if len(sys.argv) > 1 else 500_000
OUTPUT = sys.argv[2] if len(sys.argv) > 2 else "/tmp/stress_test.pcap"


def make_eth_ip_tcp(src_ip, dst_ip, src_port, dst_port, payload_len):
    """
    构建合法的 Ethernet + IPv4 + TCP 报文。
    etherparse 可成功解析此格式。
    """
    # IPv4 header (20 bytes)
    ip_header = bytearray(20)
    ip_header[0] = 0x45                 # Version(4) + IHL(5)
    ip_header[1] = 0x00                 # DSCP + ECN
    total_len = 20 + 20 + payload_len   # IP hdr + TCP hdr + payload
    struct.pack_into('>H', ip_header, 2, total_len)
    struct.pack_into('>H', ip_header, 4, random.randint(1, 65535))  # Identification
    struct.pack_into('>H', ip_header, 6, 0x4000)  # Flags: Don't Fragment
    ip_header[8] = 64                   # TTL
    ip_header[9] = 6                    # Protocol: TCP
    ip_header[10:12] = b'\x00\x00'      # Header checksum (0, let kernel fill)
    ip_header[12:16] = bytes(int(b) for b in src_ip.split('.'))
    ip_header[16:20] = bytes(int(b) for b in dst_ip.split('.'))

    # TCP header (20 bytes, SYN flag set)
    tcp_header = bytearray(20)
    struct.pack_into('>H', tcp_header, 0, src_port)
    struct.pack_into('>H', tcp_header, 2, dst_port)
    struct.pack_into('>I', tcp_header, 4, random.randint(1, 2**32 - 1))  # Seq
    struct.pack_into('>I', tcp_header, 8, 0)           # Ack (0 for SYN)
    tcp_header[12] = 0x50               # Data offset (5×4=20 bytes)
    tcp_header[13] = 0x02               # Flags: SYN
    struct.pack_into('>H', tcp_header, 14, 65535)      # Window
    struct.pack_into('>H', tcp_header, 16, 0)          # Checksum (0)

    # Ethernet header (14 bytes)
    eth = bytearray(14)
    eth[0:6] = bytes([random.randint(0, 255) for _ in range(6)])   # Dst MAC
    eth[6:12] = bytes([random.randint(0, 255) for _ in range(6)])  # Src MAC
    eth[12:14] = b'\x08\x00'            # EtherType: IPv4

    payload = bytes(random.randint(0, 255) for _ in range(payload_len))

    return bytes(eth + ip_header + tcp_header + payload)


def generate():
    print(f"Probe Agent 压力测试 PCAP 生成器")
    print(f"  包数量:   {NUM_PACKETS:,}")
    print(f"  输出文件: {OUTPUT}")
    print(f"  生成中...")

    with open(OUTPUT, 'wb') as f:
        # PCAP global header (24 bytes)
        f.write(struct.pack('<IHHiIII',
            0xa1b2c3d4,   # magic (little-endian)
            2, 4,          # version
            0, 0,          # timezone, sigfigs
            65535,         # snap length
            1))            # link type: Ethernet

        for i in range(NUM_PACKETS):
            # 随机五元组：模拟真实流量分布
            src_ip = f"192.168.{random.randint(1, 254)}.{random.randint(1, 254)}"
            dst_ip = f"10.0.{random.randint(0, 5)}.{random.randint(1, 254)}"
            src_port = random.randint(1024, 65535)
            dst_port = random.choice([80, 443, 8080, 53, 22, 3306, 6379, 9092])
            payload_len = random.randint(40, 200)

            pkt = make_eth_ip_tcp(src_ip, dst_ip, src_port, dst_port, payload_len)

            # 模拟时间戳：10000 pps 间隔
            ts_sec = i // 10000
            ts_usec = (i % 10000) * 100

            # PCAP packet header (16 bytes) + data
            f.write(struct.pack('<IIII', ts_sec, ts_usec, len(pkt), len(pkt)))
            f.write(pkt)

    size_mb = os.path.getsize(OUTPUT) / 1_048_576
    print(f"  完成: {size_mb:.1f} MB, {NUM_PACKETS:,} 包")
    print(f"  验证: file $(OUTPUT) → {os.popen(f'file {OUTPUT}').read().strip()}")


if __name__ == "__main__":
    generate()
