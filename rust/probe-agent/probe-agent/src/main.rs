use anyhow::{bail, Context, Result};
use probe_agent::config::CaptureConfig;
use probe_agent::metrics;
use probe_agent::shutdown::ShutdownHandle;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{mpsc, oneshot};
use tracing::{debug, error, info, trace, warn};
use tracing_subscriber::{self, layer::SubscriberExt, util::SubscriberInitExt, EnvFilter};

use probe_agent::aggregator::{
    Eviction, EvictionClock, EvictionConfig, GenerationalConfig, GenerationalFlowTable,
    PacketProcessor, PartitionedFlowTable,
};
use probe_agent::archiver::{
    inspect_packet_records, DurablePcapSpool, JournaledUploadRef, PcapGlobalHeader, TripleBuffer,
    TripleBufferConfig, UploadData, UploadTask, Uploader, UploaderConfig,
};
use probe_agent::capture::pcap_replay_op::{
    execute_replay_window, execute_wire_replay_window, ReplayObjectStoreConfig, S3ObjectFetcher,
};
use probe_agent::capture::wire_inject::WireInjector;
use probe_agent::capture::{create_capturer, AfPacketCapture, Capturer, CaptureTimestamp, CaptureTimestampProvenance, PacketBatch};
use probe_agent::config::gateway_sni;
use probe_agent::config::ProbeConfig;
use probe_agent::control::{BuiltinProbeExecutor, ProbeControlProcessor, ReplayExecution, ReplayOperationExecutor, ReplayWindowCommand};
use probe_agent::interface_monitor::{InterfaceMonitor, InterfaceMonitorConfig};
use probe_agent::parser::PassiveAssetDiscovery;
use probe_agent::sender::{
    AssetBindingSpool, BatchConfig, BatchSender, DurableAssetBindingSink, GrpcSender,
    GrpcSenderConfig,
};
use probe_agent::shutdown::ShutdownManager;

use proto_gen::FlowEvent;

const FLOW_CHANNEL_SIZE: usize = 100_000;
const BATCH_CHANNEL_SIZE: usize = 1_000;
const UPLOAD_CHANNEL_SIZE: usize = 100;

#[derive(Debug, Clone, Copy)]
struct PcapRecoveryPolicy {
    startup_timeout: Duration,
    supervisor_interval: Duration,
}

impl Default for PcapRecoveryPolicy {
    fn default() -> Self {
        Self {
            startup_timeout: Duration::from_secs(120),
            supervisor_interval: Duration::from_secs(300),
        }
    }
}

#[derive(Debug, Clone)]
struct CaptureAdmissionPermit {
    recovery_decided_at: tokio::time::Instant,
}

struct PcapRecoveryRuntime {
    admission: CaptureAdmissionPermit,
    supervisor: tokio::task::JoinHandle<()>,
    stop_tx: Option<oneshot::Sender<()>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum PcapShutdownStage {
    CaptureStopped,
    FinalSealed,
    JournalFlushed,
    BufferReleased,
    ProducerClosed,
    UploaderDrained,
    RecoveryStopped,
}

#[derive(Debug)]
struct PcapStageEvent {
    stage: PcapShutdownStage,
    success: bool,
    detail: String,
}

#[derive(Debug, Default)]
struct PcapShutdownReport {
    graceful: bool,
    completed_stages: Vec<PcapShutdownStage>,
    failure: Option<String>,
}

struct PcapPipelineRuntime {
    capture_stop_tx: Option<oneshot::Sender<()>>,
    final_seal_tx: Option<oneshot::Sender<()>>,
    uploader_drain_tx: Option<oneshot::Sender<()>>,
    capture_task: tokio::task::JoinHandle<()>,
    rotator_task: tokio::task::JoinHandle<()>,
    uploader_task: tokio::task::JoinHandle<()>,
    recovery: PcapRecoveryRuntime,
    stage_rx: mpsc::UnboundedReceiver<PcapStageEvent>,
}

enum PcapUploadMessage {
    Legacy(UploadTask),
    Journaled(JournaledUploadRef),
}

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

    // Probe registration is advisory: a transient gateway outage must not
    // crash the capture pipeline. Retry with backoff in the background and
    // continue capturing if registration is unavailable.
    match register_probe(&config).await {
        Ok(()) => {}
        Err(e) => {
            warn!(
                "Probe registration failed (capture will continue; registration will retry in background): {}",
                e
            );
            let retry_config = config.clone();
            tokio::spawn(async move {
                let mut attempt: u32 = 1;
                loop {
                    tokio::time::sleep(Duration::from_secs(10)).await;
                    match register_probe(&retry_config).await {
                        Ok(()) => {
                            info!("Probe registered successfully after background retry (attempt {})", attempt);
                            break;
                        }
                        Err(retry_error) => {
                            attempt += 1;
                            warn!(
                                "Probe registration retry {} failed: {}",
                                attempt, retry_error
                            );
                        }
                    }
                }
            });
        }
    }

    let components = create_components(&config, &shutdown_manager).await?;
    let mut pcap_runtime = start_components(components, &config, &shutdown_manager).await?;

    wait_for_shutdown(&shutdown_manager, &config, &mut pcap_runtime).await;

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
            .domain_name(&gateway_sni(&config.sender.gateway_addr));

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

    request.metadata_mut().insert(
        "x-tenant-id",
        tonic::metadata::MetadataValue::try_from(config.tenant_id.as_str())?,
    );
    request.metadata_mut().insert(
        "x-probe-id",
        tonic::metadata::MetadataValue::try_from(config.probe_id.as_str())?,
    );

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
    upload_tx: mpsc::Sender<PcapUploadMessage>,
    upload_rx: mpsc::Receiver<PcapUploadMessage>,
    grpc_sender: Arc<GrpcSender>,
    control_processor: Arc<ProbeControlProcessor>,
    interface_monitor: Arc<InterfaceMonitor>,
    uploader: Option<Arc<Uploader>>,
    pcap_spool: Option<Arc<DurablePcapSpool>>,
    asset_binding_spool: Option<Arc<AssetBindingSpool>>,
    asset_discovery: Option<Arc<PassiveAssetDiscovery>>,
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
    let (upload_tx, upload_rx) = mpsc::channel::<PcapUploadMessage>(UPLOAD_CHANNEL_SIZE);

    let (asset_binding_spool, asset_discovery) = if config.asset_binding_upload_admitted() {
        let spool = Arc::new(
            AssetBindingSpool::open(
                std::path::Path::new(&config.sender.cache_path),
                config.sender.cache_max_size,
            )
            .context("Failed to open durable asset binding spool")?,
        );
        let sink = Arc::new(
            DurableAssetBindingSink::new(
                spool.clone(),
                config.tenant_id.clone(),
                config.probe_id.clone(),
            )
            .context("Failed to create durable asset binding sink")?,
        );
        let discovery = Arc::new(PassiveAssetDiscovery::new().with_binding_sink(sink));
        info!(
            "Asset binding capture admitted with durable spool at {:?}",
            spool.path()
        );
        (Some(spool), Some(discovery))
    } else {
        info!("Asset binding capture and upload remain disabled");
        (None, None)
    };

