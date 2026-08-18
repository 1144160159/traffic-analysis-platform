use probe_agent::aggregator::ObservationScope;
use probe_agent::capture::pcap_offline::PcapReader;
use probe_agent::parser::{FlowFields, FlowSampleBuilder, PacketParser, TransportDecodeStatus};
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Deserialize)]
struct Manifest {
    schema_version: String,
    corpus_id: String,
    blind_label_policy: String,
    coverage: Vec<String>,
    fixtures: Vec<Fixture>,
}

#[derive(Debug, Deserialize)]
struct Fixture {
    fixture_id: String,
    relative_path: String,
    category: String,
    sha256: String,
    size_bytes: u64,
    packet_count: usize,
    expected: Expected,
    semantic_label: SemanticLabel,
}

#[derive(Debug, Deserialize)]
struct Expected {
    decoded_count: usize,
    rejected_count: usize,
    aggregation_key_count: usize,
    protocols: Vec<u8>,
    transport_statuses: Vec<String>,
    first_tuple: Option<ExpectedTuple>,
}

#[derive(Debug, Deserialize)]
struct ExpectedTuple {
    src_ip: String,
    dst_ip: String,
    src_port: u16,
    dst_port: u16,
    protocol: u8,
}

#[derive(Debug, Deserialize)]
struct SemanticLabel {
    status: String,
    blind_evaluation_eligible: bool,
}

fn corpus_dir() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../../tests/fixtures/pcap/m03")
        .canonicalize()
        .expect("M03 corpus directory")
}

fn sha256(path: &Path) -> String {
    format!("{:x}", Sha256::digest(fs::read(path).unwrap()))
}

fn status(value: TransportDecodeStatus) -> &'static str {
    match value {
        TransportDecodeStatus::Decoded => "decoded",
        TransportDecodeStatus::FirstFragment => "first_fragment",
        TransportDecodeStatus::Unsupported => "unsupported",
    }
}

fn assert_first_tuple(expected: &ExpectedTuple, fields: &FlowFields) {
    assert_eq!(fields.src_ip.to_string(), expected.src_ip);
    assert_eq!(fields.dst_ip.to_string(), expected.dst_ip);
    assert_eq!(fields.src_port, expected.src_port);
    assert_eq!(fields.dst_port, expected.dst_port);
    assert_eq!(fields.protocol, expected.protocol);
}

#[test]
fn m03_golden_manifest_replays_through_checked_production_pipeline() {
    let root = corpus_dir();
    let manifest: Manifest = serde_json::from_slice(
        &fs::read(root.join("manifest.v1.json")).expect("read M03 corpus manifest"),
    )
    .expect("parse M03 corpus manifest");
    assert_eq!(manifest.schema_version, "1.0.0");
    assert_eq!(manifest.corpus_id, "m03-pcap-golden-v1");
    assert!(manifest.blind_label_policy.contains("forbidden"));
    assert_eq!(manifest.fixtures.len(), 8);
    assert_eq!(
        manifest.coverage.into_iter().collect::<BTreeSet<_>>(),
        [
            "attack_structure",
            "empty",
            "ipv6",
            "large_flow",
            "normal",
            "quic",
            "tls",
            "truncated",
        ]
        .into_iter()
        .map(str::to_string)
        .collect()
    );

    for fixture in manifest.fixtures {
        assert_eq!(
            fixture.semantic_label.status,
            "synthetic_structure_not_ground_truth"
        );
        assert!(!fixture.semantic_label.blind_evaluation_eligible);
        assert_eq!(
            fixture.category == "attack_structure",
            fixture.fixture_id == "attack-scan-shape"
        );
        let path = root.join(&fixture.relative_path);
        assert_eq!(fs::metadata(&path).unwrap().len(), fixture.size_bytes);
        assert_eq!(sha256(&path), fixture.sha256);

        let mut reader = PcapReader::from_file(&path).unwrap();
        let mut packets = 0usize;
        let mut decoded = 0usize;
        let mut rejected = 0usize;
        let mut protocols = BTreeSet::new();
        let mut statuses = BTreeSet::new();
        let mut keys = BTreeSet::new();
        let mut first_fields: Option<FlowFields> = None;
        loop {
            let record = reader.next_packet_checked().unwrap();
            let Some(record) = record else { break };
            packets += 1;
            match PacketParser::decode_flow_fields(&record.bytes) {
                Ok(fields) => {
                    decoded += 1;
                    protocols.insert(fields.protocol);
                    statuses.insert(status(fields.transport_status).to_string());
                    if first_fields.is_none() {
                        first_fields = Some(fields.clone());
                    }
                    let sample = FlowSampleBuilder::build(
                        fields,
                        record.captured_at,
                        &ObservationScope::global_l3(),
                    )
                    .unwrap();
                    keys.insert(sample.key.cached_hash());
                }
                Err(_) => rejected += 1,
            }
        }

        assert_eq!(
            packets, fixture.packet_count,
            "{} packet count",
            fixture.fixture_id
        );
        assert_eq!(
            decoded, fixture.expected.decoded_count,
            "{} decoded",
            fixture.fixture_id
        );
        assert_eq!(
            rejected, fixture.expected.rejected_count,
            "{} rejected",
            fixture.fixture_id
        );
        assert_eq!(
            keys.len(),
            fixture.expected.aggregation_key_count,
            "{} keys",
            fixture.fixture_id
        );
        assert_eq!(protocols, fixture.expected.protocols.into_iter().collect());
        assert_eq!(
            statuses,
            fixture.expected.transport_statuses.into_iter().collect()
        );
        match (&fixture.expected.first_tuple, &first_fields) {
            (Some(expected), Some(fields)) => assert_first_tuple(expected, fields),
            (None, None) => {}
            _ => panic!("{} first tuple presence mismatch", fixture.fixture_id),
        }
    }
}
