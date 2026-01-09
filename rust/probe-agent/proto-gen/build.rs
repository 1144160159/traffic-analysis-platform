// rust/probe-agent/proto-gen/build.rs
use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .unwrap()
        .parent()
        .unwrap()
        .parent()
        .unwrap()
        .join("proto");

    let proto_files = [
        "traffic/v1/common.proto",
        "traffic/v1/flow.proto",
        "traffic/v1/session.proto",
        "traffic/v1/feature.proto",
        "traffic/v1/detection.proto",
        "traffic/v1/alert.proto",
        "traffic/v1/pcap.proto",
        "traffic/v1/ingest.proto",
        "traffic/v1/campaign.proto",
    ];

    let proto_paths: Vec<PathBuf> = proto_files
        .iter()
        .map(|f| proto_root.join(f))
        .collect();

    // 配置 prost-build
    let mut prost_config = prost_build::Config::new();
    prost_config
        .type_attribute(".", "#[derive(serde::Serialize, serde::Deserialize)]")
        .type_attribute(".", "#[serde(rename_all = \"camelCase\")]")
        .bytes(&["."])
        .compile_well_known_types();

    // 配置 tonic-build
    tonic_build::configure()
        .build_server(false)  // Probe 只需要 client
        .build_client(true)
        .out_dir("src")
        .compile_with_config(
            prost_config,
            &proto_paths,
            &[&proto_root],
        )?;

    // 重新运行条件
    for proto in &proto_files {
        println!("cargo:rerun-if-changed={}", proto_root.join(proto).display());
    }

    Ok(())
}