    let (triple_buffer, uploader, pcap_spool) = if config.archiver.enabled {
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
        uploader_config.tenant_id = config.tenant_id.clone();
        uploader_config.probe_id = config.probe_id.clone();

        let mut uploader = Uploader::new(uploader_config).context("Failed to create uploader")?;

        if let Err(e) = uploader.connect_gateway().await {
            warn!("Failed to connect to gateway for metadata upload: {}", e);
        }

        let uploader = Arc::new(uploader);
        let spool = if config.archiver.durable_spool_enabled {
            Some(uploader.durable_spool())
        } else {
            warn!("PCAP durable spool producer is disabled; using the legacy compatibility path");
            None
        };

        info!(
            "PCAP archiver enabled: buffer_size={}MB, rotation={}s",
            config.archiver.buffer_size_mb, config.archiver.rotation_interval_sec
        );

        (Some(buffer), Some(uploader), spool)
    } else {
        info!("PCAP archiver disabled");
        (None, None, None)
    };

    let mut grpc_config = GrpcSenderConfig::from(&config.sender);
    grpc_config.tenant_id = Some(config.tenant_id.clone());
    grpc_config.probe_id = Some(config.probe_id.clone());

    let grpc_sender = Arc::new(
        GrpcSender::new(grpc_config)
            .await
            .context("Failed to create gRPC sender")?,
    );
    // 回放执行器(发生地在探针):复用共享 flow_table + 聚合器单一真源;
    // 对象存储接入与 archiver 同源(PROBE_S3_* / S3_ENDPOINT)。
    // wire 回放(测试阶段数据集生成)接口 allowlist 来自配置,空 = 拒绝。
    let replay_executor = Arc::new(ProbeReplayExecutor {
        flow_table: flow_table.clone(),
        parser_route: config.aggregator.parser_route,
        object_store: ReplayObjectStoreConfig {
            endpoint: config.archiver.s3_endpoint.clone(),
            region: config.archiver.s3_region.clone(),
            access_key: config.archiver.s3_access_key.clone(),
            secret_key: config.archiver.s3_secret_key.clone(),
            cache_dir: std::path::PathBuf::from(&config.archiver.cache_path),
        },
        cache_dir: std::path::PathBuf::from(&config.archiver.cache_path),
        wire_interfaces: config.capture.wire_replay_interfaces.clone(),
    });
    let control_processor = Arc::new(
        ProbeControlProcessor::open(
            std::path::Path::new(&config.sender.cache_path),
            config.tenant_id.clone(),
            config.probe_id.clone(),
            Arc::new(
                BuiltinProbeExecutor::for_gateway(&config.sender.gateway_addr)
                    .context("Failed to configure builtin probe operation executor")?
                    .with_replay(replay_executor)
                    .with_capture_interface(config.capture.interface.clone()),
            ),
        )
        .context("Failed to create probe control processor")?,
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
        control_processor,
        interface_monitor,
        uploader,
        pcap_spool,
        asset_binding_spool,
        asset_discovery,
    })
}

