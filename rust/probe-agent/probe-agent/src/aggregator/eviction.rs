use super::flow_table::{FlowTable, FlowKey, FlowValue};
use tokio::sync::mpsc::Sender;
use tokio::time::{interval, Duration};
use std::sync::Arc;
use std::sync::atomic::Ordering;
use tracing::{debug, warn};

use proto_gen::{FlowEvent, EventHeader, FiveTuple};

#[derive(Clone)]
pub struct EvictionConfig {
    pub idle_timeout: Duration,    // 默认 120s
    pub active_timeout: Duration,  // 默认 1800s
    pub scan_interval: Duration,   // 默认 1s
}

impl Default for EvictionConfig {
    fn default() -> Self {
        Self {
            idle_timeout: Duration::from_secs(120),
            active_timeout: Duration::from_secs(1800),
            scan_interval: Duration::from_secs(1),
        }
    }
}

pub struct Eviction {
    config: EvictionConfig,
    flow_table: Arc<FlowTable>,
    output_tx: Sender<FlowEvent>,
    tenant_id: String,
    probe_id: String,
}

impl Eviction {
    pub fn new(
        config: EvictionConfig,
        flow_table: Arc<FlowTable>,
        output_tx: Sender<FlowEvent>,
        tenant_id: String,
        probe_id: String,
    ) -> Self {
        Self {
            config,
            flow_table,
            output_tx,
            tenant_id,
            probe_id,
        }
    }

    /// 启动老化任务（应在 tokio::spawn 中运行）
    pub async fn run(&self) {
        let mut ticker = interval(self.config.scan_interval);
        let mut evicted_count = 0u64;
        
        loop {
            ticker.tick().await;
            
            let now_ms = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64;
            
            // 扫描流表
            let keys_to_evict: Vec<FlowKey> = self.flow_table
                .iter()
                .filter_map(|entry| {
                    if self.should_evict(entry.key(), entry.value(), now_ms) {
                        Some(entry.key().clone())
                    } else {
                        None
                    }
                })
                .collect();
            
            // 移除并输出
            for key in keys_to_evict {
                if let Some((key, value)) = self.flow_table.remove(&key) {
                    let event = self.to_flow_event(&key, value);
                    
                    if let Err(e) = self.output_tx.send(event).await {
                        warn!("Failed to send flow event: {}", e);
                    }
                    
                    evicted_count += 1;
                }
            }
            
            if evicted_count > 0 && evicted_count % 1000 == 0 {
                debug!("Evicted {} flows so far", evicted_count);
            }
        }
    }

    /// 判断是否应该驱逐
    fn should_evict(&self, key: &FlowKey, value: &FlowValue, now_ms: u64) -> bool {
        let start_time = value.start_time.load(Ordering::Relaxed);
        let last_seen = value.last_seen.load(Ordering::Relaxed);
        
        let idle_ms = now_ms.saturating_sub(last_seen);
        let active_ms = now_ms.saturating_sub(start_time);
        
        // 条件 1: Idle 超时
        if idle_ms > self.config.idle_timeout.as_millis() as u64 {
            return true;
        }
        
        // 条件 2: Active 超时
        if active_ms > self.config.active_timeout.as_millis() as u64 {
            return true;
        }
        
        // 条件 3: TCP FIN/RST
        if key.protocol == 6 {  // TCP
            let flags_fwd = value.tcp_flags_fwd.load(Ordering::Relaxed);
            let flags_bwd = value.tcp_flags_bwd.load(Ordering::Relaxed);
            
            const FIN: u16 = 0x01;
            const RST: u16 = 0x04;
            
            if (flags_fwd & FIN) != 0 || (flags_bwd & FIN) != 0 
                || (flags_fwd & RST) != 0 || (flags_bwd & RST) != 0 {
                return true;
            }
        }
        
        false
    }

    fn to_flow_event(&self, key: &FlowKey, value: FlowValue) -> FlowEvent {
        let start_time = value.start_time.load(Ordering::Relaxed);
        let end_time = value.last_seen.load(Ordering::Relaxed);
        
        FlowEvent {
            header: Some(EventHeader {
                event_id: uuid::Uuid::new_v4().to_string(),
                tenant_id: self.tenant_id.clone(),
                run_id: String::new(),
                event_ts: end_time as i64,
                ingest_ts: chrono::Utc::now().timestamp_millis(),
                probe_id: self.probe_id.clone(),
                feature_set_id: String::new(),
            }),
            flow_id: uuid::Uuid::new_v4().to_string(),
            community_id: key.community_id(),
            tuple: Some(FiveTuple {
                src_ip: key.src_ip.to_string(),
                dst_ip: key.dst_ip.to_string(),
                src_port: key.src_port as u32,
                dst_port: key.dst_port as u32,
                protocol: key.protocol as u32,
            }),
            direction: "c2s".to_string(),
            packets_fwd: value.packets_fwd.load(Ordering::Relaxed),
            packets_bwd: value.packets_bwd.load(Ordering::Relaxed),
            bytes_fwd: value.bytes_fwd.load(Ordering::Relaxed),
            bytes_bwd: value.bytes_bwd.load(Ordering::Relaxed),
            ts_start: start_time as i64,
            ts_end: end_time as i64,
            duration_ms: (end_time - start_time) as u32,
            tcp_flags_fwd: value.tcp_flags_fwd.load(Ordering::Relaxed) as u32,
            tcp_flags_bwd: value.tcp_flags_bwd.load(Ordering::Relaxed) as u32,
            pktlen_min: 0,
            pktlen_max: 0,
            pktlen_mean: 0.0,
            pktlen_std: 0.0,
            iat_min_ms: 0.0,
            iat_max_ms: 0.0,
            iat_mean_ms: 0.0,
            iat_std_ms: 0.0,
            end_reason: "idle_timeout".to_string(),
        }
    }
}