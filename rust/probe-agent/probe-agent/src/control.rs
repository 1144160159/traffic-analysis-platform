use anyhow::{bail, Context, Result};
use async_trait::async_trait;
use prost::Message;
use proto_gen::{ProbeOperationAck, ProbeOperationCommand};
use serde_json::Value;
use sha2::{Digest, Sha256};
use sled::{Batch, Db};
use std::collections::{BTreeMap, BTreeSet};
use std::path::Path;
use std::sync::Arc;
use uuid::Uuid;

const ACK_PREFIX: &[u8] = b"ack:";
const REVISION_PREFIX: &[u8] = b"revision:";

#[derive(Debug, Clone)]
pub struct OperationExecution {
    pub applied: bool,
    pub reported_version: String,
    pub detail: Value,
    pub error: String,
}

#[async_trait]
pub trait ProbeOperationExecutor: Send + Sync {
    async fn execute(&self, operation_type: &str, command: &Value) -> Result<OperationExecution>;
}

/// Production-safe default: a transport may be enabled before individual
/// privileged operations have an approved executor, but it may never turn
/// receipt of a command into a fabricated success.
pub struct FailClosedExecutor;

#[async_trait]
impl ProbeOperationExecutor for FailClosedExecutor {
    async fn execute(&self, operation_type: &str, _command: &Value) -> Result<OperationExecution> {
        Ok(OperationExecution {
            applied: false,
            reported_version: String::new(),
            detail: serde_json::json!({"operation_type": operation_type}),
            error: format!("operation executor is not enabled for {operation_type}"),
        })
    }
}

pub struct BuiltinProbeExecutor {
    connectivity_targets: BTreeMap<String, String>,
    connect_timeout: std::time::Duration,
    replay: Option<Arc<dyn ReplayOperationExecutor>>,
    capture_interface: Option<String>,
}

impl BuiltinProbeExecutor {
    pub fn for_gateway(gateway_addr: &str) -> Result<Self> {
        let endpoint = tcp_endpoint(gateway_addr)?;
        Ok(Self {
            connectivity_targets: BTreeMap::from([("ingest-gateway".to_string(), endpoint)]),
            connect_timeout: std::time::Duration::from_secs(5),
            replay: None,
            capture_interface: None,
        })
    }

    /// 挂载回放执行器(pcap_replay 操作;未挂载时该操作 FailClosed,不伪造成功)。
    pub fn with_replay(mut self, replay: Arc<dyn ReplayOperationExecutor>) -> Self {
        self.replay = Some(replay);
        self
    }

    /// 挂载实时采集接口(capture_window 操作;未挂载时该操作 FailClosed)。
    pub fn with_capture_interface(mut self, interface: String) -> Self {
        self.capture_interface = Some(interface);
        self
    }

    #[cfg(test)]
    fn with_targets(connectivity_targets: BTreeMap<String, String>) -> Self {
        Self {
            connectivity_targets,
            connect_timeout: std::time::Duration::from_secs(1),
            replay: None,
            capture_interface: None,
        }
    }

    async fn connectivity_test(&self, command: &Value) -> Result<OperationExecution> {
        let targets = command
            .get("targets")
            .and_then(Value::as_array)
            .context("connectivity_test targets must be an array")?;
        if targets.is_empty() {
            bail!("connectivity_test targets cannot be empty");
        }
        let mut applied = true;
        let mut results = serde_json::Map::new();
        for target in targets {
            let alias = target
                .as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .context("connectivity target must be a non-empty string")?;
            let Some(endpoint) = self.connectivity_targets.get(alias) else {
                applied = false;
                results.insert(
                    alias.to_string(),
                    serde_json::json!({"reachable": false, "error": "target is not in the Agent allowlist"}),
                );
                continue;
            };
            let started = std::time::Instant::now();
            let result = tokio::time::timeout(
                self.connect_timeout,
                tokio::net::TcpStream::connect(endpoint),
            )
            .await;
            let (reachable, error) = match result {
                Ok(Ok(stream)) => {
                    drop(stream);
                    (true, String::new())
                }
                Ok(Err(error)) => (false, error.to_string()),
                Err(_) => (false, "connection timed out".to_string()),
            };
            applied &= reachable;
            results.insert(
                alias.to_string(),
                serde_json::json!({
                    "reachable": reachable,
                    "latency_ms": started.elapsed().as_millis(),
                    "error": error
                }),
            );
        }
        Ok(OperationExecution {
            applied,
            reported_version: "connectivity-v1".to_string(),
            detail: Value::Object(results),
            error: if applied {
                String::new()
            } else {
                "one or more approved connectivity targets failed".to_string()
            },
        })
    }
}

/// ReplayOperationExecutor —— 探针侧回放执行端口(main 以共享 flow_table 实现,
/// 复用既有聚合器单一真源;调度中心经探针控制通道投递 ReplayWindowCommand)。
#[async_trait]
pub trait ReplayOperationExecutor: Send + Sync {
    async fn execute_replay(&self, cmd: &ReplayWindowCommand) -> Result<ReplayExecution>;
}

