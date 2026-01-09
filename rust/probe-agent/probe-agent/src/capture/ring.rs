/// Ring Buffer 辅助结构（用于 XDP RX/TX/Fill/Completion Rings）
use std::sync::atomic::{AtomicU32, Ordering};

pub struct RingBuffer {
    producer: AtomicU32,
    consumer: AtomicU32,
    size: u32,
    mask: u32,
}

impl RingBuffer {
    pub fn new(size: u32) -> Self {
        assert!(size.is_power_of_two(), "Ring size must be power of 2");
        
        Self {
            producer: AtomicU32::new(0),
            consumer: AtomicU32::new(0),
            size,
            mask: size - 1,
        }
    }

    pub fn produce(&self, count: u32) -> bool {
        let producer = self.producer.load(Ordering::Acquire);
        let consumer = self.consumer.load(Ordering::Acquire);
        
        if producer - consumer + count > self.size {
            return false;  // Ring full
        }
        
        self.producer.fetch_add(count, Ordering::Release);
        true
    }

    pub fn consume(&self, count: u32) -> bool {
        let producer = self.producer.load(Ordering::Acquire);
        let consumer = self.consumer.load(Ordering::Acquire);
        
        if producer - consumer < count {
            return false;  // Not enough entries
        }
        
        self.consumer.fetch_add(count, Ordering::Release);
        true
    }

    pub fn available(&self) -> u32 {
        let producer = self.producer.load(Ordering::Acquire);
        let consumer = self.consumer.load(Ordering::Acquire);
        producer - consumer
    }
}