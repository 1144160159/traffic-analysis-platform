use sled::Db;
use std::path::Path;
use anyhow::{Result, Context};
use tracing::{debug, warn};

use proto_gen::FlowEvent;

pub struct LocalCache {
    db: Db,
    max_size: usize,
}

impl LocalCache {
    pub fn new(path: &Path, max_size: usize) -> Result<Self> {
        let db = sled::open(path)
            .context("Failed to open local cache database")?;
        
        let current_size = db.len();
        debug!("Local cache opened: {} entries", current_size);
        
        Ok(Self {
            db,
            max_size,
        })
    }

    /// 保存失败的批次
    pub fn save(&self, batch: &[FlowEvent]) -> Result<()> {
        // 检查容量
        if self.size()? >= self.max_size {
            warn!("Cache is full (size={}), dropping batch", self.size()?);
            return Ok(()); // 静默失败，避免阻塞
        }
        
        // 生成唯一键（时间戳 + 随机数）
        let key = format!(
            "{}-{}",
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)?
                .as_micros(),
            uuid::Uuid::new_v4()
        );
        
        // 序列化批次（使用 JSON 简化，生产环境应使用 Protobuf）
        let serialized = serde_json::to_vec(batch)
            .context("Failed to serialize batch")?;
        
        self.db.insert(key.as_bytes(), serialized)?;
        self.db.flush()?;
        
        debug!("Cached batch: {} ({} events)", key, batch.len());
        
        Ok(())
    }

    /// 获取待处理的批次
    pub fn get_pending(&self, limit: usize) -> Result<Vec<(String, Vec<FlowEvent>)>> {
        let mut result = Vec::new();
        
        for item in self.db.iter().take(limit) {
            let (key, value) = item?;
            let key_str = String::from_utf8_lossy(&key).to_string();
            let batch: Vec<FlowEvent> = serde_json::from_slice(&value)
                .context("Failed to deserialize cached batch")?;
            result.push((key_str, batch));
        }
        
        Ok(result)
    }

    /// 删除已成功发送的批次
    pub fn remove(&self, key: String) -> Result<()> {
        self.db.remove(key.as_bytes())?;
        self.db.flush()?;
        Ok(())
    }

    /// 获取缓存大小
    pub fn size(&self) -> Result<usize> {
        Ok(self.db.len())
    }

    /// 清空缓存
    pub fn clear(&self) -> Result<()> {
        self.db.clear()?;
        self.db.flush()?;
        Ok(())
    }
}