/// 回放执行结果(applied=true 表示对象校验+有界回放+共享分支喂入全程成功)。
#[derive(Debug, Clone, serde::Serialize)]
pub struct ReplayExecution {
    pub applied: bool,
    pub packets: u64,
    pub bytes_consumed: u64,
    pub watermark_ms: i64,
    pub detail: String,
}

#[async_trait]
impl ProbeOperationExecutor for BuiltinProbeExecutor {
    async fn execute(&self, operation_type: &str, command: &Value) -> Result<OperationExecution> {
        match operation_type {
            "connectivity_test" => self.connectivity_test(command).await,
            "pcap_replay" => self.pcap_replay(command).await,
            "capture_window" => self.capture_window(command).await,
            _ => FailClosedExecutor.execute(operation_type, command).await,
        }
    }
}

impl BuiltinProbeExecutor {
    /// capture_window —— 有界实时采集窗口(FP-206):
    /// 命令=窗口覆盖确认:校验命令(签名/身份/窗口/限额/配额/interface)→ 确认
    /// 该窗口落在探针常驻实时采集(ambient capture)覆盖范围内 → applied=true
    /// ACK 携带窗口坐标;窗口内流量经既有流聚合→eviction→sender 链入共享分支,
    /// 由 RunScopeRouter 按订阅窗口归属到 run。interface 不匹配或窗口已过去
    /// 则 applied=false(fail-closed,不伪造覆盖)。
    async fn capture_window(&self, command: &Value) -> Result<OperationExecution> {
        let Some(expected_interface) = self.capture_interface.as_ref() else {
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"operation_type": "capture_window"}),
                error: "capture_window executor is not enabled on this probe".to_string(),
            });
        };
        let cmd: CaptureWindowCommand = match serde_json::from_value(command.clone()) {
            Ok(cmd) => cmd,
            Err(e) => {
                return Ok(OperationExecution {
                    applied: false,
                    reported_version: String::new(),
                    detail: serde_json::json!({"operation_type": "capture_window"}),
                    error: format!("capture window command malformed: {e}"),
                })
            }
        };
        if let Err(e) = cmd.validate() {
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"validation": format!("{e}")}),
                error: format!("capture window validation failed: {e}"),
            });
        }
        if &cmd.interface != expected_interface {
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"interface_requested": cmd.interface}),
                error: format!(
                    "requested interface {} is not the monitored interface {}",
                    cmd.interface, expected_interface
                ),
            });
        }
        let now_ms = chrono::Utc::now().timestamp_millis();
        if cmd.window_end_ms <= now_ms {
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"window_end_ms": cmd.window_end_ms}),
                error: "capture window already elapsed before operation execution".to_string(),
            });
        }
        Ok(OperationExecution {
            applied: true,
            reported_version: cmd.execution_spec_sha256.clone(),
            detail: serde_json::json!({
                "run_id": cmd.run_id,
                "fencing_token": cmd.fencing_token,
                "interface": cmd.interface,
                "window_start_ms": cmd.window_start_ms,
                "window_end_ms": cmd.window_end_ms,
                "packets": 0,
                "bytes_consumed": 0,
                "watermark_ms": cmd.window_start_ms,
                "detail": "window_acknowledged"
            }),
            error: String::new(),
        })
    }

    async fn pcap_replay(&self, command: &Value) -> Result<OperationExecution> {
        let Some(replay) = self.replay.as_ref() else {
            // 诚实 FailClosed:操作已允许但执行器未挂载,applied=false
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"operation_type": "pcap_replay"}),
                error: "pcap_replay executor is not enabled on this probe".to_string(),
            });
        };
        let cmd: ReplayWindowCommand = match serde_json::from_value(command.clone()) {
            Ok(cmd) => cmd,
            Err(e) => {
                return Ok(OperationExecution {
                    applied: false,
                    reported_version: String::new(),
                    detail: serde_json::json!({"operation_type": "pcap_replay"}),
                    error: format!("replay command malformed: {e}"),
                })
            }
        };
        if let Err(e) = cmd.validate() {
            return Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"operation_type": "pcap_replay", "command": cmd}),
                error: e.to_string(),
            });
        }
        match replay.execute_replay(&cmd).await {
            Ok(ex) => Ok(OperationExecution {
                applied: ex.applied,
                reported_version: cmd.execution_spec_sha256.clone(),
                detail: serde_json::json!({
                    "packets": ex.packets,
                    "bytes_consumed": ex.bytes_consumed,
                    "watermark_ms": ex.watermark_ms,
                    "detail": ex.detail,
                    "run_id": cmd.run_id,
                    "execution_spec_sha256": cmd.execution_spec_sha256,
                    "fencing_token": cmd.fencing_token,
                }),
                error: if ex.applied { String::new() } else { ex.detail.clone() },
            }),
            Err(e) => Ok(OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"operation_type": "pcap_replay"}),
                error: format!("replay execution failed: {e}"),
            }),
        }
    }
}

