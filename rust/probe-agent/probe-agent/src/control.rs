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
}

impl BuiltinProbeExecutor {
    pub fn for_gateway(gateway_addr: &str) -> Result<Self> {
        let endpoint = tcp_endpoint(gateway_addr)?;
        Ok(Self {
            connectivity_targets: BTreeMap::from([("ingest-gateway".to_string(), endpoint)]),
            connect_timeout: std::time::Duration::from_secs(5),
        })
    }

    #[cfg(test)]
    fn with_targets(connectivity_targets: BTreeMap<String, String>) -> Self {
        Self {
            connectivity_targets,
            connect_timeout: std::time::Duration::from_secs(1),
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

#[async_trait]
impl ProbeOperationExecutor for BuiltinProbeExecutor {
    async fn execute(&self, operation_type: &str, command: &Value) -> Result<OperationExecution> {
        match operation_type {
            "connectivity_test" => self.connectivity_test(command).await,
            _ => FailClosedExecutor.execute(operation_type, command).await,
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
        let command_value: Value = serde_json::from_slice(&command.command_json)
            .context("command_json is not valid JSON")?;

        let execution = if command.expires_at_ms <= now_ms {
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
        Value::String(value) => serde_json::to_string(value).expect("string serialization"),
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
