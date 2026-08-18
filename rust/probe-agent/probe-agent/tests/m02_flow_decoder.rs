use probe_agent::parser::{
    FastDecodeError, FastFallback, FlowDecodeError, FlowSampleBuilder, PacketParser,
    TransportDecodeStatus,
};
use probe_agent::{
    aggregator::{ObservationScope, ScopePolicy},
    capture::{CaptureTimestamp, CaptureTimestampProvenance},
};

fn ethernet(ether_type: u16, payload: &[u8]) -> Vec<u8> {
    let mut frame = vec![0; 12];
    frame.extend_from_slice(&ether_type.to_be_bytes());
    frame.extend_from_slice(payload);
    frame
}

fn ipv4_udp(options: usize, fragment_bits: u16) -> Vec<u8> {
    let header_len = 20 + options;
    let total_len = header_len + 8;
    let mut packet = vec![0; total_len];
    packet[0] = 0x40 | (header_len / 4) as u8;
    packet[2..4].copy_from_slice(&(total_len as u16).to_be_bytes());
    packet[4..6].copy_from_slice(&0x1234u16.to_be_bytes());
    packet[6..8].copy_from_slice(&fragment_bits.to_be_bytes());
    packet[8] = 64;
    packet[9] = 17;
    packet[12..16].copy_from_slice(&[10, 0, 0, 1]);
    packet[16..20].copy_from_slice(&[10, 0, 0, 2]);
    packet[header_len..header_len + 2].copy_from_slice(&12345u16.to_be_bytes());
    packet[header_len + 2..header_len + 4].copy_from_slice(&53u16.to_be_bytes());
    packet[header_len + 4..header_len + 6].copy_from_slice(&8u16.to_be_bytes());
    packet
}

fn ipv6_udp_with_hop_header() -> Vec<u8> {
    let mut packet = vec![0; 40 + 8 + 8];
    packet[0] = 0x60;
    packet[4..6].copy_from_slice(&16u16.to_be_bytes());
    packet[6] = 0;
    packet[7] = 64;
    packet[23] = 1;
    packet[39] = 2;
    packet[40] = 17;
    packet[41] = 0;
    packet[48..50].copy_from_slice(&12345u16.to_be_bytes());
    packet[50..52].copy_from_slice(&53u16.to_be_bytes());
    packet[52..54].copy_from_slice(&8u16.to_be_bytes());
    packet
}

fn ipv6_without_next_header() -> Vec<u8> {
    let mut packet = vec![0; 40];
    packet[0] = 0x60;
    packet[6] = 59;
    packet[7] = 64;
    packet[23] = 1;
    packet[39] = 2;
    packet
}

#[test]
fn m02_flow_decoder_matrix() {
    let plain = ethernet(0x0800, &ipv4_udp(0, 0));
    let full = PacketParser::decode_flow_fields(&plain).unwrap();
    let fast = PacketParser::decode_flow_fields_fast(&plain).unwrap();
    assert_eq!(full, fast);
    assert_eq!((full.src_port, full.dst_port), (12345, 53));
    assert_eq!(full.transport_status, TransportDecodeStatus::Decoded);
    assert_eq!(full.application_payload_offset, 42);
    assert_eq!(full.application_payload_end, 42);

    let mut qinq_payload = Vec::new();
    qinq_payload.extend_from_slice(&100u16.to_be_bytes());
    qinq_payload.extend_from_slice(&0x8100u16.to_be_bytes());
    qinq_payload.extend_from_slice(&200u16.to_be_bytes());
    qinq_payload.extend_from_slice(&0x0800u16.to_be_bytes());
    qinq_payload.extend_from_slice(&ipv4_udp(0, 0));
    let qinq = ethernet(0x88a8, &qinq_payload);
    let fields = PacketParser::decode_flow_fields(&qinq).unwrap();
    assert_eq!(fields.vlan_stack, vec![100, 200]);
    assert_eq!(
        PacketParser::decode_flow_fields_fast(&qinq),
        Err(FastDecodeError::Fallback(FastFallback::QinQ))
    );

    let options = ethernet(0x0800, &ipv4_udp(4, 0));
    assert_eq!(
        PacketParser::decode_flow_fields(&options).unwrap().src_port,
        12345
    );
    assert_eq!(
        PacketParser::decode_flow_fields_fast(&options),
        Err(FastDecodeError::Fallback(FastFallback::Ipv4Options))
    );

    let ipv6_extension = ethernet(0x86dd, &ipv6_udp_with_hop_header());
    let fields = PacketParser::decode_flow_fields(&ipv6_extension).unwrap();
    assert_eq!(fields.protocol, 17);
    assert_eq!((fields.src_port, fields.dst_port), (12345, 53));
    assert_eq!(
        PacketParser::decode_flow_fields_fast(&ipv6_extension),
        Err(FastDecodeError::Fallback(FastFallback::Ipv6Extension))
    );

    let non_first = ethernet(0x0800, &ipv4_udp(0, 1));
    assert!(matches!(
        PacketParser::decode_flow_fields(&non_first),
        Err(FlowDecodeError::NonFirstFragment {
            protocol: 17,
            fragment_id: 0x1234
        })
    ));

    let first_fragment = ethernet(0x0800, &ipv4_udp(0, 0x2000));
    let fields = PacketParser::decode_flow_fields(&first_fragment).unwrap();
    assert_eq!(fields.fragment_id, Some(0x1234));
    assert_eq!(
        fields.transport_status,
        TransportDecodeStatus::FirstFragment
    );

    let no_next_header = ethernet(0x86dd, &ipv6_without_next_header());
    let fields = PacketParser::decode_flow_fields(&no_next_header).unwrap();
    assert_eq!(fields.protocol, 59);
    assert_eq!((fields.src_port, fields.dst_port), (0, 0));
    assert_eq!(fields.transport_status, TransportDecodeStatus::Unsupported);
}

