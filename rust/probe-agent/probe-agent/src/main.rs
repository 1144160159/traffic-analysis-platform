use anyhow::{Context, Result};
use probe_agent::config::CaptureConfig;
use probe_agent::metrics;
use probe_agent::shutdown::ShutdownHandle;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tracing::{debug, error, info, trace, warn};
use tracing_subscriber::{self, layer::SubscriberExt, util::SubscriberInitExt, EnvFilter};

use probe_agent::aggregator::{
    Eviction, EvictionConfig, GenerationalConfig, GenerationalFlowTable, PacketProcessor,
    PartitionedFlowTable,
};
use probe_agent::archiver::{
    TripleBuffer, TripleBufferConfig, UploadData, UploadTask, Uploader, UploaderConfig,
};
use probe_agent::capture::{create_capturer, AfPacketCapture, Capturer};
use probe_agent::config::ProbeConfig;
use probe_agent::interface_monitor::{InterfaceMonitor, InterfaceMonitorConfig};
use probe_agent::sender::{BatchConfig, BatchSender, GrpcSender, GrpcSenderConfig};
use probe_agent::shutdown::ShutdownManager;

use proto_gen::FlowEvent;

const FLOW_CHANNEL_SIZE: usize = 100_000;
const BATCH_CHANNEL_SIZE: usize = 1_000;
const UPLOAD_CHANNEL_SIZE: usize = 100;

#[tokio::main]
async fn main() -> Result<()> {
    let config = load_config()?;

    init_logging(&config);
    print_banner();

    apply_cpu_affinity(&config)?;

    let shutdown_manager = ShutdownManager::new();

    if let Err(e) = metrics::register_metrics() {
        warn!("Failed to register metrics: {}", e);
    }

    register_probe(&config).await?;

    let components = create_components(&config, &shutdown_manager).await?;
    start_components(components, &config, &shutdown_manager).await?;

    wait_for_shutdown(&shutdown_manager, &config).await;

    info!("Probe Agent stopped successfully");
    Ok(())
}

fn init_logging(config: &ProbeConfig) {
    let fmt_layer = tracing_subscriber::fmt::layer()
        .json()
        .flatten_event(false)
        .with_current_span(true)
        .with_span_list(true)
        .with_target(true)
        .with_thread_ids(true)
        .with_thread_names(true);

    let filter = EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| EnvFilter::new("probe_agent=info,tower=warn"));

    let probe_span = tracing::info_span!(
        "probe",
        probe_id = %config.probe_id,
        tenant_id = %config.tenant_id,
        run_id = %config.run_id.as_deref().unwrap_or("realtime"),
    );

    tracing_subscriber::registry()
        .with(filter)
        .with(fmt_layer)
        .init();

    let _guard = probe_span.enter();
    std::mem::forget(_guard);
}

fn print_banner() {
    info!("╔══════════════════════════════════════════════════╗");
    info!(
        "║        Probe Agent v{}                    ║",
        env!("CARGO_PKG_VERSION")
    );
    info!("║   High-Performance Network Traffic Collector     ║");
    info!("╚══════════════════════════════════════════════════╝");
}

fn load_config() -> Result<ProbeConfig> {
    let config_path = std::env::args()
        .nth(1)
        .unwrap_or_else(|| "config.yaml".to_string());

    info!("Loading configuration from: {}", config_path);
    let config = ProbeConfig::from_file(&config_path)?;

    info!(
        "Configuration loaded: tenant={}, probe={}, interface={}, run_id={}",
        config.tenant_id,
        config.probe_id,
        config.capture.interface,
        config.run_id.as_deref().unwrap_or("realtime")
    );

    Ok(config)
}

