use tokio::sync::mpsc::{Receiver, Sender};
use tokio::time::{Duration, Instant};
use tracing::{debug, error, info, warn};

use proto_gen::FlowEvent;

#[derive(Clone, Debug)]
pub struct BatchConfig {
    pub batch_size: usize,
    pub batch_timeout: Duration,
}

impl Default for BatchConfig {
    fn default() -> Self {
        Self {
            batch_size: 100,
            batch_timeout: Duration::from_millis(100),
        }
    }
}

impl BatchConfig {
    pub fn high_throughput() -> Self {
        Self {
            batch_size: 500,
            batch_timeout: Duration::from_millis(50),
        }
    }

    pub fn low_latency() -> Self {
        Self {
            batch_size: 50,
            batch_timeout: Duration::from_millis(20),
        }
    }
}

pub struct BatchCollector {
    config: BatchConfig,
    buffer: Vec<FlowEvent>,
    last_flush: Instant,
}

impl BatchCollector {
    pub fn new(config: BatchConfig) -> Self {
        Self {
            buffer: Vec::with_capacity(config.batch_size),
            last_flush: Instant::now(),
            config,
        }
    }

    pub fn push(&mut self, event: FlowEvent) -> Option<Vec<FlowEvent>> {
        self.buffer.push(event);

        if self.should_flush() {
            Some(self.flush())
        } else {
            None
        }
    }

    pub fn push_many(&mut self, events: Vec<FlowEvent>) -> Vec<Vec<FlowEvent>> {
        let mut batches = Vec::new();

        for event in events {
            if let Some(batch) = self.push(event) {
                batches.push(batch);
            }
        }

        batches
    }

    pub fn should_flush(&self) -> bool {
        if self.buffer.len() >= self.config.batch_size {
            return true;
        }

        if !self.buffer.is_empty() && self.last_flush.elapsed() >= self.config.batch_timeout {
            return true;
        }

        false
    }

    pub fn flush(&mut self) -> Vec<FlowEvent> {
        self.last_flush = Instant::now();
        std::mem::replace(&mut self.buffer, Vec::with_capacity(self.config.batch_size))
    }

    pub fn flush_if_not_empty(&mut self) -> Option<Vec<FlowEvent>> {
        if self.buffer.is_empty() {
            None
        } else {
            Some(self.flush())
        }
    }

    pub fn len(&self) -> usize {
        self.buffer.len()
    }

    pub fn is_empty(&self) -> bool {
        self.buffer.is_empty()
    }

    pub fn remaining_capacity(&self) -> usize {
        self.config.batch_size.saturating_sub(self.buffer.len())
    }

    pub fn time_since_flush(&self) -> Duration {
        self.last_flush.elapsed()
    }
}

pub struct BatchSender {
    config: BatchConfig,
    rx: Receiver<FlowEvent>,
    batch_tx: Sender<Vec<FlowEvent>>,
}

impl BatchSender {
    pub fn new(
        config: BatchConfig,
        rx: Receiver<FlowEvent>,
        batch_tx: Sender<Vec<FlowEvent>>,
    ) -> Self {
        Self {
            config,
            rx,
            batch_tx,
        }
    }

