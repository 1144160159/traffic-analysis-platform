use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PcapIndexMeta {
    pub tenant_id: String,
    pub probe_id: String,
    pub file_key: String,
    pub ts_start: i64,
    pub ts_end: i64,
    pub byte_size: u64,
    pub packet_count: u64,
    pub zstd_level: i32,
    pub sha256: String,
}

impl PcapIndexMeta {
    pub fn new(
        tenant_id: String,
        probe_id: String,
        file_key: String,
        ts_start: i64,
        ts_end: i64,
        byte_size: u64,
        packet_count: u64,
        zstd_level: i32,
        sha256: String,
    ) -> Self {
        Self {
            tenant_id,
            probe_id,
            file_key,
            ts_start,
            ts_end,
            byte_size,
            packet_count,
            zstd_level,
            sha256,
        }
    }
}
