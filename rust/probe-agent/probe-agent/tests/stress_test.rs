use probe_agent::aggregator::{PacketProcessor, PartitionedFlowTable};
use probe_agent::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
use probe_agent::capture::Capturer;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Instant;
use tempfile::TempDir;

#[tokio::test]
async fn stress_test_pcap_replay() {
    let (path, expected_packets, _temp_dir) = prepare_stress_pcap();
    let min_pps = std::env::var("PROBE_STRESS_MIN_PPS")
        .ok()
        .and_then(|value| value.parse::<f64>().ok())
        .unwrap_or(1_000.0);

    println!("╔══════════════════════════════════════════════╗");
    println!("║     Probe Agent 压力测试 — PCAP 离线回放      ║");
    println!("╚══════════════════════════════════════════════╝");

    // 1. Create replayer at max speed
    let mut replayer =
        PcapReplayer::new(path.to_str().unwrap(), ReplaySpeed::MaxSpeed, false)
            .expect("Failed to create replayer");

    // 2. Create flow table
    let partitions = 16usize;
    let capacity = (expected_packets.unwrap_or(50_000) as usize / partitions).max(1);
    let flow_table = Arc::new(PartitionedFlowTable::new(partitions, capacity));

    // 3. Create processor
    let mut processor = PacketProcessor::new(flow_table.clone());

    // 4. Start and measure
    replayer.start().await.expect("Failed to start replayer");

    let start = Instant::now();
    let mut total_packets: u64 = 0;
    let mut total_bytes: u64 = 0;
    let mut batches: u64 = 0;

    loop {
        match replayer.poll() {
            Ok(Some(batch)) => {
                let count = batch.len() as u64;
                let bytes = batch.total_bytes() as u64;
                total_packets += count;
                total_bytes += bytes;
                batches += 1;
                processor.process_batch(&batch);
            }
            Ok(None) => break,
            Err(e) => panic!("Poll error: {}", e),
        }
    }

    let elapsed = start.elapsed();
    let stats = processor.stats();

    // ====== Report ======
    let pps = total_packets as f64 / elapsed.as_secs_f64();
    let mbps = (total_bytes as f64 * 8.0) / (elapsed.as_secs_f64() * 1_000_000.0);
    let active_flows = flow_table.len();

    println!();
    println!("═══════════ 压力测试报告 ═══════════");
    println!("PCAP 文件:    {}", path.display());
    println!(
        "文件大小:    {:.1} MB",
        std::fs::metadata(&path).unwrap().len() as f64 / 1_048_576.0
    );
    println!("────────────────────────────────────");
    println!("总耗时:      {:.3} s", elapsed.as_secs_f64());
    println!("总包数:      {}", total_packets);
    println!("总字节:      {:.1} MB", total_bytes as f64 / 1_048_576.0);
    println!("批次数:      {}", batches);
    println!("────────────────────────────────────");
    println!("吞吐量:      {:.0} pps (packets/sec)", pps);
    println!("带宽:        {:.1} Mbps", mbps);
    println!(
        "平均每包:    {:.1} bytes",
        total_bytes as f64 / total_packets as f64
    );
    println!("────────────────────────────────────");
    println!("解析成功:    {}", stats.packets_parsed);
    println!("解析失败:    {}", stats.packets_failed);
    println!("新建Flow:    {}", stats.new_flows);
    println!("更新Flow:    {}", stats.updated_flows);
    println!("活跃Flow:    {}", active_flows);
    println!("解析成功率:  {:.2}%", stats.parse_success_rate() * 100.0);
    println!("新流比例:    {:.2}%", stats.new_flow_ratio() * 100.0);
    println!("────────────────────────────────────");
    println!(
        "内存估算:    {} partitions × ~{:.0} MB = ~{:.0} MB",
        partitions,
        (flow_table.len() as f64 * 256.0) / (1024.0 * 1024.0),
        (flow_table.len() as f64 * 256.0) / (1024.0 * 1024.0) * partitions as f64
    );
    println!("══════════════════════════════════════");

    // Assertions
    assert!(pps > min_pps, "PPS too low: {:.0} < {:.0}", pps, min_pps);
    if let Some(expected) = expected_packets {
        assert_eq!(total_packets, expected, "Unexpected replay packet count");
    } else {
        assert!(total_packets > 0, "External PCAP should contain packets");
    }
    assert!(stats.packets_parsed > 0, "No packets parsed");
    assert!(active_flows > 0, "No flows created");
    println!("\n✅ 压力测试通过");
}