async fn start_components(
    mut components: Components,
    config: &ProbeConfig,
    shutdown_manager: &Arc<ShutdownManager>,
) -> Result<Option<PcapPipelineRuntime>> {
    let durable_spool_enabled = config.archiver.enabled && config.archiver.durable_spool_enabled;
    let mut recovery_runtime = if durable_spool_enabled {
        let uploader = components
            .uploader
            .as_ref()
            .context("durable PCAP spool enabled without uploader")?;
        let handle = shutdown_manager.register("pcap_recovery", 57).await;
        Some(
            run_pcap_startup_recovery_before_capture(
                uploader.clone(),
                handle,
                PcapRecoveryPolicy::default(),
            )
            .await
            .context("PCAP startup recovery blocked capture admission")?,
        )
    } else {
        None
    };
    let capture_admission = recovery_runtime
        .as_ref()
        .map(|runtime| runtime.admission.clone());
    let (stage_tx, stage_rx) = mpsc::unbounded_channel();
    let mut capture_control = None;
    let mut final_seal_control = None;
    let mut uploader_drain_control = None;
    let mut capture_task = None;
    let mut rotator_task = None;
    let mut uploader_task = None;
    let eviction_clock = if config.capture.mode.is_pcap_offline() {
        Arc::new(EvictionClock::offline())
    } else {
        Arc::new(EvictionClock::live())
    };

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
        let cleanup_authority = components
            .uploader
            .as_ref()
            .map(|uploader| uploader.journal());
        let spool_root = std::path::PathBuf::from(&cache_path).join("pcap_spool");
        let durable_cleanup_enabled = durable_spool_enabled;

        tokio::spawn(async move {
            let monitor_config = DiskMonitorConfig {
                path: cache_path,
                check_interval: Duration::from_secs(60),
                warning_threshold_percent: 80.0,
                critical_threshold_percent: 90.0,
                cleanup_target_percent: 70.0,
                min_cleanup_interval: Duration::from_secs(300),
                min_free_bytes: 10 * 1024 * 1024 * 1024,
                auto_cleanup: durable_cleanup_enabled,
            };

            let monitor_result = match cleanup_authority {
                Some(authority) if durable_cleanup_enabled => {
                    DiskMonitor::with_pcap_cleanup_authority(monitor_config, authority, spool_root)
                }
                _ => {
                    if durable_cleanup_enabled {
                        warn!(
                            "Durable PCAP cleanup was requested but no upload journal is available; disk monitor runs without cleanup authority"
                        );
                    }
                    Ok(DiskMonitor::new(monitor_config))
                }
            };
            let monitor = match monitor_result {
                Ok(monitor) => Arc::new(monitor),
                Err(error) => {
                    error!(
                        "Failed to initialize journal-authorized disk cleanup: {}",
                        error
                    );
                    handle.complete().await;
                    return;
                }
            };

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

    if let Some(uploader) = components.uploader.take() {
        trace!("Running S3 preflight check...");

        if let Err(e) = uploader.preflight_check().await {
            trace!("🔴 S3 preflight check failed: {}", e);
        } else {
            debug!("✓ S3 preflight check succeeded");
        }
        let handle = shutdown_manager.register("pcap_uploader", 60).await;
        let upload_rx = components.upload_rx;

        let (drain_tx, drain_rx) = oneshot::channel();
        let (drain_control, legacy_drain_guard, drain_receiver, uploader_stage_tx) =
            if durable_spool_enabled {
                (Some(drain_tx), None, Some(drain_rx), Some(stage_tx.clone()))
            } else {
                (None, Some(drain_tx), None, None)
            };
        uploader_drain_control = drain_control;
        let task = tokio::spawn(async move {
            let _legacy_drain_guard = legacy_drain_guard;
            run_pcap_uploader(
                handle,
                uploader,
                upload_rx,
                drain_receiver,
                uploader_stage_tx,
            )
            .await;
        });
        if durable_spool_enabled {
            uploader_task = Some(task);
        }
    }

    if let Some(buffer) = components.triple_buffer.clone() {
        let handle = shutdown_manager.register("pcap_rotator", 55).await;
        let upload_tx = components.upload_tx.clone();
        let tenant_id = config.tenant_id.clone();
        let probe_id = config.probe_id.clone();
        let spool = components.pcap_spool.clone();
        let fallback_buffer = buffer.clone();

        let (seal_tx, seal_rx) = oneshot::channel();
        let (seal_control, legacy_seal_guard, seal_receiver, rotator_stage_tx) =
            if durable_spool_enabled {
                (Some(seal_tx), None, Some(seal_rx), Some(stage_tx.clone()))
            } else {
                (None, Some(seal_tx), None, None)
            };
        final_seal_control = seal_control;
        let task = tokio::spawn(async move {
            let _legacy_seal_guard = legacy_seal_guard;
            run_pcap_rotator(
                handle,
                buffer,
                spool,
                upload_tx,
                tenant_id,
                probe_id,
                seal_receiver,
                rotator_stage_tx,
            )
            .await;
        });
        if durable_spool_enabled {
            rotator_task = Some(task);
        }

        // Fallback overflow PCAP files are written to disk when the sealed
        // buffer cannot be rotated; without a consumer they were never
        // archived and accumulated forever. Route them through the same
        // upload pipeline (durable spool journal when enabled, legacy S3
        // otherwise) and delete them only after durable publication.
        let handle = shutdown_manager.register("fallback_consumer", 50).await;
        let upload_tx = components.upload_tx.clone();
        let tenant_id = config.tenant_id.clone();
        let probe_id = config.probe_id.clone();
        let spool = components.pcap_spool.clone();
        tokio::spawn(async move {
            run_fallback_consumer(
                handle,
                fallback_buffer,
                upload_tx,
                spool,
                tenant_id,
                probe_id,
            )
            .await;
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

    if let Some(spool) = components.asset_binding_spool.clone() {
        let handle = shutdown_manager.register("asset_binding_sender", 50).await;
        let sender = Arc::clone(&components.grpc_sender);
        let batch_size = config.sender.batch_size;
        tokio::spawn(async move {
            run_asset_binding_sender(handle, sender, spool, batch_size).await;
        });
    }

    {
        let handle = shutdown_manager.register("heartbeat", 55).await;
        let sender = Arc::clone(&components.grpc_sender);
        let monitor = Arc::clone(&components.interface_monitor);
        let control_processor = Arc::clone(&components.control_processor);

        tokio::spawn(async move {
            run_heartbeat_task(handle, sender, monitor, control_processor).await;
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
        let eviction_clock_for_task = eviction_clock.clone();

        tokio::spawn(async move {
            run_eviction(
                handle,
                eviction_config,
                flow_table,
                flow_tx,
                eviction_clock_for_task,
            )
            .await;
        });
    }

    {
        let handle = shutdown_manager.register("capture", 10).await;
        let mut capture_config = config.capture.clone();
        capture_config.producer_enabled = config.capture_producer_admitted();
        let flow_table = components.flow_table.clone();
        let triple_buffer = components.triple_buffer.clone();
        let pcap_enabled = config.archiver.enabled;
        let parser_route = config.aggregator.parser_route;
        let durable_spool_required = durable_spool_enabled;
        let capture_admission = capture_admission.clone();
        let capture_eviction_clock = eviction_clock.clone();
        let asset_discovery = components.asset_discovery.clone();

        let (capture_stop_tx, capture_stop_rx) = oneshot::channel();
        let (stop_control, legacy_stop_guard, stop_receiver, capture_stage_tx) =
            if durable_spool_enabled {
                (
                    Some(capture_stop_tx),
                    None,
                    Some(capture_stop_rx),
                    Some(stage_tx.clone()),
                )
            } else {
                (None, Some(capture_stop_tx), None, None)
            };
        capture_control = stop_control;
        let task = tokio::spawn(async move {
            let _legacy_stop_guard = legacy_stop_guard;
            run_capture(
                handle,
                capture_config,
                flow_table,
                triple_buffer,
                pcap_enabled,
                durable_spool_required,
                capture_admission,
                stop_receiver,
                capture_stage_tx,
                parser_route,
                capture_eviction_clock,
                asset_discovery,
            )
            .await;
        });
        if durable_spool_enabled {
            capture_task = Some(task);
        }
    }

    info!(
        "All {} components started",
        shutdown_manager.component_count().await
    );
    // Keep the unique recovery supervisor owned for the process lifetime. Its
    // ShutdownHandle drives the task to an explicit completion receipt.
    let pcap_runtime = match (
        capture_control,
        final_seal_control,
        uploader_drain_control,
        capture_task,
        rotator_task,
        uploader_task,
        recovery_runtime.take(),
    ) {
        (
            Some(capture_stop_tx),
            Some(final_seal_tx),
            Some(uploader_drain_tx),
            Some(capture_task),
            Some(rotator_task),
            Some(uploader_task),
            Some(recovery),
        ) => Some(PcapPipelineRuntime {
            capture_stop_tx: Some(capture_stop_tx),
            final_seal_tx: Some(final_seal_tx),
            uploader_drain_tx: Some(uploader_drain_tx),
            capture_task,
            rotator_task,
            uploader_task,
            recovery,
            stage_rx,
        }),
        (None, None, None, None, None, None, None) => None,
        _ => bail!("PCAP runtime was only partially constructed"),
    };
    Ok(pcap_runtime)
}

async fn run_pcap_startup_recovery_before_capture(
    uploader: Arc<Uploader>,
    mut handle: ShutdownHandle,
    policy: PcapRecoveryPolicy,
) -> Result<PcapRecoveryRuntime> {
    let cleanup_report = uploader
        .journal()
        .reconcile_cleanup_interruption()
        .context("cleanup interruption reconciliation failed closed")?;
    info!(
        "PCAP cleanup interruption reconciliation: {:?}",
        cleanup_report
    );

    let orphan_report = uploader
        .durable_spool()
        .reconcile_orphans()
        .await
        .context("durable PCAP spool orphan reconciliation failed closed")?;
    info!(
        "PCAP durable spool orphan reconciliation: {:?}",
        orphan_report
    );

    let startup_summary =
        tokio::time::timeout(policy.startup_timeout, uploader.recover_pending_uploads())
            .await
            .context("PCAP startup recovery exceeded its bounded deadline")??;
    if !startup_summary.admission_safe() {
        bail!(
            "PCAP startup recovery is not admission-safe: {:?}",
            startup_summary
        );
    }

    let admission = CaptureAdmissionPermit {
        recovery_decided_at: tokio::time::Instant::now(),
    };
    let (stop_tx, mut stop_rx) = oneshot::channel();
    let supervisor = tokio::spawn(async move {
        let mut ticker = tokio::time::interval(policy.supervisor_interval);
        // Startup recovery above owns the first tick; do not create a second
        // concurrent recovery owner immediately.
        ticker.tick().await;
        loop {
            tokio::select! {
                _ = ticker.tick() => {
                    if let Err(error) = uploader.recover_pending_uploads().await {
                        error!("PCAP background recovery failed: {}", error);
                    }
                }
                _ = handle.wait() => {
                    info!("PCAP recovery supervisor shutting down");
                    break;
                }
                _ = &mut stop_rx => {
                    info!("PCAP recovery supervisor received staged stop");
                    break;
                }
            }
        }
        handle.complete().await;
    });

    Ok(PcapRecoveryRuntime {
        admission,
        supervisor,
        stop_tx: Some(stop_tx),
    })
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
    control_processor: Arc<ProbeControlProcessor>,
) {
    let mut ticker = tokio::time::interval(Duration::from_secs(60));

    info!("Heartbeat task started (interval: 60s)");

    loop {
        tokio::select! {
            _ = ticker.tick() => {
                let pending_acks = match control_processor.pending_acks(100) {
                    Ok(acks) => acks,
                    Err(error) => {
                        error!("Failed to load persisted probe operation ACKs: {}", error);
                        continue;
                    }
                };
                match sender.send_heartbeat(Some(&monitor), pending_acks).await {
                    Ok(response) => {
                        if let Err(error) = control_processor
                            .acknowledge_accepted(&response.accepted_ack_operation_ids)
                        {
                            error!("Failed to remove accepted probe operation ACKs: {}", error);
                        }
                        for command in response.operation_commands {
                            let operation_id = command.operation_id.clone();
                            match control_processor.process(command).await {
                                Ok(ack) => info!(
                                    operation_id = %operation_id,
                                    command_revision = ack.command_revision,
                                    applied = ack.applied,
                                    "Probe operation executed and ACK persisted"
                                ),
                                Err(error) => warn!(
                                    operation_id = %operation_id,
                                    "Rejected probe operation before execution: {}",
                                    error
                                ),
                            }
                        }
                        debug!("✓ Heartbeat sent successfully");
                    }
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
    mut rx: mpsc::Receiver<PcapUploadMessage>,
    mut drain_rx: Option<oneshot::Receiver<()>>,
    stage_tx: Option<mpsc::UnboundedSender<PcapStageEvent>>,
) {
    info!("Starting PCAP uploader");

    let mut uploaded_count: u64 = 0;
    let mut error_count: u64 = 0;

    loop {
        tokio::select! {
            Some(task) = rx.recv() => {
                let result = match task {
                    PcapUploadMessage::Legacy(task) => uploader.upload(task).await,
                    PcapUploadMessage::Journaled(upload) => uploader.upload_journaled(upload).await,
                };
                match result {
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
            _ = async {
                match drain_rx.as_mut() {
                    Some(receiver) => { let _ = receiver.await; }
                    None => std::future::pending::<()>().await,
                }
            } => {
                info!("PCAP uploader received staged drain request");
                rx.close();
                while let Some(task) = rx.recv().await {
                    let result = match task {
                        PcapUploadMessage::Legacy(task) => uploader.upload(task).await,
                        PcapUploadMessage::Journaled(upload) => uploader.upload_journaled(upload).await,
                    };
                    if let Err(e) = result {
                        error_count += 1;
                        error!("Final staged upload failed: {}", e);
                    } else {
                        uploaded_count += 1;
                    }
                }
                let success = error_count == 0;
                if let Some(stage_tx) = stage_tx.as_ref() {
                let _ = stage_tx.send(PcapStageEvent {
                    stage: PcapShutdownStage::UploaderDrained,
                    success,
                    detail: if success {
                        "uploader channel drained".to_string()
                    } else {
                        format!("uploader drain observed {error_count} errors")
                    },
                });
                }
                break;
            }
            _ = handle.wait() => {
                info!("PCAP uploader shutting down");

                while let Ok(task) = rx.try_recv() {
                    let result = match task {
                        PcapUploadMessage::Legacy(task) => uploader.upload(task).await,
                        PcapUploadMessage::Journaled(upload) => uploader.upload_journaled(upload).await,
                    };
                    if let Err(e) = result {
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
    spool: Option<Arc<DurablePcapSpool>>,
    upload_tx: mpsc::Sender<PcapUploadMessage>,
    tenant_id: String,
    probe_id: String,
    mut final_seal_rx: Option<oneshot::Receiver<()>>,
    stage_tx: Option<mpsc::UnboundedSender<PcapStageEvent>>,
) {
    info!("Starting PCAP rotator");

    let mut interval = tokio::time::interval(Duration::from_secs(5));
    let mut rotations: u64 = 0;

    loop {
        tokio::select! {
            _ = interval.tick() => {
                buffer.try_rotate();

                if let Some(result) = check_and_get_upload(&buffer).await {
                    let buffer_idx = buffer.find_uploading_buffer();
                    let publish = publish_rotated_upload(
                        result,
                        spool.as_ref(),
                        &tenant_id,
                        &probe_id,
                    )
                    .await;
                    match publish {
                        Ok(upload) => {
                            rotations += 1;
                            let durable = matches!(&upload, PcapUploadMessage::Journaled(_));
                            if durable {
                                if let Some(idx) = buffer_idx {
                                // The sealed buffer may be reused only after the
                                // file and journal durability barriers completed.
                                buffer.complete_upload(idx);
                                }
                            }
                            if upload_tx.send(upload).await.is_err() {
                                error!(
                                    "PCAP uploader channel closed after durable journal publication; startup recovery is required"
                                );
                                break;
                            }
                            if !durable {
                                if let Some(idx) = buffer_idx {
                                    buffer.complete_upload(idx);
                                }
                            }
                        }
                        Err(error) => {
                            error!(
                                "PCAP durable spool failed; sealed buffer lease retained and capture backpressured: {}",
                                error
                            );
                        }
                    }
                }
            }
            _ = async {
                match final_seal_rx.as_mut() {
                    Some(receiver) => { let _ = receiver.await; }
                    None => std::future::pending::<()>().await,
                }
            } => {
                info!("PCAP rotator received staged final-seal request");
                let stage_sender = match stage_tx.as_ref() {
                    Some(sender) => sender,
                    None => {
                        // The staged seal path is only reachable in durable
                        // mode, which always provides a stage sender; degrade
                        // gracefully instead of panicking.
                        error!("Staged final seal requested without a stage sender; aborting staged shutdown");
                        break;
                    }
                };
                let success = final_spool_once(
                    &buffer,
                    spool.as_ref(),
                    &upload_tx,
                    &tenant_id,
                    &probe_id,
                    stage_sender,
                )
                .await;
                drop(upload_tx);
                let _ = stage_sender.send(PcapStageEvent {
                    stage: PcapShutdownStage::ProducerClosed,
                    success,
                    detail: "PCAP producer channel closed after final seal".to_string(),
                });
                break;
            }
            _ = handle.wait() => {
                info!("PCAP rotator shutting down");

                buffer.force_rotate();

                if let Some(result) = check_and_get_upload(&buffer).await {
                    let buffer_idx = buffer.find_uploading_buffer();
                    match publish_rotated_upload(
                        result,
                        spool.as_ref(),
                        &tenant_id,
                        &probe_id,
                    ).await {
                        Ok(upload) => {
                            let durable = matches!(&upload, PcapUploadMessage::Journaled(_));
                            if durable {
                                if let Some(idx) = buffer_idx {
                                buffer.complete_upload(idx);
                                }
                            }
                            if upload_tx.send(upload).await.is_err() {
                                error!(
                                    "Final PCAP was durable but uploader channel was closed; journal recovery will resume it"
                                );
                            }
                            if !durable {
                                if let Some(idx) = buffer_idx {
                                    buffer.complete_upload(idx);
                                }
                            }
                        }
                        Err(error) => {
                            error!(
                                "Final PCAP spool failed; shutdown cannot be considered graceful: {}",
                                error
                            );
                        }
                    }
                }

                break;
            }
        }
    }

    info!("PCAP rotator stopped: rotations={}", rotations);
    handle.complete().await;
}

async fn run_fallback_consumer(
    mut handle: ShutdownHandle,
    buffer: Arc<TripleBuffer>,
    upload_tx: mpsc::Sender<PcapUploadMessage>,
    spool: Option<Arc<DurablePcapSpool>>,
    tenant_id: String,
    probe_id: String,
) {
    info!("Starting fallback PCAP consumer");
    let mut ticker = tokio::time::interval(Duration::from_secs(30));
    ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
    let mut consumed: u64 = 0;
    let mut failed: u64 = 0;
    loop {
        tokio::select! {
            _ = handle.wait() => {
                info!(
                    "Fallback consumer stopping: consumed={}, failed={}",
                    consumed, failed
                );
                break;
            }
            _ = ticker.tick() => {
                let files = match buffer.get_fallback_files() {
                    Ok(files) => files,
                    Err(error) => {
                        error!("Failed to list fallback PCAP files: {}", error);
                        continue;
                    }
                };
                if files.is_empty() {
                    continue;
                }
                for path in files {
                    let data = match std::fs::read(&path) {
                        Ok(data) => data,
                        Err(error) => {
                            error!(
                                "Failed to read fallback PCAP {}: {}",
                                path.display(),
                                error
                            );
                            failed += 1;
                            continue;
                        }
                    };
                    if data.len() < PcapGlobalHeader::size() {
                        warn!(
                            "Fallback PCAP {} is smaller than a global header; removing",
                            path.display()
                        );
                        let _ = buffer.cleanup_fallback_file(&path);
                        continue;
                    }
                    let records = &data[PcapGlobalHeader::size()..];
                    let (packet_count, ts_start, ts_end) =
                        match inspect_packet_records(records) {
                            Ok(values) => values,
                            Err(error) => {
                                error!(
                                    "Invalid fallback PCAP {}: {}; retaining for next cycle",
                                    path.display(),
                                    error
                                );
                                failed += 1;
                                continue;
                            }
                        };
                    let upload = UploadData {
                        data: records.to_vec(),
                        ts_start,
                        ts_end,
                        packet_count,
                    };
                    let published = match spool.as_ref() {
                        Some(spool) => {
                            match spool
                                .persist_rotated(upload, &tenant_id, &probe_id)
                                .await
                            {
                                Ok(reference) => {
                                    upload_tx
                                        .send(PcapUploadMessage::Journaled(reference))
                                        .await
                                        .is_ok()
                                }
                                Err(error) => {
                                    error!(
                                        "Fallback spool publication failed for {}: {}",
                                        path.display(),
                                        error
                                    );
                                    false
                                }
                            }
                        }
                        None => {
                            upload_tx
                                .send(PcapUploadMessage::Legacy(UploadTask {
                                    data: upload.data,
                                    ts_start: upload.ts_start,
                                    ts_end: upload.ts_end,
                                    packet_count: upload.packet_count,
                                    tenant_id: tenant_id.clone(),
                                    probe_id: probe_id.clone(),
                                }))
                                .await
                                .is_ok()
                        }
                    };
                    if published {
                        match buffer.cleanup_fallback_file(&path) {
                            Ok(()) => {
                                consumed += 1;
                                info!(
                                    "Fallback PCAP consumed into upload pipeline: {}",
                                    path.display()
                                );
                            }
                            Err(error) => {
                                warn!(
                                    "Fallback PCAP published but cleanup failed ({}); will retry next cycle",
                                    error
                                );
                            }
                        }
                    } else {
                        error!(
                            "Upload channel closed; fallback PCAP retained: {}",
                            path.display()
                        );
                        failed += 1;
                    }
                }
            }
        }
    }
    handle.complete().await;
}

async fn final_spool_once(
    buffer: &Arc<TripleBuffer>,
    spool: Option<&Arc<DurablePcapSpool>>,
    upload_tx: &mpsc::Sender<PcapUploadMessage>,
    tenant_id: &str,
    probe_id: &str,
    stage_tx: &mpsc::UnboundedSender<PcapStageEvent>,
) -> bool {
    buffer.force_rotate();
    let Some(result) = check_and_get_upload(buffer).await else {
        let _ = stage_tx.send(PcapStageEvent {
            stage: PcapShutdownStage::FinalSealed,
            success: true,
            detail: "no non-empty final buffer".to_string(),
        });
        let _ = stage_tx.send(PcapStageEvent {
            stage: PcapShutdownStage::JournalFlushed,
            success: true,
            detail: "no final journal record required".to_string(),
        });
        let _ = stage_tx.send(PcapStageEvent {
            stage: PcapShutdownStage::BufferReleased,
            success: true,
            detail: "no final buffer lease".to_string(),
        });
        return true;
    };
    let _ = stage_tx.send(PcapStageEvent {
        stage: PcapShutdownStage::FinalSealed,
        success: true,
        detail: "final buffer lease sealed".to_string(),
    });
    let buffer_idx = buffer.find_uploading_buffer();
    match publish_rotated_upload(result, spool, tenant_id, probe_id).await {
        Ok(upload) => {
            let durable = matches!(&upload, PcapUploadMessage::Journaled(_));
            let _ = stage_tx.send(PcapStageEvent {
                stage: PcapShutdownStage::JournalFlushed,
                success: durable,
                detail: if durable {
                    "final journal entry flushed".to_string()
                } else {
                    "legacy producer has no pre-release durable journal barrier".to_string()
                },
            });
            if durable {
                if let Some(idx) = buffer_idx {
                    buffer.complete_upload(idx);
                }
            }
            let _ = stage_tx.send(PcapStageEvent {
                stage: PcapShutdownStage::BufferReleased,
                success: durable,
                detail: if durable {
                    "sealed buffer released after journal flush".to_string()
                } else {
                    "legacy sealed buffer cannot prove journal-before-release".to_string()
                },
            });
            if upload_tx.send(upload).await.is_err() {
                let _ = stage_tx.send(PcapStageEvent {
                    stage: PcapShutdownStage::ProducerClosed,
                    success: false,
                    detail: "uploader receiver closed before final enqueue".to_string(),
                });
                false
            } else {
                if !durable {
                    if let Some(idx) = buffer_idx {
                        buffer.complete_upload(idx);
                    }
                }
                durable
            }
        }
        Err(error) => {
            let _ = stage_tx.send(PcapStageEvent {
                stage: PcapShutdownStage::JournalFlushed,
                success: false,
                detail: error.to_string(),
            });
            false
        }
    }
}

async fn publish_rotated_upload(
    result: UploadData,
    spool: Option<&Arc<DurablePcapSpool>>,
    tenant_id: &str,
    probe_id: &str,
) -> Result<PcapUploadMessage> {
    match spool {
        Some(spool) => spool
            .persist_rotated(result, tenant_id, probe_id)
            .await
            .map(PcapUploadMessage::Journaled),
        None => Ok(PcapUploadMessage::Legacy(UploadTask {
            data: result.data,
            ts_start: result.ts_start,
            ts_end: result.ts_end,
            packet_count: result.packet_count,
            tenant_id: tenant_id.to_string(),
            probe_id: probe_id.to_string(),
        })),
    }
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
    // Run the sender in a spawned task so that on shutdown we can wait
    // (bounded) for it to drain its buffered events instead of dropping the
    // future, which would lose the events still in `rx` and the collector.
    let run_task = tokio::spawn(sender.run());
    tokio::pin!(run_task);
    tokio::select! {
        _ = &mut run_task => {
            debug!("Batch collector finished normally");
        }
        _ = handle.wait() => {
            info!("Batch collector shutting down; draining buffered events");
            let _ = tokio::time::timeout(Duration::from_secs(30), &mut run_task).await;
        }
    }

    handle.complete().await;
}

async fn run_eviction(
    mut handle: ShutdownHandle,
    config: EvictionConfig,
    flow_table: Arc<PartitionedFlowTable>,
    flow_tx: mpsc::Sender<FlowEvent>,
    clock: Arc<EvictionClock>,
) {
    info!(
        "Starting eviction: idle={}s, active={}s, scan={}s, timewheel={}",
        config.idle_timeout.as_secs(),
        config.active_timeout.as_secs(),
        config.scan_interval.as_secs(),
        config.use_timewheel
    );

    let eviction = Eviction::with_clock(config, flow_table, flow_tx, clock);

    tokio::select! {
        _ = eviction.run() => {
            debug!("Eviction finished normally");
        }
        _ = handle.wait() => {
            info!("Eviction shutting down; performing final drain of remaining flows");
            // Final drain: evict and emit every remaining flow so buffered
            // flows are not silently dropped with the dropped `run()` future.
            let _ = tokio::time::timeout(Duration::from_secs(30), eviction.drain_final()).await;
        }
    }

    handle.complete().await;
}

async fn run_asset_binding_sender(
    mut handle: ShutdownHandle,
    sender: Arc<GrpcSender>,
    spool: Arc<AssetBindingSpool>,
    batch_size: usize,
) {
    let mut ticker = tokio::time::interval(Duration::from_secs(1));
    ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
    loop {
        tokio::select! {
            _ = handle.wait() => {
                info!(
                    "Asset binding sender stopping with {} durable pending observations",
                    spool.pending_count().unwrap_or(0)
                );
                break;
            }
            _ = ticker.tick() => {
                let batch = match spool.pending(batch_size) {
                    Ok(batch) => batch,
                    Err(error) => {
                        metrics::ASSET_BINDING_UPLOAD_FAILURES.inc();
                        error!("Failed to read durable asset binding spool: {}", error);
                        continue;
                    }
                };
                metrics::ASSET_BINDING_WAL_PENDING.set(spool.pending_count().unwrap_or(0) as i64);
                metrics::ASSET_BINDING_WAL_REJECTED.set(spool.rejected_count().unwrap_or(0) as i64);
                if batch.is_empty() {
                    continue;
                }
                let payload = batch.iter().map(|(_, binding)| binding.clone()).collect();
                match sender.send_asset_bindings(payload).await {
                    Ok(response) => match spool.apply_response(&batch, &response) {
                        Ok(applied) => {
                            metrics::ASSET_BINDING_WAL_PENDING.set(spool.pending_count().unwrap_or(0) as i64);
                            metrics::ASSET_BINDING_WAL_REJECTED.set(spool.rejected_count().unwrap_or(0) as i64);
                            info!(
                                "Asset binding Kafka receipts applied: acked={}, rejected={}, retryable={}, pending={}",
                                applied.terminal_success,
                                applied.terminal_rejected,
                                applied.retained_retryable,
                                spool.pending_count().unwrap_or(0)
                            )
                        },
                        Err(error) => {
                            metrics::ASSET_BINDING_RESPONSE_CONTRACT_FAILURES.inc();
                            error!(
                                "Asset binding response violated exact-set contract; WAL retained: {}",
                                error
                            )
                        },
                    },
                    Err(error) => {
                        metrics::ASSET_BINDING_UPLOAD_FAILURES.inc();
                        warn!(
                            "Asset binding upload failed; {} WAL observations retained: {}",
                            batch.len(),
                            error
                        )
                    },
                }
            }
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
    durable_spool_required: bool,
    capture_admission: Option<CaptureAdmissionPermit>,
    mut staged_stop_rx: Option<oneshot::Receiver<()>>,
    stage_tx: Option<mpsc::UnboundedSender<PcapStageEvent>>,
    parser_route: probe_agent::config::ParserRoute,
    eviction_clock: Arc<EvictionClock>,
    asset_discovery: Option<Arc<PassiveAssetDiscovery>>,
) {
    if let Err(error) =
        validate_capture_admission(durable_spool_required, capture_admission.as_ref())
    {
        handle.complete_with_error(error.to_string()).await;
        return;
    }
    if let Some(permit) = capture_admission.as_ref() {
        debug!(
            recovery_decision_age_ms = permit.recovery_decided_at.elapsed().as_millis() as u64,
            "PCAP capture admission permit accepted"
        );
    }
    if !config.producer_enabled {
        info!(
			"Packet producer admission is disabled; recovery, control, heartbeat, and metrics remain active"
		);
        tokio::select! {
            _ = handle.wait() => {
                info!("Disabled capture admission received shutdown signal");
            }
            _ = async {
                match staged_stop_rx.as_mut() {
                    Some(receiver) => { let _ = receiver.await; }
                    None => std::future::pending::<()>().await,
                }
            } => {
                info!("Disabled capture admission received staged stop signal");
            }
        }
        if let Some(stage_tx) = stage_tx.as_ref() {
            let _ = stage_tx.send(PcapStageEvent {
                stage: PcapShutdownStage::CaptureStopped,
                success: true,
                detail: "capture producer admission remained disabled".to_string(),
            });
        }
        handle.complete().await;
        return;
    }
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
    }
    .with_parser_route(parser_route);
    if let Some(discovery) = asset_discovery {
        processor = processor.with_discovery(discovery);
    }

    let mut last_stats = std::time::Instant::now();
    let mut pkt_count: u64 = 0;
    let mut byte_count: u64 = 0;
    let mut last_capturer_stats = capturer.stats();

    info!("Capture started successfully");

    loop {
        tokio::select! {
            _ = async {
                // The underlying poll may block (AF_PACKET recvmmsg with a
                // 100ms socket timeout, XDP poll, PCAP replay rate limiting).
                // Run it on a freed thread so the blocking syscall never
                // stalls the async worker (agent.md: no blocking in async).
                match tokio::task::block_in_place(|| capturer.poll()) {
                    Ok(Some(batch)) => {
                        let count = batch.len();
                        let bytes = batch.total_bytes();

                        pkt_count += count as u64;
                        byte_count += bytes as u64;

                        if let Some((_, max_timestamp)) = batch.time_range() {
                            eviction_clock.observe_capture_micros(max_timestamp);
                        }

                        processor.process_batch(&batch);
                        metrics::inc_capture_local(count as u64, bytes as u64);
                    }
                    Ok(None) => {
                        if capturer.end_of_input() {
                            eviction_clock.mark_end_of_input();
                        }
                        tokio::time::sleep(Duration::from_micros(10)).await;
                    }
                    Err(e) => {
                        error!("Capture error: {}", e);
                        metrics::inc_capture_error_local();
                        tokio::time::sleep(Duration::from_millis(100)).await;
                    }
                }

                if last_stats.elapsed() >= Duration::from_secs(10) {
                    let capturer_stats = capturer.stats();
                    metrics::inc_capture_allocation_drop_local(
                        capturer_stats.allocation_drops.saturating_sub(last_capturer_stats.allocation_drops)
                    );
                    metrics::inc_capture_kernel_drop_local(
                        capturer_stats.kernel_drops.saturating_sub(last_capturer_stats.kernel_drops)
                    );
                    last_capturer_stats = capturer_stats;
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
            _ = async {
                match staged_stop_rx.as_mut() {
                    Some(receiver) => {
                        let _ = receiver.await;
                    }
                    None => std::future::pending::<()>().await,
                }
            } => {
                info!("Capture received staged PCAP stop signal");
                break;
            }
        }
    }

    if let Err(e) = capturer.stop().await {
        warn!("Error stopping capture: {}", e);
    }

    let final_capturer_stats = capturer.stats();
    metrics::inc_capture_allocation_drop_local(
        final_capturer_stats
            .allocation_drops
            .saturating_sub(last_capturer_stats.allocation_drops),
    );
    metrics::inc_capture_kernel_drop_local(
        final_capturer_stats
            .kernel_drops
            .saturating_sub(last_capturer_stats.kernel_drops),
    );

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

    if let Some(stage_tx) = stage_tx {
        let _ = stage_tx.send(PcapStageEvent {
            stage: PcapShutdownStage::CaptureStopped,
            success: true,
            detail: "capture loop joined and capturer stopped".to_string(),
        });
    }

    handle.complete().await;
}

fn validate_capture_admission(
    durable_spool_required: bool,
    permit: Option<&CaptureAdmissionPermit>,
) -> Result<()> {
    if durable_spool_required && permit.is_none() {
        bail!("PCAP capture admission denied: startup recovery permit missing");
    }
    Ok(())
}

async fn wait_for_shutdown(
    shutdown_manager: &Arc<ShutdownManager>,
    _config: &ProbeConfig,
    pcap_runtime: &mut Option<PcapPipelineRuntime>,
) {
    info!("Probe Agent running, press Ctrl+C to stop");

    // Handle both SIGINT (Ctrl+C) and SIGTERM (K8s/container stop): without
    // SIGTERM handling the process would be killed by the default signal
    // disposition and the staged PCAP shutdown / WAL flush would never run.
    let ctrl_c = tokio::signal::ctrl_c();
    let mut sigterm = match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
    {
        Ok(signal) => Some(signal),
        Err(e) => {
            warn!(
                "SIGTERM handler unavailable, falling back to Ctrl+C only: {}",
                e
            );
            None
        }
    };
    tokio::select! {
        _ = ctrl_c => {
            info!("========================================");
            info!("  Received shutdown signal (Ctrl+C)");
            info!("========================================");
        }
        _ = async {
            if let Some(sig) = sigterm.as_mut() {
                let _ = sig.recv().await;
            } else {
                std::future::pending::<()>().await;
            }
        } => {
            info!("========================================");
            info!("  Received shutdown signal (SIGTERM)");
            info!("========================================");
        }
    }

    let grace_period = Duration::from_secs(30);
    if let Some(runtime) = pcap_runtime.as_mut() {
        let report = stage_capture_spool_upload_shutdown(
            runtime,
            tokio::time::Instant::now() + grace_period,
        )
        .await;
        if report.graceful {
            info!(
                "PCAP staged shutdown completed: {:?}",
                report.completed_stages
            );
        } else {
            error!(
                "PCAP staged shutdown was not graceful: {:?}",
                report.failure
            );
        }
    }
    shutdown_manager.clone().shutdown(grace_period).await;
}

async fn stage_capture_spool_upload_shutdown(
    runtime: &mut PcapPipelineRuntime,
    deadline: tokio::time::Instant,
) -> PcapShutdownReport {
    let mut report = PcapShutdownReport::default();
    let stages = [
        PcapShutdownStage::CaptureStopped,
        PcapShutdownStage::FinalSealed,
        PcapShutdownStage::JournalFlushed,
        PcapShutdownStage::BufferReleased,
        PcapShutdownStage::ProducerClosed,
        PcapShutdownStage::UploaderDrained,
        PcapShutdownStage::RecoveryStopped,
    ];

    if runtime
        .capture_stop_tx
        .take()
        .context("capture stop control already consumed")
        .and_then(|sender| {
            sender
                .send(())
                .map_err(|_| anyhow::anyhow!("capture task closed"))
        })
        .is_err()
    {
        report.failure = Some("failed to signal capture stop".to_string());
        return report;
    }
    if !wait_for_pcap_stage(runtime, stages[0], deadline, &mut report).await {
        return report;
    }
    match tokio::time::timeout_at(deadline, &mut runtime.capture_task).await {
        Ok(Ok(())) => {}
        Ok(Err(error)) => {
            report.failure = Some(format!("capture task join failed: {error}"));
            return report;
        }
        Err(_) => {
            report.failure = Some("capture task did not join before deadline".to_string());
            return report;
        }
    }

    if runtime
        .final_seal_tx
        .take()
        .context("final seal control already consumed")
        .and_then(|sender| {
            sender
                .send(())
                .map_err(|_| anyhow::anyhow!("rotator closed"))
        })
        .is_err()
    {
        report.failure = Some("failed to signal final spool seal".to_string());
        return report;
    }
    for stage in &stages[1..5] {
        if !wait_for_pcap_stage(runtime, *stage, deadline, &mut report).await {
            return report;
        }
    }
    match tokio::time::timeout_at(deadline, &mut runtime.rotator_task).await {
        Ok(Ok(())) => {}
        Ok(Err(error)) => {
            report.failure = Some(format!("rotator task join failed: {error}"));
            return report;
        }
        Err(_) => {
            report.failure = Some("rotator task did not join before deadline".to_string());
            return report;
        }
    }

    if runtime
        .uploader_drain_tx
        .take()
        .context("uploader drain control already consumed")
        .and_then(|sender| {
            sender
                .send(())
                .map_err(|_| anyhow::anyhow!("uploader closed"))
        })
        .is_err()
    {
        report.failure = Some("failed to signal uploader drain".to_string());
        return report;
    }
    if !wait_for_pcap_stage(runtime, stages[5], deadline, &mut report).await {
        return report;
    }
    match tokio::time::timeout_at(deadline, &mut runtime.uploader_task).await {
        Ok(Ok(())) => {}
        Ok(Err(error)) => {
            report.failure = Some(format!("uploader task join failed: {error}"));
            return report;
        }
        Err(_) => {
            report.failure = Some("uploader task did not join before deadline".to_string());
            return report;
        }
    }

    if let Some(sender) = runtime.recovery.stop_tx.take() {
        if sender.send(()).is_err() {
            report.failure = Some("failed to signal recovery supervisor stop".to_string());
            return report;
        }
    }
    match tokio::time::timeout_at(deadline, &mut runtime.recovery.supervisor).await {
        Ok(Ok(())) => {}
        Ok(Err(error)) => {
            report.failure = Some(format!("recovery supervisor join failed: {error}"));
            return report;
        }
        Err(_) => {
            report.failure = Some("recovery supervisor did not join before deadline".to_string());
            return report;
        }
    }
    report
        .completed_stages
        .push(PcapShutdownStage::RecoveryStopped);
    report.graceful = true;
    report
}

async fn wait_for_pcap_stage(
    runtime: &mut PcapPipelineRuntime,
    expected: PcapShutdownStage,
    deadline: tokio::time::Instant,
    report: &mut PcapShutdownReport,
) -> bool {
    let event = match tokio::time::timeout_at(deadline, runtime.stage_rx.recv()).await {
        Ok(Some(event)) => event,
        Ok(None) => {
            report.failure = Some(format!("PCAP stage channel closed before {expected:?}"));
            return false;
        }
        Err(_) => {
            report.failure = Some(format!("deadline exceeded before {expected:?}"));
            return false;
        }
    };
    if event.stage != expected {
        report.failure = Some(format!(
            "PCAP shutdown order violation: expected {expected:?}, observed {:?}",
            event.stage
        ));
        return false;
    }
    if !event.success {
        report.failure = Some(format!("{expected:?} failed: {}", event.detail));
        return false;
    }
    report.completed_stages.push(expected);
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    fn successful_pipeline_runtime() -> PcapPipelineRuntime {
        let (capture_stop_tx, capture_stop_rx) = oneshot::channel();
        let (final_seal_tx, final_seal_rx) = oneshot::channel();
        let (uploader_drain_tx, uploader_drain_rx) = oneshot::channel();
        let (recovery_stop_tx, recovery_stop_rx) = oneshot::channel();
        let (stage_tx, stage_rx) = mpsc::unbounded_channel();

        let capture_stage_tx = stage_tx.clone();
        let capture_task = tokio::spawn(async move {
            capture_stop_rx.await.expect("capture stop signal");
            capture_stage_tx
                .send(PcapStageEvent {
                    stage: PcapShutdownStage::CaptureStopped,
                    success: true,
                    detail: "capture stopped".to_string(),
                })
                .expect("capture stage receiver");
        });

        let rotator_stage_tx = stage_tx.clone();
        let rotator_task = tokio::spawn(async move {
            final_seal_rx.await.expect("final seal signal");
            for stage in [
                PcapShutdownStage::FinalSealed,
                PcapShutdownStage::JournalFlushed,
                PcapShutdownStage::BufferReleased,
                PcapShutdownStage::ProducerClosed,
            ] {
                rotator_stage_tx
                    .send(PcapStageEvent {
                        stage,
                        success: true,
                        detail: format!("{stage:?} complete"),
                    })
                    .expect("rotator stage receiver");
            }
        });

        let uploader_task = tokio::spawn(async move {
            uploader_drain_rx.await.expect("uploader drain signal");
            stage_tx
                .send(PcapStageEvent {
                    stage: PcapShutdownStage::UploaderDrained,
                    success: true,
                    detail: "uploader drained".to_string(),
                })
                .expect("uploader stage receiver");
        });
        let supervisor = tokio::spawn(async move {
            let _ = recovery_stop_rx.await;
        });

        PcapPipelineRuntime {
            capture_stop_tx: Some(capture_stop_tx),
            final_seal_tx: Some(final_seal_tx),
            uploader_drain_tx: Some(uploader_drain_tx),
            capture_task,
            rotator_task,
            uploader_task,
            recovery: PcapRecoveryRuntime {
                admission: CaptureAdmissionPermit {
                    recovery_decided_at: tokio::time::Instant::now(),
                },
                supervisor,
                stop_tx: Some(recovery_stop_tx),
            },
            stage_rx,
        }
    }

    #[test]
    fn legacy_pcap_path_does_not_require_durable_recovery_permit() {
        assert!(validate_capture_admission(false, None).is_ok());
        assert!(validate_capture_admission(true, None).is_err());
        let permit = CaptureAdmissionPermit {
            recovery_decided_at: tokio::time::Instant::now(),
        };
        assert!(validate_capture_admission(true, Some(&permit)).is_ok());
    }

    #[tokio::test]
    async fn staged_shutdown_enforces_capture_spool_journal_buffer_producer_uploader_order() {
        let mut runtime = successful_pipeline_runtime();
        let report = stage_capture_spool_upload_shutdown(
            &mut runtime,
            tokio::time::Instant::now() + Duration::from_secs(1),
        )
        .await;

        assert!(report.graceful, "{:?}", report.failure);
        assert_eq!(
            report.completed_stages,
            vec![
                PcapShutdownStage::CaptureStopped,
                PcapShutdownStage::FinalSealed,
                PcapShutdownStage::JournalFlushed,
                PcapShutdownStage::BufferReleased,
                PcapShutdownStage::ProducerClosed,
                PcapShutdownStage::UploaderDrained,
                PcapShutdownStage::RecoveryStopped,
            ]
        );
    }

    #[tokio::test]
    async fn staged_shutdown_deadline_never_reports_false_graceful_completion() {
        let mut runtime = successful_pipeline_runtime();
        runtime.capture_task.abort();

        let report = stage_capture_spool_upload_shutdown(
            &mut runtime,
            tokio::time::Instant::now() + Duration::from_millis(25),
        )
        .await;

        assert!(!report.graceful);
        assert!(report.failure.is_some());
        assert!(!report
            .completed_stages
            .contains(&PcapShutdownStage::UploaderDrained));
    }
}

// ProbeReplayExecutor —— 探针侧回放执行:取对象→hash 校验→有界回放(时间线平移)
// → 逐包经共享 PacketProcessor 喂入聚合器(与在线采集同一真源)→ 计数回执。
// 命令携带 interface 时切换 wire 回放:帧经 AF_PACKET 注入指定接口(veth 输入端,
// 测试阶段数据集生成),allowlist 之外接口 fail-closed;进程内喂入与 wire 注入互斥
// (wire 模式不喂共享分支,该 run 下游消费共享分支同窗口流量——由输出端探针实时采集流入)。
struct ProbeReplayExecutor {
    flow_table: Arc<PartitionedFlowTable>,
    parser_route: probe_agent::config::ParserRoute,
    object_store: ReplayObjectStoreConfig,
    cache_dir: std::path::PathBuf,
    wire_interfaces: Vec<String>,
}

#[async_trait::async_trait]
impl ReplayOperationExecutor for ProbeReplayExecutor {
    async fn execute_replay(&self, cmd: &ReplayWindowCommand) -> anyhow::Result<ReplayExecution> {
        let validated = cmd.validate()?;
        if let Some(iface) = cmd.interface.as_deref().filter(|s| !s.trim().is_empty()) {
            // wire 回放:allowlist 门禁(fail-closed,不伪造注入成功)。
            if !self.wire_interfaces.iter().any(|a| a == iface) {
                return Ok(ReplayExecution {
                    applied: false,
                    packets: 0,
                    bytes_consumed: 0,
                    watermark_ms: 0,
                    detail: format!(
                        "wire replay interface {iface} is not in the probe allowlist {:?}",
                        self.wire_interfaces
                    ),
                });
            }
            let fetcher = S3ObjectFetcher::new(self.object_store.clone());
            let mut injector = match WireInjector::open(iface) {
                Ok(injector) => injector,
                Err(e) => {
                    return Ok(ReplayExecution {
                        applied: false,
                        packets: 0,
                        bytes_consumed: 0,
                        watermark_ms: 0,
                        detail: format!("open wire injector: {e}"),
                    })
                }
            };
            let outcome = execute_wire_replay_window(&fetcher, &self.cache_dir, &validated, &mut |frame| {
                injector.send_frame(frame)
            })
            .await?;
            return Ok(ReplayExecution {
                applied: outcome.applied,
                packets: outcome.packets,
                bytes_consumed: outcome.bytes_consumed,
                watermark_ms: outcome.watermark_ms,
                detail: outcome.detail,
            });
        }
        let fetcher = S3ObjectFetcher::new(self.object_store.clone());
        let mut processor = PacketProcessor::new(self.flow_table.clone())
            .with_parser_route(self.parser_route);
        let outcome = execute_replay_window(&fetcher, &self.cache_dir, &validated, &mut |data, ts_micros| {
            let ts = CaptureTimestamp::from_epoch_micros(
                ts_micros as u64,
                CaptureTimestampProvenance::SourceRecord,
            );
            let batch = PacketBatch::from_owned_packets(vec![(data.to_vec(), ts)]);
            processor.process_batch(&batch);
            Ok(())
        })
        .await?;
        Ok(ReplayExecution {
            applied: outcome.applied,
            packets: outcome.packets,
            bytes_consumed: outcome.bytes_consumed,
            watermark_ms: outcome.watermark_ms,
            detail: outcome.detail,
        })
    }
}
