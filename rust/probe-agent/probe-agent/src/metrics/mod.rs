use anyhow::Result;
use once_cell::sync::Lazy;
use prometheus::{
    Counter, Encoder, Gauge, Histogram, HistogramOpts, IntGauge, Registry, TextEncoder,
};
use std::cell::RefCell;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::Duration;
use tokio::io::AsyncWriteExt;
use tokio::net::TcpListener;
use tracing::{error, info, warn};

static METRICS_REGISTERED: AtomicBool = AtomicBool::new(false);

pub static REGISTRY: Lazy<Registry> = Lazy::new(Registry::new);

pub static PCAP_FALLBACK: Lazy<Counter> = Lazy::new(|| {
    Counter::new(
        "probe_pcap_fallback_total",
        "PCAP writes that fell back to disk",
    )
    .unwrap()
});

pub static PACKETS_CAPTURED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_packets_captured_total", "Total packets captured").unwrap());

pub static PACKETS_DROPPED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_packets_dropped_total", "Total packets dropped").unwrap());

pub static BYTES_CAPTURED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_bytes_captured_total", "Total bytes captured").unwrap());

pub static CAPTURE_PPS: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_capture_pps", "Current packets per second").unwrap());

pub static CAPTURE_ERRORS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_capture_errors_total", "Total capture errors").unwrap());

pub static UMEM_USAGE: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_umem_usage_ratio", "UMEM utilization ratio").unwrap());

pub static UMEM_FREE_FRAMES: Lazy<IntGauge> =
    Lazy::new(|| IntGauge::new("probe_umem_free_frames", "Free frames in UMEM").unwrap());

pub static UMEM_HIGH_WATERMARK: Lazy<IntGauge> = Lazy::new(|| {
    IntGauge::new(
        "probe_umem_high_watermark",
        "Maximum UMEM frames ever allocated",
    )
    .unwrap()
});

pub static PARSE_TOTAL: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_parse_total", "Total parse attempts").unwrap());

pub static PARSE_SUCCESS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_parse_success_total", "Successful parses").unwrap());

pub static PARSE_FAILED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_parse_failed_total", "Failed parses").unwrap());

pub static PARSE_SKIPPED: Lazy<Counter> = Lazy::new(|| {
    Counter::new(
        "probe_parse_skipped_total",
        "Skipped parses (non-IP packets)",
    )
    .unwrap()
});

pub static PARSE_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_parse_latency_seconds", "Parse latency")
            .buckets(vec![0.000001, 0.000005, 0.00001, 0.00005, 0.0001]),
    )
    .unwrap()
});

pub static PACKETS_PROCESSED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_packets_processed_total", "Total packets processed").unwrap());

pub static PACKETS_PARSED: Lazy<Counter> = Lazy::new(|| {
    Counter::new("probe_packets_parsed_total", "Packets successfully parsed").unwrap()
});

pub static PACKETS_FAILED: Lazy<Counter> = Lazy::new(|| {
    Counter::new(
        "probe_packets_parse_failed_total",
        "Packets failed to parse",
    )
    .unwrap()
});

pub static PROCESSOR_BATCHES: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_processor_batches_total", "Total batches processed").unwrap());

pub static PROCESSOR_BATCH_SIZE: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_processor_batch_size", "Batch size")
            .buckets(vec![1.0, 10.0, 50.0, 100.0, 500.0, 1000.0]),
    )
    .unwrap()
});

pub static PROCESSOR_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_processor_latency_seconds", "Processing latency")
            .buckets(vec![0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005]),
    )
    .unwrap()
});

pub static PROCESSOR_NEW_FLOWS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_processor_new_flows_total", "New flows created").unwrap());

pub static PROCESSOR_UPDATED_FLOWS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_processor_updated_flows_total", "Flows updated").unwrap());

pub static ACTIVE_FLOWS: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_active_flows", "Current active flows").unwrap());

pub static FLOWS_EVICTED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_flows_evicted_total", "Total flows evicted").unwrap());

pub static FLOWS_CREATED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_flows_created_total", "Total flows created").unwrap());

pub static FLOW_TABLE_UPDATES: Lazy<Counter> = Lazy::new(|| {
    Counter::new("probe_flow_table_updates_total", "Total flow table updates").unwrap()
});

