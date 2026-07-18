#!/usr/bin/env python3
"""Extract real TLS ClientHello fingerprints from a PCAP for replay validation.

The extractor computes JA3 from the wire bytes and can emit either a provenance
report or ClickHouse JSONEachRow records for traffic.feature_fp.  Replay mode
only shifts the event timestamp into the current validation window; the report
retains the source capture timestamp and SHA-256 so the input remains auditable.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import socket
import time
import uuid
from collections import Counter
from pathlib import Path
from typing import Any, Iterable

import dpkt


def is_grease(value: int) -> bool:
    return (value & 0x0F0F) == 0x0A0A and (value >> 8) == (value & 0xFF)


def u16(data: bytes, offset: int) -> tuple[int, int]:
    if offset + 2 > len(data):
        raise ValueError("truncated uint16")
    return int.from_bytes(data[offset : offset + 2], "big"), offset + 2


def vector_u16(data: bytes, offset: int) -> tuple[bytes, int]:
    size, offset = u16(data, offset)
    end = offset + size
    if end > len(data):
        raise ValueError("truncated uint16 vector")
    return data[offset:end], end


def tls_version_label(version: int) -> str:
    return {
        0x0300: "SSL 3.0",
        0x0301: "TLS 1.0",
        0x0302: "TLS 1.1",
        0x0303: "TLS 1.2/1.3",
        0x0304: "TLS 1.3",
    }.get(version, f"0x{version:04x}")


def shannon_entropy(payload: bytes) -> float:
    if not payload:
        return 0.0
    size = len(payload)
    return -sum((count / size) * math.log2(count / size) for count in Counter(payload).values())


def chi_square(payload: bytes) -> float:
    if not payload:
        return 0.0
    expected = len(payload) / 256.0
    counts = Counter(payload)
    return sum(((counts.get(value, 0) - expected) ** 2) / expected for value in range(256))


def parse_extensions(data: bytes) -> tuple[list[int], list[int], list[int], str]:
    extensions: list[int] = []
    groups: list[int] = []
    point_formats: list[int] = []
    sni = ""
    offset = 0
    while offset + 4 <= len(data):
        ext_type, offset = u16(data, offset)
        ext_data, offset = vector_u16(data, offset)
        if not is_grease(ext_type):
            extensions.append(ext_type)
        if ext_type == 10 and len(ext_data) >= 2:
            group_data, _ = vector_u16(ext_data, 0)
            groups = [
                int.from_bytes(group_data[pos : pos + 2], "big")
                for pos in range(0, len(group_data) - 1, 2)
                if not is_grease(int.from_bytes(group_data[pos : pos + 2], "big"))
            ]
        elif ext_type == 11 and ext_data:
            size = ext_data[0]
            point_formats = list(ext_data[1 : 1 + size])
        elif ext_type == 0 and len(ext_data) >= 5:
            try:
                names, _ = vector_u16(ext_data, 0)
                cursor = 0
                while cursor + 3 <= len(names):
                    name_type = names[cursor]
                    name_size = int.from_bytes(names[cursor + 1 : cursor + 3], "big")
                    cursor += 3
                    name = names[cursor : cursor + name_size]
                    cursor += name_size
                    if name_type == 0:
                        # TLS SNI carries an ASCII A-label.  Decode the wire value
                        # directly; Python's idna codec rejects non-strict handlers.
                        sni = name.decode("ascii")
                        break
            except (ValueError, UnicodeError):
                pass
    return extensions, groups, point_formats, sni


def parse_client_hello(body: bytes) -> dict[str, Any]:
    if len(body) < 35:
        raise ValueError("truncated ClientHello")
    version = int.from_bytes(body[0:2], "big")
    offset = 34
    session_size = body[offset]
    offset += 1 + session_size
    cipher_data, offset = vector_u16(body, offset)
    ciphers = [
        int.from_bytes(cipher_data[pos : pos + 2], "big")
        for pos in range(0, len(cipher_data) - 1, 2)
        if not is_grease(int.from_bytes(cipher_data[pos : pos + 2], "big"))
    ]
    if offset >= len(body):
        raise ValueError("missing compression methods")
    compression_size = body[offset]
    offset += 1 + compression_size
    extension_data = b""
    if offset + 2 <= len(body):
        extension_data, _ = vector_u16(body, offset)
    extensions, groups, point_formats, sni = parse_extensions(extension_data)
    ja3_text = ",".join(
        [
            str(version),
            "-".join(map(str, ciphers)),
            "-".join(map(str, extensions)),
            "-".join(map(str, groups)),
            "-".join(map(str, point_formats)),
        ]
    )
    return {
        "tls_version": tls_version_label(version),
        "ja3_string": ja3_text,
        "ja3": hashlib.md5(ja3_text.encode("ascii")).hexdigest(),
        "sni": sni,
    }


def ip_text(value: bytes) -> str:
    family = socket.AF_INET6 if len(value) == 16 else socket.AF_INET
    return socket.inet_ntop(family, value)


def packets(path: Path) -> Iterable[tuple[float, bytes]]:
    with path.open("rb") as handle:
        try:
            reader = dpkt.pcap.Reader(handle)
        except (ValueError, dpkt.dpkt.NeedData):
            handle.seek(0)
            reader = dpkt.pcapng.Reader(handle)
        yield from reader


def tcp_packet(frame: bytes) -> tuple[str, str, int, int, bytes] | None:
    try:
        ethernet = dpkt.ethernet.Ethernet(frame)
        ip = ethernet.data
        if not isinstance(ip, (dpkt.ip.IP, dpkt.ip6.IP6)) or not isinstance(ip.data, dpkt.tcp.TCP):
            return None
        tcp = ip.data
        return ip_text(ip.src), ip_text(ip.dst), tcp.sport, tcp.dport, bytes(tcp.data)
    except (ValueError, dpkt.dpkt.NeedData, dpkt.dpkt.UnpackError):
        return None


def client_hellos(payload: bytes) -> Iterable[dict[str, Any]]:
    offset = 0
    while offset + 5 <= len(payload):
        if payload[offset] != 22 or payload[offset + 1] != 3:
            offset += 1
            continue
        record_size = int.from_bytes(payload[offset + 3 : offset + 5], "big")
        record = payload[offset + 5 : offset + 5 + record_size]
        if len(record) < record_size:
            offset += 1
            continue
        cursor = 0
        while cursor + 4 <= len(record):
            handshake_type = record[cursor]
            handshake_size = int.from_bytes(record[cursor + 1 : cursor + 4], "big")
            body = record[cursor + 4 : cursor + 4 + handshake_size]
            if len(body) < handshake_size:
                break
            if handshake_type == 1:
                try:
                    yield parse_client_hello(body)
                except ValueError:
                    pass
            cursor += 4 + handshake_size
        offset += 5 + record_size


def extract(path: Path, limit: int, replay_epoch_ms: int) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    source_rows: list[dict[str, Any]] = []
    clickhouse_rows: list[dict[str, Any]] = []
    seen: set[tuple[str, int, str, int, str]] = set()
    for timestamp, frame in packets(path):
        packet = tcp_packet(frame)
        if not packet:
            continue
        src_ip, dst_ip, src_port, dst_port, payload = packet
        for hello in client_hellos(payload):
            key = (src_ip, src_port, dst_ip, dst_port, hello["ja3"])
            if key in seen:
                continue
            seen.add(key)
            source_ms = int(timestamp * 1000)
            flow_text = f"{src_ip}:{src_port}>{dst_ip}:{dst_port}/tcp"
            flow_hash = hashlib.sha1(flow_text.encode("utf-8")).hexdigest()
            event_id = str(uuid.uuid5(uuid.NAMESPACE_URL, f"{path}:{source_ms}:{flow_text}:{hello['ja3']}"))
            sni_hash = hashlib.sha256(hello["sni"].encode("utf-8")).hexdigest() if hello["sni"] else ""
            source_rows.append(
                {
                    "event_id": event_id,
                    "source_ts_ms": source_ms,
                    "replay_ts_ms": 0,
                    "source": f"{src_ip}:{src_port}",
                    "destination": f"{dst_ip}:{dst_port}",
                    **hello,
                }
            )
            clickhouse_rows.append(
                {
                    "tenant_id": "default",
                    "run_id": "pcap-tls-replay",
                    "feature_set_id": "tls-ja3-wire-v1",
                    "event_id": event_id,
                    "community_id": f"pcap:{flow_hash}",
                    "session_id": f"pcap-replay-{flow_hash[:24]}",
                    "ts": source_ms,
                    "is_encrypted": 1,
                    "tls_version": hello["tls_version"],
                    "ja3": hello["ja3"],
                    "sni_hash": sni_hash,
                    "cert_sha256": "",
                    "cert_is_self_signed": 0,
                    "pubkey_len": 0,
                    "hex_freq": [],
                    "hex_ratio": [],
                    "entropy_payload": round(shannon_entropy(payload), 6),
                    "chi_square_bfd": round(chi_square(payload), 6),
                }
            )
            if len(source_rows) >= limit:
                return align_replay_window(source_rows, clickhouse_rows, replay_epoch_ms)
    return align_replay_window(source_rows, clickhouse_rows, replay_epoch_ms)


def align_replay_window(
    source_rows: list[dict[str, Any]],
    clickhouse_rows: list[dict[str, Any]],
    replay_epoch_ms: int,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    """Place the final source event at replay_epoch_ms without changing event gaps."""
    if not source_rows:
        return source_rows, clickhouse_rows
    final_source_ms = max(int(row["source_ts_ms"]) for row in source_rows)
    for source_row, clickhouse_row in zip(source_rows, clickhouse_rows):
        replay_ms = replay_epoch_ms - (final_source_ms - int(source_row["source_ts_ms"]))
        source_row["replay_ts_ms"] = replay_ms
        clickhouse_row["ts"] = replay_ms
    return source_rows, clickhouse_rows


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("pcap", type=Path)
    parser.add_argument("--limit", type=int, default=100)
    parser.add_argument("--replay-epoch-ms", type=int, default=int(time.time() * 1000))
    parser.add_argument("--format", choices=("report", "clickhouse-json"), default="report")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    source_rows, clickhouse_rows = extract(args.pcap, args.limit, args.replay_epoch_ms)
    if args.format == "clickhouse-json":
        for row in clickhouse_rows:
            replay_ms = row.pop("ts")
            row["ts"] = time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime(replay_ms / 1000)) + f".{replay_ms % 1000:03d}"
            print(json.dumps(row, ensure_ascii=False, separators=(",", ":")))
        return 0 if clickhouse_rows else 2
    digest = hashlib.sha256(args.pcap.read_bytes()).hexdigest()
    rendered = json.dumps(
        {
            "result": "pass" if source_rows else "no_client_hello",
            "source_pcap": str(args.pcap),
            "source_size": args.pcap.stat().st_size,
            "source_sha256": digest,
            "replay_epoch_ms": args.replay_epoch_ms,
            "fingerprint_count": len(source_rows),
            "unique_ja3_count": len({row["ja3"] for row in source_rows}),
            "sni_count": sum(bool(row["sni"]) for row in source_rows),
            "fingerprints": source_rows,
        },
        ensure_ascii=False,
        indent=2,
    )
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered + "\n", encoding="utf-8")
    else:
        print(rendered)
    return 0 if source_rows else 2


if __name__ == "__main__":
    raise SystemExit(main())
