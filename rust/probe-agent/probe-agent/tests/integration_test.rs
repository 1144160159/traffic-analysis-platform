//! Probe Agent 集成测试
//! 覆盖完整数据链路：PCAP → Parser → FlowTable → Eviction → FlowEvent

use probe_agent::aggregator::{
    Eviction, EvictionConfig, PacketProcessor, PacketProcessor as Processor, PartitionedFlowTable,
};
use probe_agent::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
use probe_agent::capture::Capturer;
use probe_agent::parser::PacketParser;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tempfile::TempDir;

// ─── helpers ────────────────────────────────────────────────

fn gen_pcap(path: &std::path::Path, packets: &[(u32, u32, &[u8])]) {
    let mut f = std::fs::File::create(path).unwrap();
    // global header
    f.write_all(&0xa1b2c3d4u32.to_le_bytes()).unwrap();
    f.write_all(&[2u16.to_le_bytes(), 4u16.to_le_bytes()].concat())
        .unwrap();
    f.write_all(&[0u32.to_le_bytes(), 0u32.to_le_bytes()].concat())
        .unwrap();
    f.write_all(&65535u32.to_le_bytes()).unwrap();
    f.write_all(&1u32.to_le_bytes()).unwrap(); // Ethernet

    use std::io::Write;
    for &(ts_sec, ts_usec, data) in packets {
        f.write_all(&ts_sec.to_le_bytes()).unwrap();
        f.write_all(&ts_usec.to_le_bytes()).unwrap();
        f.write_all(&(data.len() as u32).to_le_bytes()).unwrap();
        f.write_all(&(data.len() as u32).to_le_bytes()).unwrap();
        f.write_all(data).unwrap();
    }
}

/// Build a valid Ethernet+IPv4+TCP packet
fn tcp_pkt(src: &str, dst: &str, sport: u16, dport: u16, plen: usize) -> Vec<u8> {
    let mut p = Vec::new();
    p.extend_from_slice(&[0xff; 6]); // dst mac
    p.extend_from_slice(&[0x00; 6]); // src mac
    p.extend_from_slice(&0x0800u16.to_be_bytes()); // EtherType IPv4
                                                   // IPv4 header
    let total = 20u16 + 20u16 + plen as u16;
    p.extend_from_slice(&[0x45, 0x00]);
    p.extend_from_slice(&total.to_be_bytes());
    p.extend_from_slice(&0x0001u16.to_be_bytes()); // id
    p.extend_from_slice(&0x4000u16.to_be_bytes()); // flags
    p.extend_from_slice(&[64, 6]); // ttl, proto tcp
    p.extend_from_slice(&[0u8; 2]); // checksum
    for b in src.split('.') {
        p.push(b.parse().unwrap());
    }
    for b in dst.split('.') {
        p.push(b.parse().unwrap());
    }
    // TCP header
    p.extend_from_slice(&sport.to_be_bytes());
    p.extend_from_slice(&dport.to_be_bytes());
    p.extend_from_slice(&1u32.to_be_bytes()); // seq
    p.extend_from_slice(&0u32.to_be_bytes()); // ack
    p.extend_from_slice(&[0x50, 0x02]); // data offset + SYN
    p.extend_from_slice(&65535u16.to_be_bytes()); // window
    p.extend_from_slice(&[0u8; 2]); // checksum
    p.extend_from_slice(&[0u8; 2]); // urgent
    p.extend_from_slice(&vec![0u8; plen]); // payload
    p
}