pub static FLOW_TABLE_UTILIZATION: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_flow_table_utilization", "Flow table utilization").unwrap());

pub static FLOW_TABLE_FRAGMENTATION: Lazy<Gauge> = Lazy::new(|| {
    Gauge::new(
        "probe_flow_table_fragmentation",
        "Flow table partition imbalance ratio (max-min)/avg",
    )
    .unwrap()
});

pub static PCAP_BUFFER_ROTATIONS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_buffer_rotations_total", "Buffer rotations").unwrap());

pub static PCAP_WRITE_SUCCESS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_write_success_total", "Successful writes").unwrap());

pub static PCAP_WRITE_BLOCKED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_write_blocked_total", "Blocked writes").unwrap());

pub static PCAP_WRITE_ERRORS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_write_errors_total", "Write errors").unwrap());

pub static PCAP_WRITER_RECEIVED: Lazy<Counter> = Lazy::new(|| {
    Counter::new(
        "probe_pcap_writer_received_total",
        "Packets received by writer",
    )
    .unwrap()
});

pub static PCAP_WRITER_WRITTEN: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_writer_written_total", "Packets written").unwrap());

pub static PCAP_WRITER_SENT: Lazy<Counter> = Lazy::new(|| {
    Counter::new(
        "probe_pcap_writer_batches_sent_total",
        "Batches sent to writer",
    )
    .unwrap()
});

pub static PCAP_WRITER_DROPPED: Lazy<Counter> = Lazy::new(|| {
    Counter::new("probe_pcap_writer_batches_dropped_total", "Batches dropped").unwrap()
});

pub static PCAP_QUEUE_SIZE: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_pcap_queue_size", "Upload queue size").unwrap());

pub static PCAP_QUEUE_FULL: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_queue_full_total", "Queue full events").unwrap());

pub static PCAP_BYTES_WRITTEN: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_bytes_written_total", "Bytes written").unwrap());

pub static PCAP_FILES_UPLOADED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_files_uploaded_total", "Files uploaded").unwrap());

pub static PCAP_UPLOAD_ERRORS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pcap_upload_errors_total", "Upload errors").unwrap());

pub static PCAP_UPLOAD_QUEUE_DEPTH: Lazy<IntGauge> = Lazy::new(|| {
    IntGauge::new(
        "probe_pcap_upload_queue_depth",
        "Number of PCAP files waiting for upload",
    )
    .unwrap()
});

pub static EVENTS_SENT: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_events_sent_total", "Flow events sent").unwrap());

pub static EVENTS_FAILED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_events_failed_total", "Flow events failed").unwrap());

pub static EVENTS_CACHED: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_events_cached", "Cached events").unwrap());

pub static BATCHES_SENT: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_batches_sent_total", "Batches sent").unwrap());

pub static BATCH_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_batch_latency_seconds", "Batch send latency")
            .buckets(vec![0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0]),
    )
    .unwrap()
});

pub static SENDER_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_sender_latency_seconds", "gRPC sender latency")
            .buckets(vec![0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]),
    )
    .unwrap()
});

pub static IN_FLIGHT_REQUESTS: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_sender_in_flight", "In-flight requests").unwrap());

pub static GRPC_WINDOW_UTILIZATION: Lazy<Gauge> = Lazy::new(|| {
    Gauge::new(
        "probe_grpc_window_utilization",
        "Sliding window utilization (0.0-1.0)",
    )
    .unwrap()
});

pub static POOL_ACQUIRED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pool_acquired_total", "Objects acquired").unwrap());

pub static POOL_RELEASED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pool_released_total", "Objects released").unwrap());

pub static POOL_CREATED: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_pool_created_total", "Objects created (pool miss)").unwrap());

pub static POOL_SIZE: Lazy<IntGauge> =
    Lazy::new(|| IntGauge::new("probe_pool_size", "Current pool size").unwrap());

pub static POOL_HIT_RATE: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_pool_hit_rate", "Pool hit rate").unwrap());

pub static MEMORY_RSS: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_memory_rss_bytes", "Resident set size").unwrap());

pub static CPU_USAGE: Lazy<Gauge> =
    Lazy::new(|| Gauge::new("probe_cpu_usage_ratio", "CPU usage").unwrap());

pub static THREAD_COUNT: Lazy<IntGauge> =
    Lazy::new(|| IntGauge::new("probe_thread_count", "Thread count").unwrap());

