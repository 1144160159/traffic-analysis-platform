use prometheus::{Encoder, TextEncoder, Registry, Counter, Gauge, Histogram, HistogramOpts};
use hyper::{
    Body, Request, Response, Server, StatusCode,
    service::{make_service_fn, service_fn},
};
use std::convert::Infallible;
use std::sync::Arc;
use anyhow::Result;

lazy_static::lazy_static! {
    pub static ref REGISTRY: Registry = Registry::new();
    
    pub static ref PACKETS_CAPTURED: Counter = Counter::new(
        "probe_packets_captured_total",
        "Total packets captured"
    ).unwrap();
    
    pub static ref PACKETS_DROPPED: Counter = Counter::new(
        "probe_packets_dropped_total",
        "Total packets dropped"
    ).unwrap();
    
    pub static ref FLOWS_ACTIVE: Gauge = Gauge::new(
        "probe_flows_active",
        "Number of active flows"
    ).unwrap();
    
    pub static ref FLOWS_EVICTED: Counter = Counter::new(
        "probe_flows_evicted_total",
        "Total flows evicted"
    ).unwrap();
    
    pub static ref GRPC_REQUESTS: Counter = Counter::new(
        "probe_grpc_requests_total",
        "Total gRPC requests sent"
    ).unwrap();
    
    pub static ref GRPC_ERRORS: Counter = Counter::new(
        "probe_grpc_errors_total",
        "Total gRPC errors"
    ).unwrap();
    
    pub static ref PCAP_UPLOADS: Counter = Counter::new(
        "probe_pcap_uploads_total",
        "Total PCAP files uploaded"
    ).unwrap();
    
    pub static ref PCAP_UPLOAD_BYTES: Counter = Counter::new(
        "probe_pcap_upload_bytes_total",
        "Total bytes uploaded"
    ).unwrap();
}

pub fn register_metrics() {
    REGISTRY.register(Box::new(PACKETS_CAPTURED.clone())).unwrap();
    REGISTRY.register(Box::new(PACKETS_DROPPED.clone())).unwrap();
    REGISTRY.register(Box::new(FLOWS_ACTIVE.clone())).unwrap();
    REGISTRY.register(Box::new(FLOWS_EVICTED.clone())).unwrap();
    REGISTRY.register(Box::new(GRPC_REQUESTS.clone())).unwrap();
    REGISTRY.register(Box::new(GRPC_ERRORS.clone())).unwrap();
    REGISTRY.register(Box::new(PCAP_UPLOADS.clone())).unwrap();
    REGISTRY.register(Box::new(PCAP_UPLOAD_BYTES.clone())).unwrap();
}

pub struct MetricsServer {
    addr: String,
}

impl MetricsServer {
    pub fn new(addr: &str) -> Result<Self> {
        register_metrics();
        Ok(Self {
            addr: addr.to_string(),
        })
    }

    pub async fn run(self) -> Result<()> {
        let addr = self.addr.parse()?;
        
        let make_svc = make_service_fn(|_conn| async {
            Ok::<_, Infallible>(service_fn(metrics_handler))
        });
        
        let server = Server::bind(&addr).serve(make_svc);
        
        tracing::info!("Metrics server listening on http://{}", addr);
        
        server.await?;
        Ok(())
    }
}

async fn metrics_handler(_req: Request<Body>) -> Result<Response<Body>, Infallible> {
    let encoder = TextEncoder::new();
    let metric_families = REGISTRY.gather();
    let mut buffer = vec![];
    encoder.encode(&metric_families, &mut buffer).unwrap();
    
    Ok(Response::builder()
        .status(StatusCode::OK)
        .header("Content-Type", encoder.format_type())
        .body(Body::from(buffer))
        .unwrap())
}