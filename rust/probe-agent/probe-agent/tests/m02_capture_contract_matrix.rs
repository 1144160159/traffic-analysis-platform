use probe_agent::aggregator::{
    canonicalize_observation, FlowAggregationKey, ObservationScope, ObservedEndpoints, ScopePolicy,
};
use probe_agent::archiver::{DurablePcapSpool, JournalObjectState, UploadData, UploadJournal};
use probe_agent::capture::pcap_offline::OfflinePcapManifest;
use sha2::{Digest, Sha256};
use std::net::{IpAddr, Ipv4Addr};
use std::sync::Arc;

const FIXTURE_RECIPE: &[u8] = b"m02-capture-contract-fixture-v1";
const FIXTURE_RECIPE_SHA256: &str =
    "3f4c8e819fb8032bc13d2db647633eb9376f0c52c779da4460a16f2a504b7f6f";

fn sha256_hex(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}

fn packet_record(timestamp_micros: u64, payload: &[u8]) -> Vec<u8> {
    let mut record = Vec::with_capacity(16 + payload.len());
    record.extend_from_slice(&((timestamp_micros / 1_000_000) as u32).to_le_bytes());
    record.extend_from_slice(&((timestamp_micros % 1_000_000) as u32).to_le_bytes());
    record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
    record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
    record.extend_from_slice(payload);
    record
}

#[tokio::test]
async fn m02_capture_contract_matrix() {
    assert_eq!(sha256_hex(FIXTURE_RECIPE), FIXTURE_RECIPE_SHA256);

    let low = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1));
    let high = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2));
    let forward = canonicalize_observation(ObservedEndpoints {
        src_ip: low,
        dst_ip: high,
        src_port: 49_152,
        dst_port: 443,
        protocol: 6,
    })
    .unwrap();
    let reverse = canonicalize_observation(ObservedEndpoints {
        src_ip: high,
        dst_ip: low,
        src_port: 443,
        dst_port: 49_152,
        protocol: 6,
    })
    .unwrap();
    let global_forward = FlowAggregationKey::new(&forward, &ObservationScope::global_l3());
    let global_reverse = FlowAggregationKey::new(&reverse, &ObservationScope::global_l3());
    assert_eq!(global_forward, global_reverse);
    assert_eq!(global_forward.community_id(), global_reverse.community_id());

    let scoped = ObservationScope {
        policy: ScopePolicy::InterfaceAndVlan,
        interface: Some("eth0".to_owned()),
        vlan_stack: vec![100, 200],
        ..ObservationScope::default()
    };
    let scoped_key = FlowAggregationKey::new(&forward, &scoped);
    assert_ne!(global_forward, scoped_key);
    assert_eq!(global_forward.community_id(), scoped_key.community_id());

    let root = tempfile::tempdir().unwrap();
    let journal = Arc::new(UploadJournal::new(root.path().join("journal")).unwrap());
    let spool = DurablePcapSpool::new(root.path().join("spool"), journal.clone(), 1).unwrap();
    let timestamp = 7_000_042;
    let upload = UploadData {
        data: packet_record(timestamp, FIXTURE_RECIPE),
        ts_start: timestamp,
        ts_end: timestamp,
        packet_count: 1,
    };
    let first = spool
        .persist_rotated(upload.clone(), "tenant-a", "probe-a")
        .await
        .unwrap();
    let replay = spool
        .persist_rotated(upload, "tenant-a", "probe-a")
        .await
        .unwrap();
    assert_eq!(first, replay, "same capture must retain one spool identity");
    assert_eq!(journal.size().unwrap(), 1);
    let entry = journal.get_entry(&first.task_id).unwrap().unwrap();
    assert_eq!(entry.object_state, JournalObjectState::Pending);
    assert_eq!(entry.packet_count, 1);
    assert_eq!(
        entry.sha256,
        sha256_hex(&std::fs::read(&first.local_path).unwrap())
    );

    let inconsistent = UploadData {
        data: packet_record(timestamp, FIXTURE_RECIPE),
        ts_start: timestamp,
        ts_end: timestamp,
        packet_count: 2,
    };
    assert!(spool
        .persist_rotated(inconsistent, "tenant-a", "probe-a")
        .await
        .unwrap_err()
        .to_string()
        .contains("count/time manifest"));

    let manifest_path = root.path().join("manifest.json");
    let escaped_manifest = serde_json::json!({
        "schema_version": "1.0.0",
        "dataset_id": "m02-fixture",
        "run_id": "m02-run",
        "base_dir": ".",
        "entries": [{
            "entry_id": "escape",
            "relative_path": "../outside.pcap",
            "sha256": "a".repeat(64),
            "size_bytes": 1,
            "byte_order": "little_endian",
            "timestamp_precision": "microsecond",
            "link_type": 1,
            "packet_count": 1
        }]
    });
    std::fs::write(
        &manifest_path,
        serde_json::to_vec(&escaped_manifest).unwrap(),
    )
    .unwrap();
    assert!(OfflinePcapManifest::load_and_validate(&manifest_path)
        .unwrap_err()
        .to_string()
        .contains("PCAP_MANIFEST_PATH_ESCAPE"));
}