fn udp_pkt(src: &str, dst: &str, sport: u16, dport: u16, plen: usize) -> Vec<u8> {
    let mut p = Vec::new();
    p.extend_from_slice(&[0xff; 6]);
    p.extend_from_slice(&[0x00; 6]);
    p.extend_from_slice(&0x0800u16.to_be_bytes());
    let total = 20u16 + 8u16 + plen as u16;
    p.extend_from_slice(&[0x45, 0x00]);
    p.extend_from_slice(&total.to_be_bytes());
    p.extend_from_slice(&0x0001u16.to_be_bytes());
    p.extend_from_slice(&0x4000u16.to_be_bytes());
    p.extend_from_slice(&[64, 17]); // proto UDP
    p.extend_from_slice(&[0u8; 2]);
    for b in src.split('.') {
        p.push(b.parse().unwrap());
    }
    for b in dst.split('.') {
        p.push(b.parse().unwrap());
    }
    // UDP header: sport, dport, length, checksum
    p.extend_from_slice(&sport.to_be_bytes());
    p.extend_from_slice(&dport.to_be_bytes());
    let udp_len = 8u16 + plen as u16;
    p.extend_from_slice(&udp_len.to_be_bytes());
    p.extend_from_slice(&[0u8; 2]); // checksum
    p.extend_from_slice(&vec![0u8; plen]);
    p
}

fn run_replayer(pcap_path: &str) -> (Vec<u64>, Vec<(u64, u64)>) {
    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(pcap_path, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        let mut counts = Vec::new();
        let mut bytes = Vec::new();
        loop {
            match r.poll() {
                Ok(Some(b)) => {
                    counts.push(b.len() as u64);
                    bytes.push((b.total_bytes() as u64, b.len() as u64));
                }
                Ok(None) => break,
                Err(e) => panic!("poll error: {e}"),
            }
        }
        (counts, bytes)
    })
}

// ─── unit: parser ───────────────────────────────────────────

#[test]
fn test_parser_tcp_syn() {
    let pkt = tcp_pkt("192.168.1.1", "10.0.0.1", 12345, 80, 0);
    let parsed = PacketParser::parse(&pkt, 1000).unwrap().unwrap();
    assert_eq!(parsed.src_ip.to_string(), "192.168.1.1");
    assert_eq!(parsed.dst_ip.to_string(), "10.0.0.1");
    assert_eq!(parsed.src_port, 12345);
    assert_eq!(parsed.dst_port, 80);
    assert_eq!(parsed.protocol, 6);
    assert_eq!(parsed.tcp_flags, 0x02); // SYN
    assert_eq!(parsed.total_len as usize, pkt.len());
}

#[test]
fn test_parser_udp() {
    let pkt = udp_pkt("10.0.0.1", "10.0.0.2", 53, 12345, 100);
    let parsed = PacketParser::parse(&pkt, 2000).unwrap().unwrap();
    assert_eq!(parsed.protocol, 17); // UDP
    assert_eq!(parsed.total_len as usize, pkt.len());
}

#[test]
fn test_parser_short_packet_returns_none() {
    assert!(PacketParser::parse(&[0u8; 10], 0).unwrap().is_none());
}

// ─── integration: parser + flow aggregation ─────────────────

#[test]
fn test_same_flow_aggregates() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("agg.pcap");
    let path_s = path.to_str().unwrap();

    // 3 packets, same 5-tuple → should be 1 flow
    let p1 = tcp_pkt("192.168.1.1", "10.0.0.1", 8080, 80, 100);
    let p2 = tcp_pkt("192.168.1.1", "10.0.0.1", 8080, 80, 200);
    let p3 = tcp_pkt("192.168.1.1", "10.0.0.1", 8080, 80, 150);
    gen_pcap(&path, &[(0, 0, &p1), (0, 100_000, &p2), (0, 200_000, &p3)]);

    let ft = Arc::new(PartitionedFlowTable::new(4, 10000));
    let mut proc = PacketProcessor::new(ft.clone());

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        while let Ok(Some(b)) = r.poll() {
            proc.process_batch(&b);
        }
    });

    let stats = proc.stats();
    assert_eq!(stats.packets_parsed, 3, "all 3 packets parsed");
    assert_eq!(stats.new_flows, 1, "same 5-tuple = 1 flow");
    assert_eq!(stats.updated_flows, 2, "2 updates after first");
    assert_eq!(ft.len(), 1, "flow table has 1 entry");
}