fn apply_cpu_affinity(config: &ProbeConfig) -> Result<()> {
    use probe_agent::cpu_affinity::{get_cpu_topology, set_cpu_affinity, CpuAffinityConfig};

    let topology = get_cpu_topology();
    info!(
        "CPU Topology: {} cores ({} physical), {} NUMA nodes",
        topology.total_cpus,
        topology.physical_cpus,
        topology.numa_nodes.len()
    );

    if !config.capture.cpu_cores.is_empty() {
        let affinity_config = CpuAffinityConfig {
            cpu_cores: config.capture.cpu_cores.clone(),
            numa_aware: config.capture.numa_aware,
        };

        match set_cpu_affinity(&affinity_config) {
            Ok(()) => {
                info!(
                    "✓ CPU affinity set: cores={:?}, numa_aware={}",
                    affinity_config.cpu_cores, affinity_config.numa_aware
                );
            }
            Err(e) => {
                warn!(
                    "Failed to set CPU affinity: {}. Continuing with default scheduling.",
                    e
                );
            }
        }
    } else {
        debug!("CPU affinity not configured, using default scheduling");
    }

    Ok(())
}

async fn register_probe(config: &ProbeConfig) -> Result<()> {
    use proto_gen::ingest_service_client::IngestServiceClient;
    use proto_gen::RegisterProbeRequest;
    use tonic::transport::{Certificate, ClientTlsConfig, Endpoint, Identity};
    use tonic::Request;

    info!(
        "Registering probe: tenant={}, probe={}",
        config.tenant_id, config.probe_id
    );

    let hardware = collect_hardware_info(&config.capture.interface)?;

    let mut endpoint = Endpoint::from_shared(config.sender.gateway_addr.clone())?
        .connect_timeout(Duration::from_secs(10));

    if let (Some(ca_cert), Some(client_cert), Some(client_key)) = (
        &config.sender.tls_ca_cert,
        &config.sender.tls_client_cert,
        &config.sender.tls_client_key,
    ) {
        let ca_pem = tokio::fs::read(ca_cert)
            .await
            .context(format!("Failed to read CA cert: {}", ca_cert))?;
        let client_cert_pem = tokio::fs::read(client_cert)
            .await
            .context(format!("Failed to read client cert: {}", client_cert))?;
        let client_key_pem = tokio::fs::read(client_key)
            .await
            .context(format!("Failed to read client key: {}", client_key))?;

        let tls_config = ClientTlsConfig::new()
            .ca_certificate(Certificate::from_pem(ca_pem))
            .identity(Identity::from_pem(client_cert_pem, client_key_pem))
            .domain_name("ingest-gateway");

        endpoint = endpoint.tls_config(tls_config)?;
    }

    let channel = endpoint
        .connect()
        .await
        .context("Failed to connect to gateway for registration")?;
    let mut client = IngestServiceClient::new(channel);

    let mut request = Request::new(RegisterProbeRequest {
        tenant_id: config.tenant_id.clone(),
        probe_id: config.probe_id.clone(),
        hardware: Some(hardware),
        software_version: env!("CARGO_PKG_VERSION").to_string(),
        build_commit: option_env!("VERGEN_GIT_SHA")
            .unwrap_or("unknown")
            .to_string(),
        build_timestamp: 0,
    });

    if let Some(ref token) = config.sender.auth_token {
        use tonic::metadata::MetadataValue;
        let token_value = MetadataValue::try_from(token.as_str())?;
        request.metadata_mut().insert("x-tenant-token", token_value);
    }

    let response = client
        .register_probe(request)
        .await
        .context("RegisterProbe RPC failed")?;
    let result = response.into_inner();

    if result.success {
        info!("✓ Probe registered successfully: {}", result.message);
    } else {
        warn!("⚠ Probe registration warning: {}", result.message);
    }

    Ok(())
}

