use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{broadcast, mpsc, Mutex, Notify, RwLock};
use tokio::time::Instant;
use tracing::{debug, error, info, warn};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ComponentState {
    Running,
    ShuttingDown,
    Stopped,
    Error,
}

impl std::fmt::Display for ComponentState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ComponentState::Running => write!(f, "Running"),
            ComponentState::ShuttingDown => write!(f, "ShuttingDown"),
            ComponentState::Stopped => write!(f, "Stopped"),
            ComponentState::Error => write!(f, "Error"),
        }
    }
}

#[derive(Debug, Clone)]
struct ComponentInfo {
    name: &'static str,
    state: ComponentState,
    priority: u8,
    registered_at: Instant,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ShutdownPhase {
    NotStarted,
    StopInput,
    FlushBuffers,
    CloseConnections,
    Cleanup,
    Complete,
}

static GLOBAL_COMPLETION_SENDER: once_cell::sync::OnceCell<
    mpsc::UnboundedSender<(&'static str, Result<(), String>)>,
> = once_cell::sync::OnceCell::new();

fn init_global_completion_queue() -> mpsc::UnboundedSender<(&'static str, Result<(), String>)> {
    let (tx, mut rx) = mpsc::unbounded_channel();

    std::thread::spawn(move || {
        let rt = tokio::runtime::Runtime::new().expect("Failed to create completion runtime");

        rt.block_on(async move {
            while let Some((name, result)) = rx.recv().await {
                debug!(
                    "Global completion queue: component '{}' completed with {:?}",
                    name, result
                );
            }
        });
    });

    tx
}

fn get_global_completion_sender(
) -> &'static mpsc::UnboundedSender<(&'static str, Result<(), String>)> {
    GLOBAL_COMPLETION_SENDER.get_or_init(init_global_completion_queue)
}

pub struct ShutdownManager {
    shutdown_tx: broadcast::Sender<ShutdownPhase>,
    completion_tx: mpsc::Sender<(&'static str, Result<(), String>)>,
    completion_rx: Mutex<mpsc::Receiver<(&'static str, Result<(), String>)>>,
    components: Arc<RwLock<HashMap<&'static str, ComponentInfo>>>,
    force_shutdown: Notify,
    current_phase: RwLock<ShutdownPhase>,
    shutdown_start: Mutex<Option<Instant>>,
}

impl ShutdownManager {
    pub fn new() -> Arc<Self> {
        let (shutdown_tx, _) = broadcast::channel(16);
        let (completion_tx, completion_rx) = mpsc::channel(64);

        Arc::new(Self {
            shutdown_tx,
            completion_tx,
            completion_rx: Mutex::new(completion_rx),
            components: Arc::new(RwLock::new(HashMap::new())),
            force_shutdown: Notify::new(),
            current_phase: RwLock::new(ShutdownPhase::NotStarted),
            shutdown_start: Mutex::new(None),
        })
    }

    pub async fn register(self: &Arc<Self>, name: &'static str, priority: u8) -> ShutdownHandle {
        let mut components = self.components.write().await;

        components.insert(
            name,
            ComponentInfo {
                name,
                state: ComponentState::Running,
                priority,
                registered_at: Instant::now(),
            },
        );

        info!("Component '{}' registered with priority {}", name, priority);

        ShutdownHandle {
            name,
            manager: Arc::clone(self),
            shutdown_rx: self.shutdown_tx.subscribe(),
            notified: false,
        }
    }

    pub async fn component_count(&self) -> usize {
        self.components.read().await.len()
    }

    pub async fn current_phase(&self) -> ShutdownPhase {
        *self.current_phase.read().await
    }

    async fn set_phase(&self, phase: ShutdownPhase) {
        let mut current = self.current_phase.write().await;
        *current = phase;

        let _ = self.shutdown_tx.send(phase);

        info!("Shutdown phase: {:?}", phase);
    }

    pub async fn shutdown(self: Arc<Self>, grace_period: Duration) {
        {
            let phase = self.current_phase.read().await;
            if *phase != ShutdownPhase::NotStarted {
                warn!("Shutdown already in progress");
                return;
            }
        }

        info!("========================================");
        info!("  Initiating graceful shutdown");
        info!("  Grace period: {}s", grace_period.as_secs());
        info!("========================================");

        let start = Instant::now();
        {
            let mut shutdown_start = self.shutdown_start.lock().await;
            *shutdown_start = Some(start);
        }

        let sorted_components: Vec<(&'static str, u8)> = {
            let components = self.components.read().await;
            let mut list: Vec<_> = components.values().map(|c| (c.name, c.priority)).collect();
            list.sort_by_key(|(_, p)| *p);
            list
        };

        info!(
            "Shutdown order: {:?}",
            sorted_components
                .iter()
                .map(|(n, _)| *n)
                .collect::<Vec<_>>()
        );

        self.set_phase(ShutdownPhase::StopInput).await;
        tokio::time::sleep(Duration::from_millis(100)).await;

        self.set_phase(ShutdownPhase::FlushBuffers).await;
        tokio::time::sleep(Duration::from_millis(500)).await;

        self.set_phase(ShutdownPhase::CloseConnections).await;

        let result = self
            .wait_for_completion(grace_period, &sorted_components)
            .await;

        self.set_phase(ShutdownPhase::Cleanup).await;
        tokio::time::sleep(Duration::from_millis(100)).await;

        self.set_phase(ShutdownPhase::Complete).await;

        let elapsed = start.elapsed();

        match result {
            ShutdownResult::Complete { completed, total } => {
                info!("========================================");
                info!("  Graceful shutdown complete");
                info!("  Components: {}/{}", completed, total);
                info!("  Duration: {:.2}s", elapsed.as_secs_f64());
                info!("========================================");
            }
            ShutdownResult::Timeout { completed, pending } => {
                warn!("========================================");
                warn!("  Shutdown timeout!");
                warn!("  Completed: {}", completed);
                warn!("  Pending: {:?}", pending);
                warn!("  Duration: {:.2}s", elapsed.as_secs_f64());
                warn!("========================================");

                self.force_shutdown.notify_waiters();
            }
            ShutdownResult::Error(e) => {
                error!("Shutdown error: {}", e);
            }
        }
    }

    async fn wait_for_completion(
        &self,
        timeout_duration: Duration,
        sorted_components: &[(&'static str, u8)],
    ) -> ShutdownResult {
        let mut completed = std::collections::HashSet::new();
        let mut errors = Vec::new();

        let total = sorted_components.len();
        let deadline = Instant::now() + timeout_duration;

        loop {
            if completed.len() >= total {
                return ShutdownResult::Complete {
                    completed: completed.len(),
                    total,
                };
            }

            let remaining = deadline.saturating_duration_since(Instant::now());

            if remaining.is_zero() {
                let pending: Vec<&'static str> = sorted_components
                    .iter()
                    .filter(|(name, _)| !completed.contains(*name))
                    .map(|(name, _)| *name)
                    .collect();

                return ShutdownResult::Timeout {
                    completed: completed.len(),
                    pending,
                };
            }

            let recv_result = {
                let mut rx = match tokio::time::timeout(
                    Duration::from_millis(100),
                    self.completion_rx.lock(),
                )
                .await
                {
                    Ok(guard) => guard,
                    Err(_) => {
                        warn!("Completion lock timeout, retrying...");
                        continue;
                    }
                };

                tokio::time::timeout(remaining, rx.recv()).await
            };

            match recv_result {
                Ok(Some((name, result))) => match result {
                    Ok(()) => {
                        completed.insert(name);

                        let components = Arc::clone(&self.components);
                        tokio::spawn(async move {
                            let mut guard = components.write().await;
                            if let Some(info) = guard.get_mut(name) {
                                info.state = ComponentState::Stopped;
                            }
                        });

                        info!(
                            "✓ Component '{}' stopped ({}/{})",
                            name,
                            completed.len(),
                            total
                        );
                    }
                    Err(e) => {
                        errors.push((name, e.clone()));
                        completed.insert(name);

                        let components = Arc::clone(&self.components);
                        tokio::spawn(async move {
                            let mut guard = components.write().await;
                            if let Some(info) = guard.get_mut(name) {
                                info.state = ComponentState::Error;
                            }
                        });

                        warn!("✗ Component '{}' error: {}", name, e);
                    }
                },
                Ok(None) => {
                    return ShutdownResult::Error("Completion channel closed".to_string());
                }
                Err(_) => {
                    let pending: Vec<&'static str> = sorted_components
                        .iter()
                        .filter(|(name, _)| !completed.contains(*name))
                        .map(|(name, _)| *name)
                        .collect();

                    return ShutdownResult::Timeout {
                        completed: completed.len(),
                        pending,
                    };
                }
            }
        }
    }

    pub async fn component_states(&self) -> Vec<(&'static str, ComponentState, u8)> {
        let components = self.components.read().await;
        let mut list: Vec<_> = components
            .values()
            .map(|info| (info.name, info.state, info.priority))
            .collect();
        list.sort_by_key(|(_, _, p)| *p);
        list
    }

    pub async fn is_shutting_down(&self) -> bool {
        let phase = self.current_phase.read().await;
        *phase != ShutdownPhase::NotStarted
    }

    pub async fn wait_force_shutdown(&self) {
        self.force_shutdown.notified().await;
    }
}

impl Default for ShutdownManager {
    fn default() -> Self {
        Arc::try_unwrap(Self::new())
            .unwrap_or_else(|_| panic!("Cannot unwrap default ShutdownManager"))
    }
}

#[derive(Debug)]
enum ShutdownResult {
    Complete {
        completed: usize,
        total: usize,
    },
    Timeout {
        completed: usize,
        pending: Vec<&'static str>,
    },
    Error(String),
}

pub struct ShutdownHandle {
    name: &'static str,
    manager: Arc<ShutdownManager>,
    shutdown_rx: broadcast::Receiver<ShutdownPhase>,
    notified: bool,
}

impl ShutdownHandle {
    pub async fn wait(&mut self) -> ShutdownPhase {
        if self.notified {
            return ShutdownPhase::Complete;
        }

        loop {
            match self.shutdown_rx.recv().await {
                Ok(phase) => {
                    if phase != ShutdownPhase::NotStarted {
                        self.notified = true;
                        return phase;
                    }
                }
                Err(broadcast::error::RecvError::Closed) => {
                    self.notified = true;
                    return ShutdownPhase::Complete;
                }
                Err(broadcast::error::RecvError::Lagged(_)) => {
                    continue;
                }
            }
        }
    }

    pub fn is_shutdown_requested(&mut self) -> bool {
        if self.notified {
            return true;
        }

        match self.shutdown_rx.try_recv() {
            Ok(phase) if phase != ShutdownPhase::NotStarted => {
                self.notified = true;
                true
            }
            _ => false,
        }
    }

    pub async fn wait_phase(&mut self, target_phase: ShutdownPhase) -> bool {
        loop {
            match self.shutdown_rx.recv().await {
                Ok(phase) => {
                    if phase == target_phase {
                        return true;
                    }
                    if phase == ShutdownPhase::Complete {
                        return false;
                    }
                }
                Err(_) => return false,
            }
        }
    }

    pub async fn wait_force(&self) {
        self.manager.force_shutdown.notified().await;
    }

    pub fn name(&self) -> &'static str {
        self.name
    }

    pub async fn complete(self) {
        let _ = self.manager.completion_tx.send((self.name, Ok(()))).await;
        debug!("Component '{}' signaled completion", self.name);
    }

    pub async fn complete_with_error(self, error: String) {
        let _ = self
            .manager
            .completion_tx
            .send((self.name, Err(error)))
            .await;
    }
}

impl Drop for ShutdownHandle {
    fn drop(&mut self) {
        if !self.notified {
            let name = self.name;

            match self.manager.completion_tx.try_send((name, Ok(()))) {
                Ok(_) => {
                    debug!("Component '{}' auto-completed (try_send)", name);
                }
                Err(mpsc::error::TrySendError::Full((name, result))) => {
                    if let Err(e) = get_global_completion_sender().send((name, result)) {
                        warn!("Component '{}' completion notification lost: {}", name, e);
                    } else {
                        debug!("Component '{}' auto-completed (global queue)", name);
                    }
                }
                Err(mpsc::error::TrySendError::Closed(_)) => {
                    debug!(
                        "Completion channel closed, ignoring notification for '{}'",
                        name
                    );
                }
            }
        }
    }
}

pub struct ShutdownAwareRunner {
    handle: ShutdownHandle,
}

impl ShutdownAwareRunner {
    pub fn new(handle: ShutdownHandle) -> Self {
        Self { handle }
    }

    pub async fn run_until_shutdown<F, Fut>(mut self, task: F)
    where
        F: FnOnce() -> Fut,
        Fut: std::future::Future<Output = ()>,
    {
        tokio::select! {
            _ = task() => {
                debug!("Task '{}' completed normally", self.handle.name);
            }
            phase = self.handle.wait() => {
                debug!("Task '{}' received shutdown signal: {:?}", self.handle.name, phase);
            }
        }

        self.handle.complete().await;
    }

    pub async fn run_with_cleanup<F, Fut, C, CFut>(mut self, task: F, cleanup: C)
    where
        F: FnOnce() -> Fut,
        Fut: std::future::Future<Output = ()>,
        C: FnOnce() -> CFut,
        CFut: std::future::Future<Output = ()>,
    {
        tokio::select! {
            _ = task() => {
                debug!("Task '{}' completed normally", self.handle.name);
            }
            phase = self.handle.wait() => {
                debug!("Task '{}' received shutdown signal: {:?}", self.handle.name, phase);
            }
        }

        debug!("Task '{}' running cleanup", self.handle.name);
        cleanup().await;

        self.handle.complete().await;
    }
}

#[macro_export]
macro_rules! spawn_component {
    ($manager:expr, $name:literal, $priority:expr, $task:expr) => {{
        let manager = $manager.clone();
        tokio::spawn(async move {
            let handle = manager.register($name, $priority).await;
            let runner = $crate::shutdown::ShutdownAwareRunner::new(handle);
            runner.run_until_shutdown(|| async { $task }).await;
        })
    }};

    ($manager:expr, $name:literal, $priority:expr, $task:expr, cleanup: $cleanup:expr) => {{
        let manager = $manager.clone();
        tokio::spawn(async move {
            let handle = manager.register($name, $priority).await;
            let runner = $crate::shutdown::ShutdownAwareRunner::new(handle);
            runner
                .run_with_cleanup(|| async { $task }, || async { $cleanup })
                .await;
        })
    }};
}
