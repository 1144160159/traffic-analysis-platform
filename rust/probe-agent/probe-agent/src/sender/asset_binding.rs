use anyhow::{bail, Context, Result};
use prost::Message;
use proto_gen::{AssetBindingItemDisposition, MacIpBinding, UploadAssetBindingsResponse};
use sha2::{Digest, Sha256};
use sled::Db;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use crate::parser::{AssetBindingObservation, AssetBindingSink};

const PENDING_PREFIX: &[u8] = b"binding:";
const REJECTED_PREFIX: &[u8] = b"rejected:";

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct AssetBindingRef {
    key: Vec<u8>,
    pub observation_id: String,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct AssetBindingAckApplication {
    pub terminal_success: usize,
    pub terminal_rejected: usize,
    pub retained_retryable: usize,
}

#[derive(Clone)]
pub struct AssetBindingSpool {
    db: Arc<Db>,
    db_path: PathBuf,
    max_entries: usize,
}

#[derive(Clone)]
pub struct DurableAssetBindingSink {
    spool: Arc<AssetBindingSpool>,
    tenant_id: String,
    probe_id: String,
}

impl DurableAssetBindingSink {
    pub fn new(
        spool: Arc<AssetBindingSpool>,
        tenant_id: impl Into<String>,
        probe_id: impl Into<String>,
    ) -> Result<Self> {
        let tenant_id = tenant_id.into();
        let probe_id = probe_id.into();
        if tenant_id.trim().is_empty() || probe_id.trim().is_empty() {
            bail!("durable asset binding sink requires tenant_id and probe_id")
        }
        Ok(Self {
            spool,
            tenant_id,
            probe_id,
        })
    }

    fn binding_from_observation(&self, observation: AssetBindingObservation) -> MacIpBinding {
        let vlan_id = observation
            .vlan_id
            .map(|value| value.to_string())
            .unwrap_or_default();
        let identity = format!(
            "{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}",
            self.tenant_id,
            self.probe_id,
            observation.source,
            observation.mac,
            observation.ip,
            observation.observed_at_ms,
            vlan_id,
            observation.source_event_id,
        );
        let digest = Sha256::digest(identity.as_bytes());
        MacIpBinding {
            mac_address: observation.mac,
            ip_address: observation.ip,
            tenant_id: self.tenant_id.clone(),
            observed_at: observation.observed_at_ms,
            source: observation.source.to_owned(),
            observation_id: format!("asset-binding-v1:{digest:x}"),
            probe_id: self.probe_id.clone(),
            vlan_id,
            source_event_id: observation.source_event_id,
            schema_version: 1,
        }
    }
}

impl AssetBindingSink for DurableAssetBindingSink {
    fn persist(&self, observation: AssetBindingObservation) -> Result<()> {
        self.spool
            .persist(&self.binding_from_observation(observation))?;
        Ok(())
    }
}

impl AssetBindingSpool {
    pub fn open(cache_root: &Path, max_entries: usize) -> Result<Self> {
        if max_entries == 0 {
            bail!("asset binding spool max_entries must be positive")
        }
        let db_path = cache_root.join("asset_binding_spool_v1");
        let db = sled::Config::new()
            .path(&db_path)
            .mode(sled::Mode::HighThroughput)
            .flush_every_ms(Some(100))
            .use_compression(false)
            .open()
            .context("failed to open asset binding spool")?;
        Ok(Self {
            db: Arc::new(db),
            db_path,
            max_entries,
        })
    }

    pub fn persist(&self, binding: &MacIpBinding) -> Result<AssetBindingRef> {
        validate_binding_identity(binding)?;
        let key = pending_key(binding);
        let bytes = binding.encode_to_vec();
        if let Some(existing) = self.db.get(&key)? {
            if existing.as_ref() != bytes.as_slice() {
                bail!("asset binding identity conflicts with different payload bytes")
            }
            return Ok(AssetBindingRef {
                key,
                observation_id: binding.observation_id.clone(),
            });
        }
        if self.pending_count()? >= self.max_entries {
            bail!(
                "asset binding spool is full (max_entries={})",
                self.max_entries
            )
        }
        match self
            .db
            .compare_and_swap(&key, None as Option<&[u8]>, Some(bytes.as_slice()))?
        {
            Ok(()) => {}
            Err(conflict) if conflict.current.as_deref() == Some(bytes.as_slice()) => {}
            Err(_) => bail!("asset binding identity raced with different payload bytes"),
        }
        self.db.flush()?;
        Ok(AssetBindingRef {
            key,
            observation_id: binding.observation_id.clone(),
        })
    }

    pub fn pending(&self, limit: usize) -> Result<Vec<(AssetBindingRef, MacIpBinding)>> {
        let mut result = Vec::new();
        let mut corrupt = Vec::new();
        for entry in self.db.scan_prefix(PENDING_PREFIX) {
            if result.len() >= limit {
                break;
            }
            let (key, value) = entry?;
            match MacIpBinding::decode(value.as_ref()) {
                Ok(binding) if validate_binding_identity(&binding).is_ok() => {
                    result.push((
                        AssetBindingRef {
                            key: key.to_vec(),
                            observation_id: binding.observation_id.clone(),
                        },
                        binding,
                    ));
                }
                _ => corrupt.push((key.to_vec(), value.to_vec())),
            }
        }
        for (key, value) in corrupt {
            let quarantine_key = rejected_key(&key, "CORRUPT_LOCAL_WAL");
            self.db.insert(quarantine_key, value)?;
            self.db.remove(key)?;
        }
        if !result.is_empty() {
            self.db.flush()?;
        }
        Ok(result)
    }

    pub fn apply_response(
        &self,
        batch: &[(AssetBindingRef, MacIpBinding)],
        response: &UploadAssetBindingsResponse,
    ) -> Result<AssetBindingAckApplication> {
        if response.response_revision != 1 {
            bail!(
                "unsupported asset binding response revision {}",
                response.response_revision
            )
        }
        if response.item_results.len() != batch.len() {
            bail!(
                "asset binding response cardinality mismatch: got {} want {}",
                response.item_results.len(),
                batch.len()
            )
        }
        let mut ordered = vec![None; batch.len()];
        for item in &response.item_results {
            let index = item.input_index as usize;
            if index >= batch.len() || ordered[index].is_some() {
                bail!("asset binding response contains duplicate or invalid input_index")
            }
            if item.observation_id != batch[index].0.observation_id {
                bail!("asset binding response observation identity mismatch")
            }
            let disposition = AssetBindingItemDisposition::try_from(item.disposition)
                .map_err(|_| anyhow::anyhow!("asset binding response disposition is invalid"))?;
            if disposition == AssetBindingItemDisposition::Unspecified {
                bail!("asset binding response disposition is unspecified")
            }
            if matches!(
                disposition,
                AssetBindingItemDisposition::KafkaAcked
                    | AssetBindingItemDisposition::DuplicateCommitted
            ) && item.ack_scope != "KAFKA_RECORD"
            {
                bail!("terminal asset binding success lacks KAFKA_RECORD scope")
            }
            ordered[index] = Some((disposition, item.reason_code.as_str()));
        }

        let mut applied = AssetBindingAckApplication::default();
        for (index, outcome) in ordered.into_iter().enumerate() {
            let (disposition, reason) = outcome.expect("exact-set validation populated every item");
            match disposition {
                AssetBindingItemDisposition::KafkaAcked
                | AssetBindingItemDisposition::DuplicateCommitted => {
                    self.db.remove(&batch[index].0.key)?;
                    applied.terminal_success += 1;
                }
                AssetBindingItemDisposition::RejectedInvalid => {
                    let value = self
                        .db
                        .get(&batch[index].0.key)?
                        .ok_or_else(|| anyhow::anyhow!("pending asset binding disappeared"))?;
                    self.db
                        .insert(rejected_key(&batch[index].0.key, reason), value)?;
                    self.db.remove(&batch[index].0.key)?;
                    applied.terminal_rejected += 1;
                }
                AssetBindingItemDisposition::Retryable
                | AssetBindingItemDisposition::OutcomeUnknown => {
                    applied.retained_retryable += 1;
                }
                AssetBindingItemDisposition::Unspecified => unreachable!(),
            }
        }
        self.db.flush()?;
        Ok(applied)
    }

    pub fn pending_count(&self) -> Result<usize> {
        Ok(self.db.scan_prefix(PENDING_PREFIX).count())
    }

    pub fn rejected_count(&self) -> Result<usize> {
        Ok(self.db.scan_prefix(REJECTED_PREFIX).count())
    }

    pub fn path(&self) -> &Path {
        &self.db_path
    }
}

fn validate_binding_identity(binding: &MacIpBinding) -> Result<()> {
    if binding.tenant_id.trim().is_empty()
        || binding.probe_id.trim().is_empty()
        || binding.observation_id.trim().is_empty()
        || binding.mac_address.trim().is_empty()
        || binding.ip_address.trim().is_empty()
        || binding.observed_at <= 0
        || !matches!(binding.source.as_str(), "arp" | "dhcp")
        || binding.schema_version != 1
    {
        bail!("asset binding stable identity or payload contract is incomplete")
    }
    Ok(())
}

fn pending_key(binding: &MacIpBinding) -> Vec<u8> {
    let identity = format!(
        "{}\0{}\0{}",
        binding.tenant_id, binding.probe_id, binding.observation_id
    );
    let digest = Sha256::digest(identity.as_bytes());
    format!("binding:{digest:x}").into_bytes()
}

fn rejected_key(pending_key: &[u8], reason: &str) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(pending_key);
    hasher.update([0]);
    hasher.update(reason.as_bytes());
    format!("rejected:{:x}", hasher.finalize()).into_bytes()
}