fn collect_hardware_info(interface: &str) -> Result<proto_gen::HardwareInfo> {
    use proto_gen::Nic;

    let cpu_model = std::fs::read_to_string("/proc/cpuinfo")
        .ok()
        .and_then(|content| {
            content
                .lines()
                .find(|line| line.starts_with("model name"))
                .and_then(|line| line.split(':').nth(1))
                .map(|s| s.trim().to_string())
        })
        .unwrap_or_else(|| "Unknown".to_string());

    let cpu_cores = num_cpus::get() as u32;

    let memory_mb = std::fs::read_to_string("/proc/meminfo")
        .ok()
        .and_then(|content| {
            content
                .lines()
                .find(|line| line.starts_with("MemTotal"))
                .and_then(|line| line.split_whitespace().nth(1))
                .and_then(|s| s.parse::<u64>().ok())
        })
        .unwrap_or(0)
        / 1024;

    let os_version = std::fs::read_to_string("/etc/os-release")
        .ok()
        .and_then(|content| {
            content
                .lines()
                .find(|line| line.starts_with("PRETTY_NAME"))
                .and_then(|line| line.split('=').nth(1))
                .map(|s| s.trim_matches('"').to_string())
        })
        .unwrap_or_else(|| "Unknown".to_string());

    let nic = Nic {
        name: interface.to_string(),
        mac_address: read_mac_address(interface).unwrap_or_default(),
        pci_address: String::new(),
        driver: read_driver_name(interface).unwrap_or_default(),
        speed_mbps: read_interface_speed(interface).unwrap_or(0),
        driver_version: String::new(),
    };

    Ok(proto_gen::HardwareInfo {
        cpu_model,
        cpu_cores,
        memory_mb,
        os_version,
        nics: vec![nic],
    })
}

fn read_mac_address(interface: &str) -> Option<String> {
    let path = format!("/sys/class/net/{}/address", interface);
    std::fs::read_to_string(path)
        .ok()
        .map(|s| s.trim().to_string())
}

fn read_driver_name(interface: &str) -> Option<String> {
    let path = format!("/sys/class/net/{}/device/driver", interface);
    std::fs::read_link(path)
        .ok()
        .and_then(|p| p.file_name().map(|s| s.to_string_lossy().to_string()))
}

fn read_interface_speed(interface: &str) -> Option<u64> {
    let path = format!("/sys/class/net/{}/speed", interface);
    std::fs::read_to_string(path)
        .ok()
        .and_then(|s| s.trim().parse().ok())
}

struct Components {
    flow_table: Arc<PartitionedFlowTable>,
    triple_buffer: Option<Arc<TripleBuffer>>,
    flow_tx: mpsc::Sender<FlowEvent>,
    flow_rx: mpsc::Receiver<FlowEvent>,
    batch_tx: mpsc::Sender<Vec<FlowEvent>>,
    batch_rx: mpsc::Receiver<Vec<FlowEvent>>,
    upload_tx: mpsc::Sender<UploadTask>,
    upload_rx: mpsc::Receiver<UploadTask>,
    grpc_sender: Arc<GrpcSender>,
    interface_monitor: Arc<InterfaceMonitor>,
    uploader: Option<Arc<Uploader>>,
}

