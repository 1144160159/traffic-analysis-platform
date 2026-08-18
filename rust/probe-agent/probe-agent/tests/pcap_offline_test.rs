use std::io::Write;
use tempfile::TempDir;

/// Create a minimal valid PCAP file with test packets
fn create_test_pcap(path: &std::path::Path) {
    let mut f = std::fs::File::create(path).unwrap();

    // PCAP global header (little-endian)
    let magic: u32 = 0xa1b2c3d4;
    let version_major: u16 = 2;
    let version_minor: u16 = 4;
    let thiszone: i32 = 0;
    let sigfigs: u32 = 0;
    let snaplen: u32 = 65535;
    let network: u32 = 1; // Ethernet

    f.write_all(&magic.to_le_bytes()).unwrap();
    f.write_all(&version_major.to_le_bytes()).unwrap();
    f.write_all(&version_minor.to_le_bytes()).unwrap();
    f.write_all(&thiszone.to_le_bytes()).unwrap();
    f.write_all(&sigfigs.to_le_bytes()).unwrap();
    f.write_all(&snaplen.to_le_bytes()).unwrap();
    f.write_all(&network.to_le_bytes()).unwrap();

    // Add 3 test packets (minimal Ethernet + IP + TCP)
    let test_payloads: Vec<(u32, u32, Vec<u8>)> = vec![
        (
            0,
            1000,
            make_ethernet_ip_tcp_packet(0xc0a80001, 0xc0a80002, 80, 12345),
        ),
        (
            1,
            2000,
            make_ethernet_ip_tcp_packet(0xc0a80002, 0xc0a80001, 12345, 80),
        ),
        (
            2,
            3000,
            make_ethernet_ip_tcp_packet(0xc0a80001, 0xc0a80002, 80, 12346),
        ),
    ];

    for (ts_sec, ts_usec, payload) in &test_payloads {
        let incl_len = payload.len() as u32;
        let orig_len = payload.len() as u32;

        f.write_all(&ts_sec.to_le_bytes()).unwrap();
        f.write_all(&ts_usec.to_le_bytes()).unwrap();
        f.write_all(&incl_len.to_le_bytes()).unwrap();
        f.write_all(&orig_len.to_le_bytes()).unwrap();
        f.write_all(payload).unwrap();
    }
}

/// Create a minimal Ethernet + IPv4 + TCP packet
fn make_ethernet_ip_tcp_packet(src_ip: u32, dst_ip: u32, src_port: u16, dst_port: u16) -> Vec<u8> {
    let mut pkt = Vec::new();

    // Ethernet header (14 bytes) - just dummy MACs
    pkt.extend_from_slice(&[0xff; 6]); // Dst MAC
    pkt.extend_from_slice(&[0x00; 6]); // Src MAC
    pkt.extend_from_slice(&0x0800u16.to_be_bytes()); // EtherType: IPv4

    // IPv4 header (20 bytes)
    pkt.push(0x45); // Version + IHL
    pkt.push(0x00); // DSCP
    let total_len: u16 = 40; // 20 IP + 20 TCP
    pkt.extend_from_slice(&total_len.to_be_bytes());
    pkt.extend_from_slice(&0x0000u16.to_be_bytes()); // ID
    pkt.extend_from_slice(&0x4000u16.to_be_bytes()); // Flags + Fragment
    pkt.push(64); // TTL
    pkt.push(6); // Protocol: TCP
    pkt.extend_from_slice(&0x0000u16.to_be_bytes()); // Checksum (0 for test)
    pkt.extend_from_slice(&src_ip.to_be_bytes());
    pkt.extend_from_slice(&dst_ip.to_be_bytes());

    // TCP header (20 bytes)
    pkt.extend_from_slice(&src_port.to_be_bytes());
    pkt.extend_from_slice(&dst_port.to_be_bytes());
    pkt.extend_from_slice(&0x00000001u32.to_be_bytes()); // Seq
    pkt.extend_from_slice(&0x00000000u32.to_be_bytes()); // Ack
    pkt.push(0x50); // Data offset + flags (SYN)
    pkt.push(0x02); // Flags (SYN)
    pkt.extend_from_slice(&0xffffu16.to_be_bytes()); // Window
    pkt.extend_from_slice(&0x0000u16.to_be_bytes()); // Checksum
    pkt.extend_from_slice(&0x0000u16.to_be_bytes()); // Urgent

    pkt
}

