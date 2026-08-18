#!/usr/bin/env python3
"""Generate the deterministic M03 parser golden PCAP corpus.

The corpus is synthetic and carries no security ground-truth labels.  Its
categories describe protocol structures that the parser must exercise; they
must never be reused as blind evaluation labels.
"""

from __future__ import annotations

import argparse
import hashlib
import ipaddress
import json
import struct
from dataclasses import dataclass
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
OUTPUT = REPO / "tests/fixtures/pcap/m03"
MANIFEST = OUTPUT / "manifest.v1.json"
MANIFEST_SCHEMA = OUTPUT / "manifest.v1.schema.json"
RECIPE_VERSION = "m03-pcap-golden-v1"
BASE_SECONDS = 1_700_000_000


def internet_checksum(data: bytes) -> int:
    if len(data) % 2:
        data += b"\x00"
    total = sum(struct.unpack(f"!{len(data) // 2}H", data))
    while total >> 16:
        total = (total & 0xFFFF) + (total >> 16)
    return (~total) & 0xFFFF


def ethernet(payload: bytes, ether_type: int = 0x0800) -> bytes:
    return bytes.fromhex("020000000002020000000001") + struct.pack("!H", ether_type) + payload


def ipv4(src: str, dst: str, protocol: int, payload: bytes, packet_id: int = 1) -> bytes:
    src_bytes = ipaddress.IPv4Address(src).packed
    dst_bytes = ipaddress.IPv4Address(dst).packed
    header = struct.pack(
        "!BBHHHBBH4s4s",
        0x45,
        0,
        20 + len(payload),
        packet_id,
        0,
        64,
        protocol,
        0,
        src_bytes,
        dst_bytes,
    )
    checksum = internet_checksum(header)
    return header[:10] + struct.pack("!H", checksum) + header[12:] + payload


def ipv6(src: str, dst: str, next_header: int, payload: bytes) -> bytes:
    return struct.pack(
        "!IHBB16s16s",
        6 << 28,
        len(payload),
        next_header,
        64,
        ipaddress.IPv6Address(src).packed,
        ipaddress.IPv6Address(dst).packed,
    ) + payload


def udp(src_port: int, dst_port: int, payload: bytes) -> bytes:
    return struct.pack("!HHHH", src_port, dst_port, 8 + len(payload), 0) + payload


def tcp(src_port: int, dst_port: int, flags: int, payload: bytes = b"", seq: int = 1) -> bytes:
    return struct.pack("!HHIIBBHHH", src_port, dst_port, seq, 0, 5 << 4, flags, 65535, 0, 0) + payload


def dns_query() -> bytes:
    return bytes.fromhex("123401000001000000000000") + b"\x03www\x07example\x03com\x00" + bytes.fromhex("00010001")


def dns_response() -> bytes:
    return (
        bytes.fromhex("123481800001000100000000")
        + b"\x03www\x07example\x03com\x00"
        + bytes.fromhex("00010001c00c000100010000003c0004c000020a")
    )


def pcap(records: list[tuple[bytes, int | None]]) -> bytes:
    output = bytearray(struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1))
    for index, (frame, original_len) in enumerate(records):
        output.extend(
            struct.pack(
                "<IIII",
                BASE_SECONDS + index,
                index * 1_000,
                len(frame),
                original_len if original_len is not None else len(frame),
            )
        )
        output.extend(frame)
    return bytes(output)


@dataclass(frozen=True)
class Fixture:
    fixture_id: str
    category: str
    records: list[tuple[bytes, int | None]]
    decoded_count: int
    rejected_count: int
    aggregation_key_count: int
    protocols: list[int]
    transport_statuses: list[str]
    first_tuple: dict[str, object] | None

    @property
    def filename(self) -> str:
        return f"{self.fixture_id}.pcap"