async fn create_components(
    config: &ProbeConfig,
    _shutdown_manager: &Arc<ShutdownManager>,
) -> Result<Components> {
    let num_partitions = num_cpus::get().max(4).next_power_of_two();
    let _generational_table: Option<Arc<GenerationalFlowTable>> =
        if config.aggregator.use_generational {
            let gen_config = GenerationalConfig {
                young_capacity: config.aggregator.flow_capacity / 2,
                old_capacity: config.aggregator.flow_capacity / 4,
                tenured_capacity: config.aggregator.flow_capacity / 4,
                idle_timeout: Duration::from_secs(config.aggregator.idle_timeout_sec),
                active_timeout: Duration::from_secs(config.aggregator.active_timeout_sec),
                ..Default::default()
            };
            let young = gen_config.young_capacity;
            let old = gen_config.old_capacity;
            let tenured = gen_config.tenured_capacity;
            let gen = Arc::new(GenerationalFlowTable::new(num_partitions, gen_config));
            info!(
                "Generational flow table created: {} partitions, young={}/old={}/tenured={} total",
                num_partitions, young, old, tenured
            );
            Some(gen)
        } else {
            None
        };
    let capacity_per_partition = config.aggregator.flow_capacity / num_partitions;
    // 当使用分代表时，用 young 表作为主处理表
    let flow_table: Arc<PartitionedFlowTable> = _generational_table
        .as_ref()
        .map(|g| g.young_table().clone())
        .unwrap_or_else(|| {
            Arc::new(PartitionedFlowTable::new(
                num_partitions,
                capacity_per_partition,
            ))
        });
    // 保存分代表引用用于后台任务 (promotion/demotion)
    if let Some(ref gen) = _generational_table {
        let old_cap = gen.gen_config().old_capacity;
        let tenured_cap = gen.gen_config().tenured_capacity;
        info!(
            "Generational mode: using young table for active processing (old={}, tenured={})",
            old_cap, tenured_cap
        );
    } else {
        info!(
            "Partitioned flow table: {} partitions × {} capacity = {} total",
            num_partitions,
            capacity_per_partition,
            num_partitions * capacity_per_partition
        );
    }

    let (flow_tx, flow_rx) = mpsc::channel::<FlowEvent>(FLOW_CHANNEL_SIZE);
    let (batch_tx, batch_rx) = mpsc::channel::<Vec<FlowEvent>>(BATCH_CHANNEL_SIZE);
    let (upload_tx, upload_rx) = mpsc::channel::<UploadTask>(UPLOAD_CHANNEL_SIZE);

    let (triple_buffer, uploader) = if config.archiver.enabled {
        let buffer_config = TripleBufferConfig {
            buffer_size: config.archiver.buffer_size_mb * 1024 * 1024,
            max_duration: Duration::from_secs(config.archiver.rotation_interval_sec),
            max_packets: 10_000_000,
            enable_fallback: true,
            fallback_path: format!("{}/pcap_overflow", config.archiver.cache_path),
            max_retries: 3,
            retry_delay: Duration::from_millis(10),
        };

        let buffer = Arc::new(TripleBuffer::new(buffer_config));

        let mut uploader_config = UploaderConfig::from(&config.archiver);
        uploader_config.gateway_addr = Some(config.sender.gateway_addr.clone());
        uploader_config.tls_ca_cert = config.sender.tls_ca_cert.clone();
        uploader_config.tls_client_cert = config.sender.tls_client_cert.clone();
        uploader_config.tls_client_key = config.sender.tls_client_key.clone();
        uploader_config.auth_token = config.sender.auth_token.clone();

        let mut uploader = Uploader::new(uploader_config).context("Failed to create uploader")?;

        if let Err(e) = uploader.connect_gateway().await {
            warn!("Failed to connect to gateway for metadata upload: {}", e);
        }

        let uploader = Arc::new(uploader);

        info!(
            "PCAP archiver enabled: buffer_size={}MB, rotation={}s",
            config.archiver.buffer_size_mb, config.archiver.rotation_interval_sec
        );

        (Some(buffer), Some(uploader))
    } else {
        info!("PCAP archiver disabled");
        (None, None)
    };

    let mut grpc_config = GrpcSenderConfig::from(&config.sender);
    grpc_config.tenant_id = Some(config.tenant_id.clone());
    grpc_config.probe_id = Some(config.probe_id.clone());

    let grpc_sender = Arc::new(
        GrpcSender::new(grpc_config)
            .await
            .context("Failed to create gRPC sender")?,
    );

    let monitor_config = InterfaceMonitorConfig {
        interfaces: vec![config.capture.interface.clone()],
        poll_interval: Duration::from_secs(10),
        enabled: true,
    };
    let interface_monitor = Arc::new(InterfaceMonitor::new(monitor_config));

    Ok(Components {
        flow_table,
        triple_buffer,
        flow_tx,
        flow_rx,
        batch_tx,
        batch_rx,
        upload_tx,
        upload_rx,
        grpc_sender,
        interface_monitor,
        uploader,
    })
}

