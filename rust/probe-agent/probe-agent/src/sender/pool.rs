use crossbeam_queue::ArrayQueue;
use proto_gen::FlowEvent;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

#[derive(Debug, Default)]
pub struct PoolStats {
    pub acquired: AtomicU64,

    pub released: AtomicU64,

    pub created: AtomicU64,

    pub pool_size: AtomicU64,
}

impl PoolStats {
    pub fn hit_rate(&self) -> f64 {
        let acquired = self.acquired.load(Ordering::Relaxed);
        let created = self.created.load(Ordering::Relaxed);
        if acquired == 0 {
            return 1.0;
        }
        1.0 - (created as f64 / acquired as f64)
    }
}

pub struct FlowEventPool {
    pool: ArrayQueue<Box<FlowEvent>>,
    capacity: usize,
    stats: PoolStats,
}

impl FlowEventPool {
    pub fn new(capacity: usize) -> Arc<Self> {
        let pool = ArrayQueue::new(capacity);

        let pre_alloc = capacity.min(1024);
        for _ in 0..pre_alloc {
            let event = Box::new(FlowEvent::default());
            pool.push(event).ok();
        }

        tracing::info!(
            "FlowEvent pool created: capacity={}, pre-allocated={}",
            capacity,
            pre_alloc
        );

        Arc::new(Self {
            pool,
            capacity,
            stats: PoolStats {
                pool_size: AtomicU64::new(pre_alloc as u64),
                ..Default::default()
            },
        })
    }

    pub fn acquire(self: &Arc<Self>) -> PooledFlowEvent {
        self.stats.acquired.fetch_add(1, Ordering::Relaxed);

        let event = match self.pool.pop() {
            Some(mut event) => {
                Self::reset_event(&mut event);
                event
            }
            None => {
                self.stats.created.fetch_add(1, Ordering::Relaxed);
                Box::new(FlowEvent::default())
            }
        };

        self.stats
            .pool_size
            .store(self.pool.len() as u64, Ordering::Relaxed);

        PooledFlowEvent {
            event: Some(event),
            pool: Arc::clone(self),
        }
    }

    fn release(&self, mut event: Box<FlowEvent>) {
        self.stats.released.fetch_add(1, Ordering::Relaxed);

        Self::reset_event(&mut event);

        if self.pool.push(event).is_err() {
            tracing::trace!("Pool full, dropping FlowEvent");
        }

        self.stats
            .pool_size
            .store(self.pool.len() as u64, Ordering::Relaxed);
    }

    #[inline]
    fn reset_event(event: &mut FlowEvent) {
        event.header = None;
        event.flow_id.clear();
        event.community_id.clear();
        event.tuple = None;
        event.direction.clear();
        event.packets_fwd = 0;
        event.packets_bwd = 0;
        event.bytes_fwd = 0;
        event.bytes_bwd = 0;
        event.ts_start = 0;
        event.ts_end = 0;
        event.duration_ms = 0;
        event.tcp_flags_fwd = 0;
        event.tcp_flags_bwd = 0;
        event.pktlen_stats = None;
        event.iat_stats = None;
        event.active_stats = None;
        event.idle_stats = None;
    }

    pub fn stats(&self) -> &PoolStats {
        &self.stats
    }

    pub fn len(&self) -> usize {
        self.pool.len()
    }

    pub fn is_empty(&self) -> bool {
        self.pool.is_empty()
    }

    pub fn capacity(&self) -> usize {
        self.capacity
    }
}

pub struct PooledFlowEvent {
    event: Option<Box<FlowEvent>>,
    pool: Arc<FlowEventPool>,
}

impl PooledFlowEvent {
    pub fn take(mut self) -> FlowEvent {
        *self
            .event
            .take()
            .expect("PooledFlowEvent: invariant violated, event is None after take")
    }

    pub fn clone_inner(&self) -> FlowEvent {
        self.event
            .as_ref()
            .expect("PooledFlowEvent: invariant violated")
            .as_ref()
            .clone()
    }

    pub fn as_mut(&mut self) -> &mut FlowEvent {
        self.event
            .as_mut()
            .expect("PooledFlowEvent: invariant violated")
    }
}

impl std::ops::Deref for PooledFlowEvent {
    type Target = FlowEvent;

    fn deref(&self) -> &Self::Target {
        self.event
            .as_ref()
            .expect("PooledFlowEvent: invariant violated in deref")
    }
}

impl std::ops::DerefMut for PooledFlowEvent {
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.event
            .as_mut()
            .expect("PooledFlowEvent: invariant violated in deref_mut")
    }
}

impl Drop for PooledFlowEvent {
    fn drop(&mut self) {
        if let Some(event) = self.event.take() {
            self.pool.release(event);
        }
    }
}

pub struct PooledEventBatch {
    events: Vec<FlowEvent>,
    pool: Arc<FlowEventPool>,
}

impl PooledEventBatch {
    pub fn new(pool: Arc<FlowEventPool>, capacity: usize) -> Self {
        Self {
            events: Vec::with_capacity(capacity),
            pool,
        }
    }

    pub fn push(&mut self, event: PooledFlowEvent) {
        self.events.push(event.take());
    }

    pub fn push_owned(&mut self, event: FlowEvent) {
        self.events.push(event);
    }

    pub fn len(&self) -> usize {
        self.events.len()
    }

    pub fn is_empty(&self) -> bool {
        self.events.is_empty()
    }

    pub fn capacity(&self) -> usize {
        self.events.capacity()
    }

    pub fn take(self) -> Vec<FlowEvent> {
        self.events
    }

    pub fn as_slice(&self) -> &[FlowEvent] {
        &self.events
    }

    pub fn clear(&mut self) {
        self.events.clear();
    }
}