#[test]
fn transport_decode_uses_declared_ip_bounds_not_ethernet_padding() {
    let mut truncated_udp = ipv4_udp(0, 0);
    truncated_udp[2..4].copy_from_slice(&24u16.to_be_bytes());
    truncated_udp.extend_from_slice(&[0xaa; 32]);
    assert_eq!(
        PacketParser::decode_flow_fields(&ethernet(0x0800, &truncated_udp)),
        Err(FlowDecodeError::TruncatedTransport { protocol: 17 })
    );

    let mut invalid_udp_len = ipv4_udp(0, 0);
    invalid_udp_len[24..26].copy_from_slice(&7u16.to_be_bytes());
    assert_eq!(
        PacketParser::decode_flow_fields(&ethernet(0x0800, &invalid_udp_len)),
        Err(FlowDecodeError::MalformedTransport {
            protocol: 17,
            reason: "UDP length is shorter than the header"
        })
    );

    let mut truncated_tcp = ipv4_udp(0, 0);
    truncated_tcp[9] = 6;
    assert_eq!(
        PacketParser::decode_flow_fields(&ethernet(0x0800, &truncated_tcp)),
        Err(FlowDecodeError::TruncatedTransport { protocol: 6 })
    );
}

#[test]
fn parsed_vlan_stack_participates_only_in_approved_observation_scopes() {
    fn tagged(vlan: u16) -> Vec<u8> {
        let mut payload = Vec::new();
        payload.extend_from_slice(&vlan.to_be_bytes());
        payload.extend_from_slice(&0x0800u16.to_be_bytes());
        payload.extend_from_slice(&ipv4_udp(0, 0));
        ethernet(0x8100, &payload)
    }

    let timestamp = CaptureTimestamp::from_epoch_micros(
        1_700_000_000_000_000,
        CaptureTimestampProvenance::SourceRecord,
    );
    let global = ObservationScope::global_l3();
    let global_100 = FlowSampleBuilder::build(
        PacketParser::decode_flow_fields(&tagged(100)).unwrap(),
        timestamp,
        &global,
    )
    .unwrap();
    let global_200 = FlowSampleBuilder::build(
        PacketParser::decode_flow_fields(&tagged(200)).unwrap(),
        timestamp,
        &global,
    )
    .unwrap();
    assert_eq!(global_100.key, global_200.key);

    let vlan_scope = ObservationScope {
        policy: ScopePolicy::InterfaceAndVlan,
        interface: Some("eth0".to_string()),
        ..ObservationScope::default()
    };
    let vlan_100 = FlowSampleBuilder::build(
        PacketParser::decode_flow_fields(&tagged(100)).unwrap(),
        timestamp,
        &vlan_scope,
    )
    .unwrap();
    let vlan_200 = FlowSampleBuilder::build(
        PacketParser::decode_flow_fields(&tagged(200)).unwrap(),
        timestamp,
        &vlan_scope,
    )
    .unwrap();
    assert_ne!(vlan_100.key, vlan_200.key);
    assert_eq!(vlan_100.key.community_id(), vlan_200.key.community_id());
}
