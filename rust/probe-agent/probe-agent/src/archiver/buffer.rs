use std::sync::atomic::{AtomicUsize, AtomicU64, Ordering};
use parking_lot::Mutex;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use anyhow::Result;

use super::pcap::{PcapGlobalHeader, PcapPacketHeader};

pub struct DoubleBuffer {
    buffer_a: Mutex<Vec<u8>>,
    buffer_b: Mutex<Vec<u8>>,
    active: AtomicUsize,         // 0 = A, 1 = B
    max_size: usize,             // 默认 256MB
    max_duration: Duration,      // 默认 60s
    start_time: AtomicU64,       // Buffer 开始时间 (ms)
    packet_count_a: AtomicU64,
    packet_count_b: AtomicU64,
}

impl DoubleBuffer {
    pub fn new(max_size: usize, max_duration: Duration) -> Self {
        let mut buffer_a = Vec::with_capacity(max_size);
        let mut buffer_b = Vec::with_capacity(max_size);
        
        // 写入 PCAP 全局头
        let header = PcapGlobalHeader::default();
        header.write_to(&mut buffer_a).unwrap();
        header.write_to(&mut buffer_b).unwrap();
        
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        
        Self {
            buffer_a: Mutex::new(buffer_a),
            buffer_b: Mutex::new(buffer_b),
            active: AtomicUsize::new(0),
            max_size,
            max_duration,
            start_time: AtomicU64::new(now_ms),
            packet_count_a: AtomicU64::new(0),
            packet_count_b: AtomicU64::new(0),
        }
    }

    /// 写入数据包
    /// 返回 Some(完整缓冲) 如果需要交换缓冲区
    pub fn write_packet(&self, timestamp_us: u64, data: &[u8]) -> Option<(Vec<u8>, u64, u64)> {
        let active_idx = self.active.load(Ordering::Acquire);
        
        // 构造 PCAP 包头
        let packet_header = PcapPacketHeader::new(timestamp_us, data.len() as u32);
        
        // 获取活动缓冲区
        let (buffer, packet_count) = if active_idx == 0 {
            (&self.buffer_a, &self.packet_count_a)
        } else {
            (&self.buffer_b, &self.packet_count_b)
        };
        
        let mut buf = buffer.lock();
        
        // 写入包头和数据
        packet_header.write_to(&mut *buf).ok()?;
        buf.extend_from_slice(data);
        packet_count.fetch_add(1, Ordering::Relaxed);
        
        // 检查是否需要交换
        let should_swap = buf.len() >= self.max_size 
            || self.buffer_age_ms() >= self.max_duration.as_millis() as u64;
        
        if should_swap {
            drop(buf);  // 释放锁
            Some(self.swap())
        } else {
            None
        }
    }

    /// 强制交换缓冲区
    pub fn swap(&self) -> (Vec<u8>, u64, u64) {
        let current_idx = self.active.load(Ordering::Acquire);
        let next_idx = 1 - current_idx;
        
        // 原子交换
        self.active.store(next_idx, Ordering::Release);
        
        // 获取已满的缓冲区
        let (old_buffer, packet_count) = if current_idx == 0 {
            (&self.buffer_a, &self.packet_count_a)
        } else {
            (&self.buffer_b, &self.packet_count_b)
        };
        
        let mut buf = old_buffer.lock();
        
        // 提取数据
        let data = buf.clone();
        let start_ts = self.start_time.load(Ordering::Acquire);
        let count = packet_count.swap(0, Ordering::Relaxed);
        
        // 重置缓冲区
        buf.clear();
        let header = PcapGlobalHeader::default();
        header.write_to(&mut *buf).unwrap();
        
        // 重置开始时间
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        self.start_time.store(now_ms, Ordering::Release);
        
        (data, start_ts, count)
    }

    /// 获取缓冲区年龄（毫秒）
    fn buffer_age_ms(&self) -> u64 {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        let start = self.start_time.load(Ordering::Acquire);
        now_ms.saturating_sub(start)
    }

    /// 获取当前活动缓冲区大小
    pub fn active_buffer_size(&self) -> usize {
        let active_idx = self.active.load(Ordering::Acquire);
        let buffer = if active_idx == 0 {
            &self.buffer_a
        } else {
            &self.buffer_b
        };
        buffer.lock().len()
    }

    /// 获取当前包数量
    pub fn packet_count(&self) -> u64 {
        let active_idx = self.active.load(Ordering::Acquire);
        let counter = if active_idx == 0 {
            &self.packet_count_a
        } else {
            &self.packet_count_b
        };
        counter.load(Ordering::Relaxed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_double_buffer_write() {
        let buffer = DoubleBuffer::new(1024, Duration::from_secs(60));
        
        let packet = vec![0u8; 100];
        let result = buffer.write_packet(1234567890, &packet);
        
        assert!(result.is_none(), "Should not swap on first write");
        assert!(buffer.active_buffer_size() > 100, "Buffer should contain data");
    }

    #[test]
    fn test_double_buffer_swap() {
        let buffer = DoubleBuffer::new(200, Duration::from_secs(60));
        
        let packet = vec![0u8; 100];
        buffer.write_packet(1234567890, &packet);
        
        // 写入足够多数据触发交换
        let result = buffer.write_packet(1234567891, &packet);
        
        assert!(result.is_some(), "Should swap when size exceeds limit");
    }
}