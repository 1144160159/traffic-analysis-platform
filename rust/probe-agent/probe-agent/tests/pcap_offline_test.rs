use std::io::Write;
use std::path::PathBuf;
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