#[derive(Clone)]
pub struct ProbeControlProcessor {
    tenant_id: String,
    probe_id: String,
    db: Arc<Db>,
    executor: Arc<dyn ProbeOperationExecutor>,
    allowed_operations: Arc<BTreeSet<String>>,
}

impl ProbeControlProcessor {
    pub fn open(
        path: &Path,
        tenant_id: impl Into<String>,
        probe_id: impl Into<String>,
        executor: Arc<dyn ProbeOperationExecutor>,
    ) -> Result<Self> {
        let tenant_id = tenant_id.into();
        let probe_id = probe_id.into();
        if tenant_id.trim().is_empty() || probe_id.trim().is_empty() {
            bail!("tenant_id and probe_id are required for probe control");
        }
        let db = sled::Config::new()
            .path(path.join("probe_control"))
            .mode(sled::Mode::HighThroughput)
            .flush_every_ms(Some(100))
            .open()
            .context("failed to open probe control journal")?;
        let allowed_operations = [
            "config_push",
            "connectivity_test",
            "cert_rotate",
            "batch_upgrade",
            "batch_state",
            "restart",
            "capture_window",
            "pcap_replay",
        ]
        .into_iter()
        .map(str::to_string)
        .collect();
        Ok(Self {
            tenant_id,
            probe_id,
            db: Arc::new(db),
            executor,
            allowed_operations: Arc::new(allowed_operations),
        })
    }

    pub async fn process(&self, command: ProbeOperationCommand) -> Result<ProbeOperationAck> {
        self.validate_envelope(&command)?;

        if let Some(existing) = self.load_ack(&command.operation_id)? {
            return Ok(existing);
        }

        let now_ms = chrono::Utc::now().timestamp_millis();
        // A malformed JSON payload must still produce a persisted failed
        // ACK; otherwise the gateway cannot confirm delivery and re-sends
        // the same command on every heartbeat forever.
        let execution = match serde_json::from_slice(&command.command_json) {
            Err(parse_error) => OperationExecution {
                applied: false,
                reported_version: String::new(),
                detail: serde_json::json!({"validation": "invalid_json_payload"}),
                error: truncate_error(&format!(
                    "command_json is not valid JSON: {parse_error}"
                )),
            },
            Ok(command_value) => {
                if command.expires_at_ms <= now_ms {
                    OperationExecution {
                        applied: false,
                        reported_version: String::new(),
                        detail: serde_json::json!({
                            "expired_at_ms": command.expires_at_ms,
                            "received_at_ms": now_ms
                        }),
                        error: "operation expired before Agent execution".to_string(),
                    }
                } else if deterministic_command_hash(&command_value) != command.command_hash {
                    OperationExecution {
                        applied: false,
                        reported_version: String::new(),
                        detail: serde_json::json!({"validation": "command_hash_mismatch"}),
                        error: "deterministic command hash mismatch".to_string(),
                    }
                } else if command.command_revision <= self.highest_revision()? {
                    OperationExecution {
                        applied: false,
                        reported_version: String::new(),
                        detail: serde_json::json!({"validation": "stale_command_revision"}),
                        error: "command revision is not newer than the persisted Agent revision"
                            .to_string(),
                    }
                } else {
                    match self
                        .executor
                        .execute(&command.operation_type, &command_value)
                        .await
                    {
                        Ok(result) => result,
                        Err(err) => OperationExecution {
                            applied: false,
                            reported_version: String::new(),
                            detail: serde_json::json!({"executor_error": err.to_string()}),
                            error: truncate_error(&err.to_string()),
                        },
                    }
                }
            }
        };

        let detail_json = canonical_json(&execution.detail).into_bytes();
        let ack = ProbeOperationAck {
            operation_id: command.operation_id.clone(),
            command_revision: command.command_revision,
            reported_version: execution.reported_version,
            reported_hash: sha256_hex(&detail_json),
            agent_version: env!("CARGO_PKG_VERSION").to_string(),
            applied: execution.applied,
            error: truncate_error(&execution.error),
            acknowledged_at_ms: chrono::Utc::now().timestamp_millis(),
            detail_json,
        };
        self.persist_ack(&ack, execution.applied)?;
        Ok(ack)
    }

    pub fn pending_acks(&self, limit: usize) -> Result<Vec<ProbeOperationAck>> {
        let mut result = Vec::new();
        for item in self.db.scan_prefix(ACK_PREFIX).take(limit.max(1)) {
            let (_, bytes) = item?;
            result.push(
                ProbeOperationAck::decode(bytes.as_ref())
                    .context("failed to decode persisted probe operation ACK")?,
            );
        }
        result.sort_by_key(|ack| (ack.command_revision, ack.operation_id.clone()));
        Ok(result)
    }