pub static FD_COUNT: Lazy<IntGauge> =
    Lazy::new(|| IntGauge::new("probe_fd_count", "Open file descriptors").unwrap());

pub static E2E_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_e2e_latency_seconds", "End-to-end latency")
            .buckets(vec![0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]),
    )
    .unwrap()
});

pub static PROCESSING_LATENCY: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new("probe_processing_latency_seconds", "Processing latency")
            .buckets(vec![0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1]),
    )
    .unwrap()
});

pub static HEARTBEAT_SUCCESS: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_heartbeat_success_total", "Successful heartbeats").unwrap());

pub static HEARTBEAT_FAILURES: Lazy<Counter> =
    Lazy::new(|| Counter::new("probe_heartbeat_failures_total", "Failed heartbeats").unwrap());

pub static EVICTION_SCAN_DURATION: Lazy<Histogram> = Lazy::new(|| {
    Histogram::with_opts(
        HistogramOpts::new(
            "probe_eviction_scan_duration_seconds",
            "Time spent scanning flow table for eviction",
        )
        .buckets(vec![0.001, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]),
    )
    .unwrap()
});

#[repr(align(64))]
struct CachePadded<T>(T);

struct BatchedMetrics {
    bytes: CachePadded<AtomicU64>,
    packets: CachePadded<AtomicU64>,
}

impl BatchedMetrics {
    const fn new() -> Self {
        Self {
            bytes: CachePadded(AtomicU64::new(0)),
            packets: CachePadded(AtomicU64::new(0)),
        }
    }

    #[inline(always)]
    fn add(&self, bytes: u64) {
        self.bytes.0.fetch_add(bytes, Ordering::Relaxed);
        self.packets.0.fetch_add(1, Ordering::Relaxed);
    }

    fn flush_to_prometheus(&self) {
        let bytes = self.bytes.0.swap(0, Ordering::Relaxed);
        let packets = self.packets.0.swap(0, Ordering::Relaxed);

        if bytes > 0 {
            BYTES_CAPTURED.inc_by(bytes as f64);
        }
        if packets > 0 {
            PACKETS_CAPTURED.inc_by(packets as f64);
        }
    }
}

static CAPTURE_METRICS: Lazy<BatchedMetrics> = Lazy::new(BatchedMetrics::new);

thread_local! {
    static LOCAL_PROCESSED_PACKETS: RefCell<u64> = RefCell::new(0);
    static LOCAL_PARSED_PACKETS: RefCell<u64> = RefCell::new(0);
    static LOCAL_FAILED_PACKETS: RefCell<u64> = RefCell::new(0);
    static LOCAL_NEW_FLOWS: RefCell<u64> = RefCell::new(0);
    static LOCAL_UPDATED_FLOWS: RefCell<u64> = RefCell::new(0);
}

const LOCAL_FLUSH_THRESHOLD_FLOWS: u64 = 1_000;

#[inline(always)]
pub fn inc_capture_local(bytes: u64) {
    CAPTURE_METRICS.add(bytes);
}

#[inline]
pub fn inc_processed_local() {
    LOCAL_PROCESSED_PACKETS.with(|cell| {
        *cell.borrow_mut() += 1;
    });
}

#[inline]
pub fn inc_parse_result_local(success: bool) {
    if success {
        LOCAL_PARSED_PACKETS.with(|cell| {
            *cell.borrow_mut() += 1;
        });
    } else {
        LOCAL_FAILED_PACKETS.with(|cell| {
            *cell.borrow_mut() += 1;
        });
    }
}

#[inline]
pub fn inc_flow_update_local(is_new: bool) {
    if is_new {
        LOCAL_NEW_FLOWS.with(|cell| {
            let mut val = cell.borrow_mut();
            *val += 1;

            if *val >= LOCAL_FLUSH_THRESHOLD_FLOWS {
                PROCESSOR_NEW_FLOWS.inc_by(*val as f64);
                FLOWS_CREATED.inc_by(*val as f64);
                *val = 0;
            }
        });
    } else {
        LOCAL_UPDATED_FLOWS.with(|cell| {
            let mut val = cell.borrow_mut();
            *val += 1;

            if *val >= LOCAL_FLUSH_THRESHOLD_FLOWS {
                PROCESSOR_UPDATED_FLOWS.inc_by(*val as f64);
                FLOW_TABLE_UPDATES.inc_by(*val as f64);
                *val = 0;
            }
        });
    }
}