#[test]
fn test_pcap_reader_basic() {
    let dir = TempDir::new().unwrap();
    let pcap_path = dir.path().join("test.pcap");
    create_test_pcap(&pcap_path);

    // Read it back using PcapReader
    let reader = probe_agent::capture::pcap_offline::PcapReader::from_file(&pcap_path);
    assert!(reader.is_ok(), "Failed to open pcap: {:?}", reader.err());

    let mut reader = reader.unwrap();
    let mut count = 0;
    while let Some((data, ts)) = reader.next_packet() {
        count += 1;
        assert!(data.len() > 0, "Packet should have data");
        assert!(ts > 0, "Timestamp should be non-zero");
    }
    assert_eq!(count, 3, "Should have read 3 packets");
}

#[test]
fn test_pcap_replayer_creates() {
    let dir = TempDir::new().unwrap();
    let pcap_path = dir.path().join("test.pcap");
    create_test_pcap(&pcap_path);

    use probe_agent::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
    let replayer = PcapReplayer::new(pcap_path.to_str().unwrap(), ReplaySpeed::MaxSpeed, false);
    assert!(
        replayer.is_ok(),
        "Failed to create replayer: {:?}",
        replayer.err()
    );
}

#[test]
fn test_pcap_replayer_poll() {
    let dir = TempDir::new().unwrap();
    let pcap_path = dir.path().join("test.pcap");
    create_test_pcap(&pcap_path);

    use probe_agent::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
    use probe_agent::capture::Capturer;

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut replayer =
            PcapReplayer::new(pcap_path.to_str().unwrap(), ReplaySpeed::MaxSpeed, false).unwrap();

        replayer.start().await.unwrap();

        let mut batches = 0;
        let mut packets = 0;
        loop {
            match replayer.poll() {
                Ok(Some(batch)) => {
                    batches += 1;
                    packets += batch.len();
                }
                Ok(None) => break,
                Err(e) => panic!("Poll error: {}", e),
            }
        }

        assert!(batches > 0, "Should get at least 1 batch");
        assert_eq!(packets, 3, "Should get 3 packets");
    });
}

#[derive(Clone, Copy)]
enum TestEndian {
    Little,
    Big,
}

fn push_u16(bytes: &mut Vec<u8>, value: u16, endian: TestEndian) {
    let encoded = match endian {
        TestEndian::Little => value.to_le_bytes(),
        TestEndian::Big => value.to_be_bytes(),
    };
    bytes.extend_from_slice(&encoded);
}

fn push_u32(bytes: &mut Vec<u8>, value: u32, endian: TestEndian) {
    let encoded = match endian {
        TestEndian::Little => value.to_le_bytes(),
        TestEndian::Big => value.to_be_bytes(),
    };
    bytes.extend_from_slice(&encoded);
}

fn pcap_bytes(endian: TestEndian, nanosecond: bool, subsecond: u32, payload: &[u8]) -> Vec<u8> {
    let mut bytes = Vec::new();
    bytes.extend_from_slice(match (endian, nanosecond) {
        (TestEndian::Little, false) => &[0xd4, 0xc3, 0xb2, 0xa1],
        (TestEndian::Big, false) => &[0xa1, 0xb2, 0xc3, 0xd4],
        (TestEndian::Little, true) => &[0x4d, 0x3c, 0xb2, 0xa1],
        (TestEndian::Big, true) => &[0xa1, 0xb2, 0x3c, 0x4d],
    });
    push_u16(&mut bytes, 2, endian);
    push_u16(&mut bytes, 4, endian);
    push_u32(&mut bytes, 0, endian);
    push_u32(&mut bytes, 0, endian);
    push_u32(&mut bytes, 65_535, endian);
    push_u32(&mut bytes, 1, endian);
    push_u32(&mut bytes, 7, endian);
    push_u32(&mut bytes, subsecond, endian);
    push_u32(&mut bytes, payload.len() as u32, endian);
    push_u32(&mut bytes, payload.len() as u32, endian);
    bytes.extend_from_slice(payload);
    bytes
}