    pub fn acknowledge_accepted(&self, operation_ids: &[String]) -> Result<usize> {
        let mut batch = Batch::default();
        let mut count = 0;
        for operation_id in operation_ids {
            if Uuid::parse_str(operation_id).is_err() {
                continue;
            }
            let key = ack_key(operation_id);
            if self.db.contains_key(&key)? {
                batch.remove(key);
                count += 1;
            }
        }
        if count > 0 {
            self.db.apply_batch(batch)?;
            self.db.flush()?;
        }
        Ok(count)
    }

    fn validate_envelope(&self, command: &ProbeOperationCommand) -> Result<()> {
        if command.tenant_id != self.tenant_id || command.probe_id != self.probe_id {
            bail!("command identity does not match this Agent");
        }
        Uuid::parse_str(&command.event_id).context("invalid command event_id")?;
        Uuid::parse_str(&command.operation_id).context("invalid command operation_id")?;
        if command.command_revision <= 0 {
            bail!("command_revision must be positive");
        }
        if !self.allowed_operations.contains(&command.operation_type) {
            bail!("operation_type is not in the Agent allowlist");
        }
        if command.command_hash.len() != 64
            || !command
                .command_hash
                .bytes()
                .all(|byte| byte.is_ascii_hexdigit())
        {
            bail!("command_hash must be a SHA-256 hex digest");
        }
        if command.command_json.is_empty() {
            bail!("command_json is required");
        }
        Ok(())
    }

    fn load_ack(&self, operation_id: &str) -> Result<Option<ProbeOperationAck>> {
        self.db
            .get(ack_key(operation_id))?
            .map(|bytes| {
                ProbeOperationAck::decode(bytes.as_ref())
                    .context("failed to decode persisted probe operation ACK")
            })
            .transpose()
    }

    fn highest_revision(&self) -> Result<i64> {
        let key = revision_key(&self.tenant_id, &self.probe_id);
        match self.db.get(key)? {
            Some(bytes) => {
                let raw: [u8; 8] = bytes
                    .as_ref()
                    .try_into()
                    .context("invalid persisted probe command revision")?;
                Ok(i64::from_be_bytes(raw))
            }
            None => Ok(0),
        }
    }

    fn persist_ack(&self, ack: &ProbeOperationAck, advance_revision: bool) -> Result<()> {
        let mut encoded = Vec::with_capacity(ack.encoded_len());
        ack.encode(&mut encoded)?;
        let mut batch = Batch::default();
        batch.insert(ack_key(&ack.operation_id), encoded);
        if advance_revision {
            batch.insert(
                revision_key(&self.tenant_id, &self.probe_id),
                ack.command_revision.to_be_bytes().to_vec(),
            );
        }
        self.db.apply_batch(batch)?;
        self.db.flush()?;
        Ok(())
    }
}

pub fn deterministic_command_hash(value: &Value) -> String {
    sha256_hex(canonical_json(value).as_bytes())
}

fn canonical_json(value: &Value) -> String {
    match value {
        Value::Null => "null".to_string(),
        Value::Bool(value) => value.to_string(),
        Value::Number(value) => value.to_string(),
        Value::String(value) => serde_json::to_string(value)
            .unwrap_or_else(|_| value.clone()), // string serialization cannot fail
        Value::Array(values) => format!(
            "[{}]",
            values
                .iter()
                .map(canonical_json)
                .collect::<Vec<_>>()
                .join(",")
        ),
        Value::Object(values) => {
            let mut entries: Vec<_> = values.iter().collect();
            entries.sort_by(|left, right| left.0.cmp(right.0));
            format!(
                "{{{}}}",
                entries
                    .into_iter()
                    .map(|(key, value)| format!(
                        "{}:{}",
                        serde_json::to_string(key).expect("key serialization"),
                        canonical_json(value)
                    ))
                    .collect::<Vec<_>>()
                    .join(",")
            )
        }
    }
}

fn sha256_hex(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}

fn truncate_error(value: &str) -> String {
    let value = value.trim();
    if value.chars().count() <= 1000 {
        return value.to_string();
    }
    value.chars().take(1000).collect()
}

fn ack_key(operation_id: &str) -> Vec<u8> {
    [ACK_PREFIX, operation_id.as_bytes()].concat()
}

fn revision_key(tenant_id: &str, probe_id: &str) -> Vec<u8> {
    [
        REVISION_PREFIX,
        tenant_id.as_bytes(),
        b":",
        probe_id.as_bytes(),
    ]
    .concat()
}

