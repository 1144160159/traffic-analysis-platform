use probe_agent::aggregator::{
    canonicalize_observation, EventTimeError, FlowKey, FlowTable, FlowUpdateError, FlowValue,
    ObservationScope, ObservedEndpoints, PacketDirection, PacketInfo,
};
use std::net::{IpAddr, Ipv4Addr};
use std::sync::atomic::Ordering;

fn key() -> FlowKey {
    let identity = canonicalize_observation(ObservedEndpoints {
        src_ip: IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
        dst_ip: IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
        src_port: 12345,
        dst_port: 443,
        protocol: 6,
    })
    .unwrap();
    FlowKey::new(&identity, &ObservationScope::global_l3())
}

#[test]
fn m02_flow_event_time_matrix() {
    let value = FlowValue::default();
    assert_eq!(value.start_time.load(Ordering::Acquire), 0);
    assert_eq!(value.last_seen.load(Ordering::Acquire), 0);

    let first = value
        .apply_event_time(20_000_000, PacketDirection::Forward)
        .unwrap();
    assert!(!first.reordered);
    let reordered = value
        .apply_event_time(10_000_000, PacketDirection::Forward)
        .unwrap();
    assert!(reordered.reordered);
    let duplicate = value
        .apply_event_time(20_000_000, PacketDirection::Forward)
        .unwrap();
    assert!(duplicate.duplicate);
    value
        .apply_event_time(30_000_000, PacketDirection::Backward)
        .unwrap();

    assert_eq!(value.start_time.load(Ordering::Acquire), 10_000);
    assert_eq!(value.last_seen.load(Ordering::Acquire), 30_000);
    assert_eq!(value.last_pkt_time_fwd.load(Ordering::Acquire), 20_000_000);
    assert_eq!(value.last_pkt_time_bwd.load(Ordering::Acquire), 30_000_000);

    let table = FlowTable::new(8);
    let invalid = PacketInfo {
        len: 60,
        tcp_flags: 0,
        direction: PacketDirection::Forward,
        timestamp: 0,
        tos: 0,
    };
    assert_eq!(
        table.update(&key(), &invalid),
        Err(FlowUpdateError::EventTime(EventTimeError::MissingTimestamp))
    );
    assert_eq!(
        table.len(),
        0,
        "invalid event time must not create a ghost flow"
    );

    let historic = PacketInfo {
        timestamp: 1_000_000,
        ..invalid
    };
    table.update(&key(), &historic).unwrap();
    let stored = table.get(&key()).unwrap();
    assert_eq!(stored.start_time.load(Ordering::Acquire), 1_000);
    assert_eq!(stored.last_seen.load(Ordering::Acquire), 1_000);
}