async fn start_components(
    components: Components,
    config: &ProbeConfig,
    shutdown_manager: &Arc<ShutdownManager>,
) -> Result<()> {
    {
        let mut handle = shutdown_manager.register("interface_monitor", 95).await;
        let monitor = Arc::clone(&components.interface_monitor);

        tokio::spawn(async move {
            let monitor_handle = monitor.clone().start().await;
            handle.wait().await;
            monitor.stop();
            monitor_handle.await.ok();
            handle.complete().await;
        });
    }

    if config.metrics.enabled {
        let handle = shutdown_manager.register("metrics_server", 90).await;
        let addr = config.metrics.listen_addr.clone();

        tokio::spawn(async move {
            run_metrics_server(handle, addr).await;
        });
    }

    if config.archiver.enabled {
        use probe_agent::archiver::disk_monitor::{DiskMonitor, DiskMonitorConfig};

        let mut handle = shutdown_manager.register("disk_monitor", 85).await;
        let cache_path = config.archiver.cache_path.clone();

        tokio::spawn(async move {
            let monitor_config = DiskMonitorConfig {
                path: cache_path,
                check_interval: Duration::from_secs(60),
                warning_threshold_percent: 80.0,
                critical_threshold_percent: 90.0,
                cleanup_target_percent: 70.0,
                min_cleanup_interval: Duration::from_secs(300),
                min_free_bytes: 10 * 1024 * 1024 * 1024,
                auto_cleanup: true,
            };

            let monitor = Arc::new(DiskMonitor::new(monitor_config));

            tokio::select! {
                _ = monitor.run() => {
                    debug!("Disk monitor finished normally");
                }
                _ = handle.wait() => {
                    info!("Disk monitor shutting down");
                }
            }

            handle.complete().await;
        });
    }

    if let Some(uploader) = components.uploader {
        trace!("Running S3 preflight check...");

        if let Err(e) = uploader.preflight_check().await {
            trace!("🔴 S3 preflight check failed: {}", e);
        } else {
            debug!("✓ S3 preflight check succeeded");
        }
        let handle = shutdown_manager.register("pcap_uploader", 60).await;
        let upload_rx = components.upload_rx;

        tokio::spawn(async move {
            run_pcap_uploader(handle, uploader, upload_rx).await;
        });
    }

    if let Some(buffer) = components.triple_buffer.clone() {
        let handle = shutdown_manager.register("pcap_rotator", 55).await;
        let upload_tx = components.upload_tx.clone();
        let tenant_id = config.tenant_id.clone();
        let probe_id = config.probe_id.clone();

        tokio::spawn(async move {
            run_pcap_rotator(handle, buffer, upload_tx, tenant_id, probe_id).await;
        });
    }

    {
        let handle = shutdown_manager.register("grpc_sender", 50).await;
        let sender = Arc::clone(&components.grpc_sender);
        let batch_rx = components.batch_rx;

        tokio::spawn(async move {
            sender.run(batch_rx).await;
            handle.complete().await;
        });
    }

    {
        let handle = shutdown_manager.register("heartbeat", 55).await;
        let sender = Arc::clone(&components.grpc_sender);
        let monitor = Arc::clone(&components.interface_monitor);

        tokio::spawn(async move {
            run_heartbeat_task(handle, sender, monitor).await;
        });
    }

    {
        let handle = shutdown_manager.register("batch_collector", 40).await;
        let batch_config = BatchConfig {
            batch_size: config.sender.batch_size,
            batch_timeout: config.batch_timeout(),
        };
        let flow_rx = components.flow_rx;
        let batch_tx = components.batch_tx;

        tokio::spawn(async move {
            run_batch_collector(handle, batch_config, flow_rx, batch_tx).await;
        });
    }

    {
        let handle = shutdown_manager.register("eviction", 30).await;
        let eviction_config = EvictionConfig {
            idle_timeout: config.idle_timeout(),
            active_timeout: config.active_timeout(),
            scan_interval: Duration::from_secs(config.aggregator.scan_interval_sec),
            tenant_id: config.tenant_id.clone(),
            probe_id: config.probe_id.clone(),
            run_id: config
                .run_id
                .clone()
                .unwrap_or_else(|| "realtime".to_string()),
            feature_set_id: "v1".to_string(),
            // The time wheel must be scheduled on every flow update. Until that path is
            // wired in, use full scans so idle flows are reliably emitted to Kafka.
            use_timewheel: false,
            timewheel_slot_duration: Duration::from_secs(10),
            timewheel_slot_count: 360,
        };
        let flow_table = components.flow_table.clone();
        let flow_tx = components.flow_tx.clone();

        tokio::spawn(async move {
            run_eviction(handle, eviction_config, flow_table, flow_tx).await;
        });
    }

    {
        let handle = shutdown_manager.register("capture", 10).await;
        let capture_config = config.capture.clone();
        let flow_table = components.flow_table.clone();
        let triple_buffer = components.triple_buffer.clone();
        let pcap_enabled = config.archiver.enabled;

        tokio::spawn(async move {
            run_capture(
                handle,
                capture_config,
                flow_table,
                triple_buffer,
                pcap_enabled,
            )
            .await;
        });
    }

    info!(
        "All {} components started",
        shutdown_manager.component_count().await
    );
    Ok(())
}