fn tcp_endpoint(value: &str) -> Result<String> {
    let value = value.trim();
    let (without_scheme, default_port) = if let Some(rest) = value.strip_prefix("https://") {
        (rest, 443)
    } else if let Some(rest) = value.strip_prefix("http://") {
        (rest, 80)
    } else {
        (value, 443)
    };
    let authority = without_scheme.split('/').next().unwrap_or_default().trim();
    if authority.is_empty() || authority.contains('@') {
        bail!("gateway_addr does not contain a safe TCP authority");
    }
    if authority.starts_with('[') {
        if authority.contains("]:") {
            return Ok(authority.to_string());
        }
        return Ok(format!("{authority}:{default_port}"));
    }
    if authority.matches(':').count() == 1 {
        Ok(authority.to_string())
    } else if authority.contains(':') {
        bail!("IPv6 gateway_addr must use bracket notation");
    } else {
        Ok(format!("{authority}:{default_port}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tempfile::TempDir;

    struct SuccessfulExecutor {
        calls: AtomicUsize,
    }

    #[async_trait]
    impl ProbeOperationExecutor for SuccessfulExecutor {
        async fn execute(
            &self,
            _operation_type: &str,
            command: &Value,
        ) -> Result<OperationExecution> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            Ok(OperationExecution {
                applied: true,
                reported_version: command["config_version"]
                    .as_str()
                    .unwrap_or_default()
                    .to_string(),
                detail: serde_json::json!({"applied": true}),
                error: String::new(),
            })
        }
    }

    fn command(value: Value, revision: i64) -> ProbeOperationCommand {
        ProbeOperationCommand {
            event_id: Uuid::new_v4().to_string(),
            tenant_id: "tenant-a".to_string(),
            probe_id: "probe-a".to_string(),
            operation_id: Uuid::new_v4().to_string(),
            operation_type: "config_push".to_string(),
            command_revision: revision,
            desired_version: "cfg-2".to_string(),
            command_hash: deterministic_command_hash(&value),
            expires_at_ms: chrono::Utc::now().timestamp_millis() + 60_000,
            trace_id: "trace-a".to_string(),
            command_json: canonical_json(&value).into_bytes(),
        }
    }

    fn processor(
        temp: &TempDir,
        executor: Arc<dyn ProbeOperationExecutor>,
    ) -> ProbeControlProcessor {
        ProbeControlProcessor::open(temp.path(), "tenant-a", "probe-a", executor).unwrap()
    }

    #[tokio::test]
    async fn persists_success_before_return_and_deduplicates_redelivery() {
        let temp = TempDir::new().unwrap();
        let executor = Arc::new(SuccessfulExecutor {
            calls: AtomicUsize::new(0),
        });
        let processor = processor(&temp, executor.clone());
        let command = command(serde_json::json!({"config_version": "cfg-2"}), 2);

        let first = processor.process(command.clone()).await.unwrap();
        let second = processor.process(command).await.unwrap();

        assert!(first.applied);
        assert_eq!(first, second);
        assert_eq!(executor.calls.load(Ordering::SeqCst), 1);
        assert_eq!(processor.pending_acks(10).unwrap(), vec![first.clone()]);
        assert_eq!(
            processor
                .acknowledge_accepted(&[first.operation_id.clone()])
                .unwrap(),
            1
        );
        assert!(processor.pending_acks(10).unwrap().is_empty());
    }

    #[tokio::test]
    async fn rejects_cross_identity_without_persisting_ack() {
        let temp = TempDir::new().unwrap();
        let executor = Arc::new(SuccessfulExecutor {
            calls: AtomicUsize::new(0),
        });
        let processor = processor(&temp, executor.clone());
        let mut command = command(serde_json::json!({"config_version": "cfg-2"}), 2);
        command.probe_id = "probe-b".to_string();

        assert!(processor.process(command).await.is_err());
        assert_eq!(executor.calls.load(Ordering::SeqCst), 0);
        assert!(processor.pending_acks(10).unwrap().is_empty());
    }

    #[tokio::test]
    async fn hash_mismatch_is_a_persisted_terminal_failure() {
        let temp = TempDir::new().unwrap();
        let executor = Arc::new(SuccessfulExecutor {
            calls: AtomicUsize::new(0),
        });
        let processor = processor(&temp, executor.clone());
        let mut command = command(serde_json::json!({"config_version": "cfg-2"}), 2);
        command.command_json = br#"{"config_version":"tampered"}"#.to_vec();

        let ack = processor.process(command).await.unwrap();

        assert!(!ack.applied);
        assert!(ack.error.contains("hash mismatch"));
        assert_eq!(executor.calls.load(Ordering::SeqCst), 0);
        assert_eq!(processor.pending_acks(10).unwrap(), vec![ack]);
    }

    #[tokio::test]
    async fn expired_command_never_reaches_executor() {
        let temp = TempDir::new().unwrap();
        let executor = Arc::new(SuccessfulExecutor {
            calls: AtomicUsize::new(0),
        });
        let processor = processor(&temp, executor.clone());
        let mut command = command(serde_json::json!({"config_version": "cfg-2"}), 2);
        command.expires_at_ms = chrono::Utc::now().timestamp_millis() - 1;

        let ack = processor.process(command).await.unwrap();

        assert!(!ack.applied);
        assert!(ack.error.contains("expired"));
        assert_eq!(executor.calls.load(Ordering::SeqCst), 0);
    }

    #[tokio::test]
    async fn builtin_connectivity_executor_checks_only_allowlisted_target() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let endpoint = listener.local_addr().unwrap().to_string();
        let executor = BuiltinProbeExecutor::with_targets(BTreeMap::from([(
            "ingest-gateway".to_string(),
            endpoint,
        )]));
        let result = executor
            .execute(
                "connectivity_test",
                &serde_json::json!({"targets": ["ingest-gateway"]}),
            )
            .await
            .unwrap();
        assert!(result.applied);
        assert_eq!(result.detail["ingest-gateway"]["reachable"], true);
    }

    #[tokio::test]
    async fn builtin_connectivity_executor_rejects_unapproved_destination() {
        let executor = BuiltinProbeExecutor::with_targets(BTreeMap::new());
        let result = executor
            .execute(
                "connectivity_test",
                &serde_json::json!({"targets": ["169.254.169.254:80"]}),
            )
            .await
            .unwrap();
        assert!(!result.applied);
        assert_eq!(
            result.detail["169.254.169.254:80"]["error"],
            "target is not in the Agent allowlist"
        );
    }

    #[test]
    fn canonical_hash_is_object_order_independent() {
        let left: Value = serde_json::from_str(r#"{"b":[2,1],"a":{"z":true,"x":"v"}}"#).unwrap();
        let right: Value = serde_json::from_str(r#"{"a":{"x":"v","z":true},"b":[2,1]}"#).unwrap();
        assert_eq!(
            deterministic_command_hash(&left),
            deterministic_command_hash(&right)
        );
    }
}

// =============================================================================
// CaptureWindowExecutor —— 有界采集窗口执行器(ATC-SRC / FP-206)
//
// 约束(76.12.2):validate 无副作用;start 先持久化 command/lease 再创建
// run-scoped spool;达到任一上限即 stop;完成后 fsync/hash 并发布 StageReceipt。
// 本核心卷落地 validate 判定核(纯校验);start/stop 与 capture 数据面接线
// 由后续 P3 集成轮次完成(TODO:capture_window_start)。
// =============================================================================

use serde::{Deserialize, Serialize};

/// 有界采集窗口命令(领域命令,typed,不含自由字符串脚本入口)。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CaptureWindowCommand {
    pub tenant_id: String,
    pub task_id: String,
    pub run_id: String,
    pub execution_spec_sha256: String,
    pub probe_id: String,
    pub interface: String,
    pub bpf_filter: Option<String>,
    pub window_start_ms: i64,
    pub window_end_ms: i64,
    pub packet_limit: u64,
    pub byte_limit: u64,
    pub spool_quota_bytes: u64,
    pub lease_epoch: u64,
    pub fencing_token: String,
}

/// 校验通过的有界窗口(类型化,防未校验直用)。
#[derive(Debug, Clone)]
pub struct ValidatedCaptureWindow {
    pub command: CaptureWindowCommand,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum CaptureWindowError {
    #[error("tenant_id is empty")]
    MissingTenant,
    #[error("run identity is incomplete")]
    IncompleteRunIdentity,
    #[error("execution_spec_sha256 is empty")]
    MissingExecutionSpec,
    #[error("window_end must be after window_start")]
    InvalidWindow,
    #[error("packet_limit and byte_limit must not both be zero (bounded capture)")]
    UnboundedCapture,
    #[error("spool_quota_bytes exceeds hard limit")]
    SpoolQuotaExceeded,
    #[error("fencing_token is empty")]
    MissingFencingToken,
    #[error("interface is empty")]
    MissingInterface,
    #[error("bpf_filter exceeds max length")]
    BpfTooComplex,
}

/// BPF 复杂度上限(核心卷:长度上限;AST 复杂度分析接后续安全轮次)。
const MAX_BPF_LEN: usize = 512;
/// spool 配额硬上限(运行配置可低于,不得高于)。
const MAX_SPOOL_QUOTA: u64 = 64 * 1024 * 1024 * 1024; // 64 GiB

impl CaptureWindowCommand {
    /// validate —— 无副作用校验(签名/租户/身份/范围/限额/配额/lease/fencing)。
    pub fn validate(&self) -> Result<ValidatedCaptureWindow, CaptureWindowError> {
        if self.tenant_id.trim().is_empty() {
            return Err(CaptureWindowError::MissingTenant);
        }
        if self.task_id.trim().is_empty()
            || self.run_id.trim().is_empty()
            || self.probe_id.trim().is_empty()
        {
            return Err(CaptureWindowError::IncompleteRunIdentity);
        }
        if self.execution_spec_sha256.trim().is_empty() {
            return Err(CaptureWindowError::MissingExecutionSpec);
        }
        if self.interface.trim().is_empty() {
            return Err(CaptureWindowError::MissingInterface);
        }
        if self.window_end_ms <= self.window_start_ms {
            return Err(CaptureWindowError::InvalidWindow);
        }
        if self.packet_limit == 0 && self.byte_limit == 0 {
            return Err(CaptureWindowError::UnboundedCapture);
        }
        if self.spool_quota_bytes > MAX_SPOOL_QUOTA {
            return Err(CaptureWindowError::SpoolQuotaExceeded);
        }
        if self.fencing_token.trim().is_empty() {
            return Err(CaptureWindowError::MissingFencingToken);
        }
        if let Some(bpf) = &self.bpf_filter {
            if bpf.len() > MAX_BPF_LEN {
                return Err(CaptureWindowError::BpfTooComplex);
            }
        }
        Ok(ValidatedCaptureWindow {
            command: self.clone(),
        })
    }
}

#[cfg(test)]
mod capture_window_tests {
    use super::*;

    fn valid_command() -> CaptureWindowCommand {
        CaptureWindowCommand {
            tenant_id: "tenant-a".into(),
            task_id: "task-1".into(),
            run_id: "run-1".into(),
            execution_spec_sha256: "spec-1".into(),
            probe_id: "probe-a".into(),
            interface: "eth0".into(),
            bpf_filter: None,
            window_start_ms: 0,
            window_end_ms: 60_000,
            packet_limit: 10_000,
            byte_limit: 0,
            spool_quota_bytes: 1024 * 1024 * 1024,
            lease_epoch: 1,
            fencing_token: "fence-1".into(),
        }
    }

    #[test]
    fn validate_accepts_valid_command() {
        assert!(valid_command().validate().is_ok());
    }

    #[test]
    fn validate_rejects_missing_tenant() {
        let mut c = valid_command();
        c.tenant_id = " ".into();
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::MissingTenant);
    }

    #[test]
    fn validate_rejects_unbounded() {
        let mut c = valid_command();
        c.packet_limit = 0;
        c.byte_limit = 0;
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::UnboundedCapture);
    }

    #[test]
    fn validate_rejects_invalid_window() {
        let mut c = valid_command();
        c.window_start_ms = 60_000;
        c.window_end_ms = 60_000;
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::InvalidWindow);
    }

    #[test]
    fn validate_rejects_oversized_spool() {
        let mut c = valid_command();
        c.spool_quota_bytes = MAX_SPOOL_QUOTA + 1;
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::SpoolQuotaExceeded);
    }

    #[test]
    fn validate_rejects_oversized_bpf() {
        let mut c = valid_command();
        c.bpf_filter = Some("x".repeat(MAX_BPF_LEN + 1));
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::BpfTooComplex);
    }

    #[test]
    fn validate_rejects_missing_fencing() {
        let mut c = valid_command();
        c.fencing_token = "".into();
        assert_eq!(c.validate().unwrap_err(), CaptureWindowError::MissingFencingToken);
    }
}