    pub async fn run(mut self) {
        let mut collector = BatchCollector::new(self.config.clone());
        let mut total_events: u64 = 0;
        let mut total_batches: u64 = 0;
        let mut shutdown_triggered = false;

        info!(
            "Batch sender started: batch_size={}, timeout={}ms",
            self.config.batch_size,
            self.config.batch_timeout.as_millis()
        );

        loop {
            let time_since_flush = collector.time_since_flush();
            let remaining_timeout = self.config.batch_timeout.saturating_sub(time_since_flush);

            tokio::select! {
                result = self.rx.recv() => {
                    match result {
                        Some(event) => {
                            total_events += 1;

                            if let Some(batch) = collector.push(event) {
                                total_batches += 1;
                                if let Err(e) = self.batch_tx.send(batch).await {
                                    error!("Failed to send batch: {}, initiating shutdown", e);
                                    shutdown_triggered = true;
                                    break;
                                }
                            }
                        }
                        None => {
                            info!("Input channel closed, initiating graceful shutdown");
                            shutdown_triggered = true;
                            break;
                        }
                    }
                }

                _ = tokio::time::sleep(remaining_timeout), if !collector.is_empty() => {
                    if collector.should_flush() && !collector.is_empty() {
                        let batch = collector.flush();
                        total_batches += 1;
                        if let Err(e) = self.batch_tx.send(batch).await {
                            error!("Failed to send batch: {}, initiating shutdown", e);
                            shutdown_triggered = true;
                            break;
                        }
                    }
                }
            }
        }

        info!("BatchSender shutdown: flushing remaining data...");

        if let Some(batch) = collector.flush_if_not_empty() {
            let batch_len = batch.len();
            total_batches += 1;

            match self.batch_tx.send(batch).await {
                Ok(_) => {
                    debug!("Flushed collector: {} events", batch_len);
                }
                Err(e) => {
                    error!(
                        "Failed to send final collector batch: {} ({} events LOST)",
                        e, batch_len
                    );
                }
            }
        }

        let mut drained_events = 0;
        let mut drain_batch = Vec::with_capacity(self.config.batch_size);

        let drain_deadline = tokio::time::Instant::now() + tokio::time::Duration::from_secs(5);

        while tokio::time::Instant::now() < drain_deadline {
            match self.rx.try_recv() {
                Ok(event) => {
                    drain_batch.push(event);
                    drained_events += 1;

                    if drain_batch.len() >= self.config.batch_size {
                        total_batches += 1;
                        let batch = std::mem::replace(
                            &mut drain_batch,
                            Vec::with_capacity(self.config.batch_size),
                        );

                        match self.batch_tx.send(batch).await {
                            Ok(_) => {
                                debug!("Sent drained batch: {} events", self.config.batch_size);
                            }
                            Err(e) => {
                                error!(
                                    "Failed to send drained batch: {} ({} events LOST)",
                                    e, self.config.batch_size
                                );
                                break;
                            }
                        }
                    }
                }
                Err(tokio::sync::mpsc::error::TryRecvError::Empty) => {
                    break;
                }
                Err(tokio::sync::mpsc::error::TryRecvError::Disconnected) => {
                    break;
                }
            }
        }

        if !drain_batch.is_empty() {
            let batch_len = drain_batch.len();
            total_batches += 1;

            match self.batch_tx.send(drain_batch).await {
                Ok(_) => {
                    debug!("Sent final drain batch: {} events", batch_len);
                }
                Err(e) => {
                    error!(
                        "Failed to send final drain batch: {} ({} events LOST)",
                        e, batch_len
                    );
                }
            }
        }

        let total_processed = total_events + drained_events as u64;

        info!(
            "BatchSender stopped: total_events={}, total_batches={}, drained={}, shutdown_clean={}",
            total_processed, total_batches, drained_events, !shutdown_triggered
        );

        drop(self.batch_tx);
    }

    pub async fn run_with_shutdown(
        mut self,
        mut shutdown_rx: tokio::sync::broadcast::Receiver<()>,
    ) {
        let mut collector = BatchCollector::new(self.config.clone());
        let mut total_events: u64 = 0;
        let mut total_batches: u64 = 0;

        info!(
            "Batch sender started (with shutdown): batch_size={}, timeout={}ms",
            self.config.batch_size,
            self.config.batch_timeout.as_millis()
        );

        loop {
            let time_since_flush = collector.time_since_flush();
            let remaining_timeout = self.config.batch_timeout.saturating_sub(time_since_flush);

            tokio::select! {
                _ = shutdown_rx.recv() => {
                    info!("Batch sender received shutdown signal, flushing {} buffered events", collector.len());
                    break;
                }

                result = self.rx.recv() => {
                    match result {
                        Some(event) => {
                            total_events += 1;

                            if let Some(batch) = collector.push(event) {
                                total_batches += 1;
                                if let Err(e) = self.batch_tx.send(batch).await {
                                    warn!("Failed to send batch: {}", e);
                                    break;
                                }
                            }
                        }
                        None => {
                            info!("Input channel closed");
                            break;
                        }
                    }
                }

                _ = tokio::time::sleep(remaining_timeout), if !collector.is_empty() => {
                    if collector.should_flush() && !collector.is_empty() {
                        let batch = collector.flush();
                        total_batches += 1;
                        if let Err(e) = self.batch_tx.send(batch).await {
                            warn!("Failed to send batch: {}", e);
                            break;
                        }
                    }
                }
            }
        }

        if let Some(batch) = collector.flush_if_not_empty() {
            let batch_len = batch.len();
            total_batches += 1;
            if let Err(e) = self.batch_tx.send(batch).await {
                warn!("Failed to send final batch: {} ({} events)", e, batch_len);
            } else {
                debug!("Flushed final batch: {} events", batch_len);
            }
        }

        let mut drained = 0;
        let mut drain_batch = Vec::with_capacity(self.config.batch_size);

        while let Ok(event) = self.rx.try_recv() {
            drain_batch.push(event);
            drained += 1;

            if drain_batch.len() >= self.config.batch_size {
                total_batches += 1;
                let batch =
                    std::mem::replace(&mut drain_batch, Vec::with_capacity(self.config.batch_size));
                if let Err(e) = self.batch_tx.send(batch).await {
                    warn!("Failed to send drained batch: {}", e);
                    break;
                }
            }
        }

        if !drain_batch.is_empty() {
            total_batches += 1;
            if let Err(e) = self.batch_tx.send(drain_batch).await {
                warn!("Failed to send final drained batch: {}", e);
            }
        }

        info!(
            "Batch sender stopped: total_events={}, total_batches={}, drained={}",
            total_events + drained as u64,
            total_batches,
            drained
        );
    }
}

