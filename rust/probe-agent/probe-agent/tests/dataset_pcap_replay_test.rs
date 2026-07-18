use probe_agent::capture::pcap_offline::{PcapReplayer, ReplaySpeed};
use probe_agent::capture::Capturer;
use probe_agent::parser::PacketParser;

/// Replays a real external dataset PCAP through the production offline capturer.
///
/// Run explicitly because the dataset is intentionally outside the repository:
/// TRAFFIC_TEST_PCAP=/absolute/sample.pcap \
///   cargo test -p probe-agent --test dataset_pcap_replay_test -- --ignored --nocapture
#[test]
#[ignore = "requires TRAFFIC_TEST_PCAP pointing to a real dataset file"]
fn replays_external_dataset_pcap() {
    let pcap_path = std::env::var("TRAFFIC_TEST_PCAP")
        .expect("TRAFFIC_TEST_PCAP must point to a readable PCAP file");
    let metadata = std::fs::metadata(&pcap_path).expect("dataset PCAP must be readable");
    assert!(metadata.is_file(), "dataset PCAP path must be a file");
    assert!(
        metadata.len() > 24,
        "dataset PCAP must contain more than a global header"
    );

    let runtime = tokio::runtime::Runtime::new().expect("tokio runtime");
    let (captured, parsed, total_bytes) = runtime.block_on(async {
        let mut replayer = PcapReplayer::new(&pcap_path, ReplaySpeed::MaxSpeed, false)
            .expect("create dataset PCAP replayer");
        replayer.start().await.expect("start dataset PCAP replayer");

        let mut captured = 0usize;
        let mut parsed = 0usize;
        let mut total_bytes = 0usize;
        const MAX_PACKETS: usize = 4_096;

        while captured < MAX_PACKETS {
            match replayer.poll() {
                Ok(Some(batch)) => {
                    for (packet, timestamp) in batch.iter() {
                        captured += 1;
                        total_bytes += packet.len();
                        if PacketParser::parse(packet, timestamp)
                            .expect("packet parser must not fail")
                            .is_some()
                        {
                            parsed += 1;
                        }
                        if captured >= MAX_PACKETS {
                            break;
                        }
                    }
                }
                Ok(None) => break,
                Err(error) => panic!("dataset replay failed: {error}"),
            }
        }
        (captured, parsed, total_bytes)
    });

    assert!(captured > 0, "dataset replay must emit packets");
    assert!(
        parsed > 0,
        "dataset replay must yield at least one supported IP packet"
    );
    assert!(total_bytes > 0, "dataset replay must yield packet bytes");
    println!(
        "DATASET_PCAP_REPLAY_OK path={} file_bytes={} captured={} parsed={} replay_bytes={}",
        pcap_path,
        metadata.len(),
        captured,
        parsed,
        total_bytes
    );
}
