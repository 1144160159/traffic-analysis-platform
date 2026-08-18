use probe_agent::aggregator::{
    canonicalize_observation, FlowAggregationKey, ObservationScope, ObservedEndpoints, ScopePolicy,
};
use std::net::{IpAddr, Ipv4Addr};

fn identity() -> probe_agent::aggregator::CanonicalFlowIdentity {
    canonicalize_observation(ObservedEndpoints {
        src_ip: IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)),
        dst_ip: IpAddr::V4(Ipv4Addr::new(198, 51, 100, 20)),
        src_port: 49152,
        dst_port: 443,
        protocol: 6,
    })
    .unwrap()
}

#[test]
fn m02_flow_aggregation_scope_matrix() {
    let identity = identity();
    let global_a = FlowAggregationKey::new(&identity, &ObservationScope::global_l3());
    let global_b = FlowAggregationKey::new(&identity, &ObservationScope::global_l3());
    assert_eq!(global_a, global_b);

    let interface_a = ObservationScope {
        policy: ScopePolicy::Interface,
        interface: Some("eth0".to_owned()),
        ..ObservationScope::default()
    };
    let interface_b = ObservationScope {
        interface: Some("eth1".to_owned()),
        ..interface_a.clone()
    };
    let interface_key_a = FlowAggregationKey::new(&identity, &interface_a);
    let interface_key_b = FlowAggregationKey::new(&identity, &interface_b);
    assert_ne!(interface_key_a, interface_key_b);
    assert_eq!(
        interface_key_a.community_id(),
        interface_key_b.community_id()
    );

    let vlan_100 = ObservationScope {
        policy: ScopePolicy::InterfaceAndVlan,
        interface: Some("eth0".to_owned()),
        vlan_stack: vec![100],
        ..ObservationScope::default()
    };
    let qinq = ObservationScope {
        vlan_stack: vec![100, 200],
        ..vlan_100.clone()
    };
    let vlan_key = FlowAggregationKey::new(&identity, &vlan_100);
    let qinq_key = FlowAggregationKey::new(&identity, &qinq);
    assert_ne!(vlan_key, qinq_key);
    assert_eq!(vlan_key.community_id(), qinq_key.community_id());
}