#[cfg(test)]
mod tests {
    use super::*;
    use proto_gen::AssetBindingItemResult;

    fn binding(observation_id: &str) -> MacIpBinding {
        MacIpBinding {
            mac_address: "00:11:22:33:44:55".to_owned(),
            ip_address: "10.0.0.8".to_owned(),
            tenant_id: "tenant-a".to_owned(),
            observed_at: 1_700_000_000_000,
            source: "arp".to_owned(),
            observation_id: observation_id.to_owned(),
            probe_id: "probe-a".to_owned(),
            vlan_id: "10".to_owned(),
            source_event_id: "packet-a".to_owned(),
            schema_version: 1,
        }
    }

    fn item(
        index: u32,
        id: &str,
        disposition: AssetBindingItemDisposition,
    ) -> AssetBindingItemResult {
        AssetBindingItemResult {
            input_index: index,
            observation_id: id.to_owned(),
            disposition: disposition as i32,
            reason_code: "TEST".to_owned(),
            ack_scope: if matches!(
                disposition,
                AssetBindingItemDisposition::KafkaAcked
                    | AssetBindingItemDisposition::DuplicateCommitted
            ) {
                "KAFKA_RECORD".to_owned()
            } else {
                "INPUT_ITEM".to_owned()
            },
        }
    }