pub fn flush_local_metrics() {
    CAPTURE_METRICS.flush_to_prometheus();

    LOCAL_PROCESSED_PACKETS.with(|cell| {
        let packets = *cell.borrow();
        if packets > 0 {
            PACKETS_PROCESSED.inc_by(packets as f64);
            *cell.borrow_mut() = 0;
        }
    });

    LOCAL_PARSED_PACKETS.with(|cell| {
        let packets = *cell.borrow();
        if packets > 0 {
            PACKETS_PARSED.inc_by(packets as f64);
            *cell.borrow_mut() = 0;
        }
    });

    LOCAL_FAILED_PACKETS.with(|cell| {
        let packets = *cell.borrow();
        if packets > 0 {
            PACKETS_FAILED.inc_by(packets as f64);
            *cell.borrow_mut() = 0;
        }
    });

    LOCAL_NEW_FLOWS.with(|cell| {
        let flows = *cell.borrow();
        if flows > 0 {
            PROCESSOR_NEW_FLOWS.inc_by(flows as f64);
            FLOWS_CREATED.inc_by(flows as f64);
            *cell.borrow_mut() = 0;
        }
    });

    LOCAL_UPDATED_FLOWS.with(|cell| {
        let flows = *cell.borrow();
        if flows > 0 {
            PROCESSOR_UPDATED_FLOWS.inc_by(flows as f64);
            FLOW_TABLE_UPDATES.inc_by(flows as f64);
            *cell.borrow_mut() = 0;
        }
    });
}

pub fn flush_all_local_metrics() {
    flush_local_metrics();
}

#[derive(Debug, Clone, Default)]
pub struct LocalMetricsSnapshot {
    pub capture_bytes: u64,
    pub capture_packets: u64,
    pub processed_packets: u64,
    pub parsed_packets: u64,
    pub failed_packets: u64,
    pub new_flows: u64,
    pub updated_flows: u64,
}

pub fn get_local_stats() -> LocalMetricsSnapshot {
    let capture_bytes = CAPTURE_METRICS.bytes.0.load(Ordering::Relaxed);
    let capture_packets = CAPTURE_METRICS.packets.0.load(Ordering::Relaxed);
    let processed = LOCAL_PROCESSED_PACKETS.with(|cell| *cell.borrow());
    let parsed = LOCAL_PARSED_PACKETS.with(|cell| *cell.borrow());
    let failed = LOCAL_FAILED_PACKETS.with(|cell| *cell.borrow());
    let new_flows = LOCAL_NEW_FLOWS.with(|cell| *cell.borrow());
    let updated_flows = LOCAL_UPDATED_FLOWS.with(|cell| *cell.borrow());

    LocalMetricsSnapshot {
        capture_bytes,
        capture_packets,
        processed_packets: processed,
        parsed_packets: parsed,
        failed_packets: failed,
        new_flows,
        updated_flows,
    }
}

pub fn update_flow_table_fragmentation(fragmentation: f64) {
    FLOW_TABLE_FRAGMENTATION.set(fragmentation);
}

pub fn update_umem_metrics(free_frames: usize, high_watermark: usize, utilization: f64) {
    UMEM_FREE_FRAMES.set(free_frames as i64);
    UMEM_HIGH_WATERMARK.set(high_watermark as i64);
    UMEM_USAGE.set(utilization);
}

pub fn update_pcap_queue_depth(depth: usize) {
    PCAP_UPLOAD_QUEUE_DEPTH.set(depth as i64);
}

pub fn update_grpc_window_utilization(utilization: f64) {
    GRPC_WINDOW_UTILIZATION.set(utilization);
}

pub fn record_eviction_scan_duration(duration_secs: f64) {
    EVICTION_SCAN_DURATION.observe(duration_secs);
}

pub fn update_pool_metrics(size: usize, hit_rate: f64) {
    POOL_SIZE.set(size as i64);
    POOL_HIT_RATE.set(hit_rate);
}