#[test]
fn test_multiple_flows() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("multi.pcap");
    let path_s = path.to_str().unwrap();

    let mut pkts = vec![];
    // 100 unique 5-tuples
    for i in 0..100 {
        let src = format!("192.168.{}.{}", (i / 254) + 1, (i % 254) + 1);
        pkts.push(tcp_pkt(&src, "10.0.0.1", (1000 + i) as u16, 80, 64));
    }

    let mut entries = vec![];
    for (idx, pkt) in pkts.iter().enumerate() {
        entries.push((0u32, idx as u32, pkt.as_slice()));
    }
    gen_pcap(&path, &entries);

    let ft = Arc::new(PartitionedFlowTable::new(4, 10000));
    let mut proc = PacketProcessor::new(ft.clone());

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        while let Ok(Some(b)) = r.poll() {
            proc.process_batch(&b);
        }
    });

    assert_eq!(proc.stats().new_flows, 100);
    assert_eq!(proc.stats().packets_parsed, 100);
    assert_eq!(ft.len(), 100);
}

// ─── integration: eviction ──────────────────────────────────

#[test]
fn test_eviction_produces_flow_events() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("evict.pcap");
    let path_s = path.to_str().unwrap();

    // Packets with timestamps spread over 5 seconds
    let p1 = tcp_pkt("10.0.0.1", "10.0.0.2", 80, 12345, 100);
    let p2 = tcp_pkt("10.0.0.3", "10.0.0.4", 443, 54321, 200);
    gen_pcap(&path, &[(0, 0, &p1), (5, 0, &p2)]);

    let ft = Arc::new(PartitionedFlowTable::new(4, 10000));
    let mut proc = PacketProcessor::new(ft.clone());

    let (tx, mut rx) = tokio::sync::mpsc::channel::<proto_gen::FlowEvent>(1000);
    let eviction_config = EvictionConfig {
        idle_timeout: Duration::from_secs(1),
        active_timeout: Duration::from_secs(30),
        scan_interval: Duration::from_millis(100),
        tenant_id: "test".into(),
        probe_id: "test".into(),
        run_id: "test_run".into(),
        feature_set_id: "v1".into(),
        use_timewheel: false,
        timewheel_slot_duration: Duration::from_secs(1),
        timewheel_slot_count: 10,
    };

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        while let Ok(Some(b)) = r.poll() {
            proc.process_batch(&b);
        }

        assert_eq!(proc.stats().new_flows, 2);

        // Run eviction
        let eviction = Eviction::new(eviction_config, ft.clone(), tx);
        tokio::spawn(async move { eviction.run().await });

        // Wait for eviction to fire (idle_timeout = 1s)
        tokio::time::sleep(Duration::from_secs(2)).await;

        let mut events = Vec::new();
        while let Ok(e) = rx.try_recv() {
            events.push(e);
        }

        assert!(!events.is_empty(), "eviction should produce events");
        for e in &events {
            assert!(!e.flow_id.is_empty(), "flow_id must be set");
            if let Some(ref hdr) = e.header {
                assert_eq!(hdr.tenant_id, "test");
                assert_eq!(hdr.run_id, "test_run");
            }
        }
    });
}

// ─── performance: throughput benchmarks ─────────────────────

#[test]
fn bench_small_packets() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("small.pcap");
    let path_s = path.to_str().unwrap();

    let pkt = tcp_pkt("192.168.1.1", "10.0.0.1", 80, 8080, 0); // min size
    let entries: Vec<_> = (0..100_000)
        .map(|i| (0u32, i as u32, pkt.as_slice()))
        .collect();
    gen_pcap(&path, &entries);

    let start = Instant::now();
    let (counts, _) = run_replayer(path_s);
    let elapsed = start.elapsed();

    let total: u64 = counts.iter().sum();
    let pps = total as f64 / elapsed.as_secs_f64();
    println!(
        "Small packets (64B): {total} pkts, {:.0} pps, {:.3}s",
        pps,
        elapsed.as_secs_f64()
    );
    assert!(pps > 50_000.0, "small packet throughput {pps:.0} < 50k");
    assert_eq!(total, 100_000);
}