#[derive(Debug, Default, Clone)]
pub struct BatchSenderStats {
    pub events_received: u64,
    pub batches_sent: u64,
    pub events_in_buffer: usize,
    pub avg_batch_size: f64,
}

impl BatchSenderStats {
    pub fn new(events_received: u64, batches_sent: u64, events_in_buffer: usize) -> Self {
        let avg_batch_size = if batches_sent > 0 {
            events_received as f64 / batches_sent as f64
        } else {
            0.0
        };

        Self {
            events_received,
            batches_sent,
            events_in_buffer,
            avg_batch_size,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use proto_gen::FlowEvent;
    use tokio::sync::mpsc;
    use tokio::time::{timeout, Duration};

    #[test]
    fn collector_flushes_at_configured_batch_size() {
        let mut collector = BatchCollector::new(BatchConfig {
            batch_size: 2,
            batch_timeout: Duration::from_secs(60),
        });

        assert!(collector.push(flow_event("flow-1")).is_none());
        let batch = collector.push(flow_event("flow-2")).expect("batch should flush");

        assert_eq!(batch.len(), 2);
        assert!(collector.is_empty());
        assert_eq!(collector.remaining_capacity(), 2);
    }

    #[tokio::test]
    async fn sender_respects_output_backpressure_and_drains_on_close() {
        let (event_tx, event_rx) = mpsc::channel(8);
        let (batch_tx, mut batch_rx) = mpsc::channel(1);
        let sender = BatchSender::new(
            BatchConfig {
                batch_size: 2,
                batch_timeout: Duration::from_secs(60),
            },
            event_rx,
            batch_tx,
        );

        for idx in 0..5 {
            event_tx
                .send(flow_event(&format!("flow-{idx}")))
                .await
                .expect("input channel should accept test event");
        }
        drop(event_tx);

        let handle = tokio::spawn(sender.run());
        let mut batches = Vec::new();
        while batches.iter().map(Vec::len).sum::<usize>() < 5 {
            let batch = timeout(Duration::from_secs(1), batch_rx.recv())
                .await
                .expect("sender should not stall under downstream backpressure")
                .expect("batch channel should remain open until all data is drained");
            batches.push(batch);
        }

        handle.await.expect("BatchSender task should finish cleanly");

        assert_eq!(batches.iter().map(Vec::len).collect::<Vec<_>>(), vec![2, 2, 1]);
        assert_eq!(
            batches
                .into_iter()
                .flatten()
                .map(|event| event.flow_id)
                .collect::<Vec<_>>(),
            vec!["flow-0", "flow-1", "flow-2", "flow-3", "flow-4"]
        );
    }

    fn flow_event(flow_id: &str) -> FlowEvent {
        FlowEvent {
            flow_id: flow_id.to_string(),
            ..FlowEvent::default()
        }
    }
}