pub fn register_metrics() -> Result<()> {
    if METRICS_REGISTERED
        .compare_exchange(false, true, Ordering::SeqCst, Ordering::SeqCst)
        .is_err()
    {
        return Ok(());
    }

    macro_rules! register {
        ($metric:expr) => {
            if let Err(e) = REGISTRY.register(Box::new($metric.clone())) {
                warn!("Failed to register metric: {}", e);
            }
        };
    }

    register!(*PACKETS_CAPTURED);
    register!(*PACKETS_DROPPED);
    register!(*BYTES_CAPTURED);
    register!(*CAPTURE_PPS);
    register!(*CAPTURE_ERRORS);
    register!(*UMEM_USAGE);
    register!(*UMEM_FREE_FRAMES);
    register!(*UMEM_HIGH_WATERMARK);

    register!(*PARSE_TOTAL);
    register!(*PARSE_SUCCESS);
    register!(*PARSE_FAILED);
    register!(*PARSE_SKIPPED);
    register!(*PARSE_LATENCY);

    register!(*PACKETS_PROCESSED);
    register!(*PACKETS_PARSED);
    register!(*PACKETS_FAILED);
    register!(*PROCESSOR_BATCHES);
    register!(*PROCESSOR_BATCH_SIZE);
    register!(*PROCESSOR_LATENCY);
    register!(*PROCESSOR_NEW_FLOWS);
    register!(*PROCESSOR_UPDATED_FLOWS);

    register!(*ACTIVE_FLOWS);
    register!(*FLOWS_EVICTED);
    register!(*FLOWS_CREATED);
    register!(*FLOW_TABLE_UPDATES);
    register!(*FLOW_TABLE_UTILIZATION);
    register!(*FLOW_TABLE_FRAGMENTATION);

    register!(*PCAP_BUFFER_ROTATIONS);
    register!(*PCAP_WRITE_SUCCESS);
    register!(*PCAP_WRITE_BLOCKED);
    register!(*PCAP_WRITE_ERRORS);
    register!(*PCAP_WRITER_RECEIVED);
    register!(*PCAP_WRITER_WRITTEN);
    register!(*PCAP_WRITER_SENT);
    register!(*PCAP_WRITER_DROPPED);
    register!(*PCAP_QUEUE_SIZE);
    register!(*PCAP_QUEUE_FULL);
    register!(*PCAP_BYTES_WRITTEN);
    register!(*PCAP_FILES_UPLOADED);
    register!(*PCAP_UPLOAD_ERRORS);
    register!(*PCAP_UPLOAD_QUEUE_DEPTH);
    register!(*PCAP_FALLBACK);

    register!(*EVENTS_SENT);
    register!(*EVENTS_FAILED);
    register!(*EVENTS_CACHED);
    register!(*BATCHES_SENT);
    register!(*BATCH_LATENCY);
    register!(*IN_FLIGHT_REQUESTS);
    register!(*SENDER_LATENCY);
    register!(*GRPC_WINDOW_UTILIZATION);

    register!(*POOL_ACQUIRED);
    register!(*POOL_RELEASED);
    register!(*POOL_CREATED);
    register!(*POOL_SIZE);
    register!(*POOL_HIT_RATE);

    register!(*MEMORY_RSS);
    register!(*CPU_USAGE);
    register!(*THREAD_COUNT);
    register!(*FD_COUNT);

    register!(*E2E_LATENCY);
    register!(*PROCESSING_LATENCY);
    register!(*HEARTBEAT_SUCCESS);
    register!(*HEARTBEAT_FAILURES);
    register!(*EVICTION_SCAN_DURATION);

    info!("Prometheus metrics registered successfully");
    Ok(())
}

pub async fn serve_metrics(addr: &str) -> Result<()> {
    let listener = TcpListener::bind(addr).await?;
    info!("Metrics server listening on {}", addr);

    tokio::spawn(collect_system_metrics());
    tokio::spawn(flush_metrics_periodically());

    loop {
        let (mut socket, peer_addr) = listener.accept().await?;

        tokio::spawn(async move {
            let mut buf = [0u8; 1024];
            if let Err(e) = tokio::io::AsyncReadExt::read(&mut socket, &mut buf).await {
                error!("Failed to read request from {}: {}", peer_addr, e);
                return;
            }

            flush_local_metrics();

            let encoder = TextEncoder::new();
            let metric_families = REGISTRY.gather();
            let mut buffer = Vec::new();

            if let Err(e) = encoder.encode(&metric_families, &mut buffer) {
                error!("Failed to encode metrics: {}", e);
                return;
            }

            let response = format!(
                "HTTP/1.1 200 OK\r\n\
                Content-Type: text/plain; charset=utf-8\r\n\
                Content-Length: {}\r\n\
                Connection: close\r\n\
                \r\n",
                buffer.len()
            );

            if let Err(e) = socket.write_all(response.as_bytes()).await {
                error!("Failed to write response header: {}", e);
                return;
            }

            if let Err(e) = socket.write_all(&buffer).await {
                error!("Failed to write response body: {}", e);
            }
        });
    }
}

