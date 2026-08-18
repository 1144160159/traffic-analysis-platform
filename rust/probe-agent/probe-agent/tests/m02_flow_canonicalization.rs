use probe_agent::aggregator::{
    canonicalize_observation, compute_community_id, FlowKey, ObservationScope, ObservedEndpoints,
    PacketDirection,
};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

fn endpoints(
    src: IpAddr,
    dst: IpAddr,
    src_port: u16,
    dst_port: u16,
    protocol: u8,
) -> ObservedEndpoints {
    ObservedEndpoints {
        src_ip: src,
        dst_ip: dst,
        src_port,
        dst_port,
        protocol,
    }
}

#[test]
fn m02_flow_canonicalization_matrix() {
    let low = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1));
    let high = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2));

    let tcp_forward = canonicalize_observation(endpoints(low, high, 12345, 443, 6)).unwrap();
    let tcp_reverse = canonicalize_observation(endpoints(high, low, 443, 12345, 6)).unwrap();
    assert_eq!(tcp_forward.community_tuple, tcp_reverse.community_tuple);
    assert_eq!(tcp_forward.packet_direction, PacketDirection::Forward);
    assert_eq!(tcp_reverse.packet_direction, PacketDirection::Backward);
    assert!(tcp_forward.reversible && tcp_reverse.reversible);

    let scope = ObservationScope::global_l3();
    let forward_key = FlowKey::new(&tcp_forward, &scope);
    let reverse_key = FlowKey::new(&tcp_reverse, &scope);
    assert_eq!(forward_key, reverse_key);
    assert_eq!(forward_key.cached_hash(), reverse_key.cached_hash());
    assert_eq!(forward_key.community_id(), reverse_key.community_id());

    let echo_request = canonicalize_observation(endpoints(low, high, 8, 0, 1)).unwrap();
    let echo_reply = canonicalize_observation(endpoints(high, low, 0, 0, 1)).unwrap();
    assert_eq!(echo_request.community_tuple, echo_reply.community_tuple);
    assert_eq!(echo_request.community_tuple.port_a, 8);
    assert!(!echo_request.community_tuple.one_way);
    assert_eq!(echo_request.packet_direction, PacketDirection::Forward);
    assert_eq!(echo_reply.packet_direction, PacketDirection::Backward);
    assert_eq!(
        compute_community_id(&echo_request.community_tuple),
        compute_community_id(&echo_reply.community_tuple)
    );

    let same_ip_request = canonicalize_observation(endpoints(low, low, 8, 0, 1)).unwrap();
    let same_ip_reply = canonicalize_observation(endpoints(low, low, 0, 0, 1)).unwrap();
    assert_eq!(
        same_ip_request.community_tuple,
        same_ip_reply.community_tuple
    );
    assert_eq!(same_ip_request.packet_direction, PacketDirection::Forward);
    assert_eq!(same_ip_reply.packet_direction, PacketDirection::Backward);

    let unpaired_error = canonicalize_observation(endpoints(high, low, 3, 1, 1)).unwrap();
    assert!(unpaired_error.community_tuple.one_way);
    assert!(!unpaired_error.reversible);
    assert_eq!(unpaired_error.community_tuple.ip_a, high);
    assert_eq!(unpaired_error.community_tuple.ip_b, low);
    assert_eq!(unpaired_error.packet_direction, PacketDirection::Forward);

    let mixed = canonicalize_observation(endpoints(low, IpAddr::V6(Ipv6Addr::LOCALHOST), 1, 2, 17));
    assert!(mixed.is_err());
}