def fixtures() -> list[Fixture]:
    normal_query = ethernet(ipv4("192.0.2.20", "8.8.8.8", 17, udp(53000, 53, dns_query())))
    normal_response = ethernet(ipv4("8.8.8.8", "192.0.2.20", 17, udp(53, 53000, dns_response()), 2))
    attack_records = [
        (ethernet(ipv4("10.0.0.50", "10.0.0.1", 6, tcp(40000 + port, port, 0x02), port)), None)
        for port in range(20, 36)
    ]
    ipv6_udp = udp(5353, 5353, b"m03-ipv6")
    hop_by_hop = bytes([17, 0]) + bytes(6) + ipv6_udp
    tls_client_hello = bytes.fromhex("16030300050100000100")
    quic_initial_shape = bytes.fromhex("c30000000108") + b"m03quic0" + b"\x00\x00"
    truncated_frame = ethernet(bytes.fromhex("45000028000100004006"))
    large_records: list[tuple[bytes, int | None]] = []
    for index in range(128):
        if index % 2 == 0:
            segment = tcp(49152, 443, 0x18, b"x" * 32, index + 1)
            frame = ethernet(ipv4("192.0.2.30", "198.51.100.40", 6, segment, index + 1))
        else:
            segment = tcp(443, 49152, 0x18, b"y" * 32, index + 1)
            frame = ethernet(ipv4("198.51.100.40", "192.0.2.30", 6, segment, index + 1))
        large_records.append((frame, None))

    return [
        Fixture(
            "normal-dns",
            "normal",
            [(normal_query, None), (normal_response, None)],
            2,
            0,
            1,
            [17],
            ["decoded"],
            {"src_ip": "192.0.2.20", "dst_ip": "8.8.8.8", "src_port": 53000, "dst_port": 53, "protocol": 17},
        ),
        Fixture(
            "attack-scan-shape",
            "attack_structure",
            attack_records,
            16,
            0,
            16,
            [6],
            ["decoded"],
            {"src_ip": "10.0.0.50", "dst_ip": "10.0.0.1", "src_port": 40020, "dst_port": 20, "protocol": 6},
        ),
        Fixture(
            "ipv6-extension",
            "ipv6",
            [(ethernet(ipv6("2001:db8::1", "2001:db8::2", 0, hop_by_hop), 0x86DD), None)],
            1,
            0,
            1,
            [17],
            ["decoded"],
            {"src_ip": "2001:db8::1", "dst_ip": "2001:db8::2", "src_port": 5353, "dst_port": 5353, "protocol": 17},
        ),
        Fixture(
            "tls-client-hello-shape",
            "tls",
            [(ethernet(ipv4("192.0.2.60", "198.51.100.60", 6, tcp(51000, 443, 0x18, tls_client_hello))), None)],
            1,
            0,
            1,
            [6],
            ["decoded"],
            {"src_ip": "192.0.2.60", "dst_ip": "198.51.100.60", "src_port": 51000, "dst_port": 443, "protocol": 6},
        ),
        Fixture(
            "quic-initial-shape",
            "quic",
            [(ethernet(ipv4("192.0.2.70", "198.51.100.70", 17, udp(52000, 443, quic_initial_shape))), None)],
            1,
            0,
            1,
            [17],
            ["decoded"],
            {"src_ip": "192.0.2.70", "dst_ip": "198.51.100.70", "src_port": 52000, "dst_port": 443, "protocol": 17},
        ),
        Fixture("truncated-frame", "truncated", [(truncated_frame, 60)], 0, 1, 0, [], [], None),
        Fixture(
            "large-bidirectional-flow",
            "large_flow",
            large_records,
            128,
            0,
            1,
            [6],
            ["decoded"],
            {"src_ip": "192.0.2.30", "dst_ip": "198.51.100.40", "src_port": 49152, "dst_port": 443, "protocol": 6},
        ),
        Fixture("empty", "empty", [], 0, 0, 0, [], [], None),
    ]


def build() -> tuple[dict[str, bytes], dict[str, object]]:
    files: dict[str, bytes] = {}
    entries: list[dict[str, object]] = []
    for fixture in fixtures():
        body = pcap(fixture.records)
        files[fixture.filename] = body
        entries.append(
            {
                "fixture_id": fixture.fixture_id,
                "relative_path": fixture.filename,
                "category": fixture.category,
                "sha256": hashlib.sha256(body).hexdigest(),
                "size_bytes": len(body),
                "packet_count": len(fixture.records),
                "expected": {
                    "decoded_count": fixture.decoded_count,
                    "rejected_count": fixture.rejected_count,
                    "aggregation_key_count": fixture.aggregation_key_count,
                    "protocols": fixture.protocols,
                    "transport_statuses": fixture.transport_statuses,
                    "first_tuple": fixture.first_tuple,
                },
                "semantic_label": {
                    "status": "synthetic_structure_not_ground_truth",
                    "blind_evaluation_eligible": False,
                },
            }
        )
    generator_hash = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    manifest: dict[str, object] = {
        "schema_version": "1.0.0",
        "corpus_id": RECIPE_VERSION,
        "generator": {
            "path": "scripts/testdata/generate_m03_pcap_golden.py",
            "sha256": generator_hash,
        },
        "source": {
            "kind": "deterministic_synthetic",
            "license": "project_generated_test_fixture",
            "custodian_role": "traffic-parser-feature-owner",
        },
        "blind_label_policy": "categories describe exercised structures only and are forbidden as model or attack ground-truth labels",
        "coverage": ["normal", "attack_structure", "ipv6", "tls", "quic", "truncated", "large_flow", "empty"],
        "fixtures": entries,
    }
    return files, manifest


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()
    files, manifest = build()
    manifest_bytes = (json.dumps(manifest, ensure_ascii=False, indent=2) + "\n").encode()
    if args.write:
        OUTPUT.mkdir(parents=True, exist_ok=True)
        for name, body in files.items():
            (OUTPUT / name).write_bytes(body)
        MANIFEST.write_bytes(manifest_bytes)
        print(json.dumps({"result": "written", "fixtures": len(files), "manifest": str(MANIFEST)}))
        return 0

    failures: list[str] = []
    expected_names = set(files) | {MANIFEST.name, MANIFEST_SCHEMA.name}
    actual_names = {path.name for path in OUTPUT.iterdir() if path.is_file()} if OUTPUT.is_dir() else set()
    if actual_names != expected_names:
        failures.append(f"file set mismatch expected={sorted(expected_names)} actual={sorted(actual_names)}")
    for name, body in files.items():
        path = OUTPUT / name
        if not path.is_file() or path.read_bytes() != body:
            failures.append(f"fixture drift: {name}")
    if not MANIFEST.is_file() or MANIFEST.read_bytes() != manifest_bytes:
        failures.append("manifest drift")
    if failures:
        raise SystemExit("M03_PCAP_GOLDEN_CHECK_FAILED: " + "; ".join(failures))
    print(json.dumps({"result": "pass", "fixtures": len(files), "manifest": str(MANIFEST)}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
