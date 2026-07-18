#!/usr/bin/env python3
"""Install or disable the canonical encrypted-traffic UI dataset in PostgreSQL.

The fixture is opt-in and tenant scoped. The alert API reads these rows only when
`active=true`; deleting/disabling them immediately restores the live ClickHouse path.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import subprocess
from typing import Any


TENANT_ID = "default"
FIXTURE_VERSION = "encrypted-traffic-canonical-ui-v2"
PG_POD = os.environ.get("PG_POD", "postgres-primary-0")


JA3 = [
    "771,4865-4866-4867-49195-49196,0-11-10-23,0",
    "cbd52c1eb6700091a3e2d4c6214aa4d8",
    "e7d705342b8acb0d0e0a7d1d4f2c9b61",
    "4d7a2f00656b5f70b6a4e3c2d1f09a88",
    "598c8ab3943e1e610a65c28e7d44fa20",
    "f5a3d7c0eb2fe4e3c6fb101e9b72a154",
    "a1b2c3d4e5f6071827354565a6b7c8d9",
    "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a",
]


def session_rows() -> list[dict[str, Any]]:
    targets = [
        ("10.12.2.36", "104.16.248.249", 443, "TLS", "cdn.example.com", 10.0, "high"),
        ("10.18.4.5", "203.0.113.45", 443, "TLS", "api.github.com", 6.8, "medium"),
        ("172.16.5.10", "185.22.14.9", 443, "TLS", "cloudflare-dns.com", 6.0, "medium"),
        ("10.12.9.33", "198.51.100.27", 443, "TLS", "*.cdn77.com", 4.9, "medium"),
        ("10.11.3.22", "37.120.196.12", 443, "QUIC", "", 3.8, "low"),
        ("10.10.2.36", "172.67.219.23", 853, "TLS", "suspicious-tunnel.org", 3.0, "high"),
        ("10.10.8.45", "104.16.248.249", 443, "TLS", "update.example.com", 2.1, "medium"),
        ("10.10.9.33", "45.77.88.102", 8443, "TLS", "cdn-update.example", 1.6, "high"),
        ("10.10.10.23", "8.8.8.8", 443, "QUIC", "dns.google", 1.2, "low"),
        ("10.10.20.31", "203.0.113.55", 22, "SSH", "", 0.96, "medium"),
        ("10.10.30.40", "198.51.100.77", 443, "TLS", "sync.example.net", 0.72, "low"),
        ("10.10.40.12", "185.199.108.153", 443, "TLS", "vpn-suspect.example", 0.64, "high"),
    ]
    base = 1750387500000
    rows: list[dict[str, Any]] = []
    for index, (source, destination, port, protocol, sni, gbps, risk) in enumerate(targets):
        rows.append({
            "session_id": f"session-ui-{index + 1:04d}",
            "community_id": f"1:UIReference{index + 1:02d}=",
            "src_ip": source,
            "dst_ip": destination,
            "dst_port": port,
            "protocol": protocol,
            "sni": sni,
            "sni_hash": sni or "unknown-sni",
            "ja3_fingerprint": JA3[index % len(JA3)],
            "ja3s_fingerprint": f"ja3s-{index + 1:02d}-canonical",
            "alpn": "h3" if protocol == "QUIC" else "h2" if protocol == "TLS" else "-",
            "tls_version": "TLS 1.3" if index % 3 else "TLS 1.2",
            "cipher_suite": "TLS_AES_128_GCM_SHA256" if index % 2 else "TLS_AES_256_GCM_SHA384",
            "certificate_hash": f"cert{index + 1:060x}",
            "certificate_issuer": ["Google Trust Services", "DigiCert Inc", "Cloudflare Inc", "Amazon RSA 2048 M01"][index % 4],
            "entropy_score": 7.9 if risk == "high" else 6.8 if risk == "medium" else 4.2,
            "anomaly_score": 0.88 if risk == "high" else 0.58 if risk == "medium" else 0.18,
            "risk_level": risk,
            "packet_count": 12000 + index * 713,
            "byte_count": int(gbps * 1024 * 1024 * 1024),
            "evidence_count": 7 - index % 4,
            "pcap_index": f"pcap-20250620-{index + 1:05d}",
            "has_handshake_metadata": protocol != "SSH",
            "start_time": base - index * 42000,
            "end_time": base - index * 42000 + 31000,
            "traffic_gbps": gbps,
            "alert_count": [18, 9, 12, 6, 3, 8, 1, 8][index % 8],
            "duration_ms": 7_200_000 if index % 4 == 0 else 1_200_000,
        })
    return rows


def reference_visuals() -> dict[str, Any]:
    ja3_rows = [
        [JA3[0], "12.8%", "10.0", "312", "18", "高危"],
        [JA3[1], "8.7%", "6.8", "198", "9", "中危"],
        [JA3[2], "7.6%", "6.0", "145", "12", "中危"],
        [JA3[3], "6.2%", "4.9", "112", "6", "低危"],
        [JA3[4], "4.9%", "3.8", "98", "7", "中危"],
        [JA3[5], "3.6%", "3.0", "76", "3", "低危"],
        [JA3[6], "2.7%", "2.1", "63", "1", "低危"],
        [JA3[7], "2.1%", "1.6", "51", "8", "高危"],
    ]
    destinations = [
        ["52.223.31.45", "美国 / AWS · AS16509", "9.90 GB", "6,251", "中危"],
        ["104.16.248.249", "美国 / Cloudflare · AS13335", "18.70 GB", "12,845", "低危"],
        ["8.8.8.8", "美国 / Google · AS15169", "11.30 GB", "9,762", "低危"],
        ["20.190.128.1", "美国 / Azure · AS8075", "6.60 GB", "4,117", "中危"],
        ["47.246.16.23", "新加坡 / Alibaba · AS37963", "5.10 GB", "3,892", "低危"],
        ["45.77.88.102", "德国 / Unknown · AS209242", "6.20 GB", "4,321", "高危"],
        ["178.128.12.45", "俄罗斯 / Unknown · AS203201", "3.10 GB", "2,187", "高危"],
    ]
    evidence_sessions = []
    for index, row in enumerate(session_rows()[:9]):
        evidence_sessions.append({
            "time": f"06-20 03:{45 - index:02d}",
            "sessionId": row["session_id"],
            "source": row["src_ip"],
            "destination": row["dst_ip"],
            "protocol": row["protocol"],
            "sni": row["sni"] or "-",
            "ja3": row["ja3_fingerprint"],
            "alpn": row["alpn"],
            "certificateHash": row["certificate_hash"],
            "pcapIndex": row["pcap_index"],
            "risk": "高危" if row["risk_level"] == "high" else "中危" if row["risk_level"] == "medium" else "低危",
            "entropy": row["entropy_score"],
        })
    payloads = {
        "tabKpis": {
            "fingerprint": [
                ["指纹总数", "18,426", "较昨日 ↑ 8.6%"], ["可疑 JA3", "312", "较昨日 ↑ 24"],
                ["未知 SNI", "1,284", "较昨日 ↑ 134"], ["异常 Issuer", "47", "较昨日 ↑ 6"],
                ["TLS1.0/1.1", "63", "较昨日 ↑ 5"], ["弱密码套件", "128", "较昨日 ↑ 16"],
                ["关联规则", "26", "较昨日 ↑ 3"],
            ],
            "tunnelDetection": [
                ["隧道告警", "412", "高风险候选"], ["DoH 会话", "287", "加密 DNS"],
                ["异常长连接", "531", "持续时间"], ["高熵流量", "193", "载荷熵值"],
                ["低熵心跳", "76", "周期通信"], ["疑似 VPN", "118", "协议候选"],
                ["已创建告警", "32", "待审核"],
            ],
        },
        "protocolRows": [["TLS", "49.8 Gbps", "63.7%", "is-info"], ["QUIC", "14.4 Gbps", "18.4%", "is-warn"], ["未知加密", "14.1 Gbps", "17.9%", "is-risk"]],
        "protocolTrend": [38, 42, 48, 52, 55, 58, 61, 59, 63, 66, 64, 60, 58, 62, 65, 61, 59, 63, 68, 72, 69, 65, 78, 56],
        "ja3Rows": ja3_rows,
        "scatterPoints": [
            {"left": 12 + (i * 17) % 78, "top": 15 + (i * 23) % 67, "tone": ["risk", "warn", "ok", "info"][i % 4]}
            for i in range(28)
        ],
        "tunnelCards": [
            ["DNS over HTTPS 会话", "412", "↑ 36", "risk"], ["异常长连接（>1h）", "287", "↑ 21", "warn"],
            ["高熵流量（>7.5）", "531", "↑ 58", "risk"], ["低频流量（<3.0）", "76", "↑ 9", "ok"],
            ["低熵心跳（疑似）", "193", "↑ 14", "warn"], ["疑似 VPN 会话", "118", "↑ 13", "warn"],
        ],
        "tunnelRows": [
            ["DoH over TLS", "SNI=DNS, ALPN=h2/h3", "10.12.2.36", "cloudflare-dns.com", "3h 24m", "2.18", "高危"],
            ["异常长连接", "长连接 > 1h", "172.16.5.10", "203.0.113.45", "6h 12m", "3.47", "高危"],
            ["心跳隧道", "固定周期 60s", "10.10.8.45", "198.51.100.27", "2h 53m", "0.96", "中危"],
            ["高熵流量", "熵值 > 7.8", "10.10.9.33", "198.16.12.34", "1h 41m", "2.66", "中危"],
            ["低频流量", "频率 < 1/min", "10.11.3.22", "185.22.14.9", "4h 30m", "0.42", "低危"],
            ["疑似 VPN", "TLS over 443", "10.12.6.77", "37.120.196.12", "5h 18m", "1.02", "中危"],
        ],
        "destinationRows": destinations,
        "adviceRows": [["阻断高风险国家/地区 17 个访问。", "生成规则"], ["隔离高风险主机 10.10.8.45。", "隔离主机"], ["评估异常域名 198 个并加入观察名单。", "评估名单"], ["检查首次出现目的地 156 个。", "检查目的地"]],
        "certificateRows": [
            ["Google Trust Services", "www.google.com", "TLS 1.3", "h2", "18", "正常"],
            ["DigiCert Inc", "api.github.com", "TLS 1.3", "h2", "9", "正常"],
            ["Cloudflare Inc", "cloudflare-dns.com", "TLS 1.3", "h3", "12", "正常"],
            ["Amazon RSA 2048 M01", "*.cdn77.com", "TLS 1.2", "http/1.1", "6", "中危"],
            ["Let's Encrypt R3", "update.example.com", "TLS 1.3", "h2", "7", "中危"],
            ["Self-Signed", "unknown", "TLS 1.0", "-", "8", "高危"],
        ],
        "tlsSuiteRows": [["TLS 1.3", "TLS_AES_128_GCM_SHA256", "31.5%", "ok"], ["TLS 1.3", "TLS_AES_256_GCM_SHA384", "23.5%", "ok"], ["TLS 1.2", "ECDHE_RSA_AES128_GCM", "22.4%", "ok"], ["TLS 1.1", "AES128_SHA", "5.5%", "risk"], ["TLS 1.0", "3DES_EDE_CBC_SHA", "6.3%", "risk"], ["QUIC", "TLS_CHACHA20_POLY1305", "10.8%", "info"]],
        "tunnelRuleRows": [["可疑旧版 TLS 客户端", JA3[7], "完全匹配", "98%", "命中", "加入名单"], ["弱密码套件连接", JA3[5], "部分匹配", "92%", "命中", "加入证据"], ["自签名证书连接", JA3[4], "证书特征", "95%", "命中", "加入证据"], ["高熵未知 SNI", JA3[3], "SNI 特征", "80%", "观察中", "创建规则"], ["HTTP/1.1 罕见 ALPN", JA3[2], "ALPN 特征", "72%", "观察中", "创建规则"]],
        "evidenceRows": [[row["src_ip"], row["sni"] or row["dst_ip"], row["protocol"], row["ja3_fingerprint"], row["pcap_index"], "高危" if row["risk_level"] == "high" else "中危"] for row in session_rows()[:6]],
        "egressKpis": [["境外目的地", "362", "较昨日 ↑ 18%"], ["CDN / 云服务", "284", "较昨日 ↑ 12%"], ["异常域名", "198", "较昨日 ↑ 21%"], ["首次出现目的地", "156", "较昨日 ↑ 9%"], ["高风险国家/地区", "17", "较昨日 ↓ 5%"], ["外联资产", "428", "较昨日 ↑ 7%"], ["图谱待查", "39", "较昨日 ↑ 14%"]],
        "egressDomainCards": [
            ["cloudflare-dns.com", "Cloudflare · 104.16.248.249 · AS13335", "12,845 会话 · 18.7 GB", "低风险"],
            ["dns.google", "Google LLC · 8.8.8.8 · AS15169", "9,762 会话 · 11.3 GB", "低风险"],
            ["cdn-update.example", "Unknown · 45.77.88.102 · AS209242", "4,321 会话 · 6.2 GB", "高风险"],
            ["api.storage-cloud.net", "Amazon AWS · 52.223.31.45 · AS16509", "6,251 会话 · 9.9 GB", "中风险"],
            ["suspicious-tunnel.org", "Privacy Protect LLC · 172.67.219.23", "2,187 会话 · 3.1 GB", "高风险"],
            ["first-seen-cdn.net", "Unknown · 103.53.49.76 · AS140709", "1,945 会话 · 2.4 GB", "中风险"],
        ],
        "egressMapNodes": [{"id": f"dst-{i}", "label": row[0], "location": row[1], "flow": row[2], "sessions": row[3], "risk": row[4], "x": 18 + (i * 13) % 75, "y": 30 + (i * 19) % 48} for i, row in enumerate(destinations)],
        "egressTrend": {"labels": [f"{hour:02d}:00" for hour in range(0, 24, 2)], "series": [{"name": "首次出现目的地", "color": "#2d8cff", "values": [22, 31, 46, 58, 51, 64, 47, 72, 61, 55, 68, 49]}, {"name": "异常域名", "color": "#ff5b62", "values": [12, 18, 24, 33, 29, 41, 36, 48, 39, 44, 52, 31]}, {"name": "未知 SNI", "color": "#ffb020", "values": [31, 42, 55, 63, 58, 71, 62, 84, 76, 69, 81, 57]}]},
        "egressAvailability": {"state": "live", "detail": "加密流量 UI 原图数据库参考态已启用。"},
        "egressRiskScore": 78,
        "egressRiskDelta": "较昨日 ↑ 12",
        "heartbeatBars": [60.4 + [0.0, 0.4, -0.3, 0.2, -0.1, 0.3, -0.2, 0.1][i % 8] for i in range(48)],
        "heartbeatSummary": {"intervalP95Seconds": 60.4, "jitterP95Seconds": 0.82, "packetCount": 2486},
        "tunnelRiskDistribution": [
            {"label": "高风险", "value": 174, "ratio": "42.2%", "status": "risk"},
            {"label": "中风险", "value": 168, "ratio": "40.8%", "status": "warn"},
            {"label": "低风险", "value": 54, "ratio": "13.1%", "status": "ok"},
            {"label": "信息", "value": 16, "ratio": "3.9%", "status": "info"},
        ],
        "evidenceCenter": {
            "availability": {"state": "live", "detail": "数据库参考态返回 1,284 条会话和 436 条 PCAP 索引。"},
            "kpis": [["关联 Session", "1,284", "完整会话"], ["PCAP 索引", "436", "时间窗"], ["证书样本", "218", "证书链"], ["握手元数据", "9,642", "TLS/QUIC"], ["已校验 Hash", "391", "校验通过"], ["取证任务", "57", "执行中"], ["待补齐证据", "23", "需处理"]],
            "sessions": evidence_sessions,
            "pcapRows": [[f"pcap-20250620-{i + 1:05d}.pcap", f"10:{15 + i:02d} - 10:{16 + i:02d}", f"{11 + i * 3}.7 MB", f"{28341 - i * 3170:,}", "pcap-prod", f"{(i + 1):08x}ab...", "已校验" if i < 5 else "待校验"] for i in range(8)],
            "pcapTrend": [{"label": f"10:{15 + i:02d}", "value": 42 + (i * 17) % 84} for i in range(36)],
            "entropyTrend": [{"label": f"10:{15 + i:02d}", "value": 6.8 + ((i * 7) % 18) / 10} for i in range(24)],
            "certificateDetails": [{"label": label, "value": value} for label, value in [["Subject", "CN=update.example.com"], ["Issuer", "R3 (Let's Encrypt)"], ["Serial", "04:2A:78:9E:3C"], ["Not Before", "2025-04-10 03:12:00"], ["Not After", "2025-07-09 03:12:00"], ["Signature", "ECDSA with SHA256"]]],
            "handshakeTimeline": [{"time": f"10:15:01.{321 + i * 19}", "event": ["ClientHello", "ServerHello", "Encrypted Extensions", "Certificate", "Certificate Verify", "Finished"][i % 6], "detail": ["TLS 1.3, SNI update.example.com", "TLS_AES_128_GCM_SHA256", "ALPN h2", "R3 Let's Encrypt", "ECDSA P-256", "Handshake complete"][i % 6], "status": "ok"} for i in range(8)],
            "completeness": [{"label": "Session", "complete": 1284, "total": 1284, "status": "ok"}, {"label": "PCAP关联", "complete": 1012, "total": 1284, "status": "ok"}, {"label": "证书", "complete": 218, "total": 236, "status": "warn"}, {"label": "握手", "complete": 9642, "total": 9820, "status": "ok"}, {"label": "索引Hash", "complete": 391, "total": 436, "status": "warn"}],
            "hashRows": [[f"{(i + 1):064x}"[:16] + "...", f"pcap/2025/06/20/{i + 1:04d}.pcap", f"10:16:{i * 5:02d}", "admin", "通过" if i < 5 else "待校验"] for i in range(8)],
            "overviewSegments": [
                {"label": "完整", "value": 1012, "ratio": "62%", "status": "ok"},
                {"label": "待补齐", "value": 312, "ratio": "19%", "status": "warn"},
                {"label": "校验中", "value": 198, "ratio": "12%", "status": "info"},
                {"label": "缺失", "value": 96, "ratio": "7%", "status": "risk"},
            ],
        },
    }
    return payloads


def fixture_payloads() -> dict[str, Any]:
    sessions = session_rows()
    visuals = reference_visuals()
    fingerprints = [{"ja3": row[0], "tls_version": "TLS 1.3", "session_count": 1000 - i * 73, "sni_count": int(row[3]), "traffic_ratio": float(row[1].rstrip("%")) / 100, "traffic_gbps": float(row[2]), "alert_count": int(row[4]), "entropy_average": 7.9 if "高" in row[5] else 6.7, "risk_level": "malicious" if "高" in row[5] else "suspicious" if "中" in row[5] else "normal"} for i, row in enumerate(visuals["ja3Rows"])]
    pcap_indexes = [{"file_key": f"pcap-20250620-{i + 1:05d}.pcap", "probe_id": f"probe-{i % 8 + 1}", "start_time": 1750385700000 + i * 300000, "end_time": 1750385760000 + i * 300000, "packet_count": 28341 - i * 3170, "byte_count": (12 + i * 3) * 1024 * 1024, "storage_path": f"s3://pcap-prod/2025/06/20/{i + 1:05d}.pcap.zst", "sha256": f"{i + 1:064x}", "compressed_size": (7 + i) * 1024 * 1024} for i in range(12)]
    payloads = {
        "stats": {"total_sessions": 18426, "observed_sessions": 28926, "traffic_gbps": 78.3, "tls_sessions": 11736, "quic_sessions": 3390, "tls_ratio": 63.7, "quic_ratio": 18.4, "unknown_encrypted_ratio": 17.9, "abnormal_certificate_count": 236, "ja3_fingerprints": 18426, "ja3_sample_count": 18426, "malicious_ja3_matches": 172, "unknown_sni_ratio": 14.6, "fixture_mode": True, "fixture_version": FIXTURE_VERSION, "ui_reference_visuals": visuals},
        "sessions": {"sessions": sessions, "total": 18426, "fixture_mode": True},
        "ja3": {"fingerprints": fingerprints, "total": 18426, "source_state": "database_reference_fixture", "fixture_mode": True},
        "tunnels": {"protocols": [{"protocol": row[0], "count": int(row[1].replace(",", "")), "total_bytes": (index + 2) * 1024**3, "feature": row[2], "threshold": row[3] if len(row) > 3 else "-", "confidence": "95%"} for index, row in enumerate(visuals["tunnelCards"])], "users": [{"ip": row[2], "protocol": row[0], "count": int(''.join(ch for ch in row[1] if ch.isdigit()) or '1'), "risk": "high" if "高" in row[6] else "medium", "total_bytes": int(float(row[5]) * 1024**3), "last_seen": 1750387500000} for row in visuals["tunnelRows"]], "total": 412, "fixture_mode": True},
        "exfiltration": {"top_sources": [{"src_ip": "10.12.2.36", "session_count": 12845, "upload_bytes": 18700000000, "total_bytes": 22600000000, "dst_count": 36, "last_seen": 1750387500000, "risk": "high"}], "top_destinations": [{"dst_ip": row[0], "location": row[1], "session_count": int(row[3].replace(",", "")), "upload_bytes": int(float(row[2].split()[0]) * 1024**3), "total_bytes": int(float(row[2].split()[0]) * 1024**3), "src_count": 8 + i, "last_seen": 1750387500000, "risk": "high" if "高" in row[4] else "medium" if "中" in row[4] else "low"} for i, row in enumerate(visuals["destinationRows"])], "risk_types": [{"type": "high_risk_region", "count": 17, "severity": "high", "total_bytes": 18000000000}, {"type": "anomalous_domain", "count": 198, "severity": "high", "total_bytes": 12000000000}, {"type": "first_seen_destination", "count": 156, "severity": "medium", "total_bytes": 9000000000}], "paths": [{"src_ip": f"10.12.{i + 1}.36", "dst_ip": row[0], "session_count": int(row[3].replace(",", "")), "upload_bytes": int(float(row[2].split()[0]) * 1024**3), "last_seen": 1750387500000, "risk": "high" if "高" in row[4] else "medium"} for i, row in enumerate(visuals["destinationRows"])], "trend": [{"bucket_start": 1750300000000 + i * 7200000, "destination_count": 22 + i * 3, "large_upload_sessions": 12 + i * 2, "long_lived_sessions": 18 + i, "non_standard_port_sessions": 8 + i, "encrypted_sessions": 46 + i * 5} for i in range(12)], "total": 362, "fixture_mode": True},
        "evidence": {"sessions": sessions[:9], "pcap_indexes": pcap_indexes, "pcap_trend": [{"bucket_start": 1750385700000 + i * 300000, "byte_count": (42 + (i * 17) % 84) * 1024**2, "packet_count": 20000 + i * 311} for i in range(36)], "entropy_trend": [{"bucket_start": 1750385700000 + i * 300000, "entropy_score": 6.8 + ((i * 7) % 18) / 10} for i in range(24)], "entropy_available": True, "completeness": [{"label": "Session", "complete": 1284, "total": 1284}, {"label": "PCAP关联", "complete": 1012, "total": 1284}, {"label": "证书", "complete": 218, "total": 236}, {"label": "握手", "complete": 9642, "total": 9820}, {"label": "索引Hash", "complete": 391, "total": 436}], "total": 1284, "fixture_mode": True},
    }
    for payload in payloads.values():
        payload["fixture_mode"] = True
        payload["fixture_version"] = FIXTURE_VERSION
    return payloads


def kubectl_psql(sql: str) -> str:
    completed = subprocess.run(
        ["kubectl", "-n", "databases", "exec", "-i", PG_POD, "--", "psql", "-q", "-U", "postgres", "-d", "traffic_platform", "-v", "ON_ERROR_STOP=1", "-At"],
        input=sql,
        text=True,
        capture_output=True,
        check=True,
        env={key: value for key, value in os.environ.items() if key.lower() not in {"http_proxy", "https_proxy", "all_proxy"}},
    )
    return completed.stdout.strip()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--disable", action="store_true", help="Disable the fixture and restore live ClickHouse responses")
    args = parser.parse_args()
    schema = """