// =============================================================================
// ReplayWindowCommand —— 人工执行链:analysis-service 经 probe.control.v2 投递的
// 有界对象回放命令(回放发生在所选探针位置;与 CaptureWindowCommand 对称)。
// =============================================================================

/// 有界对象回放命令(typed,不含自由字符串脚本入口)。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReplayWindowCommand {
    pub tenant_id: String,
    pub task_id: String,
    pub run_id: String,
    pub execution_spec_sha256: String,
    pub probe_id: String,
    pub object_ref: String,    // s3://bucket/key
    pub object_sha256: String, // 64 hex
    /// 测试阶段 wire 回放注入目标(虚拟网卡输入端);None = 进程内共享分支喂入
    /// (生产语义)。Some(iface) = 仅经 AF_PACKET 向 iface 注入真实流量,
    /// 供输出端探针实时采集;接口须在探针配置 allowlist 内,否则 fail-closed。
    #[serde(default)]
    pub interface: Option<String>,
    pub window_start_ms: i64,
    pub window_end_ms: i64,
    pub packet_limit: u64,
    pub byte_limit: u64,
    pub fencing_token: String,
}

/// 校验通过的有界回放窗口(类型化,防未校验直用)。
#[derive(Debug, Clone)]
pub struct ValidatedReplayWindow {
    pub command: ReplayWindowCommand,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ReplayWindowError {
    #[error("tenant_id is empty")]
    MissingTenant,
    #[error("run identity is incomplete")]
    IncompleteRunIdentity,
    #[error("execution_spec_sha256 is empty")]
    MissingExecutionSpec,
    #[error("object_ref must be s3://bucket/key")]
    InvalidObjectRef,
    #[error("object_sha256 must be 64 hex chars")]
    InvalidObjectSha256,
    #[error("window_end must be after window_start")]
    InvalidWindow,
    #[error("packet_limit and byte_limit must not both be zero (bounded replay)")]
    UnboundedReplay,
    #[error("wire replay interface must not be empty when present")]
    InvalidWireInterface,
    #[error("fencing_token is empty")]
    MissingFencingToken,
}

impl ReplayWindowCommand {
    /// validate —— 无副作用校验(身份/对象/hash/窗口/限额/fencing)。
    pub fn validate(&self) -> Result<ValidatedReplayWindow, ReplayWindowError> {
        if self.tenant_id.trim().is_empty() {
            return Err(ReplayWindowError::MissingTenant);
        }
        if self.task_id.trim().is_empty()
            || self.run_id.trim().is_empty()
            || self.probe_id.trim().is_empty()
        {
            return Err(ReplayWindowError::IncompleteRunIdentity);
        }
        if self.execution_spec_sha256.trim().is_empty() {
            return Err(ReplayWindowError::MissingExecutionSpec);
        }
        let rest = match self.object_ref.strip_prefix("s3://") {
            Some(rest) => rest,
            None => return Err(ReplayWindowError::InvalidObjectRef),
        };
        let mut parts = rest.splitn(2, '/');
        if parts.next().unwrap_or("").is_empty() || parts.next().unwrap_or("").is_empty() {
            return Err(ReplayWindowError::InvalidObjectRef);
        }
        if self.object_sha256.len() != 64 || !self.object_sha256.chars().all(|c| c.is_ascii_hexdigit()) {
            return Err(ReplayWindowError::InvalidObjectSha256);
        }
        if self.window_end_ms <= self.window_start_ms {
            return Err(ReplayWindowError::InvalidWindow);
        }
        if self.packet_limit == 0 && self.byte_limit == 0 {
            return Err(ReplayWindowError::UnboundedReplay);
        }
        if self.interface.as_deref().is_some_and(|s| s.trim().is_empty()) {
            return Err(ReplayWindowError::InvalidWireInterface);
        }
        if self.fencing_token.trim().is_empty() {
            return Err(ReplayWindowError::MissingFencingToken);
        }
        Ok(ValidatedReplayWindow {
            command: self.clone(),
        })
    }
}

#[cfg(test)]
mod replay_window_tests {
    use super::*;