#[test]
fn bench_large_packets() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("large.pcap");
    let path_s = path.to_str().unwrap();

    let pkt = tcp_pkt("192.168.1.1", "10.0.0.1", 80, 8080, 1400); // near MTU
    let entries: Vec<_> = (0..50_000)
        .map(|i| (0u32, i as u32, pkt.as_slice()))
        .collect();
    gen_pcap(&path, &entries);

    let start = Instant::now();
    let (counts, _) = run_replayer(path_s);
    let elapsed = start.elapsed();

    let total: u64 = counts.iter().sum();
    let pps = total as f64 / elapsed.as_secs_f64();
    println!(
        "Large packets (1514B): {total} pkts, {:.0} pps, {:.3}s",
        pps,
        elapsed.as_secs_f64()
    );
    assert_eq!(total, 50_000);
    assert!(pps > 10_000.0);
}

#[test]
fn bench_flow_aggregation() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("flowagg.pcap");
    let path_s = path.to_str().unwrap();

    // 10 flows, each with 1000 packets → 10K total
    let mut pkts: Vec<Vec<u8>> = vec![];
    for flow_id in 0..10 {
        let src = format!("192.168.{}.{}", (flow_id / 254) + 1, (flow_id % 254) + 1);
        pkts.push(tcp_pkt(&src, "10.0.0.1", (1000 + flow_id) as u16, 80, 100));
    }
    let mut entries = vec![];
    for (flow_id, pkt) in pkts.iter().enumerate() {
        for seq in 0..1000 {
            entries.push((
                0u32,
                (flow_id as u32 * 1000 + seq as u32) as u32,
                pkt.as_slice(),
            ));
        }
    }
    gen_pcap(&path, &entries);

    let ft = Arc::new(PartitionedFlowTable::new(4, 10000));
    let mut proc = PacketProcessor::new(ft.clone());

    let start = Instant::now();
    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        while let Ok(Some(b)) = r.poll() {
            proc.process_batch(&b);
        }
    });
    let elapsed = start.elapsed();

    let stats = proc.stats();
    assert_eq!(stats.packets_parsed, 10_000);
    assert_eq!(stats.new_flows, 10);
    assert_eq!(stats.updated_flows, 9_990);
    assert_eq!(ft.len(), 10);
    println!(
        "Flow aggregation: 10K pkts/10 flows, {:.3}s, {:.0} pps",
        elapsed.as_secs_f64(),
        10_000.0 / elapsed.as_secs_f64()
    );
}

// ─── boundary cases ─────────────────────────────────────────

#[test]
fn test_mixed_tcp_udp() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("mixed.pcap");
    let path_s = path.to_str().unwrap();

    let tcp = tcp_pkt("192.168.1.1", "10.0.0.1", 80, 8080, 100);
    let udp = udp_pkt("192.168.1.2", "10.0.0.2", 53, 12345, 50);
    gen_pcap(&path, &[(0, 0, &tcp), (0, 1000, &udp)]);

    let ft = Arc::new(PartitionedFlowTable::new(4, 100));
    let mut proc = PacketProcessor::new(ft.clone());

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        while let Ok(Some(b)) = r.poll() {
            proc.process_batch(&b);
        }
    });

    assert_eq!(proc.stats().packets_parsed, 2);
    assert_eq!(ft.len(), 2); // TCP + UDP = 2 unique flows
}

#[test]
fn test_empty_pcap_handled() {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("empty.pcap");
    let path_s = path.to_str().unwrap();
    gen_pcap(&path, &[]);

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let mut r = PcapReplayer::new(path_s, ReplaySpeed::MaxSpeed, false).unwrap();
        r.start().await.unwrap();
        let mut batches = 0;
        while let Ok(Some(_)) = r.poll() {
            batches += 1;
        }
        assert_eq!(batches, 0, "empty pcap yields zero batches");
    });
}
