use proto_gen::FlowEvent;
use tokio::sync::mpsc::{Sender, Receiver};
use tokio::time::{interval, Duration};

/// 批量收集器（辅助工具）
pub struct BatchCollector {
    input: Receiver<FlowEvent>,
    output: Sender<Vec<FlowEvent>>,
    batch_size: usize,
    batch_timeout: Duration,
}

impl BatchCollector {
    pub fn new(
        input: Receiver<FlowEvent>,
        output: Sender<Vec<FlowEvent>>,
        batch_size: usize,
        batch_timeout: Duration,
    ) -> Self {
        Self {
            input,
            output,
            batch_size,
            batch_timeout,
        }
    }

    pub async fn run(mut self) {
        let mut buffer = Vec::with_capacity(self.batch_size);
        let mut ticker = interval(self.batch_timeout);

        loop {
            tokio::select! {
                Some(event) = self.input.recv() => {
                    buffer.push(event);
                    
                    if buffer.len() >= self.batch_size {
                        self.flush(&mut buffer).await;
                    }
                }
                
                _ = ticker.tick() => {
                    if !buffer.is_empty() {
                        self.flush(&mut buffer).await;
                    }
                }
            }
        }
    }

    async fn flush(&self, buffer: &mut Vec<FlowEvent>) {
        let batch = std::mem::replace(buffer, Vec::with_capacity(self.batch_size));
        
        if self.output.send(batch).await.is_err() {
            tracing::error!("Failed to send batch (channel closed)");
        }
    }
}