async fn flush_metrics_periodically() {
    let mut interval = tokio::time::interval(Duration::from_secs(1));

    loop {
        interval.tick().await;
        CAPTURE_METRICS.flush_to_prometheus();
    }
}

async fn collect_system_metrics() {
    let mut interval = tokio::time::interval(Duration::from_secs(5));

    loop {
        interval.tick().await;

        if let Ok(rss) = get_memory_rss() {
            MEMORY_RSS.set(rss as f64);
        }

        if let Ok(fd) = get_fd_count() {
            FD_COUNT.set(fd as i64);
        }

        if let Ok(threads) = get_thread_count() {
            THREAD_COUNT.set(threads as i64);
        }

        if let Ok(cpu) = get_cpu_usage_percent() {
            CPU_USAGE.set(cpu);
        }
    }
}

fn get_memory_rss() -> Result<usize> {
    #[cfg(target_os = "linux")]
    {
        let content = std::fs::read_to_string("/proc/self/status")?;
        for line in content.lines() {
            if line.starts_with("VmRSS:") {
                let rss = line
                    .split_whitespace()
                    .nth(1)
                    .and_then(|s| s.parse::<usize>().ok())
                    .unwrap_or(0)
                    * 1024;
                return Ok(rss);
            }
        }
        Ok(0)
    }

    #[cfg(not(target_os = "linux"))]
    Ok(0)
}

fn get_fd_count() -> Result<usize> {
    #[cfg(target_os = "linux")]
    {
        let entries = std::fs::read_dir("/proc/self/fd")?;
        Ok(entries.count())
    }

    #[cfg(not(target_os = "linux"))]
    Ok(0)
}

fn get_thread_count() -> Result<usize> {
    #[cfg(target_os = "linux")]
    {
        let content = std::fs::read_to_string("/proc/self/status")?;
        for line in content.lines() {
            if line.starts_with("Threads:") {
                let threads = line
                    .split_whitespace()
                    .nth(1)
                    .and_then(|s| s.parse::<usize>().ok())
                    .unwrap_or(1);
                return Ok(threads);
            }
        }
        Ok(1)
    }

    #[cfg(not(target_os = "linux"))]
    Ok(1)
}

fn get_cpu_usage_percent() -> Result<f64> {
    #[cfg(target_os = "linux")]
    {
        use std::sync::Mutex;
        use std::time::Instant;

        static LAST_CPU_STATS: Lazy<Mutex<Option<(u64, u64, Instant)>>> =
            Lazy::new(|| Mutex::new(None));

        let stat = std::fs::read_to_string("/proc/self/stat")?;
        let fields: Vec<&str> = stat.split_whitespace().collect();

        if fields.len() < 15 {
            return Ok(0.0);
        }

        let utime = fields[13].parse::<u64>().unwrap_or(0);
        let stime = fields[14].parse::<u64>().unwrap_or(0);
        let total_time = utime + stime;

        let mut last_stats = LAST_CPU_STATS.lock().unwrap();
        let now = Instant::now();

        let cpu_usage = if let Some((last_total, _, last_instant)) = *last_stats {
            let time_delta = now.duration_since(last_instant).as_secs_f64();
            if time_delta > 0.0 {
                let cpu_delta = total_time.saturating_sub(last_total);
                let clk_tck = 100.0;
                (cpu_delta as f64 / clk_tck) / time_delta
            } else {
                0.0
            }
        } else {
            0.0
        };

        *last_stats = Some((total_time, total_time, now));

        Ok(cpu_usage)
    }

    #[cfg(not(target_os = "linux"))]
    Ok(0.0)
}