fn sha256_hex(bytes: &[u8]) -> String {
    use sha2::{Digest, Sha256};
    Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

#[test]
fn checked_reader_preserves_endian_and_source_precision() {
    use probe_agent::capture::pcap_offline::PcapReader;
    use probe_agent::capture::TimestampPrecision;

    let directory = TempDir::new().unwrap();
    let cases = [
        ("le-micro.pcap", TestEndian::Little, false, 123_456, 0),
        ("be-micro.pcap", TestEndian::Big, false, 123_456, 0),
        ("le-nano.pcap", TestEndian::Little, true, 123_456_789, 789),
        ("be-nano.pcap", TestEndian::Big, true, 123_456_789, 789),
    ];
    for (name, endian, nanos, subsecond, expected_loss) in cases {
        let path = directory.path().join(name);
        std::fs::write(&path, pcap_bytes(endian, nanos, subsecond, &[1, 2, 3, 4])).unwrap();
        let mut reader = PcapReader::from_file(&path).unwrap();
        let record = reader.next_packet_checked().unwrap().unwrap();
        assert_eq!(record.bytes, [1, 2, 3, 4]);
        assert_eq!(record.captured_at.epoch_micros(), 7_123_456);
        assert_eq!(record.captured_at.precision_loss_nanos, expected_loss);
        assert_eq!(
            record.captured_at.source_precision,
            if nanos {
                TimestampPrecision::Nanosecond
            } else {
                TimestampPrecision::Microsecond
            }
        );
        assert!(reader.next_packet_checked().unwrap().is_none());
    }
}

#[test]
fn checked_reader_distinguishes_clean_eof_from_truncation_without_advancing() {
    use probe_agent::capture::pcap_offline::{PcapReadError, PcapReader};

    let directory = TempDir::new().unwrap();
    let path = directory.path().join("truncated.pcap");
    let mut bytes = pcap_bytes(TestEndian::Little, false, 1, &[1, 2, 3, 4]);
    bytes.truncate(bytes.len() - 2);
    std::fs::write(&path, bytes).unwrap();
    let mut reader = PcapReader::from_file(&path).unwrap();
    assert!(matches!(
        reader.next_packet_checked(),
        Err(PcapReadError::TruncatedPacketPayload { .. })
    ));
    assert!(matches!(
        reader.next_packet_checked(),
        Err(PcapReadError::TruncatedPacketPayload { .. })
    ));

    let clean = directory.path().join("empty.pcap");
    std::fs::write(
        &clean,
        pcap_bytes(TestEndian::Little, false, 1, &[1])[..24].to_vec(),
    )
    .unwrap();
    assert!(PcapReader::from_file(&clean)
        .unwrap()
        .next_packet_checked()
        .unwrap()
        .is_none());
}

#[test]
fn manifest_route_validates_identity_and_never_scans_sibling_files() {
    use probe_agent::capture::pcap_offline::{OfflinePcapManifest, ReplaySpeed};
    use probe_agent::capture::{Capturer, ManifestPcapReplayer};

    let directory = TempDir::new().unwrap();
    let payload = [9, 8, 7, 6];
    let pcap = pcap_bytes(TestEndian::Little, false, 42, &payload);
    let pcap_path = directory.path().join("approved.pcap");
    std::fs::write(&pcap_path, &pcap).unwrap();
    std::fs::write(directory.path().join("unlisted-broken.pcap"), b"not-pcap").unwrap();
    let manifest_path = directory.path().join("manifest.json");
    let manifest_body = serde_json::json!({
        "schema_version": "1.0.0",
        "dataset_id": "dataset-a",
        "run_id": "run-a",
        "base_dir": ".",
        "entries": [{
            "entry_id": "packet-1",
            "relative_path": "approved.pcap",
            "sha256": sha256_hex(&pcap),
            "size_bytes": pcap.len(),
            "byte_order": "little_endian",
            "timestamp_precision": "microsecond",
            "link_type": 1,
            "packet_count": 1
        }]
    });
    std::fs::write(&manifest_path, serde_json::to_vec(&manifest_body).unwrap()).unwrap();

    let manifest = OfflinePcapManifest::load_and_validate(&manifest_path).unwrap();
    let body_hash = manifest.body_sha256.clone();
    let mut replayer =
        ManifestPcapReplayer::from_manifest(manifest, ReplaySpeed::MaxSpeed, false).unwrap();
    assert_eq!(replayer.manifest_body_sha256(), body_hash);
    let runtime = tokio::runtime::Runtime::new().unwrap();
    runtime.block_on(replayer.start()).unwrap();
    let batch = replayer.poll().unwrap().unwrap();
    let packets = batch.copy_packets();
    assert_eq!(packets.len(), 1);
    assert_eq!(packets[0].0, payload);
    assert_eq!(packets[0].1.epoch_micros(), 7_000_042);
    assert_eq!(replayer.current_entry_id(), Some("packet-1"));
    assert!(replayer.poll().unwrap().is_none());
}

#[test]
fn manifest_rejects_path_escape_hash_drift_and_body_hash_mismatch() {
    use probe_agent::capture::create_capturer;
    use probe_agent::capture::pcap_offline::OfflinePcapManifest;
    use probe_agent::config::{CaptureConfig, CaptureMode};

    let directory = TempDir::new().unwrap();
    let pcap = pcap_bytes(TestEndian::Little, false, 42, &[1]);
    std::fs::write(directory.path().join("approved.pcap"), &pcap).unwrap();
    let manifest_path = directory.path().join("manifest.json");
    let manifest_body = serde_json::json!({
        "schema_version": "1.0.0",
        "dataset_id": "dataset-a",
        "run_id": "run-a",
        "base_dir": ".",
        "entries": [{
            "entry_id": "packet-1",
            "relative_path": "../escape.pcap",
            "sha256": sha256_hex(&pcap),
            "size_bytes": pcap.len(),
            "byte_order": "little_endian",
            "timestamp_precision": "microsecond",
            "link_type": 1,
            "packet_count": 1
        }]
    });
    std::fs::write(&manifest_path, serde_json::to_vec(&manifest_body).unwrap()).unwrap();
    assert!(OfflinePcapManifest::load_and_validate(&manifest_path)
        .unwrap_err()
        .to_string()
        .contains("PCAP_MANIFEST_PATH_ESCAPE"));

    let valid_body = serde_json::json!({
        "schema_version": "1.0.0",
        "dataset_id": "dataset-a",
        "run_id": "run-a",
        "base_dir": ".",
        "entries": [{
            "entry_id": "packet-1",
            "relative_path": "approved.pcap",
            "sha256": sha256_hex(&pcap),
            "size_bytes": pcap.len(),
            "byte_order": "little_endian",
            "timestamp_precision": "microsecond",
            "link_type": 1,
            "packet_count": 1
        }]
    });
    std::fs::write(&manifest_path, serde_json::to_vec(&valid_body).unwrap()).unwrap();
    let config = CaptureConfig {
        mode: CaptureMode::PcapOffline,
        pcap_manifest_route_enabled: true,
        pcap_manifest_path: Some(manifest_path.to_string_lossy().into_owned()),
        pcap_manifest_sha256: Some("0".repeat(64)),
        replay_speed: Some("max".to_string()),
        ..Default::default()
    };
    let error = tokio::runtime::Runtime::new()
        .unwrap()
        .block_on(create_capturer(&config))
        .err()
        .expect("body hash drift must reject");
    assert!(error
        .to_string()
        .contains("PCAP_MANIFEST_BODY_HASH_MISMATCH"));

    std::fs::write(directory.path().join("approved.pcap"), b"mutated").unwrap();
    assert!(OfflinePcapManifest::load_and_validate(&manifest_path)
        .unwrap_err()
        .to_string()
        .contains("PCAP_MANIFEST_ENTRY_SIZE_MISMATCH"));
}