CREATE TABLE IF NOT EXISTS encrypted_traffic_ui_fixtures (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  endpoint TEXT NOT NULL CHECK (endpoint IN ('stats','sessions','ja3','tunnels','exfiltration','evidence')),
  fixture_version TEXT NOT NULL,
  payload JSONB NOT NULL,
  active BOOLEAN NOT NULL DEFAULT false,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, endpoint)
);
"""
    if args.disable:
        sql = schema + f"UPDATE encrypted_traffic_ui_fixtures SET active=false, updated_at=now() WHERE tenant_id='{TENANT_ID}';\nSELECT count(*) FROM encrypted_traffic_ui_fixtures WHERE tenant_id='{TENANT_ID}' AND active=true;\n"
        print(json.dumps({"active_rows": int(kubectl_psql(sql) or "0"), "fixture_version": FIXTURE_VERSION}))
        return

    statements = [schema]
    for endpoint, payload in fixture_payloads().items():
        encoded = base64.b64encode(json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode()).decode()
        statements.append(
            "INSERT INTO encrypted_traffic_ui_fixtures (tenant_id, endpoint, fixture_version, payload, active, updated_at) "
            f"VALUES ('{TENANT_ID}', '{endpoint}', '{FIXTURE_VERSION}', convert_from(decode('{encoded}','base64'),'UTF8')::jsonb, true, now()) "
            "ON CONFLICT (tenant_id, endpoint) DO UPDATE SET fixture_version=EXCLUDED.fixture_version, payload=EXCLUDED.payload, active=true, updated_at=now();"
        )
    statements.append(f"SELECT count(*) FROM encrypted_traffic_ui_fixtures WHERE tenant_id='{TENANT_ID}' AND active=true AND fixture_version='{FIXTURE_VERSION}';")
    active = int(kubectl_psql("\n".join(statements)) or "0")
    print(json.dumps({"active_rows": active, "fixture_version": FIXTURE_VERSION, "tenant_id": TENANT_ID}, ensure_ascii=False))


if __name__ == "__main__":
    main()