    #[test]
    fn spool_survives_reopen_and_rejects_identity_conflict_or_capacity_overflow() {
        let dir = tempfile::tempdir().unwrap();
        {
            let spool = AssetBindingSpool::open(dir.path(), 1).unwrap();
            spool.persist(&binding("obs-1")).unwrap();
            spool.persist(&binding("obs-1")).unwrap();
            let mut conflict = binding("obs-1");
            conflict.ip_address = "10.0.0.9".to_owned();
            assert!(spool.persist(&conflict).is_err());
            assert!(spool.persist(&binding("obs-2")).is_err());
        }
        let reopened = AssetBindingSpool::open(dir.path(), 1).unwrap();
        assert_eq!(reopened.pending_count().unwrap(), 1);
    }

    #[test]
    fn exact_ack_removes_only_terminal_items_and_quarantines_rejection() {
        let dir = tempfile::tempdir().unwrap();
        let spool = AssetBindingSpool::open(dir.path(), 10).unwrap();
        for id in ["obs-ok", "obs-reject", "obs-retry"] {
            spool.persist(&binding(id)).unwrap();
        }
        let batch = spool.pending(10).unwrap();
        let mut results = Vec::new();
        for (index, (reference, _)) in batch.iter().enumerate() {
            let disposition = match reference.observation_id.as_str() {
                "obs-ok" => AssetBindingItemDisposition::KafkaAcked,
                "obs-reject" => AssetBindingItemDisposition::RejectedInvalid,
                _ => AssetBindingItemDisposition::Retryable,
            };
            results.push(item(index as u32, &reference.observation_id, disposition));
        }
        let applied = spool
            .apply_response(
                &batch,
                &UploadAssetBindingsResponse {
                    accepted: 1,
                    rejected: 1,
                    item_results: results,
                    response_revision: 1,
                    message: String::new(),
                },
            )
            .unwrap();
        assert_eq!(applied.terminal_success, 1);
        assert_eq!(applied.terminal_rejected, 1);
        assert_eq!(applied.retained_retryable, 1);
        assert_eq!(spool.pending_count().unwrap(), 1);
        assert_eq!(spool.rejected_count().unwrap(), 1);
    }

    #[test]
    fn malformed_response_never_mutates_the_spool() {
        let dir = tempfile::tempdir().unwrap();
        let spool = AssetBindingSpool::open(dir.path(), 10).unwrap();
        spool.persist(&binding("obs-1")).unwrap();
        let batch = spool.pending(10).unwrap();
        let response = UploadAssetBindingsResponse {
            accepted: 1,
            rejected: 0,
            item_results: vec![],
            response_revision: 1,
            message: String::new(),
        };
        assert!(spool.apply_response(&batch, &response).is_err());
        assert_eq!(spool.pending_count().unwrap(), 1);
    }

    #[test]
    fn probe_observation_identity_is_deterministic_and_persisted_before_send() {
        let dir = tempfile::tempdir().unwrap();
        let spool = Arc::new(AssetBindingSpool::open(dir.path(), 10).unwrap());
        let sink = DurableAssetBindingSink::new(spool.clone(), "tenant-a", "probe-a").unwrap();
        let observation = AssetBindingObservation {
            mac: "00:11:22:33:44:55".to_owned(),
            ip: "10.0.0.8".to_owned(),
            observed_at_ms: 1_700_000_000_000,
            source: "arp",
            vlan_id: Some(10),
            source_event_id: "arp:Request".to_owned(),
        };
        sink.persist(observation.clone()).unwrap();
        sink.persist(observation).unwrap();
        let pending = spool.pending(10).unwrap();
        assert_eq!(pending.len(), 1);
        assert!(pending[0].1.observation_id.starts_with("asset-binding-v1:"));
    }
}
