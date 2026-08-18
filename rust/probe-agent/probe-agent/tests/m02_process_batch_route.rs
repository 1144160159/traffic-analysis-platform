use probe_agent::aggregator::{PacketProcessor, PartitionedFlowTable};
use probe_agent::capture::{CaptureTimestamp, TimestampPrecision};
use probe_agent::config::ParserRoute;
use std::sync::Arc;

fn ipv4_udp() -> Vec<u8> {
    let mut frame = vec![0; 14 + 20 + 8];
    frame[12..14].copy_from_slice(&0x0800u16.to_be_bytes());
    let ip = 14;
    frame[ip] = 0x45;
    frame[ip + 2..ip + 4].copy_from_slice(&28u16.to_be_bytes());
    frame[ip + 8] = 64;
    frame[ip + 9] = 17;
    frame[ip + 12..ip + 16].copy_from_slice(&[10, 0, 0, 1]);
    frame[ip + 16..ip + 20].copy_from_slice(&[10, 0, 0, 2]);
    let udp = ip + 20;
    frame[udp..udp + 2].copy_from_slice(&12345u16.to_be_bytes());
    frame[udp + 2..udp + 4].copy_from_slice(&53u16.to_be_bytes());
    frame[udp + 4..udp + 6].copy_from_slice(&8u16.to_be_bytes());
    frame
}

fn run(route: ParserRoute) -> (usize, u64, u64, u64) {
    let table = Arc::new(PartitionedFlowTable::new(4, 16));
    let mut processor = PacketProcessor::new(table.clone()).with_parser_route(route);
    let frame = ipv4_udp();
    processor
        .process_frame_for_test(
            &frame,
            CaptureTimestamp::from_unix_parts(
                1_700_000_000,
                123_456,
                TimestampPrecision::Microsecond,
            )
            .unwrap(),
        )
        .unwrap();
    (
        table.len(),
        table.stats().updates,
        processor.stats().new_flows,
        processor.stats().packets_failed,
    )
}

#[test]
fn m02_process_batch_route_matrix() {
    assert_eq!(run(ParserRoute::Full), (1, 1, 1, 0));
    assert_eq!(run(ParserRoute::Fast), (1, 1, 1, 0));
    assert_eq!(run(ParserRoute::Shadow), (1, 1, 1, 0));
}