    fn valid_command() -> ReplayWindowCommand {
        ReplayWindowCommand {
            tenant_id: "default".into(),
            task_id: "task-1".into(),
            run_id: "run-1".into(),
            execution_spec_sha256: "spec-1".into(),
            probe_id: "probe-agent".into(),
            object_ref: "s3://analysis-bench/pcap/x.pcap".into(),
            object_sha256: "f4c4c59d460c6748110e6a0288c84a1362d7a3fa8a34b7a5cbe423de9e903deb".into(),
            interface: None,
            window_start_ms: 1,
            window_end_ms: 1000,
            packet_limit: 1000,
            byte_limit: 0,
            fencing_token: "fence-1".into(),
        }
    }

    #[test]
    fn valid_command_passes() {
        assert!(valid_command().validate().is_ok());
    }

    #[test]
    fn rejects_empty_wire_interface() {
        let mut c = valid_command();
        c.interface = Some("   ".into());
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::InvalidWireInterface);
        let mut c2 = valid_command();
        c2.interface = Some("ta-veth-in".into());
        assert!(c2.validate().is_ok(), "wire interface must pass validation; allowlist is enforced by the executor");
    }

    #[test]
    fn rejects_missing_identity() {
        for mut c in [valid_command(), valid_command()] {
            c.task_id.clear();
            assert_eq!(c.validate().unwrap_err(), ReplayWindowError::IncompleteRunIdentity);
            break;
        }
        let mut c = valid_command();
        c.tenant_id.clear();
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::MissingTenant);
    }

    #[test]
    fn rejects_non_s3_object_ref() {
        let mut c = valid_command();
        c.object_ref = "file:///tmp/x.pcap".into();
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::InvalidObjectRef);
        let mut c2 = valid_command();
        c2.object_ref = "s3://bucketonly".into();
        assert_eq!(c2.validate().unwrap_err(), ReplayWindowError::InvalidObjectRef);
    }

    #[test]
    fn rejects_bad_sha256() {
        let mut c = valid_command();
        c.object_sha256 = "xyz".into();
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::InvalidObjectSha256);
        let mut c2 = valid_command();
        c2.object_sha256 = "g".repeat(64);
        assert_eq!(c2.validate().unwrap_err(), ReplayWindowError::InvalidObjectSha256);
    }

    #[test]
    fn rejects_unbounded_replay() {
        let mut c = valid_command();
        c.packet_limit = 0;
        c.byte_limit = 0;
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::UnboundedReplay);
    }

    #[test]
    fn rejects_invalid_window_and_fencing() {
        let mut c = valid_command();
        c.window_end_ms = c.window_start_ms;
        assert_eq!(c.validate().unwrap_err(), ReplayWindowError::InvalidWindow);
        let mut c2 = valid_command();
        c2.fencing_token.clear();
        assert_eq!(c2.validate().unwrap_err(), ReplayWindowError::MissingFencingToken);
    }
}