fn prepare_stress_pcap() -> (PathBuf, Option<u64>, Option<TempDir>) {
    if let Ok(path) = std::env::var("PROBE_STRESS_PCAP") {
        let expected = std::env::var("PROBE_STRESS_EXPECTED_PACKETS")
            .ok()
            .and_then(|value| value.parse::<u64>().ok());
        return (PathBuf::from(path), expected, None);
    }

    let packets = std::env::var("PROBE_STRESS_PACKETS")
        .ok()
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(50_000);
    let temp_dir = TempDir::new().expect("Failed to create temp dir for stress PCAP");
    let path = temp_dir.path().join("stress_test.pcap");
    create_stress_pcap(&path, packets);

    (path, Some(packets), Some(temp_dir))
}

fn create_stress_pcap(path: &Path, packets: u64) {
    let mut file = std::fs::File::create(path).expect("Failed to create stress PCAP");

    file.write_all(&0xa1b2c3d4u32.to_le_bytes()).unwrap();
    file.write_all(&2u16.to_le_bytes()).unwrap();
    file.write_all(&4u16.to_le_bytes()).unwrap();
    file.write_all(&0i32.to_le_bytes()).unwrap();
    file.write_all(&0u32.to_le_bytes()).unwrap();
    file.write_all(&65_535u32.to_le_bytes()).unwrap();
    file.write_all(&1u32.to_le_bytes()).unwrap();

    for i in 0..packets {
        let payload = make_eth_ip_tcp_packet(i);
        let ts_sec = (i / 10_000) as u32;
        let ts_usec = ((i % 10_000) * 100) as u32;
        let incl_len = payload.len() as u32;

        file.write_all(&ts_sec.to_le_bytes()).unwrap();
        file.write_all(&ts_usec.to_le_bytes()).unwrap();
        file.write_all(&incl_len.to_le_bytes()).unwrap();
        file.write_all(&incl_len.to_le_bytes()).unwrap();
        file.write_all(&payload).unwrap();
    }
}

fn make_eth_ip_tcp_packet(index: u64) -> Vec<u8> {
    let payload_len = 40 + (index as usize % 64);
    let mut packet = Vec::with_capacity(14 + 20 + 20 + payload_len);

    packet.extend_from_slice(&[0x02, 0x00, 0x00, 0x00, 0x00, (index % 255) as u8]);
    packet.extend_from_slice(&[0x02, 0x00, 0x00, 0x00, 0x01, (index % 255) as u8]);
    packet.extend_from_slice(&0x0800u16.to_be_bytes());

    packet.push(0x45);
    packet.push(0x00);
    let total_len = (20 + 20 + payload_len) as u16;
    packet.extend_from_slice(&total_len.to_be_bytes());
    packet.extend_from_slice(&(index as u16).to_be_bytes());
    packet.extend_from_slice(&0x4000u16.to_be_bytes());
    packet.push(64);
    packet.push(6);
    packet.extend_from_slice(&0u16.to_be_bytes());
    packet.extend_from_slice(&[192, 168, ((index / 254) % 254 + 1) as u8, (index % 254 + 1) as u8]);
    packet.extend_from_slice(&[10, 0, ((index / 512) % 6) as u8, (index % 254 + 1) as u8]);

    let src_port = 1024 + (index % 60_000) as u16;
    let dst_ports = [80u16, 443, 8080, 53, 22, 3306, 6379, 9092];
    let dst_port = dst_ports[index as usize % dst_ports.len()];
    packet.extend_from_slice(&src_port.to_be_bytes());
    packet.extend_from_slice(&dst_port.to_be_bytes());
    packet.extend_from_slice(&(index as u32 + 1).to_be_bytes());
    packet.extend_from_slice(&0u32.to_be_bytes());
    packet.push(0x50);
    packet.push(0x02);
    packet.extend_from_slice(&65_535u16.to_be_bytes());
    packet.extend_from_slice(&0u16.to_be_bytes());
    packet.extend_from_slice(&0u16.to_be_bytes());
    packet.extend(std::iter::repeat((index % 251) as u8).take(payload_len));

    packet
}