async fn run_metrics_server(mut handle: ShutdownHandle, addr: String) {
    info!("Starting metrics server on {}", addr);

    tokio::select! {
        result = metrics::serve_metrics(&addr) => {
            if let Err(e) = result {
                error!("Metrics server error: {}", e);
            }
        }
        _ = handle.wait() => {
            info!("Metrics server shutting down");
        }
    }

    handle.complete().await;
}

async fn run_heartbeat_task(
    mut handle: ShutdownHandle,
    sender: Arc<GrpcSender>,
    monitor: Arc<InterfaceMonitor>,
) {
    let mut ticker = tokio::time::interval(Duration::from_secs(60));

    info!("Heartbeat task started (interval: 60s)");

    loop {
        tokio::select! {
            _ = ticker.tick() => {
                match sender.send_heartbeat(Some(&monitor)).await {
                    Ok(()) => debug!("✓ Heartbeat sent successfully"),
                    Err(e) => warn!("✗ Heartbeat failed: {}", e),
                }
            }
            _ = handle.wait() => {
                info!("Heartbeat task shutting down");
                break;
            }
        }
    }

    handle.complete().await;
}

async fn run_pcap_uploader(
    mut handle: ShutdownHandle,
    uploader: Arc<Uploader>,
    mut rx: mpsc::Receiver<UploadTask>,
) {
    info!("Starting PCAP uploader");

    let mut uploaded_count: u64 = 0;
    let mut error_count: u64 = 0;

    loop {
        tokio::select! {
            Some(task) = rx.recv() => {
                match uploader.upload(task).await {
                    Ok(result) => {
                        uploaded_count += 1;
                        metrics::PCAP_FILES_UPLOADED.inc();
                        debug!("Uploaded PCAP: {} ({} bytes)", result.key, result.compressed_size);
                    }
                    Err(e) => {
                        error_count += 1;
                        error!("Upload failed: {}", e);
                        metrics::PCAP_UPLOAD_ERRORS.inc();
                    }
                }
            }
            _ = handle.wait() => {
                info!("PCAP uploader shutting down");

                while let Ok(task) = rx.try_recv() {
                    if let Err(e) = uploader.upload(task).await {
                        error!("Final upload failed: {}", e);
                    } else {
                        uploaded_count += 1;
                    }
                }

                break;
            }
        }
    }

    info!(
        "PCAP uploader stopped: uploaded={}, errors={}",
        uploaded_count, error_count
    );
    handle.complete().await;
}

