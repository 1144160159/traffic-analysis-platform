use probe_agent::aggregator::{
    canonicalize_observation, EvictionConfig, FlowEventIdentity, FlowKey, FlowSnapshot,
    FlowSnapshotError, FlowValue, ObservationScope, ObservedEndpoints, PacketDirection,
    RemovedFlow,
};
use std::net::{IpAddr, Ipv4Addr};

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

fn snapshot() -> FlowSnapshot {
    let value = FlowValue::default();
    value
        .apply_event_time(1_700_000_000_000_000, PacketDirection::Forward)
        .unwrap();
    value
        .packets_fwd
        .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
    value
        .bytes_fwd
        .fetch_add(64, std::sync::atomic::Ordering::Relaxed);
    value.pktlen_stats.update(64);
    FlowSnapshot::try_from_removed(RemovedFlow { key: key(), value }).unwrap()
}

#[test]
fn m02_flow_snapshot_and_identity_matrix() {
    let rejected = FlowSnapshot::try_from_removed(RemovedFlow {
        key: key(),
        value: FlowValue::default(),
    });
    match rejected {
        Err((FlowSnapshotError::EmptyTimeWindow, removed)) => {
            assert_eq!(removed.key, key());
        }
        other => panic!("unexpected snapshot result: {other:?}"),
    }

    let config = EvictionConfig {
        tenant_id: "tenant-a".to_owned(),
        probe_id: "probe-a".to_owned(),
        run_id: "run-a".to_owned(),
        ..EvictionConfig::default()
    };
    let first = FlowEventIdentity::derive(&config, &snapshot());
    let replay = FlowEventIdentity::derive(&config, &snapshot());
    assert_eq!(first, replay);
    assert_eq!(first.revision, 2);
    assert_eq!(first.event_id, first.idempotency_key);
    assert_ne!(first.event_id, first.flow_id);
}