async fn run_pcap_rotator(
    mut handle: ShutdownHandle,
    buffer: Arc<TripleBuffer>,
    upload_tx: mpsc::Sender<UploadTask>,
    tenant_id: String,
    probe_id: String,
) {
    info!("Starting PCAP rotator");

    let mut interval = tokio::time::interval(Duration::from_secs(5));
    let mut rotations: u64 = 0;

    loop {
        tokio::select! {
            _ = interval.tick() => {
                buffer.try_rotate();

                if let Some(result) = check_and_get_upload(&buffer).await {
                    let task = UploadTask {
                        data: result.data,
                        ts_start: result.ts_start,
                        ts_end: result.ts_end,
                        packet_count: result.packet_count,
                        tenant_id: tenant_id.clone(),
                        probe_id: probe_id.clone(),
                    };

                    if upload_tx.send(task).await.is_ok() {
                        rotations += 1;

                        if let Some(idx) = buffer.find_uploading_buffer() {
                            buffer.complete_upload(idx);
                        }
                    }
                }
            }
            _ = handle.wait() => {
                info!("PCAP rotator shutting down");

                buffer.force_rotate();

                if let Some(result) = check_and_get_upload(&buffer).await {
                    let task = UploadTask {
                        data: result.data,
                        ts_start: result.ts_start,
                        ts_end: result.ts_end,
                        packet_count: result.packet_count,
                        tenant_id: tenant_id.clone(),
                        probe_id: probe_id.clone(),
                    };

                    upload_tx.send(task).await.ok();

                    if let Some(idx) = buffer.find_uploading_buffer() {
                        buffer.complete_upload(idx);
                    }
                }

                break;
            }
        }
    }

    info!("PCAP rotator stopped: rotations={}", rotations);
    handle.complete().await;
}

async fn check_and_get_upload(buffer: &Arc<TripleBuffer>) -> Option<UploadData> {
    tokio::time::timeout(Duration::from_millis(100), buffer.wait_for_upload())
        .await
        .ok()
        .flatten()
}

async fn run_batch_collector(
    mut handle: ShutdownHandle,
    config: BatchConfig,
    rx: mpsc::Receiver<FlowEvent>,
    tx: mpsc::Sender<Vec<FlowEvent>>,
) {
    info!(
        "Starting batch collector: size={}, timeout={}ms",
        config.batch_size,
        config.batch_timeout.as_millis()
    );

    let sender = BatchSender::new(config, rx, tx);

    tokio::select! {
        _ = sender.run() => {
            debug!("Batch collector finished normally");
        }
        _ = handle.wait() => {
            info!("Batch collector shutting down");
        }
    }

    handle.complete().await;
}

async fn run_eviction(
    mut handle: ShutdownHandle,
    config: EvictionConfig,
    flow_table: Arc<PartitionedFlowTable>,
    flow_tx: mpsc::Sender<FlowEvent>,
) {
    info!(
        "Starting eviction: idle={}s, active={}s, scan={}s, timewheel={}",
        config.idle_timeout.as_secs(),
        config.active_timeout.as_secs(),
        config.scan_interval.as_secs(),
        config.use_timewheel
    );

    let eviction = Eviction::new(config, flow_table, flow_tx);

    tokio::select! {
        _ = eviction.run() => {
            debug!("Eviction finished normally");
        }
        _ = handle.wait() => {
            info!("Eviction shutting down");
        }
    }

    handle.complete().await;
}

async fn run_capture(
    mut handle: ShutdownHandle,
    config: CaptureConfig,
    flow_table: Arc<PartitionedFlowTable>,
    triple_buffer: Option<Arc<TripleBuffer>>,
    pcap_enabled: bool,
) {
    info!(
        "Starting capture on interface: {}, mode: {:?}, pcap: {}",
        config.interface, config.mode, pcap_enabled
    );

    let mut capturer = match create_capturer(&config).await {
        Ok(c) => c,
        Err(e) => {
            error!("Failed to create capturer: {}", e);
            handle.complete_with_error(e.to_string()).await;
            return;
        }
    };

    if let Err(e) = capturer.start().await {
        error!("Failed to start capture: {}", e);

        // If configured for XDP variants, attempt AF_PACKET fallback at start-time.
        match config.mode {
            probe_agent::config::CaptureMode::Xdp
            | probe_agent::config::CaptureMode::XdpSkb
            | probe_agent::config::CaptureMode::XdpOffload => {
                warn!("XDP start failed - attempting AF_PACKET fallback: {}", e);
                // drop the failed capturer and try AfPacket
                drop(capturer);

                match AfPacketCapture::new(&config) {
                    Ok(mut afp) => {
                        info!("AF_PACKET capturer created as fallback; starting...");
                        if let Err(e2) = afp.start().await {
                            error!(
                                "AF_PACKET fallback failed to start: {} (original: {})",
                                e2, e
                            );
                            handle
                                .complete_with_error(format!(
                                    "XDP start error: {}; AF_PACKET start error: {}",
                                    e, e2
                                ))
                                .await;
                            return;
                        }
                        info!("AF_PACKET fallback started successfully");
                        capturer = Box::new(afp);
                    }
                    Err(e2) => {
                        error!(
                            "AF_PACKET fallback creation failed: {} (original XDP error: {})",
                            e2, e
                        );
                        handle
                            .complete_with_error(format!(
                                "XDP start error: {}; AF_PACKET creation error: {}",
                                e, e2
                            ))
                            .await;
                        return;
                    }
                }
            }
            _ => {
                handle.complete_with_error(e.to_string()).await;
                return;
            }
        }
    }

    let mut processor = match triple_buffer {
        Some(ref buffer) => PacketProcessor::with_pcap(flow_table.clone(), buffer.clone()),
        None => PacketProcessor::new(flow_table.clone()),
    };

    let mut last_stats = std::time::Instant::now();
    let mut pkt_count: u64 = 0;
    let mut byte_count: u64 = 0;

    info!("Capture started successfully");

    loop {
        tokio::select! {
            _ = async {
                match capturer.poll() {
                    Ok(Some(batch)) => {
                        let count = batch.len();
                        let bytes = batch.total_bytes();

                        pkt_count += count as u64;
                        byte_count += bytes as u64;

                        processor.process_batch(&batch);
                        metrics::inc_capture_local(bytes as u64);
                    }
                    Ok(None) => {
                        tokio::time::sleep(Duration::from_micros(10)).await;
                    }
                    Err(e) => {
                        error!("Capture error: {}", e);
                        metrics::CAPTURE_ERRORS.inc();
                        tokio::time::sleep(Duration::from_millis(100)).await;
                    }
                }

                if last_stats.elapsed() >= Duration::from_secs(10) {
                    let elapsed = last_stats.elapsed().as_secs_f64();
                    let pps = pkt_count as f64 / elapsed;
                    let mbps = (byte_count as f64 * 8.0) / (elapsed * 1_000_000.0);

                    info!(
                        "Capture stats: {:.0} pps, {:.1} Mbps, {} flows, {} new, {} updated",
                        pps, mbps,
                        flow_table.len(),
                        processor.stats().new_flows,
                        processor.stats().updated_flows
                    );

                    metrics::CAPTURE_PPS.set(pps);
                    metrics::ACTIVE_FLOWS.set(flow_table.len() as f64);
                    metrics::flush_local_metrics();

                    pkt_count = 0;
                    byte_count = 0;
                    last_stats = std::time::Instant::now();
                }
            } => {}
            _ = handle.wait() => {
                info!("Capture received shutdown signal");
                break;
            }
        }
    }

    if let Err(e) = capturer.stop().await {
        warn!("Error stopping capture: {}", e);
    }

    metrics::flush_local_metrics();

    let final_stats = processor.stats();
    info!(
        "Capture stopped: processed={}, parsed={}, failed={}, new_flows={}, updated_flows={}",
        final_stats.packets_processed,
        final_stats.packets_parsed,
        final_stats.packets_failed,
        final_stats.new_flows,
        final_stats.updated_flows
    );

    handle.complete().await;
}

async fn wait_for_shutdown(shutdown_manager: &Arc<ShutdownManager>, _config: &ProbeConfig) {
    info!("Probe Agent running, press Ctrl+C to stop");

    tokio::signal::ctrl_c().await.ok();

    info!("========================================");
    info!("  Received shutdown signal (Ctrl+C)");
    info!("========================================");

    let grace_period = Duration::from_secs(30);
    shutdown_manager.clone().shutdown(grace_period).await;
}
